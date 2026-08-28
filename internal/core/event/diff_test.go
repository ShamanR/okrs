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
