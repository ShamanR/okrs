// Общий слой CSRF для всех SPA-страниц. Грузится ПЕРВЫМ (до markdown/sidebar/app-скриптов),
// поэтому каждая страница читает double-submit cookie одинаково. Раньше каждый entrypoint
// объявлял свой readCSRF, и часть из них теряла decodeURIComponent — латентное расхождение
// токена. Без бандлера: экспортируется голыми глобалями (как Markdown в markdown.js,
// Sidebar в sidebar.js) — readCSRF / csrfHeaders доступны всем последующим скриптам.
function readCSRF() {
  const m = document.cookie.match(/(?:^|;\s*)okr_csrf_token=([^;]*)/);
  return m ? decodeURIComponent(m[1]) : '';
}

function csrfHeaders(extra = {}) {
  return { 'X-CSRF-Token': readCSRF(), 'Content-Type': 'application/json', ...extra };
}

// ── Контекст пространства (tenant) ──────────────────────────────────────────
// Активное пространство — состояние вкладки, а не сессии: sessionStorage
// изолирован между вкладками (две вкладки — два пространства), localStorage
// хранит «последний выбранный» между запусками браузера. Каждый API-запрос
// несёт выбор заголовком X-Tenant-ID; сервер валидирует его против членств.
const TENANT_TAB_KEY = 'okrs_tenant_tab';
const TENANT_LAST_KEY = 'okrs_tenant_last';

function tenantRead() {
  try { const v = sessionStorage.getItem(TENANT_TAB_KEY); if (v) return Number(v); } catch (_) {}
  try { const v = localStorage.getItem(TENANT_LAST_KEY); if (v) return Number(v); } catch (_) {}
  return null;
}
function tenantWrite(id) {
  try { sessionStorage.setItem(TENANT_TAB_KEY, String(id)); } catch (_) {}
  try { localStorage.setItem(TENANT_LAST_KEY, String(id)); } catch (_) {}
}
function tenantClear() {
  try { sessionStorage.removeItem(TENANT_TAB_KEY); } catch (_) {}
  try { localStorage.removeItem(TENANT_LAST_KEY); } catch (_) {}
}

// Резолв контекста вкладки: ?tenant= (hand-off из /open, /invite и OAuth-колбэка)
// → хранилища → первое доступное членство. Возвращает id пространства или null,
// когда членств нет вовсе (тогда страница уходит на /no-access). Запоминается
// одним промисом на загрузку страницы: все ранние запросы ждут его, а не гоняются.
let _tenantReady = null;
function tenantResolve() {
  if (_tenantReady) return _tenantReady;
  _tenantReady = (async () => {
    const fromURL = (() => {
      const v = new URLSearchParams(location.search).get('tenant');
      const n = v ? Number(v) : null;
      return Number.isFinite(n) && n > 0 ? n : null;
    })();
    const stored = tenantRead();
    const wanted = fromURL || stored;
    let list;
    try {
      const r = await _origFetch('/api/v1/session/tenants', { credentials: 'include' });
      if (r.status === 401) { location.href = '/login?next=' + encodeURIComponent(location.pathname + location.search); return null; }
      if (!r.ok) return stored; // сеть/сервер: работаем с тем, что помним
      list = await r.json();
    } catch (_) { return stored; }
    if (!Array.isArray(list) || !list.length) { tenantClear(); return null; }
    const chosen = list.find(t => t.id === wanted) || list[0];
    tenantWrite(chosen.id);
    if (fromURL && fromURL === chosen.id) {
      // Параметр — разовый hand-off: приняли, записали, убираем из адресной строки.
      const u = new URL(location.href);
      u.searchParams.delete('tenant');
      history.replaceState(null, '', u.pathname + (u.searchParams.toString() ? '?' + u.searchParams : '') + u.hash);
    }
    return chosen.id;
  })();
  return _tenantReady;
}

// Контекст протёк (отозвали доступ, пространство приостановлено): сбрасываем
// сохранённое и перерезолвиваем с нуля.
function tenantReresolve() {
  tenantClear();
  _tenantReady = null;
  return tenantResolve();
}

// Запросы, которым контекст пространства не нужен: сам список пространств,
// глобальная идентичность, onboarding и system-плоскость (та адресует
// пространства явно, по id в пути).
function tenantExempt(url) {
  return !url.startsWith('/api/') ||
    url.startsWith('/api/v1/session/') ||
    url.startsWith('/api/v1/me') ||
    url.startsWith('/api/v1/onboarding/') ||
    url.startsWith('/api/v1/system/');
}

// Обёртка fetch: дожидается резолва контекста и проставляет X-Tenant-ID на все
// tenant-scoped API-запросы, какой бы помощник (apiGet/apiPost/голый fetch) ни
// собрал вызов. Ответы различает по коду ошибки: no_membership → страница
// no-access (но не со страниц system-плоскости: она доступна и без членств);
// tenant_forbidden → контекст вкладки протух, перерезолв и retry — только для
// безопасных методов: повторять мутацию в другом пространстве нельзя, она была
// задумана для исходного.
const _origFetch = window.fetch.bind(window);
window.fetch = async function (url, opts = {}) {
  const u = typeof url === 'string' ? url : (url && url.url) || '';
  if (tenantExempt(u)) return _origFetch(url, opts);
  const id = await tenantResolve();
  if (id === null) {
    if (!location.pathname.startsWith('/no-access') && !location.pathname.startsWith('/system')) location.href = '/no-access';
    return new Response('{"error":"no_membership"}', { status: 403, headers: { 'Content-Type': 'application/json' } });
  }
  opts.headers = { ...(opts.headers || {}), 'X-Tenant-ID': String(id) };
  let r = await _origFetch(url, opts);
  if (r.status === 403) {
    const code = await r.clone().json().then(b => b && b.error).catch(() => '');
    const method = (opts.method || 'GET').toUpperCase();
    const safe = method === 'GET' || method === 'HEAD';
    if (code === 'tenant_forbidden' && safe) {
      const fresh = await tenantReresolve();
      if (fresh === null) {
        if (!location.pathname.startsWith('/no-access') && !location.pathname.startsWith('/system')) location.href = '/no-access';
        return r;
      }
      opts.headers['X-Tenant-ID'] = String(fresh);
      r = await _origFetch(url, opts);
    } else if (code === 'no_membership' && !location.pathname.startsWith('/no-access') && !location.pathname.startsWith('/system')) {
      location.href = '/no-access';
    }
  }
  return r;
};
