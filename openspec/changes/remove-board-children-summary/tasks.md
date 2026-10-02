## 1. Согласование вида до реализации

- [x] 1.1 Собрать прототип (`./design-system/prototype/build.sh`) и согласовать на экране доски
  целевой вид: содержимое начинается со списка целей, блока сводки и заголовка «Цели этого узла»
  нет. Прототип уже рендерил доску без сводки (`childCard` в `app.js` не вызывался), поэтому
  правок разметки для согласования не потребовалось. Проверка: пользователь выбрал вариант «весь
  блок целиком» при согласовании scope.

## 2. Frontend: удаление блока с доски

- [x] 2.1 Удалить из `web/static/tracker.js` компоненты `ClusterView` и `ChildCard`, состояние
  `overview`/`setOverview`, оба запроса `GET /api/v1/teams/{id}/overview` (в эффекте по
  `periodId`/`selId` и в `reload`) и флаг `hasChildren`. Проверка: в `web/static/tracker.js` нет
  вхождений `ClusterView`, `ChildCard`, `children_summary`, `overview`; синтаксис JSX проверен
  вендорным `babel.min.js` в `node` — OK; в консоли браузера ошибок нет.
- [x] 2.2 Переписать в `App` разметку контента без сводки: убрать заголовок «Цели этого узла»,
  оставить `goalWeightWarn` одним вхождением перед `BoardFilterBar` (design, решение 7), переписать
  условия пустых состояний на `allGoals.length === 0` и `hierarchy.length` без `overview`
  (design, решение 4). Проверено в запущенном приложении на специально собранной иерархии
  Org → {Division → Payments, Growth, Platform}:
  - узел с потомками и своими целями (Division) — доска начинается со списка целей, сводки нет;
  - узел с потомками без своих целей (Org) — пустое состояние «Цели не добавлены» с кнопкой
    создания;
  - узел без потомков (Platform) — доска без изменений;
  - предупреждение о сумме весов целей рендерится один раз.
- [x] 2.3 Удалить из `web/static/tracker.css` блоки `.child-card*` и `.cluster-*` (включая
  `.cluster-loading`). Проверка: `grep 'child-card|cluster-' web/static` пуст, остальной вид
  трекера не изменился.

## 3. API: удаление эндпоинта

- [x] 3.1 Удалить пакет `internal/http/handlers/api/v1/teams/overview` вместе с `handler_test.go` и
  снять его регистрацию в `internal/http/server.go` (`registerApiRoutes`) и
  `internal/http/handlers/api/v1/testutil/integration.go` (`NewAPIV1RouterWithScope`). Проверка:
  `go build ./...` проходит; `teamsoverview|teams/overview` в `*.go` не встречается.
- [x] 3.2 ~~Добавить проверку 404 в `teams/routes_test.go`~~ → **заменено**: удаление маршрута
  фиксирует существующий `TestRoutesGolden` (design, решение 5). Обновлён
  `internal/http/testdata/routes.golden` через `go test ./internal/http/ -update-routes`. Проверка:
  в golden ровно одна убранная строка `GET /api/v1/teams/{teamID}/overview`, тест зелёный.
- [x] 3.3 Удалить `TestTeamOverviewIncludesChildrenSummaryIntegration` из
  `internal/http/handlers/api/v1/teams/integration_test.go`. Проверка:
  `go test ./internal/http/handlers/api/v1/teams/...` зелёный, DB-тесты пакета не пропущены.
- [x] 3.4 Удалить из `internal/http/handlers/api/v1/teams/teamscommon/teamscommon.go` функции
  `TeamOverviewResponse`, `ChildrenSummaryResponse` и `CollectOverviewUserUDIDs`, а из
  `internal/http/dto/team.go` — типы `TeamOverviewResponse`, `TeamChildrenSummaryResponse` и
  `TeamChildSummaryResult`. Проверка: `go build ./... && go vet ./...` проходит; `TeamOKRResponse`
  и `CollectOKRUserUDIDs` на месте, `dto.TeamInfo` и `dto.ProgressBarInfo` сохранены.

## 4. Usecase: удаление расчёта сводки

- [x] 4.1 Удалить из `internal/usecase/okrboard/okrboard.go` методы `TeamOverviewFor`,
  `DirectChildrenSummary`, `buildDirectChildrenSummary` и типы `TeamOverview`, `TeamChildSummary`.
  Удалить `TestGetTeamOverview` из `okrboard_test.go` и файл `internal_test.go` целиком. Проверка:
  `go test ./internal/usecase/okrboard/...` зелёный, остальные тесты пакета не менялись.
- [x] 4.2 Проверить, что `okrboard.TeamSummary`, `GetTeamsWithPeriodSummary` и `GetTeamOKR` не
  затронуты и неиспользуемых импортов не осталось. Проверка: `go vet ./...` чист (потребовалось
  убрать осиротевший импорт `domain` в `teamscommon.go`).

## 5. Service и store: срез мёртвой цепочки

- [x] 5.1 Удалить `goal.Service.ListTeamLastUpdateInPeriod`, запись
  `ListTeamLastGoalUpdateInPeriod` в порту `goal.Repo`, её реализацию в
  `internal/store/goals/goals.go` и заглушки в `internal/service/servicetest/goalstore.go` и
  `internal/service/servicetest/store.go`. Store-тест переработан по design, решение 8: проверка
  «правка метаданных KR обновлением не считается» перенесена на `ListGoalsByTeamPeriod`, проверка
  по расшаренной команде удалена, тест переименован в
  `TestKRActivityTimestampsUsedForGoalUpdates`. Проверка:
  `go test ./internal/store/... ./internal/service/...` зелёный с поднятым Docker, `ListTeamLast`
  в `*.go` не встречается.
- [x] 5.2 Удалить `FindDirectChildren` и `CollectDescendantIDs` из
  `internal/service/team/team.go` с тестами `TestFindDirectChildren` и `TestCollectDescendantIDs`,
  обновив комментарий в начале файла. Проверка: `go build ./... && go test
  ./internal/service/team/...` зелёный; `FindDirectChildren|CollectDescendantIDs` в `*.go` не
  встречается.
- [x] 5.3 Пройти по списку остановки из design (решение 3) и убедиться, что перечисленные там
  символы сохранены и у них остались вызовы. Проверка: `go build ./... && go vet ./...` чист,
  `ChildrenSummary|TeamChildSummary|TeamOverview` в `*.go` не встречается (кроме не связанного с
  change `TeamOverviewStats`, см. раздел 9).

## 6. Дизайн-система

- [x] 6.1 Удалить карточку `design-system/src/cards/child-cluster-cards.html` и мёртвую функцию
  `childCard` из `design-system/prototype/app.js`. Сверх плана: удалённые классы остались ещё в
  четырёх карточках, и после удаления CSS они рендерились бы без стилей —
  `progress.html` (секция «Сводка по кластеру»), `foundations-surfaces.html` (тень
  `.child-card:hover`), `foundations-typography.html` (три записи каталога шрифтов),
  `empty-states.html` (`.child-card__empty`, `.cluster-loading`). Все вычищены. Пересобрано:
  `./design-system/build.sh && ./design-system/prototype/build.sh`. Проверка: в `dist/` нет
  `components/child-cluster-cards.html` (32 карточки вместо 33), скрипт прототипа проходит
  `vm.Script`, ни одного вхождения удалённых классов в `design-system/src` и `design-system/dist`.
- [ ] 6.2 Залить обновлённые карточки и прототип в проект Claude Design
  (`12e552f0-1aaf-4d84-9013-9cef887d4dd2`). **Только вручную:** `/design-sync` зарезервирован за
  явным вызовом пользователя и не может быть запущен агентом, обходить его через DesignSync нельзя.
  Залить нужно пять изменённых карточек (`progress`, `foundations-surfaces`,
  `foundations-typography`, `empty-states`, `prototype`) и удалить `child-cluster-cards`. Проверка:
  в проекте нет карточки «Карточки команд», остальные карточки на месте.

## 7. Документация

- [x] 7.1 Убрать из `docs/interface.md` пункт про cluster overview и ссылку на скриншот, удалить
  строку `cluster-overview.png` из `docs/images/README.md` и сам файл
  `docs/images/cluster-overview.png`. Проверка: `cluster overview|cluster-overview` в `docs` (без
  `docs/old_specs` и `docs/superpowers`) не встречается, ссылки на изображения не битые.
- [x] 7.2 `README.md` описывал удалённый блок в шести местах, а не в одном (найдено на ревью).
  Поправлены: вводный абзац, пункт списка возможностей, подпись к скриншоту доски, раздел «Team
  board», перечисление в Quick start и правило роллапа прогресса. Последнее — отдельная находка:
  «a parent node's progress = the average across child teams that actually have goals» описывало
  `AverageProgress` из удалённой сводки, и после change такого правила в продукте нет нигде
  (прогресс в дереве — только по собственным целям узла, см. requirement «Показатели узла в дереве
  навигации»). Проверка: `roll-up|child team` в `README.md` не встречается, текст про subtree
  остался только там, где он верен (экспорт, область видимости уведомлений).
- [x] 7.3 Перегенерировать `docs/screenshots/tracker-board.png`: старый показывал плашку, сетку
  карточек и заголовок «Цели этого узла». Снят тот же экран (Платформа, Y2026, 23 %) на
  `seed_demo.sql` в изолированной БД, 1600×950 как у остальных скриншотов. Проверка: на новом
  скриншоте блока нет, прогресс дочерних команд виден в дереве, ошибок в консоли нет. Остальные 12
  скриншотов удалённый блок не показывают.

## 8. Итоговая проверка

- [x] 8.1 Полный прогон: `go build ./... && go vet ./... && go test ./...` зелёный, Docker поднят,
  0 пропусков «docker unavailable». Проверка:
  `children_summary|ChildrenSummary|TeamChildSummary|cluster-overview|child-card` находит только
  исторический `docs/old_specs/040-api-contract.md`, `design-system/tracker.html` (исторический
  макет вне сборки) и `openspec/changes/`.
- [x] 8.2 Ручная проверка в запущенном приложении. Сервер поднят на изолированном Postgres
  (отдельный контейнер на порту 55432, локальная БД не затронута), иерархия собрана вручную.
  Проверено: `GET /api/v1/teams/1/overview` отвечает 404; доска узла с потомками и своими целями,
  доска узла с потомками без своих целей, доска листового узла, дерево навигации с прогрессом по
  дочерним командам, степпер статусов, фильтры доски. Ошибок в консоли браузера нет. Окружение
  проверки удалено.
- [ ] 8.3 Сверить реализацию с delta-спеком: `openspec validate remove-board-children-summary`.
  При archive удалить пять requirements сводки из `openspec/specs/team-okr-board/spec.md`,
  добавить «Доска показывает только собственные цели узла» и убрать упоминание сводки по дочерним
  командам из Purpose. Проверка: после archive `openspec/specs/team-okr-board/spec.md` не содержит
  «сводк» про дочерние команды, содержит новый requirement, а requirement «Показатели узла в
  дереве навигации» не изменён.

## 9. Найдено по пути, в scope не входит

- `internal/store/goals/goals.go`: `ListTeamOverviewStats` и тип `TeamOverviewStats` не вызываются
  ниоткуда, а заглушка в `internal/service/servicetest/store.go` прямо называется
  `listTeamOverviewStatsUnused`. Это мёртвый код, существовавший до этого change: удаляемая сводка
  его не вызывала, имя совпадает случайно. Не трогаем — unrelated refactoring. Кандидат на
  отдельный change.
