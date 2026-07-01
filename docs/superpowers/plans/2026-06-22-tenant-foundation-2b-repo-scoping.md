# Tenant Foundation — Plan 2b: Repository Scoping & Isolation

> **For agentic workers:** REQUIRED SUB-SKILL: this plan is repetitive multi-file work across
> ~7 repositories — strongly prefer superpowers:subagent-driven-development (one repo per
> subagent, review + isolation test between each). Steps use checkbox (`- [ ]`) syntax.

**Goal:** Протянуть явный `domain.TenantScope` во все scoped-методы репозиториев и их вызывающих (сервисы, хендлеры), так чтобы каждый read нёс `WHERE tenant_id = $scope`, а каждый write проставлял `tenant_id`; снять транзитный `DEFAULT 1`; доказать изоляцию тестами.

**Architecture:** Механический, но обширный sweep. Один и тот же паттерн трансформации
применяется к каждому репозиторию (worked example — `teams`, Task 1), затем повторяется
по-репозиторно (Tasks 2–7). Хендлер извлекает scope из контекста
(`auth.TenantScopeFromContext`, из 2a) ровно один раз и передаёт его явным аргументом вниз.
Финал — снятие `DEFAULT 1` (Task 9) после того как все writes проставляют tenant_id явно.

**Tech Stack:** Go, pgx/v5, chi, testcontainers-go.

## Global Constraints

- **Явный параметр `domain.TenantScope`** во всех scoped-методах (подтверждённое решение).
  Сигнатура: первый бизнес-параметр после `ctx` — `scope domain.TenantScope`. Пример:
  `ListTeams(ctx context.Context, scope domain.TenantScope) (...)`.
- **Сервисы/репозитории НЕ читают тенант из контекста** — только из параметра. Контекст
  трогает только хендлер (`auth.TenantScopeFromContext`) на границе.
- Каждый SQL-read добавляет `AND tenant_id = $N` (где `$N` = `scope.TenantID`); каждый INSERT
  добавляет колонку `tenant_id` со значением `scope.TenantID`; UPDATE/DELETE добавляют
  `AND tenant_id = $N` в WHERE (нельзя мутировать чужой тенант даже по известному id).
- **Тест изоляции на каждый репозиторий** (security-ядро): сидируем два тенанта, проверяем,
  что scope тенанта A не видит/не мутирует строки тенанта B (404/пусто/0 rows affected).
- `DEFAULT 1` на `tenant_id` снимается ТОЛЬКО в Task 9, после того как все writes проставляют
  tenant_id явно (иначе существующие незаскоупленные writes упадут).
- Коммиты — за пользователем; агент только `git add` + предлагает сообщение; без упоминаний AI.
- **Порядок:** один репозиторий + его сервис-методы + его хендлеры + тест изоляции за задачу,
  чтобы diff был ревьюабельным и сборка зелёной после каждой.

---

### Task 1: Worked pattern — `teams` repository (read + write + isolation)

Это эталон. Tasks 2–7 повторяют его буквально для своих репозиториев.

**Files:**
- Modify: `internal/store/teams/teams.go` (все методы)
- Modify: `internal/service/service.go` (методы, вызывающие `s.store.Teams.*`)
- Modify: каждый хендлер/web-handler, вызывающий затронутые сервис-методы
- Test: `internal/store/teams/teams_isolation_test.go` (new)

**Interfaces:**
- Produces (примеры — привести ВСЕ методы репозитория к этой форме):
  - `(*TeamRepository) ListTeams(ctx, scope domain.TenantScope) ([]domain.Team, error)`
  - `(*TeamRepository) CreateTeam(ctx, scope domain.TenantScope, in TeamInput) (...)`
  - `(*TeamRepository) UpdateTeam(ctx, scope domain.TenantScope, ...)` / `DeleteTeam(...)`
- Consumes: `domain.TenantScope` (2a), `auth.TenantScopeFromContext` (2a).

- [ ] **Step 1: Write the failing isolation test**

```go
// internal/store/teams/teams_isolation_test.go
package teams_test

import (
	"context"
	"testing"

	"okrs/internal/domain"
	"okrs/internal/store/teams"
	"okrs/internal/store/testutil"
)

func TestTeamsScopedByTenant(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()

	// Tenant 1 (default) exists from migrations; create tenant 2.
	if _, err := pool.Exec(ctx, `INSERT INTO tenants (id, slug, name) OVERRIDING SYSTEM VALUE VALUES (2,'t2','T2')`); err != nil {
		t.Fatalf("tenant 2: %v", err)
	}
	repo := teams.NewTeamRepository(pool)
	scope1 := domain.TenantScope{TenantID: 1}
	scope2 := domain.TenantScope{TenantID: 2}

	// Seed one team in each tenant via the scoped create.
	if _, err := repo.CreateTeam(ctx, scope1, teams.TeamInput{Name: "A"}); err != nil {
		t.Fatalf("create A: %v", err)
	}
	if _, err := repo.CreateTeam(ctx, scope2, teams.TeamInput{Name: "B"}); err != nil {
		t.Fatalf("create B: %v", err)
	}

	// Each scope sees only its own team.
	a, err := repo.ListTeams(ctx, scope1)
	if err != nil {
		t.Fatalf("list scope1: %v", err)
	}
	if len(a) != 1 || a[0].Name != "A" {
		t.Fatalf("scope1 saw %+v, want [A]", a)
	}
	b, _ := repo.ListTeams(ctx, scope2)
	if len(b) != 1 || b[0].Name != "B" {
		t.Fatalf("scope2 saw %+v, want [B]", b)
	}
}
```

> **Verify `TeamInput` and create signature first:** read `teams.go` for the exact
> create-method name and input struct (it may be `CreateTeam(ctx, TeamInput)` or take fields).
> Adapt the test to the real names; the assertion (cross-tenant invisibility) is the point.

- [ ] **Step 2: Run to verify it fails to compile**

Run: `go test ./internal/store/teams -run TestTeamsScopedByTenant`
Expected: FAIL — `CreateTeam`/`ListTeams` signature mismatch (no `scope` param).

- [ ] **Step 3: Transform every method in `teams.go`**

Apply uniformly to all 12 methods:
- Add `scope domain.TenantScope` as the param right after `ctx`.
- Reads: add `AND tenant_id = $N` to the WHERE (or `WHERE tenant_id = $N` if none), binding
  `scope.TenantID`. Renumber existing `$` placeholders accordingly.
- Writes (INSERT): add `tenant_id` column + value `scope.TenantID`.
- Writes (UPDATE/DELETE): add `AND tenant_id = $N` to WHERE.
- Import `okrs/internal/domain` (already imported in this package).

Example (ListTeams):

```go
func (r *TeamRepository) ListTeams(ctx context.Context, scope domain.TenantScope) ([]domain.Team, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, name, team_type, parent_id, lead, lead_udid, description, deleted_at, created_at, updated_at
		FROM teams
		WHERE deleted_at IS NULL AND tenant_id = $1
		ORDER BY name`, scope.TenantID)
	...
}
```

Example (CreateTeam — add tenant_id to INSERT):

```go
// INSERT INTO teams (name, ..., tenant_id) VALUES (..., $K) ...
// pass scope.TenantID as the new bind.
```

- [ ] **Step 4: Thread scope through the service layer**

For each `internal/service` method that calls `s.store.Teams.*`, add `scope domain.TenantScope`
right after `ctx` and pass it down. (The service does NOT read context.)

- [ ] **Step 5: Thread scope through handlers (the only context readers)**

In each handler calling those service methods:

```go
scope, ok := auth.TenantScopeFromContext(r.Context())
if !ok {
	http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
	return
}
teams, err := h.service.ListTeams(r.Context(), scope, ...)
```

> In `AUTH_MODE=disabled` the anon middleware (2a) injects tenant #1, so `ok` is true.

- [ ] **Step 6: Run the isolation test + teams suite + build**

```bash
go test ./internal/store/teams -run TestTeamsScopedByTenant -v
go build ./... && go test ./internal/store/teams ./internal/service ./internal/http/...
```

Expected: PASS. Fix existing teams/service/handler tests whose calls now need a `scope`
argument — pass `domain.TenantScope{TenantID: 1}` (the default tenant) in those tests.

- [ ] **Step 7: Stage**

```bash
git add internal/store/teams/ internal/service/ internal/http/
# message: "feat(store): scope teams repository by tenant (explicit TenantScope)"
```

---

### Tasks 2–7: Apply the Task 1 pattern to each remaining repository

Each task is identical in shape to Task 1 (transform every method → thread service →
thread handlers → isolation test → build+test → stage). One repository per task so each
diff is reviewable and the build stays green.

- [ ] **Task 2: `periods`** (7 methods) — `internal/store/periods/`. Isolation test:
  `periods_isolation_test.go` — a period name may repeat across tenants (migration 031),
  assert scope sees only its own periods. Thread through `ListPeriods`/`GetPeriod` service
  methods + period handlers.
- [ ] **Task 3: `goals`** — `internal/store/goals/goals.go` (and its KR-aggregate loads).
  Goals carry `tenant_id`; `GoalRepository` already depends on `KRRepository` — pass `scope`
  into the KR loads too. Isolation test: goal of tenant B invisible/unmodifiable under scope A
  (assert GetGoal cross-tenant → not found, UpdateGoal cross-tenant → 0 rows).
- [ ] **Task 4: `krs`** (24 methods — the largest) — `internal/store/krs/`. KRs, meta,
  progress, project stages all carry `tenant_id` (denormalized in Plan 1). Thread `scope`
  into every method + KR handlers. Isolation test across all KR kinds.
- [ ] **Task 5: `shares`** (6 methods) — `internal/store/shares/`. `goal_shares` carry
  `tenant_id`. NOTE: a shared goal lives in the owner tenant; a share row's `tenant_id` is the
  owner tenant's (shares do not cross tenants in Phase 0). Isolation test: shares of tenant B
  invisible under scope A.
- [ ] **Task 6: `statuses`** (4 methods) — `internal/store/statuses/`. `team_period_statuses`
  carry `tenant_id`. Thread through status update handler. Isolation test.
- [ ] **Task 7: `grants`** (5 methods) — `internal/store/grants/`. Hierarchy grants carry
  `tenant_id`; both `ListUserGrants` and `ListDescendantTeamIDs` must filter by tenant.
  Isolation test: a user's grants in tenant A do not expand into tenant B teams.

---

### Task 8: Tenant-filter the PolicyEvaluator & grants scope resolution

**Files:**
- Modify: `internal/auth/policy.go` (`LoadScope`, `grantsReader` usage)
- Modify: `internal/store/grants/grants.go` (cache + queries already scoped in Task 7)
- Test: `internal/auth/policy_test.go`

**Interfaces:**
- `PolicyEvaluator.LoadScope` must resolve the allowed team set **within the active tenant**:
  the recursive descendant CTE and `ListUserGrants` take `scope`. The middleware passes the
  resolved tenant's scope (available from context after `TenantResolve`, read at the
  `ScopeMiddleware` boundary).

- [ ] **Step 1: Write the failing test** — grants in tenant A do not grant access to tenant B
  subtree; admin (nil scope) still unrestricted within the tenant.
- [ ] **Step 2–3:** thread `domain.TenantScope` into `grantsReader` methods and `LoadScope`;
  `ScopeMiddleware` reads tenant from context (boundary) and passes scope to `LoadScope`.
- [ ] **Step 4:** run `go test ./internal/auth ./internal/store/grants`.
- [ ] **Step 5:** stage.

> `ScopeMiddleware` is allowed to read tenant from context because it is part of the HTTP
> boundary (middleware), like `TenantResolve`. It converts context → explicit scope for
> `LoadScope`. Below that, no context reads.

---

### Task 9: Remove the transitional `DEFAULT 1`

**Files:**
- Create: `migrations/032_drop_tenant_default.up.sql` / `.down.sql`
- Test: `internal/store/migration_tenancy_test.go` (add)

**Interfaces:**
- Produces: `tenant_id` columns become `NOT NULL` without default on all scoped tables.
- Precondition: Tasks 1–7 done — every write passes `tenant_id` explicitly.

- [ ] **Step 1: Write the failing test** — after migration 032, an INSERT omitting
  `tenant_id` is rejected (no default to fall back on):

```go
func TestMigration032RemovesTenantDefault(t *testing.T) {
	db, cleanup := migrateTo(t, 32)
	defer cleanup()
	if _, err := db.Exec(`INSERT INTO teams (name) VALUES ('NoTenant')`); err == nil {
		t.Fatalf("insert without tenant_id should fail after default removed")
	}
}
```

- [ ] **Step 2: Run to verify it fails** (migration 032 absent).

- [ ] **Step 3: Write the up migration**

```sql
-- migrations/032_drop_tenant_default.up.sql
-- All writes now pass tenant_id explicitly (Plan 2b); drop the transitional default.
ALTER TABLE teams                  ALTER COLUMN tenant_id DROP DEFAULT;
ALTER TABLE periods                ALTER COLUMN tenant_id DROP DEFAULT;
ALTER TABLE goals                  ALTER COLUMN tenant_id DROP DEFAULT;
ALTER TABLE goal_shares            ALTER COLUMN tenant_id DROP DEFAULT;
ALTER TABLE team_period_statuses   ALTER COLUMN tenant_id DROP DEFAULT;
ALTER TABLE user_hierarchy_grants  ALTER COLUMN tenant_id DROP DEFAULT;
ALTER TABLE key_results            ALTER COLUMN tenant_id DROP DEFAULT;
ALTER TABLE goal_comments          ALTER COLUMN tenant_id DROP DEFAULT;
ALTER TABLE key_result_notes       ALTER COLUMN tenant_id DROP DEFAULT;
```

- [ ] **Step 4: Write the down migration** (restore `DEFAULT 1` on each).

- [ ] **Step 5: Run the test + FULL suite** (`go test ./...`). This is the regression gate:
  any write path that still omits `tenant_id` will now fail here and must be fixed.

- [ ] **Step 6: Stage.**

---

## Self-Review Notes

- **Spec coverage:** every scoped table's repo (Tasks 1–7), scope/policy (Task 8), default
  removal (Task 9). Each repo task carries its own isolation test — the security core.
- **Order matters:** Task 9 (drop default) is LAST. Dropping it before all writes pass
  tenant_id would break inserts. The full-suite run in Task 9 is the catch-all.
- **Seed/tests:** existing repo/service/handler tests will need `domain.TenantScope{TenantID: 1}`
  threaded into their calls; update them in the same task that changes the signature (TDD —
  keep the suite green per task).
- **Test threading note:** `seed_demo.sql` writes omit `tenant_id` and rely on `DEFAULT 1` —
  after Task 9 they would fail. Update `seed_demo.sql` in Task 9 to pass `tenant_id` (or
  set it explicitly) as part of the default-removal task.

## Execution recommendation

This is repetitive, wide multi-file work. Run it **subagent-driven** (one repository per
subagent: transform → thread → isolation test → build), reviewing + committing between repos,
so each tenant boundary is verified independently and the diff stays reviewable.
