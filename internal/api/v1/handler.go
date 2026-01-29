package v1

import (
	"net/http"

	"okrs/internal/service"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service *service.Service
}

const maxMultipartMemory = 32 << 20

func NewHandler(service *service.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/hierarchy", h.handleHierarchy)
	r.Get("/teams", h.handleTeams)
	r.Get("/teams/{teamID}", h.handleTeam)
	r.Get("/teams/{teamID}/okrs", h.handleTeamOKRs)
	r.Get("/goals/{goalID}", h.handleGoal)

	r.Post("/goals/{goalID}/share", h.handleShareGoal)
	r.Post("/goals/{goalID}/weight", h.handleUpdateGoalWeight)
	r.Post("/goals/{goalID}/comments", h.handleAddGoalComment)
	r.Post("/goals/{goalID}", h.handleUpdateGoal)
	r.Post("/goals/{goalID}/key-results", h.handleCreateKeyResult)
	r.Post("/goals/{goalID}/move-up", h.handleMoveGoalUp)
	r.Post("/goals/{goalID}/move-down", h.handleMoveGoalDown)

	r.Post("/krs/{krID}/progress/percent", h.handleUpdatePercentProgress)
	r.Post("/krs/{krID}/progress/boolean", h.handleUpdateBooleanProgress)
	r.Post("/krs/{krID}/progress/project", h.handleUpdateProjectProgress)
	r.Post("/krs/{krID}/comments", h.handleAddKRComment)
	r.Post("/krs/{krID}", h.handleUpdateKeyResult)
	r.Post("/krs/{krID}/move-up", h.handleMoveKeyResultUp)
	r.Post("/krs/{krID}/move-down", h.handleMoveKeyResultDown)

	r.Post("/teams/{teamID}/status", h.handleUpdateTeamQuarterStatus)

	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	})

	return r
}
