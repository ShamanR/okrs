package deny

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Post("/api/v1/system/tenants/{id}/members/{userID}/deny", h.Post)
}
