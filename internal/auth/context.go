package auth

import (
	"context"

	"okrs/internal/core/domain"
)

type contextKey int

const (
	userContextKey    contextKey = iota
	sessionContextKey contextKey = iota
	tenantContextKey
	activeRoleContextKey
)

func withUser(ctx context.Context, u *domain.User) context.Context {
	return context.WithValue(ctx, userContextKey, u)
}

func withSession(ctx context.Context, s *domain.AuthSession) context.Context {
	return context.WithValue(ctx, sessionContextKey, s)
}

// UserFromContext returns the authenticated user or nil (no-auth or unauthenticated).
func UserFromContext(ctx context.Context) *domain.User {
	u, _ := ctx.Value(userContextKey).(*domain.User)
	return u
}

// SessionFromContext returns the current session or nil.
func SessionFromContext(ctx context.Context) *domain.AuthSession {
	s, _ := ctx.Value(sessionContextKey).(*domain.AuthSession)
	return s
}

// WithUser injects a user into the context. Used by middleware and tests.
func WithUser(ctx context.Context, u *domain.User) context.Context {
	return withUser(ctx, u)
}

// UserIDFromContext returns the current user ID or the anonymous-local system user ID.
func UserIDFromContext(ctx context.Context) int64 {
	if u := UserFromContext(ctx); u != nil {
		return u.ID
	}
	return domain.SystemUserAnonymous
}

// WithSession injects a session into the context. Used by middleware and tests.
func WithSession(ctx context.Context, s *domain.AuthSession) context.Context {
	return withSession(ctx, s)
}

// WithTenant injects the resolved active tenant into the context (HTTP boundary only).
func WithTenant(ctx context.Context, t *domain.Tenant) context.Context {
	return context.WithValue(ctx, tenantContextKey, t)
}

// TenantFromContext returns the resolved active tenant or nil.
func TenantFromContext(ctx context.Context) *domain.Tenant {
	t, _ := ctx.Value(tenantContextKey).(*domain.Tenant)
	return t
}

// TenantScopeFromContext derives the tenant scope from the tenant in context.
// HTTP-boundary extractor: only handlers call it, then pass the scope explicitly
// down to services/repositories. Services and repositories never read it from context.
func TenantScopeFromContext(ctx context.Context) (domain.TenantScope, bool) {
	t := TenantFromContext(ctx)
	if t == nil {
		return domain.TenantScope{}, false
	}
	return domain.TenantScope{TenantID: t.ID}, true
}

// WithActiveRole injects the user's role in the active tenant into the context.
func WithActiveRole(ctx context.Context, role domain.Role) context.Context {
	return context.WithValue(ctx, activeRoleContextKey, role)
}

// ActiveRoleFromContext returns the user's role in the active tenant.
func ActiveRoleFromContext(ctx context.Context) (domain.Role, bool) {
	role, ok := ctx.Value(activeRoleContextKey).(domain.Role)
	return role, ok
}
