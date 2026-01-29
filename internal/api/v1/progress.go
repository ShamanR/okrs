package v1

import (
	"encoding/json"
	"net/http"

	"okrs/internal/http/handlers/common"
	"okrs/internal/service"

	"github.com/go-chi/chi/v5"
)

type updatePercentRequest struct {
	CurrentValue float64 `json:"current_value"`
}

type updateBooleanRequest struct {
	Done bool `json:"done"`
}

type updateProjectRequest struct {
	Stages []updateProjectStage `json:"stages"`
}

type updateProjectStage struct {
	ID   int64 `json:"id"`
	Done bool  `json:"done"`
}

// handleUpdatePercentProgress updates percent/linear current value.
func (h *Handler) handleUpdatePercentProgress(w http.ResponseWriter, r *http.Request) {
	krID, err := common.ParseID(chi.URLParam(r, "krID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid kr id", map[string]string{"kr_id": "invalid"})
		return
	}
	var req updatePercentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if err := h.service.UpdateKRProgressPercent(r.Context(), krID, req.CurrentValue); err != nil {
		writeError(w, http.StatusConflict, "CONFLICT", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleUpdateBooleanProgress updates boolean progress.
func (h *Handler) handleUpdateBooleanProgress(w http.ResponseWriter, r *http.Request) {
	krID, err := common.ParseID(chi.URLParam(r, "krID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid kr id", map[string]string{"kr_id": "invalid"})
		return
	}
	var req updateBooleanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if err := h.service.UpdateKRProgressBoolean(r.Context(), krID, req.Done); err != nil {
		writeError(w, http.StatusConflict, "CONFLICT", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleUpdateProjectProgress updates project stage progress.
func (h *Handler) handleUpdateProjectProgress(w http.ResponseWriter, r *http.Request) {
	krID, err := common.ParseID(chi.URLParam(r, "krID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid kr id", map[string]string{"kr_id": "invalid"})
		return
	}
	var req updateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if len(req.Stages) == 0 {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "stages required", map[string]string{"stages": "required"})
		return
	}
	updates := make([]service.ProjectStageUpdate, 0, len(req.Stages))
	for _, stage := range req.Stages {
		if stage.ID == 0 {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "stage id required", map[string]string{"stage_id": "required"})
			return
		}
		updates = append(updates, service.ProjectStageUpdate{ID: stage.ID, IsDone: stage.Done})
	}
	if err := h.service.UpdateKRProgressProject(r.Context(), krID, updates); err != nil {
		writeError(w, http.StatusConflict, "CONFLICT", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
