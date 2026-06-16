// Shared header navigation. Loaded as text/babel BEFORE the app JS
// (tracker.js, admin.js, settings.js, stub.js), so the global HeaderNavMenu is
// available to every React app — a single source of truth for the hamburger menu.
//
// Self-contained on purpose: renders its own avatar (inline styles, no .avatar
// class dependency), reads CSRF from the cookie, logs out via fetch, and fetches
// /api/v1/config for the documentation link. Styles come from header.css
// (.nav-menu*). Depends only on the React global.
//
// Uses React.useState/useRef (not a top-level `const { useState } = React`) to
// avoid clashing with the same destructure already declared in the app scripts
// that share this global lexical scope.

function _hdrCSRF() {
  const m = document.cookie.match(/(?:^|;\s*)okr_csrf_token=([^;]*)/);
  return m ? decodeURIComponent(m[1]) : '';
}

function HeaderAvatar({ user, size }) {
  const name = user?.display_name || 'Пользователь';
  const base = { width: size, height: size, borderRadius: '50%', flexShrink: 0, display: 'block' };
  if (user?.avatar_url) {
    return <img src={user.avatar_url} width={size} height={size} alt="" style={{ ...base, objectFit: 'cover' }} />;
  }
  const initials = name.split(' ').slice(0, 2).map(w => w[0] || '').join('').toUpperCase() || '?';
  const colors = ['#2563eb', '#7c3aed', '#059669', '#d97706', '#dc2626', '#0891b2', '#be185d', '#6366f1'];
  const bg = colors[(name.charCodeAt(0) || 0) % colors.length];
  return (
    <div style={{ ...base, display: 'inline-flex', alignItems: 'center', justifyContent: 'center', color: 'white', fontWeight: 700, fontSize: Math.round(size * 0.38), background: bg }}>{initials}</div>
  );
}

// HeaderNavMenu — глобальное гамбургер-меню. Единый источник правды для
// навигации/аккаунта в шапке всех страниц (трекер, настройки, админка, заглушки).
// Рендерит только кнопку ☰ и выезжающий слева drawer. Сам тянет /api/v1/config
// для documentation_url, чтобы пункт «Документация» вёл себя одинаково везде.
// active: 'tracker' | 'activity-log' | 'goal-tree' | null.
function HeaderNavMenu({ user, active }) {
  const [open, setOpen] = React.useState(false);
  const [docUrl, setDocUrl] = React.useState('');
  React.useEffect(() => {
    fetch('/api/v1/config', { credentials: 'include' })
      .then(r => (r.ok ? r.json() : null))
      .then(cfg => { if (cfg && cfg.documentation_url) setDocUrl(cfg.documentation_url); })
      .catch(() => {});
  }, []);
  React.useEffect(() => {
    if (!open) return;
    const onKey = e => { if (e.key === 'Escape') setOpen(false); };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [open]);
  const logout = () => fetch('/logout', { method: 'POST', credentials: 'include', headers: { 'X-CSRF-Token': _hdrCSRF() } }).then(() => location.href = '/login');
  const name = user?.display_name || 'Пользователь';
  const sections = [
    { id: 'tracker',      label: 'Цели команд',     href: '/',            icon: '🎯' },
    { id: 'activity-log', label: 'Лог активностей', href: '/activity-log', icon: '🕑' },
    { id: 'goal-tree',    label: 'Дерево целей',    href: '/goal-tree',    icon: '🕸' },
  ];
  return (
    <React.Fragment>
      <button onClick={() => setOpen(true)} className="nav-menu__burger" aria-label="Меню">☰</button>
      {open && (
        <div className="nav-menu__overlay" onClick={() => setOpen(false)}>
          <div className="nav-menu__panel" onClick={e => e.stopPropagation()}>
            <div className="nav-menu__head">
              <span className="nav-menu__head-logo">🎯 OKR Tracker</span>
              <button onClick={() => setOpen(false)} className="nav-menu__close" aria-label="Закрыть">✕</button>
            </div>
            <div className="nav-menu__sections">
              <div className="nav-menu__label">Разделы</div>
              {sections.map(s => (
                <a key={s.id} href={s.href} className={`nav-menu__item${s.id === active ? ' nav-menu__item--active' : ''}`}>
                  <span className="nav-menu__item-icon">{s.icon}</span>{s.label}
                </a>
              ))}
            </div>
            <div className="nav-menu__foot">
              <div className="nav-menu__profile">
                <HeaderAvatar user={user} size={40} />
                <div className="nav-menu__profile-info">
                  <div className="nav-menu__profile-name">{name}</div>
                  <div className="nav-menu__profile-sub">{user?.email || user?.provider || ''}</div>
                </div>
                <a href="/settings" className="nav-menu__gear" aria-label="Настройки">⚙</a>
              </div>
              <div className="nav-menu__label">Аккаунт</div>
              {docUrl && (
                <a href={docUrl} target="_blank" rel="noopener noreferrer" className="nav-menu__item">
                  <span className="nav-menu__item-icon">📖</span>Документация
                </a>
              )}
              {user?.is_admin && (
                <a href="/admin" className="nav-menu__item"><span className="nav-menu__item-icon">🛠</span>Администрирование</a>
              )}
              <button onClick={logout} className="nav-menu__item nav-menu__item--danger">
                <span className="nav-menu__item-icon">↩</span>Выйти
              </button>
            </div>
          </div>
        </div>
      )}
    </React.Fragment>
  );
}
