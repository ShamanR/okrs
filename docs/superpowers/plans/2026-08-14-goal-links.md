# Связанные цели (дерево целей) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Дать возможность привязывать цель к одной или нескольким родительским целям (из любых доступных команд и периодов) и показывать эти связи в списке целей — без влияния на расчёт прогресса.

**Architecture:** Новая M:N таблица `goal_links(tenant_id, child_goal_id, parent_goal_id)` с собственным репозиторием. Привязка — full-replace набора родителей дочерней цели (как `POST /goals/{id}/share`), с серверной проверкой tenant/scope/самоссылки/цикла. Доска (`/teams/{id}/okrs`) и `GET /goals/{id}` встраивают scope-фильтрованные `parents[]`/`children[]`. Фронтенд добавляет секцию «Связанные цели» в GoalModal и лейблы `↑N`/`↓N` + popover на карточке. Прогресс (`internal/okr`) не меняется.

**Tech Stack:** Go (pgx/v5, chi/v5), PostgreSQL (миграции `.up.sql`/`.down.sql`), браузерный React 18 + JSX через `@babel/standalone` (без сборщика), `internal/web/static/*.js|css`.

**Spec:** `docs/superpowers/specs/2026-08-14-goal-links-design.md`

## Global Constraints

- Слои не протекают: SQL только в `internal/store`, бизнес-логика только в `internal/service`, handlers без логики (`010-architecture-constraints.md`).
- Каждый repository — на одну сущность/агрегат; `store.Store` — composite-фабрика.
- Все запросы tenant-scoped: `tenant_id = scope.TenantID` присутствует в **каждом** SQL, включая обе части рекурсивного CTE.
- Никаких запросов в цикле (CLAUDE.md §9): проверка цикла — один CTE на операцию; вставка набора — один `INSERT ... SELECT unnest(...)`.
- Схема меняется только миграцией; `.up.sql` + `.down.sql` парой.
- State-changing HTTP (POST) проходит CSRF; ошибки в нормализованной форме `VALIDATION_ERROR|NOT_FOUND|CONFLICT|INTERNAL`.
- Идентификатор пользователя в публичном API — `udid`; целочисленный `id` не раскрывается.
- Прогресс НЕ агрегируется по связям (навигация-только); `internal/okr` не трогаем.
- Привязка НЕ зависит от `team period status` — разрешена во всех статусах; проверяется доступ к команде-владельцу дочерней цели.
- **Коммиты НЕ делаем** (CLAUDE.md §8) — шаги «Commit» ниже заменяем на `git add` + сообщение автору; пользователь коммитит сам. Каждый шаг «Commit» = «подготовить staged-изменения и остановиться на согласование».
- Не упоминать AI/ассистентов в коде/комментариях/доках (CLAUDE.md §5).
- Тексты дизайна/спек — на русском; интерфейс консистентный, переиспользуем общие компоненты (CLAUDE.md §11–13).

**Замечание про тесты store/service:** интеграционные тесты поднимают PostgreSQL (testcontainers) — следуй сетапу существующих файлов `internal/store/goals/goals_isolation_test.go` и `internal/service/goal_test.go` (helper'ы `store.New`, `testutil`, сидинг команд/периодов/целей). Ниже приведена только логика теста (Arrange/Act/Assert), boilerplate подключения БД копируй из соседних тестов того же пакета.

---

### Task 1: Миграция `043_goal_links` + доменные типы

**Files:**
- Create: `migrations/043_goal_links.up.sql`
- Create: `migrations/043_goal_links.down.sql`
- Modify: `internal/domain/models.go` (добавить `GoalLink`, `GoalRef`, поля `Parents`/`Children` в `Goal`, action-константы)
- Test: `internal/domain/models_test.go` (только если в пакете уже есть тест-файл; иначе пропустить — типы проверяются в Task 2)

**Interfaces:**
- Produces:
  - таблица `goal_links(tenant_id bigint, child_goal_id bigint, parent_goal_id bigint, created_at timestamptz)`;
  - `domain.GoalLink{ TenantID, ChildGoalID, ParentGoalID int64; CreatedAt time.Time }`;
  - `domain.GoalRef{ ID int64; Title string; PeriodID int64; PeriodName string; TeamID int64; TeamName string; TeamType string; Progress int }`;
  - `domain.Goal.Parents []GoalRef`, `domain.Goal.Children []GoalRef`;
  - `domain.ActionGoalLinked ActivityAction = "goal_linked"`, `domain.ActionGoalUnlinked ActivityAction = "goal_unlinked"`.

- [ ] **Step 1: Написать `043_goal_links.up.sql`**

```sql
CREATE TABLE IF NOT EXISTS goal_links (
    tenant_id      BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    child_goal_id  BIGINT NOT NULL REFERENCES goals(id)   ON DELETE CASCADE,
    parent_goal_id BIGINT NOT NULL REFERENCES goals(id)   ON DELETE CASCADE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, child_goal_id, parent_goal_id),
    CONSTRAINT goal_links_no_self CHECK (child_goal_id <> parent_goal_id)
);

-- Обход "детей" (по parent) и "родителей" (по child) — оба tenant-scoped.
CREATE INDEX IF NOT EXISTS idx_goal_links_parent ON goal_links (tenant_id, parent_goal_id);
CREATE INDEX IF NOT EXISTS idx_goal_links_child  ON goal_links (tenant_id, child_goal_id);
```

- [ ] **Step 2: Написать `043_goal_links.down.sql`**

```sql
DROP TABLE IF EXISTS goal_links;
```

- [ ] **Step 3: Добавить доменные типы в `internal/domain/models.go`**

Рядом с `type Goal struct` добавить поля (в конец структуры, после `Comments`):

```go
	Parents  []GoalRef
	Children []GoalRef
```

Ниже (например, около других goal-типов) добавить:

```go
// GoalRef — компактная сводка связанной цели для лейблов/popover (вычисляется на чтении).
type GoalRef struct {
	ID         int64
	Title      string
	PeriodID   int64
	PeriodName string
	TeamID     int64
	TeamName   string
	TeamType   string
	Progress   int
}

// GoalLink — ребро графа связей "дочерняя → родительская".
type GoalLink struct {
	TenantID     int64
	ChildGoalID  int64
	ParentGoalID int64
	CreatedAt    time.Time
}
```

- [ ] **Step 4: Добавить action-константы к блоку `ActivityAction` (около `ActionGoalShared`)**

```go
	ActionGoalLinked   ActivityAction = "goal_linked"
	ActionGoalUnlinked ActivityAction = "goal_unlinked"
```

- [ ] **Step 5: Проверить компиляцию и применение миграции**

Run: `go build ./... && go vet ./internal/domain/...`
Expected: без ошибок. (Применение миграции к БД проверится в тестах Task 2.)

- [ ] **Step 6: Commit (staged, без коммита)**

```bash
git add migrations/043_goal_links.up.sql migrations/043_goal_links.down.sql internal/domain/models.go
# сообщение автору: "feat(goal-links): migration 043 + domain types"
```

---

### Task 2: Репозиторий `goallinks` — цикл-проверка, full-replace, чтение

**Files:**
- Create: `internal/store/goallinks/goallinks.go`
- Test: `internal/store/goallinks/goallinks_test.go`

**Interfaces:**
- Consumes: `domain.TenantScope`, `domain.GoalRef`, таблица `goal_links` (Task 1).
- Produces (методы `*GoalLinkRepository`):
  - `NewGoalLinkRepository(db *pgxpool.Pool) *GoalLinkRepository`
  - `ReplaceParents(ctx, scope domain.TenantScope, childID int64, parentIDs []int64) (added, removed []int64, err error)` — транзакция: диф текущего набора, cycle-check одним CTE, delete-all + insert-set; при цикле возвращает `ErrCycle`.
  - `ListLinksForGoals(ctx, scope domain.TenantScope, goalIDs []int64, allowedTeamIDs []int64, adminAll bool) (parents, children map[int64][]domain.GoalRef, err error)` — батч, scope-filtered.
  - `ListLinkable(ctx, scope domain.TenantScope, allowedTeamIDs []int64, adminAll bool, periodID *int64, excludeGoalID int64, q string) ([]domain.GoalRef, error)` — кандидаты для пикера (в `GoalRef` доп. поля не нужны, лид/path добавит service/handler? — нет: см. ниже, вернём расширенный тип).
- Produces (типы/ошибки):
  - `var ErrCycle = errors.New("goallinks: cycle")`
  - `type LinkableGoal struct { domain.GoalRef; Lead string; TeamPath string }`

- [ ] **Step 1: Написать падающий тест на full-replace и снятие всех связей**

`internal/store/goallinks/goallinks_test.go` (сетап БД — по образцу `internal/store/goals/goals_isolation_test.go`):

```go
func TestReplaceParents_SetAndClear(t *testing.T) {
	ctx, db := setupTestDB(t) // helper из пакета-соседа: поднимает PG, применяет миграции
	scope := domain.TenantScope{TenantID: 1}
	// Arrange: тенант 1, команда, период, 3 цели c1(child), p1, p2 (helper seedGoal).
	c1 := seedGoal(t, ctx, db, scope, "child")
	p1 := seedGoal(t, ctx, db, scope, "p1")
	p2 := seedGoal(t, ctx, db, scope, "p2")
	repo := goallinks.NewGoalLinkRepository(db)

	added, removed, err := repo.ReplaceParents(ctx, scope, c1, []int64{p1, p2})
	require.NoError(t, err)
	require.ElementsMatch(t, []int64{p1, p2}, added)
	require.Empty(t, removed)

	// Замена на {p1}: p2 снимается.
	added, removed, err = repo.ReplaceParents(ctx, scope, c1, []int64{p1})
	require.NoError(t, err)
	require.Empty(t, added)
	require.ElementsMatch(t, []int64{p2}, removed)

	// Пустой набор снимает всё.
	_, removed, err = repo.ReplaceParents(ctx, scope, c1, []int64{})
	require.NoError(t, err)
	require.ElementsMatch(t, []int64{p1}, removed)
}
```

- [ ] **Step 2: Запустить — убедиться, что не компилируется/падает**

Run: `go test ./internal/store/goallinks/ -run TestReplaceParents_SetAndClear -v`
Expected: FAIL (пакет/тип `goallinks.NewGoalLinkRepository` не определён).

- [ ] **Step 3: Реализовать репозиторий (скелет + ReplaceParents с cycle-CTE)**

`internal/store/goallinks/goallinks.go`:

```go
package goallinks

import (
	"context"
	"errors"

	"okrs/internal/domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrCycle сообщает, что набор родителей замыкает цикл в графе связей.
var ErrCycle = errors.New("goallinks: cycle")

type GoalLinkRepository struct {
	db *pgxpool.Pool
}

func NewGoalLinkRepository(db *pgxpool.Pool) *GoalLinkRepository {
	return &GoalLinkRepository{db: db}
}

// ReplaceParents атомарно заменяет набор родителей дочерней цели childID.
// Цикл может создать только новое ребро C->Pi; удаление старых родителей C безопасно.
// Поэтому одним рекурсивным CTE считаем предков набора {Pi} по графу, ИСКЛЮЧИВ исходящие
// рёбра самого C (они заменяются). Если C среди предков или среди {Pi} — цикл/самоссылка.
func (r *GoalLinkRepository) ReplaceParents(ctx context.Context, scope domain.TenantScope, childID int64, parentIDs []int64) (added, removed []int64, err error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Дедуп + отсев самоссылки заранее (валидация принадлежности/scope — в service).
	seen := map[int64]bool{}
	uniq := make([]int64, 0, len(parentIDs))
	for _, p := range parentIDs {
		if p == childID {
			return nil, nil, ErrCycle // самоссылка
		}
		if !seen[p] {
			seen[p] = true
			uniq = append(uniq, p)
		}
	}

	// Текущий набор родителей — для диффа added/removed.
	before := map[int64]bool{}
	rows, err := tx.Query(ctx, `SELECT parent_goal_id FROM goal_links WHERE tenant_id=$1 AND child_goal_id=$2`, scope.TenantID, childID)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var p int64
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return nil, nil, err
		}
		before[p] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	// Cycle-check: достижим ли childID вверх по родителям из набора {uniq},
	// в графе без исходящих рёбер childID. CYCLE-клоза защищает от runaway.
	if len(uniq) > 0 {
		var hits int
		err = tx.QueryRow(ctx, `
			WITH RECURSIVE anc AS (
				SELECT gl.parent_goal_id AS goal_id
				FROM goal_links gl
				WHERE gl.tenant_id = $1
				  AND gl.child_goal_id = ANY($2)
				  AND gl.child_goal_id <> $3
				UNION
				SELECT gl.parent_goal_id
				FROM goal_links gl
				JOIN anc ON gl.child_goal_id = anc.goal_id
				WHERE gl.tenant_id = $1
				  AND gl.child_goal_id <> $3
			) CYCLE goal_id SET is_cycle USING path
			SELECT count(*) FROM anc WHERE goal_id = $3 OR goal_id = ANY($2)`,
			scope.TenantID, uniq, childID).Scan(&hits)
		if err != nil {
			return nil, nil, err
		}
		// goal_id = ANY(uniq) в anc означало бы, что один из родителей достижим как предок
		// другого + замыкает через child? Нет: проверяем именно childID. Оставляем только childID.
		var childHit int
		err = tx.QueryRow(ctx, `
			WITH RECURSIVE anc AS (
				SELECT gl.parent_goal_id AS goal_id
				FROM goal_links gl
				WHERE gl.tenant_id = $1 AND gl.child_goal_id = ANY($2) AND gl.child_goal_id <> $3
				UNION
				SELECT gl.parent_goal_id
				FROM goal_links gl JOIN anc ON gl.child_goal_id = anc.goal_id
				WHERE gl.tenant_id = $1 AND gl.child_goal_id <> $3
			) CYCLE goal_id SET is_cycle USING path
			SELECT count(*) FROM anc WHERE goal_id = $3`,
			scope.TenantID, uniq, childID).Scan(&childHit)
		if err != nil {
			return nil, nil, err
		}
		if childHit > 0 {
			return nil, nil, ErrCycle
		}
	}

	// Полная замена: удалить все исходящие рёбра childID, вставить новый набор.
	if _, err := tx.Exec(ctx, `DELETE FROM goal_links WHERE tenant_id=$1 AND child_goal_id=$2`, scope.TenantID, childID); err != nil {
		return nil, nil, err
	}
	if len(uniq) > 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO goal_links (tenant_id, child_goal_id, parent_goal_id)
			SELECT $1, $2, unnest($3::bigint[])`, scope.TenantID, childID, uniq); err != nil {
			return nil, nil, err
		}
	}

	for _, p := range uniq {
		if !before[p] {
			added = append(added, p)
		}
	}
	for p := range before {
		if !seen[p] {
			removed = append(removed, p)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	return added, removed, nil
}
```

> Примечание для реализующего: первый `hits`-запрос в черновике избыточен — оставь только второй (`childHit`). Он приведён, чтобы явно показать: проверяем достижимость **childID**, а не пересечение с набором родителей. Убери мёртвый первый запрос при реализации.

- [ ] **Step 4: Запустить тест full-replace**

Run: `go test ./internal/store/goallinks/ -run TestReplaceParents_SetAndClear -v`
Expected: PASS.

- [ ] **Step 5: Написать падающий тест на предотвращение циклов**

```go
func TestReplaceParents_RejectsCycles(t *testing.T) {
	ctx, db := setupTestDB(t)
	scope := domain.TenantScope{TenantID: 1}
	a := seedGoal(t, ctx, db, scope, "A")
	b := seedGoal(t, ctx, db, scope, "B")
	c := seedGoal(t, ctx, db, scope, "C")
	repo := goallinks.NewGoalLinkRepository(db)

	// A -> B (A дочерняя к B)
	_, _, err := repo.ReplaceParents(ctx, scope, a, []int64{b})
	require.NoError(t, err)
	// B -> A замкнёт цикл A->B->A
	_, _, err = repo.ReplaceParents(ctx, scope, b, []int64{a})
	require.ErrorIs(t, err, goallinks.ErrCycle)

	// Транзитивный: B -> C ок, затем C -> A замыкает A->B->C->A
	_, _, err = repo.ReplaceParents(ctx, scope, b, []int64{c})
	require.NoError(t, err)
	_, _, err = repo.ReplaceParents(ctx, scope, c, []int64{a})
	require.ErrorIs(t, err, goallinks.ErrCycle)

	// Самоссылка
	_, _, err = repo.ReplaceParents(ctx, scope, a, []int64{a})
	require.ErrorIs(t, err, goallinks.ErrCycle)
}
```

- [ ] **Step 6: Запустить тест циклов**

Run: `go test ./internal/store/goallinks/ -run TestReplaceParents_RejectsCycles -v`
Expected: PASS (реализация из Step 3 уже покрывает).

- [ ] **Step 7: Реализовать `ListLinksForGoals` + падающий тест на scope-фильтрацию**

Тест:

```go
func TestListLinksForGoals_ScopeFiltered(t *testing.T) {
	ctx, db := setupTestDB(t)
	scope := domain.TenantScope{TenantID: 1}
	teamA := seedTeam(t, ctx, db, scope, "A", "cluster")
	teamB := seedTeam(t, ctx, db, scope, "B", "unit")
	child := seedGoalInTeam(t, ctx, db, scope, teamA, "child")
	parent := seedGoalInTeam(t, ctx, db, scope, teamB, "parent")
	repo := goallinks.NewGoalLinkRepository(db)
	_, _, err := repo.ReplaceParents(ctx, scope, child, []int64{parent})
	require.NoError(t, err)

	// Читатель со scope только на teamA: parent (в teamB) НЕ виден.
	parents, children, err := repo.ListLinksForGoals(ctx, scope, []int64{child, parent}, []int64{teamA}, false)
	require.NoError(t, err)
	require.Empty(t, parents[child])        // parent вне scope — скрыт
	require.Empty(t, children[parent])      // child в teamA доступен, но parent вне scope → его страница вне выборки goalIDs все равно; проверяем child-side
	// Читатель со scope на обе команды: связь видна с обеих сторон.
	parents, children, err = repo.ListLinksForGoals(ctx, scope, []int64{child, parent}, []int64{teamA, teamB}, false)
	require.NoError(t, err)
	require.Len(t, parents[child], 1)
	require.Equal(t, parent, parents[child][0].ID)
	require.Len(t, children[parent], 1)
	require.Equal(t, child, children[parent][0].ID)
	require.Equal(t, "B", parents[child][0].TeamName)
}
```

Реализация метода:

```go
// ListLinksForGoals возвращает для каждого goalID его родителей и детей (сводки GoalRef),
// отфильтрованных по доступным командам (adminAll=true — все команды тенанта).
// Связи считаются по owner-команде связанной цели (goals.team_id).
func (r *GoalLinkRepository) ListLinksForGoals(ctx context.Context, scope domain.TenantScope, goalIDs, allowedTeamIDs []int64, adminAll bool) (parents, children map[int64][]domain.GoalRef, err error) {
	parents = map[int64][]domain.GoalRef{}
	children = map[int64][]domain.GoalRef{}
	if len(goalIDs) == 0 {
		return parents, children, nil
	}
	// Родители: для каждой цели из goalIDs — её parent-цели.
	// $4 = adminAll, $5 = allowedTeamIDs (используется только когда adminAll=false).
	pr, err := r.db.Query(ctx, `
		SELECT gl.child_goal_id, pg.id, pg.title, pg.period_id, pp.name, pg.team_id, pt.name, pt.type, pg.progress_cache
		FROM goal_links gl
		JOIN goals   pg ON pg.id = gl.parent_goal_id AND pg.tenant_id = gl.tenant_id
		JOIN teams   pt ON pt.id = pg.team_id
		JOIN periods pp ON pp.id = pg.period_id
		WHERE gl.tenant_id = $1 AND gl.child_goal_id = ANY($2)
		  AND ($3 OR pg.team_id = ANY($4))
		ORDER BY pt.type, pt.name, pg.sort_order, pg.id`,
		scope.TenantID, goalIDs, adminAll, allowedTeamIDs)
	if err != nil {
		return nil, nil, err
	}
	if err := scanRefs(pr, parents); err != nil {
		return nil, nil, err
	}
	// Дети: для каждой цели из goalIDs — её child-цели.
	cr, err := r.db.Query(ctx, `
		SELECT gl.parent_goal_id, cg.id, cg.title, cg.period_id, cp.name, cg.team_id, ct.name, ct.type, cg.progress_cache
		FROM goal_links gl
		JOIN goals   cg ON cg.id = gl.child_goal_id AND cg.tenant_id = gl.tenant_id
		JOIN teams   ct ON ct.id = cg.team_id
		JOIN periods cp ON cp.id = cg.period_id
		WHERE gl.tenant_id = $1 AND gl.parent_goal_id = ANY($2)
		  AND ($3 OR cg.team_id = ANY($4))
		ORDER BY ct.type, ct.name, cg.sort_order, cg.id`,
		scope.TenantID, goalIDs, adminAll, allowedTeamIDs)
	if err != nil {
		return nil, nil, err
	}
	if err := scanRefs(cr, children); err != nil {
		return nil, nil, err
	}
	return parents, children, nil
}
```

> ВАЖНО про `progress_cache`: в `goals` НЕТ хранимого прогресса — прогресс вычисляется (`internal/okr`). Проверь схему `goals` перед реализацией: если хранимого столбца прогресса нет (так и есть), верни `Progress=0` из репозитория, а **прогресс проставит service-слой**, догрузив цели через `okr`-расчёт, ЛИБО (проще и консистентно) репозиторий возвращает id+заголовок+команда+период, а service дозаполняет `Progress` из уже посчитанных на доске целей / отдельным расчётом. Реши в service-слое (Task 4): для `parents/children` на доске прогресс связанной цели считается тем же `okr.GoalProgress`, что и для остальных. Убери `progress_cache` из SQL и оставь placeholder `0`, прогресс дозаполнит service.

`scanRefs` helper:

```go
func scanRefs(rows pgx.Rows, dst map[int64][]domain.GoalRef) error {
	defer rows.Close()
	for rows.Next() {
		var key int64
		var ref domain.GoalRef
		var progress int
		if err := rows.Scan(&key, &ref.ID, &ref.Title, &ref.PeriodID, &ref.PeriodName, &ref.TeamID, &ref.TeamName, &ref.TeamType, &progress); err != nil {
			return err
		}
		ref.Progress = progress
		dst[key] = append(dst[key], ref)
	}
	return rows.Err()
}
```

(Так как хранимого прогресса нет — убери 9-й столбец из SELECT и из Scan, `ref.Progress` оставь 0.)

- [ ] **Step 8: Запустить scope-тест**

Run: `go test ./internal/store/goallinks/ -run TestListLinksForGoals_ScopeFiltered -v`
Expected: PASS.

- [ ] **Step 9: Реализовать `ListLinkable` + тест поиска/периода/исключения**

Тест:

```go
func TestListLinkable_SearchPeriodExclude(t *testing.T) {
	ctx, db := setupTestDB(t)
	scope := domain.TenantScope{TenantID: 1}
	teamA := seedTeam(t, ctx, db, scope, "Платформа", "unit")
	self := seedGoalInTeam(t, ctx, db, scope, teamA, "self")
	other := seedGoalInTeam(t, ctx, db, scope, teamA, "Снизить Time-to-Deploy")
	repo := goallinks.NewGoalLinkRepository(db)

	// exclude self; поиск по названию цели.
	got, err := repo.ListLinkable(ctx, scope, []int64{teamA}, false, nil, self, "time-to-deploy")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, other, got[0].ID)

	// поиск по названию команды.
	got, err = repo.ListLinkable(ctx, scope, []int64{teamA}, false, nil, self, "платформа")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(got), 1)
}
```

Реализация:

```go
type LinkableGoal struct {
	domain.GoalRef
	Lead     string
	TeamPath string // зарезервировано; можно пусто на этом этапе
}

// ListLinkable возвращает цели-кандидаты в родители: доступные команды тенанта,
// опционально фильтр периода, исключая excludeGoalID, с поиском q по названию цели/команды/лида.
func (r *GoalLinkRepository) ListLinkable(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, adminAll bool, periodID *int64, excludeGoalID int64, q string) ([]LinkableGoal, error) {
	like := "%" + strings.ToLower(strings.TrimSpace(q)) + "%"
	rows, err := r.db.Query(ctx, `
		SELECT g.id, g.title, g.period_id, p.name, g.team_id, t.name, t.type, COALESCE(t.lead,'')
		FROM goals g
		JOIN teams   t ON t.id = g.team_id
		JOIN periods p ON p.id = g.period_id
		WHERE g.tenant_id = $1
		  AND g.id <> $2
		  AND ($3 OR g.team_id = ANY($4))
		  AND ($5::bigint IS NULL OR g.period_id = $5)
		  AND ($6 = '' OR lower(g.title) LIKE $7 OR lower(t.name) LIKE $7 OR lower(COALESCE(t.lead,'')) LIKE $7)
		ORDER BY t.type, t.name, g.sort_order, g.id`,
		scope.TenantID, excludeGoalID, adminAll, allowedTeamIDs, periodID, strings.TrimSpace(q), like)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]LinkableGoal, 0)
	for rows.Next() {
		var lg LinkableGoal
		if err := rows.Scan(&lg.ID, &lg.Title, &lg.PeriodID, &lg.PeriodName, &lg.TeamID, &lg.TeamName, &lg.TeamType, &lg.Lead); err != nil {
			return nil, err
		}
		out = append(out, lg)
	}
	return out, rows.Err()
}
```

(Добавь `"strings"` и `"github.com/jackc/pgx/v5"` в импорты.)

- [ ] **Step 10: Запустить весь пакет**

Run: `go test ./internal/store/goallinks/ -v`
Expected: PASS (все тесты).

- [ ] **Step 11: Тест tenant-изоляции + каскада удаления**

```go
func TestGoalLinks_TenantIsolationAndCascade(t *testing.T) {
	ctx, db := setupTestDB(t)
	s1 := domain.TenantScope{TenantID: 1}
	child := seedGoal(t, ctx, db, s1, "child")
	parent := seedGoal(t, ctx, db, s1, "parent")
	repo := goallinks.NewGoalLinkRepository(db)
	_, _, err := repo.ReplaceParents(ctx, s1, child, []int64{parent})
	require.NoError(t, err)

	// Из другого тенанта связей не видно.
	s2 := domain.TenantScope{TenantID: 2}
	parents, _, err := repo.ListLinksForGoals(ctx, s2, []int64{child}, nil, true)
	require.NoError(t, err)
	require.Empty(t, parents[child])

	// Удаление родителя каскадно чистит связь.
	deleteGoal(t, ctx, db, s1, parent)
	parents, _, err = repo.ListLinksForGoals(ctx, s1, []int64{child}, nil, true)
	require.NoError(t, err)
	require.Empty(t, parents[child])
}
```

Run: `go test ./internal/store/goallinks/ -run TestGoalLinks_TenantIsolationAndCascade -v`
Expected: PASS (каскад обеспечен FK `ON DELETE CASCADE`).

- [ ] **Step 12: Commit (staged)**

```bash
git add internal/store/goallinks/
# сообщение автору: "feat(goal-links): goallinks repository (replace, cycle check, list, linkable)"
```

---

### Task 3: Подключить `GoalLinkRepository` в composite `store.Store`

**Files:**
- Modify: `internal/store/store.go` (импорт, поле `GoalLinks`, инициализация в `New`)
- Test: `internal/store/store_test.go` (добавить проверку, что поле не nil — если файл есть)

**Interfaces:**
- Consumes: `goallinks.NewGoalLinkRepository` (Task 2).
- Produces: `store.Store.GoalLinks *goallinks.GoalLinkRepository`.

- [ ] **Step 1: Добавить импорт и поле**

В `import (...)` добавить `"okrs/internal/store/goallinks"`. В `type Store struct` добавить (рядом с `Goals`):

```go
	GoalLinks *goallinks.GoalLinkRepository
```

- [ ] **Step 2: Инициализировать в `New`**

В литерале `&Store{...}` добавить:

```go
		GoalLinks: goallinks.NewGoalLinkRepository(db),
```

- [ ] **Step 3: Проверить сборку**

Run: `go build ./...`
Expected: без ошибок.

- [ ] **Step 4: Commit (staged)**

```bash
git add internal/store/store.go
# сообщение автору: "feat(goal-links): wire GoalLinks into store composite"
```

---

### Task 4: Service — интерфейс, валидация, активность, чтение

**Files:**
- Modify: `internal/service/service.go` (интерфейс `GoalLinkRepo`, поля `Deps.GoalLinks`/`Service.goalLinks`, конструктор `New`, `NewFromStore`/фабрика — там, где собираются `Deps` из `store.Store`)
- Create: `internal/service/goal_links.go` (методы `SetGoalParents`, `ListLinksForGoals`, `ListLinkableGoals`)
- Create: `internal/service/goal_links_test.go`

**Interfaces:**
- Consumes: `store.GoalLinks` (Task 3), `goallinks.ErrCycle`, `goallinks.LinkableGoal`, `GoalRepo.ListGoalOwnerTeamIDs` (существует), `ActivityRepo.Record` (существует).
- Produces (методы `*Service`):
  - `SetGoalParents(ctx, scope domain.TenantScope, allowedTeamIDs []int64, adminAll bool, childID int64, parentIDs []int64, actorUserID int64) error` — ошибки `ErrGoalLinkNotAccessible`, `ErrGoalLinkSelf`, `ErrGoalLinkCycle`.
  - `ListLinksForGoals(ctx, scope, goalIDs, allowedTeamIDs []int64, adminAll bool) (parents, children map[int64][]domain.GoalRef, error)`.
  - `ListLinkableGoals(ctx, scope, allowedTeamIDs []int64, adminAll bool, periodID *int64, excludeGoalID int64, q string) ([]goallinks.LinkableGoal, error)`.
- Produces (ошибки): `ErrGoalLinkSelf`, `ErrGoalLinkNotAccessible`, `ErrGoalLinkCycle` (в `service.go` рядом с прочими `Err...`).

- [ ] **Step 1: Объявить интерфейс и ошибки, подключить в Deps/Service/New**

В `service.go` рядом с `GoalShareRepo` добавить:

```go
// GoalLinkRepo управляет связями цель↔цель (parent/child).
type GoalLinkRepo interface {
	ReplaceParents(ctx context.Context, scope domain.TenantScope, childID int64, parentIDs []int64) (added, removed []int64, err error)
	ListLinksForGoals(ctx context.Context, scope domain.TenantScope, goalIDs, allowedTeamIDs []int64, adminAll bool) (parents, children map[int64][]domain.GoalRef, err error)
	ListLinkable(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, adminAll bool, periodID *int64, excludeGoalID int64, q string) ([]goallinks.LinkableGoal, error)
}
```

Импортировать `"okrs/internal/store/goallinks"`. Добавить в `type Deps struct` поле `GoalLinks GoalLinkRepo`; в `type Service struct` — `goalLinks GoalLinkRepo`; в `New(deps Deps)` — `goalLinks: deps.GoalLinks`; в месте сборки `Deps` из `*store.Store` (там, где `Shares: st.Shares` и `Activity: st.Activity`) — `GoalLinks: st.GoalLinks`.

Рядом с прочими ошибками сервиса:

```go
var (
	ErrGoalLinkSelf          = errors.New("service: goal cannot link to itself")
	ErrGoalLinkNotAccessible = errors.New("service: parent goal not accessible")
	ErrGoalLinkCycle         = errors.New("service: goal link cycle")
)
```

- [ ] **Step 2: Написать падающий тест сервиса (валидация доступа + цикл-маппинг)**

`internal/service/goal_links_test.go` (по образцу `internal/service/goal_test.go`):

```go
func TestSetGoalParents_ValidatesAccessAndCycle(t *testing.T) {
	ctx, svc, db, scope := setupService(t) // helper из goal_test.go
	teamA := seedTeam(t, ctx, db, scope, "A", "unit")
	teamB := seedTeam(t, ctx, db, scope, "B", "unit")
	child := seedGoalInTeam(t, ctx, db, scope, teamA, "child")
	parent := seedGoalInTeam(t, ctx, db, scope, teamB, "parent")

	// Родитель в teamB, но у вызывающего scope только teamA → not accessible.
	err := svc.SetGoalParents(ctx, scope, []int64{teamA}, false, child, []int64{parent}, 1)
	require.ErrorIs(t, err, service.ErrGoalLinkNotAccessible)

	// scope на обе → ок.
	err = svc.SetGoalParents(ctx, scope, []int64{teamA, teamB}, false, child, []int64{parent}, 1)
	require.NoError(t, err)

	// Самоссылка.
	err = svc.SetGoalParents(ctx, scope, []int64{teamA}, false, child, []int64{child}, 1)
	require.ErrorIs(t, err, service.ErrGoalLinkSelf)

	// Цикл: parent -> child (обратное) при scope на обе.
	err = svc.SetGoalParents(ctx, scope, []int64{teamA, teamB}, false, parent, []int64{child}, 1)
	require.ErrorIs(t, err, service.ErrGoalLinkCycle)
}
```

- [ ] **Step 3: Запустить — убедиться, что падает**

Run: `go test ./internal/service/ -run TestSetGoalParents_ValidatesAccessAndCycle -v`
Expected: FAIL (метод не реализован).

- [ ] **Step 4: Реализовать `internal/service/goal_links.go`**

```go
package service

import (
	"context"
	"errors"

	"okrs/internal/domain"
	"okrs/internal/store/goallinks"
)

// SetGoalParents заменяет набор родителей дочерней цели.
// Валидация: доступ к команде-владельцу ребёнка; принадлежность тенанту и scope-доступ
// каждого родителя; запрет самоссылки; запрет цикла. Пишет журнал goal_linked/goal_unlinked.
func (s *Service) SetGoalParents(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, adminAll bool, childID int64, parentIDs []int64, actorUserID int64) error {
	child, err := s.goals.GetGoal(ctx, scope, childID)
	if err != nil {
		return ErrGoalLinkNotAccessible
	}
	// Доступ к команде-владельцу ребёнка проверяет handler (CanAccessTeamFromCtx);
	// здесь дублируем через allowedTeamIDs/adminAll для service-тестов.
	if !adminAll && !containsID(allowedTeamIDs, child.TeamID) {
		return ErrGoalLinkNotAccessible
	}

	// Дедуп + самоссылка.
	seen := map[int64]bool{}
	uniq := make([]int64, 0, len(parentIDs))
	for _, p := range parentIDs {
		if p == childID {
			return ErrGoalLinkSelf
		}
		if !seen[p] {
			seen[p] = true
			uniq = append(uniq, p)
		}
	}

	// Принадлежность тенанту + scope-доступ каждого родителя одним запросом.
	if len(uniq) > 0 {
		owners, err := s.goals.ListGoalOwnerTeamIDs(ctx, scope, uniq)
		if err != nil {
			return err
		}
		for _, p := range uniq {
			teamID, ok := owners[p]
			if !ok { // нет в тенанте
				return ErrGoalLinkNotAccessible
			}
			if !adminAll && !containsID(allowedTeamIDs, teamID) {
				return ErrGoalLinkNotAccessible
			}
		}
	}

	added, removed, err := s.goalLinks.ReplaceParents(ctx, scope, childID, uniq)
	if err != nil {
		if errors.Is(err, goallinks.ErrCycle) {
			return ErrGoalLinkCycle
		}
		return err
	}

	teamID, periodID := child.TeamID, child.PeriodID
	if len(added) > 0 {
		s.recordActivity(ctx, scope, domain.ActivityEvent{
			ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionGoalLinked,
			TeamID: &teamID, PeriodID: &periodID, GoalID: &childID, EntityTitle: child.Title,
			Payload: map[string]any{"linked_parent_goal_ids": added},
		})
	}
	if len(removed) > 0 {
		s.recordActivity(ctx, scope, domain.ActivityEvent{
			ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionGoalUnlinked,
			TeamID: &teamID, PeriodID: &periodID, GoalID: &childID, EntityTitle: child.Title,
			Payload: map[string]any{"unlinked_parent_goal_ids": removed},
		})
	}
	return nil
}

// ListLinksForGoals проксирует чтение связей для сборки parents/children доски.
func (s *Service) ListLinksForGoals(ctx context.Context, scope domain.TenantScope, goalIDs, allowedTeamIDs []int64, adminAll bool) (map[int64][]domain.GoalRef, map[int64][]domain.GoalRef, error) {
	return s.goalLinks.ListLinksForGoals(ctx, scope, goalIDs, allowedTeamIDs, adminAll)
}

// ListLinkableGoals возвращает кандидатов для пикера родителя.
func (s *Service) ListLinkableGoals(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, adminAll bool, periodID *int64, excludeGoalID int64, q string) ([]goallinks.LinkableGoal, error) {
	return s.goalLinks.ListLinkable(ctx, scope, allowedTeamIDs, adminAll, periodID, excludeGoalID, q)
}

func containsID(ids []int64, id int64) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}
```

> Проверь: `containsID` не конфликтует с существующим helper в пакете `service`. Если такой уже есть — переиспользуй его и удали дубликат.

- [ ] **Step 5: Запустить тест сервиса**

Run: `go test ./internal/service/ -run TestSetGoalParents_ValidatesAccessAndCycle -v`
Expected: PASS.

- [ ] **Step 6: Тест на запись активности (goal_linked/goal_unlinked)**

```go
func TestSetGoalParents_RecordsActivity(t *testing.T) {
	ctx, svc, db, scope := setupService(t)
	teamA := seedTeam(t, ctx, db, scope, "A", "unit")
	child := seedGoalInTeam(t, ctx, db, scope, teamA, "child")
	p1 := seedGoalInTeam(t, ctx, db, scope, teamA, "p1")
	require.NoError(t, svc.SetGoalParents(ctx, scope, []int64{teamA}, false, child, []int64{p1}, 7))
	// Замена на пусто → должна залогироваться goal_unlinked.
	require.NoError(t, svc.SetGoalParents(ctx, scope, []int64{teamA}, false, child, nil, 7))
	// Проверить журнал: минимум одна запись goal_linked и одна goal_unlinked.
	assertActivityActions(t, ctx, db, scope, child, domain.ActionGoalLinked, domain.ActionGoalUnlinked)
}
```

(Хелпер `assertActivityActions` — считывает `activity_events` по `goal_id`; при отсутствии напиши минимальный SELECT в тесте.)

Run: `go test ./internal/service/ -run TestSetGoalParents_RecordsActivity -v`
Expected: PASS.

- [ ] **Step 7: Commit (staged)**

```bash
git add internal/service/service.go internal/service/goal_links.go internal/service/goal_links_test.go
# сообщение автору: "feat(goal-links): service SetGoalParents + list, with activity journaling"
```

---

### Task 5: Встроить `parents`/`children` в доску и `GET /goals/{id}`

**Files:**
- Modify: `internal/http/dto/goal.go` (поля `Parents`/`Children` + тип `GoalRef` DTO)
- Modify: `internal/service/service.go` (в `GetTeamOKR` дозагрузить связи и проставить `Progress`; в `GetGoal`-пути для API — см. ниже)
- Modify: `internal/http/handlers/api/v1/teams/response.go` (маппинг `domain.Goal.Parents/Children → dto`)
- Modify: `internal/http/handlers/api/v1/goals/response.go` (то же для `newGoalResponse`)
- Modify: `internal/http/handlers/api/v1/goals/handler.go` (`HandleGoal` — дозагрузить связи по scope)
- Test: `internal/http/handlers/api/v1/teams/integration_test.go` (доска отдаёт parents/children)

**Interfaces:**
- Consumes: `service.ListLinksForGoals` (Task 4).
- Produces: `dto.GoalRef` + `GoalDetails.Parents []GoalRef`, `GoalDetails.Children []GoalRef`.

- [ ] **Step 1: Добавить DTO**

В `internal/http/dto/goal.go`:

```go
type GoalRef struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	PeriodID   int64  `json:"period_id"`
	PeriodName string `json:"period_name"`
	TeamID     int64  `json:"team_id"`
	TeamName   string `json:"team_name"`
	TeamType   string `json:"team_type"`
	Progress   int    `json:"progress"`
}
```

В `type GoalDetails struct` добавить (после `ShareTeams`):

```go
	Parents  []GoalRef `json:"parents"`
	Children []GoalRef `json:"children"`
```

- [ ] **Step 2: Дозагрузить связи в `GetTeamOKR` (service)**

Найти `GetTeamOKR` в `service.go`. После сборки `okr.Goals` (когда есть список `domain.Goal` и посчитан их прогресс) добавить: собрать `goalIDs`, вызвать `s.goalLinks.ListLinksForGoals(ctx, scope, goalIDs, allowedTeamIDs, adminAll)`, проставить `goal.Parents/Children`. `allowedTeamIDs`/`adminAll` уже доступны в scope расчёта доски — если нет, прокинуть их параметром из handler (handler знает `AllowedTeamIDsFromCtx`/админ-флаг). Прогресс каждой связанной цели дозаполнить: связанные цели могут быть вне текущей доски, поэтому посчитать их прогресс отдельным вызовом (`okr`-расчёт по загруженным KR) ЛИБО оставить `Progress=0` на первом шаге и заполнить в отдельном под-шаге (см. Step 3).

> Решение по прогрессу связанных целей (во избежание N+1): одним батч-запросом подгрузить KR всех уникальных связанных goalIDs и посчитать `okr.GoalProgress` для каждой; заполнить `GoalRef.Progress`. Реализуй helper `s.computeGoalRefsProgress(ctx, scope, refs)` — собирает уникальные id, грузит KR батчем через существующий `GoalRepo`, считает прогресс, проставляет в срезы.

- [ ] **Step 3: Написать интеграционный тест доски**

В `teams/integration_test.go` добавить сценарий: создать child в команде доски + parent в другой доступной команде, связать (через store), запросить `/api/v1/teams/{team}/okrs?period_id=`, проверить, что у goal в ответе `parents` длины 1 с ожидаемым `title/team_name/period_name`, и что `children` пуст.

- [ ] **Step 4: Прокинуть маппинг в `teams/response.go` и `goals/response.go`**

В обоих builder'ах, где заполняется `dto.GoalDetails`, добавить:

```go
	Parents:  mapGoalRefs(goal.Parents),
	Children: mapGoalRefs(goal.Children),
```

Общий helper (положить в `internal/http/handlers/api/v1` рядом с `MapGoalComment`, чтобы переиспользовать оба handler-пакета):

```go
func MapGoalRefs(refs []domain.GoalRef) []dto.GoalRef {
	out := make([]dto.GoalRef, 0, len(refs))
	for _, r := range refs {
		out = append(out, dto.GoalRef{
			ID: r.ID, Title: r.Title, PeriodID: r.PeriodID, PeriodName: r.PeriodName,
			TeamID: r.TeamID, TeamName: r.TeamName, TeamType: r.TeamType, Progress: r.Progress,
		})
	}
	return out
}
```

- [ ] **Step 5: Дозагрузить связи в `HandleGoal` (`GET /goals/{id}`)**

В `goals/handler.go` `HandleGoal`, после проверки доступа и до `newGoalResponse`, дозагрузить связи по scope:

```go
	allowed, adminAll := auth.AllowedTeamIDsFromCtx(r.Context()) // проверь точную сигнатуру helper'а
	parents, children, _ := h.service.ListLinksForGoals(r.Context(), scope, []int64{goal.ID}, allowed, adminAll)
	goal.Parents = parents[goal.ID]
	goal.Children = children[goal.ID]
	// прогресс связанных дозаполнить тем же helper'ом, что и на доске (или сервисным методом).
```

> Проверь фактический helper получения allowedTeamIDs/adminAll из контекста (в других handler'ах используется `auth.CanAccessTeamFromCtx`; для списка — ищи `AllowedTeamIDsFromCtx` или аналог). Если админ определяется отдельно (`ActiveRoleFromContext == RoleAdmin`), собери `adminAll` из него.

- [ ] **Step 6: Запустить тесты доски и goal**

Run: `go test ./internal/http/handlers/api/v1/teams/ ./internal/http/handlers/api/v1/goals/ -run "Link|OKR|Goal" -v`
Expected: PASS (новый тест доски + существующие не сломаны).

- [ ] **Step 7: Commit (staged)**

```bash
git add internal/http/dto/goal.go internal/service/service.go internal/http/handlers/api/v1/teams/response.go internal/http/handlers/api/v1/goals/response.go internal/http/handlers/api/v1/goals/handler.go internal/http/handlers/api/v1/*.go internal/http/handlers/api/v1/teams/integration_test.go
# сообщение автору: "feat(goal-links): embed parents/children in board and goal detail"
```

---

### Task 6: API — `GET /goals/linkable` и `POST /goals/{id}/links`

**Files:**
- Create: `internal/http/handlers/api/v1/goals/links.go` (два handler'а)
- Modify: `internal/http/handlers/api/v1/goals/routes.go` (регистрация)
- Test: `internal/http/handlers/api/v1/goals/links_test.go`

**Interfaces:**
- Consumes: `service.SetGoalParents`, `service.ListLinkableGoals` (Task 4), `auth.*` helpers.
- Produces: маршруты `GET /api/v1/goals/linkable`, `POST /api/v1/goals/{goalID}/links`.

- [ ] **Step 1: Написать падающий contract-тест**

`links_test.go` (по образцу существующих `*_test.go` в пакете goals — они поднимают полноценный сервер/роутер; следуй `move_test.go`/`resolve_test.go`):

```go
func TestSetGoalParents_Contract(t *testing.T) {
	// Arrange: сервер, тенант, команда в scope, child + parent.
	// 1) POST /goals/{child}/links {parent_goal_ids:[parent]} → 204
	// 2) GET /goals/{child} → parents length 1
	// 3) POST с parent_goal_ids:[child] (self) → 400 VALIDATION_ERROR
	// 4) POST, создающий цикл (parent->child после child->parent) → 409 CONFLICT
	// 5) POST без CSRF → 403
	// 6) POST для недоступной команды-владельца child → 404
}
```

(Разверни каждый под-случай в реальные HTTP-запросы, как в `move_test.go`.)

- [ ] **Step 2: Запустить — падает**

Run: `go test ./internal/http/handlers/api/v1/goals/ -run TestSetGoalParents_Contract -v`
Expected: FAIL (роут не зарегистрирован).

- [ ] **Step 3: Реализовать handlers в `links.go`**

```go
package goals

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"okrs/internal/auth"
	"okrs/internal/domain"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/dto"
	"okrs/internal/http/handlers/web/common"
	"okrs/internal/service"

	"github.com/go-chi/chi/v5"
)

// GET /api/v1/goals/linkable?period_id=&q=&exclude_goal_id=
func (h *Handler) HandleLinkableGoals(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	var periodID *int64
	if raw := strings.TrimSpace(r.URL.Query().Get("period_id")); raw != "" && raw != "all" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid period_id", map[string]string{"period_id": "invalid"})
			return
		}
		periodID = &id
	}
	var excludeGoalID int64
	if raw := strings.TrimSpace(r.URL.Query().Get("exclude_goal_id")); raw != "" {
		excludeGoalID, _ = strconv.ParseInt(raw, 10, 64)
	}
	q := r.URL.Query().Get("q")
	allowed, adminAll := h.allowedTeams(r) // helper ниже
	items, err := h.service.ListLinkableGoals(r.Context(), scope, allowed, adminAll, periodID, excludeGoalID, q)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to list linkable goals", nil)
		return
	}
	out := make([]dto.LinkableGoal, 0, len(items))
	for _, it := range items {
		out = append(out, dto.LinkableGoal{
			GoalRef: dto.GoalRef{ID: it.ID, Title: it.Title, PeriodID: it.PeriodID, PeriodName: it.PeriodName, TeamID: it.TeamID, TeamName: it.TeamName, TeamType: it.TeamType, Progress: it.Progress},
			Lead:    it.Lead,
		})
	}
	v1.WriteJSON(w, http.StatusOK, out)
}

// POST /api/v1/goals/{goalID}/links  body {"parent_goal_ids":[...]}
func (h *Handler) HandleSetGoalParents(w http.ResponseWriter, r *http.Request) {
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
	goal, err := h.service.GetGoal(r.Context(), scope, goalID)
	if err != nil || !auth.CanAccessTeamFromCtx(r.Context(), goal.TeamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	var req struct {
		ParentGoalIDs []int64 `json:"parent_goal_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	allowed, adminAll := h.allowedTeams(r)
	if err := h.service.SetGoalParents(r.Context(), scope, allowed, adminAll, goalID, req.ParentGoalIDs, auth.UserIDFromContext(r.Context())); err != nil {
		switch {
		case errors.Is(err, service.ErrGoalLinkSelf):
			v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "goal cannot link to itself", map[string]string{"parent": "self"})
		case errors.Is(err, service.ErrGoalLinkNotAccessible):
			v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "parent goal not accessible", map[string]string{"parent": "not accessible"})
		case errors.Is(err, service.ErrGoalLinkCycle):
			v1.WriteError(w, http.StatusConflict, "GOAL_LINK_CYCLE", "goal link would create a cycle", nil)
		default:
			v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to set goal links", nil)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// allowedTeams возвращает доступные команды и признак админа (полный доступ) из контекста.
func (h *Handler) allowedTeams(r *http.Request) ([]int64, bool) {
	role, _ := auth.ActiveRoleFromContext(r.Context())
	if role == domain.RoleAdmin {
		return nil, true
	}
	return auth.AllowedTeamIDsFromCtx(r.Context()), false // проверь точное имя helper'а
}
```

Добавить DTO в `internal/http/dto/goal.go`:

```go
type LinkableGoal struct {
	GoalRef
	Lead string `json:"lead"`
}
```

> Проверь фактические имена: `auth.AllowedTeamIDsFromCtx` и `auth.ActiveRoleFromContext`. В `handler.go` уже используется `auth.ActiveRoleFromContext` и `domain.RoleAdmin`. Для списка доступных команд найди helper, которым пользуются list-endpoints (напр. в hierarchy/period-overview handlers). Если админ уже отражён в scope (все команды), можно передавать `adminAll=true` и `allowed=nil`.

- [ ] **Step 4: Зарегистрировать роуты**

В `routes.go` добавить (статический `linkable` — до параметрических; chi отдаёт приоритет статике, но регистрируем явно раньше блока `{goalID}`):

```go
	r.Get("/api/v1/goals/linkable", h.HandleLinkableGoals)
	r.Post("/api/v1/goals/{goalID}/links", h.HandleSetGoalParents)
```

- [ ] **Step 5: Запустить contract-тест**

Run: `go test ./internal/http/handlers/api/v1/goals/ -run TestSetGoalParents_Contract -v`
Expected: PASS.

- [ ] **Step 6: Прогнать весь пакет goals + vet**

Run: `go test ./internal/http/handlers/api/v1/goals/... && go vet ./...`
Expected: PASS.

- [ ] **Step 7: Commit (staged)**

```bash
git add internal/http/handlers/api/v1/goals/links.go internal/http/handlers/api/v1/goals/routes.go internal/http/handlers/api/v1/goals/links_test.go internal/http/dto/goal.go
# сообщение автору: "feat(goal-links): API endpoints linkable + set parents"
```

---

### Task 7: Seed demo-связей

**Files:**
- Modify: `internal/store/seed.go`
- Test: `internal/store/seed_test.go` (если проверяет структуру — дополнить)

**Interfaces:**
- Consumes: таблица `goal_links`, id демо-целей, создаваемых в `seedDemo`.

- [ ] **Step 1: Добавить вставку демо-связей в конце `seedDemo`**

После создания демо-целей (когда известны их id) вставить пару связей, консистентных со скриншотами: квартальные цели Платформы → годовая цель платформы. Использовать прямой INSERT через `goalsRepo.DB()` (в пакете уже так делают для seed) или добавить метод. Пример:

```go
	// Демо-связи: квартальные цели платформы наследуют годовую цель платформы.
	if _, err := goalsRepo.DB().Exec(ctx, `
		INSERT INTO goal_links (tenant_id, child_goal_id, parent_goal_id)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING`, tenantID, quarterlyPlatformGoalID, annualPlatformGoalID); err != nil {
		return err
	}
```

(Подставь реальные переменные id из окружающего кода seed; если id годовой/квартальной цели там не именованы — заведи их при создании целей.)

- [ ] **Step 2: Прогнать seed-тест / сборку**

Run: `go build ./... && go test ./internal/store/ -run Seed -v`
Expected: PASS.

- [ ] **Step 3: Commit (staged)**

```bash
git add internal/store/seed.go
# сообщение автору: "chore(goal-links): seed demo goal links"
```

---

### Task 8: Фронтенд — данные + лейблы `↑N`/`↓N` + popover на карточке

**Files:**
- Modify: `internal/web/static/tracker.js` (`mapGoal` ~160; `GoalCard` badge-strip ~1668; новый компонент `GoalLinksPopover`)
- Modify: `internal/web/static/tracker.css` (классы `.goal-links*`)

**Interfaces:**
- Consumes: поля `parents`/`children` в ответе `/teams/{id}/okrs` (Task 5).
- Produces: клиентские поля `goal.parents`/`goal.children`; компоненты `GoalLinkLabel`, `GoalLinksPopover`.

> Замечание: JS-тест-фреймворка в проекте нет (браузерный Babel, без сборки). Проверка — визуальная через запуск приложения (`/run` скилл или docker-compose) + ручной сценарий. Каждый шаг ниже завершается ручной проверкой.

- [ ] **Step 1: Прокинуть связи в `mapGoal`**

В `mapGoal(g)` (~`tracker.js:160`) добавить в возвращаемый объект:

```js
    parents: (g.parents || []).map(mapGoalRef),
    children: (g.children || []).map(mapGoalRef),
```

Рядом добавить маппер:

```js
function mapGoalRef(r) {
  return {
    id: r.id, title: r.title,
    periodId: r.period_id, periodName: r.period_name,
    teamId: r.team_id, teamName: r.team_name, teamType: r.team_type,
    progress: r.progress || 0,
  };
}
```

- [ ] **Step 2: Компонент лейбла + popover**

Добавить (рядом с `UserInfo`, чтобы переиспользовать паттерн портала/таймера, ~`tracker.js:1967`):

```jsx
function GoalLinksPopover({ dir, items, accent, onOpenGoal }) {
  // dir: 'up' (родители, "ВКЛАД В") | 'down' (дети)
  const [open, setOpen] = React.useState(false);
  const timer = React.useRef(null);
  if (!items || items.length === 0) return null;
  const label = dir === 'up' ? `↑ ${items.length}` : `↓ ${items.length}`;
  const title = dir === 'up' ? `↑ ВКЛАД В · ${items.length}` : `↓ ${items.length}`;
  const openNow = () => { clearTimeout(timer.current); setOpen(true); };
  const closeSoon = () => { timer.current = setTimeout(() => setOpen(false), 150); };
  return (
    <span className="goal-link-label" onMouseEnter={openNow} onMouseLeave={closeSoon}>
      <span className="goal-link-label__badge">{label}</span>
      {open && (
        <div className="goal-links-popup" onMouseEnter={openNow} onMouseLeave={closeSoon}>
          <div className="goal-links-popup__head">{title}</div>
          {items.map(it => (
            <button key={it.id} className="goal-links-popup__row" onClick={() => onOpenGoal(it)}>
              <span className="goal-links-popup__title">{it.title}</span>
              <span className="goal-links-popup__meta">{it.periodName} · {it.teamName}</span>
              <span className="goal-links-popup__progress">{it.progress}%</span>
            </button>
          ))}
        </div>
      )}
    </span>
  );
}
```

- [ ] **Step 3: Встроить лейблы в badge-strip `GoalCard`**

В `.goal-card__meta` (~`tracker.js:1668`), рядом с share-бейджем, добавить:

```jsx
  <GoalLinksPopover dir="up"   items={goal.parents}  accent={accent} onOpenGoal={openLinkedGoal} />
  <GoalLinksPopover dir="down" items={goal.children} accent={accent} onOpenGoal={openLinkedGoal} />
```

`openLinkedGoal(it)` — навигация deep-link на цель `it` (используй существующий механизм `deepLink`: собрать URL `?team=it.teamId&period=it.periodId&goal=it.id` и `location.assign`/router).

- [ ] **Step 4: Стили `.goal-links*` в `tracker.css`**

Добавить классы для бейджа (компактный, как `.goal-card__weight`) и popup (портал-стиль, как `.uinfo__popup`): фикс. позиционирование/тень/скролл, `.goal-links-popup__row` кликабельный, hover-подсветка. Держать визуальную грамматику как у остальных бейджей/поповеров.

- [ ] **Step 5: Ручная проверка**

Запустить приложение, открыть команду с демо-связями (Task 7). Ожидаемо: у дочерней цели виден `↑ 1`, у родительской — `↓ N`; hover раскрывает список с названием/периодом/командой/прогрессом; клик по строке открывает связанную цель.

- [ ] **Step 6: Commit (staged)**

```bash
git add internal/web/static/tracker.js internal/web/static/tracker.css
# сообщение автору: "feat(goal-links): card labels + popover"
```

---

### Task 9: Фронтенд — секция «Связанные цели» + пикер в GoalModal

**Files:**
- Modify: `internal/web/static/tracker.js` (`GoalModal` ~2070–2257; новый компонент `GoalParentPicker`; сохранение связей в `performSave`)
- Modify: `internal/web/static/tracker.css` (классы секции/пикера)

**Interfaces:**
- Consumes: `GET /api/v1/goals/linkable`, `POST /api/v1/goals/{id}/links`, компонент `PeriodSelect` (`period_select.js`), `InfoHint`.
- Produces: локальное состояние `form.parentIds` + карточки родителей + пикер.

- [ ] **Step 1: Состояние формы + загрузка текущих родителей**

В `GoalModal` завести `const [parents, setParents] = React.useState(goal?.parents || [])` (массив `GoalRef`-подобных объектов). Для нового goal — пусто.

- [ ] **Step 2: Секция «Связанные цели» (под блоком share)**

```jsx
<div className="goal-links-section">
  <div className="goal-links-section__head">
    🔗 Связанные цели
    <InfoHint text="Связь привязывает эту цель к верхнеуровневой (цель руководителя, годовая цель, цель юнита/кластера). Это НЕ делает цель общей — общие цели (⇄) видны нескольким командам, а связь соединяет две разные цели отношением «дочерняя → родительская»." />
  </div>
  <div className="goal-links-section__hint">К какой верхнеуровневой цели относится эта цель. Можно указать несколько.</div>
  {parents.map(p => (
    <div key={p.id} className="goal-link-card">
      <span className="goal-link-card__arrow">↑</span>
      <span className="goal-link-card__title">{p.title}</span>
      <span className="goal-link-card__meta">{p.periodName} · {p.teamName} · {p.progress}%</span>
      <button className="goal-link-card__remove" onClick={() => setParents(parents.filter(x => x.id !== p.id))}>✕</button>
    </div>
  ))}
  <button className="goal-link-add" onClick={() => setPickerOpen(true)}>+ Добавить связь</button>
</div>
{pickerOpen && (
  <GoalParentPicker
    excludeGoalId={goal?.id}
    currentPeriodId={periodId}
    periods={periods}
    accent={accent}
    onPick={(g) => { if (!parents.some(x => x.id === g.id)) setParents([...parents, g]); }}
    onClose={() => setPickerOpen(false)}
  />
)}
```

- [ ] **Step 3: Компонент `GoalParentPicker`**

Переиспользовать `PeriodSelect` (по умолчанию текущий период, с опцией «Все периоды» → `period_id=all`) + inline-поиск + сгруппированный по командам список из `/goals/linkable`:

```jsx
function GoalParentPicker({ excludeGoalId, currentPeriodId, periods, accent, onPick, onClose }) {
  const [periodId, setPeriodId] = React.useState(currentPeriodId); // 'all' | number
  const [q, setQ] = React.useState('');
  const [items, setItems] = React.useState([]);
  React.useEffect(() => {
    const pid = periodId === 'all' ? 'all' : String(periodId);
    const params = new URLSearchParams({ period_id: pid, q });
    if (excludeGoalId) params.set('exclude_goal_id', String(excludeGoalId));
    apiGet(`/api/v1/goals/linkable?${params.toString()}`)
      .then(rows => setItems((rows || []).map(mapGoalRef)))
      .catch(() => setItems([]));
  }, [periodId, q, excludeGoalId]); // зависимости выверены: без петли refetch
  // Группировка по команде (teamType заголовок КЛАСТЕР/ЮНИТ, отступ), рендер строк.
  // PeriodSelect с дополнительным пунктом "Все периоды".
  // ... разметка модалки-пикера аналогично TransferGoalModal ...
}
```

> Для опции «Все периоды» в `PeriodSelect`: либо расширить `PeriodSelect` пропом `allowAll`, либо добавить синтетический пункт `{id:'all', name:'Все периоды'}` в список `periods`, передаваемый в пикер (проще — синтетический пункт, не трогая общий компонент). Выбери минимально инвазивный вариант; если правишь `period_select.js` — не сломай другие места его использования.

- [ ] **Step 4: Сохранение связей в `performSave`**

После успешного сохранения/создания цели (когда известен `goalId`) вызвать full-replace:

```js
await apiPost(`/api/v1/goals/${goalId}/links`, { parent_goal_ids: parents.map(p => p.id) });
```

Обработать `409 GOAL_LINK_CYCLE` и `400` — показать сообщение пользователю (цикл/недоступная цель), не роняя сохранение остальных полей (связи сохраняем последним шагом, как share).

- [ ] **Step 5: Стили секции/пикера в `tracker.css`**

Классы `.goal-links-section*`, `.goal-link-card*`, `.goal-link-add`, `.goal-parent-picker*`. Консистентно с блоком share и `TransferGoalModal`.

- [ ] **Step 6: Ручная проверка сценария**

1) Открыть редактирование цели → секция «Связанные цели» видна, `?` показывает подсказку.
2) «+ Добавить связь» → пикер: период по умолчанию текущий, есть «Все периоды»; поиск по цели/команде/лиду; список сгруппирован по командам.
3) Выбрать родителя → карточка появилась; сохранить → на карточке цели появился `↑ 1`.
4) Попытка выбрать себя/создать цикл → понятная ошибка (400/409), остальные поля сохранены.

- [ ] **Step 7: Commit (staged)**

```bash
git add internal/web/static/tracker.js internal/web/static/tracker.css
# сообщение автору: "feat(goal-links): GoalModal parent-link section + picker"
```

---

### Task 10: Обновить канонические спеки (тот же change set)

**Files:**
- Modify: `specs/020-domain-model.md` (сущность `GoalLink`, `GoalRef`, поля `Parents`/`Children` у `Goal`, инварианты/ацикличность/каскады)
- Modify: `specs/040-api-contract.md` (три эндпоинта со всеми 7 атрибутами; расширение ответа `okrs`/`GET goals/{id}` полями `parents`/`children`; действия журнала `goal_linked`/`goal_unlinked`)
- Modify: `specs/050-permissions-and-lifecycle.md` (блок lifecycle-вопросов; добавить действия в категорию `composition`)
- Modify: `specs/030-user-flows.md` (флоу привязки/отвязки родителя и просмотра связей)

**Interfaces:** документация; сверяется с реализацией Tasks 1–9.

- [ ] **Step 1: `020-domain-model.md`**

Добавить раздел «### GoalLink» (поля/инварианты из дизайн-дока §3), упомянуть `Parents`/`Children` как вычисляемые поля `Goal` и `GoalRef`. Указать: acyclic, каскады, «связи не копируются при transfer», «прогресс не агрегируется по связям».

- [ ] **Step 2: `040-api-contract.md`**

Добавить в «Write endpoints» и «Read endpoints» три эндпоинта (`GET /goals/linkable`, `POST /goals/{id}/links`, а также расширение `okrs`/`goals/{id}` полями `parents`/`children`) — с method+path, request, validation, success, errors (`400/404/409 GOAL_LINK_CYCLE`), idempotency, side effects. Добавить `goal_linked`/`goal_unlinked` в перечисление действий журнала (категория `composition`).

- [ ] **Step 3: `050-permissions-and-lifecycle.md`**

Добавить блок «### Связывание целей (parent/child)» с ответами на обязательные lifecycle-вопросы (из дизайн-дока §6). Добавить `goal_linked`/`goal_unlinked` в список действий `composition`.

- [ ] **Step 4: `030-user-flows.md`**

Описать флоу: при редактировании цели → секция «Связанные цели» → пикер (период по умолчанию текущий, «Все периоды», поиск) → сохранение full-replace; отображение `↑N`/`↓N` + popover в списке; scope-видимость.

- [ ] **Step 5: Финальная проверка**

Run: `go build ./... && go test ./... && go vet ./...`
Expected: PASS (весь проект зелёный).

- [ ] **Step 6: Commit (staged)**

```bash
git add specs/020-domain-model.md specs/040-api-contract.md specs/050-permissions-and-lifecycle.md specs/030-user-flows.md
# сообщение автору: "docs(goal-links): update canonical specs (domain, api, lifecycle, flows)"
```

---

## Self-Review

**Spec coverage (дизайн-док → задачи):**
- §3 доменная модель (таблица/типы) → Task 1, 2.
- §4 видимость/tenant → Task 2 (scope-фильтрация, tenant-изоляция, каскад-тесты), Task 4 (валидация принадлежности/scope).
- §5 предотвращение циклов → Task 2 (CTE + тесты циклов), Task 4 (маппинг `ErrCycle`), Task 6 (409).
- §6 lifecycle → Task 6 (нет проверки статуса; проверка доступа к owner-команде), Task 10 (спека).
- §7.1 встраивание parents/children → Task 5.
- §7.2 `GET /goals/linkable` → Task 2 (repo), 4 (service), 6 (handler+тест).
- §7.3 `POST /goals/{id}/links` full-replace → Task 2/4/6.
- журналирование `goal_linked`/`goal_unlinked` → Task 1 (константы), 4 (запись + тест).
- §9 фронтенд (секция, пикер, лейблы, popover) → Task 8, 9.
- §10 обновление спек → Task 10.
- §11 seed → Task 7.
- §12 тесты → распределены по Task 2/4/5/6 (+ ручные фронт-проверки 8/9).

**Placeholder scan:** оставлены осознанные «проверь точное имя helper'а» (`AllowedTeamIDsFromCtx`, механизм deepLink) — это НЕ плейсхолдеры логики, а точки сверки с существующим кодом; в каждом случае указан образец/фолбэк. Мёртвый первый CTE-запрос в Task 2 Step 3 явно помечен на удаление.

**Type consistency:** `domain.GoalRef`/`domain.GoalLink` (Task 1) ↔ `dto.GoalRef`/`dto.LinkableGoal` (Task 5/6) ↔ клиентский `mapGoalRef` (Task 8) — поля согласованы (`period_name`, `team_name`, `team_type`, `progress`). Методы репозитория (`ReplaceParents`, `ListLinksForGoals`, `ListLinkable`) одинаково названы в Task 2 (реализация), Task 4 (интерфейс `GoalLinkRepo`). Сервисные ошибки (`ErrGoalLinkSelf/NotAccessible/Cycle`) заданы в Task 4 и используются в Task 6.

**Известные точки сверки для исполнителя (не блокеры):**
- фактический helper доступных команд/признака админа из контекста (`AllowedTeamIDsFromCtx` — имя предположительное);
- отсутствие хранимого прогресса в `goals`: `GoalRef.Progress` заполняется service-слоем (Task 5 Step 2), из репозитория приходит 0;
- механизм deep-link на цель в трекере (Task 8 Step 3).
