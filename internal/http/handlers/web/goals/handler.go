package goals

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"okrs/internal/auth"
	"okrs/internal/domain"
	"okrs/internal/http/handlers/web/common"
	"okrs/internal/service"
	"okrs/internal/store/goals"
	"okrs/internal/store/krs"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	deps common.Dependencies
}

const maxMultipartMemory = 32 << 20

func New(deps common.Dependencies) *Handler {
	return &Handler{deps: deps}
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
		http.Redirect(w, r, "/teamOkrs", http.StatusSeeOther)
		return
	}
	if err := h.deps.Service.AddGoalComment(ctx, goalID, text, auth.UserIDFromContext(ctx)); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if returnURL := r.FormValue("return"); returnURL != "" {
		http.Redirect(w, r, returnURL, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/teamOkrs", http.StatusSeeOther)
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
		common.RenderError(w, h.deps.Logger, fmt.Errorf("Некорректный тип KR или вес"))
		return
	}

	meta := service.KeyResultMetaInput{}

	switch kind {
	case domain.KRKindPercent:
		start := common.ParseFloatField(r.FormValue("percent_start"))
		target := common.ParseFloatField(r.FormValue("percent_target"))
		if start == target {
			common.RenderError(w, h.deps.Logger, fmt.Errorf("Start и Target не должны быть равны"))
			return
		}
		meta.PercentStart = start
		meta.PercentTarget = target
		meta.PercentCurrent = common.ParseFloatField(r.FormValue("percent_current"))
	case domain.KRKindLinear:
		start := common.ParseFloatField(r.FormValue("linear_start"))
		target := common.ParseFloatField(r.FormValue("linear_target"))
		if start == target {
			common.RenderError(w, h.deps.Logger, fmt.Errorf("Start и Target не должны быть равны"))
			return
		}
		meta.LinearStart = start
		meta.LinearTarget = target
		meta.LinearCurrent = common.ParseFloatField(r.FormValue("linear_current"))
	case domain.KRKindBoolean:
		meta.BooleanDone = r.FormValue("boolean_done") == "true"
	case domain.KRKindProject:
		stages, err := parseProjectStages(r)
		if err != nil {
			common.RenderError(w, h.deps.Logger, err)
			return
		}
		meta.ProjectStages = stages
	}

	if _, err := h.deps.Service.CreateKeyResultWithMeta(ctx, krs.KeyResultInput{
		GoalID:      goalID,
		Title:       common.TrimmedFormValue(r, "title"),
		Description: common.TrimmedFormValue(r, "description"),
		Weight:      weight,
		Kind:        kind,
	}, meta); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}

	if returnURL := r.FormValue("return"); returnURL != "" {
		http.Redirect(w, r, returnURL, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/teamOkrs", http.StatusSeeOther)
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
	var requestingTeamID int64
	if s := r.FormValue("team_id"); s != "" {
		requestingTeamID, _ = common.ParseID(s)
	}
	teamID, periodID, err := h.deps.Service.DeleteGoal(ctx, goalID, requestingTeamID)
	if err != nil {
		if errors.Is(err, service.ErrPeriodClosed) {
			common.RenderError(w, h.deps.Logger, fmt.Errorf("Период закрыт, изменения недоступны"))
			return
		}
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	redirectToTeam(w, r, teamID, periodID)
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
	goal, err := h.deps.Service.GetGoal(ctx, goalID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	teamID := parseOptionalTeamID(r.FormValue("team_id"), goal.TeamID)
	status, err := h.deps.Service.GetTeamPeriodStatus(ctx, teamID, goal.PeriodID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if status == domain.TeamPeriodStatusClosed {
		common.RenderError(w, h.deps.Logger, fmt.Errorf("Период закрыт, изменения недоступны"))
		return
	}
	priority := domain.Priority(r.FormValue("priority"))
	workType := domain.WorkType(r.FormValue("work_type"))
	focusType := domain.FocusType(r.FormValue("focus_type"))
	weight := common.ParseIntField(r.FormValue("weight"))
	if errMsg := common.ValidateGoalInput(priority, workType, focusType, weight); errMsg != "" {
		common.RenderError(w, h.deps.Logger, fmt.Errorf("%s", errMsg))
		return
	}
	if err := h.deps.Service.UpdateGoalFields(ctx, goals.GoalFieldsUpdateInput{
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
	if err := h.deps.Service.UpdateGoalWeight(ctx, goalID, teamID, weight); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	redirectToTeam(w, r, teamID, goal.PeriodID)
}

func parseProjectStages(r *http.Request) ([]krs.ProjectStageInput, error) {
	stages := make([]krs.ProjectStageInput, 0, 4)
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
		stages = append(stages, krs.ProjectStageInput{
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
	selected := r.Form["team_ids"]
	if len(selected) == 0 {
		common.RenderError(w, h.deps.Logger, fmt.Errorf("нужно выбрать хотя бы одну команду"))
		return
	}
	selectedIDs := make([]int64, 0, len(selected))
	seen := make(map[int64]struct{}, len(selected))
	for _, value := range selected {
		teamID, err := common.ParseID(value)
		if err != nil {
			continue
		}
		if _, exists := seen[teamID]; exists {
			continue
		}
		seen[teamID] = struct{}{}
		selectedIDs = append(selectedIDs, teamID)
	}
	if len(selectedIDs) == 0 {
		common.RenderError(w, h.deps.Logger, fmt.Errorf("нужно выбрать хотя бы одну команду"))
		return
	}
	ownerID, periodID, err := h.deps.Service.UpdateGoalOwnerAndShares(ctx, goalID, selectedIDs)
	if err != nil {
		if errors.Is(err, service.ErrCannotShareWithClosedPeriod) {
			common.RenderError(w, h.deps.Logger, fmt.Errorf("Нельзя шарить цель с закрытым периодом"))
			return
		}
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if returnURL := r.FormValue("return"); returnURL != "" {
		http.Redirect(w, r, returnURL, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/teams/%d/okr?period_id=%d", ownerID, periodID), http.StatusSeeOther)
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
