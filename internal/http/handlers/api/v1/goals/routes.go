package goals

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/api/v1/goals/{goalID}", h.HandleGoal)
	r.Post("/api/v1/goals/{goalID}/share", h.HandleShareGoal)
	r.Post("/api/v1/goals/{goalID}/weight", h.HandleUpdateGoalWeight)
	r.Post("/api/v1/goals/{goalID}/comments", h.HandleAddGoalComment)
	r.Post("/api/v1/goals/{goalID}/comments/{commentID}/resolve", h.HandleResolveGoalComment)
	r.Post("/api/v1/goals/{goalID}/comments/{commentID}/unresolve", h.HandleUnresolveGoalComment)
	r.Post("/api/v1/goals/{goalID}", h.HandleUpdateGoal)
	r.Post("/api/v1/goals/{goalID}/move-up", h.HandleMoveGoalUp)
	r.Post("/api/v1/goals/{goalID}/move-down", h.HandleMoveGoalDown)
	r.Delete("/api/v1/goals/{goalID}/share/{teamID}", h.HandleLeaveGoalShare)
	r.Delete("/api/v1/goals/{goalID}", h.HandleDeleteGoal)
}
