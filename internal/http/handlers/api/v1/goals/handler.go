// Package goals serves /api/v1/goals/{goalID}.
package goals

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/http/dto"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/api/v1/goals/goalcommon"
	"okrs/internal/http/handlers/web/common"
	goalsvc "okrs/internal/service/goal"
	goallinksvc "okrs/internal/service/goallink"
	goalsharesvc "okrs/internal/service/goalshare"
	usersvc "okrs/internal/service/user"
	"okrs/internal/store/goals"
	goaluc "okrs/internal/usecase/goal"
)

type Handler struct {
	goals  *goalsvc.Service
	shares *goalsharesvc.Service
	links  *goallinksvc.Service
	users  *usersvc.Service
	uc     *goaluc.UseCase
}

func New(goals *goalsvc.Service, shares *goalsharesvc.Service, links *goallinksvc.Service, users *usersvc.Service, uc *goaluc.UseCase) *Handler {
	return &Handler{goals: goals, shares: shares, links: links, users: users, uc: uc}
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid goal id", map[string]string{"goal_id": "invalid"})
		return
	}
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	goal, err := h.goals.Get(r.Context(), scope, goalID)
	if err != nil {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	if !auth.CanAccessTeamFromCtx(r.Context(), goal.TeamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	// Attach scope-filtered parent/child links (navigation-only).
	allowed, _ := auth.AllowedTeamIDsFromCtx(r.Context())
	if parents, children, err := h.links.ListForGoals(r.Context(), scope, []int64{goal.ID}, allowed, allowed == nil); err == nil {
		goal.Parents = parents[goal.ID]
		goal.Children = children[goal.ID]
	}
	var userRefs map[string]*dto.UserRef
	if len(goal.OwnerUDIDs) > 0 {
		users, _ := h.users.GetByUDIDs(r.Context(), goal.OwnerUDIDs)
		userRefs = v1.BuildUserRefMap(users)
	}
	v1.WriteJSON(w, http.StatusOK, goalcommon.GoalResponse(goal, userRefs))
}
func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid goal id", map[string]string{"goal_id": "invalid"})
		return
	}
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	if goal, err := h.goals.Get(r.Context(), scope, goalID); err != nil || !auth.CanAccessTeamFromCtx(r.Context(), goal.TeamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	priority := domain.Priority(r.FormValue("priority"))
	workType := domain.WorkType(r.FormValue("work_type"))
	focusType := domain.FocusType(r.FormValue("focus_type"))
	teamID, err := goalcommon.OptionalID(r.FormValue("team_id"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team_id", map[string]string{"team_id": "invalid"})
		return
	}
	weight := common.ParseIntField(r.FormValue("weight"))
	if validationErr := common.ValidateGoalInput(priority, workType, focusType, weight); validationErr != "" {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", validationErr, nil)
		return
	}
	ownerUDIDs := r.Form["owner_udids"]
	if len(ownerUDIDs) > 0 {
		missing, err := h.users.ValidateUDIDsExist(r.Context(), ownerUDIDs)
		if err != nil {
			v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to validate owners", nil)
			return
		}
		if len(missing) > 0 {
			v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "unknown owner UDIDs", map[string]string{"owner_udids": "unknown: " + strings.Join(missing, ", ")})
			return
		}
	}
	goalWeight := weight
	if teamID != nil {
		goal, err := h.goals.Get(r.Context(), scope, goalID)
		if err != nil {
			v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
			return
		}
		goalWeight = goal.Weight
	}
	if err := h.uc.Update(r.Context(), scope, goals.GoalUpdateInput{
		ID:          goalID,
		Title:       common.TrimmedFormValue(r, "title"),
		Description: common.TrimmedFormValue(r, "description"),
		Priority:    priority,
		Weight:      goalWeight,
		WorkType:    workType,
		FocusType:   focusType,
		OwnerUDIDs:  ownerUDIDs,
	}, auth.UserIDFromContext(r.Context())); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to update goal", nil)
		return
	}
	if teamID != nil {
		if err := h.shares.UpdateWeight(r.Context(), scope, goalID, *teamID, weight); err != nil {
			v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to update weight", nil)
			return
		}
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid goal id", map[string]string{"goal_id": "invalid"})
		return
	}
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	goal, err := h.goals.Get(r.Context(), scope, goalID)
	if err != nil || !auth.CanAccessTeamFromCtx(r.Context(), goal.TeamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	if _, _, err := h.uc.Delete(r.Context(), scope, goalID, goal.TeamID, auth.UserIDFromContext(r.Context())); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
