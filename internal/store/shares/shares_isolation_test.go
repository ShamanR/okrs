package shares_test

import (
	"context"
	"testing"

	"okrs/internal/domain"
	"okrs/internal/store/shares"
	"okrs/internal/store/testutil"
)

func TestGoalSharesScopedByTenant(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `INSERT INTO tenants (id, slug, name) OVERRIDING SYSTEM VALUE VALUES (2,'t2','T2')`); err != nil {
		t.Fatalf("tenant 2: %v", err)
	}
	var sharedTeamID int64
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('Iso') RETURNING id`).Scan(&sharedTeamID); err != nil {
		t.Fatalf("team: %v", err)
	}
	goalID := prepareGoal(t, pool, ctx, "iso") // tenant 1 (default)

	r := shares.NewGoalShareRepository(pool)
	if err := r.ReplaceGoalShares(ctx, sc1, goalID, []shares.GoalShareInput{{TeamID: sharedTeamID, Weight: 50}}); err != nil {
		t.Fatalf("replace shares: %v", err)
	}

	scope2 := domain.TenantScope{TenantID: 2}

	// scope2 sees no shares of the tenant-1 goal.
	if l, err := r.ListGoalShares(ctx, scope2, goalID); err != nil || len(l) != 0 {
		t.Fatalf("scope2 ListGoalShares = %+v (err %v), want empty", l, err)
	}
	// scope2 cannot read the share.
	if _, err := r.GetGoalShare(ctx, scope2, goalID, sharedTeamID); err == nil {
		t.Fatalf("scope2 must not read tenant-1 share")
	}
	// scope2 delete is a no-op on the tenant-1 share.
	if err := r.DeleteGoalShare(ctx, scope2, goalID, sharedTeamID); err != nil {
		t.Fatalf("delete (scope2): %v", err)
	}
	if l, _ := r.ListGoalShares(ctx, sc1, goalID); len(l) != 1 {
		t.Fatalf("tenant-1 share must survive scope2 delete, got %d", len(l))
	}
}
