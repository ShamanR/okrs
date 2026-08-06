# Period Overview — Balances, Progress Chart, Team Scope — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend «Обзор периода» with (1) goal balances (discovery/delivery, focuses, priorities) with drill-down, (2) a per-period progress-over-time chart backed by daily snapshots, and (3) a «Мои команды / Вся организация» scope toggle where non-admins see only teams they lead (incl. nested).

**Architecture:** A new scope-aware endpoint `GET /api/v1/periods/{periodID}/overview?scope=my_teams|org` powers the public page. The tenant-wide `HealthCheckInCache` stays as-is; scope is applied as an in-memory `teamFilter` at compute time (no per-user cache, no N+1). Balances + a slim goals list are computed from goals-in-scope in the same pass. Progress history is materialised into a new `team_period_progress_snapshots` table by a daily background goroutine and aggregated per scope at read time, with a live «today» point appended from the current cache.

**Tech Stack:** Go (chi, pgx/pgxpool, golang-migrate, testcontainers-go), in-browser React via `@babel/standalone` (no bundler), inline-style JSX components.

## Global Constraints

- No git commits — the user commits (`CLAUDE.md` §8). Checkpoints = run `go test ./...` and `go vet ./...`, then stop.
- No AI/assistant attribution anywhere (`CLAUDE.md` §5).
- Design docs / specs in Russian (`CLAUDE.md` §11).
- Update specs in the same change set; don't touch unrelated specs (`CLAUDE.md` §2–3).
- No DB queries in loops; consider caching + K8s multi-instance consistency (`CLAUDE.md` §9–10).
- Reuse shared UI components; keep controls consistent (`CLAUDE.md` §12–13).
- Keep seed demo current when table structure changes (`CLAUDE.md` §7).
- Tenant scoping: every new table has `tenant_id BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE` (no `DEFAULT`); every query filters `tenant_id = $scope.TenantID`.
- Test commands: full gate `go test ./... && go vet ./...`; single service test `go test ./internal/service/ -run <Name> -v`; store tests self-skip without Docker (`t.Skipf`).
- Frontend has no test/lint tooling — FE tasks end with a manual browser verification step, not an automated test.
- Enum string values (exact): `WorkType` = `Discovery|Delivery`; `FocusType` = `PROFITABILITY|STABILITY|SPEED_EFFICIENCY|TECH_INDEPENDENCE`; `Priority` = `P0|P1|P2|P3`.

---

## File Structure

**Backend — new**
- `migrations/041_team_period_progress_snapshots.up.sql` / `.down.sql` — snapshot table.
- `internal/store/progresssnap/progresssnap.go` — snapshot repository (upsert bulk, list by period+teams).
- `internal/store/progresssnap/progresssnap_test.go` — testcontainers store test.
- `internal/service/period_progress.go` — daily snapshot job + series aggregation (pure helpers + goroutine).
- `internal/service/period_progress_test.go` — job/aggregation unit tests.

**Backend — modified**
- `internal/store/grants/grants.go` — add `ListLeadTeamScope` (recursive CTE from lead teams) + backend iface + cache passthrough.
- `internal/service/period_overview.go` — `computePeriodOverview` gains `teamFilter`; new `balances`/`goals` DTO + aggregation; scope-aware service method returning the full payload.
- `internal/service/period_overview_test.go` — update existing call; add filter + balances tests.
- `internal/http/handlers/api/v1/admin/service_handler.go` — new `HandlePeriodOverviewScoped`; `ServiceHandler` gains a `scopeResolver` dep.
- `internal/http/handlers/api/v1/admin/period_overview_test.go` — handler tests for scope + authz.
- `internal/http/server.go` — register new route in the protected (non-admin) group; wire `scopeResolver` + start the snapshot goroutine; add snapshot repo to `hcLoader` neighbourhood.
- `internal/store/store.go` — register `ProgressSnap` repo.
- `internal/store/seed.go` + `internal/store/store.go` (`SeedDemo`) — seed snapshot rows.

**Frontend — modified**
- `internal/web/static/period-overview.js` — fetch `/api/v1/config` for `is_admin`; scope toggle; repoint to new endpoint.
- `internal/web/static/period_overview_view.js` — render balances + chart; new components.
- `internal/web/static/balance_bars.js` — new shared `BalanceBars` component.
- `internal/web/static/progress_chart.js` — new shared `ProgressChart` SVG component.
- `internal/http/templates/period_overview_shell.html` — add `<script>` tags for the two new files.

**Specs — modified**
- `specs/040-api-contract.md`, `specs/050-permissions-and-lifecycle.md`, `specs/020-domain-model.md`.

---

# Phase A — Scope foundation

### Task A1: `ListLeadTeamScope` store method

**Files:**
- Modify: `internal/store/grants/grants.go`
- Test: `internal/store/grants/lead_scope_test.go` (create)

**Interfaces:**
- Produces: `(*GrantRepository).ListLeadTeamScope(ctx context.Context, scope domain.TenantScope, userUDID string) ([]int64, error)` — team IDs where `lead_udid = userUDID` (not soft-deleted, tenant-scoped) **plus** all recursive descendants. Empty `userUDID` → `nil, nil`. Also exposed via `grantsBackend` iface and `GrantsCache` passthrough with the same signature.

- [ ] **Step 1: Write the failing store test**

Mirror the testcontainers setup used in `internal/store/store_test.go` (RunContainer → `runMigrations` → `pgxpool.New`; `t.Skipf` if Docker is unavailable). In package `grants` create `lead_scope_test.go`:

```go
func TestListLeadTeamScope_ReturnsLeadTeamsAndDescendants(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := newTestPool(t) // testcontainers helper as in store_test.go; skips w/o Docker
	defer cleanup()
	scope := domain.TenantScope{TenantID: 1}

	// user udid 'u-lead' leads team "Root"; child "Child" has a different lead; "Other" unrelated.
	var root, child, other int64
	must(pool.QueryRow(ctx, `INSERT INTO teams (name, team_type, tenant_id, lead_udid) VALUES ('Root','team',1,'11111111-1111-1111-1111-111111111111') RETURNING id`).Scan(&root))
	must(pool.QueryRow(ctx, `INSERT INTO teams (name, team_type, tenant_id, parent_id) VALUES ('Child','team',1,$1) RETURNING id`, root).Scan(&child))
	must(pool.QueryRow(ctx, `INSERT INTO teams (name, team_type, tenant_id) VALUES ('Other','team',1) RETURNING id`).Scan(&other))

	r := NewGrantRepository(pool)
	ids, err := r.ListLeadTeamScope(ctx, scope, "11111111-1111-1111-1111-111111111111")
	if err != nil { t.Fatalf("err: %v", err) }
	got := map[int64]bool{}
	for _, id := range ids { got[id] = true }
	if !got[root] || !got[child] { t.Fatalf("want root+child in scope, got %v", ids) }
	if got[other] { t.Fatalf("unrelated team leaked into scope: %v", ids) }
}
```

Add local helpers `newTestPool`/`must` copied from the container pattern in `store_test.go` (or reuse an existing helper if the `grants` package already has one — check `grants_isolation_test.go`).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/grants/ -run TestListLeadTeamScope -v`
Expected: FAIL — `ListLeadTeamScope` undefined (or SKIP if Docker absent — then verify compilation with `go vet ./internal/store/grants/`).

- [ ] **Step 3: Implement the method**

Add to `grants.go`, next to `ListDescendantTeamIDs`:

```go
// ListLeadTeamScope returns team IDs the user leads (teams.lead_udid = userUDID,
// not soft-deleted, tenant-scoped) plus all their recursive descendants.
func (r *GrantRepository) ListLeadTeamScope(ctx context.Context, scope domain.TenantScope, userUDID string) ([]int64, error) {
	if userUDID == "" {
		return nil, nil
	}
	rows, err := r.db.Query(ctx, `
		WITH RECURSIVE tree AS (
			SELECT id FROM teams
			WHERE lead_udid = $1 AND deleted_at IS NULL AND tenant_id = $2
			UNION ALL
			SELECT t.id FROM teams t JOIN tree p ON t.parent_id = p.id
			WHERE t.deleted_at IS NULL AND t.tenant_id = $2
		)
		SELECT id FROM tree`, userUDID, scope.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
```

Then expose through the cache: add `ListLeadTeamScope(ctx, scope, userUDID string) ([]int64, error)` to the `grantsBackend` interface (grants.go:121-127), add the delegating method on `storeGrantsBackend` (grants.go:130-146), and a passthrough on `GrantsCache` (grants.go:220-222) mirroring the existing `ListDescendantTeamIDs` passthrough.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/grants/ -run TestListLeadTeamScope -v` → PASS (or SKIP without Docker). Then `go vet ./internal/store/grants/`.

- [ ] **Step 5: Checkpoint**

Run: `go build ./... && go vet ./internal/store/grants/`. Stop for review (no commit).

---

### Task A2: scope-aware `computePeriodOverview`

**Files:**
- Modify: `internal/service/period_overview.go`
- Modify: `internal/service/period_overview_test.go`

**Interfaces:**
- Consumes: `PeriodData` (`internal/service/healthcheckin.go:129`).
- Produces: `computePeriodOverview(data *PeriodData, weightTolerance int, teamFilter map[int64]bool) PeriodOverview` — when `teamFilter` is non-nil, only teams whose `id` is in the set are counted; `nil` = all teams (current behaviour).

- [ ] **Step 1: Update the existing test call + add a filter test**

In `period_overview_test.go`, update every existing `computePeriodOverview(data, 0)` call to `computePeriodOverview(data, 0, nil)`. Then add:

```go
func TestComputePeriodOverview_TeamFilterScopesCounts(t *testing.T) {
	teams, goalsByTeam, statuses := threeTeamsFixture(t) // existing fixture used by the file
	data := &PeriodData{PeriodID: 7, Teams: teams, GoalsByTeam: goalsByTeam, Statuses: statuses}

	filter := map[int64]bool{teams[0].ID: true} // only first team in scope
	ov := computePeriodOverview(data, 0, filter)
	if ov.Summary.TotalTeams != 1 {
		t.Fatalf("scoped total_teams: want 1, got %d", ov.Summary.TotalTeams)
	}
	if len(ov.Teams) != 1 || ov.Teams[0].TeamID != teams[0].ID {
		t.Fatalf("scoped teams mismatch: %+v", ov.Teams)
	}
}
```

If the file builds fixtures inline rather than via a helper, extract the three-team construction into a `threeTeamsFixture(t)` helper first (pure refactor), then use it here.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/ -run TestComputePeriodOverview -v`
Expected: FAIL — signature mismatch / new assertion fails.

- [ ] **Step 3: Add the filter parameter**

In `period_overview.go`, change the signature and skip out-of-scope teams while building `teamsByID`:

```go
func computePeriodOverview(data *PeriodData, weightTolerance int, teamFilter map[int64]bool) PeriodOverview {
	teamsByID := make(map[int64]domain.Team, len(data.Teams))
	for _, t := range data.Teams {
		if t.DeletedAt != nil {
			continue
		}
		if teamFilter != nil && !teamFilter[t.ID] {
			continue
		}
		teamsByID[t.ID] = t
	}
	// ... rest unchanged ...
```

Update the two internal callers: `(*Service).PeriodOverview` (period_overview.go:158) → `computePeriodOverview(data, weightTolerance, nil)`, and `PeriodStats` (period_overview.go:176) → `computePeriodOverview(data, weightTolerance, nil)`.

Note: `buildTeamPath` already reads only `teamsByID`; a scoped team whose ancestor is out of scope will render a shorter path — acceptable (path is display-only).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/service/ -run TestComputePeriodOverview -v` → PASS.

- [ ] **Step 5: Checkpoint**

Run: `go test ./internal/service/ && go vet ./internal/service/`. Stop for review.

---

### Task A3: scope-aware endpoint + authorization

**Files:**
- Modify: `internal/service/period_overview.go` (new service method assembling the payload)
- Modify: `internal/http/handlers/api/v1/admin/service_handler.go`
- Modify: `internal/http/server.go`
- Test: `internal/http/handlers/api/v1/admin/period_overview_test.go`

**Interfaces:**
- Produces (service): `(*Service).PeriodOverviewScoped(ctx, scope domain.TenantScope, periodID int64, weightTolerance int, teamFilter map[int64]bool) (PeriodOverview, error)` — same as `PeriodOverview` but forwards `teamFilter`. (Balances/goals/series are added to the returned struct in later phases.)
- Produces (handler dep): `TeamScopeResolver interface { ListLeadTeamScope(ctx, scope, userUDID string) ([]int64, error) }`, satisfied by `*grants.GrantsCache`.
- Produces (route): `GET /api/v1/periods/{periodID}/overview?scope=my_teams|org` in the **protected** group. Default scope `my_teams`. `scope=org` requires tenant-admin (403 otherwise).

- [ ] **Step 1: Write the failing handler tests**

In a new `internal/http/handlers/api/v1/admin/period_overview_test.go` (same package as existing handler tests; reuse `withTenant`, `withURLParam`, `fakeGrants`):

```go
func TestHandlePeriodOverviewScoped_OrgForbiddenForNonAdmin(t *testing.T) {
	h := NewServiceHandler(service.New(service.Deps{}), nil, &fakeGrants{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/periods/1/overview?scope=org", nil)
	req = withURLParam(withTenant(withUser(req, "u-1", false /*isAdmin*/)), "periodID", "1")
	w := httptest.NewRecorder()
	h.HandlePeriodOverviewScoped(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin org scope, got %d", w.Code)
	}
}

func TestHandlePeriodOverviewScoped_MyTeamsResolvesLeadScope(t *testing.T) {
	// service with a nil cache returns an empty overview (see PeriodOverview guard); we only
	// assert the resolver is consulted and the request succeeds for a non-admin.
	fg := &fakeGrants{leadScope: map[string][]int64{"u-1": {10, 11}}}
	h := NewServiceHandler(service.New(service.Deps{}), nil, fg)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/periods/1/overview?scope=my_teams", nil)
	req = withURLParam(withTenant(withUser(req, "u-1", false)), "periodID", "1")
	w := httptest.NewRecorder()
	h.HandlePeriodOverviewScoped(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	if !fg.leadScopeCalled {
		t.Fatalf("expected ListLeadTeamScope to be consulted for my_teams")
	}
}
```

Add to `fakeGrants` (handler_test.go) the fields/method:

```go
// in fakeGrants
leadScope       map[string][]int64
leadScopeCalled bool

func (f *fakeGrants) ListLeadTeamScope(_ context.Context, _ domain.TenantScope, udid string) ([]int64, error) {
	f.leadScopeCalled = true
	return f.leadScope[udid], nil
}
```

Add a `withUser(req, udid string, isAdmin bool)` helper next to `withTenant` (handler_test.go) that injects the auth user + role into context using the same `auth.With*` helpers the codebase already uses (look at how `auth.UserFromContext` and role are populated elsewhere, e.g. goals handler tests, and mirror it).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/http/handlers/api/v1/admin/ -run TestHandlePeriodOverviewScoped -v`
Expected: FAIL — `HandlePeriodOverviewScoped` / new `NewServiceHandler` arity undefined.

- [ ] **Step 3: Add the service method**

In `period_overview.go`:

```go
// PeriodOverviewScoped is PeriodOverview restricted to teamFilter (nil = whole org).
func (s *Service) PeriodOverviewScoped(ctx context.Context, scope domain.TenantScope, periodID int64, weightTolerance int, teamFilter map[int64]bool) (PeriodOverview, error) {
	if s.hcCache == nil {
		return PeriodOverview{PeriodID: periodID}, nil
	}
	data, err := s.hcCache.Get(ctx, scope, periodID)
	if err != nil {
		return PeriodOverview{}, err
	}
	return computePeriodOverview(data, weightTolerance, teamFilter), nil
}
```

- [ ] **Step 4: Add the resolver dep + handler**

In `service_handler.go`: add the interface and field, extend the constructor.

```go
type TeamScopeResolver interface {
	ListLeadTeamScope(ctx context.Context, scope domain.TenantScope, userUDID string) ([]int64, error)
}

// add field: scope TeamScopeResolver
func NewServiceHandler(svc *service.Service, settings *settings.Service, scope TeamScopeResolver) *ServiceHandler {
	return &ServiceHandler{service: svc, settings: settings, scope: scope}
}
```

Add the handler (mirror `HandlePeriodOverview` for scope/period parsing; read `auth.UserFromContext` for UDID and the role/admin flag the same way goals handler does):

```go
// GET /api/v1/periods/{periodID}/overview?scope=my_teams|org
func (h *ServiceHandler) HandlePeriodOverviewScoped(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "no active tenant", nil)
		return
	}
	periodID, err := common.ParseID(chi.URLParam(r, "periodID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid period id", nil)
		return
	}
	overviewScope := r.URL.Query().Get("scope")
	if overviewScope == "" {
		overviewScope = "my_teams"
	}

	user := auth.UserFromContext(r.Context())
	isAdmin := auth.RoleFromContext(r.Context()) == domain.RoleAdmin // use the same accessor goals handler uses

	var teamFilter map[int64]bool
	switch overviewScope {
	case "org":
		if !isAdmin {
			v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "org scope requires admin", nil)
			return
		}
		teamFilter = nil
	case "my_teams":
		udid := ""
		if user != nil {
			udid = user.UDID
		}
		ids, err := h.scope.ListLeadTeamScope(r.Context(), scope, udid)
		if err != nil {
			v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to resolve scope", nil)
			return
		}
		teamFilter = make(map[int64]bool, len(ids))
		for _, id := range ids {
			teamFilter[id] = true
		}
	default:
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid scope", nil)
		return
	}

	ov, err := h.service.PeriodOverviewScoped(r.Context(), scope, periodID, h.weightTolerance(r, scope), teamFilter)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to load overview", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, ov)
}
```

Confirm the exact role accessor: grep for how `internal/http/handlers/api/v1/goals/handler.go:316` obtains `role` (`isAdmin := role == domain.RoleAdmin`) and reuse that accessor verbatim.

- [ ] **Step 5: Register route + wire the dep**

In `server.go`: update the `NewServiceHandler(...)` call (currently `apiadmin.NewServiceHandler(s.service, s.settingsSvc)`) to pass `s.store.Grants` as the resolver. Register the new route **inside the protected group** — put it in `registerApiRoutes` (the non-admin authenticated group), e.g.:

```go
r.Get("/api/v1/periods/{periodID}/overview", serviceH.HandlePeriodOverviewScoped)
```

Ensure `serviceH` is reachable there (construct it in `Routes()`/`registerApiRoutes` scope, or pass it in). Keep the existing admin route `/api/v1/admin/periods/{periodID}/overview` unchanged.

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/http/... -run TestHandlePeriodOverviewScoped -v` → PASS. Then `go build ./...`.

- [ ] **Step 7: Checkpoint**

Run: `go test ./... && go vet ./...`. Stop for review.

---

### Task A4: frontend scope toggle

**Files:**
- Modify: `internal/web/static/period-overview.js`
- Modify: `internal/web/static/period_overview_view.js` (render the toggle in the header)

**Interfaces:**
- Consumes: `GET /api/v1/periods/{id}/overview?scope=`, `GET /api/v1/config` (`is_admin`).

- [ ] **Step 1: Fetch config + hold scope state**

In `period-overview.js` `App()`: add `const [isAdmin, setIsAdmin] = useState(false);` and `const [scopeSel, setScopeSel] = useState('my_teams');`. Extend the initial `Promise.all` to include `apiGet('/api/v1/config')` and `if (cfg) setIsAdmin(!!cfg.is_admin);` (mirror `tracker.js:2259`).

- [ ] **Step 2: Repoint the overview fetch to the new endpoint + scope**

Change the `load(pid)` fetch URL to:

```js
apiGet(`/api/v1/periods/${pid}/overview?scope=${scopeSel}`)
```

and refetch when scope changes: add `scopeSel` to the effect deps that call `load`, i.e. `useEffect(() => { periodIdRef.current = periodId; load(periodId); }, [periodId, scopeSel]);`.

- [ ] **Step 3: Render the toggle**

Pass `isAdmin`, `scopeSel`, `onScope={setScopeSel}` into `PeriodOverviewContent`. In `period_overview_view.js`, render a segmented control in the header (inline styles consistent with the `PO` palette). Show the «Вся организация» segment only when `isAdmin`:

```jsx
function ScopeToggle({ isAdmin, value, onChange }) {
  const seg = (key, label) => (
    <button onClick={() => onChange(key)}
      style={{ padding: '6px 12px', border: 'none', cursor: 'pointer', fontWeight: 600,
               background: value === key ? ACCENT : 'transparent',
               color: value === key ? '#fff' : PO.mutedFg, borderRadius: 8 }}>
      {label}
    </button>
  );
  return (
    <div style={{ display: 'inline-flex', gap: 4, background: '#f1f5f9', padding: 4, borderRadius: 10 }}>
      {seg('my_teams', 'Мои команды')}
      {isAdmin && seg('org', 'Вся организация')}
    </div>
  );
}
```

Render `<ScopeToggle isAdmin={isAdmin} value={scope} onChange={onScope} />` in the header row (top-right, per the screenshot).

- [ ] **Step 4: Add script load order (if needed)**

No new files here, so no template change. Confirm `period_overview_view.js` still loads before `period-overview.js` in `period_overview_shell.html` (it does).

- [ ] **Step 5: Manual browser verification**

Run the app (`/run` skill or `go run ./cmd/server --seed`). As admin: both segments visible, default «Мои команды», switching to «Вся организация» refetches and changes counts. As a non-admin lead (set a team's `lead_udid`): only «Мои команды», data limited to their subtree. Stop for review.

---

# Phase B — Goal balances

### Task B1: balances + slim goals aggregation

**Files:**
- Modify: `internal/service/period_overview.go`
- Modify: `internal/service/period_overview_test.go`

**Interfaces:**
- Produces (DTO, added to `PeriodOverview`):

```go
type BalanceBucket struct {
	Key     string `json:"key"`
	Count   int    `json:"count"`
	Percent int    `json:"percent"`
}
type PeriodBalances struct {
	DiscoveryDelivery []BalanceBucket `json:"discovery_delivery"`
	Focuses           []BalanceBucket `json:"focuses"`
	Priorities        []BalanceBucket `json:"priorities"`
}
type PeriodGoalItem struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	TeamID    int64  `json:"team_id"`
	TeamName  string `json:"team_name"`
	WorkType  string `json:"work_type"`
	FocusType string `json:"focus_type"`
	Priority  string `json:"priority"`
	Progress  int    `json:"progress"`
}
// PeriodOverview gains: Balances PeriodBalances `json:"balances"`; Goals []PeriodGoalItem `json:"goals"`.
```

- Produces (func): `computeBalances(goals []PeriodGoalItem) PeriodBalances` — fixed category order, zero categories present, `percent = round(count/len(goals)*100)`, base = total goals in scope; dedupe by goal ID happens before this call.

- [ ] **Step 1: Write the failing test**

```go
func TestComputeBalances_CountsAndPercentsWithFixedOrder(t *testing.T) {
	goals := []PeriodGoalItem{
		{ID: 1, WorkType: "Delivery", FocusType: "STABILITY", Priority: "P1"},
		{ID: 2, WorkType: "Delivery", FocusType: "TECH_INDEPENDENCE", Priority: "P1"},
		{ID: 3, WorkType: "Discovery", FocusType: "PROFITABILITY", Priority: "P2"},
	}
	b := computeBalances(goals)
	if b.DiscoveryDelivery[0].Key != "Delivery" || b.DiscoveryDelivery[0].Count != 2 {
		t.Fatalf("delivery bucket: %+v", b.DiscoveryDelivery)
	}
	if b.DiscoveryDelivery[0].Percent != 67 { // round(2/3*100)
		t.Fatalf("delivery percent: %d", b.DiscoveryDelivery[0].Percent)
	}
	if len(b.Priorities) != 4 || b.Priorities[0].Key != "P0" || b.Priorities[0].Count != 0 {
		t.Fatalf("priorities must list P0..P3 incl zero: %+v", b.Priorities)
	}
	if len(b.Focuses) != 4 {
		t.Fatalf("focuses must list all 4 categories: %+v", b.Focuses)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/ -run TestComputeBalances -v` → FAIL (undefined).

- [ ] **Step 3: Implement `computeBalances` + wire into the overview build**

Add to `period_overview.go`:

```go
var (
	workTypeOrder = []string{"Delivery", "Discovery"}
	focusOrder    = []string{"PROFITABILITY", "STABILITY", "SPEED_EFFICIENCY", "TECH_INDEPENDENCE"}
	priorityOrder = []string{"P0", "P1", "P2", "P3"}
)

func bucketsFor(order []string, counts map[string]int, total int) []BalanceBucket {
	out := make([]BalanceBucket, 0, len(order))
	for _, k := range order {
		c := counts[k]
		pct := 0
		if total > 0 {
			pct = int(math.Round(float64(c) / float64(total) * 100))
		}
		out = append(out, BalanceBucket{Key: k, Count: c, Percent: pct})
	}
	return out
}

func computeBalances(goals []PeriodGoalItem) PeriodBalances {
	wt, ft, pr := map[string]int{}, map[string]int{}, map[string]int{}
	for _, g := range goals {
		wt[g.WorkType]++
		ft[g.FocusType]++
		pr[g.Priority]++
	}
	total := len(goals)
	return PeriodBalances{
		DiscoveryDelivery: bucketsFor(workTypeOrder, wt, total),
		Focuses:           bucketsFor(focusOrder, ft, total),
		Priorities:        bucketsFor(priorityOrder, pr, total),
	}
}
```

Now build the slim goals list inside `computePeriodOverview`, deduped by goal ID (shared goals appear under multiple teams). Inside the per-team loop where `goals` are already progress-calculated, append to a scope-level accumulator guarded by a `seen := map[int64]bool`:

```go
// before the loop:
seen := make(map[int64]bool)
goalItems := make([]PeriodGoalItem, 0)
// inside the loop, after CalculateGoalProgress for goals[i]:
if !seen[goals[i].ID] {
	seen[goals[i].ID] = true
	goalItems = append(goalItems, PeriodGoalItem{
		ID: goals[i].ID, Title: goals[i].Title,
		TeamID: id, TeamName: team.Name,
		WorkType: string(goals[i].WorkType), FocusType: string(goals[i].FocusType),
		Priority: string(goals[i].Priority), Progress: goals[i].Progress,
	})
}
```

Then set on the returned struct: `Balances: computeBalances(goalItems), Goals: goalItems`. (Verify `domain.Goal` field names `ID`, `Title`, `WorkType`, `FocusType`, `Priority` in `internal/domain/models.go`; adjust if `Title` differs.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/service/ -run 'TestComputeBalances|TestComputePeriodOverview' -v` → PASS.

- [ ] **Step 5: Checkpoint**

Run: `go test ./internal/service/ && go vet ./internal/service/`. Stop for review.

---

### Task B2: `BalanceBars` component + render

**Files:**
- Create: `internal/web/static/balance_bars.js`
- Modify: `internal/web/static/period_overview_view.js`
- Modify: `internal/http/templates/period_overview_shell.html`

**Interfaces:**
- Consumes: `data.balances` (`discovery_delivery|focuses|priorities`), `data.goals`.
- Produces: global `BalanceBars({ title, subtitle, items, labels, colors, onSelect })`.

- [ ] **Step 1: Create the component**

`balance_bars.js` (bare-global, inline-style, follows `ProgressBar`/`period_overview_view.js` conventions):

```jsx
// BalanceBars — reusable horizontal bar balance with click-to-drill. Bare global (no bundler).
function BalanceBars({ title, subtitle, items, labels, colors, onSelect }) {
  const max = Math.max(1, ...items.map(i => i.count));
  return (
    <div style={{ flex: '1 1 260px', minWidth: 240 }}>
      <div style={{ fontWeight: 700, fontSize: 15, color: '#0f172a' }}>{title}</div>
      {subtitle && <div style={{ fontSize: 12, color: '#64748b', marginBottom: 10 }}>{subtitle}</div>}
      {items.map(it => (
        <div key={it.key} onClick={() => onSelect && onSelect(it.key)}
             style={{ display: 'grid', gridTemplateColumns: '130px 1fr 70px', alignItems: 'center',
                      gap: 10, padding: '4px 0', cursor: onSelect ? 'pointer' : 'default' }}>
          <div style={{ fontSize: 13, color: it.count ? '#0f172a' : '#94a3b8' }}>{(labels && labels[it.key]) || it.key}</div>
          <div style={{ height: 10, background: '#eef2f7', borderRadius: 6 }}>
            <div style={{ width: `${(it.count / max) * 100}%`, height: '100%', borderRadius: 6,
                          background: (colors && colors[it.key]) || '#7c6cf0' }} />
          </div>
          <div style={{ fontSize: 13, textAlign: 'right', color: '#334155' }}>
            <b>{it.count}</b> · {it.percent}%
          </div>
        </div>
      ))}
    </div>
  );
}
```

- [ ] **Step 2: Add the script tag**

In `period_overview_shell.html`, add before `period_overview_view.js`:

```html
<script type="text/babel" src="/static/balance_bars.js" data-presets="react"></script>
```

- [ ] **Step 3: Render three balances + drill-down**

In `period_overview_view.js`, add a «Балансы целей» section that renders three `BalanceBars` with fixed labels/colors matching the screenshot:

```jsx
const DD_LABELS = { Delivery: 'Delivery', Discovery: 'Discovery' };
const FOCUS_LABELS = { PROFITABILITY: 'Profitability', STABILITY: 'Stability', SPEED_EFFICIENCY: 'Speed Efficiency', TECH_INDEPENDENCE: 'Tech Independency' };
const PRIO_LABELS = { P0: 'P0 · критично', P1: 'P1 · высокий', P2: 'P2 · средний', P3: 'P3 · низкий' };
// ... render with data.balances.discovery_delivery / .focuses / .priorities
// onSelect(key) → setDrill({ kind: 'goals', title, filter: { field, key } })
```

Reuse the existing drill-down panel: when `drill.kind === 'goals'`, filter `data.goals` by the selected field/key and list `{title, team_name, progress}` rows (reuse the row styling already present for the status drill-down).

- [ ] **Step 4: Manual browser verification**

Run the app; confirm three bar groups render with counts/percents matching the screenshot, and clicking a bar lists the goals in that bucket. Stop for review.

---

# Phase C — Progress-over-time chart

### Task C1: snapshot table migration

**Files:**
- Create: `migrations/041_team_period_progress_snapshots.up.sql`
- Create: `migrations/041_team_period_progress_snapshots.down.sql`

- [ ] **Step 1: Write the up migration**

`041_team_period_progress_snapshots.up.sql` (mirror the `039_activity_events` style):

```sql
-- Daily materialised progress per team per period, for the period progress chart.
CREATE TABLE team_period_progress_snapshots (
    id            BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    tenant_id     BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    period_id     BIGINT NOT NULL REFERENCES periods(id) ON DELETE CASCADE,
    team_id       BIGINT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    snapshot_date DATE   NOT NULL,
    progress      INTEGER NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, period_id, team_id, snapshot_date)
);
CREATE INDEX idx_tpps_tenant_period_date ON team_period_progress_snapshots(tenant_id, period_id, snapshot_date);
```

- [ ] **Step 2: Write the down migration**

`041_team_period_progress_snapshots.down.sql`:

```sql
DROP TABLE team_period_progress_snapshots;
```

- [ ] **Step 3: Verify migrations apply**

Run: `go test ./internal/store/ -run TestMigrations -v` if such a test exists; otherwise this is verified transitively by the store test in Task C2 (which runs all migrations). Stop for review.

---

### Task C2: snapshot repository

**Files:**
- Create: `internal/store/progresssnap/progresssnap.go`
- Create: `internal/store/progresssnap/progresssnap_test.go`
- Modify: `internal/store/store.go`

**Interfaces:**
- Produces:
  - `NewRepository(db *pgxpool.Pool) *Repository`
  - `type Snapshot struct { TeamID int64; Progress int }`
  - `(*Repository).UpsertSnapshots(ctx, scope domain.TenantScope, periodID int64, day time.Time, snaps []Snapshot) error` — bulk upsert on `(tenant_id, period_id, team_id, snapshot_date)`.
  - `type SeriesRow struct { TeamID int64; Date time.Time; Progress int }`
  - `(*Repository).ListSnapshots(ctx, scope domain.TenantScope, periodID int64, teamIDs []int64) ([]SeriesRow, error)` — all rows for the period restricted to `teamIDs` (empty → all teams in period), ordered by date.
- Registered as `st.ProgressSnap` in `store.New`.

- [ ] **Step 1: Write the failing store test**

`progresssnap_test.go` (testcontainers, self-skip w/o Docker; mirror `store_test.go`):

```go
func TestUpsertAndListSnapshots(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := newTestPool(t)
	defer cleanup()
	scope := domain.TenantScope{TenantID: 1}
	periodID, teamID := seedPeriodAndTeam(t, pool) // insert a period + team, return ids

	r := NewRepository(pool)
	day := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	must(r.UpsertSnapshots(ctx, scope, periodID, day, []Snapshot{{TeamID: teamID, Progress: 20}}))
	// idempotent: second upsert same day updates, no dup
	must(r.UpsertSnapshots(ctx, scope, periodID, day, []Snapshot{{TeamID: teamID, Progress: 35}}))

	rows, err := r.ListSnapshots(ctx, scope, periodID, []int64{teamID})
	if err != nil { t.Fatalf("list: %v", err) }
	if len(rows) != 1 { t.Fatalf("want 1 row after idempotent upsert, got %d", len(rows)) }
	if rows[0].Progress != 35 { t.Fatalf("want updated progress 35, got %d", rows[0].Progress) }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/progresssnap/ -v` → FAIL/undefined (or SKIP w/o Docker; then `go vet`).

- [ ] **Step 3: Implement the repository**

```go
package progresssnap

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"okrs/internal/domain"
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

type Snapshot struct {
	TeamID   int64
	Progress int
}

func (r *Repository) UpsertSnapshots(ctx context.Context, scope domain.TenantScope, periodID int64, day time.Time, snaps []Snapshot) error {
	if len(snaps) == 0 {
		return nil
	}
	teamIDs := make([]int64, len(snaps))
	progs := make([]int32, len(snaps))
	for i, s := range snaps {
		teamIDs[i] = s.TeamID
		progs[i] = int32(s.Progress)
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO team_period_progress_snapshots (tenant_id, period_id, team_id, snapshot_date, progress)
		SELECT $1, $2, t.team_id, $4::date, t.progress
		FROM unnest($3::bigint[], $5::int[]) AS t(team_id, progress)
		ON CONFLICT (tenant_id, period_id, team_id, snapshot_date)
		DO UPDATE SET progress = EXCLUDED.progress`,
		scope.TenantID, periodID, teamIDs, day, progs)
	return err
}

type SeriesRow struct {
	TeamID   int64
	Date     time.Time
	Progress int
}

func (r *Repository) ListSnapshots(ctx context.Context, scope domain.TenantScope, periodID int64, teamIDs []int64) ([]SeriesRow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT team_id, snapshot_date, progress
		FROM team_period_progress_snapshots
		WHERE tenant_id = $1 AND period_id = $2
		  AND ($3::bigint[] IS NULL OR team_id = ANY($3))
		ORDER BY snapshot_date`, scope.TenantID, periodID, nilIfEmpty(teamIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SeriesRow
	for rows.Next() {
		var sr SeriesRow
		if err := rows.Scan(&sr.TeamID, &sr.Date, &sr.Progress); err != nil {
			return nil, err
		}
		out = append(out, sr)
	}
	return out, rows.Err()
}

func nilIfEmpty(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}
	return ids
}
```

Register in `store.go`: add field `ProgressSnap *progresssnap.Repository` to `Store` (store.go:33-52) and `ProgressSnap: progresssnap.NewRepository(db),` in `New` (store.go:56-76).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/progresssnap/ -v` → PASS (or SKIP w/o Docker). Then `go build ./...`.

- [ ] **Step 5: Checkpoint**

Run: `go test ./... && go vet ./...`. Stop for review.

---

### Task C3: daily snapshot goroutine

**Files:**
- Create: `internal/service/period_progress.go`
- Create: `internal/service/period_progress_test.go`
- Modify: `internal/http/server.go`

**Interfaces:**
- Consumes: `hcCache.Get`, `okr.PeriodProgress`, `CalculateGoalProgress`, `progresssnap.Repository`.
- Produces:
  - `computeTeamSnapshots(data *PeriodData) []progresssnap.Snapshot` — one entry per active team **with goals** (skip soft-deleted / no-goals), progress via the same math as `computePeriodOverview`.
  - `(*Service).SnapshotActivePeriods(ctx, day time.Time, actives []HCActive) error` — for each active period, load `PeriodData`, compute, and upsert. (The advisory-lock gate and ticker live in server.go.)

- [ ] **Step 1: Write the failing test for the pure computation**

```go
func TestComputeTeamSnapshots_SkipsNoGoalAndDeletedTeams(t *testing.T) {
	teams, goalsByTeam, statuses := threeTeamsFixture(t) // team[2] has no goals; add a deleted team
	data := &PeriodData{PeriodID: 7, Teams: teams, GoalsByTeam: goalsByTeam, Statuses: statuses}
	snaps := computeTeamSnapshots(data)
	for _, s := range snaps {
		if len(data.GoalsByTeam[s.TeamID]) == 0 {
			t.Fatalf("snapshot emitted for team without goals: %d", s.TeamID)
		}
	}
	if len(snaps) == 0 {
		t.Fatalf("expected snapshots for teams with goals")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/ -run TestComputeTeamSnapshots -v` → FAIL (undefined).

- [ ] **Step 3: Implement the computation + orchestration**

`period_progress.go`:

```go
package service

import (
	"context"
	"time"

	"okrs/internal/domain"
	"okrs/internal/okr"
	"okrs/internal/store/progresssnap"
)

// computeTeamSnapshots computes current progress per active team-with-goals (no I/O).
func computeTeamSnapshots(data *PeriodData) []progresssnap.Snapshot {
	out := make([]progresssnap.Snapshot, 0, len(data.Teams))
	for _, team := range data.Teams {
		if team.DeletedAt != nil {
			continue
		}
		src := data.GoalsByTeam[team.ID]
		if len(src) == 0 {
			continue
		}
		goals := make([]domain.Goal, len(src))
		copy(goals, src)
		for i := range goals {
			goals[i].Progress = CalculateGoalProgress(&goals[i])
		}
		out = append(out, progresssnap.Snapshot{TeamID: team.ID, Progress: okr.PeriodProgress(goals)})
	}
	return out
}

// SnapshotActivePeriods materialises today's per-team progress for each active period.
func (s *Service) SnapshotActivePeriods(ctx context.Context, day time.Time, actives []HCActive) error {
	if s.hcCache == nil || s.progressSnap == nil {
		return nil
	}
	for _, a := range actives {
		if a.PeriodID == 0 {
			continue
		}
		data, err := s.hcCache.Get(ctx, a.Scope, a.PeriodID)
		if err != nil {
			continue // best-effort; logged by caller
		}
		snaps := computeTeamSnapshots(data)
		if err := s.progressSnap.UpsertSnapshots(ctx, a.Scope, a.PeriodID, day, snaps); err != nil {
			return err
		}
	}
	return nil
}
```

Add a `progressSnap *progresssnap.Repository` field to `Service` and wire it in `service.New`/`service.Deps` (mirror how `hcCache` is injected — grep `hcCache` in `internal/service/service.go` and add the field the same way; add `ProgressSnap` to `Deps`).

- [ ] **Step 4: Wire the goroutine + advisory lock in server.go**

Next to the `StartRefreshLoop` call (server.go:246), start a daily loop. Advisory lock is net-new (no helper exists) — acquire a session-scoped `pg_try_advisory_lock` on a fixed key so only one replica runs the pass:

```go
go func() {
	ctx := context.Background()
	run := func() {
		conn, err := s.store.DB.Acquire(ctx)
		if err != nil { return }
		defer conn.Release()
		var got bool
		if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(918273645)`).Scan(&got); err != nil || !got {
			return // another replica holds the lock this cycle
		}
		defer conn.Exec(ctx, `SELECT pg_advisory_unlock(918273645)`)

		now := time.Now().In(s.zone)
		day := now
		var actives []service.HCActive
		tenants, err := s.store.Tenants.List(ctx)
		if err != nil { return }
		for _, tn := range tenants {
			scope := domain.TenantScope{TenantID: tn.ID}
			p, err := s.service.FindPeriodForDate(ctx, scope, now)
			if err != nil { continue }
			actives = append(actives, service.HCActive{Scope: scope, PeriodID: p.ID})
		}
		if err := s.service.SnapshotActivePeriods(ctx, day, actives); err != nil && s.logger != nil {
			s.logger.Warn("progress snapshot failed", "err", err)
		}
	}
	run() // initial run at startup captures today
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for range ticker.C { run() }
}()
```

`FindPeriodForDate` returns the active (date-based) period, so closed/archived periods are naturally skipped. `computeTeamSnapshots` skips no-goal/deleted teams. Confirm `s.store.DB` is a `*pgxpool.Pool` (it is — store.go:34) and `s.zone`/`s.logger` exist on `Server` (they're used by the existing refresh loop).

- [ ] **Step 5: Run tests + build**

Run: `go test ./internal/service/ -run TestComputeTeamSnapshots -v` → PASS. Then `go build ./... && go vet ./...`.

- [ ] **Step 6: Checkpoint**

Run: `go test ./... && go vet ./...`. Stop for review.

---

### Task C4: progress series in the overview payload

**Files:**
- Modify: `internal/service/period_overview.go` (assemble series into `PeriodOverviewScoped`)
- Create/extend: `internal/service/period_progress.go` (`buildProgressSeries`)
- Modify: `internal/service/period_progress_test.go`

**Interfaces:**
- Produces (DTO on `PeriodOverview`):

```go
type SeriesPoint struct {
	Date     string `json:"date"`     // YYYY-MM-DD
	Progress int    `json:"progress"`
}
type ProgressSeries struct {
	PeriodStart string        `json:"period_start"` // YYYY-MM-DD
	PeriodEnd   string        `json:"period_end"`
	Points      []SeriesPoint `json:"points"`
}
// PeriodOverview gains: Progress ProgressSeries `json:"progress"`.
```

- Produces (func): `buildProgressSeries(rows []progresssnap.SeriesRow, teamFilter map[int64]bool, today string, todayAvg int, start, end time.Time) ProgressSeries` — for each date, average progress across teams-in-scope that have a snapshot that date; overwrite/insert the `today` point with the live `todayAvg`; points sorted by date. `teamFilter == nil` → all teams.

- [ ] **Step 1: Write the failing test**

```go
func TestBuildProgressSeries_AveragesByDateAndAppendsToday(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	d1 := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	rows := []progresssnap.SeriesRow{
		{TeamID: 1, Date: d1, Progress: 20},
		{TeamID: 2, Date: d1, Progress: 40},
	}
	s := buildProgressSeries(rows, nil, "2026-02-01", 55, start, end)
	if s.PeriodStart != "2026-01-01" || s.PeriodEnd != "2026-03-31" {
		t.Fatalf("period bounds: %+v", s)
	}
	if s.Points[0].Date != "2026-01-10" || s.Points[0].Progress != 30 { // avg(20,40)
		t.Fatalf("date avg: %+v", s.Points[0])
	}
	last := s.Points[len(s.Points)-1]
	if last.Date != "2026-02-01" || last.Progress != 55 {
		t.Fatalf("live today point: %+v", last)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/ -run TestBuildProgressSeries -v` → FAIL (undefined).

- [ ] **Step 3: Implement `buildProgressSeries`**

```go
func buildProgressSeries(rows []progresssnap.SeriesRow, teamFilter map[int64]bool, today string, todayAvg int, start, end time.Time) ProgressSeries {
	type acc struct{ sum, n int }
	byDate := map[string]*acc{}
	for _, r := range rows {
		if teamFilter != nil && !teamFilter[r.TeamID] {
			continue
		}
		key := r.Date.Format("2006-01-02")
		a := byDate[key]
		if a == nil {
			a = &acc{}
			byDate[key] = a
		}
		a.sum += r.Progress
		a.n++
	}
	// live today point overrides any snapshot for today
	byDate[today] = &acc{sum: todayAvg, n: 1}

	dates := make([]string, 0, len(byDate))
	for d := range byDate {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	points := make([]SeriesPoint, 0, len(dates))
	for _, d := range dates {
		a := byDate[d]
		points = append(points, SeriesPoint{Date: d, Progress: int(math.Round(float64(a.sum) / float64(a.n)))})
	}
	return ProgressSeries{
		PeriodStart: start.Format("2006-01-02"),
		PeriodEnd:   end.Format("2006-01-02"),
		Points:      points,
	}
}
```

- [ ] **Step 4: Assemble series into the scoped overview**

Change `PeriodOverviewScoped` to also load snapshots and attach the series. It needs `s.progressSnap`, the current period bounds (`data.Period.StartDate/EndDate`), today's date, and today's live scoped average (reuse `computePeriodOverview(...).Summary.AvgProgress`):

```go
func (s *Service) PeriodOverviewScoped(ctx context.Context, scope domain.TenantScope, periodID int64, weightTolerance int, teamFilter map[int64]bool) (PeriodOverview, error) {
	if s.hcCache == nil {
		return PeriodOverview{PeriodID: periodID}, nil
	}
	data, err := s.hcCache.Get(ctx, scope, periodID)
	if err != nil {
		return PeriodOverview{}, err
	}
	ov := computePeriodOverview(data, weightTolerance, teamFilter)
	if s.progressSnap != nil {
		rows, err := s.progressSnap.ListSnapshots(ctx, scope, periodID, keysOf(teamFilter))
		if err != nil {
			return PeriodOverview{}, err
		}
		today := time.Now().Format("2006-01-02")
		ov.Progress = buildProgressSeries(rows, teamFilter, today, ov.Summary.AvgProgress, data.Period.StartDate, data.Period.EndDate)
	}
	return ov, nil
}

func keysOf(m map[int64]bool) []int64 {
	if m == nil {
		return nil
	}
	out := make([]int64, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
```

(Confirm `domain.Period` exposes `StartDate`/`EndDate` as `time.Time` — periods.go scans them; adjust `.Format` if they are a nullable type.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/service/ -run 'TestBuildProgressSeries|TestComputePeriodOverview' -v` → PASS.

- [ ] **Step 6: Checkpoint**

Run: `go test ./... && go vet ./...`. Stop for review.

---

### Task C5: `ProgressChart` SVG component + render

**Files:**
- Create: `internal/web/static/progress_chart.js`
- Modify: `internal/web/static/period_overview_view.js`
- Modify: `internal/http/templates/period_overview_shell.html`

**Interfaces:**
- Consumes: `data.progress` (`{ period_start, period_end, points: [{date, progress}] }`).
- Produces: global `ProgressChart({ series })`.

- [ ] **Step 1: Create the component**

`progress_chart.js` — X = period start→end, Y = 0–100%, dashed diagonal reference, polyline + area through points, out-of-range points clamped to the left/right edge as diamonds:

```jsx
// ProgressChart — period progress over time. Bare global (no bundler).
function ProgressChart({ series }) {
  if (!series || !series.points || !series.points.length) {
    return <div style={{ color: '#94a3b8', padding: 24 }}>Нет данных о прогрессе за период.</div>;
  }
  const W = 900, H = 320, padL = 40, padR = 20, padT = 20, padB = 30;
  const x0 = padL, x1 = W - padR, y0 = H - padB, y1 = padT;
  const start = Date.parse(series.period_start), end = Date.parse(series.period_end);
  const span = Math.max(1, end - start);
  const xOf = (dateStr) => {
    const t = Date.parse(dateStr);
    const clamped = Math.min(Math.max(t, start), end); // pre-start/post-end → edge
    return x0 + ((clamped - start) / span) * (x1 - x0);
  };
  const yOf = (p) => y0 + (Math.min(Math.max(p, 0), 100) / 100) * (y1 - y0);
  const pts = series.points.map(pt => ({ x: xOf(pt.date), y: yOf(pt.progress), edge: Date.parse(pt.date) < start || Date.parse(pt.date) > end, p: pt.progress }));
  const line = pts.map((p, i) => `${i ? 'L' : 'M'}${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(' ');
  const area = `${line} L${pts[pts.length - 1].x.toFixed(1)},${y0} L${pts[0].x.toFixed(1)},${y0} Z`;
  const grid = [0, 25, 50, 75, 100];
  return (
    <svg viewBox={`0 0 ${W} ${H}`} style={{ width: '100%', height: 'auto' }}>
      {grid.map(g => (
        <g key={g}>
          <line x1={x0} y1={yOf(g)} x2={x1} y2={yOf(g)} stroke="#eef2f7" />
          <text x={x0 - 6} y={yOf(g) + 3} textAnchor="end" fontSize="10" fill="#94a3b8">{g}%</text>
        </g>
      ))}
      <line x1={x0} y1={y0} x2={x1} y2={y1} stroke="#cbd5e1" strokeDasharray="4 4" />
      <path d={area} fill="rgba(124,108,240,0.10)" />
      <path d={line} fill="none" stroke="#7c6cf0" strokeWidth="2" />
      {pts.map((p, i) => p.edge
        ? <rect key={i} x={p.x - 4} y={p.y - 4} width="8" height="8" transform={`rotate(45 ${p.x} ${p.y})`} fill="#7c6cf0" />
        : <circle key={i} cx={p.x} cy={p.y} r="3.5" fill="#fff" stroke="#7c6cf0" strokeWidth="2" />)}
      <text x={x0} y={H - 8} fontSize="10" fill="#94a3b8">{series.period_start}</text>
      <text x={x1} y={H - 8} textAnchor="end" fontSize="10" fill="#94a3b8">{series.period_end}</text>
    </svg>
  );
}
```

- [ ] **Step 2: Add the script tag**

In `period_overview_shell.html`, add before `period_overview_view.js`:

```html
<script type="text/babel" src="/static/progress_chart.js" data-presets="react"></script>
```

- [ ] **Step 3: Render the chart section**

In `period_overview_view.js`, add a «Прогресс целей за период» section rendering `<ProgressChart series={data.progress} />` with the explanatory caption from the screenshot («Пунктирная диагональ — ориентир ровного заполнения. Ромбы по краям — прогресс, зафиксированный до начала или после окончания периода.»).

- [ ] **Step 4: Manual browser verification**

Run the app with seeded snapshots (Task C6); confirm the chart draws the diagonal reference, the progress line/area, and edge diamonds for any out-of-range point. Switching scope refetches and redraws. Stop for review.

---

### Task C6: seed demo snapshots

**Files:**
- Modify: `internal/store/seed.go`
- Modify: `internal/store/store.go` (`SeedDemo` signature/wiring)

- [ ] **Step 1: Extend the seed to insert snapshot rows**

Thread the `progresssnap.Repository` into `seedDemo` the way `krsRepo` is threaded (add a parameter; pass `s.ProgressSnap` from `(*Store).SeedDemo`). After goals/KRs are created, insert a handful of ascending snapshots across the period so the demo chart is non-empty:

```go
// after teams+goals seeded, in seedDemo:
period, _ := periodsRepo.GetPeriod(ctx, scope, periodID) // or pass start/end in
days := []struct{ d time.Time; p int }{
	{period.StartDate.AddDate(0, 0, 5), 5},
	{period.StartDate.AddDate(0, 1, 0), 15},
	{period.StartDate.AddDate(0, 2, 0), 30},
}
for _, teamID := range teamIDs {
	for _, s := range days {
		_ = snapRepo.UpsertSnapshots(ctx, scope, periodID, s.d, []progresssnap.Snapshot{{TeamID: teamID, Progress: s.p}})
	}
}
```

If `seedDemo` doesn't already hold the period bounds, pass `start time.Time` in from `SeedDemo` (it's resolved in `main.go:70-90`). Keep the exact progress numbers illustrative.

- [ ] **Step 2: Verify seed runs**

Run: `go build ./... && go vet ./...`. Then run the server with `--seed` and confirm no error and the chart renders. Stop for review.

---

# Phase D — Specs + final gate

### Task D1: update specs

**Files:**
- Modify: `specs/040-api-contract.md`
- Modify: `specs/050-permissions-and-lifecycle.md`
- Modify: `specs/020-domain-model.md`

- [ ] **Step 1: `040-api-contract.md`**

Document `GET /api/v1/periods/{periodID}/overview?scope=my_teams|org`: default `my_teams`; response adds `balances` (`discovery_delivery`/`focuses`/`priorities`, each `[{key,count,percent}]`), `goals` (slim list), and `progress` (`{period_start, period_end, points:[{date,progress}]}`). Note `scope=org` requires tenant-admin (403 otherwise) and `my_teams` is available to any authenticated member. Note the legacy admin-only `/api/v1/admin/periods/{periodID}/overview` remains for the admin modal (org-wide, no balances/progress).

- [ ] **Step 2: `050-permissions-and-lifecycle.md`**

Add: overview `my_teams` scope = teams where `lead_udid` = the caller, plus recursive descendants — visible to any authenticated member; `org` scope (whole tenant) — tenant-admin only; the «Вся организация» control is hidden for non-admins.

- [ ] **Step 3: `020-domain-model.md`**

Add a `TeamPeriodProgressSnapshot` entity: fields `id, tenant_id, period_id, team_id, snapshot_date, progress, created_at`; invariants — unique `(tenant_id, period_id, team_id, snapshot_date)`; materialised daily for active periods only (closed/archived skipped); teams without goals and soft-deleted teams are not snapshotted; idempotent upsert per day.

- [ ] **Step 4: Final gate**

Run: `go test ./... && go vet ./...`. Confirm green. Stop for review (user commits).

---

## Self-Review

**Spec coverage**
- Req 1 (balances discovery/delivery, focuses, priorities, bar charts, drill-down into constituent goals) → Tasks B1 (aggregation + slim goals), B2 (`BalanceBars` + drill-down).
- Req 2 (progress-over-time chart: X dates start→end, Y %, points per date, dashed diagonal reference, pre-start/post-end points at edges) → Tasks C1–C6 (storage, job, series, `ProgressChart` with edge clamping, seed).
- Req 3 (scope toggle: my teams incl. nested by lead assignment; org admin-only; hide «Вся организация» for non-admins) → Tasks A1 (lead+descendants CTE), A3 (authz + endpoint), A4 (toggle hidden for non-admins).
- Spec-doc updates → Task D1 (040/050/020).

**Placeholder scan** — no TBD/TODO; every code step has real code; the only deliberate "verify the exact accessor/field name" notes (role accessor in A3; `domain.Goal.Title` in B1; `Period.StartDate/EndDate` type in C4) are guardrails against name drift, each with a concrete grep target, not missing content.

**Type consistency** — `teamFilter map[int64]bool` is used identically in A2/A3/C4; `computePeriodOverview(data, tol, filter)` arity is updated at every call site (A2 lists both internal callers); `progresssnap.Snapshot{TeamID,Progress}` and `SeriesRow{TeamID,Date,Progress}` match between C2 (repo), C3 (job), C4 (series); `ListLeadTeamScope` signature is identical across A1 (repo), A3 (`TeamScopeResolver`), and the `fakeGrants` test double.

**Scope** — three features share one page and one endpoint; sequenced A→B→C→D with independent, testable deliverables and a green `go test ./... && go vet ./...` gate at each checkpoint.
