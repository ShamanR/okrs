# KR Notes — Design Spec

**Date:** 2026-05-29
**Branch:** keyResultNotes

---

## Цель

Заменить механизм множественных `key_result_comments` на одну заметку (1:1) к каждому KeyResult.

Заметка:

- хранит текст, автора и дату последнего обновления;
- редактируется при обновлении прогресса KR (в `KRProgressModal`);
- раскрывается в `KRRow` по клику на иконку.

---

## Схема БД

### Новая таблица `key_result_notes`

```sql
CREATE TABLE key_result_notes (
  key_result_id  BIGINT PRIMARY KEY REFERENCES key_results(id) ON DELETE CASCADE,
  text           TEXT        NOT NULL,
  author_user_id BIGINT      NOT NULL REFERENCES users(id),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

`key_result_id` как PRIMARY KEY обеспечивает инвариант 1:1 на уровне схемы.
Каскадное удаление: при удалении KR заметка удаляется автоматически.

### Миграция `021_kr_notes`

**Up:**

```sql
CREATE TABLE key_result_notes (
  key_result_id  BIGINT PRIMARY KEY REFERENCES key_results(id) ON DELETE CASCADE,
  text           TEXT        NOT NULL,
  author_user_id BIGINT      NOT NULL REFERENCES users(id),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Переносим последний комментарий каждого KR как начальную заметку
INSERT INTO key_result_notes (key_result_id, text, author_user_id, updated_at)
SELECT DISTINCT ON (key_result_id)
  key_result_id, text, author_user_id, created_at
FROM key_result_comments
ORDER BY key_result_id, created_at DESC;

DROP TABLE key_result_comments;
```

**Down:**

```sql
CREATE TABLE key_result_comments (
  id             BIGSERIAL PRIMARY KEY,
  key_result_id  BIGINT NOT NULL REFERENCES key_results(id) ON DELETE CASCADE,
  text           TEXT   NOT NULL,
  author_user_id BIGINT NOT NULL REFERENCES users(id),
  created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO key_result_comments (key_result_id, text, author_user_id, created_at)
SELECT key_result_id, text, author_user_id, updated_at
FROM key_result_notes;

DROP TABLE key_result_notes;
```

---

## Domain model

В `internal/domain/models.go`:

```go
// KeyResultNote — единственная заметка к KeyResult (1:1).
type KeyResultNote struct {
    KeyResultID int64
    Text        string
    AuthorName  string
    AuthorUDID  string
    UpdatedAt   time.Time
}
```

`KeyResult` меняется:

```go
// было
Comments []KeyResultComment

// стало
Note *KeyResultNote  // nil если заметки нет
```

`KeyResultComment` и `GoalComment` остаются (goal-комментарии не затронуты).

---

## Store layer (`internal/store/krs/krs.go`)

Удаляются:

- `AddKeyResultComment`
- `LastKeyResultComments`

Добавляются:

```go
// UpsertKeyResultNote создаёт или обновляет заметку к KR.
func (r *KRRepository) UpsertKeyResultNote(ctx context.Context, krID int64, text string, authorUserID int64) error

// BatchLoadNotes загружает заметки для набора KR одним запросом.
// Возвращает map[krID]*domain.KeyResultNote; отсутствие заметки = nil.
func (r *KRRepository) BatchLoadNotes(ctx context.Context, krIDs []int64) (map[int64]*domain.KeyResultNote, error)
```

`BatchLoadNotes` используется в goals-агрегате при загрузке OKR команды вместо N+1-вызова на каждый KR.

`ListKeyResultsByGoal` обновляется: вместо вызова `LastKeyResultComments` на каждый KR — один `BatchLoadNotes` по всем KR.

---

## API

### Новый endpoint: `POST /api/v1/krs/{krID}/note`

Upsert заметки к KR.

**Request:**

```json
{ "text": "string, non-empty" }
```

**Validation:**

- `krID` — валидный int64, KR существует и доступен текущему пользователю по scope
- `text` — обязателен, после trim не пустой

**Success 200:**

```json
{ "status": "ok" }
```

**Errors:**

- `400 VALIDATION_ERROR` — невалидный `krID` или пустой `text`
- `404 NOT_FOUND` — KR не найден или вне scope
- `403` — отсутствует CSRF token

**Idempotency:** upsert — повторный вызов с тем же текстом обновляет `updated_at` и автора.

**Инвариант:** заметку нельзя удалить через этот endpoint — `text` обязателен и непустой. Endpoint удаления заметки не предусмотрен.

**Side effects:** none на агрегаты (progress не затрагивается).

**CSRF:** required.

**Доступность по `editMode`:** разрешён в `full` и `progress_only`; в `comments_only` (`closed`) заметка доступна только для просмотра (UI не показывает поле редактирования).

### Удаляется: `POST /api/v1/krs/{krID}/comments`

Handler `HandleAddKRComment` и маршрут удаляются.

### Изменение формата в `GET /api/v1/teams/{teamID}/okrs`

KR в ответе меняет поле:

```json
// было
"comments": [{ "id": 1, "text": "...", "author_name": "Ivan", "author_udid": "...", "created_at": "..." }]

// стало
"note": {
  "text": "...",
  "author_name": "Ivan",
  "author_udid": "...",
  "updated_at": "..."
}
// или "note": null если заметки нет
```

---

## DTO (`internal/http/dto/kr.go`)

```go
type KRNote struct {
    Text       string    `json:"text"`
    AuthorName string    `json:"author_name"`
    AuthorUDID string    `json:"author_udid"`
    UpdatedAt  time.Time `json:"updated_at"`
}

type KeyResult struct {
    // ... существующие поля ...
    Note     *KRNote   `json:"note"`      // добавляется
    // Comments []KRComment удаляется
}
```

---

## Frontend (`internal/web/static/tracker.js`)

### `mapKR` — маппер

```js
// было
notes: (kr.comments || []).map(c => ({
  id: c.id, author: c.author_name, authorUdid: c.author_udid,
  date: fmtDate(c.created_at), text: c.text
}))

// стало
note: kr.note ? {
  text: kr.note.text,
  author: kr.note.author_name,
  authorUdid: kr.note.author_udid,
  date: fmtDate(kr.note.updated_at)
} : null
```

### `KRProgressModal` — редактирование заметки

- `note` state инициализируется из `kr.note?.text ?? ''`
- Под textarea показывается строка `«Автор · дата»` если `kr.note != null`
- При сохранении: `POST /api/v1/krs/{krID}/note` вызывается только если `note.trim() !== ''` И `note !== kr.note?.text` (текст изменился)
- Если textarea пуста — заметка не трогается

```text
┌─────────────────────────────────────────────┐
│  Заметка (опционально)                      │
│  ┌────────────────────────────────────────┐ │
│  │ текст заметки (pre-filled если есть)   │ │
│  └────────────────────────────────────────┘ │
│  Иван Иванов · 23 мая                       │  ← только если note != null
└─────────────────────────────────────────────┘
```

### `KRRow` — отображение заметки

- Иконка `📝` показывается только если `kr.note != null` (без счётчика)
- State: `showNote` (bool), toggle по клику на иконку
- Развёрнутый блок:

```text
┌────────────────────────────────────────┐
│ 👤 Иван Иванов  ·  23 мая              │
│ текст заметки                          │
└────────────────────────────────────────┘
```

- Старый `showNotes` (массив) заменяется на `showNote` (single bool)
- Весь рендер `(kr.notes || []).map(...)` заменяется на рендер единственной `kr.note`

---

## Permissions / Lifecycle

| editMode       | Статус                             | Заметка                       |
|----------------|------------------------------------|-------------------------------|
| `full`         | `forming`, `ready`                 | просмотр + редактирование     |
| `progress_only`| `in_progress`                      | просмотр + редактирование     |
| `comments_only`| `closed`                           | только просмотр (иконка есть) |

Серверная проверка: `POST /api/v1/krs/{krID}/note` не блокируется lifecycle на сервере (аналогично текущему состоянию других мутаций — enforcement только в UI).

---

## Тесты

- `KRRepository.UpsertKeyResultNote` — upsert создаёт/обновляет запись
- `KRRepository.BatchLoadNotes` — возвращает корректный map, отсутствующие KR → nil
- `HandleUpsertKRNote` — 200 ok, 400 на пустой text, 404 на недоступный KR
- `HandleAddKRComment` — удаляется вместе с тестами `krs/access_test.go` в части comments

---

## Definition of done

- [ ] Миграция `021_kr_notes` (up + down)
- [ ] `domain.KeyResultNote`, `KeyResult.Note *KeyResultNote`
- [ ] `KRRepository`: `UpsertKeyResultNote`, `BatchLoadNotes`; удалены `AddKeyResultComment`, `LastKeyResultComments`
- [ ] Handler `HandleUpsertKRNote`; удалён `HandleAddKRComment`
- [ ] DTO: `KRNote`, `KeyResult.Note`; удалён `Comments`
- [ ] Обновлены specs: `020-domain-model.md`, `040-api-contract.md`
- [ ] Frontend: `mapKR`, `KRProgressModal`, `KRRow`
- [ ] Тесты на store и handler
