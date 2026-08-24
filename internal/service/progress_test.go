package service

import (
	"context"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/service/servicetest"
)

func TestNumericalReaching100AutoSetsDone(t *testing.T) {
	store := servicetest.NewStore()
	store.KeyResults[7] = domain.KeyResult{
		ID: 7, Kind: domain.KRKindNumerical, HealthStatus: domain.KRHealthNotStarted,
		Numerical: &domain.KRNumerical{StartValue: 0, TargetValue: 100, CurrentValue: 50},
	}
	svc := newTestService(store, nil)
	if err := svc.UpdateKRProgressNumerical(context.Background(), domain.TenantScope{TenantID: 1}, 7, 100, 1); err != nil {
		t.Fatalf("update: %v", err)
	}
	if store.HealthUpdates[7] != domain.KRHealthDone {
		t.Fatalf("expected auto-done, got %q", store.HealthUpdates[7])
	}
}

func TestResaveAt100DoesNotOverrideManualHealth(t *testing.T) {
	store := servicetest.NewStore()
	store.KeyResults[7] = domain.KeyResult{
		ID: 7, Kind: domain.KRKindNumerical, HealthStatus: domain.KRHealthAtRisk,
		Numerical: &domain.KRNumerical{StartValue: 0, TargetValue: 100, CurrentValue: 100},
	}
	svc := newTestService(store, nil)
	if err := svc.UpdateKRProgressNumerical(context.Background(), domain.TenantScope{TenantID: 1}, 7, 100, 1); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, ok := store.HealthUpdates[7]; ok {
		t.Fatalf("re-save at 100 must not touch health, got %q", store.HealthUpdates[7])
	}
}

func TestDroppingBelow100KeepsHealth(t *testing.T) {
	store := servicetest.NewStore()
	store.KeyResults[7] = domain.KeyResult{
		ID: 7, Kind: domain.KRKindNumerical, HealthStatus: domain.KRHealthDone,
		Numerical: &domain.KRNumerical{StartValue: 0, TargetValue: 100, CurrentValue: 100},
	}
	svc := newTestService(store, nil)
	if err := svc.UpdateKRProgressNumerical(context.Background(), domain.TenantScope{TenantID: 1}, 7, 80, 1); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, ok := store.HealthUpdates[7]; ok {
		t.Fatalf("dropping below 100 must not touch health, got %q", store.HealthUpdates[7])
	}
}

func TestBooleanDoneAutoSetsHealthDone(t *testing.T) {
	store := servicetest.NewStore()
	store.KeyResults[7] = domain.KeyResult{
		ID: 7, Kind: domain.KRKindBoolean, HealthStatus: domain.KRHealthNotStarted,
		Boolean: &domain.KRBoolean{IsDone: false},
	}
	svc := newTestService(store, nil)
	if err := svc.UpdateKRProgressBoolean(context.Background(), domain.TenantScope{TenantID: 1}, 7, true, 1); err != nil {
		t.Fatalf("update: %v", err)
	}
	if store.HealthUpdates[7] != domain.KRHealthDone {
		t.Fatalf("expected auto-done, got %q", store.HealthUpdates[7])
	}
}

func TestProjectReaching100AutoSetsDone(t *testing.T) {
	store := servicetest.NewStore()
	store.KeyResults[7] = domain.KeyResult{ID: 7, Kind: domain.KRKindProject, HealthStatus: domain.KRHealthNotStarted}
	store.ProjectStages[7] = []domain.KRProjectStage{
		{ID: 1, Weight: 60, IsDone: false},
		{ID: 2, Weight: 40, IsDone: false},
	}
	svc := newTestService(store, nil)
	updates := []ProjectStageUpdate{{ID: 1, IsDone: true}, {ID: 2, IsDone: true}}
	if err := svc.UpdateKRProgressProject(context.Background(), domain.TenantScope{TenantID: 1}, 7, updates, 1); err != nil {
		t.Fatalf("update: %v", err)
	}
	if store.HealthUpdates[7] != domain.KRHealthDone {
		t.Fatalf("expected auto-done, got %q", store.HealthUpdates[7])
	}
}
