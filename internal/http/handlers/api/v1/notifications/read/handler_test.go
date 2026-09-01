package read

import (
	"context"
	"net/http"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/handlertest"
)

type fakeMarker struct {
	err       error
	gotUserID int64
	gotIDs    []int64
	gotAll    bool
	calls     int
}

func (f *fakeMarker) MarkRead(_ context.Context, _ domain.TenantScope, userID int64, ids []int64, all bool) error {
	f.calls++
	f.gotUserID, f.gotIDs, f.gotAll = userID, ids, all
	return f.err
}

func TestForbiddenWithoutTenant(t *testing.T) {
	handlertest.RequiresTenantScope(t, New(&fakeMarker{}).Post, http.MethodPost, "/api/v1/notifications/read")
}

// The user id comes only from the authenticated session context, never from the
// request body — there is no user_id field in the payload for a client to spoof, so
// one user can never mark another user's notifications read.
func TestUserIDComesFromContextNotBody(t *testing.T) {
	fm := &fakeMarker{}
	h := New(fm)
	w := handlertest.Do(h.Post, http.MethodPost, "/api/v1/notifications/read", `{"ids":[1,2]}`,
		handlertest.Tenant(1), handlertest.UserID(7, "u7"))
	handlertest.Status(t, w, http.StatusNoContent)
	if fm.gotUserID != 7 {
		t.Fatalf("userID passed to service = %d, want 7 (the authenticated user)", fm.gotUserID)
	}
	if len(fm.gotIDs) != 2 || fm.gotIDs[0] != 1 || fm.gotIDs[1] != 2 {
		t.Fatalf("ids = %v, want [1 2]", fm.gotIDs)
	}
	if fm.gotAll {
		t.Fatal("all should be false")
	}
}

func TestAllTrueSkipsIDsRequirement(t *testing.T) {
	fm := &fakeMarker{}
	h := New(fm)
	w := handlertest.Do(h.Post, http.MethodPost, "/api/v1/notifications/read", `{"all":true}`,
		handlertest.Tenant(1), handlertest.UserID(7, "u7"))
	handlertest.Status(t, w, http.StatusNoContent)
	if !fm.gotAll {
		t.Fatal("all should be true")
	}
}

func TestNeitherIDsNorAllIsBadRequest(t *testing.T) {
	fm := &fakeMarker{}
	h := New(fm)
	w := handlertest.Do(h.Post, http.MethodPost, "/api/v1/notifications/read", `{}`,
		handlertest.Tenant(1), handlertest.UserID(7, "u7"))
	handlertest.ErrorCode(t, w, http.StatusBadRequest, "VALIDATION_ERROR")
	if fm.calls != 0 {
		t.Fatal("service must not be called on validation failure")
	}
}

func TestInvalidJSONIsBadRequest(t *testing.T) {
	h := New(&fakeMarker{})
	w := handlertest.Do(h.Post, http.MethodPost, "/api/v1/notifications/read", `not json`,
		handlertest.Tenant(1), handlertest.UserID(7, "u7"))
	handlertest.ErrorCode(t, w, http.StatusBadRequest, "VALIDATION_ERROR")
}

func TestServiceErrorIs500(t *testing.T) {
	h := New(&fakeMarker{err: context.DeadlineExceeded})
	w := handlertest.Do(h.Post, http.MethodPost, "/api/v1/notifications/read", `{"all":true}`,
		handlertest.Tenant(1), handlertest.UserID(7, "u7"))
	handlertest.ErrorCode(t, w, http.StatusInternalServerError, "INTERNAL")
}
