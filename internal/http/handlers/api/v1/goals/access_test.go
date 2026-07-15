package goals_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"okrs/internal/domain"
	"okrs/internal/http/handlers/api/v1/testutil"
	"okrs/internal/service"
	"okrs/internal/store"
	"okrs/internal/store/goals"
	"okrs/internal/store/grants"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupGoalAccessDB(t *testing.T) (*pgxpool.Pool, *store.Store, func()) {
	t.Helper()
	ctx := context.Background()
	container, err := tcpostgres.RunContainer(ctx,
		tcpostgres.WithDatabase("okrs"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(10*time.Second),
		),
	)
	testutil.RequireDockerOrSkip(t, err)

	dbURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("conn string: %v", err)
	}
	if err := testutil.RunMigrations(dbURL); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	return pool, store.New(pool), func() {
		pool.Close()
		_ = container.Terminate(ctx)
	}
}

func TestAddGoalCommentAccessControl(t *testing.T) {
	pool, repo, teardown := setupGoalAccessDB(t)
	defer teardown()
	ctx := context.Background()

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
	goalID, err := repo.Goals.CreateGoal(ctx, domain.TenantScope{TenantID: 1}, goals.GoalInput{
		TeamID: teamID, PeriodID: periodID, Title: "Goal",
		Priority: domain.PriorityP1, Weight: 100,
		WorkType: domain.WorkTypeDelivery, FocusType: domain.FocusStability,
	})
	if err != nil {
		t.Fatalf("create goal: %v", err)
	}

	svc := service.NewFromStore(repo, grants.NewGrantsCache(repo.Grants), nil, nil)
	payload, _ := json.Marshal(map[string]string{"text": "comment"})

	t.Run("denied when user has no access to team", func(t *testing.T) {
		server := httptest.NewServer(testutil.NewAPIV1RouterWithScope(svc, []int64{}))
		defer server.Close()
		resp, err := http.Post(fmt.Sprintf("%s/api/v1/goals/%d/comments", server.URL, goalID),
			"application/json", bytes.NewBuffer(payload))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("denied when allowed team list does not include goal team", func(t *testing.T) {
		server := httptest.NewServer(testutil.NewAPIV1RouterWithScope(svc, []int64{teamID + 999}))
		defer server.Close()
		resp, err := http.Post(fmt.Sprintf("%s/api/v1/goals/%d/comments", server.URL, goalID),
			"application/json", bytes.NewBuffer(payload))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("allowed when team is in scope", func(t *testing.T) {
		server := httptest.NewServer(testutil.NewAPIV1RouterWithScope(svc, []int64{teamID}))
		defer server.Close()
		resp, err := http.Post(fmt.Sprintf("%s/api/v1/goals/%d/comments", server.URL, goalID),
			"application/json", bytes.NewBuffer(payload))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("allowed for admin (nil scope)", func(t *testing.T) {
		server := httptest.NewServer(testutil.NewAPIV1RouterWithScope(svc, nil))
		defer server.Close()
		resp, err := http.Post(fmt.Sprintf("%s/api/v1/goals/%d/comments", server.URL, goalID),
			"application/json", bytes.NewBuffer(payload))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
	})
}
