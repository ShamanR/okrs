package statuses_test

import (
	"context"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/store/statuses"
	"okrs/internal/store/testutil"
)

// sc1 is the default-tenant scope used across the existing single-tenant status tests.
var sc1 = domain.TenantScope{TenantID: 1}

func TestTeamPeriodStatusRoundTrip(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	r := statuses.NewTeamStatusRepository(pool)

	var teamID, periodID int64
	pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('StatusTeam') RETURNING id`).Scan(&teamID)
	pool.QueryRow(ctx, `INSERT INTO periods (name,start_date,end_date) VALUES ('StatusPeriod','2025-01-01','2025-03-31') RETURNING id`).Scan(&periodID)

	// Missing entry returns TeamPeriodStatusNoGoals.
	status, err := r.GetTeamPeriodStatus(ctx, sc1, teamID, periodID)
	if err != nil {
		t.Fatalf("GetTeamPeriodStatus missing: %v", err)
	}
	if status != domain.TeamPeriodStatusNoGoals {
		t.Fatalf("expected NoGoals for missing entry, got %s", status)
	}

	if err := r.SetTeamPeriodStatus(ctx, sc1, teamID, periodID, domain.TeamPeriodStatusInProgress); err != nil {
		t.Fatalf("SetTeamPeriodStatus: %v", err)
	}
	got, err := r.GetTeamPeriodStatus(ctx, sc1, teamID, periodID)
	if err != nil {
		t.Fatalf("GetTeamPeriodStatus after set: %v", err)
	}
	if got != domain.TeamPeriodStatusInProgress {
		t.Fatalf("expected OnTrack, got %s", got)
	}

	// ON CONFLICT update returns the new value.
	r.SetTeamPeriodStatus(ctx, sc1, teamID, periodID, domain.TeamPeriodStatusClosed)
	got, _ = r.GetTeamPeriodStatus(ctx, sc1, teamID, periodID)
	if got != domain.TeamPeriodStatusClosed {
		t.Fatalf("expected AtRisk after overwrite, got %s", got)
	}
}

func TestListTeamPeriodStatuses(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	r := statuses.NewTeamStatusRepository(pool)

	var t1, t2, t3, periodID int64
	pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('T1') RETURNING id`).Scan(&t1)
	pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('T2') RETURNING id`).Scan(&t2)
	pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('T3') RETURNING id`).Scan(&t3)
	pool.QueryRow(ctx, `INSERT INTO periods (name,start_date,end_date) VALUES ('LP','2025-04-01','2025-06-30') RETURNING id`).Scan(&periodID)

	r.SetTeamPeriodStatus(ctx, sc1, t1, periodID, domain.TeamPeriodStatusInProgress)
	r.SetTeamPeriodStatus(ctx, sc1, t2, periodID, domain.TeamPeriodStatusClosed)

	m, err := r.ListTeamPeriodStatuses(ctx, sc1, periodID, []int64{t1, t2, t3})
	if err != nil {
		t.Fatalf("ListTeamPeriodStatuses: %v", err)
	}
	if m[t1] != domain.TeamPeriodStatusInProgress {
		t.Fatalf("expected OnTrack for t1, got %s", m[t1])
	}
	if m[t2] != domain.TeamPeriodStatusClosed {
		t.Fatalf("expected AtRisk for t2, got %s", m[t2])
	}
	// t3 has no entry — must be absent.
	if _, ok := m[t3]; ok {
		t.Fatalf("expected t3 absent, got %s", m[t3])
	}
}

func TestGetTeamPeriodStatusWithTime(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	r := statuses.NewTeamStatusRepository(pool)

	var teamID, periodID int64
	pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('TWTeam') RETURNING id`).Scan(&teamID)
	pool.QueryRow(ctx, `INSERT INTO periods (name,start_date,end_date) VALUES ('TWPeriod','2025-07-01','2025-09-30') RETURNING id`).Scan(&periodID)

	// Missing → NoGoals, nil time.
	status, ts, err := r.GetTeamPeriodStatusWithTime(ctx, sc1, teamID, periodID)
	if err != nil {
		t.Fatalf("GetTeamPeriodStatusWithTime missing: %v", err)
	}
	if status != domain.TeamPeriodStatusNoGoals || ts != nil {
		t.Fatalf("expected NoGoals+nil time, got %s %v", status, ts)
	}

	r.SetTeamPeriodStatus(ctx, sc1, teamID, periodID, domain.TeamPeriodStatusInProgress)
	status, ts, err = r.GetTeamPeriodStatusWithTime(ctx, sc1, teamID, periodID)
	if err != nil {
		t.Fatalf("GetTeamPeriodStatusWithTime after set: %v", err)
	}
	if status != domain.TeamPeriodStatusInProgress {
		t.Fatalf("expected OnTrack, got %s", status)
	}
	if ts == nil {
		t.Fatal("expected non-nil updated_at after set")
	}
}

func TestSetTeamPeriodStatuses_Batch(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	r := statuses.NewTeamStatusRepository(pool)

	var periodID int64
	pool.QueryRow(ctx, `INSERT INTO periods (name,start_date,end_date) VALUES ('BulkP','2025-01-01','2025-03-31') RETURNING id`).Scan(&periodID)
	ids := make([]int64, 3)
	for i := range ids {
		pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('BT') RETURNING id`).Scan(&ids[i])
	}
	// Seed one team already closed to prove the batch overwrites existing rows too.
	if err := r.SetTeamPeriodStatus(ctx, sc1, ids[0], periodID, domain.TeamPeriodStatusClosed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := r.SetTeamPeriodStatuses(ctx, sc1, periodID, ids, domain.TeamPeriodStatusInProgress); err != nil {
		t.Fatalf("SetTeamPeriodStatuses: %v", err)
	}
	got, err := r.ListTeamPeriodStatuses(ctx, sc1, periodID, ids)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, id := range ids {
		if got[id] != domain.TeamPeriodStatusInProgress {
			t.Fatalf("team %d: expected in_progress, got %s", id, got[id])
		}
	}

	// Empty slice is a no-op, not an error.
	if err := r.SetTeamPeriodStatuses(ctx, sc1, periodID, nil, domain.TeamPeriodStatusClosed); err != nil {
		t.Fatalf("empty slice must be no-op: %v", err)
	}
}
