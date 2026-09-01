package goal

// Тесты переехали из internal/service при выделении слоя usecase, затем — с журнала на
// шину: сценарий теперь публикует событие, а не журнальную строку, поэтому тесты
// проверяют event.Event через FakeBus, а не domain.ActivityEvent через ActivityRepo.

import (
	"context"
	"errors"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/core/event"
	"okrs/internal/service/servicetest"
	"okrs/internal/store/goals"
	"okrs/internal/store/shares"
)

func TestAddGoalCommentRecordsEvent(t *testing.T) {
	bus := &servicetest.FakeBus{}
	gf := &servicetest.GoalStore{Goals: map[int64]domain.Goal{7: {ID: 7, TeamID: 42, PeriodID: 3, Title: "P95"}}}
	s := newFromRepos(rawDeps{Events: bus, Goals: gf})
	if err := s.AddComment(context.Background(), domain.TenantScope{TenantID: 1}, 7, "blocker", 5); err != nil {
		t.Fatalf("AddGoalComment: %v", err)
	}
	if len(bus.Events) != 1 {
		t.Fatalf("want 1 event, got %d", len(bus.Events))
	}
	ev, ok := bus.Events[0].(event.CommentAdded)
	if !ok {
		t.Fatalf("wrong event type: %+v", bus.Events[0])
	}
	if ev.TeamID == nil || *ev.TeamID != 42 || ev.GoalTitle != "P95" || ev.Text != "blocker" || ev.GoalID != 7 {
		t.Fatalf("event fields wrong: %+v", ev)
	}
}

func TestSetGoalCommentResolvedRecordsEvent(t *testing.T) {
	bus := &servicetest.FakeBus{}
	gf := &servicetest.GoalStore{Goals: map[int64]domain.Goal{7: {ID: 7, TeamID: 42, PeriodID: 3, Title: "P95"}}, CommentAuthor: 9}
	s := newFromRepos(rawDeps{Events: bus, Goals: gf})
	if err := s.SetCommentResolved(context.Background(), domain.TenantScope{TenantID: 1}, 7, 11, true, 5); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(bus.Events) != 1 {
		t.Fatalf("want 1 event, got %+v", bus.Events)
	}
	resolved, ok := bus.Events[0].(event.CommentResolved)
	if !ok {
		t.Fatalf("wrong resolve event: %+v", bus.Events[0])
	}
	if resolved.AuthorUserID != 9 {
		t.Fatalf("resolve event must carry the task author (notification addressee): %+v", resolved)
	}
	bus.Events = nil
	if err := s.SetCommentResolved(context.Background(), domain.TenantScope{TenantID: 1}, 7, 11, false, 5); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if len(bus.Events) != 1 {
		t.Fatalf("want 1 event, got %+v", bus.Events)
	}
	if _, ok := bus.Events[0].(event.CommentReopened); !ok {
		t.Fatalf("wrong reopen event: %+v", bus.Events[0])
	}
}

func TestAddGoalReplyRecordsReplyAddedEvent(t *testing.T) {
	bus := &servicetest.FakeBus{}
	gf := &servicetest.GoalStore{Goals: map[int64]domain.Goal{7: {ID: 7, TeamID: 42, PeriodID: 3, Title: "P95"}}}
	s := newFromRepos(rawDeps{Events: bus, Goals: gf})
	if err := s.AddReply(context.Background(), domain.TenantScope{TenantID: 1}, 7, 11, "a reply", 5); err != nil {
		t.Fatalf("AddGoalReply: %v", err)
	}
	if len(bus.Events) != 1 {
		t.Fatalf("want 1 event, got %d", len(bus.Events))
	}
	ev, ok := bus.Events[0].(event.ReplyAdded)
	if !ok {
		t.Fatalf("wrong event type: %+v", bus.Events[0])
	}
	if ev.TeamID == nil || *ev.TeamID != 42 || ev.GoalTitle != "P95" || ev.Text != "a reply" || ev.ParentCommentID != 11 {
		t.Fatalf("event fields wrong: %+v", ev)
	}
}

func TestAddGoalReplyBadParentNoEvent(t *testing.T) {
	bus := &servicetest.FakeBus{}
	gf := &servicetest.GoalStore{Goals: map[int64]domain.Goal{7: {ID: 7}}, AddReplyErr: goals.ErrNotFound}
	s := newFromRepos(rawDeps{Events: bus, Goals: gf})
	if err := s.AddReply(context.Background(), domain.TenantScope{TenantID: 1}, 7, 999, "orphan", 5); !errors.Is(err, goals.ErrNotFound) {
		t.Fatalf("want domain.ErrNotFound, got %v", err)
	}
	if len(bus.Events) != 0 {
		t.Fatalf("no event expected on failed reply, got %d", len(bus.Events))
	}
}

func TestDeleteGoalCommentForbiddenForNonAuthorNonAdmin(t *testing.T) {
	bus := &servicetest.FakeBus{}
	gf := &servicetest.GoalStore{Goals: map[int64]domain.Goal{7: {ID: 7, TeamID: 42, PeriodID: 3, Title: "P95"}}, CommentAuthor: 2, CommentIsTask: true}
	s := newFromRepos(rawDeps{Events: bus, Goals: gf})
	// requesting user 1, author is 2, not admin → forbidden.
	if _, err := s.DeleteComment(context.Background(), domain.TenantScope{TenantID: 1}, 7, 11, 1, false); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("want domain.ErrForbidden, got %v", err)
	}
	if len(gf.DeleteCommentCalls) != 0 || len(bus.Events) != 0 {
		t.Fatalf("no delete/event expected on forbidden: deletes=%v events=%d", gf.DeleteCommentCalls, len(bus.Events))
	}
}

func TestDeleteGoalCommentAdminDeletesReplyLogsReplyDeleted(t *testing.T) {
	bus := &servicetest.FakeBus{}
	gf := &servicetest.GoalStore{Goals: map[int64]domain.Goal{7: {ID: 7, TeamID: 42, PeriodID: 3, Title: "P95"}}, CommentAuthor: 2, CommentIsTask: false}
	s := newFromRepos(rawDeps{Events: bus, Goals: gf})
	// requesting user 1 is admin, author is 2, target is a reply.
	isTask, err := s.DeleteComment(context.Background(), domain.TenantScope{TenantID: 1}, 7, 11, 1, true)
	if err != nil || isTask {
		t.Fatalf("admin delete reply: isTask=%v err=%v", isTask, err)
	}
	if len(gf.DeleteCommentCalls) != 1 {
		t.Fatalf("delete must be called once, got %d", len(gf.DeleteCommentCalls))
	}
	if len(bus.Events) != 1 {
		t.Fatalf("want 1 event, got %+v", bus.Events)
	}
	if _, ok := bus.Events[0].(event.ReplyDeleted); !ok {
		t.Fatalf("want reply_deleted, got %+v", bus.Events[0])
	}
}

func TestDeleteGoalCommentAuthorDeletesTaskLogsCommentDeleted(t *testing.T) {
	bus := &servicetest.FakeBus{}
	gf := &servicetest.GoalStore{Goals: map[int64]domain.Goal{7: {ID: 7, TeamID: 42, PeriodID: 3, Title: "P95"}}, CommentAuthor: 5, CommentIsTask: true}
	s := newFromRepos(rawDeps{Events: bus, Goals: gf})
	isTask, err := s.DeleteComment(context.Background(), domain.TenantScope{TenantID: 1}, 7, 11, 5, false)
	if err != nil || !isTask {
		t.Fatalf("author delete task: isTask=%v err=%v", isTask, err)
	}
	if len(bus.Events) != 1 {
		t.Fatalf("want 1 event, got %+v", bus.Events)
	}
	if _, ok := bus.Events[0].(event.CommentDeleted); !ok {
		t.Fatalf("want comment_deleted, got %+v", bus.Events[0])
	}
}

func TestDeleteGoalCommentMissingReturnsNotFound(t *testing.T) {
	bus := &servicetest.FakeBus{}
	gf := &servicetest.GoalStore{Goals: map[int64]domain.Goal{7: {ID: 7}}, CommentMetaErr: goals.ErrNotFound}
	s := newFromRepos(rawDeps{Events: bus, Goals: gf})
	if _, err := s.DeleteComment(context.Background(), domain.TenantScope{TenantID: 1}, 7, 11, 5, false); !errors.Is(err, goals.ErrNotFound) {
		t.Fatalf("want domain.ErrNotFound, got %v", err)
	}
	if len(bus.Events) != 0 {
		t.Fatalf("no event expected when comment missing, got %d", len(bus.Events))
	}
}

func TestUpdateGoalRecordsFieldChange(t *testing.T) {
	bus := &servicetest.FakeBus{}
	gf := &servicetest.GoalStore{Goals: map[int64]domain.Goal{7: {ID: 7, TeamID: 42, PeriodID: 3, Title: "старое"}}}
	s := newFromRepos(rawDeps{Events: bus, Goals: gf})
	if err := s.Update(context.Background(), domain.TenantScope{TenantID: 1},
		goals.GoalUpdateInput{ID: 7, Title: "новое", Priority: domain.PriorityP1, Weight: 100}, 5); err != nil {
		t.Fatalf("update goal: %v", err)
	}
	if len(bus.Events) != 1 {
		t.Fatalf("want 1 event, got %d", len(bus.Events))
	}
	ev, ok := bus.Events[0].(event.GoalFieldsChanged)
	if !ok {
		t.Fatalf("wrong event type: %+v", bus.Events[0])
	}
	title, ok := ev.Changed["title"]
	if !ok || title[1] != "новое" {
		t.Fatalf("title change not captured: %+v", ev.Changed)
	}
}

func TestCreateGoalRecordsEvent(t *testing.T) {
	bus := &servicetest.FakeBus{}
	st := servicetest.NewStore()
	s := newFromRepos(rawDeps{Events: bus, Goals: st, Statuses: st})
	if _, err := s.Create(context.Background(), domain.TenantScope{TenantID: 1}, goals.GoalInput{TeamID: 5, PeriodID: 2, Title: "ML-биддинг"}, 9); err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(bus.Events) != 1 {
		t.Fatalf("want 1 event, got %d", len(bus.Events))
	}
	ev, ok := bus.Events[0].(event.GoalCreated)
	if !ok {
		t.Fatalf("wrong event type: %+v", bus.Events[0])
	}
	if ev.Title != "ML-биддинг" {
		t.Fatalf("wrong event: %+v", ev)
	}
	if ev.TeamID == nil || *ev.TeamID != 5 {
		t.Fatalf("wrong team: %+v", ev)
	}
}

func TestShareGoalRecordsAddedTeams(t *testing.T) {
	bus := &servicetest.FakeBus{}
	st := servicetest.NewStore()
	st.Teams = []domain.Team{{ID: 2}, {ID: 4}, {ID: 21}}
	s := newFromRepos(rawDeps{Events: bus, Teams: st, Shares: st, Goals: st, Statuses: st})
	// servicetest.Store.ListGoalShares returns empty, so both targets are newly added.
	if err := s.Share(context.Background(), domain.TenantScope{TenantID: 1}, 10,
		[]ShareTarget{{TeamID: 4, Weight: 50}, {TeamID: 21, Weight: 50}}, 5); err != nil {
		t.Fatalf("share: %v", err)
	}
	if len(bus.Events) != 1 {
		t.Fatalf("want 1 goal_shared, got %+v", bus.Events)
	}
	ev, ok := bus.Events[0].(event.GoalShared)
	if !ok {
		t.Fatalf("wrong event type: %+v", bus.Events[0])
	}
	if len(ev.SharedWithTeamIDs) != 2 {
		t.Fatalf("added teams wrong: %+v", ev.SharedWithTeamIDs)
	}
}

func TestDeleteGoalByOwnerWithSharesRecordsOwnerChange(t *testing.T) {
	bus := &servicetest.FakeBus{}
	gf := &servicetest.GoalStore{
		Goals:      map[int64]domain.Goal{7: {ID: 7, TeamID: 2, PeriodID: 3, Title: "Общая цель"}},
		GoalShares: map[int64][]shares.GoalShare{7: {{GoalID: 7, TeamID: 4, Weight: 50}}},
	}
	s := newFromRepos(rawDeps{Events: bus, Goals: gf, Shares: gf, Statuses: gf})
	// Owner (team 2) "deletes" a goal that has a shared team → ownership transfers to team 4.
	if _, _, err := s.Delete(context.Background(), domain.TenantScope{TenantID: 1}, 7, 2, 9); err != nil {
		t.Fatalf("delete: %v", err)
	}
	var found *event.GoalOwnerChanged
	for i := range bus.Events {
		if ev, ok := bus.Events[i].(event.GoalOwnerChanged); ok {
			found = &ev
		}
	}
	if found == nil {
		t.Fatalf("no goal_owner_changed recorded: %+v", bus.Events)
	}
	if found.BeforeTeamID != 2 || found.AfterTeamID != 4 {
		t.Fatalf("owner change payload wrong: %+v", found)
	}
}

func TestLeaveSharedGoalRecordsEvent(t *testing.T) {
	bus := &servicetest.FakeBus{}
	gf := &servicetest.GoalStore{Goals: map[int64]domain.Goal{7: {ID: 7, TeamID: 2, PeriodID: 3, Title: "Общая цель"}}}
	s := newFromRepos(rawDeps{Events: bus, Goals: gf, Shares: gf, Statuses: gf})
	// team 5 (a sharee, not owner 2) leaves the shared goal.
	if _, _, err := s.Delete(context.Background(), domain.TenantScope{TenantID: 1}, 7, 5, 9); err != nil {
		t.Fatalf("leave share: %v", err)
	}
	if len(bus.Events) != 1 {
		t.Fatalf("want 1 event, got %d", len(bus.Events))
	}
	ev, ok := bus.Events[0].(event.GoalUnshared)
	if !ok {
		t.Fatalf("wrong event type: %+v", bus.Events[0])
	}
	if ev.Title != "Общая цель" {
		t.Fatalf("wrong event: %+v", ev)
	}
	if ev.TeamID == nil || *ev.TeamID != 2 {
		t.Fatalf("should anchor to owner team 2: %+v", ev)
	}
	if ev.DeclinedByTeamID != 5 {
		t.Fatalf("declined_by wrong: %+v", ev)
	}
}
