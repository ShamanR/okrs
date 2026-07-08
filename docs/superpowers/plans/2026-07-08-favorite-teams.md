# Favorite Teams (sidebar) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Дать пользователю возможность помечать команды «избранными» и видеть их закреплённым плоским списком вверху сайдбара, полностью на клиенте.

**Architecture:** Избранное — массив id команд в `localStorage` (per-user, порядок добавления). Сайдбар получает под-блоки «Избранное» и «Все команды». Строки дерева (`SidebarNode`) получают всегда-видимую иконку-звезду для toggle. Блок избранного переиспользует `SidebarNode` с `children: []`. Никакого бэкенда.

**Tech Stack:** Preact + htm (глобальные `React`-подобные хелперы `useState/useEffect/useCallback`), plain JS в `internal/web/static/tracker.js`, CSS в `internal/web/static/tracker.css`. Тест-раннера в репозитории нет — чистые функции проверяются через `node -e`, UI — вручную в браузере.

## Global Constraints

- Только фронтенд. Не трогать backend, БД, `specs/*.md`, seed, API.
- Никаких новых сетевых запросов — всё из уже загруженной `hierarchy` и `localStorage`.
- Ключи `localStorage` — per-user, по образцу существующих: `readTreeExpanded`/`SETTINGS_SIDEBAR_KEY`.
- Устойчивость: битый JSON → `[]`; неизвестный id → молча пропускается, из хранилища не удаляется.
- Порядок избранного — по добавлению (новые в конец).
- Звезда всегда видима. Порядок в строке: имя → ★ → %.
- Коммиты делает пользователь (CLAUDE.md #8) — шагов `git commit` в плане нет; в конце каждой задачи — точка ручной проверки.
- Стиль кода и именование — как в соседнем коде (`sidebar__*`, `sidebar-node__*`, camelCase функции, `try/catch` вокруг `localStorage`).

---

## File Structure

- Modify: `internal/web/static/tracker.js`
  - Блок persistence (рядом со строками ~52–94): добавить `FAV_KEY`, `readFavorites`, `writeFavorites`, `toggleFavorite`, `collectFavNodes`.
  - `SidebarNode` (~1663): добавить пропсы `favSet`, `onToggleFav`; отрисовать звезду; пробросить пропсы в рекурсию.
  - `App` (~1915): state `favorites`, инициализация/запись, `onToggleFav`, производные `favSet`/`favNodes`; разметка под-блоков в `sidebar__tree` (~2129).
- Modify: `internal/web/static/tracker.css`
  - Добавить `.sidebar__section-label`, `.sidebar__subsection-label`, `.sidebar-node__star`, `.sidebar-node__star--on`.

---

### Task 1: Favorites — чистые функции хранения и сборки

**Files:**
- Modify: `internal/web/static/tracker.js` (вставить после блока `filterTreeForSidebar`, ~строка 94)

**Interfaces:**
- Consumes: ничего.
- Produces:
  - `const FAV_KEY = uid => `okr_fav_teams:${uid}``
  - `readFavorites(uid): string[]` — массив id; при отсутствии/битом/не-массиве → `[]`.
  - `writeFavorites(uid, ids): void` — `JSON.stringify`, обёрнут в `try/catch`.
  - `toggleFavorite(ids, id): string[]` — иммутабельно: если `id` есть — убрать, иначе добавить в конец.
  - `collectFavNodes(nodes, favIds): Node[]` — плоско обойти дерево, вернуть узлы для id из `favIds` **в порядке `favIds`**, отсутствующие пропустить.

- [ ] **Step 1: Реализовать функции**

Вставить в `tracker.js` сразу после `filterTreeForSidebar` (после строки с закрывающей `}` этой функции, ~94):

```js
// ── FAVORITE TEAMS PERSISTENCE ────────────────────────────────────────────────
// Per-user list of favorited team ids, in add-order. Client-only (no backend).
// Unknown ids are ignored at render time but kept in storage, so a favorite that
// temporarily drops out of the hierarchy (other period / lost access) returns
// when it reappears — same resilience contract as TREE_EXPANDED_KEY.
const FAV_KEY = uid => `okr_fav_teams:${uid}`;
// Team ids may arrive from the API as numbers or strings; favorites normalize
// every id to a string so storage lookups stay consistent even if a stored id's
// type differs from the current hierarchy's (e.g. after an id-type migration).
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
// Immutable toggle: remove if present, else append (keeps add-order). Ids are
// compared as strings so a numeric click matches a stored string id and back.
function toggleFavorite(ids, id) {
  const key = favId(id);
  return ids.includes(key) ? ids.filter(x => x !== key) : [...ids, key];
}
// Flatten the tree into an id->node map, then return nodes for favIds in favIds
// order. Missing ids are skipped (not rendered), never throw.
function collectFavNodes(nodes, favIds) {
  const byId = new Map();
  const walk = list => (list || []).forEach(n => { byId.set(favId(n.id), n); walk(n.children); });
  walk(nodes);
  return favIds.map(id => byId.get(favId(id))).filter(Boolean);
}
```

- [ ] **Step 2: Sanity-check чистых функций через node**

Скопировать три функции без `localStorage`-зависимостей в разовый скрипт и проверить:

Run:
```bash
node -e '
const toggleFavorite=(ids,id)=>ids.includes(id)?ids.filter(x=>x!==id):[...ids,id];
const collectFavNodes=(nodes,favIds)=>{const m=new Map();const w=l=>(l||[]).forEach(n=>{m.set(n.id,n);w(n.children)});w(nodes);return favIds.map(i=>m.get(i)).filter(Boolean)};
console.assert(JSON.stringify(toggleFavorite([],"a"))===JSON.stringify(["a"]),"add");
console.assert(JSON.stringify(toggleFavorite(["a","b"],"a"))===JSON.stringify(["b"]),"remove");
console.assert(JSON.stringify(toggleFavorite(["a"],"b"))===JSON.stringify(["a","b"]),"append order");
const tree=[{id:"1",children:[{id:"2",children:[]},{id:"3",children:[]}]}];
console.assert(collectFavNodes(tree,["3","1"]).map(n=>n.id).join()==="3,1","order + nested");
console.assert(collectFavNodes(tree,["3","x","1"]).map(n=>n.id).join()==="3,1","skip missing");
console.log("OK");
'
```
Expected: печатает `OK`, ни одного `Assertion failed`.

- [ ] **Step 3: Проверка синтаксиса файла**

Run: `node --check internal/web/static/tracker.js`
Expected: без ошибок (пустой вывод, код 0).

**Deliverable:** Чистые функции добавлены и проверены; файл парсится.

---

### Task 2: Звезда в строке дерева (`SidebarNode`)

**Files:**
- Modify: `internal/web/static/tracker.js` (`SidebarNode`, ~1663–1690)
- Modify: `internal/web/static/tracker.css`

**Interfaces:**
- Consumes: `favSet: Set<string>`, `onToggleFav: (id:string)=>void` (передаются из `App`, Task 3).
- Produces: `SidebarNode` рендерит `.sidebar-node__star` и пробрасывает `favSet`/`onToggleFav` в рекурсивные дочерние `SidebarNode`.

- [ ] **Step 1: Добавить пропсы и звезду в `SidebarNode`**

Заменить сигнатуру и тело. Текущий блок (строки ~1663–1690) переписать так — добавлены `favSet`, `onToggleFav`, элемент звезды между именем и прогрессом, и проброс в рекурсию:

```js
function SidebarNode({ node, depth, selectedId, onSelect, expanded, toggle, accent, behindMargin, greenThreshold, favSet, onToggleFav }) {
  const ch = node.children || [];
  const isExp = expanded[node.id] !== false;
  const isSel = selectedId === node.id;
  const prog = node.progress;
  const dotC = TEAM_TYPE_COLOR[node.type] || HEALTH_COLOR.no_goals;
  const pctC = sidebarProgressColor(prog, node.forecast, node.status, behindMargin, greenThreshold);
  const pad = 14 + depth * 13;
  const isFav = favSet && favSet.has(favId(node.id));
  const nameClass = ['sidebar-node__name',
    depth === 0 ? 'sidebar-node__name--d0' : depth === 1 ? 'sidebar-node__name--d1' : 'sidebar-node__name--dx',
    isSel ? 'sidebar-node__name--selected' : '',
  ].filter(Boolean).join(' ');
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
        {onToggleFav && <span
          onClick={e => { e.stopPropagation(); onToggleFav(node.id); }}
          className={`sidebar-node__star${isFav ? ' sidebar-node__star--on' : ''}`}
          title={isFav ? 'Убрать из избранного' : 'В избранное'}>{isFav ? '★' : '☆'}</span>}
        {prog != null && <span className="sidebar-node__progress" style={{ color: isSel ? '#c4b5fd' : pctC }}>{prog}%</span>}
      </div>
      {isExp && ch.map(c => <SidebarNode key={c.id} node={c} depth={depth + 1} selectedId={selectedId} onSelect={onSelect} expanded={expanded} toggle={toggle} accent={accent} behindMargin={behindMargin} greenThreshold={greenThreshold} favSet={favSet} onToggleFav={onToggleFav} />)}
    </div>
  );
}
```

- [ ] **Step 2: Стили звезды**

Добавить в `tracker.css` после правила `.sidebar-node__progress` (после строки 55):

```css
.sidebar-node__star { flex-shrink: 0; font-size: 13px; line-height: 1; color: #475569; cursor: pointer; padding: 0 2px; }
.sidebar-node__row:hover .sidebar-node__star { color: #64748b; }
.sidebar-node__star:hover { color: #fbbf24; }
.sidebar-node__star--on { color: #fbbf24; }
```

- [ ] **Step 3: Проверка синтаксиса**

Run: `node --check internal/web/static/tracker.js`
Expected: без ошибок.

**Deliverable:** `SidebarNode` умеет показывать звезду и звать `onToggleFav`; при отсутствии `onToggleFav` (обратная совместимость) звезда не рендерится. Визуально проверяется после Task 3.

---

### Task 3: Wiring в `App` + под-блоки сайдбара

**Files:**
- Modify: `internal/web/static/tracker.js` (`App` state ~1924, разметка `sidebar__tree` ~2129–2144)

**Interfaces:**
- Consumes: `readFavorites`, `writeFavorites`, `toggleFavorite`, `collectFavNodes` (Task 1); `SidebarNode` с `favSet`/`onToggleFav` (Task 2); существующие `hierarchy`, `me`, `selId`, `selectTeam`, `expanded`, `toggle`, `accent`, `behindMargin`, `greenThreshold`, `filterTreeForSidebar`, `readSidebarSelection`.
- Produces: рабочий UI избранного.

- [ ] **Step 1: State и хендлер избранного**

В `App`, рядом с `const [expanded, setExpanded] = useState(readTreeExpanded);` (строка ~1924) добавить:

```js
  const [favorites, setFavorites] = useState(null); // null = not loaded from storage yet
```

Сентинел `null` (а не `[]`) важен: он позволяет write-эффекту отличать «ещё не
загружено» от «загружено и пусто» и не затирать сохранённое значение пустым
массивом в том же commit (см. Step 2).

- [ ] **Step 2: Инициализация из localStorage при появлении `me`**

`favorites` инициализируется `null` (при первом рендере `me` ещё нет). Добавить эффект сразу после эффекта `useEffect(() => { writeTreeExpanded(expanded); }, [expanded]);` (строка ~2043):

```js
  // Load favorites once the user id is known (favorites === null → not loaded),
  // then persist on every change. Guarding the write on the `null` sentinel (not a
  // ref) avoids a same-commit race that could clobber stored favorites with [].
  useEffect(() => { if (me && favorites === null) setFavorites(readFavorites(me.id)); }, [me, favorites]);
  useEffect(() => { if (me && favorites !== null) writeFavorites(me.id, favorites); }, [favorites, me]);
  const onToggleFav = useCallback(id => setFavorites(f => toggleFavorite(f || [], id)), []);
```

(`useCallback` уже используется в файле — импорт не нужен.) Ref-подход был бы
багом: ref флипается синхронно, поэтому write-эффект в том же commit увидел бы
«загружено» при ещё пустом state и затёр бы `localStorage` пустым массивом
(поймано на browser-verification — favorites не переживали reload).

- [ ] **Step 3: Производные значения перед `return`**

Найти `return (` разметки App (строка ~2118). Непосредственно перед ним добавить:

```js
  const favArr = favorites || [];
  const favSet = new Set(favArr);
  const favNodes = collectFavNodes(hierarchy, favArr);
  const visibleTree = filterTreeForSidebar(hierarchy, readSidebarSelection(me?.id), selId);
```

- [ ] **Step 4: Разметка под-блоков**

Заменить блок `<div className="sidebar__tree">…</div>` (строки ~2129–2144) на:

```jsx
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
                <div className="sidebar__section-label">Команды</div>
                {favNodes.length > 0 && <>
                  <div className="sidebar__subsection-label"><span className="sidebar__subsection-star">★</span> Избранное · {favNodes.length}</div>
                  {favNodes.map(n => <SidebarNode key={`fav-${n.id}`} node={{ ...n, children: [] }} depth={0} selectedId={selId} onSelect={selectTeam} expanded={expanded} toggle={toggle} accent={accent} behindMargin={behindMargin} greenThreshold={greenThreshold} favSet={favSet} onToggleFav={onToggleFav} />)}
                  <div className="sidebar__subsection-label">Все команды</div>
                </>}
                {visibleTree.map(n => <SidebarNode key={n.id} node={n} depth={0} selectedId={selId} onSelect={selectTeam} expanded={expanded} toggle={toggle} accent={accent} behindMargin={behindMargin} greenThreshold={greenThreshold} favSet={favSet} onToggleFav={onToggleFav} />)}
              </>
          }
        </div>
```

- [ ] **Step 5: Проверка синтаксиса**

Run: `node --check internal/web/static/tracker.js`
Expected: без ошибок.

**Deliverable:** Полностью подключённый UI. Проверяется в браузере в Task 5.

---

### Task 4: Стили секций сайдбара

**Files:**
- Modify: `internal/web/static/tracker.css`

**Interfaces:**
- Consumes: классы из Task 3 (`sidebar__section-label`, `sidebar__subsection-label`, `sidebar__subsection-star`).
- Produces: компактная визуальная структура блока команд.

- [ ] **Step 1: Добавить правила**

Добавить в `tracker.css` после `.sidebar__tree { … }` (после строки 19):

```css
.sidebar__section-label { font-size: 10px; color: #64748b; font-weight: 700; text-transform: uppercase; letter-spacing: 0.6px; padding: 4px 14px 2px; }
.sidebar__subsection-label { font-size: 10px; color: #64748b; font-weight: 600; text-transform: uppercase; letter-spacing: 0.5px; padding: 6px 14px 2px; display: flex; align-items: center; gap: 5px; }
.sidebar__subsection-star { color: #fbbf24; font-size: 11px; }
```

- [ ] **Step 2: Проверить, что sidebar не начал скроллиться по горизонтали**

Визуально (в Task 5): звезда и `%` помещаются в строку, длинные имена обрезаются `…` (правило `.sidebar-node__name` уже с `text-overflow: ellipsis`).

**Deliverable:** Секции выглядят как на скрине: серые uppercase-лейблы, жёлтая звезда у «Избранное», компактные отступы.

---

### Task 5: Ручная проверка сценариев в браузере

**Files:** нет (проверка).

**Interfaces:**
- Consumes: собранное приложение из Task 1–4.

- [ ] **Step 1: Запустить приложение**

Run: `go run ./cmd/server` (точка входа — `cmd/server/main.go`; при необходимости поднять зависимости через `docker-compose up -d`). Открыть страницу трекера в браузере, войти под демо-пользователем.

Expected: сайдбар показывает «Команды» → «Все команды» с деревом; у каждой строки контурная звезда `☆`.

- [ ] **Step 2: Добавление/удаление**

Кликнуть звезду у нескольких команд разного уровня.
Expected: звезда становится `★` (жёлтая); команда появляется в блоке «Избранное · N» вверху плоским списком; счётчик N растёт. Повторный клик (в дереве или в блоке избранного) убирает её, N уменьшается. Клик по звезде не меняет выбранную команду; клик по строке избранного — выбирает команду.

- [ ] **Step 3: Порядок = порядок добавления**

Добавить A, затем B, затем C.
Expected: в блоке порядок A, B, C. Убрать A, добавить снова — A уходит в конец (B, C, A).

- [ ] **Step 4: Персистентность**

Перезагрузить страницу.
Expected: избранное и порядок сохранились.

- [ ] **Step 5: Пропажа команды (устойчивость)**

Переключить период на тот, где избранная команда отсутствует.
Expected: она скрыта из блока «Избранное», ошибок в консоли нет; при возврате в период с этой командой — снова видна. N всегда = числу реально показанных.

- [ ] **Step 6: Пустое избранное и битые данные**

Убрать все избранные.
Expected: под-блок «Избранное» и лейбл «Все команды» исчезают, остаётся «Команды» + дерево.

В DevTools задать `localStorage` ключу `okr_fav_teams:<uid>` значение `"{"` (битый JSON) и перезагрузить.
Expected: страница работает, избранное пустое, ошибок нет.

**Deliverable:** Все сценарии из спеки пройдены вручную; консоль без ошибок.

---

## Self-Review

**Spec coverage:**
- Хранение (client-only, per-user, add-order) → Task 1 (`FAV_KEY`, `read/writeFavorites`, `toggleFavorite`) + Task 3 (state/persist).
- Устойчивость к пропаже → Task 1 (`collectFavNodes` skip-missing, `readFavorites` fallback) + Task 3 (счётчик по видимым) + Task 5 (Steps 5–6).
- UI: блок «Команды», под-блоки «Избранное»/«Все команды», скрытие пустого → Task 3 Step 4.
- Звезда всегда видима, имя→★→%, toggle со stopPropagation → Task 2.
- Переиспользование `SidebarNode` для избранного (`children: []`, depth 0) → Task 3 Step 4.
- «Избранное» на полной `hierarchy` (до фильтра) → Task 3 Step 3 (`collectFavNodes(hierarchy, …)`, отдельно от `visibleTree`).
- Стили → Task 2 Step 2 + Task 4.
- Тестирование → Task 1 node-проверка + Task 5 ручные сценарии.
- Никаких изменений backend/specs → соблюдено (файлы только `tracker.js`, `tracker.css`).

**Placeholder scan:** нет TBD/TODO; весь код приведён целиком.

**Type consistency:** `favSet: Set` / `onToggleFav(id)` совпадают между Task 2 (потребление) и Task 3 (передача). `collectFavNodes(nodes, favIds)`, `toggleFavorite(ids, id)`, `readFavorites(uid)`, `writeFavorites(uid, ids)` — имена и сигнатуры едины в Task 1 и Task 3. Ключ `okr_fav_teams:${uid}` един.
