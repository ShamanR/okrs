// Package goaltree builds the cross-team goal graph: goals for the selected periods
// plus the parent/child links between them, grouped by owning team.
package goaltree

import (
	"context"

	"okrs/internal/core/domain"
	"okrs/internal/core/progress"
	goalsvc "okrs/internal/service/goal"
	goallinksvc "okrs/internal/service/goallink"
	periodsvc "okrs/internal/service/period"
	teamsvc "okrs/internal/service/team"
)

type Deps struct {
	Teams   *teamsvc.Service
	Periods *periodsvc.Service
	Goals   *goalsvc.Service
	Links   *goallinksvc.Service
}

type UseCase struct {
	teams   *teamsvc.Service
	periods *periodsvc.Service
	goals   *goalsvc.Service
	links   *goallinksvc.Service
}

func New(deps Deps) *UseCase {
	return &UseCase{teams: deps.Teams, periods: deps.Periods, goals: deps.Goals, links: deps.Links}
}

type GoalTreeTeam struct {
	Team    domain.Team
	LedByMe bool
}

type GoalTreeNode struct {
	Goal          domain.Goal
	ParentGoalIDs []int64
	ChildGoalIDs  []int64
}

type GoalTreeData struct {
	Periods []domain.PeriodView
	Teams   []GoalTreeTeam
	Nodes   []GoalTreeNode
}

// GoalTree собирает граф целей и связей в scope за один проход (без N+1):
// цели-владельцы по периодам, их прогресс из KR, видимые рёбра (обрезанные до
// загруженного набора), команды целей+рёбер и их предки (для дерева корней), периоды с depth.
func (s *UseCase) GoalTree(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, adminAll bool, periodIDs []int64, callerUDID string) (GoalTreeData, error) {
	goalsList, err := s.goals.ListForPeriods(ctx, scope, periodIDs, allowedTeamIDs, adminAll)
	if err != nil {
		return GoalTreeData{}, err
	}

	inSet := make(map[int64]bool, len(goalsList))
	goalIDs := make([]int64, len(goalsList))
	for i := range goalsList {
		inSet[goalsList[i].ID] = true
		goalIDs[i] = goalsList[i].ID
	}

	parents, children, err := s.links.ListForGoals(ctx, scope, goalIDs, allowedTeamIDs, adminAll)
	if err != nil {
		return GoalTreeData{}, err
	}

	nodes := make([]GoalTreeNode, len(goalsList))
	teamIDSet := make(map[int64]bool)
	for i := range goalsList {
		g := goalsList[i]
		g.Progress = progress.ForGoal(&g)
		teamIDSet[g.TeamID] = true
		var pIDs, cIDs []int64
		for _, ref := range parents[g.ID] {
			if inSet[ref.ID] { // ребро только если родитель в наборе
				pIDs = append(pIDs, ref.ID)
			}
		}
		for _, ref := range children[g.ID] {
			if inSet[ref.ID] {
				cIDs = append(cIDs, ref.ID)
			}
		}
		nodes[i] = GoalTreeNode{Goal: g, ParentGoalIDs: pIDs, ChildGoalIDs: cIDs}
	}

	periodViews, err := s.periods.ListViews(ctx, scope, false)
	if err != nil {
		return GoalTreeData{}, err
	}

	allTeams, err := s.teams.ListAll(ctx, scope)
	if err != nil {
		return GoalTreeData{}, err
	}
	teamByID := make(map[int64]domain.Team, len(allTeams))
	for _, t := range allTeams {
		teamByID[t.ID] = t
	}
	// Добавить предков команд-целей (для древовидного отступа в списке корней).
	for id := range teamIDSet {
		cur := teamByID[id]
		for cur.ParentID != nil {
			if teamIDSet[*cur.ParentID] {
				break
			}
			parent, ok := teamByID[*cur.ParentID]
			if !ok {
				break
			}
			teamIDSet[parent.ID] = true
			cur = parent
		}
	}
	treeTeams := make([]GoalTreeTeam, 0, len(teamIDSet))
	for _, t := range allTeams { // сохраняем стабильный порядок ListAllTeams
		if !teamIDSet[t.ID] {
			continue
		}
		ledByMe := callerUDID != "" && t.LeadUDID != nil && *t.LeadUDID == callerUDID
		treeTeams = append(treeTeams, GoalTreeTeam{Team: t, LedByMe: ledByMe})
	}

	return GoalTreeData{Periods: periodViews, Teams: treeTeams, Nodes: nodes}, nil
}
