// Package weight serves the /api/v1/goals/… endpoints under its URI segment.
package weight

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"okrs/internal/auth"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/web/common"
	goalsvc "okrs/internal/service/goal"
	goalsharesvc "okrs/internal/service/goalshare"
)

type Handler struct {
	goals  *goalsvc.Service
	shares *goalsharesvc.Service
}

func New(goals *goalsvc.Service, shares *goalsharesvc.Service) *Handler {
	return &Handler{goals: goals, shares: shares}
}

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
	if goal, err := h.goals.Get(r.Context(), scope, goalID); err != nil || !auth.CanAccessTeamFromCtx(r.Context(), goal.TeamID) {
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
	if err := h.shares.UpdateWeight(r.Context(), scope, goalID, req.TeamID, req.Weight); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to update weight", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
