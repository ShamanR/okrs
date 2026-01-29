package v1

import (
	"testing"

	"okrs/internal/domain"
)

func TestBuildMeasurePercent(t *testing.T) {
	kr := domain.KeyResult{
		Kind: domain.KRKindPercent,
		Percent: &domain.KRPercent{
			StartValue:   0,
			TargetValue:  100,
			CurrentValue: 50,
			Checkpoints: []domain.KRPercentCheckpoint{
				{ID: 1, MetricValue: 25, KRPercent: 25},
			},
		},
	}
	measure := buildMeasure(kr)
	if measure.Kind != string(domain.KRKindPercent) {
		t.Fatalf("expected kind %s, got %s", domain.KRKindPercent, measure.Kind)
	}
	if measure.Percent == nil {
		t.Fatalf("expected percent measure")
	}
	if len(measure.Checkpoints) != 1 {
		t.Fatalf("expected checkpoints")
	}
}

func TestBuildMeasureLinear(t *testing.T) {
	kr := domain.KeyResult{
		Kind: domain.KRKindLinear,
		Linear: &domain.KRLinear{
			StartValue:   10,
			TargetValue:  20,
			CurrentValue: 12,
		},
	}
	measure := buildMeasure(kr)
	if measure.Kind != string(domain.KRKindLinear) {
		t.Fatalf("expected kind %s, got %s", domain.KRKindLinear, measure.Kind)
	}
	if measure.Linear == nil {
		t.Fatalf("expected linear measure")
	}
}

func TestBuildMeasureBoolean(t *testing.T) {
	kr := domain.KeyResult{
		Kind: domain.KRKindBoolean,
		Boolean: &domain.KRBoolean{
			IsDone: true,
		},
	}
	measure := buildMeasure(kr)
	if measure.Kind != string(domain.KRKindBoolean) {
		t.Fatalf("expected kind %s, got %s", domain.KRKindBoolean, measure.Kind)
	}
	if measure.Boolean == nil || !measure.Boolean.IsDone {
		t.Fatalf("expected boolean measure")
	}
}

func TestBuildMeasureProject(t *testing.T) {
	kr := domain.KeyResult{
		Kind: domain.KRKindProject,
		Project: &domain.KRProject{
			Stages: []domain.KRProjectStage{{ID: 1, Title: "Stage", Weight: 50, IsDone: true}},
		},
	}
	measure := buildMeasure(kr)
	if measure.Kind != string(domain.KRKindProject) {
		t.Fatalf("expected kind %s, got %s", domain.KRKindProject, measure.Kind)
	}
	if measure.Project == nil || len(measure.Project.Stages) != 1 {
		t.Fatalf("expected project measure")
	}
}
