-- Demo seed data for OKR Tracker
-- Applies on a clean database after all migrations have run.
--
-- Единственный источник правды о демо-данных — SQL-функция seed_demo_data
-- (миграция 051): её же вызывает приложение при создании нового пространства.
-- Этот файл — тонкая обёртка для локального окружения: наполняет демо-структурой
-- пространство по умолчанию (id 1, создаётся миграцией 027). Периоды вычисляются
-- от текущей даты — см. комментарий к функции.
--
-- Повторный запуск безопасен: функция сначала удаляет своё прежнее наполнение
-- пространства (только пространства — другие тенанты не затрагиваются).
--
-- Usage:
--   psql -U postgres -d okrs -f seed_demo.sql
-- or via Docker Compose:
--   docker compose exec -T db psql -U postgres -d okrs < seed_demo.sql

-- Тенант по умолчанию — на случай standalone-запуска сида.
INSERT INTO
    tenants (id, slug, name) OVERRIDING SYSTEM VALUE
VALUES (1, 'default', 'Default')
ON CONFLICT (id) DO NOTHING;

SELECT seed_demo_data(1);
