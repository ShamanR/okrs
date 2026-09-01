# Чек-ин по KR: одно событие, одно уведомление — план

> **Для агентов:** выполнять задача за задачей. Шаги размечены чекбоксами.

**Цель:** одно действие пользователя «отметиться по ключевому результату» порождает одно событие и одно уведомление, текст которого зависит от того, что именно изменилось: прогресс, health-статус, заметка.

**Почему.** Сегодня форма чек-ина в трекере шлёт ДВА запроса (`/progress/{kind}` и `/note`, `web/static/tracker.js:681-689`), поэтому одно действие даёт два события и два уведомления. Смена health-статуса не порождает события вообще (`internal/http/handlers/api/v1/krs/progress/*/handler.go` дёргает `krs.UpdateHealthStatus` мимо шины). Ни одно событие не несёт health-статус. `KRNoteUpdated` не имеет ветки в рендерере уведомлений и падает в заглушку «Обновление по цели» — из-за чего обновление заметки и обновление статуса выглядят одинаково. Дефект нашёл пользователь вручную.

**Решение (выбрано пользователем, вариант B):** чек-ин становится одной операцией — один HTTP-запрос, один usecase-метод, одно событие `KRCheckedIn`. Каждый подписчик шины решает сам: журнал разворачивает событие в свои строки (прогресс — в ленту прогресса, заметка — в обсуждение, как и сейчас), уведомления рисуют один текст.

## Правила текста уведомления (утверждены пользователем)

`{icon}` — из `KR_HEALTH_ICON` (`web/static/tracker.js:245`): `not_started: ○`, `on_track: ●`, `at_risk: ▲`, `done: ✓`.
`{status}` — из `KR_HEALTH_LABEL` (`tracker.js:244`), **английские**: `Not Started`, `On Track`, `At Risk`, `Closed`.

| Что изменилось | Тело уведомления |
|---|---|
| только прогресс | `{icon}{status} {from}% → {to}%` |
| прогресс и статус | `{icon}{from_status} → {icon}{to_status} {from}% → {to}%` |
| только заметка | `{icon}{status} — заметка: {note}` |
| заметка и статус | `{icon}{from_status} → {icon}{to_status} — заметка: {note}` |
| только статус | `{icon}{from_status} → {icon}{to_status}` |

**Текст заметки пишется ТОЛЬКО когда заметка изменилась, а прогресс — нет.** Если изменилось и то, и другое, заметка в текст не попадает: она не главное в этом событии, и уведомление не должно ею перегружаться.

Правило 5 приведено к симметрии с правилом 2 по решению пользователя: переход статуса заменяет префикс, а не добавляется к нему.

## Глобальные ограничения

- **Коммиты запрещены** (правило 8 `CLAUDE.md` — пользователь коммитит сам). Все файлы дерева сейчас застейджены, `HEAD` — `6685a7d`; индекс не трогать.
- Комментарии в production-коде Go — английский; в `*_test.go` и JS — русский.
- Слои: `handler → usecase → service → store`; порт объявляется на стороне потребителя.
- Схема БД — только миграциями (здесь миграции не нужны).
- `go test ./...` сейчас зелёный (157 пакетов) и обязан таким остаться.
- `gofmt -l internal/` НЕ запускать (дерево в CRLF, отметит ~450 пре-существующих файлов) — точечно.
- Диффы смотреть `git diff HEAD`, а не `git diff`: всё застейджено, и без `HEAD` покажется пустота.
- Проверять парсинг JS вендоренным Babel: `node -e "const B=require('./web/static/vendor/babel.min.js');const fs=require('fs');B.transform(fs.readFileSync('web/static/tracker.js','utf8'),{presets:['react']});console.log('ok')"`

## Чего в этом изменении НЕТ

- Журнал активности сохраняет свои категории и payload побайтово. Единственное изменение вывода: пересохранение формы БЕЗ фактического изменения прогресса раньше писало строку `kr_progress` с `before == after`, теперь не пишет ничего — это следствие правила «ничего не изменилось, события нет» и улучшение, но заявлять «вывод не меняется вовсе» было неточно. Изменение health-статуса journal-строки по-прежнему не порождает — отдельное решение, если понадобится.
- Схлопывание уведомлений (`CoalesceKey`) не трогаем.

---

## Task 1: Событие чек-ина, usecase, журнал, мост уведомлений и текст

**Ruling: задачи 1 и 3 исходного плана слиты, добавлен пропущенный файл.** Исполнитель остановился со статусом BLOCKED и был прав дважды. Первое: удаление `KRProgressUpdated`/`KRNoteUpdated` в задаче 1 ломает сборку до задачи 3, потому что на них ссылается `internal/render/notify/notify.go` — то есть между задачами репозиторий не собирался бы, вопреки требованию «после каждой задачи всё зелёное». Второе: `internal/usecase/notification/mapping.go` — мост «событие → тип уведомления, якорь, payload» — не был упомянут ни в одной задаче, а без его правки чек-ин перестал бы порождать уведомления вовсе. Замена двух типов событий на один — атомарный рефакторинг через событие, usecase, журнал, мост и рендерер; дробить его нельзя.

**Попутная находка, объясняющая исходный дефект.** В `mapping.go` есть ветка для `KRProgressUpdated` и НЕТ для `KRNoteUpdated`: обновление заметки никогда не доходило до уведомлений. Поскольку форма чек-ина всегда шлёт и запрос прогресса, и заметка, и смена статуса давали одно и то же «обновил прогресс 50% → 50%» — ровно то, что заметил пользователь.

**Файлы:**
- Изменить: `internal/core/event/events.go`, `internal/usecase/keyresult/keyresult.go`, `internal/service/activity/journal.go`, `internal/usecase/notification/mapping.go`, `internal/render/notify/notify.go`
- Тесты: соответствующие `*_test.go` во всех пяти пакетах

**Мост уведомлений.** `internal/usecase/notification/mapping.go` (120 строк) решает, какое событие становится уведомлением какого типа, к чему привязывается и какой payload несёт. Три ветки `KRProgressUpdated` заменяются на `KRCheckedIn`; payload обязан донести до рендерера всё, что нужно пяти правилам: прогресс before/after, health before/after, заметку before/after. Ветки для `KRNoteUpdated` там нет — заметка никогда не доходила до уведомлений, и это часть чинимого дефекта.

**Текст уведомления** — по таблице правил в шапке плана, дословно. Иконки и подписи статусов объявить в одном месте с комментарием, что источник — `KR_HEALTH_ICON`/`KR_HEALTH_LABEL` в `web/static/tracker.js:244-245`, и что расхождение будет заметно пользователю; сослаться на §6.12 реестра техдолга, где этот класс дублирования между Go и JS уже описан. Заголовок: `{actor} отметился по ключевому результату` (сегодня «обновил прогресс», что неверно, когда прогресс не менялся).

**Событие.** Заменяет `KRProgressUpdated` и `KRNoteUpdated` — после этой задачи их никто не публикует, поэтому оба типа и обе константы `Kind` удаляются, а `AllKinds()` пополняется `KindKRCheckedIn`. Константы `domain.ActionKRProgress` и `domain.ActionKRNoteUpdated` остаются: на них ссылаются уже записанные строки журнала.

```go
// KRCheckedIn is one check-in on a key result: the user submits progress, health
// status and note together, and this is the single event that operation produces.
// Before/after pairs are always populated — "changed" is inequality, not a flag —
// so a consumer decides for itself what mattered: the journal splits it into a
// progress row and a discussion row, the notifier renders one line.
type KRCheckedIn struct {
	Meta
	GoalID, KRID   int64
	KRTitle        string
	GoalTitle      string
	KRKind         domain.KRKind
	ProgressBefore int
	ProgressAfter  int
	HealthBefore   domain.KRHealthStatus
	HealthAfter    domain.KRHealthStatus
	NoteBefore     string
	NoteAfter      string
}

func (KRCheckedIn) Kind() Kind { return KindKRCheckedIn }
```

**Usecase.** Три метода прогресса (`UpdateProgressNumerical`, `UpdateProgressBoolean`, `UpdateProgressProject`) и `UpsertNote` перестают публиковать по событию каждый. Вместо этого появляется одна операция:

```go
// CheckInInput carries what one check-in submits. A nil field was not part of this
// submission and must be left as it is — that is what lets the note endpoint and
// the progress endpoints share one operation without inventing a second event.
type CheckInInput struct {
	Numerical *float64
	Boolean   *bool
	Project   []ProjectStageUpdate
	Health    *domain.KRHealthStatus
	Note      *string
}
```

`CheckIn(ctx, scope, krID int64, in CheckInInput, actorUserID int64) error`:
1. читает KR и его заметку ДО изменений — нужны `before` для всех трёх величин;
2. применяет прогресс (по тому из трёх полей, что непустое), health-статус и заметку;
3. публикует ровно одно `KRCheckedIn` с before/after по всем трём;
4. **не публикует ничего**, если ни одна из трёх величин фактически не изменилась — сегодня прогресс публикуется безусловно, и смена одного лишь статуса даёт «обновил прогресс 50% → 50%». Это часть чинимого дефекта.

Существующие три метода прогресса сохраняются как тонкие обёртки над `CheckIn` (их зовут другие места), либо удаляются, если вызывающих не осталось — проверить `rg` и сделать по факту, отметив в отчёте.

**Журнал.** `toRow(ev) (domain.ActivityEvent, bool)` становится `toRows(ev) []domain.ActivityEvent`; `Handle` (`journal.go:22`) добавляет `append(byTenant[tenantID], rows...)`; `ToRowForTest` заменяется на `ToRowsForTest`. Для `KRCheckedIn` возвращается:
- строка `ActivityProgress`/`ActionKRProgress` с прежним payload (`before/after.progress`, `kind`, `goal_title`) — если `ProgressBefore != ProgressAfter`;
- строка `ActivityDiscussion`/`ActionKRNoteUpdated` с прежним payload (`before/after.note`) — если `NoteBefore != NoteAfter`;
- обе, если изменилось и то, и другое.
Остальные ветки `toRows` возвращают слайс из одной строки — вывод журнала для них не меняется.

- [ ] **Шаг 1.** Написать падающие тесты: событие в `AllKinds()`; `CheckIn` публикует одно событие с корректными before/after по трём величинам; `CheckIn` не публикует ничего при отсутствии изменений; `toRows` даёт одну строку при изменении только прогресса, одну при изменении только заметки, две при изменении обоих, ноль при изменении только статуса.
- [ ] **Шаг 2.** Прогнать, убедиться что падают.
- [ ] **Шаг 3.** Реализовать событие, `CheckIn`, `toRows`.
- [ ] **Шаг 4.** `go test ./internal/core/event/... ./internal/usecase/keyresult/... ./internal/service/activity/... -count=1`
- [ ] **Шаг 5.** Тесты рендерера: по одному на каждое из пяти правил, плюс неизвестный статус не роняет рендер и пустая заметка не даёт висящего «— заметка:».
- [ ] **Шаг 6.** Мутация 1: убрать из `CheckIn` условие «ничего не изменилось — не публиковать»; падать обязан соответствующий тест. Мутация 2: поменять местами ветки «только прогресс» и «прогресс и статус» в рендерере; падать обязаны ровно два соответствующих теста. Обе через `go test -overlay`, дерево не трогать, сборка обязана проходить.
- [ ] **Шаг 7.** `go build ./... && go vet ./... && go test ./... -count=1` — 157 пакетов, ноль падений.

---

## Task 2: Один запрос на чек-ин

**Файлы:**
- Изменить: три хендлера `internal/http/handlers/api/v1/krs/progress/{numerical,boolean,project}/handler.go`, хендлер заметки, `web/static/tracker.js`
- Обновить: `internal/http/testdata/routes.golden` — **только если менялся состав маршрутов** (не должен)

**Контракт.** Три эндпоинта прогресса дополнительно принимают необязательное поле `note`:
```json
{ "current_value": 42, "health_status": "on_track", "note": "ждём поставку" }
```
`note` отсутствует — заметка не трогается; `note` присутствует (в том числе пустой строкой) — заметка выставляется в это значение. Эндпоинт заметки сохраняется для правки заметки вне чек-ина и тоже идёт через `CheckIn`.

Хендлеры перестают дёргать `krs.UpdateHealthStatus` напрямую: статус уходит в `CheckIn` вместе с остальным, иначе смена статуса снова окажется вне события.

**Фронтенд.** `web/static/tracker.js:681-689` — вместо двух запросов один: заметка едет в теле запроса прогресса. Ветку отдельного `POST /note` оставить только для пути, где заметка правится без чек-ина, если такой в интерфейсе есть; если нет — убрать.

- [ ] **Шаг 1.** Написать падающие тесты хендлеров: `note` в теле доезжает до usecase; отсутствие `note` не трогает заметку; `health_status` доезжает; невалидный `health_status` даёт 400 как и раньше.
- [ ] **Шаг 2.** Прогнать, убедиться что падают.
- [ ] **Шаг 3.** Реализовать.
- [ ] **Шаг 4.** Правка `tracker.js`; проверить парсинг Babel.
- [ ] **Шаг 5.** `go test ./internal/http/... -count=1`; `git diff HEAD --stat internal/http/testdata/routes.golden` — ожидается пусто.

---

## Task 3: Спеки и полный прогон

**Файлы:** `specs/040-api-contract.md` (поле `note` в трёх эндпоинтах прогресса), `specs/020-domain-model.md` (событие чек-ина, если события там описаны — проверить), `docs/superpowers/plans/2026-08-27-notifications-tech-debt.md` (закрыть, если что-то из §6 закрылось; добавить, если что-то отложено).

- [ ] **Шаг 1.** Правки спек. Спеки — на русском, файлы в CRLF, целиком не переписывать.
- [ ] **Шаг 2.** `go build ./... && go vet ./... && go test ./... -count=1` — 157 пакетов, ноль падений.
- [ ] **Шаг 3.** `go test ./internal/http -run 'TestSpecRouteTableMatchesRouter|TestRoutesGolden' -count=1`
