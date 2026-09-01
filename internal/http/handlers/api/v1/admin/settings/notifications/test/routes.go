package test

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	// POST, not GET: this changes state in an external system — it posts a message.
	r.Post("/api/v1/admin/settings/notifications/{channel}/test", h.Test)
}
