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

func TestNumericalProgressLinear(t *testing.T) {
	cases := []struct {
		name                   string
		start, target, current float64
		expect                 int
	}{
		{"growth midpoint", 100, 500, 300, 50},
		{"decline midpoint", 10, 5, 7.5, 50},
		{"below start", 100, 500, 80, 0},
		{"above target", 100, 500, 600, 100},
		{"at start", 0, 100, 0, 0},
		{"at target", 0, 100, 100, 100},
		{"equal reached target", 100, 100, 100, 100},
		{"equal not reached", 100, 100, 90, 0},
	}
	for _, tc := range cases {
		if got := NumericalProgress(tc.start, tc.target, tc.current, nil); got != tc.expect {
			t.Fatalf("%s: expected %d got %d", tc.name, tc.expect, got)
		}
	}
}

func TestNumericalProgressCheckpointsStep(t *testing.T) {
	cps := []domain.KRNumericalCheckpoint{
		{Value: 100, ProgressPercent: 0},
		{Value: 150, ProgressPercent: 50},
		{Value: 180, ProgressPercent: 100},
	}
	cases := []struct {
		name    string
		current float64
		expect  int
	}{
		{"between first two", 120, 0},
		{"on middle step", 150, 50},
		{"between middle and last", 170, 50},
		{"on last step", 180, 100},
		{"below first step", 90, 0},
		{"above last step", 200, 100},
	}
	for _, tc := range cases {
		if got := NumericalProgress(100, 180, tc.current, cps); got != tc.expect {
			t.Fatalf("%s: expected %d got %d", tc.name, tc.expect, got)
		}
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

