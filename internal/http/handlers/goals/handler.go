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
	quarterStatus, err := h.deps.Store.GetTeamQuarterStatus(ctx, team.ID, goal.Year, goal.Quarter)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}

	goal.Progress = common.CalculateGoalProgress(goal)
	objectiveStatus := common.ComputeObjectiveStatus(goal, time.Now().In(h.deps.Zone), h.deps.Zone)
	krBreakdown := buildKRBreakdown(goal.KeyResults)
	goalComments := buildGoalCommentViews(goal.Comments)
	lastStatusUpdate := findLatestStatusUpdate(goalComments)
	useObjectiveUIV2 := common.FeatureEnabled("okr_objective_ui_v2")

	page := struct {
		Team              domain.Team
		TeamTypeLabel     string
		Goal              domain.Goal
		IsClosed          bool
		FormError         string
		PageTitle         string
		ContentTemplate   string
		UseObjectiveUIV2  bool
		ObjectiveStatus   common.ObjectiveStatus
		KRBreakdown       []krBreakdownRow
		GoalComments      []goalCommentView
		LastStatusUpdate  *goalCommentView
		KRWeightsMismatch bool
		KRWeightsSum      int
	}{
		Team:              team,
		TeamTypeLabel:     common.TeamTypeLabel(team.Type),
		Goal:              goal,
		IsClosed:          quarterStatus == domain.TeamQuarterStatusClosed,
		PageTitle:         "Цель",
		ContentTemplate:   "goal-content",
		UseObjectiveUIV2:  useObjectiveUIV2,
		ObjectiveStatus:   objectiveStatus,
		KRBreakdown:       krBreakdown,
		GoalComments:      goalComments,
		LastStatusUpdate:  lastStatusUpdate,
		KRWeightsMismatch: sumKRWeights(goal.KeyResults) != 100,
		KRWeightsSum:      sumKRWeights(goal.KeyResults),
	}
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
	commentType := r.FormValue("comment_type")
	text := common.TrimmedFormValue(r, "text")
	if commentType == "status_update" {
		text = buildStatusUpdateText(r)
	}
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

func (h *Handler) HandleUpdateKRWeights(w http.ResponseWriter, r *http.Request) {
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

	updates := make([]store.KeyResultUpdateInput, 0, len(goal.KeyResults))
	total := 0
	for _, kr := range goal.KeyResults {
		fieldName := fmt.Sprintf("weight_%d", kr.ID)
		weight := common.ParseIntField(r.FormValue(fieldName))
		if weight < 0 || weight > 100 {
			h.renderGoalWithError(w, r, goalID, "Вес KR должен быть 0..100")
			return
		}
		total += weight
		updates = append(updates, store.KeyResultUpdateInput{
			ID:          kr.ID,
			Title:       kr.Title,
			Description: kr.Description,
			Weight:      weight,
			Kind:        kr.Kind,
		})
	}
	if total != 100 {
		h.renderGoalWithError(w, r, goalID, "Сумма весов KR должна быть равна 100")
		return
	}
	for _, update := range updates {
		if err := h.deps.Store.UpdateKeyResult(ctx, update); err != nil {
			common.RenderError(w, h.deps.Logger, err)
			return
		}
	}
	http.Redirect(w, r, fmt.Sprintf("/goals/%d", goalID), http.StatusSeeOther)
}

type krBreakdownRow struct {
	Title        string
	Weight       int
	Progress     int
	Contribution int
}

type goalCommentView struct {
	ID             int64
	Text           string
	CreatedAt      time.Time
	IsStatusUpdate bool
	UpdateFields   statusUpdateFields
}

type statusUpdateFields struct {
	Changed  string
	Blockers string
	NextStep string
	Help     string
}

func buildKRBreakdown(keyResults []domain.KeyResult) []krBreakdownRow {
	rows := make([]krBreakdownRow, 0, len(keyResults))
	for _, kr := range keyResults {
		rows = append(rows, krBreakdownRow{
			Title:        kr.Title,
			Weight:       kr.Weight,
			Progress:     kr.Progress,
			Contribution: int(float64(kr.Weight*kr.Progress) / 100.0),
		})
	}
	return rows
}

func sumKRWeights(keyResults []domain.KeyResult) int {
	total := 0
	for _, kr := range keyResults {
		total += kr.Weight
	}
	return total
}

func buildGoalCommentViews(comments []domain.GoalComment) []goalCommentView {
	views := make([]goalCommentView, 0, len(comments))
	for _, comment := range comments {
		update, ok := parseStatusUpdate(comment.Text)
		views = append(views, goalCommentView{
			ID:             comment.ID,
			Text:           comment.Text,
			CreatedAt:      comment.CreatedAt,
			IsStatusUpdate: ok,
			UpdateFields:   update,
		})
	}
	return views
}

func findLatestStatusUpdate(comments []goalCommentView) *goalCommentView {
	for i := len(comments) - 1; i >= 0; i-- {
		if comments[i].IsStatusUpdate {
			return &comments[i]
		}
	}
	return nil
}

func buildStatusUpdateText(r *http.Request) string {
	changed := common.TrimmedFormValue(r, "status_changed")
	blockers := common.TrimmedFormValue(r, "status_blockers")
	nextStep := common.TrimmedFormValue(r, "status_next")
	help := common.TrimmedFormValue(r, "status_help")
	if changed == "" && blockers == "" && nextStep == "" && help == "" {
		return ""
	}
	lines := []string{
		"[status_update]",
		fmt.Sprintf("changed: %s", changed),
		fmt.Sprintf("blockers: %s", blockers),
		fmt.Sprintf("next: %s", nextStep),
		fmt.Sprintf("help: %s", help),
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func parseStatusUpdate(text string) (statusUpdateFields, bool) {
	if !strings.HasPrefix(text, "[status_update]") {
		return statusUpdateFields{}, false
	}
	lines := strings.Split(text, "\n")
	fields := statusUpdateFields{}
	for _, line := range lines[1:] {
		switch {
		case strings.HasPrefix(line, "changed:"):
			fields.Changed = strings.TrimSpace(strings.TrimPrefix(line, "changed:"))
		case strings.HasPrefix(line, "blockers:"):
			fields.Blockers = strings.TrimSpace(strings.TrimPrefix(line, "blockers:"))
		case strings.HasPrefix(line, "next:"):
			fields.NextStep = strings.TrimSpace(strings.TrimPrefix(line, "next:"))
		case strings.HasPrefix(line, "help:"):
			fields.Help = strings.TrimSpace(strings.TrimPrefix(line, "help:"))
		}
	}
	return fields, true
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

func (h *Handler) HandleMoveGoalUp(w http.ResponseWriter, r *http.Request) {
	h.handleMoveGoal(w, r, -1)
}

func (h *Handler) HandleMoveGoalDown(w http.ResponseWriter, r *http.Request) {
	h.handleMoveGoal(w, r, 1)
}

func (h *Handler) handleMoveGoal(w http.ResponseWriter, r *http.Request, direction int) {
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
	if err := h.deps.Store.MoveGoal(ctx, goalID, direction); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if returnURL := r.FormValue("return"); returnURL != "" {
		http.Redirect(w, r, returnURL, http.StatusSeeOther)
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
	objectiveStatus := common.ComputeObjectiveStatus(goal, time.Now().In(h.deps.Zone), h.deps.Zone)
	krBreakdown := buildKRBreakdown(goal.KeyResults)
	goalComments := buildGoalCommentViews(goal.Comments)
	lastStatusUpdate := findLatestStatusUpdate(goalComments)
	useObjectiveUIV2 := common.FeatureEnabled("okr_objective_ui_v2")
	page := struct {
		Team              domain.Team
		TeamTypeLabel     string
		Goal              domain.Goal
		IsClosed          bool
		FormError         string
		PageTitle         string
		ContentTemplate   string
		UseObjectiveUIV2  bool
		ObjectiveStatus   common.ObjectiveStatus
		KRBreakdown       []krBreakdownRow
		GoalComments      []goalCommentView
		LastStatusUpdate  *goalCommentView
		KRWeightsMismatch bool
		KRWeightsSum      int
	}{
		Team:              team,
		TeamTypeLabel:     common.TeamTypeLabel(team.Type),
		Goal:              goal,
		IsClosed:          status == domain.TeamQuarterStatusClosed,
		FormError:         message,
		PageTitle:         "Цель",
		ContentTemplate:   "goal-content",
		UseObjectiveUIV2:  useObjectiveUIV2,
		ObjectiveStatus:   objectiveStatus,
		KRBreakdown:       krBreakdown,
		GoalComments:      goalComments,
		LastStatusUpdate:  lastStatusUpdate,
		KRWeightsMismatch: sumKRWeights(goal.KeyResults) != 100,
		KRWeightsSum:      sumKRWeights(goal.KeyResults),
	}
	common.RenderTemplate(w, h.deps.Templates, "base", page, h.deps.Logger)
}
