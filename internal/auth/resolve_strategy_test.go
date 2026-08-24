package auth

import (
	"context"
	"testing"

	"okrs/internal/core/domain"
)

type fakeStrategy struct {
	tenant   *domain.Tenant
	role     domain.Role
	resolved bool
}

func (f fakeStrategy) Resolve(context.Context, *domain.User, *domain.AuthSession) (*domain.Tenant, domain.Role, bool, error) {
	return f.tenant, f.role, f.resolved, nil
}

func TestTenantResolverFirstResolvedWins(t *testing.T) {
	first := fakeStrategy{resolved: false}
	second := fakeStrategy{tenant: &domain.Tenant{ID: 9}, role: domain.RoleAdmin, resolved: true}
	r := NewTenantResolver(first, second)

	tn, role, err := r.Resolve(context.Background(), &domain.User{ID: 1}, nil)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if tn.ID != 9 || role != domain.RoleAdmin {
		t.Fatalf("expected the second strategy to win, got tenant=%v role=%v", tn, role)
	}
}

func TestTenantResolverNoneResolvedIsErrNoMembership(t *testing.T) {
	r := NewTenantResolver(fakeStrategy{resolved: false})
	if _, _, err := r.Resolve(context.Background(), &domain.User{ID: 1}, nil); err != ErrNoMembership {
		t.Fatalf("expected ErrNoMembership, got %v", err)
	}
}

func TestResolveStrategyRegistryHasSession(t *testing.T) {
	f, ok := ResolveStrategyFactoryByName("session")
	if !ok || f(ResolveDeps{}) == nil {
		t.Fatal("session strategy must be registered by default")
	}
}
