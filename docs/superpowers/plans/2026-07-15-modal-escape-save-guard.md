# Закрытие модалок по Escape с guard несохранённых изменений — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Все модальные окна приложения закрываются по Escape; при несохранённых изменениях перед закрытием показывается окно-подтверждение (Enter — сохранить, повторный Escape — выйти без сохранения).

**Architecture:** Общий хук `useModalClose` и презентационный компонент `<ModalCloseConfirm>` в `ui.js` инкапсулируют клавиатуру, стек модалок, блокировку скролла и state machine подтверждения. Tracker-модалки подключают хук напрямую (они сами владеют оверлеем); общий admin-`Modal` делегирует закрытие в хук тела редактора через `closeRef`. Вложенные выпадашки при открытом дропдауне «съедают» Escape через `e.preventDefault()`, а хук такие события пропускает.

**Tech Stack:** React 18 (глобальный, без бандлера), Babel Standalone (компиляция JSX в браузере), голые глобали между скриптами (общий global lexical scope), Go-сервер отдаёт статику из `internal/web/static`.

## Global Constraints

- **Нет шага сборки.** Статические `.js` отдаются как есть и компилируются Babel в браузере (`type="text/babel" data-presets="react"`). Проверка изменений = обновление страницы; ошибки парсинга видны в консоли браузера. Автотестов фронтенда в проекте нет — верификация ручная по клик-пути.
- **Коммиты делает пользователь** (CLAUDE.md #8). Агент НЕ выполняет `git commit`. Каждая задача завершается ручной верификацией, а не коммитом.
- **`ui.js` грузится раньше `admin.js`/`tracker.js`** в общем global lexical scope, где те объявляют `const {useState,...} = React;`. Поэтому в `ui.js` хуки вызываются ТОЛЬКО как `React.useState` / `React.useEffect` / `React.useRef` / `React.useCallback` — без top-level деструктуризации (иначе `SyntaxError: Identifier 'useState' has already been declared`).
- **Токены дизайна** (`--accent`, `--border`, `--text`, `--text-faint`) доступны на обеих страницах через `tokens.css` (грузится в `spa-head`). Значение `--accent` = `#7c3aed` (= `ACCENT` в `ui.js`); в CSS всегда указывать fallback `var(--accent, #7c3aed)`.
- **Семантика кнопки «Отмена».** Явная кнопка «Отмена»/«Cancel» в футере формы остаётся прямым discard (`onClose`) — guard не показывает. Guard срабатывает только на неявные/случайные закрытия: **Escape, крестик ×, клик по оверлею**.
- **Русский язык** во всех текстах UI, комментариях, доке (CLAUDE.md #11).

---

## Файловая структура

- `internal/web/static/ui.js` — **добавить** `useModalClose`, `<ModalCloseConfirm>`, модульный стек модалок и scroll-lock. Дом общей логики (уже грузится на tracker/admin/settings/activity).
- `internal/web/static/components.css` — **добавить** классы `.modal-confirm-*` (общие стили окна-подтверждения; грузится на tracker и admin).
- `internal/web/static/tracker.js` — **изменить** `KRProgressModal`, `KREditModal`, `GoalModal`, `ConfirmModal`, `TeamCombobox`, user-combobox.
- `internal/web/static/admin.js` — **изменить** `Modal`, `PeriodModalBody`, `TeamEditor`, `UserModal`, `TeamCombobox` (admin).

---

## Task 1: Общий хук `useModalClose` + `<ModalCloseConfirm>` + стили

**Files:**
- Modify: `internal/web/static/ui.js` (добавить в конец файла, после строки 8)
- Modify: `internal/web/static/components.css` (добавить в конец файла)

**Interfaces:**
- Produces:
  - `useModalClose({ isDirty?: boolean = false, canSave?: boolean = true, onSave?: () => void, onClose: () => void }) => { requestClose: () => void, confirming: boolean, confirmEl: ReactElement|null }`
  - `ModalCloseConfirm({ canSave?: boolean, onSave: ()=>void, onDiscard: ()=>void, onCancel: ()=>void })` — используется хуком внутренне.
  - Модульные: `__modalStack` (массив токенов), scroll-lock с ref-count. Не экспортируются наружу — детали реализации.

- [ ] **Step 1: Добавить хук и компонент в `ui.js`**

Добавь в конец `internal/web/static/ui.js` (после существующей строки 8):

```jsx

// ── ОБЩЕЕ ПОВЕДЕНИЕ ЗАКРЫТИЯ МОДАЛОК ────────────────────────────────────────────
// useModalClose — единое поведение всех модальных окон приложения:
//   • Escape / крестик / клик по оверлею → requestClose();
//   • нет изменений (isDirty=false) → закрытие сразу;
//   • есть изменения → окно-подтверждение «Сохранить изменения?»:
//       Enter → onSave (если canSave), повторный Escape → закрытие без сохранения.
// На клавиши реагирует ТОЛЬКО верхняя модалка стека (вложенные ConfirmModal и т.п.).
// События с defaultPrevented игнорируются: вложенные выпадашки, «съедающие» Escape,
// вызывают e.preventDefault() и потому не закрывают модалку.
// Внимание: ui.js грузится раньше tracker.js/admin.js в общем global-scope, где те
// объявляют `const {useState,...}=React`, поэтому здесь только React.* (без деструктуризации).
const __modalStack = [];
let __modalScrollCount = 0;
let __modalPrevOverflow = '';
function __modalLockScroll() {
  if (__modalScrollCount++ === 0) {
    __modalPrevOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
  }
}
function __modalUnlockScroll() {
  if (--__modalScrollCount <= 0) {
    __modalScrollCount = 0;
    document.body.style.overflow = __modalPrevOverflow || '';
  }
}

function useModalClose({ isDirty = false, canSave = true, onSave, onClose }) {
  const [confirming, setConfirming] = React.useState(false);
  const tokenRef = React.useRef(null);
  if (tokenRef.current === null) tokenRef.current = {};
  // Свежие значения для document-listener без переподписки на каждый рендер.
  const stateRef = React.useRef({});
  stateRef.current = { isDirty, canSave, onSave, onClose, confirming, setConfirming };

  const requestClose = React.useCallback(() => {
    const s = stateRef.current;
    if (s.confirming) return;
    if (s.isDirty) s.setConfirming(true);
    else s.onClose();
  }, []);

  React.useEffect(() => {
    const token = tokenRef.current;
    __modalStack.push(token);
    __modalLockScroll();
    const onKey = e => {
      if (e.defaultPrevented) return;
      if (__modalStack[__modalStack.length - 1] !== token) return;
      const s = stateRef.current;
      if (s.confirming) {
        if (e.key === 'Enter') { e.preventDefault(); if (s.canSave) { s.setConfirming(false); s.onSave && s.onSave(); } }
        else if (e.key === 'Escape') { e.preventDefault(); s.onClose(); }
      } else if (e.key === 'Escape') {
        e.preventDefault();
        if (s.isDirty) s.setConfirming(true);
        else s.onClose();
      }
    };
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('keydown', onKey);
      const i = __modalStack.indexOf(token);
      if (i !== -1) __modalStack.splice(i, 1);
      __modalUnlockScroll();
    };
  }, []);

  const confirmEl = confirming
    ? <ModalCloseConfirm
        canSave={canSave}
        onSave={() => { setConfirming(false); if (canSave && onSave) onSave(); }}
        onDiscard={onClose}
        onCancel={() => setConfirming(false)} />
    : null;

  return { requestClose, confirming, confirmEl };
}

// Окно-подтверждение несохранённых изменений. Стили — .modal-confirm-* в components.css.
function ModalCloseConfirm({ canSave = true, onSave, onDiscard, onCancel }) {
  const down = React.useRef(false);
  return (
    <div className="modal-confirm-overlay"
      onMouseDown={e => { down.current = e.target === e.currentTarget; }}
      onMouseUp={e => { const c = down.current && e.target === e.currentTarget; down.current = false; if (c) onCancel(); }}>
      <div className="modal-confirm-box" onClick={e => e.stopPropagation()}>
        <div className="modal-confirm-title">Сохранить изменения?</div>
        <div className="modal-confirm-message">В форме есть несохранённые изменения. Сохранить их перед закрытием?</div>
        <div className="modal-confirm-actions">
          <button type="button" onClick={onCancel} className="modal-confirm-btn">Продолжить редактирование</button>
          <button type="button" onClick={onDiscard} className="modal-confirm-btn">Не сохранять</button>
          <button type="button" onClick={onSave} disabled={!canSave} autoFocus
            className="modal-confirm-btn modal-confirm-btn--primary">Сохранить</button>
        </div>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Добавить стили `.modal-confirm-*` в `components.css`**

Добавь в конец `internal/web/static/components.css`:

```css

/* Окно-подтверждение несохранённых изменений (useModalClose / ui.js).
   Общее для tracker и admin; z-index выше любых модалок приложения.
   --accent = #7c3aed (fallback на случай отсутствия tokens.css). */
.modal-confirm-overlay { position: fixed; inset: 0; z-index: 9000; display: flex; align-items: center; justify-content: center; padding: 20px; background: rgba(15, 23, 42, 0.45); }
.modal-confirm-box { width: min(420px, 94vw); background: #fff; border-radius: 14px; box-shadow: 0 24px 60px rgba(15, 23, 42, 0.3); padding: 22px 24px 18px; animation: modalConfirmIn .14s ease-out; }
@keyframes modalConfirmIn { from { opacity: 0; transform: translateY(-8px) scale(.98); } to { opacity: 1; transform: none; } }
.modal-confirm-title { font-size: 16px; font-weight: 800; color: var(--text, #0f172a); letter-spacing: -.2px; }
.modal-confirm-message { font-size: 13px; color: var(--text-faint, #6b7280); line-height: 1.5; margin-top: 8px; }
.modal-confirm-actions { display: flex; flex-wrap: wrap; gap: 8px; justify-content: flex-end; margin-top: 18px; }
.modal-confirm-btn { padding: 8px 16px; border-radius: 9px; border: 1px solid var(--border, #e5e7eb); background: #fff; color: var(--text, #374151); font-size: 13px; font-weight: 600; font-family: inherit; cursor: pointer; }
.modal-confirm-btn:hover { background: #f8fafc; }
.modal-confirm-btn--primary { border-color: transparent; background: var(--accent, #7c3aed); color: #fff; }
.modal-confirm-btn--primary:hover { background: #6d28d9; }
.modal-confirm-btn--primary:disabled { background: #e5e7eb; color: #9ca3af; cursor: default; }
```

- [ ] **Step 3: Запустить приложение и проверить отсутствие ошибок**

Запусти приложение (skill `run` или как в README) и открой трекер-страницу. Открой консоль браузера (DevTools).
Ожидается: страница загружается штатно, в консоли НЕТ ошибок парсинга Babel и НЕТ `useState has already been declared` / `ModalCloseConfirm is not defined`. Модалки пока работают по-старому (консумеров ещё нет) — это ок.

- [ ] **Step 4: Верификация завершена, коммит — за пользователем**

Изменения не коммить. Сообщи, что Task 1 готов к ревью.

---

## Task 2: Мигрировать `ConfirmModal` (tracker) на `useModalClose`

Простейший консумер: без формы, Escape просто закрывает. Даёт guard-стеку рабочую точку для вложенности.

**Files:**
- Modify: `internal/web/static/tracker.js:899-917` (`ConfirmModal`)

**Interfaces:**
- Consumes: `useModalClose` (Task 1), существующий `useOverlayClose(onClose)`.

- [ ] **Step 1: Подключить хук в `ConfirmModal`**

Найди строку 902:

```jsx
  const overlay = useOverlayClose(onClose);
```

Замени на:

```jsx
  const { requestClose } = useModalClose({ isDirty: false, onClose });
  const overlay = useOverlayClose(requestClose);
```

(Остальное тело `ConfirmModal` не трогаем: у него нет крестика и редактируемых полей, футер «Отмена» уже вызывает `onClose`.)

- [ ] **Step 2: Проверить в браузере**

Обнови страницу трекера. У любого KR нажми удаление → откроется `ConfirmModal` «Удалить Key Result?».
- Нажми **Escape** → окно закрывается (раньше Escape не работал).
- Открой снова, кликни по затемнённому фону вне окна → закрывается.
- Открой снова, «Отмена» → закрывается; «Удалить» → удаляет.

Ожидается: все четыре сценария закрывают окно; никаких окон-подтверждений сохранения (правок нет).

- [ ] **Step 3: Готово к ревью (без коммита).**

---

## Task 3: Мигрировать `KRProgressModal` (guard по dirty)

**Files:**
- Modify: `internal/web/static/tracker.js:541-696` (`KRProgressModal`)

**Interfaces:**
- Consumes: `useModalClose`, `useOverlayClose`. Существующие в компоненте: `form`, `note`, `descDraft`, `descEditing`, `saving`, `save`, `kr`, `onClose`.

- [ ] **Step 1: Вычислить `isDirty` и подключить хук**

Найди строку 548:

```jsx
  const overlay = useOverlayClose(onClose);
```

Замени на:

```jsx
  const initialNote = kr.note?.text ?? '';
  const dirtyProgress = (() => {
    if (form.krType === 'NUMERICAL') return String(form.current) !== String(kr.current);
    if (form.krType === 'BOOLEAN') return !!form.done !== !!kr.done;
    if (form.krType === 'PROJECT') return (form.stages || []).some((s, i) => !!s.done !== !!((kr.stages || [])[i] || {}).done);
    return false;
  })();
  const isDirty = dirtyProgress || note.trim() !== initialNote.trim() || (descEditing && descDraft.trim() !== '');
  const { requestClose, confirmEl } = useModalClose({ isDirty, canSave: !saving, onSave: save, onClose });
  const overlay = useOverlayClose(requestClose);
```

- [ ] **Step 2: Крестик → `requestClose`**

Найди строку 584:

```jsx
          <button onClick={onClose} className="modal-close">×</button>
```

Замени на:

```jsx
          <button onClick={requestClose} className="modal-close">×</button>
```

- [ ] **Step 3: Отрендерить `confirmEl`**

Найди конец компонента (строки 576-596 — начало `return`, и его закрытие около строки 695):

```jsx
  return (
    <div className="modal-overlay modal-overlay--z300" {...overlay}>
```

Замени на:

```jsx
  return (
    <>
    <div className="modal-overlay modal-overlay--z300" {...overlay}>
```

Затем найди закрывающие теги в конце `return` (строки ~693-696):

```jsx
      </div>
    </div>
  );
}
```

Замени на:

```jsx
      </div>
    </div>
    {confirmEl}
    </>
  );
}
```

(Проверь по структуре: это именно закрытие `KRProgressModal`, сразу перед комментарием `// ── KR EDIT MODAL ──`. Внешний `<div className="modal-overlay">` закрывается вторым `</div>`.)

- [ ] **Step 4: Проверить в браузере**

Обнови страницу. Открой «Обновить прогресс» у KR.
- Escape без правок → закрывается сразу.
- Измени значение/шаг/заметку, нажми Escape → появляется окно «Сохранить изменения?».
- В окне: **Enter** → сохраняет и закрывает (значение обновилось в списке).
- Снова измени, Escape → окно; **повторный Escape** → закрывается без сохранения (значение НЕ изменилось).
- Снова измени, Escape → окно; «Продолжить редактирование» → возврат к форме с сохранением введённого.

Ожидается: все сценарии работают; при отсутствии правок окно-подтверждение не показывается.

- [ ] **Step 5: Готово к ревью (без коммита).**

---

## Task 4: Мигрировать `KREditModal` (guard по dirty + canSave)

**Files:**
- Modify: `internal/web/static/tracker.js:699-896` (`KREditModal`)

**Interfaces:**
- Consumes: `useModalClose`, `useOverlayClose`. Существующие: `form`, `kr`, `isNew`, `saving`, `save`, `canSave`, `onClose`.

- [ ] **Step 1: Вычислить `isDirty` и подключить хук**

Найди строки 745-746:

```jsx
  const canSave = !saving && !!form.name.trim();
  const overlay = useOverlayClose(onClose);
```

Замени на:

```jsx
  const canSave = !saving && !!form.name.trim();
  const initialFormRef = useRef(null);
  if (initialFormRef.current === null) initialFormRef.current = JSON.stringify(form);
  const isDirty = JSON.stringify(form) !== initialFormRef.current;
  const { requestClose, confirmEl } = useModalClose({ isDirty, canSave, onSave: save, onClose });
  const overlay = useOverlayClose(requestClose);
```

- [ ] **Step 2: Крестик → `requestClose`**

Найди строку 752:

```jsx
          <button onClick={onClose} className="modal-close">×</button>
```

Замени на:

```jsx
          <button onClick={requestClose} className="modal-close">×</button>
```

- [ ] **Step 3: Отрендерить `confirmEl`**

Найди строки 747-748:

```jsx
  return (
    <div className="modal-overlay modal-overlay--z300" {...overlay}>
```

Замени на:

```jsx
  return (
    <>
    <div className="modal-overlay modal-overlay--z300" {...overlay}>
```

Найди закрытие компонента (строки ~893-896, перед `// ── CONFIRM MODAL ──`):

```jsx
      </div>
    </div>
  );
}
```

Замени на:

```jsx
      </div>
    </div>
    {confirmEl}
    </>
  );
}
```

- [ ] **Step 4: Проверить в браузере**

Обнови страницу. Открой «Редактировать KR» (или «Добавить KR»).
- Escape без правок → закрывается сразу.
- Измени название, Escape → окно «Сохранить изменения?»; Enter → сохраняет.
- Очисти название полностью (canSave=false), измени что-то ещё, Escape → окно; кнопка **«Сохранить» неактивна**, Enter ничего не сохраняет; повторный Escape → выход без сохранения.

Ожидается: guard срабатывает на правках; при невалидной форме «Сохранить» задизейблена.

- [ ] **Step 5: Готово к ревью (без коммита).**

---

## Task 5: Мигрировать `GoalModal` + preventDefault в tracker-выпадашках

**Files:**
- Modify: `internal/web/static/tracker.js:1516-1697` (`GoalModal`)
- Modify: `internal/web/static/tracker.js:454` (`TeamCombobox.onKey`, Escape)
- Modify: `internal/web/static/tracker.js:1362` (user-combobox `onKey`, Escape)

**Interfaces:**
- Consumes: `useModalClose`, `useOverlayClose`. Существующие: `form`, `saving`, `save`, `performSave`, `valid`, `canSave`, `confirmUnshare`, `onClose`. Вложенный `ConfirmModal` уже мигрирован (Task 2) — он корректно перехватывает Escape как верхний в стеке.

- [ ] **Step 1: Вычислить `isDirty` и подключить хук**

Найди строки 1584-1585:

```jsx
  const canSave = valid && !saving;
  const overlay = useOverlayClose(onClose);
```

Замени на:

```jsx
  const canSave = valid && !saving;
  const initialFormRef = useRef(null);
  if (initialFormRef.current === null) initialFormRef.current = JSON.stringify(form);
  const isDirty = JSON.stringify(form) !== initialFormRef.current;
  const { requestClose, confirmEl } = useModalClose({ isDirty, canSave, onSave: save, onClose });
  const overlay = useOverlayClose(requestClose);
```

- [ ] **Step 2: Крестик → `requestClose`**

Найди строку 1594:

```jsx
          <button onClick={onClose} className="modal-close modal-close--lg">×</button>
```

Замени на:

```jsx
          <button onClick={requestClose} className="modal-close modal-close--lg">×</button>
```

- [ ] **Step 3: Отрендерить `confirmEl`**

Найди строки 1586-1587:

```jsx
  return (
    <div className="modal-overlay modal-overlay--z400" {...overlay}>
```

Замени на:

```jsx
  return (
    <>
    <div className="modal-overlay modal-overlay--z400" {...overlay}>
```

Найди конец компонента (строки 1694-1697 — закрытие после блока `{confirmUnshare && (...)}`):

```jsx
      )}
    </div>
  );
}
```

Замени на:

```jsx
      )}
    </div>
    {confirmEl}
    </>
  );
}
```

(Внешний `<div className="modal-overlay modal-overlay--z400">` — это первый div после фрагмента; блок `confirmUnshare` и его `ConfirmModal` остаются внутри него без изменений.)

- [ ] **Step 4: `TeamCombobox` (tracker) — preventDefault при открытом дропдауне**

Найди строку 454:

```jsx
    else if (e.key === 'Escape') setOpen(false);
```

Замени на:

```jsx
    else if (e.key === 'Escape') { if (open) { e.preventDefault(); setOpen(false); } }
```

- [ ] **Step 5: User-combobox (tracker) — preventDefault при открытом дропдауне**

Найди строку 1362:

```jsx
    else if (e.key === 'Escape') setOpen(false);
```

Замени на:

```jsx
    else if (e.key === 'Escape') { if (open) { e.preventDefault(); setOpen(false); } }
```

- [ ] **Step 6: Проверить в браузере**

Обнови страницу. Открой «Добавить цель» / «Редактировать цель».
- Escape без правок → закрывается сразу.
- Измени название, Escape → окно «Сохранить изменения?»; Enter → создаёт/сохраняет цель.
- В поле «Общая цель» открой выпадашку команд (`TeamCombobox`), нажми **Escape** → закрывается ТОЛЬКО дропдаван, модалка остаётся. То же для селектора владельцев.
- Для уже общей цели сними тумблер «Общая» и нажми «Создать/Сохранить» → вложенный `ConfirmModal` «Сделать цель не общей?»; в нём **Escape** закрывает только его (GoalModal остаётся).

Ожидается: guard, вложенность и Escape в выпадашках работают корректно.

- [ ] **Step 7: Готово к ревью (без коммита).**

---

## Task 6: Рефактор admin `Modal` (делегирование) + `PeriodModalBody`

**Files:**
- Modify: `internal/web/static/admin.js:114-143` (`Modal`)
- Modify: `internal/web/static/admin.js:483-528` (`PeriodModalBody`)
- Modify: `internal/web/static/admin.js:396-480` (`PeriodsSection` — `closeRef` и проброс `guarded`)

**Interfaces:**
- Produces: `Modal` получает опциональные пропсы `guarded?: boolean = false`, `closeRef?: React.Ref`. При `guarded` — Modal НЕ ставит свой keydown/scroll-lock (их ведёт `useModalClose` в теле); ×/оверлей вызывают `closeRef.current()`.
- Consumes: `useModalClose` (Task 1).

- [ ] **Step 1: Рефактор `Modal` — делегирование закрытия**

Замени весь компонент `Modal` (строки 114-143) на:

```jsx
function Modal({open, title, subtitle, onClose, children, width=640, guarded=false, closeRef}) {
  // Закрываем по оверлею только если и нажатие, и отпускание мыши были на нём самом —
  // иначе выделение текста с выносом курсора за пределы окна закрывало бы модалку.
  const downOnOverlay=useRef(false);
  // guarded — закрытием (×, оверлей, Escape) управляет useModalClose в теле редактора
  // через closeRef.current (=requestClose): он покажет guard несохранённых изменений.
  const fire=()=>{ if (guarded && closeRef && closeRef.current) closeRef.current(); else onClose(); };
  const onMouseDown=e=>{downOnOverlay.current=e.target===e.currentTarget;};
  const onMouseUp=e=>{const close=downOnOverlay.current&&e.target===e.currentTarget;downOnOverlay.current=false;if(close)fire();};
  useEffect(()=>{
    if (!open) return;
    // Когда guarded — keydown и блокировку скролла ведёт useModalClose (в теле).
    if (guarded) return;
    const h=e=>{if(e.key==='Escape')onClose();};
    document.addEventListener('keydown',h);
    const prev=document.body.style.overflow; document.body.style.overflow='hidden';
    return()=>{document.removeEventListener('keydown',h);document.body.style.overflow=prev;};
  },[open,onClose,guarded]);
  if (!open) return null;
  return <div onMouseDown={onMouseDown} onMouseUp={onMouseUp} style={{position:'fixed',inset:0,background:'rgba(15,23,42,0.42)',backdropFilter:'blur(2px)',zIndex:2000,display:'flex',alignItems:'flex-start',justifyContent:'center',padding:'28px 24px 48px',overflow:'auto'}}>
    {/* overflow:visible — чтобы выпадающие списки (напр. выбор родителя) не обрезались рамкой окна */}
    <div onClick={e=>e.stopPropagation()} style={{width:'100%',maxWidth:width,background:'white',borderRadius:14,boxShadow:'0 24px 60px rgba(15,23,42,0.28)',overflow:'visible',animation:'admModalIn .16s ease-out'}}>
      <div style={{padding:'16px 22px 14px',borderBottom:'1px solid '+T.hairline,display:'flex',alignItems:'flex-start',gap:14}}>
        <div style={{flex:1,minWidth:0}}>
          <div style={{fontSize:16,fontWeight:800,color:T.headingFg,letterSpacing:'-.2px'}}>{title}</div>
          {subtitle&&<div style={{fontSize:12,color:T.mutedFg,marginTop:3}}>{subtitle}</div>}
        </div>
        <button onClick={fire} style={{width:30,height:30,borderRadius:8,border:'1px solid '+T.cardBorder,background:'white',color:T.mutedFg,cursor:'pointer',fontSize:15,lineHeight:1,padding:0,flexShrink:0}}
          onMouseEnter={e=>{e.currentTarget.style.background='#f8fafc';}}
          onMouseLeave={e=>{e.currentTarget.style.background='white';}}>×</button>
      </div>
      <div>{children}</div>
    </div>
  </div>;
}
```

- [ ] **Step 2: `PeriodModalBody` — dirty + хук + confirmEl**

Найди строку 484 (сигнатуру) и строку 492-493:

```jsx
function PeriodModalBody({modal, saving, onSave, onClose, onDelete}) {
```

Замени сигнатуру на:

```jsx
function PeriodModalBody({modal, saving, onSave, onClose, onDelete, closeRef}) {
```

Найди строки 492-493:

```jsx
  const [f, setF] = useState(initial);
  const canSave = f.name.trim() && f.start_date && f.end_date;
```

Замени на:

```jsx
  const [f, setF] = useState(initial);
  const canSave = f.name.trim() && f.start_date && f.end_date;
  const isDirty = JSON.stringify(f) !== JSON.stringify(initial);
  const { requestClose, confirmEl } = useModalClose({ isDirty, canSave: !!canSave && !saving, onSave: () => { if (canSave) onSave(f); }, onClose });
  useEffect(() => { if (closeRef) closeRef.current = requestClose; }, [closeRef, requestClose]);
```

Найди закрытие тела (строки 520-522):

```jsx
      <Btn variant="primary" onClick={()=>canSave&&onSave(f)} disabled={!canSave||saving}>{saving ? 'Сохранение…' : isNew ? 'Создать' : 'Сохранить'}</Btn>
    </div>
  </div>;
}
```

Замени на:

```jsx
      <Btn variant="primary" onClick={()=>canSave&&onSave(f)} disabled={!canSave||saving}>{saving ? 'Сохранение…' : isNew ? 'Создать' : 'Сохранить'}</Btn>
    </div>
    {confirmEl}
  </div>;
}
```

(«Отмена» на строке 519 остаётся `onClose` — явный discard, guard не показывает.)

- [ ] **Step 3: `PeriodsSection` — создать `closeRef`, передать в `Modal` и тело**

Найди строку 397 (внутри `PeriodsSection`, рядом с `const [saving, setSaving]`):

```jsx
  const [saving, setSaving] = useState(false);
```

Добавь сразу под ней:

```jsx
  const periodCloseRef = useRef(null);
```

Найди `Modal`-рендер (строки 468-480) и замени на:

```jsx
    <Modal open={!!modal}
      title={modal && modal.mode==='new' ? 'Новый период' : 'Редактировать период'}
      subtitle={modal && modal.mode==='new'
        ? 'Заполните название и даты. Статус вычисляется по датам.'
        : 'Измените название и даты. Статус вычисляется по датам.'}
      onClose={()=>setModal(null)} width={560} guarded closeRef={periodCloseRef}>
      {modal && <PeriodModalBody
        key={modal.mode==='edit' ? 'edit-'+modal.period.id : 'new-'+(modal.parent?modal.parent.id:'root')}
        modal={modal} saving={saving}
        onSave={save} onClose={()=>setModal(null)} closeRef={periodCloseRef}
        onDelete={modal.mode==='edit' ? ()=>remove(modal.period) : null}/>}
    </Modal>
```

- [ ] **Step 4: Проверить в браузере (admin → Периоды)**

Открой админку → раздел «Периоды». Открой создание/редактирование периода.
- Escape без правок → закрывается сразу.
- Измени название/дату, затем Escape → окно «Сохранить изменения?»; Enter → сохраняет период.
- Измени что-то, кликни крестик × → тоже окно-подтверждение.
- Измени что-то, кликни по затемнённому фону → тоже окно-подтверждение.
- Кнопка «Отмена» при наличии правок → закрывает без подтверждения (явный discard).
- Открой второй период — прокрутка страницы под модалкой заблокирована и восстанавливается после закрытия.

Ожидается: guard на Escape/×/оверлее; «Отмена» — прямой выход; scroll-lock работает.

- [ ] **Step 5: Готово к ревью (без коммита).**

---

## Task 7: Мигрировать admin `TeamEditor`

**Files:**
- Modify: `internal/web/static/admin.js:788-829` (`TeamEditor`)
- Modify: `internal/web/static/admin.js:672-679` (`TeamsSection` — `closeRef` и `guarded`)
- Modify: `internal/web/static/admin.js:542-543` (`TeamsSection` — объявить `closeRef`)

**Interfaces:**
- Consumes: `useModalClose`, рефакторнутый `Modal` (Task 6). Существующие: `value`, `f`, `canSave`, `onSave`, `onClose`, `saving`.

- [ ] **Step 1: `TeamEditor` — dirty + хук + confirmEl**

Найди сигнатуру (строка 788) и строки 789-791:

```jsx
function TeamEditor({value, teams, onSave, onClose, onDelete, saving}) {
  const [f, setF] = useState({...value});
  useEffect(()=>{setF({...value});},[value.id]);
  const canSave = f.name.trim() && f.type;
```

Замени на:

```jsx
function TeamEditor({value, teams, onSave, onClose, onDelete, saving, closeRef}) {
  const [f, setF] = useState({...value});
  useEffect(()=>{setF({...value});},[value.id]);
  const canSave = f.name.trim() && f.type;
  const isDirty = JSON.stringify(f) !== JSON.stringify(value);
  const { requestClose, confirmEl } = useModalClose({ isDirty, canSave: !!canSave && !saving, onSave: () => { if (canSave) onSave(f); }, onClose });
  useEffect(()=>{ if (closeRef) closeRef.current = requestClose; },[closeRef, requestClose]);
```

Найди закрытие тела (строки 826-829):

```jsx
      <Btn variant="primary" onClick={()=>canSave&&onSave(f)} disabled={!canSave||saving}>{saving?'Сохранение…':isNew?'Создать':'Сохранить'}</Btn>
    </div>
  </div>;
}
```

Замени на:

```jsx
      <Btn variant="primary" onClick={()=>canSave&&onSave(f)} disabled={!canSave||saving}>{saving?'Сохранение…':isNew?'Создать':'Сохранить'}</Btn>
    </div>
    {confirmEl}
  </div>;
}
```

- [ ] **Step 2: `TeamsSection` — объявить `closeRef`**

Найди строки 542-543:

```jsx
  const [modal, setModal] = useState(null); // {mode:'new', parentId} | {mode:'edit', team}
  const [saving, setSaving] = useState(false);
```

Замени на:

```jsx
  const [modal, setModal] = useState(null); // {mode:'new', parentId} | {mode:'edit', team}
  const [saving, setSaving] = useState(false);
  const teamCloseRef = useRef(null);
```

- [ ] **Step 3: `TeamsSection` — `Modal` render с guarded/closeRef**

Найди строки 676-679:

```jsx
    <Modal open={!!modal} title={modalTitle} subtitle={modalSubtitle} onClose={()=>setModal(null)} width={680}>
      {modal&&<TeamEditor value={modalValue} teams={activeTeams} onSave={save} onClose={()=>setModal(null)}
        onDelete={modal.mode==='edit'?()=>remove(modal.team.id,modal.team.name):null} saving={saving}/>}
    </Modal>
```

Замени на:

```jsx
    <Modal open={!!modal} title={modalTitle} subtitle={modalSubtitle} onClose={()=>setModal(null)} width={680} guarded closeRef={teamCloseRef}>
      {modal&&<TeamEditor value={modalValue} teams={activeTeams} onSave={save} onClose={()=>setModal(null)} closeRef={teamCloseRef}
        onDelete={modal.mode==='edit'?()=>remove(modal.team.id,modal.team.name):null} saving={saving}/>}
    </Modal>
```

- [ ] **Step 4: Проверить в браузере (admin → Команды)**

Открой раздел «Команды». Создай/редактируй команду.
- Escape без правок → закрывается.
- Измени название/тип, Escape → окно; Enter → сохраняет.
- Крестик × и клик по фону при правках → окно-подтверждение.

Ожидается: guard работает идентично периодам.

- [ ] **Step 5: Готово к ревью (без коммита).**

---

## Task 8: Мигрировать admin `UserModal` + preventDefault в admin `TeamCombobox`

**Files:**
- Modify: `internal/web/static/admin.js:1416-...` (`UserModal` — сигнатура, dirty, хук, confirmEl)
- Modify: `internal/web/static/admin.js:1404-1409` (`UsersSection` — `closeRef`/`guarded`)
- Modify: `internal/web/static/admin.js` (`UsersSection` — объявить `closeRef` рядом с `modalUser`/`setModalId`)
- Modify: `internal/web/static/admin.js:208` (`TeamCombobox.onKey`, Escape)

**Interfaces:**
- Consumes: `useModalClose`, рефакторнутый `Modal`. Существующие в `UserModal`: `grants`, `pendingGrantIds`, `pendingAdmin`, `loading`, `saving`, `save` (zero-arg), `user`, `onClose`, `onSaved`.

- [ ] **Step 1: `UserModal` — dirty + хук + confirmEl**

Найди сигнатуру (строка 1416):

```jsx
function UserModal({user, teams, currentUser, allUsers, ledTeams, onClose, onSaved}) {
```

Замени на:

```jsx
function UserModal({user, teams, currentUser, allUsers, ledTeams, onClose, onSaved, closeRef}) {
```

Найди конец функции `save` — строку с закрытием `save()` (около строки 1454, `} finally { setSaving(false); } }`), сразу ПОСЛЕ неё и ПЕРЕД `return <div>` добавь:

```jsx
  const grantsDirty = grants != null && (
    pendingGrantIds.length !== grants.length ||
    pendingGrantIds.some(id => !grants.some(g => g.TeamID === id))
  );
  const isDirty = (pendingAdmin !== (user.Role === 'admin')) || grantsDirty;
  const { requestClose, confirmEl } = useModalClose({ isDirty, canSave: !saving && !loading, onSave: save, onClose });
  useEffect(() => { if (closeRef) closeRef.current = requestClose; }, [closeRef, requestClose]);
```

(Помести этот блок ПОСЛЕ всех существующих хуков `useState`/`useEffect` и функции `save`, но ДО `return` — правило порядка хуков соблюдается, ранних `return` в `UserModal` нет.)

Найди `return <div>` тела `UserModal` и его закрытие. Тело завершается футером с кнопками (строки ~1512-1514, `<Btn variant="primary" onClick={save} ...>`). Найди закрытие всего компонента:

```jsx
    </div>
  </div>;
}
```

(последние строки `UserModal`, сразу перед следующей функцией). Замени на:

```jsx
    </div>
    {confirmEl}
  </div>;
}
```

Убедись, что заменяешь именно ВНЕШНЕЕ закрытие `UserModal` (корневой `return <div>...`), а не вложенный `DetailSection`. Ориентир: строка `<Btn variant="primary" onClick={save} disabled={saving||loading}>` футера идёт непосредственно перед этими закрывающими тегами.

- [ ] **Step 2: `UsersSection` — объявить `closeRef`**

Найди объявление состояния секции пользователей (там, где `const [modalUser...]` или `setModalId`). Рядом с ним добавь:

```jsx
  const userCloseRef = useRef(null);
```

(Если состояние хранится как `modalId`/`setModalId` — добавь `userCloseRef` в той же группе `useState`/`useRef` секции.)

- [ ] **Step 3: `UsersSection` — `Modal` render с guarded/closeRef**

Найди строки 1404-1409:

```jsx
    <Modal open={!!modalUser}
      title={modalUser&&<span style={{display:'inline-flex',alignItems:'center',gap:12}}><Avatar user={modalUser} size={36}/>{modalUser.DisplayName}{modalUser.ID===currentUser?.id&&<span style={{fontSize:10.5,color:T.mutedFg,background:'#f1f5f9',padding:'2px 7px',borderRadius:5,fontWeight:700}}>ВЫ</span>}</span>}
      subtitle={modalUser&&`${modalUser.Provider} · ${modalUser.Email}`}
      onClose={()=>setModalId(null)} width={760}>
      {modalUser&&<UserModal user={modalUser} teams={teams} currentUser={currentUser} allUsers={users} ledTeams={ledTeams(modalUser)} onClose={()=>setModalId(null)} onSaved={()=>{setModalId(null);reload();}}/>}
    </Modal>
```

Замени на:

```jsx
    <Modal open={!!modalUser}
      title={modalUser&&<span style={{display:'inline-flex',alignItems:'center',gap:12}}><Avatar user={modalUser} size={36}/>{modalUser.DisplayName}{modalUser.ID===currentUser?.id&&<span style={{fontSize:10.5,color:T.mutedFg,background:'#f1f5f9',padding:'2px 7px',borderRadius:5,fontWeight:700}}>ВЫ</span>}</span>}
      subtitle={modalUser&&`${modalUser.Provider} · ${modalUser.Email}`}
      onClose={()=>setModalId(null)} width={760} guarded closeRef={userCloseRef}>
      {modalUser&&<UserModal user={modalUser} teams={teams} currentUser={currentUser} allUsers={users} ledTeams={ledTeams(modalUser)} onClose={()=>setModalId(null)} onSaved={()=>{setModalId(null);reload();}} closeRef={userCloseRef}/>}
    </Modal>
```

- [ ] **Step 4: admin `TeamCombobox` — preventDefault при открытом дропдауне**

Найди строку 208:

```jsx
    else if(e.key==='Escape')setOpen(false);
```

Замени на:

```jsx
    else if(e.key==='Escape'){if(open){e.preventDefault();setOpen(false);}}
```

- [ ] **Step 5: Проверить в браузере (admin → Пользователи)**

Открой раздел «Пользователи», открой карточку пользователя.
- Escape без правок → закрывается.
- Переключи тумблер «Администратор» ИЛИ измени набор грантов, Escape → окно «Сохранить изменения?»; Enter → применяет (запросы к API) и закрывает.
- Внутри выдачи доступа открой выпадашку команд (`TeamCombobox`), нажми Escape → закрывается только дропдаван, модалка остаётся.
- Крестик × и клик по фону при правках → окно-подтверждение.

Ожидается: dirty учитывает и тумблер, и гранты; Escape в выпадашке не закрывает модалку.

- [ ] **Step 6: Готово к ревью (без коммита).**

---

## Task 9: Сквозная ручная верификация

**Files:** нет изменений — только проверка полного чек-листа из спеки.

- [ ] **Step 1: Полный прогон чек-листа**

Пройди по всем пунктам (трекер + админка):

1. Escape на чистой форме (каждая модалка) → закрывается сразу.
2. Escape / крестик × / клик по оверлею на изменённой форме → окно «Сохранить изменения?».
3. В окне: **Enter** → сохраняет и закрывает.
4. В окне: **повторный Escape** → выходит без сохранения.
5. В окне: «Продолжить редактирование» / клик вне окошка → возврат к форме (введённое сохранено).
6. Невалидная форма (`canSave=false`, напр. пустое имя KR) + Enter в окне → «Сохранить» неактивна, ничего не происходит.
7. Escape в открытом дропдауне (`TeamCombobox`, user-combobox) закрывает дропдаван, а не модалку.
8. Удаление (`ConfirmModal`): Escape закрывает без окна-подтверждения.
9. Вложенность (`ConfirmModal` поверх `GoalModal`): Escape закрывает только верхнюю.
10. Кнопка «Отмена» в футере формы при правках → прямой выход (без окна-подтверждения).
11. Блокировка скролла страницы под модалкой активна и восстанавливается после закрытия (в т.ч. после вложенной модалки).
12. Консоль браузера без ошибок на всех страницах (tracker, admin).

- [ ] **Step 2: Проверить консистентность спеки**

Открой `docs/superpowers/specs/2026-07-15-modal-escape-save-guard-design.md` и убедись, что реализация соответствует разделам «Решение», «Охват», «Проверка». Расхождений быть не должно; если появились — обнови спеку в этом же change set (CLAUDE.md).

- [ ] **Step 3: Финал**

Сообщи пользователю, что все задачи выполнены и проверены; напомни, что коммит — за ним.

---

## Self-Review (выполнено при написании плана)

**Spec coverage:**
- «Модалка закрывается по Escape» → Tasks 2-8 (все модалки).
- «Окно-подтверждение при dirty (Enter=save, Escape=discard)» → Task 1 (`useModalClose`/`ModalCloseConfirm`) + консумеры Tasks 3-8.
- «Единообразно для всех» → общий хук/компонент (Task 1), применён везде.
- «ConfirmModal без правок просто закрывается» → Task 2.
- «Стек модалок / вложенность» → Task 1 (`__modalStack`), проверка в Task 5/9.
- «Вложенные выпадашки: preventDefault» → Tasks 5, 8.
- «isDirty по снапшоту; admin уже имеет initial» → Tasks 3-8.
- «Вне scope: focus-trap, system.js confirm(), схема таблиц» → не затрагиваются.

**Placeholder scan:** плейсхолдеров нет — весь код приведён дословно.

**Type consistency:** имена стабильны во всех задачах — `useModalClose`, `requestClose`, `confirmEl`, `closeRef`, `guarded`, `ModalCloseConfirm`, `__modalStack`. Пропсы `Modal` (`guarded`, `closeRef`) согласованы между определением (Task 6) и всеми call-sites (Tasks 6-8).
