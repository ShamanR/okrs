package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"okrs/internal/core/domain"
)

func teamPtr(id int64) *int64        { return &id }
func timePtr(t time.Time) *time.Time { return &t }
func strPtr(s string) *string        { return &s }

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
		CommentDepth: 1, ResolvedCommentsLimit: 5,
		InCounter: map[string]bool{
			"stale": true, "no_goals": true,
			"awaiting_validation": true, "formation_errors": true, "lagging": false,
			"comments": false,
		},
	}
}

func TestCategories_NoGoals(t *testing.T) {
	teams := []domain.Team{makeTeam(1, "T1", "Alice", nil)}
	goals := map[int64][]domain.Goal{}
	statuses := map[int64]domain.TeamPeriodStatus{}
	data := makePeriodData(teams, goals, statuses)
	result := computeCategories(data, []int64{1}, "", makeCfg(), time.Now())
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
	result := computeCategories(data, []int64{1}, "", makeCfg(), time.Now())

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
		result := computeCategories(data, []int64{1}, "", makeCfg(), time.Now())

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
	closed := computeCategories(makeData(map[int64]domain.TeamPeriodStatus{1: domain.TeamPeriodStatusClosed}), []int64{1}, "", makeCfg(), time.Now())
	if closed.Categories["stale"].Count != 0 {
		t.Errorf("closed: stale must be suppressed, got %d", closed.Categories["stale"].Count)
	}

	// Team without a status row (never advanced to in_progress): must not flag.
	missing := computeCategories(makeData(map[int64]domain.TeamPeriodStatus{}), []int64{1}, "", makeCfg(), time.Now())
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
	result := computeCategories(data, []int64{1}, "", makeCfg(), time.Now())

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
	result := computeCategories(data, []int64{1}, "", makeCfg(), time.Now())

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
	result := computeCategories(data, []int64{1}, "", makeCfg(), time.Now())

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
		result := computeCategories(data, []int64{1}, "", makeCfg(), time.Now())
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
	result := computeCategories(data, []int64{1}, "", makeCfg(), now)
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

func TestCategories_NeverUpdated_NotStaleBeforeThreshold(t *testing.T) {
	// Период начался 3 дня назад; порог 7 дней; цель без обновлений прогресса.
	now := time.Now()
	kr := domain.KeyResult{ID: 1, GoalID: 1, Title: "KR1", Weight: 100, Kind: domain.KRKindBoolean,
		Boolean: &domain.KRBoolean{}} // ProgressUpdatedAt == nil
	g := makeGoal(1, 1, "", []domain.KeyResult{kr})
	teams := []domain.Team{makeTeam(1, "T1", "", nil)}
	goals := map[int64][]domain.Goal{1: {g}}
	statuses := map[int64]domain.TeamPeriodStatus{1: domain.TeamPeriodStatusInProgress}
	data := &PeriodData{
		PeriodID: 1,
		Period:   domain.Period{ID: 1, StartDate: now.AddDate(0, 0, -3), EndDate: now.AddDate(0, 1, 0)},
		Teams:    teams, GoalsByTeam: goals, Statuses: statuses,
	}
	result := computeCategories(data, []int64{1}, "", makeCfg(), now)
	if result.Categories["stale"].Count != 0 {
		t.Fatalf("expected 0 stale (3 days < 7 threshold from period start), got %d", result.Categories["stale"].Count)
	}
}

func TestCategories_NeverUpdated_StaleAfterThresholdFromPeriodStart(t *testing.T) {
	// Период начался 10 дней назад; порог 7; цель без обновлений → stale, days_since_update == 10.
	now := time.Now()
	kr := domain.KeyResult{ID: 1, GoalID: 1, Title: "KR1", Weight: 100, Kind: domain.KRKindBoolean,
		Boolean: &domain.KRBoolean{}}
	g := makeGoal(1, 1, "", []domain.KeyResult{kr})
	teams := []domain.Team{makeTeam(1, "T1", "", nil)}
	goals := map[int64][]domain.Goal{1: {g}}
	statuses := map[int64]domain.TeamPeriodStatus{1: domain.TeamPeriodStatusInProgress}
	data := &PeriodData{
		PeriodID: 1,
		Period:   domain.Period{ID: 1, StartDate: now.AddDate(0, 0, -10), EndDate: now.AddDate(0, 1, 0)},
		Teams:    teams, GoalsByTeam: goals, Statuses: statuses,
	}
	result := computeCategories(data, []int64{1}, "", makeCfg(), now)
	if result.Categories["stale"].Count != 1 {
		t.Fatalf("expected 1 stale (10 days > 7 from period start), got %d", result.Categories["stale"].Count)
	}
	if got := result.Categories["stale"].Items[0].DaysSinceUpdate; got != 10 {
		t.Fatalf("expected days_since_update 10 (from period start), got %d", got)
	}
}

func TestCategories_NeverUpdated_FuturePeriod_NotStale(t *testing.T) {
	// Период ещё не начался (StartDate в будущем) → не stale, даже при in_progress.
	now := time.Now()
	kr := domain.KeyResult{ID: 1, GoalID: 1, Title: "KR1", Weight: 100, Kind: domain.KRKindBoolean,
		Boolean: &domain.KRBoolean{}}
	g := makeGoal(1, 1, "", []domain.KeyResult{kr})
	teams := []domain.Team{makeTeam(1, "T1", "", nil)}
	goals := map[int64][]domain.Goal{1: {g}}
	statuses := map[int64]domain.TeamPeriodStatus{1: domain.TeamPeriodStatusInProgress}
	data := &PeriodData{
		PeriodID: 1,
		Period:   domain.Period{ID: 1, StartDate: now.AddDate(0, 0, 5), EndDate: now.AddDate(0, 2, 0)},
		Teams:    teams, GoalsByTeam: goals, Statuses: statuses,
	}
	result := computeCategories(data, []int64{1}, "", makeCfg(), now)
	if result.Categories["stale"].Count != 0 {
		t.Fatalf("expected 0 stale (future period), got %d", result.Categories["stale"].Count)
	}
}

func TestCategories_Comments_UnresolvedInScope(t *testing.T) {
	now := time.Now()
	openC := domain.GoalComment{ID: 100, GoalID: 10, Text: "уточни KR", AuthorName: "Bob", AuthorUDID: "udid-bob", CreatedAt: now}
	g := domain.Goal{ID: 10, TeamID: 1, Title: "Цель", Weight: 100, Comments: []domain.GoalComment{openC}}
	teams := []domain.Team{makeTeamWithUDID(1, "T1", strPtr("udid-alice"), nil)}
	data := &PeriodData{
		PeriodID:    1,
		Period:      domain.Period{ID: 1, StartDate: now.AddDate(0, -1, 0), EndDate: now.AddDate(0, 1, 0)},
		Teams:       teams,
		GoalsByTeam: map[int64][]domain.Goal{1: {g}},
		Statuses:    map[int64]domain.TeamPeriodStatus{1: domain.TeamPeriodStatusInProgress},
	}
	cfg := makeCfg()
	cfg.CommentDepth = 1
	cfg.ResolvedCommentsLimit = 5
	res := computeCategories(data, []int64{1}, "udid-alice", cfg, now)
	comments := res.Categories["comments"]
	if len(comments.Unresolved) != 1 || comments.Unresolved[0].CommentID != 100 {
		t.Fatalf("expected 1 unresolved comment 100, got %+v", comments.Unresolved)
	}
	if comments.Count != 1 {
		t.Fatalf("expected comments.Count == unresolved len 1, got %d", comments.Count)
	}
}

func TestCategories_Comments_ResolvedMineExcludesSelfResolved(t *testing.T) {
	now := time.Now()
	r1 := now.AddDate(0, 0, -1)
	r2 := now.AddDate(0, 0, -2)
	// Мой коммент, решён Bob → в список.
	mineResolvedByOther := domain.GoalComment{ID: 200, GoalID: 10, Text: "готово", AuthorUDID: "udid-alice", ResolvedAt: &r1, ResolvedByUDID: "udid-bob"}
	// Мой коммент, решён мной → исключить.
	mineSelfResolved := domain.GoalComment{ID: 201, GoalID: 10, Text: "сам", AuthorUDID: "udid-alice", ResolvedAt: &r2, ResolvedByUDID: "udid-alice"}
	// Чужой коммент → не мой, исключить.
	othersResolved := domain.GoalComment{ID: 202, GoalID: 10, Text: "чужой", AuthorUDID: "udid-bob", ResolvedAt: &r1, ResolvedByUDID: "udid-carol"}
	g := domain.Goal{ID: 10, TeamID: 1, Title: "Цель", Weight: 100,
		Comments: []domain.GoalComment{mineResolvedByOther, mineSelfResolved, othersResolved}}
	teams := []domain.Team{makeTeamWithUDID(1, "T1", nil, nil)} // не lead — resolved-mine не зависит от scope
	data := &PeriodData{
		PeriodID:    1,
		Period:      domain.Period{ID: 1, StartDate: now.AddDate(0, -1, 0), EndDate: now.AddDate(0, 1, 0)},
		Teams:       teams,
		GoalsByTeam: map[int64][]domain.Goal{1: {g}},
		Statuses:    map[int64]domain.TeamPeriodStatus{1: domain.TeamPeriodStatusInProgress},
	}
	cfg := makeCfg()
	res := computeCategories(data, []int64{1}, "udid-alice", cfg, now)
	resolved := res.Categories["comments"].Resolved
	if len(resolved) != 1 || resolved[0].CommentID != 200 {
		t.Fatalf("expected only comment 200 in resolved-mine, got %+v", resolved)
	}
}

func TestCategories_Comments_ResolvedMineLimitK(t *testing.T) {
	now := time.Now()
	var comments []domain.GoalComment
	for i := int64(0); i < 8; i++ {
		ts := now.AddDate(0, 0, -int(i)) // новее = меньший i
		comments = append(comments, domain.GoalComment{
			ID: 300 + i, GoalID: 10, Text: "c", AuthorUDID: "udid-alice", ResolvedAt: &ts, ResolvedByUDID: "udid-bob",
		})
	}
	g := domain.Goal{ID: 10, TeamID: 1, Title: "Цель", Weight: 100, Comments: comments}
	teams := []domain.Team{makeTeamWithUDID(1, "T1", nil, nil)}
	data := &PeriodData{
		PeriodID:    1,
		Period:      domain.Period{ID: 1, StartDate: now.AddDate(0, -1, 0), EndDate: now.AddDate(0, 1, 0)},
		Teams:       teams,
		GoalsByTeam: map[int64][]domain.Goal{1: {g}},
		Statuses:    map[int64]domain.TeamPeriodStatus{1: domain.TeamPeriodStatusInProgress},
	}
	cfg := makeCfg()
	cfg.ResolvedCommentsLimit = 5
	res := computeCategories(data, []int64{1}, "udid-alice", cfg, now)
	resolved := res.Categories["comments"].Resolved
	if len(resolved) != 5 {
		t.Fatalf("expected limit 5, got %d", len(resolved))
	}
	// Отсортировано по resolved_at DESC → первым самый новый (id 300).
	if resolved[0].CommentID != 300 {
		t.Fatalf("expected newest (300) first, got %d", resolved[0].CommentID)
	}
}

func TestCategories_Comments_SharedGoalCountedOnce(t *testing.T) {
	// Goal 10 owned by team 1, расшарена в team 2; я лид обеих (обе в comment scope).
	// ListGoalsByTeamsPeriod кладёт копию цели с TeamID = видимой команды под КАЖДУЮ команду,
	// поэтому один и тот же комментарий не должен считаться дважды.
	now := time.Now()
	open := domain.GoalComment{ID: 500, GoalID: 10, Text: "нерешённый", AuthorName: "Bob", AuthorUDID: "udid-bob", CreatedAt: now}
	rt := now.AddDate(0, 0, -1)
	mineResolved := domain.GoalComment{ID: 501, GoalID: 10, Text: "мой решённый", AuthorUDID: "udid-alice", ResolvedAt: &rt, ResolvedByUDID: "udid-bob"}
	gOwner := domain.Goal{ID: 10, TeamID: 1, Title: "Общая цель", Weight: 100, Comments: []domain.GoalComment{open, mineResolved}}
	gShared := domain.Goal{ID: 10, TeamID: 2, Title: "Общая цель", Weight: 100, Comments: []domain.GoalComment{open, mineResolved}}
	teams := []domain.Team{
		makeTeamWithUDID(1, "Owner", strPtr("udid-alice"), nil),
		makeTeamWithUDID(2, "SharedInto", strPtr("udid-alice"), nil),
	}
	data := &PeriodData{
		PeriodID:    1,
		Period:      domain.Period{ID: 1, StartDate: now.AddDate(0, -1, 0), EndDate: now.AddDate(0, 1, 0)},
		Teams:       teams,
		GoalsByTeam: map[int64][]domain.Goal{1: {gOwner}, 2: {gShared}},
		Statuses:    map[int64]domain.TeamPeriodStatus{1: domain.TeamPeriodStatusInProgress, 2: domain.TeamPeriodStatusInProgress},
	}
	cfg := makeCfg()
	res := computeCategories(data, []int64{1, 2}, "udid-alice", cfg, now)
	comments := res.Categories["comments"]
	if len(comments.Unresolved) != 1 {
		t.Fatalf("shared-goal unresolved comment must be counted once, got %d", len(comments.Unresolved))
	}
	if len(comments.Resolved) != 1 {
		t.Fatalf("shared-goal resolved-mine comment must be counted once, got %d", len(comments.Resolved))
	}
}

func TestComputeCommentScope_LeadDepth(t *testing.T) {
	// 1(lead alice) → 2 → 3 → 4 ; owner-only команда 5.
	teams := []domain.Team{
		makeTeamWithUDID(1, "L0", strPtr("udid-alice"), nil),
		makeTeamWithUDID(2, "L1", nil, teamPtr(1)),
		makeTeamWithUDID(3, "L2", nil, teamPtr(2)),
		makeTeamWithUDID(4, "L3", nil, teamPtr(3)),
		makeTeamWithUDID(5, "Owner", nil, nil),
	}
	goals := map[int64][]domain.Goal{
		5: {makeGoalWithUDIDs(50, 5, []string{"udid-alice"}, nil)},
	}

	depth0 := computeCommentScope(teams, goals, "udid-alice", 0)
	if _, ok := depth0[1]; !ok {
		t.Error("depth0 must include lead team 1")
	}
	if _, ok := depth0[2]; ok {
		t.Error("depth0 must NOT include child team 2")
	}
	if _, ok := depth0[5]; !ok {
		t.Error("depth0 must include owner team 5")
	}

	depth1 := computeCommentScope(teams, goals, "udid-alice", 1)
	if _, ok := depth1[2]; !ok {
		t.Error("depth1 must include direct child 2")
	}
	if _, ok := depth1[3]; ok {
		t.Error("depth1 must NOT include grandchild 3")
	}

	depth2 := computeCommentScope(teams, goals, "udid-alice", 2)
	if _, ok := depth2[3]; !ok {
		t.Error("depth2 must include team 3")
	}
	if _, ok := depth2[4]; ok {
		t.Error("depth2 must NOT include team 4")
	}
}

func TestComputeCommentScope_EmptyUser(t *testing.T) {
	teams := []domain.Team{makeTeamWithUDID(1, "L0", strPtr("udid-alice"), nil)}
	if s := computeCommentScope(teams, map[int64][]domain.Goal{}, "", 1); len(s) != 0 {
		t.Errorf("empty user → empty scope, got %d", len(s))
	}
}

type stubSettingsReader struct {
	raw json.RawMessage
	err error
}

func (s stubSettingsReader) GetTenant(_ context.Context, _ domain.TenantScope, _ string) (json.RawMessage, error) {
	return s.raw, s.err
}

func TestLoadConfig_CommentDefaults(t *testing.T) {
	cfg, err := LoadHealthCheckInConfig(context.Background(), domain.TenantScope{TenantID: 1}, stubSettingsReader{raw: nil})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CommentDepth != 1 {
		t.Errorf("expected default comment_depth 1, got %d", cfg.CommentDepth)
	}
	if cfg.ResolvedCommentsLimit != 5 {
		t.Errorf("expected default resolved_comments_limit 5, got %d", cfg.ResolvedCommentsLimit)
	}
	if cfg.InCounter["comments"] {
		t.Errorf("expected in_counter[comments] default false")
	}
}

func TestLoadProgressSnapshotIntervalDays(t *testing.T) {
	// Unset → default 1.
	if got := LoadProgressSnapshotIntervalDays(context.Background(), domain.TenantScope{TenantID: 1}, stubSettingsReader{raw: nil}); got != 1 {
		t.Fatalf("unset: want 1, got %d", got)
	}
	// Configured value honored.
	if got := LoadProgressSnapshotIntervalDays(context.Background(), domain.TenantScope{TenantID: 1}, stubSettingsReader{raw: json.RawMessage(`7`)}); got != 7 {
		t.Fatalf("configured: want 7, got %d", got)
	}
	// Invalid (<1) → default 1.
	if got := LoadProgressSnapshotIntervalDays(context.Background(), domain.TenantScope{TenantID: 1}, stubSettingsReader{raw: json.RawMessage(`0`)}); got != 1 {
		t.Fatalf("invalid: want 1, got %d", got)
	}
}

func TestLoadConfig_NormalizesInvalidCommentFields(t *testing.T) {
	raw := json.RawMessage(`{"comment_depth":-2,"resolved_comments_limit":0,"stale_days":7,"cache_ttl_minutes":5,"green_threshold":80}`)
	cfg, _ := LoadHealthCheckInConfig(context.Background(), domain.TenantScope{TenantID: 1}, stubSettingsReader{raw: raw})
	if cfg.CommentDepth != 0 {
		t.Errorf("negative comment_depth should clamp to 0, got %d", cfg.CommentDepth)
	}
	if cfg.ResolvedCommentsLimit != 5 {
		t.Errorf("non-positive resolved_comments_limit should reset to default 5, got %d", cfg.ResolvedCommentsLimit)
	}
}

func toSet(ids []int64) map[int64]bool {
	m := make(map[int64]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}
