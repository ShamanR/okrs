package goals

import (
	"encoding/json"
	"net/http"

	"okrs/internal/auth"
	"okrs/internal/domain"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/dto"
	"okrs/internal/http/handlers/web/common"
	"okrs/internal/service"
	"okrs/internal/store/goals"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service *service.Service
}

func New(service *service.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) HandleGoal(w http.ResponseWriter, r *http.Request) {
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid goal id", map[string]string{"goal_id": "invalid"})
		return
	}
	goal, err := h.service.GetGoal(r.Context(), goalID)
	if err != nil {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	if !auth.CanAccessTeamFromCtx(r.Context(), goal.TeamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	var userRefs map[string]*dto.UserRef
	if goal.OwnerText != "" {
		users, _ := h.service.GetUsersByDisplayNames(r.Context(), []string{goal.OwnerText})
		userRefs = v1.BuildUserRefMap(users)
	}
	v1.WriteJSON(w, http.StatusOK, newGoalResponse(goal, userRefs))
}

func (h *Handler) HandleShareGoal(w http.ResponseWriter, r *http.Request) {
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid goal id", map[string]string{"goal_id": "invalid"})
		return
	}
	if goal, err := h.service.GetGoal(r.Context(), goalID); err != nil || !auth.CanAccessTeamFromCtx(r.Context(), goal.TeamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	var req struct {
		Targets []struct {
			TeamID int64 `json:"team_id"`
			Weight int   `json:"weight"`
		} `json:"targets"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if len(req.Targets) == 0 {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "targets required", map[string]string{"targets": "required"})
		return
	}
	targets := make([]service.ShareTarget, 0, len(req.Targets))
	for _, target := range req.Targets {
		if target.TeamID == 0 {
			v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team_id", map[string]string{"team_id": "required"})
			return
		}
		if target.Weight < 0 || target.Weight > 100 {
			v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid weight", map[string]string{"weight": "0..100"})
			return
		}
		targets = append(targets, service.ShareTarget{TeamID: target.TeamID, Weight: target.Weight})
	}
	if err := h.service.ShareGoal(r.Context(), goalID, targets); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to share goal", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) HandleUpdateGoalWeight(w http.ResponseWriter, r *http.Request) {
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid goal id", map[string]string{"goal_id": "invalid"})
		return
	}
	if goal, err := h.service.GetGoal(r.Context(), goalID); err != nil || !auth.CanAccessTeamFromCtx(r.Context(), goal.TeamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	var req struct {
		TeamID int64 `json:"team_id"`
		Weight int   `json:"weight"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if req.TeamID == 0 {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "team_id required", map[string]string{"team_id": "required"})
		return
	}
	if req.Weight < 0 || req.Weight > 100 {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid weight", map[string]string{"weight": "0..100"})
		return
	}
	if err := h.service.UpdateGoalWeight(r.Context(), goalID, req.TeamID, req.Weight); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to update weight", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) HandleAddGoalComment(w http.ResponseWriter, r *http.Request) {
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid goal id", map[string]string{"goal_id": "invalid"})
		return
	}
	goal, err := h.service.GetGoal(r.Context(), goalID)
	if err != nil || !auth.CanAccessTeamFromCtx(r.Context(), goal.TeamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if req.Text == "" {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "text required", map[string]string{"text": "required"})
		return
	}
	if err := h.service.AddGoalComment(r.Context(), goalID, req.Text, auth.UserIDFromContext(r.Context())); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to add comment", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) HandleUpdateGoal(w http.ResponseWriter, r *http.Request) {
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid goal id", map[string]string{"goal_id": "invalid"})
		return
	}
	if goal, err := h.service.GetGoal(r.Context(), goalID); err != nil || !auth.CanAccessTeamFromCtx(r.Context(), goal.TeamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	priority := domain.Priority(r.FormValue("priority"))
	workType := domain.WorkType(r.FormValue("work_type"))
	focusType := domain.FocusType(r.FormValue("focus_type"))
	teamID, err := parseOptionalID(r.FormValue("team_id"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team_id", map[string]string{"team_id": "invalid"})
		return
	}
	weight := common.ParseIntField(r.FormValue("weight"))
	if validationErr := common.ValidateGoalInput(priority, workType, focusType, weight); validationErr != "" {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", validationErr, nil)
		return
	}
	goalWeight := weight
	if teamID != nil {
		goal, err := h.service.GetGoal(r.Context(), goalID)
		if err != nil {
			v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
			return
		}
		goalWeight = goal.Weight
	}
	if err := h.service.UpdateGoal(r.Context(), goals.GoalUpdateInput{
		ID:          goalID,
		Title:       common.TrimmedFormValue(r, "title"),
		Description: common.TrimmedFormValue(r, "description"),
		Priority:    priority,
		Weight:      goalWeight,
		WorkType:    workType,
		FocusType:   focusType,
		OwnerText:   common.TrimmedFormValue(r, "owner_text"),
	}); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to update goal", nil)
		return
	}
	if teamID != nil {
		if err := h.service.UpdateGoalWeight(r.Context(), goalID, *teamID, weight); err != nil {
			v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to update weight", nil)
			return
		}
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) HandleMoveGoalUp(w http.ResponseWriter, r *http.Request) {
	h.handleMoveGoal(w, r, -1)
}

func (h *Handler) HandleMoveGoalDown(w http.ResponseWriter, r *http.Request) {
	h.handleMoveGoal(w, r, 1)
}

func (h *Handler) handleMoveGoal(w http.ResponseWriter, r *http.Request, direction int) {
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid goal id", map[string]string{"goal_id": "invalid"})
		return
	}
	if goal, err := h.service.GetGoal(r.Context(), goalID); err != nil || !auth.CanAccessTeamFromCtx(r.Context(), goal.TeamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	if err := h.service.MoveGoal(r.Context(), goalID, direction); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to move goal", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) HandleLeaveGoalShare(w http.ResponseWriter, r *http.Request) {
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid goal id", map[string]string{"goal_id": "invalid"})
		return
	}
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team id", map[string]string{"team_id": "invalid"})
		return
	}
	if !auth.CanAccessTeamFromCtx(r.Context(), teamID) {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "access denied", nil)
		return
	}
	if _, _, err := h.service.DeleteGoal(r.Context(), goalID, teamID); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) HandleDeleteGoal(w http.ResponseWriter, r *http.Request) {
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid goal id", map[string]string{"goal_id": "invalid"})
		return
	}
	goal, err := h.service.GetGoal(r.Context(), goalID)
	if err != nil || !auth.CanAccessTeamFromCtx(r.Context(), goal.TeamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	if _, _, err := h.service.DeleteGoal(r.Context(), goalID, goal.TeamID); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
