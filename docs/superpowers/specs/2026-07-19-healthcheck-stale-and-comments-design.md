# Health Check-in — stale от начала периода + категория «Комментарии»

**Дата:** 2026-07-19
**Статус:** Согласовано

Дорабатываем логику колокольчика (Health Check-in). Две независимые части:

1. Исправление категории «Нет обновлений» (`stale`) — точка отсчёта для целей без обновлений.
2. Новая категория «Комментарии» (`comments`) с двумя под-типами: нерешённые комментарии в моём scope и мои решённые комментарии.

Связанные спеки-источники: [`2026-06-02-health-checkin-design.md`](2026-06-02-health-checkin-design.md), [`2026-07-13-stale-only-in-progress-design.md`](2026-07-13-stale-only-in-progress-design.md), `specs/020-domain-model.md`, `specs/040-api-contract.md`, `specs/050-permissions-and-lifecycle.md`.

---

## Часть 1 — «Нет обновлений» (`stale`): точка отсчёта = начало периода

### Проблема

В `computeCategories` ([`internal/service/healthcheckin.go`](../../../internal/service/healthcheckin.go)):

```go
isStale := len(g.KeyResults) > 0 && (lastProgress == nil || daysSince > cfg.StaleDays)
```

Цель, у которой прогресс **никогда** не обновлялся (`lastProgress == nil`), немедленно считается stale — даже если период только начался и «порог дней без обновления» ещё не прошёл. Это ложный сигнал.

### Требуемое поведение

- Если у цели не было ни одного обновления прогресса, точкой отсчёта берётся **начало периода** (`period.StartDate`). От неё отмеряется порог `StaleDays`, и лишь после его превышения цель попадает в «Нет обновлений».
- В категории «Нет обновлений» участвуют **только цели команд в статусе `in_progress`** («в работе»). Это существующий гейт `trackStale`, он сохраняется.
- Если период ещё не начался (`StartDate` в будущем) — такие цели в категорию **не** попадают, даже если у команды выставлен `in_progress`. Это следствие формулы (см. ниже), отдельного условия не требует.

### Решение

```go
ref := lastProgress                 // max(kr.progress_updated_at) по KR цели
if ref == nil {
    ref = &data.Period.StartDate
}
daysSince := int(now.Sub(*ref).Hours() / 24)
isStale := len(g.KeyResults) > 0 && daysSince > cfg.StaleDays
```

Итоговое условие попадания в `stale`:

```
status == in_progress  И  daysSince > StaleDays
```

где `daysSince` считается от `max(kr.progress_updated_at)`, либо, при отсутствии обновлений, от `period.StartDate`.

- `daysSince` может быть отрицательным (период в будущем) → условие ложно → не stale.
- `days_since_update` в ответе API для never-updated целей = дни от начала периода (не `0`).

### Frontend

`GoalCard` ([`internal/web/static/tracker.js`](../../../internal/web/static/tracker.js)) дублирует правило stale для предупреждения на карточке цели:

```js
const staleTracked = periodStatus === 'in_progress';
const isStale = staleTracked && goal.updatedDaysAgo > staleDays;
```

Синхронизируем: при `goal.updatedDaysAgo == null` (не было обновлений) считать дни от начала периода (backend уже отдаёт корректный `days_since_update`, фронт должен использовать ту же величину, а не обнулять её). Правило «только `in_progress`» и будущий период отсекаются так же, как на backend.

### Спеки

- `specs/040-api-contract.md` (правило `stale`): переписать формулировку — при отсутствии обновлений порог отсчитывается от начала периода; в категорию попадают только `in_progress` цели с превышенным порогом.

---

## Часть 2 — Категория «Комментарии» (`comments`)

Новая категория колокольчика с **двумя под-типами** внутри одной категории.

### Данные

Загрузчик `PeriodData` ([`internal/http/server.go`](../../../internal/http/server.go), `hcLoader`) расширяется: к целям периода догружаются комментарии (batch-запрос, как KR). Компьютинг категорий остаётся pure in-memory. Свежесть — в пределах TTL кэша (default 5 мин), как у остальных категорий.

`PeriodData` держит комментарии для **всех** целей периода (кэш общий на tenant+period, не per-user). Фильтрация per-user выполняется в `computeCategories` по `userUDID` — так же, как вычисление scope.

Комментарий (`domain.GoalComment`) уже содержит: `author_user_id`/автор, `resolved_at`, `resolved_by_user_id`/резолвер, `created_at`, `text`. Для матчинга «мой» используется UDID (как в остальном health-checkin).

### Под-тип 1 — Нерешённые комментарии (`unresolved`)

**Scope команд** (отдельная функция `computeCommentScope`, глубина ≠ полному lead-поддереву):

- База (depth 0): команды, где я лид (`teams.lead_udid == userUDID`, не удалённые), плюс owner-команды (где `userUDID ∈ goal.owner_udids` в этом периоде).
- Спуск от lead-команд на `comment_depth` уровней вниз по дереву (`comment_depth = 0` → только база; `1` → + прямые дети; `2` → + ещё уровень, и т.д.). Owner-команды берутся только как depth-0 (без спуска), консистентно с существующей owner-scope семантикой.

**Отбор:** все открытые комментарии (`resolved_at IS NULL`) на целях команд этого scope в выбранном периоде. Включая мои собственные комментарии (показываем все нерешённые).

**Элемент:** `team_id`, `team_name`, `team_path`, `goal_id`, `goal_title`, `comment_id`, `author_name`, `text` (markdown), `created_at`. Ссылка на цель — deep-link `?goal=<goal_id>&comment=<comment_id>` (существующий механизм навигации в `tracker.js`).

**Бейдж:** число нерешённых входит в `total_problems` только если включён admin-тумблер `in_counter["comments"]` (**default `false`**).

### Под-тип 2 — Мои решённые комментарии (`resolved`)

**Отбор:** комментарии, где `author == я` (по UDID) И `resolved_at IS NOT NULL` И `resolved_by != я`. Self-resolved (сам решил свой) — **исключаются** (в мой колокольчик не попадают).

**Лимит:** последние `resolved_comments_limit` (K, default 5) по `resolved_at DESC`, в пределах выбранного периода.

**Элемент:** `team_id`, `team_name`, `team_path`, `goal_id`, `goal_title`, `comment_id`, `text` (markdown), `resolved_at`, `resolved_by_name`. Deep-link — как выше.

**Механизм «просмотрено» (watermark в браузере, localStorage):**

- Клиент хранит в `localStorage` watermark — `comment_id`/`resolved_at` последнего просмотренного решённого комментария из счётчика (ключ, например, `hci_resolved_seen`).
- **Непросмотренные** = элементы `resolved` с `resolved_at` строго новее watermark. Их число прибавляется к бейджу **на клиенте**.
- При открытии панели watermark двигается на максимум `resolved_at` среди показанных элементов → счётчик непросмотренных обнуляется. Список последних K остаётся видимым.
- Хранение per-браузер (осознанный trade-off: на другом устройстве счётчик может появиться снова; выбрано ради простоты, без записей в БД и без per-instance-стейта на сервере в K8S).

**Бейдж:** сервер **не** знает seen-состояние → `resolved` в серверный `total_problems` не входит. Клиент показывает `displayed_badge = total_problems + unseen_resolved`.

### Форма ответа API

Категория `comments` имеет нестандартную форму (два под-списка вместо `items`); фронт рендерит её отдельной секцией:

```json
"comments": {
  "in_counter": true,
  "count": 2,
  "unresolved": [
    {
      "team_id": 2, "team_name": "Team A", "team_path": ["Cluster X", "Team A"],
      "goal_id": 10, "goal_title": "Запустить продукт",
      "comment_id": 55, "author_name": "Ivan", "text": "нужно уточнить KR",
      "created_at": "2026-07-15T10:00:00Z"
    }
  ],
  "resolved": [
    {
      "team_id": 2, "team_name": "Team A", "team_path": ["Cluster X", "Team A"],
      "goal_id": 10, "goal_title": "Запустить продукт",
      "comment_id": 40, "text": "поправил веса",
      "resolved_at": "2026-07-18T09:00:00Z", "resolved_by_name": "Petr"
    }
  ]
}
```

- `count` = число нерешённых (`unresolved`) — величина, участвующая в серверном `total_problems` (при `in_counter: true`).
- `resolved` в серверный счётчик не входит (см. watermark).
- Остальные категории (`stale`, `no_goals`, …) сохраняют прежнюю форму `{ in_counter, count, items }`.

### Admin-настройки

Расширяется `health_checkin_config` (`tenant_settings`, без миграции). Новые поля:

| Поле | Тип | Default | Смысл |
|------|-----|---------|-------|
| `comment_depth` | int ≥ 0 | `1` | Глубина спуска от моих lead-команд для нерешённых комментариев. |
| `resolved_comments_limit` | int ≥ 1 | `5` | Сколько последних решённых моих комментариев (K) показывать. |
| `in_counter["comments"]` | bool | `false` | Входят ли нерешённые комментарии в бейдж. |

Валидация в `POST /api/v1/admin/settings/health-checkin`: `comment_depth >= 0`, `resolved_comments_limit >= 1`. Невалидные/отсутствующие значения → дефолты (как для остальных полей в `LoadHealthCheckInConfig`).

Новая секция в admin-панели `/admin/health-checkin` («Комментарии»): тумблер «Входит в счётчик», поле «Глубина команд (уровней вниз)», поле «Сколько решённых показывать (K)».

### Frontend

- `HealthCheckInPanel` ([`tracker.js`](../../../internal/web/static/tracker.js)): спец-секция «Комментарии» с двумя под-списками (нерешённые / мои решённые). Текст комментария — через существующий компонент `<Markdown>`. Переход по элементу — через **единый** механизм `buildTargetURL` (вынесен в общий [`ui.js`](../../../internal/web/static/ui.js)), тот же, что в журнале событий: `<a href="/?team=&period=&goal=&comment=">`, который трекер разбирает на загрузке (выбор команды/периода, раскрытие цели и секции комментариев, скролл + подсветка). Отдельного `onSelectTeam`/scroll-механизма у колокольчика больше нет.
- Watermark-логика (localStorage) для под-типа 2: подсчёт непросмотренных, корректировка бейджа `SidebarBell`, сдвиг watermark при открытии панели.
- Фильтр-чипы: чип «Комментарии» показывается, если непусты `unresolved` или `resolved`; счётчик чипа = `unresolved.length + resolved.length` (сумма показанных элементов, консистентно с тем, что чип отражает объём секции, а не вклад в бейдж).

### Спеки

- `specs/040-api-contract.md`: форма ответа `GET /api/v1/health-checkin` — добавить категорию `comments`; тело `POST /api/v1/admin/settings/health-checkin` — новые поля `comment_depth`, `resolved_comments_limit`, ключ `comments` в `in_counter`.
- `specs/020-domain-model.md`: описание ключа `health_checkin_config` — добавить новые поля.

---

## Вне scope

- Миграции БД не добавляются (используются существующие `tenant_settings`, `user_settings` не задействуется — watermark в браузере).
- Права/доступ endpoint не меняются (`GET /api/v1/health-checkin` — любой авторизованный).
- Seed/demo не трогаем (структура таблиц не меняется).
- Матчинг scope остаётся UDID-based (существующий tech-debt по строковому матчингу не в этой итерации).

## Обязательные вопросы к mutation-фиче (по `050-permissions-and-lifecycle.md`)

Категория `comments` — **read-only** агрегирование, не mutation. Не зависит от team period status, разрешена в `validated`/`closed` (просто читает комментарии), проверок на сервере статуса не требует, от будущих permissions не зависит (scope по UDID уже есть).

## Definition of done

- [ ] Часть 1: `stale` считает порог от `period.StartDate` для never-updated целей; backend + frontend (`GoalCard`) синхронны.
- [ ] Часть 1: тесты — never-updated в начале периода → не stale; после порога от начала периода → stale; future-период → не stale; `in_progress`-гейт сохранён.
- [ ] Часть 2: `PeriodData` грузит комментарии; `computeCommentScope` (depth) покрыт тестами.
- [ ] Часть 2: категория `comments` в ответе (`unresolved` + `resolved`), self-resolved исключены, лимит K соблюдён.
- [ ] Часть 2: admin-поля `comment_depth`, `resolved_comments_limit`, `in_counter["comments"]` читаются/пишутся/валидируются; секция в `/admin/health-checkin`.
- [ ] Часть 2: frontend-секция «Комментарии» с markdown + deep-link; watermark (localStorage) корректирует бейдж.
- [ ] Обновлены спеки `040-api-contract.md`, `020-domain-model.md`.
- [ ] `go test ./...`, `go vet ./...` зелёные.
