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

- `GET /api/v1/admin/settings/access` — текущая политика для новых пользователей (`new_user_policy`, `default_hierarchy_node_id`)
- `POST /api/v1/admin/settings/access` — обновить политику; body: `{"new_user_policy": "default_node", "default_hierarchy_node_id": 42}`

Допустимые значения `new_user_policy`: `empty`, `default_node`.

Все admin API endpoints требуют CSRF token при вызове из браузера.

Ошибки возвращаются в нормализованном виде:

- `VALIDATION_ERROR`
- `NOT_FOUND`
- `CONFLICT`
- `INTERNAL`

## Read endpoints

Обязательные read endpoints:

- `GET /api/v1/hierarchy`
- `GET /api/v1/periods`
- `GET /api/v1/teams/{teamID}`
- `GET /api/v1/teams/{teamID}/okrs`
- `GET /api/v1/teams/{teamID}/overview`
- `GET /api/v1/goals/{goalID}`

## CSRF requirements for browser mutations

Для вызовов write endpoint'ов требуется CSRF token:

- через заголовок `X-CSRF-Token`; или
- через form field `csrf_token`.

Токен должен совпадать с CSRF cookie приложения; отсутствие cookie или несовпадение токена приводит к `403`, и мутация не выполняется.

CSRF token должен быть ротационным (не постоянным) и обновляться сервером на safe browser requests; клиент обязан подставлять актуальный token при submit/POST.

## Write endpoints

Обязательные write endpoints:

- share goal
- update goal weight
- add goal comment
- update goal
- create KR
- move goal up / down
- update KR progress
- add KR comment
- update KR
- move KR up / down
- update team status

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
