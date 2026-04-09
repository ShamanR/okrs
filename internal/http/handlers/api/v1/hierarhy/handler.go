package hierarhy

import (
	"net/http"

	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/web/common"
	"okrs/internal/service"
)

type Handler struct {
	service *service.Service
}

func New(service *service.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) HandleHierarchy(w http.ResponseWriter, r *http.Request) {
	v1.SetAPICacheControl(w)
	periodID, err := common.ParsePeriodID(r)
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid period id", map[string]string{"period_id": "invalid"})
		return
	}
	var periodRef *int64
	if periodID > 0 {
		periodRef = &periodID
	}
	nodes, err := h.service.GetHierarchy(r.Context(), periodRef)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to load hierarchy", nil)
		return
	}
	metrics := map[int64]service.TeamSummary{}
	if periodRef != nil {
		summaries, err := h.service.GetTeamsWithPeriodSummary(r.Context(), periodID, nil)
		if err != nil {
			v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to load hierarchy summary", nil)
			return
		}
		metrics = make(map[int64]service.TeamSummary, len(summaries))
		for _, summary := range summaries {
			metrics[summary.ID] = summary
		}
	}
	v1.WriteJSON(w, http.StatusOK, v1.NewHierarchyResponse(nodes, metrics))
}
