// Package invite serves GET /invite/{token}: redeems an invitation and focuses the session on its tenant.
package invite

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	authpkg "okrs/internal/auth"
	"okrs/internal/core/domain"
)

// Onboarder routes a freshly logged-in user: redeem an invite token or register a new
// user into the default tenant. *onboarding.Service satisfies it.
type Onboarder interface {
	ClaimInvitation(ctx context.Context, rawToken string, userID int64) (*domain.Membership, error)
	EnsureRegistration(ctx context.Context, userID int64) (bool, error)
}

// SessionWriter focuses a session on a tenant after a claim.
type SessionWriter interface {
	SetActiveTenant(ctx context.Context, sessionID string, tenantID int64) error
}

type Handler struct {
	onboard  Onboarder
	sessions SessionWriter
}

func New(onboard Onboarder, sessions SessionWriter) *Handler {
	return &Handler{onboard: onboard, sessions: sessions}
}

// Get redeems an invite-link token. An already-authenticated visitor is claimed
// immediately (and their session is focused on the invite's tenant) and sent to the app;
// an anonymous visitor gets the token stashed in a short-lived cookie and is sent to login,
// where the callback redeems it. Invalid tokens never block: authenticated visitors are
// bounced to the app (RequireMembership routes them onward), anonymous ones to login.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if user := authpkg.UserFromContext(r.Context()); user != nil {
		if m, err := h.onboard.ClaimInvitation(r.Context(), token, user.ID); err == nil {
			if sess := authpkg.SessionFromContext(r.Context()); sess != nil {
				_ = h.sessions.SetActiveTenant(r.Context(), sess.ID, m.TenantID)
			}
		}
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "okrs_invite",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   600,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/login?next=/", http.StatusFound)
}
