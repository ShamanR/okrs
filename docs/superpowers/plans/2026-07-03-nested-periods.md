# Вложенные периоды — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Сделать периоды вложенными (дерево год→кварталы) с автоматическими вложенностью, статусом и порядком, вычисляемыми из дат; ручной порядок (`sort_order`) убрать, добавить ручной статус «Архивный».

**Architecture:** Вложенность, статусы `future/active/closed` и порядок — чистые функции от `start_date`, `end_date` и «сейчас», считаются на чтении в доменном слое (`BuildPeriodViews`). Единственное хранимое дополнение — nullable `archived_at` (ручной статус `archived`). Публичный `GET /api/v1/periods` архивные не отдаёт; админка берёт всё из нового `GET /api/v1/admin/periods`.

**Tech Stack:** Go (chi router, pgx), PostgreSQL (numbered SQL migrations), React через CDN в vanilla-файлах `internal/web/static/*.js`.

## Global Constraints

- Слои: domain → store → service → http; не протекать абстракциями (CLAUDE.md #6).
- Тенант-скоуп на каждом запросе через `domain.TenantScope`; все SQL фильтруют `tenant_id`.
- Никаких запросов в цикле; агрегаты одним запросом (CLAUDE.md #9).
- Мультиинстанс K8s: не полагаться на in-memory согласованность статусов — считать из дат на каждом чтении (CLAUDE.md #10).
- Даты периодов — тип `DATE`; сравнение статуса — по дате.
- Тесты держать актуальными (CLAUDE.md #4); seed demo — актуальным (CLAUDE.md #7).
- **Не делать git commit** — пользователь коммитит сам (CLAUDE.md #8). Шаги «Commit» ниже заменяются на «остановиться и показать дифф пользователю».
- Спеки — source of truth; обновить `020/030/040/050` в этом же change set.
- В коммитах/PR/доках/комментариях не упоминать AI/ассистентов (CLAUDE.md #5).

---

## Файловая структура

**Создаются:**
- `internal/domain/period_status.go` — enum `PeriodStatus` + `PeriodStatusFor`.
- `internal/domain/period_tree.go` — `PeriodView` + `BuildPeriodViews`.
- `internal/domain/period_status_test.go`, `internal/domain/period_tree_test.go`.
- `migrations/036_period_nesting.up.sql`, `migrations/036_period_nesting.down.sql`.

**Изменяются:**
- `internal/domain/models.go` — `Period`: убрать `SortOrder`, добавить `ArchivedAt *time.Time`.
- `internal/store/periods/periods.go` — запросы (drop `sort_order`, add `archived_at`), `FindPeriodForDate` порядок, убрать `MovePeriod`, добавить `ArchivePeriod`/`UnarchivePeriod`.
- `internal/store/periods/periods_test.go`, `periods_isolation_test.go` — под новые поля/методы.
- `internal/http/dto/period.go` — `PeriodInfo`: убрать `SortOrder`, добавить `ParentID/Depth/Status`.
- `internal/http/handlers/api/v1/helpers_response.go` — `MapPeriodInfo` + новый `MapPeriodView`.
- `internal/http/handlers/api/v1/periods/handler.go`, `response.go`, `routes.go` — публичный список через views (без архивных).
- `internal/http/handlers/api/v1/admin/service_handler.go` — убрать move-хендлеры; добавить `HandleListPeriods`, `HandleArchivePeriod`, `HandleUnarchivePeriod`.
- `internal/service/service.go` — `PeriodRepo` интерфейс, методы: убрать `MovePeriod`, добавить `ListPeriodViews`, `ArchivePeriod`, `UnarchivePeriod`, `ErrPeriodNotClosed`.
- `internal/service/service_test.go`, `internal/service/goal_test.go` — fake stores под новый интерфейс.
- `internal/http/server.go` — маршруты периодов.
- `internal/web/static/admin.js` — `PeriodsSection` (вложенная таблица, статусы, архив), источник данных `/api/v1/admin/periods`.
- `internal/web/static/tracker.js` — богатый селектор периодов.
- `seed_demo.sql` — периоды-дерево без `sort_order`, с одним архивным.
- `specs/020-domain-model.md`, `specs/030-user-flows.md`, `specs/040-api-contract.md`, `specs/050-permissions-and-lifecycle.md`.

---

## Task 1: Domain — статус периода + поле `ArchivedAt`

**Files:**
- Modify: `internal/domain/models.go` (Period struct, ~175-183)
- Create: `internal/domain/period_status.go`
- Test: `internal/domain/period_status_test.go`

**Interfaces:**
- Produces: `domain.PeriodStatus` (`string`), константы `PeriodStatusFuture/Active/Closed/Archived`; `func PeriodStatusFor(p Period, now time.Time) PeriodStatus`; поле `Period.ArchivedAt *time.Time`.

- [ ] **Step 1: Добавить поле `ArchivedAt` в `Period` (аддитивно, `SortOrder` пока оставить)**

В `internal/domain/models.go`:

```go
type Period struct {
	ID         int64
	Name       string
	StartDate  time.Time
	EndDate    time.Time
	SortOrder  int
	ArchivedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
```

- [ ] **Step 2: Написать падающий тест статуса**

Создать `internal/domain/period_status_test.go`:

```go
package domain

import (
	"testing"
	"time"
)

func TestPeriodStatusFor(t *testing.T) {
	d := func(y int, m time.Month, day int) time.Time {
		return time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
	}
	now := d(2026, time.July, 3)
	archived := d(2026, time.July, 1)

	cases := []struct {
		name string
		p    Period
		want PeriodStatus
	}{
		{"future", Period{StartDate: d(2026, time.October, 1), EndDate: d(2026, time.December, 31)}, PeriodStatusFuture},
		{"active", Period{StartDate: d(2026, time.July, 1), EndDate: d(2026, time.September, 30)}, PeriodStatusActive},
		{"closed", Period{StartDate: d(2026, time.January, 1), EndDate: d(2026, time.March, 31)}, PeriodStatusClosed},
		{"boundary_start_is_active", Period{StartDate: now, EndDate: d(2026, time.August, 1)}, PeriodStatusActive},
		{"boundary_end_is_active", Period{StartDate: d(2026, time.January, 1), EndDate: now}, PeriodStatusActive},
		{"archived_overrides", Period{StartDate: d(2026, time.January, 1), EndDate: d(2026, time.March, 31), ArchivedAt: &archived}, PeriodStatusArchived},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PeriodStatusFor(c.p, now); got != c.want {
				t.Fatalf("PeriodStatusFor(%s) = %q, want %q", c.name, got, c.want)
			}
		})
	}
}
```

- [ ] **Step 3: Запустить тест — убедиться, что не компилируется/падает**

Run: `go test ./internal/domain/ -run TestPeriodStatusFor`
Expected: FAIL — `undefined: PeriodStatusFor` / `PeriodStatusFuture`.

- [ ] **Step 4: Реализовать `period_status.go`**

Создать `internal/domain/period_status.go`:

```go
package domain

import "time"

// PeriodStatus — жизненный статус периода. future/active/closed выводятся из дат,
// archived выставляется вручную (Period.ArchivedAt).
type PeriodStatus string

const (
	PeriodStatusFuture   PeriodStatus = "future"
	PeriodStatusActive   PeriodStatus = "active"
	PeriodStatusClosed   PeriodStatus = "closed"
	PeriodStatusArchived PeriodStatus = "archived"
)

// PeriodStatusFor возвращает статус периода относительно now.
// Границы включительны: now == start и now == end → active.
func PeriodStatusFor(p Period, now time.Time) PeriodStatus {
	if p.ArchivedAt != nil {
		return PeriodStatusArchived
	}
	day := now.Truncate(24 * time.Hour)
	start := p.StartDate.Truncate(24 * time.Hour)
	end := p.EndDate.Truncate(24 * time.Hour)
	switch {
	case day.Before(start):
		return PeriodStatusFuture
	case day.After(end):
		return PeriodStatusClosed
	default:
		return PeriodStatusActive
	}
}
```

- [ ] **Step 5: Запустить тест — PASS, сборка зелёная**

Run: `go test ./internal/domain/ -run TestPeriodStatusFor && go build ./...`
Expected: PASS; сборка успешна (поле `ArchivedAt` аддитивно).

- [ ] **Step 6: Показать дифф пользователю** (вместо git commit — CLAUDE.md #8).

---

## Task 2: Domain — дерево периодов `BuildPeriodViews`

**Files:**
- Create: `internal/domain/period_tree.go`
- Test: `internal/domain/period_tree_test.go`

**Interfaces:**
- Consumes: `Period`, `PeriodStatusFor` (Task 1).
- Produces:
  - `type PeriodView struct { Period; ParentID *int64; Depth int; Status PeriodStatus }`
  - `func BuildPeriodViews(periods []Period, now time.Time) []PeriodView` — возвращает элементы в порядке отображения (DFS), с вычисленными `ParentID`, `Depth`, `Status`.

**Правила (реализуются в этом таске):**
- **Родитель** `C` = самый узкий период `A` (`A != C`), где `A.start ≤ C.start` и `A.end ≥ C.end` и `A` строго больше (`A.start < C.start` OR `A.end > C.end`). Тай-брейк: меньший span → позднейший start → ранний end → меньший id.
- **Порядок корней:** ранг статуса (`future/active` = 0, `closed` = 1, `archived` = 2) по возрастанию, затем `start_date` по убыванию (новые выше), затем id.
- **Порядок детей:** если статус родителя `future`/`active` — по `start_date` возрастанию; иначе — по убыванию. Тай-брейк — id.
- Обход в глубину: период, затем его поддерево.

- [ ] **Step 1: Написать падающий тест**

Создать `internal/domain/period_tree_test.go`:

```go
package domain

import (
	"testing"
	"time"
)

func d(y int, m time.Month, day int) time.Time {
	return time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
}

func ids(vs []PeriodView) []int64 {
	out := make([]int64, len(vs))
	for i, v := range vs {
		out[i] = v.ID
	}
	return out
}

func TestBuildPeriodViews_ParentAndDepth(t *testing.T) {
	now := d(2026, time.July, 3)
	ps := []Period{
		{ID: 1, Name: "Y2026", StartDate: d(2026, time.January, 1), EndDate: d(2026, time.December, 31)},
		{ID: 2, Name: "Q1", StartDate: d(2026, time.January, 1), EndDate: d(2026, time.March, 31)},
		{ID: 3, Name: "Q3", StartDate: d(2026, time.July, 1), EndDate: d(2026, time.September, 30)},
	}
	views := BuildPeriodViews(ps, now)
	byID := map[int64]PeriodView{}
	for _, v := range views {
		byID[v.ID] = v
	}
	if byID[1].ParentID != nil {
		t.Fatalf("Y2026 must be root")
	}
	if byID[2].ParentID == nil || *byID[2].ParentID != 1 || byID[2].Depth != 1 {
		t.Fatalf("Q1 parent must be Y2026 at depth 1, got parent=%v depth=%d", byID[2].ParentID, byID[2].Depth)
	}
	if byID[3].Status != PeriodStatusActive {
		t.Fatalf("Q3 must be active, got %s", byID[3].Status)
	}
}

func TestBuildPeriodViews_EqualRangesAreSiblings(t *testing.T) {
	now := d(2026, time.July, 3)
	ps := []Period{
		{ID: 1, StartDate: d(2026, time.January, 1), EndDate: d(2026, time.December, 31)},
		{ID: 2, StartDate: d(2026, time.January, 1), EndDate: d(2026, time.December, 31)},
	}
	views := BuildPeriodViews(ps, now)
	for _, v := range views {
		if v.ParentID != nil {
			t.Fatalf("identical ranges must be siblings, period %d got parent %v", v.ID, v.ParentID)
		}
	}
}

func TestBuildPeriodViews_Order(t *testing.T) {
	now := d(2026, time.July, 3)
	ps := []Period{
		{ID: 10, Name: "Y2025", StartDate: d(2025, time.January, 1), EndDate: d(2025, time.December, 31)},
		{ID: 11, Name: "Y2025-Q3", StartDate: d(2025, time.July, 1), EndDate: d(2025, time.September, 30)},
		{ID: 12, Name: "Y2025-Q4", StartDate: d(2025, time.October, 1), EndDate: d(2025, time.December, 31)},
		{ID: 1, Name: "Y2026", StartDate: d(2026, time.January, 1), EndDate: d(2026, time.December, 31)},
		{ID: 2, Name: "Y2026-Q1", StartDate: d(2026, time.January, 1), EndDate: d(2026, time.March, 31)},
		{ID: 3, Name: "Y2026-Q3", StartDate: d(2026, time.July, 1), EndDate: d(2026, time.September, 30)},
	}
	got := ids(BuildPeriodViews(ps, now))
	// active year 2026 first (children ascending), then closed year 2025 (children descending).
	want := []int64{1, 2, 3, 10, 12, 11}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order mismatch: got %v want %v", got, want)
		}
	}
}
```

- [ ] **Step 2: Запустить — FAIL**

Run: `go test ./internal/domain/ -run TestBuildPeriodViews`
Expected: FAIL — `undefined: BuildPeriodViews` / `PeriodView`.

- [ ] **Step 3: Реализовать `period_tree.go`**

Создать `internal/domain/period_tree.go`:

```go
package domain

import (
	"sort"
	"time"
)

// PeriodView — период с вычисленными на чтении полями дерева и статусом.
type PeriodView struct {
	Period
	ParentID *int64
	Depth    int
	Status   PeriodStatus
}

// BuildPeriodViews вычисляет родителя (по строгому вхождению интервалов),
// глубину, статус и порядок отображения. Порядок: актуальные+будущие корни
// сверху, затем закрытые, затем архивные; новые годы выше; дети под родителем
// (в актуальных/будущих — по возрастанию дат, в прошедших — по убыванию).
func BuildPeriodViews(periods []Period, now time.Time) []PeriodView {
	n := len(periods)
	status := make(map[int64]PeriodStatus, n)
	for _, p := range periods {
		status[p.ID] = PeriodStatusFor(p, now)
	}

	// span в днях; для тай-брейка узости.
	span := func(p Period) int {
		return int(p.EndDate.Sub(p.StartDate).Hours() / 24)
	}
	contains := func(a, c Period) bool {
		if a.ID == c.ID {
			return false
		}
		if a.StartDate.After(c.StartDate) || a.EndDate.Before(c.EndDate) {
			return false
		}
		return a.StartDate.Before(c.StartDate) || a.EndDate.After(c.EndDate)
	}

	parent := make(map[int64]*int64, n)
	for _, c := range periods {
		var best *Period
		for i := range periods {
			a := periods[i]
			if !contains(a, c) {
				continue
			}
			if best == nil {
				b := a
				best = &b
				continue
			}
			// узость: меньший span, затем позднейший start, ранний end, меньший id.
			as, bs := span(a), span(*best)
			switch {
			case as != bs:
				if as < bs {
					b := a
					best = &b
				}
			case !a.StartDate.Equal(best.StartDate):
				if a.StartDate.After(best.StartDate) {
					b := a
					best = &b
				}
			case !a.EndDate.Equal(best.EndDate):
				if a.EndDate.Before(best.EndDate) {
					b := a
					best = &b
				}
			default:
				if a.ID < best.ID {
					b := a
					best = &b
				}
			}
		}
		if best != nil {
			pid := best.ID
			parent[c.ID] = &pid
		} else {
			parent[c.ID] = nil
		}
	}

	// depth по цепочке родителей.
	depth := make(map[int64]int, n)
	var calcDepth func(id int64) int
	calcDepth = func(id int64) int {
		if dp, ok := depth[id]; ok {
			return dp
		}
		pid := parent[id]
		if pid == nil {
			depth[id] = 0
			return 0
		}
		dp := calcDepth(*pid) + 1
		depth[id] = dp
		return dp
	}
	for _, p := range periods {
		calcDepth(p.ID)
	}

	byID := make(map[int64]Period, n)
	for _, p := range periods {
		byID[p.ID] = p
	}
	children := make(map[int64][]int64)
	var roots []int64
	for _, p := range periods {
		if pid := parent[p.ID]; pid != nil {
			children[*pid] = append(children[*pid], p.ID)
		} else {
			roots = append(roots, p.ID)
		}
	}

	rootRank := func(s PeriodStatus) int {
		switch s {
		case PeriodStatusClosed:
			return 1
		case PeriodStatusArchived:
			return 2
		default: // future, active
			return 0
		}
	}
	sort.SliceStable(roots, func(i, j int) bool {
		a, b := byID[roots[i]], byID[roots[j]]
		ra, rb := rootRank(status[a.ID]), rootRank(status[b.ID])
		if ra != rb {
			return ra < rb
		}
		if !a.StartDate.Equal(b.StartDate) {
			return a.StartDate.After(b.StartDate) // новые выше
		}
		return a.ID < b.ID
	})

	sortChildren := func(parentID int64) {
		kids := children[parentID]
		asc := status[parentID] == PeriodStatusFuture || status[parentID] == PeriodStatusActive
		sort.SliceStable(kids, func(i, j int) bool {
			a, b := byID[kids[i]], byID[kids[j]]
			if !a.StartDate.Equal(b.StartDate) {
				if asc {
					return a.StartDate.Before(b.StartDate)
				}
				return a.StartDate.After(b.StartDate)
			}
			return a.ID < b.ID
		})
	}

	out := make([]PeriodView, 0, n)
	var walk func(id int64)
	walk = func(id int64) {
		p := byID[id]
		var pid *int64
		if v := parent[id]; v != nil {
			cp := *v
			pid = &cp
		}
		out = append(out, PeriodView{Period: p, ParentID: pid, Depth: depth[id], Status: status[id]})
		sortChildren(id)
		for _, kid := range children[id] {
			walk(kid)
		}
	}
	for _, r := range roots {
		walk(r)
	}
	return out
}
```

- [ ] **Step 4: Запустить — PASS**

Run: `go test ./internal/domain/ -run TestBuildPeriodViews`
Expected: PASS (все три теста).

- [ ] **Step 5: Показать дифф пользователю.**

---

## Task 3: Убрать `sort_order` end-to-end + плюмбинг `archived_at` + миграция + seed

Крупный, но цельный таск: удаление старого механизма порядка и проводка `archived_at` через все слои, чтобы `go build ./... && go test ./...` были зелёными. Новые архив/лист-эндпоинты — в следующих тасках.

**Files:**
- Create: `migrations/036_period_nesting.up.sql`, `migrations/036_period_nesting.down.sql`
- Modify: `internal/domain/models.go`, `internal/store/periods/periods.go`, `internal/store/periods/periods_test.go`, `internal/store/periods/periods_isolation_test.go`, `internal/http/dto/period.go`, `internal/http/handlers/api/v1/helpers_response.go`, `internal/service/service.go`, `internal/service/service_test.go`, `internal/service/goal_test.go`, `internal/http/handlers/api/v1/admin/service_handler.go`, `internal/http/server.go`, `seed_demo.sql`

**Interfaces:**
- Consumes: `domain.PeriodStatusFor` (Task 1).
- Produces: `dto.PeriodInfo{ID, Name, StartDate, EndDate, ParentID *int64, Depth int, Status string}`; `PeriodRepo` без `MovePeriod`; `Period` без `SortOrder`.

- [ ] **Step 1: Миграция up/down**

`migrations/036_period_nesting.up.sql`:

```sql
ALTER TABLE periods ADD COLUMN archived_at TIMESTAMPTZ;
ALTER TABLE periods DROP COLUMN sort_order;
```

`migrations/036_period_nesting.down.sql`:

```sql
ALTER TABLE periods ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0;
ALTER TABLE periods DROP COLUMN archived_at;
```

- [ ] **Step 2: Обновить seed_demo.sql**

Заменить блок вставки периодов (сейчас `INSERT INTO periods (id, name, start_date, end_date, sort_order, created_at, updated_at) VALUES … ids 2,1,3,4`) на дерево без `sort_order`, с колонкой `archived_at`. Сохранить id 1..4 (на них ссылаются goals), добавить новые:

```sql
INSERT INTO periods (id, name, start_date, end_date, archived_at, created_at, updated_at) VALUES
  (2, 'Y2026',       '2026-01-01', '2026-12-31', NULL, NOW(), NOW()),
  (1, 'Q1 · 2026',   '2026-01-01', '2026-03-31', NULL, NOW(), NOW()),
  (3, 'Q2 · 2026',   '2026-04-01', '2026-06-30', NULL, NOW(), NOW()),
  (4, 'Q3 · 2026',   '2026-07-01', '2026-09-30', NULL, NOW(), NOW()),
  (5, 'Q4 · 2026',   '2026-10-01', '2026-12-31', NULL, NOW(), NOW()),
  (6, 'Y2025',       '2025-01-01', '2025-12-31', NULL, NOW(), NOW()),
  (7, 'Q3 · 2025',   '2025-07-01', '2025-09-30', NULL, NOW(), NOW()),
  (8, 'Q4 · 2025',   '2025-10-01', '2025-12-31', NULL, NOW(), NOW()),
  (9, 'Q2 · 2025',   '2025-04-01', '2025-06-30', NOW(), NOW(), NOW());
```

(Период id=9 архивный — для демонстрации; целей не имеет. `setval('periods_id_seq', …)` в конце файла уже пересчитывает max(id), править не нужно.) Примечание: раньше Q1 заканчивался 2026-05-31 — исправлено на 2026-03-31, чтобы даты не пересекались с Q2 (иначе Q1 и Q2 стали бы сиблингами с пересечением, а не чистыми кварталами).

- [ ] **Step 3: Обновить падающие тесты стора (TDD-red через миграцию)**

В `internal/store/periods/periods_test.go` и `periods_isolation_test.go` убрать любые ссылки на `SortOrder`, `MovePeriod`, `move-up/down`. Добавить проверку архивации в CRUD-тест (в конце `TestPeriodsCRUD`, перед удалением):

```go
	if err := r.ArchivePeriod(ctx, sc1, id); err != nil {
		t.Fatalf("ArchivePeriod: %v", err)
	}
	pa, _ := r.GetPeriod(ctx, sc1, id)
	if pa.ArchivedAt == nil {
		t.Fatal("expected archived_at set")
	}
	if err := r.UnarchivePeriod(ctx, sc1, id); err != nil {
		t.Fatalf("UnarchivePeriod: %v", err)
	}
	pu, _ := r.GetPeriod(ctx, sc1, id)
	if pu.ArchivedAt != nil {
		t.Fatal("expected archived_at cleared")
	}
```

- [ ] **Step 4: Запустить тесты стора — FAIL/не компилируется**

Run: `go test ./internal/store/periods/`
Expected: FAIL — нет `ArchivePeriod`/`UnarchivePeriod`; при подключённой миграции запросы с `sort_order` тоже упадут.

- [ ] **Step 5: `Period` без `SortOrder`**

В `internal/domain/models.go`:

```go
type Period struct {
	ID         int64
	Name       string
	StartDate  time.Time
	EndDate    time.Time
	ArchivedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
```

- [ ] **Step 6: Репозиторий — запросы, `FindPeriodForDate`, archive/unarchive, убрать `MovePeriod`**

В `internal/store/periods/periods.go` заменить select-list `id, name, start_date, end_date, sort_order, created_at, updated_at` на `id, name, start_date, end_date, archived_at, created_at, updated_at` во всех трёх запросах (`ListPeriods`, `GetPeriod`, `FindPeriodForDate`) и `Scan(... &period.EndDate, &period.ArchivedAt, &period.CreatedAt, ...)`.

`ListPeriods` — убрать `sort_order` из `ORDER BY`:

```go
		ORDER BY start_date, id`, scope.TenantID)
```

`FindPeriodForDate` — выбирать самый узкий содержащий период:

```go
	row := r.db.QueryRow(ctx, `
		SELECT id, name, start_date, end_date, archived_at, created_at, updated_at
		FROM periods
		WHERE tenant_id=$2 AND $1::date BETWEEN start_date AND end_date
		ORDER BY (end_date - start_date) ASC, end_date DESC
		LIMIT 1`, date, scope.TenantID)
```

`CreatePeriod` — убрать `sort_order`:

```go
	row := r.db.QueryRow(ctx, `
		INSERT INTO periods (name, start_date, end_date, tenant_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id`, input.Name, input.StartDate, input.EndDate, scope.TenantID)
```

Удалить метод `MovePeriod` целиком. Добавить:

```go
func (r *PeriodRepository) ArchivePeriod(ctx context.Context, scope domain.TenantScope, periodID int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE periods SET archived_at=NOW(), updated_at=NOW()
		WHERE id=$1 AND tenant_id=$2`, periodID, scope.TenantID)
	return err
}

func (r *PeriodRepository) UnarchivePeriod(ctx context.Context, scope domain.TenantScope, periodID int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE periods SET archived_at=NULL, updated_at=NOW()
		WHERE id=$1 AND tenant_id=$2`, periodID, scope.TenantID)
	return err
}
```

- [ ] **Step 7: DTO — `PeriodInfo`**

`internal/http/dto/period.go`:

```go
type PeriodInfo struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	StartDate time.Time `json:"start_date"`
	EndDate   time.Time `json:"end_date"`
	ParentID  *int64    `json:"parent_id"`
	Depth     int       `json:"depth"`
	Status    string    `json:"status"`
}
```

- [ ] **Step 8: Маппер — `MapPeriodInfo` (+ `MapPeriodView`)**

В `internal/http/handlers/api/v1/helpers_response.go` заменить `MapPeriodInfo` и добавить `MapPeriodView`:

```go
func MapPeriodInfo(period domain.Period) dto.PeriodInfo {
	return dto.PeriodInfo{
		ID:        period.ID,
		Name:      period.Name,
		StartDate: period.StartDate,
		EndDate:   period.EndDate,
		Status:    string(domain.PeriodStatusFor(period, time.Now())),
	}
}

func MapPeriodView(v domain.PeriodView) dto.PeriodInfo {
	return dto.PeriodInfo{
		ID:        v.ID,
		Name:      v.Name,
		StartDate: v.StartDate,
		EndDate:   v.EndDate,
		ParentID:  v.ParentID,
		Depth:     v.Depth,
		Status:    string(v.Status),
	}
}
```

(`MapPeriodInfo` используется в overview для одиночного периода — там дерево не нужно, `ParentID`/`Depth` нулевые, статус считается из дат.)

- [ ] **Step 9: Сервис — интерфейс и методы**

В `internal/service/service.go`: в интерфейсе `PeriodRepo` убрать строку `MovePeriod(...)` и добавить:

```go
	ArchivePeriod(ctx context.Context, scope domain.TenantScope, periodID int64) error
	UnarchivePeriod(ctx context.Context, scope domain.TenantScope, periodID int64) error
```

Удалить метод-обёртку `func (s *Service) MovePeriod(...)`. (Методы `ArchivePeriod`/`UnarchivePeriod`/`ListPeriodViews` на сервисе добавим в Task 4/5 — пока интерфейс объявляет репо-методы, чтобы `PeriodRepository` их удовлетворял.)

- [ ] **Step 10: Fake stores в тестах сервиса**

В `internal/service/service_test.go` и `internal/service/goal_test.go` удалить методы `MovePeriod` у `fakeStore`/`goalFakeStore` и добавить no-op:

```go
func (f *fakeStore) ArchivePeriod(_ context.Context, _ domain.TenantScope, _ int64) error   { return nil }
func (f *fakeStore) UnarchivePeriod(_ context.Context, _ domain.TenantScope, _ int64) error { return nil }
```

(Аналогично для `goalFakeStore`.)

- [ ] **Step 11: Убрать move-хендлеры и маршруты**

В `internal/http/handlers/api/v1/admin/service_handler.go` удалить `HandleMovePeriodUp`, `HandleMovePeriodDown`, `handleMovePeriod`.

В `internal/http/server.go` удалить строки:

```go
r.Post("/api/v1/admin/periods/{periodID}/move-up", serviceH.HandleMovePeriodUp)
r.Post("/api/v1/admin/periods/{periodID}/move-down", serviceH.HandleMovePeriodDown)
```

- [ ] **Step 12: Сборка и все тесты — зелёные**

Run: `go build ./... && go test ./...`
Expected: PASS. (Если `periods_isolation_test.go` проверял `MAX(sort_order)+1` при создании — заменить проверку на факт создания/возврата id.)

- [ ] **Step 13: Показать дифф пользователю.**

---

## Task 4: Сервис — archive/unarchive с валидацией «только из closed»

**Files:**
- Modify: `internal/service/service.go`
- Test: `internal/service/service_test.go`

**Interfaces:**
- Consumes: `PeriodRepo.GetPeriod/ArchivePeriod/UnarchivePeriod`, `domain.PeriodStatusFor`.
- Produces: `var ErrPeriodNotClosed`; `func (s *Service) ArchivePeriod(ctx, scope, id) error`; `func (s *Service) UnarchivePeriod(ctx, scope, id) error`.

- [ ] **Step 1: Падающий тест сервиса**

В `internal/service/service_test.go` добавить (fakeStore уже отдаёт периоды — использовать существующий способ подготовки; если `GetPeriod` у fake возвращает пустое, задать поле с датами). Пример с локальным fake, если существующий не позволяет задать период, — добавить в `fakeStore` поле `getPeriod domain.Period` и вернуть его из `GetPeriod`:

```go
func TestArchivePeriod_RejectsNonClosed(t *testing.T) {
	now := time.Now()
	fs := &fakeStore{getPeriod: domain.Period{
		ID: 1, StartDate: now.AddDate(0, 0, -1), EndDate: now.AddDate(0, 0, 10), // active
	}}
	svc := newTestService(fs) // как в соседних тестах пакета
	err := svc.ArchivePeriod(context.Background(), domain.TenantScope{TenantID: 1}, 1)
	if !errors.Is(err, service.ErrPeriodNotClosed) {
		t.Fatalf("expected ErrPeriodNotClosed, got %v", err)
	}
}

func TestArchivePeriod_AllowsClosed(t *testing.T) {
	now := time.Now()
	fs := &fakeStore{getPeriod: domain.Period{
		ID: 1, StartDate: now.AddDate(0, 0, -30), EndDate: now.AddDate(0, 0, -1), // closed
	}}
	svc := newTestService(fs)
	if err := svc.ArchivePeriod(context.Background(), domain.TenantScope{TenantID: 1}, 1); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}
```

(Согласовать конструктор `newTestService`/имя fake с существующими тестами пакета — переиспользовать их хелперы. Добавить поле `getPeriod` и вернуть его из метода `GetPeriod` fake-стора, если его ещё нет.)

- [ ] **Step 2: Запустить — FAIL**

Run: `go test ./internal/service/ -run TestArchivePeriod`
Expected: FAIL — `undefined: (*Service).ArchivePeriod` / `ErrPeriodNotClosed`.

- [ ] **Step 3: Реализовать**

Рядом с другими `Err…` (service.go ~141-145) добавить:

```go
	ErrPeriodNotClosed = errors.New("period must be closed to archive")
```

Заменить/добавить методы периода:

```go
func (s *Service) ArchivePeriod(ctx context.Context, scope domain.TenantScope, periodID int64) error {
	p, err := s.periods.GetPeriod(ctx, scope, periodID)
	if err != nil {
		return err
	}
	if domain.PeriodStatusFor(p, time.Now()) != domain.PeriodStatusClosed {
		return ErrPeriodNotClosed
	}
	return s.periods.ArchivePeriod(ctx, scope, periodID)
}

func (s *Service) UnarchivePeriod(ctx context.Context, scope domain.TenantScope, periodID int64) error {
	return s.periods.UnarchivePeriod(ctx, scope, periodID)
}
```

- [ ] **Step 4: Запустить — PASS**

Run: `go test ./internal/service/ -run TestArchivePeriod && go build ./...`
Expected: PASS.

- [ ] **Step 5: Показать дифф пользователю.**

---

## Task 5: Списки периодов через views (публичный без архивных + admin GET)

**Files:**
- Modify: `internal/service/service.go`, `internal/http/handlers/api/v1/periods/response.go`, `internal/http/handlers/api/v1/periods/handler.go`, `internal/http/handlers/api/v1/admin/service_handler.go`, `internal/http/server.go`
- Test: `internal/http/handlers/api/v1/periods/routes_test.go` (или новый handler-тест), `internal/service/service_test.go`

**Interfaces:**
- Consumes: `domain.BuildPeriodViews`, `MapPeriodView`.
- Produces: `func (s *Service) ListPeriodViews(ctx, scope, includeArchived bool) ([]domain.PeriodView, error)`; `func (h *ServiceHandler) HandleListPeriods(w, r)` (admin, все включая архивные); публичный `HandlePeriods` — без архивных.

- [ ] **Step 1: Падающий тест сервиса (фильтр архивных)**

```go
func TestListPeriodViews_ExcludesArchivedForPublic(t *testing.T) {
	now := time.Now()
	arch := now.AddDate(0, 0, -2)
	fs := &fakeStore{listPeriods: []domain.Period{
		{ID: 1, StartDate: now.AddDate(0, 0, -30), EndDate: now.AddDate(0, 0, -1)},          // closed
		{ID: 2, StartDate: now.AddDate(0, 0, -30), EndDate: now.AddDate(0, 0, -1), ArchivedAt: &arch}, // archived
	}}
	svc := newTestService(fs)
	pub, _ := svc.ListPeriodViews(context.Background(), domain.TenantScope{TenantID: 1}, false)
	if len(pub) != 1 || pub[0].ID != 1 {
		t.Fatalf("public must exclude archived, got %+v", pub)
	}
	all, _ := svc.ListPeriodViews(context.Background(), domain.TenantScope{TenantID: 1}, true)
	if len(all) != 2 {
		t.Fatalf("admin must include archived, got %d", len(all))
	}
}
```

(`fakeStore` дополнить полем `listPeriods []domain.Period`, возвращать его из `ListPeriods`.)

- [ ] **Step 2: Запустить — FAIL**

Run: `go test ./internal/service/ -run TestListPeriodViews`
Expected: FAIL — `undefined: (*Service).ListPeriodViews`.

- [ ] **Step 3: Реализовать `ListPeriodViews`**

В `internal/service/service.go` (рядом с `ListPeriods`):

```go
func (s *Service) ListPeriodViews(ctx context.Context, scope domain.TenantScope, includeArchived bool) ([]domain.PeriodView, error) {
	all, err := s.periods.ListPeriods(ctx, scope)
	if err != nil {
		return nil, err
	}
	src := all
	if !includeArchived {
		src = make([]domain.Period, 0, len(all))
		for _, p := range all {
			if p.ArchivedAt == nil {
				src = append(src, p)
			}
		}
	}
	return domain.BuildPeriodViews(src, time.Now()), nil
}
```

(Фильтрация архивных до `BuildPeriodViews` гарантирует, что `parent_id` в публичном списке не указывает на скрытый период.)

- [ ] **Step 4: Публичный список — через views без архивных**

`internal/http/handlers/api/v1/periods/response.go`:

```go
func newPeriodsResponse(views []domain.PeriodView) dto.PeriodsResponse {
	items := make([]dto.PeriodInfo, 0, len(views))
	for _, v := range views {
		items = append(items, v1.MapPeriodView(v))
	}
	return dto.PeriodsResponse{Items: items}
}
```

`internal/http/handlers/api/v1/periods/handler.go` — заменить вызов:

```go
	views, err := h.service.ListPeriodViews(r.Context(), scope, false)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to load periods", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, newPeriodsResponse(views))
```

- [ ] **Step 5: Admin GET-хендлер (все, включая архивные)**

В `internal/http/handlers/api/v1/admin/service_handler.go` добавить (использует те же мапперы; импортировать `domain`/`v1` уже есть):

```go
// GET /api/v1/admin/periods
func (h *ServiceHandler) HandleListPeriods(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	views, err := h.service.ListPeriodViews(r.Context(), scope, true)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to load periods", nil)
		return
	}
	items := make([]dto.PeriodInfo, 0, len(views))
	for _, v := range views {
		items = append(items, v1.MapPeriodView(v))
	}
	v1.WriteJSON(w, http.StatusOK, dto.PeriodsResponse{Items: items})
}
```

Добавить импорт `"okrs/internal/http/dto"` в этот файл, если его ещё нет.

В `internal/http/server.go` рядом с admin-периодами добавить маршрут:

```go
r.Get("/api/v1/admin/periods", serviceH.HandleListPeriods)
```

- [ ] **Step 6: Запустить тесты**

Run: `go test ./internal/service/ ./internal/http/... && go build ./...`
Expected: PASS.

- [ ] **Step 7: Показать дифф пользователю.**

---

## Task 6: Admin archive/unarchive эндпоинты

**Files:**
- Modify: `internal/http/handlers/api/v1/admin/service_handler.go`, `internal/http/server.go`
- Test: новый `internal/http/handlers/api/v1/admin/periods_archive_test.go` (или в существующий admin-тест)

**Interfaces:**
- Consumes: `service.ArchivePeriod/UnarchivePeriod`, `service.ErrPeriodNotClosed`.
- Produces: `HandleArchivePeriod`, `HandleUnarchivePeriod`; маршруты `POST …/archive`, `POST …/unarchive`.

- [ ] **Step 1: Падающий тест — 409 на не-closed**

Создать `internal/http/handlers/api/v1/admin/periods_archive_test.go` по образцу существующих admin-хендлер-тестов (собрать `ServiceHandler` с сервисом на fakeStore, чей `GetPeriod` возвращает активный период). Проверить, что `HandleArchivePeriod` пишет `409`:

```go
func TestHandleArchivePeriod_Conflict(t *testing.T) {
	now := time.Now()
	// сервис с активным периодом id=1
	h := newAdminHandlerWithPeriod(t, domain.Period{ID: 1, StartDate: now.AddDate(0, 0, -1), EndDate: now.AddDate(0, 0, 5)})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/periods/1/archive", nil)
	req = withTenantScope(req, domain.TenantScope{TenantID: 1})
	req = withURLParam(req, "periodID", "1")
	w := httptest.NewRecorder()
	h.HandleArchivePeriod(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}
```

(Хелперы `newAdminHandlerWithPeriod`, `withTenantScope`, `withURLParam` — по образцу существующих тестов пакета admin; если их нет, собрать сервис/скоуп так же, как в соседних тестах.)

- [ ] **Step 2: Запустить — FAIL**

Run: `go test ./internal/http/handlers/api/v1/admin/ -run TestHandleArchivePeriod`
Expected: FAIL — `undefined: HandleArchivePeriod`.

- [ ] **Step 3: Реализовать хендлеры**

В `service_handler.go`:

```go
// POST /api/v1/admin/periods/{periodID}/archive
func (h *ServiceHandler) HandleArchivePeriod(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	periodID, err := common.ParseID(chi.URLParam(r, "periodID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid period id", nil)
		return
	}
	if err := h.service.ArchivePeriod(r.Context(), scope, periodID); err != nil {
		if errors.Is(err, service.ErrPeriodNotClosed) {
			v1.WriteError(w, http.StatusConflict, "CONFLICT", "only a closed period can be archived", nil)
			return
		}
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to archive period", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// POST /api/v1/admin/periods/{periodID}/unarchive
func (h *ServiceHandler) HandleUnarchivePeriod(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	periodID, err := common.ParseID(chi.URLParam(r, "periodID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid period id", nil)
		return
	}
	if err := h.service.UnarchivePeriod(r.Context(), scope, periodID); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to unarchive period", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
```

Добавить импорты `"errors"` и `"okrs/internal/service"` в файл, если их ещё нет.

В `internal/http/server.go`:

```go
r.Post("/api/v1/admin/periods/{periodID}/archive", serviceH.HandleArchivePeriod)
r.Post("/api/v1/admin/periods/{periodID}/unarchive", serviceH.HandleUnarchivePeriod)
```

- [ ] **Step 4: Запустить — PASS**

Run: `go test ./internal/http/... && go build ./...`
Expected: PASS.

- [ ] **Step 5: Показать дифф пользователю.**

---

## Task 7: admin.js — `PeriodsSection` (вложенная таблица, статусы, архив)

**Files:**
- Modify: `internal/web/static/admin.js` (bootstrap-загрузка периодов ~1453; `PeriodsSection` 355-423; `PeriodEditor` 425-450)

**Interfaces:**
- Consumes: `GET /api/v1/admin/periods` (поля `parent_id`, `depth`, `status`), `POST …/{id}/archive`, `POST …/{id}/unarchive`.

- [ ] **Step 1: Переключить источник данных админки на admin-эндпоинт**

В bootstrap (строка ~1453) заменить `apiGet('/api/v1/periods')` на `apiGet('/api/v1/admin/periods')`, чтобы админка видела архивные.

- [ ] **Step 2: Хелперы статуса**

В начало секции периодов (перед `PeriodsSection`) добавить:

```js
const PERIOD_STATUS = {
  future:   {label:'Планируется', dot:'#3b82f6', bg:'#dbeafe', fg:'#1e40af'},
  active:   {label:'В работе',     dot:'#22c55e', bg:'#dcfce7', fg:'#166534'},
  closed:   {label:'Закрыто',      dot:'#9ca3af', bg:'#f3f4f6', fg:'#4b5563'},
  archived: {label:'Архив',        dot:'#9ca3af', bg:'#f3f4f6', fg:'#6b7280'},
};
function PeriodBadge({status}) {
  const s = PERIOD_STATUS[status] || PERIOD_STATUS.closed;
  return <span style={{display:'inline-flex',alignItems:'center',gap:6,padding:'2px 8px',borderRadius:999,background:s.bg,color:s.fg,fontSize:11,fontWeight:600}}>
    <span style={{width:7,height:7,borderRadius:999,background:s.dot}}/>{s.label}
  </span>;
}
```

- [ ] **Step 3: Переписать `PeriodsSection` под дерево + архив**

Заменить тело `PeriodsSection` (данные уже приходят упорядоченными и с `depth`; порядок с сервера сохраняем):

```js
function PeriodsSection({periods, reload}) {
  const [q, setQ] = useState('');
  const [selId, setSelId] = useState(null);
  const [creating, setCreating] = useState(false);
  const [createParent, setCreateParent] = useState(null);
  const [saving, setSaving] = useState(false);

  const filtered = periods.filter(p=>!q||p.name.toLowerCase().includes(q.toLowerCase()));
  const selected = creating ? null : periods.find(p=>p.id===selId);

  async function remove(id, name) {
    if (!confirm(`Удалить период «${name}»? Цели внутри останутся, но не будут отображаться.`)) return;
    const res = await apiDel(`/api/v1/admin/periods/${id}`);
    if (res && res.ok) { if(selId===id)setSelId(null); reload(); }
    else alert('Ошибка удаления периода');
  }
  async function toggleArchive(p) {
    const ep = p.status==='archived' ? 'unarchive' : 'archive';
    const res = await apiPost(`/api/v1/admin/periods/${p.id}/${ep}`, {});
    if (res && res.ok) reload();
    else if (res && res.status===409) alert('Архивировать можно только закрытый период.');
    else alert('Ошибка изменения статуса');
  }
  async function save(f) {
    setSaving(true);
    try {
      const body = {name: f.name.trim(), start_date: f.start_date, end_date: f.end_date};
      let res;
      if (f.id) res = await apiPatch(`/api/v1/admin/periods/${f.id}`, body);
      else        res = await apiPost('/api/v1/admin/periods', body);
      if (!res || !res.ok) { alert('Ошибка сохранения'); return; }
      if (!f.id) { const data = await res.json(); setSelId(data.id); }
      else setSelId(f.id);
      setCreating(false); setCreateParent(null);
      reload();
    } finally { setSaving(false); }
  }

  const createInitial = createParent
    ? {name:'', start_date: fmtDate(createParent.start_date), end_date: fmtDate(createParent.end_date)}
    : {name:'', start_date:'', end_date:''};

  return <MasterDetail
    toolbar={<div style={{display:'flex',gap:8,alignItems:'center'}}>
      <ListSearch value={q} onChange={setQ} placeholder="Поиск периода…"/>
      <Btn variant="primary" size="sm" onClick={()=>{setCreating(true);setCreateParent(null);setSelId(null);}}>+ Период</Btn>
    </div>}
    listHeader={`Всего · ${periods.length}`}
    list={filtered.map((p)=>{
      const sel=p.id===selId&&!creating;
      return <ListRow key={p.id} selected={sel} onClick={()=>{setSelId(p.id);setCreating(false);}}>
        <div style={{width: 12 + p.depth*18}}/>
        <div style={{flex:1,minWidth:0}}>
          <div style={{fontSize:13.5,fontWeight:600,color:T.headingFg}}>{p.name}</div>
          <div style={{fontSize:11,color:T.mutedFg,fontFamily:'ui-monospace,Menlo,monospace',marginTop:2}}>{fmtDate(p.start_date)} → {fmtDate(p.end_date)}</div>
        </div>
        <PeriodBadge status={p.status}/>
        <button onClick={e=>{e.stopPropagation();setCreating(true);setCreateParent(p);setSelId(null);}} title="Вложенный период"
          style={{marginLeft:8,fontSize:11,color:T.accent,background:'none',border:'none',cursor:'pointer'}}>+ вложенный</button>
      </ListRow>;
    })}
    detail={
      creating
        ? <PeriodEditor value={createInitial} onSave={save} onCancel={()=>{setCreating(false);setCreateParent(null);}} saving={saving}/>
        : selected
          ? <PeriodEditor value={selected} onSave={save}
              onDelete={()=>remove(selected.id,selected.name)}
              onArchive={()=>toggleArchive(selected)} saving={saving}/>
          : <EmptyDetail icon="📅" title="Выберите период" hint="Кликните по периоду слева или создайте новый."/>
    }
  />;
}
```

- [ ] **Step 4: Кнопка архива в `PeriodEditor`**

В `PeriodEditor` расширить сигнатуру и добавить кнопку архива (только для существующего периода; для `closed` — «Архивировать», для `archived` — «Разархивировать»):

```js
function PeriodEditor({value, onSave, onCancel, onDelete, onArchive, saving}) {
  // ...без изменений до actions...
      actions={<>
        {!isNew && onArchive && (value.status==='closed' || value.status==='archived') &&
          <Btn onClick={onArchive} disabled={saving}>{value.status==='archived'?'Разархивировать':'Архивировать'}</Btn>}
        {!isNew&&<Btn danger onClick={onDelete} disabled={saving}>Удалить</Btn>}
        {isNew&&<Btn onClick={onCancel} disabled={saving}>Отмена</Btn>}
        <Btn variant="primary" onClick={()=>canSave&&onSave(f)} disabled={!canSave||saving}>{saving?'Сохранение…':isNew?'Создать':'Сохранить'}</Btn>
      </>}
  // ...
}
```

- [ ] **Step 5: Проверить в браузере**

Run: собрать/запустить приложение (`/run` skill или `go run ./...`), открыть `/admin?section=periods`.
Expected: вложенная таблица год→кварталы с отступами, бейджи статусов, «+ вложенный» преднаполняет даты родителя, «Архивировать» доступна только у закрытого периода, архивный период виден в админке.

- [ ] **Step 6: Показать дифф пользователю.**

---

## Task 8: tracker.js — богатый селектор периодов

**Files:**
- Modify: `internal/web/static/tracker.js` (селектор ~2042-2058; данные уже приходят с `/api/v1/periods` с `parent_id/depth/status`, без архивных)

**Interfaces:**
- Consumes: `periods` из состояния (уже упорядочены сервером, содержат `depth`, `status`).

- [ ] **Step 1: Хелпер статусов (рядом с компонентом сайдбара)**

```js
const TRK_PERIOD_STATUS = {
  future:   {label:'Планируется', dot:'#3b82f6'},
  active:   {label:'В работе',     dot:'#22c55e'},
  closed:   {label:'Закрыто',      dot:'#9ca3af'},
};
```

- [ ] **Step 2: Компонент `PeriodSelect`**

Добавить компонент (кастомный dropdown; массив `periods` уже отсортирован и с `depth`):

```js
function PeriodSelect({periods, periodId, onChange}) {
  const [open, setOpen] = useState(false);
  const cur = periods.find(p=>p.id===periodId);
  const st = cur && TRK_PERIOD_STATUS[cur.status];
  return <div className="period-select" style={{position:'relative'}}>
    <button type="button" className="period-select__trigger" onClick={()=>setOpen(o=>!o)}>
      {st && <span className="period-select__dot" style={{background:st.dot}}/>}
      <span className="period-select__name">{cur ? cur.name : '—'}</span>
      <span className="period-select__chev">▾</span>
    </button>
    {open && <>
      <div className="period-select__backdrop" onClick={()=>setOpen(false)}/>
      <div className="period-select__menu">
        <div className="period-select__group">Актуальные и будущие</div>
        {periods.map(p=>{
          const s = TRK_PERIOD_STATUS[p.status] || TRK_PERIOD_STATUS.closed;
          return <button key={p.id} type="button"
            className={'period-select__item'+(p.id===periodId?' is-selected':'')}
            style={{paddingLeft: 12 + p.depth*16}}
            onClick={()=>{onChange(p.id);setOpen(false);}}>
            <span className="period-select__dot" style={{background:s.dot}}/>
            <span className="period-select__item-name">{p.name}</span>
            <span className="period-select__range">{fmtDateRange(p.start_date, p.end_date)}</span>
            <span className="period-select__badge">{s.label}</span>
          </button>;
        })}
      </div>
    </>}
  </div>;
}
```

Если хелпера `fmtDateRange` нет — добавить простой: `function fmtDateRange(a,b){return `${fmtDate(a)} – ${fmtDate(b)}`;}` (переиспользовать существующий `fmtDate` из tracker.js; проверить его наличие, иначе форматировать `new Date(x).toLocaleDateString('ru-RU')`).

- [ ] **Step 3: Встроить в сайдбар вместо `<select>`**

Заменить блок (строки ~2056-2058):

```js
<PeriodSelect periods={periods} periodId={periodId} onChange={(id)=>handlePeriodChange(id)} />
```

Убедиться, что `handlePeriodChange` принимает id (сейчас принимает `e.target.value` — привести к числу внутри или передавать число).

- [ ] **Step 4: Стили**

Добавить в соответствующий CSS (файл стилей трекера — найти `grep -rn "sidebar__period-select" internal/web/static`; добавить рядом):

```css
.period-select__trigger{display:flex;align-items:center;gap:8px;width:100%;padding:8px 10px;border-radius:8px;background:#111827;color:#e5e7eb;border:1px solid #374151;cursor:pointer;font-size:14px}
.period-select__dot{width:8px;height:8px;border-radius:999px;flex:0 0 auto}
.period-select__name{flex:1;text-align:left;font-weight:600}
.period-select__chev{color:#9ca3af}
.period-select__backdrop{position:fixed;inset:0;z-index:40}
.period-select__menu{position:absolute;left:0;right:0;top:calc(100% + 6px);z-index:41;background:#0b1220;border:1px solid #374151;border-radius:12px;padding:6px;max-height:60vh;overflow:auto;box-shadow:0 12px 40px rgba(0,0,0,.5)}
.period-select__group{font-size:11px;letter-spacing:.08em;text-transform:uppercase;color:#6b7280;padding:8px 10px 4px}
.period-select__item{display:flex;align-items:center;gap:10px;width:100%;padding:8px 10px;border:none;background:none;color:#e5e7eb;cursor:pointer;border-radius:8px;text-align:left}
.period-select__item:hover{background:#1f2937}
.period-select__item.is-selected{background:#312e81}
.period-select__item-name{font-weight:600}
.period-select__range{margin-left:auto;color:#9ca3af;font-size:12px;font-family:ui-monospace,Menlo,monospace}
.period-select__badge{color:#9ca3af;font-size:11px}
```

- [ ] **Step 5: Проверить в браузере**

Run: запустить приложение, открыть `/teamOkrs`.
Expected: селектор показывает дерево с отступами, статус-точки/бейджи, диапазоны дат; архивные периоды отсутствуют; выбор периода работает (URL `?period=` обновляется, дерево команд перезагружается).

- [ ] **Step 6: Показать дифф пользователю.**

---

## Task 9: Обновить спеки

**Files:**
- Modify: `specs/020-domain-model.md`, `specs/030-user-flows.md`, `specs/040-api-contract.md`, `specs/050-permissions-and-lifecycle.md`

- [ ] **Step 1: `020-domain-model.md` — сущность Period**

Заменить поля/инварианты Period: убрать `sort_order`; добавить `archived_at (nullable)`; описать вычисляемые `parent_id`, `depth`, `status` (`future|active|closed|archived`); правило родителя (строгое вхождение, самый узкий, тай-брейк; идентичные интервалы → сиблинги); правило статуса (границы включительны, archived перекрывает); правило порядка (DFS, зоны, направления сортировки детей). Добавить в раздел «Производные вычисления» пункты про `parent/depth/status`.

- [ ] **Step 2: `040-api-contract.md`**

- В объекте периода (строки ~564) заменить `sort_order` на `status`, `parent_id`, `depth`.
- В разделе read endpoints у `GET /api/v1/periods` описать поля `parent_id/depth/status` и что архивные не возвращаются.
- Добавить `GET /api/v1/admin/periods` (все периоды, включая архивные).
- Добавить `POST /api/v1/admin/periods/{id}/archive` (409, если не `closed`) и `/unarchive`.
- Убрать упоминания `move-up`/`move-down`.

- [ ] **Step 3: `030-user-flows.md` — раздел 2 «Управление периодами»**

Заменить «меняет порядок периодов через move up / move down» на: вложенная таблица (год→кварталы, порядок автоматический), архивирование/разархивирование закрытого периода, «+ вложенный». Описать grouped/nested селектор периодов на странице целей (статус-бейджи, скрытие архивных).

- [ ] **Step 4: `050-permissions-and-lifecycle.md`**

В права tenant-admin/periods добавить архивирование/разархивирование периода. Отметить, что статус периода `future/active/closed` вычисляется из дат, `archived` — ручной admin-переход только из `closed`. (Не путать с `TeamPeriodStatus` — отдельная per-team сущность, не меняется.)

- [ ] **Step 5: Проверка ссылочной целостности спеков**

Run: `rg -n "sort_order|move-up|move-down" specs/`
Expected: в контексте периодов совпадений нет (у goals/KR/shares `sort_order` остаётся — их не трогаем).

- [ ] **Step 6: Показать дифф пользователю.**

---

## Self-Review

**Spec coverage:**
- Вложенность (строгое вхождение, любая глубина) → Task 2 (`BuildPeriodViews`), тесты parent/depth/siblings.
- Статусы `future/active/closed` из дат, `archived` вручную из `closed` → Task 1 (`PeriodStatusFor`), Task 4 (валидация), Task 6 (409).
- Автопорядок, удаление `sort_order`/move → Task 2 (порядок), Task 3 (удаление механизма).
- Публичный список без архивных + admin GET со всеми → Task 5.
- Админка (вложенная таблица, статусы, архив, +вложенный) → Task 7.
- Селектор целей (grouped/nested, статусы) → Task 8.
- Миграция + seed → Task 3.
- Спеки → Task 9.

**Placeholder scan:** нет TBD/«handle errors»; весь код приведён. Единственные адаптации помечены явно: имена тест-хелперов пакетов (`newTestService`, admin-тест-хелперы) — переиспользовать существующие в этих пакетах; наличие `fmtDate`/`fmtDateRange` в tracker.js — проверить и добавить при отсутствии.

**Type consistency:** `PeriodView{Period; ParentID *int64; Depth int; Status PeriodStatus}` — единообразно в Task 2/3/5. `dto.PeriodInfo` (без `SortOrder`, с `ParentID/Depth/Status`) — Task 3, используется в Task 5. `ListPeriodViews(ctx, scope, includeArchived bool)` — Task 5, вызовы в публичном (false) и admin (true). Репо-методы `ArchivePeriod/UnarchivePeriod` — Task 3 (репо+интерфейс+fakes), сервис-обёртки — Task 4. Маршруты archive/unarchive — Task 6. `MovePeriod` удалён консистентно (репо, интерфейс, сервис, fakes, хендлеры, роуты) в Task 3.
