# Domain Model

## Сущности

### Team

**Поля:**

- id
- name
- type: department | cluster | unit | group | team | squad | employee — уровни оргструктуры от широкого к узкому (`department` → `cluster` → `unit` → `group` → `team` → `squad` → `employee`); тип — только ярлык уровня, дерево вкладывается по `parent_id` и типом не ограничивается
- parent_id
- lead
- lead_udid (nullable, FK → users.udid, ON DELETE SET NULL) — UDID пользователя-руководителя; NULL в режиме без авторизации
- description
- deleted_at nullable timestamp

**Инварианты:**

- имя команды **не уникально** — одинаковые имена допустимы (в больших оргструктурах имена повторяются в разных ветках иерархии); команда всегда адресуется по `id`, имя — только отображаемый ярлык, не ключ и не ссылка;
- команда может иметь родителя;
- дерево не должно содержать циклов;
- команда считается активной, если `deleted_at IS NULL`;
- если у команды когда-либо были goals/OKR хотя бы в одном периоде, удаление команды выполняется как soft delete;
- soft-deleted команда остаётся в БД, чтобы не терять исторические goals, goal shares и period status;
- hard delete команды допустим только если у команды нет goals ни в одном периоде;
- при удалении команды её дочерние команды не удаляются, а перепривязываются к родителю удаляемой команды; если родителя не было, дети становятся корневыми;
- восстановление soft-deleted команды возвращает её в список активных команд, но не откатывает автоматическое перепривязывание дочерних команд.

### Period

**Поля:**

- id
- name
- start_date
- end_date
- archived_at (nullable timestamptz) — момент ручной архивации; NULL, если период не архивирован

**Вычисляемые поля** (не хранятся в БД, вычисляются на чтении — правила см. в «Производные вычисления»):

- parent_id (nullable) — id ближайшего периода, интервал которого строго вмещает интервал этого периода
- depth (int) — глубина в дереве вложенности (0 — корень)
- status (`future` | `active` | `closed` | `archived`)

**Инварианты:**

- период имеет уникальное имя в рамках тенанта;
- start_date <= end_date;
- ручного порядка (`sort_order`) не существует (drop в миграции `036_period_nesting`) — порядок отображения полностью выводится из вложенности и статуса, см. «Производные вычисления»;
- вложенность (parent/depth) не хранится и не задаётся вручную — выводится из дат start_date/end_date других периодов того же тенанта при каждом чтении;
- archived_at может быть проставлен только через admin-действие «архивировать» и только когда текущий (date-based) статус периода — `closed`; снимается действием «разархивировать» без дополнительных условий.

### Goal

**Поля:**

- id
- team_id
- period_id
- title
- description
- priority
- weight
- work_type
- focus_type
- owner_text
- owner_udids (массив UDID'ов владельцев)

**Инварианты:**

- goal всегда принадлежит owner team и одному периоду;
- weight в диапазоне 0..100;
- порядок goals внутри (team_id, period_id) управляется sort_order; для команды-владельца используется `goals.sort_order`, для команды, куда goal расшарена, — `GoalShare.sort_order` (см. GoalShare);
- shared goal не меняет identity goal, а лишь добавляет видимость/вес/порядок для других команд.

### KeyResult

**Поля:**

- id
- goal_id
- title
- description
- zeroing_criteria — опциональный текстовый критерий обнуления (человекочитаемый, в расчётах не применяется; доступен для любого kind)
- weight
- kind
- health_status — ручной health-статус KR из закрытого справочника (`not_started` | `on_track` | `at_risk` | `done`), по умолчанию `not_started`
- sort_order
- note (nullable, 1:1 — см. KeyResultNote)

**Типы (kind):**

- BOOLEAN — бинарный (выполнен / не выполнен)
- PROJECT — проектный (этапы с весами)
- NUMERICAL — числовой (измеряется числом)

Типы `PERCENT` и `LINEAR` упразднены: миграция `023_kr_numerical` переводит их в `NUMERICAL` с `unit = '%'`.

**Поля NUMERICAL (хранятся прямо в таблице `key_results`):**

- start_value — стартовое значение
- target_value — целевое значение
- current_value — текущее значение
- unit — единица измерения из закрытого справочника (`%`, `RPS`, `мс`, `сек`, `мин`, `час`, `дней`, `шт`, `₽`, `запросов`, `ошибок`, `пользователей`, `заказов`, `рублей`)
- checkpoints — опциональные промежуточные значения, JSONB-массив `[{ value, progress_percent }]` в той же строке KR (отдельной таблицы нет)

Поля `custom_unit` нет; `unit` — не свободный текст.

**Инварианты:**

- KR принадлежит ровно одной goal;
- weight в диапазоне 0..100;
- порядок KR управляется отдельно внутри goal;
- для NUMERICAL `unit` обязателен и валидируется по справочнику;
- checkpoints валидируются при сохранении: проценты в диапазоне 0..100, значения не дублируются; checkpoints загружаются вместе с KR без дополнительных запросов;
- `health_status` — обязательное поле из закрытого справочника (`not_started` | `on_track` | `at_risk` | `done`), значение по умолчанию `not_started`; задаётся вручную и валидируется по справочнику;
- health-статус **информационный** и **не входит** в расчёт прогресса KR/goal/period и rollup;
- при переходе прогресса KR `<100% → =100%` сервер **однократно** выставляет `health_status = done` (если он ещё не `done`); повторное сохранение на 100% и падение прогресса ниже 100% статус не меняют, а явно переданный ручной статус всегда перекрывает авто-`done`.

### GoalComment

**Поля:**

- id
- goal_id
- parent_id (nullable, FK → goal_comments.id, ON DELETE CASCADE) — NULL для таски (замечания первого уровня); ссылка на таску для ответа
- text
- author_user_id (ссылка на users.id, NOT NULL)
- created_at
- resolved_at (nullable timestamptz) — момент, когда замечание отмечено решённым; NULL, если не решено
- resolved_by_user_id (nullable, FK → users.id) — кто отметил решённым; NULL, если не решено

**Инварианты:**

- комментарий всегда имеет автора;
- в режиме `auth.mode=disabled` автором выступает системный пользователь `anonymous-local` (id=1);
- исторические комментарии, созданные до миграции, бэкфиллены системным пользователем `migration` (id=2);
- комментарий первого уровня (`parent_id IS NULL`) — **таска/замечание** с состоянием: решён ⇔ `resolved_at IS NOT NULL`; `resolved_at` и `resolved_by_user_id` всегда либо оба заполнены, либо оба NULL;
- **ответ** (`parent_id` указывает на таску) таской не является: у ответа `resolved_at`/`resolved_by_user_id` всегда NULL, резолву он не подлежит;
- глубина вложенности ровно один уровень — ответ можно оставить только на таску (`parent_id` всегда указывает на строку с `parent_id IS NULL`); ответа на ответ нет;
- таски сортируются `created_at` по возрастанию (старые → новые); ответы внутри таски — тоже по возрастанию `created_at`;
- удаление таски каскадно удаляет её ответы (`ON DELETE CASCADE`); удалять таску/ответ может **автор** или **tenant-admin**;
- счётчик комментариев цели — число тасок (решённых и нерешённых); ответы в счётчик не входят;
- отметка решённым обратима (reopen обнуляет оба поля); переход **идемпотентен** — обновление и запись в журнал происходят только при реальной смене состояния (guard `parent_id IS NULL AND resolved_at IS [NOT] NULL`), поэтому повторный resolve/reopen не перезаписывает `resolved_at`/`resolved_by_user_id` и не создаёт ложную запись в логе активности.

### KeyResultNote

**Поля:**

- key_result_id (PK, FK → key_results.id)
- text
- author_user_id (FK → users.id)
- updated_at

**Инварианты:**

- одна заметка на один KR (key_result_id — PRIMARY KEY);
- заметку нельзя удалить через API — только перезаписать;
- при удалении KR заметка удаляется каскадно;
- автор заметки — текущий пользователь на момент записи; в режиме `auth.mode=disabled` — `anonymous-local`.

### GoalShare

**Поля:**

- goal_id
- team_id
- weight
- sort_order

**Инварианты:**

- одна и та же goal может быть расшарена на много команд;
- для каждой shared team хранится собственный weight и собственный sort_order — порядок общей цели можно менять в каждой команде независимо; при шаринге sort_order инициализируется значением `goals.sort_order` владельца;
- pair (goal_id, team_id) уникален.

### GoalLink

Связь между двумя целями отношением «дочерняя → родительская» (таблица `goal_links`,
миграция `043_goal_links`). Позволяет декомпозировать цель руководителя/годовую цель на
цели команды/квартала. Связь **не** делает цель общей (это отдельная механика `GoalShare`).

**Поля:**

- tenant_id (FK → tenants.id, ON DELETE CASCADE) — тенант; обе цели ребра всегда в нём;
- child_goal_id (FK → goals.id, ON DELETE CASCADE) — дочерняя цель;
- parent_goal_id (FK → goals.id, ON DELETE CASCADE) — родительская цель;
- created_at.

**Инварианты:**

- пара `(tenant_id, child_goal_id, parent_goal_id)` уникальна (PK);
- `child_goal_id <> parent_goal_id` (запрет самоссылки, DB CHECK `goal_links_no_self`);
- обе цели принадлежат одному тенанту (проверяется в service-слое tenant-scoped запросами;
  кросс-тенантные связи невозможны);
- граф связей **ацикличен** (DAG): добавление ребра, замыкающего цикл (прямой `A→B→A` или
  транзитивный), отклоняется на сервере — проверка одним рекурсивным CTE по предкам набора
  родителей (см. `040-api-contract.md`);
- цель может иметь **несколько** родителей и несколько детей (M:N);
- связи каскадно удаляются при удалении любой из двух целей (`ON DELETE CASCADE`), в т.ч. при
  переносе цели `mode=move` (жёсткое удаление исходной цели удаляет её связи);
- связи **не копируются** при `POST /goals/{id}/transfer` (как и шеры) — новая цель без связей;
- связь **навигационная**: не влияет на расчёт прогресса (rollup по связям нет), не меняет
  видимость цели командами и не зависит от `team period status`.

**Вычисляемые сводки (не хранятся):**

- `GoalRef` — компактная сводка связанной цели для лейблов/popover: `id`, `title`,
  `period_id`, `period_name`, `team_id`, `team_name`, `team_type`, `progress` (прогресс
  вычисляется на чтении по KR связанной цели).
- `Goal.Parents` / `Goal.Children` — массивы `GoalRef`, заполняются на чтении и
  **фильтруются по scope читателя**: связанная цель попадает в выборку только если её
  команда-владелец доступна читателю (grant/админ). Связь на недоступную команду скрыта.

### ActivityEvent

Append-only журнал событий OKR (таблица `activity_events`, миграция `039_activity_events`).

**Поля:**

- id
- tenant_id (FK → tenants.id, ON DELETE CASCADE)
- actor_user_id (FK → users.id) — кто совершил действие; в `auth.mode=disabled` — `anonymous-local` (id=1)
- category — `progress` | `composition` | `status` | `discussion` (совпадает с табами UI)
- action — конкретное действие: `kr_progress`, `goal_created`, `goal_deleted`, `goal_copied`, `goal_moved`, `kr_created`, `kr_deleted`, `goal_shared`, `goal_unshared`, `goal_linked`, `goal_unlinked`, `goal_owner_changed`, `goal_fields_changed`, `kr_fields_changed`, `kr_note_updated`, `status_changed`, `comment_added`, `comment_resolved`, `comment_reopened`, `reply_added`, `comment_deleted`, `reply_deleted` — категории `discussion` относятся `comment_added`/`reply_added`/`comment_resolved`/`comment_reopened`/`comment_deleted`/`reply_deleted` (таски и ответы живут под одним фильтром, но с разными описаниями)
- team_id (FK → teams.id, ON DELETE SET NULL) — команда-контекст на момент события (owner team / команда статуса)
- period_id (FK → periods.id, ON DELETE SET NULL) — период-контекст
- goal_id, kr_id, comment_id — ссылки на сущности (**не** FK: журнал переживает удаление сущности)
- entity_title — снапшот заголовка цели/KR/команды на момент события
- payload_json (JSONB) — `{ before, after, ... }` + доп. поля (проценты, текст, `changed`, `shared_with_team_ids`)
- created_at

**Инварианты:**

- журнал **append-only**; событие пишется **best-effort после** успешной мутации в service-слое — ошибка записи логируется, но не роняет действие пользователя (источник правды — сама мутация);
- actor резолвится tenant-scoped на чтении: активный участник тенанта → имя + аватар; не участник (кроме системных, provider=`system`) → нейтральный плейсхолдер `removed=true` **без** email/аватара/UDID (PII бывшего участника не утекает);
- хранимые `team_id`/`period_id` — исторический контекст (scope, счётчики, бейдж периода); навигационный target собирается на чтении;
- очистка журнала — только через admin-действие (tenant-admin своего пространства / system-admin по любому тенанту), см. `040-api-contract.md` и `050-permissions-and-lifecycle.md`.

### Notification

Персональное уведомление получателя (таблица `notifications`, миграция `044_notifications`).
Одна строка — одновременно запись ленты колокольчика и якорь для будущих внешних
доставок (Telegram/Mattermost, фаза 2).

**Поля:**

- id
- tenant_id (FK → tenants.id, ON DELETE CASCADE)
- user_id (FK → users.id, ON DELETE CASCADE) — получатель
- type — `goal_comment` | `my_comment_resolved` | `goal_changed` | `kr_progress`; из какого события какой тип
  получается, см. `notifyType` в `internal/usecase/notification/mapping.go` — часть событий шины (9 из 22)
  не порождает уведомления вовсе (напр. `status_changed`, `goal_shared`, `kr_note_updated`)
- kind — исходное действие события (те же значения, что `action` у ActivityEvent)
- actor_user_id (FK → users.id) — кто совершил действие
- team_id, period_id, goal_id, kr_id, comment_id — ссылки на сущности (**не** FK — как и у
  ActivityEvent, уведомление переживает удаление сущности, на которую указывает)
- entity_title — снапшот заголовка цели/KR на момент события
- payload_json (JSONB) — минимум, нужный рендеру текста (`internal/render/notify`), не весь payload журнала
- coalesce_key — `type:entity:actor:bucket`, где entity — `kr:<id>` только для `kr_progress`,
  иначе `goal:<id>`; bucket — фиксированное десятиминутное окно (`CoalesceWindow`), а не
  скользящее: скользящее окно потребовало бы read-then-write и гонок между репликами,
  фиксированное разменивает это на артефакт границы бакета — сознательная плата
- coalesce_count — сколько повторов схлопнулось в эту строку
- created_at, updated_at
- read_at (NULL пока не прочитано)

**Инварианты:**

- вставка — `INSERT ... ON CONFLICT (tenant_id, user_id, coalesce_key) DO UPDATE`: повтор в
  пределах окна не создаёт новую строку, а увеличивает `coalesce_count`, обновляет
  `payload_json` и **сбрасывает `read_at` в NULL** — повтор внутри окна снова помечает
  уведомление непрочитанным, потому что это новая информация, а не дубликат старой;
  один атомарный statement, конфликт арбитрирует уникальный индекс, а не read-then-write гонка
  между репликами;
- цель схлопывания — редактирование цели вместе с двумя её KR даёт одно уведомление «×3»,
  а не три отдельных: сущность в ключе схлопывания — KR только для `kr_progress`, для всех
  остальных типов — сама цель;
- адресные типы (`my_comment_resolved`) уведомляют конкретного получателя, зашитого в
  событие (автора комментария); никто не уведомляется о собственном действии — актёр события
  никогда не получатель;
- типы со скоупом (`goal_comment`, `goal_changed`, `kr_progress`) резолвятся обходом дерева
  команд (`teams.parent_id`) от team_id события вверх до корня, одним батчевым рекурсивным
  запросом на группу событий (не в цикле — N+1), по настройкам получателя, см.
  NotificationPreference ниже;
- ретенция (`PurgeOlderThan`) считает возраст по **`updated_at`, а не `created_at`**: схлопывание
  обновляет `updated_at` и сбрасывает `read_at`, поэтому уведомление, которое продолжает
  собирать повторы, не должно быть удалено из-под непрочитанного бейджа только потому, что
  впервые возникло давно — «нетронуто N дней» здесь и означает `updated_at`;
- actor резолвится на чтении так же, как у ActivityEvent (tenant-scoped участник → имя и
  аватар; бывший участник, кроме системных provider=`system` — нейтральный плейсхолдер, без PII).

### NotificationPreference

Настройки уведомлений одного пользователя в одном пространстве (таблица
`notification_preferences`, миграция `044_notifications`).

**Поля:**

- tenant_id, user_id (FK → tenants.id / users.id, ON DELETE CASCADE)
- type — тот же перечень, что у Notification.type
- enabled (по умолчанию TRUE)
- scope — `own` | `own_and_children` | `subtree`; NULL для адресных типов (у
  `my_comment_resolved` скоупа нет — получатель уже известен из события)
- channels — TEXT[], по умолчанию `{in_app}`; в фазе 1b единственный канал
- PRIMARY KEY (tenant_id, user_id, type)

**Инварианты:**

- **отсутствие строки — это норма, а не пробел**: означает дефолт (enabled=TRUE,
  scope=`own`, channels=`{in_app}`) — бэкфилл на всех пользователей при создании
  пользователя или типа не делается, дефолт подставляется на чтении и на резолве
  получателей;
- scope управляет обходом дерева команд при резолве: `own` — только лид team_id события
  (distance 0), `own_and_children` — лид этой команды и её непосредственного родителя
  (distance ≤ 1), `subtree` — лид любого предка вплоть до корня;
- изменить свои настройки может только сам пользователь (`user_id` берётся из сессии, не
  из тела запроса), см. `050-permissions-and-lifecycle.md`.

### TeamPeriodStatus

**Значения:**

- no_goals
- forming
- ready
- in_progress
- validated
- closed

Значение `ready` добавлено миграцией `018_status_ready` и соответствует шагу «К валидации» в UI.
Значение `validated` сохранено в схеме для обратной совместимости, но в UI-стиппере не отображается;
фактически используется статус `ready` перед `in_progress`.

### TeamPeriodProgressSnapshot

Суточный снапшот прогресса команды в периоде — источник данных для графика «Прогресс целей за период» (таблица `team_period_progress_snapshots`, миграция `041`).

**Поля:**

- id
- tenant_id (FK → tenants.id, ON DELETE CASCADE)
- period_id (FK → periods.id, ON DELETE CASCADE)
- team_id (FK → teams.id, ON DELETE CASCADE)
- snapshot_date (date) — дата снапшота (UTC)
- progress (int) — взвешенный прогресс команды по целям на момент снапшота (та же формула, что в обзоре)
- created_at

**Инварианты:**

- уникальность `(tenant_id, period_id, team_id, snapshot_date)` — один снапшот на команду в день; повторная запись за тот же день идемпотентна (upsert обновляет `progress`);
- материализуется фоновой задачей **только для активных (date-based) периодов**; закрытые и архивные периоды не снапшотятся. Частота фиксации точек настраивается per-tenant ключом `progress_snapshot_interval_days` (раздел «Настройки» → «Общие настройки»; ≥1, по умолчанию 1 = ежедневно): новая точка пишется, когда с последнего снимка периода прошло ≥ интервала дней (задача опрашивает состояние на фиксированной служебной частоте, но записывает по интервалу);
- команды без целей и soft-deleted команды пропускаются;
- в мультиинстанс-среде проход защищён Postgres advisory-lock — снапшот делает один инстанс за цикл (upsert идемпотентен и без блокировки);
- график читает снапшоты по охвату (my_teams / org) и агрегирует средним по командам-с-целями на каждую дату; к историческим точкам добавляется «живая» точка на сегодня из текущего состояния.

### User

**Поля:**

- id
- provider_subject_key (уникальный ключ: `provider:subject`)
- provider (google | github | keycloak | system)
- subject (ID пользователя у провайдера)
- display_name
- avatar_url
- email (nullable)
- attributes_json (JSONB, расширяемое хранилище без миграций)
- is_system_admin (суперадмин инстанса; tenant-admin — это `memberships.role = admin`, не поле пользователя)
- created_at
- updated_at
- last_login_at

**Системные пользователи (создаются миграцией):**

| id | provider_subject_key    | Назначение                                      |
|----|-------------------------|-------------------------------------------------|
| 1  | system:anonymous-local  | Автор операций в режиме без авторизации         |
| 2  | system:migration        | Синтетический автор при бэкфилле старых данных  |

**Инварианты:**

- при первом входе пользователь создаётся через `INSERT ... ON CONFLICT DO UPDATE`;
- при повторном входе обновляются `display_name`, `avatar_url`, `last_login_at`;
- `attributes_json` позволяет добавлять произвольные поля профиля без новых миграций.

### AuthSession

**Поля:**

- id (случайный hex-токен 32 байта)
- user_id
- provider
- created_at
- expires_at
- last_seen_at
- user_agent (nullable)
- ip (nullable)

**Инварианты:**

- сессия хранится на сервере (server-side); клиент получает только `id` в cookie;
- cookie: `HttpOnly`, `SameSite=Lax`, `Secure` при TLS;
- истекшие сессии не возвращаются при резолве; фоновая очистка через `DeleteExpiredSessions`.

### HierarchyGrant

**Поля:**

- id
- user_id
- team_id
- created_at
- created_by_user_id

**Инварианты:**

- пара (user_id, team_id) уникальна;
- доступ к узлу автоматически распространяется на все дочерние команды (рекурсивный запрос);
- итоговая область видимости = объединение всех явных грантов + optional default_node + все их потомки.

### Настройки — три уровня

Настройки разнесены по трём store'ам, по трём плоскостям администрирования
(см. `050-permissions-and-lifecycle.md`). На горячем пути читаются снапшотом (все ключи
одним запросом) через кэш, не per-key SQL.

#### SystemSettings (`system_settings`, global)

Плоскость **system-admin**. Глобальные key/value-ключи инстанса (без `tenant_id`):

- key (PK), value_json (JSONB)

| key                            | Тип          | Смысл                                                        |
|--------------------------------|--------------|--------------------------------------------------------------|
| default_registration_tenant_id | int64 / null | В какой тенант попадает новый пользователь без membership; `null` → страница-заглушка |

#### TenantSettings (`tenant_settings`, per-tenant)

Плоскость **tenant-admin**. PK `(tenant_id, key)`, value_json (JSONB). Два класса ключей с
разной write-authority (проверяется в service-слое):

- продуктовые ключи (пишет tenant-admin): `new_user_policy` (`"empty"`/`"default_node"`),
  `default_hierarchy_node_id` (int64/null), `health_checkin_config` (object:
  `stale_days`, `behind_margin`, `weight_tolerance`, `cache_ttl_minutes`,
  `green_threshold` (1..100, по умолчанию 80 — порог прогресса, при котором цель/команда
  считается «в плане» и подсвечивается зелёным),
  `comment_depth` (int ≥ 0, по умолчанию 1 — на сколько уровней вниз от команд пользователя
  спускаться при поиске нерешённых комментариев для категории «Комментарии»),
  `resolved_comments_limit` (int ≥ 1, по умолчанию 5 — сколько последних решённых (не самим
  пользователем) его комментариев показывать), `in_counter` map[string]bool (ключи категорий
  колокольчика, включая `comments` — по умолчанию false)),
  `progress_snapshot_interval_days` (int ≥ 1, по умолчанию 1 — как часто фоновая задача
  фиксирует точку прогресса команд для графика за период; 1 — ежедневно; редактируется в
  разделе «Настройки» → «Общие настройки»),
  `documentation_url` (абсолютный http(s) URL или пусто),
  `feedback_url`, `feedback_popup_enabled`, `feedback_menu_link_enabled`, `feedback_frequency_days`;
- ключи `entitlement.*` (пишет только system-admin/provisioning): `entitlement.sso`,
  `entitlement.subdomains`, `entitlement.file_uploads`, `entitlement.max_users`, …
  Читаются интерфейсом `Entitlements`; OSS-реализация (`UnlimitedEntitlements`) возвращает
  `true`/`∞` и эти ключи игнорирует.

Продуктовые ключи переехали сюда из `system_settings` миграцией 033.

#### UserSettings (`user_settings`, per-user)

Плоскость **user**. PK `(user_id, key)`, value_json (JSONB). Личные преференсы (напр.
`default_landing_tenant_id`). Не на горячем пути (грузится на `/settings`).

---

## Производные вычисления

- Period.parent_id — id ближайшего периода A, чей интервал **строго** вмещает интервал периода C: `A.start_date <= C.start_date AND A.end_date >= C.end_date`, и хотя бы одно из неравенств строгое. Периоды с полностью совпадающими интервалами — сиблинги, а не родитель/потомок. Если подходящих кандидатов несколько, выбирается «самый узкий» контейнер по правилам тай-брейка в порядке приоритета: 1) меньшая длительность (end_date − start_date); 2) более поздняя start_date; 3) более ранняя end_date; 4) меньший id. Периоды без подходящего контейнера — корни дерева (parent_id = null). Глубина вложенности не ограничена.
- Period.depth — глубина периода в дереве вложенности: 0 для корневых периодов, иначе depth родителя + 1.
- Period.status — если `archived_at IS NOT NULL`, статус всегда `archived` (ручной приоритетный флаг, перекрывающий дату). Иначе вычисляется относительно текущего момента: `now < start_date` → `future`; `now > end_date` → `closed`; иначе (включая `now == start_date` и `now == end_date`) → `active`.
- Порядок периодов в списках (нет хранимого `sort_order`) — DFS-обход леса периодов: среди корневых периодов сначала идут периоды со статусом `future`/`active`, затем `closed`, затем `archived`; внутри каждой из этих зон — более новые периоды (по start_date) раньше более старых, при равенстве — по возрастанию id. Дочерние периоды одного родителя сортируются по-разному в зависимости от статуса родителя: если родитель `future`/`active` — по возрастанию start_date, если `closed`/`archived` — по убыванию start_date (тай-брейк — по возрастанию id). Каждый узел выводится сразу перед своими (рекурсивно упорядоченными) потомками, поэтому итоговый список — плоский, но по нему можно восстановить дерево через `depth`.
- Archived-периоды исключаются из ответа `GET /api/v1/periods` (но не из `GET /api/v1/admin/periods`); при построении дерева видимых периодов родитель видимого периода никогда не указывает на скрытый архивный период — вычисление вложенности для публичного списка выполняется уже после фильтрации архивных.
- Goal.progress = взвешенное среднее прогресса KR.
- Period.progress = взвешенное среднее прогресса goals.
- PROJECT KR.progress = сумма весов завершённых этапов, clamp 0..100.
- BOOLEAN KR.progress = 100 или 0.
- NUMERICAL KR.progress:
  - без checkpoints — линейно от `start_value` к `target_value` (оба направления), clamp 0..100; при `start_value == target_value` → 100, если `current_value` достиг цели, иначе 0;
  - с checkpoints — линейная интерполяция между точками: `start_value` (0%), каждое промежуточное значение и `target_value` (100%), отсортированными по значению; вне диапазона — процент ближайшей крайней точки; clamp 0..100.
- Процент выполнения KR остаётся основной величиной для расчёта прогресса goal/period и rollup.

## Обязательные тест-кейсы на домен

- goal без KR даёт 0%.
- goal с KR суммарного веса 0 даёт 0%.
- project KR с completed stages >100 clamp'ится до 100.
- numerical KR без checkpoints считает линейно (рост и снижение).
- numerical KR ниже старта → 0%, выше цели → 100%.
- numerical KR при `start_value == target_value` → 100% если цель достигнута, иначе 0%.
- numerical KR с checkpoints линейно интерполирует между точками (start=0%, промежуточные значения, target=100%); ниже start → 0%, при достижении target → 100%.
- миграция LINEAR → NUMERICAL и PERCENT → NUMERICAL с `unit = '%'`, сохраняя start/target/current, веса и историю; checkpoints переносятся в JSONB-поле `key_results.checkpoints`, отдельная таблица удаляется.
- shared goal показывает разный weight для разных команд без дублирования goal identity.
