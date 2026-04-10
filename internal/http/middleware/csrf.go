package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"

	v1 "okrs/internal/http/handlers/api/v1"
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
		cookieToken, hasCookie := readCSRFCookie(r)

		if isUnsafeMethod(r.Method) {
			if !hasCookie {
				if isAPIRoute(r) {
					next.ServeHTTP(w, r)
					return
				}
				writeCSRFError(w, r)
				return
			}
			requestToken := strings.TrimSpace(r.Header.Get(csrfHeaderName))
			if requestToken == "" {
				requestToken = strings.TrimSpace(r.FormValue(csrfFieldName))
			}
			if subtle.ConstantTimeCompare([]byte(cookieToken), []byte(requestToken)) != 1 {
				writeCSRFError(w, r)
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		ensureCSRFCookie(w, r)
		next.ServeHTTP(w, r)
	})
}

func readCSRFCookie(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(csrfCookieName)
	if err != nil {
		return "", false
	}
	value := strings.TrimSpace(cookie.Value)
	if value == "" {
		return "", false
	}
	return value, true
}

func ensureCSRFCookie(w http.ResponseWriter, r *http.Request) string {
	if cookie, ok := readCSRFCookie(r); ok {
		return cookie
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

func writeCSRFError(w http.ResponseWriter, r *http.Request) {
	if isAPIRoute(r) {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "csrf token is missing or invalid", nil)
		return
	}
	http.Error(w, "CSRF token is missing or invalid", http.StatusForbidden)
}

func isAPIRoute(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/api/v1/")
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
