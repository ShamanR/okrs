// Package export serves GET /api/v1/teams/{teamID}/export.
package export

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/go-chi/chi/v5"
	"okrs/internal/auth"
	"okrs/internal/core/domain"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/web/common"
	render "okrs/internal/render/export"
	exportuc "okrs/internal/usecase/export"
)

type Handler struct {
	exportUC *exportuc.UseCase
}

func New(exportUC *exportuc.UseCase) *Handler { return &Handler{exportUC: exportUC} }

// GET /api/v1/teams/{teamID}/export
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
	scopeCtx, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "no active tenant", nil)
		return
	}
	periodID, err := common.ParsePeriodID(r)
	if err != nil || periodID == 0 {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid period id", map[string]string{"period_id": "invalid"})
		return
	}

	q := r.URL.Query()
	exportScope := render.Scope(q.Get("scope"))
	if exportScope != render.ScopeGoal && exportScope != render.ScopeTeam && exportScope != render.ScopeTree {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid scope", map[string]string{"scope": "invalid"})
		return
	}
	format := render.Format(q.Get("format"))
	if format == "" {
		format = render.FormatShort
	}
	if format != render.FormatShort && format != render.FormatFull {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid format", map[string]string{"format": "invalid"})
		return
	}
	var goalID int64
	if exportScope == render.ScopeGoal {
		goalID, err = common.ParseID(q.Get("goal_id"))
		if err != nil || goalID == 0 {
			v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "goal_id required", map[string]string{"goal_id": "required"})
			return
		}
	}
	allowed, _ := auth.AllowedTeamIDsFromCtx(r.Context())

	res, err := h.exportUC.ExportOKR(r.Context(), scopeCtx, exportuc.ExportParams{
		TeamID: teamID, PeriodID: periodID, GoalID: goalID, Scope: exportScope,
		Options:        render.Options{Format: format, Comments: q.Get("comments") == "1"},
		AllowedTeamIDs: allowed,
	})
	if err != nil {
		// Genuine "not found" (goal not on board, team invisible in period, missing team/period)
		// maps to 404; anything else (DB/store failure) is an operational 500.
		if errors.Is(err, domain.ErrGoalNotOnTeamBoard) ||
			errors.Is(err, domain.ErrTeamNotVisibleInPeriod) ||
			errors.Is(err, pgx.ErrNoRows) {
			v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "export not available", nil)
			return
		}
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to build export", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]any{
		"filename": res.Filename,
		"markdown": res.Markdown,
		"lines":    res.Lines,
	})
}
