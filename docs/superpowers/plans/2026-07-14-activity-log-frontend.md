# Activity Log — Frontend Implementation Plan (Plan 2 of 2)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the `/activity-log` SPA page (sidebar with period + team tree showing log counts, filterable time-grouped feed with БЫЛО→СТАЛО and deep links), add tracker deep-linking, and add the purge control to the admin and system panels.

**Architecture:** No-bundler React 18 (JSX compiled in-browser by `@babel/standalone`). A new SSR shell (`activity_shell.html`) serves a new `activity.js` that reuses the shared `Sidebar`/`SidebarSections` from `sidebar.js` and re-declares tracker-local helpers (`PeriodSelect`, `SidebarNode`, favorites, `apiGet`). Consumes the Plan-1 API: `GET /api/v1/activity`, `GET /api/v1/activity/tree-counts`, `GET /api/v1/hierarchy`, `GET /api/v1/periods`, `GET /api/v1/me`. **Depends on Plan 1 being merged.**

**Tech Stack:** React 18 self-hosted, chi SSR shells, existing `sidebar.js`/`api.js`/`storage.js`/`ui.js` globals. Design doc: `docs/superpowers/specs/2026-07-14-activity-log-design.md`.

## Global Constraints

- **Do NOT run `git commit`** (CLAUDE.md rule 8). End each task with a **Checkpoint**: reload the app and visually verify, then stop for review.
- **No JS bundler/toolchain.** Shared modules expose bare globals loaded via `<script type="text/babel" data-presets="react">` BEFORE the app script. Tracker components are **not exported** — reuse = copy the helper into `activity.js` (or a new shared file), not import.
- **Escape all user data.** Titles, comment text, author names render as React text (never `dangerouslySetInnerHTML` on raw values); markdown goes through the shared `Markdown` component (which sanitizes via DOMPurify).
- **Reuse before rebuild** (CLAUDE.md rule 12): the shared `Sidebar`/`SidebarSections`/`SidebarBell`, `Markdown`, `ACCENT`/`TEAM_TYPE_COLOR`, and `csrfHeaders` come from globals as-is. Period selector, team tree, favorites are copied from `tracker.js` with identical CSS classes so the two pages look the same.
- **Favorites are shared per-user** via localStorage key `okr_fav_teams:{uid}` — reading the same key gives the same starred teams as the tracker automatically.
- **Deep-link URL contract** (produced by the API's `target` + built by `buildTargetURL`): `/?team=<t>&period=<p>&goal=<g>[&kr=<k>][&comment=<c>]`.

---

## File Structure

**Create:**
- `internal/http/templates/activity_shell.html` — `{{define "activity-shell"}}`
- `internal/web/static/activity.js` — the page
- `internal/web/static/activity.css` — page styles

**Modify:**
- `internal/http/server.go` — route `/activity-log` → `activity-shell` (replace stub)
- `internal/http/templates_test.go` — add `"activity-shell"` to the shells slice
- `internal/web/static/tracker.js` — deep-link params + DOM anchors + scroll/open on target
- `internal/web/static/admin.js` — `ActivityLogPanel` (purge) in the settings section
- `internal/web/static/system.js` — purge control in `TenantsSection`
- `specs/030-user-flows.md` — `/activity-log` page description + tracker deep-link params

---

# Phase F — Activity page

### Task F1: Shell + route + template test

**Files:**
- Create: `internal/http/templates/activity_shell.html`
- Modify: `internal/http/server.go`
- Modify: `internal/http/templates_test.go`

- [ ] **Step 1: Create the shell** — `internal/http/templates/activity_shell.html`:
```html
{{define "activity-shell"}}
<!DOCTYPE html>
<html lang="ru">
<head>
{{template "spa-head" .}}
<title>OKR Tracker · Лог активностей</title>
<link rel="stylesheet" href="/static/tracker.css">
<link rel="stylesheet" href="/static/markdown.css">
<link rel="stylesheet" href="/static/sidebar.css">
<link rel="stylesheet" href="/static/activity.css">
</head>
<body>
<div id="root"><div class="loading-screen">Загрузка…</div></div>
{{template "spa-vendor" .}}
<script type="text/babel" src="/static/api.js" data-presets="react"></script>
<script type="text/babel" src="/static/storage.js" data-presets="react"></script>
<script type="text/babel" src="/static/ui.js" data-presets="react"></script>
<script type="text/babel" src="/static/markdown.js" data-presets="react"></script>
<script type="text/babel" src="/static/sidebar.js" data-presets="react"></script>
<script type="text/babel" src="/static/activity.js" data-presets="react"></script>
</body>
</html>
{{end}}
```

- [ ] **Step 2: Swap the route** — in `internal/http/server.go` `registerWebRoutes`, replace the `/activity-log` stub line (currently `r.Get("/activity-log", stubShell)` ~line 398) with:
```go
	activityShell := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = s.tmpl.ExecuteTemplate(w, "activity-shell", s.shellData())
	}
	r.Get("/activity-log", activityShell)
```
Leave `/goal-tree` on `stubShell`.

- [ ] **Step 3: Update the template test** — `internal/http/templates_test.go`, add `"activity-shell"` to the shells slice (~line 35):
```go
	shells := []string{"tracker-shell", "settings-shell", "admin-shell", "system-shell", "stub-shell", "activity-shell"}
```

- [ ] **Step 4: Create empty static files so the shell loads** — `internal/web/static/activity.css` (empty for now) and `internal/web/static/activity.js` with a minimal render so the page isn't blank:
```jsx
const { useState, useEffect } = React;
function App() {
  const [me, setMe] = useState(null);
  useEffect(() => { fetch('/api/v1/me').then(r => r.ok && r.json()).then(setMe); }, []);
  return <Sidebar user={me} active="activity-log"><div className="sidebar__tree" /></Sidebar>;
}
ReactDOM.createRoot(document.getElementById('root')).render(<App />);
```

- [ ] **Step 5: Verify** — `go test ./internal/http/ -run TestShell -v` → PASS. Then run the app (see repo README / `/run`), open `/activity-log`: the shared dark sidebar renders with «Лог активностей» highlighted. Do not commit.

---

### Task F2: Sidebar — period selector + team tree with log counts + favorites

**Files:**
- Modify: `internal/web/static/activity.js`

**Interfaces:**
- Produces the page's data layer: fetch `/api/v1/me`, `/api/v1/periods`, `/api/v1/hierarchy?period_id=`, `/api/v1/activity/tree-counts?period_id=&range=`; render `Sidebar` with `beforeSections={<PeriodSelect/>}` and a team tree whose per-node number is the **rolled-up activity count**.

- [ ] **Step 1: Copy the shared helpers into `activity.js`** — at the top, re-declare (verbatim copies, tracker.js is the source of truth — copy exact code from the referenced lines):
  - `apiFetch`/`apiGet` block (tracker.js:6-19) — GET only is needed here.
  - `PeriodSelect` + `TRK_PERIOD_STATUS` + `fmtDateRange`/`fmtPeriodDate` (tracker.js:1698-1744).
  - Favorites helpers `FAV_KEY`/`favId`/`readFavorites`/`writeFavorites`/`toggleFavorite`/`collectFavNodes` (tracker.js:92-126).
  - Tree helpers `findNodeById` (tracker.js:2130-2135).
  - `TEAM_TYPE_COLOR` comes from the shared `ui.js` global — do NOT redeclare.

- [ ] **Step 2: Write a count-aware tree node** — add an `ActivitySidebarNode` (adapted from tracker.js `SidebarNode` 1747-1779, but the trailing number is the activity count, not progress, and rolled up over the subtree):
```jsx
// sum activity counts over a node's subtree using the flat per-team counts map.
function subtreeCount(node, counts) {
  let total = counts[String(node.id)] || counts[node.id] || 0;
  (node.children || []).forEach(c => { total += subtreeCount(c, counts); });
  return total;
}

function ActivitySidebarNode({ node, depth, selectedId, onSelect, expanded, toggle, counts, favSet, onToggleFav }) {
  const ch = node.children || [];
  const isExp = expanded[node.id] !== false;
  const isSel = selectedId === node.id;
  const n = subtreeCount(node, counts);
  const dotC = (typeof TEAM_TYPE_COLOR !== 'undefined' && TEAM_TYPE_COLOR[node.type]) || '#94a3b8';
  const pad = 14 + depth * 13;
  const isFav = favSet && favSet.has(favId(node.id));
  const nameClass = ['sidebar-node__name',
    depth === 0 ? 'sidebar-node__name--d0' : depth === 1 ? 'sidebar-node__name--d1' : 'sidebar-node__name--dx',
    isSel ? 'sidebar-node__name--selected' : ''].filter(Boolean).join(' ');
  return (
    <div>
      <div onClick={() => onSelect(node.id)}
        className={`sidebar-node__row${isSel ? ' sidebar-node__row--selected' : ''}`}
        style={{ paddingLeft: pad, paddingTop: 5, paddingBottom: 5, paddingRight: 10 }}>
        {ch.length > 0
          ? <span onClick={e => { e.stopPropagation(); toggle(node.id); }} className="sidebar-node__toggle">{isExp ? '▾' : '▸'}</span>
          : <span className="sidebar-node__spacer" />}
        <span className="sidebar-node__dot" style={{ background: dotC }} />
        <span className={nameClass}>{node.name}</span>
        {onToggleFav && <span onClick={e => { e.stopPropagation(); onToggleFav(node.id); }}
          className={`sidebar-node__star${isFav ? ' sidebar-node__star--on' : ''}`}
          title={isFav ? 'Убрать из избранного' : 'В избранное'}>{isFav ? '★' : '☆'}</span>}
        {n > 0 && <span className="sidebar-node__progress" style={{ color: isSel ? '#c4b5fd' : '#64748b' }}>{n}</span>}
      </div>
      {isExp && ch.map(c => <ActivitySidebarNode key={c.id} node={c} depth={depth + 1} selectedId={selectedId} onSelect={onSelect} expanded={expanded} toggle={toggle} counts={counts} favSet={favSet} onToggleFav={onToggleFav} />)}
    </div>
  );
}
```

- [ ] **Step 3: Data + Sidebar wiring in `App`** — replace the F1 stub `App`:
```jsx
function App() {
  const [me, setMe] = useState(null);
  const [periods, setPeriods] = useState([]);
  const [periodId, setPeriodId] = useState(null);   // null = "Все периоды"
  const [hierarchy, setHierarchy] = useState([]);
  const [counts, setCounts] = useState({});
  const [selId, setSelId] = useState(null);          // selected team filter (null = all)
  const [expanded, setExpanded] = useState({});
  const [favorites, setFavorites] = useState(null);
  const [range, setRange] = useState('all');

  useEffect(() => {
    Promise.all([apiGet('/api/v1/me'), apiGet('/api/v1/periods')]).then(([meData, per]) => {
      if (meData) setMe(meData);
      setPeriods((per && per.items) || []);
    });
  }, []);
  useEffect(() => { if (me && favorites === null) setFavorites(readFavorites(me.id)); }, [me, favorites]);
  useEffect(() => { if (me && favorites !== null) writeFavorites(me.id, favorites); }, [favorites, me]);

  // hierarchy for the sidebar tree (period-scoped when a period is chosen)
  useEffect(() => {
    const qs = periodId ? `?period_id=${periodId}` : '';
    apiGet('/api/v1/hierarchy' + qs).then(d => setHierarchy((d && d.items) || []));
  }, [periodId]);

  // tree counts follow period + range only (per design)
  useEffect(() => {
    const p = new URLSearchParams();
    if (periodId) p.set('period_id', periodId);
    if (range && range !== 'all') p.set('range', range);
    apiGet('/api/v1/activity/tree-counts' + (p.toString() ? '?' + p : '')).then(d => setCounts((d && d.counts) || {}));
  }, [periodId, range]);

  const toggle = id => setExpanded(e => ({ ...e, [id]: e[id] === false ? true : false }));
  const favArr = favorites || [];
  const favSet = new Set(favArr);
  const favNodes = collectFavNodes(hierarchy, favArr);
  const onToggleFav = id => setFavorites(f => toggleFavorite(f || [], id));

  return (
    <Sidebar user={me} active="activity-log"
      beforeSections={
        <div className="sidebar__period">
          <div className="sidebar__period-label">Период</div>
          <PeriodSelect periods={[{ id: null, name: 'Все периоды', status: 'active', depth: 0 }, ...periods]}
            periodId={periodId} onChange={id => setPeriodId(id)} />
        </div>
      }>
      <div className="sidebar__tree">
        <div className="sidebar__subsection-label">Команды</div>
        {favNodes.length > 0 && <>
          <div className="sidebar__subsection-label"><span className="sidebar__subsection-star">★</span> Избранное · {favNodes.length}</div>
          {favNodes.map(n => <ActivitySidebarNode key={`fav-${n.id}`} node={{ ...n, children: [] }} depth={0} selectedId={selId} onSelect={setSelId} expanded={expanded} toggle={toggle} counts={counts} favSet={favSet} onToggleFav={onToggleFav} />)}
          <div className="sidebar__subsection-label">Все команды</div>
        </>}
        <div className="sidebar-node__row" onClick={() => setSelId(null)}
          style={{ paddingLeft: 14, cursor: 'pointer', fontWeight: selId === null ? 700 : 500 }}>
          <span className="sidebar-node__spacer" /><span className="sidebar-node__name">Все команды</span>
        </div>
        {hierarchy.map(n => <ActivitySidebarNode key={n.id} node={n} depth={0} selectedId={selId} onSelect={setSelId} expanded={expanded} toggle={toggle} counts={counts} favSet={favSet} onToggleFav={onToggleFav} />)}
      </div>
      <Feed me={me} periodId={periodId} teamId={selId} range={range} setRange={setRange} />
    </Sidebar>
  );
}
```
> `PeriodSelect`'s current code shows `TRK_PERIOD_STATUS[cur.status]`; the injected «Все периоды» pseudo-option uses `status:'active'` so it renders fine. `Feed` is implemented in F3 — add a temporary `function Feed() { return null; }` so F2 compiles.

- [ ] **Step 4: Verify** — reload `/activity-log`: sidebar shows the period selector (with «Все периоды»), the team tree with **numbers** (log counts) next to teams, favorites star toggles and persists (same stars as the tracker). Clicking a team highlights it. Do not commit.

---

### Task F3: Feed — filters, time grouping, event rows, deep links

**Files:**
- Modify: `internal/web/static/activity.js`

**Interfaces:**
- Produces the `Feed` component: top-bar (category tabs, author select, favorites toggle, range, search), time-grouped list from `GET /api/v1/activity`, each row rendering actor + БЫЛО→СТАЛО message + team/period badges + a deep link built by `buildTargetURL`.

- [ ] **Step 1: `buildTargetURL` + message helpers** — add to `activity.js`:
```jsx
function buildTargetURL(target) {
  if (!target) return null;
  const p = new URLSearchParams();
  if (target.team_id) p.set('team', target.team_id);
  if (target.period_id) p.set('period', target.period_id);
  if (target.goal_id) p.set('goal', target.goal_id);
  if (target.kr_id) p.set('kr', target.kr_id);
  if (target.comment_id) p.set('comment', target.comment_id);
  return '/?' + p.toString();   // section 'tracker' → root board
}

const CATEGORY_ICON = { progress: '📈', composition: '🧩', status: '🚦', discussion: '💬' };
const CATEGORY_LABEL = { progress: 'Прогресс', composition: 'Состав целей', status: 'Статусы и риски', discussion: 'Обсуждения' };
const STATUS_RU = { no_goals: 'Нет целей', forming: 'Черновик', ready: 'К валидации', in_progress: 'В работе', validated: 'Валидировано', closed: 'Закрыто' };

// Human БЫЛО→СТАЛО message per action, from the event payload.
function eventText(ev) {
  const p = ev.payload || {};
  const t = ev.entity_title || '';
  switch (ev.action) {
    case 'kr_progress': {
      const b = (p.before || {}).progress, a = (p.after || {}).progress;
      return <>обновил KR «{t}» — <b>{a}%</b> <span className="act-was">(было {b}%)</span></>;
    }
    case 'status_changed': {
      const b = STATUS_RU[(p.before || {}).status] || (p.before || {}).status;
      const a = STATUS_RU[(p.after || {}).status] || (p.after || {}).status;
      return <>перевёл цели команды «{t}» в статус <b>«{a}»</b> <span className="act-was">(было «{b}»)</span></>;
    }
    case 'goal_created':   return <>создал цель «{t}»</>;
    case 'goal_deleted':   return <>удалил цель «{t}»</>;
    case 'kr_created':     return <>добавил KR «{t}»</>;
    case 'kr_deleted':     return <>удалил KR «{t}»</>;
    case 'goal_shared':    return <>расшарил цель «{t}»</>;
    case 'goal_unshared':  return <>отменил шаринг цели «{t}»</>;
    case 'goal_owner_changed': return <>сменил владельца цели «{t}»</>;
    case 'goal_fields_changed': return <>изменил цель «{t}»{fieldsSummary(p.changed)}</>;
    case 'kr_fields_changed':   return <>изменил KR «{t}»{fieldsSummary(p.changed)}</>;
    case 'comment_added':  return <>прокомментировал «{t}»: {String(p.text || '').slice(0, 120)}</>;
    case 'comment_resolved': return <>отметил замечание к «{t}» решённым</>;
    case 'comment_reopened': return <>переоткрыл замечание к «{t}»</>;
    default: return <>{ev.action} «{t}»</>;
  }
}

function fieldsSummary(changed) {
  if (!changed) return null;
  const names = { title: 'название', description: 'описание', weight: 'вес', priority: 'приоритет' };
  const keys = Object.keys(changed).map(k => names[k] || k);
  return keys.length ? <span className="act-was"> ({keys.join(', ')})</span> : null;
}
```
> All interpolated strings (`t`, `p.text`) render as React text nodes — safely escaped, per the escaping constraint.

- [ ] **Step 2: Time grouping helper**:
```jsx
function groupByTime(events) {
  const now = new Date();
  const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime();
  const startOfYesterday = startOfToday - 864e5;
  const startOfWeek = startOfToday - 6 * 864e5;
  const groups = { today: [], yesterday: [], week: [], older: [] };
  events.forEach(ev => {
    const t = new Date(ev.created_at).getTime();
    if (t >= startOfToday) groups.today.push(ev);
    else if (t >= startOfYesterday) groups.yesterday.push(ev);
    else if (t >= startOfWeek) groups.week.push(ev);
    else groups.older.push(ev);
  });
  return [
    ['СЕГОДНЯ', groups.today], ['ВЧЕРА', groups.yesterday],
    ['РАНЕЕ НА ЭТОЙ НЕДЕЛЕ', groups.week], ['РАНЕЕ', groups.older],
  ].filter(([, list]) => list.length > 0);
}
```

- [ ] **Step 3: The `Feed` component** — replace the F2 temporary stub:
```jsx
function Feed({ me, periodId, teamId, range, setRange }) {
  const [events, setEvents] = useState([]);
  const [nextCursor, setNextCursor] = useState('');
  const [category, setCategory] = useState('');   // '' = Все
  const [actorUDID, setActorUDID] = useState('');
  const [favOnly, setFavOnly] = useState(false);
  const [q, setQ] = useState('');
  const [loading, setLoading] = useState(false);

  function buildQuery(cursor) {
    const p = new URLSearchParams();
    if (periodId) p.set('period_id', periodId);
    if (teamId) p.append('team_ids', teamId);
    if (category) p.set('category', category);
    if (actorUDID) p.set('actor_udid', actorUDID);
    if (range && range !== 'all') p.set('range', range);
    if (q.trim()) p.set('q', q.trim());
    if (cursor) p.set('cursor', cursor);
    return p.toString();
  }

  useEffect(() => {
    setLoading(true);
    apiGet('/api/v1/activity?' + buildQuery('')).then(d => {
      setEvents((d && d.items) || []);
      setNextCursor((d && d.next_cursor) || '');
      setLoading(false);
    });
  }, [periodId, teamId, category, actorUDID, range, q]);

  const loadMore = () => {
    if (!nextCursor) return;
    apiGet('/api/v1/activity?' + buildQuery(nextCursor)).then(d => {
      setEvents(prev => [...prev, ...((d && d.items) || [])]);
      setNextCursor((d && d.next_cursor) || '');
    });
  };

  // author options from currently-loaded, non-removed actors
  const authors = [];
  const seen = new Set();
  events.forEach(ev => { if (ev.actor && ev.actor.udid && !ev.actor.removed && !seen.has(ev.actor.udid)) { seen.add(ev.actor.udid); authors.push(ev.actor); } });

  const shown = favOnly ? events.filter(ev => ev.team_id && (favorites_has(me, ev.team_id))) : events;
  const groups = groupByTime(shown);
  const counts = {};
  events.forEach(ev => { counts[ev.category] = (counts[ev.category] || 0) + 1; });

  return (
    <div className="act-main">
      <div className="act-topbar">
        <div className="act-tabs">
          <button className={`act-tab${category === '' ? ' act-tab--on' : ''}`} onClick={() => setCategory('')}>Все <span className="act-tab__n">{events.length}</span></button>
          {['progress', 'composition', 'status', 'discussion'].map(c => (
            <button key={c} className={`act-tab${category === c ? ' act-tab--on' : ''}`} onClick={() => setCategory(c)}>
              {CATEGORY_ICON[c]} {CATEGORY_LABEL[c]} <span className="act-tab__n">{counts[c] || 0}</span>
            </button>
          ))}
        </div>
        <div className="act-filters">
          <select className="act-select" value={actorUDID} onChange={e => setActorUDID(e.target.value)}>
            <option value="">Все авторы</option>
            {authors.map(a => <option key={a.udid} value={a.udid}>{a.display_name}</option>)}
          </select>
          <button className={`act-chip${favOnly ? ' act-chip--on' : ''}`} onClick={() => setFavOnly(v => !v)}>★ Избранное</button>
          <div className="act-range">
            {[['all', 'Всё время'], ['today', 'Сегодня'], ['7d', '7 дней'], ['30d', '30 дней']].map(([v, l]) => (
              <button key={v} className={`act-range__btn${range === v ? ' act-range__btn--on' : ''}`} onClick={() => setRange(v)}>{l}</button>
            ))}
          </div>
          <input className="act-search" placeholder="Поиск по событиям…" value={q} onChange={e => setQ(e.target.value)} />
        </div>
      </div>
      <div className="act-feed">
        {loading && events.length === 0 && <div className="act-empty">Загрузка…</div>}
        {!loading && shown.length === 0 && <div className="act-empty">Событий нет</div>}
        {groups.map(([label, list]) => (
          <div key={label} className="act-group">
            <div className="act-group__label">{label}</div>
            {list.map(ev => <EventRow key={ev.id} ev={ev} />)}
          </div>
        ))}
        {nextCursor && <button className="act-more" onClick={loadMore}>Показать ещё</button>}
      </div>
    </div>
  );
}

function favorites_has(me, teamId) {
  if (!me) return false;
  return readFavorites(me.id).includes(favId(teamId));
}

function EventRow({ ev }) {
  const url = buildTargetURL(ev.target);
  const actor = ev.actor || {};
  const name = actor.removed ? 'Бывший участник' : (actor.display_name || '—');
  return (
    <div className="act-row">
      <div className={`act-row__icon act-row__icon--${ev.category}`}>{CATEGORY_ICON[ev.category]}</div>
      <div className="act-row__body">
        <div className="act-row__text"><b className="act-row__actor">{name}</b> {eventText(ev)}</div>
        <div className="act-row__meta">
          {ev.team_id && <span className="act-badge">команда #{ev.team_id}</span>}
          {ev.period_id && <span className="act-badge act-badge--period">период #{ev.period_id}</span>}
          {url && <a className="act-row__link" href={url}>↗ к цели</a>}
        </div>
      </div>
    </div>
  );
}
```
> The team/period **names** on badges: the sidebar already has the hierarchy and periods; a follow-up can pass name maps into `Feed` to show «Платформа» / «Q1 · 2026» instead of ids. For this task, ids are acceptable — wire the name maps if the hierarchy/period props are threaded in (optional polish, note it, don't block).

- [ ] **Step 4: Verify** — reload `/activity-log` against a seeded DB: the feed renders grouped by time; category tabs filter and show counts; author dropdown lists present authors; range buttons filter; search filters; clicking a team in the sidebar filters; «Показать ещё» paginates; each row shows a БЫЛО→СТАЛО line and a «↗ к цели» link. Removed authors render «Бывший участник» with no avatar. Do not commit.

---

### Task F4: `activity.css`

**Files:**
- Modify: `internal/web/static/activity.css`

- [ ] **Step 1: Style the page** — write CSS for `.act-main`, `.act-topbar`, `.act-tabs`/`.act-tab`/`.act-tab--on`/`.act-tab__n`, `.act-filters`, `.act-select`, `.act-chip`/`--on`, `.act-range`/`__btn`/`--on`, `.act-search`, `.act-feed`, `.act-group`/`__label`, `.act-row`/`__icon`/`__body`/`__text`/`__actor`/`__meta`/`__link`, `.act-badge`/`--period`, `.act-was`, `.act-more`, `.act-empty`. Reuse the CSS variables from `tokens.css` (accent, radii) and match the tracker's visual language (card list, muted group labels, pill filters) so the page looks native. Keep the feed column scroll inside `.act-feed` (the sidebar has its own scroll region).

- [ ] **Step 2: Verify** — reload; the page matches the mockup layout (left dark sidebar, right feed with top filter bar). Do not commit.

---

# Phase G — Tracker deep-linking

### Task G1: `?goal=`/`?kr=`/`?comment=` deep links in the tracker

**Files:**
- Modify: `internal/web/static/tracker.js`

**Interfaces:**
- Extends `readURLNav`/`updateURL` with `goal`/`kr`/`comment`; adds DOM anchors `goal-<id>`/`kr-<id>`/`comment-<id>`; opens KR list / comments when targeted; scrolls to the target after render.

- [ ] **Step 1: Extend URL read/write** — in `tracker.js` `readURLNav` (line 22-27) also read the anchors, and fix `updateURL` to use the current path and preserve/clear anchors:
```jsx
function readURLNav() {
  const p = new URLSearchParams(location.search);
  const num = k => (p.get(k) ? Number(p.get(k)) : null);
  const team = num('team'), period = num('period');
  return {
    team: Number.isFinite(team) ? team : null,
    period: Number.isFinite(period) ? period : null,
    goal: num('goal'), kr: num('kr'), comment: num('comment'),
  };
}
```
In `updateURL` (line 38-45) change the base path and stop dropping anchors on navigation:
```jsx
function updateURL(teamId, periodId, replace = false) {
  const p = new URLSearchParams();
  if (teamId) p.set('team', teamId);
  if (periodId) p.set('period', periodId);
  const qs = p.toString();
  const url = location.pathname + (qs ? '?' + qs : '');  // was hardcoded '/'
  if (replace) history.replaceState(null, '', url);
  else history.pushState(null, '', url);
}
```
> `updateURL` intentionally omits `goal`/`kr`/`comment` — after the deep-link scroll fires once, subsequent team/period changes write a clean URL. That is the desired behavior (the anchor is a one-shot).

- [ ] **Step 2: Add DOM anchors** — add ids to the three targets (they currently have none):
  - GoalCard root div (tracker.js:1113): add `id={`goal-${goal.id}`}`.
  - KR wrapper div (tracker.js:1208, the `<div key={kr.id} ...>`): add `id={`kr-${kr.id}`}`.
  - `CommentRow` root (tracker.js:997): add `id={`comment-${c.id}`}` (confirm the comment prop name is `c`).

- [ ] **Step 3: Open + scroll to the target after load** — in the tracker `App`, read the deep-link once and, after the goals for the selected team render, force-open and scroll. Reuse the existing HCI scroll precedent (tracker.js:2350-2352). Add near the initial-nav ref (tracker.js:2031-2039) a `deepLinkRef` capturing `{ goal, kr, comment }` from `readURLNav()`, then an effect that fires after goals load:
```jsx
useEffect(() => {
  const dl = deepLinkRef.current;
  if (!dl || (!dl.goal && !dl.kr && !dl.comment)) return;
  // wait for goal cards to render
  const id = dl.comment ? `comment-${dl.comment}` : dl.kr ? `kr-${dl.kr}` : dl.goal ? `goal-${dl.goal}` : null;
  if (!id) return;
  const timer = setTimeout(() => {
    const el = document.getElementById(id);
    if (el) { el.scrollIntoView({ behavior: 'smooth', block: 'center' }); el.classList.add('deep-link-flash'); }
    deepLinkRef.current = null;
  }, 500);
  return () => clearTimeout(timer);
}, [/* the goals-loaded signal, e.g. */ goalsForSelectedTeam]);
```
For `kr`/`comment` targets the containing GoalCard must have its KR list / comments open. Simplest robust approach: pass the deep-link target into `GoalCard` and initialize `showKR`/`showCom` open when this card is the target:
```jsx
// GoalCard (tracker.js:1069-1070) initial state, using a `deepLink` prop:
const [showKR, setShowKR] = useState((goal.krs || []).length === 0 || (deepLink && deepLink.goal === goal.id && !!deepLink.kr));
const [showCom, setShowCom] = useState(!!(deepLink && deepLink.goal === goal.id && deepLink.comment));
```
Thread `deepLink={deepLinkRef.current}` from where GoalCards are rendered. (Confirm the goal→card render site and pass the prop.)

- [ ] **Step 4: Add a flash style** — in `tracker.css` add a brief highlight so the user sees the target:
```css
.deep-link-flash { animation: deepLinkFlash 1.6s ease-out; }
@keyframes deepLinkFlash { 0% { box-shadow: 0 0 0 3px var(--accent, #7c3aed); } 100% { box-shadow: 0 0 0 0 transparent; } }
```

- [ ] **Step 5: Verify** — from `/activity-log`, click «↗ к цели» on a progress event → tracker opens at the right team/period, scrolls to the goal card (and expands the KR / opens comments when the link carried `kr`/`comment`), with a brief highlight. Existing tracker navigation (no anchor) is unchanged. Do not commit.

---

# Phase H — Purge UI (admin + system)

### Task H1: Admin — `ActivityLogPanel`

**Files:**
- Modify: `internal/web/static/admin.js`

**Interfaces:**
- Adds a card in the existing «Настройки» section: depth `<select>` (Старше квартала / Старше года / Всё) + destructive button + native `confirm()` → `POST /api/v1/admin/activity/purge`.

- [ ] **Step 1: Add the panel** — in `admin.js`, mirror `GeneralSettingsPanel` (admin.js:1052-1109) and `AccessSettingsPanel`'s `<select>` (1033-1037):
```jsx
function ActivityLogPanel() {
  const [depth, setDepth] = useState('quarter');
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState('');
  const labels = { quarter: 'старше квартала', year: 'старше года', all: 'все' };
  async function purge() {
    if (!confirm(`Удалить логи активности (${labels[depth]})? Действие необратимо.`)) return;
    setBusy(true); setMsg('');
    const res = await apiPost('/api/v1/admin/activity/purge', { older_than: depth });
    setBusy(false);
    if (res && res.ok) { const j = await res.json(); setMsg(`Удалено записей: ${j.deleted}`); }
    else if (res && res.status === 422) setMsg('Неверная глубина очистки');
    else setMsg('Ошибка очистки');
  }
  return <div>
    <DetailHeader breadcrumb="Настройки" title="Лог активности" subtitle="Очистка накопленных событий" />
    <DetailSection title="Очистить лог активности">
      <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
        <select value={depth} onChange={e => setDepth(e.target.value)} style={{ ...inpStyle, fontSize: 13, cursor: 'pointer' }}>
          <option value="quarter">Старше квартала</option>
          <option value="year">Старше года</option>
          <option value="all">Всё</option>
        </select>
        <Btn danger onClick={purge} disabled={busy}>{busy ? 'Очистка…' : 'Очистить'}</Btn>
        {msg && <span style={{ fontSize: 12, color: '#6b7280', fontWeight: 600 }}>{msg}</span>}
      </div>
    </DetailSection>
  </div>;
}
```

- [ ] **Step 2: Render it** — in the settings render block (admin.js:1565-1569) add a card:
```jsx
  <div style={{ background: 'white', borderRadius: 12, border: '1px solid ' + T.cardBorder, boxShadow: '0 1px 3px rgba(15,23,42,0.04)', overflow: 'hidden' }}><ActivityLogPanel /></div>
```

- [ ] **Step 3: Verify** — open `/admin` → Настройки; the «Лог активности» card appears; selecting a depth + «Очистить» prompts a confirm, then shows «Удалено записей: N». Non-admins never reach `/admin` (middleware). Do not commit.

---

### Task H2: System — per-tenant purge control

**Files:**
- Modify: `internal/web/static/system.js`

**Interfaces:**
- Adds a purge control to `TenantsSection` (system.js:35-87), calling `POST /api/v1/system/tenants/{id}/activity/purge`.

- [ ] **Step 1: Add per-row purge** — in the tenants table action `<td>` (system.js:71-77), add a small depth `<select>` + button, and a handler on `TenantsSection`:
```jsx
  const [purgeDepth, setPurgeDepth] = useState({}); // {tenantId: depth}
  const purge = async (id) => {
    const depth = purgeDepth[id] || 'quarter';
    const labels = { quarter: 'старше квартала', year: 'старше года', all: 'все' };
    if (!confirm(`Очистить лог активности пространства #${id} (${labels[depth]})? Необратимо.`)) return;
    setErr('');
    const res = await post(`/api/v1/system/tenants/${id}/activity/purge`, { older_than: depth });
    if (res && res.status === 200) { const j = await res.json(); setErr(`Пространство #${id}: удалено ${j.deleted}`); }
    else setErr(await errMsg(res));
  };
```
In the action `<td>` add:
```jsx
  <select style={{ ...inp, padding: '4px 6px' }} value={purgeDepth[t.id] || 'quarter'} onChange={e => setPurgeDepth(d => ({ ...d, [t.id]: e.target.value }))}>
    <option value="quarter">Кв.</option><option value="year">Год</option><option value="all">Всё</option>
  </select>
  <button style={{ ...btn, background: C.danger }} onClick={() => purge(t.id)}>Очистить лог</button>
```
> `setErr` here doubles as a status line (the section already renders `{err && ...}`). If you prefer a separate success color, add a `setNote` state; matching the existing minimal pattern, reusing `err` is acceptable.

- [ ] **Step 2: Verify** — open `/system` → Пространства; each tenant row has a depth select + «Очистить лог»; confirm + purge shows the deleted count; other tenants unaffected. Do not commit.

---

# Phase I — Spec + end-to-end verification

### Task I1: Update `030-user-flows.md` + full verification

**Files:**
- Modify: `specs/030-user-flows.md`

- [ ] **Step 1: Update the spec** — change the stub note (currently «`/activity-log` … Реальная функциональность — вне текущего scope») to describe the real page: sidebar (period incl. «Все периоды», sections, favorites, team tree with **log counts**), top-bar filters (category tabs, author, favorites, range, search), time-grouped feed with БЫЛО→СТАЛО and deep links. Document the tracker deep-link params `goal`/`kr`/`comment`. Document the admin + system «Очистить лог активности» controls.

- [ ] **Step 2: End-to-end verification** (use the `/verify` skill or manual): with the Plan-1 backend running and seed loaded —
  1. Perform each mutation type in the tracker (update KR progress, change team status, add/resolve a comment, create/delete a goal/KR, share a goal, edit a field) and confirm a matching event appears in `/activity-log`.
  2. Confirm БЫЛО→СТАЛО renders and «↗ к цели» deep-links land on the goal (and open KR/comments where applicable).
  3. Confirm period selector, category tabs, author, favorites, range, search all filter; team click filters; tree counts roll up.
  4. Confirm a sharee-team user sees a shared goal's events (share-aware); an unrelated team's user does not.
  5. Confirm admin purge and system purge remove events and report the count.

- [ ] **Step 3: Checkpoint** — `go build ./... && go test ./...` green; app verified. Do not commit.

---

## Self-Review (completed)

- **Spec coverage vs design doc:** activity page + sidebar with counts (F1–F4) ✓; period filter incl. «Все периоды» (F2) ✓; category/author/favorites/range/search filters (F3) ✓; time grouping + БЫЛО→СТАЛО + badges + deep links (F3) ✓; structured `target` → `buildTargetURL` (F3) ✓; tracker deep-link `goal`/`kr`/`comment` (G1) ✓; admin purge UI (H1) + system purge UI (H2) ✓; share-aware + removed-actor handled by the Plan-1 API and rendered in F3 (`Бывший участник`, no avatar) ✓; spec `030` (I1) ✓.
- **Consistency:** the sidebar/period/favorites/tree are copied from `tracker.js` with identical CSS classes; purge controls reuse native `confirm()` + the per-file button/select conventions (admin `Btn danger`/`inpStyle`; system `btn`/`inp`) exactly as the existing controls do.
- **Escaping:** every user string renders as a React text node; markdown via the shared sanitizing `Markdown`. No raw HTML insertion.
- **Depends on Plan 1** — the API endpoints, `target` shape, and removed-actor masking all come from Plan 1; this plan is UI-only.
