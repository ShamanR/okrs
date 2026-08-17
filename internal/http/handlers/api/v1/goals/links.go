package goals

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"okrs/internal/auth"
	"okrs/internal/http/dto"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/web/common"
	"okrs/internal/service"

	"github.com/go-chi/chi/v5"
)

// allowedTeams returns the caller's allowed team IDs and whether they are unrestricted
// (admin: nil allowed set). Mirrors how board/list endpoints read scope from context.
func allowedTeams(r *http.Request) (allowed []int64, adminAll bool) {
	allowed, _ = auth.AllowedTeamIDsFromCtx(r.Context())
	return allowed, allowed == nil
}

// HandleLinkableGoals lists candidate goals for the parent picker.
// GET /api/v1/goals/linkable?period_id=&q=&exclude_goal_id=
func (h *Handler) HandleLinkableGoals(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	var periodID *int64
	if raw := strings.TrimSpace(r.URL.Query().Get("period_id")); raw != "" && raw != "all" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid period_id", map[string]string{"period_id": "invalid"})
			return
		}
		periodID = &id
	}
	var excludeGoalID int64
	if raw := strings.TrimSpace(r.URL.Query().Get("exclude_goal_id")); raw != "" {
		excludeGoalID, _ = strconv.ParseInt(raw, 10, 64)
	}
	q := r.URL.Query().Get("q")
	allowed, adminAll := allowedTeams(r)
	items, err := h.service.ListLinkableGoals(r.Context(), scope, allowed, adminAll, periodID, excludeGoalID, q)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to list linkable goals", nil)
		return
	}
	out := make([]dto.LinkableGoal, 0, len(items))
	for _, it := range items {
		out = append(out, dto.LinkableGoal{
			GoalRef: dto.GoalRef{
				ID: it.ID, Title: it.Title, PeriodID: it.PeriodID, PeriodName: it.PeriodName,
				TeamID: it.TeamID, TeamName: it.TeamName, TeamType: it.TeamType, Progress: it.Progress,
			},
			Lead: it.Lead,
		})
	}
	v1.WriteJSON(w, http.StatusOK, out)
}

// HandleSetGoalParents replaces the parent set of a goal (full replace).
// POST /api/v1/goals/{goalID}/links  body {"parent_goal_ids":[...]}
func (h *Handler) HandleSetGoalParents(w http.ResponseWriter, r *http.Request) {
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
	goal, err := h.service.GetGoal(r.Context(), scope, goalID)
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
	allowed, adminAll := allowedTeams(r)
	if err := h.service.SetGoalParents(r.Context(), scope, allowed, adminAll, goalID, req.ParentGoalIDs, auth.UserIDFromContext(r.Context())); err != nil {
		switch {
		case errors.Is(err, service.ErrGoalLinkSelf):
			v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "goal cannot link to itself", map[string]string{"parent": "self"})
		case errors.Is(err, service.ErrGoalLinkNotAccessible):
			v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "parent goal not accessible", map[string]string{"parent": "not accessible"})
		case errors.Is(err, service.ErrGoalLinkCycle):
			v1.WriteError(w, http.StatusConflict, "GOAL_LINK_CYCLE", "goal link would create a cycle", nil)
		default:
			v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to set goal links", nil)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
