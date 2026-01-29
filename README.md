# OKR Tracker

Серверное приложение для ведения OKR нескольких команд. Реализовано на Go 1.22 с PostgreSQL и HTML-шаблонами.

## Подход: HTML-каркас + данные через API

- SSR-страницы отдают «каркас» (layout + контейнеры).
- Данные и все мутации идут через `/api/v1/...` JSON-эндпоинты.
- Фронтенд использует минимальный vanilla JS без сборщика.

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

## API v1

Базовый URL: `/api/v1`  
Content-Type: `application/json; charset=utf-8`  
Ошибки:

```json
{
  "error": {
    "code": "VALIDATION_ERROR|NOT_FOUND|CONFLICT|INTERNAL",
    "message": "Описание ошибки",
    "fields": { "field": "msg" }
  }
}
```

### Чтение

- `GET /api/v1/hierarchy`
- `GET /api/v1/teams?quarter=2024-3&org_id=123`
- `GET /api/v1/teams/{teamID}`
- `GET /api/v1/teams/{teamID}/okrs?quarter=2024-3`
- `GET /api/v1/goals/{goalID}`

### Мутации

- `POST /api/v1/krs/{id}/progress/percent`
  ```json
  { "current_value": 42.5 }
  ```
- `POST /api/v1/krs/{id}/progress/boolean`
  ```json
  { "done": true }
  ```
- `POST /api/v1/krs/{id}/progress/project`
  ```json
  { "stages": [ { "id": 1, "done": true } ] }
  ```
- `POST /api/v1/goals/{goalID}/share`
  ```json
  { "targets": [ { "team_id": 10, "weight": 50 } ] }
  ```
- `POST /api/v1/goals/{goalID}/weight`
  ```json
  { "team_id": 10, "weight": 60 }
  ```
- `POST /api/v1/goals/{goalID}/comments`
  ```json
  { "text": "Комментарий" }
  ```
- `POST /api/v1/krs/{id}/comments`
  ```json
  { "text": "Комментарий" }
  ```

## UX обновления

- На странице OKR действия целей и KR перенесены в меню «⋯», а название цели открывает модальное редактирование.
- Кнопка добавления KR находится под списком KR рядом с суммой весов.

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
- `/api/v1/teams?quarter=2024-3`

## Переменные окружения

- `DATABASE_URL` (по умолчанию `postgres://postgres:postgres@localhost:5432/okrs?sslmode=disable`)
- `PORT` (по умолчанию `8080`)
- `TZ` (по умолчанию `Asia/Bangkok`)
