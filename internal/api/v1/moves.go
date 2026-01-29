package v1

import (
	"net/http"

	"okrs/internal/http/handlers/common"

	"github.com/go-chi/chi/v5"
)

// handleMoveGoalUp moves a goal up within its team.
func (h *Handler) handleMoveGoalUp(w http.ResponseWriter, r *http.Request) {
	h.handleMoveGoal(w, r, -1)
}

// handleMoveGoalDown moves a goal down within its team.
func (h *Handler) handleMoveGoalDown(w http.ResponseWriter, r *http.Request) {
	h.handleMoveGoal(w, r, 1)
}

// handleMoveGoal moves a goal in the given direction.
func (h *Handler) handleMoveGoal(w http.ResponseWriter, r *http.Request, direction int) {
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid goal id", map[string]string{"goal_id": "invalid"})
		return
	}
	if err := r.ParseMultipartForm(maxMultipartMemory); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if err := h.service.MoveGoal(r.Context(), goalID, direction); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to move goal", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
