// Package periods serves GET /api/v1/periods.
package periods

import (
	"net/http"

	"okrs/internal/auth"
	v1 "okrs/internal/http/handlers/api/v1"
	periodsvc "okrs/internal/service/period"
)

type Handler struct {
	periods *periodsvc.Service
}

func New(periods *periodsvc.Service) *Handler { return &Handler{periods: periods} }

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	v1.SetAPICacheControl(w)
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	views, err := h.periods.ListViews(r.Context(), scope, false)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to load periods", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, newPeriodsResponse(views))
}
