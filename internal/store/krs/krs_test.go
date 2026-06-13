package krs_test

import (
	"context"
	"testing"
	"time"

	"okrs/internal/domain"
	"okrs/internal/http/handlers/api/v1/testutil"
	"okrs/internal/store/krs"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupKRTestDB(t *testing.T) (*pgxpool.Pool, *krs.KRRepository, func()) {
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
	repo := krs.NewKRRepository(pool)
	return pool, repo, func() {
		pool.Close()
		_ = container.Terminate(ctx)
	}
}

func TestUpsertKeyResultNote(t *testing.T) {
	ctx := context.Background()
	pool, repo, cleanup := setupKRTestDB(t)
	defer cleanup()

	var teamID int64
	pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('T') RETURNING id`).Scan(&teamID)
	var periodID int64
	pool.QueryRow(ctx, `INSERT INTO periods (name, start_date, end_date, sort_order) VALUES ('Q1', '2024-01-01', '2024-03-31', 1) RETURNING id`).Scan(&periodID)
	var goalID int64
	pool.QueryRow(ctx, `INSERT INTO goals (team_id, period_id, title, priority, weight, work_type, focus_type, sort_order) VALUES ($1,$2,'G','P1',100,'Delivery','STABILITY',1) RETURNING id`, teamID, periodID).Scan(&goalID)
	var userID int64
	pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name) VALUES ('system:test','system','test','Test User') RETURNING id`).Scan(&userID)
	var krID int64
	pool.QueryRow(ctx, `INSERT INTO key_results (goal_id, title, weight, kind, sort_order) VALUES ($1,'KR1',100,'NUMERICAL',1) RETURNING id`, goalID).Scan(&krID)

	// First upsert — creates
	if err := repo.UpsertKeyResultNote(ctx, krID, "first note", userID); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	notes, err := repo.BatchLoadNotes(ctx, []int64{krID})
	if err != nil {
		t.Fatalf("batch load: %v", err)
	}
	note, ok := notes[krID]
	if !ok {
		t.Fatal("note not found after first upsert")
	}
	if note.Text != "first note" {
		t.Fatalf("expected 'first note', got %q", note.Text)
	}
	if note.AuthorName != "Test User" {
		t.Fatalf("expected 'Test User', got %q", note.AuthorName)
	}

	// Second upsert — updates
	if err := repo.UpsertKeyResultNote(ctx, krID, "updated note", userID); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	notes2, _ := repo.BatchLoadNotes(ctx, []int64{krID})
	if notes2[krID].Text != "updated note" {
		t.Fatalf("expected 'updated note', got %q", notes2[krID].Text)
	}
}

func TestUpdateKeyResultDescription(t *testing.T) {
	ctx := context.Background()
	pool, repo, cleanup := setupKRTestDB(t)
	defer cleanup()

	var teamID int64
	pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('T') RETURNING id`).Scan(&teamID)
	var periodID int64
	pool.QueryRow(ctx, `INSERT INTO periods (name, start_date, end_date, sort_order) VALUES ('Q1', '2024-01-01', '2024-03-31', 1) RETURNING id`).Scan(&periodID)
	var goalID int64
	pool.QueryRow(ctx, `INSERT INTO goals (team_id, period_id, title, priority, weight, work_type, focus_type, sort_order) VALUES ($1,$2,'G','P1',100,'Delivery','STABILITY',1) RETURNING id`, teamID, periodID).Scan(&goalID)
	var krID int64
	pool.QueryRow(ctx, `INSERT INTO key_results (goal_id, title, weight, kind, sort_order) VALUES ($1,'KR1',100,'NUMERICAL',1) RETURNING id`, goalID).Scan(&krID)

	if err := repo.UpdateKeyResultDescription(ctx, krID, "added context"); err != nil {
		t.Fatalf("update description: %v", err)
	}
	kr, err := repo.GetKeyResult(ctx, krID)
	if err != nil {
		t.Fatalf("get kr: %v", err)
	}
	if kr.Description != "added context" {
		t.Fatalf("expected description 'added context', got %q", kr.Description)
	}
}

func TestBatchLoadNotes_AbsentKR(t *testing.T) {
	ctx := context.Background()
	_, repo, cleanup := setupKRTestDB(t)
	defer cleanup()

	notes, err := repo.BatchLoadNotes(ctx, []int64{99999})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := notes[99999]; ok {
		t.Fatal("expected no entry for KR without note")
	}
}

func TestBatchLoadNotes_Empty(t *testing.T) {
	ctx := context.Background()
	_, repo, cleanup := setupKRTestDB(t)
	defer cleanup()

	notes, err := repo.BatchLoadNotes(ctx, []int64{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notes) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(notes))
	}
}

func TestUpsertAndLoadNumericalMeta(t *testing.T) {
	ctx := context.Background()
	pool, repo, cleanup := setupKRTestDB(t)
	defer cleanup()

	var teamID, periodID, goalID, krID int64
	pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('T') RETURNING id`).Scan(&teamID)
	pool.QueryRow(ctx, `INSERT INTO periods (name, start_date, end_date, sort_order) VALUES ('Q1','2024-01-01','2024-03-31',1) RETURNING id`).Scan(&periodID)
	pool.QueryRow(ctx, `INSERT INTO goals (team_id, period_id, title, priority, weight, work_type, focus_type, sort_order) VALUES ($1,$2,'G','P1',100,'Delivery','STABILITY',1) RETURNING id`, teamID, periodID).Scan(&goalID)
	pool.QueryRow(ctx, `INSERT INTO key_results (goal_id, title, weight, kind, sort_order) VALUES ($1,'KR',100,'NUMERICAL',1) RETURNING id`, goalID).Scan(&krID)

	in := krs.NumericalMetaInput{
		KeyResultID:     krID,
		StartValue:      100,
		TargetValue:     180,
		CurrentValue:    150,
		Unit:            "RPS",
		ZeroingCriteria: "сервис падает = 0%",
		Checkpoints: []domain.KRNumericalCheckpoint{
			{Value: 100, ProgressPercent: 0},
			{Value: 150, ProgressPercent: 50},
			{Value: 180, ProgressPercent: 100},
		},
	}
	if err := repo.UpsertNumericalMeta(ctx, in); err != nil {
		t.Fatalf("upsert numerical meta: %v", err)
	}

	// Round-trips together with the KR (no separate query needed by the caller).
	krsLoaded, err := repo.ListKeyResultsByGoal(ctx, goalID)
	if err != nil {
		t.Fatalf("list krs: %v", err)
	}
	if len(krsLoaded) != 1 || krsLoaded[0].Numerical == nil {
		t.Fatalf("expected one numerical KR with meta, got %+v", krsLoaded)
	}
	num := krsLoaded[0].Numerical
	if num.StartValue != 100 || num.TargetValue != 180 || num.CurrentValue != 150 {
		t.Fatalf("unexpected values: %+v", num)
	}
	if num.Unit != "RPS" || num.ZeroingCriteria != "сервис падает = 0%" {
		t.Fatalf("unexpected unit/zeroing: %+v", num)
	}
	if len(num.Checkpoints) != 3 || num.Checkpoints[1].Value != 150 || num.Checkpoints[1].ProgressPercent != 50 {
		t.Fatalf("unexpected checkpoints: %+v", num.Checkpoints)
	}

	// UpdateNumericalCurrent changes only the current value and stamps progress.
	if err := repo.UpdateNumericalCurrent(ctx, krID, 170); err != nil {
		t.Fatalf("update current: %v", err)
	}
	var current float64
	var progressUpdated *time.Time
	if err := pool.QueryRow(ctx, `SELECT current_value, progress_updated_at FROM key_results WHERE id=$1`, krID).Scan(&current, &progressUpdated); err != nil {
		t.Fatalf("scan current: %v", err)
	}
	if current != 170 {
		t.Fatalf("expected current 170, got %v", current)
	}
	if progressUpdated == nil {
		t.Fatalf("expected progress_updated_at to be set")
	}
}
