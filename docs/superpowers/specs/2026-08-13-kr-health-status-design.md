# Health-статус для Key Result — дизайн

Дата: 2026-08-13

## 1. Цель и контекст

Дать возможность вручную задавать **health-статус** каждого Key Result при
обновлении прогресса и показывать этот статус в списке KR.

Значения статуса (closed set):

| Значение      | Ярлык (UI)    | Хинт                                             |
|---------------|---------------|--------------------------------------------------|
| `not_started` | Not Started   | команда не приступила к этому KR                  |
| `on_track`    | On Track      | началась работа, идёт планово                    |
| `at_risk`     | At Risk       | фиксируем существенный риск для достижения результата |
| `done`        | Done          | работа над KR завершена                          |

Правило: **прогресс KR = 100% автоматически проставляет статус `done`** —
однократно, при переходе прогресса `<100 → =100`; далее статус можно менять вручную.

### Отношение к существующим статусам

- Это **не** `TeamPeriodStatus` (`no_goals`/`forming`/`ready`/`in_progress`/`closed`) —
  тот описывает жизненный цикл команды в периоде.
- Это **не** forecast-based `progress_meta.status` (`below`/`on_track`/`above`), который
  вычисляется сервером из доли прошедшего времени периода. Несмотря на пересечение слова
  `on_track`, это другое поле — новое `health_status` задаётся **вручную** и хранится в БД.
  Во избежание путаницы на фронтенде существующий forecast-набор (`HEALTH_COLOR`) не
  переиспользуется; вводятся отдельные константы `KR_HEALTH_*`.

## 2. Ответы на обязательные вопросы к новой mutation-фиче (см. `050-permissions-and-lifecycle.md`)

- **Зависит ли от `team period status`** — нет отдельной зависимости; обновление health идёт
  тем же путём, что и обновление прогресса KR (проверка доступа к команде-владельцу).
- **Разрешена ли в `validated`** — да, как и обновление прогресса (серверных lifecycle-ограничений
  на прогресс сейчас нет).
- **Разрешена ли в `closed`** — да, как и обновление прогресса (в UI режим «только комментарии»,
  серверного запрета на прогресс нет).
- **Проверяется ли на сервере** — да: доступ к команде-владельцу (`CanAccessTeamFromCtx` — как у
  progress-эндпоинтов), валидация значения статуса по closed-set.
- **Зависит ли от будущих permissions / roles** — нет; опирается на текущую модель tenant +
  hierarchy grants, как и обновление прогресса.

## 3. Домен и данные

### 3.1 Доменный тип

`internal/domain/models.go` — по образцу `KRKind` / `KRUnits` / `IsValidKRUnit`:

```go
type KRHealthStatus string

const (
    KRHealthNotStarted KRHealthStatus = "not_started"
    KRHealthOnTrack    KRHealthStatus = "on_track"
    KRHealthAtRisk     KRHealthStatus = "at_risk"
    KRHealthDone       KRHealthStatus = "done"
)

func IsValidKRHealthStatus(s string) bool { /* по образцу IsValidKRUnit */ }
```

Поле на `KeyResult`:

```go
HealthStatus KRHealthStatus // ~рядом с Progress
```

### 3.2 Инварианты (дополнение к `020-domain-model.md`, раздел KeyResult)

- `health_status` — обязательное поле KR из закрытого справочника
  (`not_started` | `on_track` | `at_risk` | `done`), значение по умолчанию `not_started`.
- health-статус **не влияет** на расчёт прогресса KR/goal/period — чисто информационный сигнал.
- при переходе прогресса KR `<100% → =100%` сервер однократно выставляет `done`
  (если статус ещё не `done`); последующие ручные изменения не откатываются, повторное
  сохранение на 100% и падение прогресса ниже 100% статус не меняют.

### 3.3 Миграция `042_kr_health_status`

up:

```sql
ALTER TABLE key_results
  ADD COLUMN IF NOT EXISTS health_status TEXT NOT NULL DEFAULT 'not_started';

-- backfill: уже завершённые (100%) KR получают done
UPDATE key_results SET health_status = 'done'
WHERE <progress == 100>;
```

Прогресс не хранится колонкой — он вычисляемый. Backfill выполняется по правилам расчёта
прогресса для каждого kind (см. `020-domain-model.md`, «Производные вычисления»):

- BOOLEAN: `is_done = true`;
- NUMERICAL: `current_value` достиг цели (с учётом направления / checkpoints);
- PROJECT: сумма весов завершённых этапов ≥ 100.

Реализация backfill'а — набор `UPDATE ... WHERE` по kind (или единый вычисляемый предикат в SQL),
чтобы не тянуть Go-логику в миграцию; конкретные предикаты фиксируются в плане реализации и
покрываются тестом миграции на демо-данных.

down: `ALTER TABLE key_results DROP COLUMN IF EXISTS health_status;`

### 3.4 Seed / demo

- `seed_demo.sql`: NOT NULL DEFAULT покрывает существующие `INSERT`; для наглядности демо —
  проставить разные health-статусы части KR (в т.ч. `done` для KR со 100%).
- `internal/store/seed.go`: заполнить `HealthStatus` при построении `domain.KeyResult`
  (иначе Go-структура уедет с пустым значением — для консистентности выставляем явно).

## 4. Слои backend

### 4.1 Repository (`internal/store/krs/krs.go`)

- SELECT-списки в `ListKeyResultsByGoal` и `GetKeyResult` — добавить колонку `health_status` +
  соответствующие scan-таргеты.
- Новый метод `UpdateHealthStatus(ctx, krID, status)` — отдельный `UPDATE key_results SET
  health_status=$1, updated_at=NOW() WHERE id=$2`. **Не** смешивается с `UpdateNumericalCurrent` /
  `UpdateBoolean` / `BatchUpdateProjectStagesDone`.
- `CreateKeyResult` / `KeyResultInput`: новый KR создаётся со статусом `not_started` (значение
  по умолчанию; отдельного поля во входной форме создания KR не добавляем в этой итерации).

### 4.2 Service (`internal/service/service.go`)

- Новый метод `UpdateKRHealthStatus(ctx, krID, status)`:
  - валидирует `IsValidKRHealthStatus` (иначе доменная ошибка валидации);
  - проверяет доступ к команде-владельцу (как в progress-методах);
  - вызывает `krs.UpdateHealthStatus`.
- Правило 100%→done встраивается в существующие `UpdateKRProgressNumerical` /
  `UpdateKRProgressBoolean` / `UpdateKRProgressProject`:
  - до применения прогресса известен «старый» прогресс, после — «новый»;
  - если `old < 100 && new == 100 && currentHealth != done` → `krs.UpdateHealthStatus(kr, done)`.
  - Это единственный источник авто-Done, kind-agnostic, «однократность» обеспечивается условием
    перехода (`old < 100`).

### 4.3 HTTP handler (`internal/http/handlers/api/v1/krs/handler.go`)

Расширяем **существующие** progress-хендлеры, новых роутов нет:

- request-структуры `HandleUpdateNumericalProgress` / `Boolean` / `Project` получают опциональное
  поле:
  ```go
  HealthStatus *string `json:"health_status"`
  ```
- Порядок обработки в каждом хендлере:
  1. `service.UpdateKRProgress*` (внутри — возможный авто-Done на переходе к 100%);
  2. если `req.HealthStatus != nil` → `service.UpdateKRHealthStatus(...)` — **перетирает** авто-Done
     явным ручным выбором (гарантия «ручной побеждает»);
  3. вернуть актуальный KR.
- Невалидное значение `health_status` → `400 VALIDATION_ERROR`.

Это ровно то разделение, что просил пользователь: **один API-вызов**, но **отдельный метод**
обновления health внутри обработки; `health_status == nil` → health не трогаем.

### 4.4 DTO (`internal/http/dto/kr.go` + `helpers_response.go`)

- `dto.KeyResult` получает поле `HealthStatus string \`json:"health_status"\``.
- Маппинг domain→DTO в `helpers_response.go` (`dto.KeyResult{...}`) заполняет его.

## 5. Frontend (`internal/web/static/tracker.js`)

### 5.1 Модель и константы

- `mapKR`: добавить `healthStatus: kr.health_status`.
- Новые константы (не путать с forecast `HEALTH_COLOR`):
  - `KR_HEALTH_LABEL = { not_started:'Not Started', on_track:'On Track', at_risk:'At Risk', done:'Done' }`
  - `KR_HEALTH_COLOR` — цвета по скриншоту: `not_started` серый, `on_track` зелёный,
    `at_risk` янтарный, `done` залитый зелёный.
  - `KR_HEALTH_HINT` — хинты из раздела 1 (для карточек в модалке).
  - `KR_HEALTH_OPTIONS` — порядок: Not Started → On Track → At Risk → Done.

### 5.2 `KRRow` — бейдж в списке

- Health-бейдж (переиспользуем компонент `Badge`) слева перед заголовком KR — виден в списке
  (как на скриншоте). Стиль пилюли консистентен с `Badge`/`PriBadge`.

### 5.3 `KRProgressModal` — секция «Health статус»

- Между блоком прогресса и «Заметкой» — секция «Health статус» с 4 выбираемыми карточками
  (label + hint), выделение выбранной карточки — как на скриншоте.
- Состояние: `health` в `form`, флаг `healthTouched` (пользователь кликнул карточку).
- Клиентское авто-Done: когда live-`progress` (`calcKRProgress(form)`) достигает 100%, а
  пользователь не трогал селектор — карточка Done показывается выбранной (визуальное отражение
  серверного правила).
- `save()`: шлёт **один** POST на progress-эндпоинт соответствующего kind; поле `health_status`
  добавляется в body **только если** `healthTouched === true`. Если не трогали — поле опускается
  (`nil` на сервере), серверное правило 100%→done отрабатывает само.

## 6. Тесты

- **domain**: `IsValidKRHealthStatus` (валидные/невалидные значения).
- **service**:
  - авто-Done на переходе `<100→100` для NUMERICAL / BOOLEAN / PROJECT;
  - отсутствие срабатывания при повторном сохранении на 100% и при падении ниже 100%;
  - явный `health_status` в progress-запросе перетирает авто-Done (ручной побеждает);
  - `health_status == nil` не меняет статус;
  - невалидный статус → ошибка валидации.
- **repo**: чтение колонки в `ListKeyResultsByGoal`/`GetKeyResult`; `UpdateHealthStatus` пишет значение.
- **handler**: валидный/невалидный `health_status`; порядок «progress → health»; доступ к команде.
- **migration**: backfill проставляет `done` только KR со 100% на демо-данных.

## 7. Обновление проектных спеков (в том же change set при реализации)

- `020-domain-model.md` — раздел KeyResult: новое поле `health_status`, тип, инварианты, правило
  100%→done; отметка, что health не входит в rollup прогресса.
- `040-api-contract.md` — раздел «Update KR progress»: опциональное поле `health_status` в body
  numerical/boolean/project; `health_status` в response KR.
- `050-permissions-and-lifecycle.md` — блок ответов из раздела 2 этого дизайна («Требование к новым фичам»).

Не трогаем несвязанные спеки.

## 8. Вне scope (v1)

- **Activity log** ручной смены health (потенциальный `kr_health_changed`) — не в v1.
- Markdown-экспорт целей health-статус не включает.
- Фильтрация/группировка/обзорные агрегаты по health-статусу — не в этой итерации.
- Отдельное поле health в форме создания/полного редактирования KR — не в этой итерации
  (новый KR создаётся `not_started`, статус задаётся через модалку прогресса).
