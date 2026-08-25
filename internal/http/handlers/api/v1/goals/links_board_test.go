package goals_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/api/v1/testutil"
	"okrs/internal/store/goals"
	"okrs/internal/store/grants"
)

// boardGoal is a minimal view of a goal in the /teams/{id}/okrs response.
type boardGoal struct {
	ID      int64 `json:"id"`
	Parents []struct {
		ID         int64  `json:"id"`
		Title      string `json:"title"`
		TeamName   string `json:"team_name"`
		TeamType   string `json:"team_type"`
		PeriodName string `json:"period_name"`
		Progress   int    `json:"progress"`
	} `json:"parents"`
	Children []struct {
		ID int64 `json:"id"`
	} `json:"children"`
}

type boardResponse struct {
	Goals []boardGoal `json:"goals"`
}

func fetchBoard(t *testing.T, url string, teamID, periodID int64) boardResponse {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("%s/api/v1/teams/%d/okrs?period_id=%d", url, teamID, periodID))
	if err != nil {
		t.Fatalf("GET okrs: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET okrs status = %d, want 200", resp.StatusCode)
	}
	var out boardResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode board: %v", err)
	}
	return out
}

func TestBoardEmbedsParentsScopeFiltered(t *testing.T) {
	pool, repo, teardown := setupGoalAccessDB(t)
	defer teardown()
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}

	mustTeam := func(name, typ string) int64 {
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO teams (name, team_type) VALUES ($1,$2) RETURNING id`, name, typ).Scan(&id); err != nil {
			t.Fatalf("insert team %s: %v", name, err)
		}
		return id
	}
	teamChild := mustTeam("Платформа", "unit")
	teamParent := mustTeam("Реклама", "cluster")

	var periodID int64
	if err := pool.QueryRow(ctx, `INSERT INTO periods (name, start_date, end_date) VALUES ('Q1 2026','2026-01-01','2026-03-31') RETURNING id`).Scan(&periodID); err != nil {
		t.Fatalf("insert period: %v", err)
	}
	newGoal := func(teamID int64, title string) int64 {
		id, err := repo.Goals.CreateGoal(ctx, scope, goals.GoalInput{
			TeamID: teamID, PeriodID: periodID, Title: title,
			Priority: domain.PriorityP1, Weight: 100,
			WorkType: domain.WorkTypeDelivery, FocusType: domain.FocusStability,
		})
		if err != nil {
			t.Fatalf("create goal %s: %v", title, err)
		}
		return id
	}
	child := newGoal(teamChild, "Снизить Time-to-Deploy")
	parent := newGoal(teamParent, "Эталонная платформа")
	if _, _, err := repo.GoalLinks.ReplaceParents(ctx, scope, nil, true, child, []int64{parent}); err != nil {
		t.Fatalf("link: %v", err)
	}

	gc := grants.NewGrantsCache(repo.Grants)

	// Admin scope (nil): parent in another team is visible.
	adminServer := httptest.NewServer(testutil.NewAPIV1RouterWithScope(repo, gc, nil))
	defer adminServer.Close()
	board := fetchBoard(t, adminServer.URL, teamChild, periodID)
	var childGoal *boardGoal
	for i := range board.Goals {
		if board.Goals[i].ID == child {
			childGoal = &board.Goals[i]
		}
	}
	if childGoal == nil {
		t.Fatalf("child goal %d not on board", child)
	}
	if len(childGoal.Parents) != 1 || childGoal.Parents[0].ID != parent {
		t.Fatalf("child parents = %+v, want [%d]", childGoal.Parents, parent)
	}
	if childGoal.Parents[0].TeamName != "Реклама" || childGoal.Parents[0].PeriodName != "Q1 2026" {
		t.Fatalf("parent ref = %+v, want team Реклама / period Q1 2026", childGoal.Parents[0])
	}
	if len(childGoal.Children) != 0 {
		t.Fatalf("child children = %+v, want empty", childGoal.Children)
	}

	// Scoped to teamChild only: parent (in teamParent) is hidden.
	scopedServer := httptest.NewServer(testutil.NewAPIV1RouterWithScope(repo, gc, []int64{teamChild}))
	defer scopedServer.Close()
	scopedBoard := fetchBoard(t, scopedServer.URL, teamChild, periodID)
	for i := range scopedBoard.Goals {
		if scopedBoard.Goals[i].ID == child && len(scopedBoard.Goals[i].Parents) != 0 {
			t.Fatalf("scoped child parents = %+v, want empty (out of scope)", scopedBoard.Goals[i].Parents)
		}
	}
}
