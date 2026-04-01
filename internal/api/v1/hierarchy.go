package v1

import (
	"net/http"

	"okrs/internal/http/handlers/common"
)

// handleHierarchy returns the organization hierarchy tree.
func (h *Handler) handleHierarchy(w http.ResponseWriter, r *http.Request) {
	setAPICacheControl(w)
	periodID, err := common.ParsePeriodID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid period id", map[string]string{"period_id": "invalid"})
		return
	}
	var periodRef *int64
	if periodID > 0 {
		periodRef = &periodID
	}
	nodes, err := h.service.GetHierarchy(r.Context(), periodRef)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to load hierarchy", nil)
		return
	}
	writeJSON(w, http.StatusOK, hierarchyResponse{Items: mapHierarchy(nodes)})
}
