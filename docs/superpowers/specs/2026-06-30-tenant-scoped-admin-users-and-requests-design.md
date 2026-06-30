# Tenant-scoped admin users + access-request actions — design

Статус: проектная спека. Делает список пользователей в `/admin` tenant-scoped и добавляет
обработку заявок на доступ (approve/deny) в обе админ-плоскости: tenant-admin (`/admin`) и
system-admin (`/system`). Опирается на онбординг из Tenant Foundation Plan 4 (join-request,
`memberships.status = requested`, approve/deny endpoints) и system-панель.

## Контекст и цель

Сейчас `/admin?section=users` показывает **всех глобальных** пользователей
(`GET /api/v1/admin/users` → `users.ListUsers`), хотя `/admin` уже tenant-scoped
(`RequireTenantAdmin`). Это утечка кросс-тенантных данных и шум: tenant-admin видит людей, не
имеющих отношения к его тенанту. Заявки на доступ (`status = requested`) при этом видны только в
отдельной очереди и никак не отражены в списке пользователей.

Цель:
1. `/admin?section=users` показывает **только** пользователей, связанных с активным тенантом:
   активных членов (`status = active`) и запросивших доступ (`status = requested`).
2. Запросившие видны в этом же списке с действиями «Добавить» (approve) / «Отклонить» (deny).
3. `/system` → участники: запросившие отображаются вверху, с кнопками «Подключить» / «Отклонить».

Никакой новой схемы БД — только новый read-запрос и один write-эндпоинт (system deny).

## Семантика

- **Связь с тенантом** = наличие строки в `memberships` для `(user, активный тенант)`.
  Статусы: `active` (имеет доступ) и `requested` (запросил). Пользователи без membership в
  тенанте в списке `/admin` не отображаются.
- **Approve / «Добавить» / «Подключить»** = membership → `active`.
- **Deny / «Отклонить»** = удаление `requested`-membership (как в Plan 4: `DeleteRequested`).
  Пользователь может запросить доступ повторно. Применять можно только к `requested` (не к
  активным членам — это не «удаление участника», которое вне scope).

## Req 1 — tenant-scoped список пользователей в `/admin`

### Backend

- Новый `(*users.UserRepository) ListByTenant(ctx, scope domain.TenantScope) ([]TenantUser, error)`,
  где `TenantUser { User *domain.User; Status domain.MembershipStatus; Role domain.Role }`.
  Запрос: `memberships m JOIN users u ON u.id = m.user_id WHERE m.tenant_id = $1`, отсортировано
  по `u.display_name`; возвращает полного `domain.User` (для существующих полей UI) + `status` +
  `role` из membership.
- `HandleListUsers` (`/api/v1/admin/users`) переписывается: вместо `users.ListUsers(ctx)` берёт
  `users.ListByTenant(ctx, scope)`. Grant-count считается как сейчас, но только для активных
  членов (у `requested` грантов нет → `0`). Ответ на элемент:
  `{ ...domain.User (PascalCase, как сейчас), GrantedNodeCount int, Status string, Role string }`.
  Поля `Status`/`Role` — новые; остальное совместимо с текущим `admin.js`.
- Прочие admin-user-эндпоинты (`/admin/users/{id}/admin`, `/grants`) не меняются; они и так
  scoped и применяются к членам тенанта.

### Frontend (`admin.js` `UsersSection`)

- Маппинг ответа дополняется `Status`/`Role`.
- Строки c `Status === 'requested'` рендерятся как «заявка»: вместо grant-управления — две
  кнопки. Активные члены — как сейчас (грант-бейдж, управление доступом, admin-тогл).
- Фильтр «без доступа» (`!IsAdmin && GrantedNodeCount===0`) не должен помечать запросивших как
  «без доступа» — они отдельная категория (по `Status`).

## Req 2 — approve/deny в `/admin?section=users`

- Кнопки на строке-заявке дёргают **существующие** (Plan 4) tenant-admin эндпоинты:
  - «Добавить» → `POST /api/v1/admin/access-requests/{userID}/approve` → `204`.
  - «Отклонить» → `POST /api/v1/admin/access-requests/{userID}/deny` → `204`.
- После `204` — reload списка (`reload()`), запись либо станет активным членом, либо исчезнет.

## Req 3 — `/system` участники: заявки вверху + connect/deny

### Backend

- `GET /api/v1/system/tenants/{id}/members` уже отдаёт все статусы (`status` в ответе) — не
  меняется.
- Новый `(*service.ProvisioningService) DenyMember(ctx, tenantID, userID int64) error`
  = `memberships.DeleteRequested(scope, userID)` + `membershipCache.InvalidateUser(userID)`.
- Новый роут `POST /api/v1/system/tenants/{id}/members/{userID}/deny` (под `RequireSystemAdmin`)
  → `ProvisioningService.DenyMember` → `204`.
- «Подключить» для заявки = существующий `POST /api/v1/system/tenants/{id}/members`
  (`AttachMember` делает `Upsert` со `status=active` — для `requested` это перевод в активные).

### Frontend (`system.js` `MembersSection`)

- Список участников сортируется: `status === 'requested'` сверху, затем активные.
- У строки-заявки — кнопки «Подключить» (`POST .../members {user_id, role}` с ролью из заявки) и
  «Отклонить» (`POST .../members/{userID}/deny`). После успеха — рефетч участников.

## Обработка ошибок

- Все мутации возвращают `204` при успехе; `4xx/5xx` → инлайн-сообщение/alert в секции; список
  ревалидируется только при успехе.
- system deny не-system-admin → `403` (общий гейт).

## Тестирование

- **Backend (Go, testcontainers/httptest):**
  - `UserRepository.ListByTenant` — возвращает активных и запросивших активного тенанта со
    `Status`/`Role`; пользователь, не связанный с тенантом, не попадает; изоляция между тенантами.
  - `HandleListUsers` — отдаёт только tenant-scoped, каждый элемент со `Status`; глобальные
    не-члены отсутствуют; активный член с грантом имеет `GrantedNodeCount > 0`, запросивший — `0`.
  - `ProvisioningService.DenyMember` — удаляет `requested`, не трогает `active`.
  - system deny endpoint — happy-path (`204`, заявка исчезает) + гейт (не-admin → `403`).
- **Frontend:** ручная проверка по DoD (`010`): в `/admin` виден только tenant-scope, заявки с
  кнопками add/deny работают; в `/system` заявки вверху, connect/deny работают.

## Вне scope

- Per-tenant роль vs legacy `users.is_admin` toggle (оставляем как есть).
- Удаление/отвязка **активных** участников из UI; переименование тенанта.
- Серверный поиск/пагинация списков.
