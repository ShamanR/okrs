package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"okrs/internal/domain"
	"okrs/internal/store/activity"
	"okrs/internal/store/goals"
	"okrs/internal/store/shares"
)

type fakeActivityRepo struct {
	recorded []domain.ActivityEvent
	failNext bool
}

func (f *fakeActivityRepo) Record(_ context.Context, _ domain.TenantScope, ev domain.ActivityEvent) (int64, error) {
	if f.failNext {
		return 0, errors.New("boom")
	}
	f.recorded = append(f.recorded, ev)
	return int64(len(f.recorded)), nil
}
func (f *fakeActivityRepo) RecordBatch(_ context.Context, _ domain.TenantScope, evs []domain.ActivityEvent) error {
	if f.failNext {
		return errors.New("boom")
	}
	f.recorded = append(f.recorded, evs...)
	return nil
}
func (f *fakeActivityRepo) List(context.Context, domain.TenantScope, []int64, activity.ListFilter) ([]domain.ActivityEvent, *activity.Cursor, error) {
	return nil, nil, nil
}
func (f *fakeActivityRepo) TreeCounts(context.Context, domain.TenantScope, []int64, *int64, *time.Time) (map[int64]int, error) {
	return nil, nil
}
func (f *fakeActivityRepo) CategoryCounts(context.Context, domain.TenantScope, []int64, activity.ListFilter) (map[string]int, error) {
	return nil, nil
}
func (f *fakeActivityRepo) Purge(context.Context, domain.TenantScope, *time.Time) (int64, error) {
	return 0, nil
}

func TestAddGoalCommentRecordsEvent(t *testing.T) {
	fa := &fakeActivityRepo{}
	gf := &goalFakeStore{goals: map[int64]domain.Goal{7: {ID: 7, TeamID: 42, PeriodID: 3, Title: "P95"}}}
	s := New(Deps{Activity: fa, Goals: gf})
	if err := s.AddGoalComment(context.Background(), domain.TenantScope{TenantID: 1}, 7, "blocker", 5); err != nil {
		t.Fatalf("AddGoalComment: %v", err)
	}
	if len(fa.recorded) != 1 {
		t.Fatalf("want 1 event, got %d", len(fa.recorded))
	}
	ev := fa.recorded[0]
	if ev.Category != domain.ActivityDiscussion || ev.Action != domain.ActionCommentAdded {
		t.Fatalf("wrong event: %+v", ev)
	}
	if ev.TeamID == nil || *ev.TeamID != 42 || ev.EntityTitle != "P95" || ev.Payload["text"] != "blocker" {
		t.Fatalf("event fields wrong: %+v", ev)
	}
}

func TestSetGoalCommentResolvedRecordsEvent(t *testing.T) {
	fa := &fakeActivityRepo{}
	gf := &goalFakeStore{goals: map[int64]domain.Goal{7: {ID: 7, TeamID: 42, PeriodID: 3, Title: "P95"}}}
	s := New(Deps{Activity: fa, Goals: gf})
	if err := s.SetGoalCommentResolved(context.Background(), domain.TenantScope{TenantID: 1}, 7, 11, true, 5); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(fa.recorded) != 1 || fa.recorded[0].Action != domain.ActionCommentResolved {
		t.Fatalf("wrong resolve event: %+v", fa.recorded)
	}
	fa.recorded = nil
	if err := s.SetGoalCommentResolved(context.Background(), domain.TenantScope{TenantID: 1}, 7, 11, false, 5); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if len(fa.recorded) != 1 || fa.recorded[0].Action != domain.ActionCommentReopened {
		t.Fatalf("wrong reopen event: %+v", fa.recorded)
	}
}

func TestAddGoalReplyRecordsReplyAddedEvent(t *testing.T) {
	fa := &fakeActivityRepo{}
	gf := &goalFakeStore{goals: map[int64]domain.Goal{7: {ID: 7, TeamID: 42, PeriodID: 3, Title: "P95"}}}
	s := New(Deps{Activity: fa, Goals: gf})
	if err := s.AddGoalReply(context.Background(), domain.TenantScope{TenantID: 1}, 7, 11, "a reply", 5); err != nil {
		t.Fatalf("AddGoalReply: %v", err)
	}
	if len(fa.recorded) != 1 {
		t.Fatalf("want 1 event, got %d", len(fa.recorded))
	}
	ev := fa.recorded[0]
	if ev.Category != domain.ActivityDiscussion || ev.Action != domain.ActionReplyAdded {
		t.Fatalf("wrong event: %+v", ev)
	}
	if ev.TeamID == nil || *ev.TeamID != 42 || ev.EntityTitle != "P95" || ev.Payload["text"] != "a reply" {
		t.Fatalf("event fields wrong: %+v", ev)
	}
}

func TestAddGoalReplyBadParentNoEvent(t *testing.T) {
	fa := &fakeActivityRepo{}
	gf := &goalFakeStore{goals: map[int64]domain.Goal{7: {ID: 7}}, addReplyErr: goals.ErrNotFound}
	s := New(Deps{Activity: fa, Goals: gf})
	if err := s.AddGoalReply(context.Background(), domain.TenantScope{TenantID: 1}, 7, 999, "orphan", 5); !errors.Is(err, goals.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if len(fa.recorded) != 0 {
		t.Fatalf("no event expected on failed reply, got %d", len(fa.recorded))
	}
}

func TestDeleteGoalCommentForbiddenForNonAuthorNonAdmin(t *testing.T) {
	fa := &fakeActivityRepo{}
	gf := &goalFakeStore{goals: map[int64]domain.Goal{7: {ID: 7, TeamID: 42, PeriodID: 3, Title: "P95"}}, commentAuthor: 2, commentIsTask: true}
	s := New(Deps{Activity: fa, Goals: gf})
	// requesting user 1, author is 2, not admin → forbidden.
	if _, err := s.DeleteGoalComment(context.Background(), domain.TenantScope{TenantID: 1}, 7, 11, 1, false); !errors.Is(err, ErrForbidden) {
		t.Fatalf("want ErrForbidden, got %v", err)
	}
	if len(gf.deleteCommentCalls) != 0 || len(fa.recorded) != 0 {
		t.Fatalf("no delete/event expected on forbidden: deletes=%v events=%d", gf.deleteCommentCalls, len(fa.recorded))
	}
}

func TestDeleteGoalCommentAdminDeletesReplyLogsReplyDeleted(t *testing.T) {
	fa := &fakeActivityRepo{}
	gf := &goalFakeStore{goals: map[int64]domain.Goal{7: {ID: 7, TeamID: 42, PeriodID: 3, Title: "P95"}}, commentAuthor: 2, commentIsTask: false}
	s := New(Deps{Activity: fa, Goals: gf})
	// requesting user 1 is admin, author is 2, target is a reply.
	isTask, err := s.DeleteGoalComment(context.Background(), domain.TenantScope{TenantID: 1}, 7, 11, 1, true)
	if err != nil || isTask {
		t.Fatalf("admin delete reply: isTask=%v err=%v", isTask, err)
	}
	if len(gf.deleteCommentCalls) != 1 {
		t.Fatalf("delete must be called once, got %d", len(gf.deleteCommentCalls))
	}
	if len(fa.recorded) != 1 || fa.recorded[0].Action != domain.ActionReplyDeleted {
		t.Fatalf("want reply_deleted, got %+v", fa.recorded)
	}
}

func TestDeleteGoalCommentAuthorDeletesTaskLogsCommentDeleted(t *testing.T) {
	fa := &fakeActivityRepo{}
	gf := &goalFakeStore{goals: map[int64]domain.Goal{7: {ID: 7, TeamID: 42, PeriodID: 3, Title: "P95"}}, commentAuthor: 5, commentIsTask: true}
	s := New(Deps{Activity: fa, Goals: gf})
	isTask, err := s.DeleteGoalComment(context.Background(), domain.TenantScope{TenantID: 1}, 7, 11, 5, false)
	if err != nil || !isTask {
		t.Fatalf("author delete task: isTask=%v err=%v", isTask, err)
	}
	if len(fa.recorded) != 1 || fa.recorded[0].Action != domain.ActionCommentDeleted {
		t.Fatalf("want comment_deleted, got %+v", fa.recorded)
	}
}

func TestDeleteGoalCommentMissingReturnsNotFound(t *testing.T) {
	fa := &fakeActivityRepo{}
	gf := &goalFakeStore{goals: map[int64]domain.Goal{7: {ID: 7}}, commentMetaErr: goals.ErrNotFound}
	s := New(Deps{Activity: fa, Goals: gf})
	if _, err := s.DeleteGoalComment(context.Background(), domain.TenantScope{TenantID: 1}, 7, 11, 5, false); !errors.Is(err, goals.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if len(fa.recorded) != 0 {
		t.Fatalf("no event expected when comment missing, got %d", len(fa.recorded))
	}
}

func TestUpdateStatusRecordsEvent(t *testing.T) {
	fa := &fakeActivityRepo{}
	st := newFakeStore()
	st.teams = []domain.Team{{ID: 10, Name: "PaaS / Infra"}}
	s := New(Deps{Activity: fa, Teams: st, Statuses: st})
	if err := s.UpdateTeamPeriodStatus(context.Background(), domain.TenantScope{TenantID: 1}, 10, 3, domain.TeamPeriodStatusInProgress, 5); err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(fa.recorded) != 1 {
		t.Fatalf("want 1 event, got %d", len(fa.recorded))
	}
	ev := fa.recorded[0]
	if ev.Category != domain.ActivityStatus || ev.Action != domain.ActionStatusChanged || ev.EntityTitle != "PaaS / Infra" {
		t.Fatalf("wrong event: %+v", ev)
	}
	if ev.Payload["after"].(map[string]any)["status"] != string(domain.TeamPeriodStatusInProgress) {
		t.Fatalf("after status wrong: %+v", ev.Payload)
	}
}

func TestKRProgressRecordsBeforeAfterNumbers(t *testing.T) {
	fa := &fakeActivityRepo{}
	st := newFakeStore()
	// Numerical KR: 0→100, currently at 30 (=30%). Update current to 80 (=80%).
	st.keyResults[55] = domain.KeyResult{ID: 55, GoalID: 7, Kind: domain.KRKindNumerical, Title: "P95 latency",
		Numerical: &domain.KRNumerical{StartValue: 0, TargetValue: 100, CurrentValue: 30}}
	s := New(Deps{Activity: fa, KRs: st, Goals: st})
	if err := s.UpdateKRProgressNumerical(context.Background(), domain.TenantScope{TenantID: 1}, 55, 80, 5); err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(fa.recorded) != 1 {
		t.Fatalf("want 1 event, got %d", len(fa.recorded))
	}
	ev := fa.recorded[0]
	if ev.Category != domain.ActivityProgress || ev.Action != domain.ActionKRProgress || *ev.KRID != 55 {
		t.Fatalf("wrong event: %+v", ev)
	}
	// Regression: before/after must be the real computed percentages, not 0→0.
	if ev.Payload["before"].(map[string]any)["progress"] != 30 {
		t.Fatalf("before progress wrong (want 30): %+v", ev.Payload)
	}
	if ev.Payload["after"].(map[string]any)["progress"] != 80 {
		t.Fatalf("after progress wrong (want 80): %+v", ev.Payload)
	}
}

func TestKRNoteUpdateRecordsEvent(t *testing.T) {
	fa := &fakeActivityRepo{}
	st := newFakeStore()
	st.keyResults[55] = domain.KeyResult{ID: 55, GoalID: 7, Kind: domain.KRKindNumerical, Title: "P95 latency"}
	s := New(Deps{Activity: fa, KRs: st, Goals: st})
	// GetKeyResultNote returns nil (no prior note) → beforeText "" != "circuit breaker" → records.
	if err := s.UpsertKeyResultNote(context.Background(), domain.TenantScope{TenantID: 1}, 55, "добавили circuit breaker", 5); err != nil {
		t.Fatalf("note: %v", err)
	}
	if len(fa.recorded) != 1 {
		t.Fatalf("want 1 event, got %d", len(fa.recorded))
	}
	ev := fa.recorded[0]
	if ev.Category != domain.ActivityDiscussion || ev.Action != domain.ActionKRNoteUpdated || *ev.KRID != 55 {
		t.Fatalf("wrong note event: %+v", ev)
	}
	if ev.Payload["after"].(map[string]any)["note"] != "добавили circuit breaker" {
		t.Fatalf("note payload wrong: %+v", ev.Payload)
	}
}

func TestUpdateGoalRecordsFieldChange(t *testing.T) {
	fa := &fakeActivityRepo{}
	gf := &goalFakeStore{goals: map[int64]domain.Goal{7: {ID: 7, TeamID: 42, PeriodID: 3, Title: "старое"}}}
	s := New(Deps{Activity: fa, Goals: gf})
	if err := s.UpdateGoal(context.Background(), domain.TenantScope{TenantID: 1},
		goals.GoalUpdateInput{ID: 7, Title: "новое", Priority: domain.PriorityP1, Weight: 100}, 5); err != nil {
		t.Fatalf("update goal: %v", err)
	}
	if len(fa.recorded) != 1 {
		t.Fatalf("want 1 event, got %d", len(fa.recorded))
	}
	ev := fa.recorded[0]
	if ev.Action != domain.ActionGoalFieldsChanged {
		t.Fatalf("wrong action: %+v", ev)
	}
	changed := ev.Payload["changed"].(map[string]any)
	if changed["title"].(map[string]any)["after"] != "новое" {
		t.Fatalf("title change not captured: %+v", changed)
	}
}

func TestCreateGoalRecordsEvent(t *testing.T) {
	fa := &fakeActivityRepo{}
	st := newFakeStore()
	s := New(Deps{Activity: fa, Goals: st, Statuses: st})
	if _, err := s.CreateGoal(context.Background(), domain.TenantScope{TenantID: 1}, goals.GoalInput{TeamID: 5, PeriodID: 2, Title: "ML-биддинг"}, 9); err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(fa.recorded) != 1 {
		t.Fatalf("want 1 event, got %d", len(fa.recorded))
	}
	ev := fa.recorded[0]
	if ev.Category != domain.ActivityComposition || ev.Action != domain.ActionGoalCreated || ev.EntityTitle != "ML-биддинг" {
		t.Fatalf("wrong event: %+v", ev)
	}
	if ev.TeamID == nil || *ev.TeamID != 5 {
		t.Fatalf("wrong team: %+v", ev)
	}
}

func TestDiffFieldsOnlyChanged(t *testing.T) {
	changed := diffFields(map[string][2]any{
		"title":       {"old", "new"},
		"description": {"same", "same"},
		"weight":      {10, 20},
	})
	if len(changed) != 2 {
		t.Fatalf("want 2 changed, got %d: %+v", len(changed), changed)
	}
	if _, ok := changed["description"]; ok {
		t.Fatalf("unchanged field leaked: %+v", changed)
	}
	if changed["title"].(map[string]any)["after"] != "new" {
		t.Fatalf("title diff wrong: %+v", changed["title"])
	}
	if len(diffFields(map[string][2]any{"a": {1, 1}})) != 0 {
		t.Fatalf("no-op edit should produce empty diff")
	}
}

func TestShareGoalRecordsAddedTeams(t *testing.T) {
	fa := &fakeActivityRepo{}
	st := newFakeStore()
	st.teams = []domain.Team{{ID: 2}, {ID: 4}, {ID: 21}}
	s := New(Deps{Activity: fa, Teams: st, Shares: st, Goals: st})
	// fakeStore.ListGoalShares returns empty, so both targets are newly added.
	if err := s.ShareGoal(context.Background(), domain.TenantScope{TenantID: 1}, 10,
		[]ShareTarget{{TeamID: 4, Weight: 50}, {TeamID: 21, Weight: 50}}, 5); err != nil {
		t.Fatalf("share: %v", err)
	}
	if len(fa.recorded) != 1 || fa.recorded[0].Action != domain.ActionGoalShared {
		t.Fatalf("want 1 goal_shared, got %+v", fa.recorded)
	}
	ids := fa.recorded[0].Payload["shared_with_team_ids"].([]int64)
	if len(ids) != 2 {
		t.Fatalf("added teams wrong: %+v", ids)
	}
}

func TestDeleteGoalByOwnerWithSharesRecordsOwnerChange(t *testing.T) {
	fa := &fakeActivityRepo{}
	gf := &goalFakeStore{
		goals:      map[int64]domain.Goal{7: {ID: 7, TeamID: 2, PeriodID: 3, Title: "Общая цель"}},
		goalShares: map[int64][]shares.GoalShare{7: {{GoalID: 7, TeamID: 4, Weight: 50}}},
	}
	s := New(Deps{Activity: fa, Goals: gf, Shares: gf, Statuses: gf})
	// Owner (team 2) "deletes" a goal that has a shared team → ownership transfers to team 4.
	if _, _, err := s.DeleteGoal(context.Background(), domain.TenantScope{TenantID: 1}, 7, 2, 9); err != nil {
		t.Fatalf("delete: %v", err)
	}
	var found *domain.ActivityEvent
	for i := range fa.recorded {
		if fa.recorded[i].Action == domain.ActionGoalOwnerChanged {
			found = &fa.recorded[i]
		}
	}
	if found == nil {
		t.Fatalf("no goal_owner_changed recorded: %+v", fa.recorded)
	}
	if found.Payload["before"].(map[string]any)["owner_team_id"].(int64) != 2 ||
		found.Payload["after"].(map[string]any)["owner_team_id"].(int64) != 4 {
		t.Fatalf("owner change payload wrong: %+v", found.Payload)
	}
}

func TestLeaveSharedGoalRecordsEvent(t *testing.T) {
	fa := &fakeActivityRepo{}
	gf := &goalFakeStore{goals: map[int64]domain.Goal{7: {ID: 7, TeamID: 2, PeriodID: 3, Title: "Общая цель"}}}
	s := New(Deps{Activity: fa, Goals: gf, Shares: gf, Statuses: gf})
	// team 5 (a sharee, not owner 2) leaves the shared goal.
	if _, _, err := s.DeleteGoal(context.Background(), domain.TenantScope{TenantID: 1}, 7, 5, 9); err != nil {
		t.Fatalf("leave share: %v", err)
	}
	if len(fa.recorded) != 1 {
		t.Fatalf("want 1 event, got %d", len(fa.recorded))
	}
	ev := fa.recorded[0]
	if ev.Category != domain.ActivityComposition || ev.Action != domain.ActionGoalUnshared || ev.EntityTitle != "Общая цель" {
		t.Fatalf("wrong event: %+v", ev)
	}
	if ev.TeamID == nil || *ev.TeamID != 2 {
		t.Fatalf("should anchor to owner team 2: %+v", ev)
	}
	if int(ev.Payload["declined_by_team_id"].(int64)) != 5 {
		t.Fatalf("declined_by wrong: %+v", ev.Payload)
	}
}

func TestRecordActivityIsBestEffort(t *testing.T) {
	fa := &fakeActivityRepo{failNext: true}
	s := New(Deps{Activity: fa}) // logger nil → must not panic
	s.recordActivity(context.Background(), domain.TenantScope{TenantID: 1}, domain.ActivityEvent{Action: domain.ActionStatusChanged})
	if len(fa.recorded) != 0 {
		t.Fatalf("expected no recorded event on failure")
	}
	fa.failNext = false
	s.recordActivity(context.Background(), domain.TenantScope{TenantID: 1}, domain.ActivityEvent{Action: domain.ActionStatusChanged})
	if len(fa.recorded) != 1 {
		t.Fatalf("expected 1 recorded event, got %d", len(fa.recorded))
	}
}
