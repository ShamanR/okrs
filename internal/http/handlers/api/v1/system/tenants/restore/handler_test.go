package restore

// Обёртка над systemcommon.Transition: пакет отличается только тем, какой
// переход статуса организации он передаёт (Restore).

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/handlertest"
	storetenants "okrs/internal/store/tenants"
)

type fakeProv struct {
	called string
	gotID  int64
	err    error
}

func (f *fakeProv) CreateTenant(context.Context, string, string) (*domain.Tenant, error) {
	return nil, nil
}
func (f *fakeProv) UpdateTenant(context.Context, int64, string, string) (*domain.Tenant, error) {
	return nil, nil
}
func (f *fakeProv) AttachMember(context.Context, int64, int64, domain.Role) (*domain.Membership, error) {
	return nil, nil
}
func (f *fakeProv) SetEntitlements(context.Context, int64, map[string]any) error { return f.err }
func (f *fakeProv) Suspend(_ context.Context, id int64) error {
	f.called, f.gotID = "suspend", id
	return f.err
}
func (f *fakeProv) Restore(_ context.Context, id int64) error {
	f.called, f.gotID = "restore", id
	return f.err
}
func (f *fakeProv) DenyMember(context.Context, int64, int64) error                 { return f.err }
func (f *fakeProv) RemoveMember(context.Context, int64, int64) error               { return f.err }
func (f *fakeProv) SetMemberRole(context.Context, int64, int64, domain.Role) error { return f.err }
func (f *fakeProv) SetSystemAdmin(context.Context, int64, int64, bool) error       { return f.err }

const uri = "/api/v1/system/tenants/3/restore"

func TestBadTenantIDIs400(t *testing.T) {
	for _, v := range []string{"не-число", "0", "-1"} {
		w := handlertest.Do(New(&fakeProv{}).Post, http.MethodPost, uri, "", handlertest.URLParam("id", v))
		handlertest.IsError(t, w, http.StatusBadRequest)
	}
}

func TestAppliesPackageTransition(t *testing.T) {
	f := &fakeProv{}
	w := handlertest.Do(New(f).Post, http.MethodPost, uri, "", handlertest.URLParam("id", "3"))
	handlertest.Status(t, w, http.StatusNoContent)
	if f.called != "restore" {
		t.Fatalf("вызван переход %q, want %q", f.called, "restore")
	}
	if f.gotID != 3 {
		t.Fatalf("tenantID = %d, want 3", f.gotID)
	}
}

// Несуществующая организация — 404, а не общий 500.
func TestMissingTenantIs404(t *testing.T) {
	w := handlertest.Do(New(&fakeProv{err: storetenants.ErrNotFound}).Post, http.MethodPost, uri, "",
		handlertest.URLParam("id", "3"))
	handlertest.IsError(t, w, http.StatusNotFound)
}

func TestOtherErrorIs500(t *testing.T) {
	w := handlertest.Do(New(&fakeProv{err: errors.New("boom")}).Post, http.MethodPost, uri, "",
		handlertest.URLParam("id", "3"))
	handlertest.IsError(t, w, http.StatusInternalServerError)
}
