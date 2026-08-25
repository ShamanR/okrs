// Package stats serves the /api/v1/admin/… endpoints under its URI segment.
package stats

import (
	"net/http"

	"okrs/internal/auth"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/api/v1/admin/admincommon"
	perioduc "okrs/internal/usecase/period"
)

type Handler struct {
	settings admincommon.SettingsReader
	periodUC *perioduc.UseCase
}

func New(periodUC *perioduc.UseCase, settings admincommon.SettingsReader) *Handler {
	return &Handler{periodUC: periodUC, settings: settings}
}

// GET /api/v1/admin/periods/stats
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "no active tenant", nil)
		return
	}
	items, err := h.periodUC.PeriodStats(r.Context(), scope, admincommon.WeightTolerance(r, h.settings, scope))
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to load period stats", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}
