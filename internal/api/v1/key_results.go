package v1

import (
	"encoding/json"
	"net/http"

	"okrs/internal/domain"
	"okrs/internal/http/handlers/common"
	"okrs/internal/store"

	"github.com/go-chi/chi/v5"
)

// handleAddKRComment adds a comment to a key result.
func (h *Handler) handleAddKRComment(w http.ResponseWriter, r *http.Request) {
	krID, err := common.ParseID(chi.URLParam(r, "krID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid kr id", map[string]string{"kr_id": "invalid"})
		return
	}
	var req addCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if req.Text == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "text required", map[string]string{"text": "required"})
		return
	}
	if err := h.service.AddKeyResultComment(r.Context(), krID, req.Text); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to add comment", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleUpdateKeyResult updates key result fields and metadata.
func (h *Handler) handleUpdateKeyResult(w http.ResponseWriter, r *http.Request) {
	krID, err := common.ParseID(chi.URLParam(r, "krID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid kr id", map[string]string{"kr_id": "invalid"})
		return
	}
	if err := r.ParseMultipartForm(maxMultipartMemory); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	kind := domain.KRKind(r.FormValue("kind"))
	if !common.ValidKRKind(kind) {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid kr kind", map[string]string{"kind": "invalid"})
		return
	}
	weight := common.ParseIntField(r.FormValue("weight"))
	if weight < 0 || weight > 100 {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid weight", map[string]string{"weight": "0..100"})
		return
	}
	meta, err := parseKeyResultMeta(r, kind)
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	if err := h.service.UpdateKeyResultWithMeta(r.Context(), store.KeyResultUpdateInput{
		ID:          krID,
		Title:       common.TrimmedFormValue(r, "title"),
		Description: common.TrimmedFormValue(r, "description"),
		Weight:      weight,
		Kind:        kind,
	}, meta); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to update key result", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleMoveKeyResultUp moves a key result up within the goal.
func (h *Handler) handleMoveKeyResultUp(w http.ResponseWriter, r *http.Request) {
	h.handleMoveKeyResult(w, r, -1)
}

// handleMoveKeyResultDown moves a key result down within the goal.
func (h *Handler) handleMoveKeyResultDown(w http.ResponseWriter, r *http.Request) {
	h.handleMoveKeyResult(w, r, 1)
}

// handleMoveKeyResult moves a key result in the given direction.
func (h *Handler) handleMoveKeyResult(w http.ResponseWriter, r *http.Request, direction int) {
	krID, err := common.ParseID(chi.URLParam(r, "krID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid kr id", map[string]string{"kr_id": "invalid"})
		return
	}
	if err := r.ParseMultipartForm(maxMultipartMemory); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if err := h.service.MoveKeyResult(r.Context(), krID, direction); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to move key result", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
