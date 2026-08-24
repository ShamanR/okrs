package service

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"okrs/internal/core/domain"
)

// HealthCheckInConfig controls thresholds and counter membership for each category.
// Loaded from system_settings key "health_checkin_config"; defaults apply when key is absent.
type HealthCheckInConfig struct {
	StaleDays       int `json:"stale_days"`
	BehindMargin    int `json:"behind_margin"`
	WeightTolerance int `json:"weight_tolerance"`
	CacheTTLMinutes int `json:"cache_ttl_minutes"`
	// GreenThreshold is the progress percent (1..100) at or above which a goal/team is
	// considered "in plan" (green) regardless of the forecast-based pace check.
	GreenThreshold int `json:"green_threshold"`
	// CommentDepth is how many hierarchy levels below the user's lead teams are scanned
	// for unresolved comments (0 = only the user's own teams, 1 = + direct children, …).
	CommentDepth int `json:"comment_depth"`
	// ResolvedCommentsLimit is how many of the user's most recently resolved comments (K)
	// are shown in the resolved sub-list.
	ResolvedCommentsLimit int             `json:"resolved_comments_limit"`
	InCounter             map[string]bool `json:"in_counter"`
}

var defaultHealthCheckInConfig = HealthCheckInConfig{
	StaleDays:             7,
	BehindMargin:          10,
	WeightTolerance:       0,
	CacheTTLMinutes:       5,
	GreenThreshold:        80,
	CommentDepth:          1,
	ResolvedCommentsLimit: 5,
	InCounter: map[string]bool{
		"stale":               true,
		"no_goals":            true,
		"awaiting_validation": true,
		"formation_errors":    true,
		"lagging":             false,
		"comments":            false,
	},
}

// SettingsReader loads per-tenant settings; *service.SettingsService satisfies this.
type SettingsReader interface {
	GetTenant(ctx context.Context, scope domain.TenantScope, key string) (json.RawMessage, error)
}

// LoadHealthCheckInConfig reads config from the tenant's settings, falling back to defaults.
func LoadHealthCheckInConfig(ctx context.Context, scope domain.TenantScope, sr SettingsReader) (HealthCheckInConfig, error) {
	raw, err := sr.GetTenant(ctx, scope, "health_checkin_config")
	if err != nil || raw == nil {
		return defaultHealthCheckInConfig, err
	}
	cfg := defaultHealthCheckInConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return defaultHealthCheckInConfig, nil
	}
	if cfg.GreenThreshold < 1 || cfg.GreenThreshold > 100 {
		cfg.GreenThreshold = defaultHealthCheckInConfig.GreenThreshold
	}
	if cfg.CommentDepth < 0 {
		cfg.CommentDepth = 0
	}
	if cfg.ResolvedCommentsLimit < 1 {
		cfg.ResolvedCommentsLimit = defaultHealthCheckInConfig.ResolvedCommentsLimit
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

// HealthCheckInCommentItem is one entry in the comments category (unresolved or resolved sub-list).
type HealthCheckInCommentItem struct {
	TeamID         int64      `json:"team_id"`
	TeamName       string     `json:"team_name"`
	TeamPath       []string   `json:"team_path"`
	GoalID         int64      `json:"goal_id"`
	GoalTitle      string     `json:"goal_title"`
	CommentID      int64      `json:"comment_id"`
	Text           string     `json:"text"`
	AuthorName     string     `json:"author_name,omitempty"`
	CreatedAt      time.Time  `json:"created_at,omitempty"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
	ResolvedByName string     `json:"resolved_by_name,omitempty"`
}

type HealthCheckInCategory struct {
	InCounter  bool                       `json:"in_counter"`
	Count      int                        `json:"count"`
	Items      []HealthCheckInItem        `json:"items"`
	Unresolved []HealthCheckInCommentItem `json:"unresolved,omitempty"`
	Resolved   []HealthCheckInCommentItem `json:"resolved,omitempty"`
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

// computeCommentScope returns the set of team IDs whose goals' unresolved comments are
// surfaced to the user: teams the user leads plus their descendants down to `depth` levels
// (depth 0 = only the lead teams themselves), plus teams where the user owns a goal (depth 0).
func computeCommentScope(teams []domain.Team, goalsByTeam map[int64][]domain.Goal, userUDID string, depth int) map[int64]struct{} {
	scope := make(map[int64]struct{})
	if userUDID == "" {
		return scope
	}

	childrenMap := make(map[int64][]int64, len(teams))
	for _, t := range teams {
		if t.ParentID != nil {
			childrenMap[*t.ParentID] = append(childrenMap[*t.ParentID], t.ID)
		}
	}

	// BFS from each lead team, limited to `depth` levels down.
	var frontier []int64
	for _, t := range teams {
		if t.DeletedAt == nil && t.LeadUDID != nil && *t.LeadUDID == userUDID {
			if _, seen := scope[t.ID]; !seen {
				scope[t.ID] = struct{}{}
				frontier = append(frontier, t.ID)
			}
		}
	}
	for level := 0; level < depth && len(frontier) > 0; level++ {
		var next []int64
		for _, id := range frontier {
			for _, childID := range childrenMap[id] {
				if _, seen := scope[childID]; !seen {
					scope[childID] = struct{}{}
					next = append(next, childID)
				}
			}
		}
		frontier = next
	}

	// Owner teams (depth 0, no descent) — matches existing owner-scope semantics.
	for teamID, goals := range goalsByTeam {
		for _, g := range goals {
			for _, uid := range g.OwnerUDIDs {
				if uid == userUDID {
					scope[teamID] = struct{}{}
					break
				}
			}
		}
	}
	return scope
}

// ── Category computation ──────────────────────────────────────────────────────

func computeCategories(data *PeriodData, scopeIDs []int64, userUDID string, cfg HealthCheckInConfig, now time.Time) *HealthCheckInResult {
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
		"comments":            {InCounter: cfg.InCounter["comments"], Items: []HealthCheckInItem{}},
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
			// Точка отсчёта «дней без обновления»: последнее обновление прогресса, а если
			// его не было — начало периода. От неё отмеряется StaleDays. Для ещё не
			// начавшегося периода daysSince <= 0, поэтому цель не попадает в stale.
			ref := lastProgress
			if ref == nil {
				ref = &data.Period.StartDate
			}
			daysSince := int(now.Sub(*ref).Hours() / 24)
			isStale := len(g.KeyResults) > 0 && daysSince > cfg.StaleDays

			// "N дней без обновления" is an execution-phase signal: it applies only
			// while the team is in_progress ("в работе"). Drafts, goals awaiting
			// validation, closed periods, and teams without a status row yet are not
			// being actively executed, so the warning is not meaningful for them.
			trackStale := status == domain.TeamPeriodStatusInProgress

			if isStale && trackStale {
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

	// ── Comments category ─────────────────────────────────────────────
	// Комментарий привязан к цели по comment_id, а не к команде. Расшаренная цель
	// присутствует в data.GoalsByTeam под каждой командой, куда она расшарена (у копии
	// g.TeamID = видимая команда), поэтому один и тот же комментарий встречается несколько
	// раз. Дедупим по comment_id, чтобы не задваивать счётчик и список.
	commentScope := computeCommentScope(data.Teams, data.GoalsByTeam, userUDID, cfg.CommentDepth)
	commentsCat := cats["comments"]
	seenComment := make(map[int64]struct{})

	// Unresolved: open comments on goals visible to teams in commentScope.
	for teamID := range commentScope {
		team, ok := teamsByID[teamID]
		if !ok {
			continue
		}
		path := buildTeamPath(teamID, teamsByID)
		for _, g := range data.GoalsByTeam[teamID] {
			for _, c := range g.Comments {
				if c.ResolvedAt != nil {
					continue
				}
				if _, dup := seenComment[c.ID]; dup {
					continue
				}
				seenComment[c.ID] = struct{}{}
				commentsCat.Unresolved = append(commentsCat.Unresolved, HealthCheckInCommentItem{
					TeamID: teamID, TeamName: team.Name, TeamPath: path,
					GoalID: g.ID, GoalTitle: g.Title,
					CommentID: c.ID, Text: c.Text, AuthorName: c.AuthorName, CreatedAt: c.CreatedAt,
				})
			}
		}
	}

	// Resolved-mine: my comments resolved by someone else, across the whole period, newest K.
	if userUDID != "" {
		var mine []HealthCheckInCommentItem
		seenResolved := make(map[int64]struct{})
		for teamID, goals := range data.GoalsByTeam {
			team, ok := teamsByID[teamID]
			if !ok {
				continue
			}
			path := buildTeamPath(teamID, teamsByID)
			for _, g := range goals {
				for _, c := range g.Comments {
					if c.ResolvedAt == nil || c.AuthorUDID != userUDID || c.ResolvedByUDID == userUDID {
						continue
					}
					if _, dup := seenResolved[c.ID]; dup {
						continue
					}
					seenResolved[c.ID] = struct{}{}
					rc := c.ResolvedAt
					mine = append(mine, HealthCheckInCommentItem{
						TeamID: teamID, TeamName: team.Name, TeamPath: path,
						GoalID: g.ID, GoalTitle: g.Title,
						CommentID: c.ID, Text: c.Text,
						ResolvedAt: rc, ResolvedByName: c.ResolvedByName,
					})
				}
			}
		}
		sort.Slice(mine, func(i, j int) bool {
			return mine[i].ResolvedAt.After(*mine[j].ResolvedAt)
		})
		if len(mine) > cfg.ResolvedCommentsLimit {
			mine = mine[:cfg.ResolvedCommentsLimit]
		}
		commentsCat.Resolved = mine
	}

	total := 0
	for name, cat := range cats {
		if name == "comments" {
			cat.Count = len(cat.Unresolved)
		} else {
			cat.Count = len(cat.Items)
		}
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
	names := []string{"stale", "no_goals", "awaiting_validation", "formation_errors", "lagging", "comments"}
	cats := make(map[string]*HealthCheckInCategory, len(names))
	for _, n := range names {
		cats[n] = &HealthCheckInCategory{InCounter: cfg.InCounter[n], Items: []HealthCheckInItem{}}
	}
	return cats
}

// GetHealthCheckIn computes the health check-in for the given user and period.
// Uses cached period data; loads from DB on first call or after TTL.
func (s *Service) GetHealthCheckIn(ctx context.Context, scope domain.TenantScope, userUDID string, isAdmin bool, periodID int64, cfg HealthCheckInConfig) (*HealthCheckInResult, error) {
	if s.hcCache == nil {
		return &HealthCheckInResult{HasScope: false}, nil
	}
	data, err := s.hcCache.Get(ctx, scope, periodID)
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
	return computeCategories(data, scopeIDs, userUDID, cfg, time.Now()), nil
}
