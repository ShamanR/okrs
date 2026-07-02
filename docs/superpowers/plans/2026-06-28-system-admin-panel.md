# System-admin panel (`/system`) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans (inline,
> compiler-driven — subagents flap in this environment). Steps use checkbox (`- [ ]`) syntax.
> Build/vet/test green after every task; commits are the user's (agent does `git add` +
> proposes a message, no AI attribution).

**Goal:** Turn the minimal `/system` stub into a full React system-admin panel — tenants
(create / suspend / restore), members (list + attach existing users), default-registration
tenant, and `entitlement.*` editing — backed by three new read endpoints, with authorization
required on the whole `/system` plane in every auth mode.

**Architecture:** Frontend mirrors `/admin`: a React UMD + Babel SPA (`system.js`) mounted by a
rewritten `system_shell.html`, reusing the `okr_csrf_token` cookie + `apiGet/apiPost` pattern.
Backend adds three GET endpoints (`…/tenants/{id}/members`, `…/tenants/{id}/entitlements`,
`/system/settings`) over existing tables/services, and removes the `AUTH_MODE=disabled` bypass so
the system gate (`RequireSystemAdmin`: session `is_system_admin` OR `Bearer PROVISIONING_TOKEN`)
applies unconditionally.

**Tech Stack:** Go, chi, pgx/v5, testcontainers-go; React 18 (UMD) + Babel standalone (CDN), as
in `internal/web/static/admin.js`.

## Global Constraints

- **Authorization mandatory on all `/system` methods, every mode.** Remove the
  `if !s.auth.Disabled()` skip in `registerSystemRoutes`; the gate always applies. In
  `AUTH_MODE=disabled`, `anonymous-local` is **not** a system-admin → access only via
  `PROVISIONING_TOKEN`.
- **No new DB schema.** Only new read methods over existing `tenants`/`memberships`/`tenant_settings`.
- **No raw HTML in the frontend** (XSS rule, `specs/010`): render via React only.
- **Explicit `domain.TenantScope`** in services/repos; only handlers read context. System
  endpoints take `tenant_id` from the URL (cross-tenant plane), not from context.
- **Commits are the user's**; agent stages + proposes a message; no AI/Claude attribution.
- Entitlements have **no runtime effect in OSS** (`UnlimitedEntitlements` ignores them); the
  editor writes `tenant_settings` as a forward seam for SaaS. The UI states this.

---

### Task 1: `MembershipRepository.ListByTenant` (members read model)

**Files:**
- Modify: `internal/store/memberships/memberships.go` (add `Status` to `AccessRequest`; add `ListByTenant`; include `status` in the existing `ListAccessRequests` query)
- Test: `internal/store/memberships/memberships_test.go`

**Interfaces:**
- Consumes: `domain.TenantScope`.
- Produces:
  - `AccessRequest` gains `Status domain.MembershipStatus`.
  - `(*MembershipRepository) ListByTenant(ctx, scope domain.TenantScope) ([]AccessRequest, error)`
    — all statuses, ordered by `display_name`.

- [ ] **Step 1: Write the failing test**

```go
// add to internal/store/memberships/memberships_test.go
func TestListByTenantReturnsAllStatuses(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := NewMembershipRepository(pool)

	var active, pending int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('github:a','github','a','Active') RETURNING id`).Scan(&active); err != nil {
		t.Fatalf("seed active: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('github:p','github','p','Pending') RETURNING id`).Scan(&pending); err != nil {
		t.Fatalf("seed pending: %v", err)
	}
	if _, err := repo.Upsert(ctx, domain.Membership{UserID: active, TenantID: 1, Role: domain.RoleAdmin, Status: domain.MembershipActive}); err != nil {
		t.Fatalf("upsert active: %v", err)
	}
	if _, err := repo.Upsert(ctx, domain.Membership{UserID: pending, TenantID: 1, Role: domain.RoleUser, Status: domain.MembershipRequested}); err != nil {
		t.Fatalf("upsert pending: %v", err)
	}

	got, err := repo.ListByTenant(ctx, domain.TenantScope{TenantID: 1})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	byUser := map[int64]AccessRequest{}
	for _, a := range got {
		byUser[a.UserID] = a
	}
	if byUser[active].Status != domain.MembershipActive || byUser[active].Role != domain.RoleAdmin {
		t.Fatalf("active = %+v", byUser[active])
	}
	if byUser[pending].Status != domain.MembershipRequested {
		t.Fatalf("pending = %+v", byUser[pending])
	}
}
```

> Tenant #1 already has `anonymous-local`/`migration` memberships from migrations, so assert by
> looking up the seeded users (not by slice length).

- [ ] **Step 2: Run to verify it fails** — `go test ./internal/store/memberships -run TestListByTenant`
  → FAIL (`Status` field / `ListByTenant` undefined).

- [ ] **Step 3: Add `Status` to the read model and the two queries**

```go
// AccessRequest read model (extend with Status):
type AccessRequest struct {
	UserID      int64
	DisplayName string
	Email       string
	Role        domain.Role
	Status      domain.MembershipStatus
	CreatedAt   time.Time
}
```

Update `ListAccessRequests` to also select `m.status` (keep its `WHERE m.status = 'requested'`),
scanning into `&a.Status`. Then add:

```go
// ListByTenant returns every membership of the tenant (all statuses) joined to users.
func (r *MembershipRepository) ListByTenant(ctx context.Context, scope domain.TenantScope) ([]AccessRequest, error) {
	rows, err := r.db.Query(ctx, `
		SELECT m.user_id, u.display_name, COALESCE(u.email,''), m.role, m.status, m.created_at
		FROM memberships m JOIN users u ON u.id = m.user_id
		WHERE m.tenant_id = $1
		ORDER BY u.display_name`, scope.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AccessRequest
	for rows.Next() {
		var a AccessRequest
		if err := rows.Scan(&a.UserID, &a.DisplayName, &a.Email, &a.Role, &a.Status, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
```

  Also update the `ListAccessRequests` scan to include `&a.Status` and its SELECT to add `m.status`.

- [ ] **Step 4: Run** `go test ./internal/store/memberships` → PASS (both `ListByTenant` and the
  existing `ListAccessRequests` test stay green).
- [ ] **Step 5: Stage** `git add internal/store/memberships/`
  (message: `feat(store): list tenant members (all statuses)`).

---

### Task 2: `SettingsService.TenantEntitlements` (entitlement.* read)

**Files:**
- Modify: `internal/service/settings.go`
- Test: `internal/service/settings_test.go`

**Interfaces:**
- Produces: `(*SettingsService) TenantEntitlements(ctx, scope domain.TenantScope) (map[string]json.RawMessage, error)`
  — returns only keys with the `entitlement.` prefix, with the prefix stripped.

- [ ] **Step 1: Write the failing test**

```go
// add to internal/service/settings_test.go
func TestTenantEntitlementsStripsPrefix(t *testing.T) {
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

	if err := svc.SetTenantEntitlement(ctx, scope, "entitlement.sso", true); err != nil {
		t.Fatalf("set entitlement: %v", err)
	}
	if err := svc.SetTenantProduct(ctx, scope, "documentation_url", "https://x"); err != nil {
		t.Fatalf("set product: %v", err)
	}

	ent, err := svc.TenantEntitlements(ctx, scope)
	if err != nil {
		t.Fatalf("entitlements: %v", err)
	}
	if _, ok := ent["sso"]; !ok {
		t.Fatalf("expected stripped key 'sso', got %v", ent)
	}
	if _, leaked := ent["documentation_url"]; leaked {
		t.Fatalf("product key leaked into entitlements: %v", ent)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `go test ./internal/service -run TestTenantEntitlements`.

- [ ] **Step 3: Implement** (the service already imports `strings`, `json`, `domain`; `EntitlementPrefix` const exists)

```go
// TenantEntitlements returns the tenant's entitlement.* keys with the prefix stripped.
func (s *SettingsService) TenantEntitlements(ctx context.Context, scope domain.TenantScope) (map[string]json.RawMessage, error) {
	snap, err := s.tsCache.GetAll(ctx, scope)
	if err != nil {
		return nil, err
	}
	out := make(map[string]json.RawMessage)
	for k, v := range snap {
		if strings.HasPrefix(k, EntitlementPrefix) {
			out[strings.TrimPrefix(k, EntitlementPrefix)] = v
		}
	}
	return out, nil
}
```

- [ ] **Step 4: Run** `go test ./internal/service -run TestTenantEntitlements` → PASS.
- [ ] **Step 5: Stage** (message: `feat(service): read tenant entitlements (entitlement.* snapshot)`).

---

### Task 3: System handler — three new GET endpoints

**Files:**
- Modify: `internal/http/handlers/api/v1/system/handler.go`
- Test: `internal/http/handlers/api/v1/system/handler_test.go`

**Interfaces:**
- The handler gains a members source and extended settings reads. Updated constructor:
  `func New(prov Provisioner, settings SystemSettings, users UserLister, tenantsList TenantLister, members MemberLister) *Handler`.
- `SystemSettings` grows: `SystemGet(ctx, key string) (json.RawMessage, error)` and
  `TenantEntitlements(ctx, scope domain.TenantScope) (map[string]json.RawMessage, error)`
  (both satisfied by `*service.SettingsService`).
- `type MemberLister interface { ListByTenant(ctx context.Context, scope domain.TenantScope) ([]memberships.AccessRequest, error) }`
  (satisfied by `*store.MembershipRepository`).
- Produces handlers: `HandleListMembers`, `HandleGetEntitlements`, `HandleGetSettings`.

- [ ] **Step 1: Add the failing tests** (the test file already has a `buildRouter` helper from
  Plan 3 with a system-admin user; extend it to wire `members` and mount the new routes, then add
  cases). Update `buildRouter` to construct the handler with the members repo and register:

```go
	r.Get("/api/v1/system/tenants/{id}/members", h.HandleListMembers)
	r.Get("/api/v1/system/tenants/{id}/entitlements", h.HandleGetEntitlements)
	r.Get("/api/v1/system/settings", h.HandleGetSettings)
```

  and `h := apisystem.New(prov, settingsSvc, userRepo, tnRepo, memRepo)` (add `memRepo :=
  memberships.NewMembershipRepository(pool)`).

```go
func TestSystemListMembersAndEntitlements(t *testing.T) {
	admin := &domain.User{ID: 1, IsSystemAdmin: true}
	r, tnRepo, _ := buildRouter(t, admin)
	ctx := context.Background()

	// Create tenant, attach the anon user as admin, set an entitlement — all via the API.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/system/tenants",
		strings.NewReader(`{"name":"Acme","slug":"acme"}`)))
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d (%s)", w.Code, w.Body.String())
	}
	var created struct{ ID int64 `json:"id"` }
	_ = json.NewDecoder(w.Body).Decode(&created)
	id := strconv.FormatInt(created.ID, 10)

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/system/tenants/"+id+"/members",
		strings.NewReader(`{"user_id":1,"role":"admin"}`)))
	if w.Code != http.StatusCreated {
		t.Fatalf("attach: %d (%s)", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/v1/system/tenants/"+id+"/entitlements",
		strings.NewReader(`{"sso":true}`)))
	if w.Code != http.StatusNoContent {
		t.Fatalf("set entitlements: %d (%s)", w.Code, w.Body.String())
	}

	// Members lists the attached user.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/system/tenants/"+id+"/members", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("members: %d", w.Code)
	}
	var members []map[string]any
	_ = json.NewDecoder(w.Body).Decode(&members)
	if len(members) != 1 || members[0]["role"] != "admin" {
		t.Fatalf("members = %v", members)
	}

	// Entitlements returns the stripped key.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/system/tenants/"+id+"/entitlements", nil))
	var ent map[string]any
	_ = json.NewDecoder(w.Body).Decode(&ent)
	if ent["sso"] != true {
		t.Fatalf("entitlements = %v", ent)
	}
	_ = tnRepo
	_ = ctx
}

func TestSystemGetSettings(t *testing.T) {
	admin := &domain.User{ID: 1, IsSystemAdmin: true}
	r, _, _ := buildRouter(t, admin)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/v1/system/settings/default-registration-tenant",
		strings.NewReader(`{"tenant_id":1}`)))
	if w.Code != http.StatusNoContent {
		t.Fatalf("set default: %d", w.Code)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/system/settings", nil))
	var got struct {
		DefaultRegistrationTenantID *int64 `json:"default_registration_tenant_id"`
	}
	_ = json.NewDecoder(w.Body).Decode(&got)
	if got.DefaultRegistrationTenantID == nil || *got.DefaultRegistrationTenantID != 1 {
		t.Fatalf("default tenant = %v", got.DefaultRegistrationTenantID)
	}
}
```

  Also register `PUT /api/v1/system/settings/default-registration-tenant` in `buildRouter` (it may
  not be there from Plan 3 — add it) so `TestSystemGetSettings` can set then read.

- [ ] **Step 2: Run to verify it fails** — `go test ./internal/http/handlers/api/v1/system`.

- [ ] **Step 3: Extend interfaces + constructor in `handler.go`**

```go
type SystemSettings interface {
	SystemSet(ctx context.Context, key string, value any) error
	SystemGet(ctx context.Context, key string) (json.RawMessage, error)
	TenantEntitlements(ctx context.Context, scope domain.TenantScope) (map[string]json.RawMessage, error)
}

type MemberLister interface {
	ListByTenant(ctx context.Context, scope domain.TenantScope) ([]memberships.AccessRequest, error)
}

type Handler struct {
	prov     Provisioner
	settings SystemSettings
	users    UserLister
	tenants  TenantLister
	members  MemberLister
}

func New(prov Provisioner, settings SystemSettings, users UserLister, tenantsList TenantLister, members MemberLister) *Handler {
	return &Handler{prov: prov, settings: settings, users: users, tenants: tenantsList, members: members}
}
```

  Add imports `"encoding/json"` (present), `"okrs/internal/store/memberships"`.

- [ ] **Step 4: Implement the three handlers**

```go
// GET /api/v1/system/tenants/{id}/members
func (h *Handler) HandleListMembers(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	list, err := h.members.ListByTenant(r.Context(), domain.TenantScope{TenantID: id})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, m := range list {
		out = append(out, map[string]any{
			"user_id": m.UserID, "display_name": m.DisplayName, "email": m.Email,
			"role": string(m.Role), "status": string(m.Status),
		})
	}
	writeJSON(w, out)
}

// GET /api/v1/system/tenants/{id}/entitlements
func (h *Handler) HandleGetEntitlements(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ent, err := h.settings.TenantEntitlements(r.Context(), domain.TenantScope{TenantID: id})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ent == nil {
		ent = map[string]json.RawMessage{}
	}
	writeJSON(w, ent)
}

// GET /api/v1/system/settings
func (h *Handler) HandleGetSettings(w http.ResponseWriter, r *http.Request) {
	raw, err := h.settings.SystemGet(r.Context(), "default_registration_tenant_id")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var tenantID *int64
	if raw != nil {
		_ = json.Unmarshal(raw, &tenantID)
	}
	writeJSON(w, map[string]any{"default_registration_tenant_id": tenantID})
}
```

- [ ] **Step 5: Run** `go test ./internal/http/handlers/api/v1/system` → PASS. (The existing Plan-3
  tests in this file keep passing; they construct `New(...)` — update those call sites to pass
  `memRepo` as the 5th arg.)
- [ ] **Step 6: Stage** (message: `feat(system): members, entitlements, and settings read endpoints`).

---

### Task 4: Wire routes + enforce the gate unconditionally

**Files:**
- Modify: `internal/http/server.go` (`registerSystemRoutes`: drop `if !s.auth.Disabled()`; build
  handler with `s.store.Memberships`; mount the three GET routes + ensure
  `PUT …/settings/default-registration-tenant` is present)
- Test: `app/app_test.go` (disabled-mode gate assertion)

**Interfaces:**
- Consumes: `apisystem.New(..., s.store.Memberships)` (Task 3), `auth.RequireSystemAdminMiddleware`.

- [ ] **Step 1: Add the failing gate test** — in `AUTH_MODE=disabled`, `/api/v1/system/*` must be
  `403` (anonymous-local is not a system-admin, no token configured):

```go
// add to app/app_test.go
func TestSystemPlaneGatedInDisabledMode(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	a, err := app.New(app.Config{Pool: pool, Zone: time.UTC, Auth: auth.DefaultConfig()})
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	rw := httptest.NewRecorder()
	a.Handler.ServeHTTP(rw, httptest.NewRequest(http.MethodGet, "/api/v1/system/tenants", nil))
	if rw.Code != http.StatusForbidden {
		t.Fatalf("system plane must be gated even in AUTH_MODE=disabled, got %d", rw.Code)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `go test ./app -run TestSystemPlaneGatedInDisabledMode`
  → FAIL (currently `200`/reachable, gate skipped in disabled mode).

- [ ] **Step 3: Update `registerSystemRoutes`** — always apply the gate and mount the reads:

```go
func (s *Server) registerSystemRoutes(r chi.Router, csrf *middleware.CSRFMiddleware) {
	sysH := apisystem.New(s.provisioning, s.settingsSvc, s.store.Users, s.store.Tenants, s.store.Memberships)

	r.Group(func(r chi.Router) {
		// Authorization is mandatory for the whole system plane in EVERY mode (no disabled bypass):
		// anonymous-local is not a system-admin, so disabled-mode access requires a provisioning token.
		r.Use(auth.RequireSystemAdminMiddleware(s.auth.Config().ProvisioningToken))
		r.Use(csrf.Handler)

		r.Get("/system", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_ = s.tmpl.ExecuteTemplate(w, "system-shell", nil)
		})

		r.Post("/api/v1/system/tenants", sysH.HandleCreateTenant)
		r.Get("/api/v1/system/tenants", sysH.HandleListTenants)
		r.Post("/api/v1/system/tenants/{id}/members", sysH.HandleAttachMember)
		r.Get("/api/v1/system/tenants/{id}/members", sysH.HandleListMembers)
		r.Put("/api/v1/system/tenants/{id}/entitlements", sysH.HandleSetEntitlements)
		r.Get("/api/v1/system/tenants/{id}/entitlements", sysH.HandleGetEntitlements)
		r.Post("/api/v1/system/tenants/{id}/suspend", sysH.HandleSuspend)
		r.Post("/api/v1/system/tenants/{id}/restore", sysH.HandleRestore)
		r.Get("/api/v1/system/users", sysH.HandleListUsers)
		r.Get("/api/v1/system/settings", sysH.HandleGetSettings)
		r.Put("/api/v1/system/settings/default-registration-tenant", sysH.HandleSetDefaultRegistrationTenant)
	})
}
```

  > Confirm against the current `registerSystemRoutes` body and keep any routes already present;
  > the change set is: remove the `if !s.auth.Disabled()` wrapper around the gate, and add the
  > `GET members` / `GET entitlements` / `GET settings` routes.

- [ ] **Step 4: Run** `go build ./... && go test ./app ./internal/http/...` → PASS.
- [ ] **Step 5: Run the FULL suite** `go test ./...` → all green (catches any test that relied on
  disabled-mode `/system` being open).
- [ ] **Step 6: Stage** (message: `feat(system): always-gate the system plane; mount read routes`).

---

### Task 5: React panel — `system_shell.html` + `system.js`

**Files:**
- Modify: `internal/http/templates/system_shell.html` (rewrite to a React mount shell)
- Create: `internal/web/static/system.js`
- Test: manual per `specs/010` DoD (no frontend auto-tests in this project)

**Interfaces:**
- Consumes the endpoints: `GET/POST /api/v1/system/tenants`, `POST …/{id}/suspend|restore`,
  `GET/POST …/{id}/members`, `GET/PUT …/{id}/entitlements`, `GET /api/v1/system/users`,
  `GET /api/v1/system/settings`, `PUT …/settings/default-registration-tenant`.

- [ ] **Step 1: Rewrite `system_shell.html`** as a React shell (mirror `admin_shell.html` — the
  static-file server serves `/static/*`; CSRF cookie + `header.js` are shared):

```html
{{define "system-shell"}}
<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>OKR System · Управление</title>
<link rel="stylesheet" href="/static/header.css">
<style>
*{box-sizing:border-box;margin:0;padding:0}
html,body,#root{height:100%;font-family:'Inter',-apple-system,BlinkMacSystemFont,'Segoe UI',system-ui,sans-serif;background:#edf0f4;color:#111827;font-size:14px}
a{color:#2563eb;text-decoration:none}
button{font-family:inherit}
input,select{font-family:inherit;font-size:14px}
</style>
</head>
<body>
<div id="root"><div style="height:100vh;display:flex;align-items:center;justify-content:center;color:#6b7280">Загрузка…</div></div>
<script src="https://unpkg.com/react@18.3.1/umd/react.development.js" integrity="sha384-hD6/rw4ppMLGNu3tX5cjIb+uRZ7UkRJ6BPkLpg4hAu/6onKUg4lLsHAs9EBPT82L" crossorigin="anonymous"></script>
<script src="https://unpkg.com/react-dom@18.3.1/umd/react-dom.development.js" integrity="sha384-u6aeetuaXnQ38mYT8rp6sbXaQe3NL9t+IBXmnYxwkUI2Hw4bsp2Wvmx4yRQF1uAm" crossorigin="anonymous"></script>
<script src="https://unpkg.com/@babel/standalone@7.29.0/babel.min.js" integrity="sha384-m08KidiNqLdpJqLq95G/LEi8Qvjl/xUYll3QILypMoQ65QorJ9Lvtp2RXYGBFj1y" crossorigin="anonymous"></script>
<script type="text/babel" src="/static/system.js" data-presets="react"></script>
</body>
</html>
{{end}}
```

> Copy the exact `integrity` hashes from `admin_shell.html` (they must match the pinned versions);
> the values above are those hashes. If a hash mismatches at runtime, re-copy from `admin_shell.html`.

- [ ] **Step 2: Create `internal/web/static/system.js`** — the full panel:

```jsx
// OKR System-admin — React SPA (CDN React 18 + Babel standalone), mirrors admin.js conventions.
const {useState, useEffect, useCallback} = React;

function readCSRF() {
  return document.cookie.split(';').map(c=>c.trim()).find(c=>c.startsWith('okr_csrf_token='))?.split('=')[1] || '';
}
function csrfHeaders(extra={}) { return {'X-CSRF-Token': readCSRF(), 'Content-Type':'application/json', ...extra}; }
async function api(url, opts={}) {
  const res = await fetch(url, opts);
  if (res.status === 401) { window.location.href = '/login'; return null; }
  return res;
}
const get  = (u)      => api(u);
const post = (u, b)   => api(u, {method:'POST', headers:csrfHeaders(), body: b===undefined?undefined:JSON.stringify(b)});
const put  = (u, b)   => api(u, {method:'PUT',  headers:csrfHeaders(), body: JSON.stringify(b)});
async function errMsg(res){ try { const j = await res.json(); return j.error || ('Ошибка '+res.status); } catch { return 'Ошибка '+res.status; } }

const C = { card:'#fff', border:'#e5e7eb', accent:'#2563eb', danger:'#b91c1c', ok:'#047857', muted:'#6b7280' };
const box = {background:C.card, border:'1px solid '+C.border, borderRadius:10, padding:16, marginBottom:16};
const btn = {padding:'6px 12px', border:'none', borderRadius:7, background:C.accent, color:'#fff', fontWeight:600, cursor:'pointer'};
const inp = {padding:'6px 10px', border:'1.5px solid '+C.border, borderRadius:7};

function TenantsSection({tenants, reload}) {
  const [name,setName]=useState(''); const [slug,setSlug]=useState(''); const [err,setErr]=useState('');
  const create = async (e)=>{ e.preventDefault(); setErr('');
    const res = await post('/api/v1/system/tenants', {name, slug});
    if (res.status===201){ setName(''); setSlug(''); reload(); } else setErr(await errMsg(res));
  };
  const setStatus = async (id, action)=>{ const res = await post(`/api/v1/system/tenants/${id}/${action}`); if (res.status===204) reload(); else setErr(await errMsg(res)); };
  return <div style={box}>
    <h2 style={{fontSize:15,marginBottom:10}}>Тенанты</h2>
    <table style={{width:'100%',borderCollapse:'collapse'}}>
      <thead><tr>{['ID','Slug','Название','Статус',''].map(h=><th key={h} style={{textAlign:'left',padding:'6px 8px',borderBottom:'1px solid '+C.border}}>{h}</th>)}</tr></thead>
      <tbody>{(tenants||[]).map(t=><tr key={t.id}>
        <td style={{padding:'6px 8px'}}>{t.id}</td><td style={{padding:'6px 8px'}}>{t.slug}</td>
        <td style={{padding:'6px 8px'}}>{t.name}</td>
        <td style={{padding:'6px 8px',color:t.status==='suspended'?C.danger:C.ok}}>{t.status}</td>
        <td style={{padding:'6px 8px'}}>
          {t.status==='active'
            ? <button style={{...btn,background:C.danger}} onClick={()=>setStatus(t.id,'suspend')}>Suspend</button>
            : <button style={{...btn,background:C.ok}} onClick={()=>setStatus(t.id,'restore')}>Restore</button>}
        </td></tr>)}</tbody>
    </table>
    <form onSubmit={create} style={{display:'flex',gap:8,marginTop:12,flexWrap:'wrap'}}>
      <input style={inp} placeholder="Название" value={name} onChange={e=>setName(e.target.value)} required/>
      <input style={inp} placeholder="slug" value={slug} onChange={e=>setSlug(e.target.value)} required/>
      <button style={btn} type="submit">Создать</button>
    </form>
    {err && <div style={{color:C.danger,marginTop:8}}>{err}</div>}
  </div>;
}

function MembersSection({tenants, users}) {
  const [tid,setTid]=useState(''); const [members,setMembers]=useState([]);
  const [q,setQ]=useState(''); const [uid,setUid]=useState(''); const [role,setRole]=useState('user'); const [err,setErr]=useState('');
  const loadMembers = useCallback(async (id)=>{ if(!id){setMembers([]);return;} const res=await get(`/api/v1/system/tenants/${id}/members`); if(res&&res.ok) setMembers(await res.json()); },[]);
  useEffect(()=>{ loadMembers(tid); },[tid,loadMembers]);
  const attach = async (e)=>{ e.preventDefault(); setErr('');
    if(!tid||!uid){ setErr('Выберите тенант и пользователя'); return; }
    const res = await post(`/api/v1/system/tenants/${tid}/members`, {user_id:Number(uid), role});
    if (res.status===201){ setUid(''); setQ(''); loadMembers(tid); } else setErr(await errMsg(res));
  };
  const filtered = (users||[]).filter(u=>{ const s=(u.display_name+' '+u.email).toLowerCase(); return s.includes(q.toLowerCase()); }).slice(0,50);
  return <div style={box}>
    <h2 style={{fontSize:15,marginBottom:10}}>Участники</h2>
    <select style={inp} value={tid} onChange={e=>setTid(e.target.value)}>
      <option value="">— выберите тенант —</option>
      {(tenants||[]).map(t=><option key={t.id} value={t.id}>{t.name} ({t.slug})</option>)}
    </select>
    {tid && <table style={{width:'100%',borderCollapse:'collapse',marginTop:12}}>
      <thead><tr>{['Имя','Email','Роль','Статус'].map(h=><th key={h} style={{textAlign:'left',padding:'6px 8px',borderBottom:'1px solid '+C.border}}>{h}</th>)}</tr></thead>
      <tbody>{members.map(m=><tr key={m.user_id}>
        <td style={{padding:'6px 8px'}}>{m.display_name}</td><td style={{padding:'6px 8px'}}>{m.email}</td>
        <td style={{padding:'6px 8px'}}>{m.role}</td><td style={{padding:'6px 8px',color:m.status==='requested'?C.muted:C.ok}}>{m.status}</td>
      </tr>)}</tbody>
    </table>}
    {tid && <form onSubmit={attach} style={{display:'flex',gap:8,marginTop:12,flexWrap:'wrap',alignItems:'center'}}>
      <input style={inp} placeholder="поиск пользователя" value={q} onChange={e=>setQ(e.target.value)}/>
      <select style={inp} value={uid} onChange={e=>setUid(e.target.value)}>
        <option value="">— пользователь —</option>
        {filtered.map(u=><option key={u.id} value={u.id}>{u.display_name} {u.email?'· '+u.email:''}</option>)}
      </select>
      <select style={inp} value={role} onChange={e=>setRole(e.target.value)}><option value="user">user</option><option value="admin">admin</option></select>
      <button style={btn} type="submit">Подключить</button>
    </form>}
    {err && <div style={{color:C.danger,marginTop:8}}>{err}</div>}
  </div>;
}

function RegistrationSection({tenants}) {
  const [val,setVal]=useState(''); const [msg,setMsg]=useState('');
  useEffect(()=>{ (async()=>{ const res=await get('/api/v1/system/settings'); if(res&&res.ok){ const j=await res.json(); setVal(j.default_registration_tenant_id==null?'':String(j.default_registration_tenant_id)); } })(); },[]);
  const save = async ()=>{ setMsg(''); const res=await put('/api/v1/system/settings/default-registration-tenant', {tenant_id: val===''?null:Number(val)}); setMsg(res.status===204?'Сохранено':await errMsg(res)); };
  return <div style={box}>
    <h2 style={{fontSize:15,marginBottom:10}}>Тенант регистрации по умолчанию</h2>
    <div style={{color:C.muted,marginBottom:8}}>Куда попадает новый пользователь без приглашения. «Нет» → страница-заглушка.</div>
    <div style={{display:'flex',gap:8,alignItems:'center'}}>
      <select style={inp} value={val} onChange={e=>setVal(e.target.value)}>
        <option value="">— нет —</option>
        {(tenants||[]).map(t=><option key={t.id} value={t.id}>{t.name} ({t.slug})</option>)}
      </select>
      <button style={btn} onClick={save}>Сохранить</button>
      {msg && <span style={{color:msg==='Сохранено'?C.ok:C.danger}}>{msg}</span>}
    </div>
  </div>;
}

const KNOWN_ENT = ['sso','subdomains','file_uploads','max_users'];
function EntitlementsSection({tenants}) {
  const [tid,setTid]=useState(''); const [ent,setEnt]=useState({}); const [k,setK]=useState(''); const [v,setV]=useState(''); const [msg,setMsg]=useState('');
  const load = useCallback(async(id)=>{ if(!id){setEnt({});return;} const res=await get(`/api/v1/system/tenants/${id}/entitlements`); if(res&&res.ok) setEnt(await res.json()||{}); },[]);
  useEffect(()=>{ load(tid); },[tid,load]);
  const save = async (key, value)=>{ setMsg(''); const res=await put(`/api/v1/system/tenants/${tid}/entitlements`, {[key]: value}); if(res.status===204){ load(tid); setMsg('Сохранено'); } else setMsg(await errMsg(res)); };
  return <div style={box}>
    <h2 style={{fontSize:15,marginBottom:6}}>Entitlements</h2>
    <div style={{color:C.muted,marginBottom:10}}>В OSS не ограничивают (всё включено); запись — задел для SaaS-сборки.</div>
    <select style={inp} value={tid} onChange={e=>setTid(e.target.value)}>
      <option value="">— выберите тенант —</option>
      {(tenants||[]).map(t=><option key={t.id} value={t.id}>{t.name} ({t.slug})</option>)}
    </select>
    {tid && <div style={{marginTop:12,display:'flex',flexDirection:'column',gap:8}}>
      {KNOWN_ENT.map(key=>{
        const cur = ent[key];
        const isNum = key==='max_users';
        return <div key={key} style={{display:'flex',gap:8,alignItems:'center'}}>
          <code style={{minWidth:140}}>entitlement.{key}</code>
          {isNum
            ? <input style={inp} type="number" defaultValue={cur!=null?cur:''} onBlur={e=>save(key, Number(e.target.value))}/>
            : <button style={{...btn,background: cur===true?C.ok:C.muted}} onClick={()=>save(key, cur!==true)}>{cur===true?'on':'off'}</button>}
        </div>;
      })}
      <div style={{display:'flex',gap:8,marginTop:8,flexWrap:'wrap'}}>
        <input style={inp} placeholder="ключ (без entitlement.)" value={k} onChange={e=>setK(e.target.value)}/>
        <input style={inp} placeholder='значение JSON (напр. true / 50 / "x")' value={v} onChange={e=>setV(e.target.value)}/>
        <button style={btn} onClick={()=>{ try{ save(k, JSON.parse(v)); }catch{ setMsg('Значение должно быть валидным JSON'); } }}>Сохранить ключ</button>
      </div>
      {msg && <div style={{color:msg.startsWith('Сохранено')?C.ok:C.danger}}>{msg}</div>}
    </div>}
  </div>;
}

function App() {
  const [tenants,setTenants]=useState([]); const [users,setUsers]=useState([]); const [tab,setTab]=useState('tenants');
  const reloadTenants = useCallback(async()=>{ const res=await get('/api/v1/system/tenants'); if(res&&res.ok) setTenants(await res.json()||[]); },[]);
  useEffect(()=>{ reloadTenants(); (async()=>{ const res=await get('/api/v1/system/users'); if(res&&res.ok) setUsers(await res.json()||[]); })(); },[reloadTenants]);
  const tabBtn = (id,label)=><button onClick={()=>setTab(id)} style={{...btn,background:tab===id?C.accent:'#94a3b8'}}>{label}</button>;
  return <div style={{maxWidth:920,margin:'0 auto',padding:'24px 16px'}}>
    <h1 style={{fontSize:20,marginBottom:16}}>Система · Управление</h1>
    <div style={{display:'flex',gap:8,marginBottom:16,flexWrap:'wrap'}}>
      {tabBtn('tenants','Тенанты')}{tabBtn('members','Участники')}{tabBtn('registration','Регистрация')}{tabBtn('entitlements','Entitlements')}
    </div>
    {tab==='tenants' && <TenantsSection tenants={tenants} reload={reloadTenants}/>}
    {tab==='members' && <MembersSection tenants={tenants} users={users}/>}
    {tab==='registration' && <RegistrationSection tenants={tenants}/>}
    {tab==='entitlements' && <EntitlementsSection tenants={tenants}/>}
  </div>;
}

ReactDOM.createRoot(document.getElementById('root')).render(<App/>);
```

- [ ] **Step 3: Build + smoke** `go build ./...` → green (templates embed via `//go:embed`, so a
  broken template name fails the build/test). Verify the template parses:
  `go test ./internal/http/... -run TestRoutes 2>/dev/null || go build ./...` (the embed + parse
  happens in `parseTemplates`; any other server test exercising `Routes()` covers it).

- [ ] **Step 4: Manual verification (DoD, `specs/010`)** — run the app with a system-admin (or
  `BOOTSTRAP_SYSTEM_ADMIN`) or `PROVISIONING_TOKEN`, open `/system`: create a tenant; on the
  Members tab pick the tenant, search a user, attach as admin → appears in the list; set the
  default registration tenant → reload shows it; toggle `entitlement.sso` → reload shows `on`.

- [ ] **Step 5: Stage** `git add internal/http/templates/system_shell.html internal/web/static/system.js`
  (message: `feat(system): React management panel for tenants, members, registration, entitlements`).

---

### Task 6: API spec update

**Files:**
- Modify: `specs/040-api-contract.md` (System API section)

- [ ] **Step 1: Document the three new GETs + the gate-everywhere rule** — under the existing
  "System API endpoints" section, add:

```markdown
- `GET /api/v1/system/tenants/{id}/members` — участники тенанта (`[{user_id, display_name, email, role, status}]`, все статусы).
- `GET /api/v1/system/tenants/{id}/entitlements` — текущие `entitlement.*` ключи (префикс срезан): `{ "sso": true, "max_users": 50 }`.
- `GET /api/v1/system/settings` — глобальные system-настройки для UI: `{ "default_registration_tenant_id": <int|null> }`.

Авторизация на `/api/v1/system/*` и `/system` обязательна **во всех режимах**, включая
`AUTH_MODE=disabled` (там — только по `PROVISIONING_TOKEN`; `anonymous-local` не system-admin).
```

- [ ] **Step 2: Run** `go build ./...` (docs-only sanity) → green.
- [ ] **Step 3: Stage** `git add specs/040-api-contract.md`
  (message: `docs(specs): system read endpoints + gate-everywhere rule`).

---

## Self-Review Notes

- **Spec coverage:** React panel + shell (Task 5); 4 sections map to endpoints. New reads:
  members (Task 1+3), entitlements (Task 2+3), settings (Task 3). Gate-everywhere (Task 4).
  Specs (Task 6). Client-side user search, OSS entitlements caveat, no-remove-member, no-rename —
  all honored (search filter in `MembersSection`; caveat label in `EntitlementsSection`; no remove
  /rename UI).
- **Type consistency:** `AccessRequest.Status` added in Task 1 and consumed in Task 3's
  `HandleListMembers`; `SystemSettings` interface extended in Task 3 matches `*SettingsService`
  methods from Task 2 + existing `SystemGet/SystemSet`. `New(...)` 5-arg constructor used
  consistently in Task 3 tests and Task 4 wiring.
- **Behaviour change:** dropping the disabled-mode bypass (Task 4) is gated by its own test +
  full-suite run. Existing system handler tests build their own router with the gate, so they are
  unaffected.
- **No DB migration:** confirmed — only read methods over existing tables.

## Execution recommendation

Inline, compiler-driven, one task at a time. Tasks 1–4 + 6 are TDD/Go with green gates; Task 5 is
frontend (manual DoD, no auto-tests — matches project practice). After each task:
`go build ./... && go vet ./... && go test ./...` green (Task 5: build + manual), then `git add` +
propose a commit message; the user commits.
