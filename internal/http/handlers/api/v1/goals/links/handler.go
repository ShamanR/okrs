// Package links serves the /api/v1/goals/… endpoints under its URI segment.
package links

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"okrs/internal/auth"
	"okrs/internal/core/domain"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/api/v1/goals/goalcommon"
	"okrs/internal/http/handlers/web/common"
	goalsvc "okrs/internal/service/goal"
	goaluc "okrs/internal/usecase/goal"
)

type Handler struct {
	goals *goalsvc.Service
	uc    *goaluc.UseCase
}

func New(goals *goalsvc.Service, uc *goaluc.UseCase) *Handler { return &Handler{goals: goals, uc: uc} }

// Post replaces the parent set of a goal (full replace).
// POST /api/v1/goals/{goalID}/links  body {"parent_goal_ids":[...]}
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
	// The child goal's owner team must be reachable by the caller.
	goal, err := h.goals.Get(r.Context(), scope, goalID)
	if err != nil || !auth.CanAccessTeamFromCtx(r.Context(), goal.TeamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	var req struct {
		ParentGoalIDs []int64 `json:"parent_goal_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	allowed, adminAll := goalcommon.AllowedTeams(r)
	if err := h.uc.SetParents(r.Context(), scope, allowed, adminAll, goalID, req.ParentGoalIDs, auth.UserIDFromContext(r.Context())); err != nil {
		switch {
		case errors.Is(err, domain.ErrGoalLinkSelf):
			v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "goal cannot link to itself", map[string]string{"parent": "self"})
		case errors.Is(err, domain.ErrGoalLinkNotAccessible):
			v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "parent goal not accessible", map[string]string{"parent": "not accessible"})
		case errors.Is(err, domain.ErrGoalLinkCycle):
			v1.WriteError(w, http.StatusConflict, "GOAL_LINK_CYCLE", "goal link would create a cycle", nil)
		default:
			v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to set goal links", nil)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
