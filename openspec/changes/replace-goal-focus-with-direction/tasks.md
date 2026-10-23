## 1. Согласование вида в прототипе

- [x] 1.1 `design-system/prototype/data.js`: заменить `FOCUS_COLORS`/`FOCUS_LABEL`/`PO_FOCUS_*`
      на `GOAL_DIRECTIONS` (код, иконка, название, описание, цвет), у демо-целей `focus` →
      `direction` по таблице перевода. Проверка: `./design-system/prototype/build.sh` и
      `node --check` на собранном файле проходят.
- [x] 1.2 `design-system/prototype/app.js`: в `goalCard` поставить иконку направления слева от
      заголовка и убрать лейбл фокуса; в `goalModal` заменить «Фокус» на раскрывающийся селектор
      «Направление цели» по образцу селектора приоритета; в фильтре доски добавить группу
      «Направление»; в `renderOverview` — баланс «Направления целей» и иконку в детализации; в
      `goalTreeCard`/`goalTreePanel` и ленте журнала — иконку перед заголовком цели; подсказка по
      наведению на иконку. Проверка: сборка прототипа, `node --check`, все экраны открываются
      без ошибок в консоли.
- [x] 1.3 Согласовать вид с пользователем на прототипе до реализации в приложении: доска
      (карточка и фильтр), окно цели (раскрытый селектор), обзор, дерево, журнал.

## 2. Домен и база данных

- [x] 2.1 `internal/core/domain/models.go`: заменить `FocusType` и его константы на
      `GoalDirection` (`PRODUCT`, `TECH`, `PROCESS`), `GoalDirections()` и `Valid()`;
      `Goal.FocusType` → `Goal.Direction`. Добавить unit-тест порядка и `Valid()`, включая
      отказ прежним значениям фокуса. Проверка: `go test ./internal/core/domain/...`.
- [x] 2.2 Миграция `migrations/050_goal_direction.up.sql`/`.down.sql` по решению 2 в design.md.
      Добавить тест миграции рядом с `internal/store/migration_*_test.go`: четыре цели с разными
      фокусами получают PRODUCT/TECH/PROCESS/TECH; вставка недопустимого направления отклоняется
      CHECK; down возвращает `focus_type` строкам с пустым фокусом. Проверка: тест проходит при
      запущенном Docker, в выводе нет «docker unavailable».
- [x] 2.3 `internal/store/goals`: все `SELECT`/`INSERT`/`UPDATE` и `CopyGoal` работают с
      `direction` вместо `focus_type`; `GoalInput`, `GoalUpdateInput`, `GoalFieldsUpdateInput`
      получают `Direction`. Проверка: `go build ./...`, тест копирования проверяет, что у копии то
      же направление.
- [x] 2.4 Обновить все тестовые фикстуры с `FocusType`/`focus_type` (`rg -n "FocusType|focus_type"
      internal` пуст, кроме миграционного теста) и сырые `INSERT INTO goals` в тестах. Проверка:
      `go vet ./...` и `go test ./...` зелёные.
- [x] 2.5 `internal/store/seed.go` и `seed_demo.sql`: перейти на направления, у демо-целей
      должны быть все три направления. Проверка: применить `seed_demo.sql` дважды к живой БД,
      затем `SELECT direction, count(*) FROM goals GROUP BY 1` показывает три направления.

## 3. Backend: API, обзор, журнал, выгрузка

- [x] 3.1 `common.ValidateGoalInput` принимает `domain.GoalDirection` и проверяет его через
      `Valid()`, сообщение «некорректное направление цели»; удалить `ValidFocusType`. Обработчики
      создания и изменения цели (`api/v1/goals`, `api/v1/teams/goals` и все остальные, что читают
      `focus_type`) читают поле `direction`. Тесты: недопустимое направление и прежний фокус
      дают 400, корректное сохраняется. Проверка: `go test ./internal/http/...`.
- [x] 3.2 DTO `GoalDetails`, `GoalResponse` (`goalcommon`), `GoalTreeGoal` и `MapGoalDetails`,
      `buildResponse` отдают `direction` вместо `focus_type`. Проверка: контрактный тест
      `goaltree` и тесты ответа цели проверяют поле `direction`.
- [x] 3.3 `usecase/period/overview.go`: `PeriodBalances.Directions` (`directions`) по
      `domain.GoalDirections()`, `PeriodGoalItem.Direction`. Обновить `overview_test.go`: три
      категории в фиксированном порядке, включая нулевые, и доли (сценарий «Баланс между
      направлениями»). Проверка: `go test ./internal/usecase/period/...`.
- [x] 3.4 `ActivityRepository.List`: `LEFT JOIN goals` и `g.direction` в read-модели события;
      DTO `ActivityEvent.goal_direction` (`omitempty`). Интеграционный тест: у события
      существующей цели направление есть, у удалённой — нет, после смены направления старое
      событие отдаёт новое; число запросов на страницу ленты не растёт. Проверка:
      `go test ./internal/store/activity/... ./internal/http/handlers/api/v1/activity/...`.
- [x] 3.5 `render/export`: `focusLabel` → `directionLabel` («Продукт»/«Техника»/«Процессы»).
      Обновить `export_test.go`. Проверка: `go test ./internal/render/export/...`.

## 4. Frontend: общий модуль и компоненты

- [x] 4.1 Создать `web/static/goal_direction.js`: `GOAL_DIRECTIONS`, `goalDirection(code)`,
      перенесённые из `tracker.js` `useFloatingTip`/`FloatingTip`, `DirectionIcon`. Подключить
      его в `tracker_shell.html`, `goal_tree_shell.html`, `period_overview_shell.html`,
      `activity_shell.html`, `admin_shell.html` (рендерит обзор периода) до страничных скриптов. Стили `.info-hint__tooltip*` и `.dir-icon`
      перенести в `components.css`; убедиться, что он подключён во всех четырёх шаблонах.
      Проверка: JSX-проверка через вендорный babel в node+vm без ошибок, на каждой странице нет
      ошибок в консоли (нет повторного объявления `const`).
- [x] 4.2 `tracker.js`/`tracker.css`: выделить `RichSelect` из `PrioritySelect`, переименовать
      `.pri-select*` → `.rich-select*`, добавить `DirectionSelect`. Проверка: A/B в браузере —
      селектор приоритета, подсказки «?» и подсказка обрезанного заголовка ведут себя как до
      изменения.

## 5. Frontend: экраны

- [x] 5.1 Окно цели: «Фокус» → `DirectionSelect` «Направление цели» на том же месте, подсказка
      поля из `GOAL_DIRECTIONS`; новая цель по умолчанию `PRODUCT`; сохранение шлёт `direction`
      (форма и JSON). Удалить `FOCUS_OPTIONS`, `FOCUS_COLORS`, `FOCUS_HINT*`, `focusLabel`.
      Проверка: в приложении создать цель без смены направления → 🚀; сменить на «Техника» →
      ⚙️ на карточке; раскрытый список показывает три варианта с описанием и отметкой.
- [x] 5.2 Карточка цели: `DirectionIcon` перед заголовком вне кликабельной области, лейбл фокуса
      убран, `headFit` учитывает `direction`, `mapGoal` кладёт `direction`. Проверка: наведение и
      Tab на иконке показывают подсказку и не открывают окно цели; шапка остаётся в одной строке.
- [x] 5.3 Фильтр по направлению: поле `dir` в состоянии вида доски (default, normalize, equals,
      isDefault, reset), шаг направления в `visibleGoals` после приоритета, `DirectionFilter` в
      `BoardFilterBar`. Проверка в браузере по сценариям spec: сужение, счётчик 0 и недоступность,
      сочетание с приоритетом, сохранение при смене команды и перезагрузке, сброс, цель по
      прямой ссылке не скрывается, старое сохранённое значение без `dir` открывается без ошибки.
- [x] 5.4 `goal_tree.js`: иконка на карточке холста и в панели деталей, `gtFocusLabel` удалён.
      Проверка: наведение на иконку показывает подсказку, не меняет выделение и не обрезается
      при масштабе холста.
- [x] 5.5 `period_overview_view.js`: баланс «Направления целей» из `balances.directions`,
      детализация по `direction`, иконка в строках списка детализации, `PO_FOCUS_*` удалены.
      Проверка: в обзоре видны три категории с долями, клик даёт список целей направления.
- [x] 5.6 `activity.js`: `DirectionIcon` перед заголовком цели в событиях цели и её комментариев
      и перед `goal_title` в `kr_progress`; перед KR и командой иконки нет. Проверка в браузере
      по сценариям spec `activity-log`, включая удалённую цель.

## 6. Дизайн-система и документация

- [x] 6.1 Обновить карточки `design-system/src/cards/`: `goal-card.html`, `goal-form.html`,
      `goal-tree-canvas.html`, `goal-tree-panel.html`, карточки фильтров доски и баланса обзора
      (`rg -n "focus|Фокус" design-system/src/cards` пуст); завести карточку `direction-icon`;
      переименовать классы `.pri-select*` в карточках. Проверка: `./design-system/build.sh` и
      `./design-system/prototype/build.sh` проходят, результат залит в проект Claude Design.
- [x] 6.2 `README.md` (описание карточки и балансов обзора), `docs/goals.md` (раздел «Фокус» →
      «Направление цели» с тремя категориями, чек-лист), `docs/interface.md`. Проверка:
      `rg -n "фокус|focus" README.md docs/goals.md docs/interface.md` не находит упоминаний поля
      фокуса цели.

## 7. Проверка и завершение

- [x] 7.1 Полный прогон: `go build ./...`, `go vet ./...`, `go test ./...` при запущенном Docker
      (в выводе 0 «docker unavailable»). Затем прогнать приложение локально по всем сценариям
      delta specs.
- [x] 7.2 Сверить реализацию с delta specs (`/opsx:verify`) и провести code review изменений.
- [ ] 7.3 Follow-up, вне этого change: после выкладки завести change на удаление колонки
      `goals.focus_type` (contract-шаг из Migration Plan). Проверка: change создан в
      `openspec/changes/`.
