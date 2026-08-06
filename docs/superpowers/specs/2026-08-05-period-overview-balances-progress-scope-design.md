# Обзор периода: балансы, график прогресса, охват команд

Дата: 2026-08-05
Ветка: `period-review-add-teams-scope`

## Контекст и текущее состояние

Раздел «Обзор периода»:

- Backend: `internal/service/period_overview.go` (`PeriodOverview`, `computePeriodOverview`), хендлер `HandlePeriodOverview` в `internal/http/handlers/api/v1/admin/service_handler.go` (маршрут `GET /api/v1/admin/periods/{periodID}/overview`), данные грузятся через `HealthCheckInCache` (`internal/service/healthcheckin_cache.go`, TTL, ключ tenant+period), загрузчик — closure `hcLoader` в `internal/http/server.go`.
- Frontend: `internal/web/static/period-overview.js` (контейнер), `internal/web/static/period_overview_view.js` (презентационный компонент), shell `internal/http/templates/period_overview_shell.html`.
- Сейчас эндпоинт **admin-only** (`RequireTenantAdminMiddleware`) и всегда грузит **все команды тенанта** (`ListAllTeams`). В ответе — только корзины статусов, ошибки весов и один снапшот `avg_progress`.

Домен: у `Goal` уже есть `work_type` (discovery/delivery), `focus_type` (4 фокуса), `priority` (P0–P3) — но нигде в обзоре не агрегируются. Историю прогресса нигде не храним. У `Team` есть `parent_id`, `lead_udid`. Есть рекурсивный `ListDescendantTeamIDs` (`internal/store/grants/grants.go`).

## Mismatch со спеками

1. Балансы (`work_type`/`focus_type`/`priority`) существуют на `Goal`, но не агрегируются в обзоре — чисто аддитивно.
2. Истории прогресса нет — нужна новая механика хранения.
3. Обзор admin-only + tenant-wide; нужен доступ не-админам в охвате своих команд.
4. «Мои команды» по спеке = команды, где ты `lead_udid` (не гранты).
5. `040-api-contract.md`, `050-permissions-and-lifecycle.md`, `020-domain-model.md` — обновить.

## Решения (подтверждены)

- «Мои команды» = команды, где `lead_udid == текущий UDID`, + все вложенные потомки.
- История прогресса: суточные снапшоты прогресса **каждой команды** в отдельную таблицу, фоновой горутиной; закрытые/архивные периоды и неактуальные (soft-deleted / без целей) команды скипаем.
- Тоггл охвата: дефолт «Мои команды»; «Вся организация» видят только админы.
- Охват (тоггл) влияет на все три блока: статусы/качество, балансы, график.
- Для уже закрытых периодов новых точек не появится (только то, что собрано, пока период был активен, + demo-seed) — принято.

---

## A. Охват «Мои команды / Вся организация»

Понятие *overview scope* внутри тенанта — отдельно от `domain.TenantScope` (мультитенантность).

### Backend

- Параметр `?scope=my_teams|org` (дефолт `my_teams`) на эндпоинте обзора.
- Эндпоинт выносим из admin-only группы в группу аутентифицированных. Авторизация в хендлере: `org` → только tenant-admin (иначе 403); `my_teams` → любой аутентифицированный.
- Резолв «моих команд»: новый store-метод `ListLeadRootTeamIDs(ctx, scope, userUDID)` (команды с `lead_udid = userUDID`, не soft-deleted), затем `ListDescendantTeamIDs` для разворота до потомков. Результат — `map[int64]bool`.
- `HealthCheckInCache` остаётся **tenant-wide** (грузим все команды один раз, без per-user кэша, без запросов в цикле). `computePeriodOverview(data, weightTolerance, teamIDFilter map[int64]bool)`; `nil` = вся организация, иначе — фильтрация команд и целей в памяти по set'у id.
- Метрики (`total_teams`, `teams_with_goals`, `avg_progress`, `weight_error_count`) и корзины статусов считаются уже по отфильтрованному набору.

### Frontend

- Тоггл в шапке «Мои команды / Вся организация». Дефолт «Мои команды». Кнопку «Вся организация» показываем только при `is_admin` (из `/api/v1/config`).
- Выбор охвата хранится в состоянии страницы; смена — рефетч обзора и графика с новым `scope`.
- Пустое состояние: не-админ без своих команд видит «Мои команды» с пустым охватом и пояснением.

## B. Балансы целей

### DTO (расширение ответа обзора)

```
balances {
  discovery_delivery: [ { key, count, percent } ]   // work_type
  focuses:            [ { key, count, percent } ]   // focus_type, 4 фикс. категории
  priorities:         [ { key, count, percent } ]   // priority P0..P3, все 4 всегда
}
goals: [ { id, title, team_id, team_name, work_type, focus_type, priority, progress } ]
```

- Корзины считаются по целям в охвате за период. Категории с нулём тоже присутствуют (стабильный порядок, как на скрине).
- `percent` — доля от общего числа целей в охвате (округление до целых).
- Тонкий список `goals` отдаётся в том же ответе → drill-down по клику на полосу считается на клиенте без доп. запросов (без N+1).

### Frontend

- Общий переиспользуемый компонент `BalanceBars({ title, subtitle, items, onSelect })`: полоса + `count` + `percent`, клик по полосе разворачивает список входящих целей (title, команда, прогресс). Три экземпляра: Discovery/Delivery, Стратегические фокусы, Приоритеты. Консистентно с существующими тайлами.

## C. График прогресса за период

### Хранение

Новая таблица:

```
team_period_progress_snapshots (
  id, tenant_id, period_id, team_id,
  snapshot_date date, progress int, created_at,
  UNIQUE (tenant_id, period_id, team_id, snapshot_date)
)
```

Миграция + обновление seed demo (наполнить снапшотами Q1 2026, чтобы демо-график был непустым).

### Сбор — фоновая горутина (раз в сутки)

По образцу `HealthCheckInCache.StartRefreshLoop`:

- проход по тенантам → их **активные** периоды (`status == active`; closed/archived скипаем) → активные команды с целями (soft-deleted и команды без целей скипаем);
- прогресс команды считаем тем же путём, что `computePeriodOverview` (`okr.PeriodProgress` по целям команды);
- **upsert** снапшота на текущую (UTC) дату — идемпотентно (`ON CONFLICT ... DO UPDATE`).

**Multi-instance (K8S):** upsert по unique-ключу делает повторный проход на нескольких подах безопасным (без дублей). Дополнительно — Postgres advisory-lock вокруг суточного прохода, чтобы фактически работал один под, а не все реплики одновременно.

### Чтение / API

- Ряд агрегируем из per-team снапшотов: на каждую `snapshot_date` — средний прогресс по командам-с-целями **в выбранном охвате** (тот же `teamIDFilter`, что в разделе A).
- К историческим точкам добавляем «живую» точку на сегодня из текущего `PeriodData` — чтобы график был актуален, не дожидаясь суточной джобы (учёт ожиданий консистентности).
- Ответ: `period_start`, `period_end`, `points: [ { date, progress } ]`.
- Прогресс до старта / после конца периода рендерим в крайних левых/правых позициях.

Вариант размещения: отдельный подраздел ответа обзора либо отдельный эндпоинт `GET .../periods/{id}/overview/progress-series?scope=`. Финализируем на этапе плана; предпочтение — включить в тот же ответ обзора, чтобы страница делала один запрос на охват.

### Frontend

- Переиспользуемый SVG-компонент графика: ось X — даты старт→конец, ось Y — 0–100%, точки по датам, пунктирная диагональ «ровное заполнение» (ориентир идеально ровного заполнения), крайние ромбы для out-of-range точек. Матчит скрин.

## D. Specs и тесты

Обновить в этом же change set:

- `040-api-contract.md` — `scope`-параметр, поля `balances`/`goals`/`progress_series`, доступ не-админам к `my_teams`.
- `050-permissions-and-lifecycle.md` — кто что видит (не-админ → только свои команды; `org` → только админ).
- `020-domain-model.md` — сущность `TeamPeriodProgressSnapshot`.

Unrelated specs не трогаем.

Тесты:

- резолв «мои команды» = `lead_udid` + рекурсивные потомки;
- авторизация: `scope=org` не-админу → 403; `my_teams` — доступен;
- фильтрация охвата в `computePeriodOverview` (метрики и корзины по отфильтрованному набору);
- агрегация балансов: counts + percent, нулевые категории присутствуют, стабильный порядок;
- drill-down: список целей соответствует корзине;
- снапшоты: идемпотентный upsert (повтор в тот же день не плодит строк / обновляет), скип closed/архивных периодов и команд без целей;
- агрегация ряда прогресса по охвату + добавление живой точки + кламп краёв (до старта/после конца).

## Порядок реализации

1. A (охват) — фундамент: store-метод, фильтр в compute, авторизация, вынос маршрута, FE-тоггл.
2. B (балансы) — DTO + агрегатор + `BalanceBars`.
3. C (график) — миграция, snapshot store, фоновая горутина (advisory-lock), агрегатор ряда, SVG-компонент, seed.
4. D — specs + тесты (ведём по ходу каждого шага).

## Открытые вопросы (решаем в плане)

- Точное место `progress_series` (в ответе обзора vs отдельный эндпоинт) — предпочтение: в ответе обзора.
- Периодичность/тайминг суточной джобы и стартовый прогон при запуске.
