// Общий контракт localStorage между трекером (tracker.js) и страницей настроек
// (settings.js): персональные оверрайды описаний команд и выбор узлов сайдбара
// хранятся под одними ключами (per-user). Раньше строки ключей дублировались в
// обоих файлах с комментарием «must match tracker.js» — здесь единственный источник.
// Без бандлера: голые глобали (как readCSRF в api.js), грузится до app-скриптов.
const STORAGE_KEYS = {
  desc: uid => `okr_team_desc_overrides:${uid}`,
  sidebar: uid => `okr_sidebar_nodes:${uid}`,
};

function readJSON(key, fallback) {
  try { const v = localStorage.getItem(key); return v == null ? fallback : JSON.parse(v); }
  catch (_) { return fallback; }
}

function writeJSON(key, value) {
  try { localStorage.setItem(key, JSON.stringify(value)); } catch (_) { /* quota / private mode */ }
}
