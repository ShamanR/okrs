// Раздел «Дерево целей»: граф зависимостей целей. Данные — GET /api/v1/goal-tree.
const { useState, useEffect, useMemo, useCallback } = React;

function apiGet(path) {
  return fetch(path, { credentials: 'include' }).then(r => {
    if (!r.ok) throw new Error('HTTP ' + r.status);
    return r.json();
  });
}

const CARD = { cardW: 224, cardH: 112, hGap: 32, vGap: 50, bandGap: 64 }; // hGap ≥ 2*groupPadX(16), иначе бокс группы наползает на соседнюю карточку

// Цвет слоя/акцента карточки по глубине периода (совпадает с палитрой бэндов в CSS).
const GT_TIER_COLOR = { 0: '#6366f1', 1: '#14b8a6', 2: '#f59e0b', 3: '#ec4899', u: '#94a3b8' };
function gtTierColor(period) {
  if (!period) return GT_TIER_COLOR.u;
  return GT_TIER_COLOR[Math.min(period.depth, 3)] || GT_TIER_COLOR.u;
}
function gtProgressColor(p) {
  return p >= 50 ? '#16a34a' : p >= 25 ? '#d97706' : '#dc2626';
}
const TEAM_TYPE_COLOR = { department: '#6366f1', cluster: '#0ea5e9', unit: '#14b8a6', group: '#f59e0b', team: '#22c55e', squad: '#ec4899', employee: '#94a3b8' };

// ── Персистентные контролы (localStorage, через общий storage.js) ──────────
const GT_KEYS = {
  crossPeriod: 'okr_goaltree_cross_period',
  hideUnlinked: 'okr_goaltree_hide_unlinked',
  myGoals: 'okr_goaltree_my_goals',
  onlyRoots: 'okr_goaltree_only_roots',
  period: 'okr_goaltree_period',
};
function usePersistentBool(key, def) {
  const [v, setV] = useState(() => {
    const raw = readJSON(key, null);
    return typeof raw === 'boolean' ? raw : def;
  });
  useEffect(() => { writeJSON(key, v); }, [key, v]);
  return [v, setV];
}

// ── Чистые хелперы фильтрации/связности (без React/DOM — тестируемы отдельно) ──

// Связные компоненты по неориентированным рёбрам (parent+child).
function componentsOf(goals) {
  const byId = new Map(goals.map(g => [g.id, g]));
  const seen = new Set();
  const out = [];
  for (const g of goals) {
    if (seen.has(g.id)) continue;
    const stack = [g.id];
    const comp = [];
    while (stack.length) {
      const cur = stack.pop();
      if (seen.has(cur) || !byId.has(cur)) continue;
      seen.add(cur);
      comp.push(cur);
      const node = byId.get(cur);
      for (const nx of node.parent_goal_ids.concat(node.child_goal_ids)) if (!seen.has(nx)) stack.push(nx);
    }
    out.push(comp);
  }
  return out;
}

// Поддерево цели rootId: транзитивно вниз (дети) и вверх (родители) по полному графу allGoals.
function subtreeUpDown(allGoals, rootId) {
  const byId = new Map(allGoals.map(g => [g.id, g]));
  const seen = new Set([rootId]);
  const down = [rootId];
  while (down.length) {
    const c = down.pop();
    for (const ch of (byId.get(c)?.child_goal_ids || [])) if (!seen.has(ch)) { seen.add(ch); down.push(ch); }
  }
  const up = [rootId];
  while (up.length) {
    const c = up.pop();
    for (const p of (byId.get(c)?.parent_goal_ids || [])) if (!seen.has(p)) { seen.add(p); up.push(p); }
  }
  return seen;
}

// Детерминированный порядок фильтров (spec 060 §«Порядок применения фильтров», шаги 2–5;
// id выбранного периода + всех его вложенных (потомков) по дереву периодов (parent_id).
function periodSubtreeIds(periodId, periods) {
  if (periodId == null) return null;
  const children = new Map();
  for (const p of periods) {
    if (p.parent_id != null) {
      if (!children.has(p.parent_id)) children.set(p.parent_id, []);
      children.get(p.parent_id).push(p.id);
    }
  }
  const set = new Set([periodId]);
  const stack = [periodId];
  while (stack.length) {
    const c = stack.pop();
    for (const ch of (children.get(c) || [])) if (!set.has(ch)) { set.add(ch); stack.push(ch); }
  }
  return set;
}

// Фильтр «шаг 1»: оставить цели выбранного периода + вложенных. При crossPeriod — расширить
// транзитивно по связям вверх/вниз (включая цели других периодов). Рёбра пересчитываются
// на присутствующие цели.
function filterByPeriod(data, periodId, periods, crossPeriod) {
  const sub = periodSubtreeIds(periodId, periods);
  if (!sub) return data; // период не выбран → без фильтра
  const base = new Set(data.goals.filter(g => sub.has(g.period_id)).map(g => g.id));
  let keep = base;
  if (crossPeriod) {
    const byId = new Map(data.goals.map(g => [g.id, g]));
    keep = new Set(base);
    const stack = [...base];
    while (stack.length) {
      const g = byId.get(stack.pop());
      if (!g) continue;
      for (const nx of g.parent_goal_ids.concat(g.child_goal_ids)) if (!keep.has(nx)) { keep.add(nx); stack.push(nx); }
    }
  }
  const goals = data.goals.filter(g => keep.has(g.id)).map(g => ({
    ...g,
    parent_goal_ids: g.parent_goal_ids.filter(id => keep.has(id)),
    child_goal_ids: g.child_goal_ids.filter(id => keep.has(id)),
  }));
  return { ...data, goals };
}

// набор периодов — шаг 1 — задаётся запросом к API до вызова этой функции; шаг 6, свёрнутые
// поддеревья, применяется отдельно через applyCollapse после этой функции).
function filterTreeData(data, ctrls, selectedRootId) {
  let goals = data.goals;
  const byId = new Map(goals.map(g => [g.id, g]));

  // 2) скрыть цели без связи
  if (ctrls.hideUnlinked) {
    goals = goals.filter(g => g.parent_goal_ids.length + g.child_goal_ids.length > 0);
  }

  // связные компоненты (по неориентированным рёбрам) — для «Мои цели»
  const comp = componentsOf(goals);

  // 3) «Мои цели»: оставить компоненты, где есть команда с led_by_me
  if (ctrls.myGoals) {
    const ledTeams = new Set(data.teams.filter(t => t.led_by_me).map(t => t.id));
    const keep = new Set();
    for (const cids of comp) {
      if (cids.some(id => byId.get(id) && ledTeams.has(byId.get(id).team_id))) cids.forEach(id => keep.add(id));
    }
    goals = goals.filter(g => keep.has(g.id));
  }

  // 4) фильтр по выбранному корню: поддерево вниз+вверх от rootId на ПОЛНОМ графе,
  // затем пересечение с уже отфильтрованным набором (шаги 2–3 применяются первыми).
  if (selectedRootId != null) {
    const sub = subtreeUpDown(data.goals, selectedRootId);
    goals = goals.filter(g => sub.has(g.id));
  }

  // 5) только корневые (без родителей, но со связью)
  if (ctrls.onlyRoots) {
    goals = goals.filter(g => g.parent_goal_ids.length === 0 && (g.parent_goal_ids.length + g.child_goal_ids.length) > 0);
  }

  // Пересчитать рёбра, чтобы они не ссылались на отфильтрованные цели.
  const present = new Set(goals.map(g => g.id));
  goals = goals.map(g => ({
    ...g,
    parent_goal_ids: g.parent_goal_ids.filter(id => present.has(id)),
    child_goal_ids: g.child_goal_ids.filter(id => present.has(id)),
  }));
  return { ...data, goals };
}

// Потомки, скрываемые при сворачивании узлов из collapsedSet: цель скрывается, если ВСЕ
// её родители — сворачиваемые узлы (входят в collapsedSet) либо уже скрыты по этому же
// правилу (фикс-пойнт добавления). Ни один узел из collapsedSet сам себя не прячет.
function collapsedDescendants(allGoals, collapsedSet) {
  const hide = new Set();
  let changed = true;
  while (changed) {
    changed = false;
    for (const g of allGoals) {
      if (hide.has(g.id) || collapsedSet.has(g.id)) continue;
      if (g.parent_goal_ids.length === 0) continue;
      const allHidden = g.parent_goal_ids.every(p => collapsedSet.has(p) || hide.has(p));
      if (allHidden) { hide.add(g.id); changed = true; }
    }
  }
  return hide;
}

// Шаг 6 фильтров: применить свёрнутые узлы — убрать исключительно-нижестоящие поддеревья
// и скрыть все нисходящие рёбра свёрнутых узлов (даже к детям, оставшимся видимыми через
// другого родителя).
function applyCollapse(data, collapsedSet) {
  if (!collapsedSet || collapsedSet.size === 0) return data;
  const hidden = collapsedDescendants(data.goals, collapsedSet);
  const goals0 = data.goals.filter(g => !hidden.has(g.id));
  const present = new Set(goals0.map(g => g.id));
  const goals = goals0.map(g => ({
    ...g,
    parent_goal_ids: g.parent_goal_ids.filter(id => present.has(id) && !collapsedSet.has(id)),
    child_goal_ids: collapsedSet.has(g.id) ? [] : g.child_goal_ids.filter(id => present.has(id)),
  }));
  return { ...data, goals };
}

// ── Список корневых целей (дерево по иерархии команд) ──────────────────────
function RootPicker({ data, value, onChange }) {
  const teamsById = useMemo(() => new Map(data.teams.map(t => [t.id, t])), [data]);
  const periodsById = useMemo(() => new Map(data.periods.map(p => [p.id, p])), [data]);
  // Ранг команды по DFS-обходу иерархии (родитель раньше детей) — чтобы группы шли деревом.
  const teamOrder = useMemo(() => {
    const byId = new Map(data.teams.map(t => [t.id, t]));
    const children = new Map();
    const roots = [];
    for (const t of data.teams) {
      if (t.parent_id != null && byId.has(t.parent_id)) {
        if (!children.has(t.parent_id)) children.set(t.parent_id, []);
        children.get(t.parent_id).push(t);
      } else {
        roots.push(t);
      }
    }
    const rank = new Map();
    let i = 0;
    const dfs = t => { rank.set(t.id, i++); for (const c of (children.get(t.id) || [])) dfs(c); };
    for (const r of roots) dfs(r);
    return rank;
  }, [data]);
  const depthOf = useMemo(() => {
    const parent = new Map(data.teams.map(t => [t.id, t.parent_id]));
    const cache = new Map();
    const d = id => {
      if (id == null) return 0;
      if (cache.has(id)) return cache.get(id);
      const pd = parent.get(id) != null ? d(parent.get(id)) + 1 : 0;
      cache.set(id, pd);
      return pd;
    };
    return d;
  }, [data]);
  const [open, setOpen] = useState(false);
  const [q, setQ] = useState('');
  const triggerRef = React.useRef(null);
  const [pos, setPos] = useState(null);
  const openPanel = () => {
    const r = triggerRef.current.getBoundingClientRect();
    setPos({ top: r.bottom + 6, left: r.left });
    setOpen(true);
  };
  useEffect(() => {
    if (!open) return;
    const onDoc = e => { if (!e.target.closest('.gt-rootpick')) setOpen(false); };
    const onScroll = () => setOpen(false);
    document.addEventListener('mousedown', onDoc);
    window.addEventListener('scroll', onScroll, true);
    return () => { document.removeEventListener('mousedown', onDoc); window.removeEventListener('scroll', onScroll, true); };
  }, [open]);
  const roots = useMemo(() => data.goals.filter(g => g.parent_goal_ids.length === 0 && g.child_goal_ids.length > 0), [data]);
  const query = q.trim().toLowerCase();
  const rootsByTeam = useMemo(() => {
    const filtered = roots.filter(g => {
      if (!query) return true;
      const t = teamsById.get(g.team_id);
      return g.title.toLowerCase().includes(query) || (t && t.name.toLowerCase().includes(query));
    });
    const m = new Map();
    for (const g of filtered) { if (!m.has(g.team_id)) m.set(g.team_id, []); m.get(g.team_id).push(g); }
    return m;
  }, [roots, query, teamsById]);
  // Показываем команды с корневыми целями И всех их предков (предки — заголовок без целей),
  // чтобы дерево было полным и отступы вложенных команд совпадали с целями родителя.
  const displayTeams = useMemo(() => {
    const set = new Set();
    for (const tid of rootsByTeam.keys()) {
      let cur = tid;
      while (cur != null && teamsById.has(cur)) {
        if (set.has(cur)) break;
        set.add(cur);
        cur = teamsById.get(cur).parent_id;
      }
    }
    return [...set].sort((a, b) => (teamOrder.get(a) ?? 0) - (teamOrder.get(b) ?? 0));
  }, [rootsByTeam, teamsById, teamOrder]);
  const selectedGoal = value != null ? roots.find(g => g.id === value) : null;
  const pick = id => { onChange(id); setOpen(false); setQ(''); };
  return (
    <div className={'gt-rootpick' + (open ? ' gt-rootpick--open' : '')}>
      <button type="button" ref={triggerRef} className="gt-rootpick__trigger" onClick={() => (open ? setOpen(false) : openPanel())}>
        <span className="gt-rootpick__value">{selectedGoal ? selectedGoal.title : 'Все корневые цели'}</span>
        <span className="gt-rootpick__chev">▾</span>
      </button>
      {open && (
        <div className="gt-rootpick__panel" style={pos ? { top: pos.top, left: pos.left } : undefined}>
          <input className="gt-rootpick__search" placeholder="Поиск по цели или команде…"
            value={q} onChange={e => setQ(e.target.value)} autoFocus />
          <div className="gt-rootpick__list">
            <button type="button" className={'gt-rootpick__all' + (value == null ? ' gt-rootpick__all--on' : '')} onClick={() => pick(null)}>
              Все корневые цели
            </button>
            {displayTeams.length === 0 && <div className="gt-rootpick__empty">Ничего не найдено</div>}
            {displayTeams.map(teamId => {
              const t = teamsById.get(teamId);
              const gs = rootsByTeam.get(teamId) || [];
              const TAB = 16;                                 // единый «таб» для команд и целей
              const indent = 8 + depthOf(teamId) * TAB;       // команда глубины D → D табов
              const rowIndent = indent + TAB;                 // её цели → D+1 таб (вложенная команда D+1 совпадёт с ними)
              return (
                <div key={teamId} className="gt-rootpick__group">
                  <div className="gt-rootpick__group-head" style={{ paddingLeft: indent }}>
                    <span className="gt-rootpick__group-dot" style={{ background: TEAM_TYPE_COLOR[t && t.type] || '#94a3b8' }} />
                    <span className="gt-rootpick__group-name">{t ? t.name : ''}</span>
                    <span className="gt-rootpick__group-type">{((t && t.type_label) || '').toUpperCase()}</span>
                  </div>
                  {gs.map(g => {
                    const per = periodsById.get(g.period_id);
                    const sel = g.id === value;
                    return (
                      <button key={g.id} type="button" style={{ paddingLeft: rowIndent }} className={'gt-rootpick__row' + (sel ? ' gt-rootpick__row--on' : '')} onClick={() => pick(g.id)}>
                        <span className="gt-rootpick__row-title">{g.title}</span>
                        <span className="gt-rootpick__row-meta">{per ? per.name : ''} · {g.progress}%</span>
                        {sel && <span className="gt-rootpick__row-check">✓</span>}
                      </button>
                    );
                  })}
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}

// ── Мелкие визуальные помощники детальной панели ───────────────────────────
const GT_PRI_COLOR = { P0: '#dc2626', P1: '#d97706', P2: '#2563eb', P3: '#6b7280' };
// SPEED_EFFICIENCY → "Speed Efficiency" (человекочитаемое отображение UPPER_SNAKE enum).
function gtFocusLabel(f) {
  return String(f || '').split('_').filter(Boolean).map(w => w[0] + w.slice(1).toLowerCase()).join(' ');
}
function GtBadge({ label, color = '#6b7280' }) {
  return <span className="gt-badge" style={{ color, background: `${color}18` }}>{label}</span>;
}

// ── Карточка цели ────────────────────────────────────────────────────────
function GoalTreeCard({ node, teamsById, periodsById, onSelect, highlighted, dimmed, hasChildren, isCollapsed, onToggleCollapse }) {
  const g = node.goal;
  const cls = [
    'gt-card',
    node.isRoot ? 'gt-card--root' : '',
    highlighted ? 'gt-card--hl' : '',
    dimmed ? 'gt-card--dim' : '',
  ].join(' ');
  const team = teamsById.get(g.team_id);
  const period = periodsById.get(g.period_id);
  const accent = gtTierColor(period);
  return (
    <div className={cls} style={{ left: node.x, top: node.y, width: CARD.cardW, height: CARD.cardH, '--gt-accent': accent }}
         onClick={() => onSelect(g.id)}>
      <i className="gt-card__strip" style={{ background: accent }} />
      {hasChildren && (
        <button type="button" className="gt-collapse-btn"
          title={isCollapsed ? 'Развернуть поддерево' : 'Свернуть поддерево'}
          aria-label={isCollapsed ? 'Развернуть поддерево' : 'Свернуть поддерево'}
          onClick={e => { e.stopPropagation(); onToggleCollapse(g.id); }}>
          {isCollapsed ? '▸' : '▾'}
        </button>
      )}
      <div className="gt-card__title">{g.title}</div>
      <div className="gt-card__meta">
        <span className="gt-card__team">
          <span className="gt-card__dot" />
          <span className="gt-card__teamname">{team ? team.name : ''}</span>
        </span>
        {period && <span className="gt-card__period">{period.name}</span>}
        <span className="gt-card__pr" style={{ color: gtProgressColor(g.progress) }}>{g.progress}%</span>
      </div>
    </div>
  );
}

// ── Расширенная карточка (read-only детали + «Скрыть остальные цели») ─────
function GoalDetailPanel({ goal, data, focused, onToggleFocus, onClose }) {
  const teamsById = new Map(data.teams.map(t => [t.id, t]));
  const periodsById = new Map(data.periods.map(p => [p.id, p]));
  const goalsById = new Map(data.goals.map(g => [g.id, g]));
  const team = teamsById.get(goal.team_id);
  const period = periodsById.get(goal.period_id);
  const priColor = GT_PRI_COLOR[goal.priority] || '#6b7280';
  // Путь команды по иерархии (корень → … → команда цели) — кликабельные крошки.
  const pathTeams = [];
  let t = team; const guard = new Set();
  while (t && !guard.has(t.id)) { guard.add(t.id); pathTeams.unshift(t); t = t.parent_id != null ? teamsById.get(t.parent_id) : null; }
  const parents = goal.parent_goal_ids.map(id => goalsById.get(id)).filter(Boolean);
  const deepLink = `/?team=${goal.team_id}&period=${goal.period_id}&goal=${goal.id}`;
  return (
    <div className="gt-panel" role="dialog" aria-label="Детали цели">
      <div className="gt-panel__header">
        <div className="gt-panel__title">{goal.title}</div>
        <button type="button" className="gt-panel__close" aria-label="Закрыть" onClick={onClose}>×</button>
      </div>
      <div className="gt-panel__body">
        {pathTeams.length > 0 && (
          <div className="gt-panel__path">
            {pathTeams.map((pt, i) => (
              <React.Fragment key={pt.id}>
                {i > 0 && <span className="gt-panel__path-sep"> / </span>}
                <a className="gt-panel__path-link" href={`/?team=${pt.id}`} title={`Открыть команду «${pt.name}» в трекере`}>{pt.name}</a>
              </React.Fragment>
            ))}
          </div>
        )}
        <div className="gt-panel__badges">
          {period && <span className="gt-panel__pill">{period.name}</span>}
          <GtBadge label={goal.priority} color={priColor} />
          <span className="gt-panel__muted">вес {goal.weight}%</span>
          <span className="gt-panel__muted">{gtFocusLabel(goal.focus_type)}</span>
        </div>
        <div className="gt-panel__barwrap">
          <div className="gt-panel__bar"><div className="gt-panel__barfill" style={{ width: goal.progress + '%' }} /></div>
          <span className="gt-panel__pct">{goal.progress}%</span>
        </div>
        <div className="gt-panel__links">
          <span className="gt-panel__updown">↑ выше: <b>{goal.parent_goal_ids.length}</b></span>
          <span className="gt-panel__updown">↓ ниже: <b>{goal.child_goal_ids.length}</b></span>
        </div>
        {parents.length > 0 && (
          <div className="gt-panel__section">
            <div className="gt-panel__section-title">Вышестоящие цели</div>
            {parents.map(p => {
              const pt = teamsById.get(p.team_id), pp = periodsById.get(p.period_id);
              return (
                <div key={p.id} className="gt-panel__parent">
                  <div className="gt-panel__parent-title">{p.title}</div>
                  <div className="gt-panel__parent-meta">{pp ? pp.name : ''} · {pt ? pt.name : ''}</div>
                </div>
              );
            })}
          </div>
        )}
        {goal.owner_text && (
          <div className="gt-panel__row">
            <span className="gt-panel__label">Драйвер</span>
            <span className="gt-panel__value">{goal.owner_text}</span>
          </div>
        )}
      </div>
      <div className="gt-panel__footer">
        <button type="button" className={'gt-panel__toggle' + (focused ? ' gt-panel__toggle--on' : '')} onClick={onToggleFocus}>
          {focused ? '◉ Показать все' : '◎ Скрыть остальные'}
        </button>
        <a className="gt-panel__open" href={deepLink}>↗ Открыть в трекере</a>
      </div>
    </div>
  );
}

function GoalTreeCanvas({ data, periodRank, selectedId, onSelect, collapsed, onToggleCollapse, childCountMap }) {
  const teamsById = useMemo(() => new Map(data.teams.map(t => [t.id, t])), [data]);
  const periodsById = useMemo(() => new Map(data.periods.map(p => [p.id, p])), [data]);
  const layout = useMemo(() => computeGoalTreeLayout(data, { ...CARD, periodRank }), [data, periodRank]);

  const viewportRef = React.useRef(null);
  const zoomAnchorRef = React.useRef(null); // точка контента под курсором для зума к курсору
  const panRef = React.useRef(null);        // состояние drag-to-pan
  const [zoom, setZoom] = useState(1);
  const clampZoom = z => Math.max(0.15, Math.min(2.5, z));
  const fit = useCallback(() => {
    const vp = viewportRef.current;
    if (!vp || !layout.width || !layout.height) return;
    const zw = (vp.clientWidth - 32) / layout.width;
    const zh = (vp.clientHeight - 32) / layout.height;
    setZoom(clampZoom(Math.min(1, Math.min(zw, zh)))); // вписать, но не увеличивать сверх 100%
  }, [layout.width, layout.height]);
  // Авто-fit при смене набора данных/раскладки.
  useEffect(() => { fit(); }, [fit]);

  // Зум колесом/щипком (трекпад-щипок приходит как ctrl+wheel) — к точке под курсором.
  // Нативный listener с passive:false, иначе preventDefault не сработает и страница проскроллится.
  useEffect(() => {
    const vp = viewportRef.current;
    if (!vp) return;
    const onWheel = e => {
      e.preventDefault();
      const rect = vp.getBoundingClientRect();
      const ox = e.clientX - rect.left, oy = e.clientY - rect.top;
      setZoom(z => {
        const nz = Math.max(0.15, Math.min(2.5, z * Math.exp(-e.deltaY * 0.0015)));
        zoomAnchorRef.current = { cx: (ox + vp.scrollLeft) / z, cy: (oy + vp.scrollTop) / z, ox, oy };
        return nz;
      });
    };
    vp.addEventListener('wheel', onWheel, { passive: false });
    return () => vp.removeEventListener('wheel', onWheel);
  }, []);

  // После изменения зума — вернуть точку под курсором на место (стабильный зум к курсору).
  React.useLayoutEffect(() => {
    const a = zoomAnchorRef.current, vp = viewportRef.current;
    if (!a || !vp) return;
    vp.scrollLeft = a.cx * zoom - a.ox;
    vp.scrollTop = a.cy * zoom - a.oy;
    zoomAnchorRef.current = null;
  }, [zoom]);

  // Drag-to-pan по пустому фону (не по карточке/контролам) — навигация без колеса.
  const movedRef = React.useRef(false);
  const onPointerDown = e => {
    movedRef.current = false;
    if (e.button !== 0 || e.target.closest('.gt-card') || e.target.closest('.gt-zoom-ctrl')) return;
    const vp = viewportRef.current;
    panRef.current = { x: e.clientX, y: e.clientY, sl: vp.scrollLeft, st: vp.scrollTop };
    vp.classList.add('gt-viewport--panning');
    if (vp.setPointerCapture) vp.setPointerCapture(e.pointerId);
  };
  const onPointerMove = e => {
    const p = panRef.current, vp = viewportRef.current;
    if (!p || !vp) return;
    if (Math.abs(e.clientX - p.x) + Math.abs(e.clientY - p.y) > 4) movedRef.current = true;
    vp.scrollLeft = p.sl - (e.clientX - p.x);
    vp.scrollTop = p.st - (e.clientY - p.y);
  };
  const endPan = () => {
    const vp = viewportRef.current;
    if (panRef.current && vp) vp.classList.remove('gt-viewport--panning');
    panRef.current = null;
  };
  // Клик по пустому фону (не по карточке/контролам и не drag) — снять выделение.
  const onBackgroundClick = e => {
    if (movedRef.current) { movedRef.current = false; return; }
    if (e.target.closest('.gt-card') || e.target.closest('.gt-zoom-ctrl')) return;
    onSelect(null);
  };

  // подсветка: транзитивное дерево связей от выбранной цели (вверх и вниз)
  const highlightSet = useMemo(() => {
    if (selectedId == null) return null;
    const adjUp = new Map(), adjDown = new Map();
    for (const g of data.goals) {
      adjUp.set(g.id, g.parent_goal_ids);
      adjDown.set(g.id, g.child_goal_ids);
    }
    const seen = new Set([selectedId]);
    const walk = (start, adj) => {
      const stack = [start];
      while (stack.length) {
        const cur = stack.pop();
        for (const nx of (adj.get(cur) || [])) if (!seen.has(nx)) { seen.add(nx); stack.push(nx); }
      }
    };
    walk(selectedId, adjUp); walk(selectedId, adjDown);
    return seen;
  }, [selectedId, data]);

  return (
    <div className="gt-canvas-wrap">
      <div className="gt-viewport" ref={viewportRef} onClick={onBackgroundClick}
        onPointerDown={onPointerDown} onPointerMove={onPointerMove} onPointerUp={endPan} onPointerLeave={endPan}>
        <div className="gt-zoom" style={{ width: layout.width * zoom, height: layout.height * zoom }}>
          <div className="gt-canvas" style={{ width: layout.width, height: layout.height, transform: `scale(${zoom})`, transformOrigin: '0 0' }}>
            {layout.bands.map((b, i) => {
              const tier = b.depth == null ? 'u' : 'd' + Math.min(b.depth, 3);
              return (
                <div key={i} className={'gt-band gt-band--' + tier} style={{ top: b.y, height: b.height }}>
                  <span className="gt-band__label">
                    <span className="gt-band__marker">◆</span>
                    <span className="gt-band__name">{b.name}</span>
                    <span className="gt-band__count">{b.count}</span>
                  </span>
                </div>
              );
            })}
            {layout.groups.map((gr, i) => {
              const firstGoal = data.goals.find(g => g.id === gr.nodeIds[0]);
              const dot = gtTierColor(firstGoal ? periodsById.get(firstGoal.period_id) : null);
              const parentGoal = data.goals.find(g => g.id === gr.parentGoalId);
              const rawTitle = parentGoal ? parentGoal.title : gr.teamName;
              const MAX = 32;
              const shortTitle = rawTitle.length > MAX ? rawTitle.slice(0, MAX).trim() + '…' : rawTitle;
              return (
                <div key={'g' + i} className="gt-group" style={{ left: gr.x, top: gr.y, width: gr.w, height: gr.h }}>
                  <span className="gt-group__label" style={{ maxWidth: gr.w - 32 }} title={`Группа целей | ${rawTitle} | ${gr.teamName}`}>
                    <span className="gt-group__dot" style={{ background: dot }} />
                    <span className="gt-group__label-text">Группа целей | {shortTitle} | {gr.teamName}</span>
                  </span>
                </div>
              );
            })}
            <svg className="gt-edges" width={layout.width} height={layout.height}>
              {layout.edges.map((e, i) => {
                const on = !highlightSet || (highlightSet.has(e.from) && highlightSet.has(e.to));
                return <path key={i} d={e.path} className={'gt-edge' + (highlightSet && !on ? ' gt-edge--dim' : '')} fill="none" />;
              })}
            </svg>
            {layout.nodes.map(n => (
              <GoalTreeCard key={n.id} node={n} teamsById={teamsById} periodsById={periodsById} onSelect={onSelect}
                highlighted={!!highlightSet && highlightSet.has(n.id)}
                dimmed={!!highlightSet && !highlightSet.has(n.id)}
                hasChildren={(childCountMap.get(n.id) || 0) > 0}
                isCollapsed={collapsed.has(n.id)}
                onToggleCollapse={onToggleCollapse} />
            ))}
          </div>
        </div>
      </div>
      <div className="gt-zoom-ctrl">
        <button type="button" title="Отдалить" aria-label="Отдалить" onClick={() => setZoom(z => clampZoom(z / 1.2))}>−</button>
        <button type="button" className="gt-zoom-ctrl__num" title="100%" aria-label="Сбросить масштаб" onClick={() => setZoom(1)}>{Math.round(zoom * 100)}%</button>
        <button type="button" title="Приблизить" aria-label="Приблизить" onClick={() => setZoom(z => clampZoom(z * 1.2))}>+</button>
        <button type="button" title="Вписать в экран" aria-label="Вписать в экран" onClick={fit}>⤢</button>
      </div>
    </div>
  );
}

// Компактный селектор периода для сайдбара (в стиле остальных контролов дерева).
const GT_PERIOD_STATUS_COLOR = { future: '#3b82c4', active: '#1f9d55', closed: '#6b7280', archived: '#94a3b8' };
function GtPeriodSelect({ periods, value, onChange }) {
  const [open, setOpen] = useState(false);
  const triggerRef = React.useRef(null);
  const [pos, setPos] = useState(null);
  const cur = (periods || []).find(p => p.id === value);
  const openPanel = () => { const r = triggerRef.current.getBoundingClientRect(); setPos({ top: r.bottom + 6, left: r.left, width: r.width }); setOpen(true); };
  useEffect(() => {
    if (!open) return;
    const onDoc = e => { if (!e.target.closest('.gt-psel')) setOpen(false); };
    const onScroll = () => setOpen(false);
    document.addEventListener('mousedown', onDoc);
    window.addEventListener('scroll', onScroll, true);
    return () => { document.removeEventListener('mousedown', onDoc); window.removeEventListener('scroll', onScroll, true); };
  }, [open]);
  return (
    <div className={'gt-psel' + (open ? ' gt-psel--open' : '')}>
      <button type="button" ref={triggerRef} className="gt-psel__trigger" onClick={() => (open ? setOpen(false) : openPanel())}>
        <span className="gt-psel__dot" style={{ background: GT_PERIOD_STATUS_COLOR[cur && cur.status] || '#94a3b8' }} />
        <span className="gt-psel__value">{cur ? cur.name : '—'}</span>
        <span className="gt-psel__chev">▾</span>
      </button>
      {open && pos && (
        <div className="gt-psel__panel" style={{ top: pos.top, left: pos.left, width: pos.width }}>
          {(periods || []).map(p => (
            <button key={p.id} type="button" className={'gt-psel__item' + (p.id === value ? ' gt-psel__item--on' : '')} onClick={() => { onChange(p.id); setOpen(false); }}>
              <span style={{ width: (p.depth || 0) * 12, flexShrink: 0 }} />
              <span className="gt-psel__dot" style={{ background: GT_PERIOD_STATUS_COLOR[p.status] || '#94a3b8' }} />
              <span className="gt-psel__item-name">{p.name}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

function GoalTreeApp() {
  const [me, setMe] = useState(null);
  const [periods, setPeriods] = useState([]);
  const [periodId, setPeriodId] = useState(null);
  const [periodsLoaded, setPeriodsLoaded] = useState(false);
  const [data, setData] = useState(null);
  const [status, setStatus] = useState('loading'); // loading | error | ready

  const [crossPeriod, setCrossPeriod] = usePersistentBool(GT_KEYS.crossPeriod, false);
  const [hideUnlinked, setHideUnlinked] = usePersistentBool(GT_KEYS.hideUnlinked, true);
  const [myGoals, setMyGoals] = usePersistentBool(GT_KEYS.myGoals, true);
  const [onlyRoots, setOnlyRoots] = usePersistentBool(GT_KEYS.onlyRoots, false);

  const [selectedRootId, setSelectedRootId] = useState(null); // RootPicker / «Скрыть остальные цели»
  const [collapsed, setCollapsed] = useState(() => new Set());
  const [sel, setSel] = useState(null); // выбранная (подсвеченная) цель

  useEffect(() => { document.title = 'Дерево целей'; }, []);

  useEffect(() => {
    Promise.all([apiGet('/api/v1/me'), apiGet('/api/v1/periods')])
      .then(([meResp, periodsResp]) => {
        setMe(meResp);
        const items = (periodsResp && periodsResp.items) || [];
        setPeriods(items);
        if (items.length) {
          const stored = readJSON(GT_KEYS.period, null);
          const valid = stored != null && items.some(p => p.id === stored);
          setPeriodId(valid ? stored : items[0].id);
        }
        setPeriodsLoaded(true);
      })
      .catch(() => setStatus('error'));
  }, []);

  const reload = useCallback(() => {
    if (!periodsLoaded) return;
    setStatus('loading');
    // Тянем ВЕСЬ граф (все непархивированные периоды) один раз; выбор периода и
    // «Связи между периодами» — клиентские фильтры (filterByPeriod), без рефетча.
    apiGet('/api/v1/goal-tree?cross_period=1')
      .then(d => { setData(d); setStatus('ready'); })
      .catch(() => setStatus('error'));
  }, [periodsLoaded]);

  useEffect(() => { reload(); }, [reload]);

  // Смена набора данных (период/cross-period) — сбросить эфемерное состояние выбора,
  // чтобы не ссылаться на id из предыдущего графа.
  useEffect(() => {
    setSel(null);
    setSelectedRootId(null);
    setCollapsed(new Set());
  }, [data, periodId]);

  // Сохранять выбранный период между перезагрузками.
  useEffect(() => { if (periodId != null) writeJSON(GT_KEYS.period, periodId); }, [periodId]);

  const onSelect = useCallback(id => setSel(prev => (prev === id ? null : id)), []);
  const onToggleCollapse = useCallback(id => {
    setCollapsed(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
  }, []);

  // Шаг 1: цели выбранного периода + вложенных (+ связи в др. периоды при crossPeriod).
  const periodData = useMemo(() => {
    if (!data) return null;
    return filterByPeriod(data, periodId, periods, crossPeriod);
  }, [data, periodId, periods, crossPeriod]);

  // Данные для селектора корней: после фильтра периода + «Мои цели»/«Скрыть без связи», но ДО
  // root-фильтра — чтобы пикер предлагал только реально доступные корни (иначе выбор корня из
  // чужого периода/скрытой ветки пересечётся с текущим набором и даст пустой экран).
  const pickerData = useMemo(() => {
    if (!periodData) return null;
    return filterTreeData(periodData, { hideUnlinked, myGoals, onlyRoots: false }, null);
  }, [periodData, hideUnlinked, myGoals]);

  // Если выбранный корень выпал из доступного набора (сменили период/«Мои цели») — сбросить,
  // иначе root-фильтр пересечётся с пустым множеством и граф станет пустым.
  useEffect(() => {
    if (selectedRootId != null && pickerData && !pickerData.goals.some(g => g.id === selectedRootId)) {
      setSelectedRootId(null);
    }
  }, [pickerData, selectedRootId]);

  // Фильтры 2–5 (без свёрнутых поддеревьев — см. spec §«Порядок применения фильтров»).
  const preCollapseData = useMemo(() => {
    if (!periodData) return null;
    return filterTreeData(periodData, { hideUnlinked, myGoals, onlyRoots }, selectedRootId);
  }, [periodData, hideUnlinked, myGoals, onlyRoots, selectedRootId]);

  // Число детей у цели ДО применения collapse — определяет, показывать ли collapse-кнопку.
  const childCountMap = useMemo(() => {
    const m = new Map();
    if (preCollapseData) for (const g of preCollapseData.goals) m.set(g.id, g.child_goal_ids.length);
    return m;
  }, [preCollapseData]);

  // Фильтр 6: свёрнутые поддеревья — основа для раскладки.
  const finalData = useMemo(() => {
    if (!preCollapseData) return null;
    return applyCollapse(preCollapseData, collapsed);
  }, [preCollapseData, collapsed]);

  // Хронологический ранг периода для сортировки целей слева→направо (раньше→левее).
  // /api/v1/periods несёт start_date; ISO-строки сортируются лексикографически = по дате.
  const periodRank = useMemo(() => {
    const m = new Map();
    [...periods]
      .sort((a, b) => String(a.start_date || '').localeCompare(String(b.start_date || '')))
      .forEach((p, i) => m.set(p.id, i));
    return m;
  }, [periods]);

  const selGoal = data && sel != null ? data.goals.find(g => g.id === sel) : null;

  return (
    <div style={{ display: 'flex', height: '100vh', overflow: 'hidden' }}>
      <Sidebar user={me} active="goal-tree" beforeSections={periods.length > 0 ? (
        <div className="gt-sidebar-period">
          <div className="gt-controls__label">Период</div>
          <GtPeriodSelect periods={periods} value={periodId} onChange={setPeriodId} />
        </div>
      ) : null}>
        <div className="gt-controls">
          <label className="gt-toggle">
            <input type="checkbox" checked={crossPeriod} onChange={e => setCrossPeriod(e.target.checked)} />
            Связи между периодами
          </label>
          <label className="gt-toggle">
            <input type="checkbox" checked={hideUnlinked} onChange={e => setHideUnlinked(e.target.checked)} />
            Скрыть цели без связи
          </label>
          <label className="gt-toggle">
            <input type="checkbox" checked={myGoals} onChange={e => setMyGoals(e.target.checked)} />
            Мои цели
          </label>
          {pickerData && (
            <div className="gt-controls__root">
              <div className="gt-controls__label">Корневая цель</div>
              <RootPicker data={pickerData} value={selectedRootId} onChange={setSelectedRootId} />
            </div>
          )}
        </div>
      </Sidebar>
      <div className="gt-main">
        <div className="gt-topbar">
          <label className="gt-topbar__toggle">
            <input type="checkbox" checked={onlyRoots} onChange={e => setOnlyRoots(e.target.checked)} />
            Показать только корневые цели
          </label>
        </div>
        <div className="gt-main__body">
          {status === 'loading' && <div className="gt-state">Загрузка…</div>}
          {status === 'error' && (
            <div className="gt-state">
              Не удалось загрузить дерево целей.
              <button onClick={reload}>Повторить</button>
            </div>
          )}
          {status === 'ready' && finalData && (
            finalData.goals.length === 0
              ? <div className="gt-state">Нет целей со связями</div>
              : <GoalTreeCanvas data={finalData} periodRank={periodRank} selectedId={sel} onSelect={onSelect}
                  collapsed={collapsed} onToggleCollapse={onToggleCollapse} childCountMap={childCountMap} />
          )}
          {status === 'ready' && selGoal && data && (
            <GoalDetailPanel goal={selGoal} data={data}
              focused={selectedRootId === selGoal.id}
              onToggleFocus={() => setSelectedRootId(prev => (prev === selGoal.id ? null : selGoal.id))}
              onClose={() => setSel(null)} />
          )}
        </div>
      </div>
    </div>
  );
}

ReactDOM.createRoot(document.getElementById('root')).render(<GoalTreeApp />);
