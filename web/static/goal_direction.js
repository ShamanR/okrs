// Направление цели и общая тёмная подсказка. Подключается до страничных скриптов на доске,
// в дереве целей, в обзоре периода и в журнале. Модуль делит глобальную область со всеми
// babel-скриптами страницы, поэтому хуки берутся из React явно, а не деструктуризацией:
// одноимённый const в двух файлах уронил бы страницу.

// Справочник направлений. Порядок — порядок показа во всех списках, фильтрах и балансах;
// коды совпадают с domain.GoalDirection на сервере.
const GOAL_DIRECTIONS = [
  { code: 'PRODUCT', icon: '🚀', label: 'Продукт', hint: 'Развитие функциональности, ценность для пользователей, бизнес-результаты', color: '#2563eb' },
  { code: 'TECH', icon: '⚙️', label: 'Техника', hint: 'Архитектура, надёжность, безопасность, техническое качество', color: '#0891b2' },
  { code: 'PROCESS', icon: '📈', label: 'Процессы', hint: 'Эффективность, организация работы, автоматизация процессов', color: '#16a34a' },
];
const GOAL_DIRECTION_DEFAULT = 'PRODUCT';
// Неизвестный или пустой код — null: вызывающий ничего не рисует.
const goalDirection = code => GOAL_DIRECTIONS.find(d => d.code === code) || null;

// Тёмная подсказка .info-hint__tooltip у элемента-якоря — общая для «?» у полей, полного
// текста обрезанного заголовка и иконки направления. Хук держит прямоугольник якоря на момент
// показа; подсказка fixed и от якоря не отслеживается, поэтому при прокрутке скрывается, а не
// отрывается от него. anchorRef — если у якоря уже есть ref (как у заголовка из useClipped).
// enabled=false (заголовок перестал быть обрезанным) закрывает подсказку: обработчик ухода
// курсора к тому времени снят, и без сброса она всплыла бы сама при следующем обрезании.
function useFloatingTip(anchorRef, enabled = true) {
  const ownRef = React.useRef(null);
  const ref = anchorRef || ownRef;
  const [anchor, setAnchor] = React.useState(null);
  const show = () => { if (ref.current) setAnchor(ref.current.getBoundingClientRect()); };
  const hide = () => setAnchor(null);
  React.useEffect(() => { if (!enabled) setAnchor(null); }, [enabled]);
  React.useEffect(() => {
    if (!anchor) return undefined;
    window.addEventListener('scroll', hide, true);
    return () => window.removeEventListener('scroll', hide, true);
  }, [anchor]);
  return { ref, anchor, show, hide };
}

// Подсказка рисуется порталом в body: внутри якоря она унаследовала бы его шрифт (жирный у
// заголовка цели), попадала бы под overflow:hidden, на котором держится многоточие, и
// ломалась бы от transform холста дерева целей. Под якорем по центру, в пределах окна с
// отступом 8px; не помещается снизу — над якорем; не помещается ни там, ни там — прижимается
// к окну поверх якоря: подсказка pointer-events:none и наведение на якорь не перехватывает.
// Высоту сверх окна срезает max-height в CSS. Пока размер не замерен, подсказка невидима,
// чтобы не мигнуть в углу.
function FloatingTip({ anchor, width, fit, children }) {
  const ref = React.useRef(null);
  const [pos, setPos] = React.useState(null);
  React.useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const tw = el.offsetWidth, th = el.offsetHeight, gap = 6, margin = 8;
    const left = Math.min(window.innerWidth - tw - margin, Math.max(margin, anchor.left + anchor.width / 2 - tw / 2));
    let top = anchor.bottom + gap;
    if (top + th > window.innerHeight - margin) {
      const above = anchor.top - gap - th;
      top = above >= margin ? above : Math.max(margin, window.innerHeight - th - margin);
    }
    setPos({ top, left });
  }, [anchor, width, fit, children]);
  return ReactDOM.createPortal(
    <span ref={ref} role="tooltip" className={`info-hint__tooltip${fit ? ' info-hint__tooltip--fit' : ''}`}
      style={{ top: pos ? pos.top : 0, left: pos ? pos.left : 0, width, transform: 'none', visibility: pos ? 'visible' : 'hidden' }}>
      {children}
    </span>,
    document.body
  );
}

// Иконка направления цели слева от заголовка. По наведению и по фокусу с клавиатуры
// показывает название и описание направления. Пустой или неизвестный код — ничего.
// sm — уменьшенный вариант для строк текста (журнал, детализация обзора, холст дерева).
function DirectionIcon({ code, sm }) {
  const tip = useFloatingTip();
  const d = goalDirection(code);
  if (!d) return null;
  return (
    <span ref={tip.ref} className={`dir-icon${sm ? ' dir-icon--sm' : ''}`} tabIndex={0}
      aria-label={`Направление: ${d.label}`}
      onMouseEnter={tip.show} onMouseLeave={tip.hide} onFocus={tip.show} onBlur={tip.hide}>
      {d.icon}
      {tip.anchor && (
        <FloatingTip anchor={tip.anchor} fit>
          <span className="dir-tip__title">{d.icon} {d.label}</span>
          {d.hint}
        </FloatingTip>
      )}
    </span>
  );
}
