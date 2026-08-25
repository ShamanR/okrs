// Package overview serves the bulk period-status endpoint under its URI segment.
package overview

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"okrs/internal/auth"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/api/v1/admin/admincommon"
	"okrs/internal/http/handlers/api/v1/admin/periods/teams/bulkstatus"
	"okrs/internal/http/handlers/web/common"
	perioduc "okrs/internal/usecase/period"
)

type Handler struct {
	periodUC *perioduc.UseCase
	settings admincommon.SettingsReader
	teams    bulkstatus.TeamLister
	leads    bulkstatus.TeamScopeResolver
}

func New(periodUC *perioduc.UseCase, settings admincommon.SettingsReader, teams bulkstatus.TeamLister, leads bulkstatus.TeamScopeResolver) *Handler {
	return &Handler{periodUC: periodUC, settings: settings, teams: teams, leads: leads}
}

// GET /api/v1/periods/{periodID}/overview?scope=my_teams|org
// Available to any authenticated member. scope=my_teams (default) restricts the
// overview to teams the caller leads plus nested descendants; scope=org (whole
// tenant) requires tenant-admin.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "no active tenant", nil)
		return
	}
	periodID, err := common.ParseID(chi.URLParam(r, "periodID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid period id", nil)
		return
	}
	teamFilter, ok := bulkstatus.ResolveOverviewScope(w, r, h.teams, h.leads, scope)
	if !ok {
		return
	}
	ov, err := h.periodUC.PeriodOverviewScoped(r.Context(), scope, periodID, admincommon.WeightTolerance(r, h.settings, scope), teamFilter)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to load overview", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, ov)
}
