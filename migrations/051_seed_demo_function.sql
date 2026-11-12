-- +goose Up
-- +goose StatementBegin
-- Демо-наполнение пространства — единственный источник правды о демо-данных.
-- Вызывается приложением при создании пространства (в транзакции создания) и
-- вручную для локального окружения: SELECT seed_demo_data(1).
--
-- Периоды вычисляются от текущей даты: текущий год и его кварталы, предыдущий
-- год; роли целей («текущий квартал» / «история» / «планирование») привязываются
-- к фактическому кварталу вызова. Литеральных дат нет.
--
-- Функция идемпотентна: в начале она удаляет своё же наполнение, скоупленное на
-- tenant_id. Контракт вызова — свежесозданное пространство или локальный демо-
-- тенант; применять к пространству с живыми данными нельзя.
--
-- Изменение демо-контента = новая миграция с CREATE OR REPLACE FUNCTION.
CREATE OR REPLACE FUNCTION seed_demo_data(p_tenant_id bigint) RETURNS void
LANGUAGE plpgsql AS $func$
DECLARE
    v_now        timestamptz := now();
    v_year       int := extract(year from now())::int;
    v_q          int := extract(quarter from now())::int;
    v_user       bigint; -- system:anonymous-local
    v_migr_user  bigint; -- system:migration
    v_req_user   bigint; -- demo:requester
    v_new        bigint;
    v_period     bigint;
    r            record;
    -- периоды
    p_ycur   bigint;
    p_q1     bigint;
    p_q2     bigint;
    p_q3     bigint;
    p_q4     bigint;
    p_yprev  bigint;
    p_pq2    bigint; -- архивный
    p_pq3    bigint;
    p_pq4    bigint;
    p_qcur   bigint; -- текущий квартал
    p_qhist  bigint; -- «история»: предыдущий квартал
    p_qplan  bigint; -- «планирование»: следующий квартал
BEGIN
    IF NOT EXISTS (SELECT 1 FROM tenants WHERE id = p_tenant_id) THEN
        RAISE EXCEPTION 'seed_demo_data: tenant % not found', p_tenant_id;
    END IF;

    SELECT id INTO v_user FROM users WHERE provider_subject_key = 'system:anonymous-local';
    SELECT id INTO v_migr_user FROM users WHERE provider_subject_key = 'system:migration';

    -- ----------------------------------------------------------------
    -- Идемпотентность: удалить прежнее демо-наполнение этого пространства
    -- (FK-безопасный порядок). Чужие пространства не затрагиваются.
    -- ----------------------------------------------------------------
    DELETE FROM kr_project_stages s USING key_results kr
    WHERE s.key_result_id = kr.id AND kr.tenant_id = p_tenant_id;
    DELETE FROM kr_boolean_meta m USING key_results kr
    WHERE m.key_result_id = kr.id AND kr.tenant_id = p_tenant_id;
    DELETE FROM activity_events WHERE tenant_id = p_tenant_id;
    DELETE FROM notifications WHERE tenant_id = p_tenant_id;
    DELETE FROM key_result_notes WHERE tenant_id = p_tenant_id;
    DELETE FROM goal_comments WHERE tenant_id = p_tenant_id;
    DELETE FROM goal_links WHERE tenant_id = p_tenant_id;
    DELETE FROM goal_shares WHERE tenant_id = p_tenant_id;
    DELETE FROM key_results WHERE tenant_id = p_tenant_id;
    DELETE FROM team_period_statuses WHERE tenant_id = p_tenant_id;
    DELETE FROM goals WHERE tenant_id = p_tenant_id;
    DELETE FROM periods WHERE tenant_id = p_tenant_id;
    DELETE FROM teams WHERE tenant_id = p_tenant_id;

    -- Маппинг «исторический id из прежнего seed_demo.sql → реальный id».
    -- aux: для teams и goal_comments — старый parent_id.
    DROP TABLE IF EXISTS seed_map;
    CREATE TEMP TABLE seed_map (
        kind   text   NOT NULL,
        old_id bigint NOT NULL,
        new_id bigint NOT NULL,
        aux    bigint
    ) ON COMMIT DROP;

    -- ----------------------------------------------------------------
    -- Периоды: текущий год + его кварталы, предыдущий год и его Q2..Q4
    -- ----------------------------------------------------------------
    INSERT INTO periods (tenant_id, name, start_date, end_date, archived_at, created_at, updated_at)
    VALUES (p_tenant_id, 'Y' || v_year, make_date(v_year, 1, 1), make_date(v_year, 12, 31), NULL, v_now, v_now)
    RETURNING id INTO p_ycur;

    INSERT INTO periods (tenant_id, name, start_date, end_date, created_at, updated_at)
    VALUES (p_tenant_id, 'Q1 · ' || v_year, make_date(v_year, 1, 1), make_date(v_year, 3, 31), v_now, v_now)
    RETURNING id INTO p_q1;
    INSERT INTO periods (tenant_id, name, start_date, end_date, created_at, updated_at)
    VALUES (p_tenant_id, 'Q2 · ' || v_year, make_date(v_year, 4, 1), make_date(v_year, 6, 30), v_now, v_now)
    RETURNING id INTO p_q2;
    INSERT INTO periods (tenant_id, name, start_date, end_date, created_at, updated_at)
    VALUES (p_tenant_id, 'Q3 · ' || v_year, make_date(v_year, 7, 1), make_date(v_year, 9, 30), v_now, v_now)
    RETURNING id INTO p_q3;
    INSERT INTO periods (tenant_id, name, start_date, end_date, created_at, updated_at)
    VALUES (p_tenant_id, 'Q4 · ' || v_year, make_date(v_year, 10, 1), make_date(v_year, 12, 31), v_now, v_now)
    RETURNING id INTO p_q4;

    INSERT INTO periods (tenant_id, name, start_date, end_date, created_at, updated_at)
    VALUES (p_tenant_id, 'Y' || (v_year - 1), make_date(v_year - 1, 1, 1), make_date(v_year - 1, 12, 31), v_now, v_now)
    RETURNING id INTO p_yprev;
    INSERT INTO periods (tenant_id, name, start_date, end_date, created_at, updated_at)
    VALUES (p_tenant_id, 'Q3 · ' || (v_year - 1), make_date(v_year - 1, 7, 1), make_date(v_year - 1, 9, 30), v_now, v_now)
    RETURNING id INTO p_pq3;
    INSERT INTO periods (tenant_id, name, start_date, end_date, created_at, updated_at)
    VALUES (p_tenant_id, 'Q4 · ' || (v_year - 1), make_date(v_year - 1, 10, 1), make_date(v_year - 1, 12, 31), v_now, v_now)
    RETURNING id INTO p_pq4;
    INSERT INTO periods (tenant_id, name, start_date, end_date, archived_at, created_at, updated_at)
    VALUES (p_tenant_id, 'Q2 · ' || (v_year - 1), make_date(v_year - 1, 4, 1), make_date(v_year - 1, 6, 30), v_now, v_now, v_now)
    RETURNING id INTO p_pq2;

    -- Роли периодов относительно текущего квартала
    p_qcur := CASE v_q WHEN 1 THEN p_q1 WHEN 2 THEN p_q2 WHEN 3 THEN p_q3 ELSE p_q4 END;
    p_qhist := CASE v_q WHEN 1 THEN p_pq4 WHEN 2 THEN p_q1 WHEN 3 THEN p_q2 ELSE p_q3 END;
    IF v_q < 4 THEN
        p_qplan := CASE v_q WHEN 1 THEN p_q2 WHEN 2 THEN p_q3 ELSE p_q4 END;
    ELSE
        -- Создание в Q4: следующий квартал — Q1 следующего года, создаём его
        INSERT INTO periods (tenant_id, name, start_date, end_date, created_at, updated_at)
        VALUES (p_tenant_id, 'Q1 · ' || (v_year + 1), make_date(v_year + 1, 1, 1), make_date(v_year + 1, 3, 31), v_now, v_now)
        RETURNING id INTO p_qplan;
    END IF;

    -- ----------------------------------------------------------------
    -- Команды (иерархия: parent проставляется вторым проходом)
    -- ----------------------------------------------------------------
    FOR r IN SELECT * FROM (VALUES
        (5,   'Разработка',   'cluster', NULL::bigint, '',    'Кластер инженерных команд: платформа, сервисы и продуктовая разработка.'),
        (6,   'Продукт1',     'cluster', NULL,         '...', ''),
        (2,   'Платформа',    'unit',    5,            '',    'Ядро платформы: backend, инфраструктура и данные продуктов.'),
        (18,  'Promo',        'unit',    5,            '...', ''),
        (22,  'Сервисы',      'unit',    5,            '',    'Продуктовые сервисы платформы: медиа, финансы и интеграции.'),
        (3,   'DWH',          'team',    2,            '',    ''),
        (4,   'SRE',          'team',    2,            '...', ''),
        (19,  'CoreDEV',      'team',    2,            '',    ''),
        (20,  'CoreQA',       'team',    2,            '',    ''),
        (21,  'PaaS \ Infra', 'team',    2,            '...', ''),
        (23,  'Media',        'team',    22,           '',    ''),
        (24,  'Finance',      'team',    22,           '',    ''),
        (25,  'Internal',     'team',    22,           '',    ''),
        (26,  'External',     'team',    22,           '',    ''),
        (27,  'Old School',   'team',    22,           '',    ''),
        (7,   'Группа "ЛК перформанс"', 'unit', 6,     '',    ''),
        (8,   'Команда 2',    'team',    7,            '',    ''),
        (9,   'Команда 1',    'team',    7,            '',    ''),
        (10,  'Команда 3',    'team',    7,            '',    ''),
        -- Каталог «Департамент»
        (100, 'Департамент',  'department', NULL,      '',    'Департамент: каталог целей.'),
        (101, 'Разработка',   'cluster', 100,          '',    'Кластер инженерных команд департамента.'),
        (102, 'Платформа',    'unit',    101,          '',    'Ядро платформы: backend, инфраструктура и данные.'),
        (103, 'Backend & Quality', 'group', 102,       'Дмитрий Петров', 'Стандарты кода и процессов для Backend и QA команд.'),
        (104, 'CoreDEV',      'team',    103,          'Сергей Лебедев', ''),
        (105, 'API Platform', 'squad',   104,          'Сергей Лебедев', ''),
        (106, 'CoreQA',       'team',    103,          'Елена Смирнова', ''),
        (107, 'Infrastructure', 'group', 102,          '',    ''),
        (108, 'PaaS \ Infra', 'team',    107,          'Андрей Николаев', ''),
        (109, 'SRE',          'team',    107,          'Кирилл Воронов', ''),
        (110, 'DWH',          'team',    102,          'Михаил Козлов', ''),
        (111, 'Продвижение',  'unit',    101,          '',    ''),
        (112, 'Перформанс',   'team',    111,          'Роман Белов', '')
    ) AS v(old_id, name, ttype, parent_old, lead, descr) LOOP
        INSERT INTO teams (tenant_id, name, team_type, parent_id, lead, description, created_at, updated_at)
        VALUES (p_tenant_id, r.name, r.ttype, NULL, r.lead, r.descr, v_now, v_now)
        RETURNING id INTO v_new;
        INSERT INTO seed_map (kind, old_id, new_id, aux) VALUES ('team', r.old_id, v_new, r.parent_old);
    END LOOP;

    UPDATE teams t
    SET parent_id = pm.new_id
    FROM seed_map cm
    JOIN seed_map pm ON pm.kind = 'team' AND pm.old_id = cm.aux
    WHERE cm.kind = 'team' AND cm.new_id = t.id AND t.tenant_id = p_tenant_id;

    -- ----------------------------------------------------------------
    -- Цели. Роль периода: year = текущий год, cur = текущий квартал,
    -- hist = предыдущий квартал (история), plan = следующий квартал.
    -- ----------------------------------------------------------------
    FOR r IN SELECT * FROM (VALUES
        -- Базовый набор
        (32,  2,   'cur',  'Снизить P95 latency до 200ms', '', 'P1', 35, 'Delivery',  'TECH',    '...',  1),
        (33,  2,   'cur',  'Покрыть 80% кода тестами', '', 'P1', 30, 'Delivery',  'TECH',    '...',  2),
        (34,  2,   'cur',  'Запустить портал документации', '', 'P2', 20, 'Discovery', 'PROCESS', '...',  3),
        (35,  3,   'hist', 'Обеспечить 99.9% uptime DWH пайплайнов', '', 'P0', 40, 'Discovery', 'TECH', 'Гоша', 1),
        (36,  3,   'hist', 'Перевести DWH на ClickHouse 24', '', 'P0', 25, 'Discovery', 'TECH', 'Гоша', 2),
        (9,   4,   'cur',  'Система мониторинга как основной источник информации о инцидентах', '', 'P0', 25, 'Discovery', 'PROCESS', '...', 1),
        (10,  4,   'cur',  'Аналитика и исследование инцидентов на регулярной основе', '', 'P1', 35, 'Delivery', 'PROCESS', '...', 2),
        (8,   4,   'cur',  'Запуск и обеспечение функционирования режима оперативного дежурства 24/7', '', 'P0', 20, 'Discovery', 'PROCESS', '...', 3),
        (37,  4,   'cur',  'Внедрить IaC для всей инфраструктуры', '', 'P1', 50, 'Delivery', 'PROCESS', '...', 4),
        (38,  4,   'cur',  'Сократить MTTR до 30 минут', '', 'P1', 35, 'Delivery', 'TECH', '...', 5),
        (26,  21,  'cur',  'цель 1', '', 'P0', 25, 'Discovery', 'PRODUCT', '', 1),
        (27,  21,  'cur',  'цель 2', '', 'P0', 75, 'Discovery', 'PRODUCT', '', 2),
        (29,  23,  'plan', 'цель2', '', 'P0', 100, 'Discovery', 'PRODUCT', 'Гоша', 1),
        (30,  23,  'plan', 'Подключить funsun к сетке для опробирования полного цикла сетки', '', 'P0', 20, 'Discovery', 'PRODUCT', 'Гоша', 2),
        -- Каталог «Департамент» (годовые)
        (100, 102, 'year', 'Снизить P95 latency до 200ms', 'Оптимизировать критические пути запросов', 'P1', 35, 'Delivery', 'TECH', 'Алексей Иванов', 1),
        (101, 102, 'year', 'Покрыть 80% кода тестами', 'Повысить надёжность через автоматизацию', 'P1', 30, 'Delivery', 'TECH', 'Елена Смирнова', 2),
        (102, 102, 'year', 'Запустить портал документации', 'Единое место для архитектурных решений', 'P2', 20, 'Discovery', 'PROCESS', 'Михаил Козлов', 3),
        (103, 102, 'year', 'Снизить Time-to-Deploy до 15 мин', 'Ускорить CI/CD пайплайн', 'P2', 15, 'Delivery', 'PROCESS', 'Андрей Николаев', 4),
        (104, 103, 'year', 'Единая культура инженерного качества', 'Стандарты кода и процессов для Backend и QA команд', 'P1', 100, 'Discovery', 'PROCESS', 'Дмитрий Петров', 1),
        (105, 104, 'year', 'Migrate to Go 1.22', 'Обновить все сервисы, использовать новые возможности языка', 'P1', 50, 'Delivery', 'TECH', 'Сергей Лебедев', 1),
        (106, 104, 'year', 'Рефакторинг auth модуля', 'Выделить в отдельный сервис', 'P2', 50, 'Delivery', 'TECH', 'Ольга Фёдорова', 2),
        (107, 105, 'year', 'Запустить новую версию API Gateway v2', 'Рефакторинг маршрутизации, поддержка gRPC', 'P1', 70, 'Delivery', 'PRODUCT', 'Сергей Лебедев', 1),
        (108, 105, 'year', 'Документация API для партнёров', '', 'P2', 30, 'Discovery', 'PRODUCT', 'Михаил Козлов', 2),
        (109, 106, 'year', 'Автоматизировать регрессионное тестирование', 'Покрыть 200 ключевых user flows', 'P1', 100, 'Delivery', 'PROCESS', 'Елена Смирнова', 1),
        (110, 108, 'year', 'Перевести 80% сервисов на k8s', '', 'P1', 70, 'Delivery', 'TECH', 'Андрей Николаев', 1),
        (111, 108, 'year', 'SLA платформ 99.5%', '', 'P2', 30, 'Delivery', 'TECH', 'Андрей Николаев', 2),
        (112, 109, 'year', '99.9% uptime всех production сервисов', 'Обеспечить надёжность инфраструктуры', 'P0', 60, 'Delivery', 'TECH', 'Кирилл Воронов', 1),
        (113, 109, 'year', 'MTTR < 10 мин для P0 инцидентов', 'Ускорить реакцию на критические сбои', 'P1', 40, 'Delivery', 'TECH', 'Кирилл Воронов', 2),
        (114, 110, 'year', 'DWH для метрик', 'Централизованное хранилище', 'P1', 100, 'Discovery', 'PRODUCT', 'Михаил Козлов', 1),
        (115, 112, 'year', 'Снизить CPA на 15%', 'ML-биддинг', 'P0', 60, 'Delivery', 'PRODUCT', 'Роман Белов', 1),
        (116, 112, 'year', 'ML-биддинг для топ-10 клиентов', '', 'P1', 30, 'Discovery', 'PRODUCT', 'Роман Белов', 2)
    ) AS v(old_id, team_old, prole, title, descr, priority, weight, wtype, direction, owner, sort) LOOP
        SELECT tm.new_id INTO v_new FROM seed_map tm WHERE tm.kind = 'team' AND tm.old_id = r.team_old;
        v_period := CASE r.prole
            WHEN 'year' THEN p_ycur WHEN 'cur' THEN p_qcur
            WHEN 'hist' THEN p_qhist ELSE p_qplan END;
        INSERT INTO goals (tenant_id, team_id, period_id, title, description, priority, weight,
                           work_type, direction, owner_text, sort_order, created_at, updated_at)
        VALUES (p_tenant_id, v_new, v_period, r.title, r.descr, r.priority, r.weight,
                r.wtype, r.direction, r.owner, r.sort, v_now, v_now)
        RETURNING id INTO v_new;
        INSERT INTO seed_map (kind, old_id, new_id) VALUES ('goal', r.old_id, v_new);
    END LOOP;

    -- ----------------------------------------------------------------
    -- Статусы команд в периодах
    -- ----------------------------------------------------------------
    INSERT INTO team_period_statuses (tenant_id, team_id, period_id, status)
    SELECT p_tenant_id, tm.new_id,
           CASE v.prole
               WHEN 'year' THEN p_ycur WHEN 'cur' THEN p_qcur
               WHEN 'hist' THEN p_qhist ELSE p_qplan END,
           v.status
    FROM (VALUES
        (2,   'cur',  'in_progress'),
        (3,   'hist', 'closed'),
        (4,   'cur',  'in_progress'),
        (5,   'cur',  'forming'),
        (21,  'cur',  'ready'),
        (23,  'plan', 'forming'),
        (102, 'year', 'in_progress'),
        (103, 'year', 'in_progress'),
        (104, 'year', 'in_progress'),
        (105, 'year', 'forming'),
        (106, 'year', 'in_progress'),
        (108, 'year', 'in_progress'),
        (109, 'year', 'in_progress'),
        (110, 'year', 'forming'),
        (112, 'year', 'in_progress')
    ) AS v(team_old, prole, status)
    JOIN seed_map tm ON tm.kind = 'team' AND tm.old_id = v.team_old;

    -- ----------------------------------------------------------------
    -- Goal shares
    -- ----------------------------------------------------------------
    INSERT INTO goal_shares (tenant_id, goal_id, team_id, weight, sort_order, created_at, updated_at)
    SELECT p_tenant_id, gm.new_id, tm.new_id, v.weight, v.sort, v_now, v_now
    FROM (VALUES
        (30, 6, 0,  3),  -- Media/funsun → кластер Продукт1
        (32, 5, 15, 1)   -- Платформа latency → кластер Разработка
    ) AS v(goal_old, team_old, weight, sort)
    JOIN seed_map gm ON gm.kind = 'goal' AND gm.old_id = v.goal_old
    JOIN seed_map tm ON tm.kind = 'team' AND tm.old_id = v.team_old;

    -- ----------------------------------------------------------------
    -- Key Results
    -- ----------------------------------------------------------------
    FOR r IN SELECT * FROM (VALUES
        -- Goal 8 (SRE: дежурства 24/7)
        (7,   8,   'Внедрен регламент дежурств во все юниты', 'Разработан, согласован со всеми заинтересованными сторонами и утвержден регламент дежурств 24/7.', 34, 'BOOLEAN', 1),
        (8,   8,   'Внедрены инструкции для дежурных инженеров', 'Составлены и внедрены подробные инструкции для дежурных инженеров (runbooks) по действиям при различных типах инцидентов.', 33, 'BOOLEAN', 2),
        (9,   8,   'Каллендарь дежурств внедрен в 5 команд', 'Составлен и опубликован календарь дежурств команды SRE и до 5 тестовых (early-adopter) команд.', 33, 'NUMERICAL', 3),
        -- Goal 9 (SRE: система мониторинга)
        (10,  9,   'Инцидент менеджмент для P0\P1 автоматизирован', 'Реализован автоматический сценарий, который при поступлении алерта P0/P1 автоматизирует обработку инцидента.', 25, 'PROJECT', 1),
        (11,  9,   'Система мониторинга покрывает 75% критических алертов', 'Система мониторинга покрывает 75% критических алертов и включает в себя ранбуки для работы с ними.', 35, 'NUMERICAL', 2),
        (14,  9,   'Автоматизация протестирована на тестовых инцидентах', 'Автоматизированный процесс протестирован на учебных инцидентах и подтверждена его стабильная работа.', 35, 'BOOLEAN', 3),
        -- Goal 10 (SRE: аналитика инцидентов)
        (15,  10,  'Создана регулярная встреча по аналитике инцидентов за прошедший период', 'Проведена и регулярно повторяется групповая встреча со всеми тимлидами и юнит-лидами.', 25, 'BOOLEAN', 1),
        (16,  10,  'Создан план по работе с историческими данными по инцидентам', 'Создан роадмап по работе с историческими данными по инцидентам и их предотвращению.', 25, 'BOOLEAN', 2),
        (17,  10,  'Внедрен реестр инцидентов', 'Реализована точка правды по информации о инцидентах (потери, длительность, влияние и т.п.).', 25, 'BOOLEAN', 3),
        (18,  10,  'Реализован процесс подсчета потерь по инцидентам', '', 25, 'BOOLEAN', 4),
        -- Goal 26, 27 (PaaS), 29, 30 (Media)
        (43,  26,  'цель2', '', 100, 'BOOLEAN', 1),
        (42,  27,  'bool', '', 100, 'NUMERICAL', 1),
        (45,  29,  'перцент', '', 51, 'NUMERICAL', 1),
        (46,  29,  'перцент2', '', 50, 'NUMERICAL', 2),
        (47,  30,  '100% трафика на сайте крутится через сетку', 'Как проверим: ссылка на метрику.', 50, 'NUMERICAL', 1),
        (48,  30,  'Есть техническая возможность видеть баннер', '', 50, 'PROJECT', 2),
        -- Goal 32 (Платформа: P95 latency)
        (50,  32,  'P95 latency API gateway', 'Текущее: 450ms → Цель: 200ms', 40, 'NUMERICAL', 1),
        (51,  32,  'P95 latency auth service', 'Текущее: 380ms → Цель: 200ms', 30, 'NUMERICAL', 2),
        (52,  32,  'Миграция на HTTP/2', 'Переключить все внутренние вызовы', 20, 'BOOLEAN', 3),
        -- Goal 33 (покрытие тестами)
        (53,  33,  'Покрытие unit-тестами', '% покрытия core пакетов', 50, 'NUMERICAL', 1),
        (54,  33,  'Покрытие integration-тестами', '% покрытия API endpoints', 50, 'NUMERICAL', 2),
        -- Goal 34 (портал документации)
        (55,  34,  'Развернуть Confluence/Notion', 'Setup и настройка инструмента', 30, 'BOOLEAN', 1),
        (56,  34,  'Перенести ADR документы', 'Architecture Decision Records', 40, 'PROJECT', 2),
        -- Goal 37 (IaC)
        (57,  37,  'Перевести prod окружение на Terraform', 'Все сервисы prod', 60, 'NUMERICAL', 1),
        (58,  37,  'Написать модули для баз данных', 'RDS, Redis модули', 25, 'PROJECT', 2),
        -- Goal 38 (MTTR)
        (59,  38,  'Настроить runbooks для топ-5 инцидентов', 'По каждому типу инцидента', 50, 'PROJECT', 1),
        (60,  38,  'Среднее время реакции on-call', 'Текущее: 45 мин → Цель: 30 мин', 50, 'NUMERICAL', 2),
        -- Каталог «Департамент»
        (100, 100, 'P95 latency API gateway', 'Замеряем по Prometheus, окно 5 минут. Считаем latency на edge-роутах /api/v1/*.', 40, 'NUMERICAL', 1),
        (101, 100, 'P95 latency auth service', 'Внутренние gRPC-вызовы auth.Verify и auth.Refresh, без учёта таймаутов клиента.', 40, 'NUMERICAL', 2),
        (102, 100, 'Миграция на HTTP/2', 'Все внешние эндпоинты переведены на HTTP/2 с поддержкой fallback на HTTP/1.1.', 20, 'PROJECT', 3),
        (103, 101, 'Unit test coverage CoreDEV', 'Покрытие по go test -cover для пакетов core/*, без учёта моков и сгенерированного кода. Цель в 80% относится только к бизнес-логике.', 50, 'NUMERICAL', 1),
        (104, 101, 'Integration test suite', '', 20, 'BOOLEAN', 2),
        (105, 101, 'QA automation pipeline', '', 20, 'PROJECT', 3),
        (106, 102, 'Architecture decisions documented', 'ADR в репозитории docs/adr/, минимум 15 ключевых решений за квартал.', 60, 'NUMERICAL', 1),
        (107, 102, 'API docs migrated', '', 40, 'NUMERICAL', 2),
        (108, 103, 'Pipeline время P50 < 15 мин', '', 70, 'NUMERICAL', 1),
        (109, 103, 'Параллелизация тестов', '', 30, 'BOOLEAN', 2),
        (110, 104, 'Engineering handbook опубликован', '', 50, 'BOOLEAN', 1),
        (111, 104, 'Code review время < 2ч', '', 50, 'NUMERICAL', 2),
        (112, 105, 'Все сервисы обновлены', 'go.mod указывает Go 1.22, CI зелёный, прод-сборка прошла smoke-тесты.', 60, 'NUMERICAL', 1),
        (113, 105, 'Тесты проходят на Go 1.22', '', 40, 'NUMERICAL', 2),
        (114, 106, 'Auth service выделен', '', 70, 'PROJECT', 1),
        (115, 106, 'Циклические зависимости устранены', '', 30, 'BOOLEAN', 2),
        (116, 107, 'gRPC транскодинг работает', '', 40, 'BOOLEAN', 1),
        (117, 107, 'Выдерживаемая нагрузка gateway', 'Нагрузочный тест k6, рост RPS. Прогресс по промежуточным значениям.', 60, 'NUMERICAL', 2),
        (118, 108, 'API Reference опубликован', '', 100, 'BOOLEAN', 1),
        (119, 109, 'User flows покрыты авто-тестами', '', 70, 'NUMERICAL', 1),
        (120, 109, 'CI интеграция настроена', '', 30, 'BOOLEAN', 2),
        (121, 110, 'Сервисов на k8s', '', 100, 'NUMERICAL', 1),
        (122, 111, 'Uptime platform', '', 100, 'NUMERICAL', 1),
        (123, 112, 'Uptime API gateway', '', 50, 'NUMERICAL', 1),
        (124, 112, 'Uptime core services', '', 50, 'NUMERICAL', 2),
        (125, 113, 'Runbook покрытие 90%', '', 40, 'NUMERICAL', 1),
        (126, 113, 'On-call rotation настроен', '', 30, 'BOOLEAN', 2),
        (127, 113, 'P50 MTTR < 10 мин', '', 30, 'NUMERICAL', 3),
        (128, 114, 'Схема данных согласована', '', 40, 'BOOLEAN', 1),
        (129, 114, 'Pipeline ежедневных данных', '', 60, 'PROJECT', 2),
        (130, 115, 'CPA (целевой -15%)', '', 100, 'NUMERICAL', 1),
        (131, 116, 'ML-модель в проде', '', 60, 'PROJECT', 1),
        (132, 116, 'Клиентов подключено', '', 40, 'NUMERICAL', 2)
    ) AS v(old_id, goal_old, title, descr, weight, kind, sort) LOOP
        SELECT gm.new_id INTO v_new FROM seed_map gm WHERE gm.kind = 'goal' AND gm.old_id = r.goal_old;
        INSERT INTO key_results (tenant_id, goal_id, title, description, weight, kind, sort_order, created_at, updated_at)
        VALUES (p_tenant_id, v_new, r.title, r.descr, r.weight, r.kind, r.sort, v_now, v_now)
        RETURNING id INTO v_new;
        INSERT INTO seed_map (kind, old_id, new_id) VALUES ('kr', r.old_id, v_new);
    END LOOP;

    -- ----------------------------------------------------------------
    -- KR meta — BOOLEAN
    -- ----------------------------------------------------------------
    INSERT INTO kr_boolean_meta (key_result_id, is_done)
    SELECT km.new_id, v.done
    FROM (VALUES
        (7, false), (8, true), (14, false), (15, false), (16, false),
        (17, true), (18, false), (43, true), (52, false), (55, false),
        (104, false), (109, false), (110, false), (115, false), (116, false),
        (118, false), (120, true), (126, true), (128, true)
    ) AS v(old_id, done)
    JOIN seed_map km ON km.kind = 'kr' AND km.old_id = v.old_id;

    -- ----------------------------------------------------------------
    -- KR meta — NUMERICAL, определение (start/target/unit)
    -- ----------------------------------------------------------------
    UPDATE key_results kr
    SET start_value = v.start, target_value = v.target, unit = v.unit
    FROM (VALUES
        (9,   0,   5,   'шт'),  (11,  10,  75,  '%'),
        (45,  0,   100, '%'),   (46,  0,   100, '%'),  (47,  0,   100, '%'),
        (53,  0,   100, '%'),   (54,  0,   100, '%'),  (57,  0,   100, '%'),
        (42,  0,   100, '%'),   (50,  450, 200, 'мс'), (51,  380, 200, 'мс'),
        (60,  45,  30,  'мин'),
        (100, 320.0, 200.0, 'мс'), (101, 450.0, 200.0, 'мс'),
        (103, 0.0, 80.0, '%'),   (106, 0.0, 100.0, '%'), (107, 0.0, 100.0, '%'),
        (108, 48.0, 15.0, 'мин'), (111, 8.0, 2.0, 'час'),
        (112, 0.0, 100.0, '%'),  (113, 0.0, 100.0, '%'), (117, 100.0, 180.0, 'RPS'),
        (119, 0.0, 200.0, 'шт'), (121, 0.0, 80.0, '%'),
        (122, 97.0, 99.5, '%'),  (123, 98.0, 99.9, '%'), (124, 98.0, 99.9, '%'),
        (125, 0.0, 90.0, '%'),   (127, 35.0, 10.0, 'мин'),
        (130, 100.0, 85.0, '%'), (132, 0.0, 10.0, '%')
    ) AS v(old_id, start, target, unit)
    JOIN seed_map km ON km.kind = 'kr' AND km.old_id = v.old_id
    WHERE kr.id = km.new_id;

    -- Прогресс — отдельной операцией, как в приложении: current_value и
    -- progress_updated_at ставит только чек-ин. KR вне списка остаются
    -- необновлёнными — чтение отдаёт стартовое значение.
    UPDATE key_results kr
    SET current_value = v.current, progress_updated_at = v_now - interval '3 days'
    FROM (VALUES
        (9, 2), (11, 50), (45, 51), (46, 10), (42, 50),
        (100, 278.0), (101, 408.0), (103, 14.0), (106, 60.0), (107, 10.0),
        (111, 5.0), (112, 65.0), (113, 80.0), (117, 170.0), (119, 76.0),
        (121, 44.0), (122, 98.6), (123, 99.2), (124, 99.4), (125, 41.0),
        (127, 20.0), (130, 97.0), (132, 2.0)
    ) AS v(old_id, current)
    JOIN seed_map km ON km.kind = 'kr' AND km.old_id = v.old_id
    WHERE kr.id = km.new_id;

    -- KR meta — NUMERICAL checkpoints
    UPDATE key_results kr
    SET checkpoints = '[{"value":100,"progress_percent":0},{"value":150,"progress_percent":50},{"value":180,"progress_percent":100}]'::jsonb
    FROM seed_map km
    WHERE km.kind = 'kr' AND km.old_id = 117 AND kr.id = km.new_id;

    UPDATE key_results kr
    SET checkpoints = '[{"value":50,"progress_percent":25},{"value":100,"progress_percent":50},{"value":150,"progress_percent":75},{"value":200,"progress_percent":100}]'::jsonb
    FROM seed_map km
    WHERE km.kind = 'kr' AND km.old_id = 119 AND kr.id = km.new_id;

    -- KR meta — критерии обнуления
    UPDATE key_results kr
    SET zeroing_criteria = 'Если латентность достигнута только за счёт отключения проверок безопасности на edge — результат не засчитывается.'
    FROM seed_map km
    WHERE km.kind = 'kr' AND km.old_id = 100 AND kr.id = km.new_id;

    UPDATE key_results kr
    SET zeroing_criteria = 'Если хотя бы один внешний эндпоинт откатили обратно на HTTP/1.1 из-за инцидента в проде — результат не засчитывается.'
    FROM seed_map km
    WHERE km.kind = 'kr' AND km.old_id = 102 AND kr.id = km.new_id;

    -- ----------------------------------------------------------------
    -- KR meta — PROJECT stages
    -- ----------------------------------------------------------------
    INSERT INTO kr_project_stages (key_result_id, title, weight, is_done, sort_order)
    SELECT km.new_id, v.title, v.weight, v.done, v.sort
    FROM (VALUES
        (10,  'Создает тикет инцидента в заданной системе (Jira, Youtrack и т.п.) с предзаполненными полями', 33, true,  1),
        (10,  'Создает отдельный чат для коммуникации по инциденту', 33, true,  2),
        (10,  'Автоматически приглашает в созданный чат рабочую группу по данному типу инцидентов', 34, true,  3),
        (48,  'подключен SDK', 20, false, 1),
        (48,  'настроен кабинет под все виды пользователей', 30, false, 2),
        (48,  'подписан договор с внешним сайтом', 50, false, 3),
        (102, 'Конфигурация nginx обновлена', 40, true,  1),
        (102, 'Тестирование с HTTP/2', 35, false, 2),
        (102, 'Мониторинг метрик', 25, false, 3),
        (105, 'Тест-раннер настроен', 30, true,  1),
        (105, 'CI интеграция', 40, false, 2),
        (105, 'Репортинг', 30, false, 3),
        (114, 'Интерфейс auth service', 25, true,  1),
        (114, 'Деплой в dev', 35, false, 2),
        (114, 'Production миграция', 40, false, 3),
        (129, 'Источники подключены', 40, true,  1),
        (129, 'Трансформации настроены', 35, false, 2),
        (129, 'Дашборды готовы', 25, false, 3),
        (131, 'Модель обучена', 35, true,  1),
        (131, 'A/B тест', 35, true,  2),
        (131, 'Production', 30, false, 3)
    ) AS v(kr_old, title, weight, done, sort)
    JOIN seed_map km ON km.kind = 'kr' AND km.old_id = v.kr_old;

    -- ----------------------------------------------------------------
    -- Health status. Завершённые KR (прогресс 100%) — 'done' по правилу
    -- 100%→done; остальные из списка — для разнообразия, прочие держат
    -- дефолт 'not_started'.
    -- ----------------------------------------------------------------
    UPDATE key_results kr SET health_status = 'done'
    FROM seed_map km
    WHERE km.kind = 'kr' AND km.old_id IN (8, 17, 43, 10) AND kr.id = km.new_id;

    UPDATE key_results kr SET health_status = 'on_track'
    FROM seed_map km
    WHERE km.kind = 'kr' AND km.old_id IN (9, 11, 45, 42, 53, 54) AND kr.id = km.new_id;

    UPDATE key_results kr SET health_status = 'at_risk'
    FROM seed_map km
    WHERE km.kind = 'kr' AND km.old_id IN (50, 51, 60) AND kr.id = km.new_id;

    -- ----------------------------------------------------------------
    -- Комментарии к целям (автор — system:anonymous-local, если есть)
    -- ----------------------------------------------------------------
    IF v_user IS NOT NULL THEN
        FOR r IN SELECT * FROM (VALUES
            (5,   32,  NULL::bigint, 'Обновление по задаче выполнено.'),
            (7,   37,  NULL,         'Обновление по задаче выполнено.'),
            (100, 100, NULL,         'Начали профилирование. Нашли узкое место в middleware auth.'),
            (101, 100, NULL,         'Нужно учесть зависимость от PaaS/Infra по HTTP/2-конфигурации nginx.'),
            (102, 102, NULL,         'ADR по 12 ключевым решениям готовы. Миграция API docs с Confluence в процессе.'),
            (103, 105, NULL,         'Осталось 3 сервиса. Основные проблемы с устаревшими зависимостями решены.'),
            (104, 112, NULL,         'Апрельский инцидент устранён. Добавили circuit breaker на 3 критичных точки.'),
            -- Ответы (parent → таска): ветка «таска → ответы», ответ не резолвится
            (150, 100, 100,          'Согласовали с Infra: HTTP/2 включаем на следующей неделе.'),
            (151, 100, 101,          'Уточнил у PaaS — конфиг nginx поправят в этом спринте.')
        ) AS v(old_id, goal_old, parent_old, txt) LOOP
            SELECT gm.new_id INTO v_new FROM seed_map gm WHERE gm.kind = 'goal' AND gm.old_id = r.goal_old;
            INSERT INTO goal_comments (tenant_id, goal_id, parent_id, text, author_user_id, created_at)
            VALUES (p_tenant_id, v_new, NULL, r.txt, v_user, v_now)
            RETURNING id INTO v_new;
            INSERT INTO seed_map (kind, old_id, new_id, aux) VALUES ('comment', r.old_id, v_new, r.parent_old);
        END LOOP;

        UPDATE goal_comments c
        SET parent_id = pm.new_id
        FROM seed_map cm
        JOIN seed_map pm ON pm.kind = 'comment' AND pm.old_id = cm.aux
        WHERE cm.kind = 'comment' AND cm.new_id = c.id AND c.tenant_id = p_tenant_id;

        -- Одно замечание помечено решённым — демонстрирует resolve-флоу
        UPDATE goal_comments c
        SET resolved_at = v_now, resolved_by_user_id = v_user
        FROM seed_map cm
        WHERE cm.kind = 'comment' AND cm.old_id = 100 AND c.id = cm.new_id;

        -- Заметки KR (1:1)
        INSERT INTO key_result_notes (tenant_id, key_result_id, text, author_user_id, updated_at)
        SELECT p_tenant_id, km.new_id, v.txt, v_user, v_now
        FROM (VALUES
            (101, 'Нашли узкое место в JWT verify — пул соединений был слишком мал.'),
            (104, 'Ожидаем инфраструктуру от PaaS — блокер на неделю.'),
            (112, 'Осталось 3 сервиса. Проблемы с устаревшими зависимостями решены.')
        ) AS v(kr_old, txt)
        JOIN seed_map km ON km.kind = 'kr' AND km.old_id = v.kr_old;
    END IF;

    -- ----------------------------------------------------------------
    -- Goal links (декомпозиция «дочерняя → родительская»). Родители —
    -- годовые цели, потомки — квартальные. Связь не делает цель общей
    -- и не влияет на прогресс: только дерево декомпозиции для /goal-tree.
    -- ----------------------------------------------------------------
    INSERT INTO goal_links (tenant_id, child_goal_id, parent_goal_id)
    SELECT p_tenant_id, cg.new_id, pg.new_id
    FROM (VALUES
        (8,  112), (9,  112),   -- 112 «99.9% uptime» (SRE)
        (10, 113), (38, 113),   -- 113 «MTTR < 10 мин» (SRE)
        (37, 110),              -- годовая цель одной команды в квартальную соседней
        (35, 114), (36, 114)    -- 114 «DWH для метрик»
    ) AS v(child_old, parent_old)
    JOIN seed_map cg ON cg.kind = 'goal' AND cg.old_id = v.child_old
    JOIN seed_map pg ON pg.kind = 'goal' AND pg.old_id = v.parent_old;

    -- ----------------------------------------------------------------
    -- Activity log (демо-лента). actor — system:anonymous-local.
    -- Времена разнесены (сегодня / вчера / на этой неделе / старее).
    -- ----------------------------------------------------------------
    IF v_user IS NOT NULL THEN
        -- Сегодня
        INSERT INTO activity_events (tenant_id, actor_user_id, category, action, team_id, period_id,
                                     goal_id, kr_id, comment_id, entity_title, payload_json, created_at)
        VALUES
        (p_tenant_id, v_user, 'progress', 'kr_progress',
         (SELECT new_id FROM seed_map WHERE kind='team' AND old_id=4), p_qcur,
         (SELECT new_id FROM seed_map WHERE kind='goal' AND old_id=8),
         (SELECT new_id FROM seed_map WHERE kind='kr' AND old_id=9), NULL,
         'Календарь дежурств внедрен в 5 команд',
         '{"before":{"progress":40},"after":{"progress":65},"kind":"NUMERICAL"}',
         v_now - interval '2 hours'),
        (p_tenant_id, v_user, 'discussion', 'comment_added',
         (SELECT new_id FROM seed_map WHERE kind='team' AND old_id=2), p_qcur,
         (SELECT new_id FROM seed_map WHERE kind='goal' AND old_id=32), NULL,
         (SELECT new_id FROM seed_map WHERE kind='comment' AND old_id=5),
         'Снизить P95 latency до 200ms',
         '{"text":"Обновление по задаче выполнено."}',
         v_now - interval '5 hours'),
        (p_tenant_id, v_user, 'composition', 'goal_created',
         (SELECT new_id FROM seed_map WHERE kind='team' AND old_id=23), p_qplan,
         (SELECT new_id FROM seed_map WHERE kind='goal' AND old_id=29), NULL, NULL,
         'цель2', '{}', v_now - interval '8 hours'),
        -- Вчера
        (p_tenant_id, v_user, 'status', 'status_changed',
         (SELECT new_id FROM seed_map WHERE kind='team' AND old_id=21), p_qcur,
         NULL, NULL, NULL, 'PaaS / Infra',
         '{"before":{"status":"forming"},"after":{"status":"in_progress"}}',
         v_now - interval '1 day 3 hours'),
        (p_tenant_id, v_user, 'composition', 'kr_created',
         (SELECT new_id FROM seed_map WHERE kind='team' AND old_id=4), p_qcur,
         (SELECT new_id FROM seed_map WHERE kind='goal' AND old_id=9),
         (SELECT new_id FROM seed_map WHERE kind='kr' AND old_id=10), NULL,
         'Инцидент менеджмент для P0\P1 автоматизирован', '{}',
         v_now - interval '1 day 6 hours'),
        -- Ранее на этой неделе
        (p_tenant_id, v_user, 'discussion', 'comment_resolved',
         (SELECT new_id FROM seed_map WHERE kind='team' AND old_id=4), p_qcur,
         (SELECT new_id FROM seed_map WHERE kind='goal' AND old_id=37), NULL,
         (SELECT new_id FROM seed_map WHERE kind='comment' AND old_id=7),
         'Внедрить IaC для всей инфраструктуры',
         '{"before":{"resolved":false},"after":{"resolved":true}}',
         v_now - interval '3 days'),
        (p_tenant_id, v_user, 'composition', 'goal_shared',
         (SELECT new_id FROM seed_map WHERE kind='team' AND old_id=2), p_qcur,
         (SELECT new_id FROM seed_map WHERE kind='goal' AND old_id=33), NULL, NULL,
         'Покрыть 80% кода тестами',
         jsonb_build_object('shared_with_team_ids', jsonb_build_array(
             (SELECT new_id FROM seed_map WHERE kind='team' AND old_id=4))),
         v_now - interval '4 days'),
        -- Старее
        (p_tenant_id, v_user, 'composition', 'goal_fields_changed',
         (SELECT new_id FROM seed_map WHERE kind='team' AND old_id=4), p_qcur,
         (SELECT new_id FROM seed_map WHERE kind='goal' AND old_id=38), NULL, NULL,
         'Сократить MTTR до 30 минут',
         '{"changed":{"title":{"before":"Сократить MTTR","after":"Сократить MTTR до 30 минут"}}}',
         v_now - interval '6 days'),
        (p_tenant_id, v_user, 'progress', 'kr_progress',
         (SELECT new_id FROM seed_map WHERE kind='team' AND old_id=3), p_qhist,
         (SELECT new_id FROM seed_map WHERE kind='goal' AND old_id=35), NULL, NULL,
         'Обеспечить 99.9% uptime DWH пайплайнов',
         '{"before":{"progress":80},"after":{"progress":100},"kind":"NUMERICAL"}',
         v_now - interval '12 days');
    END IF;

    -- ----------------------------------------------------------------
    -- Notification preferences + демо-уведомления (миграция 044).
    -- ON CONFLICT DO NOTHING: повторный запуск не плодит дубликаты и не
    -- теряет отметки "прочитано".
    -- ----------------------------------------------------------------
    IF v_user IS NOT NULL THEN
        -- Демо-настройки: один пользователь смотрит всё поддерево. Пустая
        -- channel_overrides — «пользователь про каналы ничего не говорил».
        INSERT INTO notification_preferences (tenant_id, user_id, type, enabled, scope, channel_overrides)
        VALUES (p_tenant_id, v_user, 'goal_changed', TRUE, 'subtree', '{}'::jsonb)
        ON CONFLICT DO NOTHING;

        -- Системные уведомления по умолчанию выключены; демо-пользователь включил.
        INSERT INTO notification_preferences (tenant_id, user_id, type, enabled, scope, channel_overrides)
        VALUES (p_tenant_id, v_user, 'access_requested', TRUE, NULL, '{}'::jsonb)
        ON CONFLICT DO NOTHING;
    END IF;

    IF v_migr_user IS NOT NULL THEN
        -- Явное отклонение: чек-ины в колокольчике не нужны. Строка уведомления
        -- всё равно создаётся — из неё собирается дайджест во внешний канал.
        INSERT INTO notification_preferences (tenant_id, user_id, type, enabled, scope, channel_overrides)
        VALUES (p_tenant_id, v_migr_user, 'kr_progress', TRUE, 'own', '{"in_app": false}'::jsonb)
        ON CONFLICT DO NOTHING;
    END IF;

    IF v_user IS NOT NULL THEN
        -- Пара уведомлений, чтобы колокольчик в демо не был пустым. Аctor —
        -- system:migration: уведомление визуально отличается от действия получателя.
        INSERT INTO notifications (tenant_id, user_id, type, kind, actor_user_id, team_id, period_id,
                                   goal_id, entity_title, payload_json, coalesce_key, coalesce_count)
        SELECT p_tenant_id, v_user, 'goal_changed', 'goal_fields_changed', COALESCE(v_migr_user, v_user),
               g.team_id, g.period_id, g.id, g.title, '{}'::jsonb, 'demo:goal:' || g.id, 1
        FROM goals g
        WHERE g.tenant_id = p_tenant_id
        ORDER BY g.id
        LIMIT 2
        ON CONFLICT DO NOTHING;

        -- Уведомление об общей цели: goal 30 принадлежит Media и расшарена в
        -- Продукт1. team_id — команда-участник, переход ведёт на её доску.
        INSERT INTO notifications (tenant_id, user_id, type, kind, actor_user_id, team_id, period_id,
                                   goal_id, entity_title, payload_json, coalesce_key, coalesce_count)
        SELECT p_tenant_id, v_user, 'goal_changed', 'goal_shared', COALESCE(v_migr_user, v_user),
               s.team_id, g.period_id, g.id, g.title,
               jsonb_build_object('added_team_ids', jsonb_build_array(s.team_id)),
               'demo:shared:goal:' || g.id, 1
        FROM goals g
        JOIN goal_shares s ON s.goal_id = g.id AND s.tenant_id = p_tenant_id
        WHERE g.tenant_id = p_tenant_id
          AND g.id = (SELECT new_id FROM seed_map WHERE kind='goal' AND old_id=30)
        ON CONFLICT DO NOTHING;

        INSERT INTO notifications (tenant_id, user_id, type, kind, actor_user_id, team_id, period_id,
                                   goal_id, kr_id, entity_title, payload_json, coalesce_key, coalesce_count)
        SELECT p_tenant_id, v_user, 'kr_progress', 'kr_checked_in', COALESCE(v_migr_user, v_user),
               s.team_id, g.period_id, g.id, kr.id, kr.title,
               jsonb_build_object(
                   'before', jsonb_build_object('progress', 20, 'health', 'on_track', 'note', ''),
                   'after', jsonb_build_object('progress', 45, 'health', 'on_track', 'note', ''),
                   'goal_title', g.title,
                   'note_changed', FALSE),
               'demo:shared:kr:' || kr.id, 1
        FROM goals g
        JOIN goal_shares s ON s.goal_id = g.id AND s.tenant_id = p_tenant_id
        JOIN key_results kr ON kr.goal_id = g.id
        WHERE g.tenant_id = p_tenant_id
          AND g.id = (SELECT new_id FROM seed_map WHERE kind='goal' AND old_id=30)
        ORDER BY kr.id
        LIMIT 1
        ON CONFLICT DO NOTHING;
    END IF;

    -- ----------------------------------------------------------------
    -- Демо-заявка на доступ: пользователь с поданной, ещё не рассмотренной
    -- заявкой — виден в очереди «Заявки» админки и в уведомлении.
    -- ----------------------------------------------------------------
    INSERT INTO users (provider_subject_key, provider, subject, display_name, email)
    VALUES ('demo:requester', 'github', 'demo-requester', 'Ольга Заявкина', 'olga.requester@example.com')
    ON CONFLICT (provider_subject_key) DO NOTHING;

    SELECT id INTO v_req_user FROM users WHERE provider_subject_key = 'demo:requester';

    INSERT INTO memberships (user_id, tenant_id, role, status)
    VALUES (v_req_user, p_tenant_id, 'user', 'requested')
    ON CONFLICT (user_id, tenant_id) DO NOTHING;

    IF v_user IS NOT NULL THEN
        INSERT INTO notifications (tenant_id, user_id, type, kind, actor_user_id, entity_title,
                                   payload_json, coalesce_key, coalesce_count)
        SELECT p_tenant_id, v_user, 'access_requested', 'access_requested', v_req_user,
               t.name, '{}'::jsonb, 'demo:access_requested:tenant:' || t.id, 1
        FROM tenants t
        WHERE t.id = p_tenant_id
        ON CONFLICT DO NOTHING;
    END IF;
END;
$func$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP FUNCTION IF EXISTS seed_demo_data(bigint);
-- +goose StatementEnd
