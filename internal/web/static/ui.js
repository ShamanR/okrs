// Общие константы SPA (типы команд + акцентный цвет), переиспользуемые трекером,
// админкой и настройками. Раньше дублировались пофайлово (идентичные значения).
// Без бандлера: голые глобали, грузится до app-скриптов. Значение ACCENT совпадает
// с CSS-токеном --accent (tokens.css), ключи типов — с доменными TeamType.
const ACCENT = '#7c3aed';
const TEAM_TYPE_LABEL = { department: 'Департамент', cluster: 'Кластер', unit: 'Юнит', group: 'Группа', team: 'Команда', squad: 'Сквад', employee: 'Сотрудник' };
const TEAM_TYPE_ORDER = { department: 0, cluster: 1, unit: 2, group: 3, team: 4, squad: 5, employee: 6 };
const TEAM_TYPE_COLOR = { department: '#4338ca', cluster: '#7c3aed', unit: '#2563eb', group: '#0891b2', team: '#059669', squad: '#d97706', employee: '#64748b' };
