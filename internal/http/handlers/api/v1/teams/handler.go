package teams

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

func (h *Handler) HandleTeam(w http.ResponseWriter, r *http.Request) {
	h.core.HandleTeam()(w, r)
}

func (h *Handler) HandleTeamOKRs(w http.ResponseWriter, r *http.Request) {
	h.core.HandleTeamOKRs()(w, r)
}

func (h *Handler) HandleTeamOverview(w http.ResponseWriter, r *http.Request) {
	h.core.HandleTeamOverview()(w, r)
}

func (h *Handler) HandleUpdateTeamPeriodStatus(w http.ResponseWriter, r *http.Request) {
	h.core.HandleUpdateTeamPeriodStatus()(w, r)
}
