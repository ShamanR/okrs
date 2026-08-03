package service

import (
	"context"
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
			TeamPath:   buildTeamPath(id, teamsByID),
			Status:     bucket,
			GoalsCount: len(goals),
		}
		byStatus[bucket]++

		if len(goals) > 0 {
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
