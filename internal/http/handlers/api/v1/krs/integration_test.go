package krs_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/api/v1/testutil"
	"okrs/internal/service"
	"okrs/internal/store"
	"okrs/internal/store/goals"
	"okrs/internal/store/grants"
	"okrs/internal/store/krs"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestUpdateKRProgressIntegration(t *testing.T) {
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
	defer func() { _ = container.Terminate(ctx) }()

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
	defer pool.Close()

	repo := store.New(pool)
	var teamID int64
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('API') RETURNING id`).Scan(&teamID); err != nil {
		t.Fatalf("insert team: %v", err)
	}
	var periodID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO periods (name, start_date, end_date)
		VALUES ('2024 Q3', '2024-07-01', '2024-09-30')
		RETURNING id`).Scan(&periodID); err != nil {
		t.Fatalf("insert period: %v", err)
	}

	goalID, err := repo.Goals.CreateGoal(ctx, domain.TenantScope{TenantID: 1}, goals.GoalInput{
		TeamID:      teamID,
		PeriodID:    periodID,
		Title:       "API Goal",
		Description: "desc",
		Priority:    domain.PriorityP1,
		Weight:      100,
		WorkType:    domain.WorkTypeDelivery,
		FocusType:   domain.FocusStability,
		OwnerText:   "Owner",
	})
	if err != nil {
		t.Fatalf("create goal: %v", err)
	}

	krID, err := repo.KRs.CreateKeyResult(ctx, domain.TenantScope{TenantID: 1}, krs.KeyResultInput{
		GoalID:      goalID,
		Title:       "KR",
		Description: "",
		Weight:      100,
		Kind:        domain.KRKindNumerical,
	})
	if err != nil {
		t.Fatalf("create kr: %v", err)
	}
	if err := repo.KRs.UpsertNumericalMeta(ctx, domain.TenantScope{TenantID: 1}, krs.NumericalMetaInput{KeyResultID: krID, StartValue: 0, TargetValue: 100, CurrentValue: 0, Unit: "%"}); err != nil {
		t.Fatalf("meta: %v", err)
	}

	svc := service.NewFromStore(repo, grants.NewGrantsCache(repo.Grants), nil, nil)
	server := httptest.NewServer(testutil.NewAPIV1Router(svc))
	defer server.Close()

	payload, _ := json.Marshal(map[string]float64{"current_value": 50})
	resp, err := http.Post(fmt.Sprintf("%s/api/v1/krs/%d/progress/numerical", server.URL, krID), "application/json", bytes.NewBuffer(payload))
	if err != nil {
		t.Fatalf("post progress: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	getResp, err := http.Get(fmt.Sprintf("%s/api/v1/teams/%d/okrs?period_id=%d", server.URL, teamID, periodID))
	if err != nil {
		t.Fatalf("get okrs: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", getResp.StatusCode)
	}

	var okrResponse struct {
		Goals []struct {
			Progress   int `json:"progress"`
			KeyResults []struct {
				Progress int `json:"progress"`
			} `json:"key_results"`
		} `json:"goals"`
	}
	if err := json.NewDecoder(getResp.Body).Decode(&okrResponse); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(okrResponse.Goals) != 1 || len(okrResponse.Goals[0].KeyResults) != 1 {
		t.Fatalf("expected goal and kr")
	}
	if okrResponse.Goals[0].KeyResults[0].Progress != 50 {
		t.Fatalf("expected kr progress 50, got %d", okrResponse.Goals[0].KeyResults[0].Progress)
	}
	if okrResponse.Goals[0].Progress != 50 {
		t.Fatalf("expected goal progress 50, got %d", okrResponse.Goals[0].Progress)
	}
}

func TestUpsertKRNoteIntegration(t *testing.T) {
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
	defer func() { _ = container.Terminate(ctx) }()

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
	defer pool.Close()

	repo := store.New(pool)
	var teamID int64
	pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('NoteIntegTeam') RETURNING id`).Scan(&teamID)
	var periodID int64
	pool.QueryRow(ctx, `INSERT INTO periods (name, start_date, end_date) VALUES ('Q2', '2024-04-01', '2024-06-30') RETURNING id`).Scan(&periodID)

	goalID, err := repo.Goals.CreateGoal(ctx, domain.TenantScope{TenantID: 1}, goals.GoalInput{
		TeamID: teamID, PeriodID: periodID, Title: "Note Goal",
		Priority: domain.PriorityP1, Weight: 100,
		WorkType: domain.WorkTypeDelivery, FocusType: domain.FocusStability,
	})
	if err != nil {
		t.Fatalf("create goal: %v", err)
	}
	krID, err := repo.KRs.CreateKeyResult(ctx, domain.TenantScope{TenantID: 1}, krs.KeyResultInput{
		GoalID: goalID, Title: "Note KR", Weight: 100, Kind: domain.KRKindBoolean,
	})
	if err != nil {
		t.Fatalf("create kr: %v", err)
	}

	svc := service.NewFromStore(repo, grants.NewGrantsCache(repo.Grants), nil, nil)
	server := httptest.NewServer(testutil.NewAPIV1RouterWithScope(svc, []int64{teamID}))
	defer server.Close()

	t.Run("empty text returns 400", func(t *testing.T) {
		payload, _ := json.Marshal(map[string]string{"text": ""})
		resp, err := http.Post(
			fmt.Sprintf("%s/api/v1/krs/%d/note", server.URL, krID),
			"application/json", bytes.NewReader(payload),
		)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("valid note returns 200", func(t *testing.T) {
		payload, _ := json.Marshal(map[string]string{"text": "integration note"})
		resp, err := http.Post(
			fmt.Sprintf("%s/api/v1/krs/%d/note", server.URL, krID),
			"application/json", bytes.NewReader(payload),
		)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("note appears in OKR response", func(t *testing.T) {
		resp, err := http.Get(fmt.Sprintf("%s/api/v1/teams/%d/okrs?period_id=%d", server.URL, teamID, periodID))
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 for okrs, got %d", resp.StatusCode)
		}
		var body struct {
			Goals []struct {
				KeyResults []struct {
					ID   int64 `json:"id"`
					Note *struct {
						Text string `json:"text"`
					} `json:"note"`
				} `json:"key_results"`
			} `json:"goals"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		for _, g := range body.Goals {
			for _, kr := range g.KeyResults {
				if kr.ID == krID {
					if kr.Note == nil {
						t.Fatal("note is nil in OKR response")
					}
					if kr.Note.Text != "integration note" {
						t.Fatalf("expected 'integration note', got %q", kr.Note.Text)
					}
					return
				}
			}
		}
		t.Fatalf("KR %d not found in response", krID)
	})
}

func TestUpdateKRHealthStatusIntegration(t *testing.T) {
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
	defer func() { _ = container.Terminate(ctx) }()

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
	defer pool.Close()

	repo := store.New(pool)
	var teamID int64
	pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('HealthTeam') RETURNING id`).Scan(&teamID)
	var periodID int64
	pool.QueryRow(ctx, `INSERT INTO periods (name, start_date, end_date) VALUES ('Q4', '2024-10-01', '2024-12-31') RETURNING id`).Scan(&periodID)

	goalID, err := repo.Goals.CreateGoal(ctx, domain.TenantScope{TenantID: 1}, goals.GoalInput{
		TeamID: teamID, PeriodID: periodID, Title: "Health Goal",
		Priority: domain.PriorityP1, Weight: 100,
		WorkType: domain.WorkTypeDelivery, FocusType: domain.FocusStability,
	})
	if err != nil {
		t.Fatalf("create goal: %v", err)
	}
	krID, err := repo.KRs.CreateKeyResult(ctx, domain.TenantScope{TenantID: 1}, krs.KeyResultInput{
		GoalID: goalID, Title: "Health KR", Weight: 100, Kind: domain.KRKindNumerical,
	})
	if err != nil {
		t.Fatalf("create kr: %v", err)
	}
	if err := repo.KRs.UpsertNumericalMeta(ctx, domain.TenantScope{TenantID: 1}, krs.NumericalMetaInput{KeyResultID: krID, StartValue: 0, TargetValue: 100, CurrentValue: 0, Unit: "%"}); err != nil {
		t.Fatalf("meta: %v", err)
	}

	svc := service.NewFromStore(repo, grants.NewGrantsCache(repo.Grants), nil, nil)
	server := httptest.NewServer(testutil.NewAPIV1Router(svc))
	defer server.Close()

	postProgress := func(t *testing.T, body map[string]any) int {
		t.Helper()
		payload, _ := json.Marshal(body)
		resp, err := http.Post(fmt.Sprintf("%s/api/v1/krs/%d/progress/numerical", server.URL, krID), "application/json", bytes.NewBuffer(payload))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}
	getHealth := func(t *testing.T) string {
		t.Helper()
		resp, err := http.Get(fmt.Sprintf("%s/api/v1/teams/%d/okrs?period_id=%d", server.URL, teamID, periodID))
		if err != nil {
			t.Fatalf("get okrs: %v", err)
		}
		defer resp.Body.Close()
		var body struct {
			Goals []struct {
				KeyResults []struct {
					ID           int64  `json:"id"`
					HealthStatus string `json:"health_status"`
				} `json:"key_results"`
			} `json:"goals"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		for _, g := range body.Goals {
			for _, kr := range g.KeyResults {
				if kr.ID == krID {
					return kr.HealthStatus
				}
			}
		}
		t.Fatalf("KR %d not found", krID)
		return ""
	}

	t.Run("valid health_status applied with progress", func(t *testing.T) {
		if code := postProgress(t, map[string]any{"current_value": 42, "health_status": "at_risk"}); code != http.StatusOK {
			t.Fatalf("expected 200, got %d", code)
		}
		if got := getHealth(t); got != "at_risk" {
			t.Fatalf("expected at_risk, got %q", got)
		}
	})

	t.Run("invalid health_status returns 400 and leaves it unchanged", func(t *testing.T) {
		if code := postProgress(t, map[string]any{"current_value": 42, "health_status": "bogus"}); code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", code)
		}
		if got := getHealth(t); got != "at_risk" {
			t.Fatalf("expected at_risk unchanged, got %q", got)
		}
	})

	t.Run("omitted health_status: server auto-sets done at 100%", func(t *testing.T) {
		if code := postProgress(t, map[string]any{"current_value": 100}); code != http.StatusOK {
			t.Fatalf("expected 200, got %d", code)
		}
		if got := getHealth(t); got != "done" {
			t.Fatalf("expected auto-done, got %q", got)
		}
	})
}
