package service

import (
	"context"
	"testing"
	"time"

	"okrs/internal/domain"
	"okrs/internal/store"
)

type fakeStore struct {
	teams          []domain.Team
	periods        []domain.Period
	currentPeriod  domain.Period
	goalsByTeam    map[int64]map[int64][]domain.Goal
	statuses       map[[2]int64]domain.TeamPeriodStatus
	keyResults     map[int64]domain.KeyResult
	percentUpdates map[int64]float64
	linearUpdates  map[int64]float64
	booleanUpdates map[int64]bool
	projectStages  map[int64][]domain.KRProjectStage
	stageUpdates   map[int64]bool
	movedGoals     map[int64]int
	movedKRs       map[int64]int
	softDeleted    []int64
	restored       []int64
	hardDeleted    []int64
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		goalsByTeam:    make(map[int64]map[int64][]domain.Goal),
		statuses:       make(map[[2]int64]domain.TeamPeriodStatus),
		keyResults:     make(map[int64]domain.KeyResult),
		percentUpdates: make(map[int64]float64),
		linearUpdates:  make(map[int64]float64),
		booleanUpdates: make(map[int64]bool),
		projectStages:  make(map[int64][]domain.KRProjectStage),
		stageUpdates:   make(map[int64]bool),
		movedGoals:     make(map[int64]int),
		movedKRs:       make(map[int64]int),
	}
}

func (f *fakeStore) ListTeams(context.Context) ([]domain.Team, error) {
	items := make([]domain.Team, 0, len(f.teams))
	for _, team := range f.teams {
		if team.DeletedAt == nil {
			items = append(items, team)
		}
	}
	return items, nil
}
func (f *fakeStore) ListDeletedTeams(context.Context) ([]domain.Team, error) {
	items := make([]domain.Team, 0, len(f.teams))
	for _, team := range f.teams {
		if team.DeletedAt != nil {
			items = append(items, team)
		}
	}
	return items, nil
}
func (f *fakeStore) ListAllTeams(context.Context) ([]domain.Team, error) { return f.teams, nil }
func (f *fakeStore) GetTeam(_ context.Context, id int64) (domain.Team, error) {
	for _, team := range f.teams {
		if team.ID == id {
			return team, nil
		}
	}
	return domain.Team{}, nil
}
func (f *fakeStore) ListPeriods(context.Context) ([]domain.Period, error) { return f.periods, nil }
func (f *fakeStore) GetPeriod(_ context.Context, id int64) (domain.Period, error) {
	for _, period := range f.periods {
		if period.ID == id {
			return period, nil
		}
	}
	return domain.Period{}, nil
}
func (f *fakeStore) FindPeriodForDate(context.Context, time.Time) (domain.Period, error) {
	return f.currentPeriod, nil
}
func (f *fakeStore) ListGoalsByTeamPeriod(_ context.Context, teamID, periodID int64) ([]domain.Goal, error) {
	if f.goalsByTeam[teamID] == nil {
		return nil, nil
	}
	return f.goalsByTeam[teamID][periodID], nil
}
func (f *fakeStore) ListGoalShares(context.Context, int64) ([]store.GoalShare, error) {
	return nil, nil
}
func (f *fakeStore) GetTeamPeriodStatus(_ context.Context, teamID, periodID int64) (domain.TeamPeriodStatus, error) {
	if status, ok := f.statuses[[2]int64{teamID, periodID}]; ok {
		return status, nil
	}
	return domain.TeamPeriodStatusNoGoals, nil
}
func (f *fakeStore) TeamHasGoals(_ context.Context, id int64) (bool, error) {
	for _, goals := range f.goalsByTeam[id] {
		if len(goals) > 0 {
			return true, nil
		}
	}
	return false, nil
}
func (f *fakeStore) TeamHasGoalsInPeriod(_ context.Context, id, periodID int64) (bool, error) {
	return len(f.goalsByTeam[id][periodID]) > 0, nil
}
func (f *fakeStore) SoftDeleteTeam(_ context.Context, id int64) error {
	f.softDeleted = append(f.softDeleted, id)
	return nil
}
func (f *fakeStore) RestoreTeam(_ context.Context, id int64) error {
	f.restored = append(f.restored, id)
	return nil
}
func (f *fakeStore) HardDeleteTeam(_ context.Context, id int64) error {
	f.hardDeleted = append(f.hardDeleted, id)
	return nil
}
func (f *fakeStore) UpdatePercentCurrent(_ context.Context, krID int64, current float64) error {
	f.percentUpdates[krID] = current
	return nil
}
func (f *fakeStore) UpdateLinearCurrent(_ context.Context, krID int64, current float64) error {
	f.linearUpdates[krID] = current
	return nil
}
func (f *fakeStore) UpdateBoolean(_ context.Context, krID int64, done bool) error {
	f.booleanUpdates[krID] = done
	return nil
}
func (f *fakeStore) ListProjectStages(_ context.Context, krID int64) ([]domain.KRProjectStage, error) {
	return f.projectStages[krID], nil
}
func (f *fakeStore) UpdateProjectStageDone(_ context.Context, stageID int64, done bool) error {
	f.stageUpdates[stageID] = done
	return nil
}
func (f *fakeStore) ReplaceGoalShares(context.Context, int64, []store.GoalShareInput) error {
	return nil
}
func (f *fakeStore) UpdateGoalTeamWeight(context.Context, int64, int64, int) error { return nil }
func (f *fakeStore) GetKeyResult(_ context.Context, id int64) (domain.KeyResult, error) {
	return f.keyResults[id], nil
}
func (f *fakeStore) AddGoalComment(context.Context, int64, string) error      { return nil }
func (f *fakeStore) AddKeyResultComment(context.Context, int64, string) error { return nil }
func (f *fakeStore) GetGoal(context.Context, int64) (domain.Goal, error)      { return domain.Goal{}, nil }
func (f *fakeStore) UpdateGoal(context.Context, store.GoalUpdateInput) error  { return nil }
func (f *fakeStore) CreateKeyResult(context.Context, store.KeyResultInput) (int64, error) {
	return 0, nil
}
func (f *fakeStore) UpdateKeyResult(context.Context, store.KeyResultUpdateInput) error { return nil }
func (f *fakeStore) MoveGoal(_ context.Context, goalID int64, direction int) error {
	f.movedGoals[goalID] = direction
	return nil
}
func (f *fakeStore) MoveKeyResult(_ context.Context, krID int64, direction int) error {
	f.movedKRs[krID] = direction
	return nil
}
func (f *fakeStore) UpsertPercentMeta(context.Context, store.PercentMetaInput) error { return nil }
func (f *fakeStore) UpsertLinearMeta(context.Context, store.LinearMetaInput) error   { return nil }
func (f *fakeStore) UpsertBooleanMeta(context.Context, int64, bool) error            { return nil }
func (f *fakeStore) ReplaceProjectStages(context.Context, int64, []store.ProjectStageInput) error {
	return nil
}
func (f *fakeStore) SetTeamPeriodStatus(context.Context, int64, int64, domain.TeamPeriodStatus) error {
	return nil
}

func TestUpdateKRProgressPercent(t *testing.T) {
	store := newFakeStore()
	store.keyResults[1] = domain.KeyResult{ID: 1, Kind: domain.KRKindPercent}
	store.keyResults[2] = domain.KeyResult{ID: 2, Kind: domain.KRKindLinear}
	service := New(store)

	if err := service.UpdateKRProgressPercent(context.Background(), 1, 42); err != nil {
		t.Fatalf("update percent: %v", err)
	}
	if err := service.UpdateKRProgressPercent(context.Background(), 2, 55); err != nil {
		t.Fatalf("update linear: %v", err)
	}
	if store.percentUpdates[1] != 42 {
		t.Fatalf("expected percent update")
	}
	if store.linearUpdates[2] != 55 {
		t.Fatalf("expected linear update")
	}
}

func TestUpdateKRProgressBoolean(t *testing.T) {
	store := newFakeStore()
	store.keyResults[3] = domain.KeyResult{ID: 3, Kind: domain.KRKindBoolean}
	service := New(store)

	if err := service.UpdateKRProgressBoolean(context.Background(), 3, true); err != nil {
		t.Fatalf("update boolean: %v", err)
	}
	if !store.booleanUpdates[3] {
		t.Fatalf("expected boolean update")
	}
}

func TestUpdateKRProgressProject(t *testing.T) {
	store := newFakeStore()
	store.keyResults[4] = domain.KeyResult{ID: 4, Kind: domain.KRKindProject}
	store.projectStages[4] = []domain.KRProjectStage{{ID: 100, IsDone: false}, {ID: 101, IsDone: true}}
	service := New(store)

	updates := []ProjectStageUpdate{{ID: 100, IsDone: true}}
	if err := service.UpdateKRProgressProject(context.Background(), 4, updates); err != nil {
		t.Fatalf("update project: %v", err)
	}
	if !store.stageUpdates[100] {
		t.Fatalf("expected stage update")
	}
}

func TestMoveGoal(t *testing.T) {
	store := newFakeStore()
	service := New(store)

	if err := service.MoveGoal(context.Background(), 10, -1); err != nil {
		t.Fatalf("move goal: %v", err)
	}
	if store.movedGoals[10] != -1 {
		t.Fatalf("expected goal move direction")
	}
}

func TestMoveKeyResult(t *testing.T) {
	store := newFakeStore()
	service := New(store)

	if err := service.MoveKeyResult(context.Background(), 20, 1); err != nil {
		t.Fatalf("move kr: %v", err)
	}
	if store.movedKRs[20] != 1 {
		t.Fatalf("expected key result move direction")
	}
}

func TestDeleteTeamUsesSoftDeleteWhenTeamHasGoals(t *testing.T) {
	store := newFakeStore()
	store.goalsByTeam[10] = map[int64][]domain.Goal{1: {{ID: 1, TeamID: 10, PeriodID: 1}}}
	service := New(store)

	if err := service.DeleteTeam(context.Background(), 10); err != nil {
		t.Fatalf("delete team: %v", err)
	}
	if len(store.softDeleted) != 1 || store.softDeleted[0] != 10 {
		t.Fatalf("expected soft delete for team with goals")
	}
	if len(store.hardDeleted) != 0 {
		t.Fatalf("did not expect hard delete")
	}
}

func TestDeleteTeamUsesHardDeleteWhenTeamHasNoGoals(t *testing.T) {
	store := newFakeStore()
	service := New(store)

	if err := service.DeleteTeam(context.Background(), 10); err != nil {
		t.Fatalf("delete team: %v", err)
	}
	if len(store.hardDeleted) != 1 || store.hardDeleted[0] != 10 {
		t.Fatalf("expected hard delete for team without goals")
	}
}

func TestHardDeleteTeamRejectsTeamsWithGoals(t *testing.T) {
	store := newFakeStore()
	store.goalsByTeam[10] = map[int64][]domain.Goal{1: {{ID: 1, TeamID: 10, PeriodID: 1}}}
	service := New(store)

	if err := service.HardDeleteTeam(context.Background(), 10); err != ErrTeamHasGoals {
		t.Fatalf("expected ErrTeamHasGoals, got %v", err)
	}
}

func TestGetTeamsWithPeriodSummaryShowsOnlyHistoricalTeamsWithGoals(t *testing.T) {
	deletedAt := time.Date(2025, 2, 1, 10, 0, 0, 0, time.UTC)
	store := newFakeStore()
	store.currentPeriod = domain.Period{ID: 2}
	store.teams = []domain.Team{
		{ID: 1, Name: "Active parent", Type: domain.TeamTypeUnit},
		{ID: 2, Name: "Deleted child", Type: domain.TeamTypeTeam, ParentID: ptr(1), DeletedAt: &deletedAt},
		{ID: 3, Name: "New active child", Type: domain.TeamTypeTeam, ParentID: ptr(1)},
	}
	store.goalsByTeam[2] = map[int64][]domain.Goal{1: {{ID: 100, TeamID: 2, PeriodID: 1, Title: "Historic"}}}
	store.statuses[[2]int64{2, 1}] = domain.TeamPeriodStatusClosed
	service := New(store)

	rows, err := service.GetTeamsWithPeriodSummary(context.Background(), 1, nil)
	if err != nil {
		t.Fatalf("get team summaries: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected only historical team with goals, got %d", len(rows))
	}
	if rows[0].ID != 2 {
		t.Fatalf("expected deleted historical team to stay visible")
	}
	if rows[0].Indent != 0 {
		t.Fatalf("expected hidden parent to not reserve indent, got %d", rows[0].Indent)
	}
}

func TestGetTeamsWithPeriodSummaryShowsOnlyActiveTeamsInCurrentPeriod(t *testing.T) {
	deletedAt := time.Date(2025, 2, 1, 10, 0, 0, 0, time.UTC)
	store := newFakeStore()
	store.currentPeriod = domain.Period{ID: 2}
	store.teams = []domain.Team{
		{ID: 1, Name: "Active", Type: domain.TeamTypeUnit},
		{ID: 2, Name: "Deleted", Type: domain.TeamTypeTeam, DeletedAt: &deletedAt},
	}
	service := New(store)

	rows, err := service.GetTeamsWithPeriodSummary(context.Background(), 2, nil)
	if err != nil {
		t.Fatalf("get team summaries: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != 1 {
		t.Fatalf("expected only active team in current period, got %+v", rows)
	}
}

func TestGetTeamsWithPeriodSummaryKeepsDeletedTeamsWithCurrentGoalsVisible(t *testing.T) {
	deletedAt := time.Date(2025, 2, 1, 10, 0, 0, 0, time.UTC)
	store := newFakeStore()
	store.currentPeriod = domain.Period{ID: 2}
	store.teams = []domain.Team{
		{ID: 1, Name: "Active", Type: domain.TeamTypeUnit},
		{ID: 2, Name: "Deleted", Type: domain.TeamTypeTeam, DeletedAt: &deletedAt},
	}
	store.goalsByTeam[2] = map[int64][]domain.Goal{2: {{ID: 200, TeamID: 2, PeriodID: 2, Title: "Current"}}}
	service := New(store)

	rows, err := service.GetTeamsWithPeriodSummary(context.Background(), 2, nil)
	if err != nil {
		t.Fatalf("get team summaries: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected active team and deleted team with current goals, got %+v", rows)
	}
	if rows[1].ID != 2 {
		t.Fatalf("expected deleted team with current goals to remain visible, got %+v", rows)
	}
}

func TestGetTeamOKRAllowsDeletedTeamInHistoricalPeriodWithGoals(t *testing.T) {
	deletedAt := time.Date(2025, 2, 1, 10, 0, 0, 0, time.UTC)
	store := newFakeStore()
	store.currentPeriod = domain.Period{ID: 2}
	store.teams = []domain.Team{{ID: 2, Name: "Deleted", Type: domain.TeamTypeTeam, DeletedAt: &deletedAt}}
	store.goalsByTeam[2] = map[int64][]domain.Goal{1: {{ID: 100, TeamID: 2, PeriodID: 1, Title: "Historic"}}}
	service := New(store)

	okr, err := service.GetTeamOKR(context.Background(), 2, 1, domain.Period{ID: 1, Name: "2024 Q4"})
	if err != nil {
		t.Fatalf("get team okr: %v", err)
	}
	if okr.Team.ID != 2 || len(okr.Goals) != 1 {
		t.Fatalf("expected historical deleted team okr to load")
	}
}

func TestGetTeamOKRRejectsDeletedTeamInCurrentPeriod(t *testing.T) {
	deletedAt := time.Date(2025, 2, 1, 10, 0, 0, 0, time.UTC)
	store := newFakeStore()
	store.currentPeriod = domain.Period{ID: 2}
	store.teams = []domain.Team{{ID: 2, Name: "Deleted", Type: domain.TeamTypeTeam, DeletedAt: &deletedAt}}
	service := New(store)

	_, err := service.GetTeamOKR(context.Background(), 2, 2, domain.Period{ID: 2, Name: "2025 Q1"})
	if err != ErrTeamNotVisibleInPeriod {
		t.Fatalf("expected ErrTeamNotVisibleInPeriod, got %v", err)
	}
}

func TestGetTeamOKRAllowsDeletedTeamInCurrentPeriodWhenGoalsExist(t *testing.T) {
	deletedAt := time.Date(2025, 2, 1, 10, 0, 0, 0, time.UTC)
	store := newFakeStore()
	store.currentPeriod = domain.Period{ID: 2}
	store.teams = []domain.Team{{ID: 2, Name: "Deleted", Type: domain.TeamTypeTeam, DeletedAt: &deletedAt}}
	store.goalsByTeam[2] = map[int64][]domain.Goal{2: {{ID: 200, TeamID: 2, PeriodID: 2, Title: "Current"}}}
	service := New(store)

	okr, err := service.GetTeamOKR(context.Background(), 2, 2, domain.Period{ID: 2, Name: "2025 Q1"})
	if err != nil {
		t.Fatalf("expected deleted team with current goals to be visible, got %v", err)
	}
	if okr.Team.ID != 2 || len(okr.Goals) != 1 {
		t.Fatalf("expected current deleted team okr to load")
	}
}

func ptr(id int64) *int64 { return &id }
