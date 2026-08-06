// Общий тёмный сайдбар навигации. Грузится как text/babel ПЕРЕД app-скриптами
// (tracker.js, stub.js, settings.js, admin.js), экспортируя глобальные компоненты
// Sidebar / SidebarTenant / SidebarSections / SidebarFooter / SidebarBell —
// единый источник правды переиспользуемой навигации. Стили — sidebar.css.
//
// Самодостаточен: рендерит аватары инлайн, читает CSRF из cookie, logout через
// fetch, сам тянет /api/v1/config (футер) и /api/v1/session/tenants (шапка).
//
// Использует React.useState/useRef (не top-level деструктуризацию), чтобы не
// конфликтовать с app-скриптами, делящими ту же глобальную область.

function _sbCSRF() {
  const m = document.cookie.match(/(?:^|;\s*)okr_csrf_token=([^;]*)/);
  return m ? decodeURIComponent(m[1]) : '';
}

// Feedback nudge cookies. ~2-летний срок, site-wide path (перенесено из header.js).
function _sbFbGet(name) {
  const m = document.cookie.match(new RegExp('(?:^|;\\s*)' + name + '=([^;]*)'));
  return m ? decodeURIComponent(m[1]) : '';
}
function _sbFbSet(name, val) {
  document.cookie = name + '=' + encodeURIComponent(val) + ';path=/;max-age=' + (2 * 365 * 24 * 60 * 60) + ';SameSite=Lax';
}

// Инициалы пользователя: первые буквы двух слов, иначе одна буква.
function _sbUserInitials(name) {
  return (name || 'Пользователь').split(' ').slice(0, 2).map(w => w[0] || '').join('').toUpperCase() || '?';
}

// Инициалы тенанта: первые буквы двух слов; для одного слова — первые 2 буквы.
function _sbTenantInitials(name) {
  const words = (name || '').trim().split(/\s+/).filter(Boolean);
  if (words.length >= 2) return (words[0][0] + words[1][0]).toUpperCase();
  const w = words[0] || 'OK';
  return w.slice(0, 2).toUpperCase();
}

// Круглый аватар пользователя (инлайн-стили, без зависимости от .avatar).
function SidebarAvatar({ user, size }) {
  const name = user?.display_name || 'Пользователь';
  const base = { width: size, height: size, borderRadius: '50%', flexShrink: 0, display: 'block' };
  if (user?.avatar_url) {
    return <img src={user.avatar_url} width={size} height={size} alt="" style={{ ...base, objectFit: 'cover' }} />;
  }
  const colors = ['#2563eb', '#7c3aed', '#059669', '#d97706', '#dc2626', '#0891b2', '#be185d', '#6366f1'];
  const bg = colors[(name.charCodeAt(0) || 0) % colors.length];
  return (
    <div style={{ ...base, display: 'inline-flex', alignItems: 'center', justifyContent: 'center', color: 'white', fontWeight: 700, fontSize: Math.round(size * 0.38), background: bg }}>{_sbUserInitials(name)}</div>
  );
}

// SidebarBell — колокольчик с бейджем. Рендерится хостом в слот bell.
function SidebarBell({ count, onClick }) {
  return (
    <button className="sidebar__bell" onClick={onClick} aria-label="Health Check-in">
      <span className="sidebar__bell-icon">🔔</span>
      <span className={`sidebar__bell-badge${count === 0 ? ' sidebar__bell-badge--zero' : ''}`}>{count}</span>
    </button>
  );
}

// SidebarTenant — шапка тенанта + переключатель организаций (если тенантов > 1).
function SidebarTenant({ user, bell }) {
  const [tenants, setTenants] = React.useState([]);
  const [open, setOpen] = React.useState(false);
  React.useEffect(() => {
    fetch('/api/v1/session/tenants', { credentials: 'include' })
      .then(r => (r.ok ? r.json() : null))
      .then(list => { if (Array.isArray(list)) setTenants(list); })
      .catch(() => {});
  }, []);
  const active = tenants.find(t => t.active) || null;
  const name = active ? (active.name || active.slug) : 'OKR Tracker';
  const canSwitch = tenants.length > 1;
  const switchTo = (id) => {
    fetch('/api/v1/session/tenant', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': _sbCSRF() },
      body: JSON.stringify({ tenant_id: id }),
    }).then(r => { if (r.ok) location.reload(); });
  };
  React.useEffect(() => {
    if (!open) return;
    const onKey = e => { if (e.key === 'Escape') setOpen(false); };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [open]);
  return (
    <div className="sidebar__tenant">
      <div className="sidebar__tenant-avatar">{_sbTenantInitials(name)}</div>
      <button
        className="sidebar__tenant-main"
        disabled={!canSwitch}
        onClick={() => canSwitch && setOpen(o => !o)}
        aria-label="Текущая организация"
      >
        <span className="sidebar__tenant-name">{name}</span>
        {canSwitch && <span className="sidebar__tenant-chevron">▾</span>}
      </button>
      {bell}
      {open && canSwitch && (
        <div className="sidebar__tenant-menu" onMouseLeave={() => setOpen(false)}>
          {tenants.map(t => (
            <button
              key={t.id}
              className={`sidebar__tenant-item${t.active ? ' sidebar__tenant-item--active' : ''}`}
              onClick={() => { t.active ? setOpen(false) : switchTo(t.id); }}
            >
              <span className="sidebar__tenant-item-icon">{t.active ? '✓' : '🏢'}</span>{t.name || t.slug}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

// SidebarSections — глобальные РАЗДЕЛЫ (одинаковы на всех страницах).
const SIDEBAR_SECTIONS = [
  { id: 'tracker',      label: 'Цели команды',    href: '/',             icon: '🎯' },
  { id: 'goal-tree',    label: 'Дерево целей',    href: '/goal-tree',    icon: '🕸' },
  // Обзор периода и лог активностей доступны только tenant-admin (совпадает с серверным гейтом) — скрыты для остальных.
  { id: 'period-overview', label: 'Обзор периода', href: '/period-overview', icon: '📊' },
  { id: 'activity-log', label: 'Лог активностей', href: '/activity-log', icon: '🕑', adminOnly: true },
];
// linkParams optionally appends a query string per section id (e.g. carrying the current
// period from the tracker to the activity log so it opens for the same period).
// isAdmin скрывает admin-only разделы у обычных пользователей.
function SidebarSections({ active, linkParams, isAdmin }) {
  return (
    <div className="sidebar__sections">
      <div className="sidebar__section-label">Разделы</div>
      {SIDEBAR_SECTIONS.filter(s => !s.adminOnly || isAdmin).map(s => (
        <a key={s.id} href={s.href + ((linkParams && linkParams[s.id]) || '')} className={`sidebar__navlink${s.id === active ? ' sidebar__navlink--active' : ''}`}>
          <span className="sidebar__navlink-icon">{s.icon}</span>{s.label}
        </a>
      ))}
    </div>
  );
}

// SidebarFooter — ссылки Документация/Обратная связь + блок пользователя.
// cfg (/api/v1/config: docUrl, feedback_url, is_admin, …) приходит из Sidebar; рендерит FeedbackNudge.
function SidebarFooter({ user, cfg }) {
  const [menuOpen, setMenuOpen] = React.useState(false);
  React.useEffect(() => {
    if (!menuOpen) return;
    const onKey = e => { if (e.key === 'Escape') setMenuOpen(false); };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [menuOpen]);
  const docUrl = (cfg && cfg.documentation_url) || '';
  const name = user?.display_name || 'Пользователь';
  const logout = () => fetch('/logout', { method: 'POST', credentials: 'include', headers: { 'X-CSRF-Token': _sbCSRF() } }).then(() => location.href = '/login');
  return (
    <div className="sidebar__footer">
      {docUrl && (
        <a href={docUrl} target="_blank" rel="noopener noreferrer" className="sidebar__footer-link">
          <span className="sidebar__footer-icon">📖</span>Документация
        </a>
      )}
      {cfg && cfg.feedback_menu_link_enabled && cfg.feedback_url && (
        <a href={cfg.feedback_url} target="_blank" rel="noopener noreferrer" className="sidebar__footer-link">
          <span className="sidebar__footer-icon">☆</span>Обратная связь
        </a>
      )}
      <div className="sidebar__user">
        <SidebarAvatar user={user} size={36} />
        <div className="sidebar__user-info">
          <div className="sidebar__user-name">{name}</div>
          <div className="sidebar__user-sub">{user?.email || user?.provider || ''}</div>
        </div>
        <button className="sidebar__user-more" onClick={() => setMenuOpen(o => !o)} aria-label="Меню пользователя">···</button>
        {menuOpen && (
          <div className="sidebar__user-menu" onMouseLeave={() => setMenuOpen(false)}>
            <a href="/settings" className="sidebar__user-menu-item"><span className="sidebar__user-menu-icon">⚙</span>Настройки</a>
            {cfg && cfg.is_admin && (
              <a href="/admin" className="sidebar__user-menu-item"><span className="sidebar__user-menu-icon">🛠</span>Администрирование</a>
            )}
            {cfg && cfg.is_system_admin && (
              <a href="/system" className="sidebar__user-menu-item"><span className="sidebar__user-menu-icon">🖥</span>System</a>
            )}
            <button onClick={logout} className="sidebar__user-menu-item sidebar__user-menu-item--danger">
              <span className="sidebar__user-menu-icon">↩</span>Выйти
            </button>
          </div>
        )}
      </div>
      <FeedbackNudge cfg={cfg} />
    </div>
  );
}

// Sidebar — контейнер. children = контекстная навигация страницы (со своим
// flex:1 скролл-регионом). beforeSections — вставка между шапкой и разделами
// (на трекере — блок выбора периода).
function Sidebar({ user, active, bell, beforeSections, showSections = true, linkParams, children }) {
  // Единый источник конфига для сайдбара: разделы (activity-log под is_admin) и футер.
  const [cfg, setCfg] = React.useState(null);
  React.useEffect(() => {
    fetch('/api/v1/config', { credentials: 'include' })
      .then(r => (r.ok ? r.json() : null))
      .then(c => { if (c) setCfg(c); })
      .catch(() => {});
  }, []);
  return (
    <div className="sidebar">
      <SidebarTenant user={user} bell={bell} />
      {beforeSections}
      {showSections !== false && <SidebarSections active={active} linkParams={linkParams} isAdmin={!!(cfg && cfg.is_admin)} />}
      {children}
      <SidebarFooter user={user} cfg={cfg} />
    </div>
  );
}

// FeedbackNudge — модальное окно-просьба оставить обратную связь (перенесено из
// header.js без изменений). Логика показа на cookies.
function FeedbackNudge({ cfg }) {
  const [show, setShow] = React.useState(false);

  React.useEffect(() => {
    if (!cfg) return;
    const DAY = 86400000;
    const freqMs = (cfg.feedback_frequency_days || 30) * DAY;
    const now = Date.now();

    let start = parseInt(_sbFbGet('okr_fb_start'), 10);
    const seen = parseInt(_sbFbGet('okr_fb_seen'), 10);
    if (!start || !seen || now - seen > freqMs) {
      start = now;
      _sbFbSet('okr_fb_start', String(now));
    }
    _sbFbSet('okr_fb_seen', String(now));

    if (!cfg.feedback_popup_enabled || !cfg.feedback_url) return;
    const graceOK = now - start >= 2 * DAY;
    const dismissed = parseInt(_sbFbGet('okr_fb_dismissed'), 10);
    const cooldownOK = !dismissed || (now - dismissed >= freqMs);
    if (graceOK && cooldownOK) setShow(true);
  }, [cfg]);

  function dismiss() {
    _sbFbSet('okr_fb_dismissed', String(Date.now()));
    setShow(false);
  }

  React.useEffect(() => {
    if (!show) return;
    const onKey = e => { if (e.key === 'Escape') dismiss(); };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [show]);

  if (!show || !cfg) return null;
  return (
    <div className="fb-nudge__overlay" onClick={dismiss}>
      <div className="fb-nudge__card" onClick={e => e.stopPropagation()} role="dialog" aria-modal="true" aria-label="Обратная связь">
        <button onClick={dismiss} className="fb-nudge__close" aria-label="Закрыть">✕</button>
        <div className="fb-nudge__icon">💬</div>
        <div className="fb-nudge__title">Поделитесь обратной связью</div>
        <div className="fb-nudge__text">Помогите сделать инструмент лучше — это займёт пару минут.</div>
        <a href={cfg.feedback_url} target="_blank" rel="noopener noreferrer" className="fb-nudge__btn" onClick={dismiss}>
          Поделиться обратной связью
        </a>
      </div>
    </div>
  );
}
