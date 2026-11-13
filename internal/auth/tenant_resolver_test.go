package auth

import (
	"context"
	"errors"
	"testing"

	"okrs/internal/core/domain"
)

type fakeTenants struct{ m map[int64]*domain.Tenant }

func (f fakeTenants) GetByID(_ context.Context, id int64) (*domain.Tenant, error) {
	if t, ok := f.m[id]; ok {
		return t, nil
	}
	return nil, errors.New("not found")
}

type fakeMembers struct{ m map[int64][]domain.Membership }

func (f fakeMembers) ListByUser(_ context.Context, uid int64) ([]domain.Membership, error) {
	return f.m[uid], nil
}

func headerStrategyFixture() (fakeTenants, fakeMembers) {
	tenants := fakeTenants{m: map[int64]*domain.Tenant{
		1: {ID: 1, Slug: "default", Status: domain.TenantActive},
		2: {ID: 2, Slug: "acme", Status: domain.TenantActive},
	}}
	members := fakeMembers{m: map[int64][]domain.Membership{
		10: {{UserID: 10, TenantID: 1, Role: domain.RoleUser}, {UserID: 10, TenantID: 2, Role: domain.RoleAdmin}},
	}}
	return tenants, members
}

func TestResolveHeaderTenantWins(t *testing.T) {
	tenants, members := headerStrategyFixture()
	r := NewTenantResolver(NewHeaderStrategy(tenants, members))
	ctx := WithHeaderTenantID(context.Background(), 2)

	tn, role, err := r.Resolve(ctx, &domain.User{ID: 10}, nil)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if tn.ID != 2 || role != domain.RoleAdmin {
		t.Fatalf("got tenant %d role %s, want 2/admin", tn.ID, role)
	}
}

func TestResolveForeignHeaderTenantNotResolved(t *testing.T) {
	tenants, members := headerStrategyFixture()
	r := NewTenantResolver(NewHeaderStrategy(tenants, members))
	ctx := WithHeaderTenantID(context.Background(), 77)

	if _, _, err := r.Resolve(ctx, &domain.User{ID: 10}, nil); !errors.Is(err, ErrNoMembership) {
		t.Fatalf("want ErrNoMembership, got %v", err)
	}
}

func TestResolveSingleMembershipWithoutHeader(t *testing.T) {
	tenants := fakeTenants{m: map[int64]*domain.Tenant{1: {ID: 1, Status: domain.TenantActive}}}
	members := fakeMembers{m: map[int64][]domain.Membership{10: {{UserID: 10, TenantID: 1, Role: domain.RoleUser}}}}
	r := NewTenantResolver(NewHeaderStrategy(tenants, members))

	tn, role, err := r.Resolve(context.Background(), &domain.User{ID: 10}, nil)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if tn.ID != 1 || role != domain.RoleUser {
		t.Fatalf("got %d/%s, want 1/user", tn.ID, role)
	}
}

func TestResolveSeveralMembershipsWithoutHeader(t *testing.T) {
	tenants, members := headerStrategyFixture()
	r := NewTenantResolver(NewHeaderStrategy(tenants, members))

	if _, _, err := r.Resolve(context.Background(), &domain.User{ID: 10}, nil); !errors.Is(err, ErrNoMembership) {
		t.Fatalf("want ErrNoMembership, got %v", err)
	}
}

func TestResolveNoMembership(t *testing.T) {
	r := NewTenantResolver(NewHeaderStrategy(fakeTenants{m: map[int64]*domain.Tenant{}}, fakeMembers{m: map[int64][]domain.Membership{}}))
	if _, _, err := r.Resolve(context.Background(), &domain.User{ID: 99}, nil); !errors.Is(err, ErrNoMembership) {
		t.Fatalf("want ErrNoMembership, got %v", err)
	}
}
