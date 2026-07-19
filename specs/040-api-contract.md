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
- Помимо `led_team`, ответ содержит `email` (omitempty). Оба поля используются во всплывающей карточке пользователя (`UserInfo` в tracker.js): при наведении показываются имя, email и команда-руководство.
- Используется UserSelector в GoalModal (tracker.js) через режим `?q=` для выбора владельца цели; через режим `?ids[]=` — для загрузки полных данных пользователей, упоминающихся на странице (авторы комментариев и заметок KR, лиды команд): `UserInfo` лениво дозагружает их по `udid` при наведении (дедуплицируется). Автор заметки KR отображается как `UserInfo` — аватар + имя со всплывающей карточкой, аналогично остальным упоминаниям пользователей.

## Me endpoint

- `GET /api/v1/me` — возвращает текущего пользователя

Response:

```json
{
  "id": 42,
  "udid": "550e8400-e29b-41d4-a716-446655440000",
  "display_name": "Ivan Ivanov",
  "email": "ivan@example.com",
  "avatar_url": "https://...",
  "provider": "google"
}
```

- `udid` — UUID текущего пользователя (тот же идентификатор, что и в публичном API users/hierarchy). Используется клиентом, чтобы сопоставить пользователя с `lead.udid` узлов иерархии (имена не уникальны), напр. на странице настроек для определения команд, где пользователь является лидом.

При `AUTH_MODE=disabled` возвращает системного пользователя `anonymous-local`.

## Config endpoint

- `GET /api/v1/config` — публичная клиентская конфигурация SPA

Доступен любому авторизованному пользователю (при `AUTH_MODE=disabled` — всем). Read-only, без side effects.

Response:

```json
{
  "documentation_url": "https://github.com/ShamanR/okrs/wiki",
  "stale_days": 7,
  "behind_margin": 10,
  "green_threshold": 80,
  "feedback_url": "https://forms.gle/xxxx",
  "feedback_popup_enabled": true,
  "feedback_menu_link_enabled": true,
  "feedback_frequency_days": 30
}
```

- `documentation_url` — ссылка на внешнюю документацию из `tenant_settings`; пустая строка, если не задана. SPA показывает пункт «Документация» в меню пользователя только когда значение непустое.
- `stale_days` — порог «дней без обновления» из настроек Health Check-in (`tenant_settings` ключ `health_checkin_config.stale_days`), по умолчанию `7`. SPA использует его для предупреждения «N дней без обновлений» на страницах целей, чтобы оно совпадало с настройкой Health Check-in.
- `behind_margin` — допустимое отставание (п.п.) от ожидаемого темпа периода из категории «Отстающие» Health Check-in (`health_checkin_config.behind_margin`), по умолчанию `10`. SPA использует его для раскраски процента прогресса команды в sidebar, чтобы она совпадала с этой категорией.
- `green_threshold` — порог прогресса (1..100) из `health_checkin_config.green_threshold`, по умолчанию `80`. Цель или команда с прогрессом не ниже порога считается «в плане» и красится зелёным (и в sidebar, и на странице целей: карточки целей, дочерние карточки, кластерный обзор, верхний прогресс-бар команды) независимо от forecast-проверки темпа.
- `feedback_url` — ссылка на внешний опрос обратной связи из `tenant_settings`; пустая строка, если не задана. Пока пустая — пункт меню «Обратная связь» и всплывающее окно не показываются.
- `feedback_popup_enabled` — включено ли всплывающее окно с просьбой оставить обратную связь (`tenant_settings` ключ `feedback_popup_enabled`), по умолчанию `false`.
- `feedback_menu_link_enabled` — включена ли ссылка «Обратная связь» в футере сайдбара (`tenant_settings` ключ `feedback_menu_link_enabled`), по умолчанию `false`.
- `feedback_frequency_days` — минимальный интервал между показами всплывающего окна (дней) из `tenant_settings`, по умолчанию `30`. Логика показа на стороне SPA через cookies — см. `030-user-flows.md`.

## Admin API endpoints

Доступны только администраторам при `AUTH_MODE=enabled`. При `AUTH_MODE=disabled` доступны всем.

### Пользователи

- `GET /api/v1/admin/users` — **только пользователи активного тенанта**: активные члены и
  запросившие доступ (`memberships.status` = `active`/`requested`); пользователи без membership в
  тенанте не возвращаются. Каждый элемент: поля пользователя (id, display_name, avatar_url,
  provider, last_login_at) + `GrantedNodeCount` (число выданных узлов иерархии, считается
  только активным) + `Status` (`active`/`requested`) + `Role` (роль в тенанте). UI использует
  `Status`, чтобы показывать запросившим кнопки «Добавить»/«Отклонить» (см. ниже), а членам —
  управление доступом.
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

Политика хранится в `tenant_settings` активного тенанта (с миграции 033; ранее — в глобальном `system_settings`) и применяется без перезапуска. Изменение политики не влияет на уже выданные гранты. Все `/api/v1/admin/settings/*` читают и пишут ключи в `tenant_settings` тенанта вызывающего админа.

### Общие настройки

- `GET /api/v1/admin/settings/general` — текущие общие настройки

Response:

```json
{
  "name": "Acme",
  "documentation_url": "https://github.com/ShamanR/okrs/wiki"
}
```

- `POST /api/v1/admin/settings/general` — обновить общие настройки; body: `{"name": "Acme", "documentation_url": "https://github.com/ShamanR/okrs/wiki"}`

Validation:

- `name` — название активного пространства (tenant-admin); trim, непустое, иначе `400 VALIDATION_ERROR`. `id` тенанта берётся из контекста (`TenantScopeFromContext`); `slug` через этот endpoint не меняется (только system-admin через `PATCH /api/v1/system/tenants/{id}`).
- `documentation_url` — пустая строка (очищает ссылку, скрывает пункт меню) или абсолютный http(s) URL; иначе `400 VALIDATION_ERROR`.

Значение хранится в `tenant_settings` (ключ `documentation_url`, per-tenant) и применяется без перезапуска. Публичный `GET /api/v1/config` возвращает его авторизованному пользователю для его активного тенанта.

Общие настройки также включают `empty_hierarchy_message` (markdown, per-tenant ключ `tenant_settings`): текст, показываемый в трекере пользователю без доступных команд (пусто → дефолт). Редактируется в том же `GET/POST /api/v1/admin/settings/general` и возвращается в `GET /api/v1/config` (`empty_hierarchy_message`); рендерится как markdown.

### Настройки обратной связи

- `GET /api/v1/admin/settings/feedback` — текущие настройки сбора обратной связи

Response:

```json
{
  "feedback_url": "https://forms.gle/xxxx",
  "feedback_popup_enabled": true,
  "feedback_menu_link_enabled": true,
  "feedback_frequency_days": 30
}
```

- `POST /api/v1/admin/settings/feedback` — обновить настройки; body совпадает по форме с response выше.

Validation:

- `feedback_url` — любая ссылка (строгого требования http(s), в отличие от `documentation_url`, нет; допускаются ссылки без схемы). Пустая строка скрывает пункт меню и окно. Запрещены только потенциально опасные схемы (`javascript:`, `data:`, `vbscript:`), так как значение подставляется в `href` — иначе `400 VALIDATION_ERROR`.
- `feedback_frequency_days` — целое число `>= 1`; иначе `400 VALIDATION_ERROR`.

Значения хранятся в `tenant_settings` (ключи `feedback_url`, `feedback_popup_enabled`, `feedback_menu_link_enabled`, `feedback_frequency_days`, per-tenant) и применяются без перезапуска. Публичный `GET /api/v1/config` возвращает их авторизованному пользователю для его активного тенанта. Логика показа всплывающего окна — на стороне SPA через cookies (см. `030-user-flows.md`).

### Очистка журнала активности

- `POST /api/v1/admin/activity/purge` — очистить журнал активности **своего** пространства; body: `{"older_than": "quarter" | "year" | "all"}` → `200 {"deleted": <int>}`. `quarter` — старше 3 месяцев, `year` — старше 12 месяцев, `all` — всё. Cutoff считается на сервере. `422`, если `older_than` неизвестен. Write-authority — **admin** тенанта (гейт `RequireTenantAdminMiddleware`), tenant_id из контекста. CSRF обязателен.

### Периоды

- `GET /api/v1/admin/periods` — список **всех** периодов тенанта, включая архивные (в отличие от публичного `GET /api/v1/periods`). Та же форма ответа (`PeriodInfo`: `id`, `name`, `start_date`, `end_date`, `parent_id`, `depth`, `status`) и тот же DFS-порядок, что и у публичного endpoint'а — архивные периоды располагаются в своей зоне после `closed` (см. `020-domain-model.md`).
- `POST /api/v1/admin/periods` — создать период; body: `{"name": "...", "start_date": "2026-01-01", "end_date": "2026-03-31"}`. Полей `parent_id`/`sort_order` нет — вложенность выводится сервером из пересечения дат с уже существующими периодами. `400 VALIDATION_ERROR` при `end_date < start_date` или неуникальном имени в тенанте.
- `PATCH /api/v1/admin/periods/{periodID}` — обновить период; то же тело `{"name", "start_date", "end_date"}`; изменение дат может изменить вычисляемые `parent_id`/`depth` у этого и у других периодов на следующем чтении.
- `DELETE /api/v1/admin/periods/{periodID}` — удалить период.
- `POST /api/v1/admin/periods/{periodID}/archive` — архивировать период (ставит `archived_at = now()`). `200` при успехе; `409 CONFLICT`, если текущий (date-based) статус периода не `closed` — архивировать можно только уже закрытый период, чтобы не прятать из дерева период, который ещё используется.
- `POST /api/v1/admin/periods/{periodID}/unarchive` — разархивировать период (`archived_at = null`), без ограничений по текущему статусу.
- `move-up`/`move-down` для периодов **не существуют** — ручного порядка нет (см. `020-domain-model.md`).

Все mutating endpoints выше доступны только tenant-admin (или всем при `AUTH_MODE=disabled`) и требуют CSRF token.

Все admin API endpoints требуют CSRF token при вызове из браузера.

## System API endpoints (system-admin плоскость)

`/api/v1/system/*` — кросс-тенантная provisioning-плоскость, доступная только
**system-admin** (флаг `users.is_system_admin`) ИЛИ машинному вызывающему с заголовком
`Authorization: Bearer <PROVISIONING_TOKEN>` (instance-level токен в env). Эти endpoints
берут `tenant_id` из URL и не читают тенант из контекста; гейт — `RequireSystemAdmin`.
Не membership-gated (system-admin может не состоять ни в одном тенанте). UI — `/system`.

- `POST /api/v1/system/tenants` — создать тенант; body: `{"name": "...", "slug": "...", "entitlements": {"sso": true}}` → `201` `{id, slug, name, status}`. `422` при невалидном slug, `409` если slug занят.
- `PATCH /api/v1/system/tenants/{id}` — сменить название и slug пространства; body: `{"name": "...", "slug": "..."}` → `200` `{id, slug, name, status}`. `404` если тенант не найден, `409` если slug занят, `422` при невалидном slug или пустом имени. Смена slug — жёсткая замена: старый slug сразу перестаёт резолвиться (join по нему вернёт `404`); alias не сохраняется.
- `GET /api/v1/system/tenants` — список тенантов.
- `POST /api/v1/system/tenants/{id}/members` — прямое назначение membership существующему глобальному пользователю; body: `{"user_id": 1, "role": "admin"}` → `201`. (Самостоятельный онбординг — через пригласительные ссылки, ниже.)
- `PUT /api/v1/system/tenants/{id}/entitlements` — записать ключи `entitlement.*`; body: `{"sso": true, "max_users": 50}` (bare-ключи неймспейсятся в `entitlement.*`) → `204`.
- `POST /api/v1/system/tenants/{id}/suspend` / `POST /api/v1/system/tenants/{id}/restore` → `204`; `404` если тенант не найден.
- `POST /api/v1/system/tenants/{id}/activity/purge` — очистить журнал активности указанного тенанта (управление пространством); body: `{"older_than": "quarter" | "year" | "all"}` → `200 {"deleted": <int>}`. `422` при неизвестном `older_than`. Authority — system-admin; tenant_id из пути. CSRF обязателен.
- `GET /api/v1/system/users` — глобальный (кросс-тенантный) список пользователей.
- `GET /api/v1/system/tenants/{id}/members` — участники тенанта: `[{user_id, display_name, email, role, status}]` (все статусы, отсортировано по имени). UI показывает `requested` вверху с «Подключить» (= `POST …/members`) / «Отклонить» (deny ниже).
- `POST /api/v1/system/tenants/{id}/members/{userID}/deny` — удалить заявку (`requested`-membership) пользователя в тенанте → `204`. На активного члена не действует.
- `PUT /api/v1/system/tenants/{id}/members/{userID}/role` — сменить роль участника; body `{"role": "user"|"admin"}` → `204`. `422` невалидная роль, `404` если у пары `(tenant, user)` нет membership, `409` при попытке понизить последнего активного админа тенанта (тенант всегда сохраняет ≥1 админа). Инвалидирует membership-кэш пользователя. Идемпотентно при установке текущей роли.
- `PUT /api/v1/system/users/{userID}/system-admin` — выдать/снять инстанс-привилегию system-admin (`users.is_system_admin`); body `{"is_system_admin": bool}` → `204`. `404` если пользователь не найден, `409` при снятии последнего system-admin инстанса или при снятии привилегии с собственного аккаунта (защита от self-lockout). Идемпотентно при установке текущего значения. Прямой инстанс-уровневый аналог bootstrap `BOOTSTRAP_SYSTEM_ADMIN` (см. `050-permissions-and-lifecycle.md`).
- `DELETE /api/v1/system/tenants/{id}/members/{userID}` — удалить участника из тенанта: убирает membership (любого статуса) **и** все его hierarchy-гранты в этом тенанте → `204`. Идемпотентно. (Кнопка «Удалить» на активных строках; `deny` выше — только для заявок.)
- `GET /api/v1/system/settings` дополнительно возвращает `no_access_message` (markdown, глобально).
- `PUT /api/v1/system/settings/no-access-message` — body `{"message": "<markdown>"}` → `204`. Текст страницы `/no-access` (пусто → дефолт); рендерится как markdown.
- `GET /api/v1/system/tenants/{id}/entitlements` — текущие ключи `entitlement.*` со срезанным префиксом: `{ "sso": true, "max_users": 50 }`.
- `GET /api/v1/system/settings` — глобальные system-настройки для UI: `{ "default_registration_tenant_id": <int|null> }`.
- `PUT /api/v1/system/settings/default-registration-tenant` — глобальный ключ `default_registration_tenant_id` в `system_settings`; body: `{"tenant_id": 1}` или `{"tenant_id": null}` → `204`.

Авторизация на `/api/v1/system/*` и `/system` обязательна **во всех режимах**, включая
`AUTH_MODE=disabled` (там — только по `PROVISIONING_TOKEN`; `anonymous-local` не system-admin).
UI плоскости — React-панель `/system` (тенанты / участники / пользователи / регистрация / entitlements / сообщения). Вкладка «Участники» даёт смену роли участника (`PUT …/members/{id}/role`); вкладка «Пользователи» — тумблер system-admin (`PUT …/users/{id}/system-admin`), собственная строка задизейблена.

## Onboarding endpoints

**Tenant-admin** (`RequireTenantAdmin`, в активном тенанте):

- `POST /api/v1/admin/invitations` — создать пригласительную **ссылку** (без email); body: `{"role"?: "user|admin", "max_uses"?: int, "expires_at"?: RFC3339}` → `201` `{token, url, role, max_uses}`. `max_uses` отсутствует/`null` → безлимитная; `1` → одноразовая; `N` → до N использований; `max_uses <= 0` → `400`. Токен хранится **хэшированным**; в OSS админ передаёт `url` приглашённому сам (SMTP нет).
- `GET /api/v1/admin/invitations` — список pending-ссылок тенанта: `[{id, role, status, max_uses, use_count, created_at, expires_at}]` (без email).
- `POST /api/v1/admin/invitations/{id}/revoke` — отозвать ссылку (`status='revoked'`); идемпотентно, tenant-scoped → `204`.
- `GET /api/v1/admin/access-requests` — очередь join-request'ов (`status=requested`).
- `POST /api/v1/admin/access-requests/{userID}/approve` → `204` (membership → `active`).
- `POST /api/v1/admin/access-requests/{userID}/deny` → `204` (pending-membership удаляется).
- `DELETE /api/v1/admin/members/{userID}` — отвязать пользователя от активного тенанта: удаляются все его гранты иерархии в этом тенанте и membership (любого статуса), кэш инвалидируется. Идемпотентно → `204`.

**Любой авторизованный** (auth, но **не** membership-gated):

- `POST /api/v1/onboarding/join-request` — запросить доступ по slug; body: `{"slug": "..."}` → `204`; `404` если slug не найден, `409` если уже активный член. Переиспользуется формой «Мои пространства» на `/settings`.
- `GET /api/v1/session/memberships` — свои membership текущего пользователя (**все статусы**, active + requested) для раздела «Мои пространства» на `/settings`: `[{tenant_id, slug, name, role, status}]`, отсортировано по имени тенанта, soft-deleted тенанты исключены. Один join-запрос `memberships ⋈ tenants` (без N+1). Read-only. Держится отдельно от `GET /api/v1/session/tenants`, который остаётся active-only для tenant switcher.
- `DELETE /api/v1/session/memberships/{tenantID}` — выйти из тенанта / отменить свою заявку: удаляет собственный membership вызывающего (любого статуса) и все его hierarchy-гранты в этом тенанте, инвалидирует кэши. Идемпотентно (не-член → `204`) → `204`. `409` если вызывающий — последний активный админ тенанта (иначе тенант осиротеет). Одним эндпоинтом покрывает и «выход» (active), и «отмену заявки» (requested).

**Invite-ссылка (web):** `GET /invite/{token}` — если посетитель **уже авторизован**, ссылка гасится сразу (`Consume`), его `active` membership привязывается к **текущей идентичности** (`provider:subject`), сессия переключается на тенант ссылки, и он редиректится в приложение (`/`). Если посетитель **не авторизован** — токен кладётся в короткоживущую cookie и ведёт на логин; OAuth-callback гасит токен и привязывает membership. Многоразовость определяется `max_uses`/`use_count` (атомарный `Consume`). Истёкший/отозванный/исчерпанный/неизвестный токен — мягкий редирект без ошибки; доступ по email не выдаётся.

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
- `GET /api/v1/activity`
- `GET /api/v1/activity/tree-counts`
- `GET /api/v1/activity/category-counts`

### `GET /api/v1/activity`

Лента событий журнала активности (см. `ActivityEvent` в `020-domain-model.md`). Read-only,
tenant-scoped. **Доступ — только tenant-admin** (`RequireTenantAdmin`, `memberships.role = admin`);
не-админ получает `403`. При `AUTH_MODE=disabled` доступен всем (`anonymous-local` — admin). Админ
видит **все** события своего тенанта; событие с неизвестной командой (`team_id IS NULL`) в ленту
не попадает (fail-closed). Share-aware audience-фильтрация (`PolicyEvaluator`) в коде сохраняется,
но для admin-scope охватывает все команды тенанта.

Query-параметры (все опциональны): `period_id`, `team_ids[]` (фильтр по audience выбранных
команд), `category` (`progress`|`composition`|`status`|`discussion`), `actor_udid`, `range`
(`today`|`7d`|`30d`|`all`, по умолчанию `all`), `q` (поиск по заголовку/payload/автору),
`cursor`, `limit` (по умолчанию 50, максимум 100).

Ответ: `{ "items": [ActivityEvent], "next_cursor": "<opaque>" }`. Курсорная пагинация по
`(created_at, id)` убыванию. Каждый `item`: `id`, `category`, `action`, `actor`
(`{udid, display_name, avatar_url, removed}` — для бывшего участника `removed=true` и без
PII), `team_id`/`period_id`/`goal_id`/`kr_id`/`comment_id`, `entity_title`, `target`
(`{section:"tracker", team_id, period_id?, goal_id?, kr_id?, comment_id?}` — структурный
дескриптор перехода; `null`, если у события нет команды), `payload` (`before`/`after` + доп.),
`created_at`. `target.team_id` — **доступная зрителю** команда: owner-команда цели, если она
доступна, иначе одна из shared-команд из audience, доступная зрителю (зритель может видеть событие
расшаренной цели по `goal_shares`, не имея доступа к owner-команде — ссылка не должна вести на
недоступную доску). Вычисляется на чтении из набора доступных команд.

### `GET /api/v1/activity/tree-counts`

**Доступ — только tenant-admin** (тот же гейт, что и у `GET /api/v1/activity`); не-админ — `403`.
Query: `period_id` (опц.), `range` (опц.). Ответ: `{ "counts": { "<team_id>": <int> } }` —
прямые счётчики событий по команде за период+диапазон (audience-раскрытие: событие расшаренной
цели считается каждой командой из audience), в рамках доступного scope. Фронт агрегирует по
поддереву. Категория/автор/`q` на счётчики не влияют.

### `GET /api/v1/activity/category-counts`

**Доступ — только tenant-admin** (тот же гейт, что и у `GET /api/v1/activity`); не-админ — `403`.
Query: те же фильтры, что у `GET /api/v1/activity`, **кроме `category` и `cursor`** (`period_id`,
`team_ids[]`, `range`, `actor_udid`, `q`). Ответ: `{ "counts": { "<category>": <int> }, "total": <int> }`.
Даёт счётчики для табов ленты, стабильные при переключении категории (сам фильтр по категории
исключён из подсчёта). Фильтр «Избранное» на клиенте применяется передачей `team_ids[]` избранных
команд (серверная фильтрация до `LIMIT`/cursor) — поэтому пагинация, счётчики и матчинг событий
расшаренных целей корректны.

### `GET /api/v1/periods`

Назначение: список периодов текущего тенанта для селекторов на странице целей, в админке и в Health Check-in.

Доступен: любому авторизованному пользователю (при `AUTH_MODE=disabled` — всем). Read-only, без параметров.

Success response (`200`, массив `PeriodInfo`):

```json
[
  { "id": 12, "name": "2026", "start_date": "2026-01-01", "end_date": "2026-12-31", "parent_id": null, "depth": 0, "status": "active" },
  { "id": 13, "name": "Q1 2026", "start_date": "2026-01-01", "end_date": "2026-03-31", "parent_id": 12, "depth": 1, "status": "closed" }
]
```

- `parent_id` / `depth` / `status` — вычисляемые поля, правила см. в `020-domain-model.md` («Производные вычисления»); `sort_order` в ответе нет.
- **Архивные периоды (`status="archived"`) не возвращаются** этим endpoint'ом; чтобы увидеть их, нужен `GET /api/v1/admin/periods`.
- Элементы уже отсортированы сервером по DFS-правилу вложенности/статуса (см. `020-domain-model.md`) — клиент не должен пересортировывать список, а может использовать `depth` только для визуального отступа.

Idempotency: read-only, без side effects.

---

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
    "lagging":             { "in_counter": false, "count": 0, "items": [] },
    "comments":            { "in_counter": false, "count": 2, "unresolved": [...], "resolved": [...] }
  }
}
```

`total_problems` = Σ `count` по категориям с `in_counter: true`.

Категория `comments` имеет форму `{ in_counter, count, unresolved: [...], resolved: [...] }` (вместо `items`). `count` = число нерешённых комментариев (`unresolved`) в scope пользователя (его lead-команды + спуск на `comment_depth` уровней + owner-команды); в `total_problems` входит только при `in_counter.comments = true` (по умолчанию false). `resolved` — последние `resolved_comments_limit` комментариев пользователя, решённых **не** им самим, по `resolved_at` убыв.; в серверный `total_problems` эта величина **не** входит — их «непросмотренный» счётчик считается на клиенте (watermark в `localStorage`). Элемент `unresolved`: `team_id, team_name, team_path, goal_id, goal_title, comment_id, author_name, text, created_at`. Элемент `resolved`: те же поля + `resolved_at, resolved_by_name` (без `author_name`).

Категория `stale` («N дней без обновления») — сигнал фазы исполнения: считается только для команд в статусе `in_progress` («в работе»). Точка отсчёта порога — последнее обновление прогресса KR цели; если обновлений не было, порог отсчитывается от начала периода (`period.start_date`). Цель попадает в категорию только если прошло больше `stale_days` дней от этой точки, поэтому не начавшийся период (`start_date` в будущем) целей в категорию не добавляет. Для статусов `forming` (черновик), `ready` (к валидации), `closed` (закрыт), а также для команд без записи статуса за период (ещё не переведены в работу) предупреждение об отсутствии обновлений не применяется — такие цели не исполняются активно. То же правило действует для предупреждения на карточке цели в трекере.

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

Body: JSON объект с полями `stale_days`, `behind_margin`, `weight_tolerance`, `cache_ttl_minutes`, `green_threshold`, `comment_depth`, `resolved_comments_limit`, `in_counter` (включая ключ `comments`). Валидация: `stale_days` и `cache_ttl_minutes` > 0, `green_threshold` в диапазоне 1..100, `comment_depth` >= 0, `resolved_comments_limit` >= 1.

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
- resolve goal comment — `POST /api/v1/goals/{goalID}/comments/{commentID}/resolve`
- reopen goal comment — `POST /api/v1/goals/{goalID}/comments/{commentID}/unresolve`
- update goal
- create KR — `POST /api/v1/goals/{goalID}/key-results`
- move goal up / down — `POST /api/v1/goals/{goalID}/move-up`, `POST /api/v1/goals/{goalID}/move-down`; форма содержит `team_id` — команду, в чьём представлении периода меняется порядок. Перемещение действует на упорядоченный список этой команды (её собственные цели по `goals.sort_order` вперемешку с общими целями по `goal_shares.sort_order`), поэтому порядок общих целей можно менять в каждой команде независимо, не затрагивая порядок в других командах. Требует доступ к `team_id`; цель должна принадлежать команде (как владелец) или быть в неё расшарена.
- update KR progress — `POST /api/v1/krs/{krID}/progress/numerical|boolean|project`
- upsert KR note — `POST /api/v1/krs/{krID}/note`
- update KR description — `POST /api/v1/krs/{krID}/description` body `{ "description": <string> }`; обновляет только описание KR. Доступен при тех же условиях, что и заметка (проверка доступа к команде-владельцу), поэтому описание можно добавить из модалки обновления прогресса, когда полное редактирование KR заблокировано статусом.
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
- Ответ: `{ "id": <goalID> }`.
- Если в GoalModal при создании включён toggle «Общая цель» с выбранными командами, клиент после успешного создания вызывает `POST /api/v1/goals/{goalID}/share` с выбранными командами (по аналогии с режимом редактирования).

### `POST /api/v1/goals/{goalID}`

Обновляет goal (title, description, priority, weight, owner_udids, итд).

Body: JSON объект с полями для обновления, включая `owner_udids: ["uuid1","uuid2"]`.

- `owner_udids`: массив UDID владельцев цели.
- Validation: все UDID должны существовать в таблице users → `400 VALIDATION_ERROR` иначе.

### Admin team endpoints

- `POST /api/v1/admin/teams` — создаёт команду; принимает `lead_udid: "uuid"` (опционально); `lead` для display сохраняется как есть.
- `PATCH /api/v1/admin/teams/{teamID}` — обновляет команду; принимает `lead_udid: "uuid"` (опционально); `lead_udid` валидируется против users → `400 VALIDATION_ERROR` если UDID не существует.

### `DELETE /api/v1/goals/{goalID}`

Удаляет goal от имени команды-владельца (`requesting_team_id` = owner team).

- Если у цели есть привязки к другим командам (shared) — владелец открепляется, владение переходит к одной из оставшихся команд-участников (с наименьшим `sort_order`), цель сохраняется.
- Если привязок нет — goal удаляется полностью (включая все KR).
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

Открепляет указанную команду от цели.

- Если `teamID` — команда-участник (не владелец) — удаляется её запись goal_share.
- Если `teamID` — команда-владелец — владение переходит к одной из оставшихся команд-участников (с наименьшим `sort_order`); если других команд нет, цель удаляется полностью.

Используется GoalModal при сохранении с выключённым toggle «Общая цель» (цель ранее была shared), а также кнопкой «Удалить» на карточке общей цели — она открепляет текущую команду.

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
- `description` — описание команды (omitempty; пустая строка опускается). Используется страницей настроек для предпросмотра/редактирования описаний.
- `has_goals` — есть ли у команды goals в выбранном периоде.
- `progress` — прогресс команды (0..100), возвращается только если `has_goals=true`.
- `forecast` — прогнозный прогресс периода (0..100, та же формула, что и `progress_meta.forecast`), возвращается только если `has_goals=true`. Одинаков для всех узлов в рамках периода.
- `status` — `team_period_status` команды (`no_goals` | `forming` | `ready` | `in_progress` | `closed`); присутствует при запросе с `period_id`.

Эти поля используются sidebar/navigation UI и таблицей дочерних команд. Sidebar раскрашивает кружок узла по типу команды (`type`), а процент прогресса — зелёным, когда команда идёт по плану (`forecast - progress ≤ behind_margin`) либо при `status=closed` достигла ≥80%, и красным, когда отстаёт. `behind_margin` берётся из `GET /api/v1/config` (категория «Отстающие» Health Check-in), что синхронизирует раскраску sidebar с этой категорией.

### `GET /api/v1/teams/{teamID}/okrs?period_id={id}`

Каждый goal в `goals[]` содержит вложенный массив `comments[]`:

```json
"comments": [
  {
    "id": 1, "text": "...", "author_name": "Ivan", "author_udid": "550e8400-...", "created_at": "...",
    "resolved": true, "resolved_by_name": "Petr", "resolved_by_udid": "660e...", "resolved_at": "..."
  }
]
```

Комментарий трактуется как замечание. `resolved` (bool) — решено ли; при `resolved=false` поля `resolved_by_name`/`resolved_by_udid` пустые, а `resolved_at` — `null`. Отметить решённым / вернуть можно эндпоинтами `.../comments/{commentID}/resolve` и `.../unresolve` (доступ — как к добавлению комментария: любой пользователь в scope цели); несуществующий/чужой комментарий → `404`.

#### Key Result measure

`key_results[].kind` ∈ `BOOLEAN | PROJECT | NUMERICAL`. Поле `key_results[].zeroing_criteria` — опциональный текстовый критерий обнуления (человекочитаемый, в расчётах не применяется), доступен на уровне KR для любого `kind`. Поле `key_results[].measure` несёт данные по типу:

- `measure.boolean`: `{ is_done }`
- `measure.project`: `{ stages: [{ id, title, weight, is_done }] }`
- `measure.numerical`: `{ start_value, target_value, current_value, unit, checkpoints: [{ value, progress_percent }] }`

`unit` — значение из закрытого справочника: `%`, `RPS`, `мс`, `сек`, `мин`, `час`, `дней`, `шт`, `₽`, `запросов`, `ошибок`, `пользователей`, `заказов`, `рублей`. Варианта «другое» и поля `custom_unit` нет.

`checkpoints` хранятся в JSONB-колонке `key_results.checkpoints` и загружаются вместе с KR — без отдельной таблицы и без дополнительных запросов на каждый KR.

**Create / update KR** (`multipart/form-data`): `title`, `description`, `weight`, `kind`, опциональный `zeroing_criteria` (для любого `kind`). Для `kind=NUMERICAL`: `numerical_unit` (из справочника), `numerical_start`, `numerical_target`, `numerical_current` и повторяющиеся пары `checkpoint_value[]` / `checkpoint_percent[]` (проценты 0..100, значения не дублируются). Для `kind=BOOLEAN`: `boolean_done`. Для `kind=PROJECT`: `step_title[]`, `step_weight[]`, `step_done[]`.

**Update KR progress** `POST /api/v1/krs/{krID}/progress/numerical` принимает `{ "current_value": <number> }`.

`key_results[].note` содержит `{ text, author_name, author_udid, updated_at }` или `null`.

`author_udid` — UDID автора комментария; клиент использует его для загрузки данных пользователя через `GET /api/v1/users?ids[]=<udid>`.

Комментарии загружаются батчевым запросом (`ANY($1)`) вместе с goals — без N+1.

Объект `team` содержит `id`, `name`, `type`, `type_label`, `description` (описание команды; пустая строка опускается), `lead`, `parent_id`.

Текстовые поля `description` (goal, key result, team), `key_results[].note.text` и `comments[].text` несут подмножество CommonMark (жирный, курсив, заголовки, списки, цитаты, инлайн-код, ссылки). Хранятся как сырой Markdown; рендерятся и санитизируются на клиенте (DOMPurify, ограниченный allowlist; ссылки открываются в новой вкладке). Форма ответа не меняется — поля остаются строками.

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
  - `period` — информация о периоде (`id`, `name`, `start_date`, `end_date`, `status`); объект использует ту же форму `PeriodInfo`, что и `GET /api/v1/periods` (включая поля `parent_id`/`depth`), но для этого endpoint'а они не заполняются (`parent_id: null`, `depth: 0`) — реальная вложенность периода берётся из `GET /api/v1/periods`; `status` вычисляется так же, как описано в `020-domain-model.md`;
  - `items[]`:
    - `team` (`id`, `name`, `type`, `type_label`, `description`, `parent_id`);
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
