package activity

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/api/v1/activity", h.HandleFeed)
	r.Get("/api/v1/activity/tree-counts", h.HandleTreeCounts)
	r.Get("/api/v1/activity/category-counts", h.HandleCategoryCounts)
}
