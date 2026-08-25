// Package treecounts serves GET /api/v1/activity/tree-counts.
package treecounts

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/api/v1/activity/activitycommon"
)

// TreeCounter считает события по командам для дерева в сайдбаре.
// *activity.Service удовлетворяет.
type TreeCounter interface {
	TreeCounts(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, periodID *int64, since *time.Time) (map[int64]int, error)
}

type Handler struct {
	activity TreeCounter
	now      func() time.Time
}

func New(activity TreeCounter) *Handler {
	return &Handler{activity: activity, now: time.Now}
}

// Get serves GET /api/v1/activity/tree-counts.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	q := r.URL.Query()
	counts, err := h.activity.TreeCounts(r.Context(), scope, activitycommon.ScopeTeams(r),
		activitycommon.ParseInt64(q.Get("period_id")), activitycommon.SinceFromRange(q.Get("range"), h.now()))
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to count activity", nil)
		return
	}
	// Ключи JSON-объекта — строки, поэтому id команды форматируется в строку здесь,
	// а не тащится в сервис.
	out := make(map[string]int, len(counts))
	for teamID, n := range counts {
		out[strconv.FormatInt(teamID, 10)] = n
	}
	v1.WriteJSON(w, http.StatusOK, map[string]any{"counts": out})
}
