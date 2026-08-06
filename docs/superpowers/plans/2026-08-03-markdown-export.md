# Экспорт целей в Markdown — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Дать пользователю выгрузку OKR в Markdown (одна цель / цели команды / поддерево) в кратком или полном виде, с опциональными комментариями — через новый read-only endpoint, с превью/копированием/скачиванием в трекере.

**Architecture:** Генерация Markdown — на сервере. Чистый пакет `internal/export` форматирует доменные агрегаты в строку (без SQL/HTTP). Сервис оркеструет загрузку данных со scope-фильтрацией и batched-запросами и зовёт форматтер. Тонкий handler `GET /api/v1/teams/{teamID}/export` возвращает `{filename, markdown, lines}`. Клиент (`tracker.js`) показывает превью и по этому же тексту копирует/скачивает.

**Tech Stack:** Go 1.x, chi router, pgx/pgxpool, PostgreSQL; фронтенд — React 18 через `@babel/standalone` без сборщика (self-hosted vendor).

## Global Constraints

- Никакой бизнес-логики в handlers; расчёты прогресса — только через `internal/okr` / `internal/service` (единый источник).
- Слои не протекают: `internal/export` не импортирует `service`/`http`/`store`; handler не ходит в БД напрямую.
- Никаких запросов в цикле по командам/целям — только batched-загрузчики (без N+1).
- Изменения UI не требуют bundler/toolchain; новые библиотеки не добавляются.
- Доступ ограничен сервером: tenant (`TenantScopeFromContext`) + grant-scope (`auth.AllowedTeamIDsFromCtx` / `CanAccessTeamFromCtx`).
- Endpoint read-only (GET) — CSRF не требуется; side effects на агрегаты отсутствуют.
- Схема БД не меняется — миграция не нужна; seed не затрагивается.
- В коммитах/PR/доках не упоминать AI/ассистентов/генерацию. **Git-коммиты делает пользователь сам — шаги «Commit» в этом плане НЕ выполнять автоматически; вместо коммита останавливаться на ревью-чекпоинт.**
- Спеки-источники правды: `specs/010-architecture-constraints.md`, `020-domain-model.md`, `030-user-flows.md`, `040-api-contract.md`, `050-permissions-and-lifecycle.md`. Дизайн: `docs/superpowers/specs/2026-08-03-markdown-export-design.md`.

> **Замечание по коммитам:** проект запрещает автокоммиты (CLAUDE.md). Каждый шаг «Commit» ниже приведён для полноты TDD-цикла, но фактический `git commit` оставляется пользователю. Останавливайтесь после прохождения тестов задачи на ревью.

---

## Справочник по существующему коду (для реализатора)

- Доменные типы — `internal/domain/models.go`: `Goal{ID,TeamID,PeriodID,Title,Description,Priority,Weight,WorkType,FocusType,OwnerText,OwnerUDIDs,Progress,KeyResults,Comments}`, `KeyResult{ID,Title,Description,ZeroingCriteria,Weight,Kind,Progress,Project,Numerical,Boolean,Note}`, `KRNumerical{StartValue,TargetValue,CurrentValue,Unit,Checkpoints}`, `KRBoolean{IsDone}`, `KRProject{Stages}`, `KRProjectStage{Title,Weight,IsDone}`, `KeyResultNote{Text,AuthorName,UpdatedAt}`, `GoalComment{ID,ParentID,Text,AuthorName,CreatedAt,ResolvedAt,Replies}`, `Team{ID,Name,Type,ParentID}`, `Period{ID,Name,StartDate,EndDate}`.
- `domain.KRKind` значения: `domain.KRKindBoolean`, `domain.KRKindProject`, `domain.KRKindNumerical` (см. `internal/domain/models.go`).
- Прогресс: `service.CalculateGoalProgress(*domain.Goal) int` (ставит `goal.Progress`), `service.CalculateKRProgress(domain.KeyResult) int` (`internal/service/progress.go`).
- Загрузчики (batched):
  - `service.GetTeamOKR(ctx, scope, teamID, periodID, period) (service.TeamOKR, error)` — команда со всеми целями (owner + shared-in), у целей заполнены `KeyResults` (+ `Note`) и `Comments`; `goal.Progress` посчитан. `TeamOKR.Goals` — `[]GoalDetails{Goal domain.Goal, ShareTeams []TeamShareInfo}`.
  - `goals.GoalRepository.ListGoalsByTeamsPeriod(ctx, scope, periodID, teamIDs) (map[int64][]domain.Goal, error)` — batched по командам; заполняет `KeyResults` (numerical/project/boolean), **без** `Note` и `Comments`; `Progress` НЕ посчитан.
  - `krs.KRRepository.BatchLoadNotes(ctx, scope, krIDs) (map[int64]*domain.KeyResultNote, error)` — заметки batched.
  - `goals.GoalRepository.ListGoalCommentsByGoals(ctx, scope, goalIDs) (map[int64][]domain.GoalComment, error)` — комментарии (таски с вложенными `Replies`) batched.
  - `service.collectDescendantIDs(targetID, nodes []TeamNode) []int64` — потомки в дереве (не включает сам `targetID`).
  - `service.GetHierarchy(ctx, scope, *periodID) ([]TeamNode, error)` — видимое в периоде дерево (`TeamNode{Team, Children}`).
- Scope: `auth.AllowedTeamIDsFromCtx(ctx) ([]int64, bool)` — `nil` = admin/unrestricted; `auth.CanAccessTeamFromCtx(ctx, teamID) bool`. Инвариант: grants раскрываются на всех потомков, поэтому поддерево доступной команды целиком в allowed-наборе.
- HTTP-хелперы: `v1.WriteJSON(w, status, any)`, `v1.WriteError(w, status, code, msg, details)`; `common.ParseID`, `common.ParsePeriodID(r)`.
- Тест-харнесс: `internal/http/handlers/api/v1/testutil` (`NewAPIV1RouterWithScope(svc, allowedTeamIDs)`, `RunMigrations`, `RequireDockerOrSkip`) + testcontainers Postgres (см. `internal/http/handlers/api/v1/teams/integration_test.go`). `store.New(pool)`, `service.NewFromStore(st, grantsProvider, hcCache, logger)`.
- Фронтенд: карточка цели — `GoalCard` в `internal/web/static/tracker.js` (мета-строка ~строки 1266–1281, где живёт `CopyLinkButton`). API-хелперы — `apiGet/apiPost` и `csrfHeaders` из `api.js`. Стили — `internal/web/static/tracker.css`.

---

## File Structure

- **Create** `internal/export/export.go` — форматтер: типы `Options`, `Scope`, `TeamBlock`, функции `Markdown(...)`, `Filename(...)`, внутренние хелперы форматирования (лейблы, чеклист, KR-детали, комментарии). Единственная ответственность — доменные данные → Markdown-строка. Без внешних импортов кроме `internal/domain`, `fmt`, `strings`, `time`.
- **Create** `internal/export/export_test.go` — табличные unit-тесты форматтера (без БД).
- **Modify** `internal/service/service.go` — расширить интерфейсы `GoalRepo` (+`ListGoalCommentsByGoals`) и `KRRepo` (+`BatchLoadNotes`).
- **Create** `internal/service/export.go` — `Service.ExportOKR(...)` (оркестрация + сборка блоков + вызов форматтера).
- **Create** `internal/service/export_test.go` — DB-backed тест сборки (tree/dedup/scope).
- **Modify** `internal/http/handlers/api/v1/teams/handler.go` — `HandleTeamExport`.
- **Modify** `internal/http/handlers/api/v1/teams/routes.go` — регистрация маршрута.
- **Create** `internal/http/handlers/api/v1/teams/export_integration_test.go` — handler-тест (валидация + scope + happy path).
- **Modify** `internal/web/static/tracker.js` — `ExportModal` + пункт меню «···» на карточке цели.
- **Modify** `internal/web/static/tracker.css` — стили модалки/меню.
- **Modify** `specs/040-api-contract.md`, `specs/030-user-flows.md`, `specs/050-permissions-and-lifecycle.md` — обновление контракта/флоу/прав.

---

## Точный формат Markdown (спецификация для тестов)

Все блоки разделяются одной пустой строкой. Документ не заканчивается лишними пустыми строками (ровно один `\n` в конце).

**Заголовок документа (всегда, первая строка):**
```
<!-- OKR export · <Period.Name> -->
```

**Блок команды:** заголовок `# <TeamBlock.Heading>` (Heading задаёт сервис: имя команды для scope goal/team, путь `A / B / C` для tree).

**Цель (краткий, `format=short`):**
```
## <Goal.Title>

<Goal.Description>            ← строка(и) только если Description != ""

- [ ] <KR.Title>             ← [x] если KR.Progress == 100
- [ ] <KR.Title>
```
Если у цели нет KR — блок списка отсутствует.

**Цель (полный, `format=full`)** — как краткий, плюс строка метаданных сразу под `## <Title>` (перед описанием):
```
## <Goal.Title>

<Priority> · вес <Weight>% · <WorkTypeLabel> · <FocusLabel> · прогресс <Goal.Progress>%<DriversSuffix>

<Goal.Description>
```
- `<WorkTypeLabel>` = строковое значение `domain.WorkType` как есть (`Delivery` / `Discovery`).
- `<FocusLabel>` = Title Case из `FocusType`: `UPPER_SNAKE` → слова через пробел, каждое слово с заглавной (`TECH_INDEPENDENCE` → `Tech Independence`). Пустой FocusType → сегмент пропускается.
- `<DriversSuffix>` = ` · драйверы: <Goal.OwnerText>` если `OwnerText != ""`, иначе пусто.
- Пустые сегменты (например пустой Focus) не выводят лишних ` · `.

**KR (полный)** — под строкой `- [ ] <title>` идут вложенные строки с отступом в два пробела, только непустые:
```
- [ ] <KR.Title>
  - тип: <KindLabel><NumericalSuffix> · вес <KR.Weight>% · прогресс <KR.Progress>%
  - критерий обнуления: <KR.ZeroingCriteria>        ← если != ""
  - описание: <KR.Description>                       ← если != ""
  - заметка (<Note.AuthorName>): <Note.Text>         ← если Note != nil
```
- `<KindLabel>` = `BOOLEAN` | `PROJECT` | `NUMERICAL` (строка `KR.Kind`).
- `<NumericalSuffix>` для `NUMERICAL`: ` · <start> → <current> / <target> <unit>`, числа — `formatNumber` (целые без дробей, иначе минимальная запись; **без** группировки разрядов), `unit` пропускается если пуст. Для не-NUMERICAL — пусто.

**Комментарии (`comments=1`, независимо от format)** — после KR-списка цели, если у цели есть таски:
```
**Комментарии**
- <Task.AuthorName> (<dd.mm.yyyy>): <Task.Text><ResolvedSuffix>
  - <Reply.AuthorName> (<dd.mm.yyyy>): <Reply.Text>
```
- Таски — в порядке `Comments` (уже created_at ASC), только `ParentID == nil`.
- `<ResolvedSuffix>` = ` (решено)` если `Task.ResolvedAt != nil`, иначе пусто.
- Ответы (`Task.Replies`) — с отступом два пробела, в порядке слайса.
- Дата: формат `02.01.2006` от `CreatedAt` (UTC).

**Дедуп общих целей (tree):** в блоке команды `T` цель `g`:
- если `g.TeamID == T.ID` (владелец) — рендерится полностью по правилам выше;
- иначе (расшарена в `T`) — одной строкой-ссылкой: `## <g.Title> _(общая, владелец: <ownerName>)_` без описания/KR/комментариев. `ownerName` сервис кладёт заранее — см. Task 4/6 (передаётся в `TeamBlock`).

**Имя файла (`Filename`):**
- база периода: `y<YY>q<Q>`, где `YY = StartDate.Year() % 100` (два разряда, `%02d`), `Q = (int(StartDate.Month())-1)/3 + 1`.
- код скоупа: `goal` → `g<goalID>`; `team` → `<typeLetter><teamID>`; `tree` → `<typeLetter><teamID>-tree`.
- `<typeLetter>` = первая руна строки `Team.Type` (`unit`→`u`, `cluster`→`c`, …).
- Итог: `okr-<база>-<код>.md`. Пример: период 2026-01-01, unit-команда id=1, scope=team → `okr-y26q1-u1.md`.

---

## Task 1: Пакет `internal/export` — краткий формат (goal/team)

**Files:**
- Create: `internal/export/export.go`
- Test: `internal/export/export_test.go`

**Interfaces:**
- Consumes: `internal/domain` типы (`Goal`, `KeyResult`, `Team`, `Period`).
- Produces:
  - `type Format string`; `const FormatShort Format = "short"`; `const FormatFull Format = "full"`
  - `type Options struct { Format Format; Comments bool }`
  - `type TeamBlock struct { Heading string; TeamID int64; Goals []domain.Goal; OwnerNames map[int64]string }` (`OwnerNames` используется только в tree для строки общей цели; в Task 1 может быть nil)
  - `func Markdown(period domain.Period, blocks []TeamBlock, opts Options) string`

- [ ] **Step 1: Написать падающий тест краткого формата (одна команда, один/несколько KR, done/undone, без описания)**

```go
package export

import (
	"strings"
	"testing"

	"okrs/internal/domain"
)

func TestMarkdownShortTeam(t *testing.T) {
	period := domain.Period{Name: "Q1 2026"}
	blocks := []TeamBlock{{
		Heading: "Платформа",
		TeamID:  1,
		Goals: []domain.Goal{{
			ID: 10, TeamID: 1, Title: "Снизить P95 latency до 200ms",
			Description: "Оптимизировать критические пути запросов",
			KeyResults: []domain.KeyResult{
				{Title: "P95 latency API gateway", Progress: 0},
				{Title: "Миграция на HTTP/2", Progress: 100},
			},
		}},
	}}
	got := Markdown(period, blocks, Options{Format: FormatShort})
	want := strings.Join([]string{
		"<!-- OKR export · Q1 2026 -->",
		"",
		"# Платформа",
		"",
		"## Снизить P95 latency до 200ms",
		"",
		"Оптимизировать критические пути запросов",
		"",
		"- [ ] P95 latency API gateway",
		"- [x] Миграция на HTTP/2",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("markdown mismatch:\n--- got ---\n%q\n--- want ---\n%q", got, want)
	}
}

func TestMarkdownShortGoalWithoutDescriptionOrKRs(t *testing.T) {
	got := Markdown(domain.Period{Name: "Q1 2026"}, []TeamBlock{{
		Heading: "Платформа", TeamID: 1,
		Goals: []domain.Goal{{ID: 10, TeamID: 1, Title: "Цель без деталей"}},
	}}, Options{Format: FormatShort})
	want := "<!-- OKR export · Q1 2026 -->\n\n# Платформа\n\n## Цель без деталей\n"
	if got != want {
		t.Fatalf("mismatch:\n got: %q\nwant: %q", got, want)
	}
}
```

- [ ] **Step 2: Запустить тест — убедиться, что не компилируется/падает**

Run: `go test ./internal/export/ -run TestMarkdownShort -v`
Expected: FAIL — `undefined: Markdown` (пакет ещё пуст).

- [ ] **Step 3: Реализовать минимальный `export.go` (типы + краткий формат)**

```go
package export

import (
	"strings"

	"okrs/internal/domain"
)

type Format string

const (
	FormatShort Format = "short"
	FormatFull  Format = "full"
)

type Options struct {
	Format   Format
	Comments bool
}

type TeamBlock struct {
	Heading    string
	TeamID     int64
	Goals      []domain.Goal
	OwnerNames map[int64]string
}

// Markdown renders the export document. Blocks are rendered in the given order.
func Markdown(period domain.Period, blocks []TeamBlock, opts Options) string {
	var b strings.Builder
	b.WriteString("<!-- OKR export · " + period.Name + " -->\n")
	for _, block := range blocks {
		b.WriteString("\n# " + block.Heading + "\n")
		for _, g := range block.Goals {
			writeGoal(&b, block, g, opts)
		}
	}
	return b.String()
}

func writeGoal(b *strings.Builder, block TeamBlock, g domain.Goal, opts Options) {
	b.WriteString("\n## " + g.Title + "\n")
	if g.Description != "" {
		b.WriteString("\n" + g.Description + "\n")
	}
	if len(g.KeyResults) > 0 {
		b.WriteString("\n")
		for _, kr := range g.KeyResults {
			box := " "
			if kr.Progress == 100 {
				box = "x"
			}
			b.WriteString("- [" + box + "] " + kr.Title + "\n")
		}
	}
}
```

- [ ] **Step 4: Запустить тесты — убедиться, что проходят**

Run: `go test ./internal/export/ -run TestMarkdownShort -v`
Expected: PASS

- [ ] **Step 5: Commit** (не выполнять `git commit` — остановиться на ревью)

```bash
git add internal/export/export.go internal/export/export_test.go
git commit -m "add markdown export formatter (short format)"
```

---

## Task 2: Полный формат — метаданные цели и детали KR

**Files:**
- Modify: `internal/export/export.go`
- Test: `internal/export/export_test.go`

**Interfaces:**
- Consumes: типы из Task 1.
- Produces: расширенное поведение `Markdown` при `opts.Format == FormatFull` (сигнатуры не меняются). Внутренние хелперы `focusLabel(domain.FocusType) string`, `formatNumber(float64) string`.

- [ ] **Step 1: Написать падающий тест полного формата (метаданные цели + NUMERICAL KR с деталями, критерием, описанием, заметкой)**

```go
func TestMarkdownFullGoalAndKR(t *testing.T) {
	blocks := []TeamBlock{{
		Heading: "Платформа", TeamID: 1,
		Goals: []domain.Goal{{
			ID: 10, TeamID: 1, Title: "Снизить P95 latency",
			Description: "Оптимизировать пути",
			Priority:    domain.Priority("P1"), Weight: 20,
			WorkType: domain.WorkType("Delivery"), FocusType: domain.FocusType("TECH_INDEPENDENCE"),
			OwnerText: "Иван, Мария", Progress: 45,
			KeyResults: []domain.KeyResult{{
				Title: "P95 latency", Kind: domain.KRKindNumerical, Weight: 30, Progress: 45,
				ZeroingCriteria: "деградация SLA",
				Description:     "по данным APM",
				Numerical:       &domain.KRNumerical{StartValue: 300, CurrentValue: 250, TargetValue: 200, Unit: "мс"},
				Note:            &domain.KeyResultNote{AuthorName: "Пётр", Text: "работаем"},
			}},
		}},
	}}
	got := Markdown(domain.Period{Name: "Q1 2026"}, blocks, Options{Format: FormatFull})
	for _, want := range []string{
		"## Снизить P95 latency\n",
		"\nP1 · вес 20% · Delivery · Tech Independence · прогресс 45% · драйверы: Иван, Мария\n",
		"\nОптимизировать пути\n",
		"- [ ] P95 latency\n",
		"  - тип: NUMERICAL · 300 → 250 / 200 мс · вес 30% · прогресс 45%\n",
		"  - критерий обнуления: деградация SLA\n",
		"  - описание: по данным APM\n",
		"  - заметка (Пётр): работаем\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestMarkdownFullOmitsEmptyFocusAndDrivers(t *testing.T) {
	got := Markdown(domain.Period{Name: "Q1 2026"}, []TeamBlock{{
		Heading: "Платформа", TeamID: 1,
		Goals: []domain.Goal{{
			ID: 1, TeamID: 1, Title: "T", Priority: domain.Priority("P2"), Weight: 10,
			WorkType: domain.WorkType("Discovery"), Progress: 0,
		}},
	}}, Options{Format: FormatFull})
	if !strings.Contains(got, "\nP2 · вес 10% · Discovery · прогресс 0%\n") {
		t.Fatalf("meta line wrong:\n%s", got)
	}
}
```

- [ ] **Step 2: Запустить тест — убедиться, что падает**

Run: `go test ./internal/export/ -run TestMarkdownFull -v`
Expected: FAIL (метаданных и деталей KR ещё нет)

- [ ] **Step 3: Реализовать полный формат в `export.go`**

Заменить `writeGoal` и добавить хелперы:

```go
import (
	"strconv"
	"strings"
	// domain уже импортирован
)

func writeGoal(b *strings.Builder, block TeamBlock, g domain.Goal, opts Options) {
	b.WriteString("\n## " + g.Title + "\n")
	if opts.Format == FormatFull {
		b.WriteString("\n" + goalMetaLine(g) + "\n")
	}
	if g.Description != "" {
		b.WriteString("\n" + g.Description + "\n")
	}
	if len(g.KeyResults) > 0 {
		b.WriteString("\n")
		for _, kr := range g.KeyResults {
			writeKR(b, kr, opts)
		}
	}
}

func goalMetaLine(g domain.Goal) string {
	segs := []string{
		string(g.Priority),
		"вес " + strconv.Itoa(g.Weight) + "%",
		string(g.WorkType),
	}
	if f := focusLabel(g.FocusType); f != "" {
		segs = append(segs, f)
	}
	segs = append(segs, "прогресс "+strconv.Itoa(g.Progress)+"%")
	line := strings.Join(segs, " · ")
	if g.OwnerText != "" {
		line += " · драйверы: " + g.OwnerText
	}
	return line
}

func writeKR(b *strings.Builder, kr domain.KeyResult, opts Options) {
	box := " "
	if kr.Progress == 100 {
		box = "x"
	}
	b.WriteString("- [" + box + "] " + kr.Title + "\n")
	if opts.Format != FormatFull {
		return
	}
	detail := "тип: " + string(kr.Kind) + numericalSuffix(kr) +
		" · вес " + strconv.Itoa(kr.Weight) + "% · прогресс " + strconv.Itoa(kr.Progress) + "%"
	b.WriteString("  - " + detail + "\n")
	if kr.ZeroingCriteria != "" {
		b.WriteString("  - критерий обнуления: " + kr.ZeroingCriteria + "\n")
	}
	if kr.Description != "" {
		b.WriteString("  - описание: " + kr.Description + "\n")
	}
	if kr.Note != nil {
		b.WriteString("  - заметка (" + kr.Note.AuthorName + "): " + kr.Note.Text + "\n")
	}
}

func numericalSuffix(kr domain.KeyResult) string {
	if kr.Kind != domain.KRKindNumerical || kr.Numerical == nil {
		return ""
	}
	n := kr.Numerical
	s := " · " + formatNumber(n.StartValue) + " → " + formatNumber(n.CurrentValue) + " / " + formatNumber(n.TargetValue)
	if n.Unit != "" {
		s += " " + n.Unit
	}
	return s
}

func focusLabel(f domain.FocusType) string {
	if f == "" {
		return ""
	}
	words := strings.Split(strings.ToLower(string(f)), "_")
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

func formatNumber(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
```

- [ ] **Step 4: Запустить тесты — убедиться, что все проходят (в т.ч. Task 1)**

Run: `go test ./internal/export/ -v`
Expected: PASS

- [ ] **Step 5: Commit** (фактический `git commit` — за пользователем)

```bash
git add internal/export/export.go internal/export/export_test.go
git commit -m "add full-format goal metadata and KR details to export"
```

---

## Task 3: Блок комментариев

**Files:**
- Modify: `internal/export/export.go`
- Test: `internal/export/export_test.go`

**Interfaces:**
- Consumes: типы из Task 1–2.
- Produces: поведение при `opts.Comments == true` (сигнатуры не меняются).

- [ ] **Step 1: Написать падающий тест комментариев (таска решённая + ответ; ответы с отступом; при Comments=false — нет блока)**

```go
func TestMarkdownComments(t *testing.T) {
	resolved := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	created := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	g := domain.Goal{
		ID: 10, TeamID: 1, Title: "Цель",
		Comments: []domain.GoalComment{{
			ID: 1, Text: "Начали профилирование", AuthorName: "Алексей", CreatedAt: created, ResolvedAt: &resolved,
			Replies: []domain.GoalComment{{ID: 2, Text: "ок", AuthorName: "Дмитрий", CreatedAt: created}},
		}},
	}
	blocks := []TeamBlock{{Heading: "Платформа", TeamID: 1, Goals: []domain.Goal{g}}}

	withC := Markdown(domain.Period{Name: "Q1 2026"}, blocks, Options{Format: FormatShort, Comments: true})
	for _, want := range []string{
		"**Комментарии**\n",
		"- Алексей (20.04.2026): Начали профилирование (решено)\n",
		"  - Дмитрий (20.04.2026): ок\n",
	} {
		if !strings.Contains(withC, want) {
			t.Fatalf("missing %q in:\n%s", want, withC)
		}
	}
	noC := Markdown(domain.Period{Name: "Q1 2026"}, blocks, Options{Format: FormatShort, Comments: false})
	if strings.Contains(noC, "Комментарии") {
		t.Fatalf("comments block leaked when Comments=false:\n%s", noC)
	}
}
```

Добавить импорт `"time"` в тест-файл, если ещё не добавлен.

- [ ] **Step 2: Запустить тест — убедиться, что падает**

Run: `go test ./internal/export/ -run TestMarkdownComments -v`
Expected: FAIL

- [ ] **Step 3: Реализовать блок комментариев**

В `writeGoal`, после блока KR, добавить:

```go
	if opts.Comments {
		writeComments(b, g)
	}
```

И новый хелпер + импорт `"time"`:

```go
func writeComments(b *strings.Builder, g domain.Goal) {
	tasks := make([]domain.GoalComment, 0, len(g.Comments))
	for _, c := range g.Comments {
		if c.ParentID == nil {
			tasks = append(tasks, c)
		}
	}
	if len(tasks) == 0 {
		return
	}
	b.WriteString("\n**Комментарии**\n")
	for _, task := range tasks {
		suffix := ""
		if task.ResolvedAt != nil {
			suffix = " (решено)"
		}
		b.WriteString("- " + task.AuthorName + " (" + task.CreatedAt.UTC().Format("02.01.2006") + "): " + task.Text + suffix + "\n")
		for _, r := range task.Replies {
			b.WriteString("  - " + r.AuthorName + " (" + r.CreatedAt.UTC().Format("02.01.2006") + "): " + r.Text + "\n")
		}
	}
}
```

- [ ] **Step 4: Запустить тесты — убедиться, что все проходят**

Run: `go test ./internal/export/ -v`
Expected: PASS

- [ ] **Step 5: Commit** (за пользователем)

```bash
git add internal/export/export.go internal/export/export_test.go
git commit -m "add comments block to markdown export"
```

---

## Task 4: Tree — путь команды, дедуп общих целей, `Filename`

**Files:**
- Modify: `internal/export/export.go`
- Test: `internal/export/export_test.go`

**Interfaces:**
- Consumes: типы из Task 1–3.
- Produces:
  - `type Scope string`; `const ScopeGoal Scope = "goal"`; `const ScopeTeam Scope = "team"`; `const ScopeTree Scope = "tree"`
  - `func Filename(period domain.Period, scope Scope, team domain.Team, goalID int64) string`
  - поведение `Markdown` для нескольких блоков и строки общей цели (по `g.TeamID != block.TeamID`).

- [ ] **Step 1: Написать падающие тесты (несколько команд подряд; строка общей цели; Filename для трёх скоупов)**

```go
func TestMarkdownTreeSharedGoalReference(t *testing.T) {
	blocks := []TeamBlock{
		{Heading: "Реклама / Платформа", TeamID: 1, Goals: []domain.Goal{
			{ID: 10, TeamID: 1, Title: "Своя цель"},
		}},
		{Heading: "Реклама / Платформа / Web", TeamID: 2,
			OwnerNames: map[int64]string{1: "Платформа"},
			Goals: []domain.Goal{
				{ID: 10, TeamID: 1, Title: "Своя цель"}, // shared into team 2 (owner is team 1)
			}},
	}
	got := Markdown(domain.Period{Name: "Q1 2026"}, blocks, Options{Format: FormatShort})
	if !strings.Contains(got, "# Реклама / Платформа\n") || !strings.Contains(got, "# Реклама / Платформа / Web\n") {
		t.Fatalf("team headings missing:\n%s", got)
	}
	if !strings.Contains(got, "## Своя цель _(общая, владелец: Платформа)_\n") {
		t.Fatalf("shared reference missing:\n%s", got)
	}
	// owner block renders the goal fully (no shared suffix)
	if strings.Count(got, "## Своя цель\n") != 1 {
		t.Fatalf("owner goal should render once as full heading:\n%s", got)
	}
}

func TestFilename(t *testing.T) {
	period := domain.Period{StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	team := domain.Team{ID: 1, Type: domain.TeamType("unit")}
	cases := map[string]string{
		Filename(period, ScopeGoal, team, 55): "okr-y26q1-g55.md",
		Filename(period, ScopeTeam, team, 0):  "okr-y26q1-u1.md",
		Filename(period, ScopeTree, team, 0):  "okr-y26q1-u1-tree.md",
	}
	for got, want := range cases {
		if got != want {
			t.Fatalf("filename got %q want %q", got, want)
		}
	}
}
```

- [ ] **Step 2: Запустить тест — убедиться, что падает**

Run: `go test ./internal/export/ -run 'TestMarkdownTree|TestFilename' -v`
Expected: FAIL (`Filename`/`Scope` не определены; строка общей цели не рендерится)

- [ ] **Step 3: Реализовать дедуп в `writeGoal` и `Filename`**

В начало `writeGoal` добавить обработку общей цели:

```go
func writeGoal(b *strings.Builder, block TeamBlock, g domain.Goal, opts Options) {
	if g.TeamID != block.TeamID {
		owner := ""
		if block.OwnerNames != nil {
			owner = block.OwnerNames[g.TeamID]
		}
		b.WriteString("\n## " + g.Title + " _(общая, владелец: " + owner + ")_\n")
		return
	}
	b.WriteString("\n## " + g.Title + "\n")
	// ... остальное тело без изменений (метаданные/описание/KR/комментарии)
}
```

Добавить типы и `Filename`:

```go
import (
	"strconv"
	// ...
)

type Scope string

const (
	ScopeGoal Scope = "goal"
	ScopeTeam Scope = "team"
	ScopeTree Scope = "tree"
)

func Filename(period domain.Period, scope Scope, team domain.Team, goalID int64) string {
	start := period.StartDate.UTC()
	quarter := (int(start.Month())-1)/3 + 1
	base := "y" + pad2(start.Year()%100) + "q" + strconv.Itoa(quarter)
	var code string
	switch scope {
	case ScopeGoal:
		code = "g" + strconv.FormatInt(goalID, 10)
	case ScopeTree:
		code = typeLetter(team) + strconv.FormatInt(team.ID, 10) + "-tree"
	default: // ScopeTeam
		code = typeLetter(team) + strconv.FormatInt(team.ID, 10)
	}
	return "okr-" + base + "-" + code + ".md"
}

func pad2(n int) string {
	s := strconv.Itoa(n)
	if len(s) < 2 {
		return "0" + s
	}
	return s
}

func typeLetter(team domain.Team) string {
	t := string(team.Type)
	if t == "" {
		return "t"
	}
	return t[:1]
}
```

- [ ] **Step 4: Запустить весь пакет — убедиться, что проходят**

Run: `go test ./internal/export/ -v`
Expected: PASS

- [ ] **Step 5: Commit** (за пользователем)

```bash
git add internal/export/export.go internal/export/export_test.go
git commit -m "add tree blocks, shared-goal dedup and filename to export"
```

---

## Task 5: Сервис `ExportOKR` (оркестрация, scope, batched-загрузка)

**Files:**
- Modify: `internal/service/service.go` (интерфейсы `GoalRepo`, `KRRepo`)
- Create: `internal/service/export.go`
- Test: `internal/service/export_test.go`

**Interfaces:**
- Consumes: `export.{Options,Scope,TeamBlock,Markdown,Filename}`; `s.goals`, `s.krs`, `s.shares`, `s.teams`, `s.periods`; `GetTeamOKR`, `GetHierarchy`, `collectDescendantIDs`, `CalculateGoalProgress`, `CalculateKRProgress`.
- Produces:
  - `type ExportParams struct { TeamID, PeriodID, GoalID int64; Scope export.Scope; Options export.Options; AllowedTeamIDs []int64 }`
  - `type ExportResult struct { Filename, Markdown string; Lines int }`
  - `func (s *Service) ExportOKR(ctx context.Context, scope domain.TenantScope, p ExportParams) (ExportResult, error)`
  - интерфейс `GoalRepo` получает `ListGoalCommentsByGoals(ctx, scope, goalIDs []int64) (map[int64][]domain.GoalComment, error)`
  - интерфейс `KRRepo` получает `BatchLoadNotes(ctx, scope, krIDs []int64) (map[int64]*domain.KeyResultNote, error)`

- [ ] **Step 1: Расширить интерфейсы `GoalRepo` и `KRRepo`**

В `internal/service/service.go` добавить в `GoalRepo`:
```go
	ListGoalCommentsByGoals(ctx context.Context, scope domain.TenantScope, goalIDs []int64) (map[int64][]domain.GoalComment, error)
```
и в `KRRepo`:
```go
	BatchLoadNotes(ctx context.Context, scope domain.TenantScope, krIDs []int64) (map[int64]*domain.KeyResultNote, error)
```
(Конкретные методы уже есть в `goals.GoalRepository` и `krs.KRRepository`.)

- [ ] **Step 2: Написать падающий DB-backed тест сборки (tree: владелец рендерится полностью, у ребёнка расшаренная цель — ссылкой; scope-фильтр отсекает недоступную ветку)**

Создать `internal/service/export_test.go`. Использовать testcontainers-паттерн из `internal/http/handlers/api/v1/teams/integration_test.go` (скопировать блок поднятия контейнера и `store.New(pool)` + `service.NewFromStore(st, nil, nil, nil)`), затем:

```go
	// teams: root(1, unit) -> child(2, team); goal G owned by team 1, shared to team 2
	// period P covering now.
	// Insert via raw SQL like integration_test.go (teams, periods, goals, goal_shares, key_results).
	res, err := svc.ExportOKR(ctx, scope, service.ExportParams{
		TeamID: rootID, PeriodID: periodID, Scope: export.ScopeTree,
		Options: export.Options{Format: export.FormatShort}, AllowedTeamIDs: []int64{rootID, childID},
	})
	if err != nil {
		t.Fatalf("ExportOKR: %v", err)
	}
	if !strings.Contains(res.Markdown, "## Общая цель\n") {
		t.Fatalf("owner block should render goal fully:\n%s", res.Markdown)
	}
	if !strings.Contains(res.Markdown, "_(общая, владелец:") {
		t.Fatalf("child block should show shared reference:\n%s", res.Markdown)
	}
	if res.Filename == "" || res.Lines == 0 {
		t.Fatalf("expected filename and line count, got %+v", res)
	}
```

Добавить второй тест: `AllowedTeamIDs: []int64{rootID}` (без childID) → в выводе нет блока `childID` (недоступная ветка исключена).

- [ ] **Step 3: Запустить тест — убедиться, что падает**

Run: `go test ./internal/service/ -run TestExportOKR -v`
Expected: FAIL — `svc.ExportOKR undefined`

- [ ] **Step 4: Реализовать `internal/service/export.go`**

```go
package service

import (
	"context"
	"strings"

	"okrs/internal/domain"
	"okrs/internal/export"
)

type ExportParams struct {
	TeamID         int64
	PeriodID       int64
	GoalID         int64
	Scope          export.Scope
	Options        export.Options
	AllowedTeamIDs []int64 // nil = unrestricted (admin)
}

type ExportResult struct {
	Filename string
	Markdown string
	Lines    int
}

func (s *Service) ExportOKR(ctx context.Context, scope domain.TenantScope, p ExportParams) (ExportResult, error) {
	period, err := s.periods.GetPeriod(ctx, scope, p.PeriodID)
	if err != nil {
		return ExportResult{}, err
	}
	team, err := s.teams.GetTeam(ctx, scope, p.TeamID)
	if err != nil {
		return ExportResult{}, err
	}

	var blocks []export.TeamBlock
	switch p.Scope {
	case export.ScopeGoal:
		blocks, err = s.exportGoalBlocks(ctx, scope, team, period, p.GoalID)
	case export.ScopeTree:
		blocks, err = s.exportTreeBlocks(ctx, scope, team, period, p)
	default: // ScopeTeam
		blocks, err = s.exportTeamBlocks(ctx, scope, team, period)
	}
	if err != nil {
		return ExportResult{}, err
	}

	md := export.Markdown(period, blocks, p.Options)
	return ExportResult{
		Filename: export.Filename(period, p.Scope, team, p.GoalID),
		Markdown: md,
		Lines:    strings.Count(md, "\n"),
	}, nil
}

// exportTeamBlocks reuses GetTeamOKR (goals with KRs, notes, comments, progress).
func (s *Service) exportTeamBlocks(ctx context.Context, scope domain.TenantScope, team domain.Team, period domain.Period) ([]export.TeamBlock, error) {
	okrData, err := s.GetTeamOKR(ctx, scope, team.ID, period.ID, period)
	if err != nil {
		return nil, err
	}
	goals := make([]domain.Goal, 0, len(okrData.Goals))
	for _, gd := range okrData.Goals {
		goals = append(goals, gd.Goal)
	}
	return []export.TeamBlock{{Heading: team.Name, TeamID: team.ID, Goals: goals}}, nil
}

// exportGoalBlocks filters a single goal out of the team's board (guarantees board membership + access).
func (s *Service) exportGoalBlocks(ctx context.Context, scope domain.TenantScope, team domain.Team, period domain.Period, goalID int64) ([]export.TeamBlock, error) {
	okrData, err := s.GetTeamOKR(ctx, scope, team.ID, period.ID, period)
	if err != nil {
		return nil, err
	}
	for _, gd := range okrData.Goals {
		if gd.Goal.ID == goalID {
			return []export.TeamBlock{{Heading: team.Name, TeamID: team.ID, Goals: []domain.Goal{gd.Goal}}}, nil
		}
	}
	return nil, ErrGoalNotOnTeamBoard
}

func (s *Service) exportTreeBlocks(ctx context.Context, scope domain.TenantScope, team domain.Team, period domain.Period, p ExportParams) ([]export.TeamBlock, error) {
	hierarchy, err := s.GetHierarchy(ctx, scope, &period.ID)
	if err != nil {
		return nil, err
	}
	// team + descendants, in DFS order, intersected with allowed scope.
	ordered := orderedSubtreeIDs(team.ID, hierarchy)
	teamsByID, pathByID := indexTeamsWithPaths(hierarchy)
	teamIDs := make([]int64, 0, len(ordered))
	for _, id := range ordered {
		if allowedContains(p.AllowedTeamIDs, id) {
			teamIDs = append(teamIDs, id)
		}
	}
	goalsByTeam, err := s.goals.ListGoalsByTeamsPeriod(ctx, scope, period.ID, teamIDs)
	if err != nil {
		return nil, err
	}

	// Collect KR / goal IDs for batched notes/comments.
	var krIDs, goalIDs []int64
	for _, gs := range goalsByTeam {
		for gi := range gs {
			g := &gs[gi]
			goalIDs = append(goalIDs, g.ID)
			for ki := range g.KeyResults {
				krIDs = append(krIDs, g.KeyResults[ki].ID)
			}
		}
	}
	var notes map[int64]*domain.KeyResultNote
	if p.Options.Format == export.FormatFull && len(krIDs) > 0 {
		if notes, err = s.krs.BatchLoadNotes(ctx, scope, krIDs); err != nil {
			return nil, err
		}
	}
	var comments map[int64][]domain.GoalComment
	if p.Options.Comments && len(goalIDs) > 0 {
		if comments, err = s.goals.ListGoalCommentsByGoals(ctx, scope, goalIDs); err != nil {
			return nil, err
		}
	}

	// owner names for shared-goal reference lines.
	ownerNames := make(map[int64]string, len(teamsByID))
	for id, t := range teamsByID {
		ownerNames[id] = t.Name
	}

	blocks := make([]export.TeamBlock, 0, len(teamIDs))
	for _, id := range teamIDs {
		goals := goalsByTeam[id]
		for gi := range goals {
			g := &goals[gi]
			// compute progress (batched loader does not set it)
			for ki := range g.KeyResults {
				g.KeyResults[ki].Progress = CalculateKRProgress(g.KeyResults[ki])
			}
			g.Progress = CalculateGoalProgress(g)
			if notes != nil {
				for ki := range g.KeyResults {
					g.KeyResults[ki].Note = notes[g.KeyResults[ki].ID]
				}
			}
			if comments != nil {
				g.Comments = comments[g.ID]
			}
		}
		blocks = append(blocks, export.TeamBlock{
			Heading: strings.Join(pathByID[id], " / "),
			TeamID:  id, Goals: goals, OwnerNames: ownerNames,
		})
	}
	return blocks, nil
}

func allowedContains(allowed []int64, id int64) bool {
	if allowed == nil {
		return true // unrestricted (admin)
	}
	for _, a := range allowed {
		if a == id {
			return true
		}
	}
	return false
}
```

Добавить sentinel-ошибку рядом с остальными в `service.go`:
```go
	ErrGoalNotOnTeamBoard = errors.New("goal not on team board")
```

Добавить хелперы дерева (в `export.go` сервиса): `orderedSubtreeIDs(rootID, nodes)` — DFS начиная с root включительно; `indexTeamsWithPaths(nodes)` — карты `id→domain.Team` и `id→[]string` (путь root→node именами). Реализация:

```go
func orderedSubtreeIDs(rootID int64, nodes []TeamNode) []int64 {
	var out []int64
	var walk func(items []TeamNode, inSubtree bool)
	walk = func(items []TeamNode, inSubtree bool) {
		for _, n := range items {
			here := inSubtree || n.Team.ID == rootID
			if here {
				out = append(out, n.Team.ID)
			}
			walk(n.Children, here)
		}
	}
	walk(nodes, false)
	return out
}

func indexTeamsWithPaths(nodes []TeamNode) (map[int64]domain.Team, map[int64][]string) {
	teams := map[int64]domain.Team{}
	paths := map[int64][]string{}
	var walk func(items []TeamNode, prefix []string)
	walk = func(items []TeamNode, prefix []string) {
		for _, n := range items {
			p := append(append([]string{}, prefix...), n.Team.Name)
			teams[n.Team.ID] = n.Team
			paths[n.Team.ID] = p
			walk(n.Children, p)
		}
	}
	walk(nodes, nil)
	return teams, paths
}
```

- [ ] **Step 5: Запустить тесты сервиса — убедиться, что проходят**

Run: `go test ./internal/service/ -run TestExportOKR -v`
Expected: PASS (или SKIP, если Docker недоступен — тогда прогнать локально с Docker)

- [ ] **Step 6: Проверить компиляцию всего проекта и vet**

Run: `go build ./... && go vet ./...`
Expected: без ошибок (интерфейсы `GoalRepo`/`KRRepo` удовлетворяются конкретными репозиториями; любые тестовые фейки этих интерфейсов, если есть, должны получить новые методы — при ошибке добавить методы-заглушки в фейки).

- [ ] **Step 7: Commit** (за пользователем)

```bash
git add internal/service/service.go internal/service/export.go internal/service/export_test.go
git commit -m "add ExportOKR service orchestration"
```

---

## Task 6: HTTP endpoint `GET /api/v1/teams/{teamID}/export` + обновление спеки 040

**Files:**
- Modify: `internal/http/handlers/api/v1/teams/handler.go`
- Modify: `internal/http/handlers/api/v1/teams/routes.go`
- Create: `internal/http/handlers/api/v1/teams/export_integration_test.go`
- Modify: `specs/040-api-contract.md`

**Interfaces:**
- Consumes: `service.ExportOKR`, `service.ExportParams`, `service.ExportResult`, `export.{Scope,Options,Format}`; `auth.CanAccessTeamFromCtx`, `auth.AllowedTeamIDsFromCtx`, `auth.TenantScopeFromContext`; `common.ParseID`, `common.ParsePeriodID`; `v1.WriteJSON`, `v1.WriteError`.
- Produces: `func (h *Handler) HandleTeamExport(w http.ResponseWriter, r *http.Request)`; маршрут `GET /api/v1/teams/{teamID}/export`.

- [ ] **Step 1: Написать падающий integration-тест (валидация scope, 404 вне доступа, happy path team-scope)**

Создать `export_integration_test.go` в пакете `teams_test`, повторив setup из `integration_test.go` (контейнер, миграции, `store.New`, `service.NewFromStore`, `testutil.NewAPIV1RouterWithScope(svc, allowed)`), вставить команду+период+цель, затем:

```go
	// happy path: team scope, short
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/v1/teams/%d/export?period_id=%d&scope=team", teamID, periodID), nil)
	router.ServeHTTP(rec, req.WithContext(auth.WithAllowedTeamIDs(req.Context(), []int64{teamID})))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Filename string `json:"filename"`
		Markdown string `json:"markdown"`
		Lines    int    `json:"lines"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Filename == "" || !strings.Contains(body.Markdown, "# ") || body.Lines == 0 {
		t.Fatalf("unexpected body: %+v", body)
	}

	// invalid scope -> 400
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/v1/teams/%d/export?period_id=%d&scope=bogus", teamID, periodID), nil)
	router.ServeHTTP(rec2, req2.WithContext(auth.WithAllowedTeamIDs(req2.Context(), []int64{teamID})))
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad scope, got %d", rec2.Code)
	}

	// team out of scope -> 404
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/v1/teams/%d/export?period_id=%d&scope=team", teamID, periodID), nil)
	router.ServeHTTP(rec3, req3.WithContext(auth.WithAllowedTeamIDs(req3.Context(), []int64{99999})))
	if rec3.Code != http.StatusNotFound {
		t.Fatalf("expected 404 out of scope, got %d", rec3.Code)
	}
```

Импорты: добавить `"okrs/internal/auth"`, `"strings"` к существующим.

- [ ] **Step 2: Запустить тест — убедиться, что падает (404 на несуществующем маршруте)**

Run: `go test ./internal/http/handlers/api/v1/teams/ -run Export -v`
Expected: FAIL — маршрут ещё не зарегистрирован (все запросы дают 404, второй кейс не даёт 400).

- [ ] **Step 3: Реализовать handler**

В `handler.go` добавить (не забыть импорт `"okrs/internal/export"`):

```go
// GET /api/v1/teams/{teamID}/export
func (h *Handler) HandleTeamExport(w http.ResponseWriter, r *http.Request) {
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team id", map[string]string{"team_id": "invalid"})
		return
	}
	if !auth.CanAccessTeamFromCtx(r.Context(), teamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "team not found", nil)
		return
	}
	scopeCtx, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "no active tenant", nil)
		return
	}
	periodID, err := common.ParsePeriodID(r)
	if err != nil || periodID == 0 {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid period id", map[string]string{"period_id": "invalid"})
		return
	}

	q := r.URL.Query()
	exportScope := export.Scope(q.Get("scope"))
	if exportScope != export.ScopeGoal && exportScope != export.ScopeTeam && exportScope != export.ScopeTree {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid scope", map[string]string{"scope": "invalid"})
		return
	}
	format := export.Format(q.Get("format"))
	if format == "" {
		format = export.FormatShort
	}
	if format != export.FormatShort && format != export.FormatFull {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid format", map[string]string{"format": "invalid"})
		return
	}
	var goalID int64
	if exportScope == export.ScopeGoal {
		goalID, err = common.ParseID(q.Get("goal_id"))
		if err != nil || goalID == 0 {
			v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "goal_id required", map[string]string{"goal_id": "required"})
			return
		}
	}
	allowed, _ := auth.AllowedTeamIDsFromCtx(r.Context())

	res, err := h.service.ExportOKR(r.Context(), scopeCtx, service.ExportParams{
		TeamID: teamID, PeriodID: periodID, GoalID: goalID, Scope: exportScope,
		Options:        export.Options{Format: format, Comments: q.Get("comments") == "1"},
		AllowedTeamIDs: allowed,
	})
	if err != nil {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "export not available", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]any{
		"filename": res.Filename,
		"markdown": res.Markdown,
		"lines":    res.Lines,
	})
}
```

В `routes.go` добавить:
```go
	r.Get("/api/v1/teams/{teamID}/export", h.HandleTeamExport)
```

- [ ] **Step 4: Запустить тест — убедиться, что проходит**

Run: `go test ./internal/http/handlers/api/v1/teams/ -run Export -v`
Expected: PASS

- [ ] **Step 5: Обновить спеку `specs/040-api-contract.md`**

В раздел «Read endpoints» добавить `GET /api/v1/teams/{teamID}/export` и подробное описание по шаблону «Требования к новым endpoint'ам» (method+path; query `period_id`, `scope`, `goal_id`, `format`, `comments`; validation `400`/`404`; success `{filename, markdown, lines}`; идемпотентность read-only; отсутствие side effects; scope+tenant-фильтрация; в `tree` недоступные ветки исключаются из вывода и счётчиков).

- [ ] **Step 6: Прогнать сборку и vet**

Run: `go build ./... && go vet ./...`
Expected: без ошибок

- [ ] **Step 7: Commit** (за пользователем)

```bash
git add internal/http/handlers/api/v1/teams/ specs/040-api-contract.md
git commit -m "add team export endpoint and contract"
```

---

## Task 7: Фронтенд — `ExportModal` + пункт меню «···» на карточке цели + спеки 030/050

**Files:**
- Modify: `internal/web/static/tracker.js`
- Modify: `internal/web/static/tracker.css`
- Modify: `specs/030-user-flows.md`, `specs/050-permissions-and-lifecycle.md`

**Interfaces:**
- Consumes: `apiGet`-эквивалент (проверить имя в `api.js`; если запросы делаются через `fetch` — использовать существующий паттерн из соседних компонентов), endpoint `GET /api/v1/teams/{teamID}/export`, глобальные `React`, `useState`; данные `GoalCard` (`goal`, `currentTeamId`, `periodId`), период/путь команды из props трекера, `hierarchy` для счётчика команд.
- Produces: React-компоненты `ExportMenu` (кнопка «···» + меню) и `ExportModal`; вставка `ExportMenu` в мета-строку `GoalCard` рядом с `CopyLinkButton`.

> Фронтенд без сборщика и без JS-тест-харнесса — проверка ручная (см. verification). Все пользовательские строки рендерятся как React-текст (без `dangerouslySetInnerHTML`); превью Markdown показывается как **plain text** в `<pre>` (это исходник для копирования, не рендер).

- [ ] **Step 1: Реализовать `ExportModal` в `tracker.js`**

Добавить компонент (рядом с другими модалками). Ключевые требования:
- props: `{ goal, teamId, periodId, periodName, teamPath, teamGoalCount, subtreeTeamCount, onClose }`.
- state: `scope` (init `'goal'`), `full` (bool), `comments` (bool), `data` (`{filename, markdown, lines}` | null), `loading`, `error`, `copied`.
- эффект: при изменении `scope/full/comments` — debounced (~250ms) `fetch` к `/api/v1/teams/${teamId}/export?period_id=${periodId}&scope=${scope}&format=${full?'full':'short'}&comments=${comments?'1':'0'}` (+`&goal_id=${goal.id}` при `scope==='goal'`); ответ в `data`; ошибки в `error`.
- три карточки-скоупа с подписями и счётчиками: «Одна цель» (1 цель), «Цели команды» (`teamGoalCount` целей), «С вложенными командами» (`subtreeTeamCount` команд; число целей — из `data.lines`? нет — показать «N команд в структуре», а число целей появляется после загрузки как отдельная подпись, если решишь считать на клиенте — иначе только команды). Активная карточка подсвечена.
- два чекбокса: «Полный экспорт» → `full`, «Комментарии» → `comments`.
- превью: `<pre className="export-modal__preview">{data ? data.markdown : ''}</pre>`; при `loading` — «Загрузка превью…»; при `error` — заглушка с кнопкой «Повторить».
- футер: слева `{data?.filename} · {data?.lines} строк`; кнопки «Закрыть» (`onClose`), «Скопировать» (копирует `data.markdown` через `navigator.clipboard` с fallback на `textarea+execCommand`, показывает ✓ 1.5с), «Скачать .md» (Blob из `data.markdown`, `download = data.filename`).
- закрытие по Esc и клику по фону (как у существующих модалок — свериться с `ConfirmModal`/`GoalModal`).

- [ ] **Step 2: Добавить `ExportMenu` (кнопка «···» + выпадающее меню) и вставить в мета-строку `GoalCard`**

- Компонент `ExportMenu({ goal, teamId, periodId, periodName, teamPath, teamGoalCount, subtreeTeamCount })`: кнопка «···», по клику открывает маленькое меню с пунктом «Экспорт в Markdown» (подпись «только эта цель — или шире»); выбор пункта открывает `ExportModal`. Меню закрывается по клику вне.
- В `GoalCard` (`internal/web/static/tracker.js`, мета-строка около `CopyLinkButton`, ~строка 1280) добавить `<ExportMenu ... />` сразу после `<CopyLinkButton ... />`. Значения `periodName`, `teamPath`, `teamGoalCount`, `subtreeTeamCount` пробросить в `GoalCard` из родителя (страница трекера уже знает период, команду и hierarchy). Если проще — `teamGoalCount` = число целей на текущей доске (`goals.length`), `subtreeTeamCount` = размер поддерева из hierarchy (клиент уже имеет); `teamPath` — путь команды из hierarchy.

- [ ] **Step 3: Добавить стили в `tracker.css`**

Добавить классы `.export-modal`, `.export-modal__scopes`, `.export-modal__scope-card` (+`--active`), `.export-modal__preview` (моноширинный, скролл, `white-space: pre`), `.export-modal__footer`, `.export-menu`, `.export-menu__dropdown`, консистентно с существующими модалками/меню трекера (переиспользовать токены из `tokens.css`, отступы/радиусы как у `GoalModal`/`ConfirmModal`).

- [ ] **Step 4: Ручная проверка в приложении**

Запустить приложение (см. skill `run` или `docker-compose up`), открыть трекер:
- У цели нажать «···» → «Экспорт в Markdown» — открывается модалка со скоупом «Одна цель», превью загружается.
- Переключить на «Цели команды» и «С вложенными командами» — превью и счётчики обновляются; недоступные ветки не появляются.
- Включить «Полный экспорт» и «Комментарии» — превью дополняется метаданными/деталями KR/комментариями.
- «Скопировать» кладёт текст в буфер (проверить вставкой); «Скачать .md» скачивает файл с именем из футера.
- Проверить пользователем без доступа к части поддерева (или admin/`AUTH_MODE=disabled`), что scope-фильтрация работает.

Ожидаемо: поведение соответствует макету; ошибок в консоли нет.

- [ ] **Step 5: Обновить спеки 030 и 050**

- `specs/030-user-flows.md`: в раздел «5. Работа с целью» (или новый подпункт) добавить флоу экспорта: страница трекера, API hydration (`GET .../export`), точка входа «···» у цели, три скоупа/два вида/комментарии, копирование/скачивание, empty/error/loading, независимость от team period status, серверная scope+tenant-фильтрация.
- `specs/050-permissions-and-lifecycle.md`: в «Требование к новым фичам» зафиксировать ответы — экспорт read-only; не зависит от `team period status`; разрешён в `validated`/`closed`; проверяется на сервере (tenant+scope); не зависит от будущих ролей.

- [ ] **Step 6: Финальная проверка сборки/тестов бэкенда**

Run: `go build ./... && go test ./internal/export/ ./internal/service/ ./internal/http/handlers/api/v1/teams/ && go vet ./...`
Expected: PASS (DB-тесты могут SKIP без Docker — прогнать с Docker перед сдачей).

- [ ] **Step 7: Commit** (за пользователем)

```bash
git add internal/web/static/tracker.js internal/web/static/tracker.css specs/030-user-flows.md specs/050-permissions-and-lifecycle.md
git commit -m "add markdown export UI to tracker"
```

---

## Self-Review (выполнено автором плана)

- **Spec coverage:** три скоупа (Task 1/4/5), краткий/полный (Task 1/2), комментарии (Task 3), дедуп общих целей (Task 4), имя файла (Task 4), scope+tenant (Task 5/6), endpoint (Task 6), UI/модалка/меню/копирование/скачивание/empty-error-loading (Task 7), спеки 040/030/050 (Task 6/7). Схема БД не меняется — миграции нет (соответствует дизайну).
- **Placeholder scan:** конкретный код и тесты в каждой Go-задаче; фронтенд-задача описательна из-за отсутствия JS-тест-харнесса (проверка ручная), но с явным списком требований и verification-шагом.
- **Type consistency:** `export.{Format,Options,Scope,TeamBlock,Markdown,Filename}` определены в Task 1/2/4 и используются в Task 5/6 с теми же сигнатурами; `service.{ExportParams,ExportResult,ExportOKR}` определены в Task 5 и вызываются в Task 6; `TeamBlock.OwnerNames`/`TeamID`/`Heading` согласованы между Task 4 (дедуп) и Task 5 (заполнение). Интерфейсные методы `ListGoalCommentsByGoals`/`BatchLoadNotes` добавлены в Task 5 и уже существуют в конкретных репозиториях.
- **Известное ограничение:** в `scope=tree` порядок целей внутри команды — по `id` (загрузчик `ListGoalsByTeamsPeriod`), а не по `sort_order` (как на доске); для `goal`/`team` порядок доски сохраняется через `GetTeamOKR`. Приемлемо для экспорта; при необходимости выравнивания — отдельная задача.
```
