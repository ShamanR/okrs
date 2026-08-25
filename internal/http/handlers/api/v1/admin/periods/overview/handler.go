// Package overview serves the /api/v1/admin/… endpoints under its URI segment.
package overview

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"okrs/internal/auth"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/api/v1/admin/admincommon"
	"okrs/internal/http/handlers/web/common"
	perioduc "okrs/internal/usecase/period"
)

type Handler struct {
	settings admincommon.SettingsReader
	periodUC *perioduc.UseCase
}

func New(periodUC *perioduc.UseCase, settings admincommon.SettingsReader) *Handler {
	return &Handler{periodUC: periodUC, settings: settings}
}

// GET /api/v1/admin/periods/{periodID}/overview
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
	ov, err := h.periodUC.PeriodOverview(r.Context(), scope, periodID, admincommon.WeightTolerance(r, h.settings, scope))
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to load overview", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, ov)
}
