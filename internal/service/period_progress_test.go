package service

import (
	"testing"
	"time"

	"okrs/internal/domain"
	"okrs/internal/store/progresssnap"
)

func TestBuildProgressSeries_AveragesByDateAndAppendsToday(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	d1 := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	rows := []progresssnap.SeriesRow{
		{TeamID: 1, Date: d1, Progress: 20},
		{TeamID: 2, Date: d1, Progress: 40},
	}
	s := buildProgressSeries(rows, nil, "2026-02-01", 55, start, end)
	if s.PeriodStart != "2026-01-01" || s.PeriodEnd != "2026-03-31" {
		t.Fatalf("period bounds: %+v", s)
	}
	if s.Points[0].Date != "2026-01-10" || s.Points[0].Progress != 30 { // avg(20,40)
		t.Fatalf("date avg: %+v", s.Points[0])
	}
	last := s.Points[len(s.Points)-1]
	if last.Date != "2026-02-01" || last.Progress != 55 {
		t.Fatalf("live today point: %+v", last)
	}
}

func TestBuildProgressSeries_TeamFilterRestrictsAveraging(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	d1 := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	rows := []progresssnap.SeriesRow{
		{TeamID: 1, Date: d1, Progress: 20},
		{TeamID: 2, Date: d1, Progress: 80}, // out of scope
	}
	filter := map[int64]bool{1: true}
	s := buildProgressSeries(rows, filter, "2026-06-01", 0, start, end)
	// only team 1 counts for 2026-01-10 → 20 (today point is a separate date)
	if s.Points[0].Date != "2026-01-10" || s.Points[0].Progress != 20 {
		t.Fatalf("filtered avg wrong: %+v", s.Points[0])
	}
}

func TestComputeTeamSnapshots_SkipsNoGoalDeletedAndDraftTeams(t *testing.T) {
	deleted := domain.Team{ID: 99, Name: "Gone"}
	ts := time.Time{}
	deleted.DeletedAt = &ts
	teams := []domain.Team{
		{ID: 1, Name: "Alpha"}, // in_progress, has goals -> snapshotted
		{ID: 2, Name: "Beta"},  // no goals -> skipped
		{ID: 3, Name: "Draft"}, // forming (черновик), has goals -> skipped
		deleted,                // soft-deleted -> skipped
	}
	goalsByTeam := map[int64][]domain.Goal{
		1:  {{ID: 10, TeamID: 1, Weight: 100, KeyResults: []domain.KeyResult{numericKR(100, 100, 40)}}},
		3:  {{ID: 30, TeamID: 3, Weight: 100, KeyResults: []domain.KeyResult{numericKR(300, 100, 90)}}},
		99: {{ID: 990, TeamID: 99, Weight: 100, KeyResults: []domain.KeyResult{numericKR(9900, 100, 50)}}},
	}
	statuses := map[int64]domain.TeamPeriodStatus{
		1: domain.TeamPeriodStatusInProgress,
		3: domain.TeamPeriodStatusForming,
	}
	data := &PeriodData{PeriodID: 7, Teams: teams, GoalsByTeam: goalsByTeam, Statuses: statuses}

	snaps := computeTeamSnapshots(data)
	if len(snaps) != 1 {
		t.Fatalf("want 1 snapshot (Alpha only), got %d: %+v", len(snaps), snaps)
	}
	if snaps[0].TeamID != 1 || snaps[0].Progress != 40 {
		t.Fatalf("snapshot wrong: %+v", snaps[0])
	}
}
