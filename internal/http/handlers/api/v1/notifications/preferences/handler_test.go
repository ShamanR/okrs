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
	notificationchannelsvc "okrs/internal/service/notificationchannel"
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
	// gotCalled — строки, с которыми позвали сервис; gotSets — строки, которые он
	// в итоге записал. Различие несёт смысл: при отказе первое непусто, второе
	// пусто, и это ровно то свойство, которое проверяется — валидация всей матрицы
	// до первой записи.
	gotCalled []notificationprefs.Preference
	gotSets   []notificationprefs.Preference
	// defaults — значения каналов по умолчанию в пространстве. Экран показывает
	// действующее состояние, поэтому без них ячейку нарисовать нельзя.
	defaults    map[string]bool
	defaultsErr error
	// gotIsAdmin — роль, которую обработчик вывел из контекста запроса.
	gotIsAdmin bool
}

func (f *fakeSvc) DeliveryDefaults(context.Context, domain.TenantScope) (map[string]bool, error) {
	if f.defaultsErr != nil {
		return nil, f.defaultsErr
	}
	if f.defaults == nil {
		return map[string]bool{notificationprefs.ChannelInApp: true}, nil
	}
	return f.defaults, nil
}

// fakeChannels — колонки матрицы: внешние каналы пространства с их названиями и
// значениями по умолчанию.
type fakeChannels []notificationchannelsvc.ChannelState

func (f fakeChannels) List(context.Context, domain.TenantScope) ([]notificationchannelsvc.ChannelState, error) {
	return f, nil
}

func (f *fakeSvc) GetAll(_ context.Context, _ domain.TenantScope, userID int64, isAdmin bool) ([]notificationprefs.Preference, error) {
	f.gotUserID = userID
	f.gotIsAdmin = isAdmin
	return f.getAll, f.getErr
}

func (f *fakeSvc) SetAll(_ context.Context, _ domain.TenantScope, userID int64, isAdmin bool, ps []notificationprefs.Preference) error {
	f.gotUserID = userID
	f.gotIsAdmin = isAdmin
	f.gotCalled = append(f.gotCalled, ps...)
	// При ошибке НИЧЕГО не записываем: настоящий сервис проверяет всю матрицу до
	// первой записи, и фейк, копящий строки перед отказом, скрыл бы регресс этого
	// свойства — тест на «отказ ничего не меняет» проходил бы при сломанном коде.
	if f.setErr != nil {
		return f.setErr
	}
	f.gotSets = append(f.gotSets, ps...)
	return nil
}

// fullMatrix is what the real service returns for a user who never opened settings:
// all four types, defaults substituted, my_comment_resolved carrying no scope.
func fullMatrix() []notificationprefs.Preference {
	return []notificationprefs.Preference{
		{Type: notificationprefs.TypeGoalComment, Enabled: true, Scope: notificationprefs.ScopeOwn, ChannelOverrides: map[string]bool{"in_app": true}},
		{Type: notificationprefs.TypeMyCommentResolved, Enabled: true, Scope: "", ChannelOverrides: map[string]bool{"in_app": true}},
		{Type: notificationprefs.TypeGoalChanged, Enabled: false, Scope: notificationprefs.ScopeSubtree, ChannelOverrides: map[string]bool{"in_app": true}},
		{Type: notificationprefs.TypeKRProgress, Enabled: true, Scope: notificationprefs.ScopeOwnAndChildren, ChannelOverrides: map[string]bool{"in_app": true}},
	}
}

// GET участника обязан вернуть все четыре типа «Целей», даже если пользователь
// ничего не настраивал: иначе экран настроек у нового пользователя будет
// пустым. Значения enabled/scope разные по строкам нарочно — иначе мутация,
// зануляющая проброс полей, осталась бы незамеченной.
func TestGetForMemberReturnsTheFourGoalTypes(t *testing.T) {
	svc := &fakeSvc{getAll: fullMatrix()}
	h := preferences.New(svc, nil)

	w := handlertest.Do(h.Get, http.MethodGet, "/api/v1/notifications/preferences", "",
		handlertest.Tenant(1), handlertest.UserID(42, "u42"))
	handlertest.Status(t, w, http.StatusOK)

	var got struct {
		Items []struct {
			Type      string          `json:"type"`
			Enabled   bool            `json:"enabled"`
			Scope     string          `json:"scope"`
			Channels  map[string]bool `json:"channels"`
			Addressed bool            `json:"addressed"`
			Category  string          `json:"category"`
		} `json:"items"`
		Channels []struct {
			Name      string `json:"name"`
			Title     string `json:"title"`
			DefaultOn bool   `json:"default_on"`
		} `json:"channels"`
	}
	handlertest.DecodeJSON(t, w, &got)

	if len(got.Items) != 4 {
		t.Fatalf("got %d types, want 4", len(got.Items))
	}
	// Сборка без внешних каналов: в матрице остаётся одна колонка — колокольчик,
	// и он включён по умолчанию.
	if len(got.Channels) != 1 || got.Channels[0].Name != "in_app" {
		t.Fatalf("channels: %+v, want один in_app", got.Channels)
	}
	if !got.Channels[0].DefaultOn || got.Channels[0].Title == "" {
		t.Fatalf("колонка колокольчика неполна: %+v", got.Channels[0])
	}
	if svc.gotUserID != 42 {
		t.Errorf("userID passed to service = %d, want 42 (from the authenticated context)", svc.gotUserID)
	}
	if svc.gotIsAdmin {
		t.Error("без роли администратора в контексте сервис не должен получать isAdmin")
	}

	var sawGoalChanged, sawAddressed bool
	for _, it := range got.Items {
		if it.Category != notificationprefs.CategoryGoals {
			t.Errorf("%s: категория %q, want goals", it.Type, it.Category)
		}
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
	handlertest.RequiresTenantScope(t, preferences.New(&fakeSvc{}, nil).Get, http.MethodGet, "/api/v1/notifications/preferences")
}

func TestGetServiceErrorIs500(t *testing.T) {
	h := preferences.New(&fakeSvc{getErr: context.DeadlineExceeded}, nil)
	w := handlertest.Do(h.Get, http.MethodGet, "/api/v1/notifications/preferences", "", handlertest.Tenant(1))
	handlertest.ErrorCode(t, w, http.StatusInternalServerError, "INTERNAL")
}

// Невалидный тип — 400 с полем в details, а не 500.
func TestPutRejectsUnknownType(t *testing.T) {
	svc := &fakeSvc{setErr: notificationprefsvc.ErrInvalidType}
	h := preferences.New(svc, nil)

	body := `{"items":[{"type":"made_up","enabled":true,"scope":"own","channels":{"in_app":true}}]}`
	w := handlertest.Do(h.Put, http.MethodPut, "/api/v1/notifications/preferences", body,
		handlertest.Tenant(1), handlertest.UserID(42, "u42"))
	handlertest.ErrorCode(t, w, http.StatusBadRequest, "VALIDATION_ERROR")
	if len(svc.gotCalled) != 1 || svc.gotCalled[0].Type != "made_up" {
		t.Fatalf("сервис обязан получить спорную строку — отвергает её он, а не хендлер: got %+v", svc.gotCalled)
	}
	if len(svc.gotSets) != 0 {
		t.Errorf("при отказе не должно быть записано ничего: got %+v", svc.gotSets)
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
	h := preferences.New(svc, nil)

	body := `{"items":[{"type":"goal_comment","enabled":true,"scope":"bogus","channels":{"in_app":true}}]}`
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
	h := preferences.New(svc, nil)

	body := `{"items":[{"type":"goal_comment","enabled":true,"scope":"own","channels":{"telegram":true}}]}`
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
	h := preferences.New(&fakeSvc{}, nil)
	w := handlertest.Do(h.Put, http.MethodPut, "/api/v1/notifications/preferences", "not json",
		handlertest.Tenant(1), handlertest.UserID(42, "u42"))
	handlertest.ErrorCode(t, w, http.StatusBadRequest, "VALIDATION_ERROR")
}

func TestPutServiceErrorIs500(t *testing.T) {
	h := preferences.New(&fakeSvc{setErr: errors.New("boom")}, nil)
	body := `{"items":[{"type":"goal_comment","enabled":true,"scope":"own","channels":{"in_app":true}}]}`
	w := handlertest.Do(h.Put, http.MethodPut, "/api/v1/notifications/preferences", body,
		handlertest.Tenant(1), handlertest.UserID(42, "u42"))
	handlertest.ErrorCode(t, w, http.StatusInternalServerError, "INTERNAL")
}

func TestPutWithoutScopeIsForbidden(t *testing.T) {
	handlertest.RequiresTenantScope(t, preferences.New(&fakeSvc{}, nil).Put, http.MethodPut, "/api/v1/notifications/preferences")
}

// A payload longer than the closed type set is rejected outright, before Set is
// called even once: the loop must never be able to run more times than there are
// known types, no matter what the client sends.
func TestPutRejectsOversizedPayload(t *testing.T) {
	svc := &fakeSvc{}
	h := preferences.New(svc, nil)

	var items []string
	for i := 0; i <= len(notificationprefs.AllTypes); i++ {
		items = append(items, `{"type":"goal_comment","enabled":true,"scope":"own","channels":{"in_app":true}}`)
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
	h := preferences.New(svc, nil)

	body := `{"items":[
		{"type":"goal_comment","enabled":true,"scope":"own","channels":{"in_app":true}},
		{"type":"goal_comment","enabled":false,"scope":"subtree","channels":{"in_app":true}}
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
	h := preferences.New(svc, nil)

	body := `{"items":[
		{"type":"goal_comment","enabled":false,"scope":"own_and_children","channels":{"in_app":true}},
		{"type":"kr_progress","enabled":true,"scope":"subtree","channels":{"in_app":true}}
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
	h := preferences.New(&fakeSvc{getAll: fullMatrix()}, nil)
	w := handlertest.Do(h.Get, http.MethodGet, "/api/v1/notifications/preferences", "", handlertest.Tenant(1))
	handlertest.Status(t, w, http.StatusOK)
	if w.Header().Get("Cache-Control") == "" {
		t.Error("Get must set an API cache-control header")
	}
}

// recordingWriter изображает обёртку ответа из цепочки middleware: она и есть
// единственный адресат технической причины. Побеждает первая записанная причина —
// ровно как в middleware.Recorder.
type recordingWriter struct {
	*httptest.ResponseRecorder
	code  string
	cause error
}

func (w *recordingWriter) RecordError(code string, cause error) {
	if w.code == "" {
		w.code = code
	}
	if w.cause == nil {
		w.cause = cause
	}
}

// Отказ чтения обязан оставлять в записи о запросе НАСТОЯЩУЮ причину и то, какой
// шаг её породил. Иначе в логе остаётся тот же текст, который клиент уже увидел,
// и отличить отвалившуюся базу от недоступного канала можно только подключением
// к базе — что однажды и пришлось сделать.
func TestGetFailureRecordsTheRealCauseNotTheUserFacingText(t *testing.T) {
	boom := errors.New(`ERROR: column "channels" does not exist (SQLSTATE 42703)`)
	w := &recordingWriter{ResponseRecorder: httptest.NewRecorder()}
	h := preferences.New(&fakeSvc{getErr: boom}, nil)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/preferences", nil)
	r = handlertest.Tenant(1)(r)
	r = handlertest.UserID(42, "u42")(r)
	h.Get(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	// Клиенту — обезличенный текст, без устройства системы.
	if body := w.Body.String(); !strings.Contains(body, "failed to load preferences") {
		t.Fatalf("клиент должен получить обобщённый текст: %s", body)
	} else if strings.Contains(body, "SQLSTATE") {
		t.Fatalf("техническая причина утекла клиенту: %s", body)
	}
	// В журнал — причина и шаг.
	if w.cause == nil {
		t.Fatal("причина не дошла до записи о запросе")
	}
	if !errors.Is(w.cause, boom) {
		t.Fatalf("исходная ошибка потеряна: %v", w.cause)
	}
	if !strings.Contains(w.cause.Error(), "preferences:") {
		t.Fatalf("причина не называет упавший шаг: %v", w.cause)
	}
}

// У каждого из трёх шагов чтения — своя пометка: клиент их не различает, а
// расследование обязано.
func TestEachLoadStepRecordsItsOwnLabel(t *testing.T) {
	boom := errors.New("upstream is down")
	cases := map[string]struct {
		svc  *fakeSvc
		want string
	}{
		"настройки":        {svc: &fakeSvc{getErr: boom}, want: "preferences:"},
		"умолчания канала": {svc: &fakeSvc{defaultsErr: boom}, want: "channel defaults:"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			w := &recordingWriter{ResponseRecorder: httptest.NewRecorder()}
			h := preferences.New(tc.svc, nil)
			r := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/preferences", nil)
			r = handlertest.Tenant(1)(r)
			r = handlertest.UserID(42, "u42")(r)
			h.Get(w, r)

			if w.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500", w.Code)
			}
			if w.cause == nil || !strings.Contains(w.cause.Error(), tc.want) {
				t.Fatalf("причина не называет шаг %q: %v", tc.want, w.cause)
			}
			if !errors.Is(w.cause, boom) {
				t.Fatalf("исходная ошибка потеряна: %v", w.cause)
			}
		})
	}
}
