# Tenant access & membership management — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Grant default access when a self-requested membership is approved; let system-admins change member roles and grant/revoke system privileges; give users a "Мои пространства" section in /settings to leave tenants and request to join by slug.

**Architecture:** Reuse the existing `OnboardingService.applyNewUserPolicy` on both approval paths (tenant-admin + system-admin). Add two system-plane operations (`SetMemberRole`, `SetSystemAdmin`) to `ProvisioningService` with last-admin / self-lockout guards. Add session-plane endpoints (`GET/DELETE /api/v1/session/memberships`) backed by a new join query and an `OnboardingService.LeaveTenant`. Frontend: extend the two React SPAs (`system.js`, `settings.js`).

**Tech Stack:** Go 1.x, chi router, pgx/pgxpool, PostgreSQL; React 18 (CDN + Babel standalone) for `/system` and `/settings` SPAs. Tests are Go integration tests using `internal/store/testutil.SetupDB(t)` against a real Postgres (seeds tenant id=1, slug `default`).

## Global Constraints

- No new DB migrations: all columns already exist (`memberships.role`, `memberships.status`, `users.is_system_admin`, `tenants.slug`). Do not add migrations; do not touch `seedDemo` (no table-structure change). — per CLAUDE.md rule 7.
- Layered/clean architecture: handlers → services → repositories. Do not leak SQL into handlers or HTTP into services. — CLAUDE.md rule 6.
- No queries in loops; use single join queries. — CLAUDE.md rule 9.
- Keep specs in sync in this same change set: `specs/040-api-contract.md`, `specs/050-permissions-and-lifecycle.md` (Task 16). Do not modify unrelated specs.
- Commit messages / comments: no mention of AI/assistants/generated-by. — CLAUDE.md rule 5.
- Run `go build ./...`, `go vet ./...`, and `go test ./...` (or the targeted package) as verification. Frontend SPA changes are verified by `go build ./...` (embedded assets) + manual load; there is no JS test harness.
- Guard error → HTTP mapping (used across tasks): `memberships.ErrNotFound` → `404`; `service.ErrLastAdmin` / `service.ErrLastSystemAdmin` → `409`; `service.ErrSelfLockout` → `409`.
- Design spec: `docs/superpowers/specs/2026-07-02-tenant-access-and-membership-management-design.md`.

---

## Task 1: Apply default access on tenant-admin approval

**Files:**
- Modify: `internal/service/onboarding.go` (`ApproveRequest` ~L139; add exported `ApplyDefaultAccess`)
- Test: `internal/service/onboarding_test.go`

**Interfaces:**
- Produces: `func (s *OnboardingService) ApplyDefaultAccess(ctx context.Context, scope domain.TenantScope, userID int64) error` (exported wrapper over `applyNewUserPolicy`, for reuse by `ProvisioningService`).
- Behavior change: `ApproveRequest` grants the tenant's `default_hierarchy_node_id` when `new_user_policy = "default_node"` and the user has no grant yet (existing `applyNewUserPolicy` guard).

- [ ] **Step 1: Write the failing test**

Add to `internal/service/onboarding_test.go`. Follows the existing `TestJoinRequestApproveDeny` style (tenant id=1, slug `default`).

```go
func TestApproveRequestAppliesDefaultAccess(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	svc := newOnboardingForTest(t, pool)
	settingsSvc := newSettingsForTest(t, pool) // see note below
	grantRepo := grants.NewGrantRepository(pool)

	// Configure tenant 1 default-node policy pointing at a seeded team.
	var teamID int64
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name, type) VALUES ('Root','department') RETURNING id`).Scan(&teamID); err != nil {
		t.Fatalf("seed team: %v", err)
	}
	scope := domain.TenantScope{TenantID: 1}
	if err := settingsSvc.SetTenantProduct(ctx, scope, "new_user_policy", "default_node"); err != nil {
		t.Fatalf("set policy: %v", err)
	}
	if err := settingsSvc.SetTenantProduct(ctx, scope, "default_hierarchy_node_id", teamID); err != nil {
		t.Fatalf("set node: %v", err)
	}

	var uid int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('github:a','github','a','A') RETURNING id`).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := svc.RequestAccess(ctx, "default", uid); err != nil {
		t.Fatalf("request: %v", err)
	}
	if err := svc.ApproveRequest(ctx, scope, uid); err != nil {
		t.Fatalf("approve: %v", err)
	}

	gs, err := grantRepo.ListUserGrants(ctx, scope, uid)
	if err != nil {
		t.Fatalf("list grants: %v", err)
	}
	if len(gs) != 1 || gs[0].TeamID != teamID {
		t.Fatalf("grants = %+v, want one grant on team %d", gs, teamID)
	}
}
```

If a `newSettingsForTest` helper does not already exist in the test file, add this helper (mirrors the wiring inside `newOnboardingForTest`):

```go
func newSettingsForTest(t *testing.T, pool *pgxpool.Pool) *service.SettingsService {
	t.Helper()
	tsRepo := tenantsettings.NewTenantSettingsRepository(pool)
	sysRepo := settings.NewSettingsRepository(pool)
	return service.NewSettingsService(
		tenantsettings.NewTenantSettingsCache(tsRepo), tsRepo,
		settings.NewSystemSettingsCache(sysRepo), sysRepo,
	)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/ -run TestApproveRequestAppliesDefaultAccess -v`
Expected: FAIL — `grants = []` (approval does not yet apply the policy).

- [ ] **Step 3: Implement**

In `internal/service/onboarding.go`, add the exported wrapper and call it from `ApproveRequest`:

```go
// ApplyDefaultAccess applies the tenant's new-user policy (default-node grant) to a user if they
// have no grant there yet. Exported so the system-admin plane (ProvisioningService) can reuse the
// exact same rule when it activates/attaches a member.
func (s *OnboardingService) ApplyDefaultAccess(ctx context.Context, scope domain.TenantScope, userID int64) error {
	return s.applyNewUserPolicy(ctx, scope, userID)
}
```

Change `ApproveRequest` to apply the policy after activation:

```go
// ApproveRequest activates a pending membership and applies the tenant's default-access policy
// (same baseline a user gets on auto-registration / invite), if they have no grant there yet.
func (s *OnboardingService) ApproveRequest(ctx context.Context, scope domain.TenantScope, userID int64) error {
	if err := s.mem.SetStatus(ctx, userID, scope.TenantID, domain.MembershipActive); err != nil {
		return err
	}
	s.memCache.InvalidateUser(userID)
	return s.applyNewUserPolicy(ctx, scope, userID)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/service/ -run 'TestApproveRequest|TestJoinRequestApproveDeny' -v`
Expected: PASS (both the new test and the existing approve/deny test).

- [ ] **Step 5: Commit**

```bash
git add internal/service/onboarding.go internal/service/onboarding_test.go
git commit -m "feat(onboarding): apply default access policy on request approval"
```

---

## Task 2: Apply default access on system-admin attach

**Files:**
- Modify: `internal/service/provisioning.go` (add `defaultAccess` dependency; call it in `AttachMember`)
- Modify: `internal/http/server.go` (~L178 `NewProvisioningService` wiring)
- Test: `internal/service/provisioning_test.go`

**Interfaces:**
- Consumes: `OnboardingService.ApplyDefaultAccess` (Task 1).
- Produces: `NewProvisioningService(...)` gains a trailing `defaultAccess defaultAccessApplier` parameter, where `defaultAccessApplier interface { ApplyDefaultAccess(ctx context.Context, scope domain.TenantScope, userID int64) error }`.

- [ ] **Step 1: Write the failing test**

Add to `internal/service/provisioning_test.go` (mirror its existing `newProvisioningForTest` helper — if the helper doesn't pass a default-access applier yet, update it to pass the real `OnboardingService`).

```go
func TestAttachMemberAppliesDefaultAccess(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	prov := newProvisioningForTest(t, pool)
	settingsSvc := newSettingsForTest(t, pool)
	grantRepo := grants.NewGrantRepository(pool)

	var teamID int64
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name, type) VALUES ('Root','department') RETURNING id`).Scan(&teamID); err != nil {
		t.Fatalf("seed team: %v", err)
	}
	scope := domain.TenantScope{TenantID: 1}
	_ = settingsSvc.SetTenantProduct(ctx, scope, "new_user_policy", "default_node")
	_ = settingsSvc.SetTenantProduct(ctx, scope, "default_hierarchy_node_id", teamID)

	var uid int64
	_ = pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('github:b','github','b','B') RETURNING id`).Scan(&uid)

	if _, err := prov.AttachMember(ctx, 1, uid, domain.RoleUser); err != nil {
		t.Fatalf("attach: %v", err)
	}
	gs, err := grantRepo.ListUserGrants(ctx, scope, uid)
	if err != nil {
		t.Fatalf("list grants: %v", err)
	}
	if len(gs) != 1 || gs[0].TeamID != teamID {
		t.Fatalf("grants = %+v, want one grant on team %d", gs, teamID)
	}
}
```

Update `newProvisioningForTest` to build and pass an `OnboardingService` as the `defaultAccess` argument (it already constructs the needed repos; reuse `newOnboardingForTest(t, pool)`).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/ -run TestAttachMemberAppliesDefaultAccess -v`
Expected: FAIL to compile (missing constructor arg) or FAIL `grants = []`.

- [ ] **Step 3: Implement**

In `internal/service/provisioning.go`:

```go
// defaultAccessApplier applies a tenant's new-user policy to a member. *OnboardingService satisfies it.
type defaultAccessApplier interface {
	ApplyDefaultAccess(ctx context.Context, scope domain.TenantScope, userID int64) error
}
```

Add the field to `ProvisioningService` and constructor:

```go
type ProvisioningService struct {
	tenants       *tenants.TenantRepository
	tenantCache   *tenants.TenantCache
	members       *memberships.MembershipRepository
	memberCache   *memberships.MembershipCache
	settings      *SettingsService
	grants        grantRemover
	defaultAccess defaultAccessApplier
}

func NewProvisioningService(
	tnRepo *tenants.TenantRepository, tenantCache *tenants.TenantCache,
	memRepo *memberships.MembershipRepository, memberCache *memberships.MembershipCache,
	settings *SettingsService, grants grantRemover, defaultAccess defaultAccessApplier,
) *ProvisioningService {
	return &ProvisioningService{
		tenants: tnRepo, tenantCache: tenantCache, members: memRepo, memberCache: memberCache,
		settings: settings, grants: grants, defaultAccess: defaultAccess,
	}
}
```

Apply the policy at the end of `AttachMember` (after `InvalidateUser`):

```go
	p.memberCache.InvalidateUser(userID)
	if err := p.defaultAccess.ApplyDefaultAccess(ctx, domain.TenantScope{TenantID: tenantID}, userID); err != nil {
		return nil, err
	}
	return m, nil
```

In `internal/http/server.go`, `onboardingSvc` is constructed just after `provisioning`. Reorder so `onboardingSvc` is built first, then pass it into `NewProvisioningService`:

```go
	onboardingSvc := service.NewOnboardingService(
		st.Invitations, st.Memberships, membershipCache, st.Tenants, settingsSvc, grantsCache,
	)
	provisioning := service.NewProvisioningService(
		st.Tenants, tenantCache,
		st.Memberships, membershipCache,
		settingsSvc, grantsCache, onboardingSvc,
	)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go build ./... && go test ./internal/service/ -run 'TestAttachMember' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service/provisioning.go internal/service/provisioning_test.go internal/http/server.go
git commit -m "feat(provisioning): apply default access policy on system-admin attach"
```

---

## Task 3: Count active admins (repository)

**Files:**
- Modify: `internal/store/memberships/memberships.go`
- Test: `internal/store/memberships/memberships_test.go` (create if absent)

**Interfaces:**
- Produces: `func (r *MembershipRepository) CountActiveAdmins(ctx context.Context, scope domain.TenantScope) (int, error)` — number of `role='admin' AND status='active'` memberships in the tenant.

- [ ] **Step 1: Write the failing test**

```go
func TestCountActiveAdmins(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := memberships.NewMembershipRepository(pool)
	scope := domain.TenantScope{TenantID: 1}

	seat := func(sub string, role domain.Role, status domain.MembershipStatus) {
		var uid int64
		_ = pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
			VALUES ($1,'github',$2,$2) RETURNING id`, "github:"+sub, sub).Scan(&uid)
		_, _ = repo.Upsert(ctx, domain.Membership{UserID: uid, TenantID: 1, Role: role, Status: status})
	}
	seat("a1", domain.RoleAdmin, domain.MembershipActive)
	seat("a2", domain.RoleAdmin, domain.MembershipActive)
	seat("u1", domain.RoleUser, domain.MembershipActive)
	seat("r1", domain.RoleAdmin, domain.MembershipRequested) // requested admin doesn't count

	n, err := repo.CountActiveAdmins(ctx, scope)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Fatalf("count = %d, want 2", n)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/memberships/ -run TestCountActiveAdmins -v`
Expected: FAIL — `CountActiveAdmins` undefined.

- [ ] **Step 3: Implement**

Add to `internal/store/memberships/memberships.go`:

```go
// CountActiveAdmins returns how many active admins the tenant has (last-admin guard input).
func (r *MembershipRepository) CountActiveAdmins(ctx context.Context, scope domain.TenantScope) (int, error) {
	var n int
	err := r.db.QueryRow(ctx,
		`SELECT count(*) FROM memberships WHERE tenant_id = $1 AND role = 'admin' AND status = 'active'`,
		scope.TenantID).Scan(&n)
	return n, err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/memberships/ -run TestCountActiveAdmins -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/memberships/memberships.go internal/store/memberships/memberships_test.go
git commit -m "feat(memberships): add CountActiveAdmins"
```

---

## Task 4: SetMemberRole with last-admin guard (service)

**Files:**
- Modify: `internal/service/provisioning.go` (add `SetMemberRole`; define `ErrLastAdmin`)
- Test: `internal/service/provisioning_test.go`

**Interfaces:**
- Consumes: `memberships.CountActiveAdmins` (Task 3); `members.Get`, `members.SetRole`.
- Produces:
  - `var ErrLastAdmin = errors.New("service: cannot remove the tenant's last admin")`
  - `func (p *ProvisioningService) SetMemberRole(ctx context.Context, tenantID, userID int64, role domain.Role) error`

- [ ] **Step 1: Write the failing test**

```go
func TestSetMemberRoleLastAdminGuard(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	prov := newProvisioningForTest(t, pool)
	mem := memberships.NewMembershipRepository(pool)

	var admin int64
	_ = pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('github:sole','github','sole','Sole') RETURNING id`).Scan(&admin)
	_, _ = mem.Upsert(ctx, domain.Membership{UserID: admin, TenantID: 1, Role: domain.RoleAdmin, Status: domain.MembershipActive})

	// Demoting the only admin must be refused.
	if err := prov.SetMemberRole(ctx, 1, admin, domain.RoleUser); !errors.Is(err, service.ErrLastAdmin) {
		t.Fatalf("err = %v, want ErrLastAdmin", err)
	}

	// With a second admin present, demotion is allowed.
	var admin2 int64
	_ = pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('github:two','github','two','Two') RETURNING id`).Scan(&admin2)
	_, _ = mem.Upsert(ctx, domain.Membership{UserID: admin2, TenantID: 1, Role: domain.RoleAdmin, Status: domain.MembershipActive})
	if err := prov.SetMemberRole(ctx, 1, admin, domain.RoleUser); err != nil {
		t.Fatalf("demote with 2 admins: %v", err)
	}
	m, _ := mem.Get(ctx, admin, 1)
	if m.Role != domain.RoleUser {
		t.Fatalf("role = %q, want user", m.Role)
	}

	// Unknown membership → ErrNotFound.
	if err := prov.SetMemberRole(ctx, 1, 999999, domain.RoleUser); !errors.Is(err, memberships.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/ -run TestSetMemberRoleLastAdminGuard -v`
Expected: FAIL — `SetMemberRole` / `ErrLastAdmin` undefined.

- [ ] **Step 3: Implement**

In `internal/service/provisioning.go` add the error (near the top with other declarations) and method:

```go
var ErrLastAdmin = errors.New("service: cannot remove the tenant's last admin")

// SetMemberRole changes a member's tenant role. Refuses to demote the tenant's last active admin
// (ErrLastAdmin) so a tenant never ends up with zero admins. Unknown membership → memberships.ErrNotFound.
func (p *ProvisioningService) SetMemberRole(ctx context.Context, tenantID, userID int64, role domain.Role) error {
	scope := domain.TenantScope{TenantID: tenantID}
	cur, err := p.members.Get(ctx, userID, tenantID)
	if err != nil {
		return err // memberships.ErrNotFound bubbles up
	}
	if cur.Role == domain.RoleAdmin && role != domain.RoleAdmin && cur.Status == domain.MembershipActive {
		n, err := p.members.CountActiveAdmins(ctx, scope)
		if err != nil {
			return err
		}
		if n <= 1 {
			return ErrLastAdmin
		}
	}
	if err := p.members.SetRole(ctx, scope, userID, role); err != nil {
		return err
	}
	p.memberCache.InvalidateUser(userID)
	return nil
}
```

Add `"errors"` to the import block if not present.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/service/ -run TestSetMemberRoleLastAdminGuard -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service/provisioning.go internal/service/provisioning_test.go
git commit -m "feat(provisioning): SetMemberRole with last-admin guard"
```

---

## Task 5: System endpoint — change member role

**Files:**
- Modify: `internal/http/handlers/api/v1/system/handler.go` (extend `Provisioner`; add `HandleSetMemberRole`)
- Modify: `internal/http/server.go` (register route in `registerSystemRoutes`)
- Test: `internal/http/handlers/api/v1/system/handler_test.go` (create if absent)

**Interfaces:**
- Consumes: `ProvisioningService.SetMemberRole` (Task 4).
- Produces: route `PUT /api/v1/system/tenants/{id}/members/{userID}/role`, body `{"role":"user"|"admin"}` → `204`; `422` invalid role; `404` unknown membership; `409` last-admin.

- [ ] **Step 1: Write the failing test**

Write an httptest-based handler test. Use a small fake `Provisioner` implementing only the methods the handler calls (return canned errors to exercise mapping). Model it on any existing handler test in the repo; the essential assertions:

```go
func TestHandleSetMemberRole(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		setErr error
		want   int
	}{
		{"ok", `{"role":"admin"}`, nil, http.StatusNoContent},
		{"invalid role", `{"role":"boss"}`, nil, http.StatusUnprocessableEntity},
		{"not found", `{"role":"user"}`, memberships.ErrNotFound, http.StatusNotFound},
		{"last admin", `{"role":"user"}`, service.ErrLastAdmin, http.StatusConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := system.New(&fakeProv{setRoleErr: tc.setErr}, nil, nil, nil, nil)
			r := chi.NewRouter()
			r.Put("/api/v1/system/tenants/{id}/members/{userID}/role", h.HandleSetMemberRole)
			req := httptest.NewRequest("PUT", "/api/v1/system/tenants/1/members/2/role", strings.NewReader(tc.body))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d", w.Code, tc.want)
			}
		})
	}
}
```

Define `fakeProv` in the test file implementing the full `system` `Provisioner` interface (only `SetMemberRole` needs behavior; the rest can return `nil`/zero). Its `SetMemberRole` returns `tc.setErr`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/http/handlers/api/v1/system/ -run TestHandleSetMemberRole -v`
Expected: FAIL — `HandleSetMemberRole` undefined / interface missing method.

- [ ] **Step 3: Implement**

Add `SetMemberRole` to the `Provisioner` interface in `handler.go`:

```go
	SetMemberRole(ctx context.Context, tenantID, userID int64, role domain.Role) error
	SetSystemAdmin(ctx context.Context, callerID, targetID int64, isSystemAdmin bool) error
```

(Add both now to avoid a second interface edit in Task 9.)

Add the handler:

```go
// PUT /api/v1/system/tenants/{id}/members/{userID}/role  {role}
func (h *Handler) HandleSetMemberRole(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := pathID(w, r)
	if !ok {
		return
	}
	userID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	if err != nil || userID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var body struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	role := domain.Role(body.Role)
	if role != domain.RoleAdmin && role != domain.RoleUser {
		writeError(w, http.StatusUnprocessableEntity, "invalid role")
		return
	}
	switch err := h.prov.SetMemberRole(r.Context(), tenantID, userID, role); {
	case errors.Is(err, memberships.ErrNotFound):
		writeError(w, http.StatusNotFound, "membership not found")
	case errors.Is(err, service.ErrLastAdmin):
		writeError(w, http.StatusConflict, "cannot demote the last admin")
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error())
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
```

Add `"okrs/internal/service"` to the imports.

Register the route in `internal/http/server.go` `registerSystemRoutes`, next to the other members routes:

```go
		r.Put("/api/v1/system/tenants/{id}/members/{userID}/role", sysH.HandleSetMemberRole)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go build ./... && go test ./internal/http/handlers/api/v1/system/ -run TestHandleSetMemberRole -v`
Expected: PASS. (Build will fail until Task 9's `SetSystemAdmin` exists on the concrete `*ProvisioningService`; if you implement tasks in order, add a temporary stub or do Task 8 before building. To keep the build green, implement Task 8's `SetSystemAdmin` method before running `go build ./...` here — the interface now references it.)

> Ordering note: because Step 3 adds `SetSystemAdmin` to the `Provisioner` interface, `*ProvisioningService` must have that method for `apisystem.New(s.provisioning, …)` to compile. Do **Task 8 Step 3 (implement `SetSystemAdmin`)** before the first full `go build ./...`. Package-level tests for `system` still pass in isolation because the test uses `fakeProv`.

- [ ] **Step 5: Commit**

```bash
git add internal/http/handlers/api/v1/system/handler.go internal/http/handlers/api/v1/system/handler_test.go internal/http/server.go
git commit -m "feat(system): endpoint to change a member's tenant role"
```

---

## Task 6: /system Members UI — role control

**Files:**
- Modify: `internal/web/static/system.js` (`MembersSection`, ~L55-101)

**Interfaces:**
- Consumes: `PUT /api/v1/system/tenants/{id}/members/{userID}/role` (Task 5); existing `put`/`errMsg` helpers.

- [ ] **Step 1: Add a role control to each active member row**

In `MembersSection`, for each member row render a `<select>` bound to the member's role that calls the endpoint on change and refetches the member list. Use existing helpers (`put`, `errMsg`) and the module's style constants.

```jsx
async function changeRole(userID, role) {
  const res = await put(`/api/v1/system/tenants/${tenantId}/members/${userID}/role`, {role});
  if (!res) return;
  if (!res.ok) { setErr(await errMsg(res)); return; }
  await reload(); // re-run the existing members fetch for this tenant
}
```

In the row JSX (replace the static role text with the control):

```jsx
<select value={m.role} onChange={e => changeRole(m.user_id, e.target.value)} style={inp}>
  <option value="user">user</option>
  <option value="admin">admin</option>
</select>
```

Surface the returned error inline (e.g. "cannot demote the last admin" → the `409` body) near the members table.

- [ ] **Step 2: Verify build embeds the asset and the page loads**

Run: `go build ./...`
Then run the app and open `/system`, pick a tenant with ≥2 admins, change a role, confirm it persists after reload; try demoting the sole admin and confirm the inline error appears.
Expected: role change persists; last-admin demotion shows a conflict message.

- [ ] **Step 3: Commit**

```bash
git add internal/web/static/system.js
git commit -m "feat(system-ui): change member role from members tab"
```

---

## Task 7: Count system admins + not-found on set (repository)

**Files:**
- Modify: `internal/store/users/users.go` (`SetSystemAdmin`; add `CountSystemAdmins`, `ErrNotFound`)
- Test: `internal/store/users/users_test.go` (create if absent)

**Interfaces:**
- Produces:
  - `var ErrNotFound = errors.New("users: not found")`
  - `func (r *UserRepository) CountSystemAdmins(ctx context.Context) (int, error)`
  - `SetSystemAdmin` now returns `ErrNotFound` when no row is updated.

- [ ] **Step 1: Write the failing test**

```go
func TestSystemAdminCountAndSet(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := users.NewUserRepository(pool)

	var uid int64
	_ = pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('github:sa','github','sa','SA') RETURNING id`).Scan(&uid)

	if n, _ := repo.CountSystemAdmins(ctx); n != 0 {
		t.Fatalf("count = %d, want 0", n)
	}
	if err := repo.SetSystemAdmin(ctx, uid, true); err != nil {
		t.Fatalf("set: %v", err)
	}
	if n, _ := repo.CountSystemAdmins(ctx); n != 1 {
		t.Fatalf("count = %d, want 1", n)
	}
	if err := repo.SetSystemAdmin(ctx, 999999, true); !errors.Is(err, users.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/users/ -run TestSystemAdminCountAndSet -v`
Expected: FAIL — `CountSystemAdmins`/`ErrNotFound` undefined.

- [ ] **Step 3: Implement**

In `internal/store/users/users.go` add the error (top of file, with imports for `errors`):

```go
var ErrNotFound = errors.New("users: not found")
```

Update `SetSystemAdmin` to detect missing rows, and add the counter:

```go
// SetSystemAdmin sets the tenant-less instance superadmin flag. ErrNotFound if the user is missing.
func (r *UserRepository) SetSystemAdmin(ctx context.Context, userID int64, v bool) error {
	ct, err := r.db.Exec(ctx, `UPDATE users SET is_system_admin = $1, updated_at = NOW() WHERE id = $2`, v, userID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CountSystemAdmins returns how many instance system-admins exist (last-admin guard input).
func (r *UserRepository) CountSystemAdmins(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `SELECT count(*) FROM users WHERE is_system_admin`).Scan(&n)
	return n, err
}
```

Verify the `maybeBootstrapSystemAdmin` caller (`internal/auth/manager.go`) still compiles — it ignores the returned error already; the new `ErrNotFound` path only triggers on a missing user id, which cannot happen there (the user was just upserted).

- [ ] **Step 4: Run test to verify it passes**

Run: `go build ./... && go test ./internal/store/users/ -run TestSystemAdminCountAndSet -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/users/users.go internal/store/users/users_test.go
git commit -m "feat(users): CountSystemAdmins and not-found on SetSystemAdmin"
```

---

## Task 8: SetSystemAdmin with guards (service)

**Files:**
- Modify: `internal/service/provisioning.go` (add `users` dependency; `SetSystemAdmin`; `ErrLastSystemAdmin`, `ErrSelfLockout`)
- Modify: `internal/http/server.go` (`NewProvisioningService` wiring — pass `st.Users`)
- Test: `internal/service/provisioning_test.go`

**Interfaces:**
- Consumes: `users.SetSystemAdmin`, `users.CountSystemAdmins`, `users.ErrNotFound` (Task 7).
- Produces:
  - `var ErrLastSystemAdmin = errors.New("service: cannot revoke the last system admin")`
  - `var ErrSelfLockout = errors.New("service: cannot revoke your own system-admin")`
  - `func (p *ProvisioningService) SetSystemAdmin(ctx context.Context, callerID, targetID int64, v bool) error`
  - `NewProvisioningService` gains a `users systemAdminStore` parameter, where `systemAdminStore interface { SetSystemAdmin(ctx, userID int64, v bool) error; CountSystemAdmins(ctx) (int, error) }`.

- [ ] **Step 1: Write the failing test**

```go
func TestSetSystemAdminGuards(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	prov := newProvisioningForTest(t, pool)
	users := users.NewUserRepository(pool)

	mk := func(sub string) int64 {
		var id int64
		_ = pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
			VALUES ($1,'github',$2,$2) RETURNING id`, "github:"+sub, sub).Scan(&id)
		return id
	}
	a := mk("a")
	b := mk("b")

	// Grant b: allowed.
	if err := prov.SetSystemAdmin(ctx, a, b, true); err != nil {
		t.Fatalf("grant b: %v", err)
	}
	// Revoke sole system-admin b (caller a) → last-admin guard.
	if err := prov.SetSystemAdmin(ctx, a, b, false); !errors.Is(err, service.ErrLastSystemAdmin) {
		t.Fatalf("err = %v, want ErrLastSystemAdmin", err)
	}
	// Grant a too, then a revoking self → self-lockout guard.
	_ = prov.SetSystemAdmin(ctx, a, a, true)
	if err := prov.SetSystemAdmin(ctx, a, a, false); !errors.Is(err, service.ErrSelfLockout) {
		t.Fatalf("err = %v, want ErrSelfLockout", err)
	}
	// Now a can revoke b (two admins, not self).
	if err := prov.SetSystemAdmin(ctx, a, b, false); err != nil {
		t.Fatalf("revoke b: %v", err)
	}
	_ = users
}
```

Update `newProvisioningForTest` to pass `users.NewUserRepository(pool)` as the new `users` argument.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/ -run TestSetSystemAdminGuards -v`
Expected: FAIL — constructor arg / method undefined.

- [ ] **Step 3: Implement**

In `internal/service/provisioning.go`:

```go
// systemAdminStore toggles/counts the instance system-admin flag. *users.UserRepository satisfies it.
type systemAdminStore interface {
	SetSystemAdmin(ctx context.Context, userID int64, v bool) error
	CountSystemAdmins(ctx context.Context) (int, error)
}

var (
	ErrLastSystemAdmin = errors.New("service: cannot revoke the last system admin")
	ErrSelfLockout     = errors.New("service: cannot revoke your own system-admin")
)
```

Add the `users systemAdminStore` field + constructor parameter (append after `defaultAccess`). Then:

```go
// SetSystemAdmin grants/revokes the instance system-admin flag. Refuses to revoke the last remaining
// system-admin (ErrLastSystemAdmin) or the caller's own flag (ErrSelfLockout). Missing user →
// users.ErrNotFound. callerID may be 0 for a machine (provisioning-token) caller — never self.
func (p *ProvisioningService) SetSystemAdmin(ctx context.Context, callerID, targetID int64, v bool) error {
	if !v {
		if callerID != 0 && callerID == targetID {
			return ErrSelfLockout
		}
		n, err := p.users.CountSystemAdmins(ctx)
		if err != nil {
			return err
		}
		if n <= 1 {
			return ErrLastSystemAdmin
		}
	}
	return p.users.SetSystemAdmin(ctx, targetID, v)
}
```

Update the wiring in `internal/http/server.go`:

```go
	provisioning := service.NewProvisioningService(
		st.Tenants, tenantCache,
		st.Memberships, membershipCache,
		settingsSvc, grantsCache, onboardingSvc, st.Users,
	)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go build ./... && go test ./internal/service/ -run TestSetSystemAdminGuards -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service/provisioning.go internal/http/server.go internal/service/provisioning_test.go
git commit -m "feat(provisioning): SetSystemAdmin with last-admin and self-lockout guards"
```

---

## Task 9: System endpoint — grant/revoke system privileges

**Files:**
- Modify: `internal/http/handlers/api/v1/system/handler.go` (`HandleSetSystemAdmin`)
- Modify: `internal/http/server.go` (register route)
- Test: `internal/http/handlers/api/v1/system/handler_test.go`

**Interfaces:**
- Consumes: `ProvisioningService.SetSystemAdmin` (already added to `Provisioner` in Task 5 Step 3); caller identity via `auth.UserFromContext`.
- Produces: route `PUT /api/v1/system/users/{userID}/system-admin`, body `{"is_system_admin":bool}` → `204`; `404` unknown user; `409` last system-admin or self-revoke.

- [ ] **Step 1: Write the failing test**

```go
func TestHandleSetSystemAdmin(t *testing.T) {
	cases := []struct {
		name   string
		setErr error
		want   int
	}{
		{"ok", nil, http.StatusNoContent},
		{"not found", users.ErrNotFound, http.StatusNotFound},
		{"last admin", service.ErrLastSystemAdmin, http.StatusConflict},
		{"self", service.ErrSelfLockout, http.StatusConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := system.New(&fakeProv{sysAdminErr: tc.setErr}, nil, nil, nil, nil)
			r := chi.NewRouter()
			r.Put("/api/v1/system/users/{userID}/system-admin", h.HandleSetSystemAdmin)
			req := httptest.NewRequest("PUT", "/api/v1/system/users/2/system-admin", strings.NewReader(`{"is_system_admin":false}`))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d", w.Code, tc.want)
			}
		})
	}
}
```

Extend `fakeProv` with a `sysAdminErr` field and a `SetSystemAdmin` method returning it.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/http/handlers/api/v1/system/ -run TestHandleSetSystemAdmin -v`
Expected: FAIL — `HandleSetSystemAdmin` undefined.

- [ ] **Step 3: Implement**

Add the handler to `handler.go`:

```go
// PUT /api/v1/system/users/{userID}/system-admin  {is_system_admin}
func (h *Handler) HandleSetSystemAdmin(w http.ResponseWriter, r *http.Request) {
	userID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	if err != nil || userID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var body struct {
		IsSystemAdmin bool `json:"is_system_admin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	var callerID int64
	if u := auth.UserFromContext(r.Context()); u != nil {
		callerID = u.ID
	}
	switch err := h.prov.SetSystemAdmin(r.Context(), callerID, userID, body.IsSystemAdmin); {
	case errors.Is(err, users.ErrNotFound):
		writeError(w, http.StatusNotFound, "user not found")
	case errors.Is(err, service.ErrLastSystemAdmin):
		writeError(w, http.StatusConflict, "cannot revoke the last system admin")
	case errors.Is(err, service.ErrSelfLockout):
		writeError(w, http.StatusConflict, "cannot revoke your own system-admin")
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error())
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
```

Add imports: `"okrs/internal/auth"`, `"okrs/internal/store/users"` (and `"okrs/internal/service"` already added in Task 5).

Register the route in `registerSystemRoutes` (next to `/api/v1/system/users`):

```go
		r.Put("/api/v1/system/users/{userID}/system-admin", sysH.HandleSetSystemAdmin)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go build ./... && go test ./internal/http/handlers/api/v1/system/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/http/handlers/api/v1/system/handler.go internal/http/handlers/api/v1/system/handler_test.go internal/http/server.go
git commit -m "feat(system): endpoint to grant or revoke system privileges"
```

---

## Task 10: /system Users tab — system-admin toggle

**Files:**
- Modify: `internal/web/static/system.js` (new `UsersSection`; add to tab list in `App`)

**Interfaces:**
- Consumes: `GET /api/v1/system/users`, `PUT /api/v1/system/users/{userID}/system-admin` (Task 9), `GET /api/v1/me`.

- [ ] **Step 1: Add a Users section component**

Add a `UsersSection` that loads the user list and the caller's own id, renders a table with a system-admin toggle per row, disables the caller's own row, and shows server errors inline.

```jsx
function UsersSection() {
  const [rows, setRows] = useState([]);
  const [meId, setMeId] = useState(0);
  const [err, setErr] = useState('');
  const reload = useCallback(async () => {
    const res = await get('/api/v1/system/users'); if (!res) return;
    setRows(await res.json());
  }, []);
  useEffect(() => { reload(); (async () => {
    const res = await get('/api/v1/me'); if (res && res.ok) { const me = await res.json(); setMeId(me.id); }
  })(); }, [reload]);
  async function toggle(u) {
    setErr('');
    const res = await put(`/api/v1/system/users/${u.id}/system-admin`, {is_system_admin: !u.is_system_admin});
    if (!res) return;
    if (!res.ok) { setErr(await errMsg(res)); return; }
    reload();
  }
  return (
    <div style={box}>
      <h3>Пользователи</h3>
      {err && <div style={{color: C.danger, marginBottom: 8}}>{err}</div>}
      <table style={{width:'100%', borderCollapse:'collapse'}}>
        <thead><tr><th style={th}>Имя</th><th style={th}>Email</th><th style={th}>System-admin</th></tr></thead>
        <tbody>
          {rows.map(u => (
            <tr key={u.id}>
              <td style={th}>{u.display_name}</td>
              <td style={th}>{u.email}</td>
              <td style={th}>
                <input type="checkbox" checked={!!u.is_system_admin}
                  disabled={u.id === meId}
                  onChange={() => toggle(u)} />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
```

- [ ] **Step 2: Register the tab**

In the `App` component's tab list (the array of `{key,label}` and the switch that renders sections), add a `users` tab labelled `Пользователи` that renders `<UsersSection/>`. Match the existing tab-registration pattern in the file.

- [ ] **Step 3: Verify**

Run: `go build ./...`, open `/system`, go to the Пользователи tab. Toggle another user's system-admin on/off; confirm your own row is disabled; try revoking the last system-admin and confirm the inline `409` message.
Expected: toggles persist; own row disabled; last-admin/self guards surface as errors.

- [ ] **Step 4: Commit**

```bash
git add internal/web/static/system.js
git commit -m "feat(system-ui): users tab with system-admin toggle"
```

---

## Task 11: List memberships with tenant (repository)

**Files:**
- Modify: `internal/store/memberships/memberships.go` (new read model + query)
- Test: `internal/store/memberships/memberships_test.go`

**Interfaces:**
- Produces:
  - `type MembershipWithTenant struct { TenantID int64; Slug string; Name string; Role domain.Role; Status domain.MembershipStatus }`
  - `func (r *MembershipRepository) ListByUserWithTenant(ctx context.Context, userID int64) ([]MembershipWithTenant, error)` — all statuses, ordered by tenant name, excluding soft-deleted tenants.

- [ ] **Step 1: Write the failing test**

```go
func TestListByUserWithTenant(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := memberships.NewMembershipRepository(pool)

	var uid int64
	_ = pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('github:m','github','m','M') RETURNING id`).Scan(&uid)
	_, _ = repo.Upsert(ctx, domain.Membership{UserID: uid, TenantID: 1, Role: domain.RoleUser, Status: domain.MembershipRequested})

	list, err := repo.ListByUserWithTenant(ctx, uid)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].TenantID != 1 || list[0].Slug != "default" || list[0].Status != domain.MembershipRequested {
		t.Fatalf("list = %+v, want one requested membership in tenant default", list)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/memberships/ -run TestListByUserWithTenant -v`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement**

```go
// MembershipWithTenant is the read model for a user's own memberships (settings "Мои пространства").
type MembershipWithTenant struct {
	TenantID int64
	Slug     string
	Name     string
	Role     domain.Role
	Status   domain.MembershipStatus
}

// ListByUserWithTenant returns every membership of the user (all statuses) joined to its tenant,
// excluding soft-deleted tenants, ordered by tenant name. Single query — no N+1.
func (r *MembershipRepository) ListByUserWithTenant(ctx context.Context, userID int64) ([]MembershipWithTenant, error) {
	rows, err := r.db.Query(ctx, `
		SELECT m.tenant_id, t.slug, t.name, m.role, m.status
		FROM memberships m JOIN tenants t ON t.id = m.tenant_id
		WHERE m.user_id = $1 AND t.deleted_at IS NULL
		ORDER BY t.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MembershipWithTenant
	for rows.Next() {
		var m MembershipWithTenant
		if err := rows.Scan(&m.TenantID, &m.Slug, &m.Name, &m.Role, &m.Status); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/memberships/ -run TestListByUserWithTenant -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/memberships/memberships.go internal/store/memberships/memberships_test.go
git commit -m "feat(memberships): ListByUserWithTenant read model"
```

---

## Task 12: LeaveTenant with last-admin guard (service)

**Files:**
- Modify: `internal/service/onboarding.go` (`LeaveTenant`)
- Test: `internal/service/onboarding_test.go`

**Interfaces:**
- Consumes: `members.Get`, `members.CountActiveAdmins` (Task 3), `RemoveMember` (existing).
- Produces: `func (s *OnboardingService) LeaveTenant(ctx context.Context, tenantID, userID int64) error` — removes the caller's own membership + grants; `ErrLastAdmin` (reuse the one from `provisioning.go`) if the caller is the tenant's last active admin; no-op (nil) if not a member.

- [ ] **Step 1: Write the failing test**

```go
func TestLeaveTenantLastAdminGuard(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	svc := newOnboardingForTest(t, pool)
	mem := memberships.NewMembershipRepository(pool)

	var admin int64
	_ = pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('github:la','github','la','LA') RETURNING id`).Scan(&admin)
	_, _ = mem.Upsert(ctx, domain.Membership{UserID: admin, TenantID: 1, Role: domain.RoleAdmin, Status: domain.MembershipActive})

	// Sole admin cannot leave.
	if err := svc.LeaveTenant(ctx, 1, admin); !errors.Is(err, service.ErrLastAdmin) {
		t.Fatalf("err = %v, want ErrLastAdmin", err)
	}

	// A plain user can leave (and it removes the membership).
	var user int64
	_ = pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('github:pu','github','pu','PU') RETURNING id`).Scan(&user)
	_, _ = mem.Upsert(ctx, domain.Membership{UserID: user, TenantID: 1, Role: domain.RoleUser, Status: domain.MembershipActive})
	if err := svc.LeaveTenant(ctx, 1, user); err != nil {
		t.Fatalf("leave: %v", err)
	}
	if _, err := mem.Get(ctx, user, 1); !errors.Is(err, memberships.ErrNotFound) {
		t.Fatalf("membership still present: %v", err)
	}

	// Leaving a tenant you're not in is a no-op.
	if err := svc.LeaveTenant(ctx, 1, 999999); err != nil {
		t.Fatalf("non-member leave: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/ -run TestLeaveTenantLastAdminGuard -v`
Expected: FAIL — `LeaveTenant` undefined.

- [ ] **Step 3: Implement**

Add to `internal/service/onboarding.go`. `OnboardingService` already has `mem` (`*memberships.MembershipRepository`), so `CountActiveAdmins` is available.

```go
// LeaveTenant removes the caller's own membership in a tenant (any status) plus their grants there.
// Refuses if the caller is the tenant's last active admin (ErrLastAdmin, from provisioning.go).
// Not a member → no-op (nil), so it doubles as "cancel my pending request".
func (s *OnboardingService) LeaveTenant(ctx context.Context, tenantID, userID int64) error {
	scope := domain.TenantScope{TenantID: tenantID}
	cur, err := s.mem.Get(ctx, userID, tenantID)
	if errors.Is(err, memberships.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if cur.Role == domain.RoleAdmin && cur.Status == domain.MembershipActive {
		n, err := s.mem.CountActiveAdmins(ctx, scope)
		if err != nil {
			return err
		}
		if n <= 1 {
			return ErrLastAdmin
		}
	}
	return s.RemoveMember(ctx, scope, userID)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/service/ -run TestLeaveTenantLastAdminGuard -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service/onboarding.go internal/service/onboarding_test.go
git commit -m "feat(onboarding): LeaveTenant with last-admin guard"
```

---

## Task 13: Session endpoints — list memberships + leave

**Files:**
- Modify: `internal/http/handlers/api/v1/tenants/handler.go` (extend interfaces; add `ListMyMemberships`, `LeaveTenant`)
- Modify: `internal/http/server.go` (~L333 construction + routes)
- Test: `internal/http/handlers/api/v1/tenants/handler_test.go` (create if absent)

**Interfaces:**
- Consumes: `memberships.ListByUserWithTenant` (Task 11); `OnboardingService.LeaveTenant` (Task 12).
- Produces:
  - `GET /api/v1/session/memberships` → `[{tenant_id, slug, name, role, status}]`.
  - `DELETE /api/v1/session/memberships/{tenantID}` → `204`; `409` last-admin.
  - `apitenants.New` gains a `leaver` parameter (`interface { LeaveTenant(ctx context.Context, tenantID, userID int64) error }`), and `MembershipLookup` gains `ListByUserWithTenant`.

- [ ] **Step 1: Write the failing test**

An httptest test with fakes for the two dependencies. Assert: `GET` returns the mapped JSON array; `DELETE` returns `204` on success and `409` when the leaver returns `service.ErrLastAdmin`. Inject the user via context the same way other authed handler tests in the repo do (set `auth.UserFromContext`); if there's an existing helper for that, reuse it.

```go
func TestSessionMembershipsAndLeave(t *testing.T) {
	// GET
	h := apitenants.New(&fakeMembers{list: []memberships.MembershipWithTenant{
		{TenantID: 1, Slug: "default", Name: "Default", Role: domain.RoleUser, Status: domain.MembershipActive},
	}}, &fakeTenants{}, &fakeSessions{}, &fakeLeaver{})
	req := withUser(httptest.NewRequest("GET", "/api/v1/session/memberships", nil), 7)
	w := httptest.NewRecorder()
	h.ListMyMemberships(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"slug":"default"`) {
		t.Fatalf("GET = %d %s", w.Code, w.Body.String())
	}

	// DELETE last-admin → 409
	h2 := apitenants.New(&fakeMembers{}, &fakeTenants{}, &fakeSessions{}, &fakeLeaver{err: service.ErrLastAdmin})
	r := chi.NewRouter()
	r.Delete("/api/v1/session/memberships/{tenantID}", h2.LeaveTenant)
	req2 := withUser(httptest.NewRequest("DELETE", "/api/v1/session/memberships/1", nil), 7)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusConflict {
		t.Fatalf("DELETE = %d, want 409", w2.Code)
	}
}
```

Define the fakes (`fakeMembers` implementing `ListByUser` + `ListByUserWithTenant`, `fakeLeaver` with an `err` field, plus minimal `fakeTenants`/`fakeSessions`) and a `withUser` helper that stuffs a `*domain.User{ID: id}` into the request context via `auth`'s context key (mirror an existing authed handler test).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/http/handlers/api/v1/tenants/ -run TestSessionMembershipsAndLeave -v`
Expected: FAIL — methods/constructor arg undefined.

- [ ] **Step 3: Implement**

In `internal/http/handlers/api/v1/tenants/handler.go` extend the interfaces and handler:

```go
type MembershipLookup interface {
	ListByUser(ctx context.Context, userID int64) ([]domain.Membership, error)
	ListByUserWithTenant(ctx context.Context, userID int64) ([]memberships.MembershipWithTenant, error)
}

// MembershipLeaver removes the caller's own membership. *service.OnboardingService satisfies it.
type MembershipLeaver interface {
	LeaveTenant(ctx context.Context, tenantID, userID int64) error
}
```

Add `leaver MembershipLeaver` to the `Handler` struct and `New(...)` (append parameter). Add handlers:

```go
type membershipDTO struct {
	TenantID int64  `json:"tenant_id"`
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Status   string `json:"status"`
}

// GET /api/v1/session/memberships — the caller's memberships (all statuses) for /settings.
func (h *Handler) ListMyMemberships(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	list, err := h.members.ListByUserWithTenant(r.Context(), user.ID)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	out := make([]membershipDTO, 0, len(list))
	for _, m := range list {
		out = append(out, membershipDTO{TenantID: m.TenantID, Slug: m.Slug, Name: m.Name, Role: string(m.Role), Status: string(m.Status)})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// DELETE /api/v1/session/memberships/{tenantID} — leave a tenant / cancel a pending request.
func (h *Handler) LeaveTenant(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	tenantID, err := strconv.ParseInt(chi.URLParam(r, "tenantID"), 10, 64)
	if err != nil || tenantID <= 0 {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	switch err := h.leaver.LeaveTenant(r.Context(), tenantID, user.ID); {
	case errors.Is(err, service.ErrLastAdmin):
		http.Error(w, `{"error":"last admin cannot leave"}`, http.StatusConflict)
	case err != nil:
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
```

Add imports: `"errors"`, `"strconv"`, `"github.com/go-chi/chi/v5"`, `"okrs/internal/service"`, `"okrs/internal/store/memberships"`.

In `internal/http/server.go` update construction and routes (the `*MembershipRepository` `s.store.Memberships` now satisfies the extended `MembershipLookup`; `s.onboarding` satisfies `MembershipLeaver`):

```go
	tenantH := apitenants.New(s.store.Memberships, s.store.Tenants, s.store.Sessions, s.onboarding)
	r.Get("/api/v1/session/tenants", tenantH.ListMyTenants)
	r.Post("/api/v1/session/tenant", tenantH.SwitchTenant)
	r.Get("/api/v1/session/memberships", tenantH.ListMyMemberships)
	r.Delete("/api/v1/session/memberships/{tenantID}", tenantH.LeaveTenant)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go build ./... && go test ./internal/http/handlers/api/v1/tenants/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/http/handlers/api/v1/tenants/handler.go internal/http/handlers/api/v1/tenants/handler_test.go internal/http/server.go
git commit -m "feat(session): list own memberships and leave-tenant endpoints"
```

---

## Task 14: /settings "Мои пространства" section

**Files:**
- Modify: `internal/web/static/settings.js` (api helpers; `SECTION_META`; `sections`; new `SpacesSection`; render switch)

**Interfaces:**
- Consumes: `GET /api/v1/session/memberships`, `DELETE /api/v1/session/memberships/{tenantID}` (Task 13), `POST /api/v1/onboarding/join-request` (existing).

- [ ] **Step 1: Add POST/DELETE api helpers**

`settings.js` currently only has `apiGet`. Add CSRF-aware mutators next to it (reuse the file's existing `readCSRF`):

```js
async function apiSend(url, method, body) {
  const res = await fetch(url, {
    method, credentials: 'include',
    headers: { 'X-CSRF-Token': readCSRF(), ...(body !== undefined ? {'Content-Type':'application/json'} : {}) },
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  return res;
}
const apiPost = (url, body) => apiSend(url, 'POST', body);
const apiDelete = (url) => apiSend(url, 'DELETE');
```

- [ ] **Step 2: Add the SpacesSection component**

```jsx
function SpacesSection() {
  const [rows, setRows] = useState([]);
  const [slug, setSlug] = useState('');
  const [msg, setMsg] = useState('');
  const reload = async () => { try { setRows(await apiGet('/api/v1/session/memberships')); } catch (_) {} };
  useEffect(() => { reload(); }, []);
  async function leave(tenantID) {
    setMsg('');
    const res = await apiDelete(`/api/v1/session/memberships/${tenantID}`);
    if (res.status === 204) { reload(); return; }
    try { const j = await res.json(); setMsg(j.error || ('Ошибка ' + res.status)); } catch { setMsg('Ошибка ' + res.status); }
  }
  async function join(e) {
    e.preventDefault();
    setMsg('');
    const res = await apiPost('/api/v1/onboarding/join-request', { slug: slug.trim() });
    if (res.status === 204) { setSlug(''); setMsg('Заявка отправлена'); reload(); return; }
    try { const j = await res.json(); setMsg(j.error || ('Ошибка ' + res.status)); } catch { setMsg('Ошибка ' + res.status); }
  }
  return (
    <div className="set-panel">
      <h2>Мои пространства</h2>
      {msg && <div className="set-note">{msg}</div>}
      <ul className="set-spaces">
        {rows.map(m => (
          <li key={m.tenant_id} className="set-spaces__row">
            <span className="set-spaces__name">{m.name}</span>
            <span className="set-spaces__slug">{m.slug}</span>
            <span className="set-spaces__role">{m.role}</span>
            <span className="set-spaces__status">{m.status === 'active' ? 'Активен' : 'Заявка отправлена'}</span>
            <button onClick={() => leave(m.tenant_id)}>
              {m.status === 'active' ? 'Выйти' : 'Отменить заявку'}
            </button>
          </li>
        ))}
      </ul>
      <form onSubmit={join} className="set-spaces__join">
        <input value={slug} onChange={e => setSlug(e.target.value)} placeholder="slug пространства" />
        <button type="submit" disabled={!slug.trim()}>Отправить заявку</button>
      </form>
    </div>
  );
}
```

(Class names are illustrative — reuse existing settings.js styling conventions / inline styles as the surrounding sections do. No new CSS is required for correctness.)

- [ ] **Step 3: Register the section**

In `SECTION_META` add:

```js
  spaces: { label: 'Мои пространства', hint: 'Тенанты и заявки', icon: '🏢' },
```

Make it always available (unlike `descriptions`, which is lead-gated). Update the `sections` memo so `spaces` is always included, e.g.:

```js
  const sections = useMemo(() => [ ...(isLead ? ['descriptions'] : []), 'sidebar', 'spaces' ], [isLead]);
```

And in the section render switch (where `descriptions`/`sidebar` are rendered), add a branch rendering `<SpacesSection/>` when `active === 'spaces'`.

- [ ] **Step 4: Verify**

Run: `go build ./...`, open `/settings`, select "Мои пространства". Confirm the list shows your memberships; leave a tenant (non-last-admin) and confirm it disappears; submit a join-request by slug and confirm the "Заявка отправлена" note and that the pending row appears; try leaving as a sole admin and confirm the conflict message.
Expected: list/leave/cancel/join all work; guards surface inline.

- [ ] **Step 5: Commit**

```bash
git add internal/web/static/settings.js
git commit -m "feat(settings-ui): my spaces section — list, leave, request to join"
```

---

## Task 15: Full-suite verification

**Files:** none (verification only).

- [ ] **Step 1: Build, vet, test everything**

Run:
```bash
go build ./...
go vet ./...
go test ./...
```
Expected: all pass. Fix any failures before proceeding.

- [ ] **Step 2: Manual smoke of the happy paths**

Bring up the app (per repo run instructions). Verify end-to-end:
1. User A requests to join tenant `default` by slug from `/settings` → appears in `/admin` access-requests → admin approves → A has the default-node grant (if policy set).
2. In `/system`: change a member's role; grant/revoke system-admin (own row disabled).
3. In `/settings`: leave a tenant; cancel a pending request.

- [ ] **Step 3: Commit (if any fixups were needed)**

```bash
git add -A
git commit -m "test: verify tenant access and membership management end-to-end"
```

---

## Task 16: Update specs

**Files:**
- Modify: `specs/040-api-contract.md`
- Modify: `specs/050-permissions-and-lifecycle.md`
- Modify: `specs/030-user-flows.md` (only if a membership/onboarding flow section exists to extend)

- [ ] **Step 1: Document the new endpoints in `040-api-contract.md`**

In the **System API endpoints** section add (full contract shape, matching the file's style):

```
- `PUT /api/v1/system/tenants/{id}/members/{userID}/role` — сменить роль участника; body `{"role":"user"|"admin"}` → `204`. `422` невалидная роль, `404` нет membership, `409` понижение последнего админа тенанта.
- `PUT /api/v1/system/users/{userID}/system-admin` — выдать/снять system-привилегии; body `{"is_system_admin": bool}` → `204`. `404` нет пользователя, `409` снятие последнего system-admin или снятие собственной привилегии (self-lockout).
```

In the **Onboarding endpoints / session** area add:

```
**Любой авторизованный (не membership-gated):**
- `GET /api/v1/session/memberships` — свои membership (все статусы) для /settings: `[{tenant_id, slug, name, role, status}]`, отсортировано по имени тенанта. Read-only.
- `DELETE /api/v1/session/memberships/{tenantID}` — выйти из тенанта / отменить свою заявку (удаляет собственный membership любого статуса + свои гранты) → `204`, идемпотентно; `409` если вызывающий — последний админ тенанта.
```

Note that `/settings` reuses `POST /api/v1/onboarding/join-request` for join-by-slug.

- [ ] **Step 2: Update `050-permissions-and-lifecycle.md`**

- In the onboarding section: note that the tenant `new_user_policy` (default-node grant) is now applied on **approval** of a join-request (tenant-admin) and on **system-admin attach**, not only on auto-registration / invite-claim.
- In the system-admin plane description: add that system-admin can change tenant member roles (`PUT …/members/{id}/role`) and grant/revoke `is_system_admin` (`PUT /api/v1/system/users/{id}/system-admin`).
- Add the guardrails: a tenant always retains ≥1 active admin (enforced on role change and on leave); the instance always retains ≥1 system-admin; a system-admin cannot revoke their own `is_system_admin`.

- [ ] **Step 3: Update `030-user-flows.md` if applicable**

If the file has a membership/onboarding flow section, add the `/settings` "Мои пространства" flow (list memberships, leave / cancel request, request-to-join by slug). If there is no matching section, leave the file untouched (do not invent a new top-level structure).

- [ ] **Step 4: Verify specs read consistently**

Re-read the edited sections; ensure endpoint paths/verbs/status codes match the handlers implemented in Tasks 5, 9, 13. No code to run.

- [ ] **Step 5: Commit**

```bash
git add specs/040-api-contract.md specs/050-permissions-and-lifecycle.md specs/030-user-flows.md
git commit -m "docs(specs): document membership role, system privileges, session memberships endpoints"
```

---

## Self-Review notes

- **Spec coverage:** Task 1/2 → default access on approval (both planes). Task 3-6 → change member role in /system. Task 7-10 → system privileges in /system. Task 11-14 → /settings spaces (list/leave/cancel/join). Task 16 → spec updates. All four original requests covered.
- **Type consistency:** `ErrLastAdmin` defined once in `provisioning.go`, reused by `LeaveTenant` (same package). `SetSystemAdmin(callerID, targetID, v)` signature identical in service, `Provisioner` interface, and handler call. `MembershipWithTenant` fields match between repo (Task 11) and DTO mapping (Task 13). `Provisioner` interface gains both `SetMemberRole` and `SetSystemAdmin` in Task 5 to avoid a second edit — Task 8 supplies the concrete method (build-order note included in Task 5 Step 4).
- **No migrations / seed changes:** confirmed — all columns pre-exist.
