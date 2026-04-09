package teams

import (
	"net/http"

	"okrs/internal/domain"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/web/common"
	"okrs/internal/service"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service *service.Service
}

func New(service *service.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) HandleTeam(w http.ResponseWriter, r *http.Request) {
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team id", map[string]string{"team_id": "invalid"})
		return
	}
	team, err := h.service.GetTeam(r.Context(), teamID)
	if err != nil {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "team not found", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]any{
		"id":         team.ID,
		"name":       team.Name,
		"type":       string(team.Type),
		"type_label": common.TeamTypeLabel(team.Type),
		"parent_id":  team.ParentID,
	})
}

func (h *Handler) HandleTeamOKRs(w http.ResponseWriter, r *http.Request) {
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team id", map[string]string{"team_id": "invalid"})
		return
	}
	periodID, err := common.ParsePeriodID(r)
	if err != nil || periodID == 0 {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid period id", map[string]string{"period_id": "invalid"})
		return
	}
	period, err := h.service.GetPeriod(r.Context(), periodID)
	if err != nil {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "period not found", map[string]string{"period_id": "not_found"})
		return
	}
	okr, err := h.service.GetTeamOKR(r.Context(), teamID, periodID, period)
	if err != nil {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "team okr not found", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, v1.NewTeamOKRResponse(okr))
}

func (h *Handler) HandleTeamOverview(w http.ResponseWriter, r *http.Request) {
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team id", map[string]string{"team_id": "invalid"})
		return
	}
	periodID, err := common.ParsePeriodID(r)
	if err != nil || periodID == 0 {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid period id", map[string]string{"period_id": "invalid"})
		return
	}
	period, err := h.service.GetPeriod(r.Context(), periodID)
	if err != nil {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "period not found", map[string]string{"period_id": "not_found"})
		return
	}
	overview, err := h.service.GetTeamOverview(r.Context(), teamID, periodID)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to load team overview", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, v1.NewTeamOverviewResponse(period, overview))
}

func (h *Handler) HandleUpdateTeamPeriodStatus(w http.ResponseWriter, r *http.Request) {
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team id", map[string]string{"team_id": "invalid"})
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	periodID, err := common.ParsePeriodID(r)
	if err != nil || periodID == 0 {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid period id", map[string]string{"period_id": "invalid"})
		return
	}
	status := domain.TeamPeriodStatus(r.FormValue("status"))
	if !common.ValidTeamPeriodStatus(status) {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid status", map[string]string{"status": "invalid"})
		return
	}
	if err := h.service.UpdateTeamPeriodStatus(r.Context(), teamID, periodID, status); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to update status", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
