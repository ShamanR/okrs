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

func TestPeriodProgress(t *testing.T) {
	goals := []domain.Goal{
		{Progress: 100, Weight: 60},
		{Progress: 50, Weight: 40},
	}
	if got := PeriodProgress(goals); got != 80 {
		t.Fatalf("expected 80 got %d", got)
	}
	if got := PeriodProgress(nil); got != 0 {
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

func TestBooleanProgress(t *testing.T) {
	if got := BooleanProgress(true); got != 100 {
		t.Fatalf("done=true: expected 100, got %d", got)
	}
	if got := BooleanProgress(false); got != 0 {
		t.Fatalf("done=false: expected 0, got %d", got)
	}
}

func TestLinearProgress(t *testing.T) {
	cases := []struct {
		name                   string
		start, target, current float64
		expect                 int
	}{
		{"midpoint", 0, 100, 50, 50},
		{"at start", 0, 100, 0, 0},
		{"at target", 0, 100, 100, 100},
		{"above target clamped", 0, 100, 150, 100},
		{"below start clamped", 0, 100, -50, 0},
		{"offset range", 100, 200, 150, 50},
		{"equal start and target", 100, 100, 100, 0},
	}
	for _, tc := range cases {
		if got := LinearProgress(tc.start, tc.target, tc.current); got != tc.expect {
			t.Fatalf("%s: expected %d got %d", tc.name, tc.expect, got)
		}
	}
}

func TestGoalProgressSingleKR(t *testing.T) {
	krs := []domain.KeyResult{{Progress: 75, Weight: 100}}
	if got := GoalProgress(krs); got != 75 {
		t.Fatalf("expected 75, got %d", got)
	}
}

func TestProjectProgressAllDone(t *testing.T) {
	stages := []domain.KRProjectStage{{Weight: 60, IsDone: true}, {Weight: 40, IsDone: true}}
	if got := ProjectProgress(stages); got != 100 {
		t.Fatalf("expected 100, got %d", got)
	}
}

func TestProjectProgressNoneDone(t *testing.T) {
	stages := []domain.KRProjectStage{{Weight: 50, IsDone: false}, {Weight: 50, IsDone: false}}
	if got := ProjectProgress(stages); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
}

func TestPercentProgressEqualStartTarget(t *testing.T) {
	if got := PercentProgress(50, 50, 50, nil); got != 0 {
		t.Fatalf("equal start/target: expected 0, got %d", got)
	}
}
