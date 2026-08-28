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
