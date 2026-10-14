## Context

Мотивация — в proposal.md. Что есть сейчас:

- **Данные.** `goals.focus_type TEXT NOT NULL` (миграция `001_init`) без CHECK. Значения
  проверяет `common.ValidateGoalInput` → `ValidFocusType` (`internal/http/handlers/web/common`).
  Доменный тип — `domain.FocusType` с четырьмя константами (`internal/core/domain/models.go`).
- **Чтение и запись.** Поле читают и пишут `GoalRepository` (`Create`, `Update`,
  `UpdateFields`, `Get`, `ListGoalsBy*`, `ListGoalsForPeriods`, `CopyGoal`). Наружу оно уходит
  через `GoalDetails`, `GoalResponse`, `GoalTreeGoal`, `PeriodGoalItem` и баланс
  `PeriodBalances.Focuses`. В выгрузку попадает через `focusLabel` в `render/export`.
- **Фронтенд.** `tracker.js`: `FOCUS_OPTIONS`, `focusLabel`, `FOCUS_COLORS`, `FOCUS_HINT`, лейбл
  на карточке, `<select>` в окне цели. `goal_tree.js`: `gtFocusLabel` в панели деталей.
  `period_overview_view.js`: `PO_FOCUS_*` и `BalanceBars` «Стратегические фокусы».
  `activity.js` о фокусе ничего не знает.
- **Компоненты, на которые опираемся.** `PrioritySelect` с классами `.pri-select*` (раскрытый
  список с описанием каждого уровня); `PriorityFilter` в `BoardFilterBar` и состояние вида доски
  `boardView` (`normalizeBoardView`, `boardViewEquals`, `boardViewIsDefault`, `visibleGoals`);
  `useFloatingTip` и `FloatingTip` — подсказка, которая рендерится порталом в `body` и поэтому не
  обрезается `overflow:hidden` и не ломается от transform холста. Всё это сейчас живёт в
  `tracker.js`. `ui.js` подключён в шаблонах трекера, обзора, журнала, админки и настроек, но
  не в `goal_tree_shell.html`.
- **Развёртывание.** Kubernetes, несколько реплик, rolling update. Миграции применяет
  стартующий под (`cmd/server/main.go::runMigrations`). Какое-то время старые и новые поды
  работают с одной схемой одновременно.

## Goals / Non-Goals

**Goals:**

- Направление описано в одном месте на сервере (домен) и в одном месте на клиенте (общий
  модуль). Иконка, название, описание и цвет нигде не дублируются.
- Иконка с подсказкой — один общий компонент для доски, дерева, обзора и журнала.
- Селектор направления и селектор приоритета — один компонент, а не два похожих.
- Миграция не роняет старые поды во время rolling update.
- Без N+1: направление в ленте журнала приходит тем же запросом, что и страница ленты.

**Non-Goals:**

- Писать смену направления в журнал как изменённое поле. Сейчас смена фокуса туда не попадает,
  и это изменение такую запись не добавляет.
- Иконка направления в уведомлениях колокольчика, в выгрузке, в списке корневых целей дерева и
  в других местах, не перечисленных в specs.
- Удаление колонки `focus_type` — отдельный change в следующем релизе (см. «Migration Plan»).
- Фильтр по направлению в дереве целей и в журнале.

## Decisions

### 1. Модель: `domain.GoalDirection` с кодами `PRODUCT | TECH | PROCESS`

`FocusType` и его константы удаляются. Вместо них — `GoalDirection` с константами
`DirectionProduct`, `DirectionTech`, `DirectionProcess`, упорядоченным списком
`GoalDirections()` и `Valid()`. Поле `Goal.FocusType` переименовывается в `Goal.Direction`.
Проверка допустимости переезжает в домен, потому что это правило предметной области, а не
HTTP. `ValidateGoalInput` вызывает `Valid()` и отдаёт сообщение «некорректное направление
цели».

Коды — латиницей UPPER_SNAKE, как у прежнего фокуса: так их можно хранить в БД и передавать в
API, а названия на русском задаёт клиент. Порядок «Продукт, Техника, Процессы» задаёт
`GoalDirections()`: по нему строится баланс обзора на сервере, клиент повторяет его в общем
модуле.

Альтернатива — оставить тип `FocusType` и поменять только значения. Отклонена: имя расходилось
бы с продуктовым понятием, а на этой путанице и держалась исходная проблема.

### 2. Схема БД: expand → contract, новая колонка `direction`

Миграция `050_goal_direction`:

1. `ALTER TABLE goals ADD COLUMN direction TEXT NOT NULL DEFAULT 'PRODUCT'
   CHECK (direction IN ('PRODUCT','TECH','PROCESS'))`;
2. `UPDATE goals SET direction = CASE focus_type WHEN 'PROFITABILITY' THEN 'PRODUCT'
   WHEN 'STABILITY' THEN 'TECH' WHEN 'TECH_INDEPENDENCE' THEN 'TECH'
   WHEN 'SPEED_EFFICIENCY' THEN 'PROCESS' ELSE 'PRODUCT' END`;
3. `ALTER TABLE goals ALTER COLUMN focus_type SET DEFAULT ''` — новый код в `focus_type` не
   пишет.

Down-миграция удаляет `direction` и возвращает `focus_type` без default. Строкам, созданным
новым кодом (`focus_type = ''`), она проставляет фокус по направлению: PRODUCT →
PROFITABILITY, TECH → STABILITY, PROCESS → SPEED_EFFICIENCY.

Почему не `RENAME COLUMN` с заменой значений: старый под во время rolling update сразу начнёт
падать на `SELECT focus_type`, и у части пользователей перестанет открываться доска. При
expand старый под продолжает читать `focus_type`: у старых целей значение на месте, у новых —
пустая строка, и бейдж просто не рисуется (`goal.focus && …`). Старый под, который создаёт
цель в это окно, получает `direction = 'PRODUCT'` по default. Такой дрейф ограничен окном
выкладки, и это приемлемо.

Почему не оставить имя колонки `focus_type` навсегда: имя хранилища расходилось бы с
доменом. Repository скрыл бы это от остальных слоёв, но SQL-запросы, сид и тесты читались бы
с ошибкой.

`CopyGoal` копирует `g.direction` вместо `g.focus_type`.

### 3. API: поле `direction`, ключ баланса `directions`

`focus_type` заменяется на `direction` во всех DTO и обработчиках: форма и JSON создания и
изменения цели, `GoalDetails`, `GoalResponse`, `GoalTreeGoal`, `PeriodGoalItem`. Ключ
`balances.focuses` становится `balances.directions`. Обратной совместимости нет: API
используют только наши страницы, а статика и сервер выкладываются одним образом.

Риск: после выкладки в открытой вкладке может остаться старый JS. Его запрос на сохранение без
`direction` сервер отклонит с ошибкой валидации, и пользователь увидит ошибку. После
перезагрузки страницы всё работает. Отдельно с этим ничего не делаем.

### 4. Клиент: общий модуль `web/static/goal_direction.js`

Новый файл подключается до страничных скриптов в шаблонах трекера, дерева целей, обзора,
журнала и админки: админка рендерит тот же `period_overview_view.js`. В нём:

- `GOAL_DIRECTIONS` — упорядоченный список `{ code, icon, label, hint, color }`, и
  `goalDirection(code)` — поиск по коду (неизвестный код → `null`);
- `useFloatingTip` и `FloatingTip` — переносятся из `tracker.js` без изменения поведения.
  `InfoHint` и подсказка обрезанного заголовка в трекере пользуются ими как раньше;
- `DirectionIcon({ code })` — `<span class="dir-icon" tabIndex=0 aria-label="Направление: …">`
  с эмодзи. По `mouseenter`/`focus` он показывает `FloatingTip` с названием (жирным) и
  описанием. Неизвестный или пустой код → `null`, ничего не рисуется.

Стили подсказки (`.info-hint__tooltip`, `--fit`) и новый `.dir-icon` переносятся в
`components.css`. Перед этим проверить, что `components.css` подключён во всех четырёх
шаблонах, и подключить, где его нет.

Почему отдельный файл, а не `ui.js`: `ui.js` не грузится на странице дерева. Все babel-скрипты
делят одну глобальную лексическую область, поэтому одноимённый `const` в двух файлах роняет
страницу. Отдельный маленький модуль проще подключить везде, и он не тянет с собой лишнего.
Альтернатива — CSS-подсказка `.has-tip` (`::after`). Отклонена: на холсте дерева её обрезает
`overflow:hidden` карточки, а позиционирование ломается от масштаба.

### 5. Селектор: общий `RichSelect` вместо `PrioritySelect` и копии для направления

Из `PrioritySelect` выделяется `RichSelect({ value, options, onChange, ariaLabel })`. Каждая
опция — `{ value, mark, title, hint }`, где `mark` — React-узел (бейдж приоритета или иконка
направления). Поведение не меняется: закрытие по клику снаружи, отметка ✓ у выбранного,
закрытие после выбора. Классы `.pri-select*` переименовываются в `.rich-select*`.
`PrioritySelect` и `DirectionSelect` становятся тонкими обёртками в `tracker.js`. В окне цели
на месте `<select>` фокуса стоит `DirectionSelect` с подписью «Направление цели» и подсказкой
`FieldLabel`, собранной из `GOAL_DIRECTIONS`. `FOCUS_*` и `focusLabel` удаляются. Новая цель
получает `direction: 'PRODUCT'`.

### 6. Карточка цели на доске

`DirectionIcon` ставится в `.gc2__title-wrap` перед `.goal-card__title-row`, то есть вне
кликабельной области заголовка: наведение и фокус на иконке не открывают редактирование.
Лейбл фокуса из `.gc2__labels` убирается. В ключе `headFit` поле `goal.focus` заменяется на
`goal.direction`. `mapGoal` кладёт `direction: g.direction`.

### 7. Фильтр по направлению

Состояние вида доски получает поле `dir: { PRODUCT?: true, … }`. Его учитывают
`BOARD_VIEW_DEFAULT`, `boardViewDefault`, `normalizeBoardView` (неизвестные коды
отбрасываются), `boardViewEquals` и `boardViewIsDefault`. Ключ хранилища не меняется: старое
сохранённое значение без `dir` нормализуется в пустой фильтр.

`visibleGoals` применяет шаги в порядке: приоритет → направление → нулевой вес → сортировка.
Закреплённая по ссылке цель (`keepId`) проходит и фильтр по направлению.

`DirectionFilter` повторяет устройство `PriorityFilter`: те же классы `.filter-bar__*`,
модификатор `.filter-bar__btn--dir` с цветом направления через `--pc`, кнопка — иконка,
название и счётчик, сброс ×. В `BoardFilterBar` он стоит сразу после фильтра по приоритету.

### 8. Дерево целей

`GoalTreeCard`: `DirectionIcon` в строке заголовка карточки. Если она сожмёт заголовок до
одной строки, дать заголовку `display:flex` с иконкой как `flex: none`. Панель деталей:
иконка перед `.gt-panel__title`. Строка `gtFocusLabel(goal.focus_type)` в бейджах панели
заменяется на название направления, а `gtFocusLabel` удаляется. `buildResponse` отдаёт
`direction`.

### 9. Обзор периода

Сервер: `directionOrder` берётся из `domain.GoalDirections()`, `PeriodBalances.Directions`
(`json:"directions"`) заменяет `Focuses`, `PeriodGoalItem.Direction` (`json:"direction"`)
заменяет `FocusType`. Клиент: `BalanceBars` с заголовком «Направления целей», подписями
«🚀 Продукт» и т. д. и цветами из `GOAL_DIRECTIONS`. Детализация — `field: 'direction'`,
заголовок «Направление: …». В строке цели списка детализации `DirectionIcon` стоит перед
заголовком. `PO_FOCUS_*` удаляются.

### 10. Журнал активности: текущее направление одним запросом

`ActivityRepository.List` получает `LEFT JOIN goals g ON g.id = e.goal_id AND
g.tenant_id = e.tenant_id` и выбирает `g.direction`. Удалённая цель даёт `NULL`. Значение
попадает в read-модель события ленты (`GoalDirection *domain.GoalDirection`) и в DTO
`ActivityEvent.goal_direction` (`omitempty`). Это тот же запрос страницы, поэтому N+1 нет.
`goals.id` — первичный ключ, так что join идёт по индексу. `GetByID`, счётчики и `TreeCounts`
не меняются.

Клиент (`eventText`): иконка ставится перед `«{t}»`, когда событие относится к самой цели
(`goal_id` есть, `kr_id` нет; `goal_*` и события комментариев к цели), и перед
`p.goal_title` в `kr_progress`. Перед заголовком KR (`kr_*` с `kr_id`) и перед командой в
`status_changed` иконки нет.

Почему текущее направление, а не снимок на момент события: так решил пользователь. Ещё одна
причина — старые записи сразу получают иконку без backfill payload.

### 11. Выгрузка

`focusLabel` в `render/export` заменяется на `directionLabel`, который возвращает «Продукт» /
«Техника» / «Процессы». Пустое значение по-прежнему пропускает сегмент.

### 12. Кеширование

Не нужно. Справочник направлений статичен и зашит в код. Направление цели читается в тех же
запросах, что и остальные поля цели. Новых запросов, которые стоило бы кешировать, нет.

### Интерфейс

- **Переиспользуются:** `FloatingTip`/`useFloatingTip` (теперь общие), раскрывающийся
  селектор приоритета (обобщается в `RichSelect`), `.filter-bar__*` и устройство
  `PriorityFilter`, `BalanceBars`, `FieldLabel`.
- **Добавляются:** `DirectionIcon` (`.dir-icon`), `DirectionSelect`, `DirectionFilter`
  (`.filter-bar__btn--dir`).
- **Дизайн-система:** обновить карточки `goal-card.html` (иконка вместо лейбла фокуса),
  `goal-form.html` (селектор «Направление цели» в раскрытом виде), `goal-tree-canvas.html`,
  `goal-tree-panel.html`, карточки блока фильтров и баланса обзора (найти по `focus`/`Фокус`
  в `design-system/src/cards/`), а также прототип: `data.js` (`FOCUS_*` → `GOAL_DIRECTIONS`)
  и `app.js` (`goalCard`, `goalModal`, `renderOverview`, `goalTreeCard`, `goalTreePanel`,
  лента журнала, фильтр доски). Завести карточку `direction-icon`.
- Вид согласуется на прототипе до реализации: экраны доски (карточка + фильтр), окна цели
  (раскрытый селектор), обзора (баланс «Направления целей»), дерева и журнала.

## Risks / Trade-offs

- [Перевод фокуса в направление приблизительный: Stability и Tech Independence склеиваются в
  «Технику»] → Таблицу утвердил пользователь. Команды правят направление сами, исторических
  записей фокуса не остаётся.
- [Rolling update: старый под видит пустой `focus_type` у новых целей и создаёт цели с
  `direction = PRODUCT`] → Окно ограничено временем выкладки, ничего не падает (см. решение 2).
- [Открытая вкладка со старым JS шлёт `focus_type` без `direction`] → Сервер отвечает ошибкой
  валидации. После перезагрузки вкладки всё работает.
- [Перенос `FloatingTip` и переименование `.pri-select` → `.rich-select` задевают
  существующие подсказки и селектор приоритета] → Поведение не меняется. Проверить A/B в
  браузере: подсказки «?» у полей, полный текст обрезанного заголовка, селектор приоритета.
- [Эмодзи по-разному выглядят на разных ОС] → Это осознанный выбор из запроса пользователя.
  Ширину `.dir-icon` фиксируем, чтобы шапка карточки не прыгала.

## Migration Plan

1. Выложить релиз с миграцией `050_goal_direction` (expand). Под, стартующий первым, применяет
   её. Старые поды продолжают работать на `focus_type`.
2. После выкладки проверить: `SELECT direction, count(*) FROM goals GROUP BY 1` — нет строк
   вне `PRODUCT/TECH/PROCESS`, итог совпадает с числом целей.
3. В следующем релизе отдельным change удалить колонку `focus_type` (contract). Задача на это
   стоит в tasks.md как follow-up, в этом change она не реализуется.
4. Откат: down-миграция `050` и предыдущий образ. Цели, созданные на новом коде, получат фокус
   по обратному соответствию из решения 2.
