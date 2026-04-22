package authhandler

import (
	"crypto/rand"
	"encoding/hex"
	"html/template"
	"log/slog"
	"net/http"

	"okrs/internal/auth"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	mgr    *auth.Manager
	tmpl   *template.Template
	logger *slog.Logger
}

func New(mgr *auth.Manager, tmpl *template.Template, logger *slog.Logger) *Handler {
	return &Handler{mgr: mgr, tmpl: tmpl, logger: logger}
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

	h.logger.Info("user logged in",
		slog.Int64("user_id", user.ID),
		slog.String("provider", identity.Provider),
		slog.String("display_name", user.DisplayName),
	)

	next := "/teamOkrs"
	if nc, err := r.Cookie("okrs_oauth_next"); err == nil && nc.Value != "" {
		next = nc.Value
		http.SetCookie(w, &http.Cookie{Name: "okrs_oauth_next", MaxAge: -1, Path: "/"})
	}
	http.Redirect(w, r, next, http.StatusFound)
}

func (h *Handler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(h.mgr.CookieName())
	if err == nil {
		_ = h.mgr.Logout(r.Context(), cookie.Value)
	}
	auth.ClearSessionCookie(w, h.mgr.CookieName())
	http.Redirect(w, r, "/login", http.StatusFound)
}

func generateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
