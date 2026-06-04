# API Contract

## Общий принцип

`/api/v1` — канонический интерфейс для данных и мутаций.
Legacy `/api/*` endpoints не поддерживаются и не должны использоваться фронтендом.
Legacy SSR/form mutation endpoints для team OKR также не считаются частью контракта.

SSR-страницы должны опираться на те же правила, что и API.

## Auth endpoints

Публичные маршруты (не требуют авторизации):

- `GET /login` — страница входа (список провайдеров или немедленный редирект при одном провайдере); при `AUTH_MODE=disabled` редиректит на `/teamOkrs`
- `GET /auth/{provider}/start` — инициирует OAuth2 flow, устанавливает state cookie и редиректит на провайдера
- `GET /auth/{provider}/callback` — обрабатывает OAuth2 callback, создаёт или обновляет пользователя, устанавливает session cookie, редиректит на исходную страницу
- `POST /logout` — удаляет серверную сессию, очищает cookie, редиректит на `/login`

При `AUTH_MODE=enabled` неавторизованные запросы к SSR-страницам получают `302 → /login?next=<original_url>`. Запросы к API-endpoints получают `401`.

## Users endpoint

- `GET /api/v1/users` — возвращает список пользователей по UDID или поисковому запросу (не-системные)

Требуется хотя бы один параметр; без параметров возвращается `400 VALIDATION_ERROR`.

**Режим загрузки по UDID:**

`GET /api/v1/users?ids[]=<udid1>&ids[]=<udid2>` — возвращает пользователей с указанными UDID. Максимум 100 UDID за запрос; лишние усекаются. Scope-фильтрация не применяется — используется для загрузки уже известных ссылок (авторы комментариев, существующие владельцы целей).

**Режим поиска:**

`GET /api/v1/users?q=<строка>` — возвращает до 20 пользователей, чьё `display_name` или `email` содержит `<строка>` (case-insensitive). Если `q=` (пустая строка), возвращает 20 пользователей с самым свежим `last_login_at`.

**Scope-фильтрация в режиме поиска:**

Пользователь видит в результатах поиска только тех, кто имеет доступ к хотя бы одному узлу иерархии в его scope:

- пользователь имеет явный grant на команду, входящую в scope запрашивающего, или на родительскую команду такого узла (grant на родителя покрывает дочерние команды);
- пользователь является lead-ом команды, входящей в scope.

Администраторы видят всех пользователей без ограничений. При пустом scope (пользователь без единого гранта) возвращается пустой массив.

Response (array):

```json
[
  {
    "udid": "550e8400-e29b-41d4-a716-446655440000",
    "display_name": "Ivan Ivanov",
    "avatar_url": "https://...",
    "provider": "google",
    "email": "ivan@example.com",
    "led_team": "Platform"
  }
]
```

- Идентификатор пользователя в публичном API — `udid` (UUID), целочисленный `id` не раскрывается.
- `led_team` — имя команды, которой пользователь является лидом (по строковому совпадению `teams.lead = display_name`); поле отсутствует если команды нет.
- Endpoint доступен без admin-прав (scope: авторизованный пользователь).
- Используется UserSelector в GoalModal (tracker.js) через режим `?q=` для выбора владельца цели; через режим `?ids[]=` — для загрузки пользователей, уже упоминающихся на странице (авторы комментариев).

## Me endpoint

- `GET /api/v1/me` — возвращает текущего пользователя

Response:

```json
{
  "id": 42,
  "display_name": "Ivan Ivanov",
  "email": "ivan@example.com",
  "avatar_url": "https://...",
  "provider": "google",
  "is_admin": false
}
```

При `AUTH_MODE=disabled` возвращает системного пользователя `anonymous-local`.

## Admin API endpoints

Доступны только администраторам при `AUTH_MODE=enabled`. При `AUTH_MODE=disabled` доступны всем.

### Пользователи

- `GET /api/v1/admin/users` — список всех пользователей (id, display_name, avatar_url, provider, last_login_at, is_admin)
- `GET /api/v1/admin/users/{userID}` — карточка пользователя с grants
- `POST /api/v1/admin/users/{userID}/admin` — выдать права администратора
- `DELETE /api/v1/admin/users/{userID}/admin` — снять права администратора

### Grants

- `GET /api/v1/admin/users/{userID}/grants` — список выданных hierarchy grants
- `POST /api/v1/admin/users/{userID}/grants` — выдать грант на узел иерархии; body: `{"team_id": 42}`
- `DELETE /api/v1/admin/users/{userID}/grants/{teamID}` — отозвать грант

### Настройки доступа

- `GET /api/v1/admin/settings/access` — текущая политика для новых пользователей

Response:

```json
{
  "new_user_policy": "empty",
  "default_hierarchy_node_id": null
}
```

- `POST /api/v1/admin/settings/access` — обновить политику; body: `{"new_user_policy": "default_node", "default_hierarchy_node_id": 42}`

Допустимые значения `new_user_policy`:

- `empty` — новый пользователь не получает доступа (пустая иерархия)
- `default_node` — новый пользователь получает грант на `default_hierarchy_node_id` при первом логине, если у него ещё нет ни одного гранта

Политика хранится в `system_settings` и применяется без перезапуска. Изменение политики не влияет на уже выданные гранты.

Все admin API endpoints требуют CSRF token при вызове из браузера.

Ошибки возвращаются в нормализованном виде:

- `VALIDATION_ERROR`
- `NOT_FOUND`
- `CONFLICT`
- `INTERNAL`

## Требования к доступу для всех endpoints

Все endpoints, работающие с данными иерархии, применяют scope пользователя:

- `GET /api/v1/hierarchy` — возвращает только узлы, доступные пользователю по grants; если доступных узлов нет, возвращает пустой список (`items: []`).
- Любой endpoint, принимающий `teamID` в пути (`/teams/{teamID}/*`), возвращает `404 NOT_FOUND`, если `teamID` не входит в доступный scope пользователя.
- Любой endpoint, принимающий `goalID` в пути (`/goals/{goalID}/*`), возвращает `404 NOT_FOUND`, если команда-владелец goal не входит в scope пользователя.
- Любой endpoint, принимающий `krID` в пути (`/krs/{krID}/*`), возвращает `404 NOT_FOUND`, если команда-владелец родительской goal не входит в scope пользователя.
- Администраторы имеют доступ ко всем узлам без ограничений.
- В режиме `AUTH_MODE=disabled` системный пользователь `anonymous-local` является администратором и имеет полный доступ.

## Read endpoints

Обязательные read endpoints:

- `GET /api/v1/hierarchy`
- `GET /api/v1/periods`
- `GET /api/v1/teams/{teamID}`
- `GET /api/v1/teams/{teamID}/okrs`
- `GET /api/v1/teams/{teamID}/overview`
- `GET /api/v1/goals/{goalID}`

### `GET /api/v1/health-checkin?period_id=<int64>`

Назначение: вычислить сводку Health Check-in для текущего пользователя за выбранный период.

Доступен: любому авторизованному пользователю.

Scope: сервер определяет по UDID пользователя из сессии.

- Администраторы (включая режим `AUTH_MODE=disabled`) видят все команды без scope-фильтрации.
- Для обычных пользователей: lead-scope: команды, где `teams.lead_udid = user.udid` + все потомки; owner-scope: команды с целями, где `user.udid = ANY(goal.owner_udids)`.

Request params:
- `period_id` (int64, обязательный)

Success response (`200`):

```json
{
  "has_scope": true,
  "period_id": 1,
  "total_problems": 5,
  "categories": {
    "stale":               { "in_counter": true,  "count": 2, "items": [...] },
    "no_goals":            { "in_counter": true,  "count": 1, "items": [...] },
    "awaiting_validation": { "in_counter": true,  "count": 1, "items": [...] },
    "formation_errors":    { "in_counter": true,  "count": 1, "items": [...] },
    "lagging":             { "in_counter": false, "count": 0, "items": [] }
  }
}
```

`total_problems` = Σ `count` по категориям с `in_counter: true`.

Errors:
- `400 VALIDATION_ERROR` при отсутствии или невалидном `period_id`

Idempotency: read-only, нет side effects.

---

### `GET /api/v1/admin/settings/health-checkin`

Возвращает текущий конфиг Health Check-in. Default применяется если ключ в БД отсутствует.

Доступен: только администраторам (при `AUTH_MODE=enabled`).

---

### `POST /api/v1/admin/settings/health-checkin`

Обновляет конфиг Health Check-in. Требует admin-роли и CSRF token.

Body: JSON объект с полями `stale_days`, `behind_margin`, `weight_tolerance`, `cache_ttl_minutes`, `in_counter`.

После сохранения сбрасывает in-memory кеш.

Errors: `400` при `stale_days <= 0` или `cache_ttl_minutes <= 0`.

## CSRF requirements for browser mutations

Для вызовов write endpoint'ов требуется CSRF token:

- через заголовок `X-CSRF-Token`; или
- через form field `csrf_token`.

Токен должен совпадать с CSRF cookie приложения; отсутствие cookie или несовпадение токена приводит к `403`, и мутация не выполняется.

CSRF token должен быть ротационным (не постоянным) и обновляться сервером на safe browser requests; клиент обязан подставлять актуальный token при submit/POST.

## Write endpoints

Обязательные write endpoints:

- share goal — `POST /api/v1/goals/{goalID}/share`
- update goal weight
- add goal comment — `POST /api/v1/goals/{goalID}/comments`
- update goal
- create KR — `POST /api/v1/goals/{goalID}/key-results`
- move goal up / down — `POST /api/v1/goals/{goalID}/move-up`, `POST /api/v1/goals/{goalID}/move-down`
- update KR progress — `POST /api/v1/krs/{krID}/progress/percent|boolean|project`
- upsert KR note — `POST /api/v1/krs/{krID}/note`
- update KR — `POST /api/v1/krs/{krID}`
- move KR up / down — `POST /api/v1/krs/{krID}/move-up`, `POST /api/v1/krs/{krID}/move-down`
- update team status — `POST /api/v1/teams/{teamID}/status`
- delete goal — `DELETE /api/v1/goals/{goalID}`
- delete KR — `DELETE /api/v1/krs/{krID}`
- leave goal share — `DELETE /api/v1/goals/{goalID}/share/{teamID}`

### `POST /api/v1/teams/{teamID}/goals`

Создаёт goal для команды в текущем периоде.

Body: JSON объект с полями `title`, `description`, `priority`, `weight`, `work_type`, `focus_type`, `owner_udids` (массив UUID).

- `owner_udids`: массив UDID владельцев цели (заменяет старый `owner_text`).
- Validation: все UDID должны существовать в таблице users → `400 VALIDATION_ERROR` иначе.

### `POST /api/v1/goals/{goalID}`

Обновляет goal (title, description, priority, weight, owner_udids, итд).

Body: JSON объект с полями для обновления, включая `owner_udids: ["uuid1","uuid2"]`.

- `owner_udids`: массив UDID владельцев цели.
- Validation: все UDID должны существовать в таблице users → `400 VALIDATION_ERROR` иначе.

### Admin team endpoints

- `POST /api/v1/admin/teams` — создаёт команду; принимает `lead_udid: "uuid"` (опционально); `lead` для display сохраняется как есть.
- `PATCH /api/v1/admin/teams/{teamID}` — обновляет команду; принимает `lead_udid: "uuid"` (опционально); `lead_udid` валидируется против users → `400 VALIDATION_ERROR` если UDID не существует.

### `DELETE /api/v1/goals/{goalID}`

Удаляет goal или покидает shared goal в зависимости от роли команды.

- Если `requesting_team_id` является owner team goal — goal удаляется полностью (включая все KR и shares).
- Если `requesting_team_id` является shared team — удаляется только запись goal_share для этой команды.
- `requesting_team_id` определяется из контекста авторизации.

Side effects:

- после удаления сервер проверяет, остались ли у команды goals в данном периоде; если нет — автоматически сбрасывает `team_period_status` в `no_goals`.

Validation:

- `NOT_FOUND` если goal не найдена или недоступна текущему пользователю.

### `DELETE /api/v1/krs/{krID}`

Удаляет KR. Доступно, если текущая команда является owner team родительской goal.

Validation:

- `NOT_FOUND` если KR не найден или команда не имеет доступа к родительской goal.

### `DELETE /api/v1/goals/{goalID}/share/{teamID}`

Удаляет запись goal_share для указанной команды. Эквивалентно `DELETE /api/v1/goals/{goalID}` при вызове от имени shared team.

Используется GoalModal при сохранении с выключённым togglem «Общая цель» когда цель ранее была shared.

Validation:

- `403` если вызывающий не имеет доступа к `teamID`.

## Period-aware team visibility

`GET /api/v1/hierarchy?period_id={id}` должен возвращать команды по серверным правилам видимости:

- для актуального периода — все активные команды и soft-deleted команды, у которых уже есть goals/OKR в этом периоде;
- для исторического периода — все активные команды и soft-deleted команды, у которых есть goals/OKR в этом периоде;
- soft-deleted команды без goals не должны попадать в ответ;
- soft-deleted команды могут возвращаться для любого периода, если у них есть goals в выбранном периоде.

`GET /api/v1/teams/{teamID}/okrs?period_id={id}` должен:

- возвращать historical OKR soft-deleted команды, если в этом периоде у неё были goals;
- возвращать `NOT_FOUND`, если команда не должна быть видна в выбранном периоде.

## Требования к новым endpoint’ам

Для любого нового endpoint в spec обязательно фиксировать:

1. method + path;
2. request format;
3. validation rules;
4. success response;
5. error cases;
6. idempotency expectation;
7. side effects on aggregates.

## Acceptance criteria для API

- каждый handler имеет явную валидацию входа;
- ошибки согласованы по shape;
- изменение доменного правила сопровождается тестами;
- нет дублирования business rule между SSR handler и API handler.

## Contract extensions for team OKRs UI

### `GET /api/v1/hierarchy`

Hierarchy node shape расширен полем:

- `lead` — строка с руководителем команды.
- `has_goals` — есть ли у команды goals в выбранном периоде.
- `progress` — прогресс команды (0..100), возвращается только если `has_goals=true`.

Это поле используется sidebar/navigation UI и таблицей дочерних команд.

### `GET /api/v1/teams/{teamID}/okrs?period_id={id}`

Каждый goal в `goals[]` содержит вложенный массив `comments[]`:

```json
"comments": [
  { "id": 1, "text": "...", "author_name": "Ivan", "author_udid": "550e8400-...", "created_at": "..." }
]
```

`key_results[].note` содержит `{ text, author_name, author_udid, updated_at }` или `null`.

`author_udid` — UDID автора комментария; клиент использует его для загрузки данных пользователя через `GET /api/v1/users?ids[]=<udid>`.

Комментарии загружаются батчевым запросом (`ANY($1)`) вместе с goals — без N+1.

Response расширен полем:

- `progress_meta`:
  - `actual` (int, 0..100)
  - `forecast` (int, 0..100)
  - `delta` (int, `actual - forecast`)
  - `status` (`below` | `on_track` | `above`)

Правила расчёта:

- `forecast` рассчитывается сервером на основе доли прошедшего времени периода (`period.start_date .. period.end_date`);
- `status`:
  - `on_track`, если отклонение в диапазоне `[-10, +10]`;
  - `below`, если `delta < -10`;
  - `above`, если `delta > 10`.

`progress_meta` — обобщённая структура для прогнозного прогресс-бара и может переиспользоваться в других endpoint’ах.

### `GET /api/v1/teams/{teamID}/overview?period_id={id}`

Назначение: агрегировать overview по **всей глубине дочерней иерархии** выбранной команды за период.

Request:

- path param: `teamID` (обязательный, int64)
- query param: `period_id` (обязательный, int64)

Success response (`200`):

- `average_progress` — средний прогресс по дочерним командам, у которых есть goals;
- `teams_with_goals` — число дочерних команд с goals в периоде;
- `progress_meta` (тот же shape, что и в `/teams/{teamID}/okrs`):
  - `actual`, `forecast`, `delta`, `status`;
- `priorities`:
  - `p0`, `p1`, `p2`, `p3` — счётчики целей по приоритетам;
- `work_balance`:
  - `discovery`, `delivery` — счётчики целей по типу работы.
- `children_summary`:
  - `period` — информация о периоде (`id`, `name`, `start_date`, `end_date`, `sort_order`);
  - `items[]`:
    - `team` (`id`, `name`, `type`, `type_label`, `parent_id`);
    - `status`, `status_label`;
    - `has_goals` (bool);
    - `progress_meta` (optional; возвращается при `has_goals=true`);
  - `last_updated` (optional; timestamp последнего обновления goals/OKR команды в периоде).
    - вычисляется сервером как максимум по всем KR команды в периоде из:
      - `key_results.progress_updated_at` (последнее обновление прогресса KR);
      - `key_result_comments.created_at` (время последнего комментария к KR).

Validation and errors:

- `VALIDATION_ERROR` при невалидных `teamID` / `period_id`;
- `NOT_FOUND` если период не найден;
- `INTERNAL` при ошибках загрузки иерархии/данных.

Idempotency / side effects:

- endpoint read-only;
- не изменяет доменные агрегаты, только рассчитывает производные метрики для UI.
