# Simplify Key Result Types (NUMERICAL) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the four KR kinds (`BOOLEAN`/`PROJECT`/`LINEAR`/`PERCENT`) with three user-facing kinds (`BOOLEAN`/`PROJECT`/`NUMERICAL`), where `NUMERICAL` carries a unit, optional step checkpoints, and an optional zeroing-criteria note, storing all numerical data (including checkpoints as JSONB) directly on the `key_results` table.

**Architecture:** `LINEAR` and `PERCENT` collapse into one `NUMERICAL` kind. The dedicated `kr_percent_meta`, `kr_linear_meta` and `kr_percent_checkpoints` tables are dropped; their data migrates into new columns on `key_results` (`unit`, `start_value`, `target_value`, `current_value`, `checkpoints JSONB`, `zeroing_criteria`). Progress is computed in-memory from already-loaded rows (no new queries, no N+1). Checkpoint progress changes from interpolation to a step function. `start==target` now yields 100% when current reached target (previously 0%).

**Tech Stack:** Go 1.x, pgx/v5, chi router, in-browser Babel React (`internal/web/static/tracker.js`), golang-migrate SQL migrations, Postgres.

---

## Conventions / facts for the implementer

- Run all Go tests: `go test ./...`. Pure-logic packages (`internal/okr`, `internal/service`) need no DB. Store/handler integration tests (`internal/store/...`, `internal/http/handlers/api/v1/...`) require a Postgres reachable via the test harness (`internal/http/handlers/api/v1/testutil`, `internal/store` test setup) — run them if DB is available; otherwise run package-scoped pure tests and note DB-dependent ones.
- `go vet ./...` and `go build ./...` must pass at the end of every task that compiles code.
- Allowed units (exact, ordered) — used in backend validation and the frontend dropdown:
  `%`, `RPS`, `мс`, `сек`, `мин`, `час`, `дней`, `шт`, `₽`, `запросов`, `ошибок`, `пользователей`, `заказов`, `рублей`.
- Checkpoint JSON shape stored in `key_results.checkpoints`:
  ```json
  [{"value": 100, "progress_percent": 0}, {"value": 150, "progress_percent": 50}, {"value": 180, "progress_percent": 100}]
  ```
- Step algorithm (checkpoints present): sort points by `value`; if `current < firstValue` → first percent; if `current >= lastValue` → last percent; otherwise the percent of the largest `value <= current` (last reached step). Clamp result 0..100.
- Linear algorithm (no checkpoints): `((current-start)/(target-start))*100`, clamp 0..100 (works both directions). If `start==target`: `current==target → 100` else `0`.
- Do not mention AI/assistant/generated attribution in commits or comments (per CLAUDE.md).

---

## File Structure

**Migrations (create):**
- `migrations/023_kr_numerical.up.sql` — add columns to `key_results`, backfill from old tables, flip kind, drop old tables.
- `migrations/023_kr_numerical.down.sql` — recreate old tables, split data back, revert kind, drop new columns.

**Domain (modify):**
- `internal/domain/models.go` — add `KRKindNumerical`, `KRNumerical`, `KRNumericalCheckpoint`, unit list; remove `KRKindPercent`/`KRKindLinear`, `KRPercent`/`KRLinear`/`KRPercentCheckpoint`; replace KR struct fields.

**Progress logic (modify):**
- `internal/okr/okr.go` — replace `PercentProgress`/`LinearProgress` with `NumericalProgress`.
- `internal/okr/okr_test.go` — replace percent/linear tests with numerical tests.

**Service (modify):**
- `internal/service/progress.go` — switch `NUMERICAL` → `NumericalProgress`.
- `internal/service/service.go` — `KeyResultMetaInput` numerical fields; `applyKeyResultMeta`; `UpdateKRProgressPercent` → numerical; store-interface method names.
- `internal/service/progress_test.go`, `internal/service/service_test.go` — update to numerical.

**Store (modify):**
- `internal/store/krs/krs.go` — numerical upsert/read on `key_results`; remove percent/linear/checkpoint table methods; load numerical columns in `ListKeyResultsByGoal`.
- `internal/store/goals/goals.go` — select numerical columns in both KR queries; remove `loadPercentMeta`/`loadPercentCheckpoints`/`loadLinearMeta`; parse checkpoints JSON.
- `internal/store/krs/krs_test.go`, `internal/store/store_test.go` — update.

**HTTP DTO + mapping (modify):**
- `internal/http/dto/kr.go` — `NumericalMeasure`; replace `PercentMeasure`/`LinearMeasure`/`PercentCheckpoint`.
- `internal/http/handlers/api/v1/helpers_response.go` — `buildMeasure` numerical case.
- `internal/http/handlers/api/v1/helpers_response_test.go` — update.

**HTTP handlers (modify):**
- `internal/http/handlers/api/v1/krs/helpers.go` — `parseKeyResultMeta` numerical parsing/validation.
- `internal/http/handlers/api/v1/krs/handler.go` — `HandleUpdateNumericalProgress` (rename of percent handler).
- `internal/http/handlers/api/v1/krs/routes.go` — `/progress/numerical` route.
- `internal/http/handlers/api/v1/krs/*_test.go` — update.
- `internal/http/handlers/web/common/common.go` — `ValidKRKind` (3 kinds), unit validation + kind label helpers.

**Frontend (modify):**
- `internal/web/static/tracker.js` — 3-type dropdown + info-hint, numerical form (unit dropdown, checkpoints editor, zeroing textarea), step/linear client progress, thousands formatting + unit display; remove PERCENT/LINEAR branches.

**Seed (modify):**
- `seed_demo.sql` — convert PERCENT/LINEAR seed rows to NUMERICAL with units; move meta into `key_results` columns; drop old-table inserts.
- `internal/store/seed.go` — update if it references old kinds/tables (verify first).

**Specs (modify, same change set):**
- `specs/020-domain-model.md` — KR types list, derived computations, domain test cases.
- `specs/040-api-contract.md` — KR endpoints, measure shape, units, checkpoints.
- `specs/030-user-flows.md` — only if it references LINEAR/PERCENT/STEPS (verify first).

---

## Task 1: Database migration

**Files:**
- Create: `migrations/023_kr_numerical.up.sql`
- Create: `migrations/023_kr_numerical.down.sql`

- [ ] **Step 1: Write the up migration**

`migrations/023_kr_numerical.up.sql`:
```sql
-- Add NUMERICAL columns directly on key_results.
ALTER TABLE key_results
  ADD COLUMN IF NOT EXISTS unit TEXT,
  ADD COLUMN IF NOT EXISTS start_value DOUBLE PRECISION,
  ADD COLUMN IF NOT EXISTS target_value DOUBLE PRECISION,
  ADD COLUMN IF NOT EXISTS current_value DOUBLE PRECISION,
  ADD COLUMN IF NOT EXISTS checkpoints JSONB,
  ADD COLUMN IF NOT EXISTS zeroing_criteria TEXT NOT NULL DEFAULT '';

-- Backfill scalar values from the legacy LINEAR meta table.
UPDATE key_results kr
SET start_value = m.start_value,
    target_value = m.target_value,
    current_value = m.current_value,
    unit = '%'
FROM kr_linear_meta m
WHERE m.key_result_id = kr.id;

-- Backfill scalar values from the legacy PERCENT meta table.
UPDATE key_results kr
SET start_value = m.start_value,
    target_value = m.target_value,
    current_value = m.current_value,
    unit = '%'
FROM kr_percent_meta m
WHERE m.key_result_id = kr.id;

-- Backfill PERCENT checkpoints into the JSONB column (value/progress_percent).
UPDATE key_results kr
SET checkpoints = c.points
FROM (
  SELECT key_result_id,
         jsonb_agg(jsonb_build_object('value', metric_value, 'progress_percent', kr_percent)
                   ORDER BY metric_value) AS points
  FROM kr_percent_checkpoints
  GROUP BY key_result_id
) c
WHERE c.key_result_id = kr.id;

-- Flip legacy kinds to NUMERICAL (preserves all other KR data).
UPDATE key_results SET kind = 'NUMERICAL' WHERE kind IN ('LINEAR', 'PERCENT');

-- Drop legacy tables (checkpoints first due to FK direction independence).
DROP TABLE IF EXISTS kr_percent_checkpoints;
DROP TABLE IF EXISTS kr_percent_meta;
DROP TABLE IF EXISTS kr_linear_meta;
```

- [ ] **Step 2: Write the down migration**

`migrations/023_kr_numerical.down.sql`:
```sql
-- Recreate legacy tables.
CREATE TABLE IF NOT EXISTS kr_percent_meta (
  key_result_id INTEGER PRIMARY KEY REFERENCES key_results(id) ON DELETE CASCADE,
  start_value DOUBLE PRECISION NOT NULL,
  target_value DOUBLE PRECISION NOT NULL,
  current_value DOUBLE PRECISION NOT NULL
);

CREATE TABLE IF NOT EXISTS kr_linear_meta (
  key_result_id INTEGER PRIMARY KEY REFERENCES key_results(id) ON DELETE CASCADE,
  start_value DOUBLE PRECISION NOT NULL,
  target_value DOUBLE PRECISION NOT NULL,
  current_value DOUBLE PRECISION NOT NULL
);

CREATE TABLE IF NOT EXISTS kr_percent_checkpoints (
  id SERIAL PRIMARY KEY,
  key_result_id INTEGER NOT NULL REFERENCES key_results(id) ON DELETE CASCADE,
  metric_value DOUBLE PRECISION NOT NULL,
  kr_percent INTEGER NOT NULL CHECK (kr_percent BETWEEN 0 AND 100)
);

-- Revert NUMERICAL KRs that have checkpoints to PERCENT, the rest to LINEAR.
UPDATE key_results SET kind = 'PERCENT'
WHERE kind = 'NUMERICAL' AND checkpoints IS NOT NULL AND jsonb_array_length(checkpoints) > 0;
UPDATE key_results SET kind = 'LINEAR'
WHERE kind = 'NUMERICAL';

-- Restore meta rows.
INSERT INTO kr_percent_meta (key_result_id, start_value, target_value, current_value)
SELECT id, COALESCE(start_value,0), COALESCE(target_value,0), COALESCE(current_value,0)
FROM key_results WHERE kind = 'PERCENT';

INSERT INTO kr_linear_meta (key_result_id, start_value, target_value, current_value)
SELECT id, COALESCE(start_value,0), COALESCE(target_value,0), COALESCE(current_value,0)
FROM key_results WHERE kind = 'LINEAR';

-- Restore checkpoint rows.
INSERT INTO kr_percent_checkpoints (key_result_id, metric_value, kr_percent)
SELECT kr.id,
       (elem->>'value')::double precision,
       (elem->>'progress_percent')::int
FROM key_results kr
CROSS JOIN LATERAL jsonb_array_elements(kr.checkpoints) elem
WHERE kr.checkpoints IS NOT NULL;

ALTER TABLE key_results
  DROP COLUMN IF EXISTS unit,
  DROP COLUMN IF EXISTS start_value,
  DROP COLUMN IF EXISTS target_value,
  DROP COLUMN IF EXISTS current_value,
  DROP COLUMN IF EXISTS checkpoints,
  DROP COLUMN IF EXISTS zeroing_criteria;
```

- [ ] **Step 3: Apply migration against a dev/test DB and verify**

Run the project's migration command (same mechanism the app/tests use — check `cmd/server/main.go` / `internal/store` for the migrate call) up to 023, then verify:
- `\d key_results` shows new columns;
- `SELECT kind, count(*) FROM key_results GROUP BY kind;` shows no `LINEAR`/`PERCENT`;
- legacy tables are gone.
Expected: migration applies cleanly; down migration restores tables.

- [ ] **Step 4: Commit**

```bash
git add migrations/023_kr_numerical.up.sql migrations/023_kr_numerical.down.sql
git commit -m "migrate LINEAR/PERCENT key results to NUMERICAL with on-row checkpoints"
```

---

## Task 2: Domain model

**Files:**
- Modify: `internal/domain/models.go:30-37` (kinds), `:106-160` (KR + meta structs)

- [ ] **Step 1: Replace the KRKind constants**

In `internal/domain/models.go`, replace the `KRKind` block:
```go
type KRKind string

const (
	KRKindProject   KRKind = "PROJECT"
	KRKindNumerical KRKind = "NUMERICAL"
	KRKindBoolean   KRKind = "BOOLEAN"
)
```

- [ ] **Step 2: Add the allowed-units list**

Add below the KRKind block:
```go
// KRUnits is the closed set of measurement units a NUMERICAL KR may use.
var KRUnits = []string{"%", "RPS", "мс", "сек", "мин", "час", "дней", "шт", "₽", "запросов", "ошибок", "пользователей", "заказов", "рублей"}

// IsValidKRUnit reports whether u is an allowed NUMERICAL unit.
func IsValidKRUnit(u string) bool {
	for _, allowed := range KRUnits {
		if allowed == u {
			return true
		}
	}
	return false
}
```

- [ ] **Step 3: Replace KR meta structs**

Replace `KRPercent`, `KRLinear`, `KRPercentCheckpoint` (`:138-156`) with:
```go
type KRNumerical struct {
	StartValue      float64
	TargetValue     float64
	CurrentValue    float64
	Unit            string
	Checkpoints     []KRNumericalCheckpoint
	ZeroingCriteria string
}

type KRNumericalCheckpoint struct {
	Value           float64 `json:"value"`
	ProgressPercent int     `json:"progress_percent"`
}
```

- [ ] **Step 4: Update the KeyResult struct fields**

In `KeyResult` (`:106-123`) replace the `Percent`/`Linear` fields with one `Numerical`:
```go
	Project           *KRProject
	Numerical         *KRNumerical
	Boolean           *KRBoolean
```
(Keep `Project`, `Boolean`, `Note`, timestamps as-is.)

- [ ] **Step 5: Build to surface all references**

Run: `go build ./... 2>&1 | head -50`
Expected: compile errors only in files that referenced the removed kinds/types (okr, service, store, dto, handlers, common). These are fixed in later tasks. Do not commit yet — commit after Task 3 so the domain package compiles in isolation is not required; the repo will compile after Task 9.

---

## Task 3: Progress calculation (`internal/okr`)

**Files:**
- Modify: `internal/okr/okr.go:65-106`
- Test: `internal/okr/okr_test.go:46-99,122-126`

- [ ] **Step 1: Replace the percent/linear tests with numerical tests**

In `internal/okr/okr_test.go`, delete `TestPercentProgressLinear`, `TestPercentProgressCheckpoints`, `TestLinearProgress`, `TestPercentProgressEqualStartTarget`, and add:
```go
func TestNumericalProgressLinear(t *testing.T) {
	cases := []struct {
		name                   string
		start, target, current float64
		expect                 int
	}{
		{"growth midpoint", 100, 500, 300, 50},
		{"decline midpoint", 10, 5, 7.5, 50},
		{"below start", 100, 500, 80, 0},
		{"above target", 100, 500, 600, 100},
		{"at start", 0, 100, 0, 0},
		{"at target", 0, 100, 100, 100},
		{"equal reached target", 100, 100, 100, 100},
		{"equal not reached", 100, 100, 90, 0},
	}
	for _, tc := range cases {
		if got := NumericalProgress(tc.start, tc.target, tc.current, nil); got != tc.expect {
			t.Fatalf("%s: expected %d got %d", tc.name, tc.expect, got)
		}
	}
}

func TestNumericalProgressCheckpointsStep(t *testing.T) {
	cps := []domain.KRNumericalCheckpoint{
		{Value: 100, ProgressPercent: 0},
		{Value: 150, ProgressPercent: 50},
		{Value: 180, ProgressPercent: 100},
	}
	cases := []struct {
		name    string
		current float64
		expect  int
	}{
		{"between first two", 120, 0},
		{"on middle step", 150, 50},
		{"between middle and last", 170, 50},
		{"on last step", 180, 100},
		{"below first step", 90, 0},
		{"above last step", 200, 100},
	}
	for _, tc := range cases {
		if got := NumericalProgress(100, 180, tc.current, cps); got != tc.expect {
			t.Fatalf("%s: expected %d got %d", tc.name, tc.expect, got)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/okr/ 2>&1 | head -20`
Expected: compile failure — `NumericalProgress` undefined / `KRNumericalCheckpoint` referenced.

- [ ] **Step 3: Implement NumericalProgress and remove old functions**

In `internal/okr/okr.go`, delete `PercentProgress` and `LinearProgress` (and the now-unused `interpolate`, `point` if only used there — keep `clampPercent`, `linearPercent`). Add:
```go
// NumericalProgress computes 0..100 progress for a numerical KR.
// With no checkpoints it is linear from start to target (either direction).
// With checkpoints it is a step function: the percent of the last reached step.
func NumericalProgress(start, target, current float64, checkpoints []domain.KRNumericalCheckpoint) int {
	if len(checkpoints) == 0 {
		if start == target {
			if current >= target {
				return 100
			}
			return 0
		}
		return clampPercent(linearPercent(start, target, current))
	}

	pts := make([]domain.KRNumericalCheckpoint, len(checkpoints))
	copy(pts, checkpoints)
	sort.Slice(pts, func(i, j int) bool { return pts[i].Value < pts[j].Value })

	if current < pts[0].Value {
		return clampPercent(float64(pts[0].ProgressPercent))
	}
	result := pts[0].ProgressPercent
	for _, p := range pts {
		if current >= p.Value {
			result = p.ProgressPercent
		} else {
			break
		}
	}
	return clampPercent(float64(result))
}
```
Keep `import ("math"; "sort"; "okrs/internal/domain")`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/okr/ 2>&1 | head -20`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/models.go internal/okr/okr.go internal/okr/okr_test.go
git commit -m "add NUMERICAL kind and step/linear progress calculation"
```

---

## Task 4: Service layer

**Files:**
- Modify: `internal/service/progress.go:22-36`
- Modify: `internal/service/service.go` (store interface ~`:88-96`, `KeyResultMetaInput` `:770-787`, `applyKeyResultMeta` `:811-829`, `UpdateKRProgressPercent` `:676-688`)
- Test: `internal/service/progress_test.go`, `internal/service/service_test.go`

- [ ] **Step 1: Update progress.go switch**

Replace the `KRKindPercent` and `KRKindLinear` cases in `CalculateKRProgress` with:
```go
	case domain.KRKindNumerical:
		if kr.Numerical == nil {
			return 0
		}
		return okr.NumericalProgress(kr.Numerical.StartValue, kr.Numerical.TargetValue, kr.Numerical.CurrentValue, kr.Numerical.Checkpoints)
```

- [ ] **Step 2: Update KeyResultMetaInput**

Replace the percent/linear fields in `KeyResultMetaInput` (`:770-787`) with:
```go
type KeyResultMetaInput struct {
	NumericalStart      float64
	NumericalTarget     float64
	NumericalCurrent    float64
	NumericalUnit       string
	NumericalCheckpoints []domain.KRNumericalCheckpoint
	ZeroingCriteria     string
	BooleanDone         bool
	ProjectStages       []krs.ProjectStageInput
}
```
(Keep existing `BooleanDone`/`ProjectStages` field definitions; remove `PercentStart/Target/Current`, `LinearStart/Target/Current`.)

- [ ] **Step 3: Update applyKeyResultMeta**

Replace the `KRKindPercent`/`KRKindLinear` cases (`:813-826`) with:
```go
	case domain.KRKindNumerical:
		return s.krs.UpsertNumericalMeta(ctx, krs.NumericalMetaInput{
			KeyResultID:     krID,
			StartValue:      meta.NumericalStart,
			TargetValue:     meta.NumericalTarget,
			CurrentValue:    meta.NumericalCurrent,
			Unit:            meta.NumericalUnit,
			Checkpoints:     meta.NumericalCheckpoints,
			ZeroingCriteria: meta.ZeroingCriteria,
		})
```

- [ ] **Step 4: Update UpdateKRProgressPercent → UpdateKRProgressNumerical**

Replace `UpdateKRProgressPercent` (`:676-688`) with:
```go
func (s *Service) UpdateKRProgressNumerical(ctx context.Context, krID int64, current float64) error {
	kr, err := s.krs.GetKeyResult(ctx, krID)
	if err != nil {
		return err
	}
	if kr.Kind != domain.KRKindNumerical {
		return fmt.Errorf("unsupported kr kind for numerical update: %s", kr.Kind)
	}
	return s.krs.UpdateNumericalCurrent(ctx, krID, current)
}
```

- [ ] **Step 5: Update the store interface in service.go**

In the store interface (around `:88-96`), replace:
```go
	UpdatePercentCurrent(ctx context.Context, krID int64, current float64) error
	UpdateLinearCurrent(ctx context.Context, krID int64, current float64) error
```
with:
```go
	UpdateNumericalCurrent(ctx context.Context, krID int64, current float64) error
```
and replace:
```go
	UpsertPercentMeta(ctx context.Context, input krs.PercentMetaInput) error
	UpsertLinearMeta(ctx context.Context, input krs.LinearMetaInput) error
```
with:
```go
	UpsertNumericalMeta(ctx context.Context, input krs.NumericalMetaInput) error
```
(Leave `UpsertBooleanMeta` and other entries unchanged.)

- [ ] **Step 6: Update service tests**

In `internal/service/progress_test.go` and `internal/service/service_test.go`, replace any `KRKindPercent`/`KRKindLinear` construction, `kr.Percent`/`kr.Linear`, and old fake-store method names with the numerical equivalents (`KRKindNumerical`, `kr.Numerical = &domain.KRNumerical{...}`, `UpsertNumericalMeta`, `UpdateNumericalCurrent`). Mirror existing test structure. Add a service-level case asserting `CalculateKRProgress` returns the step value for a NUMERICAL KR with checkpoints.

- [ ] **Step 7: Run service tests**

Run: `go test ./internal/service/ 2>&1 | head -30`
Expected: PASS (these are pure-logic / fake-store tests).

- [ ] **Step 8: Commit**

```bash
git add internal/service/
git commit -m "route NUMERICAL meta and progress through service layer"
```

---

## Task 5: KR store (`internal/store/krs`)

**Files:**
- Modify: `internal/store/krs/krs.go`
- Test: `internal/store/krs/krs_test.go`

- [ ] **Step 1: Replace meta input structs**

Replace `PercentMetaInput`, `PercentCheckpointInput`, `LinearMetaInput` (`:49-70`) with:
```go
// NumericalMetaInput is used by UpsertNumericalMeta.
type NumericalMetaInput struct {
	KeyResultID     int64
	StartValue      float64
	TargetValue     float64
	CurrentValue    float64
	Unit            string
	Checkpoints     []domain.KRNumericalCheckpoint
	ZeroingCriteria string
}
```

- [ ] **Step 2: Replace meta upsert/read/update methods**

Remove `UpsertPercentMeta`, `UpdatePercentCurrent`, `UpsertLinearMeta`, `UpdateLinearCurrent`, `AddPercentCheckpoint`, `GetPercentMeta`, `GetLinearMeta`, `ListPercentCheckpoints`. Add:
```go
import "encoding/json" // ensure present at top of file

func (r *KRRepository) UpsertNumericalMeta(ctx context.Context, input NumericalMetaInput) error {
	var checkpointsJSON []byte
	if len(input.Checkpoints) > 0 {
		b, err := json.Marshal(input.Checkpoints)
		if err != nil {
			return err
		}
		checkpointsJSON = b
	}
	_, err := r.db.Exec(ctx, `
		UPDATE key_results
		SET start_value=$1, target_value=$2, current_value=$3, unit=$4,
		    checkpoints=$5, zeroing_criteria=$6, updated_at=NOW()
		WHERE id=$7`,
		input.StartValue, input.TargetValue, input.CurrentValue, input.Unit,
		checkpointsJSON, input.ZeroingCriteria, input.KeyResultID,
	)
	return err
}

func (r *KRRepository) UpdateNumericalCurrent(ctx context.Context, krID int64, current float64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE key_results
		SET current_value=$1, updated_at=NOW(), progress_updated_at=NOW()
		WHERE id=$2`, current, krID)
	return err
}
```

- [ ] **Step 3: Load numerical fields in ListKeyResultsByGoal and GetKeyResult**

In `ListKeyResultsByGoal`, change the base SELECT to include numerical columns and scan them, then build `kr.Numerical` when kind is NUMERICAL. Replace the `key_results` query (`:84-101`) so the SELECT is:
```sql
SELECT id, goal_id, title, description, weight, kind, sort_order, created_at, updated_at,
       start_value, target_value, current_value, unit, checkpoints, zeroing_criteria
FROM key_results WHERE goal_id=$1 ORDER BY sort_order, id
```
Scan into nullable holders and assemble (use a shared helper — see Step 4). Remove the `KRKindPercent`/`KRKindLinear` branches from the per-kind switch (keep `KRKindProject`/`KRKindBoolean`). Update `GetKeyResult` SELECT similarly so its returned KR carries numerical data when needed by callers.

- [ ] **Step 4: Add a scan helper for numerical columns**

Add a helper used by both `krs.go` and `goals.go` (place in `krs.go`, exported):
```go
// ParseCheckpoints decodes the key_results.checkpoints JSONB payload.
func ParseCheckpoints(raw []byte) ([]domain.KRNumericalCheckpoint, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var cps []domain.KRNumericalCheckpoint
	if err := json.Unmarshal(raw, &cps); err != nil {
		return nil, err
	}
	return cps, nil
}
```
When scanning, use `start, target, current sql.Null...`? Columns are nullable for non-numerical KRs. Use pgx-friendly holders: `var startValue, targetValue, currentValue *float64; var unit, zeroing *string; var checkpointsRaw []byte`. Build:
```go
if kr.Kind == domain.KRKindNumerical {
	num := &domain.KRNumerical{}
	if startValue != nil { num.StartValue = *startValue }
	if targetValue != nil { num.TargetValue = *targetValue }
	if currentValue != nil { num.CurrentValue = *currentValue }
	if unit != nil { num.Unit = *unit }
	if zeroing != nil { num.ZeroingCriteria = *zeroing }
	cps, err := ParseCheckpoints(checkpointsRaw)
	if err != nil { return nil, err }
	num.Checkpoints = cps
	kr.Numerical = num
}
```
Apply the same scan holders in `GetKeyResult`.

- [ ] **Step 5: Update krs_test.go**

Replace percent/linear/checkpoint store tests with numerical-meta tests: insert a NUMERICAL KR, `UpsertNumericalMeta` with checkpoints + unit + zeroing, reload via `ListKeyResultsByGoal`, assert columns round-trip and checkpoints decode. Test `UpdateNumericalCurrent` updates `current_value` and `progress_updated_at`.

- [ ] **Step 6: Run store tests (requires DB)**

Run: `go test ./internal/store/krs/ 2>&1 | head -30`
Expected: PASS when DB available. If no DB, run `go build ./internal/store/krs/` and note DB-dependent tests are deferred.

- [ ] **Step 7: Commit**

```bash
git add internal/store/krs/
git commit -m "store NUMERICAL meta and checkpoints on key_results row"
```

---

## Task 6: Goals batch loader (`internal/store/goals`)

**Files:**
- Modify: `internal/store/goals/goals.go` (KR SELECTs `:161-165` and `:484-488`; remove `loadPercentMeta`/`loadPercentCheckpoints`/`loadLinearMeta` `:236-311,566-635`; their call sites `:526-532`)
- Test: `internal/store/goals` tests (if present) / `internal/store/store_test.go`

- [ ] **Step 1: Add numerical columns to both KR queries**

In `ListGoalsByTeamsPeriod` (`:161`) and `loadKRsForGoals` (`:484`), extend the `key_results` SELECT to:
```sql
SELECT id, goal_id, title, description, weight, kind, sort_order, created_at, updated_at, progress_updated_at,
       start_value, target_value, current_value, unit, checkpoints, zeroing_criteria
FROM key_results WHERE goal_id = ANY($1) ORDER BY goal_id, sort_order, id
```
(For `loadKRsForGoals` the existing query omits `progress_updated_at`; keep it omitted there to match its current scan, only append the six numerical columns. Match each query's existing column set + the six new columns, and add matching scan targets.)

- [ ] **Step 2: Scan and assemble Numerical in both loops**

In each KR-row scan, add the numerical holders (`*float64`, `*string`, `[]byte`) and, after scanning, build `kr.Numerical` exactly as in Task 5 Step 4 using `krs.ParseCheckpoints`. Import `okrs/internal/store/krs` is already present.

- [ ] **Step 3: Remove percent/linear/checkpoint batch loaders and calls**

Delete the `percentRows`/`checkpointRows`/`linearRows` blocks in `ListGoalsByTeamsPeriod` (`:236-311`) and the `loadPercentMeta`, `loadPercentCheckpoints`, `loadLinearMeta` methods (`:566-635`) and their calls in `loadKRsForGoals` (`:526-532`). Keep `loadProjectStages`, `loadBooleanMeta`, `loadKRNotes`.

- [ ] **Step 4: Verify build and N+1 invariant**

Run: `go build ./internal/store/... 2>&1 | head -20`
Expected: PASS. Confirm by reading the code that NUMERICAL data now comes from the single `key_results` SELECT — no per-KR query, fewer total queries than before (two fewer batch queries: percent-meta, linear-meta, checkpoints → 0; numerical folded into the KR query).

- [ ] **Step 5: Run goals/store tests (requires DB)**

Run: `go test ./internal/store/... 2>&1 | head -40`
Expected: PASS when DB available.

- [ ] **Step 6: Commit**

```bash
git add internal/store/goals/
git commit -m "load NUMERICAL data with key results in batch loader, drop meta-table queries"
```

---

## Task 7: HTTP DTO + response mapping

**Files:**
- Modify: `internal/http/dto/kr.go`
- Modify: `internal/http/handlers/api/v1/helpers_response.go:209-242`
- Test: `internal/http/handlers/api/v1/helpers_response_test.go`

- [ ] **Step 1: Replace DTO measure types**

In `internal/http/dto/kr.go`, replace `PercentCheckpoint`, `PercentMeasure`, `LinearMeasure` (`:12-28`) and the `Measure` struct (`:45-52`) with:
```go
type NumericalCheckpoint struct {
	Value           float64 `json:"value"`
	ProgressPercent int     `json:"progress_percent"`
}

type NumericalMeasure struct {
	StartValue      float64               `json:"start_value"`
	TargetValue     float64               `json:"target_value"`
	CurrentValue    float64               `json:"current_value"`
	Unit            string                `json:"unit"`
	Checkpoints     []NumericalCheckpoint `json:"checkpoints,omitempty"`
	ZeroingCriteria string                `json:"zeroing_criteria,omitempty"`
}

type Measure struct {
	Kind      string            `json:"kind"`
	Numerical *NumericalMeasure `json:"numerical,omitempty"`
	Boolean   *BooleanMeasure   `json:"boolean,omitempty"`
	Project   *ProjectMeasure   `json:"project,omitempty"`
}
```
(Keep `BooleanMeasure`, `ProjectStage`, `ProjectMeasure`, `KRNote`, `KeyResult`.)

- [ ] **Step 2: Update buildMeasure**

In `helpers_response.go`, replace the `KRKindPercent` and `KRKindLinear` cases in `buildMeasure` (`:211-224`) with:
```go
	case domain.KRKindNumerical:
		if kr.Numerical == nil {
			return dto.Measure{Kind: string(kr.Kind)}
		}
		cps := make([]dto.NumericalCheckpoint, 0, len(kr.Numerical.Checkpoints))
		for _, cp := range kr.Numerical.Checkpoints {
			cps = append(cps, dto.NumericalCheckpoint{Value: cp.Value, ProgressPercent: cp.ProgressPercent})
		}
		return dto.Measure{
			Kind: string(kr.Kind),
			Numerical: &dto.NumericalMeasure{
				StartValue:      kr.Numerical.StartValue,
				TargetValue:     kr.Numerical.TargetValue,
				CurrentValue:    kr.Numerical.CurrentValue,
				Unit:            kr.Numerical.Unit,
				Checkpoints:     cps,
				ZeroingCriteria: kr.Numerical.ZeroingCriteria,
			},
		}
```

- [ ] **Step 3: Update helpers_response_test.go**

Replace percent/linear measure assertions with a NUMERICAL case: build a `domain.KeyResult{Kind: KRKindNumerical, Numerical: &domain.KRNumerical{...}}`, call `MapKeyResult`, assert `measure.Numerical` carries unit, values, checkpoints, zeroing.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/http/handlers/api/v1/ 2>&1 | head -20`
Expected: PASS for pure mapping tests.

- [ ] **Step 5: Commit**

```bash
git add internal/http/dto/kr.go internal/http/handlers/api/v1/helpers_response.go internal/http/handlers/api/v1/helpers_response_test.go
git commit -m "expose NUMERICAL measure in KR DTO"
```

---

## Task 8: HTTP handlers, routes, validation

**Files:**
- Modify: `internal/http/handlers/web/common/common.go:204-219`
- Modify: `internal/http/handlers/api/v1/krs/helpers.go:14-50`
- Modify: `internal/http/handlers/api/v1/krs/handler.go:124-147`
- Modify: `internal/http/handlers/api/v1/krs/routes.go`
- Test: `internal/http/handlers/api/v1/krs/*_test.go`

- [ ] **Step 1: Update ValidKRKind and add helpers in common.go**

Replace `ValidKRKind` body (`:204-208`):
```go
func ValidKRKind(k domain.KRKind) bool {
	switch k {
	case domain.KRKindProject, domain.KRKindNumerical, domain.KRKindBoolean:
		return true
	default:
		return false
	}
}
```
Add a kind label helper (Russian aliases) near the team-type label helper:
```go
// KRKindLabel returns the Russian UI alias for a KR kind.
func KRKindLabel(k domain.KRKind) string {
	switch k {
	case domain.KRKindBoolean:
		return "Бинарный"
	case domain.KRKindProject:
		return "Проектный"
	case domain.KRKindNumerical:
		return "Числовой"
	default:
		return string(k)
	}
}
```

- [ ] **Step 2: Update parseKeyResultMeta for NUMERICAL**

In `krs/helpers.go`, replace the `KRKindPercent` and `KRKindLinear` cases with one numerical case:
```go
	case domain.KRKindNumerical:
		unit := strings.TrimSpace(r.FormValue("numerical_unit"))
		if !domain.IsValidKRUnit(unit) {
			return service.KeyResultMetaInput{}, fmt.Errorf("Недопустимая единица измерения")
		}
		start := common.ParseFloatField(r.FormValue("numerical_start"))
		target := common.ParseFloatField(r.FormValue("numerical_target"))
		current := common.ParseFloatField(r.FormValue("numerical_current"))
		checkpoints, err := parseCheckpoints(r)
		if err != nil {
			return service.KeyResultMetaInput{}, err
		}
		return service.KeyResultMetaInput{
			NumericalStart:       start,
			NumericalTarget:      target,
			NumericalCurrent:     current,
			NumericalUnit:        unit,
			NumericalCheckpoints: checkpoints,
			ZeroingCriteria:      strings.TrimSpace(r.FormValue("numerical_zeroing")),
		}, nil
```
Add `parseCheckpoints` (parallel arrays `checkpoint_value[]`, `checkpoint_percent[]`):
```go
func parseCheckpoints(r *http.Request) ([]domain.KRNumericalCheckpoint, error) {
	values := r.Form["checkpoint_value[]"]
	percents := r.Form["checkpoint_percent[]"]
	if len(values) == 0 {
		return nil, nil
	}
	out := make([]domain.KRNumericalCheckpoint, 0, len(values))
	seen := make(map[float64]struct{}, len(values))
	for i, raw := range values {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		value := common.ParseFloatField(raw)
		percentStr := ""
		if i < len(percents) {
			percentStr = percents[i]
		}
		percent := common.ParseIntField(percentStr)
		if percent < 0 || percent > 100 {
			return nil, fmt.Errorf("Процент промежуточного значения должен быть 0..100")
		}
		if _, dup := seen[value]; dup {
			return nil, fmt.Errorf("Промежуточные значения не должны дублироваться")
		}
		seen[value] = struct{}{}
		out = append(out, domain.KRNumericalCheckpoint{Value: value, ProgressPercent: percent})
	}
	return out, nil
}
```
(Note: `start==target` is now allowed for NUMERICAL — drop the old equality rejection.)

- [ ] **Step 3: Rename the progress handler to numerical**

In `krs/handler.go`, rename `HandleUpdatePercentProgress` → `HandleUpdateNumericalProgress` and call `h.service.UpdateKRProgressNumerical(...)`. Body otherwise unchanged (still decodes `{ "current_value": ... }`).

- [ ] **Step 4: Update the route**

In `krs/routes.go`, change:
```go
	r.Post("/krs/{krID}/progress/numerical", h.HandleUpdateNumericalProgress)
```

- [ ] **Step 5: Update handler tests**

In `krs/*_test.go`, replace `kind=PERCENT`/`kind=LINEAR` create/update payloads with `kind=NUMERICAL` + `numerical_unit`, `numerical_start/target/current`, optional `checkpoint_value[]`/`checkpoint_percent[]`, `numerical_zeroing`; change `progress/percent` calls to `progress/numerical`. Add a validation test: invalid unit → 400; duplicate checkpoint value → 400; checkpoint percent 150 → 400.

- [ ] **Step 6: Run handler tests (DB for integration)**

Run: `go test ./internal/http/handlers/... 2>&1 | head -40`
Expected: PASS when DB available; otherwise build + note deferred.

- [ ] **Step 7: Commit**

```bash
git add internal/http/handlers/
git commit -m "accept NUMERICAL kind, unit, checkpoints, and zeroing criteria over the API"
```

---

## Task 9: Seed data + full build

**Files:**
- Modify: `seed_demo.sql`
- Modify: `internal/store/seed.go` (only if it references old kinds/tables — verify with `rg -n "PERCENT|LINEAR|percent_meta|linear_meta|checkpoint" internal/store/seed.go`)

- [ ] **Step 1: Convert seed KR rows and meta**

In `seed_demo.sql`: change `'PERCENT'`/`'LINEAR'` kind literals to `'NUMERICAL'`; remove the `TRUNCATE`/`INSERT` for `kr_percent_meta`, `kr_linear_meta`, `kr_percent_checkpoints` and the `setval('kr_percent_checkpoints_id_seq', 1)`; instead set numerical columns inline on the key_results rows (or via `UPDATE key_results SET start_value=..., target_value=..., current_value=..., unit='%' WHERE id=...`). Give each a sensible `unit` (`%` for coverage/percent KRs, `мс` for latency, `мин` for reaction-time, etc.).

- [ ] **Step 2: Verify seed loads**

Run the seed against a fresh migrated DB (same path the project uses). Expected: no errors; KRs appear as NUMERICAL with units.

- [ ] **Step 3: Full build, vet, test**

Run: `go build ./... && go vet ./... && go test ./... 2>&1 | tail -40`
Expected: build/vet clean; pure tests PASS; DB-backed tests PASS when DB available.

- [ ] **Step 4: Commit**

```bash
git add seed_demo.sql internal/store/seed.go
git commit -m "seed NUMERICAL key results with units"
```

---

## Task 10: Frontend (`internal/web/static/tracker.js`)

> No JS test runner exists (in-browser Babel). Verify manually by loading the tracker page; check create/edit/progress flows and list rendering.

**Files:**
- Modify: `internal/web/static/tracker.js`

- [ ] **Step 1: Add unit list, kind labels, and a number formatter near the top constants**

After `KR_TYPE_C` (`:110`), add:
```js
const KR_UNITS = ['%', 'RPS', 'мс', 'сек', 'мин', 'час', 'дней', 'шт', '₽', 'запросов', 'ошибок', 'пользователей', 'заказов', 'рублей'];
const KR_TYPE_LABEL = { BOOLEAN: 'Бинарный', PROJECT: 'Проектный', NUMERICAL: 'Числовой' };
const KR_TYPE_HINT = 'Бинарный — для результата, который либо выполнен, либо нет. Например: «Проведён аудит», «Запущен сервис».\n\nПроектный — для результата из нескольких этапов; прогресс считается как сумма вкладов завершённых этапов.\n\nЧисловой — для результата, измеряемого числом: проценты, деньги, RPS, штуки, дни, миллисекунды. Прогресс считается линейно от стартового значения к целевому или через промежуточные значения.';
function fmtNum(n) {
  if (n === null || n === undefined || n === '') return '';
  const num = Number(n);
  if (!isFinite(num)) return String(n);
  const parts = String(num).split('.');
  parts[0] = parts[0].replace(/\B(?=(\d{3})+(?!\d))/g, ' ');
  return parts.join('.');
}
function fmtVal(n, unit) { return unit ? `${fmtNum(n)} ${unit}` : fmtNum(n); }
```
Update `KR_TYPE_C` to include `NUMERICAL` (e.g. `NUMERICAL: '#2563eb'`); the `PERCENT`/`LINEAR` keys can stay harmlessly or be removed.

- [ ] **Step 2: Update client-side progress (`krProgress`, `:100-101`)**

Replace the percent/linear handling so NUMERICAL is computed client-side (linear when no checkpoints, step otherwise):
```js
  if (kr.krType === 'NUMERICAL') {
    const cps = (kr.checkpoints || []).slice().sort((a, b) => a.value - b.value);
    const cur = Number(kr.current);
    if (cps.length) {
      if (cur < cps[0].value) return clamp(cps[0].progress_percent);
      let p = cps[0].progress_percent;
      for (const c of cps) { if (cur >= c.value) p = c.progress_percent; else break; }
      return clamp(p);
    }
    const start = Number(kr.start), target = Number(kr.target);
    if (start === target) return cur >= target ? 100 : 0;
    return clamp(Math.round(((cur - start) / (target - start)) * 100));
  }
```
Add a `clamp` helper if none exists: `const clamp = v => Math.max(0, Math.min(100, v));`.

- [ ] **Step 3: Map server measure → form/list shape**

Where KRs are read from the API (`:73`, `buildMeasure` consumer), populate `krType: kr.kind`, and for NUMERICAL pull `kr.measure.numerical` into `start/target/current/unit/checkpoints/zeroing`. Ensure list/detail rendering reads these.

- [ ] **Step 4: Type dropdown (3 options) + info-hint in the KR form**

Replace the type `<select>` options (`:508`) with the three kinds showing Russian labels:
```jsx
{['BOOLEAN', 'PROJECT', 'NUMERICAL'].map(t => <option key={t} value={t}>{KR_TYPE_LABEL[t]}</option>)}
```
Wrap the type field label in `FieldLabel` with the hint (mirrors goal priority hint, `:1096`):
```jsx
<FieldLabel hint={KR_TYPE_HINT}>Тип Key Result</FieldLabel>
```
This renders in both create and edit forms (same component) — satisfies AC 30–33.

- [ ] **Step 5: NUMERICAL form section (unit dropdown, values, checkpoints, zeroing)**

Replace the `PERCENT`/`LINEAR` form branches (`:387-399`, `:512-529`) with a single `NUMERICAL` branch:
```jsx
{form.krType === 'NUMERICAL' && (
  <div className="kr-num-section">
    <div className="kr-num-section__title">Числовой прогресс</div>
    <label>Единица измерения
      <select value={form.unit || '%'} onChange={e => setForm({ ...form, unit: e.target.value })}>
        {KR_UNITS.map(u => <option key={u} value={u}>{u}</option>)}
      </select>
    </label>
    <label>Стартовое значение<input type="number" value={form.start} onChange={e => setForm({ ...form, start: e.target.value })} /></label>
    <label>Целевое значение<input type="number" value={form.target} onChange={e => setForm({ ...form, target: e.target.value })} /></label>
    <label>Текущее значение<input type="number" value={form.current} onChange={e => setForm({ ...form, current: e.target.value })} /></label>
    <div className="kr-checkpoints">
      <div className="kr-checkpoints__title">Промежуточные значения (необязательно)</div>
      {(form.checkpoints || []).map((c, i) => (
        <div key={i} className="kr-checkpoint-row">
          <input type="number" placeholder="значение" value={c.value}
            onChange={e => { const cps = [...form.checkpoints]; cps[i] = { ...cps[i], value: e.target.value }; setForm({ ...form, checkpoints: cps }); }} />
          <input type="number" placeholder="%" value={c.progress_percent}
            onChange={e => { const cps = [...form.checkpoints]; cps[i] = { ...cps[i], progress_percent: e.target.value }; setForm({ ...form, checkpoints: cps }); }} />
          <button type="button" onClick={() => setForm({ ...form, checkpoints: form.checkpoints.filter((_, j) => j !== i) })}>✕</button>
        </div>
      ))}
      <button type="button" onClick={() => setForm({ ...form, checkpoints: [...(form.checkpoints || []), { value: '', progress_percent: '' }] })}>+ Добавить шаг</button>
    </div>
    <label>Критерий обнуления (необязательно)
      <textarea value={form.zeroing || ''} onChange={e => setForm({ ...form, zeroing: e.target.value })} />
    </label>
  </div>
)}
```
Update the progress-only quick-edit field (`:387`) to show `Текущее значение ({fmtVal(form.start, form.unit)} → {fmtVal(form.target, form.unit)})` for NUMERICAL.

- [ ] **Step 6: Default form state + submit payload**

Change the default new-KR state (`:453`) to `krType: 'NUMERICAL', unit: '%', start: 0, target: 100, current: 0, checkpoints: [], zeroing: ''` (keep boolean/project fields). In the submit handler (`:469-473`) replace percent/linear branches with:
```js
else if (form.krType === 'NUMERICAL') {
  fd.append('numerical_unit', form.unit || '%');
  fd.append('numerical_start', String(form.start || 0));
  fd.append('numerical_target', String(form.target || 0));
  fd.append('numerical_current', String(form.current || 0));
  fd.append('numerical_zeroing', form.zeroing || '');
  (form.checkpoints || []).forEach(c => {
    if (c.value === '' || c.value === null) return;
    fd.append('checkpoint_value[]', String(c.value));
    fd.append('checkpoint_percent[]', String(c.progress_percent || 0));
  });
}
```

- [ ] **Step 7: Progress-update POST endpoint**

Find where the quick progress update posts to `/krs/{id}/progress/percent` and change the path to `/progress/numerical` (NUMERICAL). Keep boolean/project endpoints.

- [ ] **Step 8: List / detail display with units + thousands + step + zeroing**

In the KR detail rendering (`:598-599`), add the NUMERICAL case:
```jsx
else if (kr.krType === 'NUMERICAL') detail = (
  <span className="kr-detail">
    {fmtVal(kr.current, kr.unit)} / {fmtVal(kr.target, kr.unit)} — {krProgress(kr)}%
    {(kr.checkpoints && kr.checkpoints.length) ? reachedStep(kr) : null}
  </span>
);
```
Add a `reachedStep` helper that, for checkpoints, renders `Достигнутый шаг: <value> <unit> = <percent>%` for the last reached step (or null). Where KR details are shown, render `Критерий обнуления: {kr.zeroing}` when `kr.zeroing` is non-empty.

- [ ] **Step 9: Remove the `STEPS` mention and stale type strings**

Remove any remaining `PERCENT`/`LINEAR`/`STEPS` literals from user-facing copy and the error map (`:1314` `project_no_stages` is fine; ensure no type list still includes the old kinds).

- [ ] **Step 10: Manual verification**

Load the tracker page; for a team in `forming`/`ready` status:
- Create a NUMERICAL KR with unit `RPS`, start 100, target 500, current 300 → list shows `300 RPS / 500 RPS — 50%`.
- Add checkpoints 100=0, 150=50, 180=100, set current 170 → shows `… — 50%` and `Достигнутый шаг: 150 RPS = 50%`.
- Verify type dropdown shows only Бинарный/Проектный/Числовой and the `[?]` info-hint text covers all three.
- Verify thousands formatting: target 1000000 ₽ renders `1 000 000 ₽`.
- Verify no `custom_unit`/«другое»/decimals-setting controls exist.

- [ ] **Step 11: Commit**

```bash
git add internal/web/static/tracker.js
git commit -m "render and edit NUMERICAL key results with units, checkpoints, and zeroing criteria"
```

---

## Task 11: Update specs (same change set)

**Files:**
- Modify: `specs/020-domain-model.md`
- Modify: `specs/040-api-contract.md`
- Modify: `specs/030-user-flows.md` (only if it references LINEAR/PERCENT/STEPS)

- [ ] **Step 1: Update domain model spec**

In `specs/020-domain-model.md`: change KeyResult **Типы** list to `PROJECT`, `NUMERICAL`, `BOOLEAN`. Add NUMERICAL fields (`start_value`, `target_value`, `current_value`, `unit`, `checkpoints` (JSONB on key_results), `zeroing_criteria`). Replace the «Производные вычисления» lines for PERCENT/LINEAR with one NUMERICAL line: linear when no checkpoints (both directions; `start==target` → 100 if reached else 0), step function when checkpoints present. Update the «Обязательные тест-кейсы»: replace «percent KR … интерполирует» with «numerical KR с checkpoints считает по шагам (последний достигнутый шаг)»; add below-start / above-target / equal-start-target cases. State checkpoints live on `key_results` (no separate table).

- [ ] **Step 2: Update API contract spec**

In `specs/040-api-contract.md`: change the progress route line (`:269`) to `POST /api/v1/krs/{krID}/progress/numerical|boolean|project`. Document the create/update KR multipart fields for NUMERICAL (`kind=NUMERICAL`, `numerical_unit` from the closed unit set, `numerical_start/target/current`, repeated `checkpoint_value[]`/`checkpoint_percent[]`, `numerical_zeroing`). Document the `measure.numerical` response shape (`start_value`, `target_value`, `current_value`, `unit`, `checkpoints[]{value,progress_percent}`, `zeroing_criteria`). List the allowed units.

- [ ] **Step 3: Scan user-flows spec**

Run: `rg -n "PERCENT|LINEAR|STEPS|Процент|Линейн" specs/030-user-flows.md`
If matches exist, update them to NUMERICAL/Числовой; otherwise leave unchanged (do not touch unrelated specs).

- [ ] **Step 4: Commit**

```bash
git add specs/
git commit -m "update domain and API specs for NUMERICAL key result type"
```

---

## Self-Review checklist (run after implementation)

- **AC 1–2:** UI shows only Бинарный/Проектный/Числовой; no PERCENT/LINEAR/STEPS — Task 10 Steps 4, 9.
- **AC 3–7:** LINEAR/PERCENT → NUMERICAL, unit `%`, data preserved — Task 1.
- **AC 8–12:** unit dropdown from closed set, no `custom_unit`/«другое», unit shown — Tasks 2, 8, 10.
- **AC 13–17:** start/target/current, optional checkpoints, optional zeroing — Tasks 2, 5, 8, 10.
- **AC 18–19:** linear without checkpoints, step with — Task 3.
- **AC 20–26:** checkpoints JSONB on key_results, no separate table, no extra/N+1 queries, in-memory calc — Tasks 1, 5, 6.
- **AC 27–28:** thousands formatting default, no decimals setting — Task 10 Steps 1, 8, 10.
- **AC 29:** units shown in list, card, progress form — Task 10 Steps 5, 8.
- **AC 30–33:** info-hint by type select, create+edit, all three described — Task 10 Step 4.
- **AC 34:** percent stays the rollup driver — unchanged (`okr.GoalProgress`/`PeriodProgress`).
- **Tests:** migration (Task 1), progress (Task 3), store round-trip (Task 5), N+1 invariant (Task 6), DTO (Task 7), API validation (Task 8).
- **Type consistency:** `NumericalProgress`, `KRNumerical`, `KRNumericalCheckpoint{Value,ProgressPercent}`, `NumericalMetaInput`, `UpsertNumericalMeta`, `UpdateNumericalCurrent`, `UpdateKRProgressNumerical`, `dto.NumericalMeasure`, route `/progress/numerical`, form fields `numerical_*` + `checkpoint_value[]`/`checkpoint_percent[]` — used identically across tasks.
```
