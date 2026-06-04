package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"okrs/internal/auth"
	"okrs/internal/domain"
)

// fakeSettings is an in-memory settingsStore for handler tests.
type fakeSettings struct {
	data map[string]json.RawMessage
}

func newFakeSettings() *fakeSettings { return &fakeSettings{data: map[string]json.RawMessage{}} }

func (f *fakeSettings) GetSetting(_ context.Context, key string) (json.RawMessage, error) {
	return f.data[key], nil
}

func (f *fakeSettings) SetSetting(_ context.Context, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f.data[key] = raw
	return nil
}

func TestHandleMeReturns401WhenNoUser(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	w := httptest.NewRecorder()
	HandleMe(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestHandleMeReturnsUserJSON(t *testing.T) {
	u := &domain.User{
		ID:          99,
		DisplayName: "Alice",
		Email:       "alice@example.com",
		AvatarURL:   "https://example.com/avatar.png",
		Provider:    "google",
		IsAdmin:     true,
	}
	r := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	r = r.WithContext(auth.WithUser(r.Context(), u))
	w := httptest.NewRecorder()
	HandleMe(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var got meResponse
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if got.ID != 99 {
		t.Errorf("id: want 99, got %d", got.ID)
	}
	if got.DisplayName != "Alice" {
		t.Errorf("display_name: want Alice, got %s", got.DisplayName)
	}
	if got.Email != "alice@example.com" {
		t.Errorf("email: want alice@example.com, got %s", got.Email)
	}
	if got.AvatarURL != "https://example.com/avatar.png" {
		t.Errorf("avatar_url: want https://example.com/avatar.png, got %s", got.AvatarURL)
	}
	if got.Provider != "google" {
		t.Errorf("provider: want google, got %s", got.Provider)
	}
	if !got.IsAdmin {
		t.Errorf("is_admin: want true, got false")
	}
}

func TestHandleGetGeneralSettingsReturnsStoredURL(t *testing.T) {
	fs := newFakeSettings()
	_ = fs.SetSetting(context.Background(), "documentation_url", "https://example.com/wiki")
	h := New(nil, fs, nil, nil)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/general", nil)
	w := httptest.NewRecorder()
	h.HandleGetGeneralSettings(w, r)

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
	h := New(nil, newFakeSettings(), nil, nil)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/general", nil)
	w := httptest.NewRecorder()
	h.HandleGetGeneralSettings(w, r)

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
	h := New(nil, fs, nil, nil)

	body := strings.NewReader(`{"documentation_url":"  https://example.com/wiki  "}`)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/general", body)
	w := httptest.NewRecorder()
	h.HandleUpdateGeneralSettings(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d (%s)", w.Code, w.Body.String())
	}
	var stored string
	_ = json.Unmarshal(fs.data["documentation_url"], &stored)
	if stored != "https://example.com/wiki" {
		t.Errorf("stored url: want trimmed https://example.com/wiki, got %q", stored)
	}
}

func TestHandleUpdateGeneralSettingsAllowsEmptyToClear(t *testing.T) {
	fs := newFakeSettings()
	_ = fs.SetSetting(context.Background(), "documentation_url", "https://example.com/wiki")
	h := New(nil, fs, nil, nil)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/general", strings.NewReader(`{"documentation_url":""}`))
	w := httptest.NewRecorder()
	h.HandleUpdateGeneralSettings(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
	var stored string
	_ = json.Unmarshal(fs.data["documentation_url"], &stored)
	if stored != "" {
		t.Errorf("stored url: want empty, got %q", stored)
	}
}

func TestHandleUpdateGeneralSettingsRejectsNonHTTPURL(t *testing.T) {
	for _, bad := range []string{`{"documentation_url":"javascript:alert(1)"}`, `{"documentation_url":"not a url"}`, `{"documentation_url":"ftp://example.com"}`} {
		fs := newFakeSettings()
		h := New(nil, fs, nil, nil)
		r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/general", strings.NewReader(bad))
		w := httptest.NewRecorder()
		h.HandleUpdateGeneralSettings(w, r)
		if w.Code != http.StatusBadRequest {
			t.Errorf("body %s: expected 400, got %d", bad, w.Code)
		}
		if _, ok := fs.data["documentation_url"]; ok {
			t.Errorf("body %s: value must not be stored on validation error", bad)
		}
	}
}
