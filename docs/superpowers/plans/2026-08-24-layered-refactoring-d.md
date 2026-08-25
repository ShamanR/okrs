# Слоистый рефакторинг, этап D — слой usecase и scheduler. План реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Вынести 32 оставшихся в `service.Service` бизнес-сценария в слой `internal/usecase`, а фоновые петли — в `internal/scheduler`, не трогая handlers.

**Architecture:** Цепочка становится `handler → usecase → service → store`. Usecase оркестрирует **сервисы сущностей**, а не репозитории — для этого сервисы дополняются недостающими однострочными методами (это операции над одной сущностью, им там и место). Фасад `service.Service` продолжает держать старый API для handlers и теперь делегирует ещё и в usecase; он удаляется на этапе E.

**Tech Stack:** Go 1.25, интерфейсы на стороне потребителя, fake-репозитории `servicetest`, `pg_try_advisory_lock` для single-flight в K8s.

**Spec:** `docs/superpowers/specs/2026-08-24-layered-refactoring-design.md`

## Global Constraints

- **Не делать `git commit`** — заказчик коммитит сам (CLAUDE.md #8).
- **Baseline:** `go test ./...` → exit 0; 70 пакетов, 54 с тестами все `ok`, 0 `FAIL`.
- **Интеграционные тесты требуют Docker** (testcontainers). Перед прогоном: `docker info`.
- **⚠ Правки «на месте» — `perl -pi -e`, якорить по `\b`, НЕ по `$`.** CRLF в рабочем дереве: `$` не совпадает ни в sed, ни в perl; BSD sed не поддерживает `\b`; `perl -pi -e '… print'` дублирует файл. Все три отказывают молча.
- **⚠ Никогда `gofmt -w` по каталогу** — перепишет ~211 файлов на LF. Только по списку изменённых.
- **`\b` не работает на границе с `_`** — `s/^package foo\b/` не совпадёт с `package foo_test`. Для пакетов замена без `\b`.
- **Handlers не трогаем.** После каждой задачи: `git status --short internal/http/` должен быть пуст (кроме `server.go` в Task 8, где убираются фоновые петли).
- **Внешний контракт неизменен.** Ни один URI, метод, статус-код, формат тела.
- **Проверка форматирования** — с нормализацией переводов строк:

```bash
fmtcheck() { for f in "$@"; do
  if diff -q <(tr -d '\r' < "$f") <(tr -d '\r' < "$f" | gofmt) >/dev/null 2>&1
    then echo "  OK   $f"; else echo "  BAD  $f"; fi; done; }
```

---

## Решение: usecase зависит от сервисов, не от репозиториев

Оставшиеся сценарии делают ~50 различных вызовов репозиториев. Развилка была такой:

| | usecase → repo | usecase → service |
|---|---|---|
| Объём | меньше: сценарии переезжают как есть | больше: +~35 методов в сервисы сущностей |
| Слои | у store две двери; «оркестрация сервисов» из дизайна — неправда | одна дверь к store, цепочка `handler → usecase → service → store` честная |
| Сервисы | вырождаются в веенер для простого CRUD handlers | становятся полными |

Берём второй. Добавляемые методы — не боилерплейт ради боилерплейта: `CreateGoal`, `UpdateGoal`, `ReplaceGoalShares`, `SetTeamPeriodStatus` и прочие суть операции над одной сущностью. Сервисы неполны только потому, что на этапе C переносилось лишь то, что уже было методами `Service`; сценарии всегда ходили в репозитории напрямую.

**Исключение — батчевые чтения.** Методы вида `ListGoalsByTeamsPeriod(periodID, teamIDs)` и `ListGoalCommentsByGoals(goalIDs)` существуют ровно для того, чтобы не делать N+1 запросов в цикле (CLAUDE.md #9). Они остаются на сервисе сущности как батчевые операции — usecase зовёт их одним вызовом, а не поштучно в цикле. Ни один переезд не должен превратить батч в цикл.

---

## Карта переезда

| Пакет usecase | Сценарии | Read-model типы |
|---|---|---|
| `usecase/okrboard` | `GetTeamOKR`, `GetTeamsWithPeriodSummary` (+`…FromTeams`, `appendTeamSummaryFromBatch`), `GetDirectChildrenSummary` (+`buildDirectChildrenSummary`), `GetTeamOverview` | `TeamOKR`, `TeamSummary`, `TeamGoalSummary`, `TeamShareInfo`, `TeamChildSummary`, `TeamOverview`, `GoalDetails` |
| `usecase/goal` | `CreateGoal`, `UpdateGoal`, `UpdateGoalFields`, `DeleteGoal`, `CopyGoal`, `ShareGoal`, `UpdateGoalOwnerAndShares`, `DeleteGoalShare`, `resetStatusIfNoGoals`, `AddGoalComment`, `AddGoalReply`, `SetGoalCommentResolved`, `DeleteGoalComment`, `SetGoalParents`, `AttachGoalLinks`, `fillGoalRefProgress` | `ShareTarget`, `CopyGoalMode`(+конст), `CopyGoalParams` |
| `usecase/keyresult` | `UpdateKRProgressNumerical/Boolean/Project`, `recordKRProgress`, `CreateKeyResultWithMeta`, `UpdateKeyResultWithMeta`, `DeleteKeyResult`, `UpsertKeyResultNote` | `ProjectStageUpdate` |
| `usecase/period` | `PeriodOverview`, `PeriodOverviewScoped`, `PeriodStats`, `BulkSetTeamPeriodStatus`, `UpdateTeamPeriodStatus`, `SnapshotActivePeriods`, серия прогресса | `PeriodOverview*`, `PeriodTeamSummary`, `PeriodGoalItem`, `PeriodKRItem`, `BalanceBucket`, `PeriodBalances`, `PeriodStatsItem`, `BulkStatusResult`, `SeriesPoint`, `ProgressSeries` |
| `usecase/goaltree` | `GoalTree` | `GoalTreeData`, `GoalTreeNode`, `GoalTreeTeam` |
| `usecase/export` | `ExportOKR` (+`exportTeamBlocks`, `exportGoalBlocks`, `exportTreeBlocks`, `filterNodesByScope`, `orderedSubtreeIDs`, `indexTeamsWithPaths`) | `ExportParams`, `ExportResult` |
| `usecase/user` | `SearchUsersInScope` | — |
| `usecase/healthcheckin` | загрузка `PeriodData` (бывший `hcLoader` из `server.go`) | — |

Плюс `internal/scheduler` — механика фоновых петель (§ Task 8).

---

### Task 1: Дополнить сервисы сущностей

**Files:**
- Modify: `internal/service/{team,goal,keyresult,period,goalshare,goallink,teamstatus,user}/*.go`

**Interfaces:**
- Produces: недостающие однострочные методы, перечисленные ниже. Все — прямой проброс в репозиторий (`return s.repo.X(...)`), сигнатуры один в один с репозиторными. Батчевые остаются батчевыми.

Полный список (сверен со списком вызовов из сценариев):

```
team:        ListTeamIDsWithGoalsInPeriod
goal:        Create, Update, UpdateFields, Delete, Copy, UpdateOwner,
             ListByTeamsPeriod, ListCommentsByGoals, ListOwnerTeamIDs,
             ListByIDs, ListForPeriods, ListTeamLastUpdateInPeriod,
             AddComment, AddReply, GetCommentMeta, DeleteComment, SetCommentResolved
keyresult:   Create, Update, Delete, GetNote, UpsertNote, BatchLoadNotes,
             GetBooleanMeta, UpdateBoolean, UpdateNumericalCurrent,
             ListProjectStages, UpdateProjectStageDone, BatchUpdateProjectStagesDone,
             UpsertNumericalMeta, UpsertBooleanMeta, ReplaceProjectStages
goalshare:   ListByGoalIDs, Replace, Delete
goallink:    ReplaceParents
teamstatus:  GetWithTime, List, Set, SetMany
user:        SearchUnrestricted, SearchInSet
period:      — (всё уже есть)
```

- [x] **Step 1: Свериться с фактом перед добавлением**

Список выше составлен скриптом; перепроверить перед работой, потому что этап C мог что-то уже покрыть:

```bash
cd /Users/lakosnikov.pavel/work/github.com/okrs
rg -o 's\.(teams|goals|shares|goalLinks|periods|krs|statuses|users|progressSnap)\.(\w+)\(' -r '$1.$2' \
  internal/service/*.go | sort -u
```

- [x] **Step 2: Добавить методы**

Каждый — проброс. Образец (`internal/service/goal/goal.go`):

```go
// Create inserts a goal and returns its id. Ownership/period invariants are the
// caller's business: this is the single-entity write, the scenario lives in usecase.
func (s *Service) Create(ctx context.Context, scope domain.TenantScope, input goals.GoalInput) (int64, error) {
	return s.repo.CreateGoal(ctx, scope, input)
}

// ListByTeamsPeriod is the batched read: one query for all teams, not one per team.
// Keep it batched — turning it into a loop reintroduces N+1 (CLAUDE.md #9).
func (s *Service) ListByTeamsPeriod(ctx context.Context, scope domain.TenantScope, periodID int64, teamIDs []int64) (map[int64][]domain.Goal, error) {
	return s.repo.ListGoalsByTeamsPeriod(ctx, scope, periodID, teamIDs)
}
```

- [x] **Step 3: Проверить**

```bash
go build ./... && go vet ./...
fmtcheck internal/service/*/[a-z]*.go
docker info > /dev/null && go test ./...
```

Ожидается: exit 0, 0 FAIL. Ничего не сломалось — только добавления.

---

### Общий образец переезда сценария (Tasks 2–7)

Все задачи ниже следуют одному образцу; здесь он описан один раз, дальше указывается только состав.

**1. Пакет usecase** — `internal/usecase/<name>/<name>.go`:

```go
// Package okrboard assembles the team OKR board: goals, key results, shares and
// period status for a team. It orchestrates entity services and owns the read-model
// the HTTP layer maps to DTOs.
package okrboard

import (
	"context"

	"okrs/internal/core/domain"
	goalsvc "okrs/internal/service/goal"
	teamsvc "okrs/internal/service/team"
	// …
)

// Deps are the entity services this usecase orchestrates.
type Deps struct {
	Teams    *teamsvc.Service
	Goals    *goalsvc.Service
	Shares   *goalsharesvc.Service
	Statuses *teamstatussvc.Service
}

type UseCase struct {
	teams    *teamsvc.Service
	goals    *goalsvc.Service
	shares   *goalsharesvc.Service
	statuses *teamstatussvc.Service
}

func New(deps Deps) *UseCase { return &UseCase{teams: deps.Teams, /* … */} }
```

**2. Тела** переносятся дословно, `s.<repo>.<RepoMethod>(` → `s.<entity>.<ServiceMethod>(`. Имя метода на сервисе короче репозиторного (`s.goals.CreateGoal` → `s.goals.Create`) — сверяться со списком Task 1.

**3. Read-model типы** переезжают в тот usecase-пакет, который их производит; в фасаде остаётся `type X = <pkg>.X`.

**4. Фасад** получает поле `<name>UC *<name>.UseCase`, инициализацию в `New(deps)` после сервисов сущностей, и делегаты под старыми именами. Приватные хелперы делегатов не получают.

**5. Тесты** переезжают в пакет своего сценария, `package <name>_test`, фейки — из `servicetest`.

**6. Проверка** после каждой задачи:

```bash
go build ./... && go vet ./...
fmtcheck internal/usecase/<name>/*.go internal/service/service.go
git status --short internal/http/ | grep . && echo "^^^ handlers ТРОНУТЫ" || echo "handlers не тронуты"
docker info > /dev/null && go test ./...
```

---

### Task 2: `usecase/okrboard`

**Files:**
- Create: `internal/usecase/okrboard/okrboard.go`, `okrboard_test.go`
- Modify: `internal/service/service.go`

**Interfaces:**
- Produces: `okrboard.UseCase` с методами `TeamOKR`, `TeamsWithPeriodSummary`, `DirectChildrenSummary`, `TeamOverview`; типы `TeamOKR`, `TeamSummary`, `TeamGoalSummary`, `TeamShareInfo`, `TeamChildSummary`, `TeamOverview`, `GoalDetails`.
- Consumes: `team`, `goal`, `goalshare`, `teamstatus` сервисы.

**Внимание:** `getTeamsWithPeriodSummaryFromTeams` делает батчевую загрузку (`ListByTeamsPeriod`, `ListByGoalIDs`, `List` статусов) — сохранить батчевость дословно.

- [x] **Step 1:** Создать пакет по общему образцу, перенести семь методов и семь типов.
- [x] **Step 2:** Перенести тесты: `rg -n '^func Test' internal/service/service_test.go | rg -i 'teamokr|teamswithperiod|overview|childrensummary'`.
- [x] **Step 3:** Фасад: поле `okrboardUC`, алиасы типов, делегаты `GetTeamOKR`/`GetTeamsWithPeriodSummary`/`GetDirectChildrenSummary`/`GetTeamOverview`.
- [x] **Step 4:** Проверка по общему образцу.

---

### Task 3: `usecase/goal`

**Files:**
- Create: `internal/usecase/goal/goal.go`, `comments.go`, `links.go`, `goal_test.go`
- Modify: `internal/service/service.go`, удалить `internal/service/goal_links.go`

**Interfaces:**
- Produces: `goal.UseCase` с `Create`, `Update`, `UpdateFields`, `Delete`, `Copy`, `Share`, `UpdateOwnerAndShares`, `DeleteShare`, `AddComment`, `AddReply`, `SetCommentResolved`, `DeleteComment`, `SetParents`, `AttachLinks`; типы `ShareTarget`, `CopyGoalMode` (+`CopyGoalModeCopy`/`CopyGoalModeMove`), `CopyGoalParams`.
- Consumes: `goal`, `goalshare`, `goallink`, `teamstatus`, `period`, `team`, `activity` сервисы.

Самый крупный пакет — 16 методов. Разложить по трём файлам: `goal.go` (CRUD-сценарии + share/transfer), `comments.go` (4 метода комментариев), `links.go` (`SetParents`, `AttachLinks`, `fillGoalRefProgress`).

- [x] **Step 1:** Создать три файла по общему образцу.
- [x] **Step 2:** Перенести тесты из `goal_test.go`, `copy_goal_test.go`, `goal_links_test.go` и относящиеся к целям из `activity_test.go`.
- [x] **Step 3:** Фасад: поле `goalUC`, алиасы типов, 14 делегатов.
- [x] **Step 4:** Проверка по общему образцу.

---

### Task 4: `usecase/keyresult`

**Files:**
- Create: `internal/usecase/keyresult/keyresult.go`, `keyresult_test.go`
- Modify: `internal/service/service.go`

**Interfaces:**
- Produces: `keyresult.UseCase` с `UpdateProgressNumerical`, `UpdateProgressBoolean`, `UpdateProgressProject`, `CreateWithMeta`, `UpdateWithMeta`, `Delete`, `UpsertNote`; тип `ProjectStageUpdate`.
- Consumes: `keyresult`, `goal`, `activity` сервисы.

`recordKRProgress` — приватный хелпер пакета: он считает before/after и пишет событие, его зовут все три `UpdateProgress*`.

- [x] **Step 1:** Создать пакет, перенести семь методов + хелпер.
- [x] **Step 2:** Перенести тесты из `progress_test.go` и KR-часть `activity_test.go`.
- [x] **Step 3:** Фасад: поле `keyresultUC`, алиас `ProjectStageUpdate`, 7 делегатов.
- [x] **Step 4:** Проверка по общему образцу.

---

### Task 5: `usecase/period`

**Files:**
- Create: `internal/usecase/period/overview.go`, `bulkstatus.go`, `progress.go`, тесты
- Modify: `internal/service/service.go`, удалить `period_overview.go`, `period_bulk_status.go`, `period_progress.go`

**Interfaces:**
- Produces: `period.UseCase` с `Overview`, `OverviewScoped`, `Stats`, `BulkSetTeamStatus`, `UpdateTeamStatus`, `SnapshotActive`; типы из карты переезда.
- Consumes: `period`, `team`, `goal`, `teamstatus`, `activity` сервисы, `healthcheckin.Cache`, `ProgressSnapRepo`.

`ProgressSnapRepo` — единственный репозиторий без сервиса сущности. Завести `internal/service/progresssnap` с методами `List`/`Upsert`/`LatestDate`, чтобы правило «одна дверь к store» не нарушалось.

- [x] **Step 1:** Создать `internal/service/progresssnap` (сервис сущности для снимков).
- [x] **Step 2:** Создать `internal/usecase/period` из трёх файлов.
- [x] **Step 3:** Перенести тесты из `period_overview_test.go`, `period_bulk_status_test.go`, `period_progress_test.go`.
- [x] **Step 4:** Фасад: поле `periodUC`, алиасы типов, делегаты.
- [x] **Step 5:** Проверка по общему образцу.

---

### Task 6: `usecase/goaltree`, `usecase/export`, `usecase/user`

Три небольших пакета одной задачей: 1 + 1 + 1 публичный метод.

**Files:**
- Create: `internal/usecase/goaltree/goaltree.go`, `internal/usecase/export/export.go`, `internal/usecase/user/user.go` + тесты
- Modify: `internal/service/service.go`, удалить `goal_tree.go`, `export.go`

**Interfaces:**
- `goaltree.UseCase.Build(...)` → `GoalTreeData`; consumes `team`, `goal`, `goallink`.
- `export.UseCase.OKR(...)` → `ExportResult`; consumes `team`, `goal`, `keyresult`, `period`, `goallink` + `render/export`.
- `user.UseCase.SearchInScope(...)`; consumes `user`, `team` сервисы и `GrantsProvider`.

- [x] **Step 1:** Создать три пакета.
- [x] **Step 2:** Перенести тесты из `export_test.go`, `search_test.go` и goal-tree тесты.
- [x] **Step 3:** Фасад: три поля, алиасы `GoalTreeData`/`GoalTreeNode`/`GoalTreeTeam`/`ExportParams`/`ExportResult`, делегаты.
- [x] **Step 4:** Проверка по общему образцу.

---

### Task 7: `usecase/healthcheckin` — загрузка PeriodData

**Files:**
- Create: `internal/usecase/healthcheckin/loader.go`
- Modify: `internal/http/server.go` (удалить `hcLoader`, ~50 строк)

**Interfaces:**
- Produces: `healthcheckin.NewPeriodLoader(deps) func(ctx, scope, periodID) (*hcsvc.PeriodData, error)` — та же сигнатура, что ждёт `healthcheckin.Cache`.
- Consumes: `period`, `team`, `goal`, `teamstatus` сервисы.

Это устраняет нарушение правила 1 спеки 010: сейчас `server.go` собирает `PeriodData` из пяти репозиториев прямо в конструкторе. Батчевость (`ListByTeamsPeriod`, `ListCommentsByGoals`, `List` статусов) сохранить дословно.

- [x] **Step 1:** Создать loader, перенеся тело `hcLoader` из `server.go:114-163`.
- [x] **Step 2:** В `server.go` заменить замыкание на `healthcheckinuc.NewPeriodLoader(...)`.
- [x] **Step 3:** Проверка: `go build ./... && go vet ./...`, полный прогон.

---

### Task 8: `internal/scheduler`

**Files:**
- Create: `internal/scheduler/scheduler.go`
- Modify: `internal/http/server.go` (удалить петли, ~100 строк), `app/app.go` (запуск)

**Interfaces:**
- Produces:

```go
package scheduler

// Scheduler runs the periodic background passes. It is started by app.New, not by
// building the router: Routes() must stay a pure assembly function so tests can build
// a router without spawning goroutines.
type Scheduler struct { /* … */ }

type Deps struct {
	DB            *pgxpool.Pool
	HCCache       *hcsvc.Cache
	PeriodUC      *perioduc.UseCase
	Tenants       TenantLister
	Settings      hcsvc.SettingsReader
	PeriodService *periodsvc.Service
	Snapshots     *progresssnapsvc.Service
	Zone          *time.Location
	Logger        *slog.Logger
}

func New(deps Deps) *Scheduler
func (s *Scheduler) Start(ctx context.Context)
```

Переезжает: `startProgressSnapshotLoop`, `snapshotDuePeriods`, `activePeriods`, `daysBetween`, `progressSnapshotLockKey`, `snapshotCheckInterval`, вызов `hcCache.StartRefreshLoop`.

**Что сохранить дословно:** advisory-lock `pg_try_advisory_lock(918273645)` — single-flight между репликами в K8s; карту `lastAttempt`, которая троттлит периоды, легитимно не дающие снимков (иначе они переобрабатываются на каждом тике); и то, что мутирует её ровно один держатель лока.

- [x] **Step 1:** Создать пакет, перенеся код из `server.go:245-309, 441-480`.
- [x] **Step 2:** Удалить петли из `server.go`; `Routes()` больше не запускает goroutine.
- [x] **Step 3:** Запускать из `app.New` после сборки сервера.
- [x] **Step 4:** Проверить, что `Routes()` чист:

```bash
rg -n 'go func|StartRefreshLoop|startProgressSnapshotLoop' internal/http/server.go && echo "^^^ петли остались" || echo "Routes() чист"
wc -l internal/http/server.go   # ориентир: ~450 (было 701)
```

- [x] **Step 5:** Полный прогон + smoke: поднять приложение, убедиться, что снимки прогресса пишутся (лог `progress snapshot`), а health-checkin отдаётся.

---

### Task 9: Ревизия и спеки

**Files:**
- Modify: `internal/service/service.go`, `specs/010-architecture-constraints.md`, дизайн-док

- [x] **Step 1:** Убедиться, что в корне `internal/service` остался только `service.go` (фасад) и его тест:

```bash
ls internal/service/*.go
```

- [x] **Step 2:** Посчитать состав фасада — должны остаться только делегаты, алиасы и обёртки:

```bash
python3 - <<'PY'
import re
s=open('internal/service/service.go',encoding='utf-8').read()
d=r=0
for p in re.split(r'\n(?=func )',s):
    m=re.match(r'func \(s \*Service\) (\w+)\(',p)
    if not m: continue
    body=p.split('{',1)[1] if '{' in p else ''
    st=[l.strip() for l in body.split('\n') if l.strip() and not l.strip().startswith('//') and l.strip()!='}']
    if len(st)==1 and re.match(r'return .*s\.\w+(Svc|UC|Cache)\.',st[0]): d+=1
    else: r+=1; print("НЕ ДЕЛЕГАТ:",m.group(1))
print(f"делегатов {d}, не-делегатов {r}")
PY
```

Ожидается: не-делегатов 0. Каждый оставшийся — либо пропущенный сценарий, либо кандидат в сервис сущности; разобрать поштучно.

- [x] **Step 3:** Проверить, что экспорт «ради фасада» с этапа C можно сузить. Кандидаты: `team.BuildHierarchy`, `team.FindDirectChildren`, `team.CollectDescendantIDs`, `team.HierarchyFromTeams`, `healthcheckin.BuildTeamPath`, `healthcheckin.Abs`. Для каждого:

```bash
rg -n '\bteam\.(BuildHierarchy|FindDirectChildren|CollectDescendantIDs|HierarchyFromTeams)\b|\bhealthcheckin\.(BuildTeamPath|Abs)\b' --glob '*.go'
```

Если единственный потребитель — один пакет usecase, перенести туда и сделать приватным. Если потребителей несколько — оставить экспортированным.

- [x] **Step 4:** Обновить спеку 010: добавить слой `internal/usecase` и `internal/scheduler` в перечень слоёв, зафиксировать цепочку `handler → usecase → service → store` и правило «usecase оркестрирует сервисы, а не репозитории».

- [x] **Step 5:** Обновить дизайн-док §6 и §8 по фактическому результату; отметить, что нарушение правила 1 спеки 010 (бизнес-логика в `server.go`) устранено.

- [x] **Step 6:** Финальный прогон.

---

## Что НЕ входит в этап D

- **Перевязка handlers и удаление фасада** — этап E.
- **Разбиение handlers по URI-пакетам** и `specs/070-code-structure.md` — этап E.
- **Удаление мёртвого кода** (`handlers/web/keyresults`) — этап E.
- **Слияние двух реализаций `UserSelector`** во фронтенде — не относится к задаче.
