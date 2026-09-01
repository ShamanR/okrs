package keyresult

// Тесты переехали из internal/service при выделении слоя usecase.

import (
	"context"
	"testing"

	"okrs/internal/core/domain"
	keyresultsvc "okrs/internal/service/keyresult"
	"okrs/internal/service/servicetest"
	"okrs/internal/store/krs"
)

func TestUpdateKRProgressNumericalRejectsUnsupportedKind(t *testing.T) {
	st := servicetest.NewGoalStore()
	st.KeyResults[1] = domain.KeyResult{ID: 1, Kind: domain.KRKindBoolean}
	svc := newGoalTestService(st)

	if err := svc.CheckIn(context.Background(), domain.TenantScope{TenantID: 1}, 1, CheckInInput{Numerical: ptr(50.0)}, 1); err == nil {
		t.Fatal("expected error for boolean KR with numerical update")
	}
}

func TestUpdateKRProgressBooleanRejectsUnsupportedKind(t *testing.T) {
	st := servicetest.NewGoalStore()
	st.KeyResults[2] = domain.KeyResult{ID: 2, Kind: domain.KRKindNumerical}
	svc := newGoalTestService(st)

	if err := svc.CheckIn(context.Background(), domain.TenantScope{TenantID: 1}, 2, CheckInInput{Boolean: ptr(true)}, 1); err == nil {
		t.Fatal("expected error for numerical KR with boolean update")
	}
}

// Пустой (но не nil) слайс Project — явный сигнал «прогресс — часть этого
// чек-ина» (в отличие от nil, означающего «прогресс не отправлен вовсе»), поэтому
// именно так тест обязан вызывать CheckIn, чтобы задеть проверку вида KR — ровно
// как раньше это гарантировала обёртка UpdateProgressProject, нормализуя nil в []
// на своей стороне.
func TestUpdateKRProgressProjectRejectsUnsupportedKind(t *testing.T) {
	st := servicetest.NewGoalStore()
	st.KeyResults[3] = domain.KeyResult{ID: 3, Kind: domain.KRKindNumerical}
	svc := newGoalTestService(st)

	if err := svc.CheckIn(context.Background(), domain.TenantScope{TenantID: 1}, 3, CheckInInput{Project: []ProjectStageUpdate{}}, 1); err == nil {
		t.Fatal("expected error for numerical KR with project update")
	}
}

func TestCreateKeyResultWithMetaAppliesNumericalMeta(t *testing.T) {
	st := servicetest.NewGoalStore()
	svc := newGoalTestService(st)

	_, err := svc.CreateWithMeta(context.Background(), domain.TenantScope{TenantID: 1},
		krs.KeyResultInput{Kind: domain.KRKindNumerical},
		keyresultsvc.MetaInput{NumericalStart: 0, NumericalTarget: 100, NumericalCurrent: 30, NumericalUnit: "%"},
		1,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(st.UpsertNumericalCalls) != 1 {
		t.Fatalf("expected UpsertNumericalMeta called once, got %d", len(st.UpsertNumericalCalls))
	}
	meta := st.UpsertNumericalCalls[0]
	if meta.StartValue != 0 || meta.TargetValue != 100 || meta.CurrentValue != 30 || meta.Unit != "%" {
		t.Fatalf("unexpected numerical meta values: %+v", meta)
	}
}

func TestCreateKeyResultWithMetaAppliesBooleanMeta(t *testing.T) {
	st := servicetest.NewGoalStore()
	svc := newGoalTestService(st)

	_, err := svc.CreateWithMeta(context.Background(), domain.TenantScope{TenantID: 1},
		krs.KeyResultInput{Kind: domain.KRKindBoolean},
		keyresultsvc.MetaInput{BooleanDone: true},
		1,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(st.UpsertBoolCalls) != 1 || !st.UpsertBoolCalls[0].Done {
		t.Fatalf("expected UpsertBooleanMeta(done=true), got %+v", st.UpsertBoolCalls)
	}
}

func TestCreateKeyResultWithMetaAppliesProjectStages(t *testing.T) {
	st := servicetest.NewGoalStore()
	svc := newGoalTestService(st)

	stages := []krs.ProjectStageInput{{Title: "Step 1", Weight: 60}, {Title: "Step 2", Weight: 40}}
	_, err := svc.CreateWithMeta(context.Background(), domain.TenantScope{TenantID: 1},
		krs.KeyResultInput{Kind: domain.KRKindProject},
		keyresultsvc.MetaInput{ProjectStages: stages},
		1,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(st.ReplaceStageCalls) != 1 || len(st.ReplaceStageCalls[0].Stages) != 2 {
		t.Fatalf("expected ReplaceProjectStages with 2 stages, got %+v", st.ReplaceStageCalls)
	}
}
