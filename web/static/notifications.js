// NotificationList / NotificationsPanel / NotificationsBell — колокольчик уведомлений.
// Подключается ПЕРЕД sidebar.js во всех SPA-shell: сайдбар рендерит колокольчик сам,
// не получая его пропом, поэтому уведомления доступны на каждой странице.
//
// Использует React.useState/useEffect (не top-level деструктуризацию), чтобы не
// конфликтовать с app-скриптами, делящими ту же глобальную область.

// Опрос счётчика раз в минуту. Кэша на сервере нет намеренно: это COUNT по
// частичному индексу для одного пользователя, а кэш в памяти инстанса дал бы
// разные числа на разных репликах K8S.
const NOTIF_POLL_MS = 60000;

function _notifCSRF() {
  const m = document.cookie.match(/(?:^|;\s*)okr_csrf_token=([^;]*)/);
  return m ? decodeURIComponent(m[1]) : '';
}

// Ссылку резолвим тем же алгоритмом, каким браузер пойдёт по ней, и сверяем origin.
// Проверка по префиксу здесь ненадёжна: браузер нормализует "\" в "/", поэтому
// "/\evil.com" превращается в "//evil.com" и уводит на чужой хост. new URL бросает
// исключение на некорректном вводе — считаем это отказом (ссылку не рендерим), а не
// поводом вернуться к исходной строке.
function _notifSafeHref(url) {
  if (!url) return undefined;
  try {
    const u = new URL(url, window.location.origin);
    return u.origin === window.location.origin ? u.pathname + u.search + u.hash : undefined;
  } catch (_) {
    return undefined;
  }
}

// Относительное время: «5 мин назад». Абсолютная дата остаётся в title.
function _notifAgo(iso) {
  const then = new Date(iso).getTime();
  if (!then) return '';
  const mins = Math.floor((Date.now() - then) / 60000);
  if (mins < 1) return 'только что';
  if (mins < 60) return mins + ' мин назад';
  const hours = Math.floor(mins / 60);
  if (hours < 24) return hours + ' ч назад';
  const days = Math.floor(hours / 24);
  if (days < 30) return days + ' дн назад';
  return new Date(iso).toLocaleDateString('ru-RU');
}

// NotificationList — список записей. Вынесен отдельно от панели, чтобы его можно
// было переиспользовать на отдельной странице уведомлений, не переписывая.
function NotificationList({ items, onRead }) {
  if (!items.length) {
    return (
      <div className="notif__empty">
        <span className="notif__empty-icon">🔕</span>
        <span>Пока нет уведомлений</span>
      </div>
    );
  }
  return (
    <div className="notif__list">
      {items.map(n => {
        // Разрешаем переход только на локальный путь. Сегодня сервер шлёт только
        // "" или "/?goal_id=...", но это защита в глубину: React лишь предупреждает
        // про javascript:-href, а не блокирует его, так что единственный барьер —
        // здесь. Резолвим _notifSafeHref, а не сверяем префикс строки:
        // браузер нормализует "\" в "/", и префиксная проверка это пропускает.
        const href = _notifSafeHref(n.url);
        const cls = `notif__item${n.read ? '' : ' notif__item--unread'}`;
        const content = (
          <div className="notif__item-row">
            {/* SidebarAvatar (sidebar.js) уже умеет и картинку, и инициалы —
                переиспользуем его вместо второй реализации того же компонента.
                Пустой actor_avatar (бывший участник, PII-правило на сервере)
                молча уходит в ветку инициалов, а не в битую картинку. */}
            <SidebarAvatar user={{ display_name: n.actor_name, avatar_url: n.actor_avatar }} size={28} />
            <div className="notif__item-main">
              <div className="notif__item-head">
                {/* Текст пришёл с сервера и рендерится как текст: никакого
                    dangerouslySetInnerHTML — правило 8 в specs/010. */}
                <span className="notif__item-title">{n.title}</span>
                <span className="notif__item-time" title={n.created_at}>{_notifAgo(n.created_at)}</span>
              </div>
              <div className="notif__item-body">{n.body}</div>
            </div>
          </div>
        );
        if (href) {
          return (
            <a key={n.id} className={cls} href={href} onClick={() => onRead(n.id)}>
              {content}
            </a>
          );
        }
        // Без ссылки переход невозможен, только пометка прочитанным — <a> без
        // href не фокусируется и не активируется с клавиатуры, поэтому здесь
        // доступный div с role="button" и обработкой Enter/Space.
        const activate = () => onRead(n.id);
        return (
          <div
            key={n.id}
            className={cls}
            role="button"
            tabIndex={0}
            onClick={activate}
            onKeyDown={e => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); activate(); } }}
          >
            {content}
          </div>
        );
      })}
    </div>
  );
}

// NotificationsPanel — выпадающая панель. Данные грузит при открытии, а не при
// монтировании: на большинстве заходов панель не открывают вовсе.
function NotificationsPanel({ open, onClose, onChanged }) {
  const [items, setItems] = React.useState([]);
  const [loading, setLoading] = React.useState(false);

  React.useEffect(() => {
    if (!open) return;
    setLoading(true);
    fetch('/api/v1/notifications?limit=30', { credentials: 'include' })
      .then(r => (r.ok ? r.json() : null))
      .then(d => setItems((d && d.items) || []))
      .catch(() => {})
      .finally(() => setLoading(false));
  }, [open]);

  React.useEffect(() => {
    if (!open) return;
    const onKey = e => { if (e.key === 'Escape') onClose(); };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [open, onClose]);

  const markRead = (ids, all) => {
    fetch('/api/v1/notifications/read', {
      method: 'POST',
      credentials: 'include',
      // keepalive — клик по уведомлению одновременно шлёт этот POST и уводит
      // браузер по ссылке; без keepalive навигация может оборвать запрос на
      // лету, и пользователь вернётся к всё ещё непрочитанному уведомлению.
      keepalive: true,
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': _notifCSRF() },
      body: JSON.stringify(all ? { all: true } : { ids }),
    }).then(r => {
      if (!r.ok) return;
      setItems(prev => prev.map(n => (all || ids.includes(n.id) ? { ...n, read: true } : n)));
      onChanged();
    });
  };

  if (!open) return null;
  const hasUnread = items.some(n => !n.read);
  return (
    <>
      <div className="notif__backdrop" onClick={onClose} />
      <div className="notif__panel">
        <div className="notif__panel-head">
          <span className="notif__panel-title">Уведомления</span>
          {hasUnread && (
            <button className="notif__mark-all" onClick={() => markRead(null, true)}>
              Отметить все прочитанными
            </button>
          )}
          <button className="notif__close" onClick={onClose} aria-label="Закрыть">✕</button>
        </div>
        {loading
          ? <div className="notif__empty">Загрузка…</div>
          : <NotificationList items={items} onRead={id => markRead([id], false)} />}
      </div>
    </>
  );
}

// NotificationsBell — иконка с бейджем плюс панель. Рендерится сайдбаром на всех
// страницах; счётчик опрашивается раз в минуту и при возврате фокуса на вкладку.
function NotificationsBell() {
  const [count, setCount] = React.useState(0);
  const [open, setOpen] = React.useState(false);
  // hidden — на странице нет активного тенанта (no_membership, system без
  // тенанта): маршруты уведомлений гейтятся по членству и всегда отвечают 403.
  // Отличаем именно 403 от сетевого сбоя: временная ошибка не должна прятать
  // колокольчик, а постоянное отсутствие скоупа — должна, иначе кнопка
  // остаётся с нулевым бейджем и открывает вечно пустую панель.
  const [hidden, setHidden] = React.useState(false);

  const refresh = React.useCallback(() => {
    fetch('/api/v1/notifications/unread-count', { credentials: 'include' })
      .then(r => {
        if (r.status === 403) { setHidden(true); return null; }
        return r.ok ? r.json() : null;
      })
      .then(d => { if (d) setCount(d.count || 0); })
      .catch(() => {});
  }, []);

  React.useEffect(() => {
    if (hidden) return;
    refresh();
    const timer = setInterval(refresh, NOTIF_POLL_MS);
    const onFocus = () => refresh();
    window.addEventListener('focus', onFocus);
    return () => { clearInterval(timer); window.removeEventListener('focus', onFocus); };
  }, [refresh, hidden]);

  if (hidden) return null;

  return (
    <>
      <button className="sidebar__bell" onClick={() => setOpen(o => !o)} aria-label="Уведомления">
        <span className="sidebar__bell-icon">🔔</span>
        <span className={`sidebar__bell-badge${count === 0 ? ' sidebar__bell-badge--zero' : ''}`}>{count}</span>
      </button>
      <NotificationsPanel open={open} onClose={() => setOpen(false)} onChanged={refresh} />
    </>
  );
}
