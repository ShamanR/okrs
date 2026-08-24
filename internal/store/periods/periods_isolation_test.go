package periods_test

import (
	"context"
	"testing"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/store/periods"
	"okrs/internal/store/testutil"
)

func TestPeriodsScopedByTenant(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `INSERT INTO tenants (id, slug, name) OVERRIDING SYSTEM VALUE VALUES (2,'t2','T2')`); err != nil {
		t.Fatalf("tenant 2: %v", err)
	}
	r := periods.NewPeriodRepository(pool)
	scope2 := domain.TenantScope{TenantID: 2}

	in := periods.PeriodInput{
		Name:      "2025 Q1",
		StartDate: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC),
	}
	// Same period name is allowed in different tenants (unique per tenant, migration 031).
	id1, err := r.CreatePeriod(ctx, sc1, in)
	if err != nil {
		t.Fatalf("create scope1: %v", err)
	}
	if _, err := r.CreatePeriod(ctx, scope2, in); err != nil {
		t.Fatalf("create scope2 (same name, other tenant): %v", err)
	}

	// Each scope sees only its own period.
	l1, _ := r.ListPeriods(ctx, sc1)
	if len(l1) != 1 || l1[0].ID != id1 {
		t.Fatalf("scope1 saw %+v, want [%d]", l1, id1)
	}
	l2, _ := r.ListPeriods(ctx, scope2)
	if len(l2) != 1 {
		t.Fatalf("scope2 saw %d periods, want 1", len(l2))
	}

	// scope2 cannot read scope1's period.
	if _, err := r.GetPeriod(ctx, scope2, id1); err == nil {
		t.Fatalf("scope2 must not read scope1 period")
	}
}
