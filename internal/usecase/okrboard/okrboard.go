// Package okrboard assembles the team OKR board — goals with their key results,
// shares and period status — and the summary rows the tracker sidebar renders.
// It orchestrates entity services and owns the read-model the HTTP layer maps to DTOs.
//
// Every multi-team read here is batched on purpose: one query per collection, never
// one per team. Turning any of these into a per-team loop reintroduces N+1.
package okrboard

import (
	"context"
	"sort"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/core/progress"
	goalsvc "okrs/internal/service/goal"
	goallinksvc "okrs/internal/service/goallink"
	goalsharesvc "okrs/internal/service/goalshare"
	periodsvc "okrs/internal/service/period"
	teamsvc "okrs/internal/service/team"
	teamstatussvc "okrs/internal/service/teamstatus"
	"okrs/internal/store/shares"
)

// Deps are the entity services this usecase orchestrates.
type Deps struct {
	Teams    *teamsvc.Service
	Goals    *goalsvc.Service
	Shares   *goalsharesvc.Service
	Statuses *teamstatussvc.Service
	Periods  *periodsvc.Service
	Links    *goallinksvc.Service
}

type UseCase struct {
	teams    *teamsvc.Service
	goals    *goalsvc.Service
	shares   *goalsharesvc.Service
	statuses *teamstatussvc.Service
	periods  *periodsvc.Service
	links    *goallinksvc.Service
}

func New(deps Deps) *UseCase {
	return &UseCase{teams: deps.Teams, goals: deps.Goals, shares: deps.Shares, statuses: deps.Statuses, periods: deps.Periods, links: deps.Links}
}

type TeamSummary struct {
	ID             int64
	Name           string
	Type           domain.TeamType
	Indent         int
	Status         domain.TeamPeriodStatus
	PeriodProgress int
	GoalsCount     int
	GoalsWeight    int
	Goals          []TeamGoalSummary
}
type TeamGoalSummary struct {
	ID         int64
	Title      string
	Weight     int
	Progress   int
	ShareTeams []TeamShareInfo
	Priority   string
	WorkType   domain.WorkType
}
type TeamShareInfo struct {
	ID     int64
	Name   string
	Type   domain.TeamType
	Weight int
}
type TeamChildSummary struct {
	Team              domain.Team
	Status            domain.TeamPeriodStatus
	HasGoals          bool
	Progress          int
	GoalsCount        int
	HighPriorityCount int
	LastUpdateAt      *time.Time
}
type TeamOverview struct {
	AverageProgress int
	TeamsWithGoals  int
	ChildrenSummary []TeamChildSummary
}
type TeamOKR struct {
	Team            domain.Team
	Period          domain.Period
	PeriodStatus    domain.TeamPeriodStatus
	StatusChangedAt *time.Time
	PeriodProgress  int
	GoalsCount      int
	GoalsWeight     int
	Goals           []GoalDetails
}
type GoalDetails struct {
	Goal       domain.Goal
	ShareTeams []TeamShareInfo
}
type teamSummaryBatch struct {
	teamIDsWithGoals map[int64]struct{}
	goalsByTeam      map[int64][]domain.Goal
	statuses         map[int64]domain.TeamPeriodStatus
	sharesByGoal     map[int64][]shares.GoalShare
}

func (s *UseCase) TeamsWithPeriodSummary(ctx context.Context, scope domain.TenantScope, periodID int64, orgID *int64) ([]TeamSummary, error) {
	allTeams, err := s.teams.ListAll(ctx, scope)
	if err != nil {
		return nil, err
	}
	return s.teamsWithPeriodSummaryFromTeams(ctx, scope, allTeams, periodID, orgID)
}
func (s *UseCase) teamsWithPeriodSummaryFromTeams(ctx context.Context, scope domain.TenantScope, allTeams []domain.Team, periodID int64, orgID *int64) ([]TeamSummary, error) {
	teamsByID, childrenMap, roots := teamsvc.BuildHierarchy(allTeams)

	allTeamIDs := make([]int64, len(allTeams))
	for i, t := range allTeams {
		allTeamIDs[i] = t.ID
	}

	teamIDsWithGoals, err := s.teams.ListTeamIDsWithGoalsInPeriod(ctx, scope, periodID)
	if err != nil {
		return nil, err
	}
	goalsByTeam, err := s.goals.ListByTeamsPeriod(ctx, scope, periodID, allTeamIDs)
	if err != nil {
		return nil, err
	}
	statuses, err := s.statuses.List(ctx, scope, periodID, allTeamIDs)
	if err != nil {
		return nil, err
	}

	allGoalIDs := make([]int64, 0)
	for _, gList := range goalsByTeam {
		for _, g := range gList {
			allGoalIDs = append(allGoalIDs, g.ID)
		}
	}
	sharesByGoal, err := s.shares.ListByGoalIDs(ctx, scope, allGoalIDs)
	if err != nil {
		return nil, err
	}

	batch := &teamSummaryBatch{
		teamIDsWithGoals: teamIDsWithGoals,
		goalsByTeam:      goalsByTeam,
		statuses:         statuses,
		sharesByGoal:     sharesByGoal,
	}

	filteredRoots := roots
	if orgID != nil {
		if team, ok := teamsByID[*orgID]; ok {
			filteredRoots = []domain.Team{team}
		}
	}
	rows := make([]TeamSummary, 0, len(allTeams))
	for _, team := range filteredRoots {
		s.appendTeamSummaryFromBatch(&rows, team, 0, childrenMap, teamsByID, batch)
	}
	return rows, nil
}
func (s *UseCase) appendTeamSummaryFromBatch(rows *[]TeamSummary, team domain.Team, level int, childrenMap map[int64][]domain.Team, teamsByID map[int64]domain.Team, batch *teamSummaryBatch) {
	_, hasGoals := batch.teamIDsWithGoals[team.ID]
	visible := team.DeletedAt == nil || hasGoals

	if visible {
		goalsList := batch.goalsByTeam[team.ID]

		status := domain.TeamPeriodStatusNoGoals
		if s, ok := batch.statuses[team.ID]; ok {
			status = s
		}

		goalRows := make([]TeamGoalSummary, 0, len(goalsList))
		goalsWeight := 0
		for i := range goalsList {
			goalsList[i].Progress = progress.ForGoal(&goalsList[i])
			goalsWeight += goalsList[i].Weight
			shareTeams := buildShareInfosFromBatch(goalsList[i], batch.sharesByGoal[goalsList[i].ID], teamsByID)
			goalRows = append(goalRows, TeamGoalSummary{
				ID:         goalsList[i].ID,
				Title:      goalsList[i].Title,
				Weight:     goalsList[i].Weight,
				Progress:   goalsList[i].Progress,
				ShareTeams: shareTeams,
				Priority:   string(goalsList[i].Priority),
				WorkType:   goalsList[i].WorkType,
			})
		}

		*rows = append(*rows, TeamSummary{
			ID:             team.ID,
			Name:           team.Name,
			Type:           team.Type,
			Indent:         level * 24,
			Status:         status,
			PeriodProgress: progress.PeriodProgress(goalsList),
			GoalsCount:     len(goalsList),
			GoalsWeight:    goalsWeight,
			Goals:          goalRows,
		})
	}

	nextLevel := level
	if visible {
		nextLevel = level + 1
	}
	children := childrenMap[team.ID]
	sort.Slice(children, func(i, j int) bool { return children[i].Name < children[j].Name })
	for _, child := range children {
		s.appendTeamSummaryFromBatch(rows, child, nextLevel, childrenMap, teamsByID, batch)
	}
}
func (s *UseCase) TeamOKRFor(ctx context.Context, scope domain.TenantScope, teamID, periodID int64, period domain.Period) (TeamOKR, error) {
	team, err := s.teams.Get(ctx, scope, teamID)
	if err != nil {
		return TeamOKR{}, err
	}
	visible, err := s.teams.VisibleInPeriod(ctx, scope, team, periodID)
	if err != nil {
		return TeamOKR{}, err
	}
	if !visible {
		return TeamOKR{}, domain.ErrTeamNotVisibleInPeriod
	}
	goalsList, err := s.goals.ListByTeamPeriod(ctx, scope, teamID, periodID)
	if err != nil {
		return TeamOKR{}, err
	}

	goalIDs := make([]int64, len(goalsList))
	for i, g := range goalsList {
		goalIDs[i] = g.ID
	}
	sharesByGoal, err := s.shares.ListByGoalIDs(ctx, scope, goalIDs)
	if err != nil {
		return TeamOKR{}, err
	}
	allTeams, err := s.teams.ListAll(ctx, scope)
	if err != nil {
		return TeamOKR{}, err
	}
	teamsByID := make(map[int64]domain.Team, len(allTeams))
	for _, t := range allTeams {
		teamsByID[t.ID] = t
	}

	goalsWeight := 0
	goalDetails := make([]GoalDetails, 0, len(goalsList))
	for i := range goalsList {
		goalsList[i].Progress = progress.ForGoal(&goalsList[i])
		goalsWeight += goalsList[i].Weight
		shareTeams := buildShareInfosFromBatch(goalsList[i], sharesByGoal[goalsList[i].ID], teamsByID)
		goalDetails = append(goalDetails, GoalDetails{
			Goal:       goalsList[i],
			ShareTeams: shareTeams,
		})
	}

	periodProgress := progress.PeriodProgress(goalsList)
	status, statusChangedAt, err := s.statuses.GetWithTime(ctx, scope, teamID, periodID)
	if err != nil {
		return TeamOKR{}, err
	}
	return TeamOKR{
		Team:            team,
		Period:          period,
		PeriodStatus:    status,
		StatusChangedAt: statusChangedAt,
		PeriodProgress:  periodProgress,
		GoalsCount:      len(goalsList),
		GoalsWeight:     goalsWeight,
		Goals:           goalDetails,
	}, nil
}
func (s *UseCase) DirectChildrenSummary(ctx context.Context, scope domain.TenantScope, teamID, periodID int64) ([]TeamChildSummary, error) {
	allTeams, err := s.teams.ListAll(ctx, scope)
	if err != nil {
		return nil, err
	}
	hierarchy, err := s.teams.HierarchyFromTeams(ctx, scope, allTeams, &periodID)
	if err != nil {
		return nil, err
	}
	children := teamsvc.FindDirectChildren(teamID, hierarchy)
	if len(children) == 0 {
		return []TeamChildSummary{}, nil
	}
	return s.buildDirectChildrenSummary(ctx, scope, periodID, children, nil)
}
func (s *UseCase) buildDirectChildrenSummary(ctx context.Context, scope domain.TenantScope, periodID int64, children []teamsvc.Node, goalsByTeam map[int64][]domain.Goal) ([]TeamChildSummary, error) {
	childIDs := make([]int64, 0, len(children))
	for _, child := range children {
		childIDs = append(childIDs, child.Team.ID)
	}
	statuses, err := s.statuses.List(ctx, scope, periodID, childIDs)
	if err != nil {
		return nil, err
	}
	lastUpdates, err := s.goals.ListTeamLastUpdateInPeriod(ctx, scope, periodID, childIDs)
	if err != nil {
		return nil, err
	}
	result := make([]TeamChildSummary, 0, len(childIDs))
	for _, child := range children {
		item := TeamChildSummary{
			Team:   child.Team,
			Status: domain.TeamPeriodStatusNoGoals,
		}
		if status, ok := statuses[child.Team.ID]; ok {
			item.Status = status
		}
		var goalsList []domain.Goal
		if goalsByTeam != nil {
			goalsList = goalsByTeam[child.Team.ID]
		} else {
			goalsList, err = s.goals.ListByTeamPeriod(ctx, scope, child.Team.ID, periodID)
			if err != nil {
				return nil, err
			}
		}
		item.GoalsCount = len(goalsList)
		item.HasGoals = item.GoalsCount > 0
		if item.HasGoals {
			for i := range goalsList {
				goalsList[i].Progress = progress.ForGoal(&goalsList[i])
			}
			item.Progress = progress.PeriodProgress(goalsList)
			for _, g := range goalsList {
				if g.Priority == domain.PriorityP0 || g.Priority == domain.PriorityP1 {
					item.HighPriorityCount++
				}
			}
		}
		if updatedAt, ok := lastUpdates[child.Team.ID]; ok {
			value := updatedAt
			item.LastUpdateAt = &value
		}
		result = append(result, item)
	}
	return result, nil
}
func (s *UseCase) TeamOverviewFor(ctx context.Context, scope domain.TenantScope, teamID, periodID int64) (TeamOverview, error) {
	allTeams, err := s.teams.ListAll(ctx, scope)
	if err != nil {
		return TeamOverview{}, err
	}
	hierarchy, err := s.teams.HierarchyFromTeams(ctx, scope, allTeams, &periodID)
	if err != nil {
		return TeamOverview{}, err
	}
	children := teamsvc.FindDirectChildren(teamID, hierarchy)
	descendantIDs := teamsvc.CollectDescendantIDs(teamID, hierarchy)
	goalsByTeam, err := s.goals.ListByTeamsPeriod(ctx, scope, periodID, descendantIDs)
	if err != nil {
		return TeamOverview{}, err
	}
	summaryByID := make(map[int64]TeamSummary, len(descendantIDs))
	for _, id := range descendantIDs {
		goalsList := goalsByTeam[id]
		if len(goalsList) == 0 {
			continue
		}
		for i := range goalsList {
			goalsList[i].Progress = progress.ForGoal(&goalsList[i])
		}
		summaryByID[id] = TeamSummary{
			ID:             id,
			GoalsCount:     len(goalsList),
			PeriodProgress: progress.PeriodProgress(goalsList),
		}
	}
	childrenSummary := []TeamChildSummary{}
	if len(children) > 0 {
		childrenSummary, err = s.buildDirectChildrenSummary(ctx, scope, periodID, children, goalsByTeam)
		if err != nil {
			return TeamOverview{}, err
		}
	}

	totalProgress := 0
	teamsWithGoals := 0

	for _, id := range descendantIDs {
		summary, ok := summaryByID[id]
		if !ok || summary.GoalsCount == 0 {
			continue
		}
		teamsWithGoals++
		totalProgress += summary.PeriodProgress
	}

	avgProgress := 0
	if teamsWithGoals > 0 {
		avgProgress = totalProgress / teamsWithGoals
	}

	return TeamOverview{
		AverageProgress: avgProgress,
		TeamsWithGoals:  teamsWithGoals,
		ChildrenSummary: childrenSummary,
	}, nil
}
func buildShareInfosFromBatch(goal domain.Goal, shareList []shares.GoalShare, teamsByID map[int64]domain.Team) []TeamShareInfo {
	teamIDs := make(map[int64]struct{}, len(shareList)+1)
	teamIDs[goal.TeamID] = struct{}{}
	for _, share := range shareList {
		teamIDs[share.TeamID] = struct{}{}
	}
	teamInfos := make([]TeamShareInfo, 0, len(teamIDs))
	for teamID := range teamIDs {
		team, ok := teamsByID[teamID]
		if !ok {
			continue
		}
		weight := goal.Weight
		for _, share := range shareList {
			if share.TeamID == teamID {
				weight = share.Weight
				break
			}
		}
		teamInfos = append(teamInfos, TeamShareInfo{ID: team.ID, Name: team.Name, Type: team.Type, Weight: weight})
	}
	sort.Slice(teamInfos, func(i, j int) bool { return teamInfos[i].Name < teamInfos[j].Name })
	return teamInfos
}

// AttachGoalLinks fills Parents/Children (scope-filtered, with progress) on each board goal
// in place. Navigation-only; leaves progress/status untouched.
func (s *UseCase) AttachLinks(ctx context.Context, scope domain.TenantScope, details []GoalDetails, allowedTeamIDs []int64, adminAll bool) error {
	if len(details) == 0 {
		return nil
	}
	goalIDs := make([]int64, len(details))
	for i := range details {
		goalIDs[i] = details[i].Goal.ID
	}
	parents, children, err := s.links.ListForGoals(ctx, scope, goalIDs, allowedTeamIDs, adminAll)
	if err != nil {
		return err
	}
	for i := range details {
		details[i].Goal.Parents = parents[details[i].Goal.ID]
		details[i].Goal.Children = children[details[i].Goal.ID]
	}
	return nil
}
