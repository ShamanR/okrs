package v1

import "net/http"

func (h *Handler) HandleHierarchy() http.HandlerFunc {
	return h.handleHierarchy
}

func (h *Handler) HandlePeriods() http.HandlerFunc {
	return h.handlePeriods
}

func (h *Handler) HandleTeam() http.HandlerFunc {
	return h.handleTeam
}

func (h *Handler) HandleTeamOKRs() http.HandlerFunc {
	return h.handleTeamOKRs
}

func (h *Handler) HandleTeamOverview() http.HandlerFunc {
	return h.handleTeamOverview
}

func (h *Handler) HandleUpdateTeamPeriodStatus() http.HandlerFunc {
	return h.handleUpdateTeamPeriodStatus
}

func (h *Handler) HandleGoal() http.HandlerFunc {
	return h.handleGoal
}

func (h *Handler) HandleShareGoal() http.HandlerFunc {
	return h.handleShareGoal
}

func (h *Handler) HandleUpdateGoalWeight() http.HandlerFunc {
	return h.handleUpdateGoalWeight
}

func (h *Handler) HandleAddGoalComment() http.HandlerFunc {
	return h.handleAddGoalComment
}

func (h *Handler) HandleUpdateGoal() http.HandlerFunc {
	return h.handleUpdateGoal
}

func (h *Handler) HandleCreateKeyResult() http.HandlerFunc {
	return h.handleCreateKeyResult
}

func (h *Handler) HandleMoveGoalUp() http.HandlerFunc {
	return h.handleMoveGoalUp
}

func (h *Handler) HandleMoveGoalDown() http.HandlerFunc {
	return h.handleMoveGoalDown
}

func (h *Handler) HandleUpdatePercentProgress() http.HandlerFunc {
	return h.handleUpdatePercentProgress
}

func (h *Handler) HandleUpdateBooleanProgress() http.HandlerFunc {
	return h.handleUpdateBooleanProgress
}

func (h *Handler) HandleUpdateProjectProgress() http.HandlerFunc {
	return h.handleUpdateProjectProgress
}

func (h *Handler) HandleAddKRComment() http.HandlerFunc {
	return h.handleAddKRComment
}

func (h *Handler) HandleUpdateKeyResult() http.HandlerFunc {
	return h.handleUpdateKeyResult
}

func (h *Handler) HandleMoveKeyResultUp() http.HandlerFunc {
	return h.handleMoveKeyResultUp
}

func (h *Handler) HandleMoveKeyResultDown() http.HandlerFunc {
	return h.handleMoveKeyResultDown
}
