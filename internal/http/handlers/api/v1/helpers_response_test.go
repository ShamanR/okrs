package v1

import (
	"testing"
	"time"

	"okrs/internal/domain"
	"okrs/internal/service"
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
	detail := service.GoalDetails{
		Goal: domain.Goal{
			ID:       10,
			Progress: 40,
		},
	}

	result := MapGoalDetails(detail, period)
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
