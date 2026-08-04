package teams_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestTeamExportEndpoint(t *testing.T) {
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
	scope := domain.TenantScope{TenantID: 1}

	var teamID, periodID int64
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name, team_type) VALUES ('Платформа', 'unit') RETURNING id`).Scan(&teamID); err != nil {
		t.Fatalf("insert team: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO periods (name, start_date, end_date)
		VALUES ('Current', CURRENT_DATE - INTERVAL '5 day', CURRENT_DATE + INTERVAL '5 day')
		RETURNING id`).Scan(&periodID); err != nil {
		t.Fatalf("insert period: %v", err)
	}
	if _, err := repo.Goals.CreateGoal(ctx, scope, goals.GoalInput{
		TeamID: teamID, PeriodID: periodID, Title: "Цель",
		Priority: domain.PriorityP1, Weight: 100,
		WorkType: domain.WorkTypeDelivery, FocusType: domain.FocusStability, OwnerText: "Owner",
	}); err != nil {
		t.Fatalf("create goal: %v", err)
	}

	svc := service.NewFromStore(repo, grants.NewGrantsCache(repo.Grants), nil, nil)

	// happy path: team scope, short
	inScope := httptest.NewServer(testutil.NewAPIV1RouterWithScope(svc, []int64{teamID}))
	defer inScope.Close()

	resp, err := http.Get(fmt.Sprintf("%s/api/v1/teams/%d/export?period_id=%d&scope=team", inScope.URL, teamID, periodID))
	if err != nil {
		t.Fatalf("get export: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body struct {
		Filename string `json:"filename"`
		Markdown string `json:"markdown"`
		Lines    int    `json:"lines"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Filename == "" || !strings.Contains(body.Markdown, "# Платформа") || body.Lines == 0 {
		t.Fatalf("unexpected body: %+v", body)
	}

	// invalid scope -> 400
	resp2, err := http.Get(fmt.Sprintf("%s/api/v1/teams/%d/export?period_id=%d&scope=bogus", inScope.URL, teamID, periodID))
	if err != nil {
		t.Fatalf("get bad scope: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad scope, got %d", resp2.StatusCode)
	}

	// team out of scope -> 404
	outScope := httptest.NewServer(testutil.NewAPIV1RouterWithScope(svc, []int64{99999}))
	defer outScope.Close()
	resp3, err := http.Get(fmt.Sprintf("%s/api/v1/teams/%d/export?period_id=%d&scope=team", outScope.URL, teamID, periodID))
	if err != nil {
		t.Fatalf("get out of scope: %v", err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 out of scope, got %d", resp3.StatusCode)
	}
}
