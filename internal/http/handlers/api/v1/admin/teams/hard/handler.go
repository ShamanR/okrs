// Package hard serves the /api/v1/admin/… endpoints under its URI segment.
package hard

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

// DELETE /api/v1/admin/teams/{teamID}/hard
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team id", nil)
		return
	}
	if err := h.teams.HardDelete(r.Context(), scope, teamID); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
