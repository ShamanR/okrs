package progresssnap_test

import (
	"context"
	"testing"
	"time"

	"okrs/internal/domain"
	"okrs/internal/store/progresssnap"
	"okrs/internal/store/testutil"
)

func TestUpsertAndListSnapshots(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}

	var periodID, teamID int64
	if err := pool.QueryRow(ctx, `INSERT INTO periods (name, start_date, end_date, tenant_id)
		VALUES ('Q1', '2026-01-01', '2026-03-31', 1) RETURNING id`).Scan(&periodID); err != nil {
		t.Fatalf("insert period: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name, tenant_id) VALUES ('Alpha', 1) RETURNING id`).Scan(&teamID); err != nil {
		t.Fatalf("insert team: %v", err)
	}

	r := progresssnap.NewRepository(pool)
	day := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	if err := r.UpsertSnapshots(ctx, scope, periodID, day, []progresssnap.Snapshot{{TeamID: teamID, Progress: 20}}); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	// Idempotent: same day again updates in place, no duplicate row.
	if err := r.UpsertSnapshots(ctx, scope, periodID, day, []progresssnap.Snapshot{{TeamID: teamID, Progress: 35}}); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	rows, err := r.ListSnapshots(ctx, scope, periodID, []int64{teamID})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row after idempotent upsert, got %d", len(rows))
	}
	if rows[0].Progress != 35 {
		t.Fatalf("want updated progress 35, got %d", rows[0].Progress)
	}

	// Empty team filter lists all teams in the period.
	all, err := r.ListSnapshots(ctx, scope, periodID, nil)
	if err != nil || len(all) != 1 {
		t.Fatalf("list all: want 1 got %d (err %v)", len(all), err)
	}
}
