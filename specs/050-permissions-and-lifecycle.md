# Permissions and Lifecycle

## Текущее состояние

В проекте уже есть доменный статус периода команды:

- `no_goals`
- `forming`
- `in_progress`
- `validated`
- `closed`

Статус:

- хранится в БД;
- читается и отображается в UI;
- может обновляться через API;
- валидируется по допустимому набору значений.

## Что реализовано сейчас

На текущий момент backend гарантирует следующее:

- статус должен быть одним из допустимых значений;
- статус можно сохранить для пары `(team_id, period_id)`;
- **аутентификация** через OAuth2/OIDC провайдеры (Google, GitHub, Keycloak) или режим без авторизации;
- **сессии** хранятся на сервере (PostgreSQL), клиент получает только session ID в cookie;
- **роли**: `user` и `admin` (флаг `is_admin` на пользователе); admin-панель и admin API доступны только администраторам;
- **scope доступа**: пользователь видит только команды, к которым ему выданы hierarchy grants и их потомков (рекурсивная CTE); scope вычисляется `PolicyEvaluator` на каждый запрос;
- **no-auth mode**: при `AUTH_MODE=disabled` все маршруты доступны, операции выполняются от имени `anonymous-local` (IsAdmin=true).

На текущий момент **не реализованы как строгие серверные гарантии**:

- правила переходов между статусами;
- блокировка structural edits goal / KR в `validated` или `closed`.

## Актуальная interpretation lifecycle

На данный момент lifecycle следует понимать так:

### `no_goals`

Техническое состояние периода без оформленных целей.

### `forming`

Период находится в процессе заполнения и редактирования.

### `in_progress`

Период активен, данные и прогресс продолжают обновляться.

### `validated`

Статус существует в домене и UI, но пока не сопровождается обязательными server-side ограничениями на изменение данных.

### `closed`

Статус существует в домене и UI, но пока не делает период гарантированно read-only на уровне API.

## Текущее ограничение

Lifecycle (team period status) — это в первую очередь:

- доменное поле;
- UI-сигнал (кнопка «Добавить цель» скрыта при `validated`/`closed`);
- организационная договорённость.

Lifecycle ещё не является полноценной policy enforcement model на сервере — structural edits не блокируются API при `validated`/`closed`.

## Текущие роли и права

### Роли

- `user` — любой авторизованный пользователь;
- `admin` — пользователь с флагом `is_admin=true`.

### Права admin

- управление командами (`/admin/teams`);
- управление периодами (`/admin/periods`);
- управление пользователями и их grants (`/admin/access`, `/api/v1/admin/*`).

### Права user

- просмотр OKR в пределах scope (hierarchy grants);
- CRUD goal / KR / comment / progress в доступных командах;
- обновление team period status.

## Target state

Целевое состояние для будущих итераций:

### Granular roles

- `viewer`
- `editor`
- `validator`
- `admin`

### Scope

- global
- subtree of team hierarchy
- single team

### Целевые права

- `viewer`: только просмотр;
- `editor`: CRUD goal / KR / comment / progress в доступных командах;
- `validator`: перевод периода в `validated`;
- `admin`: periods, teams, permissions, reopen closed period.

## Target lifecycle transitions

Целевая модель переходов:

- `no_goals -> forming`
- `forming -> in_progress`
- `in_progress -> validated`
- `validated -> closed`

Исключения:

- `validated -> in_progress` только для `admin`;
- `closed -> in_progress` только для `admin` с обязательным audit reason.

## Target lifecycle restrictions

### `forming` / `in_progress`

Разрешены:

- create / update / delete goal;
- create / update / delete KR;
- reorder;
- share goal;
- comments;
- progress update.

### `validated`

Разрешены:

- comments;
- progress update;
- reorder, только если это отдельно подтверждено продуктовым решением.

Запрещены:

- structural edits goal / KR.

### `closed`

По умолчанию всё read-only.

Любые исключения должны быть явно описаны в отдельной spec.

## Требование к будущей реализации

Когда lifecycle enforcement будет реализован, backend должен:

- валидировать допустимость перехода статуса;
- применять lifecycle-ограничения в mutation handlers;
- возвращать согласованную ошибку при нарушении policy;
- не полагаться только на UI.

## Требование к новым фичам

Любая новая mutation-фича должна явно отвечать на вопросы:

- зависит ли она от `team period status`;
- разрешена ли она в `validated`;
- разрешена ли она в `closed`;
- проверяется ли это на сервере;
- зависит ли она от будущих permissions / roles.

## Team deletion lifecycle

Для оргструктуры сервер теперь обязан применять отдельные lifecycle/visibility правила:

- удаление команды проверяется на сервере;
- если у команды есть goals хотя бы в одном периоде, сервер выполняет только soft delete;
- hard delete разрешён только если goals нет ни в одном периоде;
- при нарушении правила hard delete сервер должен возвращать согласованную ошибку уровня конфликта/бизнес-ограничения;
- при удалении команды её дети автоматически перепривязываются к родителю удаляемой команды;
- visibility команд на `/api/v1/teams` и `/api/v1/teams/{teamID}/okrs` определяется на сервере с учётом soft delete, выбранного периода и наличия goals в этом периоде;
- активные команды остаются видимыми даже без goals;
- soft-deleted команда скрывается только если в выбранном периоде у неё нет goals.
