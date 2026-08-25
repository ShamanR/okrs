package activate

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Post("/api/v1/periods/{periodID}/teams/activate", h.Post)
}
