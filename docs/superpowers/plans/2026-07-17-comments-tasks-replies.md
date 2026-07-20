# Comments: Tasks with Replies — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Превратить плоские комментарии к цели в двухуровневый механизм «таска → ответы»: на таску (замечание) можно отвечать, ответы не резолвятся, свои таски/ответы можно удалять (каскадом), счётчик считает только таски, события ложатся в журнал под фильтром `discussion` с разными описаниями.

**Architecture:** Самоссылающийся `parent_id` в таблице `goal_comments` (`ON DELETE CASCADE`). Таска — строка с `parent_id IS NULL`, ответ — строка с `parent_id` на таску (глубина ровно 1). Ответы отдаются вложенными в таски (`comments[].replies[]`), поэтому счётчик = число тасок напрямую. Двухуровневый контроль доступа (tenant → команда → привязка сущности → авторство/роль) во всех comment-эндпоинтах.

**Tech Stack:** Go (chi, pgx v5), PostgreSQL, golang-migrate; фронтенд — React (UMD) в `internal/web/static/tracker.js` и `activity.js`.

## Global Constraints

- Спеки — source of truth: `specs/020-domain-model.md`, `040-api-contract.md`, `050-permissions-and-lifecycle.md`. Изменения кода и спек — в одном change set (CLAUDE.md).
- **Git-коммиты не делать** — пользователь коммитит сам (CLAUDE.md #8). Каждый таск завершается прогоном тестов/сборки, а не `git commit`.
- Дизайн-док: `docs/superpowers/specs/2026-07-17-comments-tasks-replies-design.md`.
- Поиск по репозиторию — `rg`. Семантические проверки — `go build ./...`, `go vet ./...`, `go test ./...`.
- Тесты стора/хендлеров требуют Docker (testcontainers); при недоступности они `t.Skip` — это ок.
- Не нарушать слои: store не знает про HTTP; авторизация delete (автор|admin) — в service/handler, не в SQL.
- Автор в тестах: системный `anonymous-local` (id=1); `migration` (id=2) — второй пользователь для проверки чужого авторства.
- Действия журнала — сырые строки на чтении (`string(ev.Action)`), backend-описания не нужны; тексты описаний — только во фронте (`activity.js`).

---

## File map

- **Create** `migrations/040_comment_replies.up.sql`, `migrations/040_comment_replies.down.sql` — колонка `parent_id` + индекс.
- **Modify** `internal/domain/models.go` — `GoalComment.ParentID`, `GoalComment.Replies`; новые `ActivityAction` константы.
- **Modify** `internal/store/goals/goals.go` — scan `parent_id`, вложение ответов, сортировка ASC, `AddGoalReply`, `GetGoalCommentMeta`, `DeleteGoalComment`, resolve только для таски.
- **Modify** `internal/store/goals/goals_comments_test.go` — тесты стора.
- **Modify** `internal/service/service.go` — `AddGoalReply`, `DeleteGoalComment`, `ErrForbidden`, обновление интерфейса `goalStore`.
- **Modify** `internal/service/goal_test.go` — тесты сервиса (события, авторизация, каскад).
- **Modify** `internal/http/dto/goal.go` — `GoalReply`, `GoalComment.Replies`.
- **Modify** `internal/http/handlers/api/v1/helpers_response.go` — вложение ответов в `MapGoalComment`.
- **Modify** `internal/http/handlers/api/v1/goals/handler.go`, `routes.go` — reply + delete хендлеры, resolve task-only.
- **Modify** `internal/http/handlers/api/v1/goals/resolve_test.go` (или новый `replies_test.go`) — тесты хендлеров.
- **Modify** `internal/web/static/tracker.js` — `ReplyRow`, reply-compose, кнопки удаления, маппинг `replies`, счётчик, проброс `isAdmin`.
- **Modify** `internal/web/static/activity.js` — описания `reply_added`/`comment_deleted`/`reply_deleted` + markdown-тело `reply_added`.
- **Modify** `specs/020-domain-model.md`, `specs/040-api-contract.md`, `specs/050-permissions-and-lifecycle.md`.
- **Modify** `seed_demo.sql` — примеры ответов.

---

## Task 1: Migration + domain model + read/nesting

**Files:**
- Create: `migrations/040_comment_replies.up.sql`, `migrations/040_comment_replies.down.sql`
- Modify: `internal/domain/models.go` (GoalComment struct ~L109; ActivityAction consts ~L246-248)
- Modify: `internal/store/goals/goals.go` (scanGoalComment L867, listGoalCommentsBatch L582, ListGoalComments L841)
- Test: `internal/store/goals/goals_comments_test.go`

**Interfaces:**
- Produces:
  - `domain.GoalComment` gains `ParentID *int64` and `Replies []domain.GoalComment`.
  - `domain.ActionReplyAdded`, `domain.ActionCommentDeleted`, `domain.ActionReplyDeleted` (`ActivityAction`).
  - `GoalRepository.ListGoalComments` / `listGoalCommentsBatch` now return **only tasks**, each with `Replies` filled; tasks ordered `created_at ASC`, replies `created_at ASC`.

- [ ] **Step 1: Write the migration files**

`migrations/040_comment_replies.up.sql`:
```sql
ALTER TABLE goal_comments
  ADD COLUMN parent_id BIGINT NULL REFERENCES goal_comments(id) ON DELETE CASCADE;
CREATE INDEX idx_goal_comments_parent ON goal_comments(goal_id, parent_id, created_at);
```

`migrations/040_comment_replies.down.sql`:
```sql
DROP INDEX IF EXISTS idx_goal_comments_parent;
ALTER TABLE goal_comments DROP COLUMN IF EXISTS parent_id;
```

- [ ] **Step 2: Extend the domain model**

In `internal/domain/models.go`, `GoalComment` struct — add two fields:
```go
type GoalComment struct {
	ID             int64
	GoalID         int64
	ParentID       *int64 // nil → task; non-nil → reply to that task
	Text           string
	AuthorName     string
	AuthorUDID     string
	AuthorUserID   int64 // used for delete authorization
	CreatedAt      time.Time
	ResolvedAt     *time.Time
	ResolvedByName string
	ResolvedByUDID string
	Replies        []GoalComment // populated for tasks only
}
```

In the `ActivityAction` const block (after `ActionCommentReopened`), add:
```go
	ActionReplyAdded      ActivityAction = "reply_added"
	ActionCommentDeleted  ActivityAction = "comment_deleted"
	ActionReplyDeleted    ActivityAction = "reply_deleted"
```

- [ ] **Step 3: Write the failing store test for nesting + ordering**

Add to `internal/store/goals/goals_comments_test.go`:
```go
func TestListGoalCommentsNestsRepliesInOrder(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	gr := goals.NewGoalRepository(pool, krs.NewKRRepository(pool))
	tr := teams.NewTeamRepository(pool)
	pr := periods.NewPeriodRepository(pool)
	scope := domain.TenantScope{TenantID: 1}
	_, _, goalID := seedGoal(t, ctx, gr, tr, pr, scope, "nest")

	// Two tasks, oldest first expected.
	t1, err := gr.AddGoalComment(ctx, scope, goalID, "task-1", seedUserID)
	if err != nil {
		t.Fatalf("task1: %v", err)
	}
	if _, err := gr.AddGoalComment(ctx, scope, goalID, "task-2", seedUserID); err != nil {
		t.Fatalf("task2: %v", err)
	}
	// Two replies under task-1.
	if _, err := gr.AddGoalReply(ctx, scope, goalID, t1, "reply-a", seedUserID); err != nil {
		t.Fatalf("reply-a: %v", err)
	}
	if _, err := gr.AddGoalReply(ctx, scope, goalID, t1, "reply-b", seedUserID); err != nil {
		t.Fatalf("reply-b: %v", err)
	}

	comments, err := gr.ListGoalComments(ctx, scope, goalID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(comments) != 2 {
		t.Fatalf("want 2 tasks (replies must be nested), got %d", len(comments))
	}
	if comments[0].Text != "task-1" || comments[1].Text != "task-2" {
		t.Fatalf("tasks must be oldest→newest: %q, %q", comments[0].Text, comments[1].Text)
	}
	if len(comments[0].Replies) != 2 {
		t.Fatalf("task-1 must have 2 replies, got %d", len(comments[0].Replies))
	}
	if comments[0].Replies[0].Text != "reply-a" || comments[0].Replies[1].Text != "reply-b" {
		t.Fatalf("replies must be oldest→newest: %q, %q", comments[0].Replies[0].Text, comments[0].Replies[1].Text)
	}
	if comments[0].Replies[0].ParentID == nil || *comments[0].Replies[0].ParentID != t1 {
		t.Fatalf("reply.ParentID must point to task-1")
	}
}
```
(`AddGoalReply` is added in Task 2 — this test also drives Task 2. It will not compile until Task 2's method exists; implement Steps 4–5 of this task and Task 2 together, running the test after Task 2.)

- [ ] **Step 4: Update scan + read queries to carry parent_id and nest replies**

In `internal/store/goals/goals.go`:

Update `scanGoalComment` to read `parent_id` and `author_user_id`:
```go
func scanGoalComment(rows pgx.Rows) (domain.GoalComment, error) {
	var c domain.GoalComment
	var resolverName, resolverUDID *string
	if err := rows.Scan(&c.ID, &c.GoalID, &c.ParentID, &c.Text, &c.AuthorName, &c.AuthorUDID,
		&c.AuthorUserID, &c.CreatedAt, &c.ResolvedAt, &resolverName, &resolverUDID); err != nil {
		return domain.GoalComment{}, err
	}
	if resolverName != nil {
		c.ResolvedByName = *resolverName
	}
	if resolverUDID != nil {
		c.ResolvedByUDID = *resolverUDID
	}
	return c, nil
}
```

Add a helper that nests replies under tasks (tasks ASC, replies ASC) given rows already ordered by `created_at ASC, id ASC`:
```go
// nestGoalComments splits a flat, created_at-ASC-ordered slice into top-level
// tasks (parent_id IS NULL) each carrying its replies. Replies whose parent is
// absent from the slice are dropped (defensive; should not happen within a goal).
func nestGoalComments(flat []domain.GoalComment) []domain.GoalComment {
	tasks := make([]domain.GoalComment, 0, len(flat))
	idx := make(map[int64]int, len(flat))
	for _, c := range flat {
		if c.ParentID == nil {
			idx[c.ID] = len(tasks)
			tasks = append(tasks, c)
		}
	}
	for _, c := range flat {
		if c.ParentID == nil {
			continue
		}
		if i, ok := idx[*c.ParentID]; ok {
			tasks[i].Replies = append(tasks[i].Replies, c)
		}
	}
	return tasks
}
```

Rewrite `ListGoalComments` to select all rows (tasks + replies) ASC and nest:
```go
func (r *GoalRepository) ListGoalComments(ctx context.Context, scope domain.TenantScope, goalID int64) ([]domain.GoalComment, error) {
	rows, err := r.db.Query(ctx, `
		SELECT gc.id, gc.goal_id, gc.parent_id, gc.text, u.display_name, u.udid,
		       gc.author_user_id, gc.created_at, gc.resolved_at, ru.display_name, ru.udid
		FROM goal_comments gc
		JOIN users u ON u.id = gc.author_user_id
		LEFT JOIN users ru ON ru.id = gc.resolved_by_user_id
		WHERE gc.goal_id = $1 AND gc.tenant_id = $2
		ORDER BY gc.created_at ASC, gc.id ASC`, goalID, scope.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var flat []domain.GoalComment
	for rows.Next() {
		c, err := scanGoalComment(rows)
		if err != nil {
			return nil, err
		}
		flat = append(flat, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return nestGoalComments(flat), nil
}
```

Rewrite `listGoalCommentsBatch` similarly (batch across goals), nesting per goal:
```go
func (r *GoalRepository) listGoalCommentsBatch(ctx context.Context, scope domain.TenantScope, goalIDs []int64) (map[int64][]domain.GoalComment, error) {
	if len(goalIDs) == 0 {
		return nil, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT gc.id, gc.goal_id, gc.parent_id, gc.text, u.display_name, u.udid,
		       gc.author_user_id, gc.created_at, gc.resolved_at, ru.display_name, ru.udid
		FROM goal_comments gc
		JOIN users u ON u.id = gc.author_user_id
		LEFT JOIN users ru ON ru.id = gc.resolved_by_user_id
		WHERE gc.goal_id = ANY($1) AND gc.tenant_id = $2
		ORDER BY gc.created_at ASC, gc.id ASC`, goalIDs, scope.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	flatByGoal := make(map[int64][]domain.GoalComment)
	for rows.Next() {
		c, err := scanGoalComment(rows)
		if err != nil {
			return nil, err
		}
		flatByGoal[c.GoalID] = append(flatByGoal[c.GoalID], c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make(map[int64][]domain.GoalComment, len(flatByGoal))
	for goalID, flat := range flatByGoal {
		result[goalID] = nestGoalComments(flat)
	}
	return result, nil
}
```

- [ ] **Step 5: Build**

Run: `go build ./...`
Expected: compiles (test referencing `AddGoalReply` still fails to build until Task 2 — that is expected; proceed to Task 2, then run tests).

---

## Task 2: Store writes — reply, delete, resolve-only-task

**Files:**
- Modify: `internal/store/goals/goals.go` (AddGoalComment L834, SetGoalCommentResolved L890)
- Test: `internal/store/goals/goals_comments_test.go`

**Interfaces:**
- Consumes: `domain.GoalComment.ParentID`, `nestGoalComments` (Task 1).
- Produces:
  - `func (r *GoalRepository) AddGoalReply(ctx, scope, goalID, parentID int64, text string, authorUserID int64) (int64, error)` — `ErrNotFound` if parent is not a task of this goal/tenant.
  - `func (r *GoalRepository) GetGoalCommentMeta(ctx, scope, goalID, commentID int64) (authorUserID int64, isTask bool, err error)` — `ErrNotFound` if absent.
  - `func (r *GoalRepository) DeleteGoalComment(ctx, scope, goalID, commentID int64) error` — `ErrNotFound` if absent; cascade removes replies.
  - `SetGoalCommentResolved` now treats only tasks as resolvable (reply → `ErrNotFound`).

- [ ] **Step 1: Write the failing tests for reply/delete/resolve-guard**

Add to `internal/store/goals/goals_comments_test.go`:
```go
func TestAddGoalReplyRejectsNonTaskParent(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	gr := goals.NewGoalRepository(pool, krs.NewKRRepository(pool))
	tr := teams.NewTeamRepository(pool)
	pr := periods.NewPeriodRepository(pool)
	scope := domain.TenantScope{TenantID: 1}
	_, _, goalID := seedGoal(t, ctx, gr, tr, pr, scope, "reply-guard")

	task, _ := gr.AddGoalComment(ctx, scope, goalID, "task", seedUserID)
	reply, err := gr.AddGoalReply(ctx, scope, goalID, task, "reply", seedUserID)
	if err != nil {
		t.Fatalf("valid reply: %v", err)
	}
	// Replying to a reply must be rejected (depth 1 only).
	if _, err := gr.AddGoalReply(ctx, scope, goalID, reply, "nested", seedUserID); !errors.Is(err, goals.ErrNotFound) {
		t.Fatalf("reply-to-reply must be ErrNotFound, got %v", err)
	}
	// Replying to a non-existent parent must be rejected.
	if _, err := gr.AddGoalReply(ctx, scope, goalID, 999999, "orphan", seedUserID); !errors.Is(err, goals.ErrNotFound) {
		t.Fatalf("orphan reply must be ErrNotFound, got %v", err)
	}
}

func TestDeleteGoalCommentCascadesReplies(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	gr := goals.NewGoalRepository(pool, krs.NewKRRepository(pool))
	tr := teams.NewTeamRepository(pool)
	pr := periods.NewPeriodRepository(pool)
	scope := domain.TenantScope{TenantID: 1}
	_, _, goalID := seedGoal(t, ctx, gr, tr, pr, scope, "cascade")

	task, _ := gr.AddGoalComment(ctx, scope, goalID, "task", seedUserID)
	if _, err := gr.AddGoalReply(ctx, scope, goalID, task, "reply", seedUserID); err != nil {
		t.Fatalf("reply: %v", err)
	}
	if err := gr.DeleteGoalComment(ctx, scope, goalID, task); err != nil {
		t.Fatalf("delete task: %v", err)
	}
	comments, _ := gr.ListGoalComments(ctx, scope, goalID)
	if len(comments) != 0 {
		t.Fatalf("task + cascaded replies must be gone, got %d", len(comments))
	}
	// Deleting a missing comment → ErrNotFound.
	if err := gr.DeleteGoalComment(ctx, scope, goalID, task); !errors.Is(err, goals.ErrNotFound) {
		t.Fatalf("second delete must be ErrNotFound, got %v", err)
	}
}

func TestGetGoalCommentMeta(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	gr := goals.NewGoalRepository(pool, krs.NewKRRepository(pool))
	tr := teams.NewTeamRepository(pool)
	pr := periods.NewPeriodRepository(pool)
	scope := domain.TenantScope{TenantID: 1}
	_, _, goalID := seedGoal(t, ctx, gr, tr, pr, scope, "meta")

	task, _ := gr.AddGoalComment(ctx, scope, goalID, "task", seedUserID)
	reply, _ := gr.AddGoalReply(ctx, scope, goalID, task, "reply", seedUserID)

	author, isTask, err := gr.GetGoalCommentMeta(ctx, scope, goalID, task)
	if err != nil || author != seedUserID || !isTask {
		t.Fatalf("task meta: author=%d isTask=%v err=%v", author, isTask, err)
	}
	_, isTask, err = gr.GetGoalCommentMeta(ctx, scope, goalID, reply)
	if err != nil || isTask {
		t.Fatalf("reply meta must have isTask=false: isTask=%v err=%v", isTask, err)
	}
	if _, _, err := gr.GetGoalCommentMeta(ctx, scope, goalID, 999999); !errors.Is(err, goals.ErrNotFound) {
		t.Fatalf("missing meta must be ErrNotFound, got %v", err)
	}
}

func TestResolveRejectsReply(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	gr := goals.NewGoalRepository(pool, krs.NewKRRepository(pool))
	tr := teams.NewTeamRepository(pool)
	pr := periods.NewPeriodRepository(pool)
	scope := domain.TenantScope{TenantID: 1}
	_, _, goalID := seedGoal(t, ctx, gr, tr, pr, scope, "resolve-reply")

	task, _ := gr.AddGoalComment(ctx, scope, goalID, "task", seedUserID)
	reply, _ := gr.AddGoalReply(ctx, scope, goalID, task, "reply", seedUserID)
	if _, err := gr.SetGoalCommentResolved(ctx, scope, goalID, reply, true, seedUserID); !errors.Is(err, goals.ErrNotFound) {
		t.Fatalf("resolving a reply must be ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/store/goals/ -run 'TestAddGoalReply|TestDeleteGoalComment|TestGetGoalCommentMeta|TestResolveRejectsReply|TestListGoalCommentsNestsReplies' -v`
Expected: build failure / FAIL — `AddGoalReply`, `DeleteGoalComment`, `GetGoalCommentMeta` undefined.

- [ ] **Step 3: Implement the store methods**

In `internal/store/goals/goals.go`, add:
```go
// AddGoalReply inserts a reply under a task. The INSERT..SELECT guard requires the
// parent to be a task (parent_id IS NULL) of the same goal and tenant, enforcing the
// single-level depth; a bad/absent parent yields no row → ErrNotFound.
func (r *GoalRepository) AddGoalReply(ctx context.Context, scope domain.TenantScope, goalID, parentID int64, text string, authorUserID int64) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx, `
		INSERT INTO goal_comments (goal_id, parent_id, text, author_user_id, tenant_id)
		SELECT $1, $2, $3, $4, $5
		WHERE EXISTS (
			SELECT 1 FROM goal_comments
			WHERE id = $2 AND goal_id = $1 AND tenant_id = $5 AND parent_id IS NULL
		)
		RETURNING id`, goalID, parentID, text, authorUserID, scope.TenantID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

// GetGoalCommentMeta returns the author and whether the comment is a task (parent_id IS NULL),
// pinned to the goal and tenant. ErrNotFound if the comment does not exist in this scope.
func (r *GoalRepository) GetGoalCommentMeta(ctx context.Context, scope domain.TenantScope, goalID, commentID int64) (int64, bool, error) {
	var author int64
	var isTask bool
	err := r.db.QueryRow(ctx, `
		SELECT author_user_id, parent_id IS NULL
		FROM goal_comments
		WHERE id = $1 AND goal_id = $2 AND tenant_id = $3`,
		commentID, goalID, scope.TenantID).Scan(&author, &isTask)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, ErrNotFound
	}
	return author, isTask, err
}

// DeleteGoalComment removes a task (cascading its replies via ON DELETE CASCADE) or a
// single reply. The WHERE pins the row to its goal and tenant. ErrNotFound if absent.
func (r *GoalRepository) DeleteGoalComment(ctx context.Context, scope domain.TenantScope, goalID, commentID int64) error {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM goal_comments WHERE id = $1 AND goal_id = $2 AND tenant_id = $3`,
		commentID, goalID, scope.TenantID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
```

In `SetGoalCommentResolved`, restrict to tasks. Change the guard and the existence probe:
```go
	// State guard: only a TASK (parent_id IS NULL) is resolvable; resolve only an open
	// task, reopen only a resolved one.
	guard := "parent_id IS NULL AND resolved_at IS NULL"
	if !resolved {
		guard = "parent_id IS NULL AND resolved_at IS NOT NULL"
	}
```
and the existence check (so a reply reports as "not a resolvable task" → ErrNotFound):
```go
	var exists bool
	if err := r.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM goal_comments WHERE id=$1 AND goal_id=$2 AND tenant_id=$3 AND parent_id IS NULL)`,
		commentID, goalID, scope.TenantID).Scan(&exists); err != nil {
		return false, err
	}
```

- [ ] **Step 4: Run the store tests**

Run: `go test ./internal/store/goals/ -run 'Comment|Reply|Resolve' -v`
Expected: PASS (all comment/reply/resolve tests, including Task 1's `TestListGoalCommentsNestsRepliesInOrder`).

---

## Task 3: Service layer — reply + delete events + authorization

**Files:**
- Modify: `internal/service/service.go` (goalStore interface ~L55-59, AddGoalComment L897, SetGoalCommentResolved L913)
- Test: `internal/service/goal_test.go`

**Interfaces:**
- Consumes: store `AddGoalReply`, `GetGoalCommentMeta`, `DeleteGoalComment` (Task 2); domain action consts (Task 1).
- Produces:
  - `func (s *Service) AddGoalReply(ctx, scope, goalID, parentID int64, text string, authorUserID int64) error` — `goals.ErrNotFound` propagates.
  - `func (s *Service) DeleteGoalComment(ctx, scope, goalID, commentID, requestingUserID int64, isAdmin bool) (isTask bool, err error)` — `ErrForbidden` when not author and not admin; `goals.ErrNotFound` when absent.
  - `service.ErrForbidden` (exported sentinel).

- [ ] **Step 1: Extend the goalStore interface**

In `internal/service/service.go`, add to the `goalStore` interface (near the existing comment methods ~L57-59):
```go
	AddGoalReply(ctx context.Context, scope domain.TenantScope, goalID, parentID int64, text string, authorUserID int64) (int64, error)
	GetGoalCommentMeta(ctx context.Context, scope domain.TenantScope, goalID, commentID int64) (int64, bool, error)
	DeleteGoalComment(ctx context.Context, scope domain.TenantScope, goalID, commentID int64) error
```

Add an exported sentinel near the top-level service errors (search for an existing `var Err...` block; if none in this file, add):
```go
// ErrForbidden signals an authorization failure the handler maps to HTTP 403.
var ErrForbidden = errors.New("forbidden")
```
(ensure `errors` is imported in `service.go`.)

- [ ] **Step 2: Write the failing service tests**

Add to `internal/service/goal_test.go` (follow the existing activity-assertion helpers in that file — reuse whatever helper lists events for a goal; the snippet below assumes a `listEvents(t, ctx, ...)`-style read or direct store query used elsewhere in the file. If the file already has a helper that fetches activity actions, use it; otherwise query `activity_events` directly as shown):
```go
func TestAddGoalReplyRecordsReplyAddedEvent(t *testing.T) {
	env := newGoalServiceEnv(t) // reuse the existing per-file setup helper
	defer env.cleanup()
	goalID := env.seedGoal(t, "reply-event")
	task, err := env.svc.goals.AddGoalComment(env.ctx, env.scope, goalID, "task", 1)
	if err != nil {
		t.Fatalf("task: %v", err)
	}
	if err := env.svc.AddGoalReply(env.ctx, env.scope, goalID, task, "a reply", 1); err != nil {
		t.Fatalf("reply: %v", err)
	}
	actions := env.actionsForGoal(t, goalID) // helper: SELECT action FROM activity_events WHERE goal_id=$1
	if !contains(actions, string(domain.ActionReplyAdded)) {
		t.Fatalf("expected reply_added event, got %v", actions)
	}
}

func TestDeleteGoalCommentAuthorizationAndEvents(t *testing.T) {
	env := newGoalServiceEnv(t)
	defer env.cleanup()
	goalID := env.seedGoal(t, "delete-auth")

	// Task authored by user 2 (migration).
	task, _ := env.svc.goals.AddGoalComment(env.ctx, env.scope, goalID, "task", 2)
	reply, _ := env.svc.goals.AddGoalReply(env.ctx, env.scope, goalID, task, "reply", 2)

	// Non-author, non-admin (user 1) → ErrForbidden.
	if _, err := env.svc.DeleteGoalComment(env.ctx, env.scope, goalID, reply, 1, false); !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("non-author non-admin must be forbidden, got %v", err)
	}
	// Admin (user 1, isAdmin=true) → deletes the reply, logs reply_deleted.
	isTask, err := env.svc.DeleteGoalComment(env.ctx, env.scope, goalID, reply, 1, true)
	if err != nil || isTask {
		t.Fatalf("admin delete reply: isTask=%v err=%v", isTask, err)
	}
	// Author (user 2) deletes their task → cascade, logs comment_deleted.
	isTask, err = env.svc.DeleteGoalComment(env.ctx, env.scope, goalID, task, 2, false)
	if err != nil || !isTask {
		t.Fatalf("author delete task: isTask=%v err=%v", isTask, err)
	}
	actions := env.actionsForGoal(t, goalID)
	if !contains(actions, string(domain.ActionReplyDeleted)) || !contains(actions, string(domain.ActionCommentDeleted)) {
		t.Fatalf("expected reply_deleted and comment_deleted, got %v", actions)
	}
	// Missing → ErrNotFound.
	if _, err := env.svc.DeleteGoalComment(env.ctx, env.scope, goalID, task, 2, false); !errors.Is(err, goals.ErrNotFound) {
		t.Fatalf("delete missing must be ErrNotFound, got %v", err)
	}
}
```
> Adapt `newGoalServiceEnv`/`env.seedGoal`/`env.actionsForGoal`/`contains` to the actual helpers already present in `goal_test.go` / `service_test.go`. If no `actionsForGoal` helper exists, add a small one that runs `SELECT action FROM activity_events WHERE goal_id=$1 ORDER BY id`. Do not invent an API the file doesn't have — read the file first and match its style.

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/service/ -run 'TestAddGoalReply|TestDeleteGoalCommentAuthorization' -v`
Expected: build failure / FAIL — `AddGoalReply` / `DeleteGoalComment` undefined on `*Service`.

- [ ] **Step 4: Implement the service methods**

In `internal/service/service.go`, add after `SetGoalCommentResolved`:
```go
func (s *Service) AddGoalReply(ctx context.Context, scope domain.TenantScope, goalID, parentID int64, text string, authorUserID int64) error {
	replyID, err := s.goals.AddGoalReply(ctx, scope, goalID, parentID, text, authorUserID)
	if err != nil {
		return err // includes goals.ErrNotFound for a bad/non-task parent
	}
	if g, gerr := s.goals.GetGoal(ctx, scope, goalID); gerr == nil {
		teamID, periodID := g.TeamID, g.PeriodID
		s.recordActivity(ctx, scope, domain.ActivityEvent{
			ActorUserID: authorUserID, Category: domain.ActivityDiscussion, Action: domain.ActionReplyAdded,
			TeamID: &teamID, PeriodID: &periodID, GoalID: &goalID, CommentID: &replyID,
			EntityTitle: g.Title, Payload: map[string]any{"text": text},
		})
	}
	return nil
}

// DeleteGoalComment removes a task (cascading its replies) or a reply. Authorization:
// the requesting user must be the author, or a tenant admin. Returns isTask so the
// caller/log distinguishes a task deletion (comment_deleted) from a reply (reply_deleted).
// A cascaded task deletion logs a single comment_deleted event (replies vanish silently).
func (s *Service) DeleteGoalComment(ctx context.Context, scope domain.TenantScope, goalID, commentID, requestingUserID int64, isAdmin bool) (bool, error) {
	author, isTask, err := s.goals.GetGoalCommentMeta(ctx, scope, goalID, commentID)
	if err != nil {
		return false, err // goals.ErrNotFound if absent
	}
	if author != requestingUserID && !isAdmin {
		return false, ErrForbidden
	}
	// Snapshot title before deletion for the journal.
	title := ""
	if g, gerr := s.goals.GetGoal(ctx, scope, goalID); gerr == nil {
		title = g.Title
	}
	if err := s.goals.DeleteGoalComment(ctx, scope, goalID, commentID); err != nil {
		return false, err
	}
	action := domain.ActionReplyDeleted
	if isTask {
		action = domain.ActionCommentDeleted
	}
	if g, gerr := s.goals.GetGoal(ctx, scope, goalID); gerr == nil {
		teamID, periodID := g.TeamID, g.PeriodID
		s.recordActivity(ctx, scope, domain.ActivityEvent{
			ActorUserID: requestingUserID, Category: domain.ActivityDiscussion, Action: action,
			TeamID: &teamID, PeriodID: &periodID, GoalID: &goalID, CommentID: &commentID,
			EntityTitle: title,
		})
	}
	return isTask, nil
}
```
> Note: `GetGoal` is called twice (title snapshot before delete, team/period after). Deleting a task does not delete the goal, so the post-delete `GetGoal` still succeeds. If you prefer one call, capture `teamID`/`periodID`/`title` from the pre-delete `GetGoal` and drop the second — either is fine; keep it simple and consistent with `AddGoalComment`'s existing pattern.

- [ ] **Step 5: Run the service tests**

Run: `go test ./internal/service/ -run 'Comment|Reply' -v`
Expected: PASS. Then `go build ./...` to confirm the interface additions compile everywhere the store is used (mocks/fakes may need the three new methods — see next step).

- [ ] **Step 6: Update any store fakes/mocks**

Run: `go vet ./... && go test ./... 2>&1 | rg -i 'does not implement|missing method' || echo OK`
If a test fake implements `goalStore`, add the three new methods to it (delegating to the real repo or returning zero values as the existing fakes do). Expected final: `OK`.

---

## Task 4: DTO + response nesting

**Files:**
- Modify: `internal/http/dto/goal.go` (GoalComment L13-23)
- Modify: `internal/http/handlers/api/v1/helpers_response.go` (MapGoalComment L196-208)
- Modify: `internal/http/handlers/api/v1/goals/response.go` (uses MapGoalComment — no change needed if it delegates)

**Interfaces:**
- Consumes: `domain.GoalComment.Replies` (Task 1).
- Produces: `dto.GoalComment.Replies []dto.GoalReply`; `dto.GoalReply{ID, Text, AuthorName, AuthorUDID, CreatedAt}`.

- [ ] **Step 1: Add the reply DTO and the Replies field**

In `internal/http/dto/goal.go`:
```go
type GoalReply struct {
	ID         int64     `json:"id"`
	Text       string    `json:"text"`
	AuthorName string    `json:"author_name"`
	AuthorUDID string    `json:"author_udid"`
	CreatedAt  time.Time `json:"created_at"`
}
```
Add to `GoalComment` struct:
```go
	Replies        []GoalReply `json:"replies"`
```

- [ ] **Step 2: Nest replies in MapGoalComment**

In `internal/http/handlers/api/v1/helpers_response.go`, update `MapGoalComment`:
```go
func MapGoalComment(c domain.GoalComment) dto.GoalComment {
	replies := make([]dto.GoalReply, 0, len(c.Replies))
	for _, r := range c.Replies {
		replies = append(replies, dto.GoalReply{
			ID:         r.ID,
			Text:       r.Text,
			AuthorName: r.AuthorName,
			AuthorUDID: r.AuthorUDID,
			CreatedAt:  r.CreatedAt,
		})
	}
	return dto.GoalComment{
		ID:             c.ID,
		Text:           c.Text,
		AuthorName:     c.AuthorName,
		AuthorUDID:     c.AuthorUDID,
		CreatedAt:      c.CreatedAt,
		Resolved:       c.ResolvedAt != nil,
		ResolvedByName: c.ResolvedByName,
		ResolvedByUDID: c.ResolvedByUDID,
		ResolvedAt:     c.ResolvedAt,
		Replies:        replies,
	}
}
```

- [ ] **Step 3: Build**

Run: `go build ./... && go vet ./...`
Expected: compiles. `newGoalResponse` (response.go) already iterates `goal.Comments` and calls `MapGoalComment` — replies are nested automatically; verify no other call site constructs `dto.GoalComment` literally (search):
Run: `rg -n "dto.GoalComment{" internal/`
Expected: only `MapGoalComment`. If another exists, add `Replies` there too.

---

## Task 5: HTTP handlers + routes

**Files:**
- Modify: `internal/http/handlers/api/v1/goals/routes.go`
- Modify: `internal/http/handlers/api/v1/goals/handler.go` (setGoalCommentResolved L213, add reply + delete handlers)
- Test: `internal/http/handlers/api/v1/goals/replies_test.go` (new)

**Interfaces:**
- Consumes: `service.AddGoalReply`, `service.DeleteGoalComment`, `service.ErrForbidden`, `goals.ErrNotFound`, `auth.ActiveRoleFromContext`, `domain.RoleAdmin`.
- Produces: routes `POST /api/v1/goals/{goalID}/comments/{commentID}/replies`, `DELETE /api/v1/goals/{goalID}/comments/{commentID}`.

- [ ] **Step 1: Register the new routes**

In `internal/http/handlers/api/v1/goals/routes.go`, add inside `RegisterRoutes`:
```go
	r.Post("/api/v1/goals/{goalID}/comments/{commentID}/replies", h.HandleAddGoalReply)
	r.Delete("/api/v1/goals/{goalID}/comments/{commentID}", h.HandleDeleteGoalComment)
```

- [ ] **Step 2: Write the failing handler tests**

Create `internal/http/handlers/api/v1/goals/replies_test.go` (mirror the setup in `resolve_test.go` — `setupGoalAccessDB`, `service.NewFromStore`, `testutil.NewAPIV1RouterWithScope`):
```go
package goals_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"okrs/internal/domain"
	"okrs/internal/http/handlers/api/v1/testutil"
	"okrs/internal/service"
	"okrs/internal/store/goals"
	"okrs/internal/store/grants"
)

func TestReplyAndDeleteHandlers(t *testing.T) {
	pool, repo, teardown := setupGoalAccessDB(t)
	defer teardown()
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}

	var teamID int64
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('Team') RETURNING id`).Scan(&teamID); err != nil {
		t.Fatalf("team: %v", err)
	}
	var periodID int64
	if err := pool.QueryRow(ctx, `INSERT INTO periods (name, start_date, end_date) VALUES ('Q1','2024-01-01','2024-03-31') RETURNING id`).Scan(&periodID); err != nil {
		t.Fatalf("period: %v", err)
	}
	goalID, err := repo.Goals.CreateGoal(ctx, scope, goals.GoalInput{
		TeamID: teamID, PeriodID: periodID, Title: "Goal",
		Priority: domain.PriorityP1, Weight: 100,
		WorkType: domain.WorkTypeDelivery, FocusType: domain.FocusStability,
	})
	if err != nil {
		t.Fatalf("goal: %v", err)
	}
	// Task authored by the request user (anonymous id=1) and one by user 2.
	ownTask, _ := repo.Goals.AddGoalComment(ctx, scope, goalID, "mine", 1)
	othersTask, _ := repo.Goals.AddGoalComment(ctx, scope, goalID, "theirs", 2)

	svc := service.NewFromStore(repo, grants.NewGrantsCache(repo.Grants), nil, nil)
	server := httptest.NewServer(testutil.NewAPIV1RouterWithScope(svc, []int64{teamID}))
	defer server.Close()

	post := func(path, body string) int {
		resp, err := http.Post(server.URL+path, "application/json", bytes.NewBufferString(body))
		if err != nil {
			t.Fatalf("post %s: %v", path, err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}
	del := func(path string) int {
		req, _ := http.NewRequest(http.MethodDelete, server.URL+path, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("delete %s: %v", path, err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	base := fmt.Sprintf("/api/v1/goals/%d/comments", goalID)

	// Reply to my task → 200.
	if code := post(fmt.Sprintf("%s/%d/replies", base, ownTask), `{"text":"a reply"}`); code != http.StatusOK {
		t.Fatalf("reply: want 200 got %d", code)
	}
	// Fetch the reply id via store to attempt a reply-to-reply.
	comments, _ := repo.Goals.ListGoalComments(ctx, scope, goalID)
	var replyID int64
	for _, c := range comments {
		if c.ID == ownTask && len(c.Replies) > 0 {
			replyID = c.Replies[0].ID
		}
	}
	// Reply to a reply → 404.
	if code := post(fmt.Sprintf("%s/%d/replies", base, replyID), `{"text":"nested"}`); code != http.StatusNotFound {
		t.Fatalf("reply-to-reply: want 404 got %d", code)
	}
	// Empty reply text → 400.
	if code := post(fmt.Sprintf("%s/%d/replies", base, ownTask), `{"text":""}`); code != http.StatusBadRequest {
		t.Fatalf("empty reply: want 400 got %d", code)
	}
	// Resolve a reply → 404 (reply is not resolvable).
	if code := post(fmt.Sprintf("%s/%d/resolve", base, replyID), `{}`); code != http.StatusNotFound {
		t.Fatalf("resolve reply: want 404 got %d", code)
	}
	// Delete someone else's task as non-admin (request user is 1, author is 2) → 403.
	if code := del(fmt.Sprintf("%s/%d", base, othersTask)); code != http.StatusForbidden {
		t.Fatalf("delete others: want 403 got %d", code)
	}
	// Delete my own task → 200 (cascades my reply).
	if code := del(fmt.Sprintf("%s/%d", base, ownTask)); code != http.StatusOK {
		t.Fatalf("delete mine: want 200 got %d", code)
	}
	// Delete again → 404.
	if code := del(fmt.Sprintf("%s/%d", base, ownTask)); code != http.StatusNotFound {
		t.Fatalf("delete missing: want 404 got %d", code)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/http/handlers/api/v1/goals/ -run TestReplyAndDeleteHandlers -v`
Expected: build failure / FAIL — `HandleAddGoalReply` / `HandleDeleteGoalComment` undefined.

- [ ] **Step 4: Implement the handlers**

In `internal/http/handlers/api/v1/goals/handler.go`, add the reply handler (mirrors `HandleAddGoalComment` + parses `commentID`):
```go
func (h *Handler) HandleAddGoalReply(w http.ResponseWriter, r *http.Request) {
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid goal id", map[string]string{"goal_id": "invalid"})
		return
	}
	commentID, err := common.ParseID(chi.URLParam(r, "commentID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid comment id", map[string]string{"comment_id": "invalid"})
		return
	}
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	goal, err := h.service.GetGoal(r.Context(), scope, goalID)
	if err != nil || !h.canAccessGoal(r.Context(), scope, goal) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if req.Text == "" {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "text required", map[string]string{"text": "required"})
		return
	}
	if err := h.service.AddGoalReply(r.Context(), scope, goalID, commentID, req.Text, auth.UserIDFromContext(r.Context())); err != nil {
		if errors.Is(err, goals.ErrNotFound) {
			v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "comment not found", nil)
			return
		}
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to add reply", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
```
Add the delete handler:
```go
func (h *Handler) HandleDeleteGoalComment(w http.ResponseWriter, r *http.Request) {
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid goal id", map[string]string{"goal_id": "invalid"})
		return
	}
	commentID, err := common.ParseID(chi.URLParam(r, "commentID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid comment id", map[string]string{"comment_id": "invalid"})
		return
	}
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	goal, err := h.service.GetGoal(r.Context(), scope, goalID)
	if err != nil || !h.canAccessGoal(r.Context(), scope, goal) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	role, _ := auth.ActiveRoleFromContext(r.Context())
	isAdmin := role == domain.RoleAdmin
	if _, err := h.service.DeleteGoalComment(r.Context(), scope, goalID, commentID, auth.UserIDFromContext(r.Context()), isAdmin); err != nil {
		if errors.Is(err, goals.ErrNotFound) {
			v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "comment not found", nil)
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "not allowed to delete", nil)
			return
		}
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to delete comment", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
```
(`service`, `domain`, `goals`, `errors` are already imported in this file — confirm `service` is; `resolve` handler uses `goals.ErrNotFound` and `service` is imported for `service.ShareTarget`.)

- [ ] **Step 5: Run the handler tests**

Run: `go test ./internal/http/handlers/api/v1/goals/ -run 'TestReplyAndDeleteHandlers|TestResolveGoalComment' -v`
Expected: PASS.

- [ ] **Step 6: Full backend suite**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS (Docker-gated tests may skip).

---

## Task 6: Frontend — replies UI, delete, counter (tracker.js)

**Files:**
- Modify: `internal/web/static/tracker.js` (mapping L170, CommentRow L1012, CommentsPanel L1041, GoalCard L1085/L1115-1118, footer L1208-1210, App L2033/L2095, TeamPage render L2374)

**Interfaces:**
- Consumes: API `comments[].replies[]`, `POST .../comments/{id}/replies`, `DELETE .../comments/{id}`, `me.udid`, config `is_admin`.

- [ ] **Step 1: Map replies from the API**

In the goal mapping (~L170), extend the comment mapping to carry replies and author id for the own-check:
```js
comments: (g.comments || []).map(c => ({
  id: c.id, author: c.author_name, authorUdid: c.author_udid, date: fmtDate(c.created_at),
  text: c.text, resolved: !!c.resolved, resolvedBy: c.resolved_by_name,
  resolvedByUdid: c.resolved_by_udid, resolvedAt: c.resolved_at ? fmtDate(c.resolved_at) : null,
  replies: (c.replies || []).map(rp => ({
    id: rp.id, author: rp.author_name, authorUdid: rp.author_udid,
    date: fmtDate(rp.created_at), text: rp.text,
  })),
})),
```

- [ ] **Step 2: Thread `is_admin` through App → TeamPage → GoalCard**

In `App` (~L2033) add state and set it from config (~L2095-2097):
```js
const [isAdmin, setIsAdmin] = useState(false);
```
In the config `.then(...)` block add: `if (cfg) { ...; setIsAdmin(!!cfg.is_admin); }`

Pass `isAdmin` down to the team page component that renders `GoalCard`, then to `GoalCard` (~L2374):
```js
<GoalCard key={g.id} goal={g} editMode={editMode} onReload={reload} onEditGoal={setGoalModal}
  me={me} isAdmin={isAdmin} accent={accent} currentTeamId={selId} allTeams={hierarchy}
  staleDays={staleDays} periodStatus={status} greenThreshold={greenThreshold} deepLink={deepLinkRef.current} />
```
(Trace the intermediate component between `App` and `GoalCard` — if `App` renders `TeamPage`/`TeamOKRs` which renders `GoalCard`, add an `isAdmin` prop to that component's signature and forward it. Grep `rg -n "isAdmin|<GoalCard|function Team" internal/web/static/tracker.js` to place it.)

- [ ] **Step 3: Add reply + delete actions in GoalCard**

Update `GoalCard` signature (~L1085) to accept `isAdmin`:
```js
function GoalCard({ goal, editMode, onReload, onEditGoal, me, isAdmin = false, accent, currentTeamId, allTeams, dragProps, onReorderKR, staleDays = 7, periodStatus, greenThreshold = 80, deepLink = null }) {
```
Add action callbacks near the existing comment callbacks (~L1115):
```js
const addGoalReply = async (parentId, text) => { await apiPost(`/api/v1/goals/${goal.id}/comments/${parentId}/replies`, { text }); onReload(); };
const deleteComment = async commentId => { await apiDelete(`/api/v1/goals/${goal.id}/comments/${commentId}`); onReload(); };
```
Pass the new props into `CommentsPanel` (~L1254):
```js
<CommentsPanel comments={goal.comments} onAdd={addGoalComment} onResolve={resolveComment}
  onUnresolve={unresolveComment} onReply={addGoalReply} onDelete={deleteComment}
  me={me} isAdmin={isAdmin} />
```

- [ ] **Step 4: Render replies, reply-compose, and delete buttons**

Replace `CommentRow` and add `ReplyRow` in the COMMENTS PANEL section (~L1009-1082):
```js
// canModerate: the current user authored the item, or is a tenant admin.
function canModerate(authorUdid, me, isAdmin) {
  return isAdmin || (!!me && !!authorUdid && me.udid === authorUdid);
}

function ReplyRow({ r, canDelete, onDelete }) {
  const [busy, setBusy] = useState(false);
  const [confirm, setConfirm] = useState(false);
  const act = async fn => { setBusy(true); try { await fn(); } catch { } finally { setBusy(false); } };
  return (
    <div id={`comment-${r.id}`} className="comment comment--reply">
      <AvatarWithUDID name={r.author} udid={r.authorUdid} size={24} />
      <div className="comment__content">
        <div className="comment__header">
          <span className="comment__author">{r.author}</span>
          <span className="comment__date">{r.date}</span>
        </div>
        <Markdown text={r.text} className="comment__text" />
        {canDelete && (
          <div className="comment__actions">
            {confirm ? (
              <>
                <span className="comment__confirm">Удалить ответ?</span>
                <button className="comment__link-btn" disabled={busy} onClick={() => act(() => onDelete(r.id))}>Да</button>
                <button className="comment__link-btn" disabled={busy} onClick={() => setConfirm(false)}>Отмена</button>
              </>
            ) : (
              <button className="comment__link-btn" onClick={() => setConfirm(true)}>Удалить</button>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

function CommentRow({ c, onResolve, onUnresolve, onReply, onDelete, me, isAdmin }) {
  const [busy, setBusy] = useState(false);
  const [replyText, setReplyText] = useState('');
  const [showReply, setShowReply] = useState(false);
  const [confirm, setConfirm] = useState(false);
  const act = async fn => { setBusy(true); try { await fn(); } catch { } finally { setBusy(false); } };
  const canDel = canModerate(c.authorUdid, me, isAdmin);
  const submitReply = async () => {
    if (!replyText.trim()) return;
    await act(() => onReply(c.id, replyText.trim()));
    setReplyText(''); setShowReply(false);
  };
  return (
    <div id={`comment-${c.id}`} className={`comment${c.resolved ? ' comment--resolved' : ''}`}>
      <AvatarWithUDID name={c.author} udid={c.authorUdid} size={28} />
      <div className="comment__content">
        <div className="comment__header">
          <span className="comment__author">{c.author}</span>
          <span className="comment__date">{c.date}</span>
          {c.resolved && <span className="comment__resolved-badge">✓ Решено</span>}
        </div>
        <Markdown text={c.text} className="comment__text" />
        {c.resolved ? (
          <div className="comment__resolved-meta">
            Решено{c.resolvedBy ? ` · ${c.resolvedBy}` : ''}{c.resolvedAt ? ` · ${c.resolvedAt}` : ''}
            {' · '}
            <button className="comment__link-btn" disabled={busy} onClick={() => act(() => onUnresolve(c.id))}>Вернуть</button>
          </div>
        ) : (
          <div className="comment__actions">
            <button className="comment__resolve-btn" disabled={busy} onClick={() => act(() => onResolve(c.id))}>✓ Отметить решённым</button>
            <button className="comment__link-btn" onClick={() => setShowReply(v => !v)}>Ответить</button>
            {canDel && !confirm && <button className="comment__link-btn" onClick={() => setConfirm(true)}>Удалить</button>}
            {canDel && confirm && (
              <>
                <span className="comment__confirm">Удалить замечание и ответы?</span>
                <button className="comment__link-btn" disabled={busy} onClick={() => act(() => onDelete(c.id))}>Да</button>
                <button className="comment__link-btn" disabled={busy} onClick={() => setConfirm(false)}>Отмена</button>
              </>
            )}
          </div>
        )}
        {(c.replies || []).length > 0 && (
          <div className="comment__replies">
            {c.replies.map(r => (
              <ReplyRow key={r.id} r={r} canDelete={canModerate(r.authorUdid, me, isAdmin)} onDelete={onDelete} />
            ))}
          </div>
        )}
        {showReply && (
          <div className="comment-compose comment-compose--reply">
            <MarkdownEditor value={replyText} onChange={setReplyText} rows={2} placeholder="Ответ… (Cmd+Enter)"
              onKeyDown={e => { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) submitReply(); }}
              textareaClassName="form-textarea form-textarea--sm" textareaStyle={{ width: '100%', resize: 'vertical' }} />
            <div className="comment-submit-row">
              <button onClick={submitReply} disabled={!replyText.trim() || busy}
                className={`comment-submit ${replyText.trim() ? 'comment-submit--active' : 'comment-submit--disabled'}`}>Ответить</button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
```
Update `CommentsPanel` to accept and forward the new props:
```js
function CommentsPanel({ comments, onAdd, onResolve, onUnresolve, onReply, onDelete, me, isAdmin }) {
```
and in both `open.map(...)` and `resolved.map(...)` pass:
```js
<CommentRow key={c.id || i} c={c} onResolve={onResolve} onUnresolve={onUnresolve}
  onReply={onReply} onDelete={onDelete} me={me} isAdmin={isAdmin} />
```

- [ ] **Step 5: Counter = number of tasks**

The footer button (~L1208-1210) already uses `goal.comments.length`, which is now the task count (replies are nested). Confirm the unresolved badge still counts tasks:
```js
const unresolvedCount = (goal.comments || []).filter(c => !c.resolved).length;
```
No change required beyond verifying `goal.comments` holds tasks only (it does after Step 1). Leave the `💬 {goal.comments.length}` label as-is.

- [ ] **Step 6: Add minimal styles**

Append reply/confirm styles to the tracker stylesheet (find the CSS file feeding `comment__*` classes — grep `rg -n "comment__actions|comment__link-btn" internal/web/static/*.css internal/web/**/*.html`; add to that file):
```css
.comment--reply { margin-left: 0; }
.comment__replies { margin-top: 8px; padding-left: 16px; border-left: 2px solid var(--border, #e5e7eb); display: flex; flex-direction: column; gap: 10px; }
.comment-compose--reply { margin-top: 8px; }
.comment__confirm { font-size: 12px; color: var(--danger, #b91c1c); margin-right: 4px; }
```
(If `comment__*` styles live inline in an HTML template or a JS style string, add there instead — match the existing location.)

- [ ] **Step 7: Manual verification**

Run the app (see project run instructions), open a goal card:
- add a task → appears at the bottom of the open list (oldest→newest);
- click «Ответить», submit → reply appears indented under the task; footer counter unchanged (counts tasks);
- delete a reply (as author) → gone; delete a task → task and its replies gone;
- delete button is absent on comments you didn't author (unless you are tenant admin);
- resolve still works on tasks; replies have no resolve control.

Verify no console errors and `comments.length` on the footer equals the number of tasks.

---

## Task 7: Activity log descriptions (activity.js)

**Files:**
- Modify: `internal/web/static/activity.js` (eventSummary switch L175-195, eventMarkdownBody L198-204)

**Interfaces:**
- Consumes: event actions `reply_added`, `comment_deleted`, `reply_deleted` (Task 1/3).

- [ ] **Step 1: Add the new action descriptions**

In `eventSummary` (~L191-193), change the task description and add the three new cases:
```js
    case 'comment_added': return <>оставил замечание к «{t}»</>;
    case 'reply_added': return <>ответил на замечание к «{t}»</>;
    case 'comment_resolved': return <>отметил замечание к «{t}» решённым</>;
    case 'comment_reopened': return <>переоткрыл замечание к «{t}»</>;
    case 'comment_deleted': return <>удалил замечание к «{t}»</>;
    case 'reply_deleted': return <>удалил ответ на замечание к «{t}»</>;
```

- [ ] **Step 2: Render reply text in the feed**

In `eventMarkdownBody` (~L199-203), also return text for `reply_added`:
```js
function eventMarkdownBody(ev) {
  const p = ev.payload || {};
  if (ev.action === 'comment_added' || ev.action === 'reply_added') return p.text || '';
  if (ev.action === 'kr_note_updated') return (p.after || {}).note || '';
  return null;
}
```

- [ ] **Step 3: Manual verification**

Open `/activity-log` as an admin, filter «Обсуждения»:
- adding a task shows «оставил замечание к …» with the text;
- adding a reply shows «ответил на замечание к …» with the reply text;
- deleting a task shows «удалил замечание к …»; deleting a reply shows «удалил ответ на замечание к …»;
- all four remain under the single «Обсуждения» filter.

---

## Task 8: Update specs

**Files:**
- Modify: `specs/020-domain-model.md` (GoalComment ~L117-135; ActivityEvent action list ~L178)
- Modify: `specs/040-api-contract.md` (comments write endpoints ~L454-456; `comments[]` shape ~L582-593)
- Modify: `specs/050-permissions-and-lifecycle.md` (user rights ~L149-152)
- Check: `specs/030-user-flows.md` (only if it documents the comment flow)

- [ ] **Step 1: 020-domain-model.md — GoalComment**

In the `GoalComment` **Поля** list add:
```
- parent_id (nullable, FK → goal_comments.id, ON DELETE CASCADE) — NULL для таски (замечания), ссылка на таску для ответа
```
In **Инварианты** add:
```
- комментарий первого уровня (`parent_id IS NULL`) — таска/замечание с состоянием resolve; ответ (`parent_id` указывает на таску) таской не является: у ответа `resolved_at`/`resolved_by_user_id` всегда NULL;
- глубина вложенности ровно один уровень — ответ можно оставить только на таску, ответа на ответ нет (`parent_id` всегда указывает на строку с `parent_id IS NULL`);
- удаление таски каскадно удаляет её ответы (`ON DELETE CASCADE`);
- удалять таску/ответ может автор или tenant-admin.
```
In `ActivityEvent` → **action** list, extend the enumeration with `reply_added`, `comment_deleted`, `reply_deleted`.

- [ ] **Step 2: 040-api-contract.md — endpoints + shape**

Under **Write endpoints** (near `add goal comment`), add:
```
- add goal reply — `POST /api/v1/goals/{goalID}/comments/{commentID}/replies` (ответ на таску `commentID`; `commentID` должен быть таской, иначе `404`)
- delete goal comment/reply — `DELETE /api/v1/goals/{goalID}/comments/{commentID}` (автор ИЛИ tenant-admin; удаление таски каскадно удаляет ответы)
```
Replace the `comments[]` example (~L582-593) so tasks carry nested `replies[]`, and document:
```
- `comments[]` содержит только таски (`parent_id IS NULL`), отсортированные `created_at ASC` (старые→новые); у каждой таски — `replies[]` (ответы, `created_at ASC`), несущие `id, text, author_name, author_udid, created_at` без resolve-полей;
- счётчик комментариев цели = число тасок (`comments.length`); ответы в счётчик не входят;
- `resolve`/`unresolve` применимы только к таске; для ответа → `404 NOT_FOUND`;
- контроль доступа для всех comment/reply/resolve/delete: tenant-scope → доступ к команде-владельцу или shared-команде (`404` иначе) → привязка `commentID` к `goalID`+тенанту; для `DELETE` дополнительно автор или tenant-admin (`403` иначе).
```

- [ ] **Step 3: 050-permissions-and-lifecycle.md — user rights**

In **Права user** extend the comment line:
```
- CRUD goal / KR / progress в доступных командах; создание тасок и ответов к целям; резолв/reopen тасок; удаление своих тасок/ответов (tenant-admin — любых). Удаление таски каскадно удаляет её ответы.
```
Add a short note that reply/delete do not depend on team period status (allowed wherever commenting is allowed, including `closed`), and are enforced server-side.

- [ ] **Step 4: Check 030-user-flows.md**

Run: `rg -n -i "коммент|замечан|comment" specs/030-user-flows.md`
If the comment flow is described there, add one line about replies and own-delete; if not, leave it untouched (do not modify unrelated flows).

- [ ] **Step 5: Verify specs read consistently**

Re-read the three edited sections; confirm no contradiction with the design doc. No command needed.

---

## Task 9: Seed demo

**Files:**
- Modify: `seed_demo.sql` (comment inserts ~L261-262 and ~L450-458)

- [ ] **Step 1: Add reply rows to the demo**

In the second catalog block (after the L450-454 `INSERT INTO goal_comments ... VALUES (100..104 ...)`), add replies referencing task ids, and keep the existing resolved-task update. Insert a new statement:
```sql
-- Replies (parent_id → task). Demonstrates the task→replies thread.
INSERT INTO goal_comments (id, goal_id, parent_id, text, author_user_id, created_at) VALUES
  (150, 100, 100, 'Согласовали с Infra: HTTP/2 включаем на следующей неделе.', 1, NOW()),
  (151, 100, 101, 'Уточнил у PaaS — конфиг nginx поправят в этом спринте.', 1, NOW());
```
(Task ids 100/101 exist in that block. `parent_id` column now exists post-migration 040. Ensure the `setval('goal_comments_id_seq', ...)` at ~L503 still runs after these inserts — it takes MAX(id), so no change needed since 151 > 104.)

- [ ] **Step 2: Verify the seed applies cleanly**

Run the project's seed/reset flow against a fresh DB (see project docs; typically `make seed` or applying `migrations` then `seed_demo.sql`). Expected: no FK/constraint errors; the goal 100 card shows one task with a reply and one resolved task.

- [ ] **Step 3: Final full check**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS (Docker-gated tests may skip). Manually confirm tracker and activity-log per Tasks 6–7.

---

## Self-Review

**Spec coverage (design doc → tasks):**
- parent_id model + cascade → Task 1 (migration/domain) + Task 2 (store).
- tasks oldest→newest, replies nested → Task 1 (ordering/nesting), Task 4 (DTO), Task 6 (UI).
- reply endpoint, depth-1 enforcement → Task 2 (store guard), Task 3 (service), Task 5 (handler).
- delete own/admin + cascade → Task 2/3/5; authorization matrix tested in Task 3 (service) and Task 5 (handler).
- counter = task count → Task 4 (nesting) + Task 6 Step 5.
- activity events (reply_added/comment_deleted/reply_deleted), one filter, distinct wording → Task 1 (consts), Task 3 (recording), Task 7 (descriptions).
- resolve only on tasks → Task 2 (store guard) + Task 5 (handler 404 test).
- access chain (tenant→team→binding→authorship) → Task 5 handler (access + binding) + Task 2 SQL tenant/goal pinning + Task 3 authorship.
- specs updated → Task 8; seed updated → Task 9.

**Placeholder scan:** No TBD/TODO; every code step shows concrete code. The service-test helpers (`newGoalServiceEnv`, `actionsForGoal`) are explicitly flagged to be matched to the file's existing helpers — the implementer must read `goal_test.go` first (this is guidance, not a placeholder in shipped code).

**Type consistency:** `AddGoalReply(goalID, parentID, text, authorUserID)`, `GetGoalCommentMeta → (int64, bool, error)`, `DeleteGoalComment(goalID, commentID)` store sigs match their service callers; `service.DeleteGoalComment(..., requestingUserID, isAdmin) (bool, error)` matches handler usage; `dto.GoalReply` fields match `MapGoalComment` and the `replies[]` JSON consumed in tracker.js. Action constants (`ActionReplyAdded`=`"reply_added"`, etc.) match activity.js `case` strings.
