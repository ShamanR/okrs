# Permissions and Lifecycle

## Текущее состояние

В репозитории явно формализованы lifecycle-статусы периода, но аутентификация и авторизация в явном виде пока не описаны.

Поэтому permissions стоит вынести в отдельную vNext-спеку, а не размазывать по фичам.

## Предлагаемая permission model

### Роли

- `viewer`
- `editor`
- `validator`
- `admin`

### Scope

- global
- subtree of team hierarchy
- single team

### Права

- `viewer`: только просмотр;
- `editor`: CRUD goal / KR / comment / progress в доступных командах;
- `validator`: перевод периода в `validated`;
- `admin`: periods, teams, permissions, reopen closed period.

## Lifecycle period status

Разрешённый базовый переход:

- `no_goals -> forming`
- `forming -> in_progress`
- `in_progress -> validated`
- `validated -> closed`

Исключения:

- `validated -> in_progress` только для `admin`;
- `closed -> in_progress` только для `admin` с обязательным audit reason.

## Ограничения по статусу

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

- progress update;
- reorder;
- comments.

Запрещены:

- structural edits goal / KR.

### `closed`

По умолчанию всё read-only.

Сохранение текущего поведения “можно двигать порядок и обновлять прогресс” должно быть отдельным явным продуктовым решением, а не неявной деталью.

## Требование к серверу

Ограничения по статусу должны проверяться сервером, а не только UI.
