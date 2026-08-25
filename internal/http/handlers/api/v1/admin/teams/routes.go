package teams

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/api/v1/admin/teams", h.Get)
	r.Post("/api/v1/admin/teams", h.Post)
	r.Patch("/api/v1/admin/teams/{teamID}", h.Patch)
	r.Delete("/api/v1/admin/teams/{teamID}", h.Delete)
}
