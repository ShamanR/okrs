package goals

import (
	"okrs/internal/http/handlers/api/v1"

	"github.com/go-chi/chi/v5"
)

func RegisterRoutes(r chi.Router, h *v1.Handler) {
	r.Get("/goals/{goalID}", h.HandleGoal())
	r.Post("/goals/{goalID}/share", h.HandleShareGoal())
	r.Post("/goals/{goalID}/weight", h.HandleUpdateGoalWeight())
	r.Post("/goals/{goalID}/comments", h.HandleAddGoalComment())
	r.Post("/goals/{goalID}", h.HandleUpdateGoal())
	r.Post("/goals/{goalID}/key-results", h.HandleCreateKeyResult())
	r.Post("/goals/{goalID}/move-up", h.HandleMoveGoalUp())
	r.Post("/goals/{goalID}/move-down", h.HandleMoveGoalDown())
}
