package entitlements

// Права организации читаются и пишутся по id из пути; разбор id идёт первым.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/handlertest"
)

// fakeSettings запоминает записанные системные ключи и отдаёт заранее заданные
// значения на чтение.
type fakeSettings struct {
	written map[string]any
	stored  map[string]json.RawMessage
	ent     map[string]json.RawMessage
	err     error
}

func newFakeSettings() *fakeSettings {
	return &fakeSettings{written: map[string]any{}, stored: map[string]json.RawMessage{}}
}

func (f *fakeSettings) SystemSet(_ context.Context, key string, value any) error {
	if f.err != nil {
		return f.err
	}
	f.written[key] = value
	return nil
}
func (f *fakeSettings) SystemGet(_ context.Context, key string) (json.RawMessage, error) {
	return f.stored[key], f.err
}
func (f *fakeSettings) TenantEntitlements(context.Context, domain.TenantScope) (map[string]json.RawMessage, error) {
	return f.ent, f.err
}

type fakeProv struct {
	gotID  int64
	gotEnt map[string]any
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
func (f *fakeProv) SetEntitlements(_ context.Context, id int64, ent map[string]any) error {
	f.gotID, f.gotEnt = id, ent
	return f.err
}
func (f *fakeProv) Suspend(context.Context, int64) error                           { return f.err }
func (f *fakeProv) Restore(context.Context, int64) error                           { return f.err }
func (f *fakeProv) DenyMember(context.Context, int64, int64) error                 { return f.err }
func (f *fakeProv) RemoveMember(context.Context, int64, int64) error               { return f.err }
func (f *fakeProv) SetMemberRole(context.Context, int64, int64, domain.Role) error { return f.err }
func (f *fakeProv) SetSystemAdmin(context.Context, int64, int64, bool) error       { return f.err }

const uri = "/api/v1/system/tenants/3/entitlements"

func TestBadTenantIDIs400(t *testing.T) {
	for _, v := range []string{"не-число", "0", "-1"} {
		t.Run(v, func(t *testing.T) {
			w := handlertest.Do(New(&fakeProv{}, newFakeSettings()).Put, http.MethodPut, uri, `{}`,
				handlertest.URLParam("id", v))
			handlertest.IsError(t, w, http.StatusBadRequest)
			w = handlertest.Do(New(&fakeProv{}, newFakeSettings()).Get, http.MethodGet, uri, "",
				handlertest.URLParam("id", v))
			handlertest.IsError(t, w, http.StatusBadRequest)
		})
	}
}

func TestMalformedBodyIs400(t *testing.T) {
	w := handlertest.Do(New(&fakeProv{}, newFakeSettings()).Put, http.MethodPut, uri, `{не json`,
		handlertest.URLParam("id", "3"))
	handlertest.IsError(t, w, http.StatusBadRequest)
}

func TestPutPassesTenantIDAndBody(t *testing.T) {
	p := &fakeProv{}
	w := handlertest.Do(New(p, newFakeSettings()).Put, http.MethodPut, uri, `{"sso":true}`,
		handlertest.URLParam("id", "3"))
	handlertest.Status(t, w, http.StatusNoContent)
	if p.gotID != 3 {
		t.Fatalf("tenantID = %d, want 3", p.gotID)
	}
	if v, ok := p.gotEnt["sso"]; !ok || v != true {
		t.Fatalf("права не доехали: %v", p.gotEnt)
	}
}

// Организация без выданных прав должна отдавать пустой объект, а не null:
// клиент читает его как словарь.
func TestGetEmptyIsObjectNotNull(t *testing.T) {
	w := handlertest.Do(New(&fakeProv{}, newFakeSettings()).Get, http.MethodGet, uri, "",
		handlertest.URLParam("id", "3"))
	handlertest.Status(t, w, http.StatusOK)
	if b := w.Body.String(); b == "null\n" || b == "null" {
		t.Fatalf("тело = %q, want {}", b)
	}
}

func TestStoreErrorIs500(t *testing.T) {
	w := handlertest.Do(New(&fakeProv{err: errors.New("boom")}, newFakeSettings()).Put, http.MethodPut, uri,
		`{}`, handlertest.URLParam("id", "3"))
	handlertest.IsError(t, w, http.StatusInternalServerError)
}
