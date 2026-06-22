package auth

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"okrs/internal/domain"
)

// SessionMiddleware resolves the session cookie and loads user+session into context.
// Never blocks the request — unauthenticated requests simply get no user in context.
func SessionMiddleware(mgr *Manager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(mgr.CookieName())
			if err == nil && cookie.Value != "" {
				user, sess, err := mgr.ResolveSession(r.Context(), cookie.Value)
				if err == nil {
					ctx := withUser(r.Context(), user)
					ctx = withSession(ctx, sess)
					r = r.WithContext(ctx)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAuthMiddleware redirects unauthenticated requests to /login.
// API requests (Accept: application/json or /api/ prefix) get a 401.
func RequireAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if UserFromContext(r.Context()) == nil {
			if isAPIRequest(r) {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/login?next="+r.URL.RequestURI(), http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAdminMiddleware returns 403 for non-admin users.
func RequireAdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		if user == nil || !user.IsAdmin {
			if isAPIRequest(r) {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			http.Error(w, "403 Forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ScopeMiddleware loads the user's allowed hierarchy into context.
func ScopeMiddleware(policy *PolicyEvaluator, mgr *Manager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := UserFromContext(r.Context())
			if user == nil {
				next.ServeHTTP(w, r)
				return
			}
			ctx, _ := policy.LoadScope(r.Context(), user, mgr.Config())
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// TenantResolveMiddleware resolves the active tenant for the authenticated user and
// injects it (plus the user's role in that tenant) into the request context.
// On no membership / lookup error it leaves the tenant unset; RequireMembership decides.
func TenantResolveMiddleware(resolver *TenantResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := UserFromContext(r.Context())
			if user == nil {
				next.ServeHTTP(w, r)
				return
			}
			sess := SessionFromContext(r.Context())
			tn, role, err := resolver.Resolve(r.Context(), user, sess)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			ctx := WithTenant(r.Context(), tn)
			ctx = WithActiveRole(ctx, role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireMembershipMiddleware blocks requests without an active, non-suspended tenant.
func RequireMembershipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tn := TenantFromContext(r.Context())
		if tn == nil || tn.Status != domain.TenantActive {
			if isAPIRequest(r) {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			http.Redirect(w, r, "/no-access", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// AccessLogMiddleware logs every request with auth fields.
func AccessLogMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rw, r)

			user := UserFromContext(r.Context())
			attrs := []any{
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rw.status),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			}
			if user != nil {
				attrs = append(attrs,
					slog.Bool("authenticated", true),
					slog.Int64("user_id", user.ID),
					slog.String("provider", user.Provider),
					slog.Bool("is_admin", user.IsAdmin),
				)
			} else {
				attrs = append(attrs, slog.Bool("authenticated", false))
			}
			logger.Info("request", attrs...)
		})
	}
}

// SetSessionCookie writes the session cookie to the response.
func SetSessionCookie(w http.ResponseWriter, name, value string, ttl time.Duration, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearSessionCookie removes the session cookie.
func ClearSessionCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func isAPIRequest(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/api/") ||
		strings.Contains(r.Header.Get("Accept"), "application/json")
}

// AnonymousUserMiddleware injects the anonymous-local system user when auth is disabled.
func AnonymousUserMiddleware(next http.Handler) http.Handler {
	anon := &domain.User{
		ID:          domain.SystemUserAnonymous,
		Provider:    "system",
		Subject:     "anonymous-local",
		DisplayName: "Anonymous",
		IsAdmin:     true, // no-auth mode: everyone is admin
	}
	tenant := &domain.Tenant{
		ID:     1,
		Slug:   "default",
		Status: domain.TenantActive,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := withUser(r.Context(), anon)
		ctx = WithTenant(ctx, tenant)
		ctx = WithActiveRole(ctx, domain.RoleAdmin)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}
