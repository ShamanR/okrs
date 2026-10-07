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

// notifyTypes maps an event onto the notification types it produces, or nothing
// when the event produces none. This function IS the boundary described in spec
// §6.1 — widening it is a product decision, not a refactor.
//
// Nearly every event maps onto exactly one type. The one that does not is a
// resolved task: its author is told personally, and on a SHARED goal the teams
// working on that goal are told too, because resolving a task there is part of
// the discussion they all see. The second type is dropped again for a goal that
// is not shared — see Handle.
func notifyTypes(ev event.Event) []string {
	switch ev.(type) {
	case event.CommentAdded, event.ReplyAdded:
		return []string{notificationprefs.TypeGoalComment}

	case event.CommentResolved:
		return []string{notificationprefs.TypeMyCommentResolved, notificationprefs.TypeGoalComment}

	case event.GoalCreated, event.GoalCopied, event.GoalMoved, event.GoalDeleted,
		event.GoalFieldsChanged, event.GoalOwnerChanged,
		event.GoalShared, event.GoalUnshared,
		event.KRCreated, event.KRFieldsChanged, event.KRDeleted:
		// Sharing and unsharing are goal changes like any other: the goal's set of
		// teams changed, and every team it concerns — the one added, the one
		// removed, and the ones that stay — is told under the preference the user
		// already has for goal changes.
		return []string{notificationprefs.TypeGoalChanged}

	case event.KRCheckedIn:
		// Every check-in notifies — including a note-only or health-only one. It
		// reuses the kr_progress preference bucket rather than a new type: a user
		// who opted out of KR progress notifications almost certainly wants the
		// same for the checked-in notification that replaced it (this event
		// replaces KRProgressUpdated, which used to be the only KR event that
		// notified at all — see the plan this bridges, kr-checkin-notifications).
		return []string{notificationprefs.TypeKRProgress}

	case event.AccessRequested:
		return []string{notificationprefs.TypeAccessRequested}
	}
	// Deliberately silent: goal_linked, goal_unlinked, status_changed,
	// comment_reopened, comment_deleted and reply_deleted notify nobody (spec
	// §6.1). Note goal_deleted and kr_deleted are NOT in this list: they fall
	// through to the goal_changed case above, same as any other goal or KR edit.
	return nil
}

// composedTeamIDs names the teams an event is about that the goal's CURRENT set of
// teams no longer answers for: the ones just added, and the ones just removed.
//
// The removed ones are the reason this exists. By the time the fan-out runs, their
// rows are gone from goal_shares, so the only record that the goal ever concerned
// them is the event itself — and they are exactly the people who need to be told.
func composedTeamIDs(ev event.Event) []int64 {
	switch e := ev.(type) {
	case event.GoalShared:
		return e.SharedWithTeamIDs
	case event.GoalOwnerChanged:
		// Ownership moving away can take the goal off the old owner's board
		// entirely: goal.Delete on a shared goal hands ownership to the first
		// participant and leaves the old owner with neither ownership nor a share.
		// Of everyone involved that team is the one that certainly lost the goal,
		// and the current composition no longer mentions it — so, like a removed
		// participant, it has to come from the event.
		return []int64{e.BeforeTeamID}
	case event.GoalUnshared:
		// Three publication sites, three different shapes — see the type's own
		// comment. Exactly one is ever set; reading all three is what keeps this
		// caller out of that history.
		out := append([]int64(nil), e.UnsharedTeamIDs...)
		if e.UnsharedTeamID != 0 {
			out = append(out, e.UnsharedTeamID)
		}
		if e.DeclinedByTeamID != 0 {
			out = append(out, e.DeclinedByTeamID)
		}
		return out
	}
	return nil
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
	case event.GoalShared:
		return anchor{goalID: id(e.GoalID), title: e.Title}
	case event.GoalUnshared:
		return anchor{goalID: id(e.GoalID), title: e.Title}

	case event.KRCreated:
		return anchor{goalID: id(e.GoalID), krID: id(e.KRID), title: e.KRTitle}
	case event.KRFieldsChanged:
		return anchor{goalID: id(e.GoalID), krID: id(e.KRID), title: e.KRTitle}
	case event.KRDeleted:
		return anchor{goalID: id(e.GoalID), krID: id(e.KRID), title: e.KRTitle}
	case event.KRCheckedIn:
		return anchor{goalID: id(e.GoalID), krID: id(e.KRID), title: e.KRTitle}

	case event.AccessRequested:
		// No goal: the request is about the tenant, which the title names.
		return anchor{title: e.TenantTitle}
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
	case event.GoalShared:
		// The renderer says "your team was added" or "the goal's teams changed"
		// depending on whether the reader's own team is in this list, so the list
		// has to travel with the notification: goal_shares no longer tells the two
		// apart once the change is done.
		//
		// Added and removed are SEPARATE keys, never one "changed" list, because a
		// coalesced row keeps the first event's kind but takes the last event's
		// payload (store/notifications' upsert). One list plus the kind would then
		// tell a team that was just removed it had been added: sharing and
		// unsharing the same goal inside one coalesce window is a single
		// ReplaceShares call away. Naming the verb in the payload keeps the row
		// readable whichever event wrote it last.
		return map[string]any{fieldAddedTeams: teamIDList(e.SharedWithTeamIDs)}
	case event.GoalUnshared:
		return map[string]any{fieldRemovedTeams: teamIDList(composedTeamIDs(e))}
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

// Payload keys naming the teams a composition change touched. Spelled here and
// read in render/notify, the two ends of payload_json.
const (
	fieldAddedTeams   = "added_team_ids"
	fieldRemovedTeams = "removed_team_ids"
)

// teamIDList converts ids into the shape payload_json round-trips cleanly.
//
// []any of int64 rather than []int64: payload_json is written as JSONB and read
// back as []any of float64, and the renderer has to accept one shape, not two
// that differ only by where the notification came from (freshly built in process,
// or loaded from the feed).
func teamIDList(ids []int64) []any {
	out := make([]any, 0, len(ids))
	for _, id := range ids {
		out = append(out, id)
	}
	return out
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
