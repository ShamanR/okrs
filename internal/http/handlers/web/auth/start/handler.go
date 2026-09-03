// Package start serves GET /auth/{provider}/start: begins the OAuth dance.
package start

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	authpkg "okrs/internal/auth"
	webauth "okrs/internal/http/handlers/web/auth"
	"okrs/internal/http/httperr"
)

type Handler struct {
	mgr *authpkg.Manager
}

func New(mgr *authpkg.Manager) *Handler { return &Handler{mgr: mgr} }

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "provider")
	p, ok := h.mgr.Provider(name)
	if !ok {
		http.Error(w, "unknown provider", http.StatusBadRequest)
		return
	}
	state, err := webauth.GenerateState()
	if err != nil {
		// Причина уходит в итоговую запись о запросе, а не в тело ответа.
		_ = httperr.Record(w, httperr.CodeForStatus(http.StatusInternalServerError), err)
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
