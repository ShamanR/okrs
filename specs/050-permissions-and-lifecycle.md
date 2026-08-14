# Permissions and Lifecycle

> **`Period.status` — не то же самое, что статус ниже.** У самого периода (сущность `Period`,
> см. `020-domain-model.md`) есть отдельное вычисляемое поле `status` (`future | active | closed |
> archived`): `future`/`active`/`closed` выводятся из дат периода на каждое чтение, а `archived` —
> ручной admin-переход, доступный только когда текущий (date-based) статус — `closed`
> (`POST /api/v1/admin/periods/{id}/archive`, иначе `409`), отменяемый через `unarchive`. Это
> жизненный цикл периода как такового — не команды. Весь остальной раздел ниже (`no_goals`,
> `forming`, `ready`, `in_progress`, `validated`, `closed`) описывает **`TeamPeriodStatus`** —
> отдельную сущность (прогресс конкретной команды по целям внутри периода), которая архивацией
> периода и nested-periods не затрагивается и не менялась.

## Текущее состояние

В проекте уже есть доменный статус периода команды:

- `no_goals`
- `forming`
- `ready`
- `in_progress`
- `validated`
- `closed`

Значение `ready` («К валидации») добавлено миграцией `018_status_ready`. В UI-стиппере трекера отображаются только четыре явных шага: `forming → ready → in_progress → closed`; состояние `no_goals` показывается как отдельный badge. Значение `validated` остаётся допустимым в домене и БД, но в UI-стиппере не отображается.

Статус:

- хранится в БД;
- читается и отображается в UI;
- может обновляться через API;
- валидируется по допустимому набору значений.

## Что реализовано сейчас

На текущий момент backend гарантирует следующее:

- статус должен быть одним из допустимых значений;
- статус можно сохранить для пары `(team_id, period_id)`;
- **аутентификация** через OAuth2/OIDC провайдеры (Google, GitHub, Keycloak) или режим без авторизации;
- **сессии** хранятся на сервере (PostgreSQL), клиент получает только session ID в cookie;
- **роли**: per-tenant `user`/`admin` (`memberships.role`) — `/admin` гейтится активной ролью тенанта; плюс инстанс-уровневый `users.is_system_admin` для `/system`;
- **scope доступа**: пользователь видит только команды, к которым ему выданы hierarchy grants и их потомков (рекурсивная CTE); scope вычисляется `PolicyEvaluator` на каждый запрос;
- **no-auth mode**: при `AUTH_MODE=disabled` все маршруты доступны, операции выполняются от имени `anonymous-local` с активной ролью тенанта `admin`.

На текущий момент **не реализованы как строгие серверные гарантии**:

- правила переходов между статусами;
- блокировка structural edits goal / KR в `validated` или `closed`.

## Актуальная interpretation lifecycle

На данный момент lifecycle следует понимать так:

### `no_goals`

Техническое состояние периода без оформленных целей. **Автоматически выставляется сервером** при удалении последней goal команды в периоде (если текущий статус не `no_goals`). Это серверная гарантия.

### `forming`

Период находится в процессе заполнения и редактирования. Доступен полный CRUD goal и KR.

### `ready`

Цели сформированы и переданы на валидацию («К валидации»). Полный CRUD goal и KR по-прежнему доступен — серверных ограничений нет.

### `in_progress`

Период активен, прогресс обновляется. Структурные правки (create/update/delete goal и KR) UI не предлагает; backend ограничений пока нет.

### `validated`

Статус существует в домене и БД, но в UI-стиппере не отображается и сейчас не используется в новом трекере. Server-side ограничений пока нет.

### `closed`

Статус существует в домене и UI, но пока не делает период гарантированно read-only на уровне API.

## Текущее ограничение

Lifecycle (team period status) — это в первую очередь:

- доменное поле;
- UI-сигнал (кнопка «Добавить цель» скрыта при `validated`/`closed`);
- организационная договорённость.

Lifecycle ещё не является полноценной policy enforcement model на сервере — structural edits не блокируются API при `validated`/`closed`.

## Массовые переходы team period status (admin)

Массовые операции над статусами команд периода доступны только в admin-плоскости (эндпоинты под `/api/v1/admin`, см. `040-api-contract.md`). Правила:

- затрагиваются только команды, у которых есть ≥1 цель в этом периоде; команды без целей пропускаются (`skipped`);
- операция идемпотентна: команда, уже находящаяся в целевом статусе, не затрагивается и не пишется в журнал (не входит ни в `affected`, ни в `skipped`);
- каждая реально изменённая команда порождает отдельную запись в журнале активности (`status` / `status_changed`, payload `before`/`after`, флаг `bulk`);
- `activate` переводит команды в `in_progress` (цели блокируются от структурного редактирования, остаётся обновление прогресса); `close` — в `closed` (режим «только комментарии»).

## Обзор периода — охват (my_teams / org)

`GET /api/v1/periods/{periodID}/overview?scope=` доступен любому аутентифицированному участнику и ограничивает обзор по охвату:

- `my_teams` (по умолчанию) — команды, где участник назначен руководителем (`teams.lead_udid` = его UDID), плюс все вложенные потомки (рекурсивно по `parent_id`). Доступен всем участникам. Если у участника нет своих команд — охват пуст.
- `org` — весь тенант; **только tenant-admin** (иначе `403`). В UI кнопка «Вся организация» показывается только админам.

Охват влияет на все секции обзора одинаково: статусы, качество, балансы целей и график прогресса. Легаси admin-only `GET /api/v1/admin/periods/{periodID}/overview` (весь тенант) сохранён для админ-модалки.

Массовые операции над статусами команд периода тоже scope-aware (`POST /api/v1/periods/{periodID}/teams/{activate|close}?scope=`): в охвате `my_teams` руководитель применяет их к своим командам (и вложенным), в охвате `org` — только tenant-admin ко всему тенанту. Область действия операции = разрешённый охват вызывающего (те же проверки, что у обзора). Кнопки управления периодом видны в обзоре для `my_teams` всегда, для `org` — только админу. Легаси admin-only `POST /api/v1/admin/periods/{periodID}/teams/{activate|close}` (весь тенант) сохранён для админ-модалки.

## Текущие роли и права

### Плоскости администрирования

Три раздельные плоскости управления, каждая со своим гейтом:

1. **System admin** (`users.is_system_admin`) — `/system` + `/api/v1/system/*`. Над тенантами:
   создание тенантов, **смена названия и slug пространства** (`PATCH …/tenants/{id}`),
   прямое назначение membership, **смена роли участника**
   (`PUT …/members/{id}/role`), запись `entitlement.*`, suspend/restore, глобальный список
   пользователей, **выдача/снятие system-привилегий** другим пользователям
   (`PUT /api/v1/system/users/{id}/system-admin`), `default_registration_tenant_id`.
   Это не роль внутри тенанта. Гейт `RequireSystemAdmin` пропускает либо сессию
   system-admin, либо машинный вызов с `Authorization: Bearer <PROVISIONING_TOKEN>`.
   Bootstrap первого system-admin — env `BOOTSTRAP_SYSTEM_ADMIN` (provider:subject или email),
   повышается при первом совпавшем логине, пока ни одного system-admin ещё нет; после bootstrap
   привилегию можно выдавать/снимать через API (см. выше).
   **Guardrails:** тенант всегда сохраняет ≥1 активного админа (проверяется при смене роли и при
   выходе пользователя из тенанта → `409 ErrLastAdmin`); инстанс всегда сохраняет ≥1 system-admin
   (`409` при снятии последнего); system-admin не может снять привилегию с собственного аккаунта
   (`409`, защита от self-lockout).
2. **Tenant admin** (`memberships.role = admin` в активном тенанте) — существующий `/admin` +
   `/api/v1/admin/*`, теперь tenant-scoped. Гейт `RequireTenantAdmin` проверяет активную роль
   из контекста (её ставит `TenantResolve`). Внутри своего тенанта:
   команды, периоды (создание/редактирование/удаление, а также **архивирование и
   разархивирование** периода — архивировать можно только период в статусе `closed`, иначе `409`,
   см. `020-domain-model.md` и `040-api-contract.md`), пользователи/гранты, продуктовые ключи
   `tenant_settings`, **переименование своего пространства** (`name`) в общих настройках
   (`POST /api/v1/admin/settings/general`; slug менять нельзя). **Не** может писать `entitlement.*`.
3. **User** (любой авторизованный) — `/settings`, личные `user_settings` + список своих memberships.

### Роли тенанта

- `user` — обычный член тенанта;
- `admin` — tenant-admin (`memberships.role = admin`).

> Легаси-флаг `is_admin` на пользователе удалён (миграция `038_drop_users_is_admin`): суперадмин
> инстанса — `is_system_admin` (плоскость 1), «админ организации» — `memberships.role = admin`
> (плоскость 2). Исторический backfill: `028` перенёс суперадминов в `is_system_admin`, `035` —
> tenant-админов в `memberships.role`.

### Write-authority настроек (проверяется в service-слое)

- `tenant_settings` ключи без префикса `entitlement.` — продуктовые, пишет tenant-admin
  (`SetTenantProduct`); попытка записать `entitlement.*` этим путём отклоняется.
- `tenant_settings` ключи `entitlement.*` — пишет только system-admin/provisioning
  (`SetTenantEntitlement`).
- `system_settings` — глобальные ключи инстанса, пишет только system-admin.

#### Настройки сбора обратной связи

Хранятся в `tenant_settings` активного тенанта (per-tenant; с миграции 033, ранее — в глобальном `system_settings`), читаются tenant-admin через `GET/POST /api/v1/admin/settings/feedback` и любым авторизованным пользователем (для его тенанта) через `GET /api/v1/config`:

- `feedback_url` (string, по умолчанию `""`) — ссылка на внешний опрос;
- `feedback_popup_enabled` (bool, по умолчанию `false`) — показывать всплывающее окно;
- `feedback_menu_link_enabled` (bool, по умолчанию `false`) — показывать пункт меню;
- `feedback_frequency_days` (int `>= 1`, по умолчанию `30`) — интервал охлаждения показа окна.

Логику показа окна и cookie-трекинг см. в `030-user-flows.md` (`3д`).

### Права user

- просмотр OKR в пределах scope (hierarchy grants);
- CRUD goal / KR / progress в доступных командах;
- комментарии к целям: создание тасок (замечаний) и ответов на таски; резолв/reopen тасок; удаление **своих** тасок и ответов (tenant-admin может удалить любые). Удаление таски каскадно удаляет её ответы. Ответ таской не является и не резолвится;
- обновление team period status.

Создание ответов и удаление тасок/ответов **не зависят от team period status** — комментирование разрешено во всех статусах (в `in_progress` и `closed` UI работает в режиме «только комментарии»), поэтому там же доступны ответы и удаление своих комментариев. Все проверки (доступ к команде, привязка к цели, авторство/роль для удаления) выполняются на сервере, не только в UI.

## Журнал активности (Activity Log)

**Доступ — только tenant-admin.** Раздел `/activity-log` и его API
(`GET /api/v1/activity`, `/activity/tree-counts`, `/activity/category-counts`) закрыты гейтом
`RequireTenantAdmin` (`memberships.role = admin`) — тем же, что и очистка журнала. Для обычных
пользователей раздел скрыт в навигации сайдбара (`is_admin` из `GET /api/v1/config`), а страница
`/activity-log` и все три метода отвечают `403` (fail-closed на сервере, не только в UI). При
`AUTH_MODE=disabled` `anonymous-local` имеет активную роль тенанта `admin`, поэтому доступ открыт.

**Видимость внутри тенанта.** Админ видит **все** события своего тенанта (share-aware
audience-фильтрация по `PolicyEvaluator` в коде сохраняется, но для admin-scope охватывает все
команды тенанта, поэтому фактически не ограничивает ленту):

- разделение по тенантам — все запросы фильтруются по `tenant_id` из scope;
- **fail-closed**: событие с `team_id IS NULL` (напр. команда hard-deleted) в ленту и в счётчики
  дерева не попадает;
- **бывший участник**: если actor больше не активный член тенанта, он резолвится в нейтральный
  плейсхолдер (`removed=true`, «Бывший участник») без email/аватара/UDID; резолв не бросает
  ошибку при отсутствии пользователя (edge-case «убрали из пространства» не роняет процесс).

**Очистка журнала.** Ретеншн ручной:

- tenant-admin (`memberships.role = admin`) может очистить журнал **своего** пространства —
  `POST /api/v1/admin/activity/purge` (гейт `RequireTenantAdminMiddleware`);
- system-admin может очистить журнал **любого** тенанта как элемент управления пространством —
  `POST /api/v1/system/tenants/{id}/activity/purge` (гейт `RequireSystemAdminMiddleware`);
- глубина: старше квартала / старше года / всё; операция необратима, всегда tenant-scoped
  (чужой тенант задеть нельзя).

## Онбординг и членство

Активным считается только membership со `status=active`; `RequireMembership` пропускает
только его (`requested` — нет). Авторизованный пользователь без активного membership
редиректится на `/no-access` (вне membership-gated группы, чтобы не было петли); страницу
рендерит подключаемый `NoMembershipHandler` (OSS-дефолт «stub» — заглушка + форма
join-request).

Три примитива (детали — `040-api-contract.md`):

1. **Новый пользователь.** После первого OAuth-логина (callback) без активного membership:
   если задан глобальный `default_registration_tenant_id` → автосоздаётся `active`
   membership (`role=user`) в этом тенанте и применяется его `new_user_policy`; иначе →
   `/no-access`. Логику выполняет `OnboardingService.EnsureRegistration` из callback'а
   (перенесена из `auth.Manager`, чтобы таргетить резолвнутый тенант, а не хардкод #1).
2. **Приглашение (tenant-admin).** Админ создаёт `tenant_invitations` с одноразовым токеном;
   приглашённый открывает `/invite/{token}` → токен в cookie → логинится любым OAuth →
   callback гасит токен (атомарно, single-use) и привязывает `active` membership к **текущей
   идентичности** (`provider:subject`). **Безопасность:** claim только по валидному токену;
   email-match доступа не даёт; повтор/истёкший/чужой токен — отказ; один email через двух
   провайдеров = две независимые учётки.
3. **Запрос доступа (user).** С `/no-access` (или из раздела «Мои пространства» на `/settings`)
   пользователь вводит slug → membership `status=requested` → очередь в `/admin`
   (`access-requests`) → approve (`active`) / deny (удаление). Публичного каталога тенантов нет.
   При **approve** применяется `new_user_policy` тенанта (default-node grant), если у пользователя
   ещё нет ни одного гранта в этом тенанте — тот же baseline, что при авторегистрации и инвайте.
   Это же применение выполняет system-admin при «Подключить» (`POST …/members`).

Раздел «Мои пространства» на `/settings` (любой авторизованный) даёт пользователю: список своих
пространств всех статусов (`GET /api/v1/session/memberships`), выход / отмену заявки
(`DELETE /api/v1/session/memberships/{tenantID}` — удаляет собственный membership и гранты; `409`,
если он последний активный админ тенанта) и отправку заявки по slug (переиспользует
`POST /api/v1/onboarding/join-request`).

## Target state

Целевое состояние для будущих итераций:

### Granular roles

- `viewer`
- `editor`
- `validator`
- `admin`

### Scope

- global
- subtree of team hierarchy
- single team

### Целевые права

- `viewer`: только просмотр;
- `editor`: CRUD goal / KR / comment / progress в доступных командах;
- `validator`: перевод периода в `validated`;
- `admin`: periods, teams, permissions, reopen closed period.

## Target lifecycle transitions

Целевая модель переходов:

- `no_goals -> forming`
- `forming -> in_progress`
- `in_progress -> validated`
- `validated -> closed`

Исключения:

- `validated -> in_progress` только для `admin`;
- `closed -> in_progress` только для `admin` с обязательным audit reason.

## Target lifecycle restrictions

### Целевые права для `forming` / `in_progress`

Разрешены:

- create / update / delete goal;
- create / update / delete KR;
- reorder;
- share goal;
- comments;
- progress update.

### Целевые права для `validated`

Разрешены:

- comments;
- progress update;
- reorder, только если это отдельно подтверждено продуктовым решением.

Запрещены:

- structural edits goal / KR.

### Целевые права для `closed`

По умолчанию всё read-only.

Любые исключения должны быть явно описаны в отдельной spec.

## Требование к будущей реализации

Когда lifecycle enforcement будет реализован, backend должен:

- валидировать допустимость перехода статуса;
- применять lifecycle-ограничения в mutation handlers;
- возвращать согласованную ошибку при нарушении policy;
- не полагаться только на UI.

## Требование к новым фичам

Любая новая mutation-фича должна явно отвечать на вопросы:

- зависит ли она от `team period status`;
- разрешена ли она в `validated`;
- разрешена ли она в `closed`;
- проверяется ли это на сервере;
- зависит ли она от будущих permissions / roles.

### Health-статус KR (ручной)

`POST /api/v1/krs/{krID}/progress/{numerical|boolean|project}` с опциональным полем `health_status` — mutation-фича, проходит те же вопросы:

- **зависит ли от `team period status`** — нет отдельной зависимости; идёт тем же путём, что и обновление прогресса KR;
- **разрешена ли в `validated`** — да, как и обновление прогресса (серверных lifecycle-ограничений на прогресс сейчас нет);
- **разрешена ли в `closed`** — да, как и обновление прогресса (в UI режим «только комментарии», серверного запрета на прогресс нет);
- **проверяется ли на сервере** — да: доступ к команде-владельцу (`CanAccessTeamFromCtx`, как у progress-эндпоинтов) и валидация значения `health_status` по закрытому справочнику (`400` иначе);
- **зависит ли от будущих permissions / roles** — нет; опирается на текущую модель tenant + hierarchy grants.

Правило 100%→`done` (однократный авто-переход) и приоритет ручного значения над авто-`done` описаны в `020-domain-model.md` (инварианты KeyResult) и `040-api-contract.md` («Update KR progress»).

### Копирование и перенос цели

`POST /api/v1/goals/{goalID}/transfer` (copy/move) — mutation-фича, проходит обязательные вопросы:

- **зависит ли от `team period status`** — да, от статуса **целевой** команды в **целевом** периоде: создание цели запрещено при `in_progress` / `closed` (`409`, тот же guard, что в `CreateGoal` → `ErrPeriodClosed`); статус **исходной** цели на операцию не влияет (копировать/переносить можно из любого статуса);
- **разрешена ли в `validated`** — да (ограничение только на целевую сторону);
- **разрешена ли в `closed`** — из `closed` копировать/переносить можно; **в** `closed`-целевую пару — нельзя (`409`);
- **проверяется ли на сервере** — да: доступ к обеим командам (`CanAccessTeamFromCtx` для owner-команды исходной цели и для целевой команды), guard статуса целевого периода, tenant-scope; UI-блокировка команд — лишь удобство;
- **зависит ли от будущих permissions / roles** — нет; опирается на текущую модель tenant + hierarchy grants.

Побочные эффекты: флип целевой команды `no_goals → forming`; при `mode=move` — жёсткое удаление исходной цели (каскад: KR, шеры, комментарии) и сброс статуса исходной команды в `no_goals`, если целей не осталось. Шеры исходной цели не переносятся. События журнала — `goal_copied` / `goal_moved` (категория `composition`).

### Экспорт целей в Markdown (read-only)

`GET /api/v1/teams/{teamID}/export` — не mutation-фича, но проходит те же вопросы:

- **зависит ли от `team period status`** — нет; экспорт доступен во всех статусах (`forming`/`ready`/`in_progress`/`validated`/`closed`/`no_goals`);
- **разрешён ли в `validated`** — да;
- **разрешён ли в `closed`** — да;
- **проверяется ли на сервере** — да: доступ ограничен активным тенантом (`TenantScopeFromContext`) и grant-scope (`AllowedTeamIDsFromCtx`/`CanAccessTeamFromCtx`). Для `scope=tree` вывод и счётчики строятся по пересечению поддерева со scope — недоступные ветки исключаются; для `scope=goal` цель должна быть на доске команды-контекста (владелец или расшарена в неё). Endpoint read-only, без side effects, CSRF не требуется (GET);
- **зависит ли от будущих permissions / roles** — нет; опирается только на текущую модель tenant + hierarchy grants.

## Team deletion lifecycle

Для оргструктуры сервер теперь обязан применять отдельные lifecycle/visibility правила:

- удаление команды проверяется на сервере;
- если у команды есть goals хотя бы в одном периоде, сервер выполняет только soft delete;
- hard delete разрешён только если goals нет ни в одном периоде;
- при нарушении правила hard delete сервер должен возвращать согласованную ошибку уровня конфликта/бизнес-ограничения;
- при удалении команды её дети автоматически перепривязываются к родителю удаляемой команды;
- visibility команд на `/api/v1/teams` и `/api/v1/teams/{teamID}/okrs` определяется на сервере с учётом soft delete, выбранного периода и наличия goals в этом периоде;
- активные команды остаются видимыми даже без goals;
- soft-deleted команда скрывается только если в выбранном периоде у неё нет goals.
