const { useState, useEffect, useRef, useCallback } = React;

// ── API (mirrors tracker.js; readCSRF/csrfHeaders come from shared api.js) ──────
async function apiFetch(url, opts = {}) {
  const r = await fetch(url, opts);
  if (!r.ok) {
    if (r.status === 401) { location.href = '/login?next=' + encodeURIComponent(location.pathname); return null; }
    throw new Error(`HTTP ${r.status}`);
  }
  return r.status === 204 ? null : r.json();
}
const apiGet = url => apiFetch(url);

// ── Favorite teams (shared per-user localStorage, same key as tracker) ──────────
const FAV_KEY = uid => `okr_fav_teams:${uid}`;
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
function toggleFavorite(ids, id) {
  const key = favId(id);
  return ids.includes(key) ? ids.filter(x => x !== key) : [...ids, key];
}
function collectFavNodes(nodes, favIds) {
  const byId = new Map();
  const walk = list => (list || []).forEach(n => { byId.set(favId(n.id), n); walk(n.children); });
  walk(nodes);
  return favIds.map(id => byId.get(favId(id))).filter(Boolean);
}

// Persisted filter state (per-user), so reopening the log restores the last view.
const FILTERS_KEY = uid => `okr_activity_filters:${uid}`;
function readFilters(uid) {
  try { const v = localStorage.getItem(FILTERS_KEY(uid)); return v ? JSON.parse(v) : {}; } catch { return {}; }
}
function writeFilters(uid, f) {
  try { localStorage.setItem(FILTERS_KEY(uid), JSON.stringify(f)); } catch { }
}

// ── Period selector (copied from tracker.js) ────────────────────────────────────
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
        {periods.map(p => {
          const s = TRK_PERIOD_STATUS[p.status] || TRK_PERIOD_STATUS.closed;
          return <button key={p.id == null ? 'all' : p.id} type="button"
            className={'period-select__item' + (p.id === periodId ? ' is-selected' : '')}
            onClick={() => { onChange(p.id); setOpen(false); }}>
            <span className="period-select__indent" style={{ width: (p.depth || 0) * 12 }} />
            <span className="period-select__dot" style={{ background: s.dot }} />
            <span className="period-select__item-name">{p.name}</span>
            {p.start_date && <span className="period-select__range">{fmtDateRange(p.start_date, p.end_date)}</span>}
          </button>;
        })}
      </div>
    </>}
  </div>;
}

// ── Team tree with rolled-up activity counts ────────────────────────────────────
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

// ── Feed rendering ──────────────────────────────────────────────────────────────
function buildTargetURL(target) {
  if (!target) return null;
  const p = new URLSearchParams();
  if (target.team_id) p.set('team', target.team_id);
  if (target.period_id) p.set('period', target.period_id);
  if (target.goal_id) p.set('goal', target.goal_id);
  if (target.kr_id) p.set('kr', target.kr_id);
  if (target.comment_id) p.set('comment', target.comment_id);
  return '/?' + p.toString();
}

const CATEGORY_ICON = { progress: '📈', composition: '🧩', status: '🚦', discussion: '💬' };
const CATEGORY_LABEL = { progress: 'Прогресс', composition: 'Состав целей', status: 'Статусы и риски', discussion: 'Обсуждения' };
const STATUS_RU = { no_goals: 'Нет целей', forming: 'Черновик', ready: 'К валидации', in_progress: 'В работе', validated: 'Валидировано', closed: 'Закрыто' };

function fieldsSummary(changed) {
  if (!changed) return null;
  const names = { title: 'название', description: 'описание', weight: 'вес', priority: 'приоритет' };
  const keys = Object.keys(changed).map(k => names[k] || k);
  return keys.length ? <span className="act-was"> ({keys.join(', ')})</span> : null;
}

function eventText(ev, teamNames) {
  const p = ev.payload || {};
  const t = ev.entity_title || '';
  const names = teamNames || {};
  const teamList = ids => (ids || []).map(id => names[id] || `#${id}`).join(', ');
  switch (ev.action) {
    case 'kr_progress': {
      const b = (p.before || {}).progress, a = (p.after || {}).progress;
      const goal = p.goal_title;
      return <>обновил KR «{t}»{goal ? <> цели «{goal}»</> : null} — <b>{a}%</b> <span className="act-was">(было {b}%)</span></>;
    }
    case 'status_changed': {
      const b = STATUS_RU[(p.before || {}).status] || (p.before || {}).status;
      const a = STATUS_RU[(p.after || {}).status] || (p.after || {}).status;
      return <>перевёл цели команды «{t}» в статус <b>«{a}»</b> <span className="act-was">(было «{b}»)</span></>;
    }
    case 'goal_created': return <>создал цель «{t}»</>;
    case 'goal_deleted': return <>удалил цель «{t}»</>;
    case 'kr_created': return <>добавил KR «{t}»</>;
    case 'kr_deleted': return <>удалил KR «{t}»</>;
    case 'goal_shared': {
      const ids = p.shared_with_team_ids || [];
      return <>добавил к общей цели «{t}»{ids.length ? <> команды: <b>{teamList(ids)}</b></> : null}</>;
    }
    case 'goal_unshared': {
      const rem = p.unshared_team_ids;
      const d = p.declined_by_team_id;
      if (rem && rem.length) return <>убрал из общей цели «{t}» команды: <b>{teamList(rem)}</b></>;
      return <>отказался от общей цели «{t}»{d ? <> (команда <b>{names[d] || `#${d}`}</b>)</> : null}</>;
    }
    case 'goal_owner_changed': return <>сменил владельца цели «{t}»</>;
    case 'goal_fields_changed': return <>изменил цель «{t}»{fieldsSummary(p.changed)}</>;
    case 'kr_fields_changed': return <>изменил KR «{t}»{fieldsSummary(p.changed)}</>;
    case 'kr_note_updated': return <>обновил заметку к KR «{t}»</>;
    case 'comment_added': return <>оставил замечание к «{t}»</>;
    case 'reply_added': return <>ответил на замечание к «{t}»</>;
    case 'comment_resolved': return <>отметил замечание к «{t}» решённым</>;
    case 'comment_reopened': return <>переоткрыл замечание к «{t}»</>;
    case 'comment_deleted': return <>удалил замечание к «{t}»</>;
    case 'reply_deleted': return <>удалил ответ на замечание к «{t}»</>;
    default: return <>{ev.action} «{t}»</>;
  }
}

// eventMarkdownBody returns comment/note text to render as Markdown below the summary line, or null.
function eventMarkdownBody(ev) {
  const p = ev.payload || {};
  if (ev.action === 'comment_added' || ev.action === 'reply_added') return p.text || '';
  if (ev.action === 'kr_note_updated') return (p.after || {}).note || '';
  return null;
}

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

function EventRow({ ev, teamNames, periodNames }) {
  const url = buildTargetURL(ev.target);
  const actor = ev.actor || {};
  const name = actor.removed ? 'Бывший участник' : (actor.display_name || '—');
  const initials = (name || '?').trim().charAt(0).toUpperCase();
  const md = eventMarkdownBody(ev);
  return (
    <div className="act-row">
      <div className={`act-row__icon act-row__icon--${ev.category}`}>{CATEGORY_ICON[ev.category] || '•'}</div>
      <div className="act-row__body">
        <div className="act-row__text">
          {actor.avatar_url && !actor.removed
            ? <img className="act-row__avatar" src={actor.avatar_url} alt="" />
            : <span className="act-row__avatar act-row__avatar--fallback">{initials}</span>}
          <b className="act-row__actor">{name}</b> {eventText(ev, teamNames)}
        </div>
        {md && <div className="act-row__md"><Markdown text={md} /></div>}
        <div className="act-row__meta">
          {ev.team_id != null && <span className="act-badge">{teamNames[ev.team_id] || `команда #${ev.team_id}`}</span>}
          {ev.period_id != null && <span className="act-badge act-badge--period">{periodNames[ev.period_id] || `период #${ev.period_id}`}</span>}
          {url && <a className="act-row__link" href={url}>↗ к цели</a>}
        </div>
      </div>
    </div>
  );
}

function Feed({ periodId, teamId, range, setRange, category, setCategory, actorUDID, actorName, setActor,
  favOnly, setFavOnly, q, setQ, favIds, teamNames, periodNames }) {
  const [events, setEvents] = useState([]);
  const [nextCursor, setNextCursor] = useState('');
  const [loading, setLoading] = useState(false);
  const [catCounts, setCatCounts] = useState({ counts: {}, total: 0 });
  const feedRef = useRef(null);
  const sentinelRef = useRef(null);
  const reqIdRef = useRef(0);        // feed request generation — drop responses from superseded filters
  const loadingMoreRef = useRef(false);

  // Favorites are filtered SERVER-SIDE (before LIMIT/cursor) by scoping team_ids to the favorite
  // teams, so pagination, tab counts and shared-goal matching are all correct.
  const favTeamIds = (favIds || []).map(x => String(x));
  const effTeamIds = teamId ? [String(teamId)] : (favOnly ? favTeamIds : []);
  const effTeamKey = effTeamIds.join(',');
  const noResults = favOnly && !teamId && favTeamIds.length === 0; // favorites on, but none chosen

  const baseParams = useCallback(() => {
    const p = new URLSearchParams();
    if (periodId) p.set('period_id', periodId);
    effTeamIds.forEach(id => p.append('team_ids', id));
    if (actorUDID) p.set('actor_udid', actorUDID);
    if (range && range !== 'all') p.set('range', range);
    if (q.trim()) p.set('q', q.trim());
    return p;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [periodId, effTeamKey, actorUDID, range, q]);

  // Tab counters: stable across the selected category (excludes category), same team/filter scope.
  useEffect(() => {
    if (noResults) { setCatCounts({ counts: {}, total: 0 }); return; }
    const p = baseParams();
    apiGet('/api/v1/activity/category-counts' + (p.toString() ? '?' + p : ''))
      .then(d => setCatCounts(d || { counts: {}, total: 0 })).catch(() => { });
  }, [baseParams, noResults]);

  const buildQuery = useCallback((cursor) => {
    const p = baseParams();
    if (category) p.set('category', category);
    if (cursor) p.set('cursor', cursor);
    return p.toString();
  }, [baseParams, category]);

  useEffect(() => {
    loadingMoreRef.current = false;
    const myId = ++reqIdRef.current;
    if (noResults) { setEvents([]); setNextCursor(''); setLoading(false); return; }
    setLoading(true);
    apiGet('/api/v1/activity?' + buildQuery('')).then(d => {
      if (myId !== reqIdRef.current) return; // a newer filter change superseded this request
      setEvents((d && d.items) || []);
      setNextCursor((d && d.next_cursor) || '');
      setLoading(false);
    }).catch(() => { if (myId === reqIdRef.current) setLoading(false); });
  }, [buildQuery, noResults]);

  const loadMore = useCallback(() => {
    if (!nextCursor || loadingMoreRef.current) return;
    loadingMoreRef.current = true;
    const myId = reqIdRef.current;
    apiGet('/api/v1/activity?' + buildQuery(nextCursor)).then(d => {
      loadingMoreRef.current = false;
      if (myId !== reqIdRef.current) return; // filters changed mid-flight → drop this stale page
      setEvents(prev => [...prev, ...((d && d.items) || [])]);
      setNextCursor((d && d.next_cursor) || '');
    }).catch(() => { loadingMoreRef.current = false; });
  }, [nextCursor, buildQuery]);

  // Lazy loading: auto-fetch the next page when the sentinel scrolls into view.
  useEffect(() => {
    const el = sentinelRef.current;
    if (!el || !nextCursor) return;
    const obs = new IntersectionObserver(entries => { if (entries[0].isIntersecting) loadMore(); },
      { root: feedRef.current, rootMargin: '400px' });
    obs.observe(el);
    return () => obs.disconnect();
  }, [nextCursor, loadMore]);

  const shown = events; // favorites are filtered server-side (see effTeamIds)
  const groups = groupByTime(shown);
  const counts = catCounts.counts || {};

  return (
    <div className="act-main">
      <div className="act-topbar">
        <div className="act-tabs">
          <button className={`act-tab${category === '' ? ' act-tab--on' : ''}`} onClick={() => setCategory('')}>Все <span className="act-tab__n">{catCounts.total || 0}</span></button>
          {['progress', 'composition', 'status', 'discussion'].map(c => (
            <button key={c} className={`act-tab${category === c ? ' act-tab--on' : ''}`} onClick={() => setCategory(c)}>
              {CATEGORY_ICON[c]} {CATEGORY_LABEL[c]} <span className="act-tab__n">{counts[c] || 0}</span>
            </button>
          ))}
        </div>
        <div className="act-filters">
          <div className="act-author">
            <UserSelector value={actorUDID} placeholder="Все авторы"
              fetchFn={qq => apiGet('/api/v1/users?q=' + encodeURIComponent(qq))}
              onChange={(udid, nm) => setActor(udid || '', nm || '')} />
          </div>
          <button className={`act-chip${favOnly ? ' act-chip--on' : ''}`} onClick={() => setFavOnly(v => !v)}>★ Избранное</button>
          <div className="act-range">
            {[['all', 'Всё время'], ['today', 'Сегодня'], ['7d', '7 дней'], ['30d', '30 дней']].map(([v, l]) => (
              <button key={v} className={`act-range__btn${range === v ? ' act-range__btn--on' : ''}`} onClick={() => setRange(v)}>{l}</button>
            ))}
          </div>
          <input className="act-search" placeholder="Поиск по событиям…" value={q} onChange={e => setQ(e.target.value)} />
        </div>
      </div>
      <div className="act-feed" ref={feedRef}>
        {loading && events.length === 0 && <div className="act-empty">Загрузка…</div>}
        {!loading && shown.length === 0 && <div className="act-empty">Событий нет</div>}
        {groups.map(([label, list]) => (
          <div key={label} className="act-group">
            <div className="act-group__label">{label}</div>
            {list.map(ev => <EventRow key={ev.id} ev={ev} teamNames={teamNames} periodNames={periodNames} />)}
          </div>
        ))}
        <div ref={sentinelRef} className="act-sentinel" />
        {nextCursor && <button className="act-more" onClick={loadMore}>Показать ещё</button>}
      </div>
    </div>
  );
}

// ── Page ────────────────────────────────────────────────────────────────────────
function flattenTeamNames(nodes, into) {
  (nodes || []).forEach(n => { into[n.id] = n.name; flattenTeamNames(n.children, into); });
  return into;
}

function App() {
  const [me, setMe] = useState(null);
  const [periods, setPeriods] = useState([]);
  const [hierarchy, setHierarchy] = useState([]);
  const [counts, setCounts] = useState({});
  const [expanded, setExpanded] = useState({});
  const [favorites, setFavorites] = useState(null);
  const [restored, setRestored] = useState(false);
  // Filters (persisted + shared with the Feed).
  const [periodId, setPeriodId] = useState(null);
  const [selId, setSelId] = useState(null);
  const [range, setRange] = useState('all');
  const [category, setCategory] = useState('');
  const [actorUDID, setActorUDID] = useState('');
  const [actorName, setActorName] = useState('');
  const [favOnly, setFavOnly] = useState(false);
  const [q, setQ] = useState('');

  // #5: arriving from the tracker with ?period=<id> opens the log for that period (read once).
  const urlPeriodRef = useRef(null);
  if (urlPeriodRef.current === null) {
    const raw = new URLSearchParams(location.search).get('period');
    const n = raw ? Number(raw) : 0;
    urlPeriodRef.current = Number.isFinite(n) ? n : 0;
  }

  useEffect(() => {
    Promise.all([apiGet('/api/v1/me'), apiGet('/api/v1/periods')]).then(([meData, per]) => {
      if (meData) setMe(meData);
      setPeriods((per && per.items) || []);
    });
  }, []);

  // #3: restore persisted filters once the user is known. URL ?period wins over the stored period.
  useEffect(() => {
    if (!me || restored) return;
    const f = readFilters(me.id);
    if (typeof f.category === 'string') setCategory(f.category);
    if (typeof f.actorUDID === 'string') setActorUDID(f.actorUDID);
    if (typeof f.actorName === 'string') setActorName(f.actorName);
    // Seed the user-ref cache so UserSelector renders the persisted author's name (not the raw UDID).
    if (f.actorUDID) _cacheUserRef({ udid: f.actorUDID, display_name: f.actorName || '' });
    if (typeof f.favOnly === 'boolean') setFavOnly(f.favOnly);
    if (typeof f.range === 'string') setRange(f.range);
    if (typeof f.q === 'string') setQ(f.q);
    if (f.selId != null) setSelId(f.selId);
    if (urlPeriodRef.current) setPeriodId(urlPeriodRef.current);
    else if (f.periodId != null) setPeriodId(f.periodId);
    setFavorites(readFavorites(me.id));
    setRestored(true);
  }, [me, restored]);

  // #3: persist filters on change.
  useEffect(() => {
    if (!me || !restored) return;
    writeFilters(me.id, { periodId, selId, range, category, actorUDID, actorName, favOnly, q });
  }, [me, restored, periodId, selId, range, category, actorUDID, favOnly, q]);
  useEffect(() => { if (me && favorites !== null) writeFavorites(me.id, favorites); }, [favorites, me]);

  useEffect(() => {
    const qs = periodId ? `?period_id=${periodId}` : '';
    apiGet('/api/v1/hierarchy' + qs).then(d => setHierarchy((d && d.items) || [])).catch(() => { });
  }, [periodId]);

  useEffect(() => {
    const p = new URLSearchParams();
    if (periodId) p.set('period_id', periodId);
    if (range && range !== 'all') p.set('range', range);
    apiGet('/api/v1/activity/tree-counts' + (p.toString() ? '?' + p : '')).then(d => setCounts((d && d.counts) || {})).catch(() => { });
  }, [periodId, range]);

  const toggle = id => setExpanded(e => ({ ...e, [id]: e[id] === false ? true : false }));
  // #6: clicking a team filters the feed; clicking the selected team again clears the filter.
  const selectTeam = id => setSelId(cur => (cur === id ? null : id));
  const favArr = favorites || [];
  const favSet = new Set(favArr);
  const favNodes = collectFavNodes(hierarchy, favArr);
  const onToggleFav = useCallback(id => setFavorites(f => toggleFavorite(f || [], id)), []);

  const teamNames = flattenTeamNames(hierarchy, {});
  const periodNames = {};
  periods.forEach(p => { periodNames[p.id] = p.name; });

  const periodOptions = [{ id: null, name: 'Все периоды', status: 'active', depth: 0 }, ...periods];

  return (
    <div className="app">
      <Sidebar user={me} active="activity-log"
        beforeSections={
          <div className="sidebar__period">
            <div className="sidebar__period-label">Период</div>
            <PeriodSelect periods={periodOptions} periodId={periodId} onChange={id => setPeriodId(id)} />
          </div>
        }>
        <div className="sidebar__tree">
          {/* Same structure as the tracker: «Команды» → «Избранное» (nested) → «Все команды» → tree.
             Clicking a team filters the feed; clicking it again clears the filter (shows all). */}
          <div className="sidebar__subsection-label">Команды</div>
          {favNodes.length > 0 && <>
            <div className="sidebar__subsection-label"><span className="sidebar__subsection-star">★</span> Избранное · {favNodes.length}</div>
            {favNodes.map(n => <ActivitySidebarNode key={`fav-${n.id}`} node={{ ...n, children: [] }} depth={0} selectedId={selId} onSelect={selectTeam} expanded={expanded} toggle={toggle} counts={counts} favSet={favSet} onToggleFav={onToggleFav} />)}
            <div className="sidebar__subsection-label">Все команды</div>
          </>}
          {hierarchy.map(n => <ActivitySidebarNode key={n.id} node={n} depth={0} selectedId={selId} onSelect={selectTeam} expanded={expanded} toggle={toggle} counts={counts} favSet={favSet} onToggleFav={onToggleFav} />)}
        </div>
      </Sidebar>
      <div className="main">
        <Feed periodId={periodId} teamId={selId} range={range} setRange={setRange}
          category={category} setCategory={setCategory}
          actorUDID={actorUDID} actorName={actorName} setActor={(udid, nm) => { setActorUDID(udid); setActorName(nm); }}
          favOnly={favOnly} setFavOnly={setFavOnly} q={q} setQ={setQ}
          favIds={favArr} teamNames={teamNames} periodNames={periodNames} />
      </div>
    </div>
  );
}

ReactDOM.createRoot(document.getElementById('root')).render(<App />);
