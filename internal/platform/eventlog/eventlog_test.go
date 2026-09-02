package eventlog

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/core/event"
	"okrs/internal/platform/eventbus"
	"okrs/internal/platform/logging"
)

func meta() event.Meta {
	team := int64(7)
	period := int64(9)
	return event.Meta{
		Scope:      domain.TenantScope{TenantID: 1},
		ActorID:    2,
		TeamID:     &team,
		PeriodID:   &period,
		OccurredAt: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
	}
}

// samples — по одному образцу на каждый тип доменного события, с ЗАПОЛНЕННЫМИ
// текстовыми полями: гард-тест ниже проверяет, что ни один из этих текстов не
// доходит до лога.
//
// Полнота списка проверяется отдельно, сверкой с event.AllKinds(): 22-й тип
// события, добавленный без образца здесь, роняет тест, а не остаётся незамеченным.
func samples() []event.Event {
	const text = "СЕКРЕТНЫЙ-ПОЛЬЗОВАТЕЛЬСКИЙ-ТЕКСТ"
	m := meta()
	return []event.Event{
		event.GoalCreated{Meta: m, GoalID: 10, Title: text},
		event.GoalCopied{Meta: m, GoalID: 10, Title: text, SourceGoalID: 11, WithProgress: true},
		event.GoalMoved{Meta: m, GoalID: 10, Title: text, SourceGoalID: 11},
		event.GoalDeleted{Meta: m, GoalID: 10, Title: text},
		event.GoalFieldsChanged{Meta: m, GoalID: 10, Title: text,
			Changed: map[string][2]any{"title": {text, text}, "weight": {1, 2}}},
		event.GoalOwnerChanged{Meta: m, GoalID: 10, Title: text},
		event.GoalShared{Meta: m, GoalID: 10, Title: text, SharedWithTeamIDs: []int64{3, 4}},
		event.GoalUnshared{Meta: m, GoalID: 10, Title: text},
		event.GoalLinked{Meta: m, ChildGoalID: 10, Title: text, ParentGoalIDs: []int64{5}},
		event.GoalUnlinked{Meta: m, ChildGoalID: 10, Title: text, ParentGoalIDs: []int64{5}},
		event.KRCreated{Meta: m, GoalID: 10, KRID: 20, KRTitle: text},
		event.KRDeleted{Meta: m, GoalID: 10, KRID: 20, KRTitle: text},
		event.KRFieldsChanged{Meta: m, GoalID: 10, KRID: 20, KRTitle: text,
			Changed: map[string][2]any{"target": {1, 2}}},
		event.KRCheckedIn{Meta: m, GoalID: 10, KRID: 20, KRTitle: text, GoalTitle: text,
			ProgressBefore: 1, ProgressAfter: 2, NoteBefore: text, NoteAfter: text},
		event.StatusChanged{Meta: m, TeamTitle: text, Bulk: true},
		event.CommentAdded{Meta: m, GoalID: 10, CommentID: 30, GoalTitle: text, Text: text},
		event.CommentResolved{Meta: m, GoalID: 10, CommentID: 30, GoalTitle: text, AuthorUserID: 4},
		event.CommentReopened{Meta: m, GoalID: 10, CommentID: 30, GoalTitle: text, AuthorUserID: 4},
		event.CommentDeleted{Meta: m, GoalID: 10, CommentID: 30, GoalTitle: text},
		event.ReplyAdded{Meta: m, GoalID: 10, CommentID: 30, ParentCommentID: 29, GoalTitle: text, Text: text},
		event.ReplyDeleted{Meta: m, GoalID: 10, CommentID: 30, GoalTitle: text},
	}
}

func record(t *testing.T, ev event.Event) map[string]any {
	t.Helper()
	buf := &bytes.Buffer{}
	logging.New(logging.Config{Output: buf}).Info("domain event", attrsFor(ev)...)

	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec); err != nil {
		t.Fatalf("запись не является валидным JSON: %v\n%s", err, buf.String())
	}
	return rec
}

// Полнота образцов: список выше обязан покрывать ровно те типы, которые объявлены
// в event.AllKinds().
func TestSamplesCoverEveryKind(t *testing.T) {
	covered := make(map[event.Kind]bool)
	for _, ev := range samples() {
		covered[ev.Kind()] = true
	}
	for _, k := range event.AllKinds() {
		if !covered[k] {
			t.Errorf("тип события %q не покрыт образцом — добавьте его в samples()", k)
		}
	}
	if len(covered) != len(event.AllKinds()) {
		t.Errorf("образцов %d, объявленных типов %d", len(covered), len(event.AllKinds()))
	}
}

// Главный гард: пользовательский текст не доходит до лога ни у одного типа события.
func TestNoUserTextReachesTheLog(t *testing.T) {
	const marker = "СЕКРЕТНЫЙ-ПОЛЬЗОВАТЕЛЬСКИЙ-ТЕКСТ"
	for _, ev := range samples() {
		buf := &bytes.Buffer{}
		logging.New(logging.Config{Output: buf}).Info("domain event", attrsFor(ev)...)

		if strings.Contains(buf.String(), marker) {
			t.Errorf("%s: пользовательский текст попал в лог: %s", ev.Kind(), buf.String())
		}
	}
}

func TestEventRecordCarriesKindAndContext(t *testing.T) {
	rec := record(t, event.GoalCreated{Meta: meta(), GoalID: 10, Title: "любая цель"})

	want := map[string]any{
		logging.KeyEvent:    logging.EventDomainEvent,
		"kind":              string(event.KindGoalCreated),
		logging.KeyTenantID: float64(1),
		logging.KeyActorID:  float64(2),
		logging.KeyTeamID:   float64(7),
		logging.KeyPeriodID: float64(9),
		"goal_id":           float64(10),
	}
	for k, v := range want {
		if rec[k] != v {
			t.Errorf("%s = %v, ожидалось %v", k, rec[k], v)
		}
	}
	if rec["occurred_at"] == nil {
		t.Error("нет времени наступления события")
	}
	if _, ok := rec["title"]; ok {
		t.Errorf("название цели попало в лог: %v", rec["title"])
	}
}

func TestIdentifiersAndFlagsAreExtracted(t *testing.T) {
	rec := record(t, event.GoalCopied{
		Meta: meta(), GoalID: 10, Title: "цель",
		SourceGoalID: 11, SourceTeamID: 12, SourcePeriodID: 13,
		WithProgress: true, WithComments: false,
	})

	want := map[string]any{
		"goal_id":          float64(10),
		"source_goal_id":   float64(11),
		"source_team_id":   float64(12),
		"source_period_id": float64(13),
		"with_progress":    true,
		"with_comments":    false,
	}
	for k, v := range want {
		if rec[k] != v {
			t.Errorf("%s = %v, ожидалось %v", k, rec[k], v)
		}
	}
}

func TestIDSliceIsExtracted(t *testing.T) {
	rec := record(t, event.GoalShared{Meta: meta(), GoalID: 10, Title: "цель", SharedWithTeamIDs: []int64{3, 4}})

	got, ok := rec["shared_with_team_ids"].([]any)
	if !ok || len(got) != 2 || got[0] != float64(3) || got[1] != float64(4) {
		t.Errorf("shared_with_team_ids = %v", rec["shared_with_team_ids"])
	}
}

// Перечислимые значения размечены тегом и обязаны доходить до лога: без них
// «что изменилось» невосстановимо.
func TestEnumFieldsSurvive(t *testing.T) {
	rec := record(t, event.StatusChanged{
		Meta: meta(), TeamTitle: "команда",
		Before: domain.TeamPeriodStatus("forming"), After: domain.TeamPeriodStatus("active"),
		Bulk: true,
	})

	if rec["before"] != "forming" || rec["after"] != "active" {
		t.Errorf("before/after = %v/%v", rec["before"], rec["after"])
	}
	if rec["bulk"] != true {
		t.Errorf("bulk = %v", rec["bulk"])
	}
	if _, ok := rec["team_title"]; ok {
		t.Errorf("название команды попало в лог: %v", rec["team_title"])
	}
}

// У изменения полей в лог идут ИМЕНА изменённых полей, но не их значения: значения
// и есть пользовательский текст.
func TestChangedFieldsLogNamesNotValues(t *testing.T) {
	rec := record(t, event.GoalFieldsChanged{
		Meta: meta(), GoalID: 10, Title: "цель",
		Changed: map[string][2]any{"weight": {1, 2}, "title": {"старое", "новое"}},
	})

	got, ok := rec["changed"].([]any)
	if !ok {
		t.Fatalf("changed = %v, ожидался список имён", rec["changed"])
	}
	// Отсортировано: иначе одно и то же изменение давало бы разные строки.
	if len(got) != 2 || got[0] != "title" || got[1] != "weight" {
		t.Errorf("changed = %v, ожидалось [title weight]", got)
	}
	for _, leak := range []string{"старое", "новое"} {
		if strings.Contains(rec["changed"].([]any)[0].(string), leak) {
			t.Errorf("значение изменённого поля утекло: %v", got)
		}
	}
}

// Новый тип события покрывается подписчиком без изменения кода логирования.
func TestNewEventKindIsCoveredWithoutCodeChange(t *testing.T) {
	rec := record(t, futureEvent{Meta: meta(), WidgetID: 77, Approved: true, Label: "текст пользователя"})

	if rec["kind"] != "future.invented" {
		t.Errorf("kind = %v", rec["kind"])
	}
	if rec["widget_id"] != float64(77) || rec["approved"] != true {
		t.Errorf("идентификаторы нового типа не извлечены: %v", rec)
	}
	if _, ok := rec["label"]; ok {
		t.Errorf("текстовое поле нового типа попало в лог: %v", rec["label"])
	}
	if rec[logging.KeyTenantID] != float64(1) {
		t.Errorf("контекст нового типа не извлечён: %v", rec[logging.KeyTenantID])
	}
}

type futureEvent struct {
	event.Meta
	WidgetID int64
	Approved bool
	Label    string
}

func (futureEvent) Kind() event.Kind { return event.Kind("future.invented") }

// Подписчик логирования не обращается к базе данных: у него нет и не должно
// появиться зависимости, через которую это было бы возможно. Тест фиксирует это
// структурно — Subscribe принимает только шину и логгер.
func TestSubscriberNeedsNothingButBusAndLogger(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := logging.New(logging.Config{Output: buf})

	bus := eventbus.New(logger)
	Subscribe(bus, logger)
	bus.Start(t.Context())

	bus.Publish(t.Context(), event.GoalCreated{Meta: meta(), GoalID: 10, Title: "цель"})

	if err := bus.Close(2 * time.Second); err != nil {
		t.Fatalf("шина не сдренилась: %v", err)
	}
	if !strings.Contains(buf.String(), string(event.KindGoalCreated)) {
		t.Fatalf("публикация не породила запись: %s", buf.String())
	}
	if bus.Dropped() != 0 {
		t.Errorf("события потеряны: %d", bus.Dropped())
	}

	// Сигнатура Subscribe — часть гарантии: добавить обращение к хранилищу без
	// новой зависимости невозможно, а новая зависимость сломает эту сверку.
	if got := reflect.TypeOf(Subscribe).NumIn(); got != 2 {
		t.Errorf("Subscribe принимает %d аргументов; появление третьего означает новую зависимость", got)
	}
}
