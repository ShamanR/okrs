package common

import (
	"testing"

	"okrs/internal/core/domain"
)

func TestValidateGoalInputDirection(t *testing.T) {
	for _, d := range domain.GoalDirections() {
		if msg := ValidateGoalInput(domain.PriorityP1, domain.WorkTypeDelivery, d, 50); msg != "" {
			t.Errorf("direction %q: unexpected error %q", d, msg)
		}
	}
	for _, d := range []domain.GoalDirection{"", "PROFITABILITY", "TECH_INDEPENDENCE"} {
		if msg := ValidateGoalInput(domain.PriorityP1, domain.WorkTypeDelivery, d, 50); msg != "Неверное направление цели" {
			t.Errorf("direction %q: got %q, want the direction error", d, msg)
		}
	}
}
