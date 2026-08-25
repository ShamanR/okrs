// Package resolve serves POST /api/v1/goals/{goalID}/comments/{commentID}/resolve.
package resolve

import (
	"net/http"

	"okrs/internal/http/handlers/api/v1/goals/goalcommon"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	deps goalcommon.ResolveDeps
}

func New(deps goalcommon.ResolveDeps) *Handler { return &Handler{deps: deps} }

// Post resolves the comment. The body is shared with the opposite endpoint: the two
// URIs differ only by this boolean.
func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	goalcommon.SetCommentResolved(w, r, h.deps, true)
}

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Post("/api/v1/goals/{goalID}/comments/{commentID}/resolve", h.Post)
}
