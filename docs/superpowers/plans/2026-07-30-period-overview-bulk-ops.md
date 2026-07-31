# Обзор периода, массовые операции и иконочные контролы — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Добавить в раздел «Периоды целеполагания» иконочные контролы строк, метрики строк (грузятся отдельным запросом), модалку «Управление периодом» с обзором команд по статусам/весам/прогрессу и двумя массовыми операциями (перевод команд в «В работе», закрытие всех целей периода).

**Architecture:** Backend — общий чистый агрегатор `computePeriodOverview` поверх кэшированного `PeriodData` (teams + goalsByTeam + statuses), обслуживающий batch-статистику строк и полный обзор модалки; массовые переходы — чистый расчёт затронутых команд + один batch-upsert статусов + один batch-insert op-лога; кэш инвалидируется после мутации. Frontend — общий компонент `IconBtn`, перерисованные действия строки, асинхронный догруз метрик, модалка «Управление периодом». Слои не смешиваются: store (SQL) → service (домен/агрегация) → http (handlers/DTO) → static JS (React/JSX).

**Tech Stack:** Go (chi, pgx), React через JSX (in-browser, без сборки), PostgreSQL.

## Global Constraints

- Source of truth — specs: `README-specs.md`, `specs/010`…`specs/050`. Правки specs — в этом же change set (`specs/040-api-contract.md`, `specs/050-permissions-and-lifecycle.md`); несвязанные specs не трогать.
- Чистая/слоистая архитектура — не протекать абстракциями между слоями.
- Никаких запросов в базу в цикле — только batch/множественные запросы за один round-trip.
- Не делать git-коммитов от имени агента — коммитит пользователь сам. Шаги «Commit» ниже пользователь выполняет вручную; агент останавливается на готовом, проверенном изменении.
- Тесты держать в актуальном состоянии, покрывать новый функционал.
- Не упоминать Claude/AI/ассистентов в коде, комментариях, спеках.
- Дизайн-доки и спеки — на русском. Комментарии в коде — в стиле окружающего кода (в этом репозитории по-английски).
- `seed_demo.sql` обновлять только при изменении структуры таблиц (в этом плане структура не меняется — не трогаем).
- Русские статусы в UI: `in_progress`=«В работе», `ready`=«К валидации», `forming`=«Черновик», `closed`=«Закрыто», `no_goals`=«Нет целей».
- Легаси-статус `validated` (после миграции 018 не используется приложением) в агрегации считается в бакете `in_progress`.

**Спека этой фичи:** `docs/superpowers/specs/2026-07-30-period-overview-bulk-ops-design.md`

---

### Task 1: Store — batch-методы для статусов и op-лога

**Files:**
- Modify: `internal/store/statuses/statuses.go`
- Modify: `internal/store/activity/activity.go`
- Modify: `internal/service/service.go` (расширить интерфейсы `TeamStatusRepo`, `ActivityRepo`)
- Test: `internal/store/statuses/statuses_test.go`
- Test: `internal/store/activity/activity_test.go`

**Interfaces:**
- Consumes: существующие таблицы `team_period_statuses` (unique `(team_id, period_id)`), `activity_events`.
- Produces:
  - `func (r *TeamStatusRepository) SetTeamPeriodStatuses(ctx context.Context, scope domain.TenantScope, periodID int64, teamIDs []int64, status domain.TeamPeriodStatus) error`
  - `func (r *ActivityRepository) RecordBatch(ctx context.Context, scope domain.TenantScope, evs []domain.ActivityEvent) error`
  - интерфейс `TeamStatusRepo` получает метод `SetTeamPeriodStatuses(...)`; `ActivityRepo` — метод `RecordBatch(...)`.

- [ ] **Step 1: Write failing store test for batch status upsert**

В `internal/store/statuses/statuses_test.go` добавь:

```go
func TestSetTeamPeriodStatuses_Batch(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	r := statuses.NewTeamStatusRepository(pool)

	var periodID int64
	pool.QueryRow(ctx, `INSERT INTO periods (name,start_date,end_date) VALUES ('BulkP','2025-01-01','2025-03-31') RETURNING id`).Scan(&periodID)
	ids := make([]int64, 3)
	for i := range ids {
		pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('BT') RETURNING id`).Scan(&ids[i])
	}
	// Seed one team already closed to prove the batch overwrites existing rows too.
	if err := r.SetTeamPeriodStatus(ctx, sc1, ids[0], periodID, domain.TeamPeriodStatusClosed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := r.SetTeamPeriodStatuses(ctx, sc1, periodID, ids, domain.TeamPeriodStatusInProgress); err != nil {
		t.Fatalf("SetTeamPeriodStatuses: %v", err)
	}
	got, err := r.ListTeamPeriodStatuses(ctx, sc1, periodID, ids)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, id := range ids {
		if got[id] != domain.TeamPeriodStatusInProgress {
			t.Fatalf("team %d: expected in_progress, got %s", id, got[id])
		}
	}

	// Empty slice is a no-op, not an error.
	if err := r.SetTeamPeriodStatuses(ctx, sc1, periodID, nil, domain.TeamPeriodStatusClosed); err != nil {
		t.Fatalf("empty slice must be no-op: %v", err)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/store/statuses/ -run TestSetTeamPeriodStatuses_Batch -v`
Expected: FAIL to compile (`SetTeamPeriodStatuses` undefined). If docker unavailable the test SKIPS — in that case verify compile with `go vet ./internal/store/statuses/` (must report the undefined method).

- [ ] **Step 3: Implement batch status upsert**

В `internal/store/statuses/statuses.go` добавь после `SetTeamPeriodStatus`:

```go
// SetTeamPeriodStatuses upserts the same status for many teams in one round-trip.
// Empty teamIDs is a no-op.
func (r *TeamStatusRepository) SetTeamPeriodStatuses(ctx context.Context, scope domain.TenantScope, periodID int64, teamIDs []int64, status domain.TeamPeriodStatus) error {
	if len(teamIDs) == 0 {
		return nil
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO team_period_statuses (team_id, period_id, status, tenant_id, updated_at)
		SELECT unnest($1::bigint[]), $2, $3, $4, NOW()
		ON CONFLICT (team_id, period_id)
		DO UPDATE SET status=EXCLUDED.status, updated_at=NOW()`,
		teamIDs, periodID, status, scope.TenantID,
	)
	return err
}
```

- [ ] **Step 4: Run it to verify it passes**

Run: `go test ./internal/store/statuses/ -run TestSetTeamPeriodStatuses_Batch -v`
Expected: PASS (or SKIP if docker unavailable — then `go build ./internal/store/statuses/` must succeed).

- [ ] **Step 5: Write failing store test for batch activity insert**

В `internal/store/activity/activity_test.go` добавь (используй тот же харнесс, что и соседние тесты в файле — `testutil.SetupDB`; сверься с существующим helper для scope/actor в этом файле):

```go
func TestRecordBatch(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	r := activity.NewActivityRepository(pool)
	scope := domain.TenantScope{TenantID: 1}

	t1, t2 := int64(101), int64(102)
	p := int64(9)
	evs := []domain.ActivityEvent{
		{ActorUserID: 1, Category: domain.ActivityStatus, Action: domain.ActionStatusChanged, TeamID: &t1, PeriodID: &p, EntityTitle: "A", Payload: map[string]any{"after": map[string]any{"status": "in_progress"}}},
		{ActorUserID: 1, Category: domain.ActivityStatus, Action: domain.ActionStatusChanged, TeamID: &t2, PeriodID: &p, EntityTitle: "B", Payload: map[string]any{"after": map[string]any{"status": "in_progress"}}},
	}
	if err := r.RecordBatch(ctx, scope, evs); err != nil {
		t.Fatalf("RecordBatch: %v", err)
	}

	var n int
	pool.QueryRow(ctx, `SELECT count(*) FROM activity_events WHERE tenant_id=$1 AND period_id=$2 AND action='status_changed'`, scope.TenantID, p).Scan(&n)
	if n != 2 {
		t.Fatalf("expected 2 rows, got %d", n)
	}

	// Empty slice is a no-op.
	if err := r.RecordBatch(ctx, scope, nil); err != nil {
		t.Fatalf("empty must be no-op: %v", err)
	}
}
```

- [ ] **Step 6: Run it to verify it fails**

Run: `go test ./internal/store/activity/ -run TestRecordBatch -v`
Expected: FAIL (`RecordBatch` undefined) or SKIP without docker — then `go vet ./internal/store/activity/` reports undefined.

- [ ] **Step 7: Implement batch activity insert**

В `internal/store/activity/activity.go` добавь после `Record`. Импортни `github.com/jackc/pgx/v5` если ещё не импортирован (проверь блок import — `pgx` уже используется в файле):

```go
// RecordBatch inserts many events in a single pipelined round-trip. Empty is a no-op.
func (r *ActivityRepository) RecordBatch(ctx context.Context, scope domain.TenantScope, evs []domain.ActivityEvent) error {
	if len(evs) == 0 {
		return nil
	}
	b := &pgx.Batch{}
	for _, ev := range evs {
		payload := ev.Payload
		if payload == nil {
			payload = map[string]any{}
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		b.Queue(`
			INSERT INTO activity_events
				(tenant_id, actor_user_id, category, action, team_id, period_id, goal_id, kr_id, comment_id, entity_title, payload_json)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			scope.TenantID, ev.ActorUserID, ev.Category, ev.Action,
			ev.TeamID, ev.PeriodID, ev.GoalID, ev.KRID, ev.CommentID, ev.EntityTitle, raw,
		)
	}
	br := r.db.SendBatch(ctx, b)
	defer br.Close()
	for range evs {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 8: Extend service repo interfaces**

В `internal/service/service.go` в интерфейс `TeamStatusRepo` (около строки 108) добавь строку:

```go
	SetTeamPeriodStatuses(ctx context.Context, scope domain.TenantScope, periodID int64, teamIDs []int64, status domain.TeamPeriodStatus) error
```

В интерфейс `ActivityRepo` (около строки 125) добавь:

```go
	RecordBatch(ctx context.Context, scope domain.TenantScope, evs []domain.ActivityEvent) error
```

- [ ] **Step 9: Run it to verify everything passes**

Run: `go build ./... && go test ./internal/store/activity/ ./internal/store/statuses/ -run 'RecordBatch|SetTeamPeriodStatuses_Batch' -v`
Expected: PASS or SKIP (docker); build must be green (interfaces satisfied by concrete repos).

- [ ] **Step 10: Commit** (пользователь выполняет вручную)

```bash
git add internal/store/statuses/ internal/store/activity/ internal/service/service.go
git commit -m "store: batch team-period-status upsert and batch activity insert"
```

---

### Task 2: Service — чистый агрегатор обзора периода

**Files:**
- Create: `internal/service/period_overview.go`
- Test: `internal/service/period_overview_test.go`

**Interfaces:**
- Consumes: `*PeriodData` (`internal/service/healthcheckin.go:129`), `domain.TeamPeriodStatus*` константы, `CalculateGoalProgress` (`internal/service/progress.go`), `okr.PeriodProgress` (`internal/okr/okr.go`), `buildTeamPath` (`internal/service/healthcheckin.go:529`), `abs` (`internal/service/healthcheckin.go:547`).
- Produces (в пакете `service`):
  - типы `PeriodTeamSummary`, `PeriodOverviewSummary`, `PeriodOverview`, `PeriodStatsItem` (json-теги ниже — сериализуются напрямую в HTTP, как `HealthCheckInResult`)
  - `func computePeriodOverview(data *PeriodData, weightTolerance int) PeriodOverview`
  - `func bucketStatusWithGoals(s domain.TeamPeriodStatus) string`

- [ ] **Step 1: Write failing aggregator test**

Create `internal/service/period_overview_test.go`:

```go
package service

import (
	"testing"

	"okrs/internal/domain"
)

// numericKR builds a numerical KR with a known progress (current/target of 100%).
func numericKR(id int64, weight, current int) domain.KeyResult {
	return domain.KeyResult{
		ID: id, Weight: weight, Kind: domain.KRKindNumerical,
		Numerical: &domain.KRNumerical{StartValue: 0, TargetValue: 100, CurrentValue: float64(current)},
	}
}

func TestComputePeriodOverview_CountsWeightsProgress(t *testing.T) {
	teams := []domain.Team{
		{ID: 1, Name: "Alpha"},
		{ID: 2, Name: "Beta"},
		{ID: 3, Name: "Gamma"}, // no goals
	}
	goalsByTeam := map[int64][]domain.Goal{
		// Alpha: one goal weight 100, progress 40 -> team progress 40, weight ok.
		1: {{ID: 10, TeamID: 1, Weight: 100, KeyResults: []domain.KeyResult{numericKR(100, 100, 40)}}},
		// Beta: two goals weights 50+40=90 (weight error), progress 100 & 0 -> weighted 55.
		2: {
			{ID: 20, TeamID: 2, Weight: 50, KeyResults: []domain.KeyResult{numericKR(200, 100, 100)}},
			{ID: 21, TeamID: 2, Weight: 40, KeyResults: []domain.KeyResult{numericKR(201, 100, 0)}},
		},
	}
	statuses := map[int64]domain.TeamPeriodStatus{
		1: domain.TeamPeriodStatusInProgress,
		2: domain.TeamPeriodStatusReady,
	}
	data := &PeriodData{PeriodID: 7, Teams: teams, GoalsByTeam: goalsByTeam, Statuses: statuses}

	ov := computePeriodOverview(data, 0)

	if ov.Summary.TotalTeams != 3 {
		t.Fatalf("total_teams: want 3, got %d", ov.Summary.TotalTeams)
	}
	if ov.Summary.TeamsWithGoals != 2 {
		t.Fatalf("teams_with_goals: want 2, got %d", ov.Summary.TeamsWithGoals)
	}
	if ov.Summary.ByStatus["in_progress"] != 1 || ov.Summary.ByStatus["ready"] != 1 || ov.Summary.ByStatus["no_goals"] != 1 {
		t.Fatalf("by_status wrong: %+v", ov.Summary.ByStatus)
	}
	if ov.Summary.WeightErrorCount != 1 {
		t.Fatalf("weight_error_count: want 1 (Beta), got %d", ov.Summary.WeightErrorCount)
	}
	// avg of Alpha(40) and Beta(round(100*50+0*40)/90=56) = round((40+56)/2)=48
	if ov.Summary.AvgProgress != 48 {
		t.Fatalf("avg_progress: want 48, got %d", ov.Summary.AvgProgress)
	}
}

func TestComputePeriodOverview_ValidatedCountsAsInProgress(t *testing.T) {
	data := &PeriodData{
		PeriodID: 1,
		Teams:    []domain.Team{{ID: 1, Name: "A"}},
		GoalsByTeam: map[int64][]domain.Goal{
			1: {{ID: 1, TeamID: 1, Weight: 100, KeyResults: []domain.KeyResult{numericKR(1, 100, 0)}}},
		},
		Statuses: map[int64]domain.TeamPeriodStatus{1: domain.TeamPeriodStatus("validated")},
	}
	ov := computePeriodOverview(data, 0)
	if ov.Summary.ByStatus["in_progress"] != 1 {
		t.Fatalf("validated must bucket as in_progress: %+v", ov.Summary.ByStatus)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/service/ -run TestComputePeriodOverview -v`
Expected: FAIL to compile (`computePeriodOverview`, types undefined).

- [ ] **Step 3: Implement the aggregator**

Create `internal/service/period_overview.go`:

```go
package service

import (
	"math"
	"sort"

	"okrs/internal/domain"
	"okrs/internal/okr"
)

// PeriodTeamSummary is one team's row in the period overview (drill-down source).
type PeriodTeamSummary struct {
	TeamID      int64    `json:"id"`
	TeamName    string   `json:"name"`
	TeamPath    []string `json:"path"`
	Status      string   `json:"status"`
	GoalsCount  int      `json:"goals_count"`
	Progress    int      `json:"progress"`
	WeightSum   int      `json:"weight_sum"`
	WeightError bool     `json:"weight_error"`
}

// PeriodOverviewSummary is the aggregate shown in the overview tiles.
type PeriodOverviewSummary struct {
	ByStatus         map[string]int `json:"by_status"`
	TotalTeams       int            `json:"total_teams"`
	TeamsWithGoals   int            `json:"teams_with_goals"`
	WeightErrorCount int            `json:"weight_error_count"`
	AvgProgress      int            `json:"avg_progress"`
}

// PeriodOverview is the full response for the management modal.
type PeriodOverview struct {
	PeriodID int64                 `json:"period_id"`
	Summary  PeriodOverviewSummary `json:"summary"`
	Teams    []PeriodTeamSummary   `json:"teams"`
}

// PeriodStatsItem is the lightweight per-period row metric (no team composition).
type PeriodStatsItem struct {
	PeriodID         int64 `json:"period_id"`
	TotalTeams       int   `json:"total_teams"`
	TeamsWithGoals   int   `json:"teams_with_goals"`
	AvgProgress      int   `json:"avg_progress"`
	WeightErrorCount int   `json:"weight_error_count"`
}

// bucketStatusWithGoals maps a team-with-goals status to one of the four overview
// buckets. Legacy "validated" (unused since migration 018) folds into in_progress;
// any other/unknown value defaults to forming.
func bucketStatusWithGoals(s domain.TeamPeriodStatus) string {
	switch s {
	case domain.TeamPeriodStatusInProgress, domain.TeamPeriodStatus("validated"):
		return "in_progress"
	case domain.TeamPeriodStatusReady:
		return "ready"
	case domain.TeamPeriodStatusClosed:
		return "closed"
	case domain.TeamPeriodStatusForming:
		return "forming"
	default:
		return "forming"
	}
}

// computePeriodOverview aggregates a period's teams into status buckets, weight-error
// counts and average progress, plus a per-team composition list. Pure — no I/O.
func computePeriodOverview(data *PeriodData, weightTolerance int) PeriodOverview {
	teamsByID := make(map[int64]domain.Team, len(data.Teams))
	for _, t := range data.Teams {
		if t.DeletedAt != nil {
			continue
		}
		teamsByID[t.ID] = t
	}

	byStatus := map[string]int{"in_progress": 0, "ready": 0, "forming": 0, "closed": 0, "no_goals": 0}
	out := make([]PeriodTeamSummary, 0, len(teamsByID))
	var progressSum, progressCount, weightErrors, teamsWithGoals int

	for id, team := range teamsByID {
		// Copy goals so CalculateGoalProgress writes don't mutate shared cache data.
		src := data.GoalsByTeam[id]
		goals := make([]domain.Goal, len(src))
		copy(goals, src)

		status := data.Statuses[id]
		if status == "" {
			status = domain.TeamPeriodStatusNoGoals
		}

		row := PeriodTeamSummary{
			TeamID:     id,
			TeamName:   team.Name,
			TeamPath:   buildTeamPath(id, teamsByID),
			Status:     string(status),
			GoalsCount: len(goals),
		}

		if len(goals) == 0 {
			byStatus["no_goals"]++
		} else {
			byStatus[bucketStatusWithGoals(status)]++
			teamsWithGoals++
			weightSum := 0
			for i := range goals {
				goals[i].Progress = CalculateGoalProgress(&goals[i])
				weightSum += goals[i].Weight
			}
			row.WeightSum = weightSum
			row.WeightError = abs(weightSum-100) > weightTolerance
			if row.WeightError {
				weightErrors++
			}
			row.Progress = okr.PeriodProgress(goals)
			progressSum += row.Progress
			progressCount++
		}
		out = append(out, row)
	}

	avg := 0
	if progressCount > 0 {
		avg = int(math.Round(float64(progressSum) / float64(progressCount)))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TeamName < out[j].TeamName })

	return PeriodOverview{
		PeriodID: data.PeriodID,
		Summary: PeriodOverviewSummary{
			ByStatus:         byStatus,
			TotalTeams:       len(teamsByID),
			TeamsWithGoals:   teamsWithGoals,
			WeightErrorCount: weightErrors,
			AvgProgress:      avg,
		},
		Teams: out,
	}
}
```

- [ ] **Step 4: Run it to verify it passes**

Run: `go test ./internal/service/ -run TestComputePeriodOverview -v`
Expected: PASS (both cases).

- [ ] **Step 5: Commit** (пользователь вручную)

```bash
git add internal/service/period_overview.go internal/service/period_overview_test.go
git commit -m "service: pure period-overview aggregator"
```

---

### Task 3: Service — методы `PeriodOverview` и `PeriodStats`

**Files:**
- Modify: `internal/service/period_overview.go` (добавить методы на `*Service`)
- Test: `internal/service/period_overview_test.go`

**Interfaces:**
- Consumes: `s.hcCache` (`*HealthCheckInCache`, поле `Service.hcCache`), `s.periods.ListPeriods`, `computePeriodOverview` (Task 2). Загрузчик кэша уже включает KRs (`ListGoalsByTeamsPeriod`).
- Produces:
  - `func (s *Service) PeriodOverview(ctx context.Context, scope domain.TenantScope, periodID int64, weightTolerance int) (PeriodOverview, error)`
  - `func (s *Service) PeriodStats(ctx context.Context, scope domain.TenantScope, weightTolerance int) ([]PeriodStatsItem, error)`

- [ ] **Step 1: Write failing service-method test (cache with canned loader)**

Добавь в `internal/service/period_overview_test.go`:

```go
import (
	"context"
	"time"
)

func TestServicePeriodOverview_UsesCache(t *testing.T) {
	data := &PeriodData{
		PeriodID: 5,
		Teams:    []domain.Team{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}},
		GoalsByTeam: map[int64][]domain.Goal{
			1: {{ID: 1, TeamID: 1, Weight: 100, KeyResults: []domain.KeyResult{numericKR(1, 100, 60)}}},
		},
		Statuses: map[int64]domain.TeamPeriodStatus{1: domain.TeamPeriodStatusInProgress},
		CachedAt: time.Now(),
	}
	loader := func(_ context.Context, _ domain.TenantScope, _ int64) (*PeriodData, error) { return data, nil }
	cache := NewHealthCheckInCache(loader, time.Minute, nil)
	s := &Service{hcCache: cache}

	ov, err := s.PeriodOverview(context.Background(), domain.TenantScope{TenantID: 1}, 5, 0)
	if err != nil {
		t.Fatalf("PeriodOverview: %v", err)
	}
	if ov.Summary.TotalTeams != 2 || ov.Summary.TeamsWithGoals != 1 || ov.Summary.AvgProgress != 60 {
		t.Fatalf("overview wrong: %+v", ov.Summary)
	}
}
```

Note: `numericKR` уже определён в Task 2's test file (same package).

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/service/ -run TestServicePeriodOverview_UsesCache -v`
Expected: FAIL to compile (`PeriodOverview` method undefined).

- [ ] **Step 3: Implement service methods**

Добавь в конец `internal/service/period_overview.go` (импорт `context` уже понадобится — добавь в import-блок `"context"`):

```go
// PeriodOverview returns the full overview (summary + team composition) for one period.
func (s *Service) PeriodOverview(ctx context.Context, scope domain.TenantScope, periodID int64, weightTolerance int) (PeriodOverview, error) {
	if s.hcCache == nil {
		return PeriodOverview{PeriodID: periodID}, nil
	}
	data, err := s.hcCache.Get(ctx, scope, periodID)
	if err != nil {
		return PeriodOverview{}, err
	}
	return computePeriodOverview(data, weightTolerance), nil
}

// PeriodStats returns lightweight per-period metrics for every period (no team lists).
func (s *Service) PeriodStats(ctx context.Context, scope domain.TenantScope, weightTolerance int) ([]PeriodStatsItem, error) {
	if s.hcCache == nil {
		return []PeriodStatsItem{}, nil
	}
	periods, err := s.periods.ListPeriods(ctx, scope)
	if err != nil {
		return nil, err
	}
	items := make([]PeriodStatsItem, 0, len(periods))
	for _, p := range periods {
		data, err := s.hcCache.Get(ctx, scope, p.ID)
		if err != nil {
			return nil, err
		}
		ov := computePeriodOverview(data, weightTolerance)
		items = append(items, PeriodStatsItem{
			PeriodID:         p.ID,
			TotalTeams:       ov.Summary.TotalTeams,
			TeamsWithGoals:   ov.Summary.TeamsWithGoals,
			AvgProgress:      ov.Summary.AvgProgress,
			WeightErrorCount: ov.Summary.WeightErrorCount,
		})
	}
	return items, nil
}
```

Note: `s.hcCache.Get` в цикле по периодам — это доступ к in-memory кэшу с TTL, а не запрос в БД в цикле; при холодном кэше каждый период грузится один раз пачкой репозиториев внутри loader. Периодов немного (год + кварталы).

- [ ] **Step 4: Run it to verify it passes**

Run: `go test ./internal/service/ -run 'TestServicePeriodOverview_UsesCache|TestComputePeriodOverview' -v`
Expected: PASS.

- [ ] **Step 5: Commit** (пользователь вручную)

```bash
git add internal/service/period_overview.go internal/service/period_overview_test.go
git commit -m "service: PeriodOverview and PeriodStats over cached period data"
```

---

### Task 4: Service — массовый переход статусов команд

**Files:**
- Create: `internal/service/period_bulk_status.go`
- Test: `internal/service/period_bulk_status_test.go`

**Interfaces:**
- Consumes: `s.teams.ListAllTeams`, `s.goals.ListGoalsByTeamsPeriod`, `s.statuses.ListTeamPeriodStatuses`, `s.statuses.SetTeamPeriodStatuses` (Task 1), `s.activity.RecordBatch` (Task 1), `s.hcCache.InvalidateAll`, `domain.ActivityEvent`.
- Produces:
  - тип `BulkStatusResult{ Affected int; Skipped int }`
  - `func computeBulkAffected(teams []domain.Team, goalsByTeam map[int64][]domain.Goal, statuses map[int64]domain.TeamPeriodStatus, target domain.TeamPeriodStatus) (affected []int64, skipped int)`
  - `func (s *Service) BulkSetTeamPeriodStatus(ctx context.Context, scope domain.TenantScope, periodID int64, target domain.TeamPeriodStatus, actorUserID int64) (BulkStatusResult, error)`

- [ ] **Step 1: Write failing pure-filter test**

Create `internal/service/period_bulk_status_test.go`:

```go
package service

import (
	"testing"

	"okrs/internal/domain"
)

func TestComputeBulkAffected_SkipsNoGoalsAndAlreadyTarget(t *testing.T) {
	teams := []domain.Team{
		{ID: 1, Name: "HasGoals-Ready"},
		{ID: 2, Name: "HasGoals-AlreadyInProgress"},
		{ID: 3, Name: "NoGoals"},
		{ID: 4, Name: "Deleted", DeletedAt: timePtr(nowForTest())},
	}
	goalsByTeam := map[int64][]domain.Goal{
		1: {{ID: 10, TeamID: 1, Weight: 100}},
		2: {{ID: 20, TeamID: 2, Weight: 100}},
		4: {{ID: 40, TeamID: 4, Weight: 100}},
	}
	statuses := map[int64]domain.TeamPeriodStatus{
		1: domain.TeamPeriodStatusReady,
		2: domain.TeamPeriodStatusInProgress,
	}
	affected, skipped := computeBulkAffected(teams, goalsByTeam, statuses, domain.TeamPeriodStatusInProgress)

	if len(affected) != 1 || affected[0] != 1 {
		t.Fatalf("affected: want [1], got %v", affected)
	}
	if skipped != 1 { // only team 3 (no goals); team 2 already target, team 4 deleted
		t.Fatalf("skipped: want 1, got %d", skipped)
	}
}
```

`timePtr` уже есть в `healthcheckin_test.go` (same package). Add a tiny helper if a current time is needed:

```go
func nowForTest() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
```

(добавь `import "time"` в этот тест-файл).

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/service/ -run TestComputeBulkAffected -v`
Expected: FAIL to compile (`computeBulkAffected` undefined).

- [ ] **Step 3: Implement pure filter + service method**

Create `internal/service/period_bulk_status.go`:

```go
package service

import (
	"context"
	"sort"

	"okrs/internal/domain"
)

// BulkStatusResult reports the outcome of a bulk team-period-status transition.
type BulkStatusResult struct {
	Affected int `json:"affected"`
	Skipped  int `json:"skipped"`
}

// computeBulkAffected returns the teams whose status must change to target, and the
// count of teams skipped for having no goals. Teams already in target and deleted
// teams are neither affected nor skipped (idempotent). Pure — no I/O.
func computeBulkAffected(teams []domain.Team, goalsByTeam map[int64][]domain.Goal, statuses map[int64]domain.TeamPeriodStatus, target domain.TeamPeriodStatus) (affected []int64, skipped int) {
	for _, t := range teams {
		if t.DeletedAt != nil {
			continue
		}
		if len(goalsByTeam[t.ID]) == 0 {
			skipped++
			continue
		}
		cur := statuses[t.ID]
		if cur == "" {
			cur = domain.TeamPeriodStatusNoGoals
		}
		if cur == target {
			continue
		}
		affected = append(affected, t.ID)
	}
	sort.Slice(affected, func(i, j int) bool { return affected[i] < affected[j] })
	return affected, skipped
}

// BulkSetTeamPeriodStatus sets target status for every team that has at least one goal
// in the period and is not already in target. Writes one op-log entry per affected team
// and invalidates the period cache. Loads fresh data (not cached) to decide the set.
func (s *Service) BulkSetTeamPeriodStatus(ctx context.Context, scope domain.TenantScope, periodID int64, target domain.TeamPeriodStatus, actorUserID int64) (BulkStatusResult, error) {
	allTeams, err := s.teams.ListAllTeams(ctx, scope)
	if err != nil {
		return BulkStatusResult{}, err
	}
	teamIDs := make([]int64, 0, len(allTeams))
	nameByID := make(map[int64]string, len(allTeams))
	for _, t := range allTeams {
		if t.DeletedAt != nil {
			continue
		}
		teamIDs = append(teamIDs, t.ID)
		nameByID[t.ID] = t.Name
	}

	goalsByTeam, err := s.goals.ListGoalsByTeamsPeriod(ctx, scope, periodID, teamIDs)
	if err != nil {
		return BulkStatusResult{}, err
	}
	statuses, err := s.statuses.ListTeamPeriodStatuses(ctx, scope, periodID, teamIDs)
	if err != nil {
		return BulkStatusResult{}, err
	}

	affected, skipped := computeBulkAffected(allTeams, goalsByTeam, statuses, target)
	if len(affected) == 0 {
		return BulkStatusResult{Affected: 0, Skipped: skipped}, nil
	}

	if err := s.statuses.SetTeamPeriodStatuses(ctx, scope, periodID, affected, target); err != nil {
		return BulkStatusResult{}, err
	}

	evs := make([]domain.ActivityEvent, 0, len(affected))
	for _, id := range affected {
		before := statuses[id]
		if before == "" {
			before = domain.TeamPeriodStatusNoGoals
		}
		tID, pID := id, periodID
		evs = append(evs, domain.ActivityEvent{
			ActorUserID: actorUserID,
			Category:    domain.ActivityStatus,
			Action:      domain.ActionStatusChanged,
			TeamID:      &tID,
			PeriodID:    &pID,
			EntityTitle: nameByID[id],
			Payload: map[string]any{
				"before": map[string]any{"status": string(before)},
				"after":  map[string]any{"status": string(target)},
				"bulk":   true,
			},
		})
	}
	if err := s.activity.RecordBatch(ctx, scope, evs); err != nil && s.logger != nil {
		s.logger.Warn("bulk status: activity record failed", "period", periodID, "err", err)
	}

	if s.hcCache != nil {
		s.hcCache.InvalidateAll()
	}
	return BulkStatusResult{Affected: len(affected), Skipped: skipped}, nil
}
```

- [ ] **Step 4: Write failing service-method test with fakes**

Добавь в `internal/service/period_bulk_status_test.go` минимальные фейки и happy-path тест. Фейки реализуют только методы, которые дёргает `BulkSetTeamPeriodStatus`:

```go
type bulkFakeTeams struct{ teams []domain.Team }

func (f *bulkFakeTeams) ListAllTeams(context.Context, domain.TenantScope) ([]domain.Team, error) {
	return f.teams, nil
}

type bulkFakeGoals struct{ byTeam map[int64][]domain.Goal }

func (f *bulkFakeGoals) ListGoalsByTeamsPeriod(_ context.Context, _ domain.TenantScope, _ int64, _ []int64) (map[int64][]domain.Goal, error) {
	return f.byTeam, nil
}

type bulkFakeStatuses struct {
	cur       map[int64]domain.TeamPeriodStatus
	setCalls  [][]int64
	setStatus domain.TeamPeriodStatus
}

func (f *bulkFakeStatuses) ListTeamPeriodStatuses(_ context.Context, _ domain.TenantScope, _ int64, _ []int64) (map[int64]domain.TeamPeriodStatus, error) {
	return f.cur, nil
}
func (f *bulkFakeStatuses) SetTeamPeriodStatuses(_ context.Context, _ domain.TenantScope, _ int64, ids []int64, st domain.TeamPeriodStatus) error {
	f.setCalls = append(f.setCalls, ids)
	f.setStatus = st
	return nil
}

type bulkFakeActivity struct{ batches [][]domain.ActivityEvent }

func (f *bulkFakeActivity) RecordBatch(_ context.Context, _ domain.TenantScope, evs []domain.ActivityEvent) error {
	f.batches = append(f.batches, evs)
	return nil
}

func TestBulkSetTeamPeriodStatus_ActivatesAndLogsPerTeam(t *testing.T) {
	teamsRepo := &bulkFakeTeams{teams: []domain.Team{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}, {ID: 3, Name: "C"}}}
	goalsRepo := &bulkFakeGoals{byTeam: map[int64][]domain.Goal{
		1: {{ID: 10}}, // ready -> affected
		2: {{ID: 20}}, // already in_progress -> skip (no change)
		// team 3: no goals -> skipped
	}}
	statusRepo := &bulkFakeStatuses{cur: map[int64]domain.TeamPeriodStatus{
		1: domain.TeamPeriodStatusReady,
		2: domain.TeamPeriodStatusInProgress,
	}}
	actRepo := &bulkFakeActivity{}
	s := &Service{teams: teamsRepo, goals: goalsRepo, statuses: statusRepo, activity: actRepo}

	res, err := s.BulkSetTeamPeriodStatus(context.Background(), domain.TenantScope{TenantID: 1}, 9, domain.TeamPeriodStatusInProgress, 42)
	if err != nil {
		t.Fatalf("bulk: %v", err)
	}
	if res.Affected != 1 || res.Skipped != 1 {
		t.Fatalf("result: want affected=1 skipped=1, got %+v", res)
	}
	if len(statusRepo.setCalls) != 1 || len(statusRepo.setCalls[0]) != 1 || statusRepo.setCalls[0][0] != 1 {
		t.Fatalf("set call wrong: %+v", statusRepo.setCalls)
	}
	if len(actRepo.batches) != 1 || len(actRepo.batches[0]) != 1 {
		t.Fatalf("expected one op-log entry per affected team, got %+v", actRepo.batches)
	}
	if actRepo.batches[0][0].EntityTitle != "A" {
		t.Fatalf("op-log entity title should be team name, got %q", actRepo.batches[0][0].EntityTitle)
	}
}
```

Note: типы фейков должны удовлетворять существующим интерфейсам `TeamRepo`/`GoalRepo`/`TeamStatusRepo`/`ActivityRepo` только в объёме вызываемых методов — но поля `Service` типизированы этими интерфейсами целиком. Поэтому фейки обязаны реализовать **все** методы соответствующих интерфейсов. Если интерфейсы шире — добавь недостающие методы-заглушки (возвращают нулевые значения), либо, если в пакете уже есть готовые фейки для этих интерфейсов, переиспользуй их. Перед реализацией сверься: `rg -n "type TeamRepo interface" -A15 internal/service/service.go` и аналогично для остальных, и допиши в фейки все методы интерфейса.

- [ ] **Step 5: Run it to verify it passes**

Run: `go test ./internal/service/ -run 'TestComputeBulkAffected|TestBulkSetTeamPeriodStatus' -v`
Expected: PASS.

- [ ] **Step 6: Commit** (пользователь вручную)

```bash
git add internal/service/period_bulk_status.go internal/service/period_bulk_status_test.go
git commit -m "service: bulk team-period-status transition with per-team op-log"
```

---

### Task 5: HTTP — эндпоинты overview/stats/activate/close + routes + specs

**Files:**
- Modify: `internal/http/handlers/api/v1/admin/service_handler.go` (методы + settings-поле)
- Modify: `internal/http/handlers/api/v1/admin/periods_archive_test.go` (обновить `newAdminHandlerWithPeriod` под новую сигнатуру `NewServiceHandler`)
- Create: `internal/http/handlers/api/v1/admin/period_overview_test.go`
- Modify: `internal/http/server.go` (routes + передать settings в `NewServiceHandler`)
- Modify: `specs/040-api-contract.md`
- Modify: `specs/050-permissions-and-lifecycle.md`

**Interfaces:**
- Consumes: `s.PeriodOverview`, `s.PeriodStats`, `s.BulkSetTeamPeriodStatus` (Tasks 3–4); `service.LoadHealthCheckInConfig` + settings provider; `auth.UserIDFromContext`, `auth.TenantScopeFromContext`; `v1.WriteJSON`, `v1.WriteError`; `chi.URLParam`.
- Produces (JSON, service-типы сериализуются напрямую, как `HealthCheckInResult`):
  - `GET /api/v1/admin/periods/stats` → `{"items":[PeriodStatsItem...]}`
  - `GET /api/v1/admin/periods/{periodID}/overview` → `PeriodOverview`
  - `POST /api/v1/admin/periods/{periodID}/teams/activate` → `BulkStatusResult`
  - `POST /api/v1/admin/periods/{periodID}/teams/close` → `BulkStatusResult`

- [ ] **Step 1: Add settings provider to the admin ServiceHandler**

В `internal/http/handlers/api/v1/admin/service_handler.go` замени объявление struct/конструктора:

```go
type settingsReader interface {
	GetTenant(ctx context.Context, scope domain.TenantScope, key string) (json.RawMessage, error)
}

type ServiceHandler struct {
	service  *service.Service
	settings settingsReader
}

func NewServiceHandler(svc *service.Service, settings settingsReader) *ServiceHandler {
	return &ServiceHandler{service: svc, settings: settings}
}
```

(`encoding/json` уже импортирован в файле.)

- [ ] **Step 2: Update the existing archive test to the new constructor**

В `internal/http/handlers/api/v1/admin/periods_archive_test.go`, функция `newAdminHandlerWithPeriod`: `service.New(...)` уже строит `svc`. Замени:

```go
func newAdminHandlerWithPeriod(_ *testing.T, p domain.Period) *ServiceHandler {
	svc := service.New(service.Deps{Periods: &fakePeriodRepo{period: p}})
	return NewServiceHandler(svc, nil)
}
```

(`settings` не нужен для archive-тестов — `nil` допустим, эти хендлеры его не читают.)

- [ ] **Step 3: Run to verify build breaks at the server wiring**

Run: `go build ./...`
Expected: FAIL — `server.go` вызывает `NewServiceHandler(s.service)` со старой сигнатурой. Это ожидаемо; чиним в Step 4.

- [ ] **Step 4: Fix server wiring + register routes**

В `internal/http/server.go`:

Строку `serviceH := apiadmin.NewServiceHandler(s.service)` (около 437) замени на:

```go
serviceH := apiadmin.NewServiceHandler(s.service, s.settingsSvc)
```

В блок регистрации admin-роутов периодов (около строк 485–490) добавь:

```go
		r.Get("/api/v1/admin/periods/stats", serviceH.HandlePeriodStats)
		r.Get("/api/v1/admin/periods/{periodID}/overview", serviceH.HandlePeriodOverview)
		r.Post("/api/v1/admin/periods/{periodID}/teams/activate", serviceH.HandleActivatePeriodTeams)
		r.Post("/api/v1/admin/periods/{periodID}/teams/close", serviceH.HandleClosePeriodTeams)
```

Важно: `/api/v1/admin/periods/stats` должен стоять **до** любых `{periodID}`-паттернов не требуется (chi различает статический сегмент `stats` и параметр на одном уровне), но зарегистрируй `stats` рядом с остальными для читаемости.

- [ ] **Step 5: Implement the four handlers**

В `internal/http/handlers/api/v1/admin/service_handler.go` добавь (после существующих period-хендлеров). Сверь имена `v1.WriteJSON`/`v1.WriteError` с уже используемыми в файле:

```go
// weightTolerance loads the tenant's health-checkin weight tolerance (defaults to 0).
func (h *ServiceHandler) weightTolerance(r *http.Request, scope domain.TenantScope) int {
	if h.settings == nil {
		return 0
	}
	cfg, err := service.LoadHealthCheckInConfig(r.Context(), scope, h.settings)
	if err != nil {
		return 0
	}
	return cfg.WeightTolerance
}

// GET /api/v1/admin/periods/stats
func (h *ServiceHandler) HandlePeriodStats(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "no active tenant", nil)
		return
	}
	items, err := h.service.PeriodStats(r.Context(), scope, h.weightTolerance(r, scope))
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to load period stats", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

// GET /api/v1/admin/periods/{periodID}/overview
func (h *ServiceHandler) HandlePeriodOverview(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "no active tenant", nil)
		return
	}
	periodID, err := strconv.ParseInt(chi.URLParam(r, "periodID"), 10, 64)
	if err != nil || periodID <= 0 {
		v1.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid period id", nil)
		return
	}
	ov, err := h.service.PeriodOverview(r.Context(), scope, periodID, h.weightTolerance(r, scope))
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to load overview", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, ov)
}

func (h *ServiceHandler) handleBulk(w http.ResponseWriter, r *http.Request, target domain.TeamPeriodStatus) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "no active tenant", nil)
		return
	}
	periodID, err := strconv.ParseInt(chi.URLParam(r, "periodID"), 10, 64)
	if err != nil || periodID <= 0 {
		v1.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid period id", nil)
		return
	}
	res, err := h.service.BulkSetTeamPeriodStatus(r.Context(), scope, periodID, target, auth.UserIDFromContext(r.Context()))
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to apply bulk operation", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, res)
}

// POST /api/v1/admin/periods/{periodID}/teams/activate
func (h *ServiceHandler) HandleActivatePeriodTeams(w http.ResponseWriter, r *http.Request) {
	h.handleBulk(w, r, domain.TeamPeriodStatusInProgress)
}

// POST /api/v1/admin/periods/{periodID}/teams/close
func (h *ServiceHandler) HandleClosePeriodTeams(w http.ResponseWriter, r *http.Request) {
	h.handleBulk(w, r, domain.TeamPeriodStatusClosed)
}
```

Добавь в import-блок файла `"strconv"` (если ещё не импортирован). `v1`, `auth`, `domain`, `service`, `chi` уже импортированы.

- [ ] **Step 6: Verify `v1.WriteError` signature**

Run: `rg -n "func WriteError|func WriteJSON" internal/http/handlers/api/v1/`
Expected: подтверди сигнатуры. Если `WriteError` принимает другой набор аргументов — приведи вызовы выше к фактической сигнатуре (пример из существующего `HandleListPeriods`: `v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to load periods", nil)`).

- [ ] **Step 7: Write handler test for the 403-no-scope path**

Create `internal/http/handlers/api/v1/admin/period_overview_test.go`:

```go
package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"okrs/internal/service"
)

// Without a tenant scope in context, admin period endpoints must 403.
func TestHandlePeriodOverview_ForbiddenWithoutScope(t *testing.T) {
	h := NewServiceHandler(service.New(service.Deps{}), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/periods/1/overview", nil)
	req = withURLParam(req, "periodID", "1")
	w := httptest.NewRecorder()
	h.HandlePeriodOverview(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d (%s)", w.Code, w.Body.String())
	}
}

func TestHandlePeriodStats_ForbiddenWithoutScope(t *testing.T) {
	h := NewServiceHandler(service.New(service.Deps{}), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/periods/stats", nil)
	w := httptest.NewRecorder()
	h.HandlePeriodStats(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d (%s)", w.Code, w.Body.String())
	}
}
```

(`withURLParam` уже определён в `periods_archive_test.go`, тот же пакет.)

- [ ] **Step 8: Run handler + build**

Run: `go build ./... && go test ./internal/http/handlers/api/v1/admin/ -v`
Expected: PASS (новые + существующие archive-тесты зелёные).

- [ ] **Step 9: Update specs 040 and 050**

В `specs/040-api-contract.md` добавь раздел с новыми admin-эндпоинтами (RU). Найди секцию про `admin/periods` и допиши:

```
### Метрики и обзор периодов (admin)

- `GET /api/v1/admin/periods/stats` → `{ "items": [ { "period_id", "total_teams",
  "teams_with_goals", "avg_progress", "weight_error_count" } ] }`. Лёгкие метрики
  строк по всем периодам. Список `GET /api/v1/admin/periods` остаётся без метрик.
- `GET /api/v1/admin/periods/{periodID}/overview` → `{ "period_id", "summary": {
  "by_status": {in_progress,ready,forming,closed,no_goals}, "total_teams",
  "teams_with_goals", "weight_error_count", "avg_progress" }, "teams": [ { "id",
  "name", "path", "status", "goals_count", "progress", "weight_sum",
  "weight_error" } ] }`. Полный обзор для модалки управления периодом.
- `POST /api/v1/admin/periods/{periodID}/teams/activate` → `{ "affected", "skipped" }`.
  Переводит все команды с ≥1 целью и статусом ≠ in_progress в in_progress.
- `POST /api/v1/admin/periods/{periodID}/teams/close` → `{ "affected", "skipped" }`.
  Переводит все команды с ≥1 целью и статусом ≠ closed в closed.
```

В `specs/050-permissions-and-lifecycle.md` добавь абзац про массовые переходы:

```
### Массовые переходы team period status (admin)

Массовые операции доступны только в admin-плоскости. Затрагивают лишь команды,
у которых есть ≥1 цель в периоде (команды без целей пропускаются). Операция
идемпотентна: команда, уже находящаяся в целевом статусе, не затрагивается и не
пишется в op-лог. Каждая реально изменённая команда порождает отдельную запись
op-лога (`status/status_changed`, payload before/after, флаг bulk). `activate` →
in_progress (цели блокируются от редактирования, остаётся обновление прогресса);
`close` → closed (только комментарии).
```

- [ ] **Step 10: Full build + vet**

Run: `go build ./... && go vet ./... && go test ./internal/http/handlers/api/v1/admin/ ./internal/service/ ./internal/store/statuses/ ./internal/store/activity/`
Expected: green (store tests may SKIP without docker).

- [ ] **Step 11: Commit** (пользователь вручную)

```bash
git add internal/http/ specs/040-api-contract.md specs/050-permissions-and-lifecycle.md
git commit -m "http: period overview, stats and bulk status endpoints; update specs"
```

---

### Task 6: Frontend — общий `IconBtn` и иконочные контролы строки

**Files:**
- Modify: `internal/web/static/admin.js` (`PeriodAction` → `IconBtn`; действия в `PeriodsSection`/`PeriodRow`)

**Interfaces:**
- Consumes: существующие `T` (тема), `useState`, `PeriodRow`, `toggleArchive`, `openNew`, `openEdit`, `remove`.
- Produces: компонент `IconBtn({onClick, title, danger, children})`; новый набор иконок в строке; проп-хендлер открытия модалки управления (`onManage`) — сама модалка в Task 8, здесь только кнопка-шестерёнка, временно вызывающая `openEdit` НЕ нужно — прокинь заглушку `onManage` через `PeriodsSection` state (см. ниже).

Примечание по altitude: клик по строке остаётся `openEdit` (как сейчас). Шестерёнка вызовет обзор — в этой задаче заведём state `overview` в `PeriodsSection` и хендлер `openOverview`, который пока просто ставит `setOverview(p)`; рендер модалки добавим в Task 8.

- [ ] **Step 1: Replace `PeriodAction` with `IconBtn`**

В `internal/web/static/admin.js` замени функцию `PeriodAction` (строки ~343–350) на общий иконочный компонент:

```jsx
// Иконочная кнопка-действие в строке (SVG + подсказка при наведении).
function IconBtn({onClick, title, danger, children}) {
  const [hover, setHover] = useState(false);
  const color = danger ? T.danger : T.mutedFg;
  return <button onClick={e=>{e.stopPropagation();onClick();}} title={title} aria-label={title}
    onMouseEnter={()=>setHover(true)} onMouseLeave={()=>setHover(false)}
    style={{display:'inline-flex',alignItems:'center',justifyContent:'center',width:30,height:30,
      background:hover?(danger?'#fdecec':'#f1f0fb'):'transparent',border:'none',borderRadius:8,
      cursor:'pointer',color,opacity:hover?1:0.75,transition:'background .12s,opacity .12s',padding:0}}>
    {children}
  </button>;
}

// Набор SVG-иконок 16×16 (наследуют currentColor).
const Icons = {
  gear: <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>,
  nested: <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M9 4v10a2 2 0 0 0 2 2h9"/><path d="m16 12 4 4-4 4"/></svg>,
  pencil: <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M12 20h9"/><path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4Z"/></svg>,
  trash: <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M3 6h18"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>,
  archiveIn: <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><rect x="3" y="4" width="18" height="4" rx="1"/><path d="M5 8v11a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V8"/><path d="M10 12h4"/></svg>,
  archiveOut: <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><rect x="3" y="4" width="18" height="4" rx="1"/><path d="M5 8v11a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V8"/><path d="M12 18v-6"/><path d="m9 14 3-3 3 3"/></svg>,
};
```

- [ ] **Step 2: Add overview state + wire the actions in `PeriodsSection`**

В `PeriodsSection` (около строки 400) добавь состояние обзора рядом с `modal`:

```jsx
  const [overview, setOverview] = useState(null); // period selected for the management modal
```

Замени блок построения `actions` в `.map` (строки ~462–470) на иконочный, с правильным порядком (архив — слева группы, корзина — крайняя правая):

```jsx
        : periods.map((p, i) => {
            const actions = [];
            if (p.status === 'closed')
              actions.push(<IconBtn key="arch" title="В архив — скрыть из активных списков" onClick={()=>toggleArchive(p)}>{Icons.archiveIn}</IconBtn>);
            if (p.status === 'archived')
              actions.push(<IconBtn key="unarch" title="Вернуть из архива" onClick={()=>toggleArchive(p)}>{Icons.archiveOut}</IconBtn>);
            actions.push(<IconBtn key="manage" title="Управление периодом" onClick={()=>setOverview(p)}>{Icons.gear}</IconBtn>);
            actions.push(<IconBtn key="add" title="Создать вложенный период" onClick={()=>openNew(p)}>{Icons.nested}</IconBtn>);
            actions.push(<IconBtn key="edit" title="Редактировать период" onClick={()=>openEdit(p)}>{Icons.pencil}</IconBtn>);
            actions.push(<IconBtn key="del" danger title="Удалить период" onClick={()=>remove(p)}>{Icons.trash}</IconBtn>);
            return <PeriodRow key={p.id} p={p} cols={cols} first={i===0} onOpen={()=>openEdit(p)} actions={actions}/>;
          })}
```

Requirement note: корзина всегда крайняя правая, шестерёнка/вложенный/карандаш по центру группы, архив добавляется слева. Группа выровнена по правому краю (`justifyContent:'flex-end'` уже есть в `PeriodRow`). Уменьши `gap` группы: в `PeriodRow` (строка ~396) поменяй `gap:14` на `gap:4` для компактного иконочного ряда.

- [ ] **Step 3: Reduce the fixed action column width**

В `PeriodsSection` строка `const cols = 'minmax(0,1fr) 210px 150px 280px';` — иконки уже, поэтому уменьши последнюю колонку до `210px`: `const cols = 'minmax(0,1fr) 210px 150px 210px';`. (Если после метрик в Task 7 колонка ДАТЫ/СТАТУС разъедется — вернёшься сюда.)

- [ ] **Step 4: Verify in the running app**

Run the app (используй skill `run` или существующий способ запуска), открой `/admin/periods`. Проверь визуально:
- В каждой строке иконки: шестерёнка, вложенный, карандаш, корзина; у закрытых/архивных слева добавлена иконка архива.
- Наведение показывает подсказки (`title`) и подсветку фона.
- Корзина крайняя правая во всех строках; иконки выровнены по правому краю.
- Клик по строке открывает редактирование (как раньше). Клик по шестерёнке пока ничего видимого не делает (модалка в Task 8) — ок.

Expected: строки визуально соответствуют скриншоту (иконки вместо текста), выравнивание консистентно.

- [ ] **Step 5: Commit** (пользователь вручную)

```bash
git add internal/web/static/admin.js
git commit -m "admin ui: icon controls for period rows with tooltips"
```

---

### Task 7: Frontend — асинхронный догруз метрик строк

**Files:**
- Modify: `internal/web/static/admin.js` (`PeriodsSection` fetch stats; `PeriodRow` metrics block)

**Interfaces:**
- Consumes: `apiGet` (`admin.js:11`), эндпоинт `GET /api/v1/admin/periods/stats` (Task 5), `p.id`.
- Produces: state `stats` (map periodID → item) в `PeriodsSection`, прокинутый в `PeriodRow` как `stat`.

- [ ] **Step 1: Fetch stats after render in `PeriodsSection`**

В `PeriodsSection` добавь загрузку метрик (после объявления state, до `return`):

```jsx
  const [stats, setStats] = useState({}); // periodID -> {total_teams, teams_with_goals, avg_progress, weight_error_count}
  useEffect(() => {
    let alive = true;
    apiGet('/api/v1/admin/periods/stats').then(res => {
      if (!alive || !res || !res.items) return;
      const m = {};
      for (const it of res.items) m[it.period_id] = it;
      setStats(m);
    }).catch(()=>{});
    return () => { alive = false; };
  }, [periods]);
```

(`useEffect` уже используется в файле — импорт/скоуп ok. `apiGet` возвращает распарсенный JSON — сверься с `admin.js:11` `const apiGet = url => apiFetch(url)` и с тем, что `apiFetch` резолвит в объект.)

- [ ] **Step 2: Pass stat into each row**

В `.map(...)` добавь `stat={stats[p.id]}` в `<PeriodRow .../>`:

```jsx
            return <PeriodRow key={p.id} p={p} stat={stats[p.id]} cols={cols} first={i===0} onOpen={()=>openEdit(p)} actions={actions}/>;
```

- [ ] **Step 3: Render metrics in `PeriodRow`**

В `PeriodRow` (сигнатуру расширь: `function PeriodRow({p, stat, cols, first, onOpen, actions})`) добавь блок метрик в ячейку с названием/датами. Замени ячейку дат (строка ~394) так, чтобы под датами/рядом шли метрики:

```jsx
    <div style={{minWidth:0}}>
      <div style={{fontSize:12.5,color:T.mutedFg,fontFamily:'ui-monospace,Menlo,monospace'}}>{fmtDateShort(p.start_date)} – {fmtDateShort(p.end_date)}</div>
      {stat ? <PeriodMetrics stat={stat}/> : <div style={{height:16}}/>}
    </div>
```

Добавь компонент `PeriodMetrics` рядом с `PeriodRow`:

```jsx
function PeriodMetrics({stat}) {
  const total = stat.total_teams || 0;
  const withGoals = stat.teams_with_goals || 0;
  const pct = Math.max(0, Math.min(100, stat.avg_progress || 0));
  const err = stat.weight_error_count || 0;
  return <div style={{display:'flex',alignItems:'center',gap:10,marginTop:4,flexWrap:'wrap'}}>
    <span style={{fontSize:11.5,color:T.dimFg}}>{withGoals}/{total} с целями</span>
    <span style={{display:'inline-block',width:56,height:5,borderRadius:999,background:'#eceaf6',position:'relative'}}>
      <span style={{position:'absolute',left:0,top:0,bottom:0,width:pct+'%',borderRadius:999,background:T.accent}}/>
    </span>
    <span style={{fontSize:12,fontWeight:700,color:T.headingFg}}>{pct}%</span>
    {err > 0 && <span style={{fontSize:11,fontWeight:600,color:'#b91c1c',background:'#fdecec',borderRadius:999,padding:'1px 8px'}}>веса {err}</span>}
  </div>;
}
```

Note: колонка ДАТЫ теперь несёт и даты, и метрики — колонку СТАТУС/действия это не сдвигает (grid-колонки фиксированы). Если визуально тесно — увеличь ширину колонки дат в `cols` с `210px` до `260px`.

- [ ] **Step 4: Verify in the running app**

Open `/admin/periods`. Ожидается: список появляется сразу; спустя мгновение под датами появляются `X/Y с целями`, прогресс-бар, `%`, и красный бейдж `веса N` там, где есть ошибки. Числа совпадают с модальным обзором (Task 8) и со скриншотом-ориентиром.

- [ ] **Step 5: Commit** (пользователь вручную)

```bash
git add internal/web/static/admin.js
git commit -m "admin ui: async per-period row metrics"
```

---

### Task 8: Frontend — модалка «Управление периодом»

**Files:**
- Modify: `internal/web/static/admin.js` (компонент `PeriodOverviewModal` + рендер из `PeriodsSection`)

**Interfaces:**
- Consumes: `Modal`, `Btn`, `PeriodBadge`, `apiGet`, `apiPost`, `fmtDateShort`, эндпоинты `GET …/overview`, `POST …/teams/activate`, `POST …/teams/close` (Task 5); state `overview`/`setOverview` и `reload` (Task 6).
- Produces: компонент `PeriodOverviewModal({period, onClose, onEdit, onDelete, reload})`.

- [ ] **Step 1: Add the overview modal component**

В `admin.js` добавь компонент (перед `PeriodsSection` или после `PeriodModalBody`):

```jsx
const STATUS_TILES = [
  {key:'in_progress', label:'В работе',    dot:'#22c55e', color:'#166534'},
  {key:'ready',       label:'К валидации', dot:'#f59e0b', color:'#92400e'},
  {key:'forming',     label:'Черновик',    dot:'#9ca3af', color:'#4b5563'},
  {key:'closed',      label:'Закрыто',     dot:'#9ca3af', color:'#4b5563'},
  {key:'no_goals',    label:'Нет целей',   dot:'#cbd5e1', color:'#64748b'},
];

function PeriodOverviewModal({period, onClose, onEdit, onDelete, reload}) {
  const [data, setData] = useState(null);
  const [busy, setBusy] = useState(false);
  const [drill, setDrill] = useState(null); // {title, teams:[...]}

  const load = () => apiGet(`/api/v1/admin/periods/${period.id}/overview`).then(setData).catch(()=>{});
  useEffect(() => { load(); }, [period.id]);

  if (!data) return <div style={{padding:'40px 22px',textAlign:'center',color:T.mutedFg}}>Загрузка обзора…</div>;
  const s = data.summary;

  const teamsByStatus = k => (data.teams||[]).filter(t=>t.status===k || (k==='in_progress' && t.status==='validated'));
  const teamsWithErr  = () => (data.teams||[]).filter(t=>t.weight_error);
  const teamsWithGoals= () => (data.teams||[]).filter(t=>t.goals_count>0);

  const affectActivate = teamsWithGoals().filter(t=>t.status!=='in_progress' && t.status!=='validated').length;
  const affectClose    = teamsWithGoals().filter(t=>t.status!=='closed').length;
  const skipNoGoals    = s.total_teams - s.teams_with_goals;

  async function apply(ep) {
    if (busy) return;
    setBusy(true);
    try {
      const res = await apiPost(`/api/v1/admin/periods/${period.id}/teams/${ep}`, {});
      if (!res || !res.ok && res.affected===undefined) { alert('Ошибка операции'); return; }
      await load();
      reload();
    } finally { setBusy(false); }
  }

  const tile = (label, value, sub, accent, onClick) =>
    <div onClick={onClick} style={{flex:'1 1 150px',minWidth:140,background:'white',border:'1px solid '+T.cardBorder,borderRadius:12,padding:'14px 16px',cursor:onClick?'pointer':'default'}}>
      <div style={{fontSize:12,color:T.mutedFg,fontWeight:600}}>{label}</div>
      <div style={{fontSize:26,fontWeight:800,color:accent||T.headingFg,marginTop:4}}>{value}</div>
      {sub && <div style={{fontSize:11,color:T.dimFg,marginTop:2}}>{sub}</div>}
    </div>;

  return <div>
    <div style={{padding:'18px 22px',borderBottom:'1px solid '+T.hairline,display:'flex',alignItems:'flex-start',gap:16}}>
      <div style={{flex:1}}>
        <div style={{fontSize:20,fontWeight:800,color:T.headingFg}}>{period.name}</div>
        <div style={{fontSize:12.5,color:T.mutedFg,marginTop:4,display:'flex',alignItems:'center',gap:10}}>
          <span style={{fontFamily:'ui-monospace,Menlo,monospace'}}>{fmtDateShort(period.start_date)} — {fmtDateShort(period.end_date)}</span>
          <PeriodBadge status={period.status}/>
        </div>
      </div>
      <Btn onClick={onEdit}>Редактировать</Btn>
      <Btn danger onClick={onDelete}>Удалить</Btn>
    </div>

    <div style={{padding:'18px 22px'}}>
      <div style={{fontSize:11,color:T.dimFg,fontWeight:700,textTransform:'uppercase',letterSpacing:.5,marginBottom:10}}>Команды по статусам · всего {s.total_teams}</div>
      <div style={{display:'flex',gap:10,flexWrap:'wrap'}}>
        {STATUS_TILES.map(st => tile(
          <span style={{display:'inline-flex',alignItems:'center',gap:6}}><span style={{width:7,height:7,borderRadius:999,background:st.dot}}/>{st.label}</span>,
          s.by_status[st.key]||0, 'показать состав', st.color,
          () => setDrill({title:st.label, teams:teamsByStatus(st.key)})
        ))}
      </div>

      <div style={{fontSize:11,color:T.dimFg,fontWeight:700,textTransform:'uppercase',letterSpacing:.5,margin:'18px 0 10px'}}>Качество и результат</div>
      <div style={{display:'flex',gap:10,flexWrap:'wrap'}}>
        {tile('Команды с целями', `${s.teams_with_goals}/${s.total_teams}`, 'участвуют в массовых операциях', T.accent, ()=>setDrill({title:'Команды с целями', teams:teamsWithGoals()}))}
        {tile('Ошибки весов', s.weight_error_count, 'сумма весов целей ≠ 100%', '#b91c1c', ()=>setDrill({title:'Ошибки весов', teams:teamsWithErr()}))}
        {tile('Средний прогресс', `${s.avg_progress}%`, `по ${s.teams_with_goals} командам с целями`, T.accent)}
      </div>

      {drill && <div style={{marginTop:14,border:'1px solid '+T.cardBorder,borderRadius:12,overflow:'hidden'}}>
        <div style={{padding:'10px 14px',background:'#f8fafc',display:'flex',justifyContent:'space-between',alignItems:'center'}}>
          <span style={{fontSize:12.5,fontWeight:700,color:T.headingFg}}>{drill.title} · {drill.teams.length}</span>
          <button onClick={()=>setDrill(null)} style={{background:'none',border:'none',cursor:'pointer',color:T.mutedFg,fontSize:16}}>×</button>
        </div>
        <div style={{maxHeight:220,overflowY:'auto'}}>
          {drill.teams.length===0
            ? <div style={{padding:'16px',textAlign:'center',color:T.dimFg,fontSize:12.5}}>Пусто</div>
            : drill.teams.map(t => <div key={t.id} style={{display:'flex',justifyContent:'space-between',gap:10,padding:'8px 14px',borderTop:'1px solid '+T.hairline,fontSize:12.5}}>
                <span style={{color:T.headingFg,overflow:'hidden',textOverflow:'ellipsis',whiteSpace:'nowrap'}}>{(t.path||[]).join(' › ')||t.name}</span>
                <span style={{color:t.weight_error?'#b91c1c':T.mutedFg,flexShrink:0}}>{t.goals_count>0?`${t.progress}% · веса ${t.weight_sum}`:'нет целей'}</span>
              </div>)}
        </div>
      </div>}

      <div style={{fontSize:11,color:T.dimFg,fontWeight:700,textTransform:'uppercase',letterSpacing:.5,margin:'18px 0 10px'}}>Массовые операции</div>
      <div style={{display:'flex',flexDirection:'column',gap:10}}>
        <div style={{border:'1px solid '+T.cardBorder,borderRadius:12,padding:'14px 16px',display:'flex',alignItems:'center',gap:14}}>
          <div style={{flex:1}}>
            <div style={{fontSize:13.5,fontWeight:700,color:T.headingFg}}>Перевести все команды в «В работе»</div>
            <div style={{fontSize:12,color:T.mutedFg,marginTop:3}}>Только команды с ≥1 целью. Цели блокируются от редактирования, остаётся обновление прогресса.</div>
          </div>
          <div style={{fontSize:11.5,color:T.dimFg,textAlign:'right'}}>затронет {affectActivate}<br/>пропустим {skipNoGoals} без целей</div>
          <Btn variant="primary" disabled={busy||affectActivate===0} onClick={()=>apply('activate')}>Применить</Btn>
        </div>
        <div style={{border:'1px solid '+T.cardBorder,borderRadius:12,padding:'14px 16px',display:'flex',alignItems:'center',gap:14}}>
          <div style={{flex:1}}>
            <div style={{fontSize:13.5,fontWeight:700,color:T.headingFg}}>Закрыть цели всех команд периода</div>
            <div style={{fontSize:12,color:T.mutedFg,marginTop:3}}>Команды без целей не трогаем. У остальных статус становится «Закрыто» — доступны только комментарии.</div>
          </div>
          <div style={{fontSize:11.5,color:T.dimFg,textAlign:'right'}}>затронет {affectClose}<br/>пропустим {skipNoGoals} без целей</div>
          <Btn variant="primary" disabled={busy||affectClose===0} onClick={()=>apply('close')}>Применить</Btn>
        </div>
      </div>
    </div>
  </div>;
}
```

Note: `apiPost` возвращает объект с `ok` (см. использование в `save`/`toggleArchive`). Тело bulk-ответа — `{affected, skipped}`; при `res.ok` перезагружаем. Скорректируй проверку под фактический возврат `apiPost` (в существующем коде `res.ok` — это флаг успешного HTTP; сверься с `admin.js` helper).

- [ ] **Step 2: Render the modal from `PeriodsSection`**

В `PeriodsSection` рядом с существующим `<Modal ...>` (редактирование) добавь модалку управления, управляемую state `overview` (заведён в Task 6):

```jsx
    <Modal open={!!overview} title={overview ? `Управление периодом · ${overview.name}` : ''}
      subtitle="Статусы команд, ошибки весов и массовые операции"
      onClose={()=>setOverview(null)} width={820}>
      {overview && <PeriodOverviewModal
        period={overview}
        onClose={()=>setOverview(null)}
        onEdit={()=>{ const p=overview; setOverview(null); openEdit(p); }}
        onDelete={()=>{ const p=overview; setOverview(null); remove(p); }}
        reload={reload}/>}
    </Modal>
```

- [ ] **Step 3: Verify in the running app**

Open `/admin/periods`, кликни шестерёнку у периода с командами. Проверь:
- Открывается модалка «Управление периодом · <имя>» с шапкой (даты, статус, «Редактировать», «Удалить»).
- 5 плиток статусов (сумма = всего), 3 плитки (команды с целями / ошибки весов / средний прогресс). Клик по плитке раскрывает состав команд.
- Карточки массовых операций показывают `затронет N` / `пропустим M без целей`; «Применить» на «В работе» → счётчики обновляются, статусы команд меняются; «Применить» на «Закрыть» → команды с целями закрываются, без целей пропущены.
- Числа совпадают с метриками строки (Task 7).
- Кнопки «Редактировать»/«Удалить» открывают соответствующие потоки.

- [ ] **Step 4: Full backend regression + commit** (пользователь вручную)

Run: `go build ./... && go vet ./... && go test ./...`
Expected: green (store/integration tests may SKIP without docker).

```bash
git add internal/web/static/admin.js
git commit -m "admin ui: period management modal with overview and bulk operations"
```

---

## Self-Review

**Spec coverage:**
- Иконочные контролы + подсказки + выравнивание по правому краю + архив слева группы → Task 6. ✅
- Метрики строк (X/Y с целями, прогресс, веса N) отдельным запросом → Tasks 5 (`/stats`) + 7 (async merge). ✅
- Модалка обзора: счётчики по статусам, ошибки весов, средний прогресс, drill-down состава → Tasks 2/3 (агрегатор) + 5 (`/overview`) + 8 (UI). ✅
- Массовая «в работе» (только команды с целями) → Tasks 4/5 (`activate`) + 8. ✅
- Массовое «закрыть цели» (без целей — не трогаем; с целями — closed) → Tasks 4/5 (`close`) + 8. ✅
- Op-лог на каждую затронутую команду → Task 4 (`RecordBatch`, per-team events). ✅
- Идемпотентность (только реально меняющиеся команды) → Task 4 (`computeBulkAffected`). ✅
- Список периодов остаётся лёгким; статистика отдельным запросом → Tasks 5/7. ✅
- Клик по строке = редактирование; обзор = только шестерёнка → Task 6. ✅
- Специи 040/050 обновлены в change set → Task 5. ✅
- Эффективность (без запросов в цикле) → Task 1 batch-методы, Task 4 использует их. ✅

**Placeholder scan:** нет TBD/«implement later»/«handle edge cases» без кода. Каждый шаг несёт конкретный код или конкретную команду. ✅

**Type consistency:** `computePeriodOverview(data, weightTolerance)` — единая сигнатура (Tasks 2,3). `PeriodStatsItem`/`PeriodOverview` json-теги совпадают с фронтом (`total_teams`, `teams_with_goals`, `avg_progress`, `weight_error_count`, `by_status`, `weight_error`, `weight_sum`, `goals_count`). `SetTeamPeriodStatuses`/`RecordBatch` — одинаковые сигнатуры в store (Task 1) и интерфейсах service (Task 1) и вызовах (Task 4). `NewServiceHandler(svc, settings)` — согласовано между Task 5 Step 1/2/4. `BulkStatusResult{Affected,Skipped}` — json `affected`/`skipped`, фронт читает `res.affected`. ✅

**Риск-заметки для исполнителя:**
- Точные сигнатуры `v1.WriteError`/`v1.WriteJSON`, поведение `apiPost`/`apiGet` (возврат) и полный список методов интерфейсов `TeamRepo/GoalRepo/TeamStatusRepo/ActivityRepo` нужно сверить по факту (шаги это отмечают) — репозиторий мог измениться.
- Store-тесты (`SetupDB`) требуют docker и SKIP-аются без него; проверяй компиляцию `go build`/`go vet` как минимум.
