package memberships

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/api/v1/session/memberships", h.Get)
	r.Delete("/api/v1/session/memberships/{tenantID}", h.Delete)
}
