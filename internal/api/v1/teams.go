package v1

import (
	"net/http"
	"time"

	"okrs/internal/domain"
	"okrs/internal/http/handlers/common"

	"github.com/go-chi/chi/v5"
)

// handleTeams returns team summaries for a quarter.
func (h *Handler) handleTeams(w http.ResponseWriter, r *http.Request) {
	year, quarter := common.ParseQuarter(r, time.UTC)
	orgID, err := parseOptionalID(r.URL.Query().Get("org_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid org_id", map[string]string{"org_id": "invalid"})
		return
	}
	teams, err := h.service.GetTeamsWithQuarterSummary(r.Context(), year, quarter, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to load teams", nil)
		return
	}
	writeJSON(w, http.StatusOK, mapTeamsResponse(year, quarter, teams))
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

// handleTeamOKRs returns OKR data for a team and quarter.
func (h *Handler) handleTeamOKRs(w http.ResponseWriter, r *http.Request) {
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team id", map[string]string{"team_id": "invalid"})
		return
	}
	year, quarter := common.ParseQuarter(r, time.UTC)
	okr, err := h.service.GetTeamOKR(r.Context(), teamID, year, quarter)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "team okr not found", nil)
		return
	}
	writeJSON(w, http.StatusOK, mapTeamOKRResponse(okr))
}

// handleUpdateTeamQuarterStatus updates the quarter status for a team.
func (h *Handler) handleUpdateTeamQuarterStatus(w http.ResponseWriter, r *http.Request) {
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team id", map[string]string{"team_id": "invalid"})
		return
	}
	if err := r.ParseMultipartForm(maxMultipartMemory); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	year, quarter := common.ParseQuarter(r, time.UTC)
	status := domain.TeamQuarterStatus(r.FormValue("status"))
	if !common.ValidTeamQuarterStatus(status) {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid status", map[string]string{"status": "invalid"})
		return
	}
	if err := h.service.UpdateTeamQuarterStatus(r.Context(), teamID, year, quarter, status); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to update status", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
