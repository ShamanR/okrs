package moveup

// Пакет отличается от парного только направлением, поэтому проверяется, что
// именно это направление уходит в сервис, плюс гейты общего тела MoveKeyResult.

import (
	"context"
	"net/http"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/api/v1/krs/krscommon"
	"okrs/internal/http/handlers/handlertest"
)

type fakeKRs struct {
	kr     domain.KeyResult
	gotDir int
	calls  int
}

func (f *fakeKRs) Get(context.Context, domain.TenantScope, int64) (domain.KeyResult, error) {
	return f.kr, nil
}
func (f *fakeKRs) Move(_ context.Context, _ domain.TenantScope, _ int64, direction int) error {
	f.calls++
	f.gotDir = direction
	return nil
}

type fakeGoals struct{ goal domain.Goal }

func (f *fakeGoals) Get(context.Context, domain.TenantScope, int64) (domain.Goal, error) {
	return f.goal, nil
}

const uri = "/api/v1/krs/1/move"

func deps(krs *fakeKRs) krscommon.MoveDeps {
	return krscommon.MoveDeps{KRs: krs, Goals: &fakeGoals{goal: domain.Goal{ID: 2, TeamID: 5}}}
}

func TestBadKRIDIs400(t *testing.T) {
	w := handlertest.Do(New(deps(&fakeKRs{})).Post, http.MethodPost, uri, "",
		handlertest.Tenant(1), handlertest.URLParam("krID", "не-число"))
	handlertest.IsError(t, w, http.StatusBadRequest)
}

func TestRequiresTenant(t *testing.T) {
	w := handlertest.Do(New(deps(&fakeKRs{})).Post, http.MethodPost, uri, "",
		handlertest.URLParam("krID", "1"))
	handlertest.IsError(t, w, http.StatusForbidden)
}

// KR вне доступа отвечает 404, а не 403: иначе перебором id можно было бы
// выяснить, какие KR существуют в чужих командах.
func TestInaccessibleGoalIs404(t *testing.T) {
	w := handlertest.Do(New(deps(&fakeKRs{})).Post, http.MethodPost, uri, "",
		handlertest.Tenant(1), handlertest.AllowedTeams([]int64{9}), handlertest.URLParam("krID", "1"))
	handlertest.IsError(t, w, http.StatusNotFound)
}

func TestMovesInPackageDirection(t *testing.T) {
	krs := &fakeKRs{kr: domain.KeyResult{ID: 1, GoalID: 2}}
	w := handlertest.Do(New(deps(krs)).Post, http.MethodPost, uri, "",
		handlertest.Tenant(1), handlertest.URLParam("krID", "1"))
	handlertest.Status(t, w, http.StatusOK)
	if krs.calls != 1 {
		t.Fatalf("Move вызван %d раз, want 1", krs.calls)
	}
	if krs.gotDir != -1 {
		t.Fatalf("направление = %d, want %d", krs.gotDir, -1)
	}
}
