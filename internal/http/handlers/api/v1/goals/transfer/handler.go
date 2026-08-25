// Package transfer serves the /api/v1/goals/… endpoints under its URI segment.
package transfer

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"okrs/internal/auth"
	"okrs/internal/core/domain"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/web/common"
	goalsvc "okrs/internal/service/goal"
	goaluc "okrs/internal/usecase/goal"
)

type Handler struct {
	goals *goalsvc.Service
	uc    *goaluc.UseCase
}

func New(goals *goalsvc.Service, uc *goaluc.UseCase) *Handler { return &Handler{goals: goals, uc: uc} }

func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid goal id", map[string]string{"goal_id": "invalid"})
		return
	}
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	var req struct {
		Mode           string `json:"mode"`
		TargetTeamID   int64  `json:"target_team_id"`
		TargetPeriodID int64  `json:"target_period_id"`
		WithComments   bool   `json:"with_comments"`
		WithProgress   bool   `json:"with_progress"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	var mode goaluc.CopyGoalMode
	switch req.Mode {
	case "copy":
		mode = goaluc.CopyGoalModeCopy
	case "move":
		mode = goaluc.CopyGoalModeMove
	default:
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid mode", map[string]string{"mode": "copy|move"})
		return
	}
	if req.TargetTeamID <= 0 {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid target_team_id", map[string]string{"target_team_id": "required"})
		return
	}
	if req.TargetPeriodID <= 0 {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid target_period_id", map[string]string{"target_period_id": "required"})
		return
	}
	// Source access: owner team must be reachable.
	goal, err := h.goals.Get(r.Context(), scope, goalID)
	if err != nil || !auth.CanAccessTeamFromCtx(r.Context(), goal.TeamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	// Target team access.
	if !auth.CanAccessTeamFromCtx(r.Context(), req.TargetTeamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "team not found", nil)
		return
	}
	newID, err := h.uc.Copy(r.Context(), scope, goaluc.CopyGoalParams{
		SourceGoalID:   goalID,
		TargetTeamID:   req.TargetTeamID,
		TargetPeriodID: req.TargetPeriodID,
		Mode:           mode,
		WithProgress:   req.WithProgress,
		WithComments:   req.WithComments,
	}, auth.UserIDFromContext(r.Context()))
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrPeriodClosed):
			v1.WriteError(w, http.StatusConflict, "CONFLICT", "target team period is in progress or closed", nil)
		case errors.Is(err, domain.ErrTransferTargetNotFound):
			v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "target team or period not found", nil)
		case errors.Is(err, domain.ErrTransferTargetSameAsSource):
			v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "target equals source", map[string]string{"target": "same_as_source"})
		default:
			v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to transfer goal", nil)
		}
		return
	}
	v1.WriteJSON(w, http.StatusCreated, map[string]int64{"id": newID})
}
