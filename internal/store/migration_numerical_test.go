package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestMigration023ConvertsLegacyKindsToNumerical verifies that LINEAR and PERCENT
// key results migrate into NUMERICAL with unit '%', values preserved, checkpoints
// moved into the key_results.checkpoints JSONB column, and legacy tables dropped.
func TestMigration023ConvertsLegacyKindsToNumerical(t *testing.T) {
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

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	driver, err := migratepostgres.WithInstance(db, &migratepostgres.Config{})
	if err != nil {
		t.Fatalf("driver: %v", err)
	}
	migrationsPath, err := resolveMigrationsPath()
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://"+migrationsPath, "postgres", driver)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Migrate to the pre-numerical schema (legacy meta tables still present).
	if err := m.Migrate(22); err != nil {
		t.Fatalf("migrate to 22: %v", err)
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()

	var teamID, periodID, goalID int64
	mustScan(t, pool, `INSERT INTO teams (name) VALUES ('T') RETURNING id`, &teamID)
	mustScan(t, pool, `INSERT INTO periods (name, start_date, end_date, sort_order) VALUES ('Q1','2024-01-01','2024-03-31',1) RETURNING id`, &periodID)
	mustScanArgs(t, pool, `INSERT INTO goals (team_id, period_id, title, priority, weight, work_type, focus_type, sort_order) VALUES ($1,$2,'G','P1',100,'Delivery','STABILITY',1) RETURNING id`, &goalID, teamID, periodID)

	var linearKR, percentKR, boolKR int64
	mustScanArgs(t, pool, `INSERT INTO key_results (goal_id, title, weight, kind, sort_order) VALUES ($1,'lin',40,'LINEAR',1) RETURNING id`, &linearKR, goalID)
	mustScanArgs(t, pool, `INSERT INTO key_results (goal_id, title, weight, kind, sort_order) VALUES ($1,'pct',40,'PERCENT',2) RETURNING id`, &percentKR, goalID)
	mustScanArgs(t, pool, `INSERT INTO key_results (goal_id, title, weight, kind, sort_order) VALUES ($1,'bool',20,'BOOLEAN',3) RETURNING id`, &boolKR, goalID)

	exec(t, pool, `INSERT INTO kr_linear_meta (key_result_id, start_value, target_value, current_value) VALUES ($1,10,5,7)`, linearKR)
	exec(t, pool, `INSERT INTO kr_percent_meta (key_result_id, start_value, target_value, current_value) VALUES ($1,100,180,150)`, percentKR)
	exec(t, pool, `INSERT INTO kr_percent_checkpoints (key_result_id, metric_value, kr_percent) VALUES ($1,150,50),($1,180,100)`, percentKR)
	exec(t, pool, `INSERT INTO kr_boolean_meta (key_result_id, is_done) VALUES ($1,true)`, boolKR)

	// Apply the conversion migration.
	if err := m.Migrate(23); err != nil {
		t.Fatalf("migrate to 23: %v", err)
	}

	// Legacy LINEAR/PERCENT KRs are now NUMERICAL with unit '%' and values preserved.
	assertKR(t, pool, linearKR, "NUMERICAL", "%", 10, 5, 7)
	assertKR(t, pool, percentKR, "NUMERICAL", "%", 100, 180, 150)

	// BOOLEAN is unchanged and its weight preserved.
	var boolKind string
	var boolWeight int
	if err := pool.QueryRow(ctx, `SELECT kind, weight FROM key_results WHERE id=$1`, boolKR).Scan(&boolKind, &boolWeight); err != nil {
		t.Fatalf("scan bool: %v", err)
	}
	if boolKind != "BOOLEAN" || boolWeight != 20 {
		t.Fatalf("expected BOOLEAN weight 20, got %s/%d", boolKind, boolWeight)
	}

	// Checkpoints moved into the JSONB column for the percent KR.
	var cpCount int
	if err := pool.QueryRow(ctx, `SELECT jsonb_array_length(checkpoints) FROM key_results WHERE id=$1`, percentKR).Scan(&cpCount); err != nil {
		t.Fatalf("scan checkpoints: %v", err)
	}
	if cpCount != 2 {
		t.Fatalf("expected 2 checkpoints in JSONB, got %d", cpCount)
	}

	// Legacy tables are gone.
	for _, table := range []string{"kr_percent_meta", "kr_linear_meta", "kr_percent_checkpoints"} {
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name=$1)`, table).Scan(&exists); err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if exists {
			t.Fatalf("expected legacy table %s to be dropped", table)
		}
	}
}

func assertKR(t *testing.T, pool *pgxpool.Pool, krID int64, wantKind, wantUnit string, wantStart, wantTarget, wantCurrent float64) {
	t.Helper()
	var kind, unit string
	var start, target, current float64
	if err := pool.QueryRow(context.Background(),
		`SELECT kind, unit, start_value, target_value, current_value FROM key_results WHERE id=$1`, krID).
		Scan(&kind, &unit, &start, &target, &current); err != nil {
		t.Fatalf("scan kr %d: %v", krID, err)
	}
	if kind != wantKind || unit != wantUnit || start != wantStart || target != wantTarget || current != wantCurrent {
		t.Fatalf("kr %d: got kind=%s unit=%s start=%v target=%v current=%v; want kind=%s unit=%s start=%v target=%v current=%v",
			krID, kind, unit, start, target, current, wantKind, wantUnit, wantStart, wantTarget, wantCurrent)
	}
}

func mustScan(t *testing.T, pool *pgxpool.Pool, query string, dest *int64) {
	t.Helper()
	if err := pool.QueryRow(context.Background(), query).Scan(dest); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
}

func mustScanArgs(t *testing.T, pool *pgxpool.Pool, query string, dest *int64, args ...any) {
	t.Helper()
	if err := pool.QueryRow(context.Background(), query, args...).Scan(dest); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
}

func exec(t *testing.T, pool *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}
