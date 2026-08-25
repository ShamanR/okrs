// Package okrs serves GET /api/v1/teams/{teamID}/okrs.
package okrs

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"okrs/internal/auth"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/api/v1/teams/teamscommon"
	"okrs/internal/http/handlers/web/common"
	periodsvc "okrs/internal/service/period"
	usersvc "okrs/internal/service/user"
	okrboarduc "okrs/internal/usecase/okrboard"
)

type Handler struct {
	board   *okrboarduc.UseCase
	periods *periodsvc.Service
	users   *usersvc.Service
}

func New(board *okrboarduc.UseCase, periods *periodsvc.Service, users *usersvc.Service) *Handler {
	return &Handler{board: board, periods: periods, users: users}
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team id", map[string]string{"team_id": "invalid"})
		return
	}
	if !auth.CanAccessTeamFromCtx(r.Context(), teamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "team not found", nil)
		return
	}
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	periodID, err := common.ParsePeriodID(r)
	if err != nil || periodID == 0 {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid period id", map[string]string{"period_id": "invalid"})
		return
	}
	period, err := h.periods.Get(r.Context(), scope, periodID)
	if err != nil {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "period not found", map[string]string{"period_id": "not_found"})
		return
	}
	okr, err := h.board.TeamOKRFor(r.Context(), scope, teamID, periodID, period)
	if err != nil {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "team okr not found", nil)
		return
	}
	// Attach scope-filtered parent/child links to each goal (navigation-only). Best-effort:
	// links are decoration, so a failure here degrades to a board without labels rather than
	// breaking the whole view (mirrors HandleGoal).
	allowed, _ := auth.AllowedTeamIDsFromCtx(r.Context())
	_ = h.board.AttachLinks(r.Context(), scope, okr.Goals, allowed, allowed == nil)
	udids := teamscommon.CollectOKRUserUDIDs(okr)
	users, _ := h.users.GetByUDIDs(r.Context(), udids)
	v1.WriteJSON(w, http.StatusOK, teamscommon.TeamOKRResponse(okr, v1.BuildUserRefMap(users)))
}
