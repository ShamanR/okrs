// Package project serves POST /api/v1/krs/{krID}/progress/project.
package project

import (
	"encoding/json"
	"net/http"

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
		Stages []struct {
			ID   int64 `json:"id"`
			Done bool  `json:"done"`
		} `json:"stages"`
		HealthStatus *string `json:"health_status"`
		Note         *string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	status, written := krscommon.ParseHealthStatus(w, req.HealthStatus)
	if written {
		return
	}
	if len(req.Stages) == 0 {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "stages required", map[string]string{"stages": "required"})
		return
	}
	updates := make([]kruc.ProjectStageUpdate, 0, len(req.Stages))
	for _, stage := range req.Stages {
		if stage.ID == 0 {
			v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "stage id required", map[string]string{"stage_id": "required"})
			return
		}
		updates = append(updates, kruc.ProjectStageUpdate{ID: stage.ID, IsDone: stage.Done})
	}
	if req.Note != nil {
		normalized := krscommon.NormalizeNoteText(*req.Note)
		req.Note = &normalized
	}
	in := kruc.CheckInInput{Project: updates, Note: req.Note, Health: status}
	if err := h.uc.CheckIn(r.Context(), scope, krID, in, auth.UserIDFromContext(r.Context())); err != nil {
		v1.WriteError(w, http.StatusConflict, "CONFLICT", err.Error(), nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
