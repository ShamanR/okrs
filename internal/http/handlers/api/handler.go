package api

import (
	"net/http"

	"okrs/internal/http/handlers/common"
	"okrs/internal/okr"

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
}

func (h *Handler) HandleAPITeams(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	year, quarter := common.ParseQuarter(r, h.deps.Zone)
	teams, err := h.deps.Store.ListTeams(ctx)
	if err != nil {
		common.RenderJSONError(w, h.deps.Logger, err)
		return
	}

	response := make([]teamRow, 0, len(teams))
	for _, team := range teams {
		goals, err := h.deps.Store.ListGoalsByTeamQuarter(ctx, team.ID, year, quarter)
		if err != nil {
			common.RenderJSONError(w, h.deps.Logger, err)
			return
		}
		for i := range goals {
			goals[i].Progress = common.CalculateGoalProgress(goals[i])
		}
		response = append(response, teamRow{ID: team.ID, Name: team.Name, QuarterProgress: okr.QuarterProgress(goals), GoalsCount: len(goals)})
	}

	common.WriteJSON(w, response)
}

func (h *Handler) HandleAPITeamGoals(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		common.RenderJSONError(w, h.deps.Logger, err)
		return
	}
	year, quarter := common.ParseQuarter(r, h.deps.Zone)
	goals, err := h.deps.Store.ListGoalsByTeamQuarter(ctx, teamID, year, quarter)
	if err != nil {
		common.RenderJSONError(w, h.deps.Logger, err)
		return
	}
	for i := range goals {
		goals[i].Progress = common.CalculateGoalProgress(goals[i])
	}
	common.WriteJSON(w, goals)
}

func (h *Handler) HandleAPIGoal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		common.RenderJSONError(w, h.deps.Logger, err)
		return
	}
	goal, err := h.deps.Store.GetGoal(ctx, goalID)
	if err != nil {
		common.RenderJSONError(w, h.deps.Logger, err)
		return
	}
	goal.Progress = common.CalculateGoalProgress(goal)
	common.WriteJSON(w, goal)
}
