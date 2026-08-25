// Package callback serves GET /auth/{provider}/callback: completes the OAuth dance and onboards the user.
package callback

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	authpkg "okrs/internal/auth"
	"okrs/internal/core/domain"
	webauth "okrs/internal/http/handlers/web/auth"
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
	mgr      *authpkg.Manager
	logger   *slog.Logger
	onboard  Onboarder
	sessions SessionWriter
}

func New(mgr *authpkg.Manager, logger *slog.Logger, onboard Onboarder, sessions SessionWriter) *Handler {
	return &Handler{mgr: mgr, logger: logger, onboard: onboard, sessions: sessions}
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "provider")
	p, ok := h.mgr.Provider(name)
	if !ok {
		http.Error(w, "unknown provider", http.StatusBadRequest)
		return
	}

	stateCookie, err := r.Cookie("okrs_oauth_state")
	if err != nil || r.URL.Query().Get("state") != stateCookie.Value {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "okrs_oauth_state", MaxAge: -1, Path: "/"})

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	identity, err := p.Exchange(r.Context(), code)
	if err != nil {
		h.logger.Error("oauth exchange", slog.String("provider", name), slog.String("error", err.Error()))
		http.Error(w, "authentication failed", http.StatusBadGateway)
		return
	}

	user, sess, err := h.mgr.Login(r.Context(), identity, r.UserAgent(), r.RemoteAddr)
	if err != nil {
		h.logger.Error("login", slog.String("error", err.Error()))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	secure := r.TLS != nil
	authpkg.SetSessionCookie(w, h.mgr.CookieName(), sess.ID, h.mgr.SessionTTL(), secure)

	h.onboardAfterLogin(w, r, user.ID, sess.ID)

	h.logger.Info("user logged in",
		slog.Int64("user_id", user.ID),
		slog.String("provider", identity.Provider),
		slog.String("display_name", user.DisplayName),
	)

	next := "/teamOkrs"
	if nc, err := r.Cookie("okrs_oauth_next"); err == nil && nc.Value != "" {
		if webauth.SafeRedirectPath(nc.Value) {
			next = nc.Value
		}
		http.SetCookie(w, &http.Cookie{Name: "okrs_oauth_next", MaxAge: -1, Path: "/"})
	}
	http.Redirect(w, r, next, http.StatusFound)
}

// onboardAfterLogin redeems a pending invite (priority) or registers a new user into the
// default tenant. It never blocks login: an invalid invite or a no-default-tenant case just
// leaves the user without a membership, and RequireMembershipMiddleware routes them to
// /no-access on the next request (the single source of truth for that gate).
func (h *Handler) onboardAfterLogin(w http.ResponseWriter, r *http.Request, userID int64, sessionID string) {
	if ic, err := r.Cookie("okrs_invite"); err == nil && ic.Value != "" {
		http.SetCookie(w, &http.Cookie{Name: "okrs_invite", MaxAge: -1, Path: "/"})
		if m, err := h.onboard.ClaimInvitation(r.Context(), ic.Value, userID); err == nil {
			_ = h.sessions.SetActiveTenant(r.Context(), sessionID, m.TenantID)
			return
		}
		// Invalid invite falls through to normal new-user routing below.
	}
	if _, err := h.onboard.EnsureRegistration(r.Context(), userID); err != nil {
		h.logger.Error("ensure registration", slog.String("error", err.Error()))
	}
}
