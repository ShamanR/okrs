package service

import (
	"testing"
	"time"

	"okrs/internal/domain"
)

func teamPtr(id int64) *int64 { return &id }
func timePtr(t time.Time) *time.Time { return &t }
func strPtr(s string) *string { return &s }

func makeTeam(id int64, name, lead string, parentID *int64) domain.Team {
	return domain.Team{ID: id, Name: name, Lead: lead, ParentID: parentID}
}

func makeGoal(id, teamID int64, ownerText string, krs []domain.KeyResult) domain.Goal {
	return domain.Goal{ID: id, TeamID: teamID, OwnerText: ownerText, KeyResults: krs, Weight: 100}
}

func makeTeamWithUDID(id int64, name string, leadUDID *string, parentID *int64) domain.Team {
	return domain.Team{ID: id, Name: name, LeadUDID: leadUDID, ParentID: parentID}
}

func makeGoalWithUDIDs(id, teamID int64, ownerUDIDs []string, krs []domain.KeyResult) domain.Goal {
	return domain.Goal{ID: id, TeamID: teamID, OwnerUDIDs: ownerUDIDs, KeyResults: krs, Weight: 100}
}

func TestComputeScope_LeadUDIDGetsSubtree(t *testing.T) {
	teams := []domain.Team{
		makeTeamWithUDID(1, "Root", strPtr("udid-alice"), nil),
		makeTeamWithUDID(2, "Child", nil, teamPtr(1)),
		makeTeamWithUDID(3, "Grandchild", nil, teamPtr(2)),
		makeTeamWithUDID(4, "Other", strPtr("udid-bob"), nil),
	}
	goals := map[int64][]domain.Goal{}
	ids := computeScope(teams, goals, "udid-alice")
	got := toSet(ids)
	if !got[1] || !got[2] || !got[3] {
		t.Errorf("expected IDs 1,2,3; got %v", ids)
	}
	if got[4] {
		t.Errorf("team 4 should not be in scope")
	}
}

func TestComputeScope_OwnerUDIDGetsOnlyOwnerTeam(t *testing.T) {
	teams := []domain.Team{
		makeTeamWithUDID(10, "Team A", nil, nil),
		makeTeamWithUDID(11, "Team B", nil, teamPtr(10)),
	}
	goals := map[int64][]domain.Goal{
		10: {makeGoalWithUDIDs(1, 10, []string{"udid-alice", "udid-bob"}, nil)},
	}
	ids := computeScope(teams, goals, "udid-alice")
	got := toSet(ids)
	if !got[10] {
		t.Errorf("expected team 10 in owner scope")
	}
	if got[11] {
		t.Errorf("team 11 should NOT be in owner scope (no descendants)")
	}
}

func TestComputeScope_EmptyWhenNoUDIDMatch(t *testing.T) {
	teams := []domain.Team{makeTeamWithUDID(1, "T", strPtr("udid-bob"), nil)}
	goals := map[int64][]domain.Goal{}
	if computeScope(teams, goals, "udid-alice") != nil {
		t.Error("expected nil scope for non-matching UDID")
	}
}

func TestComputeScope_EmptyOwnerUDIDsNoScope(t *testing.T) {
	teams := []domain.Team{makeTeamWithUDID(1, "T", nil, nil)}
	goals := map[int64][]domain.Goal{
		1: {makeGoalWithUDIDs(1, 1, nil, nil)},
	}
	if computeScope(teams, goals, "udid-alice") != nil {
		t.Error("expected nil scope when OwnerUDIDs is empty")
	}
}

func makePeriodData(teams []domain.Team, goals map[int64][]domain.Goal, statuses map[int64]domain.TeamPeriodStatus) *PeriodData {
	now := time.Now()
	return &PeriodData{
		PeriodID: 1,
		Period: domain.Period{
			ID:        1,
			StartDate: now.AddDate(0, -1, 0),
			EndDate:   now.AddDate(0, 1, 0),
		},
		Teams:       teams,
		GoalsByTeam: goals,
		Statuses:    statuses,
	}
}

func makeCfg() HealthCheckInConfig {
	return HealthCheckInConfig{
		StaleDays: 7, BehindMargin: 10, WeightTolerance: 0,
		InCounter: map[string]bool{
			"stale": true, "no_goals": true,
			"awaiting_validation": true, "formation_errors": true, "lagging": false,
		},
	}
}

func TestCategories_NoGoals(t *testing.T) {
	teams := []domain.Team{makeTeam(1, "T1", "Alice", nil)}
	goals := map[int64][]domain.Goal{}
	statuses := map[int64]domain.TeamPeriodStatus{}
	data := makePeriodData(teams, goals, statuses)
	result := computeCategories(data, []int64{1}, makeCfg(), time.Now())
	if result.Categories["no_goals"].Count != 1 {
		t.Errorf("expected 1 no_goals, got %d", result.Categories["no_goals"].Count)
	}
	if result.TotalProblems != 1 {
		t.Errorf("expected total 1, got %d", result.TotalProblems)
	}
}

func TestCategories_StaleGoal(t *testing.T) {
	old := time.Now().AddDate(0, 0, -10)
	kr := domain.KeyResult{ID: 1, GoalID: 1, Title: "KR1", Weight: 100, Kind: domain.KRKindBoolean,
		Boolean: &domain.KRBoolean{}, ProgressUpdatedAt: timePtr(old)}
	g := makeGoal(1, 1, "", []domain.KeyResult{kr})
	g.Weight = 100

	teams := []domain.Team{makeTeam(1, "T1", "", nil)}
	goals := map[int64][]domain.Goal{1: {g}}
	statuses := map[int64]domain.TeamPeriodStatus{1: domain.TeamPeriodStatusInProgress}
	data := makePeriodData(teams, goals, statuses)
	result := computeCategories(data, []int64{1}, makeCfg(), time.Now())

	if result.Categories["stale"].Count != 1 {
		t.Errorf("expected 1 stale, got %d", result.Categories["stale"].Count)
	}
}

func TestCategories_StaleGoal_ExcludesDraftsAndAwaitingValidation(t *testing.T) {
	old := time.Now().AddDate(0, 0, -10)
	for _, status := range []domain.TeamPeriodStatus{domain.TeamPeriodStatusForming, domain.TeamPeriodStatusReady} {
		kr := domain.KeyResult{ID: 1, GoalID: 1, Title: "KR1", Weight: 100, Kind: domain.KRKindBoolean,
			Boolean: &domain.KRBoolean{}, ProgressUpdatedAt: timePtr(old)}
		g := makeGoal(1, 1, "", []domain.KeyResult{kr})
		g.Weight = 100

		teams := []domain.Team{makeTeam(1, "T1", "", nil)}
		goals := map[int64][]domain.Goal{1: {g}}
		statuses := map[int64]domain.TeamPeriodStatus{1: status}
		data := makePeriodData(teams, goals, statuses)
		result := computeCategories(data, []int64{1}, makeCfg(), time.Now())

		if result.Categories["stale"].Count != 0 {
			t.Errorf("status %s: stale warning must be suppressed, got %d", status, result.Categories["stale"].Count)
		}
	}
}

// Stale ("N дней без обновления") is an execution-phase signal: it must be
// counted only while the team is in_progress. Closed periods and teams that
// have no status row yet (not advanced to in_progress) must not be flagged.
func TestCategories_StaleGoal_OnlyInProgress(t *testing.T) {
	old := time.Now().AddDate(0, 0, -10)
	makeData := func(statuses map[int64]domain.TeamPeriodStatus) *PeriodData {
		kr := domain.KeyResult{ID: 1, GoalID: 1, Title: "KR1", Weight: 100, Kind: domain.KRKindBoolean,
			Boolean: &domain.KRBoolean{}, ProgressUpdatedAt: timePtr(old)}
		g := makeGoal(1, 1, "", []domain.KeyResult{kr})
		g.Weight = 100
		teams := []domain.Team{makeTeam(1, "T1", "", nil)}
		goals := map[int64][]domain.Goal{1: {g}}
		return makePeriodData(teams, goals, statuses)
	}

	// Closed period: "нет обновлений" is not an actionable problem.
	closed := computeCategories(makeData(map[int64]domain.TeamPeriodStatus{1: domain.TeamPeriodStatusClosed}), []int64{1}, makeCfg(), time.Now())
	if closed.Categories["stale"].Count != 0 {
		t.Errorf("closed: stale must be suppressed, got %d", closed.Categories["stale"].Count)
	}

	// Team without a status row (never advanced to in_progress): must not flag.
	missing := computeCategories(makeData(map[int64]domain.TeamPeriodStatus{}), []int64{1}, makeCfg(), time.Now())
	if missing.Categories["stale"].Count != 0 {
		t.Errorf("missing status: stale must be suppressed, got %d", missing.Categories["stale"].Count)
	}
}

func TestCategories_AwaitingValidation(t *testing.T) {
	kr := domain.KeyResult{ID: 1, GoalID: 1, Title: "KR", Weight: 100, Kind: domain.KRKindBoolean,
		Boolean: &domain.KRBoolean{}, ProgressUpdatedAt: timePtr(time.Now())}
	g := makeGoal(1, 1, "", []domain.KeyResult{kr})
	g.Weight = 100

	teams := []domain.Team{makeTeam(1, "T1", "", nil)}
	goals := map[int64][]domain.Goal{1: {g}}
	statuses := map[int64]domain.TeamPeriodStatus{1: domain.TeamPeriodStatusReady}
	data := makePeriodData(teams, goals, statuses)
	result := computeCategories(data, []int64{1}, makeCfg(), time.Now())

	if result.Categories["awaiting_validation"].Count != 1 {
		t.Errorf("expected 1 awaiting_validation, got %d", result.Categories["awaiting_validation"].Count)
	}
}

func TestCategories_AwaitingValidation_ExcludesDrafts(t *testing.T) {
	kr := domain.KeyResult{ID: 1, GoalID: 1, Title: "KR", Weight: 100, Kind: domain.KRKindBoolean,
		Boolean: &domain.KRBoolean{}, ProgressUpdatedAt: timePtr(time.Now())}
	g := makeGoal(1, 1, "", []domain.KeyResult{kr})
	g.Weight = 100

	teams := []domain.Team{makeTeam(1, "T1", "", nil)}
	goals := map[int64][]domain.Goal{1: {g}}
	statuses := map[int64]domain.TeamPeriodStatus{1: domain.TeamPeriodStatusForming}
	data := makePeriodData(teams, goals, statuses)
	result := computeCategories(data, []int64{1}, makeCfg(), time.Now())

	if result.Categories["awaiting_validation"].Count != 0 {
		t.Errorf("draft (forming) teams must not be awaiting_validation, got %d", result.Categories["awaiting_validation"].Count)
	}
}

func TestCategories_FormationError_WeightSum(t *testing.T) {
	kr := domain.KeyResult{ID: 1, GoalID: 1, Title: "KR", Weight: 100, Kind: domain.KRKindBoolean,
		Boolean: &domain.KRBoolean{}, ProgressUpdatedAt: timePtr(time.Now())}
	g1 := makeGoal(1, 1, "", []domain.KeyResult{kr})
	g1.Weight = 60
	g2 := makeGoal(2, 1, "", []domain.KeyResult{kr})
	g2.Weight = 60

	teams := []domain.Team{makeTeam(1, "T1", "", nil)}
	goals := map[int64][]domain.Goal{1: {g1, g2}}
	statuses := map[int64]domain.TeamPeriodStatus{1: domain.TeamPeriodStatusInProgress}
	data := makePeriodData(teams, goals, statuses)
	result := computeCategories(data, []int64{1}, makeCfg(), time.Now())

	hasTeamError := false
	for _, item := range result.Categories["formation_errors"].Items {
		if item.EntityType == "team" && item.ErrorType == "weight_sum_not_100" {
			hasTeamError = true
		}
	}
	if !hasTeamError {
		t.Error("expected team-level weight_sum_not_100 error")
	}
}

func TestCategories_FormationError_OnlyForValidationAndInProgress(t *testing.T) {
	kr := domain.KeyResult{ID: 1, GoalID: 1, Title: "KR", Weight: 100, Kind: domain.KRKindBoolean,
		Boolean: &domain.KRBoolean{}, ProgressUpdatedAt: timePtr(time.Now())}
	g1 := makeGoal(1, 1, "", []domain.KeyResult{kr})
	g1.Weight = 60
	g2 := makeGoal(2, 1, "", []domain.KeyResult{kr})
	g2.Weight = 60

	teams := []domain.Team{makeTeam(1, "T1", "", nil)}
	goals := map[int64][]domain.Goal{1: {g1, g2}}

	check := func(status domain.TeamPeriodStatus) int {
		statuses := map[int64]domain.TeamPeriodStatus{1: status}
		data := makePeriodData(teams, goals, statuses)
		result := computeCategories(data, []int64{1}, makeCfg(), time.Now())
		return result.Categories["formation_errors"].Count
	}

	if c := check(domain.TeamPeriodStatusForming); c != 0 {
		t.Errorf("forming (draft) must not report formation errors, got %d", c)
	}
	if c := check(domain.TeamPeriodStatusClosed); c != 0 {
		t.Errorf("closed must not report formation errors, got %d", c)
	}
	if c := check(domain.TeamPeriodStatusReady); c == 0 {
		t.Error("ready (К валидации) must report formation errors")
	}
	if c := check(domain.TeamPeriodStatusInProgress); c == 0 {
		t.Error("in_progress (В работе) must report formation errors")
	}
}

func TestCategories_LaggingGoal(t *testing.T) {
	recent := time.Now().AddDate(0, 0, -1)
	kr := domain.KeyResult{ID: 1, GoalID: 1, Title: "KR", Weight: 100, Kind: domain.KRKindBoolean,
		Boolean: &domain.KRBoolean{IsDone: false}, ProgressUpdatedAt: timePtr(recent)}
	g := makeGoal(1, 1, "", []domain.KeyResult{kr})
	g.Weight = 100
	g.Progress = 0

	teams := []domain.Team{makeTeam(1, "T1", "", nil)}
	goals := map[int64][]domain.Goal{1: {g}}
	statuses := map[int64]domain.TeamPeriodStatus{1: domain.TeamPeriodStatusInProgress}
	now := time.Now()
	data := &PeriodData{
		PeriodID: 1,
		Period: domain.Period{
			StartDate: now.AddDate(0, -4, 0),
			EndDate:   now.AddDate(0, 1, 0),
		},
		Teams: teams, GoalsByTeam: goals, Statuses: statuses,
	}
	result := computeCategories(data, []int64{1}, makeCfg(), now)
	if result.Categories["lagging"].Count != 1 {
		t.Errorf("expected 1 lagging, got %d", result.Categories["lagging"].Count)
	}
	if result.TotalProblems != 0 {
		t.Errorf("lagging should not count toward total; got %d", result.TotalProblems)
	}
}

func TestCalcExpectedPace(t *testing.T) {
	now := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	p := domain.Period{
		StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
	}
	pace := calcExpectedPace(p, now)
	if pace < 20 || pace > 35 {
		t.Errorf("expected pace ~25, got %d", pace)
	}
}

func toSet(ids []int64) map[int64]bool {
	m := make(map[int64]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}
