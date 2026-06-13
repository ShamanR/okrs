package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"okrs/internal/domain"
)

// HealthCheckInConfig controls thresholds and counter membership for each category.
// Loaded from system_settings key "health_checkin_config"; defaults apply when key is absent.
type HealthCheckInConfig struct {
	StaleDays       int             `json:"stale_days"`
	BehindMargin    int             `json:"behind_margin"`
	WeightTolerance int             `json:"weight_tolerance"`
	CacheTTLMinutes int             `json:"cache_ttl_minutes"`
	InCounter       map[string]bool `json:"in_counter"`
}

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
	for k, v := range defaultHealthCheckInConfig.InCounter {
		if _, ok := cfg.InCounter[k]; !ok {
			cfg.InCounter[k] = v
		}
	}
	return cfg, nil
}

// ── Result types ──────────────────────────────────────────────────────────────

type HealthCheckInItem struct {
	TeamID          int64    `json:"team_id"`
	TeamName        string   `json:"team_name"`
	TeamPath        []string `json:"team_path"`
	GoalID          int64    `json:"goal_id,omitempty"`
	GoalTitle       string   `json:"goal_title,omitempty"`
	EntityType      string   `json:"entity_type,omitempty"`
	ErrorType       string   `json:"error_type,omitempty"`
	Status          string   `json:"status,omitempty"`
	DaysSinceUpdate int      `json:"days_since_update,omitempty"`
	Progress        int      `json:"progress,omitempty"`
	ExpectedPace    int      `json:"expected_pace,omitempty"`
	ActualWeightSum int      `json:"actual_weight_sum,omitempty"`
}

type HealthCheckInCategory struct {
	InCounter bool                `json:"in_counter"`
	Count     int                 `json:"count"`
	Items     []HealthCheckInItem `json:"items"`
}

type HealthCheckInResult struct {
	HasScope      bool                              `json:"has_scope"`
	PeriodID      int64                             `json:"period_id"`
	TotalProblems int                               `json:"total_problems"`
	Categories    map[string]*HealthCheckInCategory `json:"categories"`
}

// PeriodData is the pre-loaded data for one period, held in HealthCheckInCache.
type PeriodData struct {
	PeriodID    int64
	Period      domain.Period
	Teams       []domain.Team
	GoalsByTeam map[int64][]domain.Goal
	Statuses    map[int64]domain.TeamPeriodStatus
	CachedAt    time.Time
}

// ── Scope computation ─────────────────────────────────────────────────────────

func computeScope(teams []domain.Team, goalsByTeam map[int64][]domain.Goal, userUDID string) []int64 {
	if userUDID == "" {
		return nil
	}

	childrenMap := make(map[int64][]int64, len(teams))
	for _, t := range teams {
		if t.ParentID != nil {
			childrenMap[*t.ParentID] = append(childrenMap[*t.ParentID], t.ID)
		}
	}

	scopeSet := make(map[int64]struct{})

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
		if t.DeletedAt == nil && t.LeadUDID != nil && *t.LeadUDID == userUDID {
			addDescendants(t.ID)
		}
	}

	for teamID, goals := range goalsByTeam {
		for _, g := range goals {
			for _, uid := range g.OwnerUDIDs {
				if uid == userUDID {
					scopeSet[teamID] = struct{}{}
					break
				}
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

// ── Category computation ──────────────────────────────────────────────────────

func computeCategories(data *PeriodData, scopeIDs []int64, cfg HealthCheckInConfig, now time.Time) *HealthCheckInResult {
	scopeSet := make(map[int64]struct{}, len(scopeIDs))
	for _, id := range scopeIDs {
		scopeSet[id] = struct{}{}
	}

	teamsByID := make(map[int64]domain.Team, len(data.Teams))
	for _, t := range data.Teams {
		teamsByID[t.ID] = t
	}

	cats := map[string]*HealthCheckInCategory{
		"stale":               {InCounter: cfg.InCounter["stale"], Items: []HealthCheckInItem{}},
		"no_goals":            {InCounter: cfg.InCounter["no_goals"], Items: []HealthCheckInItem{}},
		"awaiting_validation": {InCounter: cfg.InCounter["awaiting_validation"], Items: []HealthCheckInItem{}},
		"formation_errors":    {InCounter: cfg.InCounter["formation_errors"], Items: []HealthCheckInItem{}},
		"lagging":             {InCounter: cfg.InCounter["lagging"], Items: []HealthCheckInItem{}},
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

		if len(goals) == 0 {
			cats["no_goals"].Items = append(cats["no_goals"].Items, HealthCheckInItem{
				TeamID: teamID, TeamName: team.Name, TeamPath: path,
			})
		}

		if len(goals) > 0 && status == domain.TeamPeriodStatusReady {
			cats["awaiting_validation"].Items = append(cats["awaiting_validation"].Items, HealthCheckInItem{
				TeamID: teamID, TeamName: team.Name, TeamPath: path,
				Status: string(status),
			})
		}

		// Formation errors are only relevant for teams that are being validated or
		// already in progress; drafts and closed periods are excluded.
		checkFormation := status == domain.TeamPeriodStatusReady || status == domain.TeamPeriodStatusInProgress

		if checkFormation && len(goals) > 0 {
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

		for _, g := range goals {
			if g.TeamID != teamID {
				continue
			}
			goalProgress := CalculateGoalProgress(&g)
			lastProgress := goalLastProgressAt(g)
			daysSince := 0
			if lastProgress != nil {
				daysSince = int(now.Sub(*lastProgress).Hours() / 24)
			}
			isStale := len(g.KeyResults) > 0 && (lastProgress == nil || daysSince > cfg.StaleDays)

			if isStale {
				cats["stale"].Items = append(cats["stale"].Items, HealthCheckInItem{
					TeamID: teamID, TeamName: team.Name, TeamPath: path,
					GoalID: g.ID, GoalTitle: g.Title,
					DaysSinceUpdate: daysSince,
				})
			}

			if !isStale && goalProgress < expectedPace-cfg.BehindMargin {
				cats["lagging"].Items = append(cats["lagging"].Items, HealthCheckInItem{
					TeamID: teamID, TeamName: team.Name, TeamPath: path,
					GoalID: g.ID, GoalTitle: g.Title,
					Progress: goalProgress, ExpectedPace: expectedPace,
				})
			}

			if checkFormation {
				goalErrors := checkGoalFormationErrors(g, cfg.WeightTolerance)
				for i := range goalErrors {
					goalErrors[i].TeamName = team.Name
					goalErrors[i].TeamPath = path
				}
				cats["formation_errors"].Items = append(cats["formation_errors"].Items, goalErrors...)
			}
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

func checkGoalFormationErrors(g domain.Goal, weightTolerance int) []HealthCheckInItem {
	item := func(errType, entityType string) HealthCheckInItem {
		return HealthCheckInItem{
			TeamID: g.TeamID, GoalID: g.ID, GoalTitle: g.Title,
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
		case domain.KRKindNumerical:
			if kr.Numerical != nil && len(kr.Numerical.Checkpoints) == 0 && kr.Numerical.TargetValue == kr.Numerical.StartValue {
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

func emptyCategories(cfg HealthCheckInConfig) map[string]*HealthCheckInCategory {
	names := []string{"stale", "no_goals", "awaiting_validation", "formation_errors", "lagging"}
	cats := make(map[string]*HealthCheckInCategory, len(names))
	for _, n := range names {
		cats[n] = &HealthCheckInCategory{InCounter: cfg.InCounter[n], Items: []HealthCheckInItem{}}
	}
	return cats
}

// GetHealthCheckIn computes the health check-in for the given user and period.
// Uses cached period data; loads from DB on first call or after TTL.
func (s *Service) GetHealthCheckIn(ctx context.Context, userUDID string, isAdmin bool, periodID int64, cfg HealthCheckInConfig) (*HealthCheckInResult, error) {
	if s.hcCache == nil {
		return &HealthCheckInResult{HasScope: false}, nil
	}
	data, err := s.hcCache.Get(ctx, periodID)
	if err != nil {
		return nil, err
	}

	var scopeIDs []int64
	if isAdmin {
		scopeIDs = make([]int64, 0, len(data.Teams))
		for _, t := range data.Teams {
			scopeIDs = append(scopeIDs, t.ID)
		}
	} else {
		scopeIDs = computeScope(data.Teams, data.GoalsByTeam, userUDID)
		if scopeIDs == nil {
			return &HealthCheckInResult{HasScope: false, PeriodID: periodID, Categories: emptyCategories(cfg)}, nil
		}
	}
	return computeCategories(data, scopeIDs, cfg, time.Now()), nil
}
