// Общий контракт навигации: выбранный период живёт в URL — `?period=<id>`.
// Единый для всех страниц с выбором периода (цели `/`, дерево целей `/goal-tree`,
// обзор периода `/period-overview`, лог активностей `/activity-log`).
//
// URL — источник истины: ссылку можно скопировать и переслать, Back/Forward
// возвращает предыдущий выбор. Локальная персистентность (cookie `okr_last`,
// `localStorage`) остаётся только fallback'ом, когда параметра в URL нет.
//
// Без бандлера: голые глобали (как readCSRF в api.js), грузится до app-скриптов.
// Хуки берём как React.* — файл не зависит от того, что app-скрипт уже объявил
// `const { useState, useEffect, useRef } = React` (он грузится позже).

const PERIOD_URL_PARAM = 'period';
const PERIOD_URL_ALL = 'all'; // «Все периоды» — вариант лога активностей

// Значение периода в URL: null — параметра нет; 'all' — все периоды; иначе id (число).
function readPeriodURL() {
  const raw = new URLSearchParams(location.search).get(PERIOD_URL_PARAM);
  if (raw === null || raw === '') return null;
  if (raw === PERIOD_URL_ALL) return PERIOD_URL_ALL;
  const n = Number(raw);
  return Number.isFinite(n) && n > 0 ? n : null;
}

// Пишет параметры навигации в текущий location.pathname.
// patch: { period: <id|'all'|null>, team: <id|null> } — null/'' удаляет параметр.
// reset: true — остальные параметры выбрасываются (трекер: team+period, без
//   deep-link хвоста ?goal/?kr/?comment, который валиден только при открытии).
// replace: true — history.replaceState (первый разрешённый выбор), иначе pushState.
function writeNavURL(patch, { replace = false, reset = false } = {}) {
  const p = reset ? new URLSearchParams() : new URLSearchParams(location.search);
  Object.keys(patch || {}).forEach(k => {
    const v = patch[k];
    if (v === null || v === undefined || v === '') p.delete(k);
    else p.set(k, String(v));
  });
  const qs = p.toString();
  const url = location.pathname + (qs ? '?' + qs : '');
  if (replace) history.replaceState(null, '', url);
  else history.pushState(null, '', url);
}

function writePeriodURL(value, replace = false) {
  writeNavURL({ [PERIOD_URL_PARAM]: value }, { replace });
}

// Начальный выбор периода: ?period (если такой период есть в списке) → первый
// валидный fallback (сохранённый локально, дефолт страницы) → null.
// items — элементы GET /api/v1/periods.
function pickPeriodFromURL(items, ...fallbacks) {
  const list = items || [];
  const valid = id => id != null && list.some(p => p.id === id);
  const fromURL = readPeriodURL();
  if (typeof fromURL === 'number' && valid(fromURL)) return fromURL;
  for (const f of fallbacks) if (valid(f)) return f;
  return null;
}

// Двусторонняя синхронизация выбранного периода с URL.
//   value — текущее значение в представлении URL (id | 'all' | null);
//   ready — false, пока начальный выбор ещё не разрешён (грузятся периоды): URL не трогаем;
//   onPop(value) — Back/Forward, value из URL (null — параметра нет).
// Первый разрешённый выбор подменяет текущую запись истории (replaceState), каждая
// последующая смена периода добавляет новую (pushState) — как в трекере. Если URL уже
// содержит нужное значение (сразу после Back/Forward) — история не трогается вовсе.
function usePeriodURLSync(value, ready, onPop) {
  const initedRef = React.useRef(false);
  React.useEffect(() => {
    if (!ready) return;
    if (readPeriodURL() === value) { initedRef.current = true; return; }
    writePeriodURL(value, !initedRef.current);
    initedRef.current = true;
  }, [value, ready]);

  const onPopRef = React.useRef(onPop);
  onPopRef.current = onPop;
  React.useEffect(() => {
    const h = () => onPopRef.current(readPeriodURL());
    window.addEventListener('popstate', h);
    return () => window.removeEventListener('popstate', h);
  }, []);
}

// Query-хвост для ссылок сайдбара: текущий период переносится на другие разделы
// с выбором периода (см. Sidebar linkParams).
function periodLinkParams(value) {
  const qs = value === null || value === undefined || value === '' ? '' : `?${PERIOD_URL_PARAM}=${value}`;
  return { 'tracker': qs, 'goal-tree': qs, 'period-overview': qs, 'activity-log': qs };
}
