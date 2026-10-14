package domain

import (
	"reflect"
	"testing"
)

func TestGoalDirectionsOrder(t *testing.T) {
	want := []GoalDirection{DirectionProduct, DirectionTech, DirectionProcess}
	if got := GoalDirections(); !reflect.DeepEqual(got, want) {
		t.Fatalf("GoalDirections() = %v, want %v", got, want)
	}
}

func TestGoalDirectionValid(t *testing.T) {
	for _, d := range GoalDirections() {
		if !d.Valid() {
			t.Errorf("%q must be valid", d)
		}
	}
	// Прежние значения фокуса после перехода на направления не принимаются.
	for _, d := range []GoalDirection{"", "product", "PROFITABILITY", "STABILITY", "SPEED_EFFICIENCY", "TECH_INDEPENDENCE"} {
		if d.Valid() {
			t.Errorf("%q must be invalid", d)
		}
	}
}
