package v1

import (
	"net/http"

	"okrs/internal/domain"
	"okrs/internal/http/handlers/common"

	"github.com/go-chi/chi/v5"
)

// handleTeams returns team summaries for a period.
func (h *Handler) handleTeams(w http.ResponseWriter, r *http.Request) {
	periodID, err := common.ParsePeriodID(r)
	if err != nil || periodID == 0 {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid period id", map[string]string{"period_id": "invalid"})
		return
	}
	orgID, err := parseOptionalID(r.URL.Query().Get("org_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid org_id", map[string]string{"org_id": "invalid"})
		return
	}
	period, err := h.service.GetPeriod(r.Context(), periodID)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "period not found", map[string]string{"period_id": "not_found"})
		return
	}
	teams, err := h.service.GetTeamsWithPeriodSummary(r.Context(), periodID, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to load teams", nil)
		return
	}
	writeJSON(w, http.StatusOK, mapTeamsResponse(period, teams))
}

// handleTeam returns a single team.
func (h *Handler) handleTeam(w http.ResponseWriter, r *http.Request) {
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team id", map[string]string{"team_id": "invalid"})
		return
	}
	team, err := h.service.GetTeam(r.Context(), teamID)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "team not found", nil)
		return
	}
	writeJSON(w, http.StatusOK, teamInfo{
		ID:        team.ID,
		Name:      team.Name,
		Type:      string(team.Type),
		TypeLabel: common.TeamTypeLabel(team.Type),
		ParentID:  team.ParentID,
	})
}

// handleTeamOKRs returns OKR data for a team and period.
func (h *Handler) handleTeamOKRs(w http.ResponseWriter, r *http.Request) {
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team id", map[string]string{"team_id": "invalid"})
		return
	}
	periodID, err := common.ParsePeriodID(r)
	if err != nil || periodID == 0 {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid period id", map[string]string{"period_id": "invalid"})
		return
	}
	period, err := h.service.GetPeriod(r.Context(), periodID)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "period not found", map[string]string{"period_id": "not_found"})
		return
	}
	okr, err := h.service.GetTeamOKR(r.Context(), teamID, periodID, period)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "team okr not found", nil)
		return
	}
	writeJSON(w, http.StatusOK, mapTeamOKRResponse(okr))
}

// handleUpdateTeamPeriodStatus updates the period status for a team.
func (h *Handler) handleUpdateTeamPeriodStatus(w http.ResponseWriter, r *http.Request) {
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team id", map[string]string{"team_id": "invalid"})
		return
	}
	if err := r.ParseMultipartForm(maxMultipartMemory); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	periodID, err := common.ParsePeriodID(r)
	if err != nil || periodID == 0 {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid period id", map[string]string{"period_id": "invalid"})
		return
	}
	status := domain.TeamPeriodStatus(r.FormValue("status"))
	if !common.ValidTeamPeriodStatus(status) {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid status", map[string]string{"status": "invalid"})
		return
	}
	if err := h.service.UpdateTeamPeriodStatus(r.Context(), teamID, periodID, status); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to update status", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
