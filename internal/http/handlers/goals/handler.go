package goals

import (
	"fmt"
	"net/http"
	"strings"

	"okrs/internal/domain"
	"okrs/internal/http/handlers/common"
	"okrs/internal/store"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	deps common.Dependencies
}

const maxMultipartMemory = 32 << 20

func New(deps common.Dependencies) *Handler {
	return &Handler{deps: deps}
}

func (h *Handler) HandleGoalDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	goal, err := h.deps.Store.GetGoal(ctx, goalID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	period, err := h.deps.Store.GetPeriod(ctx, goal.PeriodID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	teamID := goal.TeamID
	if value := r.URL.Query().Get("team"); value != "" {
		if parsed, err := common.ParseID(value); err == nil {
			if parsed != goal.TeamID {
				if share, err := h.deps.Store.GetGoalShare(ctx, goalID, parsed); err == nil {
					goal.Weight = share.Weight
					teamID = parsed
				}
			}
		}
	}
	team, err := h.deps.Store.GetTeam(ctx, teamID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	status, err := h.deps.Store.GetTeamPeriodStatus(ctx, team.ID, goal.PeriodID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}

	goal.Progress = common.CalculateGoalProgress(goal)

	page := struct {
		Team            domain.Team
		TeamTypeLabel   string
		Goal            domain.Goal
		Period          domain.Period
		IsClosed        bool
		FormError       string
		PageTitle       string
		ContentTemplate string
	}{Team: team, TeamTypeLabel: common.TeamTypeLabel(team.Type), Goal: goal, Period: period, IsClosed: status == domain.TeamPeriodStatusClosed, PageTitle: "Цель", ContentTemplate: "goal-content"}
	common.RenderTemplate(w, h.deps.Templates, "base", page, h.deps.Logger)
}

func (h *Handler) HandleAddGoalComment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if err := r.ParseMultipartForm(maxMultipartMemory); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	text := common.TrimmedFormValue(r, "text")
	if text == "" {
		if returnURL := r.FormValue("return"); returnURL != "" {
			http.Redirect(w, r, returnURL, http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/goals/%d", goalID), http.StatusSeeOther)
		return
	}
	if err := h.deps.Store.AddGoalComment(ctx, goalID, text); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if returnURL := r.FormValue("return"); returnURL != "" {
		http.Redirect(w, r, returnURL, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/goals/%d", goalID), http.StatusSeeOther)
}

func (h *Handler) HandleAddKeyResult(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if err := r.ParseMultipartForm(maxMultipartMemory); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	weight := common.ParseIntField(r.FormValue("weight"))
	kind := domain.KRKind(r.FormValue("kind"))
	if !common.ValidKRKind(kind) || weight < 0 || weight > 100 {
		h.renderGoalWithError(w, r, goalID, "Некорректный тип KR или вес")
		return
	}

	krID, err := h.deps.Store.CreateKeyResult(ctx, store.KeyResultInput{
		GoalID:      goalID,
		Title:       common.TrimmedFormValue(r, "title"),
		Description: common.TrimmedFormValue(r, "description"),
		Weight:      weight,
		Kind:        kind,
	})
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}

	if kind == domain.KRKindPercent {
		start := common.ParseFloatField(r.FormValue("percent_start"))
		target := common.ParseFloatField(r.FormValue("percent_target"))
		current := common.ParseFloatField(r.FormValue("percent_current"))
		if start == target {
			h.renderGoalWithError(w, r, goalID, "Start и Target не должны быть равны")
			return
		}
		if err := h.deps.Store.UpsertPercentMeta(ctx, store.PercentMetaInput{KeyResultID: krID, StartValue: start, TargetValue: target, CurrentValue: current}); err != nil {
			common.RenderError(w, h.deps.Logger, err)
			return
		}
	}

	if kind == domain.KRKindLinear {
		start := common.ParseFloatField(r.FormValue("linear_start"))
		target := common.ParseFloatField(r.FormValue("linear_target"))
		current := common.ParseFloatField(r.FormValue("linear_current"))
		if start == target {
			h.renderGoalWithError(w, r, goalID, "Start и Target не должны быть равны")
			return
		}
		if err := h.deps.Store.UpsertLinearMeta(ctx, store.LinearMetaInput{KeyResultID: krID, StartValue: start, TargetValue: target, CurrentValue: current}); err != nil {
			common.RenderError(w, h.deps.Logger, err)
			return
		}
	}

	if kind == domain.KRKindBoolean {
		done := r.FormValue("boolean_done") == "true"
		if err := h.deps.Store.UpsertBooleanMeta(ctx, krID, done); err != nil {
			common.RenderError(w, h.deps.Logger, err)
			return
		}
	}
	if kind == domain.KRKindProject {
		stages, err := parseProjectStages(r)
		if err != nil {
			h.renderGoalWithError(w, r, goalID, err.Error())
			return
		}
		for i := range stages {
			stages[i].KeyResultID = krID
		}
		if err := h.deps.Store.ReplaceProjectStages(ctx, krID, stages); err != nil {
			common.RenderError(w, h.deps.Logger, err)
			return
		}
	}

	if returnURL := r.FormValue("return"); returnURL != "" {
		http.Redirect(w, r, returnURL, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/goals/%d", goalID), http.StatusSeeOther)
}

func (h *Handler) HandleDeleteGoal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if err := r.ParseMultipartForm(maxMultipartMemory); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	goal, err := h.deps.Store.GetGoal(ctx, goalID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	teamID := parseOptionalTeamID(r.FormValue("team_id"), goal.TeamID)
	if teamID != goal.TeamID {
		if err := h.deps.Store.DeleteGoalShare(ctx, goalID, teamID); err != nil {
			common.RenderError(w, h.deps.Logger, err)
			return
		}
		redirectToTeam(w, r, teamID, goal.PeriodID)
		return
	}
	shares, err := h.deps.Store.ListGoalShares(ctx, goalID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if len(shares) > 0 {
		newOwner := shares[0]
		if err := h.deps.Store.UpdateGoalOwner(ctx, goalID, newOwner.TeamID, newOwner.Weight); err != nil {
			common.RenderError(w, h.deps.Logger, err)
			return
		}
		if err := h.deps.Store.DeleteGoalShare(ctx, goalID, newOwner.TeamID); err != nil {
			common.RenderError(w, h.deps.Logger, err)
			return
		}
		redirectToTeam(w, r, teamID, goal.PeriodID)
		return
	}
	status, err := h.deps.Store.GetTeamPeriodStatus(ctx, goal.TeamID, goal.PeriodID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if status == domain.TeamPeriodStatusClosed {
		h.renderGoalWithError(w, r, goalID, "Период закрыт, изменения недоступны")
		return
	}
	if err := h.deps.Store.DeleteGoal(ctx, goalID); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/teams/%d/okr?period_id=%d", goal.TeamID, goal.PeriodID), http.StatusSeeOther)
}

func (h *Handler) HandleUpdateGoal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if err := r.ParseMultipartForm(maxMultipartMemory); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	goal, err := h.deps.Store.GetGoal(ctx, goalID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	teamID := parseOptionalTeamID(r.FormValue("team_id"), goal.TeamID)
	status, err := h.deps.Store.GetTeamPeriodStatus(ctx, teamID, goal.PeriodID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if status == domain.TeamPeriodStatusClosed {
		h.renderGoalWithError(w, r, goalID, "Период закрыт, изменения недоступны")
		return
	}
	priority := domain.Priority(r.FormValue("priority"))
	workType := domain.WorkType(r.FormValue("work_type"))
	focusType := domain.FocusType(r.FormValue("focus_type"))
	weight := common.ParseIntField(r.FormValue("weight"))
	if errMsg := common.ValidateGoalInput(priority, workType, focusType, weight); errMsg != "" {
		h.renderGoalWithError(w, r, goalID, errMsg)
		return
	}
	if err := h.deps.Store.UpdateGoalFields(ctx, store.GoalFieldsUpdateInput{
		ID:          goalID,
		Title:       common.TrimmedFormValue(r, "title"),
		Description: common.TrimmedFormValue(r, "description"),
		Priority:    priority,
		WorkType:    workType,
		FocusType:   focusType,
		OwnerText:   common.TrimmedFormValue(r, "owner_text"),
	}); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if err := h.deps.Store.UpdateGoalTeamWeight(ctx, goalID, teamID, weight); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	redirectToTeam(w, r, teamID, goal.PeriodID)
}

func parseProjectStages(r *http.Request) ([]store.ProjectStageInput, error) {
	stages := make([]store.ProjectStageInput, 0, 4)
	titles := r.Form["step_title[]"]
	weights := r.Form["step_weight[]"]
	dones := r.Form["step_done[]"]
	sortOrder := 1
	for i, title := range titles {
		trimmed := strings.TrimSpace(title)
		if trimmed == "" {
			continue
		}
		weightValue := ""
		if i < len(weights) {
			weightValue = weights[i]
		}
		weight := common.ParseIntField(weightValue)
		if weight <= 0 || weight > 100 {
			return nil, fmt.Errorf("Вес шага должен быть 1..100")
		}
		isDone := false
		if i < len(dones) {
			isDone = dones[i] == "true"
		}
		stages = append(stages, store.ProjectStageInput{
			Title:     trimmed,
			Weight:    weight,
			IsDone:    isDone,
			SortOrder: sortOrder,
		})
		sortOrder++
	}
	if len(stages) == 0 {
		return nil, fmt.Errorf("Для Project KR требуется минимум один шаг")
	}
	return stages, nil
}

func (h *Handler) renderGoalWithError(w http.ResponseWriter, r *http.Request, goalID int64, message string) {
	goal, err := h.deps.Store.GetGoal(r.Context(), goalID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	teamID := parseOptionalTeamID(r.FormValue("team_id"), goal.TeamID)
	if teamID == goal.TeamID {
		if value := r.URL.Query().Get("team"); value != "" {
			teamID = parseOptionalTeamID(value, goal.TeamID)
		}
	}
	if teamID != goal.TeamID {
		if share, err := h.deps.Store.GetGoalShare(r.Context(), goalID, teamID); err == nil {
			goal.Weight = share.Weight
		}
	}
	team, err := h.deps.Store.GetTeam(r.Context(), teamID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	period, err := h.deps.Store.GetPeriod(r.Context(), goal.PeriodID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	status, err := h.deps.Store.GetTeamPeriodStatus(r.Context(), team.ID, goal.PeriodID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	goal.Progress = common.CalculateGoalProgress(goal)
	page := struct {
		Team            domain.Team
		TeamTypeLabel   string
		Goal            domain.Goal
		Period          domain.Period
		IsClosed        bool
		FormError       string
		PageTitle       string
		ContentTemplate string
	}{Team: team, TeamTypeLabel: common.TeamTypeLabel(team.Type), Goal: goal, Period: period, IsClosed: status == domain.TeamPeriodStatusClosed, FormError: message, PageTitle: "Цель", ContentTemplate: "goal-content"}
	common.RenderTemplate(w, h.deps.Templates, "base", page, h.deps.Logger)
}

func (h *Handler) HandleUpdateGoalShare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if err := r.ParseMultipartForm(maxMultipartMemory); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	goal, err := h.deps.Store.GetGoal(ctx, goalID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	selected := r.Form["team_ids"]
	if len(selected) == 0 {
		common.RenderError(w, h.deps.Logger, fmt.Errorf("нужно выбрать хотя бы одну команду"))
		return
	}
	selectedIDs := make([]int64, 0, len(selected))
	selectedSet := make(map[int64]struct{}, len(selected))
	for _, value := range selected {
		teamID, err := common.ParseID(value)
		if err != nil {
			continue
		}
		if _, exists := selectedSet[teamID]; exists {
			continue
		}
		selectedSet[teamID] = struct{}{}
		selectedIDs = append(selectedIDs, teamID)
	}
	sharesList, err := h.deps.Store.ListGoalShares(ctx, goalID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	shareWeights := make(map[int64]int, len(sharesList))
	for _, share := range sharesList {
		shareWeights[share.TeamID] = share.Weight
	}
	ownerID := goal.TeamID
	if _, ok := selectedSet[ownerID]; !ok {
		ownerID = selectedIDs[0]
	}
	shares := make([]store.GoalShareInput, 0, len(selectedIDs))
	for _, teamID := range selectedIDs {
		status, err := h.deps.Store.GetTeamPeriodStatus(ctx, teamID, goal.PeriodID)
		if err != nil {
			common.RenderError(w, h.deps.Logger, err)
			return
		}
		if status == domain.TeamPeriodStatusValidated || status == domain.TeamPeriodStatusClosed {
			common.RenderError(w, h.deps.Logger, fmt.Errorf("Нельзя шарить цель с закрытым периодом"))
			return
		}
		if teamID == ownerID {
			ownerWeight := goal.Weight
			if ownerID != goal.TeamID {
				if existingWeight, ok := shareWeights[ownerID]; ok {
					ownerWeight = existingWeight
				} else {
					ownerWeight = 0
				}
			}
			if err := h.deps.Store.UpdateGoalOwner(ctx, goalID, ownerID, ownerWeight); err != nil {
				common.RenderError(w, h.deps.Logger, err)
				return
			}
			continue
		}
		weight := 0
		if existingWeight, ok := shareWeights[teamID]; ok {
			weight = existingWeight
		}
		shares = append(shares, store.GoalShareInput{TeamID: teamID, Weight: weight})
	}
	if err := h.deps.Store.ReplaceGoalShares(ctx, goalID, shares); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if returnURL := r.FormValue("return"); returnURL != "" {
		http.Redirect(w, r, returnURL, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/teams/%d/okr?period_id=%d", ownerID, goal.PeriodID), http.StatusSeeOther)
}

func parseOptionalTeamID(value string, fallback int64) int64 {
	if value == "" {
		return fallback
	}
	parsed, err := common.ParseID(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func redirectToTeam(w http.ResponseWriter, r *http.Request, teamID, periodID int64) {
	http.Redirect(w, r, fmt.Sprintf("/teams/%d/okr?period_id=%d", teamID, periodID), http.StatusSeeOther)
}
