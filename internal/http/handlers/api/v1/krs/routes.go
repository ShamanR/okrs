package krs

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Post("/api/v1/goals/{goalID}/key-results", h.HandleCreateKeyResult)
	r.Post("/api/v1/krs/{krID}/progress/numerical", h.HandleUpdateNumericalProgress)
	r.Post("/api/v1/krs/{krID}/progress/boolean", h.HandleUpdateBooleanProgress)
	r.Post("/api/v1/krs/{krID}/progress/project", h.HandleUpdateProjectProgress)
	r.Post("/api/v1/krs/{krID}/note", h.HandleUpsertKRNote)
	r.Post("/api/v1/krs/{krID}/description", h.HandleUpdateKRDescription)
	r.Post("/api/v1/krs/{krID}", h.HandleUpdateKeyResult)
	r.Post("/api/v1/krs/{krID}/move-up", h.HandleMoveKeyResultUp)
	r.Post("/api/v1/krs/{krID}/move-down", h.HandleMoveKeyResultDown)
	r.Delete("/api/v1/krs/{krID}", h.HandleDeleteKeyResult)
}
