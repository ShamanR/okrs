// Package noaccess serves GET /no-access — the page an authenticated user without any
// active membership lands on.
//
// Unlike the SPA shells this is not a "URI → template" row: it resolves through the
// nomembership registry, which is the OSS/SaaS extension point. The box ships a stub
// that renders a configurable markdown message; a SaaS build registers its own
// "create organization / join" page under the same name.
package noaccess

import (
	"net/http"

	"okrs/internal/platform/nomembership"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	// name selects the registered handler; comes from Options.NoMembershipName.
	name string
}

func New(name string) *Handler { return &Handler{name: name} }

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	impl, ok := nomembership.Get(h.name)
	if !ok {
		http.Error(w, "no-membership handler not registered", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	impl.ServeNoMembership(w, r)
}

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/no-access", h.Get)
}
