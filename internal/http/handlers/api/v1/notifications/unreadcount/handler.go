// Package unreadcount serves GET /api/v1/notifications/unread-count — the bell badge.
package unreadcount

import (
	"context"
	"net/http"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/http/dto"
	v1 "okrs/internal/http/handlers/api/v1"
)

// UnreadCounter is the port this handler needs. *notification.Service satisfies it.
type UnreadCounter interface {
	UnreadCount(ctx context.Context, scope domain.TenantScope, userID int64) (int, error)
}

type Handler struct{ svc UnreadCounter }

func New(svc UnreadCounter) *Handler { return &Handler{svc: svc} }

// Get is polled every 60s by the sidebar. Deliberately uncached server-side: it is a
// COUNT over a partial index for one user, and caching it across K8s replicas would
// buy staleness rather than speed.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	v1.SetAPICacheControl(w)
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	n, err := h.svc.UnreadCount(r.Context(), scope, auth.UserIDFromContext(r.Context()))
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to count notifications", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, dto.UnreadCount{Count: n})
}
