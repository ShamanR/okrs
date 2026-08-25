package tenants

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Post("/api/v1/system/tenants", h.Post)
	r.Get("/api/v1/system/tenants", h.Get)
	r.Patch("/api/v1/system/tenants/{id}", h.Patch)
}
