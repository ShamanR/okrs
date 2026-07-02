package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestCSRFMiddlewareSetsCookieOnSafeRequest(t *testing.T) {
	mw := NewCSRF()
	h := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/teams", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	cookies := rr.Result().Cookies()
	found := false
	for _, cookie := range cookies {
		if cookie.Name == csrfCookieName && cookie.Value != "" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected csrf cookie to be set")
	}
}

func TestCSRFMiddlewareRotatesCookieOnSafeRequest(t *testing.T) {
	mw := NewCSRF()
	h := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/teams", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "old-token"})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var csrfCookie *http.Cookie
	for _, cookie := range rr.Result().Cookies() {
		if cookie.Name == csrfCookieName {
			csrfCookie = cookie
			break
		}
	}
	if csrfCookie == nil {
		t.Fatal("expected rotated csrf cookie")
	}
	if csrfCookie.Value == "" || csrfCookie.Value == "old-token" {
		t.Fatalf("expected csrf cookie to rotate, got %q", csrfCookie.Value)
	}
	if csrfCookie.MaxAge != 3600 {
		t.Fatalf("expected MaxAge=3600, got %d", csrfCookie.MaxAge)
	}
}

func TestCSRFMiddlewareRejectsWebPostWithoutCookie(t *testing.T) {
	mw := NewCSRF()
	h := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/teams", strings.NewReader("name=test"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestCSRFMiddlewareRejectsInvalidToken(t *testing.T) {
	mw := NewCSRF()
	h := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/teams", strings.NewReader("name=test"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "cookie-token"})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestCSRFMiddlewareAllowsHeaderToken(t *testing.T) {
	mw := NewCSRF()
	h := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	token := "csrf-token"
	req := httptest.NewRequest(http.MethodPost, "/api/v1/goals/1", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: token})
	req.Header.Set(csrfHeaderName, token)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rr.Code)
	}
}

func TestCSRFMiddlewareAllowsFormToken(t *testing.T) {
	mw := NewCSRF()
	h := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	token := "csrf-token"
	form := url.Values{}
	form.Set(csrfFieldName, token)
	req := httptest.NewRequest(http.MethodPost, "/teams", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: token})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rr.Code)
	}
}

func TestCSRFMiddlewareRejectsAPIPostWithoutCookie(t *testing.T) {
	mw := NewCSRF()
	h := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/krs/1/comments", strings.NewReader(`{"text":"ok"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("expected json content type, got %q", ct)
	}
}

// A request authenticated by a Bearer token (machine/provisioning caller) is not
// cookie-ambient, so it cannot be CSRF-forged and must pass without a CSRF cookie/header.
func TestCSRFMiddlewareAllowsBearerAuthWithoutCookie(t *testing.T) {
	mw := NewCSRF()
	called := false
	h := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/tenants", strings.NewReader(`{"name":"x","slug":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret-token")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if !called || rr.Code != http.StatusCreated {
		t.Fatalf("bearer-authenticated POST must bypass CSRF; called=%v code=%d", called, rr.Code)
	}
}

func TestCSRFMiddlewareReturnsJSONErrorForAPI(t *testing.T) {
	mw := NewCSRF()
	h := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/krs/1/comments", strings.NewReader(`{"text":"ok"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "cookie-token"})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("expected json content type, got %q", ct)
	}
	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected JSON body, got error: %v", err)
	}
}
