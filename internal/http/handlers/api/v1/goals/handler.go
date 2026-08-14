package goals

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"okrs/internal/auth"
	"okrs/internal/domain"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/dto"
	"okrs/internal/http/handlers/web/common"
	"okrs/internal/service"
	"okrs/internal/store/goals"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service *service.Service
}

func New(service *service.Service) *Handler {
	return &Handler{service: service}
}

// canAccessGoal reports whether the current request may act on a goal that is
// visible to the user: it must be owned by an accessible team or shared into one.
// A shared goal appears on the cards of every team it is shared into, so comment /
// resolve actions must accept users who can reach any of those teams — not only the
// owner team. Mirrors the shared-goal visibility used by the OKR list and move/leave-share.
func (h *Handler) canAccessGoal(ctx context.Context, scope domain.TenantScope, goal domain.Goal) bool {
	if auth.CanAccessTeamFromCtx(ctx, goal.TeamID) {
		return true
	}
	shareList, err := h.service.ListGoalShares(ctx, scope, goal.ID)
	if err != nil {
		return false
	}
	for _, sh := range shareList {
		if auth.CanAccessTeamFromCtx(ctx, sh.TeamID) {
			return true
		}
	}
	return false
}

func (h *Handler) HandleGoal(w http.ResponseWriter, r *http.Request) {
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
	goal, err := h.service.GetGoal(r.Context(), scope, goalID)
	if err != nil {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	if !auth.CanAccessTeamFromCtx(r.Context(), goal.TeamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	var userRefs map[string]*dto.UserRef
	if len(goal.OwnerUDIDs) > 0 {
		users, _ := h.service.GetUsersByUDIDs(r.Context(), goal.OwnerUDIDs)
		userRefs = v1.BuildUserRefMap(users)
	}
	v1.WriteJSON(w, http.StatusOK, newGoalResponse(goal, userRefs))
}

func (h *Handler) HandleShareGoal(w http.ResponseWriter, r *http.Request) {
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
	if goal, err := h.service.GetGoal(r.Context(), scope, goalID); err != nil || !auth.CanAccessTeamFromCtx(r.Context(), goal.TeamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	var req struct {
		Targets []struct {
			TeamID int64 `json:"team_id"`
			Weight int   `json:"weight"`
		} `json:"targets"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if len(req.Targets) == 0 {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "targets required", map[string]string{"targets": "required"})
		return
	}
	targets := make([]service.ShareTarget, 0, len(req.Targets))
	for _, target := range req.Targets {
		if target.TeamID == 0 {
			v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team_id", map[string]string{"team_id": "required"})
			return
		}
		if target.Weight < 0 || target.Weight > 100 {
			v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid weight", map[string]string{"weight": "0..100"})
			return
		}
		targets = append(targets, service.ShareTarget{TeamID: target.TeamID, Weight: target.Weight})
	}
	if err := h.service.ShareGoal(r.Context(), scope, goalID, targets, auth.UserIDFromContext(r.Context())); err != nil {
		if errors.Is(err, service.ErrShareTargetNotInTenant) {
			v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team_id", map[string]string{"team_id": "not in tenant"})
			return
		}
		if errors.Is(err, service.ErrCannotShareWithClosedPeriod) {
			v1.WriteError(w, http.StatusConflict, "PERIOD_STARTED", "cannot add a team whose period is already in progress or closed", map[string]string{"team_id": "period_started"})
			return
		}
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to share goal", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) HandleUpdateGoalWeight(w http.ResponseWriter, r *http.Request) {
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
	if goal, err := h.service.GetGoal(r.Context(), scope, goalID); err != nil || !auth.CanAccessTeamFromCtx(r.Context(), goal.TeamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	var req struct {
		TeamID int64 `json:"team_id"`
		Weight int   `json:"weight"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if req.TeamID == 0 {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "team_id required", map[string]string{"team_id": "required"})
		return
	}
	if req.Weight < 0 || req.Weight > 100 {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid weight", map[string]string{"weight": "0..100"})
		return
	}
	if err := h.service.UpdateGoalWeight(r.Context(), scope, goalID, req.TeamID, req.Weight); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to update weight", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) HandleAddGoalComment(w http.ResponseWriter, r *http.Request) {
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
	goal, err := h.service.GetGoal(r.Context(), scope, goalID)
	if err != nil || !h.canAccessGoal(r.Context(), scope, goal) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if req.Text == "" {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "text required", map[string]string{"text": "required"})
		return
	}
	if err := h.service.AddGoalComment(r.Context(), scope, goalID, req.Text, auth.UserIDFromContext(r.Context())); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to add comment", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) HandleResolveGoalComment(w http.ResponseWriter, r *http.Request) {
	h.setGoalCommentResolved(w, r, true)
}

func (h *Handler) HandleUnresolveGoalComment(w http.ResponseWriter, r *http.Request) {
	h.setGoalCommentResolved(w, r, false)
}

// setGoalCommentResolved marks a comment resolved or clears the resolution.
// Access is gated by access to the parent goal — any user in the goal's scope
// may resolve/reopen, matching who may comment.
func (h *Handler) setGoalCommentResolved(w http.ResponseWriter, r *http.Request, resolved bool) {
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid goal id", map[string]string{"goal_id": "invalid"})
		return
	}
	commentID, err := common.ParseID(chi.URLParam(r, "commentID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid comment id", map[string]string{"comment_id": "invalid"})
		return
	}
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	goal, err := h.service.GetGoal(r.Context(), scope, goalID)
	if err != nil || !h.canAccessGoal(r.Context(), scope, goal) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	if err := h.service.SetGoalCommentResolved(r.Context(), scope, goalID, commentID, resolved, auth.UserIDFromContext(r.Context())); err != nil {
		if errors.Is(err, goals.ErrNotFound) {
			v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "comment not found", nil)
			return
		}
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to update comment", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// HandleAddGoalReply creates a reply under a task. Access chain: tenant scope → goal
// reachable by the caller (owner or shared team) → the parent must be a task of this goal
// (enforced in the store; a bad/non-task parent → 404).
func (h *Handler) HandleAddGoalReply(w http.ResponseWriter, r *http.Request) {
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid goal id", map[string]string{"goal_id": "invalid"})
		return
	}
	commentID, err := common.ParseID(chi.URLParam(r, "commentID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid comment id", map[string]string{"comment_id": "invalid"})
		return
	}
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	goal, err := h.service.GetGoal(r.Context(), scope, goalID)
	if err != nil || !h.canAccessGoal(r.Context(), scope, goal) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if req.Text == "" {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "text required", map[string]string{"text": "required"})
		return
	}
	if err := h.service.AddGoalReply(r.Context(), scope, goalID, commentID, req.Text, auth.UserIDFromContext(r.Context())); err != nil {
		if errors.Is(err, goals.ErrNotFound) {
			v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "comment not found", nil)
			return
		}
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to add reply", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// HandleDeleteGoalComment deletes a task (cascading its replies) or a reply. Access chain:
// tenant scope → goal reachable by the caller → the service enforces author-or-admin and pins
// the comment to the goal/tenant. Author or tenant admin only; others → 403.
func (h *Handler) HandleDeleteGoalComment(w http.ResponseWriter, r *http.Request) {
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid goal id", map[string]string{"goal_id": "invalid"})
		return
	}
	commentID, err := common.ParseID(chi.URLParam(r, "commentID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid comment id", map[string]string{"comment_id": "invalid"})
		return
	}
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	goal, err := h.service.GetGoal(r.Context(), scope, goalID)
	if err != nil || !h.canAccessGoal(r.Context(), scope, goal) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	role, _ := auth.ActiveRoleFromContext(r.Context())
	isAdmin := role == domain.RoleAdmin
	if _, err := h.service.DeleteGoalComment(r.Context(), scope, goalID, commentID, auth.UserIDFromContext(r.Context()), isAdmin); err != nil {
		if errors.Is(err, goals.ErrNotFound) {
			v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "comment not found", nil)
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "not allowed to delete", nil)
			return
		}
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to delete comment", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) HandleUpdateGoal(w http.ResponseWriter, r *http.Request) {
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
	if goal, err := h.service.GetGoal(r.Context(), scope, goalID); err != nil || !auth.CanAccessTeamFromCtx(r.Context(), goal.TeamID) {
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
	teamID, err := parseOptionalID(r.FormValue("team_id"))
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
		missing, err := h.service.ValidateUserUDIDsExist(r.Context(), ownerUDIDs)
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
		goal, err := h.service.GetGoal(r.Context(), scope, goalID)
		if err != nil {
			v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
			return
		}
		goalWeight = goal.Weight
	}
	if err := h.service.UpdateGoal(r.Context(), scope, goals.GoalUpdateInput{
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
		if err := h.service.UpdateGoalWeight(r.Context(), scope, goalID, *teamID, weight); err != nil {
			v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to update weight", nil)
			return
		}
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) HandleMoveGoalUp(w http.ResponseWriter, r *http.Request) {
	h.handleMoveGoal(w, r, -1)
}

func (h *Handler) HandleMoveGoalDown(w http.ResponseWriter, r *http.Request) {
	h.handleMoveGoal(w, r, 1)
}

// parseTeamID extracts team_id from either a JSON body (tracker SPA via apiPost)
// or a form field (legacy multipart page). Returns 0 when absent/invalid.
func parseTeamID(r *http.Request) int64 {
	if raw := strings.TrimSpace(r.FormValue("team_id")); raw != "" {
		if id, err := common.ParseID(raw); err == nil {
			return id
		}
		return 0
	}
	var req struct {
		TeamID int64 `json:"team_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return 0
	}
	return req.TeamID
}

func (h *Handler) handleMoveGoal(w http.ResponseWriter, r *http.Request, direction int) {
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
	// The move is scoped to the team whose period is being viewed: a shared goal
	// has an independent order per team, so reordering must target that team's list.
	// The tracker SPA sends team_id as a JSON body; the legacy page as a form field.
	teamID := parseTeamID(r)
	if teamID == 0 {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team id", map[string]string{"team_id": "invalid"})
		return
	}
	if !auth.CanAccessTeamFromCtx(r.Context(), teamID) {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "access denied", nil)
		return
	}
	// The goal must actually belong to teamID's view — either owned by it or shared into it.
	goal, err := h.service.GetGoal(r.Context(), scope, goalID)
	if err != nil {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	if goal.TeamID != teamID {
		if _, err := h.service.GetGoalShare(r.Context(), scope, goalID, teamID); err != nil {
			v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
			return
		}
	}
	if err := h.service.MoveGoal(r.Context(), scope, teamID, goalID, direction); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to move goal", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) HandleLeaveGoalShare(w http.ResponseWriter, r *http.Request) {
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid goal id", map[string]string{"goal_id": "invalid"})
		return
	}
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team id", map[string]string{"team_id": "invalid"})
		return
	}
	if !auth.CanAccessTeamFromCtx(r.Context(), teamID) {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "access denied", nil)
		return
	}
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	// The goal must actually be attached to teamID (as owner or as a shared team), otherwise there
	// is nothing to detach. Returning NOT_FOUND avoids reporting a successful no-op and prevents
	// probing arbitrary goal IDs as an existence oracle (see access rules in specs/040).
	goal, err := h.service.GetGoal(r.Context(), scope, goalID)
	if err != nil {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	if goal.TeamID != teamID {
		if _, err := h.service.GetGoalShare(r.Context(), scope, goalID, teamID); err != nil {
			v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
			return
		}
	}
	if _, _, err := h.service.DeleteGoal(r.Context(), scope, goalID, teamID, auth.UserIDFromContext(r.Context())); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) HandleDeleteGoal(w http.ResponseWriter, r *http.Request) {
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
	goal, err := h.service.GetGoal(r.Context(), scope, goalID)
	if err != nil || !auth.CanAccessTeamFromCtx(r.Context(), goal.TeamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	if _, _, err := h.service.DeleteGoal(r.Context(), scope, goalID, goal.TeamID, auth.UserIDFromContext(r.Context())); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
