# Доступы в тенант и управление членством

Дата: 2026-07-02
Ветка: `defuault-accesses-for-new-tenant-users`

Четыре связанных улучшения онбординга в тенант и администрирования членства. Source-of-truth
specs, которые нужно обновить в том же change set: `specs/040-api-contract.md`,
`specs/050-permissions-and-lifecycle.md` (и `specs/030-user-flows.md` там, где flow настроек
затрагивает онбординг).

## Затрагиваемые слои (clean architecture)

- domain — новых сущностей нет; переиспользуем `Membership`, `Role`, `Tenant`, `HierarchyGrant`.
- store — небольшие добавления: join-запрос membership по пользователю; счётчик system-admin.
- service — переиспользование применения default-политики при аппруве; guard'ы last-admin / last-system-admin.
- http handlers — новые эндпоинты system + session.
- web (React SPA) — на `/system` добавляется вкладка «Пользователи» + контрол роли участника; на `/settings` — раздел с тенантами.

---

## Задача 1 — Применять доступы по-умолчанию при аппруве самостоятельной заявки

### Проблема

`OnboardingService.applyNewUserPolicy` (выдаёт грант на `default_hierarchy_node_id`, когда
`new_user_policy = "default_node"`, только если у пользователя ещё нет гранта в тенанте) вызывается
в `ClaimInvitation` и `EnsureRegistration`, но **не** при аппруве join-request. Пользователь,
который сам запросил доступ по slug и был подтверждён, не получает базового доступа.

### Изменение

Вызывать существующий шаг `applyNewUserPolicy` на обоих путях аппрува, сохраняя его текущий guard
(no-op, если у пользователя уже есть любой грант в тенанте — ручной доступ никогда не перезаписывается):

- `OnboardingService.ApproveRequest` (аппрув tenant-admin через `/admin`): после `SetStatus(active)` +
  инвалидации кэша вызвать `applyNewUserPolicy(ctx, scope, userID)`.
- Кнопка «Подключить» у system-admin (`POST /api/v1/system/tenants/{id}/members`, `HandleAttachMember`):
  после того как membership стал активным, применить тот же шаг политики. Provisioning/system-сервис
  переиспользует логику политики из `OnboardingService` (общая зависимость), а не дублирует её —
  правило выдачи гранта живёт ровно в одном месте.

Новых эндпоинтов нет. Не завязано на lifecycle (администрирование членства, независимо от статуса
периода команды).

### Тесты

- аппрув (tenant-admin) с политикой `default_node` + заданным узлом + у пользователя нет грантов → грант создаётся.
- аппрув с политикой `empty` → грант не выдаётся.
- аппрув, когда у пользователя уже есть грант → без изменений (guard идемпотентности держится).
- attach system-admin'ом `requested`/нового участника → та же самая выдача политики.

---

## Задача 2 — Смена роли участника из /system

### Эндпоинт

`PUT /api/v1/system/tenants/{id}/members/{userID}/role`

- Gate: `RequireSystemAdmin`.
- Body: `{"role": "user" | "admin"}`.
- Валидация: роль должна быть `user` или `admin` → иначе `422 VALIDATION_ERROR`.
- Успех: `204`.
- Ошибки:
  - `404 NOT_FOUND` — нет membership для `(tenant, user)`.
  - `409 CONFLICT` — понижение `admin → user`, когда это **последний** админ тенанта
    (в тенанте должен оставаться ≥1 админ).
- Идемпотентность: установка текущей роли повторно → `204`, no-op.
- Side effects: инвалидация membership-кэша пользователя (роль влияет на scope доступа).

Опирается на существующий `OnboardingService.SetMemberRole → memberships.SetRole`. Для guard'а
last-admin нужен счётчик админов тенанта (добавить `memberships.CountAdmins(scope)` или аналог).

### UI

Вкладка Members на `/system`: контрол роли (dropdown user/admin) в каждой строке участника →
вызывает эндпоинт, перезагружает список участников.

### Тесты

- смена роли admin→user сохраняется; membership-кэш инвалидируется.
- понижение последнего админа → `409`.
- неизвестный membership → `404`.
- невалидная строка роли → `422`.

---

## Задача 3 — Выдача system-привилегий из /system

### Эндпоинт

`PUT /api/v1/system/users/{userID}/system-admin`

- Gate: `RequireSystemAdmin`.
- Body: `{"is_system_admin": true | false}`.
- Успех: `204`.
- Ошибки:
  - `404 NOT_FOUND` — пользователь не существует.
  - `409 CONFLICT` — снятие (`false`) с **последнего** system-admin в инстансе.
  - `403` / `409` — вызывающий целится в **собственный** аккаунт (защита от self-lockout).
- Идемпотентность: установка текущего значения повторно → `204`, no-op.
- Side effects: только обновление `users.is_system_admin`.

Опирается на существующий `users.SetSystemAdmin`. Для guard'а last-admin нужен счётчик system-admin
(добавить `users.CountSystemAdmins()`; булев `AnySystemAdmin()` недостаточен).

### UI

На `/system` добавляется новая вкладка **«Пользователи»**, показывающая
`GET /api/v1/system/users` (уже возвращает `is_system_admin`, `display_name`, `email`, `provider`)
с per-row тумблером system-admin → вызывает эндпоинт, перезагружает список. В строке собственного
аккаунта тумблер показан задизейбленным (guard self-lockout продублирован на клиенте; сервер
enforce'ит в любом случае).

### Тесты

- тумблер вкл/выкл сохраняется.
- снятие с последнего system-admin → `409`.
- вызывающий переключает собственный аккаунт → заблокировано.
- неизвестный пользователь → `404`.

---

## Задача 4 — Раздел тенантов в /settings

### Эндпоинты

**Список своих membership** — `GET /api/v1/session/memberships`

- Gate: авторизован (не membership-gated — у пользователя могут быть только заявки).
- Ответ: `[{ "tenant_id": 1, "slug": "acme", "name": "Acme", "role": "user", "status": "active" }]`
  — **все статусы** (active + requested), отсортировано по имени.
- Реализуется одним join-запросом `memberships ⋈ tenants` (без N+1; в отличие от существующего
  `ListMyTenants`, который в цикле дёргает `GetByID`). Держится **отдельно** от
  `/api/v1/session/tenants`, который остаётся active-only для tenant switcher.
- Идемпотентность: read-only.

**Выход / отмена заявки** — `DELETE /api/v1/session/memberships/{tenantID}`

- Gate: авторизован.
- Удаляет **собственный** membership вызывающего в `{tenantID}` (любого статуса) + его hierarchy-гранты
  в этом тенанте, через существующий `OnboardingService.RemoveMember`.
- Одним эндпоинтом покрывает и «выход» (active), и «отмену заявки» (requested).
- Успех: `204`; идемпотентно (не-член → `204`).
- Ошибки: `409 CONFLICT` — вызывающий является **последним админом** тенанта (иначе тенант осиротеет).
- Side effects: инвалидация membership-кэша + grants-кэша; если покинутый тенант был активным в
  сессии, следующий запрос ре-резолвит тенант/редиректит на `/no-access`, как и сейчас.

**Заявка по slug** — переиспользуем существующий `POST /api/v1/onboarding/join-request`
`{"slug": "..."}` (`204`; `404` неизвестный slug; `409` уже активный член). Новый эндпоинт не нужен.

### UI

Новый раздел **«Мои пространства»** в `internal/web/static/settings.js`:

- На загрузке фетчит `GET /api/v1/session/memberships`.
- Показывает каждый membership: имя, slug, роль, badge статуса (Активен / Заявка отправлена).
- Активная строка → кнопка «Выйти» (`DELETE …/memberships/{tenantID}`).
- Строка-заявка → кнопка «Отменить заявку» (тот же DELETE).
- Форма заявки по slug (input + «Отправить заявку») → `POST /api/v1/onboarding/join-request`.
- Перефетч после каждой мутации. DELETE + POST несут CSRF-токен (при необходимости расширить
  api-хелпер в settings.js методом DELETE).
- Показывать conflict-ошибки инлайн (напр. «уже участник», «последний админ не может выйти»).

### Тесты

- список membership возвращает active + requested со slug/name/role/status.
- выход удаляет membership + гранты; отмена удаляет requested-membership.
- выход последнего админа → `409`.
- заявка по slug создаёт `requested`; неизвестный slug → `404`; уже член → `409`.

---

## Обновления спек (в том же change set)

- `specs/040-api-contract.md`
  - System plane: добавить `PUT …/members/{userID}/role`, `PUT /api/v1/system/users/{userID}/system-admin`.
  - Session/onboarding: добавить `GET /api/v1/session/memberships`,
    `DELETE /api/v1/session/memberships/{tenantID}`; отметить переиспользование join-request для settings.
  - Полный контракт по чеклисту спеки «Требования к новым endpoint'ам».
- `specs/050-permissions-and-lifecycle.md`
  - Онбординг: default `new_user_policy` теперь применяется при **аппруве** (обе admin-плоскости),
    а не только при авторегистрации / инвайте.
  - Плоскость system-admin: может менять роли участников тенанта и выдавать/снимать `is_system_admin`.
  - Guardrails: в тенанте остаётся ≥1 админ (смена роли + выход); в инстансе остаётся ≥1 system-admin;
    нет self-lockout при снятии system-admin.
- `specs/030-user-flows.md` — расширить flow членства/онбординга разделом тенантов в /settings
  (выход / отмена / заявка), если в спеке есть соответствующая секция flow; иначе не трогать.

## Non-goals / YAGNI

- Нет публичного каталога тенантов (для заявки по-прежнему нужно знать slug).
- Нет email/SMTP-уведомлений при аппруве или смене роли.
- Нет гранулярных целевых ролей (`viewer`/`editor`/`validator`) — остаются future work в спеке 050.
- Нет связи с lifecycle/статусом периода (всё администрирование членства не зависит от статуса).
