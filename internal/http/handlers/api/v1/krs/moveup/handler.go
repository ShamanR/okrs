// Package moveup serves POST /api/v1/krs/{krID}/move-up.
package moveup

import (
	"net/http"

	"okrs/internal/http/handlers/api/v1/krs/krscommon"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	deps krscommon.MoveDeps
}

func New(deps krscommon.MoveDeps) *Handler { return &Handler{deps: deps} }

// Post moves the key result up within its goal. The body is shared with the opposite
// endpoint: the two URIs differ only by this direction.
func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	krscommon.MoveKeyResult(w, r, h.deps, -1)
}

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Post("/api/v1/krs/{krID}/move-up", h.Post)
}
