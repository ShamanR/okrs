package keyresult

// Тесты переехали из internal/service при выделении слоя usecase.

import (
	"context"
	"testing"

	"okrs/internal/core/domain"
	keyresultsvc "okrs/internal/service/keyresult"
	"okrs/internal/service/servicetest"
)

// ListPeriodViews must filter out archived periods for the public caller (includeArchived=false)
// before building parent/depth views, so a public parent_id never points at a hidden period,
// while the admin caller (includeArchived=true) sees everything.
func TestUpdateKRProgressNumerical(t *testing.T) {
	store := servicetest.NewStore()
	store.KeyResults[1] = domain.KeyResult{ID: 1, Kind: domain.KRKindNumerical}
	service := newTestUC(store)

	if err := service.UpdateProgressNumerical(context.Background(), domain.TenantScope{TenantID: 1}, 1, 42, 1); err != nil {
		t.Fatalf("update numerical: %v", err)
	}
	if store.NumericalUpdates[1] != 42 {
		t.Fatalf("expected numerical update")
	}
}

// ArchivePeriod must only allow archiving a closed period, so an active/future period can't be
// hidden from the tree via archive.
func TestUpdateKRProgressBoolean(t *testing.T) {
	store := servicetest.NewStore()
	store.KeyResults[3] = domain.KeyResult{ID: 3, Kind: domain.KRKindBoolean}
	service := newTestUC(store)

	if err := service.UpdateProgressBoolean(context.Background(), domain.TenantScope{TenantID: 1}, 3, true, 1); err != nil {
		t.Fatalf("update boolean: %v", err)
	}
	if !store.BooleanUpdates[3] {
		t.Fatalf("expected boolean update")
	}
}

func TestUpdateKRProgressProject(t *testing.T) {
	store := servicetest.NewStore()
	store.KeyResults[4] = domain.KeyResult{ID: 4, Kind: domain.KRKindProject}
	store.ProjectStages[4] = []domain.KRProjectStage{{ID: 100, IsDone: false}, {ID: 101, IsDone: true}}
	service := newTestUC(store)

	updates := []ProjectStageUpdate{{ID: 100, IsDone: true}}
	if err := service.UpdateProgressProject(context.Background(), domain.TenantScope{TenantID: 1}, 4, updates, 1); err != nil {
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
