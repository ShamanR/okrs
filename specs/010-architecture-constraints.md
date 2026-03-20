# Architecture constraints

## Архитектурный стиль

Сохраняем текущий стиль:

- SSR-страницы отдают layout и HTML-каркас;
- данные и мутации идут через HTTP/API;
- фронтенд без сборщика, минимальный vanilla JS;
- PostgreSQL как единственное системное хранилище;
- SQL-миграции — единственный способ менять схему;
- Docker Compose остаётся базовым локальным способом запуска.

## Слои

AI должен сохранять разделение ответственности:

- internal/domain — доменные типы и enum;
- internal/okr — расчёты прогресса;
- internal/store — SQL и persistence;
- internal/service — доменные сценарии и orchestration;
- internal/http — SSR handlers и templates;
- internal/api/v1 — API-контракт для JSON/form-data.

## Жёсткие правила для AI-реализации

1. Никакой бизнес-логики в handlers.
2. Никаких schema changes без новой migration.
3. Новые внешние API — только под /api/v1.
4. Все вычисления прогресса должны оставаться в одном месте.
5. Изменения UI не должны требовать JS bundler/toolchain.
6. Существующие URL и сценарии не ломать без явной migration spec.

## Definition of done для любой фичи

Фича считается завершённой только если:

- обновлена спека;
- добавлена миграция, если менялась БД;
- обновлён store/service;
- добавлены тесты на расчёты и/или обработчики;
- обновлён README/API section, если менялся контракт.
