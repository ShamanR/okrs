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

func TestListGoalCommentsNestsRepliesInOrder(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	gr := goals.NewGoalRepository(pool, krs.NewKRRepository(pool))
	tr := teams.NewTeamRepository(pool)
	pr := periods.NewPeriodRepository(pool)
	scope := domain.TenantScope{TenantID: 1}
	_, _, goalID := seedGoal(t, ctx, gr, tr, pr, scope, "nest")

	// Two tasks, oldest first expected.
	t1, err := gr.AddGoalComment(ctx, scope, goalID, "task-1", seedUserID)
	if err != nil {
		t.Fatalf("task1: %v", err)
	}
	if _, err := gr.AddGoalComment(ctx, scope, goalID, "task-2", seedUserID); err != nil {
		t.Fatalf("task2: %v", err)
	}
	// Two replies under task-1.
	if _, err := gr.AddGoalReply(ctx, scope, goalID, t1, "reply-a", seedUserID); err != nil {
		t.Fatalf("reply-a: %v", err)
	}
	if _, err := gr.AddGoalReply(ctx, scope, goalID, t1, "reply-b", seedUserID); err != nil {
		t.Fatalf("reply-b: %v", err)
	}

	comments, err := gr.ListGoalComments(ctx, scope, goalID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(comments) != 2 {
		t.Fatalf("want 2 tasks (replies must be nested), got %d", len(comments))
	}
	if comments[0].Text != "task-1" || comments[1].Text != "task-2" {
		t.Fatalf("tasks must be oldest→newest: %q, %q", comments[0].Text, comments[1].Text)
	}
	if len(comments[0].Replies) != 2 {
		t.Fatalf("task-1 must have 2 replies, got %d", len(comments[0].Replies))
	}
	if comments[0].Replies[0].Text != "reply-a" || comments[0].Replies[1].Text != "reply-b" {
		t.Fatalf("replies must be oldest→newest: %q, %q", comments[0].Replies[0].Text, comments[0].Replies[1].Text)
	}
	if comments[0].Replies[0].ParentID == nil || *comments[0].Replies[0].ParentID != t1 {
		t.Fatalf("reply.ParentID must point to task-1")
	}
}

func TestAddGoalReplyRejectsNonTaskParent(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	gr := goals.NewGoalRepository(pool, krs.NewKRRepository(pool))
	tr := teams.NewTeamRepository(pool)
	pr := periods.NewPeriodRepository(pool)
	scope := domain.TenantScope{TenantID: 1}
	_, _, goalID := seedGoal(t, ctx, gr, tr, pr, scope, "reply-guard")

	task, _ := gr.AddGoalComment(ctx, scope, goalID, "task", seedUserID)
	reply, err := gr.AddGoalReply(ctx, scope, goalID, task, "reply", seedUserID)
	if err != nil {
		t.Fatalf("valid reply: %v", err)
	}
	// Replying to a reply must be rejected (depth 1 only).
	if _, err := gr.AddGoalReply(ctx, scope, goalID, reply, "nested", seedUserID); !errors.Is(err, goals.ErrNotFound) {
		t.Fatalf("reply-to-reply must be ErrNotFound, got %v", err)
	}
	// Replying to a non-existent parent must be rejected.
	if _, err := gr.AddGoalReply(ctx, scope, goalID, 999999, "orphan", seedUserID); !errors.Is(err, goals.ErrNotFound) {
		t.Fatalf("orphan reply must be ErrNotFound, got %v", err)
	}
}

func TestDeleteGoalCommentCascadesReplies(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	gr := goals.NewGoalRepository(pool, krs.NewKRRepository(pool))
	tr := teams.NewTeamRepository(pool)
	pr := periods.NewPeriodRepository(pool)
	scope := domain.TenantScope{TenantID: 1}
	_, _, goalID := seedGoal(t, ctx, gr, tr, pr, scope, "cascade")

	task, _ := gr.AddGoalComment(ctx, scope, goalID, "task", seedUserID)
	if _, err := gr.AddGoalReply(ctx, scope, goalID, task, "reply", seedUserID); err != nil {
		t.Fatalf("reply: %v", err)
	}
	if err := gr.DeleteGoalComment(ctx, scope, goalID, task); err != nil {
		t.Fatalf("delete task: %v", err)
	}
	comments, _ := gr.ListGoalComments(ctx, scope, goalID)
	if len(comments) != 0 {
		t.Fatalf("task + cascaded replies must be gone, got %d", len(comments))
	}
	// Deleting a missing comment → ErrNotFound.
	if err := gr.DeleteGoalComment(ctx, scope, goalID, task); !errors.Is(err, goals.ErrNotFound) {
		t.Fatalf("second delete must be ErrNotFound, got %v", err)
	}
}

func TestGetGoalCommentMeta(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	gr := goals.NewGoalRepository(pool, krs.NewKRRepository(pool))
	tr := teams.NewTeamRepository(pool)
	pr := periods.NewPeriodRepository(pool)
	scope := domain.TenantScope{TenantID: 1}
	_, _, goalID := seedGoal(t, ctx, gr, tr, pr, scope, "meta")

	task, _ := gr.AddGoalComment(ctx, scope, goalID, "task", seedUserID)
	reply, _ := gr.AddGoalReply(ctx, scope, goalID, task, "reply", seedUserID)

	author, isTask, err := gr.GetGoalCommentMeta(ctx, scope, goalID, task)
	if err != nil || author != seedUserID || !isTask {
		t.Fatalf("task meta: author=%d isTask=%v err=%v", author, isTask, err)
	}
	_, isTask, err = gr.GetGoalCommentMeta(ctx, scope, goalID, reply)
	if err != nil || isTask {
		t.Fatalf("reply meta must have isTask=false: isTask=%v err=%v", isTask, err)
	}
	if _, _, err := gr.GetGoalCommentMeta(ctx, scope, goalID, 999999); !errors.Is(err, goals.ErrNotFound) {
		t.Fatalf("missing meta must be ErrNotFound, got %v", err)
	}
}

func TestResolveRejectsReply(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	gr := goals.NewGoalRepository(pool, krs.NewKRRepository(pool))
	tr := teams.NewTeamRepository(pool)
	pr := periods.NewPeriodRepository(pool)
	scope := domain.TenantScope{TenantID: 1}
	_, _, goalID := seedGoal(t, ctx, gr, tr, pr, scope, "resolve-reply")

	task, _ := gr.AddGoalComment(ctx, scope, goalID, "task", seedUserID)
	reply, _ := gr.AddGoalReply(ctx, scope, goalID, task, "reply", seedUserID)
	if _, err := gr.SetGoalCommentResolved(ctx, scope, goalID, reply, true, seedUserID); !errors.Is(err, goals.ErrNotFound) {
		t.Fatalf("resolving a reply must be ErrNotFound, got %v", err)
	}
}
