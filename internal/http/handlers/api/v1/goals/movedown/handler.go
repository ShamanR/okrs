// Package movedown serves POST /api/v1/goals/{goalID}/move-down.
package movedown

import (
	"net/http"

	"okrs/internal/http/handlers/api/v1/goals/goalcommon"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	deps goalcommon.MoveDeps
}

func New(deps goalcommon.MoveDeps) *Handler { return &Handler{deps: deps} }

// Post moves the goal down within the viewing team's board. The body is shared with
// the opposite endpoint: the two URIs differ only by this direction.
func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	goalcommon.MoveGoal(w, r, h.deps, 1)
}

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Post("/api/v1/goals/{goalID}/move-down", h.Post)
}
