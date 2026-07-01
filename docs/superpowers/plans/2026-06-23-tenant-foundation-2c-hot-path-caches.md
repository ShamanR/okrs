# Tenant Foundation — Plan 2c: Hot-Path Caches

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans (or subagent-driven-development). Steps use checkbox (`- [ ]`) syntax.

**Goal:** Make the per-request tenant resolution and the health-check-in cache tenant-aware and fast: resolve tenant/membership from in-memory caches instead of hitting the DB every request, and key the health-check-in cache by tenant (today its loader is hardcoded to tenant #1, so health-check-in is broken for any other tenant).

**Architecture:** Two independent pieces. (1) The health-check-in cache keys by `(tenantID, periodID)`, its loader receives an explicit `TenantScope`, and the refresh loop iterates the active period of every tenant. (2) `TenantResolver` reads tenant-by-id and memberships-by-user through TTL caches that wrap the repos, with invalidation hooks for later write paths.

**Tech Stack:** Go, in-memory `sync.RWMutex` caches (same pattern as the existing `GrantsCache`), pgx, testcontainers.

## Global Constraints

- Source of truth: spec `docs/superpowers/specs/2026-06-21-tenant-foundation-design.md` (раздел «Кэши и изоляция»).
- Каждый кэш — `sync.RWMutex` + TTL + явная инвалидация, по образцу существующего `internal/store/grants.GrantsCache`.
- Контекст для тенанта читают только хендлеры; ниже — явный `domain.TenantScope` (правило слоёв из 2a/2b).
- **Коммиты — за пользователем.** Агент только `git add`; без упоминаний AI.
- Behaviour-preserving для single-tenant: дефолтный тенант #1 продолжает работать как сейчас.

---

### Task 1: Health-check-in cache keyed by tenant

**Problem:** `HealthCheckInCache` (`internal/service/healthcheckin_cache.go`) keys by `periodID` and its loader closure in `internal/http/server.go` is hardcoded to `domain.TenantScope{TenantID: 1}`. `Service.GetHealthCheckIn` (`internal/service/healthcheckin.go:389`) calls `s.hcCache.Get(ctx, periodID)` with no scope. So a health-check-in request for any tenant ≠ 1 loads tenant-1-scoped teams/goals/statuses (wrong / empty).

**Files:**
- Modify: `internal/service/healthcheckin_cache.go` (key by tenant+period, scope in loader/Get/reload)
- Modify: `internal/service/healthcheckin.go` (`GetHealthCheckIn` takes scope)
- Modify: `internal/http/handlers/api/v1/healthcheckin/handler.go` (extract scope, pass it)
- Modify: `internal/http/server.go` (loader closure uses passed scope; refresh loop iterates tenants)
- Test: `internal/service/healthcheckin_cache_test.go` (create if absent) + handler test scope injection

**Interfaces:**
- `periodLoader = func(ctx context.Context, scope domain.TenantScope, periodID int64) (*PeriodData, error)`
- `(*HealthCheckInCache) Get(ctx, scope domain.TenantScope, periodID int64) (*PeriodData, error)`
- `(*HealthCheckInCache) reload(ctx, scope, periodID)` — internal
- cache map key: `type hcKey struct { tenantID, periodID int64 }`
- `(*HealthCheckInCache) InvalidateAll()` — unchanged (clears the whole map)
- `StartRefreshLoop(ctx, interval, activePeriodsFn func(ctx) []HCActive)` where `HCActive struct { Scope domain.TenantScope; PeriodID int64 }`
- `(*Service) GetHealthCheckIn(ctx, scope domain.TenantScope, userUDID string, isAdmin bool, periodID int64, cfg HealthCheckInConfig) (*HealthCheckInResult, error)`

- [ ] **Step 1: Write the failing cache test**

```go
// internal/service/healthcheckin_cache_test.go
package service

import (
	"context"
	"testing"
	"time"

	"okrs/internal/domain"
)

func TestHealthCheckInCacheKeysByTenant(t *testing.T) {
	var calls []hcKeyProbe
	loader := func(_ context.Context, scope domain.TenantScope, periodID int64) (*PeriodData, error) {
		calls = append(calls, hcKeyProbe{scope.TenantID, periodID})
		return &PeriodData{PeriodID: periodID, CachedAt: time.Now()}, nil
	}
	c := NewHealthCheckInCache(loader, time.Minute, nil)
	ctx := context.Background()

	// Same periodID, different tenants → two distinct loads (no cross-tenant cache hit).
	if _, err := c.Get(ctx, domain.TenantScope{TenantID: 1}, 100); err != nil {
		t.Fatalf("get t1: %v", err)
	}
	if _, err := c.Get(ctx, domain.TenantScope{TenantID: 2}, 100); err != nil {
		t.Fatalf("get t2: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 loads for distinct tenants, got %d", len(calls))
	}
	// Repeat for t1 → cache hit, no new load.
	if _, err := c.Get(ctx, domain.TenantScope{TenantID: 1}, 100); err != nil {
		t.Fatalf("get t1 again: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("expected cache hit, got %d loads", len(calls))
	}
}

type hcKeyProbe struct{ tenantID, periodID int64 }
```

- [ ] **Step 2: Run to verify it fails** — `go test ./internal/service -run TestHealthCheckInCacheKeysByTenant` → FAIL (signature mismatch).

- [ ] **Step 3: Rework `healthcheckin_cache.go`** — change `periods map[int64]*PeriodData` to `map[hcKey]*PeriodData`; add `hcKey` + `HCActive` types; thread `scope` through `Get`/`reload`/`periodLoader`; `StartRefreshLoop` takes `activePeriodsFn func(ctx) []HCActive` and loops over the slice calling `reload(ctx, a.Scope, a.PeriodID)`.

- [ ] **Step 4: Update `GetHealthCheckIn`** (`healthcheckin.go:389`) to take `scope domain.TenantScope` as first business param and call `s.hcCache.Get(ctx, scope, periodID)`.

- [ ] **Step 5: Update the handler** (`api/v1/healthcheckin/handler.go`): extract `scope, ok := auth.TenantScopeFromContext(r.Context())` (403 if absent) and pass to `GetHealthCheckIn`. The cache-invalidation handler keeps calling `InvalidateAll()`.

- [ ] **Step 6: Update `server.go`** — the `hcLoader` closure takes `scope` and passes it to `GetPeriod`/`ListAllTeams`/`ListGoalsByTeamsPeriod`/`ListTeamPeriodStatuses` (replacing the hardcoded `TenantScope{TenantID:1}`). Replace the `StartRefreshLoop` `activePeriodFn` with `activePeriodsFn` that lists tenants (`s.store.Tenants.List(ctx)`) and, for each, finds the active period via `s.service.FindPeriodForDate(ctx, domain.TenantScope{TenantID: tn.ID}, now)`, returning `[]service.HCActive`.

- [ ] **Step 7: Run** `go build ./... && go test ./internal/service ./internal/http/...` → PASS. Fix any handler/service test call sites to pass scope.

- [ ] **Step 8: Stage** `git add internal/` (message: `feat(health): key health-check-in cache by tenant`).

---

### Task 2: TenantCache + MembershipCache for the resolve hot path

**Problem:** `TenantResolveMiddleware` runs `TenantResolver.Resolve` on every authenticated request, which calls `MembershipLookup.ListByUser` + `TenantLookup.GetByID` — two DB round-trips per request.

**Files:**
- Create: `internal/store/tenants/cache.go` (`TenantCache` wrapping `*TenantRepository`)
- Create: `internal/store/memberships/cache.go` (`MembershipCache` wrapping `*MembershipRepository`)
- Modify: `internal/http/server.go` (wrap repos in caches, inject into `NewTenantResolver`)
- Test: `internal/store/tenants/cache_test.go`, `internal/store/memberships/cache_test.go`

**Interfaces:**
- `TenantCache` satisfies `auth.TenantLookup` (`GetByID(ctx, id) (*domain.Tenant, error)`) and adds `GetBySlug` + `Invalidate(id int64)` + `InvalidateAll()`. TTL default 5 min (mirror `grants.defaultGrantsCacheTTL`).
- `MembershipCache` satisfies `auth.MembershipLookup` (`ListByUser(ctx, userID) ([]domain.Membership, error)`) and the handler's `MembershipLookup`; adds `InvalidateUser(userID int64)` + `InvalidateAll()`.
- Both: `sync.RWMutex` + per-key `cachedAt`, lazy reload past TTL, atomic entry replacement (same shape as `grants.GrantsCache`).

- [ ] **Step 1: Write failing tests** — for each cache: first `GetByID`/`ListByUser` hits the backend; second within TTL does not; `Invalidate*` forces reload. Use a fake backend (interface with the repo method) counting calls, mirroring `grants_cache_test.go`.

```go
// sketch — internal/store/memberships/cache_test.go
func TestMembershipCacheCachesPerUser(t *testing.T) {
	backend := &fakeMembershipBackend{data: map[int64][]domain.Membership{
		7: {{UserID: 7, TenantID: 1, Role: domain.RoleAdmin}},
	}}
	c := newMembershipCacheWithBackend(backend, time.Minute)
	ctx := context.Background()
	if _, err := c.ListByUser(ctx, 7); err != nil { t.Fatal(err) }
	if _, err := c.ListByUser(ctx, 7); err != nil { t.Fatal(err) }
	if backend.calls != 1 { t.Fatalf("expected 1 backend call, got %d", backend.calls) }
	c.InvalidateUser(7)
	if _, err := c.ListByUser(ctx, 7); err != nil { t.Fatal(err) }
	if backend.calls != 2 { t.Fatalf("expected reload after invalidate, got %d", backend.calls) }
}
```

- [ ] **Step 2: Run to verify fail.**

- [ ] **Step 3: Implement `TenantCache` and `MembershipCache`** following the `grants.GrantsCache` pattern (backend interface for testability; `NewXCache(repo)` for production, `newXCacheWithBackend(b, ttl)` for tests). `MembershipCache.ListByUser` caches the repo's already-active-only list; `TenantCache.GetByID`/`GetBySlug` cache per id/slug.

- [ ] **Step 4: Wire into `server.go`** — build `tenantCache := tenants.NewTenantCache(st.Tenants)` and `membershipCache := memberships.NewMembershipCache(st.Memberships)`, pass them to `auth.NewTenantResolver(tenantCache, membershipCache)`. (The switch-tenant handler `apitenants.New(...)` may keep using the raw repos, or the membership cache — keep raw repos there for now; switching reads are rare.)

- [ ] **Step 5: Invalidation hooks (TTL-only for now)** — document that membership/tenant write paths (provisioning, invitation-claim, join-request approve — Plans 3/4) must call `InvalidateUser`/`Invalidate`. For 2c the 5-minute TTL bounds staleness; add a `// TODO(tenancy): invalidate on membership writes (Plan 3/4)` at the cache wiring.

- [ ] **Step 6: Run** `go build ./... && go vet ./... && go test ./...` → PASS.

- [ ] **Step 7: Stage** (message: `feat(store): cache tenant and membership lookups on the resolve hot path`).

---

## Self-Review Notes

- **Spec coverage:** health-check-in cache per-tenant + refresh loop per tenant (Task 1); tenant/membership resolve caches (Task 2). Grants cache was already made tenant-keyed in Plan 2b (Task 7) — no work here.
- **Correctness vs optimization:** Task 1 is also a **bug fix** (health-check-in currently only works for tenant #1). Task 2 is pure optimization.
- **Invalidation gap:** Task 2 ships TTL-only; explicit invalidation on membership/tenant writes is wired when those write paths are built (Plans 3/4). Documented, not silent.
- **Cross-instance caches:** all caches are per-process; multi-instance invalidation (e.g. pub/sub) is out of scope and noted for the SaaS scale phase.
