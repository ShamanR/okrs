// Package onboardingcommon holds the ports and helpers the invitation, access-request
// and membership endpoints share. A leaf package, same reason as admincommon.
package onboardingcommon

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/store/memberships"

	"github.com/go-chi/chi/v5"
)

// invitationStore covers tenant invitation persistence. *store.InvitationRepository satisfies it.
type InvitationStore interface {
	Create(ctx context.Context, scope domain.TenantScope, role domain.Role, tokenHash string, createdBy int64, maxUses *int, expiresAt *time.Time) (*domain.Invitation, error)
	ListPendingByTenant(ctx context.Context, scope domain.TenantScope) ([]domain.Invitation, error)
	Revoke(ctx context.Context, scope domain.TenantScope, id int64) error
}

// onboardService covers the join-request + access-request flows. *service.OnboardingService satisfies it.
type OnboardService interface {
	RequestAccess(ctx context.Context, slug string, userID int64) error
	ListAccessRequests(ctx context.Context, scope domain.TenantScope) ([]memberships.AccessRequest, error)
	ApproveRequest(ctx context.Context, scope domain.TenantScope, userID int64) error
	DenyRequest(ctx context.Context, scope domain.TenantScope, userID int64) error
	RemoveMember(ctx context.Context, scope domain.TenantScope, userID int64) error
}

// AccessRequestAction is the body behind approve/deny/remove-member: each takes the
// user id from the path and applies one onboarding operation to it. Lives in this leaf
// package so all three endpoint packages can share it without an import cycle.
func AccessRequestAction(w http.ResponseWriter, r *http.Request, fn func(context.Context, domain.TenantScope, int64) error) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		WriteError(w, http.StatusForbidden, "no active tenant")
		return
	}
	userID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	if err != nil || userID <= 0 {
		WriteError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if err := fn(r.Context(), scope, userID); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func WriteError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
func WriteJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// InviteBaseURL returns the scheme+host to build invite links against. An explicitly configured
// baseURL (AUTH_BASE_URL) wins as an operator override; otherwise it is derived from the request
// so links point at the domain the app is actually served on, honoring the X-Forwarded-* headers
// set by an ingress/reverse proxy.
// InviteBaseURL builds the absolute base for invite links: the configured BaseURL when
// set, otherwise reconstructed from the request and its X-Forwarded-* headers.
func InviteBaseURL(r *http.Request, baseURL string) string {
	if baseURL != "" {
		return strings.TrimRight(baseURL, "/")
	}
	scheme := "http"
	if proto := FirstForwardedValue(r.Header.Get("X-Forwarded-Proto")); proto != "" {
		scheme = proto
	} else if r.TLS != nil {
		scheme = "https"
	}
	host := r.Host
	if fwd := FirstForwardedValue(r.Header.Get("X-Forwarded-Host")); fwd != "" {
		host = fwd
	}
	return scheme + "://" + host
}

// FirstForwardedValue returns the first entry of a possibly comma-separated X-Forwarded-* header.
func FirstForwardedValue(v string) string {
	if v == "" {
		return ""
	}
	if i := strings.IndexByte(v, ','); i >= 0 {
		v = v[:i]
	}
	return strings.TrimSpace(v)
}
