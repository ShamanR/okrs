// Package note serves POST /api/v1/krs/{krID}/note.
package note

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
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	req.Text = strings.ReplaceAll(req.Text, "\r\n", "\n")
	if strings.TrimSpace(req.Text) == "" {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "text required", map[string]string{"text": "required"})
		return
	}
	if err := h.uc.UpsertNote(r.Context(), scope, krID, req.Text, auth.UserIDFromContext(r.Context())); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to upsert note", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
