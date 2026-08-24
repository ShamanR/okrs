package goals_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"okrs/internal/core/domain"
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
	if _, err := repo.Goals.AddGoalComment(ctx, scope, goalID, "blocker", 1); err != nil {
		t.Fatalf("add comment: %v", err)
	}
	comments, err := repo.Goals.ListGoalComments(ctx, scope, goalID)
	if err != nil || len(comments) != 1 {
		t.Fatalf("list comments: err=%v n=%d", err, len(comments))
	}
	commentID := comments[0].ID

	svc := service.NewFromStore(repo, grants.NewGrantsCache(repo.Grants), nil, nil)
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

// A goal shared into team B must be resolvable by a user who can access team B
// but NOT the owner team A — the resolve button renders on B's card too.
func TestResolveGoalCommentOnSharedGoal(t *testing.T) {
	pool, repo, teardown := setupGoalAccessDB(t)
	defer teardown()
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}

	var ownerTeam, sharedTeam int64
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('Owner') RETURNING id`).Scan(&ownerTeam); err != nil {
		t.Fatalf("insert owner team: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('Shared') RETURNING id`).Scan(&sharedTeam); err != nil {
		t.Fatalf("insert shared team: %v", err)
	}
	var periodID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO periods (name, start_date, end_date) VALUES ('Q1', '2024-01-01', '2024-03-31') RETURNING id`).
		Scan(&periodID); err != nil {
		t.Fatalf("insert period: %v", err)
	}
	goalID, err := repo.Goals.CreateGoal(ctx, scope, goals.GoalInput{
		TeamID: ownerTeam, PeriodID: periodID, Title: "Goal",
		Priority: domain.PriorityP1, Weight: 100,
		WorkType: domain.WorkTypeDelivery, FocusType: domain.FocusStability,
	})
	if err != nil {
		t.Fatalf("create goal: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO goal_shares (goal_id, team_id, weight, sort_order) VALUES ($1,$2,50,0)`, goalID, sharedTeam); err != nil {
		t.Fatalf("share goal: %v", err)
	}
	if _, err := repo.Goals.AddGoalComment(ctx, scope, goalID, "blocker", 1); err != nil {
		t.Fatalf("add comment: %v", err)
	}
	comments, _ := repo.Goals.ListGoalComments(ctx, scope, goalID)
	commentID := comments[0].ID

	svc := service.NewFromStore(repo, grants.NewGrantsCache(repo.Grants), nil, nil)
	resolveURL := fmt.Sprintf("/api/v1/goals/%d/comments/%d/resolve", goalID, commentID)

	postWithScope := func(allowed []int64, path string) int {
		server := httptest.NewServer(testutil.NewAPIV1RouterWithScope(svc, allowed))
		defer server.Close()
		resp, err := http.Post(server.URL+path, "application/json", bytes.NewBufferString("{}"))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	// Scope limited to the shared-into team (not the owner) can resolve.
	if code := postWithScope([]int64{sharedTeam}, resolveURL); code != http.StatusOK {
		t.Fatalf("shared-team scope: expected 200, got %d", code)
	}
	// A wholly unrelated team cannot.
	if code := postWithScope([]int64{sharedTeam + 999}, resolveURL); code != http.StatusNotFound {
		t.Fatalf("unrelated scope: expected 404, got %d", code)
	}
}
