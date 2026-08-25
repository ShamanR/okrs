# Слоистый рефакторинг, этап E — handlers по URI и удаление фасада. План реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Разложить 117 handler-методов по пакетам, путь которых повторяет URI запроса; перевязать handlers на usecase и сервисы напрямую; удалить фасад `service.Service` и мёртвый код.

**Architecture:** Пакет = сегмент URI, файл = HTTP-метод. Каждый пакет получает узкий конструктор: ровно те usecase/сервисы, которые нужны его эндпоинтам. Фасад, существовавший ради неизменности handlers на этапах C–D, удаляется — его 79 делегатов и алиасов больше некому обслуживать.

**Tech Stack:** Go 1.25, chi/v5, слои `handler → usecase → service → store`.

**Spec:** `docs/superpowers/specs/2026-08-24-layered-refactoring-design.md`

**Статус: выполнено.** 81 пакет обработчиков, по одному на URI; фасад `service.Service` удалён вместе с файлом `internal/service/service.go` — в корне `internal/service` файлов больше нет. Тесты фасадных пакетов (`admin`, `system`, `onboarding`, `tenants`) разнесены по пакетам-владельцам URI либо переписаны как групповые интеграционные (`system`, `onboarding` монтируют подпакеты так же, как `server.go`). Опечатка `hierarhy` в имени пакета исправлена, из-за чего правило «путь пакета = путь URI» выполняется без исключений. `go build ./...`, `go vet ./...`, `go test ./...` — зелёные; `TestRoutesGolden` подтверждает 142 роута без изменений. Спека `specs/070-code-structure.md` написана, `specs/010` и `README-specs.md` обновлены.

## Global Constraints

- **Не делать `git commit`** — заказчик коммитит сам (CLAUDE.md #8).
- **Baseline:** `go test ./...` → exit 0; 80 пакетов, 60 с тестами все `ok`, 0 `FAIL`.
- **Интеграционные тесты требуют Docker.** Перед прогоном: `docker info`.
- **⚠ Правки «на месте» — `perl -pi -e`, якорить по `\b`, НЕ по `$`.** CRLF в рабочем дереве: `$` не совпадает ни в sed, ни в perl; BSD sed не поддерживает `\b`; `perl -pi -e '… print'` дублирует файл. Все три отказывают молча. `\b` не работает на границе с `_` (`package foo_test`).
- **⚠ Никогда `gofmt -w` по каталогу** — перепишет ~211 файлов на LF. Только по списку изменённых.
- **Внешний контракт неизменен.** Проверяется машинно (§ Инвариант роутов), а не на глаз.
- **Не трогать `docs/superpowers/**`** — исторические документы.
- **TODO(refactoring) не «заодно чинить».** В коде 16 файлов с такими метками, сводка — §13 дизайн-дока. Они отложены осознанно; закрывать их в этом этапе — расширение скоупа.

---

## Инвариант роутов

Множество зарегистрированных роутов не меняется ни на одном шаге. Проверяется **golden-тестом, обходящим реальный роутер**, а не grep'ом по исходникам:

```bash
go test ./internal/http -run RoutesGolden
```

`internal/http/routes_golden_test.go` вызывает `chi.Walk` по собранному роутеру и сравнивает все пары «метод + шаблон» с `internal/http/testdata/routes.golden` (142 строки). Рефреш после осознанной правки контракта:

```bash
go test ./internal/http -run RoutesGolden -update-routes
```

**Почему не grep.** Первая версия инварианта искала `r.Get("/…")` в исходниках. Она сломалась на первой же задаче: shell-роуты переехали в декларативную таблицу, где URI — переменная, и grep молча перестал их видеть — показал «13 роутов исчезли», хотя не исчез ни один. Обход роутера видит ровно то, что будет обслуживать chi, и переживает любой способ регистрации.

Тест не требует БД: после этапа D `Routes()` — чистая сборка (фоновые петли уехали в `internal/scheduler`), поэтому нулевого `store.Store` достаточно.

Валидация самого перехода: все 132 роута из grep-снимка, снятого до правки shell'ов, присутствуют в golden. Сверх них golden содержит `/static/*` (регистрируется `r.Handle` — все HTTP-методы) и `GET /`, которые grep не видел.

---

## Порядок: URI-раскладка и снятие фасада делаются вместе

Сначала я планировал сперва разложить по URI, потом отвязать от фасада. Это означало бы тронуть каждый пакет дважды: сначала перенести метод, потом переписать его зависимость. Вместо этого каждый ресурс обрабатывается за один проход — разложить и сразу дать пакету узкий конструктор с нужными usecase.

Выигрыш от узких зависимостей и есть смысл URI-раскладки: пакет `goals/share` должен зависеть от `goaluc.UseCase`, а не от богоподобного `*service.Service`.

---

## Правила раскладки

1. **Пакет = сегмент URI**, путь пакета повторяет путь запроса.
2. **Файл = HTTP-метод**: `get.go`, `post.go`, `put.go`, `patch.go`, `delete.go`; рядом `<method>_test.go`.
3. **Path-параметр не создаёт пакет** — `{goalID}`, `{id}` схлопываются в родительский.
4. **Дефис убирается**: `move-up` → `moveup`, `key-results` → `keyresults`, `access-requests` → `accessrequests`, `no-access-message` → `noaccessmessage`.
5. **Состав пакета**: `handler.go` (тип `Handler` + `New` с зависимостями только этого пакета), `routes.go` (`RegisterRoutes`, монтирует свои эндпоинты и зовёт подпакеты), файлы методов.
6. **Пакет без собственных эндпоинтов** (только подпакеты) содержит один `routes.go`.

Итог: **81 API-пакет**. Полная карта — в `specs/070-code-structure.md` (Task 9); она генерируется скриптом из `internal/http/testdata/routes.golden`, а не пишется руками.

### Исключение: SPA-shell'ы и legacy-редиректы

12 роутов отдают 6 shell-шаблонов, ещё 7 — редиректы на канонические URL. Это не handler-логика, а таблицы «URI → шаблон» и «URI → target». Они уезжают в `handlers/web/shell/` декларативной таблицей с одним тестом на все URI; плодить 19 пакетов с однострочником — шум без выигрыша в навигации. Исключение фиксируется в спеке 070 явно.

`/no-access` в таблицу **не** входит: это вызов через реестр `nomembership.Get(name).ServeNoMembership(w, r)` с 500-й при незарегистрированном хендлере и подмешиванием `no_access_message` из настроек. Живая логика и точка расширения OSS/SaaS — получает пакет `handlers/web/noaccess/`.

---

### Task 1: Подготовка — эталон и мёртвый код

**Files:**
- Delete: `internal/http/handlers/web/keyresults/` (весь пакет, 250 строк)
- Modify: `internal/http/handlers/web/goals/handler.go` (удалить 4 неиспользуемых метода)

- [x] **Step 1: Снять эталон роутов** — `go test ./internal/http -run RoutesGolden -update-routes`, закоммитить `internal/http/testdata/routes.golden`.

- [x] **Step 2: Убедиться, что код действительно мёртв**

```bash
rg -n 'handlers/web/keyresults' --glob '*.go'                    # ожидается: пусто
rg -n 'HandleAddGoalComment|HandleAddKeyResult|HandleUpdateGoal\b|HandleUpdateGoalShare' \
   internal/http --glob '!internal/http/handlers/web/goals/*'    # ожидается: пусто
```

Если что-то нашлось — не удалять, разобраться.

- [x] **Step 3: Удалить**

```bash
git rm -r internal/http/handlers/web/keyresults
```

Из `handlers/web/goals/handler.go` удалить `HandleAddGoalComment`, `HandleAddKeyResult`, `HandleUpdateGoal`, `HandleUpdateGoalShare`; оставить только `HandleDeleteGoal` (единственный зарегистрированный). Подчистить осиротевшие импорты и хелперы.

- [x] **Step 4: Проверить**

```bash
go build ./... && go vet ./... && docker info > /dev/null && go test ./...
go test ./internal/http -run RoutesGolden
```

Роуты не менялись — удалялся код, который ни на один URI не отвечал.

---

### Общий образец переезда ресурса (Tasks 2–7)

Описан один раз; дальше указывается только состав.

**1. Пакет** `internal/http/handlers/api/v1/<путь по URI>/handler.go`:

```go
// Package share serves /api/v1/goals/{goalID}/share.
package share

import (
	"net/http"

	goaluc "okrs/internal/usecase/goal"
)

// Handler holds exactly what this URI needs — not the whole service surface.
type Handler struct {
	goals *goaluc.UseCase
}

func New(goals *goaluc.UseCase) *Handler { return &Handler{goals: goals} }
```

**2. `routes.go`** монтирует свои методы и зовёт подпакеты:

```go
func RegisterRoutes(r chi.Router, h *Handler) {
	r.Post("/api/v1/goals/{goalID}/share", h.Post)
	r.Delete("/api/v1/goals/{goalID}/share/{teamID}", h.Delete)
}
```

**3. Файлы методов** — тело переносится дословно, `h.service.X(...)` заменяется на вызов usecase/сервиса. Карта замен: делегаты фасада (`internal/service/service.go`) показывают, куда именно уходил каждый вызов, — это готовая таблица соответствия.

**4. Тесты** переезжают рядом с методом, `package <pkg>_test`.

**5. Проверка** после каждой задачи:

```bash
go build ./... && go vet ./...
go test ./internal/http -run RoutesGolden   # инвариант контракта
docker info > /dev/null && go test ./...
```

---

### Task 2: web-хендлеры

**Files:**
- Create: `handlers/web/shell/` (таблица shell'ов + редиректов), `handlers/web/noaccess/get.go`, `handlers/web/login/get.go`, `handlers/web/logout/post.go`, `handlers/web/auth/start/get.go`, `handlers/web/auth/callback/get.go`, `handlers/web/invite/get.go`, `handlers/web/goals/delete/post.go`
- Modify: `internal/http/server.go` (убрать closures)

**Interfaces:**
- `shell.Table` — `[]shell.Route{URI, Template, Gate}` + `RegisterRoutes`; один тест проходит по всем URI и проверяет 200 и наличие ссылки на entrypoint-скрипт.
- Остальные пакеты — по общему образцу.

Shell-таблица (12 URI, 6 шаблонов): `tracker-shell` → `/`, `/teams/{teamID}/okr`; `settings-shell` → `/settings`; `period-overview-shell` → `/period-overview`; `goal-tree-shell` → `/goal-tree`; `activity-shell` → `/activity-log`; `admin-shell` → `/admin`, `/admin/access`, `/admin/teams`, `/admin/periods`, `/admin/health-checkin`; `system-shell` → `/system`.

Редиректы (7): `/teams` → `/admin/teams`, `/periods` → `/admin/periods`, `/teamOkrs` → `/` (с сохранением query), `/admin/teams/new`, `/admin/teams/{teamID}/edit`, `/admin/periods/{periodID}/edit`, `/admin/users/{userID}` → `/admin`.

- [x] **Step 1:** `handlers/web/shell` — таблица, `RegisterRoutes`, тест на все 12 URI.
- [x] **Step 2:** Таблица редиректов там же; `/teamOkrs` сохраняет `RawQuery` — это поведение проверяется тестом.
- [x] **Step 3:** Остальные шесть пакетов по общему образцу.
- [x] **Step 4:** Убрать closures из `server.go`.
- [x] **Step 5:** Проверка по общему образцу.

---

### Task 3: `/api/v1/goals` и `/api/v1/krs`

Самый крупный ресурс: 14 + 10 методов.

**Files:** `handlers/api/v1/goals/{,linkable,links,share,transfer,weight,comments,comments/replies,comments/resolve,comments/unresolve,moveup,movedown,keyresults}`, `handlers/api/v1/krs/{,description,note,moveup,movedown,progress/{boolean,numerical,project}}`

**Внимание:** `POST /api/v1/goals/{goalID}/key-results` сейчас лежит в пакете `krs` — по URI-правилу переезжает в `goals/keyresults/`. Это иллюстрация выигрыша: сегодня его по URI не найти.

- [x] **Step 1:** Разложить `goals` (13 подпакетов).
- [x] **Step 2:** Разложить `krs` (8 подпакетов); `goals/keyresults` — из пакета `krs`.
- [x] **Step 3:** Перенести тесты (`access_test`, `leave_share_test`, `links_test`, `move_test`, `replies_test`, `repro_weight_test`, `resolve_test`, `routes_test`, `integration_test`).
- [x] **Step 4:** Проверка по общему образцу.

---

### Task 4: `/api/v1/{teams,periods,activity,hierarchy,goal-tree,users,config,health-checkin}`

**Files:** соответствующие подпакеты по карте.

- [x] **Step 1:** `teams` (+`okrs`, `overview`, `export`, `goals`, `status`).
- [x] **Step 2:** `periods` (+`overview`, `teams/activate`, `teams/close`).
- [x] **Step 3:** `activity` (+`categorycounts`, `treecounts`), `hierarchy` (переименование из `hierarhy` — опечатка чинится URI-правилом), `goaltree`.
- [x] **Step 4:** Одноэндпоинтные: `users`, `config`, `healthcheckin`, `me`.
- [x] **Step 5:** Проверка по общему образцу.

---

### Task 5: `/api/v1/admin`

33 метода в двух файлах (`handler.go` 14, `service_handler.go` 19) — крупнейший монолит после `goals`.

**Files:** `handlers/api/v1/admin/{users,users/admin,users/grants,settings/{access,general,feedback,healthcheckin},periods,periods/{archive,unarchive,overview,stats,teams/activate,teams/close},teams,teams/{restore,hard},invitations,invitations/revoke,accessrequests,accessrequests/{approve,deny},members,activity/purge}`

- [x] **Step 1:** Разложить `users` и `settings`.
- [x] **Step 2:** Разложить `periods` и `teams`.
- [x] **Step 3:** Разложить `invitations`, `accessrequests`, `members`, `activity/purge`.
- [x] **Step 4:** Перенести тесты (`activity_purge_test`, `handler_test`, `period_overview_test`, `periods_archive_test`).
- [x] **Step 5:** Проверка по общему образцу.

---

### Task 6: `/api/v1/system` и `/api/v1/session`

18 + 4 метода.

**Files:** `handlers/api/v1/system/{tenants,tenants/{members,members/deny,members/role,entitlements,suspend,restore,activity/purge},users,users/systemadmin,settings,settings/{defaultregistrationtenant,noaccessmessage}}`, `handlers/api/v1/session/{tenants,tenant,memberships}`, `handlers/api/v1/onboarding/joinrequest`

- [x] **Step 1:** Разложить `system/tenants`.
- [x] **Step 2:** Разложить `system/{users,settings}`.
- [x] **Step 3:** Разложить `session` и `onboarding/joinrequest`.
- [x] **Step 4:** Перенести тесты (`activity_purge_test`, `handler_test`).
- [x] **Step 5:** Проверка по общему образцу.

---

### Task 7: Ревизия `server.go`

**Files:** `internal/http/server.go`

- [x] **Step 1:** В `server.go` не должно остаться ни одного `r.Get/Post/…` — только `RegisterRoutes` пакетов и middleware-группы:

```bash
rg -n 'r\.(Get|Post|Put|Patch|Delete)\("' internal/http/server.go && echo "^^^ инлайн-роуты остались" || echo "server.go только монтирует"
wc -l internal/http/server.go   # ориентир: ~250 (было 701 до этапа D, 567 сейчас)
```

- [x] **Step 2:** Middleware-группы и их порядок не меняются: `AccessLog → Session → (RequireAuth) → (TenantResolve → RequireMembership → Scope) → CSRF`. Гейты (`RequireTenantAdmin`, `RequireSystemAdmin`) остаются ровно на тех же группах — сверить по диффу.
- [x] **Step 3:** Проверка по общему образцу.

---

### Task 8: Удаление фасада

**Files:** Delete `internal/service/service.go`; Modify `internal/http/server.go`, `app/app.go`, `internal/scheduler/scheduler.go`

- [x] **Step 1: Убедиться, что потребителей не осталось**

```bash
rg -n '\*service\.Service|service\.NewFromStore|service\.New\(' --glob '*.go' | grep -v '^internal/service/'
```

Ожидается: пусто. Каждое найденное — недоделанный handler.

- [x] **Step 2: Собрать usecase и сервисы в одном месте.** `service.New(Deps)` собирал 9 сервисов и 8 usecase. После удаления фасада эта сборка нужна по-прежнему — переносится в `app.New` (или `internal/app/wire.go`, если `app.go` разрастётся). Один экземпляр каждого сервиса, как сейчас.

- [x] **Step 3: Переключить scheduler.** Порты `SnapshotRunner`/`PeriodFinder` сейчас удовлетворяет `*service.Service`; заменить на `*perioduc.UseCase` и `*periodsvc.Service` — по одной строке в `StartBackground`.

- [x] **Step 4: Удалить**

```bash
git rm internal/service/service.go
go build ./... && go vet ./...
```

- [x] **Step 5:** Проверка по общему образцу. Каталог `internal/service` теперь содержит только подпакеты.

---

### Task 9: Спеки

**Files:** Create `specs/070-code-structure.md`; Modify `specs/010-architecture-constraints.md`, `README-specs.md`, дизайн-док

- [x] **Step 1: Сгенерировать карту URI → пакет** скриптом из `internal/http/testdata/routes.golden`, а не переписывать руками — 133 строки, ручная копия разойдётся с кодом на первой же правке.

- [x] **Step 2: Написать `specs/070-code-structure.md`**: правила раскладки (§Правила выше), карта из Step 1, исключение для shell'ов и редиректов, правило «пакет получает узкий конструктор», порядок добавления нового эндпоинта.

- [x] **Step 3: Обновить `specs/010`**: `internal/http/handlers/*` описывается новой раскладкой; убрать упоминание удалённых web-хендлеров; убрать фразу про временный фасад (его больше нет); сослаться на 070.

- [x] **Step 4: Обновить `README-specs.md`** — добавить `070-code-structure.md` в структуру каталога.

- [x] **Step 5: Закрыть дизайн-док** — отметить все пять пунктов задания выполненными, §13 (TODO) оставить как есть.

- [x] **Step 6: Финальная проверка**

```bash
docker info > /dev/null && go build ./... && go vet ./... && go test ./...
go test ./internal/http -run RoutesGolden && echo "контракт цел"
rg -c 'TODO\(refactoring\)' --glob '*.go' | wc -l   # долг на месте, не «починен заодно»
```

---

## Что НЕ входит в этап E

- Всё, помеченное `TODO(refactoring)` — см. §13 дизайн-дока.
- Слияние двух реализаций `UserSelector` во фронтенде.
- Изменение внешнего контракта в любом виде.
