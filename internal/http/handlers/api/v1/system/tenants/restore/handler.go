// Package restore serves the /api/v1/system/… endpoints under its URI segment.
package restore

import (
	"net/http"

	"okrs/internal/http/handlers/api/v1/system/systemcommon"
)

type Handler struct {
	prov systemcommon.Provisioner
}

func New(prov systemcommon.Provisioner) *Handler { return &Handler{prov: prov} }

// POST /api/v1/system/tenants/{id}/restore
func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	systemcommon.Transition(w, r, h.prov, h.prov.Restore)
}
