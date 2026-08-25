package okrs

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/api/v1/teams/{teamID}/okrs", h.Get)
}
