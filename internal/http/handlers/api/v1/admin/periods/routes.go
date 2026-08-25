package periods

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/api/v1/admin/periods", h.Get)
	r.Post("/api/v1/admin/periods", h.Post)
	r.Patch("/api/v1/admin/periods/{periodID}", h.Patch)
	r.Delete("/api/v1/admin/periods/{periodID}", h.Delete)
}
