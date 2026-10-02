## Why

Доска команды показывает над списком целей сводку по дочерним командам: агрегированную плашку
прогресса по подчинённой иерархии и сетку карточек прямых потомков. Та же навигация по дочерним
командам уже есть в дереве слева, вместе с прогрессом по каждому узлу. Сводкой пользуются редко, а
занимает она почти весь первый экран: у родительского узла его собственные цели уходят под сгиб.

Раздел нужно убрать, оставив единственную точку входа в дочерние команды — дерево навигации.

## What Changes

- **BREAKING** С доски удаляется блок сводки по дочерним командам целиком: и сетка карточек прямых
  потомков, и агрегированная плашка (средний прогресс по подчинённой иерархии, число команд с
  целями, прогноз темпа).
- **BREAKING** Удаляется эндпоинт `GET /api/v1/teams/{teamID}/overview`: после удаления блока у
  него не остаётся ни одного потребителя.
- Доска любого узла начинается сразу с его собственных целей. Заголовок «Цели этого узла»
  удаляется: списку целей больше не предшествует другой блок, и отделять его не от чего.
- Узел без собственных целей показывает пустое состояние «Цели не добавлены» независимо от наличия
  дочерних команд. Раньше у узла с потомками это место занимала сводка, и пустое состояние не
  показывалось.
- Навигация по дочерним командам и их прогресс остаются в дереве слева — без изменений.
- Вместе со сводкой удаляется её расчёт в usecase, неиспользуемый публичный метод
  `DirectChildrenSummary` и цепочка кода, у которой после этого не остаётся потребителей:
  запрос «момент последнего обновления команды в периоде» и помощники обхода иерархии
  `FindDirectChildren` / `CollectDescendantIDs`.
- Удаляются стили `.cluster-*` и `.child-card*`, карточка дизайн-системы «Карточки команд» и
  мёртвая функция `childCard` в прототипе.
- Обновляются `README.md`, `docs/interface.md` и `docs/images/README.md`; скриншот
  `docs/images/cluster-overview.png` удаляется.

## Capabilities

### New Capabilities

Нет.

### Modified Capabilities

- `team-okr-board`:
  - удаляются requirements «Сводка по дочерним командам», «Показатели сводки по дочерним
    командам», «Строка дочерней команды в сводке», «Момент последнего обновления дочерней
    команды», «Сводка требует периода и не изменяет данные»;
  - добавляется requirement «Доска показывает только собственные цели узла»: содержимое доски
    начинается с её целей, а пустое состояние не зависит от наличия потомков;
  - Purpose спецификации при archive перестаёт упоминать сводку по дочерним командам.

Requirement «Показатели узла в дереве навигации» не меняется. После этого изменения дерево — то
единственное место, где виден прогресс и статус дочерних команд, поэтому его показатели трогать
нельзя.

Прочие capabilities не затрагиваются. `authorization` упоминает дочерние команды только в контексте
распространения грантов на потомков, что к сводке не относится.

## Противоречия между кодом и текущими specs

- «Сводка по дочерним командам» требует, чтобы сводка агрегировала всю глубину дочерней иерархии.
  Код агрегирует всю глубину только в плашке; карточки строятся по прямым потомкам
  (`FindDirectChildren`). Requirement и код удаляются, расхождение исчезает вместе с ними.
- Интерфейс показывает в плашке подпись «N из M с целями», где `N` — число команд с целями по всей
  подчинённой иерархии (`teams_with_goals`), а `M` — число прямых потомков (длина `items`). Две
  величины из разных охватов стоят рядом как одна дробь, то есть подпись неверна. Текущая
  спецификация этой дроби не описывает вовсе — требование говорит только про «число таких команд».
  Отдельно исправлять дефект не нужно: блок удаляется.
- `okrboard.UseCase.DirectChildrenSummary` экспортирован, но не вызывается ниоткуда. Мёртвый код
  ещё до этого изменения; удаляется вместе с остальной сводкой.

## Impact

- **Удаляется на бэкенде:**
  - `internal/http/handlers/api/v1/teams/overview` целиком, вместе с регистрацией в
    `internal/http/server.go` и `internal/http/handlers/api/v1/testutil/integration.go`;
  - `teamscommon.TeamOverviewResponse`, `teamscommon.ChildrenSummaryResponse`,
    `teamscommon.CollectOverviewUserUDIDs`;
  - `dto.TeamOverviewResponse`, `dto.TeamChildrenSummaryResponse`, `dto.TeamChildSummaryResult`;
  - `okrboard.TeamOverviewFor`, `okrboard.DirectChildrenSummary`,
    `okrboard.buildDirectChildrenSummary` и типы `okrboard.TeamOverview`,
    `okrboard.TeamChildSummary`;
  - `goal.Service.ListTeamLastUpdateInPeriod`, метод `ListTeamLastGoalUpdateInPeriod` в порту
    `goal.Repo`, его реализация в `internal/store/goals` и заглушки в
    `internal/service/servicetest`;
  - `teamsvc.FindDirectChildren` и `teamsvc.CollectDescendantIDs`.
- **Остаётся без изменений:** `teamscommon.TeamOKRResponse`, `okrboard.TeamOKR`,
  `okrboard.TeamSummary` и `GetTeamsWithPeriodSummary`, `GET /api/v1/hierarchy` с показателями
  узлов, расчёты прогресса и темпа.
- **API:** удаляется `GET /api/v1/teams/{teamID}/overview`. Остальные эндпоинты не меняются,
  формат ответов не меняется.
- **Frontend** (`web/static/tracker.js`): компоненты `ClusterView` и `ChildCard`, состояние
  `overview` и оба запроса сводки, флаг `hasChildren`, заголовок «Цели этого узла», условия
  пустых состояний, где участвует `overview`. В `web/static/tracker.css` удаляются блоки
  `.child-card*` и `.cluster-*`.
- **Тесты:** удаляются `TestTeamOverviewIncludesChildrenSummaryIntegration`,
  `TestGetTeamOverview`, `internal/usecase/okrboard/internal_test.go`,
  `internal/http/handlers/api/v1/teams/overview/handler_test.go`, `TestFindDirectChildren`,
  `TestCollectDescendantIDs`; из `TestKRActivityTimestampsUsedForGoalAndTeamUpdates` уходит
  проверка команды, проверка цели остаётся. Удаление маршрута фиксирует существующий
  `TestRoutesGolden`: в `internal/http/testdata/routes.golden` становится на одну строку меньше.
- **Дизайн-система:** удаляется карточка `design-system/src/cards/child-cluster-cards.html` и
  мёртвая `childCard` в `design-system/prototype/app.js`. Файл `design-system/tracker.html` —
  исторический макет вне сборки, не меняется.
- **Документация:** `README.md`, `docs/interface.md`, `docs/images/README.md`, удаление
  `docs/images/cluster-overview.png`. Исторические `docs/old_specs` и `docs/superpowers` не
  меняются.
- **Данные:** миграций нет, схема не меняется, seed demo не меняется.
