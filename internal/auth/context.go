package auth

import (
	"context"

	"okrs/internal/domain"
)

type contextKey int

const (
	userContextKey    contextKey = iota
	sessionContextKey contextKey = iota
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
