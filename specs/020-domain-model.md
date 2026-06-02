# Domain Model

## Сущности

### Team

**Поля:**

- id
- name
- type: cluster | unit | team
- parent_id
- lead
- description
- deleted_at nullable timestamp

**Инварианты:**

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
- sort_order

**Инварианты:**

- период имеет уникальное имя;
- start_date <= end_date;
- сортировка периодов управляется явно через sort_order.

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

**Инварианты:**

- goal всегда принадлежит owner team и одному периоду;
- weight в диапазоне 0..100;
- порядок goals внутри (team_id, period_id) управляется sort_order;
- shared goal не меняет identity goal, а лишь добавляет видимость/вес для других команд.

### KeyResult

**Поля:**

- id
- goal_id
- title
- description
- weight
- kind
- sort_order
- note (nullable, 1:1 — см. KeyResultNote)

**Типы:**

- PROJECT
- PERCENT
- LINEAR
- BOOLEAN

**Инварианты:**

- KR принадлежит ровно одной goal;
- weight в диапазоне 0..100;
- порядок KR управляется отдельно внутри goal.

### GoalComment

**Поля:**

- id
- goal_id
- text
- author_user_id (ссылка на users.id, NOT NULL)
- created_at

**Инварианты:**

- комментарий всегда имеет автора;
- в режиме `auth.mode=disabled` автором выступает системный пользователь `anonymous-local` (id=1);
- исторические комментарии, созданные до миграции, бэкфиллены системным пользователем `migration` (id=2).

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
- для каждой shared team хранится собственный weight;
- pair (goal_id, team_id) уникален.

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
- is_admin
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

### SystemSettings

**Поля:**

- key (PK)
- value_json (JSONB)

**Текущие ключи:**

| key                        | Тип      | Смысл                                                            |
|----------------------------|----------|------------------------------------------------------------------|
| new_user_policy            | string   | `"empty"` или `"default_node"` — политика для новых пользователей |
| default_hierarchy_node_id  | int64    | ID узла иерархии для политики `default_node`; null если не задан |
| health_checkin_config      | object   | Настройки Health Check-in: `stale_days`, `behind_margin`, `weight_tolerance`, `cache_ttl_minutes`, `in_counter` (map[string]bool) |

---

## Производные вычисления

- Goal.progress = взвешенное среднее прогресса KR.
- Period.progress = взвешенное среднее прогресса goals.
- PROJECT KR.progress = сумма весов завершённых этапов, clamp 0..100.
- BOOLEAN KR.progress = 100 или 0.
- PERCENT KR.progress = линейно, либо по checkpoint interpolation.
- LINEAR KR.progress = линейный clamp 0..100.

## Обязательные тест-кейсы на домен

- goal без KR даёт 0%.
- goal с KR суммарного веса 0 даёт 0%.
- project KR с completed stages >100 clamp'ится до 100.
- percent KR без checkpoints считает линейно.
- percent KR с checkpoints интерполирует между соседними точками.
- shared goal показывает разный weight для разных команд без дублирования goal identity.
