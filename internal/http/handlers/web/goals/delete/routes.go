package delete

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Post("/goals/{goalID}/delete", h.Post)
}
