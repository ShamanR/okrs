package event

import (
	"okrs/internal/core/domain"
)

const (
	KindGoalCreated       Kind = "goal_created"
	KindGoalCopied        Kind = "goal_copied"
	KindGoalMoved         Kind = "goal_moved"
	KindGoalDeleted       Kind = "goal_deleted"
	KindGoalFieldsChanged Kind = "goal_fields_changed"
	KindGoalOwnerChanged  Kind = "goal_owner_changed"
	KindGoalShared        Kind = "goal_shared"
	KindGoalUnshared      Kind = "goal_unshared"
	KindGoalLinked        Kind = "goal_linked"
	KindGoalUnlinked      Kind = "goal_unlinked"
	KindKRCreated         Kind = "kr_created"
	KindKRDeleted         Kind = "kr_deleted"
	KindKRFieldsChanged   Kind = "kr_fields_changed"
	KindKRCheckedIn       Kind = "kr_checked_in"
	KindStatusChanged     Kind = "status_changed"
	KindCommentAdded      Kind = "comment_added"
	KindCommentResolved   Kind = "comment_resolved"
	KindCommentReopened   Kind = "comment_reopened"
	KindCommentDeleted    Kind = "comment_deleted"
	KindReplyAdded        Kind = "reply_added"
	KindReplyDeleted      Kind = "reply_deleted"
)

// AllKinds lists every event Kind this package defines. Table-driven tests that claim
// to cover "every event type" (e.g. the journal's toRows coverage test) should iterate
// this instead of counting their own table entries: counting the table's own cases
// only proves the table is internally consistent, not that it tracks this package. A
// 22nd Kind added here without a matching test case then makes such a test fail
// instead of silently staying green. Kept next to the constants so both are edited
// together.
func AllKinds() []Kind {
	return []Kind{
		KindGoalCreated,
		KindGoalCopied,
		KindGoalMoved,
		KindGoalDeleted,
		KindGoalFieldsChanged,
		KindGoalOwnerChanged,
		KindGoalShared,
		KindGoalUnshared,
		KindGoalLinked,
		KindGoalUnlinked,
		KindKRCreated,
		KindKRDeleted,
		KindKRFieldsChanged,
		KindKRCheckedIn,
		KindStatusChanged,
		KindCommentAdded,
		KindCommentResolved,
		KindCommentReopened,
		KindCommentDeleted,
		KindReplyAdded,
		KindReplyDeleted,
	}
}

// --- Goal composition ---

type GoalCreated struct {
	Meta
	GoalID int64
	Title  string
}

func (GoalCreated) Kind() Kind { return KindGoalCreated }

// GoalCopied is a copy landing on a board. For notifications it reads as a created
// goal (see spec §6.1); the journal keeps its own goal_copied action.
//
// Fields mirror the existing journal payload one for one (usecase/goal/goal.go:267):
// source_goal_id, source_team_id, source_period_id, with_progress, with_comments.
type GoalCopied struct {
	Meta
	GoalID         int64
	Title          string
	SourceGoalID   int64
	SourceTeamID   int64
	SourcePeriodID int64
	WithProgress   bool
	WithComments   bool
}

func (GoalCopied) Kind() Kind { return KindGoalCopied }

// GoalMoved is a copy whose source was hard-deleted. Same payload as GoalCopied —
// today both come from one Record call that only switches the action.
type GoalMoved struct {
	Meta
	GoalID         int64
	Title          string
	SourceGoalID   int64
	SourceTeamID   int64
	SourcePeriodID int64
	WithProgress   bool
	WithComments   bool
}

func (GoalMoved) Kind() Kind { return KindGoalMoved }

type GoalDeleted struct {
	Meta
	GoalID int64
	Title  string
}

func (GoalDeleted) Kind() Kind { return KindGoalDeleted }

// GoalFieldsChanged carries only the fields that actually changed:
// field name → {before, after}.
type GoalFieldsChanged struct {
	Meta
	GoalID int64
	Title  string
	// Тег log:"keys" — для лога: в запись идут только имена изменённых полей,
	// но не значения — значения здесь это текст, введённый пользователем.
	Changed map[string][2]any `log:"keys"`
}

func (GoalFieldsChanged) Kind() Kind { return KindGoalFieldsChanged }

// GoalOwnerChanged is a change of the OWNING TEAM, not of goals.owner_udids —
// the journal payload has always been {before:{owner_team_id}, after:{owner_team_id}}.
type GoalOwnerChanged struct {
	Meta
	GoalID       int64
	Title        string
	BeforeTeamID int64
	AfterTeamID  int64
}

func (GoalOwnerChanged) Kind() Kind { return KindGoalOwnerChanged }

type GoalShared struct {
	Meta
	GoalID            int64
	Title             string
	SharedWithTeamIDs []int64
}

func (GoalShared) Kind() Kind { return KindGoalShared }

// GoalUnshared has three call sites today, each writing a DIFFERENT payload shape:
//
//	usecase/goal/goal.go Delete      → {"declined_by_team_id": id}
//	usecase/goal/goal.go ReplaceShares → {"unshared_team_ids": [ids]}
//	usecase/goal/goal.go DeleteShare  → {"unshared_team_id": id}
//
// Exactly one field below is set, so toRow reproduces the historical shape verbatim
// and the activity feed does not change. Normalising these three into one shape is a
// separate change — it would alter stored payloads and is out of this plan's scope.
type GoalUnshared struct {
	Meta
	GoalID           int64
	Title            string
	DeclinedByTeamID int64
	UnsharedTeamID   int64
	UnsharedTeamIDs  []int64
}

func (GoalUnshared) Kind() Kind { return KindGoalUnshared }

// GoalLinked carries the parents added in one operation: ReplaceParents emits a
// single event with all added ids, not one event per link.
type GoalLinked struct {
	Meta
	ChildGoalID   int64
	Title         string
	ParentGoalIDs []int64
}

func (GoalLinked) Kind() Kind { return KindGoalLinked }

type GoalUnlinked struct {
	Meta
	ChildGoalID   int64
	Title         string
	ParentGoalIDs []int64
}

func (GoalUnlinked) Kind() Kind { return KindGoalUnlinked }

// --- Key results. Every KR event carries GoalID (spec §4.2). ---

type KRCreated struct {
	Meta
	GoalID, KRID int64
	KRTitle      string
}

func (KRCreated) Kind() Kind { return KindKRCreated }

type KRDeleted struct {
	Meta
	GoalID, KRID int64
	KRTitle      string
}

func (KRDeleted) Kind() Kind { return KindKRDeleted }

type KRFieldsChanged struct {
	Meta
	GoalID, KRID int64
	KRTitle      string
	// См. комментарий к GoalFieldsChanged.Changed.
	Changed map[string][2]any `log:"keys"`
}

func (KRFieldsChanged) Kind() Kind { return KindKRFieldsChanged }

// KRCheckedIn is one check-in on a key result: the user submits progress, health
// status and note together, and this is the single event that operation produces.
// Before/after pairs are always populated — "changed" is inequality, not a flag —
// so a consumer decides for itself what mattered: the journal splits it into a
// progress row and a discussion row, the notifier renders one line.
//
// Replaces the former KRProgressUpdated and KRNoteUpdated (spec: kr-checkin
// notifications plan, Task 1). GoalTitle and KRKind are carried for the same reason
// KRProgressUpdated carried them: the journal's progress-row payload has always
// included {kind, goal_title}, and toRows reproduces that shape verbatim.
type KRCheckedIn struct {
	Meta
	GoalID, KRID int64
	KRTitle      string
	GoalTitle    string
	// Перечислимые значения, а не введённый текст, поэтому безопасны для лога.
	KRKind         domain.KRKind `log:"safe"`
	ProgressBefore int
	ProgressAfter  int
	HealthBefore   domain.KRHealthStatus `log:"safe"`
	HealthAfter    domain.KRHealthStatus `log:"safe"`
	NoteBefore     string
	NoteAfter      string
}

func (KRCheckedIn) Kind() Kind { return KindKRCheckedIn }

// --- Team period status ---

// StatusChanged. Bulk marks the mass transition done from the admin screen; the
// journal payload carries "bulk": true only in that case, so the flag must survive.
type StatusChanged struct {
	Meta
	TeamTitle string
	// Перечислимые значения, а не введённый текст, поэтому безопасны для лога.
	Before domain.TeamPeriodStatus `log:"safe"`
	After  domain.TeamPeriodStatus `log:"safe"`
	Bulk   bool
}

func (StatusChanged) Kind() Kind { return KindStatusChanged }

// --- Discussion ---

type CommentAdded struct {
	Meta
	GoalID, CommentID int64
	GoalTitle, Text   string
}

func (CommentAdded) Kind() Kind { return KindCommentAdded }

// CommentResolved carries AuthorUserID: the task's author is the addressee of the
// my_comment_resolved notification. Filled at publish time, where the comment is
// already loaded, so the subscriber needs no join to goal_comments.
type CommentResolved struct {
	Meta
	GoalID, CommentID int64
	GoalTitle         string
	AuthorUserID      int64
}

func (CommentResolved) Kind() Kind { return KindCommentResolved }

type CommentReopened struct {
	Meta
	GoalID, CommentID int64
	GoalTitle         string
	AuthorUserID      int64
}

func (CommentReopened) Kind() Kind { return KindCommentReopened }

type CommentDeleted struct {
	Meta
	GoalID, CommentID int64
	GoalTitle         string
}

func (CommentDeleted) Kind() Kind { return KindCommentDeleted }

type ReplyAdded struct {
	Meta
	GoalID, CommentID int64
	ParentCommentID   int64
	GoalTitle, Text   string
}

func (ReplyAdded) Kind() Kind { return KindReplyAdded }

type ReplyDeleted struct {
	Meta
	GoalID, CommentID int64
	GoalTitle         string
}

func (ReplyDeleted) Kind() Kind { return KindReplyDeleted }
