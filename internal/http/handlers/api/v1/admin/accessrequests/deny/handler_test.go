package deny

// Обёртка над общим телом AccessRequestAction: пакет отличается только тем,
// какую операция онбординга он в него передаёт — это и проверяется.

import (
	"context"
	"net/http"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/handlertest"
	"okrs/internal/store/memberships"
)

// fakeOnboard записывает, какая операция и над каким пользователем была вызвана —
// пакеты approve/deny/members отличаются ровно этим.
type fakeOnboard struct {
	called string
	userID int64
	err    error
}

func (f *fakeOnboard) RequestAccess(context.Context, string, int64) error { return f.err }
func (f *fakeOnboard) ListAccessRequests(context.Context, domain.TenantScope) ([]memberships.AccessRequest, error) {
	return nil, f.err
}
func (f *fakeOnboard) ApproveRequest(_ context.Context, _ domain.TenantScope, id int64) error {
	f.called, f.userID = "approve", id
	return f.err
}
func (f *fakeOnboard) DenyRequest(_ context.Context, _ domain.TenantScope, id int64) error {
	f.called, f.userID = "deny", id
	return f.err
}
func (f *fakeOnboard) RemoveMember(_ context.Context, _ domain.TenantScope, id int64) error {
	f.called, f.userID = "remove", id
	return f.err
}

const uri = "/api/v1/admin/access-requests/7/deny"

func TestRequiresTenant(t *testing.T) {
	w := handlertest.Do(New(&fakeOnboard{}).Post, http.MethodPost, uri, "",
		handlertest.URLParam("userID", "7"))
	handlertest.IsError(t, w, http.StatusForbidden)
}

// Неразбираемый или неположительный userID — ошибка клиента.
func TestBadUserIDIs400(t *testing.T) {
	for _, v := range []string{"не-число", "0", "-1", ""} {
		t.Run(v, func(t *testing.T) {
			w := handlertest.Do(New(&fakeOnboard{}).Post, http.MethodPost, uri, "",
				handlertest.Tenant(1), handlertest.URLParam("userID", v))
			handlertest.IsError(t, w, http.StatusBadRequest)
		})
	}
}

// Успех отвечает 204 без тела: клиенту нечего показывать, кроме факта применения.
func TestAppliesPackageOperation(t *testing.T) {
	f := &fakeOnboard{}
	w := handlertest.Do(New(f).Post, http.MethodPost, uri, "",
		handlertest.Tenant(1), handlertest.URLParam("userID", "7"))
	handlertest.Status(t, w, http.StatusNoContent)
	if f.called != "deny" {
		t.Fatalf("вызвана операция %q, want %q", f.called, "deny")
	}
	if f.userID != 7 {
		t.Fatalf("userID = %d, want 7", f.userID)
	}
}
