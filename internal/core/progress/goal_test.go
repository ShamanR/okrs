package progress_test

import (
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/core/progress"
)

func TestCalculateKRProgressNumericalLinear(t *testing.T) {
	kr := domain.KeyResult{
		Kind:      domain.KRKindNumerical,
		Numerical: &domain.KRNumerical{StartValue: 0, TargetValue: 200, CurrentValue: 100},
	}
	if got := progress.ForKR(kr); got != 50 {
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
	if got := progress.ForKR(kr); got != 83 {
		t.Fatalf("expected 83 (interpolated), got %d", got)
	}
}

func TestCalculateKRProgressBoolean(t *testing.T) {
	done := domain.KeyResult{Kind: domain.KRKindBoolean, Boolean: &domain.KRBoolean{IsDone: true}}
	notDone := domain.KeyResult{Kind: domain.KRKindBoolean, Boolean: &domain.KRBoolean{IsDone: false}}
	if got := progress.ForKR(done); got != 100 {
		t.Fatalf("done: expected 100, got %d", got)
	}
	if got := progress.ForKR(notDone); got != 0 {
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
	if got := progress.ForKR(kr); got != 40 {
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
		if got := progress.ForKR(kr); got != 0 {
			t.Fatalf("kind %s with nil meta: expected 0, got %d", kr.Kind, got)
		}
	}
}

func TestCalculateKRProgressUnknownKindReturnsZero(t *testing.T) {
	kr := domain.KeyResult{Kind: "UNKNOWN"}
	if got := progress.ForKR(kr); got != 0 {
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
	if got := progress.ForGoal(goal); got != 50 {
		t.Fatalf("expected 50, got %d", got)
	}
}

func TestCalculateGoalProgressNoKRsReturnsZero(t *testing.T) {
	goal := &domain.Goal{}
	if got := progress.ForGoal(goal); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
}

// ── KR health status: auto-done rule + manual set ────────────────────────────
