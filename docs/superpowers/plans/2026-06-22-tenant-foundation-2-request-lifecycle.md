# Tenant Foundation — Plan 2: Request Lifecycle & Scoping

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Резолвить активный тенант на каждый запрос, гейтить доступ по membership, и протянуть `TenantScope` в repository-слой так, чтобы переключение тенанта реально меняло видимые данные.

**Architecture:** Plan 2 делится на три исполнимых под-плана, каждый — рабочее тестируемое ПО:
- **2a (детализирован ниже)** — `domain.TenantScope`, tenant/role в request-context, middleware `TenantResolve` + `RequireMembership`, эндпоинт переключения тенанта, переключатель в `header.js`. Репозитории ещё не трогаем (данные остаются на `DEFAULT 1`).
- **2b (роадмап)** — явный параметр `TenantScope` во все scoped-методы репозиториев и их вызывающих; снятие `DEFAULT 1`; тесты изоляции.
- **2c (роадмап)** — hot-path кэши (`TenantCache`, `MembershipCache`, grants-кэш tenant-keyed).

**Tech Stack:** Go, pgx/v5, chi-стиль middleware (как в `internal/http/server.go`), React-через-CDN (`tracker.js`/`header.js`).

## Global Constraints

- Source of truth — спека `docs/superpowers/specs/2026-06-21-tenant-foundation-design.md`. Plan 2 реализует раздел «Жизненный цикл запроса».
- Решение по протягиванию tenant_id: **явный параметр `TenantScope`** (подтверждено). 2a вводит тип и контекст; 2b протягивает его в репозитории.
- **Граница использования контекста (ключевое правило слоёв):** request-context несёт тенант
  ТОЛЬКО на HTTP-границе. `TenantResolve`-middleware кладёт тенант в контекст; **хендлер** —
  единственный, кто читает его из контекста (`auth.TenantScopeFromContext`), один раз. Ниже
  хендлера — **только явный параметр**: хендлер → сервис → репозиторий получают
  `domain.TenantScope` аргументом. **Сервисы и репозитории НЕ читают тенант из контекста.**
  Поэтому `TenantScopeFromContext` (Task 1) — это извлекатель на границе, а не неявный канал
  для бизнес-слоёв; протягивание явного параметра вниз — работа 2b.
- Слои не смешивать: резолв/гейт — в `internal/auth` (middleware) и `internal/http` (эндпоинт); SQL — в `internal/store`.
- Auth-данные только через middleware + context-хелперы (правило `010` №9).
- Любые state-changing HTTP действия из браузера проходят CSRF (правило №7) — switch-эндпоинт под CSRF.
- **Коммиты — за пользователем.** Агент только `git add` + предлагает сообщение; без упоминаний AI.
- **Безопасность развёртывания 2a в одиночку:** до 2b запросы не фильтруются по tenant_id (всё ещё `DEFAULT 1`). Поэтому **2a нельзя использовать с более чем одним тенантом, пока не выполнен 2b** — иначе данные не изолированы. В одно-тенантном режиме 2a безопасен и поведение не меняется.

---

### Task 1: TenantScope type + tenant/role request-context

**Files:**
- Create: `internal/domain/scope.go`
- Modify: `internal/auth/context.go`
- Test: `internal/auth/context_test.go` (add cases)

**Interfaces:**
- Produces:
  - `domain.TenantScope{ TenantID int64 }`
  - `auth.WithTenant(ctx, *domain.Tenant) context.Context`, `auth.TenantFromContext(ctx) *domain.Tenant`
  - `auth.WithActiveRole(ctx, domain.Role) context.Context`, `auth.ActiveRoleFromContext(ctx) (domain.Role, bool)`
  - `auth.TenantScopeFromContext(ctx) (domain.TenantScope, bool)` — derived from the tenant in ctx
- Consumes: `domain.Tenant`, `domain.Role` (Plan 1).

> **Layering note:** `TenantScopeFromContext` is the HTTP-boundary extractor — only handlers
> call it. Services and repositories receive `domain.TenantScope` as an explicit parameter
> (threaded in 2b) and never read it from context.

- [ ] **Step 1: Write the failing test**

```go
// add to internal/auth/context_test.go
func TestTenantContextRoundTrip(t *testing.T) {
	tn := &domain.Tenant{ID: 7, Slug: "acme"}
	ctx := WithTenant(context.Background(), tn)
	if got := TenantFromContext(ctx); got == nil || got.ID != 7 {
		t.Fatalf("TenantFromContext = %+v, want id 7", got)
	}
	sc, ok := TenantScopeFromContext(ctx)
	if !ok || sc.TenantID != 7 {
		t.Fatalf("TenantScopeFromContext = %+v ok=%v, want {7} true", sc, ok)
	}

	ctx = WithActiveRole(ctx, domain.RoleAdmin)
	if role, ok := ActiveRoleFromContext(ctx); !ok || role != domain.RoleAdmin {
		t.Fatalf("ActiveRoleFromContext = %v ok=%v, want admin true", role, ok)
	}
}

func TestTenantFromContextNilWhenAbsent(t *testing.T) {
	if got := TenantFromContext(context.Background()); got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
	if _, ok := TenantScopeFromContext(context.Background()); ok {
		t.Fatalf("expected scope absent")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/auth -run TestTenantContext -v`
Expected: FAIL — `undefined: WithTenant`.

- [ ] **Step 3: Add the TenantScope type**

```go
// internal/domain/scope.go
package domain

// TenantScope is the per-request tenant boundary passed explicitly into scoped
// repository methods. Carrying it as a named type (not a bare int64) prevents
// accidentally passing some other id as the tenant.
type TenantScope struct {
	TenantID int64
}
```

- [ ] **Step 4: Add context helpers**

```go
// add to internal/auth/context.go (reuse the existing unexported ctxKey pattern in this file)
func WithTenant(ctx context.Context, t *domain.Tenant) context.Context {
	return context.WithValue(ctx, tenantKey, t)
}

func TenantFromContext(ctx context.Context) *domain.Tenant {
	t, _ := ctx.Value(tenantKey).(*domain.Tenant)
	return t
}

func TenantScopeFromContext(ctx context.Context) (domain.TenantScope, bool) {
	t := TenantFromContext(ctx)
	if t == nil {
		return domain.TenantScope{}, false
	}
	return domain.TenantScope{TenantID: t.ID}, true
}

func WithActiveRole(ctx context.Context, role domain.Role) context.Context {
	return context.WithValue(ctx, activeRoleKey, role)
}

func ActiveRoleFromContext(ctx context.Context) (domain.Role, bool) {
	role, ok := ctx.Value(activeRoleKey).(domain.Role)
	return role, ok
}
```

Add the keys next to the existing context keys in `context.go` (match the file's existing key type — it uses an unexported key type; add two new constants):

```go
const (
	tenantKey     ctxKey = iota + 100 // offset to avoid clashing with existing keys
	activeRoleKey
)
```

> **Verify the key type before writing:** open `internal/auth/context.go`, find the existing
> context-key type (e.g. `type ctxKey int` and its constants). Reuse that exact type name and
> a non-colliding constant value. If the file uses distinct typed keys per value, follow that
> style instead of the iota block above.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/auth -run TestTenantContext -v`
Expected: PASS.

- [ ] **Step 6: Stage**

```bash
git add internal/domain/scope.go internal/auth/context.go internal/auth/context_test.go
# message: "feat(auth): add TenantScope and tenant/role request context"
```

---

### Task 2: Session active tenant persistence

**Files:**
- Modify: `internal/domain/models.go` (`AuthSession.ActiveTenantID *int64`)
- Modify: `internal/store/sessions/sessions.go` (scan + `SetActiveTenant`)
- Test: `internal/store/sessions/sessions_test.go`

**Interfaces:**
- Produces:
  - `domain.AuthSession.ActiveTenantID *int64`
  - `(*SessionRepository) SetActiveTenant(ctx, sessionID string, tenantID int64) error`
  - `GetSession` now also scans `active_tenant_id`.
- Consumes: migration 031 (`auth_sessions.active_tenant_id`).

- [ ] **Step 1: Write the failing test**

```go
// add to internal/store/sessions/sessions_test.go (match the existing setup helper in this file)
func TestSetAndReadActiveTenant(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := NewSessionRepository(pool)

	var userID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('google:s2','google','s2','S2') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := repo.CreateSession(ctx, "sess-abc", userID, "google", time.Hour, "ua", "ip"); err != nil {
		t.Fatalf("create session: %v", err)
	}

	if err := repo.SetActiveTenant(ctx, "sess-abc", 1); err != nil {
		t.Fatalf("set active tenant: %v", err)
	}
	sess, err := repo.GetSession(ctx, "sess-abc")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if sess.ActiveTenantID == nil || *sess.ActiveTenantID != 1 {
		t.Fatalf("ActiveTenantID = %v, want 1", sess.ActiveTenantID)
	}
}
```

> **Verify session test imports:** check `internal/store/sessions/sessions_test.go` for the
> existing DB-setup pattern (it may already import `okrs/internal/store/testutil`). Reuse it.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/sessions -run TestSetAndReadActiveTenant -v`
Expected: FAIL — `SetActiveTenant` undefined / `ActiveTenantID` undefined.

- [ ] **Step 3: Add the domain field**

In `internal/domain/models.go`, add to `type AuthSession struct` after `IP string`:

```go
	ActiveTenantID *int64
```

- [ ] **Step 4: Add SetActiveTenant and extend GetSession scan**

In `internal/store/sessions/sessions.go`:

```go
func (r *SessionRepository) SetActiveTenant(ctx context.Context, sessionID string, tenantID int64) error {
	_, err := r.db.Exec(ctx,
		`UPDATE auth_sessions SET active_tenant_id = $2 WHERE id = $1`, sessionID, tenantID)
	return err
}
```

Extend `GetSession` to select and scan `active_tenant_id` into `sess.ActiveTenantID`
(`*int64`). Open the file and add `active_tenant_id` to the SELECT column list and a
matching `&sess.ActiveTenantID` to the `Scan(...)` call.

> **Verify GetSession exactly:** read the current `GetSession` SELECT/Scan in
> `sessions.go` and append the column + scan target in the same positions. `active_tenant_id`
> is nullable — scan into `*int64` (pgx handles NULL → nil).

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/store/sessions -run TestSetAndReadActiveTenant -v`
Expected: PASS.

- [ ] **Step 6: Run the sessions suite (no regression)**

Run: `go test ./internal/store/sessions`
Expected: PASS.

- [ ] **Step 7: Stage**

```bash
git add internal/domain/models.go internal/store/sessions/sessions.go internal/store/sessions/sessions_test.go
# message: "feat(store): persist active tenant on session"
```

---

### Task 3: Tenant resolver (SessionStrategy)

**Files:**
- Create: `internal/auth/tenant_resolver.go`
- Test: `internal/auth/tenant_resolver_test.go`

**Interfaces:**
- Produces:
  - `type TenantLookup interface { GetByID(ctx, int64) (*domain.Tenant, error) }`
  - `type MembershipLookup interface { ListByUser(ctx, int64) ([]domain.Membership, error) }`
    (both satisfied by Plan 1 repos)
  - `type TenantResolver struct { tenants TenantLookup; members MembershipLookup }`
  - `NewTenantResolver(TenantLookup, MembershipLookup) *TenantResolver`
  - `(*TenantResolver) Resolve(ctx, user *domain.User, sess *domain.AuthSession) (*domain.Tenant, domain.Role, error)` —
    returns the session's active tenant if the user is a member; otherwise the user's first
    active membership; `ErrNoMembership` if none.
  - `var ErrNoMembership = errors.New(...)`, `var ErrNotMember = errors.New(...)`
- Consumes: `domain.Tenant`, `domain.Membership`, `domain.Role`.

- [ ] **Step 1: Write the failing test**

```go
// internal/auth/tenant_resolver_test.go
package auth

import (
	"context"
	"errors"
	"testing"

	"okrs/internal/domain"
)

type fakeTenants struct{ m map[int64]*domain.Tenant }

func (f fakeTenants) GetByID(_ context.Context, id int64) (*domain.Tenant, error) {
	if t, ok := f.m[id]; ok {
		return t, nil
	}
	return nil, errors.New("not found")
}

type fakeMembers struct{ m map[int64][]domain.Membership }

func (f fakeMembers) ListByUser(_ context.Context, uid int64) ([]domain.Membership, error) {
	return f.m[uid], nil
}

func TestResolvePrefersSessionActiveTenant(t *testing.T) {
	tenants := fakeTenants{m: map[int64]*domain.Tenant{
		1: {ID: 1, Slug: "default", Status: domain.TenantActive},
		2: {ID: 2, Slug: "acme", Status: domain.TenantActive},
	}}
	members := fakeMembers{m: map[int64][]domain.Membership{
		10: {{UserID: 10, TenantID: 1, Role: domain.RoleUser}, {UserID: 10, TenantID: 2, Role: domain.RoleAdmin}},
	}}
	r := NewTenantResolver(tenants, members)
	user := &domain.User{ID: 10}
	active := int64(2)
	sess := &domain.AuthSession{ActiveTenantID: &active}

	tn, role, err := r.Resolve(context.Background(), user, sess)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if tn.ID != 2 || role != domain.RoleAdmin {
		t.Fatalf("got tenant %d role %s, want 2/admin", tn.ID, role)
	}
}

func TestResolveFallsBackToFirstMembership(t *testing.T) {
	tenants := fakeTenants{m: map[int64]*domain.Tenant{1: {ID: 1, Status: domain.TenantActive}}}
	members := fakeMembers{m: map[int64][]domain.Membership{10: {{UserID: 10, TenantID: 1, Role: domain.RoleUser}}}}
	r := NewTenantResolver(tenants, members)

	tn, role, err := r.Resolve(context.Background(), &domain.User{ID: 10}, &domain.AuthSession{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if tn.ID != 1 || role != domain.RoleUser {
		t.Fatalf("got %d/%s, want 1/user", tn.ID, role)
	}
}

func TestResolveNoMembership(t *testing.T) {
	r := NewTenantResolver(fakeTenants{m: map[int64]*domain.Tenant{}}, fakeMembers{m: map[int64][]domain.Membership{}})
	if _, _, err := r.Resolve(context.Background(), &domain.User{ID: 99}, &domain.AuthSession{}); !errors.Is(err, ErrNoMembership) {
		t.Fatalf("want ErrNoMembership, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/auth -run TestResolve -v`
Expected: FAIL — `NewTenantResolver` undefined.

- [ ] **Step 3: Write the resolver**

```go
// internal/auth/tenant_resolver.go
package auth

import (
	"context"
	"errors"

	"okrs/internal/domain"
)

var (
	ErrNoMembership = errors.New("auth: user has no active membership")
	ErrNotMember    = errors.New("auth: user is not a member of the tenant")
)

type TenantLookup interface {
	GetByID(ctx context.Context, id int64) (*domain.Tenant, error)
}

type MembershipLookup interface {
	ListByUser(ctx context.Context, userID int64) ([]domain.Membership, error)
}

// TenantResolver resolves the active tenant for a request (SessionStrategy).
// Subdomain/email-domain strategies are added later (premium) without changing this core.
type TenantResolver struct {
	tenants TenantLookup
	members MembershipLookup
}

func NewTenantResolver(t TenantLookup, m MembershipLookup) *TenantResolver {
	return &TenantResolver{tenants: t, members: m}
}

// Resolve returns the active tenant and the user's role in it.
// Preference: session.ActiveTenantID (if the user is an active member); otherwise the
// first active membership. ErrNoMembership if the user has none.
func (r *TenantResolver) Resolve(ctx context.Context, user *domain.User, sess *domain.AuthSession) (*domain.Tenant, domain.Role, error) {
	memberships, err := r.members.ListByUser(ctx, user.ID)
	if err != nil {
		return nil, "", err
	}
	if len(memberships) == 0 {
		return nil, "", ErrNoMembership
	}

	pick := memberships[0]
	if sess != nil && sess.ActiveTenantID != nil {
		found := false
		for _, m := range memberships {
			if m.TenantID == *sess.ActiveTenantID {
				pick = m
				found = true
				break
			}
		}
		if !found {
			// Session points at a tenant the user no longer belongs to; fall back.
			pick = memberships[0]
		}
	}

	tn, err := r.tenants.GetByID(ctx, pick.TenantID)
	if err != nil {
		return nil, "", err
	}
	return tn, pick.Role, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/auth -run TestResolve -v`
Expected: PASS.

- [ ] **Step 5: Stage**

```bash
git add internal/auth/tenant_resolver.go internal/auth/tenant_resolver_test.go
# message: "feat(auth): add session-strategy tenant resolver"
```

---

### Task 4: TenantResolve + RequireMembership middleware

**Files:**
- Modify: `internal/auth/middleware.go`
- Test: `internal/auth/middleware_test.go` (create if absent)

**Interfaces:**
- Consumes: `TenantResolver` (Task 3), context helpers (Task 1), `UserFromContext`/`SessionFromContext`.
- Produces:
  - `TenantResolveMiddleware(resolver *TenantResolver) func(http.Handler) http.Handler` — on success injects tenant + active role into context; on `ErrNoMembership` injects nothing (lets RequireMembership decide).
  - `RequireMembershipMiddleware(next http.Handler) http.Handler` — 403 (API) / redirect to `/no-access` (web) when no tenant in context or tenant suspended.

- [ ] **Step 1: Write the failing test**

```go
// internal/auth/middleware_test.go
package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"okrs/internal/domain"
)

func TestRequireMembershipBlocksWhenNoTenant(t *testing.T) {
	called := false
	h := RequireMembershipMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/teams", nil)
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, req)

	if called {
		t.Fatalf("handler should not be called without tenant in context")
	}
	if rw.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rw.Code)
	}
}

func TestRequireMembershipBlocksSuspendedTenant(t *testing.T) {
	called := false
	h := RequireMembershipMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/teams", nil)
	ctx := WithTenant(req.Context(), &domain.Tenant{ID: 2, Status: domain.TenantSuspended})
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, req.WithContext(ctx))

	if called || rw.Code != http.StatusForbidden {
		t.Fatalf("suspended tenant must be blocked; called=%v code=%d", called, rw.Code)
	}
}

func TestRequireMembershipAllowsActiveTenant(t *testing.T) {
	called := false
	h := RequireMembershipMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/teams", nil)
	ctx := WithTenant(req.Context(), &domain.Tenant{ID: 1, Status: domain.TenantActive})
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, req.WithContext(ctx))

	if !called || rw.Code != http.StatusOK {
		t.Fatalf("active tenant must pass; called=%v code=%d", called, rw.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/auth -run TestRequireMembership -v`
Expected: FAIL — `RequireMembershipMiddleware` undefined.

- [ ] **Step 3: Implement both middlewares**

```go
// add to internal/auth/middleware.go
// TenantResolveMiddleware resolves the active tenant for the authenticated user and
// injects it (plus the user's role in that tenant) into the request context.
func TenantResolveMiddleware(resolver *TenantResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := UserFromContext(r.Context())
			if user == nil {
				next.ServeHTTP(w, r)
				return
			}
			sess := SessionFromContext(r.Context())
			tn, role, err := resolver.Resolve(r.Context(), user, sess)
			if err != nil {
				// No membership (or lookup error): leave tenant unset; RequireMembership handles it.
				next.ServeHTTP(w, r)
				return
			}
			ctx := WithTenant(r.Context(), tn)
			ctx = WithActiveRole(ctx, role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireMembershipMiddleware blocks requests without an active, non-suspended tenant.
func RequireMembershipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tn := TenantFromContext(r.Context())
		if tn == nil || tn.Status != domain.TenantActive {
			if isAPIRequest(r) {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			http.Redirect(w, r, "/no-access", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}
```

> Note: the web-redirect target `/no-access` is the OSS `NoMembershipHandler` stub built in
> Plan 4. Until then it 404s for web; API behavior (403) is fully covered here. Add a minimal
> `/no-access` route returning 200 with a placeholder page in Task 7 so web requests don't 404.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/auth -run TestRequireMembership -v`
Expected: PASS.

- [ ] **Step 5: Stage**

```bash
git add internal/auth/middleware.go internal/auth/middleware_test.go
# message: "feat(auth): add tenant resolve and require-membership middleware"
```

---

### Task 5: Switch-tenant endpoint

**Files:**
- Create: `internal/http/handlers/api/v1/tenants/handler.go`
- Test: `internal/http/handlers/api/v1/tenants/handler_test.go`
- Modify: `internal/http/server.go` (mount route)

**Interfaces:**
- Consumes: `MembershipLookup` (verify membership), `SessionRepository.SetActiveTenant`,
  `SessionFromContext`, `UserFromContext`, `tenants.TenantRepository.GetBySlug`.
- Produces: `POST /api/v1/session/tenant` body `{"slug":"acme"}` or `{"tenant_id":2}` →
  verifies active membership, updates `active_tenant_id`, returns `204`. `403` if not a member.

- [ ] **Step 1: Write the failing test**

```go
// internal/http/handlers/api/v1/tenants/handler_test.go
package tenants

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"okrs/internal/auth"
	"okrs/internal/domain"
)

type stubDeps struct {
	memberships []domain.Membership
	setCalled   int64
}

func (s *stubDeps) ListByUser(_ contextType, _ int64) ([]domain.Membership, error) {
	return s.memberships, nil
}
func (s *stubDeps) GetBySlug(_ contextType, slug string) (*domain.Tenant, error) {
	return &domain.Tenant{ID: 2, Slug: slug, Status: domain.TenantActive}, nil
}
func (s *stubDeps) SetActiveTenant(_ contextType, _ string, tenantID int64) error {
	s.setCalled = tenantID
	return nil
}

func TestSwitchTenantRejectsNonMember(t *testing.T) {
	deps := &stubDeps{memberships: nil} // user is not a member of tenant 2
	h := New(deps, deps, deps)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/session/tenant", strings.NewReader(`{"slug":"acme"}`))
	ctx := auth.WithUser(req.Context(), &domain.User{ID: 10})
	ctx = auth.WithSession(ctx, &domain.AuthSession{ID: "s1"})
	rw := httptest.NewRecorder()
	h.SwitchTenant(rw, req.WithContext(ctx))

	if rw.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rw.Code)
	}
	if deps.setCalled != 0 {
		t.Fatalf("SetActiveTenant must not be called for non-member")
	}
}

func TestSwitchTenantUpdatesSession(t *testing.T) {
	deps := &stubDeps{memberships: []domain.Membership{{UserID: 10, TenantID: 2, Role: domain.RoleUser}}}
	h := New(deps, deps, deps)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/session/tenant", strings.NewReader(`{"slug":"acme"}`))
	ctx := auth.WithUser(req.Context(), &domain.User{ID: 10})
	ctx = auth.WithSession(ctx, &domain.AuthSession{ID: "s1"})
	rw := httptest.NewRecorder()
	h.SwitchTenant(rw, req.WithContext(ctx))

	if rw.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rw.Code)
	}
	if deps.setCalled != 2 {
		t.Fatalf("SetActiveTenant called with %d, want 2", deps.setCalled)
	}
}
```

> **Two fixups when implementing:** (1) replace the placeholder `contextType` with
> `context.Context` (the test uses an alias only to keep this listing import-light — declare
> `type contextType = context.Context` in the test or import `context` and use it directly).
> (2) `auth.WithSession` must be exported — the existing file has unexported `withSession`;
> add an exported `WithSession` wrapper in `internal/auth/context.go` (mirrors existing
> `WithUser`). Do that as the first step of implementation.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/http/handlers/api/v1/tenants -v`
Expected: FAIL — package/`New`/`SwitchTenant` undefined.

- [ ] **Step 3: Export WithSession**

```go
// add to internal/auth/context.go
// WithSession injects a session into the context. Used by middleware and tests.
func WithSession(ctx context.Context, s *domain.AuthSession) context.Context {
	return withSession(ctx, s)
}
```

- [ ] **Step 4: Write the handler**

```go
// internal/http/handlers/api/v1/tenants/handler.go
package tenants

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"okrs/internal/auth"
	"okrs/internal/domain"
)

type MembershipLookup interface {
	ListByUser(ctx context.Context, userID int64) ([]domain.Membership, error)
}
type TenantLookup interface {
	GetBySlug(ctx context.Context, slug string) (*domain.Tenant, error)
}
type SessionWriter interface {
	SetActiveTenant(ctx context.Context, sessionID string, tenantID int64) error
}

type Handler struct {
	members  MembershipLookup
	tenants  TenantLookup
	sessions SessionWriter
}

func New(m MembershipLookup, t TenantLookup, s SessionWriter) *Handler {
	return &Handler{members: m, tenants: t, sessions: s}
}

type switchRequest struct {
	Slug     string `json:"slug"`
	TenantID int64  `json:"tenant_id"`
}

// SwitchTenant sets the session's active tenant after verifying active membership.
func (h *Handler) SwitchTenant(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	sess := auth.SessionFromContext(r.Context())
	if user == nil || sess == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req switchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}

	targetID := req.TenantID
	if req.Slug != "" {
		tn, err := h.tenants.GetBySlug(r.Context(), req.Slug)
		if err != nil {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		targetID = tn.ID
	}
	if targetID == 0 {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}

	if !h.isActiveMember(r.Context(), user.ID, targetID) {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	if err := h.sessions.SetActiveTenant(r.Context(), sess.ID, targetID); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) isActiveMember(ctx context.Context, userID, tenantID int64) bool {
	ms, err := h.members.ListByUser(ctx, userID)
	if err != nil {
		return false
	}
	for _, m := range ms {
		if m.TenantID == tenantID {
			return true
		}
	}
	return false
}

var _ = errors.New // keep imports stable if trimmed during edits
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/http/handlers/api/v1/tenants -v`
Expected: PASS.

- [ ] **Step 6: Mount the route in server.go**

In `internal/http/server.go`, inside the authenticated+CSRF block (after
`ScopeMiddleware`, alongside other `/api/v1` mounts), construct and mount:

```go
tenantH := apitenants.New(s.store.Memberships, s.store.Tenants, s.store.Sessions)
r.Post("/api/v1/session/tenant", tenantH.SwitchTenant)
```

Add the import `apitenants "okrs/internal/http/handlers/api/v1/tenants"`. Place the route so
it passes through `RequireAuth` + CSRF (state-changing POST).

> **Verify the router style:** match how other `/api/v1` POST routes are registered in
> `server.go` (the repo uses chi — use the same `r.Post`/`r.Route` idiom already present).

- [ ] **Step 7: Stage**

```bash
git add internal/auth/context.go internal/http/handlers/api/v1/tenants/ internal/http/server.go
# message: "feat(api): add switch-tenant endpoint"
```

---

### Task 6: Tenant switcher in the shared header

**Files:**
- Modify: `internal/http/.../header.js` (locate via `rg -l "HeaderNavMenu" internal`)
- Modify: a `/api/v1` read that returns the user's memberships for the switcher (add to an
  existing config/me endpoint, or a new `GET /api/v1/session/tenants`).

**Interfaces:**
- Consumes: `POST /api/v1/session/tenant` (Task 5).
- Produces: a tenant dropdown in the header listing the user's active memberships; selecting
  one POSTs the switch and reloads.

- [ ] **Step 1: Add a memberships-list endpoint**

Add `GET /api/v1/session/tenants` returning `[{id, slug, name}]` for the current user's
active memberships (join memberships→tenants). Implement in the Task 5 handler package
(`Handler.ListMyTenants`) and mount it in `server.go` next to the switch route. Write a unit
test mirroring Task 5's stub style (assert it returns the membership tenants as JSON).

- [ ] **Step 2: Add the switcher UI to header.js**

In the shared header component, add a dropdown next to the account block that fetches
`/api/v1/session/tenants`, shows the active tenant (from a new field on `/api/v1/config` or
the list's marked active), and on change does:

```js
await fetch('/api/v1/session/tenant', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': readCSRFCookie() },
  body: JSON.stringify({ tenant_id: selectedId }),
});
window.location.reload();
```

Reuse the existing CSRF-cookie read already present in `header.js` (the file reads CSRF for
logout). Match existing styling/classes; no bundler/toolchain (constraint `010` №5).

- [ ] **Step 3: Manual verification**

Run the app (`/run` skill or docker compose), confirm: switcher lists memberships, switching
reloads and the session's active tenant changes (verify via DB `auth_sessions.active_tenant_id`).
Document the steps run.

- [ ] **Step 4: Stage**

```bash
git add internal/http/handlers/api/v1/tenants/ <header.js path> internal/http/server.go
# message: "feat(ui): tenant switcher in shared header"
```

---

### Task 7: Wire the middleware chain + no-auth mode

**Files:**
- Modify: `internal/http/server.go`
- Modify: `internal/auth/middleware.go` (`AnonymousUserMiddleware` → also set tenant #1)
- Test: `internal/http/...` integration test for the chain (extend existing server/middleware tests)

**Interfaces:**
- Consumes: all of Tasks 1–5.
- Produces: chain `… RequireAuth → TenantResolve → RequireMembership → Scope → CSRF …`;
  no-auth mode injects tenant #1 + admin role; a `/no-access` stub route (web).

- [ ] **Step 1: Construct the resolver in NewServer**

In `NewServer`, after the store/policy are available:

```go
tenantResolver := auth.NewTenantResolver(s.store.Tenants, s.store.Memberships)
```

Store it on the `Server` struct (add field `tenantResolver *auth.TenantResolver`).

- [ ] **Step 2: Insert middleware into the authenticated chain**

In the authenticated branch (where `RequireAuthMiddleware` + `ScopeMiddleware` are added),
insert between them:

```go
r.Use(auth.TenantResolveMiddleware(s.tenantResolver))
r.Use(auth.RequireMembershipMiddleware)
```

Order: `RequireAuth → TenantResolve → RequireMembership → Scope`.

- [ ] **Step 3: No-auth mode resolves tenant #1**

In `AnonymousUserMiddleware`, after injecting the anon user, also inject tenant #1 + admin
role so `RequireMembership` passes in `AUTH_MODE=disabled`:

```go
ctx := withUser(r.Context(), anon)
ctx = WithTenant(ctx, &domain.Tenant{ID: 1, Slug: "default", Status: domain.TenantActive})
ctx = WithActiveRole(ctx, domain.RoleAdmin)
next.ServeHTTP(w, r.WithContext(ctx))
```

> In disabled mode the chain does not run `TenantResolve` (no Session middleware either), so
> setting tenant here is what lets `RequireMembership` pass. Confirm the disabled-mode branch
> in `server.go` still adds `RequireMembershipMiddleware`; if disabled mode skips the
> authenticated block, no change is needed beyond the anon tenant for any path that does run it.

- [ ] **Step 4: Add /no-access stub route (web)**

Mount a minimal handler returning `200` with a short "no access — contact your administrator"
page for `GET /no-access` (outside RequireMembership, inside Session/RequireAuth). Plan 4
replaces it with the pluggable `NoMembershipHandler`.

- [ ] **Step 5: Run the focused chain test + full suite**

```bash
go test ./internal/auth/... ./internal/http/...
go vet ./... && go test ./...
```

Expected: PASS. Existing flows behave unchanged in single-tenant mode (every user is a member
of tenant #1 via Plan 1 backfill, so `RequireMembership` passes).

- [ ] **Step 6: Stage**

```bash
git add internal/http/server.go internal/auth/middleware.go
# message: "feat(http): wire tenant resolve/require-membership into the request chain"
```

---

## Plan 2b roadmap (next plan — repository scoping)

The big mechanical change, authored as its own plan once 2a lands and is reviewed:

- Add explicit `scope domain.TenantScope` to every scoped repository method
  (`teams`, `goals`, `periods`, `krs`, `shares`, `statuses`, `grants`) — both reads
  (`WHERE tenant_id = $scope`) and writes (`tenant_id` column on INSERT).
- Thread `TenantScope` from handlers (via `auth.TenantScopeFromContext`) through services
  into repositories.
- Update `PolicyEvaluator`/grants queries to filter by tenant.
- Remove the transitional `DEFAULT 1` from `tenant_id` columns (new migration) once all
  writes pass `tenant_id` explicitly.
- **Isolation tests** (the security core): a user scoped to tenant A cannot read/mutate
  tenant B rows on each endpoint. Seed two tenants and assert cross-tenant 404/empty.

Sequencing within 2b: one repository (+ its callers) per task, each with an isolation test,
to keep diffs reviewable. `DEFAULT 1` removal is the final task.

## Plan 2c roadmap (next plan — hot-path caches)

- `TenantCache` (id/slug → tenant), `MembershipCache` (user → memberships), grants-cache
  tenant-keyed; TTL + invalidate-on-write hook.
- `TenantResolve`/`RequireMembership` read from caches; resolve once into request-context.
- LRU/TTL eviction for many-tenant SaaS; cross-instance invalidation deferred (documented).

## Self-Review Notes

- **Spec coverage (2a slice):** TenantScope + context (Task 1), session active tenant
  (Task 2), SessionStrategy resolver (Task 3), `TenantResolve`+`RequireMembership` (Task 4),
  switch endpoint (Task 5), header switcher (Task 6), chain wiring + no-auth tenant (Task 7).
  Repository scoping + `DEFAULT 1` removal + isolation tests → 2b. Caches → 2c.
- **Verify-before-write hooks:** Tasks 1, 2, 5, 6, 7 include `rg`/read checks for the exact
  existing context-key style, `GetSession` scan, router idiom, and `header.js` location.
- **Safety:** 2a alone keeps single-tenant behavior (all users are tenant-#1 members from
  Plan 1 backfill). Multi-tenant must not be enabled until 2b lands (isolation enforcement).
