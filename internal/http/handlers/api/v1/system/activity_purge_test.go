package system

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"okrs/internal/core/domain"

	"github.com/go-chi/chi/v5"
)

type fakeSysPurger struct {
	deleted     int64
	lastScope   domain.TenantScope
	lastCutoff  *time.Time
	calledCount int
}

func (f *fakeSysPurger) Purge(_ context.Context, scope domain.TenantScope, olderThan *time.Time) (int64, error) {
	f.calledCount++
	f.lastScope = scope
	f.lastCutoff = olderThan
	return f.deleted, nil
}

// withTenantIDParam injects the {id} chi URL param, mimicking the router.
func withTenantIDParam(r *http.Request, id string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestSystemHandlePurgeActivity(t *testing.T) {
	fp := &fakeSysPurger{deleted: 3}
	h := New(nil, nil, nil, nil, nil, fp)

	// "all" for tenant 5 → 200, scope=5, nil cutoff.
	r := withTenantIDParam(httptest.NewRequest(http.MethodPost, "/api/v1/system/tenants/5/activity/purge", strings.NewReader(`{"older_than":"all"}`)), "5")
	w := httptest.NewRecorder()
	h.HandlePurgeActivity(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("all: status %d", w.Code)
	}
	var body struct {
		Deleted int64 `json:"deleted"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil || body.Deleted != 3 {
		t.Fatalf("all: body=%+v err=%v", body, err)
	}
	if fp.lastScope.TenantID != 5 || fp.lastCutoff != nil {
		t.Fatalf("all: scope=%v cutoff=%v", fp.lastScope, fp.lastCutoff)
	}

	// "year" → non-nil cutoff.
	fp.lastCutoff = nil
	r = withTenantIDParam(httptest.NewRequest(http.MethodPost, "/api/v1/system/tenants/5/activity/purge", strings.NewReader(`{"older_than":"year"}`)), "5")
	w = httptest.NewRecorder()
	h.HandlePurgeActivity(w, r)
	if w.Code != http.StatusOK || fp.lastCutoff == nil {
		t.Fatalf("year: status %d cutoff %v", w.Code, fp.lastCutoff)
	}

	// unknown depth → 422.
	r = withTenantIDParam(httptest.NewRequest(http.MethodPost, "/api/v1/system/tenants/5/activity/purge", strings.NewReader(`{"older_than":"nope"}`)), "5")
	w = httptest.NewRecorder()
	h.HandlePurgeActivity(w, r)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown: want 422, got %d", w.Code)
	}
}
