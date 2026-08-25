package joinrequest

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Post("/api/v1/onboarding/join-request", h.Post)
}
