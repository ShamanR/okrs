package teams

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/teams/{teamID}", h.HandleTeam)
	r.Get("/teams/{teamID}/okrs", h.HandleTeamOKRs)
	r.Get("/teams/{teamID}/overview", h.HandleTeamOverview)
	r.Post("/teams/{teamID}/status", h.HandleUpdateTeamPeriodStatus)
}
