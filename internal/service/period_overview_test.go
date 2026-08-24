package service

import (
	"context"
	"testing"
	"time"

	"okrs/internal/core/domain"
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

	ov := computePeriodOverview(data, 0, nil)

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

func TestComputeBalances_CountsAndPercentsWithFixedOrder(t *testing.T) {
	goals := []PeriodGoalItem{
		{ID: 1, WorkType: "Delivery", FocusType: "STABILITY", Priority: "P1"},
		{ID: 2, WorkType: "Delivery", FocusType: "TECH_INDEPENDENCE", Priority: "P1"},
		{ID: 3, WorkType: "Discovery", FocusType: "PROFITABILITY", Priority: "P2"},
	}
	b := computeBalances(goals)
	if b.DiscoveryDelivery[0].Key != "Delivery" || b.DiscoveryDelivery[0].Count != 2 {
		t.Fatalf("delivery bucket: %+v", b.DiscoveryDelivery)
	}
	if b.DiscoveryDelivery[0].Percent != 67 { // round(2/3*100)
		t.Fatalf("delivery percent: want 67, got %d", b.DiscoveryDelivery[0].Percent)
	}
	if len(b.Priorities) != 4 || b.Priorities[0].Key != "P0" || b.Priorities[0].Count != 0 {
		t.Fatalf("priorities must list P0..P3 incl zero: %+v", b.Priorities)
	}
	if len(b.Focuses) != 4 {
		t.Fatalf("focuses must list all 4 categories: %+v", b.Focuses)
	}
}

func TestComputePeriodOverview_EmitsBalancesAndGoals(t *testing.T) {
	teams := []domain.Team{{ID: 1, Name: "Alpha"}}
	goalsByTeam := map[int64][]domain.Goal{
		1: {
			{ID: 10, TeamID: 1, Title: "G1", Weight: 50, Priority: domain.PriorityP1, WorkType: domain.WorkTypeDelivery, FocusType: domain.FocusStability, KeyResults: []domain.KeyResult{numericKR(100, 100, 40)}},
			{ID: 11, TeamID: 1, Title: "G2", Weight: 50, Priority: domain.PriorityP2, WorkType: domain.WorkTypeDiscovery, FocusType: domain.FocusProfitability, KeyResults: []domain.KeyResult{numericKR(101, 100, 60)}},
		},
	}
	statuses := map[int64]domain.TeamPeriodStatus{1: domain.TeamPeriodStatusInProgress}
	data := &PeriodData{PeriodID: 7, Teams: teams, GoalsByTeam: goalsByTeam, Statuses: statuses}

	ov := computePeriodOverview(data, 0, nil)
	if len(ov.Goals) != 2 {
		t.Fatalf("goals: want 2, got %d", len(ov.Goals))
	}
	if ov.Balances.DiscoveryDelivery[0].Count != 1 || ov.Balances.DiscoveryDelivery[1].Count != 1 {
		t.Fatalf("discovery/delivery balance: %+v", ov.Balances.DiscoveryDelivery)
	}
	if ov.Goals[0].WorkType != "Delivery" || ov.Goals[0].TeamName != "Alpha" {
		t.Fatalf("slim goal mapping wrong: %+v", ov.Goals[0])
	}
}

func TestComputePeriodOverview_TeamFilterScopesCounts(t *testing.T) {
	teams := []domain.Team{
		{ID: 1, Name: "Alpha"},
		{ID: 2, Name: "Beta"},
		{ID: 3, Name: "Gamma"},
	}
	goalsByTeam := map[int64][]domain.Goal{
		1: {{ID: 10, TeamID: 1, Weight: 100, KeyResults: []domain.KeyResult{numericKR(100, 100, 40)}}},
		2: {{ID: 20, TeamID: 2, Weight: 100, KeyResults: []domain.KeyResult{numericKR(200, 100, 80)}}},
	}
	statuses := map[int64]domain.TeamPeriodStatus{
		1: domain.TeamPeriodStatusInProgress,
		2: domain.TeamPeriodStatusInProgress,
	}
	data := &PeriodData{PeriodID: 7, Teams: teams, GoalsByTeam: goalsByTeam, Statuses: statuses}

	filter := map[int64]bool{1: true} // only Alpha in scope
	ov := computePeriodOverview(data, 0, filter)
	if ov.Summary.TotalTeams != 1 {
		t.Fatalf("scoped total_teams: want 1, got %d", ov.Summary.TotalTeams)
	}
	if len(ov.Teams) != 1 || ov.Teams[0].TeamID != 1 {
		t.Fatalf("scoped teams mismatch: %+v", ov.Teams)
	}
	if ov.Summary.AvgProgress != 40 {
		t.Fatalf("scoped avg_progress: want 40 (Alpha only), got %d", ov.Summary.AvgProgress)
	}
}

func TestComputePeriodOverview_DraftTeamsExcludedFromProgress(t *testing.T) {
	teams := []domain.Team{
		{ID: 1, Name: "Working"},
		{ID: 2, Name: "Draft"},
	}
	goalsByTeam := map[int64][]domain.Goal{
		1: {{ID: 10, TeamID: 1, Weight: 100, KeyResults: []domain.KeyResult{numericKR(100, 100, 40)}}},
		2: {{ID: 20, TeamID: 2, Weight: 100, KeyResults: []domain.KeyResult{numericKR(200, 100, 90)}}},
	}
	statuses := map[int64]domain.TeamPeriodStatus{
		1: domain.TeamPeriodStatusInProgress,
		2: domain.TeamPeriodStatusForming, // черновик — excluded from progress
	}
	data := &PeriodData{PeriodID: 7, Teams: teams, GoalsByTeam: goalsByTeam, Statuses: statuses}

	ov := computePeriodOverview(data, 0, nil)
	// Both teams have goals, but only the working team feeds AvgProgress.
	if ov.Summary.TeamsWithGoals != 2 {
		t.Fatalf("teams_with_goals: want 2, got %d", ov.Summary.TeamsWithGoals)
	}
	if ov.Summary.ProgressTeams != 1 {
		t.Fatalf("progress_teams: want 1 (draft excluded), got %d", ov.Summary.ProgressTeams)
	}
	if ov.Summary.AvgProgress != 40 {
		t.Fatalf("avg_progress: want 40 (working team only, draft's 90 excluded), got %d", ov.Summary.AvgProgress)
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
	ov := computePeriodOverview(data, 0, nil)
	if ov.Summary.ByStatus["in_progress"] != 1 {
		t.Fatalf("validated must bucket as in_progress: %+v", ov.Summary.ByStatus)
	}
}

// A goal shared into a team that has no status row of its own leaves the team at
// no_goals while carrying a goal. It must bucket (and its row must serialize) as
// forming, so the Forming tile count and its drill-down agree.
func TestComputePeriodOverview_GoalsButNoGoalsStatusBucketsForming(t *testing.T) {
	data := &PeriodData{
		PeriodID: 1,
		Teams:    []domain.Team{{ID: 1, Name: "Sharee"}},
		GoalsByTeam: map[int64][]domain.Goal{
			1: {{ID: 1, TeamID: 1, Weight: 100, KeyResults: []domain.KeyResult{numericKR(1, 100, 30)}}},
		},
		Statuses: map[int64]domain.TeamPeriodStatus{}, // no row -> resolves to no_goals
	}
	ov := computePeriodOverview(data, 0, nil)
	if ov.Summary.ByStatus["forming"] != 1 || ov.Summary.ByStatus["no_goals"] != 0 {
		t.Fatalf("by_status: want forming=1,no_goals=0, got %+v", ov.Summary.ByStatus)
	}
	if ov.Summary.TeamsWithGoals != 1 {
		t.Fatalf("teams_with_goals: want 1, got %d", ov.Summary.TeamsWithGoals)
	}
	if len(ov.Teams) != 1 || ov.Teams[0].Status != "forming" {
		t.Fatalf("row status must be forming to match the tile, got %+v", ov.Teams)
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

// healthKR builds a numerical KR with an explicit health status.
func healthKR(id int64, h domain.KRHealthStatus) domain.KeyResult {
	kr := numericKR(id, 100, 0)
	kr.Title = "KR"
	kr.HealthStatus = h
	return kr
}

func TestComputePeriodOverview_HealthBalanceAndKRList(t *testing.T) {
	teams := []domain.Team{{ID: 1, Name: "T1"}}
	goalsByTeam := map[int64][]domain.Goal{
		1: {
			{ID: 10, TeamID: 1, Title: "G1", Weight: 50, KeyResults: []domain.KeyResult{
				healthKR(100, domain.KRHealthAtRisk),
				healthKR(101, domain.KRHealthOnTrack),
			}},
			{ID: 11, TeamID: 1, Title: "G2", Weight: 50, KeyResults: []domain.KeyResult{
				healthKR(102, domain.KRHealthDone),
				healthKR(103, ""), // empty defaults to not_started
			}},
		},
	}
	statuses := map[int64]domain.TeamPeriodStatus{1: domain.TeamPeriodStatusInProgress}
	data := &PeriodData{PeriodID: 7, Teams: teams, GoalsByTeam: goalsByTeam, Statuses: statuses}

	ov := computePeriodOverview(data, 0, nil)

	// Health balance buckets, fixed order: not_started, on_track, at_risk, done.
	h := ov.Balances.Health
	if len(h) != 4 {
		t.Fatalf("expected 4 health buckets, got %d", len(h))
	}
	want := map[string]int{"not_started": 1, "on_track": 1, "at_risk": 1, "done": 1}
	for _, b := range h {
		if b.Count != want[b.Key] {
			t.Fatalf("health bucket %q count = %d, want %d", b.Key, b.Count, want[b.Key])
		}
	}
	if h[0].Key != "not_started" || h[2].Key != "at_risk" {
		t.Fatalf("health bucket order wrong: %+v", h)
	}

	// KR list carries per-KR health + context; empty health normalized to not_started.
	if len(ov.KRs) != 4 {
		t.Fatalf("expected 4 KRs, got %d", len(ov.KRs))
	}
	byID := map[int64]PeriodKRItem{}
	for _, kr := range ov.KRs {
		byID[kr.ID] = kr
	}
	if byID[100].HealthStatus != "at_risk" || byID[100].GoalTitle != "G1" || byID[100].TeamName != "T1" {
		t.Fatalf("KR 100 wrong: %+v", byID[100])
	}
	if byID[103].HealthStatus != "not_started" {
		t.Fatalf("empty health should normalize to not_started, got %q", byID[103].HealthStatus)
	}
}
