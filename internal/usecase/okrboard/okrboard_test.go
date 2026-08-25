package okrboard_test

import (
	"context"
	"testing"
	"time"

	"okrs/internal/core/domain"
	goalsvc "okrs/internal/service/goal"
	goalsharesvc "okrs/internal/service/goalshare"
	"okrs/internal/service/servicetest"
	teamsvc "okrs/internal/service/team"
	teamstatussvc "okrs/internal/service/teamstatus"
	"okrs/internal/usecase/okrboard"
)

// newBoard wires the usecase against the shared in-memory fake.
func newBoard(st *servicetest.Store) *okrboard.UseCase {
	return okrboard.New(okrboard.Deps{
		Teams:    teamsvc.New(st),
		Goals:    goalsvc.New(st),
		Shares:   goalsharesvc.New(st),
		Statuses: teamstatussvc.New(st),
	})
}

func TestGetTeamsWithPeriodSummaryKeepsActiveTeamsWithoutHistoricalGoalsVisible(t *testing.T) {
	deletedAt := time.Date(2025, 2, 1, 10, 0, 0, 0, time.UTC)
	store := servicetest.NewStore()
	store.CurrentPeriod = domain.Period{ID: 2}
	store.Teams = []domain.Team{
		{ID: 1, Name: "Active parent", Type: domain.TeamTypeUnit},
		{ID: 2, Name: "Deleted child", Type: domain.TeamTypeTeam, ParentID: ptr(1), DeletedAt: &deletedAt},
		{ID: 3, Name: "New active child", Type: domain.TeamTypeTeam, ParentID: ptr(1)},
	}
	store.GoalsByTeam[2] = map[int64][]domain.Goal{1: {{ID: 100, TeamID: 2, PeriodID: 1, Title: "Historic"}}}
	store.Statuses[[2]int64{2, 1}] = domain.TeamPeriodStatusClosed
	service := newBoard(store)

	rows, err := service.TeamsWithPeriodSummary(context.Background(), domain.TenantScope{TenantID: 1}, 1, nil)
	if err != nil {
		t.Fatalf("get team summaries: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected active teams plus deleted historical team, got %d", len(rows))
	}
	if rows[0].ID != 1 || rows[1].ID != 2 || rows[2].ID != 3 {
		t.Fatalf("expected active parent, deleted child with goals, and active child, got %+v", rows)
	}
	if rows[1].Indent != 24 || rows[2].Indent != 24 {
		t.Fatalf("expected children to remain nested under active parent, got %+v", rows)
	}
}

func TestGetTeamsWithPeriodSummaryShowsOnlyActiveTeamsInCurrentPeriod(t *testing.T) {
	deletedAt := time.Date(2025, 2, 1, 10, 0, 0, 0, time.UTC)
	store := servicetest.NewStore()
	store.CurrentPeriod = domain.Period{ID: 2}
	store.Teams = []domain.Team{
		{ID: 1, Name: "Active", Type: domain.TeamTypeUnit},
		{ID: 2, Name: "Deleted", Type: domain.TeamTypeTeam, DeletedAt: &deletedAt},
	}
	service := newBoard(store)

	rows, err := service.TeamsWithPeriodSummary(context.Background(), domain.TenantScope{TenantID: 1}, 2, nil)
	if err != nil {
		t.Fatalf("get team summaries: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != 1 {
		t.Fatalf("expected only active team in current period, got %+v", rows)
	}
}

func TestGetTeamsWithPeriodSummaryKeepsDeletedTeamsWithCurrentGoalsVisible(t *testing.T) {
	deletedAt := time.Date(2025, 2, 1, 10, 0, 0, 0, time.UTC)
	store := servicetest.NewStore()
	store.CurrentPeriod = domain.Period{ID: 2}
	store.Teams = []domain.Team{
		{ID: 1, Name: "Active", Type: domain.TeamTypeUnit},
		{ID: 2, Name: "Deleted", Type: domain.TeamTypeTeam, DeletedAt: &deletedAt},
	}
	store.GoalsByTeam[2] = map[int64][]domain.Goal{2: {{ID: 200, TeamID: 2, PeriodID: 2, Title: "Current"}}}
	service := newBoard(store)

	rows, err := service.TeamsWithPeriodSummary(context.Background(), domain.TenantScope{TenantID: 1}, 2, nil)
	if err != nil {
		t.Fatalf("get team summaries: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected active team and deleted team with current goals, got %+v", rows)
	}
	if rows[1].ID != 2 {
		t.Fatalf("expected deleted team with current goals to remain visible, got %+v", rows)
	}
}

func TestGetTeamOKRAllowsDeletedTeamInHistoricalPeriodWithGoals(t *testing.T) {
	deletedAt := time.Date(2025, 2, 1, 10, 0, 0, 0, time.UTC)
	store := servicetest.NewStore()
	store.CurrentPeriod = domain.Period{ID: 2}
	store.Teams = []domain.Team{{ID: 2, Name: "Deleted", Type: domain.TeamTypeTeam, DeletedAt: &deletedAt}}
	store.GoalsByTeam[2] = map[int64][]domain.Goal{1: {{ID: 100, TeamID: 2, PeriodID: 1, Title: "Historic"}}}
	service := newBoard(store)

	okr, err := service.TeamOKRFor(context.Background(), domain.TenantScope{TenantID: 1}, 2, 1, domain.Period{ID: 1, Name: "2024 Q4"})
	if err != nil {
		t.Fatalf("get team okr: %v", err)
	}
	if okr.Team.ID != 2 || len(okr.Goals) != 1 {
		t.Fatalf("expected historical deleted team okr to load")
	}
}

func TestGetTeamOKRAllowsActiveTeamWithoutHistoricalGoals(t *testing.T) {
	store := servicetest.NewStore()
	store.CurrentPeriod = domain.Period{ID: 2}
	store.Teams = []domain.Team{{ID: 1, Name: "Active", Type: domain.TeamTypeTeam}}
	service := newBoard(store)

	okr, err := service.TeamOKRFor(context.Background(), domain.TenantScope{TenantID: 1}, 1, 1, domain.Period{ID: 1, Name: "2024 Q4"})
	if err != nil {
		t.Fatalf("expected active team without historical goals to stay visible, got %v", err)
	}
	if okr.Team.ID != 1 || len(okr.Goals) != 0 {
		t.Fatalf("expected empty okr page for active team without goals")
	}
}

func TestGetTeamOKRRejectsDeletedTeamInCurrentPeriod(t *testing.T) {
	deletedAt := time.Date(2025, 2, 1, 10, 0, 0, 0, time.UTC)
	store := servicetest.NewStore()
	store.CurrentPeriod = domain.Period{ID: 2}
	store.Teams = []domain.Team{{ID: 2, Name: "Deleted", Type: domain.TeamTypeTeam, DeletedAt: &deletedAt}}
	service := newBoard(store)

	_, err := service.TeamOKRFor(context.Background(), domain.TenantScope{TenantID: 1}, 2, 2, domain.Period{ID: 2, Name: "2025 Q1"})
	if err != domain.ErrTeamNotVisibleInPeriod {
		t.Fatalf("expected domain.ErrTeamNotVisibleInPeriod, got %v", err)
	}
}

func TestGetTeamOKRAllowsDeletedTeamInCurrentPeriodWhenGoalsExist(t *testing.T) {
	deletedAt := time.Date(2025, 2, 1, 10, 0, 0, 0, time.UTC)
	store := servicetest.NewStore()
	store.CurrentPeriod = domain.Period{ID: 2}
	store.Teams = []domain.Team{{ID: 2, Name: "Deleted", Type: domain.TeamTypeTeam, DeletedAt: &deletedAt}}
	store.GoalsByTeam[2] = map[int64][]domain.Goal{2: {{ID: 200, TeamID: 2, PeriodID: 2, Title: "Current"}}}
	service := newBoard(store)

	okr, err := service.TeamOKRFor(context.Background(), domain.TenantScope{TenantID: 1}, 2, 2, domain.Period{ID: 2, Name: "2025 Q1"})
	if err != nil {
		t.Fatalf("expected deleted team with current goals to be visible, got %v", err)
	}
	if okr.Team.ID != 2 || len(okr.Goals) != 1 {
		t.Fatalf("expected current deleted team okr to load")
	}
}

func TestGetTeamOverview(t *testing.T) {
	store := servicetest.NewStore()
	store.Teams = []domain.Team{
		{ID: 1, Name: "Parent", Type: domain.TeamTypeUnit},
		{ID: 2, Name: "Child", Type: domain.TeamTypeTeam, ParentID: ptr(1)},
	}
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	store.GoalsByTeam[2] = map[int64][]domain.Goal{
		10: {{
			ID:        200,
			TeamID:    2,
			PeriodID:  10,
			Title:     "Ship feature",
			Priority:  domain.PriorityP1,
			WorkType:  domain.WorkTypeDelivery,
			UpdatedAt: now,
		}},
	}
	store.Statuses[[2]int64{2, 10}] = domain.TeamPeriodStatusInProgress
	svc := newBoard(store)
	overview, err := svc.TeamOverviewFor(context.Background(), domain.TenantScope{TenantID: 1}, 1, 10)
	if err != nil {
		t.Fatalf("get team overview: %v", err)
	}
	if overview.TeamsWithGoals != 1 {
		t.Fatalf("expected teams_with_goals=1, got %d", overview.TeamsWithGoals)
	}
	if overview.AverageProgress != 0 {
		t.Fatalf("expected average progress=0 for zero-progress goal, got %d", overview.AverageProgress)
	}
	if len(overview.ChildrenSummary) != 1 {
		t.Fatalf("expected one direct child summary row, got %d", len(overview.ChildrenSummary))
	}
	if !overview.ChildrenSummary[0].HasGoals {
		t.Fatalf("expected child has_goals=true")
	}
}

func ptr(id int64) *int64 { return &id }
