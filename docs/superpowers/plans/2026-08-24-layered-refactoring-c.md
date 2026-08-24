# Слоистый рефакторинг, этап C — распил service. План реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Разбить `internal/service/service.go` (1865 строк, 93 метода) на девять пакетов по сущностям, не трогая handlers.

**Architecture:** 55 «чистых» методов (≤1 репозиторий и без записи в журнал активности) уезжают в `internal/service/<entity>`. Оставшиеся 34 — мультисущностные сценарии — остаются в `service.Service`, который превращается в **фасад**: хранит указатели на сервисы сущностей и делегирует им. Handlers продолжают звать `service.Service` и не меняются до этапа E; экспортируемые типы сохраняются через type alias. Фасад удаляется на этапе E, когда handlers перевяжут напрямую.

**Tech Stack:** Go 1.25, интерфейсы репозиториев на стороне потребителя, fake-репозитории в тестах.

**Spec:** `docs/superpowers/specs/2026-08-24-layered-refactoring-design.md`

## Global Constraints

- **Не делать `git commit`** — заказчик коммитит сам (CLAUDE.md #8).
- **Baseline:** `go test ./...` → exit 0; 56 пакетов, 46 с тестами все `ok`, 0 `FAIL`. Приёмка каждой задачи — этот же результат.
- **Интеграционные тесты требуют Docker** (testcontainers). Перед прогоном: `docker info`.
- **⚠ Правки «на месте» — `perl -pi -e`, якорить по `\b`, НЕ по `$`.** Рабочее дерево в CRLF (`core.autocrlf=true`), и это даёт три ловушки с *тихим* отказом:
  1. **Якорь `$` не совпадает ни в sed, ни в perl** — перед `\n` стоит `\r`. Проверено: `printf 'package okr\r\n' | perl -pe 's/^package okr$/X/'` возвращает строку без изменений. Вариант `\r?$` работает, но съедает `\r`; `\b` безопаснее.
  2. **BSD sed не поддерживает `\b`** — `sed 's/\bfoo\./bar./g'` молчаливый no-op. Perl поддерживает.
  3. **`perl -pi -e '... print'` дублирует файл** — `-p` печатает сам, явный `print` даёт вторую копию каждой строки. При равномерном дублировании восстанавливается через `awk 'NR%2==1'`, но сначала проверить парность.
- **⚠ Никогда `gofmt -w` по каталогу** — перепишет ~211 файлов на LF. Только по одному файлу / по списку изменённых.
- **Проверка форматирования** — с нормализацией переводов строк:

```bash
fmtcheck() { for f in "$@"; do
  if diff -q <(tr -d '\r' < "$f") <(tr -d '\r' < "$f" | gofmt) >/dev/null 2>&1
    then echo "  OK   $f"; else echo "  BAD  $f"; fi; done; }
```

- **Handlers в этом этапе не трогаем.** Ни один файл под `internal/http/` не меняется, кроме случаев, когда компилятор потребует — а он не должен: фасад сохраняет и методы, и типы.
- **Внешний контракт неизменен.** Ни один URI, метод, статус-код, формат тела.
- **Go-конвенция именования.** В пакете сущности методы теряют повтор имени: `service/team.Service.List`, а не `ListTeams`. Фасад делает маппинг старых имён на новые — компилятор ловит любую ошибку.
- **Правило распила (операционное).** Метод уезжает в сервис сущности, если он трогает **не более одного** репозитория **и** не пишет в журнал активности. Иначе остаётся в `Service` и станет usecase на этапе D. Правило выведено из фактического анализа кода, а не из интуиции.

---

## Расхождения с дизайн-доком, обнаруженные при анализе

Дизайн `§5`/`§6` писался до построения карты «метод → репозитории». Два пункта не подтвердились и правятся в Task 11:

1. **`usecase/team` не нужен.** Дизайн относил удаление команды с guard'ом `ErrTeamHasGoals` в usecase. Фактически guard висит на `HardDeleteTeam`, а не на `DeleteTeam`, и оба метода используют только `TeamRepo` — это инвариант одной сущности, его место в `service/team`. Попутно уточнение семантики: `DeleteTeam` ошибку не возвращает вовсе, а выбирает вид удаления — soft, если у команды есть цели (история сохраняется), hard, если целей нет. Поэтому в пакете он называется `Delete`, а не `SoftDelete`.
2. **`GetHierarchy` — не usecase.** Дизайн относил его в `usecase/okrboard`. Фактически он читает только `TeamRepo` и строит дерево в памяти → `service/team`. При этом `GetTeamsWithPeriodSummary` и `GetDirectChildrenSummary` действительно usecase: это тонкие обёртки над `getTeamsWithPeriodSummaryFromTeams` (4 репозитория) и `buildDirectChildrenSummary` (2 репозитория).

---

## Карта распила

**Уезжают в сервисы (55 методов):**

| Пакет | Методов | Методы (новое имя ← старое) |
|---|---|---|
| `service/team` | 12 | `List`←ListTeams, `ListDeleted`←ListDeletedTeams, `ListAll`←ListAllTeams, `Get`←GetTeam, `Create`←CreateTeam, `Update`←UpdateTeam, `Delete`←DeleteTeam, `Restore`←RestoreTeam, `HardDelete`←HardDeleteTeam, `Hierarchy`←GetHierarchy, `hierarchyFromTeams`←getHierarchyFromTeams, `VisibleInPeriod`←isTeamVisibleInPeriod |
| `service/period` | 9 | `List`←ListPeriods, `ListViews`←ListPeriodViews, `Get`←GetPeriod, `FindForDate`←FindPeriodForDate, `Create`←CreatePeriod, `Update`←UpdatePeriod, `Delete`←DeletePeriod, `Archive`←ArchivePeriod, `Unarchive`←UnarchivePeriod |
| `service/keyresult` | 8 | `UpdateHealthStatus`←UpdateKRHealthStatus, `autoCompleteHealth`, `UpdateDescription`←UpdateKeyResultDescription, `Get`←GetKeyResult, `Move`←MoveKeyResult, `applyMeta`←applyKeyResultMeta, `FindGoalIDByKR`, `FindGoalIDByStage` |
| `service/goal` | 5 | `Get`←GetGoal, `Move`←MoveGoal, `ListByTeamPeriod`←ListGoalsByTeamPeriod, `ListComments`←ListGoalComments, `ProgressByIDs`←goalProgressByIDs (становится экспортируемым — её зовёт `goallink`) |
| `service/activity` | 4 | `List`←ListActivity, `TreeCounts`←ActivityTreeCounts, `CategoryCounts`←ActivityCategoryCounts, `Purge`←PurgeActivity, плюс `Record` (из приватного `recordActivity`) |
| `service/user` | 4 | `GetByDisplayNames`←GetUsersByDisplayNames, `GetByUDIDs`←GetUsersByUDIDs, `ListLeadTeams`←ListUserLeadTeams, `ValidateUDIDsExist`←ValidateUserUDIDsExist |
| `service/goalshare` | 3 | `UpdateWeight`←UpdateGoalWeight, `Get`←GetGoalShare, `List`←ListGoalShares |
| `service/goallink` | 2 | `ListForGoals`←ListLinksForGoals, `ListLinkable`←ListLinkableGoals |
| `service/teamstatus` | 1 | `Get`←GetTeamPeriodStatus |

Плюс безрепозиторные хелперы уезжают вместе со своими вызывающими: `appendTeamSummaryFromBatch` и `buildTeamHierarchy`/`buildTeamNode`/`findDirectChildren`/`collectDescendantIDs` → `service/team`; `AttachGoalLinks`/`fillGoalRefProgress` → `service/goallink`; `exportTeamBlocks`/`exportGoalBlocks` остаются в `Service` (их зовёт `ExportOKR`).

**Остаются в `Service` до этапа D (34):** `ExportOKR`, `exportTreeBlocks`, `SetGoalParents`, `GoalTree`, `BulkSetTeamPeriodStatus`, `PeriodOverview*`, `PeriodStats`, `SnapshotActivePeriods`, `getTeamsWithPeriodSummaryFromTeams`, `GetTeamsWithPeriodSummary`, `GetTeamOKR`, `buildDirectChildrenSummary`, `GetDirectChildrenSummary`, `GetTeamOverview`, `UpdateKRProgress{Numerical,Boolean,Project}`, `ShareGoal`, комментарии цели (`AddGoalComment`, `AddGoalReply`, `SetGoalCommentResolved`, `DeleteGoalComment`), `UpsertKeyResultNote`, `UpdateGoal`, `UpdateGoalFields`, `CreateKeyResultWithMeta`, `UpdateKeyResultWithMeta`, `UpdateTeamPeriodStatus`, `SearchUsersInScope`, `DeleteGoalShare`, `DeleteKeyResult`, `CreateGoal`, `CopyGoal`, `DeleteGoal`, `resetStatusIfNoGoals`, `UpdateGoalOwnerAndShares`.

---

### Task 1: Доменные ошибки в `core/domain`

**Files:**
- Create: `internal/core/domain/errors.go`
- Modify: `internal/service/service.go` (блок `var (...)` с `Err*`)

**Interfaces:**
- Produces: `domain.ErrTeamHasGoals`, `ErrTeamNotVisibleInPeriod`, `ErrPeriodClosed`, `ErrPeriodNotClosed`, `ErrCannotShareWithClosedPeriod`, `ErrShareTargetNotInTenant`, `ErrTransferTargetSameAsSource`, `ErrTransferTargetNotFound`, `ErrForbidden`, `ErrGoalNotOnTeamBoard`, `ErrGoalLinkSelf`, `ErrGoalLinkNotAccessible`, `ErrGoalLinkCycle` — единый дом для service, usecase и handler.
- `service.Err*` сохраняются как переменные-алиасы (`var ErrX = domain.ErrX`), поэтому handlers и их `errors.Is` не меняются.

- [x] **Step 1: Создать файл ошибок**

Create `internal/core/domain/errors.go`:

```go
package domain

import "errors"

// Доменные инварианты, пересекающие границу сущности. Живут здесь, а не в service,
// потому что на них смотрят и сервисы, и usecase, и handlers при маппинге в HTTP-коды;
// после распила service держать их там означало бы размазать по 13 пакетам.
var (
	ErrTeamHasGoals                = errors.New("team has goals")
	ErrTeamNotVisibleInPeriod      = errors.New("team not visible in period")
	ErrPeriodClosed                = errors.New("period is closed")
	ErrPeriodNotClosed             = errors.New("period must be closed to archive")
	ErrCannotShareWithClosedPeriod = errors.New("cannot share goal with team whose period is in_progress or closed")
	ErrShareTargetNotInTenant      = errors.New("share target team is not in the active tenant")
	ErrTransferTargetSameAsSource  = errors.New("transfer target equals source team and period")
	ErrTransferTargetNotFound      = errors.New("transfer target team or period not found in tenant")
	// ErrForbidden signals an authorization failure the handler maps to HTTP 403.
	ErrForbidden = errors.New("forbidden")
	// ErrGoalNotOnTeamBoard signals a goal-scope export where the goal is not on the context team's board.
	ErrGoalNotOnTeamBoard = errors.New("goal not on team board")
	// Goal-link errors mapped by handlers to 400 (self/not accessible) and 409 (cycle).
	ErrGoalLinkSelf          = errors.New("goal cannot link to itself")
	ErrGoalLinkNotAccessible = errors.New("parent goal not accessible")
	ErrGoalLinkCycle         = errors.New("goal link would create a cycle")
)
```

- [x] **Step 2: Заменить объявления в service на алиасы**

В `internal/service/service.go` блок `var ( ErrTeamHasGoals = errors.New(...) ... )` заменить на:

```go
// Доменные ошибки переехали в core/domain (единый дом для service/usecase/handler).
// Здесь остаются алиасы, чтобы существующие errors.Is в handlers продолжали работать
// без правок; удаляются на этапе E вместе с фасадом.
var (
	ErrTeamHasGoals                = domain.ErrTeamHasGoals
	ErrTeamNotVisibleInPeriod      = domain.ErrTeamNotVisibleInPeriod
	ErrPeriodClosed                = domain.ErrPeriodClosed
	ErrPeriodNotClosed             = domain.ErrPeriodNotClosed
	ErrCannotShareWithClosedPeriod = domain.ErrCannotShareWithClosedPeriod
	ErrShareTargetNotInTenant      = domain.ErrShareTargetNotInTenant
	ErrTransferTargetSameAsSource  = domain.ErrTransferTargetSameAsSource
	ErrTransferTargetNotFound      = domain.ErrTransferTargetNotFound
	ErrForbidden                   = domain.ErrForbidden
	ErrGoalNotOnTeamBoard          = domain.ErrGoalNotOnTeamBoard
	ErrGoalLinkSelf                = domain.ErrGoalLinkSelf
	ErrGoalLinkNotAccessible       = domain.ErrGoalLinkNotAccessible
	ErrGoalLinkCycle               = domain.ErrGoalLinkCycle
)
```

Если после этого импорт `"errors"` в `service.go` перестал использоваться — удалить его.

Ошибки, специфичные для одного сервиса и не пересекающие его границу, **не трогаем**: `ErrLastAdmin`, `ErrSelfLockout`, `ErrLastSystemAdmin`, `ErrAlreadyMember`, `ErrTenantNotFound` остаются в `provisioning.go` / `onboarding.go`.

- [x] **Step 3: Проверить**

```bash
go build ./... && go vet ./... && fmtcheck internal/core/domain/errors.go internal/service/service.go
docker info > /dev/null && go test ./...
```

Ожидается: exit 0, 0 FAIL. Handlers не менялись — `errors.Is(err, service.ErrPeriodClosed)` работает через алиас, потому что это та же самая переменная.

---

### Task 2: Общие фейки в `service/servicetest`

**Files:**
- Create: `internal/service/servicetest/store.go`
- Modify: `internal/service/service_test.go` (удалить `fakeStore`, перейти на `servicetest.Store`)

**Interfaces:**
- Produces: `servicetest.Store` — экспортированный `fakeStore` (80 методов, реализует все репозиторные интерфейсы), `servicetest.NewStore() *Store`. Поля становятся экспортируемыми, чтобы тесты из других пакетов могли их заполнять.

**Почему отдельный пакет, а не `_test.go` рядом.** После распила тесты живут в девяти пакетах, и каждому нужен тот же фейк. Дублировать 80 методов девять раз нельзя; Go не даёт импортировать `_test.go` из другого пакета — значит фейк обязан быть обычным (не-test) файлом в собственном пакете.

- [x] **Step 1: Перенести fakeStore в новый пакет**

```bash
mkdir -p internal/service/servicetest
```

Перенести в `internal/service/servicetest/store.go` тип `fakeStore` (`internal/service/service_test.go:17-36`), конструктор `newFakeStore` и все 80 методов `func (f *fakeStore) ...`. Переименовать:

- тип `fakeStore` → `Store`;
- конструктор `newFakeStore` → `NewStore`;
- **поля структуры — в экспортируемые** (`teams` → `Teams`, `periods` → `Periods`, `goalsByTeam` → `GoalsByTeam`, `statuses` → `Statuses`, `keyResults` → `KeyResults`, `numericalUpdates` → `NumericalUpdates`, `healthUpdates` → `HealthUpdates`, `booleanUpdates` → `BooleanUpdates`, `projectStages` → `ProjectStages`, `stageUpdates` → `StageUpdates`, `movedGoals` → `MovedGoals`, `movedKRs` → `MovedKRs`, `softDeleted` → `SoftDeleted`, `restored` → `Restored`, `hardDeleted` → `HardDeleted`, `bulkSetTeamIDs` → `BulkSetTeamIDs`, `bulkSetStatus` → `BulkSetStatus`, `currentPeriod` → `CurrentPeriod`).

Файл — `package servicetest`, без суффикса `_test.go`.

- [x] **Step 2: Перевести существующие тесты на новый тип**

В `internal/service/*_test.go`:

```bash
cd /Users/lakosnikov.pavel/work/github.com/okrs
rg -l 'fakeStore|newFakeStore' internal/service/*_test.go | xargs perl -pi -e '
  s/\bnewFakeStore\(\)/servicetest.NewStore()/g;
  s/\*fakeStore\b/*servicetest.Store/g;
  s/\bfakeStore\{/servicetest.Store{/g;
'
```

Затем вручную поправить обращения к полям на экспортируемые имена (компилятор перечислит все) и добавить импорт `"okrs/internal/service/servicetest"` в затронутые файлы.

- [x] **Step 3: Проверить**

```bash
go build ./... && go vet ./...
rg -n '\bfakeStore\b' internal/service/ || echo "старый тип вычищен"
docker info > /dev/null && go test ./internal/service/...
```

Ожидается: `ok okrs/internal/service`, старого имени не осталось.

- [x] **Step 4: Полный прогон**

```bash
docker info > /dev/null && go test ./...
```

Ожидается: exit 0, 0 FAIL. Пакетов станет 57 (`servicetest` — без тестов).

---

### Task 3: `service/team`

**Files:**
- Create: `internal/service/team/team.go`, `internal/service/team/hierarchy.go`, `internal/service/team/team_test.go`
- Modify: `internal/service/service.go` (удалить перенесённое, добавить делегирование)

**Interfaces:**
- Consumes: `servicetest.Store` (Task 2) в тестах.
- Produces:

```go
package team

type Repo interface { /* 12 методов TeamRepo из service.go:33-47, дословно */ }

// Node is a team plus its subtree, the read-model Hierarchy returns.
type Node struct { /* поля из service.TeamNode, service.go:241-245 */ }

type Service struct { repo Repo }
func New(repo Repo) *Service

func (s *Service) List(ctx context.Context, scope domain.TenantScope) ([]domain.Team, error)
func (s *Service) ListDeleted(ctx context.Context, scope domain.TenantScope) ([]domain.Team, error)
func (s *Service) ListAll(ctx context.Context, scope domain.TenantScope) ([]domain.Team, error)
func (s *Service) Get(ctx context.Context, scope domain.TenantScope, teamID int64) (domain.Team, error)
func (s *Service) Create(ctx context.Context, scope domain.TenantScope, input teams.TeamInput) (int64, error)
func (s *Service) Update(ctx context.Context, scope domain.TenantScope, input teams.TeamInput, id int64) error
func (s *Service) Delete(ctx context.Context, scope domain.TenantScope, teamID int64) error
func (s *Service) Restore(ctx context.Context, scope domain.TenantScope, teamID int64) error
func (s *Service) HardDelete(ctx context.Context, scope domain.TenantScope, teamID int64) error
func (s *Service) Hierarchy(ctx context.Context, scope domain.TenantScope, periodID *int64) ([]Node, error)
func (s *Service) VisibleInPeriod(ctx context.Context, scope domain.TenantScope, team domain.Team, periodID int64) (bool, error)

// Экспортируются, потому что их зовут сценарии, остающиеся в Service (этап D):
func BuildHierarchy(allTeams []domain.Team) (map[int64]domain.Team, map[int64][]domain.Team, []domain.Team)
func BuildNode(team domain.Team, childrenMap map[int64][]domain.Team) Node
func FindDirectChildren(targetID int64, nodes []Node) []Node
func CollectDescendantIDs(targetID int64, nodes []Node) []int64
```

- [x] **Step 1: Создать пакет с перенесённым кодом**

`team.go` — `Repo`, `Service`, `New` и CRUD-методы (`List`…`HardDelete`), тела берутся дословно из `internal/service/service.go` с заменой `s.teams.` → `s.repo.`. `SoftDelete` сохраняет guard:

```go
// Delete picks the deletion kind: a team that still has goals is soft-deleted so its
// history survives; an empty team is removed outright. HardDelete is the explicit
// variant and is the one that refuses (domain.ErrTeamHasGoals) when goals remain.
// Both read only the team repository, so this is a single-entity invariant.
func (s *Service) Delete(ctx context.Context, scope domain.TenantScope, teamID int64) error {
	hasGoals, err := s.repo.TeamHasGoals(ctx, scope, teamID)
	if err != nil {
		return err
	}
	if hasGoals {
		return s.repo.SoftDeleteTeam(ctx, scope, teamID)
	}
	return s.repo.HardDeleteTeam(ctx, scope, teamID)
}

func (s *Service) HardDelete(ctx context.Context, scope domain.TenantScope, teamID int64) error {
	hasGoals, err := s.repo.TeamHasGoals(ctx, scope, teamID)
	if err != nil {
		return err
	}
	if hasGoals {
		return domain.ErrTeamHasGoals
	}
	return s.repo.HardDeleteTeam(ctx, scope, teamID)
}
```

`hierarchy.go` — `Node`, `Hierarchy`, `hierarchyFromTeams`, `VisibleInPeriod`, `BuildHierarchy`, `BuildNode`, `FindDirectChildren`, `CollectDescendantIDs`.

- [x] **Step 2: Перенести тесты**

Из `internal/service/service_test.go` и `goal_test.go` перенести в `internal/service/team/team_test.go` тесты, относящиеся к командам и иерархии (искать по `ListTeams|GetTeam|CreateTeam|UpdateTeam|DeleteTeam|RestoreTeam|HardDelete|Hierarchy`):

```bash
rg -n 'func Test' internal/service/service_test.go | rg -i 'team|hierarch'
```

Тела тестов не меняются, кроме: `package team_test`, конструктор `team.New(servicetest.NewStore())`, новые имена методов.

- [x] **Step 3: Подключить в фасад**

В `internal/service/service.go`: в `Service` добавить поле `teamSvc *team.Service`, инициализировать в `New(deps)` как `team.New(deps.Teams)`. Заменить перенесённые методы на делегирование, сохранив старые сигнатуры и имена:

```go
func (s *Service) ListTeams(ctx context.Context, scope domain.TenantScope) ([]domain.Team, error) {
	return s.teamSvc.List(ctx, scope)
}
func (s *Service) GetHierarchy(ctx context.Context, scope domain.TenantScope, periodID *int64) ([]TeamNode, error) {
	return s.teamSvc.Hierarchy(ctx, scope, periodID)
}
// …и так для остальных десяти
```

Добавить alias, чтобы handlers не менялись: `type TeamNode = team.Node`. Тип `TeamRepo` в `service.go` заменить на `team.Repo` (в `Deps` поле `Teams team.Repo`).

- [x] **Step 4: Проверить**

```bash
go build ./... && go vet ./...
fmtcheck internal/service/team/*.go internal/service/service.go
docker info > /dev/null && go test ./internal/service/... ./internal/http/...
```

Ожидается: зелено. **Ни один файл под `internal/http/` меняться не должен** — проверить:

```bash
git status --short internal/http/ | grep -v '^$' && echo "^^^ handlers тронуты — разобраться" || echo "handlers не тронуты"
```

- [x] **Step 5: Полный прогон**

```bash
docker info > /dev/null && go test ./...
```

---

### Task 4: `service/period`

**Files:**
- Create: `internal/service/period/period.go`, `internal/service/period/period_test.go`
- Modify: `internal/service/service.go`

**Interfaces:**
- Produces:

```go
package period

type Repo interface { /* 8 методов PeriodRepo из service.go:88-98, дословно */ }

type Service struct { repo Repo }
func New(repo Repo) *Service

func (s *Service) List(ctx context.Context, scope domain.TenantScope) ([]domain.Period, error)
func (s *Service) ListViews(ctx context.Context, scope domain.TenantScope, includeArchived bool) ([]domain.PeriodView, error)
func (s *Service) Get(ctx context.Context, scope domain.TenantScope, periodID int64) (domain.Period, error)
func (s *Service) FindForDate(ctx context.Context, scope domain.TenantScope, date time.Time) (domain.Period, error)
func (s *Service) Create(ctx context.Context, scope domain.TenantScope, input periods.PeriodInput) (int64, error)
func (s *Service) Update(ctx context.Context, scope domain.TenantScope, periodID int64, input periods.PeriodInput) error
func (s *Service) Delete(ctx context.Context, scope domain.TenantScope, periodID int64) error
func (s *Service) Archive(ctx context.Context, scope domain.TenantScope, periodID int64) error   // возвращает domain.ErrPeriodNotClosed
func (s *Service) Unarchive(ctx context.Context, scope domain.TenantScope, periodID int64) error
```

- [x] **Step 1: Создать пакет**

Тела берутся дословно из `service.go` с заменой `s.periods.` → `s.repo.`. В `Archive` возврат `ErrPeriodNotClosed` заменить на `domain.ErrPeriodNotClosed` (Task 1).

- [x] **Step 2: Перенести тесты**

```bash
rg -n 'func Test' internal/service/*_test.go | rg -i 'period' | rg -v 'overview|progress|bulk'
```

Перенести найденные в `internal/service/period/period_test.go`, `package period_test`.

- [x] **Step 3: Подключить в фасад**

Поле `periodSvc *period.Service`, инициализация `period.New(deps.Periods)`, делегирование девяти методов под старыми именами (`ListPeriods` → `s.periodSvc.List` и т.д.). `PeriodRepo` → `period.Repo`.

- [x] **Step 4: Проверить**

```bash
go build ./... && go vet ./... && fmtcheck internal/service/period/*.go internal/service/service.go
git status --short internal/http/ | grep -v '^$' && echo "^^^ handlers тронуты" || echo "handlers не тронуты"
docker info > /dev/null && go test ./...
```

---

### Task 5: `service/keyresult`

**Files:**
- Create: `internal/service/keyresult/keyresult.go`, `internal/service/keyresult/keyresult_test.go`
- Modify: `internal/service/service.go`

**Interfaces:**
- Produces:

```go
package keyresult

type Repo interface { /* 22 метода KRRepo из service.go:99-122, дословно */ }

// MetaInput carries the per-kind metadata a KR create/update applies. Lives here
// (not in service) because applyMeta moved: service aliases it back for handlers.
type MetaInput struct { /* поля из service.KeyResultMetaInput, service.go:1111-1120 */ }

type Service struct { repo Repo }
func New(repo Repo) *Service

func (s *Service) Get(ctx context.Context, scope domain.TenantScope, id int64) (domain.KeyResult, error)
func (s *Service) Move(ctx context.Context, scope domain.TenantScope, krID int64, direction int) error
func (s *Service) UpdateDescription(ctx context.Context, scope domain.TenantScope, krID int64, description string) error
func (s *Service) UpdateHealthStatus(ctx context.Context, scope domain.TenantScope, krID int64, status domain.KRHealthStatus) error
func (s *Service) FindGoalIDByKR(ctx context.Context, scope domain.TenantScope, krID int64) (int64, error)
func (s *Service) FindGoalIDByStage(ctx context.Context, scope domain.TenantScope, stageID int64) (int64, error)
// ApplyMeta экспортируется: её зовут CreateKeyResultWithMeta/UpdateKeyResultWithMeta,
// остающиеся в Service до этапа D.
func (s *Service) ApplyMeta(ctx context.Context, scope domain.TenantScope, krID int64, kind domain.KRKind, meta MetaInput) error
// AutoCompleteHealth экспортируется по той же причине — её зовут UpdateKRProgress*.
func (s *Service) AutoCompleteHealth(ctx context.Context, scope domain.TenantScope, krID int64, kr domain.KeyResult, before, after int)
```

- [x] **Step 1: Создать пакет**

Тела дословно из `service.go`, `s.krs.` → `s.repo.`.

- [x] **Step 2: Перенести тесты**

```bash
rg -n 'func Test' internal/service/*_test.go | rg -i 'kr|keyresult|health' | rg -v 'checkin'
```

- [x] **Step 3: Подключить в фасад**

Поле `krSvc *keyresult.Service`; делегирование под старыми именами; alias `type KeyResultMetaInput = keyresult.MetaInput`; `KRRepo` → `keyresult.Repo`. Внутренние вызовы `s.applyKeyResultMeta(...)` и `s.autoCompleteHealth(...)` в остающихся методах заменить на `s.krSvc.ApplyMeta(...)` / `s.krSvc.AutoCompleteHealth(...)`.

- [x] **Step 4: Проверить**

```bash
go build ./... && go vet ./... && fmtcheck internal/service/keyresult/*.go internal/service/service.go
git status --short internal/http/ | grep -v '^$' && echo "^^^ handlers тронуты" || echo "handlers не тронуты"
docker info > /dev/null && go test ./...
```

---

### Task 6: `service/goal`

**Files:**
- Create: `internal/service/goal/goal.go`, `internal/service/goal/goal_test.go`
- Modify: `internal/service/service.go`

**Interfaces:**
- Produces:

```go
package goal

type Repo interface { /* 23 метода GoalRepo из service.go:48-71, дословно */ }

type Service struct { repo Repo }
func New(repo Repo) *Service

func (s *Service) Get(ctx context.Context, scope domain.TenantScope, id int64) (domain.Goal, error)
func (s *Service) Move(ctx context.Context, scope domain.TenantScope, teamID, goalID int64, direction int) error
func (s *Service) ListByTeamPeriod(ctx context.Context, scope domain.TenantScope, teamID, periodID int64) ([]domain.Goal, error)
func (s *Service) ListComments(ctx context.Context, scope domain.TenantScope, goalID int64) ([]domain.GoalComment, error)
// ProgressByIDs экспортируется: её зовёт goallink.fillGoalRefProgress и сценарии этапа D.
func (s *Service) ProgressByIDs(ctx context.Context, scope domain.TenantScope, ids []int64) (map[int64]int, error)
```

- [x] **Step 1: Создать пакет**

Тела дословно, `s.goals.` → `s.repo.`. `GetGoal` сохраняет текущую подгрузку KR-агрегата — тело копируется как есть.

- [x] **Step 2: Перенести тесты**

```bash
rg -n 'func Test' internal/service/goal_test.go
```

Перенести только те, что покрывают пять перенесённых методов; тесты `CreateGoal`/`CopyGoal`/`DeleteGoal`/`UpdateGoal` **остаются** в `internal/service` — эти методы уезжают в usecase на этапе D.

- [x] **Step 3: Подключить в фасад**

Поле `goalSvc *goal.Service`; делегирование; `GoalRepo` → `goal.Repo`.

- [x] **Step 4: Проверить**

```bash
go build ./... && go vet ./... && fmtcheck internal/service/goal/*.go internal/service/service.go
git status --short internal/http/ | grep -v '^$' && echo "^^^ handlers тронуты" || echo "handlers не тронуты"
docker info > /dev/null && go test ./...
```

---

### Task 7: `service/goalshare`, `service/goallink`, `service/teamstatus`

Три маленьких пакета одной задачей: вместе они дают шесть методов, и делить их на три ревью-гейта незачем.

**Files:**
- Create: `internal/service/goalshare/goalshare.go`, `internal/service/goallink/goallink.go`, `internal/service/goallink/goallink_test.go`, `internal/service/teamstatus/teamstatus.go`
- Modify: `internal/service/service.go`, удалить `internal/service/goal_links.go` (его содержимое распределяется)

**Interfaces:**
- Produces:

```go
package goalshare

type Repo interface { /* 6 методов GoalShareRepo из service.go:72-81, дословно */ }
type Service struct { repo Repo }
func New(repo Repo) *Service
func (s *Service) Get(ctx context.Context, scope domain.TenantScope, goalID, teamID int64) (shares.GoalShare, error)
func (s *Service) List(ctx context.Context, scope domain.TenantScope, goalID int64) ([]shares.GoalShare, error)
func (s *Service) UpdateWeight(ctx context.Context, scope domain.TenantScope, goalID, teamID int64, weight int) error
```

```go
package goallink

type Repo interface { /* 3 метода GoalLinkRepo из service.go:82-87, дословно */ }
// GoalProgressReader — узкий порт в goal.Service, нужный для AttachLinks.
type GoalProgressReader interface {
	ProgressByIDs(ctx context.Context, scope domain.TenantScope, ids []int64) (map[int64]int, error)
}
type Service struct { repo Repo; goals GoalProgressReader }
func New(repo Repo, goals GoalProgressReader) *Service
func (s *Service) ListForGoals(ctx context.Context, scope domain.TenantScope, goalIDs, allowedTeamIDs []int64, adminAll bool) (parents, children map[int64][]domain.GoalRef, err error)
func (s *Service) ListLinkable(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, adminAll bool, periodID *int64, excludeGoalID int64, q string) ([]goallinks.LinkableGoal, error)
func (s *Service) AttachLinks(ctx context.Context, scope domain.TenantScope, goals []domain.Goal, allowedTeamIDs []int64, adminAll bool) error
```

```go
package teamstatus

type Repo interface { /* 5 методов TeamStatusRepo из service.go:123-130, дословно */ }
type Service struct { repo Repo }
func New(repo Repo) *Service
func (s *Service) Get(ctx context.Context, scope domain.TenantScope, teamID, periodID int64) (domain.TeamPeriodStatus, error)
```

- [x] **Step 1: Создать три пакета**

`goalshare` и `teamstatus` — прямой перенос тел. `goallink` собирает `ListLinksForGoals`, `ListLinkableGoals`, `AttachGoalLinks`, `fillGoalRefProgress` из `internal/service/goal_links.go`. `SetGoalParents` из того же файла **не переносится** — она пишет активность и трогает два репозитория, остаётся в `Service` до этапа D. После переноса `goal_links.go` содержит только `SetGoalParents` — переименовать файл в `goal_parents.go` через `git mv`.

`AttachLinks` зависит от прогресса целей, который теперь считает `goal.Service`. Порт `GoalProgressReader` объявлен на стороне потребителя (`goallink`), реализуется `*goal.Service` — цикла импортов нет.

- [x] **Step 2: Перенести тесты**

Из `internal/service/goal_links_test.go` перенести в `internal/service/goallink/goallink_test.go` тесты `ListLinksForGoals`/`ListLinkable`/`AttachGoalLinks`; тесты `SetGoalParents` (в т.ч. на цикл) остаются в `internal/service`.

- [x] **Step 3: Подключить в фасад**

Поля `shareSvc *goalshare.Service`, `linkSvc *goallink.Service`, `statusSvc *teamstatus.Service`. `linkSvc` строится после `goalSvc`: `goallink.New(deps.GoalLinks, goalSvc)`. Делегирование шести методов под старыми именами. `GoalShareRepo` → `goalshare.Repo`, `GoalLinkRepo` → `goallink.Repo`, `TeamStatusRepo` → `teamstatus.Repo`.

- [x] **Step 4: Проверить**

```bash
go build ./... && go vet ./...
fmtcheck internal/service/goalshare/*.go internal/service/goallink/*.go internal/service/teamstatus/*.go internal/service/service.go
git status --short internal/http/ | grep -v '^$' && echo "^^^ handlers тронуты" || echo "handlers не тронуты"
docker info > /dev/null && go test ./...
```

---

### Task 8: `service/user` и `service/activity`

**Files:**
- Create: `internal/service/user/user.go`, `internal/service/activity/activity.go`, `internal/service/activity/activity_test.go`
- Modify: `internal/service/service.go`, `internal/service/activity_test.go`

**Interfaces:**
- Produces:

```go
package user

type Repo interface { /* 6 методов UserRepo из service.go:131-140, дословно */ }
type Service struct { repo Repo }
func New(repo Repo) *Service
func (s *Service) GetByDisplayNames(ctx context.Context, names []string) ([]*domain.User, error)
func (s *Service) GetByUDIDs(ctx context.Context, udids []string) ([]*domain.User, error)
func (s *Service) ListLeadTeams(ctx context.Context) (map[string]string, error)
func (s *Service) ValidateUDIDsExist(ctx context.Context, udids []string) ([]string, error)
```

```go
package activity

import storeactivity "okrs/internal/store/activity"

type Repo interface { /* 6 методов ActivityRepo из service.go:141-149, дословно */ }
type Service struct { repo Repo; logger *slog.Logger }
func New(repo Repo, logger *slog.Logger) *Service

// Record swallows and logs errors: journal writes must never fail the business
// operation that triggered them (same contract as the old service.recordActivity).
func (s *Service) Record(ctx context.Context, scope domain.TenantScope, ev domain.ActivityEvent)
func (s *Service) List(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, f storeactivity.ListFilter) ([]domain.ActivityEvent, *storeactivity.Cursor, error)
func (s *Service) TreeCounts(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, periodID *int64, since *time.Time) (map[int64]int, error)
func (s *Service) CategoryCounts(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, f storeactivity.ListFilter) (map[string]int, error)
func (s *Service) Purge(ctx context.Context, scope domain.TenantScope, olderThan *time.Time) (int64, error)
```

**Важно:** `SearchUsersInScope` в `user` **не** переносится — она читает `grants` и `teams` помимо `users` (три репозитория) и остаётся в `Service` до этапа D.

**Важно:** приватный `recordActivity` становится `activity.Service.Record`. Все 22 остающихся в `Service` метода, которые его звали, переключаются на `s.activitySvc.Record(...)` — это и есть тот вызов, который на этапе D переедет в usecase.

- [x] **Step 1: Создать оба пакета**

Тела дословно из `service.go`. `Record` сохраняет текущее поведение «проглотить и залогировать» — скопировать тело `recordActivity` как есть.

- [x] **Step 2: Перенести тесты**

`internal/service/activity_test.go` содержит `fakeActivityRepo` (строка 15) — перенести его вместе с тестами журнала в `internal/service/activity/activity_test.go` (`package activity_test`, фейк остаётся приватным для этого файла). Тесты, проверяющие *запись* активности из бизнес-сценариев (`CopyGoal`, `DeleteGoal` и т.п.), **остаются** в `internal/service`.

- [x] **Step 3: Подключить в фасад**

Поля `userSvc *user.Service`, `activitySvc *activity.Service`. Делегирование восьми методов. Заменить все `s.recordActivity(` на `s.activitySvc.Record(` и удалить приватный метод:

```bash
rg -c 's\.recordActivity\(' internal/service/*.go
rg -l 's\.recordActivity\(' internal/service/*.go | xargs perl -pi -e 's/s\.recordActivity\(/s.activitySvc.Record(/g'
```

`UserRepo` → `user.Repo`, `ActivityRepo` → `activity.Repo`.

- [x] **Step 4: Проверить**

```bash
go build ./... && go vet ./...
rg -n 'func \(s \*Service\) recordActivity' internal/service/ && echo "^^^ приватный метод остался" || echo "recordActivity удалён"
fmtcheck internal/service/user/*.go internal/service/activity/*.go internal/service/service.go
git status --short internal/http/ | grep -v '^$' && echo "^^^ handlers тронуты" || echo "handlers не тронуты"
docker info > /dev/null && go test ./...
```

---

### Task 9: Существующие сервисы в подпакеты

`SettingsService`, `ProvisioningService`, `OnboardingService` и `HealthCheckInCache` уже отдельные типы, но лежат файлами в корне `internal/service`. Без этой задачи каталог остаётся неконсистентным — девять подпакетов вперемешку с четырьмя корневыми файлами.

**Files:**
- Move: `internal/service/settings.go` + `settings_test.go` → `internal/service/settings/`
- Move: `internal/service/provisioning.go` + `provisioning_test.go` → `internal/service/provisioning/`
- Move: `internal/service/onboarding.go` + `onboarding_test.go` → `internal/service/onboarding/`
- Move: `internal/service/healthcheckin.go`, `healthcheckin_cache.go` + их тесты → `internal/service/healthcheckin/`
- Modify: `internal/service/service.go` (алиасы + конструкторы-обёртки)

**Interfaces:**
- Produces: `settings.Service`, `provisioning.Service`, `onboarding.Service`, `healthcheckin.Cache` — типы переименовываются без повтора имени пакета. Экспорт методов не меняется.
- Фасад сохраняет для handlers: `type SettingsService = settings.Service`, `type ProvisioningService = provisioning.Service`, `type OnboardingService = onboarding.Service`, `type HealthCheckInCache = healthcheckin.Cache`, `type HCActive = healthcheckin.Active`, `type PeriodData = healthcheckin.PeriodData`.

**Конструкторы нельзя выразить алиасом** — `server.go` зовёт `service.NewSettingsService(...)`, `service.NewOnboardingService(...)`, `service.NewProvisioningService(...)`, `service.NewHealthCheckInCache(...)`. Оставить в фасаде тонкие обёртки:

```go
// Обёртки нужны потому, что Go не умеет alias для функций: server.go продолжает
// звать service.NewSettingsService до этапа E, когда перейдёт на settings.New напрямую.
func NewSettingsService(tc settings.TenantCache, tr settings.TenantRepo, sc settings.SystemCache, sr settings.SystemRepo) *settings.Service {
	return settings.New(tc, tr, sc, sr)
}
```

Точные сигнатуры взять из текущих `func NewSettingsService`, `func NewOnboardingService`, `func NewProvisioningService`, `func NewHealthCheckInCache` — параметры копируются один в один.

- [x] **Step 1: Перенести четыре пакета**

```bash
cd /Users/lakosnikov.pavel/work/github.com/okrs
for p in settings provisioning onboarding; do
  mkdir -p internal/service/$p
  git mv internal/service/$p.go internal/service/$p/$p.go
  git mv internal/service/${p}_test.go internal/service/$p/${p}_test.go
done
mkdir -p internal/service/healthcheckin
git mv internal/service/healthcheckin.go        internal/service/healthcheckin/healthcheckin.go
git mv internal/service/healthcheckin_cache.go  internal/service/healthcheckin/cache.go
git mv internal/service/healthcheckin_test.go   internal/service/healthcheckin/healthcheckin_test.go
git mv internal/service/healthcheckin_cache_test.go internal/service/healthcheckin/cache_test.go
```

- [x] **Step 2: Сменить package и убрать повтор имени в типах**

```bash
for p in settings provisioning onboarding healthcheckin; do
  perl -pi -e "s/^package service/package $p/" internal/service/$p/*.go
done
perl -pi -e 's/\bSettingsService\b/Service/g'      internal/service/settings/*.go
perl -pi -e 's/\bProvisioningService\b/Service/g'  internal/service/provisioning/*.go
perl -pi -e 's/\bOnboardingService\b/Service/g'    internal/service/onboarding/*.go
perl -pi -e 's/\bHealthCheckInCache\b/Cache/g'     internal/service/healthcheckin/*.go
perl -pi -e 's/\bNewSettingsService\b/New/g'       internal/service/settings/*.go
perl -pi -e 's/\bNewProvisioningService\b/New/g'   internal/service/provisioning/*.go
perl -pi -e 's/\bNewOnboardingService\b/New/g'     internal/service/onboarding/*.go
perl -pi -e 's/\bNewHealthCheckInCache\b/NewCache/g' internal/service/healthcheckin/*.go
```

Затем компилятор перечислит перекрёстные ссылки между этими четырьмя пакетами (например, `provisioning` зависит от `settings` и `onboarding`) — расставить импорты по его указаниям. Тесты во внешнем пакете (`package settings_test` и т.п.) поправить аналогично.

- [x] **Step 3: Добавить алиасы и обёртки в фасад**

В `internal/service/service.go` — шесть type alias и четыре конструктора-обёртки из блока **Interfaces** выше.

- [x] **Step 4: Проверить**

```bash
go build ./... && go vet ./...
fmtcheck internal/service/settings/*.go internal/service/provisioning/*.go internal/service/onboarding/*.go internal/service/healthcheckin/*.go internal/service/service.go
ls internal/service/*.go   # в корне должны остаться только service.go и файлы сценариев этапа D
git status --short internal/http/ | grep -v '^$' && echo "^^^ handlers тронуты" || echo "handlers не тронуты"
docker info > /dev/null && go test ./...
```

Ожидается: exit 0, 0 FAIL.

---

### Task 10: Ревизия фасада

**Files:**
- Modify: `internal/service/service.go`

- [x] **Step 1: Убедиться, что репозиторные интерфейсы больше не дублируются**

В `service.go` не должно остаться собственных объявлений `TeamRepo`, `GoalRepo`, `GoalShareRepo`, `GoalLinkRepo`, `PeriodRepo`, `KRRepo`, `TeamStatusRepo`, `UserRepo`, `ActivityRepo` — они переехали в свои пакеты, а `Deps` ссылается на них оттуда:

```bash
rg -n '^type (Team|Goal|GoalShare|GoalLink|Period|KR|TeamStatus|User|Activity)Repo interface' internal/service/service.go \
  && echo "^^^ дубли остались" || echo "интерфейсы переехали"
```

Если какой-то остался — заменить на алиас (`type TeamRepo = team.Repo`) либо удалить, если внешних потребителей нет:

```bash
rg -n 'service\.(Team|Goal|GoalShare|GoalLink|Period|KR|TeamStatus|User|Activity)Repo' --glob '*.go' --glob '!internal/service/*'
```

- [x] **Step 2: Замерить результат**

```bash
wc -l internal/service/service.go
rg -c 'func \(s \*Service\)' internal/service/service.go
ls internal/service/*/ -d
```

Ожидается: `service.go` заметно короче 1865 строк (ориентир — около 900–1000: осталось 34 метода из 93 плюс делегирование и типы); девять новых подпакетов плюс `servicetest`.

- [x] **Step 3: Убедиться, что handlers действительно не тронуты за весь этап**

```bash
git status --short internal/http/ | grep -v '^$' && echo "^^^ ТРОНУТЫ — разобраться" || echo "handlers не тронуты за весь этап C"
```

Это ключевой инвариант этапа: фасад существует ровно ради него.

- [x] **Step 4: Полная проверка**

```bash
docker info > /dev/null && go build ./... && go vet ./... && go test ./...
```

Ожидается: exit 0, 0 FAIL, 46 `ok` среди пакетов с тестами (плюс новые пакеты с перенесёнными тестами).

---

### Task 11: Спека этапа C

**Files:**
- Modify: `specs/010-architecture-constraints.md`
- Modify: `docs/superpowers/specs/2026-08-24-layered-refactoring-design.md` (§5, §6 — расхождения)

- [x] **Step 1: Описать слой service в 010**

В разделе «Слои» строку про `internal/service` заменить на:

> - `internal/service` — сервисы по сущностям, по пакету на сущность: `team`, `goal`, `keyresult`, `period`, `goalshare`, `goallink`, `teamstatus`, `user`, `activity`, плюс `settings`, `provisioning`, `onboarding`, `healthcheckin`. Сервис работает **с одной** сущностью через **один** репозиторий, объявленный интерфейсом на стороне потребителя (`team.Repo` и т.д.), и не пишет в журнал активности. Всё, что оркестрирует несколько сервисов, — слой `usecase`. `internal/service/servicetest` — общие fake-репозитории для тестов. Корневой `service.Service` — временный фасад, сохраняющий старый API для handlers; удаляется на этапе E.

- [x] **Step 2: Зафиксировать операционное правило распила**

Туда же:

> **Граница service / usecase.** Метод принадлежит сервису сущности, если трогает не более одного репозитория и не пишет в журнал активности. Иначе это usecase. Правило операционное: его можно проверить механически, не полагаясь на вкус.

- [x] **Step 3: Поправить дизайн-док по расхождениям**

В `docs/superpowers/specs/2026-08-24-layered-refactoring-design.md`:
- из таблицы §6 убрать строку `usecase/team` (`DeleteTeam` — инвариант одной сущности, живёт в `service/team`);
- в строке `usecase/okrboard` убрать `GetHierarchy` (однорепозиторный → `service/team`), оставив `GetTeamOKR`, `GetTeamOverview`, `GetDirectChildrenSummary`, `GetTeamsWithPeriodSummary`;
- в §5 добавить, что операционное правило распила — «≤1 репозитория и без записи активности», и что оно уточнило §6.

- [x] **Step 4: Проверить консистентность**

```bash
rg -n 'internal/service` — доменные сценарии' specs/010-architecture-constraints.md && echo "^^^ старая формулировка осталась" || echo "010 обновлена"
rg -n 'usecase/team' docs/superpowers/specs/2026-08-24-layered-refactoring-design.md && echo "^^^ расхождение осталось" || echo "дизайн-док согласован"
```

- [x] **Step 5: Финальный прогон**

```bash
docker info > /dev/null && go build ./... && go vet ./... && go test ./...
git status --short | awk '{print $1}' | sort | uniq -c
```

---

## Что НЕ входит в этап C

- **Слой `usecase`** — этап D. 34 метода остаются в `service.Service`.
- **Вынос фоновых задач в `internal/scheduler`** — этап D.
- **Перевязка handlers** и удаление фасада — этап E.
- **Разбиение handlers по URI** и `specs/070-code-structure.md` — этап E.
- **Удаление мёртвого кода** (`handlers/web/keyresults`) — этап E.
