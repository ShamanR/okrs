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
ALTER TABLE goal_comments          ALTER COLUMN tenant_id SET DEFAULT 1;

-- ----------------------------------------------------------------
-- Clear all tables in FK-safe order
-- ----------------------------------------------------------------
TRUNCATE TABLE
  kr_project_stages,
  kr_boolean_meta,
  key_result_comments,
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
INSERT INTO periods (id, name, start_date, end_date, sort_order, created_at, updated_at) VALUES
  (2, 'Y26',    '2026-01-01', '2026-12-31', 1, NOW(), NOW()),
  (1, 'Y26-Q1', '2026-01-01', '2026-05-31', 2, NOW(), NOW()),
  (3, 'Y26-Q2', '2026-04-01', '2026-06-30', 3, NOW(), NOW()),
  (4, 'Y26-Q3', '2026-07-01', '2026-09-30', 4, NOW(), NOW());

-- ----------------------------------------------------------------
-- Goals
-- ----------------------------------------------------------------
INSERT INTO goals (id, team_id, period_id, title, description, priority, weight, work_type, focus_type, owner_text, sort_order, created_at, updated_at) VALUES
  -- Платформа (team 2), Y26-Q1
  (32, 2, 1, 'Снизить P95 latency до 200ms',        '', 'P1', 35, 'Delivery',  'EFFICIENCY',   '...', 1, NOW(), NOW()),
  (33, 2, 1, 'Покрыть 80% кода тестами',            '', 'P1', 30, 'Delivery',  'QUALITY',      '...', 2, NOW(), NOW()),
  (34, 2, 1, 'Запустить портал документации',       '', 'P2', 20, 'Discovery', 'QUALITY',      '...', 3, NOW(), NOW()),
  -- DWH (team 3), Y26-Q1
  (35, 3, 1, 'Обеспечить 99.9% uptime DWH пайплайнов', '', 'P0', 40, 'Discovery', 'PROFITABILITY', 'Гоша', 1, NOW(), NOW()),
  (36, 3, 1, 'Перевести DWH на ClickHouse 24',      '', 'P0', 25, 'Discovery', 'PROFITABILITY', 'Гоша', 2, NOW(), NOW()),
  -- SRE (team 4), Y26-Q1
  (9,  4, 1, 'Система мониторинга как основной источник информации о инцидентах', '', 'P0', 25, 'Discovery', 'PROFITABILITY', '...', 1, NOW(), NOW()),
  (10, 4, 1, 'Аналитика и исследование инцидентов на регулярной основе',         '', 'P1', 35, 'Delivery',  'STABILITY',     '...', 2, NOW(), NOW()),
  (8,  4, 1, 'Запуск и обеспечение функционирования режима оперативного дежурства 24/7', '', 'P0', 20, 'Discovery', 'PROFITABILITY', '...', 3, NOW(), NOW()),
  (37, 4, 1, 'Внедрить IaC для всей инфраструктуры', '', 'P1', 50, 'Delivery',  'EFFICIENCY',   '...', 4, NOW(), NOW()),
  (38, 4, 1, 'Сократить MTTR до 30 минут',          '', 'P1', 35, 'Delivery',  'RELIABILITY',  '...', 5, NOW(), NOW()),
  -- PaaS\Infra (team 21), Y26-Q1
  (26, 21, 1, 'цель 1', '', 'P0', 25, 'Discovery', 'PROFITABILITY', '', 1, NOW(), NOW()),
  (27, 21, 1, 'цель 2', '', 'P0', 75, 'Discovery', 'PROFITABILITY', '', 2, NOW(), NOW()),
  -- Media (team 23), Y26-Q1
  (29, 23, 1, 'цель2',                                                            '', 'P0', 100, 'Discovery', 'PROFITABILITY', 'Гоша', 1, NOW(), NOW()),
  (30, 23, 1, 'Подключить funsun к сетке для опробирования полного цикла сетки', '', 'P0', 20,  'Discovery', 'PROFITABILITY', 'Гоша', 2, NOW(), NOW());

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
  (2,  1, 'forming'),
  (3,  1, 'forming'),
  (4,  1, 'forming'),
  (5,  1, 'forming'),
  (21, 1, 'forming'),
  (23, 1, 'forming');

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
-- Goal comments
-- ----------------------------------------------------------------
INSERT INTO goal_comments (id, goal_id, text, created_at) VALUES
  (5, 32, 'Обновление по задаче выполнено.', NOW()),
  (7, 37, 'Обновление по задаче выполнено.', NOW());

-- ----------------------------------------------------------------
-- Reset sequences
-- ----------------------------------------------------------------
SELECT setval('teams_id_seq',                  (SELECT MAX(id) FROM teams));
SELECT setval('periods_id_seq',                (SELECT MAX(id) FROM periods));
SELECT setval('goals_id_seq',                  (SELECT MAX(id) FROM goals));
SELECT setval('key_results_id_seq',            (SELECT MAX(id) FROM key_results));
SELECT setval('kr_project_stages_id_seq',      (SELECT MAX(id) FROM kr_project_stages));
SELECT setval('goal_comments_id_seq',          (SELECT MAX(id) FROM goal_comments));
SELECT setval('key_result_comments_id_seq',    1);

COMMIT;
