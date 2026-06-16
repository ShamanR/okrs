# Feedback Collection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Collect user feedback via an external survey link — a permanent hamburger-menu item, a modal nudge popup with cookie-based show logic, and an admin settings panel to control it.

**Architecture:** Settings live in the generic `system_settings` key/value table (no migration). `/api/v1/config` exposes feedback config to the SPA; admin manages it via `/api/v1/admin/settings/feedback`. The shared `header.js` (loaded on every shell) renders both the menu item and the modal, deciding visibility from config + three tracking cookies. Admin UI is a new panel in `admin.js`.

**Tech Stack:** Go (chi router, pgx), React 18 via CDN + Babel standalone (no bundler), plain CSS.

**Project rules (from CLAUDE.md):**
- Do NOT run `git commit` — the user commits manually. Each task ends with verification, not a commit.
- No business logic in handlers; DB access only through the repository layer (here we reuse the existing `settingsStore` interface).
- Keep specs as source of truth — Task 6 updates them in the same change set.

**Settings keys (all in `system_settings`):**

| Key | JSON type | Default | Meaning |
|---|---|---|---|
| `feedback_url` | string | `""` | Survey link |
| `feedback_popup_enabled` | bool | `false` | Show the modal nudge |
| `feedback_menu_link_enabled` | bool | `false` | Show the menu item |
| `feedback_frequency_days` | int | `30` | Cooldown between shows (days) |

---

## Task 1: Extend `/api/v1/config` with feedback fields

**Files:**
- Modify: `internal/http/handlers/api/v1/config/handler.go`
- Test: `internal/http/handlers/api/v1/config/handler_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/http/handlers/api/v1/config/handler_test.go`:

```go
func TestHandleConfigFeedbackDefaults(t *testing.T) {
	h := New(&fakeSettings{data: map[string]json.RawMessage{}})

	r := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	w := httptest.NewRecorder()
	h.HandleConfig(w, r)

	var got configResponse
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if got.FeedbackURL != "" {
		t.Errorf("feedback_url: want empty, got %q", got.FeedbackURL)
	}
	if got.FeedbackPopupEnabled {
		t.Errorf("feedback_popup_enabled: want false")
	}
	if got.FeedbackMenuLinkEnabled {
		t.Errorf("feedback_menu_link_enabled: want false")
	}
	if got.FeedbackFrequencyDays != 30 {
		t.Errorf("feedback_frequency_days: want default 30, got %d", got.FeedbackFrequencyDays)
	}
}

func TestHandleConfigFeedbackFromSettings(t *testing.T) {
	url, _ := json.Marshal("https://forms.example.com/survey")
	popup, _ := json.Marshal(true)
	menu, _ := json.Marshal(true)
	freq, _ := json.Marshal(7)
	h := New(&fakeSettings{data: map[string]json.RawMessage{
		"feedback_url":                url,
		"feedback_popup_enabled":      popup,
		"feedback_menu_link_enabled":  menu,
		"feedback_frequency_days":     freq,
	}})

	r := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	w := httptest.NewRecorder()
	h.HandleConfig(w, r)

	var got configResponse
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if got.FeedbackURL != "https://forms.example.com/survey" {
		t.Errorf("feedback_url: got %q", got.FeedbackURL)
	}
	if !got.FeedbackPopupEnabled || !got.FeedbackMenuLinkEnabled {
		t.Errorf("expected enabled flags true, got popup=%v menu=%v", got.FeedbackPopupEnabled, got.FeedbackMenuLinkEnabled)
	}
	if got.FeedbackFrequencyDays != 7 {
		t.Errorf("feedback_frequency_days: want 7, got %d", got.FeedbackFrequencyDays)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/http/handlers/api/v1/config/`
Expected: FAIL — `got.FeedbackURL` etc. undefined fields on `configResponse`.

- [ ] **Step 3: Implement**

In `internal/http/handlers/api/v1/config/handler.go`, add the setting-key constants near the existing one:

```go
const (
	settingKeyDocumentationURL        = "documentation_url"
	settingKeyFeedbackURL             = "feedback_url"
	settingKeyFeedbackPopupEnabled    = "feedback_popup_enabled"
	settingKeyFeedbackMenuLinkEnabled = "feedback_menu_link_enabled"
	settingKeyFeedbackFrequencyDays   = "feedback_frequency_days"
)
```

(Replace the existing standalone `const settingKeyDocumentationURL = "documentation_url"` line with this block.)

Add the response fields to `configResponse`:

```go
	// Feedback collection config consumed by the shared header (menu item + nudge).
	FeedbackURL             string `json:"feedback_url"`
	FeedbackPopupEnabled    bool   `json:"feedback_popup_enabled"`
	FeedbackMenuLinkEnabled bool   `json:"feedback_menu_link_enabled"`
	FeedbackFrequencyDays   int    `json:"feedback_frequency_days"`
```

Populate them in `HandleConfig`, right after the existing `resp := configResponse{...}` block:

```go
	resp.FeedbackURL = h.settingString(r.Context(), settingKeyFeedbackURL)
	resp.FeedbackPopupEnabled = h.settingBool(r.Context(), settingKeyFeedbackPopupEnabled)
	resp.FeedbackMenuLinkEnabled = h.settingBool(r.Context(), settingKeyFeedbackMenuLinkEnabled)
	resp.FeedbackFrequencyDays = h.settingInt(r.Context(), settingKeyFeedbackFrequencyDays, 30)
```

Add reader helpers at the bottom of the file (replace the existing `documentationURL` method with these, keeping the documentation behaviour via `settingString`):

```go
func (h *Handler) documentationURL(ctx context.Context) string {
	return h.settingString(ctx, settingKeyDocumentationURL)
}

func (h *Handler) settingString(ctx context.Context, key string) string {
	raw, err := h.settings.GetSetting(ctx, key)
	if err != nil || raw == nil {
		return ""
	}
	var s string
	_ = json.Unmarshal(raw, &s)
	return s
}

func (h *Handler) settingBool(ctx context.Context, key string) bool {
	raw, err := h.settings.GetSetting(ctx, key)
	if err != nil || raw == nil {
		return false
	}
	var b bool
	_ = json.Unmarshal(raw, &b)
	return b
}

// settingInt returns def when the value is unset, malformed, or < 1.
func (h *Handler) settingInt(ctx context.Context, key string, def int) int {
	raw, err := h.settings.GetSetting(ctx, key)
	if err != nil || raw == nil {
		return def
	}
	var n int
	if json.Unmarshal(raw, &n) != nil || n < 1 {
		return def
	}
	return n
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/http/handlers/api/v1/config/`
Expected: PASS (all config tests, old and new).

---

## Task 2: Admin feedback settings endpoints + route registration

**Files:**
- Modify: `internal/http/handlers/api/v1/admin/handler.go`
- Modify: `internal/http/server.go:271-272` (add routes after general-settings)
- Test: `internal/http/handlers/api/v1/admin/handler_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/http/handlers/api/v1/admin/handler_test.go`:

```go
func TestHandleGetFeedbackSettingsDefaults(t *testing.T) {
	h := New(nil, newFakeSettings(), nil, nil)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/feedback", nil)
	w := httptest.NewRecorder()
	h.HandleGetFeedbackSettings(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var got struct {
		FeedbackURL             string `json:"feedback_url"`
		FeedbackPopupEnabled    bool   `json:"feedback_popup_enabled"`
		FeedbackMenuLinkEnabled bool   `json:"feedback_menu_link_enabled"`
		FeedbackFrequencyDays   int    `json:"feedback_frequency_days"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if got.FeedbackURL != "" || got.FeedbackPopupEnabled || got.FeedbackMenuLinkEnabled {
		t.Errorf("want empty defaults, got %+v", got)
	}
	if got.FeedbackFrequencyDays != 30 {
		t.Errorf("feedback_frequency_days: want default 30, got %d", got.FeedbackFrequencyDays)
	}
}

func TestHandleUpdateFeedbackSettingsStoresValues(t *testing.T) {
	fs := newFakeSettings()
	h := New(nil, fs, nil, nil)
	body := strings.NewReader(`{"feedback_url":"  https://forms.example.com/s  ","feedback_popup_enabled":true,"feedback_menu_link_enabled":true,"feedback_frequency_days":14}`)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/feedback", body)
	w := httptest.NewRecorder()
	h.HandleUpdateFeedbackSettings(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d (%s)", w.Code, w.Body.String())
	}
	var url string
	_ = json.Unmarshal(fs.data["feedback_url"], &url)
	if url != "https://forms.example.com/s" {
		t.Errorf("feedback_url: want trimmed value, got %q", url)
	}
	var freq int
	_ = json.Unmarshal(fs.data["feedback_frequency_days"], &freq)
	if freq != 14 {
		t.Errorf("feedback_frequency_days: want 14, got %d", freq)
	}
}

func TestHandleUpdateFeedbackSettingsRejectsBadURL(t *testing.T) {
	h := New(nil, newFakeSettings(), nil, nil)
	body := strings.NewReader(`{"feedback_url":"javascript:alert(1)","feedback_frequency_days":14}`)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/feedback", body)
	w := httptest.NewRecorder()
	h.HandleUpdateFeedbackSettings(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleUpdateFeedbackSettingsRejectsBadFrequency(t *testing.T) {
	h := New(nil, newFakeSettings(), nil, nil)
	body := strings.NewReader(`{"feedback_url":"","feedback_frequency_days":0}`)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/feedback", body)
	w := httptest.NewRecorder()
	h.HandleUpdateFeedbackSettings(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/http/handlers/api/v1/admin/`
Expected: FAIL — `h.HandleGetFeedbackSettings` / `h.HandleUpdateFeedbackSettings` undefined.

- [ ] **Step 3: Implement handlers**

In `internal/http/handlers/api/v1/admin/handler.go`, add constants next to `settingKeyDocumentationURL`:

```go
const (
	settingKeyFeedbackURL             = "feedback_url"
	settingKeyFeedbackPopupEnabled    = "feedback_popup_enabled"
	settingKeyFeedbackMenuLinkEnabled = "feedback_menu_link_enabled"
	settingKeyFeedbackFrequencyDays   = "feedback_frequency_days"
)
```

Add the handlers (place them after `HandleUpdateGeneralSettings`):

```go
// GET /api/v1/admin/settings/feedback
func (h *Handler) HandleGetFeedbackSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	writeJSON(w, map[string]any{
		"feedback_url":                h.settingString(ctx, settingKeyFeedbackURL),
		"feedback_popup_enabled":      h.settingBool(ctx, settingKeyFeedbackPopupEnabled),
		"feedback_menu_link_enabled":  h.settingBool(ctx, settingKeyFeedbackMenuLinkEnabled),
		"feedback_frequency_days":     h.settingInt(ctx, settingKeyFeedbackFrequencyDays, 30),
	})
}

// POST /api/v1/admin/settings/feedback
// body: {"feedback_url":"https://...","feedback_popup_enabled":true,"feedback_menu_link_enabled":true,"feedback_frequency_days":30}
func (h *Handler) HandleUpdateFeedbackSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		FeedbackURL             string `json:"feedback_url"`
		FeedbackPopupEnabled    bool   `json:"feedback_popup_enabled"`
		FeedbackMenuLinkEnabled bool   `json:"feedback_menu_link_enabled"`
		FeedbackFrequencyDays   int    `json:"feedback_frequency_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	link := strings.TrimSpace(body.FeedbackURL)
	if link != "" && !isValidHTTPURL(link) {
		writeError(w, http.StatusBadRequest, "feedback_url must be a valid http(s) URL")
		return
	}
	if body.FeedbackFrequencyDays < 1 {
		writeError(w, http.StatusBadRequest, "feedback_frequency_days must be >= 1")
		return
	}
	set := func(key string, val any) bool {
		if err := h.settings.SetSetting(r.Context(), key, val); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return false
		}
		return true
	}
	if !set(settingKeyFeedbackURL, link) ||
		!set(settingKeyFeedbackPopupEnabled, body.FeedbackPopupEnabled) ||
		!set(settingKeyFeedbackMenuLinkEnabled, body.FeedbackMenuLinkEnabled) ||
		!set(settingKeyFeedbackFrequencyDays, body.FeedbackFrequencyDays) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// settingString reads a string setting; empty when unset or malformed.
func (h *Handler) settingString(ctx context.Context, key string) string {
	raw, err := h.settings.GetSetting(ctx, key)
	if err != nil || raw == nil {
		return ""
	}
	var s string
	_ = json.Unmarshal(raw, &s)
	return s
}

// settingBool reads a bool setting; false when unset or malformed.
func (h *Handler) settingBool(ctx context.Context, key string) bool {
	raw, err := h.settings.GetSetting(ctx, key)
	if err != nil || raw == nil {
		return false
	}
	var b bool
	_ = json.Unmarshal(raw, &b)
	return b
}

// settingInt reads an int setting; returns def when unset, malformed, or < 1.
func (h *Handler) settingInt(ctx context.Context, key string, def int) int {
	raw, err := h.settings.GetSetting(ctx, key)
	if err != nil || raw == nil {
		return def
	}
	var n int
	if json.Unmarshal(raw, &n) != nil || n < 1 {
		return def
	}
	return n
}
```

Note: `context` is already imported in this file; `strings`, `json`, `net/http` too. No new imports needed.

- [ ] **Step 4: Register routes**

In `internal/http/server.go`, after line 272 (the general-settings POST), add:

```go
		r.Get("/api/v1/admin/settings/feedback", adminAPI.HandleGetFeedbackSettings)
		r.Post("/api/v1/admin/settings/feedback", adminAPI.HandleUpdateFeedbackSettings)
```

- [ ] **Step 5: Run tests + build to verify**

Run: `go test ./internal/http/handlers/api/v1/admin/ && go build ./...`
Expected: PASS and clean build.

---

## Task 3: Hamburger-menu item + modal nudge (`header.js`, `header.css`)

No automated test harness exists for these CDN/Babel scripts — this task is verified manually in the browser (Task 7).

**Files:**
- Modify: `internal/web/static/header.js`
- Modify: `internal/web/static/header.css`

- [ ] **Step 1: Add cookie helpers + restructure config fetch in `header.js`**

At the top of `header.js`, just after `_hdrCSRF`, add cookie helpers:

```js
// Feedback nudge tracking cookies. ~2-year lifetime, site-wide path.
function _fbGet(name) {
  const m = document.cookie.match(new RegExp('(?:^|;\\s*)' + name + '=([^;]*)'));
  return m ? decodeURIComponent(m[1]) : '';
}
function _fbSet(name, val) {
  document.cookie = name + '=' + encodeURIComponent(val) + ';path=/;max-age=' + (2 * 365 * 24 * 60 * 60) + ';SameSite=Lax';
}
```

In `HeaderNavMenu`, replace the `docUrl` state + its effect with a full-config state:

Replace:
```js
  const [docUrl, setDocUrl] = React.useState('');
  React.useEffect(() => {
    fetch('/api/v1/config', { credentials: 'include' })
      .then(r => (r.ok ? r.json() : null))
      .then(cfg => { if (cfg && cfg.documentation_url) setDocUrl(cfg.documentation_url); })
      .catch(() => {});
  }, []);
```
with:
```js
  const [cfg, setCfg] = React.useState(null);
  React.useEffect(() => {
    fetch('/api/v1/config', { credentials: 'include' })
      .then(r => (r.ok ? r.json() : null))
      .then(c => { if (c) setCfg(c); })
      .catch(() => {});
  }, []);
  const docUrl = (cfg && cfg.documentation_url) || '';
```

- [ ] **Step 2: Add the "Обратная связь" menu item under "Документация"**

In `HeaderNavMenu`, immediately after the `{docUrl && ( ... Документация ... )}` block, add:

```js
              {cfg && cfg.feedback_menu_link_enabled && cfg.feedback_url && (
                <a href={cfg.feedback_url} target="_blank" rel="noopener noreferrer" className="nav-menu__item">
                  <span className="nav-menu__item-icon">💬</span>Обратная связь
                </a>
              )}
```

- [ ] **Step 3: Render the nudge inside the HeaderNavMenu fragment**

In the `return ( <React.Fragment> ... )`, add `<FeedbackNudge cfg={cfg} />` right after the `<button ... className="nav-menu__burger">` line (so it mounts on every page regardless of drawer state):

```js
      <button onClick={() => setOpen(true)} className="nav-menu__burger" aria-label="Меню">☰</button>
      <FeedbackNudge cfg={cfg} />
```

- [ ] **Step 4: Add the `FeedbackNudge` component**

At the end of `header.js` (after `HeaderNavMenu`), add:

```js
// FeedbackNudge — модальное окно-просьба оставить обратную связь. Логика показа
// на cookies: первый показ не раньше чем через 2 суток с начала вовлечения,
// повторный — не раньше чем через feedback_frequency_days после закрытия.
// Долгий перерыв (визитов не было дольше частоты) сбрасывает 2-дневный grace.
function FeedbackNudge({ cfg }) {
  const [show, setShow] = React.useState(false);

  React.useEffect(() => {
    if (!cfg) return;
    const DAY = 86400000;
    const freqMs = (cfg.feedback_frequency_days || 30) * DAY;
    const now = Date.now();

    // Engagement tracking — runs on every page load, even when the popup is off.
    let start = parseInt(_fbGet('okr_fb_start'), 10);
    const seen = parseInt(_fbGet('okr_fb_seen'), 10);
    if (!start || !seen || now - seen > freqMs) {
      start = now;               // first visit, or return after a long break
      _fbSet('okr_fb_start', String(now));
    }
    _fbSet('okr_fb_seen', String(now));

    if (!cfg.feedback_popup_enabled || !cfg.feedback_url) return;
    const graceOK = now - start >= 2 * DAY;
    const dismissed = parseInt(_fbGet('okr_fb_dismissed'), 10);
    const cooldownOK = !dismissed || (now - dismissed >= freqMs);
    if (graceOK && cooldownOK) setShow(true);
  }, [cfg]);

  function dismiss() {
    _fbSet('okr_fb_dismissed', String(Date.now()));
    setShow(false);
  }

  React.useEffect(() => {
    if (!show) return;
    const onKey = e => { if (e.key === 'Escape') dismiss(); };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [show]);

  if (!show || !cfg) return null;
  return (
    <div className="fb-nudge__overlay" onClick={dismiss}>
      <div className="fb-nudge__card" onClick={e => e.stopPropagation()} role="dialog" aria-modal="true" aria-label="Обратная связь">
        <button onClick={dismiss} className="fb-nudge__close" aria-label="Закрыть">✕</button>
        <div className="fb-nudge__icon">💬</div>
        <div className="fb-nudge__title">Поделитесь обратной связью</div>
        <div className="fb-nudge__text">Помогите сделать инструмент лучше — это займёт пару минут.</div>
        <a href={cfg.feedback_url} target="_blank" rel="noopener noreferrer" className="fb-nudge__btn" onClick={dismiss}>
          Поделиться обратной связью
        </a>
      </div>
    </div>
  );
}
```

- [ ] **Step 5: Add styles to `header.css`**

Append to `internal/web/static/header.css`:

```css
/* FeedbackNudge — модальное окно сбора обратной связи. Светлая карточка на
   затемнённом фоне, фиолетовый акцент в тон сайта (#7c3aed). */
.fb-nudge__overlay { position: fixed; inset: 0; background: rgba(12,18,32,0.55); z-index: 2100; display: flex; align-items: center; justify-content: center; padding: 16px; }
.fb-nudge__card { position: relative; width: 380px; max-width: 92vw; background: #fff; border-radius: 16px; padding: 28px 26px 26px; box-shadow: 0 20px 50px rgba(15,23,42,0.35); text-align: center; animation: fbNudgeIn 0.18s ease; }
@keyframes fbNudgeIn { from { opacity: 0; transform: translateY(8px) scale(0.98); } to { opacity: 1; transform: none; } }
.fb-nudge__close { position: absolute; top: 12px; right: 12px; background: #f1f5f9; border: none; color: #64748b; font-size: 13px; cursor: pointer; width: 30px; height: 30px; border-radius: 8px; }
.fb-nudge__close:hover { background: #e2e8f0; color: #0f172a; }
.fb-nudge__icon { font-size: 34px; line-height: 1; margin-bottom: 12px; }
.fb-nudge__title { font-size: 18px; font-weight: 800; color: #0f172a; letter-spacing: -0.3px; margin-bottom: 8px; }
.fb-nudge__text { font-size: 14px; color: #475569; line-height: 1.55; margin-bottom: 20px; }
.fb-nudge__btn { display: inline-block; width: 100%; box-sizing: border-box; padding: 11px 16px; border-radius: 10px; background: #7c3aed; color: #fff; font-size: 14px; font-weight: 700; text-decoration: none; }
.fb-nudge__btn:hover { background: #6d28d9; }
```

- [ ] **Step 6: Sanity-check the JS parses**

Run: `node --check internal/web/static/header.js`
Expected: no output (exit 0). Note: JSX is not plain JS, so `node --check` will error on JSX lines. If it errors on a `<` token, that is expected — instead verify there are no obvious bracket mismatches by eye and rely on the browser verification in Task 7. (If `node --check` passes on this repo's other `.js` files it is because they are also Babel-compiled; treat a JSX parse error as acceptable here.)

Better verification: load the tracker page in the browser (Task 7) and confirm no console error from `header.js`.

---

## Task 4: Admin feedback settings panel (`admin.js`)

Verified manually in the browser (Task 7).

**Files:**
- Modify: `internal/web/static/admin.js` (add `FeedbackSettingsPanel`, render it in the `settings` section ~line 1277-1280)

- [ ] **Step 1: Add the `FeedbackSettingsPanel` component**

Place it right after `GeneralSettingsPanel` (ends ~line 1004) in `admin.js`:

```js
function FeedbackSettingsPanel() {
  const [url, setUrl] = useState('');
  const [popup, setPopup] = useState(false);
  const [menu, setMenu] = useState(false);
  const [freq, setFreq] = useState(30);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);

  useEffect(()=>{
    apiGet('/api/v1/admin/settings/feedback').then(r=>r&&r.json()).then(data=>{
      if (!data) return;
      setUrl(data.feedback_url||'');
      setPopup(!!data.feedback_popup_enabled);
      setMenu(!!data.feedback_menu_link_enabled);
      setFreq(data.feedback_frequency_days||30);
    });
  },[]);

  async function save() {
    const n = parseInt(freq,10);
    if (!Number.isFinite(n) || n < 1) { alert('Частота должна быть целым числом ≥ 1.'); return; }
    setSaving(true); setSaved(false);
    const res = await apiPost('/api/v1/admin/settings/feedback', {
      feedback_url: url.trim(),
      feedback_popup_enabled: popup,
      feedback_menu_link_enabled: menu,
      feedback_frequency_days: n,
    });
    setSaving(false);
    if (res && res.ok) { setSaved(true); setTimeout(()=>setSaved(false), 2500); }
    else if (res && res.status===400) alert('Проверьте ссылку (http/https) и частоту (≥ 1).');
    else alert('Ошибка сохранения настроек');
  }

  const toggleRow = (checked, onChange, label, hint) => (
    <label style={{display:'flex',alignItems:'flex-start',gap:10,cursor:'pointer',marginBottom:14}}>
      <input type="checkbox" checked={checked} onChange={e=>onChange(e.target.checked)} style={{marginTop:3}}/>
      <span>
        <span style={{fontSize:13.5,fontWeight:600,color:T.bodyFg}}>{label}</span>
        <span style={{display:'block',fontSize:12,color:T.mutedFg,marginTop:2}}>{hint}</span>
      </span>
    </label>
  );

  return <div>
    <DetailHeader breadcrumb="Настройки" title="Обратная связь"
      subtitle="Сбор обратной связи по инструменту через внешний опрос"/>
    <DetailSection title="Ссылка на опрос">
      <div style={{fontSize:12.5,color:T.mutedFg,marginBottom:12,lineHeight:1.6}}>
        Ссылка на форму обратной связи. Пока поле пустое, пункт меню и всплывающее окно не показываются.
      </div>
      <input type="url" value={url} onChange={e=>setUrl(e.target.value)}
        placeholder="https://forms.gle/..."
        style={{...inpStyle,fontSize:13,marginBottom:4}}/>
    </DetailSection>
    <DetailSection title="Где показывать">
      {toggleRow(menu, setMenu, 'Пункт «Обратная связь» в меню', 'Постоянная ссылка в гамбургер-меню под «Документация».')}
      {toggleRow(popup, setPopup, 'Всплывающее окно с просьбой', 'Модальное окно появляется не ранее чем через 2 суток работы с инструментом.')}
    </DetailSection>
    <DetailSection title="Частота показа окна">
      <div style={{fontSize:12.5,color:T.mutedFg,marginBottom:10,lineHeight:1.6}}>
        Минимальный интервал между показами окна, в днях. Перерыв в работе дольше этого срока заново даёт пользователю 2 дня перед показом.
      </div>
      <input type="number" min="1" value={freq} onChange={e=>setFreq(e.target.value)}
        style={{...inpStyle,fontSize:13,width:140,marginBottom:16}}/>
      <div style={{display:'flex',alignItems:'center',gap:10}}>
        <Btn variant="primary" onClick={save} disabled={saving}>
          {saving?'Сохранение…':'Сохранить'}
        </Btn>
        {saved&&<span style={{fontSize:12,color:'#059669',fontWeight:600}}>✓ Сохранено</span>}
      </div>
    </DetailSection>
  </div>;
}
```

- [ ] **Step 2: Render the panel in the settings section**

In `admin.js` `App`, inside the `section==='settings'` block (currently the two cards at ~line 1278-1279), add a third card after `GeneralSettingsPanel`:

```js
      <div style={{background:'white',borderRadius:12,border:'1px solid '+T.cardBorder,boxShadow:'0 1px 3px rgba(15,23,42,0.04)',overflow:'hidden'}}><FeedbackSettingsPanel/></div>
```

- [ ] **Step 3: Build the server (embeds static assets) to confirm nothing broke**

Run: `go build ./...`
Expected: clean build.

---

## Task 5: Run full backend test + vet

- [ ] **Step 1: Vet + full test suite**

Run: `go vet ./... && go test ./...`
Expected: PASS across the module (no regressions in config/admin or elsewhere).

---

## Task 6: Update specs (same change set)

**Files:**
- Modify: `specs/040-api-contract.md`
- Modify: `specs/030-user-flows.md`
- Modify: `specs/050-permissions-and-lifecycle.md`

- [ ] **Step 1: API contract — `specs/040-api-contract.md`**

Find the `GET /api/v1/config` section and add the four `feedback_*` response fields (`feedback_url` string, `feedback_popup_enabled` bool, `feedback_menu_link_enabled` bool, `feedback_frequency_days` int, default 30). Find the admin settings section (where `/api/v1/admin/settings/general` is documented) and add:

```
GET  /api/v1/admin/settings/feedback   → {feedback_url, feedback_popup_enabled, feedback_menu_link_enabled, feedback_frequency_days}
POST /api/v1/admin/settings/feedback   body same shape; validation: feedback_url пустой или http(s); feedback_frequency_days >= 1. Admin-only.
```

Match the surrounding document's existing formatting/heading style.

- [ ] **Step 2: User flows — `specs/030-user-flows.md`**

Add a short "Сбор обратной связи" flow describing: пункт меню «Обратная связь» (под «Документация») при включённом тумблере и заданной ссылке; модальное окно появляется не ранее чем через 2 суток вовлечения, повторно — не раньше `feedback_frequency_days` дней после закрытия; долгий перерыв сбрасывает 2-дневный grace; закрытие по крестику/Esc/overlay/клику по ссылке; трекинг через cookies `okr_fb_start` / `okr_fb_seen` / `okr_fb_dismissed`.

- [ ] **Step 3: Permissions & settings — `specs/050-permissions-and-lifecycle.md`**

In the settings/keys area, document the four new `system_settings` keys and that they are admin-managed (read by any authenticated user via `/api/v1/config`). Note no migration is required (generic key/value table).

- [ ] **Step 2 self-check:** Ensure you did not touch unrelated spec sections (CLAUDE.md rule 3).

---

## Task 7: Manual browser verification

- [ ] **Step 1: Start the app**

Run the app per the project's usual method (`docker-compose up` or the documented run path). Log in.

- [ ] **Step 2: Admin panel**

Go to `/admin` → Настройки. Confirm the new "Обратная связь" card. Set a survey URL, enable both toggles, set frequency, Save → expect "✓ Сохранено". Reload → values persist. Try an invalid URL and frequency 0 → expect the alert (400).

- [ ] **Step 3: Menu link**

Open the hamburger menu on the tracker page. Confirm "Обратная связь" appears directly under "Документация" and opens the survey in a new tab. Disable the menu toggle in admin → reload → item disappears.

- [ ] **Step 4: Nudge show logic (via cookies in DevTools)**

With popup enabled and a URL set: clear cookies `okr_fb_*`, reload — no popup (first touch, grace). In DevTools set `okr_fb_start` to a timestamp 3 days in the past (e.g. `Date.now() - 3*86400000`) and reload — popup appears. Click ✕ — popup closes and `okr_fb_dismissed` is set. Reload — no popup (cooldown). Confirm the survey button opens a new tab and also sets `okr_fb_dismissed`.

- [ ] **Step 5: Console check**

Confirm no JS errors from `header.js` or `admin.js` in the console on tracker, admin, and settings pages.

---

## Self-Review notes

- **Spec coverage:** menu link (Task 3 Step 2), modal nudge (Task 3 Step 4), admin section with all four controls (Task 4), cookie show-logic incl. long-break reset (Task 3 Step 4), config exposure (Task 1), admin endpoints + validation (Task 2), specs (Task 6). All design sections covered.
- **No migration:** `system_settings` is generic key/value — confirmed; seed/migrations untouched.
- **Type consistency:** setting keys, JSON field names (`feedback_url`, `feedback_popup_enabled`, `feedback_menu_link_enabled`, `feedback_frequency_days`), and cookie names (`okr_fb_start`, `okr_fb_seen`, `okr_fb_dismissed`) are identical across Go (Tasks 1–2), `header.js` (Task 3), and `admin.js` (Task 4).
- **No git commits:** per CLAUDE.md the user commits manually; tasks end on verification.
