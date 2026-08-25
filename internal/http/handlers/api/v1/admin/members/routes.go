package members

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Delete("/api/v1/admin/members/{userID}", h.Delete)
}
