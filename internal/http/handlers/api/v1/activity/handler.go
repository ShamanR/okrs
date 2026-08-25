// Package activity serves GET /api/v1/activity.
package activity

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

// FeedReader отдаёт страницу журнала. *activity.Service удовлетворяет.
type FeedReader interface {
	Feed(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, f activitysvc.Filter) (activitysvc.Page, error)
}

type Handler struct {
	activity FeedReader
	now      func() time.Time
}

func New(activity FeedReader) *Handler {
	return &Handler{activity: activity, now: time.Now}
}

// Get serves GET /api/v1/activity.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	f := activitycommon.ParseFilter(r, h.now())
	page, err := h.activity.Feed(r.Context(), scope, activitycommon.ScopeTeams(r), f)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to list activity", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, activitycommon.FeedResponse(page))
}
