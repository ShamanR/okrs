package progressthresholds

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/platform/logging"
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
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f.data[fsKey(scope, key)] = raw
	return nil
}

func (f *fakeSettings) set(key, raw string) {
	f.data[fsKey(domain.TenantScope{TenantID: 1}, key)] = json.RawMessage(raw)
}

func (f *fakeSettings) get(key string) string {
	return string(f.data[fsKey(domain.TenantScope{TenantID: 1}, key)])
}

func withTenant(r *http.Request) *http.Request {
	return r.WithContext(auth.WithTenant(r.Context(), &domain.Tenant{ID: 1, Name: "Acme", Status: domain.TenantActive}))
}

func getThresholds(t *testing.T, fs *fakeSettings) map[string]int {
	t.Helper()
	r := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/progress-thresholds", nil))
	w := httptest.NewRecorder()
	New(fs).Get(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("GET: код = %d (%s)", w.Code, w.Body.String())
	}
	var got map[string]int
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return got
}

func post(fs *fakeSettings, body string) *httptest.ResponseRecorder {
	r := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/progress-thresholds", strings.NewReader(body)))
	w := httptest.NewRecorder()
	New(fs).Post(w, r)
	return w
}

// Сценарий «Значения по умолчанию».
func TestGetReturnsDefaults(t *testing.T) {
	got := getThresholds(t, newFakeSettings())
	want := map[string]int{"stale_days": 7, "behind_margin": 10, "green_threshold": 80, "weight_tolerance": 0}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: want %d, got %d", k, v, got[k])
		}
	}
}

func TestGetReturnsStoredValues(t *testing.T) {
	fs := newFakeSettings()
	fs.set("progress_stale_days", `14`)
	fs.set("progress_green_threshold", `70`)
	got := getThresholds(t, fs)
	if got["stale_days"] != 14 || got["green_threshold"] != 70 || got["behind_margin"] != 10 {
		t.Fatalf("stored values not returned: %+v", got)
	}
}

// Сценарий «Изменение порога»: сохранённые значения читаются обратно.
func TestPostStoresAllThresholds(t *testing.T) {
	fs := newFakeSettings()
	w := post(fs, `{"stale_days":10,"behind_margin":0,"green_threshold":70,"weight_tolerance":5}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("POST: код = %d (%s)", w.Code, w.Body.String())
	}
	got := getThresholds(t, fs)
	want := map[string]int{"stale_days": 10, "behind_margin": 0, "green_threshold": 70, "weight_tolerance": 5}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: want %d, got %d", k, v, got[k])
		}
	}
}

// Сценарий «Недопустимые значения»: отказ 400, ни одно значение не меняется.
func TestPostRejectsInvalidWithoutWriting(t *testing.T) {
	cases := map[string]string{
		"stale_days zero":           `{"stale_days":0,"behind_margin":10,"green_threshold":80,"weight_tolerance":0}`,
		"behind_margin negative":    `{"stale_days":7,"behind_margin":-1,"green_threshold":80,"weight_tolerance":0}`,
		"green_threshold zero":      `{"stale_days":7,"behind_margin":10,"green_threshold":0,"weight_tolerance":0}`,
		"green_threshold 101":       `{"stale_days":7,"behind_margin":10,"green_threshold":101,"weight_tolerance":0}`,
		"weight_tolerance negative": `{"stale_days":7,"behind_margin":10,"green_threshold":80,"weight_tolerance":-1}`,
		"malformed body":            `{`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			fs := newFakeSettings()
			fs.set("progress_stale_days", `9`)
			w := post(fs, body)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("код = %d, ожидался 400 (%s)", w.Code, w.Body.String())
			}
			if len(fs.data) != 1 || fs.get("progress_stale_days") != `9` {
				t.Fatalf("настройки изменены при отказе: %v", fs.data)
			}
		})
	}
}

func TestPostRequiresTenant(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/progress-thresholds",
		strings.NewReader(`{"stale_days":7,"behind_margin":10,"green_threshold":80,"weight_tolerance":0}`))
	w := httptest.NewRecorder()
	New(newFakeSettings()).Post(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("код = %d, ожидался 403", w.Code)
	}
}

// Изменение порогов — административное изменение организации: каждая запись попадает в аудит.
func TestPostIsAudited(t *testing.T) {
	buf := &bytes.Buffer{}
	r := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/progress-thresholds",
		strings.NewReader(`{"stale_days":7,"behind_margin":10,"green_threshold":80,"weight_tolerance":0}`)))
	r = r.WithContext(logging.WithLogger(r.Context(), logging.New(logging.Config{Output: buf})))
	w := httptest.NewRecorder()
	New(newFakeSettings()).Post(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("код = %d (%s)", w.Code, w.Body.String())
	}
	for _, key := range []string{"progress_stale_days", "progress_behind_margin", "progress_green_threshold", "progress_weight_tolerance"} {
		if !strings.Contains(buf.String(), `"setting":"`+key+`"`) {
			t.Errorf("нет записи аудита для %s: %s", key, buf.String())
		}
	}
}
