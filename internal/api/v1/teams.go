package v1

import (
	"errors"
	"net/http"

	"okrs/internal/domain"
	"okrs/internal/http/handlers/common"
	"okrs/internal/service"

	"github.com/go-chi/chi/v5"
)

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

func collectDescendantIDs(targetID int64, nodes []teamNode) []int64 {
	var descendants []int64
	var walk func(items []teamNode, collect bool)
	walk = func(items []teamNode, collect bool) {
		for _, node := range items {
			nextCollect := collect || node.ID == targetID
			if collect {
				descendants = append(descendants, node.ID)
			}
			if len(node.Children) > 0 {
				walk(node.Children, nextCollect)
			}
		}
	}
	walk(nodes, false)
	return descendants
}

func (h *Handler) handleTeamOverview(w http.ResponseWriter, r *http.Request) {
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

	hierarchy, err := h.service.GetHierarchy(r.Context(), &periodID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to load hierarchy", nil)
		return
	}
	nodes := mapHierarchy(hierarchy)
	descendantIDs := collectDescendantIDs(teamID, nodes)

	totalProgress := 0
	teamsWithGoals := 0
	priorities := prioritySummaryInfo{}
	workBalance := workBalanceInfo{}

	for _, id := range descendantIDs {
		okrData, err := h.service.GetTeamOKR(r.Context(), id, periodID, period)
		if err != nil {
			if errors.Is(err, service.ErrTeamNotVisibleInPeriod) {
				continue
			}
			continue
		}
		if len(okrData.Goals) == 0 {
			continue
		}
		teamsWithGoals++
		totalProgress += okrData.PeriodProgress
		for _, goal := range okrData.Goals {
			switch goal.Goal.Priority {
			case domain.PriorityP0:
				priorities.P0++
			case domain.PriorityP1:
				priorities.P1++
			case domain.PriorityP2:
				priorities.P2++
			case domain.PriorityP3:
				priorities.P3++
			}
			switch goal.Goal.WorkType {
			case domain.WorkTypeDiscovery:
				workBalance.Discovery++
			case domain.WorkTypeDelivery:
				workBalance.Delivery++
			}
		}
	}

	avgProgress := 0
	if teamsWithGoals > 0 {
		avgProgress = totalProgress / teamsWithGoals
	}

	writeJSON(w, http.StatusOK, teamOverviewResponse{
		AverageProgress: avgProgress,
		TeamsWithGoals:  teamsWithGoals,
		ProgressMeta:    buildProgressBarInfo(avgProgress, period),
		Priorities:      priorities,
		WorkBalance:     workBalance,
	})
}

func (h *Handler) handleTeamChildrenSummary(w http.ResponseWriter, r *http.Request) {
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
	items, err := h.service.GetDirectChildrenSummary(r.Context(), teamID, periodID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to load children summary", nil)
		return
	}
	writeJSON(w, http.StatusOK, mapTeamChildrenSummaryResponse(period, items))
}
