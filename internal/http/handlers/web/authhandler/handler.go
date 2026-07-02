package authhandler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"html/template"
	"log/slog"
	"net/http"
	"strings"

	"okrs/internal/auth"
	"okrs/internal/domain"

	"github.com/go-chi/chi/v5"
)

// Onboarder routes a freshly logged-in user: redeem an invite token or register a new user
// into the default tenant. *service.OnboardingService satisfies it.
type Onboarder interface {
	ClaimInvitation(ctx context.Context, rawToken string, userID int64) (*domain.Membership, error)
	EnsureRegistration(ctx context.Context, userID int64) (bool, error)
}

// sessionWriter focuses a session on a tenant after claim. *store.SessionRepository satisfies it.
type sessionWriter interface {
	SetActiveTenant(ctx context.Context, sessionID string, tenantID int64) error
}

type Handler struct {
	mgr     *auth.Manager
	tmpl    *template.Template
	logger  *slog.Logger
	onboard Onboarder
	sessions sessionWriter
}

func New(mgr *auth.Manager, tmpl *template.Template, logger *slog.Logger, onboard Onboarder, sessions sessionWriter) *Handler {
	return &Handler{mgr: mgr, tmpl: tmpl, logger: logger, onboard: onboard, sessions: sessions}
}

func (h *Handler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	providers := h.mgr.Providers()
	if len(providers) == 1 {
		http.Redirect(w, r, "/auth/"+providers[0].Name()+"/start?next="+r.URL.Query().Get("next"), http.StatusFound)
		return
	}
	if err := h.tmpl.ExecuteTemplate(w, "login", map[string]any{
		"PageTitle": "Войти",
		"Providers": providers,
		"Next":      r.URL.Query().Get("next"),
	}); err != nil {
		h.logger.Error("login template", slog.String("error", err.Error()))
	}
}

func (h *Handler) HandleProviderStart(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "provider")
	p, ok := h.mgr.Provider(name)
	if !ok {
		http.Error(w, "unknown provider", http.StatusBadRequest)
		return
	}
	state, err := generateState()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "okrs_oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   300,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	next := r.URL.Query().Get("next")
	if next != "" {
		http.SetCookie(w, &http.Cookie{
			Name:     "okrs_oauth_next",
			Value:    next,
			Path:     "/",
			MaxAge:   300,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
	}
	http.Redirect(w, r, p.AuthURL(state), http.StatusFound)
}

func (h *Handler) HandleCallback(w http.ResponseWriter, r *http.Request) {
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
	auth.SetSessionCookie(w, h.mgr.CookieName(), sess.ID, h.mgr.SessionTTL(), secure)

	h.onboardAfterLogin(w, r, user.ID, sess.ID)

	h.logger.Info("user logged in",
		slog.Int64("user_id", user.ID),
		slog.String("provider", identity.Provider),
		slog.String("display_name", user.DisplayName),
	)

	next := "/teamOkrs"
	if nc, err := r.Cookie("okrs_oauth_next"); err == nil && nc.Value != "" {
		if safeRedirectPath(nc.Value) {
			next = nc.Value
		}
		http.SetCookie(w, &http.Cookie{Name: "okrs_oauth_next", MaxAge: -1, Path: "/"})
	}
	http.Redirect(w, r, next, http.StatusFound)
}

// HandleInvite redeems an invite-link token. An already-authenticated visitor is claimed
// immediately (and their session is focused on the invite's tenant) and sent to the app;
// an anonymous visitor gets the token stashed in a short-lived cookie and is sent to login,
// where the callback redeems it. Invalid tokens never block: authenticated visitors are
// bounced to the app (RequireMembership routes them onward), anonymous ones to login.
func (h *Handler) HandleInvite(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if user := auth.UserFromContext(r.Context()); user != nil {
		if m, err := h.onboard.ClaimInvitation(r.Context(), token, user.ID); err == nil {
			if sess := auth.SessionFromContext(r.Context()); sess != nil {
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

func (h *Handler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(h.mgr.CookieName())
	if err == nil {
		_ = h.mgr.Logout(r.Context(), cookie.Value)
	}
	auth.ClearSessionCookie(w, h.mgr.CookieName())
	http.Redirect(w, r, "/login", http.StatusFound)
}

// safeRedirectPath returns true only for relative paths on this host,
// preventing open-redirect attacks via a crafted next parameter.
func safeRedirectPath(next string) bool {
	if next == "" {
		return false
	}
	// Must start with / but not // (protocol-relative URL)
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return false
	}
	return true
}

func generateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
