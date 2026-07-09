# Resolve для комментариев к целям

## Проблема

Комментарий к цели по смыслу — это замечание/блокер/вопрос, который нужно
отработать и закрыть. Сейчас комментарии плоские: их нельзя пометить решёнными,
а в списке целей не видно, есть ли непроработанные замечания.

## Требования

1. Под каждым комментарием — кнопка «Отметить решённым».
2. Нажатие помечает комментарий как решённый.
3. Решённый комментарий остаётся видимым, но визуально отмечен как решённый и его
   можно «Вернуть» (снять отметку).
4. На кнопке «Комментарии» в карточке цели — счётчик общего числа комментариев и
   числа нерешённых; нерешённые визуально выделены (заметный badge).
5. (Отложено) Автору решённого комментария уходит нотификация — задокументировано
   как будущий этап, в этой итерации не реализуется.

## Модель данных

Миграция `037_comment_resolve`. В `goal_comments` добавляются два nullable-поля:

- `resolved_at TIMESTAMPTZ NULL`
- `resolved_by_user_id BIGINT NULL REFERENCES users(id)`

Комментарий считается решённым ⇔ `resolved_at IS NOT NULL`. `resolved_by_user_id`
хранит, кто отметил. Down-миграция дропает обе колонки.

`seed_demo.sql` обновляется: часть демо-комментариев переводится в resolved-состояние.

### Domain

`domain.GoalComment` расширяется:

- `ResolvedAt *time.Time`
- `ResolvedByName string`
- `ResolvedByUDID string`

## Слои

### Store (`internal/store/goals/goals.go`)

- `listGoalCommentsBatch` и `ListGoalComments` добавляют `LEFT JOIN users ru ON
  ru.id = gc.resolved_by_user_id` и селектят `gc.resolved_at, ru.display_name,
  ru.udid`. Порядок остаётся `created_at DESC` — разбивку на решённые/нерешённые
  делает фронт.
- Новый метод `SetGoalCommentResolved(ctx, scope, goalID, commentID int64,
  resolved bool, userID int64) error`. При `resolved=true` пишет
  `resolved_at = NOW(), resolved_by_user_id = userID`; при `false` — обнуляет оба
  поля. Условие обновления включает `id = commentID AND goal_id = goalID AND
  tenant_id = scope.TenantID`; если строк не затронуто — возвращает
  `domain.ErrNotFound` (или аналог, используемый в репозитории).

### Service (`internal/service/service.go`)

- В интерфейс и реализацию добавляется `SetGoalCommentResolved(...)` — passthrough
  в store.

### API (`internal/http/handlers/api/v1/goals`)

Стиль существующих POST-роутов (без PATCH):

- `POST /goals/{goalID}/comments/{commentID}/resolve`
- `POST /goals/{goalID}/comments/{commentID}/unresolve`

Оба хендлера: парсят `goalID`/`commentID`, резолвят scope, проверяют доступ к цели
(как `HandleAddGoalComment` — `GetGoal` + `CanAccessTeamFromCtx`), вызывают
`SetGoalCommentResolved` с соответствующим флагом и `UserIDFromContext`, возвращают
`{"status":"ok"}`. Несуществующий/чужой комментарий → `404 NOT_FOUND`.

DTO `GoalComment` (`internal/http/dto/goal.go`) расширяется:

- `Resolved bool` (`json:"resolved"`)
- `ResolvedByName string` (`json:"resolved_by_name"`)
- `ResolvedByUDID string` (`json:"resolved_by_udid"`)
- `ResolvedAt *time.Time` (`json:"resolved_at"`)

`response.go` заполняет эти поля из domain (`Resolved = comment.ResolvedAt != nil`).

### Frontend (`internal/web/static/tracker.js`, `tracker.css`)

- Маппинг комментария (в `mapGoal`) тянет `resolved`, `resolved_by_name`,
  `resolved_by_udid`, `resolved_at`.
- `CommentsPanel` разбивает список на нерешённые и решённые:
  - Заголовок «Комментарии» + badge «N нерешённых» (оранжевый, заметный), если
    нерешённые есть.
  - Нерешённые сверху; под каждым — кнопка «✓ Отметить решённым».
  - Решённые — секция «Решённые · N», приглушённый стиль, badge «✓ Решено» и
    метастрока «Решено · {резолвер} · {дата} · Вернуть».
  - `onResolve(commentId)` / `onUnresolve(commentId)` → POST → `onReload()`.
- Кнопка футера карточки: `💬 {всего}` плюс красный badge с числом нерешённых, если
  оно > 0 (макет: `💬 2 [1]`).

## Права и lifecycle

Ответы на обязательные вопросы спеки к новым мутациям
(`050-permissions-and-lifecycle.md`):

- Зависит ли от `team period status`? — Нет.
- Разрешена ли в `validated`? — Да (как и добавление комментариев сейчас).
- Разрешена ли в `closed`? — Да (текущий режим комментариев доступен и в `closed`).
- Проверяется ли на сервере? — Проверяется доступ к цели (scope). Отдельного
  lifecycle-enforcement нет, как и для остальных мутаций (server-side lifecycle в
  проекте пока не реализован).
- Зависит ли от будущих permissions/roles? — Resolve/вернуть доступны любому
  пользователю в scope цели (та же модель, что comment). В target-модели ролей
  попадёт под `editor`.

## Обновляемые спеки

- `020-domain-model.md` — поля `resolved_at`/`resolved_by_user_id` в `GoalComment`.
- `040-api-contract.md` — эндпоинты resolve/unresolve.

## Тесты

- Store: `SetGoalCommentResolved` (resolve → поля заполнены; unresolve → обнулены;
  чужой `commentID`/`goalID` не трогает строку) и чтение resolved-полей в списках.
- API: `POST .../resolve` и `.../unresolve` (happy path, 404 на чужой комментарий,
  403 без доступа к цели).

## Отложено

Нотификация автору о резолве — инфраструктуры нотификаций в проекте нет; отдельный
будущий этап.
