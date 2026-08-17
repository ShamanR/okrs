package goals_test

import (
	"bytes"
	"context"
	"encoding/json"
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

func postLinks(t *testing.T, url string, goalID int64, parentIDs []int64) *http.Response {
	t.Helper()
	body, _ := json.Marshal(map[string][]int64{"parent_goal_ids": parentIDs})
	resp, err := http.Post(fmt.Sprintf("%s/api/v1/goals/%d/links", url, goalID), "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST links: %v", err)
	}
	return resp
}

func TestSetGoalParents_Contract(t *testing.T) {
	pool, repo, teardown := setupGoalAccessDB(t)
	defer teardown()
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}

	mustTeam := func(name string) int64 {
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO teams (name, team_type) VALUES ($1,'unit') RETURNING id`, name).Scan(&id); err != nil {
			t.Fatalf("insert team: %v", err)
		}
		return id
	}
	teamMain := mustTeam("Main")
	teamOther := mustTeam("Other")
	var periodID int64
	if err := pool.QueryRow(ctx, `INSERT INTO periods (name, start_date, end_date) VALUES ('Q1','2026-01-01','2026-03-31') RETURNING id`).Scan(&periodID); err != nil {
		t.Fatalf("period: %v", err)
	}
	newGoal := func(teamID int64, title string) int64 {
		id, err := repo.Goals.CreateGoal(ctx, scope, goals.GoalInput{
			TeamID: teamID, PeriodID: periodID, Title: title,
			Priority: domain.PriorityP1, Weight: 100,
			WorkType: domain.WorkTypeDelivery, FocusType: domain.FocusStability,
		})
		if err != nil {
			t.Fatalf("create goal: %v", err)
		}
		return id
	}
	child := newGoal(teamMain, "child")
	parent := newGoal(teamMain, "parent")
	otherGoal := newGoal(teamOther, "other")

	svc := service.NewFromStore(repo, grants.NewGrantsCache(repo.Grants), nil, nil)

	// Admin scope: happy path 204 + read-back.
	admin := httptest.NewServer(testutil.NewAPIV1RouterWithScope(svc, nil))
	defer admin.Close()

	if resp := postLinks(t, admin.URL, child, []int64{parent}); resp.StatusCode != http.StatusNoContent {
		resp.Body.Close()
		t.Fatalf("set parents status = %d, want 204", resp.StatusCode)
	}

	// Read back via GET /goals/{child}.
	gresp, err := http.Get(fmt.Sprintf("%s/api/v1/goals/%d", admin.URL, child))
	if err != nil {
		t.Fatalf("GET goal: %v", err)
	}
	var goalResp struct {
		Goal struct {
			Parents []struct {
				ID int64 `json:"id"`
			} `json:"parents"`
		} `json:"goal"`
	}
	if err := json.NewDecoder(gresp.Body).Decode(&goalResp); err != nil {
		t.Fatalf("decode goal: %v", err)
	}
	gresp.Body.Close()
	if len(goalResp.Goal.Parents) != 1 || goalResp.Goal.Parents[0].ID != parent {
		t.Fatalf("goal parents = %+v, want [%d]", goalResp.Goal.Parents, parent)
	}

	// Self-link → 400.
	if resp := postLinks(t, admin.URL, child, []int64{child}); resp.StatusCode != http.StatusBadRequest {
		resp.Body.Close()
		t.Fatalf("self-link status = %d, want 400", resp.StatusCode)
	}

	// Cycle: parent -> child (reverse) → 409.
	if resp := postLinks(t, admin.URL, parent, []int64{child}); resp.StatusCode != http.StatusConflict {
		resp.Body.Close()
		t.Fatalf("cycle status = %d, want 409", resp.StatusCode)
	}

	// Scoped to teamMain only: linking to a parent in teamOther → 400 (not accessible).
	scoped := httptest.NewServer(testutil.NewAPIV1RouterWithScope(svc, []int64{teamMain}))
	defer scoped.Close()
	if resp := postLinks(t, scoped.URL, child, []int64{otherGoal}); resp.StatusCode != http.StatusBadRequest {
		resp.Body.Close()
		t.Fatalf("out-of-scope parent status = %d, want 400", resp.StatusCode)
	}

	// Child owner team out of scope → 404. Scope only to teamOther, target child in teamMain.
	scopedOther := httptest.NewServer(testutil.NewAPIV1RouterWithScope(svc, []int64{teamOther}))
	defer scopedOther.Close()
	if resp := postLinks(t, scopedOther.URL, child, []int64{otherGoal}); resp.StatusCode != http.StatusNotFound {
		resp.Body.Close()
		t.Fatalf("child out-of-scope status = %d, want 404", resp.StatusCode)
	}
}

func TestLinkableGoals_Contract(t *testing.T) {
	pool, repo, teardown := setupGoalAccessDB(t)
	defer teardown()
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}

	var teamID int64
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name, team_type) VALUES ('Платформа','unit') RETURNING id`).Scan(&teamID); err != nil {
		t.Fatalf("team: %v", err)
	}
	var periodID int64
	if err := pool.QueryRow(ctx, `INSERT INTO periods (name, start_date, end_date) VALUES ('Q1','2026-01-01','2026-03-31') RETURNING id`).Scan(&periodID); err != nil {
		t.Fatalf("period: %v", err)
	}
	newGoal := func(title string) int64 {
		id, err := repo.Goals.CreateGoal(ctx, scope, goals.GoalInput{
			TeamID: teamID, PeriodID: periodID, Title: title,
			Priority: domain.PriorityP1, Weight: 100,
			WorkType: domain.WorkTypeDelivery, FocusType: domain.FocusStability,
		})
		if err != nil {
			t.Fatalf("create goal: %v", err)
		}
		return id
	}
	self := newGoal("self")
	other := newGoal("Снизить Time-to-Deploy")

	svc := service.NewFromStore(repo, grants.NewGrantsCache(repo.Grants), nil, nil)
	admin := httptest.NewServer(testutil.NewAPIV1RouterWithScope(svc, nil))
	defer admin.Close()

	resp, err := http.Get(fmt.Sprintf("%s/api/v1/goals/linkable?exclude_goal_id=%d&q=time-to-deploy", admin.URL, self))
	if err != nil {
		t.Fatalf("GET linkable: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("linkable status = %d, want 200", resp.StatusCode)
	}
	var items []struct {
		ID       int64  `json:"id"`
		Title    string `json:"title"`
		TeamName string `json:"team_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		t.Fatalf("decode linkable: %v", err)
	}
	if len(items) != 1 || items[0].ID != other {
		t.Fatalf("linkable = %+v, want [%d]", items, other)
	}
	if items[0].TeamName != "Платформа" {
		t.Fatalf("linkable team = %q, want Платформа", items[0].TeamName)
	}
}
