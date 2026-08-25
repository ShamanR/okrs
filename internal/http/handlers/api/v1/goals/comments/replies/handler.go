// Package replies serves the /api/v1/goals/… endpoints under its URI segment.
package replies

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"okrs/internal/auth"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/api/v1/goals/goalcommon"
	"okrs/internal/http/handlers/web/common"
	goalsvc "okrs/internal/service/goal"
	goalsharesvc "okrs/internal/service/goalshare"
	"okrs/internal/store/goals"
	goaluc "okrs/internal/usecase/goal"
)

type Handler struct {
	goals  *goalsvc.Service
	uc     *goaluc.UseCase
	shares *goalsharesvc.Service
}

func New(goals *goalsvc.Service, uc *goaluc.UseCase, shares *goalsharesvc.Service) *Handler {
	return &Handler{goals: goals, uc: uc, shares: shares}
}

// HandleAddGoalReply creates a reply under a task. Access chain: tenant scope → goal
// reachable by the caller (owner or shared team) → the parent must be a task of this goal
// (enforced in the store; a bad/non-task parent → 404).
func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid goal id", map[string]string{"goal_id": "invalid"})
		return
	}
	commentID, err := common.ParseID(chi.URLParam(r, "commentID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid comment id", map[string]string{"comment_id": "invalid"})
		return
	}
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	goal, err := h.goals.Get(r.Context(), scope, goalID)
	if err != nil || !goalcommon.CanAccess(r.Context(), h.shares, scope, goal) {
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
	if err := h.uc.AddReply(r.Context(), scope, goalID, commentID, req.Text, auth.UserIDFromContext(r.Context())); err != nil {
		if errors.Is(err, goals.ErrNotFound) {
			v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "comment not found", nil)
			return
		}
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to add reply", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
