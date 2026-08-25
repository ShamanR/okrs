package settings

// Сводка системных настроек собирается из двух ключей; отсутствующее значение
// должно давать пустое поле, а не ронять ответ.

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

const uri = "/api/v1/system/settings"

func TestReadsBothKeys(t *testing.T) {
	f := newFakeSettings()
	f.stored["default_registration_tenant_id"] = json.RawMessage(`3`)
	f.stored["no_access_message"] = json.RawMessage(`"нет доступа"`)
	w := handlertest.Do(New(f).Get, http.MethodGet, uri, "")
	handlertest.Status(t, w, http.StatusOK)
	var resp map[string]any
	handlertest.DecodeJSON(t, w, &resp)
	if len(resp) == 0 {
		t.Fatalf("пустой ответ: %s", w.Body.String())
	}
}

// Свежая инсталляция ещё не имеет этих ключей — ответ должен быть 200, а не 500.
func TestUnsetKeysStillReturn200(t *testing.T) {
	w := handlertest.Do(New(newFakeSettings()).Get, http.MethodGet, uri, "")
	handlertest.Status(t, w, http.StatusOK)
}

func TestStoreErrorIs500(t *testing.T) {
	f := newFakeSettings()
	f.err = errors.New("boom")
	w := handlertest.Do(New(f).Get, http.MethodGet, uri, "")
	handlertest.IsError(t, w, http.StatusInternalServerError)
}
