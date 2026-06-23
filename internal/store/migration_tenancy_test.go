package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// migrateTo brings a fresh container up and migrates to the given version.
// It returns an open *sql.DB and a cleanup func.
func migrateTo(t *testing.T, version uint) (*sql.DB, func()) {
	t.Helper()
	ctx := context.Background()
	container, err := tcpostgres.RunContainer(ctx,
		tcpostgres.WithDatabase("okrs"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(10*time.Second),
		),
	)
	if err != nil {
		t.Skipf("docker unavailable: %v", err)
	}
	dbURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("conn string: %v", err)
	}
	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	driver, err := migratepostgres.WithInstance(db, &migratepostgres.Config{})
	if err != nil {
		t.Fatalf("driver: %v", err)
	}
	path, err := resolveMigrationsPath()
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://"+path, "postgres", driver)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := m.Migrate(version); err != nil {
		t.Fatalf("migrate to %d: %v", version, err)
	}
	cleanup := func() {
		_ = db.Close()
		_ = container.Terminate(ctx)
	}
	return db, cleanup
}

// migrateDBTo applies migrations up to version on an already-open db (same container).
func migrateDBTo(t *testing.T, db *sql.DB, version uint) {
	t.Helper()
	driver, err := migratepostgres.WithInstance(db, &migratepostgres.Config{})
	if err != nil {
		t.Fatalf("driver: %v", err)
	}
	path, err := resolveMigrationsPath()
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://"+path, "postgres", driver)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := m.Migrate(version); err != nil {
		t.Fatalf("migrate to %d: %v", version, err)
	}
}

func TestMigration027CreatesDefaultTenant(t *testing.T) {
	db, cleanup := migrateTo(t, 27)
	defer cleanup()

	var id int64
	var slug, name, status string
	err := db.QueryRow(`SELECT id, slug, name, status FROM tenants WHERE id = 1`).
		Scan(&id, &slug, &name, &status)
	if err != nil {
		t.Fatalf("query default tenant: %v", err)
	}
	if slug != "default" {
		t.Fatalf("slug = %q, want default", slug)
	}
	if status != "active" {
		t.Fatalf("status = %q, want active", status)
	}
}

func TestMigration033MovesProductKeys(t *testing.T) {
	db, cleanup := migrateTo(t, 32) // up to before the move
	defer cleanup()

	if _, err := db.Exec(`INSERT INTO system_settings (key, value_json) VALUES ('documentation_url', '"https://doc"'::jsonb)`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	migrateDBTo(t, db, 33) // apply the move

	var v string
	if err := db.QueryRow(`SELECT value_json #>> '{}' FROM tenant_settings WHERE tenant_id = 1 AND key = 'documentation_url'`).Scan(&v); err != nil {
		t.Fatalf("expected key moved to tenant_settings: %v", err)
	}
	if v != "https://doc" {
		t.Fatalf("got %q, want https://doc", v)
	}

	var n int
	if err := db.QueryRow(`SELECT count(*) FROM system_settings WHERE key = 'documentation_url'`).Scan(&n); err != nil {
		t.Fatalf("system_settings check: %v", err)
	}
	if n != 0 {
		t.Fatalf("key should be removed from system_settings, found %d", n)
	}
}

func TestMigration032RemovesTenantDefault(t *testing.T) {
	db, cleanup := migrateTo(t, 32)
	defer cleanup()

	// After the default is dropped, an INSERT omitting tenant_id is rejected.
	if _, err := db.Exec(`INSERT INTO teams (name) VALUES ('NoTenant')`); err == nil {
		t.Fatalf("insert without tenant_id should fail after default removed")
	}
	// Explicit tenant_id still works.
	if _, err := db.Exec(`INSERT INTO teams (name, tenant_id) VALUES ('WithTenant', 1)`); err != nil {
		t.Fatalf("insert with explicit tenant_id should succeed: %v", err)
	}
}

func TestMigration031InvitationsAndPeriodUniqueness(t *testing.T) {
	db, cleanup := migrateTo(t, 31)
	defer cleanup()

	// active_tenant_id column exists; invitations table is usable.
	if _, err := db.Exec(`
		INSERT INTO tenant_invitations (tenant_id, email, role, token_hash, status)
		VALUES (1, 'x@example.com', 'user', 'deadbeef', 'pending')`); err != nil {
		t.Fatalf("insert invitation: %v", err)
	}

	// Same period name is allowed in different tenants, rejected within one.
	if _, err := db.Exec(`INSERT INTO tenants (id, slug, name) OVERRIDING SYSTEM VALUE VALUES (2, 'two', 'Two')`); err != nil {
		t.Fatalf("second tenant: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO periods (name, start_date, end_date, sort_order, tenant_id) VALUES ('Q1', '2026-01-01','2026-03-31',1,1)`); err != nil {
		t.Fatalf("period t1: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO periods (name, start_date, end_date, sort_order, tenant_id) VALUES ('Q1', '2026-01-01','2026-03-31',1,2)`); err != nil {
		t.Fatalf("period t2 (same name, other tenant) should be allowed: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO periods (name, start_date, end_date, sort_order, tenant_id) VALUES ('Q1', '2026-01-01','2026-03-31',2,1)`); err == nil {
		t.Fatalf("duplicate period name within tenant 1 should be rejected")
	}
}

func TestMigration030CreatesSettingsTiers(t *testing.T) {
	db, cleanup := migrateTo(t, 30)
	defer cleanup()

	// tenant_settings is per-tenant and usable.
	if _, err := db.Exec(`INSERT INTO tenant_settings (tenant_id, key, value_json) VALUES (1, 'documentation_url', '"https://docs"'::jsonb)`); err != nil {
		t.Fatalf("insert tenant_settings: %v", err)
	}
	// user_settings is per-user and usable.
	var userID int64
	if err := db.QueryRow(`
		INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('google:s', 'google', 's', 'S') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO user_settings (user_id, key, value_json) VALUES ($1, 'theme', '"dark"'::jsonb)`, userID); err != nil {
		t.Fatalf("insert user_settings: %v", err)
	}

	// Product keys remain in system_settings for now (move + read repointing is a later plan):
	// behavior must be preserved by this plan.
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM system_settings WHERE key='new_user_policy'`).Scan(&n); err != nil {
		t.Fatalf("system_settings check: %v", err)
	}
	if n != 1 {
		t.Fatalf("new_user_policy should remain in system_settings, count=%d", n)
	}
}

func TestMigration029AddsTenantIDWithDefault(t *testing.T) {
	db, cleanup := migrateTo(t, 29)
	defer cleanup()

	// Insert without tenant_id — must default to 1 (single-tenant compatibility).
	var teamID int64
	if err := db.QueryRow(`INSERT INTO teams (name) VALUES ('QA') RETURNING id`).Scan(&teamID); err != nil {
		t.Fatalf("insert team: %v", err)
	}
	var tenantID int64
	if err := db.QueryRow(`SELECT tenant_id FROM teams WHERE id=$1`, teamID).Scan(&tenantID); err != nil {
		t.Fatalf("read tenant_id: %v", err)
	}
	if tenantID != 1 {
		t.Fatalf("tenant_id = %d, want 1", tenantID)
	}

	// FK is enforced: a bogus tenant_id is rejected.
	if _, err := db.Exec(`INSERT INTO teams (name, tenant_id) VALUES ('X', 999)`); err == nil {
		t.Fatalf("expected FK violation for tenant_id=999")
	}
}

func TestMigration028BackfillsMemberships(t *testing.T) {
	db, cleanup := migrateTo(t, 27)
	defer cleanup()

	// Seed an admin and a regular user under the pre-028 schema.
	_, err := db.Exec(`
		INSERT INTO users (provider_subject_key, provider, subject, display_name, is_admin)
		VALUES ('google:a', 'google', 'a', 'Admin', true),
		       ('google:u', 'google', 'u', 'User', false)`)
	if err != nil {
		t.Fatalf("seed users: %v", err)
	}

	migrateDBTo(t, db, 28)

	var role string
	err = db.QueryRow(`
		SELECT m.role FROM memberships m
		JOIN users u ON u.id = m.user_id
		WHERE u.provider_subject_key = 'google:a' AND m.tenant_id = 1`).Scan(&role)
	if err != nil {
		t.Fatalf("admin membership: %v", err)
	}
	if role != "admin" {
		t.Fatalf("admin role = %q, want admin", role)
	}

	var sysAdmin bool
	if err := db.QueryRow(`SELECT is_system_admin FROM users WHERE provider_subject_key='google:a'`).Scan(&sysAdmin); err != nil {
		t.Fatalf("is_system_admin: %v", err)
	}
	if !sysAdmin {
		t.Fatalf("admin is_system_admin = false, want true")
	}

	var userRole string
	if err := db.QueryRow(`
		SELECT m.role FROM memberships m JOIN users u ON u.id=m.user_id
		WHERE u.provider_subject_key='google:u'`).Scan(&userRole); err != nil {
		t.Fatalf("user membership: %v", err)
	}
	if userRole != "user" {
		t.Fatalf("user role = %q, want user", userRole)
	}
}
