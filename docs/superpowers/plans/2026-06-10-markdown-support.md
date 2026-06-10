# Markdown support + simple WYSIWYG Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Markdown support with a lightweight toolbar+preview editor for goal/KR/team descriptions and progress notes, and render Markdown (sanitized) in read mode.

**Architecture:** A shared browser module `markdown.js` exposes global `renderMarkdown()`, `<Markdown>`, and `<MarkdownEditor>`. It is loaded (Babel-transpiled in-browser) before both React apps (`tracker.js`, `admin.js`). Markdown is parsed by `marked`, sanitized by `DOMPurify` with a strict allowlist, and rendered client-side. Storage stays as raw Markdown in existing text columns — no DB/API/DTO changes.

**Tech Stack:** React 18 (UMD, CDN), Babel standalone (in-browser), `marked@12.0.2`, `DOMPurify@3.1.7`, Go (chi) backend (unchanged except a spec note). Static assets served from disk (`internal/web/static`); templates embedded via `go:embed`.

**Reference spec:** `docs/superpowers/specs/2026-06-10-markdown-support-design.md`

**Manual-verification note:** This repo has no JS test harness (browser Babel, no bundler/Node toolchain). "Verify" steps load the running app and observe behavior. Go tests must stay green throughout (`go build ./...`, `go test ./...`). Hard-refresh the browser (Cmd+Shift+R) after each static-file change — `tracker.css`/`*.js` are cached.

**Pinned CDN deps (real SRI hashes, both UMD globals):**
- marked: `https://unpkg.com/marked@12.0.2/marked.min.js` → global `marked` (`marked.parse(md, opts)`)
  `integrity="sha384-/TQbtLCAerC3jgaim+N78RZSDYV7ryeoBCVqTuzRrFec2akfBkHS7ACQ3PQhvMVi"`
- DOMPurify: `https://unpkg.com/dompurify@3.1.7/dist/purify.min.js` → global `DOMPurify` (`DOMPurify.sanitize(html, cfg)`)
  `integrity="sha384-XQqX/4yiUGu+oyr87jvWzRuqBUK/adrY0DunhL+tID9m/9dwSpV8h9Fk/Sg6ifVQ"`

---

## File Structure

- **Create** `internal/web/static/markdown.js` — shared module: `renderMarkdown`, `Markdown`, `MarkdownEditor` (global functions, matching the codebase's global-component style). One responsibility: Markdown render + edit.
- **Create** `internal/web/static/markdown.css` — rendered-content typography (`.md-content`) + editor toolbar/tab styles (`.md-editor*`).
- **Modify** `internal/http/templates/tracker_shell.html` — add CDN scripts, css link, `markdown.js` before `tracker.js`.
- **Modify** `internal/http/templates/admin_shell.html` — same wiring before `admin.js`.
- **Modify** `internal/web/static/tracker.js` — swap 5 read sites to `<Markdown>`, 3 edit sites to `<MarkdownEditor>`.
- **Modify** `internal/web/static/admin.js` — swap team-description textarea to `<MarkdownEditor>`.
- **Modify** `specs/040-api-contract.md` — note Markdown semantics for description/note fields.

---

## Task 1: Shared module `markdown.js`

**Files:**
- Create: `internal/web/static/markdown.js`

- [ ] **Step 1: Create the module**

Create `internal/web/static/markdown.js` with exactly this content:

```jsx
// Shared Markdown render + edit module.
// Loaded as text/babel BEFORE tracker.js / admin.js, so these globals are
// available to both React apps. Depends on globals: React, marked, DOMPurify.

// Restrict rendered output to the agreed minimal feature set. Anything richer
// (headings, code blocks, images, scripts, tables) is stripped to plain text.
const MD_ALLOWED_TAGS = ['p', 'br', 'strong', 'em', 'b', 'i', 'ul', 'ol', 'li', 'a'];
const MD_ALLOWED_ATTR = ['href', 'target', 'rel'];

// Force every link to open safely in a new tab. Runs once at load.
if (typeof DOMPurify !== 'undefined' && DOMPurify.addHook) {
  DOMPurify.addHook('afterSanitizeAttributes', node => {
    if (node.tagName === 'A') {
      node.setAttribute('target', '_blank');
      node.setAttribute('rel', 'noopener noreferrer');
    }
  });
}

function renderMarkdown(text) {
  if (!text) return '';
  const raw = marked.parse(String(text), { breaks: true, gfm: true });
  return DOMPurify.sanitize(raw, {
    ALLOWED_TAGS: MD_ALLOWED_TAGS,
    ALLOWED_ATTR: MD_ALLOWED_ATTR,
    ALLOWED_URI_REGEXP: /^(?:https?:|mailto:)/i,
  });
}

// Read component: renders sanitized Markdown HTML. Renders null for empty text.
function Markdown({ text, className }) {
  const html = renderMarkdown(text);
  if (!html) return null;
  return React.createElement('div', {
    className: 'md-content' + (className ? ' ' + className : ''),
    dangerouslySetInnerHTML: { __html: html },
  });
}

// Apply a markdown transform to the current textarea selection.
function applyMarkdownFormat(el, kind, onChange) {
  const start = el.selectionStart;
  const end = el.selectionEnd;
  const value = el.value;
  const sel = value.slice(start, end);
  let inserted;
  let selFrom;
  let selTo;
  if (kind === 'bold' || kind === 'italic') {
    const mark = kind === 'bold' ? '**' : '*';
    const inner = sel || (kind === 'bold' ? 'жирный' : 'курсив');
    inserted = mark + inner + mark;
    selFrom = start + mark.length;
    selTo = selFrom + inner.length;
  } else if (kind === 'ul' || kind === 'ol') {
    const lines = (sel || 'пункт').split('\n');
    inserted = lines.map((l, i) => (kind === 'ul' ? '- ' : (i + 1) + '. ') + l).join('\n');
    selFrom = start;
    selTo = start + inserted.length;
  } else if (kind === 'link') {
    const label = sel || 'текст';
    inserted = '[' + label + '](url)';
    selFrom = start + inserted.length - 4; // caret over "url"
    selTo = selFrom + 3;
  } else {
    return;
  }
  const next = value.slice(0, start) + inserted + value.slice(end);
  onChange(next);
  // Restore focus/selection after React re-render.
  requestAnimationFrame(() => {
    el.focus();
    el.setSelectionRange(selFrom, selTo);
  });
}

const MD_TOOLBAR = [
  { kind: 'bold', label: 'B', title: 'Жирный', style: { fontWeight: 800 } },
  { kind: 'italic', label: 'I', title: 'Курсив', style: { fontStyle: 'italic' } },
  { kind: 'ul', label: '•', title: 'Маркированный список', style: {} },
  { kind: 'ol', label: '1.', title: 'Нумерованный список', style: {} },
  { kind: 'link', label: '🔗', title: 'Ссылка', style: {} },
];

// Write component: toolbar + textarea + preview toggle.
function MarkdownEditor({ value, onChange, rows, placeholder, textareaClassName, textareaStyle }) {
  const [preview, setPreview] = React.useState(false);
  const ref = React.useRef(null);
  const val = value || '';
  const fmt = kind => { if (ref.current) applyMarkdownFormat(ref.current, kind, onChange); };
  return React.createElement('div', { className: 'md-editor' },
    React.createElement('div', { className: 'md-editor__bar' },
      React.createElement('div', { className: 'md-editor__tools' },
        MD_TOOLBAR.map(b => React.createElement('button', {
          key: b.kind, type: 'button', className: 'md-editor__btn', title: b.title,
          style: b.style, disabled: preview,
          onMouseDown: e => e.preventDefault(), // keep textarea selection
          onClick: () => fmt(b.kind),
        }, b.label))
      ),
      React.createElement('button', {
        type: 'button',
        className: 'md-editor__tab' + (preview ? ' md-editor__tab--active' : ''),
        onClick: () => setPreview(p => !p),
      }, preview ? 'Редактор' : 'Превью')
    ),
    preview
      ? React.createElement('div', { className: 'md-editor__preview' },
          val.trim()
            ? React.createElement(Markdown, { text: val })
            : React.createElement('div', { className: 'md-editor__empty' }, 'Нечего показать'))
      : React.createElement('textarea', {
          ref,
          value: val,
          onChange: e => onChange(e.target.value),
          rows: rows || 3,
          placeholder: placeholder || '',
          className: textareaClassName || 'form-textarea',
          style: textareaStyle,
        })
  );
}
```

- [ ] **Step 2: Syntax-check the JSX transpiles**

This file uses `React.createElement` (no JSX tags) so it is valid even before Babel,
but confirm it parses as JS:

Run: `node --check internal/web/static/markdown.js`
Expected: no output, exit 0. (If `node` is unavailable, skip — the browser Babel step in Task 8 covers it.)

- [ ] **Step 3: Commit**

```bash
git add internal/web/static/markdown.js
git commit -m "feat(web): shared markdown render + editor module"
```

---

## Task 2: Markdown styles `markdown.css`

**Files:**
- Create: `internal/web/static/markdown.css`

- [ ] **Step 1: Create the stylesheet**

Create `internal/web/static/markdown.css` with exactly this content:

```css
/* Rendered markdown content */
.md-content { font-size: inherit; color: inherit; line-height: 1.5; word-break: break-word; }
.md-content p { margin: 0 0 6px; }
.md-content p:last-child { margin-bottom: 0; }
.md-content ul, .md-content ol { margin: 4px 0 6px; padding-left: 20px; }
.md-content li { margin: 2px 0; }
.md-content a { color: var(--accent, #6d28d9); text-decoration: none; }
.md-content a:hover { text-decoration: underline; }
.md-content strong { font-weight: 700; }

/* Markdown editor */
.md-editor { display: flex; flex-direction: column; gap: 6px; }
.md-editor__bar { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.md-editor__tools { display: flex; align-items: center; gap: 4px; }
.md-editor__btn { min-width: 28px; height: 28px; padding: 0 7px; border: 1px solid #e5e7eb; background: #fff; border-radius: 6px; font-size: 13px; color: #374151; cursor: pointer; line-height: 1; }
.md-editor__btn:hover:not(:disabled) { background: #f3f4f6; }
.md-editor__btn:disabled { opacity: 0.4; cursor: default; }
.md-editor__tab { padding: 4px 10px; border: 1px solid #e5e7eb; background: #fff; border-radius: 6px; font-size: 12px; color: #6b7280; cursor: pointer; }
.md-editor__tab--active { background: var(--accent, #6d28d9); border-color: var(--accent, #6d28d9); color: #fff; }
.md-editor__preview { min-height: 64px; padding: 9px 12px; border: 1px solid #e5e7eb; border-radius: 8px; background: #fafafa; font-size: 13px; }
.md-editor__empty { color: #9ca3af; font-size: 13px; }
```

- [ ] **Step 2: Commit**

```bash
git add internal/web/static/markdown.css
git commit -m "feat(web): markdown content and editor styles"
```

---

## Task 3: Wire CDN deps + module into both shells

**Files:**
- Modify: `internal/http/templates/tracker_shell.html`
- Modify: `internal/http/templates/admin_shell.html`

- [ ] **Step 1: Add stylesheet link in `tracker_shell.html`**

Find this line:

```html
<link rel="stylesheet" href="/static/tracker.css">
```

Add immediately after it:

```html
<link rel="stylesheet" href="/static/markdown.css">
```

- [ ] **Step 2: Add scripts in `tracker_shell.html`**

Find this line:

```html
<script type="text/babel" src="/static/tracker.js" data-presets="react"></script>
```

Replace it with these four lines (deps first, then the shared module, then the app):

```html
<script src="https://unpkg.com/marked@12.0.2/marked.min.js" integrity="sha384-/TQbtLCAerC3jgaim+N78RZSDYV7ryeoBCVqTuzRrFec2akfBkHS7ACQ3PQhvMVi" crossorigin="anonymous"></script>
<script src="https://unpkg.com/dompurify@3.1.7/dist/purify.min.js" integrity="sha384-XQqX/4yiUGu+oyr87jvWzRuqBUK/adrY0DunhL+tID9m/9dwSpV8h9Fk/Sg6ifVQ" crossorigin="anonymous"></script>
<script type="text/babel" src="/static/markdown.js" data-presets="react"></script>
<script type="text/babel" src="/static/tracker.js" data-presets="react"></script>
```

- [ ] **Step 3: Add stylesheet link in `admin_shell.html`**

`admin_shell.html` has no external CSS link (inline `<style>` only). Find the opening:

```html
<style>
```

Add immediately BEFORE the `<style>` line:

```html
<link rel="stylesheet" href="/static/markdown.css">
```

- [ ] **Step 4: Add scripts in `admin_shell.html`**

Find this line:

```html
<script type="text/babel" src="/static/admin.js" data-presets="react"></script>
```

Replace it with:

```html
<script src="https://unpkg.com/marked@12.0.2/marked.min.js" integrity="sha384-/TQbtLCAerC3jgaim+N78RZSDYV7ryeoBCVqTuzRrFec2akfBkHS7ACQ3PQhvMVi" crossorigin="anonymous"></script>
<script src="https://unpkg.com/dompurify@3.1.7/dist/purify.min.js" integrity="sha384-XQqX/4yiUGu+oyr87jvWzRuqBUK/adrY0DunhL+tID9m/9dwSpV8h9Fk/Sg6ifVQ" crossorigin="anonymous"></script>
<script type="text/babel" src="/static/markdown.js" data-presets="react"></script>
<script type="text/babel" src="/static/admin.js" data-presets="react"></script>
```

- [ ] **Step 5: Build (templates are embedded — must rebuild)**

Run: `go build ./...`
Expected: exit 0, no output.

- [ ] **Step 6: Verify globals load (smoke test)**

Start the app, open the tracker page, open DevTools console, run:

```js
typeof renderMarkdown === 'function' && typeof Markdown === 'function' && typeof MarkdownEditor === 'function' && renderMarkdown('**hi**')
```

Expected: returns `"<p><strong>hi</strong></p>\n"` (no errors). Confirms ordering: `markdown.js` globals exist for the app and deps resolved.

- [ ] **Step 7: Commit**

```bash
git add internal/http/templates/tracker_shell.html internal/http/templates/admin_shell.html
git commit -m "feat(web): load marked, dompurify and markdown module in shells"
```

---

## Task 4: Render Markdown in tracker read surfaces

**Files:**
- Modify: `internal/web/static/tracker.js`

- [ ] **Step 1: Goal description**

Find (in `GoalCard`):

```jsx
        {goal.desc && <div className="goal-card__desc">{goal.desc}</div>}
```

Replace with:

```jsx
        {goal.desc && <Markdown text={goal.desc} className="goal-card__desc" />}
```

- [ ] **Step 2: KR description (row)**

Find (in `KRRow`):

```jsx
            {kr.desc && <div className="kr-desc">{kr.desc}</div>}
```

Replace with:

```jsx
            {kr.desc && <Markdown text={kr.desc} className="kr-desc" />}
```

- [ ] **Step 3: KR description (progress modal)**

Find (in `KRProgressModal`):

```jsx
            <div className="kr-progress-desc__text">{kr.desc}</div>
```

Replace with:

```jsx
            <Markdown text={kr.desc} className="kr-progress-desc__text" />
```

- [ ] **Step 4: Progress note**

Find (in `KRRow`):

```jsx
                <div className="kr-note__text" style={{ whiteSpace: 'pre-wrap' }}>{kr.note.text}</div>
```

Replace with (drop the manual `pre-wrap`; `breaks:true` handles newlines):

```jsx
                <Markdown text={kr.note.text} className="kr-note__text" />
```

- [ ] **Step 5: Team description (topbar)**

Find:

```jsx
          {teamOKR?.team?.description && <div className="topbar__desc">{teamOKR.team.description}</div>}
```

Replace with:

```jsx
          {teamOKR?.team?.description && <Markdown text={teamOKR.team.description} className="topbar__desc" />}
```

- [ ] **Step 6: Verify**

Hard-refresh the tracker. For a team/goal/KR that has descriptions or a note, confirm:
- `**bold**`, `*italic*`, `- item` lists, and `[label](https://example.com)` render as formatted HTML;
- a link opens in a new tab;
- plain-text (no markdown) descriptions still display normally;
- the `kr-note__text` multi-line note keeps its line breaks.

- [ ] **Step 7: Commit**

```bash
git add internal/web/static/tracker.js
git commit -m "feat(web): render markdown in tracker read surfaces"
```

---

## Task 5: Markdown editor in tracker write surfaces

**Files:**
- Modify: `internal/web/static/tracker.js`

- [ ] **Step 1: Goal description editor (`GoalModal`)**

Find:

```jsx
            <textarea value={form.desc || ''} onChange={e => set('desc', e.target.value)} rows={3} placeholder="Дополнительный контекст…" className="form-textarea" />
```

Replace with:

```jsx
            <MarkdownEditor value={form.desc} onChange={v => set('desc', v)} rows={3} placeholder="Дополнительный контекст…" textareaClassName="form-textarea" />
```

- [ ] **Step 2: KR description editor (`KREditModal`)**

Find:

```jsx
            <textarea value={form.desc || ''} onChange={e => set('desc', e.target.value)} rows={2}
              className="form-textarea form-textarea--sm" style={{ resize: 'vertical' }} />
```

Replace with:

```jsx
            <MarkdownEditor value={form.desc} onChange={v => set('desc', v)} rows={2}
              textareaClassName="form-textarea form-textarea--sm" textareaStyle={{ resize: 'vertical' }} />
```

- [ ] **Step 3: Progress note editor (`KRProgressModal`)**

Find:

```jsx
            <textarea value={note} onChange={e => setNote(e.target.value)} rows={3} placeholder="Контекст, блокеры…"
              className="form-textarea form-textarea--sm" style={{ resize: 'vertical' }} />
```

Replace with:

```jsx
            <MarkdownEditor value={note} onChange={setNote} rows={3} placeholder="Контекст, блокеры…"
              textareaClassName="form-textarea form-textarea--sm" textareaStyle={{ resize: 'vertical' }} />
```

- [ ] **Step 4: Verify**

Hard-refresh. In each modal (edit goal, edit KR, update progress):
- toolbar buttons insert markdown around the selection (select text → click **B** → wraps in `**`);
- the bulleted/numbered buttons prefix each selected line;
- the link button inserts `[text](url)` with `url` selected;
- **Превью** shows the rendered result; toggling back returns to the editor with content intact;
- Save persists; reopening shows the saved markdown source; the read view (Task 4) shows it rendered.

- [ ] **Step 5: Commit**

```bash
git add internal/web/static/tracker.js
git commit -m "feat(web): markdown editor for goal/KR descriptions and progress note"
```

---

## Task 6: Markdown editor in admin team description

**Files:**
- Modify: `internal/web/static/admin.js`

- [ ] **Step 1: Team description editor (`TeamEditor`)**

Find:

```jsx
      <Field label="Описание" hint="необязательно"><textarea rows={2} value={f.description||''} onChange={e=>setF({...f,description:e.target.value})} style={{...inpStyle,resize:'vertical',lineHeight:1.5,minHeight:64}}/></Field>
```

Replace with:

```jsx
      <Field label="Описание" hint="необязательно"><MarkdownEditor value={f.description} onChange={v=>setF({...f,description:v})} rows={2} textareaStyle={{...inpStyle,resize:'vertical',lineHeight:1.5,minHeight:64}}/></Field>
```

- [ ] **Step 2: Verify**

Hard-refresh `/admin/teams`. Open a team for editing:
- the description field now has the toolbar + preview;
- enter `**Ядро** платформы` and save;
- open the team in the tracker — the topbar description (Task 4 / earlier topbar work) renders **Ядро** in bold.

- [ ] **Step 3: Commit**

```bash
git add internal/web/static/admin.js
git commit -m "feat(web): markdown editor for team description in admin"
```

---

## Task 7: Spec note

**Files:**
- Modify: `specs/040-api-contract.md`

- [ ] **Step 1: Add Markdown semantics note**

Find this line (added in earlier work):

```
Объект `team` содержит `id`, `name`, `type`, `type_label`, `description` (описание команды; пустая строка опускается), `lead`, `parent_id`.
```

Add a new paragraph immediately after it:

```
Текстовые поля `description` (goal, key result, team) и `key_results[].note.text` несут подмножество CommonMark (жирный, курсив, списки, ссылки). Хранятся как сырой Markdown; рендерятся и санитизируются на клиенте (DOMPurify, ограниченный allowlist; ссылки открываются в новой вкладке). Форма ответа не меняется — поля остаются строками.
```

- [ ] **Step 2: Build/tests still green**

Run: `go build ./... && go test ./internal/http/... 2>&1 | tail -5`
Expected: build exit 0; tests `ok`.

- [ ] **Step 3: Commit**

```bash
git add specs/040-api-contract.md
git commit -m "docs(specs): note markdown semantics for description/note fields"
```

---

## Task 8: End-to-end + security verification

**Files:** none (verification only)

- [ ] **Step 1: XSS sanitization check (critical)**

In the goal description editor, enter each payload, save, and view the rendered read surface:

```
<script>alert(1)</script>
<img src=x onerror=alert(1)>
[click](javascript:alert(1))
# Heading should degrade
> quote should degrade
```

Expected on read render:
- no alert fires;
- `<script>`/`<img>` removed (their text content may remain, but no element/handler);
- the link renders with no `javascript:` href (href stripped / link inert);
- `#`/`>` lines show as plain text (no `<h1>`/`<blockquote>` — outside allowlist), not raw HTML.

Confirm in DevTools Elements that the rendered node contains only `p/strong/em/ul/ol/li/a` tags.

- [ ] **Step 2: Link hardening check**

Render a `[x](https://example.com)` description; inspect the `<a>` in DevTools.
Expected: `target="_blank"` and `rel="noopener noreferrer"` present.

- [ ] **Step 3: Regression check**

Confirm unrelated tracker behavior is intact: progress save, status stepper, cluster view, and the progress modal sizing from earlier work all still function.

- [ ] **Step 4: Full backend test sweep**

Run: `go build ./... && go test ./...`
Expected: all packages `ok` (DB-backed integration tests require Docker/testcontainers; if unavailable in this environment, run at least `go build ./...` and `go vet ./...` green and note the skip).

- [ ] **Step 5: Final commit (if any verification fixups were needed)**

```bash
git add -A && git commit -m "test: verify markdown rendering and sanitization"
```

---

## Self-Review notes (author)

- **Spec coverage:** module (Task 1) ✓; minimal allowlist render (Task 1) ✓; toolbar+preview editor (Task 1) ✓; CDN deps + SRI + both shells (Task 3) ✓; 5 read sites (Task 4) ✓; 4 edit sites (Tasks 5–6) ✓; storage unchanged / no migration (no task needed) ✓; spec note (Task 7) ✓; manual + XSS verification (Task 8) ✓.
- **Type consistency:** `renderMarkdown(text)`, `Markdown({text, className})`, `MarkdownEditor({value, onChange, rows, placeholder, textareaClassName, textareaStyle})` — used identically across Tasks 4–6. `onChange` receives the new string value (not an event), matching all call sites.
- **No backend changes** beyond the spec note; DTO `description` field already added in prior work.
