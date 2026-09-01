package goal

// Тесты переехали из internal/service при выделении слоя usecase.

import (
	"context"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/service/servicetest"
)

// newTestService keeps the old call shape the migrated tests use; grants are unused
// by goal scenarios (scope-aware search moved to usecase/user).
func newTestService(st *servicetest.Store, _ any) *UseCase {
	return newFromRepos(rawDeps{Teams: st, Goals: st, Shares: st, Periods: st, Statuses: st, Events: &servicetest.FakeBus{}})
}

// ShareGoal must reject targets that don't belong to the active tenant, so a caller can't attach
// a goal to a foreign/global team ID (cross-tenant reference).
func TestShareGoalRejectsForeignTeamTarget(t *testing.T) {
	store := servicetest.NewStore()
	store.Teams = []domain.Team{{ID: 1}, {ID: 2}} // only these belong to the tenant
	svc := newTestService(store, nil)
	scope := domain.TenantScope{TenantID: 1}

	err := svc.Share(context.Background(), scope, 10, []ShareTarget{{TeamID: 2, Weight: 50}, {TeamID: 99, Weight: 50}}, 1)
	if err != domain.ErrShareTargetNotInTenant {
		t.Fatalf("foreign target (99) must be rejected with domain.ErrShareTargetNotInTenant, got %v", err)
	}

	if err := svc.Share(context.Background(), scope, 10, []ShareTarget{{TeamID: 1, Weight: 100}}, 1); err != nil {
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

	if err := svc.Share(context.Background(), scope, 10, []ShareTarget{{TeamID: 2, Weight: 50}}, 1); err != domain.ErrCannotShareWithClosedPeriod {
		t.Fatalf("in_progress target must be rejected with domain.ErrCannotShareWithClosedPeriod, got %v", err)
	}
	if err := svc.Share(context.Background(), scope, 10, []ShareTarget{{TeamID: 3, Weight: 50}}, 1); err != domain.ErrCannotShareWithClosedPeriod {
		t.Fatalf("closed target must be rejected with domain.ErrCannotShareWithClosedPeriod, got %v", err)
	}
	// A team with an open (forming) period is still addable.
	store.Statuses[[2]int64{1, 0}] = domain.TeamPeriodStatusForming
	if err := svc.Share(context.Background(), scope, 10, []ShareTarget{{TeamID: 1, Weight: 100}}, 1); err != nil {
		t.Fatalf("forming target must be accepted, got %v", err)
	}
}
