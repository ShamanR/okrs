# Health Check-in Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Добавить инструмент Health Check-in — постоянную кнопку в sidebar трекера с badge счётчиком проблем и боковой панелью, которая автоматически находит проблемы в OKR-дереве руководителя (нет обновлений, нет целей, ошибки весов, отставание).

**Architecture:** Новый `GET /api/v1/health-checkin?period_id=X` endpoint. Сервер вычисляет scope по `display_name` пользователя (лид + владелец цели). `HealthCheckInCache` хранит данные периода in-memory (TTL 5 мин, фоновый refresh), вычисление категорий — pure in-memory функции. Настройки хранятся в `system_settings` (JSONB), без новых таблиц/миграций.

**Tech Stack:** Go (backend), React 18 CDN + Babel standalone (frontend), PostgreSQL, pgx/v5. Тесты: stdlib `testing` + fake stores для unit, testcontainers-go для integration handler tests.

---

## File map

**Создать:**
- `internal/service/healthcheckin.go` — config, scope, category computation, service method
- `internal/service/healthcheckin_cache.go` — PeriodData struct, cache + background refresh
- `internal/service/healthcheckin_test.go` — unit tests для scope + categories
- `internal/http/handlers/api/v1/healthcheckin/handler.go` — GET /api/v1/health-checkin + admin settings handlers

**Изменить:**
- `internal/domain/models.go` — добавить `ProgressUpdatedAt *time.Time` в `KeyResult`
- `internal/store/goals/goals.go` — добавить `progress_updated_at` в SELECT KR в `ListGoalsByTeamsPeriod`
- `internal/service/service.go` — добавить `hcCache` в Deps/Service/NewFromStore
- `internal/http/server.go` — инициализация кеша, регистрация маршрутов
- `internal/web/static/tracker.css` — `.hci-*` классы
- `internal/web/static/tracker.js` — HealthCheckInButton, HealthCheckInPanel, data fetch
- `internal/web/static/admin.js` — раздел health-checkin + HealthCheckInSettingsPanel
- `specs/040-api-contract.md` — документировать новый endpoint
- `specs/020-domain-model.md` — документировать ключ `health_checkin_config`

---

## Task 1: Добавить ProgressUpdatedAt в domain.KeyResult и обновить store

**Files:**
- Modify: `internal/domain/models.go` (после `UpdatedAt time.Time` в KeyResult)
- Modify: `internal/store/goals/goals.go` (KR SELECT в ListGoalsByTeamsPeriod ~line 158, scan ~line 172)

- [ ] **Step 1: Открыть `internal/domain/models.go`, добавить поле в KeyResult**

В структуре `KeyResult` добавить поле после `UpdatedAt`:

```go
type KeyResult struct {
	ID                int64
	GoalID            int64
	Title             string
	Description       string
	Weight            int
	Kind              KRKind
	Progress          int
	SortOrder         int
	Project           *KRProject
	Percent           *KRPercent
	Linear            *KRLinear
	Boolean           *KRBoolean
	Note              *KeyResultNote
	CreatedAt         time.Time
	UpdatedAt         time.Time
	ProgressUpdatedAt *time.Time   // nullable: nil = never had a progress update
}
```

- [ ] **Step 2: Обновить SELECT в `ListGoalsByTeamsPeriod` (`internal/store/goals/goals.go`)**

Найти строку (~158):
```go
	krRows, err := r.db.Query(ctx, `
		SELECT id, goal_id, title, description, weight, kind, sort_order, created_at, updated_at
		FROM key_results
		WHERE goal_id = ANY($1)
		ORDER BY goal_id, sort_order, id`, goalIDs)
```
Заменить на:
```go
	krRows, err := r.db.Query(ctx, `
		SELECT id, goal_id, title, description, weight, kind, sort_order, created_at, updated_at, progress_updated_at
		FROM key_results
		WHERE goal_id = ANY($1)
		ORDER BY goal_id, sort_order, id`, goalIDs)
```

- [ ] **Step 3: Обновить Scan (~line 172)**

```go
		if err := krRows.Scan(&kr.ID, &kr.GoalID, &kr.Title, &kr.Description, &kr.Weight, &kr.Kind, &kr.SortOrder, &kr.CreatedAt, &kr.UpdatedAt, &kr.ProgressUpdatedAt); err != nil {
```

- [ ] **Step 4: Проверить компиляцию**

```bash
go build ./...
```
Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/models.go internal/store/goals/goals.go
git commit -m "feat: add ProgressUpdatedAt to domain.KeyResult"
```

---

## Task 2: HealthCheckInConfig + default

**Files:**
- Create: `internal/service/healthcheckin.go`

- [ ] **Step 1: Создать файл с config типами и default**

```go
package service

import (
	"context"
	"encoding/json"
	"time"

	"okrs/internal/domain"
	"okrs/internal/okr"
)

// HealthCheckInConfig controls thresholds and counter membership for each category.
// Loaded from system_settings key "health_checkin_config"; defaults apply when key is absent.
type HealthCheckInConfig struct {
	StaleDays       int            `json:"stale_days"`
	BehindMargin    int            `json:"behind_margin"`
	WeightTolerance int            `json:"weight_tolerance"`
	CacheTTLMinutes int            `json:"cache_ttl_minutes"`
	InCounter       map[string]bool `json:"in_counter"`
}

// defaultHealthCheckInConfig is used when no config is stored in system_settings.
var defaultHealthCheckInConfig = HealthCheckInConfig{
	StaleDays:       7,
	BehindMargin:    10,
	WeightTolerance: 0,
	CacheTTLMinutes: 5,
	InCounter: map[string]bool{
		"stale":               true,
		"no_goals":            true,
		"awaiting_validation": true,
		"formation_errors":    true,
		"lagging":             false,
	},
}

// SettingsReader loads system settings; *store.SettingsRepository satisfies this.
type SettingsReader interface {
	GetSetting(ctx context.Context, key string) (json.RawMessage, error)
}

// LoadHealthCheckInConfig reads config from system_settings, falling back to defaults.
func LoadHealthCheckInConfig(ctx context.Context, sr SettingsReader) (HealthCheckInConfig, error) {
	raw, err := sr.GetSetting(ctx, "health_checkin_config")
	if err != nil || raw == nil {
		return defaultHealthCheckInConfig, err
	}
	cfg := defaultHealthCheckInConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return defaultHealthCheckInConfig, nil
	}
	// ensure InCounter always has all keys (partial config in DB)
	for k, v := range defaultHealthCheckInConfig.InCounter {
		if _, ok := cfg.InCounter[k]; !ok {
			cfg.InCounter[k] = v
		}
	}
	return cfg, nil
}

// ── Result types ──────────────────────────────────────────────────────────────

type HealthCheckInItem struct {
	TeamID        int64    `json:"team_id"`
	TeamName      string   `json:"team_name"`
	TeamPath      []string `json:"team_path"`
	GoalID        int64    `json:"goal_id,omitempty"`
	GoalTitle     string   `json:"goal_title,omitempty"`
	EntityType    string   `json:"entity_type,omitempty"`   // "team" | "goal" | "kr"
	ErrorType     string   `json:"error_type,omitempty"`    // formation_errors only
	Status        string   `json:"status,omitempty"`        // awaiting_validation
	DaysSinceUpdate int    `json:"days_since_update,omitempty"` // stale
	Progress      int      `json:"progress,omitempty"`      // lagging
	ExpectedPace  int      `json:"expected_pace,omitempty"` // lagging
	ActualWeightSum int    `json:"actual_weight_sum,omitempty"` // formation_errors team-level
}

type HealthCheckInCategory struct {
	InCounter bool                `json:"in_counter"`
	Count     int                 `json:"count"`
	Items     []HealthCheckInItem `json:"items"`
}

type HealthCheckInResult struct {
	HasScope      bool                               `json:"has_scope"`
	PeriodID      int64                              `json:"period_id"`
	TotalProblems int                                `json:"total_problems"`
	Categories    map[string]*HealthCheckInCategory  `json:"categories"`
}

// ── Scope computation ─────────────────────────────────────────────────────────

// computeScope returns team IDs visible to the user:
//   - Lead-scope: teams where teams.lead == displayName + all descendants
//   - Owner-scope: teams that own goals where goal.OwnerText contains displayName (word match)
//
// Returns nil if no scope found (has_scope = false).
func computeScope(teams []domain.Team, goalsByTeam map[int64][]domain.Goal, displayName string) []int64 {
	if displayName == "" {
		return nil
	}

	teamsByID := make(map[int64]domain.Team, len(teams))
	childrenMap := make(map[int64][]int64, len(teams))
	for _, t := range teams {
		teamsByID[t.ID] = t
		if t.ParentID != nil {
			childrenMap[*t.ParentID] = append(childrenMap[*t.ParentID], t.ID)
		}
	}

	scopeSet := make(map[int64]struct{})

	// Lead-scope: find lead teams + all descendants.
	var addDescendants func(id int64)
	addDescendants = func(id int64) {
		if _, exists := scopeSet[id]; exists {
			return
		}
		scopeSet[id] = struct{}{}
		for _, childID := range childrenMap[id] {
			addDescendants(childID)
		}
	}
	for _, t := range teams {
		if t.DeletedAt == nil && t.Lead == displayName {
			addDescendants(t.ID)
		}
	}

	// Owner-scope: teams whose goals list displayName as owner (word match).
	for teamID, goals := range goalsByTeam {
		for _, g := range goals {
			if ownerTextContains(g.OwnerText, displayName) {
				scopeSet[teamID] = struct{}{}
				break
			}
		}
	}

	if len(scopeSet) == 0 {
		return nil
	}
	result := make([]int64, 0, len(scopeSet))
	for id := range scopeSet {
		result = append(result, id)
	}
	return result
}

// ownerTextContains reports whether name appears as a whole word in the comma/space-separated ownerText.
func ownerTextContains(ownerText, name string) bool {
	if ownerText == "" || name == "" {
		return false
	}
	// split by comma, then trim spaces
	for _, part := range splitOwners(ownerText) {
		if strings.EqualFold(part, name) {
			return true
		}
	}
	return false
}

func splitOwners(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			result = append(result, t)
		}
	}
	return result
}

// ── Category computation ──────────────────────────────────────────────────────

// computeCategories computes all 5 health check-in categories for the given scope.
func computeCategories(
	data *PeriodData,
	scopeIDs []int64,
	cfg HealthCheckInConfig,
	now time.Time,
) *HealthCheckInResult {
	scopeSet := make(map[int64]struct{}, len(scopeIDs))
	for _, id := range scopeIDs {
		scopeSet[id] = struct{}{}
	}

	teamsByID := make(map[int64]domain.Team, len(data.Teams))
	childrenMap := make(map[int64][]int64)
	for _, t := range data.Teams {
		teamsByID[t.ID] = t
		if t.ParentID != nil {
			childrenMap[*t.ParentID] = append(childrenMap[*t.ParentID], t.ID)
		}
	}

	cats := map[string]*HealthCheckInCategory{
		"stale":               {InCounter: cfg.InCounter["stale"]},
		"no_goals":            {InCounter: cfg.InCounter["no_goals"]},
		"awaiting_validation": {InCounter: cfg.InCounter["awaiting_validation"]},
		"formation_errors":    {InCounter: cfg.InCounter["formation_errors"]},
		"lagging":             {InCounter: cfg.InCounter["lagging"]},
	}

	expectedPace := calcExpectedPace(data.Period, now)

	for _, teamID := range scopeIDs {
		team, ok := teamsByID[teamID]
		if !ok {
			continue
		}
		path := buildTeamPath(teamID, teamsByID)
		goals := data.GoalsByTeam[teamID]
		status := data.Statuses[teamID]

		// no_goals
		if len(goals) == 0 {
			cats["no_goals"].Items = append(cats["no_goals"].Items, HealthCheckInItem{
				TeamID: teamID, TeamName: team.Name, TeamPath: path,
			})
		}

		// awaiting_validation
		if len(goals) > 0 && (status == domain.TeamPeriodStatusForming || status == domain.TeamPeriodStatusReady) {
			cats["awaiting_validation"].Items = append(cats["awaiting_validation"].Items, HealthCheckInItem{
				TeamID: teamID, TeamName: team.Name, TeamPath: path,
				Status: string(status),
			})
		}

		// formation_errors (team level): sum of goal weights ≠ 100
		if len(goals) > 0 {
			weightSum := 0
			for _, g := range goals {
				weightSum += g.Weight
			}
			if abs(weightSum-100) > cfg.WeightTolerance {
				cats["formation_errors"].Items = append(cats["formation_errors"].Items, HealthCheckInItem{
					TeamID: teamID, TeamName: team.Name, TeamPath: path,
					EntityType: "team", ErrorType: "weight_sum_not_100",
					ActualWeightSum: weightSum,
				})
			}
		}

		// per-goal checks
		for _, g := range goals {
			if g.TeamID != teamID {
				// shared goal — skip (counted under owner team)
				continue
			}
			goalProgress := CalculateGoalProgress(&g)
			lastProgress := goalLastProgressAt(g)
			daysSince := 0
			if lastProgress != nil {
				daysSince = int(now.Sub(*lastProgress).Hours() / 24)
			}
			isStale := len(g.KeyResults) > 0 && (lastProgress == nil || daysSince > cfg.StaleDays)

			// stale
			if isStale {
				cats["stale"].Items = append(cats["stale"].Items, HealthCheckInItem{
					TeamID: teamID, TeamName: team.Name, TeamPath: path,
					GoalID: g.ID, GoalTitle: g.Title,
					DaysSinceUpdate: daysSince,
				})
			}

			// lagging
			if !isStale && goalProgress < expectedPace-cfg.BehindMargin {
				cats["lagging"].Items = append(cats["lagging"].Items, HealthCheckInItem{
					TeamID: teamID, TeamName: team.Name, TeamPath: path,
					GoalID: g.ID, GoalTitle: g.Title,
					Progress: goalProgress, ExpectedPace: expectedPace,
				})
			}

			// formation_errors (goal level)
			goalErrors := checkGoalFormationErrors(g, cfg.WeightTolerance)
			for i := range goalErrors {
				goalErrors[i].TeamName = team.Name  // fill in team context
				goalErrors[i].TeamPath = path
			}
			cats["formation_errors"].Items = append(cats["formation_errors"].Items, goalErrors...)
		}
	}

	total := 0
	for _, cat := range cats {
		cat.Count = len(cat.Items)
		if cat.InCounter {
			total += cat.Count
		}
	}

	return &HealthCheckInResult{
		HasScope:      true,
		PeriodID:      data.PeriodID,
		TotalProblems: total,
		Categories:    cats,
	}
}

// goalLastProgressAt returns the latest progress_updated_at across all KRs of a goal.
// Returns nil if no KR was ever updated.
func goalLastProgressAt(g domain.Goal) *time.Time {
	var latest *time.Time
	for _, kr := range g.KeyResults {
		if kr.ProgressUpdatedAt == nil {
			continue
		}
		if latest == nil || kr.ProgressUpdatedAt.After(*latest) {
			latest = kr.ProgressUpdatedAt
		}
	}
	return latest
}

// checkGoalFormationErrors returns formation error items for a single goal.
func checkGoalFormationErrors(g domain.Goal, weightTolerance int) []HealthCheckInItem {
	path := []string{} // filled by caller; passed as empty here, populated by computeCategories
	item := func(errType, entityType string) HealthCheckInItem {
		return HealthCheckInItem{
			TeamID: g.TeamID, TeamName: "", TeamPath: path,
			GoalID: g.ID, GoalTitle: g.Title,
			EntityType: entityType, ErrorType: errType,
		}
	}

	var errs []HealthCheckInItem

	if len(g.KeyResults) == 0 {
		errs = append(errs, item("no_krs", "goal"))
		return errs
	}

	krWeightSum := 0
	for _, kr := range g.KeyResults {
		krWeightSum += kr.Weight

		if strings.TrimSpace(kr.Title) == "" {
			errs = append(errs, item("kr_no_title", "kr"))
		}
		switch kr.Kind {
		case domain.KRKindPercent:
			if kr.Percent != nil && kr.Percent.TargetValue == kr.Percent.StartValue {
				errs = append(errs, item("kr_zero_range", "kr"))
			}
		case domain.KRKindLinear:
			if kr.Linear != nil && kr.Linear.TargetValue == kr.Linear.StartValue {
				errs = append(errs, item("kr_zero_range", "kr"))
			}
		case domain.KRKindProject:
			if kr.Project == nil || len(kr.Project.Stages) == 0 {
				errs = append(errs, item("project_no_stages", "kr"))
			} else {
				stageWeightSum := 0
				for _, s := range kr.Project.Stages {
					stageWeightSum += s.Weight
				}
				if abs(stageWeightSum-100) > weightTolerance {
					errs = append(errs, item("project_stage_weight_sum_not_100", "kr"))
				}
			}
		}
	}

	if abs(krWeightSum-100) > weightTolerance {
		errs = append(errs, item("kr_weight_sum_not_100", "goal"))
	}

	return errs
}

// calcExpectedPace returns the expected progress (0-100) based on period elapsed fraction.
func calcExpectedPace(p domain.Period, now time.Time) int {
	total := p.EndDate.Sub(p.StartDate).Hours()
	if total <= 0 {
		return 100
	}
	elapsed := now.Sub(p.StartDate).Hours()
	frac := elapsed / total
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	return int(frac * 100)
}

func buildTeamPath(teamID int64, teamsByID map[int64]domain.Team) []string {
	var path []string
	visited := make(map[int64]struct{})
	cur, ok := teamsByID[teamID]
	for ok {
		if _, seen := visited[cur.ID]; seen {
			break
		}
		visited[cur.ID] = struct{}{}
		path = append([]string{cur.Name}, path...)
		if cur.ParentID == nil {
			break
		}
		cur, ok = teamsByID[*cur.ParentID]
	}
	return path
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
```

- [ ] **Step 2: Добавить import "strings" в файл**

В начало файла добавить:
```go
import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"okrs/internal/domain"
	"okrs/internal/okr"
)
```

- [ ] **Step 3: Проверить компиляцию**

```bash
go build ./internal/service/...
```

Expected: no errors (функция `CalculateGoalProgress` уже есть в `service/progress.go`).

- [ ] **Step 4: Commit**

```bash
git add internal/service/healthcheckin.go
git commit -m "feat: add HealthCheckInConfig and category/scope computation"
```

---

## Task 3: HealthCheckInCache с background refresh

**Files:**
- Create: `internal/service/healthcheckin_cache.go`

- [ ] **Step 1: Создать файл кеша**

```go
package service

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"okrs/internal/domain"
)

// PeriodData is the pre-loaded data for one period, held in cache.
type PeriodData struct {
	PeriodID    int64
	Period      domain.Period
	Teams       []domain.Team
	GoalsByTeam map[int64][]domain.Goal
	Statuses    map[int64]domain.TeamPeriodStatus
	CachedAt    time.Time
}

// periodLoader loads raw data for a period from the DB.
// Implemented as a closure in server.go that captures store repos.
type periodLoader func(ctx context.Context, periodID int64) (*PeriodData, error)

// HealthCheckInCache holds PeriodData per period_id with TTL-based expiry.
type HealthCheckInCache struct {
	mu      sync.RWMutex
	periods map[int64]*PeriodData
	ttl     time.Duration
	loader  periodLoader
	logger  *slog.Logger
}

// NewHealthCheckInCache creates a new cache with the given loader and TTL.
func NewHealthCheckInCache(loader periodLoader, ttl time.Duration, logger *slog.Logger) *HealthCheckInCache {
	return &HealthCheckInCache{
		periods: make(map[int64]*PeriodData),
		ttl:     ttl,
		loader:  loader,
		logger:  logger,
	}
}

// Get returns cached PeriodData for the given period, loading from DB if stale or absent.
func (c *HealthCheckInCache) Get(ctx context.Context, periodID int64) (*PeriodData, error) {
	c.mu.RLock()
	entry := c.periods[periodID]
	c.mu.RUnlock()

	if entry != nil && time.Since(entry.CachedAt) < c.ttl {
		return entry, nil
	}
	return c.reload(ctx, periodID)
}

func (c *HealthCheckInCache) reload(ctx context.Context, periodID int64) (*PeriodData, error) {
	data, err := c.loader(ctx, periodID)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.periods[periodID] = data
	c.mu.Unlock()
	return data, nil
}

// InvalidateAll clears all cached entries; next Get will reload from DB.
func (c *HealthCheckInCache) InvalidateAll() {
	c.mu.Lock()
	c.periods = make(map[int64]*PeriodData)
	c.mu.Unlock()
}

// StartRefreshLoop runs a background goroutine that proactively refreshes the active period.
// activePeriodFn returns 0 if no active period exists.
func (c *HealthCheckInCache) StartRefreshLoop(ctx context.Context, interval time.Duration, activePeriodFn func(ctx context.Context) int64) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				periodID := activePeriodFn(ctx)
				if periodID == 0 {
					continue
				}
				if _, err := c.reload(ctx, periodID); err != nil {
					if c.logger != nil {
						c.logger.Warn("health-checkin cache refresh failed", "err", err)
					}
				}
			}
		}
	}()
}
```

- [ ] **Step 2: Проверить компиляцию**

```bash
go build ./internal/service/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/service/healthcheckin_cache.go
git commit -m "feat: add HealthCheckInCache with TTL and background refresh"
```

---

## Task 4: Unit-тесты для scope и category computation

**Files:**
- Create: `internal/service/healthcheckin_test.go`

- [ ] **Step 1: Написать failing тесты для computeScope**

```go
package service

import (
	"testing"
	"time"

	"okrs/internal/domain"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func teamPtr(id int64) *int64 { return &id }
func timePtr(t time.Time) *time.Time { return &t }

func makeTeam(id int64, name, lead string, parentID *int64) domain.Team {
	return domain.Team{ID: id, Name: name, Lead: lead, ParentID: parentID}
}

func makeGoal(id, teamID int64, ownerText string, krs []domain.KeyResult) domain.Goal {
	return domain.Goal{ID: id, TeamID: teamID, OwnerText: ownerText, KeyResults: krs, Weight: 100}
}

// ── computeScope tests ────────────────────────────────────────────────────────

func TestComputeScope_LeadGetsSubtree(t *testing.T) {
	teams := []domain.Team{
		makeTeam(1, "Root", "Alice", nil),
		makeTeam(2, "Child", "", teamPtr(1)),
		makeTeam(3, "Grandchild", "", teamPtr(2)),
		makeTeam(4, "Other", "Bob", nil),
	}
	goals := map[int64][]domain.Goal{}
	ids := computeScope(teams, goals, "Alice")
	got := toSet(ids)
	if !got[1] || !got[2] || !got[3] {
		t.Errorf("expected IDs 1,2,3; got %v", ids)
	}
	if got[4] {
		t.Errorf("team 4 should not be in scope")
	}
}

func TestComputeScope_OwnerGetsOnlyOwnerTeam(t *testing.T) {
	teams := []domain.Team{
		makeTeam(10, "Team A", "", nil),
		makeTeam(11, "Team B", "", teamPtr(10)),
	}
	goals := map[int64][]domain.Goal{
		10: {makeGoal(1, 10, "Alice, Bob", nil)},
	}
	ids := computeScope(teams, goals, "Alice")
	got := toSet(ids)
	if !got[10] {
		t.Errorf("expected team 10 in owner scope")
	}
	if got[11] {
		t.Errorf("team 11 should NOT be in owner scope (no descendants)")
	}
}

func TestComputeScope_EmptyWhenNoMatch(t *testing.T) {
	teams := []domain.Team{makeTeam(1, "T", "Bob", nil)}
	goals := map[int64][]domain.Goal{}
	if computeScope(teams, goals, "Alice") != nil {
		t.Error("expected nil scope for non-lead non-owner")
	}
}

func TestOwnerTextContains(t *testing.T) {
	cases := []struct{text, name string; want bool}{
		{"Alice, Bob",    "Alice",       true},
		{"Alice, Bob",    "Bob",         true},
		{"Alice, Bob",    "ali",         false}, // no partial match
		{"",              "Alice",       false},
		{"Aleksander",    "Alex",        false},
	}
	for _, tc := range cases {
		if got := ownerTextContains(tc.text, tc.name); got != tc.want {
			t.Errorf("ownerTextContains(%q,%q)=%v want %v", tc.text, tc.name, got, tc.want)
		}
	}
}

// ── computeCategories tests ───────────────────────────────────────────────────

func makePeriodData(teams []domain.Team, goals map[int64][]domain.Goal, statuses map[int64]domain.TeamPeriodStatus) *PeriodData {
	now := time.Now()
	return &PeriodData{
		PeriodID: 1,
		Period: domain.Period{
			ID:        1,
			StartDate: now.AddDate(0, -1, 0),
			EndDate:   now.AddDate(0, 1, 0),
		},
		Teams:       teams,
		GoalsByTeam: goals,
		Statuses:    statuses,
	}
}

func makeCfg() HealthCheckInConfig {
	return HealthCheckInConfig{
		StaleDays: 7, BehindMargin: 10, WeightTolerance: 0,
		InCounter: map[string]bool{
			"stale": true, "no_goals": true,
			"awaiting_validation": true, "formation_errors": true, "lagging": false,
		},
	}
}

func TestCategories_NoGoals(t *testing.T) {
	teams := []domain.Team{makeTeam(1, "T1", "Alice", nil)}
	goals := map[int64][]domain.Goal{}
	statuses := map[int64]domain.TeamPeriodStatus{}
	data := makePeriodData(teams, goals, statuses)
	result := computeCategories(data, []int64{1}, makeCfg(), time.Now())
	if result.Categories["no_goals"].Count != 1 {
		t.Errorf("expected 1 no_goals, got %d", result.Categories["no_goals"].Count)
	}
	if result.TotalProblems != 1 {
		t.Errorf("expected total 1, got %d", result.TotalProblems)
	}
}

func TestCategories_StaleGoal(t *testing.T) {
	old := time.Now().AddDate(0, 0, -10)
	kr := domain.KeyResult{ID: 1, GoalID: 1, Title: "KR1", Weight: 100, Kind: domain.KRKindBoolean,
		Boolean: &domain.KRBoolean{}, ProgressUpdatedAt: timePtr(old)}
	g := makeGoal(1, 1, "", []domain.KeyResult{kr})
	g.Weight = 100

	teams := []domain.Team{makeTeam(1, "T1", "", nil)}
	goals := map[int64][]domain.Goal{1: {g}}
	statuses := map[int64]domain.TeamPeriodStatus{1: domain.TeamPeriodStatusInProgress}
	data := makePeriodData(teams, goals, statuses)
	result := computeCategories(data, []int64{1}, makeCfg(), time.Now())

	if result.Categories["stale"].Count != 1 {
		t.Errorf("expected 1 stale, got %d", result.Categories["stale"].Count)
	}
}

func TestCategories_AwaitingValidation(t *testing.T) {
	kr := domain.KeyResult{ID: 1, GoalID: 1, Title: "KR", Weight: 100, Kind: domain.KRKindBoolean,
		Boolean: &domain.KRBoolean{}, ProgressUpdatedAt: timePtr(time.Now())}
	g := makeGoal(1, 1, "", []domain.KeyResult{kr})
	g.Weight = 100

	teams := []domain.Team{makeTeam(1, "T1", "", nil)}
	goals := map[int64][]domain.Goal{1: {g}}
	statuses := map[int64]domain.TeamPeriodStatus{1: domain.TeamPeriodStatusReady}
	data := makePeriodData(teams, goals, statuses)
	result := computeCategories(data, []int64{1}, makeCfg(), time.Now())

	if result.Categories["awaiting_validation"].Count != 1 {
		t.Errorf("expected 1 awaiting_validation, got %d", result.Categories["awaiting_validation"].Count)
	}
}

func TestCategories_FormationError_WeightSum(t *testing.T) {
	kr := domain.KeyResult{ID: 1, GoalID: 1, Title: "KR", Weight: 100, Kind: domain.KRKindBoolean,
		Boolean: &domain.KRBoolean{}, ProgressUpdatedAt: timePtr(time.Now())}
	g1 := makeGoal(1, 1, "", []domain.KeyResult{kr})
	g1.Weight = 60
	g2 := makeGoal(2, 1, "", []domain.KeyResult{kr})
	g2.Weight = 60 // sum 120 ≠ 100

	teams := []domain.Team{makeTeam(1, "T1", "", nil)}
	goals := map[int64][]domain.Goal{1: {g1, g2}}
	statuses := map[int64]domain.TeamPeriodStatus{1: domain.TeamPeriodStatusInProgress}
	data := makePeriodData(teams, goals, statuses)
	result := computeCategories(data, []int64{1}, makeCfg(), time.Now())

	hasTeamError := false
	for _, item := range result.Categories["formation_errors"].Items {
		if item.EntityType == "team" && item.ErrorType == "weight_sum_not_100" {
			hasTeamError = true
		}
	}
	if !hasTeamError {
		t.Error("expected team-level weight_sum_not_100 error")
	}
}

func TestCategories_LaggingGoal(t *testing.T) {
	recent := time.Now().AddDate(0, 0, -1)
	kr := domain.KeyResult{ID: 1, GoalID: 1, Title: "KR", Weight: 100, Kind: domain.KRKindBoolean,
		Boolean: &domain.KRBoolean{IsDone: false}, ProgressUpdatedAt: timePtr(recent)}
	g := makeGoal(1, 1, "", []domain.KeyResult{kr})
	g.Weight = 100
	g.Progress = 0

	teams := []domain.Team{makeTeam(1, "T1", "", nil)}
	goals := map[int64][]domain.Goal{1: {g}}
	statuses := map[int64]domain.TeamPeriodStatus{1: domain.TeamPeriodStatusInProgress}
	// Period where 80% of time has elapsed (expectedPace ≈ 80), goal at 0 → lagging
	now := time.Now()
	data := &PeriodData{
		PeriodID: 1,
		Period: domain.Period{
			StartDate: now.AddDate(0, -4, 0),
			EndDate:   now.AddDate(0, 1, 0),
		},
		Teams: teams, GoalsByTeam: goals, Statuses: statuses,
	}
	result := computeCategories(data, []int64{1}, makeCfg(), now)
	if result.Categories["lagging"].Count != 1 {
		t.Errorf("expected 1 lagging, got %d", result.Categories["lagging"].Count)
	}
	// lagging not in counter by default
	if result.TotalProblems != 0 {
		t.Errorf("lagging should not count toward total; got %d", result.TotalProblems)
	}
}

func TestCalcExpectedPace(t *testing.T) {
	now := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	p := domain.Period{
		StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
	}
	pace := calcExpectedPace(p, now)
	// ~25% elapsed
	if pace < 20 || pace > 35 {
		t.Errorf("expected pace ~25, got %d", pace)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func toSet(ids []int64) map[int64]bool {
	m := make(map[int64]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}
```

- [ ] **Step 2: Запустить тесты (они должны упасть — функций ещё нет)**

```bash
go test ./internal/service/... -run TestComputeScope -v 2>&1 | head -20
```

Expected: `FAIL` с ошибкой компиляции или "undefined: computeScope" — OK.

- [ ] **Step 3: Запустить тесты после Task 2 (когда код написан)**

```bash
go test ./internal/service/... -run "TestComputeScope|TestOwnerText|TestCategories|TestCalcExpected" -v
```

Expected: все тесты `PASS`.

- [ ] **Step 4: Commit**

```bash
git add internal/service/healthcheckin_test.go
git commit -m "test: add unit tests for health-checkin scope and category computation"
```

---

## Task 5: Service.GetHealthCheckIn + wire cache в Deps

**Files:**
- Modify: `internal/service/service.go`
- Modify: `internal/service/healthcheckin.go` (добавить метод GetHealthCheckIn)

- [ ] **Step 1: Добавить `hcCache` в Deps и Service в `service.go`**

В `Deps` добавить поле:
```go
type Deps struct {
	Teams    TeamRepo
	Goals    GoalRepo
	Shares   GoalShareRepo
	Periods  PeriodRepo
	KRs      KRRepo
	Statuses TeamStatusRepo
	Users    UserRepo
	Grants   GrantsProvider
	HCCache  *HealthCheckInCache  // новое поле
}
```

В `Service` добавить поле:
```go
type Service struct {
	teams    TeamRepo
	goals    GoalRepo
	shares   GoalShareRepo
	periods  PeriodRepo
	krs      KRRepo
	statuses TeamStatusRepo
	users    UserRepo
	grants   GrantsProvider
	hcCache  *HealthCheckInCache  // новое поле
}
```

В конструкторе `New`:
```go
func New(deps Deps) *Service {
	return &Service{
		teams:    deps.Teams,
		goals:    deps.Goals,
		shares:   deps.Shares,
		periods:  deps.Periods,
		krs:      deps.KRs,
		statuses: deps.Statuses,
		users:    deps.Users,
		grants:   deps.Grants,
		hcCache:  deps.HCCache,
	}
}
```

Обновить `NewFromStore` — добавить параметр:
```go
func NewFromStore(st *store.Store, grantsProvider GrantsProvider, hcCache *HealthCheckInCache) *Service {
	return New(Deps{
		Teams:    st.Teams,
		Goals:    st.Goals,
		Shares:   st.Shares,
		Periods:  st.Periods,
		KRs:      st.KRs,
		Statuses: st.Statuses,
		Users:    st.Users,
		Grants:   grantsProvider,
		HCCache:  hcCache,
	})
}
```

- [ ] **Step 2: Добавить метод `GetHealthCheckIn` в `healthcheckin.go`**

```go
// GetHealthCheckIn computes the health check-in for the given user and period.
// Uses cached period data; loads from DB on first call or after TTL.
func (s *Service) GetHealthCheckIn(ctx context.Context, userDisplayName string, periodID int64, cfg HealthCheckInConfig) (*HealthCheckInResult, error) {
	if s.hcCache == nil {
		return &HealthCheckInResult{HasScope: false}, nil
	}
	data, err := s.hcCache.Get(ctx, periodID)
	if err != nil {
		return nil, err
	}
	scopeIDs := computeScope(data.Teams, data.GoalsByTeam, userDisplayName)
	if scopeIDs == nil {
		return &HealthCheckInResult{HasScope: false, PeriodID: periodID, Categories: emptyCategories(cfg)}, nil
	}
	return computeCategories(data, scopeIDs, cfg, time.Now()), nil
}

func emptyCategories(cfg HealthCheckInConfig) map[string]*HealthCheckInCategory {
	names := []string{"stale", "no_goals", "awaiting_validation", "formation_errors", "lagging"}
	cats := make(map[string]*HealthCheckInCategory, len(names))
	for _, n := range names {
		cats[n] = &HealthCheckInCategory{InCounter: cfg.InCounter[n], Items: []HealthCheckInItem{}}
	}
	return cats
}
```

- [ ] **Step 3: Исправить вызов `NewFromStore` в `server.go` (временно передать nil, до Task 8)**

Найти в `server.go`:
```go
service:     service.NewFromStore(st, grantsCache),
```
Заменить на:
```go
service:     service.NewFromStore(st, grantsCache, nil),
```

- [ ] **Step 4: Проверить компиляцию**

```bash
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add internal/service/service.go internal/service/healthcheckin.go
git commit -m "feat: add Service.GetHealthCheckIn and wire HealthCheckInCache into Deps"
```

---

## Task 6: HTTP handler + admin settings handlers

**Files:**
- Create: `internal/http/handlers/api/v1/healthcheckin/handler.go`

- [ ] **Step 1: Создать файл handler**

```go
package healthcheckin

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"okrs/internal/auth"
	"okrs/internal/service"
)

// serviceProvider exposes the service method used by this handler.
type serviceProvider interface {
	GetHealthCheckIn(ctx context.Context, userDisplayName string, periodID int64, cfg service.HealthCheckInConfig) (*service.HealthCheckInResult, error)
}

// settingsProvider reads system settings.
type settingsProvider interface {
	GetSetting(ctx context.Context, key string) (json.RawMessage, error)
	SetSetting(ctx context.Context, key string, value any) error
}

// cacheInvalidator allows flushing the cache when config changes.
type cacheInvalidator interface {
	InvalidateAll()
}

// Handler handles GET /api/v1/health-checkin and admin settings endpoints.
type Handler struct {
	svc      serviceProvider
	settings settingsProvider
	cache    cacheInvalidator
}

// New creates a Handler.
func New(svc serviceProvider, settings settingsProvider, cache cacheInvalidator) *Handler {
	return &Handler{svc: svc, settings: settings, cache: cache}
}

// HandleHealthCheckIn serves GET /api/v1/health-checkin?period_id=X
func (h *Handler) HandleHealthCheckIn(w http.ResponseWriter, r *http.Request) {
	periodIDStr := r.URL.Query().Get("period_id")
	periodID, err := strconv.ParseInt(periodIDStr, 10, 64)
	if err != nil || periodID <= 0 {
		writeError(w, http.StatusBadRequest, "period_id required")
		return
	}

	user := auth.UserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	cfg, err := service.LoadHealthCheckInConfig(r.Context(), h.settings)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return
	}

	result, err := h.svc.GetHealthCheckIn(r.Context(), user.DisplayName, periodID, cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, result)
}

// HandleGetHealthCheckInSettings serves GET /api/v1/admin/settings/health-checkin
func (h *Handler) HandleGetHealthCheckInSettings(w http.ResponseWriter, r *http.Request) {
	cfg, err := service.LoadHealthCheckInConfig(r.Context(), h.settings)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, cfg)
}

// HandleUpdateHealthCheckInSettings serves POST /api/v1/admin/settings/health-checkin
func (h *Handler) HandleUpdateHealthCheckInSettings(w http.ResponseWriter, r *http.Request) {
	var body service.HealthCheckInConfig
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.StaleDays <= 0 {
		writeError(w, http.StatusBadRequest, "stale_days must be > 0")
		return
	}
	if body.CacheTTLMinutes <= 0 {
		writeError(w, http.StatusBadRequest, "cache_ttl_minutes must be > 0")
		return
	}
	if err := h.settings.SetSetting(r.Context(), "health_checkin_config", body); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.cache != nil {
		h.cache.InvalidateAll()
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
```

- [ ] **Step 2: Проверить компиляцию**

```bash
go build ./internal/http/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/http/handlers/api/v1/healthcheckin/handler.go
git commit -m "feat: add health-checkin HTTP handler and admin settings handlers"
```

---

## Task 7: Инициализация кеша и регистрация маршрутов в server.go

**Files:**
- Modify: `internal/http/server.go`

- [ ] **Step 1: Добавить поле `hcCache` в Server и обновить конструктор**

Добавить поле в `Server`:
```go
type Server struct {
	store       *store.Store
	logger      *slog.Logger
	tmpl        *template.Template
	zone        *time.Location
	service     *service.Service
	auth        *auth.Manager
	policy      *auth.PolicyEvaluator
	grantsCache *grants.GrantsCache
	hcCache     *service.HealthCheckInCache  // новое
}
```

- [ ] **Step 2: Создать `periodLoader` closure и инициализировать кеш в `NewServer`**

В конце `NewServer`, перед `return`:

```go
// Build period loader for HealthCheckInCache.
hcLoader := func(ctx context.Context, periodID int64) (*service.PeriodData, error) {
    period, err := st.Periods.GetPeriod(ctx, periodID)
    if err != nil {
        return nil, err
    }
    allTeams, err := st.Teams.ListAllTeams(ctx)
    if err != nil {
        return nil, err
    }
    allTeamIDs := make([]int64, len(allTeams))
    for i, t := range allTeams {
        allTeamIDs[i] = t.ID
    }
    goalsByTeam, err := st.Goals.ListGoalsByTeamsPeriod(ctx, periodID, allTeamIDs)
    if err != nil {
        return nil, err
    }
    statuses, err := st.Statuses.ListTeamPeriodStatuses(ctx, periodID, allTeamIDs)
    if err != nil {
        return nil, err
    }
    return &service.PeriodData{
        PeriodID:    periodID,
        Period:      period,
        Teams:       allTeams,
        GoalsByTeam: goalsByTeam,
        Statuses:    statuses,
        CachedAt:    time.Now(),
    }, nil
}

cacheTTL := 5 * time.Minute
hcCache := service.NewHealthCheckInCache(hcLoader, cacheTTL, logger)

return &Server{
    store:       st,
    logger:      logger,
    tmpl:        tmpl,
    zone:        zone,
    service:     service.NewFromStore(st, grantsCache, hcCache),  // передаём кеш
    auth:        authMgr,
    policy:      auth.NewPolicyEvaluator(grantsCache, logger),
    grantsCache: grantsCache,
    hcCache:     hcCache,
}, nil
```

- [ ] **Step 3: Запустить фоновый refresh в `Routes()`**

В начале `Routes()` добавить:
```go
func (s *Server) Routes() http.Handler {
    // Start background refresh for health-checkin cache.
    ctx := context.Background()
    s.hcCache.StartRefreshLoop(ctx, 5*time.Minute, func(ctx context.Context) int64 {
        p, err := s.service.FindPeriodForDate(ctx, time.Now().In(s.zone))
        if err != nil {
            return 0
        }
        return p.ID
    })
    // ... rest of Routes()
```

Добавить import `"context"` если не было.

- [ ] **Step 4: Зарегистрировать маршруты в `registerApiRoutes` и `registerAdminRoutes`**

В `registerApiRoutes`:
```go
import apihealthcheckin "okrs/internal/http/handlers/api/v1/healthcheckin"

// в теле функции registerApiRoutes:
hcHandler := apihealthcheckin.New(s.service, s.store.Settings, s.hcCache)
r.Get("/api/v1/health-checkin", hcHandler.HandleHealthCheckIn)
```

В `registerAdminRoutes`, внутри admin-only блока:
```go
r.Get("/api/v1/admin/settings/health-checkin", hcHandler.HandleGetHealthCheckInSettings)
r.Post("/api/v1/admin/settings/health-checkin", hcHandler.HandleUpdateHealthCheckInSettings)
```

Также добавить admin shell route:
```go
r.Get("/admin/health-checkin", adminShell)
```

- [ ] **Step 5: Проверить компиляцию и запуск**

```bash
go build ./...
```

```bash
# Проверить что сервер стартует:
go run ./cmd/... &
sleep 2
curl -s http://127.0.0.1:8080/api/v1/health-checkin?period_id=1 | head -c 200
kill %1
```

Expected: JSON ответ (может быть `{"has_scope":false,...}` в demo режиме).

- [ ] **Step 6: Commit**

```bash
git add internal/http/server.go
git commit -m "feat: initialize HealthCheckInCache and register health-checkin routes"
```

---

## Task 8: tracker.css — новые CSS классы

**Files:**
- Modify: `internal/web/static/tracker.css`

- [ ] **Step 1: Добавить классы в конец файла**

```css
/* ── Health Check-in ────────────────────────────────────────────────── */
.hci-button {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 9px 12px;
  border: none;
  background: rgba(124, 58, 237, 0.12);
  color: #c4b5fd;
  border-radius: 8px;
  cursor: pointer;
  font-family: inherit;
  font-size: 13px;
  font-weight: 600;
  text-align: left;
  transition: background 0.15s;
}
.hci-button:hover { background: rgba(124, 58, 237, 0.22); }

.hci-badge {
  min-width: 20px;
  height: 20px;
  padding: 0 6px;
  border-radius: 10px;
  font-size: 11px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
  margin-left: auto;
  background: #f59e0b;
  color: #fff;
}
.hci-badge--zero {
  background: #6b7280;
  color: #fff;
}

/* Backdrop */
.hci-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.35);
  z-index: 200;
  animation: hciFadeIn 0.18s ease;
}
@keyframes hciFadeIn { from { opacity: 0; } to { opacity: 1; } }

/* Panel */
.hci-panel {
  position: fixed;
  top: 0;
  right: 0;
  width: 480px;
  max-width: 100vw;
  height: 100vh;
  background: #fff;
  box-shadow: -4px 0 24px rgba(0, 0, 0, 0.14);
  z-index: 201;
  display: flex;
  flex-direction: column;
  transform: translateX(100%);
  transition: transform 0.22s cubic-bezier(0.4, 0, 0.2, 1);
}
.hci-panel--open { transform: translateX(0); }

.hci-panel__header {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 20px 20px 16px;
  border-bottom: 1px solid #e5e7eb;
  flex-shrink: 0;
}
.hci-panel__title {
  font-size: 16px;
  font-weight: 700;
  color: #0f172a;
  margin: 0;
}
.hci-panel__subtitle {
  font-size: 12px;
  color: #6b7280;
  margin: 2px 0 0;
}
.hci-panel__close {
  margin-left: auto;
  width: 28px;
  height: 28px;
  border: none;
  background: #f1f5f9;
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.hci-panel__close:hover { background: #e2e8f0; }

/* Filter chips */
.hci-chips {
  display: flex;
  gap: 6px;
  padding: 10px 20px;
  border-bottom: 1px solid #f1f5f9;
  flex-wrap: wrap;
  flex-shrink: 0;
}
.hci-chip {
  padding: 4px 10px;
  border-radius: 12px;
  border: 1px solid #e5e7eb;
  background: #fff;
  font-size: 12px;
  font-weight: 500;
  color: #374151;
  cursor: pointer;
  white-space: nowrap;
  transition: all 0.12s;
}
.hci-chip:hover { border-color: #9ca3af; }
.hci-chip--active {
  background: #7c3aed;
  border-color: #7c3aed;
  color: #fff;
}

/* Body / sections */
.hci-body {
  flex: 1;
  overflow-y: auto;
  padding: 8px 0 24px;
}

.hci-section { margin-bottom: 4px; }

.hci-section__header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 20px 6px;
  font-size: 12px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: #374151;
}
.hci-section__count {
  margin-left: auto;
  font-size: 11px;
  font-weight: 600;
  color: #9ca3af;
}

/* Team group within section */
.hci-team { padding: 4px 20px; }
.hci-team__name {
  font-size: 12px;
  font-weight: 600;
  color: #6b7280;
  margin-bottom: 4px;
  display: flex;
  align-items: center;
  gap: 4px;
}

/* Item row */
.hci-item {
  padding: 8px 12px;
  border-radius: 8px;
  background: #f8fafc;
  border: 1px solid #e5e7eb;
  margin-bottom: 6px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.hci-item__title {
  font-size: 13px;
  color: #111827;
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.hci-item__meta {
  font-size: 11px;
  color: #6b7280;
}
.hci-item__action {
  font-size: 12px;
  font-weight: 600;
  color: #7c3aed;
  background: none;
  border: none;
  cursor: pointer;
  padding: 0;
  text-align: left;
  text-decoration: underline;
}
.hci-item__action:hover { color: #5b21b6; }

/* Empty state */
.hci-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 48px 24px;
  text-align: center;
  color: #9ca3af;
  font-size: 14px;
  gap: 8px;
}
.hci-empty__icon { font-size: 32px; }
```

- [ ] **Step 2: Verify в браузере что CSS загружается**

Открыть `http://127.0.0.1:8080/teamOkrs` в браузере.
Открыть DevTools → Network → reload → найти `tracker.css`, убедиться нет 404.

- [ ] **Step 3: Commit**

```bash
git add internal/web/static/tracker.css
git commit -m "feat: add health-checkin CSS classes"
```

---

## Task 9: HealthCheckInButton + HealthCheckInPanel в tracker.js

**Files:**
- Modify: `internal/web/static/tracker.js`

Найти компонент `App` (функция верхнего уровня, которая рендерит Sidebar + main content).

- [ ] **Step 1: Добавить state + data fetch в App**

В App-компоненте найти блок с `const [me, setMe]` / `useEffect` для начальной загрузки данных. Добавить рядом:

```js
const [hciData, setHciData] = useState(null);
const [hciOpen, setHciOpen] = useState(false);

const loadHCI = useCallback((pid) => {
  if (!pid) return;
  apiGet(`/api/v1/health-checkin?period_id=${pid}`)
    .then(d => d && setHciData(d));
}, []);

// Перезагружать при смене periodId (добавить periodId в deps существующего useEffect или создать новый):
useEffect(() => { loadHCI(periodId); }, [periodId]);
```

- [ ] **Step 2: Добавить функцию `HealthCheckInButton` перед App**

```js
const HCI_CAT_META = {
  stale:               { icon: '🕐', label: 'Нет обновлений',         color: '#f59e0b' },
  no_goals:            { icon: '○',  label: 'Нет целей',              color: '#6b7280' },
  awaiting_validation: { icon: '○',  label: 'Ожидают перевода',       color: '#6b7280' },
  formation_errors:    { icon: '⚠',  label: 'Ошибки формирования',    color: '#ef4444' },
  lagging:             { icon: '▼',  label: 'Отстающие',              color: '#3b82f6' },
};

const HCI_CAT_ORDER = ['stale', 'no_goals', 'awaiting_validation', 'formation_errors', 'lagging'];

const HCI_ACTION_LABEL = {
  stale:               '→ Обновить прогресс',
  no_goals:            '→ Перейти к команде',
  awaiting_validation: '→ Перейти к команде',
  formation_errors:    '→ Исправить',
  lagging:             '→ Перейти к цели',
};

function HealthCheckInButton({ data, onClick }) {
  if (!data || !data.has_scope) return null;
  const count = data.total_problems;
  return (
    <button className="hci-button" onClick={onClick}>
      <span>⚡ Health Check-in</span>
      <span className={`hci-badge${count === 0 ? ' hci-badge--zero' : ''}`}>{count}</span>
    </button>
  );
}
```

- [ ] **Step 3: Добавить компонент `HealthCheckInPanel`**

```js
function HealthCheckInPanel({ data, open, onClose, onSelectTeam }) {
  const [filter, setFilter] = useState(null);

  if (!data) return null;

  const subtitle = (() => {
    if (data.total_problems > 0) return `Найдено проблем: ${data.total_problems}`;
    const lagging = data.categories?.lagging?.count ?? 0;
    if (lagging > 0) return 'Проблем нет · есть отстающие цели';
    return 'Всё в порядке';
  })();

  // Build non-empty categories for chips
  const nonEmptyCats = HCI_CAT_ORDER.filter(k => (data.categories?.[k]?.count ?? 0) > 0);

  // All items grouped by category → team
  const visibleCats = filter ? [filter] : HCI_CAT_ORDER;

  return (
    <>
      {open && <div className="hci-backdrop" onClick={onClose}/>}
      <div className={`hci-panel${open ? ' hci-panel--open' : ''}`}>
        <div className="hci-panel__header">
          <div style={{flex:1}}>
            <p className="hci-panel__title">⚡ Health Check-in</p>
            <p className="hci-panel__subtitle">{subtitle}</p>
          </div>
          <button className="hci-panel__close" onClick={onClose}>✕</button>
        </div>

        {nonEmptyCats.length > 0 && (
          <div className="hci-chips">
            <button
              className={`hci-chip${!filter ? ' hci-chip--active' : ''}`}
              onClick={() => setFilter(null)}>
              Все · {data.total_problems + (data.categories?.lagging?.count ?? 0)}
            </button>
            {nonEmptyCats.map(k => {
              const cat = data.categories[k];
              const meta = HCI_CAT_META[k];
              return (
                <button
                  key={k}
                  className={`hci-chip${filter === k ? ' hci-chip--active' : ''}`}
                  onClick={() => setFilter(filter === k ? null : k)}>
                  {meta.icon} {cat.count}
                </button>
              );
            })}
          </div>
        )}

        <div className="hci-body">
          {visibleCats.every(k => (data.categories?.[k]?.count ?? 0) === 0) ? (
            <div className="hci-empty">
              <span className="hci-empty__icon">{filter ? '🔍' : '✅'}</span>
              <span>{filter ? 'По выбранному фильтру ничего нет' : 'Всё ok'}</span>
            </div>
          ) : (
            visibleCats.map(k => {
              const cat = data.categories?.[k];
              if (!cat || cat.count === 0) return null;
              const meta = HCI_CAT_META[k];

              // Group items by team
              const byTeam = {};
              for (const item of cat.items) {
                const key = item.team_id;
                if (!byTeam[key]) byTeam[key] = { name: item.team_name, path: item.team_path, items: [] };
                byTeam[key].items.push(item);
              }

              return (
                <div key={k} className="hci-section">
                  <div className="hci-section__header" style={{color: meta.color}}>
                    <span>{meta.icon}</span>
                    <span>{meta.label}</span>
                    <span className="hci-section__count">{cat.count}</span>
                  </div>
                  {Object.entries(byTeam).map(([teamIdStr, group]) => (
                    <div key={teamIdStr} className="hci-team">
                      <div className="hci-team__name">
                        <span>▸</span>
                        <span>{group.path?.join(' › ') || group.name}</span>
                      </div>
                      {group.items.map((item, idx) => (
                        <div key={idx} className="hci-item">
                          {item.goal_title && (
                            <div className="hci-item__title">{item.goal_title}</div>
                          )}
                          {item.days_since_update > 0 && (
                            <div className="hci-item__meta">{item.days_since_update} дн. без обновлений</div>
                          )}
                          {item.error_type && (
                            <div className="hci-item__meta">{formatErrorType(item.error_type, item)}</div>
                          )}
                          {item.progress !== undefined && item.expected_pace !== undefined && (
                            <div className="hci-item__meta">Прогресс: {item.progress}% · Ожидалось: {item.expected_pace}%</div>
                          )}
                          <button
                            className="hci-item__action"
                            onClick={() => { onSelectTeam(item.team_id, item.goal_id); onClose(); }}>
                            {HCI_ACTION_LABEL[k]}
                          </button>
                        </div>
                      ))}
                    </div>
                  ))}
                </div>
              );
            })
          )}
        </div>
      </div>
    </>
  );
}

function formatErrorType(errType, item) {
  const labels = {
    weight_sum_not_100: `Сумма весов целей: ${item.actual_weight_sum ?? '?'}% (должно быть 100%)`,
    no_krs: 'У цели нет ключевых результатов',
    kr_weight_sum_not_100: 'Сумма весов KR ≠ 100%',
    project_no_stages: 'PROJECT KR без шагов',
    project_stage_weight_sum_not_100: 'Сумма весов шагов ≠ 100%',
    kr_zero_range: 'Нулевой диапазон (start = target)',
    kr_no_title: 'KR без названия',
  };
  return labels[errType] || errType;
}
```

- [ ] **Step 4: Вставить кнопку в Sidebar**

Найти в компоненте Sidebar (или рядом с account-widget) место для кнопки. Добавить перед account-widget:

```js
// В JSX sidebar-а, перед блоком с аватаром/выходом:
<div style={{padding: '8px 8px 0'}}>
  <HealthCheckInButton data={hciData} onClick={() => setHciOpen(true)}/>
</div>
```

- [ ] **Step 5: Вставить Panel в App (после sidebar)**

В JSX App-а добавить рядом с основным layout:
```js
<HealthCheckInPanel
  data={hciData}
  open={hciOpen}
  onClose={() => setHciOpen(false)}
  onSelectTeam={(teamId, goalId) => {
    selectTeam(teamId);  // существующая функция выбора команды
    if (goalId) {
      // опционально: прокрутить к цели после загрузки
      setTimeout(() => {
        const el = document.getElementById(`goal-${goalId}`);
        if (el) el.scrollIntoView({ behavior: 'smooth', block: 'center' });
      }, 400);
    }
  }}
/>
```

- [ ] **Step 6: Добавить перезагрузку после мутаций**

В callback-ах обновления KR-прогресса / смены статуса добавить:
```js
// после успешного fetch:
loadHCI(periodId);
```

- [ ] **Step 7: Ручное тестирование**

Открыть `http://127.0.0.1:8080/teamOkrs`. Проверить:
- [ ] Кнопка "⚡ Health Check-in" появляется в нижней части sidebar (если `has_scope: true`)
- [ ] Клик открывает панель справа с backdrop
- [ ] Badge показывает корректный счётчик
- [ ] Фильтр-чипы работают (клик → фильтрует, повторный → сброс)
- [ ] Клик по action-ссылке выбирает команду в sidebar

- [ ] **Step 8: Commit**

```bash
git add internal/web/static/tracker.js
git commit -m "feat: add HealthCheckInButton and HealthCheckInPanel to tracker"
```

---

## Task 10: Admin Health Check-in settings в admin.js

**Files:**
- Modify: `internal/web/static/admin.js`

- [ ] **Step 1: Добавить раздел в navigation array**

Найти строки (~325-329):
```js
const sections = [
  {id:'periods',  label:'Периоды', ...},
  {id:'teams',    label:'Команды', ...},
  {id:'users',    label:'Пользователи', ...},
  {id:'settings', label:'Настройки', ...},
];
```
Добавить новый раздел:
```js
{id:'health-checkin', label:'Health Check-in', hint:'Настройки проверок', icon:'⚡'},
```

- [ ] **Step 2: Добавить компонент `HealthCheckInSettingsPanel` перед Shell**

```js
function HealthCheckInSettingsPanel() {
  const [cfg, setCfg] = useState(null);
  const [saving, setSaving] = useState(false);
  const [msg, setMsg] = useState('');

  useEffect(() => {
    apiGet('/api/v1/admin/settings/health-checkin')
      .then(r => r && r.json())
      .then(d => d && setCfg(d));
  }, []);

  if (!cfg) return <div style={{padding:24, color: T.mutedFg}}>Загрузка…</div>;

  const update = (key, val) => setCfg(prev => ({...prev, [key]: val}));
  const updateCounter = (k, val) => setCfg(prev => ({...prev, in_counter: {...prev.in_counter, [k]: val}}));

  const save = async () => {
    setSaving(true); setMsg('');
    const res = await apiPost('/api/v1/admin/settings/health-checkin', cfg);
    setSaving(false);
    setMsg(res && res.ok ? 'Сохранено' : 'Ошибка сохранения');
  };

  const catConfig = [
    {
      key: 'stale', icon: '🕐', label: 'Нет обновлений',
      hint: 'Цели и KR без обновления прогресса более N дней. Руководителю нужно напомнить команде обновить прогресс.',
      param: { field: 'stale_days', label: 'Порог (дней без обновления)', min: 1 },
    },
    {
      key: 'no_goals', icon: '○', label: 'Не заведены цели',
      hint: 'Команды без ни одной цели в периоде. Руководителю нужно инициировать заведение OKR.',
    },
    {
      key: 'awaiting_validation', icon: '○', label: 'Ожидают перевода в работу',
      hint: 'Команды со статусом «Черновик» или «К валидации». Нужно перевести в «В работе».',
    },
    {
      key: 'formation_errors', icon: '⚠', label: 'Ошибки формирования',
      hint: 'Суммы весов ≠ 100%, отсутствие KR, нулевые диапазоны. Мешают корректному расчёту прогресса.',
      param: { field: 'weight_tolerance', label: 'Допуск по весам (%)', min: 0 },
    },
    {
      key: 'lagging', icon: '▼', label: 'Отстающие',
      hint: 'Цели ниже ожидаемого темпа периода. Информационная категория — показывает риски.',
      param: { field: 'behind_margin', label: 'Отставание (п.п.)', min: 1 },
    },
  ];

  const fieldStyle = {background:'#f8fafc', border:'1px solid #e5e7eb', borderRadius:6, padding:'6px 10px', fontSize:13, width:70, fontFamily:'inherit'};
  const labelStyle = {fontSize:13, color: T.bodyFg, fontWeight:500};
  const hintStyle  = {fontSize:12, color: T.mutedFg, marginTop:4, lineHeight:1.5};

  return (
    <div style={{padding:'20px 24px 32px'}}>
      <div style={{background:'white', borderRadius:12, border:'1px solid '+T.cardBorder, padding:'20px 24px'}}>
        <div style={{fontSize:15, fontWeight:700, color: T.headingFg, marginBottom:4}}>⚡ Health Check-in — настройки</div>
        <div style={{fontSize:13, color: T.mutedFg, marginBottom:24}}>
          Определяют, какие проблемы попадают в счётчик кнопки ⚡ Health Check-in в трекере.
        </div>

        {catConfig.map(cat => (
          <div key={cat.key} style={{borderTop:'1px solid #f1f5f9', paddingTop:16, marginTop:16}}>
            <div style={{display:'flex', alignItems:'flex-start', gap:12}}>
              <div style={{flex:1}}>
                <div style={{display:'flex', alignItems:'center', gap:8, marginBottom:4}}>
                  <span style={{fontSize:16}}>{cat.icon}</span>
                  <span style={{fontSize:14, fontWeight:600, color: T.headingFg}}>{cat.label}</span>
                </div>
                <div style={hintStyle}>{cat.hint}</div>
                {cat.param && (
                  <div style={{display:'flex', alignItems:'center', gap:8, marginTop:10}}>
                    <span style={labelStyle}>{cat.param.label}:</span>
                    <input
                      type="number" min={cat.param.min}
                      value={cfg[cat.param.field] ?? ''}
                      onChange={e => update(cat.param.field, Number(e.target.value))}
                      style={fieldStyle}
                    />
                  </div>
                )}
              </div>
              <div style={{display:'flex', flexDirection:'column', alignItems:'flex-end', gap:4, flexShrink:0}}>
                <label style={{display:'flex', alignItems:'center', gap:6, cursor:'pointer'}}>
                  <input
                    type="checkbox"
                    checked={cfg.in_counter?.[cat.key] ?? false}
                    onChange={e => updateCounter(cat.key, e.target.checked)}
                  />
                  <span style={{fontSize:12, color: T.mutedFg}}>В счётчик</span>
                </label>
              </div>
            </div>
          </div>
        ))}

        <div style={{borderTop:'1px solid #f1f5f9', paddingTop:16, marginTop:16}}>
          <div style={{fontSize:14, fontWeight:600, color: T.headingFg, marginBottom:8}}>Кеш</div>
          <div style={{display:'flex', alignItems:'center', gap:8, marginBottom:6}}>
            <span style={labelStyle}>Время жизни (мин):</span>
            <input
              type="number" min={1}
              value={cfg.cache_ttl_minutes ?? 5}
              onChange={e => update('cache_ttl_minutes', Number(e.target.value))}
              style={fieldStyle}
            />
          </div>
          <div style={hintStyle}>Интервал фонового пересчёта. Меньше — актуальнее данные, больше — меньше нагрузка на БД.</div>
        </div>

        <div style={{marginTop:24, display:'flex', alignItems:'center', gap:12}}>
          <button
            onClick={save}
            disabled={saving}
            style={{padding:'8px 20px', background: T.accent, color:'white', border:'none', borderRadius:8, fontSize:13, fontWeight:600, cursor:'pointer'}}>
            {saving ? 'Сохранение…' : 'Сохранить'}
          </button>
          {msg && <span style={{fontSize:13, color: msg.startsWith('Ош') ? T.danger : T.success}}>{msg}</span>}
        </div>
      </div>
    </div>
  );
}
```

- [ ] **Step 3: Добавить рендер раздела в Shell (строка ~1004)**

Найти:
```js
{section==='settings'&&<div ...><AccessSettingsPanel teams={teams}/></div></div>}
```
После добавить:
```js
{section==='health-checkin'&&<HealthCheckInSettingsPanel/>}
```

- [ ] **Step 4: Ручное тестирование**

Открыть `http://127.0.0.1:8080/admin/health-checkin`. Проверить:
- [ ] Новая вкладка «⚡ Health Check-in» появляется в sidebar
- [ ] Форма показывает текущие настройки (дефолтные при первом открытии)
- [ ] Изменение значений и «Сохранить» отдаёт 204
- [ ] После сохранения reload страницы показывает сохранённые значения

- [ ] **Step 5: Commit**

```bash
git add internal/web/static/admin.js
git commit -m "feat: add Health Check-in settings panel in admin"
```

---

## Task 11: Обновить specs

**Files:**
- Modify: `specs/040-api-contract.md`
- Modify: `specs/020-domain-model.md`

- [ ] **Step 1: Добавить endpoint в `specs/040-api-contract.md`**

Добавить новый раздел после `## Read endpoints`:

```markdown
### `GET /api/v1/health-checkin?period_id=<int64>`

Назначение: вычислить сводку Health Check-in для текущего пользователя за выбранный период.

Доступен: любому авторизованному пользователю.

Scope: сервер определяет по `user.display_name` из сессии:
- lead-scope: команды, где `teams.lead = display_name` + все потомки
- owner-scope: команды с целями, где `goal.owner_text` содержит `display_name` (word match)

Request params:
- `period_id` (int64, обязательный)

Success response (`200`):
```json
{
  "has_scope": true,
  "period_id": 1,
  "total_problems": 5,
  "categories": {
    "stale":               { "in_counter": true,  "count": 2, "items": [...] },
    "no_goals":            { "in_counter": true,  "count": 1, "items": [...] },
    "awaiting_validation": { "in_counter": true,  "count": 1, "items": [...] },
    "formation_errors":    { "in_counter": true,  "count": 1, "items": [...] },
    "lagging":             { "in_counter": false, "count": 0, "items": [] }
  }
}
```

`total_problems` = Σ `count` по категориям с `in_counter: true`.

Errors:
- `400 VALIDATION_ERROR` при отсутствии или невалидном `period_id`

Idempotency: read-only, нет side effects.

---

### `GET /api/v1/admin/settings/health-checkin`

Возвращает текущий конфиг Health Check-in. Default применяется если ключ в БД отсутствует.

---

### `POST /api/v1/admin/settings/health-checkin`

Обновляет конфиг. Требует admin-роли и CSRF token. Body: `HealthCheckInConfig` JSON.
После сохранения сбрасывает in-memory кеш.

Errors: `400` при `stale_days <= 0` или `cache_ttl_minutes <= 0`.
```

- [ ] **Step 2: Добавить ключ в `specs/020-domain-model.md`**

В раздел `SystemSettings`, в таблицу `Текущие ключи` добавить строку:

```markdown
| health_checkin_config | object | Настройки Health Check-in: `stale_days`, `behind_margin`, `weight_tolerance`, `cache_ttl_minutes`, `in_counter` (map) |
```

- [ ] **Step 3: Commit**

```bash
git add specs/040-api-contract.md specs/020-domain-model.md
git commit -m "docs: update specs with health-checkin endpoint and settings"
```

---

## Self-review checklist

После завершения всех задач:

- [ ] `GET /api/v1/health-checkin` → корректный JSON с `has_scope`, `total_problems`, `categories`
- [ ] Кнопка не рендерится при `has_scope: false`
- [ ] Badge и subtitle в панели из одного источника (`hciData.total_problems`)
- [ ] Фильтр-чипы: клик фильтрует, повторный — сброс, при пустом результате — "По выбранному фильтру ничего нет"
- [ ] Все 5 категорий проверяются (включая formation_errors на уровне team + goal + KR)
- [ ] Кеш: фоновый refresh горутина запущена, не паникует при отсутствии активного периода
- [ ] Admin settings: GET возвращает defaults при пустой БД, POST сохраняет и сбрасывает кеш
- [ ] `go test ./internal/service/...` — все тесты проходят
- [ ] `go build ./...` — компилируется без ошибок
