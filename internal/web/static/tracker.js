const { useState, useCallback, useRef, useEffect } = React;
const ACCENT = '#7c3aed';

// ── API ───────────────────────────────────────────────────────────────────────
function readCSRF() {
  const part = document.cookie.split(';').map(s => s.trim()).find(s => s.startsWith('okr_csrf_token='));
  return part ? decodeURIComponent(part.split('=').slice(1).join('=')) : '';
}
function csrfHeaders(extra = {}) { return { 'X-CSRF-Token': readCSRF(), 'Content-Type': 'application/json', ...extra }; }
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
  const team = p.get('team') ? Number(p.get('team')) : null;
  const period = p.get('period') ? Number(p.get('period')) : null;
  return { team: Number.isFinite(team) ? team : null, period: Number.isFinite(period) ? period : null };
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
  const url = '/' + (qs ? '?' + qs : '');
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
  try { localStorage.setItem(TREE_EXPANDED_KEY, JSON.stringify(expanded)); } catch {}
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
  let unit = '%', checkpoints = [], zeroing = '';
  if (m.numerical) {
    start = m.numerical.start_value; target = m.numerical.target_value; current = m.numerical.current_value;
    unit = m.numerical.unit || '%';
    checkpoints = (m.numerical.checkpoints || []).map(c => ({ value: c.value, progress_percent: c.progress_percent }));
    zeroing = m.numerical.zeroing_criteria || '';
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
    comments: (g.comments || []).map(c => ({ id: c.id, author: c.author_name, authorUdid: c.author_udid, date: fmtDate(c.created_at), text: c.text })),
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
const FOCUS_OPTIONS = ['PROFITABILITY', 'STABILITY', 'SPEED_EFFICIENCY', 'TECH_INDEPENDENCE'];
const STATUS_STEPS = [{ k: 'forming', l: 'Черновик' }, { k: 'ready', l: 'К валидации' }, { k: 'in_progress', l: 'В работе' }, { k: 'closed', l: 'Закрыты' }];
const TEAM_TYPE_LABEL = { cluster: 'Кластер', unit: 'Юнит', group: 'Группа', team: 'Команда', squad: 'Сквад' };
const TEAM_TYPE_COLOR = { cluster: '#7c3aed', unit: '#2563eb', group: '#0891b2', team: '#059669', squad: '#d97706' };

function healthOf(p, stale, forecast) {
  if (p === null || p === undefined) return 'no_goals';
  if (stale) return 'stale';
  if (forecast == null) return 'on_track';
  const delta = forecast - p;
  if (delta < -10) return 'ahead';
  if (delta > 10) return 'below';
  return 'on_track';
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

// ── HEADER USER MENU ──────────────────────────────────────────────────────────
function HeaderUserMenu({ user, docUrl }) {
  const [open, setOpen] = useState(false);
  const timer = useRef();
  const show = () => { clearTimeout(timer.current); setOpen(true); };
  const hide = () => { clearTimeout(timer.current); timer.current = setTimeout(() => setOpen(false), 150); };
  const logout = () => { fetch('/logout', { method: 'POST', credentials: 'include', headers: { 'X-CSRF-Token': readCSRF() } }).then(() => location.href = '/login'); };
  const name = user?.display_name || 'Пользователь';
  return (
    <div className="user-menu" onMouseEnter={show} onMouseLeave={hide}>
      <button onClick={() => setOpen(o => !o)} className={`user-menu__trigger${open ? ' user-menu__trigger--open' : ''}`}>
        <Avatar name={name} avatarUrl={user?.avatar_url} size={28} />
        <span className={`user-menu__chevron${open ? ' user-menu__chevron--open' : ''}`}>▾</span>
      </button>
      {open && (
        <div onMouseEnter={show} onMouseLeave={hide} className="user-menu__dropdown">
          <div className="user-menu__profile">
            <Avatar name={name} avatarUrl={user?.avatar_url} size={36} />
            <div>
              <div className="user-menu__name">{name}</div>
              <div className="user-menu__email">{user?.email || user?.provider || ''}</div>
            </div>
          </div>
          {docUrl && (
            <a href={docUrl} target="_blank" rel="noopener noreferrer" className="user-menu__item">
              <span className="user-menu__item-icon">📖</span>Документация
            </a>
          )}
          {user?.is_admin && (
            <a href="/admin" className="user-menu__item">
              <span className="user-menu__item-icon">⚙</span>Управление
            </a>
          )}
          <div className="user-menu__divider">
            <button onClick={logout} className="user-menu__item user-menu__item--danger">
              <span className="user-menu__item-icon">↩</span><span>Выйти</span>
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

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
    else if (e.key === 'Escape') setOpen(false);
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
  const set = (k, v) => setForm(f => ({ ...f, [k]: v }));
  const setStage = (i, k, v) => setForm(f => { const ss = [...f.stages]; ss[i] = { ...ss[i], [k]: v }; return { ...f, stages: ss }; });
  const progress = calcKRProgress(form);
  const overlay = useOverlayClose(onClose);
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
      onSave();
    } catch (e) { alert('Ошибка сохранения: ' + e.message); }
    finally { setSaving(false); }
  };
  return (
    <div className="modal-overlay modal-overlay--z300" {...overlay}>
      <div onClick={e => e.stopPropagation()} className="modal-box modal-box--w480">
        <div className="modal-header">
          <div>
            <div className="modal-title">Обновить прогресс</div>
            <div className="modal-subtitle">{kr.name}</div>
          </div>
          <button onClick={onClose} className="modal-close">×</button>
        </div>
        {kr.desc && (
          <div className="kr-progress-desc">
            <div className="kr-progress-desc__label">Описание</div>
            <div className="kr-progress-desc__text">{kr.desc}</div>
          </div>
        )}
        <div className="modal-body">
          {form.krType === 'NUMERICAL' && (
            <div className="kr-progress-field">
              <div className="kr-progress-field__label">Текущее значение <span className="kr-progress-field__hint">({fmtVal(form.start, form.unit)} → {fmtVal(form.target, form.unit)})</span></div>
              <input type="number" value={form.current} onChange={e => set('current', e.target.value)} className="form-input" />
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
              <div style={{ marginTop: 10 }}>
                <div className="kr-progress-row">
                  <span className="kr-progress-row__label">Прогресс</span>
                  <span style={{ fontSize: 13, fontWeight: 700, color: accent }}>{progress}%</span>
                </div>
                <ProgressBar value={progress} h={6} color={accent} />
                {form.zeroing && <div className="kr-progress-row" style={{ marginTop: 6 }}><span className="kr-progress-row__label">Критерий обнуления: {form.zeroing}</span></div>}
              </div>
            </div>
          )}
          {form.krType === 'BOOLEAN' && (
            <label className="kr-boolean-label">
              <input type="checkbox" checked={!!form.done} onChange={e => set('done', e.target.checked)} style={{ width: 18, height: 18, accentColor: accent }} />
              <span className="kr-boolean-text">Выполнено</span>
              <span className="kr-boolean-pct" style={{ color: form.done ? '#16a34a' : '#9ca3af' }}>{form.done ? '100%' : '0%'}</span>
            </label>
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
            <textarea value={note} onChange={e => setNote(e.target.value)} rows={3} placeholder="Контекст, блокеры…"
              className="form-textarea form-textarea--sm" style={{ resize: 'vertical' }} />
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
      if (form.krType === 'NUMERICAL') {
        fd.append('numerical_unit', form.unit || '%');
        fd.append('numerical_start', String(form.start || 0));
        fd.append('numerical_target', String(form.target || 0));
        fd.append('numerical_current', String(form.current || 0));
        fd.append('numerical_zeroing', form.zeroing || '');
        (form.checkpoints || []).forEach(c => {
          if (c.value === '' || c.value === null || c.value === undefined) return;
          fd.append('checkpoint_value[]', String(c.value));
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
  const overlay = useOverlayClose(onClose);
  return (
    <div className="modal-overlay modal-overlay--z300" {...overlay}>
      <div onClick={e => e.stopPropagation()} className="modal-box modal-box--w560">
        <div className="modal-header modal-header--sticky">
          <div className="modal-title modal-title--lg">{isNew ? 'Добавить KR' : 'Редактировать KR'}</div>
          <button onClick={onClose} className="modal-close">×</button>
        </div>
        <div className="modal-body">
          <div className="form-group--sm">
            <div className="kr-num-field__label">Название</div>
            <input value={form.name} onChange={e => set('name', e.target.value)} placeholder="Что измеряет этот KR?" className="form-input" />
          </div>
          <div className="form-group--sm">
            <div className="kr-num-field__label">Описание</div>
            <textarea value={form.desc || ''} onChange={e => set('desc', e.target.value)} rows={2}
              className="form-textarea form-textarea--sm" style={{ resize: 'vertical' }} />
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
                    <input type="number" value={form.start} onChange={e => set('start', e.target.value)} className="form-input form-input--sm" />
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
                    <input type="number" value={form.target} onChange={e => set('target', e.target.value)} className="form-input form-input--sm" />
                    <span className="kr-num-input-suffix__unit">{form.unit}</span>
                  </div>
                </div>
                <div className="form-col">
                  <div className="kr-num-field__label">Текущее значение</div>
                  <div className="kr-num-input-suffix">
                    <input type="number" value={form.current} onChange={e => set('current', e.target.value)} className="form-input form-input--sm" />
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
                    <input type="number" placeholder="напр. 150" value={c.value} onChange={e => setCp(i, 'value', e.target.value)} className="form-input form-input--sm" />
                    <input type="number" placeholder="0–100" min={0} max={100} value={c.progress_percent} onChange={e => setCp(i, 'progress_percent', e.target.value)} className="form-input form-input--sm" />
                    <button onClick={() => remCp(i)} className="kr-step-delete">×</button>
                  </div>
                ))}
                <button type="button" onClick={addCp} className="kr-dashed-btn">+ Добавить промежуточное значение</button>
              </div>
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
  );
}

// ── CONFIRM MODAL ─────────────────────────────────────────────────────────────
function ConfirmModal({ title, message, confirmLabel, onConfirm, onClose }) {
  const [busy, setBusy] = React.useState(false);
  const run = async () => { setBusy(true); try { await onConfirm(); } finally { setBusy(false); } };
  const overlay = useOverlayClose(onClose);
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
function KRRow({ kr, goalId, editMode, onReload, accent }) {
  const [modal, setModal] = useState(null);
  const [showNote, setShowNote] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const progress = kr.progress;
  const staleC = kr.updatedDaysAgo > 7 ? '#dc2626' : kr.updatedDaysAgo > 4 ? '#d97706' : '#10b981';
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
            {kr.desc && <div className="kr-desc">{kr.desc}</div>}
            <div className="kr-detail-row">
              <div className="kr-bar-wrap"><ProgressBar value={progress} h={4} color={accent} /></div>
              <span className="kr-pct" style={{ color: accent }}>{progress}%</span>
              {detail}
            </div>
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
                <div className="kr-note__text" style={{ whiteSpace: 'pre-wrap' }}>{kr.note.text}</div>
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
function CommentsPanel({ comments, onAdd, me }) {
  const [text, setText] = useState(''); const [saving, setSaving] = useState(false);
  const submit = async () => {
    if (!text.trim()) return;
    setSaving(true); try { await onAdd(text.trim()); setText(''); } catch { } finally { setSaving(false); }
  };
  const hasText = !!text.trim();
  return (
    <div className="comments-panel">
      <div className="comments-panel__title">Комментарии</div>
      {(comments || []).map((c, i) => (
        <div key={c.id || i} className="comment">
          <AvatarWithUDID name={c.author} udid={c.authorUdid} size={28} />
          <div className="comment__content">
            <div className="comment__header">
              <span className="comment__author">{c.author}</span>
              <span className="comment__date">{c.date}</span>
            </div>
            <div className="comment__text">{c.text}</div>
          </div>
        </div>
      ))}
      <div className="comment-compose">
        <Avatar name={me?.display_name} avatarUrl={me?.avatar_url} size={28} />
        <div className="comment-compose__right">
          <textarea value={text} onChange={e => setText(e.target.value)} placeholder="Контекст, блокер, заметка… (Cmd+Enter)"
            onKeyDown={e => { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) submit(); }}
            className="form-textarea form-textarea--sm" style={{ width: '100%' }} />
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
function GoalCard({ goal, editMode, onReload, onEditGoal, me, accent, currentTeamId, allTeams, dragProps, onReorderKR }) {
  const [showKR, setShowKR] = useState(false);
  const [showCom, setShowCom] = useState(false);
  const [newKR, setNewKR] = useState(false);
  const [krDrag, setKrDrag] = useState(null);
  const [goalDraggable, setGoalDraggable] = useState(false);
  const [confirmDeleteGoal, setConfirmDeleteGoal] = useState(false);
  const prog = goal.progress || 0;
  const isStale = goal.updatedDaysAgo > 7;
  const forecast = goal.progressMeta?.forecast ?? null;
  const hC = HEALTH_COLOR[healthOf(prog, isStale, forecast)];
  const health = healthOf(prog, isStale, forecast);
  const canEdit = editMode === 'full';
  const canReorderGoal = canEdit && !!dragProps;
  const { isDragging, ...rootDrag } = dragProps || {};
  const otherTeams = (goal.shareTeams || []).filter(t => t.id !== currentTeamId);
  const krWeightSum = (goal.krs || []).reduce((s, k) => s + (k.weight || 0), 0);
  const krWeightOff = krWeightSum !== 100;
  const krWeightDelta = 100 - krWeightSum;

  const addGoalComment = async text => { await apiPost(`/api/v1/goals/${goal.id}/comments`, { text }); onReload(); };
  const handleDeleteGoal = async () => { await apiDelete(`/api/v1/goals/${goal.id}`); onReload(); };

  const cardClass = ['goal-card',
    isDragging ? 'goal-card--dragging' : '',
    canReorderGoal ? 'goal-card--reorderable' : '',
    otherTeams.length > 0 ? 'goal-card--shared' : '',
    isStale ? 'goal-card--stale' : '',
  ].filter(Boolean).join(' ');

  return (
    <div {...rootDrag} draggable={!!(canReorderGoal && goalDraggable)}
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
              <span className="goal-card__owner-label">Владелец</span>
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
        {goal.desc && <div className="goal-card__desc">{goal.desc}</div>}
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
            <span className="goal-card__updated" style={{ color: goal.updatedDaysAgo < 14 ? '#16a34a' : '#dc2626' }}>
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
          {goal.focus && <Badge label={goal.focus} color={FOCUS_COLORS[goal.focus] || FOCUS_COLORS.DEFAULT} />}
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
              <div key={kr.id}
                draggable={!!canReorderKR}
                onDragStart={canReorderKR ? (e) => { e.stopPropagation(); e.dataTransfer.effectAllowed = 'move'; e.dataTransfer.setData('text/plain', 'kr'); setKrDrag(kr.id); } : undefined}
                onDragOver={canReorderKR ? (e) => { if (krDrag && krDrag !== kr.id) { e.preventDefault(); e.stopPropagation(); e.dataTransfer.dropEffect = 'move'; } } : undefined}
                onDrop={canReorderKR ? (e) => { e.preventDefault(); e.stopPropagation(); if (krDrag && krDrag !== kr.id) onReorderKR(krDrag, kr.id); setKrDrag(null); } : undefined}
                onDragEnd={canReorderKR ? () => setKrDrag(null) : undefined}
                className={`kr-item${isKrDrag ? ' kr-item--dragging' : ''}${canReorderKR ? ' kr-item--reorderable' : ''}`}>
                {canReorderKR && <div className="kr-item__drag-handle">⋮⋮</div>}
                <KRRow kr={kr} goalId={goal.id} editMode={editMode} onReload={onReload} accent={accent} />
              </div>
            );
          })}
          {editMode === 'full' && <button onClick={() => setNewKR(true)} className="kr-add-btn">+ Добавить KR</button>}
        </div>
      )}
      {newKR && <KREditModal kr={null} goalId={goal.id} onSave={() => { setNewKR(false); onReload(); }} onClose={() => setNewKR(false)} accent={accent} />}
      {confirmDeleteGoal && <ConfirmModal title="Удалить цель?" message={`«${goal.title}» и все её Key Results будут удалены без возможности восстановления.`}
        onConfirm={handleDeleteGoal} onClose={() => setConfirmDeleteGoal(false)} />}
      {showCom && (
        <div className="comments-section">
          <CommentsPanel comments={goal.comments} onAdd={addGoalComment} me={me} />
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
      }).catch(() => {});
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
    else if (e.key === 'Escape') setOpen(false);
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
  const initAv   = userRef?.avatar_url || null;

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

function GoalModal({ goal, teamId, periodId, teamName, periodName, existingGoals, me, onSave, onClose, accent, allTeams }) {
  const isEdit = !!goal;
  const usedWeight = (existingGoals || []).filter(g => !isEdit || g.id !== goal?.id).reduce((s, g) => s + g.weight, 0);
  const wasShared = isEdit && (goal.shareTeams || []).filter(t => t.id !== teamId).length > 0;
  const [form, setForm] = useState(goal
    ? { shareTeamIds: (goal.shareTeams || []).filter(t => t.id !== teamId).map(t => t.id), ...goal, shared: wasShared, ownerUDIDs: (goal.owners || []).map(u => u.udid).filter(Boolean) }
    : { title: '', desc: '', priority: 'P1', weight: Math.min(20, 100 - usedWeight), type: 'delivery', focus: 'PROFITABILITY', shared: false, shareTeamIds: [], ownerUDIDs: [] });
  const [saving, setSaving] = useState(false);
  const set = (k, v) => setForm(f => ({ ...f, [k]: v }));
  const totalAfter = usedWeight + (isEdit ? form.weight - (goal?.weight || 0) : form.weight);
  const overWeight = totalAfter > 100;
  const valid = form.title.trim() && !overWeight && form.weight > 0 && (!form.shared || (form.shareTeamIds || []).length > 0);
  const save = async () => {
    if (!valid || saving) return;
    setSaving(true);
    try {
      if (isEdit) {
        if (wasShared && !form.shared) { await apiDelete(`/api/v1/goals/${goal.id}/share/${teamId}`); onSave(); return; }
        const fd = new FormData();
        fd.append('title', form.title.trim()); fd.append('description', form.desc || '');
        fd.append('priority', form.priority); fd.append('weight', String(form.weight));
        fd.append('work_type', form.type === 'delivery' ? 'Delivery' : 'Discovery');
        fd.append('focus_type', form.focus);
        (form.ownerUDIDs || []).forEach(u => fd.append('owner_udids', u));
        fd.append('team_id', String(teamId));
        await apiForm(`/api/v1/goals/${goal.id}`, fd);
        if (form.shared && (form.shareTeamIds || []).length > 0) {
          await apiPost(`/api/v1/goals/${goal.id}/share`, { targets: (form.shareTeamIds || []).map(id => ({ team_id: id, weight: 100 })) });
        }
      } else {
        await apiPost(`/api/v1/teams/${teamId}/goals`, {
          period_id: periodId, title: form.title.trim(), description: form.desc || '',
          priority: form.priority, weight: form.weight,
          work_type: form.type === 'delivery' ? 'Delivery' : 'Discovery',
          focus_type: form.focus, owner_udids: form.ownerUDIDs || [],
        });
      }
      onSave();
    } catch (e) { alert('Ошибка: ' + e.message); }
    finally { setSaving(false); }
  };
  const canSave = valid && !saving;
  const overlay = useOverlayClose(onClose);
  return (
    <div className="modal-overlay modal-overlay--z400" {...overlay}>
      <div onClick={e => e.stopPropagation()} className="modal-box modal-box--w600">
        <div className="modal-header modal-header--sticky modal-header--goal">
          <div>
            <div className="modal-title--goal">{isEdit ? 'Редактировать цель' : 'Новая цель'}</div>
            <div className="modal-subtitle">{periodName} · {teamName}</div>
          </div>
          <button onClick={onClose} className="modal-close modal-close--lg">×</button>
        </div>
        <div className="modal-body modal-body--goal">
          <div className="form-group">
            <FieldLabel required hint="Objective — качественное описание того, чего команда хочет достичь. Без цифр (они в KR).">Название</FieldLabel>
            <input value={form.title} onChange={e => set('title', e.target.value)} placeholder="Чего хотим достичь?" className="form-input form-input--goal" />
          </div>
          <div className="form-group">
            <FieldLabel hint="Контекст, почему эта цель важна. Не дублируйте название.">Описание</FieldLabel>
            <textarea value={form.desc || ''} onChange={e => set('desc', e.target.value)} rows={3} placeholder="Дополнительный контекст…" className="form-textarea" />
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
                <span>Вес <span style={{ fontWeight: 400, color: overWeight ? '#ef4444' : '#9ca3af', fontSize: 12 }}>({totalAfter}/100)</span></span>
              </FieldLabel>
              <div className="form-weight-wrap">
                <input type="number" min={1} max={100} value={form.weight}
                  onChange={e => set('weight', Math.max(1, Math.min(100, Number(e.target.value))))}
                  className={`form-input form-input--weight${overWeight ? ' form-input--error' : ''}`} />
                <span className="form-weight-pct">%</span>
              </div>
              {overWeight && <div className="form-error-msg">Превышает 100%</div>}
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
                {FOCUS_OPTIONS.map(f => <option key={f} value={f}>{f}</option>)}
              </select>
            </div>
          </div>
          <div className="form-group">
            <FieldLabel>Владелец</FieldLabel>
            <UserSelector multiple
              value={form.ownerUDIDs}
              onChange={arr => set('ownerUDIDs', arr)}
              fetchFn={q => apiGet(`/api/v1/users?q=${encodeURIComponent(q)}&scope_team_id=${teamId}`)}
              placeholder="Добавить владельца" />
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
    </div>
  );
}

// ── SIDEBAR NODE ──────────────────────────────────────────────────────────────
function SidebarNode({ node, depth, selectedId, onSelect, expanded, toggle, accent }) {
  const ch = node.children || [];
  const isExp = expanded[node.id] !== false;
  const isSel = selectedId === node.id;
  const prog = node.progress;
  const hC = HEALTH_COLOR[healthOf(prog, false)];
  const pad = 14 + depth * 13;
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
        <span className="sidebar-node__dot" style={{ background: hC }} />
        <span className={nameClass}>{node.name}</span>
        {prog != null && <span className="sidebar-node__progress" style={{ color: isSel ? '#c4b5fd' : hC }}>{prog}%</span>}
      </div>
      {isExp && ch.map(c => <SidebarNode key={c.id} node={c} depth={depth + 1} selectedId={selectedId} onSelect={onSelect} expanded={expanded} toggle={toggle} accent={accent} />)}
    </div>
  );
}

// ── CHILD CARD ────────────────────────────────────────────────────────────────
function ChildCard({ item, onSelect }) {
  const prog = item.progress_meta ? item.progress_meta.actual : null;
  const forecast = item.progress_meta ? item.progress_meta.forecast : null;
  const health = healthOf(prog, false, forecast);
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
function ClusterView({ overview, onSelect }) {
  if (!overview) return <div className="cluster-loading">Загрузка…</div>;
  const avg = overview.average_progress || 0;
  const avgForecast = overview.progress_meta?.forecast ?? null;
  const hC = HEALTH_COLOR[healthOf(avg, false, avgForecast)];
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
            <span className="cluster-overview__health-label" style={{ color: hC }}>{HEALTH_LABEL[healthOf(avg, false, avgForecast)]}</span>
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
        {items.map(item => <ChildCard key={item.team.id} item={item} onSelect={onSelect} />)}
      </div>
    </div>
  );
}

// ── HEALTH CHECK-IN ───────────────────────────────────────────────────────────
const HCI_CAT_META = {
  stale:               { icon: '🕐', label: 'Нет обновлений',      color: '#f59e0b' },
  no_goals:            { icon: '○',  label: 'Нет целей',           color: '#6b7280' },
  awaiting_validation: { icon: '○',  label: 'Ожидают валидации',   color: '#6b7280' },
  formation_errors:    { icon: '⚠',  label: 'Ошибки формирования', color: '#ef4444' },
  lagging:             { icon: '▼',  label: 'Отстающие',           color: '#3b82f6' },
};
const HCI_CAT_ORDER = ['stale', 'no_goals', 'awaiting_validation', 'formation_errors', 'lagging'];
const HCI_ACTION_LABEL = {
  stale:               '→ Обновить прогресс',
  no_goals:            '→ Перейти к команде',
  awaiting_validation: '→ Перейти к команде',
  formation_errors:    '→ Исправить',
  lagging:             '→ Перейти к цели',
};

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

function HealthCheckInPanel({ data, open, onClose, onSelectTeam }) {
  const [filter, setFilter] = useState(null);
  if (!data) return null;

  const subtitle = (() => {
    if (data.total_problems > 0) return `Найдено проблем: ${data.total_problems}`;
    const lagging = data.categories?.lagging?.count ?? 0;
    if (lagging > 0) return 'Проблем нет · есть отстающие цели';
    return 'Всё в порядке';
  })();

  const nonEmptyCats = HCI_CAT_ORDER.filter(k => (data.categories?.[k]?.count ?? 0) > 0);
  const visibleCats = filter ? [filter] : HCI_CAT_ORDER;

  return (
    <>
      {open && <div className="hci-backdrop" onClick={onClose}/>}
      <div className={`hci-panel${open ? ' hci-panel--open' : ''}`}>
        <div className="hci-panel__header">
          <div style={{flex:1}}>
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
              Все · {data.total_problems + (data.categories?.lagging?.count ?? 0)}
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
                  {meta.icon} {meta.label} · {cat.count}
                </button>
              );
            })}
          </div>
        )}

        <div className="hci-body">
          {visibleCats.every(k => (data.categories?.[k]?.count ?? 0) === 0) ? (
            <div className="hci-empty">
              <span className="hci-empty__icon">{filter ? '🔍' : '✅'}</span>
              <span>{filter ? 'По выбранному фильтру ничего нет' : 'Всё ok'}</span>
            </div>
          ) : (
            visibleCats.map(k => {
              const cat = data.categories?.[k];
              if (!cat || cat.count === 0) return null;
              const meta = HCI_CAT_META[k];
              const byTeam = {};
              for (const item of cat.items) {
                if (!byTeam[item.team_id]) byTeam[item.team_id] = { name: item.team_name, path: item.team_path, items: [] };
                byTeam[item.team_id].items.push(item);
              }
              return (
                <div key={k} className="hci-section">
                  <div className="hci-section__header" style={{color: meta.color}}>
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
                          <button className="hci-item__action"
                            onClick={() => { onSelectTeam(item.team_id, item.goal_id); onClose(); }}>
                            {HCI_ACTION_LABEL[k]}
                          </button>
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
  const [goalModal, setGoalModal] = useState(null);
  const [accent] = useState(ACCENT);
  const [hciData, setHciData] = useState(null);
  const [hciOpen, setHciOpen] = useState(false);
  const [docUrl, setDocUrl] = useState('');

  const loadHCI = useCallback((pid) => {
    if (!pid) return;
    apiGet(`/api/v1/health-checkin?period_id=${pid}`)
      .then(d => d && setHciData(d));
  }, []);

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

  useEffect(() => {
    Promise.all([apiGet('/api/v1/me'), apiGet('/api/v1/periods'), apiGet('/api/v1/config')]).then(([meData, perData, cfg]) => {
      if (meData) setMe(meData);
      if (cfg) setDocUrl(cfg.documentation_url || '');
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
      if (!selId) {
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
    apiGet(`/api/v1/teams/${selId}/okrs?period_id=${periodId}`).then(data => { if (data) { _cacheUserRefsFromOKR(data); setTeamOKR(data); } });
    const node = findNodeById(hierarchy, selId);
    if (node && (node.children || []).length > 0) {
      apiGet(`/api/v1/teams/${selId}/overview?period_id=${periodId}`).then(data => { if (data) setOverview(data); }).catch(() => {});
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

  const findFirstNode = nodes => {
    for (const n of nodes) { if (!n.children || n.children.length === 0) return n; const c = findFirstNode(n.children || []); if (c) return c; }
    return nodes[0] || null;
  };
  function findNodeById(nodes, id) {
    for (const n of nodes) { if (n.id === id) return n; const f = findNodeById(n.children || [], id); if (f) return f; }
    return null;
  }

  const toggle = useCallback(id => setExpanded(m => ({ ...m, [id]: m[id] === false ? true : !m[id] })), []);
  useEffect(() => { writeTreeExpanded(expanded); }, [expanded]);
  const selectTeam = useCallback(id => setSelId(id), []);
  const handlePeriodChange = id => { setPeriodId(Number(id)); setSelId(null); };

  const reload = useCallback(() => {
    if (!periodId || !selId) return;
    apiGet(`/api/v1/teams/${selId}/okrs?period_id=${periodId}`).then(data => { if (data) { _cacheUserRefsFromOKR(data); setTeamOKR(data); } });
    apiGet(`/api/v1/hierarchy?period_id=${periodId}`).then(data => {
      if (!data) return;
      _cacheUserRefsFromHierarchyNodes(data.items || []);
      setHierarchy(data.items || []);
      const node = findNodeById(data.items || [], selId);
      if (node && (node.children || []).length > 0) {
        apiGet(`/api/v1/teams/${selId}/overview?period_id=${periodId}`).then(d => { if (d) setOverview(d); }).catch(() => {});
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
    for (let i = 0; i < Math.abs(fi - ti); i++) await apiPost(`/api/v1/goals/${fromId}/${dir}`, {});
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

  return (
    <div className="app">
      <div className="sidebar">
        <div className="sidebar__header">
          <div className="sidebar__logo">OKR Tracker</div>
          {me && <HeaderUserMenu user={me} docUrl={docUrl} accent={accent} />}
        </div>
        <div className="sidebar__period">
          <div className="sidebar__period-label">Период</div>
          <select value={periodId || ''} onChange={e => handlePeriodChange(e.target.value)} className="sidebar__period-select">
            {periods.map(p => <option key={p.id} value={p.id} style={{ background: '#1f2937' }}>{p.name}</option>)}
          </select>
        </div>
        <div className="sidebar__tree">
          {!loading && hierarchy.length === 0
            ? (
              <div className="no-access">
                <div className="no-access__icon">🔒</div>
                <div className="no-access__text">Нет доступа к командам</div>
                <div className="no-access__hint">За доступом обратитесь к администратору</div>
              </div>
            )
            : hierarchy.map(n => <SidebarNode key={n.id} node={n} depth={0} selectedId={selId} onSelect={selectTeam} expanded={expanded} toggle={toggle} accent={accent} />)
          }
        </div>
        <div style={{padding: '8px 8px 0'}}>
          <HealthCheckInButton data={hciData} onClick={() => setHciOpen(true)}/>
        </div>
      </div>

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
                    color={HEALTH_COLOR[teamOKR.progress_meta.status === 'above' ? 'ahead' : teamOKR.progress_meta.status === 'below' ? 'below' : 'on_track']} />
                </div>
                <span className="topbar__progress-pct">{teamOKR.period_progress || 0}%</span>
              </div>
            )}
            {editMode === 'full' && selId && <button onClick={() => setGoalModal('new')} className="topbar__add-btn">+ Добавить цель</button>}
          </div>
          {teamOKR?.team?.description && <div className="topbar__desc">{teamOKR.team.description}</div>}
        </div>

        <StatusStepper status={status} hasGoals={hasGoals} onChange={handleChangeStatus} accent={accent} statusChangedAt={teamOKR?.status_changed_at} />

        <div className="content">
          {overview && (overview.children_summary?.items?.length > 0) && <ClusterView overview={overview} onSelect={selectTeam} />}
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
          {overview && (overview.children_summary?.items?.length > 0) && goals.length > 0 && <div className="section-label">Цели этого узла</div>}
          {goals.map(g => <GoalCard key={g.id} goal={g} editMode={editMode} onReload={reload} onEditGoal={setGoalModal} me={me} accent={accent} currentTeamId={selId} allTeams={hierarchy}
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
        onSelectTeam={(teamId, goalId) => {
          selectTeam(teamId);
          if (goalId) {
            setTimeout(() => {
              const el = document.getElementById(`goal-${goalId}`);
              if (el) el.scrollIntoView({ behavior: 'smooth', block: 'center' });
            }, 400);
          }
        }}
      />
    </div>
  );
}

ReactDOM.createRoot(document.getElementById('root')).render(<App />);
