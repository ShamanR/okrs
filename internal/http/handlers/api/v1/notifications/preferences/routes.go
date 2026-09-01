package preferences

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/api/v1/notifications/preferences", h.Get)
	r.Put("/api/v1/notifications/preferences", h.Put)
}
