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

// Feedback nudge tracking cookies. ~2-year lifetime, site-wide path.
function _fbGet(name) {
  const m = document.cookie.match(new RegExp('(?:^|;\\s*)' + name + '=([^;]*)'));
  return m ? decodeURIComponent(m[1]) : '';
}
function _fbSet(name, val) {
  document.cookie = name + '=' + encodeURIComponent(val) + ';path=/;max-age=' + (2 * 365 * 24 * 60 * 60) + ';SameSite=Lax';
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
  const [cfg, setCfg] = React.useState(null);
  React.useEffect(() => {
    fetch('/api/v1/config', { credentials: 'include' })
      .then(r => (r.ok ? r.json() : null))
      .then(c => { if (c) setCfg(c); })
      .catch(() => {});
  }, []);
  const docUrl = (cfg && cfg.documentation_url) || '';
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
      <FeedbackNudge cfg={cfg} />
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
              {cfg && cfg.feedback_menu_link_enabled && cfg.feedback_url && (
                <a href={cfg.feedback_url} target="_blank" rel="noopener noreferrer" className="nav-menu__item">
                  <span className="nav-menu__item-icon">💬</span>Обратная связь
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

// FeedbackNudge — модальное окно-просьба оставить обратную связь. Логика показа
// на cookies: первый показ не раньше чем через 2 суток с начала вовлечения,
// повторный — не раньше чем через feedback_frequency_days после закрытия.
// Долгий перерыв (визитов не было дольше частоты) сбрасывает 2-дневный grace.
function FeedbackNudge({ cfg }) {
  const [show, setShow] = React.useState(false);

  React.useEffect(() => {
    if (!cfg) return;
    const DAY = 86400000;
    const freqMs = (cfg.feedback_frequency_days || 30) * DAY;
    const now = Date.now();

    // Engagement tracking — runs on every page load, even when the popup is off.
    let start = parseInt(_fbGet('okr_fb_start'), 10);
    const seen = parseInt(_fbGet('okr_fb_seen'), 10);
    if (!start || !seen || now - seen > freqMs) {
      start = now;               // first visit, or return after a long break
      _fbSet('okr_fb_start', String(now));
    }
    _fbSet('okr_fb_seen', String(now));

    if (!cfg.feedback_popup_enabled || !cfg.feedback_url) return;
    const graceOK = now - start >= 2 * DAY;
    const dismissed = parseInt(_fbGet('okr_fb_dismissed'), 10);
    const cooldownOK = !dismissed || (now - dismissed >= freqMs);
    if (graceOK && cooldownOK) setShow(true);
  }, [cfg]);

  function dismiss() {
    _fbSet('okr_fb_dismissed', String(Date.now()));
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
