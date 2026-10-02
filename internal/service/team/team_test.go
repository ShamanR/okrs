package team_test

import (
	"context"
	"testing"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/service/servicetest"
	"okrs/internal/service/team"
)

func TestDeleteTeamUsesSoftDeleteWhenTeamHasGoals(t *testing.T) {
	store := servicetest.NewStore()
	store.GoalsByTeam[10] = map[int64][]domain.Goal{1: {{ID: 1, TeamID: 10, PeriodID: 1}}}
	service := team.New(store)

	if err := service.Delete(context.Background(), domain.TenantScope{TenantID: 1}, 10); err != nil {
		t.Fatalf("delete team: %v", err)
	}
	if len(store.SoftDeleted) != 1 || store.SoftDeleted[0] != 10 {
		t.Fatalf("expected soft delete for team with goals")
	}
	if len(store.HardDeleted) != 0 {
		t.Fatalf("did not expect hard delete")
	}
}

func TestDeleteTeamUsesHardDeleteWhenTeamHasNoGoals(t *testing.T) {
	store := servicetest.NewStore()
	service := team.New(store)

	if err := service.Delete(context.Background(), domain.TenantScope{TenantID: 1}, 10); err != nil {
		t.Fatalf("delete team: %v", err)
	}
	if len(store.HardDeleted) != 1 || store.HardDeleted[0] != 10 {
		t.Fatalf("expected hard delete for team without goals")
	}
}

func TestHardDeleteTeamRejectsTeamsWithGoals(t *testing.T) {
	store := servicetest.NewStore()
	store.GoalsByTeam[10] = map[int64][]domain.Goal{1: {{ID: 1, TeamID: 10, PeriodID: 1}}}
	service := team.New(store)

	if err := service.HardDelete(context.Background(), domain.TenantScope{TenantID: 1}, 10); err != domain.ErrTeamHasGoals {
		t.Fatalf("expected domain.ErrTeamHasGoals, got %v", err)
	}
}

func TestGetHierarchyWithoutPeriodHidesDeletedTeams(t *testing.T) {
	deletedAt := time.Date(2025, 2, 1, 10, 0, 0, 0, time.UTC)
	store := servicetest.NewStore()
	store.Teams = []domain.Team{
		{ID: 1, Name: "Active", Type: domain.TeamTypeUnit},
		{ID: 2, Name: "Deleted", Type: domain.TeamTypeTeam, DeletedAt: &deletedAt},
	}
	store.GoalsByTeam[2] = map[int64][]domain.Goal{
		1: {{ID: 100, TeamID: 2, PeriodID: 1, Title: "Historical"}},
	}
	service := team.New(store)

	nodes, err := service.Hierarchy(context.Background(), domain.TenantScope{TenantID: 1}, nil)
	if err != nil {
		t.Fatalf("get hierarchy: %v", err)
	}
	ids := flattenNodeIDs(nodes)
	if _, ok := ids[1]; !ok {
		t.Fatalf("expected active team in hierarchy, got %+v", ids)
	}
	if _, ok := ids[2]; ok {
		t.Fatalf("expected deleted team to be hidden without period filter, got %+v", ids)
	}
}

func TestGetHierarchyWithPeriodIncludesDeletedTeamsWithGoals(t *testing.T) {
	deletedAt := time.Date(2025, 2, 1, 10, 0, 0, 0, time.UTC)
	store := servicetest.NewStore()
	store.Teams = []domain.Team{
		{ID: 1, Name: "Active", Type: domain.TeamTypeUnit},
		{ID: 2, Name: "Deleted with goals", Type: domain.TeamTypeTeam, DeletedAt: &deletedAt},
		{ID: 3, Name: "Deleted no goals", Type: domain.TeamTypeTeam, DeletedAt: &deletedAt},
	}
	store.GoalsByTeam[2] = map[int64][]domain.Goal{
		5: {{ID: 200, TeamID: 2, PeriodID: 5, Title: "Current"}},
	}
	service := team.New(store)
	periodID := int64(5)

	nodes, err := service.Hierarchy(context.Background(), domain.TenantScope{TenantID: 1}, &periodID)
	if err != nil {
		t.Fatalf("get hierarchy: %v", err)
	}
	ids := flattenNodeIDs(nodes)
	if _, ok := ids[1]; !ok {
		t.Fatalf("expected active team in hierarchy, got %+v", ids)
	}
	if _, ok := ids[2]; !ok {
		t.Fatalf("expected deleted team with period goals in hierarchy, got %+v", ids)
	}
	if _, ok := ids[3]; ok {
		t.Fatalf("expected deleted team without period goals to be hidden, got %+v", ids)
	}
}

func flattenNodeIDs(nodes []team.Node) map[int64]struct{} {
	ids := make(map[int64]struct{})
	var walk func(items []team.Node)
	walk = func(items []team.Node) {
		for _, node := range items {
			ids[node.Team.ID] = struct{}{}
			if len(node.Children) > 0 {
				walk(node.Children)
			}
		}
	}
	walk(nodes)
	return ids
}
