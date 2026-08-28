package notify_test

import (
	"strings"
	"testing"

	"okrs/internal/core/event"
	"okrs/internal/render/notify"
)

func TestRenderCommentAdded(t *testing.T) {
	got := notify.Render(notify.Input{
		Kind: event.KindCommentAdded, ActorName: "Пётр", EntityTitle: "Снизить отток", Count: 1,
		Payload: map[string]any{"text": "Уточните метрику"},
	})
	if !strings.Contains(got.Title, "Пётр") || !strings.Contains(got.Title, "комментарий") {
		t.Errorf("заголовок: %q", got.Title)
	}
	if !strings.Contains(got.Body, "Снизить отток") {
		t.Errorf("тело должно называть цель: %q", got.Body)
	}
}

// Схлопнутое уведомление обязано сообщать, сколько событий за ним стоит,
// иначе «×3» в интерфейсе окажется единственным намёком.
func TestRenderMentionsCoalesceCount(t *testing.T) {
	got := notify.Render(notify.Input{
		Kind: event.KindKRProgressUpdated, ActorName: "Пётр", EntityTitle: "MAU", Count: 3,
		Payload: map[string]any{"after": map[string]any{"progress": 60}},
	})
	if !strings.Contains(got.Title, "3") {
		t.Errorf("заголовок схлопнутого уведомления должен нести счётчик: %q", got.Title)
	}
}

func TestRenderProgressShowsPercent(t *testing.T) {
	got := notify.Render(notify.Input{
		Kind: event.KindKRProgressUpdated, ActorName: "Пётр", EntityTitle: "MAU", Count: 1,
		Payload: map[string]any{
			"before": map[string]any{"progress": float64(10)},
			"after":  map[string]any{"progress": float64(60)},
		},
	})
	if !strings.Contains(got.Body, "60") {
		t.Errorf("тело должно показывать новый процент: %q", got.Body)
	}
}

// Progress payloads from in-process construction use int, not float64 from JSONB.
// The percent helper must handle both types, or progress silently disappears after
// a database round-trip while tests stay green.
func TestRenderProgressWithIntPayload(t *testing.T) {
	got := notify.Render(notify.Input{
		Kind: event.KindKRProgressUpdated, ActorName: "Пётр", EntityTitle: "Revenue", Count: 1,
		Payload: map[string]any{
			"before": map[string]any{"progress": 25},
			"after":  map[string]any{"progress": 75},
		},
	})
	if !strings.Contains(got.Body, "75") {
		t.Errorf("тело должно показывать процент из int-типа payload: %q", got.Body)
	}
}

func TestRenderMyCommentResolved(t *testing.T) {
	got := notify.Render(notify.Input{
		Kind: event.KindCommentResolved, ActorName: "Анна", EntityTitle: "Снизить отток", Count: 1,
	})
	if !strings.Contains(got.Title, "Анна") || !strings.Contains(strings.ToLower(got.Title), "решил") {
		t.Errorf("заголовок: %q", got.Title)
	}
}

// Неизвестный kind не должен приводить к пустой строке в интерфейсе:
// уведомление уже создано, и показать его надо хоть как-то.
func TestRenderUnknownKindHasFallback(t *testing.T) {
	got := notify.Render(notify.Input{Kind: "made_up", ActorName: "Пётр", EntityTitle: "Цель", Count: 1})
	if got.Title == "" {
		t.Fatal("для неизвестного kind обязан быть запасной заголовок")
	}
}

// Все 13 kind, порождающих уведомления, должны рендериться осмысленно —
// иначе новое событие в фазе 2 молча даст пустой заголовок.
func TestRenderCoversEveryNotifyingKind(t *testing.T) {
	kinds := []event.Kind{
		event.KindCommentAdded, event.KindReplyAdded, event.KindCommentResolved,
		event.KindGoalCreated, event.KindGoalCopied, event.KindGoalMoved, event.KindGoalDeleted,
		event.KindGoalFieldsChanged, event.KindGoalOwnerChanged,
		event.KindKRCreated, event.KindKRFieldsChanged, event.KindKRDeleted,
		event.KindKRProgressUpdated,
	}
	if len(kinds) != 13 {
		t.Fatalf("перечислено %d kind, ожидалось 13", len(kinds))
	}
	for _, k := range kinds {
		got := notify.Render(notify.Input{Kind: k, ActorName: "Пётр", EntityTitle: "Цель", Count: 1})
		if got.Title == "" || got.Title == notify.FallbackTitle {
			t.Errorf("%s: нет собственного заголовка (got %q)", k, got.Title)
		}
	}
}
