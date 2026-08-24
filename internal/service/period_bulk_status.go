package service

import (
	"context"
	"sort"

	"okrs/internal/core/domain"
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
// in the period and is not already in target. When teamFilter is non-nil, only teams in
// the set are considered (my-teams scope); nil covers the whole tenant (org scope).
// Writes one op-log entry per affected team and invalidates the period cache. Loads fresh
// data (not cached) to decide the set.
func (s *Service) BulkSetTeamPeriodStatus(ctx context.Context, scope domain.TenantScope, periodID int64, target domain.TeamPeriodStatus, actorUserID int64, teamFilter map[int64]bool) (BulkStatusResult, error) {
	loaded, err := s.teams.ListAllTeams(ctx, scope)
	if err != nil {
		return BulkStatusResult{}, err
	}
	allTeams := make([]domain.Team, 0, len(loaded))
	teamIDs := make([]int64, 0, len(loaded))
	nameByID := make(map[int64]string, len(loaded))
	for _, t := range loaded {
		if t.DeletedAt != nil {
			continue
		}
		if teamFilter != nil && !teamFilter[t.ID] {
			continue
		}
		allTeams = append(allTeams, t)
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
