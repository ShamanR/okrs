// Прототип трекера OKR: рендер и поведение поверх продуктового CSS.
// Разметка повторяет компоненты из web/static/tracker.js и sidebar.js — порядок
// блоков, классы и подписи, — чтобы утверждённый вид переносился в код дословно.

const esc = s => String(s == null ? '' : s)
  .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');

const GREEN_THRESHOLD = 80;   // настройка пространства, дефолт 80
const KR_BEHIND_PP = 20;    // KR краснеет, если отстаёт от плана больше чем на 20 п.п.
const BEHIND_MARGIN = 10;     // отставание от прогноза, при котором процент краснеет

const state = {
  section: 'tracker',
  teamId: 11,
  periodId: 3,
  expanded: { 10: true, 12: true },
  favorites: { 11: true, 12: true, 15: true },
  // Настройки показа целей на доске — один объект, как BOARD_VIEW в tracker.js.
  boardView: { sort: 'custom', pri: {}, notes: false, hideZeroWeight: false },
  topMenu: false,
  showCom: {},        // goalId → развёрнуты комментарии
  noteVis: {},        // krId → перекрытие птички «Заметки к KR» (undefined — следовать ей)
  noteCollapsed: {},  // krId → длинная заметка свёрнута до одной строки
  navOpen: false,
  notifOpen: false,
  periodMenu: false,
  tenantMenu: false,
  userMenu: false,
  modal: null,
  goalMenu: null,     // id цели, у которой открыто меню «···»
  drill: null,
  scope: 'my_teams',
  activity: { category: '', actor: '', range: 'all', favOnly: false, q: '' },
  tree: {
    selectedId: null, collapsed: new Set(), zoom: 1, focused: false, highlight: null,
    // Дефолты фильтров совпадают с GoalTreeApp.
    crossPeriod: false, hideUnlinked: true, myGoals: true, onlyRoots: false,
    rootId: null, rootOpen: false, rootQ: '', periodMenu: false,
  },
  chartHover: null,
};

const period = () => PERIODS.find(p => p.id === state.periodId) || PERIODS[0];
const forecastNow = () => Math.round(period().elapsed * 100);

// ── Расчёты ──────────────────────────────────────────────────────────────────
const clampPct = v => Math.max(0, Math.min(100, v));

// Повторяет calcKRProgress из tracker.js, включая интерполяцию по промежуточным
// значениям.
function calcKRProgress(kr) {
  if (kr.krType === 'BOOLEAN') return kr.done ? 100 : 0;
  if (kr.krType === 'PROJECT') return Math.min(100, (kr.stages || []).filter(s => s.done).reduce((a, s) => a + (s.weight || 0), 0));
  const start = Number(kr.start || 0), target = Number(kr.target ?? 100), cur = Number(kr.current || 0);
  const raw = (kr.checkpoints || []).filter(c => c.value !== '' && c.value !== null && c.value !== undefined);
  if (raw.length) {
    const pts = [{ value: start, pct: 0 }, ...raw.map(c => ({ value: Number(c.value), pct: Number(c.progress_percent) })), { value: target, pct: 100 }]
      .sort((a, b) => a.value - b.value);
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
const krProgress = calcKRProgress;

function goalProgress(goal) {
  const total = goal.krs.reduce((s, k) => s + k.weight, 0);
  if (!total) return 0;
  return Math.round(goal.krs.reduce((s, k) => s + krProgress(k) * k.weight, 0) / total);
}

function goalsOf(teamId) {
  return ((BOARDS[teamId] && BOARDS[teamId].goals) || []).filter(g => g.periodId === state.periodId);
}
function boardOf(teamId) { return BOARDS[teamId] || { status: 'no_goals', changedAt: '—', goals: [] }; }

function teamProgress(teamId) {
  const goals = goalsOf(teamId);
  if (!goals.length) return null;
  const total = goals.reduce((s, g) => s + g.weight, 0) || 1;
  return Math.round(goals.reduce((s, g) => s + goalProgress(g) * g.weight, 0) / total);
}

// Повторяет healthOf из tracker.js.
function healthOf(p, stale, forecast, greenThreshold = GREEN_THRESHOLD) {
  if (p === null || p === undefined) return 'no_goals';
  if (stale) return 'stale';
  if (p >= greenThreshold) return 'ahead';
  if (forecast == null) return 'on_track';
  const delta = forecast - p;
  if (delta < -10) return 'ahead';
  if (delta > 10) return 'below';
  return 'on_track';
}

// Повторяет sidebarProgressColor из tracker.js; behindMargin в продукте приходит
// из настроек пространства, здесь фиксирован.
function sidebarProgressColor(prog, status, forecast) {
  if (prog == null) return HEALTH_COLOR.no_goals;
  if (prog >= GREEN_THRESHOLD) return HEALTH_COLOR.ahead;
  if (status === 'closed') return HEALTH_COLOR.below;
  if (forecast != null && forecast - prog > BEHIND_MARGIN) return HEALTH_COLOR.below;
  return HEALTH_COLOR.ahead;
}

// Признак «нет обновлений» отслеживается только в фазе исполнения.
function goalIsStale(goal, teamId) {
  return boardOf(teamId).status === 'in_progress' && goal.updatedDaysAgo > STALE_DAYS;
}

function findTeam(id, nodes = TREE) {
  for (const n of nodes) {
    if (n.id === id) return n;
    const f = findTeam(id, n.children);
    if (f) return f;
  }
  return null;
}
function flatTeams(nodes = TREE, out = []) { for (const n of nodes) { out.push(n); flatTeams(n.children, out); } return out; }
function teamPath(id, nodes = TREE, trail = []) {
  for (const n of nodes) {
    const next = [...trail, n.name];
    if (n.id === id) return next;
    const f = teamPath(id, n.children, next);
    if (f) return f;
  }
  return null;
}

const fmtNum = n => (Math.round(n * 100) / 100).toString().replace('.', ',');
const fmtVal = (v, unit) => `${fmtNum(v)}${unit ? ' ' + unit : ''}`;

// ── Микрокомпоненты (Badge / ProgressBar / PriBadge / KRHealthBadge) ─────────
function badge(label, color = '#6b7280', bg) {
  return `<span class="badge" style="color:${color};background:${bg || color + '18'}">${label}</span>`;
}
const priBadge = p => badge(p, PRI_COLOR[p] || '#6b7280');
// Цвет приезжает из модификатора .kr-hdot--* в tracker.css, как в приложении: у закрытого
// состояния три вида по прогрессу, и инлайновый --hc их бы перекрыл.
function krHealthDot(status, progress = 0) {
  const s = KR_HEALTH_LABEL[status] ? status : 'not_started';
  const view = s === 'done' ? closedViewOf(progress, GREEN_THRESHOLD) : s;
  const closed = KR_CLOSED_VIEW[view];
  const icon = closed ? closed.icon : KR_HEALTH_ICON[s];
  const hint = closed ? closed.hint(progress) : (KR_HEALTH_HINT[s] || '');
  return `<span class="kr-hdot kr-hdot--${view}" tabindex="0" aria-label="Статус KR: ${KR_HEALTH_LABEL[s]}${closed && closed.note ? ', ' + closed.note : ''}" data-no-drag>
    <span class="kr-hdot__mark">${icon}</span>
    ${closed && closed.flag ? `<span class="kr-hdot__flag" aria-hidden="true">${closed.flag}</span>` : ''}
    <span class="kr-hdot__pop" role="tooltip">
      <span class="kr-hdot__title">${icon} ${KR_HEALTH_LABEL[s]}</span>
      <span class="kr-hdot__hint">${esc(hint)}</span>
    </span>
  </span>`;
}
function krHealthBadge(status) {
  const s = KR_HEALTH_LABEL[status] ? status : 'not_started';
  const label = `${KR_HEALTH_ICON[s]} ${KR_HEALTH_LABEL[s]}`;
  return s === 'done' ? badge(label, '#ffffff', KR_HEALTH_COLOR[s]) : badge(label, KR_HEALTH_COLOR[s]);
}
function progressBar(value, forecast, h = 8, color) {
  return `<div class="progress-bar" style="height:${h}px;border-radius:${h / 2}px">
    <div class="progress-bar__fill" style="width:${Math.min(value || 0, 100)}%;background:${color || ACCENT};border-radius:${h / 2}px"></div>
    ${forecast != null ? `<div class="progress-bar__forecast" style="top:-3px;left:${forecast}%;height:${h + 6}px"></div>` : ''}
  </div>`;
}
// Круглый аватар пользователя, как SidebarAvatar / UserInfo.
function avatar(initials, size) {
  const colors = ['#2563eb', '#7c3aed', '#059669', '#d97706', '#dc2626', '#0891b2', '#be185d', '#6366f1'];
  const bg = colors[(initials.charCodeAt(0) || 0) % colors.length];
  return `<span style="width:${size}px;height:${size}px;border-radius:50%;flex-shrink:0;display:inline-flex;align-items:center;justify-content:center;color:#fff;font-weight:700;font-size:${Math.round(size * 0.38)}px;background:${bg}">${esc(initials)}</span>`;
}
function userInfo(u, size) {
  return `<span class="uinfo">${avatar(u.initials, size)}<span class="uinfo__name">${esc(u.name)}</span></span>`;
}

// Единственное место, где интерфейс знает правило «статус → редактирование».
// structure — изменение состава целей и KR, progress — обновление прогресса.
const LOCK_REASON = {
  progress_only: 'Состав целей зафиксирован: команда в статусе «В работе»',
  view: 'Период закрыт: доступны только комментарии',
};
const MODE_ALLOWS = {
  full: { structure: true, progress: true },
  progress_only: { structure: false, progress: true },
  view: { structure: false, progress: false },
};
function lockFor(mode, kind) {
  const allows = MODE_ALLOWS[mode] || MODE_ALLOWS.full;
  return allows[kind] ? null : LOCK_REASON[mode];
}

// Баннер режима под степпером: в полном редактировании не рендерится.
const MODE_BANNER = {
  progress_only: { cls: 'progress', status: 'В работе',
    text: 'меняются только прогресс и комментарии',
    full: 'Состав целей зафиксирован. Доступны обновление прогресса и комментарии.' },
  view: { cls: 'closed', status: 'Период закрыт',
    text: 'доступны только комментарии', full: 'Доступны только комментарии.' },
};
const STATUS_BANNER = {
  forming: { cls: 'info', icon: '✎', status: 'Черновик',
    text: 'заполните черновик целей. Когда будете готовы показать руководителю — переведите в статус «К валидации»',
    full: 'Заполните черновик целей. Когда будете готовы показать руководителю — переведите в статус «К валидации».' },
  ready: { cls: 'info', icon: '✓', status: 'К валидации',
    text: 'получите от руководителя апрув на цели и переведите в статус «В работе»',
    full: 'Получите от руководителя апрув на цели и переведите в статус «В работе».' },
};
function modeBanner(mode, status) {
  const b = (mode === 'full' && STATUS_BANNER[status]) || MODE_BANNER[mode];
  if (!b) return '';
  return `<div class="mode-banner mode-banner--${b.cls}" title="${b.status}. ${b.full}">
    <span class="mode-banner__icon">${b.icon || '🔒'}</span>
    <span class="mode-banner__text"><span class="mode-banner__status">${b.status}</span><span class="mode-banner__rest"> — ${b.text}</span></span>
  </div>`;
}

// ── Сайдбар ──────────────────────────────────────────────────────────────────
// Порядок как в Sidebar: шапка тенанта → блок периода (beforeSections) →
// разделы → контекстная навигация (дерево команд) → футер.
const SIDEBAR_SECTIONS = [
  { id: 'tracker', label: 'Цели команды', icon: '🎯' },
  { id: 'goal-tree', label: 'Дерево целей', icon: '🕸' },
  { id: 'period-overview', label: 'Обзор периода', icon: '📊' },
  { id: 'activity-log', label: 'Лог активностей', icon: '🕑' },
];

function treeNode(node, depth) {
  const prog = teamProgress(node.id);
  const color = sidebarProgressColor(prog, boardOf(node.id).status, forecastNow());
  const nameCls = depth === 0 ? 'd0' : depth === 1 ? 'd1' : 'dx';
  const selected = node.id === state.teamId && state.section === 'tracker';
  const hasKids = node.children.length > 0;
  const open = !!state.expanded[node.id];
  const rows = [`
    <div class="sidebar-node__row${selected ? ' sidebar-node__row--selected' : ''}"
         style="padding:5px 10px 5px ${10 + depth * 12}px" data-team="${node.id}" role="button" tabindex="0">
      ${hasKids ? `<span class="sidebar-node__toggle" data-toggle="${node.id}">${open ? '▾' : '▸'}</span>`
                : '<span class="sidebar-node__spacer"></span>'}
      <span class="sidebar-node__dot" style="background:${TEAM_TYPE_COLOR[node.type]}"></span>
      <span class="sidebar-node__name sidebar-node__name--${nameCls}${selected ? ' sidebar-node__name--selected' : ''}">${esc(node.name)}</span>
      <span class="sidebar-node__star${state.favorites[node.id] ? ' sidebar-node__star--on' : ''}" data-fav="${node.id}">${state.favorites[node.id] ? '★' : '☆'}</span>
      <span class="sidebar-node__progress" style="color:${color}">${prog == null ? '' : prog + '%'}</span>
    </div>`];
  if (hasKids && open) node.children.forEach(c => rows.push(treeNode(c, depth + 1)));
  return rows.join('');
}

function renderSidebar() {
  const p = period();
  const tenant = TENANTS.find(t => t.active) || TENANTS[0];
  const canSwitch = TENANTS.length > 1;
  const unread = NOTIFICATIONS.filter(n => n.unread).length;
  const favs = flatTeams().filter(t => state.favorites[t.id]);
  document.getElementById('sidebar').innerHTML = `
    <div class="sidebar__tenant">
      <div class="sidebar__tenant-avatar">${tenantInitials(tenant.name)}</div>
      <button class="sidebar__tenant-main"${canSwitch ? ' data-tenant="1"' : ' disabled'}>
        <span class="sidebar__tenant-name">${esc(tenant.name)}</span>
        ${canSwitch ? '<span class="sidebar__tenant-chevron">▾</span>' : ''}
      </button>
      <button class="sidebar__bell" id="bell" aria-label="Уведомления">
        <span class="sidebar__bell-icon">🔔</span>
        <span class="sidebar__bell-badge${unread ? '' : ' sidebar__bell-badge--zero'}">${unread}</span>
      </button>
      ${state.tenantMenu && canSwitch ? `<div class="sidebar__tenant-menu" data-tenant-menu="1">
        ${TENANTS.map(t => `<button class="sidebar__tenant-item${t.active ? ' sidebar__tenant-item--active' : ''}" data-tenant-set="${t.id}">
          <span class="sidebar__tenant-item-icon">${t.active ? '✓' : '🏢'}</span>${esc(t.name)}
        </button>`).join('')}
      </div>` : ''}
    </div>

    ${state.section === 'tree'
      ? `<div class="gt-sidebar-period">
          <div class="gt-controls__label">Период</div>
          ${gtPeriodSelect()}
        </div>`
      : `<div class="sidebar__period">
      <div class="sidebar__period-label">Период</div>
      <div class="period-select">
        <button class="period-select__trigger" id="period-trigger">
          <span class="period-select__dot" style="background:${p.color}"></span>
          <span class="period-select__name">${esc(p.name)}</span>
          <span class="period-select__chev">▾</span>
        </button>
        ${state.periodMenu ? `<div class="period-select__menu" style="left:${(state.periodMenuRect || {}).left || 0}px;top:${(state.periodMenuRect || {}).top || 0}px;width:380px">
          <div class="period-select__group">Периоды</div>
          ${PERIODS.map(x => `<button class="period-select__item${x.id === p.id ? ' is-selected' : ''}" data-period="${x.id}">
            <span class="period-select__indent" style="width:${(x.depth || 0) * 16}px"></span>
            <span class="period-select__item-name">${esc(x.name)}</span>
            <span class="period-select__range">${esc(x.range)}</span>
            <span class="period-select__badge-wrap">${x.state ? `<span class="period-select__badge" style="color:${x.color};background:${x.color}22">${esc(x.state)}</span>` : ''}</span>
          </button>`).join('')}
        </div>` : ''}
      </div>
    </div>`}

    <div class="sidebar__sections">
      <div class="sidebar__section-label">Разделы</div>
      ${SIDEBAR_SECTIONS.map(s => `<button class="sidebar__navlink${s.id === (state.section === 'overview' ? 'period-overview' : state.section === 'tree' ? 'goal-tree' : state.section === 'activity' ? 'activity-log' : 'tracker') ? ' sidebar__navlink--active' : ''}" data-section="${s.id}">
        <span class="sidebar__navlink-icon">${s.icon}</span>${s.label}</button>`).join('')}
    </div>

    ${state.section === 'tree' ? gtSidebarControls() : `<div class="sidebar__tree">
      <div class="sidebar__section-label">Команды</div>
      ${favs.length ? `<div class="sidebar__subsection-label"><span class="sidebar__subsection-star">★</span>Избранное · ${favs.length}</div>
        ${favs.map(t => treeNode({ ...t, children: [] }, 0)).join('')}` : ''}
      <div class="sidebar__subsection-label">Все команды</div>
      ${TREE.map(n => treeNode(n, 0)).join('')}
    </div>`}

    <div class="sidebar__footer">
      <a class="sidebar__footer-link" href="#"><span class="sidebar__footer-icon">📖</span>Документация</a>
      <a class="sidebar__footer-link" href="#"><span class="sidebar__footer-icon">☆</span>Обратная связь</a>
      <div class="sidebar__user">
        ${avatar(ME.initials, 36)}
        <div class="sidebar__user-info">
          <div class="sidebar__user-name">${esc(ME.name)}</div>
          <div class="sidebar__user-sub">${esc(ME.email)}</div>
        </div>
        <button class="sidebar__user-more" aria-label="Меню пользователя" data-user-menu="1">···</button>
        ${state.userMenu ? `<div class="sidebar__user-menu" data-user-menu-box="1">
          <a href="#" class="sidebar__user-menu-item"><span class="sidebar__user-menu-icon">⚙</span>Настройки</a>
          ${ME.isAdmin ? '<a href="#" class="sidebar__user-menu-item"><span class="sidebar__user-menu-icon">🛠</span>Администрирование</a>' : ''}
          ${ME.isSystemAdmin ? '<a href="#" class="sidebar__user-menu-item"><span class="sidebar__user-menu-icon">🖥</span>System</a>' : ''}
          <button class="sidebar__user-menu-item sidebar__user-menu-item--danger">
            <span class="sidebar__user-menu-icon">↩</span>Выйти
          </button>
        </div>` : ''}
      </div>
    </div>`;
}

// ── Топбар и степпер ─────────────────────────────────────────────────────────
function renderTopbar() {
  const bar = document.getElementById('topbar');
  const stepper = document.getElementById('stepper');

  if (state.section === 'activity') {
    bar.innerHTML = `<div class="topbar">
      <div class="topbar__main">
        <button class="pt-burger" id="burger" aria-label="Меню">☰</button>
        <div class="topbar__title">Лог активностей</div>
        <div class="topbar__spacer"></div>
      </div>
      <div class="topbar__desc">Что происходило с целями и результатами в периоде ${esc(period().name)}</div>
    </div>`;
    stepper.innerHTML = '';
    return;
  }

  if (state.section === 'tree') {
    bar.innerHTML = `<div class="topbar">
      <div class="topbar__main">
        <button class="pt-burger" id="burger" aria-label="Меню">☰</button>
        <div class="topbar__title">Дерево целей</div>
        <div class="topbar__spacer"></div>
      </div>
      <div class="topbar__desc">Связи целей между периодами и командами: годовые сверху, квартальные под ними</div>
    </div>`;
    stepper.innerHTML = '';
    return;
  }

  if (state.section === 'overview') {
    bar.innerHTML = `<div class="topbar">
      <div class="topbar__main">
        <button class="pt-burger" id="burger" aria-label="Меню">☰</button>
        <div class="topbar__title">Обзор периода</div>
        <div class="topbar__spacer"></div>
        <div style="display:inline-flex;align-items:center;gap:8px">
          <span style="font-size: 13px;color: #6b7280;font-weight:600">Охват</span>
          <div style="display:inline-flex;gap:4px;background:#f1f5f9;padding:4px;border-radius:10px">
            ${['my_teams', 'org'].map(k => `<button data-scope="${k}" style="padding:6px 14px;border:none;cursor:pointer;font-size: 14px;font-weight:600;border-radius:8px;background:${state.scope === k ? ACCENT : 'transparent'};color:${state.scope === k ? '#fff' : '#6b7280'}">${k === 'org' ? 'Вся организация' : 'Мои команды'}</button>`).join('')}
          </div>
        </div>
      </div>
      <div class="topbar__desc">Состояние целей всех команд в периоде ${esc(period().name)}</div>
    </div>`;
    stepper.innerHTML = '';
    return;
  }

  const team = findTeam(state.teamId);
  const board = boardOf(state.teamId);
  const prog = teamProgress(state.teamId);
  const color = sidebarProgressColor(prog, board.status, forecastNow());

  bar.innerHTML = `<div class="topbar">
    <div class="topbar__main">
      <button class="pt-burger" id="burger" aria-label="Меню">☰</button>
      <div class="topbar__title">${esc(team.name)}</div>
      ${team.lead ? `<span class="tb-lead">${avatar(team.leadInitials || '', 22)}<span class="tb-lead__name">${esc(team.lead)}</span><span class="tb-lead__role">· лид</span></span>` : ''}
      <div class="topbar__spacer"></div>
      <div class="topbar__progress">
        <span class="tb-cap">Цели узла</span>
        <div class="topbar__progress-bar">${progressBar(prog || 0, forecastNow(), 6, color)}</div>
        <span class="topbar__progress-pct" style="color:${color}">${prog == null ? '—' : prog + '%'}</span>
      </div>
      ${(() => {
        const lock = lockFor(editModeOf(state.teamId), 'structure');
        return lock
          ? `<span class="action-lock"><button class="topbar__add-btn topbar__add-btn--locked" type="button" aria-disabled="true">+ Добавить цель 🔒</button><span class="action-lock__tip" role="tooltip">${esc(lock)}</span></span>`
          : '<button class="topbar__add-btn" data-modal="goal-new">+ Добавить цель</button>';
      })()}
      <div class="export-menu tb-menu" data-topmenu-wrap="1">
        <button type="button" class="tb-menu__btn" title="Ещё" aria-label="Ещё" data-topmenu="1">···</button>
        ${state.topMenu ? `<div class="export-menu__dropdown tb-menu__dropdown">
          ${menuItem('⇩', 'Экспорт в Markdown', 'data-open-team-export="1"')}
        </div>` : ''}
      </div>
    </div>
    ${team.desc ? `<div class="topbar__desc md-content">${team.desc.split('\n').map(l => `<p>${esc(l)}</p>`).join('')}</div>` : ''}
  </div>`;

  const curIdx = STATUS_STEPS.findIndex(s => s.k === board.status);
  const hasGoals = goalsOf(state.teamId).length > 0;
  stepper.innerHTML = `<div class="status-stepper">
    ${!hasGoals ? '<span class="status-stepper__no-goals">Нет целей</span>' : ''}
    ${STATUS_STEPS.map((s, i) => {
      const isCur = s.k === board.status, isPast = i < curIdx;
      const style = `background:${isCur ? ACCENT : isPast ? ACCENT + '15' : 'transparent'};color:${isCur ? 'white' : isPast ? ACCENT : '#9ca3af'}`;
      return (i > 0 ? '<div class="status-stepper__connector"></div>' : '') +
        `<button class="status-stepper__btn" style="${style}" data-status="${s.k}"${hasGoals ? '' : ' disabled'}>${s.l}</button>`;
    }).join('')}
    ${modeBanner(editModeOf(state.teamId), board.status)}
    <div class="status-stepper__meta">
      <span class="status-stepper__changed">изменён ${esc(board.changedAt)}</span>
    </div>
  </div>`;
}

// ── Строка ключевого результата ──────────────────────────────────────────────
// Режим правки: в фазе исполнения состав заблокирован и доступно только обновление
// прогресса, в закрытом периоде — ничего. Правило прототипное, точный гейт в
// продукте приходит с сервера.
function editModeOf(teamId) {
  const st = boardOf(teamId).status;
  return st === 'in_progress' ? 'progress_only' : st === 'closed' ? 'view' : 'full';
}

// Правка текста цели/KR (клик по заголовку и пункт меню) — только в «Черновике» и «К валидации».
// Карандаш у заголовка — лишь признак редактируемости.
function canEditText(teamId) {
  const st = boardOf(teamId).status;
  return st === 'forming' || st === 'ready';
}

function editLockReason(teamId, what) {
  const st = (BOARDS[teamId] || {}).status;
  if (st === 'closed') return `Период закрыт — ${what} уже нельзя изменить.`;
  if (st === 'in_progress') return `Цели в статусе «В работе»: состав зафиксирован. Меняется только прогресс. Чтобы редактировать ${what}, верните статус «К валидации».`;
  return `Редактирование недоступно в текущем статусе.`;
}

function deleteLockReason(teamId, what) {
  const st = (BOARDS[teamId] || {}).status;
  const acc = what === 'цель' ? 'цель' : 'KR';
  if (st === 'closed') return `Период закрыт — удалить ${acc} нельзя, история периода сохраняется.`;
  if (st === 'in_progress') return `Цели в статусе «В работе»: состав зафиксирован. Чтобы удалить ${acc}, верните статус «К валидации».`;
  return `Удаление недоступно в текущем статусе.`;
}

function menuItem(icon, label, attrs, opts = {}) {
  const cls = ['act-menu__item', opts.danger ? 'act-menu__item--danger' : '', opts.disabled ? 'act-menu__item--disabled' : ''].filter(Boolean).join(' ');
  const tip = opts.disabled && opts.reason ? `<span class="act-menu__why" role="tooltip">${esc(opts.reason)}</span>` : '';
  return `<button type="button" class="${cls}${tip ? ' act-menu__item--why' : ''}"${opts.disabled ? ' aria-disabled="true"' + (opts.reason ? ` aria-label="${esc(label + '. ' + opts.reason)}"` : '') : ' data-menu-close="1" ' + attrs}><span class="act-menu__ic">${icon}</span>${label}${opts.disabled && opts.reason ? '<span class="act-menu__lock" aria-hidden="true">?</span>' : ''}${tip}</button>`;
}

function krMenu(goalId, krId, teamId) {
  const key = goalId + ':' + krId;
  const open = state.krMenu === key;
  const structLock = lockFor(editModeOf(teamId), 'structure');
  const progressLock = lockFor(editModeOf(teamId), 'progress');
  return `<div class="export-menu" data-menu-wrap="kr-${key}">
    <button type="button" class="export-menu__btn" title="Ещё" aria-label="Ещё" data-kr-menu="${key}">···</button>
    ${open ? `<div class="export-menu__dropdown">
      ${menuItem('↻', 'Обновить прогресс', `data-kr-progress="${key}"`, { disabled: !!progressLock, reason: 'Период закрыт — прогресс ключевого результата уже нельзя обновить.' })}
      ${menuItem('✎', 'Редактировать', `data-kr-edit="${key}"`, { disabled: !canEditText(teamId), reason: editLockReason(teamId, 'KR') })}
      ${menuItem('🔗', 'Копировать ссылку', '')}
      ${krNoteText(krOf(goalId, krId)) ? menuItem('📝', krNoteVisible(krId) ? 'Скрыть заметку' : 'Показать заметку', `data-kr-note-vis="${krId}"`) : ''}
      <div class="act-menu__sep"></div>
      ${menuItem('×', 'Удалить', '', { danger: true, disabled: !!structLock, reason: deleteLockReason(teamId, 'KR') })}
    </div>` : ''}
  </div>`;
}

function krNoteText(kr) {
  if (typeof kr.note === 'string') return kr.note;
  if (!kr.note) return '';
  return KR_NOTE_SAMPLES[kr.id % KR_NOTE_SAMPLES.length];
}
const KR_NOTE_SAMPLES = [
  '80% — TV-баннеры уже заводятся в SS, остались ООН.\nБлокер: ждём от юристов согласование оферты для ООН-форматов.\nПлан: после согласования — 2 недели на доработку, релиз ожидаем в конце месяца.',
  'Сделано полностью: лимиты доступны всем РА с 15-го числа, адопция — 34 агентства.',
  '70% — click-in запущен в пилоте с тремя агентствами. Осталось закрыть биллинг и отчётность; риск — зависимость от команды Биллинга.',
];

// Заметка видна, если включена птичка «Заметки к KR» или её показали у этого KR из
// меню. Перекрытие per-KR сбрасывается при переключении птички — как в KRRow.
function krNoteVisible(krId) {
  const ov = state.noteVis[krId];
  return ov === undefined ? state.boardView.notes : ov;
}

function krOf(goalId, krId) {
  const g = goalsOf(state.teamId).find(x => x.id === +goalId);
  return (g && g.krs.find(k => k.id === +krId)) || {};
}

function krNote(kr) {
  const text = krNoteText(kr);
  if (!text) return '';
  if (!krNoteVisible(kr.id)) return '';
  // Показанная заметка развёрнута: иначе её пришлось бы раскрывать по одному клику на KR —
  // ровно та проблема, из-за которой птичка и появилась. Клик сворачивает до одной строки.
  const open = !state.noteCollapsed[kr.id];
  const long = text.length > 90 || text.includes('\n');
  const ago = kr.updatedDaysAgo === 0 ? 'сегодня' : kr.updatedDaysAgo + 'д назад';
  return `<div class="kr-note${open ? ' kr-note--open' : ''}${long ? ' kr-note--long' : ''}"${long ? ` role="button" tabindex="0" aria-expanded="${open}" data-kr-note="${kr.id}"` : ''} data-no-drag>
    <span class="kr-note__label">Заметка</span>
    <div class="md-content kr-note__text"><p>${esc(text)}</p></div>
    ${long ? `<span class="kr-note__toggle">${open ? 'Свернуть' : 'Ещё'}</span>` : ''}
    ${open ? `<span class="kr-note__meta">Обновлена вместе с прогрессом · ${ago}</span>` : ''}
  </div>`;
}

// Колонка «цель» у KR: откуда и куда движемся. Для 0→100% ничего не пишем — хватает полоски.
function krTargetText(kr) {
  const lbl = '';
  if (kr.krType === 'BOOLEAN') return `${lbl}<span class="kr-target__val">false → true</span>`;
  if (kr.krType === 'PROJECT') {
    const st = kr.stages || [];
    return `${lbl}<span class="kr-target__val">${st.filter(s => s.done).length} → ${st.length} шагов</span>`;
  }
  const start = Number(kr.start) || 0, target = Number(kr.target) || 0, unit = kr.unit || '';
  const up = target >= start;
  if (false) {
    return `<span class="kr-target__val"><span class="kr-target__dir kr-target__dir--up">↑</span>${fmtNum(Number(kr.current) || 0)} → ${fmtNum(target)} %</span>`;
  }
  const cur = Number(kr.current) || 0;
  return `<span class="kr-target__val"><span class="kr-target__dir kr-target__dir--${up ? 'up' : 'down'}">${up ? '↑' : '↓'}</span>${fmtNum(cur)} → ${fmtNum(target)}${unit ? ' ' + esc(unit) : ''}</span>`;
}

function krDetail(kr) {
  if (kr.krType === 'BOOLEAN') {
    return `<span class="kr-detail" style="color:${kr.done ? '#16a34a' : '#9ca3af'};font-weight:600">${kr.done ? '✓ Выполнено' : '○ Не выполнено'}</span>`;
  }
  if (kr.krType === 'PROJECT') {
    return `<span class="kr-detail">${(kr.stages || []).filter(s => s.done).length}/${(kr.stages || []).length} шагов</span>`;
  }
  return `<span class="kr-detail">${fmtVal(kr.current, kr.unit)} / ${fmtVal(kr.target, kr.unit)}</span>`;
}

function krRow(goal, kr, teamId) {
  const board = boardOf(teamId);
  const displayHealth = board.status === 'closed' ? 'done' : kr.healthStatus;
  const progress = krProgress(kr);
  const staleC = kr.updatedDaysAgo > STALE_DAYS ? '#dc2626'
    : kr.updatedDaysAgo > STALE_DAYS * 0.6 ? '#d97706' : '#10b981';
  const mode = editModeOf(teamId);
  const krBehind = board.status !== 'closed' && forecastNow() - progress > KR_BEHIND_PP;
  const krBarC = krBehind ? '#dc2626' : ACCENT;
  return `<div class="kr-item${mode === 'full' ? ' kr-item--reorderable' : ''}"
       ${mode === 'full' ? `draggable="true" data-kr-item="${goal.id}:${kr.id}"` : ''}>
    ${mode === 'full' ? '<div class="kr-item__drag-handle">⋮⋮</div>' : ''}
    <div class="kr-row">
      <div class="kr-row__main">
        ${krHealthDot(displayHealth, progress)}
        <div class="kr-weight-chip has-tip" tabindex="0" data-tip-title="Вес KR · ${kr.weight}%" data-tip="Доля KR в прогрессе цели. Сумма весов всех KR цели — 100%.">${kr.weight}%</div>
        <div class="kr-info">
          <div class="kr-name-row${mode === 'comments_only' ? '' : ' title-editable'}"${
            mode === 'full' ? ` role="button" tabindex="0" title="Редактировать KR" data-kr-edit="${goal.id}:${kr.id}"`
            : mode === 'progress_only' ? ` role="button" tabindex="0" title="Обновить прогресс" data-kr-progress="${goal.id}:${kr.id}"`
            : ''}>
            <div class="kr-name">${esc(kr.name)}</div>
            ${mode === 'full' ? `<span class="title-edit" aria-hidden="true">✎</span>`
              : mode === 'progress_only' ? `<span class="title-edit title-edit--progress" aria-hidden="true">↻</span>`
              : `<span class="action-lock"><span class="title-edit title-edit--locked" tabindex="0" role="img" aria-label="Период закрыт">🔒</span><span class="action-lock__tip" role="tooltip">Период закрыт: доступны только комментарии</span></span>`}
          </div>
          ${kr.desc ? `<div class="kr-desc md-content" data-desc-key="k${kr.id}">${mdRender(kr.desc)}</div>` : ''}
          ${kr.zeroing ? `<div class="kr-zeroing-note kr-zeroing-note--clamp"><span class="kr-zeroing-note__icon">⊘</span>Критерий обнуления: ${esc(kr.zeroing)}</div>` : ''}
        </div>
        <div class="kr-measure">
        <div class="kr-target-col">${krTargetText(kr)}</div>
        <div class="kr-progress-col">
          <div class="kr-progress-col__body kr-progress-col__body--row" title="Прогресс ${progress}% · ожидаемо к сегодня ${forecastNow()}%${krBehind ? ' · отставание больше ' + KR_BEHIND_PP + ' п.п.' : ''}">
            <div class="kr-progress-col__bar">${progressBar(progress, forecastNow(), 5, krBarC)}</div>
            <span class="kr-pct" style="color:${krBarC}">${progress}%</span>
          </div>
        </div>
        </div>
        <div class="kr-row__actions" data-no-drag>
          <span class="kr-row__spacer"></span>
          ${mode === 'full' ? '' : `<span class="kr-updated" style="color:${staleC}" title="Последнее обновление прогресса">${kr.updatedDaysAgo === 0 ? 'обн. сегодня' : 'обн. ' + kr.updatedDaysAgo + 'д назад'}</span>`}
          ${krMenu(goal.id, kr.id, teamId)}
        </div>
      </div>
      ${krNote(kr)}
    </div>
  </div>`;
}

// ── Комментарии ──────────────────────────────────────────────────────────────
function commentRow(c) {
  return `<div class="comment${c.resolved ? ' comment--resolved' : ' comment--task-open'}">
    ${avatar(c.initials, 28)}
    <div class="comment__content">
      <div class="comment__header">
        <span class="comment__author">${esc(c.author)}</span>
        <span class="comment__date">${esc(c.date)}</span>
        ${c.resolved ? '<span class="comment__resolved-badge">✓ Решено</span>' : ''}
      </div>
      <div class="comment__text md-content">${esc(c.text)}</div>
      ${c.resolved
        ? `<div class="comment__resolved-meta"><span class="comment__resolved-info">Решено · ${esc(c.resolvedBy)} · ${esc(c.resolvedAt)}</span></div>`
        : `<div class="comment__actions">
             <button class="comment__resolve-btn" data-resolve="${c.id}">✓ Отметить решённым</button>
             <button class="comment__link-btn"><span class="comment__link-ic">↩</span>Ответить</button>
           </div>`}
      ${(c.replies || []).length ? `<div class="comment__replies">
        ${c.replies.map(r => `<div class="comment comment--reply" style="margin-bottom:0">
          ${avatar(r.initials, 22)}
          <div class="comment__content">
            <div class="comment__header"><span class="comment__author">${esc(r.author)}</span><span class="comment__date">${esc(r.date)}</span></div>
            <div class="comment__text md-content">${esc(r.text)}</div>
          </div>
        </div>`).join('')}
      </div>` : ''}
    </div>
  </div>`;
}

function commentsPanel(goal) {
  const open = goal.comments.filter(c => !c.resolved);
  const resolved = goal.comments.filter(c => c.resolved);
  return `<div class="comments-section"><div class="comments-panel">
    <div class="comments-panel__title">Комментарии
      ${open.length ? `<span class="comments-panel__unresolved">${open.length} нерешённых</span>` : ''}</div>
    ${open.map(commentRow).join('')}
    ${resolved.length ? `<div class="comments-panel__resolved-head">Решённые · ${resolved.length}</div>` : ''}
    ${resolved.map(commentRow).join('')}
    <div class="comment-compose">
      ${avatar(ME.initials, 28)}
      <div class="comment-compose__right">
        <textarea class="form-textarea form-textarea--sm" style="width:100%;min-height:64px"
          placeholder="Контекст, блокер, заметка… (Cmd+Enter)" data-compose="${goal.id}"></textarea>
        <div class="comment-submit-row">
          <button class="comment-submit comment-submit--disabled" data-send="${goal.id}">Отправить</button>
        </div>
      </div>
    </div>
  </div></div>`;
}

// Название команды-участника — переход на её доску, как в GoalCard. Текущая команда
// переходом не становится: её доска уже открыта. В приложении это <a href>, собранный
// buildTargetURL; у прототипа адресов нет, поэтому переход идёт через data-team — тот
// же обработчик, что у дерева команд.
function shareTeamChip(name, isOwner) {
  const cls = `gc-share-strip__team${isOwner ? ' gc-share-strip__team--owner' : ''}`;
  const tip = isOwner ? ' title="Команда-владелец цели"' : '';
  const body = `${esc(name)}${isOwner ? '<span class="gc-share-strip__role">владелец</span>' : ''}`;
  const team = flatTeams().find(t => t.name === name);
  return team && team.id !== state.teamId
    ? `<a class="${cls}"${tip} data-team="${team.id}">${body}</a>`
    : `<span class="${cls}"${tip}>${body}</span>`;
}

// ── Карточка цели ────────────────────────────────────────────────────────────
// Порядок как в GoalCard: body → footer (переключатели KR и комментариев) →
// секция KR → секция комментариев. Тулбар стоит НАД списком KR, а не под ним.
function goalCard(goal, teamId) {
  const prog = goalProgress(goal);
  const isStale = goalIsStale(goal, teamId);
  const forecast = forecastNow();
  const health = healthOf(prog, isStale, forecast);
  const hC = HEALTH_COLOR[health];
  const mode = editModeOf(teamId);
  // Перетаскивание меняет пользовательский порядок, поэтому при сортировке по
  // приоритету ручка не рисуется: бросок делался бы по одному порядку, а применялся
  // к другому. Перетаскивание KR внутри цели это не затрагивает.
  const canEdit = mode === 'full' && state.boardView.sort === 'custom';
  const structLock = lockFor(mode, 'structure');
  const krWeightSum = goal.krs.reduce((s, k) => s + k.weight, 0);
  const krWeightOff = krWeightSum !== 100;
  const krWeightDelta = 100 - krWeightSum;
  const unresolved = goal.comments.filter(c => !c.resolved).length;
  const showCom = !!state.showCom[goal.id];
  const shared = goal.shareTeams;
  const updatedText = goal.updatedDaysAgo === 0 ? 'сегодня' : goal.updatedDaysAgo + 'д назад';

  return `<div class="goal-card${shared ? ' goal-card--shared' : ''}${isStale ? ' goal-card--stale' : ''}${canEdit ? '' : ''}"
       style="--health:${hC};--health-soft:${hC}18"
       ${canEdit ? `data-goal-item="${goal.id}"` : ''}>
    ${canEdit ? `<div class="drag-handle" title="Перетащите для изменения порядка" data-goal-handle="${goal.id}">⋮⋮</div>` : ''}
    <div class="goal-card__body gc2">
      <div class="gc2__head">
        <span class="gc2__weight has-tip" tabindex="0" data-tip-title="Вес цели · ${goal.weight}%" data-tip="Доля цели в общем прогрессе команды за период. Сумма весов всех целей — 100%.">${goal.weight}%</span>
        <span class="has-tip" tabindex="0" data-tip-title="Приоритет · ${goal.priority}" data-tip="${PRI_HINT[goal.priority] || ''}">${priBadge(goal.priority)}</span>
        <div class="gc2__title-wrap">
          <div class="goal-card__title-row${canEditText(teamId) ? ' title-editable' : ''}"${canEditText(teamId) ? ` role="button" tabindex="0" title="Редактировать цель" data-modal="goal-edit" data-goal="${goal.id}"` : ''}>
            <div class="goal-card__title goal-card__title--readonly">${esc(goal.title)}</div>
            ${canEditText(teamId) ? `<span class="title-edit" aria-hidden="true">✎</span>` : ''}
          </div>
          ${goalLinksLabel('up', goal)}
          ${goalLinksLabel('down', goal)}
        </div>
        <div class="gc2__labels">
          ${badge(goal.type === 'delivery' ? 'Delivery' : 'Discovery', goal.type === 'delivery' ? '#374151' : '#7c3aed')}
          ${goal.focus ? badge(FOCUS_LABEL[goal.focus] || goal.focus, FOCUS_COLORS[goal.focus] || FOCUS_COLORS.DEFAULT) : ''}
        </div>
        ${goal.owners.length ? `<div class="goal-card__owner gc2__owner" title="Драйвер цели">${goal.owners.map(u => userInfo(u, 18)).join('')}</div>` : ''}
        <div class="gc2__bar" title="Прогресс ${prog}% · прогноз ${forecast}%">${progressBar(prog, forecast, 5, 'var(--health)')}</div>
        <span class="goal-card__progress-pct">${prog}%</span>
        ${exportMenu(goal.id, teamId)}
      </div>
      ${goal.desc ? `<div class="goal-card__desc md-content" data-desc-key="g${goal.id}">${mdRender(goal.desc)}</div>` : ''}
    </div>

    ${shared ? `<div class="gc-share-strip" aria-label="Общая цель">
      <span class="gc-share-strip__label">⇄ Общая цель</span>
      ${shareTeamChip(shared.ownerTeam, true)}
      ${shared.teams.map(t => shareTeamChip(t, false)).join('')}
    </div>` : ''}
    <div class="kr-section">
      ${krWeightOff ? `<div class="kr-weight-warn">
        <span class="kr-weight-warn__icon">⚠</span>
        <span>Сумма весов KR = ${krWeightSum}%, ожидается 100% · ${krWeightDelta > 0 ? `не распределено ${krWeightDelta}%` : `превышено на ${-krWeightDelta}%`}</span>
      </div>` : ''}
      ${goal.krs.length
        ? goal.krs.map(kr => krRow(goal, kr, teamId)).join('')
        : '<div class="kr-section__empty">Ключевых результатов пока нет</div>'}
      ${structLock ? '' : `<button class="kr-add-btn" data-kr-add="${goal.id}">+ Добавить KR</button>`}
    </div>


    <button type="button" class="gc-comments-toggle${showCom ? ' gc-comments-toggle--open' : ''}${unresolved ? ' gc-comments-toggle--warn' : ''}" data-comments="${goal.id}" aria-expanded="${showCom ? 'true' : 'false'}">
      <span class="gc-comments-toggle__caret">▶</span>
      <span>Комментарии</span>
      ${goal.comments.length ? `<span class="gc-comments-toggle__count">· ${goal.comments.length}</span>` : ''}
      ${unresolved ? `<span class="gc-comments-toggle__pill">не решено ${unresolved}</span>` : ''}
    </button>
    ${showCom ? commentsPanel(goal) : ''}
  </div>`;
}

// ── Ряд управления показом целей ─────────────────────────────────────────────
// Повторяет BoardFilterBar из tracker.js: порядок, фильтр приоритета, две птички,
// сброс. Классы — общие .filter-bar* из components.css.
const PRI_LEVELS = ['P0', 'P1', 'P2', 'P3'];
const BOARD_SORTS = [['custom', 'Мой порядок'], ['priority', 'По приоритету']];

const boardViewDirty = () => {
  const v = state.boardView;
  return v.sort !== 'custom' || PRI_LEVELS.some(p => v.pri[p]) || v.notes || v.hideZeroWeight;
};

function boardFilters(goals) {
  const v = state.boardView;
  const anyPri = PRI_LEVELS.some(p => v.pri[p]);
  return `<div class="filter-bar">
    <div class="filter-bar__group">
      <span class="filter-bar__cap">Порядок</span>
      <div class="filter-bar__seg" role="group" aria-label="Порядок целей">
        ${BOARD_SORTS.map(([k, l]) => `<button type="button" class="filter-bar__btn${v.sort === k ? ' filter-bar__btn--on' : ''}" data-flt-sort="${k}" aria-pressed="${v.sort === k}">${l}</button>`).join('')}
      </div>
    </div>
    <div class="filter-bar__group">
      <span class="filter-bar__cap">Приоритет</span>
      <div class="filter-bar__seg" role="group" aria-label="Фильтр по приоритету">
        ${PRI_LEVELS.map(p => {
          const n = goals.filter(g => g.priority === p).length;
          const on = !!v.pri[p];
          return `<button type="button" class="filter-bar__btn filter-bar__btn--pri${on ? ' filter-bar__btn--on' : ''}" style="--pc:${PRI_COLOR[p]}" data-pri-filter="${p}" aria-pressed="${on}"${n ? '' : ' disabled'}>${p}<span class="filter-bar__n">${n}</span></button>`;
        }).join('')}
      </div>
      ${anyPri ? '<button type="button" class="filter-bar__clear" data-pri-clear="1" title="Сбросить фильтр по приоритету" aria-label="Сбросить фильтр по приоритету">×</button>' : ''}
    </div>
    <label class="filter-bar__check"><input type="checkbox" data-flt-notes="1"${v.notes ? ' checked' : ''}> Заметки к KR</label>
    <label class="filter-bar__check"><input type="checkbox" data-flt-zero="1"${v.hideZeroWeight ? ' checked' : ''}> Скрыть цели с весом 0</label>
    <span class="filter-bar__spacer"></span>
    ${boardViewDirty() ? '<button type="button" class="filter-bar__reset" data-flt-reset="1">Сбросить</button>' : ''}
  </div>`;
}

// Фильтры сужают показ, сортировка упорядочивает то, что осталось. Порядок шагов
// фиксирован: приоритет → нулевой вес → сортировка. Сортировка стабильная, поэтому
// цели одного приоритета сохраняют пользовательский порядок.
function shownGoals(goals) {
  const v = state.boardView;
  const sel = PRI_LEVELS.filter(p => v.pri[p]);
  let out = sel.length ? goals.filter(g => sel.includes(g.priority)) : goals.slice();
  if (v.hideZeroWeight) out = out.filter(g => (g.weight || 0) !== 0);
  if (v.sort === 'priority') out = out.slice().sort((a, b) => PRI_LEVELS.indexOf(a.priority) - PRI_LEVELS.indexOf(b.priority));
  return out;
}

function renderTracker() {
  const goals = goalsOf(state.teamId);
  if (!goals.length) {
    return `<div class="empty-state">
      <div class="empty-state__icon">🎯</div>
      <div class="empty-state__title">Целей на период нет</div>
      <div class="empty-state__text">Создайте первую цель, чтобы команда видела приоритеты квартала</div>
      <button class="empty-state__btn" data-modal="goal-new">Создать цель</button>
    </div>`;
  }
  const shown = shownGoals(goals);
  return `${boardFilters(goals)}
    ${shown.length
      ? shown.map(g => goalCard(g, state.teamId)).join('')
      : `<div class="empty-state">
          <div class="empty-state__icon">🔍</div>
          <div class="empty-state__title">Под настройки показа не подходит ни одна цель</div>
          <div class="empty-state__text">Цели у команды есть, но все они скрыты настройками показа</div>
          <button class="empty-state__btn" data-flt-reset="1">Сбросить настройки</button>
        </div>`}`;
}

// ── Лог активностей ──────────────────────────────────────────────────────────
// Повторяет Feed / EventRow / eventText / groupByTime из web/static/activity.js:
// вкладки категорий со счётчиками, фильтры, группировка по времени и строка
// события с иконкой категории, автором, бейджами команды и периода.
function fmtLogDate(iso) {
  const d = new Date(iso);
  const p = n => String(n).padStart(2, '0');
  return `${p(d.getDate())}/${p(d.getMonth() + 1)}/${String(d.getFullYear()).slice(2)}`;
}
function fmtLogDateTimeFull(iso) {
  const d = new Date(iso);
  const p = n => String(n).padStart(2, '0');
  return `${p(d.getDate())}.${p(d.getMonth() + 1)}.${d.getFullYear()} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}

const FIELD_RU = { weight: 'вес', priority: 'приоритет', title: 'название', desc: 'описание', focus: 'фокус', type: 'тип работы' };
const fieldsSummary = changed =>
  (changed || []).length ? `<span class="act-was"> (${changed.map(k => FIELD_RU[k] || k).join(', ')})</span>` : '';

function eventText(ev, teamNames) {
  const p = ev.payload || {};
  const t = esc(ev.entity_title || '');
  const teamList = ids => (ids || []).map(id => esc(teamNames[id] || `#${id}`)).join(', ');
  switch (ev.action) {
    case 'kr_progress': {
      const b = (p.before || {}).progress, a = (p.after || {}).progress;
      return `обновил KR «${t}»${p.goal_title ? ` цели «${esc(p.goal_title)}»` : ''} — <b>${a}%</b> <span class="act-was">(было ${b}%)</span>`;
    }
    case 'status_changed': {
      const b = STATUS_RU[(p.before || {}).status] || (p.before || {}).status;
      const a = STATUS_RU[(p.after || {}).status] || (p.after || {}).status;
      return `перевёл цели команды «${t}» в статус <b>«${a}»</b> <span class="act-was">(было «${b}»)</span>`;
    }
    case 'goal_created': return `создал цель «${t}»`;
    case 'goal_deleted': return `удалил цель «${t}»`;
    case 'kr_created': return `добавил KR «${t}»`;
    case 'kr_deleted': return `удалил KR «${t}»`;
    case 'goal_shared': {
      const ids = p.shared_with_team_ids || [];
      return `добавил к общей цели «${t}»${ids.length ? ` команды: <b>${teamList(ids)}</b>` : ''}`;
    }
    case 'goal_unshared': {
      const rem = p.unshared_team_ids;
      if (rem && rem.length) return `убрал из общей цели «${t}» команды: <b>${teamList(rem)}</b>`;
      return `отказался от общей цели «${t}»`;
    }
    case 'goal_owner_changed': return `сменил владельца цели «${t}»`;
    case 'goal_fields_changed': return `изменил цель «${t}»${fieldsSummary(p.changed)}`;
    case 'kr_fields_changed': return `изменил KR «${t}»${fieldsSummary(p.changed)}`;
    case 'kr_note_updated': return `обновил заметку к KR «${t}»`;
    case 'comment_added': return `оставил замечание к «${t}»`;
    case 'reply_added': return `ответил на замечание к «${t}»`;
    case 'comment_resolved': return `отметил замечание к «${t}» решённым`;
    case 'comment_reopened': return `переоткрыл замечание к «${t}»`;
    case 'comment_deleted': return `удалил замечание к «${t}»`;
    case 'reply_deleted': return `удалил ответ на замечание к «${t}»`;
    default: return `${esc(ev.action)} «${t}»`;
  }
}

// Текст комментария или заметки показывается markdown-блоком под строкой события.
const eventMarkdownBody = ev =>
  ['comment_added', 'reply_added'].includes(ev.action) ? (ev.payload || {}).text || null : null;

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
  return [['СЕГОДНЯ', groups.today], ['ВЧЕРА', groups.yesterday],
    ['РАНЕЕ НА ЭТОЙ НЕДЕЛЕ', groups.week], ['РАНЕЕ', groups.older]].filter(([, l]) => l.length > 0);
}

function activityRow(ev, teamNames, periodNames) {
  const actor = ev.actor || {};
  const name = actor.removed ? 'Бывший участник' : (actor.display_name || '—');
  const initials = (name || '?').trim().charAt(0).toUpperCase();
  const md = eventMarkdownBody(ev);
  return `<div class="act-row">
    <div class="act-row__icon act-row__icon--${ev.category}">${CATEGORY_ICON[ev.category] || '•'}</div>
    <div class="act-row__body">
      <div class="act-row__text">
        <span class="act-row__avatar act-row__avatar--fallback">${esc(initials)}</span>
        <b class="act-row__actor">${esc(name)}</b> ${eventText(ev, teamNames)}
      </div>
      ${md ? `<div class="act-row__md"><div class="md-content">${mdRender(md)}</div></div>` : ''}
      <div class="act-row__meta">
        ${ev.team_id != null ? `<span class="act-badge">${esc(teamNames[ev.team_id] || ('команда #' + ev.team_id))}</span>` : ''}
        ${ev.period_id != null ? `<span class="act-badge act-badge--period">${esc(periodNames[ev.period_id] || ('период #' + ev.period_id))}</span>` : ''}
        <a class="act-row__link" href="#">↗ к цели</a>
        <span class="act-row__time" title="${fmtLogDateTimeFull(ev.created_at)}">${fmtLogDate(ev.created_at)}</span>
      </div>
    </div>
  </div>`;
}

// Фильтрация повторяет параметры запроса Feed: период, команды, автор, диапазон, поиск.
function activityFiltered(withCategory) {
  const a = state.activity;
  const rangeMs = { today: 0, '7d': 7, '30d': 30 };
  const now = new Date();
  const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime();
  const q = (a.q || '').trim().toLowerCase();
  return ACTIVITY.filter(ev => {
    if (ev.period_id !== state.periodId) return false;
    if (a.favOnly && !state.favorites[ev.team_id]) return false;
    if (a.actor && (ev.actor || {}).display_name !== a.actor) return false;
    if (a.range !== 'all') {
      const t = new Date(ev.created_at).getTime();
      const from = a.range === 'today' ? startOfToday : startOfToday - rangeMs[a.range] * 864e5;
      if (t < from) return false;
    }
    if (q && !(ev.entity_title || '').toLowerCase().includes(q)
        && !((ev.payload || {}).text || '').toLowerCase().includes(q)) return false;
    if (withCategory && a.category && ev.category !== a.category) return false;
    return true;
  });
}

function renderActivity() {
  const a = state.activity;
  const teamNames = {};
  flatTeamsDepth().forEach(t => { teamNames[t.id] = t.name; });
  const periodNames = {};
  PERIODS.forEach(p => { periodNames[p.id] = p.name; });

  // Счётчики вкладок считаются без фильтра категории — как category-counts на сервере.
  const base = activityFiltered(false);
  const counts = {};
  ['progress', 'composition', 'status', 'discussion'].forEach(c => { counts[c] = base.filter(e => e.category === c).length; });
  const shown = activityFiltered(true);
  const groups = groupByTime(shown);
  const authors = [...new Set(ACTIVITY.map(e => (e.actor || {}).display_name).filter(Boolean))];

  return `<div class="act-main">
    <div class="act-topbar">
      <div class="act-tabs">
        <button class="act-tab${a.category === '' ? ' act-tab--on' : ''}" data-act-cat="">Все <span class="act-tab__n">${base.length}</span></button>
        ${['progress', 'composition', 'status', 'discussion'].map(c =>
          `<button class="act-tab${a.category === c ? ' act-tab--on' : ''}" data-act-cat="${c}">
            ${CATEGORY_ICON[c]} ${CATEGORY_LABEL[c]} <span class="act-tab__n">${counts[c]}</span></button>`).join('')}
      </div>
      <div class="act-filters">
        <div class="act-author">
          <select class="act-select" data-act-actor="1" style="width:100%">
            <option value="">Все авторы</option>
            ${authors.map(n => `<option${a.actor === n ? ' selected' : ''}>${esc(n)}</option>`).join('')}
          </select>
        </div>
        <button class="act-chip${a.favOnly ? ' act-chip--on' : ''}" data-act-fav="1">★ Избранное</button>
        <div class="act-range">
          ${[['all', 'Всё время'], ['today', 'Сегодня'], ['7d', '7 дней'], ['30d', '30 дней']].map(([v, l]) =>
            `<button class="act-range__btn${a.range === v ? ' act-range__btn--on' : ''}" data-act-range="${v}">${l}</button>`).join('')}
        </div>
        <input id="act-q" class="act-search" placeholder="Поиск по событиям…" value="${esc(a.q || '')}">
      </div>
    </div>
    <div class="act-feed" id="act-feed">
      ${shown.length === 0 ? '<div class="act-empty">Событий нет</div>' : ''}
      ${groups.map(([label, list]) => `<div class="act-group">
        <div class="act-group__label">${label}</div>
        ${list.map(ev => activityRow(ev, teamNames, periodNames)).join('')}
      </div>`).join('')}
      <div class="act-sentinel"></div>
    </div>
  </div>`;
}

// ── Сайдбар раздела «Дерево целей» ───────────────────────────────────────────
// Повторяет состав из GoalTreeApp: блок периода перед разделами (GtPeriodSelect),
// а под разделами — три переключателя фильтров и выбор корневой цели (RootPicker).
// Дерева команд здесь нет: оно принадлежит трекеру.
const GT_PERIOD_STATUS_COLOR = { future: '#3b82c4', active: '#1f9d55', closed: '#6b7280', archived: '#94a3b8' };

function gtPeriodSelect() {
  const cur = period();
  const pos = state.tree.periodRect || {};
  return `<div class="gt-psel${state.tree.periodMenu ? ' gt-psel--open' : ''}">
    <button type="button" class="gt-psel__trigger" data-gt-period="1">
      <span class="gt-psel__dot" style="background:${GT_PERIOD_STATUS_COLOR[cur.status] || '#94a3b8'}"></span>
      <span class="gt-psel__value">${esc(cur.name)}</span>
      <span class="gt-psel__chev">▾</span>
    </button>
    ${state.tree.periodMenu ? `<div class="gt-psel__panel" style="top:${pos.top || 0}px;left:${pos.left || 0}px;width:${pos.width || 220}px">
      ${PERIODS.map(p => `<button type="button" class="gt-psel__item${p.id === cur.id ? ' gt-psel__item--on' : ''}" data-gt-period-set="${p.id}">
        <span style="width:${(p.depth || 0) * 12}px;flex-shrink:0"></span>
        <span class="gt-psel__dot" style="background:${GT_PERIOD_STATUS_COLOR[p.status] || '#94a3b8'}"></span>
        <span class="gt-psel__item-name">${esc(p.name)}</span>
      </button>`).join('')}
    </div>` : ''}
  </div>`;
}

// Корневые цели — те, у кого нет родителей, но есть дети.
function gtRootPicker() {
  const raw = goalTreeData();
  const roots = raw.goals.filter(g => g.parent_goal_ids.length === 0 && g.child_goal_ids.length > 0);
  const q = (state.tree.rootQ || '').trim().toLowerCase();
  const teamsById = new Map(raw.teams.map(t => [t.id, t]));
  const periodsById = new Map(PERIODS.map(p => [p.id, p]));
  const filtered = roots.filter(g => !q || g.title.toLowerCase().includes(q)
    || ((teamsById.get(g.team_id) || {}).name || '').toLowerCase().includes(q));
  const byTeam = new Map();
  filtered.forEach(g => { if (!byTeam.has(g.team_id)) byTeam.set(g.team_id, []); byTeam.get(g.team_id).push(g); });
  // Показываем команды с корнями и всех их предков, чтобы дерево было полным.
  const shown = new Set();
  byTeam.forEach((_, tid) => {
    let cur = tid;
    while (cur != null && teamsById.has(cur)) {
      if (shown.has(cur)) break;
      shown.add(cur);
      cur = (teamsById.get(cur) || {}).parentId;
    }
  });
  const order = new Map(raw.teams.map((t, i) => [t.id, i]));
  const teams = [...shown].sort((a, b) => (order.get(a) ?? 0) - (order.get(b) ?? 0));
  const selected = state.tree.rootId != null ? roots.find(g => g.id === state.tree.rootId) : null;
  const pos = state.tree.rootRect || {};
  const TAB = 16;

  return `<div class="gt-rootpick${state.tree.rootOpen ? ' gt-rootpick--open' : ''}">
    <button type="button" class="gt-rootpick__trigger" data-gt-root="1">
      <span class="gt-rootpick__value">${esc(selected ? selected.title : 'Все корневые цели')}</span>
      <span class="gt-rootpick__chev">▾</span>
    </button>
    ${state.tree.rootOpen ? `<div class="gt-rootpick__panel" style="top:${pos.top || 0}px;left:${pos.left || 0}px">
      <input id="gt-root-q" class="gt-rootpick__search" placeholder="Поиск по цели или команде…" value="${esc(state.tree.rootQ || '')}">
      <div class="gt-rootpick__list" id="gt-root-list">${gtRootListHTML(teams, byTeam, teamsById, periodsById, TAB)}</div>
    </div>` : ''}
  </div>`;
}

function gtRootListHTML(teams, byTeam, teamsById, periodsById, TAB) {
  const depthOf = id => (flatTeamsDepth().find(t => t.id === id) || {}).depth || 0;
  return `<button type="button" class="gt-rootpick__all${state.tree.rootId == null ? ' gt-rootpick__all--on' : ''}" data-gt-root-set="all">Все корневые цели</button>
    ${teams.length ? '' : '<div class="gt-rootpick__empty">Ничего не найдено</div>'}
    ${teams.map(tid => {
      const t = teamsById.get(tid);
      const gs = byTeam.get(tid) || [];
      const indent = 8 + depthOf(tid) * TAB;
      const rowIndent = indent + TAB;
      return `<div class="gt-rootpick__group">
        <div class="gt-rootpick__group-head" style="padding-left:${indent}px">
          <span class="gt-rootpick__group-dot" style="background:${GT_TEAM_TYPE_COLOR[t.type] || '#94a3b8'}"></span>
          <span class="gt-rootpick__group-name">${esc(t.name)}</span>
          <span class="gt-rootpick__group-type">${esc((TEAM_TYPE_LABEL[t.type] || '').toUpperCase())}</span>
        </div>
        ${gs.map(g => {
          const per = periodsById.get(g.period_id);
          const on = g.id === state.tree.rootId;
          return `<button type="button" style="padding-left:${rowIndent}px" class="gt-rootpick__row${on ? ' gt-rootpick__row--on' : ''}" data-gt-root-set="${g.id}">
            <span class="gt-rootpick__row-title">${esc(g.title)}</span>
            <span class="gt-rootpick__row-meta">${esc(per ? per.name : '')} · ${g.progress}%</span>
            ${on ? '<span class="gt-rootpick__row-check">✓</span>' : ''}
          </button>`;
        }).join('')}
      </div>`;
    }).join('')}`;
}

function gtSidebarControls() {
  const f = state.tree;
  const toggle = (key, label) =>
    `<label class="gt-toggle"><input type="checkbox" data-gt-flag="${key}"${f[key] ? ' checked' : ''}> ${label}</label>`;
  return `<div class="gt-controls">
    ${toggle('crossPeriod', 'Связи между периодами')}
    ${toggle('hideUnlinked', 'Скрыть цели без связи')}
    ${toggle('myGoals', 'Мои цели')}
    <div class="gt-controls__root">
      <div class="gt-controls__label">Корневая цель</div>
      ${gtRootPicker()}
    </div>
  </div>`;
}

// ── Дерево целей ─────────────────────────────────────────────────────────────
// Раскладка перенесена из web/static/goal_tree_layout.js дословно: слоёная
// (Sugiyama-подобная) схема — бэнды по глубине периода, tidy-раскладка по X,
// цели без связей отдельной сеткой снизу.
const CARD = { cardW: 224, cardH: 112, hGap: 32, vGap: 50, bandGap: 64 };
const GT_TIER_COLOR = { 0: '#6366f1', 1: '#14b8a6', 2: '#f59e0b', 3: '#ec4899', u: '#94a3b8' };
const GT_TEAM_TYPE_COLOR = { department: '#6366f1', cluster: '#0ea5e9', unit: '#14b8a6', group: '#f59e0b', team: '#22c55e', squad: '#ec4899', employee: '#94a3b8' };
const GT_PRI_COLOR = { P0: '#dc2626', P1: '#d97706', P2: '#2563eb', P3: '#6b7280' };
const gtTierColor = p => (!p ? GT_TIER_COLOR.u : GT_TIER_COLOR[Math.min(p.depth, 3)] || GT_TIER_COLOR.u);
const gtProgressColor = p => (p >= 50 ? '#16a34a' : p >= 25 ? '#d97706' : '#dc2626');

function orthEdgePath(x1, y1, x2, y2, r, turnY) {
  const midY = (turnY == null) ? (y1 + y2) / 2 : turnY;
  const dir = x2 >= x1 ? 1 : -1;
  const rr = Math.min(r, Math.abs(x2 - x1) / 2, Math.abs(midY - y1), Math.abs(y2 - midY));
  if (rr < 1 || Math.abs(x2 - x1) < 1) return `M ${x1} ${y1} L ${x1} ${midY} L ${x2} ${midY} L ${x2} ${y2}`;
  return [`M ${x1} ${y1}`, `L ${x1} ${midY - rr}`, `Q ${x1} ${midY} ${x1 + dir * rr} ${midY}`,
    `L ${x2 - dir * rr} ${midY}`, `Q ${x2} ${midY} ${x2} ${midY + rr}`, `L ${x2} ${y2}`].join(' ');
}

function computeGoalTreeLayout(data, opts) {
  const cardW = opts.cardW, cardH = opts.cardH;
  const hGap = opts.hGap, vGap = opts.vGap, bandGap = opts.bandGap;
  const colPitch = cardW + hGap, rowPitch = cardH + vGap;
  const padTop = 70, padBottom = 16, margin = 40;
  const periodById = new Map(data.periods.map(p => [p.id, p]));
  const goalById = new Map(data.goals.map(g => [g.id, g]));
  const teamOrder = new Map(data.teams.map((t, i) => [t.id, i]));
  const teamName = new Map(data.teams.map(t => [t.id, t.name]));
  const periodRank = (opts.periodRank instanceof Map) ? opts.periodRank : null;
  const pr = g => (periodRank ? (periodRank.get(g.period_id) ?? 0) : 0);
  const cmp = (a, b) => pr(a) - pr(b) || (teamOrder.get(a.team_id) ?? 1e9) - (teamOrder.get(b.team_id) ?? 1e9) || a.id - b.id;
  const hasLink = g => (g.parent_goal_ids.length + g.child_goal_ids.length) > 0;
  const periodDepth = g => { const p = periodById.get(g.period_id); return p ? p.depth : 0; };
  const linkedGoals = data.goals.filter(hasLink);
  const unlinkedGoals = data.goals.filter(g => !hasLink(g)).sort(cmp);

  const linkedDepths = Array.from(new Set(linkedGoals.map(periodDepth))).sort((a, b) => a - b);
  const linkedByDepth = new Map(linkedDepths.map(d => [d, []]));
  for (const g of linkedGoals) linkedByDepth.get(periodDepth(g)).push(g);

  const sublevel = new Map();
  const sameBandParents = g => g.parent_goal_ids.filter(pid => {
    const p = goalById.get(pid);
    return p && hasLink(p) && periodDepth(p) === periodDepth(g);
  });
  const computeSub = (g, stack) => {
    if (sublevel.has(g.id)) return sublevel.get(g.id);
    let m = 0;
    for (const pid of sameBandParents(g)) {
      if (stack.has(pid)) continue;
      stack.add(pid);
      m = Math.max(m, computeSub(goalById.get(pid), stack) + 1);
      stack.delete(pid);
    }
    sublevel.set(g.id, m);
    return m;
  };
  for (const g of linkedGoals) computeSub(g, new Set([g.id]));

  const rowY = new Map();
  const bands = [];
  let runningY = 0;
  for (const d of linkedDepths) {
    const gs = linkedByDepth.get(d);
    let maxSub = 0;
    for (const g of gs) maxSub = Math.max(maxSub, sublevel.get(g.id));
    const bandTop = runningY, firstRowY = bandTop + padTop;
    for (let s = 0; s <= maxSub; s++) rowY.set(`${d}|${s}`, firstRowY + s * rowPitch);
    const bandHeight = (firstRowY + maxSub * rowPitch + cardH + padBottom) - bandTop;
    const name = d === 0 ? 'Годовые цели' : d === 1 ? 'Квартальные цели' : `Уровень ${d}`;
    bands.push({ depth: d, name, count: gs.length, y: bandTop, height: bandHeight });
    runningY = bandTop + bandHeight + bandGap;
  }

  const rowKeyOf = g => `${periodDepth(g)}|${sublevel.get(g.id)}`;
  const nodeById = new Map();
  const nodes = [];
  for (const g of linkedGoals) {
    const n = { id: g.id, goal: g, x: 0, y: rowY.get(rowKeyOf(g)), isRoot: g.parent_goal_ids.length === 0, teamId: g.team_id, rowKey: rowKeyOf(g) };
    nodes.push(n); nodeById.set(g.id, n);
  }

  const treeChildren = new Map();
  const inForest = new Set();
  const roots = linkedGoals.filter(g => g.parent_goal_ids.length === 0).sort(cmp);
  const buildForest = id => {
    if (inForest.has(id)) return;
    inForest.add(id);
    const kids = (goalById.get(id).child_goal_ids || []).slice()
      .filter(c => nodeById.has(c) && !inForest.has(c))
      .sort((a, b) => cmp(goalById.get(a), goalById.get(b)));
    treeChildren.set(id, kids);
    for (const c of kids) buildForest(c);
  };
  for (const r of roots) buildForest(r.id);
  for (const g of linkedGoals) if (!inForest.has(g.id)) { buildForest(g.id); roots.push(g); }

  const rowCursor = new Map();
  const nextX = rk => (rowCursor.has(rk) ? rowCursor.get(rk) : 0);
  const bump = (rk, x) => rowCursor.set(rk, x + colPitch);
  const shiftSubtree = (id, dx, vis) => {
    if (vis.has(id)) return;
    vis.add(id);
    const n = nodeById.get(id);
    n.x += dx; bump(n.rowKey, n.x);
    for (const c of (treeChildren.get(id) || [])) shiftSubtree(c, dx, vis);
  };
  const place = id => {
    const n = nodeById.get(id);
    const kids = treeChildren.get(id) || [];
    if (kids.length === 0) { n.x = nextX(n.rowKey); bump(n.rowKey, n.x); return; }
    for (const c of kids) place(c);
    const xs = kids.map(c => nodeById.get(c).x);
    let cx = (Math.min(...xs) + Math.max(...xs)) / 2;
    const minHere = nextX(n.rowKey);
    if (cx < minHere) { shiftSubtree(id, minHere - cx, new Set()); cx = minHere; }
    n.x = cx; bump(n.rowKey, n.x);
  };
  for (const r of roots) place(r.id);

  let minX = Infinity;
  for (const n of nodes) if (n.x < minX) minX = n.x;
  if (!isFinite(minX)) minX = 0;
  for (const n of nodes) n.x = n.x - minX + margin;
  const linkedRight = nodes.length ? Math.max(...nodes.map(n => n.x + cardW)) : margin;

  if (unlinkedGoals.length) {
    let cols = nodes.length ? Math.max(1, Math.round((linkedRight - margin) / colPitch))
                            : Math.max(1, Math.round(Math.sqrt(unlinkedGoals.length)));
    cols = Math.min(cols, unlinkedGoals.length);
    const rows = Math.ceil(unlinkedGoals.length / cols);
    const bandTop = nodes.length ? runningY : 0;
    const firstRowY = bandTop + padTop;
    unlinkedGoals.forEach((g, i) => {
      const n = { id: g.id, goal: g, x: margin + (i % cols) * colPitch, y: firstRowY + Math.floor(i / cols) * rowPitch, isRoot: false, teamId: g.team_id, rowKey: 'unlinked' };
      nodes.push(n); nodeById.set(g.id, n);
    });
    const bandHeight = (firstRowY + (rows - 1) * rowPitch + cardH + padBottom) - bandTop;
    bands.push({ depth: null, name: 'Без связей', count: unlinkedGoals.length, y: bandTop, height: bandHeight });
    runningY = bandTop + bandHeight + bandGap;
  }

  const edges = [];
  for (const g of linkedGoals) {
    const child = nodeById.get(g.id);
    if (!child) continue;
    for (const pid of g.parent_goal_ids) {
      const parent = nodeById.get(pid);
      if (!parent) continue;
      const x1 = parent.x + cardW / 2, y1 = parent.y + cardH;
      const x2 = child.x + cardW / 2, y2 = child.y;
      const turnY = Math.min(y1 + 22, y2 - 30);
      edges.push({ from: pid, to: g.id, path: orthEdgePath(x1, y1, x2, y2, 12, turnY) });
    }
  }

  const byRowTeamParent = new Map();
  for (const g of linkedGoals) {
    const n = nodeById.get(g.id);
    if (!n || g.parent_goal_ids.length === 0) continue;
    for (const pid of g.parent_goal_ids) {
      const k = `${n.rowKey}|${g.team_id}|${pid}`;
      if (!byRowTeamParent.has(k)) byRowTeamParent.set(k, []);
      byRowTeamParent.get(k).push(n);
    }
  }
  const groups = [];
  const groupHeader = 20, groupPadX = 16, groupPadBottom = 16;
  for (const [key, ns] of byRowTeamParent) {
    if (ns.length < 2) continue;
    const minGX = Math.min(...ns.map(n => n.x)) - groupPadX;
    const maxGX = Math.max(...ns.map(n => n.x + cardW)) + groupPadX;
    const parts = key.split('|');
    groups.push({ teamId: ns[0].teamId, teamName: teamName.get(ns[0].teamId) || '', parentGoalId: Number(parts[parts.length - 1]),
      x: minGX, y: ns[0].y - groupHeader, w: maxGX - minGX, h: groupHeader + cardH + groupPadBottom, nodeIds: ns.map(n => n.id) });
  }

  const width = Math.max(margin, ...nodes.map(n => n.x + cardW)) + margin;
  const height = (bands.length ? bands[bands.length - 1].y + bands[bands.length - 1].height : 0) + 40;
  return { bands, nodes, edges, groups, width, height };
}

// Данные графа в том же виде, в каком их отдаёт GET /api/v1/goal-tree.
// Показываем цели всех периодов сразу: в продукте это режим «связи между периодами».
function goalTreeData() {
  const teams = flatTeamsDepth();
  const goals = [];
  teams.forEach(t => {
    (BOARDS[t.id] || { goals: [] }).goals.forEach(g => goals.push({
      id: g.id, title: g.title, team_id: t.id, period_id: g.periodId,
      parent_goal_ids: [...g.parentIds], child_goal_ids: [...g.childIds],
      progress: goalProgress(g), priority: g.priority, weight: g.weight,
      focus_type: g.focus, owner_text: (g.owners || []).map(o => o.name).join(', '),
    }));
  });
  return { teams, goals, periods: PERIODS };
}

function goalTreeCard(n, byId) {
  const g = n.goal;
  const team = byId.teams.get(g.team_id);
  const per = byId.periods.get(g.period_id);
  const accent = gtTierColor(per);
  const hl = state.tree.highlight;
  const cls = ['gt-card', n.isRoot ? 'gt-card--root' : '',
    hl && hl.has(n.id) ? 'gt-card--hl' : '', hl && !hl.has(n.id) ? 'gt-card--dim' : ''].filter(Boolean).join(' ');
  const hasChildren = g.child_goal_ids.length > 0;
  const collapsed = state.tree.collapsed.has(n.id);
  return `<div class="${cls}" style="left:${n.x}px;top:${n.y}px;width:${CARD.cardW}px;height:${CARD.cardH}px;--gt-accent:${accent}" data-gt-card="${n.id}">
    <i class="gt-card__strip" style="background:${accent}"></i>
    ${hasChildren ? `<button type="button" class="gt-collapse-btn" data-gt-collapse="${n.id}"
      title="${collapsed ? 'Развернуть поддерево' : 'Свернуть поддерево'}">${collapsed ? '▸' : '▾'}</button>` : ''}
    <div class="gt-card__title">${esc(g.title)}</div>
    <div class="gt-card__meta">
      <span class="gt-card__team"><span class="gt-card__dot"></span><span class="gt-card__teamname">${esc(team ? team.name : '')}</span></span>
      ${per ? `<span class="gt-card__period">${esc(per.name)}</span>` : ''}
      <span class="gt-card__pr" style="color:${gtProgressColor(g.progress)}">${g.progress}%</span>
    </div>
  </div>`;
}

function goalTreePanel(goal, data) {
  const teams = new Map(data.teams.map(t => [t.id, t]));
  const periods = new Map(data.periods.map(p => [p.id, p]));
  const goals = new Map(data.goals.map(g => [g.id, g]));
  const team = teams.get(goal.team_id), per = periods.get(goal.period_id);
  const pathTeams = teamPath(goal.team_id) || [];
  const parents = goal.parent_goal_ids.map(id => goals.get(id)).filter(Boolean);
  return `<div class="gt-panel" role="dialog" aria-label="Детали цели">
    <div class="gt-panel__header">
      <div class="gt-panel__title">${esc(goal.title)}</div>
      <button type="button" class="gt-panel__close" aria-label="Закрыть" data-gt-close="1">×</button>
    </div>
    <div class="gt-panel__body">
      ${pathTeams.length ? `<div class="gt-panel__path">${pathTeams.map((nm, i) =>
        `${i > 0 ? '<span class="gt-panel__path-sep"> / </span>' : ''}<a class="gt-panel__path-link" href="#">${esc(nm)}</a>`).join('')}</div>` : ''}
      <div class="gt-panel__badges">
        ${per ? `<span class="gt-panel__pill">${esc(per.name)}</span>` : ''}
        ${badge(goal.priority, GT_PRI_COLOR[goal.priority] || '#6b7280')}
        <span class="gt-panel__muted">вес ${goal.weight}%</span>
        <span class="gt-panel__muted">${esc(FOCUS_LABEL[goal.focus_type] || '')}</span>
      </div>
      <div class="gt-panel__barwrap">
        <div class="gt-panel__bar"><div class="gt-panel__barfill" style="width:${goal.progress}%"></div></div>
        <span class="gt-panel__pct">${goal.progress}%</span>
      </div>
      <div class="gt-panel__links">
        <span class="gt-panel__updown">↑ выше: <b>${goal.parent_goal_ids.length}</b></span>
        <span class="gt-panel__updown">↓ ниже: <b>${goal.child_goal_ids.length}</b></span>
      </div>
      ${parents.length ? `<div class="gt-panel__section">
        <div class="gt-panel__section-title">Вышестоящие цели</div>
        ${parents.map(p => `<div class="gt-panel__parent">
          <div class="gt-panel__parent-title">${esc(p.title)}</div>
          <div class="gt-panel__parent-meta">${esc((periods.get(p.period_id) || {}).name || '')} · ${esc((teams.get(p.team_id) || {}).name || '')}</div>
        </div>`).join('')}
      </div>` : ''}
      ${goal.owner_text ? `<div class="gt-panel__row">
        <span class="gt-panel__label">Драйвер</span><span class="gt-panel__value">${esc(goal.owner_text)}</span>
      </div>` : ''}
    </div>
    <div class="gt-panel__footer">
      <button type="button" class="gt-panel__toggle${state.tree.focused ? ' gt-panel__toggle--on' : ''}" data-gt-focus="1">
        ${state.tree.focused ? '◉ Показать все' : '◎ Скрыть остальные'}</button>
      <a class="gt-panel__open" href="#">↗ Открыть в трекере</a>
    </div>
  </div>`;
}

function renderGoalTree() {
  const raw = goalTreeData();
  const byIdAll = new Map(raw.goals.map(g => [g.id, g]));
  let goals = raw.goals;

  // Шаг 1 — период. При «Связи между периодами» набор расширяется транзитивно
  // по связям вверх и вниз, включая цели других периодов.
  const inPeriod = new Set(goals.filter(g => g.period_id === state.periodId).map(g => g.id));
  if (state.tree.crossPeriod) {
    const stack = [...inPeriod];
    while (stack.length) {
      const cur = byIdAll.get(stack.pop());
      if (!cur) continue;
      for (const nx of [...cur.parent_goal_ids, ...cur.child_goal_ids]) {
        if (!inPeriod.has(nx)) { inPeriod.add(nx); stack.push(nx); }
      }
    }
  }
  goals = goals.filter(g => inPeriod.has(g.id));

  // «Мои цели» — в продукте это цели, к которым причастен текущий пользователь;
  // в прототипе берём команды из избранного.
  if (state.tree.myGoals) goals = goals.filter(g => state.favorites[g.team_id]);
  if (state.tree.hideUnlinked) {
    goals = goals.filter(g => g.parent_goal_ids.length + g.child_goal_ids.length > 0);
  }
  if (state.tree.rootId != null) {
    const keep = new Set([state.tree.rootId]);
    const walk = (id, key) => { for (const nx of (byIdAll.get(id) || {})[key] || []) if (!keep.has(nx)) { keep.add(nx); walk(nx, key); } };
    walk(state.tree.rootId, 'parent_goal_ids');
    walk(state.tree.rootId, 'child_goal_ids');
    goals = goals.filter(g => keep.has(g.id));
  }
  if (state.tree.onlyRoots) goals = goals.filter(g => g.parent_goal_ids.length === 0);
  // Свёрнутое поддерево: прячем цели, у которых все родители свёрнуты.
  if (state.tree.collapsed.size) {
    const hidden = new Set();
    let changed = true;
    while (changed) {
      changed = false;
      for (const g of goals) {
        if (hidden.has(g.id) || state.tree.collapsed.has(g.id)) continue;
        const ps = g.parent_goal_ids;
        if (ps.length && ps.every(p => state.tree.collapsed.has(p) || hidden.has(p))) { hidden.add(g.id); changed = true; }
      }
    }
    goals = goals.filter(g => !hidden.has(g.id));
  }
  if (state.tree.focused && state.tree.selectedId != null) {
    const keep = new Set([state.tree.selectedId]);
    const byId = new Map(raw.goals.map(g => [g.id, g]));
    const walk = (id, key) => { for (const nx of (byId.get(id) || {})[key] || []) if (!keep.has(nx)) { keep.add(nx); walk(nx, key); } };
    walk(state.tree.selectedId, 'parent_goal_ids');
    walk(state.tree.selectedId, 'child_goal_ids');
    goals = goals.filter(g => keep.has(g.id));
  }

  const ids = new Set(goals.map(g => g.id));
  const data = { teams: raw.teams, periods: raw.periods,
    goals: goals.map(g => ({ ...g,
      parent_goal_ids: g.parent_goal_ids.filter(p => ids.has(p)),
      child_goal_ids: g.child_goal_ids.filter(c => ids.has(c)) })) };

  // Подсветка: транзитивные связи выбранной цели вверх и вниз.
  state.tree.highlight = null;
  if (state.tree.selectedId != null && ids.has(state.tree.selectedId)) {
    const byId = new Map(data.goals.map(g => [g.id, g]));
    const seen = new Set([state.tree.selectedId]);
    const walk = (start, key) => {
      const stack = [start];
      while (stack.length) {
        const cur = stack.pop();
        for (const nx of (byId.get(cur) || {})[key] || []) if (!seen.has(nx)) { seen.add(nx); stack.push(nx); }
      }
    };
    walk(state.tree.selectedId, 'parent_goal_ids');
    walk(state.tree.selectedId, 'child_goal_ids');
    state.tree.highlight = seen;
  }

  const periodRank = new Map(PERIODS.map(p => [p.id, p.rank]));
  const layout = computeGoalTreeLayout(data, { ...CARD, periodRank });
  const byId = { teams: new Map(data.teams.map(t => [t.id, t])), periods: new Map(data.periods.map(p => [p.id, p])) };
  const zoom = state.tree.zoom;
  const selected = data.goals.find(g => g.id === state.tree.selectedId);

  return `<div class="gt-main">
    <div class="gt-topbar">
      <label class="gt-topbar__toggle"><input type="checkbox" data-gt-roots="1"${state.tree.onlyRoots ? ' checked' : ''}> Показать только корневые цели</label>
      ${state.tree.selectedId != null || state.tree.collapsed.size || state.tree.onlyRoots
        ? '<button class="gt-topbar__reset" data-gt-reset="1">Сбросить фильтры</button>' : ''}
    </div>
    <div class="gt-main__body">
      ${data.goals.length === 0 ? '<div class="gt-state">Нет целей со связями</div>' : ''}
      <div class="gt-canvas-wrap"${data.goals.length === 0 ? ' hidden' : ''}>
        <div class="gt-viewport" id="gt-viewport">
          <div class="gt-zoom" style="width:${layout.width * zoom}px;height:${layout.height * zoom}px">
            <div class="gt-canvas" style="width:${layout.width}px;height:${layout.height}px;transform:scale(${zoom});transform-origin:0 0">
              ${layout.bands.map(b => {
                const tier = b.depth == null ? 'u' : 'd' + Math.min(b.depth, 3);
                return `<div class="gt-band gt-band--${tier}" style="top:${b.y}px;height:${b.height}px">
                  <span class="gt-band__label"><span class="gt-band__marker">◆</span><span class="gt-band__name">${b.name}</span><span class="gt-band__count">${b.count}</span></span>
                </div>`;
              }).join('')}
              ${layout.groups.map(gr => {
                const first = data.goals.find(g => g.id === gr.nodeIds[0]);
                const dot = gtTierColor(first ? byId.periods.get(first.period_id) : null);
                const parentGoal = data.goals.find(g => g.id === gr.parentGoalId);
                const rawTitle = parentGoal ? parentGoal.title : gr.teamName;
                const short = rawTitle.length > 32 ? rawTitle.slice(0, 32).trim() + '…' : rawTitle;
                return `<div class="gt-group" style="left:${gr.x}px;top:${gr.y}px;width:${gr.w}px;height:${gr.h}px">
                  <span class="gt-group__label" style="max-width:${gr.w - 32}px" title="Группа целей | ${esc(rawTitle)} | ${esc(gr.teamName)}">
                    <span class="gt-group__dot" style="background:${dot}"></span>
                    <span class="gt-group__label-text">Группа целей | ${esc(short)} | ${esc(gr.teamName)}</span>
                  </span>
                </div>`;
              }).join('')}
              <svg class="gt-edges" width="${layout.width}" height="${layout.height}">
                ${layout.edges.map(e => {
                  const hl = state.tree.highlight;
                  const on = !hl || (hl.has(e.from) && hl.has(e.to));
                  return `<path d="${e.path}" class="gt-edge${hl && !on ? ' gt-edge--dim' : ''}" fill="none"></path>`;
                }).join('')}
              </svg>
              ${layout.nodes.map(n => goalTreeCard(n, byId)).join('')}
            </div>
          </div>
        </div>
        <div class="gt-zoom-ctrl">
          <button type="button" title="Отдалить" data-gt-zoom="out">−</button>
          <button type="button" class="gt-zoom-ctrl__num" title="Сбросить масштаб" data-gt-zoom="reset">${Math.round(zoom * 100)}%</button>
          <button type="button" title="Приблизить" data-gt-zoom="in">+</button>
          <button type="button" title="Вписать в экран" data-gt-zoom="fit">⤢</button>
        </div>
      </div>
      ${selected ? goalTreePanel(selected, data) : ''}
    </div>
  </div>`;
}

// ── Обзор периода ────────────────────────────────────────────────────────────
// Структура повторяет PeriodOverviewContent из web/static/period_overview_view.js.
// Тот экран написан целиком на инлайн-стилях, без CSS-классов, — здесь так же,
// чтобы вёрстка переносилась обратно один в один.
function overviewTeams() {
  return flatTeams().map(t => {
    const goals = goalsOf(t.id);
    const ws = goals.reduce((s, g) => s + g.weight, 0);
    const p = teamProgress(t.id);
    return {
      id: t.id, name: t.name, path: teamPath(t.id) || [t.name],
      status: boardOf(t.id).status,
      goals_count: goals.length,
      progress: p == null ? 0 : p,
      weight_sum: ws,
      weight_error: goals.length > 0 && ws !== 100,
    };
  });
}
const allGoals = () => flatTeams().flatMap(t => goalsOf(t.id).map(g => ({ ...g, team_id: t.id, team_name: t.name })));
const allKRs = () => allGoals().flatMap(g => g.krs.map(kr => ({ ...kr, team_name: g.team_name, goal_title: g.title })));

function balanceItems(rows, field, keys) {
  const total = rows.length || 1;
  return keys.map(k => {
    const count = rows.filter(r => r[field] === k).length;
    return { key: k, count, percent: Math.round(count / total * 100) };
  });
}

const poEyebrow = (text, margin) =>
  `<div style="font-size: 12px;color:${PO.dimFg};font-weight:700;text-transform:uppercase;letter-spacing:.5px;margin:${margin}">${text}</div>`;

const poTile = (label, value, sub, accent, attrs) =>
  `<div style="flex:1 1 150px;min-width:140px;background:white;border:1px solid ${PO.cardBorder};border-radius:12px;padding:14px 16px;cursor:${attrs ? 'pointer' : 'default'}"${attrs ? ' ' + attrs : ''}>
    <div style="font-size: 13px;color:${PO.mutedFg};font-weight:600">${label}</div>
    <div style="font-size: 27px;font-weight:800;color:${accent || PO.headingFg};margin-top:4px">${value}</div>
    ${sub ? `<div style="font-size: 12px;color:${PO.dimFg};margin-top:2px">${sub}</div>` : ''}
  </div>`;

function balanceBars(title, subtitle, items, labels, colors, field) {
  const max = Math.max(1, ...items.map(i => i.count));
  return `<div style="flex:1 1 260px;min-width:240px">
    <div style="font-weight:700;font-size: 15.5px;color:${PO.headingFg}">${esc(title)}</div>
    <div style="font-size: 13px;color:#64748b;margin:2px 0 10px">${esc(subtitle)}</div>
    ${items.map(it => `<div style="display:grid;grid-template-columns:130px 1fr 72px;align-items:center;gap:10px;padding:5px 0;cursor:${it.count ? 'pointer' : 'default'}"
        ${it.count ? `data-bal="${field}:${it.key}"` : ''}>
      <div style="font-size: 14px;color:${it.count ? PO.headingFg : '#94a3b8'}">${esc(labels[it.key] || it.key)}</div>
      <div style="height:10px;background:#eef2f7;border-radius:6px">
        <div style="width:${(it.count / max) * 100}%;height:100%;border-radius:6px;background:${colors[it.key] || '#7c6cf0'}"></div>
      </div>
      <div style="font-size: 14px;text-align:right;color:#334155"><b>${it.count}</b> · ${it.percent}%</div>
    </div>`).join('')}
  </div>`;
}

const poDrillRow = (left, right, danger) =>
  `<div style="display:flex;justify-content:space-between;gap:10px;padding:8px 14px;border-top:1px solid ${PO.hairline};font-size: 13.5px">
    <span style="color:${PO.headingFg};overflow:hidden;text-overflow:ellipsis;white-space:nowrap">${left}</span>
    <span style="color:${danger ? PO.danger : PO.mutedFg};flex-shrink:0">${right}</span>
  </div>`;

const poDrillPanel = (title, rows) =>
  `<div style="margin-top:14px;border:1px solid ${PO.cardBorder};border-radius:12px;overflow:hidden">
    <div style="padding:10px 14px;background:#f8fafc;display:flex;justify-content:space-between;align-items:center">
      <span style="font-size: 13.5px;font-weight:700;color:${PO.headingFg}">${esc(title)} · ${rows.length}</span>
      <button data-drill-close="1" style="background:none;border:none;cursor:pointer;color:${PO.mutedFg};font-size: 17px">×</button>
    </div>
    <div style="max-height:220px;overflow-y:auto">
      ${rows.length ? rows.join('') : `<div style="padding:16px;text-align:center;color:${PO.dimFg};font-size: 13.5px">Пусто</div>`}
    </div>
  </div>`;

function progressChartSVG() {
  const series = PROGRESS_SERIES;
  const W = 900, H = 320, padL = 40, padR = 20, padT = 20, padB = 30;
  const x0 = padL, x1 = W - padR, y0 = H - padB, y1 = padT;
  const start = Date.parse(series.period_start), end = Date.parse(series.period_end);
  const span = Math.max(1, end - start);
  const xOf = d => { const t = Math.min(Math.max(Date.parse(d), start), end); return x0 + ((t - start) / span) * (x1 - x0); };
  const yOf = p => y0 + (Math.min(Math.max(p, 0), 100) / 100) * (y1 - y0);
  const pts = series.points.map(pt => ({
    x: xOf(pt.date), y: yOf(pt.progress),
    edge: Date.parse(pt.date) < start || Date.parse(pt.date) > end, p: pt.progress, date: pt.date,
  }));
  const line = [{ x: x0, y: y0 }, ...pts].map((p, i) => `${i ? 'L' : 'M'}${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(' ');
  const last = pts[pts.length - 1];
  const area = `${line} L${last.x.toFixed(1)},${y0} L${x0},${y0} Z`;
  const hi = state.chartHover, hp = hi != null ? pts[hi] : null;
  const MONTHS = ['янв', 'фев', 'мар', 'апр', 'мая', 'июн', 'июл', 'авг', 'сен', 'окт', 'ноя', 'дек'];
  const fmtD = iso => { const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(iso); return m ? `${+m[3]} ${MONTHS[+m[2] - 1]} ${m[1]}` : iso; };
  const tip = hp ? `${fmtD(hp.date)} · ${hp.p}%` : '';
  const tipW = Math.max(70, tip.length * 6.6 + 16);
  const tipX = hp ? Math.min(Math.max(hp.x - tipW / 2, x0), x1 - tipW) : 0;
  const tipY = hp ? Math.max(hp.y - 34, y1) : 0;
  return `<svg viewBox="0 0 ${W} ${H}" style="width:100%;height:auto">
    ${[0, 25, 50, 75, 100].map(g => `<line x1="${x0}" y1="${yOf(g)}" x2="${x1}" y2="${yOf(g)}" stroke="#eef2f7"/>
      <text x="${x0 - 6}" y="${yOf(g) + 3}" text-anchor="end" font-size="10" fill="#94a3b8">${g}%</text>`).join('')}
    <line x1="${x0}" y1="${y0}" x2="${x1}" y2="${y1}" stroke="#cbd5e1" stroke-dasharray="4 4"/>
    <path d="${area}" fill="rgba(124,108,240,0.10)"/>
    <path d="${line}" fill="none" stroke="#7c6cf0" stroke-width="2"/>
    ${hp ? `<line x1="${hp.x}" y1="${y0}" x2="${hp.x}" y2="${hp.y}" stroke="#7c6cf0" stroke-opacity="0.35" stroke-dasharray="3 3"/>` : ''}
    ${pts.map((p, i) => {
      const a = i === hi;
      const mark = p.edge
        ? `<rect x="${p.x - (a ? 5 : 4)}" y="${p.y - (a ? 5 : 4)}" width="${a ? 10 : 8}" height="${a ? 10 : 8}" transform="rotate(45 ${p.x} ${p.y})" fill="#7c6cf0"/>`
        : `<circle cx="${p.x}" cy="${p.y}" r="${a ? 5 : 3.5}" fill="${a ? '#7c6cf0' : '#fff'}" stroke="#7c6cf0" stroke-width="2"/>`;
      return `<g data-pt="${i}" style="cursor:pointer">${mark}<circle cx="${p.x}" cy="${p.y}" r="12" fill="transparent"/></g>`;
    }).join('')}
    ${hp ? `<g pointer-events="none"><rect x="${tipX}" y="${tipY}" width="${tipW}" height="22" rx="6" fill="#0f172a" opacity="0.92"/>
      <text x="${tipX + tipW / 2}" y="${tipY + 15}" text-anchor="middle" font-size="11" fill="#fff">${esc(tip)}</text></g>` : ''}
    <text x="${x0}" y="${H - 8}" font-size="10" fill="#94a3b8">${series.period_start}</text>
    <text x="${x1}" y="${H - 8}" text-anchor="end" font-size="10" fill="#94a3b8">${series.period_end}</text>
  </svg>`;
}

function renderOverview() {
  const teams = overviewTeams(), goals = allGoals(), krs = allKRs();
  const withGoals = teams.filter(t => t.goals_count > 0);
  const progressTeams = withGoals.filter(t => t.status !== 'forming');
  const byStatus = {};
  PO_STATUS_TILES.forEach(st => { byStatus[st.key] = teams.filter(t => t.status === st.key).length; });
  const errCount = teams.filter(t => t.weight_error).length;
  const avg = progressTeams.length ? Math.round(progressTeams.reduce((s, t) => s + t.progress, 0) / progressTeams.length) : 0;
  const affectActivate = withGoals.filter(t => t.status !== 'in_progress').length;
  const affectClose = withGoals.filter(t => t.status !== 'closed').length;
  const skipNoGoals = teams.length - withGoals.length;

  const d = state.drill;
  let tilesDrill = '', balanceDrill = '';
  if (d && (d.kind === 'status' || d.kind === 'err' || d.kind === 'goals')) {
    const dt = d.kind === 'status' ? teams.filter(t => t.status === d.key)
      : d.kind === 'err' ? teams.filter(t => t.weight_error) : withGoals;
    tilesDrill = poDrillPanel(d.title, dt.map(t => poDrillRow(esc(t.path.join(' › ')),
      t.goals_count > 0 ? `${t.progress}% · веса ${t.weight_sum}` : 'нет целей', t.weight_error)));
  }
  if (d && d.kind === 'balance') {
    balanceDrill = poDrillPanel(d.title, goals.filter(g => g[d.field] === d.key).map(g => poDrillRow(
      `${esc(g.title)} <span style="color:${PO.dimFg}">· ${esc(g.team_name)}</span>`, `${goalProgress(g)}%`)));
  }
  if (d && d.kind === 'krhealth') {
    balanceDrill = poDrillPanel(d.title, krs.filter(kr => kr.healthStatus === d.key).map(kr => poDrillRow(
      `${esc(kr.name)} <span style="color:${PO.dimFg}">· ${esc(kr.goal_title)} · ${esc(kr.team_name)}</span>`, `${krProgress(kr)}%`)));
  }

  return `<div style="padding:18px 22px">
    ${poEyebrow(`Команды по статусам · всего ${teams.length}`, '0 0 10px')}
    <div style="display:flex;gap:10px;flex-wrap:wrap">
      ${PO_STATUS_TILES.map(st => poTile(
        `<span style="display:inline-flex;align-items:center;gap:6px"><span style="width:7px;height:7px;border-radius:999px;background:${st.dot}"></span>${st.label}</span>`,
        byStatus[st.key], 'показать состав', st.color, `data-drill="status:${st.key}:${st.label}"`)).join('')}
    </div>

    ${poEyebrow('Качество и результат', '18px 0 10px')}
    <div style="display:flex;gap:10px;flex-wrap:wrap">
      ${poTile('Команды с целями', `${withGoals.length}/${teams.length}`, 'только они участвуют в массовых операциях', PO.accent, 'data-drill="goals::Команды с целями"')}
      ${poTile('Ошибки весов', errCount, 'сумма весов целей ≠ 100%', PO.danger, 'data-drill="err::Ошибки весов"')}
      ${poTile('Средний прогресс', `${avg}%`, `по ${progressTeams.length} командам с целями (без черновиков)`, PO.accent)}
    </div>
    ${tilesDrill}

    ${poEyebrow('Балансы целей · клик по полосе — состав', '18px 0 10px')}
    <div style="display:flex;gap:28px;flex-wrap:wrap">
      ${balanceBars('Discovery / Delivery', 'Соотношение исследовательской и поставочной работы',
        balanceItems(goals, 'type', ['delivery', 'discovery']), PO_DD_LABELS, PO_DD_COLORS, 'type')}
      ${balanceBars('Стратегические фокусы', 'Profitability · Stability · Speed Efficiency · Tech Independency',
        balanceItems(goals, 'focus', Object.keys(PO_FOCUS_LABELS)), PO_FOCUS_LABELS, PO_FOCUS_COLORS, 'focus')}
      ${balanceBars('Приоритеты', 'Распределение целей по приоритету P0–P3',
        balanceItems(goals, 'priority', Object.keys(PO_PRIO_LABELS)), PO_PRIO_LABELS, PO_PRIO_COLORS, 'priority')}
      ${balanceBars('Статусы KR', 'Распределение key results по health-статусу',
        balanceItems(krs, 'healthStatus', Object.keys(PO_HEALTH_LABELS)), PO_HEALTH_LABELS, PO_HEALTH_COLORS, 'healthStatus')}
    </div>
    ${balanceDrill}

    ${poEyebrow('Прогресс целей за период', '22px 0 4px')}
    <div style="font-size: 13px;color:${PO.mutedFg};margin-bottom:10px">Пунктирная диагональ — ориентир ровного заполнения периода. Ромбы по краям — прогресс, зафиксированный до начала или после окончания периода.</div>
    <div style="border:1px solid ${PO.cardBorder};border-radius:12px;padding:12px 14px" id="chart">${progressChartSVG()}</div>

    ${poEyebrow(`Управление периодом · ${state.scope === 'org' ? 'вся организация' : 'мои команды'}`, '22px 0 10px')}
    <div style="display:flex;flex-direction:column;gap:10px">
      ${[{ t: 'Перевести в «В работе»', d: 'Затронет только команды с хотя бы одной целью в этом периоде. Цели блокируются от редактирования, остаётся обновление прогресса.', n: affectActivate },
         { t: 'Закрыть цели периода', d: 'Команды без целей не трогаем. У остальных статус становится «Закрыто» — доступны только комментарии.', n: affectClose }]
        .map(op => `<div style="border:1px solid ${PO.cardBorder};border-radius:12px;padding:14px 16px;display:flex;align-items:center;gap:14px;flex-wrap:wrap">
        <div style="flex:1 1 260px">
          <div style="font-size: 14.5px;font-weight:700;color:${PO.headingFg}">${op.t}</div>
          <div style="font-size: 13px;color:${PO.mutedFg};margin-top:3px">${op.d}</div>
        </div>
        <div style="font-size: 12.5px;color:${PO.dimFg};text-align:right">затронет ${op.n}<br>пропустим ${skipNoGoals} без целей</div>
        <button style="display:inline-flex;align-items:center;justify-content:center;padding:8px 16px;border-radius:20px;border:1.5px solid ${PO.accent};background:${PO.accent};color:white;font-size: 14px;font-weight:600;cursor:${op.n ? 'pointer' : 'not-allowed'};opacity:${op.n ? 1 : .5};white-space:nowrap"${op.n ? '' : ' disabled'}>Применить</button>
      </div>`).join('')}
    </div>
  </div>`;
}

// ── Оверлеи: модалки и панель уведомлений ────────────────────────────────────
function findKR(goalId, krId) {
  const goal = goalsOf(state.teamId).find(g => g.id === goalId);
  return { goal, kr: goal && goal.krs.find(k => k.id === krId) };
}

function krModal(m) {
  const { goal, kr } = findKR(m.goalId, m.krId);
  const draft = m.draft;
  const p = krProgress({ ...kr, ...draft });
  const c = KR_HEALTH_COLOR[draft.healthStatus];
  let body = '';
  if (kr.krType === 'NUMERICAL') {
    const cps = (kr.checkpoints || []).filter(c => c.value !== '' && c.value != null).map(c => ({ v: Number(c.value), pct: Number(c.progress_percent) }));
    const s0 = Number(kr.start || 0), t0 = Number(kr.target ?? 100);
    const posOf = v => t0 === s0 ? 0 : Math.max(0, Math.min(100, (v - s0) / (t0 - s0) * 100));
    const curV = Number(draft.current);
    body = `<div class="kr-num-section kr-num-section--compact">
      <div class="kr-num-line">
        <span class="kr-num-line__lbl">Текущее значение</span>
        <label class="kr-num-input-suffix kr-num-input-suffix--compact">
          <input class="form-input form-input--sm" id="kr-current" type="number" step="0.01" value="${draft.current}">
          ${kr.unit ? `<span class="kr-num-input-suffix__unit">${esc(kr.unit)}</span>` : ''}
        </label>
        <span class="kr-num-line__range">из ${fmtVal(kr.target, kr.unit)} · старт ${fmtVal(kr.start, kr.unit)}</span>
        <span class="kr-pct kr-num-line__pct" style="color:${ACCENT}">${p}%</span>
      </div>
      <div class="kr-scale">
        <div class="kr-scale__track"><div class="kr-scale__fill" style="width:${posOf(curV)}%;background:${ACCENT}"></div></div>
        ${cps.map(c => `<span class="kr-scale__cp${curV >= c.v === t0 >= s0 ? ' kr-scale__cp--hit' : ''}" style="left:${posOf(c.v)}%" title="Промежуточное значение: ${fmtVal(c.v, kr.unit)} → ${c.pct}% прогресса"></span>`).join('')}
        <span class="kr-scale__cur" style="left:${posOf(curV)}%;border-color:${ACCENT}"></span>
      </div>
      <div class="kr-scale__labels">
        <span class="kr-scale__lbl" style="left:0">${fmtNum(s0)}</span>
        ${cps.map(c => `<span class="kr-scale__lbl kr-scale__lbl--cp" style="left:${posOf(c.v)}%">${fmtNum(c.v)}<b>${c.pct}%</b></span>`).join('')}
        <span class="kr-scale__lbl" style="left:100%">${fmtNum(t0)}</span>
      </div>
      ${cps.length ? `<div class="kr-scale__note">Промежуточные значения задают, какой прогресс даёт значение метрики — шкала нелинейная.</div>` : ''}
    </div>`;
  } else if (kr.krType === 'BOOLEAN') {
    body = `<label class="kr-boolean-label">
      <input type="checkbox" id="kr-done"${draft.done ? ' checked' : ''}>
      <span class="kr-boolean-text">Результат достигнут</span>
      <span class="kr-boolean-pct" style="color:${ACCENT}">${p}%</span>
    </label>`;
  } else {
    body = `<div class="kr-num-section">
      <div class="kr-num-section__title">Этапы</div>
      ${draft.stages.map((s, i) => `<label class="kr-stage-label"${s.done ? ' style="background:#f5f3ff"' : ''}>
        <input type="checkbox" data-stage="${i}"${s.done ? ' checked' : ''}>
        <span class="kr-stage-name">${esc(s.title)}</span>
        <span class="kr-stage-weight">${s.weight}%</span>
      </label>`).join('')}
      <div class="kr-progress-row" style="margin-top:10px"><span class="kr-progress-row__label">Прогресс</span>
        <span class="kr-pct" style="color:${ACCENT}">${p}%</span></div>
      ${progressBar(p, null, 6, ACCENT)}
    </div>`;
  }

  return `<div class="modal-overlay modal-overlay--z300">
    <div class="modal-box modal-box--w480" style="width:min(760px,94vw)">
      <div class="modal-header">
        <div>
          <div class="modal-title modal-title--lg">Обновить прогресс</div>
          <div class="modal-subtitle"><span class="modal-cap">Цель</span>${esc(goal.title)}</div>
        </div>
        <button class="modal-close" data-close="1">×</button>
      </div>
      <div class="modal-body">
        <span class="modal-cap">Ключевой результат</span>
        <div class="field-label">${esc(kr.name)}</div>
        ${kr.desc ? `<div class="kr-modal-desc md-content" data-desc-key="m${kr.id}">${mdRender(kr.desc)}</div>` : ''}
        ${body}
        <div class="kr-health-section">
          <div class="kr-health-section__label">Статус результата</div>
          <div class="kr-health-cards">
            ${KR_HEALTH_OPTIONS.map(s => {
              const on = draft.healthStatus === s, col = KR_HEALTH_COLOR[s];
              return `<button class="kr-health-card${on ? ' kr-health-card--active' : ''}" data-health="${s}"
                ${on ? `style="border-color:${col};background:${col}0f;color:${col}"` : ''}>
                <span class="kr-health-card__title" style="color:${col}">${KR_HEALTH_ICON[s]} ${KR_HEALTH_LABEL[s]}</span>
                <span class="kr-health-card__hint">${KR_HEALTH_HINT[s]}</span>
              </button>`;
            }).join('')}
          </div>
        </div>
        <div class="kr-progress-field" style="margin-top:14px">
          <div class="kr-progress-field__label">Комментарий <span class="kr-progress-field__hint">необязательно</span></div>
          ${markdownEditor(m.comment || '', 'Что изменилось с прошлой отметки', !!m.preview, 'kr-comment', 3)}
        </div>
      </div>
      <div class="modal-footer">
        <button class="btn btn--secondary" data-close="1">Отмена</button>
        <button class="btn btn--primary" id="kr-save">Сохранить</button>
      </div>
    </div>
  </div>`;
}

// ── Markdown-редактор ────────────────────────────────────────────────────────
// Панель, табы и подсказка повторяют MarkdownEditor из web/static/markdown.js,
// включая иконки. Отличие одно: предпросмотр рендерит markdown минимальным
// собственным преобразованием, а не marked + DOMPurify, чтобы прототип работал
// без внешних библиотек.
const MD_ICON_LIST = `<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
  <line x1="8" y1="6" x2="20" y2="6"></line><line x1="8" y1="12" x2="20" y2="12"></line><line x1="8" y1="18" x2="20" y2="18"></line>
  <line x1="3.5" y1="6" x2="3.51" y2="6"></line><line x1="3.5" y1="12" x2="3.51" y2="12"></line><line x1="3.5" y1="18" x2="3.51" y2="18"></line></svg>`;
const MD_ICON_LINK = `<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
  <path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"></path>
  <path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"></path></svg>`;

const MD_TOOLBAR = [
  { kind: 'bold', node: 'B', title: 'Жирный', style: 'font-weight:800' },
  { kind: 'italic', node: 'I', title: 'Курсив', style: 'font-style:italic;font-family:Georgia,"Times New Roman",serif' },
  { kind: 'heading', node: 'H', title: 'Заголовок', style: 'font-weight:700' },
  'divider',
  { kind: 'ul', node: MD_ICON_LIST, title: 'Маркированный список', style: '' },
  { kind: 'ol', node: '1.', title: 'Нумерованный список', style: 'font-weight:600;font-size: 13px' },
  'divider',
  { kind: 'quote', node: '❝', title: 'Цитата', style: 'font-size: 16px;line-height:1' },
  { kind: 'code', node: '&lt;/&gt;', title: 'Код', style: 'font-family:ui-monospace,Menlo,monospace;font-size: 12px;font-weight:600' },
  { kind: 'link', node: MD_ICON_LINK, title: 'Ссылка', style: '' },
];
const MD_HINT = ': **жирный**, *курсив*, # заголовок, - список, [ссылка](url), `код`';

// Повторяет applyMarkdownFormat из markdown.js.
function applyMarkdownFormat(el, kind, onChange) {
  const start = el.selectionStart, end = el.selectionEnd, value = el.value;
  const sel = value.slice(start, end);
  let inserted, selFrom, selTo;
  if (kind === 'bold' || kind === 'italic' || kind === 'code') {
    const mark = kind === 'bold' ? '**' : kind === 'italic' ? '*' : '`';
    const inner = sel || (kind === 'bold' ? 'жирный' : kind === 'italic' ? 'курсив' : 'код');
    inserted = mark + inner + mark;
    selFrom = start + mark.length; selTo = selFrom + inner.length;
  } else if (kind === 'link') {
    const label = sel || 'текст';
    inserted = '[' + label + '](url)';
    selFrom = start + inserted.length - 4; selTo = selFrom + 3;
  } else {
    const ph = kind === 'quote' ? 'цитата' : kind === 'heading' ? 'заголовок' : 'пункт';
    const lines = (sel || ph).split('\n');
    const prefix = i => kind === 'ul' ? '- ' : kind === 'ol' ? (i + 1) + '. ' : kind === 'quote' ? '> ' : '# ';
    inserted = lines.map((l, i) => prefix(i) + l).join('\n');
    selFrom = start; selTo = start + inserted.length;
  }
  const next = value.slice(0, start) + inserted + value.slice(end);
  onChange(next);
  requestAnimationFrame(() => { el.focus(); el.setSelectionRange(selFrom, selTo); });
}

// Минимальный рендер markdown для предпросмотра.
function mdRender(text) {
  const lines = esc(text).split('\n');
  const out = [];
  let list = null;
  const inline = s => s
    .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
    .replace(/(^|[^*])\*([^*]+)\*/g, '$1<em>$2</em>')
    .replace(/`([^`]+)`/g, '<code>$1</code>')
    .replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2">$1</a>');
  const closeList = () => { if (list) { out.push(`</${list}>`); list = null; } };
  for (const raw of lines) {
    const l = raw.trim();
    let m;
    if ((m = /^(#{1,3})\s+(.*)$/.exec(l))) { closeList(); out.push(`<h${m[1].length}>${inline(m[2])}</h${m[1].length}>`); }
    else if ((m = /^[-*]\s+(.*)$/.exec(l))) { if (list !== 'ul') { closeList(); out.push('<ul>'); list = 'ul'; } out.push(`<li>${inline(m[1])}</li>`); }
    else if ((m = /^\d+\.\s+(.*)$/.exec(l))) { if (list !== 'ol') { closeList(); out.push('<ol>'); list = 'ol'; } out.push(`<li>${inline(m[1])}</li>`); }
    else if ((m = /^(?:>|&gt;)\s+(.*)$/.exec(l))) { closeList(); out.push(`<blockquote>${inline(m[1])}</blockquote>`); }
    else if (l) { closeList(); out.push(`<p>${inline(l)}</p>`); }
    else closeList();
  }
  closeList();
  return out.join('');
}

function markdownEditor(value, placeholder, preview, id = 'goal-desc', rows = 3) {
  return `<div class="md-editor">
    <div class="md-editor__bar">
      <div class="md-editor__tools">
        ${MD_TOOLBAR.map((b, i) => b === 'divider'
          ? '<span class="md-editor__divider"></span>'
          : `<button type="button" class="md-editor__btn" title="${b.title}" style="${b.style}" data-md="${b.kind}"${preview ? ' disabled' : ''}>${b.node}</button>`).join('')}
      </div>
      <div class="md-editor__tabs">
        <button type="button" class="md-editor__tab${!preview ? ' md-editor__tab--active' : ''}" data-md-tab="write">Написать</button>
        <button type="button" class="md-editor__tab${preview ? ' md-editor__tab--active' : ''}" data-md-tab="preview">Просмотр</button>
      </div>
    </div>
    ${preview
      ? `<div class="md-editor__preview">${value.trim() ? `<div class="md-content">${mdRender(value)}</div>` : '<div class="md-editor__empty">Нечего показать</div>'}</div>`
      : `<textarea id="${id}" rows="${rows}" placeholder="${placeholder}" class="form-textarea">${esc(value)}</textarea>`}
    ${preview ? '' : `<div class="md-editor__hint">Поддерживается <strong>Markdown</strong>${esc(MD_HINT)}</div>`}
  </div>`;
}

// Подпись поля с подсказкой — FieldLabel + InfoHint. В продукте подсказка
// раскрывается по клику во всплывающий тултип; здесь это нативный title.
const fieldLabel = (text, required, hint, hintHtml) =>
  `<div class="field-label">${text}${required ? '<span class="field-label__required">*</span>' : ''}${hint ? `<span class="info-hint" tabindex="0"${hintHtml ? ` data-hint-html="${esc(hintHtml)}"` : ''} data-hint="${esc(hint)}" aria-label="${esc(hint)}">?</span>` : ''}</div>`;

// Подсказка фокуса: по строке на тему, название — своим цветом.
const FOCUS_HINT = {
  PROFITABILITY: 'выручка и маржинальность',
  STABILITY: 'надёжность и меньше инцидентов',
  SPEED_EFFICIENCY: 'скорость поставки и эффективность процессов',
  TECH_INDEPENDENCE: 'уход от внешних и legacy-зависимостей',
};
const FOCUS_KEYS = Object.keys(FOCUS_HINT);
const FOCUS_HINT_TEXT = 'На какую стратегическую тему работает цель.\n'
  + FOCUS_KEYS.map(f => `${FOCUS_LABEL[f] || f} — ${FOCUS_HINT[f]}`).join('\n');
const FOCUS_HINT_HTML = 'На какую стратегическую тему работает цель.'
  + FOCUS_KEYS.map(f =>
      `<span class="hint-row"><span class="hint-code" style="color:${FOCUS_COLORS[f] || FOCUS_COLORS.DEFAULT}">${esc(FOCUS_LABEL[f] || f)}</span> — ${esc(FOCUS_HINT[f])}</span>`).join('');

// Подсказка приоритета: по строке на уровень, код уровня — своим цветом.
const PRIORITY_HINT_TEXT = 'Насколько цель важна относительно других.\n'
  + ['P0', 'P1', 'P2', 'P3'].map(p => `${p} — ${PRI_HINT[p]}`).join('\n');
const PRIORITY_HINT_HTML = 'Насколько цель важна относительно других.'
  + ['P0', 'P1', 'P2', 'P3'].map(p =>
      `<span class="hint-row"><span class="hint-code" style="color:${PRI_COLOR[p]}">${p}</span> — ${esc(PRI_HINT[p])}</span>`).join('');

// ── Редактор Key Result ──────────────────────────────────────────────────────
// Повторяет KREditModal: название, markdown-описание, вес и тип, затем блок,
// зависящий от типа (числовой прогресс с промежуточными значениями, флажок
// «Выполнено» или шаги проекта), и критерий обнуления.
function krEditModal(m) {
  const f = m.form;
  const prev = calcKRProgress(f);
  const sw = f.stages.reduce((s, st) => s + Number(st.weight || 0), 0);
  const canSave = !!f.name.trim();

  let typeBlock = '';
  if (f.krType === 'NUMERICAL') {
    typeBlock = `<div class="kr-num-section">
      <div class="kr-num-section__title">Метрика</div>
      <div class="kre-metric-row">
        <div class="kre-metric-row__unit">
          <div class="kr-num-field__label">Единица</div>
          <select id="kre-unit" class="form-select form-select--sm">
            ${KR_UNITS.map(u => `<option${u === f.unit ? ' selected' : ''}>${esc(u)}</option>`).join('')}
          </select>
        </div>
        <div class="kre-metric-row__val">
          <div class="kr-num-field__label">Старт (было)</div>
          <div class="kr-num-input-suffix">
            <input id="kre-start" type="number" step="any" value="${f.start}" class="form-input form-input--sm">
            <span class="kr-num-input-suffix__unit">${esc(f.unit)}</span>
          </div>
        </div>
        <span class="kre-metric-row__arrow" aria-hidden="true">→</span>
        <div class="kre-metric-row__val">
          <div class="kr-num-field__label">Цель (станет)</div>
          <div class="kr-num-input-suffix">
            <input id="kre-target" type="number" step="any" value="${f.target}" class="form-input form-input--sm">
            <span class="kr-num-input-suffix__unit">${esc(f.unit)}</span>
          </div>
        </div>
      </div>
      ${m.krId ? '' : '<div class="kre-metric-note">Текущее значение на старте равно стартовому — дальше меняется при обновлении прогресса.</div>'}
      <div class="kr-checkpoints" style="margin-top:12px">
        <div class="kr-section-head">
          <span class="kr-section-head__title">Промежуточные значения</span>
          <span class="kr-section-head__opt">опционально</span>
          <span class="info-hint" title="Промежуточное значение задаёт, какой процент достижения KR даёт конкретное значение метрики. Прогресс интерполируется линейно между стартом (0%), промежуточными значениями и целью (100%).">?</span>
        </div>
        ${f.checkpoints.length ? `<div class="kr-cp-head">
          <span class="kr-cp-head__label">Значение (${esc(f.unit)})</span>
          <span class="kr-cp-head__label">Прогресс, %</span>
          <span></span>
        </div>` : ''}
        ${f.checkpoints.map((c, i) => `<div class="kr-cp-row">
          <input type="number" step="any" placeholder="напр. 150" value="${c.value}" data-cp-value="${i}" class="form-input form-input--sm">
          <input type="number" placeholder="0–100" min="0" max="100" value="${c.progress_percent}" data-cp-pct="${i}" class="form-input form-input--sm">
          <button class="kr-step-delete" data-cp-del="${i}">×</button>
        </div>`).join('')}
        <button type="button" class="kr-dashed-btn" data-cp-add="1">+ Добавить промежуточное значение</button>
      </div>
    </div>`;
  } else if (f.krType === 'BOOLEAN') {
    typeBlock = `<div class="kr-num-section">
      <div class="kr-num-section__title">True / False</div>
      <div class="kre-metric-note" style="margin-top:0">Прогресс 0% до достижения результата, 100% — после. Отмечается при обновлении прогресса.</div>
    </div>`;
  } else {
    typeBlock = `<div class="kr-num-section">
      <div class="kr-steps-header">
        <div class="kr-steps-title">Шаги проекта</div>
        <div class="kr-steps-sum ${Math.abs(sw - 100) < 1 ? 'kr-steps-sum--ok' : 'kr-steps-sum--bad'}">Сумма: ${sw}</div>
      </div>
      ${f.stages.length ? `<div class="kr-steps-cols">
        <span class="kr-steps-cols__name">Название шага</span>
        <span class="kr-steps-cols__weight">Вес, %</span>
        <span class="kr-steps-cols__del"></span>
      </div>` : ''}
      ${f.stages.map((st, i) => `<div class="kr-step-row">
        <input value="${esc(st.title || '')}" placeholder="Название шага" data-st-name="${i}" class="form-input form-input--sm" style="flex:1">
        <input type="number" min="0" value="${st.weight}" data-st-weight="${i}" class="form-input form-input--sm form-input--center" style="width:60px">
        <button class="kr-step-delete" data-st-del="${i}">×</button>
      </div>`).join('')}
      <button class="kr-step-add" data-st-add="1">+ Добавить шаг</button>
    </div>`;
  }

  return `<div class="modal-overlay modal-overlay--z300">
    <div class="modal-box modal-box--w560">
      <div class="modal-header modal-header--sticky">
        <div class="modal-title modal-title--lg">${m.krId ? 'Редактировать KR' : 'Добавить KR'}</div>
        <button class="modal-close" data-close="1">×</button>
      </div>
      <div class="modal-body">
        <div class="kre-name-row">
          <div class="kre-name-row__name">
            <div class="kr-num-field__label">Название</div>
            <input id="kre-name" value="${esc(f.name)}" placeholder="Что измеряет этот KR?" class="form-input">
          </div>
          <div class="kre-name-row__weight">
            <div class="kr-num-field__label">Вес<span class="has-tip kre-tip" tabindex="0" data-tip-title="Вес KR" data-tip="Доля KR в прогрессе цели. Сумма весов всех KR цели должна быть 100%. Больше вес — сильнее KR влияет на итог цели.">?</span></div>
            <label class="kr-num-input-suffix">
              <input id="kre-weight" type="number" min="0" max="100" value="${f.weight}" class="form-input form-input--center">
              <span class="kr-num-input-suffix__unit">%</span>
            </label>
          </div>
        </div>
        <div class="form-group--sm">
          <div class="kr-num-field__label">Описание</div>
          ${markdownEditor(f.desc, '', m.preview, 'kre-desc', 2)}
        </div>
        <div class="kr-measure-pick">
          <div class="kr-num-field__label">Как измеряем прогресс</div>
          <div class="kr-measure-pick__hint">Определяет, как считается процент выполнения KR</div>
          <div class="kr-measure-pick__opts" role="radiogroup">
            ${KR_TYPE_OPTIONS.map(t => `<button type="button" role="radio" aria-checked="${f.krType === t}" class="kr-measure-opt${f.krType === t ? ' kr-measure-opt--on' : ''}" data-kre-type="${t}">
              <span class="kr-measure-opt__head"><span class="kr-measure-opt__ic">${KR_MEASURE_INFO[t].icon}</span>${KR_MEASURE_INFO[t].title}</span>
              <span class="kr-measure-opt__desc">${KR_MEASURE_INFO[t].desc}</span>
              <span class="kr-measure-opt__ex">${KR_MEASURE_INFO[t].ex}</span>
            </button>`).join('')}
          </div>
        </div>
        ${typeBlock}
        <div class="kr-section-sep"></div>
        ${m.showZeroing
          ? `<div class="kr-num-field">
              <div class="kr-section-head"><span class="kr-section-head__title">Критерий обнуления</span></div>
              <textarea id="kre-zeroing" rows="2" class="form-textarea form-textarea--sm" style="resize:vertical">${esc(f.zeroing || '')}</textarea>
            </div>`
          : `<button type="button" class="kr-zeroing-btn" data-zeroing="1"><span class="kr-zeroing-btn__icon">⊘</span> Критерий обнуления</button>`}
      </div>
      <div class="modal-footer modal-footer--sticky">
        <button class="btn btn--secondary" data-close="1">Отмена</button>
        <button class="btn btn--primary"${canSave ? ' data-kre-save="1"' : ' disabled'}
          style="background:${canSave ? ACCENT : '#e5e7eb'};color:${canSave ? 'white' : '#9ca3af'};cursor:${canSave ? 'pointer' : 'default'}">Сохранить</button>
      </div>
    </div>
  </div>`;
}

// ── Связанные цели: лейбл ↑N / ↓N со всплывающим списком ─────────────────────
// Повторяет GoalLinksPopover: поповер рисуется порталом в body (position: fixed),
// появляется по наведению и закрывается с задержкой 150 мс, чтобы курсор успел
// перейти на сам список.
function goalLinksLabel(dir, goal) {
  const items = dir === 'up' ? (goal.parents || []) : (goal.children || []);
  if (!items.length) return '';
  return `<span class="goal-link-label goal-link-label--${dir}" data-links="${dir}:${goal.id}">${dir === 'up' ? '↑' : '↓'} ${items.length}</span>`;
}

let linksTimer = null;

function showLinksPopup(el) {
  clearTimeout(linksTimer);
  const [dir, goalId] = el.dataset.links.split(':');
  const goal = goalsOf(state.teamId).find(g => g.id === +goalId);
  if (!goal) return;
  const items = dir === 'up' ? goal.parents : goal.children;
  const r = el.getBoundingClientRect();
  const left = Math.max(8, Math.min(r.left, window.innerWidth - 328));
  const title = dir === 'up' ? `↑ ВКЛАД В · ${items.length}` : `↓ ${items.length}`;
  document.getElementById('popovers').innerHTML = `
    <div class="goal-links-popup" id="links-popup" style="top:${r.bottom + 6}px;left:${left}px">
      <div class="goal-links-popup__head">${title}</div>
      ${items.map(it => `<button type="button" class="goal-links-popup__row">
        <span class="goal-links-popup__row-title">${esc(it.title)}</span>
        <span class="goal-links-popup__row-meta">${esc(it.periodName)} · ${esc(it.teamName)}</span>
        <span class="goal-links-popup__row-progress">${(() => { if (it.progress != null) return it.progress; for (const bd of Object.values(BOARDS)) { const g = (bd.goals || []).find(x => x.id === it.id); if (g) return goalProgress(g); } return 0; })()}%</span>
      </button>`).join('')}
    </div>`;
}

function hideLinksPopup() {
  clearTimeout(linksTimer);
  linksTimer = setTimeout(() => { document.getElementById('popovers').innerHTML = ''; }, 150);
}

document.addEventListener('mouseover', e => {
  const label = e.target.closest && e.target.closest('[data-links]');
  if (label) { showLinksPopup(label); return; }
  if (e.target.closest && e.target.closest('#links-popup')) clearTimeout(linksTimer);
});

document.addEventListener('mouseout', e => {
  const menu = e.target.closest && e.target.closest('[data-tenant-menu]');
  if (menu && state.tenantMenu && !menu.contains(e.relatedTarget)) { state.tenantMenu = false; renderSidebar(); return; }
  const umenu = e.target.closest && e.target.closest('[data-user-menu-box]');
  if (umenu && state.userMenu && !umenu.contains(e.relatedTarget)) { state.userMenu = false; renderSidebar(); return; }
  if (e.target.closest && e.target.closest('[data-links], #links-popup')) hideLinksPopup();
});

// ── Меню «···» на карточке цели ──────────────────────────────────────────────
// Повторяет ExportMenu: два пункта с подписями, закрывается кликом вне.
function exportMenu(goalId, teamId) {
  const open = state.goalMenu === goalId;
  const structLock = lockFor(editModeOf(teamId), 'structure');
  return `<div class="export-menu" data-menu-wrap="${goalId}">
    <button type="button" class="export-menu__btn" title="Ещё" aria-label="Ещё" data-menu="${goalId}">···</button>
    ${open ? `<div class="export-menu__dropdown">
      ${menuItem('✎', 'Редактировать', `data-modal="goal-edit" data-goal="${goalId}"`, { disabled: !canEditText(teamId), reason: editLockReason(teamId, 'цель') })}
      ${menuItem('🔗', 'Копировать ссылку', '')}
      ${menuItem('➡', 'Перенести или скопировать', `data-open-transfer="${goalId}"`)}
      <div class="act-menu__sep"></div>
      ${menuItem('×', 'Удалить', '', { danger: true, disabled: !!structLock, reason: deleteLockReason(teamId, 'цель') })}
    </div>` : ''}
  </div>`;
}

// ── Экспорт в Markdown ───────────────────────────────────────────────────────
// Разметка повторяет ExportModal. Сам текст экспорта в продукте формирует сервер
// (internal/render/export), поэтому здесь он собирается локально и его формат —
// приблизительный: важна структура окна, а не точный текст.
function exportMarkdown(m) {
  const team = findTeam(state.teamId);
  const goals = m.scope === 'goal'
    ? [goalsOf(state.teamId).find(g => g.id === m.goalId)]
    : m.scope === 'team' ? goalsOf(state.teamId)
    : flatTeamsDepth().flatMap(t => goalsOf(t.id));
  const out = [`# ${team.name} · ${goalsPeriod().name}`, ''];
  goals.filter(Boolean).forEach(g => {
    out.push(`## ${g.title}`);
    out.push(`**${g.priority}** · вес ${g.weight}% · прогресс ${goalProgress(g)}%`);
    if (m.full && g.desc) out.push('', g.desc);
    out.push('');
    g.krs.forEach(kr => {
      out.push(`- [${krProgress(kr)}%] ${kr.name} — ${krDetailText(kr)}`);
      if (m.full && kr.desc) out.push(`  ${kr.desc}`);
    });
    if (m.comments && g.comments.length) {
      out.push('', '### Комментарии');
      g.comments.forEach(c => out.push(`- **${c.author}** (${c.date}): ${c.text}`));
    }
    out.push('');
  });
  return out.join('\n');
}

function krDetailText(kr) {
  if (kr.krType === 'BOOLEAN') return kr.done ? 'выполнено' : 'не выполнено';
  if (kr.krType === 'PROJECT') return `${(kr.stages || []).filter(s => s.done).length}/${(kr.stages || []).length} шагов`;
  return `${fmtVal(kr.current, kr.unit)} / ${fmtVal(kr.target, kr.unit)}`;
}

const pluralRu = (n, forms) => {
  const m10 = n % 10, m100 = n % 100;
  if (m10 === 1 && m100 !== 11) return forms[0];
  if (m10 >= 2 && m10 <= 4 && (m100 < 10 || m100 >= 20)) return forms[1];
  return forms[2];
};

function exportModal(m) {
  const goal = goalsOf(state.teamId).find(g => g.id === m.goalId);
  const team = findTeam(state.teamId);
  const teamGoals = goalsOf(state.teamId).length;
  const subtreeTeams = flatTeamsDepth().length;
  const md = exportMarkdown(m);
  const lines = md.split('\n').length;
  const filename = `okr-${team.name.toLowerCase().replace(/\s+/g, '-')}-${goalsPeriod().name}.md`;
  const cards = [
    ...(goal ? [{ key: 'goal', title: 'Одна цель', sub: goal.title }] : []),
    { key: 'team', title: 'Цели команды', sub: `${teamGoals} ${pluralRu(teamGoals, ['цель', 'цели', 'целей'])}` },
    { key: 'tree', title: 'С вложенными командами', sub: `${subtreeTeams} ${pluralRu(subtreeTeams, ['команда', 'команды', 'команд'])} в структуре` },
  ];
  return `<div class="modal-overlay modal-overlay--z600">
    <div class="modal-box modal-box--w720 export-modal">
      <div class="export-modal__header">
        <div class="modal-title">Экспорт целей в Markdown</div>
        <div class="modal-subtitle">${esc(goalsPeriod().name)} · ${esc((teamPath(state.teamId) || []).join(' / '))}</div>
      </div>
      <div class="export-modal__scopes">
        ${cards.map(c => `<button type="button" class="export-modal__scope-card${m.scope === c.key ? ' export-modal__scope-card--active' : ''}" data-scope-set="${c.key}">
          <div class="export-modal__scope-title">${esc(c.title)}</div>
          <div class="export-modal__scope-sub">${esc(c.sub)}</div>
        </button>`).join('')}
      </div>
      <div class="export-modal__options">
        <label class="export-modal__check"><input type="checkbox" data-exp-full="1"${m.full ? ' checked' : ''}> Полный экспорт</label>
        <label class="export-modal__check"><input type="checkbox" data-exp-comments="1"${m.comments ? ' checked' : ''}> Комментарии</label>
      </div>
      <div class="export-modal__preview-wrap">
        <pre class="export-modal__preview">${esc(md)}</pre>
      </div>
      <div class="export-modal__footer">
        <div class="export-modal__filename">${m.downloadBlocked
          ? 'Скачивание недоступно в опубликованной версии — нажмите «Скопировать» или откройте локальную копию'
          : `${esc(filename)} · ${lines} ${pluralRu(lines, ['строка', 'строки', 'строк'])}`}</div>
        <div class="export-modal__actions">
          <button type="button" class="btn btn--secondary" data-close="1">Закрыть</button>
          <button type="button" class="btn btn--secondary" data-exp-copy="1">${m.copied ? '✓ Скопировано' : 'Скопировать'}</button>
          <button type="button" class="btn btn--primary" data-exp-download="1">Скачать .md</button>
        </div>
      </div>
    </div>
  </div>`;
}

// ── Перенос или копирование ──────────────────────────────────────────────────
// Повторяет TransferGoalModal: режим, целевая команда (TeamCombobox в single),
// целевой период, флажки переноса и объяснение блокировок.
function transferTeamOptions(m) {
  const ql = (m.teamQ || '').trim().toLowerCase();
  return flatTeamsDepth().filter(t => !ql || t.name.toLowerCase().includes(ql) || (t.lead || '').toLowerCase().includes(ql));
}

function transferTeamDropdownHTML(m) {
  const list = transferTeamOptions(m);
  if (!list.length) return `<div class="team-combobox__empty">${m.teamQ ? 'Не найдено' : 'Нет команд'}</div>`;
  return list.map(t => {
    const color = TEAM_TYPE_COLOR[t.type] || '#6b7280';
    const status = boardOf(t.id).status;
    const blocked = status === 'in_progress' || status === 'closed';
    const reason = status === 'in_progress' ? 'в работе' : status === 'closed' ? 'закрыто' : '';
    const isSel = m.targetTeam === t.id;
    return `<div class="team-combobox__option${blocked ? ' team-combobox__option--blocked' : ''}${isSel ? ' team-combobox__option--selected' : ''}"
        style="padding:7px 12px 7px ${8 + t.depth * 14}px" data-transfer-team="${t.id}"${blocked ? ` title="${reason}"` : ''}>
      <div class="team-combobox__option-stripe" style="background:${color}"></div>
      <span class="team-combobox__option-type" style="color:${color};background:${color}12">${esc(TEAM_TYPE_LABEL[t.type] || t.type)}</span>
      <span class="team-combobox__option-name">${esc(t.name)}</span>
      ${t.lead ? `<span class="team-combobox__option-lead">${esc(t.lead)}</span>` : ''}
      ${blocked ? `<span class="team-combobox__option-blocked-tag">${reason}</span>` : ''}
    </div>`;
  }).join('');
}

function transferModal(m) {
  const goal = goalsOf(state.teamId).find(g => g.id === m.goalId);
  const target = findTeam(m.targetTeam);
  const targetStatus = boardOf(m.targetTeam).status;
  const targetBlocked = ['in_progress', 'closed'].includes(targetStatus);
  const sameAsSource = m.mode === 'move' && m.targetTeam === state.teamId && m.targetPeriod === state.periodId;
  const canSubmit = m.targetTeam && m.targetPeriod && !sameAsSource && !targetBlocked;
  const singleLabel = target ? `${TEAM_TYPE_LABEL[target.type] || target.type} · ${target.name}` : 'Найдите команду';
  const per = PERIODS.find(p => p.id === m.targetPeriod) || PERIODS[0];
  return `<div class="modal-overlay modal-overlay--z400">
    <div class="modal-box transfer-modal">
      <div class="modal-header">
        <div>
          <div class="transfer-modal__title">Перенести или скопировать цель</div>
          <div class="transfer-modal__subtitle">${esc(goal.title)}</div>
        </div>
        <button class="modal-close" data-close="1">×</button>
      </div>
      <div class="modal-body">
        <div class="seg-group transfer-modal__mode">
          <button type="button" class="seg-btn${m.mode === 'copy' ? ' seg-btn--active' : ''}" data-tr-mode="copy">⧉ Копировать</button>
          <button type="button" class="seg-btn${m.mode === 'move' ? ' seg-btn--active' : ''}" data-tr-mode="move">➡ Перенести</button>
        </div>

        <div class="transfer-modal__field">
          <div class="transfer-modal__label">Куда — команда</div>
          <div class="team-combobox">
            <div class="team-combobox__input-area${m.teamOpen ? ' team-combobox__input-area--open' : ''}" data-transfer-field="1">
              <input id="tr-team-q" class="team-combobox__input" value="${esc(m.teamQ || '')}" placeholder="${esc(singleLabel)}">
            </div>
            ${m.teamOpen ? `<div class="team-combobox__dropdown" id="tr-team-dd">${transferTeamDropdownHTML(m)}</div>` : ''}
          </div>
        </div>

        <div class="transfer-modal__field">
          <div class="transfer-modal__label">Куда — период</div>
          <div class="period-select period-select--light" style="width:100%">
            <button class="period-select__trigger" data-tr-period="1">
              <span class="period-select__dot" style="background:${per.color}"></span>
              <span class="period-select__name">${esc(per.name)}</span>
              <span class="period-select__chev">▾</span>
            </button>
            ${m.periodMenu ? `<div class="period-select__menu" style="left:${(m.periodRect || {}).left || 0}px;top:${(m.periodRect || {}).top || 0}px;width:360px">
              <div class="period-select__group">Периоды</div>
              ${PERIODS.map(p => `<button class="period-select__item${p.id === m.targetPeriod ? ' is-selected' : ''}" data-tr-period-set="${p.id}">
                <span class="period-select__indent" style="width:${(p.depth || 0) * 16}px"></span>
                <span class="period-select__item-name">${esc(p.name)}</span>
                <span class="period-select__range">${esc(p.range)}</span>
                <span class="period-select__badge-wrap">${p.state ? `<span class="period-select__badge" style="color:${p.color};background:${p.color}22">${esc(p.state)}</span>` : ''}</span>
              </button>`).join('')}
            </div>` : ''}
          </div>
        </div>

        <label class="transfer-modal__check"><input type="checkbox" data-tr-comments="1"${m.withComments ? ' checked' : ''}><span>Перенести комментарии</span></label>
        <label class="transfer-modal__check"><input type="checkbox" data-tr-progress="1"${m.withProgress ? ' checked' : ''}><span>Перенести прогресс и заметки KR</span></label>

        ${sameAsSource ? '<div class="transfer-modal__error">Перенос в ту же команду и период невозможен.</div>' : ''}
        ${targetBlocked && !sameAsSource ? '<div class="transfer-modal__error">Цели выбранной команды в этом периоде уже в работе или закрыты.</div>' : ''}
      </div>
      <div class="modal-footer">
        <button type="button" class="btn btn--secondary" data-close="1">Отмена</button>
        <button type="button" class="btn btn--primary"${canSubmit ? ' data-tr-submit="1"' : ' disabled'}>${m.mode === 'move' ? 'Перенести' : 'Скопировать'}</button>
      </div>
    </div>
  </div>`;
}

// ── Выбор людей и команд ─────────────────────────────────────────────────────
// UserSelector и TeamCombobox повторяют одноимённые компоненты из tracker.js:
// поле с чипами, выпадающий список с подсветкой, поиск, блокировка команд с
// уже начатым периодом.
function flatTeamsDepth(nodes = TREE, depth = 0, out = [], parentId = null) {
  for (const n of nodes) { out.push({ ...n, depth, parentId }); flatTeamsDepth(n.children, depth + 1, out, n.id); }
  return out;
}

function userOptions(f, q) {
  const ql = (q || '').toLowerCase();
  const taken = f.owners.map(u => u.udid);
  return USERS.filter(u => !taken.includes(u.udid) && (!ql || u.name.toLowerCase().includes(ql)));
}

function userDropdownHTML(f, q, hi) {
  const list = userOptions(f, q);
  if (!list.length) return `<div class="user-selector__empty">${q ? 'Пользователи не найдены' : 'Список пуст'}</div>`;
  return list.slice(0, 20).map((u, i) => `<div class="user-selector__option${i === hi ? ' user-selector__option--hi' : ''}" data-user-pick="${u.udid}">
    ${avatar(u.initials, 26)}
    <div class="user-selector__option-info">
      <span class="user-selector__option-name">${esc(u.name)}</span>
      ${u.led_team ? `<span class="user-selector__option-team">${esc(u.led_team)}</span>` : ''}
    </div>
  </div>`).join('');
}

function userSelector(m) {
  const f = m.form;
  return `<div class="user-selector">
    <div class="user-selector__field${m.userOpen ? ' user-selector__field--open' : ''}" data-user-field="1">
      ${f.owners.map(u => `<span class="user-chip">
        ${avatar(u.initials, 18)}
        <span class="user-chip__name">${esc(u.name)}</span>
        <button type="button" class="user-chip__remove" data-owner-del="${u.udid}">×</button>
      </span>`).join('')}
      <input id="user-q" class="user-selector__input" value="${esc(m.userQ || '')}"
        placeholder="${f.owners.length ? 'Ещё…' : 'Добавить драйвера цели'}">
    </div>
    ${m.userOpen ? `<div class="user-selector__dropdown" id="user-dd">${userDropdownHTML(f, m.userQ, m.userHi || 0)}</div>` : ''}
  </div>`;
}

// Команда заблокирована для шаринга, если её период уже «в работе» или «закрыт».
const teamShareBlocked = id => ['in_progress', 'closed'].includes(boardOf(id).status);

function teamOptions(f, q) {
  const ql = (q || '').trim().toLowerCase();
  return flatTeamsDepth()
    .filter(t => t.id !== state.teamId)
    .filter(t => !ql || t.name.toLowerCase().includes(ql) || (t.lead || '').toLowerCase().includes(ql))
    .filter(t => !f.shareTeamIds.includes(t.id));
}

function teamDropdownHTML(f, q, hi) {
  const list = teamOptions(f, q);
  if (!list.length) return `<div class="team-combobox__empty">${q ? 'Не найдено' : 'Нет команд'}</div>`;
  return list.map((t, i) => {
    const color = TEAM_TYPE_COLOR[t.type] || '#6b7280';
    const blocked = teamShareBlocked(t.id);
    return `<div class="team-combobox__option${i === hi ? ' team-combobox__option--hi' : ''}${blocked ? ' team-combobox__option--blocked' : ''}"
        style="padding:7px 12px 7px ${8 + t.depth * 14}px" data-team-pick="${t.id}"
        ${blocked ? 'title="У команды уже начат период — сначала верните цели в черновик"' : ''}>
      <div class="team-combobox__option-stripe" style="background:${color}"></div>
      <span class="team-combobox__option-type" style="color:${color};background:${color}12">${esc(TEAM_TYPE_LABEL[t.type] || t.type)}</span>
      <span class="team-combobox__option-name">${esc(t.name)}</span>
      ${t.lead ? `<span class="team-combobox__option-lead" title="Руководитель: ${esc(t.lead)}">${esc(t.lead)}</span>` : ''}
      ${blocked ? '<span class="team-combobox__option-locked">🔒</span>' : ''}
    </div>`;
  }).join('');
}

function teamCombobox(m) {
  const f = m.form;
  return `<div class="team-combobox">
    <div class="team-combobox__input-area${m.teamOpen ? ' team-combobox__input-area--open' : ''}" data-team-field="1">
      ${f.shareTeamIds.map(id => {
        const t = findTeam(id), color = TEAM_TYPE_COLOR[t.type] || '#6b7280';
        return `<div class="team-combobox__tag" style="background:${color}15;border:1px solid ${color}40">
          <span class="team-combobox__tag-type" style="color:${color}">${esc(TEAM_TYPE_LABEL[t.type] || t.type)}</span>
          <span class="team-combobox__tag-name">${esc(t.name)}</span>
          <button type="button" class="team-combobox__tag-remove" data-share-del="${id}">×</button>
        </div>`;
      }).join('')}
      <input id="team-q" class="team-combobox__input" value="${esc(m.teamQ || '')}"
        placeholder="${f.shareTeamIds.length ? 'Ещё…' : 'Найдите команду'}">
    </div>
    ${m.teamOpen ? `<div class="team-combobox__dropdown" id="team-dd">${teamDropdownHTML(f, m.teamQ, m.teamHi || 0)}</div>` : ''}
  </div>`;
}

// ── Выбор родительской цели ──────────────────────────────────────────────────
// Повторяет GoalParentPicker: селектор периода со «Все периоды», поиск и список,
// сгруппированный по командам в порядке оргиерархии с отступом по глубине.
const GOALS_PERIOD_ID = 3;
const goalsPeriod = () => PERIODS.find(p => p.id === GOALS_PERIOD_ID) || PERIODS[0];

function pickerGroups(m) {
  const ql = (m.pickerQ || '').trim().toLowerCase();
  const groups = [];
  flatTeamsDepth().forEach(t => {
    const rows = goalsOf(t.id)
      .filter(g => g.id !== m.goalId)
      .filter(g => m.pickerPeriod === 'all' || m.pickerPeriod === GOALS_PERIOD_ID)
      .filter(g => !ql || g.title.toLowerCase().includes(ql) || t.name.toLowerCase().includes(ql));
    if (rows.length) groups.push({ team: t, rows });
  });
  return groups;
}

function goalParentPicker(m) {
  const groups = pickerGroups(m);
  const selected = new Set(m.form.parents.map(p => p.id));
  const periodName = m.pickerPeriod === 'all' ? 'Все периоды' : period().name;
  return `<div class="modal-overlay modal-overlay--z600" data-picker-overlay="1">
    <div class="modal-box goal-parent-picker">
      <div class="modal-header">
        <div class="goal-parent-picker__title">Выбрать родительскую цель</div>
        <button class="modal-close" data-picker-close="1">×</button>
      </div>
      <div class="goal-parent-picker__filters">
        <div class="period-select period-select--light">
          <button class="period-select__trigger" data-picker-period="1">
            <span class="period-select__dot" style="background:${m.pickerPeriod === 'all' ? '#94a3b8' : period().color}"></span>
            <span class="period-select__name">${esc(periodName)}</span>
            <span class="period-select__chev">▾</span>
          </button>
          ${m.pickerPeriodMenu ? `<div class="period-select__menu" style="left:${(m.pickerPeriodRect || {}).left || 0}px;top:${(m.pickerPeriodRect || {}).top || 0}px;width:320px">
            <div class="period-select__group">Периоды</div>
            <button class="period-select__item${m.pickerPeriod === 'all' ? ' is-selected' : ''}" data-picker-period-set="all">
              <span class="period-select__indent"></span><span class="period-select__item-name">Все периоды</span>
              <span class="period-select__range"></span><span class="period-select__badge-wrap"></span>
            </button>
            ${PERIODS.map(p => `<button class="period-select__item${m.pickerPeriod === p.id ? ' is-selected' : ''}" data-picker-period-set="${p.id}">
              <span class="period-select__indent" style="width:${(p.depth || 0) * 16}px"></span><span class="period-select__item-name">${esc(p.name)}</span>
              <span class="period-select__range">${esc(p.range)}</span><span class="period-select__badge-wrap"></span>
            </button>`).join('')}
          </div>` : ''}
        </div>
        <input id="picker-q" class="goal-parent-picker__search" placeholder="Поиск по названию или команде…" value="${esc(m.pickerQ || '')}">
      </div>
      <div class="goal-parent-picker__list" id="picker-list">${pickerListHTML(m, groups, selected)}</div>
    </div>
  </div>`;
}

function pickerListHTML(m, groups, selected) {
  if (!groups.length) return '<div class="goal-parent-picker__empty">Ничего не найдено</div>';
  return groups.map(g => `<div class="goal-parent-picker__group">
    <div class="goal-parent-picker__group-head" style="padding-left:${8 + g.team.depth * 18}px">
      <span class="goal-parent-picker__group-dot" style="background:${TEAM_TYPE_COLOR[g.team.type] || '#6b7280'}"></span>
      ${esc(g.team.name)}
      <span class="goal-parent-picker__group-type">${esc((TEAM_TYPE_LABEL[g.team.type] || '').toUpperCase())}</span>
    </div>
    ${g.rows.map(it => {
      const isSel = selected.has(it.id);
      return `<button type="button" class="goal-parent-picker__row${isSel ? ' goal-parent-picker__row--selected' : ''}"
          style="padding-left:${26 + g.team.depth * 18}px" data-picker-pick="${g.team.id}:${it.id}"${isSel ? ' disabled' : ''}>
        <span class="goal-parent-picker__row-title">${esc(it.title)}</span>
        <span class="goal-parent-picker__row-meta">${esc(goalsPeriod().name)} · ${esc(g.team.name)} · ${goalProgress(it)}%</span>
        ${isSel ? '<span class="goal-parent-picker__row-check">✓</span>' : ''}
      </button>`;
    }).join('')}
  </div>`).join('');
}

// ── Модалка цели ─────────────────────────────────────────────────────────────
// Повторяет GoalModal: шапка с периодом и командой, название, markdown-описание,
// приоритет сегментами, вес со счётчиком, тип работы, фокус, драйвер, тумблер
// общей цели и секция связанных целей.
function goalModal(m) {
  const f = m.form;
  const team = findTeam(state.teamId);
  const others = goalsOf(state.teamId).filter(g => g.id !== m.goalId);
  const usedWeight = others.reduce((s, g) => s + g.weight, 0);
  const weightNum = Number(f.weight) || 0;
  const totalAfter = usedWeight + weightNum;
  const overWeight = totalAfter > 100;
  const valid = f.title.trim() && (!f.shared || f.shareTeamIds.length > 0);

  return `<div class="modal-overlay modal-overlay--z400">
    <div class="modal-box modal-box--w600">
      <div class="modal-header modal-header--sticky modal-header--goal">
        <div>
          <div class="modal-title--goal">${m.goalId ? 'Редактировать цель' : 'Новая цель'}</div>
          <div class="modal-subtitle">${esc(period().name)} · ${esc(team.name)}</div>
        </div>
        <button class="modal-close modal-close--lg" data-close="1">×</button>
      </div>

      <div class="modal-body modal-body--goal">
        <div class="form-row goal-name-row">
          <div class="form-col">
            ${fieldLabel('Название', true, 'Objective — качественное описание того, чего команда хочет достичь. Без цифр (они в KR).')}
            <input id="goal-title" value="${esc(f.title)}" placeholder="Чего хотим достичь?" class="form-input form-input--goal">
          </div>
          <div class="form-col--w140">
            ${fieldLabel(`<span>Вес <span id="weight-counter" style="font-weight:400;color:${overWeight ? '#d97706' : '#9ca3af'};font-size: 13px">(${totalAfter}/100)</span></span>`, false, 'Доля цели в общем результате команды. Сумма весов = 100%.')}
            <div class="form-weight-wrap">
              <input id="goal-weight" type="number" min="0" max="100" value="${f.weight}" class="form-input form-input--weight">
              <span class="form-weight-pct">%</span>
            </div>
            ${overWeight ? '<div class="form-error-msg" style="color:#d97706">Сумма весов больше 100% — сохранить можно</div>' : ''}
          </div>
        </div>

        <div class="form-group">
          ${fieldLabel('Описание', false, 'Контекст, почему эта цель важна. Не дублируйте название.')}
          ${markdownEditor(f.desc, 'Дополнительный контекст…', m.preview)}
        </div>

        <div class="form-row goal-meta-row">
          <div class="form-col">
            ${fieldLabel('Приоритет', false, PRIORITY_HINT_TEXT, PRIORITY_HINT_HTML)}
            <div class="pri-select" data-menu-wrap>
              <button type="button" class="pri-select__btn" data-pri-open="1" aria-haspopup="listbox" aria-expanded="${!!m.priOpen}">
                ${priBadge(f.priority)}<span class="pri-select__txt">${esc(PRI_SHORT[f.priority] || '')}</span><span class="pri-select__caret">▾</span>
              </button>
              ${m.priOpen ? `<div class="pri-select__menu" role="listbox">
                ${['P0', 'P1', 'P2', 'P3'].map(p => `<button type="button" role="option" aria-selected="${f.priority === p}" class="pri-select__opt${f.priority === p ? ' pri-select__opt--on' : ''}" data-pri="${p}">
                  ${priBadge(p)}<span class="pri-select__opt-txt"><b>${esc(PRI_SHORT[p])}</b><span>${esc(PRI_HINT[p] || '')}</span></span>${f.priority === p ? '<span class="pri-select__check">✓</span>' : ''}
                </button>`).join('')}
              </div>` : ''}
            </div>
          </div>
          <div class="form-col">
            ${fieldLabel('Тип работы', false, 'Delivery — известный результат. Discovery — исследование гипотезы.')}
            <div class="seg-group">
              ${['delivery', 'discovery'].map(t => {
                const sel = f.type === t;
                return `<button class="seg-btn" data-wtype="${t}" style="border-color:${sel ? ACCENT : '#e5e7eb'};background:${sel ? ACCENT + '10' : 'white'};color:${sel ? ACCENT : '#6b7280'}">${t}</button>`;
              }).join('')}
            </div>
          </div>
          <div class="form-col">
            ${fieldLabel('Фокус', false, FOCUS_HINT_TEXT, FOCUS_HINT_HTML)}
            <select id="goal-focus" class="form-select">
              ${Object.keys(FOCUS_LABEL).map(k => `<option value="${k}"${f.focus === k ? ' selected' : ''}>${FOCUS_LABEL[k]}</option>`).join('')}
            </select>
          </div>
        </div>


        <div class="form-group">
          ${fieldLabel('Драйвер цели', false, 'Ответственный за достижение цели.\nСледит за прогрессом KR, обновляет статус и эскалирует блокеры.\nМожно указать несколько человек.')}
          ${userSelector(m)}
        </div>

        <div class="toggle-box">
          <label class="toggle-row">
            <div class="toggle-track${f.shared ? ' toggle-track--on' : ''}" data-shared="1">
              <div class="toggle-knob${f.shared ? ' toggle-knob--on' : ''}"></div>
            </div>
            <div class="toggle-text">
              <div class="toggle-title">Общая цель</div>
              <div class="toggle-subtitle">Разделить цель с другими командами</div>
            </div>
          </label>
          ${f.shared ? `<div class="toggle-content">
            <div class="toggle-content__label">С какими командами <span class="toggle-content__required">*</span></div>
            ${teamCombobox(m)}
          </div>` : ''}
        </div>

        <div class="goal-links-section">
          <div class="goal-links-section__head">
            🔗 Связанные цели
            <span class="info-hint" title="Связь привязывает эту цель к верхнеуровневой (цель руководителя, годовая цель, цель юнита/кластера). Это НЕ делает цель общей — общие цели (⇄) видны нескольким командам, а связь соединяет две разные цели отношением «дочерняя → родительская».">?</span>
          </div>
          <div class="goal-links-section__hint">К какой верхнеуровневой цели относится эта цель. Можно указать несколько.</div>
          ${f.parents.map((p, i) => `<div class="goal-link-card">
            <span class="goal-link-card__arrow">↑</span>
            <span class="goal-link-card__title">${esc(p.title)}</span>
            <span class="goal-link-card__meta">${esc(p.periodName)} · ${esc(p.teamName)} · ${p.progress}%</span>
            <button type="button" class="goal-link-card__remove" title="Убрать связь" data-parent-del="${i}">✕</button>
          </div>`).join('')}
          <button type="button" class="goal-link-add" data-parent-add="1">+ Добавить связь</button>
        </div>
      </div>

      <div class="modal-footer modal-footer--goal">
        <button class="btn btn--secondary" style="padding:10px 20px;font-size: 15px" data-close="1">Отмена</button>
        <button class="btn btn--primary" style="padding:10px 28px;font-size: 15px;background:${valid ? ACCENT : '#e5e7eb'};color:${valid ? 'white' : '#9ca3af'};cursor:${valid ? 'pointer' : 'default'}"
          ${valid ? 'id="goal-save"' : 'disabled'}>${m.goalId ? 'Сохранить' : 'Создать цель'}</button>
      </div>
    </div>
  </div>`;
}

function notifPanel() {
  return `<div class="notif__backdrop"></div>
  <div class="notif__panel">
    <div class="notif__panel-head">
      <span class="notif__panel-title">Уведомления</span>
      <button class="notif__mark-all" id="mark-all">Прочитать все</button>
      <button class="notif__close" data-close="1">×</button>
    </div>
    <div class="notif__list">
      ${NOTIFICATIONS.length ? NOTIFICATIONS.map(n => `<div class="notif__entry">
        <div class="notif__item${n.unread ? ' notif__item--unread' : ''}">
          <div class="notif__item-row"><div class="notif__item-main">
            <div class="notif__item-head">
              <span class="notif__item-title">${esc(n.title)}</span>
              <span class="notif__item-time">${esc(n.time)}</span>
            </div>
            <div class="notif__item-body">${esc(n.body)}</div>
            <div class="notif__item-context">${esc(n.context)}</div>
          </div></div>
        </div>
        <button class="notif__item-delete" data-notif-del="${n.id}" aria-label="Удалить">×</button>
      </div>`).join('')
      : '<div class="notif__empty"><div class="notif__empty-icon">🔕</div>Уведомлений пока нет</div>'}
    </div>
  </div>`;
}

function renderOverlays() {
  const el = document.getElementById('overlays');
  if (state.notifOpen) { el.innerHTML = notifPanel(); return; }
  const m = state.modal;
  if (!m) { el.innerHTML = ''; return; }
  if (m.type === 'kr') { el.innerHTML = krModal(m); return; }
  if (m.type === 'kredit') { el.innerHTML = krEditModal(m); return; }
  if (m.type === 'export') { el.innerHTML = exportModal(m); return; }
  if (m.type === 'transfer') { el.innerHTML = transferModal(m); return; }
  el.innerHTML = goalModal(m) + (m.pickerOpen ? goalParentPicker(m) : '');
}

// Имена драйверов уступают место заголовку цели: показываем их, пока из-за них не обрезается
// заголовок. То же правило, что у useOwnersFit в tracker.js, но здесь состояния нет, поэтому
// оно выражено одним проходом: снять модификатор со всех шапок, измерить, вернуть там, где
// заголовок обрезан. Императивный проход цикла не образует, и отметка «не влезло» ему не нужна.
function fitGoalCardOwners() {
  document.querySelectorAll('.gc2__head').forEach(head => {
    const owner = head.querySelector('.gc2__owner');
    const title = head.querySelector('.goal-card__title');
    if (!owner || !title) return;
    owner.classList.remove('gc2__owner--compact');
    if (title.scrollWidth - title.clientWidth > 1) owner.classList.add('gc2__owner--compact');
  });
}

// Подсказка с полным текстом только у обрезанного заголовка — то же правило, что у useClipped
// в tracker.js: у влезающего она дублировала бы видимый текст и отняла бы подсказку действия
// у строки-предка. В приложении это React-хук с ResizeObserver, здесь достаточно прохода
// после отрисовки и по изменению размера окна.
function markClippedTitles() {
  document.querySelectorAll('.goal-card__title, .kr-name').forEach(el => {
    if (el.scrollWidth - el.clientWidth > 1) el.setAttribute('title', el.textContent);
    else el.removeAttribute('title');
  });
}

// Порядок важен: форма драйверов меняет ширину заголовка, поэтому подсказки считаем после неё.
function refitBoard() { fitGoalCardOwners(); markClippedTitles(); }
window.addEventListener('resize', refitBoard);

// ── Рендер целиком ───────────────────────────────────────────────────────────
function render() {
  document.getElementById('app').className = 'app' + (state.navOpen ? ' app--nav-open' : '');
  renderSidebar();
  renderTopbar();
  const content = document.getElementById('content');
  content.className = 'content' + (['overview', 'tree', 'activity'].includes(state.section) ? ' content--flush' : '');
  content.innerHTML = state.section === 'overview' ? renderOverview()
    : state.section === 'tree' ? renderGoalTree()
    : state.section === 'activity' ? renderActivity() : renderTracker();
  if (state.section === 'tree') fitGoalTree();
  renderOverlays();
  refitBoard();
}
const redrawContent = () => {
  document.getElementById('content').innerHTML =
    state.section === 'overview' ? renderOverview()
      : state.section === 'tree' ? renderGoalTree()
      : state.section === 'activity' ? renderActivity() : renderTracker();
  refitBoard();
};

// ── События ──────────────────────────────────────────────────────────────────
function closeOverlays() { state.modal = null; state.notifOpen = false; renderOverlays(); }

document.addEventListener('click', e => {
  const t = e.target;
  const hit = sel => t.closest && t.closest(sel);

  if (t.id === 'banner-close') { document.getElementById('banner').hidden = true; return; }
  if (t.classList.contains('modal-overlay') || t.classList.contains('notif__backdrop')) { closeOverlays(); return; }
  if (hit('button[data-close]')) { closeOverlays(); return; }

  // Меню «···» на карточке цели: закрывается кликом вне своего блока.
  if (hit('[data-menu-wrap] [aria-disabled]')) return;
  if (hit('[data-menu-close]')) { state.goalMenu = null; state.krMenu = null; redrawContent(); }
  const menuBtn = hit('[data-menu]');
  if (menuBtn) {
    const id = +menuBtn.dataset.menu;
    state.goalMenu = state.goalMenu === id ? null : id;
    state.krMenu = null;
    redrawContent(); return;
  }
  const krMenuBtn = hit('[data-kr-menu]');
  if (krMenuBtn) {
    const key = krMenuBtn.dataset.krMenu;
    state.krMenu = state.krMenu === key ? null : key;
    state.goalMenu = null;
    redrawContent(); return;
  }
  if (state.krMenu && !hit('[data-menu-wrap]')) { state.krMenu = null; redrawContent(); }
  const pri = hit('[data-pri-filter]');
  if (pri) { const p = pri.dataset.priFilter; state.boardView.pri[p] = !state.boardView.pri[p]; render(); return; }
  if (hit('[data-pri-clear]')) { state.boardView.pri = {}; render(); return; }
  const fltSort = hit('[data-flt-sort]');
  if (fltSort) { state.boardView.sort = fltSort.dataset.fltSort; render(); return; }
  // Переключение птички сбрасывает перекрытия показа заметок у отдельных KR — после него
  // доска однородна. Та же механика, что у notesOpen в KRRow.
  if (hit('[data-flt-notes]')) { state.boardView.notes = !state.boardView.notes; state.noteVis = {}; state.noteCollapsed = {}; render(); return; }
  if (hit('[data-flt-zero]')) { state.boardView.hideZeroWeight = !state.boardView.hideZeroWeight; render(); return; }
  if (hit('[data-flt-reset]')) {
    state.boardView = { sort: 'custom', pri: {}, notes: false, hideZeroWeight: false };
    state.noteVis = {}; state.noteCollapsed = {};
    render(); return;
  }
  if (hit('[data-topmenu]')) { state.topMenu = !state.topMenu; render(); return; }
  if (hit('[data-open-team-export]')) {
    state.topMenu = false;
    const first = goalsOf(state.teamId)[0];
    state.modal = { type: 'export', goalId: first ? first.id : null, scope: 'team', full: false, comments: false, copied: false };
    render(); renderOverlays(); return;
  }
  if (state.topMenu && !hit('[data-topmenu-wrap]')) { state.topMenu = false; render(); }
  const openExport = hit('[data-open-export]');
  if (openExport) {
    state.goalMenu = null;
    state.modal = { type: 'export', goalId: +openExport.dataset.openExport, scope: 'goal', full: false, comments: false, copied: false };
    redrawContent(); renderOverlays(); return;
  }
  const openTransfer = hit('[data-open-transfer]');
  if (openTransfer) {
    state.goalMenu = null;
    state.modal = { type: 'transfer', goalId: +openTransfer.dataset.openTransfer, mode: 'copy',
      targetTeam: state.teamId, targetPeriod: state.periodId, withComments: false, withProgress: false,
      teamOpen: false, teamQ: '', periodMenu: false };
    redrawContent(); renderOverlays(); return;
  }
  if (state.goalMenu !== null && !hit('[data-menu-wrap]')) { state.goalMenu = null; redrawContent(); }

  // ── Внутри модалки экспорта ──
  if (state.modal && state.modal.type === 'export') {
    const m3 = state.modal;
    const sc = hit('[data-scope-set]');
    if (sc) { m3.scope = sc.dataset.scopeSet; renderOverlays(); return; }
    if (t.dataset.expFull != null) { m3.full = t.checked; renderOverlays(); return; }
    if (t.dataset.expComments != null) { m3.comments = t.checked; renderOverlays(); return; }
    if (hit('[data-exp-copy]')) {
      const md = exportMarkdown(m3);
      if (navigator.clipboard && navigator.clipboard.writeText) navigator.clipboard.writeText(md).catch(() => {});
      m3.copied = true; renderOverlays();
      setTimeout(() => { if (state.modal === m3) { m3.copied = false; renderOverlays(); } }, 1500);
      return;
    }
    if (hit('[data-exp-download]')) {
      // Песочница опубликованного артефакта не даёт странице отдать файл, поэтому
      // там кнопка честно объясняет это вместо молчания. В локальной копии
      // (design-system/dist/prototype.local.html) скачивание работает.
      if (window.claude) { m3.downloadBlocked = true; renderOverlays(); return; }
      const blob = new Blob([exportMarkdown(m3)], { type: 'text/markdown;charset=utf-8' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url; a.download = 'okr-export.md';
      document.body.appendChild(a); a.click(); document.body.removeChild(a);
      URL.revokeObjectURL(url);
      return;
    }
  }

  // ── Внутри модалки переноса ──
  if (state.modal && state.modal.type === 'transfer') {
    const m4 = state.modal;
    if (!hit('.team-combobox') && m4.teamOpen) { m4.teamOpen = false; renderOverlays(); }
    const trMode = hit('[data-tr-mode]');
    if (trMode) { m4.mode = trMode.dataset.trMode; renderOverlays(); return; }
    if (hit('[data-transfer-field]')) { m4.teamOpen = true; renderOverlays(); document.getElementById('tr-team-q')?.focus(); return; }
    const trTeam = hit('[data-transfer-team]');
    if (trTeam) {
      const id = +trTeam.dataset.transferTeam;
      const st = boardOf(id).status;
      if (!['in_progress', 'closed'].includes(st)) { m4.targetTeam = id; m4.teamOpen = false; m4.teamQ = ''; }
      renderOverlays(); return;
    }
    const trPer = hit('[data-tr-period]');
    if (trPer) {
      const r = trPer.getBoundingClientRect();
      m4.periodRect = { left: r.left, top: r.bottom + 4 };
      m4.periodMenu = !m4.periodMenu; renderOverlays(); return;
    }
    const trPerSet = hit('[data-tr-period-set]');
    if (trPerSet) { m4.targetPeriod = +trPerSet.dataset.trPeriodSet; m4.periodMenu = false; renderOverlays(); return; }
    if (t.dataset.trComments != null) { m4.withComments = t.checked; return; }
    if (t.dataset.trProgress != null) { m4.withProgress = t.checked; return; }
    if (hit('[data-tr-submit]')) { closeOverlays(); render(); return; }
  }

  if (t.id === 'burger') { state.navOpen = true; render(); return; }
  if (t.id === 'scrim') { state.navOpen = false; render(); return; }
  if (hit('[data-user-menu]')) { state.userMenu = !state.userMenu; renderSidebar(); return; }
  if (state.userMenu && !hit('.sidebar__user')) { state.userMenu = false; renderSidebar(); }

  if (hit('[data-tenant]')) { state.tenantMenu = !state.tenantMenu; renderSidebar(); return; }
  const tenantSet = hit('[data-tenant-set]');
  if (tenantSet) {
    const id = +tenantSet.dataset.tenantSet;
    TENANTS.forEach(t => { t.active = t.id === id; });
    state.tenantMenu = false;
    // В продукте смена пространства перезагружает страницу; здесь просто
    // перерисовываем — данные у прототипа одни на все пространства.
    render(); return;
  }
  if (state.tenantMenu && !hit('.sidebar__tenant')) { state.tenantMenu = false; renderSidebar(); }

  if (hit('#bell')) { state.notifOpen = true; renderOverlays(); return; }
  if (t.dataset.notifDel) {
    const i = NOTIFICATIONS.findIndex(n => n.id === +t.dataset.notifDel);
    if (i >= 0) NOTIFICATIONS.splice(i, 1);
    renderOverlays(); renderSidebar(); return;
  }
  if (t.id === 'mark-all') { NOTIFICATIONS.forEach(n => n.unread = false); renderOverlays(); renderSidebar(); return; }

  const section = hit('[data-section]');
  if (section) {
    const id = section.dataset.section;
    state.section = id === 'period-overview' ? 'overview' : id === 'goal-tree' ? 'tree'
      : id === 'activity-log' ? 'activity' : 'tracker';
    if (state.section === 'tree') state.tree.needFit = true;
    state.navOpen = false; state.drill = null;
    render(); return;
  }

  const scopeBtn = hit('[data-scope]');
  if (scopeBtn) { state.scope = scopeBtn.dataset.scope; state.drill = null; render(); return; }
  if (hit('[data-drill-close]')) { state.drill = null; redrawContent(); return; }
  const drillEl = hit('[data-drill]');
  if (drillEl) {
    const [kind, key, title] = drillEl.dataset.drill.split(':');
    state.drill = (state.drill && state.drill.kind === kind && state.drill.key === key) ? null : { kind, key, title };
    redrawContent(); return;
  }
  const balEl = hit('[data-bal]');
  if (balEl) {
    const [field, key] = balEl.dataset.bal.split(':');
    const titles = {
      type: k => 'Discovery / Delivery: ' + (PO_DD_LABELS[k] || k),
      focus: k => 'Фокус: ' + (PO_FOCUS_LABELS[k] || k),
      priority: k => 'Приоритет: ' + (PO_PRIO_LABELS[k] || k),
      healthStatus: k => 'Статус KR: ' + (PO_HEALTH_LABELS[k] || k),
    };
    const kind = field === 'healthStatus' ? 'krhealth' : 'balance';
    state.drill = (state.drill && state.drill.key === key && state.drill.field === field)
      ? null : { kind, field, key, title: titles[field](key) };
    redrawContent(); return;
  }

  const sbPer = hit('#period-trigger');
  if (sbPer) {
    const r = sbPer.getBoundingClientRect();
    state.periodMenuRect = { left: r.left, top: r.bottom + 4 };
    state.periodMenu = !state.periodMenu;
    renderSidebar(); return;
  }
  const per = hit('[data-period]');
  if (per) { state.periodId = +per.dataset.period; state.periodMenu = false; render(); return; }

  if (t.dataset.toggle) { state.expanded[+t.dataset.toggle] = !state.expanded[+t.dataset.toggle]; renderSidebar(); return; }
  if (t.dataset.fav) { state.favorites[+t.dataset.fav] = !state.favorites[+t.dataset.fav]; renderSidebar(); return; }

  const teamEl = hit('[data-team]');
  if (teamEl) {
    state.teamId = +teamEl.dataset.team;
    state.section = 'tracker'; state.navOpen = false;
    render(); return;
  }

  const comments = hit('[data-comments]');
  const noteVis = hit('[data-kr-note-vis]');
  if (noteVis) {
    const id = +noteVis.dataset.krNoteVis;
    state.noteVis[id] = !krNoteVisible(id);
    state.krMenu = null;
    redrawContent(); return;
  }
  const noteEl = hit('[data-kr-note]');
  if (noteEl) {
    const id = +noteEl.dataset.krNote;
    state.noteCollapsed[id] = !state.noteCollapsed[id];
    redrawContent(); return;
  }
  if (comments) {
    const id = +comments.dataset.comments;
    state.showCom[id] = !state.showCom[id];
    redrawContent(); return;
  }
  const resolve = hit('[data-resolve]');
  if (resolve) {
    const id = +resolve.dataset.resolve;
    goalsOf(state.teamId).forEach(g => g.comments.forEach(c => {
      if (c.id === id) { c.resolved = true; c.resolvedBy = ME.name; c.resolvedAt = 'только что'; }
    }));
    redrawContent(); return;
  }
  const send = hit('[data-send]');
  if (send) {
    const id = +send.dataset.send;
    const ta = document.querySelector(`[data-compose="${id}"]`);
    const text = ta && ta.value.trim();
    if (!text) return;
    goalsOf(state.teamId).find(g => g.id === id).comments
      .unshift({ id: Date.now(), author: ME.name, initials: ME.initials, date: 'только что', text, resolved: false, replies: [] });
    redrawContent(); return;
  }

  const krBtn = hit('[data-kr-progress]');
  if (krBtn) {
    const [goalId, krId] = krBtn.dataset.krProgress.split(':').map(Number);
    const { kr } = findKR(goalId, krId);
    state.modal = { type: 'kr', goalId, krId,
      draft: { current: kr.current, done: kr.done, healthStatus: kr.healthStatus, stages: (kr.stages || []).map(s => ({ ...s })) } };
    renderOverlays(); return;
  }
  const health = hit('[data-health]');
  if (health && state.modal) { state.modal.draft.healthStatus = health.dataset.health; renderOverlays(); return; }
  if (t.dataset.stage != null && state.modal) { state.modal.draft.stages[+t.dataset.stage].done = t.checked; renderOverlays(); return; }
  if (t.id === 'kr-done' && state.modal) { state.modal.draft.done = t.checked; renderOverlays(); return; }
  if (t.id === 'kr-save' && state.modal) {
    const { kr } = findKR(state.modal.goalId, state.modal.krId);
    Object.assign(kr, { current: state.modal.draft.current, done: state.modal.draft.done,
      healthStatus: state.modal.draft.healthStatus, updatedDaysAgo: 0 });
    if (kr.stages) kr.stages = state.modal.draft.stages;
    closeOverlays(); render(); return;
  }

  // ── Редактор Key Result ──
  const krAdd = hit('[data-kr-add]');
  const krEdit = hit('[data-kr-edit]');
  if (krAdd || krEdit) {
    let goalId, kr = null;
    if (krEdit) {
      const [gid, kid] = krEdit.dataset.krEdit.split(':').map(Number);
      goalId = gid; kr = findKR(gid, kid).kr;
    } else goalId = +krAdd.dataset.krAdd;
    state.modal = {
      type: 'kredit', goalId, krId: kr ? kr.id : null, preview: false, showZeroing: !!(kr && kr.zeroing),
      form: kr
        ? { name: kr.name, desc: kr.desc || '', weight: kr.weight, krType: kr.krType, unit: kr.unit || '%',
            start: kr.start ?? 0, target: kr.target ?? 100, current: kr.current ?? 0, done: !!kr.done,
            stages: (kr.stages || []).map(s => ({ ...s })), checkpoints: (kr.checkpoints || []).map(c => ({ ...c })), zeroing: kr.zeroing || '' }
        : { name: '', desc: '', weight: 20, krType: 'NUMERICAL', unit: '%', start: 0, target: 100, current: 0,
            done: false, stages: [], checkpoints: [], zeroing: '' },
    };
    renderOverlays(); return;
  }

  if (state.modal && state.modal.type === 'kr') {
    const mk = state.modal;
    const tabK = hit('[data-md-tab]');
    if (tabK) { const ta = document.getElementById('kr-comment'); if (ta) mk.comment = ta.value; mk.preview = tabK.dataset.mdTab === 'preview'; renderOverlays(); return; }
    const mdK = hit('[data-md]');
    if (mdK) {
      const ta = document.getElementById('kr-comment');
      if (ta) applyMarkdownFormat(ta, mdK.dataset.md, v => { mk.comment = v; ta.value = v; });
      return;
    }
  }

  if (state.modal && state.modal.type === 'kredit') {
    const m5 = state.modal;
    const f5 = m5.form;
    const tab = hit('[data-md-tab]');
    if (tab) { m5.preview = tab.dataset.mdTab === 'preview'; renderOverlays(); return; }
    const md = hit('[data-md]');
    if (md) {
      const ta = document.getElementById('kre-desc');
      if (ta) applyMarkdownFormat(ta, md.dataset.md, v => { f5.desc = v; ta.value = v; });
      return;
    }
    const kt = hit('[data-kre-type]');
    if (kt) { f5.krType = kt.dataset.kreType; renderOverlays(); return; }
    if (hit('[data-zeroing]')) { m5.showZeroing = true; renderOverlays(); return; }
    if (hit('[data-cp-add]')) { f5.checkpoints.push({ value: '', progress_percent: '' }); renderOverlays(); return; }
    const cpDel = hit('[data-cp-del]');
    if (cpDel) { f5.checkpoints.splice(+cpDel.dataset.cpDel, 1); renderOverlays(); return; }
    if (hit('[data-st-add]')) { f5.stages.push({ title: '', weight: 0, done: false }); renderOverlays(); return; }
    const stDel = hit('[data-st-del]');
    if (stDel) { f5.stages.splice(+stDel.dataset.stDel, 1); renderOverlays(); return; }
    if (t.dataset.stDone != null) { f5.stages[+t.dataset.stDone].done = t.checked; renderOverlays(); return; }
    if (t.id === 'kre-done') { f5.done = t.checked; renderOverlays(); return; }
    if (hit('[data-kre-save]')) {
      const goal = goalsOf(state.teamId).find(g => g.id === m5.goalId);
      const payload = {
        name: f5.name.trim(), desc: f5.desc, weight: Number(f5.weight) || 0, krType: f5.krType,
        unit: f5.unit, start: Number(f5.start) || 0, target: Number(f5.target) || 0, current: m5.krId ? (Number(f5.current) || 0) : (Number(f5.start) || 0),
        done: f5.done, stages: f5.stages, checkpoints: f5.checkpoints, zeroing: f5.zeroing, updatedDaysAgo: 0,
      };
      if (m5.krId) Object.assign(goal.krs.find(k => k.id === m5.krId), payload);
      else goal.krs.push({ id: Date.now(), healthStatus: 'not_started', ...payload });
      closeOverlays(); render(); return;
    }
  }

  const modalBtn = hit('[data-modal]');
  if (modalBtn) {
    const gid = modalBtn.dataset.goal ? +modalBtn.dataset.goal : null;
    const g = gid ? goalsOf(state.teamId).find(x => x.id === gid) : null;
    const used = goalsOf(state.teamId).filter(x => !gid || x.id !== gid).reduce((s, x) => s + x.weight, 0);
    state.modal = {
      type: 'goal', goalId: gid, preview: false,
      form: g
        ? { title: g.title, desc: g.desc || '', priority: g.priority, weight: g.weight, type: g.type,
            focus: g.focus,
            // Драйверы и команды-участники приводим к тем же сущностям, с которыми
            // работают UserSelector и TeamCombobox.
            owners: (g.owners || []).map(o => USERS.find(u => u.name === o.name) || { udid: 'x' + o.name, ...o }),
            shared: !!g.shareTeams,
            shareTeamIds: g.shareTeams
              ? g.shareTeams.teams.map(n => (flatTeams().find(t => t.name === n) || {}).id).filter(Boolean)
              : [],
            parents: [...(g.parents || [])] }
        : { title: '', desc: '', priority: 'P1', weight: Math.max(0, Math.min(20, 100 - used)),
            type: 'delivery', focus: 'PROFITABILITY', owners: [], shared: false, shareTeamIds: [], parents: [] },
    };
    renderOverlays(); return;
  }

  // ── Внутри модалки цели ──
  if (state.modal && state.modal.type === 'goal') {
    const m2 = state.modal;
    const f = m2.form;
    const pri = hit('[data-pri]');
    if (pri) { f.priority = pri.dataset.pri; m2.priOpen = false; renderOverlays(); return; }
    if (hit('[data-pri-open]')) { m2.priOpen = !m2.priOpen; renderOverlays(); return; }
    if (m2.priOpen && !hit('.pri-select')) { m2.priOpen = false; renderOverlays(); }
    const wt = hit('[data-wtype]');
    if (wt) { f.type = wt.dataset.wtype; renderOverlays(); return; }
    if (hit('[data-shared]')) { f.shared = !f.shared; renderOverlays(); return; }
    const tab = hit('[data-md-tab]');
    if (tab) { state.modal.preview = tab.dataset.mdTab === 'preview'; renderOverlays(); return; }
    const md = hit('[data-md]');
    if (md) {
      const ta = document.getElementById('goal-desc');
      if (ta) applyMarkdownFormat(ta, md.dataset.md, v => { f.desc = v; ta.value = v; });
      return;
    }
    // Выпадающие списки закрываются кликом вне своего блока.
    if (!hit('.user-selector') && m2.userOpen) { m2.userOpen = false; renderOverlays(); }
    if (!hit('.team-combobox') && m2.teamOpen) { m2.teamOpen = false; renderOverlays(); }

    if (hit('[data-user-field]')) { m2.userOpen = true; renderOverlays(); document.getElementById('user-q')?.focus(); return; }
    const userPick = hit('[data-user-pick]');
    if (userPick) {
      const u = USERS.find(x => x.udid === userPick.dataset.userPick);
      if (u) f.owners.push(u);
      m2.userQ = '';
      renderOverlays(); document.getElementById('user-q')?.focus(); return;
    }
    const ownerDel = hit('[data-owner-del]');
    if (ownerDel) { f.owners = f.owners.filter(u => u.udid !== ownerDel.dataset.ownerDel); renderOverlays(); return; }

    if (hit('[data-team-field]')) { m2.teamOpen = true; renderOverlays(); document.getElementById('team-q')?.focus(); return; }
    const teamPick = hit('[data-team-pick]');
    if (teamPick) {
      const id = +teamPick.dataset.teamPick;
      if (!teamShareBlocked(id)) { f.shareTeamIds.push(id); m2.teamQ = ''; }
      renderOverlays(); document.getElementById('team-q')?.focus(); return;
    }

    // Выбор родительской цели
    if (hit('[data-picker-close]') || t.dataset.pickerOverlay) { m2.pickerOpen = false; m2.pickerPeriodMenu = false; renderOverlays(); return; }
    const pickerPer = hit('[data-picker-period]');
    if (pickerPer) {
      const r = pickerPer.getBoundingClientRect();
      m2.pickerPeriodRect = { left: r.left, top: r.bottom + 4 };
      m2.pickerPeriodMenu = !m2.pickerPeriodMenu;
      renderOverlays(); return;
    }
    const perSet = hit('[data-picker-period-set]');
    if (perSet) {
      const v = perSet.dataset.pickerPeriodSet;
      m2.pickerPeriod = v === 'all' ? 'all' : +v;
      m2.pickerPeriodMenu = false; renderOverlays(); return;
    }
    const pick = hit('[data-picker-pick]');
    if (pick) {
      const [teamId, goalId] = pick.dataset.pickerPick.split(':').map(Number);
      const team = findTeam(teamId);
      const g = goalsOf(teamId).find(x => x.id === goalId);
      if (g && !f.parents.some(p => p.id === g.id)) {
        f.parents.push({ id: g.id, title: g.title, periodName: goalsPeriod().name, teamName: team.name, progress: goalProgress(g) });
      }
      m2.pickerOpen = false; renderOverlays(); return;
    }
    const shareDel = hit('[data-share-del]');
    if (shareDel) { f.shareTeamIds = f.shareTeamIds.filter(id => id !== +shareDel.dataset.shareDel); renderOverlays(); return; }
    const parentDel = hit('[data-parent-del]');
    if (parentDel) { f.parents.splice(+parentDel.dataset.parentDel, 1); renderOverlays(); return; }
    if (hit('[data-parent-add]')) { m2.pickerOpen = true; m2.pickerPeriod = state.periodId; m2.pickerQ = ''; renderOverlays(); return; }
    if (t.id === 'goal-save') {
      const g = state.modal.goalId ? goalsOf(state.teamId).find(x => x.id === state.modal.goalId) : null;
      if (g) Object.assign(g, { title: f.title, desc: f.desc, priority: f.priority, weight: Number(f.weight) || 0, type: f.type, focus: f.focus, owners: f.owners, updatedDaysAgo: 0 });
      else goalsOf(state.teamId).push({ id: Date.now(), title: f.title, desc: f.desc, priority: f.priority,
        weight: Number(f.weight) || 0, type: f.type, focus: f.focus, owners: f.owners, updatedDaysAgo: 0,
        parents: 0, children: 0, shareTeams: null, krs: [], comments: [] });
      closeOverlays(); render(); return;
    }
  }

  const status = hit('[data-status]');
  if (status && !status.disabled) {
    boardOf(state.teamId).status = status.dataset.status;
    boardOf(state.teamId).changedAt = 'только что';
    render(); return;
  }
});

document.addEventListener('mouseover', e => {
  if (state.section !== 'overview') return;
  const g = e.target.closest && e.target.closest('[data-pt]');
  const idx = g ? +g.dataset.pt : null;
  if (idx === state.chartHover) return;
  state.chartHover = idx;
  const chart = document.getElementById('chart');
  if (chart) chart.innerHTML = progressChartSVG();
});

function syncGoalModalChrome() {
  const m = state.modal;
  if (!m || m.type !== 'goal') return;
  const f = m.form;
  const used = goalsOf(state.teamId).filter(x => !m.goalId || x.id !== m.goalId).reduce((s, x) => s + x.weight, 0);
  const total = used + (Number(f.weight) || 0);
  const over = total > 100;
  const counter = document.getElementById('weight-counter');
  if (counter) { counter.textContent = `(${total}/100)`; counter.style.color = over ? '#d97706' : '#9ca3af'; }
  const save = document.querySelector('.modal-footer--goal .btn--primary');
  if (save) {
    const valid = f.title.trim() && (!f.shared || f.shareTeamIds.length > 0);
    save.disabled = !valid;
    save.id = valid ? 'goal-save' : '';
    save.style.background = valid ? ACCENT : '#e5e7eb';
    save.style.color = valid ? 'white' : '#9ca3af';
    save.style.cursor = valid ? 'pointer' : 'default';
  }
}

document.addEventListener('change', e => {
  if (e.target.dataset && e.target.dataset.actActor != null) {
    state.activity.actor = e.target.value; redrawContent(); return;
  }
  if (!state.modal) return;
  if (e.target.id === 'goal-focus') { state.modal.form.focus = e.target.value; return; }
  if (state.modal.type === 'kredit') {
    if (e.target.id === 'kre-type') { state.modal.form.krType = e.target.value; renderOverlays(); return; }
    if (e.target.id === 'kre-unit') { state.modal.form.unit = e.target.value; renderOverlays(); return; }
  }
});

// Пересчитывает прогресс в редакторе KR, не перерисовывая поля ввода.
function syncKREditProgress() {
  const m5 = state.modal;
  if (!m5 || m5.type !== 'kredit') return;
  const p = calcKRProgress(m5.form);
  const pct = document.getElementById('kre-pct');
  const bar = document.getElementById('kre-bar');
  if (pct) pct.textContent = p + '%';
  if (bar) bar.innerHTML = progressBar(p, null, 5, ACCENT);
  const save = document.querySelector('.modal-footer--sticky .btn--primary');
  if (save) {
    const ok = !!m5.form.name.trim();
    save.disabled = !ok;
    save.style.background = ok ? ACCENT : '#e5e7eb';
    save.style.color = ok ? 'white' : '#9ca3af';
    if (ok) save.dataset.kreSave = '1'; else delete save.dataset.kreSave;
  }
}

document.addEventListener('input', e => {
  if (e.target.id === 'act-q') {
    state.activity.q = e.target.value;
    const sel = document.activeElement === e.target;
    redrawContent();
    if (sel) { const el = document.getElementById('act-q'); if (el) { el.focus(); el.setSelectionRange(el.value.length, el.value.length); } }
    return;
  }
  if (e.target.id === 'gt-root-q') { state.tree.rootQ = e.target.value; renderSidebar(); document.getElementById('gt-root-q')?.focus(); return; }
  if (state.modal && state.modal.type === 'kredit') {
    const f5 = state.modal.form;
    const id = e.target.id;
    if (id === 'kre-name') { f5.name = e.target.value; syncKREditProgress(); return; }
    if (id === 'kre-desc') { f5.desc = e.target.value; return; }
    if (id === 'kre-zeroing') { f5.zeroing = e.target.value; return; }
    if (id === 'kre-weight') { f5.weight = e.target.value; return; }
    if (['kre-start', 'kre-target', 'kre-current'].includes(id)) {
      f5[id.replace('kre-', '')] = e.target.value;
      syncKREditProgress(); return;
    }
    if (e.target.dataset.cpValue != null) { f5.checkpoints[+e.target.dataset.cpValue].value = e.target.value; syncKREditProgress(); return; }
    if (e.target.dataset.cpPct != null) { f5.checkpoints[+e.target.dataset.cpPct].progress_percent = e.target.value; syncKREditProgress(); return; }
    if (e.target.dataset.stName != null) { f5.stages[+e.target.dataset.stName].title = e.target.value; return; }
    if (e.target.dataset.stWeight != null) { f5.stages[+e.target.dataset.stWeight].weight = Number(e.target.value) || 0; return; }
  }
  if (state.modal && state.modal.type === 'transfer' && e.target.id === 'tr-team-q') {
    const m4 = state.modal;
    m4.teamQ = e.target.value;
    const dd = document.getElementById('tr-team-dd');
    if (dd) dd.innerHTML = transferTeamDropdownHTML(m4);
    return;
  }
  if (state.modal && state.modal.type === 'goal') {
    const m2 = state.modal;
    if (e.target.id === 'user-q') {
      m2.userQ = e.target.value; m2.userHi = 0;
      const dd = document.getElementById('user-dd');
      if (dd) dd.innerHTML = userDropdownHTML(m2.form, m2.userQ, 0);
      return;
    }
    if (e.target.id === 'team-q') {
      m2.teamQ = e.target.value; m2.teamHi = 0;
      const dd = document.getElementById('team-dd');
      if (dd) dd.innerHTML = teamDropdownHTML(m2.form, m2.teamQ, 0);
      return;
    }
    if (e.target.id === 'tr-team-q') {
      m2.teamQ = e.target.value;
      const dd = document.getElementById('tr-team-dd');
      if (dd) dd.innerHTML = transferTeamDropdownHTML(m2);
      return;
    }
    if (e.target.id === 'picker-q') {
      m2.pickerQ = e.target.value;
      const list = document.getElementById('picker-list');
      if (list) list.innerHTML = pickerListHTML(m2, pickerGroups(m2), new Set(m2.form.parents.map(p => p.id)));
      return;
    }
    if (e.target.id === 'goal-title') { state.modal.form.title = e.target.value; syncGoalModalChrome(); return; }
    if (e.target.id === 'goal-weight') { state.modal.form.weight = e.target.value; syncGoalModalChrome(); return; }
    if (e.target.id === 'goal-desc') { state.modal.form.desc = e.target.value; return; }
  }
  if (e.target.id === 'kr-comment' && state.modal) { state.modal.comment = e.target.value; return; }
  if (e.target.id === 'kr-current' && state.modal) {
    const v = parseFloat(e.target.value);
    state.modal.draft.current = isNaN(v) ? 0 : v;
    // Перерисовываем только прогресс, чтобы поле не теряло фокус и каретку.
    const { kr } = findKR(state.modal.goalId, state.modal.krId);
    const p = krProgress({ ...kr, current: state.modal.draft.current });
    const cv = state.modal.draft.current;
    const s0 = Number(kr.start || 0), t0 = Number(kr.target ?? 100);
    const pos = v => t0 === s0 ? 0 : Math.max(0, Math.min(100, (v - s0) / (t0 - s0) * 100));
    const pctEl = document.querySelector('.kr-num-line__pct');
    const fill = document.querySelector('.kr-scale__fill');
    const cur = document.querySelector('.kr-scale__cur');
    if (pctEl) pctEl.textContent = p + '%';
    if (fill) fill.style.width = pos(cv) + '%';
    if (cur) cur.style.left = pos(cv) + '%';
    const cps = (kr.checkpoints || []).filter(c => c.value !== '' && c.value != null);
    document.querySelectorAll('.kr-scale__cp').forEach((el, i) => {
      const c = cps[i]; if (!c) return;
      el.classList.toggle('kr-scale__cp--hit', t0 >= s0 ? cv >= Number(c.value) : cv <= Number(c.value));
    });
  }
  if (e.target.dataset && e.target.dataset.compose) {
    const btn = document.querySelector(`[data-send="${e.target.dataset.compose}"]`);
    if (btn) btn.className = 'comment-submit ' + (e.target.value.trim() ? 'comment-submit--active' : 'comment-submit--disabled');
  }
});

document.addEventListener('keydown', e => {
  if (e.key === 'Escape') { state.periodMenu = false; state.tenantMenu = false; state.userMenu = false; closeOverlays(); renderSidebar(); }
  if (e.key === 'Enter' && e.target.classList && e.target.classList.contains('sidebar-node__row')) e.target.click();
});

// ── Перетаскивание: порядок целей и KR ───────────────────────────────────────
// Схема из GoalCard: строка KR перетаскивается целиком, а карточка цели — только
// после захвата за ручку (иначе любой протяг по тексту начинал бы перестановку).
// Во время перетаскивания DOM не перерисовывается, иначе браузер обрывает drag;
// подсветка ставится классом напрямую.
const drag = { kr: null, goal: null };

function reorder(list, fromId, toId) {
  const from = list.findIndex(x => x.id === fromId);
  const to = list.findIndex(x => x.id === toId);
  if (from < 0 || to < 0 || from === to) return;
  const [moved] = list.splice(from, 1);
  list.splice(to, 0, moved);
}

document.addEventListener('mousedown', e => {
  const handle = e.target.closest && e.target.closest('[data-goal-handle]');
  if (!handle) return;
  const card = handle.closest('[data-goal-item]');
  if (card) card.setAttribute('draggable', 'true');
});

document.addEventListener('mouseup', () => {
  document.querySelectorAll('[data-goal-item][draggable="true"]').forEach(el => el.setAttribute('draggable', 'false'));
});

document.addEventListener('dragstart', e => {
  const krItem = e.target.closest && e.target.closest('[data-kr-item]');
  if (krItem) {
    e.stopPropagation();
    drag.kr = krItem.dataset.krItem;
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', 'kr');
    krItem.classList.add('kr-item--dragging');
    return;
  }
  const card = e.target.closest && e.target.closest('[data-goal-item]');
  if (card) {
    drag.goal = +card.dataset.goalItem;
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', 'goal');
    card.classList.add('goal-card--dragging');
  }
});

document.addEventListener('dragover', e => {
  const krItem = e.target.closest && e.target.closest('[data-kr-item]');
  if (drag.kr && krItem && krItem.dataset.krItem !== drag.kr) {
    e.preventDefault(); e.stopPropagation();
    e.dataTransfer.dropEffect = 'move';
    return;
  }
  const card = e.target.closest && e.target.closest('[data-goal-item]');
  if (drag.goal && card && +card.dataset.goalItem !== drag.goal) {
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
  }
});

document.addEventListener('drop', e => {
  const krItem = e.target.closest && e.target.closest('[data-kr-item]');
  if (drag.kr && krItem) {
    e.preventDefault(); e.stopPropagation();
    const [goalId, fromKr] = drag.kr.split(':').map(Number);
    const [, toKr] = krItem.dataset.krItem.split(':').map(Number);
    const goal = goalsOf(state.teamId).find(g => g.id === goalId);
    if (goal) reorder(goal.krs, fromKr, toKr);
    drag.kr = null; redrawContent(); return;
  }
  const card = e.target.closest && e.target.closest('[data-goal-item]');
  if (drag.goal && card) {
    e.preventDefault();
    reorder(goalsOf(state.teamId), drag.goal, +card.dataset.goalItem);
    drag.goal = null; redrawContent();
  }
});

document.addEventListener('dragend', () => {
  drag.kr = null; drag.goal = null;
  document.querySelectorAll('.kr-item--dragging').forEach(el => el.classList.remove('kr-item--dragging'));
  document.querySelectorAll('.goal-card--dragging').forEach(el => el.classList.remove('goal-card--dragging'));
  document.querySelectorAll('[data-goal-item][draggable="true"]').forEach(el => el.setAttribute('draggable', 'false'));
});

// ── Дерево целей: зум, панорамирование, выбор ───────────────────────────────
// Вписывание считается после отрисовки: размеры вьюпорта известны только тогда.
function fitGoalTree() {
  if (!state.tree.needFit) return;
  const vp = document.getElementById('gt-viewport');
  const canvas = vp && vp.querySelector('.gt-canvas');
  if (!vp || !canvas) return;
  const w = parseFloat(canvas.style.width), h = parseFloat(canvas.style.height);
  if (!w || !h) return;
  state.tree.needFit = false;
  const z = Math.max(0.15, Math.min(1, Math.min((vp.clientWidth - 32) / w, (vp.clientHeight - 32) / h)));
  if (Math.abs(z - state.tree.zoom) > 0.01) { state.tree.zoom = z; redrawContent(); }
}

const clampZoom = z => Math.max(0.15, Math.min(2.5, z));

document.addEventListener('click', e => {
  if (state.section !== 'tree') return;
  const t = e.target;
  const hit = sel => t.closest && t.closest(sel);

  if (state.section === 'activity') {
    const cat = hit('[data-act-cat]');
    if (cat) { state.activity.category = cat.dataset.actCat; redrawContent(); return; }
    if (hit('[data-act-fav]')) { state.activity.favOnly = !state.activity.favOnly; redrawContent(); return; }
    const rng = hit('[data-act-range]');
    if (rng) { state.activity.range = rng.dataset.actRange; redrawContent(); return; }
  }

  const flag = hit('[data-gt-flag]');
  if (flag) {
    const k = flag.dataset.gtFlag;
    state.tree[k] = !state.tree[k];
    state.tree.needFit = true;
    render(); fitGoalTree(); return;
  }
  const gtPer = hit('[data-gt-period]');
  if (gtPer) {
    const r = gtPer.getBoundingClientRect();
    state.tree.periodRect = { top: r.bottom + 6, left: r.left, width: r.width };
    state.tree.periodMenu = !state.tree.periodMenu;
    renderSidebar(); return;
  }
  const gtPerSet = hit('[data-gt-period-set]');
  if (gtPerSet) {
    state.periodId = +gtPerSet.dataset.gtPeriodSet;
    state.tree.periodMenu = false; state.tree.needFit = true;
    render(); fitGoalTree(); return;
  }
  const gtRoot = hit('[data-gt-root]');
  if (gtRoot) {
    const r = gtRoot.getBoundingClientRect();
    state.tree.rootRect = { top: r.bottom + 6, left: r.left };
    state.tree.rootOpen = !state.tree.rootOpen;
    renderSidebar();
    document.getElementById('gt-root-q')?.focus();
    return;
  }
  const gtRootSet = hit('[data-gt-root-set]');
  if (gtRootSet) {
    const v = gtRootSet.dataset.gtRootSet;
    state.tree.rootId = v === 'all' ? null : +v;
    state.tree.rootOpen = false; state.tree.rootQ = ''; state.tree.needFit = true;
    render(); fitGoalTree(); return;
  }
  if (state.tree.rootOpen && !hit('.gt-rootpick')) { state.tree.rootOpen = false; renderSidebar(); }
  if (state.tree.periodMenu && !hit('.gt-psel')) { state.tree.periodMenu = false; renderSidebar(); }

  const zoomBtn = hit('[data-gt-zoom]');
  if (zoomBtn) {
    const k = zoomBtn.dataset.gtZoom;
    if (k === 'in') state.tree.zoom = clampZoom(state.tree.zoom * 1.2);
    else if (k === 'out') state.tree.zoom = clampZoom(state.tree.zoom / 1.2);
    else if (k === 'reset') state.tree.zoom = 1;
    else { state.tree.needFit = true; redrawContent(); fitGoalTree(); return; }
    redrawContent(); return;
  }
  const collapse = hit('[data-gt-collapse]');
  if (collapse) {
    e.stopPropagation();
    const id = +collapse.dataset.gtCollapse;
    if (state.tree.collapsed.has(id)) state.tree.collapsed.delete(id); else state.tree.collapsed.add(id);
    redrawContent(); return;
  }
  if (hit('[data-gt-close]')) { state.tree.selectedId = null; state.tree.focused = false; redrawContent(); return; }
  if (hit('[data-gt-focus]')) { state.tree.focused = !state.tree.focused; redrawContent(); return; }
  if (hit('[data-gt-reset]')) {
    state.tree.selectedId = null; state.tree.focused = false;
    state.tree.onlyRoots = false; state.tree.collapsed = new Set(); state.tree.rootId = null;
    state.tree.needFit = true; redrawContent(); fitGoalTree(); return;
  }
  const card = hit('[data-gt-card]');
  if (card) { state.tree.selectedId = +card.dataset.gtCard; redrawContent(); return; }
  // Клик по пустому фону снимает выделение — но не после протяжки.
  if (hit('#gt-viewport') && !treePan.moved) { state.tree.selectedId = null; state.tree.focused = false; redrawContent(); }
});

document.addEventListener('change', e => {
  if (state.section === 'tree' && e.target.dataset.gtRoots != null) {
    state.tree.onlyRoots = e.target.checked;
    state.tree.needFit = true;
    redrawContent(); fitGoalTree();
  }
});

// Панорамирование по пустому фону — как в GoalTreeCanvas.
const treePan = { active: null, moved: false };

document.addEventListener('pointerdown', e => {
  if (state.section !== 'tree' || e.button !== 0) return;
  const vp = e.target.closest && e.target.closest('#gt-viewport');
  if (!vp || e.target.closest('.gt-card') || e.target.closest('.gt-zoom-ctrl')) return;
  treePan.moved = false;
  treePan.active = { x: e.clientX, y: e.clientY, sl: vp.scrollLeft, st: vp.scrollTop, vp };
  vp.classList.add('gt-viewport--panning');
});

document.addEventListener('pointermove', e => {
  const p = treePan.active;
  if (!p) return;
  if (Math.abs(e.clientX - p.x) + Math.abs(e.clientY - p.y) > 4) treePan.moved = true;
  p.vp.scrollLeft = p.sl - (e.clientX - p.x);
  p.vp.scrollTop = p.st - (e.clientY - p.y);
});

document.addEventListener('pointerup', () => {
  if (treePan.active) treePan.active.vp.classList.remove('gt-viewport--panning');
  treePan.active = null;
  setTimeout(() => { treePan.moved = false; }, 0);
});

// Зум колесом к точке под курсором; listener неpassive, иначе страница проскроллится.
document.addEventListener('wheel', e => {
  if (state.section !== 'tree') return;
  const vp = e.target.closest && e.target.closest('#gt-viewport');
  if (!vp) return;
  e.preventDefault();
  const rect = vp.getBoundingClientRect();
  const ox = e.clientX - rect.left, oy = e.clientY - rect.top;
  const z = state.tree.zoom;
  const nz = clampZoom(z * Math.exp(-e.deltaY * 0.0015));
  const cx = (ox + vp.scrollLeft) / z, cy = (oy + vp.scrollTop) / z;
  state.tree.zoom = nz;
  redrawContent();
  const nvp = document.getElementById('gt-viewport');
  if (nvp) { nvp.scrollLeft = cx * nz - ox; nvp.scrollTop = cy * nz - oy; }
}, { passive: false });

render();

// Всплывающая подсказка для «?» (.info-hint): читает data-hint или title.
(() => {
  let tip = null, cur = null;
  const show = el => {
    const html = el.dataset.hintHtml;
    const text = el.dataset.hint || el.getAttribute('title');
    if (!html && !text) return;
    if (el.hasAttribute('title')) { el.dataset.hint = text; el.removeAttribute('title'); }
    if (!tip) { tip = document.createElement('div'); tip.className = 'info-hint__tooltip'; tip.setAttribute('role', 'tooltip'); document.body.appendChild(tip); }
    if (html) tip.innerHTML = html; else tip.textContent = text;
    tip.style.display = 'block'; tip.style.transform = 'none'; tip.style.whiteSpace = 'pre-line'; tip.style.maxWidth = '300px';
    const r = el.getBoundingClientRect(), tw = tip.offsetWidth, th = tip.offsetHeight;
    let left = Math.min(window.innerWidth - tw - 8, Math.max(8, r.left + r.width / 2 - tw / 2));
    let top = r.top - th - 8; if (top < 8) top = r.bottom + 8;
    tip.style.left = left + 'px'; tip.style.top = top + 'px'; cur = el;
  };
  const hide = () => { if (tip) tip.style.display = 'none'; cur = null; };
  document.addEventListener('mouseover', e => { const el = e.target.closest && e.target.closest('.info-hint'); if (el && el !== cur) show(el); else if (!el && cur) hide(); });
  document.addEventListener('focusin', e => { const el = e.target.closest && e.target.closest('.info-hint'); if (el) show(el); });
  document.addEventListener('focusout', e => { if (e.target.closest && e.target.closest('.info-hint')) hide(); });
  document.addEventListener('scroll', hide, true);
})();

// Схлопывание длинных описаний целей/KR до 2 строк (высота — --md-collapse-max в CSS).
const descExpanded = new Set();
function enhanceDescs() {
  document.querySelectorAll('[data-desc-key]').forEach(el => {
    if (el.parentElement && el.parentElement.classList.contains('md-collapsible')) return;
    const key = el.dataset.descKey;
    const wrap = document.createElement('div');
    wrap.className = 'md-collapsible';
    el.parentNode.insertBefore(wrap, el);
    wrap.appendChild(el);
    el.classList.add('md-collapsible__body');
    if (el.scrollHeight <= el.clientHeight + 2) return;
    const open = descExpanded.has(key);
    wrap.classList.add('md-collapsible--clipped');
    if (open) wrap.classList.add('md-collapsible--expanded');
    const btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'md-collapsible__toggle';
    btn.setAttribute('data-no-drag', '');
    const paint = o => { btn.innerHTML = `<span class="md-collapsible__label">${o ? 'Свернуть' : 'Показать полностью'}</span><span class="md-collapsible__caret">${o ? '▴' : '▾'}</span>`; btn.setAttribute('aria-expanded', o); };
    paint(open);
    btn.addEventListener('click', e => {
      e.stopPropagation();
      const o = !wrap.classList.contains('md-collapsible--expanded');
      wrap.classList.toggle('md-collapsible--expanded', o);
      o ? descExpanded.add(key) : descExpanded.delete(key);
      paint(o);
    });
    wrap.appendChild(btn);
  });
}
enhanceDescs();
new MutationObserver(() => enhanceDescs()).observe(document.body, { childList: true, subtree: true });
