package grants_test

import (
	"context"
	"testing"

	"okrs/internal/domain"
	"okrs/internal/store/grants"
	"okrs/internal/store/testutil"

	"github.com/jackc/pgx/v5/pgxpool"
)

// sc1 is the default-tenant scope used across the existing single-tenant grant tests.
var sc1 = domain.TenantScope{TenantID: 1}

func insertTeam(t *testing.T, pool *pgxpool.Pool, ctx context.Context, name string, parentID *int64) int64 {
	t.Helper()
	var id int64
	var err error
	if parentID == nil {
		err = pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ($1) RETURNING id`, name).Scan(&id)
	} else {
		err = pool.QueryRow(ctx, `INSERT INTO teams (name, parent_id) VALUES ($1,$2) RETURNING id`, name, *parentID).Scan(&id)
	}
	if err != nil {
		t.Fatalf("insertTeam %s: %v", name, err)
	}
	return id
}

func TestGrantRepositoryCRUD(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	r := grants.NewGrantRepository(pool)

	var userID int64
	pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key,provider,subject,display_name) VALUES ('g|u1','github','u1','Granter') RETURNING id`).Scan(&userID)
	teamID := insertTeam(t, pool, ctx, "GrantTeam", nil)

	gs, err := r.ListUserGrants(ctx, sc1, userID)
	if err != nil {
		t.Fatalf("ListUserGrants empty: %v", err)
	}
	if len(gs) != 0 {
		t.Fatalf("expected 0 grants, got %d", len(gs))
	}

	if err := r.AddUserGrant(ctx, sc1, userID, teamID, 1); err != nil {
		t.Fatalf("AddUserGrant: %v", err)
	}
	gs, err = r.ListUserGrants(ctx, sc1, userID)
	if err != nil {
		t.Fatalf("ListUserGrants after add: %v", err)
	}
	if len(gs) != 1 || gs[0].TeamID != teamID {
		t.Fatalf("expected 1 grant for teamID %d, got %+v", teamID, gs)
	}

	// ON CONFLICT DO NOTHING — idempotent.
	if err := r.AddUserGrant(ctx, sc1, userID, teamID, 1); err != nil {
		t.Fatalf("AddUserGrant duplicate: %v", err)
	}
	gs, _ = r.ListUserGrants(ctx, sc1, userID)
	if len(gs) != 1 {
		t.Fatalf("expected 1 grant after duplicate add, got %d", len(gs))
	}

	if err := r.RemoveUserGrant(ctx, sc1, userID, teamID); err != nil {
		t.Fatalf("RemoveUserGrant: %v", err)
	}
	gs, _ = r.ListUserGrants(ctx, sc1, userID)
	if len(gs) != 0 {
		t.Fatalf("expected 0 grants after remove, got %d", len(gs))
	}
}

func TestListDescendantTeamIDsFlat(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	r := grants.NewGrantRepository(pool)

	id := insertTeam(t, pool, ctx, "Lone", nil)

	ids, err := r.ListDescendantTeamIDs(ctx, sc1, []int64{id})
	if err != nil {
		t.Fatalf("ListDescendantTeamIDs: %v", err)
	}
	if len(ids) != 1 || ids[0] != id {
		t.Fatalf("expected [%d], got %v", id, ids)
	}
}

func TestListDescendantTeamIDsTree(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	r := grants.NewGrantRepository(pool)

	root := insertTeam(t, pool, ctx, "Root", nil)
	child := insertTeam(t, pool, ctx, "Child", &root)
	grand := insertTeam(t, pool, ctx, "Grand", &child)

	ids, err := r.ListDescendantTeamIDs(ctx, sc1, []int64{root})
	if err != nil {
		t.Fatalf("ListDescendantTeamIDs tree: %v", err)
	}
	want := map[int64]bool{root: true, child: true, grand: true}
	if len(ids) != 3 {
		t.Fatalf("expected 3 IDs, got %v", ids)
	}
	for _, id := range ids {
		if !want[id] {
			t.Fatalf("unexpected ID %d in result %v", id, ids)
		}
	}
}

func TestListDescendantTeamIDsEmpty(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	r := grants.NewGrantRepository(pool)

	ids, err := r.ListDescendantTeamIDs(ctx, sc1, nil)
	if err != nil {
		t.Fatalf("ListDescendantTeamIDs nil: %v", err)
	}
	if ids != nil {
		t.Fatalf("expected nil for empty input, got %v", ids)
	}
}
