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
