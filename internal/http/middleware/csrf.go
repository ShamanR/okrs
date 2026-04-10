package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"
)

const (
	csrfCookieName = "okr_csrf_token"
	csrfHeaderName = "X-CSRF-Token"
	csrfFieldName  = "csrf_token"
)

type CSRFMiddleware struct{}

func NewCSRF() *CSRFMiddleware {
	return &CSRFMiddleware{}
}

func (m *CSRFMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := ensureCSRFCookie(w, r)
		if isUnsafeMethod(r.Method) {
			requestToken := strings.TrimSpace(r.Header.Get(csrfHeaderName))
			if requestToken == "" {
				requestToken = strings.TrimSpace(r.FormValue(csrfFieldName))
			}
			if subtle.ConstantTimeCompare([]byte(token), []byte(requestToken)) != 1 {
				http.Error(w, "CSRF token is missing or invalid", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func ensureCSRFCookie(w http.ResponseWriter, r *http.Request) string {
	if cookie, err := r.Cookie(csrfCookieName); err == nil && strings.TrimSpace(cookie.Value) != "" {
		return cookie.Value
	}
	token := generateCSRFToken()
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
	})
	return token
}

func generateCSRFToken() string {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		panic("failed to generate CSRF token")
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func isUnsafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return false
	default:
		return true
	}
}
