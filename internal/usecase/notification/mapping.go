package notification

import (
	"unicode/utf8"

	"okrs/internal/core/event"
	"okrs/internal/store/notificationprefs"
)

// payloadTextPreviewLimit bounds how much of a comment's text is copied into
// payload_json. Comment length has no cap elsewhere in the system, and this is the
// only place that fans a user-supplied field out N ways (one payload_json copy per
// recipient) — without a limit, one long comment on a well-shared goal amplifies
// unbounded text across every notification row it creates. The full text still
// lives on the comment itself; the notification only needs enough to preview it.
const payloadTextPreviewLimit = 500

// notifyType maps an event onto the notification type it produces, or "" when the
// event produces none. This function IS the boundary described in spec §6.1 —
// widening it is a product decision, not a refactor.
func notifyType(ev event.Event) string {
	switch ev.(type) {
	case event.CommentAdded, event.ReplyAdded:
		return notificationprefs.TypeGoalComment

	case event.CommentResolved:
		return notificationprefs.TypeMyCommentResolved

	case event.GoalCreated, event.GoalCopied, event.GoalMoved, event.GoalDeleted,
		event.GoalFieldsChanged, event.GoalOwnerChanged,
		event.KRCreated, event.KRFieldsChanged, event.KRDeleted:
		return notificationprefs.TypeGoalChanged

	case event.KRProgressUpdated:
		return notificationprefs.TypeKRProgress
	}
	// Deliberately silent: goal_shared, goal_unshared, goal_linked, goal_unlinked,
	// kr_note_updated, status_changed, comment_reopened, comment_deleted and
	// reply_deleted notify nobody (spec §6.1). Note goal_deleted and kr_deleted are
	// NOT in this list: they fall through to the goal_changed case above, same as
	// any other goal or KR edit.
	return ""
}

// anchor is what a notification points at: the goal (or KR, for progress) plus the
// ids stored on the row.
type anchor struct {
	goalID    *int64
	krID      *int64
	commentID *int64
	title     string
	// addressee is set only for addressed types.
	addressee int64
}

// anchorOf extracts the anchor from an event. Every KR event carries GoalID, which
// is why a KR change can be addressed and coalesced as a change to its goal without
// an extra query.
func anchorOf(ev event.Event) anchor {
	id := func(v int64) *int64 { return &v }
	switch e := ev.(type) {
	case event.CommentAdded:
		return anchor{goalID: id(e.GoalID), commentID: id(e.CommentID), title: e.GoalTitle}
	case event.ReplyAdded:
		return anchor{goalID: id(e.GoalID), commentID: id(e.CommentID), title: e.GoalTitle}
	case event.CommentResolved:
		return anchor{goalID: id(e.GoalID), commentID: id(e.CommentID), title: e.GoalTitle, addressee: e.AuthorUserID}

	case event.GoalCreated:
		return anchor{goalID: id(e.GoalID), title: e.Title}
	case event.GoalCopied:
		return anchor{goalID: id(e.GoalID), title: e.Title}
	case event.GoalMoved:
		return anchor{goalID: id(e.GoalID), title: e.Title}
	case event.GoalDeleted:
		return anchor{goalID: id(e.GoalID), title: e.Title}
	case event.GoalFieldsChanged:
		return anchor{goalID: id(e.GoalID), title: e.Title}
	case event.GoalOwnerChanged:
		return anchor{goalID: id(e.GoalID), title: e.Title}

	case event.KRCreated:
		return anchor{goalID: id(e.GoalID), krID: id(e.KRID), title: e.KRTitle}
	case event.KRFieldsChanged:
		return anchor{goalID: id(e.GoalID), krID: id(e.KRID), title: e.KRTitle}
	case event.KRDeleted:
		return anchor{goalID: id(e.GoalID), krID: id(e.KRID), title: e.KRTitle}
	case event.KRProgressUpdated:
		return anchor{goalID: id(e.GoalID), krID: id(e.KRID), title: e.KRTitle}
	}
	return anchor{}
}

// payloadOf carries only what the renderer needs, not the whole journal payload.
func payloadOf(ev event.Event) map[string]any {
	switch e := ev.(type) {
	case event.CommentAdded:
		return map[string]any{"text": truncateText(e.Text)}
	case event.ReplyAdded:
		return map[string]any{"text": truncateText(e.Text)}
	case event.KRProgressUpdated:
		return map[string]any{
			"before":     map[string]any{"progress": e.Before},
			"after":      map[string]any{"progress": e.After},
			"goal_title": e.GoalTitle,
		}
	}
	return map[string]any{}
}

// truncateText caps a user-supplied string at payloadTextPreviewLimit runes, adding
// an ellipsis when it cuts. Rune-safe, not byte-safe: a byte cut could split a
// multi-byte character and produce invalid UTF-8 in payload_json.
func truncateText(s string) string {
	if utf8.RuneCountInString(s) <= payloadTextPreviewLimit {
		return s
	}
	runes := []rune(s)
	return string(runes[:payloadTextPreviewLimit]) + "…"
}
