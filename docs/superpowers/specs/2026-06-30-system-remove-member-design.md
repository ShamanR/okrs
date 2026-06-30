# Remove member from tenant (`/system`) — design

Статус: проектная спека. Добавляет в system-admin плоскость возможность **удалить участника из
тенанта** — операцию, ранее явно отложенную («удаление активных участников — вне scope»). Дополняет
connect/deny из предыдущего шага.

## Контекст и цель

В `/system` → участники сейчас можно подключить пользователя (`AttachMember`), отклонить заявку
(`DenyMember`, только `requested`), но нельзя **убрать активного члена** из тенанта. Цель —
кнопка «Удалить» на строках активных участников + бэкенд, полностью разрывающий доступ участника
к тенанту.

## Семантика (принятое решение)

**Удаление участника = удалить его membership в тенанте (любого статуса) И все его hierarchy
grants в этом тенанте.** Полный разрыв доступа; при повторном вступлении старые гранты не
«воскресают». Гранты в других тенантах и membership'ы в других тенантах не затрагиваются (всё
tenant-scoped по `tenant_id`).

Отличие от `deny` (предыдущий шаг): `deny` удаляет только `requested`-membership (у заявки грантов
нет) и служит UX-кнопкой «Отклонить» для заявок. `RemoveMember` удаляет membership любого статуса
+ гранты и служит кнопкой «Удалить» для активных членов. Оба сосуществуют.

Guardrail'ов нет (можно удалить последнего админа / себя): `/system` — это плоскость оператора
инстанса (system-admin/provisioning-token), ответственность на операторе. Восстановление —
повторное подключение через ту же панель.

## Архитектура

### Backend

- `(*memberships.MembershipRepository) Delete(ctx, scope domain.TenantScope, userID int64) error`
  — удаляет membership `(user, tenant)` любого статуса; no-op, если строки нет.
- `(*grants.GrantRepository) RemoveAllUserGrants(ctx, scope domain.TenantScope, userID int64) error`
  — `DELETE FROM user_hierarchy_grants WHERE user_id = $1 AND tenant_id = $2`. Зеркало на
  `(*grants.GrantsCache) RemoveAllUserGrants` (write-through + инвалидация кэша грантов тенанта),
  по образцу существующего `RemoveUserGrant`.
- `(*service.ProvisioningService) RemoveMember(ctx, tenantID, userID int64) error` — вызывает
  `grants.RemoveAllUserGrants(scope, userID)` затем `memberships.Delete(scope, userID)`;
  инвалидирует membership-кэш (`memberCache.InvalidateUser`). ProvisioningService получает новую
  зависимость — grants-remover (интерфейс с `RemoveAllUserGrants`; `*grants.GrantsCache`
  удовлетворяет). Конструктор `NewProvisioningService` расширяется этим аргументом; обновляются
  все вызовы (`server.go`, тесты).
- HTTP: `DELETE /api/v1/system/tenants/{id}/members/{userID}` → `RequireSystemAdmin` →
  `Provisioner.RemoveMember` → `204`. `Provisioner` интерфейс system-хендлера дополняется
  `RemoveMember`.

### Frontend (`system.js` `MembersSection`)

- На строках **активных** участников — кнопка «Удалить» (`DELETE .../members/{user_id}`), после
  успеха — рефетч участников. Запросившие (`requested`) сохраняют «Подключить»/«Отклонить»
  (без «Удалить»). Добавляется `del`-хелпер (fetch `DELETE` + `csrfHeaders`).
- Лёгкое подтверждение перед удалением (`confirm(...)`), чтобы случайный клик не убирал участника.

## Обработка ошибок

- `RemoveMember` идемпотентен: нет membership → grants-delete и membership-delete как no-op → `204`.
- Не-system-admin → `403` (общий гейт). Невалидный id → `400`.

## Тестирование

- **Backend (Go, testcontainers/httptest):**
  - `memberships.Delete` — удаляет active/requested; изоляция по тенанту (membership того же
    пользователя в другом тенанте не трогается).
  - `grants.RemoveAllUserGrants` — удаляет все гранты пользователя в тенанте; гранты в другом
    тенанте/у другого пользователя целы.
  - `ProvisioningService.RemoveMember` — после вызова `Memberships.Get` → `ErrNotFound` и
    `Grants.ListUserGrants(scope, user)` пуст; membership в другом тенанте не затронут.
  - system DELETE endpoint — happy-path (`204`, участник исчезает из `GET members`) + route
    wiring; гейт (не-admin → `403`) уже покрыт.
- **Frontend:** ручная проверка по DoD (`010`): «Удалить» убирает активного члена из списка; его
  гранты в тенанте сняты (после повторного подключения доступов нет).

## Вне scope

- Guardrail'ы (последний админ / самоудаление).
- Удаление участника из tenant-admin плоскости (`/admin`) — отдельный запрос, если понадобится.
- Soft-delete / аудит удалений.
