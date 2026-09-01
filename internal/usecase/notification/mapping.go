package notification

import (
	"unicode/utf8"

	"okrs/internal/core/event"
	"okrs/internal/store/notificationprefs"
)

// payloadTextPreviewLimit bounds how much of a user-supplied text field is copied
// into payload_json: a comment's or a reply's text, and a check-in note. Neither is
// capped anywhere else in the system (comments have no limit; key_result_notes.text
// is an unbounded TEXT column with no maxlength in the form and no server-side
// check), and this is the only place that fans such a field out N ways (one
// payload_json copy per recipient) — without a limit, one long comment on a
// well-shared goal, or one long note, amplifies unbounded text across every
// notification row it creates. The note is worse than the comment: CheckIn reads it
// unconditionally, so BOTH before and after land in every recipient's payload on
// EVERY check-in, including the ones that never touched the note. The full text
// still lives on the comment or the note itself; the notification only needs enough
// to preview it.
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

	case event.KRCheckedIn:
		// Every check-in notifies — including a note-only or health-only one. It
		// reuses the kr_progress preference bucket rather than a new type: a user
		// who opted out of KR progress notifications almost certainly wants the
		// same for the checked-in notification that replaced it (this event
		// replaces KRProgressUpdated, which used to be the only KR event that
		// notified at all — see the plan this bridges, kr-checkin-notifications).
		return notificationprefs.TypeKRProgress
	}
	// Deliberately silent: goal_shared, goal_unshared, goal_linked, goal_unlinked,
	// status_changed, comment_reopened, comment_deleted and reply_deleted notify
	// nobody (spec §6.1). Note goal_deleted and kr_deleted are NOT in this list:
	// they fall through to the goal_changed case above, same as any other goal or
	// KR edit.
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
	case event.KRCheckedIn:
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
	case event.KRCheckedIn:
		// before/after carry all three values the checked-in event tracks, not just
		// progress: the renderer (internal/render/notify) picks a different wording
		// depending on which of progress/health/note actually changed, and needs
		// all three to tell which case it is in.
		return map[string]any{
			"before": map[string]any{
				"progress": e.ProgressBefore,
				"health":   string(e.HealthBefore),
				// Both note sides go through truncateText for the same reason the
				// comment text does — see payloadTextPreviewLimit. The renderer only
				// ever previews the note in one line of the body, so a cut costs
				// nothing; the untruncated text stays on the note itself.
				"note": truncateText(e.NoteBefore),
			},
			"after": map[string]any{
				"progress": e.ProgressAfter,
				"health":   string(e.HealthAfter),
				"note":     truncateText(e.NoteAfter),
			},
			"goal_title": e.GoalTitle,
			// note_changed is decided HERE, on the full strings, because the two
			// note fields above are truncated and the renderer cannot tell a real
			// edit from a cut tail once they are: a note longer than the limit that
			// was edited past the limit truncates to two identical strings, and a
			// renderer comparing them would conclude the note did not change and
			// silently drop rule 3's wording. Progress and health need no such flag
			// — nothing truncates them.
			"note_changed": e.NoteBefore != e.NoteAfter,
		}
	}
	return map[string]any{}
}

// truncateText caps a user-supplied string (comment text, check-in note) at
// payloadTextPreviewLimit runes, adding an ellipsis when it cuts. Rune-safe, not
// byte-safe: a byte cut could split a multi-byte character and produce invalid
// UTF-8 in payload_json.
func truncateText(s string) string {
	if utf8.RuneCountInString(s) <= payloadTextPreviewLimit {
		return s
	}
	runes := []rune(s)
	return string(runes[:payloadTextPreviewLimit]) + "…"
}
