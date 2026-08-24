package activity_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"okrs/internal/core/domain"
	apiv1testutil "okrs/internal/http/handlers/api/v1/testutil"
	"okrs/internal/service"
	"okrs/internal/store"
	"okrs/internal/store/periods"
	"okrs/internal/store/teams"
	storetestutil "okrs/internal/store/testutil"

	"net/http/httptest"
)

func TestActivityFeedAndTreeCountsEndpoints(t *testing.T) {
	pool, cleanup := storetestutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}
	repo := store.New(pool)

	teamID, err := repo.Teams.CreateTeam(ctx, scope, teams.TeamInput{Name: "Платформа", Type: domain.TeamTypeTeam})
	if err != nil {
		t.Fatalf("team: %v", err)
	}
	periodID, err := repo.Periods.CreatePeriod(ctx, scope, periods.PeriodInput{
		Name: "Q1", StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), EndDate: time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("period: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := repo.Activity.Record(ctx, scope, domain.ActivityEvent{
			ActorUserID: 1, Category: domain.ActivityStatus, Action: domain.ActionStatusChanged,
			TeamID: &teamID, PeriodID: &periodID, EntityTitle: "Платформа",
		}); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
	}

	svc := service.NewFromStore(repo, store.NewGrantsCache(repo.Grants), nil, nil)
	srv := httptest.NewServer(apiv1testutil.NewAPIV1RouterWithScope(svc, []int64{teamID}))
	defer srv.Close()

	// Feed
	resp, err := http.Get(srv.URL + "/api/v1/activity")
	if err != nil {
		t.Fatalf("get feed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("feed status: %d", resp.StatusCode)
	}
	var feed struct {
		Items []struct {
			Action string `json:"action"`
			Target *struct {
				TeamID int64 `json:"team_id"`
			} `json:"target"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&feed); err != nil {
		t.Fatalf("decode feed: %v", err)
	}
	if len(feed.Items) != 2 {
		t.Fatalf("want 2 items, got %d", len(feed.Items))
	}
	if feed.Items[0].Target == nil || feed.Items[0].Target.TeamID != teamID {
		t.Fatalf("target not resolved: %+v", feed.Items[0])
	}

	// Tree counts
	resp2, err := http.Get(srv.URL + "/api/v1/activity/tree-counts")
	if err != nil {
		t.Fatalf("get tree-counts: %v", err)
	}
	defer resp2.Body.Close()
	var tc struct {
		Counts map[string]int `json:"counts"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&tc); err != nil {
		t.Fatalf("decode tree-counts: %v", err)
	}
	if tc.Counts[strconv.FormatInt(teamID, 10)] != 2 {
		t.Fatalf("want count 2 for team, got %+v", tc.Counts)
	}
}
