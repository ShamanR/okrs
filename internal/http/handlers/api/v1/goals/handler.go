package goals

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

func (h *Handler) HandleGoal(w http.ResponseWriter, r *http.Request) {
	h.core.HandleGoal()(w, r)
}

func (h *Handler) HandleShareGoal(w http.ResponseWriter, r *http.Request) {
	h.core.HandleShareGoal()(w, r)
}

func (h *Handler) HandleUpdateGoalWeight(w http.ResponseWriter, r *http.Request) {
	h.core.HandleUpdateGoalWeight()(w, r)
}

func (h *Handler) HandleAddGoalComment(w http.ResponseWriter, r *http.Request) {
	h.core.HandleAddGoalComment()(w, r)
}

func (h *Handler) HandleUpdateGoal(w http.ResponseWriter, r *http.Request) {
	h.core.HandleUpdateGoal()(w, r)
}

func (h *Handler) HandleCreateKeyResult(w http.ResponseWriter, r *http.Request) {
	h.core.HandleCreateKeyResult()(w, r)
}

func (h *Handler) HandleMoveGoalUp(w http.ResponseWriter, r *http.Request) {
	h.core.HandleMoveGoalUp()(w, r)
}

func (h *Handler) HandleMoveGoalDown(w http.ResponseWriter, r *http.Request) {
	h.core.HandleMoveGoalDown()(w, r)
}
