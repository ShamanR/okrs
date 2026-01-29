package v1

import (
	"net/http"
)

// handleHierarchy returns the organization hierarchy tree.
func (h *Handler) handleHierarchy(w http.ResponseWriter, r *http.Request) {
	nodes, err := h.service.GetHierarchy(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to load hierarchy", nil)
		return
	}
	writeJSON(w, http.StatusOK, hierarchyResponse{Items: mapHierarchy(nodes)})
}
