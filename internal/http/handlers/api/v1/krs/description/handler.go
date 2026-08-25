// Package description serves POST /api/v1/krs/{krID}/description.
package description

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"okrs/internal/auth"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/api/v1/krs/krscommon"
	"okrs/internal/http/handlers/web/common"
	goalsvc "okrs/internal/service/goal"
	keyresultsvc "okrs/internal/service/keyresult"
)

type Handler struct {
	goals *goalsvc.Service
	krs   *keyresultsvc.Service
}

func New(goals *goalsvc.Service, krs *keyresultsvc.Service) *Handler {
	return &Handler{goals: goals, krs: krs}
}

// HandleUpdateKRDescription updates only the description of a KR. Allowed in the
// same situations as notes (access check only), so a description can be added
// from the progress-update modal when full editing is otherwise locked.
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
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	req.Description = strings.ReplaceAll(req.Description, "\r\n", "\n")
	if err := h.krs.UpdateDescription(r.Context(), scope, krID, req.Description); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to update description", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
