package periods

import (
	"net/http"

	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/service"
)

type Handler struct {
	service *service.Service
}

func New(service *service.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) HandlePeriods(w http.ResponseWriter, r *http.Request) {
	v1.SetAPICacheControl(w)
	periods, err := h.service.ListPeriods(r.Context())
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to load periods", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, v1.NewPeriodsResponse(periods))
}
