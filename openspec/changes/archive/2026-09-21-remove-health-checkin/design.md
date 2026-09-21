## Context

Мотивация описана в proposal.md, раздел Why. Пакеты Health Check-in не самодостаточны: в них
живёт код, на котором держатся другие функции.

- **Кэш данных периода** (`internal/service/healthcheckin/cache.go`):
  - типы `Cache`, `PeriodData`, `Active`;
  - загрузчик `internal/usecase/healthcheckin.NewPeriodLoader`;
  - вспомогательные функции `BuildTeamPath` и `Abs`.

  Этот кэш читают обзор периода и метрики периодов (`usecase/period/overview.go`), снимки
  прогресса (`usecase/period/progress.go`) и массовая смена статусов (`bulkstatus.go`, делает
  `InvalidateAll`). Фоновый цикл обновления запускает `internal/scheduler`.
- **`LoadConfig` / `Config`** — единственный читатель JSON-ключа `health_checkin_config`. Из него
  берутся:
  - поля `stale_days`, `behind_margin` и `green_threshold` в `GET /api/v1/config`, которые
    использует `web/static/tracker.js`;
  - допуск по весам (`admincommon.WeightTolerance`) для обзора периода и метрик.
- **`SettingsReader`, `ProgressSnapshotIntervalDaysKey` и `LoadProgressSnapshotIntervalDays`**
  относятся к настройке интервала снимков прогресса. Используются в `scheduler` и
  `admincommon`, а в пакете healthcheckin оказались случайно.
- **Раздел «Настройки» в админке** (`admin.js`, `section==='settings'`) состоит из карточек-блоков
  `GeneralSettingsPanel`, `AccessSettingsPanel`, `FeedbackSettingsPanel` и `ActivityLogPanel`.
  - У каждого блока свой обработчик `internal/http/handlers/api/v1/admin/settings/<block>` с
    `GET`/`POST`.
  - Каждое значение хранится отдельным продуктовым ключом `tenant_settings`: запись через
    `SetTenantProduct`, чтение через `admincommon.SettingInt` и аналоги, аудит
    `tenant_setting_saved`.

Клиентской части сводки (кнопка ⚡, панель, отметка просмотра) уже нет. Осталась страница
настроек в `admin.js` и блок `.hci-*` в `tracker.css`.

## Goals / Non-Goals

**Goals:**
- Удалить каталоги `internal/service/healthcheckin`, `internal/usecase/healthcheckin` и оба
  HTTP-пакета healthcheckin целиком, чтобы ни один импорт healthcheckin не остался.
- Сохранить поведение доски, дерева, обзора периода, метрик периодов, снимков прогресса и
  фонового прогрева кэша. Пороги сохраняют текущие значения пространства.
- Редактировать четыре нужных порога в разделе «Настройки» по тому же образцу, что и остальные
  блоки.

**Non-Goals:**
- Менять алгоритм, время жизни или интервал обновления кэша периода.
- Менять формулы здоровья цели и темпа. Меняется только источник порогов.
- Менять формат ответа `GET /api/v1/config` и клиентский код трекера, который его читает.
- Трогать исторические документы (`docs/old_specs`, `docs/superpowers`) и исторические
  миграции (033).
- Менять уведомления (колокольчик).

## Decisions

### 1. Кэш периода переезжает в `internal/usecase/period`

Это решение пользователя. Туда переносятся `Cache` (переименовывается в `PeriodCache`),
`PeriodData`, `Active` (переименовывается в `ActivePeriod`), загрузчик `NewPeriodLoader` с его
`Deps` и неэкспортируемые `buildTeamPath` и `abs`. Главные потребители кэша уже находятся в
`usecase/period`, а загрузчик — это оркестрация четырёх entity-сервисов, то есть логика слоя
usecase.

- `internal/scheduler` импортирует `usecase/period` ради `PeriodCache` и `ActivePeriod`. Слой
  scheduler внешний по отношению к usecase, поэтому зависимость направлена правильно. Интерфейс
  `SnapshotRunner` остаётся, в нём меняется только тип аргумента.
- Имя задачи фонового цикла меняется с `healthcheckin_cache_refresh` на
  `period_cache_refresh`. Тест `refreshloop_test.go` обновляется вместе с ним.
- Рассмотренная альтернатива: отдельный пакет `periodsnapshot`. Пользователь выбрал
  существующий пакет `period`.

### 2. Настройки прогресса живут в `internal/service/settings`

Туда переезжают:
- `ProgressSnapshotIntervalDaysKey` и `LoadProgressSnapshotIntervalDays`, а порт
  `SettingsReader` становится интерфейсом `settings.Reader`, которому удовлетворяет
  `*settings.Service`;
- новые константы ключей `progress_stale_days`, `progress_behind_margin`,
  `progress_green_threshold` и `progress_weight_tolerance`;
- тип `ProgressThresholds{StaleDays, BehindMargin, GreenThreshold, WeightTolerance}` со
  значениями по умолчанию 7/10/80/0 и функция `LoadProgressThresholds(ctx, scope, Reader)`.
  Отсутствующее или некорректное значение заменяется значением по умолчанию поштучно.

Потребители:
- `api/v1/config` читает три порога для `stale_days`, `behind_margin` и `green_threshold`.
  Формат ответа не меняется.
- `admincommon.WeightTolerance` сохраняется, но читает `LoadProgressThresholds().WeightTolerance`.
  Сигнатура `computePeriodOverview` и обработчики обзора и метрик не меняются.
- Новый обработчик блока (решение 3).

Это настройки пространства, и их место рядом с остальными настройками. Знание ключей
сосредоточено в одном месте, и HTTP-слой не разбирает хранение сам.

### 3. Отдельные ключи вместо JSON-объекта (решение пользователя)

Каждый порог хранится своим продуктовым ключом, как в блоке «Обратная связь». Так каждое значение
можно читать и записывать поштучно общими помощниками, а в аудит пишется по записи на ключ.

Рассмотренные альтернативы:
- один JSON-ключ `progress_thresholds`;
- оставить `health_checkin_config`, отбросив лишние поля. Отклонено: в хранилище осталось бы имя
  удалённой механики.

### 4. Блок «Пороги прогресса» в разделе «Настройки»

**Бэкенд.** Пакет `internal/http/handlers/api/v1/admin/settings/progressthresholds` устроен по
образцу `feedback`:
- `GET` возвращает `{stale_days, behind_margin, green_threshold, weight_tolerance}` через
  `LoadProgressThresholds`;
- `POST` сначала проверяет все значения (`stale_days > 0`, `behind_margin >= 0`,
  `1 <= green_threshold <= 100`, `weight_tolerance >= 0`). При ошибке отвечает 400 и не пишет
  ничего. Затем пишет четыре ключа через `SetTenantProduct` с аудитом `tenant_setting_saved` на
  каждый ключ;
- маршруты регистрируются в `registerAdminRoutes` вместо `adminhc`, в той же группе с доступом
  только для администратора пространства.

**Фронтенд.** Компонент `ProgressThresholdsSettingsPanel` в `admin.js` построен на тех же общих
элементах, что `FeedbackSettingsPanel`: заголовок карточки, числовые поля, подсказки, кнопка
сохранения и сообщение об ошибке. Он добавляется отдельной карточкой в разделе
`section==='settings'` после `FeedbackSettingsPanel`. Подписи и подсказки к четырём полям
переносятся из `HealthCheckInSettingsPanel`. Ссылки на категории сводки из них убираются.
Раздел `health-checkin`, `HealthCheckInSettingsPanel` и маршрут `/admin/health-checkin`
удаляются.

### 5. Кэш и распространение изменений

Пороги не участвуют в кэше периода: допуск по весам применяется при расчёте обзора поверх
закэшированных данных, остальные пороги применяются на клиенте. Поэтому инвалидация кэша
периода при сохранении порогов, которую делал старый `POST`, не нужна.

Новые значения доходят до других экземпляров приложения так же, как любые настройки
пространства: через чтение `tenant_settings` с учётом общего кэша настроек, если он включён.
Отдельный локальный кэш не вводится.

### 6. Миграция данных

Миграция `047_split_health_checkin_config` (up):
1. Для каждой строки с ключом `health_checkin_config` вставляет строки `progress_*` с
   соответствующими полями JSON. Переносятся только присутствующие поля;
   `ON CONFLICT (tenant_id, key) DO NOTHING`.
2. Удаляет строки `health_checkin_config`.

Down собирает `health_checkin_config` обратно из четырёх ключей (`jsonb_build_object` только по
присутствующим ключам) и удаляет ключи `progress_*`. Отброшенные поля сводки не
восстанавливаются, старый код подставит для них значения по умолчанию. Seed демо-данных
продуктовые настройки не пишет, в нём нужно только поправить комментарий со списком ключей.

### 7. Спецификация `health-checkin`

Delta удаляет все её requirements. При archive каталог `openspec/specs/health-checkin`
удаляется целиком. Требования, которые остаются в силе, перенесены в `settings` и
`team-okr-board`.

## Risks / Trade-offs

- **Потеря значений при переносе.** → Миграция переносит поля поштучно. Тест миграции на
  локальной БД сравнивает значения до и после. Перед применением на локальной БД сделать бэкап
  строк `health_checkin_config`.
- **Некорректные значения, сохранённые раньше** (например, отрицательное отставание). Старый
  обработчик их не проверял. → `LoadProgressThresholds` подставляет значение по умолчанию для
  недопустимого значения, а новый блок не даст сохранить такое снова.
- **Пропустить потребителя при переносе типов.** → Перед удалением каталогов нужен поиск
  `healthcheckin|hcsvc|hcuc|health_checkin|HealthCheck|\.hci-` по всему дереву, затем
  `go build ./...` и `go test ./...`.
- **Производительность.** `LoadProgressThresholds` читает четыре ключа вместо одного. Если
  у `settings.Service` есть пакетное чтение нескольких ключей, использовать его, иначе
  проверить, что чтение идёт через кэш настроек. Новых запросов в циклах нет, удаление
  эндпоинта сводки снимает нагрузку.

## Migration Plan

1. Развернуть сборку с миграцией 047. Миграция идемпотентна благодаря
   `ON CONFLICT DO NOTHING` и удалению по ключу.
2. Откат: применить down-миграцию 047, затем откатить сборку. Старый код получит
   `health_checkin_config` с перенесёнными порогами и значениями по умолчанию для остальных полей.
