package unreadcount

import (
	"context"
	"net/http"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/handlertest"
)

type fakeCounter struct {
	n         int
	err       error
	gotUserID int64
}

func (f *fakeCounter) UnreadCount(_ context.Context, _ domain.TenantScope, userID int64) (int, error) {
	f.gotUserID = userID
	return f.n, f.err
}

func TestForbiddenWithoutTenant(t *testing.T) {
	handlertest.RequiresTenantScope(t, New(&fakeCounter{}).Get, http.MethodGet, "/api/v1/notifications/unread-count")
}

func TestReturnsCountForAuthenticatedUser(t *testing.T) {
	fc := &fakeCounter{n: 3}
	h := New(fc)
	w := handlertest.Do(h.Get, http.MethodGet, "/api/v1/notifications/unread-count", "",
		handlertest.Tenant(1), handlertest.UserID(7, "u7"))
	handlertest.Status(t, w, http.StatusOK)
	var resp struct {
		Count int `json:"count"`
	}
	handlertest.DecodeJSON(t, w, &resp)
	if resp.Count != 3 {
		t.Fatalf("count = %d, want 3", resp.Count)
	}
	if fc.gotUserID != 7 {
		t.Fatalf("userID = %d, want 7", fc.gotUserID)
	}
}

func TestServiceErrorIs500(t *testing.T) {
	h := New(&fakeCounter{err: context.DeadlineExceeded})
	w := handlertest.Do(h.Get, http.MethodGet, "/api/v1/notifications/unread-count", "", handlertest.Tenant(1))
	handlertest.ErrorCode(t, w, http.StatusInternalServerError, "INTERNAL")
}
