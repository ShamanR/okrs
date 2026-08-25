package krs_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/api/v1/testutil"
	"okrs/internal/store"
	"okrs/internal/store/goals"
	"okrs/internal/store/grants"
	"okrs/internal/store/krs"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupKRAccessDB(t *testing.T) (*pgxpool.Pool, *store.Store, func()) {
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

// buildKRAccessFixture creates a team, period, goal, and percent KR; returns their IDs.
func buildKRAccessFixture(t *testing.T, pool *pgxpool.Pool, repo *store.Store) (teamID, periodID, goalID, krID int64) {
	t.Helper()
	ctx := context.Background()
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('Team') RETURNING id`).Scan(&teamID); err != nil {
		t.Fatalf("insert team: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO periods (name, start_date, end_date) VALUES ('Q1', '2024-01-01', '2024-03-31') RETURNING id`).
		Scan(&periodID); err != nil {
		t.Fatalf("insert period: %v", err)
	}
	var err error
	goalID, err = repo.Goals.CreateGoal(ctx, domain.TenantScope{TenantID: 1}, goals.GoalInput{
		TeamID: teamID, PeriodID: periodID, Title: "Goal",
		Priority: domain.PriorityP1, Weight: 100,
		WorkType: domain.WorkTypeDelivery, FocusType: domain.FocusStability,
	})
	if err != nil {
		t.Fatalf("create goal: %v", err)
	}
	krID, err = repo.KRs.CreateKeyResult(ctx, domain.TenantScope{TenantID: 1}, krs.KeyResultInput{
		GoalID: goalID, Title: "KR", Weight: 100, Kind: domain.KRKindNumerical,
	})
	if err != nil {
		t.Fatalf("create kr: %v", err)
	}
	if err := repo.KRs.UpsertNumericalMeta(ctx, domain.TenantScope{TenantID: 1}, krs.NumericalMetaInput{
		KeyResultID: krID, StartValue: 0, TargetValue: 100, CurrentValue: 0, Unit: "%",
	}); err != nil {
		t.Fatalf("upsert numerical meta: %v", err)
	}
	return
}

func multipartBody(fields map[string]string) (io.Reader, string) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	w.Close()
	return &buf, w.FormDataContentType()
}

func TestKRProgressAccessControl(t *testing.T) {
	pool, repo, teardown := setupKRAccessDB(t)
	defer teardown()
	teamID, _, _, krID := buildKRAccessFixture(t, pool, repo)
	gc := grants.NewGrantsCache(repo.Grants)
	payload, _ := json.Marshal(map[string]float64{"current_value": 50})

	t.Run("denied with empty scope", func(t *testing.T) {
		server := httptest.NewServer(testutil.NewAPIV1RouterWithScope(repo, gc, []int64{}))
		defer server.Close()
		resp, err := http.Post(fmt.Sprintf("%s/api/v1/krs/%d/progress/numerical", server.URL, krID),
			"application/json", bytes.NewBuffer(payload))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("denied when team not in scope", func(t *testing.T) {
		server := httptest.NewServer(testutil.NewAPIV1RouterWithScope(repo, gc, []int64{teamID + 999}))
		defer server.Close()
		resp, err := http.Post(fmt.Sprintf("%s/api/v1/krs/%d/progress/numerical", server.URL, krID),
			"application/json", bytes.NewBuffer(payload))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("allowed when team in scope", func(t *testing.T) {
		server := httptest.NewServer(testutil.NewAPIV1RouterWithScope(repo, gc, []int64{teamID}))
		defer server.Close()
		resp, err := http.Post(fmt.Sprintf("%s/api/v1/krs/%d/progress/numerical", server.URL, krID),
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
		server := httptest.NewServer(testutil.NewAPIV1RouterWithScope(repo, gc, nil))
		defer server.Close()
		resp, err := http.Post(fmt.Sprintf("%s/api/v1/krs/%d/progress/numerical", server.URL, krID),
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

func TestKRNoteAccessControl(t *testing.T) {
	pool, repo, teardown := setupKRAccessDB(t)
	defer teardown()
	teamID, _, _, krID := buildKRAccessFixture(t, pool, repo)
	gc := grants.NewGrantsCache(repo.Grants)
	payload, _ := json.Marshal(map[string]string{"text": "test note"})

	t.Run("denied with empty scope", func(t *testing.T) {
		server := httptest.NewServer(testutil.NewAPIV1RouterWithScope(repo, gc, []int64{}))
		defer server.Close()
		resp, err := http.Post(fmt.Sprintf("%s/api/v1/krs/%d/note", server.URL, krID),
			"application/json", bytes.NewBuffer(payload))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("denied when team not in scope", func(t *testing.T) {
		server := httptest.NewServer(testutil.NewAPIV1RouterWithScope(repo, gc, []int64{teamID + 999}))
		defer server.Close()
		resp, err := http.Post(fmt.Sprintf("%s/api/v1/krs/%d/note", server.URL, krID),
			"application/json", bytes.NewBuffer(payload))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("allowed when team in scope", func(t *testing.T) {
		server := httptest.NewServer(testutil.NewAPIV1RouterWithScope(repo, gc, []int64{teamID}))
		defer server.Close()
		resp, err := http.Post(fmt.Sprintf("%s/api/v1/krs/%d/note", server.URL, krID),
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
		server := httptest.NewServer(testutil.NewAPIV1RouterWithScope(repo, gc, nil))
		defer server.Close()
		resp, err := http.Post(fmt.Sprintf("%s/api/v1/krs/%d/note", server.URL, krID),
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

func TestCreateKRAccessControl(t *testing.T) {
	pool, repo, teardown := setupKRAccessDB(t)
	defer teardown()
	teamID, _, goalID, _ := buildKRAccessFixture(t, pool, repo)
	gc := grants.NewGrantsCache(repo.Grants)

	body, ct := multipartBody(map[string]string{
		"title": "New KR", "kind": "NUMERICAL", "weight": "50",
		"numerical_unit": "%", "numerical_start": "0", "numerical_target": "100",
	})

	t.Run("denied with empty scope", func(t *testing.T) {
		server := httptest.NewServer(testutil.NewAPIV1RouterWithScope(repo, gc, []int64{}))
		defer server.Close()
		body, ct := multipartBody(map[string]string{
			"title": "New KR", "kind": "NUMERICAL", "weight": "50",
			"numerical_unit": "%", "numerical_start": "0", "numerical_target": "100",
		})
		resp, err := http.Post(fmt.Sprintf("%s/api/v1/goals/%d/key-results", server.URL, goalID), ct, body)
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("allowed when team in scope", func(t *testing.T) {
		server := httptest.NewServer(testutil.NewAPIV1RouterWithScope(repo, gc, []int64{teamID}))
		defer server.Close()
		resp, err := http.Post(fmt.Sprintf("%s/api/v1/goals/%d/key-results", server.URL, goalID), ct, body)
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
	})
}
