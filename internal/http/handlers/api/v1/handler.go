package v1

import (
	"okrs/internal/service"
)

type Handler struct {
	service *service.Service
}

const maxMultipartMemory = 32 << 20

func NewHandler(service *service.Service) *Handler {
	return &Handler{service: service}
}
