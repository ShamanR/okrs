package feedback

// Тесты переехали из пакета admin вместе с обработчиками GET/POST /api/v1/admin/settings/feedback.

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

// withTenant attaches the default tenant #1 so TenantScopeFromContext returns {1}.
func withTenant(r *http.Request) *http.Request {
	return r.WithContext(auth.WithTenant(r.Context(), &domain.Tenant{ID: 1, Name: "Acme", Status: domain.TenantActive}))
}

func TestHandleGetFeedbackSettingsDefaults(t *testing.T) {
	h := New(newFakeSettings())
	r := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/feedback", nil))
	w := httptest.NewRecorder()
	h.Get(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var got struct {
		FeedbackURL             string `json:"feedback_url"`
		FeedbackPopupEnabled    bool   `json:"feedback_popup_enabled"`
		FeedbackMenuLinkEnabled bool   `json:"feedback_menu_link_enabled"`
		FeedbackFrequencyDays   int    `json:"feedback_frequency_days"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if got.FeedbackURL != "" || got.FeedbackPopupEnabled || got.FeedbackMenuLinkEnabled {
		t.Errorf("want empty defaults, got %+v", got)
	}
	if got.FeedbackFrequencyDays != 30 {
		t.Errorf("feedback_frequency_days: want default 30, got %d", got.FeedbackFrequencyDays)
	}
}

func TestHandleUpdateFeedbackSettingsStoresValues(t *testing.T) {
	fs := newFakeSettings()
	h := New(fs)
	body := strings.NewReader(`{"feedback_url":"  https://forms.example.com/s  ","feedback_popup_enabled":true,"feedback_menu_link_enabled":true,"feedback_frequency_days":14}`)
	r := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/feedback", body))
	w := httptest.NewRecorder()
	h.Post(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d (%s)", w.Code, w.Body.String())
	}
	rawURL, _ := fs.get("feedback_url")
	var url string
	_ = json.Unmarshal(rawURL, &url)
	if url != "https://forms.example.com/s" {
		t.Errorf("feedback_url: want trimmed value, got %q", url)
	}
	rawFreq, _ := fs.get("feedback_frequency_days")
	var freq int
	_ = json.Unmarshal(rawFreq, &freq)
	if freq != 14 {
		t.Errorf("feedback_frequency_days: want 14, got %d", freq)
	}
}

func TestHandleUpdateFeedbackSettingsRejectsUnsafeScheme(t *testing.T) {
	for _, bad := range []string{
		`{"feedback_url":"javascript:alert(1)","feedback_frequency_days":14}`,
		`{"feedback_url":"  JavaScript:alert(1)","feedback_frequency_days":14}`,
		`{"feedback_url":"data:text/html,<script>1</script>","feedback_frequency_days":14}`,
	} {
		fs := newFakeSettings()
		h := New(fs)
		r := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/feedback", strings.NewReader(bad)))
		w := httptest.NewRecorder()
		h.Post(w, r)
		if w.Code != http.StatusBadRequest {
			t.Errorf("body %s: expected 400, got %d", bad, w.Code)
		}
		if _, ok := fs.get("feedback_url"); ok {
			t.Errorf("body %s: value must not be stored on validation error", bad)
		}
	}
}

func TestHandleUpdateFeedbackSettingsAcceptsNonHTTPURL(t *testing.T) {
	// Unlike documentation_url, the feedback link has no strict http(s) requirement.
	for _, link := range []string{"forms.gle/demo", "ftp://example.com/survey", "/internal/survey"} {
		fs := newFakeSettings()
		h := New(fs)
		body := strings.NewReader(`{"feedback_url":"` + link + `","feedback_frequency_days":30}`)
		r := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/feedback", body))
		w := httptest.NewRecorder()
		h.Post(w, r)
		if w.Code != http.StatusNoContent {
			t.Fatalf("link %q: expected 204, got %d (%s)", link, w.Code, w.Body.String())
		}
		raw, _ := fs.get("feedback_url")
		var stored string
		_ = json.Unmarshal(raw, &stored)
		if stored != link {
			t.Errorf("link %q: stored %q", link, stored)
		}
	}
}

func TestHandleUpdateFeedbackSettingsRejectsBadFrequency(t *testing.T) {
	h := New(newFakeSettings())
	body := strings.NewReader(`{"feedback_url":"","feedback_frequency_days":0}`)
	r := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/feedback", body))
	w := httptest.NewRecorder()
	h.Post(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
