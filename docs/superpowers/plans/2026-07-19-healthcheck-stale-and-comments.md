# Health Check-in: stale от начала периода + категория «Комментарии» — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Исправить категорию «Нет обновлений» (порог от начала периода) и добавить в колокольчик новую категорию «Комментарии» с двумя под-типами (нерешённые в scope + мои решённые), с admin-настройками и watermark-счётчиком в браузере.

**Architecture:** Backend вычисляет категории in-memory из кэша `PeriodData` (tenant+period), догружая комментарии к целям. Категории комментариев считаются по `userUDID` независимо от admin-scope. Счётчик решённых комментариев корректируется на клиенте по watermark в `localStorage` (сервер не знает seen-состояние). Admin-настройки хранятся в существующем ключе `health_checkin_config` (`tenant_settings`), без миграций.

**Tech Stack:** Go (backend, `internal/service`, `internal/store/goals`, `internal/http`), React через babel-in-browser (frontend `internal/web/static/*.js`), PostgreSQL (pgx).

## Global Constraints

- Не добавлять SQL-миграции: используются существующие `tenant_settings`; seen-состояние — в браузере (`localStorage`), не в БД.
- Права endpoint `GET /api/v1/health-checkin` не меняются (любой авторизованный). Категории комментариев — read-only агрегирование, от team period status не зависят.
- Матчинг «мой» — по `user.UDID` (как в существующем health-checkin), не по display_name.
- Clean/layered architecture: SQL — только в `internal/store`, бизнес-логика — в `internal/service`, HTTP — в handlers. Не протекать абстракциями между слоями.
- Спеки на русском; дизайн-источник: `docs/superpowers/specs/2026-07-19-healthcheck-stale-and-comments-design.md`.
- Не делать git commit'ов вне шагов плана (пользователь коммитит сам — шаги «Commit» оставлены как ориентир; при inline-исполнении спрашивать подтверждение перед commit).
- Проверки: `go test ./...`, `go vet ./...` должны быть зелёными.

---

## Обзор структуры файлов

- `internal/service/healthcheckin.go` — конфиг (+`CommentDepth`, +`ResolvedCommentsLimit`), stale-фикс, `computeCommentScope`, comment-категория, новые result-типы.
- `internal/service/healthcheckin_test.go` — тесты (stale + comment scope + comment категории). Обновить сигнатуру вызовов `computeCategories`.
- `internal/store/goals/goals.go` — экспортированный метод `ListGoalCommentsByGoals`.
- `internal/store/goals/goals_comments_test.go` — тест на новый store-метод.
- `internal/http/server.go` — `hcLoader` догружает комментарии в `PeriodData`.
- `internal/http/handlers/api/v1/healthcheckin/handler.go` — валидация новых полей конфига.
- `internal/web/static/admin.js` — секция «Комментарии» в настройках health-checkin.
- `internal/web/static/tracker.js` — рендер категории `comments`, watermark, корректировка бейджа.
- `specs/040-api-contract.md`, `specs/020-domain-model.md` — обновления контракта/домена.

---

## Task 1: Backend stale-фикс — порог от начала периода

**Files:**
- Modify: `internal/service/healthcheckin.go` (функция `computeCategories`, блок вычисления `isStale`, ~строки 223-247)
- Test: `internal/service/healthcheckin_test.go`
- Modify (docs): `specs/040-api-contract.md:410`

**Interfaces:**
- Consumes: `computeCategories(data *PeriodData, scopeIDs []int64, cfg HealthCheckInConfig, now time.Time)` (существующая сигнатура — в этой задаче НЕ меняется; `userUDID` добавит Task 5).
- Produces: поведение — цель с `lastProgress == nil` использует `data.Period.StartDate` как точку отсчёта; `days_since_update` считается от неё.

- [ ] **Step 1: Написать падающий тест — never-updated в начале периода не stale**

В `internal/service/healthcheckin_test.go` добавить:

```go
func TestCategories_NeverUpdated_NotStaleBeforeThreshold(t *testing.T) {
	// Период начался 3 дня назад; порог 7 дней; цель без обновлений прогресса.
	now := time.Now()
	kr := domain.KeyResult{ID: 1, GoalID: 1, Title: "KR1", Weight: 100, Kind: domain.KRKindBoolean,
		Boolean: &domain.KRBoolean{}} // ProgressUpdatedAt == nil
	g := makeGoal(1, 1, "", []domain.KeyResult{kr})
	teams := []domain.Team{makeTeam(1, "T1", "", nil)}
	goals := map[int64][]domain.Goal{1: {g}}
	statuses := map[int64]domain.TeamPeriodStatus{1: domain.TeamPeriodStatusInProgress}
	data := &PeriodData{
		PeriodID: 1,
		Period:   domain.Period{ID: 1, StartDate: now.AddDate(0, 0, -3), EndDate: now.AddDate(0, 1, 0)},
		Teams:    teams, GoalsByTeam: goals, Statuses: statuses,
	}
	result := computeCategories(data, []int64{1}, makeCfg(), now)
	if result.Categories["stale"].Count != 0 {
		t.Fatalf("expected 0 stale (3 days < 7 threshold from period start), got %d", result.Categories["stale"].Count)
	}
}

func TestCategories_NeverUpdated_StaleAfterThresholdFromPeriodStart(t *testing.T) {
	// Период начался 10 дней назад; порог 7; цель без обновлений → stale, days_since_update == 10.
	now := time.Now()
	kr := domain.KeyResult{ID: 1, GoalID: 1, Title: "KR1", Weight: 100, Kind: domain.KRKindBoolean,
		Boolean: &domain.KRBoolean{}}
	g := makeGoal(1, 1, "", []domain.KeyResult{kr})
	teams := []domain.Team{makeTeam(1, "T1", "", nil)}
	goals := map[int64][]domain.Goal{1: {g}}
	statuses := map[int64]domain.TeamPeriodStatus{1: domain.TeamPeriodStatusInProgress}
	data := &PeriodData{
		PeriodID: 1,
		Period:   domain.Period{ID: 1, StartDate: now.AddDate(0, 0, -10), EndDate: now.AddDate(0, 1, 0)},
		Teams:    teams, GoalsByTeam: goals, Statuses: statuses,
	}
	result := computeCategories(data, []int64{1}, makeCfg(), now)
	if result.Categories["stale"].Count != 1 {
		t.Fatalf("expected 1 stale (10 days > 7 from period start), got %d", result.Categories["stale"].Count)
	}
	if got := result.Categories["stale"].Items[0].DaysSinceUpdate; got != 10 {
		t.Fatalf("expected days_since_update 10 (from period start), got %d", got)
	}
}

func TestCategories_NeverUpdated_FuturePeriod_NotStale(t *testing.T) {
	// Период ещё не начался (StartDate в будущем) → не stale, даже при in_progress.
	now := time.Now()
	kr := domain.KeyResult{ID: 1, GoalID: 1, Title: "KR1", Weight: 100, Kind: domain.KRKindBoolean,
		Boolean: &domain.KRBoolean{}}
	g := makeGoal(1, 1, "", []domain.KeyResult{kr})
	teams := []domain.Team{makeTeam(1, "T1", "", nil)}
	goals := map[int64][]domain.Goal{1: {g}}
	statuses := map[int64]domain.TeamPeriodStatus{1: domain.TeamPeriodStatusInProgress}
	data := &PeriodData{
		PeriodID: 1,
		Period:   domain.Period{ID: 1, StartDate: now.AddDate(0, 0, 5), EndDate: now.AddDate(0, 2, 0)},
		Teams:    teams, GoalsByTeam: goals, Statuses: statuses,
	}
	result := computeCategories(data, []int64{1}, makeCfg(), now)
	if result.Categories["stale"].Count != 0 {
		t.Fatalf("expected 0 stale (future period), got %d", result.Categories["stale"].Count)
	}
}
```

- [ ] **Step 2: Запустить тесты — убедиться, что падают**

Run: `go test ./internal/service/ -run 'TestCategories_NeverUpdated' -v`
Expected: FAIL — `TestCategories_NeverUpdated_NotStaleBeforeThreshold` падает (сейчас `lastProgress == nil` → немедленно stale, count 1 вместо 0).

- [ ] **Step 3: Исправить вычисление isStale**

В `internal/service/healthcheckin.go`, внутри цикла `for _, g := range goals` найти блок:

```go
			goalProgress := CalculateGoalProgress(&g)
			lastProgress := goalLastProgressAt(g)
			daysSince := 0
			if lastProgress != nil {
				daysSince = int(now.Sub(*lastProgress).Hours() / 24)
			}
			isStale := len(g.KeyResults) > 0 && (lastProgress == nil || daysSince > cfg.StaleDays)
```

Заменить на:

```go
			goalProgress := CalculateGoalProgress(&g)
			lastProgress := goalLastProgressAt(g)
			// Точка отсчёта «дней без обновления»: последнее обновление прогресса, а если
			// его не было — начало периода. От неё отмеряется StaleDays. Для ещё не
			// начавшегося периода daysSince <= 0, поэтому цель не попадает в stale.
			ref := lastProgress
			if ref == nil {
				ref = &data.Period.StartDate
			}
			daysSince := int(now.Sub(*ref).Hours() / 24)
			isStale := len(g.KeyResults) > 0 && daysSince > cfg.StaleDays
```

- [ ] **Step 4: Запустить тесты — убедиться, что проходят**

Run: `go test ./internal/service/ -run 'TestCategories' -v`
Expected: PASS — все три новых теста зелёные; существующие `TestCategories_StaleGoal*` не сломаны (там `ProgressUpdatedAt` задан, ветка `ref == nil` не активируется).

- [ ] **Step 5: Обновить спеку контракта**

В `specs/040-api-contract.md` заменить абзац про `stale` (строка ~410) на:

```
Категория `stale` («N дней без обновления») — сигнал фазы исполнения: считается только для команд в статусе `in_progress` («в работе»). Точка отсчёта порога — последнее обновление прогресса KR цели; если обновлений не было, порог отсчитывается от начала периода (`period.start_date`). Цель попадает в категорию только если прошло больше `stale_days` дней от этой точки, поэтому не начавшийся период (start_date в будущем) целей в категорию не добавляет. Для статусов `forming`, `ready`, `closed` и для команд без записи статуса за период предупреждение не применяется. То же правило действует для предупреждения на карточке цели в трекере.
```

- [ ] **Step 6: Commit**

```bash
git add internal/service/healthcheckin.go internal/service/healthcheckin_test.go specs/040-api-contract.md
git commit -m "fix(health-checkin): отсчитывать stale от начала периода для целей без обновлений"
```

**Примечание по frontend (Часть 1):** отдельного изменения `GoalCard` не требуется. `GoalCard` уже гейтит предупреждение на `periodStatus === 'in_progress'` и считает `goal.updatedDaysAgo` от `updated_at` (для нетронутой цели ≈ момент создания ≈ начало периода), поэтому «мгновенного stale от нуля» на фронте нет — баг был только в backend health-checkin. Проверить вручную в Task 8 (панель трекера).

---

## Task 2: Конфиг — поля comment_depth, resolved_comments_limit, ключ comments

**Files:**
- Modify: `internal/service/healthcheckin.go` (`HealthCheckInConfig`, `defaultHealthCheckInConfig`, `LoadHealthCheckInConfig`)
- Modify: `internal/http/handlers/api/v1/healthcheckin/handler.go` (`HandleUpdateHealthCheckInSettings` — валидация)
- Test: `internal/service/healthcheckin_test.go`
- Modify (docs): `specs/020-domain-model.md` (описание `health_checkin_config`), `specs/040-api-contract.md` (тело POST settings)

**Interfaces:**
- Produces: `HealthCheckInConfig.CommentDepth int`, `HealthCheckInConfig.ResolvedCommentsLimit int`; дефолты `CommentDepth=1`, `ResolvedCommentsLimit=5`, `InCounter["comments"]=false`.

- [ ] **Step 1: Написать падающий тест на дефолты и нормализацию**

В `internal/service/healthcheckin_test.go` добавить (тип `stubSettingsReader` объявить, если его ещё нет):

```go
type stubSettingsReader struct{ raw json.RawMessage; err error }

func (s stubSettingsReader) GetTenant(_ context.Context, _ domain.TenantScope, _ string) (json.RawMessage, error) {
	return s.raw, s.err
}

func TestLoadConfig_CommentDefaults(t *testing.T) {
	cfg, err := LoadHealthCheckInConfig(context.Background(), domain.TenantScope{TenantID: 1}, stubSettingsReader{raw: nil})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CommentDepth != 1 {
		t.Errorf("expected default comment_depth 1, got %d", cfg.CommentDepth)
	}
	if cfg.ResolvedCommentsLimit != 5 {
		t.Errorf("expected default resolved_comments_limit 5, got %d", cfg.ResolvedCommentsLimit)
	}
	if cfg.InCounter["comments"] {
		t.Errorf("expected in_counter[comments] default false")
	}
}

func TestLoadConfig_NormalizesInvalidCommentFields(t *testing.T) {
	raw := json.RawMessage(`{"comment_depth":-2,"resolved_comments_limit":0,"stale_days":7,"cache_ttl_minutes":5,"green_threshold":80}`)
	cfg, _ := LoadHealthCheckInConfig(context.Background(), domain.TenantScope{TenantID: 1}, stubSettingsReader{raw: raw})
	if cfg.CommentDepth != 0 {
		t.Errorf("negative comment_depth should clamp to 0, got %d", cfg.CommentDepth)
	}
	if cfg.ResolvedCommentsLimit != 5 {
		t.Errorf("non-positive resolved_comments_limit should reset to default 5, got %d", cfg.ResolvedCommentsLimit)
	}
}
```

Убедиться, что в начале файла есть импорты `context` и `encoding/json` (добавить при необходимости).

- [ ] **Step 2: Запустить — убедиться, что падает**

Run: `go test ./internal/service/ -run 'TestLoadConfig' -v`
Expected: FAIL — поля `CommentDepth`/`ResolvedCommentsLimit` не существуют (ошибка компиляции).

- [ ] **Step 3: Добавить поля и дефолты**

В `internal/service/healthcheckin.go`:

В структуру `HealthCheckInConfig` добавить поля (после `GreenThreshold`):

```go
	// CommentDepth is how many hierarchy levels below the user's lead teams are scanned
	// for unresolved comments (0 = only the user's own teams, 1 = + direct children, …).
	CommentDepth int `json:"comment_depth"`
	// ResolvedCommentsLimit is how many of the user's most recently resolved comments (K)
	// are shown in the resolved sub-list.
	ResolvedCommentsLimit int             `json:"resolved_comments_limit"`
	InCounter             map[string]bool `json:"in_counter"`
```

(Строку `InCounter map[string]bool json:"in_counter"` перенести/оставить одну — не дублировать.)

В `defaultHealthCheckInConfig` добавить:

```go
	CommentDepth:          1,
	ResolvedCommentsLimit: 5,
```

и в `InCounter` map добавить ключ:

```go
		"comments": false,
```

В `LoadHealthCheckInConfig`, после блока нормализации `GreenThreshold`, добавить:

```go
	if cfg.CommentDepth < 0 {
		cfg.CommentDepth = 0
	}
	if cfg.ResolvedCommentsLimit < 1 {
		cfg.ResolvedCommentsLimit = defaultHealthCheckInConfig.ResolvedCommentsLimit
	}
```

- [ ] **Step 4: Добавить валидацию в handler**

В `internal/http/handlers/api/v1/healthcheckin/handler.go`, в `HandleUpdateHealthCheckInSettings`, после проверки `green_threshold` добавить:

```go
	if body.CommentDepth < 0 {
		writeError(w, http.StatusBadRequest, "comment_depth must be >= 0")
		return
	}
	if body.ResolvedCommentsLimit < 1 {
		writeError(w, http.StatusBadRequest, "resolved_comments_limit must be >= 1")
		return
	}
```

- [ ] **Step 5: Запустить — убедиться, что проходит**

Run: `go test ./internal/service/ -run 'TestLoadConfig' -v && go vet ./internal/service/ ./internal/http/...`
Expected: PASS; vet чистый.

- [ ] **Step 6: Обновить спеки**

В `specs/020-domain-model.md`, в описании `health_checkin_config` (строка ~299) добавить в перечисление полей: `comment_depth` (int ≥ 0, по умолчанию 1 — глубина спуска для нерешённых комментариев), `resolved_comments_limit` (int ≥ 1, по умолчанию 5 — сколько моих последних решённых комментариев показывать), а в `in_counter` — ключ `comments` (по умолчанию false).

В `specs/040-api-contract.md`, в теле `POST /api/v1/admin/settings/health-checkin` (строка ~431) добавить поля `comment_depth`, `resolved_comments_limit` и ключ `comments` в `in_counter`; валидация: `comment_depth >= 0`, `resolved_comments_limit >= 1`.

- [ ] **Step 7: Commit**

```bash
git add internal/service/healthcheckin.go internal/http/handlers/api/v1/healthcheckin/handler.go internal/service/healthcheckin_test.go specs/020-domain-model.md specs/040-api-contract.md
git commit -m "feat(health-checkin): конфиг comment_depth, resolved_comments_limit, ключ comments"
```

---

## Task 3: Store — загрузка комментариев к целям + wiring в PeriodData

**Files:**
- Modify: `internal/store/goals/goals.go` (новый экспортируемый метод `ListGoalCommentsByGoals`)
- Modify: `internal/http/server.go` (`hcLoader` — догрузка комментариев в goals)
- Test: `internal/store/goals/goals_comments_test.go`

**Interfaces:**
- Produces: `func (r *GoalRepository) ListGoalCommentsByGoals(ctx context.Context, scope domain.TenantScope, goalIDs []int64) (map[int64][]domain.GoalComment, error)` — комментарии по списку goal ID (включая `AuthorUDID`, `ResolvedAt`, `ResolvedByUDID`).
- Consumes (в server.go): `service.PeriodData.GoalsByTeam` — каждому goal проставляется `.Comments`.

- [ ] **Step 1: Написать падающий тест на store-метод**

В `internal/store/goals/goals_comments_test.go` добавить тест по образцу существующих (использовать те же helpers/фикстуры, что в файле — открытие тестовой БД, создание team/period/goal, `AddGoalComment`). Пример:

```go
func TestListGoalCommentsByGoals(t *testing.T) {
	ctx := context.Background()
	repo, scope, cleanup := newTestGoalRepo(t) // helper из этого пакета; если имя иное — использовать существующий
	defer cleanup()

	goalID := seedGoalWithTeamPeriod(t, repo, scope) // helper из этого пакета
	c1, err := repo.AddGoalComment(ctx, scope, goalID, "открытый", 1)
	if err != nil { t.Fatal(err) }
	c2, err := repo.AddGoalComment(ctx, scope, goalID, "решённый", 1)
	if err != nil { t.Fatal(err) }
	if _, err := repo.SetGoalCommentResolved(ctx, scope, goalID, c2, true, 1); err != nil { t.Fatal(err) }

	byGoal, err := repo.ListGoalCommentsByGoals(ctx, scope, []int64{goalID})
	if err != nil { t.Fatal(err) }
	if len(byGoal[goalID]) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(byGoal[goalID]))
	}
	var open, resolved int
	for _, c := range byGoal[goalID] {
		if c.ResolvedAt == nil { open++ } else { resolved++ }
	}
	if open != 1 || resolved != 1 {
		t.Fatalf("expected 1 open + 1 resolved, got %d/%d", open, resolved)
	}
	_ = c1
}
```

> Перед написанием: открыть `internal/store/goals/goals_comments_test.go` и переиспользовать фактические имена helper'ов (setup БД, создание goal). Не выдумывать — скопировать паттерн ближайшего теста в файле.

- [ ] **Step 2: Запустить — убедиться, что падает**

Run: `go test ./internal/store/goals/ -run 'TestListGoalCommentsByGoals' -v`
Expected: FAIL — метод `ListGoalCommentsByGoals` не определён.

- [ ] **Step 3: Добавить экспортируемый метод**

В `internal/store/goals/goals.go` добавить (рядом с `listGoalCommentsBatch`, переиспользуя его):

```go
// ListGoalCommentsByGoals returns all comments (open and resolved) for the given goal IDs,
// keyed by goal ID. Used by the health check-in cache to compute comment categories.
func (r *GoalRepository) ListGoalCommentsByGoals(ctx context.Context, scope domain.TenantScope, goalIDs []int64) (map[int64][]domain.GoalComment, error) {
	return r.listGoalCommentsBatch(ctx, scope, goalIDs)
}
```

- [ ] **Step 4: Запустить — убедиться, что проходит**

Run: `go test ./internal/store/goals/ -run 'TestListGoalCommentsByGoals' -v`
Expected: PASS.

- [ ] **Step 5: Догрузить комментарии в hcLoader**

В `internal/http/server.go`, в `hcLoader`, после получения `goalsByTeam` (строка ~129) и перед `return &service.PeriodData{...}` добавить:

```go
		goalIDSet := make(map[int64]struct{})
		for _, goals := range goalsByTeam {
			for _, g := range goals {
				goalIDSet[g.ID] = struct{}{}
			}
		}
		goalIDs := make([]int64, 0, len(goalIDSet))
		for id := range goalIDSet {
			goalIDs = append(goalIDs, id)
		}
		commentsByGoal, err := st.Goals.ListGoalCommentsByGoals(ctx, scope, goalIDs)
		if err != nil {
			return nil, err
		}
		for teamID, goals := range goalsByTeam {
			for i := range goals {
				goals[i].Comments = commentsByGoal[goals[i].ID]
			}
			goalsByTeam[teamID] = goals
		}
```

> `goalsByTeam` — `map[int64][]domain.Goal`; итерируем по значению-слайсу и мутируем по индексу `goals[i]`, затем переприсваиваем в map (слайс — референс на backing array, переприсваивание безопасно и явно).

- [ ] **Step 6: Проверка сборки**

Run: `go build ./... && go vet ./internal/http/...`
Expected: успешно, без ошибок.

- [ ] **Step 7: Commit**

```bash
git add internal/store/goals/goals.go internal/store/goals/goals_comments_test.go internal/http/server.go
git commit -m "feat(health-checkin): догрузка комментариев целей в PeriodData"
```

---

## Task 4: computeCommentScope — scope нерешённых комментариев по глубине

**Files:**
- Modify: `internal/service/healthcheckin.go` (новая функция `computeCommentScope`)
- Test: `internal/service/healthcheckin_test.go`

**Interfaces:**
- Produces: `func computeCommentScope(teams []domain.Team, goalsByTeam map[int64][]domain.Goal, userUDID string, depth int) map[int64]struct{}` — множество team ID: команды, где юзер лид, + их потомки до `depth` уровней, + owner-команды (depth 0).

- [ ] **Step 1: Написать падающий тест**

В `internal/service/healthcheckin_test.go` добавить:

```go
func TestComputeCommentScope_LeadDepth(t *testing.T) {
	// 1(lead alice) → 2 → 3 → 4 ; owner-only команда 5.
	teams := []domain.Team{
		makeTeamWithUDID(1, "L0", strPtr("udid-alice"), nil),
		makeTeamWithUDID(2, "L1", nil, teamPtr(1)),
		makeTeamWithUDID(3, "L2", nil, teamPtr(2)),
		makeTeamWithUDID(4, "L3", nil, teamPtr(3)),
		makeTeamWithUDID(5, "Owner", nil, nil),
	}
	goals := map[int64][]domain.Goal{
		5: {makeGoalWithUDIDs(50, 5, []string{"udid-alice"}, nil)},
	}

	depth0 := computeCommentScope(teams, goals, "udid-alice", 0)
	if _, ok := depth0[1]; !ok { t.Error("depth0 must include lead team 1") }
	if _, ok := depth0[2]; ok { t.Error("depth0 must NOT include child team 2") }
	if _, ok := depth0[5]; !ok { t.Error("depth0 must include owner team 5") }

	depth1 := computeCommentScope(teams, goals, "udid-alice", 1)
	if _, ok := depth1[2]; !ok { t.Error("depth1 must include direct child 2") }
	if _, ok := depth1[3]; ok { t.Error("depth1 must NOT include grandchild 3") }

	depth2 := computeCommentScope(teams, goals, "udid-alice", 2)
	if _, ok := depth2[3]; !ok { t.Error("depth2 must include team 3") }
	if _, ok := depth2[4]; ok { t.Error("depth2 must NOT include team 4") }
}

func TestComputeCommentScope_EmptyUser(t *testing.T) {
	teams := []domain.Team{makeTeamWithUDID(1, "L0", strPtr("udid-alice"), nil)}
	if s := computeCommentScope(teams, map[int64][]domain.Goal{}, "", 1); len(s) != 0 {
		t.Errorf("empty user → empty scope, got %d", len(s))
	}
}
```

- [ ] **Step 2: Запустить — убедиться, что падает**

Run: `go test ./internal/service/ -run 'TestComputeCommentScope' -v`
Expected: FAIL — `computeCommentScope` не определена.

- [ ] **Step 3: Реализовать computeCommentScope**

В `internal/service/healthcheckin.go` добавить:

```go
// computeCommentScope returns the set of team IDs whose goals' unresolved comments are
// surfaced to the user: teams the user leads plus their descendants down to `depth` levels
// (depth 0 = only the lead teams themselves), plus teams where the user owns a goal (depth 0).
func computeCommentScope(teams []domain.Team, goalsByTeam map[int64][]domain.Goal, userUDID string, depth int) map[int64]struct{} {
	scope := make(map[int64]struct{})
	if userUDID == "" {
		return scope
	}

	childrenMap := make(map[int64][]int64, len(teams))
	for _, t := range teams {
		if t.ParentID != nil {
			childrenMap[*t.ParentID] = append(childrenMap[*t.ParentID], t.ID)
		}
	}

	// BFS from each lead team, limited to `depth` levels down.
	var frontier []int64
	for _, t := range teams {
		if t.DeletedAt == nil && t.LeadUDID != nil && *t.LeadUDID == userUDID {
			if _, seen := scope[t.ID]; !seen {
				scope[t.ID] = struct{}{}
				frontier = append(frontier, t.ID)
			}
		}
	}
	for level := 0; level < depth && len(frontier) > 0; level++ {
		var next []int64
		for _, id := range frontier {
			for _, childID := range childrenMap[id] {
				if _, seen := scope[childID]; !seen {
					scope[childID] = struct{}{}
					next = append(next, childID)
				}
			}
		}
		frontier = next
	}

	// Owner teams (depth 0, no descent) — matches existing owner-scope semantics.
	for teamID, goals := range goalsByTeam {
		for _, g := range goals {
			for _, uid := range g.OwnerUDIDs {
				if uid == userUDID {
					scope[teamID] = struct{}{}
					break
				}
			}
		}
	}
	return scope
}
```

- [ ] **Step 4: Запустить — убедиться, что проходит**

Run: `go test ./internal/service/ -run 'TestComputeCommentScope' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service/healthcheckin.go internal/service/healthcheckin_test.go
git commit -m "feat(health-checkin): computeCommentScope с ограничением глубины"
```

---

## Task 5: Категория comments в computeCategories (unresolved + resolved)

**Files:**
- Modify: `internal/service/healthcheckin.go` (result-типы, сигнатура `computeCategories`, `emptyCategories`, `GetHealthCheckIn`)
- Test: `internal/service/healthcheckin_test.go` (обновить все вызовы `computeCategories` + новые тесты)
- Modify (docs): `specs/040-api-contract.md` (форма ответа — категория `comments`)

**Interfaces:**
- Consumes: `computeCommentScope(...)` (Task 4), `HealthCheckInConfig.CommentDepth`/`ResolvedCommentsLimit`/`InCounter["comments"]` (Task 2), `domain.Goal.Comments` (Task 3).
- Produces: новый тип `HealthCheckInCommentItem`; поля `Unresolved`/`Resolved []HealthCheckInCommentItem` в `HealthCheckInCategory`; новая сигнатура `computeCategories(data *PeriodData, scopeIDs []int64, userUDID string, cfg HealthCheckInConfig, now time.Time) *HealthCheckInResult`.

- [ ] **Step 1: Написать падающие тесты на категорию comments**

В `internal/service/healthcheckin_test.go` добавить:

```go
func TestCategories_Comments_UnresolvedInScope(t *testing.T) {
	now := time.Now()
	openC := domain.GoalComment{ID: 100, GoalID: 10, Text: "уточни KR", AuthorName: "Bob", AuthorUDID: "udid-bob", CreatedAt: now}
	g := domain.Goal{ID: 10, TeamID: 1, Title: "Цель", Weight: 100, Comments: []domain.GoalComment{openC}}
	teams := []domain.Team{makeTeamWithUDID(1, "T1", strPtr("udid-alice"), nil)}
	data := &PeriodData{
		PeriodID: 1,
		Period:   domain.Period{ID: 1, StartDate: now.AddDate(0, -1, 0), EndDate: now.AddDate(0, 1, 0)},
		Teams:    teams,
		GoalsByTeam: map[int64][]domain.Goal{1: {g}},
		Statuses:    map[int64]domain.TeamPeriodStatus{1: domain.TeamPeriodStatusInProgress},
	}
	cfg := makeCfg()
	cfg.CommentDepth = 1
	cfg.ResolvedCommentsLimit = 5
	res := computeCategories(data, []int64{1}, "udid-alice", cfg, now)
	comments := res.Categories["comments"]
	if len(comments.Unresolved) != 1 || comments.Unresolved[0].CommentID != 100 {
		t.Fatalf("expected 1 unresolved comment 100, got %+v", comments.Unresolved)
	}
	if comments.Count != 1 {
		t.Fatalf("expected comments.Count == unresolved len 1, got %d", comments.Count)
	}
}

func TestCategories_Comments_ResolvedMineExcludesSelfResolved(t *testing.T) {
	now := time.Now()
	r1 := now.AddDate(0, 0, -1)
	r2 := now.AddDate(0, 0, -2)
	// Мой коммент, решён Bob → в список.
	mineResolvedByOther := domain.GoalComment{ID: 200, GoalID: 10, Text: "готово", AuthorUDID: "udid-alice", ResolvedAt: &r1, ResolvedByUDID: "udid-bob"}
	// Мой коммент, решён мной → исключить.
	mineSelfResolved := domain.GoalComment{ID: 201, GoalID: 10, Text: "сам", AuthorUDID: "udid-alice", ResolvedAt: &r2, ResolvedByUDID: "udid-alice"}
	// Чужой коммент → не мой, исключить.
	othersResolved := domain.GoalComment{ID: 202, GoalID: 10, Text: "чужой", AuthorUDID: "udid-bob", ResolvedAt: &r1, ResolvedByUDID: "udid-carol"}
	g := domain.Goal{ID: 10, TeamID: 1, Title: "Цель", Weight: 100,
		Comments: []domain.GoalComment{mineResolvedByOther, mineSelfResolved, othersResolved}}
	teams := []domain.Team{makeTeamWithUDID(1, "T1", nil, nil)} // не lead — resolved-mine не зависит от scope
	data := &PeriodData{
		PeriodID: 1,
		Period:   domain.Period{ID: 1, StartDate: now.AddDate(0, -1, 0), EndDate: now.AddDate(0, 1, 0)},
		Teams:    teams,
		GoalsByTeam: map[int64][]domain.Goal{1: {g}},
		Statuses:    map[int64]domain.TeamPeriodStatus{1: domain.TeamPeriodStatusInProgress},
	}
	cfg := makeCfg()
	res := computeCategories(data, []int64{1}, "udid-alice", cfg, now)
	resolved := res.Categories["comments"].Resolved
	if len(resolved) != 1 || resolved[0].CommentID != 200 {
		t.Fatalf("expected only comment 200 in resolved-mine, got %+v", resolved)
	}
}

func TestCategories_Comments_ResolvedMineLimitK(t *testing.T) {
	now := time.Now()
	var comments []domain.GoalComment
	for i := int64(0); i < 8; i++ {
		ts := now.AddDate(0, 0, -int(i)) // новее = меньший i
		comments = append(comments, domain.GoalComment{
			ID: 300 + i, GoalID: 10, Text: "c", AuthorUDID: "udid-alice", ResolvedAt: &ts, ResolvedByUDID: "udid-bob",
		})
	}
	g := domain.Goal{ID: 10, TeamID: 1, Title: "Цель", Weight: 100, Comments: comments}
	teams := []domain.Team{makeTeamWithUDID(1, "T1", nil, nil)}
	data := &PeriodData{
		PeriodID: 1,
		Period:   domain.Period{ID: 1, StartDate: now.AddDate(0, -1, 0), EndDate: now.AddDate(0, 1, 0)},
		Teams:    teams,
		GoalsByTeam: map[int64][]domain.Goal{1: {g}},
		Statuses:    map[int64]domain.TeamPeriodStatus{1: domain.TeamPeriodStatusInProgress},
	}
	cfg := makeCfg()
	cfg.ResolvedCommentsLimit = 5
	res := computeCategories(data, []int64{1}, "udid-alice", cfg, now)
	resolved := res.Categories["comments"].Resolved
	if len(resolved) != 5 {
		t.Fatalf("expected limit 5, got %d", len(resolved))
	}
	// Отсортировано по resolved_at DESC → первым самый новый (id 300).
	if resolved[0].CommentID != 300 {
		t.Fatalf("expected newest (300) first, got %d", resolved[0].CommentID)
	}
}
```

- [ ] **Step 2: Запустить — убедиться, что падает (компиляция)**

Run: `go test ./internal/service/ -run 'TestCategories_Comments' -v`
Expected: FAIL — `computeCategories` не принимает `userUDID`; поля `Unresolved`/`Resolved`/`HealthCheckInCommentItem` не существуют.

- [ ] **Step 3: Добавить result-типы**

В `internal/service/healthcheckin.go`, рядом с `HealthCheckInItem`, добавить:

```go
// HealthCheckInCommentItem is one entry in the comments category (unresolved or resolved sub-list).
type HealthCheckInCommentItem struct {
	TeamID         int64      `json:"team_id"`
	TeamName       string     `json:"team_name"`
	TeamPath       []string   `json:"team_path"`
	GoalID         int64      `json:"goal_id"`
	GoalTitle      string     `json:"goal_title"`
	CommentID      int64      `json:"comment_id"`
	Text           string     `json:"text"`
	AuthorName     string     `json:"author_name,omitempty"`
	CreatedAt      time.Time  `json:"created_at,omitempty"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
	ResolvedByName string     `json:"resolved_by_name,omitempty"`
}
```

В структуру `HealthCheckInCategory` добавить два поля:

```go
	Unresolved []HealthCheckInCommentItem `json:"unresolved,omitempty"`
	Resolved   []HealthCheckInCommentItem `json:"resolved,omitempty"`
```

- [ ] **Step 4: Обновить сигнатуру computeCategories и добавить comments-логику**

В `internal/service/healthcheckin.go`:

Изменить сигнатуру:

```go
func computeCategories(data *PeriodData, scopeIDs []int64, userUDID string, cfg HealthCheckInConfig, now time.Time) *HealthCheckInResult {
```

В инициализацию `cats` добавить категорию:

```go
		"comments":            {InCounter: cfg.InCounter["comments"], Items: []HealthCheckInItem{}},
```

После основного цикла `for _, teamID := range scopeIDs { ... }` (перед подсчётом `total`) добавить вычисление comments:

```go
	// ── Comments category ─────────────────────────────────────────────
	commentScope := computeCommentScope(data.Teams, data.GoalsByTeam, userUDID, cfg.CommentDepth)
	commentsCat := cats["comments"]

	// Unresolved: open comments on goals owned by teams in commentScope.
	for teamID := range commentScope {
		team, ok := teamsByID[teamID]
		if !ok {
			continue
		}
		path := buildTeamPath(teamID, teamsByID)
		for _, g := range data.GoalsByTeam[teamID] {
			if g.TeamID != teamID { // skip shared goals (count under owner team only)
				continue
			}
			for _, c := range g.Comments {
				if c.ResolvedAt != nil {
					continue
				}
				commentsCat.Unresolved = append(commentsCat.Unresolved, HealthCheckInCommentItem{
					TeamID: teamID, TeamName: team.Name, TeamPath: path,
					GoalID: g.ID, GoalTitle: g.Title,
					CommentID: c.ID, Text: c.Text, AuthorName: c.AuthorName, CreatedAt: c.CreatedAt,
				})
			}
		}
	}

	// Resolved-mine: my comments resolved by someone else, across the whole period, newest K.
	if userUDID != "" {
		var mine []HealthCheckInCommentItem
		for teamID, goals := range data.GoalsByTeam {
			team, ok := teamsByID[teamID]
			if !ok {
				continue
			}
			path := buildTeamPath(teamID, teamsByID)
			for _, g := range goals {
				if g.TeamID != teamID {
					continue
				}
				for _, c := range g.Comments {
					if c.ResolvedAt == nil || c.AuthorUDID != userUDID || c.ResolvedByUDID == userUDID {
						continue
					}
					rc := c.ResolvedAt
					mine = append(mine, HealthCheckInCommentItem{
						TeamID: teamID, TeamName: team.Name, TeamPath: path,
						GoalID: g.ID, GoalTitle: g.Title,
						CommentID: c.ID, Text: c.Text,
						ResolvedAt: rc, ResolvedByName: c.ResolvedByName,
					})
				}
			}
		}
		sort.Slice(mine, func(i, j int) bool {
			return mine[i].ResolvedAt.After(*mine[j].ResolvedAt)
		})
		if len(mine) > cfg.ResolvedCommentsLimit {
			mine = mine[:cfg.ResolvedCommentsLimit]
		}
		commentsCat.Resolved = mine
	}
```

Найти цикл подсчёта `total` / `cat.Count`:

```go
	total := 0
	for _, cat := range cats {
		cat.Count = len(cat.Items)
		if cat.InCounter {
			total += cat.Count
		}
	}
```

Заменить на (для comments `Count` = число нерешённых):

```go
	total := 0
	for name, cat := range cats {
		if name == "comments" {
			cat.Count = len(cat.Unresolved)
		} else {
			cat.Count = len(cat.Items)
		}
		if cat.InCounter {
			total += cat.Count
		}
	}
```

Добавить импорт `"sort"` в начало файла.

- [ ] **Step 5: Обновить emptyCategories и вызовы**

В `emptyCategories` добавить `"comments"` в срез `names`:

```go
	names := []string{"stale", "no_goals", "awaiting_validation", "formation_errors", "lagging", "comments"}
```

В `GetHealthCheckIn` найти вызов `computeCategories(data, scopeIDs, cfg, time.Now())` и заменить на:

```go
	return computeCategories(data, scopeIDs, userUDID, cfg, time.Now()), nil
```

Обновить ВСЕ вызовы `computeCategories(...)` в `healthcheckin_test.go`, добавив аргумент `userUDID` четвёртым (перед `cfg`). Для существующих тестов, не проверяющих комментарии, передавать `""`. Пример: `computeCategories(data, []int64{1}, "", makeCfg(), time.Now())`.

Обновить `makeCfg()` в тесте — добавить дефолты, чтобы comments не влиял на существующие проверки total:

```go
func makeCfg() HealthCheckInConfig {
	return HealthCheckInConfig{
		StaleDays: 7, BehindMargin: 10, WeightTolerance: 0,
		CommentDepth: 1, ResolvedCommentsLimit: 5,
		InCounter: map[string]bool{
			"stale": true, "no_goals": true,
			"awaiting_validation": true, "formation_errors": true, "lagging": false,
			"comments": false,
		},
	}
}
```

- [ ] **Step 6: Запустить весь пакет — убедиться, что зелёный**

Run: `go test ./internal/service/ -v`
Expected: PASS — новые comments-тесты проходят; все существующие тесты (обновлённые вызовы) зелёные.

- [ ] **Step 7: Обновить спеку контракта (форма ответа)**

В `specs/040-api-contract.md`, в описании success-ответа `GET /api/v1/health-checkin`, добавить в объект `categories` категорию `comments` с полями `in_counter`, `count` (= число `unresolved`), `unresolved: []`, `resolved: []`, и текстом:

```
Категория `comments` имеет форму `{ in_counter, count, unresolved: [...], resolved: [...] }` (вместо `items`). `count` = число нерешённых комментариев (`unresolved`) в scope пользователя (его lead-команды + спуск на `comment_depth` уровней + owner-команды); в `total_problems` входит только при `in_counter.comments = true` (по умолчанию false). `resolved` — последние `resolved_comments_limit` комментариев пользователя, решённых не им самим, по `resolved_at` убыв.; в серверный `total_problems` НЕ входит — их «непросмотренный» счётчик считается на клиенте (watermark в localStorage). Элемент unresolved: `team_id, team_name, team_path, goal_id, goal_title, comment_id, author_name, text, created_at`. Элемент resolved: те же + `resolved_at, resolved_by_name` (без `author_name`).
```

- [ ] **Step 8: Commit**

```bash
git add internal/service/healthcheckin.go internal/service/healthcheckin_test.go specs/040-api-contract.md
git commit -m "feat(health-checkin): категория comments (unresolved + resolved-mine)"
```

---

## Task 6: Admin UI — секция «Комментарии» в настройках health-checkin

**Files:**
- Modify: `internal/web/static/admin.js` (`HealthCheckInSettingsPanel`)

**Interfaces:**
- Consumes: конфиг с полями `comment_depth`, `resolved_comments_limit`, `in_counter.comments` (Task 2). Форма шлёт их обратно через существующий `POST /api/v1/admin/settings/health-checkin`.

- [ ] **Step 1: Добавить категорию comments в catConfig**

В `internal/web/static/admin.js`, в `HealthCheckInSettingsPanel`, в массив `catConfig` добавить (после элемента `lagging`):

```js
    {
      key: 'comments', icon: '💬', label: 'Комментарии',
      hint: 'Нерешённые комментарии к целям ваших команд и команд под ними. Тумблер «В счётчик» включает их в бейдж (по умолчанию выключено). «Мои решённые» показываются всегда, их непросмотренный счётчик считается локально.',
      param: { field: 'comment_depth', label: 'Глубина команд (уровней вниз)', min: 0 },
    },
```

- [ ] **Step 2: Добавить отдельное поле «Сколько решённых показывать (K)»**

Поскольку у категории `comments` два числовых параметра, а `catConfig` поддерживает один `param`, добавить второе поле отдельным блоком. После блока `<div>...Цвет прогресса...</div>` (перед блоком «Кеш») вставить:

```jsx
        <div style={{borderTop:'1px solid #f1f5f9', paddingTop:16, marginTop:16}}>
          <div style={{fontSize:14, fontWeight:600, color: T.headingFg, marginBottom:8}}>💬 Мои решённые комментарии</div>
          <div style={{display:'flex', alignItems:'center', gap:8, marginBottom:6}}>
            <span style={labelStyle}>Сколько показывать (K):</span>
            <input
              type="number" min={1}
              value={cfg.resolved_comments_limit ?? 5}
              onChange={e => update('resolved_comments_limit', Number(e.target.value))}
              style={fieldStyle}
            />
          </div>
          <div style={hintStyle}>Сколько ваших последних решённых (не вами) комментариев показывать в колокольчике.</div>
        </div>
```

- [ ] **Step 3: Ручная проверка**

Запустить приложение (skill `run` или существующий способ запуска), открыть `/admin/health-checkin`. Ожидается: секция «💬 Комментарии» с тумблером «В счётчик» (выкл по умолчанию) и полем «Глубина команд»; отдельная секция «Мои решённые комментарии» с полем K=5. Сохранить → «Сохранено». Перезагрузить → значения сохранились.

- [ ] **Step 4: Commit**

```bash
git add internal/web/static/admin.js
git commit -m "feat(health-checkin): admin-настройки категории Комментарии"
```

---

## Task 7: Tracker UI — рендер категории comments + watermark-счётчик

**Files:**
- Modify: `internal/web/static/tracker.js` (`HCI_CAT_META`, `HCI_CAT_ORDER`, `HealthCheckInPanel`, `App` — badge/watermark, deep-link)

**Interfaces:**
- Consumes: `hciData.categories.comments.{unresolved,resolved}` (Task 5); существующий `<Markdown>`, `SidebarBell`, `onSelectTeam`.
- Produces: адаптированный бейдж `total_problems + unseenResolved`; watermark в `localStorage`.

- [ ] **Step 1: Зарегистрировать категорию comments в meta/order**

В `internal/web/static/tracker.js` найти `HCI_CAT_META` (строка ~1883) и `HCI_CAT_ORDER` (~1890). В `HCI_CAT_META` добавить запись:

```js
  comments: { icon: '💬', label: 'Комментарии', color: '#8b5cf6' },
```

В `HCI_CAT_ORDER` добавить `'comments'` в конец массива:

```js
const HCI_CAT_ORDER = ['stale', 'no_goals', 'awaiting_validation', 'formation_errors', 'lagging', 'comments'];
```

В `HCI_ACTION_LABEL` (объект рядом, строка ~1892) добавить:

```js
  comments: '→ Перейти к комментарию',
```

- [ ] **Step 2: Добавить helper подсчёта непросмотренных решённых (watermark)**

В `internal/web/static/tracker.js`, рядом с `HealthCheckInButton` добавить хелперы:

```js
function hciSeenKey(meId) { return `hci_resolved_seen_${meId || 'anon'}`; }

// Непросмотренные решённые = те, чей resolved_at строго новее сохранённого watermark.
function hciUnseenResolved(hciData, meId) {
  const resolved = hciData?.categories?.comments?.resolved || [];
  if (resolved.length === 0) return 0;
  const wm = localStorage.getItem(hciSeenKey(meId));
  const wmMs = wm ? new Date(wm).getTime() : 0;
  return resolved.filter(r => new Date(r.resolved_at).getTime() > wmMs).length;
}

// Двигает watermark на максимум resolved_at среди показанных решённых.
function hciMarkResolvedSeen(hciData, meId) {
  const resolved = hciData?.categories?.comments?.resolved || [];
  if (resolved.length === 0) return;
  const maxMs = Math.max(...resolved.map(r => new Date(r.resolved_at).getTime()));
  localStorage.setItem(hciSeenKey(meId), new Date(maxMs).toISOString());
}
```

- [ ] **Step 3: Скорректировать бейдж в App**

В `App` (строка ~2287) заменить рендер `SidebarBell`:

```jsx
        bell={hciData && hciData.has_scope
          ? <SidebarBell count={hciData.total_problems} onClick={() => setHciOpen(true)} />
          : null}
```

на:

```jsx
        bell={hciData && hciData.has_scope
          ? <SidebarBell count={hciData.total_problems + hciUnseenResolved(hciData, me?.id)} onClick={() => setHciOpen(true)} />
          : null}
```

- [ ] **Step 4: Двигать watermark при открытии панели**

В `App`, найти обработчик открытия колокольчика (`onClick={() => setHciOpen(true)}` в bell — уже изменён выше). Добавить эффект, который при открытии панели помечает решённые как просмотренные. Рядом с объявлением `const [hciOpen, setHciOpen] = useState(false);` (строка ~2046) добавить:

```jsx
  useEffect(() => {
    if (hciOpen && hciData) {
      hciMarkResolvedSeen(hciData, me?.id);
    }
  }, [hciOpen]);
```

> Watermark двигается один раз при открытии (зависимость только `hciOpen`). После закрытия/повторного открытия с теми же данными счётчик уже 0.

- [ ] **Step 5: Рендер секции comments в HealthCheckInPanel**

В `HealthCheckInPanel` (строка ~1923) секция рендерится циклом `visibleCats.map(k => {...})`, который ожидает `cat.items`. Категория `comments` имеет `unresolved`/`resolved` вместо `items` — добавить спец-ветку. Внутри `visibleCats.map`, в самом начале колбэка (после `const cat = data.categories?.[k];`) добавить обработку comments перед общей логикой:

```jsx
            visibleCats.map(k => {
              const cat = data.categories?.[k];
              if (!cat) return null;

              if (k === 'comments') {
                const unresolved = cat.unresolved || [];
                const resolved = cat.resolved || [];
                if (unresolved.length === 0 && resolved.length === 0) return null;
                const meta = HCI_CAT_META[k];
                const renderRow = (item, kind) => (
                  <div key={`${kind}-${item.comment_id}`} className="hci-item">
                    <div className="hci-item__title">{item.goal_title}</div>
                    <Markdown text={item.text} className="hci-item__comment" />
                    <div className="hci-item__meta">
                      {kind === 'unresolved'
                        ? (item.author_name || '')
                        : `решил: ${item.resolved_by_name || ''}`}
                      {' · '}{(item.team_path || []).join(' › ')}
                    </div>
                    <button className="hci-item__action"
                      onClick={() => { onSelectTeam(item.team_id, item.goal_id, item.comment_id); onClose(); }}>
                      {HCI_ACTION_LABEL[k]}
                    </button>
                  </div>
                );
                return (
                  <div key={k} className="hci-section">
                    <div className="hci-section__header" style={{ color: meta.color }}>
                      <span>{meta.icon}</span><span>{meta.label}</span>
                      <span className="hci-section__count">{unresolved.length + resolved.length}</span>
                    </div>
                    {unresolved.length > 0 && <>
                      <div className="hci-team__name"><span>▸</span><span>Нерешённые · {unresolved.length}</span></div>
                      {unresolved.map(it => renderRow(it, 'unresolved'))}
                    </>}
                    {resolved.length > 0 && <>
                      <div className="hci-team__name"><span>▸</span><span>Мои решённые · {resolved.length}</span></div>
                      {resolved.map(it => renderRow(it, 'resolved'))}
                    </>}
                  </div>
                );
              }

              if (cat.count === 0) return null;
```

> Обратите внимание: старую строку `if (!cat || cat.count === 0) return null;` заменяем на `if (!cat) return null;` в начале и `if (cat.count === 0) return null;` уже ПОСЛЕ ветки comments (как показано). Остальной существующий код секции (`const meta = ...; const byTeam = {}; ...`) остаётся без изменений ниже.

- [ ] **Step 6: Учесть comments в списке непустых категорий и в чипах**

В `HealthCheckInPanel` найти:

```jsx
  const nonEmptyCats = HCI_CAT_ORDER.filter(k => (data.categories?.[k]?.count ?? 0) > 0);
```

Заменить на (comments непуст, если есть unresolved или resolved):

```jsx
  const nonEmptyCats = HCI_CAT_ORDER.filter(k => {
    if (k === 'comments') {
      const c = data.categories?.comments;
      return (c?.unresolved?.length || 0) + (c?.resolved?.length || 0) > 0;
    }
    return (data.categories?.[k]?.count ?? 0) > 0;
  });
```

И в чипах (строка ~1969) счётчик чипа `· {cat.count}` для comments должен показывать сумму; заменить `{cat.count}` в рендере чипа на:

```jsx
                  {meta.icon} {meta.label} · {k === 'comments' ? ((cat.unresolved?.length || 0) + (cat.resolved?.length || 0)) : cat.count}
```

Также `visibleCats` (`filter ? [filter] : counterCats`) — чтобы comments-секция показывалась без фильтра, добавить её в базовый список видимых, когда она непуста. Заменить:

```jsx
  const visibleCats = filter ? [filter] : counterCats;
```

на:

```jsx
  const commentsNonEmpty = nonEmptyCats.includes('comments');
  const baseCats = commentsNonEmpty && !counterCats.includes('comments')
    ? [...counterCats, 'comments'] : counterCats;
  const visibleCats = filter ? [filter] : baseCats;
```

> Это гарантирует, что секция «Комментарии» видна по умолчанию (она не в счётчике при `in_counter.comments=false`, но должна отображаться в панели).

- [ ] **Step 7: Deep-link на комментарий**

В `App`, обработчик `onSelectTeam` (строка ~2399) сейчас `(teamId, goalId) => {...}`. Расширить до `(teamId, goalId, commentId)`:

```jsx
        onSelectTeam={(teamId, goalId, commentId) => {
          selectTeam(teamId);
          if (goalId) {
            setTimeout(() => {
              const target = commentId
                ? document.getElementById(`comment-${commentId}`)
                : document.getElementById(`goal-${goalId}`);
              const el = target || document.getElementById(`goal-${goalId}`);
              if (el) el.scrollIntoView({ behavior: 'smooth', block: 'center' });
            }, 400);
          }
        }}
```

> `comment-${id}` — существующий id элемента комментария в `GoalCard`/`CommentsPanel`. Если комментарий свёрнут (секция закрыта), fallback на `goal-${goalId}`.

- [ ] **Step 8: Добавить минимальный стиль для текста комментария (опционально)**

В `internal/web/static/tracker.css` добавить:

```css
.hci-item__comment { font-size: 12px; color: #475569; margin: 4px 0; line-height: 1.4; }
.hci-item__comment p { margin: 0; }
```

- [ ] **Step 9: Ручная проверка**

Запустить приложение. Предусловие: у текущего пользователя есть lead/owner команды с целями, к которым есть нерешённые комментарии, и есть его комментарии, решённые другим пользователем (использовать seed/demo или создать вручную).

Проверить:
1. В админке включить `in_counter.comments` → бейдж колокольчика увеличивается на число нерешённых.
2. Открыть панель → секция «💬 Комментарии» с под-списками «Нерешённые» и «Мои решённые», текст рендерится как markdown.
3. Бейдж включает непросмотренные решённые; после открытия панели и перезагрузки страницы счётчик решённых обнулился (watermark в localStorage).
4. Клик по элементу → переход к команде и прокрутка к цели/комментарию.
5. `comment_depth = 0` в админке → показываются только комментарии моих команд, без дочерних.

- [ ] **Step 10: Commit**

```bash
git add internal/web/static/tracker.js internal/web/static/tracker.css
git commit -m "feat(health-checkin): секция Комментарии в трекере + watermark-счётчик"
```

---

## Task 8: Финальная проверка и синхронизация spec/seed

**Files:**
- Verify only; при необходимости — правки спек.

- [ ] **Step 1: Полный прогон тестов и vet**

Run: `go test ./... && go vet ./...`
Expected: всё зелёное.

- [ ] **Step 2: Проверить актуальность спек**

Убедиться, что обновлены: `specs/040-api-contract.md` (stale-правило + форма ответа comments + тело POST settings), `specs/020-domain-model.md` (поля `health_checkin_config`). Seed/demo не трогается (структура таблиц не менялась) — подтвердить, что новых колонок нет.

- [ ] **Step 3: Ручная проверка Части 1 (stale) в трекере**

Открыть трекер с командой в статусе «В работе», где есть цель без обновлений прогресса и период начался недавно (< порога): убедиться, что цель НЕ в «Нет обновлений» колокольчика; для периода, начавшегося давно (> порога), — в категории с корректным числом дней от начала периода.

- [ ] **Step 4: Commit (если были правки спек)**

```bash
git add specs/
git commit -m "docs(health-checkin): синхронизация спек по stale и категории Комментарии"
```

---

## Self-Review чеклист (выполнен при написании плана)

- **Покрытие спеки:** Часть 1 (stale от начала периода) → Task 1; конфиг-поля → Task 2; догрузка комментариев → Task 3; scope глубины → Task 4; категория comments/unresolved/resolved/self-resolve/limit → Task 5; admin UI → Task 6; frontend рендер+watermark+deep-link → Task 7; спеки → Tasks 1/2/5/8. Все разделы дизайна покрыты.
- **Типы согласованы:** `computeCategories(data, scopeIDs, userUDID, cfg, now)` — новая сигнатура зафиксирована в Task 5 и все вызовы (включая тесты и `GetHealthCheckIn`) обновляются там же. `HealthCheckInCommentItem`, `HealthCheckInCategory.Unresolved/Resolved`, `computeCommentScope(... ) map[int64]struct{}`, `ListGoalCommentsByGoals(...)` — определены до использования.
- **Watermark:** сервер отдаёт `resolved` без seen-состояния; клиент считает непросмотренные и двигает watermark при открытии панели (Task 7 steps 2-4). Бейдж = `total_problems + unseenResolved`.
- **Плейсхолдеров нет:** каждый шаг содержит конкретный код/команду. Store-тест (Task 3) явно требует переиспользовать фактические helper'ы из `goals_comments_test.go` — свериться с файлом перед написанием.
