# Customizable markdown messages — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans (inline,
> compiler-driven — subagents flap here). Steps use checkbox (`- [ ]`) syntax. Build/vet/test
> green after every task; commits are the user's (agent does `git add` + proposes a message, no
> AI attribution).

**Goal:** Make the `/no-access` message (global, edited in `/system`) and the tracker's
empty-hierarchy message (per-tenant, edited in `/admin?section=settings`) customizable, stored as
markdown and rendered via the shared `Markdown` component, with defaults when unset.

**Architecture:** Two settings: `system_settings.no_access_message` (system-admin) and
`tenant_settings.empty_hierarchy_message` (tenant-admin product key). The no-access page (authed,
no membership) gets its message via server-side template injection (hidden element → JS reads
`textContent` → `Markdown`); the tracker gets its message via `/api/v1/config`. Editors use the
existing `MarkdownEditor`; `system`/`no_membership` shells gain the markdown libs (`admin`/`tracker`
already have them).

**Tech Stack:** Go, chi, pgx/v5, testcontainers; React 18 UMD + Babel, shared `markdown.js`
(`Markdown`/`MarkdownEditor`, marked + DOMPurify).

## Global Constraints

- Messages are **markdown source** stored in key/value settings; rendered only via `Markdown`
  (marked + DOMPurify) — no raw HTML (XSS rule `010`).
- Empty/unset → **default text** (current hardcoded strings); behaviour unchanged.
- Write-authority: `no_access_message` is **system-only** (`SystemSet`); `empty_hierarchy_message`
  is a **tenant-admin product key** (`SetTenantProduct`).
- Explicit `domain.TenantScope`; handlers read context; system endpoints under `RequireSystemAdmin`,
  admin endpoints under `RequireTenantAdmin`.
- Commits are the user's; no AI/Claude attribution.

## Default texts (verbatim)

- No-access (current `no_access.js`): `У вашей учётной записи нет доступа ни к одной организации. Обратитесь к администратору или запросите доступ по короткому имени (slug) организации.`
- Empty hierarchy (current `tracker.js`): title `Нет доступа к командам`, hint `За доступом обратитесь к администратору`.

---

### Task 1: System `no_access_message` read/write endpoints

**Files:**
- Modify: `internal/http/handlers/api/v1/system/handler.go` (extend `HandleGetSettings`; add `HandleSetNoAccessMessage`)
- Modify: `internal/http/server.go` (mount `PUT /api/v1/system/settings/no-access-message`)
- Test: `internal/http/handlers/api/v1/system/handler_test.go`

**Interfaces:**
- `GET /api/v1/system/settings` response gains `no_access_message string`.
- `PUT /api/v1/system/settings/no-access-message` body `{"message": "..."}` → `204`.
- Consumes existing `SystemSettings` iface (`SystemGet`/`SystemSet` already present).

- [ ] **Step 1: Add failing tests** (register the route in `buildRouter`:
  `r.Put("/api/v1/system/settings/no-access-message", h.HandleSetNoAccessMessage)`)

```go
func TestSystemNoAccessMessageRoundTrip(t *testing.T) {
	admin := &domain.User{ID: 1, IsSystemAdmin: true}
	r, _, _ := buildRouter(t, admin)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/v1/system/settings/no-access-message",
		strings.NewReader(`{"message":"# Hello\nask **ops**"}`)))
	if w.Code != http.StatusNoContent {
		t.Fatalf("put: %d (%s)", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/system/settings", nil))
	var got struct {
		NoAccessMessage string `json:"no_access_message"`
	}
	_ = json.NewDecoder(w.Body).Decode(&got)
	if got.NoAccessMessage != "# Hello\nask **ops**" {
		t.Fatalf("no_access_message = %q", got.NoAccessMessage)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `go test ./internal/http/handlers/api/v1/system -run TestSystemNoAccessMessage`.

- [ ] **Step 3: Implement** — extend `HandleGetSettings` and add the setter:

```go
// GET /api/v1/system/settings
func (h *Handler) HandleGetSettings(w http.ResponseWriter, r *http.Request) {
	raw, err := h.settings.SystemGet(r.Context(), "default_registration_tenant_id")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var tenantID *int64
	if raw != nil {
		_ = json.Unmarshal(raw, &tenantID)
	}
	msgRaw, err := h.settings.SystemGet(r.Context(), "no_access_message")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var msg string
	if msgRaw != nil {
		_ = json.Unmarshal(msgRaw, &msg)
	}
	writeJSON(w, map[string]any{
		"default_registration_tenant_id": tenantID,
		"no_access_message":              msg,
	})
}

// PUT /api/v1/system/settings/no-access-message  {message}
func (h *Handler) HandleSetNoAccessMessage(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.settings.SystemSet(r.Context(), "no_access_message", body.Message); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

  In `server.go` `registerSystemRoutes`, after the default-registration PUT:
  `r.Put("/api/v1/system/settings/no-access-message", sysH.HandleSetNoAccessMessage)`.

- [ ] **Step 4: Run** `go build ./... && go test ./internal/http/handlers/api/v1/system` → PASS.
- [ ] **Step 5: Stage** (message: `feat(system): no-access message setting (get/set)`).

---

### Task 2: Tenant `empty_hierarchy_message` (admin general + config)

**Files:**
- Modify: `internal/http/handlers/api/v1/admin/handler.go` (general GET/POST + key const)
- Modify: `internal/http/handlers/api/v1/config/handler.go` (`configResponse` + build + key const)
- Test: `internal/http/handlers/api/v1/admin/handler_test.go`, `internal/http/handlers/api/v1/config/handler_test.go`

**Interfaces:**
- `GET /api/v1/admin/settings/general` response gains `empty_hierarchy_message`; `POST` accepts it.
- `GET /api/v1/config` response gains `empty_hierarchy_message`.
- Key const `settingKeyEmptyHierarchyMessage = "empty_hierarchy_message"` (declared in both handlers).

- [ ] **Step 1: Add failing tests.** Admin (uses the existing `fakeSettings` + `withTenant`):

```go
func TestHandleGeneralSettingsEmptyHierarchyMessage(t *testing.T) {
	fs := newFakeSettings()
	h := New(nil, fs, nil, nil)
	body := strings.NewReader(`{"documentation_url":"","empty_hierarchy_message":"ask **ops**"}`)
	r := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/general", body))
	w := httptest.NewRecorder()
	h.HandleUpdateGeneralSettings(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("post: %d (%s)", w.Code, w.Body.String())
	}
	r = withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/general", nil))
	w = httptest.NewRecorder()
	h.HandleGetGeneralSettings(w, r)
	var got struct {
		EmptyHierarchyMessage string `json:"empty_hierarchy_message"`
	}
	_ = json.NewDecoder(w.Body).Decode(&got)
	if got.EmptyHierarchyMessage != "ask **ops**" {
		t.Fatalf("empty_hierarchy_message = %q", got.EmptyHierarchyMessage)
	}
}
```

  Config (uses the config `fakeSettings` keyed by plain key):

```go
func TestHandleConfigEmptyHierarchyMessage(t *testing.T) {
	raw, _ := json.Marshal("ask ops")
	h := New(&fakeSettings{data: map[string]json.RawMessage{"empty_hierarchy_message": raw}})
	r := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	w := httptest.NewRecorder()
	h.HandleConfig(w, r)
	var got struct {
		EmptyHierarchyMessage string `json:"empty_hierarchy_message"`
	}
	_ = json.NewDecoder(w.Body).Decode(&got)
	if got.EmptyHierarchyMessage != "ask ops" {
		t.Fatalf("empty_hierarchy_message = %q", got.EmptyHierarchyMessage)
	}
}
```

- [ ] **Step 2: Run to verify both fail.**

- [ ] **Step 3: Admin general handler** — add the const near the other setting keys
  (`const settingKeyEmptyHierarchyMessage = "empty_hierarchy_message"`), then:

```go
func (h *Handler) HandleGetGeneralSettings(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusForbidden, "no active tenant")
		return
	}
	writeJSON(w, map[string]any{
		"documentation_url":       h.documentationURL(r.Context(), scope),
		"empty_hierarchy_message": h.settingString(r.Context(), scope, settingKeyEmptyHierarchyMessage),
	})
}
```

  In `HandleUpdateGeneralSettings`, extend the body struct and persist (after the existing
  documentation_url write, reusing the already-resolved `scope`):

```go
	var body struct {
		DocumentationURL      string `json:"documentation_url"`
		EmptyHierarchyMessage string `json:"empty_hierarchy_message"`
	}
	// ... existing decode + documentation_url validation + scope lookup + SetTenantProduct(documentation_url) ...
	if err := h.settings.SetTenantProduct(r.Context(), scope, settingKeyEmptyHierarchyMessage, body.EmptyHierarchyMessage); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
```

> No validation on the message (markdown text; sanitized at render). Keep the existing
> documentation_url http(s) validation unchanged.

- [ ] **Step 4: Config handler** — add the const + `configResponse` field + build line:

```go
const settingKeyEmptyHierarchyMessage = "empty_hierarchy_message"
// configResponse: add
EmptyHierarchyMessage string `json:"empty_hierarchy_message"`
// HandleConfig: add after the feedback lines
resp.EmptyHierarchyMessage = h.settingString(r.Context(), scope, settingKeyEmptyHierarchyMessage)
```

- [ ] **Step 5: Run** `go build ./... && go test ./internal/http/handlers/api/v1/admin ./internal/http/handlers/api/v1/config` → PASS.
- [ ] **Step 6: Stage** (message: `feat(admin/config): empty-hierarchy message setting`).

---

### Task 3: No-access page delivery (server inject + render)

**Files:**
- Modify: `internal/http/server.go` (`StubHandler.Render` passes the message to the template)
- Modify: `internal/http/templates/no_membership.html` (hidden source element + markdown libs)
- Modify: `internal/web/static/no_access.js` (render `Markdown` from the hidden element)
- Test: `app/app_test.go` (injected message appears in `/no-access` HTML)

**Interfaces:**
- Template data: `{{.NoAccessMessage}}` (string; empty when unset).

- [ ] **Step 1: Add failing app test** — seed `system_settings.no_access_message`, GET `/no-access`,
  assert the message text is in the HTML:

```go
// app/app_test.go
func TestNoAccessPageInjectsCustomMessage(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO system_settings (key, value_json) VALUES ('no_access_message', '"ping the **ops** team"')`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	a, err := app.New(app.Config{Pool: pool, Zone: time.UTC, Auth: auth.DefaultConfig()})
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	rw := httptest.NewRecorder()
	a.Handler.ServeHTTP(rw, httptest.NewRequest(http.MethodGet, "/no-access", nil))
	if rw.Code != http.StatusOK || !strings.Contains(rw.Body.String(), "ping the **ops** team") {
		t.Fatalf("no-access must embed the custom message; code=%d body has it=%v", rw.Code, strings.Contains(rw.Body.String(), "ops"))
	}
}
```

  (Add `context`/`strings` imports to `app_test.go` if missing.)

- [ ] **Step 2: Run to verify it fails** (template passes `nil`, no message in body).

- [ ] **Step 3: `StubHandler.Render` reads the setting + passes template data.** In `server.go`
  `Routes()` where `onboarding.Register("stub", ...)` is set up:

```go
onboarding.Register("stub", onboarding.StubHandler{Render: func(w http.ResponseWriter, r *http.Request) {
	var msg string
	if raw, _ := s.settingsSvc.SystemGet(r.Context(), "no_access_message"); raw != nil {
		_ = json.Unmarshal(raw, &msg)
	}
	_ = s.tmpl.ExecuteTemplate(w, "no-membership", map[string]any{"NoAccessMessage": msg})
}})
```

  (Add `encoding/json` to `server.go` imports if not present.)

- [ ] **Step 4: Template** — in `no_membership.html`, add the markdown libs (copy the exact
  `marked`/`dompurify` script tags + integrity from `admin_shell.html`) before `header.js`/`no_access.js`,
  and add a hidden source element inside `<body>`:

```html
<div id="na-msg-src" hidden>{{.NoAccessMessage}}</div>
```

- [ ] **Step 5: `no_access.js`** — read the hidden element and render markdown when present:

```jsx
const customMsg = (document.getElementById('na-msg-src')?.textContent || '').trim();
```
  In the card, replace the static `<p>...</p>` with:
```jsx
{customMsg
  ? <div className="na-card-md"><Markdown text={customMsg}/></div>
  : <p>У вашей учётной записи нет доступа ни к одной организации. Обратитесь к администратору
    или запросите доступ по короткому имени (slug) организации.</p>}
```
  (`Markdown` is global from `markdown.js`. Read `customMsg` once at module scope or inside the
  component.)

- [ ] **Step 6: Run** `go build ./... && go test ./app -run TestNoAccessPageInjectsCustomMessage` → PASS.
- [ ] **Step 7: Manual** — open `/no-access`: with the setting unset → default paragraph; set →
  markdown renders (e.g. bold), no script execution.
- [ ] **Step 8: Stage** (message: `feat(onboarding): render customizable markdown no-access message`).

---

### Task 4: `/system` editor for the no-access message

**Files:**
- Modify: `internal/http/templates/system_shell.html` (markdown libs)
- Modify: `internal/web/static/system.js` (new «Сообщения» section)
- Test: manual (DoD).

- [ ] **Step 1: Load markdown libs in `system_shell.html`** — add the `marked`/`dompurify`/`markdown.js`
  script tags (exact tags + integrity from `admin_shell.html`) before `system.js`.

- [ ] **Step 2: Add a «Сообщения» section to `system.js`** — a new tab + component:

```jsx
function MessagesSection() {
  const [msg, setMsg] = useState('');
  const [saved, setSaved] = useState('');
  useEffect(()=>{ (async()=>{ const r=await get('/api/v1/system/settings'); if(r&&r.ok){ const j=await r.json(); setMsg(j.no_access_message||''); } })(); },[]);
  const save = async ()=>{ setSaved(''); const r=await put('/api/v1/system/settings/no-access-message', {message: msg}); setSaved(r.status===204?'Сохранено':await errMsg(r)); };
  return <div style={box}>
    <h2 style={{fontSize:15,marginBottom:6}}>Сообщение «нет доступа»</h2>
    <div style={{color:C.muted,marginBottom:10}}>Markdown. Показывается на странице /no-access. Пусто → текст по умолчанию.</div>
    <MarkdownEditor value={msg} onChange={setMsg} rows={6}/>
    <div style={{marginTop:8,display:'flex',gap:8,alignItems:'center'}}>
      <button style={btn} onClick={save}>Сохранить</button>
      {saved && <span style={{color:saved==='Сохранено'?C.ok:C.danger}}>{saved}</span>}
    </div>
  </div>;
}
```

  Add tab `{tabBtn('messages','Сообщения')}` and `{tab==='messages' && <MessagesSection/>}` in `App`.
  (`MarkdownEditor` is global once the shell loads `markdown.js`.)

- [ ] **Step 3: Build** `go build ./...` → green.
- [ ] **Step 4: Manual (DoD)** — `/system` → Сообщения: edit markdown, save; reload `/no-access`
  shows it.
- [ ] **Step 5: Stage** (message: `feat(system-ui): no-access message editor`).

---

### Task 5: `/admin` editor + tracker render for empty-hierarchy message

**Files:**
- Modify: `internal/web/static/admin.js` (`GeneralSettingsPanel`)
- Modify: `internal/web/static/tracker.js` (empty-hierarchy block)
- Test: manual (DoD).

- [ ] **Step 1: `admin.js` `GeneralSettingsPanel`** — load + edit + save the message alongside
  `documentation_url`:

```jsx
// add state
const [emptyMsg, setEmptyMsg] = useState('');
// in the load .then(data=>{ ... }):
if (data) { setUrl(data.documentation_url||''); setEmptyMsg(data.empty_hierarchy_message||''); }
// in save: include the field
const res = await apiPost('/api/v1/admin/settings/general', {documentation_url: url.trim(), empty_hierarchy_message: emptyMsg});
// in the panel JSX, add a field:
<Field label="Сообщение при отсутствии доступа к командам" hint="markdown, необязательно">
  <MarkdownEditor value={emptyMsg} onChange={setEmptyMsg} rows={4}/>
</Field>
```

  (`MarkdownEditor` + `Field` already used in `admin.js`; match the surrounding markup of the
  documentation_url field — read the panel to place the field consistently.)

- [ ] **Step 2: `tracker.js` empty-hierarchy block** — render the configured message when present.
  The tracker already loads `/api/v1/config` into a `config`/`cfg` object (it reads
  `documentation_url`, `stale_days`, etc.). Use `config.empty_hierarchy_message`:

```jsx
{!loading && hierarchy.length === 0
  ? (
    <div className="no-access">
      <div className="no-access__icon">🔒</div>
      {config?.empty_hierarchy_message
        ? <div className="no-access__text"><Markdown text={config.empty_hierarchy_message}/></div>
        : <>
            <div className="no-access__text">Нет доступа к командам</div>
            <div className="no-access__hint">За доступом обратитесь к администратору</div>
          </>}
    </div>
  )
  : /* existing sidebar map */}
```

  > Verify the actual variable holding the `/api/v1/config` response in `tracker.js` (it may be
  > `cfg`, `config`, or fields lifted onto state) and read the message from there; `Markdown` is
  > global.

- [ ] **Step 3: Build** `go build ./...` → green.
- [ ] **Step 4: Manual (DoD)** — set the message in `/admin` → Общие; a user with no granted teams
  sees it (markdown) in the tracker sidebar; unset → default two lines.
- [ ] **Step 5: Stage** (message: `feat(admin-ui/tracker): customizable empty-hierarchy message`).

---

### Task 6: API spec update

**Files:**
- Modify: `specs/040-api-contract.md`

- [ ] **Step 1: Document** — under System: `GET /api/v1/system/settings` returns `no_access_message`;
  new `PUT /api/v1/system/settings/no-access-message {message}` → `204`. Under admin general
  settings + `/api/v1/config`: note the new `empty_hierarchy_message` (markdown, per-tenant). One
  line each.
- [ ] **Step 2: Run** `go build ./...` → green.
- [ ] **Step 3: Stage** (message: `docs(specs): customizable message settings`).

---

## Self-Review Notes

- **Spec coverage:** Req 1 = Task 1 (settings API) + Task 3 (page delivery/render) + Task 4 (editor);
  Req 2 = Task 2 (admin/config API) + Task 5 (editor + tracker render); specs = Task 6. Markdown
  render via shared `Markdown`; defaults preserved; write-authority (system vs product key)
  honored in Tasks 1–2.
- **Type consistency:** `no_access_message` (system key) read in `HandleGetSettings` + injected by
  `StubHandler` + consumed by `no_access.js` via `#na-msg-src`. `empty_hierarchy_message`
  (`settingKeyEmptyHierarchyMessage`) written by admin POST, read by admin GET + `/api/v1/config`
  (`EmptyHierarchyMessage` JSON `empty_hierarchy_message`), consumed by `tracker.js`. Endpoint
  paths consistent with existing system/admin routes.
- **No new DB schema, no migration.** Both are key/value settings rows.
- **Shells:** `system`/`no_membership` gain markdown libs (Tasks 3–4); `admin`/`tracker` already
  have them.

## Execution recommendation

Inline, one task at a time. Backend (1–2, and Task 3's app test, 6) is TDD/Go with green gates;
frontend (4–5, and Task 3's JS) is manual DoD. After each task: `go build ./... && go vet ./... &&
go test ./...` green (frontend: build + manual), then `git add` + propose a commit message; the
user commits. Live-verify the UI in a browser when Docker is available.
