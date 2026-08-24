package service

import (
	"context"
	"testing"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/service/servicetest"
)

func newTestService(st *servicetest.Store, grants GrantsProvider) *Service {
	return New(Deps{Teams: st, Goals: st, Shares: st, Periods: st, KRs: st, Statuses: st, Users: st, Grants: grants})
}

// ShareGoal must reject targets that don't belong to the active tenant, so a caller can't attach
// a goal to a foreign/global team ID (cross-tenant reference).
func TestShareGoalRejectsForeignTeamTarget(t *testing.T) {
	store := servicetest.NewStore()
	store.Teams = []domain.Team{{ID: 1}, {ID: 2}} // only these belong to the tenant
	svc := newTestService(store, nil)
	scope := domain.TenantScope{TenantID: 1}

	err := svc.ShareGoal(context.Background(), scope, 10, []ShareTarget{{TeamID: 2, Weight: 50}, {TeamID: 99, Weight: 50}}, 1)
	if err != ErrShareTargetNotInTenant {
		t.Fatalf("foreign target (99) must be rejected with ErrShareTargetNotInTenant, got %v", err)
	}

	if err := svc.ShareGoal(context.Background(), scope, 10, []ShareTarget{{TeamID: 1, Weight: 100}}, 1); err != nil {
		t.Fatalf("all-valid targets must be accepted, got %v", err)
	}
}

// A team whose period is already in_progress or closed must not be newly added as a share target —
// its OKR set for the period is locked. Only newly added teams are guarded; a team already sharing
// the goal stays untouched even if its period has since advanced.
func TestShareGoalRejectsStartedPeriodTarget(t *testing.T) {
	store := servicetest.NewStore()
	store.Teams = []domain.Team{{ID: 1}, {ID: 2}, {ID: 3}}
	svc := newTestService(store, nil)
	scope := domain.TenantScope{TenantID: 1}
	// servicetest.Store.GetGoal returns a zero goal, so the goal's period is 0; status is keyed on period 0.
	store.Statuses[[2]int64{2, 0}] = domain.TeamPeriodStatusInProgress
	store.Statuses[[2]int64{3, 0}] = domain.TeamPeriodStatusClosed

	if err := svc.ShareGoal(context.Background(), scope, 10, []ShareTarget{{TeamID: 2, Weight: 50}}, 1); err != ErrCannotShareWithClosedPeriod {
		t.Fatalf("in_progress target must be rejected with ErrCannotShareWithClosedPeriod, got %v", err)
	}
	if err := svc.ShareGoal(context.Background(), scope, 10, []ShareTarget{{TeamID: 3, Weight: 50}}, 1); err != ErrCannotShareWithClosedPeriod {
		t.Fatalf("closed target must be rejected with ErrCannotShareWithClosedPeriod, got %v", err)
	}
	// A team with an open (forming) period is still addable.
	store.Statuses[[2]int64{1, 0}] = domain.TeamPeriodStatusForming
	if err := svc.ShareGoal(context.Background(), scope, 10, []ShareTarget{{TeamID: 1, Weight: 100}}, 1); err != nil {
		t.Fatalf("forming target must be accepted, got %v", err)
	}
}

// ListPeriodViews must filter out archived periods for the public caller (includeArchived=false)
// before building parent/depth views, so a public parent_id never points at a hidden period,
// while the admin caller (includeArchived=true) sees everything.
func TestUpdateKRProgressNumerical(t *testing.T) {
	store := servicetest.NewStore()
	store.KeyResults[1] = domain.KeyResult{ID: 1, Kind: domain.KRKindNumerical}
	service := newTestService(store, nil)

	if err := service.UpdateKRProgressNumerical(context.Background(), domain.TenantScope{TenantID: 1}, 1, 42, 1); err != nil {
		t.Fatalf("update numerical: %v", err)
	}
	if store.NumericalUpdates[1] != 42 {
		t.Fatalf("expected numerical update")
	}
}

// ArchivePeriod must only allow archiving a closed period, so an active/future period can't be
// hidden from the tree via archive.
func TestUpdateKRProgressBoolean(t *testing.T) {
	store := servicetest.NewStore()
	store.KeyResults[3] = domain.KeyResult{ID: 3, Kind: domain.KRKindBoolean}
	service := newTestService(store, nil)

	if err := service.UpdateKRProgressBoolean(context.Background(), domain.TenantScope{TenantID: 1}, 3, true, 1); err != nil {
		t.Fatalf("update boolean: %v", err)
	}
	if !store.BooleanUpdates[3] {
		t.Fatalf("expected boolean update")
	}
}

func TestUpdateKRProgressProject(t *testing.T) {
	store := servicetest.NewStore()
	store.KeyResults[4] = domain.KeyResult{ID: 4, Kind: domain.KRKindProject}
	store.ProjectStages[4] = []domain.KRProjectStage{{ID: 100, IsDone: false}, {ID: 101, IsDone: true}}
	service := newTestService(store, nil)

	updates := []ProjectStageUpdate{{ID: 100, IsDone: true}}
	if err := service.UpdateKRProgressProject(context.Background(), domain.TenantScope{TenantID: 1}, 4, updates, 1); err != nil {
		t.Fatalf("update project: %v", err)
	}
	if !store.StageUpdates[100] {
		t.Fatalf("expected stage update")
	}
}

func TestMoveGoal(t *testing.T) {
	store := servicetest.NewStore()
	service := newTestService(store, nil)

	if err := service.MoveGoal(context.Background(), domain.TenantScope{TenantID: 1}, 5, 10, -1); err != nil {
		t.Fatalf("move goal: %v", err)
	}
	if store.MovedGoals[10] != -1 {
		t.Fatalf("expected goal move direction")
	}
}

func TestMoveKeyResult(t *testing.T) {
	store := servicetest.NewStore()
	service := newTestService(store, nil)

	if err := service.MoveKeyResult(context.Background(), domain.TenantScope{TenantID: 1}, 20, 1); err != nil {
		t.Fatalf("move kr: %v", err)
	}
	if store.MovedKRs[20] != 1 {
		t.Fatalf("expected key result move direction")
	}
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
	service := newTestService(store, nil)

	rows, err := service.GetTeamsWithPeriodSummary(context.Background(), domain.TenantScope{TenantID: 1}, 1, nil)
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
	service := newTestService(store, nil)

	rows, err := service.GetTeamsWithPeriodSummary(context.Background(), domain.TenantScope{TenantID: 1}, 2, nil)
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
	service := newTestService(store, nil)

	rows, err := service.GetTeamsWithPeriodSummary(context.Background(), domain.TenantScope{TenantID: 1}, 2, nil)
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
	service := newTestService(store, nil)

	okr, err := service.GetTeamOKR(context.Background(), domain.TenantScope{TenantID: 1}, 2, 1, domain.Period{ID: 1, Name: "2024 Q4"})
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
	service := newTestService(store, nil)

	okr, err := service.GetTeamOKR(context.Background(), domain.TenantScope{TenantID: 1}, 1, 1, domain.Period{ID: 1, Name: "2024 Q4"})
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
	service := newTestService(store, nil)

	_, err := service.GetTeamOKR(context.Background(), domain.TenantScope{TenantID: 1}, 2, 2, domain.Period{ID: 2, Name: "2025 Q1"})
	if err != ErrTeamNotVisibleInPeriod {
		t.Fatalf("expected ErrTeamNotVisibleInPeriod, got %v", err)
	}
}

func TestGetTeamOKRAllowsDeletedTeamInCurrentPeriodWhenGoalsExist(t *testing.T) {
	deletedAt := time.Date(2025, 2, 1, 10, 0, 0, 0, time.UTC)
	store := servicetest.NewStore()
	store.CurrentPeriod = domain.Period{ID: 2}
	store.Teams = []domain.Team{{ID: 2, Name: "Deleted", Type: domain.TeamTypeTeam, DeletedAt: &deletedAt}}
	store.GoalsByTeam[2] = map[int64][]domain.Goal{2: {{ID: 200, TeamID: 2, PeriodID: 2, Title: "Current"}}}
	service := newTestService(store, nil)

	okr, err := service.GetTeamOKR(context.Background(), domain.TenantScope{TenantID: 1}, 2, 2, domain.Period{ID: 2, Name: "2025 Q1"})
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
	svc := newTestService(store, nil)
	overview, err := svc.GetTeamOverview(context.Background(), domain.TenantScope{TenantID: 1}, 1, 10)
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

func TestBuildDirectChildrenSummaryWithoutSummaryMap(t *testing.T) {
	store := servicetest.NewStore()
	store.Teams = []domain.Team{
		{ID: 1, Name: "Parent"},
		{ID: 2, Name: "Child", ParentID: ptr(1)},
	}
	store.Statuses[[2]int64{2, 11}] = domain.TeamPeriodStatusClosed
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	store.GoalsByTeam[2] = map[int64][]domain.Goal{
		11: {{
			ID:        300,
			TeamID:    2,
			PeriodID:  11,
			Title:     "Child goal",
			UpdatedAt: now,
		}},
	}
	svc := newTestService(store, nil)
	children := []TeamNode{{Team: domain.Team{ID: 2, Name: "Child", ParentID: ptr(1)}}}

	rows, err := svc.buildDirectChildrenSummary(context.Background(), domain.TenantScope{TenantID: 1}, 11, children, nil)
	if err != nil {
		t.Fatalf("build summary: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if !rows[0].HasGoals {
		t.Fatalf("expected has_goals=true")
	}
	if rows[0].Status != domain.TeamPeriodStatusClosed {
		t.Fatalf("expected status closed, got %s", rows[0].Status)
	}
	if rows[0].LastUpdateAt == nil {
		t.Fatalf("expected last_updated to be set")
	}
}

func ptr(id int64) *int64 { return &id }
