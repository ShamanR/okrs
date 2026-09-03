// Package callback serves GET /auth/{provider}/callback: completes the OAuth dance and onboards the user.
package callback

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	authpkg "okrs/internal/auth"
	"okrs/internal/core/domain"
	webauth "okrs/internal/http/handlers/web/auth"
	"okrs/internal/http/httperr"
	"okrs/internal/platform/logging"
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

	start := time.Now()
	identity, err := p.Exchange(r.Context(), code)
	logging.ExternalCall(r.Context(), "oauth_provider", time.Since(start), err,
		slog.String("provider", name))
	if err != nil {
		h.logger.WarnContext(r.Context(), "oauth exchange failed",
			slog.String(logging.KeyEvent, logging.EventAuthFailed),
			slog.String("provider", name),
			slog.String("err", err.Error()))
		http.Error(w, "authentication failed", http.StatusBadGateway)
		return
	}

	user, sess, err := h.mgr.Login(r.Context(), identity, r.UserAgent(), r.RemoteAddr)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "login failed",
			slog.String(logging.KeyEvent, logging.EventAuthFailed),
			slog.String("provider", name),
			slog.String("err", err.Error()))
		_ = httperr.Record(w, httperr.CodeForStatus(http.StatusInternalServerError), err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	secure := r.TLS != nil
	authpkg.SetSessionCookie(w, h.mgr.CookieName(), sess.ID, h.mgr.SessionTTL(), secure)

	h.onboardAfterLogin(w, r, user.ID, sess.ID)

	h.logLogin(r.Context(), user.ID, identity.Provider)

	next := "/teamOkrs"
	if nc, err := r.Cookie("okrs_oauth_next"); err == nil && nc.Value != "" {
		if webauth.SafeRedirectPath(nc.Value) {
			next = nc.Value
		}
		http.SetCookie(w, &http.Cookie{Name: "okrs_oauth_next", MaxAge: -1, Path: "/"})
	}
	http.Redirect(w, r, next, http.StatusFound)
}

// logLogin фиксирует успешный вход.
//
// Отдельный метод, а не вызов логгера по месту: состав полей этой записи —
// требование безопасности, а не деталь оформления. Пользователь опознаётся
// числовым идентификатором; отображаемое имя и адрес почты в лог не попадают
// (раньше здесь писался display_name). Иначе это правило нечем закрепить:
// Get требует настоящего Manager с провайдером и базой, поэтому запись о входе
// оставалась бы единственной auth-записью без гарда.
func (h *Handler) logLogin(ctx context.Context, userID int64, provider string) {
	h.logger.InfoContext(ctx, "user logged in",
		slog.String(logging.KeyEvent, logging.EventAuthLogin),
		slog.Int64(logging.KeyActorID, userID),
		slog.String("provider", provider),
	)
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
		h.logger.ErrorContext(r.Context(), "ensure registration failed",
			slog.String(logging.KeyEvent, logging.EventAuthLogin),
			slog.Int64(logging.KeyActorID, userID),
			slog.String("err", err.Error()))
	}
}
