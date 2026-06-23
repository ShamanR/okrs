package goals_test

import (
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
	"okrs/internal/store/shares"
)

// TestLeaveGoalShareRemovesGoalFromTeamOnly verifies that DELETE /goals/{id}/share/{teamID}
// (turning off "общая цель" for a shared, non-owner team) removes the goal from that team's
// list only — the owner and the other participating teams keep it.
func TestLeaveGoalShareRemovesGoalFromTeamOnly(t *testing.T) {
	pool, repo, teardown := setupGoalAccessDB(t)
	defer teardown()
	ctx := context.Background()

	mustTeam := func(name string) int64 {
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ($1) RETURNING id`, name).Scan(&id); err != nil {
			t.Fatalf("insert team %s: %v", name, err)
		}
		return id
	}
	ownerTeam := mustTeam("Owner")
	teamB := mustTeam("TeamB")
	teamC := mustTeam("TeamC")
	var periodID int64
	if err := pool.QueryRow(ctx, `INSERT INTO periods (name,start_date,end_date,sort_order) VALUES ('Q1','2026-01-01','2026-03-31',1) RETURNING id`).Scan(&periodID); err != nil {
		t.Fatal(err)
	}
	goalID, err := repo.Goals.CreateGoal(ctx, domain.TenantScope{TenantID: 1}, goals.GoalInput{
		TeamID: ownerTeam, PeriodID: periodID, Title: "Shared goal",
		Priority: domain.PriorityP1, Weight: 100,
		WorkType: domain.WorkTypeDelivery, FocusType: domain.FocusStability,
	})
	if err != nil {
		t.Fatal(err)
	}
	sr := shares.NewGoalShareRepository(pool)
	if err := sr.ReplaceGoalShares(ctx, goalID, []shares.GoalShareInput{
		{TeamID: teamB, Weight: 50},
		{TeamID: teamC, Weight: 30},
	}); err != nil {
		t.Fatal(err)
	}

	svc := service.NewFromStore(repo, grants.NewGrantsCache(repo.Grants), nil)
	server := httptest.NewServer(testutil.NewAPIV1RouterWithScope(svc, nil))
	defer server.Close()

	sees := func(teamID int64) bool {
		resp, err := http.Get(fmt.Sprintf("%s/api/v1/teams/%d/okrs?period_id=%d", server.URL, teamID, periodID))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var payload struct {
			Goals []struct {
				ID int64 `json:"id"`
			} `json:"goals"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("decode okrs for team %d: %v", teamID, err)
		}
		for _, g := range payload.Goals {
			if g.ID == goalID {
				return true
			}
		}
		return false
	}

	if !sees(teamB) || !sees(teamC) || !sees(ownerTeam) {
		t.Fatalf("precondition: all teams should see the goal")
	}

	// Team B turns off "общая цель" → DELETE share for itself. Must not 404.
	req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/v1/goals/%d/share/%d", server.URL, goalID, teamB), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("leave share: status %d, want 200", resp.StatusCode)
	}

	if sees(teamB) {
		t.Fatalf("team B should no longer see the goal after leaving the share")
	}
	if !sees(teamC) {
		t.Fatalf("team C must still see the goal")
	}
	if !sees(ownerTeam) {
		t.Fatalf("owner must still see the goal")
	}
}

// TestLeaveGoalShareUnattachedTeamReturns404 verifies that detaching a team that is neither the
// owner nor a shared participant is rejected with 404 (rather than a misleading 200 no-op), and
// that a non-existent goal also returns 404 — so the endpoint can't be used as an existence
// oracle for arbitrary goal IDs.
func TestLeaveGoalShareUnattachedTeamReturns404(t *testing.T) {
	pool, repo, teardown := setupGoalAccessDB(t)
	defer teardown()
	ctx := context.Background()

	mustTeam := func(name string) int64 {
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ($1) RETURNING id`, name).Scan(&id); err != nil {
			t.Fatalf("insert team %s: %v", name, err)
		}
		return id
	}
	ownerTeam := mustTeam("Owner")
	teamB := mustTeam("TeamB")
	unattached := mustTeam("Unattached")
	var periodID int64
	if err := pool.QueryRow(ctx, `INSERT INTO periods (name,start_date,end_date,sort_order) VALUES ('Q1','2026-01-01','2026-03-31',1) RETURNING id`).Scan(&periodID); err != nil {
		t.Fatal(err)
	}
	goalID, err := repo.Goals.CreateGoal(ctx, domain.TenantScope{TenantID: 1}, goals.GoalInput{
		TeamID: ownerTeam, PeriodID: periodID, Title: "Shared goal",
		Priority: domain.PriorityP1, Weight: 100,
		WorkType: domain.WorkTypeDelivery, FocusType: domain.FocusStability,
	})
	if err != nil {
		t.Fatal(err)
	}
	sr := shares.NewGoalShareRepository(pool)
	if err := sr.ReplaceGoalShares(ctx, goalID, []shares.GoalShareInput{{TeamID: teamB, Weight: 50}}); err != nil {
		t.Fatal(err)
	}

	svc := service.NewFromStore(repo, grants.NewGrantsCache(repo.Grants), nil)
	server := httptest.NewServer(testutil.NewAPIV1RouterWithScope(svc, nil)) // admin scope: passes the team-access check
	defer server.Close()

	del := func(gID, tID int64) int {
		req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/v1/goals/%d/share/%d", server.URL, gID, tID), nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	// Team that is neither owner nor shared → 404, not a 200 no-op.
	if code := del(goalID, unattached); code != http.StatusNotFound {
		t.Fatalf("detach unattached team: status %d, want 404", code)
	}
	// Non-existent goal → 404 (no existence oracle).
	if code := del(goalID+9999, ownerTeam); code != http.StatusNotFound {
		t.Fatalf("detach on missing goal: status %d, want 404", code)
	}

	// Nothing should have been detached.
	shareList, err := sr.ListGoalShares(ctx, goalID)
	if err != nil {
		t.Fatal(err)
	}
	if len(shareList) != 1 || shareList[0].TeamID != teamB {
		t.Fatalf("shares unexpectedly changed: %+v", shareList)
	}
}

// TestDetachOwnerFromSharedGoalKeepsItForOthers verifies that detaching the OWNER team from a
// shared goal (delete pressed in the owner's context) removes it from the owner's list while the
// goal lives on for the remaining participating teams (ownership transfers to one of them).
func TestDetachOwnerFromSharedGoalKeepsItForOthers(t *testing.T) {
	pool, repo, teardown := setupGoalAccessDB(t)
	defer teardown()
	ctx := context.Background()

	mustTeam := func(name string) int64 {
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ($1) RETURNING id`, name).Scan(&id); err != nil {
			t.Fatalf("insert team %s: %v", name, err)
		}
		return id
	}
	ownerTeam := mustTeam("Owner")
	teamB := mustTeam("TeamB")
	teamC := mustTeam("TeamC")
	var periodID int64
	if err := pool.QueryRow(ctx, `INSERT INTO periods (name,start_date,end_date,sort_order) VALUES ('Q1','2026-01-01','2026-03-31',1) RETURNING id`).Scan(&periodID); err != nil {
		t.Fatal(err)
	}
	goalID, err := repo.Goals.CreateGoal(ctx, domain.TenantScope{TenantID: 1}, goals.GoalInput{
		TeamID: ownerTeam, PeriodID: periodID, Title: "Shared goal",
		Priority: domain.PriorityP1, Weight: 100,
		WorkType: domain.WorkTypeDelivery, FocusType: domain.FocusStability,
	})
	if err != nil {
		t.Fatal(err)
	}
	sr := shares.NewGoalShareRepository(pool)
	if err := sr.ReplaceGoalShares(ctx, goalID, []shares.GoalShareInput{
		{TeamID: teamB, Weight: 50},
		{TeamID: teamC, Weight: 30},
	}); err != nil {
		t.Fatal(err)
	}

	svc := service.NewFromStore(repo, grants.NewGrantsCache(repo.Grants), nil)
	server := httptest.NewServer(testutil.NewAPIV1RouterWithScope(svc, nil))
	defer server.Close()

	sees := func(teamID int64) bool {
		resp, err := http.Get(fmt.Sprintf("%s/api/v1/teams/%d/okrs?period_id=%d", server.URL, teamID, periodID))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var payload struct {
			Goals []struct {
				ID int64 `json:"id"`
			} `json:"goals"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("decode okrs for team %d: %v", teamID, err)
		}
		for _, g := range payload.Goals {
			if g.ID == goalID {
				return true
			}
		}
		return false
	}

	// Detach the owner team (delete pressed in owner context).
	req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/v1/goals/%d/share/%d", server.URL, goalID, ownerTeam), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("detach owner: status %d, want 200", resp.StatusCode)
	}

	if sees(ownerTeam) {
		t.Fatalf("owner should no longer see the goal after detaching itself")
	}
	if !sees(teamB) || !sees(teamC) {
		t.Fatalf("remaining teams B and C must still see the goal (B sees=%v, C sees=%v)", sees(teamB), sees(teamC))
	}
}
