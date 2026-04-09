package teams

import (
	"okrs/internal/http/handlers/api/v1"

	"github.com/go-chi/chi/v5"
)

func RegisterRoutes(r chi.Router, h *v1.Handler) {
	r.Get("/teams/{teamID}", h.HandleTeam())
	r.Get("/teams/{teamID}/okrs", h.HandleTeamOKRs())
	r.Get("/teams/{teamID}/overview", h.HandleTeamOverview())
	r.Post("/teams/{teamID}/status", h.HandleUpdateTeamPeriodStatus())
}
