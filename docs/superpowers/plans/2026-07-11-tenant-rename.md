# Tenant Rename Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Дать tenant-admin возможность менять название своего пространства (в `/admin?section=settings`), а system-admin — название и slug любого пространства (в `/system`).

**Architecture:** Новые методы репозитория тенантов (`Rename`, `Update`) → методы `ProvisioningService` (`RenameTenant`, `UpdateTenant`) с инвалидацией кэша → два HTTP-входа: расширение `POST /api/v1/admin/settings/general` (только `name`, tenant из контекста) и новый `PATCH /api/v1/system/tenants/{id}` (`name`+`slug`, tenant из URL). Frontend — поле в `GeneralSettingsPanel` (admin.js) и inline-редактирование в `TenantsSection` (system.js).

**Tech Stack:** Go 1.x, chi router, pgx/pgxpool, PostgreSQL; frontend — React 18 (CDN) + Babel standalone, без сборки.

## Global Constraints

- Slug валидируется только через `domain.ValidTenantSlug` (lowercase, 2..32, без ведущего/замыкающего дефиса, зарезервированные слова). Не дублировать грамматику.
- Смена slug — жёсткая замена; alias/редирект старого slug не делаем.
- Tenant-admin получает id тенанта **только** из `auth.TenantScopeFromContext(ctx)`; никогда из body/URL. Поле `slug` в admin-эндпоинте не читается.
- System-плоскость гейтится `RequireSystemAdminMiddleware` (id из URL); admin-плоскость — `RequireTenantAdminMiddleware`.
- Кэш тенантов инвалидируется после каждой записи (`tenantCache.Invalidate(id)`).
- Все mutation-эндпоинты требуют CSRF при вызове из браузера.
- DB-тесты используют `testutil.SetupDB(t)` (Postgres testcontainer; `t.Skip` если docker недоступен) — как в существующих `internal/store/tenants/tenants_test.go`.
- Коммиты делает пользователь вручную. **НЕ выполнять `git commit`.** Шаги «Commit» ниже — сигнал завершённой единицы работы; вместо коммита оставить изменения в рабочем дереве и сообщить о готовности задачи.

---

### Task 1: Store — методы `Rename` и `Update` для тенанта

**Files:**
- Modify: `internal/store/tenants/tenants.go`
- Test: `internal/store/tenants/tenants_test.go`

**Interfaces:**
- Consumes: `domain.ValidTenantSlug`, `domain.Tenant`, существующие `ErrInvalidSlug`, `ErrSlugTaken`, `ErrNotFound`, `pgErrCode`.
- Produces:
  - `var ErrInvalidName = errors.New("tenants: invalid name")`
  - `func (r *TenantRepository) Rename(ctx context.Context, id int64, name string) error`
  - `func (r *TenantRepository) Update(ctx context.Context, id int64, name, slug string) (*domain.Tenant, error)`

- [ ] **Step 1: Написать падающие тесты**

Добавить в `internal/store/tenants/tenants_test.go`:

```go
func TestTenantRename(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := NewTenantRepository(pool)
	ctx := context.Background()

	tn, err := repo.Create(ctx, "acme", "Acme Inc")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.Rename(ctx, tn.ID, "Acme LLC"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	got, err := repo.GetByID(ctx, tn.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "Acme LLC" {
		t.Fatalf("name = %q, want Acme LLC", got.Name)
	}
	if err := repo.Rename(ctx, tn.ID, "  "); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("blank name: want ErrInvalidName, got %v", err)
	}
	if err := repo.Rename(ctx, 999999, "X"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing tenant: want ErrNotFound, got %v", err)
	}
}

func TestTenantUpdate(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := NewTenantRepository(pool)
	ctx := context.Background()

	tn, err := repo.Create(ctx, "acme", "Acme Inc")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := repo.Create(ctx, "globex", "Globex"); err != nil {
		t.Fatalf("create globex: %v", err)
	}

	upd, err := repo.Update(ctx, tn.ID, "Acme LLC", "acme-llc")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if upd.Name != "Acme LLC" || upd.Slug != "acme-llc" {
		t.Fatalf("update result = %+v", upd)
	}
	// старый slug освобождён (жёсткая замена)
	if _, err := repo.GetBySlug(ctx, "acme"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("old slug still resolves: %v", err)
	}

	if _, err := repo.Update(ctx, tn.ID, "X", "ACME"); !errors.Is(err, ErrInvalidSlug) {
		t.Fatalf("invalid slug: want ErrInvalidSlug, got %v", err)
	}
	if _, err := repo.Update(ctx, tn.ID, "  ", "acme-llc"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("blank name: want ErrInvalidName, got %v", err)
	}
	if _, err := repo.Update(ctx, tn.ID, "X", "globex"); !errors.Is(err, ErrSlugTaken) {
		t.Fatalf("taken slug: want ErrSlugTaken, got %v", err)
	}
	if _, err := repo.Update(ctx, 999999, "X", "free-slug"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing tenant: want ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: Запустить тесты — убедиться, что не компилируются/падают**

Run: `go test ./internal/store/tenants/ -run 'TestTenantRename|TestTenantUpdate' -v`
Expected: FAIL — `repo.Rename`/`repo.Update`/`ErrInvalidName` не определены.

- [ ] **Step 3: Реализовать методы**

В `internal/store/tenants/tenants.go` добавить в блок `var (...)` ошибку:

```go
	ErrInvalidName = errors.New("tenants: invalid name")
```

И методы (после `SetStatus`), с импортом `strings`:

```go
// Rename updates only the tenant's display name (tenant-admin path). Empty/blank name → ErrInvalidName.
func (r *TenantRepository) Rename(ctx context.Context, id int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrInvalidName
	}
	ct, err := r.db.Exec(ctx, `UPDATE tenants SET name = $2 WHERE id = $1`, id, name)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Update changes both name and slug (system-admin path). Hard slug cutover: the old slug is freed.
func (r *TenantRepository) Update(ctx context.Context, id int64, name, slug string) (*domain.Tenant, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrInvalidName
	}
	if !domain.ValidTenantSlug(slug) {
		return nil, ErrInvalidSlug
	}
	var t domain.Tenant
	err := r.db.QueryRow(ctx, `
		UPDATE tenants SET name = $2, slug = $3 WHERE id = $1
		RETURNING id, slug, name, status, created_at, deleted_at`,
		id, name, slug).Scan(&t.ID, &t.Slug, &t.Name, &t.Status, &t.CreatedAt, &t.DeletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		if pgErrCode(err) == "23505" { // unique_violation
			return nil, ErrSlugTaken
		}
		return nil, fmt.Errorf("update tenant: %w", err)
	}
	return &t, nil
}
```

Добавить `"strings"` в импорты файла.

- [ ] **Step 4: Запустить тесты — убедиться, что проходят**

Run: `go test ./internal/store/tenants/ -run 'TestTenantRename|TestTenantUpdate' -v`
Expected: PASS (или SKIP, если docker недоступен).

- [ ] **Step 5: Проверка сборки пакета**

Run: `go build ./internal/store/tenants/ && go vet ./internal/store/tenants/`
Expected: без ошибок.

- [ ] **Step 6: Завершить задачу** (без git commit — оставить изменения в рабочем дереве)

---

### Task 2: Service — `RenameTenant` и `UpdateTenant` с инвалидацией кэша

**Files:**
- Modify: `internal/service/provisioning.go`
- Test: `internal/service/provisioning_test.go`

**Interfaces:**
- Consumes: `tenants.TenantRepository.Rename`, `tenants.TenantRepository.Update`, `tenants.TenantCache.Invalidate` (Task 1); `p.tenants`, `p.tenantCache` (существующие поля).
- Produces:
  - `func (p *ProvisioningService) RenameTenant(ctx context.Context, id int64, name string) error`
  - `func (p *ProvisioningService) UpdateTenant(ctx context.Context, id int64, name, slug string) (*domain.Tenant, error)`

- [ ] **Step 1: Написать падающий тест**

Добавить в `internal/service/provisioning_test.go` (пакет `service_test`) helper построения сервиса можно скопировать из существующих тестов; тест:

```go
func TestUpdateTenantInvalidatesCache(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	tnRepo := tenants.NewTenantRepository(pool)
	tnCache := tenants.NewTenantCache(tnRepo)
	memRepo := memberships.NewMembershipRepository(pool)
	tsRepo := tenantsettings.NewTenantSettingsRepository(pool)
	sysRepo := settings.NewSettingsRepository(pool)
	settingsSvc := service.NewSettingsService(
		tenantsettings.NewTenantSettingsCache(tsRepo), tsRepo,
		settings.NewSystemSettingsCache(sysRepo), sysRepo,
	)
	grantRepo := grants.NewGrantRepository(pool)
	prov := service.NewProvisioningService(
		tnRepo, tnCache,
		memRepo, memberships.NewMembershipCache(memRepo),
		settingsSvc, grants.NewGrantsCache(grantRepo), newOnboardingForTest(t, pool), users.NewUserRepository(pool),
	)

	tn, err := tnRepo.Create(ctx, "acme", "Acme Inc")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// прогреваем кэш по старому slug
	if _, err := tnCache.GetBySlug(ctx, "acme"); err != nil {
		t.Fatalf("warm cache: %v", err)
	}

	upd, err := prov.UpdateTenant(ctx, tn.ID, "Acme LLC", "acme-llc")
	if err != nil {
		t.Fatalf("update tenant: %v", err)
	}
	if upd.Slug != "acme-llc" || upd.Name != "Acme LLC" {
		t.Fatalf("update result = %+v", upd)
	}
	// после инвалидации старый slug больше не резолвится из кэша
	if _, err := tnCache.GetBySlug(ctx, "acme"); !errors.Is(err, tenants.ErrNotFound) {
		t.Fatalf("old slug still cached: %v", err)
	}
	// rename тоже работает
	if err := prov.RenameTenant(ctx, tn.ID, "Acme Group"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	got, err := tnRepo.GetByID(ctx, tn.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "Acme Group" {
		t.Fatalf("name = %q, want Acme Group", got.Name)
	}
}
```

(Убедиться, что импорты `errors` и нужные store-пакеты присутствуют в тестовом файле — они уже используются существующими тестами.)

- [ ] **Step 2: Запустить тест — убедиться, что падает**

Run: `go test ./internal/service/ -run TestUpdateTenantInvalidatesCache -v`
Expected: FAIL — `prov.UpdateTenant`/`prov.RenameTenant` не определены.

- [ ] **Step 3: Реализовать методы сервиса**

В `internal/service/provisioning.go` после `CreateTenant`:

```go
// RenameTenant changes only the tenant's display name (tenant-admin path) and invalidates the cache.
func (p *ProvisioningService) RenameTenant(ctx context.Context, id int64, name string) error {
	if err := p.tenants.Rename(ctx, id, name); err != nil {
		return err
	}
	p.tenantCache.Invalidate(id)
	return nil
}

// UpdateTenant changes name and slug (system-admin path) and invalidates the cache.
func (p *ProvisioningService) UpdateTenant(ctx context.Context, id int64, name, slug string) (*domain.Tenant, error) {
	t, err := p.tenants.Update(ctx, id, name, slug)
	if err != nil {
		return nil, err
	}
	p.tenantCache.Invalidate(id)
	return t, nil
}
```

- [ ] **Step 4: Запустить тест — убедиться, что проходит**

Run: `go test ./internal/service/ -run TestUpdateTenantInvalidatesCache -v`
Expected: PASS (или SKIP без docker).

- [ ] **Step 5: Проверка сборки**

Run: `go build ./internal/service/ && go vet ./internal/service/`
Expected: без ошибок.

- [ ] **Step 6: Завершить задачу**

---

### Task 3: HTTP System — `PATCH /api/v1/system/tenants/{id}`

**Files:**
- Modify: `internal/http/handlers/api/v1/system/handler.go`
- Modify: `internal/http/server.go` (регистрация роута в `registerSystemRoutes`)

**Interfaces:**
- Consumes: `ProvisioningService.UpdateTenant` (Task 2); `tenants.ErrNotFound/ErrSlugTaken/ErrInvalidSlug/ErrInvalidName` (Task 1); существующие `pathID`, `toTenantDTO`, `writeError`, `writeJSON`.
- Produces:
  - Метод интерфейса `Provisioner`: `UpdateTenant(ctx context.Context, tenantID int64, name, slug string) (*domain.Tenant, error)`
  - `func (h *Handler) HandlePatchTenant(w http.ResponseWriter, r *http.Request)`

- [ ] **Step 1: Расширить интерфейс `Provisioner`**

В `internal/http/handlers/api/v1/system/handler.go`, в `type Provisioner interface`, добавить строку:

```go
	UpdateTenant(ctx context.Context, tenantID int64, name, slug string) (*domain.Tenant, error)
```

- [ ] **Step 2: Написать handler**

Добавить после `HandleListTenants`:

```go
// PATCH /api/v1/system/tenants/{id}  {name, slug}
func (h *Handler) HandlePatchTenant(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	tn, err := h.prov.UpdateTenant(r.Context(), tenantID, body.Name, body.Slug)
	if err != nil {
		switch {
		case errors.Is(err, tenants.ErrNotFound):
			writeError(w, http.StatusNotFound, "tenant not found")
		case errors.Is(err, tenants.ErrSlugTaken):
			writeError(w, http.StatusConflict, "slug already taken")
		case errors.Is(err, tenants.ErrInvalidSlug):
			writeError(w, http.StatusUnprocessableEntity, "invalid slug")
		case errors.Is(err, tenants.ErrInvalidName):
			writeError(w, http.StatusUnprocessableEntity, "invalid name")
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, toTenantDTO(tn))
}
```

- [ ] **Step 3: Зарегистрировать роут**

В `internal/http/server.go`, в `registerSystemRoutes`, рядом с `POST /api/v1/system/tenants` (строка ~519), добавить:

```go
	r.Patch("/api/v1/system/tenants/{id}", sysH.HandlePatchTenant)
```

(Разместить рядом с другими `sysH.*` роутами тенантов, внутри группы с гейтом `RequireSystemAdminMiddleware`.)

- [ ] **Step 4: Сборка и vet**

Run: `go build ./... && go vet ./internal/http/...`
Expected: без ошибок.

- [ ] **Step 5: Проверить, что `ProvisioningService` удовлетворяет `Provisioner`**

Run: `go build ./internal/http/...`
Expected: без ошибок (если сигнатура `UpdateTenant` не совпадает — компилятор упадёт на `apisystem.New(s.provisioning, ...)`).

- [ ] **Step 6: Ручная проверка доступа и поведения**

Запустить приложение (`/run` или существующая команда запуска) с system-admin сессией и выполнить:

```bash
# успех
curl -sS -X PATCH "$BASE/api/v1/system/tenants/1" \
  -H "X-CSRF-Token: $CSRF" -H 'Content-Type: application/json' \
  --cookie "$COOKIE" -d '{"name":"Renamed","slug":"renamed"}'
# ожидаем 200 и {"id":1,"slug":"renamed","name":"Renamed","status":"active"}

# занятый slug → 409; невалидный slug (напр. "AB") → 422; несуществующий id → 404
# без system-admin сессии → 403 (гейт RequireSystemAdminMiddleware)
```

Expected: коды ответов совпадают с описанием.

- [ ] **Step 7: Завершить задачу**

---

### Task 4: HTTP Admin — поле `name` в общих настройках

**Files:**
- Modify: `internal/http/handlers/api/v1/admin/handler.go`
- Modify: `internal/http/server.go` (передать зависимость в `apiadmin.New`)

**Interfaces:**
- Consumes: `auth.TenantFromContext`, `auth.TenantScopeFromContext`; `ProvisioningService.RenameTenant` (Task 2); `tenants.ErrInvalidName` — но admin-пакет не должен импортировать store/tenants ради ошибки; см. ниже.
- Produces:
  - Интерфейс в admin-пакете: `type tenantRenamer interface { RenameTenant(ctx context.Context, id int64, name string) error }`
  - Поле `renamer tenantRenamer` в `Handler`; изменённая сигнатура `New(...)`.
  - `GET/POST /api/v1/admin/settings/general` теперь читают/пишут `name`.

- [ ] **Step 1: Добавить зависимость renamer в admin.Handler**

В `internal/http/handlers/api/v1/admin/handler.go` рядом с другими интерфейсами добавить:

```go
// tenantRenamer renames the active tenant. *service.ProvisioningService satisfies it.
type tenantRenamer interface {
	RenameTenant(ctx context.Context, id int64, name string) error
}
```

В `type Handler struct` добавить поле `renamer tenantRenamer`. Обновить конструктор:

```go
func New(users userAdminStore, settings tenantSettings, mgr *auth.Manager, grants grantsStore, roles memberRoleSetter, renamer tenantRenamer) *Handler {
	return &Handler{users: users, settings: settings, mgr: mgr, grants: grants, roles: roles, renamer: renamer}
}
```

- [ ] **Step 2: Расширить GET general — вернуть `name`**

В `HandleGetGeneralSettings` (после получения `scope`) добавить чтение имени из контекста и вернуть его:

```go
func (h *Handler) HandleGetGeneralSettings(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusForbidden, "no active tenant")
		return
	}
	name := ""
	if t := auth.TenantFromContext(r.Context()); t != nil {
		name = t.Name
	}
	writeJSON(w, map[string]any{
		"name":                    name,
		"documentation_url":       h.documentationURL(r.Context(), scope),
		"empty_hierarchy_message": h.settingString(r.Context(), scope, settingKeyEmptyHierarchyMessage),
	})
}
```

- [ ] **Step 3: Расширить POST general — принять и записать `name`**

В `HandleUpdateGeneralSettings` добавить поле `Name` в body-структуру, провалидировать непустоту, переименовать тенант по id из контекста (не из body):

```go
func (h *Handler) HandleUpdateGeneralSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name                  string `json:"name"`
		DocumentationURL      string `json:"documentation_url"`
		EmptyHierarchyMessage string `json:"empty_hierarchy_message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name must not be empty")
		return
	}
	link := strings.TrimSpace(body.DocumentationURL)
	if link != "" && !isValidHTTPURL(link) {
		writeError(w, http.StatusBadRequest, "documentation_url must be a valid http(s) URL")
		return
	}
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusForbidden, "no active tenant")
		return
	}
	if err := h.renamer.RenameTenant(r.Context(), scope.TenantID, name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.settings.SetTenantProduct(r.Context(), scope, settingKeyDocumentationURL, link); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.settings.SetTenantProduct(r.Context(), scope, settingKeyEmptyHierarchyMessage, body.EmptyHierarchyMessage); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

(`strings` уже импортирован в файле.)

- [ ] **Step 4: Прокинуть renamer в конструктор**

В `internal/http/server.go` строка ~424 изменить:

```go
	adminAPI := apiadmin.New(s.store.Users, s.settingsSvc, s.auth, s.grantsCache, s.onboarding, s.provisioning)
```

- [ ] **Step 5: Сборка и vet**

Run: `go build ./... && go vet ./internal/http/...`
Expected: без ошибок (в т.ч. `*service.ProvisioningService` удовлетворяет `tenantRenamer`).

- [ ] **Step 6: Ручная проверка**

С сессией tenant-admin:

```bash
curl -sS "$BASE/api/v1/admin/settings/general" --cookie "$COOKIE"
# ожидаем поле "name" в ответе

curl -sS -X POST "$BASE/api/v1/admin/settings/general" \
  -H "X-CSRF-Token: $CSRF" -H 'Content-Type: application/json' --cookie "$COOKIE" \
  -d '{"name":"My Space","documentation_url":"","empty_hierarchy_message":""}'
# ожидаем 204; повторный GET показывает "name":"My Space"

# пустое имя → 400; сессия обычного члена (не admin) → 403 (гейт RequireTenantAdminMiddleware)
```

Проверить, что `slug` в body игнорируется (тенант меняет только имя).

Expected: коды и поведение совпадают.

- [ ] **Step 7: Завершить задачу**

---

### Task 5: Frontend admin.js — поле «Название пространства»

**Files:**
- Modify: `internal/web/static/admin.js` (компонент `GeneralSettingsPanel`, ~строка 1094)

**Interfaces:**
- Consumes: расширенные `GET/POST /api/v1/admin/settings/general` (Task 4); существующие `apiGet`, `apiPost`, `inpStyle`, `Btn`, `DetailHeader`, `DetailSection`, `T`.

- [ ] **Step 1: Добавить состояние и загрузку имени**

В `GeneralSettingsPanel` добавить state `name` и подгрузку из GET:

```javascript
function GeneralSettingsPanel() {
  const [name, setName] = useState('');
  const [url, setUrl] = useState('');
  const [emptyMsg, setEmptyMsg] = useState('');
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);

  useEffect(()=>{
    apiGet('/api/v1/admin/settings/general').then(r=>r&&r.json()).then(data=>{
      if (data) { setName(data.name||''); setUrl(data.documentation_url||''); setEmptyMsg(data.empty_hierarchy_message||''); }
    });
  },[]);
```

- [ ] **Step 2: Отправлять `name` и валидировать непустоту**

Заменить `save()`:

```javascript
  async function save() {
    if (!name.trim()) { alert('Укажите название пространства.'); return; }
    setSaving(true); setSaved(false);
    const res = await apiPost('/api/v1/admin/settings/general', {name: name.trim(), documentation_url: url.trim(), empty_hierarchy_message: emptyMsg});
    setSaving(false);
    if (res && res.ok) { setSaved(true); setTimeout(()=>setSaved(false), 2500); }
    else if (res && res.status===400) alert('Проверьте название пространства и ссылку на документацию.');
    else alert('Ошибка сохранения настроек');
  }
```

- [ ] **Step 3: Добавить секцию с полем «Название пространства»**

Вставить первой секцией внутри возвращаемого `<div>` (перед `DetailSection "Ссылка на документацию"`), после `DetailHeader`:

```javascript
    <DetailSection title="Название пространства">
      <div style={{fontSize:12.5,color:T.mutedFg,marginBottom:16,lineHeight:1.6}}>
        Отображается в переключателе пространств и заголовках. Slug пространства меняется только в системном разделе.
      </div>
      <input type="text" value={name} onChange={e=>setName(e.target.value)}
        placeholder="Название пространства"
        style={{...inpStyle,fontSize:13,marginBottom:16}}/>
    </DetailSection>
```

Оставить единственную кнопку «Сохранить» в последней секции (как сейчас) — она сохраняет всю панель, включая имя.

- [ ] **Step 4: Проверка в браузере**

Открыть `/admin?section=settings`, панель «Документация/Общие». Убедиться: поле «Название пространства» заполнено текущим именем; изменение + «Сохранить» → «✓ Сохранено»; пустое имя → alert; после перезагрузки имя сохранилось; переключатель пространств показывает новое имя (может обновиться в течение TTL кэша).

- [ ] **Step 5: Завершить задачу**

---

### Task 6: Frontend system.js — inline-редактирование name+slug

**Files:**
- Modify: `internal/web/static/system.js` (helper `patch`, компонент `TenantsSection`, ~строка 25)

**Interfaces:**
- Consumes: `PATCH /api/v1/system/tenants/{id}` (Task 3); существующие `api`, `csrfHeaders`, `errMsg`, `post`, стили `box/btn/inp/th`, `C`.

- [ ] **Step 1: Добавить `patch`-helper**

Рядom с `post/put/del` (~строка 16):

```javascript
const patch = (u, b) => api(u, {method:'PATCH', headers:csrfHeaders(), body: JSON.stringify(b)});
```

- [ ] **Step 2: Добавить состояние inline-редактирования в `TenantsSection`**

В начале компонента добавить:

```javascript
  const [editId,setEditId]=useState(null);
  const [editName,setEditName]=useState('');
  const [editSlug,setEditSlug]=useState('');
  const startEdit = (t)=>{ setErr(''); setEditId(t.id); setEditName(t.name); setEditSlug(t.slug); };
  const cancelEdit = ()=>{ setEditId(null); setEditName(''); setEditSlug(''); };
  const saveEdit = async (id)=>{ setErr('');
    const res = await patch(`/api/v1/system/tenants/${id}`, {name: editName.trim(), slug: editSlug.trim()});
    if (res.status===200){ cancelEdit(); reload(); } else setErr(await errMsg(res));
  };
```

- [ ] **Step 3: Отрисовать редактируемую строку**

Заменить тело `<tbody>` так, чтобы в режиме редактирования строка показывала инпуты, а действия — «Сохранить»/«Отмена»:

```javascript
      <tbody>{(tenants||[]).map(t=> editId===t.id
        ? <tr key={t.id}>
            <td style={{padding:'6px 8px'}}>{t.id}</td>
            <td style={{padding:'6px 8px'}}><input style={inp} value={editSlug} onChange={e=>setEditSlug(e.target.value)}/></td>
            <td style={{padding:'6px 8px'}}><input style={inp} value={editName} onChange={e=>setEditName(e.target.value)}/></td>
            <td style={{padding:'6px 8px',color:t.status==='suspended'?C.danger:C.ok}}>{t.status}</td>
            <td style={{padding:'6px 8px',display:'flex',gap:6}}>
              <button style={btn} onClick={()=>saveEdit(t.id)}>Сохранить</button>
              <button style={{...btn,background:C.muted}} onClick={cancelEdit}>Отмена</button>
            </td>
          </tr>
        : <tr key={t.id}>
            <td style={{padding:'6px 8px'}}>{t.id}</td>
            <td style={{padding:'6px 8px'}}>{t.slug}</td>
            <td style={{padding:'6px 8px'}}>{t.name}</td>
            <td style={{padding:'6px 8px',color:t.status==='suspended'?C.danger:C.ok}}>{t.status}</td>
            <td style={{padding:'6px 8px',display:'flex',gap:6}}>
              <button style={{...btn,background:C.muted}} onClick={()=>startEdit(t)}>Изменить</button>
              {t.status==='active'
                ? <button style={{...btn,background:C.danger}} onClick={()=>setStatus(t.id,'suspend')}>Suspend</button>
                : <button style={{...btn,background:C.ok}} onClick={()=>setStatus(t.id,'restore')}>Restore</button>}
            </td>
          </tr>)}</tbody>
```

- [ ] **Step 4: Проверка в браузере**

Открыть `/system` (system-admin), таб «Пространства». По кнопке «Изменить» строка превращается в поля slug+name; «Сохранить» → PATCH → таблица обновляется. Проверить ошибки: занятый slug → сообщение (409), невалидный slug → сообщение (422). «Отмена» возвращает строку без изменений.

- [ ] **Step 5: Завершить задачу**

---

### Task 7: Обновление спецификаций

**Files:**
- Modify: `specs/040-api-contract.md`
- Modify: `specs/050-permissions-and-lifecycle.md`
- Modify: `specs/020-domain-model.md`

**Interfaces:** нет кода; синхронизация source-of-truth спек с реализацией.

- [ ] **Step 1: `040-api-contract.md` — system PATCH**

В разделе «System API endpoints», после строки про `POST /api/v1/system/tenants`, добавить:

```markdown
- `PATCH /api/v1/system/tenants/{id}` — сменить название и slug пространства; body: `{"name": "...", "slug": "..."}` → `200` `{id, slug, name, status}`. `404` если тенант не найден, `409` если slug занят, `422` при невалидном slug или пустом имени. Смена slug — жёсткая замена: старый slug сразу перестаёт резолвиться (join по нему вернёт `404`); alias не сохраняется.
```

- [ ] **Step 2: `040-api-contract.md` — admin general + name**

В разделе «Общие настройки» обновить response и body:

```markdown
- `GET /api/v1/admin/settings/general` — текущие общие настройки

Response:

```json
{
  "name": "Acme",
  "documentation_url": "https://github.com/ShamanR/okrs/wiki"
}
```

- `POST /api/v1/admin/settings/general` — обновить общие настройки; body: `{"name": "Acme", "documentation_url": "https://github.com/ShamanR/okrs/wiki"}`
```

И в блок Validation добавить строку:

```markdown
- `name` — название активного пространства (tenant-admin); trim, непустое, иначе `400 VALIDATION_ERROR`. `id` тенанта берётся из контекста; `slug` через этот endpoint не меняется.
```

- [ ] **Step 3: `050-permissions-and-lifecycle.md` — кто что меняет**

В разделе «Плоскости администрирования»: в пункт **Tenant admin** добавить упоминание «переименование своего пространства (`name`) через общие настройки»; в пункт **System admin** — «смена названия и slug пространства (`PATCH …/tenants/{id}`)».

Конкретно, в пункте 1 (System admin) после «suspend/restore» добавить: `, смена названия и slug тенанта (PATCH …/tenants/{id})`. В пункте 2 (Tenant admin) после «продуктовые ключи `tenant_settings`» добавить: `, переименование своего пространства (name) в общих настройках`.

- [ ] **Step 4: `020-domain-model.md` — изменяемость name/slug**

В сущности **Tenant** нет отдельного блока инвариантов в текущем файле для tenants (он в `SystemSettings`-разделе описывает настройки). Найти описание Tenant/slug — если отсутствует явный инвариант об изменяемости, добавить короткую заметку рядом с описанием slug (там, где определяется уникальность slug), например в `050`/`040` уже покрыто; в `020` добавить строку в раздел про tenant settings нельзя — вместо этого убедиться, что модель не противоречит. Если в `020` нет сущности Tenant с полями name/slug, этот шаг — no-op; зафиксировать в сообщении, что `020` не требует изменений.

(Примечание исполнителю: сначала `rg -n "slug|Tenant" specs/020-domain-model.md`. Если найден инвариант «slug неизменяем» — исправить на «slug изменяем system-admin'ом, жёсткая замена». Если такого инварианта нет — изменения в `020` не требуются.)

- [ ] **Step 5: Проверка согласованности**

Run: `rg -n "PATCH /api/v1/system/tenants|\"name\"" specs/040-api-contract.md`
Expected: новые строки присутствуют, старые описания general обновлены (нет противоречий — в response general теперь есть `name`).

- [ ] **Step 6: Завершить задачу**

---

## Self-Review

**Spec coverage:**
- Смена name tenant-admin'ом → Task 4 (backend) + Task 5 (frontend).
- Смена name+slug system-admin'ом → Task 3 (backend) + Task 6 (frontend).
- Store/service слой + инвалидация кэша → Task 1, Task 2.
- Контроль доступа: system-гейт (Task 3, гейт группы + ручная проверка 403), admin-гейт + id из контекста + slug игнорируется (Task 4).
- Валидация slug/name (422/400/409) → Task 1 (store) + Task 3/4 (маппинг кодов).
- Жёсткая замена slug (старый освобождается) → Task 1 тест `TestTenantUpdate`, Task 2 тест инвалидации кэша.
- Обновление спек → Task 7.
- Seed demo: новых таблиц нет — изменения не требуются (зафиксировано в дизайне).

**Placeholder scan:** Task 7 Step 4 намеренно условный (зависит от фактического содержимого `020`), но содержит точную инструкцию (rg-проверка + правило if/else), а не «TBD». Остальные шаги содержат полный код.

**Type consistency:** `Rename(ctx,id,name) error` и `Update(ctx,id,name,slug) (*domain.Tenant, error)` — единые сигнатуры в Task 1 (store), Task 2 (service обёртки), Task 3 (`Provisioner.UpdateTenant`), Task 4 (`tenantRenamer.RenameTenant`). Коды ошибок `ErrInvalidName/ErrSlugTaken/ErrInvalidSlug/ErrNotFound` согласованы между Task 1 и обработчиками Task 3/4.
