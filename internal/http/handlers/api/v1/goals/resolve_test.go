package goals_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"okrs/internal/domain"
	"okrs/internal/http/handlers/api/v1/testutil"
	"okrs/internal/service"
	"okrs/internal/store/goals"
	"okrs/internal/store/grants"
)

func TestResolveGoalComment(t *testing.T) {
	pool, repo, teardown := setupGoalAccessDB(t)
	defer teardown()
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}

	var teamID int64
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('Team') RETURNING id`).Scan(&teamID); err != nil {
		t.Fatalf("insert team: %v", err)
	}
	var periodID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO periods (name, start_date, end_date) VALUES ('Q1', '2024-01-01', '2024-03-31') RETURNING id`).
		Scan(&periodID); err != nil {
		t.Fatalf("insert period: %v", err)
	}
	goalID, err := repo.Goals.CreateGoal(ctx, scope, goals.GoalInput{
		TeamID: teamID, PeriodID: periodID, Title: "Goal",
		Priority: domain.PriorityP1, Weight: 100,
		WorkType: domain.WorkTypeDelivery, FocusType: domain.FocusStability,
	})
	if err != nil {
		t.Fatalf("create goal: %v", err)
	}
	// author 1 is the seeded system user.
	if err := repo.Goals.AddGoalComment(ctx, scope, goalID, "blocker", 1); err != nil {
		t.Fatalf("add comment: %v", err)
	}
	comments, err := repo.Goals.ListGoalComments(ctx, scope, goalID)
	if err != nil || len(comments) != 1 {
		t.Fatalf("list comments: err=%v n=%d", err, len(comments))
	}
	commentID := comments[0].ID

	svc := service.NewFromStore(repo, grants.NewGrantsCache(repo.Grants), nil)
	server := httptest.NewServer(testutil.NewAPIV1RouterWithScope(svc, []int64{teamID}))
	defer server.Close()

	post := func(path string) int {
		resp, err := http.Post(server.URL+path, "application/json", bytes.NewBufferString("{}"))
		if err != nil {
			t.Fatalf("post %s: %v", path, err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	// Resolve the comment.
	if code := post(fmt.Sprintf("/api/v1/goals/%d/comments/%d/resolve", goalID, commentID)); code != http.StatusOK {
		t.Fatalf("resolve: expected 200, got %d", code)
	}
	comments, _ = repo.Goals.ListGoalComments(ctx, scope, goalID)
	if comments[0].ResolvedAt == nil {
		t.Fatalf("comment must be resolved after POST resolve")
	}

	// Reopen the comment.
	if code := post(fmt.Sprintf("/api/v1/goals/%d/comments/%d/unresolve", goalID, commentID)); code != http.StatusOK {
		t.Fatalf("unresolve: expected 200, got %d", code)
	}
	comments, _ = repo.Goals.ListGoalComments(ctx, scope, goalID)
	if comments[0].ResolvedAt != nil {
		t.Fatalf("comment must be unresolved after POST unresolve")
	}

	// Unknown comment id → 404.
	if code := post(fmt.Sprintf("/api/v1/goals/%d/comments/%d/resolve", goalID, 999999)); code != http.StatusNotFound {
		t.Fatalf("unknown comment: expected 404, got %d", code)
	}
}
