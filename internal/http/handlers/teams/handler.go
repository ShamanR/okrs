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

const maxMultipartMemory = 32 << 20

func New(deps common.Dependencies) *Handler {
	return &Handler{deps: deps}
}

type teamRow struct {
	ID             int64
	Name           string
	TypeLabel      string
	Indent         int
	StatusLabel    string
	PeriodProgress int
	GoalsCount     int
	Goals          []teamGoalRow
	GoalsWeight    int
}

type goalShareTeam struct {
	ID        int64
	Name      string
	TypeLabel string
}

type shareTeamOption struct {
	ID       int64
	Label    string
	Disabled bool
	Selected bool
}

type teamGoalRow struct {
	ID         int64
	Title      string
	Weight     int
	Progress   int
	ShareTeams []goalShareTeam
}

type teamFilterOption struct {
	Value    string
	Label    string
	Selected bool
}

type teamOkrsPage struct {
	PeriodOptions   []periodOption
	SelectedPeriod  int64
	SelectedTeam    string
	PageTitle       string
	ContentTemplate string
}

type periodOption struct {
	ID       int64
	Name     string
	Selected bool
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

type teamManageRow struct {
	ID          int64
	Name        string
	TypeLabel   string
	Lead        string
	Description string
	IndentPx    int
}

type teamManagePage struct {
	PageTitle       string
	ContentTemplate string
	Teams           []teamManageRow
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
	TeamLead        string
	TeamDescription string
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
	Period           domain.Period
	Goals            []domain.Goal
	GoalShares       map[int64][]goalShareTeam
	GoalShareIDs     map[int64]map[int64]bool
	GoalShareTargets map[int64][]shareTeamOption
	PeriodStatus     domain.TeamPeriodStatus
	StatusOptions    []teamStatusOption
	IsClosed         bool
	PeriodProgress   int
	GoalsCount       int
	GoalsWeight      int
	FormError        string
	ObjectiveBlockV2 bool
	PageTitle        string
	ContentTemplate  string
}

func (h *Handler) HandleTeamOKRs(w http.ResponseWriter, r *http.Request) {
	period, periodValue, periods, err := h.resolvePeriodFilter(r.Context(), r)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	options := buildPeriodOptions(periods, period.ID)
	selectedFilter := resolveTeamFilter(r)

	page := teamOkrsPage{
		PeriodOptions:   options,
		SelectedPeriod:  period.ID,
		SelectedTeam:    selectedFilter,
		PageTitle:       "OKR команд",
		ContentTemplate: "teams-content",
	}
	persistTeamsFilters(w, periodValue, selectedFilter)
	common.RenderTemplate(w, h.deps.Templates, "base", page, h.deps.Logger)
}

func (h *Handler) HandleTeamManagement(w http.ResponseWriter, r *http.Request) {
	teams, err := h.deps.Store.ListTeams(r.Context())
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	page := teamManagePage{
		PageTitle:       "Управление командами",
		ContentTemplate: "team-manage-content",
		Teams:           buildTeamManagementRows(teams),
	}
	common.RenderTemplate(w, h.deps.Templates, "base", page, h.deps.Logger)
}

func (h *Handler) resolvePeriodFilter(ctx context.Context, r *http.Request) (domain.Period, string, []domain.Period, error) {
	periods, err := h.deps.Store.ListPeriods(ctx)
	if err != nil {
		return domain.Period{}, "", nil, err
	}
	var selectedID int64
	if periodID, err := common.ParsePeriodID(r); err != nil {
		return domain.Period{}, "", nil, err
	} else if periodID > 0 {
		selectedID = periodID
	}
	if selectedID == 0 {
		if cookie, err := r.Cookie("teams_period"); err == nil {
			if parsed, err := common.ParseID(cookie.Value); err == nil {
				selectedID = parsed
			}
		}
	}
	var selectedPeriod domain.Period
	if selectedID != 0 {
		for _, period := range periods {
			if period.ID == selectedID {
				selectedPeriod = period
				break
			}
		}
	}
	if selectedPeriod.ID == 0 && len(periods) > 0 {
		if current, err := h.deps.Store.FindPeriodForDate(ctx, time.Now().In(h.deps.Zone)); err == nil {
			selectedPeriod = current
		} else {
			selectedPeriod = periods[0]
		}
	}
	if selectedPeriod.ID == 0 {
		return domain.Period{}, "", periods, nil
	}
	return selectedPeriod, fmt.Sprintf("%d", selectedPeriod.ID), periods, nil
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

func buildPeriodOptions(periods []domain.Period, selectedID int64) []periodOption {
	options := make([]periodOption, 0, len(periods))
	for _, period := range periods {
		options = append(options, periodOption{
			ID:       period.ID,
			Name:     period.Name,
			Selected: period.ID == selectedID,
		})
	}
	return options
}

func persistTeamsFilters(w http.ResponseWriter, periodValue, selectedFilter string) {
	http.SetCookie(w, &http.Cookie{
		Name:   "teams_period",
		Value:  periodValue,
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
	lead := common.TrimmedFormValue(r, "lead")
	description := common.TrimmedFormValue(r, "description")
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
			Message:     "Название команды обязательно",
			Name:        name,
			Type:        teamType,
			ParentID:    parentID,
			Lead:        lead,
			Description: description,
		}, teams, 0, false)
		return
	}
	if !common.ValidTeamType(teamType) {
		h.renderTeamForm(w, r, teamFormValues{
			Message:     "Неверный тип команды",
			Name:        name,
			Type:        teamType,
			ParentID:    parentID,
			Lead:        lead,
			Description: description,
		}, teams, 0, false)
		return
	}
	if parentID != nil && !teamExists(teams, *parentID) {
		h.renderTeamForm(w, r, teamFormValues{
			Message:     "Выбранная родительская команда не найдена",
			Name:        name,
			Type:        teamType,
			ParentID:    parentID,
			Lead:        lead,
			Description: description,
		}, teams, 0, false)
		return
	}
	if _, err := h.deps.Store.CreateTeam(ctx, store.TeamInput{Name: name, Type: teamType, ParentID: parentID, Lead: lead, Description: description}); err != nil {
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
		Name:        "",
		Type:        domain.TeamTypeTeam,
		ParentID:    nil,
		Lead:        "",
		Description: "",
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
		Name:        team.Name,
		Type:        team.Type,
		ParentID:    team.ParentID,
		Lead:        team.Lead,
		Description: team.Description,
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
	lead := common.TrimmedFormValue(r, "lead")
	description := common.TrimmedFormValue(r, "description")
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
			Message:     "Название команды обязательно",
			Name:        name,
			Type:        teamType,
			ParentID:    parentID,
			Lead:        lead,
			Description: description,
		}, teams, teamID, true)
		return
	}
	if !common.ValidTeamType(teamType) {
		h.renderTeamForm(w, r, teamFormValues{
			Message:     "Неверный тип команды",
			Name:        name,
			Type:        teamType,
			ParentID:    parentID,
			Lead:        lead,
			Description: description,
		}, teams, teamID, true)
		return
	}
	if parentID != nil {
		if *parentID == teamID {
			h.renderTeamForm(w, r, teamFormValues{
				Message:     "Команда не может быть родителем самой себя",
				Name:        name,
				Type:        teamType,
				ParentID:    parentID,
				Lead:        lead,
				Description: description,
			}, teams, teamID, true)
			return
		}
		if !teamExists(teams, *parentID) {
			h.renderTeamForm(w, r, teamFormValues{
				Message:     "Выбранная родительская команда не найдена",
				Name:        name,
				Type:        teamType,
				ParentID:    parentID,
				Lead:        lead,
				Description: description,
			}, teams, teamID, true)
			return
		}
		descendants := collectDescendants(teams, teamID)
		if descendants[*parentID] {
			h.renderTeamForm(w, r, teamFormValues{
				Message:     "Нельзя привязать команду к её дочерней команде",
				Name:        name,
				Type:        teamType,
				ParentID:    parentID,
				Lead:        lead,
				Description: description,
			}, teams, teamID, true)
			return
		}
	}
	if err := h.deps.Store.UpdateTeam(ctx, store.TeamInput{Name: name, Type: teamType, ParentID: parentID, Lead: lead, Description: description}, teamID); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	http.Redirect(w, r, "/teams", http.StatusSeeOther)
}

type teamFormValues struct {
	Message     string
	Name        string
	Type        domain.TeamType
	ParentID    *int64
	Lead        string
	Description string
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
		TeamLead:        values.Lead,
		TeamDescription: values.Description,
		TeamTypes:       types,
		ParentTeams:     parentOptions,
	}
	common.RenderTemplate(w, h.deps.Templates, "base", page, h.deps.Logger)
}

func (h *Handler) HandleUpdateTeamPeriodStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if err := r.ParseMultipartForm(maxMultipartMemory); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	periodID, err := common.ParsePeriodID(r)
	if err != nil || periodID == 0 {
		common.RenderError(w, h.deps.Logger, fmt.Errorf("invalid period id"))
		return
	}
	status := domain.TeamPeriodStatus(r.FormValue("status"))
	if !common.ValidTeamPeriodStatus(status) {
		common.RenderError(w, h.deps.Logger, fmt.Errorf("invalid team period status"))
		return
	}
	if err := h.deps.Store.SetTeamPeriodStatus(ctx, teamID, periodID, status); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/teams/%d/okr?period_id=%d", teamID, periodID), http.StatusSeeOther)
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
	period, _, _, err := h.resolvePeriodFilter(ctx, r)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if period.ID == 0 {
		common.RenderError(w, h.deps.Logger, fmt.Errorf("period not found"))
		return
	}
	team, err := h.deps.Service.GetTeam(ctx, teamID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	page := teamOKRPage{
		Team:            team,
		TeamTypeLabel:   common.TeamTypeLabel(team.Type),
		Period:          period,
		PageTitle:       "OKR команды",
		ContentTemplate: "team-okr-content",
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
	if err := r.ParseMultipartForm(maxMultipartMemory); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	periodID, err := common.ParsePeriodID(r)
	if err != nil || periodID == 0 {
		common.RenderError(w, h.deps.Logger, fmt.Errorf("invalid period id"))
		return
	}
	status, err := h.deps.Store.GetTeamPeriodStatus(ctx, teamID, periodID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if status == domain.TeamPeriodStatusClosed {
		h.renderTeamOKRWithError(w, r, teamID, periodID, "Период закрыт, изменения недоступны")
		return
	}
	weight := common.ParseIntField(r.FormValue("weight"))
	priority := domain.Priority(r.FormValue("priority"))
	workType := domain.WorkType(r.FormValue("work_type"))
	focusType := domain.FocusType(r.FormValue("focus_type"))

	validationErr := common.ValidateGoalInput(priority, workType, focusType, weight)
	if validationErr != "" {
		h.renderTeamOKRWithError(w, r, teamID, periodID, validationErr)
		return
	}

	_, err = h.deps.Store.CreateGoal(ctx, store.GoalInput{
		TeamID:      teamID,
		PeriodID:    periodID,
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
	if status == domain.TeamPeriodStatusNoGoals {
		if err := h.deps.Store.SetTeamPeriodStatus(ctx, teamID, periodID, domain.TeamPeriodStatusForming); err != nil {
			common.RenderError(w, h.deps.Logger, err)
			return
		}
	}

	http.Redirect(w, r, fmt.Sprintf("/teams/%d/okr?period_id=%d", teamID, periodID), http.StatusSeeOther)
}

func (h *Handler) renderTeamOKRWithError(w http.ResponseWriter, r *http.Request, teamID, periodID int64, message string) {
	team, err := h.deps.Store.GetTeam(r.Context(), teamID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	period, err := h.deps.Store.GetPeriod(r.Context(), periodID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	goals, err := h.deps.Store.ListGoalsByTeamPeriod(r.Context(), teamID, periodID)
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
	teams, err := h.deps.Store.ListTeams(r.Context())
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	teamsByID := make(map[int64]domain.Team, len(teams))
	for _, teamItem := range teams {
		teamsByID[teamItem.ID] = teamItem
	}
	goalShares, goalShareIDs, err := h.buildGoalSharesMap(r.Context(), goals, teamsByID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	_, childrenMap, rootTeams := buildTeamHierarchy(teams)
	statuses, err := h.buildTeamPeriodStatuses(r.Context(), teams, periodID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	shareTargets := buildShareTargets(rootTeams, childrenMap, statuses)
	goalShareTargets := buildGoalShareTargets(shareTargets, goalShareIDs)
	status, err := h.deps.Store.GetTeamPeriodStatus(r.Context(), teamID, periodID)
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
		Period:           period,
		Goals:            goals,
		GoalShares:       goalShares,
		GoalShareIDs:     goalShareIDs,
		GoalShareTargets: goalShareTargets,
		PeriodStatus:     status,
		StatusOptions:    buildTeamStatusOptions(status),
		IsClosed:         status == domain.TeamPeriodStatusClosed,
		PeriodProgress:   okr.PeriodProgress(goals),
		GoalsCount:       len(goals),
		GoalsWeight:      totalWeight,
		FormError:        message,
		PageTitle:        "OKR команды",
		ContentTemplate:  "team-okr-content",
		ObjectiveBlockV2: common.FeatureEnabled("okr_objective_block_ui_v2"),
	}
	common.RenderTemplate(w, h.deps.Templates, "base", page, h.deps.Logger)
}

func (h *Handler) appendTeamRows(ctx context.Context, rows *[]teamRow, team domain.Team, level int, periodID int64, childrenMap map[int64][]domain.Team, teamsByID map[int64]domain.Team) error {
	goals, err := h.deps.Store.ListGoalsByTeamPeriod(ctx, team.ID, periodID)
	if err != nil {
		return err
	}
	status, err := h.deps.Store.GetTeamPeriodStatus(ctx, team.ID, periodID)
	if err != nil {
		return err
	}
	goalRows := make([]teamGoalRow, 0, len(goals))
	for i := range goals {
		goals[i].Progress = common.CalculateGoalProgress(goals[i])
		shareTeams, err := h.buildGoalShareTeams(ctx, goals[i], teamsByID)
		if err != nil {
			return err
		}
		goalRows = append(goalRows, teamGoalRow{
			ID:         goals[i].ID,
			Title:      goals[i].Title,
			Weight:     goals[i].Weight,
			Progress:   goals[i].Progress,
			ShareTeams: shareTeams,
		})
	}
	periodProgress := okr.PeriodProgress(goals)
	totalWeight := 0
	for _, goal := range goals {
		totalWeight += goal.Weight
	}
	*rows = append(*rows, teamRow{
		ID:             team.ID,
		Name:           team.Name,
		TypeLabel:      common.TeamTypeLabel(team.Type),
		Indent:         level * 24,
		StatusLabel:    common.TeamPeriodStatusLabel(status),
		PeriodProgress: periodProgress,
		GoalsCount:     len(goals),
		Goals:          goalRows,
		GoalsWeight:    totalWeight,
	})
	for _, child := range childrenMap[team.ID] {
		if err := h.appendTeamRows(ctx, rows, child, level+1, periodID, childrenMap, teamsByID); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) buildGoalShareTeams(ctx context.Context, goal domain.Goal, teamsByID map[int64]domain.Team) ([]goalShareTeam, error) {
	shares, err := h.deps.Store.ListGoalShares(ctx, goal.ID)
	if err != nil {
		return nil, err
	}
	teamIDs := make(map[int64]struct{}, len(shares)+1)
	teamIDs[goal.TeamID] = struct{}{}
	for _, share := range shares {
		teamIDs[share.TeamID] = struct{}{}
	}
	teams := make([]goalShareTeam, 0, len(teamIDs))
	for teamID := range teamIDs {
		team, ok := teamsByID[teamID]
		if !ok {
			continue
		}
		teams = append(teams, goalShareTeam{
			ID:        team.ID,
			Name:      team.Name,
			TypeLabel: common.TeamTypeLabel(team.Type),
		})
	}
	sort.Slice(teams, func(i, j int) bool {
		return teams[i].Name < teams[j].Name
	})
	return teams, nil
}

func (h *Handler) buildGoalSharesMap(ctx context.Context, goals []domain.Goal, teamsByID map[int64]domain.Team) (map[int64][]goalShareTeam, map[int64]map[int64]bool, error) {
	result := make(map[int64][]goalShareTeam, len(goals))
	ids := make(map[int64]map[int64]bool, len(goals))
	for _, goal := range goals {
		teams, err := h.buildGoalShareTeams(ctx, goal, teamsByID)
		if err != nil {
			return nil, nil, err
		}
		result[goal.ID] = teams
		idSet := make(map[int64]bool, len(teams))
		for _, team := range teams {
			idSet[team.ID] = true
		}
		ids[goal.ID] = idSet
	}
	return result, ids, nil
}

func (h *Handler) buildTeamPeriodStatuses(ctx context.Context, teams []domain.Team, periodID int64) (map[int64]domain.TeamPeriodStatus, error) {
	statuses := make(map[int64]domain.TeamPeriodStatus, len(teams))
	for _, team := range teams {
		status, err := h.deps.Store.GetTeamPeriodStatus(ctx, team.ID, periodID)
		if err != nil {
			return nil, err
		}
		statuses[team.ID] = status
	}
	return statuses, nil
}

func buildShareTargets(rootTeams []domain.Team, childrenMap map[int64][]domain.Team, statuses map[int64]domain.TeamPeriodStatus) []shareTeamOption {
	options := make([]shareTeamOption, 0, len(rootTeams))
	for _, team := range rootTeams {
		appendShareTarget(&options, team, childrenMap, statuses, 0)
	}
	return options
}

func appendShareTarget(options *[]shareTeamOption, team domain.Team, childrenMap map[int64][]domain.Team, statuses map[int64]domain.TeamPeriodStatus, level int) {
	label := fmt.Sprintf("%s%s %s", teamHierarchyPrefix(level), common.TeamTypeLabel(team.Type), team.Name)
	status := statuses[team.ID]
	disabled := status == domain.TeamPeriodStatusValidated || status == domain.TeamPeriodStatusClosed
	*options = append(*options, shareTeamOption{
		ID:       team.ID,
		Label:    label,
		Disabled: disabled,
	})
	for _, child := range childrenMap[team.ID] {
		appendShareTarget(options, child, childrenMap, statuses, level+1)
	}
}

func buildGoalShareTargets(base []shareTeamOption, goalShareIDs map[int64]map[int64]bool) map[int64][]shareTeamOption {
	result := make(map[int64][]shareTeamOption, len(goalShareIDs))
	for goalID, teamIDs := range goalShareIDs {
		options := make([]shareTeamOption, 0, len(base))
		for _, option := range base {
			selected := teamIDs[option.ID]
			disabled := option.Disabled && !selected
			options = append(options, shareTeamOption{
				ID:       option.ID,
				Label:    option.Label,
				Disabled: disabled,
				Selected: selected,
			})
		}
		result[goalID] = options
	}
	return result
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

func buildTeamManagementRows(teams []domain.Team) []teamManageRow {
	_, childrenMap, roots := buildTeamHierarchy(teams)
	rows := make([]teamManageRow, 0, len(teams))
	var appendTeam func(team domain.Team, level int)
	appendTeam = func(team domain.Team, level int) {
		rows = append(rows, teamManageRow{
			ID:          team.ID,
			Name:        team.Name,
			TypeLabel:   common.TeamTypeLabel(team.Type),
			Lead:        team.Lead,
			Description: team.Description,
			IndentPx:    level * 24,
		})
		for _, child := range childrenMap[team.ID] {
			appendTeam(child, level+1)
		}
	}
	for _, team := range roots {
		appendTeam(team, 0)
	}
	return rows
}

func buildTeamFilterOptions(rootTeams []domain.Team, childrenMap map[int64][]domain.Team, selected string) []teamFilterOption {
	options := []teamFilterOption{{Value: "ALL", Label: "Все команды", Selected: selected == "ALL"}}
	for _, team := range rootTeams {
		appendTeamFilterOption(&options, team, childrenMap, 0, selected)
	}
	return options
}

func appendTeamFilterOption(options *[]teamFilterOption, team domain.Team, childrenMap map[int64][]domain.Team, level int, selected string) {
	value := fmt.Sprintf("%d", team.ID)
	label := fmt.Sprintf("%s%s %s", teamHierarchyPrefix(level), common.TeamTypeLabel(team.Type), team.Name)
	*options = append(*options, teamFilterOption{Value: value, Label: label, Selected: selected == value})
	for _, child := range childrenMap[team.ID] {
		appendTeamFilterOption(options, child, childrenMap, level+1, selected)
	}
}

func teamHierarchyPrefix(level int) string {
	if level == 0 {
		return ""
	}
	return fmt.Sprintf("%s|-- ", strings.Repeat("  ", level-1))
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

func buildTeamStatusOptions(selected domain.TeamPeriodStatus) []teamStatusOption {
	return []teamStatusOption{
		{Value: string(domain.TeamPeriodStatusNoGoals), Label: common.TeamPeriodStatusLabel(domain.TeamPeriodStatusNoGoals), Selected: selected == domain.TeamPeriodStatusNoGoals},
		{Value: string(domain.TeamPeriodStatusForming), Label: common.TeamPeriodStatusLabel(domain.TeamPeriodStatusForming), Selected: selected == domain.TeamPeriodStatusForming},
		{Value: string(domain.TeamPeriodStatusInProgress), Label: common.TeamPeriodStatusLabel(domain.TeamPeriodStatusInProgress), Selected: selected == domain.TeamPeriodStatusInProgress},
		{Value: string(domain.TeamPeriodStatusValidated), Label: common.TeamPeriodStatusLabel(domain.TeamPeriodStatusValidated), Selected: selected == domain.TeamPeriodStatusValidated},
		{Value: string(domain.TeamPeriodStatusClosed), Label: common.TeamPeriodStatusLabel(domain.TeamPeriodStatusClosed), Selected: selected == domain.TeamPeriodStatusClosed},
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
