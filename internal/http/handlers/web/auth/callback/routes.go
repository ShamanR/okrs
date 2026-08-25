package callback

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/auth/{provider}/callback", h.Get)
}
