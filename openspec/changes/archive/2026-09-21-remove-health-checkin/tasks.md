## 1. Настройки прогресса в service/settings

- [x] 1.1 Перенести `ProgressSnapshotIntervalDaysKey` и `LoadProgressSnapshotIntervalDays` в `internal/service/settings`, заменить порт `SettingsReader` интерфейсом `settings.Reader` и перенести тесты функции. Проверка: `go test ./internal/service/settings/...` проходит.
- [x] 1.2 Перевести `internal/scheduler` (`Deps.Settings`, `snapshotDuePeriods`) и `admincommon.SettingsReader` на `settings.Reader`. Проверка: `go build ./...` проходит, `go test ./internal/scheduler/...` зелёный.
- [x] 1.3 Написать тесты `LoadProgressThresholds`: без ключей возвращаются значения 7/10/80/0; сохранённые значения читаются; недопустимое значение (0 дней, отрицательное отставание или допуск, порог «в плане» вне 1..100) поштучно заменяется значением по умолчанию. Затем реализовать в `internal/service/settings` константы ключей `progress_*`, тип `ProgressThresholds` и `LoadProgressThresholds`. Проверка: тесты проходят.

## 2. Перенос кэша периода в usecase/period

- [x] 2.1 Перенести `Cache` (как `PeriodCache`), `PeriodData`, `Active` (как `ActivePeriod`), `NewCache` и `StartRefreshLoop` в `internal/usecase/period`, сменив имя задачи цикла на `period_cache_refresh`. Перенести `cache_test.go` и `refreshloop_test.go` с обновлённым ожидаемым именем задачи. Проверка: перенесённые тесты проходят в `internal/usecase/period`.
- [x] 2.2 Перенести загрузчик `NewPeriodLoader`/`Deps` из `internal/usecase/healthcheckin` в `internal/usecase/period`, а `BuildTeamPath` и `Abs` — туда же неэкспортируемыми. Обновить комментарий в `internal/service/team/team.go`. Проверка: `go build ./...` проходит.
- [x] 2.3 Перевести потребителей на новые типы: `usecase/period` (`overview.go`, `progress.go`, `bulkstatus.go`), `internal/scheduler` (`Deps.HCCache`, `activePeriods`, `snapshotDuePeriods`, `SnapshotRunner`), `internal/http/server.go`, `internal/http/httpdeps` (`Build`, `PeriodUC`), а также тесты `usecase/period/overview_test.go`, `usecase/period/progress_test.go` и `api/v1/periods/overview/handler_test.go`. Проверка: `go test ./internal/usecase/period/... ./internal/scheduler/... ./internal/http/...` зелёный.

## 3. Потребители порогов переходят на новые ключи

- [x] 3.1 Перевести `admincommon.WeightTolerance` на `settings.LoadProgressThresholds`. Обновить тест обзора периода или метрик: при сохранённом `progress_weight_tolerance = 5` сумма весов 97 не считается ошибкой, а при значении по умолчанию считается. Проверка: `go test ./internal/http/handlers/api/v1/admin/... ./internal/http/handlers/api/v1/periods/...` зелёный.
- [x] 3.2 Перевести `api/v1/config` `Handler.Get` на `settings.LoadProgressThresholds`, не меняя формат ответа. Тесты `TestHandleConfigStaleDays*` и `TestHandleConfigBehindMargin*` должны засевать ключи `progress_*` вместо `health_checkin_config`; добавить аналогичный тест для `green_threshold`. Проверка: `go test ./internal/http/handlers/api/v1/config/...` зелёный.

## 4. Блок «Пороги прогресса» (бэкенд)

- [x] 4.1 Написать тесты обработчика `internal/http/handlers/api/v1/admin/settings/progressthresholds`:
  - `GET` возвращает значения по умолчанию и сохранённые значения;
  - `POST` сохраняет все четыре значения;
  - `POST` с любым недопустимым значением отвечает 400 и ничего не меняет;
  - участник без роли администратора пространства получает отказ.

  Это сценарии requirement «Пороги оценки прогресса». Проверка: до реализации тесты падают.
- [x] 4.2 Реализовать `Handler.Get`/`Handler.Post` и `RegisterRoutes` по образцу `admin/settings/feedback`: валидация всех полей до записи, `SetTenantProduct` с аудитом `tenant_setting_saved`. Зарегистрировать маршруты в `registerAdminRoutes`. Проверка: тесты из 4.1 проходят.

## 5. Удаление Health Check-in с бэкенда

- [x] 5.1 Удалить `internal/http/handlers/api/v1/healthcheckin` и `internal/http/handlers/api/v1/admin/settings/healthcheckin` с тестами. Убрать их регистрацию в `internal/http/server.go`, поле `Deps.HC` и `hcsvc.New` в `internal/http/httpdeps`, маршрут `/admin/health-checkin` в `internal/http/handlers/web/shell/handler.go`. Проверка: `go build ./...` проходит; если есть тест роутинга, он подтверждает, что `GET /api/v1/health-checkin` и `GET/POST /api/v1/admin/settings/health-checkin` отвечают 404.
- [x] 5.2 Удалить каталоги `internal/service/healthcheckin` и `internal/usecase/healthcheckin`. Проверка: `go build ./... && go vet ./...` проходит, а `rg -n "healthcheckin|hcsvc|hcuc|health_checkin|HealthCheck" --type go` не находит ничего, кроме миграции 033 и новой миграции 047.

## 6. Данные

- [x] 6.1 Добавить миграцию `migrations/047_split_health_checkin_config` (`.up.sql`/`.down.sql`) по design, решение 6: up переносит `stale_days`, `behind_margin`, `green_threshold` и `weight_tolerance` в ключи `progress_*` и удаляет `health_checkin_config`; down собирает ключ обратно и удаляет `progress_*`. Проверка на локальной БД после бэкапа строк `health_checkin_config`: после up значения в `progress_*` совпадают с исходным JSON и старого ключа нет; после down JSON восстановлен с теми же четырьмя значениями; повторный up не падает.
- [x] 6.2 Обновить комментарий о продуктовых настройках в `seed_demo.sql`: вместо `health_checkin_config` указать `progress_*`. Проверка: seed применяется на чистой БД без ошибок.

## 7. Frontend

- [x] 7.1 В `web/static/admin.js` добавить компонент `ProgressThresholdsSettingsPanel` на общих элементах, как у `FeedbackSettingsPanel`. В нём четыре числовых поля с подписями и подсказками, перенесёнными из `HealthCheckInSettingsPanel` без упоминаний сводки. Компонент читает и пишет `/api/v1/admin/settings/progress-thresholds` и показывает ошибку валидации. Добавить его карточкой в раздел `settings`. Проверка: в «Настройках» блок показывает 7/10/80/0 по умолчанию; после сохранения значение «в плане» 70 цель с прогрессом 75 % на доске зелёная; недопустимое значение показывает ошибку и не сохраняется.
- [x] 7.2 Удалить из `web/static/admin.js` раздел `health-checkin` из `ADMIN_SECTIONS`, компонент `HealthCheckInSettingsPanel` и его ветку в `App`. Проверка: в админке нет раздела «⚡ Health Check-in», остальные разделы открываются, в консоли нет ошибок.
- [x] 7.3 Удалить блок `/* ── Health Check-in ── */` (все `.hci-*` и `@keyframes hciFadeIn`) из `web/static/tracker.css`. Переписать комментарии в `web/static/tracker.js` (`sidebarProgressColor`, `GoalCard`) и `web/static/ui.js` без упоминаний Health Check-in. Проверка: `rg -n "hci|Health Check" web/static` пуст, внешний вид трекера не изменился.

## 8. Документация

- [x] 8.1 Обновить `README.md`: убрать упоминания Health Check-in и скриншот, описать блок «Пороги прогресса» в разделе «Настройки» (что каждый порог влияет и значения по умолчанию). Проверка: `rg -n "Health Check" README.md` пуст, ссылки в README не битые.
- [x] 8.2 Удалить раздел «Health Check-in» из `docs/mid-quarter.md` и строку из `docs/images/README.md`, удалить `docs/screenshots/admin-health-checkin.png` и `docs/images/health-checkin.png`. Проверка: `rg -n "health-checkin.png|admin-health-checkin" --glob '!docs/superpowers/**' --glob '!docs/old_specs/**'` пуст.

## 9. Итоговая проверка

- [x] 9.1 Полный прогон: `go build ./... && go vet ./... && go test ./...` зелёный. `rg -n "healthcheckin|health-checkin|health_checkin|hcsvc|hcuc|HealthCheck|\.hci-"` по репозиторию находит только исторические `docs/superpowers`, `docs/old_specs`, `openspec/changes/archive`, миграции 033 и 047 и файлы, которые Gortex генерирует сам (`.agents/skills`, `.opencode/skills`, `AGENTS.md`).
- [x] 9.2 Ручная проверка в запущенном приложении: доска команды (предупреждение «нет обновлений» при статусе «в работе», порог «в плане»), дерево навигации, обзор периода, метрики периодов в админке, блок «Пороги прогресса» и колокольчик уведомлений работают. `/api/v1/config` отдаёт пороги из новых ключей.
- [x] 9.3 Сверить реализацию с delta-спеками: `openspec validate remove-health-checkin`. При archive удалить каталог `openspec/specs/health-checkin` и убрать «health check-in» из Purpose `openspec/specs/progress-tracking/spec.md`. Проверка: после archive `openspec/specs` не содержит `health-checkin`, в `settings` есть «Пороги оценки прогресса», в `team-okr-board` — «Предупреждение «нет обновлений» на карточке цели».
