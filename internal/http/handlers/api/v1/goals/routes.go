package goals

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/api/v1/goals/{goalID}", h.Get)
	r.Post("/api/v1/goals/{goalID}", h.Post)
	r.Delete("/api/v1/goals/{goalID}", h.Delete)
}
