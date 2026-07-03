package store

import (
	"context"
	"testing"

	"okrs/internal/store/testutil"
)

// After migration 024 drops the unique team-name constraint, SeedDemo must not
// rely on ON CONFLICT (name) (PostgreSQL would error with no conflict arbiter)
// and must stay idempotent across re-runs.
func TestSeedDemoIdempotentWithoutNameConstraint(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	s := New(pool)

	var periodID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO periods (name, start_date, end_date)
		VALUES ('2024 Q3', '2024-07-01', '2024-09-30')
		RETURNING id`).Scan(&periodID); err != nil {
		t.Fatalf("insert period: %v", err)
	}

	if err := s.SeedDemo(ctx, periodID); err != nil {
		t.Fatalf("SeedDemo first run: %v", err)
	}
	if err := s.SeedDemo(ctx, periodID); err != nil {
		t.Fatalf("SeedDemo second run must be idempotent: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM teams WHERE name='Platform' AND deleted_at IS NULL`).Scan(&count); err != nil {
		t.Fatalf("count teams: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 active 'Platform' team after re-seed, got %d", count)
	}
}
