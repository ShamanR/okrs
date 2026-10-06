// Тестовые данные прототипа. Вымышленные: команды, люди и числа не соответствуют
// ничему реальному. Имена полей намеренно совпадают с теми, что приходят в
// компоненты трекера (kr.krType, kr.healthStatus, kr.updatedDaysAgo, goal.type,
// goal.focus, goal.priority), чтобы разметка переносилась в код дословно.

// ── Константы из web/static/ui.js и tracker.js ───────────────────────────────
const ACCENT = '#7c3aed';
const TEAM_TYPE_COLOR = { department: '#4338ca', cluster: '#7c3aed', unit: '#2563eb', group: '#0891b2', team: '#059669', squad: '#d97706', employee: '#64748b' };
const TEAM_TYPE_LABEL = { department: 'Департамент', cluster: 'Кластер', unit: 'Юнит', group: 'Группа', team: 'Команда', squad: 'Сквад', employee: 'Сотрудник' };

const HEALTH_COLOR = { ahead: '#16a34a', on_track: '#2563eb', below: '#ef4444', stale: '#d97706', no_goals: '#d1d5db' };
const HEALTH_LABEL = { ahead: 'опережает', on_track: 'в плане', below: 'отстаёт', stale: 'нет обновлений', no_goals: 'нет целей' };
// Подписи на карточке цели — со значком, как в GoalCard.
const HEALTH_CARD_LABEL = { ahead: '▲ опережает', on_track: '✓ в плане', stale: '⚠ нет обновлений', below: '▼ отстаёт' };

const PRI_SHORT = { P0: 'Критичный', P1: 'Высокий', P2: 'Средний', P3: 'Низкий' };
const PRI_HINT = {
  P0: 'Критично: без этой цели период провален. Ресурсы — в первую очередь.',
  P1: 'Высокий: ключевая цель периода, делаем обязательно.',
  P2: 'Средний: важно, но можно сдвинуть при нехватке ресурсов.',
  P3: 'Низкий: делаем, если останется время.',
};
const PRI_COLOR = { P0: '#dc2626', P1: '#d97706', P2: '#2563eb', P3: '#6b7280' };
const FOCUS_COLORS = { EFFICIENCY: '#0891b2', QUALITY: '#7c3aed', RELIABILITY: '#059669', GROWTH: '#d97706', PROFITABILITY: '#dc2626', STABILITY: '#6366f1', SPEED_EFFICIENCY: '#0891b2', TECH_INDEPENDENCE: '#be185d', DEFAULT: '#6b7280' };
const FOCUS_LABEL = { PROFITABILITY: 'Profitability', STABILITY: 'Stability', SPEED_EFFICIENCY: 'Speed Efficiency', TECH_INDEPENDENCE: 'Tech Independency' };

const KR_TYPE_C = { NUMERICAL: '#2563eb', BOOLEAN: '#7c3aed', PROJECT: '#d97706' };
const KR_MEASURE_INFO = {
  NUMERICAL: { icon: '123', title: 'Метрика', desc: 'Старт → цель, вводите текущее значение', ex: 'p95 latency 800 → 300 мс' },
  BOOLEAN:   { icon: '✓', title: 'True / False', desc: 'Достигнут или нет: 0% или 100%', ex: 'сервис переведён на k8s' },
  PROJECT:   { icon: '☰', title: 'Шаги проекта', desc: 'Сумма весов выполненных шагов', ex: 'RFC → миграция БД → отключение legacy' },
};
const KR_TYPE_LABEL = { BOOLEAN: 'Бинарный', PROJECT: 'Проектный', NUMERICAL: 'Числовой' };

const KR_HEALTH_COLOR = { not_started: '#6b7280', on_track: '#16a34a', at_risk: '#d97706', done: '#15803d' };
const KR_HEALTH_LABEL = { not_started: 'Not Started', on_track: 'On Track', at_risk: 'At Risk', done: 'Closed' };
const KR_HEALTH_ICON = { not_started: '○', on_track: '●', at_risk: '▲', done: '✓' };
const KR_HEALTH_OPTIONS = ['not_started', 'on_track', 'at_risk', 'done'];
const KR_HEALTH_HINT = {
  not_started: 'Работа ещё не началась',
  on_track: 'Идём по плану',
  at_risk: 'Есть риск не достичь',
  done: 'Работы по KR прекращены — прогресс больше не предполагается менять',
};

// Вид закрытого KR — тот же набор, что в tracker.js. Статус сообщает, что работ больше
// не будет, а чем они закончились — прогресс.
const KR_CLOSED_VIEW = {
  done: { icon: '✓', flag: '', note: '', hint: () => 'Работы прекращены, результат достигнут' },
  done_near: { icon: '✓', flag: '!', note: 'результат не достигнут',
    hint: p => `Закрыт на ${p}% — работы прекращены, результат не достигнут` },
  done_short: { icon: '✕', flag: '', note: 'результат не достигнут',
    hint: p => `Закрыт на ${p}% — работы прекращены, результат не достигнут` },
};
function closedViewOf(progress, greenThreshold) {
  if (progress >= 100) return 'done';
  return progress >= greenThreshold ? 'done_near' : 'done_short';
}

const STATUS_STEPS = [
  { k: 'forming', l: 'Черновик' },
  { k: 'ready', l: 'К валидации' },
  { k: 'in_progress', l: 'В работе' },
  { k: 'closed', l: 'Закрыты' },
];

const STALE_DAYS = 7;

const KR_TYPE_OPTIONS = ['BOOLEAN', 'PROJECT', 'NUMERICAL'];
const KR_UNITS = ['%', 'RPS', 'мс', 'сек', 'мин', 'час', 'дней', 'шт', '₽', 'запросов', 'ошибок', 'пользователей', 'заказов', 'рублей'];

// depth — уровень периода (0 годовой, 1 квартальный): по нему граф раскладывает бэнды.
// rank — хронология: раньше → левее.
const PERIODS = [
  { id: 10, name: 'Y27',   range: '01.01.27 — 31.12.27', state: 'планирование', color: '#6366f1', elapsed: 0, depth: 0, rank: 0, status: 'planned' },
  { id: 11, name: 'Q1Y27', range: '01.01.27 — 31.03.27', state: '', color: '#94a3b8', elapsed: 0, depth: 1, rank: 1, status: 'planned' },
  { id: 12, name: 'Q2Y27', range: '01.04.27 — 30.06.27', state: '', color: '#94a3b8', elapsed: 0, depth: 1, rank: 1, status: 'planned' },
  { id: 13, name: 'Q3Y27', range: '01.07.27 — 30.09.27', state: '', color: '#94a3b8', elapsed: 0, depth: 1, rank: 1, status: 'planned' },
  { id: 14, name: 'Q4Y27', range: '01.10.27 — 31.12.27', state: '', color: '#94a3b8', elapsed: 0, depth: 1, rank: 1, status: 'planned' },
  { id: 1,  name: 'Y26',   range: '01.01.26 — 31.12.26', state: 'активный', color: '#6366f1', elapsed: 0.74, depth: 0, rank: 0, status: 'active' },
  { id: 4,  name: 'Q1Y26', range: '01.01.26 — 31.03.26', state: 'закрыт', color: '#94a3b8', elapsed: 1, depth: 1, rank: 1, status: 'closed' },
  { id: 2,  name: 'Q2Y26', range: '01.04.26 — 30.06.26', state: 'закрыт', color: '#94a3b8', elapsed: 1, depth: 1, rank: 1, status: 'closed' },
  { id: 3,  name: 'Q3Y26', range: '01.07.26 — 30.09.26', state: 'активный', color: '#16a34a', elapsed: 0.98, depth: 1, rank: 2, status: 'active' },
  { id: 5,  name: 'Q4Y26', range: '01.10.26 — 31.12.26', state: 'планирование', color: '#d97706', elapsed: 0, depth: 1, rank: 1, status: 'planned' },
];

// isAdmin / isSystemAdmin приходят из /api/v1/config и решают, какие пункты
// показывать в меню пользователя.
const ME = { name: 'Анна Петрова', initials: 'АП', email: 'a.petrova@example.com', isAdmin: true, isSystemAdmin: true };
// Пространства (тенанты). Переключатель в шапке сайдбара появляется только
// когда их больше одного — как в SidebarTenant.
const TENANTS = [
  { id: 1, name: 'Департамент медиа', active: true },
  { id: 2, name: 'Test', active: false },
];

// Инициалы тенанта: два слова → первые буквы, одно слово → первые две буквы.
function tenantInitials(name) {
  const words = (name || '').trim().split(/\s+/).filter(Boolean);
  if (words.length >= 2) return (words[0][0] + words[1][0]).toUpperCase();
  return (words[0] || 'OK').slice(0, 2).toUpperCase();
}

const TREE = [
  {
    id: 10, name: 'Департамент разработки', type: 'department', lead: 'Сергей Иванов', leadInitials: 'СИ',
    desc: 'Платформа, мобильные продукты и аналитика',
    children: [
      { id: 11, name: 'Платформенная команда', type: 'team', lead: 'Сергей Иванов', leadInitials: 'СИ',
        desc: 'Команда отвечает за платформенные сервисы\nКанал поддержки ~platform\nСервисы команды', children: [] },
      { id: 12, name: 'Мобильная разработка', type: 'team', lead: 'Иван Петров', leadInitials: 'ИП',
        desc: 'iOS и Android приложения', children: [
          { id: 14, name: 'Мобильный сквад', type: 'squad', lead: 'Дмитрий Смирнов', leadInitials: 'ДС', desc: 'Кроссплатформенные эксперименты', children: [] },
        ] },
      { id: 15, name: 'Инфраструктура', type: 'unit', lead: 'Дмитрий Смирнов', leadInitials: 'ДС', desc: 'Сети, железо, дежурства', children: [] },
      { id: 13, name: 'Аналитика', type: 'team', lead: 'Мария Ковалёва', leadInitials: 'МК', desc: 'Витрины данных и продуктовая аналитика', children: [] },
    ],
  },
];

const BOARDS = {
  10: { status: 'no_goals', changedAt: '—', goals: [] },
  11: {
    status: 'ready', changedAt: '15 июн.',
    goals: [
      {
        id: 101, priority: 'P1', weight: 35, type: 'delivery', focus: 'SPEED_EFFICIENCY',
        title: 'Отказ от Маркетинговой Админки', desc: 'Переводим все сценарии заведения рекламы из **Маркетинговой Админки** в Self Service, чтобы у агентств и внутренних команд остался один инструмент.\n\n**Зачем**\n- Админка не развивается с 2023 года\n- на её поддержку уходит ~20% времени команды\n- ошибки ручного заведения дают до 15 инцидентов в месяц\n\n**Критерий успеха:** Админка в режиме *только для чтения*, новых заведений нет 4 недели подряд.\n\n**Вне скоупа:** миграция исторических кампаний — остаются в архиве. Подробности — [RFC-214](#).', updatedDaysAgo: 114,
        owners: [{ name: 'Сергей Иванов', initials: 'СИ' }], periodId: 3, parentIds: [103], childIds: [],
        parents: [{ id: 103, title: 'Единый онбординг сервисов', periodName: 'Y26', teamName: 'Платформенная команда' }], shareTeams: null,
        krs: [
          { id: 1011, weight: 10, name: 'Заведение TV и ООН баннеров в Self Service', desc: 'Пейсменты TV и ООН доступны в SS: менеджер может выбрать формат, загрузить креатив, пройти модерацию и запустить размещение без участия команды поддержки. Считаем долю размещений TV и ООН, заведённых через SS, от общего числа за месяц.',
            krType: 'NUMERICAL', unit: '%', start: 0, target: 100, current: 80, checkpoints: [{ value: 30, progress_percent: 50 }, { value: 70, progress_percent: 85 }], healthStatus: 'done', updatedDaysAgo: 116, note: true },
          { id: 1012, weight: 30, name: 'Доступна настройка лимитов для РА', desc: 'В кабинете реализован функционал настройки лимитов: дневные и общие бюджеты на уровне агентства и отдельного клиента, уведомления при достижении 80% и 100% лимита.',
            krType: 'NUMERICAL', unit: '%', start: 0, target: 100, current: 100, healthStatus: 'done', updatedDaysAgo: 114, note: true },
          { id: 1013, weight: 30, name: 'Запуск click-in РК через РА', desc: 'РА могут запускать click-in РК в SS самостоятельно — от создания кампании до выгрузки отчёта. Метрика: доля click-in кампаний, запущенных агентствами без обращения в поддержку.',
            krType: 'NUMERICAL', unit: '%', start: 0, target: 100, current: 70, healthStatus: 'done', updatedDaysAgo: 116, note: true },
          { id: 1014, weight: 30, name: 'Маркетинговые баннеры заводятся в SS', desc: 'Маркетинговые баннеры заводятся в SS',
            krType: 'NUMERICAL', unit: '%', start: 0, target: 100, current: 100, healthStatus: 'done', updatedDaysAgo: 114, note: true },
        ],
        comments: [
          { id: 9001, author: 'Анна Петрова', initials: 'АП', date: '2 дня назад', text: 'Нужно пересчитать базу: методику поменяли в середине квартала.', resolved: false,
            replies: [{ id: 9002, author: 'Сергей Иванов', initials: 'СИ', date: 'вчера', text: 'Пересчитал, расхождение 0,4 п.п. — обновил значение.' }] },
          { id: 9003, author: 'Иван Петров', initials: 'ИП', date: 'неделю назад', text: 'Добавьте ссылку на дашборд в описание.', resolved: false, replies: [] },
        ],
      },
      {
        id: 102, priority: 'P1', weight: 10, type: 'delivery', focus: 'SPEED_EFFICIENCY',
        title: 'Отказ от старого кабинета Медиа', desc: 'Закрываем старый кабинет Медиа: медиапланирование и отчётность переезжают в новый кабинет. После переезда отключаем старые API и освобождаем 3 сервера.', updatedDaysAgo: 114,
        owners: [{ name: 'Анна Петрова', initials: 'АП' }, { name: 'Иван Петров', initials: 'ИП' }], periodId: 3, parentIds: [103], childIds: [202],
        parents: [{ id: 103, title: 'Единый онбординг сервисов', periodName: 'Y26', teamName: 'Платформенная команда' }],
        children: [{ id: 202, title: 'Запуск офлайн-режима', periodName: 'Q3Y26', teamName: 'Мобильная разработка' }],
        shareTeams: { ownerTeam: 'Платформенная команда', teams: ['Мобильная разработка'] },
        krs: [
          { id: 1021, weight: 50, name: 'Медиапланирование в новом кабинете', desc: 'Медиапланы создаются, согласуются и экспортируются в новом кабинете; шаблоны старого кабинета перенесены автоматически. Проверяем на 5 крупнейших агентствах.', krType: 'PROJECT', healthStatus: 'at_risk', updatedDaysAgo: 114,
            stages: [{ title: 'Пилот', weight: 40, done: true }, { title: 'Раскатка', weight: 60, done: false }] },
          { id: 1022, weight: 50, name: 'Отчётность перенесена', desc: 'Все **14 регулярных отчётов** доступны в новом кабинете:\n- те же цифры, расхождение со старым — не более `0,5%`\n- выгрузка в XLSX и CSV\n- рассылка по расписанию', krType: 'BOOLEAN', done: false, healthStatus: 'not_started', updatedDaysAgo: 120 },
        ],
        comments: [],
      },
      {
        id: 103, priority: 'P2', weight: 20, type: 'discovery', focus: 'TECH_INDEPENDENCE',
        title: 'Единый онбординг сервисов', desc: 'Общая цель с мобильной разработкой и аналитикой. Новый сервис подключается к платформе **за один день** по общему шаблону.\n\nИз коробки:\n1. CI/CD и `deploy.yaml`\n2. логирование и метрики\n3. алёрты и дашборд\n\n> Сейчас на это уходит 1–2 недели, и у каждой команды свой набор инструментов.', updatedDaysAgo: 5,
        owners: [{ name: 'Сергей Иванов', initials: 'СИ' }, { name: 'Мария Ковалёва', initials: 'МК' }, { name: 'Дмитрий Смирнов', initials: 'ДС' }], periodId: 1, parentIds: [], childIds: [101, 102, 301],
        children: [
          { id: 101, title: 'Отказ от Маркетинговой Админки', periodName: 'Q3Y26', teamName: 'Платформенная команда' },
          { id: 102, title: 'Отказ от старого кабинета Медиа', periodName: 'Q3Y26', teamName: 'Платформенная команда' },
          { id: 301, title: 'Витрина удержания в проде', periodName: 'Q3Y26', teamName: 'Аналитика' }],
        shareTeams: { ownerTeam: 'Платформенная команда', teams: ['Мобильная разработка', 'Аналитика'] },
        krs: [
          { id: 1031, weight: 100, name: 'Сервисов на общем шаблоне', desc: 'Количество продовых сервисов, развёрнутых из общего шаблона и прошедших чек-лист готовности.', krType: 'NUMERICAL', unit: 'шт', start: 2, target: 12, current: 11, healthStatus: 'on_track', updatedDaysAgo: 5 },
        ],
        comments: [],
      },
      {
        // Вес 0: цель в периоде есть, но в прогресс команды не входит. Нужна, чтобы на
        // прототипе было видно работу птички «Скрыть цели с весом 0».
        id: 104, priority: 'P3', weight: 0, type: 'discovery', focus: 'TECH_INDEPENDENCE',
        title: 'Разобрать технический долг по логам', desc: 'Цель вынесена из расчёта прогресса: берём её, если останется время после P0–P2.', updatedDaysAgo: 21,
        owners: [{ name: 'Мария Ковалёва', initials: 'МК' }], periodId: 3, parentIds: [], childIds: [],
        krs: [
          // Прогресс ни разу не обновляли: значение не записывали (currentRecorded: false),
          // поэтому current равен start. На этом KR проверяется строка подсказки
          // «прогресс ещё не обновляли».
          { id: 1041, weight: 100, name: 'Сервисов с единым форматом логов', desc: '', krType: 'NUMERICAL', unit: 'шт', start: 0, target: 6, current: 0, currentRecorded: false, healthStatus: 'not_started', updatedDaysAgo: 21, progressDaysAgo: null, note: true },
        ],
        comments: [],
      },
    ],
  },
  12: {
    status: 'in_progress', changedAt: '2 июл.',
    goals: [
      {
        id: 201, priority: 'P0', weight: 50, type: 'delivery', focus: 'STABILITY',
        title: 'Оценка приложения в сторах не ниже 4,6', desc: 'Средневзвешенная по App Store и Google Play.', updatedDaysAgo: 6,
        owners: [{ name: 'Иван Петров', initials: 'ИП' }], periodId: 3, parentIds: [401], childIds: [], shareTeams: null,
        krs: [
          { id: 2011, weight: 70, name: 'Средняя оценка', desc: '', krType: 'NUMERICAL', unit: '', start: 4.1, target: 4.6, current: 4.22, healthStatus: 'at_risk', updatedDaysAgo: 6 },
          { id: 2012, weight: 30, name: 'Доля падений ниже 0,3%', desc: '', krType: 'NUMERICAL', unit: '%', start: 1.4, target: 0.3, current: 0.9, healthStatus: 'on_track', updatedDaysAgo: 4 },
        ],
        comments: [
          { id: 9101, author: 'Мария Ковалёва', initials: 'МК', date: '3 дня назад', text: 'Оценка в Google Play просела после релиза 4.12 — стоит посмотреть отзывы.', resolved: false, replies: [] },
        ],
      },
      {
        id: 202, priority: 'P1', weight: 40, type: 'discovery', focus: 'PROFITABILITY',
        title: 'Запуск офлайн-режима', desc: '', updatedDaysAgo: 15,
        owners: [], periodId: 3, parentIds: [401, 102], childIds: [], shareTeams: null,
        krs: [
          { id: 2021, weight: 100, name: 'Офлайн-режим в проде', desc: '', krType: 'PROJECT', healthStatus: 'at_risk', updatedDaysAgo: 15,
            // Двенадцать шагов: на них проверяется предел перечисления в подсказке (8 + счётчик).
            stages: [{ title: 'Хранилище', weight: 20, done: true }, { title: 'Схема данных', weight: 10, done: true }, { title: 'Очередь изменений', weight: 10, done: false }, { title: 'Синхронизация', weight: 10, done: false }, { title: 'Разрешение конфликтов', weight: 10, done: false }, { title: 'Фоновая догрузка', weight: 5, done: false }, { title: 'Индикатор офлайна', weight: 5, done: false }, { title: 'Экраны без сети', weight: 5, done: false }, { title: 'Телеметрия', weight: 5, done: false }, { title: 'Бета на 5%', weight: 5, done: false }, { title: 'Раскатка 50%', weight: 5, done: false }, { title: 'Раскатка 100%', weight: 10, done: false }] },
        ],
        comments: [],
      },
    ],
  },
  13: {
    status: 'forming', changedAt: 'сегодня',
    goals: [
      {
        id: 301, priority: 'P2', weight: 100, type: 'discovery', focus: 'TECH_INDEPENDENCE',
        title: 'Витрина удержания в проде', desc: 'Единый источник для всех продуктовых команд.', updatedDaysAgo: 0,
        owners: [{ name: 'Мария Ковалёва', initials: 'МК' }], periodId: 3, parentIds: [103], childIds: [], shareTeams: null,
        krs: [
          { id: 3011, weight: 100, name: 'Витрина dm_retention доступна', desc: '', krType: 'BOOLEAN', done: true, healthStatus: 'done', updatedDaysAgo: 0 },
        ],
        comments: [],
      },
    ],
  },
  14: { status: 'no_goals', changedAt: '—', goals: [] },
  15: {
    status: 'closed', changedAt: '3 дня назад',
    goals: [
      {
        id: 401, priority: 'P1', weight: 60, type: 'delivery', focus: 'STABILITY',
        title: 'Аптайм дежурного контура 99,95%', desc: '', updatedDaysAgo: 3,
        owners: [{ name: 'Дмитрий Смирнов', initials: 'ДС' }], periodId: 1, parentIds: [], childIds: [201, 202], shareTeams: null,
        krs: [
          { id: 4011, weight: 100, name: 'Аптайм за квартал', desc: '', krType: 'NUMERICAL', unit: '%', start: 99.5, target: 99.95, current: 99.94, healthStatus: 'done', updatedDaysAgo: 3 },
        ],
        comments: [],
      },
      {
        id: 402, priority: 'P3', weight: 30, type: 'delivery', focus: 'SPEED_EFFICIENCY',
        title: 'Замена сетевого оборудования в DC2', desc: '', updatedDaysAgo: 10,
        owners: [], periodId: 3, parentIds: [], childIds: [], shareTeams: null,
        krs: [
          { id: 4021, weight: 100, name: 'Стойки переведены', desc: '', krType: 'NUMERICAL', unit: 'шт', start: 0, target: 8, current: 8, healthStatus: 'done', updatedDaysAgo: 10 },
        ],
        comments: [],
      },
    ],
  },
};

const NOTIFICATIONS = [
  { id: 1, title: 'Анна Петрова отметилась по KR', body: '● On Track → ▲ At Risk. «Интеграция откладывается до следующего спринта»', time: '12 мин', context: 'Платформенная команда · Q3-Y26', unread: true },
  { id: 2, title: 'Запрос доступа к пространству', body: 'Иван Петров просит доступ к пространству «Платформа»', time: '2 ч', context: 'Администрирование', unread: true },
  { id: 3, title: 'Новый комментарий к цели', body: '«Давайте поднимем цель на уровень департамента»', time: 'вчера', context: 'Единый онбординг сервисов', unread: false },
];

// ── Обзор периода ────────────────────────────────────────────────────────────
// Константы и структура повторяют web/static/period_overview_view.js.
const PO = { cardBorder: '#e5e7eb', hairline: '#f1f5f9', headingFg: '#0f172a', mutedFg: '#6b7280', dimFg: '#9ca3af', accent: ACCENT, danger: '#b91c1c' };

const PO_STATUS_TILES = [
  { key: 'no_goals',    label: 'Нет целей',   dot: '#cbd5e1', color: '#64748b' },
  { key: 'forming',     label: 'Черновик',    dot: '#f59e0b', color: '#92400e' },
  { key: 'ready',       label: 'К валидации', dot: '#3b82f6', color: '#1e40af' },
  { key: 'in_progress', label: 'В работе',    dot: '#22c55e', color: '#166534' },
  { key: 'closed',      label: 'Закрыто',     dot: '#9ca3af', color: '#4b5563' },
];

const PO_DD_LABELS = { delivery: 'Delivery', discovery: 'Discovery' };
const PO_DD_COLORS = { delivery: '#475569', discovery: '#7c6cf0' };
const PO_FOCUS_LABELS = FOCUS_LABEL;
const PO_FOCUS_COLORS = { PROFITABILITY: '#ef4444', STABILITY: '#22c55e', SPEED_EFFICIENCY: '#f59e0b', TECH_INDEPENDENCE: '#7c6cf0' };
const PO_PRIO_LABELS = { P0: 'P0 · критично', P1: 'P1 · высокий', P2: 'P2 · средний', P3: 'P3 · низкий' };
const PO_PRIO_COLORS = { P0: '#dc2626', P1: '#f59e0b', P2: '#3b82f6', P3: '#94a3b8' };
const PO_HEALTH_LABELS = KR_HEALTH_LABEL;
const PO_HEALTH_COLORS = KR_HEALTH_COLOR;

const PROGRESS_SERIES = {
  period_start: '2026-07-01',
  period_end: '2026-09-30',
  points: [
    { date: '2026-06-24', progress: 4 },
    { date: '2026-07-14', progress: 12 },
    { date: '2026-07-29', progress: 21 },
    { date: '2026-08-12', progress: 33 },
    { date: '2026-08-26', progress: 44 },
    { date: '2026-09-09', progress: 52 },
    { date: '2026-09-23', progress: 58 },
  ],
};

// Пользователи для выбора драйвера цели (UserSelector). led_team — команда,
// которой человек руководит; в выпадающем списке она показана под именем.
const USERS = [
  { udid: 'u1', name: 'Анна Петрова', initials: 'АП', led_team: 'Платформенная команда' },
  { udid: 'u2', name: 'Сергей Иванов', initials: 'СИ', led_team: 'Платформенная команда' },
  { udid: 'u3', name: 'Иван Петров', initials: 'ИП', led_team: 'Мобильная разработка' },
  { udid: 'u4', name: 'Мария Ковалёва', initials: 'МК', led_team: 'Аналитика' },
  { udid: 'u5', name: 'Дмитрий Смирнов', initials: 'ДС', led_team: 'Инфраструктура' },
  { udid: 'u6', name: 'Ольга Никитина', initials: 'ОН', led_team: '' },
  { udid: 'u7', name: 'Артём Волков', initials: 'АВ', led_team: '' },
  { udid: 'u8', name: 'Екатерина Соболева', initials: 'ЕС', led_team: '' },
];

// ── Лог активностей ──────────────────────────────────────────────────────────
// События в том виде, в каком их отдаёт GET /api/v1/activity. Даты задаются
// сдвигом от текущего момента, чтобы группировка «сегодня / вчера / ранее»
// работала при любом открытии прототипа.
const CATEGORY_ICON = { progress: '📈', composition: '🧩', status: '🚦', discussion: '💬' };
const CATEGORY_LABEL = { progress: 'Прогресс', composition: 'Состав целей', status: 'Статусы и риски', discussion: 'Обсуждения' };
const STATUS_RU = { no_goals: 'Нет целей', forming: 'Черновик', ready: 'К валидации', in_progress: 'В работе', validated: 'Валидировано', closed: 'Закрыто' };

const _ago = (days, hours = 0) => new Date(Date.now() - days * 864e5 - hours * 36e5).toISOString();

const ACTIVITY = [
  { id: 1, category: 'progress', action: 'kr_progress', actor: { display_name: 'Анна Петрова' }, team_id: 11, period_id: 3,
    entity_title: 'Доля активных пользователей в неделю', created_at: _ago(0, 1),
    payload: { before: { progress: 64 }, after: { progress: 78 }, goal_title: 'Отказ от Маркетинговой Админки' } },
  { id: 2, category: 'discussion', action: 'comment_added', actor: { display_name: 'Сергей Иванов' }, team_id: 11, period_id: 3,
    entity_title: 'Отказ от Маркетинговой Админки', created_at: _ago(0, 3),
    payload: { text: 'Пересчитал по новой методике — расхождение `0,4 п.п.`\n\n- витрина обновлена\n- дашборд перепривязан' } },
  { id: 3, category: 'status', action: 'status_changed', actor: { display_name: 'Иван Петров' }, team_id: 12, period_id: 3,
    entity_title: 'Мобильная разработка', created_at: _ago(0, 5),
    payload: { before: { status: 'forming' }, after: { status: 'in_progress' } } },
  { id: 4, category: 'composition', action: 'kr_created', actor: { display_name: 'Мария Ковалёва' }, team_id: 13, period_id: 3,
    entity_title: 'Витрина dm_retention доступна', created_at: _ago(1, 2), payload: {} },
  { id: 5, category: 'progress', action: 'kr_progress', actor: { display_name: 'Дмитрий Смирнов' }, team_id: 15, period_id: 3,
    entity_title: 'Стойки переведены', created_at: _ago(1, 6),
    payload: { before: { progress: 75 }, after: { progress: 100 }, goal_title: 'Замена сетевого оборудования в DC2' } },
  { id: 6, category: 'composition', action: 'goal_shared', actor: { display_name: 'Сергей Иванов' }, team_id: 11, period_id: 1,
    entity_title: 'Единый онбординг сервисов', created_at: _ago(1, 9),
    payload: { shared_with_team_ids: [12, 13] } },
  { id: 7, category: 'discussion', action: 'reply_added', actor: { display_name: 'Анна Петрова' }, team_id: 11, period_id: 3,
    entity_title: 'Отказ от Маркетинговой Админки', created_at: _ago(2, 1),
    payload: { text: 'Согласна, оставляем как есть до конца квартала.' } },
  { id: 8, category: 'composition', action: 'goal_fields_changed', actor: { display_name: 'Иван Петров' }, team_id: 12, period_id: 3,
    entity_title: 'Оценка приложения в сторах не ниже 4,6', created_at: _ago(2, 4),
    payload: { changed: ['weight', 'priority'] } },
  { id: 9, category: 'discussion', action: 'comment_resolved', actor: { display_name: 'Сергей Иванов' }, team_id: 11, period_id: 3,
    entity_title: 'Отказ от старого кабинета Медиа', created_at: _ago(3, 2), payload: {} },
  { id: 10, category: 'progress', action: 'kr_progress', actor: { display_name: 'Иван Петров' }, team_id: 12, period_id: 3,
    entity_title: 'Средняя оценка', created_at: _ago(3, 7),
    payload: { before: { progress: 18 }, after: { progress: 24 }, goal_title: 'Оценка приложения в сторах не ниже 4,6' } },
  { id: 11, category: 'composition', action: 'goal_created', actor: { display_name: 'Мария Ковалёва' }, team_id: 13, period_id: 3,
    entity_title: 'Витрина удержания в проде', created_at: _ago(4, 3), payload: {} },
  { id: 12, category: 'status', action: 'status_changed', actor: { display_name: 'Дмитрий Смирнов' }, team_id: 15, period_id: 3,
    entity_title: 'Инфраструктура', created_at: _ago(5, 1),
    payload: { before: { status: 'in_progress' }, after: { status: 'closed' } } },
  { id: 13, category: 'composition', action: 'kr_deleted', actor: { display_name: 'Анна Петрова', removed: true }, team_id: 11, period_id: 3,
    entity_title: 'Старый счётчик конверсии', created_at: _ago(6, 5), payload: {} },
  { id: 14, category: 'discussion', action: 'comment_added', actor: { display_name: 'Мария Ковалёва' }, team_id: 12, period_id: 3,
    entity_title: 'Запуск офлайн-режима', created_at: _ago(9, 2),
    payload: { text: 'Синхронизация упирается в лимиты API — нужен разговор с платформой.' } },
  { id: 15, category: 'composition', action: 'goal_unshared', actor: { display_name: 'Сергей Иванов' }, team_id: 11, period_id: 1,
    entity_title: 'Единый онбординг сервисов', created_at: _ago(12, 4),
    payload: { unshared_team_ids: [13] } },
  { id: 16, category: 'progress', action: 'kr_note_updated', actor: { display_name: 'Дмитрий Смирнов' }, team_id: 15, period_id: 1,
    entity_title: 'Аптайм за квартал', created_at: _ago(14, 6), payload: {} },
  { id: 17, category: 'composition', action: 'goal_owner_changed', actor: { display_name: 'Иван Петров' }, team_id: 12, period_id: 3,
    entity_title: 'Запуск офлайн-режима', created_at: _ago(18, 1), payload: {} },
  { id: 18, category: 'status', action: 'status_changed', actor: { display_name: 'Анна Петрова' }, team_id: 11, period_id: 3,
    entity_title: 'Платформенная команда', created_at: _ago(21, 3),
    payload: { before: { status: 'no_goals' }, after: { status: 'forming' } } },
];
