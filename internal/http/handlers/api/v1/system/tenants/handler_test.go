package tenants

// Создание и переименование организации: доменные ошибки различимы клиентом,
// иначе UI не сможет объяснить, что именно не так со slug.

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
	created  *domain.Tenant
	gotName  string
	gotSlug  string
	gotEnt   map[string]any
	entCalls int
	err      error
}

func (f *fakeProv) CreateTenant(_ context.Context, name, slug string) (*domain.Tenant, error) {
	f.gotName, f.gotSlug = name, slug
	if f.err != nil {
		return nil, f.err
	}
	return f.created, nil
}
func (f *fakeProv) UpdateTenant(_ context.Context, _ int64, name, slug string) (*domain.Tenant, error) {
	f.gotName, f.gotSlug = name, slug
	if f.err != nil {
		return nil, f.err
	}
	return f.created, nil
}
func (f *fakeProv) AttachMember(context.Context, int64, int64, domain.Role) (*domain.Membership, error) {
	return nil, nil
}
func (f *fakeProv) SetEntitlements(_ context.Context, _ int64, ent map[string]any) error {
	f.entCalls++
	f.gotEnt = ent
	return nil
}
func (f *fakeProv) Suspend(context.Context, int64) error                           { return nil }
func (f *fakeProv) Restore(context.Context, int64) error                           { return nil }
func (f *fakeProv) DenyMember(context.Context, int64, int64) error                 { return nil }
func (f *fakeProv) RemoveMember(context.Context, int64, int64) error               { return nil }
func (f *fakeProv) SetMemberRole(context.Context, int64, int64, domain.Role) error { return nil }
func (f *fakeProv) SetSystemAdmin(context.Context, int64, int64, bool) error       { return nil }

type fakeList struct {
	list []domain.Tenant
	err  error
}

func (f *fakeList) List(context.Context) ([]domain.Tenant, error) { return f.list, f.err }

const uri = "/api/v1/system/tenants"

func okProv() *fakeProv {
	return &fakeProv{created: &domain.Tenant{ID: 3, Slug: "acme", Name: "Acme", Status: domain.TenantActive}}
}

func TestPostMalformedBodyIs400(t *testing.T) {
	w := handlertest.Do(New(okProv(), &fakeList{}).Post, http.MethodPost, uri, `{не json`)
	handlertest.IsError(t, w, http.StatusBadRequest)
}

func TestPostCreatesAndReturns201(t *testing.T) {
	p := okProv()
	w := handlertest.Do(New(p, &fakeList{}).Post, http.MethodPost, uri, `{"name":"Acme","slug":"acme"}`)
	handlertest.Status(t, w, http.StatusCreated)
	if p.gotName != "Acme" || p.gotSlug != "acme" {
		t.Fatalf("создание получило name=%q slug=%q", p.gotName, p.gotSlug)
	}
}

// Права выдаются вторым шагом и только если их прислали: пустой набор не должен
// приводить к лишней записи.
func TestPostSetsEntitlementsOnlyWhenGiven(t *testing.T) {
	p := okProv()
	handlertest.Do(New(p, &fakeList{}).Post, http.MethodPost, uri, `{"name":"Acme","slug":"acme"}`)
	if p.entCalls != 0 {
		t.Fatalf("SetEntitlements вызван %d раз без прав в теле", p.entCalls)
	}
	p2 := okProv()
	handlertest.Do(New(p2, &fakeList{}).Post, http.MethodPost, uri, `{"name":"Acme","slug":"acme","entitlements":{"sso":true}}`)
	if p2.entCalls != 1 {
		t.Fatalf("SetEntitlements вызван %d раз, want 1", p2.entCalls)
	}
}

// Клиенту нужно отличить «slug занят» от «slug неверного формата»: первое
// лечится другим именем, второе — исправлением ввода.
func TestPostErrorMapping(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"неверный slug", storetenants.ErrInvalidSlug, http.StatusUnprocessableEntity},
		{"slug занят", storetenants.ErrSlugTaken, http.StatusConflict},
		{"прочее", errors.New("boom"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := okProv()
			p.err = c.err
			w := handlertest.Do(New(p, &fakeList{}).Post, http.MethodPost, uri, `{"name":"Acme","slug":"acme"}`)
			handlertest.IsError(t, w, c.want)
		})
	}
}

// Пустой список организаций должен быть [], а не null.
func TestGetEmptyIsArray(t *testing.T) {
	w := handlertest.Do(New(okProv(), &fakeList{}).Get, http.MethodGet, uri, "")
	handlertest.Status(t, w, http.StatusOK)
	var out []map[string]any
	handlertest.DecodeJSON(t, w, &out)
	if out == nil {
		t.Fatalf("список = null, want []")
	}
}

func TestGetStoreErrorIs500(t *testing.T) {
	w := handlertest.Do(New(okProv(), &fakeList{err: errors.New("boom")}).Get, http.MethodGet, uri, "")
	handlertest.IsError(t, w, http.StatusInternalServerError)
}

func TestPatchBadTenantIDIs400(t *testing.T) {
	w := handlertest.Do(New(okProv(), &fakeList{}).Patch, http.MethodPatch, uri+"/x", `{}`,
		handlertest.URLParam("id", "не-число"))
	handlertest.IsError(t, w, http.StatusBadRequest)
}

func TestPatchErrorMapping(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"нет организации", storetenants.ErrNotFound, http.StatusNotFound},
		{"slug занят", storetenants.ErrSlugTaken, http.StatusConflict},
		{"неверный slug", storetenants.ErrInvalidSlug, http.StatusUnprocessableEntity},
		{"неверное имя", storetenants.ErrInvalidName, http.StatusUnprocessableEntity},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := okProv()
			p.err = c.err
			w := handlertest.Do(New(p, &fakeList{}).Patch, http.MethodPatch, uri+"/3", `{"name":"X","slug":"x"}`,
				handlertest.URLParam("id", "3"))
			handlertest.IsError(t, w, c.want)
		})
	}
}
