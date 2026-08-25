package members

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Post("/api/v1/system/tenants/{id}/members", h.Post)
	r.Get("/api/v1/system/tenants/{id}/members", h.Get)
	r.Delete("/api/v1/system/tenants/{id}/members/{userID}", h.Delete)
}
