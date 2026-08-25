// Package categorycounts serves GET /api/v1/activity/category-counts.
package categorycounts

import (
	"context"
	"net/http"
	"time"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/api/v1/activity/activitycommon"
	activitysvc "okrs/internal/service/activity"
)

// CategoryCounter считает события по категориям. *activity.Service удовлетворяет.
type CategoryCounter interface {
	CountByCategory(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, f activitysvc.Filter) (map[string]int, error)
}

type Handler struct {
	activity CategoryCounter
	now      func() time.Time
}

func New(activity CategoryCounter) *Handler {
	return &Handler{activity: activity, now: time.Now}
}

// Get serves GET /api/v1/activity/category-counts — счётчики по категориям для текущих
// фильтров. Саму категорию сервис из фильтра выбрасывает, чтобы счётчики вкладок не
// менялись при переключении вкладки.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	counts, err := h.activity.CountByCategory(r.Context(), scope, activitycommon.ScopeTeams(r), activitycommon.ParseFilter(r, h.now()))
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to count activity", nil)
		return
	}
	total := 0
	for _, n := range counts {
		total += n
	}
	v1.WriteJSON(w, http.StatusOK, map[string]any{"counts": counts, "total": total})
}
