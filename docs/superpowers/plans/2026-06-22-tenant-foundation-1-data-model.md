# Tenant Foundation — Plan 1: Data Model & Default Tenant

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ввести в коробку схему мультитенантности (`tenants` + дефолтный тенант #1,
`memberships`, `tenant_id` на scoped-таблицах, `tenant_settings`/`user_settings`/
`tenant_invitations`, `is_system_admin`) с backfill существующих данных в `tenant #1`, не
меняя текущее поведение приложения.

**Architecture:** Серия SQL-миграций (golang-migrate, как в репозитории) добавляет схему и
делает backfill. Новые доменные типы `Tenant`/`Membership` и репозитории
`TenantRepository`/`MembershipRepository` встраиваются в существующий composite `store.Store`.
`tenant_id` добавляется как `DEFAULT 1 NOT NULL` — существующие запросы/инсёрты продолжают
работать (получают тенант #1); явное scoping-чтение придёт в Plan 2. Это Plan 1 из серии;
resolver/middleware/онбординг/façade — последующие планы.

**Tech Stack:** Go, pgx/v5, golang-migrate/v4 (file source), PostgreSQL, testcontainers-go.

## Global Constraints

- Source of truth — спека `docs/superpowers/specs/2026-06-21-tenant-foundation-design.md` и
  `specs/0*.md`. Plan реализует только её Фазу 0, слой данных.
- Миграции — единственный способ менять схему; каждая `NNN_name.up.sql` + `NNN_name.down.sql`
  в `migrations/`, нумерация продолжает существующую (последняя — `026`).
- Слои не смешивать: SQL только в `internal/store/<entity>`; никакой бизнес-логики в handlers.
- Каждая сущность — отдельный repository-тип (один файл на сущность), как в существующем
  `internal/store`.
- Тесты БД — паттерн `testcontainers-go` + `runMigrations`/`resolveMigrationsPath`, как в
  `internal/store/store_test.go`; при отсутствии docker — `t.Skipf`.
- Поддерживать `seed_demo.sql` в актуальном состоянии при изменении схемы.
- **Коммиты — за пользователем** (правило репозитория). Если план исполняют агенты — агент
  делает `git add`, формирует commit message и предлагает его; финальный `git commit`
  выполняет пользователь. **Никаких упоминаний AI/Claude/ассистента в commit-сообщениях.**
- `tenant_id` на scoped-таблицах в этом плане — `DEFAULT 1 NOT NULL`. Дефолт `1` —
  транзитный (совместимость одно-тенантного периода); Plan 2 снимает дефолт, когда все
  записи станут tenant-aware.
- `users.is_admin` в этом плане **не удаляется** — только добавляется `is_system_admin` и
  backfill; перевод кода на `membership.role` и удаление `is_admin` — отдельный план.

---

### Task 1: Migration 027 — tenants + default tenant

**Files:**
- Create: `migrations/027_tenants.up.sql`
- Create: `migrations/027_tenants.down.sql`
- Test: `internal/store/migration_tenancy_test.go`

**Interfaces:**
- Produces: таблица `tenants(id, slug, name, status, created_at, deleted_at)` с строкой
  `id=1, slug='default'`. На неё ссылаются FK последующих миграций.

- [ ] **Step 1: Write the failing test**

```go
// internal/store/migration_tenancy_test.go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store -run TestMigration027CreatesDefaultTenant -v`
Expected: FAIL — `relation "tenants" does not exist` (или ошибка миграции: файла нет).

- [ ] **Step 3: Write the up migration**

```sql
-- migrations/027_tenants.up.sql
-- Multi-tenancy foundation: a tenant scopes teams/periods/goals/users.
-- A single-install OSS box uses one default tenant (#1); existing data backfills into it.
CREATE TABLE tenants (
    id         BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    slug       TEXT NOT NULL UNIQUE,
    name       TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

-- Default tenant for OSS single-install and as backfill target for existing rows.
INSERT INTO tenants (id, slug, name) OVERRIDING SYSTEM VALUE
VALUES (1, 'default', 'Default');

-- Advance the identity sequence past the manually inserted id=1, otherwise the next
-- auto-generated id would collide with the default tenant.
SELECT setval(pg_get_serial_sequence('tenants', 'id'), (SELECT max(id) FROM tenants));
```

- [ ] **Step 4: Write the down migration**

```sql
-- migrations/027_tenants.down.sql
DROP TABLE IF EXISTS tenants;
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/store -run TestMigration027CreatesDefaultTenant -v`
Expected: PASS.

- [ ] **Step 6: Stage and prepare commit**

```bash
git add migrations/027_tenants.up.sql migrations/027_tenants.down.sql internal/store/migration_tenancy_test.go
# commit message (user commits): "feat(migrations): add tenants table with default tenant"
```

---

### Task 2: Migration 028 — memberships + is_system_admin

**Files:**
- Create: `migrations/028_memberships.up.sql`
- Create: `migrations/028_memberships.down.sql`
- Modify: `internal/store/migration_tenancy_test.go` (add test)

**Interfaces:**
- Consumes: `tenants` (id=1) из Task 1; `users` (existing, `is_admin`).
- Produces: `memberships(id, user_id, tenant_id, role, status, created_at, created_by_user_id)`,
  unique `(user_id, tenant_id)`; `users.is_system_admin BOOLEAN`. Backfill: каждый
  существующий user получает membership в tenant #1 с `role = CASE is_admin THEN 'admin' ELSE 'user'`;
  `is_system_admin = is_admin`.

- [ ] **Step 1: Write the failing test**

```go
// add to internal/store/migration_tenancy_test.go
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

	// Apply 028.
	driver, _ := migratepostgres.WithInstance(db, &migratepostgres.Config{})
	path, _ := resolveMigrationsPath()
	m, _ := migrate.NewWithDatabaseInstance("file://"+path, "postgres", driver)
	if err := m.Migrate(28); err != nil {
		t.Fatalf("migrate 28: %v", err)
	}

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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store -run TestMigration028BackfillsMemberships -v`
Expected: FAIL — `relation "memberships" does not exist`.

- [ ] **Step 3: Write the up migration**

```sql
-- migrations/028_memberships.up.sql
-- Membership maps a global user into N tenants with a per-tenant role.
-- is_system_admin is the instance superadmin (tenant-less); split out of legacy is_admin.
ALTER TABLE users ADD COLUMN is_system_admin BOOLEAN NOT NULL DEFAULT false;

CREATE TABLE memberships (
    id                 BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    user_id            BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id          BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    role               TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin')),
    status             TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'requested')),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    UNIQUE (user_id, tenant_id)
);

-- Backfill: every existing user becomes an active member of the default tenant.
-- Legacy admins become tenant admins AND instance system admins (they were the operator).
INSERT INTO memberships (user_id, tenant_id, role, status)
SELECT id, 1, CASE WHEN is_admin THEN 'admin' ELSE 'user' END, 'active'
FROM users;

UPDATE users SET is_system_admin = true WHERE is_admin = true;
```

- [ ] **Step 4: Write the down migration**

```sql
-- migrations/028_memberships.down.sql
DROP TABLE IF EXISTS memberships;
ALTER TABLE users DROP COLUMN IF EXISTS is_system_admin;
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/store -run TestMigration028BackfillsMemberships -v`
Expected: PASS.

- [ ] **Step 6: Stage and prepare commit**

```bash
git add migrations/028_memberships.up.sql migrations/028_memberships.down.sql internal/store/migration_tenancy_test.go
# message: "feat(migrations): add memberships and is_system_admin with backfill"
```

---

### Task 3: Migration 029 — tenant_id on scoped tables

**Files:**
- Create: `migrations/029_tenant_scoping.up.sql`
- Create: `migrations/029_tenant_scoping.down.sql`
- Modify: `internal/store/migration_tenancy_test.go` (add test)

**Interfaces:**
- Consumes: `tenants` (id=1).
- Produces: колонка `tenant_id BIGINT NOT NULL DEFAULT 1 REFERENCES tenants(id)` на
  `teams`, `periods`, `goals`, `goal_shares`, `team_period_statuses`,
  `user_hierarchy_grants`, `key_results`, `goal_comments`, `key_result_notes`.

- [ ] **Step 1: Write the failing test**

```go
// add to internal/store/migration_tenancy_test.go
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
	_, err := db.Exec(`INSERT INTO teams (name, tenant_id) VALUES ('X', 999)`)
	if err == nil {
		t.Fatalf("expected FK violation for tenant_id=999")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store -run TestMigration029AddsTenantIDWithDefault -v`
Expected: FAIL — `column "tenant_id" does not exist`.

- [ ] **Step 3: Write the up migration**

```sql
-- migrations/029_tenant_scoping.up.sql
-- Scope every tenant-owned table by tenant_id. DEFAULT 1 keeps existing single-tenant
-- writes working and backfills existing rows; the default is transitional (removed once
-- all writes are tenant-aware). tenant_id is denormalized onto child tables (key_results,
-- goal_comments, key_result_notes) for defense-in-depth: every query carries tenant_id.
ALTER TABLE teams                  ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 1 REFERENCES tenants(id);
ALTER TABLE periods                ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 1 REFERENCES tenants(id);
ALTER TABLE goals                  ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 1 REFERENCES tenants(id);
ALTER TABLE goal_shares            ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 1 REFERENCES tenants(id);
ALTER TABLE team_period_statuses   ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 1 REFERENCES tenants(id);
ALTER TABLE user_hierarchy_grants  ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 1 REFERENCES tenants(id);
ALTER TABLE key_results            ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 1 REFERENCES tenants(id);
ALTER TABLE goal_comments          ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 1 REFERENCES tenants(id);
ALTER TABLE key_result_notes       ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 1 REFERENCES tenants(id);

CREATE INDEX idx_teams_tenant                 ON teams(tenant_id);
CREATE INDEX idx_periods_tenant               ON periods(tenant_id);
CREATE INDEX idx_goals_tenant                 ON goals(tenant_id);
CREATE INDEX idx_goal_shares_tenant           ON goal_shares(tenant_id);
CREATE INDEX idx_team_period_statuses_tenant  ON team_period_statuses(tenant_id);
CREATE INDEX idx_user_hierarchy_grants_tenant ON user_hierarchy_grants(tenant_id);
CREATE INDEX idx_key_results_tenant           ON key_results(tenant_id);
CREATE INDEX idx_goal_comments_tenant         ON goal_comments(tenant_id);
CREATE INDEX idx_key_result_notes_tenant      ON key_result_notes(tenant_id);
```

> **Verify table names before running:** confirm `key_result_notes`, `goal_comments`,
> `team_period_statuses`, `user_hierarchy_grants` match actual table names with
> `rg -n "CREATE TABLE" migrations/`. If a name differs, use the real one.

- [ ] **Step 4: Write the down migration**

```sql
-- migrations/029_tenant_scoping.down.sql
ALTER TABLE teams                  DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE periods                DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE goals                  DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE goal_shares            DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE team_period_statuses   DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE user_hierarchy_grants  DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE key_results            DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE goal_comments          DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE key_result_notes       DROP COLUMN IF EXISTS tenant_id;
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/store -run TestMigration029AddsTenantIDWithDefault -v`
Expected: PASS.

- [ ] **Step 6: Run the full store suite to confirm no behavior change**

Run: `go test ./internal/store/...`
Expected: PASS (existing inserts omit tenant_id and default to 1).

- [ ] **Step 7: Stage and prepare commit**

```bash
git add migrations/029_tenant_scoping.up.sql migrations/029_tenant_scoping.down.sql internal/store/migration_tenancy_test.go
# message: "feat(migrations): add tenant_id scoping to tenant-owned tables"
```

---

### Task 4: Migration 030 — tenant_settings + user_settings

**Files:**
- Create: `migrations/030_settings_tiers.up.sql`
- Create: `migrations/030_settings_tiers.down.sql`
- Modify: `internal/store/migration_tenancy_test.go` (add test)

**Interfaces:**
- Consumes: `tenants` (id=1), `users`.
- Produces: `tenant_settings(tenant_id, key, value_json)` PK `(tenant_id, key)`;
  `user_settings(user_id, key, value_json)` PK `(user_id, key)`.

> **Correction (found during execution):** the product-key MOVE из `system_settings` в
> `tenant_settings` НЕ делается в этом плане. Рантайм всё ещё читает эти ключи из
> `system_settings` через `SettingsRepository`; перенос без репойнтинга чтений молча
> сбросил бы настройки в дефолты (нарушение «поведение не меняется»). Перенос + репойнтинг
> чтений выполняются вместе в Plan 3 (settings tier / Entitlements). Здесь — только создание
> двух таблиц.

- [ ] **Step 1: Write the failing test**

```go
// add to internal/store/migration_tenancy_test.go
func TestMigration030MovesProductKeysToTenantSettings(t *testing.T) {
	db, cleanup := migrateTo(t, 29)
	defer cleanup()

	_, err := db.Exec(`INSERT INTO system_settings (key, value_json) VALUES ('new_user_policy', '"empty"'::jsonb)`)
	if err != nil {
		t.Fatalf("seed system_settings: %v", err)
	}

	driver, _ := migratepostgres.WithInstance(db, &migratepostgres.Config{})
	path, _ := resolveMigrationsPath()
	m, _ := migrate.NewWithDatabaseInstance("file://"+path, "postgres", driver)
	if err := m.Migrate(30); err != nil {
		t.Fatalf("migrate 30: %v", err)
	}

	var val string
	err = db.QueryRow(`SELECT value_json::text FROM tenant_settings WHERE tenant_id=1 AND key='new_user_policy'`).Scan(&val)
	if err != nil {
		t.Fatalf("tenant_settings key: %v", err)
	}
	if val != `"empty"` {
		t.Fatalf("value = %s, want \"empty\"", val)
	}

	// system_settings keeps the global table (now without the moved product key).
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM system_settings WHERE key='new_user_policy'`).Scan(&n); err != nil {
		t.Fatalf("system_settings check: %v", err)
	}
	if n != 0 {
		t.Fatalf("product key still in system_settings: %d", n)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store -run TestMigration030MovesProductKeysToTenantSettings -v`
Expected: FAIL — `relation "tenant_settings" does not exist`.

- [ ] **Step 3: Write the up migration**

```sql
-- migrations/030_settings_tiers.up.sql
-- Three settings tiers: system_settings (global, system-admin) stays as-is for global keys;
-- tenant_settings (per-tenant, tenant-admin, plus entitlement.* keys) is new;
-- user_settings (per-user) is new. Existing product keys move from system_settings into
-- tenant_settings under the default tenant.
CREATE TABLE tenant_settings (
    tenant_id  BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    key        TEXT NOT NULL,
    value_json JSONB NOT NULL,
    PRIMARY KEY (tenant_id, key)
);

CREATE TABLE user_settings (
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key        TEXT NOT NULL,
    value_json JSONB NOT NULL,
    PRIMARY KEY (user_id, key)
);

-- Move per-tenant product keys into the default tenant; keep system_settings for global keys.
INSERT INTO tenant_settings (tenant_id, key, value_json)
SELECT 1, key, value_json FROM system_settings
WHERE key IN ('new_user_policy', 'default_hierarchy_node_id', 'documentation_url', 'health_checkin_config')
   OR key LIKE 'feedback_%';

DELETE FROM system_settings
WHERE key IN ('new_user_policy', 'default_hierarchy_node_id', 'documentation_url', 'health_checkin_config')
   OR key LIKE 'feedback_%';
```

> **Verify key names before running:** confirm the exact `system_settings` keys with
> `rg -n "feedback_|new_user_policy|documentation_url|health_checkin_config|default_hierarchy_node_id" internal specs`.
> Adjust the `IN (...)` / `LIKE` list to the real keys.

- [ ] **Step 4: Write the down migration**

```sql
-- migrations/030_settings_tiers.down.sql
INSERT INTO system_settings (key, value_json)
SELECT key, value_json FROM tenant_settings WHERE tenant_id = 1
ON CONFLICT (key) DO NOTHING;

DROP TABLE IF EXISTS user_settings;
DROP TABLE IF EXISTS tenant_settings;
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/store -run TestMigration030MovesProductKeysToTenantSettings -v`
Expected: PASS.

- [ ] **Step 6: Stage and prepare commit**

```bash
git add migrations/030_settings_tiers.up.sql migrations/030_settings_tiers.down.sql internal/store/migration_tenancy_test.go
# message: "feat(migrations): add tenant_settings and user_settings tiers"
```

---

### Task 5: Migration 031 — invitations, session active tenant, period uniqueness

**Files:**
- Create: `migrations/031_invitations_session_periods.up.sql`
- Create: `migrations/031_invitations_session_periods.down.sql`
- Modify: `internal/store/migration_tenancy_test.go` (add test)

**Interfaces:**
- Produces: `tenant_invitations(id, tenant_id, email, role, token_hash, status, created_by_user_id, created_at, expires_at)`;
  `auth_sessions.active_tenant_id BIGINT REFERENCES tenants(id)`; `periods` имя уникально
  per-tenant (`UNIQUE (tenant_id, name)` вместо глобального).

- [ ] **Step 1: Write the failing test**

```go
// add to internal/store/migration_tenancy_test.go
func TestMigration031InvitationsAndPeriodUniqueness(t *testing.T) {
	db, cleanup := migrateTo(t, 31)
	defer cleanup()

	// active_tenant_id column exists and accepts a tenant.
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store -run TestMigration031InvitationsAndPeriodUniqueness -v`
Expected: FAIL — `relation "tenant_invitations" does not exist`.

- [ ] **Step 3: Write the up migration**

```sql
-- migrations/031_invitations_session_periods.up.sql
-- Token-based invitations (claimed by redeeming the link while logged in, not by email-match).
CREATE TABLE tenant_invitations (
    id                 BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    tenant_id          BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    email              TEXT NOT NULL,
    role               TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin')),
    token_hash         TEXT NOT NULL,
    status             TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'claimed', 'revoked')),
    created_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at         TIMESTAMPTZ
);
CREATE INDEX idx_tenant_invitations_tenant ON tenant_invitations(tenant_id);

-- Session remembers which tenant it is currently viewing.
ALTER TABLE auth_sessions ADD COLUMN active_tenant_id BIGINT REFERENCES tenants(id);

-- Period name is unique per tenant, not globally.
ALTER TABLE periods DROP CONSTRAINT IF EXISTS periods_name_key;
ALTER TABLE periods ADD CONSTRAINT periods_tenant_name_key UNIQUE (tenant_id, name);
```

> **Verify the period name constraint name before running:** find it with
> `rg -n "periods" migrations/*.up.sql | rg -i "unique|constraint|name"`. If the existing
> unique constraint is named differently, drop that exact name.

- [ ] **Step 4: Write the down migration**

```sql
-- migrations/031_invitations_session_periods.down.sql
ALTER TABLE periods DROP CONSTRAINT IF EXISTS periods_tenant_name_key;
ALTER TABLE periods ADD CONSTRAINT periods_name_key UNIQUE (name);
ALTER TABLE auth_sessions DROP COLUMN IF EXISTS active_tenant_id;
DROP TABLE IF EXISTS tenant_invitations;
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/store -run TestMigration031InvitationsAndPeriodUniqueness -v`
Expected: PASS.

- [ ] **Step 6: Stage and prepare commit**

```bash
git add migrations/031_invitations_session_periods.up.sql migrations/031_invitations_session_periods.down.sql internal/store/migration_tenancy_test.go
# message: "feat(migrations): add invitations, session active tenant, per-tenant period names"
```

---

### Task 6: Domain types — Tenant, Membership

**Files:**
- Modify: `internal/domain/models.go`
- Test: `internal/domain/tenant_test.go`

**Interfaces:**
- Produces: `domain.Tenant`, `domain.Membership`, `domain.MembershipStatus`,
  `domain.Role` (`RoleUser`, `RoleAdmin`); `domain.TenantStatus`
  (`TenantActive`, `TenantSuspended`); `User.IsSystemAdmin bool`. Используется
  репозиториями в Task 7–8.

- [ ] **Step 1: Write the failing test**

```go
// internal/domain/tenant_test.go
package domain

import "testing"

func TestTenantSlugValid(t *testing.T) {
	cases := map[string]bool{
		"default":  true,
		"acme":     true,
		"acme-eu":  true,
		"a":        false, // too short (min 2)
		"Acme":     false, // uppercase
		"-acme":    false, // leading dash
		"acme-":    false, // trailing dash
		"www":      false, // reserved
		"api":      false, // reserved
	}
	for slug, want := range cases {
		if got := ValidTenantSlug(slug); got != want {
			t.Errorf("ValidTenantSlug(%q) = %v, want %v", slug, got, want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain -run TestTenantSlugValid -v`
Expected: FAIL — `undefined: ValidTenantSlug`.

- [ ] **Step 3: Add the domain types and slug validator**

```go
// append to internal/domain/models.go
type TenantStatus string

const (
	TenantActive    TenantStatus = "active"
	TenantSuspended TenantStatus = "suspended"
)

type Tenant struct {
	ID        int64
	Slug      string
	Name      string
	Status    TenantStatus
	CreatedAt time.Time
	DeletedAt *time.Time
}

type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

type MembershipStatus string

const (
	MembershipActive    MembershipStatus = "active"
	MembershipRequested MembershipStatus = "requested"
)

type Membership struct {
	ID              int64
	UserID          int64
	TenantID        int64
	Role            Role
	Status          MembershipStatus
	CreatedAt       time.Time
	CreatedByUserID *int64
}

var reservedTenantSlugs = map[string]bool{
	"www": true, "api": true, "app": true, "admin": true, "static": true,
	"assets": true, "mail": true, "auth": true, "system": true,
}

// ValidTenantSlug enforces the slug grammar ^[a-z0-9]([a-z0-9-]{0,30}[a-z0-9])?$
// (lowercase, 2..32 chars, no leading/trailing dash) and rejects reserved subdomains.
func ValidTenantSlug(s string) bool {
	if len(s) < 2 || len(s) > 32 || reservedTenantSlugs[s] {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		isLower := c >= 'a' && c <= 'z'
		isDigit := c >= '0' && c <= '9'
		isDash := c == '-'
		if !isLower && !isDigit && !isDash {
			return false
		}
		if isDash && (i == 0 || i == len(s)-1) {
			return false
		}
	}
	return true
}
```

- [ ] **Step 4: Add the IsSystemAdmin field to User**

In `internal/domain/models.go`, add to `type User struct` after `IsAdmin bool`:

```go
	IsSystemAdmin      bool
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/domain -run TestTenantSlugValid -v`
Expected: PASS.

- [ ] **Step 6: Stage and prepare commit**

```bash
git add internal/domain/models.go internal/domain/tenant_test.go
# message: "feat(domain): add Tenant, Membership types and slug validation"
```

---

### Task 7: TenantRepository

**Files:**
- Create: `internal/store/tenants/tenants.go`
- Test: `internal/store/tenants/tenants_test.go`

**Interfaces:**
- Consumes: `domain.Tenant`, `domain.ValidTenantSlug`.
- Produces:
  - `NewTenantRepository(db *pgxpool.Pool) *TenantRepository`
  - `(*TenantRepository) Create(ctx, slug, name string) (*domain.Tenant, error)` — валидирует slug, конфликт slug → ошибка
  - `(*TenantRepository) GetByID(ctx, id int64) (*domain.Tenant, error)`
  - `(*TenantRepository) GetBySlug(ctx, slug string) (*domain.Tenant, error)`
  - `(*TenantRepository) List(ctx) ([]domain.Tenant, error)` — только `deleted_at IS NULL`

- [ ] **Step 1: Write the failing test**

```go
// internal/store/tenants/tenants_test.go
package tenants

import (
	"context"
	"errors"
	"testing"

	"okrs/internal/store/storetest" // see note below

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTenantRepositoryCreateAndGet(t *testing.T) {
	pool := storetest.NewPool(t) // skips if docker unavailable; runs migrations
	repo := NewTenantRepository(pool)
	ctx := context.Background()

	tn, err := repo.Create(ctx, "acme", "Acme Inc")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if tn.Slug != "acme" || tn.Name != "Acme Inc" {
		t.Fatalf("unexpected tenant: %+v", tn)
	}

	got, err := repo.GetBySlug(ctx, "acme")
	if err != nil {
		t.Fatalf("get by slug: %v", err)
	}
	if got.ID != tn.ID {
		t.Fatalf("id mismatch: %d != %d", got.ID, tn.ID)
	}

	if _, err := repo.Create(ctx, "ACME", "bad"); !errors.Is(err, ErrInvalidSlug) {
		t.Fatalf("expected ErrInvalidSlug, got %v", err)
	}
}

var _ = pgxpool.Pool{}
```

> **Note on `storetest.NewPool`:** if a shared test helper package does not yet exist,
> inline the testcontainers + `runMigrations` setup from `internal/store/store_test.go`
> directly in this test file instead of importing `storetest`. Do not block on creating a
> new helper package — match whatever the repo already does for repo-level DB tests.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/tenants -v`
Expected: FAIL — package/`NewTenantRepository`/`ErrInvalidSlug` undefined.

- [ ] **Step 3: Write the repository**

```go
// internal/store/tenants/tenants.go
package tenants

import (
	"context"
	"errors"
	"fmt"

	"okrs/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidSlug = errors.New("tenants: invalid slug")
	ErrSlugTaken   = errors.New("tenants: slug already taken")
	ErrNotFound    = errors.New("tenants: not found")
)

type TenantRepository struct {
	db *pgxpool.Pool
}

func NewTenantRepository(db *pgxpool.Pool) *TenantRepository {
	return &TenantRepository{db: db}
}

func (r *TenantRepository) Create(ctx context.Context, slug, name string) (*domain.Tenant, error) {
	if !domain.ValidTenantSlug(slug) {
		return nil, ErrInvalidSlug
	}
	var t domain.Tenant
	err := r.db.QueryRow(ctx, `
		INSERT INTO tenants (slug, name) VALUES ($1, $2)
		RETURNING id, slug, name, status, created_at, deleted_at`,
		slug, name).Scan(&t.ID, &t.Slug, &t.Name, &t.Status, &t.CreatedAt, &t.DeletedAt)
	if err != nil {
		// 23505 = unique_violation
		if pgErrCode(err) == "23505" {
			return nil, ErrSlugTaken
		}
		return nil, fmt.Errorf("create tenant: %w", err)
	}
	return &t, nil
}

func (r *TenantRepository) GetByID(ctx context.Context, id int64) (*domain.Tenant, error) {
	return r.getBy(ctx, `WHERE id = $1`, id)
}

func (r *TenantRepository) GetBySlug(ctx context.Context, slug string) (*domain.Tenant, error) {
	return r.getBy(ctx, `WHERE slug = $1`, slug)
}

func (r *TenantRepository) getBy(ctx context.Context, where string, arg any) (*domain.Tenant, error) {
	var t domain.Tenant
	err := r.db.QueryRow(ctx, `
		SELECT id, slug, name, status, created_at, deleted_at FROM tenants `+where, arg).
		Scan(&t.ID, &t.Slug, &t.Name, &t.Status, &t.CreatedAt, &t.DeletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *TenantRepository) List(ctx context.Context) ([]domain.Tenant, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, slug, name, status, created_at, deleted_at FROM tenants
		WHERE deleted_at IS NULL ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Tenant
	for rows.Next() {
		var t domain.Tenant
		if err := rows.Scan(&t.ID, &t.Slug, &t.Name, &t.Status, &t.CreatedAt, &t.DeletedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func pgErrCode(err error) string {
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) {
		return pgErr.SQLState()
	}
	return ""
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/tenants -v`
Expected: PASS.

- [ ] **Step 5: Stage and prepare commit**

```bash
git add internal/store/tenants/
# message: "feat(store): add TenantRepository"
```

---

### Task 8: MembershipRepository

**Files:**
- Create: `internal/store/memberships/memberships.go`
- Test: `internal/store/memberships/memberships_test.go`

**Interfaces:**
- Consumes: `domain.Membership`, `domain.Role`, `domain.MembershipStatus`.
- Produces:
  - `NewMembershipRepository(db *pgxpool.Pool) *MembershipRepository`
  - `(*MembershipRepository) ListByUser(ctx, userID int64) ([]domain.Membership, error)` — только `status='active'`
  - `(*MembershipRepository) Get(ctx, userID, tenantID int64) (*domain.Membership, error)`
  - `(*MembershipRepository) Upsert(ctx, m domain.Membership) (*domain.Membership, error)` — по `(user_id, tenant_id)`
  - `(*MembershipRepository) SetStatus(ctx, userID, tenantID int64, status domain.MembershipStatus) error`

- [ ] **Step 1: Write the failing test**

```go
// internal/store/memberships/memberships_test.go
package memberships

import (
	"context"
	"testing"

	"okrs/internal/domain"
	"okrs/internal/store/storetest"
)

func TestMembershipUpsertAndList(t *testing.T) {
	pool := storetest.NewPool(t) // or inline testcontainers setup, see Task 7 note
	ctx := context.Background()

	var userID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('google:m', 'google', 'm', 'M') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	repo := NewMembershipRepository(pool)
	if _, err := repo.Upsert(ctx, domain.Membership{
		UserID: userID, TenantID: 1, Role: domain.RoleAdmin, Status: domain.MembershipActive,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := repo.ListByUser(ctx, userID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].Role != domain.RoleAdmin {
		t.Fatalf("unexpected memberships: %+v", got)
	}

	// requested membership is excluded from the active list.
	if err := repo.SetStatus(ctx, userID, 1, domain.MembershipRequested); err != nil {
		t.Fatalf("set status: %v", err)
	}
	got, _ = repo.ListByUser(ctx, userID)
	if len(got) != 0 {
		t.Fatalf("requested membership should be excluded, got %+v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/memberships -v`
Expected: FAIL — `NewMembershipRepository` undefined.

- [ ] **Step 3: Write the repository**

```go
// internal/store/memberships/memberships.go
package memberships

import (
	"context"
	"errors"

	"okrs/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("memberships: not found")

type MembershipRepository struct {
	db *pgxpool.Pool
}

func NewMembershipRepository(db *pgxpool.Pool) *MembershipRepository {
	return &MembershipRepository{db: db}
}

func (r *MembershipRepository) Upsert(ctx context.Context, m domain.Membership) (*domain.Membership, error) {
	role := m.Role
	if role == "" {
		role = domain.RoleUser
	}
	status := m.Status
	if status == "" {
		status = domain.MembershipActive
	}
	var out domain.Membership
	err := r.db.QueryRow(ctx, `
		INSERT INTO memberships (user_id, tenant_id, role, status, created_by_user_id)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, tenant_id) DO UPDATE SET role = EXCLUDED.role, status = EXCLUDED.status
		RETURNING id, user_id, tenant_id, role, status, created_at, created_by_user_id`,
		m.UserID, m.TenantID, role, status, m.CreatedByUserID).
		Scan(&out.ID, &out.UserID, &out.TenantID, &out.Role, &out.Status, &out.CreatedAt, &out.CreatedByUserID)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *MembershipRepository) Get(ctx context.Context, userID, tenantID int64) (*domain.Membership, error) {
	var m domain.Membership
	err := r.db.QueryRow(ctx, `
		SELECT id, user_id, tenant_id, role, status, created_at, created_by_user_id
		FROM memberships WHERE user_id = $1 AND tenant_id = $2`, userID, tenantID).
		Scan(&m.ID, &m.UserID, &m.TenantID, &m.Role, &m.Status, &m.CreatedAt, &m.CreatedByUserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *MembershipRepository) ListByUser(ctx context.Context, userID int64) ([]domain.Membership, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, user_id, tenant_id, role, status, created_at, created_by_user_id
		FROM memberships WHERE user_id = $1 AND status = 'active' ORDER BY tenant_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Membership
	for rows.Next() {
		var m domain.Membership
		if err := rows.Scan(&m.ID, &m.UserID, &m.TenantID, &m.Role, &m.Status, &m.CreatedAt, &m.CreatedByUserID); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *MembershipRepository) SetStatus(ctx context.Context, userID, tenantID int64, status domain.MembershipStatus) error {
	ct, err := r.db.Exec(ctx, `UPDATE memberships SET status = $3 WHERE user_id = $1 AND tenant_id = $2`,
		userID, tenantID, status)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/memberships -v`
Expected: PASS.

- [ ] **Step 5: Stage and prepare commit**

```bash
git add internal/store/memberships/
# message: "feat(store): add MembershipRepository"
```

---

### Task 9: Wire repositories into store.Store + seed default tenant

**Files:**
- Modify: `internal/store/store.go`
- Modify: `seed_demo.sql`
- Test: `internal/store/store_test.go` (extend existing CRUD test minimally)

**Interfaces:**
- Consumes: `tenants.NewTenantRepository`, `memberships.NewMembershipRepository`.
- Produces: `store.Store.Tenants *tenants.TenantRepository`,
  `store.Store.Memberships *memberships.MembershipRepository`.

- [ ] **Step 1: Write the failing test**

```go
// add to internal/store/store_test.go
func TestStoreExposesTenantRepos(t *testing.T) {
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
	defer func() { _ = container.Terminate(ctx) }()
	dbURL, _ := container.ConnectionString(ctx, "sslmode=disable")
	if err := runMigrations(dbURL); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, _ := pgxpool.New(ctx, dbURL)
	defer pool.Close()

	s := New(pool)
	tn, err := s.Tenants.GetBySlug(ctx, "default")
	if err != nil {
		t.Fatalf("default tenant: %v", err)
	}
	if tn.ID != 1 {
		t.Fatalf("default tenant id = %d, want 1", tn.ID)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store -run TestStoreExposesTenantRepos -v`
Expected: FAIL — `s.Tenants undefined`.

- [ ] **Step 3: Add fields and wiring to store.go**

In `internal/store/store.go`, add imports `"okrs/internal/store/tenants"` and
`"okrs/internal/store/memberships"`. Add fields to `type Store struct`:

```go
	Tenants     *tenants.TenantRepository
	Memberships *memberships.MembershipRepository
```

And in `func New`, add to the returned `&Store{...}`:

```go
		Tenants:     tenants.NewTenantRepository(db),
		Memberships: memberships.NewMembershipRepository(db),
```

- [ ] **Step 4: Keep seed_demo current**

In `seed_demo.sql`, ensure the default tenant exists before seeded rows (idempotent;
migration already inserts it, so guard against duplicates):

```sql
-- Default tenant (created by migration 027; ensure present for standalone seed runs).
INSERT INTO tenants (id, slug, name) OVERRIDING SYSTEM VALUE
VALUES (1, 'default', 'Default')
ON CONFLICT (id) DO NOTHING;
```

Seeded teams/periods/goals omit `tenant_id` and default to 1 (migration 029) — no other
seed changes required in Plan 1.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/store -run TestStoreExposesTenantRepos -v`
Expected: PASS.

- [ ] **Step 6: Run the full suite + vet**

Run: `go vet ./... && go test ./...`
Expected: PASS (no behavior change for existing flows).

- [ ] **Step 7: Stage and prepare commit**

```bash
git add internal/store/store.go seed_demo.sql internal/store/store_test.go
# message: "feat(store): wire tenant and membership repositories into Store"
```

---

## Self-Review Notes (for the implementer)

- **Spec coverage (Plan 1 slice):** tenants + default tenant (Task 1), memberships +
  `is_system_admin` split (Task 2), `tenant_id` denormalized on scoped + child tables
  (Task 3), three settings tier TABLES created — product-key move deferred to Plan 3 with
  read repointing (Task 4), invitations + session
  active tenant + per-tenant period names (Task 5), domain types + slug rules (Task 6),
  Tenant/Membership repositories (Tasks 7–8), Store wiring + seed (Task 9). Out of this
  plan by design: resolver/middleware/scoping enforcement, entitlements interface,
  provisioning/system endpoints, onboarding handlers, `app` façade — these are Plans 2+.
- **Verify-before-write hooks:** Tasks 3, 4, 5 include `rg` checks for exact table/column/
  constraint/key names. Run them; the SQL assumes conventional names that must be confirmed
  against the real schema.
- **Transitional `DEFAULT 1`:** intentional for single-tenant compatibility; Plan 2 removes
  it once writes pass `tenant_id` explicitly via `TenantScope`.
- **`users.is_admin` retained:** not dropped here; code still reads it. Migration off it is a
  later plan.
