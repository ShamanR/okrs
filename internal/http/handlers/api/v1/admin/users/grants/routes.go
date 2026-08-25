package grants

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/api/v1/admin/users/{userID}/grants", h.Get)
	r.Post("/api/v1/admin/users/{userID}/grants", h.Post)
	r.Delete("/api/v1/admin/users/{userID}/grants/{teamID}", h.Delete)
}
