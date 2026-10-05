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
// team+period пишутся общим writeNavURL (period_url.js) в режиме reset: остальные
// параметры (deep-link ?goal/?kr/?comment) валидны только при открытии страницы и
// после первой навигации выбрасываются.
function updateURL(teamId, periodId, replace = false) {
  writeNavURL({ team: teamId || null, period: periodId || null }, { replace, reset: true });
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

// ── BOARD VIEW PERSISTENCE ────────────────────────────────────────────────────
// Настройки показа целей на доске: порядок, фильтр по приоритету, показ заметок к KR,
// скрытие целей с нулевым весом. Один объект под одним ключом, не привязанным ни к
// команде, ни к периоду — поэтому смена команды и смена периода их не сбрасывают, а
// перезагрузка восстанавливает. Настройки влияют только на показ: прогресс, сумма
// весов, охват выгрузки и занятый вес в окне цели считаются по всем целям периода.
const BOARD_SORTS = ['custom', 'priority'];
const BOARD_VIEW_DEFAULT = { sort: 'custom', pri: {}, notes: false, hideZeroWeight: false };

// Прочитанное значение не принимается на веру: неизвестный порядок, лишние уровни
// приоритета и нечитаемый JSON дают дефолты, а не ошибку рендера — как в
// readTreeExpanded и readFavorites.
function normalizeBoardView(raw) {
  const v = raw && typeof raw === 'object' ? raw : {};
  const pri = {};
  if (v.pri && typeof v.pri === 'object') PRI_LEVELS.forEach(p => { if (v.pri[p]) pri[p] = true; });
  return {
    sort: BOARD_SORTS.includes(v.sort) ? v.sort : BOARD_VIEW_DEFAULT.sort,
    pri,
    notes: !!v.notes,
    hideZeroWeight: !!v.hideZeroWeight,
  };
}
const readBoardView = () => normalizeBoardView(readJSON(STORAGE_KEYS.boardView, null));
const writeBoardView = v => writeJSON(STORAGE_KEYS.boardView, v);
const boardViewDefault = () => ({ ...BOARD_VIEW_DEFAULT, pri: {} });
const boardViewIsDefault = v =>
  v.sort === BOARD_VIEW_DEFAULT.sort && !PRI_LEVELS.some(p => v.pri[p]) && !v.notes && !v.hideZeroWeight;
// Нажатие на уже выбранный порядок приходит как onChange с теми же настройками. Такое
// обращение не должно считаться изменением: оно снимало бы закрепление цели по прямой
// ссылке, и цель исчезала бы с доски, хотя пользователь ничего не менял.
const boardViewEquals = (a, b) =>
  a.sort === b.sort && !!a.notes === !!b.notes && !!a.hideZeroWeight === !!b.hideZeroWeight
  && PRI_LEVELS.every(p => !!a.pri[p] === !!b.pri[p]);

// Фильтры сужают показ, сортировка упорядочивает то, что осталось. Порядок шагов
// фиксирован: приоритет → нулевой вес → сортировка. Сортировка стабильная, поэтому
// цели одного приоритета сохраняют пользовательский порядок. Неизвестный приоритет
// уходит в конец, а не в начало (indexOf дал бы -1).
// keepId — цель, открытая по прямой ссылке: фильтры её не скрывают, иначе общая ссылка
// молча не срабатывала бы у получателя, у которого с прошлого раза остался фильтр.
const priRank = p => { const i = PRI_LEVELS.indexOf(p); return i < 0 ? PRI_LEVELS.length : i; };
function visibleGoals(allGoals, view, keepId = null) {
  const pinned = g => keepId != null && g.id === keepId;
  const activePri = PRI_LEVELS.filter(p => view.pri[p]);
  let out = activePri.length ? allGoals.filter(g => pinned(g) || activePri.includes(g.priority)) : allGoals.slice();
  if (view.hideZeroWeight) out = out.filter(g => pinned(g) || (g.weight || 0) !== 0);
  if (view.sort === 'priority') out.sort((a, b) => priRank(a.priority) - priRank(b.priority));
  return out;
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
    healthStatus: kr.health_status || 'not_started',
    start, target, current, done, stages, unit, checkpoints, zeroing,
    // daysAgo у заметки — её собственный, не KR: правка названия или веса KR трогает
    // kr.updated_at, но не заметку, и старая заметка не должна выглядеть свежей.
    note: kr.note ? { text: kr.note.text, author: kr.note.author_name, authorUdid: kr.note.author_udid, date: fmtDate(kr.note.updated_at), daysAgo: daysAgo(kr.note.updated_at) } : null,
    updatedAt: kr.updated_at, updatedDaysAgo: daysAgo(kr.updated_at),
    // Давность ПРОГРЕССА — отдельное поле: kr.updated_at двигает любая правка KR
    // (название, вес, описание, состояние), а progress_updated_at — только чек-ин.
    // null означает, что прогресс ещё не обновляли ни разу.
    progressDaysAgo: kr.progress_updated_at ? daysAgo(kr.progress_updated_at) : null,
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
    comments: (g.comments || []).map(c => ({ id: c.id, author: c.author_name, authorUdid: c.author_udid, date: fmtDate(c.created_at), text: c.text, resolved: !!c.resolved, resolvedBy: c.resolved_by_name, resolvedByUdid: c.resolved_by_udid, resolvedAt: c.resolved_at ? fmtDate(c.resolved_at) : null, replies: (c.replies || []).map(rp => ({ id: rp.id, author: rp.author_name, authorUdid: rp.author_udid, date: fmtDate(rp.created_at), text: rp.text })) })),
    shareTeams: g.share_teams || [],
    shared: (g.share_teams || []).length > 0,
    parents: (g.parents || []).map(mapGoalRef),
    children: (g.children || []).map(mapGoalRef),
    updatedAt: g.updated_at,
    updatedDaysAgo: daysAgo(g.updated_at),
  };
}
// Компактная сводка связанной цели (родитель/ребёнок) для лейблов и popover.
function mapGoalRef(r) {
  return {
    id: r.id, title: r.title,
    periodId: r.period_id, periodName: r.period_name,
    teamId: r.team_id, teamName: r.team_name, teamType: r.team_type,
    progress: r.progress || 0,
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

// ── DESIGN CONSTANTS ──────────────────────────────────────────────────────────
const HEALTH_COLOR = { ahead: '#16a34a', on_track: '#2563eb', below: '#ef4444', stale: '#d97706', no_goals: '#d1d5db' };
const HEALTH_LABEL = { ahead: 'опережает', on_track: 'в плане', below: 'отстаёт', stale: 'нет обновлений', no_goals: 'нет целей' };
const FOCUS_COLORS = { EFFICIENCY: '#0891b2', QUALITY: '#7c3aed', RELIABILITY: '#059669', GROWTH: '#d97706', PROFITABILITY: '#dc2626', STABILITY: '#6366f1', SPEED_EFFICIENCY: '#0891b2', TECH_INDEPENDENCE: '#be185d', DEFAULT: '#6b7280' };
const KR_TYPE_C = { NUMERICAL: '#2563eb', BOOLEAN: '#7c3aed', PROJECT: '#d97706' };
const KR_UNITS = ['%', 'RPS', 'мс', 'сек', 'мин', 'час', 'дней', 'шт', '₽', 'запросов', 'ошибок', 'пользователей', 'заказов', 'рублей'];
const KR_TYPE_LABEL = { BOOLEAN: 'Бинарный', PROJECT: 'Проектный', NUMERICAL: 'Числовой' };
const KR_TYPE_OPTIONS = ['BOOLEAN', 'PROJECT', 'NUMERICAL'];
// Выбор способа измерения в окне KR: значок, название, что считает и пример.
const KR_MEASURE_INFO = {
  NUMERICAL: { icon: '123', title: 'Метрика', desc: 'Старт → цель, вводите текущее значение', ex: 'p95 latency 800 → 300 мс' },
  BOOLEAN: { icon: '✓', title: 'True / False', desc: 'Достигнут или нет: 0% или 100%', ex: 'сервис переведён на k8s' },
  PROJECT: { icon: '☰', title: 'Шаги проекта', desc: 'Сумма весов выполненных шагов', ex: 'RFC → миграция БД → отключение legacy' },
};
// Manual KR health status (not the forecast-based HEALTH_COLOR above).
const KR_HEALTH_COLOR = { not_started: '#6b7280', on_track: '#16a34a', at_risk: '#d97706', done: '#15803d' };
const KR_HEALTH_LABEL = { not_started: 'Not Started', on_track: 'On Track', at_risk: 'At Risk', done: 'Closed' };
const KR_HEALTH_ICON = { not_started: '○', on_track: '●', at_risk: '▲', done: '✓' };
const KR_HEALTH_OPTIONS = ['not_started', 'on_track', 'at_risk', 'done'];
const KR_HEALTH_HINT = {
  not_started: 'Команда не приступила к этому KR',
  on_track: 'Началась работа, идёт планово',
  at_risk: 'Фиксируем существенный риск для достижения результата',
  // Статус говорит только о прекращении работ, не о достижении результата:
  // достигнут он или нет, говорит прогресс — см. KR_CLOSED_VIEW ниже.
  done: 'Работы по KR прекращены — прогресс больше не предполагается менять',
};

// Вид закрытого KR. Статус «закрыт» сообщает, что работ больше не будет, а чем они
// закончились — прогресс. Ключи совпадают с модификаторами .kr-hdot--* в tracker.css.
const KR_CLOSED_VIEW = {
  done: { icon: '✓', flag: '', note: '', hint: () => 'Работы прекращены, результат достигнут' },
  done_near: { icon: '✓', flag: '!', note: 'результат не достигнут',
    hint: p => `Закрыт на ${p}% — работы прекращены, результат не достигнут` },
  done_short: { icon: '✕', flag: '', note: 'результат не достигнут',
    hint: p => `Закрыт на ${p}% — работы прекращены, результат не достигнут` },
};

// Граница между done_near и done_short — порог «в плане» из настроек пространства,
// тот же, по которому «в плане» считаются цель и команда (healthOf). Включительна,
// как и там: прогресс, равный порогу, даёт done_near.
function closedViewOf(progress, greenThreshold = 80) {
  if (progress >= 100) return 'done';
  return progress >= greenThreshold ? 'done_near' : 'done_short';
}

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
// The lag tolerance is the tenant's behind_margin progress threshold: red when
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

// Приоритет цели: короткое имя и цвет — для селектора в окне цели и бейджа на карточке.
const PRI_LEVELS = ['P0', 'P1', 'P2', 'P3'];
const PRI_SHORT = { P0: 'Критичный', P1: 'Высокий', P2: 'Средний', P3: 'Низкий' };
const PRI_COLOR = { P0: '#dc2626', P1: '#d97706', P2: '#2563eb', P3: '#6b7280' };
// Подсказка приоритета цели.
const PRI_HINT = {
  P0: 'Критично: без этой цели период провален. Ресурсы — в первую очередь.',
  P1: 'Высокий: ключевая цель периода, делаем обязательно.',
  P2: 'Средний: важно, но можно сдвинуть при нехватке ресурсов.',
  P3: 'Низкий: делаем, если останется время.',
};
// Полоса KR краснеет, если отставание от ожидаемого темпа больше этого значения.
const KR_BEHIND_PP = 20;

// Здоровье KR — цветная точка с поповером. Поповер и подсказки has-tip рисует
// только CSS (::after + attr, :hover/:focus), поэтому обработчиков здесь нет.
function KRHealthDot({ status, progress = 0, greenThreshold = 80 }) {
  const s = KR_HEALTH_LABEL[status] ? status : 'not_started';
  // Вид выводится здесь, а не в строке KR: правило «как выглядит состояние»
  // принадлежит тому, кто его рисует, иначе каждое место с точкой повторит его заново.
  // Прогресс по умолчанию 0, а не 100: если его не передали, точка не должна
  // утверждать достижение результата — именно это и исправляет разделение видов.
  const view = s === 'done' ? closedViewOf(progress, greenThreshold) : s;
  const closed = KR_CLOSED_VIEW[view];
  const icon = closed ? closed.icon : KR_HEALTH_ICON[s];
  const hint = closed ? closed.hint(progress) : (KR_HEALTH_HINT[s] || '');
  const label = KR_HEALTH_LABEL[s];
  return (
    <span className={`kr-hdot kr-hdot--${view}`} tabIndex={0}
      aria-label={`Статус KR: ${label}${closed && closed.note ? `, ${closed.note}` : ''}`} data-no-drag>
      <span className="kr-hdot__mark">{icon}</span>
      {closed && closed.flag && <span className="kr-hdot__flag" aria-hidden="true">{closed.flag}</span>}
      <span className="kr-hdot__pop" role="tooltip">
        <span className="kr-hdot__title">{icon} {label}</span>
        <span className="kr-hdot__hint">{hint}</span>
      </span>
    </span>
  );
}

// Подсказка фокуса: по строке на тему, название окрашено как бейдж на карточке.
// Названия берутся из focusLabel, чтобы подсказка и выпадающий список не разошлись.
const FOCUS_HINT = {
  PROFITABILITY: 'выручка и маржинальность',
  STABILITY: 'надёжность и меньше инцидентов',
  SPEED_EFFICIENCY: 'скорость поставки и эффективность процессов',
  TECH_INDEPENDENCE: 'уход от внешних и legacy-зависимостей',
};
const FOCUS_HINT_BLOCK = (
  <>
    На какую стратегическую тему работает цель.
    {FOCUS_OPTIONS.map(f => (
      <span key={f} className="hint-row">
        <span className="hint-code" style={{ color: FOCUS_COLORS[f] || FOCUS_COLORS.DEFAULT }}>{focusLabel(f)}</span>
        {' — '}{FOCUS_HINT[f]}
      </span>
    ))}
  </>
);

// Подсказка приоритета: вступление и по строке на уровень, код уровня окрашен так же,
// как бейдж на карточке, — подсказка и доска читаются одним кодом.
const PRIORITY_HINT = (
  <>
    Насколько цель важна относительно других.
    {PRI_LEVELS.map(p => (
      <span key={p} className="hint-row">
        <span className="hint-code" style={{ color: PRI_COLOR[p] }}>{p}</span>
        {' — '}{PRI_HINT[p]}
      </span>
    ))}
  </>
);

// Выбор приоритета в окне цели: бейдж, короткое имя и пояснение каждого уровня —
// вместо четырёх кнопок P0..P3, по которым не видно, чем они отличаются.
function PrioritySelect({ value, onChange }) {
  const [open, setOpen] = useState(false);
  const ref = React.useRef(null);
  React.useEffect(() => {
    if (!open) return;
    const onDoc = e => { if (ref.current && !ref.current.contains(e.target)) setOpen(false); };
    document.addEventListener('mousedown', onDoc);
    return () => document.removeEventListener('mousedown', onDoc);
  }, [open]);
  return (
    <div className="pri-select" ref={ref}>
      <button type="button" className="pri-select__btn" aria-haspopup="listbox" aria-expanded={open}
        onClick={() => setOpen(!open)}>
        <PriBadge p={value} />
        <span className="pri-select__txt">{PRI_SHORT[value] || ''}</span>
        <span className="pri-select__caret">▾</span>
      </button>
      {open && (
        <div className="pri-select__menu" role="listbox">
          {PRI_LEVELS.map(p => (
            <button key={p} type="button" role="option" aria-selected={value === p}
              className={`pri-select__opt${value === p ? ' pri-select__opt--on' : ''}`}
              onClick={() => { onChange(p); setOpen(false); }}>
              <PriBadge p={p} />
              <span className="pri-select__opt-txt">
                <b>{PRI_SHORT[p]}</b>
                <span>{PRI_HINT[p] || ''}</span>
              </span>
              {value === p && <span className="pri-select__check">✓</span>}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

// Меню действий строки — одинаковое у цели и у KR. Пункт, недоступный по статусу,
// остаётся в списке и объясняет причину, а не исчезает.
function MenuItem({ icon, label, onClick, danger = false, reason = null, onDone, confirmLabel = null, failLabel = 'Не удалось' }) {
  const [state, setState] = useState(null); // null | 'done' | 'fail'
  const done = state === 'done', failed = state === 'fail';
  const cls = ['act-menu__item', danger ? 'act-menu__item--danger' : '',
    done ? 'act-menu__item--done' : '', failed ? 'act-menu__item--fail' : '',
    reason ? 'act-menu__item--disabled act-menu__item--why' : ''].filter(Boolean).join(' ');
  // Пункт с confirmLabel отчитывается о результате на себе же и только потом закрывает
  // меню: иначе и подтверждение, и отказ исчезают вместе с меню, и действие выглядит
  // беззвучным — а копирование в буфер отказать может (страница без фокуса, нет прав).
  const run = async () => {
    if (!confirmLabel) { onDone(); onClick(); return; }
    const ok = await onClick();
    setState(ok === false ? 'fail' : 'done');
    setTimeout(onDone, ok === false ? 1600 : 900);
  };
  return (
    <button type="button" className={cls}
      aria-disabled={reason ? true : undefined}
      aria-label={reason ? `${label}. ${reason}` : undefined}
      onClick={reason ? undefined : run}>
      <span className="act-menu__ic">{done ? '✓' : failed ? '⚠' : icon}</span>
      {done ? confirmLabel : failed ? failLabel : label}
      {state && <span className="visually-hidden" role="status">{done ? confirmLabel : failLabel}</span>}
      {reason && <span className="act-menu__lock" aria-hidden="true">?</span>}
      {reason && <span className="act-menu__why" role="tooltip">{reason}</span>}
    </button>
  );
}

// btnClass/dropdownClass — чтобы меню в шапке доски выглядело как в прототипе:
// там это кнопка 36×36 с рамкой, а не безрамочные «···» строки цели.
function RowMenu({ items, btnClass = 'export-menu__btn', dropdownClass = '' }) {
  const [open, setOpen] = useState(false);
  const ref = React.useRef(null);
  React.useEffect(() => {
    if (!open) return;
    const onDoc = e => { if (ref.current && !ref.current.contains(e.target)) setOpen(false); };
    document.addEventListener('mousedown', onDoc);
    return () => document.removeEventListener('mousedown', onDoc);
  }, [open]);
  return (
    <div className="export-menu" ref={ref} data-no-drag>
      <button type="button" className={btnClass} title="Ещё" aria-label="Ещё"
        onClick={() => setOpen(!open)}>···</button>
      {open && (
        <div className={`export-menu__dropdown${dropdownClass ? ` ${dropdownClass}` : ''}`}>
          {items.filter(Boolean).map((it, i) => it.sep
            ? <div key={i} className="act-menu__sep" />
            : <MenuItem key={i} {...it} onDone={() => setOpen(false)} />)}
        </div>
      )}
    </div>
  );
}

function KRHealthBadge({ status }) {
  const s = KR_HEALTH_LABEL[status] ? status : 'not_started';
  const color = KR_HEALTH_COLOR[s];
  const label = `${KR_HEALTH_ICON[s]} ${KR_HEALTH_LABEL[s]}`;
  // 'done' reads as a solid pill; others use the tinted Badge default.
  return s === 'done'
    ? <Badge label={label} color="#ffffff" bg={color} />
    : <Badge label={label} color={color} />;
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

// findTreeNode returns the hierarchy node with the given id (searching the whole forest).
function findTreeNode(nodes, id) {
  for (const n of nodes || []) {
    if (n.id === id) return n;
    const f = findTreeNode(n.children || [], id);
    if (f) return f;
  }
  return null;
}

// countSubtree counts a node and all its descendants (1 for a leaf).
function countSubtree(node) {
  if (!node) return 0;
  return 1 + (node.children || []).reduce((s, c) => s + countSubtree(c), 0);
}

// treePathNames returns the names root→node for the given id, or null if not found.
function treePathNames(nodes, id, trail = []) {
  for (const n of nodes || []) {
    const next = [...trail, n.name];
    if (n.id === id) return next;
    const f = treePathNames(n.children || [], id, next);
    if (f) return f;
  }
  return null;
}

function TeamCombobox({ selectedIds, onChange, excludeId, accent, allTeams, single = false, blockedIds = [], blockedReason = {} }) {
  const [q, setQ] = useState(''); const [open, setOpen] = useState(false); const [hi, setHi] = useState(0);
  const [blockedTeam, setBlockedTeam] = useState(null);
  const inputRef = useRef(); const wrapRef = useRef();
  const blocked = new Set(blockedIds || []);
  const flat = flattenTree(allTeams || []).filter(t => t.id !== excludeId);
  const ql = q.trim().toLowerCase();
  // Inline-поиск матчит по названию команды ИЛИ по имени руководителя, чтобы команду можно было
  // найти по её тимлиду.
  const filtered = ql ? flat.filter(t => t.name.toLowerCase().includes(ql) || (t.lead?.display_name || '').toLowerCase().includes(ql)) : flat;
  // In single mode the currently selected item stays visible in the list (as selected);
  // in multi mode selected items move to tags and are removed from the dropdown.
  const available = single ? filtered : filtered.filter(t => !selectedIds.includes(t.id));
  useEffect(() => { setHi(0); }, [q]);
  useEffect(() => {
    const h = e => { if (wrapRef.current && !wrapRef.current.contains(e.target)) setOpen(false); };
    document.addEventListener('mousedown', h);
    return () => document.removeEventListener('mousedown', h);
  }, []);
  // Команда заблокирована, если её период уже «в работе»/«закрыт» (её набор OKR на период
  // закрыт — сервер тоже отклоняет такой шаринг, ErrCannotShareWithClosedPeriod), либо если она
  // явно передана в blockedIds (напр. модалка переноса/копирования блокирует по статусу целевого периода).
  const isStatusBlocked = t => t.status === 'in_progress' || t.status === 'closed';
  const isBlocked = t => blocked.has(t.id) || isStatusBlocked(t);
  const add = t => {
    if (single) { onChange([t.id]); setQ(''); setOpen(false); }
    else { onChange([...selectedIds, t.id]); setQ(''); inputRef.current?.focus(); }
  };
  // В мультиселекте (шаринг) блокировка открывает модалку-объяснение; в single-режиме (перенос)
  // выбор просто игнорируется — причина уже показана в самой модалке переноса.
  const choose = t => { if (isBlocked(t)) { if (!single) setBlockedTeam(t); return; } add(t); };
  const rem = id => onChange(selectedIds.filter(x => x !== id));
  const sel = selectedIds.map(id => flat.find(t => t.id === id)).filter(Boolean);
  const onKey = e => {
    if (e.key === 'ArrowDown') { e.preventDefault(); setOpen(true); setHi(h => Math.min(available.length - 1, h + 1)); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); setHi(h => Math.max(0, h - 1)); }
    else if (e.key === 'Enter') { e.preventDefault(); if (open && available[hi]) choose(available[hi]); }
    else if (e.key === 'Escape') { if (open) { e.preventDefault(); setOpen(false); } }
    else if (e.key === 'Backspace' && !q && !single && sel.length > 0) rem(sel[sel.length - 1].id);
  };
  const singleLabel = single && sel[0] ? `${TEAM_TYPE_LABEL[sel[0].type] || sel[0].type} · ${sel[0].name}` : '';
  return (
    <div ref={wrapRef} className="team-combobox">
      <div onClick={() => { setOpen(true); inputRef.current?.focus(); }}
        className={`team-combobox__input-area${open ? ' team-combobox__input-area--open' : ''}`}>
        {!single && sel.map(t => {
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
          placeholder={single ? (singleLabel || 'Найдите команду') : (sel.length ? 'Ещё…' : 'Найдите команду')}
          className="team-combobox__input" />
      </div>
      {open && (
        <div className="team-combobox__dropdown">
          {available.length === 0
            ? <div className="team-combobox__empty">{ql ? 'Не найдено' : 'Нет команд'}</div>
            : available.map((t, i) => {
              const color = TEAM_TYPE_COLOR[t.type] || '#6b7280';
              const optBlocked = isBlocked(t);
              const isSel = single && selectedIds.includes(t.id);
              return (
                <div key={t.id} onClick={() => choose(t)} onMouseEnter={() => setHi(i)}
                  className={`team-combobox__option${i === hi ? ' team-combobox__option--hi' : ''}${optBlocked ? ' team-combobox__option--blocked' : ''}${isSel ? ' team-combobox__option--selected' : ''}`}
                  style={{ padding: `7px 12px 7px ${8 + t.depth * 14}px` }}
                  title={optBlocked && blockedReason[t.id] ? blockedReason[t.id] : ''}>
                  <div className="team-combobox__option-stripe" style={{ background: color }} />
                  <span className="team-combobox__option-type" style={{ color, background: `${color}12` }}>{TEAM_TYPE_LABEL[t.type] || t.type}</span>
                  <span className="team-combobox__option-name">{t.name}</span>
                  {t.lead?.display_name && <span className="team-combobox__option-lead" title={`Руководитель: ${t.lead.display_name}`}>{t.lead.display_name}</span>}
                  {optBlocked && (blockedReason[t.id]
                    ? <span className="team-combobox__option-blocked-tag">{blockedReason[t.id]}</span>
                    : <span className="team-combobox__option-locked">🔒</span>)}
                </div>
              );
            })}
        </div>
      )}
      {blockedTeam && <PeriodStartedTeamModal team={blockedTeam} onClose={() => setBlockedTeam(null)} />}
    </div>
  );
}

// Объясняет, почему команду с уже начатым периодом (в работе/закрыт) нельзя добавить в общую цель,
// и предлагает написать её руководителю, чтобы он вернул цели в черновик. Серверной гарантией
// выступает ErrCannotShareWithClosedPeriod в ShareGoal — эта модалка лишь превентивный UX-сигнал.
function PeriodStartedTeamModal({ team, onClose }) {
  const { requestClose } = useModalClose({ isDirty: false, onClose });
  const overlay = useOverlayClose(requestClose);
  const statusLabel = (STATUS_STEPS.find(s => s.k === team.status) || {}).l || team.status;
  const lead = team.lead?.display_name;
  return (
    <div className="modal-overlay modal-overlay--z600" {...overlay}>
      <div onClick={e => e.stopPropagation()} className="modal-box modal-box--w380">
        <div className="confirm-body">
          <div className="confirm-title">Нельзя добавить общую цель</div>
          <div className="confirm-message">
            {`Команда «${team.name}» уже начала период — цели в статусе «${statusLabel}». Добавить общую цель в уже начатый период нельзя. `}
            {lead
              ? `Обратитесь к руководителю команды ${lead}, чтобы он перевёл цели в черновик.`
              : 'Обратитесь к руководителю команды, чтобы он перевёл цели в черновик.'}
          </div>
        </div>
        <div className="confirm-footer">
          <button onClick={onClose} className="btn btn--primary">Понятно</button>
        </div>
      </div>
    </div>
  );
}

// ── STATUS STEPPER ────────────────────────────────────────────────────────────
function StatusStepper({ status, hasGoals, onChange, accent, statusChangedAt, editMode }) {
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
      <ModeBanner editMode={editMode} status={status} />
      <div className="status-stepper__meta">
        {statusChangedAt && <span className="status-stepper__changed">изменён {fmtDate(statusChangedAt)}</span>}
      </div>
    </div>
  );
}

// Фильтр доски по приоритету. Счётчик у кнопки показывает, сколько целей этого
// приоритета есть в периоде; приоритет без целей нажать нельзя. Фильтр не сужает
// данные — только то, что показано, поэтому сброс возвращает полный список.
// Живёт в ряду управления показом (BoardFilterBar), а не в шапке доски: классы
// поэтому общие .filter-bar*, а не tb-* — tb- означает top-bar.
function PriorityFilter({ goals, value, onChange }) {
  const any = PRI_LEVELS.some(p => value[p]);
  return (
    <div className="filter-bar__group">
      <span className="filter-bar__cap">Приоритет</span>
      <div className="filter-bar__seg" role="group" aria-label="Фильтр по приоритету">
        {PRI_LEVELS.map(p => {
          const n = goals.filter(g => g.priority === p).length;
          const on = !!value[p];
          return (
            <button key={p} type="button" aria-pressed={on} disabled={!n}
              className={`filter-bar__btn filter-bar__btn--pri${on ? ' filter-bar__btn--on' : ''}`}
              style={{ '--pc': PRI_COLOR[p] }}
              onClick={() => onChange({ ...value, [p]: !on })}>
              {p}<span className="filter-bar__n">{n}</span>
            </button>
          );
        })}
      </div>
      {any && (
        <button type="button" className="filter-bar__clear" title="Сбросить фильтр по приоритету"
          aria-label="Сбросить фильтр по приоритету" onClick={() => onChange({})}>×</button>
      )}
    </div>
  );
}

// ── BOARD FILTER BAR ──────────────────────────────────────────────────────────
// Ряд управления показом целей: порядок, фильтр по приоритету, показ заметок к KR,
// скрытие целей с нулевым весом, сброс. Стоит над списком целей, а не в шапке доски:
// шапка описывает команду, а это управление списком, который начинается ниже.
// У команды без целей управлять показом нечем — ряд не рендерится.
const BOARD_SORT_LABEL = { custom: 'Мой порядок', priority: 'По приоритету' };

function BoardFilterBar({ goals, value, onChange }) {
  if (!goals.length) return null;
  const set = patch => onChange({ ...value, ...patch });
  return (
    <div className="filter-bar">
      <div className="filter-bar__group">
        <span className="filter-bar__cap">Порядок</span>
        <div className="filter-bar__seg" role="group" aria-label="Порядок целей">
          {BOARD_SORTS.map(k => (
            <button key={k} type="button" aria-pressed={value.sort === k}
              className={`filter-bar__btn${value.sort === k ? ' filter-bar__btn--on' : ''}`}
              onClick={() => set({ sort: k })}>{BOARD_SORT_LABEL[k]}</button>
          ))}
        </div>
      </div>
      <PriorityFilter goals={goals} value={value.pri} onChange={pri => set({ pri })} />
      <label className="filter-bar__check">
        <input type="checkbox" checked={value.notes} onChange={e => set({ notes: e.target.checked })} />
        Заметки к KR
      </label>
      <label className="filter-bar__check">
        <input type="checkbox" checked={value.hideZeroWeight} onChange={e => set({ hideZeroWeight: e.target.checked })} />
        Скрыть цели с весом 0
      </label>
      <span className="filter-bar__spacer" />
      {!boardViewIsDefault(value) && (
        <button type="button" className="filter-bar__reset"
          onClick={() => onChange(boardViewDefault())}>Сбросить</button>
      )}
    </div>
  );
}

// ── MODE BANNER ───────────────────────────────────────────────────────────────
// Объясняет режим редактирования целиком: какой статус, что доступно, что закрыто.
// В полном редактировании объяснять нечего — баннер не рендерится.
// В полном редактировании баннер подсказывает следующий шаг жизненного цикла,
// в остальных режимах — объясняет, что заблокировано и почему.
const STATUS_BANNER = {
  forming: { variant: 'info', icon: '✎', status: 'Черновик',
    text: 'заполните черновик целей. Когда будете готовы показать руководителю — переведите в статус «К валидации»',
    full: 'Заполните черновик целей. Когда будете готовы показать руководителю — переведите в статус «К валидации».' },
  ready: { variant: 'info', icon: '✓', status: 'К валидации',
    text: 'получите от руководителя апрув на цели и переведите в статус «В работе»',
    full: 'Получите от руководителя апрув на цели и переведите в статус «В работе».' },
};
const MODE_BANNER = {
  progress_only: { variant: 'progress', icon: '🔒', status: 'В работе',
    text: 'меняются только прогресс и комментарии',
    full: 'Состав целей зафиксирован. Доступны обновление прогресса и комментарии.' },
  comments_only: { variant: 'closed', icon: '🔒', status: 'Период закрыт',
    text: 'доступны только комментарии', full: 'Доступны только комментарии.' },
};
function ModeBanner({ editMode, status }) {
  const b = (editMode === 'full' && STATUS_BANNER[status]) || MODE_BANNER[editMode];
  if (!b) return null;
  return (
    <div className={`mode-banner mode-banner--${b.variant}`} title={`${b.status}. ${b.full}`}>
      <span className="mode-banner__icon" aria-hidden="true">{b.icon}</span>
      <span className="mode-banner__text">
        <span className="mode-banner__status">{b.status}</span>
        <span className="mode-banner__rest"> — {b.text}</span>
      </span>
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
function KRProgressModal({ kr, onSave, onClose, accent, goalTitle = '' }) {
  const [form, setForm] = useState({ ...kr, stages: (kr.stages || []).map(s => ({ ...s })) });
  const [note, setNote] = useState(kr.note?.text ?? ''); const [saving, setSaving] = useState(false);
  const [health, setHealth] = useState(kr.healthStatus || 'not_started');
  const [healthTouched, setHealthTouched] = useState(false);
  const pickHealth = (s) => { setHealth(s); setHealthTouched(true); };
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
  const isDirty = dirtyProgress || healthTouched || note.trim() !== initialNote.trim();
  // Mirror the server's 100%→done rule, which fires ONLY on a real <100→100 transition: preview
  // Done only when an unedited-health KR is being pushed from below 100 up to 100 in this modal.
  // A KR already at 100% (kr.progress === 100) keeps its stored/manual health — matching the badge
  // and the server, which does not re-touch health on a repeated save at 100%.
  const displayHealth = (!healthTouched && kr.progress < 100 && progress === 100) ? 'done' : health;
  const save = async () => {
    setSaving(true);
    try {
      const healthField = healthTouched ? { health_status: health } : {};
      // Заметка едет вместе с прогрессом одним запросом — это и есть чек-ин: одно
      // действие пользователя, один usecase-вызов CheckIn, одно событие/уведомление.
      // Отдельного POST /note в интерфейсе больше нет: единственный путь правки
      // заметки — эта форма, а форма всегда бьёт вместе с прогрессом/статусом.
      // Сохранено прежнее поведение "не отправлять пустую и не отправлять
      // неизменившуюся" — очистка заметки через эту форму как не работала, так и
      // не работает; это не относится к переносу заметки в тело запроса прогресса.
      const trimmed = note.trim();
      const noteField = (trimmed && trimmed !== (kr.note?.text ?? '')) ? { note: trimmed } : {};
      if (form.krType === 'NUMERICAL') {
        await apiPost(`/api/v1/krs/${kr.id}/progress/numerical`, { current_value: parseFloat(form.current) || 0, ...healthField, ...noteField });
      } else if (form.krType === 'BOOLEAN') {
        await apiPost(`/api/v1/krs/${kr.id}/progress/boolean`, { done: !!form.done, ...healthField, ...noteField });
      } else if (form.krType === 'PROJECT') {
        await apiPost(`/api/v1/krs/${kr.id}/progress/project`, { stages: form.stages.map(s => ({ id: s.id, done: !!s.done })), ...healthField, ...noteField });
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
            <div className="modal-title modal-title--lg">Обновить прогресс</div>
            {goalTitle && (
              <div className="modal-subtitle">
                <span className="modal-cap">Цель</span>
                {goalTitle}
              </div>
            )}
          </div>
          <button onClick={requestClose} className="modal-close">×</button>
        </div>
        <div className="modal-body">
          <span className="modal-cap">Ключевой результат</span>
          <div className="field-label">{kr.name}</div>
          {kr.desc && <Markdown text={kr.desc} className="kr-modal-desc md-content" />}
          {form.krType === 'NUMERICAL' && (
            <div className="kr-num-section kr-num-section--compact">
              <div className="kr-num-line">
                <span className="kr-num-line__lbl">Текущее значение</span>
                <label className="kr-num-input-suffix kr-num-input-suffix--compact">
                  <NumInput value={form.current} onChange={v => set('current', v)} className="form-input form-input--sm" />
                  {form.unit && <span className="kr-num-input-suffix__unit">{form.unit}</span>}
                </label>
                <span className="kr-num-line__range">из {fmtVal(form.target, form.unit)} · старт {fmtVal(form.start, form.unit)}</span>
                <span className="kr-pct kr-num-line__pct" style={{ color: accent }}>{progress}%</span>
              </div>
              <KRScale start={form.start} target={form.target} current={form.current}
                unit={form.unit} checkpoints={kr.checkpoints} color={accent} />
              {zeroingNote}
            </div>
          )}
          {form.krType === 'BOOLEAN' && (
            <>
              <label className="kr-boolean-label">
                <input type="checkbox" checked={!!form.done} onChange={e => set('done', e.target.checked)} style={{ width: 18, height: 18, accentColor: accent }} />
                <span className="kr-boolean-text">Результат достигнут</span>
                <span className="kr-boolean-pct" style={{ color: form.done ? '#16a34a' : '#9ca3af' }}>{form.done ? '100%' : '0%'}</span>
              </label>
              {zeroingNote}
            </>
          )}
          {form.krType === 'PROJECT' && (
            <div className="kr-num-section">
              <div className="kr-num-section__title">Этапы</div>
              {form.stages.map((s, i) => (
                <label key={s.id || i} className="kr-stage-label"
                  style={{ background: s.done ? `${accent}08` : '#f9fafb', border: `1px solid ${s.done ? `${accent}30` : '#f0f1f3'}` }}>
                  <input type="checkbox" checked={!!s.done} onChange={e => setStage(i, 'done', e.target.checked)} style={{ width: 16, height: 16, accentColor: accent }} />
                  <span className="kr-stage-name" style={{ fontWeight: s.done ? 600 : 400 }}>{s.name}</span>
                  <span className="kr-stage-weight">{s.weight}%</span>
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
          <div className="kr-health-section">
            <div className="kr-health-section__label">Статус результата</div>
            <div className="kr-health-cards">
              {KR_HEALTH_OPTIONS.map(s => (
                <button key={s} type="button" onClick={() => pickHealth(s)}
                  className={`kr-health-card${displayHealth === s ? ' kr-health-card--active' : ''}`}
                  style={displayHealth === s ? { borderColor: KR_HEALTH_COLOR[s], background: `${KR_HEALTH_COLOR[s]}0f`, color: KR_HEALTH_COLOR[s] } : undefined}>
                  <span className="kr-health-card__title" style={{ color: KR_HEALTH_COLOR[s] }}>
                    {KR_HEALTH_ICON[s]} {KR_HEALTH_LABEL[s]}
                  </span>
                  <span className="kr-health-card__hint">{KR_HEALTH_HINT[s]}</span>
                </button>
              ))}
            </div>
          </div>
          <div className="kr-progress-field" style={{ marginTop: 14 }}>
            <div className="kr-progress-field__label">Комментарий <span className="kr-progress-field__hint">необязательно</span></div>
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

// Шкала текущего значения с промежуточными отметками. Положение отметки — доля
// пути от старта к цели, а её подпись говорит, какой процент прогресса она даёт:
// из-за промежуточных значений шкала нелинейная, и это видно глазом.
function KRScale({ start, target, current, unit, checkpoints, color }) {
  const s0 = Number(start) || 0;
  const t0 = Number(target ?? 100);
  const cur = Number(current) || 0;
  const posOf = v => t0 === s0 ? 0 : Math.max(0, Math.min(100, (v - s0) / (t0 - s0) * 100));
  const cps = (checkpoints || [])
    .filter(c => c.value !== '' && c.value !== null && c.value !== undefined)
    .map(c => ({ v: Number(c.value), pct: Number(c.progress_percent) }));
  const up = t0 >= s0;
  return (
    <>
      <div className="kr-scale">
        <div className="kr-scale__track">
          <div className="kr-scale__fill" style={{ width: `${posOf(cur)}%`, background: color }} />
        </div>
        {/* Сравнение включающее и по направлению: у убывающей метрики значение,
            равное отметке, уже её достигло — прогресс за неё расчёт начисляет. */}
        {cps.map((c, i) => (
          <span key={i} className={`kr-scale__cp${(up ? cur >= c.v : cur <= c.v) ? ' kr-scale__cp--hit' : ''}`}
            style={{ left: `${posOf(c.v)}%` }}
            title={`Промежуточное значение: ${fmtVal(c.v, unit)} → ${c.pct}% прогресса`} />
        ))}
        <span className="kr-scale__cur" style={{ left: `${posOf(cur)}%`, borderColor: color }} />
      </div>
      <div className="kr-scale__labels">
        <span className="kr-scale__lbl" style={{ left: 0 }}>{fmtNum(s0)}</span>
        {cps.map((c, i) => (
          <span key={i} className="kr-scale__lbl kr-scale__lbl--cp" style={{ left: `${posOf(c.v)}%` }}>
            {fmtNum(c.v)}<b>{c.pct}%</b>
          </span>
        ))}
        <span className="kr-scale__lbl" style={{ left: '100%' }}>{fmtNum(t0)}</span>
      </div>
      {cps.length > 0 && (
        <div className="kr-scale__note">Промежуточные значения задают, какой прогресс даёт значение метрики — шкала нелинейная.</div>
      )}
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
        // Редактор не трогает прогресс: у нового KR текущее значение равно стартовому,
        // дальше оно меняется только через окно обновления прогресса.
        fd.append('numerical_current', String(Number(isNew ? form.start : form.current) || 0));
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
          <div className="kre-name-row">
            <div className="kre-name-row__name">
              <div className="kr-num-field__label">Название</div>
              <input value={form.name} onChange={e => set('name', e.target.value)} placeholder="Что измеряет этот KR?" className="form-input" />
            </div>
            <div className="kre-name-row__weight">
              <div className="kr-num-field__label">Вес<span className="has-tip kre-tip" tabIndex={0}
                data-tip-title="Вес KR"
                data-tip="Доля KR в прогрессе цели. Сумма весов всех KR цели должна быть 100%. Больше вес — сильнее KR влияет на итог цели.">?</span></div>
              <label className="kr-num-input-suffix">
                <input type="number" min={0} max={100} value={form.weight} onChange={e => set('weight', e.target.value)} className="form-input form-input--center" />
                <span className="kr-num-input-suffix__unit">%</span>
              </label>
            </div>
          </div>
          <div className="form-group--sm">
            <div className="kr-num-field__label">Описание</div>
            <MarkdownEditor value={form.desc} onChange={v => set('desc', v)} rows={2}
              textareaClassName="form-textarea form-textarea--sm" textareaStyle={{ resize: 'vertical' }} />
          </div>
          {/* Способ измерения — карточками: по списку было не видно, чем типы отличаются. */}
          <div className="kr-measure-pick">
            <div className="kr-num-field__label">Как измеряем прогресс</div>
            <div className="kr-measure-pick__hint">Определяет, как считается процент выполнения KR</div>
            <div className="kr-measure-pick__opts" role="radiogroup">
              {KR_TYPE_OPTIONS.map(t => (
                <button key={t} type="button" role="radio" aria-checked={form.krType === t}
                  className={`kr-measure-opt${form.krType === t ? ' kr-measure-opt--on' : ''}`}
                  onClick={() => set('krType', t)}>
                  <span className="kr-measure-opt__head">
                    <span className="kr-measure-opt__ic">{KR_MEASURE_INFO[t].icon}</span>{KR_MEASURE_INFO[t].title}
                  </span>
                  <span className="kr-measure-opt__desc">{KR_MEASURE_INFO[t].desc}</span>
                  <span className="kr-measure-opt__ex">{KR_MEASURE_INFO[t].ex}</span>
                </button>
              ))}
            </div>
          </div>
          {form.krType === 'NUMERICAL' && (
            <div className="kr-num-section">
              <div className="kr-num-section__title">Метрика</div>
              {/* Единица, старт и цель — одной строкой. Текущее значение редактор не
                  показывает: им распоряжается окно обновления прогресса. */}
              <div className="kre-metric-row">
                <div className="kre-metric-row__unit">
                  <div className="kr-num-field__label">Единица</div>
                  <select value={form.unit || '%'} onChange={e => set('unit', e.target.value)} className="form-select form-select--sm">
                    {KR_UNITS.map(u => <option key={u} value={u}>{u}</option>)}
                  </select>
                </div>
                <div className="kre-metric-row__val">
                  <div className="kr-num-field__label">Старт (было)</div>
                  <div className="kr-num-input-suffix">
                    <NumInput value={form.start} onChange={v => set('start', v)} className="form-input form-input--sm" />
                    <span className="kr-num-input-suffix__unit">{form.unit}</span>
                  </div>
                </div>
                <span className="kre-metric-row__arrow" aria-hidden="true">→</span>
                <div className="kre-metric-row__val">
                  <div className="kr-num-field__label">Цель (станет)</div>
                  <div className="kr-num-input-suffix">
                    <NumInput value={form.target} onChange={v => set('target', v)} className="form-input form-input--sm" />
                    <span className="kr-num-input-suffix__unit">{form.unit}</span>
                  </div>
                </div>
              </div>
              {isNew && <div className="kre-metric-note">Текущее значение на старте равно стартовому — дальше меняется при обновлении прогресса.</div>}
              <div className="kr-checkpoints">
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
            </div>
          )}
          {form.krType === 'BOOLEAN' && (
            <div className="kr-num-section">
              <div className="kr-num-section__title">True / False</div>
              <div className="kre-metric-note" style={{ marginTop: 0 }}>Прогресс 0% до достижения результата, 100% — после. Отмечается при обновлении прогресса.</div>
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
                  <span className="kr-steps-cols__name">Название шага</span>
                  <span className="kr-steps-cols__weight">Вес, %</span>
                  <span className="kr-steps-cols__del" />
                </div>
              )}
              {form.stages.map((st, i) => (
                <div key={st.id || i} className="kr-step-row">
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
// Целевое значение KR: направление ↑/↓ выводится из того, растёт метрика или падает.
function KRTarget({ kr }) {
  if (kr.krType === 'BOOLEAN') return <span className="kr-target__val">false → true</span>;
  if (kr.krType === 'PROJECT') {
    const st = kr.stages || [];
    return <span className="kr-target__val">{st.filter(s => s.done).length} → {st.length} шагов</span>;
  }
  const start = Number(kr.start) || 0;
  const target = Number(kr.target) || 0;
  const cur = Number(kr.current) || 0;
  const up = target >= start;
  return (
    <span className="kr-target__val">
      <span className={`kr-target__dir kr-target__dir--${up ? 'up' : 'down'}`}>{up ? '↑' : '↓'}</span>
      {fmtNum(cur)} → {fmtNum(target)}{kr.unit ? ` ${kr.unit}` : ''}
    </span>
  );
}

// Заметка к прогрессу — однострочная под строкой KR, длинная разворачивается по клику.
function KRNote({ note, open, onToggle }) {
  const text = (note && note.text) || '';
  if (!text) return null;
  const long = text.length > 90 || text.includes('\n');
  // Давность берём у самой заметки: kr.updated_at двигает любая правка KR.
  const d = note.daysAgo;
  const ago = d === 0 ? 'сегодня' : d != null ? `${d}д назад` : note.date;
  const cls = ['kr-note', open ? 'kr-note--open' : '', long ? 'kr-note--long' : ''].filter(Boolean).join(' ');
  const press = long ? { role: 'button', tabIndex: 0, 'aria-expanded': open, onClick: onToggle,
    onKeyDown: e => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onToggle(); } } } : {};
  return (
    <div className={cls} data-no-drag {...press}>
      <span className="kr-note__label">Заметка</span>
      {/* Разметка в тексте заметки нормативна (content-safety, «Поля, поддерживающие
          разметку»), поэтому текст всегда идёт через Markdown. В свёрнутом виде CSS
          делает содержимое строчным и обрезает его в одну строку. */}
      <Markdown text={text} className="kr-note__text" />
      {long && <span className="kr-note__toggle">{open ? 'Свернуть' : 'Ещё'}</span>}
      {open && <span className="kr-note__meta">
        Обновлена вместе с прогрессом{note.author ? ` · ${note.author}` : ''} · {ago}
      </span>}
    </div>
  );
}

// Обрезан ли текст в отведённой ему ширине. Обрезку нельзя угадать, её нужно измерить: один
// и тот же заголовок обрезается или нет в зависимости от числа лейблов, драйверов, связей и
// ширины окна. Механизм тот же, что у CollapsibleMarkdown в markdown.js — измерение в
// layout-фазе плюс ResizeObserver, который покрывает и изменение размера окна, и сворачивание
// боковой панели, и догрузку шрифтов. useLayoutEffect берём через React: деструктуризация в
// начале файла — глобальные const, делимые со всеми скриптами страницы, и лишнее имя там
// заводить незачем.
//
// На этом признаке держатся две вещи, поэтому заголовок измеряется один раз: подсказка с
// полным текстом (нужна только обрезанному — у влезающего она дублировала бы видимый текст и
// отняла бы подсказку действия у строки-предка, которая несёт title «Редактировать цель»;
// title внутреннего элемента вытесняет title предка на время наведения именно на текст) и
// выбор формы драйверов в useOwnersFit.
function useClipped(text) {
  const ref = useRef(null);
  const [clipped, setClipped] = useState(false);
  React.useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return undefined;
    // 1px запаса гасит субпиксельное округление ширины.
    const measure = () => setClipped(el.scrollWidth - el.clientWidth > 1);
    measure();
    if (typeof ResizeObserver === 'undefined') return undefined;
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, [text]);
  return [ref, clipped];
}

// Насколько шапка должна стать шире, чтобы снова пробовать имена драйверов. Без этой полосы
// каждый пиксель протягивания границы окна стоил бы цикла «вернули имена → заголовок обрезан →
// снова убрали» на каждой карточке доски. Цена полосы — имена возвращаются на несколько
// пикселей позже, чем могли бы.
const OWNERS_REFIT_PX = 20;

// Имена драйверов стоят в той же строке, что и заголовок цели, и спор за место решается в
// пользу заголовка: имена показываем, пока из-за них не обрезается заголовок. Порога по
// количеству драйверов нет — он ошибался бы в обе стороны: один драйвер при длинном заголовке
// на узком окне уже вытесняет текст, а три коротких имени при коротком заголовке помещаются
// свободно.
//
// Решение меняет ту самую раскладку, которую измеряет: убрали имена — заголовок перестал
// обрезаться — «место есть» — вернули имена — заголовок снова обрезан. Этот цикл разрывает
// память о ширине шапки, на которой имена не влезли: пока шапка не стала заметно шире,
// пробовать снова незачем, получили бы тот же обрезанный заголовок.
//
// Наблюдаем шапку, а не окно: её ширину меняет и размер окна, и сворачивание боковой панели,
// а наше переключение не меняет — меняется состав её детей, — поэтому наблюдатель не
// реагирует на собственную правку. Переключение идёт в layout-фазе, до отрисовки, так что
// промежуточное состояние «имена показаны и заголовок обрезан» на экран не попадает.
function useOwnersFit(titleClipped, resetKey) {
  const headRef = useRef(null);
  const [compact, setCompact] = useState(false);
  const brokeAt = useRef(Infinity);
  // Другой заголовок или другой состав драйверов — прежняя отметка не про них.
  React.useLayoutEffect(() => { brokeAt.current = Infinity; setCompact(false); }, [resetKey]);
  React.useLayoutEffect(() => {
    const el = headRef.current;
    if (!el) return undefined;
    const decide = () => {
      const w = el.clientWidth;
      if (!compact && titleClipped) { brokeAt.current = w; setCompact(true); }
      else if (compact && w > brokeAt.current + OWNERS_REFIT_PX) setCompact(false);
    };
    decide();
    if (typeof ResizeObserver === 'undefined') return undefined;
    const ro = new ResizeObserver(decide);
    ro.observe(el);
    return () => ro.disconnect();
  }, [compact, titleClipped]);
  return [headRef, compact];
}

function KRRow({ kr, goalId, goalTitle = '', editMode, onReload, accent, staleDays = 7, periodStatus, forecast = null, teamId = null, periodId = null, greenThreshold = 80, notesOpen = false }) {
  // Closed period is shown as fully done — purely visual (stored health_status is untouched),
  // so reopening the period restores each KR's original status.
  const displayHealth = periodStatus === 'closed' ? 'done' : kr.healthStatus;
  const [modal, setModal] = useState(null);
  // Показ заметки по умолчанию следует птичке доски (notesOpen). Пункт меню перекрывает
  // её для этого KR; переключение птички сбрасывает перекрытие, и доска снова однородна.
  // noteExpanded — не «видна ли заметка», а «развёрнута ли длинная»: нажатие на саму
  // заметку сворачивает её до одной строки, как и обещает подпись «Свернуть», а скрывает
  // заметку только пункт меню.
  const [noteVisOverride, setNoteVisOverride] = useState(null);
  const [noteExpanded, setNoteExpanded] = useState(true);
  const showNote = noteVisOverride === null ? notesOpen : noteVisOverride;
  useEffect(() => { setNoteVisOverride(null); setNoteExpanded(true); }, [notesOpen]);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [nameRef, nameClipped] = useClipped(kr.name);
  const progress = kr.progress;
  // Цвет давности задаётся классом: fresh → warn → stale по тому же порогу.
  // Цвет давности считаем по прогрессу, а не по kr.updated_at. Если прогресс ещё
  // не обновляли, это не «свежо» — показываем как устаревшее.
  const pAgo = kr.progressDaysAgo;
  const staleLevel = pAgo == null || pAgo > staleDays ? 'stale' : pAgo > staleDays * 0.6 ? 'warn' : 'fresh';
  // Правка текстовых полей открыта только в «черновике» и «к валидации»; в «в работе»
  // строка предлагает обновление прогресса, в закрытом периоде — ничего.
  // Через actionAvailability, а не через editMode === 'full': знание «что при каком
  // статусе можно» живёт в одном месте (решение 4 в design.md).
  const canEditText = !lockReason(editMode, 'kr_edit');
  // Полоса KR краснеет, когда отставание от ожидаемого темпа больше порога.
  const krBehind = periodStatus !== 'closed' && forecast != null && forecast - progress > KR_BEHIND_PP;
  const krBarC = krBehind ? '#dc2626' : 'var(--accent)';
  const openEdit = () => setModal('edit');
  const onSaved = () => { setModal(null); onReload(); };
  const progressTitle = `Прогресс ${progress}%`
    + (forecast != null ? ` · ожидаемо к сегодня ${forecast}%` : '')
    + (krBehind ? ` · отставание больше ${KR_BEHIND_PP} п.п.` : '');
  return (
    <>
      <div className="kr-row">
        <div className="kr-row__main">
          <KRHealthDot status={displayHealth} progress={progress} greenThreshold={greenThreshold} />
          <div className="kr-weight-chip has-tip" tabIndex={0}
            data-tip-title={`Вес KR · ${kr.weight}%`}
            data-tip="Доля KR в прогрессе цели. Сумма весов всех KR цели — 100%.">{kr.weight}%</div>
          <div className="kr-info">
            <div className={`kr-name-row${canEditText ? ' title-editable' : ''}`}
              {...(canEditText ? { role: 'button', tabIndex: 0, title: 'Редактировать KR', onClick: openEdit,
                onKeyDown: e => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); openEdit(); } } } : {})}>
              <div ref={nameRef} title={nameClipped ? kr.name : undefined} className="kr-name">{kr.name}</div>
              {canEditText && <span className="title-edit" aria-hidden="true">✎</span>}
            </div>
            {kr.desc && <CollapsibleMarkdown text={kr.desc} className="kr-desc" />}
            {kr.zeroing && (
              <div className="kr-zeroing-note kr-zeroing-note--clamp" title={kr.zeroing}>
                <span className="kr-zeroing-note__icon">⊘</span>Критерий обнуления: {kr.zeroing}
              </div>
            )}
          </div>
          <div className="kr-measure">
            <div className="kr-target-col"><KRTarget kr={kr} /></div>
            <div className="kr-progress-col">
              <div className="kr-progress-col__body kr-progress-col__body--row" title={progressTitle}>
                <div className="kr-progress-col__bar">
                  <ProgressBar value={progress} forecast={forecast} h={5} color={krBarC} />
                </div>
                <span className="kr-pct" style={{ color: krBarC }}>{progress}%</span>
              </div>
            </div>
          </div>
          <div className="kr-row__actions" data-no-drag>
            {/* Распорка держит колонку действий одной ширины во всех режимах:
                в «закрыт» кнопки нет, но строка не должна съезжать. */}
            <span className="kr-row__spacer" />
            <div className="kr-update-stack">
              {editMode === 'progress_only' && (
                <button type="button" className="kr-row-btn kr-row-btn--accent" onClick={() => setModal('progress')}>
                  <span>↻</span>Обновить
                </button>
              )}
              {canEditText && (
                <button type="button" className="kr-row-btn" onClick={openEdit}>
                  <span>✎</span>Редактировать
                </button>
              )}
              {!canEditText && (
                <span className={`kr-updated kr-updated--${staleLevel}`}
                  title={pAgo == null ? 'Прогресс ещё не обновляли' : 'Последнее обновление прогресса'}>
                  {pAgo == null ? 'без обновлений' : pAgo === 0 ? 'обн. сегодня' : `обн. ${pAgo}д назад`}
                </span>
              )}
            </div>
            <RowMenu items={[
              { icon: '✎', label: 'Редактировать', onClick: openEdit, reason: editLockReason(editMode, 'KR') },
              { icon: '🔗', label: 'Копировать ссылку', confirmLabel: 'Скопировано',
                onClick: () => copyGoalURL(teamId, periodId, goalId, kr.id) },
              // Признак — текст заметки, а не сама запись: удалить запись через API нельзя,
              // поэтому у KR с очищенной заметкой note остаётся объектом с пустым text
              // (контракт key_results[].note). По записи пункт предлагал бы показать то,
              // чего нет: KRNote на пустом тексте не рисует ничего.
              kr.note?.text && { icon: '📝', label: showNote ? 'Скрыть заметку' : 'Показать заметку', onClick: () => setNoteVisOverride(!showNote) },
              { sep: true },
              { icon: '×', label: 'Удалить', danger: true, onClick: () => setConfirmDelete(true),
                reason: deleteLockReason(editMode, 'KR') },
            ]} />
          </div>
        </div>
        {showNote && <KRNote note={kr.note} open={noteExpanded} onToggle={() => setNoteExpanded(v => !v)} />}
      </div>
      {modal === 'progress' && <KRProgressModal kr={kr} goalTitle={goalTitle} onSave={onSaved} onClose={() => setModal(null)} accent={accent} />}
      {modal === 'edit' && <KREditModal kr={kr} goalId={goalId} onSave={onSaved} onClose={() => setModal(null)} accent={accent} />}
      {confirmDelete && <ConfirmModal title="Удалить Key Result?" message={`«${kr.name}» будет удалён без возможности восстановления.`}
        onConfirm={async () => { await apiDelete(`/api/v1/krs/${kr.id}`); setConfirmDelete(false); onReload(); }}
        onClose={() => setConfirmDelete(false)} />}
    </>
  );
}

// ── COMMENTS PANEL ────────────────────────────────────────────────────────────
// canModerate: the current user authored the item, or is a tenant admin.
function canModerate(authorUdid, me, isAdmin) {
  return isAdmin || (!!me && !!authorUdid && me.udid === authorUdid);
}

// A single reply row (one level, under a task). Replies are not resolvable.
function ReplyRow({ r, canDelete, onDelete }) {
  const [busy, setBusy] = useState(false);
  const [confirm, setConfirm] = useState(false);
  const act = async fn => { setBusy(true); try { await fn(); } catch { } finally { setBusy(false); } };
  return (
    <div id={`comment-${r.id}`} className="comment comment--reply">
      <AvatarWithUDID name={r.author} udid={r.authorUdid} size={24} />
      <div className="comment__content">
        <div className="comment__header">
          <span className="comment__author">{r.author}</span>
          <span className="comment__date">{r.date}</span>
        </div>
        <Markdown text={r.text} className="comment__text" />
        {canDelete && (
          <div className="comment__actions">
            {confirm ? (
              <>
                <span className="comment__confirm">Удалить ответ?</span>
                <button className="comment__link-btn" disabled={busy} onClick={() => act(() => onDelete(r.id))}>Да</button>
                <button className="comment__link-btn" disabled={busy} onClick={() => setConfirm(false)}>Отмена</button>
              </>
            ) : (
              <button className="comment__link-btn" onClick={() => setConfirm(true)}>Удалить</button>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

// A single task row (first-level comment). Unresolved tasks expose "resolve"; resolved
// ones are dimmed with a reopen link. Tasks can be replied to and deleted (author/admin).
function CommentRow({ c, onResolve, onUnresolve, onReply, onDelete, me, isAdmin }) {
  const [busy, setBusy] = useState(false);
  const [replyText, setReplyText] = useState('');
  const [showReply, setShowReply] = useState(false);
  const [confirm, setConfirm] = useState(false);
  const act = async fn => { setBusy(true); try { await fn(); } catch { } finally { setBusy(false); } };
  const canDel = canModerate(c.authorUdid, me, isAdmin);
  const submitReply = async () => {
    if (!replyText.trim()) return;
    await act(() => onReply(c.id, replyText.trim()));
    setReplyText(''); setShowReply(false);
  };
  return (
    <div id={`comment-${c.id}`} className={`comment${c.resolved ? ' comment--resolved' : ' comment--task-open'}`}>
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
            <span className="comment__resolved-info">Решено{c.resolvedBy ? ` · ${c.resolvedBy}` : ''}{c.resolvedAt ? ` · ${c.resolvedAt}` : ''}</span>
            <span className="comment__actions-inline">
              <button className="comment__link-btn" disabled={busy} onClick={() => act(() => onUnresolve(c.id))}><span className="comment__link-ic">↺</span>Вернуть</button>
              <span className="comment__sep">·</span>
              <button className="comment__link-btn" onClick={() => setShowReply(v => !v)}><span className="comment__link-ic">↩</span>Ответить</button>
              {canDel && !confirm && <><span className="comment__sep">·</span>
                <button className="comment__link-btn comment__link-btn--danger" onClick={() => setConfirm(true)}><span className="comment__link-ic">🗑</span>Удалить</button></>}
              {canDel && confirm && (
                <>
                  <span className="comment__sep">·</span>
                  <span className="comment__confirm">Удалить замечание и ответы?</span>
                  <button className="comment__link-btn comment__link-btn--danger" disabled={busy} onClick={() => act(() => onDelete(c.id))}>Да</button>
                  <span className="comment__sep">·</span>
                  <button className="comment__link-btn" disabled={busy} onClick={() => setConfirm(false)}>Отмена</button>
                </>
              )}
            </span>
          </div>
        ) : (
          <div className="comment__actions">
            <button className="comment__resolve-btn" disabled={busy} onClick={() => act(() => onResolve(c.id))}>✓ Отметить решённым</button>
            <button className="comment__link-btn" onClick={() => setShowReply(v => !v)}><span className="comment__link-ic">↩</span>Ответить</button>
            {canDel && !confirm && <button className="comment__link-btn comment__link-btn--danger" onClick={() => setConfirm(true)}><span className="comment__link-ic">🗑</span>Удалить</button>}
            {canDel && confirm && (
              <>
                <span className="comment__confirm">Удалить замечание и ответы?</span>
                <button className="comment__link-btn comment__link-btn--danger" disabled={busy} onClick={() => act(() => onDelete(c.id))}>Да</button>
                <button className="comment__link-btn" disabled={busy} onClick={() => setConfirm(false)}>Отмена</button>
              </>
            )}
          </div>
        )}
        {(c.replies || []).length > 0 && (
          <div className="comment__replies">
            {c.replies.map(r => (
              <ReplyRow key={r.id} r={r} canDelete={canModerate(r.authorUdid, me, isAdmin)} onDelete={onDelete} />
            ))}
          </div>
        )}
        {showReply && (
          <div className="comment-compose comment-compose--reply">
            <MarkdownEditor value={replyText} onChange={setReplyText} rows={2} placeholder="Ответ… (Cmd+Enter)"
              onKeyDown={e => { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) submitReply(); }}
              textareaClassName="form-textarea form-textarea--sm" textareaStyle={{ width: '100%', resize: 'vertical' }} />
            <div className="comment-submit-row">
              <button onClick={() => { setShowReply(false); setReplyText(''); }} disabled={busy}
                className="comment-cancel">Отмена</button>
              <button onClick={submitReply} disabled={!replyText.trim() || busy}
                className={`comment-submit ${replyText.trim() ? 'comment-submit--active' : 'comment-submit--disabled'}`}>Ответить</button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

function CommentsPanel({ comments, onAdd, onResolve, onUnresolve, onReply, onDelete, me, isAdmin }) {
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
        <CommentRow key={c.id || i} c={c} onResolve={onResolve} onUnresolve={onUnresolve} onReply={onReply} onDelete={onDelete} me={me} isAdmin={isAdmin} />
      ))}
      {resolved.length > 0 && (
        <div className="comments-panel__resolved-head">Решённые · {resolved.length}</div>
      )}
      {resolved.map((c, i) => (
        <CommentRow key={c.id || `r${i}`} c={c} onResolve={onResolve} onUnresolve={onUnresolve} onReply={onReply} onDelete={onDelete} me={me} isAdmin={isAdmin} />
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

// Copy a shareable deep-link to this goal. URL shape and open behavior match the
// activity-log "↗ к цели" link (shared buildTargetURL from ui.js). For a shared goal
// the link points at the currently-open team, so it resolves back to the same board.
// Копирование ссылки на цель или на её ключевой результат: пункт «Копировать ссылку»
// есть в меню обоих, и оба подтверждают копирование через confirmLabel в MenuItem.
async function copyGoalURL(teamId, periodId, goalId, krId = null) {
  const path = buildTargetURL({ team_id: teamId, period_id: periodId, goal_id: goalId, kr_id: krId });
  if (!path) return false;
  const url = location.origin + path;
  // Clipboard API может существовать и при этом отказать: страница без фокуса,
  // окно без разрешения, небезопасный контекст. Поэтому textarea — не ветка
  // «если API нет», а именно резерв на отказ: иначе копирование молча срывается.
  const viaTextarea = () => {
    const ta = document.createElement('textarea');
    ta.value = url; ta.style.position = 'fixed'; ta.style.opacity = '0';
    document.body.appendChild(ta); ta.focus(); ta.select();
    const ok = document.execCommand('copy');
    document.body.removeChild(ta);
    return ok;
  };
  if (navigator.clipboard && navigator.clipboard.writeText) {
    try {
      await navigator.clipboard.writeText(url);
      return true;
    } catch { /* падаем в резерв ниже */ }
  }
  try { return viaTextarea(); } catch { return false; }
}

// ── MARKDOWN EXPORT ─────────────────────────────────────────────────────────
function pluralRu(n, forms) {
  const mod10 = n % 10, mod100 = n % 100;
  if (mod10 === 1 && mod100 !== 11) return forms[0];
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 10 || mod100 >= 20)) return forms[1];
  return forms[2];
}

// ExportModal fetches server-rendered Markdown for the chosen scope/options and lets the user
// preview, copy or download it. Generation is entirely server-side (single source of truth);
// this component only displays the returned text and derives the download Blob from it.
// initialScope задаёт охват по точке вызова: из меню цели — «одна цель», из меню
// команды — «цели команды». Так охват совпадает с тем, откуда экспорт открыли.
function ExportModal({ goal, teamId, periodId, info, onClose, initialScope = 'goal' }) {
  const [scope, setScope] = useState(initialScope);
  const [full, setFull] = useState(false);
  const [comments, setComments] = useState(false);
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const [copied, setCopied] = useState(false);
  const [reloadTick, setReloadTick] = useState(0);
  const { requestClose } = useModalClose({ isDirty: false, onClose });
  const overlay = useOverlayClose(requestClose);

  // Identifies the current selection. Responses are tagged with the key they were fetched for, so
  // a response for stale options never drives the preview, filename or copy/download actions.
  const curKey = `${scope}|${full ? 'full' : 'short'}|${comments ? '1' : '0'}`;

  useEffect(() => {
    let cancelled = false;
    setLoading(true); setError(false);
    const params = new URLSearchParams({
      period_id: periodId, scope,
      format: full ? 'full' : 'short',
      comments: comments ? '1' : '0',
    });
    if (scope === 'goal') params.set('goal_id', goal.id);
    const t = setTimeout(async () => {
      try {
        const d = await apiGet(`/api/v1/teams/${teamId}/export?${params.toString()}`);
        if (!cancelled) { setData({ ...d, key: curKey }); setLoading(false); }
      } catch {
        if (!cancelled) { setError(true); setLoading(false); }
      }
    }, 250);
    return () => { cancelled = true; clearTimeout(t); };
  }, [scope, full, comments, reloadTick, teamId, periodId, goal.id]);

  // Only a response matching the current selection (and not mid-load/error) is actionable.
  const fresh = !loading && !error && data && data.key === curKey ? data : null;

  const copy = async () => {
    if (!fresh) return;
    try {
      if (navigator.clipboard && navigator.clipboard.writeText) {
        await navigator.clipboard.writeText(fresh.markdown);
      } else {
        const ta = document.createElement('textarea');
        ta.value = fresh.markdown; ta.style.position = 'fixed'; ta.style.opacity = '0';
        document.body.appendChild(ta); ta.focus(); ta.select();
        document.execCommand('copy'); document.body.removeChild(ta);
      }
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch { /* silent */ }
  };

  const download = () => {
    if (!fresh) return;
    const blob = new Blob([fresh.markdown], { type: 'text/markdown;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url; a.download = fresh.filename; document.body.appendChild(a); a.click();
    document.body.removeChild(a); URL.revokeObjectURL(url);
  };

  const teamGoals = info.teamGoalCount || 0;
  const subtreeTeams = info.subtreeTeamCount || 0;
  const cards = [
    { key: 'goal', title: 'Одна цель', sub: goal.title },
    { key: 'team', title: 'Цели команды', sub: `${teamGoals} ${pluralRu(teamGoals, ['цель', 'цели', 'целей'])}` },
    { key: 'tree', title: 'С вложенными командами', sub: `${subtreeTeams} ${pluralRu(subtreeTeams, ['команда', 'команды', 'команд'])} в структуре` },
  ];

  return (
    <div className="modal-overlay modal-overlay--z600" {...overlay}>
      <div onClick={e => e.stopPropagation()} className="modal-box modal-box--w720 export-modal">
        <div className="export-modal__header">
          <div className="modal-title">Экспорт целей в Markdown</div>
          <div className="modal-subtitle">{[info.periodName, (info.teamPath || []).join(' / ')].filter(Boolean).join(' · ')}</div>
        </div>
        <div className="export-modal__scopes">
          {cards.map(c => (
            <button key={c.key} type="button"
              onClick={() => setScope(c.key)}
              className={`export-modal__scope-card${scope === c.key ? ' export-modal__scope-card--active' : ''}`}>
              <div className="export-modal__scope-title">{c.title}</div>
              <div className="export-modal__scope-sub">{c.sub}</div>
            </button>
          ))}
        </div>
        <div className="export-modal__options">
          <label className="export-modal__check"><input type="checkbox" checked={full} onChange={e => setFull(e.target.checked)} /> Полный экспорт</label>
          <label className="export-modal__check"><input type="checkbox" checked={comments} onChange={e => setComments(e.target.checked)} /> Комментарии</label>
        </div>
        <div className="export-modal__preview-wrap">
          {error && (
            <div className="export-modal__state">
              Не удалось построить экспорт.
              <button type="button" className="btn btn--secondary export-modal__retry" onClick={() => setReloadTick(t => t + 1)}>Повторить</button>
            </div>
          )}
          {!error && !fresh && <div className="export-modal__state">Загрузка превью…</div>}
          {!error && fresh && <pre className="export-modal__preview">{fresh.markdown}</pre>}
        </div>
        <div className="export-modal__footer">
          <div className="export-modal__filename">{fresh ? `${fresh.filename} · ${fresh.lines} ${pluralRu(fresh.lines, ['строка', 'строки', 'строк'])}` : ''}</div>
          <div className="export-modal__actions">
            <button type="button" className="btn btn--secondary" onClick={onClose}>Закрыть</button>
            <button type="button" className="btn btn--secondary" onClick={copy} disabled={!fresh}>{copied ? '✓ Скопировано' : 'Скопировать'}</button>
            <button type="button" className="btn btn--primary" onClick={download} disabled={!fresh}>Скачать .md</button>
          </div>
        </div>
      </div>
    </div>
  );
}

// TransferGoalModal copies or moves a goal into a chosen team + period.
function TransferGoalModal({ goal, teamId, periodId, allTeams, onClose, onDone, editMode = 'full' }) {
  // Перенос убирает цель из состава текущей команды, поэтому закрыт тем же замком,
  // что правка и удаление. Копирование состав не меняет и доступно в любом статусе:
  // скопировать цели закрытого периода в новый — обычная работа, а не правка старого.
  const moveLock = lockReason(editMode, 'goal_move');
  const [mode, setMode] = useState('copy'); // 'copy' | 'move'
  const [targetTeam, setTargetTeam] = useState(teamId);
  const [targetPeriod, setTargetPeriod] = useState(periodId);
  const [withComments, setWithComments] = useState(false);
  const [withProgress, setWithProgress] = useState(false);
  const [periods, setPeriods] = useState([]);
  const [hierarchy, setHierarchy] = useState(allTeams || []);
  const [loadingTeams, setLoadingTeams] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');

  useEffect(() => { apiGet('/api/v1/periods').then(d => setPeriods((d && d.items) || [])).catch(() => setPeriods([])); }, []);

  // Reload hierarchy for the selected target period to know per-team status blocking.
  useEffect(() => {
    let alive = true;
    setLoadingTeams(true);
    apiGet(`/api/v1/hierarchy?period_id=${targetPeriod}`)
      .then(data => { if (alive) setHierarchy((data && data.items) || []); })
      .catch(() => { if (alive) setHierarchy([]); })
      .finally(() => { if (alive) setLoadingTeams(false); });
    return () => { alive = false; };
  }, [targetPeriod]);

  // Teams whose status in the target period is in_progress/closed are blocked.
  const flat = flattenTree(hierarchy || []);
  const blockedIds = flat.filter(t => t.status === 'in_progress' || t.status === 'closed').map(t => t.id);
  const blockedReason = {};
  flat.forEach(t => { if (t.status === 'in_progress') blockedReason[t.id] = 'в работе'; else if (t.status === 'closed') blockedReason[t.id] = 'закрыто'; });

  // Compare against the goal's OWNER team/period (goal.teamId/periodId), not the current board:
  // for a shared goal, teamId here is the board team, and moving into it is a valid ownership
  // transfer the backend allows (it compares against src.TeamID).
  const sameAsSource = mode === 'move' && targetTeam === goal.teamId && targetPeriod === goal.periodId;
  const targetBlocked = blockedIds.includes(targetTeam);
  const canSubmit = !!targetTeam && !!targetPeriod && !sameAsSource && !targetBlocked && !busy;

  const requestClose = () => { if (!busy) onClose(); };
  const overlay = useOverlayClose(requestClose);

  const submit = async () => {
    setBusy(true); setErr('');
    try {
      await apiPost(`/api/v1/goals/${goal.id}/transfer`, {
        mode, target_team_id: targetTeam, target_period_id: targetPeriod,
        with_comments: withComments, with_progress: withProgress,
      });
      onDone && onDone();
      onClose();
    } catch (e) {
      setErr('Не удалось выполнить операцию. Возможно, цели целевой команды уже в работе или закрыты.');
      setBusy(false);
    }
  };

  return (
    <div className="modal-overlay modal-overlay--z400" {...overlay}>
      <div className="modal-box transfer-modal">
        <div className="modal-header">
          <div>
            <div className="transfer-modal__title">Перенести или скопировать цель</div>
            <div className="transfer-modal__subtitle">{goal.title}</div>
          </div>
          <button onClick={requestClose} className="modal-close">×</button>
        </div>
        <div className="modal-body">
          <div className="seg-group transfer-modal__mode">
            <button type="button" className={`seg-btn${mode === 'copy' ? ' seg-btn--active' : ''}`} onClick={() => setMode('copy')}>⧉ Копировать</button>
            {moveLock
              ? <LockedAction reason={moveLock}>
                  <button type="button" aria-disabled="true" className="seg-btn seg-btn--locked">➡ Перенести 🔒</button>
                </LockedAction>
              : <button type="button" className={`seg-btn${mode === 'move' ? ' seg-btn--active' : ''}`} onClick={() => setMode('move')}>➡ Перенести</button>}
          </div>

          <div className="transfer-modal__field">
            <div className="transfer-modal__label">Куда — команда</div>
            <TeamCombobox single selectedIds={targetTeam ? [targetTeam] : []} onChange={ids => setTargetTeam(ids[0])}
              allTeams={hierarchy} blockedIds={blockedIds} blockedReason={blockedReason} />
            {loadingTeams && <div className="transfer-modal__hint">Загрузка команд…</div>}
          </div>

          <div className="transfer-modal__field">
            <div className="transfer-modal__label">Куда — период</div>
            <PeriodSelect periods={periods} periodId={targetPeriod} onChange={setTargetPeriod} width="100%" variant="light" />
          </div>

          <label className="transfer-modal__check">
            <input type="checkbox" checked={withComments} onChange={e => setWithComments(e.target.checked)} />
            <span>Перенести комментарии</span>
          </label>
          <label className="transfer-modal__check">
            <input type="checkbox" checked={withProgress} onChange={e => setWithProgress(e.target.checked)} />
            <span>Перенести прогресс и заметки KR</span>
          </label>

          {sameAsSource && <div className="transfer-modal__error">Перенос в ту же команду и период невозможен.</div>}
          {targetBlocked && !sameAsSource && <div className="transfer-modal__error">Цели выбранной команды в этом периоде уже в работе или закрыты.</div>}
          {err && <div className="transfer-modal__error">{err}</div>}
        </div>
        <div className="modal-footer">
          <button type="button" className="btn btn--secondary" onClick={requestClose}>Отмена</button>
          <button type="button" className="btn btn--primary" disabled={!canSubmit} onClick={submit}>
            {busy ? '…' : (mode === 'move' ? 'Перенести' : 'Скопировать')}
          </button>
        </div>
      </div>
    </div>
  );
}

function GoalCard({ goal, editMode, onReload, onEditGoal, onExportGoal = () => { }, me, isAdmin = false, accent, currentTeamId, periodId, allTeams, dragProps, onReorderKR, staleDays = 7, periodStatus, greenThreshold = 80, deepLink = null, notesOpen = false }) {
  // A deep link (?goal/kr/comment) targeting this goal forces the relevant sections open.
  const isDeepTarget = deepLink && deepLink.goal === goal.id;
  // Ключевые результаты видны всегда, поэтому раскрывать по глубокой ссылке нечего:
  // остаётся только автораскрытие обсуждения по ссылке на комментарий.
  const [showCom, setShowCom] = useState(!!(isDeepTarget && deepLink.comment));
  const [newKR, setNewKR] = useState(false);
  const [krDrag, setKrDrag] = useState(null);
  // A KR row is draggable as a whole, so a press on a nested control (the description's
  // "Показать полностью") would otherwise start a reorder. A ref, not state: it is read
  // synchronously in onDragStart, with no re-render in between to make it stale.
  const krPressNoDrag = React.useRef(false);
  // Карточка становится перетаскиваемой только на время жеста от ручки: будь она
  // draggable всегда, в описании цели нельзя было бы выделить текст.
  const [goalDraggable, setGoalDraggable] = useState(false);
  // Признак НЕЛЬЗЯ снимать по mouseleave ручки. Ручка 16×42px, а чтобы потащить карточку,
  // указатель обязан её покинуть — mouseleave успевал снять draggable раньше, чем браузер
  // решал, что это перетаскивание, и dragstart не наступал вовсе: порядок целей не менялся.
  // Снимаем по dragend карточки, а этим эффектом — по отпусканию кнопки где угодно (ручку
  // нажали, но не потащили). Во время перетаскивания mouseup не приходит, вместо него
  // приходит dragend, поэтому эффект начатый жест не прерывает.
  useEffect(() => {
    if (!goalDraggable) return;
    const release = () => setGoalDraggable(false);
    window.addEventListener('mouseup', release);
    return () => window.removeEventListener('mouseup', release);
  }, [goalDraggable]);
  const [confirmDeleteGoal, setConfirmDeleteGoal] = useState(false);
  const [transfer, setTransfer] = useState(false);
  const [titleRef, titleClipped] = useClipped(goal.title);
  // Имена драйверов уступают место заголовку: решение измеряется по фактической раскладке,
  // а не по количеству драйверов. Отметка «не влезло» привязана к заголовку и составу
  // драйверов — меняется что-то из этого, и подбор начинается заново.
  const [headRef, ownersCompact] = useOwnersFit(titleClipped, `${goal.title}|${goal.owners.length}`);
  const prog = goal.progress || 0;
  // "N дней без обновления" is an execution-phase signal: it applies only while
  // the team is in_progress ("в работе"). Drafts, goals awaiting validation and
  // closed periods are not being actively executed, so it is not meaningful for
  // them. The threshold is the tenant's stale_days progress threshold.
  const staleTracked = periodStatus === 'in_progress';
  const isStale = staleTracked && goal.updatedDaysAgo > staleDays;
  const forecast = goal.progressMeta?.forecast ?? null;
  const hC = HEALTH_COLOR[healthOf(prog, isStale, forecast, greenThreshold)];
  const health = healthOf(prog, isStale, forecast, greenThreshold);
  // Как и в KRRow — через actionAvailability, чтобы правило статуса было записано
  // в одном месте (решение 4 в design.md). Переупорядочивание идёт тем же замком.
  const canEdit = !lockReason(editMode, 'reorder');
  // Правка текстовых полей цели открыта только там же, где полное редактирование:
  // в «в работе» и «закрыт» заголовок перестаёт быть элементом управления.
  const canEditText = canEdit;
  // Недоступное по статусу действие не исчезает: остаётся на месте и называет причину.
  const goalEditLock = lockReason(editMode, 'goal_edit');
  const goalDeleteLock = lockReason(editMode, 'goal_delete');
  const krCreateLock = lockReason(editMode, 'kr_create');
  const canReorderGoal = canEdit && !!dragProps;
  const { isDragging, ...rootDrag } = dragProps || {};
  const otherTeams = (goal.shareTeams || []).filter(t => t.id !== currentTeamId);
  const isShared = otherTeams.length > 0;
  const krWeightSum = (goal.krs || []).reduce((s, k) => s + (k.weight || 0), 0);
  // У цели без KR сумма весов равна нулю по определению, а не по ошибке: предупреждать
  // там не о чем, и «нет KR» говорит сама пустая секция. Так же огранено предупреждение
  // о сумме весов целей в App.
  const krWeightOff = (goal.krs || []).length > 0 && krWeightSum !== 100;
  const krWeightDelta = 100 - krWeightSum;

  const addGoalComment = async text => { await apiPost(`/api/v1/goals/${goal.id}/comments`, { text }); onReload(); };
  const addGoalReply = async (parentId, text) => { await apiPost(`/api/v1/goals/${goal.id}/comments/${parentId}/replies`, { text }); onReload(); };
  const deleteComment = async commentId => { await apiDelete(`/api/v1/goals/${goal.id}/comments/${commentId}`); onReload(); };
  const resolveComment = async commentId => { await apiPost(`/api/v1/goals/${goal.id}/comments/${commentId}/resolve`, {}); onReload(); };
  const unresolveComment = async commentId => { await apiPost(`/api/v1/goals/${goal.id}/comments/${commentId}/unresolve`, {}); onReload(); };
  const unresolvedCount = (goal.comments || []).filter(c => !c.resolved).length;
  // For a shared goal, deleting only detaches the current team (leaves the share); the goal stays
  // for the other participating teams. A goal that is not shared is deleted outright.
  const handleDeleteGoal = async () => {
    await apiDelete(isShared ? `/api/v1/goals/${goal.id}/share/${currentTeamId}` : `/api/v1/goals/${goal.id}`);
    onReload();
  };

  // Модификатора --reorderable у цели нет: ручка перетаскивания и так рисуется
  // условно и позиционируется сама, отдельного класса-крючка под неё не нужно.
  // У .kr-item--reorderable он есть — там он реально сдвигает строку под ручку.
  const cardClass = ['goal-card',
    isDragging ? 'goal-card--dragging' : '',
    isShared ? 'goal-card--shared' : '',
    isStale ? 'goal-card--stale' : '',
  ].filter(Boolean).join(' ');

  return (
    <div {...rootDrag} id={`goal-${goal.id}`} draggable={!!(canReorderGoal && goalDraggable)}
      onDragEnd={e => { setGoalDraggable(false); rootDrag.onDragEnd && rootDrag.onDragEnd(e); }}
      style={{ '--health': hC, '--health-soft': `${hC}18` }}
      className={cardClass}>
      {canReorderGoal && (
        <div className="drag-handle" title="Перетащите для изменения порядка"
          onMouseDown={() => setGoalDraggable(true)}>⋮⋮</div>
      )}
      {/* Шапка цели — одна строка: вес · приоритет · заголовок со связями · лейблы ·
          драйвер · полоса прогресса · процент · меню. Заголовок и есть элемент
          редактирования, ✎ рядом — только признак того, что по нему можно нажать. */}
      <div className="goal-card__body gc2">
        <div ref={headRef} className="gc2__head">
          <span className="gc2__weight has-tip" tabIndex={0}
            data-tip-title={`Вес цели · ${goal.weight}%`}
            data-tip="Доля цели в общем прогрессе команды за период. Сумма весов всех целей — 100%.">{goal.weight}%</span>
          <span className="has-tip" tabIndex={0}
            data-tip-title={`Приоритет · ${goal.priority}`}
            data-tip={PRI_HINT[goal.priority] || ''}><PriBadge p={goal.priority} /></span>
          <div className="gc2__title-wrap">
            <div className={`goal-card__title-row${canEditText ? ' title-editable' : ''}`}
              {...(canEditText ? { role: 'button', tabIndex: 0, title: 'Редактировать цель',
                onClick: () => onEditGoal(goal),
                onKeyDown: e => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onEditGoal(goal); } } } : {})}>
              <div ref={titleRef} title={titleClipped ? goal.title : undefined} className="goal-card__title goal-card__title--readonly">{goal.title}</div>
              {canEditText && <span className="title-edit" aria-hidden="true">✎</span>}
            </div>
            <GoalLinksPopover dir="up" items={goal.parents} />
            <GoalLinksPopover dir="down" items={goal.children} />
          </div>
          <div className="gc2__labels">
            <Badge label={goal.type === 'delivery' ? 'Delivery' : 'Discovery'} color={goal.type === 'delivery' ? '#374151' : '#7c3aed'} />
            {goal.focus && <Badge label={focusLabel(goal.focus)} color={FOCUS_COLORS[goal.focus] || FOCUS_COLORS.DEFAULT} />}
          </div>
          {goal.owners.length > 0 && (
            // Когда имена не вмещаются без обрезания заголовка, остаются только аватары.
            // Список при этом полный: имя отдаёт карточка пользователя по наведению.
            <div className={`goal-card__owner gc2__owner${ownersCompact ? ' gc2__owner--compact' : ''}`}
              title="Драйвер цели">
              {goal.owners.map(u => <UserInfo key={u.udid || u.display_name} userRef={u} size={18} />)}
            </div>
          )}
          <div className="gc2__bar" title={`Прогресс ${prog}%${forecast != null ? ` · прогноз ${forecast}%` : ''}`}>
            <ProgressBar value={prog} forecast={forecast} h={5} color="var(--health)" />
          </div>
          <span className="goal-card__progress-pct">{prog}%</span>
          <RowMenu items={[
            { icon: '✎', label: 'Редактировать', onClick: () => onEditGoal(goal), reason: editLockReason(editMode, 'цель') },
            { icon: '🔗', label: 'Копировать ссылку', confirmLabel: 'Скопировано',
              onClick: () => copyGoalURL(currentTeamId, periodId, goal.id) },
            { icon: '➡', label: lockReason(editMode, 'goal_move') ? 'Скопировать в другую команду' : 'Перенести или скопировать',
              onClick: () => setTransfer(true) },
            // Экспорт остаётся и здесь: охват «одна цель» иначе недоступен — из меню
            // команды выбрать конкретную цель нечем. Меню команды даёт охват команды
            // и поддерева, это меню — охват самой цели.
            { icon: '⇩', label: 'Экспорт в Markdown', onClick: () => onExportGoal(goal) },
            { sep: true },
            { icon: '×', label: isShared ? 'Открепить от команды' : 'Удалить', danger: true,
              onClick: () => setConfirmDeleteGoal(true), reason: deleteLockReason(editMode, 'цель') },
          ]} />
        </div>
        {goal.desc && <CollapsibleMarkdown text={goal.desc} className="goal-card__desc" />}
      </div>
      {otherTeams.length > 0 && (
        <div className="gc-share-strip" aria-label="Общая цель">
          <span className="gc-share-strip__label">⇄ Общая цель</span>
          {/* Текущая команда идёт первой. Отдельной пометки у неё нет: её отличает место в
              списке и то, что её название не является переходом. */}
          {[...(goal.shareTeams || []).filter(t => t.id === currentTeamId), ...otherTeams].map(t => {
            const isOwner = t.id === goal.teamId;
            const cls = `gc-share-strip__team${isOwner ? ' gc-share-strip__team--owner' : ''}`;
            const tip = isOwner ? 'Команда-владелец цели' : undefined;
            const body = (
              <>
                {t.name}
                {isOwner && <span className="gc-share-strip__role">владелец</span>}
              </>
            );
            // Переход на доску этой команды в периоде цели: общая цель живёт в одном
            // периоде, и все команды-участники видят её в нём же. Ссылка, а не кнопка с
            // location.assign, как в GoalLinksPopover: элемент статичен, и <a> бесплатно
            // даёт средний щелчок и контекстное меню браузера. Текущая команда переходом
            // не становится — её доска уже открыта. Доступность команды здесь не
            // проверяется: состав участников по спецификации полон, а поведение доски для
            // ссылки на недоступную команду уже определено.
            const href = t.id === currentTeamId ? null : buildTargetURL({ team_id: t.id, period_id: periodId });
            return href
              ? <a key={t.id} className={cls} title={tip} href={href}>{body}</a>
              : <span key={t.id} className={cls} title={tip}>{body}</span>;
          })}
        </div>
      )}
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
        {(goal.krs || []).length === 0 && (
          <div className="kr-section__empty">Ключевых результатов пока нет</div>
        )}
        {(goal.krs || []).map(kr => {
          const canReorderKR = canEdit && !!onReorderKR;
          const isKrDrag = krDrag === kr.id;
          return (
            <div key={kr.id} id={`kr-${kr.id}`}
              draggable={!!canReorderKR}
              onMouseDownCapture={canReorderKR ? (e) => { krPressNoDrag.current = !!(e.target.closest && e.target.closest('[data-no-drag]')); } : undefined}
              onDragStart={canReorderKR ? (e) => { if (krPressNoDrag.current) { e.preventDefault(); return; } e.stopPropagation(); e.dataTransfer.effectAllowed = 'move'; e.dataTransfer.setData('text/plain', 'kr'); setKrDrag(kr.id); } : undefined}
              onDragOver={canReorderKR ? (e) => { if (krDrag && krDrag !== kr.id) { e.preventDefault(); e.stopPropagation(); e.dataTransfer.dropEffect = 'move'; } } : undefined}
              onDrop={canReorderKR ? (e) => { e.preventDefault(); e.stopPropagation(); if (krDrag && krDrag !== kr.id) onReorderKR(krDrag, kr.id); setKrDrag(null); } : undefined}
              onDragEnd={canReorderKR ? () => setKrDrag(null) : undefined}
              className={`kr-item${isKrDrag ? ' kr-item--dragging' : ''}${canReorderKR ? ' kr-item--reorderable' : ''}`}>
              {canReorderKR && <div className="kr-item__drag-handle">⋮⋮</div>}
              <KRRow kr={kr} goalId={goal.id} goalTitle={goal.title} editMode={editMode} onReload={onReload}
                accent={accent} staleDays={staleDays} periodStatus={periodStatus} forecast={forecast}
                teamId={currentTeamId} periodId={periodId} greenThreshold={greenThreshold} notesOpen={notesOpen} />
            </div>
          );
        })}
        {/* Как в прототипе: кнопка добавления при блокировке скрывается. Объяснять
            причину — задача пунктов меню, которые остаются на месте и говорят её. */}
        {!krCreateLock && <button onClick={() => setNewKR(true)} className="kr-add-btn">+ Добавить KR</button>}
      </div>
      <button type="button" aria-expanded={showCom}
        className={`gc-comments-toggle${showCom ? ' gc-comments-toggle--open' : ''}${unresolvedCount > 0 ? ' gc-comments-toggle--warn' : ''}`}
        onClick={() => setShowCom(!showCom)}>
        <span className="gc-comments-toggle__caret">▶</span>
        <span>Комментарии</span>
        {(goal.comments || []).length > 0 && <span className="gc-comments-toggle__count">· {goal.comments.length}</span>}
        {unresolvedCount > 0 && <span className="gc-comments-toggle__pill">не решено {unresolvedCount}</span>}
      </button>
      {transfer && <TransferGoalModal goal={goal} teamId={currentTeamId} periodId={periodId} allTeams={allTeams}
        editMode={editMode} onClose={() => setTransfer(false)} onDone={onReload} />}
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
          <CommentsPanel comments={goal.comments} onAdd={addGoalComment} onResolve={resolveComment} onUnresolve={unresolveComment} onReply={addGoalReply} onDelete={deleteComment} me={me} isAdmin={isAdmin} />
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

// GoalLinksPopover — лейбл ↑N (родители «Вклад в») / ↓N (дети) с всплывающим списком
// связанных целей (название, период, команда, прогресс). Клик по строке — deep-link на цель.
// Паттерн портала/таймера повторяет UserInfo.
function GoalLinksPopover({ dir, items }) {
  const [popup, setPopup] = useState(null);
  const ref = useRef();
  const timer = useRef();
  if (!items || items.length === 0) return null;
  const isUp = dir === 'up';
  const label = `${isUp ? '↑' : '↓'} ${items.length}`;
  const title = isUp ? `↑ ВКЛАД В · ${items.length}` : `↓ ${items.length}`;
  const show = () => {
    clearTimeout(timer.current);
    if (!ref.current) return;
    const r = ref.current.getBoundingClientRect();
    const left = Math.max(8, Math.min(r.left, window.innerWidth - 328));
    setPopup({ top: r.bottom + 6, left });
  };
  const hide = () => { timer.current = setTimeout(() => setPopup(null), 150); };
  const openGoal = it => {
    const url = buildTargetURL({ team_id: it.teamId, period_id: it.periodId, goal_id: it.id });
    if (url) location.assign(url);
  };
  const popupEl = popup && ReactDOM.createPortal(
    <div className="goal-links-popup" style={{ top: popup.top, left: popup.left }}
      onMouseEnter={() => clearTimeout(timer.current)} onMouseLeave={hide}>
      <div className="goal-links-popup__head">{title}</div>
      {items.map(it => (
        <button key={it.id} type="button" className="goal-links-popup__row" onClick={() => openGoal(it)}>
          <span className="goal-links-popup__row-title">{it.title}</span>
          <span className="goal-links-popup__row-meta">{it.periodName} · {it.teamName}</span>
          <span className="goal-links-popup__row-progress">{it.progress}%</span>
        </button>
      ))}
    </div>,
    document.body
  );
  return (
    <span ref={ref} className={`goal-link-label goal-link-label--${isUp ? 'up' : 'down'}`}
      onMouseEnter={show} onMouseLeave={hide}>
      {label}
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

// GoalParentPicker — выбор родительской цели: селектор периода (по умолчанию текущий,
// плюс «Все периоды»), inline-поиск по названию цели/команде/лиду и список кандидатов,
// сгруппированный по командам (из /api/v1/goals/linkable). Переиспользует PeriodSelect.
function GoalParentPicker({ excludeGoalId, currentPeriodId, periods, selectedIds, allTeams, onPick, onClose }) {
  const [periodSel, setPeriodSel] = useState(currentPeriodId);
  const [q, setQ] = useState('');
  const [items, setItems] = useState([]);
  const [loading, setLoading] = useState(false);
  const selected = new Set(selectedIds || []);
  // Порядок и глубина команд — из той же иерархии, что и в сайдбаре (DFS с depth),
  // чтобы команды (и цели под ними) шли вложенно. Команды вне текущей иерархии
  // (напр. при «Все периоды») отображаются после, без отступа.
  const teamMeta = new Map();
  flattenTree(allTeams || []).forEach((t, i) => teamMeta.set(t.id, { depth: t.depth, order: i }));
  // Селектор периода: синтетический пункт «Все периоды» без правки общего компонента.
  const pickerPeriods = [{ id: 'all', name: 'Все периоды', status: 'active', depth: 0 }, ...(periods || [])];

  useEffect(() => {
    let alive = true;
    setLoading(true);
    const params = new URLSearchParams();
    params.set('period_id', periodSel === 'all' ? 'all' : String(periodSel));
    if (q.trim()) params.set('q', q.trim());
    if (excludeGoalId) params.set('exclude_goal_id', String(excludeGoalId));
    apiGet(`/api/v1/goals/linkable?${params.toString()}`)
      .then(rows => { if (alive) setItems((rows || []).map(mapGoalRef)); })
      .catch(() => { if (alive) setItems([]); })
      .finally(() => { if (alive) setLoading(false); });
    return () => { alive = false; };
  }, [periodSel, q, excludeGoalId]);

  // Группировка по команде, затем упорядочивание по оргиерархии (DFS-порядок из allTeams).
  const byTeam = new Map();
  items.forEach(it => {
    let g = byTeam.get(it.teamId);
    if (!g) { g = { teamId: it.teamId, teamName: it.teamName, teamType: it.teamType, rows: [] }; byTeam.set(it.teamId, g); }
    g.rows.push(it);
  });
  const groups = [...byTeam.values()].sort((a, b) => {
    const ma = teamMeta.get(a.teamId), mb = teamMeta.get(b.teamId);
    if (ma && mb) return ma.order - mb.order;   // обе в иерархии → порядок сайдбара
    if (ma) return -1;                          // известные команды выше
    if (mb) return 1;
    return 0;                                   // обе вне иерархии → серверный порядок
  });

  const overlay = useOverlayClose(onClose);
  return (
    <div className="modal-overlay modal-overlay--z400" {...overlay}>
      <div className="modal-box goal-parent-picker">
        <div className="modal-header">
          <div className="goal-parent-picker__title">Выбрать родительскую цель</div>
          <button onClick={onClose} className="modal-close">×</button>
        </div>
        <div className="goal-parent-picker__filters">
          <PeriodSelect periods={pickerPeriods} periodId={periodSel} onChange={setPeriodSel} variant="light" />
          <input className="goal-parent-picker__search" placeholder="Поиск по названию или команде…"
            value={q} onChange={e => setQ(e.target.value)} autoFocus />
        </div>
        <div className="goal-parent-picker__list">
          {loading && <div className="goal-parent-picker__empty">Загрузка…</div>}
          {!loading && groups.length === 0 && <div className="goal-parent-picker__empty">Ничего не найдено</div>}
          {!loading && groups.map(g => {
            const depth = (teamMeta.get(g.teamId) || {}).depth || 0;
            return (
              <div key={g.teamId} className="goal-parent-picker__group">
                <div className="goal-parent-picker__group-head" style={{ paddingLeft: 8 + depth * 18 }}>
                  <span className="goal-parent-picker__group-dot" style={{ background: TEAM_TYPE_COLOR[g.teamType] || '#6b7280' }} />
                  {g.teamName}
                  <span className="goal-parent-picker__group-type">{(TEAM_TYPE_LABEL[g.teamType] || g.teamType || '').toUpperCase()}</span>
                </div>
                {g.rows.map(it => {
                  const isSel = selected.has(it.id);
                  return (
                    <button key={it.id} type="button" disabled={isSel}
                      className={`goal-parent-picker__row${isSel ? ' goal-parent-picker__row--selected' : ''}`}
                      style={{ paddingLeft: 26 + depth * 18 }}
                      onClick={() => { onPick(it); onClose(); }}>
                      <span className="goal-parent-picker__row-title">{it.title}</span>
                      <span className="goal-parent-picker__row-meta">{it.periodName} · {it.teamName} · {it.progress}%</span>
                      {isSel && <span className="goal-parent-picker__row-check">✓</span>}
                    </button>
                  );
                })}
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}

function GoalModal({ goal, teamId, periodId, teamName, periodName, existingGoals, me, onSave, onClose, accent, allTeams, periods }) {
  const isEdit = !!goal;
  const usedWeight = (existingGoals || []).filter(g => !isEdit || g.id !== goal?.id).reduce((s, g) => s + g.weight, 0);
  const wasShared = isEdit && (goal.shareTeams || []).filter(t => t.id !== teamId).length > 0;
  const [form, setForm] = useState(goal
    ? { shareTeamIds: (goal.shareTeams || []).filter(t => t.id !== teamId).map(t => t.id), ...goal, shared: wasShared, ownerUDIDs: (goal.owners || []).map(u => u.udid).filter(Boolean), parents: goal.parents || [] }
    : { title: '', desc: '', priority: 'P1', weight: Math.max(0, Math.min(20, 100 - usedWeight)), type: 'delivery', focus: 'PROFITABILITY', shared: false, shareTeamIds: [], ownerUDIDs: [], parents: [] });
  const [saving, setSaving] = useState(false);
  const [confirmUnshare, setConfirmUnshare] = useState(false);
  const [pickerOpen, setPickerOpen] = useState(false);
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
  // saveLinks commits the parent set (full replace). Maps cycle/access errors to friendly text.
  // parentsChanged compares the current parent set against the goal's original (as loaded).
  // The visible set is scope-filtered, so re-saving an unchanged set would be a redundant
  // full-replace; skipping it also avoids touching links on unrelated edits.
  const parentsChanged = () => {
    const a = (goal?.parents || []).map(p => p.id).sort((x, y) => x - y);
    const b = (form.parents || []).map(p => p.id).sort((x, y) => x - y);
    return a.length !== b.length || a.some((id, i) => id !== b[i]);
  };
  const saveLinks = async goalIdForLinks => {
    try {
      await apiPost(`/api/v1/goals/${goalIdForLinks}/links`, { parent_goal_ids: (form.parents || []).map(p => p.id) });
    } catch (e) {
      const m = String(e.message || '');
      if (m.includes('409')) throw new Error('Нельзя создать циклическую связь целей');
      if (m.includes('400')) throw new Error('Одна из выбранных родительских целей недоступна');
      throw e;
    }
  };
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
        if (parentsChanged()) await saveLinks(goal.id);
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
        if ((form.parents || []).length > 0) await saveLinks(newGoalId);
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
          {/* Название и вес — одной строкой: вес читается вместе с названием, а не
              отдельным блоком ниже. */}
          <div className="form-row goal-name-row">
            <div className="form-col">
              <FieldLabel required hint="Objective — качественное описание того, чего команда хочет достичь. Без цифр (они в KR).">Название</FieldLabel>
              <input value={form.title} onChange={e => set('title', e.target.value)} placeholder="Чего хотим достичь?" className="form-input form-input--goal" />
            </div>
            <div className="form-col--w140">
              <FieldLabel hint="Доля цели в общем результате команды. Сумма весов = 100%.">
                <span>Вес <span style={{ fontWeight: 400, color: overWeight ? '#d97706' : '#9ca3af', fontSize: 13 }}>({totalAfter}/100)</span></span>
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
          <div className="form-group">
            <FieldLabel hint="Контекст, почему эта цель важна. Не дублируйте название.">Описание</FieldLabel>
            <MarkdownEditor value={form.desc} onChange={v => set('desc', v)} rows={3} placeholder="Дополнительный контекст…" textareaClassName="form-textarea" />
          </div>
          {/* Приоритет, тип работы и фокус — одной строкой. */}
          <div className="form-row goal-meta-row">
            <div className="form-col">
              <FieldLabel hint={PRIORITY_HINT}>Приоритет</FieldLabel>
              <PrioritySelect value={form.priority} onChange={p => set('priority', p)} />
            </div>
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
              <FieldLabel hint={FOCUS_HINT_BLOCK}>Фокус</FieldLabel>
              <select value={form.focus} onChange={e => set('focus', e.target.value)} className="form-select">
                {FOCUS_OPTIONS.map(f => <option key={f} value={f}>{focusLabel(f)}</option>)}
              </select>
            </div>
          </div>
          <div className="form-group">
            <FieldLabel hint={'Ответственный за достижение цели.\nСледит за прогрессом KR, обновляет статус и эскалирует блокеры.\nМожно указать несколько человек.'}>Драйвер цели</FieldLabel>
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
          <div className="goal-links-section">
            <div className="goal-links-section__head">
              🔗 Связанные цели
              <InfoHint width={320}>Связь привязывает эту цель к верхнеуровневой (цель руководителя, годовая цель, цель юнита/кластера). Это <b>не</b> делает цель общей — общие цели (⇄) видны нескольким командам, а связь соединяет две <b>разные</b> цели отношением «дочерняя → родительская».</InfoHint>
            </div>
            <div className="goal-links-section__hint">К какой верхнеуровневой цели относится эта цель. Можно указать несколько.</div>
            {(form.parents || []).map(p => (
              <div key={p.id} className="goal-link-card">
                <span className="goal-link-card__arrow">↑</span>
                <span className="goal-link-card__title">{p.title}</span>
                <span className="goal-link-card__meta">{p.periodName} · {p.teamName} · {p.progress}%</span>
                <button type="button" className="goal-link-card__remove" title="Убрать связь"
                  onClick={() => set('parents', (form.parents || []).filter(x => x.id !== p.id))}>✕</button>
              </div>
            ))}
            <button type="button" className="goal-link-add" onClick={() => setPickerOpen(true)}>+ Добавить связь</button>
          </div>
        </div>
        <div className="modal-footer modal-footer--goal">
          <button onClick={onClose} className="btn btn--secondary" style={{ padding: '10px 20px', fontSize: 15 }}>Отмена</button>
          <button onClick={save} disabled={!canSave} className="btn btn--primary"
            style={{ padding: '10px 28px', fontSize: 15, background: canSave ? accent : '#e5e7eb', color: canSave ? 'white' : '#9ca3af', cursor: canSave ? 'pointer' : 'default' }}>
            {saving ? 'Сохраняем…' : isEdit ? 'Сохранить' : 'Создать цель'}
          </button>
        </div>
      </div>
      {pickerOpen && (
        <GoalParentPicker
          excludeGoalId={goal?.id}
          currentPeriodId={periodId}
          periods={periods || []}
          selectedIds={(form.parents || []).map(p => p.id)}
          allTeams={allTeams}
          onPick={g => { if (!(form.parents || []).some(x => x.id === g.id)) set('parents', [...(form.parents || []), g]); }}
          onClose={() => setPickerOpen(false)} />
      )}
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
// TRK_PERIOD_STATUS / fmtPeriodDate / fmtDateRange / PeriodSelect — общие символы
// из period_select.js (грузится раньше tracker.js).

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

// ── APP ───────────────────────────────────────────────────────────────────────
function App() {
  const [me, setMe] = useState(null);
  const [loading, setLoading] = useState(true);
  const [periods, setPeriods] = useState([]);
  const [periodId, setPeriodId] = useState(null);
  const [hierarchy, setHierarchy] = useState([]);
  const [selId, setSelId] = useState(null);
  const [teamOKR, setTeamOKR] = useState(null);
  const [expanded, setExpanded] = useState(readTreeExpanded);
  const [favorites, setFavorites] = useState(null); // null = not loaded from storage yet
  const [goalModal, setGoalModal] = useState(null);
  const [accent] = useState(ACCENT);
  const [docUrl, setDocUrl] = useState('');
  const [emptyHierMsg, setEmptyHierMsg] = useState('');
  const [staleDays, setStaleDays] = useState(7);
  const [behindMargin, setBehindMargin] = useState(10);
  const [greenThreshold, setGreenThreshold] = useState(80);
  const [isAdmin, setIsAdmin] = useState(false);

  // Read desired initial team+period once from URL (highest prio) then cookie.
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
      if (cfg) { setDocUrl(cfg.documentation_url || ''); if (cfg.stale_days > 0) setStaleDays(cfg.stale_days); if (typeof cfg.behind_margin === 'number') setBehindMargin(cfg.behind_margin); if (cfg.green_threshold >= 1 && cfg.green_threshold <= 100) setGreenThreshold(cfg.green_threshold); setEmptyHierMsg(cfg.empty_hierarchy_message || ''); setIsAdmin(!!cfg.is_admin); }
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
    });
  }, [periodId]);

  useEffect(() => {
    if (!periodId || !selId) return;
    setTeamOKR(null);
    apiGet(`/api/v1/teams/${selId}/okrs?period_id=${periodId}`).then(data => { if (data) { _cacheUserRefsFromOKR(data); setTeamOKR(data); } }).catch(() => setTeamOKR(null));
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
    });
  }, [periodId, selId]);

  const handleChangeStatus = async newStatus => {
    try { await apiPost(`/api/v1/teams/${selId}/status`, { period_id: periodId, status: newStatus }); reload(); }
    catch (e) { alert('Ошибка: ' + e.message); }
  };

  const [dragState, setDragState] = useState({ srcId: null });
  // Настройки показа целей: читаются из localStorage один раз и пишутся обратно при
  // каждом изменении. Ключ не зависит от команды и периода, поэтому переключение
  // команды и периода их не сбрасывает.
  const [boardView, setBoardView] = useState(readBoardView);
  useEffect(() => { writeBoardView(boardView); }, [boardView]);
  // Цель, открытая по прямой ссылке, не прячется сохранёнными настройками показа.
  // Закрепление снимается при первом же изменении настроек пользователем: дальше решает
  // он, а не ссылка. deepLinkRef для этого не годится — он обнуляется после прокрутки, и
  // цель исчезла бы из-под курсора.
  const [pinnedGoalId, setPinnedGoalId] = useState(() => readURLNav().goal || null);
  // Закрепление снимается только при фактическом изменении настроек — см. boardViewEquals.
  const changeBoardView = useCallback(v => {
    if (boardViewEquals(boardView, v)) return;
    setPinnedGoalId(null);
    setBoardView(v);
  }, [boardView]);
  const [exportOpen, setExportOpen] = useState(false);
  // Экспорт открывается из двух мест: меню команды (охват «цели команды») и меню
  // цели (охват «одна цель»). Окно живёт здесь, потому что здесь лежит exportInfo.
  const [exportGoal, setExportGoal] = useState(null);
  const allGoals = (teamOKR?.goals || []).map(mapGoal);
  // Настройки показа сужают и переупорядочивают только показ, данные остаются прежними:
  // всё, что считается ниже (прогресс, сумма весов, охват выгрузки, занятый вес в окне
  // цели), считается по allGoals.
  const goals = visibleGoals(allGoals, boardView, pinnedGoalId);

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
  const goalCreateLock = lockReason(editMode, 'goal_create');

  // A team's goal weights are expected to sum to 100%. When they don't, surface a
  // warning so the author can redistribute weight or add a goal.
  // Считается по всем целям периода: фильтр по приоритету сужает показ, а не состав,
  // иначе при выбранном фильтре появлялось бы ложное «сумма весов ≠ 100%».
  const goalWeightSum = allGoals.reduce((s, g) => s + (g.weight || 0), 0);
  const goalWeightOff = allGoals.length > 0 && goalWeightSum !== 100;
  const goalWeightDelta = 100 - goalWeightSum;
  // Context passed to the per-goal export menu: period label, team hierarchy path and the
  // scope-card counts (subtree team count comes from the already-loaded hierarchy).
  const exportInfo = selId ? {
    periodName: curPeriod?.name || '',
    teamPath: treePathNames(hierarchy, selId) || (teamOKR?.team?.name ? [teamOKR.team.name] : []),
    teamGoalCount: allGoals.length,
    subtreeTeamCount: countSubtree(findTreeNode(hierarchy, selId)),
  } : null;
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
        linkParams={periodLinkParams(periodId)}
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
              <div className="tb-lead">
                <UserInfo userRef={teamOKR.team.lead} size={22} />
                <span className="tb-lead__role">· лид</span>
              </div>
            )}
            <div className="topbar__spacer" />
            {hasGoals && teamOKR?.progress_meta && (
              <div className="topbar__progress">
                <span className="tb-cap">Цели узла</span>
                <div className="topbar__progress-bar">
                  <ProgressBar value={teamOKR.period_progress || 0} forecast={teamOKR.progress_meta.forecast} h={6}
                    color={HEALTH_COLOR[(teamOKR.period_progress || 0) >= greenThreshold ? 'ahead' : teamOKR.progress_meta.status === 'above' ? 'ahead' : teamOKR.progress_meta.status === 'below' ? 'below' : 'on_track']} />
                </div>
                <span className="topbar__progress-pct">{teamOKR.period_progress || 0}%</span>
              </div>
            )}
            {selId && (goalCreateLock
              ? <LockedAction reason={goalCreateLock}>
                  <button type="button" aria-disabled="true" className="topbar__add-btn topbar__add-btn--locked">+ Добавить цель 🔒</button>
                </LockedAction>
              : <button onClick={() => setGoalModal('new')} className="topbar__add-btn">+ Добавить цель</button>)}
            {/* Экспорт переехал из меню цели в меню команды: выгрузка всё равно умеет
                расширять охват до команды, а по цели она якорится первой из списка. */}
            <div className="tb-menu">
              <RowMenu btnClass="tb-menu__btn" dropdownClass="tb-menu__dropdown" items={[
                { icon: '⇩', label: 'Экспорт в Markdown', onClick: () => setExportOpen(true),
                  reason: allGoals.length ? null : 'В этом периоде у команды нет целей — выгружать нечего.' },
              ]} />
            </div>
          </div>
          {(() => {
            const ov = readDescOverrides(me?.id);
            const desc = teamOKR?.team && ov[teamOKR.team.id] !== undefined ? ov[teamOKR.team.id] : teamOKR?.team?.description;
            return desc ? <Markdown text={desc} className="topbar__desc" /> : null;
          })()}
        </div>

        <StatusStepper status={status} hasGoals={hasGoals} onChange={handleChangeStatus} accent={accent}
          statusChangedAt={teamOKR?.status_changed_at} editMode={editMode} />

        <div className="content">
          {allGoals.length === 0 && hierarchy.length === 0 && !loading && (
            <div className="empty-state">
              <div className="empty-state__icon">🔒</div>
              <div className="empty-state__title">Нет доступа</div>
              <div className="empty-state__text">За доступом обратитесь к администратору</div>
            </div>
          )}
          {allGoals.length === 0 && hierarchy.length > 0 && (
            <div className="empty-state">
              <div className="empty-state__icon">📋</div>
              <div className="empty-state__title">Цели не добавлены</div>
              <div className="empty-state__text">Начните период с постановки OKR</div>
              {!goalCreateLock && selId && <button onClick={() => setGoalModal('new')} className="empty-state__btn">+ Создать первую цель</button>}
            </div>
          )}
          {goalWeightWarn}
          <BoardFilterBar goals={allGoals} value={boardView} onChange={changeBoardView} />
          {/* Цели у команды есть, но под настройки показа не подошла ни одна. Своё
              состояние, а не пустая страница: иначе доска выглядит как команда без
              целей — и предлагать «создать первую цель» здесь нечего. */}
          {allGoals.length > 0 && goals.length === 0 && (
            <div className="empty-state">
              <div className="empty-state__icon">🔍</div>
              <div className="empty-state__title">Под настройки показа не подходит ни одна цель</div>
              <div className="empty-state__text">Цели у команды есть, но все они скрыты настройками показа</div>
              <button type="button" className="empty-state__btn" onClick={() => changeBoardView(boardViewDefault())}>Сбросить настройки</button>
            </div>
          )}
          {goals.map(g => <GoalCard key={g.id} goal={g} editMode={editMode} onReload={reload} onEditGoal={setGoalModal} onExportGoal={setExportGoal} me={me} isAdmin={isAdmin} accent={accent} currentTeamId={selId} periodId={periodId} allTeams={hierarchy} staleDays={staleDays} periodStatus={status} greenThreshold={greenThreshold} deepLink={deepLinkRef.current} notesOpen={boardView.notes}
            /* Перетаскивание меняет пользовательский порядок, а бросок делается по
               видимому: при сортировке по приоритету это разные порядки, и результат
               был бы непредсказуем. Без dragProps GoalCard не рисует ручку. */
            dragProps={!lockReason(editMode, 'reorder') && boardView.sort === 'custom' ? {
              isDragging: dragState.srcId === g.id,
              onDragStart: (e) => { e.dataTransfer.effectAllowed = 'move'; setDragState({ srcId: g.id }); },
              onDragOver: (e) => { if (dragState.srcId && dragState.srcId !== g.id) { e.preventDefault(); e.dataTransfer.dropEffect = 'move'; } },
              onDrop: (e) => { e.preventDefault(); handleReorderGoals(dragState.srcId, g.id); setDragState({ srcId: null }); },
              onDragEnd: () => setDragState({ srcId: null }),
            } : null}
            onReorderKR={!lockReason(editMode, 'reorder') ? (fromId, toId) => handleReorderKRs(g.id, fromId, toId) : null}
          />)}
        </div>
      </div>

      {exportOpen && allGoals.length > 0 && (
        <ExportModal goal={allGoals[0]} teamId={selId} periodId={periodId} info={exportInfo}
          initialScope="team" onClose={() => setExportOpen(false)} />
      )}
      {exportGoal && (
        <ExportModal goal={exportGoal} teamId={selId} periodId={periodId} info={exportInfo}
          initialScope="goal" onClose={() => setExportGoal(null)} />
      )}
      {goalModal && <GoalModal
        goal={goalModal === 'new' ? null : goalModal}
        teamId={selId} periodId={periodId}
        teamName={teamOKR?.team?.name || ''} periodName={curPeriod?.name || ''}
        existingGoals={allGoals} me={me}
        onSave={() => { setGoalModal(null); reload(); }}
        onClose={() => setGoalModal(null)}
        accent={accent} allTeams={hierarchy} periods={periods} />}
    </div>
  );
}

ReactDOM.createRoot(document.getElementById('root')).render(<App />);
