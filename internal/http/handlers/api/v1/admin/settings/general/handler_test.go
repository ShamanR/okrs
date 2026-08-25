package general

// Тесты переехали из пакета admin вместе с обработчиками GET/POST /api/v1/admin/settings/general.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"strconv"
	"strings"
	"testing"
)

type fakeSettings struct {
	data map[string]json.RawMessage
}

func newFakeSettings() *fakeSettings { return &fakeSettings{data: map[string]json.RawMessage{}} }

func fsKey(scope domain.TenantScope, key string) string {
	return strconv.FormatInt(scope.TenantID, 10) + ":" + key
}

func (f *fakeSettings) GetTenant(_ context.Context, scope domain.TenantScope, key string) (json.RawMessage, error) {
	return f.data[fsKey(scope, key)], nil
}

func (f *fakeSettings) SetTenantProduct(_ context.Context, scope domain.TenantScope, key string, value any) error {
	if strings.HasPrefix(key, "entitlement.") {
		return errors.New("entitlement.* is system-admin only")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f.data[fsKey(scope, key)] = raw
	return nil
}

// set is a test helper mirroring the old SetSetting (tenant #1).
func (f *fakeSettings) set(key string, value any) {
	raw, _ := json.Marshal(value)
	f.data[fsKey(domain.TenantScope{TenantID: 1}, key)] = raw
}

// get is a test helper reading a tenant #1 key.
func (f *fakeSettings) get(key string) (json.RawMessage, bool) {
	v, ok := f.data[fsKey(domain.TenantScope{TenantID: 1}, key)]
	return v, ok
}

// fakeRenamer records RenameTenant calls for handler tests.
type fakeRenamer struct {
	id   int64
	name string
	err  error
}

func (f *fakeRenamer) RenameTenant(_ context.Context, id int64, name string) error {
	if f.err != nil {
		return f.err
	}
	f.id, f.name = id, name
	return nil
}

// withTenant attaches the default tenant #1 so TenantScopeFromContext returns {1}.
func withTenant(r *http.Request) *http.Request {
	return r.WithContext(auth.WithTenant(r.Context(), &domain.Tenant{ID: 1, Name: "Acme", Status: domain.TenantActive}))
}

func TestHandleGetGeneralSettingsReturnsStoredURL(t *testing.T) {
	fs := newFakeSettings()
	fs.set("documentation_url", "https://example.com/wiki")
	h := New(nil, fs)

	r := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/general", nil))
	w := httptest.NewRecorder()
	h.Get(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var got struct {
		DocumentationURL string `json:"documentation_url"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if got.DocumentationURL != "https://example.com/wiki" {
		t.Errorf("documentation_url: want https://example.com/wiki, got %q", got.DocumentationURL)
	}
}

func TestHandleGetGeneralSettingsEmptyWhenUnset(t *testing.T) {
	h := New(nil, newFakeSettings())
	r := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/general", nil))
	w := httptest.NewRecorder()
	h.Get(w, r)

	var got struct {
		DocumentationURL string `json:"documentation_url"`
	}
	_ = json.NewDecoder(w.Body).Decode(&got)
	if got.DocumentationURL != "" {
		t.Errorf("documentation_url: want empty, got %q", got.DocumentationURL)
	}
}

func TestHandleUpdateGeneralSettingsStoresValidURL(t *testing.T) {
	fs := newFakeSettings()
	fr := &fakeRenamer{}
	h := New(fr, fs)

	body := strings.NewReader(`{"name":"Acme","documentation_url":"  https://example.com/wiki  "}`)
	r := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/general", body))
	w := httptest.NewRecorder()
	h.Post(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d (%s)", w.Code, w.Body.String())
	}
	raw, _ := fs.get("documentation_url")
	var stored string
	_ = json.Unmarshal(raw, &stored)
	if stored != "https://example.com/wiki" {
		t.Errorf("stored url: want trimmed https://example.com/wiki, got %q", stored)
	}
}

func TestHandleUpdateGeneralSettings_OmittedIntervalPreservesStored(t *testing.T) {
	fs := newFakeSettings()
	fs.set("progress_snapshot_interval_days", 7)
	h := New(&fakeRenamer{}, fs)

	// Payload without progress_snapshot_interval_days (e.g. an older client).
	body := strings.NewReader(`{"name":"Acme","documentation_url":""}`)
	r := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/general", body))
	w := httptest.NewRecorder()
	h.Post(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d (%s)", w.Code, w.Body.String())
	}
	raw, _ := fs.get("progress_snapshot_interval_days")
	var stored int
	_ = json.Unmarshal(raw, &stored)
	if stored != 7 {
		t.Fatalf("interval must be preserved when the field is omitted, got %d", stored)
	}
}

func TestHandleUpdateGeneralSettings_IntervalClampedAndStored(t *testing.T) {
	fs := newFakeSettings()
	h := New(&fakeRenamer{}, fs)

	body := strings.NewReader(`{"name":"Acme","documentation_url":"","progress_snapshot_interval_days":0}`)
	r := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/general", body))
	w := httptest.NewRecorder()
	h.Post(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
	raw, _ := fs.get("progress_snapshot_interval_days")
	var stored int
	_ = json.Unmarshal(raw, &stored)
	if stored != 1 {
		t.Fatalf("interval <1 must clamp to 1, got %d", stored)
	}
}

func TestHandleUpdateGeneralSettingsRenamesTenant(t *testing.T) {
	fs := newFakeSettings()
	fr := &fakeRenamer{}
	h := New(fr, fs)

	body := strings.NewReader(`{"name":"  Acme LLC  ","documentation_url":""}`)
	r := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/general", body))
	w := httptest.NewRecorder()
	h.Post(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d (%s)", w.Code, w.Body.String())
	}
	if fr.id != 1 || fr.name != "Acme LLC" {
		t.Fatalf("rename call = {id:%d name:%q}, want {1 Acme LLC}", fr.id, fr.name)
	}
}

func TestHandleUpdateGeneralSettingsRejectsEmptyName(t *testing.T) {
	fs := newFakeSettings()
	fr := &fakeRenamer{}
	h := New(fr, fs)

	r := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/general", strings.NewReader(`{"name":"  ","documentation_url":""}`)))
	w := httptest.NewRecorder()
	h.Post(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if fr.name != "" {
		t.Errorf("rename must not be called on empty name, got %q", fr.name)
	}
}

func TestHandleUpdateGeneralSettingsAllowsEmptyToClear(t *testing.T) {
	fs := newFakeSettings()
	fs.set("documentation_url", "https://example.com/wiki")
	h := New(&fakeRenamer{}, fs)

	r := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/general", strings.NewReader(`{"name":"Acme","documentation_url":""}`)))
	w := httptest.NewRecorder()
	h.Post(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
	raw, _ := fs.get("documentation_url")
	var stored string
	_ = json.Unmarshal(raw, &stored)
	if stored != "" {
		t.Errorf("stored url: want empty, got %q", stored)
	}
}

func TestHandleUpdateGeneralSettingsRejectsNonHTTPURL(t *testing.T) {
	for _, bad := range []string{`{"name":"Acme","documentation_url":"javascript:alert(1)"}`, `{"name":"Acme","documentation_url":"not a url"}`, `{"name":"Acme","documentation_url":"ftp://example.com"}`} {
		fs := newFakeSettings()
		h := New(&fakeRenamer{}, fs)
		r := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/general", strings.NewReader(bad)))
		w := httptest.NewRecorder()
		h.Post(w, r)
		if w.Code != http.StatusBadRequest {
			t.Errorf("body %s: expected 400, got %d", bad, w.Code)
		}
		if _, ok := fs.get("documentation_url"); ok {
			t.Errorf("body %s: value must not be stored on validation error", bad)
		}
	}
}

func TestHandleGeneralSettingsEmptyHierarchyMessage(t *testing.T) {
	fs := newFakeSettings()
	h := New(&fakeRenamer{}, fs)
	body := strings.NewReader(`{"name":"Acme","documentation_url":"","empty_hierarchy_message":"ask **ops**"}`)
	r := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/general", body))
	w := httptest.NewRecorder()
	h.Post(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("post: %d (%s)", w.Code, w.Body.String())
	}
	r = withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/general", nil))
	w = httptest.NewRecorder()
	h.Get(w, r)
	var got struct {
		EmptyHierarchyMessage string `json:"empty_hierarchy_message"`
	}
	_ = json.NewDecoder(w.Body).Decode(&got)
	if got.EmptyHierarchyMessage != "ask **ops**" {
		t.Fatalf("empty_hierarchy_message = %q", got.EmptyHierarchyMessage)
	}
}
