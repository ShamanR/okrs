package auth

import (
	"context"
	"testing"
)

func ctxWithAllowedTeams(ids []int64) context.Context {
	return context.WithValue(context.Background(), allowedTeamsKey, ids)
}

func TestCanAccessTeamFromCtxNoScopeAllows(t *testing.T) {
	// Scope key not present → treat as unrestricted (e.g. tests without middleware).
	if !CanAccessTeamFromCtx(context.Background(), 42) {
		t.Fatal("expected access when scope not loaded")
	}
}

func TestCanAccessTeamFromCtxAdminNilSliceAllowsAll(t *testing.T) {
	ctx := ctxWithAllowedTeams(nil) // nil = admin / unrestricted
	if !CanAccessTeamFromCtx(ctx, 1) {
		t.Fatal("admin (nil slice) should have unrestricted access")
	}
	if !CanAccessTeamFromCtx(ctx, 99999) {
		t.Fatal("admin should access any team ID")
	}
}

func TestCanAccessTeamFromCtxEmptySliceDeniesAll(t *testing.T) {
	ctx := ctxWithAllowedTeams([]int64{}) // empty = no grants
	if CanAccessTeamFromCtx(ctx, 1) {
		t.Fatal("empty slice should deny access to all teams")
	}
}

func TestCanAccessTeamFromCtxGrantedTeamAllowed(t *testing.T) {
	ctx := ctxWithAllowedTeams([]int64{10, 20, 30})
	if !CanAccessTeamFromCtx(ctx, 20) {
		t.Fatal("team 20 should be accessible")
	}
}

func TestCanAccessTeamFromCtxUngrantedTeamDenied(t *testing.T) {
	ctx := ctxWithAllowedTeams([]int64{10, 20, 30})
	if CanAccessTeamFromCtx(ctx, 99) {
		t.Fatal("team 99 should not be accessible")
	}
}

func TestAllowedTeamIDsFromCtxNotSet(t *testing.T) {
	_, ok := AllowedTeamIDsFromCtx(context.Background())
	if ok {
		t.Fatal("expected false when scope not loaded")
	}
}

func TestAllowedTeamIDsFromCtxAdminNil(t *testing.T) {
	ctx := ctxWithAllowedTeams(nil)
	ids, ok := AllowedTeamIDsFromCtx(ctx)
	if !ok {
		t.Fatal("expected ok=true when scope is loaded")
	}
	if ids != nil {
		t.Fatal("admin scope should be nil")
	}
}

func TestAllowedTeamIDsFromCtxReturnsIDs(t *testing.T) {
	want := []int64{5, 10, 15}
	ctx := ctxWithAllowedTeams(want)
	got, ok := AllowedTeamIDsFromCtx(ctx)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d IDs, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("mismatch at index %d: want %d got %d", i, want[i], got[i])
		}
	}
}
