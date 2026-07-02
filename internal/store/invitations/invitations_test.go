package invitations_test

import (
	"context"
	"testing"

	"okrs/internal/domain"
	"okrs/internal/store/invitations"
	"okrs/internal/store/testutil"
)

func intp(n int) *int { return &n }

func TestConsumeOneTime(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := invitations.NewInvitationRepository(pool)
	scope := domain.TenantScope{TenantID: 1}

	inv, err := repo.Create(ctx, scope, domain.RoleUser, "hash-one", 1, intp(1), nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if inv.Status != domain.InvitationPending || inv.Email != nil || inv.MaxUses == nil || *inv.MaxUses != 1 {
		t.Fatalf("unexpected invitation: %+v", inv)
	}

	res, err := repo.Consume(ctx, "hash-one")
	if err != nil {
		t.Fatalf("first consume: %v", err)
	}
	if res.TenantID != 1 || res.Role != domain.RoleUser {
		t.Fatalf("claim result = %+v", res)
	}
	if _, err := repo.Consume(ctx, "hash-one"); err != invitations.ErrNotFound {
		t.Fatalf("second consume of one-time link must fail, got %v", err)
	}
}

func TestConsumeLimited(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := invitations.NewInvitationRepository(pool)
	scope := domain.TenantScope{TenantID: 1}

	if _, err := repo.Create(ctx, scope, domain.RoleUser, "hash-lim", 1, intp(2), nil); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := repo.Consume(ctx, "hash-lim"); err != nil {
		t.Fatalf("consume 1: %v", err)
	}
	if _, err := repo.Consume(ctx, "hash-lim"); err != nil {
		t.Fatalf("consume 2: %v", err)
	}
	if _, err := repo.Consume(ctx, "hash-lim"); err != invitations.ErrNotFound {
		t.Fatalf("consume 3 must fail (limit 2), got %v", err)
	}
}

func TestConsumeUnlimited(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := invitations.NewInvitationRepository(pool)
	scope := domain.TenantScope{TenantID: 1}

	if _, err := repo.Create(ctx, scope, domain.RoleAdmin, "hash-unl", 1, nil, nil); err != nil {
		t.Fatalf("create: %v", err)
	}
	for i := 0; i < 5; i++ {
		res, err := repo.Consume(ctx, "hash-unl")
		if err != nil {
			t.Fatalf("consume %d: %v", i, err)
		}
		if res.Role != domain.RoleAdmin {
			t.Fatalf("role = %q", res.Role)
		}
	}
}

func TestConsumeUnknown(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := invitations.NewInvitationRepository(pool)
	if _, err := repo.Consume(ctx, "nope"); err != invitations.ErrNotFound {
		t.Fatalf("unknown token → ErrNotFound, got %v", err)
	}
}

func TestRevokeThenConsume(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := invitations.NewInvitationRepository(pool)
	scope := domain.TenantScope{TenantID: 1}

	inv, err := repo.Create(ctx, scope, domain.RoleUser, "hash-rev", 1, nil, nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.Revoke(ctx, scope, inv.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := repo.Consume(ctx, "hash-rev"); err != invitations.ErrNotFound {
		t.Fatalf("revoked link must not consume, got %v", err)
	}
	// Revoking again / a non-existent id is idempotent.
	if err := repo.Revoke(ctx, scope, inv.ID); err != nil {
		t.Fatalf("re-revoke should be no-op nil, got %v", err)
	}
	if err := repo.Revoke(ctx, scope, 999999); err != nil {
		t.Fatalf("revoke missing id should be no-op nil, got %v", err)
	}
	// Revoke is tenant-scoped: another tenant cannot revoke this row (no error, no effect).
	if err := repo.Revoke(ctx, domain.TenantScope{TenantID: 2}, inv.ID); err != nil {
		t.Fatalf("cross-tenant revoke should be no-op nil, got %v", err)
	}
}

func TestListPendingByTenantFields(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := invitations.NewInvitationRepository(pool)
	scope := domain.TenantScope{TenantID: 1}

	if _, err := repo.Create(ctx, scope, domain.RoleUser, "hash-list", 1, intp(3), nil); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := repo.Consume(ctx, "hash-list"); err != nil {
		t.Fatalf("consume: %v", err)
	}
	list, err := repo.ListPendingByTenant(ctx, scope)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("want 1 pending, got %d", len(list))
	}
	got := list[0]
	if got.MaxUses == nil || *got.MaxUses != 3 || got.UseCount != 1 {
		t.Fatalf("list fields = %+v", got)
	}
}
