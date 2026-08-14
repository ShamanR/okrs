# Копирование и перенос цели — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Дать пользователю возможность скопировать или перенести цель (с её KR и опционально прогрессом/заметками и комментариями) в другую команду и/или период из меню `···` карточки цели.

**Architecture:** Новый store-примитив `GoalRepository.CopyGoal` делает атомарный deep-copy (goal + KR + meta + опц. заметки/комментарии) в одной транзакции. Сервис `Service.CopyGoal` оборачивает его правилами доступа/статуса и активностью; при `mode=move` дополнительно жёстко удаляет исходную цель (каскад по FK). HTTP-эндпоинт `POST /api/v1/goals/{goalID}/transfer`. На фронте — новый пункт меню + модалка `TransferGoalModal`, переиспользующая `PeriodSelect` и доработанный `TeamCombobox`.

**Tech Stack:** Go (chi, pgx v5, PostgreSQL), React (in-browser Babel, без бандлера), тесты стора — `internal/store/testutil.SetupDB` (реальный Postgres), тесты сервиса — фейковый стор (`goalFakeStore`).

**Spec:** `docs/superpowers/specs/2026-08-14-copy-move-goal-design.md`

## Global Constraints

- Все запросы к БД tenant-scoped: каждый метод принимает `domain.TenantScope` и фильтрует по `tenant_id`.
- Слои не смешиваются: HTTP → `service` → `store`. Бизнес-правила (guard статуса, активность) — только в `service`; SQL — только в `store`.
- Активность пишется best-effort через `s.recordActivity` (ошибка логируется, не роняет мутацию).
- Мутации через браузер требуют CSRF; новый POST под `/api/v1` наследует CSRF + auth + tenant-scope + membership от группы роутов в `internal/http/server.go` — отдельная регистрация в `server.go` не нужна, роут добавляется в `internal/http/handlers/api/v1/goals/routes.go`.
- Миграции БД **не создаются**: переиспользуются существующие таблицы; `activity_events.action` — свободный текст.
- В сообщениях коммитов, комментариях, доках **не упоминать** AI/ассистентов/генерацию (CLAUDE.md).
- Коммиты делает пользователь сам — **шаг «Commit» в задачах не выполняем автоматически**, а оставляем как отметку готовности задачи (CLAUDE.md, п. 8). Реально запускать `git commit` не нужно; отметить чекбокс и сообщить пользователю.
- Единица измерения справочника, приоритеты, focus/work-type — существующие доменные типы, не расширяются.

---

## Файловая структура

**Backend (создать/изменить):**

- `internal/store/goals/copy.go` — **создать**: `CopyGoalInput`, `GoalRepository.CopyGoal` (deep-copy в одной tx).
- `internal/store/goals/copy_test.go` — **создать**: DB-backed тесты deep-copy.
- `internal/domain/models.go` — **изменить**: добавить `ActionGoalCopied`, `ActionGoalMoved`.
- `internal/service/service.go` — **изменить**: добавить `CopyGoal` в интерфейс `GoalRepo` (стр. 47-66), новые ошибки, метод `Service.CopyGoal`, типы `CopyGoalMode`/`CopyGoalParams`.
- `internal/service/copy_goal_test.go` — **создать**: фейк-тесты логики сервиса.
- `internal/service/goal_test.go` — **изменить**: добавить метод `CopyGoal` в `goalFakeStore`.
- `internal/http/handlers/api/v1/goals/handler.go` — **изменить**: `HandleTransferGoal`.
- `internal/http/handlers/api/v1/goals/routes.go` — **изменить**: зарегистрировать роут.

**Frontend (изменить):**

- `internal/web/static/tracker.js` — доработать `TeamCombobox`; добавить `TransferGoalModal`; добавить пункт в `ExportMenu`.
- `internal/web/static/tracker.css` — стили модалки/тумблеров/заблокированных опций.

**Docs (изменить):**

- `specs/040-api-contract.md`, `specs/030-user-flows.md`, `specs/050-permissions-and-lifecycle.md`, `specs/020-domain-model.md`.

---

## Task 1: Store — `GoalRepository.CopyGoal` (атомарный deep-copy)

**Files:**
- Create: `internal/store/goals/copy.go`
- Create: `internal/store/goals/copy_test.go`

**Interfaces:**
- Consumes: `*GoalRepository` (поле `db *pgxpool.Pool`, `krs *krs.KRRepository`), `domain.TenantScope`.
- Produces:
  ```go
  type CopyGoalInput struct {
      SourceGoalID   int64
      TargetTeamID   int64
      TargetPeriodID int64
      WithProgress   bool
      WithComments   bool
  }
  func (r *GoalRepository) CopyGoal(ctx context.Context, scope domain.TenantScope, in CopyGoalInput) (newGoalID int64, err error)
  ```

- [ ] **Step 1: Написать падающий тест — базовая структура копируется**

Создать `internal/store/goals/copy_test.go`:

```go
package goals_test

import (
	"context"
	"testing"

	"okrs/internal/domain"
	"okrs/internal/store/goals"
	"okrs/internal/store/krs"
	"okrs/internal/store/testutil"
)

var copyScope = domain.TenantScope{TenantID: 1}

// Enum values are constructed via type conversion from their stored string form
// (priority "P0".."P3"; work_type "Delivery"/"Discovery"; focus UPPER_SNAKE) to avoid
// depending on exact Go constant names — see specs/020-domain-model.md.

func TestCopyGoalDuplicatesStructure(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	krRepo := krs.NewKRRepository(pool)
	repo := goals.NewGoalRepository(pool, krRepo)

	var srcTeam, dstTeam, period int64
	pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('Src') RETURNING id`).Scan(&srcTeam)
	pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('Dst') RETURNING id`).Scan(&dstTeam)
	pool.QueryRow(ctx, `INSERT INTO periods (name,start_date,end_date) VALUES ('P','2026-01-01','2026-12-31') RETURNING id`).Scan(&period)

	srcGoal, err := repo.CreateGoal(ctx, copyScope, goals.GoalInput{
		TeamID: srcTeam, PeriodID: period, Title: "Src goal", Description: "d",
		Priority: domain.Priority("P1"), Weight: 40, WorkType: domain.WorkType("Delivery"), FocusType: domain.FocusType("STABILITY"),
	})
	if err != nil {
		t.Fatalf("CreateGoal: %v", err)
	}
	krID, err := krRepo.CreateKeyResult(ctx, copyScope, krs.KeyResultInput{
		GoalID: srcGoal, Title: "KR", Description: "kd", Weight: 100, Kind: domain.KRKindNumerical,
	})
	if err != nil {
		t.Fatalf("CreateKeyResult: %v", err)
	}
	if err := krRepo.UpsertNumericalMeta(ctx, copyScope, krs.NumericalMetaInput{
		KeyResultID: krID, StartValue: 0, TargetValue: 100, CurrentValue: 55, Unit: "%",
	}); err != nil {
		t.Fatalf("UpsertNumericalMeta: %v", err)
	}

	newID, err := repo.CopyGoal(ctx, copyScope, goals.CopyGoalInput{
		SourceGoalID: srcGoal, TargetTeamID: dstTeam, TargetPeriodID: period,
		WithProgress: false, WithComments: false,
	})
	if err != nil {
		t.Fatalf("CopyGoal: %v", err)
	}
	if newID == srcGoal {
		t.Fatal("expected a new goal id, got the source id")
	}

	got, err := repo.GetGoal(ctx, copyScope, newID)
	if err != nil {
		t.Fatalf("GetGoal(new): %v", err)
	}
	if got.TeamID != dstTeam || got.PeriodID != period {
		t.Fatalf("target mismatch: team=%d period=%d", got.TeamID, got.PeriodID)
	}
	if got.Title != "Src goal" || got.Weight != 40 || got.Priority != domain.PriorityP1 {
		t.Fatalf("fields not copied: %+v", got)
	}
	if len(got.KeyResults) != 1 {
		t.Fatalf("expected 1 KR, got %d", len(got.KeyResults))
	}
	kr := got.KeyResults[0]
	if kr.Kind != domain.KRKindNumerical || kr.Numerical == nil {
		t.Fatalf("KR kind/meta not copied: %+v", kr)
	}
	// WithProgress=false → current reset to start_value (0).
	if kr.Numerical.CurrentValue != 0 || kr.Numerical.TargetValue != 100 {
		t.Fatalf("progress not reset: current=%v target=%v", kr.Numerical.CurrentValue, kr.Numerical.TargetValue)
	}
}
```

Удалить неиспользуемый `seedSourceGoal` stub перед первым запуском (оставлен как напоминание — заменяется инлайновым сидом выше; в финальном файле его быть не должно).

- [ ] **Step 2: Запустить тест — убедиться, что падает (нет метода CopyGoal)**

Run: `go test ./internal/store/goals/ -run TestCopyGoalDuplicatesStructure -v`
Expected: FAIL при компиляции — `repo.CopyGoal undefined` и `goals.CopyGoalInput` не объявлен.

- [ ] **Step 3: Реализовать `CopyGoal`**

Создать `internal/store/goals/copy.go`:

```go
package goals

import (
	"context"

	"okrs/internal/domain"
	"okrs/internal/store/krs"
)

// CopyGoalInput describes a deep-copy of a source goal into a target (team, period).
type CopyGoalInput struct {
	SourceGoalID   int64
	TargetTeamID   int64
	TargetPeriodID int64
	WithProgress   bool // carry KR progress (current_value / is_done / health_status) and KR notes
	WithComments   bool // carry goal comments (tasks + replies), authors and resolve state preserved
}

// CopyGoal deep-copies a goal (goal fields, all KRs with their meta, optionally KR
// notes/progress and goal comments) into the target team/period within one transaction.
// Shares are never copied. Returns the new goal id.
func (r *GoalRepository) CopyGoal(ctx context.Context, scope domain.TenantScope, in CopyGoalInput) (int64, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	// 1) New goal row, sort_order appended to the target board.
	var newGoalID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO goals (team_id, period_id, title, description, priority, weight, work_type, focus_type, owner_text, owner_udids, sort_order, tenant_id)
		SELECT $1, $2, g.title, g.description, g.priority, g.weight, g.work_type, g.focus_type, g.owner_text, g.owner_udids,
		       (SELECT COALESCE(MAX(sort_order),0)+1 FROM goals WHERE team_id=$1 AND period_id=$2 AND tenant_id=$4),
		       $4
		FROM goals g WHERE g.id=$3 AND g.tenant_id=$4
		RETURNING id`,
		in.TargetTeamID, in.TargetPeriodID, in.SourceGoalID, scope.TenantID,
	).Scan(&newGoalID); err != nil {
		return 0, err
	}

	// 2) Copy KRs (ordered), each with its meta.
	rows, err := tx.Query(ctx, `
		SELECT id, title, description, weight, kind, sort_order, zeroing_criteria, health_status,
		       start_value, target_value, current_value, unit, checkpoints
		FROM key_results WHERE goal_id=$1 AND tenant_id=$2 ORDER BY sort_order, id`,
		in.SourceGoalID, scope.TenantID)
	if err != nil {
		return 0, err
	}
	type srcKR struct {
		id                            int64
		title, description, kind      string
		weight, sortOrder             int
		zeroing                       *string
		health                        string
		start, target, current        *float64
		unit                          *string
		checkpoints                   []byte
	}
	var srcKRs []srcKR
	for rows.Next() {
		var k srcKR
		if err := rows.Scan(&k.id, &k.title, &k.description, &k.weight, &k.kind, &k.sortOrder, &k.zeroing, &k.health,
			&k.start, &k.target, &k.current, &k.unit, &k.checkpoints); err != nil {
			rows.Close()
			return 0, err
		}
		srcKRs = append(srcKRs, k)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	for _, k := range srcKRs {
		health := k.health
		if !in.WithProgress {
			health = "not_started" // default health when progress is not carried
		}
		var newKRID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO key_results (goal_id, title, description, zeroing_criteria, weight, kind, sort_order, health_status, tenant_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
			newGoalID, k.title, k.description, k.zeroing, k.weight, k.kind, k.sortOrder, health, scope.TenantID,
		).Scan(&newKRID); err != nil {
			return 0, err
		}

		switch domain.KRKind(k.kind) {
		case domain.KRKindNumerical:
			current := 0.0
			if k.start != nil {
				current = *k.start // reset → start
			}
			if in.WithProgress && k.current != nil {
				current = *k.current
			}
			if _, err := tx.Exec(ctx, `
				UPDATE key_results SET start_value=$1, target_value=$2, current_value=$3, unit=$4, checkpoints=$5
				WHERE id=$6 AND tenant_id=$7`,
				k.start, k.target, current, k.unit, k.checkpoints, newKRID, scope.TenantID); err != nil {
				return 0, err
			}
		case domain.KRKindBoolean:
			var done bool
			if err := tx.QueryRow(ctx, `SELECT COALESCE((SELECT is_done FROM kr_boolean_meta WHERE key_result_id=$1), false)`, k.id).Scan(&done); err != nil {
				return 0, err
			}
			if !in.WithProgress {
				done = false
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO kr_boolean_meta (key_result_id, is_done) VALUES ($1,$2)
				ON CONFLICT (key_result_id) DO UPDATE SET is_done=EXCLUDED.is_done`, newKRID, done); err != nil {
				return 0, err
			}
		case domain.KRKindProject:
			stageRows, err := tx.Query(ctx, `
				SELECT title, weight, is_done, sort_order FROM kr_project_stages WHERE key_result_id=$1 ORDER BY sort_order`, k.id)
			if err != nil {
				return 0, err
			}
			type stg struct {
				title  string
				weight int
				done   bool
				order  int
			}
			var stages []stg
			for stageRows.Next() {
				var s stg
				if err := stageRows.Scan(&s.title, &s.weight, &s.done, &s.order); err != nil {
					stageRows.Close()
					return 0, err
				}
				stages = append(stages, s)
			}
			stageRows.Close()
			if err := stageRows.Err(); err != nil {
				return 0, err
			}
			for _, s := range stages {
				done := s.done
				if !in.WithProgress {
					done = false
				}
				if _, err := tx.Exec(ctx, `
					INSERT INTO kr_project_stages (key_result_id, title, weight, is_done, sort_order)
					VALUES ($1,$2,$3,$4,$5)`, newKRID, s.title, s.weight, done, s.order); err != nil {
					return 0, err
				}
			}
		}

		// KR note travels with progress.
		if in.WithProgress {
			if _, err := tx.Exec(ctx, `
				INSERT INTO key_result_notes (key_result_id, text, author_user_id, updated_at, tenant_id)
				SELECT $1, n.text, n.author_user_id, n.updated_at, $3
				FROM key_result_notes n WHERE n.key_result_id=$2 AND n.tenant_id=$3`,
				newKRID, k.id, scope.TenantID); err != nil {
				return 0, err
			}
		}
	}

	// 3) Optionally copy comments (tasks first, then replies with remapped parent_id).
	if in.WithComments {
		if err := copyGoalComments(ctx, tx, scope, in.SourceGoalID, newGoalID); err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return newGoalID, nil
}
```

Добавить хелпер `copyGoalComments` в тот же файл (принимает `pgx.Tx`; импортировать `github.com/jackc/pgx/v5`):

```go
func copyGoalComments(ctx context.Context, tx pgx.Tx, scope domain.TenantScope, srcGoalID, dstGoalID int64) error {
	// Insert tasks (parent_id IS NULL), keep old→new id map, then insert replies.
	rows, err := tx.Query(ctx, `
		SELECT id, text, author_user_id, created_at, resolved_at, resolved_by_user_id
		FROM goal_comments WHERE goal_id=$1 AND tenant_id=$2 AND parent_id IS NULL
		ORDER BY created_at, id`, srcGoalID, scope.TenantID)
	if err != nil {
		return err
	}
	type task struct {
		id         int64
		text       string
		author     int64
		createdAt  time.Time
		resolvedAt *time.Time
		resolvedBy *int64
	}
	var tasks []task
	for rows.Next() {
		var t task
		if err := rows.Scan(&t.id, &t.text, &t.author, &t.createdAt, &t.resolvedAt, &t.resolvedBy); err != nil {
			rows.Close()
			return err
		}
		tasks = append(tasks, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	idMap := make(map[int64]int64, len(tasks))
	for _, t := range tasks {
		var newID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO goal_comments (goal_id, parent_id, text, author_user_id, created_at, resolved_at, resolved_by_user_id, tenant_id)
			VALUES ($1, NULL, $2, $3, $4, $5, $6, $7) RETURNING id`,
			dstGoalID, t.text, t.author, t.createdAt, t.resolvedAt, t.resolvedBy, scope.TenantID).Scan(&newID); err != nil {
			return err
		}
		idMap[t.id] = newID
	}
	replyRows, err := tx.Query(ctx, `
		SELECT parent_id, text, author_user_id, created_at
		FROM goal_comments WHERE goal_id=$1 AND tenant_id=$2 AND parent_id IS NOT NULL
		ORDER BY created_at, id`, srcGoalID, scope.TenantID)
	if err != nil {
		return err
	}
	type reply struct {
		parent    int64
		text      string
		author    int64
		createdAt time.Time
	}
	var replies []reply
	for replyRows.Next() {
		var rp reply
		if err := replyRows.Scan(&rp.parent, &rp.text, &rp.author, &rp.createdAt); err != nil {
			replyRows.Close()
			return err
		}
		replies = append(replies, rp)
	}
	replyRows.Close()
	if err := replyRows.Err(); err != nil {
		return err
	}
	for _, rp := range replies {
		newParent, ok := idMap[rp.parent]
		if !ok {
			continue // orphan guard; single-level depth guarantees a parent task exists
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO goal_comments (goal_id, parent_id, text, author_user_id, created_at, tenant_id)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			dstGoalID, newParent, rp.text, rp.author, rp.createdAt, scope.TenantID); err != nil {
			return err
		}
	}
	return nil
}
```

Добавить импорты в `copy.go`: `context`, `time`, `okrs/internal/domain`, `okrs/internal/store/krs` (для типов из пакета, если понадобится — иначе убрать), `github.com/jackc/pgx/v5`. Проверить, что `domain.KRHealthNotStarted` — верное имя константы (если в домене она называется иначе, напр. `KRHealthStatusNotStarted`, использовать фактическое; см. `grep -n "not_started" internal/domain/models.go`).

- [ ] **Step 4: Запустить тест — должен пройти**

Run: `go test ./internal/store/goals/ -run TestCopyGoalDuplicatesStructure -v`
Expected: PASS. Если `domain.KRHealthNotStarted` не компилируется — заменить на фактическое имя константы `not_started` из `internal/domain/models.go`.

- [ ] **Step 5: Добавить тест — прогресс/заметки и комментарии переносятся при флагах ON**

Дописать в `copy_test.go`:

```go
func TestCopyGoalCarriesProgressNotesAndComments(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	krRepo := krs.NewKRRepository(pool)
	repo := goals.NewGoalRepository(pool, krRepo)

	var team, period int64
	pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('T') RETURNING id`).Scan(&team)
	pool.QueryRow(ctx, `INSERT INTO periods (name,start_date,end_date) VALUES ('P','2026-01-01','2026-12-31') RETURNING id`).Scan(&period)

	srcGoal, _ := repo.CreateGoal(ctx, copyScope, goals.GoalInput{
		TeamID: team, PeriodID: period, Title: "G", Priority: domain.Priority("P2"), Weight: 10,
		WorkType: domain.WorkType("Delivery"), FocusType: domain.FocusType("STABILITY"),
	})
	krID, _ := krRepo.CreateKeyResult(ctx, copyScope, krs.KeyResultInput{GoalID: srcGoal, Title: "KR", Weight: 100, Kind: domain.KRKindNumerical})
	krRepo.UpsertNumericalMeta(ctx, copyScope, krs.NumericalMetaInput{KeyResultID: krID, StartValue: 0, TargetValue: 100, CurrentValue: 70, Unit: "%"})
	krRepo.UpsertKeyResultNote(ctx, copyScope, krID, "note text", 1)
	// A task + a reply.
	var taskID int64
	pool.QueryRow(ctx, `INSERT INTO goal_comments (goal_id, text, author_user_id, tenant_id) VALUES ($1,'task',1,1) RETURNING id`, srcGoal).Scan(&taskID)
	pool.Exec(ctx, `INSERT INTO goal_comments (goal_id, parent_id, text, author_user_id, tenant_id) VALUES ($1,$2,'reply',1,1)`, srcGoal, taskID)

	newID, err := repo.CopyGoal(ctx, copyScope, goals.CopyGoalInput{
		SourceGoalID: srcGoal, TargetTeamID: team, TargetPeriodID: period, WithProgress: true, WithComments: true,
	})
	if err != nil {
		t.Fatalf("CopyGoal: %v", err)
	}
	got, _ := repo.GetGoal(ctx, copyScope, newID)
	if got.KeyResults[0].Numerical.CurrentValue != 70 {
		t.Fatalf("progress not carried: %v", got.KeyResults[0].Numerical.CurrentValue)
	}
	if got.KeyResults[0].Note == nil || got.KeyResults[0].Note.Text != "note text" {
		t.Fatalf("note not carried: %+v", got.KeyResults[0].Note)
	}
	if len(got.Comments) != 1 || got.Comments[0].Text != "task" {
		t.Fatalf("comment task not carried: %+v", got.Comments)
	}
	if len(got.Comments[0].Replies) != 1 || got.Comments[0].Replies[0].Text != "reply" {
		t.Fatalf("reply not carried: %+v", got.Comments)
	}
}
```

(Проверить фактическое имя поля вложенных ответов в `domain.GoalComment` — `Replies`; если иное, поправить.)

- [ ] **Step 6: Запустить оба теста — должны пройти**

Run: `go test ./internal/store/goals/ -run 'TestCopyGoal' -v`
Expected: PASS (оба).

- [ ] **Step 7: Отметить задачу готовой** (коммит делает пользователь; см. Global Constraints)

```
Готово к коммиту: store CopyGoal + тесты.
Файлы: internal/store/goals/copy.go, internal/store/goals/copy_test.go
```

---

## Task 2: Service — `Service.CopyGoal` (правила, статус, активность, move)

**Files:**
- Modify: `internal/domain/models.go:259-278` (константы action)
- Modify: `internal/service/service.go:47-66` (интерфейс `GoalRepo`), `:170-176` (ошибки), новый метод
- Create: `internal/service/copy_goal_test.go`
- Modify: `internal/service/goal_test.go` (метод `CopyGoal` у `goalFakeStore`)

**Interfaces:**
- Consumes: `goals.CopyGoalInput`/`GoalRepository.CopyGoal` (Task 1); `s.statuses.GetTeamPeriodStatus`, `s.statuses.SetTeamPeriodStatus`, `s.goals.GetGoal`, `s.goals.DeleteGoal`, `s.goals.ListGoalsByTeamPeriod` (через `resetStatusIfNoGoals`), `s.recordActivity`.
- Produces:
  ```go
  type CopyGoalMode string
  const ( CopyGoalModeCopy CopyGoalMode = "copy"; CopyGoalModeMove CopyGoalMode = "move" )
  type CopyGoalParams struct {
      SourceGoalID, TargetTeamID, TargetPeriodID int64
      Mode         CopyGoalMode
      WithProgress bool
      WithComments bool
  }
  func (s *Service) CopyGoal(ctx context.Context, scope domain.TenantScope, p CopyGoalParams, actorUserID int64) (newGoalID int64, err error)
  // sentinel: service.ErrTransferTargetSameAsSource
  ```

- [ ] **Step 1: Добавить action-константы**

В `internal/domain/models.go` в блок `const (...)` (после `ActionReplyDeleted`, стр. ~277) добавить:

```go
	ActionGoalCopied ActivityAction = "goal_copied"
	ActionGoalMoved  ActivityAction = "goal_moved"
```

- [ ] **Step 2: Расширить интерфейс `GoalRepo` и добавить ошибку**

В `internal/service/service.go` в интерфейс `GoalRepo` (после `CreateGoal`, стр. 51) добавить строку:

```go
	CopyGoal(ctx context.Context, scope domain.TenantScope, in goals.CopyGoalInput) (int64, error)
```

В блок `var (...)` с ошибками (рядом с `ErrPeriodClosed`, стр. ~173) добавить:

```go
	ErrTransferTargetSameAsSource = errors.New("transfer target equals source team and period")
```

- [ ] **Step 3: Добавить `CopyGoal` в фейк-стор**

В `internal/service/goal_test.go` (рядом с `CreateGoal`, стр. 88) добавить метод и поле трекинга. Сначала в структуру `goalFakeStore` (стр. 18-44) добавить поле:

```go
	copyGoalCalls []goals.CopyGoalInput
```

Затем метод:

```go
func (f *goalFakeStore) CopyGoal(_ context.Context, _ domain.TenantScope, in goals.CopyGoalInput) (int64, error) {
	f.copyGoalCalls = append(f.copyGoalCalls, in)
	id := f.nextGoalID
	f.nextGoalID++
	return id, nil
}
```

- [ ] **Step 4: Написать падающий тест сервиса**

Создать `internal/service/copy_goal_test.go`:

```go
package service

import (
	"context"
	"errors"
	"testing"

	"okrs/internal/domain"
)

func TestCopyGoalCopyFlipsTargetStatusAndRecordsCopied(t *testing.T) {
	gf := newGoalFakeStore()
	gf.goals[10] = domain.Goal{ID: 10, TeamID: 1, PeriodID: 100, Title: "G"}
	// target (team 2, period 200) has no status → NoGoals → should flip to Forming.
	svc := newGoalTestService(gf)

	newID, err := svc.CopyGoal(context.Background(), domain.TenantScope{TenantID: 1}, CopyGoalParams{
		SourceGoalID: 10, TargetTeamID: 2, TargetPeriodID: 200, Mode: CopyGoalModeCopy,
	}, 7)
	if err != nil {
		t.Fatalf("CopyGoal: %v", err)
	}
	if newID == 0 {
		t.Fatal("expected new id")
	}
	if len(gf.copyGoalCalls) != 1 || gf.copyGoalCalls[0].TargetTeamID != 2 || gf.copyGoalCalls[0].TargetPeriodID != 200 {
		t.Fatalf("CopyGoal store not called correctly: %+v", gf.copyGoalCalls)
	}
	// no source delete on copy
	if len(gf.deleteGoalCalls) != 0 {
		t.Fatalf("copy must not delete source, got %v", gf.deleteGoalCalls)
	}
	// status flip to forming on target
	flipped := false
	for _, c := range gf.setStatusCalls {
		if c.teamID == 2 && c.periodID == 200 && c.status == domain.TeamPeriodStatusForming {
			flipped = true
		}
	}
	if !flipped {
		t.Fatalf("expected target status flip to forming, calls=%+v", gf.setStatusCalls)
	}
}

func TestCopyGoalRejectsClosedTarget(t *testing.T) {
	gf := newGoalFakeStore()
	gf.goals[10] = domain.Goal{ID: 10, TeamID: 1, PeriodID: 100, Title: "G"}
	gf.statuses[[2]int64{2, 200}] = domain.TeamPeriodStatusInProgress
	svc := newGoalTestService(gf)

	_, err := svc.CopyGoal(context.Background(), domain.TenantScope{TenantID: 1}, CopyGoalParams{
		SourceGoalID: 10, TargetTeamID: 2, TargetPeriodID: 200, Mode: CopyGoalModeCopy,
	}, 7)
	if !errors.Is(err, ErrPeriodClosed) {
		t.Fatalf("expected ErrPeriodClosed, got %v", err)
	}
	if len(gf.copyGoalCalls) != 0 {
		t.Fatal("must not copy into a closed/in-progress target")
	}
}

func TestCopyGoalMoveDeletesSourceAndRejectsSamePair(t *testing.T) {
	gf := newGoalFakeStore()
	gf.goals[10] = domain.Goal{ID: 10, TeamID: 1, PeriodID: 100, Title: "G"}
	svc := newGoalTestService(gf)
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}

	// same pair rejected
	if _, err := svc.CopyGoal(ctx, scope, CopyGoalParams{
		SourceGoalID: 10, TargetTeamID: 1, TargetPeriodID: 100, Mode: CopyGoalModeMove,
	}, 7); !errors.Is(err, ErrTransferTargetSameAsSource) {
		t.Fatalf("expected ErrTransferTargetSameAsSource, got %v", err)
	}

	// real move deletes source
	if _, err := svc.CopyGoal(ctx, scope, CopyGoalParams{
		SourceGoalID: 10, TargetTeamID: 2, TargetPeriodID: 200, Mode: CopyGoalModeMove,
	}, 7); err != nil {
		t.Fatalf("move: %v", err)
	}
	if len(gf.deleteGoalCalls) != 1 || gf.deleteGoalCalls[0] != 10 {
		t.Fatalf("move must hard-delete source, got %v", gf.deleteGoalCalls)
	}
}
```

- [ ] **Step 5: Запустить тесты — падают (нет метода CopyGoal)**

Run: `go test ./internal/service/ -run 'TestCopyGoal' -v`
Expected: FAIL — `svc.CopyGoal undefined`.

- [ ] **Step 6: Реализовать `Service.CopyGoal`**

В `internal/service/service.go` рядом с `CreateGoal` (после стр. 1534) добавить:

```go
// CopyGoalMode selects copy (keep source) or move (copy then hard-delete source).
type CopyGoalMode string

const (
	CopyGoalModeCopy CopyGoalMode = "copy"
	CopyGoalModeMove CopyGoalMode = "move"
)

// CopyGoalParams are the inputs for CopyGoal.
type CopyGoalParams struct {
	SourceGoalID   int64
	TargetTeamID   int64
	TargetPeriodID int64
	Mode           CopyGoalMode
	WithProgress   bool
	WithComments   bool
}

// CopyGoal copies (or moves) a goal into a target team/period.
// It rejects a target whose team period status is InProgress/Closed (ErrPeriodClosed),
// and a move whose target equals the source pair (ErrTransferTargetSameAsSource).
// Shares are never carried. On move, the source is hard-deleted (cascade).
func (s *Service) CopyGoal(ctx context.Context, scope domain.TenantScope, p CopyGoalParams, actorUserID int64) (int64, error) {
	src, err := s.goals.GetGoal(ctx, scope, p.SourceGoalID)
	if err != nil {
		return 0, err
	}
	if p.Mode == CopyGoalModeMove && p.TargetTeamID == src.TeamID && p.TargetPeriodID == src.PeriodID {
		return 0, ErrTransferTargetSameAsSource
	}
	targetStatus, err := s.statuses.GetTeamPeriodStatus(ctx, scope, p.TargetTeamID, p.TargetPeriodID)
	if err != nil {
		return 0, err
	}
	if targetStatus == domain.TeamPeriodStatusClosed || targetStatus == domain.TeamPeriodStatusInProgress {
		return 0, ErrPeriodClosed
	}

	newGoalID, err := s.goals.CopyGoal(ctx, scope, goals.CopyGoalInput{
		SourceGoalID:   p.SourceGoalID,
		TargetTeamID:   p.TargetTeamID,
		TargetPeriodID: p.TargetPeriodID,
		WithProgress:   p.WithProgress,
		WithComments:   p.WithComments,
	})
	if err != nil {
		return 0, err
	}

	if targetStatus == domain.TeamPeriodStatusNoGoals {
		if err := s.statuses.SetTeamPeriodStatus(ctx, scope, p.TargetTeamID, p.TargetPeriodID, domain.TeamPeriodStatusForming); err != nil {
			return 0, err
		}
	}

	action := domain.ActionGoalCopied
	if p.Mode == CopyGoalModeMove {
		action = domain.ActionGoalMoved
	}
	tt, tp, ng := p.TargetTeamID, p.TargetPeriodID, newGoalID
	s.recordActivity(ctx, scope, domain.ActivityEvent{
		ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: action,
		TeamID: &tt, PeriodID: &tp, GoalID: &ng, EntityTitle: src.Title,
		Payload: map[string]any{
			"source_goal_id":   src.ID,
			"source_team_id":   src.TeamID,
			"source_period_id": src.PeriodID,
			"with_progress":    p.WithProgress,
			"with_comments":    p.WithComments,
		},
	})

	if p.Mode == CopyGoalModeMove {
		if err := s.goals.DeleteGoal(ctx, scope, p.SourceGoalID); err != nil {
			return 0, err
		}
		_ = s.resetStatusIfNoGoals(ctx, scope, src.TeamID, src.PeriodID)
	}
	return newGoalID, nil
}
```

- [ ] **Step 7: Запустить тесты сервиса — проходят**

Run: `go test ./internal/service/ -run 'TestCopyGoal' -v`
Expected: PASS (все три).

- [ ] **Step 8: Прогнать пакеты целиком**

Run: `go build ./... && go vet ./internal/service/... ./internal/store/... && go test ./internal/service/... ./internal/store/goals/...`
Expected: успешная сборка и зелёные тесты.

- [ ] **Step 9: Отметить задачу готовой** (коммит — за пользователем)

---

## Task 3: HTTP — `POST /api/v1/goals/{goalID}/transfer`

**Files:**
- Modify: `internal/http/handlers/api/v1/goals/handler.go`
- Modify: `internal/http/handlers/api/v1/goals/routes.go`

**Interfaces:**
- Consumes: `service.CopyGoal`, `service.CopyGoalParams`, `service.CopyGoalMode{Copy,Move}`, `service.ErrPeriodClosed`, `service.ErrTransferTargetSameAsSource`; `auth.CanAccessTeamFromCtx`, `auth.TenantScopeFromContext`, `auth.UserIDFromContext`; `common.ParseID`; `v1.WriteError`, `v1.WriteJSON`.
- Produces: `Handler.HandleTransferGoal`, роут `POST /api/v1/goals/{goalID}/transfer`.

- [ ] **Step 1: Реализовать handler**

В `internal/http/handlers/api/v1/goals/handler.go` добавить метод (рядом с `HandleShareGoal`):

```go
func (h *Handler) HandleTransferGoal(w http.ResponseWriter, r *http.Request) {
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid goal id", map[string]string{"goal_id": "invalid"})
		return
	}
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	var req struct {
		Mode           string `json:"mode"`
		TargetTeamID   int64  `json:"target_team_id"`
		TargetPeriodID int64  `json:"target_period_id"`
		WithComments   bool   `json:"with_comments"`
		WithProgress   bool   `json:"with_progress"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	var mode service.CopyGoalMode
	switch req.Mode {
	case "copy":
		mode = service.CopyGoalModeCopy
	case "move":
		mode = service.CopyGoalModeMove
	default:
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid mode", map[string]string{"mode": "copy|move"})
		return
	}
	if req.TargetTeamID <= 0 {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid target_team_id", map[string]string{"target_team_id": "required"})
		return
	}
	if req.TargetPeriodID <= 0 {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid target_period_id", map[string]string{"target_period_id": "required"})
		return
	}
	// Source access: owner team must be reachable.
	goal, err := h.service.GetGoal(r.Context(), scope, goalID)
	if err != nil || !auth.CanAccessTeamFromCtx(r.Context(), goal.TeamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	// Target team access.
	if !auth.CanAccessTeamFromCtx(r.Context(), req.TargetTeamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "team not found", nil)
		return
	}
	newID, err := h.service.CopyGoal(r.Context(), scope, service.CopyGoalParams{
		SourceGoalID:   goalID,
		TargetTeamID:   req.TargetTeamID,
		TargetPeriodID: req.TargetPeriodID,
		Mode:           mode,
		WithProgress:   req.WithProgress,
		WithComments:   req.WithComments,
	}, auth.UserIDFromContext(r.Context()))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPeriodClosed):
			v1.WriteError(w, http.StatusConflict, "CONFLICT", "target team period is in progress or closed", nil)
		case errors.Is(err, service.ErrTransferTargetSameAsSource):
			v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "target equals source", map[string]string{"target": "same_as_source"})
		default:
			v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to transfer goal", nil)
		}
		return
	}
	v1.WriteJSON(w, http.StatusCreated, map[string]int64{"id": newID})
}
```

(`errors`, `encoding/json`, `service` уже импортированы в `handler.go`.)

- [ ] **Step 2: Зарегистрировать роут**

В `internal/http/handlers/api/v1/goals/routes.go` добавить строку (например после `share`, стр. 7):

```go
	r.Post("/api/v1/goals/{goalID}/transfer", h.HandleTransferGoal)
```

- [ ] **Step 3: Собрать и проверить**

Run: `go build ./... && go vet ./internal/http/...`
Expected: успешно.

- [ ] **Step 4: Проверить руками (endpoint отвечает)**

Запустить приложение (`/run` или как принято в проекте) и выполнить (подставив реальный `goalID`, целевые id и CSRF cookie/заголовок):

```bash
curl -sS -X POST "http://localhost:8080/api/v1/goals/1/transfer" \
  -H "Content-Type: application/json" -H "X-CSRF-Token: <token>" -b "okr_csrf_token=<token>; okr_session=<sid>" \
  -d '{"mode":"copy","target_team_id":2,"target_period_id":1,"with_comments":false,"with_progress":false}'
```

Expected: `201` с `{"id": <newGoalID>}`; повтор в `in_progress`/`closed` целевую пару → `409`; `mode` не copy/move → `400`.

- [ ] **Step 5: Отметить задачу готовой**

---

## Task 4: Frontend — доработать `TeamCombobox` (single-select, поиск по руководителю, блокировка)

**Files:**
- Modify: `internal/web/static/tracker.js` (`TeamCombobox`, стр. 484-544)

**Interfaces:**
- Consumes: `flattenTree`, `TEAM_TYPE_COLOR`, `TEAM_TYPE_LABEL`; узлы иерархии несут `id, name, type, depth, lead {display_name, udid}, status`.
- Produces: `TeamCombobox` с новыми пропами `single` (bool), `blockedIds` (Set/array id) и `blockedReason` (map id→строка). Существующие вызовы (шаринг) работают без изменений.

> Замечание: правки аддитивны — при отсутствии новых пропов поведение прежнее. Флоу шаринга (`GoalModal`, стр. ~2055) продолжает передавать только `selectedIds/onChange/excludeId/accent/allTeams`.

- [ ] **Step 1: Заменить фильтрацию и рендер опций на версию с lead-поиском и блокировкой**

Заменить тело `TeamCombobox` (стр. 484-544) на:

```jsx
function TeamCombobox({ selectedIds, onChange, excludeId, accent, allTeams, single = false, blockedIds = [], blockedReason = {} }) {
  const [q, setQ] = useState(''); const [open, setOpen] = useState(false); const [hi, setHi] = useState(0);
  const inputRef = useRef(); const wrapRef = useRef();
  const blocked = new Set(blockedIds || []);
  const flat = flattenTree(allTeams || []).filter(t => t.id !== excludeId);
  const ql = q.trim().toLowerCase();
  const matches = t => {
    const leadName = (t.lead && t.lead.display_name || '').toLowerCase();
    return t.name.toLowerCase().includes(ql) || leadName.includes(ql);
  };
  const filtered = ql ? flat.filter(matches) : flat;
  // In single mode the currently selected item stays visible in the list (as selected);
  // in multi mode selected items move to tags and are removed from the dropdown.
  const available = single ? filtered : filtered.filter(t => !selectedIds.includes(t.id));
  useEffect(() => { setHi(0); }, [q]);
  useEffect(() => {
    const h = e => { if (wrapRef.current && !wrapRef.current.contains(e.target)) setOpen(false); };
    document.addEventListener('mousedown', h);
    return () => document.removeEventListener('mousedown', h);
  }, []);
  const pick = t => {
    if (blocked.has(t.id)) return;
    if (single) { onChange([t.id]); setQ(''); setOpen(false); }
    else { onChange([...selectedIds, t.id]); setQ(''); inputRef.current?.focus(); }
  };
  const rem = id => onChange(selectedIds.filter(x => x !== id));
  const sel = selectedIds.map(id => flat.find(t => t.id === id)).filter(Boolean);
  const onKey = e => {
    if (e.key === 'ArrowDown') { e.preventDefault(); setOpen(true); setHi(h => Math.min(available.length - 1, h + 1)); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); setHi(h => Math.max(0, h - 1)); }
    else if (e.key === 'Enter') { e.preventDefault(); if (open && available[hi]) pick(available[hi]); }
    else if (e.key === 'Escape') { if (open) { e.preventDefault(); setOpen(false); } }
    else if (e.key === 'Backspace' && !q && !single && sel.length > 0) rem(sel[sel.length - 1].id);
  };
  const singleLabel = single && sel[0] ? `${TEAM_TYPE_LABEL[sel[0].type] || sel[0].type} · ${sel[0].name}` : '';
  return (
    <div ref={wrapRef} className="team-combobox">
      <div onClick={() => { setOpen(true); inputRef.current?.focus(); }}
        className={`team-combobox__input-area${open ? ' team-combobox__input-area--open' : ''}`}>
        {!single && sel.map(t => {
          const color = TEAM_TYPE_COLOR[t.type] || '#6b7280';
          return (
            <div key={t.id} className="team-combobox__tag" style={{ background: `${color}15`, border: `1px solid ${color}40` }}>
              <span className="team-combobox__tag-type" style={{ color }}>{TEAM_TYPE_LABEL[t.type] || t.type}</span>
              <span className="team-combobox__tag-name">{t.name}</span>
              <button onClick={e => { e.stopPropagation(); rem(t.id); }} className="team-combobox__tag-remove">×</button>
            </div>
          );
        })}
        <input ref={inputRef} value={q} onChange={e => { setQ(e.target.value); setOpen(true); }} onFocus={() => setOpen(true)} onKeyDown={onKey}
          placeholder={single ? (singleLabel || 'Найдите команду') : (sel.length ? 'Ещё…' : 'Найдите команду')}
          className="team-combobox__input" />
      </div>
      {open && (
        <div className="team-combobox__dropdown">
          {available.length === 0
            ? <div className="team-combobox__empty">{ql ? 'Не найдено' : 'Нет команд'}</div>
            : available.map((t, i) => {
              const color = TEAM_TYPE_COLOR[t.type] || '#6b7280';
              const isBlocked = blocked.has(t.id);
              const isSel = single && selectedIds.includes(t.id);
              return (
                <div key={t.id} onClick={() => pick(t)} onMouseEnter={() => setHi(i)}
                  className={`team-combobox__option${i === hi ? ' team-combobox__option--hi' : ''}${isBlocked ? ' team-combobox__option--blocked' : ''}${isSel ? ' team-combobox__option--selected' : ''}`}
                  style={{ padding: `7px 12px 7px ${8 + t.depth * 14}px` }}
                  title={isBlocked ? (blockedReason[t.id] || 'Недоступно в выбранном периоде') : ''}>
                  <div className="team-combobox__option-stripe" style={{ background: color }} />
                  <span className="team-combobox__option-type" style={{ color, background: `${color}12` }}>{TEAM_TYPE_LABEL[t.type] || t.type}</span>
                  <span className="team-combobox__option-name">{t.name}</span>
                  {t.lead && t.lead.display_name && <span className="team-combobox__option-lead">{t.lead.display_name}</span>}
                  {isBlocked && <span className="team-combobox__option-blocked-tag">{blockedReason[t.id] || 'недоступно'}</span>}
                </div>
              );
            })}
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Проверить регресс шаринга вручную**

Запустить приложение, открыть цель в редактируемом статусе (`forming`), включить «Общая цель», убедиться что мультиселект добавляет/удаляет команды как раньше и что поиск теперь находит команду также по имени руководителя.

Expected: шаринг работает без изменений; поиск по руководителю добавился.

- [ ] **Step 3: Отметить задачу готовой**

---

## Task 5: Frontend — `TransferGoalModal` + пункт меню + вызов API

**Files:**
- Modify: `internal/web/static/tracker.js` (новый компонент `TransferGoalModal`; пункт в `ExportMenu`, стр. 1410-1435)

**Interfaces:**
- Consumes: `apiGet`, `apiPost`; `PeriodSelect` (`period_select.js`); доработанный `TeamCombobox` (Task 4); `useModalClose`/`useOverlayClose` (см. существующие модалки); `flattenTree`.
- Produces: `TransferGoalModal({ goal, teamId, periodId, periods, allTeams, onClose, onDone })`; пункт «Перенести или скопировать» в `ExportMenu`.

> `ExportMenu` сейчас получает `goal, teamId, periodId, info`. Нужны также `periods` и `allTeams` для модалки — пробросить их от `GoalCard` (у карточки уже есть `allTeams`, `periodId`; `periods` пробросить от родителя, где рендерится доска). Если проброс `periods` требует изменения нескольких уровней, допустимо запросить периоды внутри модалки через `apiGet('/api/v1/periods')` при открытии — это read-only и дешёвый вызов. Ниже модалка грузит периоды сама, чтобы не менять сигнатуры по всей цепочке.

- [ ] **Step 1: Добавить компонент `TransferGoalModal`**

В `tracker.js` перед `ExportMenu` (стр. 1410) добавить:

```jsx
// TransferGoalModal copies or moves a goal into a chosen team + period.
function TransferGoalModal({ goal, teamId, periodId, allTeams, onClose, onDone }) {
  const [mode, setMode] = useState('copy'); // 'copy' | 'move'
  const [targetTeam, setTargetTeam] = useState(teamId);
  const [targetPeriod, setTargetPeriod] = useState(periodId);
  const [withComments, setWithComments] = useState(false);
  const [withProgress, setWithProgress] = useState(false);
  const [periods, setPeriods] = useState([]);
  const [hierarchy, setHierarchy] = useState(allTeams || []);
  const [loadingTeams, setLoadingTeams] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');

  useEffect(() => { apiGet('/api/v1/periods').then(setPeriods).catch(() => setPeriods([])); }, []);

  // Reload hierarchy for the selected target period to know per-team status blocking.
  useEffect(() => {
    let alive = true;
    setLoadingTeams(true);
    apiGet(`/api/v1/hierarchy?period_id=${targetPeriod}`)
      .then(data => { if (alive) setHierarchy(data.items || data || []); })
      .catch(() => { if (alive) setHierarchy([]); })
      .finally(() => { if (alive) setLoadingTeams(false); });
    return () => { alive = false; };
  }, [targetPeriod]);

  // Teams whose status in the target period is in_progress/closed are blocked.
  const flat = flattenTree(hierarchy || []);
  const blockedIds = flat.filter(t => t.status === 'in_progress' || t.status === 'closed').map(t => t.id);
  const blockedReason = {};
  flat.forEach(t => { if (t.status === 'in_progress') blockedReason[t.id] = 'в работе'; else if (t.status === 'closed') blockedReason[t.id] = 'закрыто'; });

  const sameAsSource = mode === 'move' && targetTeam === teamId && targetPeriod === periodId;
  const targetBlocked = blockedIds.includes(targetTeam);
  const canSubmit = !!targetTeam && !!targetPeriod && !sameAsSource && !targetBlocked && !busy;

  const requestClose = () => { if (!busy) onClose(); };
  useOverlayClose(requestClose);

  const submit = async () => {
    setBusy(true); setErr('');
    try {
      await apiPost(`/api/v1/goals/${goal.id}/transfer`, {
        mode, target_team_id: targetTeam, target_period_id: targetPeriod,
        with_comments: withComments, with_progress: withProgress,
      });
      onDone && onDone();
      onClose();
    } catch (e) {
      setErr('Не удалось выполнить операцию. Возможно, цели целевой команды уже в работе или закрыты.');
      setBusy(false);
    }
  };

  return (
    <div className="modal-overlay modal-overlay--z400" onMouseDown={e => { if (e.target === e.currentTarget) requestClose(); }}>
      <div className="modal-box modal-box--w600 transfer-modal">
        <div className="modal-header">
          <div>
            <div className="transfer-modal__title">Перенести или скопировать цель</div>
            <div className="transfer-modal__subtitle">{goal.title}</div>
          </div>
          <button className="modal-close" onClick={requestClose}>×</button>
        </div>
        <div className="modal-body">
          <div className="seg-group transfer-modal__mode">
            <button type="button" className={`seg-btn${mode === 'copy' ? ' seg-btn--active' : ''}`} onClick={() => setMode('copy')}>⧉ Копировать</button>
            <button type="button" className={`seg-btn${mode === 'move' ? ' seg-btn--active' : ''}`} onClick={() => setMode('move')}>➡ Перенести</button>
          </div>

          <div className="transfer-modal__field">
            <div className="transfer-modal__label">Куда — команда</div>
            <TeamCombobox single selectedIds={targetTeam ? [targetTeam] : []} onChange={ids => setTargetTeam(ids[0])}
              allTeams={hierarchy} blockedIds={blockedIds} blockedReason={blockedReason} />
            {loadingTeams && <div className="transfer-modal__hint">Загрузка команд…</div>}
          </div>

          <div className="transfer-modal__field">
            <div className="transfer-modal__label">Куда — период</div>
            <PeriodSelect periods={periods} periodId={targetPeriod} onChange={setTargetPeriod} />
          </div>

          <label className="transfer-modal__check">
            <input type="checkbox" checked={withComments} onChange={e => setWithComments(e.target.checked)} />
            <span>Перенести комментарии</span>
          </label>
          <label className="transfer-modal__check">
            <input type="checkbox" checked={withProgress} onChange={e => setWithProgress(e.target.checked)} />
            <span>Перенести прогресс и заметки KR</span>
          </label>

          {sameAsSource && <div className="transfer-modal__error">Перенос в ту же команду и период невозможен.</div>}
          {targetBlocked && !sameAsSource && <div className="transfer-modal__error">Цели выбранной команды в этом периоде уже в работе или закрыты.</div>}
          {err && <div className="transfer-modal__error">{err}</div>}
        </div>
        <div className="modal-footer">
          <button type="button" className="btn btn--ghost" onClick={requestClose}>Отмена</button>
          <button type="button" className="btn btn--primary" disabled={!canSubmit} onClick={submit}>
            {mode === 'move' ? 'Перенести' : 'Скопировать'}
          </button>
        </div>
      </div>
    </div>
  );
}
```

> Проверить фактические имена: класс кнопок (`btn btn--primary`/`btn--ghost`), z-модификатор оверлея (`modal-overlay--z400`) и наличие `useOverlayClose` в `tracker.js`. Если имена отличаются — привести к используемым в соседних модалках (`ExportModal`, `GoalModal`); функциональность та же.

- [ ] **Step 2: Добавить пункт в `ExportMenu`**

В `ExportMenu` (стр. 1411-1434) добавить состояние и пункт. Заменить объявление состояний (стр. 1412-1413) на:

```jsx
  const [open, setOpen] = useState(false);
  const [modal, setModal] = useState(false);
  const [transfer, setTransfer] = useState(false);
```

В `export-menu__dropdown` (после кнопки экспорта, перед `</div>` на стр. 1430) добавить:

```jsx
          <button type="button" className="export-menu__item" onClick={() => { setOpen(false); setTransfer(true); }}>
            <span className="export-menu__item-title">➡ Перенести или скопировать</span>
            <span className="export-menu__item-sub">в другую команду или период</span>
          </button>
```

Перед закрывающим `</div>` компонента (после строки с `{modal && <ExportModal ... />}`, стр. 1432) добавить:

```jsx
      {transfer && <TransferGoalModal goal={goal} teamId={teamId} periodId={periodId} allTeams={allTeams}
        onClose={() => setTransfer(false)} onDone={onReloadBoard} />}
```

Для этого пробросить в `ExportMenu` пропы `allTeams` и `onReloadBoard`:

- в сигнатуру `ExportMenu` (стр. 1411) добавить `allTeams, onReloadBoard`;
- в месте рендера `ExportMenu` внутри `GoalCard` (стр. 1512) передать их:

```jsx
          {exportInfo && <ExportMenu goal={goal} teamId={currentTeamId} periodId={periodId} info={exportInfo}
            allTeams={allTeams} onReloadBoard={onReload} />}
```

(`allTeams` и `onReload` уже доступны в пропсах `GoalCard`, стр. 1438.)

- [ ] **Step 3: Проверить вручную — happy path и блокировки**

Запустить приложение. На карточке цели открыть `···` → «Перенести или скопировать»:

1. Копирование в другой период той же команды (по умолчанию оба тумблера OFF) → `201`, новая цель появляется на доске целевого периода (переключить период, проверить чистый шаблон без прогресса/комментариев).
2. Включить оба тумблера, скопировать → прогресс/заметки/комментарии на месте.
3. Выбрать целевой период, где команда в `in_progress`/`closed` → команда заблокирована в списке (тултип «в работе»/«закрыто»), primary-кнопка недоступна.
4. Режим «Перенести» в ту же команду+период → предупреждение, кнопка недоступна; в другую пару → исходная цель исчезает с доски (`onReload`).

Expected: все сценарии как описано; ошибки сервера показываются инлайн.

- [ ] **Step 4: Отметить задачу готовой**

---

## Task 6: Frontend — стили модалки/тумблеров/заблокированных опций

**Files:**
- Modify: `internal/web/static/tracker.css`

**Interfaces:**
- Consumes: существующие классы `modal-overlay`, `modal-box`, `seg-group/seg-btn`, `team-combobox__*`.
- Produces: стили `.transfer-modal*`, `.team-combobox__option--blocked/--selected`, `.team-combobox__option-lead`.

- [ ] **Step 1: Добавить стили**

В `tracker.css` добавить блок (согласовать переменные/цвета с существующими — использовать те же токены, что рядом с `.export-menu`, `.seg-btn`):

```css
.transfer-modal__title { font-weight: 700; font-size: 18px; }
.transfer-modal__subtitle { color: #6b7280; font-size: 13px; margin-top: 2px; }
.transfer-modal__mode { margin-bottom: 16px; }
.transfer-modal__field { margin-bottom: 16px; }
.transfer-modal__label { font-weight: 600; font-size: 13px; margin-bottom: 6px; }
.transfer-modal__hint { color: #9ca3af; font-size: 12px; margin-top: 4px; }
.transfer-modal__check { display: flex; align-items: center; gap: 8px; padding: 6px 0; font-size: 14px; cursor: pointer; }
.transfer-modal__error { color: #dc2626; background: #fef2f2; border-radius: 8px; padding: 8px 10px; font-size: 13px; margin-top: 8px; }
.team-combobox__option-lead { color: #9ca3af; font-size: 12px; margin-left: 8px; }
.team-combobox__option--selected { background: rgba(99,102,241,0.08); }
.team-combobox__option--blocked { opacity: 0.5; cursor: not-allowed; }
.team-combobox__option-blocked-tag { margin-left: auto; font-size: 11px; color: #9ca3af; text-transform: lowercase; }
```

- [ ] **Step 2: Проверить визуально**

Открыть модалку, убедиться, что вёрстка согласована с остальными модалками (отступы, кнопки, сегмент-переключатель), заблокированные опции приглушены, у опций виден руководитель.

- [ ] **Step 3: Отметить задачу готовой**

---

## Task 7: Обновить нумерованные спеки

**Files:**
- Modify: `specs/040-api-contract.md`, `specs/030-user-flows.md`, `specs/050-permissions-and-lifecycle.md`, `specs/020-domain-model.md`

- [ ] **Step 1: `040-api-contract.md`** — в список write endpoints добавить `transfer goal — POST /api/v1/goals/{goalID}/transfer`; добавить подраздел с полным контрактом (method+path, request `{mode,target_team_id,target_period_id,with_comments,with_progress}`, validation, success `201 {"id"}`, errors `400/404/409`, idempotency: нет, side effects: новая цель + флип статуса; для move — удаление исходной + возможный сброс статуса). Формулировки взять из `docs/superpowers/specs/2026-08-14-copy-move-goal-design.md` §5.

- [ ] **Step 2: `030-user-flows.md`** — в разделе 5 «Работа с целью» добавить пункт про копирование/перенос: живёт на трекере, механика API mutation + hydration, точка входа `···`-меню, модалка `TransferGoalModal` (сегмент-переключатель, выбор команды/периода, два тумблера OFF по умолчанию), empty/error/loading, зависимость от `team period status` только на целевой стороне (сервер).

- [ ] **Step 3: `050-permissions-and-lifecycle.md`** — в блок «Требование к новым фичам» добавить подраздел «Копирование/перенос цели» с ответами на 5 обязательных вопросов (из §3 дизайн-дока).

- [ ] **Step 4: `020-domain-model.md`** — в перечислении `ActivityEvent.action` добавить `goal_copied`, `goal_moved` (категория `composition`).

- [ ] **Step 5: Проверить консистентность**

Run: `rg -n "transfer|goal_copied|goal_moved" specs/`
Expected: новые значения присутствуют в 040/050/020; 030 описывает модалку.

- [ ] **Step 6: Отметить задачу готовой**

---

## Финальная проверка (после всех задач)

- [ ] `go build ./...`
- [ ] `go vet ./...`
- [ ] `go test ./...` (store + service тесты зелёные; DB-тесты требуют поднятого Postgres через `testutil.SetupDB`)
- [ ] Ручная проверка UI-сценариев из Task 5, Step 3.
- [ ] Сообщить пользователю, что всё готово к его коммиту (сам коммит — за пользователем, CLAUDE.md п. 8).
