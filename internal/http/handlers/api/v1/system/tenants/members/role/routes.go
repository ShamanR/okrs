package role

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Put("/api/v1/system/tenants/{id}/members/{userID}/role", h.Put)
}
