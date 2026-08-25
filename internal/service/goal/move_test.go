package goal_test

import (
	"context"
	"testing"

	"okrs/internal/core/domain"
	goalsvc "okrs/internal/service/goal"
	"okrs/internal/service/servicetest"
)

func TestMoveGoal(t *testing.T) {
	store := servicetest.NewStore()

	if err := goalsvc.New(store).Move(context.Background(), domain.TenantScope{TenantID: 1}, 5, 10, -1); err != nil {
		t.Fatalf("move goal: %v", err)
	}
	if store.MovedGoals[10] != -1 {
		t.Fatalf("expected goal move direction")
	}
}
