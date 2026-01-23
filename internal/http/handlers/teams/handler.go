package teams

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"okrs/internal/domain"
	"okrs/internal/http/handlers/common"
	"okrs/internal/okr"
	"okrs/internal/store"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	deps common.Dependencies
}

func New(deps common.Dependencies) *Handler {
	return &Handler{deps: deps}
}

type teamRow struct {
	ID              int64
	Name            string
	TypeLabel       string
	Indent          int
	StatusLabel     string
	QuarterProgress int
	GoalsCount      int
	Goals           []domain.Goal
	GoalsWeight     int
}

type teamFilterOption struct {
	Value    string
	Label    string
	Selected bool
}

type teamsPage struct {
	QuarterOptions  []common.QuarterOption
	SelectedYear    int
	SelectedQuarter int
	Teams           []teamRow
	TeamFilters     []teamFilterOption
	CurrentYear     int
	PageTitle       string
	ContentTemplate string
}

type teamTypeOption struct {
	Value    string
	Label    string
	Selected bool
}

type teamParentOption struct {
	Value    string
	Label    string
	Selected bool
}

type teamFormPage struct {
	FormError       string
	PageTitle       string
	ContentTemplate string
	BreadcrumbTitle string
	Title           string
	FormAction      string
	SubmitLabel     string
	TeamName        string
	TeamTypes       []teamTypeOption
	ParentTeams     []teamParentOption
}

type teamStatusOption struct {
	Value    string
	Label    string
	Selected bool
}

type teamOKRPage struct {
	Team             domain.Team
	TeamTypeLabel    string
	Year             int
	Quarter          int
	Goals            []domain.Goal
	QuarterStatus    domain.TeamQuarterStatus
	StatusOptions    []teamStatusOption
	IsClosed         bool
	QuarterProgress  int
	GoalsCount       int
	GoalsWeight      int
	FormError        string
	ObjectiveBlockV2 bool
	PageTitle        string
	ContentTemplate  string
}

func (h *Handler) HandleTeams(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	year, quarter, quarterValue := resolveQuarterFilter(r, h.deps.Zone)
	options := common.BuildQuarterOptions(year, quarter, h.deps.Zone)
	selectedFilter := resolveTeamFilter(r)
	var selectedTeamID *int64
	if selectedFilter != "ALL" {
		id, err := strconv.ParseInt(selectedFilter, 10, 64)
		if err == nil {
			selectedTeamID = &id
		}
	}

	teams, err := h.deps.Store.ListTeams(ctx)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}

	teamsByID, childrenMap, rootTeams := buildTeamHierarchy(teams)
	if selectedTeamID != nil {
		if _, ok := teamsByID[*selectedTeamID]; !ok {
			selectedTeamID = nil
			selectedFilter = "ALL"
		}
	}
	filterOptions := buildTeamFilterOptions(rootTeams, selectedFilter)
	filteredRoots := rootTeams
	if selectedTeamID != nil {
		if team, ok := teamsByID[*selectedTeamID]; ok {
			filteredRoots = []domain.Team{team}
		}
	}

	rows := make([]teamRow, 0, len(teams))
	for _, team := range filteredRoots {
		if err := h.appendTeamRows(ctx, &rows, team, 0, year, quarter, childrenMap); err != nil {
			common.RenderError(w, h.deps.Logger, err)
			return
		}
	}

	page := teamsPage{
		QuarterOptions:  options,
		SelectedYear:    year,
		SelectedQuarter: quarter,
		Teams:           rows,
		TeamFilters:     filterOptions,
		CurrentYear:     year,
		PageTitle:       "Команды",
		ContentTemplate: "teams-content",
	}
	persistTeamsFilters(w, quarterValue, selectedFilter)
	common.RenderTemplate(w, h.deps.Templates, "base", page, h.deps.Logger)
}

func resolveQuarterFilter(r *http.Request, zone *time.Location) (int, int, string) {
	quarterValue := r.URL.Query().Get("quarter")
	if quarterValue != "" {
		if year, quarter, ok := parseQuarterValue(quarterValue); ok {
			return year, quarter, quarterValue
		}
	}
	if cookie, err := r.Cookie("teams_quarter"); err == nil {
		if year, quarter, ok := parseQuarterValue(cookie.Value); ok {
			return year, quarter, cookie.Value
		}
	}
	year, quarter := common.ParseQuarter(r, zone)
	return year, quarter, fmt.Sprintf("%d-%d", year, quarter)
}

func resolveTeamFilter(r *http.Request) string {
	selectedFilter := r.URL.Query().Get("team")
	if selectedFilter != "" {
		return selectedFilter
	}
	if cookie, err := r.Cookie("teams_filter"); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	return "ALL"
}

func parseQuarterValue(value string) (int, int, bool) {
	parts := strings.Split(value, "-")
	if len(parts) != 2 {
		return 0, 0, false
	}
	year, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	quarter, err := strconv.Atoi(parts[1])
	if err != nil || quarter < 1 || quarter > 4 {
		return 0, 0, false
	}
	return year, quarter, true
}

func persistTeamsFilters(w http.ResponseWriter, quarterValue, selectedFilter string) {
	http.SetCookie(w, &http.Cookie{
		Name:   "teams_quarter",
		Value:  quarterValue,
		Path:   "/",
		MaxAge: 60 * 60 * 24 * 180,
	})
	http.SetCookie(w, &http.Cookie{
		Name:   "teams_filter",
		Value:  selectedFilter,
		Path:   "/",
		MaxAge: 60 * 60 * 24 * 180,
	})
}

func (h *Handler) HandleCreateTeam(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	name := common.TrimmedFormValue(r, "name")
	teamType := domain.TeamType(r.FormValue("team_type"))
	parentID, err := parseOptionalID(r.FormValue("parent_id"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	teams, err := h.deps.Store.ListTeams(ctx)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}

	if name == "" {
		h.renderTeamForm(w, r, teamFormValues{
			Message:  "Название команды обязательно",
			Name:     name,
			Type:     teamType,
			ParentID: parentID,
		}, teams, 0, false)
		return
	}
	if !common.ValidTeamType(teamType) {
		h.renderTeamForm(w, r, teamFormValues{
			Message:  "Неверный тип команды",
			Name:     name,
			Type:     teamType,
			ParentID: parentID,
		}, teams, 0, false)
		return
	}
	if parentID != nil && !teamExists(teams, *parentID) {
		h.renderTeamForm(w, r, teamFormValues{
			Message:  "Выбранная родительская команда не найдена",
			Name:     name,
			Type:     teamType,
			ParentID: parentID,
		}, teams, 0, false)
		return
	}
	if _, err := h.deps.Store.CreateTeam(ctx, store.TeamInput{Name: name, Type: teamType, ParentID: parentID}); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	http.Redirect(w, r, "/teams", http.StatusSeeOther)
}

func (h *Handler) HandleNewTeam(w http.ResponseWriter, r *http.Request) {
	teams, err := h.deps.Store.ListTeams(r.Context())
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	h.renderTeamForm(w, r, teamFormValues{
		Name:     "",
		Type:     domain.TeamTypeTeam,
		ParentID: nil,
	}, teams, 0, false)
}

func (h *Handler) HandleEditTeam(w http.ResponseWriter, r *http.Request) {
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	team, err := h.deps.Store.GetTeam(r.Context(), teamID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	teams, err := h.deps.Store.ListTeams(r.Context())
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	h.renderTeamForm(w, r, teamFormValues{
		Name:     team.Name,
		Type:     team.Type,
		ParentID: team.ParentID,
	}, teams, teamID, true)
}

func (h *Handler) HandleUpdateTeam(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	name := common.TrimmedFormValue(r, "name")
	teamType := domain.TeamType(r.FormValue("team_type"))
	parentID, err := parseOptionalID(r.FormValue("parent_id"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	teams, err := h.deps.Store.ListTeams(ctx)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}

	if name == "" {
		h.renderTeamForm(w, r, teamFormValues{
			Message:  "Название команды обязательно",
			Name:     name,
			Type:     teamType,
			ParentID: parentID,
		}, teams, teamID, true)
		return
	}
	if !common.ValidTeamType(teamType) {
		h.renderTeamForm(w, r, teamFormValues{
			Message:  "Неверный тип команды",
			Name:     name,
			Type:     teamType,
			ParentID: parentID,
		}, teams, teamID, true)
		return
	}
	if parentID != nil {
		if *parentID == teamID {
			h.renderTeamForm(w, r, teamFormValues{
				Message:  "Команда не может быть родителем самой себя",
				Name:     name,
				Type:     teamType,
				ParentID: parentID,
			}, teams, teamID, true)
			return
		}
		if !teamExists(teams, *parentID) {
			h.renderTeamForm(w, r, teamFormValues{
				Message:  "Выбранная родительская команда не найдена",
				Name:     name,
				Type:     teamType,
				ParentID: parentID,
			}, teams, teamID, true)
			return
		}
		descendants := collectDescendants(teams, teamID)
		if descendants[*parentID] {
			h.renderTeamForm(w, r, teamFormValues{
				Message:  "Нельзя привязать команду к её дочерней команде",
				Name:     name,
				Type:     teamType,
				ParentID: parentID,
			}, teams, teamID, true)
			return
		}
	}
	if err := h.deps.Store.UpdateTeam(ctx, store.TeamInput{Name: name, Type: teamType, ParentID: parentID}, teamID); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	http.Redirect(w, r, "/teams", http.StatusSeeOther)
}

type teamFormValues struct {
	Message  string
	Name     string
	Type     domain.TeamType
	ParentID *int64
}

func (h *Handler) renderTeamForm(w http.ResponseWriter, r *http.Request, values teamFormValues, teams []domain.Team, teamID int64, isEdit bool) {
	types := buildTeamTypeOptions(values.Type)
	parentOptions := buildParentTeamOptions(teams, values.ParentID, teamID)
	page := teamFormPage{
		FormError:       values.Message,
		PageTitle:       "Команда",
		ContentTemplate: "team-form-content",
		BreadcrumbTitle: map[bool]string{true: "Редактирование", false: "Новая команда"}[isEdit],
		Title:           map[bool]string{true: "Редактировать команду", false: "Создать команду"}[isEdit],
		FormAction:      map[bool]string{true: fmt.Sprintf("/teams/%d/update", teamID), false: "/teams"}[isEdit],
		SubmitLabel:     map[bool]string{true: "Сохранить", false: "Создать"}[isEdit],
		TeamName:        values.Name,
		TeamTypes:       types,
		ParentTeams:     parentOptions,
	}
	common.RenderTemplate(w, h.deps.Templates, "base", page, h.deps.Logger)
}

func (h *Handler) HandleUpdateTeamQuarterStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	year, quarter := common.ParseQuarter(r, h.deps.Zone)
	status := domain.TeamQuarterStatus(r.FormValue("status"))
	if !common.ValidTeamQuarterStatus(status) {
		common.RenderError(w, h.deps.Logger, fmt.Errorf("invalid team quarter status"))
		return
	}
	if err := h.deps.Store.SetTeamQuarterStatus(ctx, teamID, year, quarter, status); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/teams/%d/okr?year=%d&quarter=%d", teamID, year, quarter), http.StatusSeeOther)
}

func (h *Handler) HandleDeleteTeam(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if err := h.deps.Store.DeleteTeam(ctx, teamID); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	http.Redirect(w, r, "/teams", http.StatusSeeOther)
}

func (h *Handler) HandleTeamOKR(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	year, quarter := common.ParseQuarter(r, h.deps.Zone)

	team, err := h.deps.Store.GetTeam(ctx, teamID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}

	goals, err := h.deps.Store.ListGoalsByTeamQuarter(ctx, teamID, year, quarter)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	for i := range goals {
		comments, err := h.deps.Store.ListGoalComments(ctx, goals[i].ID)
		if err != nil {
			common.RenderError(w, h.deps.Logger, err)
			return
		}
		goals[i].Comments = comments
	}
	status, err := h.deps.Store.GetTeamQuarterStatus(ctx, teamID, year, quarter)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	var totalWeight int
	for i := range goals {
		goals[i].Progress = common.CalculateGoalProgress(goals[i])
		totalWeight += goals[i].Weight
	}
	page := teamOKRPage{
		Team:             team,
		TeamTypeLabel:    common.TeamTypeLabel(team.Type),
		Year:             year,
		Quarter:          quarter,
		Goals:            goals,
		QuarterStatus:    status,
		StatusOptions:    buildTeamStatusOptions(status),
		IsClosed:         status == domain.TeamQuarterStatusClosed,
		QuarterProgress:  okr.QuarterProgress(goals),
		GoalsCount:       len(goals),
		GoalsWeight:      totalWeight,
		PageTitle:        "OKR команды",
		ContentTemplate:  "team-okr-content",
		ObjectiveBlockV2: common.FeatureEnabled("okr_objective_block_ui_v2"),
	}
	common.RenderTemplate(w, h.deps.Templates, "base", page, h.deps.Logger)
}

func (h *Handler) HandleCreateGoal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	year, quarter := common.ParseQuarter(r, h.deps.Zone)
	status, err := h.deps.Store.GetTeamQuarterStatus(ctx, teamID, year, quarter)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if status == domain.TeamQuarterStatusClosed {
		h.renderTeamOKRWithError(w, r, teamID, year, quarter, "Квартал закрыт, изменения недоступны")
		return
	}
	weight := common.ParseIntField(r.FormValue("weight"))
	priority := domain.Priority(r.FormValue("priority"))
	workType := domain.WorkType(r.FormValue("work_type"))
	focusType := domain.FocusType(r.FormValue("focus_type"))

	validationErr := common.ValidateGoalInput(priority, workType, focusType, weight)
	if validationErr != "" {
		h.renderTeamOKRWithError(w, r, teamID, year, quarter, validationErr)
		return
	}

	_, err = h.deps.Store.CreateGoal(ctx, store.GoalInput{
		TeamID:      teamID,
		Year:        year,
		Quarter:     quarter,
		Title:       common.TrimmedFormValue(r, "title"),
		Description: common.TrimmedFormValue(r, "description"),
		Priority:    priority,
		Weight:      weight,
		WorkType:    workType,
		FocusType:   focusType,
		OwnerText:   common.TrimmedFormValue(r, "owner_text"),
	})
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if status == domain.TeamQuarterStatusNoGoals {
		if err := h.deps.Store.SetTeamQuarterStatus(ctx, teamID, year, quarter, domain.TeamQuarterStatusForming); err != nil {
			common.RenderError(w, h.deps.Logger, err)
			return
		}
	}

	http.Redirect(w, r, fmt.Sprintf("/teams/%d/okr?year=%d&quarter=%d", teamID, year, quarter), http.StatusSeeOther)
}

func (h *Handler) renderTeamOKRWithError(w http.ResponseWriter, r *http.Request, teamID int64, year, quarter int, message string) {
	team, err := h.deps.Store.GetTeam(r.Context(), teamID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	goals, err := h.deps.Store.ListGoalsByTeamQuarter(r.Context(), teamID, year, quarter)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	for i := range goals {
		comments, err := h.deps.Store.ListGoalComments(r.Context(), goals[i].ID)
		if err != nil {
			common.RenderError(w, h.deps.Logger, err)
			return
		}
		goals[i].Comments = comments
	}
	status, err := h.deps.Store.GetTeamQuarterStatus(r.Context(), teamID, year, quarter)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	var totalWeight int
	for i := range goals {
		goals[i].Progress = common.CalculateGoalProgress(goals[i])
		totalWeight += goals[i].Weight
	}
	page := teamOKRPage{
		Team:             team,
		TeamTypeLabel:    common.TeamTypeLabel(team.Type),
		Year:             year,
		Quarter:          quarter,
		Goals:            goals,
		QuarterStatus:    status,
		StatusOptions:    buildTeamStatusOptions(status),
		IsClosed:         status == domain.TeamQuarterStatusClosed,
		QuarterProgress:  okr.QuarterProgress(goals),
		GoalsCount:       len(goals),
		GoalsWeight:      totalWeight,
		FormError:        message,
		PageTitle:        "OKR команды",
		ContentTemplate:  "team-okr-content",
		ObjectiveBlockV2: common.FeatureEnabled("okr_objective_block_ui_v2"),
	}
	common.RenderTemplate(w, h.deps.Templates, "base", page, h.deps.Logger)
}

func (h *Handler) appendTeamRows(ctx context.Context, rows *[]teamRow, team domain.Team, level int, year, quarter int, childrenMap map[int64][]domain.Team) error {
	goals, err := h.deps.Store.ListGoalsByTeamQuarter(ctx, team.ID, year, quarter)
	if err != nil {
		return err
	}
	status, err := h.deps.Store.GetTeamQuarterStatus(ctx, team.ID, year, quarter)
	if err != nil {
		return err
	}
	for i := range goals {
		goals[i].Progress = common.CalculateGoalProgress(goals[i])
	}
	quarterProgress := okr.QuarterProgress(goals)
	totalWeight := 0
	for _, goal := range goals {
		totalWeight += goal.Weight
	}
	*rows = append(*rows, teamRow{
		ID:              team.ID,
		Name:            team.Name,
		TypeLabel:       common.TeamTypeLabel(team.Type),
		Indent:          level * 24,
		StatusLabel:     common.TeamQuarterStatusLabel(status),
		QuarterProgress: quarterProgress,
		GoalsCount:      len(goals),
		Goals:           goals,
		GoalsWeight:     totalWeight,
	})
	for _, child := range childrenMap[team.ID] {
		if err := h.appendTeamRows(ctx, rows, child, level+1, year, quarter, childrenMap); err != nil {
			return err
		}
	}
	return nil
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

func buildTeamFilterOptions(rootTeams []domain.Team, selected string) []teamFilterOption {
	options := []teamFilterOption{{Value: "ALL", Label: "Все команды", Selected: selected == "ALL"}}
	for _, team := range rootTeams {
		value := fmt.Sprintf("%d", team.ID)
		label := fmt.Sprintf("%s %s", common.TeamTypeLabel(team.Type), team.Name)
		options = append(options, teamFilterOption{Value: value, Label: label, Selected: selected == value})
	}
	return options
}

func buildTeamTypeOptions(selected domain.TeamType) []teamTypeOption {
	return []teamTypeOption{
		{Value: string(domain.TeamTypeCluster), Label: common.TeamTypeLabel(domain.TeamTypeCluster), Selected: selected == domain.TeamTypeCluster},
		{Value: string(domain.TeamTypeUnit), Label: common.TeamTypeLabel(domain.TeamTypeUnit), Selected: selected == domain.TeamTypeUnit},
		{Value: string(domain.TeamTypeTeam), Label: common.TeamTypeLabel(domain.TeamTypeTeam), Selected: selected == domain.TeamTypeTeam},
	}
}

func buildParentTeamOptions(teams []domain.Team, selectedParentID *int64, excludeID int64) []teamParentOption {
	descendants := map[int64]bool{}
	if excludeID != 0 {
		descendants = collectDescendants(teams, excludeID)
		descendants[excludeID] = true
	}
	options := []teamParentOption{{Value: "", Label: "Без родителя", Selected: selectedParentID == nil}}
	for _, team := range teams {
		if descendants[team.ID] {
			continue
		}
		value := fmt.Sprintf("%d", team.ID)
		selected := selectedParentID != nil && *selectedParentID == team.ID
		label := fmt.Sprintf("%s %s", common.TeamTypeLabel(team.Type), team.Name)
		options = append(options, teamParentOption{Value: value, Label: label, Selected: selected})
	}
	return options
}

func buildTeamStatusOptions(selected domain.TeamQuarterStatus) []teamStatusOption {
	return []teamStatusOption{
		{Value: string(domain.TeamQuarterStatusNoGoals), Label: common.TeamQuarterStatusLabel(domain.TeamQuarterStatusNoGoals), Selected: selected == domain.TeamQuarterStatusNoGoals},
		{Value: string(domain.TeamQuarterStatusForming), Label: common.TeamQuarterStatusLabel(domain.TeamQuarterStatusForming), Selected: selected == domain.TeamQuarterStatusForming},
		{Value: string(domain.TeamQuarterStatusInProgress), Label: common.TeamQuarterStatusLabel(domain.TeamQuarterStatusInProgress), Selected: selected == domain.TeamQuarterStatusInProgress},
		{Value: string(domain.TeamQuarterStatusValidated), Label: common.TeamQuarterStatusLabel(domain.TeamQuarterStatusValidated), Selected: selected == domain.TeamQuarterStatusValidated},
		{Value: string(domain.TeamQuarterStatusClosed), Label: common.TeamQuarterStatusLabel(domain.TeamQuarterStatusClosed), Selected: selected == domain.TeamQuarterStatusClosed},
	}
}

func collectDescendants(teams []domain.Team, rootID int64) map[int64]bool {
	_, childrenMap, _ := buildTeamHierarchy(teams)
	visited := map[int64]bool{}
	var walk func(id int64)
	walk = func(id int64) {
		for _, child := range childrenMap[id] {
			if visited[child.ID] {
				continue
			}
			visited[child.ID] = true
			walk(child.ID)
		}
	}
	walk(rootID)
	return visited
}

func parseOptionalID(value string) (*int64, error) {
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func teamExists(teams []domain.Team, id int64) bool {
	for _, team := range teams {
		if team.ID == id {
			return true
		}
	}
	return false
}
