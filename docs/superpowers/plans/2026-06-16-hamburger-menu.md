# Глобальное гамбургер-меню — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Добавить единое гамбургер-меню (`☰`), идентичное на трекере, настройках, админке и двух новых страницах-заглушках; объединить навигацию по разделам и аккаунт-действия в одном выезжающем слева drawer'е.

**Architecture:** Новый глобальный React-компонент `HeaderNavMenu` в `header.js` (загружается как `text/babel` до app-скриптов — единый источник правды, как `markdown.js`/`header.js` сейчас). Старый `HeaderUserMenu` удаляется. Две заглушки (`/activity-log`, `/goal-tree`) отдаются новым Go-шаблоном `stub-shell` + маленьким `stub.js`.

**Tech Stack:** Go (chi router, `html/template`, embed), React 18 + Babel standalone (inline в браузере), без сборщика.

> **ВАЖНО про коммиты:** В этом проекте действует правило CLAUDE.md «не делать git commits — пользователь коммитит сам». Поэтому шаги «Commit» заменены на чекпойнты «Запустить проверки и остановиться для ревью». НЕ выполняй `git commit`.

---

## File Structure

- `internal/web/static/header.js` — **Modify.** Добавить `HeaderNavMenu`; удалить `HeaderUserMenu` (после миграции всех вызовов). `HeaderAvatar`/`_hdrCSRF` остаются (переиспользуются).
- `internal/web/static/header.css` — **Modify.** Добавить стили `.nav-menu__*`; удалить `.user-menu__*`.
- `internal/web/static/tracker.js` — **Modify.** Заменить `<HeaderUserMenu>` на `<HeaderNavMenu>` в шапке сайдбара.
- `internal/web/static/admin.js` — **Modify.** То же.
- `internal/web/static/settings.js` — **Modify.** То же.
- `internal/web/static/stub.js` — **Create.** Мини-SPA для страниц-заглушек.
- `internal/http/templates/stub_shell.html` — **Create.** Go-шаблон `{{define "stub-shell"}}`.
- `internal/http/server.go` — **Modify.** Извлечь `parseTemplates()`; добавить роуты `/activity-log`, `/goal-tree`.
- `internal/http/templates_test.go` — **Create.** Тест рендера `stub-shell`.
- `specs/010-architecture-constraints.md` — **Modify.** Обновить описание `header.js`.
- `specs/030-user-flows.md` — **Modify.** Обновить упоминания аккаунт-виджета + добавить раздел про навигацию/заглушки.

---

## Task 1: Компонент `HeaderNavMenu` + стили

Добавляем новый компонент рядом с существующим `HeaderUserMenu` (пока не удаляем старый — чтобы страницы не сломались до миграции в задачах 2–4).

**Files:**
- Modify: `internal/web/static/header.js`
- Modify: `internal/web/static/header.css`

- [ ] **Step 1: Добавить `HeaderNavMenu` в конец `header.js`**

В конец файла `internal/web/static/header.js` (после `HeaderUserMenu`, перед концом файла) добавить:

```jsx
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
    { id: 'tracker',      label: 'OKR Tracker',     href: '/',            icon: '🎯' },
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
```

- [ ] **Step 2: Добавить стили `.nav-menu__*` в конец `header.css`**

В конец `internal/web/static/header.css` добавить:

```css
/* HeaderNavMenu — глобальное гамбургер-меню. Тёмный drawer, выезжает слева;
   совпадает по визуальному языку с тёмными сайдбарами трекера/админки. */
.nav-menu__burger { background: transparent; border: none; color: #cbd5e1; font-size: 18px; line-height: 1; cursor: pointer; padding: 4px 7px; border-radius: 8px; flex-shrink: 0; }
.nav-menu__burger:hover { background: rgba(255,255,255,0.1); color: #fff; }
.nav-menu__overlay { position: fixed; inset: 0; background: rgba(0,0,0,0.45); z-index: 2000; display: flex; }
.nav-menu__panel { width: 300px; max-width: 85vw; height: 100%; background: #0c1220; display: flex; flex-direction: column; box-shadow: 4px 0 24px rgba(0,0,0,0.4); animation: navMenuIn 0.18s ease; }
@keyframes navMenuIn { from { transform: translateX(-100%); } to { transform: translateX(0); } }
.nav-menu__head { display: flex; align-items: center; justify-content: space-between; padding: 14px 16px; border-bottom: 1px solid rgba(255,255,255,0.06); }
.nav-menu__head-logo { font-size: 15px; font-weight: 800; color: #fff; letter-spacing: -0.3px; }
.nav-menu__close { background: rgba(255,255,255,0.06); border: none; color: #cbd5e1; font-size: 13px; cursor: pointer; width: 30px; height: 30px; border-radius: 8px; }
.nav-menu__close:hover { background: rgba(255,255,255,0.14); color: #fff; }
.nav-menu__sections { padding: 10px 8px; flex: 1; overflow-y: auto; }
.nav-menu__label { font-size: 10px; color: #64748b; font-weight: 700; letter-spacing: 0.6px; text-transform: uppercase; padding: 10px 12px 6px; }
.nav-menu__item { display: flex; align-items: center; gap: 12px; padding: 10px 12px; text-decoration: none; color: #cbd5e1; font-size: 14px; font-weight: 500; border-radius: 9px; cursor: pointer; background: none; border: none; width: 100%; text-align: left; font-family: inherit; }
.nav-menu__item:hover { background: rgba(255,255,255,0.05); color: #fff; }
.nav-menu__item--active { background: rgba(124,58,237,0.18); color: #fff; }
.nav-menu__item--active .nav-menu__item-icon { color: #a78bfa; }
.nav-menu__item-icon { width: 22px; text-align: center; font-size: 15px; flex-shrink: 0; }
.nav-menu__item--danger, .nav-menu__item--danger:hover { color: #f87171; }
.nav-menu__foot { border-top: 1px solid rgba(255,255,255,0.06); padding: 10px 8px 14px; }
.nav-menu__profile { display: flex; align-items: center; gap: 10px; padding: 10px; }
.nav-menu__profile-info { flex: 1; min-width: 0; }
.nav-menu__profile-name { font-size: 14px; font-weight: 700; color: #fff; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.nav-menu__profile-sub { font-size: 12px; color: #94a3b8; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.nav-menu__gear { display: inline-flex; align-items: center; justify-content: center; width: 34px; height: 34px; border-radius: 9px; background: rgba(255,255,255,0.06); color: #cbd5e1; text-decoration: none; font-size: 15px; flex-shrink: 0; }
.nav-menu__gear:hover { background: rgba(255,255,255,0.14); color: #fff; }
```

- [ ] **Step 3: Проверка синтаксиса JS (Babel)**

JS не компилируется тулчейном; проверяем синтаксис локально через node (Babel в браузере не падает молча, но node ловит грубые ошибки скобок). Запусти:

Run: `node --check internal/web/static/header.js 2>&1 | head` 

Expected: либо пусто (ок), либо ошибка про JSX (`Unexpected token <`) — это нормально, `node --check` не понимает JSX. Если ошибка указывает на несбалансированные скобки/кавычки — исправь. Достаточная проверка — отсутствие синтаксических ошибок вне JSX. (Полная визуальная проверка — в Task 6.)

---

## Task 2: Подключить `HeaderNavMenu` в трекере

**Files:**
- Modify: `internal/web/static/tracker.js` (~строки 1860–1867)

- [ ] **Step 1: Заменить шапку сайдбара трекера**

Найти в `internal/web/static/tracker.js`:

```jsx
        <div className="sidebar__header">
          <div className="sidebar__logo">OKR Tracker</div>
          {me && <HeaderUserMenu user={me} docUrl={docUrl} showTrackerLink={false} />}
        </div>
```

Заменить на:

```jsx
        <div className="sidebar__header">
          {me && <HeaderNavMenu user={me} active="tracker" />}
          <div className="sidebar__logo">OKR Tracker</div>
        </div>
```

(`docUrl` теперь компонент тянет сам; стейт `docUrl` в трекере оставляем как есть — он используется в Promise.all и безвреден.)

- [ ] **Step 2: Проверка**

Run: `git diff --stat internal/web/static/tracker.js`
Expected: один изменённый файл, ~3 строки изменены. Визуальная проверка — в Task 6.

---

## Task 3: Подключить `HeaderNavMenu` в админке

**Files:**
- Modify: `internal/web/static/admin.js` (~строки 307–314)

- [ ] **Step 1: Заменить шапку сайдбара админки**

Найти в `internal/web/static/admin.js`:

```jsx
      <div style={{padding:'12px 14px',borderBottom:'1px solid rgba(255,255,255,0.06)',display:'flex',alignItems:'center',gap:10}}>
        <div style={{flex:1,minWidth:0}}>
          <div style={{fontSize:14,fontWeight:800,color:'white',letterSpacing:'-.3px'}}>OKR Tracker</div>
          <div style={{fontSize:10,color:T.sidebarMuted,fontWeight:600,textTransform:'uppercase',letterSpacing:.5,marginTop:1}}>Управление</div>
        </div>
        <HeaderUserMenu user={currentUser}/>
      </div>
```

Заменить на:

```jsx
      <div style={{padding:'12px 14px',borderBottom:'1px solid rgba(255,255,255,0.06)',display:'flex',alignItems:'center',gap:10}}>
        {currentUser && <HeaderNavMenu user={currentUser} active={null}/>}
        <div style={{flex:1,minWidth:0}}>
          <div style={{fontSize:14,fontWeight:800,color:'white',letterSpacing:'-.3px'}}>OKR Tracker</div>
          <div style={{fontSize:10,color:T.sidebarMuted,fontWeight:600,textTransform:'uppercase',letterSpacing:.5,marginTop:1}}>Управление</div>
        </div>
      </div>
```

- [ ] **Step 2: Проверка**

Run: `rg -n "HeaderUserMenu|HeaderNavMenu" internal/web/static/admin.js`
Expected: только `HeaderNavMenu` (плюс возможный комментарий о header.js). `<HeaderUserMenu ...>` больше не вызывается.

---

## Task 4: Подключить `HeaderNavMenu` в настройках

**Files:**
- Modify: `internal/web/static/settings.js` (~строки 357–363)

- [ ] **Step 1: Заменить шапку сайдбара настроек**

Найти в `internal/web/static/settings.js`:

```jsx
        <div className="set-sidebar__header">
          <div>
            <div className="set-sidebar__logo">OKR Tracker</div>
            <div className="set-sidebar__sub">Настройки</div>
          </div>
          {me && <HeaderUserMenu user={me} />}
        </div>
```

Заменить на:

```jsx
        <div className="set-sidebar__header">
          {me && <HeaderNavMenu user={me} active={null} />}
          <div>
            <div className="set-sidebar__logo">OKR Tracker</div>
            <div className="set-sidebar__sub">Настройки</div>
          </div>
        </div>
```

- [ ] **Step 2: Проверка**

Run: `rg -n "HeaderUserMenu" internal/web/static/settings.js`
Expected: ни одного вызова `<HeaderUserMenu>` (возможен только текстовый комментарий на строке 69 — его тоже можно поправить в Task 6).

---

## Task 5: Страницы-заглушки (`stub.js`, `stub-shell`, роуты, Go-тест)

Сначала пишем падающий Go-тест на рендер шаблона `stub-shell`, затем реализуем.

**Files:**
- Create: `internal/http/templates_test.go`
- Modify: `internal/http/server.go`
- Create: `internal/http/templates/stub_shell.html`
- Create: `internal/web/static/stub.js`

- [ ] **Step 1: Написать падающий тест**

Создать `internal/http/templates_test.go`:

```go
package http

import (
	"bytes"
	"strings"
	"testing"
)

func TestStubShellRenders(t *testing.T) {
	tmpl, err := parseTemplates()
	if err != nil {
		t.Fatalf("parseTemplates: %v", err)
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "stub-shell", nil); err != nil {
		t.Fatalf("execute stub-shell: %v", err)
	}
	out := buf.String()
	for _, want := range []string{`/static/header.js`, `/static/stub.js`, `id="root"`} {
		if !strings.Contains(out, want) {
			t.Errorf("stub-shell output missing %q", want)
		}
	}
}
```

- [ ] **Step 2: Запустить тест — убедиться, что не компилируется/падает**

Run: `go test ./internal/http/ -run TestStubShellRenders 2>&1 | head`
Expected: ошибка компиляции `undefined: parseTemplates` (функции ещё нет).

- [ ] **Step 3: Извлечь `parseTemplates()` в `server.go`**

В `internal/http/server.go` добавить функцию (например, сразу после объявления `var templatesFS embed.FS`):

```go
func parseTemplates() (*template.Template, error) {
	return template.New("").Funcs(template.FuncMap{
		"sumKRWeights": func(keyResults []domain.KeyResult) int {
			total := 0
			for _, kr := range keyResults {
				total += kr.Weight
			}
			return total
		},
		"sumStageWeights": func(stages []domain.KRProjectStage) int {
			total := 0
			for _, stage := range stages {
				total += stage.Weight
			}
			return total
		},
		"priorityBadgeClass": func(priority domain.Priority) string {
			switch priority {
			case domain.PriorityP0:
				return "text-bg-danger"
			case domain.PriorityP1, domain.PriorityP2:
				return "text-bg-success"
			case domain.PriorityP3:
				return "text-bg-secondary"
			default:
				return "text-bg-secondary"
			}
		},
	}).ParseFS(templatesFS, "templates/*.html")
}
```

Затем в `NewServer` заменить блок:

```go
	tmpl, err := template.New("").Funcs(template.FuncMap{
		"sumKRWeights": func(keyResults []domain.KeyResult) int {
			total := 0
			for _, kr := range keyResults {
				total += kr.Weight
			}
			return total
		},
		"sumStageWeights": func(stages []domain.KRProjectStage) int {
			total := 0
			for _, stage := range stages {
				total += stage.Weight
			}
			return total
		},
		"priorityBadgeClass": func(priority domain.Priority) string {
			switch priority {
			case domain.PriorityP0:
				return "text-bg-danger"
			case domain.PriorityP1, domain.PriorityP2:
				return "text-bg-success"
			case domain.PriorityP3:
				return "text-bg-secondary"
			default:
				return "text-bg-secondary"
			}
		},
	}).ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
```

на:

```go
	tmpl, err := parseTemplates()
	if err != nil {
		return nil, err
	}
```

- [ ] **Step 4: Создать шаблон `stub_shell.html`**

Создать `internal/http/templates/stub_shell.html`:

```html
{{define "stub-shell"}}
<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>OKR Tracker</title>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700;800&display=swap" rel="stylesheet">
<link rel="stylesheet" href="/static/header.css">
<style>
*{box-sizing:border-box;margin:0;padding:0}
html,body,#root{height:100%;font-family:'Inter',sans-serif;background:#edf0f4;color:#111827;-webkit-font-smoothing:antialiased}
button{font-family:inherit}
</style>
</head>
<body>
<div id="root"><div style="height:100vh;display:flex;align-items:center;justify-content:center;color:#6b7280;font-size:14px">Загрузка…</div></div>
<script src="https://unpkg.com/react@18.3.1/umd/react.development.js" integrity="sha384-hD6/rw4ppMLGNu3tX5cjIb+uRZ7UkRJ6BPkLpg4hAu/6onKUg4lLsHAs9EBPT82L" crossorigin="anonymous"></script>
<script src="https://unpkg.com/react-dom@18.3.1/umd/react-dom.development.js" integrity="sha384-u6aeetuaXnQ38mYT8rp6sbXaQe3NL9t+IBXmnYxwkUI2Hw4bsp2Wvmx4yRQF1uAm" crossorigin="anonymous"></script>
<script src="https://unpkg.com/@babel/standalone@7.29.0/babel.min.js" integrity="sha384-m08KidiNqLdpJqLq95G/LEi8Qvjl/xUYll3QILypMoQ65QorJ9Lvtp2RXYGBFj1y" crossorigin="anonymous"></script>
<script type="text/babel" src="/static/header.js" data-presets="react"></script>
<script type="text/babel" src="/static/stub.js" data-presets="react"></script>
</body>
</html>
{{end}}
```

- [ ] **Step 5: Создать `stub.js`**

Создать `internal/web/static/stub.js`:

```jsx
// Страницы-заглушки «в разработке» (/activity-log, /goal-tree).
// Рендерят тот же общий HeaderNavMenu из header.js + плейсхолдер контента.
const STUB_SECTIONS = {
  '/activity-log': { id: 'activity-log', title: 'Лог активностей', icon: '🕑' },
  '/goal-tree':    { id: 'goal-tree',    title: 'Дерево целей',    icon: '🕸' },
};

function StubApp() {
  const [me, setMe] = React.useState(null);
  React.useEffect(() => {
    fetch('/api/v1/me', { credentials: 'include' })
      .then(r => (r.ok ? r.json() : null))
      .then(d => { if (d) setMe(d); })
      .catch(() => {});
  }, []);
  const meta = STUB_SECTIONS[location.pathname] || { id: null, title: 'Раздел', icon: '•' };
  return (
    <div style={{ display: 'flex', height: '100vh', overflow: 'hidden' }}>
      <div style={{ width: 252, background: '#0c1220', display: 'flex', flexDirection: 'column', flexShrink: 0 }}>
        <div style={{ padding: '12px 14px', borderBottom: '1px solid rgba(255,255,255,0.06)', display: 'flex', alignItems: 'center', gap: 10 }}>
          {me && <HeaderNavMenu user={me} active={meta.id} />}
          <div style={{ fontSize: 14, fontWeight: 800, color: 'white', letterSpacing: '-0.3px' }}>OKR Tracker</div>
        </div>
      </div>
      <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', flexDirection: 'column', gap: 12 }}>
        <div style={{ fontSize: 44 }}>{meta.icon}</div>
        <div style={{ fontSize: 22, fontWeight: 700, color: '#111827' }}>{meta.title}</div>
        <div style={{ fontSize: 14, color: '#6b7280' }}>Раздел в разработке</div>
      </div>
    </div>
  );
}

ReactDOM.createRoot(document.getElementById('root')).render(<StubApp />);
```

- [ ] **Step 6: Добавить роуты в `server.go`**

В `internal/http/server.go` сразу после блока роута `/settings` (после закрывающей `})` хендлера `settings-shell`) добавить:

```go
	// Страницы-заглушки разделов навигации (гамбургер-меню). Доступны любому
	// авторизованному пользователю, как /settings.
	stubShell := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = s.tmpl.ExecuteTemplate(w, "stub-shell", nil)
	}
	r.Get("/activity-log", stubShell)
	r.Get("/goal-tree", stubShell)
```

- [ ] **Step 7: Запустить тест — должен пройти**

Run: `go test ./internal/http/ -run TestStubShellRenders -v`
Expected: PASS.

- [ ] **Step 8: Vet + полный билд**

Run: `go vet ./... && go build ./...`
Expected: без ошибок.

---

## Task 6: Удалить `HeaderUserMenu`, визуальная проверка

Все вызовы мигрированы (Tasks 2–4) — старый компонент и его стили теперь мертвы.

**Files:**
- Modify: `internal/web/static/header.js`
- Modify: `internal/web/static/header.css`
- Modify: `internal/web/static/settings.js` (комментарий на строке ~69), `tracker.js`/`admin.js` (комментарии о header.js — по желанию, оставить корректными)

- [ ] **Step 1: Убедиться, что вызовов не осталось**

Run: `rg -n "<HeaderUserMenu" internal/web/static/`
Expected: ничего не найдено (нет JSX-вызовов). Текстовые комментарии со словом `HeaderUserMenu` допустимы, но обнови их при желании на `HeaderNavMenu`.

- [ ] **Step 2: Удалить функцию `HeaderUserMenu` из `header.js`**

Удалить из `internal/web/static/header.js` всю функцию `function HeaderUserMenu({ user, docUrl, showTrackerLink = true }) { ... }` целиком (строки ~32–75). `HeaderAvatar` и `_hdrCSRF` НЕ трогать — их использует `HeaderNavMenu`.

- [ ] **Step 3: Удалить стили `.user-menu__*` из `header.css`**

Удалить из `internal/web/static/header.css` все правила, начинающиеся с `.user-menu` (блок `.user-menu { ... }` … `.user-menu__divider { ... }`). Комментарий-заголовок файла оставить/обновить.

- [ ] **Step 4: Запустить сервер для визуальной проверки**

Run (фоном): `go run ./cmd/server` — затем открыть в браузере:
- `/` (трекер): слева вверху есть `☰`; клик открывает drawer; «OKR Tracker» подсвечен в «Разделы»; внизу профиль + ⚙ + Документация/Администрирование/Выйти. Esc и клик по фону закрывают.
- `/settings`: тот же `☰`, ни один раздел не подсвечен.
- `/admin` (под админом): `☰` слева, drawer идентичен.
- `/activity-log` и `/goal-tree`: страница-заглушка «Раздел в разработке», в drawer подсвечен соответствующий раздел.
- «Администрирование» в меню показывается только если `is_admin`.

Expected: меню выглядит и ведёт себя идентично на всех страницах; старого аватар-меню справа больше нет.

- [ ] **Step 5: Чекпойнт (без коммита)**

Run: `go test ./... && go vet ./...`
Expected: всё зелёное. НЕ коммитить — остановиться для ревью пользователем.

---

## Task 7: Обновить specs

**Files:**
- Modify: `specs/010-architecture-constraints.md` (строка 11)
- Modify: `specs/030-user-flows.md`

- [ ] **Step 1: Обновить `010-architecture-constraints.md`**

Заменить строку 11:

```
- общие модули загружаются как `text/babel` ПЕРЕД app-скриптом каждого shell, экспортируя глобальные компоненты: `markdown.js` (`Markdown`, `MarkdownEditor`) и `header.js` (`HeaderUserMenu` — меню аватарки в шапке). Это единственный источник правды для переиспользуемого UI: меню аватарки одинаково (раскрытие по наведению, одни и те же пункты и CSS) в трекере, админке и настройках. `header.js` самодостаточен (свой рендер аватара инлайн-стилями, чтение CSRF из cookie, logout), стили — `header.css`, подключён во всех трёх shell (`tracker_shell`, `admin_shell`, `settings_shell`);
```

на:

```
- общие модули загружаются как `text/babel` ПЕРЕД app-скриптом каждого shell, экспортируя глобальные компоненты: `markdown.js` (`Markdown`, `MarkdownEditor`) и `header.js` (`HeaderNavMenu` — глобальное гамбургер-меню в шапке). Это единственный источник правды для переиспользуемого UI: гамбургер-меню одинаково (кнопка `☰` → выезжающий слева drawer с разделами навигации и аккаунт-блоком, одни и те же пункты и CSS) в трекере, админке, настройках и страницах-заглушках. `header.js` самодостаточен (свой рендер аватара инлайн-стилями, чтение CSRF из cookie, logout, запрос `/api/v1/config` для ссылки на документацию), стили — `header.css`, подключён во всех shell (`tracker_shell`, `admin_shell`, `settings_shell`, `stub_shell`);
```

- [ ] **Step 2: Обновить «Выход» в `030-user-flows.md`**

Заменить строку:

```
- Пользователь нажимает «Выйти» в аккаунт-виджете (правый верхний угол).
```

на:

```
- Пользователь нажимает «Выйти» в гамбургер-меню (`☰` слева вверху → нижний блок «Аккаунт»).
```

- [ ] **Step 3: Обновить layout-строку**

Заменить строку:

```
- layout: тёмный sidebar слева (иерархия команд + выбор периода + аккаунт-виджет) и основная область справа;
```

на:

```
- layout: тёмный sidebar слева (гамбургер-меню `☰` + иерархия команд + выбор периода) и основная область справа;
```

- [ ] **Step 4: Обновить строку про аккаунт-виджет**

Заменить строку:

```
- аккаунт-виджет в правом верхнем углу sidebar (в трекере) и navbar (в admin) показывает аватар, имя, кнопку выхода и ссылку на `/admin` для администраторов;
```

на:

```
- гамбургер-меню (`☰`) слева вверху sidebar открывает drawer с разделами навигации («OKR Tracker», «Лог активностей», «Дерево целей») и аккаунт-блоком (аватар, имя, ⚙ «Настройки», «Документация», «Администрирование» для админов, «Выйти»); идентично на всех страницах;
```

- [ ] **Step 5: Добавить раздел про навигацию и заглушки**

В `030-user-flows.md` после раздела «### 3в. Персональные настройки — `/settings`» (перед следующим разделом) добавить:

```
### 3г. Глобальная навигация (гамбургер-меню)

Во всех страницах (трекер, настройки, админка, заглушки) слева вверху sidebar есть кнопка `☰`, открывающая выезжающий слева drawer — единый компонент `HeaderNavMenu` из `header.js`. Drawer закрывается по клику на фон, крестику или клавише Esc.

- **Разделы:** «OKR Tracker» → `/`, «Лог активностей» → `/activity-log`, «Дерево целей» → `/goal-tree`. Активный раздел подсвечивается по текущей странице (на `/settings` и `/admin*` не подсвечен ни один).
- **Аккаунт (низ):** карточка профиля (аватар + имя + email) с кнопкой ⚙ → `/settings`; «Документация» (только если задан `documentation_url` из `GET /api/v1/config`); «Администрирование» → `/admin` (только для `is_admin`); «Выйти» (POST `/logout`).

**Страницы-заглушки.** `/activity-log` и `/goal-tree` доступны любому авторизованному пользователю (как `/settings`) и отдают общий shell `stub-shell` + `stub.js`: тёмный sidebar с тем же гамбургер-меню и контент-плейсхолдер «Раздел в разработке». Реальная функциональность — вне текущего scope.
```

- [ ] **Step 6: Финальный чекпойнт**

Run: `go test ./... && go vet ./...`
Expected: зелёное. НЕ коммитить — остановиться для ревью.

---

## Notes для исполнителя

- **Коммиты не делать** — пользователь коммитит сам (правило CLAUDE.md). Чекпойнты = прогнать `go test ./...` + `go vet ./...` и остановиться.
- JS не покрыт юнит-тестами в проекте — для фронтенда верификация визуальная (Task 6, Step 4). Не вводить новый JS-тулчейн.
- `seed_demo.sql` и структура БД не меняются — миграции не нужны.
- Эмодзи-иконки (`🎯🕑🕸⚙📖🛠↩`) согласованы со стилем существующего `HeaderUserMenu`; при желании можно заменить на SVG, но это вне scope.
