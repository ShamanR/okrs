package memberships

import (
	"context"
	"testing"
	"time"

	"okrs/internal/core/domain"
)

type fakeMembershipBackend struct {
	data  map[int64][]domain.Membership
	calls int
}

func (f *fakeMembershipBackend) ListByUser(_ context.Context, userID int64) ([]domain.Membership, error) {
	f.calls++
	return f.data[userID], nil
}

func TestMembershipCacheCachesPerUser(t *testing.T) {
	backend := &fakeMembershipBackend{data: map[int64][]domain.Membership{
		7: {{UserID: 7, TenantID: 1, Role: domain.RoleAdmin}},
	}}
	c := newMembershipCacheWithBackend(backend, time.Minute)
	ctx := context.Background()

	if _, err := c.ListByUser(ctx, 7); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := c.ListByUser(ctx, 7); err != nil {
		t.Fatalf("second: %v", err)
	}
	if backend.calls != 1 {
		t.Fatalf("expected 1 backend call (cache hit), got %d", backend.calls)
	}

	c.InvalidateUser(7)
	if _, err := c.ListByUser(ctx, 7); err != nil {
		t.Fatalf("after invalidate: %v", err)
	}
	if backend.calls != 2 {
		t.Fatalf("expected reload after invalidate, got %d", backend.calls)
	}
}
