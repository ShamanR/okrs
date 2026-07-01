# Tenant-scoped admin users + access-request actions — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans (inline,
> compiler-driven — subagents flap here). Steps use checkbox (`- [ ]`) syntax. Build/vet/test
> green after every task; commits are the user's (agent does `git add` + proposes a message, no
> AI attribution).

**Goal:** Make `/admin?section=users` show only the active tenant's members + access requesters
(not all global users), surface approve/deny on requesters in both `/admin` and `/system`, and
add the missing system-side deny.

**Architecture:** Backend: a new `UserRepository.ListByTenant` (join memberships+users → full
user + status + role) backs a rewritten `HandleListUsers`; a new `ProvisioningService.DenyMember`
+ `POST /api/v1/system/tenants/{id}/members/{userID}/deny` mirror the tenant-admin deny on the
system plane. Frontend: `admin.js` renders requester rows with add/deny (existing
`/api/v1/admin/access-requests/*`); `system.js` sorts requesters first with connect/deny. Reuses
approve/deny from Plan 4 and `AttachMember`/members-list from the system panel.

**Tech Stack:** Go, pgx/v5, chi, testcontainers-go; React 18 UMD + Babel (admin.js / system.js).

## Global Constraints

- **Tenant relation = a `memberships` row** for (user, active tenant); statuses `active`
  (has access) / `requested` (asked). Users with no membership in the tenant must NOT appear in
  `/admin?section=users`.
- **Approve / Add / Connect** = membership → `active`. **Deny / Reject** = delete the
  `requested` membership (Plan 4 `DeleteRequested`); applies only to `requested`, never active.
- **Explicit `domain.TenantScope`** in services/repos; only handlers read context. System
  endpoints take ids from the URL (cross-tenant plane), gated by `RequireSystemAdmin`.
- **No new DB schema.** Only a new read query + one write endpoint.
- **Response compatibility:** `/api/v1/admin/users` items keep their current PascalCase shape
  (`*domain.User` fields + `GrantedNodeCount`) and ADD `Status`, `Role`. `admin.js` reads these
  Go field names directly.
- **Commits are the user's**; no AI/Claude attribution.

---

### Task 1: `UserRepository.ListByTenant` (members + requesters, with status/role)

**Files:**
- Modify: `internal/store/users/users.go`
- Test: `internal/store/users/users_test.go`

**Interfaces:**
- Produces:
  - `type TenantUser struct { User *domain.User; Status domain.MembershipStatus; Role domain.Role }`
  - `(*UserRepository) ListByTenant(ctx context.Context, scope domain.TenantScope) ([]TenantUser, error)`
    — every user with a membership in the tenant (any status), ordered by `display_name`.

- [ ] **Step 1: Write the failing test**

```go
// add to internal/store/users/users_test.go
func TestUserListByTenantScopedWithStatus(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	r := users.NewUserRepository(pool)

	if _, err := pool.Exec(ctx, `INSERT INTO tenants (id, slug, name) OVERRIDING SYSTEM VALUE VALUES (2,'t2','T2')`); err != nil {
		t.Fatalf("tenant 2: %v", err)
	}
	mk := func(key string) int64 {
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
			VALUES ($1,'github',$1,$1) RETURNING id`, key).Scan(&id); err != nil {
			t.Fatalf("user %s: %v", key, err)
		}
		return id
	}
	memberA := mk("a") // active in tenant 1
	reqB := mk("b")    // requested in tenant 1
	otherC := mk("c")  // member of tenant 2 only
	mustExec(t, pool, `INSERT INTO memberships (user_id, tenant_id, role, status) VALUES ($1,1,'admin','active')`, memberA)
	mustExec(t, pool, `INSERT INTO memberships (user_id, tenant_id, role, status) VALUES ($1,1,'user','requested')`, reqB)
	mustExec(t, pool, `INSERT INTO memberships (user_id, tenant_id, role, status) VALUES ($1,2,'user','active')`, otherC)

	got, err := r.ListByTenant(ctx, domain.TenantScope{TenantID: 1})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	byID := map[int64]users.TenantUser{}
	for _, tu := range got {
		byID[tu.User.ID] = tu
	}
	if _, leaked := byID[otherC]; leaked {
		t.Fatalf("tenant-2-only user must not appear under tenant 1")
	}
	if byID[memberA].Status != domain.MembershipActive || byID[memberA].Role != domain.RoleAdmin {
		t.Fatalf("memberA = %+v", byID[memberA])
	}
	if byID[reqB].Status != domain.MembershipRequested {
		t.Fatalf("reqB status = %q, want requested", byID[reqB].Status)
	}
	if byID[memberA].User.DisplayName != "a" {
		t.Fatalf("user fields not loaded: %+v", byID[memberA].User)
	}
}

func mustExec(t *testing.T, pool interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec: %v", err)
	}
}
```

> If adding the `pgconn` import for `mustExec` is awkward, inline the inserts with
> `pool.Exec` + an `if err != nil` check instead — the assertions are the point.

- [ ] **Step 2: Run to verify it fails** — `go test ./internal/store/users -run TestUserListByTenant`
  → FAIL (`TenantUser`/`ListByTenant` undefined).

- [ ] **Step 3: Implement** (mirror the existing SELECT column list in `users.go`; the scanUser
  helper already scans the full user — reuse the same column order)

```go
// TenantUser pairs a user with its membership in a specific tenant.
type TenantUser struct {
	User   *domain.User
	Status domain.MembershipStatus
	Role   domain.Role
}

// ListByTenant returns every user with a membership in the tenant (any status), with that
// membership's status and role, ordered by display name.
func (r *UserRepository) ListByTenant(ctx context.Context, scope domain.TenantScope) ([]TenantUser, error) {
	rows, err := r.db.Query(ctx, `
		SELECT u.id, u.udid, u.provider_subject_key, u.provider, u.subject, u.display_name,
		       u.avatar_url, COALESCE(u.email,''), u.attributes_json, u.is_admin, u.is_system_admin,
		       u.created_at, u.updated_at, u.last_login_at, m.status, m.role
		FROM memberships m JOIN users u ON u.id = m.user_id
		WHERE m.tenant_id = $1
		ORDER BY u.display_name`, scope.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TenantUser
	for rows.Next() {
		var u domain.User
		var attrRaw []byte
		var status domain.MembershipStatus
		var role domain.Role
		if err := rows.Scan(
			&u.ID, &u.UDID, &u.ProviderSubjectKey, &u.Provider, &u.Subject, &u.DisplayName,
			&u.AvatarURL, &u.Email, &attrRaw, &u.IsAdmin, &u.IsSystemAdmin,
			&u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt, &status, &role,
		); err != nil {
			return nil, err
		}
		if len(attrRaw) > 0 {
			_ = json.Unmarshal(attrRaw, &u.AttributesJSON)
		}
		out = append(out, TenantUser{User: &u, Status: status, Role: role})
	}
	return out, rows.Err()
}
```

> Verify the exact column list/scan order against `scanUser` in `users.go` (Plan 3 added
> `is_system_admin` after `is_admin`). Match it so the scan lines up.

- [ ] **Step 4: Run** `go test ./internal/store/users` → PASS.
- [ ] **Step 5: Stage** `git add internal/store/users/`
  (message: `feat(store): list tenant users with membership status/role`).

---

### Task 2: Tenant-scope `HandleListUsers`

**Files:**
- Modify: `internal/http/handlers/api/v1/admin/handler.go` (`userAdminStore`, `HandleListUsers`)
- Test: `internal/http/handlers/api/v1/admin/handler_test.go`

**Interfaces:**
- `userAdminStore` gains `ListByTenant(ctx, scope domain.TenantScope) ([]users.TenantUser, error)`
  (satisfied by `*store.UserRepository`); `HandleListUsers` no longer calls `ListUsers`.
- Response item: `{ *domain.User fields (PascalCase), GrantedNodeCount int, Status string, Role string }`.

- [ ] **Step 1: Update the test** — the existing `fakeUsers` + `TestHandleListUsersExcludesDeletedTeamGrantsFromCount`
  drive `ListUsers`; switch them to `ListByTenant` and assert `Status` is present and only tenant
  users appear. Add `ListByTenant` to `fakeUsers`:

```go
// in handler_test.go — extend fakeUsers
func (f *fakeUsers) ListByTenant(context.Context, domain.TenantScope) ([]users.TenantUser, error) {
	return f.tenantUsers, nil
}
```

  Give `fakeUsers` a `tenantUsers []users.TenantUser` field and rewrite the count test to seed it:

```go
func TestHandleListUsersIsTenantScopedWithStatus(t *testing.T) {
	users10 := &domain.User{ID: 10, DisplayName: "Active"}
	users20 := &domain.User{ID: 20, DisplayName: "Requester"}
	fu := &fakeUsers{tenantUsers: []users.TenantUser{
		{User: users10, Status: domain.MembershipActive, Role: domain.RoleUser},
		{User: users20, Status: domain.MembershipRequested, Role: domain.RoleUser},
	}}
	g := &fakeGrants{
		all:           map[int64][]grants.HierarchyGrant{10: {{UserID: 10, TeamID: 1}}},
		activeTeamIDs: map[int64]bool{1: true},
	}
	h := New(fu, nil, nil, g)
	r := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil))
	w := httptest.NewRecorder()
	h.HandleListUsers(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("code %d", w.Code)
	}
	var got []map[string]any
	_ = json.NewDecoder(w.Body).Decode(&got)
	if len(got) != 2 {
		t.Fatalf("want 2 tenant users, got %d", len(got))
	}
	byID := map[float64]map[string]any{}
	for _, u := range got {
		byID[u["ID"].(float64)] = u
	}
	if byID[10]["Status"] != "active" || byID[10]["GrantedNodeCount"].(float64) != 1 {
		t.Fatalf("active member = %+v", byID[10])
	}
	if byID[20]["Status"] != "requested" || byID[20]["GrantedNodeCount"].(float64) != 0 {
		t.Fatalf("requester = %+v", byID[20])
	}
}
```

  Remove/replace the old `TestHandleListUsersExcludesDeletedTeamGrantsFromCount` (its intent —
  grant counting for active members — is covered by user 10 above). Add the `withTenant` helper if
  not already shared in this test file (it exists from the settings tests in this package).

- [ ] **Step 2: Run to verify it fails** — compile error (`fakeUsers` lacks `ListByTenant` /
  `New` signature) then assertion.

- [ ] **Step 3: Implement** — add to the `userAdminStore` interface and rewrite the top of
  `HandleListUsers`:

```go
// userAdminStore (add the method)
ListByTenant(ctx context.Context, scope domain.TenantScope) ([]users.TenantUser, error)
```

```go
func (h *Handler) HandleListUsers(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusForbidden, "no active tenant")
		return
	}
	tenantUsers, err := h.users.ListByTenant(r.Context(), scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	allGrants, err := h.grants.AllGrants(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// (existing distinct-roots → ListDescendantTeamIDs → activeSet block, unchanged)
	distinct := make(map[int64]struct{})
	for _, gs := range allGrants {
		for _, g := range gs {
			distinct[g.TeamID] = struct{}{}
		}
	}
	roots := make([]int64, 0, len(distinct))
	for id := range distinct {
		roots = append(roots, id)
	}
	activeIDs, err := h.grants.ListDescendantTeamIDs(r.Context(), scope, roots)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	activeSet := make(map[int64]struct{}, len(activeIDs))
	for _, id := range activeIDs {
		activeSet[id] = struct{}{}
	}

	type userListItem struct {
		*domain.User
		GrantedNodeCount int
		Status           string
		Role             string
	}
	items := make([]userListItem, 0, len(tenantUsers))
	for _, tu := range tenantUsers {
		count := 0
		for _, g := range allGrants[tu.User.ID] {
			if _, ok := activeSet[g.TeamID]; ok {
				count++
			}
		}
		items = append(items, userListItem{
			User: tu.User, GrantedNodeCount: count,
			Status: string(tu.Status), Role: string(tu.Role),
		})
	}
	writeJSON(w, items)
}
```

  Add `"okrs/internal/store/users"` to the handler imports. Drop the now-unused scope lookup that
  was lower in the old body.

- [ ] **Step 4: Run** `go build ./... && go test ./internal/http/handlers/api/v1/admin` → PASS.
- [ ] **Step 5: Stage** (message: `feat(admin): tenant-scope the users list (members + requesters, with status)`).

---

### Task 3: System-side deny — `ProvisioningService.DenyMember` + route

**Files:**
- Modify: `internal/service/provisioning.go`
- Modify: `internal/http/handlers/api/v1/system/handler.go` (`Provisioner` iface + `HandleDenyMember`)
- Modify: `internal/http/server.go` (mount route)
- Test: `internal/service/provisioning_test.go`, `internal/http/handlers/api/v1/system/handler_test.go`

**Interfaces:**
- Produces:
  - `(*ProvisioningService) DenyMember(ctx, tenantID, userID int64) error` — removes a `requested`
    membership; invalidates the membership cache.
  - `Provisioner` gains `DenyMember(ctx, tenantID, userID int64) error`.
  - `(*system.Handler) HandleDenyMember` → `POST /api/v1/system/tenants/{id}/members/{userID}/deny` → `204`.

- [ ] **Step 1: Write the failing service test**

```go
// add to internal/service/provisioning_test.go (inside the existing testcontainers setup pattern)
func TestProvisioningDenyMember(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	tnRepo := tenants.NewTenantRepository(pool)
	memRepo := memberships.NewMembershipRepository(pool)
	tsRepo := tenantsettings.NewTenantSettingsRepository(pool)
	sysRepo := settings.NewSettingsRepository(pool)
	settingsSvc := service.NewSettingsService(
		tenantsettings.NewTenantSettingsCache(tsRepo), tsRepo,
		settings.NewSystemSettingsCache(sysRepo), sysRepo,
	)
	prov := service.NewProvisioningService(tnRepo, tenants.NewTenantCache(tnRepo), memRepo, memberships.NewMembershipCache(memRepo), settingsSvc)

	if _, err := memRepo.Upsert(ctx, domain.Membership{UserID: 1, TenantID: 1, Role: domain.RoleUser, Status: domain.MembershipRequested}); err != nil {
		t.Fatalf("seed requested: %v", err)
	}
	if err := prov.DenyMember(ctx, 1, 1); err != nil {
		t.Fatalf("deny: %v", err)
	}
	if _, err := memRepo.Get(ctx, 1, 1); err != memberships.ErrNotFound {
		t.Fatalf("requested membership should be gone, got %v", err)
	}
}
```

- [ ] **Step 2: Run to verify it fails.**

- [ ] **Step 3: Implement `DenyMember`** in `provisioning.go`

```go
// DenyMember removes a pending (requested) membership in a tenant.
func (p *ProvisioningService) DenyMember(ctx context.Context, tenantID, userID int64) error {
	scope := domain.TenantScope{TenantID: tenantID}
	if err := p.members.DeleteRequested(ctx, scope, userID); err != nil {
		return err
	}
	p.memberCache.InvalidateUser(userID)
	return nil
}
```

- [ ] **Step 4: Add the handler + route + interface.** In `system/handler.go`, add to `Provisioner`:
  `DenyMember(ctx context.Context, tenantID, userID int64) error`, and the handler:

```go
// POST /api/v1/system/tenants/{id}/members/{userID}/deny
func (h *Handler) HandleDenyMember(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	userID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	if err != nil || userID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if err := h.prov.DenyMember(r.Context(), id, userID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

  In `server.go` `registerSystemRoutes`, after the members POST:
  `r.Post("/api/v1/system/tenants/{id}/members/{userID}/deny", sysH.HandleDenyMember)`.

- [ ] **Step 5: Add a handler test** in `system/handler_test.go` (register the route in
  `buildRouter`: `r.Post("/api/v1/system/tenants/{id}/members/{userID}/deny", h.HandleDenyMember)`):

```go
func TestSystemDenyMember(t *testing.T) {
	admin := &domain.User{ID: 1, IsSystemAdmin: true}
	r, _, _ := buildRouter(t, admin)
	// seed a requested membership for user 1 in tenant 1 via the DB pool is not exposed here;
	// instead create tenant 2 + a requested membership through the API-less path:
	// reuse the members POST is "attach active", so seed via a direct requested row.
	// buildRouter returns repos enough? If not, attach then deny is the simplest reachable path:
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/system/tenants/1/members",
		strings.NewReader(`{"user_id":1,"role":"user"}`)))
	if w.Code != http.StatusCreated {
		t.Fatalf("attach: %d", w.Code)
	}
	// deny removes only 'requested'; an active membership is NOT removed → members still lists it.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/system/tenants/1/members/1/deny", nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("deny: %d (%s)", w.Code, w.Body.String())
	}
}
```

> The service-level test (Step 1) is the authoritative behavioral check (requested → gone). The
> handler test asserts the route wiring + gate (a non-admin gets 403 via the shared gate, already
> covered by `TestSystemAPIForbiddenForNonAdmin`). Keep this handler test minimal — wiring only.

- [ ] **Step 6: Run** `go build ./... && go test ./internal/service ./internal/http/...` → PASS.
- [ ] **Step 7: Stage** (message: `feat(system): deny a requested member (DenyMember + route)`).

---

### Task 4: `admin.js` — requester rows with Add/Deny; map status

**Files:**
- Modify: `internal/web/static/admin.js` (`_adminLoadUsers` map, `UsersSection`)
- Test: manual (DoD `010`).

- [ ] **Step 1: Map `Status`/`Role`** in `_adminLoadUsers` (line ~618) and ensure the App-level
  users load passes them through (the App maps the raw response; `UsersSection` reads PascalCase
  `u.Status`). In `_adminLoadUsers`, add `status: u.Status, role: u.Role` to the mapped object.

- [ ] **Step 2: In `UsersSection`, exclude requesters from the "no access" count/filter** and
  render them distinctly. Replace the no-access count + filter predicate to ignore requested:

```jsx
const isRequester = u => u.Status === 'requested';
const noAccessCount = users.filter(u=>!isRequester(u) && !u.IsAdmin && (u.GrantedNodeCount||0)===0).length;
const requestCount = users.filter(isRequester).length;
```

  Add a chip `{id:'requests', label:'Заявки', count:requestCount}` and in the filter predicate:
  `if (filter==='requests' && !isRequester(u)) return false;` and
  `if (filter==='noaccess' && (isRequester(u) || u.IsAdmin || (u.GrantedNodeCount||0)>0)) return false;`.

- [ ] **Step 3: Render the right-side cell for requesters** as Add/Deny buttons (replace the
  badge block at lines ~1153-1162 with a branch):

```jsx
<div onClick={e=>e.stopPropagation()} style={{display:'flex',alignItems:'center',gap:8,flexShrink:0}}>
  {isRequester(u)
    ? <>
        <button onClick={async()=>{ const r=await apiPost(`/api/v1/admin/access-requests/${u.ID}/approve`,{}); if(r&&r.ok) reload(); else alert('Не удалось добавить'); }}
          style={{padding:'6px 12px',border:'none',borderRadius:7,background:'#059669',color:'#fff',fontWeight:600,cursor:'pointer'}}>Добавить</button>
        <button onClick={async()=>{ const r=await apiPost(`/api/v1/admin/access-requests/${u.ID}/deny`,{}); if(r&&r.ok) reload(); else alert('Не удалось отклонить'); }}
          style={{padding:'6px 12px',border:'1.5px solid '+T.cardBorder,borderRadius:7,background:'#fff',color:T.danger,fontWeight:600,cursor:'pointer'}}>Отклонить</button>
      </>
    : <>{/* existing badge: Admin / N узл / нет доступа + RowAction edit */}</>}
</div>
```

  Keep the existing badge + edit `RowAction` markup in the `else` branch (move lines ~1153-1162
  there). A requester row should NOT open the grant modal — guard the row `onClick`:
  `onClick={()=> { if(!isRequester(u)) setModalId(u.ID); }}`.

- [ ] **Step 4: Manual verify (DoD)** — with a real tenant: a `requested` membership shows in
  `/admin?section=users` (and under the «Заявки» chip) with Add/Deny; Add → becomes a member,
  Deny → disappears; non-members of the tenant never appear.

- [ ] **Step 5: Stage** (message: `feat(admin-ui): show access requesters with add/deny in users list`).

---

### Task 5: `system.js` — requesters first, connect/deny

**Files:**
- Modify: `internal/web/static/system.js` (`MembersSection`)
- Test: manual (DoD `010`).

- [ ] **Step 1: Sort requesters first** in `MembersSection` after loading members:

```jsx
const ordered = [...members].sort((a,b)=>(a.status==='requested'?0:1)-(b.status==='requested'?0:1));
```

  Render `ordered` instead of `members` in the table body.

- [ ] **Step 2: Add a right-side actions cell** to the members table — header `''` column, and per
  row for `status==='requested'`:

```jsx
{m.status==='requested' && <td style={{padding:'6px 8px'}}>
  <button style={{...btn,background:C.ok,marginRight:6}}
    onClick={async()=>{ const res=await post(`/api/v1/system/tenants/${tid}/members`, {user_id:m.user_id, role:m.role}); if(res.status===201) loadMembers(tid); else setErr(await errMsg(res)); }}>Подключить</button>
  <button style={{...btn,background:C.muted}}
    onClick={async()=>{ const res=await post(`/api/v1/system/tenants/${tid}/members/${m.user_id}/deny`); if(res.status===204) loadMembers(tid); else setErr(await errMsg(res)); }}>Отклонить</button>
</td>}
```

  (`loadMembers`, `tid`, `post`, `setErr`, `errMsg`, `btn`, `C` already exist in `system.js`. Add
  an empty header cell so columns align; active rows render an empty `<td>` in that column.)

- [ ] **Step 3: Manual verify (DoD)** — in `/system` → Участники, pick a tenant with a pending
  request: it appears at the top; «Подключить» makes it active (status flips), «Отклонить» removes
  it from the list.

- [ ] **Step 4: Stage** (message: `feat(system-ui): surface access requesters first with connect/deny`).

---

### Task 6: API spec update

**Files:**
- Modify: `specs/040-api-contract.md`

- [ ] **Step 1: Document the scoping + new endpoint.** Under the admin section, note
  `GET /api/v1/admin/users` returns only the active tenant's members + requesters, each with
  `Status`/`Role` (PascalCase, alongside the user fields + `GrantedNodeCount`). Under the System
  section, add:

```markdown
- `POST /api/v1/system/tenants/{id}/members/{userID}/deny` — удалить заявку (`requested`-membership) пользователя в тенанте → `204`. (Подключение заявки — существующий `POST …/members`.)
```

- [ ] **Step 2: Run** `go build ./...` (docs sanity) → green.
- [ ] **Step 3: Stage** (message: `docs(specs): tenant-scoped admin users + system member deny`).

---

## Self-Review Notes

- **Spec coverage:** Req 1 → Tasks 1–2 (tenant-scoped list with status); Req 2 → Task 4 (admin
  add/deny via existing Plan-4 endpoints); Req 3 → Task 3 (system deny endpoint) + Task 5
  (system UI requesters-first connect/deny). Specs → Task 6.
- **Type consistency:** `users.TenantUser{User,Status,Role}` defined in Task 1, consumed in Task 2
  (`userAdminStore.ListByTenant`) and its fake. Response adds PascalCase `Status`/`Role`, which
  `admin.js` (Task 4) reads as `u.Status`. `Provisioner.DenyMember(ctx, tenantID, userID)` defined
  in Task 3 and called by `HandleDenyMember`; the route path param is `{userID}` (matches the
  existing `{id}` tenant param + a new `{userID}`).
- **Side effect (intended):** `/api/v1/admin/users` is also used by `admin.js`'s UserSelector
  (team-lead picker) — it now offers only tenant members, which is correct for a tenant-admin.
  Noted; no separate endpoint.
- **Behaviour-preserving where required:** active-member rows, grant management, admin toggle,
  grant counts all unchanged; only requesters get the new treatment and non-members drop out.

## Execution recommendation

Inline, one task at a time. Backend (1–3, 6) is TDD/Go with green gates; frontend (4–5) is manual
DoD (no frontend auto-tests in this project). After each task: `go build ./... && go vet ./... &&
go test ./...` green (4–5: build + manual), then `git add` + propose a commit message; the user
commits. Live-verify 4–5 in a browser once Docker is available.
