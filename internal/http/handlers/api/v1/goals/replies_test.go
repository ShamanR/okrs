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
	"okrs/internal/store/goals"
	"okrs/internal/store/grants"
)

func TestReplyAndDeleteHandlers(t *testing.T) {
	pool, repo, teardown := setupGoalAccessDB(t)
	defer teardown()
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}

	var teamID int64
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('Team') RETURNING id`).Scan(&teamID); err != nil {
		t.Fatalf("team: %v", err)
	}
	var periodID int64
	if err := pool.QueryRow(ctx, `INSERT INTO periods (name, start_date, end_date) VALUES ('Q1','2024-01-01','2024-03-31') RETURNING id`).Scan(&periodID); err != nil {
		t.Fatalf("period: %v", err)
	}
	goalID, err := repo.Goals.CreateGoal(ctx, scope, goals.GoalInput{
		TeamID: teamID, PeriodID: periodID, Title: "Goal",
		Priority: domain.PriorityP1, Weight: 100,
		WorkType: domain.WorkTypeDelivery, FocusType: domain.FocusStability,
	})
	if err != nil {
		t.Fatalf("goal: %v", err)
	}
	// Task authored by the request user (anonymous id=1) and one by user 2 (migration).
	ownTask, err := repo.Goals.AddGoalComment(ctx, scope, goalID, "mine", 1)
	if err != nil {
		t.Fatalf("own task: %v", err)
	}
	othersTask, err := repo.Goals.AddGoalComment(ctx, scope, goalID, "theirs", 2)
	if err != nil {
		t.Fatalf("others task: %v", err)
	}

	gc := grants.NewGrantsCache(repo.Grants)
	server := httptest.NewServer(testutil.NewAPIV1RouterWithScope(t, repo, gc, []int64{teamID}))
	defer server.Close()

	post := func(path, body string) int {
		resp, err := http.Post(server.URL+path, "application/json", bytes.NewBufferString(body))
		if err != nil {
			t.Fatalf("post %s: %v", path, err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}
	del := func(path string) int {
		req, _ := http.NewRequest(http.MethodDelete, server.URL+path, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("delete %s: %v", path, err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	base := fmt.Sprintf("/api/v1/goals/%d/comments", goalID)

	// Reply to my task → 200.
	if code := post(fmt.Sprintf("%s/%d/replies", base, ownTask), `{"text":"a reply"}`); code != http.StatusOK {
		t.Fatalf("reply: want 200 got %d", code)
	}
	// Fetch the reply id via store to attempt a reply-to-reply.
	comments, _ := repo.Goals.ListGoalComments(ctx, scope, goalID)
	var replyID int64
	for _, c := range comments {
		if c.ID == ownTask && len(c.Replies) > 0 {
			replyID = c.Replies[0].ID
		}
	}
	if replyID == 0 {
		t.Fatalf("reply was not created")
	}
	// Reply to a reply → 404.
	if code := post(fmt.Sprintf("%s/%d/replies", base, replyID), `{"text":"nested"}`); code != http.StatusNotFound {
		t.Fatalf("reply-to-reply: want 404 got %d", code)
	}
	// Empty reply text → 400.
	if code := post(fmt.Sprintf("%s/%d/replies", base, ownTask), `{"text":""}`); code != http.StatusBadRequest {
		t.Fatalf("empty reply: want 400 got %d", code)
	}
	// Resolve a reply → 404 (reply is not resolvable).
	if code := post(fmt.Sprintf("%s/%d/resolve", base, replyID), `{}`); code != http.StatusNotFound {
		t.Fatalf("resolve reply: want 404 got %d", code)
	}
	// Delete someone else's task as non-admin (request user is 1, author is 2) → 403.
	if code := del(fmt.Sprintf("%s/%d", base, othersTask)); code != http.StatusForbidden {
		t.Fatalf("delete others: want 403 got %d", code)
	}
	// Delete my own task → 200 (cascades my reply).
	if code := del(fmt.Sprintf("%s/%d", base, ownTask)); code != http.StatusOK {
		t.Fatalf("delete mine: want 200 got %d", code)
	}
	// Delete again → 404.
	if code := del(fmt.Sprintf("%s/%d", base, ownTask)); code != http.StatusNotFound {
		t.Fatalf("delete missing: want 404 got %d", code)
	}
	// The cascade removed the reply too: only the other user's task remains.
	comments, _ = repo.Goals.ListGoalComments(ctx, scope, goalID)
	if len(comments) != 1 || comments[0].ID != othersTask {
		t.Fatalf("after cascade only othersTask must remain, got %+v", comments)
	}
}
