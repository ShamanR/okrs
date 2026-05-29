package krs

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Post("/goals/{goalID}/key-results", h.HandleCreateKeyResult)
	r.Post("/krs/{krID}/progress/percent", h.HandleUpdatePercentProgress)
	r.Post("/krs/{krID}/progress/boolean", h.HandleUpdateBooleanProgress)
	r.Post("/krs/{krID}/progress/project", h.HandleUpdateProjectProgress)
	r.Post("/krs/{krID}/note", h.HandleUpsertKRNote)
	r.Post("/krs/{krID}", h.HandleUpdateKeyResult)
	r.Post("/krs/{krID}/move-up", h.HandleMoveKeyResultUp)
	r.Post("/krs/{krID}/move-down", h.HandleMoveKeyResultDown)
}
