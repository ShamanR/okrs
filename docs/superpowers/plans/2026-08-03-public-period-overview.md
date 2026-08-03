# Публичный раздел «Обзор периода» (admin-only) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Добавить в основное приложение раздел «Обзор периода» (виден в сайдбаре только админам), визуально повторяющий обзор периода из админ-панели, с выбором периода выпадающим списком в сайдбаре.

**Architecture:** Переиспользуем готовые admin-эндпоинты (overview/activate/close) под тем же admin-гейтом — новых API нет. Выносим `PeriodSelect` из `tracker.js` и презентационный обзор из `admin.js` в общие файлы, подключаемые в оба шелла. Новый бандл-страница `period-overview.js` собирает: сайдбар с дропдауном периода + шапка + общий компонент обзора.

**Tech Stack:** Go (chi, html/template, embed), React через JSX (in-browser Babel, без сборки).

## Global Constraints

- Source of truth — specs: `README-specs.md`, `specs/010`…`specs/050`. Правки specs — в том же change set (в этом плане серверная логика не меняется, специи не трогаем).
- Чистая/слоистая архитектура; не протекать абстракциями между слоями.
- Без запросов в базу в цикле.
- Никаких git-коммитов от имени агента — коммитит пользователь сам (шаги «Commit» ручные).
- Не упоминать Claude/AI/ассистентов в коде/комментариях/спеках.
- Дизайн-доки и спеки — на русском; комментарии в коде — в стиле окружающего кода.
- Раздел строго **admin-only**: серверный `RequireTenantAdminMiddleware` + клиентский `adminOnly`/`cfg.is_admin`.
- Переиспользуемые эндпоинты (не менять): `GET /api/v1/admin/periods/{id}/overview`, `POST …/teams/activate`, `POST …/teams/close`, `GET /api/v1/periods`, `GET /api/v1/me`, `GET /api/v1/config`.
- Порядок плиток статусов (единый в обоих местах, жизненный цикл): `no_goals` → `forming` → `ready` → `in_progress` → `closed`.
- `apiGet`/`apiPost` в admin-бандле возвращают `Response` (нужен `.json()`/`.ok`). В новом бандле `apiGet` возвращает уже распарсенный JSON (как в `activity.js`) — см. код задач.

**Спека:** `docs/superpowers/specs/2026-08-03-public-period-overview-design.md`

**JSX-проверка (используется в шагах ниже):** Babel установлен в скретчпаде. Команда проверки компиляции:

```bash
SP=/private/tmp/claude-502/-Users-lakosnikov-pavel-work-github-com-okrs/34316bdf-ead2-48da-802b-5305b4ee8375/scratchpad
NODE_PATH=$SP/node_modules node -e '
const b=require("@babel/core"),fs=require("fs");
process.argv.slice(1).forEach(f=>{try{b.transformSync(fs.readFileSync(f,"utf8"),{presets:["@babel/preset-react"]});console.log("JSX_OK",f)}catch(e){console.log("JSX_ERR",f,e.message)}});
' <files...>
```

Если скретчпад недоступен — установить разово: `cd "$SP" && npm i @babel/core @babel/preset-react`.

---

### Task 1: Вынести `PeriodSelect` в общий `period_select.js`

**Files:**
- Create: `internal/web/static/period_select.js`
- Modify: `internal/web/static/tracker.js` (удалить перенесённые определения)
- Modify: `internal/http/templates/tracker_shell.html` (подключить `period_select.js` перед `tracker.js`)

**Interfaces:**
- Consumes: React-хуки (`useState`, `useRef` — глобальны в бандле), CSS-классы `period-select__*` из `tracker.css`.
- Produces (глобальные символы бандла): `TRK_PERIOD_STATUS`, `fmtPeriodDate(iso)`, `fmtDateRange(a,b)`, `PeriodSelect({periods, periodId, onChange})`.

- [ ] **Step 1: Создать `period_select.js` с перенесённым кодом**

Скопируй из `tracker.js` блок «PERIOD SELECT» (строки `const TRK_PERIOD_STATUS = { … }` … по конец `function PeriodSelect(...) { … }`) в новый файл. Содержимое:

```jsx
// Общий компонент выбора периода (сайдбар). Используется трекером и обзором периода.
// CSS-классы period-select__* — в tracker.css.
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
        <div className="period-select__group">Актуальные и будущие</div>
        {periods.map(p => {
          const s = TRK_PERIOD_STATUS[p.status] || TRK_PERIOD_STATUS.closed;
          return <button key={p.id} type="button"
            className={'period-select__item' + (p.id === periodId ? ' is-selected' : '')}
            onClick={() => { onChange(p.id); setOpen(false); }}>
            <span className="period-select__indent" style={{ width: p.depth * 12 }} />
            <span className="period-select__dot" style={{ background: s.dot }} />
            <span className="period-select__item-name">{p.name}</span>
            <span className="period-select__range">{fmtDateRange(p.start_date, p.end_date)}</span>
            <span className="period-select__badge-wrap"><span className="period-select__badge" style={{ background: s.bg, color: s.fg }}>{s.label}</span></span>
          </button>;
        })}
      </div>
    </>}
  </div>;
}
```

- [ ] **Step 2: Удалить перенесённый блок из `tracker.js`**

Удали из `internal/web/static/tracker.js` те же строки (от комментария `// ── PERIOD SELECT ───…` / `const TRK_PERIOD_STATUS = {` до конца `function PeriodSelect(...) {...}` включительно). Остальной `tracker.js` не трогай — он продолжит использовать эти символы (теперь из `period_select.js`).

- [ ] **Step 3: Проверить, что дублей не осталось**

Run:
```bash
grep -n "function PeriodSelect\|const TRK_PERIOD_STATUS\|function fmtPeriodDate\|function fmtDateRange" internal/web/static/tracker.js
```
Expected: пусто (все определения теперь только в `period_select.js`). Использования `PeriodSelect`/`fmtDateRange` в `tracker.js` остаются.

- [ ] **Step 4: Подключить `period_select.js` в tracker-shell перед `tracker.js`**

В `internal/http/templates/tracker_shell.html` добавь строку перед `tracker.js`:

```html
<script type="text/babel" src="/static/sidebar.js" data-presets="react"></script>
<script type="text/babel" src="/static/period_select.js" data-presets="react"></script>
<script type="text/babel" src="/static/tracker.js" data-presets="react"></script>
```

- [ ] **Step 5: Проверить компиляцию JSX**

Run (подставь путь скретчпада из шапки):
```bash
NODE_PATH=$SP/node_modules node -e '...' internal/web/static/period_select.js internal/web/static/tracker.js
```
Expected: `JSX_OK` для обоих файлов.

- [ ] **Step 6: Commit** (пользователь вручную)

```bash
git add internal/web/static/period_select.js internal/web/static/tracker.js internal/http/templates/tracker_shell.html
git commit -m "web: extract PeriodSelect into shared period_select.js"
```

---

### Task 2: Общий компонент обзора `period_overview_view.js` + рефактор админ-модалки

**Files:**
- Create: `internal/web/static/period_overview_view.js`
- Modify: `internal/web/static/admin.js` (рефактор `PeriodOverviewModal`, удалить локальные `STATUS_TILES` и дубли тела)
- Modify: `internal/http/templates/admin_shell.html` (подключить `period_overview_view.js` перед `admin.js`)

**Interfaces:**
- Consumes: `ACCENT` из `ui.js` (в обоих шеллах грузится раньше).
- Produces (глобальные символы бандла): `PO_STATUS_TILES`, `PeriodOverviewContent({data, busy, onApply})` — рендерит плитки статусов, плитки качества, inline drill-down и карточки массовых операций. `onApply(ep)` где `ep ∈ {"activate","close"}`.

- [ ] **Step 1: Создать `period_overview_view.js`**

Самодостаточный по стилю компонент (значения цветов скопированы из admin `T`, чтобы вид не менялся). Порядок плиток — жизненный цикл.

```jsx
// Общий презентационный компонент обзора периода — тело для админ-модалки и
// публичной страницы «Обзор периода». Данные и колбэк массовых операций
// прокидывает контекст. Стили самодостаточны (не зависят от admin.js T/Btn).
const PO = {
  cardBorder: '#e5e7eb', hairline: '#f1f5f9', headingFg: '#0f172a',
  mutedFg: '#6b7280', dimFg: '#9ca3af', accent: ACCENT, danger: '#b91c1c',
};
// Порядок жизненного цикла: нет целей → черновик → к валидации → в работе → закрыто.
const PO_STATUS_TILES = [
  { key: 'no_goals',    label: 'Нет целей',   dot: '#cbd5e1', color: '#64748b' },
  { key: 'forming',     label: 'Черновик',    dot: '#f59e0b', color: '#92400e' },
  { key: 'ready',       label: 'К валидации', dot: '#3b82f6', color: '#1e40af' },
  { key: 'in_progress', label: 'В работе',    dot: '#22c55e', color: '#166534' },
  { key: 'closed',      label: 'Закрыто',     dot: '#9ca3af', color: '#4b5563' },
];

function POPrimaryBtn({ disabled, onClick, children }) {
  return <button type="button" onClick={onClick} disabled={disabled}
    style={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'center', padding: '8px 16px',
      borderRadius: 20, border: '1.5px solid ' + PO.accent, background: PO.accent, color: 'white',
      fontSize: 13, fontWeight: 600, cursor: disabled ? 'not-allowed' : 'pointer', opacity: disabled ? .5 : 1,
      whiteSpace: 'nowrap' }}>{children}</button>;
}

function PeriodOverviewContent({ data, busy, onApply }) {
  const [drill, setDrill] = useState(null); // {title, teams:[...]}
  if (!data) return <div style={{ padding: '40px 22px', textAlign: 'center', color: PO.mutedFg }}>Загрузка обзора…</div>;
  const s = data.summary;
  const allTeams = data.teams || [];

  const teamsByStatus = k => allTeams.filter(t => t.status === k || (k === 'in_progress' && t.status === 'validated'));
  const teamsWithErr = () => allTeams.filter(t => t.weight_error);
  const teamsWithGoals = () => allTeams.filter(t => t.goals_count > 0);

  const affectActivate = teamsWithGoals().filter(t => t.status !== 'in_progress' && t.status !== 'validated').length;
  const affectClose = teamsWithGoals().filter(t => t.status !== 'closed').length;
  const skipNoGoals = s.total_teams - s.teams_with_goals;

  const tile = (label, value, sub, accent, onClick) =>
    <div onClick={onClick} style={{ flex: '1 1 150px', minWidth: 140, background: 'white', border: '1px solid ' + PO.cardBorder, borderRadius: 12, padding: '14px 16px', cursor: onClick ? 'pointer' : 'default' }}>
      <div style={{ fontSize: 12, color: PO.mutedFg, fontWeight: 600 }}>{label}</div>
      <div style={{ fontSize: 26, fontWeight: 800, color: accent || PO.headingFg, marginTop: 4 }}>{value}</div>
      {sub && <div style={{ fontSize: 11, color: PO.dimFg, marginTop: 2 }}>{sub}</div>}
    </div>;

  return <div style={{ padding: '18px 22px' }}>
    <div style={{ fontSize: 11, color: PO.dimFg, fontWeight: 700, textTransform: 'uppercase', letterSpacing: .5, marginBottom: 10 }}>Команды по статусам · всего {s.total_teams}</div>
    <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap' }}>
      {PO_STATUS_TILES.map(st => tile(
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}><span style={{ width: 7, height: 7, borderRadius: 999, background: st.dot }} />{st.label}</span>,
        (s.by_status && s.by_status[st.key]) || 0, 'показать состав', st.color,
        () => setDrill({ title: st.label, teams: teamsByStatus(st.key) })
      ))}
    </div>

    <div style={{ fontSize: 11, color: PO.dimFg, fontWeight: 700, textTransform: 'uppercase', letterSpacing: .5, margin: '18px 0 10px' }}>Качество и результат</div>
    <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap' }}>
      {tile('Команды с целями', `${s.teams_with_goals}/${s.total_teams}`, 'только они участвуют в массовых операциях', PO.accent, () => setDrill({ title: 'Команды с целями', teams: teamsWithGoals() }))}
      {tile('Ошибки весов', s.weight_error_count, 'сумма весов целей ≠ 100%', '#b91c1c', () => setDrill({ title: 'Ошибки весов', teams: teamsWithErr() }))}
      {tile('Средний прогресс', `${s.avg_progress}%`, `по ${s.teams_with_goals} командам с целями`, PO.accent)}
    </div>

    {drill && <div style={{ marginTop: 14, border: '1px solid ' + PO.cardBorder, borderRadius: 12, overflow: 'hidden' }}>
      <div style={{ padding: '10px 14px', background: '#f8fafc', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <span style={{ fontSize: 12.5, fontWeight: 700, color: PO.headingFg }}>{drill.title} · {drill.teams.length}</span>
        <button onClick={() => setDrill(null)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: PO.mutedFg, fontSize: 16 }}>×</button>
      </div>
      <div style={{ maxHeight: 220, overflowY: 'auto' }}>
        {drill.teams.length === 0
          ? <div style={{ padding: '16px', textAlign: 'center', color: PO.dimFg, fontSize: 12.5 }}>Пусто</div>
          : drill.teams.map(t => <div key={t.id} style={{ display: 'flex', justifyContent: 'space-between', gap: 10, padding: '8px 14px', borderTop: '1px solid ' + PO.hairline, fontSize: 12.5 }}>
              <span style={{ color: PO.headingFg, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{(t.path || []).join(' › ') || t.name}</span>
              <span style={{ color: t.weight_error ? PO.danger : PO.mutedFg, flexShrink: 0 }}>{t.goals_count > 0 ? `${t.progress}% · веса ${t.weight_sum}` : 'нет целей'}</span>
            </div>)}
      </div>
    </div>}

    <div style={{ fontSize: 11, color: PO.dimFg, fontWeight: 700, textTransform: 'uppercase', letterSpacing: .5, margin: '18px 0 10px' }}>Массовые операции · только админ</div>
    <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
      <div style={{ border: '1px solid ' + PO.cardBorder, borderRadius: 12, padding: '14px 16px', display: 'flex', alignItems: 'center', gap: 14 }}>
        <div style={{ flex: 1 }}>
          <div style={{ fontSize: 13.5, fontWeight: 700, color: PO.headingFg }}>Перевести все команды в «В работе»</div>
          <div style={{ fontSize: 12, color: PO.mutedFg, marginTop: 3 }}>Затронет только команды, у которых есть хотя бы одна цель в этом периоде. Цели блокируются от редактирования, остаётся обновление прогресса.</div>
        </div>
        <div style={{ fontSize: 11.5, color: PO.dimFg, textAlign: 'right' }}>затронет {affectActivate}<br />пропустим {skipNoGoals} без целей</div>
        <POPrimaryBtn disabled={busy || affectActivate === 0} onClick={() => onApply('activate')}>Применить</POPrimaryBtn>
      </div>
      <div style={{ border: '1px solid ' + PO.cardBorder, borderRadius: 12, padding: '14px 16px', display: 'flex', alignItems: 'center', gap: 14 }}>
        <div style={{ flex: 1 }}>
          <div style={{ fontSize: 13.5, fontWeight: 700, color: PO.headingFg }}>Закрыть цели всех команд периода</div>
          <div style={{ fontSize: 12, color: PO.mutedFg, marginTop: 3 }}>Команды без целей не трогаем. У остальных статус становится «Закрыто» — доступны только комментарии.</div>
        </div>
        <div style={{ fontSize: 11.5, color: PO.dimFg, textAlign: 'right' }}>затронет {affectClose}<br />пропустим {skipNoGoals} без целей</div>
        <POPrimaryBtn disabled={busy || affectClose === 0} onClick={() => onApply('close')}>Применить</POPrimaryBtn>
      </div>
    </div>
  </div>;
}
```

- [ ] **Step 2: Рефактор `PeriodOverviewModal` в `admin.js`**

Замени в `internal/web/static/admin.js` весь блок `const STATUS_TILES = [ … ];` и функцию `function PeriodOverviewModal(...) { … }` (строки ~433–546) на версию, где тело делегируется в общий компонент, а шапка и `Modal`-логика остаются:

```jsx
// Модалка «Управление периодом»: шапка + общий компонент обзора (period_overview_view.js).
function PeriodOverviewModal({period, onEdit, onDelete, reload}) {
  const [data, setData] = useState(null);
  const [busy, setBusy] = useState(false);

  const load = () => apiGet(`/api/v1/admin/periods/${period.id}/overview`).then(r => r && r.json()).then(setData).catch(()=>{});
  useEffect(() => { load(); }, [period.id]);

  async function onApply(ep) {
    if (busy) return;
    setBusy(true);
    try {
      const res = await apiPost(`/api/v1/admin/periods/${period.id}/teams/${ep}`, {});
      if (!res || !res.ok) { alert('Ошибка операции'); return; }
      await load();
      reload();
    } finally { setBusy(false); }
  }

  return <div>
    <div style={{padding:'18px 22px',borderBottom:'1px solid '+T.hairline,display:'flex',alignItems:'flex-start',gap:16}}>
      <div style={{flex:1}}>
        <div style={{fontSize:20,fontWeight:800,color:T.headingFg}}>{period.name}</div>
        <div style={{fontSize:12.5,color:T.mutedFg,marginTop:4,display:'flex',alignItems:'center',gap:10}}>
          <span style={{fontFamily:'ui-monospace,Menlo,monospace'}}>{fmtDateShort(period.start_date)} — {fmtDateShort(period.end_date)}</span>
          <PeriodBadge status={period.status}/>
        </div>
      </div>
      <Btn onClick={onEdit}>Редактировать</Btn>
      <Btn danger onClick={onDelete}>Удалить</Btn>
    </div>
    <PeriodOverviewContent data={data} busy={busy} onApply={onApply}/>
  </div>;
}
```

Примечание: `PeriodMetrics` в `admin.js` (метрики строк) НЕ трогаем — это отдельный компонент. Удаляется только `STATUS_TILES` и старое тело `PeriodOverviewModal`.

- [ ] **Step 3: Проверить, что в `admin.js` не осталось ссылок на удалённое**

Run:
```bash
grep -n "STATUS_TILES\|setDrill\|affectActivate\|affectClose" internal/web/static/admin.js
```
Expected: пусто (эти символы теперь только в `period_overview_view.js`).

- [ ] **Step 4: Подключить `period_overview_view.js` в admin-shell перед `admin.js`**

В `internal/http/templates/admin_shell.html`:

```html
<script type="text/babel" src="/static/sidebar.js" data-presets="react"></script>
<script type="text/babel" src="/static/period_overview_view.js" data-presets="react"></script>
<script type="text/babel" src="/static/admin.js" data-presets="react"></script>
```

- [ ] **Step 5: Проверить компиляцию JSX**

Run:
```bash
NODE_PATH=$SP/node_modules node -e '...' internal/web/static/period_overview_view.js internal/web/static/admin.js
```
Expected: `JSX_OK` для обоих.

- [ ] **Step 6: Commit** (пользователь вручную)

```bash
git add internal/web/static/period_overview_view.js internal/web/static/admin.js internal/http/templates/admin_shell.html
git commit -m "web: extract shared PeriodOverviewContent; admin modal reuses it"
```

---

### Task 3: Страница `period-overview.js`

**Files:**
- Create: `internal/web/static/period-overview.js`

**Interfaces:**
- Consumes: `Sidebar` (sidebar.js), `PeriodSelect` (period_select.js), `PeriodOverviewContent` (period_overview_view.js), `ReactDOM`, React-хуки.
- Produces: смонтированное SPA-приложение страницы `/period-overview`.

- [ ] **Step 1: Создать `period-overview.js`**

По образцу `activity.js` (`apiGet` возвращает распарсенный JSON). Выбор периода по умолчанию — текущий активный (самый короткий активный по датам), иначе первый.

```jsx
const { useState, useEffect } = React;

async function apiFetch(url, opts = {}) {
  const r = await fetch(url, opts);
  if (!r.ok) {
    if (r.status === 401) { location.href = '/login?next=' + encodeURIComponent(location.pathname); return null; }
    throw new Error(`HTTP ${r.status}`);
  }
  return r.status === 204 ? null : r.json();
}
const apiGet = url => apiFetch(url);
const apiPostJSON = (url, body) => apiFetch(url, { method: 'POST', headers: csrfHeaders(), body: JSON.stringify(body || {}) });

// Текущий активный период = самый короткий активный по датам (как FindPeriodForDate).
function pickDefaultPeriodId(periods) {
  if (!periods || periods.length === 0) return null;
  const active = periods.filter(p => p.status === 'active');
  if (active.length === 0) return periods[0].id;
  const dur = p => new Date(p.end_date) - new Date(p.start_date);
  active.sort((a, b) => (dur(a) - dur(b)) || (new Date(b.end_date) - new Date(a.end_date)));
  return active[0].id;
}

function App() {
  const [me, setMe] = useState(null);
  const [periods, setPeriods] = useState([]);
  const [periodId, setPeriodId] = useState(null);
  const [data, setData] = useState(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState(false);

  useEffect(() => {
    Promise.all([apiGet('/api/v1/me'), apiGet('/api/v1/periods')]).then(([meData, per]) => {
      if (meData) setMe(meData);
      const items = (per && per.items) || [];
      setPeriods(items);
      setPeriodId(pickDefaultPeriodId(items));
    }).catch(() => setErr(true));
  }, []);

  const load = (pid) => {
    if (!pid) return;
    setData(null); setErr(false);
    apiGet(`/api/v1/admin/periods/${pid}/overview`).then(d => setData(d)).catch(() => setErr(true));
  };
  useEffect(() => { load(periodId); }, [periodId]);

  async function onApply(ep) {
    if (busy || !periodId) return;
    setBusy(true);
    try {
      const res = await apiPostJSON(`/api/v1/admin/periods/${periodId}/teams/${ep}`, {});
      if (res === undefined) { /* apiFetch threw handled below */ }
      load(periodId);
    } catch { alert('Ошибка операции'); }
    finally { setBusy(false); }
  }

  const cur = periods.find(p => p.id === periodId);
  const total = data && data.summary ? data.summary.total_teams : null;

  return (
    <div className="app-layout">
      <Sidebar
        user={me}
        active="period-overview"
        beforeSections={
          <div className="sidebar__period">
            <div className="sidebar__period-label">Период</div>
            <PeriodSelect periods={periods} periodId={periodId} onChange={setPeriodId} />
          </div>
        }
      />
      <main className="app-main" style={{ padding: '24px 28px', overflow: 'auto' }}>
        {periods.length === 0
          ? <div style={{ color: '#6b7280', fontSize: 14 }}>Периодов пока нет.</div>
          : <>
              <div style={{ display: 'flex', alignItems: 'baseline', gap: 12, marginBottom: 18 }}>
                <div style={{ fontSize: 24, fontWeight: 800, color: '#0f172a', letterSpacing: '-.4px' }}>{cur ? cur.name : '—'}</div>
                {total != null && <div style={{ fontSize: 14, color: '#6b7280' }}>{total} команд в вашем доступе</div>}
              </div>
              {err
                ? <div style={{ color: '#b91c1c', fontSize: 13 }}>Не удалось загрузить обзор периода.</div>
                : <div style={{ background: 'white', border: '1px solid #e5e7eb', borderRadius: 14, boxShadow: '0 1px 3px rgba(15,23,42,0.04)', overflow: 'hidden' }}>
                    <PeriodOverviewContent data={data} busy={busy} onApply={onApply} />
                  </div>}
            </>}
      </main>
    </div>
  );
}

ReactDOM.createRoot(document.getElementById('root')).render(<App />);
```

Note: `csrfHeaders` — глобаль из `api.js` (грузится раньше). Класс `app-layout`/`app-main` — проверь по факту, как `activity.js` оборачивает `Sidebar` + контент; если у activity другой контейнер-класс, используй такой же (см. Step 2).

- [ ] **Step 2: Сверить layout-обёртку с `activity.js`**

Run:
```bash
sed -n '470,509p' internal/web/static/activity.js
```
Возьми тот же корневой контейнер/класс, что оборачивает `<Sidebar/>` + основной контент в `activity.js`, и приведи обёртку в `period-overview.js` к нему (замени `app-layout`/`app-main`, если у activity иначе).

- [ ] **Step 3: Проверить компиляцию JSX**

Run:
```bash
NODE_PATH=$SP/node_modules node -e '...' internal/web/static/period-overview.js
```
Expected: `JSX_OK`.

- [ ] **Step 4: Commit** (пользователь вручную)

```bash
git add internal/web/static/period-overview.js
git commit -m "web: period-overview page bundle"
```

---

### Task 4: Шелл, маршрут, пункт сайдбара, тест шаблона

**Files:**
- Create: `internal/http/templates/period_overview_shell.html`
- Modify: `internal/http/server.go` (маршрут `/period-overview` в `registerAdminRoutes`)
- Modify: `internal/web/static/sidebar.js` (пункт `SIDEBAR_SECTIONS`)
- Modify: `internal/http/templates_test.go` (добавить shell в проверки + отдельный тест)

**Interfaces:**
- Consumes: `s.tmpl` (parseTemplates глобит `templates/*.html`), `s.shellData()`, `RequireTenantAdminMiddleware`.
- Produces: страница `GET /period-overview` (admin-gated), пункт сайдбара `period-overview`.

- [ ] **Step 1: Создать шелл `period_overview_shell.html`**

По образцу `activity_shell.html`; грузит `period_select.js` и `period_overview_view.js` перед `period-overview.js`:

```html
{{define "period-overview-shell"}}
<!DOCTYPE html>
<html lang="ru">
<head>
{{template "spa-head" .}}
<title>OKR Tracker · Обзор периода</title>
<link rel="stylesheet" href="/static/tracker.css">
<link rel="stylesheet" href="/static/markdown.css">
<link rel="stylesheet" href="/static/components.css">
<link rel="stylesheet" href="/static/sidebar.css">
</head>
<body>
<div id="root"><div class="loading-screen">Загрузка…</div></div>
{{template "spa-vendor" .}}
<script type="text/babel" src="/static/api.js" data-presets="react"></script>
<script type="text/babel" src="/static/storage.js" data-presets="react"></script>
<script type="text/babel" src="/static/ui.js" data-presets="react"></script>
<script type="text/babel" src="/static/sidebar.js" data-presets="react"></script>
<script type="text/babel" src="/static/period_select.js" data-presets="react"></script>
<script type="text/babel" src="/static/period_overview_view.js" data-presets="react"></script>
<script type="text/babel" src="/static/period-overview.js" data-presets="react"></script>
</body>
</html>
{{end}}
```

- [ ] **Step 2: Зарегистрировать маршрут в `registerAdminRoutes`**

В `internal/http/server.go`, рядом с `/activity-log` (внутри группы под `RequireTenantAdminMiddleware`), добавь:

```go
		r.Get("/period-overview", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_ = s.tmpl.ExecuteTemplate(w, "period-overview-shell", s.shellData())
		})
```

- [ ] **Step 3: Добавить пункт сайдбара перед `activity-log`**

В `internal/web/static/sidebar.js`, в массив `SIDEBAR_SECTIONS`, перед элементом `activity-log`:

```jsx
  { id: 'goal-tree',    label: 'Дерево целей',    href: '/goal-tree',    icon: '🕸' },
  { id: 'period-overview', label: 'Обзор периода', href: '/period-overview', icon: '📊', adminOnly: true },
  { id: 'activity-log', label: 'Лог активностей', href: '/activity-log', icon: '🕑', adminOnly: true },
```

- [ ] **Step 4: Добавить проверку шелла в `templates_test.go`**

В `internal/http/templates_test.go`: в `TestShellSharedPartials` добавь `"period-overview-shell"` в срез `shells`. И добавь отдельный тест:

```go
func TestPeriodOverviewShellRenders(t *testing.T) {
	out := renderShell(t, "period-overview-shell", shellData{})
	for _, want := range []string{`/static/sidebar.js`, `/static/period_select.js`, `/static/period_overview_view.js`, `/static/period-overview.js`, `id="root"`} {
		if !strings.Contains(out, want) {
			t.Errorf("period-overview-shell missing %q", want)
		}
	}
}
```

- [ ] **Step 5: Собрать и прогнать тесты шаблонов**

Run:
```bash
go build ./... && go test ./internal/http/ -run 'Shell|Template' -v
```
Expected: PASS (включая `TestPeriodOverviewShellRenders` и `TestShellSharedPartials`).

- [ ] **Step 6: Проверить компиляцию JSX сайдбара**

Run:
```bash
NODE_PATH=$SP/node_modules node -e '...' internal/web/static/sidebar.js
```
Expected: `JSX_OK`.

- [ ] **Step 7: Commit** (пользователь вручную)

```bash
git add internal/http/templates/period_overview_shell.html internal/http/server.go internal/web/static/sidebar.js internal/http/templates_test.go
git commit -m "web: /period-overview admin route, shell, sidebar entry"
```

---

### Task 5: Визуальная проверка в запущенном приложении

**Files:** —

- [ ] **Step 1: Запустить приложение**

```bash
docker compose up -d db
export DATABASE_URL=postgres://postgres:postgres@localhost:5432/okrs?sslmode=disable PORT=8080
go run ./cmd/server
```
При необходимости засеять демо: `docker compose exec -T db psql -U postgres -d okrs < seed_demo.sql`.

- [ ] **Step 2: Проверить как админ**

Открой `http://localhost:8080/period-overview`. Ожидается:
- Пункт «Обзор периода» 📊 виден в сайдбаре (раздел «Разделы»), перед «Лог активностей».
- В сайдбаре выпадающий список «Период»; по умолчанию — текущий активный (самый короткий активный по датам).
- Плитки статусов в порядке: Нет целей → Черновик → К валидации → В работе → Закрыто; сумма = всего.
- Плитки «Команды с целями / Ошибки весов / Средний прогресс».
- Клик по плитке → drill-down состава.
- Смена периода в дропдауне → обзор перезагружается, шапка «`<Имя>` · `N` команд в вашем доступе» обновляется.
- Массовые операции «Применить» работают; счётчики после операции обновляются.
- Вид совпадает с админ-модалкой на `/admin/periods` (та же вёрстка тела).

- [ ] **Step 3: Проверить, что раздел скрыт у не-админа и трекер не сломался**

- Под обычным пользователем (или при `AUTH_MODE=enabled` с не-админ аккаунтом) пункт «Обзор периода» в сайдбаре отсутствует, а прямой заход на `/period-overview` даёт `403`.
- Открой трекер `/` — выпадающий список периода в сайдбаре работает как раньше (регресс после выноса `PeriodSelect`).

---

## Self-Review

**Spec coverage:**
- Раздел в основном приложении, admin-only, в сайдбаре → Task 4 (маршрут под `RequireTenantAdminMiddleware`, пункт `adminOnly`). ✅
- Визуально повторяет админ-обзор (общий компонент) → Task 2 (`PeriodOverviewContent`, admin-модалка рефакторится на него). ✅
- Период выбирается выпадающим списком в сайдбаре (существующий компонент) → Task 1 (вынос `PeriodSelect`) + Task 3 (`beforeSections` + `PeriodSelect`). ✅
- По умолчанию текущий активный период → Task 3 (`pickDefaultPeriodId`). ✅
- Переиспользование admin-эндпоинтов, без новых API → Task 3 (`/api/v1/admin/periods/{id}/overview` + bulk). ✅
- Порядок плиток — жизненный цикл, единый в обоих местах → Task 2 (`PO_STATUS_TILES`). ✅
- Пустые состояния / ошибки → Task 3 (нет периодов / ошибка загрузки). ✅
- Тест шелла → Task 4. ✅
- Трекер не ломается после выноса → Task 1 Step 3 + Task 5 Step 3. ✅

**Placeholder scan:** конкретный код/команды в каждом шаге. Единственные «подстановки» — путь скретчпада `$SP` (задан в шапке) и сверка layout-обёртки с `activity.js` (Task 3 Step 2, с явной командой). Не плейсхолдеры логики. ✅

**Type consistency:** `PeriodOverviewContent({data, busy, onApply})` — одинаково в Task 2 (определение), Task 2 Step 2 (admin) и Task 3 (страница). `PeriodSelect({periods, periodId, onChange})` — Task 1 (определение), Task 3 (использование). `pickDefaultPeriodId(periods)` — Task 3. Шелл-имя `period-overview-shell` — Task 4 Steps 1/2/4 согласованы. Порядок скриптов: `period_select.js` и `period_overview_view.js` грузятся до `period-overview.js` (Task 4 Step 1) и до `admin.js` (Task 2 Step 4) / `tracker.js` (Task 1 Step 4). ✅

**Риск-заметки исполнителю:**
- `apiGet` в новом бандле возвращает распарсенный JSON (как в activity.js), а в admin.js — `Response`. Код задач это учитывает; не смешивать.
- Точную корневую обёртку (`app-layout`/`app-main`) сверить с `activity.js` (Task 3 Step 2).
- Визуальная проверка (Task 5) требует запущенного Postgres; если Docker недоступен — выполнить позже, остальные задачи проверяются `go test`/JSX-компиляцией.
