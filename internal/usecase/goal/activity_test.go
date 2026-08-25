package goal

// Тесты переехали из internal/service при выделении слоя usecase.

import (
	"context"
	"errors"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/service/servicetest"
	"okrs/internal/store/goals"
	"okrs/internal/store/shares"
)

func TestAddGoalCommentRecordsEvent(t *testing.T) {
	fa := &servicetest.ActivityRepo{}
	gf := &servicetest.GoalStore{Goals: map[int64]domain.Goal{7: {ID: 7, TeamID: 42, PeriodID: 3, Title: "P95"}}}
	s := newFromRepos(rawDeps{Activity: fa, Goals: gf})
	if err := s.AddComment(context.Background(), domain.TenantScope{TenantID: 1}, 7, "blocker", 5); err != nil {
		t.Fatalf("AddGoalComment: %v", err)
	}
	if len(fa.Recorded) != 1 {
		t.Fatalf("want 1 event, got %d", len(fa.Recorded))
	}
	ev := fa.Recorded[0]
	if ev.Category != domain.ActivityDiscussion || ev.Action != domain.ActionCommentAdded {
		t.Fatalf("wrong event: %+v", ev)
	}
	if ev.TeamID == nil || *ev.TeamID != 42 || ev.EntityTitle != "P95" || ev.Payload["text"] != "blocker" {
		t.Fatalf("event fields wrong: %+v", ev)
	}
}

func TestSetGoalCommentResolvedRecordsEvent(t *testing.T) {
	fa := &servicetest.ActivityRepo{}
	gf := &servicetest.GoalStore{Goals: map[int64]domain.Goal{7: {ID: 7, TeamID: 42, PeriodID: 3, Title: "P95"}}}
	s := newFromRepos(rawDeps{Activity: fa, Goals: gf})
	if err := s.SetCommentResolved(context.Background(), domain.TenantScope{TenantID: 1}, 7, 11, true, 5); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(fa.Recorded) != 1 || fa.Recorded[0].Action != domain.ActionCommentResolved {
		t.Fatalf("wrong resolve event: %+v", fa.Recorded)
	}
	fa.Recorded = nil
	if err := s.SetCommentResolved(context.Background(), domain.TenantScope{TenantID: 1}, 7, 11, false, 5); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if len(fa.Recorded) != 1 || fa.Recorded[0].Action != domain.ActionCommentReopened {
		t.Fatalf("wrong reopen event: %+v", fa.Recorded)
	}
}

func TestAddGoalReplyRecordsReplyAddedEvent(t *testing.T) {
	fa := &servicetest.ActivityRepo{}
	gf := &servicetest.GoalStore{Goals: map[int64]domain.Goal{7: {ID: 7, TeamID: 42, PeriodID: 3, Title: "P95"}}}
	s := newFromRepos(rawDeps{Activity: fa, Goals: gf})
	if err := s.AddReply(context.Background(), domain.TenantScope{TenantID: 1}, 7, 11, "a reply", 5); err != nil {
		t.Fatalf("AddGoalReply: %v", err)
	}
	if len(fa.Recorded) != 1 {
		t.Fatalf("want 1 event, got %d", len(fa.Recorded))
	}
	ev := fa.Recorded[0]
	if ev.Category != domain.ActivityDiscussion || ev.Action != domain.ActionReplyAdded {
		t.Fatalf("wrong event: %+v", ev)
	}
	if ev.TeamID == nil || *ev.TeamID != 42 || ev.EntityTitle != "P95" || ev.Payload["text"] != "a reply" {
		t.Fatalf("event fields wrong: %+v", ev)
	}
}

func TestAddGoalReplyBadParentNoEvent(t *testing.T) {
	fa := &servicetest.ActivityRepo{}
	gf := &servicetest.GoalStore{Goals: map[int64]domain.Goal{7: {ID: 7}}, AddReplyErr: goals.ErrNotFound}
	s := newFromRepos(rawDeps{Activity: fa, Goals: gf})
	if err := s.AddReply(context.Background(), domain.TenantScope{TenantID: 1}, 7, 999, "orphan", 5); !errors.Is(err, goals.ErrNotFound) {
		t.Fatalf("want domain.ErrNotFound, got %v", err)
	}
	if len(fa.Recorded) != 0 {
		t.Fatalf("no event expected on failed reply, got %d", len(fa.Recorded))
	}
}

func TestDeleteGoalCommentForbiddenForNonAuthorNonAdmin(t *testing.T) {
	fa := &servicetest.ActivityRepo{}
	gf := &servicetest.GoalStore{Goals: map[int64]domain.Goal{7: {ID: 7, TeamID: 42, PeriodID: 3, Title: "P95"}}, CommentAuthor: 2, CommentIsTask: true}
	s := newFromRepos(rawDeps{Activity: fa, Goals: gf})
	// requesting user 1, author is 2, not admin → forbidden.
	if _, err := s.DeleteComment(context.Background(), domain.TenantScope{TenantID: 1}, 7, 11, 1, false); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("want domain.ErrForbidden, got %v", err)
	}
	if len(gf.DeleteCommentCalls) != 0 || len(fa.Recorded) != 0 {
		t.Fatalf("no delete/event expected on forbidden: deletes=%v events=%d", gf.DeleteCommentCalls, len(fa.Recorded))
	}
}

func TestDeleteGoalCommentAdminDeletesReplyLogsReplyDeleted(t *testing.T) {
	fa := &servicetest.ActivityRepo{}
	gf := &servicetest.GoalStore{Goals: map[int64]domain.Goal{7: {ID: 7, TeamID: 42, PeriodID: 3, Title: "P95"}}, CommentAuthor: 2, CommentIsTask: false}
	s := newFromRepos(rawDeps{Activity: fa, Goals: gf})
	// requesting user 1 is admin, author is 2, target is a reply.
	isTask, err := s.DeleteComment(context.Background(), domain.TenantScope{TenantID: 1}, 7, 11, 1, true)
	if err != nil || isTask {
		t.Fatalf("admin delete reply: isTask=%v err=%v", isTask, err)
	}
	if len(gf.DeleteCommentCalls) != 1 {
		t.Fatalf("delete must be called once, got %d", len(gf.DeleteCommentCalls))
	}
	if len(fa.Recorded) != 1 || fa.Recorded[0].Action != domain.ActionReplyDeleted {
		t.Fatalf("want reply_deleted, got %+v", fa.Recorded)
	}
}

func TestDeleteGoalCommentAuthorDeletesTaskLogsCommentDeleted(t *testing.T) {
	fa := &servicetest.ActivityRepo{}
	gf := &servicetest.GoalStore{Goals: map[int64]domain.Goal{7: {ID: 7, TeamID: 42, PeriodID: 3, Title: "P95"}}, CommentAuthor: 5, CommentIsTask: true}
	s := newFromRepos(rawDeps{Activity: fa, Goals: gf})
	isTask, err := s.DeleteComment(context.Background(), domain.TenantScope{TenantID: 1}, 7, 11, 5, false)
	if err != nil || !isTask {
		t.Fatalf("author delete task: isTask=%v err=%v", isTask, err)
	}
	if len(fa.Recorded) != 1 || fa.Recorded[0].Action != domain.ActionCommentDeleted {
		t.Fatalf("want comment_deleted, got %+v", fa.Recorded)
	}
}

func TestDeleteGoalCommentMissingReturnsNotFound(t *testing.T) {
	fa := &servicetest.ActivityRepo{}
	gf := &servicetest.GoalStore{Goals: map[int64]domain.Goal{7: {ID: 7}}, CommentMetaErr: goals.ErrNotFound}
	s := newFromRepos(rawDeps{Activity: fa, Goals: gf})
	if _, err := s.DeleteComment(context.Background(), domain.TenantScope{TenantID: 1}, 7, 11, 5, false); !errors.Is(err, goals.ErrNotFound) {
		t.Fatalf("want domain.ErrNotFound, got %v", err)
	}
	if len(fa.Recorded) != 0 {
		t.Fatalf("no event expected when comment missing, got %d", len(fa.Recorded))
	}
}

func TestUpdateGoalRecordsFieldChange(t *testing.T) {
	fa := &servicetest.ActivityRepo{}
	gf := &servicetest.GoalStore{Goals: map[int64]domain.Goal{7: {ID: 7, TeamID: 42, PeriodID: 3, Title: "старое"}}}
	s := newFromRepos(rawDeps{Activity: fa, Goals: gf})
	if err := s.Update(context.Background(), domain.TenantScope{TenantID: 1},
		goals.GoalUpdateInput{ID: 7, Title: "новое", Priority: domain.PriorityP1, Weight: 100}, 5); err != nil {
		t.Fatalf("update goal: %v", err)
	}
	if len(fa.Recorded) != 1 {
		t.Fatalf("want 1 event, got %d", len(fa.Recorded))
	}
	ev := fa.Recorded[0]
	if ev.Action != domain.ActionGoalFieldsChanged {
		t.Fatalf("wrong action: %+v", ev)
	}
	changed := ev.Payload["changed"].(map[string]any)
	if changed["title"].(map[string]any)["after"] != "новое" {
		t.Fatalf("title change not captured: %+v", changed)
	}
}

func TestCreateGoalRecordsEvent(t *testing.T) {
	fa := &servicetest.ActivityRepo{}
	st := servicetest.NewStore()
	s := newFromRepos(rawDeps{Activity: fa, Goals: st, Statuses: st})
	if _, err := s.Create(context.Background(), domain.TenantScope{TenantID: 1}, goals.GoalInput{TeamID: 5, PeriodID: 2, Title: "ML-биддинг"}, 9); err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(fa.Recorded) != 1 {
		t.Fatalf("want 1 event, got %d", len(fa.Recorded))
	}
	ev := fa.Recorded[0]
	if ev.Category != domain.ActivityComposition || ev.Action != domain.ActionGoalCreated || ev.EntityTitle != "ML-биддинг" {
		t.Fatalf("wrong event: %+v", ev)
	}
	if ev.TeamID == nil || *ev.TeamID != 5 {
		t.Fatalf("wrong team: %+v", ev)
	}
}

func TestShareGoalRecordsAddedTeams(t *testing.T) {
	fa := &servicetest.ActivityRepo{}
	st := servicetest.NewStore()
	st.Teams = []domain.Team{{ID: 2}, {ID: 4}, {ID: 21}}
	s := newFromRepos(rawDeps{Activity: fa, Teams: st, Shares: st, Goals: st, Statuses: st})
	// servicetest.Store.ListGoalShares returns empty, so both targets are newly added.
	if err := s.Share(context.Background(), domain.TenantScope{TenantID: 1}, 10,
		[]ShareTarget{{TeamID: 4, Weight: 50}, {TeamID: 21, Weight: 50}}, 5); err != nil {
		t.Fatalf("share: %v", err)
	}
	if len(fa.Recorded) != 1 || fa.Recorded[0].Action != domain.ActionGoalShared {
		t.Fatalf("want 1 goal_shared, got %+v", fa.Recorded)
	}
	ids := fa.Recorded[0].Payload["shared_with_team_ids"].([]int64)
	if len(ids) != 2 {
		t.Fatalf("added teams wrong: %+v", ids)
	}
}

func TestDeleteGoalByOwnerWithSharesRecordsOwnerChange(t *testing.T) {
	fa := &servicetest.ActivityRepo{}
	gf := &servicetest.GoalStore{
		Goals:      map[int64]domain.Goal{7: {ID: 7, TeamID: 2, PeriodID: 3, Title: "Общая цель"}},
		GoalShares: map[int64][]shares.GoalShare{7: {{GoalID: 7, TeamID: 4, Weight: 50}}},
	}
	s := newFromRepos(rawDeps{Activity: fa, Goals: gf, Shares: gf, Statuses: gf})
	// Owner (team 2) "deletes" a goal that has a shared team → ownership transfers to team 4.
	if _, _, err := s.Delete(context.Background(), domain.TenantScope{TenantID: 1}, 7, 2, 9); err != nil {
		t.Fatalf("delete: %v", err)
	}
	var found *domain.ActivityEvent
	for i := range fa.Recorded {
		if fa.Recorded[i].Action == domain.ActionGoalOwnerChanged {
			found = &fa.Recorded[i]
		}
	}
	if found == nil {
		t.Fatalf("no goal_owner_changed recorded: %+v", fa.Recorded)
	}
	if found.Payload["before"].(map[string]any)["owner_team_id"].(int64) != 2 ||
		found.Payload["after"].(map[string]any)["owner_team_id"].(int64) != 4 {
		t.Fatalf("owner change payload wrong: %+v", found.Payload)
	}
}

func TestLeaveSharedGoalRecordsEvent(t *testing.T) {
	fa := &servicetest.ActivityRepo{}
	gf := &servicetest.GoalStore{Goals: map[int64]domain.Goal{7: {ID: 7, TeamID: 2, PeriodID: 3, Title: "Общая цель"}}}
	s := newFromRepos(rawDeps{Activity: fa, Goals: gf, Shares: gf, Statuses: gf})
	// team 5 (a sharee, not owner 2) leaves the shared goal.
	if _, _, err := s.Delete(context.Background(), domain.TenantScope{TenantID: 1}, 7, 5, 9); err != nil {
		t.Fatalf("leave share: %v", err)
	}
	if len(fa.Recorded) != 1 {
		t.Fatalf("want 1 event, got %d", len(fa.Recorded))
	}
	ev := fa.Recorded[0]
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
