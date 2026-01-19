# OKR Tracker

Серверное приложение для ведения OKR нескольких команд. Реализовано на Go 1.22 с PostgreSQL и HTML-шаблонами.

## Запуск

### Через Docker Compose (Postgres локально)

```bash
docker compose up -d
export DATABASE_URL=postgres://postgres:postgres@localhost:5432/okrs?sslmode=disable
export TZ=Asia/Bangkok
export PORT=8080

go run ./cmd/server
```

### С демо-данными

```bash
DATABASE_URL=postgres://postgres:postgres@localhost:5432/okrs?sslmode=disable \
TZ=Asia/Bangkok PORT=8080 \
go run ./cmd/server --seed
```

Откройте [http://localhost:8080/teams](http://localhost:8080/teams).

## Тесты

```bash
go test ./...
```

## Прогресс вычисляется

- **Goal.progress**: среднее по KR с учётом их весов (если суммарный вес = 0 → 0%).
- **Quarter.progress**: среднее по целям с учётом их весов (если суммарный вес = 0 → 0%).
- **PROJECT KR**: сумма весов выполненных этапов.
- **PERCENT KR**: линейная интерполяция между start/target (или по checkpoints).
- **BOOLEAN KR**: 100% если done, иначе 0%.

## Примеры URL

- `/teams` — список команд и фильтр квартала.
- `/teams/{teamID}/okr?year=2024&quarter=3`
- `/goals/{goalID}`
- `/api/teams?year=2024&quarter=3`

## Переменные окружения

- `DATABASE_URL` (по умолчанию `postgres://postgres:postgres@localhost:5432/okrs?sslmode=disable`)
- `PORT` (по умолчанию `8080`)
- `TZ` (по умолчанию `Asia/Bangkok`)
