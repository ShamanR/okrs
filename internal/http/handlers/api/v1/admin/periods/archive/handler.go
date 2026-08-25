// Package archive serves the /api/v1/admin/… endpoints under its URI segment.
package archive

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"okrs/internal/auth"
	"okrs/internal/core/domain"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/web/common"
	periodsvc "okrs/internal/service/period"
)

type Handler struct {
	periods *periodsvc.Service
}

func New(periods *periodsvc.Service) *Handler { return &Handler{periods: periods} }

// POST /api/v1/admin/periods/{periodID}/archive
func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	periodID, err := common.ParseID(chi.URLParam(r, "periodID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid period id", nil)
		return
	}
	if err := h.periods.Archive(r.Context(), scope, periodID); err != nil {
		if errors.Is(err, domain.ErrPeriodNotClosed) {
			v1.WriteError(w, http.StatusConflict, "CONFLICT", "only a closed period can be archived", nil)
			return
		}
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to archive period", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
