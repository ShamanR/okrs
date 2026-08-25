package teams_test

import (
	"context"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/store/teams"
	"okrs/internal/store/testutil"
)

func TestTeamsScopedByTenant(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()

	// Tenant 1 (default) exists from migrations; create tenant 2.
	if _, err := pool.Exec(ctx, `INSERT INTO tenants (id, slug, name) OVERRIDING SYSTEM VALUE VALUES (2,'t2','T2')`); err != nil {
		t.Fatalf("tenant 2: %v", err)
	}
	repo := teams.NewTeamRepository(pool)
	scope1 := domain.TenantScope{TenantID: 1}
	scope2 := domain.TenantScope{TenantID: 2}

	idA, err := repo.CreateTeam(ctx, scope1, teams.TeamInput{Name: "A", Type: domain.TeamTypeTeam})
	if err != nil {
		t.Fatalf("create A: %v", err)
	}
	if _, err := repo.CreateTeam(ctx, scope2, teams.TeamInput{Name: "B", Type: domain.TeamTypeTeam}); err != nil {
		t.Fatalf("create B: %v", err)
	}

	// Each scope sees only its own team.
	a, err := repo.ListTeams(ctx, scope1)
	if err != nil {
		t.Fatalf("list scope1: %v", err)
	}
	if len(a) != 1 || a[0].Name != "A" {
		t.Fatalf("scope1 saw %+v, want [A]", a)
	}
	b, _ := repo.ListTeams(ctx, scope2)
	if len(b) != 1 || b[0].Name != "B" {
		t.Fatalf("scope2 saw %+v, want [B]", b)
	}

	// Team A is invisible/unmodifiable under scope2.
	if _, err := repo.GetTeam(ctx, scope2, idA); err == nil {
		t.Fatalf("scope2 must not read team A")
	}
}
