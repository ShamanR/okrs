# Activity Log — Backend Implementation Plan (Plan 1 of 2)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Record OKR activity events at every mutation point and expose them via a scoped, paginated read API plus admin/system purge endpoints.

**Architecture:** New append-only `activity_events` table + `ActivityRepository` (store). The `service` layer records events best-effort after each successful mutation (actor passed explicitly, never breaks the mutation). Read side is share-aware and tenant/team-scoped. This plan is backend-only; the frontend (`/activity-log` page, tracker deep-linking, admin/system purge UI) is **Plan 2** and consumes the API defined here.

**Tech Stack:** Go, PostgreSQL (pgx v5 / pgxpool), chi router, golang-migrate, testcontainers-go for store tests. Design doc: `docs/superpowers/specs/2026-07-14-activity-log-design.md`.

## Global Constraints

- **Do NOT run `git commit`.** CLAUDE.md rule 8: the user commits. Each task ends with a **Checkpoint** (green `go build ./...` + `go test`), then stop for review. Never invoke git commit/push.
- **Schema changes only via migration.** Up/down pair, zero-padded numeric prefix, **next number is `039`**. `migration_numerical_test.go` enforces the `.up.sql`/`.down.sql` pairing — always create both.
- **All SQL lives in the repository layer.** No SQL in service/handlers. Every query filters `tenant_id` from `domain.TenantScope`.
- **Layering:** `service` must NOT import `internal/auth`. The acting user is passed as an explicit `actorUserID int64` parameter (mirrors existing `AddGoalComment(..., authorUserID)`). Handlers obtain it via `auth.UserIDFromContext(r.Context())`.
- **Best-effort recording:** an event write failure is logged (`*slog.Logger`, nil-guarded) and swallowed — it must never fail the user's mutation.
- **Store test convention:** package `<pkg>_test`; `pool, cleanup := testutil.SetupDB(t)` (skips if Docker absent); `scope := domain.TenantScope{TenantID: 1}`; seeded system user id is `int64(1)` (`anonymous-local`); no testify — assert with `t.Fatalf`.
- **Compiler-driven caller updates:** several service signatures gain an `actorUserID` param. After each such change run `go build ./...` and `go test ./...`; every failing call site is a caller to update (handlers pass `auth.UserIDFromContext(r.Context())`; tests pass a literal user id such as `1`). This is the exhaustive, non-placeholder way to find them.
- **CSRF is automatic:** the protected, admin, and system route groups already mount `csrf.Handler` — new POST endpoints under them need no per-handler CSRF code.

---

## File Structure

**Create:**
- `migrations/039_activity_events.up.sql`, `migrations/039_activity_events.down.sql`
- `internal/store/activity/activity.go` — `ActivityRepository` (Record, GetByID, List, TreeCounts, Purge) + `ListFilter`/`Cursor`
- `internal/store/activity/activity_test.go`, `internal/store/activity/activity_isolation_test.go`
- `internal/http/dto/activity.go` — response DTOs
- `internal/http/handlers/api/v1/activity/handler.go`, `routes.go`, `response.go`, `handler_test.go`, `routes_test.go`
- `seed_demo.sql` additions (activity_events)

**Modify:**
- `internal/domain/models.go` — `ActivityEvent`, `ActivityCategory`, `ActivityAction`
- `internal/store/store.go` — `Activity` field + constructor line + import
- `internal/service/service.go` — `ActivityRepo` interface, `Deps.Activity`/`Deps.Logger`, `Service.activity`/`Service.logger`, `recordActivity`, `ListActivity`/`ActivityTreeCounts`/`PurgeActivity`, and instrumentation of ~12 mutation methods (+ `actorUserID` params)
- `internal/store/goals/goals.go` — `AddGoalComment` returns `(int64, error)`
- `internal/http/server.go` — `NewFromStore` logger arg; register activity routes; admin + system purge routes
- `internal/http/handlers/api/v1/admin/handler.go` — `HandlePurgeActivity` + dep interface
- `internal/http/handlers/api/v1/system/handler.go` — `HandlePurgeActivity` + `Provisioner` method
- `internal/service/provisioning.go` — `PurgeActivityForTenant`
- specs: `020-domain-model.md`, `040-api-contract.md`, `050-permissions-and-lifecycle.md`

---

# Phase A — Store foundation

### Task A1: Migration, domain types, repository skeleton (Record + GetByID)

**Files:**
- Create: `migrations/039_activity_events.up.sql`, `migrations/039_activity_events.down.sql`
- Modify: `internal/domain/models.go` (append at end)
- Create: `internal/store/activity/activity.go`
- Modify: `internal/store/store.go`
- Create: `internal/store/activity/activity_test.go`

**Interfaces:**
- Produces `domain.ActivityEvent`, `domain.ActivityCategory`, `domain.ActivityAction` (consts below).
- Produces `activity.NewActivityRepository(db *pgxpool.Pool) *activity.ActivityRepository` with:
  - `Record(ctx context.Context, scope domain.TenantScope, ev domain.ActivityEvent) (int64, error)`
  - `GetByID(ctx context.Context, scope domain.TenantScope, id int64) (domain.ActivityEvent, error)`
- Produces `store.Store.Activity *activity.ActivityRepository`.

- [ ] **Step 1: Write the migration (up + down)**

`migrations/039_activity_events.up.sql`:
```sql
-- Activity log: append-only journal of OKR mutations (progress, composition, status, discussion).
CREATE TABLE activity_events (
    id            BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    tenant_id     BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    actor_user_id BIGINT NOT NULL REFERENCES users(id),
    category      TEXT NOT NULL CHECK (category IN ('progress','composition','status','discussion')),
    action        TEXT NOT NULL,
    team_id       BIGINT REFERENCES teams(id) ON DELETE SET NULL,
    period_id     BIGINT REFERENCES periods(id) ON DELETE SET NULL,
    goal_id       BIGINT,
    kr_id         BIGINT,
    comment_id    BIGINT,
    entity_title  TEXT NOT NULL DEFAULT '',
    payload_json  JSONB NOT NULL DEFAULT '{}',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- goal_id/kr_id/comment_id are intentionally NOT foreign keys: the journal is append-only and
-- must survive deletion of the referenced entity (entity_title keeps the row readable).
CREATE INDEX idx_activity_events_tenant_created ON activity_events(tenant_id, created_at DESC, id DESC);
CREATE INDEX idx_activity_events_tenant_period  ON activity_events(tenant_id, period_id, created_at DESC);
CREATE INDEX idx_activity_events_tenant_team    ON activity_events(tenant_id, team_id, created_at DESC);
CREATE INDEX idx_activity_events_tenant_actor   ON activity_events(tenant_id, actor_user_id);
CREATE INDEX idx_activity_events_goal           ON activity_events(goal_id);
```

`migrations/039_activity_events.down.sql`:
```sql
DROP TABLE activity_events;
```

- [ ] **Step 2: Add domain types** — append to `internal/domain/models.go`:
```go
type ActivityCategory string

const (
	ActivityProgress    ActivityCategory = "progress"
	ActivityComposition ActivityCategory = "composition"
	ActivityStatus      ActivityCategory = "status"
	ActivityDiscussion  ActivityCategory = "discussion"
)

type ActivityAction string

const (
	ActionKRProgress        ActivityAction = "kr_progress"
	ActionGoalCreated       ActivityAction = "goal_created"
	ActionGoalDeleted       ActivityAction = "goal_deleted"
	ActionKRCreated         ActivityAction = "kr_created"
	ActionKRDeleted         ActivityAction = "kr_deleted"
	ActionGoalShared        ActivityAction = "goal_shared"
	ActionGoalUnshared      ActivityAction = "goal_unshared"
	ActionGoalOwnerChanged  ActivityAction = "goal_owner_changed"
	ActionGoalFieldsChanged ActivityAction = "goal_fields_changed"
	ActionKRFieldsChanged   ActivityAction = "kr_fields_changed"
	ActionStatusChanged     ActivityAction = "status_changed"
	ActionCommentAdded      ActivityAction = "comment_added"
	ActionCommentResolved   ActivityAction = "comment_resolved"
	ActionCommentReopened   ActivityAction = "comment_reopened"
)

// ActivityEvent is one append-only journal row. Pointer fields are nullable columns.
// Actor* fields are denormalized on read (join on users/memberships); for a user who is no
// longer an active member of the tenant, ActorRemoved is true and Actor{DisplayName,AvatarURL,UDID}
// are blanked so no PII of a former member leaks.
type ActivityEvent struct {
	ID          int64
	ActorUserID int64
	Category    ActivityCategory
	Action      ActivityAction
	TeamID      *int64
	PeriodID    *int64
	GoalID      *int64
	KRID        *int64
	CommentID   *int64
	EntityTitle string
	Payload     map[string]any
	CreatedAt   time.Time

	ActorUDID        string
	ActorDisplayName string
	ActorAvatarURL   string
	ActorRemoved     bool
}
```

- [ ] **Step 3: Write the repository skeleton** — `internal/store/activity/activity.go`:
```go
package activity

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"okrs/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ActivityRepository persists the append-only activity_events journal.
type ActivityRepository struct {
	db *pgxpool.Pool
}

func NewActivityRepository(db *pgxpool.Pool) *ActivityRepository {
	return &ActivityRepository{db: db}
}

var ErrNotFound = errors.New("activity event not found")

// Record inserts one event and returns its id.
func (r *ActivityRepository) Record(ctx context.Context, scope domain.TenantScope, ev domain.ActivityEvent) (int64, error) {
	payload := ev.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	var id int64
	err = r.db.QueryRow(ctx, `
		INSERT INTO activity_events
			(tenant_id, actor_user_id, category, action, team_id, period_id, goal_id, kr_id, comment_id, entity_title, payload_json)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
		scope.TenantID, ev.ActorUserID, ev.Category, ev.Action,
		ev.TeamID, ev.PeriodID, ev.GoalID, ev.KRID, ev.CommentID, ev.EntityTitle, raw,
	).Scan(&id)
	return id, err
}

// GetByID returns a single event (no actor join; used by tests and internal lookups).
func (r *ActivityRepository) GetByID(ctx context.Context, scope domain.TenantScope, id int64) (domain.ActivityEvent, error) {
	var ev domain.ActivityEvent
	var raw []byte
	err := r.db.QueryRow(ctx, `
		SELECT id, actor_user_id, category, action, team_id, period_id, goal_id, kr_id, comment_id, entity_title, payload_json, created_at
		FROM activity_events WHERE id=$1 AND tenant_id=$2`, id, scope.TenantID).
		Scan(&ev.ID, &ev.ActorUserID, &ev.Category, &ev.Action, &ev.TeamID, &ev.PeriodID,
			&ev.GoalID, &ev.KRID, &ev.CommentID, &ev.EntityTitle, &raw, &ev.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ActivityEvent{}, ErrNotFound
		}
		return domain.ActivityEvent{}, err
	}
	_ = json.Unmarshal(raw, &ev.Payload)
	return ev, nil
}

// ── used by later tasks (List/TreeCounts/Purge) ──────────────────────────────
// The unused imports (strconv, strings, time, pgconn) are consumed by Tasks A2–A4.
var _ = strconv.Itoa
var _ = strings.Join
var _ = time.Now
var _ pgconn.CommandTag
```
> The trailing `var _` lines are placeholders **only to keep A1 compiling in isolation** — delete them in A2 once the imports are used. If you implement A1→A4 back-to-back, you may add the imports incrementally instead.

- [ ] **Step 4: Wire into `store.Store`** — `internal/store/store.go`: add the import in the import block:
```go
	"okrs/internal/store/activity"
```
add the field after `Settings` (line ~44):
```go
	Activity *activity.ActivityRepository
```
add the constructor line after `Settings:` (line ~66):
```go
		Activity: activity.NewActivityRepository(db),
```

- [ ] **Step 5: Write the round-trip test** — `internal/store/activity/activity_test.go`:
```go
package activity_test

import (
	"context"
	"testing"
	"time"

	"okrs/internal/domain"
	"okrs/internal/store/activity"
	"okrs/internal/store/goals"
	"okrs/internal/store/krs"
	"okrs/internal/store/periods"
	"okrs/internal/store/teams"
	"okrs/internal/store/testutil"
)

const seedUserID = int64(1)

// seedGoal creates a team+period+goal under scope, returning (teamID, periodID, goalID).
func seedGoal(t *testing.T, ctx context.Context, pool interface{ /* pgxpool.Pool */ }, scope domain.TenantScope, name string) (int64, int64, int64) {
	t.Helper()
	// NOTE: build the repos from the *pgxpool.Pool passed by the caller; see usage below.
	panic("replaced inline in test bodies")
}

func TestRecordAndGetByID(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}

	tr := teams.NewTeamRepository(pool)
	pr := periods.NewPeriodRepository(pool)
	gr := goals.NewGoalRepository(pool, krs.NewKRRepository(pool))
	ar := activity.NewActivityRepository(pool)

	teamID, err := tr.CreateTeam(ctx, scope, teams.TeamInput{Name: "Платформа", Type: domain.TeamTypeTeam})
	if err != nil {
		t.Fatalf("team: %v", err)
	}
	periodID, err := pr.CreatePeriod(ctx, scope, periods.PeriodInput{
		Name: "Q1", StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), EndDate: time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("period: %v", err)
	}
	goalID, err := gr.CreateGoal(ctx, scope, goals.GoalInput{
		TeamID: teamID, PeriodID: periodID, Title: "P95 latency", Priority: domain.PriorityP1,
		Weight: 100, WorkType: domain.WorkTypeDelivery, FocusType: domain.FocusStability,
	})
	if err != nil {
		t.Fatalf("goal: %v", err)
	}

	id, err := ar.Record(ctx, scope, domain.ActivityEvent{
		ActorUserID: seedUserID, Category: domain.ActivityProgress, Action: domain.ActionKRProgress,
		TeamID: &teamID, PeriodID: &periodID, GoalID: &goalID, EntityTitle: "P95 latency",
		Payload: map[string]any{"before": map[string]any{"progress": 40}, "after": map[string]any{"progress": 61}},
	})
	if err != nil || id == 0 {
		t.Fatalf("record: id=%d err=%v", id, err)
	}
	got, err := ar.GetByID(ctx, scope, id)
	if err != nil {
		t.Fatalf("getByID: %v", err)
	}
	if got.Action != domain.ActionKRProgress || got.EntityTitle != "P95 latency" {
		t.Fatalf("unexpected event: %+v", got)
	}
	if got.Payload["after"].(map[string]any)["progress"].(float64) != 61 {
		t.Fatalf("payload roundtrip: %+v", got.Payload)
	}
	// tenant isolation: another tenant cannot read it
	if _, err := ar.GetByID(ctx, domain.TenantScope{TenantID: 999}, id); err != activity.ErrNotFound {
		t.Fatalf("cross-tenant read: want ErrNotFound, got %v", err)
	}
}
```
> Delete the stub `seedGoal`/`pool interface{}` helper — it is inlined in the test body above. (It is shown only to flag that later tests reuse the same team/period/goal fixture; copy the inline block.)

- [ ] **Step 6: Run migration-pairing + repo tests**

Run: `go build ./... && go test ./internal/store/activity/ ./internal/store/ -run 'Activity|Migration' -v`
Expected: PASS (or `SKIP` for the container test if Docker is unavailable — that is acceptable locally; CI has Docker). `TestMigrationNumericalConsistency` must PASS (both 039 files present).

- [ ] **Step 7: Checkpoint** — `go build ./... && go vet ./...` clean. Do **not** commit; stop for review.

---

### Task A2: `List` — filters, share-aware visibility, cursor pagination, actor join

**Files:**
- Modify: `internal/store/activity/activity.go`
- Modify: `internal/store/activity/activity_test.go`

**Interfaces:**
- Consumes `domain.ActivityEvent`, `store.Store.Activity`.
- Produces:
```go
type Cursor struct { CreatedAt time.Time; ID int64 }
type ListFilter struct {
	PeriodID  *int64
	TeamIDs   []int64   // sidebar team click (audience match)
	Category  string    // "" = all
	ActorUDID string    // "" = all authors
	Since     *time.Time
	Query     string
	Limit     int       // default 50, max 100
	Cursor    *Cursor
}
func (r *ActivityRepository) List(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, f ListFilter) ([]domain.ActivityEvent, *Cursor, error)
```
- **Contract:** `allowedTeamIDs == nil` means admin/unrestricted (no team predicate). A non-nil slice (incl. empty) restricts to events whose *audience* — owner team **or** any team the goal is shared to — intersects the slice. Removed actors have blanked PII + `ActorRemoved=true`.

- [ ] **Step 1: Write failing tests** — append to `activity_test.go`:
```go
func TestListShareAwareAndScope(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}

	tr := teams.NewTeamRepository(pool)
	pr := periods.NewPeriodRepository(pool)
	gr := goals.NewGoalRepository(pool, krs.NewKRRepository(pool))
	sr := shares.NewGoalShareRepository(pool)
	ar := activity.NewActivityRepository(pool)

	ownerTeam, _ := tr.CreateTeam(ctx, scope, teams.TeamInput{Name: "Owner", Type: domain.TeamTypeTeam})
	shareTeam, _ := tr.CreateTeam(ctx, scope, teams.TeamInput{Name: "Sharee", Type: domain.TeamTypeTeam})
	otherTeam, _ := tr.CreateTeam(ctx, scope, teams.TeamInput{Name: "Other", Type: domain.TeamTypeTeam})
	periodID, _ := pr.CreatePeriod(ctx, scope, periods.PeriodInput{Name: "Q1", StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), EndDate: time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)})
	goalID, _ := gr.CreateGoal(ctx, scope, goals.GoalInput{TeamID: ownerTeam, PeriodID: periodID, Title: "Shared goal", Priority: domain.PriorityP1, Weight: 100, WorkType: domain.WorkTypeDelivery, FocusType: domain.FocusStability})
	if err := sr.ReplaceGoalShares(ctx, scope, goalID, []shares.GoalShareInput{{TeamID: shareTeam, Weight: 50}}); err != nil {
		t.Fatalf("share: %v", err)
	}
	if _, err := ar.Record(ctx, scope, domain.ActivityEvent{ActorUserID: seedUserID, Category: domain.ActivityDiscussion, Action: domain.ActionCommentAdded, TeamID: &ownerTeam, PeriodID: &periodID, GoalID: &goalID, EntityTitle: "Shared goal"}); err != nil {
		t.Fatalf("record: %v", err)
	}

	// admin (nil) sees it
	if evs, _, _ := ar.List(ctx, scope, nil, activity.ListFilter{}); len(evs) != 1 {
		t.Fatalf("admin list: want 1 got %d", len(evs))
	}
	// sharee-team accessor sees it (share-aware)
	if evs, _, _ := ar.List(ctx, scope, []int64{shareTeam}, activity.ListFilter{}); len(evs) != 1 {
		t.Fatalf("sharee list: want 1 got %d", len(evs))
	}
	// unrelated team does NOT
	if evs, _, _ := ar.List(ctx, scope, []int64{otherTeam}, activity.ListFilter{}); len(evs) != 0 {
		t.Fatalf("other list: want 0 got %d", len(evs))
	}
	// empty allowed set => no access => nothing
	if evs, _, _ := ar.List(ctx, scope, []int64{}, activity.ListFilter{}); len(evs) != 0 {
		t.Fatalf("empty list: want 0 got %d", len(evs))
	}
	// period filter mismatch => nothing
	wrong := periodID + 12345
	if evs, _, _ := ar.List(ctx, scope, nil, activity.ListFilter{PeriodID: &wrong}); len(evs) != 0 {
		t.Fatalf("wrong period: want 0 got %d", len(evs))
	}
}

func TestListCursorPagination(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}
	tr := teams.NewTeamRepository(pool)
	ar := activity.NewActivityRepository(pool)
	team, _ := tr.CreateTeam(ctx, scope, teams.TeamInput{Name: "T", Type: domain.TeamTypeTeam})
	for i := 0; i < 5; i++ {
		if _, err := ar.Record(ctx, scope, domain.ActivityEvent{ActorUserID: seedUserID, Category: domain.ActivityStatus, Action: domain.ActionStatusChanged, TeamID: &team}); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
	}
	page1, cur, _ := ar.List(ctx, scope, nil, activity.ListFilter{Limit: 2})
	if len(page1) != 2 || cur == nil {
		t.Fatalf("page1: n=%d cur=%v", len(page1), cur)
	}
	page2, _, _ := ar.List(ctx, scope, nil, activity.ListFilter{Limit: 2, Cursor: cur})
	if len(page2) != 2 || page2[0].ID >= page1[1].ID {
		t.Fatalf("page2 not older than page1: %+v %+v", page1, page2)
	}
}
```
(add `"okrs/internal/store/shares"` to the test imports.)

- [ ] **Step 2: Run tests to confirm they fail** — Run: `go test ./internal/store/activity/ -run TestList -v` → FAIL (`List` undefined).

- [ ] **Step 3: Implement `List`** — in `activity.go` remove the A1 placeholder `var _` lines and add:
```go
type Cursor struct {
	CreatedAt time.Time
	ID        int64
}

type ListFilter struct {
	PeriodID  *int64
	TeamIDs   []int64
	Category  string
	ActorUDID string
	Since     *time.Time
	Query     string
	Limit     int
	Cursor    *Cursor
}

// audiencePredicate returns SQL matching events whose audience (owner team OR any shared team)
// intersects the team ids bound to placeholder p.
func audiencePredicate(p string) string {
	return "(e.team_id = ANY(" + p + ") OR EXISTS (SELECT 1 FROM goal_shares gs WHERE gs.goal_id = e.goal_id AND gs.team_id = ANY(" + p + ")))"
}

func (r *ActivityRepository) List(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, f ListFilter) ([]domain.ActivityEvent, *Cursor, error) {
	var where []string
	var args []any
	arg := func(v any) string { args = append(args, v); return "$" + strconv.Itoa(len(args)) }

	where = append(where, "e.tenant_id = "+arg(scope.TenantID))
	if allowedTeamIDs != nil { // nil = admin/unrestricted
		where = append(where, audiencePredicate(arg(allowedTeamIDs)))
	}
	if f.PeriodID != nil {
		where = append(where, "e.period_id = "+arg(*f.PeriodID))
	}
	if len(f.TeamIDs) > 0 {
		where = append(where, audiencePredicate(arg(f.TeamIDs)))
	}
	if f.Category != "" {
		where = append(where, "e.category = "+arg(f.Category))
	}
	if f.ActorUDID != "" {
		where = append(where, "e.actor_user_id = (SELECT id FROM users WHERE udid = "+arg(f.ActorUDID)+")")
	}
	if f.Since != nil {
		where = append(where, "e.created_at >= "+arg(*f.Since))
	}
	if f.Query != "" {
		q := arg("%" + strings.ToLower(f.Query) + "%")
		where = append(where, "(LOWER(e.entity_title) LIKE "+q+" OR LOWER(e.payload_json::text) LIKE "+q+" OR LOWER(COALESCE(u.display_name,'')) LIKE "+q+")")
	}
	if f.Cursor != nil {
		where = append(where, "(e.created_at, e.id) < ("+arg(f.Cursor.CreatedAt)+", "+arg(f.Cursor.ID)+")")
	}

	limit := f.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	limPlaceholder := arg(limit + 1) // one extra row => there is a next page

	sql := `
		SELECT e.id, e.actor_user_id, e.category, e.action, e.team_id, e.period_id, e.goal_id, e.kr_id, e.comment_id,
		       e.entity_title, e.payload_json, e.created_at,
		       COALESCE(u.udid,''), COALESCE(u.display_name,''), COALESCE(u.avatar_url,''), COALESCE(u.provider,''),
		       EXISTS (SELECT 1 FROM memberships m WHERE m.user_id = e.actor_user_id AND m.tenant_id = e.tenant_id AND m.status = 'active')
		FROM activity_events e
		LEFT JOIN users u ON u.id = e.actor_user_id
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY e.created_at DESC, e.id DESC
		LIMIT ` + limPlaceholder

	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var out []domain.ActivityEvent
	for rows.Next() {
		var ev domain.ActivityEvent
		var raw []byte
		var provider string
		var active bool
		if err := rows.Scan(&ev.ID, &ev.ActorUserID, &ev.Category, &ev.Action, &ev.TeamID, &ev.PeriodID,
			&ev.GoalID, &ev.KRID, &ev.CommentID, &ev.EntityTitle, &raw, &ev.CreatedAt,
			&ev.ActorUDID, &ev.ActorDisplayName, &ev.ActorAvatarURL, &provider, &active); err != nil {
			return nil, nil, err
		}
		_ = json.Unmarshal(raw, &ev.Payload)
		// A former member (non-system, not currently active) must not leak PII.
		if provider != "system" && !active {
			ev.ActorRemoved = true
			ev.ActorDisplayName = ""
			ev.ActorAvatarURL = ""
			ev.ActorUDID = ""
		}
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	var next *Cursor
	if len(out) > limit {
		last := out[limit-1]
		next = &Cursor{CreatedAt: last.CreatedAt, ID: last.ID}
		out = out[:limit]
	}
	return out, next, nil
}
```

- [ ] **Step 4: Run tests** — Run: `go test ./internal/store/activity/ -run TestList -v` → PASS.

- [ ] **Step 5: Checkpoint** — `go build ./... && go vet ./...` clean. Do not commit.

---

### Task A3: `TreeCounts` — audience-expanded per-team counts

**Files:**
- Modify: `internal/store/activity/activity.go`
- Modify: `internal/store/activity/activity_test.go`

**Interfaces:**
- Produces `func (r *ActivityRepository) TreeCounts(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, periodID *int64, since *time.Time) (map[int64]int, error)`.
- **Contract:** returns direct per-team counts where a team's count includes every event whose audience contains it (owner team + each shared team). `allowedTeamIDs == nil` = admin (all teams). The frontend rolls these up over the subtree.

- [ ] **Step 1: Write failing test** — append to `activity_test.go`:
```go
func TestTreeCountsAudience(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}
	tr := teams.NewTeamRepository(pool)
	pr := periods.NewPeriodRepository(pool)
	gr := goals.NewGoalRepository(pool, krs.NewKRRepository(pool))
	sr := shares.NewGoalShareRepository(pool)
	ar := activity.NewActivityRepository(pool)

	owner, _ := tr.CreateTeam(ctx, scope, teams.TeamInput{Name: "Owner", Type: domain.TeamTypeTeam})
	sharee, _ := tr.CreateTeam(ctx, scope, teams.TeamInput{Name: "Sharee", Type: domain.TeamTypeTeam})
	periodID, _ := pr.CreatePeriod(ctx, scope, periods.PeriodInput{Name: "Q1", StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), EndDate: time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)})
	goalID, _ := gr.CreateGoal(ctx, scope, goals.GoalInput{TeamID: owner, PeriodID: periodID, Title: "G", Priority: domain.PriorityP1, Weight: 100, WorkType: domain.WorkTypeDelivery, FocusType: domain.FocusStability})
	_ = sr.ReplaceGoalShares(ctx, scope, goalID, []shares.GoalShareInput{{TeamID: sharee, Weight: 50}})
	// one shared-goal event → counts under BOTH owner and sharee
	_, _ = ar.Record(ctx, scope, domain.ActivityEvent{ActorUserID: seedUserID, Category: domain.ActivityProgress, Action: domain.ActionKRProgress, TeamID: &owner, PeriodID: &periodID, GoalID: &goalID})
	// one owner-only status event (no goal) → counts under owner only
	_, _ = ar.Record(ctx, scope, domain.ActivityEvent{ActorUserID: seedUserID, Category: domain.ActivityStatus, Action: domain.ActionStatusChanged, TeamID: &owner, PeriodID: &periodID})

	counts, err := ar.TreeCounts(ctx, scope, nil, &periodID, nil)
	if err != nil {
		t.Fatalf("treecounts: %v", err)
	}
	if counts[owner] != 2 {
		t.Fatalf("owner count: want 2 got %d", counts[owner])
	}
	if counts[sharee] != 1 {
		t.Fatalf("sharee count: want 1 got %d", counts[sharee])
	}
	// restricted to sharee only
	restricted, _ := ar.TreeCounts(ctx, scope, []int64{sharee}, &periodID, nil)
	if restricted[owner] != 0 || restricted[sharee] != 1 {
		t.Fatalf("restricted counts: %+v", restricted)
	}
}
```

- [ ] **Step 2: Run to confirm failure** — Run: `go test ./internal/store/activity/ -run TestTreeCounts -v` → FAIL.

- [ ] **Step 3: Implement** — add to `activity.go`:
```go
func (r *ActivityRepository) TreeCounts(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, periodID *int64, since *time.Time) (map[int64]int, error) {
	var filter []string
	var args []any
	arg := func(v any) string { args = append(args, v); return "$" + strconv.Itoa(len(args)) }

	filter = append(filter, "e.tenant_id = "+arg(scope.TenantID))
	if periodID != nil {
		filter = append(filter, "e.period_id = "+arg(*periodID))
	}
	if since != nil {
		filter = append(filter, "e.created_at >= "+arg(*since))
	}
	base := strings.Join(filter, " AND ")

	sql := `
		SELECT team_id, count(*) FROM (
			SELECT e.id, e.team_id AS team_id FROM activity_events e WHERE ` + base + ` AND e.team_id IS NOT NULL
			UNION ALL
			SELECT e.id, gs.team_id FROM activity_events e JOIN goal_shares gs ON gs.goal_id = e.goal_id WHERE ` + base + `
		) x`
	if allowedTeamIDs != nil {
		sql += " WHERE team_id = ANY(" + arg(allowedTeamIDs) + ")"
	}
	sql += " GROUP BY team_id"

	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := make(map[int64]int)
	for rows.Next() {
		var teamID int64
		var n int
		if err := rows.Scan(&teamID, &n); err != nil {
			return nil, err
		}
		counts[teamID] = n
	}
	return counts, rows.Err()
}
```
> The two `WHERE `+base` occurrences reuse the same `$1..$N` placeholders — pgx binds each once from `args`; the `allowedTeamIDs` placeholder is appended afterwards so its number is correct.

- [ ] **Step 4: Run** — `go test ./internal/store/activity/ -run TestTreeCounts -v` → PASS.

- [ ] **Step 5: Checkpoint** — `go build ./... && go vet ./...`. Do not commit.

---

### Task A4: `Purge` + tenant-isolation test

**Files:**
- Modify: `internal/store/activity/activity.go`
- Create: `internal/store/activity/activity_isolation_test.go`

**Interfaces:**
- Produces `func (r *ActivityRepository) Purge(ctx context.Context, scope domain.TenantScope, olderThan *time.Time) (int64, error)` — deletes tenant events; `olderThan == nil` deletes all; returns rows deleted.

- [ ] **Step 1: Write failing test** — `internal/store/activity/activity_isolation_test.go`:
```go
package activity_test

import (
	"context"
	"testing"
	"time"

	"okrs/internal/domain"
	"okrs/internal/store/activity"
	"okrs/internal/store/teams"
	"okrs/internal/store/testutil"
)

func TestPurgeIsTenantScopedAndDepth(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO tenants (id, slug, name) OVERRIDING SYSTEM VALUE VALUES (2,'t2','T2')`); err != nil {
		t.Fatalf("tenant 2: %v", err)
	}
	s1 := domain.TenantScope{TenantID: 1}
	s2 := domain.TenantScope{TenantID: 2}
	tr := teams.NewTeamRepository(pool)
	ar := activity.NewActivityRepository(pool)
	team1, _ := tr.CreateTeam(ctx, s1, teams.TeamInput{Name: "T1", Type: domain.TeamTypeTeam})

	old := time.Now().Add(-400 * 24 * time.Hour)
	recent := time.Now()
	// two rows in tenant 1 (one old, one recent); backdate the old one directly
	idOld, _ := ar.Record(ctx, s1, domain.ActivityEvent{ActorUserID: 1, Category: domain.ActivityStatus, Action: domain.ActionStatusChanged, TeamID: &team1})
	_, _ = ar.Record(ctx, s1, domain.ActivityEvent{ActorUserID: 1, Category: domain.ActivityStatus, Action: domain.ActionStatusChanged, TeamID: &team1})
	if _, err := pool.Exec(ctx, `UPDATE activity_events SET created_at=$1 WHERE id=$2`, old, idOld); err != nil {
		t.Fatalf("backdate: %v", err)
	}
	// one row in tenant 2
	_, _ = ar.Record(ctx, s2, domain.ActivityEvent{ActorUserID: 1, Category: domain.ActivityStatus, Action: domain.ActionStatusChanged})

	// purge tenant1 older than 90 days => removes only the old one
	cutoff := recent.Add(-90 * 24 * time.Hour)
	n, err := ar.Purge(ctx, s1, &cutoff)
	if err != nil || n != 1 {
		t.Fatalf("purge quarter: n=%d err=%v", n, err)
	}
	// tenant2 untouched
	if evs, _, _ := ar.List(ctx, s2, nil, activity.ListFilter{}); len(evs) != 1 {
		t.Fatalf("tenant2 leaked: %d", len(evs))
	}
	// purge all of tenant1 => removes the remaining recent one
	n, err = ar.Purge(ctx, s1, nil)
	if err != nil || n != 1 {
		t.Fatalf("purge all: n=%d err=%v", n, err)
	}
	if evs, _, _ := ar.List(ctx, s1, nil, activity.ListFilter{}); len(evs) != 0 {
		t.Fatalf("tenant1 not empty: %d", len(evs))
	}
}
```

- [ ] **Step 2: Run to confirm failure** — Run: `go test ./internal/store/activity/ -run TestPurge -v` → FAIL.

- [ ] **Step 3: Implement** — add to `activity.go`:
```go
func (r *ActivityRepository) Purge(ctx context.Context, scope domain.TenantScope, olderThan *time.Time) (int64, error) {
	var tag pgconn.CommandTag
	var err error
	if olderThan == nil {
		tag, err = r.db.Exec(ctx, `DELETE FROM activity_events WHERE tenant_id=$1`, scope.TenantID)
	} else {
		tag, err = r.db.Exec(ctx, `DELETE FROM activity_events WHERE tenant_id=$1 AND created_at < $2`, scope.TenantID, *olderThan)
	}
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
```

- [ ] **Step 4: Run** — `go test ./internal/store/activity/ -run TestPurge -v` → PASS.

- [ ] **Step 5: Checkpoint** — `go build ./... && go test ./internal/store/activity/`. Do not commit.

---

# Phase B — Service recording

### Task B1: Service wiring — ActivityRepo, logger, `recordActivity`, read/purge methods

**Files:**
- Modify: `internal/service/service.go`
- Modify: `internal/http/server.go` (the single `NewFromStore` caller)
- Create: `internal/service/activity_test.go`

**Interfaces:**
- Produces on `*service.Service`:
```go
func (s *Service) recordActivity(ctx context.Context, scope domain.TenantScope, ev domain.ActivityEvent) // best-effort, unexported
func (s *Service) ListActivity(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, f activity.ListFilter) ([]domain.ActivityEvent, *activity.Cursor, error)
func (s *Service) ActivityTreeCounts(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, periodID *int64, since *time.Time) (map[int64]int, error)
func (s *Service) PurgeActivity(ctx context.Context, scope domain.TenantScope, olderThan *time.Time) (int64, error)
```
- Changes `service.NewFromStore` signature to accept a trailing `logger *slog.Logger`.

- [ ] **Step 1: Add the interface + wiring** — in `service.go`:

Add imports:
```go
	"log/slog"

	"okrs/internal/store/activity"
```
Add the interface (near the other repo interfaces, ~line 106):
```go
type ActivityRepo interface {
	Record(ctx context.Context, scope domain.TenantScope, ev domain.ActivityEvent) (int64, error)
	List(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, f activity.ListFilter) ([]domain.ActivityEvent, *activity.Cursor, error)
	TreeCounts(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, periodID *int64, since *time.Time) (map[int64]int, error)
	Purge(ctx context.Context, scope domain.TenantScope, olderThan *time.Time) (int64, error)
}
```
Add to `Deps` (after `HCCache`):
```go
	Activity ActivityRepo
	Logger   *slog.Logger
```
Add to `Service` struct (after `hcCache`):
```go
	activity ActivityRepo
	logger   *slog.Logger
```
Map in `New` (after `hcCache:`):
```go
		activity: deps.Activity,
		logger:   deps.Logger,
```
Change `NewFromStore` signature + body:
```go
func NewFromStore(st *store.Store, grantsProvider GrantsProvider, hcCache *HealthCheckInCache, logger *slog.Logger) *Service {
	return New(Deps{
		Teams:    st.Teams,
		Goals:    st.Goals,
		Shares:   st.Shares,
		Periods:  st.Periods,
		KRs:      st.KRs,
		Statuses: st.Statuses,
		Users:    st.Users,
		Grants:   grantsProvider,
		HCCache:  hcCache,
		Activity: st.Activity,
		Logger:   logger,
	})
}
```

- [ ] **Step 2: Add helper + read/purge methods** — append to `service.go`:
```go
// recordActivity persists one event best-effort: a failure is logged, never returned,
// so the activity journal can never break the user's mutation.
func (s *Service) recordActivity(ctx context.Context, scope domain.TenantScope, ev domain.ActivityEvent) {
	if s.activity == nil {
		return
	}
	if _, err := s.activity.Record(ctx, scope, ev); err != nil && s.logger != nil {
		s.logger.Warn("activity: record failed", "action", string(ev.Action), "tenant", scope.TenantID, "err", err)
	}
}

func (s *Service) ListActivity(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, f activity.ListFilter) ([]domain.ActivityEvent, *activity.Cursor, error) {
	return s.activity.List(ctx, scope, allowedTeamIDs, f)
}

func (s *Service) ActivityTreeCounts(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, periodID *int64, since *time.Time) (map[int64]int, error) {
	return s.activity.TreeCounts(ctx, scope, allowedTeamIDs, periodID, since)
}

// PurgeActivity deletes journal rows for the caller's tenant. Authority (tenant-admin) is
// enforced by RequireTenantAdminMiddleware on the route; the system plane uses
// ProvisioningService.PurgeActivityForTenant instead.
func (s *Service) PurgeActivity(ctx context.Context, scope domain.TenantScope, olderThan *time.Time) (int64, error) {
	return s.activity.Purge(ctx, scope, olderThan)
}
```

- [ ] **Step 3: Fix the `NewFromStore` caller** — in `internal/http/server.go` find the `service.NewFromStore(...)` call (grep: `rg -n "NewFromStore" internal/http/server.go`) and add `s.logger` as the final argument, e.g.:
```go
	s.service = service.NewFromStore(s.store, s.grantsCache, s.hcCache, s.logger)
```
Then `go build ./...` — fix any other `NewFromStore` callers (tests) the compiler flags by passing `nil` for the logger.

- [ ] **Step 4: Write the best-effort test** — `internal/service/activity_test.go`:
```go
package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"okrs/internal/domain"
	"okrs/internal/store/activity"
)

type fakeActivityRepo struct {
	recorded []domain.ActivityEvent
	failNext bool
}

func (f *fakeActivityRepo) Record(_ context.Context, _ domain.TenantScope, ev domain.ActivityEvent) (int64, error) {
	if f.failNext {
		return 0, errors.New("boom")
	}
	f.recorded = append(f.recorded, ev)
	return int64(len(f.recorded)), nil
}
func (f *fakeActivityRepo) List(context.Context, domain.TenantScope, []int64, activity.ListFilter) ([]domain.ActivityEvent, *activity.Cursor, error) {
	return nil, nil, nil
}
func (f *fakeActivityRepo) TreeCounts(context.Context, domain.TenantScope, []int64, *int64, *time.Time) (map[int64]int, error) {
	return nil, nil
}
func (f *fakeActivityRepo) Purge(context.Context, domain.TenantScope, *time.Time) (int64, error) {
	return 0, nil
}

func TestRecordActivityIsBestEffort(t *testing.T) {
	fa := &fakeActivityRepo{failNext: true}
	s := New(Deps{Activity: fa}) // logger nil → must not panic
	// must not panic and must not surface the error
	s.recordActivity(context.Background(), domain.TenantScope{TenantID: 1}, domain.ActivityEvent{Action: domain.ActionStatusChanged})
	if len(fa.recorded) != 0 {
		t.Fatalf("expected no recorded event on failure")
	}
	fa.failNext = false
	s.recordActivity(context.Background(), domain.TenantScope{TenantID: 1}, domain.ActivityEvent{Action: domain.ActionStatusChanged})
	if len(fa.recorded) != 1 {
		t.Fatalf("expected 1 recorded event, got %d", len(fa.recorded))
	}
}
```
> This test is in-package (`package service`) so it can call the unexported `recordActivity`. Reuse `fakeActivityRepo` in later B-tasks.

- [ ] **Step 5: Run** — `go test ./internal/service/ -run TestRecordActivityIsBestEffort -v` → PASS. Then `go build ./...` clean.

- [ ] **Step 6: Checkpoint** — `go build ./... && go test ./internal/service/ ./internal/http/...`. Do not commit.

---

### Task B2: Instrument discussion events (comments)

**Files:**
- Modify: `internal/store/goals/goals.go` (`AddGoalComment` returns id)
- Modify: `internal/service/service.go` (`GoalRepo` interface + `AddGoalComment`/`SetGoalCommentResolved`)
- Modify: `internal/service/activity_test.go`

**Interfaces:**
- `GoalRepository.AddGoalComment` / `GoalRepo.AddGoalComment` now return `(int64, error)`.
- `Service.AddGoalComment` and `Service.SetGoalCommentResolved` record `discussion` events. Signatures unchanged (both already carry the actor).

- [ ] **Step 1: Change the store method to return the id** — in `internal/store/goals/goals.go`:
```go
func (r *GoalRepository) AddGoalComment(ctx context.Context, scope domain.TenantScope, goalID int64, text string, authorUserID int64) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx,
		`INSERT INTO goal_comments (goal_id, text, author_user_id, tenant_id) VALUES ($1,$2,$3,$4) RETURNING id`,
		goalID, text, authorUserID, scope.TenantID).Scan(&id)
	return id, err
}
```
Update the `GoalRepo` interface in `service.go`:
```go
	AddGoalComment(ctx context.Context, scope domain.TenantScope, goalID int64, text string, authorUserID int64) (int64, error)
```

- [ ] **Step 2: Instrument the service methods** — replace `Service.AddGoalComment` and `Service.SetGoalCommentResolved` in `service.go`:
```go
func (s *Service) AddGoalComment(ctx context.Context, scope domain.TenantScope, goalID int64, text string, authorUserID int64) error {
	commentID, err := s.goals.AddGoalComment(ctx, scope, goalID, text, authorUserID)
	if err != nil {
		return err
	}
	if g, gerr := s.goals.GetGoal(ctx, scope, goalID); gerr == nil {
		teamID, periodID := g.TeamID, g.PeriodID
		s.recordActivity(ctx, scope, domain.ActivityEvent{
			ActorUserID: authorUserID, Category: domain.ActivityDiscussion, Action: domain.ActionCommentAdded,
			TeamID: &teamID, PeriodID: &periodID, GoalID: &goalID, CommentID: &commentID,
			EntityTitle: g.Title, Payload: map[string]any{"text": text},
		})
	}
	return nil
}

func (s *Service) SetGoalCommentResolved(ctx context.Context, scope domain.TenantScope, goalID, commentID int64, resolved bool, userID int64) error {
	if err := s.goals.SetGoalCommentResolved(ctx, scope, goalID, commentID, resolved, userID); err != nil {
		return err
	}
	if g, gerr := s.goals.GetGoal(ctx, scope, goalID); gerr == nil {
		action := domain.ActionCommentReopened
		if resolved {
			action = domain.ActionCommentResolved
		}
		teamID, periodID := g.TeamID, g.PeriodID
		s.recordActivity(ctx, scope, domain.ActivityEvent{
			ActorUserID: userID, Category: domain.ActivityDiscussion, Action: action,
			TeamID: &teamID, PeriodID: &periodID, GoalID: &goalID, CommentID: &commentID,
			EntityTitle: g.Title,
			Payload: map[string]any{"before": map[string]any{"resolved": !resolved}, "after": map[string]any{"resolved": resolved}},
		})
	}
	return nil
}
```

- [ ] **Step 3: Fix all callers of the changed store signature** — Run `go build ./...`. Update every failing call site of `AddGoalComment` (store tests, handler tests) to consume the new `(id, err)`. In test files that ignored the return, change `err := ...AddGoalComment(...)` to `_, err := ...AddGoalComment(...)`. Known sites: `internal/store/goals/goals_comments_test.go`, `internal/http/handlers/api/v1/goals/resolve_test.go`. Re-run `go build ./...` until clean.

- [ ] **Step 4: Write the recording test** — append to `internal/service/activity_test.go` a test using a fake `GoalRepo`. Add a minimal fake that satisfies just the methods used:
```go
type fakeGoalRepoDiscussion struct {
	nextCommentID int64
	goal          domain.Goal
}

func (f *fakeGoalRepoDiscussion) AddGoalComment(context.Context, domain.TenantScope, int64, string, int64) (int64, error) {
	f.nextCommentID++
	return f.nextCommentID, nil
}
func (f *fakeGoalRepoDiscussion) SetGoalCommentResolved(context.Context, domain.TenantScope, int64, int64, bool, int64) error {
	return nil
}
func (f *fakeGoalRepoDiscussion) GetGoal(context.Context, domain.TenantScope, int64) (domain.Goal, error) {
	return f.goal, nil
}

// The remaining GoalRepo methods are unused here; embed the interface to satisfy it with nil bodies.
type stubGoalRepo struct{ GoalRepo }

func TestAddGoalCommentRecordsEvent(t *testing.T) {
	fa := &fakeActivityRepo{}
	fg := &fakeGoalRepoDiscussion{goal: domain.Goal{ID: 7, TeamID: 42, PeriodID: 3, Title: "P95"}}
	s := New(Deps{Activity: fa, Goals: goalRepoWith(fg)})
	if err := s.AddGoalComment(context.Background(), domain.TenantScope{TenantID: 1}, 7, "blocker", 5); err != nil {
		t.Fatalf("AddGoalComment: %v", err)
	}
	if len(fa.recorded) != 1 {
		t.Fatalf("want 1 event, got %d", len(fa.recorded))
	}
	ev := fa.recorded[0]
	if ev.Category != domain.ActivityDiscussion || ev.Action != domain.ActionCommentAdded {
		t.Fatalf("wrong event: %+v", ev)
	}
	if ev.TeamID == nil || *ev.TeamID != 42 || ev.EntityTitle != "P95" || ev.Payload["text"] != "blocker" {
		t.Fatalf("event fields wrong: %+v", ev)
	}
}
```
Add a small adapter so the fake satisfies the full `GoalRepo` interface (only the 3 methods above are exercised; the rest panic if called):
```go
type goalRepoAdapter struct {
	*fakeGoalRepoDiscussion
	GoalRepo // embedded nil interface → any un-overridden method panics if called
}

func goalRepoWith(f *fakeGoalRepoDiscussion) GoalRepo { return &goalRepoAdapter{fakeGoalRepoDiscussion: f} }
```
> `goalRepoAdapter` promotes the 3 real methods from `fakeGoalRepoDiscussion` and falls back to the embedded (nil) `GoalRepo` for the rest — which are never called in this test. This keeps the fake tiny.

- [ ] **Step 5: Run** — `go test ./internal/service/ -run 'Comment' -v` → PASS.

- [ ] **Step 6: Checkpoint** — `go build ./... && go test ./internal/service/ ./internal/store/goals/ ./internal/http/handlers/api/v1/goals/`. Do not commit.

---

### Task B3: Instrument status changes (thread actor)

**Files:**
- Modify: `internal/service/service.go` (`UpdateTeamPeriodStatus` gains `actorUserID`)
- Modify: the handler that calls it (compiler-driven) + tests

**Interfaces:**
- `Service.UpdateTeamPeriodStatus(ctx, scope, teamID, periodID int64, status domain.TeamPeriodStatus, actorUserID int64) error` — records a `status` event with before/after status and the team name as `entity_title`.

- [ ] **Step 1: Rewrite the method** in `service.go`:
```go
func (s *Service) UpdateTeamPeriodStatus(ctx context.Context, scope domain.TenantScope, teamID, periodID int64, status domain.TeamPeriodStatus, actorUserID int64) error {
	before, _ := s.statuses.GetTeamPeriodStatus(ctx, scope, teamID, periodID)
	if err := s.statuses.SetTeamPeriodStatus(ctx, scope, teamID, periodID, status); err != nil {
		return err
	}
	title := ""
	if team, terr := s.teams.GetTeam(ctx, scope, teamID); terr == nil {
		title = team.Name
	}
	tID, pID := teamID, periodID
	s.recordActivity(ctx, scope, domain.ActivityEvent{
		ActorUserID: actorUserID, Category: domain.ActivityStatus, Action: domain.ActionStatusChanged,
		TeamID: &tID, PeriodID: &pID, EntityTitle: title,
		Payload: map[string]any{"before": map[string]any{"status": string(before)}, "after": map[string]any{"status": string(status)}},
	})
	return nil
}
```

- [ ] **Step 2: Fix the caller** — Run `go build ./...`. The single handler caller (grep: `rg -n "UpdateTeamPeriodStatus" internal/http`) must pass `auth.UserIDFromContext(r.Context())` as the new last arg. Update it and any test callers (pass `1`). Re-run until clean.

- [ ] **Step 3: Write test** — append to `internal/service/activity_test.go` a fake `TeamStatusRepo` + `TeamRepo` and assert the event. Follow the B2 adapter pattern:
```go
type fakeStatusRepo struct{ before domain.TeamPeriodStatus }

func (f *fakeStatusRepo) GetTeamPeriodStatus(context.Context, domain.TenantScope, int64, int64) (domain.TeamPeriodStatus, error) {
	return f.before, nil
}
func (f *fakeStatusRepo) SetTeamPeriodStatus(context.Context, domain.TenantScope, int64, int64, domain.TeamPeriodStatus) error {
	return nil
}
func (f *fakeStatusRepo) GetTeamPeriodStatusWithTime(context.Context, domain.TenantScope, int64, int64) (domain.TeamPeriodStatus, *time.Time, error) {
	return f.before, nil, nil
}
func (f *fakeStatusRepo) ListTeamPeriodStatuses(context.Context, domain.TenantScope, int64, []int64) (map[int64]domain.TeamPeriodStatus, error) {
	return nil, nil
}

type fakeTeamRepoName struct{ name string }

func (f *fakeTeamRepoName) GetTeam(context.Context, domain.TenantScope, int64) (domain.Team, error) {
	return domain.Team{Name: f.name}, nil
}

type teamRepoAdapter struct {
	*fakeTeamRepoName
	TeamRepo
}

func TestUpdateStatusRecordsEvent(t *testing.T) {
	fa := &fakeActivityRepo{}
	s := New(Deps{
		Activity: fa,
		Statuses: &fakeStatusRepo{before: domain.TeamPeriodStatusForming},
		Teams:    &teamRepoAdapter{fakeTeamRepoName: &fakeTeamRepoName{name: "PaaS / Infra"}},
	})
	if err := s.UpdateTeamPeriodStatus(context.Background(), domain.TenantScope{TenantID: 1}, 10, 3, domain.TeamPeriodStatusInProgress, 5); err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(fa.recorded) != 1 {
		t.Fatalf("want 1 event, got %d", len(fa.recorded))
	}
	ev := fa.recorded[0]
	if ev.Action != domain.ActionStatusChanged || ev.EntityTitle != "PaaS / Infra" {
		t.Fatalf("wrong event: %+v", ev)
	}
	if ev.Payload["after"].(map[string]any)["status"] != string(domain.TeamPeriodStatusInProgress) {
		t.Fatalf("after status wrong: %+v", ev.Payload)
	}
}
```

- [ ] **Step 4: Run** — `go test ./internal/service/ -run TestUpdateStatus -v` → PASS. Then `go build ./... && go test ./internal/http/...`.

- [ ] **Step 5: Checkpoint** — full `go test ./...` (status callers green). Do not commit.

---

### Task B4: Instrument KR progress (thread actor)

**Files:**
- Modify: `internal/service/service.go` (`UpdateKRProgressNumerical/Boolean/Project` gain `actorUserID`)
- Modify: their handler callers (compiler-driven) + tests

**Interfaces:**
- Each `UpdateKRProgress*` gains a trailing `actorUserID int64` and records a `progress` event with before/after `Progress` (%), goal team/period, and KR title.

- [ ] **Step 1: Add a shared recording helper** in `service.go`:
```go
// recordKRProgress records a progress event from the KR's before/after computed Progress.
func (s *Service) recordKRProgress(ctx context.Context, scope domain.TenantScope, krID int64, before domain.KeyResult, actorUserID int64) {
	after, err := s.krs.GetKeyResult(ctx, scope, krID)
	if err != nil {
		return
	}
	g, gerr := s.goals.GetGoal(ctx, scope, after.GoalID)
	if gerr != nil {
		return
	}
	teamID, periodID, goalID, kr := g.TeamID, g.PeriodID, g.ID, krID
	s.recordActivity(ctx, scope, domain.ActivityEvent{
		ActorUserID: actorUserID, Category: domain.ActivityProgress, Action: domain.ActionKRProgress,
		TeamID: &teamID, PeriodID: &periodID, GoalID: &goalID, KRID: &kr, EntityTitle: after.Title,
		Payload: map[string]any{
			"before": map[string]any{"progress": before.Progress},
			"after":  map[string]any{"progress": after.Progress},
			"kind":   string(after.Kind),
		},
	})
}
```

- [ ] **Step 2: Thread actor + record in each of the three methods.** Numerical:
```go
func (s *Service) UpdateKRProgressNumerical(ctx context.Context, scope domain.TenantScope, krID int64, current float64, actorUserID int64) error {
	kr, err := s.krs.GetKeyResult(ctx, scope, krID)
	if err != nil {
		return err
	}
	if kr.Kind != domain.KRKindNumerical {
		return fmt.Errorf("unsupported kr kind for numerical update: %s", kr.Kind)
	}
	if err := s.krs.UpdateNumericalCurrent(ctx, scope, krID, current); err != nil {
		return err
	}
	s.recordKRProgress(ctx, scope, krID, kr, actorUserID)
	return nil
}
```
Boolean and Project: apply the same shape — load `kr` before the mutation (they already do, or add `kr, _ := s.krs.GetKeyResult(ctx, scope, krID)` before the update call), keep the existing mutation call, then `s.recordKRProgress(ctx, scope, krID, kr, actorUserID)` before `return nil`. (Open `service.go:710` and `:726` and mirror the numerical structure exactly.)

- [ ] **Step 3: Fix callers** — Run `go build ./...`. Update the KR handler callers (grep: `rg -n "UpdateKRProgress" internal/http`) to pass `auth.UserIDFromContext(r.Context())`; update test callers to pass `1`. Re-run until clean.

- [ ] **Step 4: Write test** — append a store-backed test in `internal/store/activity/` is overkill; instead extend `internal/service/activity_test.go` with a fake KRRepo returning a KR whose `Progress` differs before/after. Since `recordKRProgress` re-reads the KR, model that with a fake that returns a higher progress on the second `GetKeyResult` call:
```go
type fakeKRRepoProgress struct{ calls int }

func (f *fakeKRRepoProgress) GetKeyResult(context.Context, domain.TenantScope, int64) (domain.KeyResult, error) {
	f.calls++
	if f.calls == 1 {
		return domain.KeyResult{ID: 55, GoalID: 7, Title: "P95 latency", Kind: domain.KRKindNumerical, Progress: 40}, nil
	}
	return domain.KeyResult{ID: 55, GoalID: 7, Title: "P95 latency", Kind: domain.KRKindNumerical, Progress: 61}, nil
}
func (f *fakeKRRepoProgress) UpdateNumericalCurrent(context.Context, domain.TenantScope, int64, float64) error {
	return nil
}

type krRepoAdapter struct {
	*fakeKRRepoProgress
	KRRepo
}
type goalRepoGet struct{ goal domain.Goal }

func (g *goalRepoGet) GetGoal(context.Context, domain.TenantScope, int64) (domain.Goal, error) {
	return g.goal, nil
}

type goalRepoGetAdapter struct {
	*goalRepoGet
	GoalRepo
}

func TestKRProgressRecordsEvent(t *testing.T) {
	fa := &fakeActivityRepo{}
	s := New(Deps{
		Activity: fa,
		KRs:      &krRepoAdapter{fakeKRRepoProgress: &fakeKRRepoProgress{}},
		Goals:    &goalRepoGetAdapter{goalRepoGet: &goalRepoGet{goal: domain.Goal{ID: 7, TeamID: 42, PeriodID: 3, Title: "P95"}}},
	})
	if err := s.UpdateKRProgressNumerical(context.Background(), domain.TenantScope{TenantID: 1}, 55, 278, 5); err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(fa.recorded) != 1 {
		t.Fatalf("want 1, got %d", len(fa.recorded))
	}
	ev := fa.recorded[0]
	if ev.Category != domain.ActivityProgress || ev.Action != domain.ActionKRProgress {
		t.Fatalf("wrong event: %+v", ev)
	}
	if ev.Payload["before"].(map[string]any)["progress"] != 40 || ev.Payload["after"].(map[string]any)["progress"] != 61 {
		t.Fatalf("before/after wrong: %+v", ev.Payload)
	}
}
```

- [ ] **Step 5: Run** — `go test ./internal/service/ -run TestKRProgress -v` → PASS. Then `go build ./... && go test ./internal/http/...`.

- [ ] **Step 6: Checkpoint** — full `go test ./...`. Do not commit.

---

### Task B5: Instrument composition — create/delete (goals & KRs)

**Files:**
- Modify: `internal/service/service.go` (`CreateGoal`, `DeleteGoal`, `CreateKeyResultWithMeta`, `DeleteKeyResult` gain `actorUserID`)
- Modify: handler callers (compiler-driven) + tests

**Interfaces:**
- Each of the four methods gains a trailing `actorUserID int64` and records a `composition` event (`goal_created`/`goal_deleted`/`kr_created`/`kr_deleted`).

- [ ] **Step 1: `CreateGoal`** — after obtaining the new goal id, load it and record:
```go
func (s *Service) CreateGoal(ctx context.Context, scope domain.TenantScope, input goals.GoalInput, actorUserID int64) (int64, error) {
	id, err := s.goals.CreateGoal(ctx, scope, input) // keep existing body; only add actor + recording
	if err != nil {
		return 0, err
	}
	if g, gerr := s.goals.GetGoal(ctx, scope, id); gerr == nil {
		teamID, periodID, gid := g.TeamID, g.PeriodID, g.ID
		s.recordActivity(ctx, scope, domain.ActivityEvent{
			ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionGoalCreated,
			TeamID: &teamID, PeriodID: &periodID, GoalID: &gid, EntityTitle: g.Title,
		})
	}
	return id, nil
}
```
> Open the current `CreateGoal` (`service.go:1156`) and preserve any existing validation/logic; only add the `actorUserID` param and the recording block after success.

- [ ] **Step 2: `DeleteGoal`** — capture the goal BEFORE deleting (the current method already computes `effectiveTeamID, periodID`; also fetch the title). Add recording after successful delete:
```go
// inside DeleteGoal, before deletion:
g, _ := s.goals.GetGoal(ctx, scope, goalID)
// ... existing delete logic ...
// after success:
title := g.Title
teamID, pID, gid := effectiveTeamID, periodID, goalID
s.recordActivity(ctx, scope, domain.ActivityEvent{
	ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionGoalDeleted,
	TeamID: &teamID, PeriodID: &pID, GoalID: &gid, EntityTitle: title,
})
```
Add `actorUserID int64` to the `DeleteGoal` signature (`service.go:1179`).

- [ ] **Step 3: `CreateKeyResultWithMeta`** — after the KR id is returned, resolve goal for team/period and record `kr_created`:
```go
// after success (id is the new KR id):
if kr, kerr := s.krs.GetKeyResult(ctx, scope, id); kerr == nil {
	if g, gerr := s.goals.GetGoal(ctx, scope, kr.GoalID); gerr == nil {
		teamID, periodID, gid, krid := g.TeamID, g.PeriodID, g.ID, id
		s.recordActivity(ctx, scope, domain.ActivityEvent{
			ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionKRCreated,
			TeamID: &teamID, PeriodID: &periodID, GoalID: &gid, KRID: &krid, EntityTitle: kr.Title,
		})
	}
}
```
Add `actorUserID int64` to the signature (`service.go:837`).

- [ ] **Step 4: `DeleteKeyResult`** — capture the KR + its goal BEFORE delete, record `kr_deleted` after success. Add `actorUserID int64` (`service.go:1140`):
```go
kr, _ := s.krs.GetKeyResult(ctx, scope, id)
var g domain.Goal
if kr.GoalID != 0 {
	g, _ = s.goals.GetGoal(ctx, scope, kr.GoalID)
}
// ... existing delete ...
teamID, periodID, gid, krid := g.TeamID, g.PeriodID, g.ID, id
s.recordActivity(ctx, scope, domain.ActivityEvent{
	ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionKRDeleted,
	TeamID: &teamID, PeriodID: &periodID, GoalID: &gid, KRID: &krid, EntityTitle: kr.Title,
})
```

- [ ] **Step 5: Fix callers** — `go build ./...`; update goal/KR handler callers to pass `auth.UserIDFromContext(r.Context())` and test callers to pass `1`. (grep: `rg -n "CreateGoal|DeleteGoal|CreateKeyResultWithMeta|DeleteKeyResult" internal/http internal/service/*_test.go`.)

- [ ] **Step 6: Write test** — extend `internal/service/activity_test.go`: reuse the `goalRepoGet` fake (return a fixed goal from `GetGoal`) plus a `CreateGoal` that returns an id, assert one `goal_created` event with `EntityTitle` from the fetched goal. Model like `TestKRProgressRecordsEvent`.

- [ ] **Step 7: Checkpoint** — `go build ./... && go test ./...`. Do not commit.

---

### Task B6: Instrument composition — sharing & owner change

**Files:**
- Modify: `internal/service/service.go` (`ShareGoal`, `DeleteGoalShare`, `UpdateGoalOwnerAndShares` gain `actorUserID`)
- Modify: handler callers + tests

**Interfaces:**
- `ShareGoal(..., actorUserID)` records `goal_shared`; `DeleteGoalShare(..., actorUserID)` records `goal_unshared`; `UpdateGoalOwnerAndShares(..., actorUserID)` records `goal_owner_changed`.

- [ ] **Step 1: `ShareGoal`** — after success, load the goal and record with the target team ids:
```go
// after existing share logic succeeds; `targets` is the []ShareTarget arg:
if g, gerr := s.goals.GetGoal(ctx, scope, goalID); gerr == nil {
	teamIDs := make([]int64, 0, len(targets))
	for _, tgt := range targets {
		teamIDs = append(teamIDs, tgt.TeamID)
	}
	teamID, periodID, gid := g.TeamID, g.PeriodID, g.ID
	s.recordActivity(ctx, scope, domain.ActivityEvent{
		ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionGoalShared,
		TeamID: &teamID, PeriodID: &periodID, GoalID: &gid, EntityTitle: g.Title,
		Payload: map[string]any{"shared_with_team_ids": teamIDs},
	})
}
```
Add `actorUserID int64` to `ShareGoal` (`service.go:756`). (Adjust `tgt.TeamID` to the actual field name on `ShareTarget`; open the type to confirm.)

- [ ] **Step 2: `DeleteGoalShare`** — record `goal_unshared` with the removed team id; capture goal before removal for team/period/title. Add `actorUserID int64` (`service.go:1130`):
```go
g, _ := s.goals.GetGoal(ctx, scope, goalID)
// ... existing delete-share logic ...
teamID, periodID, gid, removed := g.TeamID, g.PeriodID, g.ID, teamID2 // teamID2 = the share-target arg
s.recordActivity(ctx, scope, domain.ActivityEvent{
	ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionGoalUnshared,
	TeamID: &teamID, PeriodID: &periodID, GoalID: &gid, EntityTitle: g.Title,
	Payload: map[string]any{"unshared_team_id": removed},
})
```
> The current signature is `DeleteGoalShare(ctx, scope, goalID, teamID int64)`. Rename the second team param inside the body to avoid clashing with the owner `teamID` (e.g. call the share-target `shareTeamID`), and use it in the payload.

- [ ] **Step 3: `UpdateGoalOwnerAndShares`** — record `goal_owner_changed`; the method already returns `(ownerID, periodID, err)`. Capture old owner before, record after. Add `actorUserID int64` (`service.go:1237`):
```go
oldGoal, _ := s.goals.GetGoal(ctx, scope, goalID)
// ... existing logic that computes ownerID, periodID ...
// after success:
gid := goalID
oldOwner := oldGoal.TeamID
s.recordActivity(ctx, scope, domain.ActivityEvent{
	ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionGoalOwnerChanged,
	TeamID: &ownerID, PeriodID: &periodID, GoalID: &gid, EntityTitle: oldGoal.Title,
	Payload: map[string]any{"before": map[string]any{"owner_team_id": oldOwner}, "after": map[string]any{"owner_team_id": ownerID}},
})
```

- [ ] **Step 4: Fix callers + test** — `go build ./...`; thread `auth.UserIDFromContext` / `1`. Add one service test asserting `ShareGoal` records `goal_shared` with the target ids (reuse `fakeActivityRepo` + `goalRepoGet`).

- [ ] **Step 5: Checkpoint** — `go build ./... && go test ./...`. Do not commit.

---

### Task B7: Instrument composition — field edits (before→after)

**Files:**
- Modify: `internal/service/service.go` (`UpdateGoal`/`UpdateGoalFields`, `UpdateKeyResultWithMeta`/`UpdateKeyResultDescription` gain `actorUserID`)
- Modify: handler callers + tests

**Interfaces:**
- Goal edits record `goal_fields_changed`; KR edits record `kr_fields_changed`. Payload carries a `changed` map of `{field: {before, after}}` for the fields that actually changed.

- [ ] **Step 1: Add a diff helper** in `service.go`:
```go
// diffFields returns only the entries whose before != after, as {field: {"before":x,"after":y}}.
func diffFields(pairs map[string][2]any) map[string]any {
	out := map[string]any{}
	for field, ba := range pairs {
		if ba[0] != ba[1] {
			out[field] = map[string]any{"before": ba[0], "after": ba[1]}
		}
	}
	return out
}
```

- [ ] **Step 2: `UpdateGoal` / `UpdateGoalFields`** — load the goal before, apply, load after (or use the input), record if any field changed. Example for `UpdateGoalFields` (`service.go:1114`), add `actorUserID int64`:
```go
before, _ := s.goals.GetGoal(ctx, scope, input.GoalID) // adjust to the input's id field
if err := s.goals.UpdateGoalFields(ctx, scope, input); err != nil {
	return err
}
after, aerr := s.goals.GetGoal(ctx, scope, input.GoalID)
if aerr == nil {
	changed := diffFields(map[string][2]any{
		"title":       {before.Title, after.Title},
		"description": {before.Description, after.Description},
		"weight":      {before.Weight, after.Weight},
		"priority":    {string(before.Priority), string(after.Priority)},
	})
	if len(changed) > 0 {
		teamID, periodID, gid := after.TeamID, after.PeriodID, after.ID
		s.recordActivity(ctx, scope, domain.ActivityEvent{
			ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionGoalFieldsChanged,
			TeamID: &teamID, PeriodID: &periodID, GoalID: &gid, EntityTitle: after.Title,
			Payload: map[string]any{"changed": changed},
		})
	}
}
return nil
```
Apply the same shape to `UpdateGoal` (`service.go:825`). (Confirm the id field name on `goals.GoalFieldsUpdateInput` / `goals.GoalUpdateInput` — likely `GoalID` or `ID`.)

- [ ] **Step 3: `UpdateKeyResultWithMeta` / `UpdateKeyResultDescription`** — same pattern keyed on the KR. Load KR before + its goal (for team/period), apply, diff `title`/`description`/`weight`, record `kr_fields_changed`. Add `actorUserID int64` to both (`service.go:848`, `:798`).

- [ ] **Step 4: Fix callers + test** — `go build ./...`; thread actor. Add a service test: edit a goal's title via `UpdateGoalFields`, assert one `goal_fields_changed` event whose `changed.title.after` matches; and a no-op edit (same values) records **nothing**.

- [ ] **Step 5: Checkpoint** — `go build ./... && go test ./...`. Do not commit.

---

# Phase C — Read API

### Task C1: `GET /api/v1/activity` — DTO, handler, routes, registration

**Files:**
- Create: `internal/http/dto/activity.go`
- Create: `internal/http/handlers/api/v1/activity/handler.go`, `response.go`, `routes.go`, `routes_test.go`
- Modify: `internal/http/server.go` (register in `registerApiRoutes`)

**Interfaces:**
- Consumes `service.ListActivity`, `auth.AllowedTeamIDsFromCtx`, `auth.TenantScopeFromContext`, `activity.ListFilter`.
- Produces route `GET /api/v1/activity` returning `dto.ActivityFeedResponse`.

- [ ] **Step 1: DTOs** — `internal/http/dto/activity.go`:
```go
package dto

type ActivityActor struct {
	UDID        string `json:"udid,omitempty"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	Removed     bool   `json:"removed"`
}

type ActivityTarget struct {
	Section   string `json:"section"`
	TeamID    int64  `json:"team_id"`
	PeriodID  *int64 `json:"period_id,omitempty"`
	GoalID    *int64 `json:"goal_id,omitempty"`
	KRID      *int64 `json:"kr_id,omitempty"`
	CommentID *int64 `json:"comment_id,omitempty"`
}

type ActivityEvent struct {
	ID          int64          `json:"id"`
	Category    string         `json:"category"`
	Action      string         `json:"action"`
	Actor       ActivityActor  `json:"actor"`
	TeamID      *int64         `json:"team_id,omitempty"`
	PeriodID    *int64         `json:"period_id,omitempty"`
	GoalID      *int64         `json:"goal_id,omitempty"`
	KRID        *int64         `json:"kr_id,omitempty"`
	CommentID   *int64         `json:"comment_id,omitempty"`
	EntityTitle string         `json:"entity_title"`
	Target      *ActivityTarget `json:"target,omitempty"`
	Payload     map[string]any `json:"payload,omitempty"`
	CreatedAt   string         `json:"created_at"`
}

type ActivityFeedResponse struct {
	Items      []ActivityEvent `json:"items"`
	NextCursor string          `json:"next_cursor,omitempty"`
}
```

- [ ] **Step 2: Response mapper** — `internal/http/handlers/api/v1/activity/response.go`:
```go
package activity

import (
	"encoding/base64"
	"strconv"
	"strings"
	"time"

	"okrs/internal/domain"
	"okrs/internal/http/dto"
	storeactivity "okrs/internal/store/activity"
)

func encodeCursor(c *storeactivity.Cursor) string {
	if c == nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(c.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + strconv.FormatInt(c.ID, 10)))
}

func decodeCursor(s string) *storeactivity.Cursor {
	if s == "" {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return nil
	}
	ts, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return nil
	}
	return &storeactivity.Cursor{CreatedAt: ts, ID: id}
}

// buildTarget resolves navigation: for now every target routes to the tracker board of the
// event's team. (Owner-current resolution is a Plan-2/refinement; here we use the recorded team.)
func buildTarget(ev domain.ActivityEvent) *dto.ActivityTarget {
	if ev.TeamID == nil {
		return nil
	}
	return &dto.ActivityTarget{
		Section: "tracker", TeamID: *ev.TeamID, PeriodID: ev.PeriodID,
		GoalID: ev.GoalID, KRID: ev.KRID, CommentID: ev.CommentID,
	}
}

func newFeedResponse(events []domain.ActivityEvent, next *storeactivity.Cursor) dto.ActivityFeedResponse {
	items := make([]dto.ActivityEvent, 0, len(events))
	for _, ev := range events {
		items = append(items, dto.ActivityEvent{
			ID: ev.ID, Category: string(ev.Category), Action: string(ev.Action),
			Actor: dto.ActivityActor{UDID: ev.ActorUDID, DisplayName: ev.ActorDisplayName, AvatarURL: ev.ActorAvatarURL, Removed: ev.ActorRemoved},
			TeamID: ev.TeamID, PeriodID: ev.PeriodID, GoalID: ev.GoalID, KRID: ev.KRID, CommentID: ev.CommentID,
			EntityTitle: ev.EntityTitle, Target: buildTarget(ev), Payload: ev.Payload,
			CreatedAt: ev.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return dto.ActivityFeedResponse{Items: items, NextCursor: encodeCursor(next)}
}
```
> When Plan 2 wants the removed-actor label rendered, the frontend shows «Бывший участник» for `removed: true`; the API deliberately keeps `display_name` empty so no former-member PII crosses the wire.

- [ ] **Step 3: Handler** — `internal/http/handlers/api/v1/activity/handler.go`:
```go
package activity

import (
	"net/http"
	"strconv"
	"time"

	"okrs/internal/auth"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/service"
	storeactivity "okrs/internal/store/activity"
)

type Handler struct {
	service *service.Service
}

func New(service *service.Service) *Handler { return &Handler{service: service} }

// scopeTeams returns (allowedTeamIDs, restricted). nil = admin/unrestricted.
func scopeTeams(r *http.Request) []int64 {
	allowed, ok := auth.AllowedTeamIDsFromCtx(r.Context())
	if ok && allowed != nil {
		return allowed // may be empty => no access => fail-closed
	}
	return nil // not loaded (disabled mode) or admin => unrestricted
}

func parseInt64(s string) *int64 {
	if s == "" {
		return nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil
	}
	return &v
}

func sinceFromRange(rng string) *time.Time {
	now := time.Now()
	switch rng {
	case "today":
		t := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		return &t
	case "7d":
		t := now.Add(-7 * 24 * time.Hour)
		return &t
	case "30d":
		t := now.Add(-30 * 24 * time.Hour)
		return &t
	default: // "all" or empty
		return nil
	}
}

func (h *Handler) HandleFeed(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	q := r.URL.Query()
	var teamIDs []int64
	for _, s := range q["team_ids"] {
		if p := parseInt64(s); p != nil {
			teamIDs = append(teamIDs, *p)
		}
	}
	limit := 50
	if p := parseInt64(q.Get("limit")); p != nil {
		limit = int(*p)
	}
	f := storeactivity.ListFilter{
		PeriodID:  parseInt64(q.Get("period_id")),
		TeamIDs:   teamIDs,
		Category:  q.Get("category"),
		ActorUDID: q.Get("actor_udid"),
		Since:     sinceFromRange(q.Get("range")),
		Query:     q.Get("q"),
		Limit:     limit,
		Cursor:    decodeCursor(q.Get("cursor")),
	}
	events, next, err := h.service.ListActivity(r.Context(), scope, scopeTeams(r), f)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to list activity", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, newFeedResponse(events, next))
}
```

- [ ] **Step 4: Routes + registration** — `internal/http/handlers/api/v1/activity/routes.go`:
```go
package activity

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/api/v1/activity", h.HandleFeed)
	r.Get("/api/v1/activity/tree-counts", h.HandleTreeCounts) // added in Task C2
}
```
In `internal/http/server.go` `registerApiRoutes`, add the aliased import `apiactivity "okrs/internal/http/handlers/api/v1/activity"` and the line:
```go
	apiactivity.RegisterRoutes(r, apiactivity.New(s.service))
```
> `routes.go` references `HandleTreeCounts` which is added in C2; implement C1 and C2 together, or temporarily comment the second route line until C2.

- [ ] **Step 5: Routes test** — `internal/http/handlers/api/v1/activity/routes_test.go` mirroring the pattern in `goals/routes_test.go` (open it and copy the shape): assert `RegisterRoutes` registers `GET /api/v1/activity` on a `chi.NewRouter()`. Run: `go test ./internal/http/handlers/api/v1/activity/ -v`.

- [ ] **Step 6: Checkpoint** — `go build ./...`; the full integration router test (`internal/http`) still green. Do not commit.

---

### Task C2: `GET /api/v1/activity/tree-counts`

**Files:**
- Modify: `internal/http/handlers/api/v1/activity/handler.go`

**Interfaces:**
- Produces `GET /api/v1/activity/tree-counts?period_id=&range=` → `{ "counts": { "<team_id>": <int> } }`.

- [ ] **Step 1: Add handler** — append to `handler.go`:
```go
func (h *Handler) HandleTreeCounts(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	q := r.URL.Query()
	counts, err := h.service.ActivityTreeCounts(r.Context(), scope, scopeTeams(r), parseInt64(q.Get("period_id")), sinceFromRange(q.Get("range")))
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to count activity", nil)
		return
	}
	out := make(map[string]int, len(counts))
	for teamID, n := range counts {
		out[strconv.FormatInt(teamID, 10)] = n
	}
	v1.WriteJSON(w, http.StatusOK, map[string]any{"counts": out})
}
```

- [ ] **Step 2: Ensure route enabled** — the second line in `routes.go` (C1 Step 4) now compiles. Run `go build ./...`.

- [ ] **Step 3: Handler-level integration test** — add `internal/http/handlers/api/v1/activity/handler_test.go` that boots the real router the way `goals`/`hierarhy` handler tests do (open one for the exact `testutil`/router setup), seeds a couple of events (via `store.Activity.Record`), calls `GET /api/v1/activity` and `.../tree-counts`, and asserts the JSON shape + scope filtering. Run: `go test ./internal/http/handlers/api/v1/activity/ -v`.

- [ ] **Step 4: Checkpoint** — `go build ./... && go test ./internal/http/...`. Do not commit.

---

# Phase D — Purge API

### Task D1: `POST /api/v1/admin/activity/purge` (tenant-admin)

**Files:**
- Modify: `internal/http/handlers/api/v1/admin/handler.go`
- Modify: `internal/http/server.go` (`registerAdminRoutes` + `apiadmin.New` wiring)

**Interfaces:**
- Consumes `service.PurgeActivity`. Produces `POST /api/v1/admin/activity/purge` body `{"older_than":"quarter"|"year"|"all"}` → `200 {"deleted":N}`.

- [ ] **Step 1: Add a cutoff helper (shared)** — `internal/http/handlers/api/v1/activity/handler.go` already has `sinceFromRange`. For purge depth add a small shared function in the admin handler file (or duplicate — tiny):
```go
// cutoffFor maps the purge depth to a cutoff time; ok=false for unknown depth.
// "all" returns (zero, true) meaning "delete everything" (caller passes nil to the service).
func purgeCutoff(depth string) (t *time.Time, ok bool) {
	now := time.Now()
	switch depth {
	case "quarter":
		c := now.AddDate(0, -3, 0)
		return &c, true
	case "year":
		c := now.AddDate(0, -12, 0)
		return &c, true
	case "all":
		return nil, true
	default:
		return nil, false
	}
}
```

- [ ] **Step 2: Handler** — add to `admin/handler.go` (uses the existing local `writeError`/`writeJSON`):
```go
// POST /api/v1/admin/activity/purge  body: {"older_than":"quarter"|"year"|"all"}
func (h *Handler) HandlePurgeActivity(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OlderThan string `json:"older_than"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	cutoff, ok := purgeCutoff(body.OlderThan)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "invalid older_than")
		return
	}
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusForbidden, "no active tenant")
		return
	}
	deleted, err := h.activity.PurgeActivity(r.Context(), scope, cutoff)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted})
}
```
Add a small dependency interface + field on the admin `Handler` struct (mirror the existing `tenantRenamer` seam):
```go
type activityPurger interface {
	PurgeActivity(ctx context.Context, scope domain.TenantScope, olderThan *time.Time) (int64, error)
}
```
Add `activity activityPurger` to the `Handler` struct and a constructor param. `*service.Service` satisfies `activityPurger` (from B1). Then in `server.go:415` update the `apiadmin.New(...)` call to pass `s.service` for the new param (compiler will flag the arity change; add `s.service`). Add imports `context`, `time`, `okrs/internal/domain`, `okrs/internal/auth` if not present.

- [ ] **Step 3: Register route** — in `registerAdminRoutes` (after the settings block, ~server.go:452):
```go
	r.Post("/api/v1/admin/activity/purge", adminAPI.HandlePurgeActivity)
```

- [ ] **Step 4: Test** — add an admin handler test (open `internal/http/handlers/api/v1/admin/*_test.go` for the setup) that seeds events, POSTs `{"older_than":"all"}`, asserts `200 {"deleted":N}` and that events are gone; and that `{"older_than":"nope"}` → `422`. Also assert a non-admin membership is rejected `403` (the middleware handles it in the full-router test).

- [ ] **Step 5: Checkpoint** — `go build ./... && go test ./internal/http/...`. Do not commit.

---

### Task D2: `POST /api/v1/system/tenants/{id}/activity/purge` (system-admin)

**Files:**
- Modify: `internal/service/provisioning.go` (`PurgeActivityForTenant`)
- Modify: `internal/http/handlers/api/v1/system/handler.go` (`Provisioner` interface + `HandlePurgeActivity`)
- Modify: `internal/http/server.go` (`registerSystemRoutes`)

**Interfaces:**
- `ProvisioningService.PurgeActivityForTenant(ctx, tenantID int64, olderThan *time.Time) (int64, error)`.
- Produces `POST /api/v1/system/tenants/{id}/activity/purge` body `{"older_than":...}` → `200 {"deleted":N}`.

- [ ] **Step 1: Provisioning method** — the service needs the activity repo. Add an `activityPurger` field to `ProvisioningService` (interface with `Purge(ctx, scope, *time.Time) (int64, error)`), wired from `st.Activity` at construction (`server.go` `NewProvisioningService(...)` call). Then:
```go
func (p *ProvisioningService) PurgeActivityForTenant(ctx context.Context, tenantID int64, olderThan *time.Time) (int64, error) {
	return p.activity.Purge(ctx, domain.TenantScope{TenantID: tenantID}, olderThan)
}
```
> Confirm the `NewProvisioningService(...)` call site in `server.go` and add `s.store.Activity` as the new arg. `*store.ActivityRepository` satisfies the tiny `activityPurger` interface directly.

- [ ] **Step 2: System handler** — add to the `Provisioner` interface in `system/handler.go`:
```go
	PurgeActivityForTenant(ctx context.Context, tenantID int64, olderThan *time.Time) (int64, error)
```
and the handler (reuse `pathID` + the same `purgeCutoff` logic — duplicate the tiny helper into this file or move it to a shared spot):
```go
// POST /api/v1/system/tenants/{id}/activity/purge  {"older_than":"quarter"|"year"|"all"}
func (h *Handler) HandlePurgeActivity(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		OlderThan string `json:"older_than"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	cutoff, ok := purgeCutoff(body.OlderThan)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "invalid older_than")
		return
	}
	deleted, err := h.prov.PurgeActivityForTenant(r.Context(), tenantID, cutoff)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted})
}
```

- [ ] **Step 3: Register route** — in `registerSystemRoutes` (~server.go:520, near suspend/restore):
```go
	r.Post("/api/v1/system/tenants/{id}/activity/purge", sysH.HandlePurgeActivity)
```

- [ ] **Step 4: Test** — add a system handler test: seed events in tenant 1, POST `.../tenants/1/activity/purge` `{"older_than":"all"}`, assert `200 {"deleted":N}` and tenant-2 events untouched; unknown depth → `422`.

- [ ] **Step 5: Checkpoint** — `go build ./... && go test ./...`. Do not commit.

---

# Phase E — Seed + specs

### Task E1: Demo activity in `seed_demo.sql`

**Files:**
- Modify: `seed_demo.sql`

- [ ] **Step 1: Append demo events** — after the existing goals/KR/comment seed blocks, add `INSERT INTO activity_events (...)` rows that mirror the mockup (progress, composition, status, discussion) across a few demo teams/periods/goals, using the seeded `anonymous-local` actor (id 1) or other seeded demo users. Reference existing goal/kr/team/period ids already inserted earlier in the file (grep the file for the demo goal ids). Use explicit `created_at` values spread across today/yesterday/this-week so the frontend time-grouping shows sections. Example row:
```sql
INSERT INTO activity_events (tenant_id, actor_user_id, category, action, team_id, period_id, goal_id, kr_id, entity_title, payload_json, created_at) VALUES
  (1, 1, 'progress', 'kr_progress', <teamId>, <periodId>, <goalId>, <krId>, 'P95 latency API gateway',
   '{"before":{"progress":40},"after":{"progress":61}}', now() - interval '2 hours');
```

- [ ] **Step 2: Verify seed loads** — reload the demo DB per the repo's seed procedure (grep README for the seed command) and confirm no FK/constraint errors. Then `SELECT count(*) FROM activity_events;` returns the expected number.

- [ ] **Step 3: Checkpoint** — seed applies cleanly. Do not commit.

### Task E2: Update specs

**Files:**
- Modify: `specs/020-domain-model.md`, `specs/040-api-contract.md`, `specs/050-permissions-and-lifecycle.md`

- [ ] **Step 1: `020-domain-model.md`** — add an `### ActivityEvent` section (fields from Task A1, invariants: append-only; best-effort recording; actor resolved tenant-scoped with removed→placeholder-without-PII; `team_id`/`period_id` = context; goal/kr/comment ids are non-FK references).

- [ ] **Step 2: `040-api-contract.md`** — document `GET /api/v1/activity` (query params, `ActivityFeedResponse` shape, cursor), `GET /api/v1/activity/tree-counts`, `POST /api/v1/admin/activity/purge`, `POST /api/v1/system/tenants/{id}/activity/purge` (bodies, auth level, `422` on bad `older_than`).

- [ ] **Step 3: `050-permissions-and-lifecycle.md`** — document share-aware visibility (audience = owner ∪ sharees) within tenant+team scope, fail-closed on `team_id IS NULL`, removed-member actor handling, and who may purge (tenant-admin own space; system-admin any tenant).

- [ ] **Step 4: Checkpoint** — `go test ./...` still green (specs are docs; no code impact). Do not commit.

---

## Self-Review (completed)

- **Spec coverage:** recording (A1–A4 store, B1–B7 service) ✓; read API `GET /api/v1/activity` (C1) + tree-counts (C2) ✓; share-aware visibility + fail-closed + removed-actor PII masking (A2 `List`) ✓; purge admin (D1) + system (D2) ✓; seed (E1) ✓; specs 020/040/050 (E2) ✓. Frontend (page, deep-link, purge UI) + spec `030-user-flows.md` are **Plan 2** (out of scope here, called out).
- **Types consistent:** `ActivityEvent`/`ActivityCategory`/`ActivityAction` defined in A1 and used verbatim in A2–E; `ListFilter`/`Cursor` defined in A2 and consumed by B1/C1; `PurgeActivity`/`Purge`/`PurgeActivityForTenant` names consistent across B1/D1/D2.
- **Placeholders:** the A1 `var _` import stubs are explicitly flagged for deletion in A2; the B5–B7 "open the current method and preserve existing logic" notes point at exact line numbers and give the complete recording block to add — no behavioral placeholders.
- **Commit steps:** replaced with Checkpoints per CLAUDE.md rule 8.
