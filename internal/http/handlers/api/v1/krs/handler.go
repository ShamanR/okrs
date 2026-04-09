package krs

import (
	"net/http"

	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/service"
)

type Handler struct {
	core *v1.Handler
}

func New(service *service.Service) *Handler {
	return &Handler{core: v1.NewHandler(service)}
}

func (h *Handler) HandleUpdatePercentProgress(w http.ResponseWriter, r *http.Request) {
	h.core.HandleUpdatePercentProgress()(w, r)
}

func (h *Handler) HandleUpdateBooleanProgress(w http.ResponseWriter, r *http.Request) {
	h.core.HandleUpdateBooleanProgress()(w, r)
}

func (h *Handler) HandleUpdateProjectProgress(w http.ResponseWriter, r *http.Request) {
	h.core.HandleUpdateProjectProgress()(w, r)
}

func (h *Handler) HandleAddKRComment(w http.ResponseWriter, r *http.Request) {
	h.core.HandleAddKRComment()(w, r)
}

func (h *Handler) HandleUpdateKeyResult(w http.ResponseWriter, r *http.Request) {
	h.core.HandleUpdateKeyResult()(w, r)
}

func (h *Handler) HandleMoveKeyResultUp(w http.ResponseWriter, r *http.Request) {
	h.core.HandleMoveKeyResultUp()(w, r)
}

func (h *Handler) HandleMoveKeyResultDown(w http.ResponseWriter, r *http.Request) {
	h.core.HandleMoveKeyResultDown()(w, r)
}
