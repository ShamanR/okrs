// Общие константы SPA (типы команд + акцентный цвет), переиспользуемые трекером,
// админкой и настройками. Раньше дублировались пофайлово (идентичные значения).
// Без бандлера: голые глобали, грузится до app-скриптов. Значение ACCENT совпадает
// с CSS-токеном --accent (tokens.css), ключи типов — с доменными TeamType.
const ACCENT = '#7c3aed';
const TEAM_TYPE_LABEL = { department: 'Департамент', cluster: 'Кластер', unit: 'Юнит', group: 'Группа', team: 'Команда', squad: 'Сквад', employee: 'Сотрудник' };
const TEAM_TYPE_ORDER = { department: 0, cluster: 1, unit: 2, group: 3, team: 4, squad: 5, employee: 6 };
const TEAM_TYPE_COLOR = { department: '#4338ca', cluster: '#7c3aed', unit: '#2563eb', group: '#0891b2', team: '#059669', squad: '#d97706', employee: '#64748b' };

// buildTargetURL — единый механизм перехода к команде/цели/KR/комментарию в трекере
// (из журнала событий и из колокольчика Health Check-in). Собирает deep-link на трекер
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
