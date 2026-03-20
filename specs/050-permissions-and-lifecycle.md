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

На текущий момент backend гарантирует только следующее:

- статус должен быть одним из допустимых значений;
- статус можно сохранить для пары `(team_id, period_id)`.

На текущий момент **не реализованы как строгие серверные гарантии**:

- аутентификация;
- авторизация;
- роли;
- scope доступа к дереву команд;
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

Сейчас lifecycle — это в первую очередь:

- доменное поле;
- UI-сигнал;
- организационная договорённость.

Сейчас lifecycle ещё не является полноценной policy enforcement model на сервере.

## Target state

Целевое состояние для будущих итераций:

### Roles

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
