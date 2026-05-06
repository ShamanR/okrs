# Architecture constraints

## Архитектурный стиль

Сохраняем текущий стиль:

- SSR-страницы отдают layout и HTML-каркас;
- данные и мутации идут через HTTP/API;
- фронтенд без сборщика: React 18 подключается через CDN (unpkg), JSX компилируется @babel/standalone в браузере — toolchain не нужен;
- компоненты трекера живут в `tracker.js`, стили — в `tracker.css`; добавление нового CDN-пакета = новый `<script>` в shell-шаблоне;
- компонент `UserSelector` реализован трижды с одинаковым поведением и CSS-классами: в `tracker.js` (React, multi-select для владельца цели), в `admin.js` (React, single-select для руководителя команды в React-админке), в `app.js` (vanilla JS, single-select для команд на Bootstrap-страницах); CSS-классы (`.user-selector*`, `.user-chip*`, `.user-avatar__fallback`) определены в `tracker.css`, `base.html` и `admin_shell.html`;
- PostgreSQL как единственное системное хранилище;
- SQL-миграции — единственный способ менять схему;
- Docker Compose остаётся базовым локальным способом запуска.

## Слои

AI должен сохранять разделение ответственности:

- `internal/domain` — доменные типы и enum, включая `User`, `AuthSession`;
- `internal/okr` — расчёты прогресса;
- `internal/store` — SQL и persistence; auth-методы (users, sessions, grants, settings) живут здесь;
- `internal/service` — доменные сценарии и orchestration;
- `internal/auth` — auth manager, middleware chain, provider interface, policy evaluator;
- `internal/auth/providers/{name}` — реализации провайдеров; каждый провайдер — изолированный пакет;
- `internal/http` — SSR handlers и templates;
- `internal/http/handlers/api/v1` — API-контракт для JSON/form-data;
- `internal/http/handlers/web/admin` — admin SSR handlers.

## Auth layer

Auth реализован как отдельный слой над роутером:

- `auth.Manager` — инициализирует провайдеры из конфига, управляет логином и сессиями;
- `auth.Provider` — интерфейс провайдера; регистрация через `auth.Register(name, factory)`;
- `auth.PolicyEvaluator` — вычисляет per-request набор доступных team IDs;
- Middleware chain: `AccessLog → Session → (RequireAuth) → (Scope) → CSRF → handlers`.

Провайдеры регистрируются через side-effect imports в `main.go`:

```go
import (
    _ "okrs/internal/auth/providers/google"
    _ "okrs/internal/auth/providers/github"
    _ "okrs/internal/auth/providers/keycloak"
)
```

Добавление нового провайдера = новый пакет + blank import + конфиг. Core-код не меняется.

## Режим без авторизации

При `AUTH_MODE=disabled`:

- весь middleware chain работает, но `RequireAuth` не применяется;
- в контекст запроса инжектируется системный пользователь `anonymous-local` (IsAdmin=true);
- `/admin/*` доступен всем;
- комментарии пишутся от `anonymous-local`.

## Жёсткие правила для AI-реализации

1. Никакой бизнес-логики в handlers.
2. Никаких schema changes без новой migration.
3. Новые внешние API — только под `/api/v1`.
4. Все вычисления прогресса должны оставаться в одном месте.
5. Изменения UI не должны требовать JS bundler/toolchain.
6. Существующие URL и сценарии не ломать без явной migration spec.
7. Любые state-changing HTTP действия (POST/PUT/PATCH/DELETE), вызываемые из браузера, должны проходить CSRF-проверку.
8. Любые пользовательские данные в UI должны рендериться через безопасное экранирование (без raw HTML-вставок).
9. Auth-данные не должны размазываться по handlers — только через middleware и context helpers.
10. Провайдер-специфичный код не должен появляться за пределами `internal/auth/providers/{name}`.

## Definition of done для любой фичи

Фича считается завершённой только если:

- обновлена спека;
- добавлена миграция, если менялась БД;
- обновлён store/service;
- добавлены тесты на расчёты и/или обработчики;
- обновлён README/API section, если менялся контракт.
