# 070. Структура кода

Статус: актуально. Источник правды по раскладке пакетов и по соответствию
«URI → пакет обработчика». Правила архитектурных слоёв — в
[010-architecture-constraints.md](010-architecture-constraints.md); эта спека
отвечает на вопрос «где физически лежит код», а не «кому что можно вызывать».

## 1. Верхний уровень

```
app/                 точка сборки: конфиг, стор, auth, сервер, фоновые задачи
web/                 templates/ и static/ + embed для шаблонов
internal/
  core/              домен без зависимостей: domain (сущности, ошибки), progress (расчёт)
  platform/          инфраструктурные мелочи: entitlements, nomembership
  render/            представление, не привязанное к HTTP: export
  store/             репозитории и кэши, по пакету на агрегат
  service/           операции над одной сущностью, по пакету на сущность
  usecase/           бизнес-сценарии над несколькими сущностями
  scheduler/         фоновые проходы (снапшоты прогресса, прогрев кэша)
  http/              сервер, сборка зависимостей, DTO ответов, обработчики
```

Публичных пакетов ровно два: `app` и `web`. Всё остальное — под `internal/`.

Внутри `http/` кроме `handlers/` лежат `httpdeps/` (сборка графа сервисов и
usecase), `middleware/` и `dto/` — структуры JSON-ответов с их тегами. `dto`
вынесен из обработчиков потому, что одну и ту же форму ответа собирают разные
пакеты (`teamscommon`, `goalcommon`, `activitycommon`), и держать её копии рядом
с каждым обработчиком означало бы разъезжающийся контракт.

## 2. Слои `service` и `usecase`

`service/<сущность>` — операции над одной сущностью, ходит в свой репозиторий:

```
service/  activity  goal  goallink  goalshare  healthcheckin  keyresult
          onboarding  period  progresssnap  provisioning  settings  team
          teamstatus  user
          servicetest/  — фейки стора для тестов сервисов
```

`usecase/<сценарий>` — сценарий поверх нескольких сервисов, **в репозитории не
ходит**:

```
usecase/  export  goal  goaltree  healthcheckin  keyresult  okrboard  period  user
```

Правила именования при импорте: `<сущность>svc` для сервиса, `<сценарий>uc` для
usecase — `goalsvc`, `perioduc`. Пакеты стора берут множественное число
(`store/goals`), сервисы — единственное (`service/goal`), чтобы `goals` и `goal`
в одном файле не путались.

Фасада `service.Service`, через который раньше ходили все обработчики, больше
нет: каждый пакет получает ровно те зависимости, которые использует.

## 3. Обработчик на URI

**Один URI — один пакет.** Путь пакета повторяет путь URI:

- префикс группы: `/api/v1/...` → `internal/http/handlers/api/v1/...`,
  страницы SSR → `internal/http/handlers/web/...`;
- сегменты-параметры (`{goalID}`) выбрасываются;
- дефисы убираются: `/key-results` → `keyresults`, `/health-checkin` →
  `healthcheckin`.

Все методы одного URI живут в одном пакете и называются по глаголу: `Get`,
`Post`, `Patch`, `Delete`.

**Имена файлов внутри пакета фиксированы:**

| Файл | Что в нём |
|---|---|
| `handler.go` | `type Handler`, `New`, порты, методы-глаголы |
| `routes.go` | только `RegisterRoutes(r chi.Router, h *Handler)` |
| `handler_test.go` | тесты обработчика |
| `response.go` | сборка DTO, если она не помещается в `handler.go` |

Файл **не** называется по имени пакета (`treecounts.go`, `periods.go`): такое имя
ничего не добавляет — пакет уже назван по URI, — а при навигации по десяткам
пакетов глаз ищет одинаковую строку, а не разную. Дополнительные тесты по темам
(`access_test.go`, `links_test.go`) допустимы, когда одного файла мало.

Исключения перечислены в §7 и только там; для всего остального соответствие
проверяется по таблице в §6.

## 4. Общий код внутри группы

Родительский пакет монтирует подпакеты, поэтому подпакет не может импортировать
родителя — иначе цикл. Общий для группы код выносится в **лист-пакет**, который
никто не монтирует:

```
api/v1/goals/goalcommon        api/v1/krs/krscommon
api/v1/teams/teamscommon       api/v1/activity/activitycommon
api/v1/admin/admincommon       api/v1/system/systemcommon
api/v1/onboarding/onboardingcommon
api/v1/admin/periods/teams/bulkstatus
web/common                     web/auth
```

У каждого такого пакета в doc-комментарии написано, почему он лист — чтобы его
не «починили» обратно в родителя.

## 5. Порты объявляются на стороне потребителя

Пакет обработчика описывает интерфейс с ровно теми методами, которые вызывает, и
принимает его в `New`. Реализация (сервис или usecase) про этот интерфейс не
знает. Когда методы приходят из разных источников, вместо одного «широкого» порта
используется структура из нескольких узких — `goalcommon.MoveDeps`,
`goalcommon.ResolveDeps`, `krscommon.MoveDeps`.

## 6. Карта URI → пакет

Пути пакетов даны относительно `internal/http/handlers/`. Стрелка `↑` означает
«тот же пакет, что строкой выше».

| URI | Методы | Пакет |
|---|---|---|
| `/api/v1/activity` | GET | `api/v1/activity` |
| `/api/v1/activity/category-counts` | GET | `api/v1/activity/categorycounts` |
| `/api/v1/activity/tree-counts` | GET | `api/v1/activity/treecounts` |
| `/api/v1/admin/access-requests` | GET | `api/v1/admin/accessrequests` |
| `/api/v1/admin/access-requests/{userID}/approve` | POST | `api/v1/admin/accessrequests/approve` |
| `/api/v1/admin/access-requests/{userID}/deny` | POST | `api/v1/admin/accessrequests/deny` |
| `/api/v1/admin/activity/purge` | POST | `api/v1/admin/activity/purge` |
| `/api/v1/admin/invitations` | GET POST | `api/v1/admin/invitations` |
| `/api/v1/admin/invitations/{id}/revoke` | POST | `api/v1/admin/invitations/revoke` |
| `/api/v1/admin/members/{userID}` | DELETE | `api/v1/admin/members` |
| `/api/v1/admin/periods` | GET POST | `api/v1/admin/periods` |
| `/api/v1/admin/periods/{periodID}` | PATCH DELETE | ↑ |
| `/api/v1/admin/periods/{periodID}/archive` | POST | `api/v1/admin/periods/archive` |
| `/api/v1/admin/periods/{periodID}/overview` | GET | `api/v1/admin/periods/overview` |
| `/api/v1/admin/periods/stats` | GET | `api/v1/admin/periods/stats` |
| `/api/v1/admin/periods/{periodID}/teams/activate` | POST | `api/v1/admin/periods/teams/activate` |
| `/api/v1/admin/periods/{periodID}/teams/close` | POST | `api/v1/admin/periods/teams/close` |
| `/api/v1/admin/periods/{periodID}/unarchive` | POST | `api/v1/admin/periods/unarchive` |
| `/api/v1/admin/settings/access` | GET POST | `api/v1/admin/settings/access` |
| `/api/v1/admin/settings/feedback` | GET POST | `api/v1/admin/settings/feedback` |
| `/api/v1/admin/settings/general` | GET POST | `api/v1/admin/settings/general` |
| `/api/v1/admin/settings/health-checkin` | GET POST | `api/v1/admin/settings/healthcheckin` |
| `/api/v1/admin/teams` | GET POST | `api/v1/admin/teams` |
| `/api/v1/admin/teams/{teamID}` | PATCH DELETE | ↑ |
| `/api/v1/admin/teams/{teamID}/hard` | DELETE | `api/v1/admin/teams/hard` |
| `/api/v1/admin/teams/{teamID}/restore` | POST | `api/v1/admin/teams/restore` |
| `/api/v1/admin/users` | GET | `api/v1/admin/users` |
| `/api/v1/admin/users/{userID}` | GET | ↑ |
| `/api/v1/admin/users/{userID}/admin` | POST DELETE | `api/v1/admin/users/admin` |
| `/api/v1/admin/users/{userID}/grants` | GET POST | `api/v1/admin/users/grants` |
| `/api/v1/admin/users/{userID}/grants/{teamID}` | DELETE | ↑ |
| `/api/v1/config` | GET | `api/v1/config` |
| `/api/v1/goals/{goalID}` | GET POST DELETE | `api/v1/goals` |
| `/api/v1/goals/{goalID}/comments` | POST | `api/v1/goals/comments` |
| `/api/v1/goals/{goalID}/comments/{commentID}` | DELETE | ↑ |
| `/api/v1/goals/{goalID}/comments/{commentID}/replies` | POST | `api/v1/goals/comments/replies` |
| `/api/v1/goals/{goalID}/comments/{commentID}/resolve` | POST | `api/v1/goals/comments/resolve` |
| `/api/v1/goals/{goalID}/comments/{commentID}/unresolve` | POST | `api/v1/goals/comments/unresolve` |
| `/api/v1/goals/{goalID}/key-results` | POST | `api/v1/goals/keyresults` |
| `/api/v1/goals/linkable` | GET | `api/v1/goals/linkable` |
| `/api/v1/goals/{goalID}/links` | POST | `api/v1/goals/links` |
| `/api/v1/goals/{goalID}/move-down` | POST | `api/v1/goals/movedown` |
| `/api/v1/goals/{goalID}/move-up` | POST | `api/v1/goals/moveup` |
| `/api/v1/goals/{goalID}/share` | POST | `api/v1/goals/share` |
| `/api/v1/goals/{goalID}/share/{teamID}` | DELETE | ↑ |
| `/api/v1/goals/{goalID}/transfer` | POST | `api/v1/goals/transfer` |
| `/api/v1/goals/{goalID}/weight` | POST | `api/v1/goals/weight` |
| `/api/v1/goal-tree` | GET | `api/v1/goaltree` |
| `/api/v1/health-checkin` | GET | `api/v1/healthcheckin` |
| `/api/v1/hierarchy` | GET | `api/v1/hierarchy` |
| `/api/v1/krs/{krID}` | POST DELETE | `api/v1/krs` |
| `/api/v1/krs/{krID}/description` | POST | `api/v1/krs/description` |
| `/api/v1/krs/{krID}/move-down` | POST | `api/v1/krs/movedown` |
| `/api/v1/krs/{krID}/move-up` | POST | `api/v1/krs/moveup` |
| `/api/v1/krs/{krID}/note` | POST | `api/v1/krs/note` |
| `/api/v1/krs/{krID}/progress/boolean` | POST | `api/v1/krs/progress/boolean` |
| `/api/v1/krs/{krID}/progress/numerical` | POST | `api/v1/krs/progress/numerical` |
| `/api/v1/krs/{krID}/progress/project` | POST | `api/v1/krs/progress/project` |
| `/api/v1/me` | GET | `api/v1/me` |
| `/api/v1/onboarding/join-request` | POST | `api/v1/onboarding/joinrequest` |
| `/api/v1/periods` | GET | `api/v1/periods` |
| `/api/v1/periods/{periodID}/overview` | GET | `api/v1/periods/overview` |
| `/api/v1/periods/{periodID}/teams/activate` | POST | `api/v1/periods/teams/activate` |
| `/api/v1/periods/{periodID}/teams/close` | POST | `api/v1/periods/teams/close` |
| `/api/v1/session/memberships` | GET | `api/v1/session/memberships` |
| `/api/v1/session/memberships/{tenantID}` | DELETE | ↑ |
| `/api/v1/session/tenant` | POST | `api/v1/session/tenant` |
| `/api/v1/session/tenants` | GET | `api/v1/session/tenants` |
| `/api/v1/system/settings` | GET | `api/v1/system/settings` |
| `/api/v1/system/settings/default-registration-tenant` | PUT | `api/v1/system/settings/defaultregistrationtenant` |
| `/api/v1/system/settings/no-access-message` | PUT | `api/v1/system/settings/noaccessmessage` |
| `/api/v1/system/tenants` | GET POST | `api/v1/system/tenants` |
| `/api/v1/system/tenants/{id}` | PATCH | ↑ |
| `/api/v1/system/tenants/{id}/activity/purge` | POST | `api/v1/system/tenants/activity/purge` |
| `/api/v1/system/tenants/{id}/entitlements` | GET PUT | `api/v1/system/tenants/entitlements` |
| `/api/v1/system/tenants/{id}/members` | GET POST | `api/v1/system/tenants/members` |
| `/api/v1/system/tenants/{id}/members/{userID}` | DELETE | ↑ |
| `/api/v1/system/tenants/{id}/members/{userID}/deny` | POST | `api/v1/system/tenants/members/deny` |
| `/api/v1/system/tenants/{id}/members/{userID}/role` | PUT | `api/v1/system/tenants/members/role` |
| `/api/v1/system/tenants/{id}/restore` | POST | `api/v1/system/tenants/restore` |
| `/api/v1/system/tenants/{id}/suspend` | POST | `api/v1/system/tenants/suspend` |
| `/api/v1/system/users` | GET | `api/v1/system/users` |
| `/api/v1/system/users/{userID}/system-admin` | PUT | `api/v1/system/users/systemadmin` |
| `/api/v1/teams/{teamID}` | GET | `api/v1/teams` |
| `/api/v1/teams/{teamID}/export` | GET | `api/v1/teams/export` |
| `/api/v1/teams/{teamID}/goals` | POST | `api/v1/teams/goals` |
| `/api/v1/teams/{teamID}/okrs` | GET | `api/v1/teams/okrs` |
| `/api/v1/teams/{teamID}/overview` | GET | `api/v1/teams/overview` |
| `/api/v1/teams/{teamID}/status` | POST | `api/v1/teams/status` |
| `/api/v1/users` | GET | `api/v1/users` |
| `/auth/{provider}/callback` | GET | `web/auth/callback` |
| `/auth/{provider}/start` | GET | `web/auth/start` |
| `/goals/{goalID}/delete` | POST | `web/goals/delete` |
| `/invite/{token}` | GET | `web/invite` |
| `/login` | GET | `web/login` |
| `/logout` | POST | `web/logout` |
| `/no-access` | GET | `web/noaccess` |
## 7. Исключения из правила «пакет = URI»

Их три, и других нет. Таблица §6 покрывает всё остальное.

**1. SSR-shell'ы и legacy-редиректы — `web/shell`.** Один пакет обслуживает
девятнадцать URI, потому что за ними нет логики: shell — строка таблицы
«URI → шаблон», редирект — строка таблицы «URI → target». Девятнадцать пакетов
с одним `ExecuteTemplate` каждый были бы шумом без навигационного выигрыша.
Таблицы — экспортируемые переменные `shell.Public`, `shell.TenantAdmin`,
`shell.System`, `shell.PublicRedirects`, `shell.MemberRedirects`,
`shell.AdminRedirects`; в каком middleware-уровне они смонтированы, видно в
`server.go`:

| URI | Что отдаёт | Таблица |
|---|---|---|
| `/`, `/teams/{teamID}/okr` | `tracker-shell` | `shell.Public` |
| `/settings` | `settings-shell` | ↑ |
| `/period-overview` | `period-overview-shell` | ↑ |
| `/goal-tree` | `goal-tree-shell` | ↑ |
| `/admin`, `/admin/access`, `/admin/teams`, `/admin/periods`, `/admin/health-checkin` | `admin-shell` | `shell.TenantAdmin` |
| `/activity-log` | `activity-shell` | ↑ |
| `/system` | `system-shell` | `shell.System` |
| `/teams` → `/admin/teams`, `/periods` → `/admin/periods` | 302 | `shell.PublicRedirects` |
| `/teamOkrs` → `/` (с сохранением query) | 302 | `shell.MemberRedirects` |
| `/admin/teams/new`, `/admin/teams/{teamID}/edit`, `/admin/periods/{periodID}/edit`, `/admin/users/{userID}` → `/admin` | 302 | `shell.AdminRedirects` |

`/no-access` в эту таблицу НЕ входит: он резолвится через реестр
`platform/nomembership` и подставляет настраиваемое сообщение — это живая
логика, поэтому у него свой пакет `web/noaccess` и строка в §6.

**2. `/static/*`** — раздача файлов с диска, доменного обработчика у неё нет;
регистрируется в `server.go` напрямую.

**3. `/login` в режиме `AUTH_MODE=disabled`** — редирект на корень, потому что
выбирать провайдера не из чего; в обычном режиме URI обслуживает `web/login`.

## 8. Тесты обработчиков

У каждого handler-пакета есть тесты — это условие, а не пожелание: пакет на URI
имеет смысл ровно потому, что его можно проверить изолированно.

Общая обвязка живёт в `internal/http/handlers/handlertest`: сборка запроса с
tenant-scope, пользователем, ролью, ограничением видимости и параметрами пути,
плюс проверки кода ответа и конверта ошибки. Без неё эти двадцать строк
копировались бы в каждый из 89 пакетов.

Минимум, который проверяется в каждом пакете:

- **разбор пути** — неразбираемый или неположительный id даёт 400, а не 404/500;
- **разбор тела** — испорченный JSON и нарушенные инварианты полей дают 400.

Плюс **гейт tenant-scope** — но только там, где он есть: без активного tenant
эндпоинт отвечает 403, а не пустым телом и не паникой
(`handlertest.RequiresTenantScope`). Гейта нет и проверять нечего у tenant-less
плоскостей: `/api/v1/system/**` (гейт — `RequireSystemAdmin`), `/api/v1/me`,
`/api/v1/session/**` и SSR-страницы аутентификации. Там, где гейт вынесен в
лист-пакет (`krscommon.TenantScope`, `goalcommon`, `activitycommon`,
`admincommon`), он проверяется в тестах пакетов-потребителей, а не повторно в
каждом.

Открытый долг: пятнадцать пакетов с инлайновым `TenantScopeFromContext` пока не
имеют собственного теста на 403 — `activity` и оба его счётчика,
`admin/activity/purge`, `admin/periods/archive`, `admin/settings/{feedback,general}`,
`admin/users`, `config`, `goaltree`, `hierarchy`, `periods`, `teams`, `users`.

Где обработчик принимает узкие порты, к этому добавляются happy path и маппинг
доменных ошибок в коды (404 против 409 против 500). Пакеты-обёртки над общим
телом (`goals/moveup` ↔ `goals/movedown`, `comments/resolve` ↔ `unresolve`,
`system/tenants/suspend` ↔ `restore`) проверяют главное: что в общее тело уходит
именно та константа, ради которой пакет и заведён.

**Доступ к чужому объекту — 404, а не 403.** Это проверяется явно: 403 сообщал бы
о существовании объекта тому, кто не должен о нём знать, и эндпоинт работал бы
оракулом перебора id.

## 9. Инвариант маршрутов

`internal/http/routes_golden_test.go` обходит собранный роутер через `chi.Walk` и
сравнивает набор пар «метод + шаблон» с `internal/http/testdata/routes.golden`.
Тест не требует базы: `Routes()` — чистая сборка, фоновые задачи вынесены в
`internal/scheduler`.

Любое изменение набора маршрутов роняет этот тест. Если изменение намеренное:

```
go test ./internal/http -run RoutesGolden -update-routes
```

Обновлённый golden идёт в тот же change set, что и правка маршрутов.

Вторым тестом (`internal/http/spec_routes_test.go`) golden сверяется с таблицей §6
и списком исключений §7: маршрут, которого нет ни там, ни там, роняет сборку, как и
строка в спеке без соответствующего маршрута. Без него таблица §6 разъезжается молча
— в первой же редакции в ней не хватало шести живых эндпоинтов.
