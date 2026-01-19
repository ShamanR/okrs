package goals

import (
	"fmt"
	"net/http"
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

	goal.Progress = common.CalculateGoalProgress(goal)

	page := struct {
		Team            domain.Team
		Goal            domain.Goal
		FormError       string
		PageTitle       string
		ContentTemplate string
	}{Team: team, Goal: goal, PageTitle: "Цель", ContentTemplate: "goal-content"}
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
	if err := h.deps.Store.DeleteGoal(ctx, goalID); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/teams/%d/okr?year=%d&quarter=%d", goal.TeamID, goal.Year, goal.Quarter), http.StatusSeeOther)
}

type yearGoalsPage struct {
	Year            int
	Goals           []store.GoalWithTeam
	YearValues      []int
	PageTitle       string
	ContentTemplate string
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
	values := buildYearOptions(year)
	page := yearGoalsPage{
		Year:            year,
		Goals:           goals,
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
	goal.Progress = common.CalculateGoalProgress(goal)
	page := struct {
		Team            domain.Team
		Goal            domain.Goal
		FormError       string
		PageTitle       string
		ContentTemplate string
	}{Team: team, Goal: goal, FormError: message, PageTitle: "Цель", ContentTemplate: "goal-content"}
	common.RenderTemplate(w, h.deps.Templates, "base", page, h.deps.Logger)
}
