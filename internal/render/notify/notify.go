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

// healthIcon and healthLabel mirror KR_HEALTH_ICON and KR_HEALTH_LABEL in
// web/static/tracker.js:244-245 (labels stay English there — a deliberate product
// decision, not an oversight here). There is no shared source between Go and JS for
// this mapping — the same class of duplication already tracked in
// docs/superpowers/plans/2026-08-27-notifications-tech-debt.md §6.12 for the
// notification deep-link format. A health status added to the tracker without a
// matching entry here silently renders as an empty icon/label in every check-in
// notification rather than failing loudly — see TestRenderCheckedInUnknownHealthDoesNotPanic.
var healthIcon = map[string]string{
	"not_started": "○",
	"on_track":    "●",
	"at_risk":     "▲",
	"done":        "✓",
}

var healthLabel = map[string]string{
	"not_started": "Not Started",
	"on_track":    "On Track",
	"at_risk":     "At Risk",
	"done":        "Closed",
}

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
	// Subject names WHAT the notification is about, on its own line above the body,
	// and is empty for every kind whose body already names it. Only the check-in
	// needs it: its body is a status/progress line built from the five wording
	// rules and has no room for the key result's title, so on a team with ten key
	// results the reader otherwise cannot tell which one moved. The server decides
	// this, not the client — the client must not have to know which kinds repeat
	// their title in the body.
	Subject string
}

// Render builds the notification's title, body and optional subject line.
func Render(in Input) Text {
	title, body := wording(in)
	if in.Count > 1 {
		title = fmt.Sprintf("%s (%d)", title, in.Count)
	}
	return Text{Title: title, Body: body, Subject: subject(in)}
}

// subject returns the entity the notification is about when the body does not
// already name it, and "" otherwise. Every kind but the check-in interpolates
// EntityTitle into its body (see wording above), so returning it here too would
// print the same title twice in the card. The legacy kr_progress kind is not
// included for the same reason: legacyProgressBody names the entity itself.
func subject(in Input) string {
	if in.Kind == event.KindKRCheckedIn {
		return in.EntityTitle
	}
	return ""
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

	case event.KindKRCheckedIn:
		// "отметился", not "обновил прогресс": progress may not have changed at
		// all (a health-only or note-only check-in), and the old title claimed it
		// always had.
		return actor + " отметился по ключевому результату", checkedInBody(in)

	// legacyKindKRProgress: the notifications table stores the publishing event's
	// Kind verbatim (internal/usecase/notification/notification.go writes it,
	// this switch reads it back), and a coalesced row's kind is deliberately never
	// refreshed on repeat (internal/store/notifications/notifications.go's upsert
	// comment). KRCheckedIn replaced KRProgressUpdated, but every row already
	// written — or still live inside its coalesce window — before this change
	// deployed still carries the old "kr_progress" kind forever (kind is not
	// migrated, unlike domain.ActionKRProgress in the activity journal, which is
	// exactly why that constant was kept). Dropping this branch would silently
	// turn every such row into FallbackTitle. Remove only once "kr_progress" rows
	// have aged out of the notifications table's retention window — there is no
	// kr_note_updated equivalent to keep: mapping.go never produced that kind.
	case legacyKindKRProgress:
		return actor + " обновил прогресс", legacyProgressBody(in)
	}
	return FallbackTitle, in.EntityTitle
}

// legacyKindKRProgress is event.KRProgressUpdated's old Kind value, spelled out as
// a literal because the type itself no longer exists in package event.
const legacyKindKRProgress event.Kind = "kr_progress"

// legacyProgressBody is KindKRProgressUpdated's old body wording, kept verbatim
// for legacyKindKRProgress above.
func legacyProgressBody(in Input) string {
	after, ok := percent(in.Payload, "after")
	if !ok {
		return in.EntityTitle
	}
	if before, ok := percent(in.Payload, "before"); ok {
		return fmt.Sprintf("%s: %d%% → %d%%", in.EntityTitle, before, after)
	}
	return fmt.Sprintf("%s: %d%%", in.EntityTitle, after)
}

// quoteOr returns the payload's field value if present and non-empty, otherwise
// returns the provided fallback. Used to extract text content from payloads.
func quoteOr(in Input, field, fallback string) string {
	if s, ok := in.Payload[field].(string); ok && s != "" {
		return s
	}
	return fallback
}

// checkedInBody renders a KRCheckedIn per the five rules (plan
// docs/superpowers/plans/2026-08-31-kr-checkin-notifications.md, table in the
// header): which of progress/health/note changed decides the wording, and a
// changed note is shown ONLY when progress did not also change — it is not the
// point of the notification when both moved together.
func checkedInBody(in Input) string {
	beforeProg, hasBefore := percent(in.Payload, "before")
	afterProg, hasAfter := percent(in.Payload, "after")
	progressChanged := hasBefore && hasAfter && beforeProg != afterProg

	beforeHealth, _ := stringAt(in.Payload, "before", "health")
	afterHealth, _ := stringAt(in.Payload, "after", "health")
	healthChanged := beforeHealth != afterHealth

	afterNote, _ := stringAt(in.Payload, "after", "note")
	// Whether the note changed is NOT decided here by comparing the two note
	// strings: both sides arrive truncated (internal/usecase/notification's
	// payloadOf caps them, since payload_json is copied once per recipient), so a
	// long note edited past the cut point would compare equal and rule 3 would
	// silently vanish. The producer decides it on the full strings and ships the
	// answer as note_changed. A note "changed" to empty (cleared) must still never
	// render a hanging "— заметка:" with nothing after it, so an empty afterNote
	// never counts as showable.
	noteChanged, _ := boolAt(in.Payload, "note_changed")
	showNote := noteChanged && afterNote != ""

	icon := func(status string) string { return healthIcon[status] }
	label := func(status string) string { return healthLabel[status] }
	statusText := func(status string) string { return icon(status) + label(status) }
	transition := func(before, after string) string {
		return statusText(before) + " → " + statusText(after)
	}

	switch {
	case progressChanged && healthChanged:
		return fmt.Sprintf("%s %d%% → %d%%", transition(beforeHealth, afterHealth), beforeProg, afterProg)
	case progressChanged:
		return fmt.Sprintf("%s %d%% → %d%%", statusText(afterHealth), beforeProg, afterProg)
	case healthChanged && showNote:
		return fmt.Sprintf("%s — заметка: %s", transition(beforeHealth, afterHealth), afterNote)
	case showNote:
		return fmt.Sprintf("%s — заметка: %s", statusText(afterHealth), afterNote)
	case healthChanged:
		return transition(beforeHealth, afterHealth)
	default:
		// Reached when nothing SHOWABLE changed even though CheckIn did publish.
		// The all-three-unchanged case cannot get here (CheckIn would not have
		// published it), and neither can a long note edited past the truncation
		// limit any more — that used to land here, because showNote compared the
		// two truncated strings; it now reads note_changed instead. What remains is
		// one real case: a note cleared to empty with progress and health both
		// unchanged. note_changed is true there (the note did change, so CheckIn
		// publishes), yet showNote is false because an empty afterNote is never
		// shown — see its comment. Fall back to the current status alone rather
		// than an empty body.
		return statusText(afterHealth)
	}
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

// boolAt digs a top-level boolean flag out of the payload. Values arriving from
// JSONB and values built in-process are both plain bool; a missing flag reads as
// false, which is why the producer must always write note_changed rather than
// omitting it when the note did not change.
func boolAt(payload map[string]any, field string) (bool, bool) {
	b, ok := payload[field].(bool)
	return b, ok
}

// stringAt digs a string field out of the {before|after: {field: "..."}} payload.
func stringAt(payload map[string]any, side, field string) (string, bool) {
	m, ok := payload[side].(map[string]any)
	if !ok {
		return "", false
	}
	s, ok := m[field].(string)
	return s, ok
}
