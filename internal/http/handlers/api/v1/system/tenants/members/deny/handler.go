// Package deny serves the /api/v1/system/… endpoints under its URI segment.
package deny

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"okrs/internal/http/handlers/api/v1/system/systemcommon"
)

type Handler struct {
	prov systemcommon.Provisioner
}

func New(prov systemcommon.Provisioner) *Handler { return &Handler{prov: prov} }

// POST /api/v1/system/tenants/{id}/members/{userID}/deny
func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	id, ok := systemcommon.PathID(w, r)
	if !ok {
		return
	}
	userID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	if err != nil || userID <= 0 {
		systemcommon.WriteError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if err := h.prov.DenyMember(r.Context(), id, userID); err != nil {
		systemcommon.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
