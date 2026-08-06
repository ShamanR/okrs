package grants_test

import (
	"context"
	"testing"

	"okrs/internal/store/grants"
	"okrs/internal/store/testutil"
)

func TestListLeadTeamScope_ReturnsLeadTeamsAndDescendants(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	r := grants.NewGrantRepository(pool)

	// A user whose udid we assign as the lead of the root team.
	var leadUDID string
	if err := pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key,provider,subject,display_name)
		VALUES ('lead|1','github','lead1','Lead One') RETURNING udid`).Scan(&leadUDID); err != nil {
		t.Fatalf("insert lead user: %v", err)
	}

	var root, child int64
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name, lead_udid) VALUES ('Root',$1) RETURNING id`, leadUDID).Scan(&root); err != nil {
		t.Fatalf("insert root: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name, parent_id) VALUES ('Child',$1) RETURNING id`, root).Scan(&child); err != nil {
		t.Fatalf("insert child: %v", err)
	}
	other := insertTeam(t, pool, ctx, "Other", nil)

	ids, err := r.ListLeadTeamScope(ctx, sc1, leadUDID)
	if err != nil {
		t.Fatalf("ListLeadTeamScope: %v", err)
	}
	got := map[int64]bool{}
	for _, id := range ids {
		got[id] = true
	}
	if !got[root] || !got[child] {
		t.Fatalf("want root+child in scope, got %v", ids)
	}
	if got[other] {
		t.Fatalf("unrelated team leaked into scope: %v", ids)
	}

	// Empty udid resolves to no scope.
	empty, err := r.ListLeadTeamScope(ctx, sc1, "")
	if err != nil || empty != nil {
		t.Fatalf("empty udid: want nil,nil got %v,%v", empty, err)
	}
}
