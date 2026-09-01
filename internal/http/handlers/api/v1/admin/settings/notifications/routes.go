package notifications

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/api/v1/admin/settings/notifications", h.List)
	r.Put("/api/v1/admin/settings/notifications/{channel}", h.Save)
}
