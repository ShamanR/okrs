# KR Health Status Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a manually-set `health_status` field to each Key Result (`not_started` / `on_track` / `at_risk` / `done`), settable in the progress-update modal and visible in the KR list, with progress `<100→=100` auto-setting `done` once.

**Architecture:** New closed-set domain enum + one NOT NULL DB column. Progress-update endpoints are extended with an **optional** `health_status` field (one API call), but internally the handler runs two independent mutations — the existing progress update (which may auto-set `done` on the `<100→100` transition) followed by an explicit health update that overrides it (`nil` → health untouched). Frontend reuses the `Badge` component for the list and adds a 4-card selector to the progress modal.

**Tech Stack:** Go (chi, pgx), PostgreSQL raw-SQL migrations, React 18 via in-browser Babel (no build step, single file `tracker.js`), inline Russian UI strings.

## Global Constraints

- **No git commits by the agent.** Project rule (`CLAUDE.md` §8): the user commits. Each task ends with a **Stop-for-review checkpoint**, not a commit. Do not run `git commit`.
- `health_status` closed set, exact string values: `not_started`, `on_track`, `at_risk`, `done`. Default `not_started`.
- `health_status` is informational only — it MUST NOT enter progress/rollup math for KR/goal/period.
- Auto-`done` fires **only** on the progress transition `old < 100 && new == 100`, and only if current health ≠ `done` (the "once" guarantee). Never on repeated saves at 100%; never reverts when progress drops below 100%.
- Explicit `health_status` in a request always wins over auto-`done` (handler order: progress first, then health).
- Access control for the health mutation = same as progress update (`goalForKR` + `auth.CanAccessTeamFromCtx`). No new team-period-status dependency.
- Follow existing patterns: enum like `KRKind` (`internal/domain/models.go:30-49`), inline Russian strings, `Badge` reuse.
- Prefer `rg` over `grep`. Verify with `go build ./...`, `go vet ./...`, `go test ./...`.

---

### Task 1: Domain enum + validator + KeyResult field

**Files:**
- Modify: `internal/domain/models.go` (add type/consts near `:36`; add field on `KeyResult` struct `:132-149`)
- Test: `internal/domain/kr_health_test.go` (create)

**Interfaces:**
- Produces: `domain.KRHealthStatus` (string type); consts `domain.KRHealthNotStarted`, `domain.KRHealthOnTrack`, `domain.KRHealthAtRisk`, `domain.KRHealthDone`; `func domain.IsValidKRHealthStatus(s string) bool`; field `KeyResult.HealthStatus domain.KRHealthStatus`.

- [ ] **Step 1: Write the failing test**

Create `internal/domain/kr_health_test.go`:

```go
package domain

import "testing"

func TestIsValidKRHealthStatus(t *testing.T) {
	valid := []string{"not_started", "on_track", "at_risk", "done"}
	for _, s := range valid {
		if !IsValidKRHealthStatus(s) {
			t.Errorf("expected %q to be valid", s)
		}
	}
	invalid := []string{"", "NOT_STARTED", "onTrack", "risk", "completed"}
	for _, s := range invalid {
		if IsValidKRHealthStatus(s) {
			t.Errorf("expected %q to be invalid", s)
		}
	}
}

func TestKRHealthConstsMatchStrings(t *testing.T) {
	if string(KRHealthNotStarted) != "not_started" ||
		string(KRHealthOnTrack) != "on_track" ||
		string(KRHealthAtRisk) != "at_risk" ||
		string(KRHealthDone) != "done" {
		t.Fatal("health status const string values drifted")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/ -run TestIsValidKRHealthStatus -v`
Expected: FAIL (compile error — `IsValidKRHealthStatus`/consts undefined)

- [ ] **Step 3: Add the enum and validator**

In `internal/domain/models.go`, after the `KRUnits`/`IsValidKRUnit` block (`:49`):

```go
type KRHealthStatus string

const (
	KRHealthNotStarted KRHealthStatus = "not_started"
	KRHealthOnTrack    KRHealthStatus = "on_track"
	KRHealthAtRisk     KRHealthStatus = "at_risk"
	KRHealthDone       KRHealthStatus = "done"
)

// KRHealthStatuses is the closed set of manual health statuses a KR may have.
var KRHealthStatuses = []KRHealthStatus{KRHealthNotStarted, KRHealthOnTrack, KRHealthAtRisk, KRHealthDone}

// IsValidKRHealthStatus reports whether s is an allowed KR health status.
func IsValidKRHealthStatus(s string) bool {
	for _, allowed := range KRHealthStatuses {
		if string(allowed) == s {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Add the field on KeyResult**

In the `KeyResult` struct (`internal/domain/models.go:132-149`), add after `Progress int` (`:140`):

```go
	HealthStatus      KRHealthStatus
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/domain/ -run 'TestIsValidKRHealthStatus|TestKRHealthConstsMatchStrings' -v` and `go build ./...`
Expected: PASS; build OK.

- [ ] **Step 6: Stop-for-review checkpoint** (no commit — user commits)

---

### Task 2: Migration 042 — column + backfill

**Files:**
- Create: `migrations/042_kr_health_status.up.sql`
- Create: `migrations/042_kr_health_status.down.sql`
- Test: `internal/store/migration_kr_health_test.go` (create; model on `internal/store/migration_numerical_test.go`)

**Interfaces:**
- Produces: column `key_results.health_status TEXT NOT NULL DEFAULT 'not_started'`, backfilled to `'done'` for KRs whose current progress is 100%.

- [ ] **Step 1: Write the up migration**

Create `migrations/042_kr_health_status.up.sql`:

```sql
ALTER TABLE key_results
    ADD COLUMN IF NOT EXISTS health_status TEXT NOT NULL DEFAULT 'not_started';

-- Backfill: KRs already at 100% progress become 'done' (consistent with the 100%→done rule).
-- Progress is derived, so the predicate mirrors okr.*Progress at the 100% boundary per kind.

-- BOOLEAN: done when its boolean meta is_done = true.
UPDATE key_results kr
SET health_status = 'done'
WHERE kr.kind = 'BOOLEAN'
  AND EXISTS (SELECT 1 FROM kr_boolean_meta b WHERE b.key_result_id = kr.id AND b.is_done);

-- PROJECT: done when the sum of completed stage weights >= 100 (ProjectProgress clamps to 100).
UPDATE key_results kr
SET health_status = 'done'
WHERE kr.kind = 'PROJECT'
  AND COALESCE((SELECT SUM(s.weight) FROM kr_project_stages s
                WHERE s.key_result_id = kr.id AND s.is_done), 0) >= 100;

-- NUMERICAL: target is always the 100% point (with or without checkpoints); done when current
-- reached target in the goal direction. Increasing: target >= start & current >= target.
-- Decreasing: target < start & current <= target.
UPDATE key_results kr
SET health_status = 'done'
WHERE kr.kind = 'NUMERICAL'
  AND kr.start_value IS NOT NULL AND kr.target_value IS NOT NULL AND kr.current_value IS NOT NULL
  AND (
        (kr.target_value >= kr.start_value AND kr.current_value >= kr.target_value)
     OR (kr.target_value <  kr.start_value AND kr.current_value <= kr.target_value)
  );
```

- [ ] **Step 2: Write the down migration**

Create `migrations/042_kr_health_status.down.sql`:

```sql
ALTER TABLE key_results DROP COLUMN IF EXISTS health_status;
```

- [ ] **Step 3: Write the migration test**

Open `internal/store/migration_numerical_test.go` to copy its harness (how it opens a test DB, applies migrations, and asserts). Create `internal/store/migration_kr_health_test.go` following that exact pattern. It must:

1. Apply all migrations up to and including 042 against a fresh test DB.
2. Insert one goal, then four KRs:
   - a BOOLEAN KR with `kr_boolean_meta.is_done = true`,
   - a PROJECT KR with stages whose done weights sum to ≥100,
   - a NUMERICAL increasing KR with `current_value >= target_value`,
   - a NUMERICAL KR still below target.
3. Assert: the first three have `health_status = 'done'`; the fourth has `health_status = 'not_started'`.

Use the same DB-acquisition helper the sibling test uses (e.g. `testutil`), the same skip-if-no-DB guard, and the same migration-runner call. Do not invent a new harness — mirror the existing file's imports and setup line-for-line, changing only the inserted rows and assertions.

- [ ] **Step 4: Run the migration test**

Run: `go test ./internal/store/ -run TestMigrationKRHealth -v`
Expected: PASS (or SKIP if the suite skips without a test DB — then verify manually by running the full store suite in CI).

- [ ] **Step 5: Stop-for-review checkpoint**

---

### Task 3: Repository — read column + UpdateHealthStatus

**Files:**
- Modify: `internal/store/krs/krs.go` (`ListKeyResultsByGoal` query `:109-112` + scan `:123-124`; `GetKeyResult` query `:398-401` + scan `:402-403`; add new method near `UpdateNumericalCurrent` `:378-384`)
- Test: `internal/store/krs/krs_test.go`

**Interfaces:**
- Consumes: `domain.KeyResult.HealthStatus`, `domain.KRHealthStatus` (Task 1).
- Produces: `func (r *KRRepository) UpdateHealthStatus(ctx context.Context, scope domain.TenantScope, krID int64, status domain.KRHealthStatus) error`. Both `ListKeyResultsByGoal` and `GetKeyResult` now populate `kr.HealthStatus`.

- [ ] **Step 1: Write the failing test**

In `internal/store/krs/krs_test.go`, add a test that (following the file's existing setup helpers for tenant scope + a seeded KR) reads a KR, asserts default `health_status == not_started`, calls `UpdateHealthStatus(..., domain.KRHealthAtRisk)`, re-reads via `GetKeyResult`, and asserts `HealthStatus == domain.KRHealthAtRisk`. Also assert `ListKeyResultsByGoal` returns the same value. Mirror an existing test in this file for boilerplate (DB acquisition, scope, goal/KR creation).

```go
func TestUpdateHealthStatus(t *testing.T) {
	// ... acquire repo + scope + a created goalID/krID via the same helpers other tests use ...
	kr, err := repo.GetKeyResult(ctx, scope, krID)
	if err != nil { t.Fatal(err) }
	if kr.HealthStatus != domain.KRHealthNotStarted {
		t.Fatalf("default = %q, want not_started", kr.HealthStatus)
	}
	if err := repo.UpdateHealthStatus(ctx, scope, krID, domain.KRHealthAtRisk); err != nil {
		t.Fatal(err)
	}
	kr, _ = repo.GetKeyResult(ctx, scope, krID)
	if kr.HealthStatus != domain.KRHealthAtRisk {
		t.Fatalf("after update = %q, want at_risk", kr.HealthStatus)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/krs/ -run TestUpdateHealthStatus -v`
Expected: FAIL (compile error — `UpdateHealthStatus` undefined; `HealthStatus` unset)

- [ ] **Step 3: Add column to both SELECTs and scans**

`ListKeyResultsByGoal` query (`:110-112`) — append `health_status` to the column list:

```go
		SELECT id, goal_id, title, description, weight, kind, sort_order, created_at, updated_at,
		       start_value, target_value, current_value, unit, checkpoints, zeroing_criteria, health_status
		FROM key_results WHERE goal_id=$1 AND tenant_id=$2 ORDER BY sort_order, id`
```

Its scan (`:123-124`) — add a `&kr.HealthStatus` target at the end:

```go
		if err := rows.Scan(&kr.ID, &kr.GoalID, &kr.Title, &kr.Description, &kr.Weight, &kr.Kind, &kr.SortOrder, &kr.CreatedAt, &kr.UpdatedAt,
			&startValue, &targetValue, &currentValue, &unit, &checkpointsRaw, &zeroing, &kr.HealthStatus); err != nil {
```

`GetKeyResult` query (`:399-401`) — same column addition:

```go
		SELECT id, goal_id, title, description, weight, kind, sort_order, created_at, updated_at,
		       start_value, target_value, current_value, unit, checkpoints, zeroing_criteria, health_status
		FROM key_results WHERE id=$1 AND tenant_id=$2`
```

Its scan (`:402-403`) — add `&kr.HealthStatus`:

```go
	if err := row.Scan(&kr.ID, &kr.GoalID, &kr.Title, &kr.Description, &kr.Weight, &kr.Kind, &kr.SortOrder, &kr.CreatedAt, &kr.UpdatedAt,
		&startValue, &targetValue, &currentValue, &unit, &checkpointsRaw, &zeroing, &kr.HealthStatus); err != nil {
```

> Note: `pgx` scans a SQL `TEXT` into a `domain.KRHealthStatus` (a `string` subtype) directly — no `*string` indirection needed since the column is `NOT NULL`.

- [ ] **Step 4: Add the UpdateHealthStatus method**

After `UpdateNumericalCurrent` (`internal/store/krs/krs.go:378-384`):

```go
func (r *KRRepository) UpdateHealthStatus(ctx context.Context, scope domain.TenantScope, krID int64, status domain.KRHealthStatus) error {
	_, err := r.db.Exec(ctx, `
		UPDATE key_results
		SET health_status=$1, updated_at=NOW()
		WHERE id=$2 AND tenant_id=$3`, string(status), krID, scope.TenantID)
	return err
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/store/krs/ -run TestUpdateHealthStatus -v` and `go build ./...`
Expected: PASS; build OK.

- [ ] **Step 6: Stop-for-review checkpoint**

---

### Task 4: Service — UpdateKRHealthStatus + auto-done rule

**Files:**
- Modify: `internal/service/service.go` (three progress methods `:757-835`; add helper + new method)
- Test: `internal/service/progress_test.go` (add cases) or `internal/service/kr_health_test.go` (create)

**Interfaces:**
- Consumes: `krs.UpdateHealthStatus` (Task 3); `domain.KRHealth*` (Task 1); existing `okr.NumericalProgress/BooleanProgress/ProjectProgress`.
- Produces: `func (s *Service) UpdateKRHealthStatus(ctx context.Context, scope domain.TenantScope, krID int64, status domain.KRHealthStatus) error`. Progress methods now auto-set `done` on the `<100→100` transition.

- [ ] **Step 1: Write the failing tests**

Add to `internal/service/progress_test.go` (reuse its existing service+DB setup helpers). Four behaviors:

```go
// 1. Auto-done fires on <100 -> 100 (numerical increasing).
func TestNumericalReaching100AutoSetsDone(t *testing.T) {
	// seed numerical KR: start=0, target=100, current=50, health=not_started
	// UpdateKRProgressNumerical(..., 100, ...)
	// GetKeyResult -> HealthStatus == done
}

// 2. Re-saving at 100 does NOT re-touch health (manual override survives).
func TestResaveAt100DoesNotOverrideManualHealth(t *testing.T) {
	// seed numerical KR already at current=100 (progress 100), health=at_risk
	// UpdateKRProgressNumerical(..., 100, ...)   // before=100, not a transition
	// GetKeyResult -> HealthStatus == at_risk (unchanged)
}

// 3. Dropping below 100 does not change health.
func TestDroppingBelow100KeepsHealth(t *testing.T) {
	// seed numerical KR current=100, health=done
	// UpdateKRProgressNumerical(..., 80, ...)
	// GetKeyResult -> HealthStatus == done (unchanged)
}

// 4. Explicit UpdateKRHealthStatus overrides.
func TestUpdateKRHealthStatusSets(t *testing.T) {
	// seed KR health=not_started
	// UpdateKRHealthStatus(..., domain.KRHealthOnTrack)
	// GetKeyResult -> HealthStatus == on_track
}
```

Add one boolean and one project auto-done case mirroring #1:

```go
func TestBooleanDoneAutoSetsHealthDone(t *testing.T) {
	// seed BOOLEAN KR is_done=false, health=not_started
	// UpdateKRProgressBoolean(..., true, ...)
	// GetKeyResult -> HealthStatus == done
}

func TestProjectReaching100AutoSetsDone(t *testing.T) {
	// seed PROJECT KR with stages summing to 100 weight, all not done, health=not_started
	// UpdateKRProgressProject(..., mark all stages done, ...)
	// GetKeyResult -> HealthStatus == done
}
```

Fill in each body using the same seeding helpers the existing tests in this file use (do not invent a new harness).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/service/ -run 'TestNumericalReaching100AutoSetsDone|TestResaveAt100DoesNotOverrideManualHealth|TestDroppingBelow100KeepsHealth|TestUpdateKRHealthStatusSets|TestBooleanDoneAutoSetsHealthDone|TestProjectReaching100AutoSetsDone' -v`
Expected: FAIL (`UpdateKRHealthStatus` undefined; auto-done not applied)

- [ ] **Step 3: Add the service method + auto-done helper**

Near the progress methods in `internal/service/service.go`, add:

```go
// UpdateKRHealthStatus sets the manual health status of a KR. Access is checked by the caller
// (same as progress update). health status is informational and does not affect progress math.
func (s *Service) UpdateKRHealthStatus(ctx context.Context, scope domain.TenantScope, krID int64, status domain.KRHealthStatus) error {
	if !domain.IsValidKRHealthStatus(string(status)) {
		return fmt.Errorf("invalid health status: %s", status)
	}
	return s.krs.UpdateHealthStatus(ctx, scope, krID, status)
}

// autoCompleteHealth sets health=done exactly once, on the progress transition <100 -> =100,
// and only if the KR is not already done. Never reverts on later drops. kr is the pre-update state.
func (s *Service) autoCompleteHealth(ctx context.Context, scope domain.TenantScope, krID int64, kr domain.KeyResult, before, after int) {
	if before < 100 && after == 100 && kr.HealthStatus != domain.KRHealthDone {
		// best-effort: an auto-complete failure must not fail the progress mutation
		_ = s.krs.UpdateHealthStatus(ctx, scope, krID, domain.KRHealthDone)
	}
}
```

- [ ] **Step 4: Wire auto-done into the three progress methods**

`UpdateKRProgressNumerical` (`:768-772`) — inside the `if n := kr.Numerical` block, after `s.recordKRProgress(...)`:

```go
	if n := kr.Numerical; n != nil {
		beforeProg := okr.NumericalProgress(n.StartValue, n.TargetValue, n.CurrentValue, n.Checkpoints)
		afterProg := okr.NumericalProgress(n.StartValue, n.TargetValue, current, n.Checkpoints)
		s.recordKRProgress(ctx, scope, krID, kr, beforeProg, afterProg, actorUserID)
		s.autoCompleteHealth(ctx, scope, krID, kr, beforeProg, afterProg)
	}
```

`UpdateKRProgressBoolean` (`:791`) — after `recordKRProgress`:

```go
	beforeProg := okr.BooleanProgress(beforeDone)
	afterProg := okr.BooleanProgress(done)
	s.recordKRProgress(ctx, scope, krID, kr, beforeProg, afterProg, actorUserID)
	s.autoCompleteHealth(ctx, scope, krID, kr, beforeProg, afterProg)
```

(Replace the single inlined `recordKRProgress` call at `:791` with the four lines above.)

`UpdateKRProgressProject` (`:833`) — after `recordKRProgress`:

```go
	beforeProg := okr.ProjectProgress(stages)
	afterProg := okr.ProjectProgress(afterStages)
	s.recordKRProgress(ctx, scope, krID, kr, beforeProg, afterProg, actorUserID)
	s.autoCompleteHealth(ctx, scope, krID, kr, beforeProg, afterProg)
```

(`beforeProg` is already computed at `:822`; move/reuse it and add `afterProg` + the two calls, replacing the inlined `recordKRProgress` at `:833`. Ensure no duplicate `beforeProg` declaration remains.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/service/ -run 'TestNumericalReaching100AutoSetsDone|TestResaveAt100DoesNotOverrideManualHealth|TestDroppingBelow100KeepsHealth|TestUpdateKRHealthStatusSets|TestBooleanDoneAutoSetsHealthDone|TestProjectReaching100AutoSetsDone' -v` and `go build ./...` and `go vet ./internal/service/`
Expected: PASS; build/vet OK.

- [ ] **Step 6: Stop-for-review checkpoint**

---

### Task 5: HTTP handlers — optional health_status in progress requests

**Files:**
- Modify: `internal/http/handlers/api/v1/krs/handler.go` (`HandleUpdateNumericalProgress` `:144-171`, `HandleUpdateBooleanProgress` `:173-200`, `HandleUpdateProjectProgress` `:202-244`)
- Test: `internal/http/handlers/api/v1/krs/handler_test.go` (create if absent, or add to the existing handler test file for this package)

**Interfaces:**
- Consumes: `service.UpdateKRHealthStatus` (Task 4), `domain.IsValidKRHealthStatus` (Task 1).
- Produces: each progress endpoint now accepts optional body field `"health_status"`; invalid value → `400 VALIDATION_ERROR`; valid value applied after the progress mutation.

- [ ] **Step 1: Write the failing test**

Add handler-level tests (mirror the package's existing handler test setup for building a request, router, and stub/real service). Cover the numerical endpoint at minimum:

```go
// Posting a valid health_status alongside current_value applies it after progress.
func TestNumericalProgressWithHealthStatus(t *testing.T) {
	// POST /api/v1/krs/{id}/progress/numerical  {"current_value": 42, "health_status": "at_risk"}
	// expect 200; GetKeyResult -> HealthStatus == at_risk
}

// Invalid health_status -> 400.
func TestNumericalProgressInvalidHealthStatus(t *testing.T) {
	// POST ... {"current_value": 42, "health_status": "bogus"}
	// expect 400 VALIDATION_ERROR; health unchanged
}

// Omitted health_status -> health untouched (server auto-done still applies on 100%).
func TestNumericalProgressOmittedHealthStatus(t *testing.T) {
	// seed health=on_track, current below target
	// POST ... {"current_value": <below target>}   // no health_status field
	// expect 200; HealthStatus still on_track
}
```

If this package has no existing handler test harness, mirror the closest handler test in `internal/http/handlers/api/v1/` for router + service wiring.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/http/handlers/api/v1/krs/ -run 'HealthStatus' -v`
Expected: FAIL (field not decoded / not applied)

- [ ] **Step 3: Extend the numerical handler**

In `HandleUpdateNumericalProgress` (`:159-170`), change the request struct and add the post-progress health step:

```go
	var req struct {
		CurrentValue float64 `json:"current_value"`
		HealthStatus *string `json:"health_status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if req.HealthStatus != nil && !domain.IsValidKRHealthStatus(*req.HealthStatus) {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid health_status", map[string]string{"health_status": "invalid"})
		return
	}
	if err := h.service.UpdateKRProgressNumerical(r.Context(), scope, krID, req.CurrentValue, auth.UserIDFromContext(r.Context())); err != nil {
		v1.WriteError(w, http.StatusConflict, "CONFLICT", err.Error(), nil)
		return
	}
	if req.HealthStatus != nil {
		if err := h.service.UpdateKRHealthStatus(r.Context(), scope, krID, domain.KRHealthStatus(*req.HealthStatus)); err != nil {
			v1.WriteError(w, http.StatusConflict, "CONFLICT", err.Error(), nil)
			return
		}
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
```

> Import `domain` in this file if not already imported (`internal/domain`).

- [ ] **Step 4: Extend the boolean handler**

In `HandleUpdateBooleanProgress` (`:188-199`), same shape:

```go
	var req struct {
		Done         bool    `json:"done"`
		HealthStatus *string `json:"health_status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if req.HealthStatus != nil && !domain.IsValidKRHealthStatus(*req.HealthStatus) {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid health_status", map[string]string{"health_status": "invalid"})
		return
	}
	if err := h.service.UpdateKRProgressBoolean(r.Context(), scope, krID, req.Done, auth.UserIDFromContext(r.Context())); err != nil {
		v1.WriteError(w, http.StatusConflict, "CONFLICT", err.Error(), nil)
		return
	}
	if req.HealthStatus != nil {
		if err := h.service.UpdateKRHealthStatus(r.Context(), scope, krID, domain.KRHealthStatus(*req.HealthStatus)); err != nil {
			v1.WriteError(w, http.StatusConflict, "CONFLICT", err.Error(), nil)
			return
		}
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
```

- [ ] **Step 5: Extend the project handler**

In `HandleUpdateProjectProgress` (`:217-243`), add `HealthStatus *string` to the request struct (keeping the existing `Stages` field), add the same validation right after decode, and after the successful `UpdateKRProgressProject` call add:

```go
	if req.HealthStatus != nil {
		if err := h.service.UpdateKRHealthStatus(r.Context(), scope, krID, domain.KRHealthStatus(*req.HealthStatus)); err != nil {
			v1.WriteError(w, http.StatusConflict, "CONFLICT", err.Error(), nil)
			return
		}
	}
```

The struct becomes:

```go
	var req struct {
		Stages []struct {
			ID   int64 `json:"id"`
			Done bool  `json:"done"`
		} `json:"stages"`
		HealthStatus *string `json:"health_status"`
	}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/http/handlers/api/v1/krs/ -v` and `go build ./...` and `go vet ./...`
Expected: PASS; build/vet OK.

- [ ] **Step 7: Stop-for-review checkpoint**

---

### Task 6: DTO + response mapping

**Files:**
- Modify: `internal/http/dto/kr.go` (`KeyResult` struct `:47-60`)
- Modify: `internal/http/handlers/api/v1/helpers_response.go` (`dto.KeyResult{...}` build `:231-244`)
- Test: covered via handler test asserting the JSON field, or add an assertion to an existing okrs-response test.

**Interfaces:**
- Consumes: `domain.KeyResult.HealthStatus` (Task 1).
- Produces: JSON field `key_results[].health_status` in the KR response.

- [ ] **Step 1: Add the DTO field**

In `internal/http/dto/kr.go`, add to `KeyResult` (after `Progress`, `:55`):

```go
	Progress        int       `json:"progress"`
	HealthStatus    string    `json:"health_status"`
```

- [ ] **Step 2: Map it in the response builder**

In `internal/http/handlers/api/v1/helpers_response.go`, in the `return dto.KeyResult{...}` (`:231-244`), add:

```go
		Progress:        kr.Progress,
		HealthStatus:    string(kr.HealthStatus),
```

- [ ] **Step 3: Assert the field is serialized**

Add/extend a test in the package that builds a KR response and asserts `health_status` is present with the expected value (mirror an existing response-shape test; if none, assert via the krs handler `GET okrs` path used elsewhere).

- [ ] **Step 4: Run tests + build**

Run: `go test ./internal/http/... -v` and `go build ./...`
Expected: PASS; build OK.

- [ ] **Step 5: Stop-for-review checkpoint**

---

### Task 7: Seed / demo data

**Files:**
- Modify: `seed_demo.sql` (KR inserts ~`:144`; add a few explicit health statuses for demo realism)
- Modify: `internal/store/seed.go` (KR construction `:60`, `:87` — set `HealthStatus`)
- Test: `internal/store/seed_test.go` already validates seed applies; extend only if it asserts KR fields.

**Interfaces:**
- Consumes: column from Task 2, field from Task 1.

- [ ] **Step 1: Set HealthStatus in the Go seed**

In `internal/store/seed.go`, for each `domain.KeyResult{...}` literal (`:60` PROJECT, `:87` NUMERICAL, and any BOOLEAN), add an explicit `HealthStatus:` so demo data is representative, e.g.:

```go
		Kind:         domain.KRKindNumerical,
		HealthStatus: domain.KRHealthOnTrack,
```

Give at least one KR `KRHealthDone` (ideally one whose progress is 100%, consistent with the rule), one `KRHealthAtRisk`, one `KRHealthOnTrack`, leave others `KRHealthNotStarted`.

- [ ] **Step 2: Set health_status in seed_demo.sql**

`seed_demo.sql` inserts don't list `health_status`, so the `DEFAULT 'not_started'` applies — valid but all not_started. For demo realism, after the KR inserts add explicit updates (or extend the column list). Simplest, low-risk: append targeted updates near the numerical/boolean/project backfill blocks (`~:227`, `~:247`):

```sql
UPDATE key_results SET health_status = 'on_track' WHERE id IN (/* a couple of in-progress KR ids */);
UPDATE key_results SET health_status = 'at_risk'  WHERE id IN (/* a KR at risk */);
UPDATE key_results SET health_status = 'done'     WHERE id IN (/* the 100%-complete KR ids */);
```

Pick ids that already exist in the seed file (cross-check the KR `INSERT ... VALUES` id column). Keep the "done" ids consistent with KRs the demo shows at 100%.

- [ ] **Step 3: Verify seed applies cleanly**

Run: `go test ./internal/store/ -run TestSeed -v` (or the project's seed check). Also run `go build ./...`.
Expected: PASS; build OK.

- [ ] **Step 4: Stop-for-review checkpoint**

---

### Task 8: Frontend — constants, mapKR, list badge

**Files:**
- Modify: `internal/web/static/tracker.js` (constants block `:225-232`; `mapKR` `:139-158`; `KRRow` `:978-1012`)

**Interfaces:**
- Consumes: `kr.health_status` from the API (Task 6); `Badge` component (`:368`).
- Produces: globals `KR_HEALTH_LABEL`, `KR_HEALTH_COLOR`, `KR_HEALTH_HINT`, `KR_HEALTH_OPTIONS`; `mappedKR.healthStatus`.

- [ ] **Step 1: Add the health constants**

In the DESIGN CONSTANTS block (after `KR_TYPE_OPTIONS`, `:232`). Note: colors from the screenshot; `done` is a filled/solid green pill (pass `bg` to `Badge`):

```jsx
const KR_HEALTH_COLOR = { not_started: '#6b7280', on_track: '#16a34a', at_risk: '#d97706', done: '#15803d' };
const KR_HEALTH_LABEL = { not_started: 'Not Started', on_track: 'On Track', at_risk: 'At Risk', done: 'Done' };
const KR_HEALTH_ICON  = { not_started: '○', on_track: '●', at_risk: '▲', done: '✓' };
const KR_HEALTH_OPTIONS = ['not_started', 'on_track', 'at_risk', 'done'];
const KR_HEALTH_HINT = {
  not_started: 'Команда не приступила к этому KR',
  on_track: 'Началась работа, идёт планово',
  at_risk: 'Фиксируем существенный риск для достижения результата',
  done: 'Работа над KR завершена',
};
```

- [ ] **Step 2: Map the field in mapKR**

In `mapKR` (`internal/web/static/tracker.js:151-157`), add `healthStatus` to the returned object:

```jsx
    weight: kr.weight, krType: kr.kind, progress: kr.progress,
    healthStatus: kr.health_status || 'not_started',
    start, target, current, done, stages, unit, checkpoints, zeroing,
```

- [ ] **Step 3: Render the health badge in KRRow**

In `KRRow`, add a health badge as the first element of `.kr-info` (before `.kr-name`, `:983-984`) so it reads like the screenshot (status label above the title). `done` uses a solid pill:

```jsx
          <div className="kr-info">
            <div className="kr-health-badge-row">
              <Badge
                label={`${KR_HEALTH_ICON[kr.healthStatus]} ${KR_HEALTH_LABEL[kr.healthStatus]}`}
                color={kr.healthStatus === 'done' ? '#ffffff' : KR_HEALTH_COLOR[kr.healthStatus]}
                bg={kr.healthStatus === 'done' ? KR_HEALTH_COLOR.done : undefined}
              />
            </div>
            <div className="kr-name">{kr.name}</div>
```

Add a minimal spacing rule in `internal/web/static/tracker.css` (near the other `.kr-*` rules):

```css
.kr-health-badge-row { margin-bottom: 4px; }
```

- [ ] **Step 4: Manual verify**

Reload the tracker in the browser (no build step). Confirm each KR row shows a health badge matching its status color, and `done` KRs show a solid green pill. (Use the project's run flow — `/run` skill or existing dev server.)

- [ ] **Step 5: Stop-for-review checkpoint**

---

### Task 9: Frontend — health selector in KRProgressModal

**Files:**
- Modify: `internal/web/static/tracker.js` (`KRProgressModal` `:568-735`)
- Modify: `internal/web/static/tracker.css` (health-card styles)

**Interfaces:**
- Consumes: `KR_HEALTH_*` (Task 8); the progress endpoints extended with `health_status` (Task 5).
- Produces: modal writes `health_status` (only when the user changed it) in the single progress POST.

- [ ] **Step 1: Add health state + touched flag**

In `KRProgressModal`, after the existing state hooks (`:570-571`):

```jsx
  const [health, setHealth] = useState(kr.healthStatus || 'not_started');
  const [healthTouched, setHealthTouched] = useState(false);
  const pickHealth = (s) => { setHealth(s); setHealthTouched(true); };
```

Add health to the dirty check (`:582`) so the footer Save enables on a health-only change:

```jsx
  const isDirty = dirtyProgress || healthTouched || note.trim() !== initialNote.trim() || (descEditing && descDraft.trim() !== '');
```

- [ ] **Step 2: Reflect auto-done at 100% in the selector**

Compute the displayed status: if the user hasn't touched it and live progress is 100%, show `done` (mirrors the server rule); otherwise show `health`:

```jsx
  const displayHealth = (!healthTouched && progress === 100) ? 'done' : health;
```

(`progress` is already computed at `:574` via `calcKRProgress(form)`.)

- [ ] **Step 3: Render the "Health статус" section**

Insert between the kind-specific block and the note block (before the `<div>` at `:712`):

```jsx
          <div className="kr-health-section">
            <div className="kr-health-section__label">Health статус</div>
            <div className="kr-health-cards">
              {KR_HEALTH_OPTIONS.map(s => (
                <button key={s} type="button" onClick={() => pickHealth(s)}
                  className={`kr-health-card${displayHealth === s ? ' kr-health-card--active' : ''}`}
                  style={displayHealth === s ? { borderColor: KR_HEALTH_COLOR[s], background: `${KR_HEALTH_COLOR[s]}0f` } : undefined}>
                  <span className="kr-health-card__title" style={{ color: KR_HEALTH_COLOR[s] }}>
                    {KR_HEALTH_ICON[s]} {KR_HEALTH_LABEL[s]}
                  </span>
                  <span className="kr-health-card__hint">{KR_HEALTH_HINT[s]}</span>
                </button>
              ))}
            </div>
          </div>
```

- [ ] **Step 4: Send health_status in save() only when touched**

In `save()` (`:585-592`), add `health_status` to each progress POST body when `healthTouched`:

```jsx
      const healthField = healthTouched ? { health_status: health } : {};
      if (form.krType === 'NUMERICAL') {
        await apiPost(`/api/v1/krs/${kr.id}/progress/numerical`, { current_value: parseFloat(form.current) || 0, ...healthField });
      } else if (form.krType === 'BOOLEAN') {
        await apiPost(`/api/v1/krs/${kr.id}/progress/boolean`, { done: !!form.done, ...healthField });
      } else if (form.krType === 'PROJECT') {
        await apiPost(`/api/v1/krs/${kr.id}/progress/project`, { stages: form.stages.map(s => ({ id: s.id, done: !!s.done })), ...healthField });
      }
```

> When untouched, no `health_status` is sent → server leaves it alone, and the server's `<100→100` rule sets `done` automatically. When the user explicitly picks a status, it is sent and overrides any auto-done (handler applies health after progress).

- [ ] **Step 5: Add the card styles**

In `internal/web/static/tracker.css`, near the modal styles:

```css
.kr-health-section { margin-top: 14px; }
.kr-health-section__label { font-size: 13px; font-weight: 700; margin-bottom: 8px; }
.kr-health-cards { display: grid; grid-template-columns: repeat(2, 1fr); gap: 8px; }
.kr-health-card { display: flex; flex-direction: column; gap: 4px; align-items: flex-start;
  padding: 10px 12px; border: 1px solid #e5e7eb; border-radius: 8px; background: #fff;
  cursor: pointer; text-align: left; }
.kr-health-card--active { box-shadow: 0 0 0 1px currentColor inset; }
.kr-health-card__title { font-size: 13px; font-weight: 700; }
.kr-health-card__hint { font-size: 11px; color: #6b7280; line-height: 1.3; }
```

- [ ] **Step 6: Manual verify**

Reload the tracker. Open "Обновить прогресс" for a KR:
- 4 cards render with hints; clicking selects one and enables Save.
- Set a numerical KR's value to its target → progress hits 100% → Done card shows selected (without a manual click).
- Save with an explicit "At Risk" while at 100% → reload shows At Risk (manual wins).
- Save without touching health on a mid-progress KR → status unchanged.

- [ ] **Step 7: Stop-for-review checkpoint**

---

### Task 10: Update project specs (same change set)

**Files:**
- Modify: `specs/020-domain-model.md` (KeyResult section)
- Modify: `specs/040-api-contract.md` ("Update KR progress" + KR response shape)
- Modify: `specs/050-permissions-and-lifecycle.md` ("Требование к новым фичам")

**Interfaces:** documentation only — must match the implemented behavior.

- [ ] **Step 1: 020-domain-model.md**

In the `KeyResult` **Поля** list add `health_status`; in **Инварианты** add the three bullets from the design doc §3.2 (closed set + default `not_started`; not part of progress rollup; auto-`done` on `<100→100` once).

- [ ] **Step 2: 040-api-contract.md**

At "Update KR progress" (`:664-666`) document the optional `health_status` field on all three progress endpoints and the auto-`done` rule + explicit-override ordering. In the KR response shape (near `measure`, `:654`) note the new `key_results[].health_status` field.

- [ ] **Step 3: 050-permissions-and-lifecycle.md**

Under "Требование к новым фичам" add a subsection answering the five questions (design doc §2): no team-period-status dependency; allowed in `validated`/`closed` like progress; checked on server (team access + value validation); no future-roles dependency.

- [ ] **Step 4: Consistency pass**

Re-read the three edited spec sections against the implemented endpoints and field names. Ensure `health_status` values and the endpoint list match Tasks 1–6 exactly.

- [ ] **Step 5: Stop-for-review checkpoint**

---

## Notes for the executor

- **Test DB:** the store/service tests need a Postgres test DB (see `internal/store/testutil`). If tests SKIP locally without one, they must still pass in the environment that has a DB — don't delete or weaken them to make them "pass".
- **No commits:** end every task at its checkpoint; the user reviews and commits.
- **Out of scope (do not build):** activity-log journaling of manual health changes; health in markdown export; health filters/aggregates; a health field in the full KR create/edit form.
