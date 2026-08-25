package goaltree

import (
	"net/http"
	"strconv"
	"strings"

	"context"
	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/http/dto"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/web/common"
	goaltreeuc "okrs/internal/usecase/goaltree"
)

// PeriodViewLister отдаёт список периодов для режима cross_period.
// *period.Service удовлетворяет.
type PeriodViewLister interface {
	ListViews(ctx context.Context, scope domain.TenantScope, includeArchived bool) ([]domain.PeriodView, error)
}

// TreeBuilder собирает граф целей и связей. *goaltree.UseCase удовлетворяет.
type TreeBuilder interface {
	GoalTree(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, adminAll bool, periodIDs []int64, callerUDID string) (goaltreeuc.GoalTreeData, error)
}

type Handler struct {
	periods PeriodViewLister
	tree    TreeBuilder
}

func New(periods PeriodViewLister, tree TreeBuilder) *Handler {
	return &Handler{periods: periods, tree: tree}
}

// HandleGoalTree отдаёт агрегированный граф целей/связей в scope.
// GET /api/v1/goal-tree?period_id=&cross_period=
func (h *Handler) HandleGoalTree(w http.ResponseWriter, r *http.Request) {
	v1.SetAPICacheControl(w)
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	crossPeriod := r.URL.Query().Get("cross_period") == "1"

	views, err := h.periods.ListViews(r.Context(), scope, false)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to load periods", nil)
		return
	}
	var periodIDs []int64
	if crossPeriod {
		for _, v := range views {
			periodIDs = append(periodIDs, v.ID)
		}
	} else if raw := strings.TrimSpace(r.URL.Query().Get("period_id")); raw != "" {
		id, perr := strconv.ParseInt(raw, 10, 64)
		if perr != nil {
			v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid period_id", map[string]string{"period_id": "invalid"})
			return
		}
		periodIDs = []int64{id}
	}

	allowed, _ := auth.AllowedTeamIDsFromCtx(r.Context())
	adminAll := allowed == nil
	callerUDID := ""
	if u := auth.UserFromContext(r.Context()); u != nil {
		callerUDID = u.UDID
	}

	data, err := h.tree.GoalTree(r.Context(), scope, allowed, adminAll, periodIDs, callerUDID)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to load goal tree", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, buildResponse(data))
}

func buildResponse(data goaltreeuc.GoalTreeData) dto.GoalTreeResponse {
	resp := dto.GoalTreeResponse{
		Periods: make([]dto.GoalTreePeriod, 0, len(data.Periods)),
		Teams:   make([]dto.GoalTreeTeam, 0, len(data.Teams)),
		Goals:   make([]dto.GoalTreeGoal, 0, len(data.Nodes)),
	}
	for _, p := range data.Periods {
		resp.Periods = append(resp.Periods, dto.GoalTreePeriod{ID: p.ID, Name: p.Name, Depth: p.Depth, Status: string(p.Status)})
	}
	for _, t := range data.Teams {
		resp.Teams = append(resp.Teams, dto.GoalTreeTeam{
			ID: t.Team.ID, Name: t.Team.Name, Type: string(t.Team.Type),
			TypeLabel: common.TeamTypeLabel(t.Team.Type), ParentID: t.Team.ParentID,
			LedByMe: t.LedByMe,
		})
	}
	for _, n := range data.Nodes {
		g := n.Goal
		resp.Goals = append(resp.Goals, dto.GoalTreeGoal{
			ID: g.ID, Title: g.Title, TeamID: g.TeamID, PeriodID: g.PeriodID,
			Progress: g.Progress, Priority: string(g.Priority), Weight: g.Weight,
			WorkType: string(g.WorkType), FocusType: string(g.FocusType), OwnerText: g.OwnerText,
			ParentGoalIDs: emptyIfNil(n.ParentGoalIDs), ChildGoalIDs: emptyIfNil(n.ChildGoalIDs),
		})
	}
	return resp
}

func emptyIfNil(ids []int64) []int64 {
	if ids == nil {
		return []int64{}
	}
	return ids
}
