package notifications_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"okrs/internal/core/domain"
	apiv1testutil "okrs/internal/http/handlers/api/v1/testutil"
	"okrs/internal/store"
	"okrs/internal/store/goals"
	"okrs/internal/store/grants"
	storetestutil "okrs/internal/store/testutil"
)

// TestGoalCommentNotifiesTeamLeadThroughRealBus drives a real mutation (posting a
// goal comment) through the fully assembled router and asserts a row lands in the
// notifications table for the goal's team lead.
//
// Every layer of the notifications feature — event publishing, the fan-out usecase,
// recipient resolution, the store insert — already has isolated tests. None of them
// can catch the wiring itself going missing: if the
// eventbus.SubscribeAll(bus, "notifications", ...) registration in httpdeps.Build
// were ever deleted, the comment POST below would still answer 200 (nothing on the
// HTTP path depends on the subscriber) and every other suite would stay green while
// the feature went silently dead in production. This mirrors resolve_test.go's
// TestResolveGoalComment, which closes the identical gap for the (synchronous)
// activity journal.
//
// Unlike the journal, the notifications subscriber is registered with
// eventbus.WithMode(eventbus.Async): the row is written on a background goroutine
// sometime after the HTTP response returns. The assertion below polls with a bounded
// wait instead of checking immediately after the POST, so a genuine regression fails
// in seconds rather than the test being flaky under load or hanging the suite.
func TestGoalCommentNotifiesTeamLeadThroughRealBus(t *testing.T) {
	pool, cleanup := storetestutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}
	repo := store.New(pool)

	// Recipient: the goal's owning team's lead. The fan-out resolves recipients
	// through the team tree (the goal's team lead, then ancestor leads) and
	// deliberately never notifies the actor about their own action — so the lead
	// must be a user distinct from whoever posts the comment.
	var leadID int64
	var leadUDID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('lead-e2e', 'system', 'lead-e2e', 'Team Lead')
		RETURNING id, udid`).Scan(&leadID, &leadUDID); err != nil {
		t.Fatalf("insert lead: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO memberships (user_id, tenant_id, role, status) VALUES ($1, 1, 'user', 'active')`,
		leadID); err != nil {
		t.Fatalf("insert lead membership: %v", err)
	}

	// Actor: a distinct user who will post the comment.
	var actorID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('actor-e2e', 'system', 'actor-e2e', 'Actor')
		RETURNING id`).Scan(&actorID); err != nil {
		t.Fatalf("insert actor: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO memberships (user_id, tenant_id, role, status) VALUES ($1, 1, 'user', 'active')`,
		actorID); err != nil {
		t.Fatalf("insert actor membership: %v", err)
	}

	var teamID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO teams (name, team_type, tenant_id, lead_udid) VALUES ('Team', 'team', 1, $1) RETURNING id`,
		leadUDID).Scan(&teamID); err != nil {
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

	gc := grants.NewGrantsCache(repo.Grants)
	// NewAPIV1RouterWithUser builds the real router, the real *eventbus.Bus, and
	// registers the real subscribers on it (via httpdeps.Build), then starts the
	// bus — the same assembly app.New uses in production. The actor is placed in
	// the request context so the comment's authorUserID is actorID, not the
	// anonymous system user.
	router := apiv1testutil.NewAPIV1RouterWithUser(t, repo, gc, []int64{teamID}, &domain.User{ID: actorID})
	server := httptest.NewServer(router)
	defer server.Close()

	resp, err := http.Post(
		fmt.Sprintf("%s/api/v1/goals/%d/comments", server.URL, goalID),
		"application/json",
		bytes.NewBufferString(`{"text":"blocker"}`),
	)
	if err != nil {
		t.Fatalf("post comment: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("post comment: expected 200, got %d", resp.StatusCode)
	}

	// The subscriber runs asynchronously (eventbus.WithMode(eventbus.Async)), so the
	// row may not exist yet when the response above returns. Poll with a bounded
	// wait rather than asserting immediately: this way a genuine regression fails
	// deterministically within ~2s instead of the test racing the background
	// goroutine and flaking on a slow CI box.
	const pollTimeout = 2 * time.Second
	const pollInterval = 50 * time.Millisecond
	deadline := time.Now().Add(pollTimeout)
	var count int
	for {
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM notifications
			  WHERE tenant_id = $1 AND user_id = $2 AND goal_id = $3 AND type = $4`,
			scope.TenantID, leadID, goalID, "goal_comment").Scan(&count); err != nil {
			t.Fatalf("count notifications: %v", err)
		}
		if count == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf(
				"want 1 goal_comment notification row for the team lead after polling for %s, got %d — "+
					"the notifications subscriber may not be wired to the usecase's bus",
				pollTimeout, count)
		}
		time.Sleep(pollInterval)
	}
}
