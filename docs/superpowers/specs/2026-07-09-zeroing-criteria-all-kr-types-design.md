# Критерий обнуления для всех типов Key Result

Дата: 2026-07-09
Ветка: `zeroing_criterion`

## Проблема

Критерий обнуления (`zeroing_criteria`) — опциональное человекочитаемое поле KR,
которое описывает условие, при котором результат обнуляется. В расчётах прогресса
не участвует (см. [specs/020-domain-model.md](../../../specs/020-domain-model.md)).

Сейчас поле доступно только для KR типа `NUMERICAL`:

- в domain оно лежит на `KRNumerical`, в DTO — на `NumericalMeasure`;
- пишется только внутри `UpsertNumericalMeta` (UPDATE числовых колонок);
- при чтении маппится в структуру только при `kind == NUMERICAL`;
- form-поле называется `numerical_zeroing`, парсится только в `ParseNumericalMeta`;
- в UI блок «Критерий обнуления» отрисован внутри секции NUMERICAL формы KR.

При этом DB-колонка `key_results.zeroing_criteria` (`TEXT NOT NULL DEFAULT ''`,
миграция `023_kr_numerical`) физически существует для всех строк `key_results`
независимо от типа.

## Цель

Сделать критерий обнуления доступным для **всех** типов KR (`NUMERICAL`,
`BOOLEAN`, `PROJECT`). Существующие критерии у числовых KR не должны пострадать:
данные в колонке остаются нетронутыми, миграция данных не требуется.

## Решение

Критерий обнуления — это generic-свойство самого KR, а не его measure, и колонка
уже находится на уровне `key_results`. Поэтому поднимаем поле из measure на
уровень `KeyResult` (рядом с `Title`/`Description`/`Weight`).

Миграции БД нет — колонка уже есть. Меняется только код чтения/записи, форма JSON
API, имя form-поля и расположение UI-блока.

### Слой domain

`internal/domain/models.go`:

- удалить `ZeroingCriteria` из `KRNumerical`;
- добавить `ZeroingCriteria string` в `KeyResult`.

### Слой DTO

`internal/http/dto/kr.go`:

- удалить `ZeroingCriteria` из `NumericalMeasure`;
- добавить `ZeroingCriteria string \`json:"zeroing_criteria,omitempty"\`` в
  `KeyResult` (top-level).

Форма JSON меняется: `measure.numerical.zeroing_criteria` → top-level
`zeroing_criteria`. Единственный потребитель API — фронтенд `tracker.js`,
обновляется в том же change set.

### Слой store

`internal/store/krs/krs.go`:

- Чтение: колонка `zeroing_criteria` уже в SELECT (`ListKeyResultsByGoal`,
  `GetKeyResult` и парный запрос в `internal/store/goals/goals.go`). Маппить её
  в `kr.ZeroingCriteria` для **любого** kind, а не только NUMERICAL. Убрать
  параметр/обработку zeroing из `scanNumerical`.
- Запись: добавить `ZeroingCriteria` в `KeyResultInput` и
  `KeyResultUpdateInput`; писать значение в INSERT (`CreateKeyResult`) и UPDATE
  (`UpdateKeyResult`) базовой таблицы `key_results`.
- Удалить `ZeroingCriteria` из `NumericalMetaInput` и убрать `zeroing_criteria`
  из UPDATE в `UpsertNumericalMeta` (числовой upsert больше не трогает колонку).

Проверить парный SELECT в `internal/store/goals/goals.go` и обновить его маппинг
аналогично.

### Слой service

`internal/service/service.go`:

- удалить `ZeroingCriteria` из `KeyResultMetaInput`;
- `applyKeyResultMeta` больше не передаёт zeroing в `NumericalMetaInput`;
- значение критерия попадает в `krs.KeyResultInput` / `KeyResultUpdateInput`
  на уровне `CreateKeyResultWithMeta` / `UpdateKeyResultWithMeta` (или у их
  вызывающих обработчиков — см. ниже).

### Слой handlers

Form-поле переименовать из `numerical_zeroing` в `zeroing_criteria` и парсить на
уровне базового KR-инпута (независимо от kind) во всех путях создания/обновления
KR:

- `internal/http/handlers/api/v1/krs` (create + update);
- `internal/http/handlers/web/keyresults/handler.go`;
- `internal/http/handlers/web/goals/handler.go`.

Убрать `ZeroingCriteria` из `common.ParseNumericalMeta`
(`internal/http/handlers/web/common/common.go`). Добавить общий парс trimmed
`zeroing_criteria` там, где формируется `KeyResultInput`/`KeyResultUpdateInput`.

Ответ API (`internal/http/handlers/api/v1/helpers_response.go`): убрать
`ZeroingCriteria` из `buildMeasure` (ветка numerical), выставлять top-level
`ZeroingCriteria` в `buildKeyResult`.

### Frontend

`internal/web/static/tracker.js`:

- `mapKR`: читать `zeroing` из `kr.zeroing_criteria` (top-level), а не из
  `m.numerical.zeroing_criteria`.
- В `save`: отправлять `zeroing_criteria` (переименовать с `numerical_zeroing`)
  для любого типа KR.
- В форме KR: вынести блок «⊘ Критерий обнуления» (кнопка-раскрытие + textarea)
  из секции `form.krType === 'NUMERICAL'` в общую часть — после всех
  type-специфичных секций и перед футером модалки, чтобы блок был виден для
  NUMERICAL, BOOLEAN и PROJECT. Поведение (кнопка → textarea, `showZeroing`)
  сохранить как есть.

Read-only отображение критерия (`Критерий обнуления: {form.zeroing}`) уже
использует mapped-поле `zeroing` — начнёт работать для всех типов автоматически.

### Спеки

- `specs/020-domain-model.md`: описать `zeroing_criteria` как поле уровня KR
  (для всех kind), а не свойство числового measure.
- `specs/040-api-contract.md`: в описании KR перенести `zeroing_criteria` на
  top-level; в разделе create/update заменить `numerical_zeroing` (только для
  NUMERICAL) на общий form-параметр `zeroing_criteria` для всех kind.

### Тесты

- `internal/http/handlers/api/v1/helpers_response_test.go`: проверять
  `ZeroingCriteria` на top-level `KeyResult`, а не на `NumericalMeasure`.
  Добавить/поправить кейс, что для BOOLEAN/PROJECT top-level поле проксируется.
- `internal/store/krs/krs_test.go`: добавить проверку записи и чтения
  `zeroing_criteria` для BOOLEAN и PROJECT KR (round-trip), и что существующий
  NUMERICAL round-trip не сломан.

### Seed

`seed_demo.sql`: добавить демонстрационный критерий обнуления на один
не-числовой KR (BOOLEAN или PROJECT), чтобы фича была видна в демо. Структура
таблиц не меняется, существующий UPDATE для числового KR оставить.

## Что НЕ делаем

- Не мигрируем данные (колонка и содержимое не меняются).
- Не меняем логику расчёта прогресса — критерий по-прежнему человекочитаемый и в
  расчётах не применяется.
- Не трогаем несвязанные спеки.

## Совместимость и риски

- Существующие числовые критерии сохраняются: та же колонка, тот же контент,
  меняется только код чтения/записи и место поля в JSON.
- Смена формы JSON и имени form-поля безопасна: единственный клиент API —
  `tracker.js`, обновляется в этом же change set.
- Внешних потребителей API нет, версия API не бампится.
