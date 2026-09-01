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
		Kind: event.KindKRCheckedIn, ActorName: "Пётр", EntityTitle: "MAU", Count: 3,
		Payload: map[string]any{
			"before": map[string]any{"progress": 30, "health": "on_track", "note": ""},
			"after":  map[string]any{"progress": 60, "health": "on_track", "note": ""},
		},
	})
	if !strings.Contains(got.Title, "3") {
		t.Errorf("заголовок схлопнутого уведомления должен нести счётчик: %q", got.Title)
	}
}

// checkedIn — общий конструктор Payload для тестов ниже: before/after по всем трём
// величинам плюс признак note_changed, как их кладёт
// internal/usecase/notification/mapping.go. Признак здесь считается по полным
// строкам ровно потому же, почему его считает продюсер: в payload заметка едет
// обрезанной, и сравнивать обрезанные строки в рендерере нельзя. Что настоящий
// payloadOf кладёт то же самое, проверяет шовный тест в пакете notification.
func checkedIn(beforeProg, afterProg any, beforeHealth, afterHealth, beforeNote, afterNote string) map[string]any {
	return map[string]any{
		"before":       map[string]any{"progress": beforeProg, "health": beforeHealth, "note": beforeNote},
		"after":        map[string]any{"progress": afterProg, "health": afterHealth, "note": afterNote},
		"note_changed": beforeNote != afterNote,
	}
}

// Заголовок больше не утверждает «обновил прогресс»: чек-ин мог не тронуть
// прогресс вовсе (например, менялся только health-статус).
func TestRenderCheckedInTitle(t *testing.T) {
	got := notify.Render(notify.Input{
		Kind: event.KindKRCheckedIn, ActorName: "Пётр", EntityTitle: "MAU", Count: 1,
		Payload: checkedIn(50, 50, "on_track", "at_risk", "", ""),
	})
	if !strings.Contains(got.Title, "Пётр") || !strings.Contains(got.Title, "отметился") {
		t.Errorf("заголовок: %q", got.Title)
	}
	if strings.Contains(got.Title, "прогресс") {
		t.Errorf("заголовок не должен утверждать про прогресс, когда он не менялся: %q", got.Title)
	}
}

// Правило 1 таблицы: только прогресс → "{icon}{status} {from}% → {to}%".
func TestRenderRuleProgressOnly(t *testing.T) {
	got := notify.Render(notify.Input{
		Kind: event.KindKRCheckedIn, EntityTitle: "MAU", Count: 1,
		Payload: checkedIn(10, 60, "on_track", "on_track", "", ""),
	})
	want := "●On Track 10% → 60%"
	if got.Body != want {
		t.Errorf("body = %q, want %q", got.Body, want)
	}
}

// Правило 2: прогресс и статус → "{icon}{from} → {icon}{to} {from}% → {to}%".
func TestRenderRuleProgressAndStatus(t *testing.T) {
	got := notify.Render(notify.Input{
		Kind: event.KindKRCheckedIn, EntityTitle: "MAU", Count: 1,
		Payload: checkedIn(10, 60, "not_started", "on_track", "", ""),
	})
	want := "○Not Started → ●On Track 10% → 60%"
	if got.Body != want {
		t.Errorf("body = %q, want %q", got.Body, want)
	}
}

// Правило 3: только заметка → "{icon}{status} — заметка: {note}".
func TestRenderRuleNoteOnly(t *testing.T) {
	got := notify.Render(notify.Input{
		Kind: event.KindKRCheckedIn, EntityTitle: "MAU", Count: 1,
		Payload: checkedIn(50, 50, "on_track", "on_track", "", "ждём поставку"),
	})
	want := "●On Track — заметка: ждём поставку"
	if got.Body != want {
		t.Errorf("body = %q, want %q", got.Body, want)
	}
}

// Правило 4: заметка и статус → "{icon}{from} → {icon}{to} — заметка: {note}".
func TestRenderRuleNoteAndStatus(t *testing.T) {
	got := notify.Render(notify.Input{
		Kind: event.KindKRCheckedIn, EntityTitle: "MAU", Count: 1,
		Payload: checkedIn(50, 50, "at_risk", "done", "", "починили"),
	})
	want := "▲At Risk → ✓Closed — заметка: починили"
	if got.Body != want {
		t.Errorf("body = %q, want %q", got.Body, want)
	}
}

// Правило 5: только статус → "{icon}{from} → {icon}{to}" — без "%" и без "заметка:".
func TestRenderRuleStatusOnly(t *testing.T) {
	got := notify.Render(notify.Input{
		Kind: event.KindKRCheckedIn, EntityTitle: "MAU", Count: 1,
		Payload: checkedIn(50, 50, "not_started", "on_track", "", ""),
	})
	want := "○Not Started → ●On Track"
	if got.Body != want {
		t.Errorf("body = %q, want %q", got.Body, want)
	}
}

// Изменились и прогресс, и заметка: заметка НЕ должна попасть в текст — правило
// сформулировано в шапке плана явно ("заметка не главное в этом событии, если
// изменилось и то, и другое"). Правило 1/2 должно победить.
func TestRenderProgressAndNoteDropsNote(t *testing.T) {
	got := notify.Render(notify.Input{
		Kind: event.KindKRCheckedIn, EntityTitle: "MAU", Count: 1,
		Payload: checkedIn(10, 60, "on_track", "on_track", "было", "стало"),
	})
	if strings.Contains(got.Body, "заметка") {
		t.Errorf("заметка не должна попасть в тело, когда прогресс тоже изменился: %q", got.Body)
	}
	if !strings.Contains(got.Body, "60") {
		t.Errorf("прогресс обязан остаться в теле: %q", got.Body)
	}
}

// Неизвестный health-статус (рассинхрон с web/static/tracker.js) не должен ронять
// рендер — просто пустые icon/label вместо паники на отсутствующем ключе карты.
func TestRenderCheckedInUnknownHealthDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("рендер запаниковал на неизвестном статусе: %v", r)
		}
	}()
	got := notify.Render(notify.Input{
		Kind: event.KindKRCheckedIn, EntityTitle: "MAU", Count: 1,
		Payload: checkedIn(10, 60, "on_track", "made_up_status", "", ""),
	})
	if !strings.Contains(got.Body, "60") {
		t.Errorf("прогресс всё равно обязан отрендериться: %q", got.Body)
	}
}

// Заметка, изменившаяся на пустую строку (очищена), не должна оставить висящее
// "— заметка: " без текста после двоеточия.
func TestRenderCheckedInEmptyNoteHasNoDanglingSuffix(t *testing.T) {
	got := notify.Render(notify.Input{
		Kind: event.KindKRCheckedIn, EntityTitle: "MAU", Count: 1,
		Payload: checkedIn(50, 50, "on_track", "on_track", "было", ""),
	})
	if strings.Contains(got.Body, "заметка:") {
		t.Errorf("пустая заметка не должна давать висящий суффикс: %q", got.Body)
	}
}

func TestRenderProgressWithIntPayload(t *testing.T) {
	got := notify.Render(notify.Input{
		Kind: event.KindKRCheckedIn, ActorName: "Пётр", EntityTitle: "Revenue", Count: 1,
		Payload: checkedIn(25, 75, "on_track", "on_track", "", ""),
	})
	if !strings.Contains(got.Body, "75") {
		t.Errorf("тело должно показывать процент из int-типа payload: %q", got.Body)
	}
}

// Progress payloads round-tripped through JSONB come back as float64, not int.
func TestRenderProgressWithFloatPayload(t *testing.T) {
	got := notify.Render(notify.Input{
		Kind: event.KindKRCheckedIn, ActorName: "Пётр", EntityTitle: "Revenue", Count: 1,
		Payload: checkedIn(float64(25), float64(75), "on_track", "on_track", "", ""),
	})
	if !strings.Contains(got.Body, "75") {
		t.Errorf("тело должно показывать процент из float64-типа payload: %q", got.Body)
	}
}

// Regression from code review: notifications.kind is stored on write
// (internal/usecase/notification/notification.go) and never migrated — a
// coalesced row's kind is deliberately not refreshed either
// (internal/store/notifications/notifications.go's upsert). Every row already
// written with the old "kr_progress" kind (payload shaped {before/after:
// {progress: N}}, no health/note keys) must keep rendering its old wording after
// KRCheckedIn replaces KindKRProgressUpdated, or it silently degrades to
// FallbackTitle for its whole retention window.
func TestRenderLegacyKRProgressKind(t *testing.T) {
	got := notify.Render(notify.Input{
		Kind: event.Kind("kr_progress"), ActorName: "Пётр", EntityTitle: "MAU", Count: 1,
		Payload: map[string]any{
			"before": map[string]any{"progress": float64(10)},
			"after":  map[string]any{"progress": float64(60)},
		},
	})
	if got.Title != "Пётр обновил прогресс" {
		t.Errorf("title = %q, want %q", got.Title, "Пётр обновил прогресс")
	}
	if !strings.Contains(got.Body, "10%") || !strings.Contains(got.Body, "60%") {
		t.Errorf("body должен показывать старый before/after: %q", got.Body)
	}
	if got.Title == notify.FallbackTitle {
		t.Fatal("старый kind не должен падать в заглушку")
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

// Находка ревью: в теле чек-ина названия KR нет вовсе (пять правил текста его туда
// не кладут), поэтому в команде с десятком KR читатель не понимает, о каком именно
// уведомление. Название приезжает третьей частью — «предметом», отдельной строкой в
// карточке, а решает это сервер, а не клиент.
func TestRenderCheckedInHasSubject(t *testing.T) {
	got := notify.Render(notify.Input{
		Kind: event.KindKRCheckedIn, ActorName: "Пётр", EntityTitle: "MAU", Count: 1,
		Payload: checkedIn(10, 60, "on_track", "on_track", "", ""),
	})
	if got.Subject != "MAU" {
		t.Errorf("subject = %q, want %q", got.Subject, "MAU")
	}
	// Тело остаётся ровно по правилу 1: предмет его не префиксует.
	if got.Body != "●On Track 10% → 60%" {
		t.Errorf("тело не должно меняться из-за предмета: %q", got.Body)
	}
}

// У остальных двенадцати видов название уже внутри тела, поэтому предмет обязан быть
// пустым: иначе карточка напечатает одно и то же название дважды. Легаси-kind
// kr_progress — там же: legacyProgressBody сам называет сущность.
func TestRenderSubjectEmptyForKindsThatNameEntityInBody(t *testing.T) {
	kinds := []event.Kind{
		event.KindCommentAdded, event.KindReplyAdded, event.KindCommentResolved,
		event.KindGoalCreated, event.KindGoalCopied, event.KindGoalMoved, event.KindGoalDeleted,
		event.KindGoalFieldsChanged, event.KindGoalOwnerChanged,
		event.KindKRCreated, event.KindKRFieldsChanged, event.KindKRDeleted,
		"kr_progress", "made_up",
	}
	for _, k := range kinds {
		got := notify.Render(notify.Input{
			Kind: k, ActorName: "Пётр", EntityTitle: "Снизить отток", Count: 1,
			Payload: map[string]any{"text": "Уточните метрику"},
		})
		if got.Subject != "" {
			t.Errorf("%s: предмет должен быть пуст, got %q", k, got.Subject)
		}
		if !strings.Contains(got.Body, "Снизить отток") {
			t.Errorf("%s: тело обязано называть сущность само, раз предмет пуст: %q", k, got.Body)
		}
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
		event.KindKRCheckedIn,
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
