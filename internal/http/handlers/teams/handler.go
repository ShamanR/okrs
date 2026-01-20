package teams

import (
	"fmt"
	"net/http"

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
	QuarterProgress int
	GoalsCount      int
	Goals           []domain.Goal
}

type teamsPage struct {
	QuarterOptions  []common.QuarterOption
	SelectedYear    int
	SelectedQuarter int
	Teams           []teamRow
	CurrentYear     int
	PageTitle       string
	ContentTemplate string
}

type teamOKRPage struct {
	Team            domain.Team
	Year            int
	Quarter         int
	Goals           []domain.Goal
	QuarterProgress int
	GoalsCount      int
	GoalsWeight     int
	FormError       string
	PageTitle       string
	ContentTemplate string
}

func (h *Handler) HandleTeams(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	year, quarter := common.ParseQuarter(r, h.deps.Zone)
	options := common.BuildQuarterOptions(year, quarter, h.deps.Zone)

	teams, err := h.deps.Store.ListTeams(ctx)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}

	rows := make([]teamRow, 0, len(teams))
	for _, team := range teams {
		goals, err := h.deps.Store.ListGoalsByTeamQuarter(ctx, team.ID, year, quarter)
		if err != nil {
			common.RenderError(w, h.deps.Logger, err)
			return
		}
		for i := range goals {
			goals[i].Progress = common.CalculateGoalProgress(goals[i])
		}
		quarterProgress := okr.QuarterProgress(goals)
		rows = append(rows, teamRow{ID: team.ID, Name: team.Name, QuarterProgress: quarterProgress, GoalsCount: len(goals), Goals: goals})
	}

	page := teamsPage{
		QuarterOptions:  options,
		SelectedYear:    year,
		SelectedQuarter: quarter,
		Teams:           rows,
		CurrentYear:     year,
		PageTitle:       "Команды",
		ContentTemplate: "teams-content",
	}
	common.RenderTemplate(w, h.deps.Templates, "base", page, h.deps.Logger)
}

func (h *Handler) HandleCreateTeam(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	name := common.TrimmedFormValue(r, "name")
	if name == "" {
		h.renderTeamForm(w, r, "Название команды обязательно")
		return
	}
	if _, err := h.deps.Store.CreateTeam(ctx, name); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	http.Redirect(w, r, "/teams", http.StatusSeeOther)
}

func (h *Handler) HandleNewTeam(w http.ResponseWriter, r *http.Request) {
	h.renderTeamForm(w, r, "")
}

func (h *Handler) renderTeamForm(w http.ResponseWriter, r *http.Request, message string) {
	page := struct {
		FormError       string
		PageTitle       string
		ContentTemplate string
	}{
		FormError:       message,
		PageTitle:       "Новая команда",
		ContentTemplate: "team-new-content",
	}
	common.RenderTemplate(w, h.deps.Templates, "base", page, h.deps.Logger)
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
	var totalWeight int
	for i := range goals {
		goals[i].Progress = common.CalculateGoalProgress(goals[i])
		totalWeight += goals[i].Weight
	}
	page := teamOKRPage{
		Team:            team,
		Year:            year,
		Quarter:         quarter,
		Goals:           goals,
		QuarterProgress: okr.QuarterProgress(goals),
		GoalsCount:      len(goals),
		GoalsWeight:     totalWeight,
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
	if err := r.ParseForm(); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	year, quarter := common.ParseQuarter(r, h.deps.Zone)
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
	var totalWeight int
	for i := range goals {
		goals[i].Progress = common.CalculateGoalProgress(goals[i])
		totalWeight += goals[i].Weight
	}
	page := teamOKRPage{
		Team:            team,
		Year:            year,
		Quarter:         quarter,
		Goals:           goals,
		QuarterProgress: okr.QuarterProgress(goals),
		GoalsCount:      len(goals),
		GoalsWeight:     totalWeight,
		FormError:       message,
		PageTitle:       "OKR команды",
		ContentTemplate: "team-okr-content",
	}
	common.RenderTemplate(w, h.deps.Templates, "base", page, h.deps.Logger)
}
