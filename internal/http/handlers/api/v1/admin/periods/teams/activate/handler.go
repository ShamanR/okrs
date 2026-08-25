// Package activate serves the bulk period-status endpoint under its URI segment.
package activate

import (
	"net/http"

	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/api/v1/admin/periods/teams/bulkstatus"
	perioduc "okrs/internal/usecase/period"
)

type Handler struct {
	leads    bulkstatus.TeamScopeResolver
	periodUC *perioduc.UseCase
	teams    bulkstatus.TeamLister
}

func New(periodUC *perioduc.UseCase, teams bulkstatus.TeamLister, leads bulkstatus.TeamScopeResolver) *Handler {
	return &Handler{periodUC: periodUC, teams: teams, leads: leads}
}

// POST /api/v1/admin/periods/{periodID}/teams/activate (legacy admin-only, whole tenant)
func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	bulkstatus.Run(w, r, h.periodUC, h.teams, h.leads, domain.TeamPeriodStatusInProgress)
}
