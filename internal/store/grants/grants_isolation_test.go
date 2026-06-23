package grants_test

import (
	"context"
	"testing"

	"okrs/internal/domain"
	"okrs/internal/store/grants"
	"okrs/internal/store/testutil"
)

func TestGrantsScopedByTenant(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `INSERT INTO tenants (id, slug, name) OVERRIDING SYSTEM VALUE VALUES (2,'t2','T2')`); err != nil {
		t.Fatalf("tenant 2: %v", err)
	}
	var userID int64
	pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key,provider,subject,display_name) VALUES ('g|iso','github','iso','Iso') RETURNING id`).Scan(&userID)
	teamID := insertTeam(t, pool, ctx, "IsoTeam", nil) // tenant 1 (default)

	r := grants.NewGrantRepository(pool)
	if err := r.AddUserGrant(ctx, sc1, userID, teamID, 1); err != nil {
		t.Fatalf("add grant: %v", err)
	}

	scope2 := domain.TenantScope{TenantID: 2}

	// scope2 sees none of the user's tenant-1 grants.
	gs, err := r.ListUserGrants(ctx, scope2, userID)
	if err != nil {
		t.Fatalf("list scope2: %v", err)
	}
	if len(gs) != 0 {
		t.Fatalf("scope2 saw %d grants, want 0", len(gs))
	}

	// The recursive descendant query under scope2 does not traverse tenant-1 teams.
	ids, err := r.ListDescendantTeamIDs(ctx, scope2, []int64{teamID})
	if err != nil {
		t.Fatalf("descendants scope2: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("scope2 descendants of tenant-1 team = %v, want empty", ids)
	}

	// Under tenant 1 the grant and team are visible.
	if gs1, _ := r.ListUserGrants(ctx, sc1, userID); len(gs1) != 1 {
		t.Fatalf("scope1 grants = %d, want 1", len(gs1))
	}
}
