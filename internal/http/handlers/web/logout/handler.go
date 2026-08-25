// Package logout serves POST /logout.
package logout

import (
	"net/http"

	authpkg "okrs/internal/auth"
)

type Handler struct {
	mgr *authpkg.Manager
}

func New(mgr *authpkg.Manager) *Handler { return &Handler{mgr: mgr} }

func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(h.mgr.CookieName())
	if err == nil {
		_ = h.mgr.Logout(r.Context(), cookie.Value)
	}
	authpkg.ClearSessionCookie(w, h.mgr.CookieName())
	http.Redirect(w, r, "/login", http.StatusFound)
}
