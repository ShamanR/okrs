// Personal settings SPA.
// Two sections:
//   1. "Описание команд" — a team lead edits the description of teams they lead
//      and every team nested under them.
//   2. "Мой сайдбар" — pick which hierarchy nodes show up in the tracker sidebar.
//
// Persistence is per-user localStorage (keyed by the current user id), shared with
// tracker.js: descriptions are a local override layer, the sidebar selection drives
// which nodes the tracker renders. Depends on globals from markdown.js: Markdown,
// MarkdownEditor (and React).

const { useState, useCallback, useRef, useEffect, useMemo } = React;

// ── SHARED localStorage CONTRACT (must match tracker.js) ──────────────────────
const SETTINGS_KEYS = {
  desc: uid => `okr_team_desc_overrides:${uid}`,
  sidebar: uid => `okr_sidebar_nodes:${uid}`,
};
function readJSON(key, fallback) {
  try { const v = localStorage.getItem(key); return v == null ? fallback : JSON.parse(v); }
  catch (_) { return fallback; }
}
function writeJSON(key, value) {
  try { localStorage.setItem(key, JSON.stringify(value)); } catch (_) { /* quota / private mode */ }
}

const TEAM_TYPE_LABEL = { department: 'Департамент', cluster: 'Кластер', unit: 'Юнит', group: 'Группа', team: 'Команда', squad: 'Сквад', employee: 'Сотрудник' };
const TEAM_TYPE_COLOR = { department: '#4338ca', cluster: '#7c3aed', unit: '#2563eb', group: '#0891b2', team: '#059669', squad: '#d97706', employee: '#64748b' };
const ACCENT = '#7c3aed';

// ── API helpers ───────────────────────────────────────────────────────────────
function readCSRF() {
  const part = document.cookie.split(';').map(s => s.trim()).find(s => s.startsWith('okr_csrf_token='));
  return part ? decodeURIComponent(part.split('=')[1]) : '';
}
async function apiGet(url) {
  const res = await fetch(url, { credentials: 'include' });
  if (!res.ok) throw new Error('HTTP ' + res.status);
  return res.json();
}
async function apiSend(url, method, body) {
  return fetch(url, {
    method, credentials: 'include',
    headers: { 'X-CSRF-Token': readCSRF(), ...(body !== undefined ? { 'Content-Type': 'application/json' } : {}) },
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
}
const apiPost = (url, body) => apiSend(url, 'POST', body);
const apiDelete = (url) => apiSend(url, 'DELETE');

// ── tree helpers ────────────────────────────────────────────────────────────
function flatten(nodes, depth = 0, out = []) {
  (nodes || []).forEach(n => { out.push({ node: n, depth }); flatten(n.children || [], depth + 1, out); });
  return out;
}
function subtreeIds(node, acc = []) {
  acc.push(node.id);
  (node.children || []).forEach(c => subtreeIds(c, acc));
  return acc;
}
function findNode(nodes, id) {
  for (const n of nodes) { if (n.id === id) return n; const f = findNode(n.children || [], id); if (f) return f; }
  return null;
}

// ── small UI pieces ───────────────────────────────────────────────────────────
function Avatar({ name, avatarUrl, size = 28 }) {
  const initials = (name || '?').trim().split(/\s+/).slice(0, 2).map(w => w[0]).join('').toUpperCase();
  if (avatarUrl) return <img src={avatarUrl} width={size} height={size} className="avatar__img" alt={name || ''} />;
  return <div className="avatar__initials" style={{ width: size, height: size, background: ACCENT, fontSize: size * 0.38 }}>{initials}</div>;
}

function TypeBadge({ type }) {
  const color = TEAM_TYPE_COLOR[type] || '#6b7280';
  return <span className="set-type-badge" style={{ color, background: `${color}15` }}>{(TEAM_TYPE_LABEL[type] || type).toUpperCase()}</span>;
}

// AccountMenu replaced by the shared HeaderNavMenu from header.js.

// ── SECTION: TEAM DESCRIPTIONS ────────────────────────────────────────────────
// A row in the editable, indented hierarchy of teams the user may edit.
function DescriptionRow({ entry, value, onSave }) {
  const { node, depth } = entry;
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(value || '');
  const hasDesc = !!(value && value.trim());
  const color = TEAM_TYPE_COLOR[node.type] || '#6b7280';
  const open = () => { setDraft(value || ''); setEditing(true); };
  const save = () => { onSave(node.id, draft); setEditing(false); };
  return (
    <div className="desc-row" style={{ marginLeft: depth * 22, borderLeftColor: color }}>
      <div className="desc-row__head">
        <div className="desc-row__title">
          <TypeBadge type={node.type} />
          <span className="desc-row__name">{node.name}</span>
        </div>
        {!editing && <button className="desc-row__btn" onClick={open}>Изменить</button>}
      </div>
      {node.lead && <div className="desc-row__lead"><Avatar name={node.lead.display_name} avatarUrl={node.lead.avatar_url} size={18} /><span>{node.lead.display_name} · лид</span></div>}
      {editing ? (
        <div className="desc-row__editor">
          <MarkdownEditor value={draft} onChange={setDraft} rows={4} placeholder="Опишите назначение команды…" textareaClassName="set-textarea" />
          <div className="desc-row__actions">
            <button className="set-btn set-btn--ghost" onClick={() => setEditing(false)}>Отмена</button>
            <button className="set-btn set-btn--primary" onClick={save}>Сохранить</button>
          </div>
        </div>
      ) : hasDesc
        ? <Markdown text={value} className="desc-row__desc" />
        : <div className="desc-row__empty">Описание не задано</div>}
    </div>
  );
}

function DescriptionsSection({ me, hierarchy }) {
  const [overrides, setOverrides] = useState(() => readJSON(SETTINGS_KEYS.desc(me.id), {}));

  // ledAll — every team the user leads (for the "Вы лид" banner, including nested ones).
  // editableRoots — only the top-most led teams; their subtrees are flattened into the
  // editable list so a led team nested under another isn't listed twice.
  const { ledAll, editable } = useMemo(() => {
    const isLead = n => n.lead && me.udid && n.lead.udid === me.udid;
    const all = [];
    const collectAll = nodes => nodes.forEach(n => { if (isLead(n)) all.push(n); collectAll(n.children || []); });
    collectAll(hierarchy);
    const roots = [];
    const collectRoots = nodes => nodes.forEach(n => { if (isLead(n)) roots.push(n); else collectRoots(n.children || []); });
    collectRoots(hierarchy);
    const ed = [];
    roots.forEach(r => flatten([r], 0, ed));
    return { ledAll: all, editable: ed };
  }, [hierarchy, me]);

  const effectiveDesc = node => (overrides[node.id] !== undefined ? overrides[node.id] : (node.description || ''));
  const onSave = useCallback((teamId, text) => {
    setOverrides(prev => { const next = { ...prev, [teamId]: text }; writeJSON(SETTINGS_KEYS.desc(me.id), next); return next; });
  }, [me.id]);

  if (ledAll.length === 0) {
    return (
      <div className="set-panel">
        <div className="set-empty">
          <div className="set-empty__icon">🔒</div>
          <div className="set-empty__title">Редактирование недоступно</div>
          <div className="set-empty__text">Описание можно менять только в командах, где вы являетесь лидом. Сейчас таких команд нет.</div>
        </div>
      </div>
    );
  }

  return (
    <div className="set-panel">
      <div className="set-lead-banner">
        <span className="set-lead-banner__label">Вы лид{ledAll.length > 1 ? ` (${ledAll.length})` : ''}:</span>
        {ledAll.map(r => {
          const color = TEAM_TYPE_COLOR[r.type] || '#6b7280';
          return <span key={r.id} className="set-lead-chip" style={{ borderColor: `${color}55` }}><span className="set-lead-chip__dot" style={{ background: color }} />{r.name}</span>;
        })}
      </div>
      <div className="set-list-head">Доступно для правки · {editable.length}</div>
      <div className="desc-list">
        {editable.map(e => (
          <DescriptionRow key={e.node.id} entry={e} value={effectiveDesc(e.node)} onSave={onSave} />
        ))}
      </div>
    </div>
  );
}

// ── SECTION: SIDEBAR PICKER ───────────────────────────────────────────────────
// A node is "checked" when its whole subtree is selected, "partial" (–) when only
// part of it is, and empty otherwise. Clicking a node cycles: ∅ → full → self-only → ∅.
function subtreeState(node, sel) {
  const ids = subtreeIds(node);
  const n = ids.filter(id => sel.has(id)).length;
  if (n === 0) return 'none';
  if (n === ids.length) return 'full';
  return 'partial';
}

function PickerNode({ node, depth, sel, expandedMap, onToggleExpand, onCycle, forceOpen }) {
  const children = node.children || [];
  const hasChildren = children.length > 0;
  const expanded = forceOpen || (expandedMap[node.id] !== false);
  const state = hasChildren ? subtreeState(node, sel) : (sel.has(node.id) ? 'full' : 'none');
  const color = TEAM_TYPE_COLOR[node.type] || '#6b7280';
  // "N из M" — how many direct children have any selection.
  const selChildren = hasChildren ? children.filter(c => subtreeState(c, sel) !== 'none').length : 0;
  return (
    <div>
      <div className="pick-row" style={{ paddingLeft: 8 + depth * 24 }}>
        {hasChildren
          ? <button className="pick-row__caret" onClick={() => onToggleExpand(node.id)}>{expanded ? '▾' : '▸'}</button>
          : <span className="pick-row__caret-spacer" />}
        <button className={`pick-box pick-box--${state}`} onClick={() => onCycle(node)} aria-label="toggle">
          {state === 'full' ? '✓' : state === 'partial' ? '–' : ''}
        </button>
        <span className="pick-row__dot" style={{ background: color }} />
        <span className="set-type-badge" style={{ color, background: `${color}15` }}>{(TEAM_TYPE_LABEL[node.type] || node.type).toUpperCase()}</span>
        <span className="pick-row__name">{node.name}</span>
        {hasChildren && <span className="pick-row__count">{selChildren} из {children.length}</span>}
      </div>
      {hasChildren && expanded && children.map(c => (
        <PickerNode key={c.id} node={c} depth={depth + 1} sel={sel} expandedMap={expandedMap}
          onToggleExpand={onToggleExpand} onCycle={onCycle} forceOpen={forceOpen} />
      ))}
    </div>
  );
}

function SidebarSection({ me, hierarchy }) {
  const allIds = useMemo(() => { const out = []; hierarchy.forEach(n => subtreeIds(n, out)); return out; }, [hierarchy]);
  // Absence of the stored key means "not configured" → default to everything selected
  // (matching the tracker's show-all default). Once the user edits, we persist.
  const [sel, setSel] = useState(() => {
    const stored = readJSON(SETTINGS_KEYS.sidebar(me.id), null);
    return new Set(Array.isArray(stored) ? stored : allIds);
  });
  const [expandedMap, setExpandedMap] = useState({});
  const [q, setQ] = useState('');

  const persist = useCallback(next => { writeJSON(SETTINGS_KEYS.sidebar(me.id), [...next]); }, [me.id]);
  const update = useCallback(mut => {
    setSel(prev => { const next = new Set(prev); mut(next); persist(next); return next; });
  }, [persist]);

  const onCycle = useCallback(node => {
    const ids = subtreeIds(node);
    update(next => {
      if (ids.length === 1) { next.has(node.id) ? next.delete(node.id) : next.add(node.id); return; }
      const cur = ids.filter(id => next.has(id)).length;
      const full = cur === ids.length;
      const selfOnly = next.has(node.id) && ids.slice(1).every(id => !next.has(id));
      if (full) { ids.slice(1).forEach(id => next.delete(id)); } // keep parent only
      else if (selfOnly) { ids.forEach(id => next.delete(id)); }  // clear all
      else { ids.forEach(id => next.add(id)); }                   // select whole branch
    });
  }, [update]);

  const onToggleExpand = useCallback(id => setExpandedMap(m => ({ ...m, [id]: m[id] === false ? true : false })), []);
  const selectAll = () => update(next => allIds.forEach(id => next.add(id)));
  const clearAll = () => update(next => next.clear());

  // Search: keep nodes that match or have a matching descendant; render them expanded.
  const ql = q.trim().toLowerCase();
  const filtered = useMemo(() => {
    if (!ql) return hierarchy;
    const keep = node => {
      const self = node.name.toLowerCase().includes(ql);
      const kids = (node.children || []).map(keep).filter(Boolean);
      if (self) return { ...node, children: node.children || [] };
      if (kids.length) return { ...node, children: kids };
      return null;
    };
    return hierarchy.map(keep).filter(Boolean);
  }, [ql, hierarchy]);

  // How many nodes are visible in the sidebar = selected OR has a selected descendant.
  const visibleCount = useMemo(() => {
    let count = 0;
    const walk = node => {
      const ids = subtreeIds(node);
      if (ids.some(id => sel.has(id))) count++;
      (node.children || []).forEach(walk);
    };
    hierarchy.forEach(walk);
    return count;
  }, [sel, hierarchy]);

  return (
    <div className="set-panel">
      <div className="pick-toolbar">
        <div className="pick-search">
          <span className="pick-search__icon">⌕</span>
          <input value={q} onChange={e => setQ(e.target.value)} placeholder="Поиск команды…" className="pick-search__input" />
        </div>
        <button className="set-btn set-btn--ghost" onClick={selectAll}>Выбрать все</button>
        <button className="set-btn set-btn--ghost" onClick={clearAll}>Снять все</button>
      </div>
      <div className="pick-head">
        <span className="pick-head__label">Иерархия · {allIds.length}</span>
        <span className="pick-head__counter">
          <strong>Выбрано: {sel.size}</strong> · в сайдбаре {visibleCount} {visibleCount === 1 ? 'узел' : visibleCount < 5 ? 'узла' : 'узлов'}
        </span>
      </div>
      <div className="pick-tree">
        {filtered.length === 0
          ? <div className="pick-empty">Ничего не найдено</div>
          : filtered.map(n => (
            <PickerNode key={n.id} node={n} depth={0} sel={sel} expandedMap={expandedMap}
              onToggleExpand={onToggleExpand} onCycle={onCycle} forceOpen={!!ql} />
          ))}
      </div>
    </div>
  );
}

// ── SECTION: MY SPACES ────────────────────────────────────────────────────────
function SpacesSection() {
  const [rows, setRows] = useState([]);
  const [slug, setSlug] = useState('');
  const [msg, setMsg] = useState('');
  const reload = async () => { try { setRows(await apiGet('/api/v1/session/memberships')); } catch (_) { /* keep prior list */ } };
  useEffect(() => { reload(); }, []);
  async function readErr(res) { try { const j = await res.json(); return j.error || ('Ошибка ' + res.status); } catch { return 'Ошибка ' + res.status; } }
  async function leave(tenantID) {
    setMsg('');
    const res = await apiDelete(`/api/v1/session/memberships/${tenantID}`);
    if (res.status === 204) { reload(); return; }
    setMsg(await readErr(res));
  }
  async function join(e) {
    e.preventDefault();
    setMsg('');
    const res = await apiPost('/api/v1/onboarding/join-request', { slug: slug.trim() });
    if (res.status === 204) { setSlug(''); setMsg('Заявка отправлена'); reload(); return; }
    setMsg(await readErr(res));
  }
  return (
    <div className="set-panel set-spaces">
      {msg && <div className="set-intro" style={{ color: '#b45309' }}>{msg}</div>}
      {rows.length === 0
        ? <div className="set-empty"><div className="set-empty__icon">🏢</div><div className="set-empty__title">Вы пока не состоите ни в одном пространстве</div><div className="set-empty__text">Отправьте заявку по slug ниже.</div></div>
        : <ul className="set-spaces__list" style={{ listStyle: 'none', padding: 0, margin: '0 0 20px' }}>
            {rows.map(m => (
              <li key={m.tenant_id} style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '10px 0', borderBottom: '1px solid #eee' }}>
                <span style={{ fontWeight: 600, minWidth: 160 }}>{m.name}</span>
                <span style={{ color: '#6b7280' }}>{m.slug}</span>
                <span style={{ color: '#6b7280' }}>{m.role}</span>
                <span style={{ marginLeft: 'auto', color: m.status === 'active' ? '#047857' : '#b45309' }}>
                  {m.status === 'active' ? 'Активен' : 'Заявка отправлена'}
                </span>
                <button onClick={() => leave(m.tenant_id)}>
                  {m.status === 'active' ? 'Выйти' : 'Отменить заявку'}
                </button>
              </li>
            ))}
          </ul>}
      <form onSubmit={join} style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
        <input value={slug} onChange={e => setSlug(e.target.value)} placeholder="slug пространства" />
        <button type="submit" disabled={!slug.trim()}>Отправить заявку</button>
      </form>
    </div>
  );
}

// ── APP SHELL ─────────────────────────────────────────────────────────────────
const SECTION_META = {
  descriptions: { label: 'Описание команд', hint: 'Только команды, где вы лид', icon: '📝' },
  sidebar: { label: 'Мой сайдбар', hint: 'Какие узлы показывать', icon: '☰' },
  spaces: { label: 'Мои пространства', hint: 'Тенанты и заявки', icon: '🏢' },
};
const SECTION_KEY = 'okr_settings_section';
function readSectionFromURL() {
  const s = new URLSearchParams(window.location.search).get('section');
  return SECTION_META[s] ? s : null;
}

function App() {
  const [me, setMe] = useState(null);
  const [hierarchy, setHierarchy] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const [section, setSection] = useState(() => readSectionFromURL() || localStorage.getItem(SECTION_KEY) || 'descriptions');

  useEffect(() => {
    (async () => {
      try {
        const [meData, perData] = await Promise.all([apiGet('/api/v1/me'), apiGet('/api/v1/periods')]);
        setMe(meData);
        const periods = perData?.items || [];
        if (periods.length === 0) { setLoading(false); return; }
        const today = new Date().toISOString().slice(0, 10);
        const cur = periods.find(p => p.start_date <= today && today <= p.end_date) || periods[0];
        const h = await apiGet(`/api/v1/hierarchy?period_id=${cur.id}`);
        setHierarchy(h?.items || []);
      } catch (_) { setError(true); }
      finally { setLoading(false); }
    })();
  }, []);

  const isLead = useMemo(() => {
    if (!me?.udid) return false;
    let found = false;
    const walk = nodes => nodes.forEach(n => { if (n.lead && n.lead.udid === me.udid) found = true; walk(n.children || []); });
    walk(hierarchy);
    return found;
  }, [me, hierarchy]);

  // Available sections — descriptions only when the user actually leads a team.
  const sections = useMemo(() => [...(isLead ? ['descriptions'] : []), 'sidebar', 'spaces'], [isLead]);
  const active = sections.includes(section) ? section : sections[0];

  // Заголовок вкладки: «Профиль {раздел}».
  useEffect(() => {
    const label = SECTION_META[active]?.label;
    document.title = label ? `Профиль ${label}` : 'Профиль';
  }, [active]);

  // Keep the URL (?section=) in sync and support browser back/forward.
  useEffect(() => { localStorage.setItem(SECTION_KEY, active); }, [active]);
  useEffect(() => {
    if (readSectionFromURL() !== active) window.history.replaceState({ section: active }, '', '/settings?section=' + active);
  }, [active]);
  const navigate = useCallback(id => {
    setSection(id);
    window.history.pushState({ section: id }, '', '/settings?section=' + id);
  }, []);
  useEffect(() => {
    const onPop = () => { const s = readSectionFromURL(); if (s) setSection(s); };
    window.addEventListener('popstate', onPop);
    return () => window.removeEventListener('popstate', onPop);
  }, []);

  if (loading) return <div className="loading-screen">Загрузка…</div>;

  const cur = SECTION_META[active];

  return (
    <div className="set-app">
      <aside className="set-sidebar">
        <div className="set-sidebar__header">
          {me && <HeaderNavMenu user={me} active={null} />}
          <div>
            <div className="set-sidebar__logo">OKR Tracker</div>
            <div className="set-sidebar__sub">Настройки</div>
          </div>
        </div>
        <nav className="set-nav">
          <div className="set-nav__label">Разделы</div>
          {sections.map(id => {
            const m = SECTION_META[id];
            return (
              <button key={id} className={`set-nav__item${id === active ? ' set-nav__item--active' : ''}`} onClick={() => navigate(id)}>
                <span className="set-nav__icon">{m.icon}</span>
                <span className="set-nav__body"><span className="set-nav__title">{m.label}</span><span className="set-nav__hint">{m.hint}</span></span>
              </button>
            );
          })}
        </nav>
        <div className="set-sidebar__footer">
          <a href="/" className="set-back-btn"><span className="set-back-btn__arrow">←</span> Вернуться к OKR Tracker</a>
        </div>
      </aside>

      <div className="set-content">
        <header className="set-topbar">
          <a href="/" className="set-topbar__pill"><span>←</span> OKR Tracker</a>
          <span className="set-topbar__divider" />
          <div className="set-breadcrumbs">
            <span className="set-breadcrumbs__root">Настройки</span>
            <span className="set-breadcrumbs__sep">/</span>
            <span className="set-breadcrumbs__cur">{cur.label}</span>
          </div>
        </header>

        <main className="set-main">
          {error
            ? <div className="set-panel"><div className="set-empty"><div className="set-empty__icon">⚠️</div><div className="set-empty__title">Не удалось загрузить данные</div><div className="set-empty__text">Попробуйте обновить страницу.</div></div></div>
            : active === 'descriptions'
              ? (<><p className="set-intro">Вы можете редактировать описание команд, в которых являетесь лидом, а также всех вложенных в них команд.</p><DescriptionsSection me={me} hierarchy={hierarchy} /></>)
              : active === 'spaces'
                ? (<><p className="set-intro">Пространства (тенанты), в которых вы состоите. Можно выйти из пространства, отменить заявку или отправить новую заявку на вступление по slug.</p><SpacesSection /></>)
                : (<><p className="set-intro">Выберите узлы иерархии, которые будут видны в вашем сайдбаре. Можно отметить одну команду, целую ветвь или несколько команд из разных ветвей — родительские узлы показываются автоматически для навигации.</p><SidebarSection me={me} hierarchy={hierarchy} /></>)}
        </main>
      </div>
    </div>
  );
}

ReactDOM.createRoot(document.getElementById('root')).render(<App />);
