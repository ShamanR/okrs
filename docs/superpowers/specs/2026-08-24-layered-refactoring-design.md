# Слоистый рефакторинг: handlers по URI, распил service, слой usecase — дизайн

**Дата:** 2026-08-24
**Статус:** утверждён, реализация не начата
**Область:** структура кода. Внешний контракт (`specs/040-api-contract.md`) не меняется ни на одном этапе — ни один URI, метод, код ответа или формат тела не затрагивается.

---

## 1. Задача

Пять целей, поставленных заказчиком:

1. Каждый handler-метод — в отдельный модуль, путь модуля отражает URI запроса (навигация + изоляция тестов).
2. Распил `internal/service` на отдельные сервисы по логическим сущностям (устранить взаимное протекание моделей).
3. Выделить слой `usecase` — основные бизнес-сценарии.
4. Перенести статический контент в `/web/templates/` и `/web/static/`.
5. Убрать из корня `/internal` пакеты, которым там не место: `domain`, `okr`, `entitlements`, `export`, `onboarding`.

---

## 2. Исходное состояние

| Что | Факт |
|---|---|
| Handlers | 117 методов в 18 файлах. Разбиение по ресурсам частично есть, но внутри пакета всё в одном `handler.go`: `goals/handler.go` — 622 строки, `admin/` — 600 + 613, `system/handler.go` — 529 |
| Service | `internal/service/service.go` — **1865 строк**, ~90 методов по 7 сущностям вперемешку. Отдельно уже вынесены `SettingsService`, `ProvisioningService`, `OnboardingService`, `HealthCheckInCache` |
| Usecase | Слоя нет. `GetTeamOKR`, `CopyGoal`, `DeleteGoal`, `ExportOKR`, `PeriodOverview` лежат рядом с CRUD-однострочниками вида `ListTeams` |
| Статика | `internal/http/templates/*.html` (embed) + `internal/web/static/` (с диска, `http.Dir("internal/web/static")` — путь относительно cwd) |
| server.go | 704 строки, из них ~200 — чистая бизнес-логика (`hcLoader`, петля progress-снапшотов) |
| Корень `/internal` | `domain`, `okr`, `entitlements`, `export`, `onboarding`, `auth`, `http`, `service`, `store`, `web` |

### 2.1. Mismatch со спеками, обнаруженный при анализе

1. **`specs/010-architecture-constraints.md`, раздел «Repository layer»** — таблица описывает `internal/store/teams.go`, `goals.go`, `periods.go` и т.д. Фактически store давно разбит на 18 подпакетов-репозиториев плюс `testutil` (`internal/store/teams/teams.go`, …), а перечень полей `store.Store` в тексте под таблицей отстал на восемь полей. Спека не соответствует коду; чиним в этапе B, где и так правится соседний раздел «Слои».
2. **Мёртвый код.** Пакет `internal/http/handlers/web/keyresults` (250 строк, 5 handler-методов) не зарегистрирован ни в одном роутере. В `internal/http/handlers/web/goals` из 5 методов подключён только `HandleDeleteGoal`. Спека 010 перечисляет эти web-хендлеры как живые. Удаляем в этапе E.
3. **Нарушение правила 1 спеки 010** («Никакой бизнес-логики в handlers») — `hcLoader` собирает `PeriodData` из пяти репозиториев прямо в `server.go`, там же живут петля снапшотов, advisory-lock и `daysBetween`. Выносим в этапе D.

---

## 3. Целевое дерево

```
/web/
  web.go              package web — //go:embed templates/*.html → TemplatesFS
  templates/*.html    ← internal/http/templates
  static/…            ← internal/web/static, отдаётся http.Dir("web/static")

/app/                 без изменений (фасад)
/cmd/server/          без изменений

/internal/
  core/
    domain/           ← internal/domain, + errors.go (см. §7)
    progress/         ← internal/okr
  platform/
    entitlements/     ← internal/entitlements
    nomembership/     ← internal/onboarding
  render/
    export/           ← internal/export
  auth/               без изменений
  store/              без изменений
  service/            §5
  usecase/            §6
  scheduler/          §8
  http/
    server.go  dto/  middleware/  handlers/…   §9
```

### 3.1. Почему `platform/nomembership`, а не `platform/onboarding`

После распила (§5) появляется `service/onboarding` — инвайты, access-requests, join-requests. Два пакета `onboarding` в одном дереве означают постоянные алиасы и путаницу на ревью. Текущий `internal/onboarding` — это буквально реестр `NoMembershipHandler` (`Register`/`Get` + `StubHandler`), обслуживающий одну страницу `/no-access`. Имя `nomembership` описывает содержимое точно.

Затрагивает OSS/SaaS-сейм: `onboarding.Register("stub", …)` → `nomembership.Register("stub", …)`. Имя строкового ключа (`"stub"`) и поле `Options.NoMembershipName` не меняются, поэтому конфигурация приватного `okrs-saas` остаётся валидной — по описанию сейма в спеке 010 должен потребоваться только новый import path в его blank-import'е. Репозиторий `okrs-saas` лежит вне этого дерева, проверить это утверждение отсюда нельзя. Фиксируем в спеке 010 как breaking change для встраивающего репозитория.

### 3.2. Правило именования пакетов

Store — множественное число, service и usecase — единственное. Коллизии разрешаются алиасом **на месте импорта**, а не переименованием каталога:

```go
import (
    goals   "okrs/internal/store/goals"
    goalsvc "okrs/internal/service/goal"
    goaluc  "okrs/internal/usecase/goal"
)
```

Алиасы `<entity>svc` / `<entity>uc` обязательны и единообразны во всём дереве.

---

## 4. `/web` и публичные пакеты

`//go:embed` не может выходить за пределы каталога своего пакета — `//go:embed ../../web/templates/*.html` даёт `invalid pattern syntax` (проверено). Поэтому якорь embed обязан лежать внутри `/web/`:

```go
// Package web embeds the SSR shell templates. Static assets (/web/static)
// are served from disk and are deliberately NOT embedded.
package web

import "embed"

//go:embed templates/*.html
var TemplatesFS embed.FS
```

Следствие: `/web` лежит вне `internal/`, поэтому `package web` становится **вторым публичным пакетом** модуля `okrs`. Спека 010 сейчас утверждает «`app` … Единственный публичный пакет; всё остальное — `internal/`» — формулировка меняется на:

> Два публичных пакета: `app` — фасад приложения, `web` — SSR-ассеты (только `embed.FS`, без логики). Всё остальное — `internal/`.

Решение принято осознанно: цена — одна фраза в спеке, выгода — сохранение compile-time гарантии на наличие шаблонов, которая есть сегодня.

Статика (`/web/static`) **не** встраивается и по-прежнему отдаётся с диска: правки JS/CSS видны после обновления страницы без пересборки, что критично для фронтенд-цикла без бандлера. `http.Dir("internal/web/static")` → `http.Dir("web/static")`; `Dockerfile` меняет `COPY internal/web /app/internal/web` на `COPY web /app/web`.

---

## 5. Слой service

**Определение:** операции над **одной** сущностью через **один** репозиторий. Валидация полей, CRUD, сортировка. Сервис не читает чужие агрегаты и не пишет в журнал активности.

| Пакет | Источник | Содержимое |
|---|---|---|
| `service/team` | service.go | `List`, `ListAll`, `ListDeleted`, `Get`, `Create`, `Update`, `SoftDelete`, `Restore`, `HardDelete`, `buildTeamHierarchy`, `buildTeamNode` |
| `service/goal` | service.go | `Get`, `ListByTeamPeriod`, `Create`, `Update`, `UpdateFields`, `UpdateWeight`, `Move`, комментарии (`AddComment`, `AddReply`, `ListComments`, `DeleteComment`, `SetCommentResolved`) |
| `service/keyresult` | service.go | `Get`, `CreateWithMeta`, `UpdateWithMeta`, `Delete`, `Move`, `UpsertNote`, `UpdateDescription`, `applyMeta`, `FindGoalIDByKR`, `FindGoalIDByStage` |
| `service/period` | service.go | `List`, `ListViews`, `Get`, `Create`, `Update`, `Delete`, `FindForDate` |
| `service/goalshare` | service.go | `Get`, `List`, `Delete` |
| `service/goallink` | goal_links.go | `ListForGoals`, `ListLinkable`, `Attach` |
| `service/teamstatus` | service.go | `Get`, `Set` |
| `service/user` | service.go | `GetByDisplayNames`, `GetByUDIDs`, `ListUserLeadTeams`, `ValidateUDIDsExist`, `SearchInScope` |
| `service/activity` | service.go | `Record`, `List`, `TreeCounts`, `CategoryCounts`, `Purge` |
| `service/settings` | settings.go | без изменений в логике |
| `service/provisioning` | provisioning.go | без изменений в логике |
| `service/onboarding` | onboarding.go | без изменений в логике |
| `service/healthcheckin` | healthcheckin*.go | конфиг + кэш; загрузка `PeriodData` уезжает в usecase (§6) |
| `service/servicetest` | *_test.go | общие fake-репозитории для тестов всех сервисов и usecase |

Тесты разносятся за кодом: `service_test.go` (956 строк), `goal_test.go` (660), `healthcheckin_test.go` (609) распадаются по пакетам своих сервисов. Логика тестов не меняется — меняется только расположение и импорт fakes из `servicetest`.

---

## 6. Слой usecase

**Определение:** оркестрация ≥2 сервисов, инварианты между сущностями, запись в журнал активности как часть сценария, сборка read-model из нескольких источников.

| Пакет | Сценарии |
|---|---|
| `usecase/okrboard` | `GetTeamOKR`, `GetTeamOverview`, `GetDirectChildrenSummary`, `GetTeamsWithPeriodSummary`, `GetHierarchy` |
| `usecase/goal` | `Copy`, `Delete`, `Share`, `Transfer` (`UpdateGoalOwnerAndShares`), `SetParents` (проверка цикла), `Update` с записью activity, `resetStatusIfNoGoals` |
| `usecase/keyresult` | `UpdateProgressNumerical`/`Boolean`/`Project` (пересчёт цели + activity + auto-complete health), `UpdateHealthStatus`, `Delete` |
| `usecase/period` | `Overview`, `OverviewScoped`, `BulkSetTeamPeriodStatus`, `Archive`, `Unarchive`, `Stats` |
| `usecase/team` | `Delete` (guard `ErrTeamHasGoals`) |
| `usecase/goaltree` | `GoalTree` |
| `usecase/healthcheckin` | сценарий расчёта + `LoadPeriodData` (бывший `hcLoader` из server.go) |
| `usecase/export` | `ExportOKR` и его помощники |
| `usecase/progresssnapshot` | `SnapshotActivePeriods`, отбор due-периодов |

Read-model типы (`TeamOKR`, `TeamSummary`, `TeamNode`, `TeamOverview`, `TeamChildSummary`, `GoalDetails`, `GoalTreeData`, `PeriodData`, `PeriodOverview`, `HealthCheckInResult`) переезжают в тот usecase-пакет, который их производит. Handler маппит их в `internal/http/dto`.

### 6.1. Одноимённые методы в service и usecase

`service/goal.Update` и `usecase/goal.Update` — не дубль, а два уровня одной операции: сервис пишет поля цели в свой репозиторий, usecase вызывает сервис и добавляет то, что выходит за границу сущности (запись в журнал активности, пересчёт статуса команды, инварианты периода). То же для `service/team.SoftDelete` ↔ `usecase/team.Delete` и `service/keyresult.*` ↔ `usecase/keyresult.*`.

Правило для handler'а: если для URI существует usecase — handler зовёт **только** его и никогда не зовёт сервис напрямую в обход. Сервис вызывается напрямую лишь там, где usecase не заведён (чистый read или CRUD одной сущности).

---

## 7. Ошибки

Сегодня 15+ `Err*` объявлены в `internal/service` и на них напрямую смотрят handlers при маппинге в HTTP-коды. После распила они окажутся размазаны по 13 пакетам, а handler'у придётся импортировать половину из них.

Доменные инварианты переезжают в `internal/core/domain/errors.go` — один источник для service, usecase и handler, без циклов импорта:

`ErrTeamHasGoals`, `ErrTeamNotVisibleInPeriod`, `ErrPeriodClosed`, `ErrPeriodNotClosed`, `ErrCannotShareWithClosedPeriod`, `ErrShareTargetNotInTenant`, `ErrTransferTargetSameAsSource`, `ErrTransferTargetNotFound`, `ErrForbidden`, `ErrGoalNotOnTeamBoard`, `ErrGoalLinkSelf`, `ErrGoalLinkNotAccessible`, `ErrGoalLinkCycle`.

Ошибки, специфичные для одного сервиса и не пересекающие его границу (`ErrLastAdmin`, `ErrSelfLockout`, `ErrLastSystemAdmin`, `ErrAlreadyMember`, `ErrTenantNotFound`), остаются в своих пакетах (`service/provisioning`, `service/onboarding`).

---

## 8. `internal/scheduler`

Из `server.go` выносятся: `startProgressSnapshotLoop`, `snapshotDuePeriods`, `daysBetween`, `progressSnapshotLockKey`, `snapshotCheckInterval`, `activePeriods`, `hcLoader`.

- **бизнес-логика** (что считать, какие периоды due) → `usecase/progresssnapshot` и `usecase/healthcheckin`;
- **механика** (тикеры, `pg_try_advisory_lock`, graceful stop по `ctx`) → `internal/scheduler`;
- **запуск** → `app.New`, а не `Server.Routes()`.

Сейчас фоновые петли стартуют побочным эффектом вызова `Routes()`, что делает построение роутера непригодным для изолированного теста без запуска goroutine. После выноса `Routes()` становится чистой функцией сборки.

`server.go` худеет с 704 до ~250 строк.

---

## 9. Handlers: пакет на URI-сегмент

### 9.1. Правила

1. **Пакет = сегмент URI.** Путь пакета повторяет путь запроса.
2. **Файл = HTTP-метод** на этом URI: `get.go`, `post.go`, `put.go`, `patch.go`, `delete.go`. Рядом — `<method>_test.go`.
3. **Path-параметр не создаёт пакет.** `{goalID}`, `{commentID}`, `{id}` схлопываются в родительский пакет — иначе получаем `goals/goalid/comments/commentid/resolve`.
4. **Дефис убирается:** `move-up` → `moveup`, `key-results` → `keyresults`, `tree-counts` → `treecounts`, `access-requests` → `accessrequests`, `system-admin` → `systemadmin`.
5. **Состав пакета:** `handler.go` (тип `Handler` + `New` с зависимостями **только этого** пакета), `routes.go` (`RegisterRoutes` монтирует свои эндпоинты и вызывает `RegisterRoutes` подпакетов), файлы методов.
6. Пакет, у которого нет собственных эндпоинтов (только подпакеты), содержит один `routes.go`.

### 9.2. Пример: `/api/v1/goals`

```
handlers/api/v1/goals/
  handler.go  routes.go
  get.go post.go delete.go            /api/v1/goals/{goalID}
  linkable/get.go                     /api/v1/goals/linkable
  links/post.go                       …/{goalID}/links
  share/post.go  share/delete.go      …/{goalID}/share[/{teamID}]
  transfer/post.go
  weight/post.go
  comments/post.go  comments/delete.go
  comments/replies/post.go
  comments/resolve/post.go
  comments/unresolve/post.go
  moveup/post.go  movedown/post.go
  keyresults/post.go                  …/{goalID}/key-results
```

Последняя строка — иллюстрация выигрыша: `POST /api/v1/goals/{goalID}/key-results` сегодня лежит в пакете `krs`, найти его по URI невозможно.

Попутно чинится опечатка: пакет `hierarhy`, обслуживающий `/api/v1/hierarchy`, становится `hierarchy`.

### 9.3. Исключение: SPA-shell'ы

12 роутов, отдающих 6 shell-шаблонов, — это не handler-логика, а таблица «URI → имя шаблона → гейт». Сегодня это closures в `server.go`:

| Шаблон | URI |
|---|---|
| `tracker-shell` | `/`, `/teams/{teamID}/okr` |
| `settings-shell` | `/settings` |
| `period-overview-shell` | `/period-overview` |
| `goal-tree-shell` | `/goal-tree` |
| `activity-shell` | `/activity-log` |
| `admin-shell` | `/admin`, `/admin/access`, `/admin/teams`, `/admin/periods`, `/admin/health-checkin` |
| `system-shell` | `/system` |

Они выносятся в `handlers/web/shell/` — декларативная таблица + один тест, проходящий по всем URI. Плодить 12 пакетов с однострочным `ExecuteTemplate` в каждом — шум без выигрыша в навигации. Исключение фиксируется в спеке 070 явно, чтобы не читалось как недоделка.

Туда же — legacy-редиректы (`/teams`, `/periods`, `/teamOkrs`, `/admin/teams/new`, `/admin/teams/{teamID}/edit`, `/admin/periods/{periodID}/edit`, `/admin/users/{userID}`): таблица «URI → target».

**`/no-access` в таблицу shell'ов не входит.** Это не `ExecuteTemplate`, а вызов через реестр: `nomembership.Get(name).ServeNoMembership(w, r)` с 500-й при незарегистрированном хендлере, плюс OSS-`StubHandler` подмешивает в шаблон `no_access_message` из системных настроек. Живая логика и точка расширения OSS/SaaS — получает собственный пакет `handlers/web/noaccess/get.go` по общему правилу 9.1.

Все остальные web-хендлеры (`/login`, `/auth/{provider}/start`, `/auth/{provider}/callback`, `/invite/{token}`, `/logout`, `/goals/{goalID}/delete`) раскладываются по общему правилу 9.1.

### 9.4. Карта URI → пакет

Полная карта всех 117 эндпоинтов — в `specs/070-code-structure.md` (создаётся в этапе E). Верхний уровень:

```
handlers/
  api/v1/
    me/            config/       users/        hierarchy/
    activity/      + category counts, tree counts
    periods/       + {id}/overview, {id}/teams/{activate,close}
    teams/         + {id}/{okrs,overview,export,goals,status}
    goals/         см. 9.2
    goaltree/      krs/          healthcheckin/
    onboarding/joinrequest/
    session/       tenants/, tenant/, memberships/
    admin/         users/, settings/, periods/, teams/,
                   invitations/, accessrequests/, members/, activity/
    system/        tenants/, users/, settings/
  web/
    shell/         таблица SPA-shell'ов + legacy-редиректов
    login/  logout/  auth/  invite/  goals/delete/
```

---

## 10. Этапы

Каждый этап — отдельный change set с чекпоинтом. Приёмка каждого: `go build ./...`, `go vet ./...`, `go test ./...` — зелёные.

Baseline, снятый 2026-08-24 перед началом работ: `go test ./...` → **exit 0**; 55 пакетов, из них 46 с тестами — все `ok`, 9 без тестов, 0 `FAIL`. Прогон включает интеграционные тесты на testcontainers (нужен работающий Docker).

| | Этап | Объём | Дополнительная приёмка |
|---|---|---|---|
| **A** | `/web/{templates,static}` + `package web`, Dockerfile | ~15 файлов | страницы открываются, статика отдаётся |
| **B** | `core/`, `platform/`, `render/`; `onboarding` → `nomembership` | ~55 файлов (импорты) | — |
| **C** | распил `service.go` на 13 пакетов + `servicetest` | ~30 файлов | `service.go` не существует |
| **D** | `usecase/` + `scheduler/`; `server.go` → ~250 строк | ~35 файлов | `Routes()` не запускает goroutine |
| **E** | handlers по URI-пакетам, тесты рядом, удаление мёртвого кода | ~120 файлов | карта роутов до/после идентична |

**Порядок обязателен:** D зависит от C (usecase собирается из сервисов), E зависит от C+D (меняются конструкторы handler'ов). A и B независимы, но идут первыми как самые дешёвые.

**Инвариант всех этапов:** множество зарегистрированных роутов (метод + путь) не меняется. Перед этапом E снимается эталонный дамп роутов; после — сверяется побайтово.

---

## 11. Обновление спек

В том же change set, что и соответствующий этап:

| Спека | Этап | Что меняется |
|---|---|---|
| `specs/010-architecture-constraints.md` | A, B, C, D, E | Раздел «Слои» переписывается целиком. Чинится таблица «Repository layer» (§2.1.1). Формулировка о публичных пакетах (§4). Правила именования пакетов и алиасов (§3.2). Границы service/usecase (§5, §6). Из перечня живых web-хендлеров убирается удалённый мёртвый код. Пути статики. |
| `specs/040-api-contract.md` | B | Единственная ссылка на путь `internal/*` — строка 914, «пакет `internal/export`» → `internal/render/export`. Контракт эндпоинтов не затрагивается |
| `specs/070-code-structure.md` | E | **Новая.** Полная карта «URI → пакет handler'а» (117 эндпоинтов), карта service/usecase, правила разложения для новых эндпоинтов, явное исключение для SPA-shell'ов |
| `README-specs.md` | E | Структура каталога `/specs` |

Спеки `000`, `020`, `030`, `050`, `060` не затрагиваются — они не содержат ссылок на пути файлов.

---

## 12. Что осознанно НЕ делается

- **Не меняется внешний контракт.** Ни один URI, метод, статус-код или формат тела.
- **Не трогаются `internal/auth` и `internal/store`** — они уже разложены корректно.
- **Не переезжает `internal/http`** в `internal/adapter/http` — за рамками задачи.
- **Не пишутся новые тесты «про запас».** Существующие переносятся за кодом; новые добавляются только там, где разделение вскрывает непокрытый публичный метод.
- **Не сливаются две реализации `UserSelector`** (`tracker.js` / `admin.js`) — известный техдолг фронтенда, не относится к этой задаче.
- **Не делаются git-коммиты** (CLAUDE.md #8) — коммитит заказчик.
