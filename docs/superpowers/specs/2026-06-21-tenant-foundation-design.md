# Tenant Foundation — design (Фаза 0)

Статус: проектная спека. Покрывает только **Фазу 0** — мультитенантный фундамент внутри
OSS-коробки. SaaS control-plane (Фаза 1) и on-prem делегирование (Фаза 2) — отдельные
спеки, которые опираются на этот фундамент.

## Контекст и цель

Коробка сейчас однотенантная: одно дерево команд (`department → … → employee` по
`parent_id`), одна общая БД, scope доступа через `HierarchyGrant` внутри единственного
дерева. Сущности `Tenant`/`Organization` нет.

Развитие идёт в три стороны: **SaaS** (точно пробуем, приоритет), **OpenSource**
(точно сохраняем как чистую коробку), **on-prem мультидепартамент** (возможно, не
гарантировано). И SaaS, и on-prem-мультидепартамент требуют одного примитива —
тенанта, скоупящего команды/периоды/цели/пользователей. Различается только
control-plane сверху (биллинг vs service-desk).

Цель Фазы 0 — ввести этот примитив один раз, пока данных мало и ретрофит дёшев, так
чтобы:

- коробка осталась чистой OSS (ноль SaaS-кода, схемы, секретов);
- текущая on-prem инсталляция продолжила работать как единственный дефолтный тенант;
- и SaaS, и мультидепартамент разблокировались поверх одного и того же seam'а.

## Принятые решения (фундамент)

1. **Tenancy model:** tenant как first-class сущность, **shared DB**, row-level scoping
   по `tenant_id`. Single-install OSS = один дефолтный тенант.
2. **Tenant resolution:** абстракция `TenantResolver` со стратегиями. `SessionStrategy` —
   дефолт (везде, OSS и SaaS-starter). `SubdomainStrategy` — премиум, подключается позже
   через registry, не трогая core.
3. **Настройки — три уровня по плоскостям администрирования:** `system_settings`
   (global, system-admin), `tenant_settings` (per-tenant, tenant-admin) и `user_settings`
   (per-user). Entitlements — это ключи `entitlement.*` внутри `tenant_settings`,
   писать может только system-admin/provisioning; интерфейс `Entitlements` читает их,
   OSS-реализация — `unlimited`. Коробка не знает о тарифах/Stripe.
4. **Identity:** глобальный `User` (SSO-личность, как сейчас) + новая `Membership(user,
   tenant, role)`. `HierarchyGrant` становится tenant-scoped. Переключение департамента =
   выбор активного membership.
5. **Auth & онбординг:** только OAuth/OIDC (никакого своего email+password; пароль —
   будущий provider-плагин). Дефолтные методы — instance-level в env. Тенант резолвится из
   membership после логина. Per-tenant IdP (BYO-SSO) — премиум позже. Онбординг-примитивы
   (invitation по email, join-request с апрувом, pluggable no-membership-страница) — в
   коробке.
6. **OSS boundary:** приватный `okrs-saas` импортирует коробку как Go-модуль и
   регистрирует SaaS-реализации через registry-паттерн (как `auth.Register`). Лендинг —
   третий репо.
7. **HTTP-кэш scoped-ответов:** Вариант B — `private, no-cache` + ETag, включающий
   `tenant_id` (URL-контракт не ломаем; tenant-in-URL отложен).

## Модель данных

### Новые сущности

**`tenants`**

| Поле | Тип | Назначение |
|------|-----|-----------|
| `id` | bigint PK | Внутренний стабильный ключ. Только он идёт в FK (`tenant_id`). Не светится наружу, не меняется. |
| `slug` | text, UNIQUE (глобально) | Внутренняя машинная идентичность **и** поддомен (`<slug>.okrs.com`). Lowercase, URL-safe: `^[a-z0-9]([a-z0-9-]{0,30}[a-z0-9])?$`. Проверяется по blocklist зарезервированных (`www`, `api`, `app`, `admin`, `static`, `assets`, `mail`, `auth`, …). **Иммутабелен** после создания. |
| `name` | text | Человекочитаемое название. Показывается в UI/переключателе. Свободно редактируется, не уникально. |
| `status` | enum `active`/`suspended` | `suspended` блокирует доступ (напр. неоплата), данные сохраняются; отклоняется на middleware. |
| `created_at` | timestamptz | |
| `deleted_at` | timestamptz null | Soft-delete (история целей не теряется). |

Разделение идентификаторов: `id` → внутренние связи; `slug` → роутинг/внешние ссылки;
`name` → отображение. Брендинг (лого/цвета/кастомный домен) — вне Фазы 0.

**`memberships`**

`id`, `user_id` → users, `tenant_id` → tenants, `role`, `status`, `created_at`,
`created_by_user_id`. Уникальность `(user_id, tenant_id)`. Это «человек в N
департаментах». Роль в Фазе 0 — `user`/`admin` (per-tenant). Granular roles
(viewer/editor/validator) — отдельный target-state, не связан с тенантностью.

`status`: `active` | `requested` (юзер запросил доступ, ждёт апрува tenant-admin).
Активным считается только `active`; `RequireMembership` пропускает только его.

**`tenant_invitations`**

`id`, `tenant_id`, `email`, `role`, `token_hash` (random single-use, хранится
хэшированным), `status` (`pending`/`claimed`/`revoked`), `created_by_user_id`,
`created_at`, `expires_at`. Приглашение существует **без `user_id`** (юзер ещё не
логинился).

**Claim только по токен-ссылке, не по email-match.** Email — это метка/канал доставки, не
ключ привязки. Юзер открывает invite-ссылку → логинится любым OAuth → токен гасится →
`active` membership привязывается к текущей идентичности (`provider:subject`). Это снимает
проблему «один email в двух провайдерах = две учётки»: связывает владение ссылкой, а не
строка email. Опционально: если verified-email вошедшего ≠ приглашённый email — показать
предупреждение, но не блокировать (доверие к email у провайдеров разное; авторитет —
токен). В OSS админ берёт invite-ссылку из `/admin` и передаёт сам (без SMTP); SaaS шлёт
письмом.

### Настройки — три уровня (по плоскостям администрирования)

Настройки разнесены по трём store'ам, ровно по трём плоскостям управления (см. раздел
«Плоскости администрирования»). Это нужно и для прав записи, и для оптимизации
(см. «Оптимизация горячего пути»).

**`system_settings` (global, key/value)** — остаётся как было, но теперь это плоскость
**system-admin**. Глобальные ключи инстанса, управляемые из `/system`:

- `default_registration_tenant_id` (int64, nullable) — в какой тенант попадает новый
  пользователь без membership; `null` → страница-заглушка.

Таблица НЕ получает `tenant_id` — она глобальная.

**`tenant_settings` (tenant_id, key, value_json)** — плоскость **tenant-admin**, per-tenant.
Сюда переезжают продуктовые ключи из прежнего `system_settings` плюс entitlements. Два
класса ключей с разной write-authority:

- продуктовые настройки (`new_user_policy`, `default_hierarchy_node_id`,
  `documentation_url`, `health_checkin_config`, `feedback_*`) — пишет tenant-admin из
  `/admin`;
- entitlement-ключи в namespace `entitlement.*` (`entitlement.sso`,
  `entitlement.subdomains`, `entitlement.file_uploads`, `entitlement.max_users`) — пишет
  только system-admin/provisioning.

Write-authority по namespace проверяется в service-слое — tenant-admin не может выдать
себе `entitlement.sso`. Интерфейс `Entitlements` читает `entitlement.*`; OSS-реализация
возвращает unlimited и эти ключи игнорирует, так что в OSS таблица — просто настройки
тенанта. SaaS не форкает миграции.

**`user_settings` (user_id, key, value_json)** — плоскость **user**, per-user, управляется
из `/settings`. В Фазе 0 — минимально: дом под личные преференсы + опциональный
`default_landing_tenant_id` (какой тенант активировать при входе). Конкретные UI-преференсы
— позже.

`new_user_policy` сознательно остаётся per-tenant (как новый член скоупится *внутри*
тенанта), а «в какой тенант вообще попадает новый юзер» — это global
`default_registration_tenant_id`. Два разных уровня.

### Идентичность (остаётся глобальной)

- `users` — без `tenant_id`, это SSO-личность. Системные `anonymous-local` (id=1),
  `migration` (id=2) — глобальны.
- Флаг `users.is_admin` **расщепляется**: `users.is_system_admin` (суперадмин инстанса,
  только provisioning/кросс-тенант) + «админ организации» переезжает в
  `memberships.role = admin` (per-tenant).
- Грань идентичности — `provider:subject` (как сейчас). Email — атрибут профиля, **не**
  ключ. Один человек, зашедший через разные провайдеры с одним email, — это **две разные
  учётки** и два независимых membership-набора; в Фазе 0 они не сливаются (это безопасно,
  т.к. инвайты/гранты привязаны к `provider:subject`). Cross-provider account linking
  (привязать второй провайдер из `/settings`) — будущая фича, вне Фазы 0.

### `tenant_id` на scoped-таблицах

Добавляется на: `teams`, `periods`, `goals`, `goal_shares`, `team_period_statuses`,
`user_hierarchy_grants`. **Денормализуется и на дочерние** (`key_results`,
`goal_comments`, `key_result_notes`) — defense-in-depth: каждый запрос несёт `tenant_id`,
не полагаясь на join. `system_settings` остаётся глобальной (без `tenant_id`);
per-tenant-настройки живут в `tenant_settings`.

### Изменения существующих ограничений

- Продуктовые ключи из `system_settings` переезжают в `tenant_settings` (per-tenant);
  сам `system_settings` остаётся глобальным для system-admin-ключей
  (`default_registration_tenant_id`). Добавляется `user_settings` (per-user). Инстансные
  секреты (auth-провайдеры, DSN, provisioning-token) остаются в env.
- `periods.name`: глобальная уникальность → уникальность в пределах тенанта.
- `auth_sessions`: добавляется `active_tenant_id` (nullable) — тенант, который сейчас
  смотрит сессия. Сессия по-прежнему привязана к глобальному user.

### Миграция существующих данных (`027_tenancy`)

Одна миграция, идемпотентная, с down:

1. Создать `tenant #1 (slug='default')`.
2. Проставить `tenant_id = 1` во все существующие строки scoped-таблиц.
3. Завести membership'ы для всех текущих пользователей в `tenant #1` (роль из старого
   `is_admin`).
4. Перенести `is_admin` суперадминов в `is_system_admin`.
5. Создать `tenant_settings` и `user_settings`; перенести продуктовые ключи из
   `system_settings` в `tenant_settings` под `tenant #1`; `system_settings` оставить
   глобальным; добавить `active_tenant_id`; поменять уникальность `periods.name`.

Текущая on-prem инсталляция продолжает работать как один тенант — поведение не меняется.

## Жизненный цикл запроса

Цепочка middleware:

```
AccessLog → Session → (RequireAuth) → TenantResolve → RequireMembership → Scope → CSRF → handlers
```

- **`TenantResolve`** — `TenantResolver` (упорядоченный список стратегий, первая давшая
  тенант выигрывает) кладёт тенант в контекст. `SessionStrategy`: `active_tenant_id` из
  сессии, иначе дефолт на первый/единственный membership. `SubdomainStrategy` (премиум):
  `Host` → `slug` → тенант.
- **`RequireMembership`** — главный гейт изоляции. Проверяет membership в резолвнутом
  тенанте и что тенант не `suspended`. Иначе 403.
- **`Scope` (PolicyEvaluator)** — считает видимые team IDs **внутри** тенанта: hierarchy
  grants фильтруются по `tenant_id`, рекурсивная CTE дополняется условием тенанта.

**Единая точка enforcement в данных:** `TenantScope` — value из контекста (id текущего
тенанта); scoped-методы репозиториев обязаны его принимать; каждый scoped-запрос несёт
`tenant_id = $scope`. Денормализация (см. выше) — defense-in-depth.

**Переключение тенанта:** `POST /api/v1/session/tenant {slug|tenant_id}` — проверяет
membership, обновляет `auth_sessions.active_tenant_id`. Переключатель — в общем
`header.js` рядом с гамбургер-меню: список memberships + смена активного.

**No-auth (`AUTH_MODE=disabled`):** `anonymous-local` получает membership в `tenant #1`;
резолвер всегда отдаёт `tenant #1`. OSS single-tenant работает без изменений.

**Enforcement фич:** premium-эндпоинты спрашивают `entitlements.Has(tenant, "…")` /
`entitlements.Limit(tenant, "…")`. В OSS — всегда `true`/`∞`.

## Оптимизация горячего пути

Каждый запрос проходит resolve тенанта → membership → scope → (опц.) entitlements/настройки.
Делать это SQL-запросами на каждый request — недопустимо. Принцип: **снапшот грузится один
раз и кэшируется в памяти; на горячем пути — только lookup'ы в map, ноль DB**. Инвалидация —
по событию записи (per-tenant), TTL — как backstop.

**Снапшот настроек, не чтение по ключу.** Настройки тенанта малы (десяток ключей). Грузим
**все ключи тенанта одним запросом** в `map[key]value` и кэшируем как единый снапшот, а не
дёргаем БД на каждый `Has`/`Get`. `Entitlements.Has/Limit` и чтение продуктовых ключей —
это lookup в закэшированном снапшоте. Аналогично `system_settings` (глобальный снапшот,
крошечный) и `user_settings` (per-user, грузится только на `/settings`, не на горячем пути).

**Кэши (паттерн существующих grants/healthcheckin — TTL + invalidate-on-write):**

| Кэш | Ключ | Что | Инвалидация |
| --- | --- | --- | --- |
| `TenantCache` | `id`, `slug` | строка `tenants` | create/update/suspend тенанта |
| `MembershipCache` | `user_id` → `[{tenant_id, role}]` | membership'ы юзера | изменение membership |
| `TenantSettingsCache` | `tenant_id` → снапшот | все ключи `tenant_settings` (вкл. `entitlement.*`) | запись `tenant_settings` тенанта |
| `SystemSettingsCache` | — | глобальный снапшот | запись `system_settings` |
| grants-кэш (есть) | `tenant_id` (добавить) | гранты | изменение грантов |

**Resolve один раз в request-context.** `TenantResolve`/`RequireMembership`-middleware
кладут резолвнутые `tenant`, `membership/role`, снапшот настроек и `entitlements` в контекст
запроса (преимущественно cache-hit'ы). Хендлеры/сервисы читают из контекста, повторно в БД
не ходят. За запрос на горячем пути — максимум lookup'ы, не SQL.

**Эвикция для масштаба.** В SaaS тенантов много — кэши не грузят всё разом: load-on-demand
по активному тенанту + LRU/TTL-эвикция простаивающих (как healthcheckin грузит per-period).

**Многоинстансность (scale-out) — осознанно отложено.** Сейчас single-process in-memory:
инвалидация локальная + TTL. При горизонтальном масштабировании понадобится кросс-инстансная
инвалидация (pub/sub или короткий TTL) — это будущий пункт, не Фаза 0. Под это заложено: TTL
есть у каждого кэша, инвалидация идёт через единый хук записи (легко заменить на pub/sub).

## Кэши и изоляция (HTTP)

- **HTTP-кэш scoped-ответов** перестаёт быть `public`. Вариант B: `private, no-cache` +
  ETag, включающий `tenant_id`. Устаревший entry ревалидируется и не утекает между
  тенантами на общем домене. На поддомене (премиум) Host сам разводит кэш. Текущий
  `Cache-Control: public, max-age=300` (`internal/http/handlers/api/v1/cache.go`) —
  убрать для tenant-scoped эндпоинтов.
- **Grants-кэш** (`internal/store/grants/grants.go`, `listAllGrants`) — ключ/фильтр
  включает `tenant_id`.
- **Health-checkin кэш** (`internal/service/healthcheckin_cache.go`) — keyed/iterated
  per-tenant; инвалидация скоупится тенантом; «активный период» и refresh-loop становятся
  per-tenant (N активных периодов вместо одного).

## Плоскости администрирования

Три раздельные плоскости, каждая со своим UI-shell и своим уровнем настроек.

**1. System admin** (`users.is_system_admin`) — `/system/*` (новый shell). Над тенантами:

- список тенантов + их настройки;
- **глобальный** список пользователей (кросс-тенант — для этого `User` глобальный);
- прикрепить пользователя к тенанту (создать membership) — UI над provisioning API;
- задать `default_registration_tenant_id` (global setting).

Не роль внутри тенанта. OSS = оператор; SaaS = сервис-аккаунт control-plane; on-prem =
service-desk.

**2. Tenant admin** (`membership.role = admin`) — существующий `/admin/*`, теперь
tenant-scoped. Внутри своего тенанта: иерархия команд, периоды, health-check settings,
`new_user_policy`, документация, feedback, пользователи/гранты. Пишет продуктовые ключи
`tenant_settings`, но **не** `entitlement.*`. Делегированный админ департамента.

**3. User** (любой авторизованный) — существующий `/settings/*`. Личные настройки
(`user_settings`) + «мои организации» (список memberships, опц. дефолтный лендинг-тенант).

### Новый пользователь (регистрация)

Первый SSO-вход без membership нигде:

- если задан `default_registration_tenant_id` → автосоздаётся membership (`role=user`) в
  этом тенанте, далее применяется `new_user_policy` **этого тенанта** для начального grant;
- иначе → **страница-заглушка** (юзер создан, доступа нет, «обратитесь к администратору»).

Приоритет с invitation определяется наличием токена в URL: пришёл по invite-ссылке →
редимится приглашение (→ его тенант); обычный вход без токена → авто-дефолт или заглушка.
По email при логине приглашения не ищутся.

### Provisioning API (в коробке, под system-credential)

- `POST /api/v1/system/tenants {name, slug, entitlements?}` — создать тенант.
- `POST /api/v1/system/tenants/{id}/members {identity_ref|email, role}` — назначить
  админа (через `tenant_invitations` по email или прямой membership по identity).
- `PUT /api/v1/system/tenants/{id}/entitlements` — записать `entitlement.*` ключи в
  `tenant_settings` (SaaS-биллинг; в OSS есть, но реализация всё равно unlimited).
- `POST /api/v1/system/tenants/{id}/suspend|restore`.

Tenant-admin-эндпоинты онбординга (под membership.role=admin, не system-credential):

- `POST /api/v1/admin/invitations {email, role}` — пригласить (создаёт `tenant_invitations`).
- `GET /api/v1/admin/access-requests` / `POST …/access-requests/{id}/approve|deny` —
  очередь join-request'ов.

Эти эндпоинты — единый seam: и SaaS-signup (Фаза 1), и service-desk (Фаза 2) дёргают их
как system-credentialed caller.

**Аутентификация machine-вызовов:** provisioning-token в env/config (instance-level, как
DSN). В OSS-коде секрета нет — только проверка.

**Bootstrap:**

- `AUTH_MODE=disabled`: `anonymous-local` уже admin в `tenant #1`.
- `AUTH_MODE=enabled`, свежая инсталляция: env `BOOTSTRAP_SYSTEM_ADMIN=<provider:subject|email>`
  — первый совпавший логин становится system-admin.
- **Invitation by token-link** — назначение до первого входа через `tenant_invitations`;
  claim по одноразовой токен-ссылке (не по email-match), привязка к идентичности при редиме.

## Аутентификация, регистрация, онбординг

**Регистрация — только OAuth/OIDC.** Своего email+password нет (коробка не знает про
пароли). SaaS-self-service — «Continue with Google/GitHub», корпоратив/on-prem — Keycloak.
Email+password при необходимости — будущий provider-плагин (`providers/password`), тот же
registry-паттерн.

**Дефолтные методы авторизации — instance-level, в env**, задаются оператором при деплое
(`EnabledProviders` + client id/secret). Логин-экран показывает включённое в инстансе.
Per-tenant переопределения дефолтных методов и UI для instance-провайдеров нет.

**Per-tenant SSO (премиум, Фаза 1):** конфиг IdP тенанта в `tenant_settings` под `sso.*`
(issuer, client_id, client_secret), **секрет шифруется at-rest** (app-ключ из env), гейт
`entitlement.sso`. Настраивает tenant-admin в `/admin → SSO`. Требует tenant-before-login
→ поддомен (или email-domain routing). Реализуется тем же OIDC-плагином, но конфиг берётся
из tenant-store, а не из env. **Заклад в Фазе 0 (seam, без реализации):** фабрика
провайдера принимает источник конфига (env ИЛИ tenant-store); утилита шифрования секретов;
путь tenant-before-login.

**No-membership-страница — pluggable seam.** Когда у авторизованного юзера нет активного
membership:

- OSS-дефолт — заглушка «обратитесь к администратору» (+ форма join-request по slug);
- SaaS регистрирует онбординг «Создать организацию» / «Вступить».

**Онбординг-флоу (примитивы в коробке):**

- **A. Регистрация + создание тенанта (SaaS):** OAuth → глобальный `User` без membership →
  no-membership-страница → «Создать организацию» → имя + slug → тенант, создатель =
  tenant-admin. Self-service создание = SaaS-обёртка над provisioning-примитивом с лимитами
  тарифа; OSS/on-prem self-service не включают (там `/system` или service-desk).
- **B. Приглашение (tenant-admin):** `/admin` → invite по email → `tenant_invitations` с
  токен-ссылкой (OSS: ссылку показываем админу; SaaS: шлём письмом) → приглашённый
  открывает ссылку, логинится любым OAuth → токен гасится → `active` membership на текущую
  идентичность.
- **C. Запрос доступа (юзер):** no-membership-страница → ввести **slug** тенанта → membership
  `status=requested` → очередь «Запросы доступа» в `/admin` → апрув → `active`. Публичного
  каталога тенантов нет.

**OSS — дефолт-тенант:**

- Одно-департаментный: `default_registration_tenant_id = 1` → каждый SSO-вход
  авто-попадает в единственный тенант (поведение как сегодня).
- Мульти-департаментный: `null` → новые входы на onboarding/заглушку; админы прикрепляют
  через `/system` или через invitations/requests.
- `AUTH_MODE=disabled`: `anonymous-local` — admin в `tenant #1`.

## OSS / SaaS разделение

**Коробка (`okrs`, public)** самодостаточна и мультитенантна сама по себе (несколько
департаментов на одном OSS-инстансе через Session-стратегию + ручной provisioning, без
SaaS). Определяет:

- интерфейсы: `TenantResolver`/`ResolveStrategy`, `Entitlements`, порт записи
  provisioning, `NoMembershipHandler` (онбординг-страница), источник конфига auth-провайдера
  (env ИЛИ tenant-store);
- OSS-реализации: `SessionStrategy`, `UnlimitedEntitlements`, заглушка-`NoMembershipHandler`
  с формой join-request;
- registry: `tenant.RegisterResolver(name, factory)`, `entitlements.Register(name,
  factory)`, регистрация `NoMembershipHandler`. Конфиг выбирает реализацию по имени.
  SaaS регистрирует `SubdomainStrategy`, биллинг-`Entitlements`, онбординг-страницу.

**Ограничение Go:** `internal/` нельзя импортировать из другого модуля. Поэтому Фаза 0
вводит публичный **`okrs/app`** façade — собирает приложение из `Config` + инжектируемых
seam'ов (resolver-стратегии, реализация `Entitlements`, доп. mount'ы роутов/middleware).
Внутренние пакеты остаются `internal/`; наружу торчит только `app`. Конструкция сервера
в `internal/http` параметризуется; `app` — тонкая обёртка.

**Три репо:** `okrs` (public, коробка + OSS `main`), `okrs-saas` (private, `require okrs`,
свой `main` с blank-import SaaS-пакетов + control-plane роуты), `okrs-landing` (private,
статика).

**SaaS-данные** (биллинг, Stripe, тарифы) — в собственной БД `okrs-saas`, не в схеме
коробки. SaaS отражает результат в коробку через `PUT …/entitlements`. В схеме коробки
нет ни одного SaaS-понятия.

Итог: коробка физически не содержит SaaS-кода (отдельный модуль), схемы (только
нейтральная `tenant_entitlements`), секретов (token в env). «OSS без деталей
saas-инсталяции» выполняется by construction.

## Порядок реализации

1. Миграция `027_tenancy` (схема + backfill + down): `tenants`, `memberships`
   (+`status`), `tenant_invitations`, `tenant_settings`, `user_settings`; `tenant_id` на
   scoped-таблицы; split `is_admin`; `active_tenant_id`; перенос продуктовых ключей в
   `tenant_settings`.
2. Домен: `Tenant`, `Membership`, `Role`; правки `User`.
3. Store: `TenantRepository`, `MembershipRepository`, `TenantSettingsRepository`,
   `UserSettingsRepository`; `tenant_id`-scoping во все репозитории через `TenantScope`.
4. Кэши горячего пути: `TenantCache`, `MembershipCache`, `TenantSettingsCache`,
   `SystemSettingsCache`; grants-кэш tenant-keyed; единый invalidate-on-write хук.
5. Service: резолвер тенанта, проверка membership, provisioning-сервис, `Entitlements` +
   `UnlimitedEntitlements`; онбординг (invitation-claim при логине, join-request+approve,
   `default_registration_tenant_id` / `NoMembershipHandler`-seam); auth provider-фабрика с
   источником конфига + утилита шифрования секретов (seam, без per-tenant реализации).
6. Auth/middleware: `TenantResolve` + `RequireMembership` (resolve once в request-context);
   `PolicyEvaluator` с фильтром по тенанту; `active_tenant_id`; эндпоинт переключения.
7. HTTP: переключатель в `header.js`; `private, no-cache` + tenant-ETag для scoped-GET;
   `/system/*` shell (тенанты, глобальные юзеры, attach, default-tenant); tenant-scoped
   `/admin/*` (+ invitations, access-requests); `/settings/*` (memberships-view);
   OSS-заглушка `NoMembershipHandler` с формой join-request.
8. Health-checkin кэш per-tenant; refresh-loop по активному периоду каждого тенанта.
9. `app`-façade + параметризованный сервер; OSS `main` на дефолтах.
10. `seed_demo.sql` — дефолтный тенант.
11. Specs: `010`, `020`, `040`, `050`.

## Тесты

- **Изоляция (главные security-тесты):** юзер тенанта A не читает/не мутирует данные B на
  каждом scoped-эндпоинте.
- Membership-гейт: 403 без membership, блок `suspended`.
- Резолвер: session-дефолт + переключение, мультимембершип.
- Scope: гранты фильтруются тенантом, рекурсивная CTE внутри тенанта.
- Provisioning: создание тенанта, назначение админа, read-back entitlements.
- Онбординг: invitation claim'ится **только по валидному одноразовому токену** (повтор/
  истёкший/чужой токен — отказ), привязка к текущей идентичности; email-match НЕ даёт
  доступ; один email через два провайдера = две учётки; join-request по slug, approve/deny
  меняет `status`; `requested` не проходит `RequireMembership`.
- New-user flow: с `default_registration_tenant_id` → membership + `new_user_policy`
  тенанта; без него → `NoMembershipHandler` (OSS-заглушка).
- Entitlements: OSS отдаёт `true`/`∞`, gating-эндпоинт разрешает.
- Tenant settings write-authority: tenant-admin пишет продуктовые ключи, но получает 403
  на `entitlement.*`; system-admin/provisioning пишет `entitlement.*`.
- Миграция/backfill: строки уходят в `tenant #1`, membership'ы созданы, split `is_admin`
  корректен, продуктовые ключи перенесены в `tenant_settings`, идемпотентность.
- Горячий путь: настройки/membership читаются из кэша (снапшот), не per-key SQL;
  инвалидация по записи сбрасывает нужный per-tenant ключ; resolve кладётся в
  request-context один раз.
- Кэши изоляции: scoped-GET отдаёт `private/no-cache` + tenant-ETag; grants/healthcheckin
  keyed by tenant.

## Вне scope Фазы 0

- Control-plane, биллинг, Stripe, тарифы, self-service создание тенанта, доставка
  invite-писем — Фаза 1.
- Реализация `SubdomainStrategy` — Фаза 1 (сейчас абстракция + registry).
- Per-tenant IdP / BYO-SSO — премиум позже (в Фазе 0 только seam: источник конфига
  провайдера + шифрование секретов).
- Свой email+password — будущий provider-плагин, не сейчас.
- Брендинг тенанта.
- Service-desk адаптер — Фаза 2 (примитив provisioning API уже есть).
- Tenant-in-URL кэширование (Вариант A).
- Granular roles (viewer/editor/validator).
