# Copy Goal Link Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a "copy link" icon to each goal card that copies a shareable deep-link URL, and make opening any such link expand the sidebar team tree to reveal the target team.

**Architecture:** Pure frontend change in the tracker SPA (`tracker.js` + `tracker.css`). The copy-link reuses the existing `buildTargetURL` helper (`ui.js`) that the activity log and Health Check-in bell already use, so the URL shape and open behavior are identical to the "↗ к цели" link. A separate helper expands the target team's ancestors on URL-driven init, closing an existing gap shared by the activity-log deep-link.

**Tech Stack:** React (in-browser Babel transpile, no build step, no JS test harness), served static by the Go server (`go run ./cmd/server`).

## Global Constraints

- Frontend-only: no Go changes, no new endpoints, no DB/schema changes, no seed changes.
- Reuse the existing deep-link convention (`buildTargetURL` from `ui.js`, `readURLNav` in `tracker.js`); do NOT invent a second URL format.
- `ui.js` is loaded before `tracker.js` in global scope, so `buildTargetURL` is available unqualified inside `tracker.js`.
- No JS unit-test harness exists; the frontend is transpiled in-browser via Babel. JSX means `node --check` cannot lint these files. Verification is `go build ./...` (server intact) plus manual browser verification. Do not fabricate automated test steps.
- Do not mention AI/assistant/generated-by attribution in any code, comment, doc, or spec text.
- Follow existing `tracker.js`/`tracker.css` naming conventions (`goal-card__*` BEM-ish classes, camelCase JS).
- No git commits — the repository owner commits manually. "Commit" steps below are omitted intentionally; stop at a clean, verified working tree per task.

---

### Task 1: Sidebar tree reveals the target team on URL open

**Files:**
- Modify: `internal/web/static/tracker.js` (add `findAncestorIds` helper near `findNodeById`; expand ancestors in the hierarchy-load effect; track whether the initial team came from the URL)

**Interfaces:**
- Produces: `findAncestorIds(nodes, id)` → `number[]` — ids of all ancestors of the node with `id` (excluding the node itself); `[]` if not found or top-level.
- Consumes: existing `initialNavRef`, `setExpanded`, hierarchy-load effect (`useEffect(..., [periodId])` around `tracker.js:2191`).

**Context for the implementer:**
- Tree expansion state lives in `expanded` (a `useState(readTreeExpanded)` map). Semantics: **absence ≡ expanded**; only explicitly collapsed nodes are stored as `false` (see comment at `tracker.js:2270`). So to force-reveal a node, set each ancestor id to `true` (overriding a stored `false`).
- The current team is always rendered even under a collapsed parent (`filterTreeForSidebar` keeps `currentId`), but a collapsed ancestor hides it visually in the tree. Expanding ancestors is what actually reveals it.
- We only want this on URL-originated navigation (a shared/deep link), NOT on cookie-restored reopen. `initialNavRef.current.team` currently mixes `url.team || cookie.team`; add a flag to remember the URL case.

- [ ] **Step 1: Track whether the initial team came from the URL**

In `tracker.js`, find the `initialNavRef` initializer (around `tracker.js:2142`):

```javascript
  const initialNavRef = useRef(null);
  if (initialNavRef.current === null) {
    const url = readURLNav();
    const cookie = readLastNav();
    initialNavRef.current = {
      team: url.team || cookie.team || null,
      period: url.period || cookie.period || null,
      used: false,
    };
  }
```

Replace it with (adds `fromUrl`):

```javascript
  const initialNavRef = useRef(null);
  if (initialNavRef.current === null) {
    const url = readURLNav();
    const cookie = readLastNav();
    initialNavRef.current = {
      team: url.team || cookie.team || null,
      period: url.period || cookie.period || null,
      fromUrl: !!url.team, // team came from a shared/deep link → reveal it in the tree
      used: false,
    };
  }
```

- [ ] **Step 2: Add the `findAncestorIds` helper**

In `tracker.js`, immediately after the `findNodeById` function (around `tracker.js:2265-2268`):

```javascript
  function findNodeById(nodes, id) {
    for (const n of nodes) { if (n.id === id) return n; const f = findNodeById(n.children || [], id); if (f) return f; }
    return null;
  }
```

add:

```javascript
  // Ancestor ids of the node with `id` (excluding the node itself), root→parent order.
  // Used to force-expand a collapsed tree so a deep-linked team becomes visible.
  function findAncestorIds(nodes, id) {
    const path = [];
    const walk = (list, trail) => {
      for (const n of (list || [])) {
        if (n.id === id) { path.push(...trail); return true; }
        if (walk(n.children, [...trail, n.id])) return true;
      }
      return false;
    };
    walk(nodes, []);
    return path;
  }
```

- [ ] **Step 3: Expand ancestors when resolving the initial team from the URL**

In `tracker.js`, find the hierarchy-load effect body (around `tracker.js:2200-2208`):

```javascript
      if (!selId || !findNodeById(nodes, selId)) {
        let target = null;
        if (!initialNavRef.current.used && initialNavRef.current.team) {
          target = findNodeById(nodes, initialNavRef.current.team) || null;
        }
        initialNavRef.current.used = true;
        if (!target) target = findFirstNode(nodes);
        if (target) setSelId(target.id);
      }
```

Replace with:

```javascript
      if (!selId || !findNodeById(nodes, selId)) {
        let target = null;
        if (!initialNavRef.current.used && initialNavRef.current.team) {
          target = findNodeById(nodes, initialNavRef.current.team) || null;
          // Opened via a shared/deep link: reveal the target team by expanding its
          // ancestors, overriding any stored collapsed state. Same behavior the
          // activity-log "↗ к цели" link now gets.
          if (target && initialNavRef.current.fromUrl) {
            const anc = findAncestorIds(nodes, target.id);
            if (anc.length) setExpanded(m => { const next = { ...m }; anc.forEach(aid => { next[aid] = true; }); return next; });
          }
        }
        initialNavRef.current.used = true;
        if (!target) target = findFirstNode(nodes);
        if (target) setSelId(target.id);
      }
```

- [ ] **Step 4: Verify the server still builds**

Run: `go build ./...`
Expected: no output, exit 0 (we changed only static JS, but this confirms nothing else broke).

- [ ] **Step 5: Manual browser verification**

Start the app: `go run ./cmd/server` (seed data, `AUTH_MODE=disabled` default per README).
1. Open the tracker, collapse a parent team in the sidebar so a nested child is hidden.
2. Note the nested child's team id and current period id from its URL when selected.
3. Open a new tab at `/?team=<nestedChildId>&period=<periodId>` (or `&goal=<id>`).
4. Expected: the sidebar tree is expanded down to the nested child, and the child is selected/highlighted — not hidden under the collapsed parent.
5. Reopen the app plainly (no `?team=` in URL) after collapsing a parent whose child was your last-visited team. Expected: the collapse state is preserved (cookie-restore does NOT force-expand).

---

### Task 2: Copy-link button on the goal card

**Files:**
- Modify: `internal/web/static/tracker.js` (add `CopyLinkButton` component; thread `periodId` into `GoalCard`; render the button in the meta row)
- Modify: `internal/web/static/tracker.css` (styles for `.goal-card__copy-link`)

**Interfaces:**
- Consumes: `buildTargetURL(target)` from `ui.js` → returns `"/?team=&period=&goal=..."` or `null`; `currentTeamId` and new `periodId` props on `GoalCard`.
- Produces: `CopyLinkButton({ teamId, periodId, goalId })` React component rendered inside `.goal-card__meta`.

**Context for the implementer:**
- `GoalCard` is rendered inside `App()` (single component) at `tracker.js:2456`; `periodId` and `selId` are in scope there.
- `GoalCard`'s signature is at `tracker.js:1085`. It already receives `currentTeamId={selId}`.
- The meta row is `.goal-card__meta` (`tracker.js:1142-1156`); a `.goal-card__spacer` (`flex:1`) right-aligns the trailing group (stale badge + owner). The button goes after the owner block so it sits to the right of the driver name; the spacer keeps it in the same right-hand spot whether or not a driver is present.
- The URL must be absolute so it's shareable: `location.origin + buildTargetURL(...)`.
- For a shared goal, use `teamId` = the currently-open team (`currentTeamId`), never the owner team.

- [ ] **Step 1: Add the `CopyLinkButton` component**

In `tracker.js`, immediately before `function GoalCard(` (around `tracker.js:1085`), add:

```javascript
// Copy a shareable deep-link to this goal. URL shape and open behavior match the
// activity-log "↗ к цели" link (shared buildTargetURL from ui.js). For a shared goal
// the link points at the currently-open team, so it resolves back to the same board.
function CopyLinkButton({ teamId, periodId, goalId }) {
  const [copied, setCopied] = useState(false);
  const copy = async e => {
    e.stopPropagation();
    const path = buildTargetURL({ team_id: teamId, period_id: periodId, goal_id: goalId });
    if (!path) return;
    const url = location.origin + path;
    try {
      if (navigator.clipboard && navigator.clipboard.writeText) {
        await navigator.clipboard.writeText(url);
      } else {
        const ta = document.createElement('textarea');
        ta.value = url; ta.style.position = 'fixed'; ta.style.opacity = '0';
        document.body.appendChild(ta); ta.focus(); ta.select();
        document.execCommand('copy'); document.body.removeChild(ta);
      }
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch { /* silent: leave icon unchanged on failure */ }
  };
  return (
    <button type="button" onClick={copy}
      className={`goal-card__copy-link${copied ? ' goal-card__copy-link--copied' : ''}`}
      title={copied ? 'Скопировано' : 'Скопировать ссылку на цель'}
      aria-label="Скопировать ссылку на цель">
      {copied ? '✓' : '🔗'}
    </button>
  );
}
```

- [ ] **Step 2: Add `periodId` to the `GoalCard` signature**

In `tracker.js:1085`, change the destructured props from:

```javascript
function GoalCard({ goal, editMode, onReload, onEditGoal, me, accent, currentTeamId, allTeams, dragProps, onReorderKR, staleDays = 7, periodStatus, greenThreshold = 80, deepLink = null }) {
```

to (adds `periodId`):

```javascript
function GoalCard({ goal, editMode, onReload, onEditGoal, me, accent, currentTeamId, periodId, allTeams, dragProps, onReorderKR, staleDays = 7, periodStatus, greenThreshold = 80, deepLink = null }) {
```

- [ ] **Step 3: Render the button in the meta row**

In `tracker.js`, find the owner block in the meta row (`tracker.js:1148-1155`):

```javascript
          {goal.owners.length > 0 && (
            <div className="goal-card__owner">
              <span className="goal-card__owner-label">Драйвер цели</span>
              {goal.owners.map(u => (
                <UserInfo key={u.udid || u.display_name} userRef={u} size={18} />
              ))}
            </div>
          )}
```

Add the button immediately after that closing `)}` (still inside `.goal-card__meta`):

```javascript
          {goal.owners.length > 0 && (
            <div className="goal-card__owner">
              <span className="goal-card__owner-label">Драйвер цели</span>
              {goal.owners.map(u => (
                <UserInfo key={u.udid || u.display_name} userRef={u} size={18} />
              ))}
            </div>
          )}
          <CopyLinkButton teamId={currentTeamId} periodId={periodId} goalId={goal.id} />
```

Note: when there is no driver, the owner block is absent but `.goal-card__spacer` still right-aligns `CopyLinkButton`, so it stays in the same spot.

- [ ] **Step 4: Pass `periodId` where `GoalCard` is rendered**

In `tracker.js:2456`, find the render:

```javascript
          {goals.map(g => <GoalCard key={g.id} goal={g} editMode={editMode} onReload={reload} onEditGoal={setGoalModal} me={me} accent={accent} currentTeamId={selId} allTeams={hierarchy} staleDays={staleDays} periodStatus={status} greenThreshold={greenThreshold} deepLink={deepLinkRef.current}
```

Change the props to add `periodId={periodId}` (right after `currentTeamId={selId}`):

```javascript
          {goals.map(g => <GoalCard key={g.id} goal={g} editMode={editMode} onReload={reload} onEditGoal={setGoalModal} me={me} accent={accent} currentTeamId={selId} periodId={periodId} allTeams={hierarchy} staleDays={staleDays} periodStatus={status} greenThreshold={greenThreshold} deepLink={deepLinkRef.current}
```

- [ ] **Step 5: Add CSS for the button**

In `tracker.css`, after the `.goal-card__owner-label` rule (`tracker.css:360`), add:

```css
.goal-card__copy-link { flex-shrink: 0; display: inline-flex; align-items: center; justify-content: center; width: 26px; height: 26px; padding: 0; border: 1px solid #e5e7eb; border-radius: 7px; background: #fff; color: #6b7280; font-size: 13px; line-height: 1; cursor: pointer; transition: background .12s, border-color .12s, color .12s; }
.goal-card__copy-link:hover { background: #f9fafb; border-color: #d1d5db; color: #374151; }
.goal-card__copy-link--copied { border-color: #86efac; background: #f0fdf4; color: #16a34a; }
```

- [ ] **Step 6: Verify the server still builds**

Run: `go build ./...`
Expected: no output, exit 0.

- [ ] **Step 7: Manual browser verification**

With `go run ./cmd/server` running:
1. Open a team with goals. Confirm a 🔗 button sits at the right end of each goal card's top meta row — right of the driver ("Драйвер цели") when present, and in the same spot on a goal that has no driver.
2. Click it. Expected: icon briefly turns into a green ✓ ("Скопировано") for ~1.5s, then back to 🔗.
3. Paste the clipboard into a new tab. Expected URL shape: `https://<host>/?team=<currentTeamId>&period=<periodId>&goal=<goalId>`. Opening it selects that team/period, scrolls to and flashes the goal — identical to the activity-log "↗ к цели" behavior.
4. Open a **shared** goal from a partner team (not the owner). Copy its link. Expected: `team=` is the currently-open team's id (not the owner team's), and opening the link returns to this same team's board.
5. Combined with Task 1: copy a link for a team nested under a collapsed parent, open it in a fresh tab → the tree expands to reveal that team.

---

### Task 3: Update the user-flows spec

**Files:**
- Modify: `internal/../specs/030-user-flows.md` (repo path: `specs/030-user-flows.md`) — document the copy-link control and the deep-link tree-reveal behavior.

**Context for the implementer:**
- CLAUDE.md requires specs to stay in sync in the same change set. Two edits: §4 "Карточка цели" (new affordance) and §3д (deep-link now reveals the team in the tree).

- [ ] **Step 1: Document the copy-link control in §4 "Карточка цели"**

In `specs/030-user-flows.md`, find the "Карточка цели" bullet list (around line 240-248). After the bullet:

```
- заголовок, приоритет (P0–P3), вес, тип работы (Delivery/Discovery), фокус, драйвер цели;
```

add a new bullet:

```
- в правом конце мета-строки карточки — иконка «Скопировать ссылку на цель» (🔗), всегда на одной линии справа от драйвера цели (если драйвера нет — в том же месте). Клик копирует абсолютную deep-ссылку `/?team=&period=&goal=` (та же конвенция и поведение, что и «↗ к цели» из лога активностей); для общей (расшаренной) цели ссылка ведёт в текущую открытую команду, а не в команду-владельца. После копирования иконка кратко меняется на ✓ «Скопировано». Клиентская операция без обращений к серверу; не зависит от статуса периода, роли и auth mode.
```

- [ ] **Step 2: Document tree-reveal in §3д (Deep-link в трекер)**

In `specs/030-user-flows.md`, find the "Deep-link в трекер" paragraph (around line 198). At the end of that paragraph, after the sentence ending "…параметры аддитивны, `updateURL` пишет в текущий `location.pathname`.", append:

```
При открытии deep-link (из лога активностей или по скопированной с карточки цели ссылке) сайдбар дополнительно раскрывает предков целевой команды, чтобы она была видна в дереве, даже если её родитель был свёрнут пользователем; раскрытие применяется только для команды, заданной в URL (не для восстановления из cookie `okr_last`).
```

- [ ] **Step 3: Verify no other spec references contradict**

Run: `rg -n "скопировать ссылку|copy.link|раскрыва.* предк" specs/030-user-flows.md`
Expected: matches only in the two spots you just edited; no contradictions elsewhere.

---

## Self-Review

**Spec coverage** (against `docs/superpowers/specs/2026-07-19-copy-goal-link-design.md`):
- URL via `buildTargetURL`, absolute, shared-goal → currentTeam → Task 2 (Steps 1, 3–4).
- Placement right of driver, always present → Task 2 (Steps 3, 5).
- Copy feedback ✓ inline + execCommand fallback → Task 2 (Step 1).
- Thread `periodId` into `GoalCard` → Task 2 (Steps 2, 4).
- Equivalence to activity-log link → Task 2 (reuses `buildTargetURL`; Step 7.3 verifies).
- Tree reveal on open (`findAncestorIds`, URL-only) → Task 1 (Steps 1–3).
- Spec updates §4 + §3д → Task 3.
- Frontend-only, no API/DB/tests → Global Constraints; build check in each task.

**Placeholder scan:** No TBD/TODO; all code shown in full; verification steps are concrete commands/observations (no fabricated automated tests — none exist for this layer).

**Type consistency:** `findAncestorIds(nodes, id)` defined in Task 1 and used only there. `CopyLinkButton({ teamId, periodId, goalId })` defined and consumed in Task 2; prop `periodId` added to `GoalCard` signature (Step 2) and passed at render (Step 4). `buildTargetURL({ team_id, period_id, goal_id })` matches the `ui.js` signature (snake_case keys) verified at `ui.js:15-25`.
