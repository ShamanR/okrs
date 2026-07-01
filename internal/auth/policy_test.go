package auth

import (
	"context"
	"testing"

	"okrs/internal/domain"
	"okrs/internal/store/grants"
)

func ctxWithAllowedTeams(ids []int64) context.Context {
	return context.WithValue(context.Background(), allowedTeamsKey, ids)
}

// fakeGrants is a grantsReader stub for LoadScope tests (no DB).
type fakeGrants struct {
	byUser      map[int64][]grants.HierarchyGrant
	descendants map[int64][]int64
}

func (f fakeGrants) ListUserGrants(_ context.Context, _ domain.TenantScope, userID int64) ([]grants.HierarchyGrant, error) {
	return f.byUser[userID], nil
}
func (f fakeGrants) ListDescendantTeamIDs(_ context.Context, _ domain.TenantScope, rootIDs []int64) ([]int64, error) {
	var out []int64
	for _, id := range rootIDs {
		out = append(out, f.descendants[id]...)
	}
	return out, nil
}

// A user carrying the legacy global users.is_admin flag but whose ACTIVE tenant role is not
// admin must NOT get unrestricted access — scope comes from the tenant grants, not the flag.
func TestLoadScopeIgnoresGlobalIsAdminFlag(t *testing.T) {
	e := NewPolicyEvaluator(fakeGrants{
		byUser:      map[int64][]grants.HierarchyGrant{7: {{TeamID: 10}}},
		descendants: map[int64][]int64{10: {10, 11}},
	}, nil)
	user := &domain.User{ID: 7, IsAdmin: true} // global flag set, but not a tenant admin
	ctx, err := e.LoadScope(context.Background(), domain.TenantScope{TenantID: 1}, user, Config{})
	if err != nil {
		t.Fatalf("LoadScope: %v", err)
	}
	ids, ok := AllowedTeamIDsFromCtx(ctx)
	if !ok || ids == nil {
		t.Fatalf("global is_admin must not grant unrestricted access; ids=%v ok=%v", ids, ok)
	}
	if len(ids) != 2 {
		t.Fatalf("want grant expansion [10 11], got %v", ids)
	}
}

// A user whose ACTIVE tenant role is admin gets unrestricted access (nil slice).
func TestLoadScopeActiveAdminRoleUnrestricted(t *testing.T) {
	e := NewPolicyEvaluator(fakeGrants{}, nil)
	user := &domain.User{ID: 7} // not global admin
	ctx := WithActiveRole(context.Background(), domain.RoleAdmin)
	ctx, err := e.LoadScope(ctx, domain.TenantScope{TenantID: 1}, user, Config{})
	if err != nil {
		t.Fatalf("LoadScope: %v", err)
	}
	if ids, ok := AllowedTeamIDsFromCtx(ctx); !ok || ids != nil {
		t.Fatalf("active admin role must be unrestricted (nil); ids=%v ok=%v", ids, ok)
	}
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
