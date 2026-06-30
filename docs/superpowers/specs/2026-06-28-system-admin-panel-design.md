# System-admin panel (`/system`) — design

Статус: проектная спека. Управляющий интерфейс для system-admin плоскости: создание тенантов,
подключение пользователей, дефолт-тенант регистрации и entitlements. Опирается на provisioning
API из Tenant Foundation Plan 3 и app-фасад из Plan 5.

## Контекст и цель

System-admin плоскость (`/api/v1/system/*` + минимальный `/system` shell) появилась в Plan 3:
provisioning API готов целиком, но UI — это заглушка (список тенантов + форма создания на
vanilla-JS). Цель — полноценная React-панель `/system` (объём C), покрывающая весь жизненный
цикл управления тенантами и их участниками, в едином стиле с существующей `/admin` (React,
`admin.js` / `admin_shell.html`).

Это **не** tenant-admin плоскость (`/admin`, per-tenant) и **не** SaaS control-plane (отдельный
потребитель API). Это инструмент оператора инстанса (OSS), он же сервис-аккаунт в SaaS.

## Гейт и доступ

Панель и **все** её эндпоинты (`/system` shell + весь `/api/v1/system/*`, включая GET-чтения)
живут за `auth.RequireSystemAdminMiddleware` (сессия `users.is_system_admin` ИЛИ
`Authorization: Bearer <PROVISIONING_TOKEN>`), tenant-less группа (вне membership-гейта).

**Авторизация обязательна всегда — даже в `AUTH_MODE=disabled`.** Это меняет текущее поведение:
сейчас `registerSystemRoutes` навешивает гейт под `if !s.auth.Disabled()`, то есть в no-auth
режиме `/system/*` открыт всем. Убираем это исключение — гейт применяется безусловно.
Следствие: `anonymous-local` (no-auth режим) **не** system-admin, поэтому в `AUTH_MODE=disabled`
доступ к `/system` возможен только по `PROVISIONING_TOKEN`; без настроенного токена и без
bootstrap'нутого system-admin плоскость недоступна (безопасно by default). Управление тенантами
не должно быть открыто анонимно ни в каком режиме.

## Архитектура

### Frontend

- **`internal/web/static/system.js`** (new) — React-приложение (React 18 UMD + Babel standalone,
  тот же набор скриптов, что `admin.js`). Одна точка монтирования `#root`, секции переключаются
  табами внутри SPA. Рендер — только через React/`textContent`-семантику (no raw HTML; правило
  `010` про XSS).
- **`internal/http/templates/system_shell.html`** (rewrite) — из минимальной vanilla-страницы в
  React-shell по образцу `admin_shell.html`: грузит React UMD, Babel, `header.js`, `system.js`.
  Маршрут `GET /system` (уже есть) рендерит этот shell.

Секции панели (одна ответственность на секцию):

1. **Тенанты** — таблица (`id`, `slug`, `name`, `status`); «Создать» (`name` + `slug`,
   клиентская валидация slug под грамматику `domain.ValidTenantSlug`); на строке `suspend` (для
   `active`) / `restore` (для `suspended`).
2. **Участники** — выбор тенанта → список текущих участников (`display_name`, `email`, `role`) +
   форма «Подключить»: поиск по глобальному списку пользователей (client-side фильтр по
   `display_name`/`email`), роль `user`/`admin` → attach.
3. **Регистрация** — селектор `default_registration_tenant_id`: один из тенантов или «нет»
   (`null` → новый юзер без membership уходит на no-membership-страницу).
4. **Entitlements** — выбор тенанта → редактор ключей `entitlement.*`: известные
   `sso`/`subdomains`/`file_uploads` (bool), `max_users` (int) + поле для произвольного ключа →
   сохранить. Значения предзаполняются текущими.

### Backend

Существующие эндпоинты (Plan 3, не меняются): `POST /api/v1/system/tenants`,
`GET /api/v1/system/tenants`, `POST /api/v1/system/tenants/{id}/members`,
`PUT /api/v1/system/tenants/{id}/entitlements`, `POST …/{id}/suspend|restore`,
`GET /api/v1/system/users`, `PUT /api/v1/system/settings/default-registration-tenant`.

Добавляются два **read**-эндпоинта (под тем же гейтом):

- **`GET /api/v1/system/tenants/{id}/members`** → `[{user_id, display_name, email, role, status}]`.
  Источник: `memberships ⋈ users` по `tenant_id`, **все статусы** (`active` и `requested`),
  отсортировано по `display_name`; `status` в ответе, чтобы оператор видел и подключённых, и
  ожидающих. Новый метод репозитория
  `MembershipRepository.ListByTenant(ctx, scope) ([]AccessRequest, error)` (та же read-модель, что
  `ListAccessRequests`, но без фильтра `status='requested'`).
- **`GET /api/v1/system/tenants/{id}/entitlements`** → `{ "sso": true, "max_users": 50, ... }`
  (только ключи с префиксом `entitlement.`, префикс в ответе срезается). Источник: снапшот
  `tenant_settings` через `SettingsService`. Новый метод
  `SettingsService.TenantEntitlements(ctx, scope) (map[string]json.RawMessage, error)` — фильтрует
  снапшот по префиксу `entitlement.` и срезает его.

Также для секции «Регистрация» нужно **прочитать** текущий `default_registration_tenant_id`
(сейчас есть только PUT). Добавляется чтение в `GET /api/v1/system/settings` →
`{ "default_registration_tenant_id": <int|null> }` (через `SettingsService.SystemGet`).

### Поток данных

Панель при загрузке тянет `GET /system/tenants`, `GET /system/users`, `GET /system/settings`.
Секции 2 и 4 при выборе тенанта дотягивают `…/{id}/members` и `…/{id}/entitlements`. Мутации
(`POST/PUT`) шлют CSRF-токен из cookie (как `admin.js`), после успеха — рефетч затронутого
ресурса. Ошибки API (`{"error": "..."}`) показываются инлайн в секции.

## Обработка ошибок

- Создание тенанта: `422` (невалидный slug) / `409` (slug занят) → инлайн-сообщение под формой.
- Attach/suspend/restore/set-entitlements: `4xx/5xx` → инлайн-сообщение в секции; список
  ревалидируется только при `2xx`.
- Suspended-тенант: `suspend` повторно — идемпотентно (статус уже suspended); кнопка показывает
  обратное действие.

## Тестирование

- **Backend (Go, testcontainers/httptest):** новые `GET …/members` и `GET …/entitlements` и
  `GET /system/settings` — happy-path + гейт (не-system-admin → `403`; в `AUTH_MODE=disabled`
  без provisioning-token → `403`, с валидным Bearer-токеном → `2xx`). `ListByTenant` репозитория
  — изоляция по тенанту (участники тенанта A не видны под scope B). `TenantEntitlements` сервиса —
  возвращает только `entitlement.*` со срезанным префиксом, продуктовые ключи не утекают.
- **Frontend:** ручная проверка по DoD (`010`): создать тенант, подключить юзера, выставить
  дефолт-тенант, записать entitlement; перезагрузка отражает состояние. (Авто-тестов фронта в
  проекте нет — следуем существующей практике.)

## Решения и ограничения (YAGNI)

- **Поиск пользователей — client-side** фильтр поверх `GET /system/users` (отдаёт всех). Для
  Phase 0 достаточно; server-side `?q=` — follow-up при тысячах юзеров. Зафиксировано как
  осознанное ограничение.
- **Entitlements в OSS не влияют на рантайм:** `entitlements.UnlimitedEntitlements` игнорирует
  ключи; редактор пишет их в `tenant_settings` как задел для SaaS-билда, который их читает. В
  OSS-панели рядом с секцией — поясняющая подпись «в OSS не ограничивает; задел для SaaS».
- **Отключение участника** (remove membership) — **вне scope** этого шага (есть deny для
  join-request'ов в `/admin`, но system-level remove — отдельная фича). Панель только подключает.
- **Редактирование name/slug тенанта** — вне scope (slug иммутабелен по спеке; rename name —
  follow-up).
- Никакой новой схемы БД; только новые read-методы поверх существующих таблиц.

## Вне scope

- Server-side поиск пользователей; пагинация списков.
- Удаление/отвязка участников, rename тенанта, soft-delete тенанта из UI.
- SaaS control-plane (отдельный сервис) — потребляет тот же API, не часть этой панели.
