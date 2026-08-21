package service

import (
	"context"

	"okrs/internal/domain"
)

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
func (s *Service) GoalTree(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, adminAll bool, periodIDs []int64, callerUDID string) (GoalTreeData, error) {
	goalsList, err := s.goals.ListGoalsForPeriods(ctx, scope, periodIDs, allowedTeamIDs, adminAll)
	if err != nil {
		return GoalTreeData{}, err
	}

	inSet := make(map[int64]bool, len(goalsList))
	goalIDs := make([]int64, len(goalsList))
	for i := range goalsList {
		inSet[goalsList[i].ID] = true
		goalIDs[i] = goalsList[i].ID
	}

	parents, children, err := s.goalLinks.ListLinksForGoals(ctx, scope, goalIDs, allowedTeamIDs, adminAll)
	if err != nil {
		return GoalTreeData{}, err
	}

	nodes := make([]GoalTreeNode, len(goalsList))
	teamIDSet := make(map[int64]bool)
	for i := range goalsList {
		g := goalsList[i]
		g.Progress = CalculateGoalProgress(&g)
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

	periodViews, err := s.ListPeriodViews(ctx, scope, false)
	if err != nil {
		return GoalTreeData{}, err
	}

	allTeams, err := s.teams.ListAllTeams(ctx, scope)
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
