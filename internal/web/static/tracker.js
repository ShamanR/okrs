const { useState, useCallback, useRef, useEffect } = React;
// ACCENT, TEAM_TYPE_* — общие константы из ui.js (грузится раньше).

// ── API ───────────────────────────────────────────────────────────────────────
// readCSRF / csrfHeaders — общие глобали из api.js (грузится раньше).
async function apiFetch(url, opts = {}) {
  const r = await fetch(url, opts);
  if (!r.ok) {
    if (r.status === 401) { location.href = '/login?next=' + encodeURIComponent(location.pathname); return null; }
    throw new Error(`HTTP ${r.status}`);
  }
  return r.status === 204 ? null : r.json();
}
const apiGet = url => apiFetch(url);
const apiPost = (url, body) => apiFetch(url, { method: 'POST', headers: csrfHeaders(), body: JSON.stringify(body) });
const apiDelete = url => apiFetch(url, { method: 'DELETE', headers: csrfHeaders() });
function apiForm(url, fd) {
  return apiFetch(url, { method: 'POST', headers: { 'X-CSRF-Token': readCSRF() }, body: fd });
}

// ── NAV PERSISTENCE (URL + cookie) ───────────────────────────────────────────
function readURLNav() {
  const p = new URLSearchParams(location.search);
  const num = k => { const v = p.get(k) ? Number(p.get(k)) : null; return Number.isFinite(v) ? v : null; };
  return { team: num('team'), period: num('period'), goal: num('goal'), kr: num('kr'), comment: num('comment') };
}
function readLastNav() {
  const m = document.cookie.match(/(?:^|;\s*)okr_last=([^;]*)/);
  if (!m) return {};
  try { return JSON.parse(decodeURIComponent(m[1])); } catch { return {}; }
}
function writeLastNav(teamId, periodId) {
  const val = encodeURIComponent(JSON.stringify({ team: teamId, period: periodId }));
  const exp = new Date(Date.now() + 30 * 864e5).toUTCString();
  document.cookie = 'okr_last=' + val + ';path=/;expires=' + exp;
}
function updateURL(teamId, periodId, replace = false) {
  const p = new URLSearchParams();
  if (teamId) p.set('team', teamId);
  if (periodId) p.set('period', periodId);
  const qs = p.toString();
  const url = location.pathname + (qs ? '?' + qs : '');
  if (replace) history.replaceState(null, '', url);
  else history.pushState(null, '', url);
}

// ── SIDEBAR TREE EXPANSION PERSISTENCE ────────────────────────────────────────
// Map of nodeId -> false (collapsed). Absence means expanded (default).
// Stored by id, so adding/removing teams never breaks: unknown ids are ignored,
// new ids fall back to the expanded default.
const TREE_EXPANDED_KEY = 'okr_tree_expanded';
function readTreeExpanded() {
  try {
    const raw = localStorage.getItem(TREE_EXPANDED_KEY);
    if (!raw) return {};
    const v = JSON.parse(raw);
    return v && typeof v === 'object' ? v : {};
  } catch { return {}; }
}
function writeTreeExpanded(expanded) {
  try { localStorage.setItem(TREE_EXPANDED_KEY, JSON.stringify(expanded)); } catch { }
}

// Personal settings persisted by the /settings page (per-user localStorage).
// Ключи — из общего storage.js (STORAGE_KEYS), единый контракт с settings.js.
const SETTINGS_DESC_KEY = STORAGE_KEYS.desc;
const SETTINGS_SIDEBAR_KEY = STORAGE_KEYS.sidebar;
function readDescOverrides(uid) {
  try { const v = localStorage.getItem(SETTINGS_DESC_KEY(uid)); const m = v ? JSON.parse(v) : null; return m && typeof m === 'object' ? m : {}; }
  catch { return {}; }
}
// Returns a Set of selected node ids, or null when the user never configured the
// sidebar (→ show everything). An empty configured selection returns an empty Set.
function readSidebarSelection(uid) {
  try { const v = localStorage.getItem(SETTINGS_SIDEBAR_KEY(uid)); if (v == null) return null; const a = JSON.parse(v); return Array.isArray(a) ? new Set(a) : null; }
  catch { return null; }
}
// Keeps a node when its subtree intersects (selected ∪ current team) — so picked
// nodes, their ancestors (for navigation) and the current team always render.
function filterTreeForSidebar(nodes, sel, currentId) {
  if (!sel) return nodes;
  const keep = node => {
    const kids = (node.children || []).map(keep).filter(Boolean);
    const selfVisible = sel.has(node.id) || node.id === currentId;
    if (selfVisible || kids.length) return { ...node, children: kids };
    return null;
  };
  return nodes.map(keep).filter(Boolean);
}

// ── FAVORITE TEAMS PERSISTENCE ────────────────────────────────────────────────
// Per-user list of favorited team ids, in add-order. Client-only (no backend).
// Unknown ids are ignored at render time but kept in storage, so a favorite that
// temporarily drops out of the hierarchy (other period / lost access) returns
// when it reappears — same resilience contract as TREE_EXPANDED_KEY.
const FAV_KEY = uid => `okr_fav_teams:${uid}`;
// Team ids may arrive from the API as numbers or strings; favorites normalize
// every id to a string so storage lookups stay consistent even if a stored id's
// type differs from the current hierarchy's (e.g. after an id-type migration).
const favId = x => String(x);
function readFavorites(uid) {
  try {
    const v = localStorage.getItem(FAV_KEY(uid));
    if (v == null) return [];
    const a = JSON.parse(v);
    return Array.isArray(a) ? a.filter(x => x != null).map(favId) : [];
  } catch { return []; }
}
function writeFavorites(uid, ids) {
  try { localStorage.setItem(FAV_KEY(uid), JSON.stringify(ids)); } catch { }
}
// Immutable toggle: remove if present, else append (keeps add-order). Ids are
// compared as strings so a numeric click matches a stored string id and back.
function toggleFavorite(ids, id) {
  const key = favId(id);
  return ids.includes(key) ? ids.filter(x => x !== key) : [...ids, key];
}
// Flatten the tree into an id->node map, then return nodes for favIds in favIds
// order. Missing ids are skipped (not rendered), never throw.
function collectFavNodes(nodes, favIds) {
  const byId = new Map();
  const walk = list => (list || []).forEach(n => { byId.set(favId(n.id), n); walk(n.children); });
  walk(nodes);
  return favIds.map(id => byId.get(favId(id))).filter(Boolean);
}

// ── DATE HELPERS ──────────────────────────────────────────────────────────────
function daysAgo(iso) {
  if (!iso) return 0;
  const ms = Date.now() - new Date(iso).getTime();
  return Math.max(0, Math.floor(ms / (1000 * 60 * 60 * 24)));
}
function fmtDate(iso) {
  if (!iso) return '';
  return new Date(iso).toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' });
}

// ── MAPPERS ───────────────────────────────────────────────────────────────────
function mapKR(kr) {
  const m = kr.measure || {};
  let start = 0, target = 100, current = 0, done = false, stages = [];
  let unit = '%', checkpoints = [];
  const zeroing = kr.zeroing_criteria || '';
  if (m.numerical) {
    start = m.numerical.start_value; target = m.numerical.target_value; current = m.numerical.current_value;
    unit = m.numerical.unit || '%';
    checkpoints = (m.numerical.checkpoints || []).map(c => ({ value: c.value, progress_percent: c.progress_percent }));
  }
  if (m.boolean) { done = m.boolean.is_done; }
  if (m.project) { stages = (m.project.stages || []).map(s => ({ id: s.id, name: s.title, weight: s.weight, done: s.is_done })); }
  return {
    id: kr.id, goalId: kr.goal_id, name: kr.title, desc: kr.description,
    weight: kr.weight, krType: kr.kind, progress: kr.progress,
    start, target, current, done, stages, unit, checkpoints, zeroing,
    note: kr.note ? { text: kr.note.text, author: kr.note.author_name, authorUdid: kr.note.author_udid, date: fmtDate(kr.note.updated_at) } : null,
    updatedAt: kr.updated_at, updatedDaysAgo: daysAgo(kr.updated_at),
  };
}
function mapGoal(g) {
  return {
    id: g.id, teamId: g.team_id, periodId: g.period_id,
    title: g.title, desc: g.description,
    priority: g.priority, weight: g.weight,
    type: (g.work_type || '').toLowerCase(),
    focus: g.focus_type,
    owners: g.owners || [],
    progress: g.progress,
    progressMeta: g.progress_meta,
    krs: (g.key_results || []).map(mapKR),
    comments: (g.comments || []).map(c => ({ id: c.id, author: c.author_name, authorUdid: c.author_udid, date: fmtDate(c.created_at), text: c.text, resolved: !!c.resolved, resolvedBy: c.resolved_by_name, resolvedByUdid: c.resolved_by_udid, resolvedAt: c.resolved_at ? fmtDate(c.resolved_at) : null })),
    shareTeams: g.share_teams || [],
    shared: (g.share_teams || []).length > 0,
    updatedAt: g.updated_at,
    updatedDaysAgo: daysAgo(g.updated_at),
  };
}

// ── KR PROGRESS CALC (client-side for modals) ─────────────────────────────────
const clampPct = v => Math.max(0, Math.min(100, v));

function calcKRProgress(kr) {
  if (kr.krType === 'BOOLEAN') return kr.done ? 100 : 0;
  if (kr.krType === 'PROJECT') return Math.min(100, (kr.stages || []).filter(s => s.done).reduce((a, s) => a + (s.weight || 0), 0));
  // NUMERICAL: linear interpolation between points when checkpoints are set, otherwise plain linear.
  const start = Number(kr.start || 0), target = Number(kr.target ?? 100), cur = Number(kr.current || 0);
  const raw = (kr.checkpoints || []).filter(c => c.value !== '' && c.value !== null && c.value !== undefined);
  if (raw.length) {
    const pts = [{ value: start, pct: 0 }, ...raw.map(c => ({ value: Number(c.value), pct: Number(c.progress_percent) })), { value: target, pct: 100 }].sort((a, b) => a.value - b.value);
    if (cur <= pts[0].value) return clampPct(pts[0].pct);
    const last = pts[pts.length - 1];
    if (cur >= last.value) return clampPct(last.pct);
    for (let i = 0; i < pts.length - 1; i++) {
      const l = pts[i], r = pts[i + 1];
      if (cur >= l.value && cur <= r.value) {
        if (r.value === l.value) return clampPct(l.pct);
        return clampPct(Math.round(l.pct + (cur - l.value) / (r.value - l.value) * (r.pct - l.pct)));
      }
    }
    return 0;
  }
  if (start === target) return cur >= target ? 100 : 0;
  return clampPct(Math.round((cur - start) / (target - start) * 100));
}

// numericalGuide builds the checkpoint guide for a numerical KR: every checkpoint
// with its reached state, plus the next unreached checkpoint (or the target once
// all checkpoints are passed) and the remaining distance to it. Returns null when
// the KR has no checkpoints.
function numericalGuide(kr) {
  const cps = (kr.checkpoints || [])
    .filter(c => c.value !== '' && c.value !== null && c.value !== undefined)
    .map(c => ({ value: Number(c.value), pct: Number(c.progress_percent) }))
    .sort((a, b) => a.value - b.value);
  if (!cps.length) return null;
  const cur = Number(kr.current || 0);
  const target = Number(kr.target ?? 100);
  const steps = cps.map(c => ({ ...c, reached: cur >= c.value }));
  const upcoming = cps.find(c => c.value > cur);
  let next = null;
  if (upcoming) next = { value: upcoming.value, pct: upcoming.pct, remaining: upcoming.value - cur, isTarget: false };
  else if (cur < target) next = { value: target, pct: 100, remaining: target - cur, isTarget: true };
  return { steps, next };
}

// ── DESIGN CONSTANTS ──────────────────────────────────────────────────────────
const HEALTH_COLOR = { ahead: '#16a34a', on_track: '#2563eb', below: '#ef4444', stale: '#d97706', no_goals: '#d1d5db' };
const HEALTH_LABEL = { ahead: 'опережает', on_track: 'в плане', below: 'отстаёт', stale: 'нет обновлений', no_goals: 'нет целей' };
const FOCUS_COLORS = { EFFICIENCY: '#0891b2', QUALITY: '#7c3aed', RELIABILITY: '#059669', GROWTH: '#d97706', PROFITABILITY: '#dc2626', STABILITY: '#6366f1', SPEED_EFFICIENCY: '#0891b2', TECH_INDEPENDENCE: '#be185d', DEFAULT: '#6b7280' };
const KR_TYPE_C = { NUMERICAL: '#2563eb', BOOLEAN: '#7c3aed', PROJECT: '#d97706' };
const KR_UNITS = ['%', 'RPS', 'мс', 'сек', 'мин', 'час', 'дней', 'шт', '₽', 'запросов', 'ошибок', 'пользователей', 'заказов', 'рублей'];
const KR_TYPE_LABEL = { BOOLEAN: 'Бинарный', PROJECT: 'Проектный', NUMERICAL: 'Числовой' };
const KR_TYPE_OPTIONS = ['BOOLEAN', 'PROJECT', 'NUMERICAL'];
const KR_TYPE_HINT = (
  <span className="kr-type-hint">
    <span style={{ display: 'block', marginBottom: 8 }}>
      <b style={{ color: KR_TYPE_C.BOOLEAN }}>Бинарный</b> — результат либо выполнен, либо нет. Например: «Проведён аудит», «Запущен сервис».
    </span>
    <span style={{ display: 'block', marginBottom: 8 }}>
      <b style={{ color: KR_TYPE_C.PROJECT }}>Проектный</b> — результат состоит из нескольких этапов. Прогресс — сумма вкладов завершённых этапов.
    </span>
    <span style={{ display: 'block' }}>
      <b style={{ color: KR_TYPE_C.NUMERICAL }}>Числовой</b> — результат измеряется числом: проценты, деньги, RPS, штуки, дни, миллисекунды. Прогресс считается линейно от старта к цели или через промежуточные значения.
    </span>
  </span>
);

// fmtNum formats a number with space thousands separators, keeping existing fractional digits.
function fmtNum(n) {
  if (n === null || n === undefined || n === '') return '';
  const num = Number(n);
  if (!isFinite(num)) return String(n);
  const parts = String(num).split('.');
  parts[0] = parts[0].replace(/\B(?=(\d{3})+(?!\d))/g, ' ');
  return parts.join('.');
}
function fmtVal(n, unit) { return unit ? `${fmtNum(n)} ${unit}` : fmtNum(n); }

// groupDigits formats a RAW numeric string with space thousands separators while
// preserving a trailing dot, fractional digits and a leading minus during typing
// (unlike fmtNum, which normalizes through Number() and would drop "123." → "123").
function groupDigits(raw) {
  if (raw === null || raw === undefined || raw === '') return '';
  let s = String(raw);
  const neg = s.startsWith('-');
  if (neg) s = s.slice(1);
  const dot = s.indexOf('.');
  const intPart = dot === -1 ? s : s.slice(0, dot);
  const fracPart = dot === -1 ? '' : s.slice(dot + 1);
  const groupedInt = intPart.replace(/\B(?=(\d{3})+(?!\d))/g, ' ');
  return (neg ? '-' : '') + groupedInt + (dot === -1 ? '' : '.' + fracPart);
}
// sanitizeNum keeps only digits, at most one dot and an optional single leading minus.
function sanitizeNum(s) {
  if (s === null || s === undefined) return '';
  s = String(s);
  const neg = s.trim().startsWith('-');
  s = s.replace(/[^\d.]/g, '');
  const firstDot = s.indexOf('.');
  if (firstDot !== -1) s = s.slice(0, firstDot + 1) + s.slice(firstDot + 1).replace(/\./g, '');
  return (neg ? '-' : '') + s;
}
// sigCountBefore counts non-space chars in str.slice(0, idx); caretForSig finds the
// index right after `sig` non-space chars — together they keep the caret stable when
// grouping spaces shift on input.
function sigCountBefore(str, idx) {
  let n = 0;
  for (let i = 0; i < idx && i < str.length; i++) if (str[i] !== ' ') n++;
  return n;
}
function caretForSig(formatted, sig) {
  if (sig <= 0) return 0;
  let seen = 0;
  for (let i = 0; i < formatted.length; i++) {
    if (formatted[i] !== ' ') seen++;
    if (seen >= sig) return i + 1;
  }
  return formatted.length;
}
// NumInput displays large numeric values with space thousands separators
// (250 000, 340 000 000 ₽) while emitting the raw unformatted numeric string via
// onChange, preserving the caret across reformatting. Use for metric values only —
// percents and weights (0–100) stay as plain number inputs.
function NumInput({ value, onChange, className, placeholder, ...rest }) {
  const ref = useRef(null);
  const caretRef = useRef(null);
  const display = groupDigits(value == null ? '' : value);
  React.useLayoutEffect(() => {
    if (caretRef.current != null && ref.current) {
      const pos = caretForSig(ref.current.value, caretRef.current);
      ref.current.setSelectionRange(pos, pos);
      caretRef.current = null;
    }
  });
  const handleChange = e => {
    const el = e.target;
    const caret = el.selectionStart == null ? el.value.length : el.selectionStart;
    caretRef.current = sigCountBefore(el.value, caret);
    onChange(sanitizeNum(el.value));
  };
  return <input type="text" inputMode="decimal" ref={ref} value={display} onChange={handleChange}
    className={className} placeholder={placeholder} {...rest} />;
}
const FOCUS_OPTIONS = ['PROFITABILITY', 'STABILITY', 'SPEED_EFFICIENCY', 'TECH_INDEPENDENCE'];
// focusLabel title-cases an UPPER_SNAKE focus enum for display (SPEED_EFFICIENCY → "Speed
// Efficiency"), matching the Title Case style of work-type labels (Delivery/Discovery).
function focusLabel(f) {
  if (!f) return '';
  return String(f).split('_').filter(Boolean)
    .map(w => w.charAt(0).toUpperCase() + w.slice(1).toLowerCase()).join(' ');
}
const STATUS_STEPS = [{ k: 'forming', l: 'Черновик' }, { k: 'ready', l: 'К валидации' }, { k: 'in_progress', l: 'В работе' }, { k: 'closed', l: 'Закрыты' }];

// greenThreshold: progress at or above this percent is considered "in plan" (green)
// regardless of the forecast-based pace check. Configurable via admin settings (default 80).
function healthOf(p, stale, forecast, greenThreshold = 80) {
  if (p === null || p === undefined) return 'no_goals';
  if (stale) return 'stale';
  if (p >= greenThreshold) return 'ahead';
  if (forecast == null) return 'on_track';
  const delta = forecast - p;
  if (delta < -10) return 'ahead';
  if (delta > 10) return 'below';
  return 'on_track';
}

// sidebarProgressColor colors the team progress percent in the sidebar: green when the team
// has reached the green threshold or is keeping pace, red when it lags behind the period pace.
// The lag tolerance mirrors the Health Check-in "Отстающие" category (behind_margin): red when
// progress < forecast - behindMargin.
function sidebarProgressColor(prog, forecast, status, behindMargin = 10, greenThreshold = 80) {
  if (prog == null) return HEALTH_COLOR.no_goals;
  if (prog >= greenThreshold) return HEALTH_COLOR.ahead;
  if (status === 'closed') return HEALTH_COLOR.below;
  if (forecast != null && forecast - prog > behindMargin) return HEALTH_COLOR.below;
  return HEALTH_COLOR.ahead;
}

// ── MICRO COMPONENTS ──────────────────────────────────────────────────────────
function ProgressBar({ value, forecast, h = 8, color }) {
  return (
    <div className="progress-bar" style={{ height: h, borderRadius: h / 2 }}>
      <div className="progress-bar__fill" style={{ width: `${Math.min(value || 0, 100)}%`, background: color || ACCENT, borderRadius: h / 2 }} />
      {forecast != null && <div className="progress-bar__forecast" style={{ top: -3, left: `${forecast}%`, height: h + 6 }} />}
    </div>
  );
}

function Badge({ label, color = '#6b7280', bg }) {
  return <span className="badge" style={{ color, background: bg || `${color}18` }}>{label}</span>;
}

function PriBadge({ p }) {
  const c = { P0: '#dc2626', P1: '#d97706', P2: '#2563eb', P3: '#6b7280' }[p] || '#6b7280';
  return <Badge label={p} color={c} />;
}

function InfoHint({ children, width = 300 }) {
  const [open, setOpen] = useState(false);
  const [pos, setPos] = useState(null);
  const ref = useRef();
  const show = () => {
    if (!ref.current) return;
    const r = ref.current.getBoundingClientRect();
    setPos({ top: r.bottom + 6, left: r.left + r.width / 2 });
    setOpen(true);
  };
  return (
    <span ref={ref} onMouseEnter={show} onMouseLeave={() => setOpen(false)}
      tabIndex={0} onFocus={show} onBlur={() => setOpen(false)} className="info-hint">
      ?
      {open && pos && <span className="info-hint__tooltip" style={{ top: pos.top, left: pos.left, width }}>{children}</span>}
    </span>
  );
}

function FieldLabel({ children, hint, required, size = 13 }) {
  return (
    <div className="field-label" style={size !== 13 ? { fontSize: size } : undefined}>
      <span>{children}</span>
      {required && <span className="field-label__required">*</span>}
      {hint && <InfoHint>{hint}</InfoHint>}
    </div>
  );
}

function Avatar({ name, avatarUrl, size = 28, showName = false }) {
  const initials = (name || '?').split(' ').slice(0, 2).map(w => w[0] || '').join('').toUpperCase();
  const colors = ['#2563eb', '#7c3aed', '#059669', '#d97706', '#dc2626', '#0891b2', '#be185d', '#6366f1'];
  const color = colors[(name || '').charCodeAt(0) % colors.length] || colors[0];
  return (
    <div className="avatar">
      {avatarUrl
        ? <img src={avatarUrl} width={size} height={size} className="avatar__img" alt={name || ''} />
        : <div className="avatar__initials" style={{ width: size, height: size, background: color, fontSize: size * 0.38 }}>{initials}</div>}
      {showName && <span className="avatar__name">{name}</span>}
    </div>
  );
}

// Shows the avatar for a comment author. Uses the name/UDID cache (no network call).
function AvatarWithUDID({ name, udid, size = 28 }) {
  const cached = udid ? _userByUdid.get(udid) : _userByName.get(name);
  return <Avatar name={name} avatarUrl={cached?.avatar_url || null} size={size} />;
}

// Sidebar lives in the shared sidebar.js module (loaded before this script).

// ── TEAM COMBOBOX ─────────────────────────────────────────────────────────────
function flattenTree(nodes, depth = 0) {
  const out = [];
  (nodes || []).forEach(n => { out.push({ ...n, depth }); flattenTree(n.children || [], depth + 1).forEach(c => out.push(c)); });
  return out;
}

function TeamCombobox({ selectedIds, onChange, excludeId, accent, allTeams }) {
  const [q, setQ] = useState(''); const [open, setOpen] = useState(false); const [hi, setHi] = useState(0);
  const inputRef = useRef(); const wrapRef = useRef();
  const flat = flattenTree(allTeams || []).filter(t => t.id !== excludeId);
  const ql = q.trim().toLowerCase();
  const filtered = ql ? flat.filter(t => t.name.toLowerCase().includes(ql)) : flat;
  const available = filtered.filter(t => !selectedIds.includes(t.id));
  useEffect(() => { setHi(0); }, [q]);
  useEffect(() => {
    const h = e => { if (wrapRef.current && !wrapRef.current.contains(e.target)) setOpen(false); };
    document.addEventListener('mousedown', h);
    return () => document.removeEventListener('mousedown', h);
  }, []);
  const add = t => { onChange([...selectedIds, t.id]); setQ(''); inputRef.current?.focus(); };
  const rem = id => onChange(selectedIds.filter(x => x !== id));
  const sel = selectedIds.map(id => flat.find(t => t.id === id)).filter(Boolean);
  const onKey = e => {
    if (e.key === 'ArrowDown') { e.preventDefault(); setOpen(true); setHi(h => Math.min(available.length - 1, h + 1)); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); setHi(h => Math.max(0, h - 1)); }
    else if (e.key === 'Enter') { e.preventDefault(); if (open && available[hi]) add(available[hi]); }
    else if (e.key === 'Escape') { if (open) { e.preventDefault(); setOpen(false); } }
    else if (e.key === 'Backspace' && !q && sel.length > 0) rem(sel[sel.length - 1].id);
  };
  return (
    <div ref={wrapRef} className="team-combobox">
      <div onClick={() => { setOpen(true); inputRef.current?.focus(); }}
        className={`team-combobox__input-area${open ? ' team-combobox__input-area--open' : ''}`}>
        {sel.map(t => {
          const color = TEAM_TYPE_COLOR[t.type] || '#6b7280';
          return (
            <div key={t.id} className="team-combobox__tag" style={{ background: `${color}15`, border: `1px solid ${color}40` }}>
              <span className="team-combobox__tag-type" style={{ color }}>{TEAM_TYPE_LABEL[t.type] || t.type}</span>
              <span className="team-combobox__tag-name">{t.name}</span>
              <button onClick={e => { e.stopPropagation(); rem(t.id); }} className="team-combobox__tag-remove">×</button>
            </div>
          );
        })}
        <input ref={inputRef} value={q} onChange={e => { setQ(e.target.value); setOpen(true); }} onFocus={() => setOpen(true)} onKeyDown={onKey}
          placeholder={sel.length ? 'Ещё…' : 'Найдите команду'} className="team-combobox__input" />
      </div>
      {open && (
        <div className="team-combobox__dropdown">
          {available.length === 0
            ? <div className="team-combobox__empty">{ql ? 'Не найдено' : 'Все добавлены'}</div>
            : available.map((t, i) => {
              const color = TEAM_TYPE_COLOR[t.type] || '#6b7280';
              return (
                <div key={t.id} onClick={() => add(t)} onMouseEnter={() => setHi(i)}
                  className={`team-combobox__option${i === hi ? ' team-combobox__option--hi' : ''}`}
                  style={{ padding: `7px 12px 7px ${8 + t.depth * 14}px` }}>
                  <div className="team-combobox__option-stripe" style={{ background: color }} />
                  <span className="team-combobox__option-type" style={{ color, background: `${color}12` }}>{TEAM_TYPE_LABEL[t.type] || t.type}</span>
                  <span className="team-combobox__option-name">{t.name}</span>
                </div>
              );
            })}
        </div>
      )}
    </div>
  );
}

// ── STATUS STEPPER ────────────────────────────────────────────────────────────
function StatusStepper({ status, hasGoals, onChange, accent, statusChangedAt }) {
  const curIdx = STATUS_STEPS.findIndex(s => s.k === status);
  return (
    <div className="status-stepper">
      {!hasGoals && <span className="status-stepper__no-goals">Нет целей</span>}
      {STATUS_STEPS.map((s, i) => {
        const isCur = s.k === status; const isPast = i < curIdx;
        return (
          <React.Fragment key={s.k}>
            {i > 0 && <div className="status-stepper__connector" />}
            <button onClick={() => hasGoals && onChange(s.k)} disabled={!hasGoals} className="status-stepper__btn"
              style={{ background: isCur ? accent : isPast ? `${accent}15` : 'transparent', color: isCur ? 'white' : isPast ? accent : '#9ca3af' }}>
              {s.l}
            </button>
          </React.Fragment>
        );
      })}
      <div className="status-stepper__meta">
        {statusChangedAt && <span className="status-stepper__changed">изменён {fmtDate(statusChangedAt)}</span>}
        {status === 'in_progress' && <span className="status-stepper__locked status-stepper__locked--progress">🔒 Редактирование заблокировано</span>}
        {status === 'closed' && <span className="status-stepper__locked status-stepper__locked--closed">🔒 Период закрыт</span>}
      </div>
    </div>
  );
}

// ── MODAL OVERLAY CLOSE ───────────────────────────────────────────────────────
// Закрывает модалку только если и нажатие, и отпускание мыши произошли на самом
// оверлее. Иначе выделение текста, начатое внутри модалки, при выносе курсора с
// зажатой кнопкой за её пределы (mouseup на оверлее) закрывало бы окно без сохранения.
function useOverlayClose(onClose) {
  const downOnOverlay = useRef(false);
  return {
    onMouseDown: e => { downOnOverlay.current = e.target === e.currentTarget; },
    onMouseUp: e => {
      const shouldClose = downOnOverlay.current && e.target === e.currentTarget;
      downOnOverlay.current = false;
      if (shouldClose) onClose();
    },
  };
}

// ── KR PROGRESS MODAL ─────────────────────────────────────────────────────────
function KRProgressModal({ kr, onSave, onClose, accent }) {
  const [form, setForm] = useState({ ...kr, stages: (kr.stages || []).map(s => ({ ...s })) });
  const [note, setNote] = useState(kr.note?.text ?? ''); const [saving, setSaving] = useState(false);
  const [descDraft, setDescDraft] = useState(''); const [descEditing, setDescEditing] = useState(false);
  const set = (k, v) => setForm(f => ({ ...f, [k]: v }));
  const setStage = (i, k, v) => setForm(f => { const ss = [...f.stages]; ss[i] = { ...ss[i], [k]: v }; return { ...f, stages: ss }; });
  const progress = calcKRProgress(form);
  const initialNote = kr.note?.text ?? '';
  const dirtyProgress = (() => {
    if (form.krType === 'NUMERICAL') return String(form.current) !== String(kr.current);
    if (form.krType === 'BOOLEAN') return !!form.done !== !!kr.done;
    if (form.krType === 'PROJECT') return (form.stages || []).some((s, i) => !!s.done !== !!((kr.stages || [])[i] || {}).done);
    return false;
  })();
  const isDirty = dirtyProgress || note.trim() !== initialNote.trim() || (descEditing && descDraft.trim() !== '');
  const save = async () => {
    setSaving(true);
    try {
      if (form.krType === 'NUMERICAL') {
        await apiPost(`/api/v1/krs/${kr.id}/progress/numerical`, { current_value: parseFloat(form.current) || 0 });
      } else if (form.krType === 'BOOLEAN') {
        await apiPost(`/api/v1/krs/${kr.id}/progress/boolean`, { done: !!form.done });
      } else if (form.krType === 'PROJECT') {
        await apiPost(`/api/v1/krs/${kr.id}/progress/project`, { stages: form.stages.map(s => ({ id: s.id, done: !!s.done })) });
      }
      const trimmed = note.trim();
      if (trimmed && trimmed !== (kr.note?.text ?? '')) {
        await apiPost(`/api/v1/krs/${kr.id}/note`, { text: trimmed });
      }
      const trimmedDesc = descDraft.trim();
      if (descEditing && trimmedDesc) {
        await apiPost(`/api/v1/krs/${kr.id}/description`, { description: trimmedDesc });
      }
      onSave();
    } catch (e) { alert('Ошибка сохранения: ' + e.message); }
    finally { setSaving(false); }
  };
  const { requestClose, confirmEl } = useModalClose({ isDirty, canSave: !saving, onSave: save, onClose });
  const overlay = useOverlayClose(requestClose);
  const zeroingNote = form.zeroing ? (
    <div className="kr-zeroing-note kr-zeroing-note--md">
      <span className="kr-zeroing-note__icon">⊘</span>Критерий обнуления: {form.zeroing}
    </div>
  ) : null;
  return (
    <>
    <div className="modal-overlay modal-overlay--z300" {...overlay}>
      <div onClick={e => e.stopPropagation()} className="modal-box modal-box--w480">
        <div className="modal-header">
          <div>
            <div className="modal-title">Обновить прогресс</div>
            <div className="modal-subtitle">{kr.name}</div>
          </div>
          <button onClick={requestClose} className="modal-close">×</button>
        </div>
        {kr.desc ? (
          <div className="kr-progress-desc">
            <div className="kr-progress-desc__label">Описание</div>
            <Markdown text={kr.desc} className="kr-progress-desc__text" />
          </div>
        ) : descEditing ? (
          <div className="kr-progress-desc">
            <div className="kr-progress-desc__label">Описание</div>
            <textarea value={descDraft} onChange={e => setDescDraft(e.target.value)} rows={3} autoFocus
              placeholder="Добавьте описание для контекста…"
              className="form-textarea form-textarea--sm" style={{ resize: 'vertical' }} />
          </div>
        ) : (
          <div className="kr-progress-desc">
            <button type="button" onClick={() => setDescEditing(true)} className="kr-zeroing-btn">
              <span className="kr-zeroing-btn__icon">＋</span> Добавить описание
            </button>
          </div>
        )}
        <div className="modal-body">
          {form.krType === 'NUMERICAL' && (
            <div className="kr-progress-field">
              <div className="kr-progress-field__label">Текущее значение <span className="kr-progress-field__hint">({fmtVal(form.start, form.unit)} → {fmtVal(form.target, form.unit)})</span></div>
              <NumInput value={form.current} onChange={v => set('current', v)} className="form-input" />
              {(() => {
                const guide = numericalGuide(form);
                if (!guide) return null;
                return (
                  <div className="kr-guide">
                    <div className="kr-guide__title">Ориентир по шагам</div>
                    <div className="kr-guide__steps">
                      {guide.steps.map((s, i) => (
                        <div key={i} className={`kr-guide__step${s.reached ? ' kr-guide__step--reached' : ''}`}>
                          <span className="kr-guide__step-val">{fmtVal(s.value, form.unit)}</span>
                          <span className="kr-guide__step-pct">{s.pct}%</span>
                        </div>
                      ))}
                    </div>
                    {guide.next && (
                      <div className="kr-guide__hint">
                        {guide.next.isTarget
                          ? `До цели ${fmtVal(guide.next.value, form.unit)}: ${fmtVal(guide.next.remaining, form.unit)}`
                          : `До следующего шага ${fmtVal(guide.next.value, form.unit)} (${guide.next.pct}%): ${fmtVal(guide.next.remaining, form.unit)}`}
                      </div>
                    )}
                  </div>
                );
              })()}
              {zeroingNote}
              <div style={{ marginTop: 10 }}>
                <div className="kr-progress-row">
                  <span className="kr-progress-row__label">Прогресс</span>
                  <span style={{ fontSize: 13, fontWeight: 700, color: accent }}>{progress}%</span>
                </div>
                <ProgressBar value={progress} h={6} color={accent} />
              </div>
            </div>
          )}
          {form.krType === 'BOOLEAN' && (
            <>
              <label className="kr-boolean-label">
                <input type="checkbox" checked={!!form.done} onChange={e => set('done', e.target.checked)} style={{ width: 18, height: 18, accentColor: accent }} />
                <span className="kr-boolean-text">Выполнено</span>
                <span className="kr-boolean-pct" style={{ color: form.done ? '#16a34a' : '#9ca3af' }}>{form.done ? '100%' : '0%'}</span>
              </label>
              {zeroingNote}
            </>
          )}
          {form.krType === 'PROJECT' && (
            <div className="kr-progress-field">
              <div className="kr-progress-field__label">Шаги</div>
              {form.stages.map((s, i) => (
                <label key={s.id || i} className="kr-stage-label"
                  style={{ background: s.done ? `${accent}08` : '#f9fafb', border: `1px solid ${s.done ? `${accent}30` : '#f0f1f3'}` }}>
                  <input type="checkbox" checked={!!s.done} onChange={e => setStage(i, 'done', e.target.checked)} style={{ width: 16, height: 16, accentColor: accent }} />
                  <span className="kr-stage-name" style={{ fontWeight: s.done ? 600 : 400 }}>{s.name}</span>
                  <span className="kr-stage-weight">вес {s.weight}</span>
                </label>
              ))}
              {zeroingNote}
              <div style={{ marginTop: 10 }}>
                <div className="kr-progress-row">
                  <span className="kr-progress-row__label">Прогресс</span>
                  <span style={{ fontSize: 13, fontWeight: 700, color: accent }}>{progress}%</span>
                </div>
                <ProgressBar value={progress} h={6} color={accent} />
              </div>
            </div>
          )}
          <div>
            <div className="kr-note-label">Заметка <span className="kr-note-optional">(опционально)</span></div>
            <MarkdownEditor value={note} onChange={setNote} rows={3} placeholder="Контекст, блокеры…"
              textareaClassName="form-textarea form-textarea--sm" textareaStyle={{ resize: 'vertical' }} />
            {kr.note && (
              <div className="kr-note-meta" style={{ display: 'flex', alignItems: 'center', gap: 6, marginTop: 6, fontSize: 11, color: '#9ca3af' }}>
                <UserInfo name={kr.note.author} udid={kr.note.authorUdid} size={18} /> · {kr.note.date}
              </div>
            )}
          </div>
        </div>
        <div className="modal-footer">
          <button onClick={onClose} className="btn btn--secondary">Отмена</button>
          <button onClick={save} disabled={saving} className="btn btn--primary"
            style={{ background: saving ? '#e5e7eb' : accent, color: saving ? '#9ca3af' : 'white', cursor: saving ? 'default' : 'pointer' }}>
            {saving ? 'Сохраняем…' : 'Сохранить'}
          </button>
        </div>
      </div>
    </div>
    {confirmEl}
    </>
  );
}

// ── KR EDIT MODAL ─────────────────────────────────────────────────────────────
function KREditModal({ kr, goalId, onSave, onClose, accent }) {
  const isNew = !kr;
  const [form, setForm] = useState(kr
    ? { ...kr, stages: (kr.stages || []).map(s => ({ ...s })), checkpoints: (kr.checkpoints || []).map(c => ({ ...c })) }
    : { name: '', desc: '', weight: 20, krType: 'NUMERICAL', unit: '%', start: 0, target: 100, current: 0, done: false, stages: [], checkpoints: [], zeroing: '' });
  const [saving, setSaving] = useState(false);
  const [showZeroing, setShowZeroing] = useState(!!(kr && kr.zeroing));
  const set = (k, v) => setForm(f => ({ ...f, [k]: v }));
  const setSt = (i, k, v) => setForm(f => { const ss = [...f.stages]; ss[i] = { ...ss[i], [k]: v }; return { ...f, stages: ss }; });
  const addSt = () => setForm(f => ({ ...f, stages: [...f.stages, { id: `s_${Date.now()}`, name: '', weight: 0, done: false }] }));
  const remSt = i => setForm(f => ({ ...f, stages: f.stages.filter((_, j) => j !== i) }));
  const setCp = (i, k, v) => setForm(f => { const cc = [...(f.checkpoints || [])]; cc[i] = { ...cc[i], [k]: v }; return { ...f, checkpoints: cc }; });
  const addCp = () => setForm(f => ({ ...f, checkpoints: [...(f.checkpoints || []), { value: '', progress_percent: '' }] }));
  const remCp = i => setForm(f => ({ ...f, checkpoints: (f.checkpoints || []).filter((_, j) => j !== i) }));
  const sw = form.stages.reduce((s, st) => s + Number(st.weight || 0), 0);
  const prev = calcKRProgress(form);
  const save = async () => {
    if (!form.name.trim()) return;
    setSaving(true);
    try {
      const fd = new FormData();
      fd.append('title', form.name.trim());
      fd.append('description', form.desc || '');
      fd.append('weight', String(Number(form.weight) || 0));
      fd.append('kind', form.krType);
      fd.append('zeroing_criteria', form.zeroing || '');
      if (form.krType === 'NUMERICAL') {
        fd.append('numerical_unit', form.unit || '%');
        fd.append('numerical_start', String(Number(form.start) || 0));
        fd.append('numerical_target', String(Number(form.target) || 0));
        fd.append('numerical_current', String(Number(form.current) || 0));
        (form.checkpoints || []).forEach(c => {
          if (c.value === '' || c.value === null || c.value === undefined) return;
          fd.append('checkpoint_value[]', String(Number(c.value) || 0));
          fd.append('checkpoint_percent[]', String(c.progress_percent || 0));
        });
      }
      else if (form.krType === 'BOOLEAN') { fd.append('boolean_done', form.done ? 'true' : 'false'); }
      else if (form.krType === 'PROJECT') {
        (form.stages || []).forEach(st => { fd.append('step_title[]', st.name || ''); fd.append('step_weight[]', String(st.weight || 0)); fd.append('step_done[]', st.done ? 'true' : 'false'); });
      }
      await apiForm(isNew ? `/api/v1/goals/${goalId}/key-results` : `/api/v1/krs/${kr.id}`, fd);
      onSave();
    } catch (e) { alert('Ошибка: ' + e.message); }
    finally { setSaving(false); }
  };
  const canSave = !saving && !!form.name.trim();
  const initialFormRef = useRef(null);
  if (initialFormRef.current === null) initialFormRef.current = JSON.stringify(form);
  const isDirty = JSON.stringify(form) !== initialFormRef.current;
  const { requestClose, confirmEl } = useModalClose({ isDirty, canSave, onSave: save, onClose });
  const overlay = useOverlayClose(requestClose);
  return (
    <>
    <div className="modal-overlay modal-overlay--z300" {...overlay}>
      <div onClick={e => e.stopPropagation()} className="modal-box modal-box--w560">
        <div className="modal-header modal-header--sticky">
          <div className="modal-title modal-title--lg">{isNew ? 'Добавить KR' : 'Редактировать KR'}</div>
          <button onClick={requestClose} className="modal-close">×</button>
        </div>
        <div className="modal-body">
          <div className="form-group--sm">
            <div className="kr-num-field__label">Название</div>
            <input value={form.name} onChange={e => set('name', e.target.value)} placeholder="Что измеряет этот KR?" className="form-input" />
          </div>
          <div className="form-group--sm">
            <div className="kr-num-field__label">Описание</div>
            <MarkdownEditor value={form.desc} onChange={v => set('desc', v)} rows={2}
              textareaClassName="form-textarea form-textarea--sm" textareaStyle={{ resize: 'vertical' }} />
          </div>
          <div className="form-row" style={{ marginBottom: 14 }}>
            <div className="form-col">
              <div className="kr-num-field__label">Вес</div>
              <input type="number" min={0} max={100} value={form.weight} onChange={e => set('weight', e.target.value)} className="form-input" />
            </div>
            <div className="form-col">
              <div className="kr-num-field__label">Тип Key Result<InfoHint>{KR_TYPE_HINT}</InfoHint></div>
              <select value={form.krType} onChange={e => set('krType', e.target.value)} className="form-select">
                {KR_TYPE_OPTIONS.map(t => <option key={t} value={t}>{KR_TYPE_LABEL[t]}</option>)}
              </select>
            </div>
          </div>
          {form.krType === 'NUMERICAL' && (
            <div className="kr-num-section">
              <div className="kr-num-section__title">Числовой прогресс</div>
              <div className="form-row" style={{ marginBottom: 10 }}>
                <div className="form-col">
                  <div className="kr-num-field__label">Стартовое значение</div>
                  <div className="kr-num-input-suffix">
                    <NumInput value={form.start} onChange={v => set('start', v)} className="form-input form-input--sm" />
                    <span className="kr-num-input-suffix__unit">{form.unit}</span>
                  </div>
                </div>
                <div className="form-col">
                  <div className="kr-num-field__label">Единица измерения</div>
                  <select value={form.unit || '%'} onChange={e => set('unit', e.target.value)} className="form-select form-select--sm">
                    {KR_UNITS.map(u => <option key={u} value={u}>{u}</option>)}
                  </select>
                </div>
              </div>
              <div className="form-row" style={{ marginBottom: 10 }}>
                <div className="form-col">
                  <div className="kr-num-field__label">Цель</div>
                  <div className="kr-num-input-suffix">
                    <NumInput value={form.target} onChange={v => set('target', v)} className="form-input form-input--sm" />
                    <span className="kr-num-input-suffix__unit">{form.unit}</span>
                  </div>
                </div>
                <div className="form-col">
                  <div className="kr-num-field__label">Текущее значение</div>
                  <div className="kr-num-input-suffix">
                    <NumInput value={form.current} onChange={v => set('current', v)} className="form-input form-input--sm" />
                    <span className="kr-num-input-suffix__unit">{form.unit}</span>
                  </div>
                </div>
              </div>
              <div className="kr-checkpoints" style={{ marginTop: 12 }}>
                <div className="kr-section-head">
                  <span className="kr-section-head__title">Промежуточные значения</span>
                  <span className="kr-section-head__opt">опционально</span>
                  <InfoHint>Промежуточное значение задаёт, какой процент достижения KR даёт конкретное значение метрики. Прогресс интерполируется линейно между стартом (0%), промежуточными значениями и целью (100%).</InfoHint>
                </div>
                {(form.checkpoints || []).length > 0 && (
                  <div className="kr-cp-head">
                    <span className="kr-cp-head__label">Значение ({form.unit})</span>
                    <span className="kr-cp-head__label">Прогресс, %</span>
                    <span />
                  </div>
                )}
                {(form.checkpoints || []).map((c, i) => (
                  <div key={i} className="kr-cp-row">
                    <NumInput placeholder="напр. 150" value={c.value} onChange={v => setCp(i, 'value', v)} className="form-input form-input--sm" />
                    <input type="number" placeholder="0–100" min={0} max={100} value={c.progress_percent} onChange={e => setCp(i, 'progress_percent', e.target.value)} className="form-input form-input--sm" />
                    <button onClick={() => remCp(i)} className="kr-step-delete">×</button>
                  </div>
                ))}
                <button type="button" onClick={addCp} className="kr-dashed-btn">+ Добавить промежуточное значение</button>
              </div>
              <div className="kr-progress-row" style={{ marginTop: 10 }}>
                <span className="kr-progress-row__label">Прогресс</span>
                <span style={{ fontSize: 12, fontWeight: 700, color: accent }}>{prev}%</span>
              </div>
              <ProgressBar value={prev} h={5} color={accent} />
            </div>
          )}
          {form.krType === 'BOOLEAN' && (
            <div className="kr-num-section">
              <label style={{ display: 'flex', alignItems: 'center', gap: 10, cursor: 'pointer' }}>
                <input type="checkbox" checked={!!form.done} onChange={e => set('done', e.target.checked)} style={{ width: 18, height: 18, accentColor: accent }} />
                <span className="kr-boolean-text">Выполнено</span>
                <span style={{ marginLeft: 'auto', fontWeight: 700, color: form.done ? '#16a34a' : '#9ca3af' }}>{form.done ? '100%' : '0%'}</span>
              </label>
            </div>
          )}
          {form.krType === 'PROJECT' && (
            <div className="kr-num-section">
              <div className="kr-steps-header">
                <div className="kr-steps-title">Шаги проекта</div>
                <div className={`kr-steps-sum ${Math.abs(sw - 100) < 1 ? 'kr-steps-sum--ok' : 'kr-steps-sum--bad'}`}>Сумма: {sw}</div>
              </div>
              {form.stages.length > 0 && (
                <div className="kr-steps-cols">
                  <span className="kr-steps-cols__check">✓</span>
                  <span className="kr-steps-cols__name">Название шага</span>
                  <span className="kr-steps-cols__weight">Вес, %</span>
                  <span className="kr-steps-cols__del" />
                </div>
              )}
              {form.stages.map((st, i) => (
                <div key={st.id || i} className="kr-step-row">
                  <input type="checkbox" checked={!!st.done} onChange={e => setSt(i, 'done', e.target.checked)} style={{ width: 16, height: 16, accentColor: accent, flexShrink: 0 }} />
                  <input value={st.name} onChange={e => setSt(i, 'name', e.target.value)} placeholder="Название шага" className="form-input form-input--sm" style={{ flex: 1 }} />
                  <input type="number" min={0} value={st.weight} onChange={e => setSt(i, 'weight', Number(e.target.value))} className="form-input form-input--sm form-input--center" style={{ width: 60 }} />
                  <button onClick={() => remSt(i)} className="kr-step-delete">×</button>
                </div>
              ))}
              <button onClick={addSt} className="kr-step-add">+ Добавить шаг</button>
            </div>
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
        <div className="modal-footer modal-footer--sticky">
          <button onClick={onClose} className="btn btn--secondary">Отмена</button>
          <button onClick={save} disabled={!canSave} className="btn btn--primary"
            style={{ background: canSave ? accent : '#e5e7eb', color: canSave ? 'white' : '#9ca3af', cursor: canSave ? 'pointer' : 'default' }}>
            Сохранить
          </button>
        </div>
      </div>
    </div>
    {confirmEl}
    </>
  );
}

// ── CONFIRM MODAL ─────────────────────────────────────────────────────────────
function ConfirmModal({ title, message, confirmLabel, onConfirm, onClose }) {
  const [busy, setBusy] = React.useState(false);
  const run = async () => { setBusy(true); try { await onConfirm(); } finally { setBusy(false); } };
  const { requestClose } = useModalClose({ isDirty: false, onClose });
  const overlay = useOverlayClose(requestClose);
  return (
    <div className="modal-overlay modal-overlay--z600" {...overlay}>
      <div onClick={e => e.stopPropagation()} className="modal-box modal-box--w380">
        <div className="confirm-body">
          <div className="confirm-title">{title}</div>
          <div className="confirm-message">{message}</div>
        </div>
        <div className="confirm-footer">
          <button onClick={onClose} disabled={busy} className="btn btn--secondary">Отмена</button>
          <button onClick={run} disabled={busy} className="btn btn--danger">{busy ? 'Удаляем…' : (confirmLabel || 'Удалить')}</button>
        </div>
      </div>
    </div>
  );
}

// ── KR ROW ────────────────────────────────────────────────────────────────────
function KRRow({ kr, goalId, editMode, onReload, accent, staleDays = 7 }) {
  const [modal, setModal] = useState(null);
  const [showNote, setShowNote] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const progress = kr.progress;
  const staleC = kr.updatedDaysAgo > staleDays ? '#dc2626' : kr.updatedDaysAgo > staleDays * 0.6 ? '#d97706' : '#10b981';
  let detail = null;
  if (kr.krType === 'BOOLEAN') detail = <span className="kr-detail" style={{ color: kr.done ? '#16a34a' : '#9ca3af', fontWeight: 600 }}>{kr.done ? '✓ Выполнено' : '○ Не выполнено'}</span>;
  else if (kr.krType === 'PROJECT') detail = <span className="kr-detail">{(kr.stages || []).filter(s => s.done).length}/{(kr.stages || []).length} шагов</span>;
  else detail = <span className="kr-detail">{fmtVal(kr.current, kr.unit)} / {fmtVal(kr.target, kr.unit)}</span>;
  const onSaved = () => { setModal(null); onReload(); };
  return (
    <>
      <div className="kr-row">
        <div className="kr-row__main">
          <div className="kr-weight-chip">{kr.weight}</div>
          <div className="kr-info">
            <div className="kr-name">{kr.name}</div>
            {kr.desc && <Markdown text={kr.desc} className="kr-desc" />}
            <div className="kr-detail-row">
              <div className="kr-bar-wrap"><ProgressBar value={progress} h={4} color={accent} /></div>
              <span className="kr-pct" style={{ color: accent }}>{progress}%</span>
              {detail}
            </div>
            {kr.zeroing && (
              <div className="kr-zeroing-note kr-zeroing-note--clamp" title={kr.zeroing}>
                <span className="kr-zeroing-note__icon">⊘</span>Критерий обнуления: {kr.zeroing}
              </div>
            )}
          </div>
          <Badge label={KR_TYPE_LABEL[kr.krType] || kr.krType} color={KR_TYPE_C[kr.krType]} />
          <span className="kr-updated" style={{ color: staleC }}>{kr.updatedDaysAgo === 0 ? 'сегодня' : `${kr.updatedDaysAgo}д назад`}</span>
          <span className="kr-notes-slot">
            {kr.note && <button onClick={() => setShowNote(!showNote)} className="kr-notes-btn">📝</button>}
          </span>
          {editMode === 'full' && <>
            <button onClick={() => setModal('edit')} className="kr-edit-btn">Редактировать</button>
            <button onClick={() => setConfirmDelete(true)} title="Удалить KR" className="kr-delete-btn">×</button>
          </>}
          {editMode === 'progress_only' && (
            <button onClick={() => setModal('progress')}
              style={{ padding: '5px 10px', border: `1px solid ${accent}`, borderRadius: 6, background: `${accent}10`, color: accent, fontSize: 12, fontWeight: 600, cursor: 'pointer', flexShrink: 0 }}>
              Обновить прогресс
            </button>
          )}
        </div>
        {showNote && kr.note && (
          <div className="kr-notes">
            <div className="kr-note">
              <div className="kr-note__content">
                <div className="kr-note__header">
                  <UserInfo name={kr.note.author} udid={kr.note.authorUdid} size={22} />
                  <span className="kr-note__date">{kr.note.date}</span>
                </div>
                <Markdown text={kr.note.text} className="kr-note__text" />
              </div>
            </div>
          </div>
        )}
      </div>
      {modal === 'progress' && <KRProgressModal kr={kr} onSave={onSaved} onClose={() => setModal(null)} accent={accent} />}
      {modal === 'edit' && <KREditModal kr={kr} goalId={goalId} onSave={onSaved} onClose={() => setModal(null)} accent={accent} />}
      {confirmDelete && <ConfirmModal title="Удалить Key Result?" message={`«${kr.name}» будет удалён без возможности восстановления.`}
        onConfirm={async () => { await apiDelete(`/api/v1/krs/${kr.id}`); setConfirmDelete(false); onReload(); }}
        onClose={() => setConfirmDelete(false)} />}
    </>
  );
}

// ── COMMENTS PANEL ────────────────────────────────────────────────────────────
// A single comment row. Unresolved comments expose a "resolve" action; resolved
// ones are visually dimmed and carry a resolver/date meta line with a "reopen" link.
function CommentRow({ c, onResolve, onUnresolve }) {
  const [busy, setBusy] = useState(false);
  const act = async fn => { setBusy(true); try { await fn(); } catch { } finally { setBusy(false); } };
  return (
    <div id={`comment-${c.id}`} className={`comment${c.resolved ? ' comment--resolved' : ''}`}>
      <AvatarWithUDID name={c.author} udid={c.authorUdid} size={28} />
      <div className="comment__content">
        <div className="comment__header">
          <span className="comment__author">{c.author}</span>
          <span className="comment__date">{c.date}</span>
          {c.resolved && <span className="comment__resolved-badge">✓ Решено</span>}
        </div>
        <Markdown text={c.text} className="comment__text" />
        {c.resolved ? (
          <div className="comment__resolved-meta">
            Решено{c.resolvedBy ? ` · ${c.resolvedBy}` : ''}{c.resolvedAt ? ` · ${c.resolvedAt}` : ''}
            {' · '}
            <button className="comment__link-btn" disabled={busy} onClick={() => act(() => onUnresolve(c.id))}>Вернуть</button>
          </div>
        ) : (
          <div className="comment__actions">
            <button className="comment__resolve-btn" disabled={busy} onClick={() => act(() => onResolve(c.id))}>✓ Отметить решённым</button>
          </div>
        )}
      </div>
    </div>
  );
}

function CommentsPanel({ comments, onAdd, onResolve, onUnresolve, me }) {
  const [text, setText] = useState(''); const [saving, setSaving] = useState(false);
  const submit = async () => {
    if (!text.trim()) return;
    setSaving(true); try { await onAdd(text.trim()); setText(''); } catch { } finally { setSaving(false); }
  };
  const hasText = !!text.trim();
  const list = comments || [];
  const open = list.filter(c => !c.resolved);
  const resolved = list.filter(c => c.resolved);
  return (
    <div className="comments-panel">
      <div className="comments-panel__title">
        Комментарии
        {open.length > 0 && <span className="comments-panel__unresolved">{open.length} нерешённых</span>}
      </div>
      {open.map((c, i) => (
        <CommentRow key={c.id || i} c={c} onResolve={onResolve} onUnresolve={onUnresolve} />
      ))}
      {resolved.length > 0 && (
        <div className="comments-panel__resolved-head">Решённые · {resolved.length}</div>
      )}
      {resolved.map((c, i) => (
        <CommentRow key={c.id || `r${i}`} c={c} onResolve={onResolve} onUnresolve={onUnresolve} />
      ))}
      <div className="comment-compose">
        <Avatar name={me?.display_name} avatarUrl={me?.avatar_url} size={28} />
        <div className="comment-compose__right">
          <MarkdownEditor value={text} onChange={setText} rows={3} placeholder="Контекст, блокер, заметка… (Cmd+Enter)"
            onKeyDown={e => { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) submit(); }}
            textareaClassName="form-textarea form-textarea--sm" textareaStyle={{ width: '100%', resize: 'vertical' }} />
          <div className="comment-submit-row">
            <button onClick={submit} disabled={!hasText || saving}
              className={`comment-submit ${hasText ? 'comment-submit--active' : 'comment-submit--disabled'}`}>
              {saving ? '…' : 'Отправить'}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}

// ── GOAL CARD ─────────────────────────────────────────────────────────────────
function GoalCard({ goal, editMode, onReload, onEditGoal, me, accent, currentTeamId, allTeams, dragProps, onReorderKR, staleDays = 7, periodStatus, greenThreshold = 80, deepLink = null }) {
  // A deep link (?goal/kr/comment) targeting this goal forces the relevant sections open.
  const isDeepTarget = deepLink && deepLink.goal === goal.id;
  // Goals without key results start expanded so the author's attention is drawn
  // to filling them in; goals that already have KRs start collapsed.
  const [showKR, setShowKR] = useState((goal.krs || []).length === 0 || !!(isDeepTarget && deepLink.kr));
  const [showCom, setShowCom] = useState(!!(isDeepTarget && deepLink.comment));
  const [newKR, setNewKR] = useState(false);
  const [krDrag, setKrDrag] = useState(null);
  const [goalDraggable, setGoalDraggable] = useState(false);
  const [confirmDeleteGoal, setConfirmDeleteGoal] = useState(false);
  const prog = goal.progress || 0;
  // "N дней без обновления" is an execution-phase signal: it applies only while
  // the team is in_progress ("в работе"). Drafts, goals awaiting validation and
  // closed periods are not being actively executed, so it is not meaningful for
  // them. Kept in sync with the Health Check-in "stale" category (bell).
  const staleTracked = periodStatus === 'in_progress';
  const isStale = staleTracked && goal.updatedDaysAgo > staleDays;
  const forecast = goal.progressMeta?.forecast ?? null;
  const hC = HEALTH_COLOR[healthOf(prog, isStale, forecast, greenThreshold)];
  const health = healthOf(prog, isStale, forecast, greenThreshold);
  const canEdit = editMode === 'full';
  const canReorderGoal = canEdit && !!dragProps;
  const { isDragging, ...rootDrag } = dragProps || {};
  const otherTeams = (goal.shareTeams || []).filter(t => t.id !== currentTeamId);
  const isShared = otherTeams.length > 0;
  const krWeightSum = (goal.krs || []).reduce((s, k) => s + (k.weight || 0), 0);
  const krWeightOff = krWeightSum !== 100;
  const krWeightDelta = 100 - krWeightSum;

  const addGoalComment = async text => { await apiPost(`/api/v1/goals/${goal.id}/comments`, { text }); onReload(); };
  const resolveComment = async commentId => { await apiPost(`/api/v1/goals/${goal.id}/comments/${commentId}/resolve`, {}); onReload(); };
  const unresolveComment = async commentId => { await apiPost(`/api/v1/goals/${goal.id}/comments/${commentId}/unresolve`, {}); onReload(); };
  const unresolvedCount = (goal.comments || []).filter(c => !c.resolved).length;
  // For a shared goal, deleting only detaches the current team (leaves the share); the goal stays
  // for the other participating teams. A goal that is not shared is deleted outright.
  const handleDeleteGoal = async () => {
    await apiDelete(isShared ? `/api/v1/goals/${goal.id}/share/${currentTeamId}` : `/api/v1/goals/${goal.id}`);
    onReload();
  };

  const cardClass = ['goal-card',
    isDragging ? 'goal-card--dragging' : '',
    canReorderGoal ? 'goal-card--reorderable' : '',
    isShared ? 'goal-card--shared' : '',
    isStale ? 'goal-card--stale' : '',
  ].filter(Boolean).join(' ');

  return (
    <div {...rootDrag} id={`goal-${goal.id}`} draggable={!!(canReorderGoal && goalDraggable)}
      onDragEnd={e => { setGoalDraggable(false); rootDrag.onDragEnd && rootDrag.onDragEnd(e); }}
      className={cardClass}>
      {canReorderGoal && (
        <div className="drag-handle" title="Перетащите для изменения порядка"
          onMouseDown={() => setGoalDraggable(true)} onMouseUp={() => setGoalDraggable(false)} onMouseLeave={() => setGoalDraggable(false)}>⋮⋮</div>
      )}
      <div className="goal-card__body">
        <div className="goal-card__meta">
          <PriBadge p={goal.priority} />
          <span className="goal-card__weight">вес {goal.weight}%</span>
          {otherTeams.length > 0 && <Badge label={`⇄ Общая · ${otherTeams.length + 1} команд`} color="#0891b2" />}
          <div className="goal-card__spacer" />
          {isStale && <Badge label={`⚠ ${goal.updatedDaysAgo}д без обновлений`} color="#d97706" bg="#fffbeb" />}
          {goal.owners.length > 0 && (
            <div className="goal-card__owner">
              <span className="goal-card__owner-label">Драйвер цели</span>
              {goal.owners.map(u => (
                <UserInfo key={u.udid || u.display_name} userRef={u} size={18} />
              ))}
            </div>
          )}
        </div>
        <div className="goal-card__title-row">
          <div onClick={canEdit ? () => onEditGoal(goal) : undefined}
            className={`goal-card__title${canEdit ? '' : ' goal-card__title--readonly'}`}>
            {goal.title}
            {canEdit && <span className="goal-card__edit-hint">✎</span>}
          </div>
          {canEdit && <button onClick={() => setConfirmDeleteGoal(true)} title="Удалить цель" className="goal-card__delete-btn">×</button>}
        </div>
        {goal.desc && <Markdown text={goal.desc} className="goal-card__desc" />}
        {otherTeams.length > 0 && (
          <div className="shared-banner">
            <span className="shared-banner__label">⇄ Общая с:</span>
            {[...(goal.shareTeams || []).filter(t => t.id === currentTeamId).map(t => ({ ...t, isSelf: true })), ...otherTeams].map(t => (
              <span key={t.id} className={`shared-pill${t.isSelf ? ' shared-pill--self' : ''}`}>
                {t.name}{t.isSelf && <span className="shared-pill__you"> · Вы</span>}
              </span>
            ))}
          </div>
        )}
        <div className="goal-card__progress">
          <div className="goal-card__progress-header">
            <div className="goal-card__progress-left">
              <span className="goal-card__progress-pct" style={{ color: hC }}>{prog}%</span>
              <span className="goal-card__health-badge" style={{ color: hC, background: `${hC}15` }}>
                {health === 'ahead' ? '▲ опережает' : health === 'on_track' ? '✓ в плане' : health === 'stale' ? '⚠ нет обновлений' : '▼ отстаёт'}
              </span>
            </div>
            <span className="goal-card__updated" style={{ color: isStale ? '#dc2626' : '#16a34a' }}>
              {'Обновлено: '}{goal.updatedDaysAgo === 0 ? 'сегодня' : `${goal.updatedDaysAgo}д назад`}
            </span>
          </div>
          {goal.progressMeta && (
            <>
              <ProgressBar value={prog} forecast={goal.progressMeta.forecast} h={9} color={hC} />
              <div className="goal-card__forecast-label">прогноз {goal.progressMeta.forecast}%</div>
            </>
          )}
        </div>
        <div className="goal-card__tags">
          <Badge label={goal.type === 'delivery' ? 'Delivery' : 'Discovery'} color={goal.type === 'delivery' ? '#374151' : '#7c3aed'} />
          {goal.focus && <Badge label={focusLabel(goal.focus)} color={FOCUS_COLORS[goal.focus] || FOCUS_COLORS.DEFAULT} />}
        </div>
      </div>
      <div className="goal-card__footer">
        <button onClick={() => setShowKR(!showKR)} className="goal-card__footer-btn">
          <span style={{ fontSize: 9 }}>{showKR ? '▲' : '▼'}</span>
          {showKR ? 'Скрыть KR' : `KR (${(goal.krs || []).length})`}
          {krWeightOff && <span className="kr-weight-badge" title="Сумма весов KR не равна 100%">⚠ {krWeightSum} %</span>}
        </button>
        <div className="goal-card__footer-divider" />
        <button onClick={() => setShowCom(!showCom)}
          className={`goal-card__footer-btn${(goal.comments || []).length > 0 ? ' goal-card__footer-btn--has-comments' : ''}`}>
          {(goal.comments || []).length > 0 ? `💬 ${goal.comments.length}` : '💬 Комментарии'}
          {unresolvedCount > 0 && <span className="comment-unresolved-badge" title={`${unresolvedCount} нерешённых`}>{unresolvedCount}</span>}
        </button>
      </div>
      {showKR && (
        <div className="kr-section">
          {krWeightOff && (
            <div className="kr-weight-warn">
              <span className="kr-weight-warn__icon">⚠</span>
              <span>
                Сумма весов KR = {krWeightSum}%, ожидается 100%
                {' · '}
                {krWeightDelta > 0 ? `не распределено ${krWeightDelta}%` : `превышено на ${-krWeightDelta}%`}
              </span>
            </div>
          )}
          {(goal.krs || []).map(kr => {
            const canReorderKR = canEdit && !!onReorderKR;
            const isKrDrag = krDrag === kr.id;
            return (
              <div key={kr.id} id={`kr-${kr.id}`}
                draggable={!!canReorderKR}
                onDragStart={canReorderKR ? (e) => { e.stopPropagation(); e.dataTransfer.effectAllowed = 'move'; e.dataTransfer.setData('text/plain', 'kr'); setKrDrag(kr.id); } : undefined}
                onDragOver={canReorderKR ? (e) => { if (krDrag && krDrag !== kr.id) { e.preventDefault(); e.stopPropagation(); e.dataTransfer.dropEffect = 'move'; } } : undefined}
                onDrop={canReorderKR ? (e) => { e.preventDefault(); e.stopPropagation(); if (krDrag && krDrag !== kr.id) onReorderKR(krDrag, kr.id); setKrDrag(null); } : undefined}
                onDragEnd={canReorderKR ? () => setKrDrag(null) : undefined}
                className={`kr-item${isKrDrag ? ' kr-item--dragging' : ''}${canReorderKR ? ' kr-item--reorderable' : ''}`}>
                {canReorderKR && <div className="kr-item__drag-handle">⋮⋮</div>}
                <KRRow kr={kr} goalId={goal.id} editMode={editMode} onReload={onReload} accent={accent} staleDays={staleDays} />
              </div>
            );
          })}
          {editMode === 'full' && <button onClick={() => setNewKR(true)} className="kr-add-btn">+ Добавить KR</button>}
        </div>
      )}
      {newKR && <KREditModal kr={null} goalId={goal.id} onSave={() => { setNewKR(false); onReload(); }} onClose={() => setNewKR(false)} accent={accent} />}
      {confirmDeleteGoal && <ConfirmModal
        title={isShared ? 'Открепить цель от команды?' : 'Удалить цель?'}
        message={isShared
          ? `Цель «${goal.title}» будет удалена из целей этой команды. У других команд-участников она сохранится.`
          : `«${goal.title}» и все её Key Results будут удалены без возможности восстановления.`}
        confirmLabel={isShared ? 'Открепить' : 'Удалить'}
        onConfirm={handleDeleteGoal} onClose={() => setConfirmDeleteGoal(false)} />}
      {showCom && (
        <div className="comments-section">
          <CommentsPanel comments={goal.comments} onAdd={addGoalComment} onResolve={resolveComment} onUnresolve={unresolveComment} me={me} />
        </div>
      )}
    </div>
  );
}

// ── GOAL MODAL ────────────────────────────────────────────────────────────────
// ── USER CACHE ────────────────────────────────────────────────────────────────
// Populated from UserRef objects embedded in API responses (owners, lead fields).
const _userByUdid = new Map();
const _userByName = new Map();

function _cacheUserRef(ref) {
  if (!ref) return;
  // Merge so richer fields (email, led_team from /api/v1/users) survive a later
  // minimal ref (udid/display_name/avatar_url from OKR/hierarchy payloads).
  if (ref.udid) {
    const prev = _userByUdid.get(ref.udid);
    _userByUdid.set(ref.udid, prev ? { ...prev, ...ref } : ref);
  }
  if (ref.display_name) {
    const prev = _userByName.get(ref.display_name);
    _userByName.set(ref.display_name, prev ? { ...prev, ...ref } : ref);
  }
}

// Lazily load a user's full details (email, led_team) by UDID and merge them
// into the cache. Deduped per UDID — at most one request, even on repeated hover.
const _userDetailFetched = new Map();
function _fetchUserDetail(udid) {
  if (!udid) return Promise.resolve(null);
  const cached = _userByUdid.get(udid);
  if (cached && cached.email !== undefined) return Promise.resolve(cached);
  if (_userDetailFetched.has(udid)) return _userDetailFetched.get(udid);
  const p = apiGet(`/api/v1/users?ids[]=${encodeURIComponent(udid)}`)
    .then(arr => {
      const item = Array.isArray(arr) ? (arr.find(u => u.udid === udid) || arr[0]) : null;
      if (item) _cacheUserRef(item);
      return item || null;
    })
    .catch(() => null);
  _userDetailFetched.set(udid, p);
  return p;
}

function _cacheUserRefsFromHierarchyNodes(nodes) {
  for (const node of nodes || []) {
    if (node.lead) _cacheUserRef(node.lead);
    _cacheUserRefsFromHierarchyNodes(node.children);
  }
}

function _cacheUserRefsFromOKR(data) {
  if (!data) return;
  if (data.team?.lead) _cacheUserRef(data.team.lead);
  for (const g of data.goals || []) {
    for (const u of g.owners || []) _cacheUserRef(u);
  }
}

function _cachedUsersList() {
  return Array.from(_userByName.values());
}

function UserAvatar({ user, size = 24 }) {
  if (user && user.avatar_url) {
    return <img src={user.avatar_url} width={size} height={size} alt=""
      style={{ borderRadius: '50%', objectFit: 'cover', flexShrink: 0, display: 'block' }} />;
  }
  return (
    <span className="user-avatar__fallback" style={{ width: size, height: size, fontSize: Math.round(size * 0.45) }}>
      {user && user.display_name ? user.display_name[0].toUpperCase() : '?'}
    </span>
  );
}

function UserSelector({ value, onChange, multiple = false, placeholder = 'Поиск пользователя…', fetchFn }) {
  const [q, setQ] = useState('');
  const [open, setOpen] = useState(false);
  const [hi, setHi] = useState(0);
  const [fetchedUsers, setFetchedUsers] = useState(null);
  const fetchTimer = useRef(null);
  const inputRef = useRef(null);
  const wrapRef = useRef(null);

  useEffect(() => { setHi(0); }, [q]);
  useEffect(() => {
    const h = e => { if (wrapRef.current && !wrapRef.current.contains(e.target)) setOpen(false); };
    document.addEventListener('mousedown', h);
    return () => document.removeEventListener('mousedown', h);
  }, []);

  useEffect(() => {
    if (!fetchFn || !open) return;
    clearTimeout(fetchTimer.current);
    fetchTimer.current = setTimeout(() => {
      fetchFn(q).then(data => {
        if (Array.isArray(data)) {
          data.forEach(u => _cacheUserRef(u));
          setFetchedUsers(data);
        }
      }).catch(() => { });
    }, q ? 200 : 0);
    return () => clearTimeout(fetchTimer.current);
  }, [q, open, fetchFn]);

  const qLow = q.toLowerCase();
  const users = fetchFn
    ? (fetchedUsers || [])
    : (qLow ? _cachedUsersList().filter(u => u.display_name?.toLowerCase().includes(qLow)) : _cachedUsersList());

  const handleQueryChange = newQ => { setQ(newQ); if (fetchFn) setFetchedUsers(null); };

  const values = multiple ? (Array.isArray(value) ? value : []) : (value ? [value] : []);
  const findUserByValue = v => multiple ? (_userByUdid.get(v) || users.find(u => u.udid === v)) : (_userByName.get(v) || users.find(u => u.display_name === v));
  const available = multiple ? users.filter(u => !values.includes(u.udid)) : users;

  const select = u => {
    if (multiple) { if (!values.includes(u.udid)) onChange([...values, u.udid]); }
    else { onChange(u.display_name); setOpen(false); }
    setQ(''); inputRef.current?.focus();
  };
  const remove = udid => { if (multiple) onChange(values.filter(v => v !== udid)); else onChange(''); };
  const onKey = e => {
    if (e.key === 'ArrowDown') { e.preventDefault(); setOpen(true); setHi(h => Math.min(available.length - 1, h + 1)); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); setHi(h => Math.max(0, h - 1)); }
    else if (e.key === 'Enter') { e.preventDefault(); if (open && available[hi]) select(available[hi]); }
    else if (e.key === 'Escape') { if (open) { e.preventDefault(); setOpen(false); } }
    else if (e.key === 'Backspace' && !q && multiple && values.length > 0) remove(values[values.length - 1]);
  };

  return (
    <div ref={wrapRef} className="user-selector">
      <div onClick={() => { setOpen(true); inputRef.current?.focus(); }}
        className={`user-selector__field${open ? ' user-selector__field--open' : ''}`}>
        {values.map(v => {
          const u = findUserByValue(v);
          return (
            <span key={v} className="user-chip">
              <UserAvatar user={u} size={18} />
              <span className="user-chip__name">{u?.display_name || v}</span>
              <button type="button" onClick={e => { e.stopPropagation(); remove(v); }} className="user-chip__remove">×</button>
            </span>
          );
        })}
        {(multiple || values.length === 0) && (
          <input ref={inputRef} value={q} onChange={e => { handleQueryChange(e.target.value); setOpen(true); }}
            onFocus={() => setOpen(true)} onKeyDown={onKey}
            placeholder={values.length === 0 ? placeholder : 'Ещё…'}
            className="user-selector__input" />
        )}
      </div>
      {open && (
        <div className="user-selector__dropdown">
          {available.length === 0
            ? <div className="user-selector__empty">{q ? 'Пользователи не найдены' : 'Список пуст'}</div>
            : available.slice(0, 20).map((u, i) => (
              <div key={u.udid} onClick={() => select(u)} onMouseEnter={() => setHi(i)}
                className={`user-selector__option${i === hi ? ' user-selector__option--hi' : ''}`}>
                <UserAvatar user={u} size={26} />
                <div className="user-selector__option-info">
                  <span className="user-selector__option-name">{u.display_name}</span>
                  {u.led_team && <span className="user-selector__option-team">{u.led_team}</span>}
                </div>
              </div>
            ))
          }
        </div>
      )}
    </div>
  );
}

// ── USER INFO (avatar + name + hover popup) ───────────────────────────────────
// Accepts a `userRef` object {udid,display_name,avatar_url} (from API responses),
// or separate `name`/`udid` props for comment/note authors and legacy call sites.
// Renders from the local cache; on hover the popup lazily loads full details
// (email, led_team) by UDID via _fetchUserDetail (deduped, at most one request).
function UserInfo({ userRef, name: nameProp, udid: udidProp, size = 22 }) {
  const initName = userRef?.display_name ?? nameProp ?? '';
  const initUdid = userRef?.udid || udidProp || '';
  const initAv = userRef?.avatar_url || null;

  if (userRef) _cacheUserRef(userRef);

  const [popup, setPopup] = useState(null);
  const [, force] = useState(0);
  const ref = useRef();
  const timer = useRef();

  const cached = initUdid ? _userByUdid.get(initUdid) : _userByName.get(initName);
  const name = cached?.display_name || initName || '?';
  const show = () => {
    clearTimeout(timer.current);
    // Fetch email/led_team lazily so the popup can show full details.
    if (initUdid) _fetchUserDetail(initUdid).then(d => { if (d) force(n => n + 1); });
    if (!ref.current) return;
    const r = ref.current.getBoundingClientRect();
    const left = Math.max(8, Math.min(r.left, window.innerWidth - 248));
    setPopup({ top: r.bottom + 6, left });
  };
  const hide = () => { timer.current = setTimeout(() => setPopup(null), 150); };

  const initials = name.split(' ').slice(0, 2).map(w => w[0] || '').join('').toUpperCase() || '?';
  const palette = ['#2563eb', '#7c3aed', '#059669', '#d97706', '#dc2626', '#0891b2', '#be185d', '#6366f1'];
  const bg = palette[name.charCodeAt(0) % palette.length] || palette[0];
  const av = cached?.avatar_url || initAv;

  const popupEl = popup && ReactDOM.createPortal(
    <span className="uinfo__popup" style={{ top: popup.top, left: popup.left }}
      onMouseEnter={() => clearTimeout(timer.current)} onMouseLeave={hide}>
      {av
        ? <img src={av} width={44} height={44} className="uinfo__popup-avatar" alt="" />
        : <span className="uinfo__popup-initials" style={{ width: 44, height: 44, background: bg, fontSize: 17 }}>{initials}</span>
      }
      <span className="uinfo__popup-body">
        <span className="uinfo__popup-name">{name}</span>
        {cached?.email && <span className="uinfo__popup-email">{cached.email}</span>}
        {cached?.led_team && <span className="uinfo__popup-team">Руководит: {cached.led_team}</span>}
      </span>
    </span>,
    document.body
  );

  return (
    <span ref={ref} className="uinfo" onMouseEnter={show} onMouseLeave={hide}>
      {av
        ? <img src={av} width={size} height={size} className="uinfo__img" alt="" />
        : <span className="uinfo__initials" style={{ width: size, height: size, background: bg, fontSize: Math.round(size * 0.42) }}>{initials}</span>
      }
      <span className="uinfo__name">{name}</span>
      {popupEl}
    </span>
  );
}

// Builds the multipart payload for goal create/update from the modal form.
// Shared by the edit path and the create-retry path so both persist the same fields.
function goalFormData(form, teamId) {
  const fd = new FormData();
  fd.append('title', form.title.trim());
  fd.append('description', form.desc || '');
  fd.append('priority', form.priority);
  fd.append('weight', String(Number(form.weight) || 0));
  fd.append('work_type', form.type === 'delivery' ? 'Delivery' : 'Discovery');
  fd.append('focus_type', form.focus);
  (form.ownerUDIDs || []).forEach(u => fd.append('owner_udids', u));
  fd.append('team_id', String(teamId));
  return fd;
}

// Builds the goal_shares targets for the edit path. The /share endpoint replaces the COMPLETE
// set of non-owner participants, so the editing team must stay in the set (unless it is the
// owner, which is tracked on the goal itself and never stored as a share). Existing per-team
// weights are preserved so editing one team's weight never alters another team's weight; only
// newly added teams fall back to a default. form.shareTeamIds excludes the editing team, so for
// a non-owner editor it would otherwise include the owner and drop the editor itself.
function goalShareTargets(form, goal, teamId) {
  const ownerId = goal.teamId;
  const existingWeight = {};
  (goal.shareTeams || []).forEach(t => { existingWeight[t.id] = t.weight; });
  const teamIds = new Set(form.shareTeamIds || []);
  if (teamId !== ownerId) teamIds.add(teamId);
  teamIds.delete(ownerId);
  return [...teamIds].map(id => ({
    team_id: id,
    weight: id === teamId ? (Number(form.weight) || 0) : (existingWeight[id] != null ? existingWeight[id] : 100),
  }));
}

// Reports whether the set of non-owner participating teams differs from what is currently
// persisted on the goal. Lets the edit path skip the /share call (a full goal_shares replace)
// when only the weight or other goal fields changed.
function shareTeamsChanged(form, goal, teamId) {
  const desired = new Set(goalShareTargets(form, goal, teamId).map(t => t.team_id));
  const current = new Set((goal.shareTeams || []).map(t => t.id).filter(id => id !== goal.teamId));
  if (desired.size !== current.size) return true;
  for (const id of desired) if (!current.has(id)) return true;
  return false;
}

function GoalModal({ goal, teamId, periodId, teamName, periodName, existingGoals, me, onSave, onClose, accent, allTeams }) {
  const isEdit = !!goal;
  const usedWeight = (existingGoals || []).filter(g => !isEdit || g.id !== goal?.id).reduce((s, g) => s + g.weight, 0);
  const wasShared = isEdit && (goal.shareTeams || []).filter(t => t.id !== teamId).length > 0;
  const [form, setForm] = useState(goal
    ? { shareTeamIds: (goal.shareTeams || []).filter(t => t.id !== teamId).map(t => t.id), ...goal, shared: wasShared, ownerUDIDs: (goal.owners || []).map(u => u.udid).filter(Boolean) }
    : { title: '', desc: '', priority: 'P1', weight: Math.max(0, Math.min(20, 100 - usedWeight)), type: 'delivery', focus: 'PROFITABILITY', shared: false, shareTeamIds: [], ownerUDIDs: [] });
  const [saving, setSaving] = useState(false);
  const [confirmUnshare, setConfirmUnshare] = useState(false);
  // Remembers the goal created in this modal session so that, if the optional
  // share step fails after the goal was already created, a retry only re-runs
  // the share instead of creating a duplicate goal.
  const createdGoalIdRef = useRef(null);
  const set = (k, v) => setForm(f => ({ ...f, [k]: v }));
  // Weight may be blank while editing; coerce to a number for arithmetic/save. A zero (or blank)
  // weight is a valid draft state, so it never gates the save.
  const weightNum = Number(form.weight) || 0;
  const totalAfter = usedWeight + (isEdit ? weightNum - (goal?.weight || 0) : weightNum);
  // Sum over 100% is allowed (draft in progress) — it only surfaces a non-blocking warning,
  // never gates the save.
  const overWeight = totalAfter > 100;
  const valid = form.title.trim() && (!form.shared || (form.shareTeamIds || []).length > 0);
  // Turning the "Общая цель" toggle off on an already-shared goal removes the goal from THIS
  // team's list (leaves the share), so it needs explicit confirmation before saving.
  const leavingShare = isEdit && wasShared && !form.shared;
  const performSave = async () => {
    if (!valid || saving) return;
    setSaving(true);
    try {
      if (isEdit) {
        if (leavingShare) { await apiDelete(`/api/v1/goals/${goal.id}/share/${teamId}`); onSave(); return; }
        await apiForm(`/api/v1/goals/${goal.id}`, goalFormData(form, teamId));
        // The per-team weight is already persisted by the goal update above. /share is only
        // needed when the set of participating teams actually changed — re-sending it on a plain
        // weight edit would be a redundant full replace of all goal_shares.
        if (form.shared && (form.shareTeamIds || []).length > 0 && shareTeamsChanged(form, goal, teamId)) {
          await apiPost(`/api/v1/goals/${goal.id}/share`, { targets: goalShareTargets(form, goal, teamId) });
        }
      } else {
        let newGoalId = createdGoalIdRef.current;
        if (!newGoalId) {
          const created = await apiPost(`/api/v1/teams/${teamId}/goals`, {
            period_id: periodId, title: form.title.trim(), description: form.desc || '',
            priority: form.priority, weight: Number(form.weight) || 0,
            work_type: form.type === 'delivery' ? 'Delivery' : 'Discovery',
            focus_type: form.focus, owner_udids: form.ownerUDIDs || [],
          });
          if (!created || !created.id) throw new Error('не удалось создать цель');
          newGoalId = created.id;
          createdGoalIdRef.current = newGoalId;
        } else {
          // Goal was created on a previous attempt but a later step failed; persist
          // any edits made since before retrying the share, so they aren't dropped.
          await apiForm(`/api/v1/goals/${newGoalId}`, goalFormData(form, teamId));
        }
        if (form.shared && (form.shareTeamIds || []).length > 0) {
          await apiPost(`/api/v1/goals/${newGoalId}/share`, { targets: (form.shareTeamIds || []).map(id => ({ team_id: id, weight: 100 })) });
        }
      }
      onSave();
    } catch (e) { alert('Ошибка: ' + e.message); }
    finally { setSaving(false); }
  };
  const save = async () => {
    if (!valid || saving) return;
    if (leavingShare) { setConfirmUnshare(true); return; }
    await performSave();
  };
  const canSave = valid && !saving;
  const initialFormRef = useRef(null);
  if (initialFormRef.current === null) initialFormRef.current = JSON.stringify(form);
  const isDirty = JSON.stringify(form) !== initialFormRef.current;
  const { requestClose, confirmEl } = useModalClose({ isDirty, canSave, onSave: save, onClose });
  const overlay = useOverlayClose(requestClose);
  return (
    <>
    <div className="modal-overlay modal-overlay--z400" {...overlay}>
      <div onClick={e => e.stopPropagation()} className="modal-box modal-box--w600">
        <div className="modal-header modal-header--sticky modal-header--goal">
          <div>
            <div className="modal-title--goal">{isEdit ? 'Редактировать цель' : 'Новая цель'}</div>
            <div className="modal-subtitle">{periodName} · {teamName}</div>
          </div>
          <button onClick={requestClose} className="modal-close modal-close--lg">×</button>
        </div>
        <div className="modal-body modal-body--goal">
          <div className="form-group">
            <FieldLabel required hint="Objective — качественное описание того, чего команда хочет достичь. Без цифр (они в KR).">Название</FieldLabel>
            <input value={form.title} onChange={e => set('title', e.target.value)} placeholder="Чего хотим достичь?" className="form-input form-input--goal" />
          </div>
          <div className="form-group">
            <FieldLabel hint="Контекст, почему эта цель важна. Не дублируйте название.">Описание</FieldLabel>
            <MarkdownEditor value={form.desc} onChange={v => set('desc', v)} rows={3} placeholder="Дополнительный контекст…" textareaClassName="form-textarea" />
          </div>
          <div className="form-row">
            <div className="form-col">
              <FieldLabel hint="Относительная важность: P0 — must-have, P1 — высокий приоритет, P2 — важная, P3 — желательная.">Приоритет</FieldLabel>
              <div className="seg-group">
                {['P0', 'P1', 'P2', 'P3'].map(p => {
                  const c = { P0: '#dc2626', P1: '#d97706', P2: '#2563eb', P3: '#6b7280' }[p];
                  const sel = form.priority === p;
                  return (
                    <button key={p} onClick={() => set('priority', p)} className="seg-btn"
                      style={{ borderColor: sel ? c : '#e5e7eb', background: sel ? `${c}12` : 'white', color: sel ? c : '#6b7280' }}>{p}</button>
                  );
                })}
              </div>
            </div>
            <div className="form-col--w140">
              <FieldLabel hint="Доля цели в общем результате команды. Сумма весов = 100%.">
                <span>Вес <span style={{ fontWeight: 400, color: overWeight ? '#d97706' : '#9ca3af', fontSize: 12 }}>({totalAfter}/100)</span></span>
              </FieldLabel>
              <div className="form-weight-wrap">
                <input type="number" min={0} max={100} value={form.weight}
                  onChange={e => { const v = e.target.value; set('weight', v === '' ? '' : Math.max(0, Math.min(100, Number(v)))); }}
                  className="form-input form-input--weight" />
                <span className="form-weight-pct">%</span>
              </div>
              {overWeight && <div className="form-error-msg" style={{ color: '#d97706' }}>Сумма весов больше 100% — сохранить можно</div>}
            </div>
          </div>
          <div className="form-row">
            <div className="form-col">
              <FieldLabel hint="Delivery — известный результат. Discovery — исследование гипотезы.">Тип работы</FieldLabel>
              <div className="seg-group">
                {['delivery', 'discovery'].map(t => {
                  const sel = form.type === t;
                  return (
                    <button key={t} onClick={() => set('type', t)} className="seg-btn"
                      style={{ borderColor: sel ? accent : '#e5e7eb', background: sel ? `${accent}10` : 'white', color: sel ? accent : '#6b7280' }}>{t}</button>
                  );
                })}
              </div>
            </div>
            <div className="form-col">
              <FieldLabel>Фокус</FieldLabel>
              <select value={form.focus} onChange={e => set('focus', e.target.value)} className="form-select">
                {FOCUS_OPTIONS.map(f => <option key={f} value={f}>{focusLabel(f)}</option>)}
              </select>
            </div>
          </div>
          <div className="form-group">
            <FieldLabel>Драйвер цели</FieldLabel>
            <UserSelector multiple
              value={form.ownerUDIDs}
              onChange={arr => set('ownerUDIDs', arr)}
              fetchFn={q => apiGet(`/api/v1/users?q=${encodeURIComponent(q)}&scope_team_id=${teamId}`)}
              placeholder="Добавить драйвера цели" />
          </div>
          <div className="toggle-box">
            <label className="toggle-row">
              <div onClick={() => set('shared', !form.shared)} className={`toggle-track${form.shared ? ' toggle-track--on' : ''}`}>
                <div className={`toggle-knob${form.shared ? ' toggle-knob--on' : ''}`} />
              </div>
              <div className="toggle-text">
                <div className="toggle-title">Общая цель</div>
                <div className="toggle-subtitle">Разделить цель с другими командами</div>
              </div>
            </label>
            {form.shared && (
              <div className="toggle-content">
                <div className="toggle-content__label">С какими командами <span className="toggle-content__required">*</span></div>
                <TeamCombobox selectedIds={form.shareTeamIds || []} onChange={ids => set('shareTeamIds', ids)} excludeId={teamId} accent={accent} allTeams={allTeams} />
              </div>
            )}
          </div>
        </div>
        <div className="modal-footer modal-footer--goal">
          <button onClick={onClose} className="btn btn--secondary" style={{ padding: '10px 20px', fontSize: 14 }}>Отмена</button>
          <button onClick={save} disabled={!canSave} className="btn btn--primary"
            style={{ padding: '10px 28px', fontSize: 14, background: canSave ? accent : '#e5e7eb', color: canSave ? 'white' : '#9ca3af', cursor: canSave ? 'pointer' : 'default' }}>
            {saving ? 'Сохраняем…' : isEdit ? 'Сохранить' : 'Создать цель'}
          </button>
        </div>
      </div>
      {confirmUnshare && (
        <ConfirmModal
          title="Сделать цель не общей?"
          message={`Цель «${form.title}» будет удалена из целей команды «${teamName}». У других команд-участников она сохранится.`}
          confirmLabel="Убрать из команды"
          onConfirm={performSave}
          onClose={() => setConfirmUnshare(false)} />
      )}
    </div>
    {confirmEl}
    </>
  );
}

// ── PERIOD SELECT ─────────────────────────────────────────────────────────────
const TRK_PERIOD_STATUS = {
  future: { label: 'Планируется', dot: 'rgb(59,130,196)', bg: 'rgba(59,130,196,0.15)', fg: 'rgb(59,130,196)' },
  active: { label: 'В работе', dot: 'rgb(31,157,85)', bg: 'rgba(31,157,85,0.15)', fg: 'rgb(31,157,85)' },
  closed: { label: 'Закрыто', dot: 'rgb(107,114,128)', bg: 'rgba(107,114,128,0.14)', fg: 'rgb(107,114,128)' },
};
function fmtPeriodDate(iso) {
  if (!iso) return '';
  return new Date(iso).toLocaleDateString('ru-RU', { day: '2-digit', month: '2-digit', year: '2-digit' });
}
function fmtDateRange(a, b) { return `${fmtPeriodDate(a)} – ${fmtPeriodDate(b)}`; }
function PeriodSelect({ periods, periodId, onChange }) {
  const [open, setOpen] = useState(false);
  const [pos, setPos] = useState(null);
  const triggerRef = useRef(null);
  const cur = periods.find(p => p.id === periodId);
  const st = cur && TRK_PERIOD_STATUS[cur.status];
  const openMenu = () => {
    const r = triggerRef.current.getBoundingClientRect();
    setPos({ top: r.bottom + 6, left: r.left });
    setOpen(true);
  };
  return <div className="period-select" style={{ position: 'relative' }}>
    <button ref={triggerRef} type="button" className="period-select__trigger" onClick={() => open ? setOpen(false) : openMenu()}>
      {st && <span className="period-select__dot" style={{ background: st.dot }} />}
      <span className="period-select__name">{cur ? cur.name : '—'}</span>
      <span className="period-select__chev">▾</span>
    </button>
    {open && pos && <>
      <div className="period-select__backdrop" onClick={() => setOpen(false)} />
      <div className="period-select__menu" style={{ top: pos.top, left: pos.left }}>
        <div className="period-select__group">Актуальные и будущие</div>
        {periods.map(p => {
          const s = TRK_PERIOD_STATUS[p.status] || TRK_PERIOD_STATUS.closed;
          return <button key={p.id} type="button"
            className={'period-select__item' + (p.id === periodId ? ' is-selected' : '')}
            onClick={() => { onChange(p.id); setOpen(false); }}>
            <span className="period-select__indent" style={{ width: p.depth * 12 }} />
            <span className="period-select__dot" style={{ background: s.dot }} />
            <span className="period-select__item-name">{p.name}</span>
            <span className="period-select__range">{fmtDateRange(p.start_date, p.end_date)}</span>
            <span className="period-select__badge-wrap"><span className="period-select__badge" style={{ background: s.bg, color: s.fg }}>{s.label}</span></span>
          </button>;
        })}
      </div>
    </>}
  </div>;
}

// ── SIDEBAR NODE ──────────────────────────────────────────────────────────────
function SidebarNode({ node, depth, selectedId, onSelect, expanded, toggle, accent, behindMargin, greenThreshold, favSet, onToggleFav }) {
  const ch = node.children || [];
  const isExp = expanded[node.id] !== false;
  const isSel = selectedId === node.id;
  const prog = node.progress;
  const dotC = TEAM_TYPE_COLOR[node.type] || HEALTH_COLOR.no_goals;
  const pctC = sidebarProgressColor(prog, node.forecast, node.status, behindMargin, greenThreshold);
  const pad = 14 + depth * 13;
  const isFav = favSet && favSet.has(favId(node.id));
  const nameClass = ['sidebar-node__name',
    depth === 0 ? 'sidebar-node__name--d0' : depth === 1 ? 'sidebar-node__name--d1' : 'sidebar-node__name--dx',
    isSel ? 'sidebar-node__name--selected' : '',
  ].filter(Boolean).join(' ');
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
        {onToggleFav && <span
          onClick={e => { e.stopPropagation(); onToggleFav(node.id); }}
          className={`sidebar-node__star${isFav ? ' sidebar-node__star--on' : ''}`}
          title={isFav ? 'Убрать из избранного' : 'В избранное'}>{isFav ? '★' : '☆'}</span>}
        {prog != null && <span className="sidebar-node__progress" style={{ color: isSel ? '#c4b5fd' : pctC }}>{prog}%</span>}
      </div>
      {isExp && ch.map(c => <SidebarNode key={c.id} node={c} depth={depth + 1} selectedId={selectedId} onSelect={onSelect} expanded={expanded} toggle={toggle} accent={accent} behindMargin={behindMargin} greenThreshold={greenThreshold} favSet={favSet} onToggleFav={onToggleFav} />)}
    </div>
  );
}

// ── CHILD CARD ────────────────────────────────────────────────────────────────
function ChildCard({ item, onSelect, greenThreshold = 80 }) {
  const prog = item.progress_meta ? item.progress_meta.actual : null;
  const forecast = item.progress_meta ? item.progress_meta.forecast : null;
  const health = healthOf(prog, false, forecast, greenThreshold);
  const hC = HEALTH_COLOR[health];
  const goalsCount = item.goals_count || 0;
  const highPri = item.high_priority_count || 0;
  return (
    <div onClick={() => onSelect(item.team.id)} className="child-card">
      <div className="child-card__header">
        <div className="child-card__info">
          <div className="child-card__name">{item.team.name}</div>
          {item.team.lead && (
            <div className="child-card__lead">
              <UserInfo userRef={item.team.lead} size={16} />
            </div>
          )}
        </div>
        <span className="child-card__health" style={{ color: hC, background: `${hC}15` }}>{HEALTH_LABEL[health]}</span>
      </div>
      {goalsCount > 0 ? (
        <>
          <div className="child-card__goals-row">
            <span className="child-card__goals-label">
              {goalsCount} {goalsCount === 1 ? 'цель' : goalsCount < 5 ? 'цели' : 'целей'} · <span className="child-card__goals-status">{item.status_label}</span>
            </span>
            <span className="child-card__goals-pct" style={{ color: hC }}>{prog ?? 0}%</span>
          </div>
          <ProgressBar value={prog || 0} forecast={forecast} h={5} color={hC} />
          {highPri > 0 && <div className="child-card__priority">● {highPri} приоритетных P0–P1</div>}
        </>
      ) : (
        <div className="child-card__empty">Цели не добавлены</div>
      )}
    </div>
  );
}

// ── CLUSTER VIEW ──────────────────────────────────────────────────────────────
function ClusterView({ overview, onSelect, greenThreshold = 80 }) {
  if (!overview) return <div className="cluster-loading">Загрузка…</div>;
  const avg = overview.average_progress || 0;
  const avgForecast = overview.progress_meta?.forecast ?? null;
  const hC = HEALTH_COLOR[healthOf(avg, false, avgForecast, greenThreshold)];
  const items = overview.children_summary?.items || [];
  return (
    <div>
      <div className="cluster-overview">
        <div className="cluster-overview__left">
          <div className="cluster-overview__label">Прогресс</div>
          <div className="cluster-overview__pct" style={{ color: hC }}>{avg}%</div>
        </div>
        <div className="cluster-overview__right">
          <div className="cluster-overview__bar-header">
            <span className="cluster-overview__teams-label">{overview.teams_with_goals} из {items.length} с целями</span>
            <span className="cluster-overview__health-label" style={{ color: hC }}>{HEALTH_LABEL[healthOf(avg, false, avgForecast, greenThreshold)]}</span>
          </div>
          {overview.progress_meta && (
            <>
              <ProgressBar value={avg} forecast={overview.progress_meta.forecast} h={10} color={hC} />
              <div className="cluster-overview__forecast">прогноз {overview.progress_meta.forecast}%</div>
            </>
          )}
        </div>
      </div>
      <div className="cluster-grid">
        {items.map(item => <ChildCard key={item.team.id} item={item} onSelect={onSelect} greenThreshold={greenThreshold} />)}
      </div>
    </div>
  );
}

// ── HEALTH CHECK-IN ───────────────────────────────────────────────────────────
const HCI_CAT_META = {
  stale: { icon: '🕐', label: 'Нет обновлений', color: '#f59e0b' },
  no_goals: { icon: '○', label: 'Нет целей', color: '#6b7280' },
  awaiting_validation: { icon: '○', label: 'Ожидают валидации', color: '#6b7280' },
  formation_errors: { icon: '⚠', label: 'Ошибки формирования', color: '#ef4444' },
  lagging: { icon: '▼', label: 'Отстающие', color: '#3b82f6' },
  comments: { icon: '💬', label: 'Комментарии', color: '#8b5cf6' },
};
const HCI_CAT_ORDER = ['stale', 'no_goals', 'awaiting_validation', 'formation_errors', 'lagging', 'comments'];
const HCI_ACTION_LABEL = {
  stale: '→ Обновить прогресс',
  no_goals: '→ Перейти к команде',
  awaiting_validation: '→ Перейти к команде',
  formation_errors: '→ Исправить',
  lagging: '→ Перейти к цели',
  comments: '→ Перейти к комментарию',
};

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

function HealthCheckInButton({ data, onClick }) {
  if (!data || !data.has_scope) return null;
  const count = data.total_problems;
  return (
    <button className="hci-button" onClick={onClick}>
      <span>⚡ Health Check-in</span>
      <span className={`hci-badge${count === 0 ? ' hci-badge--zero' : ''}`}>{count}</span>
    </button>
  );
}

function formatHCIErrorType(errType, item) {
  const labels = {
    weight_sum_not_100: `Сумма весов целей: ${item.actual_weight_sum ?? '?'}% (должно быть 100%)`,
    no_krs: 'У цели нет ключевых результатов',
    kr_weight_sum_not_100: 'Сумма весов KR ≠ 100%',
    project_no_stages: 'PROJECT KR без шагов',
    project_stage_weight_sum_not_100: 'Сумма весов шагов ≠ 100%',
    kr_zero_range: 'Нулевой диапазон (start = target)',
    kr_no_title: 'KR без названия',
  };
  return labels[errType] || errType;
}

function HealthCheckInPanel({ data, open, onClose }) {
  const [filter, setFilter] = useState(null);
  if (!data) return null;

  const subtitle = (() => {
    if (data.total_problems > 0) return `Найдено проблем: ${data.total_problems}`;
    const lagging = data.categories?.lagging?.count ?? 0;
    if (lagging > 0) return 'Проблем нет · есть отстающие цели';
    return 'Всё в порядке';
  })();

  // Для категории comments «объём» = нерешённые + мои решённые (у неё нет items/count-семантики badge).
  const catVisibleCount = (k) => {
    if (k === 'comments') {
      const c = data.categories?.comments;
      return (c?.unresolved?.length || 0) + (c?.resolved?.length || 0);
    }
    return data.categories?.[k]?.count ?? 0;
  };
  const nonEmptyCats = HCI_CAT_ORDER.filter(k => catVisibleCount(k) > 0);
  const counterCats = HCI_CAT_ORDER.filter(k => data.categories?.[k]?.in_counter);
  // Секция «Комментарии» видна по умолчанию, даже если не в счётчике (in_counter=false).
  const commentsNonEmpty = nonEmptyCats.includes('comments');
  const baseCats = commentsNonEmpty && !counterCats.includes('comments')
    ? [...counterCats, 'comments'] : counterCats;
  const visibleCats = filter ? [filter] : baseCats;

  return (
    <>
      {open && <div className="hci-backdrop" onClick={onClose} />}
      <div className={`hci-panel${open ? ' hci-panel--open' : ''}`}>
        <div className="hci-panel__header">
          <div style={{ flex: 1 }}>
            <p className="hci-panel__title">⚡ Health Check-in</p>
            <p className="hci-panel__subtitle">{subtitle}</p>
          </div>
          <button className="hci-panel__close" onClick={onClose}>✕</button>
        </div>

        {nonEmptyCats.length > 0 && (
          <div className="hci-chips">
            <button
              className={`hci-chip${!filter ? ' hci-chip--active' : ''}`}
              onClick={() => setFilter(null)}>
              Все · {data.total_problems}
            </button>
            {nonEmptyCats.map(k => {
              const cat = data.categories[k];
              const meta = HCI_CAT_META[k];
              const isActive = filter === k;
              const chipStyle = isActive
                ? { background: meta.color, borderColor: meta.color, color: '#fff' }
                : { borderColor: meta.color + '60', color: meta.color };
              return (
                <button key={k}
                  className="hci-chip"
                  style={chipStyle}
                  onClick={() => setFilter(isActive ? null : k)}>
                  {meta.icon} {meta.label} · {catVisibleCount(k)}
                </button>
              );
            })}
          </div>
        )}

        <div className="hci-body">
          {visibleCats.every(k => catVisibleCount(k) === 0) ? (
            <div className="hci-empty">
              <span className="hci-empty__icon">{filter ? '🔍' : '✅'}</span>
              <span>{filter ? 'По выбранному фильтру ничего нет' : 'Всё ok'}</span>
            </div>
          ) : (
            visibleCats.map(k => {
              const cat = data.categories?.[k];
              if (!cat) return null;

              if (k === 'comments') {
                const unresolved = cat.unresolved || [];
                const resolved = cat.resolved || [];
                if (unresolved.length === 0 && resolved.length === 0) return null;
                const cmeta = HCI_CAT_META[k];
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
                    <a className="hci-item__action"
                      href={buildTargetURL({ team_id: item.team_id, period_id: data.period_id, goal_id: item.goal_id, comment_id: item.comment_id })}>
                      {HCI_ACTION_LABEL[k]}
                    </a>
                  </div>
                );
                return (
                  <div key={k} className="hci-section">
                    <div className="hci-section__header" style={{ color: cmeta.color }}>
                      <span>{cmeta.icon}</span><span>{cmeta.label}</span>
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
              const meta = HCI_CAT_META[k];
              const byTeam = {};
              for (const item of cat.items) {
                if (!byTeam[item.team_id]) byTeam[item.team_id] = { name: item.team_name, path: item.team_path, items: [] };
                byTeam[item.team_id].items.push(item);
              }
              return (
                <div key={k} className="hci-section">
                  <div className="hci-section__header" style={{ color: meta.color }}>
                    <span>{meta.icon}</span>
                    <span>{meta.label}</span>
                    <span className="hci-section__count">{cat.count}</span>
                  </div>
                  {Object.entries(byTeam).map(([teamIdStr, group]) => (
                    <div key={teamIdStr} className="hci-team">
                      <div className="hci-team__name">
                        <span>▸</span>
                        <span>{group.path?.join(' › ') || group.name}</span>
                      </div>
                      {group.items.map((item, idx) => (
                        <div key={idx} className="hci-item">
                          {item.goal_title && <div className="hci-item__title">{item.goal_title}</div>}
                          {item.days_since_update > 0 && <div className="hci-item__meta">{item.days_since_update} дн. без обновлений</div>}
                          {item.error_type && <div className="hci-item__meta">{formatHCIErrorType(item.error_type, item)}</div>}
                          {item.progress !== undefined && item.expected_pace !== undefined && (
                            <div className="hci-item__meta">Прогресс: {item.progress}% · Ожидалось: {item.expected_pace}%</div>
                          )}
                          <a className="hci-item__action"
                            href={buildTargetURL({ team_id: item.team_id, period_id: data.period_id, goal_id: item.goal_id })}>
                            {HCI_ACTION_LABEL[k]}
                          </a>
                        </div>
                      ))}
                    </div>
                  ))}
                </div>
              );
            })
          )}
        </div>
      </div>
    </>
  );
}

// ── APP ───────────────────────────────────────────────────────────────────────
function App() {
  const [me, setMe] = useState(null);
  const [loading, setLoading] = useState(true);
  const [periods, setPeriods] = useState([]);
  const [periodId, setPeriodId] = useState(null);
  const [hierarchy, setHierarchy] = useState([]);
  const [selId, setSelId] = useState(null);
  const [teamOKR, setTeamOKR] = useState(null);
  const [overview, setOverview] = useState(null);
  const [expanded, setExpanded] = useState(readTreeExpanded);
  const [favorites, setFavorites] = useState(null); // null = not loaded from storage yet
  const [goalModal, setGoalModal] = useState(null);
  const [accent] = useState(ACCENT);
  const [hciData, setHciData] = useState(null);
  const [hciOpen, setHciOpen] = useState(false);
  const [docUrl, setDocUrl] = useState('');
  const [emptyHierMsg, setEmptyHierMsg] = useState('');
  const [staleDays, setStaleDays] = useState(7);
  const [behindMargin, setBehindMargin] = useState(10);
  const [greenThreshold, setGreenThreshold] = useState(80);

  const loadHCI = useCallback((pid) => {
    if (!pid) return;
    apiGet(`/api/v1/health-checkin?period_id=${pid}`)
      .then(d => d && setHciData(d));
  }, []);

  // При открытии панели помечаем решённые комментарии просмотренными (watermark в localStorage),
  // чтобы после первого просмотра их непросмотренный счётчик в бейдже обнулился.
  useEffect(() => {
    if (hciOpen && hciData) {
      hciMarkResolvedSeen(hciData, me?.id);
    }
  }, [hciOpen]);

  // Read desired initial team+period once from URL (highest prio) then cookie.
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

  // Deep link (?goal/kr/comment) captured once; consumed after goals render (scroll + flash).
  const deepLinkRef = useRef(null);
  if (deepLinkRef.current === null) {
    const url = readURLNav();
    deepLinkRef.current = { goal: url.goal, kr: url.kr, comment: url.comment };
  }
  useEffect(() => {
    const dl = deepLinkRef.current;
    if (!dl || (!dl.goal && !dl.kr && !dl.comment)) return;
    const id = dl.comment ? `comment-${dl.comment}` : dl.kr ? `kr-${dl.kr}` : dl.goal ? `goal-${dl.goal}` : null;
    if (!id) return;
    const timer = setTimeout(() => {
      const el = document.getElementById(id);
      if (el) {
        el.scrollIntoView({ behavior: 'smooth', block: 'center' });
        el.classList.add('deep-link-flash');
        setTimeout(() => el.classList.remove('deep-link-flash'), 1800);
      }
      deepLinkRef.current = null;
    }, 500);
    return () => clearTimeout(timer);
  }, [teamOKR]);

  useEffect(() => {
    Promise.all([apiGet('/api/v1/me'), apiGet('/api/v1/periods'), apiGet('/api/v1/config')]).then(([meData, perData, cfg]) => {
      if (meData) setMe(meData);
      if (cfg) { setDocUrl(cfg.documentation_url || ''); if (cfg.stale_days > 0) setStaleDays(cfg.stale_days); if (typeof cfg.behind_margin === 'number') setBehindMargin(cfg.behind_margin); if (cfg.green_threshold >= 1 && cfg.green_threshold <= 100) setGreenThreshold(cfg.green_threshold); setEmptyHierMsg(cfg.empty_hierarchy_message || ''); }
      const items = perData?.items || [];
      setPeriods(items);
      if (items.length > 0) {
        const desired = initialNavRef.current.period;
        const found = desired ? items.find(p => p.id === desired) : null;
        setPeriodId(found ? found.id : items[0].id);
      }
      setLoading(false);
    }).catch(() => setLoading(false));
  }, []);

  useEffect(() => {
    if (!periodId) return;
    apiGet(`/api/v1/hierarchy?period_id=${periodId}`).then(data => {
      if (!data) return;
      const nodes = data.items || [];
      _cacheUserRefsFromHierarchyNodes(nodes);
      setHierarchy(nodes);
      // Keep the current selection when it still exists in the new period's tree;
      // otherwise fall back to the URL/cookie team or the first node.
      if (!selId || !findNodeById(nodes, selId)) {
        let target = null;
        if (!initialNavRef.current.used && initialNavRef.current.team) {
          target = findNodeById(nodes, initialNavRef.current.team) || null;
        }
        initialNavRef.current.used = true;
        if (!target) target = findFirstNode(nodes);
        if (target) setSelId(target.id);
      }
    });
  }, [periodId]);

  useEffect(() => {
    if (!periodId || !selId) return;
    setTeamOKR(null); setOverview(null);
    apiGet(`/api/v1/teams/${selId}/okrs?period_id=${periodId}`).then(data => { if (data) { _cacheUserRefsFromOKR(data); setTeamOKR(data); } }).catch(() => setTeamOKR(null));
    const node = findNodeById(hierarchy, selId);
    if (node && (node.children || []).length > 0) {
      apiGet(`/api/v1/teams/${selId}/overview?period_id=${periodId}`).then(data => { if (data) setOverview(data); }).catch(() => { });
    }
  }, [periodId, selId]);

  // Keep URL and cookie in sync with current navigation state.
  // The first resolved team+period replaces the current entry; every later
  // navigation pushes a new one so the browser Back button steps through
  // visited teams. Changes that originate from Back/Forward (popstate) must
  // not push again — they are flagged via fromPopRef.
  const urlInitedRef = useRef(false);
  const fromPopRef = useRef(false);
  useEffect(() => {
    if (!periodId || !selId) return;
    if (fromPopRef.current) {
      fromPopRef.current = false; // URL already reflects the target; don't push again
    } else {
      updateURL(selId, periodId, !urlInitedRef.current);
      urlInitedRef.current = true;
    }
    writeLastNav(selId, periodId);
  }, [selId, periodId]);

  // Browser Back/Forward: re-sync navigation state from the URL.
  useEffect(() => {
    const onPop = () => {
      const { team, period } = readURLNav();
      fromPopRef.current = true;
      if (period) setPeriodId(period);
      if (team) setSelId(team);
    };
    window.addEventListener('popstate', onPop);
    return () => window.removeEventListener('popstate', onPop);
  }, []);

  useEffect(() => { loadHCI(periodId); }, [periodId]);

  // Заголовок вкладки: «Цели {команда}» для выбранного узла.
  useEffect(() => {
    const node = findNodeById(hierarchy, selId);
    const name = teamOKR?.team?.name || node?.name;
    document.title = name ? `Цели ${name}` : 'Цели команд';
  }, [teamOKR, hierarchy, selId]);

  const findFirstNode = nodes => {
    for (const n of nodes) { if (!n.children || n.children.length === 0) return n; const c = findFirstNode(n.children || []); if (c) return c; }
    return nodes[0] || null;
  };
  function findNodeById(nodes, id) {
    for (const n of nodes) { if (n.id === id) return n; const f = findNodeById(n.children || [], id); if (f) return f; }
    return null;
  }

  // Nodes are expanded by default (absence ≡ expanded), so the effective state is
  // `m[id] !== false`. Toggling stores the negation: collapse → false, expand → true.
  const toggle = useCallback(id => setExpanded(m => ({ ...m, [id]: m[id] === false })), []);
  useEffect(() => { writeTreeExpanded(expanded); }, [expanded]);

  // Load favorites once the user id is known (favorites === null → not loaded),
  // then persist on every change. Guarding the write on the `null` sentinel (not a
  // ref) avoids a same-commit race that could clobber stored favorites with [].
  useEffect(() => { if (me && favorites === null) setFavorites(readFavorites(me.id)); }, [me, favorites]);
  useEffect(() => { if (me && favorites !== null) writeFavorites(me.id, favorites); }, [favorites, me]);
  const onToggleFav = useCallback(id => setFavorites(f => toggleFavorite(f || [], id)), []);
  const selectTeam = useCallback(id => setSelId(id), []);
  const handlePeriodChange = id => { setPeriodId(Number(id)); };

  const reload = useCallback(() => {
    if (!periodId || !selId) return;
    apiGet(`/api/v1/teams/${selId}/okrs?period_id=${periodId}`).then(data => { if (data) { _cacheUserRefsFromOKR(data); setTeamOKR(data); } }).catch(() => setTeamOKR(null));
    apiGet(`/api/v1/hierarchy?period_id=${periodId}`).then(data => {
      if (!data) return;
      _cacheUserRefsFromHierarchyNodes(data.items || []);
      setHierarchy(data.items || []);
      const node = findNodeById(data.items || [], selId);
      if (node && (node.children || []).length > 0) {
        apiGet(`/api/v1/teams/${selId}/overview?period_id=${periodId}`).then(d => { if (d) setOverview(d); }).catch(() => { });
      } else {
        setOverview(null);
      }
    });
  }, [periodId, selId]);

  const handleChangeStatus = async newStatus => {
    try { await apiPost(`/api/v1/teams/${selId}/status`, { period_id: periodId, status: newStatus }); reload(); }
    catch (e) { alert('Ошибка: ' + e.message); }
  };

  const [dragState, setDragState] = useState({ srcId: null });
  const goals = (teamOKR?.goals || []).map(mapGoal);

  const handleReorderGoals = useCallback(async (fromId, toId) => {
    if (!fromId || !toId || fromId === toId) return;
    const cur = (teamOKR?.goals || []).map(mapGoal);
    const fi = cur.findIndex(g => g.id === fromId), ti = cur.findIndex(g => g.id === toId);
    if (fi < 0 || ti < 0) return;
    const dir = fi > ti ? 'move-up' : 'move-down';
    const teamId = teamOKR?.team?.id;
    if (!teamId) return;
    for (let i = 0; i < Math.abs(fi - ti); i++) await apiPost(`/api/v1/goals/${fromId}/${dir}`, { team_id: teamId });
    reload();
  }, [teamOKR, reload]);

  const handleReorderKRs = useCallback(async (goalId, fromId, toId) => {
    const g = (teamOKR?.goals || []).map(mapGoal).find(x => x.id === goalId); if (!g) return;
    const krs = g.krs || [];
    const fi = krs.findIndex(k => k.id === fromId), ti = krs.findIndex(k => k.id === toId);
    if (fi < 0 || ti < 0 || fi === ti) return;
    const dir = fi > ti ? 'move-up' : 'move-down';
    for (let i = 0; i < Math.abs(fi - ti); i++) await apiPost(`/api/v1/krs/${fromId}/${dir}`, {});
    reload();
  }, [teamOKR, reload]);

  if (loading) return <div className="loading-screen">Загрузка…</div>;

  const curPeriod = periods.find(p => p.id === periodId);
  const status = teamOKR?.period_status || 'no_goals';
  const hasGoals = (teamOKR?.goals_count || 0) > 0;
  const editMode = status === 'forming' || status === 'ready' || status === 'no_goals' ? 'full' : status === 'in_progress' ? 'progress_only' : 'comments_only';

  // A team's goal weights are expected to sum to 100%. When they don't, surface a
  // warning so the author can redistribute weight or add a goal.
  const goalWeightSum = goals.reduce((s, g) => s + (g.weight || 0), 0);
  const goalWeightOff = goals.length > 0 && goalWeightSum !== 100;
  const goalWeightDelta = 100 - goalWeightSum;
  const hasChildren = overview && (overview.children_summary?.items?.length > 0);
  const goalWeightWarn = goalWeightOff ? (
    <div className="goal-weight-warn">
      <span className="goal-weight-warn__icon">⚠</span>
      <div className="goal-weight-warn__body">
        <div className="goal-weight-warn__title">Сумма весов целей = {goalWeightSum}%, ожидается 100%</div>
        <div className="goal-weight-warn__hint">
          {goalWeightDelta > 0
            ? `Не распределено ${goalWeightDelta}% — расширьте важность одной из целей или добавьте новую.`
            : `Превышено на ${-goalWeightDelta}% — уменьшите важность одной из целей.`}
        </div>
      </div>
    </div>
  ) : null;

  const favArr = favorites || [];
  const favSet = new Set(favArr);
  const favNodes = collectFavNodes(hierarchy, favArr);
  const visibleTree = filterTreeForSidebar(hierarchy, readSidebarSelection(me?.id), selId);

  return (
    <div className="app">
      <Sidebar
        user={me}
        active="tracker"
        linkParams={{ 'activity-log': periodId ? `?period=${periodId}` : '' }}
        bell={hciData && hciData.has_scope
          ? <SidebarBell count={hciData.total_problems + hciUnseenResolved(hciData, me?.id)} onClick={() => setHciOpen(true)} />
          : null}
        beforeSections={
          <div className="sidebar__period">
            <div className="sidebar__period-label">Период</div>
            <PeriodSelect periods={periods} periodId={periodId} onChange={id => handlePeriodChange(id)} />
          </div>
        }
      >
        <div className="sidebar__tree">
          {!loading && hierarchy.length === 0
            ? (
              <div className="no-access">
                <div className="no-access__icon">🔒</div>
                {emptyHierMsg
                  ? <div className="no-access__text"><Markdown text={emptyHierMsg} /></div>
                  : <>
                      <div className="no-access__text">Нет доступа к командам</div>
                      <div className="no-access__hint">За доступом обратитесь к администратору</div>
                    </>}
              </div>
            )
            : <>
                <div className="sidebar__subsection-label">Команды</div>
                {favNodes.length > 0 && <>
                  <div className="sidebar__subsection-label"><span className="sidebar__subsection-star">★</span> Избранное · {favNodes.length}</div>
                  {favNodes.map(n => <SidebarNode key={`fav-${n.id}`} node={{ ...n, children: [] }} depth={0} selectedId={selId} onSelect={selectTeam} expanded={expanded} toggle={toggle} accent={accent} behindMargin={behindMargin} greenThreshold={greenThreshold} favSet={favSet} onToggleFav={onToggleFav} />)}
                  <div className="sidebar__subsection-label">Все команды</div>
                </>}
                {visibleTree.map(n => <SidebarNode key={n.id} node={n} depth={0} selectedId={selId} onSelect={selectTeam} expanded={expanded} toggle={toggle} accent={accent} behindMargin={behindMargin} greenThreshold={greenThreshold} favSet={favSet} onToggleFav={onToggleFav} />)}
              </>
          }
        </div>
      </Sidebar>

      <div className="main">
        <div className="topbar">
          <div className="topbar__main">
            <span className="topbar__title">{teamOKR?.team?.name || 'Выберите команду'}</span>
            {teamOKR?.team?.type && <Badge label={TEAM_TYPE_LABEL[teamOKR.team.type] || teamOKR.team.type} color={TEAM_TYPE_COLOR[teamOKR.team.type] || '#6b7280'} />}
            {teamOKR?.team?.lead && (
              <div className="topbar__lead">
                <UserInfo userRef={teamOKR.team.lead} size={22} />
                <span className="topbar__lead-role">лид</span>
              </div>
            )}
            <div className="topbar__spacer" />
            {hasGoals && teamOKR?.progress_meta && (
              <div className="topbar__progress">
                <div style={{ width: 140 }}>
                  <ProgressBar value={teamOKR.period_progress || 0} forecast={teamOKR.progress_meta.forecast} h={6}
                    color={HEALTH_COLOR[(teamOKR.period_progress || 0) >= greenThreshold ? 'ahead' : teamOKR.progress_meta.status === 'above' ? 'ahead' : teamOKR.progress_meta.status === 'below' ? 'below' : 'on_track']} />
                </div>
                <span className="topbar__progress-pct">{teamOKR.period_progress || 0}%</span>
              </div>
            )}
            {editMode === 'full' && selId && <button onClick={() => setGoalModal('new')} className="topbar__add-btn">+ Добавить цель</button>}
          </div>
          {(() => {
            const ov = readDescOverrides(me?.id);
            const desc = teamOKR?.team && ov[teamOKR.team.id] !== undefined ? ov[teamOKR.team.id] : teamOKR?.team?.description;
            return desc ? <Markdown text={desc} className="topbar__desc" /> : null;
          })()}
        </div>

        <StatusStepper status={status} hasGoals={hasGoals} onChange={handleChangeStatus} accent={accent} statusChangedAt={teamOKR?.status_changed_at} />

        <div className="content">
          {!hasChildren && goalWeightWarn}
          {hasChildren && <ClusterView overview={overview} onSelect={selectTeam} greenThreshold={greenThreshold} />}
          {goals.length === 0 && !overview && hierarchy.length === 0 && !loading && (
            <div className="empty-state">
              <div className="empty-state__icon">🔒</div>
              <div className="empty-state__title">Нет доступа</div>
              <div className="empty-state__text">За доступом обратитесь к администратору</div>
            </div>
          )}
          {goals.length === 0 && !overview && hierarchy.length > 0 && (
            <div className="empty-state">
              <div className="empty-state__icon">📋</div>
              <div className="empty-state__title">Цели не добавлены</div>
              <div className="empty-state__text">Начните период с постановки OKR</div>
              {editMode === 'full' && selId && <button onClick={() => setGoalModal('new')} className="empty-state__btn">+ Создать первую цель</button>}
            </div>
          )}
          {hasChildren && goals.length > 0 && <div className="section-label">Цели этого узла</div>}
          {hasChildren && goalWeightWarn}
          {goals.map(g => <GoalCard key={g.id} goal={g} editMode={editMode} onReload={reload} onEditGoal={setGoalModal} me={me} accent={accent} currentTeamId={selId} allTeams={hierarchy} staleDays={staleDays} periodStatus={status} greenThreshold={greenThreshold} deepLink={deepLinkRef.current}
            dragProps={editMode === 'full' ? {
              isDragging: dragState.srcId === g.id,
              onDragStart: (e) => { e.dataTransfer.effectAllowed = 'move'; setDragState({ srcId: g.id }); },
              onDragOver: (e) => { if (dragState.srcId && dragState.srcId !== g.id) { e.preventDefault(); e.dataTransfer.dropEffect = 'move'; } },
              onDrop: (e) => { e.preventDefault(); handleReorderGoals(dragState.srcId, g.id); setDragState({ srcId: null }); },
              onDragEnd: () => setDragState({ srcId: null }),
            } : null}
            onReorderKR={editMode === 'full' ? (fromId, toId) => handleReorderKRs(g.id, fromId, toId) : null}
          />)}
        </div>
      </div>

      {goalModal && <GoalModal
        goal={goalModal === 'new' ? null : goalModal}
        teamId={selId} periodId={periodId}
        teamName={teamOKR?.team?.name || ''} periodName={curPeriod?.name || ''}
        existingGoals={goals} me={me}
        onSave={() => { setGoalModal(null); reload(); }}
        onClose={() => setGoalModal(null)}
        accent={accent} allTeams={hierarchy} />}
      <HealthCheckInPanel
        data={hciData}
        open={hciOpen}
        onClose={() => setHciOpen(false)}
      />
    </div>
  );
}

ReactDOM.createRoot(document.getElementById('root')).render(<App />);
