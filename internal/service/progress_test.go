package service

import (
	"testing"

	"okrs/internal/domain"
)

func TestCalculateKRProgressPercent(t *testing.T) {
	kr := domain.KeyResult{
		Kind: domain.KRKindPercent,
		Percent: &domain.KRPercent{StartValue: 0, TargetValue: 100, CurrentValue: 60},
	}
	if got := CalculateKRProgress(kr); got != 60 {
		t.Fatalf("expected 60, got %d", got)
	}
}

func TestCalculateKRProgressLinear(t *testing.T) {
	kr := domain.KeyResult{
		Kind:   domain.KRKindLinear,
		Linear: &domain.KRLinear{StartValue: 0, TargetValue: 200, CurrentValue: 100},
	}
	if got := CalculateKRProgress(kr); got != 50 {
		t.Fatalf("expected 50, got %d", got)
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
		{Kind: domain.KRKindPercent, Percent: nil},
		{Kind: domain.KRKindLinear, Linear: nil},
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
