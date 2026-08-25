package unresolve

// Пакет отличается от парного только булевым флагом, поэтому проверяется, что
// в usecase уходит именно он, плюс гейты общего тела SetCommentResolved.

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/api/v1/goals/goalcommon"
	"okrs/internal/http/handlers/handlertest"
	storegoals "okrs/internal/store/goals"
	"okrs/internal/store/shares"
)

type fakeGoals struct{ goal domain.Goal }

func (f *fakeGoals) Get(context.Context, domain.TenantScope, int64) (domain.Goal, error) {
	return f.goal, nil
}

type fakeShares struct{}

func (f *fakeShares) List(context.Context, domain.TenantScope, int64) ([]shares.GoalShare, error) {
	return nil, nil
}
func (f *fakeShares) Get(context.Context, domain.TenantScope, int64, int64) (shares.GoalShare, error) {
	return shares.GoalShare{}, errors.New("no share")
}

type fakeUC struct {
	got   bool
	calls int
	err   error
}

func (f *fakeUC) SetCommentResolved(_ context.Context, _ domain.TenantScope, _, _ int64, resolved bool, _ int64) error {
	f.calls++
	f.got = resolved
	return f.err
}

const uri = "/api/v1/goals/1/comments/2/state"

func deps(uc *fakeUC) goalcommon.ResolveDeps {
	return goalcommon.ResolveDeps{
		Goals: &fakeGoals{goal: domain.Goal{ID: 1, TeamID: 5}}, Shares: &fakeShares{}, UC: uc,
	}
}

func TestBadGoalIDIs400(t *testing.T) {
	w := handlertest.Do(New(deps(&fakeUC{})).Post, http.MethodPost, uri, "",
		handlertest.Tenant(1), handlertest.URLParam("goalID", "не-число"), handlertest.URLParam("commentID", "2"))
	handlertest.IsError(t, w, http.StatusBadRequest)
}

func TestBadCommentIDIs400(t *testing.T) {
	w := handlertest.Do(New(deps(&fakeUC{})).Post, http.MethodPost, uri, "",
		handlertest.Tenant(1), handlertest.URLParam("goalID", "1"), handlertest.URLParam("commentID", "не-число"))
	handlertest.IsError(t, w, http.StatusBadRequest)
}

func TestRequiresTenant(t *testing.T) {
	w := handlertest.Do(New(deps(&fakeUC{})).Post, http.MethodPost, uri, "",
		handlertest.URLParam("goalID", "1"), handlertest.URLParam("commentID", "2"))
	handlertest.IsError(t, w, http.StatusForbidden)
}

// Доступ к комментарию определяется доступом к цели: чужая цель — 404.
func TestInaccessibleGoalIs404(t *testing.T) {
	w := handlertest.Do(New(deps(&fakeUC{})).Post, http.MethodPost, uri, "",
		handlertest.Tenant(1), handlertest.AllowedTeams([]int64{9}),
		handlertest.URLParam("goalID", "1"), handlertest.URLParam("commentID", "2"))
	handlertest.IsError(t, w, http.StatusNotFound)
}

func TestSetsPackageFlag(t *testing.T) {
	uc := &fakeUC{}
	w := handlertest.Do(New(deps(uc)).Post, http.MethodPost, uri, "",
		handlertest.Tenant(1), handlertest.URLParam("goalID", "1"), handlertest.URLParam("commentID", "2"))
	handlertest.Status(t, w, http.StatusOK)
	if uc.calls != 1 {
		t.Fatalf("SetCommentResolved вызван %d раз, want 1", uc.calls)
	}
	if uc.got != false {
		t.Fatalf("resolved = %v, want %v", uc.got, false)
	}
}

// Несуществующий комментарий у существующей цели — 404, а не 500.
func TestMissingCommentIs404(t *testing.T) {
	uc := &fakeUC{err: storegoals.ErrNotFound}
	w := handlertest.Do(New(deps(uc)).Post, http.MethodPost, uri, "",
		handlertest.Tenant(1), handlertest.URLParam("goalID", "1"), handlertest.URLParam("commentID", "2"))
	handlertest.IsError(t, w, http.StatusNotFound)
}
