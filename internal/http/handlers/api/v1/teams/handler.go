package teams

import (
	"encoding/json"
	"net/http"
	"strings"

	"okrs/internal/auth"
	"okrs/internal/domain"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/web/common"
	"okrs/internal/service"
	"okrs/internal/store"

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
	if !auth.CanAccessTeamFromCtx(r.Context(), teamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "team not found", nil)
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
	if !auth.CanAccessTeamFromCtx(r.Context(), teamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "team not found", nil)
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
	names := collectOKRUserNames(okr)
	users, _ := h.service.GetUsersByDisplayNames(r.Context(), names)
	v1.WriteJSON(w, http.StatusOK, newTeamOKRResponse(okr, v1.BuildUserRefMap(users)))
}

func (h *Handler) HandleTeamOverview(w http.ResponseWriter, r *http.Request) {
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team id", map[string]string{"team_id": "invalid"})
		return
	}
	if !auth.CanAccessTeamFromCtx(r.Context(), teamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "team not found", nil)
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
	names := collectOverviewUserNames(overview)
	users, _ := h.service.GetUsersByDisplayNames(r.Context(), names)
	v1.WriteJSON(w, http.StatusOK, newTeamOverviewResponse(period, overview, v1.BuildUserRefMap(users)))
}

func (h *Handler) HandleUpdateTeamPeriodStatus(w http.ResponseWriter, r *http.Request) {
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team id", map[string]string{"team_id": "invalid"})
		return
	}
	if !auth.CanAccessTeamFromCtx(r.Context(), teamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "team not found", nil)
		return
	}
	var req struct {
		PeriodID int64  `json:"period_id"`
		Status   string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if req.PeriodID == 0 {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "period_id required", map[string]string{"period_id": "required"})
		return
	}
	status := domain.TeamPeriodStatus(req.Status)
	if !common.ValidTeamPeriodStatus(status) {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid status", map[string]string{"status": "invalid"})
		return
	}
	if err := h.service.UpdateTeamPeriodStatus(r.Context(), teamID, req.PeriodID, status); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to update status", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func collectOKRUserNames(okr service.TeamOKR) []string {
	seen := make(map[string]struct{})
	if okr.Team.Lead != "" {
		seen[okr.Team.Lead] = struct{}{}
	}
	for _, g := range okr.Goals {
		for _, part := range strings.Split(g.Goal.OwnerText, ",") {
			if name := strings.TrimSpace(part); name != "" {
				seen[name] = struct{}{}
			}
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	return names
}

func collectOverviewUserNames(overview service.TeamOverview) []string {
	seen := make(map[string]struct{})
	for _, item := range overview.ChildrenSummary {
		if item.Team.Lead != "" {
			seen[item.Team.Lead] = struct{}{}
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	return names
}

// POST /api/v1/teams/{teamID}/goals
func (h *Handler) HandleCreateGoal(w http.ResponseWriter, r *http.Request) {
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
		PeriodID    int64  `json:"period_id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Priority    string `json:"priority"`
		Weight      int    `json:"weight"`
		WorkType    string `json:"work_type"`
		FocusType   string `json:"focus_type"`
		OwnerText   string `json:"owner_text"`
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
	goalID, err := h.service.CreateGoal(r.Context(), store.GoalInput{
		TeamID:      teamID,
		PeriodID:    req.PeriodID,
		Title:       req.Title,
		Description: req.Description,
		Priority:    priority,
		Weight:      req.Weight,
		WorkType:    workType,
		FocusType:   focusType,
		OwnerText:   req.OwnerText,
	})
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to create goal", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]int64{"id": goalID})
}
