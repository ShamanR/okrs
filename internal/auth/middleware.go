package auth

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"okrs/internal/core/domain"
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

// RequireTenantAdminMiddleware gates the tenant-admin plane (/admin, /api/v1/admin/*).
// It admits the request only when the active role in the resolved tenant is admin
// (set by TenantResolveMiddleware). A plain member of the tenant gets 403.
func RequireTenantAdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role, ok := ActiveRoleFromContext(r.Context())
		if !ok || role != domain.RoleAdmin {
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

// RequireSystemAdminMiddleware gates the system-admin plane (/system, /api/v1/system/*).
// It is the SOLE gate for the plane (spec 040) and must NOT be chained behind RequireAuth:
// RequireAuth would 401/redirect a token-only machine caller (no session cookie) before this
// gate could honor the token, breaking cross-tenant provisioning in AUTH_MODE=enabled.
//
// It admits the request when the session user is a system admin, OR when a non-empty
// provisioning token is configured and the request carries "Authorization: Bearer <token>"
// (machine/control-plane callers). An unauthenticated browser (no session user at all) is
// redirected to /login so it can sign in as a system admin — the redirect RequireAuth used to
// provide. Every other caller — authenticated non-admin, or any API request without a valid
// token — gets 403.
func RequireSystemAdminMiddleware(provisioningToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := UserFromContext(r.Context())
			if user != nil && user.IsSystemAdmin {
				next.ServeHTTP(w, r)
				return
			}
			if provisioningToken != "" {
				const prefix = "Bearer "
				h := r.Header.Get("Authorization")
				if strings.HasPrefix(h, prefix) &&
					subtle.ConstantTimeCompare([]byte(h[len(prefix):]), []byte(provisioningToken)) == 1 {
					next.ServeHTTP(w, r)
					return
				}
			}
			// No system-admin session and no valid token. A browser with no session user at
			// all (AUTH_MODE=enabled, cookie missing/expired) is sent to login — it may just
			// need to sign in as a system admin. In disabled mode anonymous-local is always
			// present (non-nil), so this branch is skipped and the request falls through to
			// 403, matching the spec that disabled-mode system access requires the token.
			if user == nil && !isAPIRequest(r) {
				http.Redirect(w, r, "/login?next="+r.URL.RequestURI(), http.StatusFound)
				return
			}
			if isAPIRequest(r) {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			http.Error(w, "403 Forbidden", http.StatusForbidden)
		})
	}
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
			scope, _ := TenantScopeFromContext(r.Context())
			ctx, _ := policy.LoadScope(r.Context(), scope, user, mgr.Config())
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
					slog.Bool("is_system_admin", user.IsSystemAdmin),
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
