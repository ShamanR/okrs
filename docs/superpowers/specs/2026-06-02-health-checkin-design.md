# Health Check-in — Design Spec

**Date:** 2026-06-02  
**Branch:** healthCheckIn  
**Status:** Approved

---

## Назначение

Health Check-in — инструмент руководителя/владельца дерева целей. За выбранный период автоматически находит проблемные места в OKR и показывает их сгруппированными по типам.

**Цели:**

- Дать руководителю единую точку, где видно что требует внимания прямо сейчас, без ручного обхода всех команд.
- Отделить проблемы процесса (нет обновлений, не заведены цели, ошибки конфигурации) от проблем результата (отставание от плана).

---

## Scope правила

> **TODO (tech debt):** Вся привязка scope к пользователю идёт через `user.display_name` (строковое совпадение с `teams.lead` и `goal.owner_text`). Это ведёт к коллизиям при одинаковых именах. Переработать на `user.ID`-based matching — в отдельной итерации.

Кнопка появляется только если у пользователя есть scope:

1. **Lead-scope**: команды, где `teams.lead = user.display_name` + рекурсивно все их потомки (полное поддерево).
2. **Owner-scope**: команды, владеющие целями текущего периода, где `goal.owner_text` содержит `display_name` как отдельное слово (разделители — запятая/пробел, case-insensitive). Аналогично тому, как работает `ListUserLeadTeams`. Только сами команды, без потомков.

Union обоих множеств. Если scope пуст — `has_scope: false`, кнопка не рендерится.

**Period**: анализируется тот период, который выбран в sidebar трекера.

---

## Категории проблем

| # | Категория | Иконка | В счётчике по умолчанию | Условие |
|---|-----------|--------|------------------------|---------|
| 1 | Нет обновлений | 🕐 | ✓ | `days_since_update > STALE_DAYS` (default 7). `days_since_update` = дней с момента max(`kr.progress_updated_at`) по KR цели. Если все KR никогда не обновлялись — считается stale. Применяется только к целям с хотя бы одним KR; цели без KR попадают в `formation_errors` (no_krs). |
| 2 | Не заведены цели | ○ | ✓ | Команда в scope, `has_goals = false` в периоде. |
| 3 | Ожидают перевода в работу | ○ | ✓ | Команда с целями, статус `ready` (К валидации). Черновики (`forming`) сюда не попадают. |
| 4 | Ошибки формирования | ⚠ | ✓ | Только для команд в статусе `ready` (К валидации) или `in_progress` (В работе). На уровне команды: Σ весов целей ≠ 100 (±`weight_tolerance`). На уровне цели: нет KR; Σ весов KR ≠ 100; PROJECT KR без стадий или Σ весов стадий ≠ 100; LINEAR/PERCENT KR с `target == start` (нулевой диапазон); KR без title. |
| 5 | Отстающие | ▼ | ✗ (информационная) | `days_since_update ≤ STALE_DAYS` И `goal.progress < expectedPace − BEHIND_MARGIN` (default 10 п.п.). `expectedPace = clamp(0, elapsed_fraction * 100, 100)` — тот же расчёт что `progress_meta.forecast`. |

**`total_problems`** = Σ count по категориям с `in_counter: true`.

---

## Backend

### Новый endpoint

```
GET /api/v1/health-checkin?period_id=<int64>
```

- Доступен любому авторизованному пользователю (не только admin).
- Scope определяется сервером по `user.display_name` из сессии.
- Если scope пуст: `200 { "has_scope": false, "total_problems": 0 }`.

### Response shape

```json
{
  "has_scope": true,
  "period_id": 1,
  "total_problems": 5,
  "categories": {
    "stale": {
      "in_counter": true,
      "count": 2,
      "items": [
        {
          "team_id": 2,
          "team_name": "Team A",
          "team_path": ["Cluster X", "Unit Y", "Team A"],
          "goal_id": 10,
          "goal_title": "Запустить продукт",
          "days_since_update": 10
        }
      ]
    },
    "no_goals": {
      "in_counter": true,
      "count": 1,
      "items": [
        { "team_id": 3, "team_name": "Team B", "team_path": ["Cluster X", "Team B"] }
      ]
    },
    "awaiting_validation": {
      "in_counter": true,
      "count": 1,
      "items": [
        { "team_id": 4, "team_name": "Team C", "team_path": [...], "status": "forming" }
      ]
    },
    "formation_errors": {
      "in_counter": true,
      "count": 1,
      "items": [
        {
          "team_id": 5,
          "team_name": "Team D",
          "team_path": [...],
          "entity_type": "team",
          "error_type": "weight_sum_not_100",
          "actual_weight_sum": 80
        },
        {
          "team_id": 5,
          "team_name": "Team D",
          "team_path": [...],
          "entity_type": "goal",
          "goal_id": 20,
          "goal_title": "Цель без KR",
          "error_type": "no_krs"
        }
      ]
    },
    "lagging": {
      "in_counter": false,
      "count": 0,
      "items": []
    }
  }
}
```

`error_type` enum: `weight_sum_not_100` | `no_krs` | `kr_weight_sum_not_100` | `project_no_stages` | `project_stage_weight_sum_not_100` | `kr_zero_range` | `kr_no_title`.

### Слои

**`internal/service/healthcheckin.go`**

```go
type HealthCheckInConfig struct {
    StaleDays          int
    BehindMargin       int
    WeightTolerance    int
    InCounter          map[string]bool
}

type HealthCheckInResult struct {
    HasScope     bool
    PeriodID     int64
    TotalProblems int
    Categories   map[string]*HealthCheckInCategory
}

func (s *Service) GetHealthCheckIn(ctx context.Context, userDisplayName string, periodID int64, cfg HealthCheckInConfig) (*HealthCheckInResult, error)
```

Метод:

1. Получает `*PeriodData` из `HealthCheckInCache` (in-memory или lazy DB load).
2. Вычисляет scope в памяти по `userDisplayName`.
3. Запускает `computeCategories(data, scopeIDs, cfg, period)` — pure in-memory.

**`internal/service/healthcheckin_cache.go`**

```go
type PeriodData struct {
    PeriodID    int64
    Period      domain.Period
    Teams       []domain.Team
    GoalsByTeam map[int64][]domain.Goal   // goals с KR + meta
    Statuses    map[int64]domain.TeamPeriodStatus
    CachedAt    time.Time
}

type HealthCheckInCache struct {
    mu      sync.RWMutex
    periods map[int64]*PeriodData
    ttl     time.Duration
    loader  func(ctx context.Context, periodID int64) (*PeriodData, error)
}

func (c *HealthCheckInCache) Get(ctx context.Context, periodID int64) (*PeriodData, error)
func (c *HealthCheckInCache) InvalidateAll()
func (c *HealthCheckInCache) StartRefreshLoop(ctx context.Context, interval time.Duration, activePeriodFn func() int64)
```

DB-загрузка — 3 batch-запроса:

1. `ListAllTeams()` — все команды для scope и hierarchy.
2. `ListGoalsByTeamsPeriod(periodID, allTeamIDs)` — все цели с KR и метаданными.
3. `ListTeamPeriodStatuses(periodID, allTeamIDs)` — статусы команд.

Активный период обновляется фоновой горутиной каждые `cache_ttl_minutes` (default 5). Если `FindPeriodForDate` возвращает ошибку (нет активного периода) — горутина пропускает итерацию без паники и повторяет попытку на следующем тике. Исторические периоды — lazy load + TTL expiry.

**Инициализация в `server.go`:**

```go
hcCache := service.NewHealthCheckInCache(st, cfg.CacheTTL)
go hcCache.StartRefreshLoop(ctx, cfg.CacheTTL, func() int64 { /* FindPeriodForDate(time.Now()) */ })
svc := service.NewFromStore(st, grantsCache, hcCache)
```

**`internal/http/handlers/api/v1/healthcheckin/handler.go`** — новый handler пакет.

### Admin settings endpoints

```
GET  /api/v1/admin/settings/health-checkin
POST /api/v1/admin/settings/health-checkin
```

Хранение: ключ `health_checkin_config` в `system_settings` (существующая таблица, без миграции).

Default config (вшит в код, применяется если ключа нет в БД):

```json
{
  "stale_days": 7,
  "behind_margin": 10,
  "weight_tolerance": 0,
  "cache_ttl_minutes": 5,
  "in_counter": {
    "stale": true,
    "no_goals": true,
    "awaiting_validation": true,
    "formation_errors": true,
    "lagging": false
  }
}
```

После `POST` — вызов `hcCache.InvalidateAll()` чтобы новый конфиг подхватился немедленно.

### NFR

- **NFR-1**: Вычисление — pure in-memory после первой загрузки. Типичный response time < 5 мс.
- **NFR-2**: Все пороги в `health_checkin_config` → изменяются через admin panel без рестарта.
- **NFR-3**: Детерминировано: одни и те же `PeriodData` + `userDisplayName` → одинаковый результат.

---

## Frontend

### Компоненты в `tracker.js`

**`HealthCheckInButton`** — нижняя часть sidebar, над аккаунт-виджетом:

- Показывается только если `has_scope: true`.
- Badge: число `total_problems`, amber (#f59e0b) если > 0, neutral-grey (#6b7280) если 0.
- Клик → открывает `HealthCheckInPanel`.

**`HealthCheckInPanel`** — `position: fixed; right: 0; top: 0; height: 100vh; width: 480px`, CSS-transition slide-in, backdrop-overlay.

Структура панели:

```
[✕]  ⚡ Health Check-in
     Найдено проблем: N  /  Всё в порядке  /  Проблем нет · есть отстающие

────────────────────────────────
[Все·N] [🕐·n] [⚠·n] [○·n] [▼·n]
────────────────────────────────
🕐 Нет обновлений (n)
  ▸ Team A
    • Цель X · 10 дней назад
      [→ Обновить прогресс]
  ▸ Team B
    • Цель Y · 8 дней назад
      [→ Обновить прогресс]

○ Не заведены цели (n)
  ▸ Team C
      [→ Перейти к команде]
```

### Цветовая кодировка (UX-1)

| Категория | Иконка | Цвет |
|-----------|--------|------|
| `stale` | 🕐 | amber `#f59e0b` |
| `no_goals` | ○ | grey `#6b7280` |
| `awaiting_validation` | ○ | grey `#6b7280` |
| `formation_errors` | ⚠ | red `#ef4444` |
| `lagging` | ▼ | blue `#3b82f6` |

### Subtitle логика

- Проблем > 0: «Найдено проблем: N»
- Проблем = 0, но есть `lagging`: «Проблем нет · есть отстающие цели»
- Всё пусто: «Всё в порядке»

### Фильтр-чипы

Чипы показываются для каждой непустой категории. Повторный клик / клик на «Все» — сброс фильтра. Если активный фильтр не даёт элементов — показывается «По выбранному фильтру ничего нет».

### Действия из панели (UX-4)

| Категория | Действие при клике |
|-----------|-------------------|
| `stale` | Выбрать команду в sidebar → прокрутить к цели |
| `no_goals` | Выбрать команду в sidebar |
| `awaiting_validation` | Выбрать команду в sidebar (status stepper виден) |
| `formation_errors` (goal) | Выбрать команду → открыть GoalModal цели |
| `formation_errors` (team) | Выбрать команду в sidebar |
| `lagging` | Выбрать команду → прокрутить к цели |

### Data flow

- Фетч при монтировании App и при смене `periodID` (параллельно с остальными запросами).
- Ref: `hciData` на уровне App, передаётся в Sidebar.
- После мутации KR/goal (обновление прогресса, смена статуса): если панель открыта — refetch `health-checkin`.
- Badge и subtitle вычисляются из одного объекта `hciData` (единый источник, FR-5).

### Новые CSS-классы в `tracker.css`

```css
.hci-button          /* кнопка в sidebar */
.hci-badge           /* badge-счётчик */
.hci-panel           /* правая панель, position:fixed */
.hci-panel--open     /* transition: translate(0) */
.hci-backdrop        /* полупрозрачный overlay */
.hci-chip            /* фильтр-чип */
.hci-chip--active    /* активный чип */
.hci-section         /* секция категории */
.hci-section__header /* заголовок секции с иконкой и счётчиком */
.hci-item            /* строка проблемы */
.hci-item__action    /* ссылка-действие */
```

---

## Admin panel

### Новый раздел

- Маршрут: `/admin/health-checkin` → тот же `admin-shell`.
- Новая вкладка в навигации admin SPA: **⚡ Health Check-in**.

### UI

Форма с секциями по одной на категорию:

```
⚡ Health Check-in — настройки
Эти параметры определяют, какие проблемы попадают в счётчик
и с какими порогами отображаются в трекере.

🕐 Нет обновлений
  [✓] Входит в счётчик
  Порог (дней): [7]
  Подсказка: Цели и KR без обновления прогресса более N дней.

○ Не заведены цели
  [✓] Входит в счётчик
  Подсказка: Команды без ни одной цели в периоде.

○ Ожидают перевода в работу
  [✓] Входит в счётчик
  Подсказка: Команды со статусом «Черновик» или «К валидации».

⚠ Ошибки формирования
  [✓] Входит в счётчик
  Допуск по весам (%): [0]
  Подсказка: Суммы весов ≠ 100, отсутствие KR, нулевые диапазоны.

▼ Отстающие
  [ ] Входит в счётчик
  Отставание (п.п.): [10]
  Подсказка: Цели ниже ожидаемого темпа. Информационная категория.

Кеш
  Время жизни (мин): [5]
  Подсказка: Интервал фонового пересчёта. Меньше — актуальнее,
  больше — меньше нагрузка на БД.

[Сохранить]
```

### Backend для admin

Новые handlers в `internal/http/handlers/api/v1/admin`:

- `HandleGetHealthCheckInSettings` — читает `health_checkin_config` из `system_settings`.
- `HandleUpdateHealthCheckInSettings` — пишет, затем вызывает `hcCache.InvalidateAll()`.

---

## Mismatch между текущим кодом и specs

1. **`specs/040-api-contract.md`** не содержит `GET /api/v1/health-checkin` — нужно добавить.
2. **`specs/020-domain-model.md`** не описывает `SystemSettings` ключ `health_checkin_config` — нужно добавить.
3. **KR metadata в domain**: при реализации уточнить, что `domain.KeyResult` включает `PercentMeta.StartValue`, `PercentMeta.TargetValue`, `LinearMeta.StartValue`, `LinearMeta.TargetValue` и `ProjectStages []KRProjectStage` с `.Weight` для formation error checks.

---

## Definition of done

- [ ] Обновлены specs (040-api-contract, 020-domain-model).
- [ ] Новый `HealthCheckInCache` с TTL и background refresh loop.
- [ ] Новый service метод `GetHealthCheckIn` с pure in-memory computation.
- [ ] Handler `GET /api/v1/health-checkin`.
- [ ] Admin handlers `GET/POST /api/v1/admin/settings/health-checkin`.
- [ ] Новый маршрут `/admin/health-checkin` + вкладка в admin SPA.
- [ ] `HealthCheckInButton` + `HealthCheckInPanel` в `tracker.js`.
- [ ] Новые CSS-классы в `tracker.css`.
- [ ] Unit-тесты на `computeScope` и `computeCategories` (pure functions).
- [ ] Тест на handler (golden path + empty scope).
- [ ] Нет новых SQL-миграций (используется существующая `system_settings`).
