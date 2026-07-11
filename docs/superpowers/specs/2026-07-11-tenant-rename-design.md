# Дизайн: смена названия и slug пространства

Дата: 2026-07-11
Статус: согласован, готов к плану реализации

## Контекст

«Пространство» в продукте — это сущность **Tenant** (`internal/domain/tenant.go`), с
полями `Name` и `Slug`. Сейчас `name`/`slug` можно задать только при создании тенанта
(`POST /api/v1/system/tenants`); методов обновления нет ни в репозитории, ни в сервисе,
ни в HTTP-слое. Единственная мутация тенанта — смена статуса (`SetStatus`, suspend/restore).

Slug уникален (`tenants.slug NOT NULL UNIQUE`, миграция `027_tenants`) и валидируется в
домене `domain.ValidTenantSlug` (lowercase, 2..32 символа, без ведущего/замыкающего дефиса,
зарезервированные слова заблокированы). `name` уникальности не имеет.

Slug используется в онбординге: join-request по slug (`POST /api/v1/onboarding/join-request`,
страница `/no-access`, раздел «Мои пространства» на `/settings`).

Кэш тенантов (`internal/store/tenants/cache.go`) — TTL 5 минут, keyed по id и slug.
`Invalidate(id)` корректно чистит и старый slug (сканирует `bySlug` по `ID`).

## Цель (scope)

- **Tenant-admin** (`/admin?section=settings`): может менять **только название** своего
  активного пространства.
- **System-admin** (`/system`, таб «Пространства»): может менять **название + slug** любого
  пространства.
- Смена slug — **жёсткая замена**: старый slug сразу перестаёт резолвиться, join по нему
  вернёт `404`. Alias/редирект старого slug не делаем (YAGNI).

Вне scope: переименование через личные настройки `/settings`; массовые операции; история
переименований/аудит.

## Контроль доступа

### System — `PATCH /api/v1/system/tenants/{id}`
- Гейт `RequireSystemAdminMiddleware` (навешан на всю группу `registerSystemRoutes`) —
  пропускает сессию с `users.is_system_admin` **или** машинный вызов с `PROVISIONING_TOKEN`.
  Работает даже при `AUTH_MODE=disabled` (без bypass).
- `tenant_id` берётся из URL (штатный паттерн system-плоскости; system-admin кросс-тенантный).
- CSRF-токен обязателен при вызове из браузера.

### Admin — `POST /api/v1/admin/settings/general` (+ поле `name`)
- Гейт `RequireTenantAdminMiddleware` — только `memberships.role=admin` активного тенанта;
  обычный член → `403`.
- `id` тенанта берётся строго из `auth.TenantScopeFromContext(ctx)`, **не** из body и **не**
  из URL → tenant-admin не может переименовать чужой тенант.
- Поле `slug` в этом endpoint **не читается** → tenant-admin не может сменить slug.
- CSRF обязателен.

## Изменения по слоям

### Store — `internal/store/tenants/tenants.go`
Новые sentinel-ошибки: `ErrInvalidName`.

Новые методы `TenantRepository`:
- `Rename(ctx, id, name)` — `UPDATE tenants SET name=$2 WHERE id=$1`. Trim + непустое имя,
  иначе `ErrInvalidName`. `ErrNotFound` при 0 затронутых строк.
- `Update(ctx, id, name, slug)` — один `UPDATE` обеих колонок. Валидирует имя (как выше) и
  slug через `domain.ValidTenantSlug` (`ErrInvalidSlug`); маппит unique-violation `23505` →
  `ErrSlugTaken`; `ErrNotFound` при 0 строк.

### Service — `internal/service/provisioning.go`
- `RenameTenant(ctx, id, name)` — `Rename` + `tenantCache.Invalidate(id)`.
- `UpdateTenant(ctx, id, name, slug)` — `Update` + `tenantCache.Invalidate(id)`.

### HTTP

**System** (`internal/http/handlers/api/v1/system/handler.go`, роут в `registerSystemRoutes`):
- `PATCH /api/v1/system/tenants/{id}` — body `{"name": "...", "slug": "..."}` →
  `200 {id, slug, name, status}`.
  - `404` — тенант не найден;
  - `409` — slug занят (`ErrSlugTaken`);
  - `422` — невалидный slug (`ErrInvalidSlug`) или пустое имя (`ErrInvalidName`).
- Метод добавляется в интерфейс `Provisioner`, используемый хэндлером.

**Admin** (`internal/http/handlers/api/v1/admin/handler.go`):
- `GET /api/v1/admin/settings/general` — в ответ добавляется `"name"` (текущее имя тенанта,
  читается из resolved-тенанта в контексте).
- `POST /api/v1/admin/settings/general` — дополнительно принимает `"name"` (trim, непустое,
  иначе `400`). Хэндлер оркестрирует: `name` пишется в колонку через `RenameTenant`,
  `documentation_url`/`empty_hierarchy_message` — как и раньше через `SetTenantProduct`.
- Admin-хэндлер зависит от узкого интерфейса `TenantRenamer { RenameTenant(ctx, id, name) error }`
  (реализует provisioning-сервис), чтобы cross-tenant surface не протекала в admin-слой.

### Frontend
- **`internal/web/static/admin.js` → `GeneralSettingsPanel`** (около строки 1094): поле
  «Название пространства» (через `Field`/`inpStyle`) вверху панели; грузится из расширенного
  GET, сохраняется общим `save()`. Клиентская проверка на непустоту.
- **`internal/web/static/system.js` → `TenantsSection`** (около строки 25): на строку тенанта —
  действие «Изменить» → inline-режим с полями name+slug → «Сохранить» (PATCH) / «Отмена».
  Показ ошибок `409` (slug занят) / `422` (формат). Реюз инлайновых стилей секции (модалки в
  system.js нет).

## Кэш и консистентность (multi-instance)

`Invalidate(id)` чистит локальный кэш инстанса (и старый, и новый slug). В K8s на других подах
старое имя/slug могут резолвиться ещё до истечения TTL (5 минут) — это тот же eventual-consistency,
что уже действует для suspend/restore. Приемлемо: пользовательское ожидание — «изменение
применяется в течение нескольких минут на всех узлах».

## Lifecycle-чеклист (по требованию `specs/050-permissions-and-lifecycle.md`)

- Зависит от `team period status`? — Нет.
- Разрешено в `validated`/`closed`? — Да (операция на уровне тенанта, к периодам не относится).
- Проверяется на сервере? — Да (гейты доступа + валидация в store/service).
- Зависит от будущих permissions/roles? — Нет.

## Обновления spec (в том же change set)

- `specs/040-api-contract.md`: добавить `PATCH /api/v1/system/tenants/{id}`; поле `name` в
  `GET/POST /api/v1/admin/settings/general`.
- `specs/050-permissions-and-lifecycle.md`: tenant-admin может переименовать свой тенант
  (только name); system-admin — name+slug.
- `specs/020-domain-model.md`: отметить, что `name` изменяемо (tenant-admin/system-admin),
  `slug` изменяем system-admin'ом (жёсткая замена, старый slug освобождается).

## Тесты

- **Store:** `Rename` (успех, пустое имя → `ErrInvalidName`, не найден → `ErrNotFound`);
  `Update` (успех, slug занят → `ErrSlugTaken`, невалидный slug → `ErrInvalidSlug`, не найден).
- **Service/cache:** после `RenameTenant`/`UpdateTenant` старый slug перестаёт резолвиться
  локально, новый резолвится.
- **Handler — admin:** `POST /admin/settings/general` с `name` переименовывает **только свой**
  тенант (id из контекста); `slug` в body игнорируется; пустое имя → `400`; обычный член → `403`.
- **Handler — system:** `PATCH /system/tenants/{id}` — `200`; `409`/`422`/`404`; без
  system-admin → `403`; с `PROVISIONING_TOKEN` → OK.
- **Seed demo:** без изменений (новых таблиц нет).
