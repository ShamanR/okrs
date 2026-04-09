package v1

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func RegisterHierarchyRoutes(r chi.Router, h *Handler) {
	r.Get("/hierarchy", h.handleHierarchy)
}

func RegisterPeriodsRoutes(r chi.Router, h *Handler) {
	r.Get("/periods", h.handlePeriods)
}

func RegisterTeamsRoutes(r chi.Router, h *Handler) {
	r.Get("/teams/{teamID}", h.handleTeam)
	r.Get("/teams/{teamID}/okrs", h.handleTeamOKRs)
	r.Get("/teams/{teamID}/overview", h.handleTeamOverview)
	r.Post("/teams/{teamID}/status", h.handleUpdateTeamPeriodStatus)
}

func RegisterGoalsRoutes(r chi.Router, h *Handler) {
	r.Get("/goals/{goalID}", h.handleGoal)
	r.Post("/goals/{goalID}/share", h.handleShareGoal)
	r.Post("/goals/{goalID}/weight", h.handleUpdateGoalWeight)
	r.Post("/goals/{goalID}/comments", h.handleAddGoalComment)
	r.Post("/goals/{goalID}", h.handleUpdateGoal)
	r.Post("/goals/{goalID}/key-results", h.handleCreateKeyResult)
	r.Post("/goals/{goalID}/move-up", h.handleMoveGoalUp)
	r.Post("/goals/{goalID}/move-down", h.handleMoveGoalDown)
}

func RegisterKeyResultsRoutes(r chi.Router, h *Handler) {
	r.Post("/krs/{krID}/progress/percent", h.handleUpdatePercentProgress)
	r.Post("/krs/{krID}/progress/boolean", h.handleUpdateBooleanProgress)
	r.Post("/krs/{krID}/progress/project", h.handleUpdateProjectProgress)
	r.Post("/krs/{krID}/comments", h.handleAddKRComment)
	r.Post("/krs/{krID}", h.handleUpdateKeyResult)
	r.Post("/krs/{krID}/move-up", h.handleMoveKeyResultUp)
	r.Post("/krs/{krID}/move-down", h.handleMoveKeyResultDown)
}

func RegisterMethodNotAllowed(r chi.Router) {
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	})
}
