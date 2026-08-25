package users

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/api/v1/admin/users", h.Get)
	r.Get("/api/v1/admin/users/{userID}", h.GetOne)
}
