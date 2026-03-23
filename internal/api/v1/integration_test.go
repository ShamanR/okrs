package v1

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"okrs/internal/domain"
	"okrs/internal/service"
	"okrs/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/golang-migrate/migrate/v4"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
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
	if err != nil {
		t.Skipf("docker unavailable: %v", err)
	}
	defer func() { _ = container.Terminate(ctx) }()

	dbURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("conn string: %v", err)
	}
	if err := runMigrations(dbURL); err != nil {
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

	goalID, err := repo.CreateGoal(ctx, store.GoalInput{
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

	krID, err := repo.CreateKeyResult(ctx, store.KeyResultInput{
		GoalID:      goalID,
		Title:       "KR",
		Description: "",
		Weight:      100,
		Kind:        domain.KRKindPercent,
	})
	if err != nil {
		t.Fatalf("create kr: %v", err)
	}
	if err := repo.UpsertPercentMeta(ctx, store.PercentMetaInput{KeyResultID: krID, StartValue: 0, TargetValue: 100, CurrentValue: 0}); err != nil {
		t.Fatalf("meta: %v", err)
	}

	svc := service.New(repo)
	handler := NewHandler(svc)
	router := chi.NewRouter()
	router.Mount("/api/v1", handler.Routes())

	server := httptest.NewServer(router)
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

	var okrResponse teamOKRResponse
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
	if err != nil {
		t.Skipf("docker unavailable: %v", err)
	}
	defer func() { _ = container.Terminate(ctx) }()

	dbURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("conn string: %v", err)
	}
	if err := runMigrations(dbURL); err != nil {
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

	goalID, err := repo.CreateGoal(ctx, store.GoalInput{
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

	krID, err := repo.CreateKeyResult(ctx, store.KeyResultInput{
		GoalID:      goalID,
		Title:       "KR",
		Description: "",
		Weight:      100,
		Kind:        domain.KRKindBoolean,
	})
	if err != nil {
		t.Fatalf("create kr: %v", err)
	}

	svc := service.New(repo)
	handler := NewHandler(svc)
	router := chi.NewRouter()
	router.Mount("/api/v1", handler.Routes())

	server := httptest.NewServer(router)
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

	comments, err := repo.LastKeyResultComments(ctx, krID)
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

func TestDeletedTeamsVisibilityDependsOnPeriodIntegration(t *testing.T) {
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
	if err != nil {
		t.Skipf("docker unavailable: %v", err)
	}
	defer func() { _ = container.Terminate(ctx) }()

	dbURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("conn string: %v", err)
	}
	if err := runMigrations(dbURL); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()

	repo := store.New(pool)
	var deletedTeamID, activeTeamID, currentPeriodID, historyPeriodID int64
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name, team_type) VALUES ('Deleted team', 'team') RETURNING id`).Scan(&deletedTeamID); err != nil {
		t.Fatalf("insert deleted team: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name, team_type) VALUES ('Active team', 'team') RETURNING id`).Scan(&activeTeamID); err != nil {
		t.Fatalf("insert active team: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO periods (name, start_date, end_date, sort_order)
		VALUES ('History', '2025-01-01', '2025-03-31', 1)
		RETURNING id`).Scan(&historyPeriodID); err != nil {
		t.Fatalf("insert history period: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO periods (name, start_date, end_date, sort_order)
		VALUES ('Current', CURRENT_DATE - INTERVAL '5 day', CURRENT_DATE + INTERVAL '5 day', 2)
		RETURNING id`).Scan(&currentPeriodID); err != nil {
		t.Fatalf("insert current period: %v", err)
	}

	if _, err := repo.CreateGoal(ctx, store.GoalInput{
		TeamID:      deletedTeamID,
		PeriodID:    historyPeriodID,
		Title:       "History goal",
		Description: "desc",
		Priority:    domain.PriorityP1,
		Weight:      100,
		WorkType:    domain.WorkTypeDelivery,
		FocusType:   domain.FocusStability,
		OwnerText:   "Owner",
	}); err != nil {
		t.Fatalf("create historical goal: %v", err)
	}
	if err := repo.SoftDeleteTeam(ctx, deletedTeamID); err != nil {
		t.Fatalf("soft delete team: %v", err)
	}

	svc := service.New(repo)
	handler := NewHandler(svc)
	router := chi.NewRouter()
	router.Mount("/api/v1", handler.Routes())

	server := httptest.NewServer(router)
	defer server.Close()

	currentResp, err := http.Get(fmt.Sprintf("%s/api/v1/teams?period_id=%d", server.URL, currentPeriodID))
	if err != nil {
		t.Fatalf("get current teams: %v", err)
	}
	defer currentResp.Body.Close()
	if currentResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for current teams, got %d", currentResp.StatusCode)
	}
	var currentTeams teamsResponse
	if err := json.NewDecoder(currentResp.Body).Decode(&currentTeams); err != nil {
		t.Fatalf("decode current teams: %v", err)
	}
	if len(currentTeams.Items) != 1 || currentTeams.Items[0].ID != activeTeamID {
		t.Fatalf("expected only active team in current period, got %+v", currentTeams.Items)
	}

	historyResp, err := http.Get(fmt.Sprintf("%s/api/v1/teams?period_id=%d", server.URL, historyPeriodID))
	if err != nil {
		t.Fatalf("get history teams: %v", err)
	}
	defer historyResp.Body.Close()
	if historyResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for history teams, got %d", historyResp.StatusCode)
	}
	var historyTeams teamsResponse
	if err := json.NewDecoder(historyResp.Body).Decode(&historyTeams); err != nil {
		t.Fatalf("decode history teams: %v", err)
	}
	if len(historyTeams.Items) != 2 {
		t.Fatalf("expected active team without goals plus deleted historical team, got %+v", historyTeams.Items)
	}
	if historyTeams.Items[0].ID != activeTeamID || historyTeams.Items[1].ID != deletedTeamID {
		t.Fatalf("unexpected history teams order/content: %+v", historyTeams.Items)
	}

	okrResp, err := http.Get(fmt.Sprintf("%s/api/v1/teams/%d/okrs?period_id=%d", server.URL, deletedTeamID, historyPeriodID))
	if err != nil {
		t.Fatalf("get historical okr: %v", err)
	}
	defer okrResp.Body.Close()
	if okrResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for historical deleted team, got %d", okrResp.StatusCode)
	}

	activeHistoryResp, err := http.Get(fmt.Sprintf("%s/api/v1/teams/%d/okrs?period_id=%d", server.URL, activeTeamID, historyPeriodID))
	if err != nil {
		t.Fatalf("get active team history okr: %v", err)
	}
	defer activeHistoryResp.Body.Close()
	if activeHistoryResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for active team without historical goals, got %d", activeHistoryResp.StatusCode)
	}

	currentDeletedResp, err := http.Get(fmt.Sprintf("%s/api/v1/teams/%d/okrs?period_id=%d", server.URL, deletedTeamID, currentPeriodID))
	if err != nil {
		t.Fatalf("get current deleted team okr: %v", err)
	}
	defer currentDeletedResp.Body.Close()
	if currentDeletedResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for deleted team in current period, got %d", currentDeletedResp.StatusCode)
	}

	if _, err := repo.CreateGoal(ctx, store.GoalInput{
		TeamID:      deletedTeamID,
		PeriodID:    currentPeriodID,
		Title:       "Current goal",
		Description: "desc",
		Priority:    domain.PriorityP1,
		Weight:      100,
		WorkType:    domain.WorkTypeDelivery,
		FocusType:   domain.FocusStability,
		OwnerText:   "Owner",
	}); err != nil {
		t.Fatalf("create current goal: %v", err)
	}

	currentAgainResp, err := http.Get(fmt.Sprintf("%s/api/v1/teams?period_id=%d", server.URL, currentPeriodID))
	if err != nil {
		t.Fatalf("get current teams after goal: %v", err)
	}
	defer currentAgainResp.Body.Close()
	if currentAgainResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for current teams after goal, got %d", currentAgainResp.StatusCode)
	}
	var currentTeamsWithGoal teamsResponse
	if err := json.NewDecoder(currentAgainResp.Body).Decode(&currentTeamsWithGoal); err != nil {
		t.Fatalf("decode current teams after goal: %v", err)
	}
	if len(currentTeamsWithGoal.Items) != 2 {
		t.Fatalf("expected deleted team with current goal to become visible, got %+v", currentTeamsWithGoal.Items)
	}

	currentDeletedVisibleResp, err := http.Get(fmt.Sprintf("%s/api/v1/teams/%d/okrs?period_id=%d", server.URL, deletedTeamID, currentPeriodID))
	if err != nil {
		t.Fatalf("get current deleted team okr after goal: %v", err)
	}
	defer currentDeletedVisibleResp.Body.Close()
	if currentDeletedVisibleResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for deleted team with current goal, got %d", currentDeletedVisibleResp.StatusCode)
	}
}

func runMigrations(databaseURL string) error {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return err
	}
	driver, err := migratepostgres.WithInstance(db, &migratepostgres.Config{})
	if err != nil {
		return err
	}
	migrationsPath, err := resolveMigrationsPath()
	if err != nil {
		return err
	}
	m, err := migrate.NewWithDatabaseInstance("file://"+migrationsPath, "postgres", driver)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

func resolveMigrationsPath() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "migrations"), nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found (start dir: %s)", dir)
		}
		dir = parent
	}
}
