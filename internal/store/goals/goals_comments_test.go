package goals_test

import (
	"context"
	"errors"
	"testing"

	"okrs/internal/domain"
	"okrs/internal/store/goals"
	"okrs/internal/store/krs"
	"okrs/internal/store/periods"
	"okrs/internal/store/teams"
	"okrs/internal/store/testutil"
)

// author 1 is the system `anonymous-local` user seeded by migrations.
const seedUserID = int64(1)

func TestSetGoalCommentResolved(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()

	gr := goals.NewGoalRepository(pool, krs.NewKRRepository(pool))
	tr := teams.NewTeamRepository(pool)
	pr := periods.NewPeriodRepository(pool)
	scope := domain.TenantScope{TenantID: 1}

	_, _, goalID := seedGoal(t, ctx, gr, tr, pr, scope, "resolve")
	if err := gr.AddGoalComment(ctx, scope, goalID, "blocker", seedUserID); err != nil {
		t.Fatalf("add comment: %v", err)
	}
	comments, err := gr.ListGoalComments(ctx, scope, goalID)
	if err != nil || len(comments) != 1 {
		t.Fatalf("list comments: err=%v n=%d", err, len(comments))
	}
	commentID := comments[0].ID
	if comments[0].ResolvedAt != nil {
		t.Fatalf("fresh comment must be unresolved")
	}

	// Resolve stamps time + resolver.
	if err := gr.SetGoalCommentResolved(ctx, scope, goalID, commentID, true, seedUserID); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	comments, _ = gr.ListGoalComments(ctx, scope, goalID)
	if comments[0].ResolvedAt == nil {
		t.Fatalf("resolved_at must be set after resolve")
	}
	if comments[0].ResolvedByName == "" {
		t.Fatalf("resolver name must be populated after resolve")
	}

	// Reopen clears both fields.
	if err := gr.SetGoalCommentResolved(ctx, scope, goalID, commentID, false, seedUserID); err != nil {
		t.Fatalf("unresolve: %v", err)
	}
	comments, _ = gr.ListGoalComments(ctx, scope, goalID)
	if comments[0].ResolvedAt != nil || comments[0].ResolvedByName != "" {
		t.Fatalf("reopen must clear resolve fields, got %+v", comments[0])
	}

	// Unknown comment id → ErrNotFound.
	if err := gr.SetGoalCommentResolved(ctx, scope, goalID, 999999, true, seedUserID); !errors.Is(err, goals.ErrNotFound) {
		t.Fatalf("unknown comment: want ErrNotFound, got %v", err)
	}

	// Comment cannot be resolved through a different goal id.
	if err := gr.SetGoalCommentResolved(ctx, scope, goalID+1, commentID, true, seedUserID); !errors.Is(err, goals.ErrNotFound) {
		t.Fatalf("wrong goal id: want ErrNotFound, got %v", err)
	}
}
