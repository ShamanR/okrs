package openlink

import (
	"github.com/go-chi/chi/v5"

	"okrs/internal/render/notify"
)

// Path is the route this handler serves. It is the one notification links are
// built with — see notify.OpenPath, which builds them.
const Path = notify.OpenPath

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get(Path, h.Get)
}
