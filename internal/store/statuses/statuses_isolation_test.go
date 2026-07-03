package statuses_test

import (
	"context"
	"testing"

	"okrs/internal/domain"
	"okrs/internal/store/statuses"
	"okrs/internal/store/testutil"
)

func TestTeamPeriodStatusScopedByTenant(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `INSERT INTO tenants (id, slug, name) OVERRIDING SYSTEM VALUE VALUES (2,'t2','T2')`); err != nil {
		t.Fatalf("tenant 2: %v", err)
	}
	var teamID, periodID int64
	pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('S') RETURNING id`).Scan(&teamID)
	pool.QueryRow(ctx, `INSERT INTO periods (name,start_date,end_date) VALUES ('P','2025-01-01','2025-03-31') RETURNING id`).Scan(&periodID)

	r := statuses.NewTeamStatusRepository(pool)
	if err := r.SetTeamPeriodStatus(ctx, sc1, teamID, periodID, domain.TeamPeriodStatusInProgress); err != nil {
		t.Fatalf("set: %v", err)
	}

	scope2 := domain.TenantScope{TenantID: 2}
	// scope2 does not see tenant-1's status (returns the NoGoals default).
	got, err := r.GetTeamPeriodStatus(ctx, scope2, teamID, periodID)
	if err != nil {
		t.Fatalf("get scope2: %v", err)
	}
	if got != domain.TeamPeriodStatusNoGoals {
		t.Fatalf("scope2 saw status %s, want NoGoals (isolated)", got)
	}
	// scope2 list is empty for the tenant-1 team.
	m, _ := r.ListTeamPeriodStatuses(ctx, scope2, periodID, []int64{teamID})
	if _, ok := m[teamID]; ok {
		t.Fatalf("scope2 must not list tenant-1 status")
	}
}
