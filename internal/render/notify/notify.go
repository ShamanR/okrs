// Package notify renders a notification into human text. Server-side on purpose:
// phase 2 sends the same strings to Telegram and Mattermost, and keeping templates
// in both Go and JS would guarantee the wordings drift apart.
//
// Mirrors internal/render/export, which renders OKRs to Markdown.
package notify

import (
	"fmt"

	"okrs/internal/core/event"
)

// FallbackTitle is used when a kind has no dedicated wording. Exported so tests can
// assert that every notifying kind has its own text rather than falling through.
const FallbackTitle = "Обновление по цели"

type Input struct {
	Kind        event.Kind
	ActorName   string
	EntityTitle string
	Count       int // coalesce_count: >1 means several events collapsed into one
	Payload     map[string]any
}

type Text struct {
	Title string
	Body  string
}

// Render builds the notification's title and body.
func Render(in Input) Text {
	title, body := wording(in)
	if in.Count > 1 {
		title = fmt.Sprintf("%s (%d)", title, in.Count)
	}
	return Text{Title: title, Body: body}
}

func wording(in Input) (title, body string) {
	actor := in.ActorName
	if actor == "" {
		actor = "Кто-то"
	}
	switch in.Kind {
	case event.KindCommentAdded:
		text := quoteOr(in, "text", "")
		if text != "" {
			return actor + " оставил комментарий", in.EntityTitle + ": " + text
		}
		return actor + " оставил комментарий", in.EntityTitle
	case event.KindReplyAdded:
		text := quoteOr(in, "text", "")
		if text != "" {
			return actor + " ответил в обсуждении", in.EntityTitle + ": " + text
		}
		return actor + " ответил в обсуждении", in.EntityTitle
	case event.KindCommentResolved:
		return actor + " решил ваш комментарий", in.EntityTitle

	case event.KindGoalCreated:
		return actor + " создал цель", in.EntityTitle
	case event.KindGoalCopied:
		return actor + " скопировал цель", in.EntityTitle
	case event.KindGoalMoved:
		return actor + " перенёс цель", in.EntityTitle
	case event.KindGoalDeleted:
		return actor + " удалил цель", in.EntityTitle
	case event.KindGoalFieldsChanged:
		return actor + " изменил цель", in.EntityTitle
	case event.KindGoalOwnerChanged:
		return actor + " сменил владельца цели", in.EntityTitle

	case event.KindKRCreated:
		return actor + " добавил ключевой результат", in.EntityTitle
	case event.KindKRFieldsChanged:
		return actor + " изменил ключевой результат", in.EntityTitle
	case event.KindKRDeleted:
		return actor + " удалил ключевой результат", in.EntityTitle

	case event.KindKRProgressUpdated:
		return actor + " обновил прогресс", progressBody(in)
	}
	return FallbackTitle, in.EntityTitle
}

// quoteOr returns the payload's field value if present and non-empty, otherwise
// returns the provided fallback. Used to extract text content from payloads.
func quoteOr(in Input, field, fallback string) string {
	if s, ok := in.Payload[field].(string); ok && s != "" {
		return s
	}
	return fallback
}

func progressBody(in Input) string {
	after, ok := percent(in.Payload, "after")
	if !ok {
		return in.EntityTitle
	}
	if before, ok := percent(in.Payload, "before"); ok {
		return fmt.Sprintf("%s: %d%% → %d%%", in.EntityTitle, before, after)
	}
	return fmt.Sprintf("%s: %d%%", in.EntityTitle, after)
}

// percent digs progress out of the {before|after: {progress: N}} payload. Values
// arriving from JSONB are float64, values built in-process are int — handle both.
func percent(payload map[string]any, side string) (int, bool) {
	m, ok := payload[side].(map[string]any)
	if !ok {
		return 0, false
	}
	switch v := m["progress"].(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	}
	return 0, false
}
