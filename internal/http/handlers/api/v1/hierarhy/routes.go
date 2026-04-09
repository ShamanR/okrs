package hierarhy

import (
	"okrs/internal/http/handlers/api/v1"

	"github.com/go-chi/chi/v5"
)

func RegisterRoutes(r chi.Router, h *v1.Handler) {
	r.Get("/hierarchy", h.HandleHierarchy())
}
