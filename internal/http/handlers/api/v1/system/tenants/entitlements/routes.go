package entitlements

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Put("/api/v1/system/tenants/{id}/entitlements", h.Put)
	r.Get("/api/v1/system/tenants/{id}/entitlements", h.Get)
}
