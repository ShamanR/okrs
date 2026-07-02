# Invite links (generic, reusable) — design

Статус: проектная спека. Спек B из связки кастомизации. Делает пригласительные ссылки в тенант:
generic (без email), одноразовые или многоразовые (с лимитом или безлимитные), создаются
tenant-admin'ом в `/admin?section=users`. Авторизованный пользователь, открывший ссылку, сразу
добавляется в тенант.

## Контекст

Из Tenant Foundation Plan 4 уже есть: таблица `tenant_invitations` (email NOT NULL, single-use:
`MarkClaimed` гасит статус), `OnboardingService.ClaimInvitation` (GetPendingByTokenHash →
MarkClaimed → active membership), web-флоу `/invite/{token}` (cookie → логин → claim в
OAuth-callback) и `POST /api/v1/admin/invitations {email, role}`. Claim привязывается к
идентичности (`provider:subject`), а не к email — email был лишь меткой.

Нужно: убрать привязку к email, добавить многоразовость (лимит `max_uses`), UI создания/отзыва в
`/admin?section=users`, и сделать так, чтобы **уже авторизованный** пользователь по ссылке
добавлялся сразу.

## Модель / схема (миграция 034)

`tenant_invitations`:

- `email` → **NULLABLE** (generic-ссылки не привязаны к email; новые ссылки пишут `NULL`).
- + `max_uses INT NULL` — `NULL` = безлимит, `1` = одноразовая, `N` = до N использований.
- + `use_count INT NOT NULL DEFAULT 0`.
- Backfill (down тоже): существующим строкам проставить `max_uses = 1` (сохраняют одноразовость).
- `expires_at` (уже есть) — опциональный срок.

`down`: убрать `max_uses`/`use_count`, вернуть `email` в NOT NULL (с дефолтом `''` для NULL-строк,
чтобы down не падал).

## Атомарный claim (репозиторий)

- `Create(ctx, scope, role domain.Role, tokenHash string, createdBy int64, maxUses *int, expiresAt *time.Time) (*domain.Invitation, error)`
  — **без email** (пишет `NULL`); хранит `max_uses`.
- `Consume(ctx, tokenHash string) (*ClaimResult, error)` — один атомарный `UPDATE`:

  ```sql
  UPDATE tenant_invitations
  SET use_count = use_count + 1,
      status = CASE WHEN max_uses IS NOT NULL AND use_count + 1 >= max_uses THEN 'claimed' ELSE status END
  WHERE token_hash = $1 AND status = 'pending'
    AND (expires_at IS NULL OR expires_at > now())
    AND (max_uses IS NULL OR use_count < max_uses)
  RETURNING tenant_id, role
  ```

  Вернулась строка → `ClaimResult{TenantID, Role}` + ok; иначе `ErrNotFound` (невалидно/исчерпано/
  истекло/отозвано). Конкурентно-безопасно (атомарный инкремент под `use_count < max_uses`).
  Заменяет пару `GetPendingByTokenHash`+`MarkClaimed` в claim-пути.
- `Revoke(ctx, scope, id int64) error` → `status='revoked'` (scoped по тенанту).
- `ListPendingByTenant` — в ответ добавляются `max_uses`, `use_count`.

`domain.Invitation` дополняется `MaxUses *int`, `UseCount int`; `Email *string` (nullable).

## Сервис / claim-флоу

- `OnboardingService.ClaimInvitation(ctx, rawToken, userID)` → `Consume(HashInviteToken(raw))`;
  при ok → `Upsert` active membership (уже-член → no-op, идемпотентно) с ролью из `ClaimResult`;
  иначе `ErrInvalidInvitation`.
- **Уже авторизованный по ссылке (закрываем пробел).** `authhandler.HandleInvite` (`/invite/{token}`)
  меняется: если в контексте есть пользователь (есть сессия) — claim **сразу**
  (`onboard.ClaimInvitation`), при успехе `sessions.SetActiveTenant` на тенант инвайта и редирект в
  приложение (`/`); если пользователя нет — текущее поведение: cookie `okrs_invite` + редирект на
  логин, claim произойдёт в OAuth-callback (`onboardAfterLogin`). Невалидный токен у
  авторизованного → редирект на `/` без ошибки (или `/no-access`); UX не блокируется.

## API (`/admin`, tenant-admin)

- `POST /api/v1/admin/invitations` — body `{role, max_uses?, expires_at?}` (**email больше не
  принимается**) → `201 {token, url, role, max_uses}`. `max_uses` отсутствует/`null` → безлимит;
  `1` → одноразовая. Роль `user`/`admin` (дефолт `user`). `url = baseURL + "/invite/" + token`.
- `POST /api/v1/admin/invitations/{id}/revoke` → `204` (tenant-scoped).
- `GET /api/v1/admin/invitations` — список pending ссылок тенанта с `id, role, status, max_uses,
  use_count, created_at, expires_at` (без email).

## UI — `/admin?section=users`

Панель «Пригласительные ссылки» (в секции users): форма — роль (`user`/`admin`); тип:
«одноразовая» / «многоразовая (без лимита)» / «до N» (число → `max_uses`); опц. срок. По «Создать»
показывается готовый URL с кнопкой «Копировать». Ниже — список активных ссылок (роль,
`использовано x/N` или `x/∞`, срок) с кнопкой «Отозвать». `MarkdownEditor`/markdown не нужны.

## Обработка ошибок

- Невалидный/исчерпанный/истёкший/отозванный токен: claim → `ErrInvalidInvitation`; web — мягкий
  редирект, без падения.
- Создание: некорректный `max_uses` (≤0, кроме null) → `400`. Revoke чужого/несуществующего →
  идемпотентно `204`/`404` (выберем `204` no-op при 0 rows — согласуется с другими deny/no-op).
- Гейты: create/revoke — `RequireTenantAdmin`; `/invite` — авторизованный или ведёт на логин.

## Тесты

- **Repo:** `Consume` — одноразовая валидна один раз, потом нет; `max_uses=N` — валидна N раз;
  безлимит (`NULL`) — многократно; истёкшая/отозванная/неизвестный токен → `ErrNotFound`;
  двойной `Consume` одноразовой подряд → ровно один успех. `Revoke` — статус, изоляция по тенанту.
  Backfill миграции — старые строки получают `max_uses=1`.
- **Service:** `ClaimInvitation` по multi-use дважды разными userID → оба получают active
  membership; повтор тем же userID → no-op (idempotent Upsert); исчерпанная → `ErrInvalidInvitation`.
- **Handler:** create (`max_uses` null/1/N, без email) → `201` + url; revoke → `204`; gate
  (не-tenant-admin → `403`). `/invite/{token}` авторизованным → сразу active membership (через
  `onboardAfterLogin`-эквивалент в HandleInvite).
- **Frontend:** ручная проверка по DoD (`010`): создать одноразовую/многоразовую ссылку, скопировать,
  открыть авторизованным → добавлен; одноразовая повторно → отказ; отзыв убирает из списка.

## Вне scope

- Доставка приглашений письмом (SMTP); брендинг/кастом-домены ссылок; аналитика переходов;
  приглашение конкретного email с матчингом (claim и так по идентичности).
