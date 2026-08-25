package general

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/api/v1/admin/settings/general", h.Get)
	r.Post("/api/v1/admin/settings/general", h.Post)
}
