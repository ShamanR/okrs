// Package teams serves /api/v1/teams/{teamID}.
package teams

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"okrs/internal/auth"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/web/common"
	teamsvc "okrs/internal/service/team"
)

type Handler struct {
	teams *teamsvc.Service
}

func New(teams *teamsvc.Service) *Handler { return &Handler{teams: teams} }

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team id", map[string]string{"team_id": "invalid"})
		return
	}
	if !auth.CanAccessTeamFromCtx(r.Context(), teamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "team not found", nil)
		return
	}
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	team, err := h.teams.Get(r.Context(), scope, teamID)
	if err != nil {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "team not found", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]any{
		"id":         team.ID,
		"name":       team.Name,
		"type":       string(team.Type),
		"type_label": common.TeamTypeLabel(team.Type),
		"parent_id":  team.ParentID,
	})
}
