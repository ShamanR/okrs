# Уведомления, фаза 1a: шина доменных событий — план реализации

> **Для агентов:** ОБЯЗАТЕЛЬНЫЙ SUB-SKILL: используй superpowers:subagent-driven-development (рекомендуется) или superpowers:executing-plans, чтобы выполнять план задача за задачей. Шаги размечены чекбоксами (`- [ ]`).

**Цель:** внутрипроцессная шина типизированных доменных событий и перевод журнала активности на неё как на первого подписчика — без изменения поведения для пользователя.

**Архитектура:** usecase публикует семантическое событие вместо ручной сборки журнальной строки. Шина маршрутизирует по типу события; каждый подписчик получает свой буферизованный канал и goroutine. Журнал подписан синхронно и сам кастует события в `activity_events`, поэтому знание о форме журнальной строки уезжает из 24 мест в одно.

**Стек:** Go 1.25 — generics для типизированной подписки, `context.WithoutCancel` для отвязки от контекста запроса.

**Спека:** [`docs/superpowers/specs/2026-08-26-notifications-design.md`](../specs/2026-08-26-notifications-design.md), разделы §4 и §5 — план опирается на неё, исполнителю нужно прочитать обе.

**Место в фазе 1.** Это первая половина фазы 1. Вторая — [`2026-08-27-notifications-1b-in-app.md`](2026-08-27-notifications-1b-in-app.md): модель данных уведомлений, резолв получателей, API, колокольчик и настройки пользователя. Разделены потому, что 1a — законченный рефакторинг с собственным критерием приёмки («лента активности пишет ровно то же, что писала»), и его стоит просмотреть и принять до того, как поверх начнёт строиться новая функциональность.

**Критерий готовности 1a:** `go test ./internal/...` зелёный, в слое usecase не осталось ни одного обращения к `service/activity`, а журнал наполняется через шину. Пользователь при этом не видит ничего нового — и это ожидаемо.

## Глобальные ограничения

- **Коммиты не делает исполнитель.** По правилу 8 `CLAUDE.md` («Dont make any git commits, i will do it myself») задачи заканчиваются прогоном тестов, а не `git commit`. Владелец репозитория коммитит сам.
- **Схема БД меняется только миграцией** (правило 2 в `specs/010-architecture-constraints.md`). Файлы `migrations/NNN_name.up.sql` + `.down.sql`.
- **Никакой бизнес-логики в handlers** (правило 1 там же).
- **Слои:** `handler → usecase → service → store`. Usecase не обращается к репозиториям; сервис работает с одним репозиторием; порт объявляется на стороне потребителя.
- **Именование:** store — множественное число, service — единственное (`store/notifications` ↔ `service/notification`). Алиас импорта `<entity>svc`.
- **Пакет на URI:** путь пакета обработчика повторяет путь URI без сегментов-параметров и дефисов.
- **Никаких N+1** (правило 9 `CLAUDE.md`). Батчевые методы помечаются комментарием `// Батчевая операция: не превращать в цикл — это N+1.`
- **Тесты с БД** поднимают контейнер через `testutil.SetupDB(t)`; без Docker тест скипается — это нормально.
- **Golden-тест маршрутов**: любое изменение набора маршрутов обновляется `go test ./internal/http -run RoutesGolden -update-routes` в той же задаче.
- **Фронтенд без сборщика**: новые модули — файл в `web/static/` + `<script type="text/babel">` в shell-шаблонах.
- **Язык:** комментарии в Go — по-английски (как в существующем коде), пользовательские строки в UI — по-русски.

---

## Карта файлов

**Создаются:**

| Файл | Ответственность |
|---|---|
| `internal/core/event/event.go` | `Kind`, `Event`, `Meta` — каркас типа события |
| `internal/core/event/events.go` | 22 структуры событий |
| `internal/platform/eventbus/bus.go` | шина: `Bus`, `Subscribe`, `SubscribeAll`, `Publish`, `Start`, `Close` |
| `internal/platform/eventbus/bus_test.go` | тесты маршрутизации, дропа, паники, дренажа |
| `internal/service/activity/journal.go` | `Handle` + `toRow`: каст события в строку журнала |
| `internal/core/event/diff.go` | `event.Diff` — отбор реально изменившихся полей |
| `internal/service/activity/journal.go` | `Handle` + `toRow`: каст события в строку журнала |
| `internal/service/servicetest/eventbus.go` | `FakeBus` для тестов usecase |

**Изменяются:**

| Файл | Что |
|---|---|
| `internal/service/activity/activity.go` | удаление `DiffFields` после переезда на `event.Diff` |
| `internal/service/servicetest/activity.go` | счётчик `BatchCalls` в fake-репозитории |
| `internal/usecase/goal/goal.go`, `comments.go`, `links.go` | 17 точек: `activity.Record` → `events.Publish` |
| `internal/usecase/keyresult/keyresult.go` | 5 точек |
| `internal/usecase/period/bulkstatus.go` | 2 точки |
| `internal/http/httpdeps/httpdeps.go` | шина параметром `Build`, подписка журнала |
| `app/app.go` | создание шины, `Start`, `Close` |

`internal/core/domain/models.go` **не меняется**: `ActivityEvent` остаётся формой журнальной строки, а не публикуемого события.

---

## Task 1: Типы доменных событий

**Файлы:**
- Создать: `internal/core/event/event.go`
- Создать: `internal/core/event/events.go`
- Тест: `internal/core/event/event_test.go`

**Интерфейсы:**
- Потребляет: `domain.TenantScope` из `okrs/internal/core/domain`.
- Производит: `event.Kind` (string), `event.Event` (интерфейс с `Kind() Kind`), `event.Meta`, 22 структуры событий и 22 константы `KindXxx`. Всё это используют задачи 2, 3, 4, 8, 9.

- [ ] **Шаг 1: Написать падающий тест**

`internal/core/event/event_test.go`:

```go
package event_test

import (
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/core/event"
)

// Каждое событие обязано возвращать свой собственный Kind: Kind — ключ
// маршрутизации в шине, и совпадение у двух типов означало бы, что подписчик
// одного получает чужие события.
func TestKindsAreUniqueAndNonEmpty(t *testing.T) {
	all := []event.Event{
		event.GoalCreated{}, event.GoalCopied{}, event.GoalMoved{}, event.GoalDeleted{},
		event.GoalFieldsChanged{}, event.GoalOwnerChanged{}, event.GoalShared{}, event.GoalUnshared{},
		event.GoalLinked{}, event.GoalUnlinked{},
		event.KRCreated{}, event.KRDeleted{}, event.KRFieldsChanged{}, event.KRProgressUpdated{},
		event.KRNoteUpdated{},
		event.StatusChanged{},
		event.CommentAdded{}, event.CommentResolved{}, event.CommentReopened{},
		event.CommentDeleted{}, event.ReplyAdded{}, event.ReplyDeleted{},
	}
	if len(all) != 22 {
		t.Fatalf("ожидалось 22 типа событий, перечислено %d", len(all))
	}
	seen := map[event.Kind]bool{}
	for _, ev := range all {
		k := ev.Kind()
		if k == "" {
			t.Errorf("%T: пустой Kind", ev)
		}
		if seen[k] {
			t.Errorf("%T: Kind %q уже занят другим типом", ev, k)
		}
		seen[k] = true
	}
}

// Meta встроена в каждое событие, поэтому Scope и ActorID читаются единообразно,
// без type switch. На это опирается и журнал, и подписчик уведомлений.
func TestMetaIsEmbedded(t *testing.T) {
	teamID := int64(7)
	ev := event.CommentAdded{
		Meta:      event.Meta{Scope: domain.TenantScope{TenantID: 3}, ActorID: 42, TeamID: &teamID},
		GoalID:    1,
		CommentID: 2,
		GoalTitle: "Цель",
		Text:      "текст",
	}
	if ev.Scope.TenantID != 3 || ev.ActorID != 42 || *ev.TeamID != 7 {
		t.Fatalf("Meta не встроена: %+v", ev)
	}
}

// KR-события несут GoalID: уведомление о правке KR адресуется как изменение цели
// и схлопывается по цели. Без этого поля подписчику пришлось бы догружать цель
// запросом на каждое событие — N+1.
func TestKREventsCarryGoalID(t *testing.T) {
	ev := event.KRProgressUpdated{GoalID: 11, KRID: 22, Before: 10, After: 60}
	if ev.GoalID == 0 {
		t.Fatal("KRProgressUpdated обязан нести GoalID")
	}
}
```

- [ ] **Шаг 2: Прогнать тест и убедиться, что он падает**

Запустить: `go test ./internal/core/event/ -run TestKinds -v`
Ожидается: FAIL с `no required module provides package okrs/internal/core/event` или `undefined: event.GoalCreated`.

- [ ] **Шаг 3: Написать каркас типа события**

`internal/core/event/event.go`:

```go
// Package event holds the OKR domain events. One struct per event type, pure data,
// no I/O — the activity journal and the notification fan-out are both subscribers,
// neither owns these types.
package event

import (
	"time"

	"okrs/internal/core/domain"
)

// Kind is the routing key of an event type. eventbus.Subscribe reads it off the
// zero value of T, so it must be a constant per type and never depend on state.
type Kind string

// Event is the marker every domain event implements.
type Event interface {
	Kind() Kind
	// Context exposes the embedded Meta, so a subscriber can read scope and actor
	// without a type switch over all 22 types. Promoted through embedding.
	Context() Meta
}

// Meta is the context every event carries. Embedded, so Scope/ActorID are readable
// without a type switch.
type Meta struct {
	Scope      domain.TenantScope
	ActorID    int64
	TeamID     *int64
	PeriodID   *int64
	OccurredAt time.Time
}

// Context returns the event's common context. Because Meta is embedded in every
// event, declaring Kind is all a new event type needs to satisfy Event.
func (m Meta) Context() Meta { return m }
```

- [ ] **Шаг 4: Написать 22 структуры событий**

`internal/core/event/events.go`:

```go
package event

const (
	KindGoalCreated       Kind = "goal_created"
	KindGoalCopied        Kind = "goal_copied"
	KindGoalMoved         Kind = "goal_moved"
	KindGoalDeleted       Kind = "goal_deleted"
	KindGoalFieldsChanged Kind = "goal_fields_changed"
	KindGoalOwnerChanged  Kind = "goal_owner_changed"
	KindGoalShared        Kind = "goal_shared"
	KindGoalUnshared      Kind = "goal_unshared"
	KindGoalLinked        Kind = "goal_linked"
	KindGoalUnlinked      Kind = "goal_unlinked"
	KindKRCreated         Kind = "kr_created"
	KindKRDeleted         Kind = "kr_deleted"
	KindKRFieldsChanged   Kind = "kr_fields_changed"
	KindKRProgressUpdated Kind = "kr_progress"
	KindKRNoteUpdated     Kind = "kr_note_updated"
	KindStatusChanged     Kind = "status_changed"
	KindCommentAdded      Kind = "comment_added"
	KindCommentResolved   Kind = "comment_resolved"
	KindCommentReopened   Kind = "comment_reopened"
	KindCommentDeleted    Kind = "comment_deleted"
	KindReplyAdded        Kind = "reply_added"
	KindReplyDeleted      Kind = "reply_deleted"
)

// --- Goal composition ---

type GoalCreated struct {
	Meta
	GoalID int64
	Title  string
}

func (GoalCreated) Kind() Kind { return KindGoalCreated }

// GoalCopied is a copy landing on a board. For notifications it reads as a created
// goal (see spec §6.1); the journal keeps its own goal_copied action.
//
// Fields mirror the existing journal payload one for one (usecase/goal/goal.go:267):
// source_goal_id, source_team_id, source_period_id, with_progress, with_comments.
type GoalCopied struct {
	Meta
	GoalID         int64
	Title          string
	SourceGoalID   int64
	SourceTeamID   int64
	SourcePeriodID int64
	WithProgress   bool
	WithComments   bool
}

func (GoalCopied) Kind() Kind { return KindGoalCopied }

// GoalMoved is a copy whose source was hard-deleted. Same payload as GoalCopied —
// today both come from one Record call that only switches the action.
type GoalMoved struct {
	Meta
	GoalID         int64
	Title          string
	SourceGoalID   int64
	SourceTeamID   int64
	SourcePeriodID int64
	WithProgress   bool
	WithComments   bool
}

func (GoalMoved) Kind() Kind { return KindGoalMoved }

type GoalDeleted struct {
	Meta
	GoalID int64
	Title  string
}

func (GoalDeleted) Kind() Kind { return KindGoalDeleted }

// GoalFieldsChanged carries only the fields that actually changed:
// field name → {before, after}.
type GoalFieldsChanged struct {
	Meta
	GoalID  int64
	Title   string
	Changed map[string][2]any
}

func (GoalFieldsChanged) Kind() Kind { return KindGoalFieldsChanged }

// GoalOwnerChanged is a change of the OWNING TEAM, not of goals.owner_udids —
// the journal payload has always been {before:{owner_team_id}, after:{owner_team_id}}.
type GoalOwnerChanged struct {
	Meta
	GoalID        int64
	Title         string
	BeforeTeamID  int64
	AfterTeamID   int64
}

func (GoalOwnerChanged) Kind() Kind { return KindGoalOwnerChanged }

type GoalShared struct {
	Meta
	GoalID            int64
	Title             string
	SharedWithTeamIDs []int64
}

func (GoalShared) Kind() Kind { return KindGoalShared }

// GoalUnshared has three call sites today, each writing a DIFFERENT payload shape:
//
//	usecase/goal/goal.go Delete      → {"declined_by_team_id": id}
//	usecase/goal/goal.go ReplaceShares → {"unshared_team_ids": [ids]}
//	usecase/goal/goal.go DeleteShare  → {"unshared_team_id": id}
//
// Exactly one field below is set, so toRow reproduces the historical shape verbatim
// and the activity feed does not change. Normalising these three into one shape is a
// separate change — it would alter stored payloads and is out of this plan's scope.
type GoalUnshared struct {
	Meta
	GoalID           int64
	Title            string
	DeclinedByTeamID int64
	UnsharedTeamID   int64
	UnsharedTeamIDs  []int64
}

func (GoalUnshared) Kind() Kind { return KindGoalUnshared }

// GoalLinked carries the parents added in one operation: ReplaceParents emits a
// single event with all added ids, not one event per link.
type GoalLinked struct {
	Meta
	ChildGoalID   int64
	Title         string
	ParentGoalIDs []int64
}

func (GoalLinked) Kind() Kind { return KindGoalLinked }

type GoalUnlinked struct {
	Meta
	ChildGoalID   int64
	Title         string
	ParentGoalIDs []int64
}

func (GoalUnlinked) Kind() Kind { return KindGoalUnlinked }

// --- Key results. Every KR event carries GoalID (spec §4.2). ---

type KRCreated struct {
	Meta
	GoalID, KRID int64
	KRTitle      string
}

func (KRCreated) Kind() Kind { return KindKRCreated }

type KRDeleted struct {
	Meta
	GoalID, KRID int64
	KRTitle      string
}

func (KRDeleted) Kind() Kind { return KindKRDeleted }

type KRFieldsChanged struct {
	Meta
	GoalID, KRID int64
	KRTitle      string
	Changed      map[string][2]any
}

func (KRFieldsChanged) Kind() Kind { return KindKRFieldsChanged }

// KRProgressUpdated carries KRKind and GoalTitle because the journal payload has
// always included them ({before,after,kind,goal_title}); the feed renders from those.
type KRProgressUpdated struct {
	Meta
	GoalID, KRID  int64
	KRTitle       string
	GoalTitle     string
	KRKind        domain.KRKind
	Before, After int
}

func (KRProgressUpdated) Kind() Kind { return KindKRProgressUpdated }

type KRNoteUpdated struct {
	Meta
	GoalID, KRID  int64
	KRTitle       string
	BeforeText    string
	AfterText     string
}

func (KRNoteUpdated) Kind() Kind { return KindKRNoteUpdated }

// --- Team period status ---

// StatusChanged. Bulk marks the mass transition done from the admin screen; the
// journal payload carries "bulk": true only in that case, so the flag must survive.
type StatusChanged struct {
	Meta
	TeamTitle string
	Before    domain.TeamPeriodStatus
	After     domain.TeamPeriodStatus
	Bulk      bool
}

func (StatusChanged) Kind() Kind { return KindStatusChanged }

// --- Discussion ---

type CommentAdded struct {
	Meta
	GoalID, CommentID int64
	GoalTitle, Text   string
}

func (CommentAdded) Kind() Kind { return KindCommentAdded }

// CommentResolved carries AuthorUserID: the task's author is the addressee of the
// my_comment_resolved notification. Filled at publish time, where the comment is
// already loaded, so the subscriber needs no join to goal_comments.
type CommentResolved struct {
	Meta
	GoalID, CommentID int64
	GoalTitle         string
	AuthorUserID      int64
}

func (CommentResolved) Kind() Kind { return KindCommentResolved }

type CommentReopened struct {
	Meta
	GoalID, CommentID int64
	GoalTitle         string
	AuthorUserID      int64
}

func (CommentReopened) Kind() Kind { return KindCommentReopened }

type CommentDeleted struct {
	Meta
	GoalID, CommentID int64
	GoalTitle         string
}

func (CommentDeleted) Kind() Kind { return KindCommentDeleted }

type ReplyAdded struct {
	Meta
	GoalID, CommentID int64
	ParentCommentID   int64
	GoalTitle, Text   string
}

func (ReplyAdded) Kind() Kind { return KindReplyAdded }

type ReplyDeleted struct {
	Meta
	GoalID, CommentID int64
	GoalTitle         string
}

func (ReplyDeleted) Kind() Kind { return KindReplyDeleted }
```

- [ ] **Шаг 5: Прогнать тесты и убедиться, что они проходят**

Запустить: `go test ./internal/core/event/ -v`
Ожидается: PASS, три теста.

- [ ] **Шаг 6: Проверить, что значения `Kind` совпадают с существующими action журнала**

Запустить: `go vet ./internal/core/event/ && rg -n 'ActivityAction = ' internal/core/domain/models.go`
Ожидается: `go vet` без замечаний; строковые литералы в списке совпадают с 22 константами `KindXxx` один в один. Это не случайность — задача 3 полагается на совпадение, чтобы `toRow` не заводил вторую таблицу соответствий.

---

## Task 2: Шина событий

**Файлы:**
- Создать: `internal/platform/eventbus/bus.go`
- Тест: `internal/platform/eventbus/bus_test.go`

**Интерфейсы:**
- Потребляет: `event.Event`, `event.Kind` из задачи 1.
- Производит:
  - `eventbus.New(logger *slog.Logger) *Bus`
  - `eventbus.Handler[T event.Event] = func(ctx context.Context, evs []T) error`
  - `eventbus.Subscribe[T event.Event](b *Bus, name string, h Handler[T], opts ...Option)`
  - `eventbus.SubscribeAll(b *Bus, name string, h Handler[event.Event], opts ...Option)`
  - `(*Bus).Publish(ctx context.Context, ev event.Event)`
  - `(*Bus).PublishBatch(ctx context.Context, evs []event.Event)`
  - `(*Bus).Start(ctx context.Context)`
  - `(*Bus).Close(timeout time.Duration) error`
  - `(*Bus).Dropped() int64` — счётчик дропнутых событий, нужен тесту и метрикам
  - Опции: `eventbus.WithBuffer(n int)`, `eventbus.WithMode(m Mode)`, `eventbus.WithTimeout(d time.Duration)`; `eventbus.Sync`, `eventbus.Async`
- Используют: задачи 4 (публикация), 3 и 9 (подписка), 14 (сборка).

- [ ] **Шаг 1: Написать падающие тесты**

`internal/platform/eventbus/bus_test.go`:

```go
package eventbus_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"okrs/internal/core/event"
	"okrs/internal/platform/eventbus"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// collector — потокобезопасный сборщик, общий для тестов ниже.
type collector struct {
	mu   sync.Mutex
	seen []event.Kind
}

func (c *collector) add(ks ...event.Kind) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seen = append(c.seen, ks...)
}

func (c *collector) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.seen)
}

// Главное свойство типизированной подписки: подписчик одного типа не должен
// видеть события другого. Без этого гранулярность подписки — фикция.
func TestSubscribeRoutesByType(t *testing.T) {
	b := eventbus.New(quietLogger())
	var comments, progress collector

	eventbus.Subscribe(b, "comments", func(_ context.Context, evs []event.CommentAdded) error {
		for range evs {
			comments.add(event.KindCommentAdded)
		}
		return nil
	}, eventbus.WithMode(eventbus.Sync))

	eventbus.Subscribe(b, "progress", func(_ context.Context, evs []event.KRProgressUpdated) error {
		for range evs {
			progress.add(event.KindKRProgressUpdated)
		}
		return nil
	}, eventbus.WithMode(eventbus.Sync))

	b.Start(context.Background())
	defer b.Close(time.Second)

	b.Publish(context.Background(), event.CommentAdded{GoalID: 1})
	b.Publish(context.Background(), event.CommentAdded{GoalID: 2})
	b.Publish(context.Background(), event.KRProgressUpdated{KRID: 3})

	if got := comments.len(); got != 2 {
		t.Errorf("подписчик комментариев: got %d, want 2", got)
	}
	if got := progress.len(); got != 1 {
		t.Errorf("подписчик прогресса: got %d, want 1", got)
	}
}

// SubscribeAll существует ради журнала: ему нужны все типы, и перечислять 22
// подписки значит забыть про 23-ю.
func TestSubscribeAllReceivesEveryType(t *testing.T) {
	b := eventbus.New(quietLogger())
	var all collector

	eventbus.SubscribeAll(b, "journal", func(_ context.Context, evs []event.Event) error {
		for _, ev := range evs {
			all.add(ev.Kind())
		}
		return nil
	}, eventbus.WithMode(eventbus.Sync))

	b.Start(context.Background())
	defer b.Close(time.Second)

	b.Publish(context.Background(), event.CommentAdded{})
	b.Publish(context.Background(), event.KRProgressUpdated{})
	b.Publish(context.Background(), event.StatusChanged{})

	if got := all.len(); got != 3 {
		t.Fatalf("wildcard-подписчик: got %d, want 3", got)
	}
}

// Паника в одном обработчике не должна убивать ни шину, ни соседей: подписчики
// изолированы, иначе один плохой слушатель роняет журнал.
func TestPanicInHandlerIsIsolated(t *testing.T) {
	b := eventbus.New(quietLogger())
	var good collector

	eventbus.Subscribe(b, "panicky", func(_ context.Context, _ []event.CommentAdded) error {
		panic("boom")
	}, eventbus.WithMode(eventbus.Sync))

	eventbus.Subscribe(b, "good", func(_ context.Context, evs []event.CommentAdded) error {
		good.add(event.KindCommentAdded)
		return nil
	}, eventbus.WithMode(eventbus.Sync))

	b.Start(context.Background())
	defer b.Close(time.Second)

	b.Publish(context.Background(), event.CommentAdded{}) // не должно паниковать наружу

	if got := good.len(); got != 1 {
		t.Fatalf("соседний подписчик не отработал: got %d, want 1", got)
	}
}

// Ошибка обработчика логируется, но не всплывает: публикация никогда не должна
// ронять пользовательскую мутацию.
func TestHandlerErrorDoesNotPropagate(t *testing.T) {
	b := eventbus.New(quietLogger())
	eventbus.Subscribe(b, "failing", func(_ context.Context, _ []event.CommentAdded) error {
		return errors.New("db down")
	}, eventbus.WithMode(eventbus.Sync))

	b.Start(context.Background())
	defer b.Close(time.Second)

	b.Publish(context.Background(), event.CommentAdded{}) // не должно паниковать и не должно блокировать
}

// Переполнение буфера роняет событие и считает дроп, но не блокирует Publish.
// Обработчик держим заблокированным, чтобы канал гарантированно переполнился.
func TestFullBufferDropsInsteadOfBlocking(t *testing.T) {
	b := eventbus.New(quietLogger())
	release := make(chan struct{})

	eventbus.Subscribe(b, "slow", func(_ context.Context, _ []event.CommentAdded) error {
		<-release
		return nil
	}, eventbus.WithMode(eventbus.Async), eventbus.WithBuffer(1))

	b.Start(context.Background())

	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			b.Publish(context.Background(), event.CommentAdded{GoalID: int64(i)})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish заблокировался на полном буфере — должен дропать")
	}

	close(release)
	_ = b.Close(time.Second)

	if b.Dropped() == 0 {
		t.Fatal("ожидались дропнутые события при переполнении буфера")
	}
}

// Async-обработчик не должен зависеть от ctx запроса: тот отменяется, как только
// handler вернул ответ, а работа подписчика продолжается уже после этого.
func TestAsyncHandlerSurvivesRequestContextCancel(t *testing.T) {
	b := eventbus.New(quietLogger())
	got := make(chan error, 1)

	eventbus.Subscribe(b, "async", func(ctx context.Context, _ []event.CommentAdded) error {
		got <- ctx.Err()
		return nil
	}, eventbus.WithMode(eventbus.Async))

	b.Start(context.Background())
	defer b.Close(time.Second)

	reqCtx, cancel := context.WithCancel(context.Background())
	b.Publish(reqCtx, event.CommentAdded{})
	cancel() // запрос завершился

	select {
	case err := <-got:
		if err != nil {
			t.Fatalf("ctx обработчика отменён вместе с запросом: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("async-обработчик не был вызван")
	}
}

// Close дренирует буферы: события, лежавшие в канале на момент остановки,
// должны быть обработаны, а не потеряны при штатном SIGTERM.
func TestCloseDrainsBuffer(t *testing.T) {
	b := eventbus.New(quietLogger())
	var seen collector
	gate := make(chan struct{})

	eventbus.Subscribe(b, "drain", func(_ context.Context, evs []event.CommentAdded) error {
		<-gate
		for range evs {
			seen.add(event.KindCommentAdded)
		}
		return nil
	}, eventbus.WithMode(eventbus.Async), eventbus.WithBuffer(16))

	b.Start(context.Background())
	for i := 0; i < 5; i++ {
		b.Publish(context.Background(), event.CommentAdded{GoalID: int64(i)})
	}
	close(gate)

	if err := b.Close(2 * time.Second); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := seen.len(); got != 5 {
		t.Fatalf("дренаж потерял события: got %d, want 5", got)
	}
}

// PublishBatch должен доходить до обработчика одним срезом, а не пятью вызовами:
// на этом держится RecordBatch в журнале (иначе N+1).
func TestPublishBatchArrivesAsOneSlice(t *testing.T) {
	b := eventbus.New(quietLogger())
	sizes := make(chan int, 4)

	eventbus.SubscribeAll(b, "batch", func(_ context.Context, evs []event.Event) error {
		sizes <- len(evs)
		return nil
	}, eventbus.WithMode(eventbus.Sync))

	b.Start(context.Background())
	defer b.Close(time.Second)

	b.PublishBatch(context.Background(), []event.Event{
		event.CommentAdded{}, event.CommentAdded{}, event.KRProgressUpdated{},
	})

	select {
	case n := <-sizes:
		if n != 3 {
			t.Fatalf("батч пришёл срезом длины %d, want 3", n)
		}
	default:
		t.Fatal("обработчик не вызван")
	}
}
```

- [ ] **Шаг 2: Прогнать тесты и убедиться, что они падают**

Запустить: `go test ./internal/platform/eventbus/ -v`
Ожидается: FAIL — пакет не существует (`no required module provides package okrs/internal/platform/eventbus`).

- [ ] **Шаг 3: Реализовать шину**

`internal/platform/eventbus/bus.go`:

```go
// Package eventbus is the in-process domain event bus. Each subscription owns a
// buffered channel and a goroutine, so a slow subscriber never blocks a fast one and
// event order is preserved per subscriber (one goroutine = FIFO).
//
// Publish never blocks and never fails: a full buffer drops the event for that one
// subscriber, logs, and bumps a counter. That is the same guarantee the activity
// journal already gave — a bookkeeping write must not break a user's mutation.
package eventbus

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"okrs/internal/core/event"
)

// Mode selects how a subscriber is invoked.
type Mode int

const (
	// Async runs the handler on the subscriber's goroutine. Default.
	Async Mode = iota
	// Sync runs the handler inline inside Publish. Used by the activity journal so a
	// mutation's event is durable before the HTTP response, exactly as before the bus.
	Sync
)

const (
	defaultBuffer  = 256
	defaultTimeout = 30 * time.Second
)

// Handler receives a batch. A single Publish delivers a slice of one; PublishBatch
// and the async drain deliver bigger slices. Always-a-slice means a batching
// subscriber cannot silently degrade into a per-event loop.
type Handler[T event.Event] func(ctx context.Context, evs []T) error

type options struct {
	buffer  int
	mode    Mode
	timeout time.Duration
}

type Option func(*options)

func WithBuffer(n int) Option        { return func(o *options) { o.buffer = n } }
func WithMode(m Mode) Option         { return func(o *options) { o.mode = m } }
func WithTimeout(d time.Duration) Option { return func(o *options) { o.timeout = d } }

// deliver is the type-erased handler stored per subscription.
type deliver func(ctx context.Context, evs []event.Event) error

// queued carries the detached context alongside the event, so an async handler runs
// with the publisher's values but not its cancellation.
type queued struct {
	ctx context.Context
	ev  event.Event
}

type subscriber struct {
	name    string
	mode    Mode
	timeout time.Duration
	ch      chan queued
	fn      deliver
}

type Bus struct {
	logger *slog.Logger

	mu      sync.RWMutex
	byKind  map[event.Kind][]*subscriber
	all     []*subscriber
	started bool

	wg      sync.WaitGroup
	dropped atomic.Int64
}

func New(logger *slog.Logger) *Bus {
	return &Bus{logger: logger, byKind: make(map[event.Kind][]*subscriber)}
}

// Dropped reports how many events were discarded because a subscriber's buffer was
// full. Non-zero in production means the buffer or the handler needs attention.
func (b *Bus) Dropped() int64 { return b.dropped.Load() }

func newSubscriber(name string, mode Mode, buffer int, timeout time.Duration, fn deliver) *subscriber {
	return &subscriber{name: name, mode: mode, timeout: timeout, ch: make(chan queued, buffer), fn: fn}
}

func resolve(opts []Option) options {
	o := options{buffer: defaultBuffer, mode: Async, timeout: defaultTimeout}
	for _, fn := range opts {
		fn(&o)
	}
	if o.buffer <= 0 {
		o.buffer = defaultBuffer
	}
	if o.timeout <= 0 {
		o.timeout = defaultTimeout
	}
	return o
}

// Subscribe registers a handler for one concrete event type. It is a package
// function, not a method: Go methods cannot take type parameters.
//
// The routing key comes from the zero value of T, so no reflection is involved.
// Must be called before Start.
func Subscribe[T event.Event](b *Bus, name string, h Handler[T], opts ...Option) {
	var zero T
	kind := zero.Kind()
	o := resolve(opts)

	fn := func(ctx context.Context, evs []event.Event) error {
		typed := make([]T, 0, len(evs))
		for _, ev := range evs {
			if t, ok := any(ev).(T); ok {
				typed = append(typed, t)
			}
		}
		if len(typed) == 0 {
			return nil
		}
		return h(ctx, typed)
	}

	s := newSubscriber(name, o.mode, o.buffer, o.timeout, fn)

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.started {
		panic("eventbus: Subscribe after Start")
	}
	b.byKind[kind] = append(b.byKind[kind], s)
}

// SubscribeAll registers a handler for every event type. It exists for the activity
// journal, which needs all 22 kinds — listing them one by one would silently miss
// the 23rd.
func SubscribeAll(b *Bus, name string, h Handler[event.Event], opts ...Option) {
	o := resolve(opts)
	s := newSubscriber(name, o.mode, o.buffer, o.timeout, func(ctx context.Context, evs []event.Event) error {
		return h(ctx, evs)
	})

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.started {
		panic("eventbus: SubscribeAll after Start")
	}
	b.all = append(b.all, s)
}

// Start launches one goroutine per async subscriber. Sync subscribers need none.
func (b *Bus) Start(context.Context) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.started {
		return
	}
	b.started = true
	for _, s := range b.subscribersLocked() {
		if s.mode != Async {
			continue
		}
		b.wg.Add(1)
		go b.run(s)
	}
}

func (b *Bus) subscribersLocked() []*subscriber {
	out := make([]*subscriber, 0, len(b.all))
	out = append(out, b.all...)
	for _, list := range b.byKind {
		out = append(out, list...)
	}
	return out
}

// run drains the subscriber's channel, coalescing whatever is already queued into a
// single batch — a burst of publishes becomes one handler call, not N.
func (b *Bus) run(s *subscriber) {
	defer b.wg.Done()
	for first := range s.ch {
		ctx, batch := first.ctx, []event.Event{first.ev}
	drain:
		for {
			select {
			case next, ok := <-s.ch:
				if !ok {
					break drain
				}
				batch = append(batch, next.ev)
			default:
				break drain
			}
		}
		b.invoke(s, ctx, batch)
	}
}

// invoke calls the handler with panic and error containment plus a timeout.
func (b *Bus) invoke(s *subscriber, ctx context.Context, evs []event.Event) {
	defer func() {
		if r := recover(); r != nil {
			b.logger.Error("eventbus: handler panicked", "subscriber", s.name, "panic", fmt.Sprint(r))
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	if err := s.fn(ctx, evs); err != nil {
		b.logger.Warn("eventbus: handler failed", "subscriber", s.name, "err", err)
	}
}

// Publish delivers one event. Never blocks, never returns an error.
func (b *Bus) Publish(ctx context.Context, ev event.Event) {
	b.PublishBatch(ctx, []event.Event{ev})
}

// PublishBatch delivers many events at once. Each subscriber receives the subset it
// is registered for, in one handler call — that is what keeps the journal's
// RecordBatch batched.
func (b *Bus) PublishBatch(ctx context.Context, evs []event.Event) {
	if len(evs) == 0 {
		return
	}
	b.mu.RLock()
	targets := make(map[*subscriber][]event.Event)
	for _, ev := range evs {
		for _, s := range b.byKind[ev.Kind()] {
			targets[s] = append(targets[s], ev)
		}
		for _, s := range b.all {
			targets[s] = append(targets[s], ev)
		}
	}
	b.mu.RUnlock()

	// The request context is detached: values (trace, logger) survive, cancellation
	// does not — an async handler outlives the request that triggered it.
	async := context.WithoutCancel(ctx)

	for s, batch := range targets {
		if s.mode == Sync {
			b.invoke(s, ctx, batch)
			continue
		}
		for _, ev := range batch {
			select {
			case s.ch <- queued{ctx: async, ev: ev}:
			default:
				b.dropped.Add(1)
				b.logger.Warn("eventbus: buffer full, event dropped",
					"subscriber", s.name, "kind", string(ev.Kind()))
			}
		}
	}
}

// Close stops accepting events and waits for the buffers to drain, so a graceful
// SIGTERM does not lose what is already queued.
func (b *Bus) Close(timeout time.Duration) error {
	b.mu.Lock()
	if !b.started {
		b.mu.Unlock()
		return nil
	}
	b.started = false
	subs := b.subscribersLocked()
	b.mu.Unlock()

	for _, s := range subs {
		if s.mode == Async {
			close(s.ch)
		}
	}

	done := make(chan struct{})
	go func() { b.wg.Wait(); close(done) }()

	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("eventbus: drain timed out after %s", timeout)
	}
}
```

- [ ] **Шаг 4: Прогнать тесты и убедиться, что они проходят**

Запустить: `go test ./internal/platform/eventbus/ -race -v`
Ожидается: PASS, восемь тестов, гонок нет. Флаг `-race` здесь обязателен — пакет целиком про конкурентность.

- [ ] **Шаг 5: Прогнать тест на гонки под нагрузкой**

Три теста выше проверяют конкурентность точечно; здесь нужен прогон, который реально нагружает планировщик, иначе гонка на `byKind`/`dropped` может не проявиться.

Запустить: `go test ./internal/platform/eventbus/ -race -count=20`
Ожидается: PASS двадцать раз подряд, без `DATA RACE`.

- [ ] **Шаг 6: Проверить, что пакет не зависит от слоёв выше**

Шина обязана оставаться чистым сеймом: зависимость на `store`, `service` или `http` означала бы, что её нельзя собрать в тесте без БД.

Запустить: `go list -deps ./internal/platform/eventbus/ | rg 'okrs/internal/(store|service|usecase|http)'`
Ожидается: пустой вывод (`rg` вернёт код 1). Единственная внутренняя зависимость пакета — `okrs/internal/core/event`.

---

## Task 3: Журнал как подписчик шины

Журнал перестаёт быть тем, что вызывают из usecase, и становится тем, что слушает шину. Вся форма строки `activity_events` съезжает в одно место — `toRow`.

**Файлы:**
- Создать: `internal/core/event/diff.go`
- Создать: `internal/service/activity/journal.go`
- Тест: `internal/service/activity/journal_test.go`
- Изменить: `internal/service/activity/activity.go` (удалить `DiffFields` после переезда)

**Интерфейсы:**
- Потребляет: `event.Event` и все 22 структуры (задача 1); `activitysvc.Service.RecordBatch` (существует).
- Производит:
  - `event.Diff(pairs map[string][2]any) map[string][2]any` — оставляет только реально изменившиеся поля
  - `(*activitysvc.Service).Handle(ctx context.Context, evs []event.Event) error` — сигнатура ровно под `eventbus.SubscribeAll`
- Используют: задачи 4 (публикация вместо Record) и 14 (подписка при сборке).

- [ ] **Шаг 1: Написать падающий тест на `event.Diff`**

`internal/core/event/diff_test.go`:

```go
package event_test

import (
	"testing"

	"okrs/internal/core/event"
)

// Diff отдаёт только изменившиеся поля: событие «поля изменились» не должно
// публиковаться, если ничего не изменилось, иначе лента заполнится пустыми записями.
func TestDiffKeepsOnlyChanged(t *testing.T) {
	got := event.Diff(map[string][2]any{
		"title":       {"Старое", "Новое"},
		"description": {"Одно и то же", "Одно и то же"},
		"weight":      {10, 20},
	})
	if len(got) != 2 {
		t.Fatalf("got %d изменившихся полей, want 2: %+v", len(got), got)
	}
	if _, ok := got["description"]; ok {
		t.Error("неизменившееся поле попало в результат")
	}
	if got["title"][0] != "Старое" || got["title"][1] != "Новое" {
		t.Errorf("пара before/after искажена: %+v", got["title"])
	}
}

func TestDiffEmptyWhenNothingChanged(t *testing.T) {
	got := event.Diff(map[string][2]any{"title": {"A", "A"}})
	if len(got) != 0 {
		t.Fatalf("want пустой результат, got %+v", got)
	}
}
```

- [ ] **Шаг 2: Прогнать тест и убедиться, что он падает**

Запустить: `go test ./internal/core/event/ -run TestDiff -v`
Ожидается: FAIL, `undefined: event.Diff`.

- [ ] **Шаг 3: Реализовать `event.Diff`**

`internal/core/event/diff.go`:

```go
package event

// Diff keeps only the entries whose before differs from after. Publishers use it to
// decide whether a *FieldsChanged event is worth emitting at all.
//
// It replaces activity.DiffFields, which returned the journal's own wire shape
// ({field: {"before": x, "after": y}}). Producing that shape is the journal's job
// now (see service/activity/journal.go), not the publisher's.
func Diff(pairs map[string][2]any) map[string][2]any {
	out := make(map[string][2]any, len(pairs))
	for field, ba := range pairs {
		if ba[0] != ba[1] {
			out[field] = ba
		}
	}
	return out
}
```

- [ ] **Шаг 4: Прогнать тест и убедиться, что он проходит**

Запустить: `go test ./internal/core/event/ -run TestDiff -v`
Ожидается: PASS, два теста.

- [ ] **Шаг 5: Написать падающий тест на `Handle` и `toRow`**

Тест табличный и покрывает все 22 типа: он же страховка от того, что при добавлении 23-го события про журнал забудут.

`internal/service/activity/journal_test.go`:

```go
package activity_test

import (
	"context"
	"reflect"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/core/event"
	activitysvc "okrs/internal/service/activity"
	"okrs/internal/service/servicetest"
)

func ptr(v int64) *int64 { return &v }

func meta(tenant int64) event.Meta {
	return event.Meta{
		Scope:    domain.TenantScope{TenantID: tenant},
		ActorID:  7,
		TeamID:   ptr(11),
		PeriodID: ptr(22),
	}
}

// Каждое из 22 событий обязано превращаться в строку журнала с той же категорией,
// action и payload, что писались до переезда на шину. Расхождение здесь — это
// молчаливая порча ленты активности.
func TestToRowCoversEveryEventType(t *testing.T) {
	cases := []struct {
		name     string
		ev       event.Event
		category domain.ActivityCategory
		action   domain.ActivityAction
		title    string
		payload  map[string]any
	}{
		{
			name:     "goal_created",
			ev:       event.GoalCreated{Meta: meta(1), GoalID: 5, Title: "Цель"},
			category: domain.ActivityComposition, action: domain.ActionGoalCreated,
			title: "Цель", payload: map[string]any{},
		},
		{
			name: "goal_copied",
			ev: event.GoalCopied{Meta: meta(1), GoalID: 5, Title: "Цель",
				SourceGoalID: 4, SourceTeamID: 3, SourcePeriodID: 2, WithProgress: true, WithComments: false},
			category: domain.ActivityComposition, action: domain.ActionGoalCopied,
			title: "Цель",
			payload: map[string]any{
				"source_goal_id": int64(4), "source_team_id": int64(3), "source_period_id": int64(2),
				"with_progress": true, "with_comments": false,
			},
		},
		{
			name: "goal_moved",
			ev: event.GoalMoved{Meta: meta(1), GoalID: 5, Title: "Цель",
				SourceGoalID: 4, SourceTeamID: 3, SourcePeriodID: 2},
			category: domain.ActivityComposition, action: domain.ActionGoalMoved,
			title: "Цель",
			payload: map[string]any{
				"source_goal_id": int64(4), "source_team_id": int64(3), "source_period_id": int64(2),
				"with_progress": false, "with_comments": false,
			},
		},
		{
			name:     "goal_deleted",
			ev:       event.GoalDeleted{Meta: meta(1), GoalID: 5, Title: "Цель"},
			category: domain.ActivityComposition, action: domain.ActionGoalDeleted,
			title: "Цель", payload: map[string]any{},
		},
		{
			name: "goal_fields_changed",
			ev: event.GoalFieldsChanged{Meta: meta(1), GoalID: 5, Title: "Новое",
				Changed: map[string][2]any{"title": {"Старое", "Новое"}}},
			category: domain.ActivityComposition, action: domain.ActionGoalFieldsChanged,
			title: "Новое",
			payload: map[string]any{"changed": map[string]any{
				"title": map[string]any{"before": "Старое", "after": "Новое"},
			}},
		},
		{
			name:     "goal_owner_changed",
			ev:       event.GoalOwnerChanged{Meta: meta(1), GoalID: 5, Title: "Цель", BeforeTeamID: 3, AfterTeamID: 9},
			category: domain.ActivityComposition, action: domain.ActionGoalOwnerChanged,
			title: "Цель",
			payload: map[string]any{
				"before": map[string]any{"owner_team_id": int64(3)},
				"after":  map[string]any{"owner_team_id": int64(9)},
			},
		},
		{
			name:     "goal_shared",
			ev:       event.GoalShared{Meta: meta(1), GoalID: 5, Title: "Цель", SharedWithTeamIDs: []int64{8, 9}},
			category: domain.ActivityComposition, action: domain.ActionGoalShared,
			title: "Цель", payload: map[string]any{"shared_with_team_ids": []int64{8, 9}},
		},
		{
			name:     "goal_unshared_declined",
			ev:       event.GoalUnshared{Meta: meta(1), GoalID: 5, Title: "Цель", DeclinedByTeamID: 8},
			category: domain.ActivityComposition, action: domain.ActionGoalUnshared,
			title: "Цель", payload: map[string]any{"declined_by_team_id": int64(8)},
		},
		{
			name:     "goal_unshared_single",
			ev:       event.GoalUnshared{Meta: meta(1), GoalID: 5, Title: "Цель", UnsharedTeamID: 8},
			category: domain.ActivityComposition, action: domain.ActionGoalUnshared,
			title: "Цель", payload: map[string]any{"unshared_team_id": int64(8)},
		},
		{
			name:     "goal_unshared_bulk",
			ev:       event.GoalUnshared{Meta: meta(1), GoalID: 5, Title: "Цель", UnsharedTeamIDs: []int64{8, 9}},
			category: domain.ActivityComposition, action: domain.ActionGoalUnshared,
			title: "Цель", payload: map[string]any{"unshared_team_ids": []int64{8, 9}},
		},
		{
			name:     "goal_linked",
			ev:       event.GoalLinked{Meta: meta(1), ChildGoalID: 5, Title: "Цель", ParentGoalIDs: []int64{1, 2}},
			category: domain.ActivityComposition, action: domain.ActionGoalLinked,
			title: "Цель", payload: map[string]any{"linked_parent_goal_ids": []int64{1, 2}},
		},
		{
			name:     "goal_unlinked",
			ev:       event.GoalUnlinked{Meta: meta(1), ChildGoalID: 5, Title: "Цель", ParentGoalIDs: []int64{1}},
			category: domain.ActivityComposition, action: domain.ActionGoalUnlinked,
			title: "Цель", payload: map[string]any{"unlinked_parent_goal_ids": []int64{1}},
		},
		{
			name:     "kr_created",
			ev:       event.KRCreated{Meta: meta(1), GoalID: 5, KRID: 6, KRTitle: "KR"},
			category: domain.ActivityComposition, action: domain.ActionKRCreated,
			title: "KR", payload: map[string]any{},
		},
		{
			name:     "kr_deleted",
			ev:       event.KRDeleted{Meta: meta(1), GoalID: 5, KRID: 6, KRTitle: "KR"},
			category: domain.ActivityComposition, action: domain.ActionKRDeleted,
			title: "KR", payload: map[string]any{},
		},
		{
			name: "kr_fields_changed",
			ev: event.KRFieldsChanged{Meta: meta(1), GoalID: 5, KRID: 6, KRTitle: "KR",
				Changed: map[string][2]any{"weight": {10, 20}}},
			category: domain.ActivityComposition, action: domain.ActionKRFieldsChanged,
			title: "KR",
			payload: map[string]any{"changed": map[string]any{
				"weight": map[string]any{"before": 10, "after": 20},
			}},
		},
		{
			name: "kr_progress",
			ev: event.KRProgressUpdated{Meta: meta(1), GoalID: 5, KRID: 6, KRTitle: "KR",
				GoalTitle: "Цель", KRKind: domain.KRKind("NUMERICAL"), Before: 10, After: 60},
			category: domain.ActivityProgress, action: domain.ActionKRProgress,
			title: "KR",
			payload: map[string]any{
				"before": map[string]any{"progress": 10},
				"after":  map[string]any{"progress": 60},
				"kind":   "NUMERICAL", "goal_title": "Цель",
			},
		},
		{
			name: "kr_note_updated",
			ev: event.KRNoteUpdated{Meta: meta(1), GoalID: 5, KRID: 6, KRTitle: "KR",
				BeforeText: "было", AfterText: "стало"},
			category: domain.ActivityDiscussion, action: domain.ActionKRNoteUpdated,
			title: "KR",
			payload: map[string]any{
				"before": map[string]any{"note": "было"},
				"after":  map[string]any{"note": "стало"},
			},
		},
		{
			name: "status_changed",
			ev: event.StatusChanged{Meta: meta(1), TeamTitle: "Команда",
				Before: domain.TeamPeriodStatusForming, After: domain.TeamPeriodStatusReady},
			category: domain.ActivityStatus, action: domain.ActionStatusChanged,
			title: "Команда",
			payload: map[string]any{
				"before": map[string]any{"status": string(domain.TeamPeriodStatusForming)},
				"after":  map[string]any{"status": string(domain.TeamPeriodStatusReady)},
			},
		},
		{
			name: "status_changed_bulk",
			ev: event.StatusChanged{Meta: meta(1), TeamTitle: "Команда", Bulk: true,
				Before: domain.TeamPeriodStatusForming, After: domain.TeamPeriodStatusReady},
			category: domain.ActivityStatus, action: domain.ActionStatusChanged,
			title: "Команда",
			payload: map[string]any{
				"before": map[string]any{"status": string(domain.TeamPeriodStatusForming)},
				"after":  map[string]any{"status": string(domain.TeamPeriodStatusReady)},
				"bulk":   true,
			},
		},
		{
			name:     "comment_added",
			ev:       event.CommentAdded{Meta: meta(1), GoalID: 5, CommentID: 6, GoalTitle: "Цель", Text: "текст"},
			category: domain.ActivityDiscussion, action: domain.ActionCommentAdded,
			title: "Цель", payload: map[string]any{"text": "текст"},
		},
		{
			name:     "comment_resolved",
			ev:       event.CommentResolved{Meta: meta(1), GoalID: 5, CommentID: 6, GoalTitle: "Цель", AuthorUserID: 3},
			category: domain.ActivityDiscussion, action: domain.ActionCommentResolved,
			title: "Цель",
			payload: map[string]any{
				"before": map[string]any{"resolved": false},
				"after":  map[string]any{"resolved": true},
			},
		},
		{
			name:     "comment_reopened",
			ev:       event.CommentReopened{Meta: meta(1), GoalID: 5, CommentID: 6, GoalTitle: "Цель", AuthorUserID: 3},
			category: domain.ActivityDiscussion, action: domain.ActionCommentReopened,
			title: "Цель",
			payload: map[string]any{
				"before": map[string]any{"resolved": true},
				"after":  map[string]any{"resolved": false},
			},
		},
		{
			name:     "comment_deleted",
			ev:       event.CommentDeleted{Meta: meta(1), GoalID: 5, CommentID: 6, GoalTitle: "Цель"},
			category: domain.ActivityDiscussion, action: domain.ActionCommentDeleted,
			title: "Цель", payload: map[string]any{},
		},
		{
			name: "reply_added",
			ev: event.ReplyAdded{Meta: meta(1), GoalID: 5, CommentID: 6, ParentCommentID: 4,
				GoalTitle: "Цель", Text: "ответ"},
			category: domain.ActivityDiscussion, action: domain.ActionReplyAdded,
			title: "Цель", payload: map[string]any{"text": "ответ"},
		},
		{
			name:     "reply_deleted",
			ev:       event.ReplyDeleted{Meta: meta(1), GoalID: 5, CommentID: 6, GoalTitle: "Цель"},
			category: domain.ActivityDiscussion, action: domain.ActionReplyDeleted,
			title: "Цель", payload: map[string]any{},
		},
	}

	// Все 22 Kind должны быть представлены — иначе тест «покрывает всё» только на словах.
	seen := map[event.Kind]bool{}
	for _, tc := range cases {
		seen[tc.ev.Kind()] = true
	}
	if len(seen) != 22 {
		t.Fatalf("таблица покрывает %d типов из 22", len(seen))
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			row, ok := activitysvc.ToRowForTest(tc.ev)
			if !ok {
				t.Fatalf("toRow отказался обрабатывать %T", tc.ev)
			}
			if row.Category != tc.category {
				t.Errorf("category: got %q, want %q", row.Category, tc.category)
			}
			if row.Action != tc.action {
				t.Errorf("action: got %q, want %q", row.Action, tc.action)
			}
			if row.EntityTitle != tc.title {
				t.Errorf("entity_title: got %q, want %q", row.EntityTitle, tc.title)
			}
			if row.ActorUserID != 7 {
				t.Errorf("actor: got %d, want 7", row.ActorUserID)
			}
			if !reflect.DeepEqual(row.Payload, tc.payload) {
				t.Errorf("payload:\n got %#v\nwant %#v", row.Payload, tc.payload)
			}
		})
	}
}

// Handle пишет батч одним RecordBatch, а не циклом Record: иначе всплеск
// публикаций превращается в N вставок (правило 9 CLAUDE.md).
func TestHandleWritesOneBatchPerTenant(t *testing.T) {
	repo := &servicetest.ActivityRepo{}
	svc := activitysvc.New(repo, nil)

	err := svc.Handle(context.Background(), []event.Event{
		event.GoalCreated{Meta: meta(1), GoalID: 1, Title: "A"},
		event.GoalCreated{Meta: meta(1), GoalID: 2, Title: "B"},
		event.GoalCreated{Meta: meta(2), GoalID: 3, Title: "C"},
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	// Два тенанта → ровно два вызова RecordBatch, три строки суммарно.
	if repo.BatchCalls != 2 {
		t.Errorf("RecordBatch вызван %d раз, want 2 (по одному на тенант)", repo.BatchCalls)
	}
	if len(repo.Events) != 3 {
		t.Errorf("записано %d строк, want 3", len(repo.Events))
	}
}
```

- [ ] **Шаг 6: Прогнать тест и убедиться, что он падает**

Запустить: `go test ./internal/service/activity/ -run TestToRow -v`
Ожидается: FAIL, `undefined: activitysvc.ToRowForTest`.

- [ ] **Шаг 7: Добавить в fake-репозиторий счётчик батчей**

`internal/service/servicetest/activity.go` — в структуру `ActivityRepo` добавить поле и инкремент, чтобы тест мог отличить один `RecordBatch` от цикла:

```go
type ActivityRepo struct {
	Events     []domain.ActivityEvent
	BatchCalls int // сколько раз вызывали RecordBatch — отличает батч от цикла Record
}

func (f *ActivityRepo) RecordBatch(_ context.Context, _ domain.TenantScope, evs []domain.ActivityEvent) error {
	f.BatchCalls++
	f.Events = append(f.Events, evs...)
	return nil
}
```

Остальные методы `ActivityRepo` не трогать.

- [ ] **Шаг 8: Реализовать `journal.go`**

`internal/service/activity/journal.go`:

```go
package activity

import (
	"context"

	"okrs/internal/core/domain"
	"okrs/internal/core/event"
)

// Handle is the bus subscriber that turns domain events into journal rows. It is the
// only place that knows the shape of an activity_events row — publishers do not.
//
// Registered synchronously (eventbus.Sync), so a mutation's event is durable before
// the HTTP response, exactly as it was when usecases called Record directly.
func (s *Service) Handle(ctx context.Context, evs []event.Event) error {
	// A batch may span tenants: one instance serves many requests concurrently.
	// Group first, then one write per tenant.
	// Батчевая операция: не превращать в цикл Record — это N+1.
	byTenant := make(map[int64][]domain.ActivityEvent)
	for _, ev := range evs {
		row, ok := toRow(ev)
		if !ok {
			continue
		}
		tenantID := ev.Context().Scope.TenantID
		byTenant[tenantID] = append(byTenant[tenantID], row)
	}
	for tenantID, rows := range byTenant {
		if err := s.RecordBatch(ctx, domain.TenantScope{TenantID: tenantID}, rows); err != nil {
			return err
		}
	}
	return nil
}

// ToRowForTest exposes toRow to the package's external test. Keeping toRow itself
// unexported preserves the rule that only this file knows the journal row shape.
func ToRowForTest(ev event.Event) (domain.ActivityEvent, bool) { return toRow(ev) }

// base fills the fields every row shares, from the event's Meta.
func base(m event.Meta, category domain.ActivityCategory, action domain.ActivityAction, title string) domain.ActivityEvent {
	return domain.ActivityEvent{
		ActorUserID: m.ActorID,
		Category:    category,
		Action:      action,
		TeamID:      m.TeamID,
		PeriodID:    m.PeriodID,
		EntityTitle: title,
		Payload:     map[string]any{},
	}
}

// changedPayload renders {field: {before, after}} in the journal's historical wire
// shape. The event carries typed pairs; the wire shape belongs to the journal.
func changedPayload(changed map[string][2]any) map[string]any {
	out := make(map[string]any, len(changed))
	for field, ba := range changed {
		out[field] = map[string]any{"before": ba[0], "after": ba[1]}
	}
	return map[string]any{"changed": out}
}

// toRow maps a domain event onto a journal row. Payloads reproduce the shapes written
// before the bus existed, byte for byte — the activity feed reads stored rows and must
// not notice the refactor.
func toRow(ev event.Event) (domain.ActivityEvent, bool) {
	switch e := ev.(type) {

	case event.GoalCreated:
		r := base(e.Meta, domain.ActivityComposition, domain.ActionGoalCreated, e.Title)
		r.GoalID = &e.GoalID
		return r, true

	case event.GoalCopied:
		r := base(e.Meta, domain.ActivityComposition, domain.ActionGoalCopied, e.Title)
		r.GoalID = &e.GoalID
		r.Payload = copyPayload(e.SourceGoalID, e.SourceTeamID, e.SourcePeriodID, e.WithProgress, e.WithComments)
		return r, true

	case event.GoalMoved:
		r := base(e.Meta, domain.ActivityComposition, domain.ActionGoalMoved, e.Title)
		r.GoalID = &e.GoalID
		r.Payload = copyPayload(e.SourceGoalID, e.SourceTeamID, e.SourcePeriodID, e.WithProgress, e.WithComments)
		return r, true

	case event.GoalDeleted:
		r := base(e.Meta, domain.ActivityComposition, domain.ActionGoalDeleted, e.Title)
		r.GoalID = &e.GoalID
		return r, true

	case event.GoalFieldsChanged:
		r := base(e.Meta, domain.ActivityComposition, domain.ActionGoalFieldsChanged, e.Title)
		r.GoalID = &e.GoalID
		r.Payload = changedPayload(e.Changed)
		return r, true

	case event.GoalOwnerChanged:
		r := base(e.Meta, domain.ActivityComposition, domain.ActionGoalOwnerChanged, e.Title)
		r.GoalID = &e.GoalID
		r.Payload = map[string]any{
			"before": map[string]any{"owner_team_id": e.BeforeTeamID},
			"after":  map[string]any{"owner_team_id": e.AfterTeamID},
		}
		return r, true

	case event.GoalShared:
		r := base(e.Meta, domain.ActivityComposition, domain.ActionGoalShared, e.Title)
		r.GoalID = &e.GoalID
		r.Payload = map[string]any{"shared_with_team_ids": e.SharedWithTeamIDs}
		return r, true

	case event.GoalUnshared:
		r := base(e.Meta, domain.ActivityComposition, domain.ActionGoalUnshared, e.Title)
		r.GoalID = &e.GoalID
		// Three historical shapes; exactly one field is set. See the type's doc comment.
		switch {
		case e.DeclinedByTeamID != 0:
			r.Payload = map[string]any{"declined_by_team_id": e.DeclinedByTeamID}
		case e.UnsharedTeamID != 0:
			r.Payload = map[string]any{"unshared_team_id": e.UnsharedTeamID}
		default:
			r.Payload = map[string]any{"unshared_team_ids": e.UnsharedTeamIDs}
		}
		return r, true

	case event.GoalLinked:
		r := base(e.Meta, domain.ActivityComposition, domain.ActionGoalLinked, e.Title)
		r.GoalID = &e.ChildGoalID
		r.Payload = map[string]any{"linked_parent_goal_ids": e.ParentGoalIDs}
		return r, true

	case event.GoalUnlinked:
		r := base(e.Meta, domain.ActivityComposition, domain.ActionGoalUnlinked, e.Title)
		r.GoalID = &e.ChildGoalID
		r.Payload = map[string]any{"unlinked_parent_goal_ids": e.ParentGoalIDs}
		return r, true

	case event.KRCreated:
		r := base(e.Meta, domain.ActivityComposition, domain.ActionKRCreated, e.KRTitle)
		r.GoalID, r.KRID = &e.GoalID, &e.KRID
		return r, true

	case event.KRDeleted:
		r := base(e.Meta, domain.ActivityComposition, domain.ActionKRDeleted, e.KRTitle)
		r.GoalID, r.KRID = &e.GoalID, &e.KRID
		return r, true

	case event.KRFieldsChanged:
		r := base(e.Meta, domain.ActivityComposition, domain.ActionKRFieldsChanged, e.KRTitle)
		r.GoalID, r.KRID = &e.GoalID, &e.KRID
		r.Payload = changedPayload(e.Changed)
		return r, true

	case event.KRProgressUpdated:
		r := base(e.Meta, domain.ActivityProgress, domain.ActionKRProgress, e.KRTitle)
		r.GoalID, r.KRID = &e.GoalID, &e.KRID
		r.Payload = map[string]any{
			"before":     map[string]any{"progress": e.Before},
			"after":      map[string]any{"progress": e.After},
			"kind":       string(e.KRKind),
			"goal_title": e.GoalTitle,
		}
		return r, true

	case event.KRNoteUpdated:
		r := base(e.Meta, domain.ActivityDiscussion, domain.ActionKRNoteUpdated, e.KRTitle)
		r.GoalID, r.KRID = &e.GoalID, &e.KRID
		r.Payload = map[string]any{
			"before": map[string]any{"note": e.BeforeText},
			"after":  map[string]any{"note": e.AfterText},
		}
		return r, true

	case event.StatusChanged:
		r := base(e.Meta, domain.ActivityStatus, domain.ActionStatusChanged, e.TeamTitle)
		r.Payload = map[string]any{
			"before": map[string]any{"status": string(e.Before)},
			"after":  map[string]any{"status": string(e.After)},
		}
		if e.Bulk {
			r.Payload["bulk"] = true
		}
		return r, true

	case event.CommentAdded:
		r := base(e.Meta, domain.ActivityDiscussion, domain.ActionCommentAdded, e.GoalTitle)
		r.GoalID, r.CommentID = &e.GoalID, &e.CommentID
		r.Payload = map[string]any{"text": e.Text}
		return r, true

	case event.CommentResolved:
		r := base(e.Meta, domain.ActivityDiscussion, domain.ActionCommentResolved, e.GoalTitle)
		r.GoalID, r.CommentID = &e.GoalID, &e.CommentID
		r.Payload = resolvedPayload(true)
		return r, true

	case event.CommentReopened:
		r := base(e.Meta, domain.ActivityDiscussion, domain.ActionCommentReopened, e.GoalTitle)
		r.GoalID, r.CommentID = &e.GoalID, &e.CommentID
		r.Payload = resolvedPayload(false)
		return r, true

	case event.CommentDeleted:
		r := base(e.Meta, domain.ActivityDiscussion, domain.ActionCommentDeleted, e.GoalTitle)
		r.GoalID, r.CommentID = &e.GoalID, &e.CommentID
		return r, true

	case event.ReplyAdded:
		r := base(e.Meta, domain.ActivityDiscussion, domain.ActionReplyAdded, e.GoalTitle)
		r.GoalID, r.CommentID = &e.GoalID, &e.CommentID
		r.Payload = map[string]any{"text": e.Text}
		return r, true

	case event.ReplyDeleted:
		r := base(e.Meta, domain.ActivityDiscussion, domain.ActionReplyDeleted, e.GoalTitle)
		r.GoalID, r.CommentID = &e.GoalID, &e.CommentID
		return r, true
	}
	return domain.ActivityEvent{}, false
}

func copyPayload(srcGoal, srcTeam, srcPeriod int64, withProgress, withComments bool) map[string]any {
	return map[string]any{
		"source_goal_id":   srcGoal,
		"source_team_id":   srcTeam,
		"source_period_id": srcPeriod,
		"with_progress":    withProgress,
		"with_comments":    withComments,
	}
}

func resolvedPayload(resolved bool) map[string]any {
	return map[string]any{
		"before": map[string]any{"resolved": !resolved},
		"after":  map[string]any{"resolved": resolved},
	}
}
```

- [ ] **Шаг 9: Прогнать тесты и убедиться, что они проходят**

Запустить: `go test ./internal/service/activity/ ./internal/core/event/ -v`
Ожидается: PASS. Табличный тест прогоняет 24 кейса (22 типа, у `goal_unshared` три варианта и у `status_changed` два) и проверяет, что покрыты все 22 `Kind`.

- [ ] **Шаг 10: Удалить осиротевший `DiffFields`**

`activitysvc.DiffFields` заменён на `event.Diff` и после задачи 4 не останется вызовов. Удалить функцию из `internal/service/activity/activity.go` вместе с её тестом, если он есть.

Запустить: `rg -n 'DiffFields' internal/ ; go build ./...`
Ожидается: `rg` ничего не находит (код 1), сборка проходит. Если `rg` что-то нашёл — эти места правятся в задаче 4, тогда удаление переносится в её конец.

---

## Task 4: Перевод 24 точек публикации на шину

Механическая, но самая рискованная задача: если хоть одна точка потеряется, из ленты активности пропадёт тип события. Страховка — существующие тесты usecase плюс `FakeBus`.

**Файлы:**
- Создать: `internal/service/servicetest/eventbus.go`
- Изменить: `internal/usecase/goal/goal.go` (11 точек), `internal/usecase/goal/comments.go` (4), `internal/usecase/goal/links.go` (2), `internal/usecase/keyresult/keyresult.go` (5), `internal/usecase/period/bulkstatus.go` (2)
- Изменить: `internal/http/httpdeps/httpdeps.go` (подать шину в usecase)
- Тест: существующие `*_test.go` рядом с каждым usecase

**Интерфейсы:**
- Потребляет: `event.*` (задача 1), `eventbus.Bus` (задача 2).
- Производит:
  - `servicetest.FakeBus` с методами `Publish(ctx, ev)`, `PublishBatch(ctx, evs)` и полем `Events []event.Event`
  - в каждом usecase-пакете — порт `Publisher` на стороне потребителя
- Используют: задачи 9 и 14.

- [ ] **Шаг 1: Написать `FakeBus`**

`internal/service/servicetest/eventbus.go`:

```go
package servicetest

import (
	"context"
	"sync"

	"okrs/internal/core/event"
)

// FakeBus records what a usecase published. Usecase tests assert on domain events
// now, not on journal rows — the journal shape is service/activity's business.
type FakeBus struct {
	mu     sync.Mutex
	Events []event.Event
}

func (f *FakeBus) Publish(_ context.Context, ev event.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Events = append(f.Events, ev)
}

func (f *FakeBus) PublishBatch(_ context.Context, evs []event.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Events = append(f.Events, evs...)
}

// KindsPublished is a convenience for assertions: the ordered list of what was sent.
func (f *FakeBus) KindsPublished() []event.Kind {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]event.Kind, 0, len(f.Events))
	for _, ev := range f.Events {
		out = append(out, ev.Kind())
	}
	return out
}
```

- [ ] **Шаг 2: Объявить порт и заменить поле в `usecase/goal`**

В `internal/usecase/goal/goal.go` заменить зависимость от журнала на узкий порт (правило «порт объявляется на стороне потребителя» из `specs/010`):

```go
// Publisher publishes domain events. *eventbus.Bus satisfies it.
// Narrow port on the consumer side: the usecase must not know that a journal,
// a notifier, or anything else is listening.
type Publisher interface {
	Publish(ctx context.Context, ev event.Event)
	PublishBatch(ctx context.Context, evs []event.Event)
}

type Deps struct {
	Goals    *goalsvc.Service
	Shares   *goalsharesvc.Service
	Links    *goallinksvc.Service
	Statuses *teamstatussvc.Service
	Periods  *periodsvc.Service
	Teams    *teamsvc.Service
	Events   Publisher
}

type UseCase struct {
	goals    *goalsvc.Service
	shares   *goalsharesvc.Service
	links    *goallinksvc.Service
	statuses *teamstatussvc.Service
	periods  *periodsvc.Service
	teams    *teamsvc.Service
	events   Publisher
}

func New(deps Deps) *UseCase {
	return &UseCase{goals: deps.Goals, shares: deps.Shares, links: deps.Links, statuses: deps.Statuses,
		periods: deps.Periods, teams: deps.Teams, events: deps.Events}
}
```

Импорт `activitysvc "okrs/internal/service/activity"` из пакета удаляется, добавляется `"okrs/internal/core/event"`.

Такой же порт объявить в `internal/usecase/keyresult` и `internal/usecase/period` (каждый — свой, дублирование интерфейса из трёх строк здесь правильнее общего пакета: иначе три usecase свяжутся через общий тип).

- [ ] **Шаг 3: Переписать 24 вызова**

Соответствие «было → стало» для каждой точки. Поля `Meta` берутся из тех же локальных переменных, что сейчас передаются в `ActivityEvent`.

| Файл:строка | Было `Action` | Стало |
|---|---|---|
| `goal/goal.go:91` | `goal_created` | `event.GoalCreated{Meta:…, GoalID: goalID, Title: input.Title}` |
| `goal/goal.go:111`, `:133` | `goal_fields_changed` | `event.GoalFieldsChanged{…, Changed: event.Diff(pairs)}`, публиковать только при `len(changed) > 0` |
| `goal/goal.go:161` | `goal_unshared` | `event.GoalUnshared{…, DeclinedByTeamID: decliner}` |
| `goal/goal.go:183` | `goal_owner_changed` | `event.GoalOwnerChanged{…, BeforeTeamID: oldOwner, AfterTeamID: newOwnerTeam}` |
| `goal/goal.go:202` | `goal_deleted` | `event.GoalDeleted{…, GoalID: goalID, Title: goal.Title}` |
| `goal/goal.go:264` | `goal_copied` / `goal_moved` | `event.GoalCopied{…}` либо `event.GoalMoved{…}` — `if p.Mode == CopyGoalModeMove`, поля из `src` и `p` |
| `goal/goal.go:358` | `goal_shared` | `event.GoalShared{…, SharedWithTeamIDs: added}` |
| `goal/goal.go:365` | `goal_unshared` | `event.GoalUnshared{…, UnsharedTeamIDs: removed}` |
| `goal/goal.go:433` | `goal_owner_changed` | `event.GoalOwnerChanged{…, BeforeTeamID: oldOwner, AfterTeamID: ownerID}` |
| `goal/goal.go:447` | `goal_unshared` | `event.GoalUnshared{…, UnsharedTeamID: shareTeam}` |
| `goal/comments.go:16` | `comment_added` | `event.CommentAdded{…, Text: text}` |
| `goal/comments.go:31` | `reply_added` | `event.ReplyAdded{…, ParentCommentID: parentID, Text: text}` |
| `goal/comments.go:53` | `comment_resolved` / `comment_reopened` | `event.CommentResolved{…}` либо `event.CommentReopened{…}` — по `resolved` |
| `goal/comments.go:86` | `comment_deleted` / `reply_deleted` | `event.CommentDeleted{…}` либо `event.ReplyDeleted{…}` |
| `goal/links.go:65` | `goal_linked` | `event.GoalLinked{…, ParentGoalIDs: added}` |
| `goal/links.go:72` | `goal_unlinked` | `event.GoalUnlinked{…, ParentGoalIDs: removed}` |
| `keyresult/keyresult.go:129` | `kr_progress` | `event.KRProgressUpdated{…, GoalTitle: g.Title, KRKind: kr.Kind, Before: beforeProg, After: afterProg}` |
| `keyresult/keyresult.go:150` | `kr_created` | `event.KRCreated{…, KRTitle: input.Title}` |
| `keyresult/keyresult.go:174` | `kr_fields_changed` | `event.KRFieldsChanged{…, Changed: event.Diff(pairs)}` |
| `keyresult/keyresult.go:194` | `kr_deleted` | `event.KRDeleted{…, KRTitle: kr.Title}` |
| `keyresult/keyresult.go:212` | `kr_note_updated` | `event.KRNoteUpdated{…, BeforeText: beforeText, AfterText: text}` |
| `period/bulkstatus.go:105` | `status_changed` ×N | накопить `[]event.Event` и один `s.events.PublishBatch(ctx, evs)`; у каждого `Bulk: true` |
| `period/bulkstatus.go:125` | `status_changed` | `event.StatusChanged{…, Before: before, After: status}` |

Образец одной замены, `comments.go:16`:

```go
func (s *UseCase) AddComment(ctx context.Context, scope domain.TenantScope, goalID int64, text string, authorUserID int64) error {
	commentID, err := s.goals.AddComment(ctx, scope, goalID, text, authorUserID)
	if err != nil {
		return err
	}
	if g, gerr := s.goals.Get(ctx, scope, goalID); gerr == nil {
		teamID, periodID := g.TeamID, g.PeriodID
		s.events.Publish(ctx, event.CommentAdded{
			Meta: event.Meta{Scope: scope, ActorID: authorUserID, TeamID: &teamID, PeriodID: &periodID},
			GoalID: goalID, CommentID: commentID, GoalTitle: g.Title, Text: text,
		})
	}
	return nil
}
```

Образец с диффом, `goal.go:111`:

```go
	if after, aerr := s.goals.Get(ctx, scope, input.ID); aerr == nil {
		changed := event.Diff(map[string][2]any{
			"title":       {before.Title, after.Title},
			"description": {before.Description, after.Description},
			"priority":    {string(before.Priority), string(after.Priority)},
			"weight":      {before.Weight, after.Weight},
		})
		if len(changed) > 0 {
			teamID, periodID := after.TeamID, after.PeriodID
			s.events.Publish(ctx, event.GoalFieldsChanged{
				Meta: event.Meta{Scope: scope, ActorID: actorUserID, TeamID: &teamID, PeriodID: &periodID},
				GoalID: after.ID, Title: after.Title, Changed: changed,
			})
		}
	}
```

**Важно про `CommentResolved.AuthorUserID`:** его надо заполнить, иначе адресное уведомление из задачи 9 будет некому отправить. В `SetCommentResolved` автор таски читается из уже загруженного комментария. Если текущий код автора не грузит, добавить чтение через `s.goals` **до** мутации — один запрос, не в цикле.

- [ ] **Шаг 4: Обновить существующие тесты usecase**

Тесты, которые сейчас проверяют журнал через `servicetest.ActivityRepo`, переводятся на `FakeBus`: вместо `repo.Events[0].Action == domain.ActionCommentAdded` — `bus.KindsPublished()` содержит `event.KindCommentAdded`.

Запустить: `rg -ln 'ActivityRepo' internal/usecase/`
Ожидается: список файлов, которые надо поправить. Каждый — заменить конструирование `Deps{… Activity: repo}` на `Deps{… Events: bus}`.

- [ ] **Шаг 5: Подать шину в сборку**

`internal/http/httpdeps/httpdeps.go`: `Build` получает шину параметром и раздаёт её в usecase.

```go
func Build(st *store.Store, grantsCache *grants.GrantsCache, hcCache *hcsvc.Cache, bus *eventbus.Bus, logger *slog.Logger) Deps {
	…
	activity := activitysvc.New(st.Activity, logger)

	// The journal is a synchronous subscriber: a mutation's event must be durable
	// before the HTTP response, exactly as it was when usecases wrote it inline.
	eventbus.SubscribeAll(bus, "activity-journal", activity.Handle, eventbus.WithMode(eventbus.Sync))

	return Deps{
		…
		GoalUC: goaluc.New(goaluc.Deps{Goals: goals, Shares: shares, Links: links, Statuses: statuses,
			Periods: periods, Teams: teams, Events: bus}),
		KrUC: kruc.New(kruc.Deps{KRs: krs, Goals: goals, Events: bus}),
		…
	}
}
```

`app/app.go` создаёт шину до `NewServer`, запускает и останавливает её рядом со `scheduler`:

```go
	bus := eventbus.New(logger)
	// Subscribers are registered inside httpdeps.Build; Start must come after it.
	defer func() { _ = bus.Close(5 * time.Second) }()
```

Точный порядок: `bus := eventbus.New(logger)` → `httpserver.NewServer(...)` (внутри которого `httpdeps.Build` регистрирует подписчиков) → `bus.Start(ctx)`. Регистрация после `Start` паникует намеренно — это ловит неправильный порядок сборки сразу, а не в проде.

- [ ] **Шаг 6: Прогнать всё и сверить набор событий**

Запустить: `go build ./... && go test ./internal/... -count=1`
Ожидается: PASS.

- [ ] **Шаг 7: Проверить, что ни одна точка не потеряна**

Запустить: `rg -c 's\.events\.Publish' internal/usecase/ && rg -n 's\.activity\.' internal/usecase/`
Ожидается: суммарно 24 публикации (`goal/goal.go` 11, `goal/comments.go` 4, `goal/links.go` 2, `keyresult/keyresult.go` 5, `period/bulkstatus.go` 2 — из них одна `PublishBatch`), и **ни одного** оставшегося `s.activity.` в слое usecase.

- [ ] **Шаг 8: Проверить журнал сквозным прогоном**

Запустить: `go test ./internal/store/activity/ ./internal/service/activity/ ./internal/usecase/... -count=1 -v`
Ожидается: PASS. Это доказательство, что журнал через шину пишет то же самое, что писал напрямую.

---

## Приёмка фазы 1a

Проверяется целиком, после всех четырёх задач.

- [ ] **Шаг 1: Полный прогон**

Запустить: `go build ./... && go vet ./... && go test ./... -count=1`
Ожидается: PASS. Тесты с БД скипаются без Docker — тогда прогон обязателен на машине с Docker, иначе задачи 3 и 4 не считаются проверенными.

- [ ] **Шаг 2: Убедиться, что слой usecase развязан с журналом**

Запустить: `rg -n 'service/activity' internal/usecase/`
Ожидается: пустой вывод. Ни один usecase больше не импортирует журнал — он публикует события и не знает, кто их слушает. Это и есть проверяемый результат фазы.

- [ ] **Шаг 3: Убедиться, что форма журнальной строки живёт в одном месте**

Запустить: `rg -ln 'domain.ActivityEvent{' internal/`
Ожидается: ровно два файла — `internal/service/activity/journal.go` (сборка строки) и `internal/store/activity/activity.go` (чтение/запись). До рефакторинга таких мест было шесть.

- [ ] **Шаг 4: Ручная проверка ленты активности**

Поднять приложение (`docker compose up`), создать цель, изменить её, добавить комментарий, отметить его решённым, обновить прогресс KR. Открыть страницу активности.
Ожидается: все пять событий в ленте с теми же формулировками и в тех же категориях, что до рефакторинга. Это то, что автотесты проверить не могут: они сверяют структуру, а не то, как лента читается человеком.

---

## Что дальше

После приёмки 1a — [`2026-08-27-notifications-1b-in-app.md`](2026-08-27-notifications-1b-in-app.md).
