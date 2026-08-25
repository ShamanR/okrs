package treecounts

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/handlertest"
)

type fakeTree struct {
	counts    map[int64]int
	err       error
	gotPeriod *int64
	gotSince  *time.Time
	gotIDs    []int64
}

func (f *fakeTree) TreeCounts(_ context.Context, _ domain.TenantScope, ids []int64, periodID *int64, since *time.Time) (map[int64]int, error) {
	f.gotIDs, f.gotPeriod, f.gotSince = ids, periodID, since
	return f.counts, f.err
}

func TestForbiddenWithoutTenant(t *testing.T) {
	handlertest.RequiresTenantScope(t, New(&fakeTree{}).Get, http.MethodGet, "/api/v1/activity/tree-counts")
}

// Ключи JSON-объекта — строки, поэтому id команды форматируется в строку. Клиент
// читает счётчик по строковому id команды из дерева.
func TestFormatsTeamIDsAsStringKeys(t *testing.T) {
	h := New(&fakeTree{counts: map[int64]int{12: 3}})
	w := handlertest.Do(h.Get, http.MethodGet, "/api/v1/activity/tree-counts", "", handlertest.Tenant(1))
	handlertest.Status(t, w, http.StatusOK)
	var resp struct {
		Counts map[string]int `json:"counts"`
	}
	handlertest.DecodeJSON(t, w, &resp)
	if resp.Counts["12"] != 3 {
		t.Fatalf("counts = %v, want ключ \"12\" со значением 3", resp.Counts)
	}
}

func TestPassesPeriodRangeAndScope(t *testing.T) {
	ft := &fakeTree{counts: map[int64]int{}}
	h := New(ft)
	h.now = func() time.Time { return time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC) }
	w := handlertest.Do(h.Get, http.MethodGet, "/api/v1/activity/tree-counts?period_id=4&range=today", "",
		handlertest.Tenant(1), handlertest.AllowedTeams([]int64{9}))
	handlertest.Status(t, w, http.StatusOK)
	if ft.gotPeriod == nil || *ft.gotPeriod != 4 {
		t.Fatalf("period_id = %v", ft.gotPeriod)
	}
	if ft.gotSince == nil || ft.gotSince.Day() != 25 || ft.gotSince.Hour() != 0 {
		t.Fatalf("range=today должен дать начало суток, получено %v", ft.gotSince)
	}
	if len(ft.gotIDs) != 1 || ft.gotIDs[0] != 9 {
		t.Fatalf("allowedTeamIDs = %v", ft.gotIDs)
	}
}

func TestServiceErrorIs500(t *testing.T) {
	h := New(&fakeTree{err: errors.New("boom")})
	w := handlertest.Do(h.Get, http.MethodGet, "/api/v1/activity/tree-counts", "", handlertest.Tenant(1))
	handlertest.ErrorCode(t, w, http.StatusInternalServerError, "INTERNAL")
}
