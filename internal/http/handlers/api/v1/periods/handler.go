package periods

import (
	"net/http"

	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/service"
)

type Handler struct {
	core *v1.Handler
}

func New(service *service.Service) *Handler {
	return &Handler{core: v1.NewHandler(service)}
}

func (h *Handler) HandlePeriods(w http.ResponseWriter, r *http.Request) {
	h.core.HandlePeriods()(w, r)
}
