package categorycounts

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/handlertest"
	activitysvc "okrs/internal/service/activity"
)

type fakeCounter struct {
	counts map[string]int
	err    error
	gotF   activitysvc.Filter
	gotIDs []int64
}

func (f *fakeCounter) CountByCategory(_ context.Context, _ domain.TenantScope, ids []int64, flt activitysvc.Filter) (map[string]int, error) {
	f.gotIDs, f.gotF = ids, flt
	return f.counts, f.err
}

func TestForbiddenWithoutTenant(t *testing.T) {
	handlertest.RequiresTenantScope(t, New(&fakeCounter{}).Get, http.MethodGet, "/api/v1/activity/category-counts")
}

// total считается на стороне обработчика — клиент рисует его на вкладке «все».
func TestSumsTotal(t *testing.T) {
	h := New(&fakeCounter{counts: map[string]int{"goal": 3, "kr": 4}})
	w := handlertest.Do(h.Get, http.MethodGet, "/api/v1/activity/category-counts", "", handlertest.Tenant(1))
	handlertest.Status(t, w, http.StatusOK)
	var resp struct {
		Counts map[string]int `json:"counts"`
		Total  int            `json:"total"`
	}
	handlertest.DecodeJSON(t, w, &resp)
	if resp.Total != 7 {
		t.Fatalf("total = %d, want 7", resp.Total)
	}
	if resp.Counts["goal"] != 3 {
		t.Fatalf("counts = %v", resp.Counts)
	}
}

// Ограничение видимости обязано доехать до сервиса: без него счётчики покажут
// события команд, которых пользователь не видит.
func TestPassesScopeAndFilter(t *testing.T) {
	fc := &fakeCounter{counts: map[string]int{}}
	h := New(fc)
	w := handlertest.Do(h.Get, http.MethodGet, "/api/v1/activity/category-counts?period_id=5&actor_udid=u-9", "",
		handlertest.Tenant(1), handlertest.AllowedTeams([]int64{2, 3}))
	handlertest.Status(t, w, http.StatusOK)
	if len(fc.gotIDs) != 2 {
		t.Fatalf("allowedTeamIDs = %v, want [2 3]", fc.gotIDs)
	}
	if fc.gotF.PeriodID == nil || *fc.gotF.PeriodID != 5 || fc.gotF.ActorUDID != "u-9" {
		t.Fatalf("фильтр не доехал: %+v", fc.gotF)
	}
}

func TestServiceErrorIs500(t *testing.T) {
	h := New(&fakeCounter{err: errors.New("boom")})
	w := handlertest.Do(h.Get, http.MethodGet, "/api/v1/activity/category-counts", "", handlertest.Tenant(1))
	handlertest.ErrorCode(t, w, http.StatusInternalServerError, "INTERNAL")
}
