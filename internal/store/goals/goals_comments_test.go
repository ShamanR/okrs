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

func TestSetGoalCommentResolvedIdempotent(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	gr := goals.NewGoalRepository(pool, krs.NewKRRepository(pool))
	tr := teams.NewTeamRepository(pool)
	pr := periods.NewPeriodRepository(pool)
	scope := domain.TenantScope{TenantID: 1}
	_, _, goalID := seedGoal(t, ctx, gr, tr, pr, scope, "idem")
	if _, err := gr.AddGoalComment(ctx, scope, goalID, "blocker", seedUserID); err != nil {
		t.Fatalf("add: %v", err)
	}
	comments, _ := gr.ListGoalComments(ctx, scope, goalID)
	commentID := comments[0].ID

	// First resolve → changed.
	changed, err := gr.SetGoalCommentResolved(ctx, scope, goalID, commentID, true, seedUserID)
	if err != nil || !changed {
		t.Fatalf("first resolve: changed=%v err=%v", changed, err)
	}
	comments, _ = gr.ListGoalComments(ctx, scope, goalID)
	firstAt := comments[0].ResolvedAt
	// Second resolve on an already-resolved comment → no change, resolved_at untouched.
	changed, err = gr.SetGoalCommentResolved(ctx, scope, goalID, commentID, true, seedUserID)
	if err != nil || changed {
		t.Fatalf("second resolve must be a no-op: changed=%v err=%v", changed, err)
	}
	comments, _ = gr.ListGoalComments(ctx, scope, goalID)
	if firstAt == nil || comments[0].ResolvedAt == nil || !firstAt.Equal(*comments[0].ResolvedAt) {
		t.Fatalf("resolved_at must not change on a repeated resolve: %v vs %v", firstAt, comments[0].ResolvedAt)
	}
	// Reopen twice: first changes, second is a no-op.
	if changed, _ := gr.SetGoalCommentResolved(ctx, scope, goalID, commentID, false, seedUserID); !changed {
		t.Fatalf("first reopen must change")
	}
	if changed, _ := gr.SetGoalCommentResolved(ctx, scope, goalID, commentID, false, seedUserID); changed {
		t.Fatalf("second reopen must be a no-op")
	}
}

func TestListGoalCommentsByGoals(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()

	gr := goals.NewGoalRepository(pool, krs.NewKRRepository(pool))
	tr := teams.NewTeamRepository(pool)
	pr := periods.NewPeriodRepository(pool)
	scope := domain.TenantScope{TenantID: 1}

	_, _, goalID := seedGoal(t, ctx, gr, tr, pr, scope, "by-goals")
	if _, err := gr.AddGoalComment(ctx, scope, goalID, "открытый", seedUserID); err != nil {
		t.Fatalf("add open: %v", err)
	}
	if _, err := gr.AddGoalComment(ctx, scope, goalID, "решённый", seedUserID); err != nil {
		t.Fatalf("add resolved: %v", err)
	}
	comments, _ := gr.ListGoalComments(ctx, scope, goalID)
	// Resolve one of the two comments.
	if _, err := gr.SetGoalCommentResolved(ctx, scope, goalID, comments[0].ID, true, seedUserID); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	byGoal, err := gr.ListGoalCommentsByGoals(ctx, scope, []int64{goalID})
	if err != nil {
		t.Fatalf("ListGoalCommentsByGoals: %v", err)
	}
	if len(byGoal[goalID]) != 2 {
		t.Fatalf("expected 2 comments for goal, got %d", len(byGoal[goalID]))
	}
	var open, resolved int
	for _, c := range byGoal[goalID] {
		if c.ResolvedAt == nil {
			open++
		} else {
			resolved++
		}
	}
	if open != 1 || resolved != 1 {
		t.Fatalf("expected 1 open + 1 resolved, got %d/%d", open, resolved)
	}

	// Empty input → empty result, no error.
	empty, err := gr.ListGoalCommentsByGoals(ctx, scope, nil)
	if err != nil {
		t.Fatalf("empty input: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected empty map for nil goal IDs, got %d", len(empty))
	}
}

func TestSetGoalCommentResolved(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()

	gr := goals.NewGoalRepository(pool, krs.NewKRRepository(pool))
	tr := teams.NewTeamRepository(pool)
	pr := periods.NewPeriodRepository(pool)
	scope := domain.TenantScope{TenantID: 1}

	_, _, goalID := seedGoal(t, ctx, gr, tr, pr, scope, "resolve")
	if _, err := gr.AddGoalComment(ctx, scope, goalID, "blocker", seedUserID); err != nil {
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
	if _, err := gr.SetGoalCommentResolved(ctx, scope, goalID, commentID, true, seedUserID); err != nil {
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
	if _, err := gr.SetGoalCommentResolved(ctx, scope, goalID, commentID, false, seedUserID); err != nil {
		t.Fatalf("unresolve: %v", err)
	}
	comments, _ = gr.ListGoalComments(ctx, scope, goalID)
	if comments[0].ResolvedAt != nil || comments[0].ResolvedByName != "" {
		t.Fatalf("reopen must clear resolve fields, got %+v", comments[0])
	}

	// Unknown comment id → ErrNotFound.
	if _, err := gr.SetGoalCommentResolved(ctx, scope, goalID, 999999, true, seedUserID); !errors.Is(err, goals.ErrNotFound) {
		t.Fatalf("unknown comment: want ErrNotFound, got %v", err)
	}

	// Comment cannot be resolved through a different goal id.
	if _, err := gr.SetGoalCommentResolved(ctx, scope, goalID+1, commentID, true, seedUserID); !errors.Is(err, goals.ErrNotFound) {
		t.Fatalf("wrong goal id: want ErrNotFound, got %v", err)
	}
}
