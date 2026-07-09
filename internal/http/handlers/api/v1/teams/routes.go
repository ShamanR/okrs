package teams

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/api/v1/teams/{teamID}", h.HandleTeam)
	r.Get("/api/v1/teams/{teamID}/okrs", h.HandleTeamOKRs)
	r.Get("/api/v1/teams/{teamID}/overview", h.HandleTeamOverview)
	r.Post("/api/v1/teams/{teamID}/status", h.HandleUpdateTeamPeriodStatus)
	r.Post("/api/v1/teams/{teamID}/goals", h.HandleCreateGoal)
}
