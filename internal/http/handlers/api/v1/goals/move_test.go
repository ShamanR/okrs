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
	"okrs/internal/store/shares"
)

// TestMoveGoalReadsTeamIDFromJSON replays the exact request the tracker SPA makes
// (POST /api/v1/goals/{id}/move-up with a JSON team_id body) and verifies the
// handler parses team_id and reorders the shared goal within that team's view.
func TestMoveGoalReadsTeamIDFromJSON(t *testing.T) {
	pool, repo, teardown := setupGoalAccessDB(t)
	defer teardown()
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}

	var receiver, owner, periodID int64
	mustTeam := func(name string) int64 {
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ($1) RETURNING id`, name).Scan(&id); err != nil {
			t.Fatalf("insert team %s: %v", name, err)
		}
		return id
	}
	receiver = mustTeam("Receiver")
	owner = mustTeam("Owner")
	if err := pool.QueryRow(ctx, `INSERT INTO periods (name, start_date, end_date) VALUES ('Q1','2026-01-01','2026-03-31') RETURNING id`).Scan(&periodID); err != nil {
		t.Fatal(err)
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
	goalA := newGoal(receiver, "A")
	shared := newGoal(owner, "S")
	sr := shares.NewGoalShareRepository(pool)
	if err := sr.ReplaceGoalShares(ctx, scope, shared, []shares.GoalShareInput{{TeamID: receiver, Weight: 100}}); err != nil {
		t.Fatal(err)
	}

	svc := service.NewFromStore(repo, grants.NewGrantsCache(repo.Grants), nil)
	server := httptest.NewServer(testutil.NewAPIV1RouterWithScope(svc, nil)) // admin scope
	defer server.Close()

	// The tracker SPA sends team_id as a JSON body via apiPost.
	body := bytes.NewReader([]byte(fmt.Sprintf(`{"team_id": %d}`, receiver)))
	resp, err := http.Post(fmt.Sprintf("%s/api/v1/goals/%d/move-up", server.URL, shared), "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("move-up: status %d, want 200", resp.StatusCode)
	}

	list, err := repo.Goals.ListGoalsByTeamPeriod(ctx, scope, receiver, periodID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != shared || list[1].ID != goalA {
		t.Fatalf("after move-up order = %+v, want [S A]", list)
	}
}
