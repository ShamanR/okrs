# Критерий обнуления для всех типов KR — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Сделать критерий обнуления (`zeroing_criteria`) доступным для всех типов Key Result (NUMERICAL, BOOLEAN, PROJECT), не затрагивая существующие данные.

**Architecture:** Поле `zeroing_criteria` — generic-свойство самого KR, а DB-колонка `key_results.zeroing_criteria` уже существует для всех строк. Поднимаем поле из measure (`KRNumerical`/`NumericalMeasure`) на уровень `KeyResult`. Пишем его при базовой записи KR (INSERT/UPDATE `key_results`), читаем для всех kind. Миграций БД нет.

**Tech Stack:** Go (chi, pgx), Postgres, серверный React без сборки (`internal/web/static/tracker.js`, JSX через runtime Babel).

## Global Constraints

- **Без git-коммитов.** Согласно CLAUDE.md коммиты делает пользователь вручную. Каждая задача завершается прогоном `go build ./...`, `go vet ./...` и релевантных тестов — НЕ командой `git commit`.
- Спеки и дизайн-доки — на русском (CLAUDE.md).
- Чистая слоёвая архитектура: не протекать абстракциями между слоями (CLAUDE.md).
- Существующие критерии обнуления числовых KR не должны пострадать: колонка и её содержимое не мигрируются.
- Store-тесты используют `testutil.SetupDB` (Postgres testcontainer). Без Docker тест сам делает `t.Skip` — это ожидаемо.
- Единственный клиент API — `internal/web/static/tracker.js`; смена формы JSON и имени form-поля обновляется в этом же наборе изменений.

---

### Task 1: Перенести ZeroingCriteria на уровень KeyResult (backend)

Это атомарная задача: перенос поля связывает domain → dto → store → service → handlers → api-response. Проект не компилируется, пока все правки не согласованы, поэтому они входят в одну задачу с единым green-билдом в конце.

**Files:**
- Modify: `internal/domain/models.go` (`KeyResult`, `KRNumerical`)
- Modify: `internal/http/dto/kr.go` (`KeyResult`, `NumericalMeasure`)
- Modify: `internal/store/krs/krs.go` (`KeyResultInput`, `KeyResultUpdateInput`, `NumericalMetaInput`, `scanNumerical`, `CreateKeyResult`, `UpdateKeyResult`, `UpsertNumericalMeta`, `ListKeyResultsByGoal`, `GetKeyResult`)
- Modify: `internal/store/goals/goals.go` (два скан-блока KR)
- Modify: `internal/service/service.go` (`KeyResultMetaInput`, `applyKeyResultMeta`)
- Modify: `internal/http/handlers/web/common/common.go` (`ParseNumericalMeta`)
- Modify: `internal/http/handlers/api/v1/krs/handler.go` (create + update)
- Modify: `internal/http/handlers/web/goals/handler.go` (create)
- Modify: `internal/http/handlers/web/keyresults/handler.go` (update)
- Modify: `internal/http/handlers/api/v1/helpers_response.go` (`MapKeyResult`, `buildMeasure`)
- Test: `internal/http/handlers/api/v1/helpers_response_test.go`
- Test: `internal/store/krs/krs_test.go`

**Interfaces:**
- Produces: `domain.KeyResult.ZeroingCriteria string`; `dto.KeyResult.ZeroingCriteria string` (json `zeroing_criteria,omitempty`); `krs.KeyResultInput.ZeroingCriteria string`; `krs.KeyResultUpdateInput.ZeroingCriteria string`.
- Removes: `domain.KRNumerical.ZeroingCriteria`; `dto.NumericalMeasure.ZeroingCriteria`; `krs.NumericalMetaInput.ZeroingCriteria`; `service.KeyResultMetaInput.ZeroingCriteria`.
- Form-поле переименовано: `numerical_zeroing` → `zeroing_criteria` (парсится для всех kind).

- [ ] **Step 1: domain — перенести поле**

В `internal/domain/models.go` в структуре `KeyResult` добавить поле после `Description`:

```go
	Description       string
	ZeroingCriteria   string
```

И удалить строку `ZeroingCriteria string` из структуры `KRNumerical` (оставить `Unit`, `Checkpoints` и т.д.).

- [ ] **Step 2: dto — перенести поле**

В `internal/http/dto/kr.go` удалить из `NumericalMeasure` строку:

```go
	ZeroingCriteria string                `json:"zeroing_criteria,omitempty"`
```

И добавить в `KeyResult` после `Description`:

```go
	Description string    `json:"description"`
	ZeroingCriteria string `json:"zeroing_criteria,omitempty"`
```

- [ ] **Step 3: store — типы ввода и scanNumerical**

В `internal/store/krs/krs.go`:

Добавить поле в `KeyResultInput`:

```go
type KeyResultInput struct {
	GoalID          int64
	Title           string
	Description     string
	ZeroingCriteria string
	Weight          int
	Kind            domain.KRKind
}
```

Добавить поле в `KeyResultUpdateInput`:

```go
type KeyResultUpdateInput struct {
	ID              int64
	Title           string
	Description     string
	ZeroingCriteria string
	Weight          int
	Kind            domain.KRKind
}
```

Удалить `ZeroingCriteria string` из `NumericalMetaInput`.

Заменить сигнатуру и тело `scanNumerical` — убрать параметр `zeroing`:

```go
func scanNumerical(start, target, current *float64, unit *string, checkpointsRaw []byte) (*domain.KRNumerical, error) {
	num := &domain.KRNumerical{}
	if start != nil {
		num.StartValue = *start
	}
	if target != nil {
		num.TargetValue = *target
	}
	if current != nil {
		num.CurrentValue = *current
	}
	if unit != nil {
		num.Unit = *unit
	}
	cps, err := ParseCheckpoints(checkpointsRaw)
	if err != nil {
		return nil, err
	}
	num.Checkpoints = cps
	return num, nil
}
```

- [ ] **Step 4: store — запись при CreateKeyResult / UpdateKeyResult**

В `CreateKeyResult` добавить `zeroing_criteria` в INSERT:

```go
	err := r.db.QueryRow(ctx, `
		INSERT INTO key_results (goal_id, title, description, zeroing_criteria, weight, kind, sort_order, tenant_id)
		VALUES ($1,$2,$3,$4,$5,$6, (SELECT COALESCE(MAX(sort_order), 0) + 1 FROM key_results WHERE goal_id=$1 AND tenant_id=$7), $7)
		RETURNING id`,
		input.GoalID, input.Title, input.Description, input.ZeroingCriteria, input.Weight, input.Kind, scope.TenantID,
	).Scan(&id)
```

В `UpdateKeyResult` добавить `zeroing_criteria` в UPDATE:

```go
	_, err := r.db.Exec(ctx, `
		UPDATE key_results
		SET title=$1, description=$2, zeroing_criteria=$3, weight=$4, kind=$5, updated_at=NOW()
		WHERE id=$6 AND tenant_id=$7`,
		input.Title, input.Description, input.ZeroingCriteria, input.Weight, input.Kind, input.ID, scope.TenantID,
	)
```

- [ ] **Step 5: store — UpsertNumericalMeta больше не пишет zeroing**

В `UpsertNumericalMeta` убрать `zeroing_criteria` из UPDATE:

```go
	_, err := r.db.Exec(ctx, `
		UPDATE key_results
		SET start_value=$1, target_value=$2, current_value=$3, unit=$4,
		    checkpoints=$5, updated_at=NOW()
		WHERE id=$6 AND tenant_id=$7`,
		input.StartValue, input.TargetValue, input.CurrentValue, input.Unit,
		checkpointsJSON, input.KeyResultID, scope.TenantID,
	)
```

- [ ] **Step 6: store — чтение zeroing для всех kind (krs.go)**

В `ListKeyResultsByGoal` и `GetKeyResult`: после `rows.Scan(...)` / `row.Scan(...)` (в обоих `zeroing` уже сканируется в `*string`) присвоить поле KR ДО ветки по kind и убрать `zeroing` из вызова `scanNumerical`.

В `ListKeyResultsByGoal` внутри цикла:

```go
		kr.ZeroingCriteria = derefString(zeroing)
		if kr.Kind == domain.KRKindNumerical {
			num, err := scanNumerical(startValue, targetValue, currentValue, unit, checkpointsRaw)
			if err != nil {
				return nil, err
			}
			kr.Numerical = num
		}
```

Если в пакете `krs` нет хелпера `derefString`, заменить на инлайн:

```go
		if zeroing != nil {
			kr.ZeroingCriteria = *zeroing
		}
```

Проверить наличие хелпера: `rg -n "func derefString" internal/store/krs`. В `krs.go` — использовать тот вариант, который компилируется (инлайн-проверка `if zeroing != nil` безопасна всегда).

В `GetKeyResult` аналогично:

```go
	if zeroing != nil {
		kr.ZeroingCriteria = *zeroing
	}
	if kr.Kind == domain.KRKindNumerical {
		num, err := scanNumerical(startValue, targetValue, currentValue, unit, checkpointsRaw)
		if err != nil {
			return domain.KeyResult{}, err
		}
		kr.Numerical = num
	}
```

- [ ] **Step 7: store — чтение zeroing для всех kind (goals.go)**

В `internal/store/goals/goals.go` в обоих скан-блоках (около строк 178 и 440):

После `Scan(...)` добавить присвоение до ветки numerical и убрать `ZeroingCriteria:` из литерала `&domain.KRNumerical{...}`:

```go
		if zeroing != nil {
			kr.ZeroingCriteria = *zeroing
		}
		if kr.Kind == domain.KRKindNumerical {
			num, err := krs.ParseCheckpoints(checkpointsRaw)
			if err != nil {
				return nil, err
			}
			kr.Numerical = &domain.KRNumerical{Unit: derefString(unit), Checkpoints: num}
			...
		}
```

Во втором блоке переменная чекпоинтов называется `cps` — сохранить локальное имя как в исходнике. `derefString` в пакете `goals` уже используется (см. `derefString(unit)`), поэтому здесь можно `kr.ZeroingCriteria = derefString(zeroing)`.

- [ ] **Step 8: service — убрать zeroing из meta**

В `internal/service/service.go`:

Удалить поле `ZeroingCriteria string` из `KeyResultMetaInput`.

В `applyKeyResultMeta`, ветка `KRKindNumerical`, убрать строку `ZeroingCriteria: meta.ZeroingCriteria,` из `krs.NumericalMetaInput{...}`.

- [ ] **Step 9: common — убрать zeroing из ParseNumericalMeta**

В `internal/http/handlers/web/common/common.go`, в `ParseNumericalMeta`, удалить строку:

```go
		ZeroingCriteria:      strings.TrimSpace(r.FormValue("numerical_zeroing")),
```

Обновить doc-комментарий функции: убрать упоминание «optional zeroing criteria».

- [ ] **Step 10: handlers — парсить zeroing_criteria в базовый инпут**

Добавить `ZeroingCriteria: common.TrimmedFormValue(r, "zeroing_criteria"),` в каждый из четырёх литералов:

`internal/http/handlers/api/v1/krs/handler.go` — в `CreateKeyResultWithMeta`:

```go
	krID, err := h.service.CreateKeyResultWithMeta(r.Context(), scope, krs.KeyResultInput{
		GoalID:          goalID,
		Title:           common.TrimmedFormValue(r, "title"),
		Description:     common.TrimmedFormValue(r, "description"),
		ZeroingCriteria: common.TrimmedFormValue(r, "zeroing_criteria"),
		Weight:          weight,
		Kind:            kind,
	}, meta)
```

`internal/http/handlers/api/v1/krs/handler.go` — в `UpdateKeyResultWithMeta`:

```go
	if err := h.service.UpdateKeyResultWithMeta(r.Context(), scope, krs.KeyResultUpdateInput{
		ID:              krID,
		Title:           common.TrimmedFormValue(r, "title"),
		Description:     common.TrimmedFormValue(r, "description"),
		ZeroingCriteria: common.TrimmedFormValue(r, "zeroing_criteria"),
		Weight:          weight,
		Kind:            kind,
	}, meta); err != nil {
```

`internal/http/handlers/web/goals/handler.go` — в `CreateKeyResultWithMeta` (около строки 109): добавить `ZeroingCriteria: common.TrimmedFormValue(r, "zeroing_criteria"),` в литерал `krs.KeyResultInput{...}`.

`internal/http/handlers/web/keyresults/handler.go` — в `UpdateKeyResultWithMeta` (около строки 75): добавить `ZeroingCriteria: common.TrimmedFormValue(r, "zeroing_criteria"),` в литерал `krs.KeyResultUpdateInput{...}`.

- [ ] **Step 11: api-response — top-level zeroing**

В `internal/http/handlers/api/v1/helpers_response.go`:

В `buildMeasure`, ветка numerical, удалить строку `ZeroingCriteria: kr.Numerical.ZeroingCriteria,` из `&dto.NumericalMeasure{...}`.

В `MapKeyResult`, в литерал `dto.KeyResult{...}` добавить после `Description`:

```go
		Description:     kr.Description,
		ZeroingCriteria: kr.ZeroingCriteria,
```

- [ ] **Step 12: обновить unit-тест ответа под новую форму**

В `internal/http/handlers/api/v1/helpers_response_test.go`:

В `TestBuildMeasureNumerical` удалить из литерала `KRNumerical{...}` строку `ZeroingCriteria: "падение сервиса = 0%",` и удалить блок ассерта:

```go
	if measure.Numerical.ZeroingCriteria == "" {
		t.Fatalf("expected zeroing criteria")
	}
```

Добавить новый тест, проверяющий top-level проксирование для всех kind:

```go
func TestMapKeyResultZeroingTopLevel(t *testing.T) {
	for _, kind := range []domain.KRKind{domain.KRKindNumerical, domain.KRKindBoolean, domain.KRKindProject} {
		kr := domain.KeyResult{Kind: kind, ZeroingCriteria: "падение сервиса = 0%"}
		switch kind {
		case domain.KRKindNumerical:
			kr.Numerical = &domain.KRNumerical{Unit: "%"}
		case domain.KRKindBoolean:
			kr.Boolean = &domain.KRBoolean{}
		case domain.KRKindProject:
			kr.Project = &domain.KRProject{}
		}
		dtoKR := MapKeyResult(kr)
		if dtoKR.ZeroingCriteria != "падение сервиса = 0%" {
			t.Fatalf("kind %s: expected top-level zeroing, got %q", kind, dtoKR.ZeroingCriteria)
		}
	}
}
```

- [ ] **Step 13: обновить store-тест numerical и добавить тест для всех kind**

В `internal/store/krs/krs_test.go`:

В `TestUpsertAndLoadNumericalMeta` удалить из `krs.NumericalMetaInput{...}` строку `ZeroingCriteria: "сервис падает = 0%",`.

Заменить блок ассерта

```go
	if num.Unit != "RPS" || num.ZeroingCriteria != "сервис падает = 0%" {
		t.Fatalf("unexpected unit/zeroing: %+v", num)
	}
```
на
```go
	if num.Unit != "RPS" {
		t.Fatalf("unexpected unit: %+v", num)
	}
```

Добавить новый тест round-trip zeroing на уровне KR для BOOLEAN и PROJECT:

```go
func TestZeroingCriteriaAllKinds(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := krs.NewKRRepository(pool)
	scope := domain.TenantScope{TenantID: 1}

	var teamID, periodID, goalID int64
	pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('T') RETURNING id`).Scan(&teamID)
	pool.QueryRow(ctx, `INSERT INTO periods (name, start_date, end_date) VALUES ('Q1','2024-01-01','2024-03-31') RETURNING id`).Scan(&periodID)
	pool.QueryRow(ctx, `INSERT INTO goals (team_id, period_id, title, priority, weight, work_type, focus_type, sort_order) VALUES ($1,$2,'G','P1',100,'Delivery','STABILITY',1) RETURNING id`, teamID, periodID).Scan(&goalID)

	for _, kind := range []domain.KRKind{domain.KRKindBoolean, domain.KRKindProject} {
		krID, err := repo.CreateKeyResult(ctx, scope, krs.KeyResultInput{
			GoalID:          goalID,
			Title:           "KR " + string(kind),
			ZeroingCriteria: "инцидент P1 = 0%",
			Weight:          10,
			Kind:            kind,
		})
		if err != nil {
			t.Fatalf("create %s: %v", kind, err)
		}
		got, err := repo.GetKeyResult(ctx, scope, krID)
		if err != nil {
			t.Fatalf("get %s: %v", kind, err)
		}
		if got.ZeroingCriteria != "инцидент P1 = 0%" {
			t.Fatalf("kind %s: expected zeroing round-trip, got %q", kind, got.ZeroingCriteria)
		}
	}

	// Update path also persists zeroing.
	krID, _ := repo.CreateKeyResult(ctx, scope, krs.KeyResultInput{GoalID: goalID, Title: "U", Weight: 10, Kind: domain.KRKindBoolean})
	if err := repo.UpdateKeyResult(ctx, scope, krs.KeyResultUpdateInput{ID: krID, Title: "U", ZeroingCriteria: "новый критерий", Weight: 10, Kind: domain.KRKindBoolean}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := repo.GetKeyResult(ctx, scope, krID)
	if got.ZeroingCriteria != "новый критерий" {
		t.Fatalf("expected updated zeroing, got %q", got.ZeroingCriteria)
	}
}
```

- [ ] **Step 14: собрать и проверить весь backend**

Run:
```bash
go build ./... && go vet ./...
```
Expected: без ошибок компиляции и vet.

Run:
```bash
go test ./internal/http/handlers/api/v1/... -run 'TestBuildMeasure|TestMapKeyResultZeroingTopLevel' -v
```
Expected: PASS (эти тесты без БД).

Run:
```bash
go test ./internal/store/krs/... -run 'TestUpsertAndLoadNumericalMeta|TestZeroingCriteriaAllKinds' -v
```
Expected: PASS, либо SKIP с «docker unavailable», если Docker недоступен.

- [ ] **Step 15: убедиться, что не осталось старого имени поля/формы**

Run:
```bash
rg -n "numerical_zeroing|Numerical\.ZeroingCriteria|NumericalMeasure\{[^}]*Zeroing|meta\.ZeroingCriteria" internal/ ; echo "exit: $?"
```
Expected: пусто (exit 1 у `rg` = нет совпадений). Любое совпадение — недоделанная правка, исправить.

---

### Task 2: Вынести UI критерия обнуления на уровень формы KR (frontend)

**Files:**
- Modify: `internal/web/static/tracker.js` (`mapKR`, KR-модалка `save`, JSX-блок zeroing)

**Interfaces:**
- Consumes: API теперь отдаёт `kr.zeroing_criteria` на top-level (Task 1, Step 11) и принимает form-поле `zeroing_criteria` для любого kind (Task 1, Step 10).

- [ ] **Step 1: mapKR читает top-level zeroing**

В `internal/web/static/tracker.js`, функция `mapKR`: удалить строку `zeroing = m.numerical.zeroing_criteria || '';` из блока `if (m.numerical) {...}` и задать `zeroing` из top-level. Изменить инициализацию (около строки 147):

```js
  let unit = '%', checkpoints = [];
  let zeroing = kr.zeroing_criteria || '';
```

Внутри `if (m.numerical) { ... }` оставить только `start/target/current/unit/checkpoints` (строку с `zeroing` убрать).

- [ ] **Step 2: save отправляет zeroing_criteria для всех типов**

В функции `save` KR-модалки: удалить строку `fd.append('numerical_zeroing', form.zeroing || '');` из блока `if (form.krType === 'NUMERICAL')` и добавить общий append сразу после `fd.append('kind', form.krType);`:

```js
      fd.append('kind', form.krType);
      fd.append('zeroing_criteria', form.zeroing || '');
```

- [ ] **Step 3: перенести JSX-блок zeroing из секции NUMERICAL в общую часть**

Удалить из секции `form.krType === 'NUMERICAL'` блок (kr-section-sep + showZeroing? textarea : button), т.е. текущие строки:

```jsx
              <div className="kr-section-sep" />
              {showZeroing ? (
                <div className="kr-num-field">
                  <div className="kr-section-head"><span className="kr-section-head__title">Критерий обнуления</span></div>
                  <textarea value={form.zeroing || ''} onChange={e => set('zeroing', e.target.value)} rows={2}
                    className="form-textarea form-textarea--sm" style={{ resize: 'vertical' }} autoFocus />
                </div>
              ) : (
                <button type="button" onClick={() => setShowZeroing(true)} className="kr-zeroing-btn">
                  <span className="kr-zeroing-btn__icon">⊘</span> Критерий обнуления
                </button>
              )}
```

Вставить его в общую часть формы — сразу после закрытия блока `{form.krType === 'PROJECT' && ( ... )}` и перед закрывающим `</div>` элемента `modal-body`:

```jsx
          {form.krType === 'PROJECT' && (
            ...
          )}
          <div className="kr-section-sep" />
          {showZeroing ? (
            <div className="kr-num-field">
              <div className="kr-section-head"><span className="kr-section-head__title">Критерий обнуления</span></div>
              <textarea value={form.zeroing || ''} onChange={e => set('zeroing', e.target.value)} rows={2}
                className="form-textarea form-textarea--sm" style={{ resize: 'vertical' }} autoFocus />
            </div>
          ) : (
            <button type="button" onClick={() => setShowZeroing(true)} className="kr-zeroing-btn">
              <span className="kr-zeroing-btn__icon">⊘</span> Критерий обнуления
            </button>
          )}
        </div>
```

Состояние `showZeroing`, `form.zeroing`, `set('zeroing', ...)` и инициализатор `useState(!!(kr && kr.zeroing))` не меняются.

- [ ] **Step 4: проверить руками в приложении**

Запустить приложение согласно проектному способу (см. skill `run` / README). Открыть форму KR, переключить тип на BOOLEAN и на PROJECT — блок «⊘ Критерий обнуления» виден и сохраняется. Открыть числовой KR со старым критерием — значение подгружается и сохраняется. Проверить, что после сохранения `GET` KR возвращает `zeroing_criteria` на top-level (DevTools → Network).

Note: если автоматический прогон недоступен, зафиксировать это и оставить проверку ревьюеру.

---

### Task 3: Обновить спеки и seed

**Files:**
- Modify: `specs/020-domain-model.md`
- Modify: `specs/040-api-contract.md`
- Modify: `seed_demo.sql`

- [ ] **Step 1: 020-domain-model.md**

Строку про `zeroing_criteria` (сейчас в описании числового measure) переформулировать так, чтобы поле относилось к KR любого типа. Пример замены:

```
- zeroing_criteria — опциональный текстовый критерий обнуления уровня KR (для любого типа: NUMERICAL, BOOLEAN, PROJECT), человекочитаемый, в расчётах не применяется
```

Убедиться, что поле упоминается в разделе общих атрибутов KR, а не только в подсекции NUMERICAL.

- [ ] **Step 2: 040-api-contract.md**

В строке 541 (описание measure) убрать `zeroing_criteria` из перечня полей `measure.numerical` и добавить `zeroing_criteria` в общие поля объекта KR (рядом с `title`, `description`, `weight`, `kind`).

В строке 547 (Create/Update KR) заменить упоминание `numerical_zeroing` (только для NUMERICAL) на общий параметр: `zeroing_criteria` — опциональный текстовый критерий обнуления, принимается для всех `kind`.

- [ ] **Step 3: seed_demo.sql**

Оставить существующий `UPDATE key_results SET zeroing_criteria = '...' WHERE id = 100;` (числовой KR).

Добавить демонстрационный критерий на один не-числовой KR. Найти подходящий id BOOLEAN/PROJECT KR в seed:

```bash
rg -n "INSERT INTO key_results" seed_demo.sql | head
```

Затем добавить рядом с существующим UPDATE строку (подставив реальный id BOOLEAN/PROJECT KR из seed):

```sql
UPDATE key_results SET zeroing_criteria = 'Если фича выкачена, но откачена из-за инцидента — результат не засчитывается.' WHERE id = <BOOLEAN_OR_PROJECT_KR_ID>;
```

- [ ] **Step 4: финальная проверка**

Run:
```bash
go build ./... && go vet ./...
```
Expected: без ошибок.

Run:
```bash
rg -n "zeroing" specs/020-domain-model.md specs/040-api-contract.md
```
Expected: описания отражают KR-level поле и form-параметр `zeroing_criteria`; нет упоминаний `numerical_zeroing`.

---

## Self-Review

**Spec coverage:**
- «Поле на уровне KR» → Task 1 (domain/dto/store/service/handlers/response). ✓
- «Доступно для всех типов в UI» → Task 2. ✓
- «Существующие критерии не страдают» → колонка не мигрируется; Step 6/7 читают для всех kind; Step 4 пишет при базовой записи; тесты Step 13 проверяют round-trip и сохранность numerical. ✓
- «Спеки/seed» → Task 3. ✓

**Placeholder scan:** TODO/TBD/«implement later» отсутствуют; все шаги с кодом содержат конкретный код.

**Type consistency:** `ZeroingCriteria` — единое имя во всех Go-структурах; json/form-ключ — `zeroing_criteria` (совпадает в dto, tracker.js `mapKR` и `save`, handlers). `MapKeyResult` (экспортируемая) — используется в тесте Step 12. `CreateKeyResult`/`UpdateKeyResult`/`GetKeyResult` — существующие сигнатуры, тест Step 13 им соответствует.
