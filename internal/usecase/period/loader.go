package period

import (
	"context"
	"time"

	"okrs/internal/core/domain"
	goalsvc "okrs/internal/service/goal"
	periodsvc "okrs/internal/service/period"
	teamsvc "okrs/internal/service/team"
	teamstatussvc "okrs/internal/service/teamstatus"
)

// LoaderDeps are the entity services the period loader reads through.
type LoaderDeps struct {
	Periods  *periodsvc.Service
	Teams    *teamsvc.Service
	Goals    *goalsvc.Service
	Statuses *teamstatussvc.Service
}

// NewPeriodLoader returns the loader PeriodCache calls on a miss: it assembles the period
// snapshot (teams, their goals with comments, and per-team status).
//
// Every read is batched across the whole tenant — one query for all teams' goals, one
// for all statuses, one for all comments. Turning any of these into a per-team or
// per-goal loop reintroduces N+1 on a hot, cached-but-cold-on-miss path.
func NewPeriodLoader(deps LoaderDeps) func(ctx context.Context, scope domain.TenantScope, periodID int64) (*PeriodData, error) {
	return func(ctx context.Context, scope domain.TenantScope, periodID int64) (*PeriodData, error) {
		period, err := deps.Periods.Get(ctx, scope, periodID)
		if err != nil {
			return nil, err
		}
		allTeams, err := deps.Teams.ListAll(ctx, scope)
		if err != nil {
			return nil, err
		}
		allTeamIDs := make([]int64, len(allTeams))
		for i, t := range allTeams {
			allTeamIDs[i] = t.ID
		}
		goalsByTeam, err := deps.Goals.ListByTeamsPeriod(ctx, scope, periodID, allTeamIDs)
		if err != nil {
			return nil, err
		}
		statuses, err := deps.Statuses.List(ctx, scope, periodID, allTeamIDs)
		if err != nil {
			return nil, err
		}
		goalIDSet := make(map[int64]struct{})
		for _, goals := range goalsByTeam {
			for _, g := range goals {
				goalIDSet[g.ID] = struct{}{}
			}
		}
		goalIDs := make([]int64, 0, len(goalIDSet))
		for id := range goalIDSet {
			goalIDs = append(goalIDs, id)
		}
		commentsByGoal, err := deps.Goals.ListCommentsByGoals(ctx, scope, goalIDs)
		if err != nil {
			return nil, err
		}
		for teamID, goals := range goalsByTeam {
			for i := range goals {
				goals[i].Comments = commentsByGoal[goals[i].ID]
			}
			goalsByTeam[teamID] = goals
		}
		return &PeriodData{
			PeriodID:    periodID,
			Period:      period,
			Teams:       allTeams,
			GoalsByTeam: goalsByTeam,
			Statuses:    statuses,
			CachedAt:    time.Now(),
		}, nil
	}
}
