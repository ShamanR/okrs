package movedown

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/api/v1/goals/goalcommon"
	"okrs/internal/http/handlers/handlertest"
	"okrs/internal/store/shares"
)

type fakeGoals struct{ goal domain.Goal }

func (f *fakeGoals) Get(context.Context, domain.TenantScope, int64) (domain.Goal, error) {
	return f.goal, nil
}

type fakeShares struct{ list []shares.GoalShare }

func (f *fakeShares) List(context.Context, domain.TenantScope, int64) ([]shares.GoalShare, error) {
	return f.list, nil
}
func (f *fakeShares) Get(_ context.Context, _ domain.TenantScope, _, teamID int64) (shares.GoalShare, error) {
	for _, s := range f.list {
		if s.TeamID == teamID {
			return s, nil
		}
	}
	return shares.GoalShare{}, errors.New("no share")
}

type fakeMover struct {
	calls  int
	gotDir int
	gotIDs [2]int64
}

func (f *fakeMover) Move(_ context.Context, _ domain.TenantScope, teamID, goalID int64, direction int) error {
	f.calls++
	f.gotDir = direction
	f.gotIDs = [2]int64{goalID, teamID}
	return nil
}

func deps(goal domain.Goal, m *fakeMover) goalcommon.MoveDeps {
	return goalcommon.MoveDeps{Goals: &fakeGoals{goal: goal}, Shares: &fakeShares{}, Mover: m}
}

const uri = "/api/v1/goals/1/move"

func TestBadGoalIDIs400(t *testing.T) {
	h := New(deps(domain.Goal{ID: 1, TeamID: 5}, &fakeMover{}))
	w := handlertest.Do(h.Post, http.MethodPost, uri, `{"team_id":5}`,
		handlertest.Tenant(1), handlertest.URLParam("goalID", "не-число"))
	handlertest.ErrorCode(t, w, http.StatusBadRequest, "VALIDATION_ERROR")
}

func TestForbiddenWithoutTenant(t *testing.T) {
	h := New(deps(domain.Goal{ID: 1, TeamID: 5}, &fakeMover{}))
	w := handlertest.Do(h.Post, http.MethodPost, uri, `{"team_id":5}`, handlertest.URLParam("goalID", "1"))
	handlertest.ErrorCode(t, w, http.StatusForbidden, "FORBIDDEN")
}

// Порядок на доске у расшаренной цели свой в каждой команде, поэтому team_id
// обязателен: без него непонятно, чей список переупорядочивать.
func TestMissingTeamIDIs400(t *testing.T) {
	h := New(deps(domain.Goal{ID: 1, TeamID: 5}, &fakeMover{}))
	w := handlertest.Do(h.Post, http.MethodPost, uri, `{}`,
		handlertest.Tenant(1), handlertest.URLParam("goalID", "1"))
	handlertest.ErrorCode(t, w, http.StatusBadRequest, "VALIDATION_ERROR")
}

// Переставить порядок в чужой команде нельзя, даже зная её id.
func TestInaccessibleTeamIs403(t *testing.T) {
	h := New(deps(domain.Goal{ID: 1, TeamID: 5}, &fakeMover{}))
	w := handlertest.Do(h.Post, http.MethodPost, uri, `{"team_id":5}`,
		handlertest.Tenant(1), handlertest.AllowedTeams([]int64{9}), handlertest.URLParam("goalID", "1"))
	handlertest.ErrorCode(t, w, http.StatusForbidden, "FORBIDDEN")
}

// Этот пакет отличается от парного только направлением +1 (вниз) — оно и проверяется.
func TestMovesInPackageDirection(t *testing.T) {
	m := &fakeMover{}
	h := New(deps(domain.Goal{ID: 1, TeamID: 5}, m))
	w := handlertest.Do(h.Post, http.MethodPost, uri, `{"team_id":5}`,
		handlertest.Tenant(1), handlertest.URLParam("goalID", "1"))
	handlertest.Status(t, w, http.StatusOK)
	if m.calls != 1 {
		t.Fatalf("Move вызван %d раз, want 1", m.calls)
	}
	if m.gotDir != 1 {
		t.Fatalf("направление = %d, want %d", m.gotDir, 1)
	}
	if m.gotIDs != [2]int64{1, 5} {
		t.Fatalf("Move получил %v, want [1 5]", m.gotIDs)
	}
}
