package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"okrs/internal/auth"
	"okrs/internal/domain"
	"okrs/internal/store/grants"
	"okrs/internal/store/users"
)

// fakeSettings is an in-memory tenantSettings for handler tests. It keys by
// (tenant_id, key); single-tenant tests use tenant #1 throughout.
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
	return r.WithContext(auth.WithTenant(r.Context(), &domain.Tenant{ID: 1, Status: domain.TenantActive}))
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
		UDID:        "550e8400-e29b-41d4-a716-446655440000",
		DisplayName: "Alice",
		Email:       "alice@example.com",
		AvatarURL:   "https://example.com/avatar.png",
		Provider:    "google",
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
	if got.UDID != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("udid: want 550e8400-e29b-41d4-a716-446655440000, got %s", got.UDID)
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
}

func TestHandleGetGeneralSettingsReturnsStoredURL(t *testing.T) {
	fs := newFakeSettings()
	fs.set("documentation_url", "https://example.com/wiki")
	h := New(nil, fs, nil, nil, nil)

	r := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/general", nil))
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
	h := New(nil, newFakeSettings(), nil, nil, nil)
	r := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/general", nil))
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
	h := New(nil, fs, nil, nil, nil)

	body := strings.NewReader(`{"documentation_url":"  https://example.com/wiki  "}`)
	r := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/general", body))
	w := httptest.NewRecorder()
	h.HandleUpdateGeneralSettings(w, r)

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

func TestHandleUpdateGeneralSettingsAllowsEmptyToClear(t *testing.T) {
	fs := newFakeSettings()
	fs.set("documentation_url", "https://example.com/wiki")
	h := New(nil, fs, nil, nil, nil)

	r := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/general", strings.NewReader(`{"documentation_url":""}`)))
	w := httptest.NewRecorder()
	h.HandleUpdateGeneralSettings(w, r)

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
	for _, bad := range []string{`{"documentation_url":"javascript:alert(1)"}`, `{"documentation_url":"not a url"}`, `{"documentation_url":"ftp://example.com"}`} {
		fs := newFakeSettings()
		h := New(nil, fs, nil, nil, nil)
		r := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/general", strings.NewReader(bad)))
		w := httptest.NewRecorder()
		h.HandleUpdateGeneralSettings(w, r)
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
	h := New(nil, fs, nil, nil, nil)
	body := strings.NewReader(`{"documentation_url":"","empty_hierarchy_message":"ask **ops**"}`)
	r := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/general", body))
	w := httptest.NewRecorder()
	h.HandleUpdateGeneralSettings(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("post: %d (%s)", w.Code, w.Body.String())
	}
	r = withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/general", nil))
	w = httptest.NewRecorder()
	h.HandleGetGeneralSettings(w, r)
	var got struct {
		EmptyHierarchyMessage string `json:"empty_hierarchy_message"`
	}
	_ = json.NewDecoder(w.Body).Decode(&got)
	if got.EmptyHierarchyMessage != "ask **ops**" {
		t.Fatalf("empty_hierarchy_message = %q", got.EmptyHierarchyMessage)
	}
}

func TestHandleGetFeedbackSettingsDefaults(t *testing.T) {
	h := New(nil, newFakeSettings(), nil, nil, nil)
	r := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/feedback", nil))
	w := httptest.NewRecorder()
	h.HandleGetFeedbackSettings(w, r)

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
	h := New(nil, fs, nil, nil, nil)
	body := strings.NewReader(`{"feedback_url":"  https://forms.example.com/s  ","feedback_popup_enabled":true,"feedback_menu_link_enabled":true,"feedback_frequency_days":14}`)
	r := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/feedback", body))
	w := httptest.NewRecorder()
	h.HandleUpdateFeedbackSettings(w, r)

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
		h := New(nil, fs, nil, nil, nil)
		r := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/feedback", strings.NewReader(bad)))
		w := httptest.NewRecorder()
		h.HandleUpdateFeedbackSettings(w, r)
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
		h := New(nil, fs, nil, nil, nil)
		body := strings.NewReader(`{"feedback_url":"` + link + `","feedback_frequency_days":30}`)
		r := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/feedback", body))
		w := httptest.NewRecorder()
		h.HandleUpdateFeedbackSettings(w, r)
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
	h := New(nil, newFakeSettings(), nil, nil, nil)
	body := strings.NewReader(`{"feedback_url":"","feedback_frequency_days":0}`)
	r := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/feedback", body))
	w := httptest.NewRecorder()
	h.HandleUpdateFeedbackSettings(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// fakeUsers is an in-memory userAdminStore for handler tests.
type fakeUsers struct {
	users       []*domain.User
	tenantUsers []users.TenantUser
}

func (f *fakeUsers) ListUsers(context.Context) ([]*domain.User, error)    { return f.users, nil }
func (f *fakeUsers) GetUser(context.Context, int64) (*domain.User, error) { return nil, nil }
func (f *fakeUsers) ListByTenant(context.Context, domain.TenantScope) ([]users.TenantUser, error) {
	return f.tenantUsers, nil
}

// fakeGrants is an in-memory grantsStore. activeTeamIDs models which granted
// teams are still active; ListDescendantTeamIDs returns only the active roots
// (descendant expansion is irrelevant for the membership test the handler does).
type fakeGrants struct {
	all           map[int64][]grants.HierarchyGrant
	activeTeamIDs map[int64]bool
}

func (f *fakeGrants) ListUserGrants(context.Context, domain.TenantScope, int64) ([]grants.HierarchyGrant, error) {
	return nil, nil
}
func (f *fakeGrants) AllGrants(context.Context) (map[int64][]grants.HierarchyGrant, error) {
	return f.all, nil
}
func (f *fakeGrants) ListDescendantTeamIDs(_ context.Context, _ domain.TenantScope, roots []int64) ([]int64, error) {
	var out []int64
	for _, id := range roots {
		if f.activeTeamIDs[id] {
			out = append(out, id)
		}
	}
	return out, nil
}
func (f *fakeGrants) AddUserGrant(context.Context, domain.TenantScope, int64, int64, int64) error { return nil }
func (f *fakeGrants) RemoveUserGrant(context.Context, domain.TenantScope, int64, int64) error     { return nil }

// The users list is tenant-scoped (members + requesters), each item carries Status, and
// GrantedNodeCount counts only grants to still-active teams (requesters have none).
func TestHandleListUsersIsTenantScopedWithStatus(t *testing.T) {
	fu := &fakeUsers{tenantUsers: []users.TenantUser{
		{User: &domain.User{ID: 10, DisplayName: "Active"}, Status: domain.MembershipActive, Role: domain.RoleUser},
		{User: &domain.User{ID: 20, DisplayName: "Requester"}, Status: domain.MembershipRequested, Role: domain.RoleUser},
	}}
	g := &fakeGrants{
		all: map[int64][]grants.HierarchyGrant{
			10: {{UserID: 10, TeamID: 1}, {UserID: 10, TeamID: 2}}, // team 1 active, team 2 deleted
		},
		activeTeamIDs: map[int64]bool{1: true},
	}
	h := New(fu, nil, nil, g, nil)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	r = r.WithContext(auth.WithTenant(r.Context(), &domain.Tenant{ID: 1, Status: domain.TenantActive}))
	w := httptest.NewRecorder()
	h.HandleListUsers(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var got []struct {
		ID               int64
		Status           string
		GrantedNodeCount int
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 tenant users, got %d", len(got))
	}
	by := map[int64]struct {
		ID               int64
		Status           string
		GrantedNodeCount int
	}{}
	for _, u := range got {
		by[u.ID] = u
	}
	if by[10].Status != "active" || by[10].GrantedNodeCount != 1 {
		t.Errorf("active member = %+v (want status=active, count=1 active team only)", by[10])
	}
	if by[20].Status != "requested" || by[20].GrantedNodeCount != 0 {
		t.Errorf("requester = %+v (want status=requested, count=0)", by[20])
	}
}
