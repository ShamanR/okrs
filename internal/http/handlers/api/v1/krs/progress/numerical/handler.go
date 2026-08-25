// Package numerical serves POST /api/v1/krs/{krID}/progress/numerical.
package numerical

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"okrs/internal/auth"
	"okrs/internal/core/domain"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/api/v1/krs/krscommon"
	"okrs/internal/http/handlers/web/common"
	goalsvc "okrs/internal/service/goal"
	keyresultsvc "okrs/internal/service/keyresult"
	kruc "okrs/internal/usecase/keyresult"
)

type Handler struct {
	goals *goalsvc.Service
	krs   *keyresultsvc.Service
	uc    *kruc.UseCase
}

func New(goals *goalsvc.Service, krs *keyresultsvc.Service, uc *kruc.UseCase) *Handler {
	return &Handler{goals: goals, krs: krs, uc: uc}
}

func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	krID, err := common.ParseID(chi.URLParam(r, "krID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid kr id", map[string]string{"kr_id": "invalid"})
		return
	}
	scope, ok := krscommon.TenantScope(w, r)
	if !ok {
		return
	}
	goal, err := krscommon.GoalForKR(r.Context(), h.krs, h.goals, scope, krID)
	if err != nil || !auth.CanAccessTeamFromCtx(r.Context(), goal.TeamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "key result not found", nil)
		return
	}
	var req struct {
		CurrentValue float64 `json:"current_value"`
		HealthStatus *string `json:"health_status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if req.HealthStatus != nil && !domain.IsValidKRHealthStatus(*req.HealthStatus) {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid health_status", map[string]string{"health_status": "invalid"})
		return
	}
	if err := h.uc.UpdateProgressNumerical(r.Context(), scope, krID, req.CurrentValue, auth.UserIDFromContext(r.Context())); err != nil {
		v1.WriteError(w, http.StatusConflict, "CONFLICT", err.Error(), nil)
		return
	}
	if req.HealthStatus != nil {
		if err := h.krs.UpdateHealthStatus(r.Context(), scope, krID, domain.KRHealthStatus(*req.HealthStatus)); err != nil {
			v1.WriteError(w, http.StatusConflict, "CONFLICT", err.Error(), nil)
			return
		}
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
