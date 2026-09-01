package notifications_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"okrs/internal/core/domain"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/api/v1/notifications"
	"okrs/internal/http/handlers/handlertest"
	notificationsvc "okrs/internal/service/notification"
	storenotif "okrs/internal/store/notifications"
)

type fakeSvc struct {
	deleteResult  bool
	deleteErr     error
	gotDeleteUser int64
	gotDeleteID   int64
	items         []storenotif.Notification
	err           error
	// next is what List returns as the opaque pagination token; "" means "no next page".
	next string
	// gotUserID records the userID the handler passed through, so a test can assert
	// it came from the authenticated context rather than anywhere client-controlled.
	gotUserID int64
	// gotFilter records the filter the handler built from query params, so a test can
	// assert query-string parsing (unread, limit) actually reaches the service.
	gotFilter storenotif.ListFilter
	// gotCursor records the raw cursor string the handler forwarded, so a test can
	// assert the handler passes it through opaque rather than decoding it itself.
	gotCursor string
}

// Сигнатура обязана совпадать с портом NotificationReader дословно, включая
// context.Context: иначе фейк не удовлетворит интерфейс и тест не соберётся.
func (f *fakeSvc) List(_ context.Context, _ domain.TenantScope, userID int64, filter storenotif.ListFilter, cursor string) ([]storenotif.Notification, string, error) {
	f.gotUserID = userID
	f.gotFilter = filter
	f.gotCursor = cursor
	return f.items, f.next, f.err
}

// POST to a GET-only route must answer 405, which also proves the path is registered:
// an unregistered path would answer 404 and the assertion would fail for the wrong reason.
func TestMethodNotAllowed(t *testing.T) {
	r := chi.NewRouter()
	notifications.RegisterRoutes(r, notifications.New(&fakeSvc{}))
	v1.RegisterMethodNotAllowed(r)

	w := handlertest.Do(r.ServeHTTP, http.MethodPost, "/api/v1/notifications", "")
	handlertest.Status(t, w, http.StatusMethodNotAllowed)
}

// Заголовок и тело собираются на сервере: клиент не должен знать формулировок.
func TestGetRendersTitleAndBody(t *testing.T) {
	goalID := int64(5)
	teamID := int64(9)
	svc := &fakeSvc{items: []storenotif.Notification{{
		ID: 1, Type: "goal_comment", Kind: "comment_added",
		ActorDisplayName: "Пётр", ActorAvatarURL: "https://example.com/p.png",
		EntityTitle:   "Снизить отток",
		Payload:       map[string]any{"text": "Уточните метрику"},
		CoalesceCount: 1, CreatedAt: time.Now(), GoalID: &goalID, TeamID: &teamID,
	}}}

	h := notifications.New(svc)
	w := handlertest.Do(h.Get, http.MethodGet, "/api/v1/notifications", "",
		handlertest.Tenant(1), handlertest.UserID(42, "u42"))
	handlertest.Status(t, w, http.StatusOK)

	var got struct {
		Items []struct {
			Title       string `json:"title"`
			Body        string `json:"body"`
			URL         string `json:"url"`
			ActorName   string `json:"actor_name"`
			ActorAvatar string `json:"actor_avatar"`
			Count       int    `json:"count"`
			Read        bool   `json:"read"`
		} `json:"items"`
	}
	handlertest.DecodeJSON(t, w, &got)
	if len(got.Items) != 1 {
		t.Fatalf("got %d items", len(got.Items))
	}
	item := got.Items[0]
	if item.Title == "" || item.Body == "" {
		t.Errorf("сервер обязан отдавать готовый текст: %+v", item)
	}
	if item.URL == "" {
		t.Error("уведомление с целью обязано нести ссылку")
	}
	if item.ActorName != "Пётр" || item.ActorAvatar == "" {
		t.Errorf("актёр не резолвится: %+v", item)
	}
	if svc.gotUserID != 42 {
		t.Errorf("userID переданный в сервис = %d, want 42 (из контекста аутентификации)", svc.gotUserID)
	}
}

// Former member: List already blanks the name/avatar for a former member, but the
// fixture here deliberately still carries a non-empty ActorDisplayName and
// ActorAvatarURL alongside ActorRemoved=true. That isolates the DTO's own guard: a
// fixture with both fields already blank cannot distinguish the handler's placeholder
// logic from the store's blanking, and cannot catch a regression that starts leaking
// ActorAvatarURL straight through once ActorRemoved is set (see toDTO's `if
// !n.ActorRemoved` guard around ActorAvatar).
func TestGetFormerMemberGetsPlaceholder(t *testing.T) {
	svc := &fakeSvc{items: []storenotif.Notification{{
		ID: 2, Type: "goal_comment", Kind: "comment_added",
		ActorDisplayName: "Мария", ActorAvatarURL: "https://example.com/old.png", ActorRemoved: true,
		EntityTitle: "Снизить отток", CoalesceCount: 1, CreatedAt: time.Now(),
	}}}

	h := notifications.New(svc)
	w := handlertest.Do(h.Get, http.MethodGet, "/api/v1/notifications", "",
		handlertest.Tenant(1), handlertest.UserID(42, "u42"))
	handlertest.Status(t, w, http.StatusOK)

	var got struct {
		Items []struct {
			ActorName   string `json:"actor_name"`
			ActorAvatar string `json:"actor_avatar"`
		} `json:"items"`
	}
	handlertest.DecodeJSON(t, w, &got)
	if len(got.Items) != 1 {
		t.Fatalf("got %d items", len(got.Items))
	}
	if got.Items[0].ActorName == "" || got.Items[0].ActorName == "Мария" {
		t.Errorf("бывший участник должен получить нейтральный плейсхолдер, а не пустое имя или настоящее: got %q", got.Items[0].ActorName)
	}
	if got.Items[0].ActorAvatar != "" {
		t.Errorf("бывший участник не должен нести аватар: got %q", got.Items[0].ActorAvatar)
	}
}

// Без tenant-скоупа — 403, а не паника и не пустой список.
func TestGetWithoutScopeIsForbidden(t *testing.T) {
	handlertest.RequiresTenantScope(t, notifications.New(&fakeSvc{}).Get, http.MethodGet, "/api/v1/notifications")
}

func TestGetServiceErrorIs500(t *testing.T) {
	h := notifications.New(&fakeSvc{err: context.DeadlineExceeded})
	w := handlertest.Do(h.Get, http.MethodGet, "/api/v1/notifications", "", handlertest.Tenant(1))
	handlertest.ErrorCode(t, w, http.StatusInternalServerError, "INTERNAL")
}

// The handler no longer decodes the cursor itself (see IMPORTANT 4 / specs/010 §66):
// it forwards the raw string to the service, which is what now answers
// ErrInvalidCursor. This test simulates that by having the fake service return the
// sentinel error, the same way the real service would for "not-base64!!".
func TestGetInvalidCursorIsBadRequest(t *testing.T) {
	h := notifications.New(&fakeSvc{err: notificationsvc.ErrInvalidCursor})
	w := handlertest.Do(h.Get, http.MethodGet, "/api/v1/notifications?cursor=not-base64!!", "", handlertest.Tenant(1))
	handlertest.ErrorCode(t, w, http.StatusBadRequest, "VALIDATION_ERROR")
}

// TestCursorRoundTrips closes the loop end to end: the opaque next_cursor a first
// page hands back to the client must, when fed back as ?cursor=, reach the service
// as that exact same string — the handler must not decode, re-encode, or otherwise
// touch it along the way. The codec itself (encodeCursor/decodeCursor) now lives in
// service/notification and is covered there (see TestCursorRoundTrip in that
// package); what this test protects is the handler boundary staying opaque.
func TestCursorRoundTrips(t *testing.T) {
	page1 := &fakeSvc{
		items: []storenotif.Notification{{
			ID: 1, Type: "goal_comment", Kind: "comment_added",
			ActorDisplayName: "Пётр", EntityTitle: "Снизить отток",
			CoalesceCount: 1, CreatedAt: time.Now(),
		}},
		next: "opaque-token-777",
	}
	h1 := notifications.New(page1)
	w1 := handlertest.Do(h1.Get, http.MethodGet, "/api/v1/notifications", "", handlertest.Tenant(1))
	handlertest.Status(t, w1, http.StatusOK)

	var resp struct {
		NextCursor string `json:"next_cursor"`
	}
	handlertest.DecodeJSON(t, w1, &resp)
	if resp.NextCursor != "opaque-token-777" {
		t.Fatalf("next_cursor = %q, want the service's token passed through verbatim", resp.NextCursor)
	}

	page2 := &fakeSvc{}
	h2 := notifications.New(page2)
	target := "/api/v1/notifications?cursor=" + resp.NextCursor
	w2 := handlertest.Do(h2.Get, http.MethodGet, target, "", handlertest.Tenant(1))
	handlertest.Status(t, w2, http.StatusOK)

	if page2.gotCursor != "opaque-token-777" {
		t.Fatalf("cursor reaching the service = %q, want %q (unmodified round trip)", page2.gotCursor, "opaque-token-777")
	}
}

// Query-string parsing (unread, limit) must reach the service filter: a handler that
// silently ignored either param would still pass every other test in this file.
func TestQueryParamsReachFilter(t *testing.T) {
	svc := &fakeSvc{}
	h := notifications.New(svc)

	w := handlertest.Do(h.Get, http.MethodGet, "/api/v1/notifications?unread=1&limit=5", "", handlertest.Tenant(1))
	handlertest.Status(t, w, http.StatusOK)
	if !svc.gotFilter.UnreadOnly {
		t.Error("unread=1 must set UnreadOnly")
	}
	if svc.gotFilter.Limit != 5 {
		t.Errorf("limit=5 must reach the filter as 5, got %d", svc.gotFilter.Limit)
	}

	// Out-of-range and unset limits both fall back to the default rather than being
	// passed through verbatim.
	svc2 := &fakeSvc{}
	h2 := notifications.New(svc2)
	w2 := handlertest.Do(h2.Get, http.MethodGet, "/api/v1/notifications?limit=500", "", handlertest.Tenant(1))
	handlertest.Status(t, w2, http.StatusOK)
	if svc2.gotFilter.Limit != 20 {
		t.Errorf("out-of-range limit must fall back to the default 20, got %d", svc2.gotFilter.Limit)
	}
	if svc2.gotFilter.UnreadOnly {
		t.Error("unread must default to false when absent")
	}
}

// deleteResult/deleteErr задают исход удаления, а gotDeleteUser/gotDeleteID
// записывают аргументы: тест обязан убедиться, что получатель взят из контекста
// аутентификации, а не из запроса.
func (f *fakeSvc) Delete(_ context.Context, _ domain.TenantScope, userID, id int64) (bool, error) {
	f.gotDeleteUser, f.gotDeleteID = userID, id
	return f.deleteResult, f.deleteErr
}

// Удаление: получатель обязан браться из контекста аутентификации, а не из
// запроса — иначе идентификатор владельца стал бы управляемым клиентом.
func TestDeleteUsesAuthenticatedUser(t *testing.T) {
	svc := &fakeSvc{deleteResult: true}
	h := notifications.New(svc)

	w := handlertest.Do(h.Delete, http.MethodDelete, "/api/v1/notifications/7", "",
		handlertest.Tenant(1), handlertest.UserID(42, "u42"), handlertest.URLParam("id", "7"))
	handlertest.Status(t, w, http.StatusNoContent)

	if svc.gotDeleteUser != 42 {
		t.Errorf("получатель = %d, ожидался 42 из контекста", svc.gotDeleteUser)
	}
	if svc.gotDeleteID != 7 {
		t.Errorf("id = %d, ожидался 7", svc.gotDeleteID)
	}
}

// Чужое и несуществующее обязаны быть неразличимы: иначе по коду ответа можно
// перебором подтвердить существование чужих уведомлений.
func TestDeleteMissingAndForeignAreIndistinguishable(t *testing.T) {
	h := notifications.New(&fakeSvc{deleteResult: false})

	foreign := handlertest.Do(h.Delete, http.MethodDelete, "/api/v1/notifications/7", "",
		handlertest.Tenant(1), handlertest.UserID(42, "u42"), handlertest.URLParam("id", "7"))
	missing := handlertest.Do(h.Delete, http.MethodDelete, "/api/v1/notifications/999", "",
		handlertest.Tenant(1), handlertest.UserID(42, "u42"), handlertest.URLParam("id", "999"))

	handlertest.Status(t, foreign, http.StatusNotFound)
	handlertest.Status(t, missing, http.StatusNotFound)
	if foreign.Body.String() != missing.Body.String() {
		t.Errorf("ответы различимы:\nчужое:        %s\nнесуществующее: %s",
			foreign.Body.String(), missing.Body.String())
	}
}

// Без активного пространства удалять нечего — и это не 404, а отказ в доступе.
func TestDeleteRequiresTenantScope(t *testing.T) {
	h := notifications.New(&fakeSvc{deleteResult: true})
	w := handlertest.Do(h.Delete, http.MethodDelete, "/api/v1/notifications/7", "",
		handlertest.UserID(42, "u42"), handlertest.URLParam("id", "7"))
	handlertest.Status(t, w, http.StatusForbidden)
}

// Нечисловой id — ошибка запроса, а не 404: 404 означал бы «такого уведомления нет».
func TestDeleteRejectsNonNumericID(t *testing.T) {
	h := notifications.New(&fakeSvc{deleteResult: true})
	w := handlertest.Do(h.Delete, http.MethodDelete, "/api/v1/notifications/abc", "",
		handlertest.Tenant(1), handlertest.UserID(42, "u42"), handlertest.URLParam("id", "abc"))
	handlertest.Status(t, w, http.StatusBadRequest)
}
