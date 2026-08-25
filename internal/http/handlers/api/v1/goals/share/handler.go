// Package share serves the /api/v1/goals/… endpoints under its URI segment.
package share

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"okrs/internal/auth"
	"okrs/internal/core/domain"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/web/common"
	goalsvc "okrs/internal/service/goal"
	goalsharesvc "okrs/internal/service/goalshare"
	goaluc "okrs/internal/usecase/goal"
)

type Handler struct {
	goals  *goalsvc.Service
	shares *goalsharesvc.Service
	uc     *goaluc.UseCase
}

func New(goals *goalsvc.Service, shares *goalsharesvc.Service, uc *goaluc.UseCase) *Handler {
	return &Handler{goals: goals, shares: shares, uc: uc}
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
	targets := make([]goaluc.ShareTarget, 0, len(req.Targets))
	for _, target := range req.Targets {
		if target.TeamID == 0 {
			v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team_id", map[string]string{"team_id": "required"})
			return
		}
		if target.Weight < 0 || target.Weight > 100 {
			v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid weight", map[string]string{"weight": "0..100"})
			return
		}
		targets = append(targets, goaluc.ShareTarget{TeamID: target.TeamID, Weight: target.Weight})
	}
	if err := h.uc.Share(r.Context(), scope, goalID, targets, auth.UserIDFromContext(r.Context())); err != nil {
		if errors.Is(err, domain.ErrShareTargetNotInTenant) {
			v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team_id", map[string]string{"team_id": "not in tenant"})
			return
		}
		if errors.Is(err, domain.ErrCannotShareWithClosedPeriod) {
			v1.WriteError(w, http.StatusConflict, "PERIOD_STARTED", "cannot add a team whose period is already in progress or closed", map[string]string{"team_id": "period_started"})
			return
		}
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to share goal", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
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
	goal, err := h.goals.Get(r.Context(), scope, goalID)
	if err != nil {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	if goal.TeamID != teamID {
		if _, err := h.shares.Get(r.Context(), scope, goalID, teamID); err != nil {
			v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
			return
		}
	}
	if _, _, err := h.uc.Delete(r.Context(), scope, goalID, teamID, auth.UserIDFromContext(r.Context())); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
