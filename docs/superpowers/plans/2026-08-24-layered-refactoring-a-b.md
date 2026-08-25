# Слоистый рефакторинг, этапы A и B — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Перенести статический контент в `/web/{templates,static}` и убрать из корня `/internal` пакеты `domain`, `okr`, `entitlements`, `export`, `onboarding`, разложив их по тематическим группам `core/`, `platform/`, `render/`.

**Architecture:** Оба этапа — чисто механические перемещения без изменения логики. Этап A создаёт публичный пакет `web` с якорем `//go:embed` (директива не умеет выходить за пределы каталога своего пакета) и переключает отдачу статики на новый путь. Этап B меняет import paths и два имени пакетов (`okr` → `progress`, `onboarding` → `nomembership`). Внешний HTTP-контракт не затрагивается ни в одном шаге.

**Tech Stack:** Go 1.25, chi/v5, `html/template` + `//go:embed`, статика с диска без бандлера, testcontainers для интеграционных тестов.

**Spec:** `docs/superpowers/specs/2026-08-24-layered-refactoring-design.md`

## Global Constraints

- **Не делать `git commit`** — заказчик коммитит сам (CLAUDE.md #8). Каждая задача заканчивается прогоном тестов, а не коммитом.
- **Перемещать файлы через `git mv`**, а не `mv` + `git add` — сохраняется история.
- **Baseline (снят 2026-08-24):** `go test ./...` → exit 0; 55 пакетов, 46 с тестами все `ok`, 9 без тестов, 0 `FAIL`. Любая задача считается принятой только при этом же результате.
- **Интеграционные тесты требуют работающего Docker** (testcontainers). Перед прогоном: `docker info`.
- **Внешний контракт неизменен.** Ни один URI, HTTP-метод, статус-код или формат тела не меняется ни в одной задаче этого плана.
- **⚠ Правки «на месте» — `perl -pi -e`, якорить по `\b`, НЕ по `$`.** Рабочее дерево в CRLF (`core.autocrlf=true`), и это даёт три ловушки с *тихим* отказом:
  1. **Якорь `$` не совпадает ни в sed, ни в perl** — перед `\n` стоит `\r`. Проверено: `printf 'package okr\r\n' | perl -pe 's/^package okr$/X/'` возвращает строку без изменений. Вариант `\r?$` работает, но съедает `\r`; `\b` безопаснее.
  2. **BSD sed не поддерживает `\b`** — `sed 's/\bfoo\./bar./g'` молчаливый no-op. Perl поддерживает.
  3. **`perl -pi -e '... print'` дублирует файл** — `-p` печатает сам, явный `print` даёт вторую копию каждой строки. При равномерном дублировании восстанавливается через `awk 'NR%2==1'`, но сначала проверить парность.
- **⚠ НИКОГДА не запускать `gofmt -w` по каталогам.** В репозитории задан `core.autocrlf=true`, поэтому рабочее дерево выложено с CRLF, а `gofmt` ожидает LF. Из-за этого `gofmt -l ./internal ./app ./cmd ./web` перечисляет **211 файлов** — почти весь репозиторий, — и это ничего не говорит о форматировании. `gofmt -w` переписал бы все 211 на LF и утопил бы диффы рефакторинга. Реально не соответствуют gofmt только 5 файлов, и все они такими были до начала работ: `internal/http/handlers/api/v1/admin/handler_test.go`, `internal/http/handlers/api/v1/goals/handler.go`, `internal/http/handlers/web/authhandler/handler.go`, `internal/okr/okr_test.go`, `internal/store/teams/teams_test.go` (лишняя пустая строка / порядок импортов). Приводить их в порядок в рамках этого плана не нужно — это unrelated churn.
- **После правки импортов прогонять `gofmt -w` по каждому изменённому файлу отдельно** — смена import path меняет порядок сортировки внутри группы импортов, и файл перестаёт соответствовать gofmt. По одному файлу это безопасно (см. пункт выше).
- **Проверка форматирования** — только по изменённым файлам и без учёта переводов строк:

```bash
# fmtcheck <file>... — печатает BAD только при реальном расхождении с gofmt
fmtcheck() {
  for f in "$@"; do
    if diff -q <(tr -d '\r' < "$f") <(tr -d '\r' < "$f" | gofmt) >/dev/null 2>&1
      then echo "  OK   $f"; else echo "  BAD  $f"; fi
  done
}
```

  Если файл помечен `BAD` — исправлять `gofmt -w <файл>` строго по одному файлу. Это безопасно: пострадает только он. Опасен именно проход по каталогу (`gofmt -w ./internal`), который переписал бы все 211.
- **`docs/superpowers/**` не трогать.** Там 55 файлов со ссылками на `internal/web` и `internal/http/templates` — это исторические документы, описывающие состояние на момент их написания. Переписывать их — фальсификация истории. Живых ссылок на эти пути в коде ровно две: `Dockerfile:21` и `internal/http/server.go:325`.
- **Язык спек и дизайн-доков — русский** (CLAUDE.md #11).

---

# Этап A — статика в `/web`

### Task 1: Пакет `web` и перенос шаблонов

**Files:**
- Create: `web/web.go`
- Move: `internal/http/templates/*.html` (11 файлов) → `web/templates/`
- Modify: `internal/http/server.go:1-50` (импорты, `templatesFS`, `parseTemplates`)

**Interfaces:**
- Produces: `web.TemplatesFS` (`embed.FS`) — содержит `templates/*.html`. Потребляется `internal/http.parseTemplates` в этой же задаче и больше нигде.

- [x] **Step 1: Перенести каталог шаблонов**

```bash
mkdir -p web
git mv internal/http/templates web/templates
ls web/templates | wc -l   # ожидается 11
```

- [x] **Step 2: Создать якорь embed**

Create `web/web.go`:

```go
// Package web embeds the SSR shell templates. Static assets (/web/static) are
// served from disk and are deliberately NOT embedded, so that JS/CSS edits are
// visible on page reload without rebuilding the binary.
//
// This package exists because //go:embed cannot reference paths outside its own
// package directory: an embed directive in internal/http could not reach
// /web/templates. It is one of exactly two public packages in this module (the
// other is app) and must stay free of logic — only the FS.
package web

import "embed"

//go:embed templates/*.html
var TemplatesFS embed.FS
```

- [x] **Step 3: Переключить `parseTemplates` на новый FS**

В `internal/http/server.go` удалить блок:

```go
//go:embed templates/*.html
var templatesFS embed.FS

func parseTemplates() (*template.Template, error) {
	return template.New("").ParseFS(templatesFS, "templates/*.html")
}
```

и заменить на:

```go
func parseTemplates() (*template.Template, error) {
	return template.New("").ParseFS(web.TemplatesFS, "templates/*.html")
}
```

В блоке импортов `internal/http/server.go` удалить строку `"embed"` и добавить `"okrs/web"` (отдельной группой после `okrs/internal/...`, так как это не internal-пакет).

- [x] **Step 4: Проверить сборку и тесты шаблонов**

```bash
go build ./... && go vet ./internal/http/... && go test ./internal/http/...
```

Ожидается: `ok okrs/internal/http`. Тест `TestShellsReferenceTheirEntrypoint` в `internal/http/templates_test.go:81` обращается к `../web/static/period_url.js` — на этом шаге путь ещё указывает на `internal/web/static`, который пока существует, поэтому тест зелёный. Он чинится в Task 2.

- [x] **Step 5: Полный прогон**

```bash
docker info > /dev/null && go test ./...
```

Ожидается: exit 0, 0 FAIL.

---

### Task 2: Перенос статики и путей отдачи

**Files:**
- Move: `internal/web/static/` (38 файлов) → `web/static/`
- Delete: пустой каталог `internal/web/`
- Modify: `internal/http/server.go:325` (`http.Dir`)
- Modify: `internal/http/templates_test.go:81` (относительный путь)
- Modify: `Dockerfile:21`

**Interfaces:**
- Consumes: `web.TemplatesFS` из Task 1 (уже на месте).
- Produces: статика доступна по `web/static/…` относительно рабочего каталога процесса; URL-префикс `/static/` не меняется.

- [x] **Step 1: Перенести статику**

```bash
git mv internal/web/static web/static
rmdir internal/web
find web/static -type f | wc -l   # ожидается 38
```

- [x] **Step 2: Починить путь отдачи**

В `internal/http/server.go:325` заменить:

```go
	staticFiles := http.StripPrefix("/static/", http.FileServer(http.Dir("internal/web/static")))
```

на:

```go
	staticFiles := http.StripPrefix("/static/", http.FileServer(http.Dir("web/static")))
```

Комментарий над этой строкой (про `no-cache` и деплой в K8s) не трогать — он описывает кэширование, а не путь.

- [x] **Step 3: Починить путь в тесте**

В `internal/http/templates_test.go:81` заменить:

```go
	if _, err := os.Stat("../web/static/period_url.js"); err != nil {
```

на:

```go
	if _, err := os.Stat("../../web/static/period_url.js"); err != nil {
```

Тест лежит в `internal/http/`, поэтому до корня репозитория теперь два уровня вверх, а не один.

- [x] **Step 4: Починить Dockerfile**

В `Dockerfile:21` заменить:

```dockerfile
COPY internal/web /app/internal/web
```

на:

```dockerfile
COPY web /app/web
```

Строку `COPY migrations /app/migrations` и `WORKDIR /app` не трогать: процесс стартует из `/app`, поэтому относительный `http.Dir("web/static")` разрешается в `/app/web/static`.

- [x] **Step 5: Убедиться, что живых ссылок на старые пути не осталось**

```bash
rg -n 'internal/web|internal/http/templates' --glob '!docs/**' --glob '!.git/**'
```

Ожидается: пустой вывод. Если что-то нашлось — починить, кроме файлов под `docs/`.

- [x] **Step 6: Полный прогон**

```bash
docker info > /dev/null && go build ./... && go vet ./... && go test ./...
```

Ожидается: exit 0, 0 FAIL.

- [x] **Step 7: Smoke-проверка отдачи в рантайме**

Тесты не покрывают `http.Dir` — путь резолвится только при живом запросе. Поднять сервер из корня репозитория (переменные окружения — см. README) и проверить:

```bash
curl -sS -o /dev/null -w '%{http_code}\n' http://localhost:8080/static/tokens.css        # 200
curl -sS -o /dev/null -w '%{http_code}\n' http://localhost:8080/static/vendor/react.production.min.js  # 200
curl -sS http://localhost:8080/login | grep -c 'static/'                                  # > 0
```

Если `/static/*` отдаёт 404 — процесс запущен не из корня репозитория; это ожидаемое поведение и сегодня, менять его в рамках этого плана не требуется.

---

### Task 3: Спеки этапа A

**Files:**
- Modify: `specs/010-architecture-constraints.md` (раздел «Слои», раздел «Архитектурный стиль»)

- [x] **Step 1: Обновить утверждение о публичных пакетах**

В `specs/010-architecture-constraints.md`, раздел «Слои», в абзаце про `app` фраза:

> Единственный публичный пакет; всё остальное — `internal/`.

заменяется на:

> Публичных пакетов ровно два: `app` — фасад приложения, и `web` — SSR-ассеты (только `embed.FS` с шаблонами, без логики; существует потому, что `//go:embed` не может ссылаться за пределы каталога своего пакета). Всё остальное — `internal/`.

- [x] **Step 2: Обновить описание слоя `internal/http`**

В том же разделе строку:

> - `internal/http` — SSR handlers и templates; `NewServer(..., Options)` параметризуется…

заменить на:

> - `internal/http` — SSR handlers; шаблоны живут в `/web/templates` и встраиваются пакетом `web`; `NewServer(..., Options)` параметризуется…

- [x] **Step 3: Зафиксировать расположение статики**

В раздел «Архитектурный стиль» добавить пункт после пункта про self-hosted вендоринг:

> - файловая раскладка ассетов: `/web/templates/*.html` — SSR-shell'ы, встраиваются в бинарь через `web.TemplatesFS`; `/web/static/**` — JS/CSS/vendor, отдаются с диска (`http.Dir("web/static")` относительно рабочего каталога процесса), поэтому правки видны после обновления страницы без пересборки. URL-префикс `/static/` не зависит от раскладки на диске.

- [x] **Step 4: Проверить, что других живых расхождений не осталось**

```bash
rg -n 'internal/web|internal/http/templates' specs/ README.md README-specs.md
```

Ожидается: пустой вывод.

`specs/040-api-contract.md` на этапе A не трогается: его единственная ссылка на перемещаемый пакет — строка 914, «пакет `internal/export`», и она чинится в Task 7 вместе с переездом самого пакета.

---

# Этап B — пакеты из корня `/internal`

Порядок задач внутри этапа произволен, кроме Task 9 (спека) — она идёт последней. Каждая задача самодостаточна и заканчивается зелёным `go test ./...`.

### Task 4: `internal/okr` → `internal/core/progress`

**Files:**
- Move: `internal/okr/okr.go`, `internal/okr/okr_test.go` → `internal/core/progress/`
- Modify: `internal/service/progress.go`, `internal/service/period_progress.go`, `internal/service/service.go`, `internal/service/period_overview.go`

**Interfaces:**
- Produces: пакет `progress` с функциями `GoalProgress`, `PeriodProgress`, `ProjectProgress`, `BooleanProgress`, `NumericalProgress` — сигнатуры не меняются, меняется только имя пакета при вызове (`okr.GoalProgress` → `progress.GoalProgress`).

**⚠ Ловушка этой задачи.** `okr` используется в репозитории и как имя локальной переменной, в файлах, которые пакет `internal/okr` **не импортируют**:
- `internal/http/handlers/api/v1/teams/handler.go:93,245,246,248` — `okr.Goals`, `okr.Team.LeadUDID`
- `internal/service/service_test.go:702,717,747` — `okr.Team.ID`, `okr.Goals`

Глобальный `sed 's/okr\./progress./g'` по репозиторию сломает эти файлы. Замена делается **только в четырёх файлах**, перечисленных в **Files** выше.

- [x] **Step 1: Зафиксировать эталон перед изменением**

```bash
go test ./internal/okr/... -v 2>&1 | tail -20
```

Записать имена прошедших тестов — после переезда должен пройти тот же набор.

- [x] **Step 2: Перенести пакет и переименовать**

```bash
mkdir -p internal/core
git mv internal/okr internal/core/progress
git mv internal/core/progress/okr.go internal/core/progress/progress.go
git mv internal/core/progress/okr_test.go internal/core/progress/progress_test.go
perl -pi -e 's/^package okr\b/package progress/' \
  internal/core/progress/progress.go internal/core/progress/progress_test.go
```

- [x] **Step 3: Переписать импорт и обращения в четырёх файлах**

```bash
for f in internal/service/progress.go internal/service/period_progress.go \
         internal/service/service.go internal/service/period_overview.go; do
  perl -pi -e 's|"okrs/internal/okr"|"okrs/internal/core/progress"|; s/\bokr\./progress./g' "$f"
done
```

- [x] **Step 4: Убедиться, что ловушка не сработала**

```bash
rg -n '\bprogress\.' internal/http/handlers/api/v1/teams/handler.go internal/service/service_test.go
```

Ожидается: **пустой вывод**. Если что-то нашлось — замена задела локальные переменные, откатить эти два файла (`git checkout -- <file>`) и повторить Step 3 строго по списку.

```bash
rg -n 'okrs/internal/okr"' --glob '*.go'
```

Ожидается: пустой вывод.

- [x] **Step 5: Проверить**

```bash
go build ./... && go vet ./... && go test ./internal/core/progress/... ./internal/service/... 
```

Ожидается: тот же набор тестов, что записан в Step 1, плюс зелёный `internal/service`.

- [x] **Step 6: Полный прогон**

```bash
docker info > /dev/null && go test ./...
```

Ожидается: exit 0, 0 FAIL.

---

### Task 5: `internal/domain` → `internal/core/domain`

**Files:**
- Move: `internal/domain/` (9 файлов) → `internal/core/domain/`
- Modify: 153 файла — только строка импорта

**Interfaces:**
- Produces: пакет `domain` по пути `okrs/internal/core/domain`. **Имя пакета не меняется**, поэтому все обращения `domain.TenantScope`, `domain.Goal` и т.д. остаются как есть. Меняется исключительно import path.

- [x] **Step 1: Перенести пакет**

```bash
mkdir -p internal/core
git mv internal/domain internal/core/domain
```

- [x] **Step 2: Переписать import path во всём дереве**

Замена идёт по строке в кавычках, поэтому ложных срабатываний быть не может — в отличие от Task 4, здесь глобальный проход безопасен:

```bash
rg -l '"okrs/internal/domain"' --glob '*.go' | \
  xargs perl -pi -e 's|"okrs/internal/domain"|"okrs/internal/core/domain"|'
```

- [x] **Step 3: Проверить полноту замены**

```bash
rg -n 'okrs/internal/domain"' --glob '*.go'
```

Ожидается: пустой вывод.

```bash
rg -c '"okrs/internal/core/domain"' --glob '*.go' | wc -l
```

Ожидается: 153.

- [x] **Step 4: Проверить**

```bash
go build ./... && go vet ./...
fmtcheck $(rg -l '"okrs/internal/core/domain"' --glob '*.go' | head -20)
```

Ожидается: сборка чистая, `fmtcheck` печатает только `OK`. `fmtcheck` определён в Global Constraints; обычный `gofmt -l` здесь неприменим (CRLF в рабочем дереве). Если файл помечен `BAD` — sed нарушил группировку импортов, поправить точечно.

- [x] **Step 5: Полный прогон**

```bash
docker info > /dev/null && go test ./...
```

Ожидается: exit 0, 0 FAIL.

---

### Task 6: `internal/entitlements` → `internal/platform/entitlements`

**Files:**
- Move: `internal/entitlements/` (2 файла) → `internal/platform/entitlements/`
- Modify: `app/app.go:15`, `cmd/server/main.go:20`, `internal/http/server.go`

**Interfaces:**
- Produces: пакет `entitlements` по пути `okrs/internal/platform/entitlements`. Имя пакета и весь экспорт (`Entitlements`, `UnlimitedEntitlements`, `Factory`, `Register`, `Get`, `Unlimited`) не меняются.

- [x] **Step 1: Перенести пакет**

```bash
mkdir -p internal/platform
git mv internal/entitlements internal/platform/entitlements
```

- [x] **Step 2: Переписать import path**

```bash
rg -l '"okrs/internal/entitlements"' --glob '*.go' | \
  xargs perl -pi -e 's|"okrs/internal/entitlements"|"okrs/internal/platform/entitlements"|'
```

- [x] **Step 3: Проверить полноту**

```bash
rg -n 'okrs/internal/entitlements"' --glob '*.go'
```

Ожидается: пустой вывод.

- [x] **Step 4: Проверить и прогнать**

```bash
go build ./... && go vet ./...
fmtcheck app/app.go cmd/server/main.go internal/http/server.go
docker info > /dev/null && go test ./...
```

Ожидается: `fmtcheck` только `OK`, тесты exit 0, 0 FAIL.

---

### Task 7: `internal/export` → `internal/render/export`

**Files:**
- Move: `internal/export/` (2 файла) → `internal/render/export/`
- Modify: `internal/service/export.go`, `internal/service/export_test.go`, `internal/http/handlers/api/v1/teams/handler.go`

**Interfaces:**
- Produces: пакет `export` по пути `okrs/internal/render/export`. Имя пакета и экспорт (`Format`, `Options`, `Scope`, `TeamBlock`, `Markdown`, `Filename`) не меняются.

- [x] **Step 1: Перенести пакет**

```bash
mkdir -p internal/render
git mv internal/export internal/render/export
```

- [x] **Step 2: Переписать import path**

```bash
rg -l '"okrs/internal/export"' --glob '*.go' | \
  xargs perl -pi -e 's|"okrs/internal/export"|"okrs/internal/render/export"|'
```

- [x] **Step 3: Проверить полноту**

```bash
rg -n 'okrs/internal/export"' --glob '*.go'
```

Ожидается: пустой вывод.

- [x] **Step 4: Починить ссылку в контракте API**

В `specs/040-api-contract.md:914` строка:

```
Генерация — на сервере (пакет `internal/export`, единый источник форматирования).
```

заменяется на:

```
Генерация — на сервере (пакет `internal/render/export`, единый источник форматирования).
```

Это единственная ссылка на перемещаемые пакеты во всём `specs/040`; контракт эндпоинтов не затрагивается.

- [x] **Step 5: Проверить и прогнать**

```bash
go build ./... && go vet ./...
fmtcheck internal/service/export.go internal/service/export_test.go internal/http/handlers/api/v1/teams/handler.go
docker info > /dev/null && go test ./...
```

Ожидается: `fmtcheck` только `OK`, тесты exit 0, 0 FAIL. Особое внимание — `okrs/internal/render/export` и `okrs/internal/http/handlers/api/v1/teams` (там лежит `export_integration_test.go`).

---

### Task 8: `internal/onboarding` → `internal/platform/nomembership`

**Files:**
- Move: `internal/onboarding/nomembership.go`, `internal/onboarding/nomembership_test.go` → `internal/platform/nomembership/`
- Modify: `internal/http/server.go:101,238,380`

**Interfaces:**
- Produces: пакет `nomembership` по пути `okrs/internal/platform/nomembership`. Экспортируемые имена не меняются: `NoMembershipHandler`, `StubHandler`, `Register(name string, h NoMembershipHandler)`, `Get(name string) (NoMembershipHandler, bool)`. Строковый ключ `"stub"` и поле `Options.NoMembershipName` не меняются — конфигурация встраивающих репозиториев остаётся валидной.

**Примечание.** Переименование сделано, чтобы освободить имя `onboarding` для будущего `internal/service/onboarding` (этап C): два пакета `onboarding` в одном дереве означают постоянные алиасы. Это breaking change по import path для приватного `okrs-saas`, который blank-import'ит этот реестр; сам репозиторий вне этого дерева и отсюда не проверяется.

- [x] **Step 1: Перенести и переименовать пакет**

```bash
mkdir -p internal/platform
git mv internal/onboarding internal/platform/nomembership
perl -pi -e 's/^package onboarding\b/package nomembership/' \
  internal/platform/nomembership/nomembership.go \
  internal/platform/nomembership/nomembership_test.go
```

**Здесь `\b` использовать НЕЛЬЗЯ.** `_` — word-символ, поэтому границы слова между `onboarding` и `_test` нет, и `s/^package onboarding\b/` не совпадёт с `package onboarding_test` (внешний тестовый пакет останется со старым именем, а компилятор промолчит до первого обращения). Замена идёт без `\b`. Проверить результат:

```bash
head -1 internal/platform/nomembership/nomembership_test.go
```

- [x] **Step 2: Переписать импорт и обращения**

Обращения к пакету есть только в `internal/http/server.go` (строки 101 — комментарий, 238 — `Register`, 380 — `Get`) и в самом тесте пакета:

```bash
perl -pi -e 's|"okrs/internal/onboarding"|"okrs/internal/platform/nomembership"|; s/\bonboarding\./nomembership./g' \
  internal/http/server.go
perl -pi -e 's/\bonboarding\./nomembership./g' internal/platform/nomembership/nomembership_test.go
```

- [x] **Step 3: Проверить, что замена не задела чужое**

На момент этой задачи пакета `internal/service/onboarding` ещё не существует (он появится в этапе C), а `service.OnboardingService` пишется через `service.`, а не `onboarding.`. Убедиться:

```bash
rg -n '\bonboarding\.' --glob '*.go'
```

Ожидается: пустой вывод.

```bash
rg -n 'okrs/internal/onboarding"' --glob '*.go'
```

Ожидается: пустой вывод.

- [x] **Step 4: Проверить комментарий на строке 101**

```bash
rg -n 'NoMembershipName' internal/http/server.go
```

Комментарий должен читаться `// NoMembershipName selects the registered nomembership.NoMembershipHandler; "" → "stub".` — если sed его не поймал, поправить вручную.

- [x] **Step 5: Проверить и прогнать**

```bash
go build ./... && go vet ./...
fmtcheck internal/http/server.go internal/platform/nomembership/nomembership.go internal/platform/nomembership/nomembership_test.go
docker info > /dev/null && go test ./...
```

Ожидается: `fmtcheck` только `OK`, тесты exit 0, 0 FAIL.

- [x] **Step 6: Smoke-проверка страницы `/no-access`**

Реестр резолвится в рантайме по строковому ключу — компилятор эту связь не проверяет. Поднять сервер и открыть `/no-access` под пользователем без membership: должна отрендериться страница-заглушка, а не `500 no-membership handler not registered`.

---

### Task 9: Спека этапа B

**Files:**
- Modify: `specs/010-architecture-constraints.md` (разделы «Слои», «Repository layer», «OSS / SaaS split»)

- [x] **Step 1: Переписать перечень слоёв**

В разделе «Слои» заменить первые две строки:

```
- `internal/domain` — доменные типы и enum, включая `User`, `AuthSession`;
- `internal/okr` — расчёты прогресса;
```

на:

```
- `internal/core/domain` — доменные типы и enum, включая `User`, `AuthSession`;
- `internal/core/progress` — расчёты прогресса (бывший `internal/okr`);
- `internal/platform/entitlements` — реализация `Entitlements` и её реестр;
- `internal/platform/nomembership` — реестр страницы «нет доступа» (бывший `internal/onboarding`);
- `internal/render/export` — рендер OKR в Markdown;
```

- [x] **Step 2: Добавить правило группировки и именования**

В конец раздела «Слои» добавить:

> **Группировка в `internal/`.** В корне `internal/` лежат только слои и группы, не отдельные доменные пакеты: `core/` — чистая логика без I/O; `platform/` — registry-сеймы OSS/SaaS; `render/` — форматтеры; плюс `auth/`, `http/`, `service/`, `store/`.
>
> **Именование.** Store — множественное число, service и usecase — единственное (`store/goals` ↔ `service/goal`). Коллизии имён пакетов разрешаются алиасом на месте импорта, а не переименованием каталога: `goals "okrs/internal/store/goals"`, `goalsvc "okrs/internal/service/goal"`. Алиасы `<entity>svc` / `<entity>uc` единообразны во всём дереве.

- [x] **Step 3: Починить устаревшую таблицу Repository layer**

Таблица в разделе «Repository layer» описывает файлы `internal/store/teams.go`, `goals.go`, `periods.go` и т.д. — такой раскладки не существует уже давно: store разбит на 18 подпакетов плюс `testutil`. Заменить таблицу целиком на:

| Поле `store.Store` | Пакет | Тип |
|---|---|---|
| `Teams` | `internal/store/teams` | `*teams.TeamRepository` |
| `Goals` | `internal/store/goals` | `*goals.GoalRepository` |
| `GoalLinks` | `internal/store/goallinks` | `*goallinks.GoalLinkRepository` |
| `Periods` | `internal/store/periods` | `*periods.PeriodRepository` |
| `KRs` | `internal/store/krs` | `*krs.KRRepository` |
| `Shares` | `internal/store/shares` | `*shares.GoalShareRepository` |
| `Statuses` | `internal/store/statuses` | `*statuses.TeamStatusRepository` |
| `Users` | `internal/store/users` | `*users.UserRepository` |
| `Sessions` | `internal/store/sessions` | `*sessions.SessionRepository` |
| `Grants` | `internal/store/grants` | `*grants.GrantRepository` |
| `Settings` | `internal/store/settings` | `*settings.SettingsRepository` |
| `Activity` | `internal/store/activity` | `*activity.ActivityRepository` |
| `ProgressSnap` | `internal/store/progresssnap` | `*progresssnap.Repository` |
| `Tenants` | `internal/store/tenants` | `*tenants.TenantRepository` |
| `Memberships` | `internal/store/memberships` | `*memberships.MembershipRepository` |
| `TenantSettings` | `internal/store/tenantsettings` | `*tenantsettings.TenantSettingsRepository` |
| `UserSettings` | `internal/store/usersettings` | `*usersettings.UserSettingsRepository` |
| `Invitations` | `internal/store/invitations` | `*invitations.InvitationRepository` |

Плюс поле `DB *pgxpool.Pool` и вспомогательный пакет `internal/store/testutil` (хелперы для тестов, не репозиторий).

Абзац ниже таблицы, начинающийся со слов «`store.Store` — composite, созданный через `store.New(db)`. Содержит поля `Teams`, `Goals`, `Periods`, `KRs`, `Shares`, `Statuses`, `Users`, `Sessions`, `Grants`, `Settings`», привести к перечню из таблицы — сейчас в нём не хватает `GoalLinks`, `Activity`, `ProgressSnap`, `Tenants`, `Memberships`, `TenantSettings`, `UserSettings`, `Invitations`.

Сверить с кодом перед правкой (состав мог измениться с момента написания плана):

```bash
rg -n '^\s+[A-Z][A-Za-z]*\s+\*' internal/store/store.go
```

- [x] **Step 4: Обновить упоминание сейма в OSS / SaaS split**

Строку:

```
- `onboarding.Register(name, handler)` — no-membership-страница (OSS: `stub`);
```

заменить на:

```
- `nomembership.Register(name, handler)` — no-membership-страница (OSS: `stub`); ключ `"stub"` и `Options.NoMembershipName` не менялись при переименовании пакета из `onboarding` — обновляется только import path во встраивающем репозитории;
```

- [x] **Step 5: Финальная проверка расхождений спеки и кода**

```bash
rg -n 'internal/(domain|okr|entitlements|export|onboarding|web|http/templates)\b' specs/ README.md README-specs.md
```

Ожидается: пустой вывод. Любое совпадение — устаревшая ссылка, которую надо привести к новому пути.

- [x] **Step 6: Финальный прогон всего**

```bash
docker info > /dev/null && go build ./... && go vet ./... && go test ./...
git status --short
```

Ожидается: тесты exit 0, 0 FAIL. В `git status` не должно быть удалённых-и-неотслеженных пар вместо переименований — если `git mv` использовался корректно, изменения показываются как `R` (renamed).

---

## Что НЕ входит в этот план

- Этапы C (распил `service`), D (`usecase` + `scheduler`) и E (handlers по URI) — у каждого будет свой план, написанный непосредственно перед этапом. Их задачи опираются на точные сигнатуры, которые появятся только после предыдущего этапа.
- Создание `specs/070-code-structure.md` — этап E.
- Удаление мёртвого кода (`handlers/web/keyresults`, неиспользуемые методы `handlers/web/goals`) — этап E.
- Правка исторических документов в `docs/superpowers/**`.
