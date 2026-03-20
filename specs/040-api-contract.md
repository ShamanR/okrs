# API Contract

## Общий принцип

`/api/v1` — канонический интерфейс для данных и мутаций.

SSR-страницы должны опираться на те же правила, что и API.

Ошибки возвращаются в нормализованном виде:

- `VALIDATION_ERROR`
- `NOT_FOUND`
- `CONFLICT`
- `INTERNAL`

## Read endpoints

Обязательные read endpoints:

- `GET /api/v1/hierarchy`
- `GET /api/v1/periods`
- `GET /api/v1/teams`
- `GET /api/v1/teams/{teamID}`
- `GET /api/v1/teams/{teamID}/okrs`
- `GET /api/v1/goals/{goalID}`

## Write endpoints

Обязательные write endpoints:

- share goal
- update goal weight
- add goal comment
- update goal
- create KR
- move goal up / down
- update KR progress
- add KR comment
- update KR
- move KR up / down
- update team status

## Требования к новым endpoint’ам

Для любого нового endpoint в spec обязательно фиксировать:

1. method + path;
2. request format;
3. validation rules;
4. success response;
5. error cases;
6. idempotency expectation;
7. side effects on aggregates.

## Acceptance criteria для API

- каждый handler имеет явную валидацию входа;
- ошибки согласованы по shape;
- изменение доменного правила сопровождается тестами;
- нет дублирования business rule между SSR handler и API handler.
