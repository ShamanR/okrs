package v1

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestUpdateKRProgressIntegration(t *testing.T) {
	ctx := context.Background()
	container, err := tcpostgres.RunContainer(ctx,
		tcpostgres.WithDatabase("okrs"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
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

	goalID, err := repo.CreateGoal(ctx, store.GoalInput{
		TeamID:      teamID,
		Year:        2024,
		Quarter:     3,
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

	getResp, err := http.Get(fmt.Sprintf("%s/api/v1/teams/%d/okrs?quarter=2024-3", server.URL, teamID))
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
	m, err := migrate.NewWithDatabaseInstance("file://../../migrations", "postgres", driver)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}
