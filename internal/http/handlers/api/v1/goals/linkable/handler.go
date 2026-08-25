// Package linkable serves the /api/v1/goals/… endpoints under its URI segment.
package linkable

import (
	"net/http"
	"strconv"
	"strings"

	"okrs/internal/auth"
	"okrs/internal/http/dto"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/api/v1/goals/goalcommon"
	goallinksvc "okrs/internal/service/goallink"
)

type Handler struct {
	links *goallinksvc.Service
}

func New(links *goallinksvc.Service) *Handler { return &Handler{links: links} }

// HandleLinkableGoals lists candidate goals for the parent picker.
// GET /api/v1/goals/linkable?period_id=&q=&exclude_goal_id=
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
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
	allowed, adminAll := goalcommon.AllowedTeams(r)
	items, err := h.links.ListLinkable(r.Context(), scope, allowed, adminAll, periodID, excludeGoalID, q)
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
