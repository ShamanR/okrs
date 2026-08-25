package share

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Post("/api/v1/goals/{goalID}/share", h.Post)
	r.Delete("/api/v1/goals/{goalID}/share/{teamID}", h.Delete)
}
