// Общие константы SPA (типы команд + акцентный цвет), переиспользуемые трекером,
// админкой и настройками. Раньше дублировались пофайлово (идентичные значения).
// Без бандлера: голые глобали, грузится до app-скриптов. Значение ACCENT совпадает
// с CSS-токеном --accent (tokens.css), ключи типов — с доменными TeamType.
const ACCENT = '#7c3aed';
const TEAM_TYPE_LABEL = { department: 'Департамент', cluster: 'Кластер', unit: 'Юнит', group: 'Группа', team: 'Команда', squad: 'Сквад', employee: 'Сотрудник' };
const TEAM_TYPE_ORDER = { department: 0, cluster: 1, unit: 2, group: 3, team: 4, squad: 5, employee: 6 };
const TEAM_TYPE_COLOR = { department: '#4338ca', cluster: '#7c3aed', unit: '#2563eb', group: '#0891b2', team: '#059669', squad: '#d97706', employee: '#64748b' };

// buildTargetURL — единый механизм перехода к команде/цели/KR/комментарию в трекере
// (из журнала событий, колокольчика уведомлений и обзора периода). Собирает deep-link на трекер
// (`/?team=&period=&goal=&kr=&comment=`), который трекер разбирает на загрузке:
// выбирает команду/период, раскрывает цель и секцию комментариев, скроллит и подсвечивает.
// target: { team_id?, period_id?, goal_id?, kr_id?, comment_id? }.
function buildTargetURL(target) {
  if (!target) return null;
  const p = new URLSearchParams();
  if (target.team_id) p.set('team', target.team_id);
  if (target.period_id) p.set('period', target.period_id);
  if (target.goal_id) p.set('goal', target.goal_id);
  if (target.kr_id) p.set('kr', target.kr_id);
  if (target.comment_id) p.set('comment', target.comment_id);
  const qs = p.toString();
  return qs ? '/?' + qs : null;
}

// ── СТАТУС КОМАНДЫ → РЕЖИМ РЕДАКТИРОВАНИЯ ДОСКИ ────────────────────────────────
// Единственное место, где интерфейс знает, что при каком статусе доступно. Режим
// выводится из статуса команды в периоде (см. spec team-period-lifecycle):
//   full           — «нет целей», «черновик», «к валидации»
//   progress_only  — «в работе»
//   comments_only  — «закрыт»
// Виды действий:
//   structure — меняют состав целей и KR: создание, правка, удаление, порядок
//   progress  — обновление прогресса ключевого результата
// Интерфейс намеренно строже сервера: в «закрыт» сервер принимает обновление
// прогресса, но доска предлагает только комментарии — так предписывает требование
// «Режимы редактирования выводятся из статуса».
const EDIT_MODE_LOCK = {
  progress_only: 'Состав целей зафиксирован: команда в статусе «В работе»',
  comments_only: 'Период закрыт: доступны только комментарии',
};
const EDIT_MODE_ALLOWS = {
  full: { structure: true, progress: true },
  progress_only: { structure: false, progress: true },
  comments_only: { structure: false, progress: false },
};
// goal_move — это перенос цели ИЗ текущей команды: он убирает цель из её состава,
// поэтому подчиняется тому же замку, что создание и удаление. Копирование цели в
// другую команду или период состав текущей команды не меняет и замку не подчиняется —
// отдельного вида для него здесь нет намеренно.
const ACTION_KIND = {
  goal_create: 'structure', goal_edit: 'structure', goal_delete: 'structure',
  goal_move: 'structure',
  kr_create: 'structure', kr_edit: 'structure', kr_delete: 'structure',
  reorder: 'structure', progress_update: 'progress',
};

// { allowed, reason }: reason заполнен только когда действие заблокировано.
function actionAvailability(editMode, action) {
  const kind = ACTION_KIND[action];
  if (!kind) return { allowed: true, reason: null };
  const allows = EDIT_MODE_ALLOWS[editMode] || EDIT_MODE_ALLOWS.full;
  if (allows[kind]) return { allowed: true, reason: null };
  return { allowed: false, reason: EDIT_MODE_LOCK[editMode] || null };
}

// Короткая форма для разметки: причина блокировки либо null.
const lockReason = (editMode, action) => actionAvailability(editMode, action).reason;

// Развёрнутые причины для пунктов меню: называют статус и говорят, что сделать,
// чтобы действие открылось. `what` — «цель» или «KR».
function editLockReason(editMode, what) {
  if (editMode === 'comments_only') return `Период закрыт — ${what} уже нельзя изменить.`;
  if (editMode === 'progress_only') return `Цели в статусе «В работе»: состав зафиксирован. Меняется только прогресс. Чтобы редактировать ${what}, верните статус «К валидации».`;
  return null;
}
function deleteLockReason(editMode, what) {
  if (editMode === 'comments_only') return `Период закрыт — удалить ${what} нельзя, история периода сохраняется.`;
  if (editMode === 'progress_only') return `Цели в статусе «В работе»: состав зафиксирован. Чтобы удалить ${what}, верните статус «К валидации».`;
  return null;
}
// Обновление прогресса закрыто только в «закрыт», и условие берётся из
// actionAvailability, а не сравнением режима со значением: иначе появилось бы
// третье место, знающее про статусы. Здесь только формулировка причины.
function progressLockReason(editMode) {
  return lockReason(editMode, 'progress_update')
    ? 'Период закрыт — прогресс ключевого результата уже нельзя обновить.'
    : null;
}

// ── ЗАБЛОКИРОВАННОЕ ДЕЙСТВИЕ ──────────────────────────────────────────────────
// Недоступное по статусу действие не исчезает с доски: кнопка остаётся на месте,
// принимает фокус с клавиатуры (поэтому не disabled — иначе фокус не дать) и
// называет причину. Подсказку .action-lock__tip рисует CSS по :hover и :focus-within.

// Обёртка подсказки о блокировке. Годится и для текстовых действий доски —
// «+ Добавить цель», «+ Добавить KR», — которые остаются кнопками, а не иконками.
function LockedAction({ reason, children }) {
  return (
    <span className="action-lock">
      {children}
      <span className="action-lock__tip" role="tooltip">{reason}</span>
    </span>
  );
}

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
        <div className="modal-confirm-title">Есть несохранённые изменения</div>
        <div className="modal-confirm-message">Сохранить изменения перед выходом?</div>
        <div className="modal-confirm-hint">
          <span className="modal-confirm-key">Enter</span> — сохранить и выйти
          {' · '}
          <span className="modal-confirm-key">Esc</span> — выйти без сохранения
        </div>
        <div className="modal-confirm-actions">
          <button type="button" onClick={onCancel} className="modal-confirm-btn">Отмена</button>
          <button type="button" onClick={onDiscard} className="modal-confirm-btn modal-confirm-btn--danger">Не сохранять</button>
          <button type="button" onClick={onSave} disabled={!canSave} autoFocus
            className="modal-confirm-btn modal-confirm-btn--primary">Сохранить</button>
        </div>
      </div>
    </div>
  );
}
