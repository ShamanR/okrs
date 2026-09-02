// Package logout serves POST /logout.
package logout

import (
	"log/slog"
	"net/http"

	authpkg "okrs/internal/auth"
	"okrs/internal/platform/logging"
)

type Handler struct {
	mgr *authpkg.Manager
}

func New(mgr *authpkg.Manager) *Handler { return &Handler{mgr: mgr} }

func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cookie, err := r.Cookie(h.mgr.CookieName())
	if err == nil {
		_ = h.mgr.Logout(ctx, cookie.Value)
		// Логгер из контекста: у этого обработчика нет своего, а выход —
		// событие, которое расследование инцидента обязано видеть.
		attrs := []any{slog.String(logging.KeyEvent, logging.EventAuthLogout)}
		if u := authpkg.UserFromContext(ctx); u != nil {
			attrs = append(attrs, slog.Int64(logging.KeyActorID, u.ID))
		}
		logging.FromContext(ctx).InfoContext(ctx, "user logged out", attrs...)
	}
	authpkg.ClearSessionCookie(w, h.mgr.CookieName())
	http.Redirect(w, r, "/login", http.StatusFound)
}
