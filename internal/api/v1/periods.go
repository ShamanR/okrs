package v1

import (
	"net/http"
)

func (h *Handler) handlePeriods(w http.ResponseWriter, r *http.Request) {
	periods, err := h.service.ListPeriods(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to load periods", nil)
		return
	}
	items := make([]periodInfo, 0, len(periods))
	for _, period := range periods {
		items = append(items, mapPeriodInfo(period))
	}
	writeJSON(w, http.StatusOK, periodsResponse{Items: items})
}
