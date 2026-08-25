// Package goals serves POST /api/v1/teams/{teamID}/goals.
package goals

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"okrs/internal/auth"
	"okrs/internal/core/domain"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/web/common"
	usersvc "okrs/internal/service/user"
	"okrs/internal/store/goals"
	goaluc "okrs/internal/usecase/goal"
)

type Handler struct {
	goalUC *goaluc.UseCase
	users  *usersvc.Service
}

func New(goalUC *goaluc.UseCase, users *usersvc.Service) *Handler {
	return &Handler{goalUC: goalUC, users: users}
}

// POST /api/v1/teams/{teamID}/goals
func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team id", nil)
		return
	}
	if !auth.CanAccessTeamFromCtx(r.Context(), teamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "team not found", nil)
		return
	}
	var req struct {
		PeriodID    int64    `json:"period_id"`
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Priority    string   `json:"priority"`
		Weight      int      `json:"weight"`
		WorkType    string   `json:"work_type"`
		FocusType   string   `json:"focus_type"`
		OwnerUDIDs  []string `json:"owner_udids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if req.Title == "" {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "title required", map[string]string{"title": "required"})
		return
	}
	if req.PeriodID == 0 {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "period_id required", map[string]string{"period_id": "required"})
		return
	}
	priority := domain.Priority(req.Priority)
	workType := domain.WorkType(req.WorkType)
	focusType := domain.FocusType(req.FocusType)
	if msg := common.ValidateGoalInput(priority, workType, focusType, req.Weight); msg != "" {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", msg, nil)
		return
	}
	if len(req.OwnerUDIDs) > 0 {
		missing, err := h.users.ValidateUDIDsExist(r.Context(), req.OwnerUDIDs)
		if err != nil {
			v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to validate owners", nil)
			return
		}
		if len(missing) > 0 {
			v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "unknown owner UDIDs", map[string]string{"owner_udids": "unknown: " + strings.Join(missing, ", ")})
			return
		}
	}
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	goalID, err := h.goalUC.Create(r.Context(), scope, goals.GoalInput{
		TeamID:      teamID,
		PeriodID:    req.PeriodID,
		Title:       req.Title,
		Description: req.Description,
		Priority:    priority,
		Weight:      req.Weight,
		WorkType:    workType,
		FocusType:   focusType,
		OwnerUDIDs:  req.OwnerUDIDs,
	}, auth.UserIDFromContext(r.Context()))
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to create goal", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]int64{"id": goalID})
}
