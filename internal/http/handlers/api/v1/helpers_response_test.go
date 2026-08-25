package v1

import (
	okrboarduc "okrs/internal/usecase/okrboard"
	"testing"
	"time"

	"okrs/internal/core/domain"
)

func TestBuildMeasureNumerical(t *testing.T) {
	kr := domain.KeyResult{
		Kind: domain.KRKindNumerical,
		Numerical: &domain.KRNumerical{
			StartValue:   0,
			TargetValue:  100,
			CurrentValue: 50,
			Unit:         "RPS",
			Checkpoints: []domain.KRNumericalCheckpoint{
				{Value: 25, ProgressPercent: 25},
			},
		},
	}
	measure := buildMeasure(kr)
	if measure.Kind != string(domain.KRKindNumerical) {
		t.Fatalf("expected kind %s, got %s", domain.KRKindNumerical, measure.Kind)
	}
	if measure.Numerical == nil {
		t.Fatalf("expected numerical measure")
	}
	if measure.Numerical.Unit != "RPS" {
		t.Fatalf("expected unit RPS, got %s", measure.Numerical.Unit)
	}
	if len(measure.Numerical.Checkpoints) != 1 {
		t.Fatalf("expected checkpoints")
	}
}

func TestMapKeyResultZeroingTopLevel(t *testing.T) {
	for _, kind := range []domain.KRKind{domain.KRKindNumerical, domain.KRKindBoolean, domain.KRKindProject} {
		kr := domain.KeyResult{Kind: kind, ZeroingCriteria: "падение сервиса = 0%"}
		switch kind {
		case domain.KRKindNumerical:
			kr.Numerical = &domain.KRNumerical{Unit: "%"}
		case domain.KRKindBoolean:
			kr.Boolean = &domain.KRBoolean{}
		case domain.KRKindProject:
			kr.Project = &domain.KRProject{}
		}
		dtoKR := MapKeyResult(kr)
		if dtoKR.ZeroingCriteria != "падение сервиса = 0%" {
			t.Fatalf("kind %s: expected top-level zeroing, got %q", kind, dtoKR.ZeroingCriteria)
		}
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

func TestCalculatePeriodForecastBounds(t *testing.T) {
	period := domain.Period{
		StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 1, 11, 0, 0, 0, 0, time.UTC),
	}

	if got := CalculatePeriodForecast(period, time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)); got != 0 {
		t.Fatalf("before start: want 0, got %d", got)
	}
	if got := CalculatePeriodForecast(period, time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC)); got != 100 {
		t.Fatalf("after end: want 100, got %d", got)
	}

	mid := time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC)
	if got := CalculatePeriodForecast(period, mid); got < 49 || got > 51 {
		t.Fatalf("mid period: want ~50, got %d", got)
	}
}

func TestBuildProgressBarInfoStatusByDelta(t *testing.T) {
	period := domain.Period{
		StartDate: time.Now().Add(-100 * time.Hour),
		EndDate:   time.Now().Add(100 * time.Hour),
	}

	infoBelow := BuildProgressBarInfo(0, period)
	if infoBelow.Status != "below" {
		t.Fatalf("expected below status, got %s (actual=%d forecast=%d delta=%d)", infoBelow.Status, infoBelow.Actual, infoBelow.Forecast, infoBelow.Delta)
	}

	infoAbove := BuildProgressBarInfo(100, period)
	if infoAbove.Status != "above" {
		t.Fatalf("expected above status, got %s (actual=%d forecast=%d delta=%d)", infoAbove.Status, infoAbove.Actual, infoAbove.Forecast, infoAbove.Delta)
	}

	onTrackActual := infoAbove.Forecast
	infoOnTrack := BuildProgressBarInfo(onTrackActual, period)
	if infoOnTrack.Status != "on_track" {
		t.Fatalf("expected on_track status, got %s (actual=%d forecast=%d delta=%d)", infoOnTrack.Status, infoOnTrack.Actual, infoOnTrack.Forecast, infoOnTrack.Delta)
	}
}

func TestMapGoalDetailsIncludesProgressMeta(t *testing.T) {
	period := domain.Period{
		StartDate: time.Now().Add(-24 * time.Hour),
		EndDate:   time.Now().Add(24 * time.Hour),
	}
	detail := okrboarduc.GoalDetails{
		Goal: domain.Goal{
			ID:       10,
			Progress: 40,
		},
	}

	result := MapGoalDetails(detail, period, nil)
	if result.ProgressMeta.Actual != 40 {
		t.Fatalf("expected progress_meta.actual=40, got %d", result.ProgressMeta.Actual)
	}
	if result.ProgressMeta.Forecast < 0 || result.ProgressMeta.Forecast > 100 {
		t.Fatalf("expected progress_meta.forecast in [0,100], got %d", result.ProgressMeta.Forecast)
	}
	if result.ProgressMeta.Status == "" {
		t.Fatalf("expected non-empty progress_meta.status")
	}
}
