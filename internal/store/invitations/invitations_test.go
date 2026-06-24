package invitations_test

import (
	"context"
	"testing"

	"okrs/internal/domain"
	"okrs/internal/store/invitations"
	"okrs/internal/store/testutil"
)

func TestInvitationCreateClaimSingleUse(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := invitations.NewInvitationRepository(pool)
	scope := domain.TenantScope{TenantID: 1}

	inv, err := repo.Create(ctx, scope, "a@example.com", domain.RoleUser, "hash123", 1, nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if inv.Status != domain.InvitationPending {
		t.Fatalf("status = %q", inv.Status)
	}

	got, err := repo.GetPendingByTokenHash(ctx, "hash123")
	if err != nil || got.ID != inv.ID {
		t.Fatalf("get by hash: %v / %+v", err, got)
	}

	ok, err := repo.MarkClaimed(ctx, inv.ID)
	if err != nil || !ok {
		t.Fatalf("first claim should succeed: ok=%v err=%v", ok, err)
	}
	// Second claim is a no-op (single use).
	ok, _ = repo.MarkClaimed(ctx, inv.ID)
	if ok {
		t.Fatalf("second claim must not change a row")
	}
	// No longer pending → lookup fails.
	if _, err := repo.GetPendingByTokenHash(ctx, "hash123"); err != invitations.ErrNotFound {
		t.Fatalf("claimed token must not be pending, got %v", err)
	}
}
