-- Demo seed data for OKR Tracker
-- Applies on a clean database after all migrations have run.
--
-- Usage:
--   psql -U postgres -d okrs -f seed_demo.sql
-- or via Docker Compose:
--   docker compose exec -T db psql -U postgres -d okrs < seed_demo.sql

BEGIN;

-- ----------------------------------------------------------------
-- Default tenant (created by migration 027). Seeded teams/periods/goals omit tenant_id
-- and default to 1; ensure it exists for standalone seed runs.
-- ----------------------------------------------------------------
INSERT INTO tenants (id, slug, name) OVERRIDING SYSTEM VALUE
VALUES (1, 'default', 'Default')
ON CONFLICT (id) DO NOTHING;

-- Per-tenant product settings (new_user_policy, documentation_url, feedback_*,
-- health_checkin_config, etc.) now live in tenant_settings (tenant_id, key, value_json)
-- since migration 033 — NOT in the global system_settings. The demo seed does not write
-- product settings; any future seeded product key must target tenant_settings under
-- tenant #1. system_settings is reserved for global keys (e.g. default_registration_tenant_id).

-- This demo seed is single-tenant: all rows belong to the default tenant. Migration 032
-- dropped the transitional tenant_id DEFAULT 1; restore it for the duration of the seed so
-- the INSERTs below (which omit tenant_id) land in the default tenant.
ALTER TABLE teams                  ALTER COLUMN tenant_id SET DEFAULT 1;
ALTER TABLE periods                ALTER COLUMN tenant_id SET DEFAULT 1;
ALTER TABLE goals                  ALTER COLUMN tenant_id SET DEFAULT 1;
ALTER TABLE goal_shares            ALTER COLUMN tenant_id SET DEFAULT 1;
ALTER TABLE team_period_statuses   ALTER COLUMN tenant_id SET DEFAULT 1;
ALTER TABLE key_results            ALTER COLUMN tenant_id SET DEFAULT 1;
ALTER TABLE key_result_notes       ALTER COLUMN tenant_id SET DEFAULT 1;
ALTER TABLE goal_comments          ALTER COLUMN tenant_id SET DEFAULT 1;

-- ----------------------------------------------------------------
-- Clear all tables in FK-safe order
-- ----------------------------------------------------------------
TRUNCATE TABLE
  kr_project_stages,
  kr_boolean_meta,
  key_result_notes,
  goal_comments,
  key_results,
  goal_shares,
  team_period_statuses,
  goals,
  periods,
  teams
CASCADE;

-- ----------------------------------------------------------------
-- Teams  (hierarchy: cluster → unit → team)
-- ----------------------------------------------------------------
INSERT INTO teams (id, name, team_type, parent_id, lead, description, created_at, updated_at) VALUES
  -- Root clusters
  (5,  'Разработка',                        'cluster', NULL, '',    'Кластер инженерных команд: платформа, сервисы и продуктовая разработка.', NOW(), NOW()),
  (6,  'Продукт1',                          'cluster', NULL, '...', '', NOW(), NOW()),
  -- Units inside Разработка
  (2,  'Платформа',                         'unit',    5,    '',    'Ядро платформы: backend, инфраструктура и данные рекламных продуктов.', NOW(), NOW()),
  (18, 'Promo',                             'unit',    5,    '...', '', NOW(), NOW()),
  (22, 'Сервисы',                           'unit',    5,    '',    'Продуктовые сервисы рекламной платформы: медиа, финансы и интеграции.', NOW(), NOW()),
  -- Teams inside Платформа
  (3,  'DWH',                               'team',    2,    '...', '', NOW(), NOW()),
  (4,  'SRE',                               'team',    2,    '...', '', NOW(), NOW()),
  (19, 'CoreDEV',                           'team',    2,    '...', '', NOW(), NOW()),
  (20, 'CoreQA',                            'team',    2,    '...', '', NOW(), NOW()),
  (21, 'PaaS \ Infra',                      'team',    2,    '...', '', NOW(), NOW()),
  -- Teams inside Сервисы
  (23, 'Media',                             'team',    22,   '',    '', NOW(), NOW()),
  (24, 'Finance',                           'team',    22,   '',    '', NOW(), NOW()),
  (25, 'Internal',                          'team',    22,   '',    '', NOW(), NOW()),
  (26, 'SSP',                               'team',    22,   '',    '', NOW(), NOW()),
  (27, 'Old School',                        'team',    22,   '',    '', NOW(), NOW()),
  -- Unit inside Продукт1
  (7,  'Группа "ЛК Продвижение/перформанс"','unit',    6,    '',    '', NOW(), NOW()),
  -- Teams inside that unit
  (8,  'Команда 2',                         'team',    7,    '',    '', NOW(), NOW()),
  (9,  'Команда 1',                         'team',    7,    '',    '', NOW(), NOW()),
  (10, 'Команда 3',                         'team',    7,    '',    '', NOW(), NOW());

-- ----------------------------------------------------------------
-- Periods
-- ----------------------------------------------------------------
INSERT INTO periods (id, name, start_date, end_date, archived_at, created_at, updated_at) VALUES
  (2, 'Y2026',       '2026-01-01', '2026-12-31', NULL, NOW(), NOW()),
  (1, 'Q1 · 2026',   '2026-01-01', '2026-03-31', NULL, NOW(), NOW()),
  (3, 'Q2 · 2026',   '2026-04-01', '2026-06-30', NULL, NOW(), NOW()),
  (4, 'Q3 · 2026',   '2026-07-01', '2026-09-30', NULL, NOW(), NOW()),
  (5, 'Q4 · 2026',   '2026-10-01', '2026-12-31', NULL, NOW(), NOW()),
  (6, 'Y2025',       '2025-01-01', '2025-12-31', NULL, NOW(), NOW()),
  (7, 'Q3 · 2025',   '2025-07-01', '2025-09-30', NULL, NOW(), NOW()),
  (8, 'Q4 · 2025',   '2025-10-01', '2025-12-31', NULL, NOW(), NOW()),
  (9, 'Q2 · 2025',   '2025-04-01', '2025-06-30', NOW(), NOW(), NOW());

-- ----------------------------------------------------------------
-- Goals
-- ----------------------------------------------------------------
INSERT INTO goals (id, team_id, period_id, title, description, priority, weight, work_type, focus_type, owner_text, sort_order, created_at, updated_at) VALUES
  -- Платформа (team 2) → Q3 · 2026 (текущий квартал)
  (32, 2, 4, 'Снизить P95 latency до 200ms',        '', 'P1', 35, 'Delivery',  'EFFICIENCY',   '...', 1, NOW(), NOW()),
  (33, 2, 4, 'Покрыть 80% кода тестами',            '', 'P1', 30, 'Delivery',  'QUALITY',      '...', 2, NOW(), NOW()),
  (34, 2, 4, 'Запустить портал документации',       '', 'P2', 20, 'Discovery', 'QUALITY',      '...', 3, NOW(), NOW()),
  -- DWH (team 3) → Q1 · 2026 (закрытый квартал, история)
  (35, 3, 1, 'Обеспечить 99.9% uptime DWH пайплайнов', '', 'P0', 40, 'Discovery', 'PROFITABILITY', 'Гоша', 1, NOW(), NOW()),
  (36, 3, 1, 'Перевести DWH на ClickHouse 24',      '', 'P0', 25, 'Discovery', 'PROFITABILITY', 'Гоша', 2, NOW(), NOW()),
  -- SRE (team 4) → Q3 · 2026 (текущий квартал)
  (9,  4, 4, 'Система мониторинга как основной источник информации о инцидентах', '', 'P0', 25, 'Discovery', 'PROFITABILITY', '...', 1, NOW(), NOW()),
  (10, 4, 4, 'Аналитика и исследование инцидентов на регулярной основе',         '', 'P1', 35, 'Delivery',  'STABILITY',     '...', 2, NOW(), NOW()),
  (8,  4, 4, 'Запуск и обеспечение функционирования режима оперативного дежурства 24/7', '', 'P0', 20, 'Discovery', 'PROFITABILITY', '...', 3, NOW(), NOW()),
  (37, 4, 4, 'Внедрить IaC для всей инфраструктуры', '', 'P1', 50, 'Delivery',  'EFFICIENCY',   '...', 4, NOW(), NOW()),
  (38, 4, 4, 'Сократить MTTR до 30 минут',          '', 'P1', 35, 'Delivery',  'RELIABILITY',  '...', 5, NOW(), NOW()),
  -- PaaS\Infra (team 21) → Q3 · 2026 (текущий квартал)
  (26, 21, 4, 'цель 1', '', 'P0', 25, 'Discovery', 'PROFITABILITY', '', 1, NOW(), NOW()),
  (27, 21, 4, 'цель 2', '', 'P0', 75, 'Discovery', 'PROFITABILITY', '', 2, NOW(), NOW()),
  -- Media (team 23) → Q4 · 2026 (будущий квартал, планирование)
  (29, 23, 5, 'цель2',                                                            '', 'P0', 100, 'Discovery', 'PROFITABILITY', 'Гоша', 1, NOW(), NOW()),
  (30, 23, 5, 'Подключить funsun к сетке для опробирования полного цикла сетки', '', 'P0', 20,  'Discovery', 'PROFITABILITY', 'Гоша', 2, NOW(), NOW());

-- ----------------------------------------------------------------
-- Goal shares
-- ----------------------------------------------------------------
INSERT INTO goal_shares (goal_id, team_id, weight, sort_order, created_at, updated_at) VALUES
  -- Goal 30 (Media/funsun) shared to Продукт1 cluster
  (30, 6,  0, 3, NOW(), NOW()),
  -- Goal 32 (Платформа latency) shared to Разработка cluster
  (32, 5, 15, 1, NOW(), NOW());

-- ----------------------------------------------------------------
-- Team-period statuses
-- ----------------------------------------------------------------
INSERT INTO team_period_statuses (team_id, period_id, status) VALUES
  (2,  4, 'in_progress'),
  (3,  1, 'closed'),
  (4,  4, 'in_progress'),
  (5,  4, 'forming'),
  (21, 4, 'ready'),
  (23, 5, 'forming');

-- ----------------------------------------------------------------
-- Key Results
-- ----------------------------------------------------------------
INSERT INTO key_results (id, goal_id, title, description, weight, kind, sort_order, created_at, updated_at) VALUES
  -- Goal 8 (SRE: дежурства 24/7)
  (7,  8, 'Внедрен регламент дежурств во все юниты',
      'Разработан, согласован со всеми заинтересованными сторонами и утвержден регламент дежурств 24/7.',
      34, 'BOOLEAN', 1, NOW(), NOW()),
  (8,  8, 'Внедрены инструкции для дежурных инженеров',
      'Составлены и внедрены подробные инструкции для дежурных инженеров (runbooks) по действиям при различных типах инцидентов.',
      33, 'BOOLEAN', 2, NOW(), NOW()),
  (9,  8, 'Каллендарь дежурств внедрен в 5 команд',
      'Составлен и опубликован календарь дежурств команды SRE и до 5 тестовых (early-adopter) команд.',
      33, 'NUMERICAL', 3, NOW(), NOW()),
  -- Goal 9 (SRE: система мониторинга)
  (10, 9, 'Инцидент менеджмент для P0\P1 автоматизирован',
      'Реализован автоматический сценарий, который при поступлении алерта P0/P1 автоматизирует обработку инцидента.',
      25, 'PROJECT', 1, NOW(), NOW()),
  (11, 9, 'Система мониторинга покрывает 75% критических алертов',
      'Система мониторинга покрывает 75% критических алертов и включает в себя ранбуки для работы с ними.',
      35, 'NUMERICAL', 2, NOW(), NOW()),
  (14, 9, 'Автоматизация протестирована на тестовых инцидентах',
      'Автоматизированный процесс протестирован на учебных инцидентах и подтверждена его стабильная работа.',
      35, 'BOOLEAN', 3, NOW(), NOW()),
  -- Goal 10 (SRE: аналитика инцидентов)
  (15, 10, 'Создана регулярная встреча по аналитике инцидентов за прошедший период',
       'Проведена и регулярно повторяется групповая встреча со всеми тимлидами и юнит-лидами.',
       25, 'BOOLEAN', 1, NOW(), NOW()),
  (16, 10, 'Создан план по работе с историческими данными по инцидентам',
       'Создан роадмап по работе с историческими данными по инцидентам и их предотвращению.',
       25, 'BOOLEAN', 2, NOW(), NOW()),
  (17, 10, 'Внедрен реестр инцидентов',
       'Реализована точка правды по информации о инцидентах (потери, длительность, влияние и т.п.).',
       25, 'BOOLEAN', 3, NOW(), NOW()),
  (18, 10, 'Реализован процесс подсчета потерь по инцидентам',
       '',
       25, 'BOOLEAN', 4, NOW(), NOW()),
  -- Goal 26 (PaaS: цель 1)
  (43, 26, 'цель2', '', 100, 'BOOLEAN', 1, NOW(), NOW()),
  -- Goal 27 (PaaS: цель 2)
  (42, 27, 'bool', '', 100, 'NUMERICAL', 1, NOW(), NOW()),
  -- Goal 29 (Media: цель2)
  (45, 29, 'перцент',  '', 51, 'NUMERICAL', 1, NOW(), NOW()),
  (46, 29, 'перцент2', '', 50, 'NUMERICAL', 2, NOW(), NOW()),
  -- Goal 30 (Media: funsun)
  (47, 30, '100% рекламы на сайте крутится через сетку',
       'Как проверим: ссылка на метрику.',
       50, 'NUMERICAL', 1, NOW(), NOW()),
  (48, 30, 'Есть техническая возможность видеть баннер',
       '',
       50, 'PROJECT', 2, NOW(), NOW()),
  -- Goal 32 (Платформа: P95 latency)
  (50, 32, 'P95 latency API gateway',    'Текущее: 450ms → Цель: 200ms', 40, 'NUMERICAL', 1, NOW(), NOW()),
  (51, 32, 'P95 latency auth service',   'Текущее: 380ms → Цель: 200ms', 30, 'NUMERICAL', 2, NOW(), NOW()),
  (52, 32, 'Миграция на HTTP/2',         'Переключить все внутренние вызовы', 20, 'BOOLEAN', 3, NOW(), NOW()),
  -- Goal 33 (Платформа: 80% покрытие)
  (53, 33, 'Покрытие unit-тестами',         '% покрытия core пакетов',    50, 'NUMERICAL', 1, NOW(), NOW()),
  (54, 33, 'Покрытие integration-тестами',  '% покрытия API endpoints',   50, 'NUMERICAL', 2, NOW(), NOW()),
  -- Goal 34 (Платформа: портал документации)
  (55, 34, 'Развернуть Confluence/Notion',  'Setup и настройка инструмента',     30, 'BOOLEAN', 1, NOW(), NOW()),
  (56, 34, 'Перенести ADR документы',       'Architecture Decision Records',     40, 'PROJECT', 2, NOW(), NOW()),
  -- Goal 37 (SRE: IaC)
  (57, 37, 'Перевести prod окружение на Terraform', 'Все сервисы prod', 60, 'NUMERICAL', 1, NOW(), NOW()),
  (58, 37, 'Написать модули для баз данных',        'RDS, Redis модули', 25, 'PROJECT', 2, NOW(), NOW()),
  -- Goal 38 (SRE: MTTR)
  (59, 38, 'Настроить runbooks для топ-5 инцидентов', 'По каждому типу инцидента',       50, 'PROJECT', 1, NOW(), NOW()),
  (60, 38, 'Среднее время реакции on-call',           'Текущее: 45 мин → Цель: 30 мин', 50, 'NUMERICAL', 2, NOW(), NOW());

-- ----------------------------------------------------------------
-- KR meta — BOOLEAN
-- ----------------------------------------------------------------
INSERT INTO kr_boolean_meta (key_result_id, is_done) VALUES
  (7,  false),
  (8,  true),
  (14, false),
  (15, false),
  (16, false),
  (17, true),
  (18, false),
  (43, true),
  (52, false),
  (55, false);

-- ----------------------------------------------------------------
-- KR meta — NUMERICAL (start/target/current/unit stored on key_results)
-- ----------------------------------------------------------------
UPDATE key_results SET start_value = v.start, target_value = v.target, current_value = v.current, unit = v.unit
FROM (VALUES
  (9,  0,   5,   2,   'шт'),
  (11, 10,  75,  50,  '%'),
  (45, 0,   100, 51,  '%'),
  (46, 0,   100, 10,  '%'),
  (47, 0,   100, 0,   '%'),
  (53, 0,   100, 0,   '%'),
  (54, 0,   100, 0,   '%'),
  (57, 0,   100, 0,   '%'),
  (42, 0,   100, 50,  '%'),
  (50, 450, 200, 450, 'мс'),
  (51, 380, 200, 380, 'мс'),
  (60, 45,  30,  45,  'мин')
) AS v(kr_id, start, target, current, unit)
WHERE key_results.id = v.kr_id;

-- ----------------------------------------------------------------
-- KR meta — PROJECT stages
-- ----------------------------------------------------------------
INSERT INTO kr_project_stages (id, key_result_id, title, weight, is_done, sort_order) VALUES
  -- KR 10 (автоматизация инцидентов P0/P1)
  (19, 10, 'Создает тикет инцидента в заданной системе (Jira, Youtrack и т.п.) с предзаполненными полями',  33, true,  1),
  (20, 10, 'Создает отдельный чат для коммуникации по инциденту',                                           33, true,  2),
  (21, 10, 'Автоматически приглашает в созданный чат рабочую группу по данному типу инцидентов',           34, true,  3),
  -- KR 48 (Есть техническая возможность видеть баннер)
  (58, 48, 'подключен SDK',                           20, false, 1),
  (59, 48, 'настроен кабинет под все виды рекламы',   30, false, 2),
  (60, 48, 'подписан договор с внешним сайтом',       50, false, 3);

-- ----------------------------------------------------------------
-- KR health status (manual). Completed KRs (progress 100%) are 'done'
-- to stay consistent with the 100%→done rule; a few others show variety.
-- The rest keep the column default 'not_started'.
-- ----------------------------------------------------------------
UPDATE key_results SET health_status = 'done'     WHERE id IN (8, 17, 43, 10);
UPDATE key_results SET health_status = 'on_track' WHERE id IN (9, 11, 45, 42, 53, 54);
UPDATE key_results SET health_status = 'at_risk'  WHERE id IN (50, 51, 60);

-- ----------------------------------------------------------------
-- Goal comments
-- ----------------------------------------------------------------
-- author_user_id = 1 (system:anonymous-local), required NOT NULL since migration 017.
INSERT INTO goal_comments (id, goal_id, text, author_user_id, created_at) VALUES
  (5, 32, 'Обновление по задаче выполнено.', 1, NOW()),
  (7, 37, 'Обновление по задаче выполнено.', 1, NOW());

-- ================================================================
-- OKR Catalog — департамент «Реклама» (Y2026)
-- Отдельный корневой узел (id 100), полностью отделимый от данных
-- выше: teams 100-112, goals 100-116, key_results 100-132.
-- ================================================================

-- Teams: Реклама → Разработка → {Платформа, Продвижение}
INSERT INTO teams (id, name, team_type, parent_id, lead, description, created_at, updated_at) VALUES
  (100, 'Реклама',           'department', NULL, '',                'Рекламный департамент: каталог целей Y26 Q1.', NOW(), NOW()),
  (101, 'Разработка',        'cluster',    100,  '',                'Кластер инженерных команд рекламного департамента.', NOW(), NOW()),
  (102, 'Платформа',         'unit',       101,  '',                'Ядро платформы: backend, инфраструктура и данные.', NOW(), NOW()),
  (103, 'Backend & Quality', 'group',      102,  'Дмитрий Петров',  'Стандарты кода и процессов для Backend и QA команд.', NOW(), NOW()),
  (104, 'CoreDEV',           'team',       103,  'Сергей Лебедев',  '', NOW(), NOW()),
  (105, 'API Platform',      'squad',      104,  'Сергей Лебедев',  '', NOW(), NOW()),
  (106, 'CoreQA',            'team',       103,  'Елена Смирнова',  '', NOW(), NOW()),
  (107, 'Infrastructure',    'group',      102,  '',                '', NOW(), NOW()),
  (108, 'PaaS \ Infra',      'team',       107,  'Андрей Николаев', '', NOW(), NOW()),
  (109, 'SRE',               'team',       107,  'Кирилл Воронов',  '', NOW(), NOW()),
  (110, 'DWH',               'team',       102,  'Михаил Козлов',   '', NOW(), NOW()),
  (111, 'Продвижение',       'unit',       101,  '',                '', NOW(), NOW()),
  (112, 'Перформанс',        'team',       111,  'Роман Белов',     '', NOW(), NOW());

-- Goals → Y2026 (годовые цели департамента «Реклама»)
INSERT INTO goals (id, team_id, period_id, title, description, priority, weight, work_type, focus_type, owner_text, sort_order, created_at, updated_at) VALUES
  -- Платформа (102)
  (100, 102, 2, 'Снизить P95 latency до 200ms',          'Оптимизировать критические пути запросов',                'P1', 35, 'Delivery',  'EFFICIENCY',    'Алексей Иванов',   1, NOW(), NOW()),
  (101, 102, 2, 'Покрыть 80% кода тестами',              'Повысить надёжность через автоматизацию',                 'P1', 30, 'Delivery',  'QUALITY',       'Елена Смирнова',   2, NOW(), NOW()),
  (102, 102, 2, 'Запустить портал документации',         'Единое место для архитектурных решений',                  'P2', 20, 'Discovery', 'RELIABILITY',   'Михаил Козлов',    3, NOW(), NOW()),
  (103, 102, 2, 'Снизить Time-to-Deploy до 15 мин',      'Ускорить CI/CD пайплайн',                                 'P2', 15, 'Delivery',  'EFFICIENCY',    'Андрей Николаев',  4, NOW(), NOW()),
  -- Backend & Quality (103)
  (104, 103, 2, 'Единая культура инженерного качества',  'Стандарты кода и процессов для Backend и QA команд',      'P1', 100,'Discovery', 'QUALITY',       'Дмитрий Петров',   1, NOW(), NOW()),
  -- CoreDEV (104)
  (105, 104, 2, 'Migrate to Go 1.22',                    'Обновить все сервисы, использовать новые возможности языка','P1',50, 'Delivery',  'RELIABILITY',   'Сергей Лебедев',   1, NOW(), NOW()),
  (106, 104, 2, 'Рефакторинг auth модуля',               'Выделить в отдельный сервис',                             'P2', 50, 'Delivery',  'QUALITY',       'Ольга Фёдорова',   2, NOW(), NOW()),
  -- API Platform (105)
  (107, 105, 2, 'Запустить новую версию API Gateway v2', 'Рефакторинг маршрутизации, поддержка gRPC',               'P1', 70, 'Delivery',  'RELIABILITY',   'Сергей Лебедев',   1, NOW(), NOW()),
  (108, 105, 2, 'Документация API для партнёров',        '',                                                        'P2', 30, 'Discovery', 'GROWTH',        'Михаил Козлов',    2, NOW(), NOW()),
  -- CoreQA (106)
  (109, 106, 2, 'Автоматизировать регрессионное тестирование','Покрыть 200 ключевых user flows',                    'P1', 100,'Delivery',  'QUALITY',       'Елена Смирнова',   1, NOW(), NOW()),
  -- PaaS \ Infra (108)
  (110, 108, 2, 'Перевести 80% сервисов на k8s',         '',                                                        'P1', 70, 'Delivery',  'EFFICIENCY',    'Андрей Николаев',  1, NOW(), NOW()),
  (111, 108, 2, 'SLA платформ 99.5%',                    '',                                                        'P2', 30, 'Delivery',  'RELIABILITY',   'Андрей Николаев',  2, NOW(), NOW()),
  -- SRE (109)
  (112, 109, 2, '99.9% uptime всех production сервисов',  'Обеспечить надёжность инфраструктуры',                    'P0', 60, 'Delivery',  'RELIABILITY',   'Кирилл Воронов',   1, NOW(), NOW()),
  (113, 109, 2, 'MTTR < 10 мин для P0 инцидентов',        'Ускорить реакцию на критические сбои',                    'P1', 40, 'Delivery',  'RELIABILITY',   'Кирилл Воронов',   2, NOW(), NOW()),
  -- DWH (110)
  (114, 110, 2, 'DWH для рекламных метрик',               'Централизованное хранилище',                             'P1', 100,'Discovery', 'GROWTH',        'Михаил Козлов',    1, NOW(), NOW()),
  -- Перформанс (112)
  (115, 112, 2, 'Снизить CPA на 15%',                     'ML-биддинг',                                             'P0', 60, 'Delivery',  'PROFITABILITY', 'Роман Белов',      1, NOW(), NOW()),
  (116, 112, 2, 'ML-биддинг для топ-10 клиентов',         '',                                                        'P1', 30, 'Discovery', 'GROWTH',        'Роман Белов',      2, NOW(), NOW());

-- Team-period statuses for the Реклама catalog (period 2 = Y2026)
INSERT INTO team_period_statuses (team_id, period_id, status) VALUES
  (102, 2, 'in_progress'),
  (103, 2, 'in_progress'),
  (104, 2, 'in_progress'),
  (105, 2, 'forming'),
  (106, 2, 'in_progress'),
  (108, 2, 'in_progress'),
  (109, 2, 'in_progress'),
  (110, 2, 'forming'),
  (112, 2, 'in_progress');

-- Key Results
INSERT INTO key_results (id, goal_id, title, description, weight, kind, sort_order, created_at, updated_at) VALUES
  -- Goal 100 (P95 latency)
  (100, 100, 'P95 latency API gateway',  'Замеряем по Prometheus, окно 5 минут. Считаем latency на edge-роутах /api/v1/*.', 40, 'NUMERICAL', 1, NOW(), NOW()),
  (101, 100, 'P95 latency auth service', 'Внутренние gRPC-вызовы auth.Verify и auth.Refresh, без учёта таймаутов клиента.', 40, 'NUMERICAL', 2, NOW(), NOW()),
  (102, 100, 'Миграция на HTTP/2',       'Все внешние эндпоинты переведены на HTTP/2 с поддержкой fallback на HTTP/1.1.',   20, 'PROJECT',   3, NOW(), NOW()),
  -- Goal 101 (покрытие тестами)
  (103, 101, 'Unit test coverage CoreDEV', 'Покрытие по go test -cover для пакетов core/*, без учёта моков и сгенерированного кода. Цель в 80% относится только к бизнес-логике.', 50, 'NUMERICAL', 1, NOW(), NOW()),
  (104, 101, 'Integration test suite',     '',                                                                              20, 'BOOLEAN',   2, NOW(), NOW()),
  (105, 101, 'QA automation pipeline',     '',                                                                              20, 'PROJECT',   3, NOW(), NOW()),
  -- Goal 102 (портал документации)
  (106, 102, 'Architecture decisions documented', 'ADR в репозитории docs/adr/, минимум 15 ключевых решений за квартал.', 60, 'NUMERICAL', 1, NOW(), NOW()),
  (107, 102, 'API docs migrated',                 '',                                                                      40, 'NUMERICAL', 2, NOW(), NOW()),
  -- Goal 103 (Time-to-Deploy)
  (108, 103, 'Pipeline время P50 < 15 мин', '',                                                                           70, 'NUMERICAL', 1, NOW(), NOW()),
  (109, 103, 'Параллелизация тестов',        '',                                                                          30, 'BOOLEAN',   2, NOW(), NOW()),
  -- Goal 104 (инженерное качество)
  (110, 104, 'Engineering handbook опубликован', '',                                                                      50, 'BOOLEAN',   1, NOW(), NOW()),
  (111, 104, 'Code review время < 2ч',           '',                                                                      50, 'NUMERICAL', 2, NOW(), NOW()),
  -- Goal 105 (Go 1.22)
  (112, 105, 'Все сервисы обновлены',     'go.mod указывает Go 1.22, CI зелёный, прод-сборка прошла smoke-тесты.',        60, 'NUMERICAL', 1, NOW(), NOW()),
  (113, 105, 'Тесты проходят на Go 1.22', '',                                                                            40, 'NUMERICAL', 2, NOW(), NOW()),
  -- Goal 106 (рефакторинг auth)
  (114, 106, 'Auth service выделен',         '',                                                                          70, 'PROJECT',   1, NOW(), NOW()),
  (115, 106, 'Циклические зависимости устранены', '',                                                                    30, 'BOOLEAN',   2, NOW(), NOW()),
  -- Goal 107 (API Gateway v2)
  (116, 107, 'gRPC транскодинг работает',    '',                                                                          40, 'BOOLEAN',   1, NOW(), NOW()),
  (117, 107, 'Выдерживаемая нагрузка gateway','Нагрузочный тест k6, рост RPS. Прогресс по промежуточным значениям.',      60, 'NUMERICAL', 2, NOW(), NOW()),
  -- Goal 108 (документация партнёрам)
  (118, 108, 'API Reference опубликован',    '',                                                                         100, 'BOOLEAN',   1, NOW(), NOW()),
  -- Goal 109 (регрессионное тестирование)
  (119, 109, 'User flows покрыты авто-тестами','',                                                                        70, 'NUMERICAL', 1, NOW(), NOW()),
  (120, 109, 'CI интеграция настроена',        '',                                                                        30, 'BOOLEAN',   2, NOW(), NOW()),
  -- Goal 110 (k8s)
  (121, 110, 'Сервисов на k8s',              '',                                                                         100, 'NUMERICAL', 1, NOW(), NOW()),
  -- Goal 111 (SLA)
  (122, 111, 'Uptime platform',              '',                                                                         100, 'NUMERICAL', 1, NOW(), NOW()),
  -- Goal 112 (99.9% uptime)
  (123, 112, 'Uptime API gateway',           '',                                                                          50, 'NUMERICAL', 1, NOW(), NOW()),
  (124, 112, 'Uptime core services',         '',                                                                          50, 'NUMERICAL', 2, NOW(), NOW()),
  -- Goal 113 (MTTR)
  (125, 113, 'Runbook покрытие 90%',         '',                                                                          40, 'NUMERICAL', 1, NOW(), NOW()),
  (126, 113, 'On-call rotation настроен',     '',                                                                         30, 'BOOLEAN',   2, NOW(), NOW()),
  (127, 113, 'P50 MTTR < 10 мин',            '',                                                                          30, 'NUMERICAL', 3, NOW(), NOW()),
  -- Goal 114 (DWH)
  (128, 114, 'Схема данных согласована',     '',                                                                          40, 'BOOLEAN',   1, NOW(), NOW()),
  (129, 114, 'Pipeline ежедневных данных',   '',                                                                          60, 'PROJECT',   2, NOW(), NOW()),
  -- Goal 115 (CPA)
  (130, 115, 'CPA (целевой -15%)',           '',                                                                         100, 'NUMERICAL', 1, NOW(), NOW()),
  -- Goal 116 (ML-биддинг)
  (131, 116, 'ML-модель в проде',            '',                                                                          60, 'PROJECT',   1, NOW(), NOW()),
  (132, 116, 'Клиентов подключено',          '',                                                                          40, 'NUMERICAL', 2, NOW(), NOW());

-- KR meta — BOOLEAN
INSERT INTO kr_boolean_meta (key_result_id, is_done) VALUES
  (104, false),
  (109, false),
  (110, false),
  (115, false),
  (116, false),
  (118, false),
  (120, true),
  (126, true),
  (128, true);

-- KR meta — NUMERICAL
UPDATE key_results SET start_value = v.start, target_value = v.target, current_value = v.current, unit = v.unit
FROM (VALUES
  (100, 320.0, 200.0, 278.0, 'мс'),
  (101, 450.0, 200.0, 408.0, 'мс'),
  (103, 0.0,   80.0,  14.0,  '%'),
  (106, 0.0,   100.0, 60.0,  '%'),
  (107, 0.0,   100.0, 10.0,  '%'),
  (108, 48.0,  15.0,  48.0,  'мин'),
  (111, 8.0,   2.0,   5.0,   'час'),
  (112, 0.0,   100.0, 65.0,  '%'),
  (113, 0.0,   100.0, 80.0,  '%'),
  (117, 100.0, 180.0, 170.0, 'RPS'),
  (119, 0.0,   200.0, 76.0,  'шт'),
  (121, 0.0,   80.0,  44.0,  '%'),
  (122, 97.0,  99.5,  98.6,  '%'),
  (123, 98.0,  99.9,  99.2,  '%'),
  (124, 98.0,  99.9,  99.4,  '%'),
  (125, 0.0,   90.0,  41.0,  '%'),
  (127, 35.0,  10.0,  20.0,  'мин'),
  (130, 100.0, 85.0,  97.0,  '%'),
  (132, 0.0,   10.0,  2.0,   '%')
) AS v(kr_id, start, target, current, unit)
WHERE key_results.id = v.kr_id;

-- KR meta — NUMERICAL checkpoints (JSONB [{value, progress_percent}])
UPDATE key_results SET checkpoints = '[{"value":100,"progress_percent":0},{"value":150,"progress_percent":50},{"value":180,"progress_percent":100}]'::jsonb WHERE id = 117;
UPDATE key_results SET checkpoints = '[{"value":50,"progress_percent":25},{"value":100,"progress_percent":50},{"value":150,"progress_percent":75},{"value":200,"progress_percent":100}]'::jsonb WHERE id = 119;

-- KR meta — zeroing criteria (доступно для любого типа KR)
UPDATE key_results SET zeroing_criteria = 'Если латентность достигнута только за счёт отключения проверок безопасности на edge — результат не засчитывается.' WHERE id = 100;
UPDATE key_results SET zeroing_criteria = 'Если хотя бы один внешний эндпоинт откатили обратно на HTTP/1.1 из-за инцидента в проде — результат не засчитывается.' WHERE id = 102;

-- KR meta — PROJECT stages
INSERT INTO kr_project_stages (id, key_result_id, title, weight, is_done, sort_order) VALUES
  -- KR 102 (Миграция на HTTP/2)
  (100, 102, 'Конфигурация nginx обновлена', 40, true,  1),
  (101, 102, 'Тестирование с HTTP/2',        35, false, 2),
  (102, 102, 'Мониторинг метрик',            25, false, 3),
  -- KR 105 (QA automation pipeline)
  (103, 105, 'Тест-раннер настроен',         30, true,  1),
  (104, 105, 'CI интеграция',                40, false, 2),
  (105, 105, 'Репортинг',                    30, false, 3),
  -- KR 114 (Auth service выделен)
  (106, 114, 'Интерфейс auth service',       25, true,  1),
  (107, 114, 'Деплой в dev',                 35, false, 2),
  (108, 114, 'Production миграция',          40, false, 3),
  -- KR 129 (Pipeline ежедневных данных)
  (109, 129, 'Источники подключены',         40, true,  1),
  (110, 129, 'Трансформации настроены',      35, false, 2),
  (111, 129, 'Дашборды готовы',              25, false, 3),
  -- KR 131 (ML-модель в проде)
  (112, 131, 'Модель обучена',               35, true,  1),
  (113, 131, 'A/B тест',                     35, true,  2),
  (114, 131, 'Production',                   30, false, 3);

-- Goal comments (author_user_id = 1, system:anonymous-local)
INSERT INTO goal_comments (id, goal_id, text, author_user_id, created_at) VALUES
  (100, 100, 'Начали профилирование. Нашли узкое место в middleware auth.', 1, NOW()),
  (101, 100, 'Нужно учесть зависимость от PaaS/Infra по HTTP/2-конфигурации nginx.', 1, NOW()),
  (102, 102, 'ADR по 12 ключевым решениям готовы. Миграция API docs с Confluence в процессе.', 1, NOW()),
  (103, 105, 'Осталось 3 сервиса. Основные проблемы с устаревшими зависимостями решены.', 1, NOW()),
  (104, 112, 'Апрельский инцидент устранён. Добавили circuit breaker на 3 критичных точках.', 1, NOW());

-- Ответы (parent_id → таска). Демонстрирует ветку «таска → ответы»: ответ не резолвится.
INSERT INTO goal_comments (id, goal_id, parent_id, text, author_user_id, created_at) VALUES
  (150, 100, 100, 'Согласовали с Infra: HTTP/2 включаем на следующей неделе.', 1, NOW()),
  (151, 100, 101, 'Уточнил у PaaS — конфиг nginx поправят в этом спринте.', 1, NOW());

-- Одно замечание помечено решённым, второе остаётся нерешённым — демонстрирует resolve-флоу.
UPDATE goal_comments SET resolved_at = NOW(), resolved_by_user_id = 1 WHERE id = 100;

-- KR notes (1:1, author_user_id = 1)
INSERT INTO key_result_notes (key_result_id, text, author_user_id, updated_at) VALUES
  (101, 'Нашли узкое место в JWT verify — пул соединений был слишком мал.', 1, NOW()),
  (104, 'Ожидаем инфраструктуру от PaaS — блокер на неделю.', 1, NOW()),
  (112, 'Осталось 3 сервиса. Проблемы с устаревшими зависимостями решены.', 1, NOW());

-- ----------------------------------------------------------------
-- Activity log (демо-лента). actor_user_id = 1 (anonymous-local).
-- id авто-генерируется (IDENTITY) — сброс последовательности не нужен.
-- Времена разнесены (сегодня / вчера / ранее на неделе / старее) для группировки.
-- ----------------------------------------------------------------
INSERT INTO activity_events (tenant_id, actor_user_id, category, action, team_id, period_id, goal_id, kr_id, comment_id, entity_title, payload_json, created_at) VALUES
  -- Сегодня
  (1, 1, 'progress', 'kr_progress', 4, 4, 8, 9, NULL, 'Календарь дежурств внедрен в 5 команд',
     '{"before":{"progress":40},"after":{"progress":65},"kind":"NUMERICAL"}', NOW() - INTERVAL '2 hours'),
  (1, 1, 'discussion', 'comment_added', 2, 4, 32, NULL, 5, 'Снизить P95 latency до 200ms',
     '{"text":"Обновление по задаче выполнено."}', NOW() - INTERVAL '5 hours'),
  (1, 1, 'composition', 'goal_created', 23, 5, 29, NULL, NULL, 'цель2',
     '{}', NOW() - INTERVAL '8 hours'),
  -- Вчера
  (1, 1, 'status', 'status_changed', 21, 4, NULL, NULL, NULL, 'PaaS / Infra',
     '{"before":{"status":"forming"},"after":{"status":"in_progress"}}', NOW() - INTERVAL '1 day 3 hours'),
  (1, 1, 'composition', 'kr_created', 4, 4, 9, 10, NULL, 'Инцидент менеджмент для P0\P1 автоматизирован',
     '{}', NOW() - INTERVAL '1 day 6 hours'),
  -- Ранее на этой неделе
  (1, 1, 'discussion', 'comment_resolved', 4, 4, 37, NULL, 7, 'Внедрить IaC для всей инфраструктуры',
     '{"before":{"resolved":false},"after":{"resolved":true}}', NOW() - INTERVAL '3 days'),
  (1, 1, 'composition', 'goal_shared', 2, 4, 33, NULL, NULL, 'Покрыть 80% кода тестами',
     '{"shared_with_team_ids":[4]}', NOW() - INTERVAL '4 days'),
  -- Старее
  (1, 1, 'composition', 'goal_fields_changed', 4, 4, 38, NULL, NULL, 'Сократить MTTR до 30 минут',
     '{"changed":{"title":{"before":"Сократить MTTR","after":"Сократить MTTR до 30 минут"}}}', NOW() - INTERVAL '6 days'),
  (1, 1, 'progress', 'kr_progress', 3, 1, 35, NULL, NULL, 'Обеспечить 99.9% uptime DWH пайплайнов',
     '{"before":{"progress":80},"after":{"progress":100},"kind":"NUMERICAL"}', NOW() - INTERVAL '12 days');

-- ----------------------------------------------------------------
-- Notifications (миграция 044). ON CONFLICT DO NOTHING — эти INSERT'ы не в TRUNCATE
-- выше, чтобы повторный запуск сида не плодил дубликаты и не терял отметки "прочитано".
-- ----------------------------------------------------------------

-- Демо-настройки уведомлений: один пользователь смотрит всё поддерево,
-- остальные остаются на дефолте (строк нет — дефолт подставляется на чтении).
INSERT INTO notification_preferences (tenant_id, user_id, type, enabled, scope, channels)
SELECT 1, u.id, 'goal_changed', TRUE, 'subtree', '{in_app}'
  FROM users u WHERE u.provider_subject_key = 'system:anonymous-local'
ON CONFLICT DO NOTHING;

-- Пара уведомлений, чтобы колокольчик в демо не был пустым. actor_user_id = 2 —
-- системный пользователь "Migration" (migrations/013), а не anonymous-local: так
-- уведомление визуально отличается от собственного действия получателя.
INSERT INTO notifications
  (tenant_id, user_id, type, kind, actor_user_id, team_id, period_id, goal_id,
   entity_title, payload_json, coalesce_key, coalesce_count)
SELECT 1, 1, 'goal_changed', 'goal_fields_changed', 2, g.team_id, g.period_id, g.id,
       g.title, '{}'::jsonb, 'demo:goal:' || g.id, 1
  FROM goals g WHERE g.tenant_id = 1 ORDER BY g.id LIMIT 2
ON CONFLICT DO NOTHING;

-- ----------------------------------------------------------------
-- Reset sequences
-- ----------------------------------------------------------------
SELECT setval('teams_id_seq',                  (SELECT MAX(id) FROM teams));
SELECT setval('periods_id_seq',                (SELECT MAX(id) FROM periods));
SELECT setval('goals_id_seq',                  (SELECT MAX(id) FROM goals));
SELECT setval('key_results_id_seq',            (SELECT MAX(id) FROM key_results));
SELECT setval('kr_project_stages_id_seq',      (SELECT MAX(id) FROM kr_project_stages));
SELECT setval('goal_comments_id_seq',          (SELECT MAX(id) FROM goal_comments));

COMMIT;
