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

	"okrs/internal/domain"
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
		INSERT INTO periods (name, start_date, end_date, sort_order)
		VALUES ('2024 Q3', '2024-07-01', '2024-09-30', 1)
		RETURNING id`).Scan(&periodID); err != nil {
		t.Fatalf("insert period: %v", err)
	}

	goalID, err := repo.Goals.CreateGoal(ctx, goals.GoalInput{
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

	krID, err := repo.KRs.CreateKeyResult(ctx, krs.KeyResultInput{
		GoalID:      goalID,
		Title:       "KR",
		Description: "",
		Weight:      100,
		Kind:        domain.KRKindPercent,
	})
	if err != nil {
		t.Fatalf("create kr: %v", err)
	}
	if err := repo.KRs.UpsertPercentMeta(ctx, krs.PercentMetaInput{KeyResultID: krID, StartValue: 0, TargetValue: 100, CurrentValue: 0}); err != nil {
		t.Fatalf("meta: %v", err)
	}

	svc := service.NewFromStore(repo, grants.NewGrantsCache(repo.Grants))
	server := httptest.NewServer(testutil.NewAPIV1Router(svc))
	defer server.Close()

	payload, _ := json.Marshal(map[string]float64{"current_value": 50})
	resp, err := http.Post(fmt.Sprintf("%s/api/v1/krs/%d/progress/percent", server.URL, krID), "application/json", bytes.NewBuffer(payload))
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

func TestAddKRCommentPreservesMultilineIntegration(t *testing.T) {
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
		INSERT INTO periods (name, start_date, end_date, sort_order)
		VALUES ('2024 Q3', '2024-07-01', '2024-09-30', 1)
		RETURNING id`).Scan(&periodID); err != nil {
		t.Fatalf("insert period: %v", err)
	}

	goalID, err := repo.Goals.CreateGoal(ctx, goals.GoalInput{
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

	krID, err := repo.KRs.CreateKeyResult(ctx, krs.KeyResultInput{
		GoalID:      goalID,
		Title:       "KR",
		Description: "",
		Weight:      100,
		Kind:        domain.KRKindBoolean,
	})
	if err != nil {
		t.Fatalf("create kr: %v", err)
	}

	svc := service.NewFromStore(repo, grants.NewGrantsCache(repo.Grants))
	server := httptest.NewServer(testutil.NewAPIV1Router(svc))
	defer server.Close()

	commentText := "Первая строка\r\nВторая строка\r\nТретья строка"
	payload, _ := json.Marshal(map[string]string{"text": commentText})
	resp, err := http.Post(fmt.Sprintf("%s/api/v1/krs/%d/comments", server.URL, krID), "application/json", bytes.NewBuffer(payload))
	if err != nil {
		t.Fatalf("post comment: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	comments, err := repo.KRs.LastKeyResultComments(ctx, krID)
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("expected one comment, got %d", len(comments))
	}
	want := "Первая строка\nВторая строка\nТретья строка"
	if comments[0].Text != want {
		t.Fatalf("expected %q, got %q", want, comments[0].Text)
	}
}
