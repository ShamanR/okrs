package defaultregistrationtenant

// Системная настройка пишется целиком одним PUT: тело валидируется, значение
// уходит под фиксированным ключом.

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

const uri = "/api/v1/system/settings/default-registration-tenant"

func TestMalformedBodyIs400(t *testing.T) {
	w := handlertest.Do(New(newFakeSettings()).Put, http.MethodPut, uri, `{не json`)
	handlertest.IsError(t, w, http.StatusBadRequest)
}

func TestStoresUnderFixedKey(t *testing.T) {
	f := newFakeSettings()
	w := handlertest.Do(New(f).Put, http.MethodPut, uri, `{"tenant_id":3}`)
	handlertest.Status(t, w, http.StatusNoContent)
	if _, ok := f.written["default_registration_tenant_id"]; !ok {
		t.Fatalf("ключ %q не записан, записано: %v", "default_registration_tenant_id", f.written)
	}
}

func TestStoreErrorIs500(t *testing.T) {
	f := newFakeSettings()
	f.err = errors.New("boom")
	w := handlertest.Do(New(f).Put, http.MethodPut, uri, `{"tenant_id":3}`)
	handlertest.IsError(t, w, http.StatusInternalServerError)
}
