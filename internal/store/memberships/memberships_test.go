package memberships

import (
	"context"
	"testing"

	"okrs/internal/domain"
	"okrs/internal/store/testutil"
)

func TestMembershipUpsertAndList(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()

	var userID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('google:m', 'google', 'm', 'M') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	repo := NewMembershipRepository(pool)
	if _, err := repo.Upsert(ctx, domain.Membership{
		UserID: userID, TenantID: 1, Role: domain.RoleAdmin, Status: domain.MembershipActive,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := repo.ListByUser(ctx, userID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].Role != domain.RoleAdmin {
		t.Fatalf("unexpected memberships: %+v", got)
	}

	// requested membership is excluded from the active list.
	if err := repo.SetStatus(ctx, userID, 1, domain.MembershipRequested); err != nil {
		t.Fatalf("set status: %v", err)
	}
	got, _ = repo.ListByUser(ctx, userID)
	if len(got) != 0 {
		t.Fatalf("requested membership should be excluded, got %+v", got)
	}
}
