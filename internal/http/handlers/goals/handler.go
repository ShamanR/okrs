package goals

import (
	"fmt"
	"net/http"
	"sort"
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

func (h *Handler) HandleUpdateKeyResultWeights(w http.ResponseWriter, r *http.Request) {
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
	for _, kr := range goal.KeyResults {
		field := fmt.Sprintf("kr_weight_%d", kr.ID)
		weight := common.ParseIntField(r.FormValue(field))
		if weight < 0 || weight > 100 {
			common.RenderError(w, h.deps.Logger, fmt.Errorf("Вес KR должен быть 0..100"))
			return
		}
		if err := h.deps.Store.UpdateKeyResultWeight(ctx, kr.ID, weight); err != nil {
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
	if err := r.ParseForm(); err != nil {
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
		redirectToTeam(w, r, teamID, goal.Year, goal.Quarter)
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
		redirectToTeam(w, r, teamID, goal.Year, goal.Quarter)
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
	teamID := parseOptionalTeamID(r.FormValue("team_id"), goal.TeamID)
	if teamID != goal.TeamID {
		if returnURL := r.FormValue("return"); returnURL != "" {
			http.Redirect(w, r, returnURL, http.StatusSeeOther)
			return
		}
		redirectToTeam(w, r, teamID, goal.Year, goal.Quarter)
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
	teamID := parseOptionalTeamID(r.FormValue("team_id"), goal.TeamID)
	status, err := h.deps.Store.GetTeamQuarterStatus(ctx, teamID, goal.Year, goal.Quarter)
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
	redirectToTeam(w, r, teamID, goal.Year, goal.Quarter)
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

type goalShareOption struct {
	ID       int64
	Label    string
	Selected bool
	Weight   int
}

type goalSharePage struct {
	Goal            domain.Goal
	Owner           domain.Team
	OwnerTypeLabel  string
	Options         []goalShareOption
	ReturnURL       string
	PageTitle       string
	ContentTemplate string
}

func (h *Handler) HandleShareGoal(w http.ResponseWriter, r *http.Request) {
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
	owner, err := h.deps.Store.GetTeam(ctx, goal.TeamID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	teams, err := h.deps.Store.ListTeams(ctx)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	shares, err := h.deps.Store.ListGoalShares(ctx, goalID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	sharesByTeam := make(map[int64]store.GoalShare, len(shares))
	for _, share := range shares {
		sharesByTeam[share.TeamID] = share
	}
	_, childrenMap, rootTeams := buildTeamHierarchy(teams)
	options := buildGoalShareOptions(rootTeams, childrenMap, sharesByTeam, goal.TeamID, goal.Weight)
	returnURL := r.URL.Query().Get("return")
	page := goalSharePage{
		Goal:            goal,
		Owner:           owner,
		OwnerTypeLabel:  common.TeamTypeLabel(owner.Type),
		Options:         options,
		ReturnURL:       returnURL,
		PageTitle:       "Шаринг цели",
		ContentTemplate: "goal-share-content",
	}
	common.RenderTemplate(w, h.deps.Templates, "base", page, h.deps.Logger)
}

func (h *Handler) HandleUpdateGoalShare(w http.ResponseWriter, r *http.Request) {
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
	ownerID := goal.TeamID
	if _, ok := selectedSet[ownerID]; !ok {
		ownerID = selectedIDs[0]
	}
	shares := make([]store.GoalShareInput, 0, len(selectedIDs))
	for _, teamID := range selectedIDs {
		weight := common.ParseIntField(r.FormValue(fmt.Sprintf("weight_%d", teamID)))
		if weight < 0 || weight > 100 {
			common.RenderError(w, h.deps.Logger, fmt.Errorf("Вес должен быть 0..100"))
			return
		}
		if teamID == ownerID {
			if err := h.deps.Store.UpdateGoalOwner(ctx, goalID, ownerID, weight); err != nil {
				common.RenderError(w, h.deps.Logger, err)
				return
			}
			continue
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
	http.Redirect(w, r, fmt.Sprintf("/teams/%d/okr?year=%d&quarter=%d", ownerID, goal.Year, goal.Quarter), http.StatusSeeOther)
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

func redirectToTeam(w http.ResponseWriter, r *http.Request, teamID int64, year, quarter int) {
	http.Redirect(w, r, fmt.Sprintf("/teams/%d/okr?year=%d&quarter=%d", teamID, year, quarter), http.StatusSeeOther)
}

func buildTeamHierarchy(teams []domain.Team) (map[int64]domain.Team, map[int64][]domain.Team, []domain.Team) {
	teamsByID := make(map[int64]domain.Team, len(teams))
	childrenMap := make(map[int64][]domain.Team)
	for _, team := range teams {
		teamsByID[team.ID] = team
		parentKey := int64(0)
		if team.ParentID != nil {
			parentKey = *team.ParentID
		}
		childrenMap[parentKey] = append(childrenMap[parentKey], team)
	}
	for key := range childrenMap {
		sort.Slice(childrenMap[key], func(i, j int) bool {
			return childrenMap[key][i].Name < childrenMap[key][j].Name
		})
	}
	rootTeams := childrenMap[0]
	return teamsByID, childrenMap, rootTeams
}

func buildGoalShareOptions(rootTeams []domain.Team, childrenMap map[int64][]domain.Team, shares map[int64]store.GoalShare, ownerID int64, goalWeight int) []goalShareOption {
	options := make([]goalShareOption, 0, len(rootTeams))
	for _, team := range rootTeams {
		appendGoalShareOption(&options, team, childrenMap, shares, ownerID, goalWeight, 0)
	}
	return options
}

func appendGoalShareOption(options *[]goalShareOption, team domain.Team, childrenMap map[int64][]domain.Team, shares map[int64]store.GoalShare, ownerID int64, goalWeight int, level int) {
	label := fmt.Sprintf("%s%s %s", teamHierarchyPrefix(level), common.TeamTypeLabel(team.Type), team.Name)
	share, ok := shares[team.ID]
	weight := goalWeight
	if ok {
		weight = share.Weight
	}
	if team.ID == ownerID {
		weight = goalWeight
	}
	option := goalShareOption{
		ID:       team.ID,
		Label:    label,
		Selected: ok || team.ID == ownerID,
		Weight:   weight,
	}
	*options = append(*options, option)
	for _, child := range childrenMap[team.ID] {
		appendGoalShareOption(options, child, childrenMap, shares, ownerID, goalWeight, level+1)
	}
}

func teamHierarchyPrefix(level int) string {
	if level == 0 {
		return ""
	}
	return fmt.Sprintf("%s|-- ", strings.Repeat("  ", level-1))
}
