package activity_test

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/core/event"
	activitysvc "okrs/internal/service/activity"
	"okrs/internal/service/servicetest"
)

func ptr(v int64) *int64 { return &v }

func meta(tenant int64) event.Meta {
	return event.Meta{
		Scope:    domain.TenantScope{TenantID: tenant},
		ActorID:  7,
		TeamID:   ptr(11),
		PeriodID: ptr(22),
	}
}

// idStr renders a *int64 for failure messages: dereferencing so the message shows the
// value, not the pointer address that %v on a pointer would print.
func idStr(id *int64) string {
	if id == nil {
		return "nil"
	}
	return fmt.Sprintf("%d", *id)
}

func idEqual(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// Каждое из 21 событий обязано превращаться в строку журнала с той же категорией,
// action, entity id колонками (goal_id/kr_id/comment_id/team_id/period_id) и
// payload, что писались до переезда на шину. Расхождение здесь — это молчаливая
// порча ленты активности и её навигационных ссылок.
//
// KRCheckedIn — особый случай (0/1/2 строки в зависимости от того, что изменилось)
// и здесь представлен только своей "прогресс-строкой": остальные его комбинации
// покрыты отдельными тестами ниже (TestKRCheckedInToRows*).
func TestToRowCoversEveryEventType(t *testing.T) {
	// team_id/period_id come from Meta and are the same for every case below
	// (meta() always sets them to 11/22); toRow's base() must copy them through
	// unconditionally, so there is nothing case-specific to vary here.
	wantTeamID, wantPeriodID := ptr(11), ptr(22)

	cases := []struct {
		name      string
		ev        event.Event
		category  domain.ActivityCategory
		action    domain.ActivityAction
		title     string
		payload   map[string]any
		goalID    *int64
		krID      *int64
		commentID *int64
	}{
		{
			name:     "goal_created",
			ev:       event.GoalCreated{Meta: meta(1), GoalID: 5, Title: "Цель"},
			category: domain.ActivityComposition, action: domain.ActionGoalCreated,
			title: "Цель", payload: map[string]any{}, goalID: ptr(5),
		},
		{
			name: "goal_copied",
			ev: event.GoalCopied{Meta: meta(1), GoalID: 5, Title: "Цель",
				SourceGoalID: 4, SourceTeamID: 3, SourcePeriodID: 2, WithProgress: true, WithComments: false},
			category: domain.ActivityComposition, action: domain.ActionGoalCopied,
			title: "Цель", goalID: ptr(5),
			payload: map[string]any{
				"source_goal_id": int64(4), "source_team_id": int64(3), "source_period_id": int64(2),
				"with_progress": true, "with_comments": false,
			},
		},
		{
			name: "goal_moved",
			ev: event.GoalMoved{Meta: meta(1), GoalID: 5, Title: "Цель",
				SourceGoalID: 4, SourceTeamID: 3, SourcePeriodID: 2},
			category: domain.ActivityComposition, action: domain.ActionGoalMoved,
			title: "Цель", goalID: ptr(5),
			payload: map[string]any{
				"source_goal_id": int64(4), "source_team_id": int64(3), "source_period_id": int64(2),
				"with_progress": false, "with_comments": false,
			},
		},
		{
			name:     "goal_deleted",
			ev:       event.GoalDeleted{Meta: meta(1), GoalID: 5, Title: "Цель"},
			category: domain.ActivityComposition, action: domain.ActionGoalDeleted,
			title: "Цель", payload: map[string]any{}, goalID: ptr(5),
		},
		{
			name: "goal_fields_changed",
			ev: event.GoalFieldsChanged{Meta: meta(1), GoalID: 5, Title: "Новое",
				Changed: map[string][2]any{"title": {"Старое", "Новое"}}},
			category: domain.ActivityComposition, action: domain.ActionGoalFieldsChanged,
			title: "Новое", goalID: ptr(5),
			payload: map[string]any{"changed": map[string]any{
				"title": map[string]any{"before": "Старое", "after": "Новое"},
			}},
		},
		{
			name:     "goal_owner_changed",
			ev:       event.GoalOwnerChanged{Meta: meta(1), GoalID: 5, Title: "Цель", BeforeTeamID: 3, AfterTeamID: 9},
			category: domain.ActivityComposition, action: domain.ActionGoalOwnerChanged,
			title: "Цель", goalID: ptr(5),
			payload: map[string]any{
				"before": map[string]any{"owner_team_id": int64(3)},
				"after":  map[string]any{"owner_team_id": int64(9)},
			},
		},
		{
			name:     "goal_shared",
			ev:       event.GoalShared{Meta: meta(1), GoalID: 5, Title: "Цель", SharedWithTeamIDs: []int64{8, 9}},
			category: domain.ActivityComposition, action: domain.ActionGoalShared,
			title: "Цель", payload: map[string]any{"shared_with_team_ids": []int64{8, 9}}, goalID: ptr(5),
		},
		{
			name:     "goal_unshared_declined",
			ev:       event.GoalUnshared{Meta: meta(1), GoalID: 5, Title: "Цель", DeclinedByTeamID: 8},
			category: domain.ActivityComposition, action: domain.ActionGoalUnshared,
			title: "Цель", payload: map[string]any{"declined_by_team_id": int64(8)}, goalID: ptr(5),
		},
		{
			name:     "goal_unshared_single",
			ev:       event.GoalUnshared{Meta: meta(1), GoalID: 5, Title: "Цель", UnsharedTeamID: 8},
			category: domain.ActivityComposition, action: domain.ActionGoalUnshared,
			title: "Цель", payload: map[string]any{"unshared_team_id": int64(8)}, goalID: ptr(5),
		},
		{
			name:     "goal_unshared_bulk",
			ev:       event.GoalUnshared{Meta: meta(1), GoalID: 5, Title: "Цель", UnsharedTeamIDs: []int64{8, 9}},
			category: domain.ActivityComposition, action: domain.ActionGoalUnshared,
			title: "Цель", payload: map[string]any{"unshared_team_ids": []int64{8, 9}}, goalID: ptr(5),
		},
		{
			// GoalID must anchor on the CHILD (the goal that gained a parent), not on
			// one of the parents — the feed's "go to goal" link is for the child.
			name:     "goal_linked",
			ev:       event.GoalLinked{Meta: meta(1), ChildGoalID: 5, Title: "Цель", ParentGoalIDs: []int64{1, 2}},
			category: domain.ActivityComposition, action: domain.ActionGoalLinked,
			title: "Цель", payload: map[string]any{"linked_parent_goal_ids": []int64{1, 2}}, goalID: ptr(5),
		},
		{
			name:     "goal_unlinked",
			ev:       event.GoalUnlinked{Meta: meta(1), ChildGoalID: 5, Title: "Цель", ParentGoalIDs: []int64{1}},
			category: domain.ActivityComposition, action: domain.ActionGoalUnlinked,
			title: "Цель", payload: map[string]any{"unlinked_parent_goal_ids": []int64{1}}, goalID: ptr(5),
		},
		{
			name:     "kr_created",
			ev:       event.KRCreated{Meta: meta(1), GoalID: 5, KRID: 6, KRTitle: "KR"},
			category: domain.ActivityComposition, action: domain.ActionKRCreated,
			title: "KR", payload: map[string]any{}, goalID: ptr(5), krID: ptr(6),
		},
		{
			name:     "kr_deleted",
			ev:       event.KRDeleted{Meta: meta(1), GoalID: 5, KRID: 6, KRTitle: "KR"},
			category: domain.ActivityComposition, action: domain.ActionKRDeleted,
			title: "KR", payload: map[string]any{}, goalID: ptr(5), krID: ptr(6),
		},
		{
			name: "kr_fields_changed",
			ev: event.KRFieldsChanged{Meta: meta(1), GoalID: 5, KRID: 6, KRTitle: "KR",
				Changed: map[string][2]any{"weight": {10, 20}}},
			category: domain.ActivityComposition, action: domain.ActionKRFieldsChanged,
			title: "KR", goalID: ptr(5), krID: ptr(6),
			payload: map[string]any{"changed": map[string]any{
				"weight": map[string]any{"before": 10, "after": 20},
			}},
		},
		{
			// KRCheckedIn only touching progress: the "progress row" half of its
			// toRows split. Note unchanged (before==after) so no discussion row.
			name: "kr_progress",
			ev: event.KRCheckedIn{Meta: meta(1), GoalID: 5, KRID: 6, KRTitle: "KR",
				GoalTitle: "Цель", KRKind: domain.KRKind("NUMERICAL"),
				ProgressBefore: 10, ProgressAfter: 60},
			category: domain.ActivityProgress, action: domain.ActionKRProgress,
			title: "KR", goalID: ptr(5), krID: ptr(6),
			payload: map[string]any{
				"before": map[string]any{"progress": 10},
				"after":  map[string]any{"progress": 60},
				"kind":   "NUMERICAL", "goal_title": "Цель",
			},
		},
		{
			name: "status_changed",
			ev: event.StatusChanged{Meta: meta(1), TeamTitle: "Команда",
				Before: domain.TeamPeriodStatusForming, After: domain.TeamPeriodStatusReady},
			category: domain.ActivityStatus, action: domain.ActionStatusChanged,
			title: "Команда",
			payload: map[string]any{
				"before": map[string]any{"status": string(domain.TeamPeriodStatusForming)},
				"after":  map[string]any{"status": string(domain.TeamPeriodStatusReady)},
			},
		},
		{
			name: "status_changed_bulk",
			ev: event.StatusChanged{Meta: meta(1), TeamTitle: "Команда", Bulk: true,
				Before: domain.TeamPeriodStatusForming, After: domain.TeamPeriodStatusReady},
			category: domain.ActivityStatus, action: domain.ActionStatusChanged,
			title: "Команда",
			payload: map[string]any{
				"before": map[string]any{"status": string(domain.TeamPeriodStatusForming)},
				"after":  map[string]any{"status": string(domain.TeamPeriodStatusReady)},
				"bulk":   true,
			},
		},
		{
			name:     "comment_added",
			ev:       event.CommentAdded{Meta: meta(1), GoalID: 5, CommentID: 6, GoalTitle: "Цель", Text: "текст"},
			category: domain.ActivityDiscussion, action: domain.ActionCommentAdded,
			title: "Цель", payload: map[string]any{"text": "текст"}, goalID: ptr(5), commentID: ptr(6),
		},
		{
			name:     "comment_resolved",
			ev:       event.CommentResolved{Meta: meta(1), GoalID: 5, CommentID: 6, GoalTitle: "Цель", AuthorUserID: 3},
			category: domain.ActivityDiscussion, action: domain.ActionCommentResolved,
			title: "Цель", goalID: ptr(5), commentID: ptr(6),
			payload: map[string]any{
				"before": map[string]any{"resolved": false},
				"after":  map[string]any{"resolved": true},
			},
		},
		{
			name:     "comment_reopened",
			ev:       event.CommentReopened{Meta: meta(1), GoalID: 5, CommentID: 6, GoalTitle: "Цель", AuthorUserID: 3},
			category: domain.ActivityDiscussion, action: domain.ActionCommentReopened,
			title: "Цель", goalID: ptr(5), commentID: ptr(6),
			payload: map[string]any{
				"before": map[string]any{"resolved": true},
				"after":  map[string]any{"resolved": false},
			},
		},
		{
			name:     "comment_deleted",
			ev:       event.CommentDeleted{Meta: meta(1), GoalID: 5, CommentID: 6, GoalTitle: "Цель"},
			category: domain.ActivityDiscussion, action: domain.ActionCommentDeleted,
			title: "Цель", payload: map[string]any{}, goalID: ptr(5), commentID: ptr(6),
		},
		{
			name: "reply_added",
			ev: event.ReplyAdded{Meta: meta(1), GoalID: 5, CommentID: 6, ParentCommentID: 4,
				GoalTitle: "Цель", Text: "ответ"},
			category: domain.ActivityDiscussion, action: domain.ActionReplyAdded,
			title: "Цель", payload: map[string]any{"text": "ответ"}, goalID: ptr(5), commentID: ptr(6),
		},
		{
			name:     "reply_deleted",
			ev:       event.ReplyDeleted{Meta: meta(1), GoalID: 5, CommentID: 6, GoalTitle: "Цель"},
			category: domain.ActivityDiscussion, action: domain.ActionReplyDeleted,
			title: "Цель", payload: map[string]any{}, goalID: ptr(5), commentID: ptr(6),
		},
	}

	// Every Kind event.AllKinds() lists must be represented — not just every Kind the
	// table happens to mention. Counting the table's own cases (as an earlier version
	// of this test did) would still report success after a 22nd event type was added
	// to the event package without a table entry for it, because the count is derived
	// from the same table it is meant to check.
	seen := map[event.Kind]bool{}
	for _, tc := range cases {
		seen[tc.ev.Kind()] = true
	}
	for _, k := range event.AllKinds() {
		if !seen[k] {
			t.Errorf("Kind %q из event.AllKinds() не покрыт таблицей toRows", k)
		}
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows := activitysvc.ToRowsForTest(tc.ev)
			if len(rows) != 1 {
				t.Fatalf("toRows(%T) вернул %d строк, want 1", tc.ev, len(rows))
			}
			row := rows[0]
			if row.Category != tc.category {
				t.Errorf("category: got %q, want %q", row.Category, tc.category)
			}
			if row.Action != tc.action {
				t.Errorf("action: got %q, want %q", row.Action, tc.action)
			}
			if row.EntityTitle != tc.title {
				t.Errorf("entity_title: got %q, want %q", row.EntityTitle, tc.title)
			}
			if row.ActorUserID != 7 {
				t.Errorf("actor: got %d, want 7", row.ActorUserID)
			}
			if !idEqual(row.GoalID, tc.goalID) {
				t.Errorf("goal_id: got %s, want %s", idStr(row.GoalID), idStr(tc.goalID))
			}
			if !idEqual(row.KRID, tc.krID) {
				t.Errorf("kr_id: got %s, want %s", idStr(row.KRID), idStr(tc.krID))
			}
			if !idEqual(row.CommentID, tc.commentID) {
				t.Errorf("comment_id: got %s, want %s", idStr(row.CommentID), idStr(tc.commentID))
			}
			if !idEqual(row.TeamID, wantTeamID) {
				t.Errorf("team_id: got %s, want %s", idStr(row.TeamID), idStr(wantTeamID))
			}
			if !idEqual(row.PeriodID, wantPeriodID) {
				t.Errorf("period_id: got %s, want %s", idStr(row.PeriodID), idStr(wantPeriodID))
			}
			if !reflect.DeepEqual(row.Payload, tc.payload) {
				t.Errorf("payload:\n got %#v\nwant %#v", row.Payload, tc.payload)
			}
		})
	}
}

// unknownTestEvent is a stand-in for an event.Event type toRows has no case for. It
// is test-only: production code never sees an event.Event that isn't one of the 21
// listed in event.AllKinds().
type unknownTestEvent struct{ event.Meta }

func (unknownTestEvent) Kind() event.Kind { return event.Kind("unknown_test_kind") }

// toRows must skip an event type it does not recognise rather than emit a malformed,
// mostly-zero row: a silent "unknown -> empty row" would be worse than dropping it,
// since it would show up in the feed as a blank entry instead of being caught by an
// empty slice at the call site.
func TestToRowSkipsUnmappedEvents(t *testing.T) {
	rows := activitysvc.ToRowsForTest(unknownTestEvent{Meta: meta(1)})
	if len(rows) != 0 {
		t.Fatalf("toRows accepted an unmapped event type, produced %+v", rows)
	}
}

// KRCheckedIn is the one event type that can produce zero, one or two rows,
// depending on which of progress/note actually changed (health-only never
// produces a row — see toRows' comment). These four cases are the ones
// TestToRowCoversEveryEventType's single-scenario table cannot express.
func TestKRCheckedInToRows(t *testing.T) {
	base := event.KRCheckedIn{
		Meta: meta(1), GoalID: 5, KRID: 6, KRTitle: "KR", GoalTitle: "Цель",
		KRKind: domain.KRKind("NUMERICAL"),
	}

	t.Run("только прогресс — одна строка в ленте прогресса", func(t *testing.T) {
		ev := base
		ev.ProgressBefore, ev.ProgressAfter = 10, 60
		rows := activitysvc.ToRowsForTest(ev)
		if len(rows) != 1 {
			t.Fatalf("want 1 row, got %d: %+v", len(rows), rows)
		}
		if rows[0].Category != domain.ActivityProgress || rows[0].Action != domain.ActionKRProgress {
			t.Errorf("row: got category=%q action=%q", rows[0].Category, rows[0].Action)
		}
	})

	t.Run("только заметка — одна строка в обсуждении", func(t *testing.T) {
		ev := base
		ev.NoteBefore, ev.NoteAfter = "было", "стало"
		rows := activitysvc.ToRowsForTest(ev)
		if len(rows) != 1 {
			t.Fatalf("want 1 row, got %d: %+v", len(rows), rows)
		}
		if rows[0].Category != domain.ActivityDiscussion || rows[0].Action != domain.ActionKRNoteUpdated {
			t.Errorf("row: got category=%q action=%q", rows[0].Category, rows[0].Action)
		}
		wantPayload := map[string]any{
			"before": map[string]any{"note": "было"},
			"after":  map[string]any{"note": "стало"},
		}
		if !reflect.DeepEqual(rows[0].Payload, wantPayload) {
			t.Errorf("payload:\n got %#v\nwant %#v", rows[0].Payload, wantPayload)
		}
	})

	t.Run("прогресс и заметка — две строки", func(t *testing.T) {
		ev := base
		ev.ProgressBefore, ev.ProgressAfter = 10, 60
		ev.NoteBefore, ev.NoteAfter = "было", "стало"
		rows := activitysvc.ToRowsForTest(ev)
		if len(rows) != 2 {
			t.Fatalf("want 2 rows, got %d: %+v", len(rows), rows)
		}
		var gotProgress, gotDiscussion bool
		for _, r := range rows {
			switch r.Category {
			case domain.ActivityProgress:
				gotProgress = true
			case domain.ActivityDiscussion:
				gotDiscussion = true
			}
		}
		if !gotProgress || !gotDiscussion {
			t.Errorf("want one progress row and one discussion row, got %+v", rows)
		}
	})

	t.Run("только health — ноль строк", func(t *testing.T) {
		ev := base
		ev.HealthBefore, ev.HealthAfter = domain.KRHealthAtRisk, domain.KRHealthOnTrack
		rows := activitysvc.ToRowsForTest(ev)
		if len(rows) != 0 {
			t.Fatalf("want 0 rows for a health-only check-in, got %d: %+v", len(rows), rows)
		}
	})
}

// Handle пишет батч одним RecordBatch на тенант, а не циклом Record: иначе всплеск
// публикаций превращается в N вставок (правило 9 CLAUDE.md). Батч также обязан
// уйти под ПРАВИЛЬНЫМ TenantScope — перепутанный tenant в RecordBatch означает
// утечку журнальных строк в чужой тенант.
func TestHandleWritesOneBatchPerTenant(t *testing.T) {
	repo := &servicetest.ActivityRepo{}
	svc := activitysvc.New(repo, nil)

	err := svc.Handle(context.Background(), []event.Event{
		event.GoalCreated{Meta: meta(1), GoalID: 1, Title: "A"},
		event.GoalCreated{Meta: meta(1), GoalID: 2, Title: "B"},
		event.GoalCreated{Meta: meta(2), GoalID: 3, Title: "C"},
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	// Два тенанта → ровно два вызова RecordBatch, три строки суммарно.
	if repo.BatchCalls != 2 {
		t.Errorf("RecordBatch вызван %d раз, want 2 (по одному на тенант)", repo.BatchCalls)
	}
	if len(repo.Events) != 3 {
		t.Errorf("записано %d строк, want 3", len(repo.Events))
	}
	// Каждый вызов обязан нести TenantScope своего тенанта: {1, 2}, а не {0, 0}
	// (нулевой Scope) и не {1, 1} (Scope первого тенанта, применённый ко всем).
	gotTenants := make([]int64, len(repo.BatchScopes))
	for i, sc := range repo.BatchScopes {
		gotTenants[i] = sc.TenantID
	}
	sort.Slice(gotTenants, func(i, j int) bool { return gotTenants[i] < gotTenants[j] })
	wantTenants := []int64{1, 2}
	if !reflect.DeepEqual(gotTenants, wantTenants) {
		t.Errorf("RecordBatch tenant scopes: got %v, want %v", gotTenants, wantTenants)
	}
}
