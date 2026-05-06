package admin

import (
	"html/template"
	"log/slog"
	"net/http"
	"strconv"

	"okrs/internal/auth"
	"okrs/internal/store"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	store  *store.Store
	tmpl   *template.Template
	policy *auth.PolicyEvaluator
	logger *slog.Logger
}

func New(st *store.Store, tmpl *template.Template, policy *auth.PolicyEvaluator, logger *slog.Logger) *Handler {
	return &Handler{store: st, tmpl: tmpl, policy: policy, logger: logger}
}

func (h *Handler) HandleIndex(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin/access", http.StatusFound)
}

func (h *Handler) HandleAccess(w http.ResponseWriter, r *http.Request) {
	users, err := h.store.ListUsers(r.Context())
	if err != nil {
		h.logger.Error("list users", slog.String("error", err.Error()))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := h.tmpl.ExecuteTemplate(w, "admin-access", map[string]any{
		"PageTitle":       "Управление доступом",
		"ContentTemplate": "admin-access-content",
		"CurrentUser":     auth.UserFromContext(r.Context()),
		"Users":           users,
	}); err != nil {
		h.logger.Error("admin-access template", slog.String("error", err.Error()))
	}
}

func (h *Handler) HandleUserDetail(w http.ResponseWriter, r *http.Request) {
	userID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	if err != nil {
		http.Error(w, "bad user id", http.StatusBadRequest)
		return
	}
	user, err := h.store.GetUser(r.Context(), userID)
	if err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	grants, err := h.store.ListUserGrants(r.Context(), userID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := h.tmpl.ExecuteTemplate(w, "admin-user-detail", map[string]any{
		"PageTitle":   "Пользователь",
		"User":        user,
		"Grants":      grants,
		"CurrentUser": auth.UserFromContext(r.Context()),
	}); err != nil {
		h.logger.Error("admin-user-detail template", slog.String("error", err.Error()))
	}
}
