package hierarchy

import (
	"net/http"
	"slices"
	"time"

	"context"
	"okrs/internal/auth"
	"okrs/internal/core/domain"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/web/common"
	teamsvc "okrs/internal/service/team"
	okrboarduc "okrs/internal/usecase/okrboard"
)

// TeamHierarchy строит дерево команд. *team.Service удовлетворяет.
type TeamHierarchy interface {
	Hierarchy(ctx context.Context, scope domain.TenantScope, periodID *int64) ([]teamsvc.Node, error)
}

// BoardSummary отдаёт метрики команд за период. *okrboard.UseCase удовлетворяет.
type BoardSummary interface {
	TeamsWithPeriodSummary(ctx context.Context, scope domain.TenantScope, periodID int64, orgID *int64) ([]okrboarduc.TeamSummary, error)
}

// PeriodReader читает период для расчёта прогноза. *period.Service удовлетворяет.
type PeriodReader interface {
	Get(ctx context.Context, scope domain.TenantScope, periodID int64) (domain.Period, error)
}

// UserDirectory резолвит лидов команд в отображаемых пользователей.
// *user.Service удовлетворяет.
type UserDirectory interface {
	GetByUDIDs(ctx context.Context, udids []string) ([]*domain.User, error)
}

type Handler struct {
	teams   TeamHierarchy
	board   BoardSummary
	periods PeriodReader
	users   UserDirectory
}

func New(teams TeamHierarchy, board BoardSummary, periods PeriodReader, users UserDirectory) *Handler {
	return &Handler{teams: teams, board: board, periods: periods, users: users}
}

func (h *Handler) HandleHierarchy(w http.ResponseWriter, r *http.Request) {
	v1.SetAPICacheControl(w)
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	periodID, err := common.ParsePeriodID(r)
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid period id", map[string]string{"period_id": "invalid"})
		return
	}
	var periodRef *int64
	if periodID > 0 {
		periodRef = &periodID
	}
	nodes, err := h.teams.Hierarchy(r.Context(), scope, periodRef)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to load hierarchy", nil)
		return
	}
	metrics := map[int64]okrboarduc.TeamSummary{}
	forecast := 0
	if periodRef != nil {
		summaries, err := h.board.TeamsWithPeriodSummary(r.Context(), scope, periodID, nil)
		if err != nil {
			v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to load hierarchy summary", nil)
			return
		}
		metrics = make(map[int64]okrboarduc.TeamSummary, len(summaries))
		for _, summary := range summaries {
			metrics[summary.ID] = summary
		}
		period, err := h.periods.Get(r.Context(), scope, periodID)
		if err != nil {
			v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to load hierarchy period", nil)
			return
		}
		forecast = v1.CalculatePeriodForecast(period, time.Now())
	}
	if allowedIDs, ok := auth.AllowedTeamIDsFromCtx(r.Context()); ok && allowedIDs != nil {
		nodes = filterNodesByScope(nodes, allowedIDs)
	}
	leadUDIDs := collectLeadUDIDs(nodes)
	users, _ := h.users.GetByUDIDs(r.Context(), leadUDIDs)
	v1.WriteJSON(w, http.StatusOK, newHierarchyResponse(nodes, metrics, v1.BuildUserRefMap(users), forecast))
}

func collectLeadUDIDs(nodes []teamsvc.Node) []string {
	seen := make(map[string]struct{})
	var walk func([]teamsvc.Node)
	walk = func(ns []teamsvc.Node) {
		for _, n := range ns {
			if n.Team.LeadUDID != nil {
				seen[*n.Team.LeadUDID] = struct{}{}
			}
			walk(n.Children)
		}
	}
	walk(nodes)
	udids := make([]string, 0, len(seen))
	for uid := range seen {
		udids = append(udids, uid)
	}
	return udids
}

// filterNodesByScope removes tree nodes not in allowedIDs and promotes orphaned children to their
// parent's level so the user sees a valid subtree rooted at their access boundary.
func filterNodesByScope(nodes []teamsvc.Node, allowedIDs []int64) []teamsvc.Node {
	result := make([]teamsvc.Node, 0, len(nodes))
	for _, node := range nodes {
		filteredChildren := filterNodesByScope(node.Children, allowedIDs)
		if slices.Contains(allowedIDs, node.Team.ID) {
			node.Children = filteredChildren
			result = append(result, node)
		} else {
			// promote accessible children to this level
			result = append(result, filteredChildren...)
		}
	}
	return result
}
