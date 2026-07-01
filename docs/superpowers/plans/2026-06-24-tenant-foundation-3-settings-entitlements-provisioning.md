# Tenant Foundation — Plan 3: Settings Tier, Entitlements & Provisioning

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans (inline,
> compiler-driven — subagents flap in this environment). Steps use checkbox (`- [ ]`) syntax.
> Build/vet/test must be green after every task; commits are the user's (agent does `git add`
> + proposes a message, no AI attribution).

**Goal:** Stand up the three-tier settings model (`system_settings` global / `tenant_settings`
per-tenant / `user_settings` per-user), move the product keys out of `system_settings` into
`tenant_settings` and repoint every read, introduce the `Entitlements` seam (OSS = unlimited),
and add the system-admin provisioning plane (`/api/v1/system/*` + `/system` shell) with
per-namespace write-authority.

**Architecture:** Two parts behind one review gate. **Part A** (Tasks 1–9) builds the settings
stores + snapshot caches + `Entitlements`, migrates product keys to `tenant_settings`, and
repoints reads to carry an explicit `domain.TenantScope`. **Part B** (Tasks 10–16) loads the
already-existing `is_system_admin` column, adds the system-admin gate (session flag **or**
provisioning token), the provisioning service + HTTP API, switches `/admin` to gate on the
active tenant role, and wires cache invalidation. Settings reads go through per-tenant snapshot
caches (one query → `map[key]value`), mirroring the existing `grants.GrantsCache` /
`TenantCache` pattern from Plans 2b/2c.

**Tech Stack:** Go, pgx/v5, chi, testcontainers-go, golang-migrate (last applied migration: 032).

---

## Spec ↔ code mismatch notes (read before implementing)

1. **Product keys still in `system_settings`.** `new_user_policy`, `default_hierarchy_node_id`,
   `documentation_url`, `feedback_*`, `health_checkin_config` are read via
   `store.GetSetting`/`SettingsRepository` (global, unscoped). Spec §"Настройки — три уровня"
   wants them in `tenant_settings`. Migration 030 created the table but deliberately did **not**
   move them (its own comment defers it to "the settings tier / Entitlements plan" — this plan).
   → Tasks 7–9.
2. **`is_system_admin` is written but never read.** Migration 028 added `users.is_system_admin`
   and `domain.User.IsSystemAdmin` exists, but every `SELECT` in `internal/store/users/users.go`
   ends `… is_admin, created_at …` and `scanUser` scans `&u.IsAdmin, &u.CreatedAt, …` — the
   column is never loaded, so `IsSystemAdmin` is always `false`. The system-admin plane can't
   work until this is fixed. → Task 10.
3. **No `Entitlements`, no provisioning, no `/system`.** None of these types/routes exist yet.
   → Tasks 6, 13, 14.
4. **`/admin` gates on the legacy global `user.IsAdmin`** (`auth.RequireAdminMiddleware`), not on
   the per-tenant membership role. Spec §"Плоскости администрирования" makes `/admin` the
   tenant-admin plane (`membership.role = admin`). The active role is already in context
   (`auth.ActiveRoleFromContext`, set by `TenantResolveMiddleware`). → Task 15.

These are addressed in this change set. No unrelated specs are touched; specs `040`/`050` (+`020`)
are updated in Task 16 to match.

---

## Global Constraints

- **Explicit `domain.TenantScope`** is the first business param (after `ctx`) on every scoped
  service/repo method. Services/repos never read tenant from context; only HTTP handlers do, via
  `auth.TenantScopeFromContext` (returns `(scope, ok)`; 403 when `!ok`). System-admin status is
  read from context only at the HTTP boundary too (`auth.UserFromContext().IsSystemAdmin`).
- **Write-authority by namespace lives in the service layer.** Keys under `entitlement.*` are
  writable only by system-admin/provisioning; all other `tenant_settings` keys are tenant-admin
  product keys. The store is policy-free; the guard is in `service.SettingsService`.
- **`system_settings` stays global** (no `tenant_id`). Only `default_registration_tenant_id` (and
  future instance keys) live there.
- **Snapshot, not per-key reads.** Tenant settings load as one `map[string]json.RawMessage` per
  tenant and are cached; `Has`/`Get`/entitlement lookups hit the cached snapshot. Mirror
  `internal/store/grants.GrantsCache`: `sync.RWMutex` + per-key `cachedAt` + TTL (5 min, same
  `defaultGrantsCacheTTL` value) + explicit `Invalidate`.
- **Migrations** are golang-migrate, idempotent, with `.down.sql`; new ones start at **033**.
  `store/testutil` + `api/v1/testutil` restore `DEFAULT 1` on scoped tables for single-tenant
  fixtures (Plan 2b Task 9 dropped the production default).
- **Behaviour-preserving for the default tenant #1.** Single-tenant OSS keeps working unchanged.
- **Commits are the user's.** Agent stages (`git add`) and proposes a message; **no mention of
  AI/Claude/assistants** in messages, code, or comments (CLAUDE.md).
- Keep `seed_demo.sql` and the demo seed in sync when table structure or seeded settings change.

---

# Part A — Settings tier, key move, Entitlements

### Task 1: `TenantSettings` repository (snapshot reads + scoped writes)

**Files:**
- Create: `internal/store/tenantsettings/tenantsettings.go`
- Test: `internal/store/tenantsettings/tenantsettings_test.go`

**Interfaces:**
- Consumes: `domain.TenantScope` (`scope.TenantID int64`), pgxpool.
- Produces:
  - `func NewTenantSettingsRepository(db *pgxpool.Pool) *TenantSettingsRepository`
  - `(*TenantSettingsRepository) GetAll(ctx, scope domain.TenantScope) (map[string]json.RawMessage, error)`
  - `(*TenantSettingsRepository) Get(ctx, scope domain.TenantScope, key string) (json.RawMessage, error)`
  - `(*TenantSettingsRepository) Set(ctx, scope domain.TenantScope, key string, value any) error`
  - `(*TenantSettingsRepository) Delete(ctx, scope domain.TenantScope, key string) error`

- [ ] **Step 1: Write the failing isolation test**

```go
// internal/store/tenantsettings/tenantsettings_test.go
package tenantsettings_test

import (
	"context"
	"encoding/json"
	"testing"

	"okrs/internal/domain"
	"okrs/internal/store/tenantsettings"
	"okrs/internal/store/testutil"
)

func TestTenantSettingsScopedByTenant(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO tenants (id, slug, name) OVERRIDING SYSTEM VALUE VALUES (2,'t2','T2')`); err != nil {
		t.Fatalf("tenant 2: %v", err)
	}
	repo := tenantsettings.NewTenantSettingsRepository(pool)
	s1 := domain.TenantScope{TenantID: 1}
	s2 := domain.TenantScope{TenantID: 2}

	if err := repo.Set(ctx, s1, "documentation_url", "https://a"); err != nil {
		t.Fatalf("set t1: %v", err)
	}
	if err := repo.Set(ctx, s2, "documentation_url", "https://b"); err != nil {
		t.Fatalf("set t2: %v", err)
	}

	got, err := repo.Get(ctx, s1, "documentation_url")
	if err != nil {
		t.Fatalf("get t1: %v", err)
	}
	var url string
	_ = json.Unmarshal(got, &url)
	if url != "https://a" {
		t.Fatalf("t1 saw %q, want https://a", url)
	}

	all2, err := repo.GetAll(ctx, s2)
	if err != nil {
		t.Fatalf("getall t2: %v", err)
	}
	if len(all2) != 1 {
		t.Fatalf("t2 snapshot = %v, want 1 key", all2)
	}
}
```

- [ ] **Step 2: Run to verify it fails to compile** — `go test ./internal/store/tenantsettings`
  → FAIL (package/methods absent).

- [ ] **Step 3: Implement the repository**

```go
// internal/store/tenantsettings/tenantsettings.go
package tenantsettings

import (
	"context"
	"encoding/json"

	"okrs/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TenantSettingsRepository persists per-tenant key/value settings (product keys
// + entitlement.* keys). It is policy-free; write-authority is enforced in the
// service layer.
type TenantSettingsRepository struct {
	db *pgxpool.Pool
}

func NewTenantSettingsRepository(db *pgxpool.Pool) *TenantSettingsRepository {
	return &TenantSettingsRepository{db: db}
}

// GetAll loads the whole tenant snapshot in one query.
func (r *TenantSettingsRepository) GetAll(ctx context.Context, scope domain.TenantScope) (map[string]json.RawMessage, error) {
	rows, err := r.db.Query(ctx, `SELECT key, value_json FROM tenant_settings WHERE tenant_id = $1`, scope.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]json.RawMessage)
	for rows.Next() {
		var k string
		var v json.RawMessage
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

func (r *TenantSettingsRepository) Get(ctx context.Context, scope domain.TenantScope, key string) (json.RawMessage, error) {
	row := r.db.QueryRow(ctx, `SELECT value_json FROM tenant_settings WHERE tenant_id = $1 AND key = $2`, scope.TenantID, key)
	var raw json.RawMessage
	err := row.Scan(&raw)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return raw, err
}

func (r *TenantSettingsRepository) Set(ctx context.Context, scope domain.TenantScope, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO tenant_settings (tenant_id, key, value_json) VALUES ($1, $2, $3)
		ON CONFLICT (tenant_id, key) DO UPDATE SET value_json = EXCLUDED.value_json`,
		scope.TenantID, key, raw)
	return err
}

func (r *TenantSettingsRepository) Delete(ctx context.Context, scope domain.TenantScope, key string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM tenant_settings WHERE tenant_id = $1 AND key = $2`, scope.TenantID, key)
	return err
}
```

- [ ] **Step 4: Run** `go test ./internal/store/tenantsettings` → PASS.
- [ ] **Step 5: Stage** — `git add internal/store/tenantsettings/`
  (message: `feat(store): add tenant_settings repository`).

---

### Task 2: `TenantSettingsCache` (per-tenant snapshot, TTL + invalidate)

**Files:**
- Create: `internal/store/tenantsettings/cache.go`
- Test: `internal/store/tenantsettings/cache_test.go`

**Interfaces:**
- Consumes: a backend `snapshotBackend interface { GetAll(ctx, domain.TenantScope) (map[string]json.RawMessage, error) }` (the repo satisfies it).
- Produces:
  - `func NewTenantSettingsCache(repo *TenantSettingsRepository) *TenantSettingsCache`
  - `(*TenantSettingsCache) GetAll(ctx, scope domain.TenantScope) (map[string]json.RawMessage, error)`
  - `(*TenantSettingsCache) Invalidate(tenantID int64)` / `InvalidateAll()`
  - test ctor `newTenantSettingsCacheWithBackend(b snapshotBackend, ttl time.Duration) *TenantSettingsCache`

- [ ] **Step 1: Write the failing test** (mirror `grants_cache_test.go`)

```go
// internal/store/tenantsettings/cache_test.go
package tenantsettings

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"okrs/internal/domain"
)

type fakeBackend struct {
	calls int
	data  map[int64]map[string]json.RawMessage
}

func (f *fakeBackend) GetAll(_ context.Context, scope domain.TenantScope) (map[string]json.RawMessage, error) {
	f.calls++
	return f.data[scope.TenantID], nil
}

func TestTenantSettingsCacheCachesPerTenant(t *testing.T) {
	b := &fakeBackend{data: map[int64]map[string]json.RawMessage{
		1: {"documentation_url": json.RawMessage(`"https://a"`)},
	}}
	c := newTenantSettingsCacheWithBackend(b, time.Minute)
	ctx := context.Background()
	s1 := domain.TenantScope{TenantID: 1}

	if _, err := c.GetAll(ctx, s1); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetAll(ctx, s1); err != nil {
		t.Fatal(err)
	}
	if b.calls != 1 {
		t.Fatalf("expected 1 backend call, got %d", b.calls)
	}
	c.Invalidate(1)
	if _, err := c.GetAll(ctx, s1); err != nil {
		t.Fatal(err)
	}
	if b.calls != 2 {
		t.Fatalf("expected reload after invalidate, got %d", b.calls)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `go test ./internal/store/tenantsettings -run TestTenantSettingsCache`.

- [ ] **Step 3: Implement the cache** (same shape as `grants.GrantsCache`)

```go
// internal/store/tenantsettings/cache.go
package tenantsettings

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"okrs/internal/domain"
)

const defaultTenantSettingsCacheTTL = 5 * time.Minute

type snapshotBackend interface {
	GetAll(ctx context.Context, scope domain.TenantScope) (map[string]json.RawMessage, error)
}

type cacheEntry struct {
	snapshot map[string]json.RawMessage
	cachedAt time.Time
}

// TenantSettingsCache wraps a snapshot backend with a TTL + invalidate-on-write cache,
// keyed by tenant id. Single-process; cross-instance invalidation is a SaaS-scale concern.
type TenantSettingsCache struct {
	backend snapshotBackend
	ttl     time.Duration
	mu      sync.RWMutex
	entries map[int64]cacheEntry
}

func NewTenantSettingsCache(repo *TenantSettingsRepository) *TenantSettingsCache {
	return newTenantSettingsCacheWithBackend(repo, defaultTenantSettingsCacheTTL)
}

func newTenantSettingsCacheWithBackend(b snapshotBackend, ttl time.Duration) *TenantSettingsCache {
	return &TenantSettingsCache{backend: b, ttl: ttl, entries: make(map[int64]cacheEntry)}
}

func (c *TenantSettingsCache) GetAll(ctx context.Context, scope domain.TenantScope) (map[string]json.RawMessage, error) {
	c.mu.RLock()
	e, ok := c.entries[scope.TenantID]
	c.mu.RUnlock()
	if ok && time.Since(e.cachedAt) < c.ttl {
		return e.snapshot, nil
	}
	snap, err := c.backend.GetAll(ctx, scope)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.entries[scope.TenantID] = cacheEntry{snapshot: snap, cachedAt: time.Now()}
	c.mu.Unlock()
	return snap, nil
}

func (c *TenantSettingsCache) Invalidate(tenantID int64) {
	c.mu.Lock()
	delete(c.entries, tenantID)
	c.mu.Unlock()
}

func (c *TenantSettingsCache) InvalidateAll() {
	c.mu.Lock()
	c.entries = make(map[int64]cacheEntry)
	c.mu.Unlock()
}
```

> `time.Now()` is fine in production code; the cache test injects a long TTL and only checks
> call counts, not wall-clock expiry, so it stays deterministic.

- [ ] **Step 4: Run** `go test ./internal/store/tenantsettings` → PASS.
- [ ] **Step 5: Stage** (message: `feat(store): cache tenant_settings snapshots per tenant`).

---

### Task 3: `system_settings` global snapshot — `ListAll` + `SystemSettingsCache`

**Files:**
- Modify: `internal/store/settings/settings.go` (add `ListAll`)
- Create: `internal/store/settings/cache.go`
- Test: `internal/store/settings/cache_test.go`

**Interfaces:**
- Produces:
  - `(*SettingsRepository) ListAll(ctx) (map[string]json.RawMessage, error)`
  - `func NewSystemSettingsCache(repo *SettingsRepository) *SystemSettingsCache`
  - `(*SystemSettingsCache) GetAll(ctx) (map[string]json.RawMessage, error)`
  - `(*SystemSettingsCache) Get(ctx, key string) (json.RawMessage, error)` (snapshot lookup)
  - `(*SystemSettingsCache) Invalidate()`

- [ ] **Step 1: Write the failing cache test** (fake backend, count calls)

```go
// internal/store/settings/cache_test.go
package settings

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

type fakeSysBackend struct {
	calls int
	data  map[string]json.RawMessage
}

func (f *fakeSysBackend) ListAll(context.Context) (map[string]json.RawMessage, error) {
	f.calls++
	return f.data, nil
}

func TestSystemSettingsCacheGlobalSnapshot(t *testing.T) {
	b := &fakeSysBackend{data: map[string]json.RawMessage{"default_registration_tenant_id": json.RawMessage(`1`)}}
	c := newSystemSettingsCacheWithBackend(b, time.Minute)
	ctx := context.Background()
	if _, err := c.GetAll(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(ctx, "default_registration_tenant_id"); err != nil {
		t.Fatal(err)
	}
	if b.calls != 1 {
		t.Fatalf("expected 1 backend call (snapshot reused), got %d", b.calls)
	}
	c.Invalidate()
	if _, err := c.GetAll(ctx); err != nil {
		t.Fatal(err)
	}
	if b.calls != 2 {
		t.Fatalf("expected reload after invalidate, got %d", b.calls)
	}
}
```

- [ ] **Step 2: Run to verify it fails.**

- [ ] **Step 3: Add `ListAll` to the repository**

```go
// append to internal/store/settings/settings.go
func (r *SettingsRepository) ListAll(ctx context.Context) (map[string]json.RawMessage, error) {
	rows, err := r.db.Query(ctx, `SELECT key, value_json FROM system_settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]json.RawMessage)
	for rows.Next() {
		var k string
		var v json.RawMessage
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Implement `SystemSettingsCache`**

```go
// internal/store/settings/cache.go
package settings

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

const defaultSystemSettingsCacheTTL = 5 * time.Minute

type listAllBackend interface {
	ListAll(ctx context.Context) (map[string]json.RawMessage, error)
}

// SystemSettingsCache holds the (tiny) global system_settings snapshot.
type SystemSettingsCache struct {
	backend  listAllBackend
	ttl      time.Duration
	mu       sync.RWMutex
	snapshot map[string]json.RawMessage
	cachedAt time.Time
	loaded   bool
}

func NewSystemSettingsCache(repo *SettingsRepository) *SystemSettingsCache {
	return newSystemSettingsCacheWithBackend(repo, defaultSystemSettingsCacheTTL)
}

func newSystemSettingsCacheWithBackend(b listAllBackend, ttl time.Duration) *SystemSettingsCache {
	return &SystemSettingsCache{backend: b, ttl: ttl}
}

func (c *SystemSettingsCache) GetAll(ctx context.Context) (map[string]json.RawMessage, error) {
	c.mu.RLock()
	if c.loaded && time.Since(c.cachedAt) < c.ttl {
		snap := c.snapshot
		c.mu.RUnlock()
		return snap, nil
	}
	c.mu.RUnlock()
	snap, err := c.backend.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.snapshot, c.cachedAt, c.loaded = snap, time.Now(), true
	c.mu.Unlock()
	return snap, nil
}

func (c *SystemSettingsCache) Get(ctx context.Context, key string) (json.RawMessage, error) {
	snap, err := c.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	return snap[key], nil
}

func (c *SystemSettingsCache) Invalidate() {
	c.mu.Lock()
	c.loaded = false
	c.mu.Unlock()
}
```

- [ ] **Step 5: Run** `go test ./internal/store/settings` → PASS.
- [ ] **Step 6: Stage** (message: `feat(store): add system_settings global snapshot cache`).

---

### Task 4: `UserSettings` repository (minimal, per-user)

**Files:**
- Create: `internal/store/usersettings/usersettings.go`
- Test: `internal/store/usersettings/usersettings_test.go`

**Interfaces:**
- Produces:
  - `func NewUserSettingsRepository(db *pgxpool.Pool) *UserSettingsRepository`
  - `(*UserSettingsRepository) GetAll(ctx, userID int64) (map[string]json.RawMessage, error)`
  - `(*UserSettingsRepository) Get(ctx, userID int64, key string) (json.RawMessage, error)`
  - `(*UserSettingsRepository) Set(ctx, userID int64, key string, value any) error`

> Not on the hot path (loaded only on `/settings`), so no cache — per spec §"Оптимизация
> горячего пути".

- [ ] **Step 1: Write the failing test**

```go
// internal/store/usersettings/usersettings_test.go
package usersettings_test

import (
	"context"
	"encoding/json"
	"testing"

	"okrs/internal/store/testutil"
	"okrs/internal/store/usersettings"
)

func TestUserSettingsRoundTrip(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := usersettings.NewUserSettingsRepository(pool)

	// user id 1 (anonymous-local) exists from migrations.
	if err := repo.Set(ctx, 1, "default_landing_tenant_id", 1); err != nil {
		t.Fatalf("set: %v", err)
	}
	raw, err := repo.Get(ctx, 1, "default_landing_tenant_id")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	var id int64
	_ = json.Unmarshal(raw, &id)
	if id != 1 {
		t.Fatalf("got %d, want 1", id)
	}
}
```

- [ ] **Step 2: Run to verify it fails.**

- [ ] **Step 3: Implement** (same shape as `tenantsettings` but keyed by `user_id`; `GetAll`
  selects `WHERE user_id = $1`, `Set` upserts on `(user_id, key)`).

```go
// internal/store/usersettings/usersettings.go
package usersettings

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserSettingsRepository struct {
	db *pgxpool.Pool
}

func NewUserSettingsRepository(db *pgxpool.Pool) *UserSettingsRepository {
	return &UserSettingsRepository{db: db}
}

func (r *UserSettingsRepository) GetAll(ctx context.Context, userID int64) (map[string]json.RawMessage, error) {
	rows, err := r.db.Query(ctx, `SELECT key, value_json FROM user_settings WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]json.RawMessage)
	for rows.Next() {
		var k string
		var v json.RawMessage
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

func (r *UserSettingsRepository) Get(ctx context.Context, userID int64, key string) (json.RawMessage, error) {
	row := r.db.QueryRow(ctx, `SELECT value_json FROM user_settings WHERE user_id = $1 AND key = $2`, userID, key)
	var raw json.RawMessage
	err := row.Scan(&raw)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return raw, err
}

func (r *UserSettingsRepository) Set(ctx context.Context, userID int64, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO user_settings (user_id, key, value_json) VALUES ($1, $2, $3)
		ON CONFLICT (user_id, key) DO UPDATE SET value_json = EXCLUDED.value_json`,
		userID, key, raw)
	return err
}
```

- [ ] **Step 4: Run** `go test ./internal/store/usersettings` → PASS.
- [ ] **Step 5: Wire all four new repos into `internal/store/store.go`** — add fields
  `TenantSettings *tenantsettings.TenantSettingsRepository`, `UserSettings *usersettings.UserSettingsRepository`
  and construct them in `New` (alongside the existing `Settings`, `Tenants`, `Memberships`).
  `go build ./...` → green.
- [ ] **Step 6: Stage** (message: `feat(store): add user_settings repository and wire settings stores`).

---

### Task 5: `SettingsService` — snapshot reads + write-authority guard

**Files:**
- Create: `internal/service/settings.go`
- Test: `internal/service/settings_test.go`

**Interfaces:**
- Consumes: `*tenantsettings.TenantSettingsCache`, `*tenantsettings.TenantSettingsRepository`
  (for writes), `*settings.SystemSettingsCache`.
- Produces:
  - `const EntitlementPrefix = "entitlement."`
  - `var ErrEntitlementNamespace = errors.New("settings: entitlement.* is system-admin only")`
  - `func NewSettingsService(tsCache *tenantsettings.TenantSettingsCache, tsRepo *tenantsettings.TenantSettingsRepository, sysCache *settings.SystemSettingsCache) *SettingsService`
  - `(*SettingsService) TenantSnapshot(ctx, scope domain.TenantScope) (map[string]json.RawMessage, error)`
  - `(*SettingsService) GetTenant(ctx, scope, key string) (json.RawMessage, error)` (snapshot lookup)
  - `(*SettingsService) SetTenantProduct(ctx, scope, key string, value any) error` — rejects
    `entitlement.*` with `ErrEntitlementNamespace`; invalidates the tenant snapshot.
  - `(*SettingsService) SetTenantEntitlement(ctx, scope, key string, value any) error` — system
    path; requires `entitlement.*` prefix; invalidates.
  - `(*SettingsService) SystemGet(ctx, key string) (json.RawMessage, error)` /
    `SystemSet(ctx, key string, value any) error` (system_settings; invalidates the global cache —
    needs the underlying repo too; pass `*settings.SettingsRepository` in).

> The service holds both the cache (reads) and the repo (writes) and invalidates the cache after
> each write — the invalidate-on-write hook the spec asks for. `GetTenant` does a snapshot
> lookup, not a per-key SQL read.

- [ ] **Step 1: Write the failing test** (no DB — use the cache's test backend? It needs a real
  repo for writes). Use `testutil.SetupDB` for a real round-trip + authority:

```go
// internal/service/settings_test.go
package service_test

import (
	"context"
	"testing"

	"okrs/internal/domain"
	"okrs/internal/service"
	"okrs/internal/store/settings"
	"okrs/internal/store/tenantsettings"
	"okrs/internal/store/testutil"
)

func TestSettingsServiceWriteAuthority(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	tsRepo := tenantsettings.NewTenantSettingsRepository(pool)
	sysRepo := settings.NewSettingsRepository(pool)
	svc := service.NewSettingsService(
		tenantsettings.NewTenantSettingsCache(tsRepo), tsRepo,
		settings.NewSystemSettingsCache(sysRepo), sysRepo,
	)
	scope := domain.TenantScope{TenantID: 1}

	// Tenant-admin product write is allowed and visible via snapshot.
	if err := svc.SetTenantProduct(ctx, scope, "documentation_url", "https://x"); err != nil {
		t.Fatalf("product write: %v", err)
	}
	if raw, _ := svc.GetTenant(ctx, scope, "documentation_url"); raw == nil {
		t.Fatalf("product key not visible after write")
	}

	// Tenant-admin cannot write entitlement.* via the product path.
	if err := svc.SetTenantProduct(ctx, scope, "entitlement.sso", true); err != service.ErrEntitlementNamespace {
		t.Fatalf("expected ErrEntitlementNamespace, got %v", err)
	}

	// System/provisioning path can write entitlement.*.
	if err := svc.SetTenantEntitlement(ctx, scope, "entitlement.sso", true); err != nil {
		t.Fatalf("entitlement write: %v", err)
	}
}
```

- [ ] **Step 2: Run to verify it fails.**

- [ ] **Step 3: Implement `SettingsService`** with the signatures above. `NewSettingsService`
  takes `(tsCache, tsRepo, sysCache, sysRepo)`. `SetTenantProduct` returns
  `ErrEntitlementNamespace` if `strings.HasPrefix(key, EntitlementPrefix)`, else
  `tsRepo.Set` + `tsCache.Invalidate(scope.TenantID)`. `SetTenantEntitlement` requires the prefix
  (else `ErrEntitlementNamespace`), then `tsRepo.Set` + invalidate. `TenantSnapshot`/`GetTenant`
  read from `tsCache`. `SystemSet` = `sysRepo.SetSetting` + `sysCache.Invalidate()`.

- [ ] **Step 4: Run** `go test ./internal/service -run TestSettingsServiceWriteAuthority` → PASS.
- [ ] **Step 5: Stage** (message: `feat(service): settings service with per-namespace write-authority`).

---

### Task 6: `Entitlements` interface + `UnlimitedEntitlements` + registry

**Files:**
- Create: `internal/entitlements/entitlements.go`
- Test: `internal/entitlements/entitlements_test.go`

**Interfaces:**
- Produces:
  - `type Entitlements interface { Has(scope domain.TenantScope, key string) bool; Limit(scope domain.TenantScope, key string) int64 }`
  - `const Unlimited int64 = -1`
  - `type UnlimitedEntitlements struct{}` implementing the interface (`Has` → `true`,
    `Limit` → `Unlimited`).
  - `type Factory func() Entitlements`
  - `func Register(name string, f Factory)` / `func Get(name string) (Factory, bool)` (registry,
    same shape as `auth.Register`).

> OSS registers `"unlimited"`. SaaS later registers a snapshot-reading impl that consults
> `entitlement.*` keys; that impl is out of scope (Фаза 1). The interface takes `domain.TenantScope`
> (not context) so it obeys the layering rule. Keep this package free of `internal/store` imports —
> it is a pure seam.

- [ ] **Step 1: Write the failing test**

```go
// internal/entitlements/entitlements_test.go
package entitlements_test

import (
	"testing"

	"okrs/internal/domain"
	"okrs/internal/entitlements"
)

func TestUnlimitedEntitlements(t *testing.T) {
	var e entitlements.Entitlements = entitlements.UnlimitedEntitlements{}
	scope := domain.TenantScope{TenantID: 1}
	if !e.Has(scope, "entitlement.sso") {
		t.Fatal("OSS must allow every feature")
	}
	if e.Limit(scope, "entitlement.max_users") != entitlements.Unlimited {
		t.Fatal("OSS limit must be Unlimited")
	}
}

func TestRegistry(t *testing.T) {
	entitlements.Register("unlimited", func() entitlements.Entitlements { return entitlements.UnlimitedEntitlements{} })
	f, ok := entitlements.Get("unlimited")
	if !ok || f() == nil {
		t.Fatal("registry round-trip failed")
	}
}
```

- [ ] **Step 2: Run to verify it fails.**
- [ ] **Step 3: Implement** the interface, `UnlimitedEntitlements`, and a mutex-guarded registry
  map (`map[string]Factory`).
- [ ] **Step 4: Run** `go test ./internal/entitlements` → PASS.
- [ ] **Step 5: Stage** (message: `feat(entitlements): Entitlements seam with OSS unlimited impl`).

---

### Task 7: Migration 033 — move product keys `system_settings` → `tenant_settings` (tenant #1)

**Files:**
- Create: `migrations/033_move_product_settings.up.sql` / `.down.sql`
- Test: `internal/store/migration_tenancy_test.go` (add a case)

**Interfaces:**
- Produces: product keys live in `tenant_settings` under tenant #1; `system_settings` keeps only
  global keys. Idempotent (re-run is a no-op).
- Product keys (verbatim): `new_user_policy`, `default_hierarchy_node_id`, `documentation_url`,
  `feedback_url`, `feedback_popup_enabled`, `feedback_menu_link_enabled`, `feedback_frequency_days`,
  `health_checkin_config`.

- [ ] **Step 1: Write the failing test** (seed a product key in `system_settings` before
  migrating, assert it lands in `tenant_settings`). Follow the existing `migrateTo`/`migration_tenancy_test.go`
  harness — check there for the exact helper name (it migrates to a target version):

```go
// add to internal/store/migration_tenancy_test.go
func TestMigration033MovesProductKeys(t *testing.T) {
	db, cleanup := migrateTo(t, 32) // up to before the move
	defer cleanup()
	ctx := context.Background()
	if _, err := db.Exec(ctx, `INSERT INTO system_settings (key, value_json) VALUES ('documentation_url', '"https://doc"')`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	stepTo(t, db, 33) // apply the move (use the harness's "migrate one of the open DB" helper)

	var v string
	if err := db.QueryRow(ctx, `SELECT value_json #>> '{}' FROM tenant_settings WHERE tenant_id = 1 AND key = 'documentation_url'`).Scan(&v); err != nil {
		t.Fatalf("expected key moved to tenant_settings: %v", err)
	}
	if v != "https://doc" {
		t.Fatalf("got %q", v)
	}
	var n int
	_ = db.QueryRow(ctx, `SELECT count(*) FROM system_settings WHERE key = 'documentation_url'`).Scan(&n)
	if n != 0 {
		t.Fatalf("key should be removed from system_settings, found %d", n)
	}
}
```

> **Verify the harness first:** open `migration_tenancy_test.go` and reuse its real
> migrate-to-version helper(s) (`migrateTo`, and however it steps an already-open DB forward).
> Adapt the two helper calls above to the actual names; the assertions are the point.

- [ ] **Step 2: Run to verify it fails** (migration 033 absent).

- [ ] **Step 3: Write the up migration**

```sql
-- migrations/033_move_product_settings.up.sql
-- Move product keys out of the global system_settings into per-tenant tenant_settings
-- (default tenant #1). system_settings keeps only instance-global keys. Idempotent.
INSERT INTO tenant_settings (tenant_id, key, value_json)
SELECT 1, key, value_json FROM system_settings
WHERE key IN (
    'new_user_policy', 'default_hierarchy_node_id', 'documentation_url',
    'feedback_url', 'feedback_popup_enabled', 'feedback_menu_link_enabled',
    'feedback_frequency_days', 'health_checkin_config'
)
ON CONFLICT (tenant_id, key) DO NOTHING;

DELETE FROM system_settings
WHERE key IN (
    'new_user_policy', 'default_hierarchy_node_id', 'documentation_url',
    'feedback_url', 'feedback_popup_enabled', 'feedback_menu_link_enabled',
    'feedback_frequency_days', 'health_checkin_config'
);
```

- [ ] **Step 4: Write the down migration** (reverse: copy those keys back from
  `tenant_settings` tenant #1 into `system_settings`, then delete them from `tenant_settings`).

```sql
-- migrations/033_move_product_settings.down.sql
INSERT INTO system_settings (key, value_json)
SELECT key, value_json FROM tenant_settings
WHERE tenant_id = 1 AND key IN (
    'new_user_policy', 'default_hierarchy_node_id', 'documentation_url',
    'feedback_url', 'feedback_popup_enabled', 'feedback_menu_link_enabled',
    'feedback_frequency_days', 'health_checkin_config'
)
ON CONFLICT (key) DO NOTHING;

DELETE FROM tenant_settings
WHERE tenant_id = 1 AND key IN (
    'new_user_policy', 'default_hierarchy_node_id', 'documentation_url',
    'feedback_url', 'feedback_popup_enabled', 'feedback_menu_link_enabled',
    'feedback_frequency_days', 'health_checkin_config'
);
```

- [ ] **Step 5: Run** `go test ./internal/store -run TestMigration033` → PASS.
- [ ] **Step 6: Stage** (message: `feat(migrations): move product settings to tenant_settings (033)`).

> After this task the running code STILL reads from `system_settings` and will silently fall back
> to defaults until Tasks 8–9 repoint the reads. Tasks 7→8→9 are one logical unit — do not pause
> for review between them; the suite is only fully consistent after Task 9.

---

### Task 8: Repoint the admin settings handler to `tenant_settings`

**Files:**
- Modify: `internal/http/handlers/api/v1/admin/handler.go` (settings get/set methods + the
  `settingsStore` interface)
- Modify: `internal/http/server.go` (`registerAdminRoutes` — construct `apiadmin.New` with the
  `SettingsService`)
- Test: `internal/http/handlers/api/v1/admin/handler_test.go` (or its settings test file)

**Interfaces:**
- The admin handler stops depending on `settingsStore { GetSetting; SetSetting }` and instead
  depends on a scoped interface satisfied by `*service.SettingsService`:

```go
type tenantSettings interface {
	GetTenant(ctx context.Context, scope domain.TenantScope, key string) (json.RawMessage, error)
	SetTenantProduct(ctx context.Context, scope domain.TenantScope, key string, value any) error
}
```

- [ ] **Step 1: Update the failing test first** — existing admin settings tests call the handlers
  without a tenant in context; add the default scope to the request context using the auth helper
  (`auth.WithTenant(ctx, &domain.Tenant{ID: 1, Status: domain.TenantActive})`) so
  `TenantScopeFromContext` returns `{1}`. Assert a product write is read back per-tenant. Run →
  FAIL (signature mismatch).

- [ ] **Step 2: Change the handler** — replace `h.settings.GetSetting(ctx, key)` /
  `SetSetting(ctx, key, val)` with scoped calls. Each settings method extracts scope at the top:

```go
scope, ok := auth.TenantScopeFromContext(r.Context())
if !ok {
	writeError(w, http.StatusForbidden, "no active tenant")
	return
}
// reads:  h.settings.GetTenant(r.Context(), scope, key)
// writes: h.settings.SetTenantProduct(r.Context(), scope, key, val)
```

  Apply to `HandleGetAccessSettings`, `HandleUpdateAccessSettings`, `HandleGetGeneralSettings`,
  `HandleUpdateGeneralSettings`, `HandleGetFeedbackSettings`, `HandleUpdateFeedbackSettings`, and
  the `settingString/settingBool/settingInt` helpers (give them a `scope` param). A
  `SetTenantProduct` returning `ErrEntitlementNamespace` maps to 403 — but the admin handler only
  writes known product keys, so this is defense-in-depth.

- [ ] **Step 3: Update `New`** — change `apiadmin.New(users, settings, auth, grantsCache)` to take
  the `tenantSettings` service instead of the raw `settings` store; update `server.go`
  accordingly (the `SettingsService` is built in Task 16's wiring, but you can construct it inline
  here and finalize wiring in Task 16 — keep `go build` green).

- [ ] **Step 4: Run** `go test ./internal/http/handlers/api/v1/admin ./internal/http/...` → PASS.
- [ ] **Step 5: Stage** (message: `feat(admin): read/write product settings from tenant_settings`).

---

### Task 9: Repoint config, health-check-in config, and new-user policy reads

**Files:**
- Modify: `internal/http/handlers/api/v1/config/handler.go` (scope-threaded reads)
- Modify: `internal/service/healthcheckin.go` (`LoadHealthCheckInConfig` takes scope + a scoped reader)
- Modify: `internal/http/handlers/api/v1/healthcheckin/handler.go` (pass scope)
- Modify: `internal/auth/manager.go` (`applyNewUserPolicy` reads tenant_settings of the
  registration tenant), `internal/auth/policy.go` (`DefaultNodeIDFromSettings`)
- Modify: `internal/http/server.go` (construct config/hc handlers with the scoped reader)
- Test: config handler test, `internal/service/healthcheckin_test.go`, `internal/auth/manager_test.go`

**Interfaces:**
- New scoped reader interface (satisfied by `*service.SettingsService`):

```go
type tenantSettingsReader interface {
	GetTenant(ctx context.Context, scope domain.TenantScope, key string) (json.RawMessage, error)
}
```

- `service.LoadHealthCheckInConfig(ctx, scope domain.TenantScope, sr ScopedSettingsReader) (HealthCheckInConfig, error)`
  where `ScopedSettingsReader.GetTenant(ctx, scope, key)`.

- [ ] **Step 1: Update the config handler test** — inject tenant #1 into the request context;
  seed a `tenant_settings` doc URL; assert `/api/v1/config` returns it. Run → FAIL.

- [ ] **Step 2: Thread scope through the config handler** — `Handler` depends on
  `tenantSettingsReader`; `HandleConfig` extracts scope (403 if absent) and passes it to every
  `settingString/...` helper and to `LoadHealthCheckInConfig`.

- [ ] **Step 3: Change `LoadHealthCheckInConfig`** to `(ctx, scope, sr)` reading
  `sr.GetTenant(ctx, scope, "health_checkin_config")`; update the `SettingsReader` interface in
  `healthcheckin.go` to the scoped shape. Update `GetHealthCheckIn` and the hc handler/loader
  closure (they already carry `scope` from Plan 2c) to pass scope into `LoadHealthCheckInConfig`.

- [ ] **Step 4: Repoint `applyNewUserPolicy` and `DefaultNodeIDFromSettings`** — read
  `new_user_policy` / `default_hierarchy_node_id` from `tenant_settings` for the **registration
  tenant**. In Phase 0 that is tenant #1 (the existing hardcoded scope; OSS
  `default_registration_tenant_id = 1`). Inject a scoped reader into `auth.Manager` and replace
  `m.store.GetSetting(ctx, "new_user_policy")` with `reader.GetTenant(ctx, domain.TenantScope{TenantID: 1}, "new_user_policy")`.
  Keep the existing `// TODO(tenancy): … default tenant` and extend it: Plan 4 resolves the real
  registration tenant from `default_registration_tenant_id`. `DefaultNodeIDFromSettings` takes a
  `scope` param (callers pass `{TenantID:1}` for now).

- [ ] **Step 5: Run the full suite** `go build ./... && go vet ./... && go test ./...` → PASS.
  This is the consistency gate for the key move (Tasks 7–9): any remaining `system_settings` read
  of a moved key surfaces here as a behavior regression in an existing test.

- [ ] **Step 6: Stage** (message: `feat(settings): repoint config/health-checkin/new-user reads to tenant_settings`).

---

# Part B — Provisioning & system-admin plane

> **Review checkpoint:** Part A is independently shippable (settings tier + key move + Entitlements
> seam, suite green). Recommended to review/commit Part A before starting Part B.

### Task 10: Load `is_system_admin` from the users repository

**Files:**
- Modify: `internal/store/users/users.go` (every `SELECT` list + `scanUser` + `SetSystemAdmin`)
- Test: `internal/store/users/users_test.go`

**Interfaces:**
- Produces: `domain.User.IsSystemAdmin` is populated on every load;
  `(*UserRepository) SetSystemAdmin(ctx, userID int64, v bool) error`.

- [ ] **Step 1: Write the failing test** — upsert a user, `SetSystemAdmin(ctx, id, true)`, reload
  via `GetUser`, assert `IsSystemAdmin == true`. Run → FAIL (column never selected).

- [ ] **Step 2: Add `is_system_admin` to every SELECT column list** in `users.go` (there are
  ~7 identical lists ending `… is_admin, created_at, updated_at, last_login_at`) — insert
  `is_system_admin` right after `is_admin`. Update `scanUser` to scan `&u.IsSystemAdmin` in the
  matching position. Add `RETURNING … is_system_admin …` to `UpsertUser` too.

- [ ] **Step 3: Add `SetSystemAdmin`**

```go
func (r *UserRepository) SetSystemAdmin(ctx context.Context, userID int64, v bool) error {
	_, err := r.db.Exec(ctx, `UPDATE users SET is_system_admin = $1, updated_at = NOW() WHERE id = $2`, v, userID)
	return err
}
```

- [ ] **Step 4: Run** `go test ./internal/store/users ./internal/auth/...` → PASS (watch for any
  `scanUser` callers / column-count mismatches).
- [ ] **Step 5: Stage** (message: `fix(store): load is_system_admin on user reads`).

---

### Task 11: System-admin gate — middleware, provisioning token, bootstrap

**Files:**
- Modify: `internal/auth/config.go` (`ProvisioningToken string`, `BootstrapSystemAdmin string`)
- Modify: `internal/auth/middleware.go` (`RequireSystemAdminMiddleware`)
- Modify: `internal/auth/manager.go` (`Login` promotes the bootstrap identity)
- Modify: `cmd/server/main.go` (read `PROVISIONING_TOKEN`, `BOOTSTRAP_SYSTEM_ADMIN` env)
- Test: `internal/auth/middleware_test.go`, `internal/auth/manager_test.go`

**Interfaces:**
- `func RequireSystemAdminMiddleware(provisioningToken string) func(http.Handler) http.Handler` —
  allows the request when the session user has `IsSystemAdmin`, **or** when
  `provisioningToken != ""` and the request carries `Authorization: Bearer <provisioningToken>`.
  Else 403 (JSON for API requests, `403 Forbidden` otherwise).
- Bootstrap: in `Manager.Login`, after upsert, if `cfg.BootstrapSystemAdmin != ""` and matches the
  identity (`provider:subject` **or** verified email) and no system-admin exists yet, call
  `SetSystemAdmin(ctx, user.ID, true)`.

- [ ] **Step 1: Write failing middleware test** — three cases: (a) system-admin user → 200;
  (b) non-admin user, no token → 403; (c) no user but `Authorization: Bearer secret` with
  `provisioningToken="secret"` → 200. Use `auth.WithUser(ctx, &domain.User{IsSystemAdmin: …})`.

- [ ] **Step 2: Implement `RequireSystemAdminMiddleware`** per the interface above (constant-time
  token compare via `subtle.ConstantTimeCompare`).

- [ ] **Step 3: Write failing bootstrap test** — `Manager.Login` with
  `cfg.BootstrapSystemAdmin = "github:42"` for a fresh user promotes it; a second login of another
  identity does not (a system-admin already exists). Needs a `hasAnySystemAdmin` check — add
  `(*UserRepository) AnySystemAdmin(ctx) (bool, error)` (`SELECT EXISTS(SELECT 1 FROM users WHERE is_system_admin)`).

- [ ] **Step 4: Implement bootstrap promotion** in `Login` + `AnySystemAdmin` in the users repo.

- [ ] **Step 5: Read env in `main.go`** — `cfg.ProvisioningToken = getEnv("PROVISIONING_TOKEN", "")`,
  `cfg.BootstrapSystemAdmin = os.Getenv("BOOTSTRAP_SYSTEM_ADMIN")`.

- [ ] **Step 6: Run** `go test ./internal/auth ./internal/store/users` → PASS.
- [ ] **Step 7: Stage** (message: `feat(auth): system-admin gate, provisioning token, bootstrap admin`).

---

### Task 12: Tenant status transitions in the repository

**Files:**
- Modify: `internal/store/tenants/tenants.go` (`SetStatus`)
- Test: `internal/store/tenants/tenants_test.go`

**Interfaces:**
- Produces: `(*TenantRepository) SetStatus(ctx, id int64, status domain.TenantStatus) error`
  (returns `ErrNotFound` on 0 rows).

- [ ] **Step 1: Write the failing test** — create a tenant, `SetStatus(..., TenantSuspended)`,
  `GetByID` → `Status == suspended`. Run → FAIL.

- [ ] **Step 2: Implement**

```go
func (r *TenantRepository) SetStatus(ctx context.Context, id int64, status domain.TenantStatus) error {
	ct, err := r.db.Exec(ctx, `UPDATE tenants SET status = $2 WHERE id = $1`, id, status)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
```

- [ ] **Step 3: Run** `go test ./internal/store/tenants` → PASS.
- [ ] **Step 4: Stage** (message: `feat(store): tenant status transitions`).

---

### Task 13: Provisioning service

**Files:**
- Create: `internal/service/provisioning.go`
- Test: `internal/service/provisioning_test.go`

**Interfaces:**
- Consumes: `*tenants.TenantRepository` + `*tenants.TenantCache`, `*memberships.MembershipRepository`
  + `*memberships.MembershipCache`, `*service.SettingsService`.
- Produces `*ProvisioningService` with:
  - `CreateTenant(ctx, name, slug string) (*domain.Tenant, error)` — validates slug
    (`domain.ValidTenantSlug`), creates, returns; maps `tenants.ErrSlugTaken`/`ErrInvalidSlug`.
  - `AttachMember(ctx, tenantID, userID int64, role domain.Role) (*domain.Membership, error)` —
    direct membership upsert (`status=active`) + `membershipCache.InvalidateUser(userID)`.
  - `SetEntitlements(ctx, tenantID int64, entitlements map[string]any) error` — writes each key
    (prefixed `entitlement.` if not already) via `settings.SetTenantEntitlement(scope, …)`;
    invalidation handled inside `SettingsService`.
  - `Suspend(ctx, tenantID int64) error` / `Restore(ctx, tenantID int64) error` —
    `TenantRepository.SetStatus` + `tenantCache.Invalidate(tenantID)`.

> Direct membership only (attach an existing global user). Email→invitation creation is Plan 4
> (onboarding). `AttachMember` resolving the bootstrap caller's identity vs. an arbitrary user is a
> Plan-4 concern; here we take an explicit `userID`.

- [ ] **Step 1: Write the failing test** (testcontainers) — `CreateTenant` then `AttachMember`
  (anonymous-local user id 1) as admin; assert `Memberships.Get` returns role=admin/active.
  `SetEntitlements({"sso": true})` → `SettingsService.GetTenant(scope, "entitlement.sso")` non-nil.
  `Suspend` → `Tenants.GetByID` status suspended. Run → FAIL.

- [ ] **Step 2: Implement `ProvisioningService`** per the interfaces (invalidate the relevant cache
  after each write — this resolves the Plan 2c `TODO(tenancy): invalidate on membership writes`).

- [ ] **Step 3: Run** `go test ./internal/service -run TestProvisioning` → PASS.
- [ ] **Step 4: Stage** (message: `feat(service): tenant provisioning service`).

---

### Task 14: Provisioning HTTP API `/api/v1/system/*`

**Files:**
- Create: `internal/http/handlers/api/v1/system/handler.go`
- Modify: `internal/http/server.go` (mount under `RequireSystemAdminMiddleware`)
- Test: `internal/http/handlers/api/v1/system/handler_test.go`

**Interfaces (endpoints):**
- `POST /api/v1/system/tenants` `{name, slug, entitlements?}` → 201 `{id, slug, name, status}`.
- `GET  /api/v1/system/tenants` → list.
- `POST /api/v1/system/tenants/{id}/members` `{user_id, role}` → 201 (direct membership).
- `PUT  /api/v1/system/tenants/{id}/entitlements` `{ "sso": true, "max_users": 50 }` → 204.
- `POST /api/v1/system/tenants/{id}/suspend` / `POST /api/v1/system/tenants/{id}/restore` → 204.
- `GET  /api/v1/system/users` → global user list (cross-tenant; reuses `Users.ListUsers`).
- `PUT  /api/v1/system/settings/default-registration-tenant` `{tenant_id|null}` → 204
  (writes the global `default_registration_tenant_id` via `SettingsService.SystemSet`).

> The provisioning endpoints take `tenant_id` from the URL and an explicit `tenant_id` scope —
> they are system-credentialed and operate across tenants, so they do NOT read tenant from
> context. They are the one legitimate cross-tenant surface; the gate
> (`RequireSystemAdminMiddleware`) is the boundary.

- [ ] **Step 1: Write failing handler tests** — (a) create tenant returns 201 + slug; (b) attach
  member then verify membership; (c) PUT entitlements then read back via settings; (d) suspend.
  Drive through an `httptest` router with `RequireSystemAdminMiddleware` and a system-admin user
  in context (and a parallel case asserting a non-system-admin gets 403). Use the existing
  `api/v1/testutil` harness.

- [ ] **Step 2: Implement the handlers** delegating to `ProvisioningService` /
  `SettingsService` / `Users`. Validate slug (422 on `ErrInvalidSlug`), 409 on `ErrSlugTaken`.

- [ ] **Step 3: Mount in `server.go`** — a new group outside the membership-gated group (system
  admin is tenant-less), gated by `auth.RequireSystemAdminMiddleware(s.auth.Config().ProvisioningToken)`:

```go
r.Group(func(r chi.Router) {
	if !s.auth.Disabled() {
		r.Use(auth.RequireSystemAdminMiddleware(s.auth.Config().ProvisioningToken))
	}
	r.Use(csrf.Handler) // session-based callers; token callers send no cookie
	sysH := apisystem.New(s.provisioning, s.settingsSvc, s.store.Users, s.store.Tenants)
	r.Post("/api/v1/system/tenants", sysH.HandleCreateTenant)
	// … remaining routes …
})
```

  > Confirm `auth.Manager` exposes `Config()` (it is used in `ScopeMiddleware` wiring as
  > `mgr.Config()`); reuse it. CSRF on token-only callers: provisioning-token callers are exempt
  > because they carry no session cookie — verify the CSRF handler skips non-cookie requests, else
  > mount these without `csrf.Handler` (machine API). Decide during Step 3 by reading `csrf.Handler`.

- [ ] **Step 4: Run** `go test ./internal/http/...` → PASS.
- [ ] **Step 5: Stage** (message: `feat(system): provisioning API under system-admin gate`).

---

### Task 15: Gate `/admin` on the active tenant-admin role

**Files:**
- Modify: `internal/auth/middleware.go` (`RequireTenantAdminMiddleware`)
- Modify: `internal/http/server.go` (`registerAdminRoutes` uses the new middleware)
- Test: `internal/auth/middleware_test.go`

**Interfaces:**
- Produces: `func RequireTenantAdminMiddleware(next http.Handler) http.Handler` — 403 unless
  `auth.ActiveRoleFromContext(ctx)` returns `(domain.RoleAdmin, true)`. (Active role is set by
  `TenantResolveMiddleware` from Plan 2a.)

- [ ] **Step 1: Write the failing test** — request with active role `user` → 403; with `admin`
  → next called. Run → FAIL.

- [ ] **Step 2: Implement `RequireTenantAdminMiddleware`** (JSON 403 for API requests, `403
  Forbidden` otherwise — mirror `RequireAdminMiddleware`).

- [ ] **Step 3: Swap it into `registerAdminRoutes`** — replace `auth.RequireAdminMiddleware` with
  `auth.RequireTenantAdminMiddleware` for the `/admin` + `/api/v1/admin/*` group. Leave
  `RequireAdminMiddleware` in place only if still referenced elsewhere; otherwise remove it.

  > **Behaviour check:** in `AUTH_MODE=disabled`, the admin group is not gated (`if !s.auth.Disabled()`),
  > so anon-local still reaches `/admin`. In auth mode, the resolver sets the role from the user's
  > membership in the active tenant — a tenant-admin keeps access; a plain member now correctly
  > loses `/admin`. Verify existing admin handler tests inject an `admin` active role (extend the
  > test context helper if needed).

- [ ] **Step 4: Run** `go test ./internal/auth ./internal/http/...` → PASS. Fix any admin handler
  test that now needs `auth.WithActiveRole(ctx, domain.RoleAdmin)`.
- [ ] **Step 5: Stage** (message: `feat(admin): gate /admin on active tenant-admin role`).

---

### Task 16: Wire caches + invalidation in `server.go`; `/system` shell; seed & specs

**Files:**
- Modify: `internal/http/server.go` (build settings caches + `SettingsService` + `ProvisioningService`;
  inject into handlers; register `/system` shell route)
- Modify: `cmd/server/main.go` if construction lives there (register the OSS `"unlimited"`
  entitlements impl via `entitlements.Register` at startup; select it by config name)
- Modify: `seed_demo.sql` (note below)
- Modify: `specs/040-api-contract.md`, `specs/050-permissions-and-lifecycle.md`,
  `specs/020-domain-model.md`
- Test: `go test ./...` (full suite, the integration gate)

- [ ] **Step 1: Build and inject the singletons in `server.go`** — once, near the other caches
  (`grantsCache`, `tenantCache`, `membershipCache` from Plan 2c):

```go
tenantSettingsCache := tenantsettings.NewTenantSettingsCache(s.store.TenantSettings)
systemSettingsCache := settings.NewSystemSettingsCache(s.store.Settings)
s.settingsSvc = service.NewSettingsService(tenantSettingsCache, s.store.TenantSettings, systemSettingsCache, s.store.Settings)
s.provisioning = service.NewProvisioningService(
	s.store.Tenants, tenantCache,
	s.store.Memberships, membershipCache,
	s.settingsSvc,
)
```

  Replace the `s.store.Settings` passed to `apiadmin.New`, `apiconfig.New`, and the
  health-check-in handlers with the scoped `s.settingsSvc` (from Tasks 8–9).

- [ ] **Step 2: Register the `/system` shell route** — a thin shell template under the
  system-admin gate (the React/SPA UI is powered by the Task 14 APIs):

```go
r.Get("/system", func(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.tmpl.ExecuteTemplate(w, "system-shell", nil)
})
```

  Add a minimal `system-shell` template (mirror `admin-shell`) listing tenants + a create form +
  a "default registration tenant" selector wired to the Task 14 endpoints. Keep it minimal —
  rich UI is iterative; the APIs + a functional shell satisfy the spec's `/system` plane.

- [ ] **Step 3: Register the OSS entitlements impl** — at startup
  `entitlements.Register("unlimited", func() entitlements.Entitlements { return entitlements.UnlimitedEntitlements{} })`
  and select it (config default `"unlimited"`). No gating endpoints consume it yet in Phase 0
  (premium gates arrive with SaaS), so just make the seam live and covered.

- [ ] **Step 4: `seed_demo.sql`** — no product-setting rows are seeded today (keys are written via
  the admin UI), so no data move is needed. Add a comment documenting that per-tenant product
  settings now live in `tenant_settings (tenant_id, key, value_json)` and that the demo tenant is
  #1, so any future seeded product key must target `tenant_settings`, not `system_settings`.

- [ ] **Step 5: Update specs (same change set, no unrelated edits):**
  - `specs/040-api-contract.md`: add the `/api/v1/system/*` provisioning endpoints and note the
    `/api/v1/admin/settings/*` endpoints now operate on `tenant_settings` (per active tenant).
  - `specs/050-permissions-and-lifecycle.md`: document the three admin planes (system-admin via
    `is_system_admin` or provisioning token; tenant-admin via active `membership.role=admin`;
    user), and the per-namespace write-authority (`entitlement.*` = system only).
  - `specs/020-domain-model.md`: confirm the settings tiers (`system_settings` global /
    `tenant_settings` per-tenant incl. `entitlement.*` / `user_settings`) and the `Entitlements`
    seam are described; align if drifted.

- [ ] **Step 6: Run the FULL suite** `go build ./... && go vet ./... && go test ./...` → all
  packages PASS. This is the final integration gate.
- [ ] **Step 7: Stage** `git add internal/ cmd/ migrations/ seed_demo.sql specs/`
  (message: `feat(tenancy): wire settings caches, provisioning, and /system shell`).

---

## Self-Review Notes

- **Spec coverage:**
  - Three settings tiers (§"Настройки — три уровня"): Tasks 1–5 (repos + caches + service).
  - Snapshot hot-path optimization (§"Оптимизация горячего пути"): Tasks 2–3 (per-tenant + global
    snapshot caches), invalidate-on-write in `SettingsService` + `ProvisioningService`.
  - Entitlements seam, OSS unlimited, registry (§"Принятые решения" 3, §"OSS/SaaS"): Task 6.
  - Deferred product-key move + read repointing (Plan 1 leftover, §"Изменения существующих
    ограничений"): Tasks 7–9.
  - Per-namespace write-authority (§"tenant_settings"): Task 5 + enforced at admin (Task 8) and
    provisioning (Tasks 13–14).
  - System-admin plane + provisioning API (§"Плоскости администрирования", §"Provisioning API"):
    Tasks 10–14, 16.
  - Tenant-admin plane = active role gate (§"Плоскости администрирования" 2): Task 15.
  - Bootstrap system-admin + provisioning token (§"Bootstrap"): Task 11.
- **Out of scope here (Plan 4 / later):** invitation creation + claim, join-request + approve,
  no-membership pluggable handler, `default_registration_tenant_id` *consumption* in the new-user
  flow (this plan only adds the setting + its `/system` writer), per-tenant SSO, `SubdomainStrategy`,
  app façade + OSS/SaaS split (Plan 5).
- **Order dependency:** Tasks 7→8→9 are one unit (the suite is inconsistent between them — do not
  pause for review mid-unit). Part A (1–9) and Part B (10–16) are separate review batches.
- **Mismatch fixed:** Task 10 closes the `is_system_admin`-never-loaded gap that would otherwise
  make the entire system-admin plane silently inert.
- **Layering:** `Entitlements` takes `domain.TenantScope` (no context, no store import);
  services take explicit scope; only handlers + boundary middleware read context. Provisioning
  endpoints are the sanctioned cross-tenant surface, gated by the system-admin middleware.

## Execution recommendation

Inline, compiler-driven, one task at a time (subagents flap in this environment). Two natural
review/commit batches: **Part A** (Tasks 1–9, settings tier + key move + entitlements — green and
shippable on its own) and **Part B** (Tasks 10–16, provisioning + system-admin plane). After each
task: `go build ./... && go vet ./... && go test ./...` green, then `git add` + propose a commit
message (no AI attribution); the user commits.
