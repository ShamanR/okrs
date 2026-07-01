# Tenant Foundation — Plan 5: App Façade & OSS/SaaS Split

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans (inline,
> compiler-driven — subagents flap in this environment). Steps use checkbox (`- [ ]`) syntax.
> Build/vet/test green after every task; commits are the user's (agent does `git add` +
> proposes a message, no AI attribution).

**Goal:** Introduce the public `okrs/app` façade and parameterize the HTTP server so the box
can be assembled from a `Config` plus injectable seams (tenant-resolve strategies, an
`Entitlements` implementation, the no-membership page, and per-tier route mounts) — letting a
private `okrs-saas` module import the box as a Go module, register SaaS implementations, and
mount its control-plane routes in the same process, while OSS `main` runs on defaults.

**Topology (decided): one binary, shared session.** `okrs-saas` imports `okrs/app`, builds one
process, and mounts its control-plane routes (self-service signup / "create organization",
billing UI, Stripe webhooks) into the box's chi router via `app.New`. Mounting in-process lets
those routes reuse the box's middleware chain — `auth.UserFromContext`, the active-tenant scope,
CSRF — so the control-plane and the OKR tracker are one logged-in site. SaaS billing/Stripe data
lives in SaaS's own DB connection (not the box schema); it reflects entitlement changes into the
box via the provisioning service. The box never gains SaaS-specific schema or secrets.

**Architecture:** Three moves. (1) Turn the single hardcoded `TenantResolver` into an ordered
list of `ResolveStrategy` with a registry (`auth.RegisterResolveStrategy`), `SessionStrategy`
as the OSS default. (2) Give `internal/http.NewServer` an `Options` struct (resolver,
entitlements, no-membership name, and **three route-mount hooks** — public, authed, tenant — one
per existing middleware tier) instead of building everything inline. (3) Add a public `okrs/app`
package that assembles store → caches → auth → server from a `Config`, moving the wiring out of
`cmd/server/main.go`; `main` becomes a thin OSS entrypoint. Registries mirror the existing
`auth.Register` / `entitlements.Register` / `onboarding.Register` pattern.

**Tech Stack:** Go, chi, pgx/v5, testcontainers-go.

---

## Spec ↔ code mismatch notes (read before implementing)

1. **No public façade.** `cmd/server/main.go` directly wires `store.New`, `grants.NewGrantsCache`,
   `auth.NewManager`, `httpserver.NewServer`. Go forbids importing another module's `internal/`,
   so `okrs-saas` cannot reuse any of this. Spec §"Ограничение Go" requires a public `okrs/app`.
   → Tasks 3–4.
2. **`TenantResolver` is a single hardcoded strategy.** `auth.NewTenantResolver(t, m)` builds one
   session-based resolver; spec §"Жизненный цикл запроса" wants an ordered strategy list with a
   registry so `SubdomainStrategy` can be added later without touching core. → Task 1.
3. **`Entitlements` is registered but never consumed.** `main.go` calls
   `entitlements.Register("unlimited", …)` but nothing selects/holds an instance. The façade
   should select it by config name and inject it into the server (available for premium gates).
   → Tasks 2–3.
4. **No-membership handler name is a hardcoded const** (`noMembershipHandlerName = "stub"` in
   `server.go`). It should come from `Options` so SaaS can select its own page. → Task 2.

Addressed in this change set. Specs `010` (+ a short OSS/SaaS seam note) are updated in Task 5;
no unrelated specs touched.

---

## Global Constraints

- **`internal/` stays internal; only `okrs/app` is exported.** The façade is the sole public
  surface; `app` is a thin assembler over `internal/*`.
- **Registries mirror the existing pattern** (`auth.Register`, `entitlements.Register`,
  `onboarding.Register`): a package-level `map[string]Factory` guarded by a mutex, populated from
  `init()` or at startup, selected by config name. Unknown name → explicit error.
- **Behaviour-preserving for OSS:** the default assembly (session resolver, unlimited
  entitlements, stub no-membership) must produce byte-for-byte the same routes/behaviour as today.
- **Explicit `domain.TenantScope`** and the layering rules from Plans 2–4 are unchanged.
- **Migrations** are golang-migrate (last applied: 033). This plan adds no migration.
- **Commits are the user's.** Agent stages (`git add`) and proposes a message; **no AI/Claude
  attribution** anywhere (CLAUDE.md).
- Keep `seed_demo.sql` and the demo seed in sync if structure changes (it does not here).

---

### Task 1: `ResolveStrategy` seam + registry

**Files:**
- Create: `internal/auth/resolve_strategy.go`
- Modify: `internal/auth/tenant_resolver.go` (`TenantResolver` becomes a strategy list)
- Modify: `internal/http/server.go` (build resolver from the session strategy)
- Test: `internal/auth/resolve_strategy_test.go`

**Interfaces:**
- Consumes: `TenantLookup`, `MembershipLookup` (existing, `tenant_resolver.go`).
- Produces:
  - `type ResolveStrategy interface { Resolve(ctx context.Context, user *domain.User, sess *domain.AuthSession) (*domain.Tenant, domain.Role, bool, error) }`
    — the `bool` is "resolved"; `false` means "pass to the next strategy".
  - `type ResolveDeps struct { Tenants TenantLookup; Memberships MembershipLookup }`
  - `type ResolveStrategyFactory func(ResolveDeps) ResolveStrategy`
  - `func RegisterResolveStrategy(name string, f ResolveStrategyFactory)` / `func ResolveStrategyFactoryByName(name string) (ResolveStrategyFactory, bool)`
  - `func NewSessionStrategy(t TenantLookup, m MembershipLookup) ResolveStrategy`
  - `func NewTenantResolver(strategies ...ResolveStrategy) *TenantResolver`
  - `(*TenantResolver) Resolve(...)` unchanged signature `(*domain.Tenant, domain.Role, error)`.
  - `"session"` is registered by default in this file's `init()`.

- [ ] **Step 1: Write the failing strategy-ordering test**

```go
// internal/auth/resolve_strategy_test.go
package auth

import (
	"context"
	"testing"

	"okrs/internal/domain"
)

type fakeStrategy struct {
	tenant   *domain.Tenant
	role     domain.Role
	resolved bool
}

func (f fakeStrategy) Resolve(context.Context, *domain.User, *domain.AuthSession) (*domain.Tenant, domain.Role, bool, error) {
	return f.tenant, f.role, f.resolved, nil
}

func TestTenantResolverFirstResolvedWins(t *testing.T) {
	first := fakeStrategy{resolved: false}
	second := fakeStrategy{tenant: &domain.Tenant{ID: 9}, role: domain.RoleAdmin, resolved: true}
	r := NewTenantResolver(first, second)

	tn, role, err := r.Resolve(context.Background(), &domain.User{ID: 1}, nil)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if tn.ID != 9 || role != domain.RoleAdmin {
		t.Fatalf("expected the second strategy to win, got tenant=%v role=%v", tn, role)
	}
}

func TestTenantResolverNoneResolvedIsErrNoMembership(t *testing.T) {
	r := NewTenantResolver(fakeStrategy{resolved: false})
	if _, _, err := r.Resolve(context.Background(), &domain.User{ID: 1}, nil); err != ErrNoMembership {
		t.Fatalf("expected ErrNoMembership, got %v", err)
	}
}

func TestResolveStrategyRegistryHasSession(t *testing.T) {
	f, ok := ResolveStrategyFactoryByName("session")
	if !ok || f(ResolveDeps{}) == nil {
		t.Fatal("session strategy must be registered by default")
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `go test ./internal/auth -run TestTenantResolver` → FAIL
  (`NewTenantResolver` signature mismatch / `ResolveStrategy` undefined).

- [ ] **Step 3: Create the strategy + registry**

```go
// internal/auth/resolve_strategy.go
package auth

import (
	"context"
	"sync"

	"okrs/internal/domain"
)

// ResolveStrategy resolves the active tenant for a request. The bool reports whether this
// strategy handled it; false means "let the next strategy try".
type ResolveStrategy interface {
	Resolve(ctx context.Context, user *domain.User, sess *domain.AuthSession) (*domain.Tenant, domain.Role, bool, error)
}

// ResolveDeps are the lookups a strategy factory may need.
type ResolveDeps struct {
	Tenants     TenantLookup
	Memberships MembershipLookup
}

// ResolveStrategyFactory builds a strategy from shared deps.
type ResolveStrategyFactory func(ResolveDeps) ResolveStrategy

var (
	resolveRegistryMu sync.RWMutex
	resolveRegistry   = map[string]ResolveStrategyFactory{}
)

func RegisterResolveStrategy(name string, f ResolveStrategyFactory) {
	resolveRegistryMu.Lock()
	defer resolveRegistryMu.Unlock()
	resolveRegistry[name] = f
}

func ResolveStrategyFactoryByName(name string) (ResolveStrategyFactory, bool) {
	resolveRegistryMu.RLock()
	defer resolveRegistryMu.RUnlock()
	f, ok := resolveRegistry[name]
	return f, ok
}

func init() {
	RegisterResolveStrategy("session", func(d ResolveDeps) ResolveStrategy {
		return NewSessionStrategy(d.Tenants, d.Memberships)
	})
}

// SessionStrategy resolves from auth_sessions.active_tenant_id, falling back to the first
// active membership. This is the OSS default (works everywhere).
type SessionStrategy struct {
	tenants TenantLookup
	members MembershipLookup
}

func NewSessionStrategy(t TenantLookup, m MembershipLookup) ResolveStrategy {
	return &SessionStrategy{tenants: t, members: m}
}

func (s *SessionStrategy) Resolve(ctx context.Context, user *domain.User, sess *domain.AuthSession) (*domain.Tenant, domain.Role, bool, error) {
	memberships, err := s.members.ListByUser(ctx, user.ID)
	if err != nil {
		return nil, "", false, err
	}
	if len(memberships) == 0 {
		return nil, "", false, nil // not resolved → next strategy / ErrNoMembership
	}
	pick := memberships[0]
	if sess != nil && sess.ActiveTenantID != nil {
		for _, m := range memberships {
			if m.TenantID == *sess.ActiveTenantID {
				pick = m
				break
			}
		}
	}
	tn, err := s.tenants.GetByID(ctx, pick.TenantID)
	if err != nil {
		return nil, "", false, err
	}
	return tn, pick.Role, true, nil
}
```

- [ ] **Step 4: Reduce `tenant_resolver.go` to the strategy list**

Replace the struct/ctor/Resolve in `internal/auth/tenant_resolver.go` (keep the `ErrNoMembership`,
`ErrNotMember`, `TenantLookup`, `MembershipLookup` declarations):

```go
// TenantResolver runs an ordered list of strategies; the first that resolves wins.
type TenantResolver struct {
	strategies []ResolveStrategy
}

func NewTenantResolver(strategies ...ResolveStrategy) *TenantResolver {
	return &TenantResolver{strategies: strategies}
}

func (r *TenantResolver) Resolve(ctx context.Context, user *domain.User, sess *domain.AuthSession) (*domain.Tenant, domain.Role, error) {
	for _, st := range r.strategies {
		tn, role, ok, err := st.Resolve(ctx, user, sess)
		if err != nil {
			return nil, "", err
		}
		if ok {
			return tn, role, nil
		}
	}
	return nil, "", ErrNoMembership
}
```

- [ ] **Step 5: Update `server.go` to build the resolver from the session strategy**

In `NewServer`, replace
`tenantResolver: auth.NewTenantResolver(tenantCache, membershipCache)` (currently
`auth.NewTenantResolver(tenantCache, membershipCache)`) with:

```go
tenantResolver: auth.NewTenantResolver(auth.NewSessionStrategy(tenantCache, membershipCache)),
```

> Task 2 replaces this with an Options-supplied resolver; this keeps the build green meanwhile.

- [ ] **Step 6: Run** `go test ./internal/auth ./internal/http/...` → PASS. Existing resolver
  tests (`tenant_resolver_test.go`, if any) keep passing because `Resolve`'s signature is unchanged.
- [ ] **Step 7: Stage** `git add internal/auth/ internal/http/server.go`
  (message: `feat(auth): tenant-resolve strategy list + registry`).

---

### Task 2: Parameterize `NewServer` with an `Options` struct

**Files:**
- Modify: `internal/http/server.go` (`Options`, `NewServer` signature, `Routes` uses options)
- Test: build + Task 3's integration test (NewServer needs a real store; covered there).

**Interfaces:**
- Produces:
  - `type Options struct { Resolver *auth.TenantResolver; Entitlements entitlements.Entitlements; NoMembershipName string; PublicRoutes func(chi.Router); AuthedRoutes func(chi.Router); TenantRoutes func(chi.Router) }`
  - `func NewServer(st *store.Store, grantsCache *grants.GrantsCache, logger *slog.Logger, zone *time.Location, authMgr *auth.Manager, opts Options) (*Server, error)`
- Consumes: `auth.NewSessionStrategy` (Task 1), `entitlements.Entitlements` (Plan 3).

> **Three mount hooks, one per existing middleware tier** (Model B — one binary, shared session):
> - `PublicRoutes` → outer group (Session loaded, **no** auth gate, **no** CSRF) — Stripe/webhooks.
> - `AuthedRoutes` → authed-but-not-membership-gated group (RequireAuth + CSRF) — self-service
>   "create organization" (the new user has no membership yet).
> - `TenantRoutes` → membership-gated group (RequireAuth + TenantResolve + RequireMembership +
>   Scope + CSRF) — tenant-scoped billing UI.
> Each defaults to `nil` (no mounts) → OSS behaviour unchanged.

- [ ] **Step 1: Add the `Options` type and fields to `Server`**

```go
// in internal/http/server.go
type Options struct {
	// Resolver, if nil, defaults to a session-only resolver built from the tenant/membership caches.
	Resolver *auth.TenantResolver
	// Entitlements, if nil, defaults to entitlements.UnlimitedEntitlements{}.
	Entitlements entitlements.Entitlements
	// NoMembershipName selects the registered onboarding.NoMembershipHandler; "" → "stub".
	NoMembershipName string
	// Route-mount hooks for an embedded control-plane (SaaS). Each nil → no extra routes.
	PublicRoutes func(chi.Router) // outer: Session loaded, no auth gate, no CSRF (webhooks)
	AuthedRoutes func(chi.Router) // authed, not membership-gated (create-organization)
	TenantRoutes func(chi.Router) // membership-gated, tenant-scoped (billing UI)
}
```

Add to `Server`: `entitlements entitlements.Entitlements`, `noMembershipName string`,
`publicRoutes, authedRoutes, tenantRoutes func(chi.Router)`. (Import `okrs/internal/entitlements`.)

- [ ] **Step 2: Change `NewServer` to accept and apply `Options`**

Replace the inline resolver/defaults with:

```go
func NewServer(st *store.Store, grantsCache *grants.GrantsCache, logger *slog.Logger, zone *time.Location, authMgr *auth.Manager, opts Options) (*Server, error) {
	// ... existing tmpl/hcLoader/caches/settingsSvc/provisioning/onboardingSvc setup ...

	resolver := opts.Resolver
	if resolver == nil {
		resolver = auth.NewTenantResolver(auth.NewSessionStrategy(tenantCache, membershipCache))
	}
	ent := opts.Entitlements
	if ent == nil {
		ent = entitlements.UnlimitedEntitlements{}
	}
	noMembership := opts.NoMembershipName
	if noMembership == "" {
		noMembership = "stub"
	}

	return &Server{
		// ... existing fields ...
		tenantResolver:   resolver,
		tenantCache:      tenantCache,
		membershipCache:  membershipCache,
		settingsSvc:      settingsSvc,
		provisioning:     provisioning,
		onboarding:       onboardingSvc,
		entitlements:     ent,
		noMembershipName: noMembership,
		publicRoutes:     opts.PublicRoutes,
		authedRoutes:     opts.AuthedRoutes,
		tenantRoutes:     opts.TenantRoutes,
	}, nil
}
```

Remove the old `tenantResolver: auth.NewTenantResolver(...)` line from the struct literal (now set
via `resolver`).

- [ ] **Step 3: Replace the `noMembershipHandlerName` const usage with the field**

Delete the `const noMembershipHandlerName = "stub"` declaration. In the `/no-access` handler and
the stub registration in `Routes()`, use `s.noMembershipName`:

```go
// stub registration stays (the box ships the OSS default), keyed by its own name:
onboarding.Register("stub", onboarding.StubHandler{Render: func(w http.ResponseWriter, r *http.Request) {
	_ = s.tmpl.ExecuteTemplate(w, "no-membership", nil)
}})
// ...
h, ok := onboarding.Get(s.noMembershipName)
```

- [ ] **Step 4: Mount the three hooks in their groups in `Routes()`**

  - `PublicRoutes`: in the outer group, next to the auth/`/invite`/`/no-access` routes (no CSRF):
    ```go
    if s.publicRoutes != nil {
    	s.publicRoutes(r)
    }
    ```
  - `AuthedRoutes`: inside the existing join-request group (the one with `RequireAuthMiddleware`
    + `csrf.Handler`, not membership-gated), after the join-request route:
    ```go
    if s.authedRoutes != nil {
    	s.authedRoutes(r)
    }
    ```
  - `TenantRoutes`: inside the protected/membership-gated group, after `s.registerAdminRoutes(r, deps)`:
    ```go
    if s.tenantRoutes != nil {
    	s.tenantRoutes(r)
    }
    ```

- [ ] **Step 5: Update the existing `NewServer` caller** in `cmd/server/main.go` temporarily to
  pass a zero `Options{}` so the build stays green (Task 4 moves this into `app`):
  `httpserver.NewServer(pgstore, grantsCache, logger, zone, authMgr, httpserver.Options{})`.

- [ ] **Step 6: Run** `go build ./... && go vet ./...` → green. (`go test ./internal/http/...`
  still passes; behaviour is unchanged because all options default to today's values.)
- [ ] **Step 7: Stage** (message: `feat(http): parameterize server with Options (resolver, entitlements, no-membership, route mounts)`).

---

### Task 3: Public `okrs/app` façade

**Files:**
- Create: `app/app.go` (package `app`)
- Test: `app/app_test.go`

**Interfaces:**
- Produces:
  - `type Config struct { Pool *pgxpool.Pool; Logger *slog.Logger; Zone *time.Location; Auth auth.Config; EntitlementsName string; NoMembershipName string; ResolveStrategyNames []string; PublicRoutes func(chi.Router); AuthedRoutes func(chi.Router); TenantRoutes func(chi.Router) }`
  - `type App struct { Handler http.Handler }`
  - `func New(cfg Config) (*App, error)` — assembles store → caches → auth → server and returns
    the wired `http.Handler`.
- Consumes: `store.New`, `grants.NewGrantsCache`, `auth.NewManager`, `auth.NewTenantResolver`,
  `auth.ResolveStrategyFactoryByName`, `entitlements.Get`, `httpserver.NewServer` + `Options`.

> `app` takes an already-connected `*pgxpool.Pool` (the caller owns connection + migrations);
> this keeps the façade free of migration-path/env concerns and makes it testable with a
> testcontainers pool. OSS `main` connects + migrates, then calls `app.New`.

- [ ] **Step 1: Write the failing façade integration test** (testcontainers, mirrors
  `api/v1/testutil` setup; run migrations then build the app)

```go
// app/app_test.go
package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"okrs/app"
	"okrs/internal/auth"
	"okrs/internal/store/testutil"

	"github.com/go-chi/chi/v5"
)

func TestAppAssemblesAndServes(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()

	mounted := false
	a, err := app.New(app.Config{
		Pool:                 pool,
		Zone:                 time.UTC,
		Auth:                 auth.DefaultConfig(), // AUTH_MODE=disabled
		ResolveStrategyNames: []string{"session"},
		// Mount into the public tier so the test needs no auth/session plumbing.
		PublicRoutes: func(r chi.Router) {
			r.Get("/ext/ping", func(w http.ResponseWriter, _ *http.Request) { mounted = true; w.WriteHeader(http.StatusOK) })
		},
	})
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}

	// An injected control-plane route is reachable.
	rw := httptest.NewRecorder()
	a.Handler.ServeHTTP(rw, httptest.NewRequest(http.MethodGet, "/ext/ping", nil))
	if rw.Code != http.StatusOK || !mounted {
		t.Fatalf("mounted route not reachable: code=%d mounted=%v", rw.Code, mounted)
	}

	_ = context.Background()
}

func TestAppUnknownEntitlementsName(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	if _, err := app.New(app.Config{Pool: pool, Zone: time.UTC, Auth: auth.DefaultConfig(), EntitlementsName: "nope"}); err == nil {
		t.Fatal("unknown entitlements name must error")
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `go test ./app` → FAIL (package `app` absent).

- [ ] **Step 3: Implement `app/app.go`**

```go
// Package app is the public façade that assembles the OKR application from a Config plus
// injectable seams. internal/* packages stay internal; this is the only exported surface, so a
// private okrs-saas module can import the box and register SaaS implementations.
package app

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"okrs/internal/auth"
	"okrs/internal/entitlements"
	httpserver "okrs/internal/http"
	"okrs/internal/store"
	"okrs/internal/store/grants"
	"okrs/internal/store/memberships"
	"okrs/internal/store/tenants"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	Pool                 *pgxpool.Pool
	Logger               *slog.Logger
	Zone                 *time.Location
	Auth                 auth.Config
	EntitlementsName     string   // registry key; "" → "unlimited"
	NoMembershipName     string   // "" → "stub"
	ResolveStrategyNames []string // nil → ["session"]
	// Embedded control-plane route mounts (SaaS), one per middleware tier; each nil in OSS.
	PublicRoutes func(chi.Router)
	AuthedRoutes func(chi.Router)
	TenantRoutes func(chi.Router)
}

type App struct {
	Handler http.Handler
}

func New(cfg Config) (*App, error) {
	if cfg.Pool == nil {
		return nil, fmt.Errorf("app: Config.Pool is required")
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	}
	zone := cfg.Zone
	if zone == nil {
		zone = time.UTC
	}

	st := store.New(cfg.Pool)
	grantsCache := grants.NewGrantsCache(st.Grants)

	authMgr, err := auth.NewManager(cfg.Auth, st)
	if err != nil {
		return nil, fmt.Errorf("app: auth: %w", err)
	}

	// Resolve strategies (default: session).
	names := cfg.ResolveStrategyNames
	if len(names) == 0 {
		names = []string{"session"}
	}
	deps := auth.ResolveDeps{
		Tenants:     tenants.NewTenantCache(st.Tenants),
		Memberships: memberships.NewMembershipCache(st.Memberships),
	}
	var strategies []auth.ResolveStrategy
	for _, name := range names {
		f, ok := auth.ResolveStrategyFactoryByName(name)
		if !ok {
			return nil, fmt.Errorf("app: unknown resolve strategy %q", name)
		}
		strategies = append(strategies, f(deps))
	}

	// Entitlements implementation.
	entName := cfg.EntitlementsName
	if entName == "" {
		entName = "unlimited"
	}
	entFactory, ok := entitlements.Get(entName)
	if !ok {
		return nil, fmt.Errorf("app: unknown entitlements %q", entName)
	}

	srv, err := httpserver.NewServer(st, grantsCache, logger, zone, authMgr, httpserver.Options{
		Resolver:         auth.NewTenantResolver(strategies...),
		Entitlements:     entFactory(),
		NoMembershipName: cfg.NoMembershipName,
		PublicRoutes:     cfg.PublicRoutes,
		AuthedRoutes:     cfg.AuthedRoutes,
		TenantRoutes:     cfg.TenantRoutes,
	})
	if err != nil {
		return nil, err
	}
	return &App{Handler: srv.Routes()}, nil
}
```

> The resolver here builds its **own** tenant/membership caches via `ResolveDeps`. That matches
> today (the server also built caches for the resolver); the server still builds its own caches
> internally for provisioning invalidation. Acceptable for Phase 0 — note it; a later pass can
> share one cache set. Register `"unlimited"` must happen before `app.New` (Task 4 does it in
> `main`; the test relies on it — add `entitlements.Register("unlimited", …)` at the top of the
> test, or import a package that does. The test below registers it inline.)

- [ ] **Step 4: Make the test self-sufficient for the registry** — at the top of `app_test.go`,
  register the OSS unlimited entitlements in an `init()` (the registry is global, so this is
  enough):

```go
func init() {
	entitlements.Register("unlimited", func() entitlements.Entitlements { return entitlements.UnlimitedEntitlements{} })
}
```

  (Add the `okrs/internal/entitlements` import to the test.)

- [ ] **Step 5: Run** `go test ./app` → PASS.
- [ ] **Step 6: Stage** `git add app/` (message: `feat(app): public façade assembling the box from Config + seams`).

---

### Task 4: Thin `cmd/server/main.go` onto the façade

**Files:**
- Modify: `cmd/server/main.go` (delegate assembly to `app.New`; keep env parsing + migrations + seed)
- Test: `go build ./... && go vet ./...` + the existing full suite.

**Interfaces:**
- Consumes: `app.New`, `app.Config` (Task 3).

- [ ] **Step 1: Replace the manual wiring with `app.New`**

Keep `loadAuthConfig`, `runMigrations`, the pool connection, and the `-seed` block. Replace the
`grantsCache`/`auth.NewManager`/`httpserver.NewServer` block (and the now-stale `Options{}` call
from Task 2 Step 5) with:

```go
// OSS feature-gating default.
entitlements.Register("unlimited", func() entitlements.Entitlements { return entitlements.UnlimitedEntitlements{} })

a, err := app.New(app.Config{
	Pool:   pool,
	Logger: logger,
	Zone:   zone,
	Auth:   loadAuthConfig(),
	// OSS defaults: EntitlementsName "unlimited", NoMembershipName "stub",
	// ResolveStrategyNames ["session"], no extra routes.
})
if err != nil {
	logger.Error("failed to assemble app", slog.String("error", err.Error()))
	os.Exit(1)
}

addr := fmt.Sprintf(":%s", port)
logger.Info("listening", slog.String("addr", addr))
if err := http.ListenAndServe(addr, a.Handler); err != nil {
	logger.Error("server stopped", slog.String("error", err.Error()))
	os.Exit(1)
}
```

Remove now-unused imports (`httpserver`, `grants`, `auth` if no longer referenced — `loadAuthConfig`
still uses `auth`, so keep it; drop `httpserver` and `grants`). Add `okrs/app`.

- [ ] **Step 2: Run** `go build ./... && go vet ./...` → green.
- [ ] **Step 3: Run the FULL suite** `go test ./...` → all packages PASS (behaviour unchanged:
  OSS defaults reproduce today's assembly).
- [ ] **Step 4: Smoke-run the binary** (no DB needed to fail-fast on wiring): `go run ./cmd/server -h`
  compiles and prints flag usage; or build the binary: `go build -o /tmp/okrs-server ./cmd/server`
  → exits 0.
- [ ] **Step 5: Stage** (message: `refactor(cmd): assemble server via the app façade`).

---

### Task 5: Specs & docs

**Files:**
- Modify: `specs/010-architecture-constraints.md` (add the `app` façade layer + OSS/SaaS seam note)
- Modify: `README.md` (one line: entrypoint is `okrs/app`, `cmd/server` is the OSS main) — only if
  README documents architecture; otherwise skip.

- [ ] **Step 1: Add the façade to the layer list in `010`** (after the `internal/http` bullets):

```markdown
- `app` (public, **корень модуля**) — фасад: `app.New(Config) (*App, error)` собирает приложение
  из `Config` + инжектируемых seam'ов (resolve-стратегии по имени из `auth.RegisterResolveStrategy`,
  `Entitlements` по имени из `entitlements.Register`, no-membership-страница по имени из
  `onboarding.Register`, mount'ы control-plane роутов через `PublicRoutes`/`AuthedRoutes`/`TenantRoutes`
  — по одному на middleware-уровень). Единственный публичный пакет; всё остальное — `internal/`.
  `cmd/server` — тонкий OSS-entrypoint поверх `app`.
```

- [ ] **Step 2: Add a short "OSS / SaaS split" subsection to `010`** documenting the registry
  seams and the three-repo layout (verbatim from the design spec §"OSS / SaaS разделение"):

```markdown
## OSS / SaaS split

Коробка (`okrs`, public) самодостаточна и мультитенантна. Расширяется через registry-seam'ы,
выбираемые по имени в `app.Config`:
- `auth.RegisterResolveStrategy(name, factory)` — стратегии резолва тенанта (OSS: `session`;
  премиум: `subdomain`);
- `entitlements.Register(name, factory)` — реализация `Entitlements` (OSS: `unlimited`);
- `onboarding.Register(name, handler)` — no-membership-страница (OSS: `stub`);
- `auth.Register(name, factory)` — OAuth-провайдеры (blank-import).

Приватный `okrs-saas` импортирует `okrs/app`, blank-import'ит пакеты с SaaS-регистрациями,
выбирает их по имени в `Config` и монтирует control-plane роуты в один процесс через
`PublicRoutes`/`AuthedRoutes`/`TenantRoutes` (по одному на middleware-уровень: вебхуки без auth,
self-service создание орг под auth-без-membership, биллинг-UI под membership-гейтом) — общая
сессия с трекером. Биллинг/Stripe-данные — в собственной БД `okrs-saas`, не в схеме коробки;
результат отражается в коробку через provisioning. Схема коробки не содержит SaaS-понятий;
секреты — в env.
```

- [ ] **Step 3: Run** `go build ./...` (docs-only change; sanity) → green.
- [ ] **Step 4: Stage** `git add specs/010-architecture-constraints.md README.md`
  (message: `docs(specs): document the app façade and OSS/SaaS registry seams`).

---

## Self-Review Notes

- **Spec coverage:**
  - Public `okrs/app` façade + `internal/` boundary (§"Ограничение Go", Порядок 9): Tasks 3–4.
  - Parameterized server (`Options`): Task 2.
  - Resolve-strategy registry + `SessionStrategy` default, `SubdomainStrategy` addable later
    (§"Жизненный цикл запроса", §"OSS/SaaS" registry): Task 1.
  - Entitlements selected by name and injected (§"Enforcement фич"): Tasks 2–3.
  - No-membership handler selectable by name (§"No-membership-страница — pluggable seam"): Task 2.
  - Registry seams documented (§"OSS/SaaS разделение"): Task 5.
- **Out of scope (later / other repos):** the actual `okrs-saas` and `okrs-landing` repos;
  `SubdomainStrategy` implementation (Phase 1 — only the seam ships here); billing `Entitlements`;
  graceful shutdown / server timeouts (orthogonal hardening).
- **Behaviour-preserving:** every `Options` field defaults to today's value; the full-suite run in
  Task 4 is the regression gate.
- **Known minor duplication:** the resolver builds its own tenant/membership caches in `app`,
  while the server builds another set internally for provisioning invalidation. Noted in Task 3;
  unifying them is a non-blocking follow-up, not a Phase-0 requirement.

## Execution recommendation

Inline, compiler-driven, one task at a time. Single review batch (it's a contained refactor).
After each task: `go build ./... && go vet ./... && go test ./...` green, then `git add` +
propose a commit message (no AI attribution); the user commits.
