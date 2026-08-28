package preferences_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/api/v1/notifications/preferences"
	"okrs/internal/http/handlers/handlertest"
	notificationprefsvc "okrs/internal/service/notificationpref"
	"okrs/internal/store/notificationprefs"
)

// fakeSvc stands in for *notificationpref.Service. getAll is what Get returns
// verbatim; setErr controls what Set answers for every call, and gotSets records
// every (userID, Preference) pair Set actually received so a test can assert what
// reached the service, not just the HTTP status.
type fakeSvc struct {
	getAll    []notificationprefs.Preference
	getErr    error
	setErr    error
	gotUserID int64
	gotSets   []notificationprefs.Preference
}

func (f *fakeSvc) GetAll(_ context.Context, _ domain.TenantScope, userID int64) ([]notificationprefs.Preference, error) {
	f.gotUserID = userID
	return f.getAll, f.getErr
}

func (f *fakeSvc) Set(_ context.Context, _ domain.TenantScope, userID int64, p notificationprefs.Preference) error {
	f.gotUserID = userID
	f.gotSets = append(f.gotSets, p)
	return f.setErr
}

// fullMatrix is what the real service returns for a user who never opened settings:
// all four types, defaults substituted, my_comment_resolved carrying no scope.
func fullMatrix() []notificationprefs.Preference {
	return []notificationprefs.Preference{
		{Type: notificationprefs.TypeGoalComment, Enabled: true, Scope: notificationprefs.ScopeOwn, Channels: []string{"in_app"}},
		{Type: notificationprefs.TypeMyCommentResolved, Enabled: true, Scope: "", Channels: []string{"in_app"}},
		{Type: notificationprefs.TypeGoalChanged, Enabled: false, Scope: notificationprefs.ScopeSubtree, Channels: []string{"in_app"}},
		{Type: notificationprefs.TypeKRProgress, Enabled: true, Scope: notificationprefs.ScopeOwnAndChildren, Channels: []string{"in_app"}},
	}
}

// GET обязан вернуть все четыре типа, даже если пользователь ничего не настраивал:
// иначе экран настроек у нового пользователя будет пустым. Значения enabled/scope
// разные по строкам нарочно — иначе мутация, зануляющая проброс полей, осталась бы
// незамеченной.
func TestGetReturnsAllFourTypes(t *testing.T) {
	svc := &fakeSvc{getAll: fullMatrix()}
	h := preferences.New(svc)

	w := handlertest.Do(h.Get, http.MethodGet, "/api/v1/notifications/preferences", "",
		handlertest.Tenant(1), handlertest.UserID(42, "u42"))
	handlertest.Status(t, w, http.StatusOK)

	var got struct {
		Items []struct {
			Type      string   `json:"type"`
			Enabled   bool     `json:"enabled"`
			Scope     string   `json:"scope"`
			Channels  []string `json:"channels"`
			Addressed bool     `json:"addressed"`
		} `json:"items"`
		Channels []string `json:"channels"`
	}
	handlertest.DecodeJSON(t, w, &got)

	if len(got.Items) != 4 {
		t.Fatalf("got %d types, want 4", len(got.Items))
	}
	// В фазе 1b канал ровно один — фронт по этому признаку скрывает колонки каналов.
	if len(got.Channels) != 1 || got.Channels[0] != "in_app" {
		t.Fatalf("channels: %v, want [in_app]", got.Channels)
	}
	if svc.gotUserID != 42 {
		t.Errorf("userID passed to service = %d, want 42 (from the authenticated context)", svc.gotUserID)
	}

	var sawGoalChanged, sawAddressed bool
	for _, it := range got.Items {
		switch it.Type {
		case notificationprefs.TypeMyCommentResolved:
			sawAddressed = true
			if !it.Addressed {
				t.Error("my_comment_resolved must be marked addressed")
			}
			if it.Scope != "" {
				t.Errorf("addressed type must carry no scope, got %q", it.Scope)
			}
		case notificationprefs.TypeGoalChanged:
			sawGoalChanged = true
			if it.Addressed {
				t.Error("goal_changed is scope-based, must not be marked addressed")
			}
			if it.Enabled {
				t.Error("goal_changed fixture has enabled=false; handler must not force it true")
			}
			if it.Scope != notificationprefs.ScopeSubtree {
				t.Errorf("scope = %q, want %q (pass-through must not be dropped)", it.Scope, notificationprefs.ScopeSubtree)
			}
		}
	}
	if !sawAddressed || !sawGoalChanged {
		t.Fatalf("fixture types missing from response: %+v", got.Items)
	}
}

func TestGetWithoutScopeIsForbidden(t *testing.T) {
	handlertest.RequiresTenantScope(t, preferences.New(&fakeSvc{}).Get, http.MethodGet, "/api/v1/notifications/preferences")
}

func TestGetServiceErrorIs500(t *testing.T) {
	h := preferences.New(&fakeSvc{getErr: context.DeadlineExceeded})
	w := handlertest.Do(h.Get, http.MethodGet, "/api/v1/notifications/preferences", "", handlertest.Tenant(1))
	handlertest.ErrorCode(t, w, http.StatusInternalServerError, "INTERNAL")
}

// Невалидный тип — 400 с полем в details, а не 500.
func TestPutRejectsUnknownType(t *testing.T) {
	svc := &fakeSvc{setErr: notificationprefsvc.ErrInvalidType}
	h := preferences.New(svc)

	body := `{"items":[{"type":"made_up","enabled":true,"scope":"own","channels":["in_app"]}]}`
	w := handlertest.Do(h.Put, http.MethodPut, "/api/v1/notifications/preferences", body,
		handlertest.Tenant(1), handlertest.UserID(42, "u42"))
	handlertest.ErrorCode(t, w, http.StatusBadRequest, "VALIDATION_ERROR")
	if len(svc.gotSets) != 1 || svc.gotSets[0].Type != "made_up" {
		t.Fatalf("Set must still be called with the offending row so the service is what rejects it: got %+v", svc.gotSets)
	}
	if got := errorField(t, w); got != "type" {
		t.Errorf("details field = %q, want %q", got, "type")
	}
}

// Невалидный scope — тоже 400, отдельная ветка от невалидного типа. The details
// field must name "scope", not "type": the two branches must not collapse into
// one message that always blames the same field.
func TestPutRejectsUnknownScope(t *testing.T) {
	svc := &fakeSvc{setErr: notificationprefsvc.ErrInvalidScope}
	h := preferences.New(svc)

	body := `{"items":[{"type":"goal_comment","enabled":true,"scope":"bogus","channels":["in_app"]}]}`
	w := handlertest.Do(h.Put, http.MethodPut, "/api/v1/notifications/preferences", body,
		handlertest.Tenant(1), handlertest.UserID(42, "u42"))
	handlertest.ErrorCode(t, w, http.StatusBadRequest, "VALIDATION_ERROR")
	if got := errorField(t, w); got != "scope" {
		t.Errorf("details field = %q, want %q", got, "scope")
	}
}

// Unknown channel is a distinct 400 branch from unknown type/scope: field must name
// "channels".
func TestPutRejectsUnknownChannel(t *testing.T) {
	svc := &fakeSvc{setErr: notificationprefsvc.ErrInvalidChannel}
	h := preferences.New(svc)

	body := `{"items":[{"type":"goal_comment","enabled":true,"scope":"own","channels":["telegram"]}]}`
	w := handlertest.Do(h.Put, http.MethodPut, "/api/v1/notifications/preferences", body,
		handlertest.Tenant(1), handlertest.UserID(42, "u42"))
	handlertest.ErrorCode(t, w, http.StatusBadRequest, "VALIDATION_ERROR")
	if got := errorField(t, w); got != "channels" {
		t.Errorf("details field = %q, want %q", got, "channels")
	}
}

// errorField extracts the single key of the error envelope's "fields" details map.
func errorField(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var resp struct {
		Error struct {
			Fields map[string]string `json:"fields"`
		} `json:"error"`
	}
	handlertest.DecodeJSON(t, w, &resp)
	if len(resp.Error.Fields) != 1 {
		t.Fatalf("want exactly one details field, got %v", resp.Error.Fields)
	}
	for k := range resp.Error.Fields {
		return k
	}
	return ""
}

func TestPutInvalidJSONIsBadRequest(t *testing.T) {
	h := preferences.New(&fakeSvc{})
	w := handlertest.Do(h.Put, http.MethodPut, "/api/v1/notifications/preferences", "not json",
		handlertest.Tenant(1), handlertest.UserID(42, "u42"))
	handlertest.ErrorCode(t, w, http.StatusBadRequest, "VALIDATION_ERROR")
}

func TestPutServiceErrorIs500(t *testing.T) {
	h := preferences.New(&fakeSvc{setErr: errors.New("boom")})
	body := `{"items":[{"type":"goal_comment","enabled":true,"scope":"own","channels":["in_app"]}]}`
	w := handlertest.Do(h.Put, http.MethodPut, "/api/v1/notifications/preferences", body,
		handlertest.Tenant(1), handlertest.UserID(42, "u42"))
	handlertest.ErrorCode(t, w, http.StatusInternalServerError, "INTERNAL")
}

func TestPutWithoutScopeIsForbidden(t *testing.T) {
	handlertest.RequiresTenantScope(t, preferences.New(&fakeSvc{}).Put, http.MethodPut, "/api/v1/notifications/preferences")
}

// A payload longer than the closed type set is rejected outright, before Set is
// called even once: the loop must never be able to run more times than there are
// known types, no matter what the client sends.
func TestPutRejectsOversizedPayload(t *testing.T) {
	svc := &fakeSvc{}
	h := preferences.New(svc)

	var items []string
	for i := 0; i <= len(notificationprefs.AllTypes); i++ {
		items = append(items, `{"type":"goal_comment","enabled":true,"scope":"own","channels":["in_app"]}`)
	}
	body := `{"items":[` + strings.Join(items, ",") + `]}`
	w := handlertest.Do(h.Put, http.MethodPut, "/api/v1/notifications/preferences", body,
		handlertest.Tenant(1), handlertest.UserID(42, "u42"))
	handlertest.ErrorCode(t, w, http.StatusBadRequest, "VALIDATION_ERROR")
	if len(svc.gotSets) != 0 {
		t.Fatalf("Set must not be called for an oversized payload, got %d calls", len(svc.gotSets))
	}
}

// Two entries for the same type in one payload are a malformed request, not two
// writes: applying the first and then the second would silently discard whichever
// lost the race, and a client cannot observe which one "won".
func TestPutRejectsDuplicateType(t *testing.T) {
	svc := &fakeSvc{}
	h := preferences.New(svc)

	body := `{"items":[
		{"type":"goal_comment","enabled":true,"scope":"own","channels":["in_app"]},
		{"type":"goal_comment","enabled":false,"scope":"subtree","channels":["in_app"]}
	]}`
	w := handlertest.Do(h.Put, http.MethodPut, "/api/v1/notifications/preferences", body,
		handlertest.Tenant(1), handlertest.UserID(42, "u42"))
	handlertest.ErrorCode(t, w, http.StatusBadRequest, "VALIDATION_ERROR")
	if len(svc.gotSets) != 0 {
		t.Fatalf("Set must not be called when the payload carries a duplicate type, got %d calls", len(svc.gotSets))
	}
	if got := errorField(t, w); got != "type" {
		t.Errorf("details field = %q, want %q", got, "type")
	}
}

// PUT заменяет всю матрицу целиком: каждая строка payload обязана дойти до Set, а
// userID обязан браться из контекста аутентификации, а не из тела — тела с полем
// user_id для этого эндпоинта вообще нет.
func TestPutReplacesWholeMatrix(t *testing.T) {
	svc := &fakeSvc{}
	h := preferences.New(svc)

	body := `{"items":[
		{"type":"goal_comment","enabled":false,"scope":"own_and_children","channels":["in_app"]},
		{"type":"kr_progress","enabled":true,"scope":"subtree","channels":["in_app"]}
	]}`
	w := handlertest.Do(h.Put, http.MethodPut, "/api/v1/notifications/preferences", body,
		handlertest.Tenant(1), handlertest.UserID(42, "u42"))
	handlertest.Status(t, w, http.StatusNoContent)

	if svc.gotUserID != 42 {
		t.Errorf("userID passed to service = %d, want 42 (from the authenticated context, never the body)", svc.gotUserID)
	}
	if len(svc.gotSets) != 2 {
		t.Fatalf("Set calls = %d, want 2 (one per item, whole matrix)", len(svc.gotSets))
	}
	if svc.gotSets[0].Type != notificationprefs.TypeGoalComment || svc.gotSets[0].Enabled {
		t.Errorf("first row = %+v, want type=goal_comment enabled=false", svc.gotSets[0])
	}
	if svc.gotSets[1].Type != notificationprefs.TypeKRProgress || svc.gotSets[1].Scope != notificationprefs.ScopeSubtree {
		t.Errorf("second row = %+v, want type=kr_progress scope=subtree", svc.gotSets[1])
	}
}

// Ensures Get actually flushes cache-control headers the way the other GET
// endpoints in this API do.
func TestGetSetsAPICacheControl(t *testing.T) {
	h := preferences.New(&fakeSvc{getAll: fullMatrix()})
	w := handlertest.Do(h.Get, http.MethodGet, "/api/v1/notifications/preferences", "", handlertest.Tenant(1))
	handlertest.Status(t, w, http.StatusOK)
	if w.Header().Get("Cache-Control") == "" {
		t.Error("Get must set an API cache-control header")
	}
}
