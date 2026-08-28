package notification

import (
	"strings"
	"testing"
	"unicode/utf8"

	"okrs/internal/core/event"
)

// Комментарий не имеет ограничения длины нигде в системе, а payloadOf — единственное
// место, которое размножает его текст на N получателей (по копии payload_json на
// каждого адресата). Без обрезки один длинный комментарий на широко расшаренной цели
// усиливает неограниченный пользовательский текст N раз.
func TestPayloadOfTruncatesLongCommentText(t *testing.T) {
	long := strings.Repeat("я", payloadTextPreviewLimit+50)
	p := payloadOf(event.CommentAdded{GoalID: 1, CommentID: 2, GoalTitle: "Цель", Text: long})
	got, _ := p["text"].(string)
	if utf8.RuneCountInString(got) > payloadTextPreviewLimit+1 { // +1 допускает добавленное многоточие
		t.Fatalf("длина усечённого текста в рунах = %d, want <= %d", utf8.RuneCountInString(got), payloadTextPreviewLimit+1)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("усечённый текст должен заканчиваться многоточием: %q", got)
	}
}

// Короткий текст обязан пройти без изменений — обрезка не должна портить обычный
// случай добавлением многоточия там, где текст и так укладывается в лимит.
func TestPayloadOfLeavesShortCommentTextUntouched(t *testing.T) {
	p := payloadOf(event.ReplyAdded{GoalID: 1, CommentID: 2, GoalTitle: "Цель", Text: "коротко"})
	got, _ := p["text"].(string)
	if got != "коротко" {
		t.Errorf("text = %q, want %q без изменений", got, "коротко")
	}
}

// truncateText режет по рунам, а не по байтам: обрезка посередине многобайтового
// символа дала бы невалидный UTF-8 в payload_json.
func TestTruncateTextIsRuneSafe(t *testing.T) {
	s := strings.Repeat("駄", payloadTextPreviewLimit) + "駄駄駄"
	got := truncateText(s)
	if !utf8.ValidString(got) {
		t.Fatalf("truncateText вернул невалидный UTF-8: %q", got)
	}
	if utf8.RuneCountInString(got) != payloadTextPreviewLimit+1 {
		t.Fatalf("длина в рунах = %d, want %d (лимит + многоточие)", utf8.RuneCountInString(got), payloadTextPreviewLimit+1)
	}
}
