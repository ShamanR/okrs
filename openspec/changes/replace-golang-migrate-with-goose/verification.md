# Результаты проверки

Проверено 2026-10-08. Goose: **v3.26.0**. Основной `go.mod` сохраняет
`go 1.25.0`; образ собран Go **1.25.14**, host-проверки — Go 1.26.8.
PostgreSQL: **15.18**, Docker: **29.8.1**.
Исходная ревизия старого релиза:
`b043172662f972542dadf874bcbae7e430467967`.

## Эталоны и SQL

- `testdata/legacy/generate.sh` извлекает SQL исходной ревизии и собирает
  отдельный временный модуль с golang-migrate v4.17.1. Дампы 22/47/48
  восстановлены с `ON_ERROR_STOP=1`; версия и отсутствие dirty проверены.
- Сохранены схема 48 и независимые продолжения тех же входов старым runner
  до 48. Fixtures содержат legacy KR с checkpoint, BOOLEAN, KR без meta-строки,
  уведомление с JSON payload и пользовательскую настройку.
- Проверены ровно 48 последовательных файлов и точное совпадение всех 96
  SQL-тел с исходной ревизией. Постоянный тест:
  `TestSQLBodiesMatchHistoricalRevision`.
- В отдельном временном модуле парсер goose v3.26.0
  `internal/sqlparser.ParseAllFromFS` успешно разобрал обе секции всех файлов.
- Goose CLI v3.26.0 создал `00049_next_change.sql` через `-s create` в
  временной копии каталога. В production-каталоге версии 49 нет.

## Runner и сохранность

`internal/platform/migrations/migrations_test.go` проверяет:

- Чистую установку, повторный старт, целевые версии и `48 → 47 → 48`,
  одинаковую целевую версию, сохранение владения вызывающим пулом.
- Эквивалентность schema-only pg_dump независимому старому эталону:
  столбцы, defaults, constraints, indexes, определения и ownership sequences.
- Полный импорт 0–N, замороженную старую историю, отсутствие повторных
  преобразований и точное сохранение строк/sequence states на версии 48.
- Результаты обновления 22/47 через сравнение полного прикладного дампа
  с независимым продолжением старого runner. Для 22 нормализуются только
  `tenants.created_at` и `memberships.created_at`, создаваемые миграциями
  027/028. Исходные значения времени не нормализуются.
- Отказ без изменения дампа при dirty, будущей/неизвестной версии,
  нескольких старых записях, неверной структуре, пустой/неполной goose-истории,
  прикладных объектах без истории. Пустая старая история и начальная goose 0
  без прикладных объектов принимаются; освобождение lock проверено.
- Откат bootstrap до commit через SQL event trigger; разрыв реальной
  PostgreSQL-сессии после commit импорта до вызовов Provider; ошибку
  транзакционной миграции после импорта. Повторный запуск во всех случаях
  успешен, ошибочная версия не зарегистрирована, частичный SQL откатан.
- Конкуренцию независимых пулов на новой и прежней БД, непрерывный lock
  во время SQL, взаимное ожидание со старым драйвером в обоих порядках,
  отмену контекста, timeout и схему `"Fixture Schema"`.
- Совпадение lock keys с настоящим v4.17.1 для разных БД/схем и
  `новый runner → старый runner → новый runner`.

Пример запуска:

```sh
MIGRATIONS_TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable' \
LEGACY_MIGRATION_RUNNER=/tmp/okrs-legacy-runner \
go test -v ./internal/platform/migrations
```

`LEGACY_RUNNER_OUTPUT=/tmp/okrs-legacy-runner` при генерации fixtures сохраняет
проверочный старый бинарник вне основного модуля. Блокирующая синтетическая
версия 49 существует только в in-memory FS интеграционного теста.

## Сервер и старый релиз

`cmd/server/migrations_test.go`: успешное применение/no-op; ошибки подключения,
прав и миграции; код 1; отсутствие готовности; единственная запись ERROR,
обязательные поля события migration, отсутствие DSN и пароля.

Предыдущий настоящий сервер собран из `git archive` исходной ревизии в
отдельном `/tmp` checkout. Скрипт `testdata/release-smoke.py` подтвердил:

1. Новый сервер → предыдущий сервер → новый сервер: все объявили готовность,
   полный прикладной дамп и sequence states после каждого запуска совпали.
2. Совместно работающие новый и предыдущий серверы на исходной старой 48:
   оба готовы, данные сохранены.
3. Новая БД и отдельная новая БД с `-seed`: успешный старт, версия 48.

Переменные сценария: `MIGRATIONS_TEST_DATABASE_URL`, `CURRENT_SERVER`,
`PREVIOUS_SERVER`, `PREVIOUS_ROOT`. Предыдущий сервер запускается именно
из исторического checkout со старым SQL.

## Общая интеграция

```sh
MIGRATIONS_TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable' \
LEGACY_MIGRATION_RUNNER=/tmp/okrs-legacy-runner \
DOCKER_HOST=unix:///run/user/1000/docker.sock \
TESTCONTAINERS_RYUK_DISABLED=true \
go test -p 4 -json ./...

go vet ./...
go mod tidy
openspec validate replace-golang-migrate-with-goose --strict
```

- Полный прогон: код 0, 187 пакетов, 1377 тестов/подтестов,
  1372 PASS и 5 SKIP из-за коллизий host-портов rootless Docker.
  Все пять существующих тестов затем выполнены последовательно: 5 PASS,
  0 SKIP. Новые DB-тесты выполнены без пропусков. Дополнительные финальные
  проверки разрыва сессии, quoted schema и SQL-тел также прошли.
- `go vet ./...`, `go mod tidy` и OpenSpec strict завершились успешно.
  Старой библиотеки нет в основном go.mod/go.sum, production и обычных helpers;
  исторический reference runner остаётся только в `testdata`.
- JSON результаты текущего запуска сохранены в `/tmp/okrs-goose-final-tests.json`
  и `/tmp/okrs-goose-retried-tests.json`.

## Docker

Образ `okrs:goose-validation` собран с исходными этапами Dockerfile.
Для ускорения сетевых загрузок временный Dockerfile добавляет перед
`go mod download` только COPY локального проверенного module-cache в builder:

```sh
docker build -f /tmp/okrs-goose-cached.Dockerfile \
  --build-context cached_modules=/tmp/okrs-goose-docker-module-cache \
  -t okrs:goose-validation .
```

Runtime-stage содержит сервер, web и `/app/migrations`. Обычный Dockerfile
сохраняет тот же способ сборки и доставки. Кода golang-migrate в образе нет:
от него взята только формула ключа advisory lock, источник указан в
комментарии к `legacyLockID`.

```sh
MIGRATIONS_TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable' \
POSTGRES_CONTAINER=<изолированный-PG15-контейнер> \
python3 internal/platform/migrations/testdata/docker-smoke.py
```

Smoke сценарий прошёл: новая БД, повторный старт, перенос старой 48,
повторный старт перенесённой БД, точное сохранение данных/sequence states.
`migration` предшествует `app_ready`, ERROR отсутствуют. `-seed` создаёт
демонстрационные goals, прикладная схема совпадает с новой БД без seed.

## Итоговый scope

49 исторических миграций сохранены без изменений SQL. Production-версий 50+
нет. UI, доменная логика и seed demo не изменены. Изменения дополнительных
интеграционных fixtures ограничены выбором образа PostgreSQL 15. README и
артефакты описывают фактическое управление одной сессией и проверенные
ограничения выпуска/возврата.

## Повторная проверка после rebase на upstream

Проверено 2026-10-08. Change перенесён на `upstream/main`
`58fbf14f93a98c32fb33d8bb99d96dfab20de531`, где добавлена миграция
`049_kr_current_value_null` в прежнем формате. Разделы выше описывают
первоначальную проверку на ревизии `b043172…` с последней версией 48.

- `049` объединена в один goose-файл; тела Up/Down побайтно совпадают с
  исходной ревизией (`TestSQLBodiesMatchHistoricalRevision`).
- Исходная ревизия эталонов и тестов — `58fbf14…`, последняя версия прежнего
  мигратора — 49 (`lastLegacyVersion`). Fixtures перегенерированы
  `generate.sh` настоящим golang-migrate v4.17.1 на PostgreSQL 15.18:
  входы 22/47/48/49, продолжения до 49, `schema49.sql`. Прикладная схема 49
  совпала со схемой 48: миграция 049 меняет только данные.
- Тесты больше не фиксируют число файлов: сравнение с эталонами идёт до
  `lastLegacyVersion`, после чего проверяется `Up` до последней версии.
  Проверки через git пропускаются, если ревизии нет в checkout.
- `MIGRATIONS_TEST_DATABASE_URL` + `LEGACY_MIGRATION_RUNNER`:
  `go test ./internal/platform/migrations ./cmd/server` — все тесты PASS,
  без пропусков. `go test ./...` — PASS, включая тесты upstream на 049
  (`TestMigration049*`) через goose.
- `docker-smoke.py` и `release-smoke.py` переведены на вычисляемые версии,
  но после rebase повторно не запускались.
