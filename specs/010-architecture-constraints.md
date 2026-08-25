# Architecture constraints

## Архитектурный стиль

Сохраняем текущий стиль:

- SSR отдаёт HTML-каркас (shell) для каждой SPA-страницы; общий `<head>` и вендор-блок вынесены в партиал `shell_partials.html` (`spa-head` / `spa-vendor`), поэтому shell-шаблоны не дублируют CDN/reset/loader;
- данные и мутации идут через HTTP/API;
- фронтенд без сборщика (toolchain не нужен): React 18 и прочие библиотеки self-hosted из `/static/vendor` (не CDN), JSX компилируется `@babel/standalone` в браузере; dev- или production-сборка React выбирается флагом `WEB_ASSETS_DEV` (env → `Options.AssetsDev` → данные shell-шаблона `{{.Dev}}`), по умолчанию — production;
- файловая раскладка ассетов: `/web/templates/*.html` — SSR-shell'ы, встраиваются в бинарь через `web.TemplatesFS` (отсутствующий шаблон ловится на `go build`); `/web/static/**` — JS/CSS/vendor, отдаются с диска (`http.Dir("web/static")` относительно рабочего каталога процесса), поэтому правки видны после обновления страницы без пересборки. URL-префикс `/static/` не зависит от раскладки на диске. В контейнере рабочий каталог — `/app`, куда Dockerfile кладёт `COPY web /app/web`;
- компоненты трекера живут в `tracker.js`, стили — в `tracker.css`; добавление новой библиотеки = файл в `/static/vendor` + `<script>` в партиале `spa-vendor`; общие токены/каркас/компоненты — `tokens.css` (CSS-переменные: акцент, палитра, радиусы), `shell.css` (reset, скроллбар, `.loading-screen`), `components.css` (`.user-selector*`/`.user-chip*`/`.user-avatar__fallback`) — подключаются во всех SPA-shell;
- общие модули загружаются как `text/babel` ПЕРЕД app-скриптом каждого shell, экспортируя глобальные функции/компоненты: `api.js` (`readCSRF`, `csrfHeaders` — единый CSRF-слой для всех страниц; раньше дублировался в каждом entrypoint с расхождениями), `storage.js` (`STORAGE_KEYS`, `readJSON`/`writeJSON` — общий контракт localStorage трекера и настроек; подключается только в `tracker_shell`/`settings_shell`), `markdown.js` (`Markdown`, `MarkdownEditor`) и `sidebar.js` (`Sidebar`, `SidebarTenant`, `SidebarSections`, `SidebarFooter`, `SidebarBell`, `FeedbackNudge` — общий тёмный сайдбар навигации). Это единственный источник правды переиспользуемой навигации: постоянно видимый тёмный сайдбар одинаков (шапка тенанта с переключателем организаций и колокольчиком; блок «Разделы» с ссылками; футер с документацией/обратной связью и блоком пользователя, у которого меню «···» → Настройки/Администрирование/System/Выйти) в трекере, админке, настройках и страницах-заглушках. Контекстная навигация страницы передаётся через `children` (на трекере — период и дерево команд, в настройках/админке — их локальные секции). Блок глобальных «Разделов» можно скрыть пропом `showSections={false}` — так сделано в админке, где остаётся только её собственная навигация. `sidebar.js` самодостаточен (свой рендер аватара инлайн-стилями, чтение CSRF из cookie, logout, запросы `/api/v1/config` и `/api/v1/session/tenants`), стили — `sidebar.css`, подключён во всех SPA-shell (`tracker_shell`, `admin_shell`, `settings_shell`, `system_shell`, `stub_shell`). Тот же `Sidebar` (со скрытыми глобальными «Разделами», `showSections={false}`) используется и на странице «нет доступа» (`no_membership`), и в системной superadmin-панели (`system.js`): последняя зеркалит layout админки — тёмный сайдбар с контекстной навигацией разделов («Пространства», «Участники», «Пользователи», «Регистрация», «Entitlements», «Сообщения») + верхняя строка-хлебные крошки и скролл-регион контента. Cross-tenant специфика панели (она tenant-less, вне membership-гейта) на layout не влияет: `/api/v1/me` и `/api/v1/session/tenants` доступны любому аутентифицированному пользователю, а недоступный вне membership-гейта `/api/v1/config` деградирует мягко (футер сайдбара рендерит профиль/выход при `cfg === null`);
- компонент `UserSelector` реализован дважды с одинаковым поведением и CSS-классами: в `tracker.js` (React, multi-select для владельца цели) и в `admin.js` (React, single-select для руководителя команды); CSS-классы (`.user-selector*`, `.user-chip*`, `.user-avatar__fallback`) вынесены в общий `components.css` (подключается в `tracker_shell`/`admin_shell`). Слияние двух React-реализаций в один параметризуемый компонент — открытый пункт технического долга;
- PostgreSQL как единственное системное хранилище;
- SQL-миграции — единственный способ менять схему;
- Docker Compose остаётся базовым локальным способом запуска.

## Слои

AI должен сохранять разделение ответственности:

- `internal/core/domain` — доменные типы и enum, включая `User`, `AuthSession`;
- `internal/core/progress` — расчёты прогресса (бывший `internal/okr`): формулы по типам KR плюс `ForKR`/`ForGoal`, диспетчеризующие по виду KR и агрегирующие цель. Чистые функции над доменными типами, без I/O;
- `internal/platform/entitlements` — интерфейс `Entitlements` и его реестр;
- `internal/platform/nomembership` — реестр страницы «нет доступа» (бывший `internal/onboarding`);
- `internal/render/export` — рендер OKR в Markdown;
- `internal/store` — SQL и persistence; каждый тип сущности имеет свой отдельный repository-тип; `store.Store` — composite-фабрика; auth-методы (users, sessions, grants, settings) живут здесь;
- `internal/service` — сервисы по сущностям, по пакету на сущность: `team`, `goal`, `keyresult`, `period`, `goalshare`, `goallink`, `teamstatus`, `user`, `activity`, `progresssnap`, плюс `settings`, `provisioning`, `onboarding`, `healthcheckin`. Каждый работает **с одной** сущностью через **один** репозиторий, объявленный интерфейсом на стороне потребителя (`team.Repo`, `goal.Repo` и т.д.), и не пишет в журнал активности. `internal/service/servicetest` — общие fake-репозитории для тестов всех сервисов и usecase. Файлов в корне `internal/service` нет: фасад `service.Service` удалён, каждый обработчик получает ровно те сервисы и usecase, которые вызывает;
- `internal/usecase` — бизнес-сценарии: `okrboard`, `goal`, `keyresult`, `period`, `goaltree`, `export`, `user`, `healthcheckin`. Каждый оркестрирует **сервисы сущностей**, а не репозитории: цепочка `handler → usecase → service → store`, у store одна дверь;
- `internal/scheduler` — фоновые петли (обновление кэша health check-in, снимки прогресса). Запускается из `app.New`, а НЕ из `Routes()`: построение роутера должно оставаться чистой сборкой, иначе его нельзя вызвать в тесте без goroutine и БД;
- `internal/auth` — auth manager, middleware chain, provider interface, policy evaluator;
- `internal/auth/providers/{name}` — реализации провайдеров; каждый провайдер — изолированный пакет;
- `internal/http` — SSR handlers; шаблоны живут в `/web/templates` и встраиваются пакетом `web`
  (`web.TemplatesFS`); `NewServer(..., Options)` параметризуется
  инжектируемыми seam'ами (resolver, `Entitlements`, имя no-membership-страницы, mount'ы
  control-plane роутов по уровням);
- `internal/http/httpdeps` — сборка графа сервисов и usecase: `Build(store, grantsCache, hcCache, logger) Deps`. Единственное место, где известен полный список зависимостей; `server.go` раздаёт из него поля по пакетам;
- `internal/http/dto` — структуры JSON-ответов с тегами. Отдельный пакет, потому что одну и ту же форму ответа собирают разные обработчики и их `*common`-пакеты; копии рядом с каждым обработчиком означали бы расходящийся контракт. Внутрь `dto` не попадают ни доменные типы, ни типы стора — только то, что уходит в сеть;
- `internal/http/handlers/api/v1` — API-контракт для JSON/form-data; **пакет на URI**, см. [070-code-structure.md](070-code-structure.md);
- `internal/http/handlers/web` — веб-хендлеры SSR-страниц (login, no-access, goal-delete); все `/admin*`, `/`, `/settings`, `/system` отдают React-shell из `server.go` (единый источник правды навигации — `sidebar.js`);
- `app` (public, **корень модуля**) — фасад: `app.New(Config) (*App, error)` собирает приложение
  из `Config` + seam'ов, выбираемых по имени из реестров (`auth.RegisterResolveStrategy`,
  `entitlements.Register`, `nomembership.Register`) и mount-хуков `PublicRoutes`/`AuthedRoutes`/
  `TenantRoutes` (по одному на middleware-уровень). `cmd/server` — тонкий OSS-entrypoint поверх `app`;
- `web` (public, **корень модуля**) — SSR-ассеты: только `embed.FS` с шаблонами, без логики.
  Существует потому, что `//go:embed` не может ссылаться за пределы каталога своего пакета —
  директива в `internal/http` не дотянулась бы до `/web/templates`.

Публичных пакетов ровно два: `app` — фасад приложения, и `web` — SSR-ассеты. Всё остальное — `internal/`.

**Группировка в `internal/`.** В корне `internal/` лежат только слои и группы, не отдельные доменные пакеты: `core/` — чистая логика без I/O; `platform/` — registry-сеймы OSS/SaaS; `render/` — форматтеры; плюс `auth/`, `http/`, `service/`, `store/`. Новый пакет кладётся в существующую группу; заводить пакет в корне `internal/` — повод сначала решить, к какой группе он относится.

**Граница service / usecase.** Метод принадлежит сервису сущности, если трогает не более одного репозитория **и** не пишет в журнал активности. Иначе это usecase. Правило операционное — проверяется механически, а не на вкус: карту «метод → репозитории» можно построить скриптом и свериться.

**Usecase не ходит в репозитории.** Если сценарию нужна операция над одной сущностью, её добавляют в сервис этой сущности (обычно однострочный проброс), а не тянут репозиторий в usecase. Иначе слой service вырождается, а у store появляется вторая дверь.

**Зависимости между usecase запрещены.** Если сценарию нужен результат другого сценария, объявляется узкий порт на стороне потребителя, а не импорт чужого пакета целиком. Так сделано в `usecase/export` (порт `BoardReader` в `okrboard`), `service/goallink` (`GoalProgressReader` в `goal`) и `internal/scheduler` (`PeriodFinder`, `SnapshotRunner`, `ActivePeriodLister`). Порт называет ровно то, что нужно потребителю, и не даёт слою слипнуться внутри себя.

Запрещена зависимость от *поведения*: вызов чужого usecase напрямую. Импорт ради *типа* в сигнатуре собственного порта допустим и неизбежен — `usecase/export` импортирует `usecase/okrboard` только затем, чтобы назвать `okrboard.TeamOKR` в `BoardReader`. Признак, по которому это отличается от нарушения: в пакете нет ни одного обращения к методам чужого usecase, и подмена реализации в тесте не требует его вовсе.

**Обработчик на URI.** Один URI обслуживает один пакет, путь которого повторяет путь URI (сегменты-параметры выброшены, дефисы убраны): `/api/v1/goals/{goalID}/key-results` → `handlers/api/v1/goals/keyresults`. Методы называются по глаголу (`Get`, `Post`, `Patch`, `Delete`), регистрация — в `routes.go` того же пакета. Общий для группы код выносится в лист-пакет (`goalcommon`, `admincommon`, …), потому что родитель монтирует подпакеты и обратный импорт даёт цикл. Полная карта и два намеренных исключения — в [070-code-structure.md](070-code-structure.md).

**Handler не работает с сырыми данными store.** Обработчик принимает и отдаёт итоговые модели: доменные типы, DTO ответа и типы своего сервиса/usecase. Позиция строки в таблице, курсор пагинации, `*Filter`/`*Input` репозитория — детали слоя store, и в handler им делать нечего. Показательный случай — лента активности: курсор `created_at|id` кодируется и разбирается в `service/activity`, наружу и обратно ходит непрозрачная строка (`activity.Filter.Cursor`, `activity.Page.NextCursor`), а `activitycommon` занимается только разбором query-параметров и сборкой DTO.

Остаточные протечки store-типов в handlers числятся долгом и перечислены в TODO дизайн-дока: sentinel-ошибки (`goals.ErrNotFound` и подобные — им место в `core/domain`), `*Input`-структуры репозиториев и read-модели вида `memberships.MembershipWithTenant`.

**Набор маршрутов зафиксирован golden-тестом.** `internal/http/routes_golden_test.go` обходит собранный роутер через `chi.Walk` и сверяется с `testdata/routes.golden`. Grep по исходникам для этого не годится: как только URI становится переменной (табличная регистрация SSR-страниц), он молча перестаёт их видеть. Намеренное изменение контракта обновляет golden в том же change set: `go test ./internal/http -run RoutesGolden -update-routes`.

**Батчевые чтения остаются батчевыми.** Методы вида `ListByTeamsPeriod(periodID, teamIDs)`, `ListCommentsByGoals(goalIDs)`, `RecordBatch` существуют, чтобы не делать N+1 (правило 9 в CLAUDE.md). Они помечены комментарием в коде; превращение любого из них в цикл — регрессия, а не рефакторинг.

**Именование.** Store — множественное число, service — единственное (`store/goals` ↔ `service/goal`). Коллизии имён пакетов разрешаются алиасом на месте импорта, а не переименованием каталога:

```go
import (
    goals   "okrs/internal/store/goals"
    goalsvc "okrs/internal/service/goal"
)
```

Алиас `<entity>svc` единообразен во всём дереве.

## Repository layer

`internal/store` содержит отдельные repository-типы по одному на тип сущности, каждый в своём подпакете:

| Поле `store.Store` | Пакет | Тип | Сущности |
|-----|------|------|----------|
| `Teams` | `store/teams` | `*teams.TeamRepository` | teams CRUD |
| `Goals` | `store/goals` | `*goals.GoalRepository` | goals, comments, KR-loading (aggregate), copy |
| `GoalLinks` | `store/goallinks` | `*goallinks.GoalLinkRepository` | связи целей (иерархия) |
| `Periods` | `store/periods` | `*periods.PeriodRepository` | periods CRUD |
| `KRs` | `store/krs` | `*krs.KRRepository` | key results, meta, progress, project stages |
| `Shares` | `store/shares` | `*shares.GoalShareRepository` | goal shares |
| `Statuses` | `store/statuses` | `*statuses.TeamStatusRepository` | team period statuses |
| `Users` | `store/users` | `*users.UserRepository` | users |
| `Sessions` | `store/sessions` | `*sessions.SessionRepository` | auth sessions |
| `Grants` | `store/grants` | `*grants.GrantRepository` | hierarchy grants (+ кэш) |
| `Settings` | `store/settings` | `*settings.SettingsRepository` | system settings (+ кэш) |
| `Activity` | `store/activity` | `*activity.ActivityRepository` | журнал активности |
| `ProgressSnap` | `store/progresssnap` | `*progresssnap.Repository` | снимки прогресса |
| `Tenants` | `store/tenants` | `*tenants.TenantRepository` | тенанты (+ кэш) |
| `Memberships` | `store/memberships` | `*memberships.MembershipRepository` | членства (+ кэш) |
| `TenantSettings` | `store/tenantsettings` | `*tenantsettings.TenantSettingsRepository` | настройки тенанта (+ кэш) |
| `UserSettings` | `store/usersettings` | `*usersettings.UserSettingsRepository` | пользовательские настройки |
| `Invitations` | `store/invitations` | `*invitations.InvitationRepository` | инвайты и заявки на доступ |

Плюс поле `DB *pgxpool.Pool` и вспомогательный пакет `store/testutil` (хелперы тестов, не репозиторий).

`store.Store` — composite, созданный через `store.New(db)`; поля перечислены в таблице выше. Дополнительно реализует `auth.authStorage` интерфейс (forwarding-методы к подрепозиториям) для совместимости с `auth.Manager`.

Правила:
- С базой данных работает только repository-слой — никаких прямых SQL-запросов в service/handler.
- Каждый repository отвечает ровно за одну сущность или агрегат.
- `GoalRepository` зависит от `KRRepository` (инжектируется в конструкторе) для загрузки KR как части goal-агрегата.

## Auth layer

Auth реализован как отдельный слой над роутером:

- `auth.Manager` — инициализирует провайдеры из конфига, управляет логином и сессиями;
- `auth.Provider` — интерфейс провайдера; регистрация через `auth.Register(name, factory)`;
- `auth.PolicyEvaluator` — вычисляет per-request набор доступных team IDs;
- Middleware chain: `AccessLog → Session → (RequireAuth) → (Scope) → CSRF → handlers`.

Провайдеры регистрируются через side-effect imports в `main.go`:

```go
import (
    _ "okrs/internal/auth/providers/google"
    _ "okrs/internal/auth/providers/github"
    _ "okrs/internal/auth/providers/keycloak"
)
```

Добавление нового провайдера = новый пакет + blank import + конфиг. Core-код не меняется.

## Режим без авторизации

При `AUTH_MODE=disabled`:

- весь middleware chain работает, но `RequireAuth` не применяется;
- в контекст запроса инжектируется системный пользователь `anonymous-local` с активной ролью тенанта `admin` (полный доступ в рамках тенанта #1);
- `/admin/*` доступен всем;
- комментарии пишутся от `anonymous-local`.

## OSS / SaaS split

Коробка (`okrs`, public) самодостаточна и мультитенантна. Расширяется через registry-seam'ы,
выбираемые по имени в `app.Config`:

- `auth.RegisterResolveStrategy(name, factory)` — стратегии резолва тенанта (OSS: `session`;
  премиум `subdomain` — Фаза 1, регистрируется blank-import'ом, не трогая core);
- `entitlements.Register(name, factory)` — реализация `Entitlements` (OSS: `unlimited`);
- `nomembership.Register(name, handler)` — no-membership-страница (OSS: `stub`); пакет назывался `onboarding`, переименован, чтобы освободить имя для сервиса онбординга. Ключ `"stub"` и `Options.NoMembershipName` не менялись — встраивающему репозиторию нужен только новый import path;
- `auth.Register(name, factory)` — OAuth-провайдеры (blank-import).

Приватный `okrs-saas` импортирует `okrs/app`, blank-import'ит пакеты с SaaS-регистрациями,
выбирает их по имени в `Config` и монтирует control-plane роуты в **один процесс** через
`PublicRoutes`/`AuthedRoutes`/`TenantRoutes` (по одному на middleware-уровень: вебхуки без auth;
self-service создание организации под auth-без-membership; биллинг-UI под membership-гейтом) —
общая сессия с трекером. Биллинг/Stripe-данные — в собственной БД `okrs-saas`, не в схеме
коробки; результат отражается через provisioning (`PUT /api/v1/system/.../entitlements`). Схема
коробки не содержит SaaS-понятий; секреты (provisioning-token, DSN) — в env. Лендинг —
отдельный репо `okrs-landing` (статика).

## Жёсткие правила для AI-реализации

1. Никакой бизнес-логики в handlers.
2. Никаких schema changes без новой migration.
3. Новые внешние API — только под `/api/v1`.
4. Все вычисления прогресса должны оставаться в одном месте.
5. Изменения UI не должны требовать JS bundler/toolchain.
6. Существующие URL и сценарии не ломать без явной migration spec.
7. Любые state-changing HTTP действия (POST/PUT/PATCH/DELETE), вызываемые из браузера, должны проходить CSRF-проверку.
8. Любые пользовательские данные в UI должны рендериться через безопасное экранирование (без raw HTML-вставок).
9. Auth-данные не должны размазываться по handlers — только через middleware и context helpers.
10. Провайдер-специфичный код не должен появляться за пределами `internal/auth/providers/{name}`.

## Definition of done для любой фичи

Фича считается завершённой только если:

- обновлена спека;
- добавлена миграция, если менялась БД;
- обновлён store/service;
- добавлены тесты на расчёты и/или обработчики;
- обновлён README/API section, если менялся контракт.
