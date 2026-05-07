package statuses_test

import (
	"context"
	"testing"

	"okrs/internal/domain"
	"okrs/internal/store/statuses"
	"okrs/internal/store/testutil"
)

func TestTeamPeriodStatusRoundTrip(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	r := statuses.NewTeamStatusRepository(pool)

	var teamID, periodID int64
	pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('StatusTeam') RETURNING id`).Scan(&teamID)
	pool.QueryRow(ctx, `INSERT INTO periods (name,start_date,end_date,sort_order) VALUES ('StatusPeriod','2025-01-01','2025-03-31',1) RETURNING id`).Scan(&periodID)

	// Missing entry returns TeamPeriodStatusNoGoals.
	status, err := r.GetTeamPeriodStatus(ctx, teamID, periodID)
	if err != nil {
		t.Fatalf("GetTeamPeriodStatus missing: %v", err)
	}
	if status != domain.TeamPeriodStatusNoGoals {
		t.Fatalf("expected NoGoals for missing entry, got %s", status)
	}

	if err := r.SetTeamPeriodStatus(ctx, teamID, periodID, domain.TeamPeriodStatusInProgress); err != nil {
		t.Fatalf("SetTeamPeriodStatus: %v", err)
	}
	got, err := r.GetTeamPeriodStatus(ctx, teamID, periodID)
	if err != nil {
		t.Fatalf("GetTeamPeriodStatus after set: %v", err)
	}
	if got != domain.TeamPeriodStatusInProgress {
		t.Fatalf("expected OnTrack, got %s", got)
	}

	// ON CONFLICT update returns the new value.
	r.SetTeamPeriodStatus(ctx, teamID, periodID, domain.TeamPeriodStatusClosed)
	got, _ = r.GetTeamPeriodStatus(ctx, teamID, periodID)
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
	pool.QueryRow(ctx, `INSERT INTO periods (name,start_date,end_date,sort_order) VALUES ('LP','2025-04-01','2025-06-30',1) RETURNING id`).Scan(&periodID)

	r.SetTeamPeriodStatus(ctx, t1, periodID, domain.TeamPeriodStatusInProgress)
	r.SetTeamPeriodStatus(ctx, t2, periodID, domain.TeamPeriodStatusClosed)

	m, err := r.ListTeamPeriodStatuses(ctx, periodID, []int64{t1, t2, t3})
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
	pool.QueryRow(ctx, `INSERT INTO periods (name,start_date,end_date,sort_order) VALUES ('TWPeriod','2025-07-01','2025-09-30',1) RETURNING id`).Scan(&periodID)

	// Missing → NoGoals, nil time.
	status, ts, err := r.GetTeamPeriodStatusWithTime(ctx, teamID, periodID)
	if err != nil {
		t.Fatalf("GetTeamPeriodStatusWithTime missing: %v", err)
	}
	if status != domain.TeamPeriodStatusNoGoals || ts != nil {
		t.Fatalf("expected NoGoals+nil time, got %s %v", status, ts)
	}

	r.SetTeamPeriodStatus(ctx, teamID, periodID, domain.TeamPeriodStatusInProgress)
	status, ts, err = r.GetTeamPeriodStatusWithTime(ctx, teamID, periodID)
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
