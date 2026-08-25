package feedback

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/api/v1/admin/settings/feedback", h.Get)
	r.Post("/api/v1/admin/settings/feedback", h.Post)
}
