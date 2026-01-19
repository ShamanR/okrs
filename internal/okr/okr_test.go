package okr

import (
	"testing"

	"okrs/internal/domain"
)

func TestGoalProgress(t *testing.T) {
	cases := []struct {
		name   string
		krs    []domain.KeyResult
		expect int
	}{
		{name: "no krs", krs: nil, expect: 0},
		{name: "zero weights", krs: []domain.KeyResult{{Progress: 50, Weight: 0}, {Progress: 20, Weight: 0}}, expect: 0},
		{name: "weighted", krs: []domain.KeyResult{{Progress: 100, Weight: 50}, {Progress: 0, Weight: 50}}, expect: 50},
	}
	for _, tc := range cases {
		if got := GoalProgress(tc.krs); got != tc.expect {
			t.Fatalf("%s: expected %d got %d", tc.name, tc.expect, got)
		}
	}
}

func TestQuarterProgress(t *testing.T) {
	goals := []domain.Goal{
		{Progress: 100, Weight: 60},
		{Progress: 50, Weight: 40},
	}
	if got := QuarterProgress(goals); got != 80 {
		t.Fatalf("expected 80 got %d", got)
	}
	if got := QuarterProgress(nil); got != 0 {
		t.Fatalf("expected 0 got %d", got)
	}
}

func TestProjectProgress(t *testing.T) {
	stages := []domain.KRProjectStage{{Weight: 30, IsDone: true}, {Weight: 70, IsDone: false}}
	if got := ProjectProgress(stages); got != 30 {
		t.Fatalf("expected 30 got %d", got)
	}
}

func TestPercentProgressLinear(t *testing.T) {
	if got := PercentProgress(0, 100, 50, nil); got != 50 {
		t.Fatalf("expected 50 got %d", got)
	}
	if got := PercentProgress(100, 0, 50, nil); got != 50 {
		t.Fatalf("reverse expected 50 got %d", got)
	}
}

func TestPercentProgressCheckpoints(t *testing.T) {
	checkpoints := []domain.KRPercentCheckpoint{
		{MetricValue: 50, KRPercent: 40},
		{MetricValue: 80, KRPercent: 70},
	}
	if got := PercentProgress(0, 100, 60, checkpoints); got != 50 {
		t.Fatalf("expected 50 got %d", got)
	}
	if got := PercentProgress(0, 100, -10, checkpoints); got != 0 {
		t.Fatalf("expected 0 got %d", got)
	}
	if got := PercentProgress(0, 100, 110, checkpoints); got != 100 {
		t.Fatalf("expected 100 got %d", got)
	}
}
