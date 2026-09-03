package auth

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/httproute"
	"okrs/internal/platform/logging"
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
			logDenied(r, "no authenticated user")
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
			logDenied(r, "active role is not tenant admin")
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
			if user == nil {
				logDenied(r, "no system-admin session and no valid provisioning token")
			} else {
				logDenied(r, "user is not a system admin")
			}
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
			if tn == nil {
				logDenied(r, "no active tenant membership")
			} else {
				logDenied(r, "tenant is not active")
			}
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

// Логирование запросов живёт в internal/http/middleware: это не забота
// аутентификации, а обёртка ответа теперь несёт ещё и код с причиной
// ошибки ответа.

// logDenied фиксирует отказ в доступе отдельной записью.
//
// Отдельная запись, а не только повышенный уровень итоговой записи о запросе:
// на event=authz_denied строится алерт «кого и куда перестали пускать», и он не
// должен зависеть от разбора статуса и кода ошибки.
//
// Логгер берётся из контекста, потому что сигнатуры гейтов заданы типом
// func(http.Handler) http.Handler и места для зависимости в них нет; вне цепочки
// middleware (юнит-тест гейта) FromContext вернёт slog.Default(), и отказ всё равно
// не потеряется.
func logDenied(r *http.Request, reason string) {
	ctx := r.Context()
	attrs := []any{
		slog.String(logging.KeyEvent, logging.EventAuthzDenied),
		slog.String("reason", reason),
		slog.String("method", r.Method),
		// Шаблон маршрута, а не конкретный путь: за гейтом может оказаться
		// маршрут, чей параметр является учётными данными, и отказ записал бы
		// их в лог. Гейты стоят внутри группы роутера, где шаблон уже известен.
		slog.String("path", httproute.Pattern(r)),
	}
	if u := UserFromContext(ctx); u != nil {
		// Пользователь опознаётся числовым идентификатором: адрес почты и имя
		// в логи не попадают.
		attrs = append(attrs, slog.Int64(logging.KeyActorID, u.ID))
	}
	logging.FromContext(ctx).WarnContext(ctx, "access denied", attrs...)
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
