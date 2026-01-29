package service

import (
	"context"
	"testing"

	"okrs/internal/domain"
	"okrs/internal/store"
)

type fakeStore struct {
	keyResults     map[int64]domain.KeyResult
	percentUpdates map[int64]float64
	linearUpdates  map[int64]float64
	booleanUpdates map[int64]bool
	projectStages  map[int64][]domain.KRProjectStage
	stageUpdates   map[int64]bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		keyResults:     make(map[int64]domain.KeyResult),
		percentUpdates: make(map[int64]float64),
		linearUpdates:  make(map[int64]float64),
		booleanUpdates: make(map[int64]bool),
		projectStages:  make(map[int64][]domain.KRProjectStage),
		stageUpdates:   make(map[int64]bool),
	}
}

func (f *fakeStore) ListTeams(context.Context) ([]domain.Team, error) {
	return nil, nil
}
func (f *fakeStore) GetTeam(context.Context, int64) (domain.Team, error) {
	return domain.Team{}, nil
}
func (f *fakeStore) ListGoalsByTeamQuarter(context.Context, int64, int, int) ([]domain.Goal, error) {
	return nil, nil
}
func (f *fakeStore) ListGoalShares(context.Context, int64) ([]store.GoalShare, error) {
	return nil, nil
}
func (f *fakeStore) GetTeamQuarterStatus(context.Context, int64, int, int) (domain.TeamQuarterStatus, error) {
	return domain.TeamQuarterStatusNoGoals, nil
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
func (f *fakeStore) UpdateGoalTeamWeight(context.Context, int64, int64, int) error {
	return nil
}
func (f *fakeStore) GetKeyResult(_ context.Context, id int64) (domain.KeyResult, error) {
	return f.keyResults[id], nil
}
func (f *fakeStore) AddGoalComment(context.Context, int64, string) error {
	return nil
}
func (f *fakeStore) AddKeyResultComment(context.Context, int64, string) error {
	return nil
}
func (f *fakeStore) GetGoal(context.Context, int64) (domain.Goal, error) {
	return domain.Goal{}, nil
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
