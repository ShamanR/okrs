package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"okrs/internal/domain"
)

type fakePurger struct {
	deleted    int64
	called     bool
	lastCutoff *time.Time
}

func (f *fakePurger) PurgeActivity(_ context.Context, _ domain.TenantScope, olderThan *time.Time) (int64, error) {
	f.called = true
	f.lastCutoff = olderThan
	return f.deleted, nil
}

func TestHandlePurgeActivity(t *testing.T) {
	fp := &fakePurger{deleted: 7}
	h := New(nil, nil, nil, nil, nil, nil, fp)

	// "all" → 200, deleted count, nil cutoff.
	r := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/admin/activity/purge", strings.NewReader(`{"older_than":"all"}`)))
	w := httptest.NewRecorder()
	h.HandlePurgeActivity(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("all: status %d", w.Code)
	}
	var body struct {
		Deleted int64 `json:"deleted"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil || body.Deleted != 7 {
		t.Fatalf("all: body=%+v err=%v", body, err)
	}
	if !fp.called || fp.lastCutoff != nil {
		t.Fatalf("all: expected purge with nil cutoff, got called=%v cutoff=%v", fp.called, fp.lastCutoff)
	}

	// "quarter" → non-nil cutoff.
	fp.lastCutoff = nil
	r = withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/admin/activity/purge", strings.NewReader(`{"older_than":"quarter"}`)))
	w = httptest.NewRecorder()
	h.HandlePurgeActivity(w, r)
	if w.Code != http.StatusOK || fp.lastCutoff == nil {
		t.Fatalf("quarter: status %d cutoff %v", w.Code, fp.lastCutoff)
	}

	// unknown depth → 422.
	r = withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/admin/activity/purge", strings.NewReader(`{"older_than":"nope"}`)))
	w = httptest.NewRecorder()
	h.HandlePurgeActivity(w, r)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown depth: want 422, got %d", w.Code)
	}
}
