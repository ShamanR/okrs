// Package period holds the period-level scenarios: the cross-team overview the admin
// panel renders, bulk activate/close of team boards, and the dated progress snapshots
// that back the period chart. All of them span several entities.
//
// Every read here is batched across teams on purpose — one query per collection.
package period

import (
	"context"
	"log/slog"
	"math"
	"sort"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/core/progress"
	activitysvc "okrs/internal/service/activity"
	goalsvc "okrs/internal/service/goal"
	hcsvc "okrs/internal/service/healthcheckin"
	periodsvc "okrs/internal/service/period"
	progresssnapsvc "okrs/internal/service/progresssnap"
	teamsvc "okrs/internal/service/team"
	teamstatussvc "okrs/internal/service/teamstatus"
)

// Deps are the entity services this usecase orchestrates.
type Deps struct {
	Periods  *periodsvc.Service
	Teams    *teamsvc.Service
	Goals    *goalsvc.Service
	Statuses *teamstatussvc.Service
	Snaps    *progresssnapsvc.Service
	Activity *activitysvc.Service
	HCCache  *hcsvc.Cache
	Logger   *slog.Logger
}

type UseCase struct {
	periods  *periodsvc.Service
	teams    *teamsvc.Service
	goals    *goalsvc.Service
	statuses *teamstatussvc.Service
	snaps    *progresssnapsvc.Service
	activity *activitysvc.Service
	hcCache  *hcsvc.Cache
	logger   *slog.Logger
}

func New(deps Deps) *UseCase {
	return &UseCase{periods: deps.Periods, teams: deps.Teams, goals: deps.Goals,
		statuses: deps.Statuses, snaps: deps.Snaps, activity: deps.Activity, hcCache: deps.HCCache, logger: deps.Logger}
}

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
	// ProgressTeams is the number of teams averaged into AvgProgress: teams with
	// goals whose status is not «черновик» (forming). Draft teams are excluded.
	ProgressTeams int `json:"progress_teams"`
}

// BalanceBucket is one category of a goal balance (its count and share of the scope).
type BalanceBucket struct {
	Key     string `json:"key"`
	Count   int    `json:"count"`
	Percent int    `json:"percent"`
}

// PeriodBalances groups the three goal balances shown as bar charts.
type PeriodBalances struct {
	DiscoveryDelivery []BalanceBucket `json:"discovery_delivery"`
	Focuses           []BalanceBucket `json:"focuses"`
	Priorities        []BalanceBucket `json:"priorities"`
	Health            []BalanceBucket `json:"health"`
}

// PeriodGoalItem is a slim goal row for balance drill-down (client-side filtering).
type PeriodGoalItem struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	TeamID    int64  `json:"team_id"`
	TeamName  string `json:"team_name"`
	WorkType  string `json:"work_type"`
	FocusType string `json:"focus_type"`
	Priority  string `json:"priority"`
	Progress  int    `json:"progress"`
}

// PeriodKRItem is a slim KR row for the health-status breakdown drill-down.
type PeriodKRItem struct {
	ID           int64  `json:"id"`
	Title        string `json:"title"`
	GoalTitle    string `json:"goal_title"`
	TeamName     string `json:"team_name"`
	HealthStatus string `json:"health_status"`
	Progress     int    `json:"progress"`
}

// PeriodOverview is the full response for the management modal.
type PeriodOverview struct {
	PeriodID int64                 `json:"period_id"`
	Summary  PeriodOverviewSummary `json:"summary"`
	Teams    []PeriodTeamSummary   `json:"teams"`
	Balances PeriodBalances        `json:"balances"`
	Goals    []PeriodGoalItem      `json:"goals"`
	KRs      []PeriodKRItem        `json:"krs"`
	Progress ProgressSeries        `json:"progress"`
}

// Fixed category orders so bars render consistently (zero categories included).
var (
	workTypeOrder = []string{"Delivery", "Discovery"}
	focusOrder    = []string{"PROFITABILITY", "STABILITY", "SPEED_EFFICIENCY", "TECH_INDEPENDENCE"}
	priorityOrder = []string{"P0", "P1", "P2", "P3"}
	krHealthOrder = []string{"not_started", "on_track", "at_risk", "done"}
)

func bucketsFor(order []string, counts map[string]int, total int) []BalanceBucket {
	out := make([]BalanceBucket, 0, len(order))
	for _, k := range order {
		c := counts[k]
		pct := 0
		if total > 0 {
			pct = int(math.Round(float64(c) / float64(total) * 100))
		}
		out = append(out, BalanceBucket{Key: k, Count: c, Percent: pct})
	}
	return out
}

// computeBalances tallies goals-in-scope by work type, focus and priority. Percent is
// the share of all scoped goals; every fixed category is present even when zero.
func computeBalances(goals []PeriodGoalItem) PeriodBalances {
	wt, ft, pr := map[string]int{}, map[string]int{}, map[string]int{}
	for _, g := range goals {
		wt[g.WorkType]++
		ft[g.FocusType]++
		pr[g.Priority]++
	}
	total := len(goals)
	return PeriodBalances{
		DiscoveryDelivery: bucketsFor(workTypeOrder, wt, total),
		Focuses:           bucketsFor(focusOrder, ft, total),
		Priorities:        bucketsFor(priorityOrder, pr, total),
	}
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
// When teamFilter is non-nil, only teams whose ID is in the set are counted
// (my-teams scope); nil counts every team (whole-organization scope).
func computePeriodOverview(data *hcsvc.PeriodData, weightTolerance int, teamFilter map[int64]bool) PeriodOverview {
	teamsByID := make(map[int64]domain.Team, len(data.Teams))
	for _, t := range data.Teams {
		if t.DeletedAt != nil {
			continue
		}
		if teamFilter != nil && !teamFilter[t.ID] {
			continue
		}
		teamsByID[t.ID] = t
	}

	byStatus := map[string]int{"in_progress": 0, "ready": 0, "forming": 0, "closed": 0, "no_goals": 0}
	out := make([]PeriodTeamSummary, 0, len(teamsByID))
	var progressSum, progressCount, weightErrors, teamsWithGoals int

	// Slim goal list for balance drill-down, deduped by goal ID (a shared goal appears
	// under several teams). Balances are derived from the same list.
	seenGoals := make(map[int64]bool)
	goalItems := make([]PeriodGoalItem, 0)
	// KR-level rows + health-status tally for the "Статусы KR" breakdown drill-down.
	krItems := make([]PeriodKRItem, 0)
	healthCounts := map[string]int{}

	for id, team := range teamsByID {
		// Copy goals so progress.ForGoal writes don't mutate shared cache data.
		src := data.GoalsByTeam[id]
		goals := make([]domain.Goal, len(src))
		copy(goals, src)

		status := data.Statuses[id]
		if status == "" {
			status = domain.TeamPeriodStatusNoGoals
		}

		// Serialize the bucket the summary counts this team under (not the raw
		// status), so a drill-down that filters teams by tile key matches the tile
		// count. Notably a team with a goal shared into it but no status row of its
		// own is bucketed as forming, not no_goals.
		bucket := "no_goals"
		if len(goals) > 0 {
			bucket = bucketStatusWithGoals(status)
		}

		row := PeriodTeamSummary{
			TeamID:     id,
			TeamName:   team.Name,
			TeamPath:   hcsvc.BuildTeamPath(id, teamsByID),
			Status:     bucket,
			GoalsCount: len(goals),
		}
		byStatus[bucket]++

		if len(goals) > 0 {
			teamsWithGoals++
			weightSum := 0
			for i := range goals {
				goals[i].Progress = progress.ForGoal(&goals[i])
				weightSum += goals[i].Weight
			}
			row.WeightSum = weightSum
			row.WeightError = hcsvc.Abs(weightSum-100) > weightTolerance
			if row.WeightError {
				weightErrors++
			}
			row.Progress = progress.PeriodProgress(goals)
			// Draft (черновик/forming) teams are excluded from the aggregate progress —
			// their goals are still being shaped. Their own row.Progress is kept.
			if bucket != "forming" {
				progressSum += row.Progress
				progressCount++
			}

			for i := range goals {
				if seenGoals[goals[i].ID] {
					continue
				}
				seenGoals[goals[i].ID] = true
				goalItems = append(goalItems, PeriodGoalItem{
					ID:        goals[i].ID,
					Title:     goals[i].Title,
					TeamID:    id,
					TeamName:  team.Name,
					WorkType:  string(goals[i].WorkType),
					FocusType: string(goals[i].FocusType),
					Priority:  string(goals[i].Priority),
					Progress:  goals[i].Progress,
				})
				for _, kr := range goals[i].KeyResults {
					hs := string(kr.HealthStatus)
					if hs == "" {
						hs = string(domain.KRHealthNotStarted)
					}
					healthCounts[hs]++
					krItems = append(krItems, PeriodKRItem{
						ID:           kr.ID,
						Title:        kr.Title,
						GoalTitle:    goals[i].Title,
						TeamName:     team.Name,
						HealthStatus: hs,
						Progress:     progress.ForKR(kr),
					})
				}
			}
		}
		out = append(out, row)
	}
	sort.Slice(goalItems, func(i, j int) bool { return goalItems[i].ID < goalItems[j].ID })
	sort.Slice(krItems, func(i, j int) bool { return krItems[i].ID < krItems[j].ID })

	balances := computeBalances(goalItems)
	balances.Health = bucketsFor(krHealthOrder, healthCounts, len(krItems))

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
			ProgressTeams:    progressCount,
		},
		Teams:    out,
		Balances: balances,
		Goals:    goalItems,
		KRs:      krItems,
	}
}

// PeriodOverview returns the full overview (summary + team composition) for one period.
func (s *UseCase) PeriodOverview(ctx context.Context, scope domain.TenantScope, periodID int64, weightTolerance int) (PeriodOverview, error) {
	if s.hcCache == nil {
		return PeriodOverview{PeriodID: periodID}, nil
	}
	data, err := s.hcCache.Get(ctx, scope, periodID)
	if err != nil {
		return PeriodOverview{}, err
	}
	return computePeriodOverview(data, weightTolerance, nil), nil
}

// PeriodOverviewScoped is PeriodOverview restricted to teamFilter (nil = whole org),
// enriched with the per-scope progress-over-time series.
func (s *UseCase) PeriodOverviewScoped(ctx context.Context, scope domain.TenantScope, periodID int64, weightTolerance int, teamFilter map[int64]bool) (PeriodOverview, error) {
	if s.hcCache == nil {
		return PeriodOverview{PeriodID: periodID}, nil
	}
	data, err := s.hcCache.Get(ctx, scope, periodID)
	if err != nil {
		return PeriodOverview{}, err
	}
	ov := computePeriodOverview(data, weightTolerance, teamFilter)
	if s.snaps != nil {
		rows, err := s.snaps.List(ctx, scope, periodID, keysOf(teamFilter))
		if err != nil {
			return PeriodOverview{}, err
		}
		today := time.Now().Format(dateLayout)
		ov.Progress = buildProgressSeries(rows, teamFilter, today, ov.Summary.AvgProgress, data.Period.StartDate, data.Period.EndDate)
	}
	return ov, nil
}

// keysOf returns the keys of a team-filter set (nil when the filter is nil = whole org).
func keysOf(m map[int64]bool) []int64 {
	if m == nil {
		return nil
	}
	out := make([]int64, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// PeriodStats returns lightweight per-period metrics for every period (no team lists).
func (s *UseCase) PeriodStats(ctx context.Context, scope domain.TenantScope, weightTolerance int) ([]PeriodStatsItem, error) {
	if s.hcCache == nil {
		return []PeriodStatsItem{}, nil
	}
	periods, err := s.periods.List(ctx, scope)
	if err != nil {
		return nil, err
	}
	items := make([]PeriodStatsItem, 0, len(periods))
	for _, p := range periods {
		data, err := s.hcCache.Get(ctx, scope, p.ID)
		if err != nil {
			return nil, err
		}
		ov := computePeriodOverview(data, weightTolerance, nil)
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
