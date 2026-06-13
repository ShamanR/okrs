// Shared header account menu. Loaded as text/babel BEFORE the app JS
// (tracker.js, admin.js, settings.js), so the global HeaderUserMenu is available
// to all three React apps — a single source of truth for the avatar menu.
//
// Self-contained on purpose: renders its own avatar (inline styles, no .avatar
// class dependency), reads CSRF from the cookie, and logs out via fetch. Styles
// come from header.css (.user-menu*). Depends only on the React global.
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

function HeaderUserMenu({ user, docUrl, showTrackerLink = true }) {
  const [open, setOpen] = React.useState(false);
  const timer = React.useRef();
  const show = () => { clearTimeout(timer.current); setOpen(true); };
  const hide = () => { clearTimeout(timer.current); timer.current = setTimeout(() => setOpen(false), 150); };
  const logout = () => fetch('/logout', { method: 'POST', credentials: 'include', headers: { 'X-CSRF-Token': _hdrCSRF() } }).then(() => location.href = '/login');
  const name = user?.display_name || 'Пользователь';
  return (
    <div className="user-menu" onMouseEnter={show} onMouseLeave={hide}>
      <button onClick={() => setOpen(o => !o)} className={`user-menu__trigger${open ? ' user-menu__trigger--open' : ''}`}>
        <HeaderAvatar user={user} size={28} />
        <span className={`user-menu__chevron${open ? ' user-menu__chevron--open' : ''}`}>▾</span>
      </button>
      {open && (
        <div onMouseEnter={show} onMouseLeave={hide} className="user-menu__dropdown">
          <div className="user-menu__profile">
            <HeaderAvatar user={user} size={36} />
            <div>
              <div className="user-menu__name">{name}</div>
              <div className="user-menu__email">{user?.email || user?.provider || ''}</div>
            </div>
          </div>
          {showTrackerLink && (
            <a href="/" className="user-menu__item"><span className="user-menu__item-icon">←</span>OKR Tracker</a>
          )}
          {docUrl && (
            <a href={docUrl} target="_blank" rel="noopener noreferrer" className="user-menu__item">
              <span className="user-menu__item-icon">📖</span>Документация
            </a>
          )}
          <a href="/settings" className="user-menu__item"><span className="user-menu__item-icon">⚙</span>Настройки</a>
          {user?.is_admin && (
            <a href="/admin" className="user-menu__item"><span className="user-menu__item-icon">🛠</span>Администрирование</a>
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
