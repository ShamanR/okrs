package auth

import (
	"context"
	"errors"
	"testing"

	"okrs/internal/domain"
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

func TestResolvePrefersSessionActiveTenant(t *testing.T) {
	tenants := fakeTenants{m: map[int64]*domain.Tenant{
		1: {ID: 1, Slug: "default", Status: domain.TenantActive},
		2: {ID: 2, Slug: "acme", Status: domain.TenantActive},
	}}
	members := fakeMembers{m: map[int64][]domain.Membership{
		10: {{UserID: 10, TenantID: 1, Role: domain.RoleUser}, {UserID: 10, TenantID: 2, Role: domain.RoleAdmin}},
	}}
	r := NewTenantResolver(tenants, members)
	user := &domain.User{ID: 10}
	active := int64(2)
	sess := &domain.AuthSession{ActiveTenantID: &active}

	tn, role, err := r.Resolve(context.Background(), user, sess)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if tn.ID != 2 || role != domain.RoleAdmin {
		t.Fatalf("got tenant %d role %s, want 2/admin", tn.ID, role)
	}
}

func TestResolveFallsBackToFirstMembership(t *testing.T) {
	tenants := fakeTenants{m: map[int64]*domain.Tenant{1: {ID: 1, Status: domain.TenantActive}}}
	members := fakeMembers{m: map[int64][]domain.Membership{10: {{UserID: 10, TenantID: 1, Role: domain.RoleUser}}}}
	r := NewTenantResolver(tenants, members)

	tn, role, err := r.Resolve(context.Background(), &domain.User{ID: 10}, &domain.AuthSession{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if tn.ID != 1 || role != domain.RoleUser {
		t.Fatalf("got %d/%s, want 1/user", tn.ID, role)
	}
}

func TestResolveNoMembership(t *testing.T) {
	r := NewTenantResolver(fakeTenants{m: map[int64]*domain.Tenant{}}, fakeMembers{m: map[int64][]domain.Membership{}})
	if _, _, err := r.Resolve(context.Background(), &domain.User{ID: 99}, &domain.AuthSession{}); !errors.Is(err, ErrNoMembership) {
		t.Fatalf("want ErrNoMembership, got %v", err)
	}
}
