package systemadmin

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Put("/api/v1/system/users/{userID}/system-admin", h.Put)
}
