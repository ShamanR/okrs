package service

import (
	"context"
	"testing"

	"okrs/internal/core/domain"
)

func TestCalculateKRProgressNumericalLinear(t *testing.T) {
	kr := domain.KeyResult{
		Kind:      domain.KRKindNumerical,
		Numerical: &domain.KRNumerical{StartValue: 0, TargetValue: 200, CurrentValue: 100},
	}
	if got := CalculateKRProgress(kr); got != 50 {
		t.Fatalf("expected 50, got %d", got)
	}
}

func TestCalculateKRProgressNumericalCheckpoints(t *testing.T) {
	kr := domain.KeyResult{
		Kind: domain.KRKindNumerical,
		Numerical: &domain.KRNumerical{
			StartValue: 100, TargetValue: 180, CurrentValue: 170,
			Checkpoints: []domain.KRNumericalCheckpoint{
				{Value: 150, ProgressPercent: 50},
			},
		},
	}
	// Interpolates between checkpoint (150,50%) and target (180,100%): 170 → 83%.
	if got := CalculateKRProgress(kr); got != 83 {
		t.Fatalf("expected 83 (interpolated), got %d", got)
	}
}

func TestCalculateKRProgressBoolean(t *testing.T) {
	done := domain.KeyResult{Kind: domain.KRKindBoolean, Boolean: &domain.KRBoolean{IsDone: true}}
	notDone := domain.KeyResult{Kind: domain.KRKindBoolean, Boolean: &domain.KRBoolean{IsDone: false}}
	if got := CalculateKRProgress(done); got != 100 {
		t.Fatalf("done: expected 100, got %d", got)
	}
	if got := CalculateKRProgress(notDone); got != 0 {
		t.Fatalf("not done: expected 0, got %d", got)
	}
}

func TestCalculateKRProgressProject(t *testing.T) {
	kr := domain.KeyResult{
		Kind: domain.KRKindProject,
		Project: &domain.KRProject{
			Stages: []domain.KRProjectStage{
				{Weight: 40, IsDone: true},
				{Weight: 60, IsDone: false},
			},
		},
	}
	if got := CalculateKRProgress(kr); got != 40 {
		t.Fatalf("expected 40, got %d", got)
	}
}

func TestCalculateKRProgressNilMetaReturnsZero(t *testing.T) {
	cases := []domain.KeyResult{
		{Kind: domain.KRKindNumerical, Numerical: nil},
		{Kind: domain.KRKindBoolean, Boolean: nil},
		{Kind: domain.KRKindProject, Project: nil},
	}
	for _, kr := range cases {
		if got := CalculateKRProgress(kr); got != 0 {
			t.Fatalf("kind %s with nil meta: expected 0, got %d", kr.Kind, got)
		}
	}
}

func TestCalculateKRProgressUnknownKindReturnsZero(t *testing.T) {
	kr := domain.KeyResult{Kind: "UNKNOWN"}
	if got := CalculateKRProgress(kr); got != 0 {
		t.Fatalf("unknown kind: expected 0, got %d", got)
	}
}

func TestCalculateGoalProgressAggregatesKRs(t *testing.T) {
	goal := &domain.Goal{
		KeyResults: []domain.KeyResult{
			{Kind: domain.KRKindBoolean, Boolean: &domain.KRBoolean{IsDone: true}, Weight: 50},
			{Kind: domain.KRKindBoolean, Boolean: &domain.KRBoolean{IsDone: false}, Weight: 50},
		},
	}
	if got := CalculateGoalProgress(goal); got != 50 {
		t.Fatalf("expected 50, got %d", got)
	}
}

func TestCalculateGoalProgressNoKRsReturnsZero(t *testing.T) {
	goal := &domain.Goal{}
	if got := CalculateGoalProgress(goal); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
}

// ── KR health status: auto-done rule + manual set ────────────────────────────

func TestNumericalReaching100AutoSetsDone(t *testing.T) {
	store := newFakeStore()
	store.keyResults[7] = domain.KeyResult{
		ID: 7, Kind: domain.KRKindNumerical, HealthStatus: domain.KRHealthNotStarted,
		Numerical: &domain.KRNumerical{StartValue: 0, TargetValue: 100, CurrentValue: 50},
	}
	svc := newTestService(store, nil)
	if err := svc.UpdateKRProgressNumerical(context.Background(), domain.TenantScope{TenantID: 1}, 7, 100, 1); err != nil {
		t.Fatalf("update: %v", err)
	}
	if store.healthUpdates[7] != domain.KRHealthDone {
		t.Fatalf("expected auto-done, got %q", store.healthUpdates[7])
	}
}

func TestResaveAt100DoesNotOverrideManualHealth(t *testing.T) {
	store := newFakeStore()
	store.keyResults[7] = domain.KeyResult{
		ID: 7, Kind: domain.KRKindNumerical, HealthStatus: domain.KRHealthAtRisk,
		Numerical: &domain.KRNumerical{StartValue: 0, TargetValue: 100, CurrentValue: 100},
	}
	svc := newTestService(store, nil)
	if err := svc.UpdateKRProgressNumerical(context.Background(), domain.TenantScope{TenantID: 1}, 7, 100, 1); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, ok := store.healthUpdates[7]; ok {
		t.Fatalf("re-save at 100 must not touch health, got %q", store.healthUpdates[7])
	}
}

func TestDroppingBelow100KeepsHealth(t *testing.T) {
	store := newFakeStore()
	store.keyResults[7] = domain.KeyResult{
		ID: 7, Kind: domain.KRKindNumerical, HealthStatus: domain.KRHealthDone,
		Numerical: &domain.KRNumerical{StartValue: 0, TargetValue: 100, CurrentValue: 100},
	}
	svc := newTestService(store, nil)
	if err := svc.UpdateKRProgressNumerical(context.Background(), domain.TenantScope{TenantID: 1}, 7, 80, 1); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, ok := store.healthUpdates[7]; ok {
		t.Fatalf("dropping below 100 must not touch health, got %q", store.healthUpdates[7])
	}
}

func TestUpdateKRHealthStatusSets(t *testing.T) {
	store := newFakeStore()
	store.keyResults[7] = domain.KeyResult{ID: 7, Kind: domain.KRKindNumerical, HealthStatus: domain.KRHealthNotStarted}
	svc := newTestService(store, nil)
	if err := svc.UpdateKRHealthStatus(context.Background(), domain.TenantScope{TenantID: 1}, 7, domain.KRHealthOnTrack); err != nil {
		t.Fatalf("update: %v", err)
	}
	if store.healthUpdates[7] != domain.KRHealthOnTrack {
		t.Fatalf("expected on_track, got %q", store.healthUpdates[7])
	}
}

func TestUpdateKRHealthStatusRejectsInvalid(t *testing.T) {
	store := newFakeStore()
	store.keyResults[7] = domain.KeyResult{ID: 7, Kind: domain.KRKindNumerical}
	svc := newTestService(store, nil)
	if err := svc.UpdateKRHealthStatus(context.Background(), domain.TenantScope{TenantID: 1}, 7, domain.KRHealthStatus("bogus")); err == nil {
		t.Fatal("expected error for invalid health status")
	}
	if _, ok := store.healthUpdates[7]; ok {
		t.Fatal("invalid status must not be written")
	}
}

func TestBooleanDoneAutoSetsHealthDone(t *testing.T) {
	store := newFakeStore()
	store.keyResults[7] = domain.KeyResult{
		ID: 7, Kind: domain.KRKindBoolean, HealthStatus: domain.KRHealthNotStarted,
		Boolean: &domain.KRBoolean{IsDone: false},
	}
	svc := newTestService(store, nil)
	if err := svc.UpdateKRProgressBoolean(context.Background(), domain.TenantScope{TenantID: 1}, 7, true, 1); err != nil {
		t.Fatalf("update: %v", err)
	}
	if store.healthUpdates[7] != domain.KRHealthDone {
		t.Fatalf("expected auto-done, got %q", store.healthUpdates[7])
	}
}

func TestProjectReaching100AutoSetsDone(t *testing.T) {
	store := newFakeStore()
	store.keyResults[7] = domain.KeyResult{ID: 7, Kind: domain.KRKindProject, HealthStatus: domain.KRHealthNotStarted}
	store.projectStages[7] = []domain.KRProjectStage{
		{ID: 1, Weight: 60, IsDone: false},
		{ID: 2, Weight: 40, IsDone: false},
	}
	svc := newTestService(store, nil)
	updates := []ProjectStageUpdate{{ID: 1, IsDone: true}, {ID: 2, IsDone: true}}
	if err := svc.UpdateKRProgressProject(context.Background(), domain.TenantScope{TenantID: 1}, 7, updates, 1); err != nil {
		t.Fatalf("update: %v", err)
	}
	if store.healthUpdates[7] != domain.KRHealthDone {
		t.Fatalf("expected auto-done, got %q", store.healthUpdates[7])
	}
}
