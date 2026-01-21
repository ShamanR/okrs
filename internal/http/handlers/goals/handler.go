package goals

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"okrs/internal/domain"
	"okrs/internal/http/handlers/common"
	"okrs/internal/store"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	deps common.Dependencies
}

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
	team, err := h.deps.Store.GetTeam(ctx, goal.TeamID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	status, err := h.deps.Store.GetTeamQuarterStatus(ctx, team.ID, goal.Year, goal.Quarter)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}

	goal.Progress = common.CalculateGoalProgress(goal)

	page := struct {
		Team            domain.Team
		TeamTypeLabel   string
		Goal            domain.Goal
		IsClosed        bool
		FormError       string
		PageTitle       string
		ContentTemplate string
	}{Team: team, TeamTypeLabel: common.TeamTypeLabel(team.Type), Goal: goal, IsClosed: status == domain.TeamQuarterStatusClosed, PageTitle: "Цель", ContentTemplate: "goal-content"}
	common.RenderTemplate(w, h.deps.Templates, "base", page, h.deps.Logger)
}

func (h *Handler) HandleAddGoalComment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	text := common.TrimmedFormValue(r, "text")
	if text == "" {
		http.Redirect(w, r, fmt.Sprintf("/goals/%d", goalID), http.StatusSeeOther)
		return
	}
	if err := h.deps.Store.AddGoalComment(ctx, goalID, text); err != nil {
		common.RenderError(w, h.deps.Logger, err)
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
	if err := r.ParseForm(); err != nil {
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

	http.Redirect(w, r, fmt.Sprintf("/goals/%d", goalID), http.StatusSeeOther)
}

func (h *Handler) HandleDeleteGoal(w http.ResponseWriter, r *http.Request) {
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
	status, err := h.deps.Store.GetTeamQuarterStatus(ctx, goal.TeamID, goal.Year, goal.Quarter)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if status == domain.TeamQuarterStatusClosed {
		h.renderGoalWithError(w, r, goalID, "Квартал закрыт, изменения недоступны")
		return
	}
	if err := h.deps.Store.DeleteGoal(ctx, goalID); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/teams/%d/okr?year=%d&quarter=%d", goal.TeamID, goal.Year, goal.Quarter), http.StatusSeeOther)
}

func (h *Handler) HandleUpdateGoal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	goal, err := h.deps.Store.GetGoal(ctx, goalID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	status, err := h.deps.Store.GetTeamQuarterStatus(ctx, goal.TeamID, goal.Year, goal.Quarter)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if status == domain.TeamQuarterStatusClosed {
		h.renderGoalWithError(w, r, goalID, "Квартал закрыт, изменения недоступны")
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
	if err := h.deps.Store.UpdateGoal(ctx, store.GoalUpdateInput{
		ID:          goalID,
		Title:       common.TrimmedFormValue(r, "title"),
		Description: common.TrimmedFormValue(r, "description"),
		Priority:    priority,
		Weight:      weight,
		WorkType:    workType,
		FocusType:   focusType,
		OwnerText:   common.TrimmedFormValue(r, "owner_text"),
	}); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/teams/%d/okr?year=%d&quarter=%d", goal.TeamID, goal.Year, goal.Quarter), http.StatusSeeOther)
}

type yearGoalsPage struct {
	Year            int
	Goals           []yearGoalRow
	YearValues      []int
	PageTitle       string
	ContentTemplate string
}

type yearGoalRow struct {
	Goal          domain.Goal
	TeamName      string
	TeamTypeLabel string
}

func (h *Handler) HandleYearGoals(w http.ResponseWriter, r *http.Request) {
	year := common.ParseIntField(r.URL.Query().Get("year"))
	if year == 0 {
		current := store.CurrentQuarter(time.Now().In(h.deps.Zone))
		year = current.Year
	}
	goals, err := h.deps.Store.ListGoalsByYear(r.Context(), year)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	rows := make([]yearGoalRow, 0, len(goals))
	for _, goal := range goals {
		rows = append(rows, yearGoalRow{
			Goal:          goal.Goal,
			TeamName:      goal.TeamName,
			TeamTypeLabel: common.TeamTypeLabel(goal.TeamType),
		})
	}
	values := buildYearOptions(year)
	page := yearGoalsPage{
		Year:            year,
		Goals:           rows,
		YearValues:      values,
		PageTitle:       "Цели за год",
		ContentTemplate: "year-goals-content",
	}
	common.RenderTemplate(w, h.deps.Templates, "base", page, h.deps.Logger)
}

func buildYearOptions(selected int) []int {
	values := make([]int, 0, 7)
	start := selected - 3
	for i := 0; i < 7; i++ {
		values = append(values, start+i)
	}
	return values
}

func parseProjectStages(r *http.Request) ([]store.ProjectStageInput, error) {
	stages := make([]store.ProjectStageInput, 0, 4)
	totalWeight := 0
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
		totalWeight += weight
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
	if totalWeight != 100 {
		return nil, fmt.Errorf("Сумма весов шагов должна быть равна 100")
	}
	return stages, nil
}

func (h *Handler) renderGoalWithError(w http.ResponseWriter, r *http.Request, goalID int64, message string) {
	goal, err := h.deps.Store.GetGoal(r.Context(), goalID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	team, err := h.deps.Store.GetTeam(r.Context(), goal.TeamID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	status, err := h.deps.Store.GetTeamQuarterStatus(r.Context(), team.ID, goal.Year, goal.Quarter)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	goal.Progress = common.CalculateGoalProgress(goal)
	page := struct {
		Team            domain.Team
		TeamTypeLabel   string
		Goal            domain.Goal
		IsClosed        bool
		FormError       string
		PageTitle       string
		ContentTemplate string
	}{Team: team, TeamTypeLabel: common.TeamTypeLabel(team.Type), Goal: goal, IsClosed: status == domain.TeamQuarterStatusClosed, FormError: message, PageTitle: "Цель", ContentTemplate: "goal-content"}
	common.RenderTemplate(w, h.deps.Templates, "base", page, h.deps.Logger)
}
