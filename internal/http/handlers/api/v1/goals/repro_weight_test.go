package goals_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
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

// TestSharedGoalWeightEditKeepsGoalVisible replays the exact sequence of API calls the
// GoalModal edit path (tracker.js) makes when a SHARED (non-owner) team changes the weight
// of a shared goal. Per docs/shared-goals.md, the edit must:
//   - change only the editing team's weight,
//   - leave the goal visible for the editing team (must not disappear),
//   - leave every other participating team's weight untouched.
func TestSharedGoalWeightEditKeepsGoalVisible(t *testing.T) {
	pool, repo, teardown := setupGoalAccessDB(t)
	defer teardown()
	ctx := context.Background()

	var ownerTeam, teamB, teamC, periodID int64
	mustInsertTeam := func(name string) int64 {
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ($1) RETURNING id`, name).Scan(&id); err != nil {
			t.Fatalf("insert team %s: %v", name, err)
		}
		return id
	}
	ownerTeam = mustInsertTeam("Owner")
	teamB = mustInsertTeam("TeamB")
	teamC = mustInsertTeam("TeamC")
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
	server := httptest.NewServer(testutil.NewAPIV1RouterWithScope(svc, nil)) // admin scope
	defer server.Close()

	weightFor := func(teamID int64) int {
		resp, err := http.Get(fmt.Sprintf("%s/api/v1/teams/%d/okrs?period_id=%d", server.URL, teamID, periodID))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var payload struct {
			Goals []struct {
				ID     int64 `json:"id"`
				Weight int   `json:"weight"`
			} `json:"goals"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("decode okrs for team %d: %v", teamID, err)
		}
		for _, g := range payload.Goals {
			if g.ID == goalID {
				return g.Weight
			}
		}
		return -1 // goal not visible for this team
	}

	// Call 1: POST /api/v1/goals/{id} with team_id = editing team, new weight (per-team weight update).
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("title", "Shared goal")
	_ = mw.WriteField("description", "")
	_ = mw.WriteField("priority", string(domain.PriorityP1))
	_ = mw.WriteField("work_type", string(domain.WorkTypeDelivery))
	_ = mw.WriteField("focus_type", string(domain.FocusStability))
	_ = mw.WriteField("weight", "70")
	_ = mw.WriteField("team_id", fmt.Sprintf("%d", teamB))
	mw.Close()
	resp, err := http.Post(fmt.Sprintf("%s/api/v1/goals/%d", server.URL, goalID), mw.FormDataContentType(), &body)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update goal: status %d", resp.StatusCode)
	}

	// Call 2: POST /api/v1/goals/{id}/share with the CORRECTED targets the fixed client sends:
	// the complete set of non-owner participants (editing team included, owner excluded), each
	// with its preserved weight (editing team carries the new weight).
	sharePayload, _ := json.Marshal(map[string]any{
		"targets": []map[string]any{
			{"team_id": teamB, "weight": 70}, // editing team, new weight
			{"team_id": teamC, "weight": 30}, // other team, preserved
		},
	})
	shResp, err := http.Post(fmt.Sprintf("%s/api/v1/goals/%d/share", server.URL, goalID), "application/json", bytes.NewReader(sharePayload))
	if err != nil {
		t.Fatal(err)
	}
	shResp.Body.Close()
	if shResp.StatusCode != http.StatusOK {
		t.Fatalf("share: status %d", shResp.StatusCode)
	}

	if w := weightFor(teamB); w != 70 {
		t.Fatalf("editing team B weight=%d, want 70 (goal disappeared if -1)", w)
	}
	if w := weightFor(teamC); w != 30 {
		t.Fatalf("other team C weight=%d, want 30 (must be untouched)", w)
	}
	if w := weightFor(ownerTeam); w != 100 {
		t.Fatalf("owner weight=%d, want 100 (must be untouched)", w)
	}
}
