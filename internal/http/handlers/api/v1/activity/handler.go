package activity

import (
	"net/http"
	"strconv"
	"time"

	"okrs/internal/auth"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/service"
	storeactivity "okrs/internal/store/activity"
)

type Handler struct {
	service *service.Service
}

func New(service *service.Service) *Handler { return &Handler{service: service} }

// scopeTeams returns the allowed team ids for the request. nil => admin/unrestricted (or scope
// not loaded, e.g. auth disabled). A non-nil slice (incl. empty) restricts visibility; an empty
// slice fails closed (no access).
func scopeTeams(r *http.Request) []int64 {
	allowed, ok := auth.AllowedTeamIDsFromCtx(r.Context())
	if ok && allowed != nil {
		return allowed
	}
	return nil
}

func parseInt64(s string) *int64 {
	if s == "" {
		return nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil
	}
	return &v
}

func sinceFromRange(rng string) *time.Time {
	now := time.Now()
	switch rng {
	case "today":
		t := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		return &t
	case "7d":
		t := now.Add(-7 * 24 * time.Hour)
		return &t
	case "30d":
		t := now.Add(-30 * 24 * time.Hour)
		return &t
	default: // "all" or empty
		return nil
	}
}

// HandleFeed serves GET /api/v1/activity.
func (h *Handler) HandleFeed(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	q := r.URL.Query()
	var teamIDs []int64
	for _, s := range q["team_ids"] {
		if p := parseInt64(s); p != nil {
			teamIDs = append(teamIDs, *p)
		}
	}
	limit := 50
	if p := parseInt64(q.Get("limit")); p != nil {
		limit = int(*p)
	}
	f := storeactivity.ListFilter{
		PeriodID:  parseInt64(q.Get("period_id")),
		TeamIDs:   teamIDs,
		Category:  q.Get("category"),
		ActorUDID: q.Get("actor_udid"),
		Since:     sinceFromRange(q.Get("range")),
		Query:     q.Get("q"),
		Limit:     limit,
		Cursor:    decodeCursor(q.Get("cursor")),
	}
	events, next, err := h.service.ListActivity(r.Context(), scope, scopeTeams(r), f)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to list activity", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, newFeedResponse(events, next))
}

// HandleCategoryCounts serves GET /api/v1/activity/category-counts — per-category totals for the
// current filters (period/team/range/author/q), excluding category so tab counters stay stable.
func (h *Handler) HandleCategoryCounts(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	q := r.URL.Query()
	var teamIDs []int64
	for _, s := range q["team_ids"] {
		if p := parseInt64(s); p != nil {
			teamIDs = append(teamIDs, *p)
		}
	}
	f := storeactivity.ListFilter{
		PeriodID:  parseInt64(q.Get("period_id")),
		TeamIDs:   teamIDs,
		ActorUDID: q.Get("actor_udid"),
		Since:     sinceFromRange(q.Get("range")),
		Query:     q.Get("q"),
	}
	counts, err := h.service.ActivityCategoryCounts(r.Context(), scope, scopeTeams(r), f)
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

// HandleTreeCounts serves GET /api/v1/activity/tree-counts.
func (h *Handler) HandleTreeCounts(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	q := r.URL.Query()
	counts, err := h.service.ActivityTreeCounts(r.Context(), scope, scopeTeams(r), parseInt64(q.Get("period_id")), sinceFromRange(q.Get("range")))
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to count activity", nil)
		return
	}
	out := make(map[string]int, len(counts))
	for teamID, n := range counts {
		out[strconv.FormatInt(teamID, 10)] = n
	}
	v1.WriteJSON(w, http.StatusOK, map[string]any{"counts": out})
}
