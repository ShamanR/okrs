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
  const h = { 'X-CSRF-Token': readCSRF() };
  return apiFetch(url, { method: 'POST', headers: h, body: fd });
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
function todayStr() { return new Date().toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' }); }

// ── MAPPERS ───────────────────────────────────────────────────────────────────
function mapKR(kr) {
  const m = kr.measure || {};
  let start = 0, target = 100, current = 0, done = false, stages = [];
  if (m.percent) { start = m.percent.start_value; target = m.percent.target_value; current = m.percent.current_value; }
  if (m.linear) { start = m.linear.start_value; target = m.linear.target_value; current = m.linear.current_value; }
  if (m.boolean) { done = m.boolean.is_done; }
  if (m.project) { stages = (m.project.stages || []).map(s => ({ id: s.id, name: s.title, weight: s.weight, done: s.is_done })); }
  return {
    id: kr.id, goalId: kr.goal_id, name: kr.title, desc: kr.description,
    weight: kr.weight, krType: kr.kind, progress: kr.progress,
    start, target, current, done, stages,
    notes: (kr.comments || []).map(c => ({ id: c.id, author: c.author_name, date: fmtDate(c.created_at), text: c.text })),
    updatedAt: kr.updated_at, updatedDaysAgo: daysAgo(kr.updated_at),
  };
}
function mapGoal(g) {
  const shared = (g.share_teams || []).length > 0;
  return {
    id: g.id, teamId: g.team_id, periodId: g.period_id,
    title: g.title, desc: g.description,
    priority: g.priority, weight: g.weight,
    type: (g.work_type || '').toLowerCase(),
    focus: g.focus_type,
    ownerText: g.owner_text,
    progress: g.progress,
    progressMeta: g.progress_meta,
    krs: (g.key_results || []).map(mapKR),
    comments: (g.comments || []).map(c => ({ id: c.id, author: c.author_name, date: fmtDate(c.created_at), text: c.text })),
    shareTeams: g.share_teams || [],
    shared,
    updatedAt: g.updated_at,
    updatedDaysAgo: daysAgo(g.updated_at),
  };
}

// ── KR PROGRESS CALC (client-side for modals) ─────────────────────────────────
function calcKRProgress(kr) {
  if (kr.krType === 'BOOLEAN') return kr.done ? 100 : 0;
  if (kr.krType === 'PROJECT') return Math.min(100, (kr.stages || []).filter(s => s.done).reduce((a, s) => a + (s.weight || 0), 0));
  const r = (kr.target || 100) - (kr.start || 0);
  return r ? Math.min(100, Math.max(0, Math.round(((kr.current || 0) - (kr.start || 0)) / r * 100))) : 0;
}

// ── DESIGN CONSTANTS ──────────────────────────────────────────────────────────
const HEALTH_COLOR = { ahead: '#16a34a', on_track: '#2563eb', below: '#ef4444', stale: '#d97706', no_goals: '#d1d5db' };
const HEALTH_LABEL = { ahead: 'опережает', on_track: 'в плане', below: 'отстаёт', stale: 'нет обновлений', no_goals: 'нет целей' };
const FOCUS_COLORS = { EFFICIENCY: '#0891b2', QUALITY: '#7c3aed', RELIABILITY: '#059669', GROWTH: '#d97706', PROFITABILITY: '#dc2626', STABILITY: '#6366f1', SPEED_EFFICIENCY: '#0891b2', TECH_INDEPENDENCE: '#be185d', DEFAULT: '#6b7280' };
const KR_TYPE_C = { PERCENT: '#2563eb', LINEAR: '#0891b2', BOOLEAN: '#7c3aed', PROJECT: '#d97706' };
const FOCUS_OPTIONS = ['PROFITABILITY', 'STABILITY', 'SPEED_EFFICIENCY', 'TECH_INDEPENDENCE'];
const STATUS_STEPS = [{ k: 'forming', l: 'Черновик' }, { k: 'ready', l: 'К валидации' }, { k: 'in_progress', l: 'В работе' }, { k: 'closed', l: 'Закрыты' }];
const TEAM_TYPE_LABEL = { cluster: 'Кластер', unit: 'Юнит', group: 'Группа', team: 'Команда', squad: 'Сквад' };
const TEAM_TYPE_COLOR = { cluster: '#7c3aed', unit: '#2563eb', group: '#0891b2', team: '#059669', squad: '#d97706' };
const iSt = { width: '100%', padding: '10px 12px', borderRadius: 8, border: '1px solid #e5e7eb', fontSize: 14, outline: 'none', color: '#111827', background: 'white' };

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
    <div style={{ position: 'relative', height: h, background: '#e5e7eb', borderRadius: h / 2, overflow: 'visible' }}>
      <div style={{ height: '100%', width: `${Math.min(value || 0, 100)}%`, background: color || ACCENT, borderRadius: h / 2, transition: 'width .4s ease' }} />
      {forecast != null && <div style={{ position: 'absolute', top: -3, left: `${forecast}%`, width: 2, height: h + 6, background: 'rgba(0,0,0,0.18)', borderRadius: 1, transform: 'translateX(-50%)' }} />}
    </div>
  );
}
function Badge({ label, color = '#6b7280', bg }) {
  return <span style={{ display: 'inline-flex', alignItems: 'center', padding: '2px 7px', borderRadius: 4, fontSize: 11, fontWeight: 600, letterSpacing: .3, color, background: bg || `${color}18` }}>{label}</span>;
}
function PriBadge({ p }) {
  const c = { P0: '#dc2626', P1: '#d97706', P2: '#2563eb', P3: '#6b7280' }[p] || '#6b7280';
  return <Badge label={p} color={c} />;
}
function InfoHint({ children, width = 300 }) {
  const [open, setOpen] = useState(false);
  const [pos, setPos] = useState(null);
  const ref = useRef();
  const show = () => { if (!ref.current) return; const r = ref.current.getBoundingClientRect(); setPos({ top: r.bottom + 6, left: r.left + r.width / 2 }); setOpen(true); };
  return (
    <span ref={ref} onMouseEnter={show} onMouseLeave={() => setOpen(false)} tabIndex={0} onFocus={show} onBlur={() => setOpen(false)}
      style={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'center', width: 16, height: 16, borderRadius: '50%', background: open ? '#dbeafe' : '#f3f4f6', color: open ? '#2563eb' : '#9ca3af', fontSize: 10, fontWeight: 700, cursor: 'help', marginLeft: 5, userSelect: 'none', outline: 'none' }}>
      ?
      {open && pos && <span style={{ position: 'fixed', top: pos.top, left: pos.left, transform: 'translateX(-50%)', width, background: '#111827', color: '#f3f4f6', fontSize: 12, lineHeight: 1.55, padding: '10px 13px', borderRadius: 8, boxShadow: '0 8px 24px rgba(0,0,0,0.25)', zIndex: 2000, pointerEvents: 'none' }}>{children}</span>}
    </span>
  );
}
function FieldLabel({ children, hint, required, size = 13 }) {
  return (
    <div style={{ fontSize: size, fontWeight: 600, color: '#374151', marginBottom: 6, display: 'flex', alignItems: 'center' }}>
      <span>{children}</span>
      {required && <span style={{ color: '#ef4444', marginLeft: 4 }}>*</span>}
      {hint && <InfoHint>{hint}</InfoHint>}
    </div>
  );
}
function Avatar({ name, avatarUrl, size = 28, showName = false }) {
  const initials = (name || '?').split(' ').slice(0, 2).map(w => w[0] || '').join('').toUpperCase();
  const colors = ['#2563eb', '#7c3aed', '#059669', '#d97706', '#dc2626', '#0891b2', '#be185d', '#6366f1'];
  const color = colors[(name || '').charCodeAt(0) % colors.length] || colors[0];
  return (
    <div style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
      {avatarUrl
        ? <img src={avatarUrl} width={size} height={size} style={{ borderRadius: '50%', objectFit: 'cover' }} alt={name || ''} />
        : <div style={{ width: size, height: size, borderRadius: '50%', background: color, display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: size * 0.38, fontWeight: 700, color: 'white', flexShrink: 0 }}>{initials}</div>}
      {showName && <span style={{ fontSize: 13, color: '#374151', fontWeight: 500 }}>{name}</span>}
    </div>
  );
}

// ── HEADER USER MENU ──────────────────────────────────────────────────────────
function HeaderUserMenu({ user, accent }) {
  const [open, setOpen] = useState(false);
  const timer = useRef();
  const show = () => { clearTimeout(timer.current); setOpen(true); };
  const hide = () => { clearTimeout(timer.current); timer.current = setTimeout(() => setOpen(false), 150); };
  const logout = () => { fetch('/logout', { method: 'POST', credentials: 'include', headers: { 'X-CSRF-Token': readCSRF() } }).then(() => location.href = '/login'); };
  const name = user?.display_name || 'Пользователь';
  return (
    <div style={{ position: 'relative' }} onMouseEnter={show} onMouseLeave={hide}>
      <button onClick={() => setOpen(o => !o)}
        style={{ display: 'flex', alignItems: 'center', gap: 7, background: open ? 'rgba(255,255,255,0.1)' : 'transparent', border: 'none', borderRadius: 8, padding: '4px 7px 4px 4px', cursor: 'pointer' }}>
        <Avatar name={name} avatarUrl={user?.avatar_url} size={28} />
        <span style={{ color: '#94a3b8', fontSize: 10, transform: open ? 'rotate(180deg)' : 'none' }}>▾</span>
      </button>
      {open && (
        <div onMouseEnter={show} onMouseLeave={hide}
          style={{ position: 'absolute', top: 'calc(100% + 6px)', right: 0, background: 'white', borderRadius: 10, boxShadow: '0 12px 32px rgba(0,0,0,0.25)', border: '1px solid #e5e7eb', minWidth: 220, zIndex: 1000, overflow: 'hidden' }}>
          <div style={{ padding: '12px 14px', borderBottom: '1px solid #f3f4f6', display: 'flex', alignItems: 'center', gap: 10 }}>
            <Avatar name={name} avatarUrl={user?.avatar_url} size={36} />
            <div><div style={{ fontSize: 13, fontWeight: 700, color: '#111827' }}>{name}</div><div style={{ fontSize: 11, color: '#6b7280' }}>{user?.email || user?.provider || ''}</div></div>
          </div>
          {user?.is_admin && <a href="/admin" style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '9px 14px', textDecoration: 'none', color: '#374151', fontSize: 13 }}
            onMouseEnter={e => e.currentTarget.style.background = '#f9fafb'} onMouseLeave={e => e.currentTarget.style.background = 'none'}>
            <span style={{ color: '#6b7280', width: 20, textAlign: 'center' }}>⚙</span>Управление
          </a>}
          <div style={{ borderTop: '1px solid #f3f4f6' }}>
            <button onClick={logout} style={{ width: '100%', display: 'flex', alignItems: 'center', gap: 10, padding: '9px 14px', background: 'none', border: 'none', cursor: 'pointer', fontSize: 13 }}
              onMouseEnter={e => e.currentTarget.style.background = '#fef2f2'} onMouseLeave={e => e.currentTarget.style.background = 'none'}>
              <span style={{ color: '#dc2626', width: 20, textAlign: 'center' }}>↩</span><span style={{ color: '#dc2626', fontWeight: 500 }}>Выйти</span>
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
  useEffect(() => { const h = e => { if (wrapRef.current && !wrapRef.current.contains(e.target)) setOpen(false); }; document.addEventListener('mousedown', h); return () => document.removeEventListener('mousedown', h); }, []);
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
    <div ref={wrapRef} style={{ position: 'relative' }}>
      <div onClick={() => { setOpen(true); inputRef.current?.focus(); }}
        style={{ minHeight: 42, padding: '5px 7px', border: `1px solid ${open ? accent : '#e5e7eb'}`, borderRadius: 9, background: 'white', display: 'flex', flexWrap: 'wrap', gap: 5, alignItems: 'center', cursor: 'text' }}>
        {sel.map(t => {
          const color = TEAM_TYPE_COLOR[t.type] || '#6b7280'; return (
            <div key={t.id} style={{ display: 'inline-flex', alignItems: 'center', gap: 6, padding: '3px 4px 3px 8px', borderRadius: 16, background: `${color}15`, border: `1px solid ${color}40` }}>
              <span style={{ fontSize: 9, fontWeight: 700, color, textTransform: 'uppercase', letterSpacing: .4 }}>{TEAM_TYPE_LABEL[t.type] || t.type}</span>
              <span style={{ fontSize: 12, fontWeight: 600, color: '#111827' }}>{t.name}</span>
              <button onClick={e => { e.stopPropagation(); rem(t.id); }} style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#6b7280', fontSize: 14, lineHeight: 1, padding: '0 4px' }}>×</button>
            </div>
          );
        })}
        <input ref={inputRef} value={q} onChange={e => { setQ(e.target.value); setOpen(true); }} onFocus={() => setOpen(true)} onKeyDown={onKey}
          placeholder={sel.length ? 'Ещё…' : 'Найдите команду'} style={{ flex: 1, minWidth: 140, border: 'none', outline: 'none', fontSize: 13, padding: '6px 4px', background: 'transparent', fontFamily: 'inherit' }} />
      </div>
      {open && <div style={{ position: 'absolute', top: 'calc(100% + 4px)', left: 0, right: 0, background: 'white', borderRadius: 9, boxShadow: '0 10px 30px rgba(0,0,0,0.15)', border: '1px solid #e5e7eb', maxHeight: 280, overflow: 'auto', zIndex: 50 }}>
        {available.length === 0 ? <div style={{ padding: '14px 16px', fontSize: 12, color: '#9ca3af', textAlign: 'center' }}>{ql ? 'Не найдено' : 'Все добавлены'}</div> :
          available.map((t, i) => {
            const color = TEAM_TYPE_COLOR[t.type] || '#6b7280'; return (
              <div key={t.id} onClick={() => add(t)} onMouseEnter={() => setHi(i)}
                style={{ padding: '7px 12px 7px ' + (8 + t.depth * 14) + 'px', display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer', background: i === hi ? '#f3f4f6' : 'white' }}>
                <div style={{ width: 3, height: 18, borderRadius: 2, background: color, flexShrink: 0 }} />
                <span style={{ fontSize: 9, fontWeight: 700, color, textTransform: 'uppercase', letterSpacing: .4, padding: '1px 4px', background: `${color}12`, borderRadius: 3 }}>{TEAM_TYPE_LABEL[t.type] || t.type}</span>
                <span style={{ fontSize: 13, color: '#111827', flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{t.name}</span>
              </div>
            );
          })}
      </div>}
    </div>
  );
}

// ── STATUS STEPPER ────────────────────────────────────────────────────────────
function StatusStepper({ status, hasGoals, onChange, accent, statusChangedAt }) {
  const curIdx = STATUS_STEPS.findIndex(s => s.k === status);
  return (
    <div style={{ background: 'white', borderBottom: '1px solid #e5e7eb', padding: '10px 24px', display: 'flex', alignItems: 'center', gap: 0, flexWrap: 'wrap' }}>
      {!hasGoals && <span style={{ fontSize: 12, fontWeight: 600, color: '#9ca3af', background: '#f3f4f6', padding: '5px 12px', borderRadius: 20, marginRight: 8 }}>Нет целей</span>}
      {STATUS_STEPS.map((s, i) => {
        const isCur = s.k === status; const isPast = i < curIdx;
        return (
          <React.Fragment key={s.k}>
            {i > 0 && <div style={{ width: 20, height: 1, background: '#e5e7eb', flexShrink: 0 }} />}
            <button onClick={() => hasGoals && onChange(s.k)} disabled={!hasGoals}
              style={{
                padding: '5px 12px', borderRadius: 20, fontSize: 12, fontWeight: 600, border: 'none', cursor: hasGoals ? 'pointer' : 'default', transition: 'all .15s',
                background: isCur ? accent : isPast ? `${accent}15` : 'transparent', color: isCur ? 'white' : isPast ? accent : '#9ca3af'
              }}>
              {s.l}
            </button>
          </React.Fragment>
        );
      })}
      <div style={{ marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: 8 }}>
        {statusChangedAt && <span style={{ fontSize: 11, color: '#9ca3af' }}>изменён {fmtDate(statusChangedAt)}</span>}
        {status === 'in_progress' && <span style={{ fontSize: 11, fontWeight: 600, color: '#ef4444', background: '#fef2f2', padding: '2px 8px', borderRadius: 4 }}>🔒 Редактирование заблокировано</span>}
        {status === 'closed' && <span style={{ fontSize: 11, fontWeight: 600, color: '#6b7280', background: '#f3f4f6', padding: '2px 8px', borderRadius: 4 }}>🔒 Период закрыт</span>}
      </div>
    </div>
  );
}

// ── KR PROGRESS MODAL ─────────────────────────────────────────────────────────
function KRProgressModal({ kr, onSave, onClose, accent }) {
  const [form, setForm] = useState({ ...kr, stages: (kr.stages || []).map(s => ({ ...s })) });
  const [note, setNote] = useState(''); const [saving, setSaving] = useState(false);
  const set = (k, v) => setForm(f => ({ ...f, [k]: v }));
  const setStage = (i, k, v) => setForm(f => { const ss = [...f.stages]; ss[i] = { ...ss[i], [k]: v }; return { ...f, stages: ss }; });
  const progress = calcKRProgress(form);
  const save = async () => {
    setSaving(true);
    try {
      if (form.krType === 'PERCENT' || form.krType === 'LINEAR') {
        await apiPost(`/api/v1/krs/${kr.id}/progress/percent`, { current_value: parseFloat(form.current) || 0 });
      } else if (form.krType === 'BOOLEAN') {
        await apiPost(`/api/v1/krs/${kr.id}/progress/boolean`, { done: !!form.done });
      } else if (form.krType === 'PROJECT') {
        await apiPost(`/api/v1/krs/${kr.id}/progress/project`, { stages: form.stages.map(s => ({ id: s.id, done: !!s.done })) });
      }
      if (note.trim()) {
        await apiPost(`/api/v1/krs/${kr.id}/comments`, { text: note.trim() });
      }
      onSave();
    } catch (e) { alert('Ошибка сохранения: ' + e.message); }
    finally { setSaving(false); }
  };
  return (
    <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', zIndex: 300, display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 20 }} onClick={onClose}>
      <div onClick={e => e.stopPropagation()} style={{ background: 'white', borderRadius: 14, width: 480, boxShadow: '0 20px 60px rgba(0,0,0,0.2)', overflow: 'hidden' }}>
        <div style={{ padding: '18px 22px 14px', borderBottom: '1px solid #f3f4f6', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <div><div style={{ fontSize: 15, fontWeight: 700, color: '#111827' }}>Обновить прогресс</div><div style={{ fontSize: 12, color: '#6b7280', marginTop: 2 }}>{kr.name}</div></div>
          <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', fontSize: 20, color: '#9ca3af', padding: 4 }}>×</button>
        </div>
        <div style={{ padding: '18px 22px' }}>
          {(form.krType === 'PERCENT' || form.krType === 'LINEAR') && (
            <div style={{ marginBottom: 14 }}>
              <div style={{ fontSize: 12, fontWeight: 600, color: '#374151', marginBottom: 5 }}>Текущее значение <span style={{ color: '#9ca3af', fontWeight: 400 }}>({form.start} → {form.target})</span></div>
              <input type="number" value={form.current} onChange={e => set('current', e.target.value)} style={iSt} />
              <div style={{ marginTop: 10 }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}><span style={{ fontSize: 12, color: '#6b7280' }}>Прогресс</span><span style={{ fontSize: 13, fontWeight: 700, color: accent }}>{progress}%</span></div>
                <ProgressBar value={progress} h={6} color={accent} />
              </div>
            </div>
          )}
          {form.krType === 'BOOLEAN' && (
            <label style={{ display: 'flex', alignItems: 'center', gap: 10, cursor: 'pointer', marginBottom: 14, padding: '12px', background: '#f9fafb', borderRadius: 8 }}>
              <input type="checkbox" checked={!!form.done} onChange={e => set('done', e.target.checked)} style={{ width: 18, height: 18, accentColor: accent }} />
              <span style={{ fontSize: 14, fontWeight: 500, color: '#111827' }}>Выполнено</span>
              <span style={{ marginLeft: 'auto', fontSize: 13, fontWeight: 700, color: form.done ? '#16a34a' : '#9ca3af' }}>{form.done ? '100%' : '0%'}</span>
            </label>
          )}
          {form.krType === 'PROJECT' && (
            <div style={{ marginBottom: 14 }}>
              <div style={{ fontSize: 12, fontWeight: 600, color: '#374151', marginBottom: 8 }}>Шаги</div>
              {form.stages.map((s, i) => (
                <label key={s.id || i} style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '8px 10px', borderRadius: 7, cursor: 'pointer', marginBottom: 4, background: s.done ? `${accent}08` : '#f9fafb', border: `1px solid ${s.done ? `${accent}30` : '#f0f1f3'}` }}>
                  <input type="checkbox" checked={!!s.done} onChange={e => setStage(i, 'done', e.target.checked)} style={{ width: 16, height: 16, accentColor: accent }} />
                  <span style={{ flex: 1, fontSize: 13, color: '#374151', fontWeight: s.done ? 600 : 400 }}>{s.name}</span>
                  <span style={{ fontSize: 11, color: '#9ca3af' }}>вес {s.weight}</span>
                </label>
              ))}
              <div style={{ marginTop: 10 }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}><span style={{ fontSize: 12, color: '#6b7280' }}>Прогресс</span><span style={{ fontSize: 13, fontWeight: 700, color: accent }}>{progress}%</span></div>
                <ProgressBar value={progress} h={6} color={accent} />
              </div>
            </div>
          )}
          <div>
            <div style={{ fontSize: 12, fontWeight: 600, color: '#374151', marginBottom: 5 }}>Заметка <span style={{ color: '#9ca3af', fontWeight: 400 }}>(опционально)</span></div>
            <textarea value={note} onChange={e => setNote(e.target.value)} rows={3} placeholder="Контекст, блокеры…" style={{ ...iSt, resize: 'vertical' }} />
          </div>
        </div>
        <div style={{ padding: '0 22px 18px', display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
          <button onClick={onClose} style={{ padding: '9px 18px', border: '1px solid #e5e7eb', borderRadius: 8, background: 'white', color: '#374151', fontSize: 13, cursor: 'pointer' }}>Отмена</button>
          <button onClick={save} disabled={saving} style={{ padding: '9px 22px', border: 'none', borderRadius: 8, background: saving ? '#e5e7eb' : accent, color: 'white', fontSize: 13, fontWeight: 600, cursor: saving ? 'default' : 'pointer' }}>{saving ? 'Сохраняем…' : 'Сохранить'}</button>
        </div>
      </div>
    </div>
  );
}

// ── KR EDIT MODAL ─────────────────────────────────────────────────────────────
function KREditModal({ kr, goalId, onSave, onClose, accent }) {
  const isNew = !kr;
  const [form, setForm] = useState(kr ? { ...kr, stages: (kr.stages || []).map(s => ({ ...s })) } : { name: '', desc: '', weight: 20, krType: 'PERCENT', start: 0, target: 100, current: 0, done: false, stages: [] });
  const [saving, setSaving] = useState(false);
  const set = (k, v) => setForm(f => ({ ...f, [k]: v }));
  const setSt = (i, k, v) => setForm(f => { const ss = [...f.stages]; ss[i] = { ...ss[i], [k]: v }; return { ...f, stages: ss }; });
  const addSt = () => setForm(f => ({ ...f, stages: [...f.stages, { id: `s_${Date.now()}`, name: '', weight: 0, done: false }] }));
  const remSt = i => setForm(f => ({ ...f, stages: f.stages.filter((_, j) => j !== i) }));
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
      if (form.krType === 'PERCENT') { fd.append('percent_start', String(form.start || 0)); fd.append('percent_target', String(form.target || 100)); fd.append('percent_current', String(form.current || 0)); }
      else if (form.krType === 'LINEAR') { fd.append('linear_start', String(form.start || 0)); fd.append('linear_target', String(form.target || 100)); fd.append('linear_current', String(form.current || 0)); }
      else if (form.krType === 'BOOLEAN') { fd.append('boolean_done', form.done ? 'true' : 'false'); }
      else if (form.krType === 'PROJECT') {
        (form.stages || []).forEach(st => { fd.append('step_title[]', st.name || ''); fd.append('step_weight[]', String(st.weight || 0)); fd.append('step_done[]', st.done ? 'true' : 'false'); });
      }
      const url = isNew ? `/api/v1/goals/${goalId}/key-results` : `/api/v1/krs/${kr.id}`;
      await apiForm(url, fd);
      onSave();
    } catch (e) { alert('Ошибка: ' + e.message); }
    finally { setSaving(false); }
  };
  return (
    <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', zIndex: 300, display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 20 }} onClick={onClose}>
      <div onClick={e => e.stopPropagation()} style={{ background: 'white', borderRadius: 14, width: 560, maxHeight: '90vh', overflow: 'auto', boxShadow: '0 20px 60px rgba(0,0,0,0.25)' }}>
        <div style={{ padding: '20px 24px 14px', borderBottom: '1px solid #f3f4f6', display: 'flex', justifyContent: 'space-between', alignItems: 'center', position: 'sticky', top: 0, background: 'white', zIndex: 1 }}>
          <div style={{ fontSize: 16, fontWeight: 700, color: '#111827' }}>{isNew ? 'Добавить KR' : 'Редактировать KR'}</div>
          <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', fontSize: 20, color: '#9ca3af', padding: 4 }}>×</button>
        </div>
        <div style={{ padding: '18px 24px' }}>
          <div style={{ marginBottom: 12 }}><div style={{ fontSize: 12, fontWeight: 600, color: '#374151', marginBottom: 5 }}>Название</div><input value={form.name} onChange={e => set('name', e.target.value)} placeholder="Что измеряет этот KR?" style={iSt} /></div>
          <div style={{ marginBottom: 12 }}><div style={{ fontSize: 12, fontWeight: 600, color: '#374151', marginBottom: 5 }}>Описание</div><textarea value={form.desc || ''} onChange={e => set('desc', e.target.value)} rows={2} style={{ ...iSt, resize: 'vertical' }} /></div>
          <div style={{ display: 'flex', gap: 12, marginBottom: 14 }}>
            <div style={{ flex: 1 }}><div style={{ fontSize: 12, fontWeight: 600, color: '#374151', marginBottom: 5 }}>Вес</div><input type="number" min={0} max={100} value={form.weight} onChange={e => set('weight', e.target.value)} style={iSt} /></div>
            <div style={{ flex: 1 }}><div style={{ fontSize: 12, fontWeight: 600, color: '#374151', marginBottom: 5 }}>Тип</div>
              <select value={form.krType} onChange={e => set('krType', e.target.value)} style={{ ...iSt, cursor: 'pointer' }}>
                {['PERCENT', 'LINEAR', 'BOOLEAN', 'PROJECT'].map(t => <option key={t}>{t}</option>)}
              </select>
            </div>
          </div>
          {(form.krType === 'PERCENT' || form.krType === 'LINEAR') && (
            <div style={{ background: '#f9fafb', borderRadius: 10, padding: '14px', marginBottom: 14, border: '1px solid #f0f1f3' }}>
              <div style={{ fontSize: 11, fontWeight: 700, color: '#6b7280', textTransform: 'uppercase', marginBottom: 12 }}>{form.krType === 'PERCENT' ? 'Процент' : 'Числовой прогресс'}</div>
              <div style={{ display: 'flex', gap: 12, marginBottom: 12 }}>
                {['start', 'target', 'current'].map(f2 => (
                  <div key={f2} style={{ flex: 1 }}>
                    <div style={{ fontSize: 11, fontWeight: 600, color: '#374151', marginBottom: 4, textTransform: 'capitalize' }}>{f2}</div>
                    <input type="number" value={form[f2]} onChange={e => set(f2, e.target.value)} style={{ ...iSt, padding: '7px 10px', fontSize: 13 }} />
                  </div>
                ))}
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}><span style={{ fontSize: 12, color: '#6b7280' }}>Прогресс</span><span style={{ fontSize: 12, fontWeight: 700, color: accent }}>{prev}%</span></div>
              <ProgressBar value={prev} h={5} color={accent} />
            </div>
          )}
          {form.krType === 'BOOLEAN' && (
            <div style={{ background: '#f9fafb', borderRadius: 10, padding: '14px', marginBottom: 14, border: '1px solid #f0f1f3' }}>
              <label style={{ display: 'flex', alignItems: 'center', gap: 10, cursor: 'pointer' }}>
                <input type="checkbox" checked={!!form.done} onChange={e => set('done', e.target.checked)} style={{ width: 18, height: 18, accentColor: accent }} />
                <span style={{ fontSize: 14, fontWeight: 500, color: '#111827' }}>Выполнено</span>
                <span style={{ marginLeft: 'auto', fontWeight: 700, color: form.done ? '#16a34a' : '#9ca3af' }}>{form.done ? '100%' : '0%'}</span>
              </label>
            </div>
          )}
          {form.krType === 'PROJECT' && (
            <div style={{ background: '#f9fafb', borderRadius: 10, padding: '14px', marginBottom: 14, border: '1px solid #f0f1f3' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 10 }}>
                <div style={{ fontSize: 11, fontWeight: 700, color: '#6b7280', textTransform: 'uppercase' }}>Шаги проекта</div>
                <div style={{ fontSize: 11, fontWeight: 600, color: Math.abs(sw - 100) < 1 ? '#16a34a' : '#ef4444' }}>Сумма: {sw}</div>
              </div>
              {form.stages.map((st, i) => (
                <div key={st.id || i} style={{ display: 'flex', gap: 8, alignItems: 'center', marginBottom: 8 }}>
                  <input type="checkbox" checked={!!st.done} onChange={e => setSt(i, 'done', e.target.checked)} style={{ width: 16, height: 16, accentColor: accent, flexShrink: 0 }} />
                  <input value={st.name} onChange={e => setSt(i, 'name', e.target.value)} placeholder="Название шага" style={{ ...iSt, flex: 1, padding: '7px 10px', fontSize: 13 }} />
                  <input type="number" min={0} value={st.weight} onChange={e => setSt(i, 'weight', Number(e.target.value))} style={{ ...iSt, width: 60, padding: '7px 10px', fontSize: 13, textAlign: 'center' }} />
                  <button onClick={() => remSt(i)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#ef4444', fontSize: 18, flexShrink: 0 }}>×</button>
                </div>
              ))}
              <button onClick={addSt} style={{ width: '100%', padding: '7px', border: '1px dashed #d1d5db', borderRadius: 7, background: 'transparent', color: '#6b7280', fontSize: 12, cursor: 'pointer' }}>+ Добавить шаг</button>
            </div>
          )}
        </div>
        <div style={{ padding: '0 24px 18px', display: 'flex', gap: 10, justifyContent: 'flex-end', position: 'sticky', bottom: 0, background: 'white', borderTop: '1px solid #f3f4f6', paddingTop: 14 }}>
          <button onClick={onClose} style={{ padding: '9px 18px', border: '1px solid #e5e7eb', borderRadius: 8, background: 'white', color: '#374151', fontSize: 13, cursor: 'pointer' }}>Отмена</button>
          <button onClick={save} disabled={saving || !form.name.trim()} style={{ padding: '9px 24px', border: 'none', borderRadius: 8, background: (saving || !form.name.trim()) ? '#e5e7eb' : accent, color: (saving || !form.name.trim()) ? '#9ca3af' : 'white', fontSize: 13, fontWeight: 600, cursor: (saving || !form.name.trim()) ? 'default' : 'pointer' }}>Сохранить</button>
        </div>
      </div>
    </div>
  );
}

// ── KR ROW ────────────────────────────────────────────────────────────────────
// ── CONFIRM MODAL ─────────────────────────────────────────────────────────────
function ConfirmModal({ title, message, confirmLabel, onConfirm, onClose }) {
  const [busy, setBusy] = React.useState(false);
  const run = async () => {
    setBusy(true);
    try { await onConfirm(); } finally { setBusy(false); }
  };
  return (
    <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.45)', zIndex: 600, display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 20 }} onClick={onClose}>
      <div onClick={e => e.stopPropagation()} style={{ background: 'white', borderRadius: 14, width: 380, boxShadow: '0 20px 60px rgba(0,0,0,0.25)', overflow: 'hidden' }}>
        <div style={{ padding: '22px 24px 16px' }}>
          <div style={{ fontSize: 16, fontWeight: 700, color: '#111827', marginBottom: 8 }}>{title}</div>
          <div style={{ fontSize: 13, color: '#6b7280', lineHeight: 1.5 }}>{message}</div>
        </div>
        <div style={{ padding: '12px 24px 20px', display: 'flex', gap: 10, justifyContent: 'flex-end' }}>
          <button onClick={onClose} disabled={busy} style={{ padding: '9px 18px', border: '1px solid #e5e7eb', borderRadius: 8, background: 'white', color: '#374151', fontSize: 13, cursor: 'pointer' }}>Отмена</button>
          <button onClick={run} disabled={busy} style={{ padding: '9px 18px', border: 'none', borderRadius: 8, background: busy ? '#e5e7eb' : '#dc2626', color: busy ? '#9ca3af' : 'white', fontSize: 13, fontWeight: 700, cursor: busy ? 'default' : 'pointer' }}>{busy ? 'Удаляем…' : (confirmLabel || 'Удалить')}</button>
        </div>
      </div>
    </div>
  );
}

function KRRow({ kr, goalId, editMode, onReload, accent }) {
  const [modal, setModal] = useState(null);
  const [showNotes, setShowNotes] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const progress = kr.progress;
  const staleC = kr.updatedDaysAgo > 7 ? '#dc2626' : kr.updatedDaysAgo > 4 ? '#d97706' : '#10b981';
  let detail = null;
  if (kr.krType === 'BOOLEAN') detail = <span style={{ fontSize: 12, fontWeight: 600, color: kr.done ? '#16a34a' : '#9ca3af' }}>{kr.done ? '✓ Выполнено' : '○ Не выполнено'}</span>;
  else if (kr.krType === 'PROJECT') detail = <span style={{ fontSize: 12, color: '#6b7280' }}>{(kr.stages || []).filter(s => s.done).length}/{(kr.stages || []).length} шагов</span>;
  else detail = <span style={{ fontSize: 12, color: '#6b7280', fontVariantNumeric: 'tabular-nums' }}>{kr.current} → {kr.target}</span>;
  const onSaved = () => { setModal(null); onReload(); };
  return (
    <>
      <div style={{ padding: '10px 0', borderBottom: '1px solid #f3f4f6' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
          <div style={{ width: 26, height: 26, borderRadius: 6, background: '#f3f4f6', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 10, fontWeight: 700, color: '#6b7280', flexShrink: 0 }}>{kr.weight}</div>
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{ fontSize: 13, fontWeight: 500, color: '#111827', marginBottom: 3, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{kr.name}</div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
              <div style={{ flex: 1, maxWidth: 160 }}><ProgressBar value={progress} h={4} color={accent} /></div>
              <span style={{ fontSize: 11, fontWeight: 700, color: accent, flexShrink: 0 }}>{progress}%</span>
              {detail}
            </div>
          </div>
          <Badge label={kr.krType} color={KR_TYPE_C[kr.krType]} />
          <span style={{ fontSize: 11, color: staleC, flexShrink: 0, minWidth: 72, textAlign: 'right' }}>{kr.updatedDaysAgo === 0 ? 'сегодня' : `${kr.updatedDaysAgo}д назад`}</span>
          {kr.notes && kr.notes.length > 0 && <button onClick={() => setShowNotes(!showNotes)} style={{ fontSize: 11, color: '#6b7280', background: 'none', border: 'none', cursor: 'pointer', flexShrink: 0 }}>📝 {kr.notes.length}</button>}
          {editMode === 'full' && <>
            <button onClick={() => setModal('edit')} style={{ padding: '5px 10px', border: '1px solid #e5e7eb', borderRadius: 6, background: 'white', color: '#374151', fontSize: 12, fontWeight: 500, cursor: 'pointer', flexShrink: 0 }}>Редактировать</button>
            <button onClick={() => setConfirmDelete(true)} title="Удалить KR" style={{ padding: '4px 7px', border: '1px solid #fca5a5', borderRadius: 6, background: '#fff1f1', color: '#dc2626', fontSize: 14, fontWeight: 700, cursor: 'pointer', flexShrink: 0, lineHeight: 1 }}>×</button>
          </>}
          {editMode === 'progress_only' && <button onClick={() => setModal('progress')} style={{ padding: '5px 10px', border: `1px solid ${accent}`, borderRadius: 6, background: `${accent}10`, color: accent, fontSize: 12, fontWeight: 600, cursor: 'pointer', flexShrink: 0 }}>Обновить прогресс</button>}
        </div>
        {showNotes && (kr.notes || []).length > 0 && (
          <div style={{ marginTop: 10, marginLeft: 36, borderLeft: '2px solid #f0f1f3', paddingLeft: 12 }}>
            {(kr.notes || []).map((n, i) => (
              <div key={n.id || i} style={{ display: 'flex', gap: 8, marginBottom: 8, alignItems: 'flex-start' }}>
                <Avatar name={n.author} size={22} />
                <div style={{ flex: 1 }}>
                  <div style={{ display: 'flex', gap: 6, alignItems: 'baseline', marginBottom: 2 }}>
                    <span style={{ fontSize: 11, fontWeight: 600, color: '#374151' }}>{n.author}</span>
                    <span style={{ fontSize: 10, color: '#9ca3af' }}>{n.date}</span>
                  </div>
                  <div style={{ fontSize: 12, color: '#374151', background: '#f9fafb', padding: '6px 9px', borderRadius: 6 }}>{n.text}</div>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
      {modal === 'progress' && <KRProgressModal kr={kr} onSave={onSaved} onClose={() => setModal(null)} accent={accent} />}
      {modal === 'edit' && <KREditModal kr={kr} goalId={goalId} onSave={onSaved} onClose={() => setModal(null)} accent={accent} />}
      {confirmDelete && <ConfirmModal title="Удалить Key Result?" message={`«${kr.name}» будет удалён без возможности восстановления.`} onConfirm={async () => { await apiDelete(`/api/v1/krs/${kr.id}`); setConfirmDelete(false); onReload(); }} onClose={() => setConfirmDelete(false)} />}
    </>
  );
}

// ── COMMENTS PANEL ────────────────────────────────────────────────────────────
function CommentsPanel({ comments, onAdd, me }) {
  const [text, setText] = useState(''); const [saving, setSaving] = useState(false);
  const submit = async () => {
    if (!text.trim()) return;
    setSaving(true); try { await onAdd(text.trim()); setText(''); } catch { } finally { setSaving(false); };
  };
  return (
    <div style={{ paddingTop: 14 }}>
      <div style={{ fontSize: 11, fontWeight: 700, color: '#9ca3af', textTransform: 'uppercase', letterSpacing: .6, marginBottom: 12 }}>Комментарии</div>
      {(comments || []).map((c, i) => (
        <div key={c.id || i} style={{ display: 'flex', gap: 10, marginBottom: 14 }}>
          <Avatar name={c.author} size={28} />
          <div style={{ flex: 1 }}>
            <div style={{ display: 'flex', gap: 8, alignItems: 'baseline', marginBottom: 3 }}>
              <span style={{ fontSize: 12, fontWeight: 600, color: '#374151' }}>{c.author}</span>
              <span style={{ fontSize: 11, color: '#9ca3af' }}>{c.date}</span>
            </div>
            <div style={{ fontSize: 13, color: '#374151', lineHeight: 1.55, background: '#f9fafb', padding: '9px 12px', borderRadius: 8, border: '1px solid #f3f4f6' }}>{c.text}</div>
          </div>
        </div>
      ))}
      <div style={{ display: 'flex', gap: 8, alignItems: 'flex-start' }}>
        <Avatar name={me?.display_name} avatarUrl={me?.avatar_url} size={28} />
        <div style={{ flex: 1 }}>
          <textarea value={text} onChange={e => setText(e.target.value)} placeholder="Контекст, блокер, заметка… (Cmd+Enter)"
            onKeyDown={e => { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) submit(); }}
            style={{ width: '100%', padding: '9px 11px', borderRadius: 8, border: '1px solid #e5e7eb', fontSize: 13, lineHeight: 1.5, minHeight: 60, outline: 'none', color: '#374151' }} />
          <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 5 }}>
            <button onClick={submit} disabled={!text.trim() || saving} style={{ padding: '6px 14px', background: text.trim() ? ACCENT : '#e5e7eb', color: text.trim() ? '#fff' : '#9ca3af', borderRadius: 7, border: 'none', cursor: text.trim() ? 'pointer' : 'default', fontSize: 13, fontWeight: 600 }}>{saving ? '…' : 'Отправить'}</button>
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

  const addGoalComment = async text => {
    await apiPost(`/api/v1/goals/${goal.id}/comments`, { text });
    onReload();
  };
  const handleDeleteGoal = async () => {
    await apiDelete(`/api/v1/goals/${goal.id}`);
    onReload();
  };

  return (
    <div {...rootDrag} draggable={!!(canReorderGoal && goalDraggable)}
      onDragEnd={e => { setGoalDraggable(false); rootDrag.onDragEnd && rootDrag.onDragEnd(e); }}
      style={{ background: 'white', borderRadius: 12, marginBottom: 12, boxShadow: isDragging ? '0 8px 24px rgba(0,0,0,0.15)' : '0 1px 3px rgba(0,0,0,0.05)', border: isStale ? '1px solid #fde68a' : '1px solid #f0f1f3', borderLeft: otherTeams.length > 0 ? '3px solid #0891b2' : (isStale ? '1px solid #fde68a' : '1px solid #f0f1f3'), opacity: isDragging ? 0.5 : 1, transition: 'box-shadow .15s,opacity .15s', position: 'relative', marginLeft: canReorderGoal ? 14 : 0 }}>
      {canReorderGoal && (
        <div onMouseDown={() => setGoalDraggable(true)} onMouseUp={() => setGoalDraggable(false)} onMouseLeave={() => setGoalDraggable(false)}
          title="Перетащите для изменения порядка"
          style={{ position: 'absolute', left: -14, top: '50%', transform: 'translateY(-50%)', width: 14, height: 42, display: 'flex', alignItems: 'center', justifyContent: 'center', cursor: 'grab', color: '#d1d5db', fontSize: 13, userSelect: 'none', borderRadius: 4 }}
          onMouseEnter={e => e.currentTarget.style.color = '#6b7280'} onMouseLeave={e => { e.currentTarget.style.color = '#d1d5db'; setGoalDraggable(false); }}>⋮⋮</div>
      )}
      <div style={{ padding: '16px 18px' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8, flexWrap: 'wrap' }}>
          <PriBadge p={goal.priority} />
          <span style={{ fontSize: 11, color: '#9ca3af', fontWeight: 500 }}>вес {goal.weight}%</span>
          {otherTeams.length > 0 && <Badge label={`⇄ Общая · ${otherTeams.length + 1} команд`} color="#0891b2" />}
          <div style={{ flex: 1 }} />
          {isStale && <Badge label={`⚠ ${goal.updatedDaysAgo}д без обновлений`} color="#d97706" bg="#fffbeb" />}
          {goal.ownerText && <div style={{ display: 'flex', alignItems: 'center', gap: 5, fontSize: 11, color: '#6b7280' }}><span style={{ color: '#9ca3af', fontWeight: 500 }}>Владелец</span><span>👤</span><span>{goal.ownerText}</span></div>}
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 4 }}>
          <div onClick={canEdit ? () => onEditGoal(goal) : undefined}
            style={{ fontSize: 15, fontWeight: 700, color: '#111827', lineHeight: 1.35, cursor: canEdit ? 'pointer' : 'default', display: 'inline-flex', alignItems: 'center', gap: 6, flex: 1 }}>
            {goal.title}
            {canEdit && <span style={{ fontSize: 11, color: '#9ca3af', fontWeight: 500 }}>✎</span>}
          </div>
          {canEdit && <button onClick={() => setConfirmDeleteGoal(true)} title="Удалить цель" style={{ padding: '3px 7px', border: '1px solid #fca5a5', borderRadius: 6, background: '#fff1f1', color: '#dc2626', fontSize: 15, fontWeight: 700, cursor: 'pointer', flexShrink: 0, lineHeight: 1 }}>×</button>}
        </div>
        {goal.desc && <div style={{ fontSize: 13, color: '#6b7280', marginBottom: 12, lineHeight: 1.5 }}>{goal.desc}</div>}
        {otherTeams.length > 0 && (
          <div style={{ marginBottom: 12, padding: '8px 10px', background: '#ecfeff', borderRadius: 8, border: '1px solid #a5f3fc', display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
            <span style={{ fontSize: 11, color: '#0e7490', fontWeight: 600, flexShrink: 0 }}>⇄ Общая с:</span>
            {[...(goal.shareTeams || []).filter(t => t.id === currentTeamId).map(t => ({ ...t, isSelf: true })), ...otherTeams].map(t => (
              <span key={t.id} style={{ display: 'inline-flex', alignItems: 'center', gap: 4, padding: '2px 8px', borderRadius: 12, background: t.isSelf ? '#e0f2fe' : 'white', border: `1px solid ${t.isSelf ? '#38bdf8' : '#0891b260'}`, fontSize: 11, fontWeight: 500, color: t.isSelf ? '#0369a1' : '#164e63' }}>
                {t.name}{t.isSelf && <span style={{ color: '#0369a1', fontWeight: 700 }}> · Вы</span>}
              </span>
            ))}
          </div>
        )}
        <div style={{ marginBottom: 10 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 5 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <span style={{ fontSize: 20, fontWeight: 800, color: hC, letterSpacing: -.5 }}>{prog}%</span>
              <span style={{ fontSize: 11, fontWeight: 700, color: hC, background: `${hC}15`, padding: '2px 7px', borderRadius: 4 }}>
                {health === 'ahead' ? '▲ опережает' : health === 'on_track' ? '✓ в плане' : health === 'stale' ? '⚠ нет обновлений' : '▼ отстаёт'}
              </span>
            </div>
            <span style={{ fontSize: 11, color: '#9ca3af' }}>{goal.updatedDaysAgo === 0 ? 'сегодня' : `${goal.updatedDaysAgo}д назад`}</span>
          </div>
          {goal.progressMeta && <><ProgressBar value={prog} forecast={goal.progressMeta.forecast} h={9} color={hC} />
            <div style={{ fontSize: 10, color: '#9ca3af', marginTop: 3, textAlign: 'right' }}>прогноз {goal.progressMeta.forecast}%</div></>}
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'wrap' }}>
          <Badge label={goal.type === 'delivery' ? 'Delivery' : 'Discovery'} color={goal.type === 'delivery' ? '#374151' : '#7c3aed'} />
          {goal.focus && <Badge label={goal.focus} color={FOCUS_COLORS[goal.focus] || FOCUS_COLORS.DEFAULT} />}
        </div>
      </div>
      <div style={{ display: 'flex', borderTop: '1px solid #f3f4f6', background: '#fafafa' }}>
        <button onClick={() => setShowKR(!showKR)} style={{ flex: 1, padding: '8px 0', border: 'none', background: 'transparent', cursor: 'pointer', fontSize: 12, fontWeight: 500, color: '#6b7280', display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 5 }}>
          <span style={{ fontSize: 9 }}>{showKR ? '▲' : '▼'}</span>{showKR ? 'Скрыть KR' : `KR (${(goal.krs || []).length})`}
        </button>
        <div style={{ width: 1, background: '#f3f4f6' }} />
        <button onClick={() => setShowCom(!showCom)} style={{ flex: 1, padding: '8px 0', border: 'none', background: 'transparent', cursor: 'pointer', fontSize: 12, fontWeight: 500, color: (goal.comments || []).length > 0 ? '#374151' : '#6b7280', display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 5 }}>
          {(goal.comments || []).length > 0 ? `💬 ${goal.comments.length}` : '💬 Комментарии'}
        </button>
      </div>
      {showKR && (
        <div style={{ padding: '4px 18px 14px', borderTop: '1px solid #f3f4f6' }}>
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
                style={{ position: 'relative', opacity: isKrDrag ? 0.45 : 1, transition: 'opacity .15s', marginLeft: canReorderKR ? 14 : 0 }}>
                {canReorderKR && (
                  <div style={{ position: 'absolute', left: -14, top: '50%', transform: 'translateY(-50%)', width: 14, fontSize: 11, color: '#d1d5db', cursor: 'grab', userSelect: 'none', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>⋮⋮</div>
                )}
                <KRRow kr={kr} goalId={goal.id} editMode={editMode} onReload={onReload} accent={accent} />
              </div>
            );
          })}
          {editMode === 'full' && <button onClick={() => setNewKR(true)} style={{ marginTop: 10, padding: '7px 14px', border: '1px dashed #d1d5db', borderRadius: 7, background: 'transparent', color: '#6b7280', fontSize: 12, cursor: 'pointer' }}>+ Добавить KR</button>}
        </div>
      )}
      {newKR && <KREditModal kr={null} goalId={goal.id} onSave={() => { setNewKR(false); onReload(); }} onClose={() => setNewKR(false)} accent={accent} />}
      {confirmDeleteGoal && <ConfirmModal title="Удалить цель?" message={`«${goal.title}» и все её Key Results будут удалены без возможности восстановления.`} onConfirm={handleDeleteGoal} onClose={() => setConfirmDeleteGoal(false)} />}
      {showCom && (
        <div style={{ padding: '0 18px 16px', borderTop: '1px solid #f3f4f6' }}>
          <CommentsPanel comments={goal.comments} onAdd={addGoalComment} me={me} />
        </div>
      )}
    </div>
  );
}

// ── GOAL MODAL ────────────────────────────────────────────────────────────────
function GoalModal({ goal, teamId, periodId, teamName, periodName, existingGoals, me, onSave, onClose, accent, allTeams }) {
  const isEdit = !!goal;
  const usedWeight = (existingGoals || []).filter(g => !isEdit || g.id !== goal?.id).reduce((s, g) => s + g.weight, 0);
  const wasShared = isEdit && (goal.shareTeams || []).filter(t => t.id !== teamId).length > 0;
  const [form, setForm] = useState(goal ? { shareTeamIds: (goal.shareTeams || []).filter(t => t.id !== teamId).map(t => t.id), ...goal, shared: wasShared } : {
    title: '', desc: '', priority: 'P1', weight: Math.min(20, 100 - usedWeight),
    type: 'delivery', focus: 'PROFITABILITY', shared: false, shareTeamIds: [], ownerText: '',
  });
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
        if (wasShared && !form.shared) {
          await apiDelete(`/api/v1/goals/${goal.id}/share/${teamId}`);
          onSave();
          return;
        }
        const fd = new FormData();
        fd.append('title', form.title.trim());
        fd.append('description', form.desc || '');
        fd.append('priority', form.priority);
        fd.append('weight', String(form.weight));
        fd.append('work_type', form.type === 'delivery' ? 'Delivery' : 'Discovery');
        fd.append('focus_type', form.focus);
        fd.append('owner_text', form.ownerText || '');
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
          focus_type: form.focus, owner_text: form.ownerText || '',
        });
      }
      onSave();
    } catch (e) { alert('Ошибка: ' + e.message); }
    finally { setSaving(false); }
  };
  return (
    <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', zIndex: 400, display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 20 }} onClick={onClose}>
      <div onClick={e => e.stopPropagation()} style={{ background: 'white', borderRadius: 16, width: 600, maxHeight: '92vh', overflow: 'auto', boxShadow: '0 24px 80px rgba(0,0,0,0.3)' }}>
        <div style={{ padding: '22px 28px 16px', borderBottom: '1px solid #f3f4f6', position: 'sticky', top: 0, background: 'white', zIndex: 1, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <div>
            <div style={{ fontSize: 18, fontWeight: 700, color: '#111827' }}>{isEdit ? 'Редактировать цель' : 'Новая цель'}</div>
            <div style={{ fontSize: 12, color: '#6b7280', marginTop: 2 }}>{periodName} · {teamName}</div>
          </div>
          <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', fontSize: 22, color: '#9ca3af', padding: 4 }}>×</button>
        </div>
        <div style={{ padding: '22px 28px' }}>
          <div style={{ marginBottom: 16 }}>
            <FieldLabel required hint="Objective — качественное описание того, чего команда хочет достичь. Без цифр (они в KR).">Название</FieldLabel>
            <input value={form.title} onChange={e => set('title', e.target.value)} placeholder="Чего хотим достичь?" style={{ width: '100%', padding: '11px 13px', borderRadius: 9, border: '1.5px solid #e5e7eb', fontSize: 14, outline: 'none', color: '#111827' }} />
          </div>
          <div style={{ marginBottom: 16 }}>
            <FieldLabel hint="Контекст, почему эта цель важна. Не дублируйте название.">Описание</FieldLabel>
            <textarea value={form.desc || ''} onChange={e => set('desc', e.target.value)} rows={3} placeholder="Дополнительный контекст…" style={{ width: '100%', padding: '10px 13px', borderRadius: 9, border: '1.5px solid #e5e7eb', fontSize: 14, outline: 'none', color: '#111827', lineHeight: 1.5 }} />
          </div>
          <div style={{ display: 'flex', gap: 14, marginBottom: 16 }}>
            <div style={{ flex: 1 }}>
              <FieldLabel hint="Относительная важность: P0 — must-have, P1 — высокий приоритет, P2 — важная, P3 — желательная.">Приоритет</FieldLabel>
              <div style={{ display: 'flex', gap: 6 }}>
                {['P0', 'P1', 'P2', 'P3'].map(p => {
                  const c = { P0: '#dc2626', P1: '#d97706', P2: '#2563eb', P3: '#6b7280' }[p]; const sel = form.priority === p; return (
                    <button key={p} onClick={() => set('priority', p)} style={{ flex: 1, padding: '9px 0', borderRadius: 8, border: `2px solid ${sel ? c : '#e5e7eb'}`, background: sel ? `${c}12` : 'white', color: sel ? c : '#6b7280', fontSize: 13, fontWeight: 700, cursor: 'pointer' }}>{p}</button>
                  );
                })}
              </div>
            </div>
            <div style={{ width: 140 }}>
              <FieldLabel hint="Доля цели в общем результате команды. Сумма весов = 100%.">
                <span>Вес <span style={{ fontWeight: 400, color: overWeight ? '#ef4444' : '#9ca3af', fontSize: 12 }}>({totalAfter}/100)</span></span>
              </FieldLabel>
              <div style={{ position: 'relative' }}>
                <input type="number" min={1} max={100} value={form.weight} onChange={e => set('weight', Math.max(1, Math.min(100, Number(e.target.value))))}
                  style={{ width: '100%', padding: '10px 30px 10px 13px', borderRadius: 9, border: `1.5px solid ${overWeight ? '#ef4444' : '#e5e7eb'}`, fontSize: 14, outline: 'none', color: '#111827' }} />
                <span style={{ position: 'absolute', right: 10, top: '50%', transform: 'translateY(-50%)', fontSize: 12, color: '#9ca3af' }}>%</span>
              </div>
              {overWeight && <div style={{ fontSize: 11, color: '#ef4444', marginTop: 3 }}>Превышает 100%</div>}
            </div>
          </div>
          <div style={{ display: 'flex', gap: 14, marginBottom: 16 }}>
            <div style={{ flex: 1 }}>
              <FieldLabel hint="Delivery — известный результат. Discovery — исследование гипотезы.">Тип работы</FieldLabel>
              <div style={{ display: 'flex', gap: 6 }}>
                {['delivery', 'discovery'].map(t => {
                  const sel = form.type === t; return (
                    <button key={t} onClick={() => set('type', t)} style={{ flex: 1, padding: '9px', borderRadius: 8, border: `2px solid ${sel ? accent : '#e5e7eb'}`, background: sel ? `${accent}10` : 'white', color: sel ? accent : '#6b7280', fontSize: 13, fontWeight: 600, cursor: 'pointer', textTransform: 'capitalize' }}>{t}</button>
                  );
                })}
              </div>
            </div>
            <div style={{ flex: 1 }}>
              <FieldLabel>Фокус</FieldLabel>
              <select value={form.focus} onChange={e => set('focus', e.target.value)} style={{ width: '100%', padding: '10px 13px', borderRadius: 9, border: '1.5px solid #e5e7eb', fontSize: 13, outline: 'none', color: '#111827', background: 'white', cursor: 'pointer' }}>
                {FOCUS_OPTIONS.map(f => <option key={f} value={f}>{f}</option>)}
              </select>
            </div>
          </div>
          <div style={{ marginBottom: 16 }}>
            <FieldLabel>Владелец</FieldLabel>
            <input value={form.ownerText || ''} onChange={e => set('ownerText', e.target.value)} placeholder="Имя или роль" style={iSt} />
          </div>
          <div style={{ padding: '12px 14px', background: '#f9fafb', borderRadius: 10, border: '1px solid #f0f1f3' }}>
            <label style={{ display: 'flex', alignItems: 'center', gap: 10, cursor: 'pointer' }}>
              <div onClick={() => set('shared', !form.shared)} style={{ width: 38, height: 22, borderRadius: 11, background: form.shared ? accent : '#d1d5db', position: 'relative', flexShrink: 0, cursor: 'pointer' }}>
                <div style={{ position: 'absolute', top: 3, left: form.shared ? 18 : 3, width: 16, height: 16, borderRadius: '50%', background: 'white', transition: 'left .15s', boxShadow: '0 1px 3px rgba(0,0,0,0.2)' }} />
              </div>
              <div style={{ flex: 1 }}>
                <div style={{ fontSize: 13, fontWeight: 600, color: '#374151' }}>Общая цель</div>
                <div style={{ fontSize: 11, color: '#9ca3af' }}>Разделить цель с другими командами</div>
              </div>
            </label>
            {form.shared && (
              <div style={{ marginTop: 12, paddingTop: 12, borderTop: '1px dashed #e5e7eb' }}>
                <div style={{ fontSize: 12, fontWeight: 600, color: '#374151', marginBottom: 6 }}>С какими командами <span style={{ color: '#ef4444' }}>*</span></div>
                <TeamCombobox selectedIds={form.shareTeamIds || []} onChange={ids => set('shareTeamIds', ids)} excludeId={teamId} accent={accent} allTeams={allTeams} />
              </div>
            )}
          </div>
        </div>
        <div style={{ padding: '14px 28px 22px', borderTop: '1px solid #f3f4f6', display: 'flex', gap: 10, justifyContent: 'flex-end', position: 'sticky', bottom: 0, background: 'white' }}>
          <button onClick={onClose} style={{ padding: '10px 20px', border: '1px solid #e5e7eb', borderRadius: 9, background: 'white', color: '#374151', fontSize: 14, cursor: 'pointer' }}>Отмена</button>
          <button onClick={save} disabled={!valid || saving}
            style={{ padding: '10px 28px', border: 'none', borderRadius: 9, background: (valid && !saving) ? accent : '#e5e7eb', color: (valid && !saving) ? 'white' : '#9ca3af', fontSize: 14, fontWeight: 700, cursor: (valid && !saving) ? 'pointer' : 'default' }}>
            {saving ? 'Сохраняем…' : isEdit ? 'Сохранить' : 'Создать цель'}
          </button>
        </div>
      </div>
    </div>
  );
}

// ── SIDEBAR NODE (uses API hierarchy tree) ────────────────────────────────────
function SidebarNode({ node, depth, selectedId, onSelect, expanded, toggle, accent }) {
  const ch = node.children || [];
  const isExp = expanded[node.id] !== false;
  const isSel = selectedId === node.id;
  const prog = node.progress;
  const health = healthOf(prog, false);
  const hC = HEALTH_COLOR[health];
  const pad = 14 + depth * 13;
  return (
    <div>
      <div onClick={() => onSelect(node.id)}
        style={{ display: 'flex', alignItems: 'center', gap: 5, padding: `5px 10px 5px ${pad}px`, cursor: 'pointer', background: isSel ? `${accent}25` : 'transparent', borderLeft: isSel ? `2px solid ${accent}` : '2px solid transparent', transition: 'background .1s' }}
        onMouseEnter={e => { if (!isSel) e.currentTarget.style.background = 'rgba(255,255,255,0.06)'; }}
        onMouseLeave={e => { if (!isSel) e.currentTarget.style.background = 'transparent'; }}>
        {ch.length > 0
          ? <span onClick={e => { e.stopPropagation(); toggle(node.id); }} style={{ color: '#4b5563', fontSize: 9, width: 11, textAlign: 'center', flexShrink: 0 }}>{isExp ? '▾' : '▸'}</span>
          : <span style={{ width: 11 }} />}
        <span style={{ width: 6, height: 6, borderRadius: '50%', background: hC, flexShrink: 0 }} />
        <span style={{ flex: 1, fontSize: depth === 0 ? 13 : 12, fontWeight: depth === 0 ? 700 : depth === 1 ? 500 : 400, color: isSel ? '#c4b5fd' : depth === 0 ? '#f1f5f9' : depth === 1 ? '#cbd5e1' : '#94a3b8', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', lineHeight: 1.3 }}>{node.name}</span>
        {prog != null && <span style={{ fontSize: 10, color: isSel ? '#c4b5fd' : hC, flexShrink: 0, fontWeight: 600 }}>{prog}%</span>}
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
    <div onClick={() => onSelect(item.team.id)}
      style={{ background: 'white', borderRadius: 10, padding: '14px 16px', cursor: 'pointer', border: '1px solid #f0f1f3', boxShadow: '0 1px 2px rgba(0,0,0,0.05)', transition: 'all .14s' }}
      onMouseEnter={e => { e.currentTarget.style.boxShadow = '0 4px 14px rgba(0,0,0,0.1)'; e.currentTarget.style.transform = 'translateY(-1px)'; }}
      onMouseLeave={e => { e.currentTarget.style.boxShadow = '0 1px 2px rgba(0,0,0,0.05)'; e.currentTarget.style.transform = 'none'; }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 10, gap: 8 }}>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ fontSize: 13, fontWeight: 600, color: '#111827', lineHeight: 1.3, marginBottom: 2, overflow: 'hidden', textOverflow: 'ellipsis' }}>{item.team.name}</div>
          {item.team.lead && <div style={{ fontSize: 11, color: '#6b7280', display: 'flex', alignItems: 'center', gap: 4, marginBottom: 2 }}><Avatar name={item.team.lead} size={14} /><span>{item.team.lead}</span></div>}
        </div>
        <span style={{ fontSize: 10, fontWeight: 700, color: hC, background: `${hC}15`, padding: '2px 6px', borderRadius: 4, flexShrink: 0 }}>{HEALTH_LABEL[health]}</span>
      </div>
      {goalsCount > 0 ? (
        <>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: 5 }}>
            <span style={{ fontSize: 11, color: '#6b7280' }}>{goalsCount} {goalsCount === 1 ? 'цель' : goalsCount < 5 ? 'цели' : 'целей'} · <span style={{ color: '#9ca3af' }}>{item.status_label}</span></span>
            <span style={{ fontSize: 14, fontWeight: 800, color: hC }}>{prog ?? 0}%</span>
          </div>
          <ProgressBar value={prog || 0} forecast={forecast} h={5} color={hC} />
          {highPri > 0 && <div style={{ marginTop: 6, fontSize: 11, color: '#d97706', fontWeight: 500 }}>● {highPri} приоритетных P0–P1</div>}
        </>
      ) : (
        <div style={{ fontSize: 12, color: '#9ca3af', fontStyle: 'italic' }}>Цели не добавлены</div>
      )}
    </div>
  );
}

// ── CLUSTER VIEW ──────────────────────────────────────────────────────────────
function ClusterView({ overview, onSelect, accent }) {
  if (!overview) return <div style={{ textAlign: 'center', padding: '60px 0', color: '#9ca3af' }}>Загрузка…</div>;
  const avg = overview.average_progress || 0;
  const avgForecast = overview.progress_meta?.forecast ?? null;
  const hC = HEALTH_COLOR[healthOf(avg, false, avgForecast)];
  const items = overview.children_summary?.items || [];
  return (
    <div>
      <div style={{ background: 'white', borderRadius: 12, padding: '20px 24px', marginBottom: 20, boxShadow: '0 1px 3px rgba(0,0,0,0.05)', border: '1px solid #f0f1f3', display: 'flex', alignItems: 'center', gap: 24 }}>
        <div style={{ minWidth: 80 }}>
          <div style={{ fontSize: 11, color: '#9ca3af', fontWeight: 500, marginBottom: 4, textTransform: 'uppercase', letterSpacing: .5 }}>Прогресс</div>
          <div style={{ fontSize: 34, fontWeight: 800, color: hC, letterSpacing: -1 }}>{avg}%</div>
        </div>
        <div style={{ flex: 1 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 6 }}>
            <span style={{ fontSize: 12, color: '#9ca3af' }}>{overview.teams_with_goals} из {items.length} с целями</span>
            <span style={{ fontSize: 12, fontWeight: 700, color: hC }}>{HEALTH_LABEL[healthOf(avg, false, avgForecast)]}</span>
          </div>
          {overview.progress_meta && <><ProgressBar value={avg} forecast={overview.progress_meta.forecast} h={10} color={hC} />
            <div style={{ fontSize: 10, color: '#9ca3af', marginTop: 4 }}>прогноз {overview.progress_meta.forecast}%</div></>}
        </div>
      </div>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill,minmax(200px,1fr))', gap: 10 }}>
        {items.map(item => <ChildCard key={item.team.id} item={item} onSelect={onSelect} />)}
      </div>
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
  const [overview, setOverview] = useState(null);
  const [expanded, setExpanded] = useState({});
  const [goalModal, setGoalModal] = useState(null);
  const [accent] = useState(ACCENT);

  // Load me + periods on mount
  useEffect(() => {
    Promise.all([apiGet('/api/v1/me'), apiGet('/api/v1/periods')]).then(([meData, perData]) => {
      if (meData) setMe(meData);
      const items = (perData?.items || []);
      setPeriods(items);
      if (items.length > 0) {
        const saved = localStorage.getItem('okr_period');
        const found = items.find(p => String(p.id) === saved);
        setPeriodId(found ? found.id : items[0].id);
      }
      setLoading(false);
    }).catch(() => setLoading(false));
  }, []);

  // Load hierarchy when period changes
  useEffect(() => {
    if (!periodId) return;
    apiGet(`/api/v1/hierarchy?period_id=${periodId}`).then(data => {
      if (!data) return;
      const nodes = data.items || [];
      setHierarchy(nodes);
      // Auto-select first leaf
      if (!selId) {
        const firstNode = findFirstNode(nodes);
        if (firstNode) setSelId(firstNode.id);
      }
    });
  }, [periodId]);

  // Load team OKR when selection changes
  useEffect(() => {
    if (!periodId || !selId) return;
    setTeamOKR(null); setOverview(null);
    apiGet(`/api/v1/teams/${selId}/okrs?period_id=${periodId}`).then(data => {
      if (data) setTeamOKR(data);
    });
    const node = findNodeById(hierarchy, selId);
    if (node && (node.children || []).length > 0) {
      apiGet(`/api/v1/teams/${selId}/overview?period_id=${periodId}`).then(data => {
        if (data) setOverview(data);
      }).catch(() => { });
    }
  }, [periodId, selId]);

  const findFirstNode = (nodes) => {
    for (const n of nodes) { if (!n.children || n.children.length === 0) return n; const c = findFirstNode(n.children || []); if (c) return c; }
    return nodes[0] || null;
  };
  function findNodeById(nodes, id) {
    for (const n of nodes) { if (n.id === id) return n; const f = findNodeById(n.children || [], id); if (f) return f; }
    return null;
  }
  const toggle = useCallback(id => setExpanded(m => ({ ...m, [id]: m[id] === false ? true : !m[id] })), []);
  const selectTeam = useCallback(id => { setSelId(id); localStorage.setItem('okr_team', id); }, []);
  const handlePeriodChange = id => { setPeriodId(Number(id)); setSelId(null); localStorage.setItem('okr_period', id); };

  const reload = useCallback(() => {
    if (!periodId || !selId) return;
    apiGet(`/api/v1/teams/${selId}/okrs?period_id=${periodId}`).then(data => { if (data) setTeamOKR(data); });
    apiGet(`/api/v1/hierarchy?period_id=${periodId}`).then(data => {
      if (!data) return;
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
    try { await apiPost(`/api/v1/teams/${selId}/status`, { period_id: periodId, status: newStatus }); reload(); } catch (e) { alert('Ошибка: ' + e.message); }
  };

  const [dragState, setDragState] = useState({ srcId: null });

  const goals = (teamOKR?.goals || []).map(mapGoal);

  const handleReorderGoals = useCallback(async (fromId, toId) => {
    if (!fromId || !toId || fromId === toId) return;
    const currentGoals = (teamOKR?.goals || []).map(mapGoal);
    const fi = currentGoals.findIndex(g => g.id === fromId), ti = currentGoals.findIndex(g => g.id === toId);
    if (fi < 0 || ti < 0) return;
    const dir = fi > ti ? 'move-up' : 'move-down';
    const steps = Math.abs(fi - ti);
    for (let i = 0; i < steps; i++)await apiPost(`/api/v1/goals/${fromId}/${dir}`, {});
    reload();
  }, [teamOKR, reload]);
  const handleReorderKRs = useCallback(async (goalId, fromId, toId) => {
    const currentGoals = (teamOKR?.goals || []).map(mapGoal);
    const g = currentGoals.find(x => x.id === goalId); if (!g) return;
    const krs = g.krs || [];
    const fi = krs.findIndex(k => k.id === fromId), ti = krs.findIndex(k => k.id === toId);
    if (fi < 0 || ti < 0 || fi === ti) return;
    const dir = fi > ti ? 'move-up' : 'move-down';
    const steps = Math.abs(fi - ti);
    for (let i = 0; i < steps; i++)await apiPost(`/api/v1/krs/${fromId}/${dir}`, {});
    reload();
  }, [teamOKR, reload]);

  if (loading) return <div style={{ height: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', fontFamily: 'Inter,sans-serif', color: '#6b7280' }}>Загрузка…</div>;

  const curPeriod = periods.find(p => p.id === periodId);
  const status = teamOKR?.period_status || 'no_goals';
  const hasGoals = (teamOKR?.goals_count || 0) > 0;
  const editMode = status === 'forming' || status === 'ready' || status === 'no_goals' ? 'full' : status === 'in_progress' ? 'progress_only' : 'comments_only';
  const isLeafNode = selId && !hierarchy.some(n => hasChild(n, selId));

  function hasChild(node, id) { return (node.children || []).some(c => c.id === id || hasChild(c, id)); }

  return (
    <div style={{ display: 'flex', height: '100vh', overflow: 'hidden', fontFamily: 'Inter,sans-serif' }}>
      {/* SIDEBAR */}
      <div style={{ width: 252, background: '#0c1220', display: 'flex', flexDirection: 'column', flexShrink: 0, overflow: 'hidden' }}>
        <div style={{ padding: '12px 14px', borderBottom: '1px solid rgba(255,255,255,0.06)', display: 'flex', alignItems: 'center', gap: 10 }}>
          <div style={{ fontSize: 14, fontWeight: 800, color: 'white', letterSpacing: -.3, flex: 1 }}>OKR Tracker</div>
          {me && <HeaderUserMenu user={me} accent={accent} />}
        </div>
        <div style={{ padding: '9px 12px', borderBottom: '1px solid rgba(255,255,255,0.06)' }}>
          <div style={{ fontSize: 10, color: '#64748b', fontWeight: 600, textTransform: 'uppercase', letterSpacing: .6, marginBottom: 4 }}>Период</div>
          <select value={periodId || ''} onChange={e => handlePeriodChange(e.target.value)}
            style={{ width: '100%', background: 'rgba(255,255,255,0.07)', border: '1px solid rgba(255,255,255,0.1)', color: '#e5e7eb', borderRadius: 6, padding: '6px 8px', fontSize: 13, outline: 'none', cursor: 'pointer', fontFamily: 'Inter,sans-serif' }}>
            {periods.map(p => <option key={p.id} value={p.id} style={{ background: '#1f2937' }}>{p.name}</option>)}
          </select>
        </div>
        <div style={{ flex: 1, overflow: 'auto', padding: '6px 0' }}>
          {hierarchy.map(n => <SidebarNode key={n.id} node={n} depth={0} selectedId={selId} onSelect={selectTeam} expanded={expanded} toggle={toggle} accent={accent} />)}
        </div>
      </div>

      {/* MAIN */}
      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden', background: '#edf0f4' }}>
        <div style={{ padding: '0 24px', background: 'white', borderBottom: '1px solid #e5e7eb', display: 'flex', alignItems: 'center', height: 54, gap: 14, flexShrink: 0 }}>
          <span style={{ fontSize: 17, fontWeight: 800, color: '#111827', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{teamOKR?.team?.name || 'Выберите команду'}</span>
          {teamOKR?.team?.type && <Badge label={(TEAM_TYPE_LABEL[teamOKR.team.type] || teamOKR.team.type)} color={TEAM_TYPE_COLOR[teamOKR.team.type] || '#6b7280'} />}
          {teamOKR?.team?.lead && (
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, padding: '3px 8px 3px 4px', background: '#f3f4f6', borderRadius: 20, flexShrink: 0 }}>
              <Avatar name={teamOKR.team.lead} size={22} />
              <span style={{ fontSize: 12, color: '#374151', fontWeight: 500 }}>{teamOKR.team.lead}</span>
              <span style={{ fontSize: 11, color: '#9ca3af' }}>· лид</span>
            </div>
          )}
          <div style={{ flex: 1 }} />
          {hasGoals && teamOKR?.progress_meta && (
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <div style={{ width: 140 }}><ProgressBar value={teamOKR.period_progress || 0} forecast={teamOKR.progress_meta.forecast} h={6} color={HEALTH_COLOR[teamOKR.progress_meta.status === 'above' ? 'ahead' : teamOKR.progress_meta.status === 'below' ? 'below' : 'on_track']} /></div>
              <span style={{ fontSize: 13, fontWeight: 700, color: '#374151', flexShrink: 0 }}>{teamOKR.period_progress || 0}%</span>
            </div>
          )}
          {editMode === 'full' && selId && <button onClick={() => setGoalModal('new')} style={{ padding: '7px 14px', borderRadius: 8, background: accent, color: 'white', border: 'none', fontSize: 13, fontWeight: 600, cursor: 'pointer', flexShrink: 0 }}>+ Добавить цель</button>}
        </div>

        <StatusStepper status={status} hasGoals={hasGoals} onChange={handleChangeStatus} accent={accent} statusChangedAt={teamOKR?.status_changed_at} />

        <div style={{ flex: 1, overflow: 'auto', padding: '20px 24px' }}>
          {overview && (overview.children_summary?.items?.length > 0) && <ClusterView overview={overview} onSelect={selectTeam} accent={accent} />}
          {goals.length === 0 && !overview && <div style={{ textAlign: 'center', padding: '80px 0', color: '#9ca3af' }}>
            <div style={{ fontSize: 36, marginBottom: 12 }}>📋</div>
            <div style={{ fontSize: 15, fontWeight: 600, color: '#6b7280', marginBottom: 6 }}>Цели не добавлены</div>
            <div style={{ fontSize: 13, marginBottom: 20 }}>Начните период с постановки OKR</div>
            {editMode === 'full' && selId && <button onClick={() => setGoalModal('new')} style={{ padding: '10px 22px', borderRadius: 10, background: accent, color: 'white', border: 'none', fontSize: 14, fontWeight: 700, cursor: 'pointer' }}>+ Создать первую цель</button>}
          </div>}

          {overview && (overview.children_summary?.items?.length > 0) && goals.length > 0 && <div style={{ fontSize: 11, fontWeight: 700, color: '#9ca3af', textTransform: 'uppercase', letterSpacing: 0.6, marginTop: 24, marginBottom: 12 }}>Цели этого узла</div>}

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
    </div>
  );
}

ReactDOM.createRoot(document.getElementById('root')).render(<App />);
