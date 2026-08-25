// Package login serves GET /login: the provider chooser, or a straight redirect when only one provider is configured.
package login

import (
	"html/template"
	"log/slog"
	"net/http"

	authpkg "okrs/internal/auth"
)

type Handler struct {
	mgr    *authpkg.Manager
	tmpl   *template.Template
	logger *slog.Logger
}

func New(mgr *authpkg.Manager, tmpl *template.Template, logger *slog.Logger) *Handler {
	return &Handler{mgr: mgr, tmpl: tmpl, logger: logger}
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
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
