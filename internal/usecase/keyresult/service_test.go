package keyresult

// Тесты переехали из internal/service при выделении слоя usecase.

import (
	"context"
	"testing"

	"okrs/internal/core/domain"
	keyresultsvc "okrs/internal/service/keyresult"
	"okrs/internal/service/servicetest"
)

// CheckIn с заполненным Numerical обязан долетать до store.UpdateNumericalCurrent.
func TestUpdateKRProgressNumerical(t *testing.T) {
	store := servicetest.NewStore()
	store.KeyResults[1] = domain.KeyResult{ID: 1, Kind: domain.KRKindNumerical}
	service := newTestUC(store)

	if err := service.CheckIn(context.Background(), domain.TenantScope{TenantID: 1}, 1, CheckInInput{Numerical: ptr(42.0)}, 1); err != nil {
		t.Fatalf("update numerical: %v", err)
	}
	if store.NumericalUpdates[1] != 42 {
		t.Fatalf("expected numerical update")
	}
}

// CheckIn с заполненным Boolean обязан долетать до store.UpdateBoolean.
func TestUpdateKRProgressBoolean(t *testing.T) {
	store := servicetest.NewStore()
	store.KeyResults[3] = domain.KeyResult{ID: 3, Kind: domain.KRKindBoolean}
	service := newTestUC(store)

	if err := service.CheckIn(context.Background(), domain.TenantScope{TenantID: 1}, 3, CheckInInput{Boolean: ptr(true)}, 1); err != nil {
		t.Fatalf("update boolean: %v", err)
	}
	if !store.BooleanUpdates[3] {
		t.Fatalf("expected boolean update")
	}
}

// CheckIn с заполненным Project обязан долетать до store.BatchUpdateProjectStagesDone.
func TestUpdateKRProgressProject(t *testing.T) {
	store := servicetest.NewStore()
	store.KeyResults[4] = domain.KeyResult{ID: 4, Kind: domain.KRKindProject}
	store.ProjectStages[4] = []domain.KRProjectStage{{ID: 100, IsDone: false}, {ID: 101, IsDone: true}}
	service := newTestUC(store)

	updates := []ProjectStageUpdate{{ID: 100, IsDone: true}}
	if err := service.CheckIn(context.Background(), domain.TenantScope{TenantID: 1}, 4, CheckInInput{Project: updates}, 1); err != nil {
		t.Fatalf("update project: %v", err)
	}
	if !store.StageUpdates[100] {
		t.Fatalf("expected stage update")
	}
}

func TestMoveKeyResult(t *testing.T) {
	store := servicetest.NewStore()

	if err := keyresultsvc.New(store).Move(context.Background(), domain.TenantScope{TenantID: 1}, 20, 1); err != nil {
		t.Fatalf("move kr: %v", err)
	}
	if store.MovedKRs[20] != 1 {
		t.Fatalf("expected key result move direction")
	}
}
