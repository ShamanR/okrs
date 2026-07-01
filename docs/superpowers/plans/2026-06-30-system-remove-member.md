# Remove member from tenant (`/system`) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans (inline,
> compiler-driven — subagents flap here). Steps use checkbox (`- [ ]`) syntax. Build/vet/test
> green after every task; commits are the user's (agent does `git add` + proposes a message, no
> AI attribution).

**Goal:** Add a "remove member" action to the `/system` members view that deletes a user's
membership in a tenant AND all their hierarchy grants in that tenant (full access severance).

**Architecture:** New repo deletes (`MembershipRepository.Delete`,
`GrantRepository.RemoveAllUserGrants` + cache mirror) composed by a new
`ProvisioningService.RemoveMember`, exposed as `DELETE /api/v1/system/tenants/{id}/members/{userID}`
under `RequireSystemAdmin`; `system.js` adds a confirm-guarded «Удалить» on active member rows.
Distinct from the existing `deny` (requested-only, no grants).

**Tech Stack:** Go, pgx/v5, chi, testcontainers-go; React 18 UMD + Babel (system.js).

## Global Constraints

- **Remove member = delete membership (any status) + delete all the user's grants in that
  tenant.** Other tenants untouched (everything tenant-scoped by `tenant_id`). Idempotent: absent
  rows → no-op → `204`.
- **No guardrails** (last admin / self-removal): `/system` is the instance-operator plane.
- **`deny` stays** (requested-only, no grant cleanup); `RemoveMember` is the any-status + grants
  variant. Both coexist.
- **Explicit `domain.TenantScope`** in services/repos; handlers take ids from the URL; gated by
  `RequireSystemAdmin`.
- **Commits are the user's**; no AI/Claude attribution.

---

### Task 1: `MembershipRepository.Delete`

**Files:**
- Modify: `internal/store/memberships/memberships.go`
- Test: `internal/store/memberships/memberships_test.go`

**Interfaces:**
- Produces: `(*MembershipRepository) Delete(ctx, scope domain.TenantScope, userID int64) error`
  — deletes the `(user, tenant)` membership of any status; no-op if none.

- [ ] **Step 1: Write the failing test**

```go
// add to internal/store/memberships/memberships_test.go
func TestMembershipDeleteAnyStatusScoped(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := NewMembershipRepository(pool)

	var uid int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('github:d','github','d','D') RETURNING id`).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tenants (id, slug, name) OVERRIDING SYSTEM VALUE VALUES (2,'t2','T2')`); err != nil {
		t.Fatalf("tenant 2: %v", err)
	}
	if _, err := repo.Upsert(ctx, domain.Membership{UserID: uid, TenantID: 1, Role: domain.RoleAdmin, Status: domain.MembershipActive}); err != nil {
		t.Fatalf("seed t1: %v", err)
	}
	if _, err := repo.Upsert(ctx, domain.Membership{UserID: uid, TenantID: 2, Role: domain.RoleUser, Status: domain.MembershipActive}); err != nil {
		t.Fatalf("seed t2: %v", err)
	}

	if err := repo.Delete(ctx, domain.TenantScope{TenantID: 1}, uid); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repo.Get(ctx, uid, 1); err != ErrNotFound {
		t.Fatalf("t1 membership should be gone, got %v", err)
	}
	if _, err := repo.Get(ctx, uid, 2); err != nil {
		t.Fatalf("t2 membership must survive: %v", err)
	}
	// Idempotent.
	if err := repo.Delete(ctx, domain.TenantScope{TenantID: 1}, uid); err != nil {
		t.Fatalf("second delete should be a no-op: %v", err)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `go test ./internal/store/memberships -run TestMembershipDeleteAnyStatusScoped`.

- [ ] **Step 3: Implement**

```go
// Delete removes the (user, tenant) membership regardless of status. No-op if none.
func (r *MembershipRepository) Delete(ctx context.Context, scope domain.TenantScope, userID int64) error {
	_, err := r.db.Exec(ctx, `DELETE FROM memberships WHERE user_id = $2 AND tenant_id = $1`, scope.TenantID, userID)
	return err
}
```

- [ ] **Step 4: Run** `go test ./internal/store/memberships` → PASS.
- [ ] **Step 5: Stage** (message: `feat(store): delete a tenant membership (any status)`).

---

### Task 2: `GrantRepository.RemoveAllUserGrants` (+ cache mirror)

**Files:**
- Modify: `internal/store/grants/grants.go` (repo method, `grantsBackend` iface, `storeGrantsBackend`, `GrantsCache`)
- Test: `internal/store/grants/grants_test.go` (or the existing grants cache test file)

**Interfaces:**
- Produces:
  - `(*GrantRepository) RemoveAllUserGrants(ctx, scope domain.TenantScope, userID int64) error`
  - `(*GrantsCache) RemoveAllUserGrants(ctx, scope domain.TenantScope, userID int64) error`
    (write-through + cache invalidation; satisfies the `grantRemover` interface used in Task 3).

- [ ] **Step 1: Write the failing test** (repo-level; mirror existing grants tests — find the test
  file and its `SetupDB` usage)

```go
// add to the grants test file (package grants or grants_test, match the existing one)
func TestRemoveAllUserGrantsScoped(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := NewGrantRepository(pool)
	s1 := domain.TenantScope{TenantID: 1}

	// Seed two teams + grants for user 7 in tenant 1, and a grant for user 8.
	var t1, t2 int64
	pool.QueryRow(ctx, `INSERT INTO teams (name, tenant_id) VALUES ('A',1) RETURNING id`).Scan(&t1)
	pool.QueryRow(ctx, `INSERT INTO teams (name, tenant_id) VALUES ('B',1) RETURNING id`).Scan(&t2)
	if err := repo.AddUserGrant(ctx, s1, 7, t1, 1); err != nil { t.Fatal(err) }
	if err := repo.AddUserGrant(ctx, s1, 7, t2, 1); err != nil { t.Fatal(err) }
	if err := repo.AddUserGrant(ctx, s1, 8, t1, 1); err != nil { t.Fatal(err) }

	if err := repo.RemoveAllUserGrants(ctx, s1, 7); err != nil {
		t.Fatalf("remove all: %v", err)
	}
	g7, _ := repo.ListUserGrants(ctx, s1, 7)
	if len(g7) != 0 {
		t.Fatalf("user 7 grants should be gone, got %d", len(g7))
	}
	g8, _ := repo.ListUserGrants(ctx, s1, 8)
	if len(g8) != 1 {
		t.Fatalf("user 8 grants must survive, got %d", len(g8))
	}
}
```

> Check the exact signatures of `NewGrantRepository`, `AddUserGrant`, `ListUserGrants` in
> `grants.go` (all take `scope` as the second arg per Plan 2b) and the test file's package/imports
> before writing; adapt the seeding if `teams` needs more columns.

- [ ] **Step 2: Run to verify it fails.**

- [ ] **Step 3: Implement the repo method** (mirror `RemoveUserGrant`)

```go
// RemoveAllUserGrants deletes every hierarchy grant a user has within the tenant.
func (r *GrantRepository) RemoveAllUserGrants(ctx context.Context, scope domain.TenantScope, userID int64) error {
	_, err := r.db.Exec(ctx, `DELETE FROM user_hierarchy_grants WHERE user_id = $1 AND tenant_id = $2`, userID, scope.TenantID)
	return err
}
```

- [ ] **Step 4: Add it to the cache path** — extend `grantsBackend`, `storeGrantsBackend`, and
  `GrantsCache`:

```go
// in grantsBackend interface:
removeAllUserGrants(ctx context.Context, scope domain.TenantScope, userID int64) error
```
```go
// storeGrantsBackend method:
func (b *storeGrantsBackend) removeAllUserGrants(ctx context.Context, scope domain.TenantScope, userID int64) error {
	return b.r.RemoveAllUserGrants(ctx, scope, userID)
}
```
```go
// GrantsCache method (mirror RemoveUserGrant):
func (c *GrantsCache) RemoveAllUserGrants(ctx context.Context, scope domain.TenantScope, userID int64) error {
	if err := c.backend.removeAllUserGrants(ctx, scope, userID); err != nil {
		return err
	}
	c.invalidate()
	return nil
}
```

> If `grantsBackend` has any other test/fake implementer in the package, add the new method there
> too so it still satisfies the interface (build will tell you).

- [ ] **Step 5: Run** `go test ./internal/store/grants` → PASS (+ `go build ./...`).
- [ ] **Step 6: Stage** (message: `feat(store): remove all of a user's grants in a tenant`).

---

### Task 3: `ProvisioningService.RemoveMember`

**Files:**
- Modify: `internal/service/provisioning.go` (add `grantRemover` dep + `RemoveMember`)
- Modify: `internal/http/server.go` (pass `grantsCache` to `NewProvisioningService`)
- Test: `internal/service/provisioning_test.go`

**Interfaces:**
- Consumes: `MembershipRepository.Delete` (Task 1), `GrantsCache.RemoveAllUserGrants` (Task 2).
- Produces:
  - `NewProvisioningService(..., grants grantRemover)` — new trailing param.
  - `type grantRemover interface { RemoveAllUserGrants(ctx context.Context, scope domain.TenantScope, userID int64) error }`
  - `(*ProvisioningService) RemoveMember(ctx, tenantID, userID int64) error`.

- [ ] **Step 1: Write the failing test** (extend the testcontainers setup; build a real grants cache)

```go
// add to internal/service/provisioning_test.go
func TestProvisioningRemoveMember(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	tnRepo := tenants.NewTenantRepository(pool)
	memRepo := memberships.NewMembershipRepository(pool)
	grantRepo := grants.NewGrantRepository(pool)
	grantsCache := grants.NewGrantsCache(grantRepo)
	tsRepo := tenantsettings.NewTenantSettingsRepository(pool)
	sysRepo := settings.NewSettingsRepository(pool)
	settingsSvc := service.NewSettingsService(
		tenantsettings.NewTenantSettingsCache(tsRepo), tsRepo,
		settings.NewSystemSettingsCache(sysRepo), sysRepo,
	)
	prov := service.NewProvisioningService(tnRepo, tenants.NewTenantCache(tnRepo), memRepo, memberships.NewMembershipCache(memRepo), settingsSvc, grantsCache)

	scope := domain.TenantScope{TenantID: 1}
	var teamID int64
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name, tenant_id) VALUES ('Root',1) RETURNING id`).Scan(&teamID); err != nil {
		t.Fatalf("seed team: %v", err)
	}
	if _, err := memRepo.Upsert(ctx, domain.Membership{UserID: 1, TenantID: 1, Role: domain.RoleUser, Status: domain.MembershipActive}); err != nil {
		t.Fatalf("seed membership: %v", err)
	}
	if err := grantRepo.AddUserGrant(ctx, scope, 1, teamID, 1); err != nil {
		t.Fatalf("seed grant: %v", err)
	}

	if err := prov.RemoveMember(ctx, 1, 1); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := memRepo.Get(ctx, 1, 1); err != memberships.ErrNotFound {
		t.Fatalf("membership should be gone, got %v", err)
	}
	g, _ := grantRepo.ListUserGrants(ctx, scope, 1)
	if len(g) != 0 {
		t.Fatalf("grants should be gone, got %d", len(g))
	}
}
```

- [ ] **Step 2: Run to verify it fails** — compile error (`NewProvisioningService` arity /
  `RemoveMember` undefined).

- [ ] **Step 3: Implement** — add the dep + method in `provisioning.go`:

```go
// grantRemover removes all of a user's grants in a tenant. *grants.GrantsCache satisfies it.
type grantRemover interface {
	RemoveAllUserGrants(ctx context.Context, scope domain.TenantScope, userID int64) error
}
```

  Add field `grants grantRemover` to the struct; add the param to `NewProvisioningService` (last)
  and set `grants: grants` in the literal. Then:

```go
// RemoveMember severs a user's access to a tenant: delete their grants there, then their
// membership (any status). Idempotent.
func (p *ProvisioningService) RemoveMember(ctx context.Context, tenantID, userID int64) error {
	scope := domain.TenantScope{TenantID: tenantID}
	if err := p.grants.RemoveAllUserGrants(ctx, scope, userID); err != nil {
		return err
	}
	if err := p.members.Delete(ctx, scope, userID); err != nil {
		return err
	}
	p.memberCache.InvalidateUser(userID)
	return nil
}
```

- [ ] **Step 4: Update callers** — pass the grants cache:
  - `internal/http/server.go`: `service.NewProvisioningService(st.Tenants, tenantCache, st.Memberships, membershipCache, settingsSvc, grantsCache)` (`grantsCache` is the `NewServer` param).
  - `internal/service/provisioning_test.go` (the existing `TestProvisioningLifecycle` + `TestProvisioningDenyMember`): add a `grants.NewGrantsCache(grants.NewGrantRepository(pool))` and pass it as the 6th arg.
  - `internal/http/handlers/api/v1/system/handler_test.go` `buildRouter`: same — build a grants cache and pass it.

- [ ] **Step 5: Run** `go build ./... && go test ./internal/service ./internal/http/...` → PASS.
- [ ] **Step 6: Stage** (message: `feat(service): RemoveMember severs membership + tenant grants`).

---

### Task 4: System `DELETE …/members/{userID}` endpoint

**Files:**
- Modify: `internal/http/handlers/api/v1/system/handler.go` (`Provisioner` iface + `HandleRemoveMember`)
- Modify: `internal/http/server.go` (mount route)
- Test: `internal/http/handlers/api/v1/system/handler_test.go`

**Interfaces:**
- `Provisioner` gains `RemoveMember(ctx, tenantID, userID int64) error`.
- `(*Handler) HandleRemoveMember` → `DELETE /api/v1/system/tenants/{id}/members/{userID}` → `204`.

- [ ] **Step 1: Add the failing handler test** (register the route in `buildRouter`:
  `r.Delete("/api/v1/system/tenants/{id}/members/{userID}", h.HandleRemoveMember)`)

```go
func TestSystemRemoveMember(t *testing.T) {
	admin := &domain.User{ID: 1, IsSystemAdmin: true}
	r, _, _ := buildRouter(t, admin)
	// Attach user 1 (active), then remove → members list becomes empty.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/system/tenants/1/members",
		strings.NewReader(`{"user_id":1,"role":"admin"}`)))
	if w.Code != http.StatusCreated {
		t.Fatalf("attach: %d", w.Code)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/v1/system/tenants/1/members/1", nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("remove: %d (%s)", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/system/tenants/1/members", nil))
	var members []map[string]any
	_ = json.NewDecoder(w.Body).Decode(&members)
	for _, m := range members {
		if m["user_id"].(float64) == 1 {
			t.Fatalf("user 1 should be removed, still present: %v", members)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails** (`HandleRemoveMember` / `RemoveMember` undefined).

- [ ] **Step 3: Implement** — add to `Provisioner`:
  `RemoveMember(ctx context.Context, tenantID, userID int64) error`, and the handler (mirror
  `HandleDenyMember`):

```go
// DELETE /api/v1/system/tenants/{id}/members/{userID}
func (h *Handler) HandleRemoveMember(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	userID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	if err != nil || userID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if err := h.prov.RemoveMember(r.Context(), id, userID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

  In `server.go` `registerSystemRoutes`, after the deny route:
  `r.Delete("/api/v1/system/tenants/{id}/members/{userID}", sysH.HandleRemoveMember)`.

- [ ] **Step 4: Run** `go build ./... && go test ./internal/http/handlers/api/v1/system` → PASS.
- [ ] **Step 5: Stage** (message: `feat(system): DELETE member endpoint (remove from tenant)`).

---

### Task 5: `system.js` — «Удалить» on active member rows

**Files:**
- Modify: `internal/web/static/system.js` (`MembersSection` + a `del` helper)
- Test: manual (DoD `010`).

- [ ] **Step 1: Add a `del` helper** next to `get/post/put` (top of `system.js`):

```js
const del = (u) => api(u, {method:'DELETE', headers: csrfHeaders()});
```

- [ ] **Step 2: Add «Удалить» to active rows** in `MembersSection` — in the actions `<td>`, add an
  else-branch for non-requested rows:

```jsx
const remove = async (m)=>{ if(!confirm(`Удалить ${m.display_name||m.email||('пользователя #'+m.user_id)} из тенанта?`)) return; setErr(''); const res=await del(`/api/v1/system/tenants/${tid}/members/${m.user_id}`); if(res.status===204) loadMembers(tid); else setErr(await errMsg(res)); };
```
```jsx
<td style={{padding:'6px 8px',textAlign:'right'}}>{m.status==='requested'
  ? <span>
      <button style={{...btn,background:C.ok,marginRight:6}} onClick={()=>connect(m)}>Подключить</button>
      <button style={{...btn,background:C.muted}} onClick={()=>deny(m)}>Отклонить</button>
    </span>
  : <button style={{...btn,background:C.danger}} onClick={()=>remove(m)}>Удалить</button>}</td>
```

  (`C.danger` exists in the theme; `connect`/`deny`/`loadMembers`/`tid`/`setErr`/`errMsg` already
  defined from the previous step.)

- [ ] **Step 3: Build** `go build ./...` → green (templates/static unaffected by Go build, but keep
  the gate).

- [ ] **Step 4: Manual verify (DoD)** — in `/system` → Участники, an active member shows «Удалить»;
  clicking (and confirming) removes them from the list; re-connecting them shows no prior grants
  (access starts empty).

- [ ] **Step 5: Stage** (message: `feat(system-ui): remove member button on active rows`).

---

### Task 6: API spec update

**Files:**
- Modify: `specs/040-api-contract.md`

- [ ] **Step 1: Document the endpoint** — under the System members lines, add:

```markdown
- `DELETE /api/v1/system/tenants/{id}/members/{userID}` — удалить участника из тенанта: убирает membership (любого статуса) **и** все его hierarchy-гранты в этом тенанте → `204`. Идемпотентно. (Кнопка «Удалить» на активных строках; `deny` выше — только для заявок.)
```

- [ ] **Step 2: Run** `go build ./...` (docs sanity) → green.
- [ ] **Step 3: Stage** (message: `docs(specs): system DELETE member endpoint`).

---

## Self-Review Notes

- **Spec coverage:** membership delete (Task 1) + grants cleanup (Task 2) composed by RemoveMember
  (Task 3); HTTP DELETE (Task 4); UI button with confirm (Task 5); specs (Task 6). The "delete
  membership + grants, other tenants untouched, idempotent, no guardrails" semantics are realized
  in Tasks 1–3 and asserted by their tests.
- **Type consistency:** `grantRemover.RemoveAllUserGrants(ctx, scope, userID)` (Task 3) matches
  `GrantsCache.RemoveAllUserGrants` (Task 2). `NewProvisioningService` 6th param `grants` updated
  at all 4 call sites (server + 2 provisioning tests + system buildRouter). `RemoveMember(ctx,
  tenantID, userID)` consistent between service, `Provisioner` iface, and handler. Route uses the
  existing `{id}`/`{userID}` params + `pathID` helper.
- **Coexistence:** `deny` (requested-only, POST `.../deny`) and `RemoveMember` (any status + grants,
  DELETE `.../{userID}`) are separate; UI routes requesters to deny, active members to remove.

## Execution recommendation

Inline, one task at a time. Backend (1–4, 6) is TDD/Go with green gates; frontend (5) is manual
DoD. After each task: `go build ./... && go vet ./... && go test ./...` green (5: build + manual),
then `git add` + propose a commit message; the user commits. Live-verify 5 in a browser when
Docker is available.
