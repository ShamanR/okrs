// Package status serves POST /api/v1/teams/{teamID}/status.
package status

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"okrs/internal/auth"
	"okrs/internal/core/domain"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/web/common"
	perioduc "okrs/internal/usecase/period"
)

type Handler struct {
	periodUC *perioduc.UseCase
}

func New(periodUC *perioduc.UseCase) *Handler { return &Handler{periodUC: periodUC} }

func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "no active tenant", nil)
		return
	}
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team id", map[string]string{"team_id": "invalid"})
		return
	}
	if !auth.CanAccessTeamFromCtx(r.Context(), teamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "team not found", nil)
		return
	}
	var req struct {
		PeriodID int64  `json:"period_id"`
		Status   string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if req.PeriodID == 0 {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "period_id required", map[string]string{"period_id": "required"})
		return
	}
	status := domain.TeamPeriodStatus(req.Status)
	if !common.ValidTeamPeriodStatus(status) {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid status", map[string]string{"status": "invalid"})
		return
	}
	if err := h.periodUC.UpdateTeamStatus(r.Context(), scope, teamID, req.PeriodID, status, auth.UserIDFromContext(r.Context())); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to update status", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
