# Рефакторинг сайдбара — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Убрать глобальное гамбургер-меню и развернуть навигацию прямо в тёмный сайдбар — единый переиспользуемый компонент на страницах трекера, заглушек, настроек и админки.

**Architecture:** Новый общий модуль `internal/web/static/sidebar.js` (+ `sidebar.css`), подключаемый как `text/babel` перед app-скриптами каждого shell (по образцу текущего `header.js`). Экспортирует в глобальную область компоненты `Sidebar`, `SidebarTenant`, `SidebarSections`, `SidebarFooter`, `SidebarBell` и перенесённый `FeedbackNudge`. Каждый app-скрипт (`tracker.js`, `stub.js`, `settings.js`, `admin.js`) заменяет свой сайдбар на общий `Sidebar`, передавая контекстную часть через `children`.

**Tech Stack:** React 18 через `@babel/standalone` (JSX прямо в браузере, без сборки), Go `html/template` + `//go:embed` для shell-шаблонов, статика отдаётся с диска (`http.Dir("internal/web/static")`).

## Global Constraints

- **Не делать `git commit`** — пользователь коммитит сам (CLAUDE.md #8). Каждая задача заканчивается ручной/визуальной проверкой, а не коммитом.
- **Специфика окружения статики:** файлы `internal/web/static/*.js` и `*.css` отдаются с диска — правки видны после обновления страницы в браузере, пересборка не нужна. Файлы `internal/http/templates/*.html` встроены через `//go:embed` — их изменения требуют перезапуска сервера (`go run ./cmd/server`).
- **Общие модули грузятся как `text/babel` ПЕРЕД app-скриптом** и экспортируют глобальные компоненты (см. `specs/010-architecture-constraints.md`). Не использовать `import/export` — только глобальные `function`.
- **Не использовать `React.useState` через top-level `const { useState } = React`** в `sidebar.js`: писать `React.useState`/`React.useEffect`/`React.useRef`, чтобы не конфликтовать с деструктуризацией в app-скриптах, делящих ту же глобальную область (как это сделано в `header.js`).
- **Язык интерфейса — русский.** Точные подписи: «Разделы», «Цели команды», «Дерево целей», «Лог активностей», «Документация», «Обратная связь», «Настройки», «Администрирование», «Выйти».
- **Тёмная палитра сайдбара** (из `header.css`/`tracker.css`): фон `#0c1220`, границы `rgba(255,255,255,0.06)`, текст `#cbd5e1`/`#f1f5f9`, приглушённый `#64748b`/`#94a3b8`, акцент фиолетовый `#7c3aed`/`#a78bfa`, активный фон `rgba(124,58,237,0.18)`.
- **Запуск для проверки:** сервер поднимается `go run ./cmd/server` (нужны `DATABASE_URL`, `TZ`, `PORT=8080`, см. README). Демо-данные — `seed_demo.sql`. Все страницы открываются на `http://localhost:8080`.
- **Backend не меняем** (кроме shell-шаблонов и одного теста): существующие `go test ./...`, `go vet ./...`, `go build ./...` должны оставаться зелёными.

---

### Task 1: Общий модуль `sidebar.js` + `sidebar.css` и интеграция в трекер

**Files:**
- Create: `internal/web/static/sidebar.js`
- Create: `internal/web/static/sidebar.css`
- Modify: `internal/http/templates/tracker_shell.html` (подключить sidebar.js/sidebar.css)
- Modify: `internal/web/static/tracker.js:2233-2271` (заменить сайдбар на `<Sidebar>`)

**Interfaces:**
- Produces (глобальные компоненты из `sidebar.js`):
  - `Sidebar({ user, active, bell, beforeSections, children })` — контейнер тёмного сайдбара. Порядок: `SidebarTenant` → `beforeSections` → `SidebarSections` → `children` → `SidebarFooter`.
  - `SidebarTenant({ user, bell })` — шапка тенанта (аватар-инициалы + название + шеврон-переключатель) + слот `bell` справа. Сам тянет `GET /api/v1/session/tenants`.
  - `SidebarSections({ active })` — глобальные РАЗДЕЛЫ; `active ∈ 'tracker' | 'goal-tree' | 'activity-log' | null`.
  - `SidebarFooter({ user })` — ссылки Документация/Обратная связь + блок пользователя с меню «···». Сам тянет `GET /api/v1/config`.
  - `SidebarBell({ count, onClick })` — кнопка-колокольчик с бейджем.
  - `FeedbackNudge({ cfg })` — перенесён из `header.js` (без изменений логики).
- Consumes: `user` — объект из `GET /api/v1/me` (`display_name`, `email`, `provider`, `avatar_url`). `cfg` — из `GET /api/v1/config` (`documentation_url`, `feedback_url`, `feedback_menu_link_enabled`, `is_admin`).

- [ ] **Step 1: Создать `internal/web/static/sidebar.js`**

```jsx
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
  { id: 'activity-log', label: 'Лог активностей', href: '/activity-log', icon: '🕑' },
];
function SidebarSections({ active }) {
  return (
    <div className="sidebar__sections">
      <div className="sidebar__section-label">Разделы</div>
      {SIDEBAR_SECTIONS.map(s => (
        <a key={s.id} href={s.href} className={`sidebar__navlink${s.id === active ? ' sidebar__navlink--active' : ''}`}>
          <span className="sidebar__navlink-icon">{s.icon}</span>{s.label}
        </a>
      ))}
    </div>
  );
}

// SidebarFooter — ссылки Документация/Обратная связь + блок пользователя.
// Сам тянет /api/v1/config (для docUrl, feedback_url, is_admin) и рендерит FeedbackNudge.
function SidebarFooter({ user }) {
  const [cfg, setCfg] = React.useState(null);
  const [menuOpen, setMenuOpen] = React.useState(false);
  React.useEffect(() => {
    fetch('/api/v1/config', { credentials: 'include' })
      .then(r => (r.ok ? r.json() : null))
      .then(c => { if (c) setCfg(c); })
      .catch(() => {});
  }, []);
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
function Sidebar({ user, active, bell, beforeSections, children }) {
  return (
    <div className="sidebar">
      <SidebarTenant user={user} bell={bell} />
      {beforeSections}
      <SidebarSections active={active} />
      {children}
      <SidebarFooter user={user} />
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
```

- [ ] **Step 2: Создать `internal/web/static/sidebar.css`**

```css
/* Общий тёмный сайдбар навигации. Подключается во всех SPA-shell (tracker,
   stub, settings, admin) вместе с sidebar.js. Самодостаточен: не зависит от
   переменных app-специфичного CSS. Тёмная палитра совпадает с трекером. */

.sidebar { width: 252px; background: #0c1220; display: flex; flex-direction: column; flex-shrink: 0; height: 100%; overflow: hidden; }

/* ── Шапка тенанта ── */
.sidebar__tenant { position: relative; display: flex; align-items: center; gap: 10px; padding: 12px 12px; border-bottom: 1px solid rgba(255,255,255,0.06); }
.sidebar__tenant-avatar { width: 34px; height: 34px; border-radius: 8px; background: #16a34a; color: #fff; font-weight: 800; font-size: 13px; letter-spacing: -0.3px; display: inline-flex; align-items: center; justify-content: center; flex-shrink: 0; }
.sidebar__tenant-main { flex: 1; min-width: 0; display: flex; align-items: center; gap: 6px; background: none; border: none; color: #fff; cursor: pointer; font-family: inherit; padding: 0; text-align: left; }
.sidebar__tenant-main:disabled { cursor: default; }
.sidebar__tenant-name { flex: 1; min-width: 0; font-size: 15px; font-weight: 800; letter-spacing: -0.3px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.sidebar__tenant-chevron { color: #64748b; font-size: 11px; flex-shrink: 0; }
.sidebar__tenant-menu { position: absolute; top: calc(100% - 2px); left: 12px; right: 12px; z-index: 50; background: #131b2e; border: 1px solid rgba(255,255,255,0.1); border-radius: 10px; box-shadow: 0 8px 24px rgba(0,0,0,0.4); padding: 6px; }
.sidebar__tenant-item { display: flex; align-items: center; gap: 10px; width: 100%; padding: 9px 10px; background: none; border: none; color: #cbd5e1; font-size: 13px; font-weight: 500; border-radius: 8px; cursor: pointer; font-family: inherit; text-align: left; }
.sidebar__tenant-item:hover { background: rgba(255,255,255,0.05); color: #fff; }
.sidebar__tenant-item--active { color: #fff; }
.sidebar__tenant-item-icon { width: 18px; text-align: center; flex-shrink: 0; }

/* ── Колокольчик ── */
.sidebar__bell { position: relative; background: rgba(255,255,255,0.06); border: none; color: #cbd5e1; width: 34px; height: 34px; border-radius: 9px; cursor: pointer; flex-shrink: 0; display: inline-flex; align-items: center; justify-content: center; font-size: 15px; }
.sidebar__bell:hover { background: rgba(255,255,255,0.14); color: #fff; }
.sidebar__bell-icon { line-height: 1; }
.sidebar__bell-badge { position: absolute; top: -4px; right: -4px; min-width: 17px; height: 17px; padding: 0 4px; border-radius: 9px; background: #dc2626; color: #fff; font-size: 10px; font-weight: 700; display: flex; align-items: center; justify-content: center; }
.sidebar__bell-badge--zero { background: rgba(255,255,255,0.14); color: #94a3b8; }

/* ── Глобальные разделы ── */
.sidebar__sections { padding: 8px 8px 4px; }
.sidebar__section-label { font-size: 10px; color: #64748b; font-weight: 700; text-transform: uppercase; letter-spacing: 0.6px; padding: 6px 6px 4px; }
.sidebar__navlink { display: flex; align-items: center; gap: 12px; padding: 9px 10px; text-decoration: none; color: #cbd5e1; font-size: 14px; font-weight: 500; border-radius: 9px; cursor: pointer; background: none; border: none; width: 100%; text-align: left; font-family: inherit; }
.sidebar__navlink:hover { background: rgba(255,255,255,0.05); color: #fff; }
.sidebar__navlink--active { background: rgba(124,58,237,0.18); color: #fff; }
.sidebar__navlink--active .sidebar__navlink-icon { color: #a78bfa; }
.sidebar__navlink-icon { width: 20px; text-align: center; font-size: 14px; flex-shrink: 0; }

/* ── Контекстная навигация (settings/admin) ── */
.sidebar__context { flex: 1; min-height: 0; overflow-y: auto; padding: 6px 8px; border-top: 1px solid rgba(255,255,255,0.06); }

/* ── Футер ── */
.sidebar__footer { border-top: 1px solid rgba(255,255,255,0.06); padding: 8px 8px 12px; }
.sidebar__footer-link { display: flex; align-items: center; gap: 12px; padding: 9px 10px; text-decoration: none; color: #cbd5e1; font-size: 14px; font-weight: 500; border-radius: 9px; }
.sidebar__footer-link:hover { background: rgba(255,255,255,0.05); color: #fff; }
.sidebar__footer-icon { width: 20px; text-align: center; font-size: 14px; flex-shrink: 0; }
.sidebar__user { position: relative; display: flex; align-items: center; gap: 10px; padding: 10px; margin-top: 4px; }
.sidebar__user-info { flex: 1; min-width: 0; }
.sidebar__user-name { font-size: 14px; font-weight: 700; color: #fff; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.sidebar__user-sub { font-size: 12px; color: #94a3b8; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.sidebar__user-more { background: none; border: none; color: #94a3b8; font-size: 18px; line-height: 1; cursor: pointer; padding: 4px 6px; border-radius: 8px; flex-shrink: 0; letter-spacing: 1px; }
.sidebar__user-more:hover { background: rgba(255,255,255,0.1); color: #fff; }
.sidebar__user-menu { position: absolute; bottom: calc(100% - 2px); left: 10px; right: 10px; z-index: 50; background: #131b2e; border: 1px solid rgba(255,255,255,0.1); border-radius: 10px; box-shadow: 0 8px 24px rgba(0,0,0,0.4); padding: 6px; }
.sidebar__user-menu-item { display: flex; align-items: center; gap: 10px; width: 100%; padding: 9px 10px; background: none; border: none; color: #cbd5e1; font-size: 13px; font-weight: 500; border-radius: 8px; cursor: pointer; font-family: inherit; text-align: left; text-decoration: none; }
.sidebar__user-menu-item:hover { background: rgba(255,255,255,0.05); color: #fff; }
.sidebar__user-menu-item--danger, .sidebar__user-menu-item--danger:hover { color: #f87171; }
.sidebar__user-menu-icon { width: 18px; text-align: center; flex-shrink: 0; }

/* ── FeedbackNudge (перенесено из header.css) ── */
.fb-nudge__overlay { position: fixed; inset: 0; background: rgba(12,18,32,0.55); z-index: 2100; display: flex; align-items: center; justify-content: center; padding: 16px; }
.fb-nudge__card { position: relative; width: 380px; max-width: 92vw; background: #fff; border-radius: 16px; padding: 28px 26px 26px; box-shadow: 0 20px 50px rgba(15,23,42,0.35); text-align: center; animation: fbNudgeIn 0.18s ease; }
@keyframes fbNudgeIn { from { opacity: 0; transform: translateY(8px) scale(0.98); } to { opacity: 1; transform: none; } }
.fb-nudge__close { position: absolute; top: 12px; right: 12px; background: #f1f5f9; border: none; color: #64748b; font-size: 13px; cursor: pointer; width: 30px; height: 30px; border-radius: 8px; }
.fb-nudge__close:hover { background: #e2e8f0; color: #0f172a; }
.fb-nudge__icon { font-size: 34px; line-height: 1; margin-bottom: 12px; }
.fb-nudge__title { font-size: 18px; font-weight: 800; color: #0f172a; letter-spacing: -0.3px; margin-bottom: 8px; }
.fb-nudge__text { font-size: 14px; color: #475569; line-height: 1.55; margin-bottom: 20px; }
.fb-nudge__btn { display: inline-block; width: 100%; box-sizing: border-box; padding: 11px 16px; border-radius: 10px; background: #7c3aed; color: #fff; font-size: 14px; font-weight: 700; text-decoration: none; }
.fb-nudge__btn:hover { background: #6d28d9; }
```

- [ ] **Step 3: Подключить sidebar.js/sidebar.css в `tracker_shell.html`**

В `internal/http/templates/tracker_shell.html` добавить в `<head>` после строки `<link rel="stylesheet" href="/static/header.css">`:

```html
<link rel="stylesheet" href="/static/sidebar.css">
```

И перед строкой `<script type="text/babel" src="/static/tracker.js" ...>` добавить:

```html
<script type="text/babel" src="/static/sidebar.js" data-presets="react"></script>
```

(Строку с `header.js` пока НЕ трогаем — удалим в Task 5, чтобы каждая задача была самодостаточной и откатываемой.)

- [ ] **Step 4: Заменить сайдбар в `tracker.js`**

В `internal/web/static/tracker.js` заменить блок разметки сайдбара (текущие строки 2235–2271, от `<div className="sidebar">` до закрывающего `</div>` перед `<div className="main">`) на использование общего `Sidebar`. Итоговый фрагмент:

```jsx
      <Sidebar
        user={me}
        active="tracker"
        bell={hciData && hciData.has_scope
          ? <SidebarBell count={hciData.total_problems} onClick={() => setHciOpen(true)} />
          : null}
        beforeSections={
          <div className="sidebar__period">
            <div className="sidebar__period-label">Период</div>
            <PeriodSelect periods={periods} periodId={periodId} onChange={id => handlePeriodChange(id)} />
          </div>
        }
      >
        <div className="sidebar__tree">
          {!loading && hierarchy.length === 0
            ? (
              <div className="no-access">
                <div className="no-access__icon">🔒</div>
                {emptyHierMsg
                  ? <div className="no-access__text"><Markdown text={emptyHierMsg} /></div>
                  : <>
                      <div className="no-access__text">Нет доступа к командам</div>
                      <div className="no-access__hint">За доступом обратитесь к администратору</div>
                    </>}
              </div>
            )
            : <>
                <div className="sidebar__subsection-label">Команды</div>
                {favNodes.length > 0 && <>
                  <div className="sidebar__subsection-label"><span className="sidebar__subsection-star">★</span> Избранное · {favNodes.length}</div>
                  {favNodes.map(n => <SidebarNode key={`fav-${n.id}`} node={{ ...n, children: [] }} depth={0} selectedId={selId} onSelect={selectTeam} expanded={expanded} toggle={toggle} accent={accent} behindMargin={behindMargin} greenThreshold={greenThreshold} favSet={favSet} onToggleFav={onToggleFav} />)}
                  <div className="sidebar__subsection-label">Все команды</div>
                </>}
                {visibleTree.map(n => <SidebarNode key={n.id} node={n} depth={0} selectedId={selId} onSelect={selectTeam} expanded={expanded} toggle={toggle} accent={accent} behindMargin={behindMargin} greenThreshold={greenThreshold} favSet={favSet} onToggleFav={onToggleFav} />)}
              </>
          }
        </div>
      </Sidebar>
```

Заметки к замене:
- Раньше метка «Команды» использовала `sidebar__section-label`; теперь этот класс отдан глобальным РАЗДЕЛАМ, поэтому «Команды» переведена на `sidebar__subsection-label` (уже существует в `tracker.css`), чтобы не дублировать визуальный вес.
- Кнопка `HealthCheckInButton` и её обёртка `<div style={{ padding: '8px 8px 0' }}>` удаляются (её роль берёт `SidebarBell`). Компонент `HealthCheckInButton` в файле можно оставить неиспользуемым — он безвреден; удалять не обязательно.
- `<HealthCheckInPanel .../>` в конце `App` (текущая строка ~2346) НЕ трогаем — колокольчик открывает именно её через `setHciOpen`.
- `.sidebar__tree` уже имеет `flex:1; overflow:auto` в `tracker.css` — это и есть скролл-регион внутри `Sidebar`.

- [ ] **Step 5: Убрать дублирование `.sidebar__section-label` из tracker.css**

В `internal/web/static/tracker.css` удалить строку с определением `.sidebar__section-label` (текущая строка 20) — теперь класс определён в `sidebar.css` (загружается на трекере после `tracker.css`). Определения `.sidebar`, `.sidebar__period*`, `.sidebar__tree`, `.sidebar__subsection-label*`, `.sidebar-node__*` оставить как есть. `.sidebar` продублирован в `sidebar.css` идентичными значениями (безвредно; sidebar.css грузится последним).

- [ ] **Step 6: Проверка (сборка + браузер)**

Run: `go build ./... && go vet ./...`
Expected: без ошибок.

Перезапустить сервер (`go run ./cmd/server`), открыть `http://localhost:8080/`. Ожидаемо:
- Вверху сайдбара — квадратный аватар-инициалы + название тенанта; бургер-кнопки `☰` больше нет.
- Справа от названия — колокольчик 🔔 с числовым бейджем (если у пользователя есть scope Health Check-in); клик открывает панель Health Check-in.
- Под шапкой — блок «ПЕРИОД» с селектом; ниже — «РАЗДЕЛЫ» со ссылками (Цели команды подсвечена), затем «Команды»/«Избранное»/дерево (скроллится).
- Внизу — «Документация», «Обратная связь» (если включены в конфиге) и блок пользователя с «···» → меню (Настройки / Администрирование для админов / Выйти).
- Переключение тенанта работает (если тенантов > 1): клик по названию → выпадающий список.

---

### Task 2: Миграция заглушек (`stub.js` + `stub_shell.html`)

**Files:**
- Modify: `internal/http/templates/stub_shell.html` (подключить sidebar.js/sidebar.css)
- Modify: `internal/web/static/stub.js:18-33` (заменить самодельный сайдбар на `<Sidebar>`)

**Interfaces:**
- Consumes: `Sidebar`, `SidebarSections` из `sidebar.js` (Task 1). `active` берётся из `meta.id ∈ 'activity-log' | 'goal-tree'`.

- [ ] **Step 1: Подключить sidebar.js/sidebar.css в `stub_shell.html`**

В `internal/http/templates/stub_shell.html`:
- В `<head>` после `<link rel="stylesheet" href="/static/header.css">` добавить:

```html
<link rel="stylesheet" href="/static/sidebar.css">
```

- Перед `<script type="text/babel" src="/static/stub.js" ...>` добавить:

```html
<script type="text/babel" src="/static/sidebar.js" data-presets="react"></script>
```

- [ ] **Step 2: Заменить сайдбар в `stub.js`**

В `internal/web/static/stub.js` заменить возвращаемую разметку `StubApp` (текущий блок от `<div style={{ display: 'flex', height: '100vh', overflow: 'hidden' }}>` до конца) на:

```jsx
  return (
    <div style={{ display: 'flex', height: '100vh', overflow: 'hidden' }}>
      <Sidebar user={me} active={meta.id} />
      <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', flexDirection: 'column', gap: 12 }}>
        <div style={{ fontSize: 44 }}>{meta.icon}</div>
        <div style={{ fontSize: 22, fontWeight: 700, color: '#111827' }}>{meta.title}</div>
        <div style={{ fontSize: 14, color: '#6b7280' }}>Раздел в разработке</div>
      </div>
    </div>
  );
```

Старый инлайновый сайдбар (`<div style={{ width: 252, ... }}>` с `HeaderNavMenu` и логотипом) удаляется целиком.

- [ ] **Step 3: Проверка (браузер)**

Перезапустить сервер, открыть `http://localhost:8080/goal-tree` и `http://localhost:8080/activity-log`. Ожидаемо:
- Тот же тёмный сайдбар, что на трекере: шапка тенанта (без колокольчика), «РАЗДЕЛЫ» с подсвеченным активным разделом (Дерево целей / Лог активностей), футер с пользователем и ссылками.
- Блока «ПЕРИОД» и дерева команд нет (это контекст только трекера).
- Справа — плейсхолдер «Раздел в разработке».
- Клик по «Цели команды» ведёт на `/`.

---

### Task 3: Миграция настроек (`settings.js` + `settings_shell.html`)

**Files:**
- Modify: `internal/http/templates/settings_shell.html` (подключить sidebar.js/sidebar.css)
- Modify: `internal/web/static/settings.js:421-444` (заменить `set-sidebar` на `<Sidebar>` с контекстом)

**Interfaces:**
- Consumes: `Sidebar` из `sidebar.js`. Локальные секции настроек передаются как `children` в `.sidebar__context`. `active={null}` (глобальные разделы не подсвечены на `/settings`).
- Внутренняя навигация настроек (`navigate`, `SECTION_META`, `active`, `sections`) не меняется — меняется только разметка.

- [ ] **Step 1: Подключить sidebar.js/sidebar.css в `settings_shell.html`**

В `internal/http/templates/settings_shell.html`:
- В `<head>` после `<link rel="stylesheet" href="/static/settings.css">` добавить:

```html
<link rel="stylesheet" href="/static/sidebar.css">
```

- Перед `<script type="text/babel" src="/static/settings.js" ...>` добавить:

```html
<script type="text/babel" src="/static/sidebar.js" data-presets="react"></script>
```

- [ ] **Step 2: Заменить `<aside className="set-sidebar">` в `settings.js`**

В `internal/web/static/settings.js` заменить весь блок `<aside className="set-sidebar">...</aside>` (текущие строки 421–444) на:

```jsx
      <Sidebar user={me} active={null}>
        <div className="sidebar__context">
          <div className="sidebar__section-label">Настройки</div>
          {sections.map(id => {
            const m = SECTION_META[id];
            return (
              <button
                key={id}
                className={`sidebar__navlink${id === active ? ' sidebar__navlink--active' : ''}`}
                onClick={() => navigate(id)}
              >
                <span className="sidebar__navlink-icon">{m.icon}</span>{m.label}
              </button>
            );
          })}
        </div>
      </Sidebar>
```

Заметки:
- Подсказки (`hint`) секций опускаются — глобальные РАЗДЕЛЫ подписей не имеют, единый визуальный язык одной колонки. Данные не теряются: раздел раскрывает описание в контентной области.
- Кнопка «← Вернуться к OKR Tracker» убрана — возврат к трекеру идёт через глобальные РАЗДЕЛЫ («Цели команды»). Верхний бар настроек (`set-topbar` с пилюлей «← OKR Tracker») не трогаем.
- Внешний контейнер `<div className="set-app">` и `<div className="set-content">` остаются; `.sidebar` встаёт как flex-ребёнок вместо `.set-sidebar`.

- [ ] **Step 3: Проверка (браузер)**

Перезапустить сервер, открыть `http://localhost:8080/settings`. Ожидаемо:
- Тёмный сайдбар: шапка тенанта, «РАЗДЕЛЫ» (ничего не подсвечено), ниже «НАСТРОЙКИ» с секциями (Описание команд для лида / Мой сайдбар / Мои пространства), активная секция подсвечена, футер с пользователем.
- Переключение секций настроек работает (URL `?section=` меняется), контент справа обновляется.
- Клик по «Цели команды» в РАЗДЕЛАХ ведёт на `/`.

---

### Task 4: Миграция админки (`admin.js` + `admin_shell.html`)

**Files:**
- Modify: `internal/http/templates/admin_shell.html` (подключить sidebar.js/sidebar.css)
- Modify: `internal/web/static/admin.js:305-338` (заменить инлайн-сайдбар в `Shell` на `<Sidebar>`)

**Interfaces:**
- Consumes: `Sidebar` из `sidebar.js`. `ADMIN_SECTIONS` рендерятся как `children` в `.sidebar__context`. `active={null}`.
- `Shell({section, setSection, currentUser, children})` — сигнатура не меняется; `setSection` по-прежнему переключает раздел админки.

- [ ] **Step 1: Подключить sidebar.js/sidebar.css в `admin_shell.html`**

В `internal/http/templates/admin_shell.html`:
- В `<head>` после `<link rel="stylesheet" href="/static/header.css">` добавить:

```html
<link rel="stylesheet" href="/static/sidebar.css">
```

- Перед `<script type="text/babel" src="/static/admin.js" ...>` добавить:

```html
<script type="text/babel" src="/static/sidebar.js" data-presets="react"></script>
```

- [ ] **Step 2: Заменить инлайн-сайдбар в `admin.js`**

В `internal/web/static/admin.js`, в компоненте `Shell`, заменить первый дочерний `<div>` сайдбара (текущие строки 309–338, `<div style={{width:252,...}}>...</div>`) на:

```jsx
    <Sidebar user={currentUser} active={null}>
      <div className="sidebar__context">
        <div className="sidebar__section-label">Администрирование</div>
        {sections.map(s => (
          <button
            key={s.id}
            onClick={() => setSection(s.id)}
            className={`sidebar__navlink${s.id === section ? ' sidebar__navlink--active' : ''}`}
          >
            <span className="sidebar__navlink-icon">{s.icon}</span>{s.label}
          </button>
        ))}
      </div>
    </Sidebar>
```

Заметки:
- Внешний `<div style={{display:'flex',height:'100vh',overflow:'hidden'}}>` и правый контент-`<div>` остаются без изменений.
- Кнопка «← Вернуться к OKR Tracker» из сайдбара убрана (возврат — через РАЗДЕЛЫ). Верхняя пилюля «← OKR Tracker» в контент-баре (строки ~341–342) остаётся.
- Подсказки `hint` секций опускаются — единый визуальный язык с остальными пунктами.

- [ ] **Step 3: Проверка (браузер)**

Перезапустить сервер, войти админом, открыть `http://localhost:8080/admin`. Ожидаемо:
- Тёмный сайдбар: шапка тенанта, «РАЗДЕЛЫ», ниже «АДМИНИСТРИРОВАНИЕ» с секциями (Периоды / Команды / Пользователи / Настройки / Health Check-in), активная подсвечена, футер.
- Переключение секций админки работает. Клик по «Цели команды» ведёт на `/`.

---

### Task 5: Удалить `header.js`/`header.css` и обновить тест шаблона

**Files:**
- Modify: `internal/http/templates/tracker_shell.html`, `stub_shell.html`, `settings_shell.html`, `admin_shell.html` (убрать header.js/header.css)
- Delete: `internal/web/static/header.js`, `internal/web/static/header.css`
- Modify: `internal/http/templates_test.go:19` (заменить `/static/header.js` на `/static/sidebar.js`)

**Interfaces:**
- Consumes: все 4 shell уже подключают `sidebar.js`/`sidebar.css` (Tasks 1–4) и не используют `HeaderNavMenu`.

- [ ] **Step 1: Убрать подключение header.js/header.css из всех 4 shell**

В каждом из `tracker_shell.html`, `stub_shell.html`, `settings_shell.html`, `admin_shell.html` удалить две строки:

```html
<link rel="stylesheet" href="/static/header.css">
```
```html
<script type="text/babel" src="/static/header.js" data-presets="react"></script>
```

(В `base.html` `header.js`/`header.css` не подключаются — его не трогаем.)

- [ ] **Step 2: Проверить, что нигде не осталось ссылок на header.js/HeaderNavMenu**

Run: `rg -n "header\.js|header\.css|HeaderNavMenu" internal/`
Expected: пусто (ни одного совпадения).

Если что-то найдено — заменить/удалить перед продолжением.

- [ ] **Step 3: Удалить файлы**

```bash
rm internal/web/static/header.js internal/web/static/header.css
```

- [ ] **Step 4: Обновить `templates_test.go`**

В `internal/http/templates_test.go` заменить строку 19:

```go
	for _, want := range []string{`/static/header.js`, `/static/stub.js`, `id="root"`} {
```

на:

```go
	for _, want := range []string{`/static/sidebar.js`, `/static/stub.js`, `id="root"`} {
```

- [ ] **Step 5: Проверка (тесты + браузер)**

Run: `go test ./internal/http/... && go build ./... && go vet ./...`
Expected: PASS, без ошибок.

Перезапустить сервер и быстро проверить все 4 поверхности (`/`, `/goal-tree`, `/settings`, `/admin`): сайдбар рендерится, бургер-кнопки `☰` нигде нет, FeedbackNudge по-прежнему может всплывать (логика перенесена в `SidebarFooter`).

---

### Task 6: Обновить specs

**Files:**
- Modify: `specs/010-architecture-constraints.md:11`
- Modify: `specs/030-user-flows.md` (§3г и упоминания бургера)
- Modify: `specs/040-api-contract.md:115`

**Interfaces:** документация; кода не касается.

- [ ] **Step 1: `specs/010-architecture-constraints.md`**

В пункте про общие модули (строка 11) заменить описание `header.js` (`HeaderNavMenu` — гамбургер-меню) на `sidebar.js`: общий тёмный сайдбар навигации (`Sidebar`, `SidebarTenant`, `SidebarSections`, `SidebarFooter`, `SidebarBell`, `FeedbackNudge`), единый источник правды переиспользуемой навигации в трекере, заглушках, настройках и админке; самодостаточен (свой рендер аватара, чтение CSRF, logout, запросы `/api/v1/config` и `/api/v1/session/tenants`), стили — `sidebar.css`, подключён во всех SPA-shell. Убрать упоминание `header.css`. Указать, что `system.js` (superadmin) вне этой унификации.

- [ ] **Step 2: `specs/030-user-flows.md`**

Обновить упоминания гамбургер-меню:
- §3г «Глобальная навигация (гамбургер-меню)» (строки ~165–189): переименовать в «Глобальная навигация (сайдбар)»; описать постоянно видимый тёмный сайдбар (шапка тенанта с переключателем и колокольчиком Health Check-in на трекере; блок «РАЗДЕЛЫ» с постоянными ссылками; футер с документацией/обратной связью и блоком пользователя с меню «···» → Настройки/Администрирование/Выйти). Заглушки (~189): тот же общий сайдбар, без колокольчика.
- Строка ~24: «Пользователь нажимает „Выйти“ в гамбургер-меню» → «в меню пользователя внизу сайдбара («···»)».
- Строки ~201 и ~302: описания layout трекера/общего сайдбара — убрать `☰`, заменить на постоянную навигацию в сайдбаре.

- [ ] **Step 3: `specs/040-api-contract.md`**

Строка 115: `feedback_menu_link_enabled` — «включён ли пункт „Обратная связь“ в гамбургер-меню» → «включена ли ссылка „Обратная связь“ в футере сайдбара».

- [ ] **Step 4: Проверка**

Run: `rg -n -i "гамбургер|burger|☰" specs/`
Expected: не осталось описаний активного гамбургер-меню как текущего UI (допустимы только исторические/changelog-упоминания, если такие есть).

---

## Итоговая проверка (после всех задач)

- `go build ./... && go vet ./... && go test ./...` — зелёные.
- `rg -n "header\.js|HeaderNavMenu" internal/` — пусто.
- Ручной проход по `/`, `/goal-tree`, `/activity-log`, `/settings`, `/admin`: единый сайдбар, работающий переключатель тенанта, колокольчик Health Check-in на трекере, корректная подсветка активного раздела, футер с меню пользователя, отсутствие бургера.
