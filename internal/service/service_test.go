package service

import (
	"context"
	"testing"

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
