package restore

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Post("/api/v1/admin/teams/{teamID}/restore", h.Post)
}
