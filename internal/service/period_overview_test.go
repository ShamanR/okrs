package service

import (
	"context"
	"testing"
	"time"

	"okrs/internal/domain"
)

// numericKR builds a numerical KR with a known progress (current/target of 100%).
func numericKR(id int64, weight, current int) domain.KeyResult {
	return domain.KeyResult{
		ID: id, Weight: weight, Kind: domain.KRKindNumerical,
		Numerical: &domain.KRNumerical{StartValue: 0, TargetValue: 100, CurrentValue: float64(current)},
	}
}

func TestComputePeriodOverview_CountsWeightsProgress(t *testing.T) {
	teams := []domain.Team{
		{ID: 1, Name: "Alpha"},
		{ID: 2, Name: "Beta"},
		{ID: 3, Name: "Gamma"}, // no goals
	}
	goalsByTeam := map[int64][]domain.Goal{
		// Alpha: one goal weight 100, progress 40 -> team progress 40, weight ok.
		1: {{ID: 10, TeamID: 1, Weight: 100, KeyResults: []domain.KeyResult{numericKR(100, 100, 40)}}},
		// Beta: two goals weights 50+40=90 (weight error), progress 100 & 0 -> weighted 55->56.
		2: {
			{ID: 20, TeamID: 2, Weight: 50, KeyResults: []domain.KeyResult{numericKR(200, 100, 100)}},
			{ID: 21, TeamID: 2, Weight: 40, KeyResults: []domain.KeyResult{numericKR(201, 100, 0)}},
		},
	}
	statuses := map[int64]domain.TeamPeriodStatus{
		1: domain.TeamPeriodStatusInProgress,
		2: domain.TeamPeriodStatusReady,
	}
	data := &PeriodData{PeriodID: 7, Teams: teams, GoalsByTeam: goalsByTeam, Statuses: statuses}

	ov := computePeriodOverview(data, 0)

	if ov.Summary.TotalTeams != 3 {
		t.Fatalf("total_teams: want 3, got %d", ov.Summary.TotalTeams)
	}
	if ov.Summary.TeamsWithGoals != 2 {
		t.Fatalf("teams_with_goals: want 2, got %d", ov.Summary.TeamsWithGoals)
	}
	if ov.Summary.ByStatus["in_progress"] != 1 || ov.Summary.ByStatus["ready"] != 1 || ov.Summary.ByStatus["no_goals"] != 1 {
		t.Fatalf("by_status wrong: %+v", ov.Summary.ByStatus)
	}
	if ov.Summary.WeightErrorCount != 1 {
		t.Fatalf("weight_error_count: want 1 (Beta), got %d", ov.Summary.WeightErrorCount)
	}
	// avg of Alpha(40) and Beta(round(100*50+0*40)/90=56) = round((40+56)/2)=48
	if ov.Summary.AvgProgress != 48 {
		t.Fatalf("avg_progress: want 48, got %d", ov.Summary.AvgProgress)
	}
}

func TestComputePeriodOverview_ValidatedCountsAsInProgress(t *testing.T) {
	data := &PeriodData{
		PeriodID: 1,
		Teams:    []domain.Team{{ID: 1, Name: "A"}},
		GoalsByTeam: map[int64][]domain.Goal{
			1: {{ID: 1, TeamID: 1, Weight: 100, KeyResults: []domain.KeyResult{numericKR(1, 100, 0)}}},
		},
		Statuses: map[int64]domain.TeamPeriodStatus{1: domain.TeamPeriodStatus("validated")},
	}
	ov := computePeriodOverview(data, 0)
	if ov.Summary.ByStatus["in_progress"] != 1 {
		t.Fatalf("validated must bucket as in_progress: %+v", ov.Summary.ByStatus)
	}
}

func TestServicePeriodOverview_UsesCache(t *testing.T) {
	data := &PeriodData{
		PeriodID: 5,
		Teams:    []domain.Team{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}},
		GoalsByTeam: map[int64][]domain.Goal{
			1: {{ID: 1, TeamID: 1, Weight: 100, KeyResults: []domain.KeyResult{numericKR(1, 100, 60)}}},
		},
		Statuses: map[int64]domain.TeamPeriodStatus{1: domain.TeamPeriodStatusInProgress},
		CachedAt: time.Now(),
	}
	loader := func(_ context.Context, _ domain.TenantScope, _ int64) (*PeriodData, error) { return data, nil }
	cache := NewHealthCheckInCache(loader, time.Minute, nil)
	s := &Service{hcCache: cache}

	ov, err := s.PeriodOverview(context.Background(), domain.TenantScope{TenantID: 1}, 5, 0)
	if err != nil {
		t.Fatalf("PeriodOverview: %v", err)
	}
	if ov.Summary.TotalTeams != 2 || ov.Summary.TeamsWithGoals != 1 || ov.Summary.AvgProgress != 60 {
		t.Fatalf("overview wrong: %+v", ov.Summary)
	}
}
