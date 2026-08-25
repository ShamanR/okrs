// Package systemcommon holds the ports and helpers the /api/v1/system/** endpoints
// share. A leaf package for the same reason as admincommon — the parent mounts the
// sub-packages, so importing it back would be an import cycle.
package systemcommon

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/store/memberships"
	"okrs/internal/store/tenants"

	"github.com/go-chi/chi/v5"
)

// Provisioner is the control surface; *service.ProvisioningService satisfies it.
type Provisioner interface {
	CreateTenant(ctx context.Context, name, slug string) (*domain.Tenant, error)
	UpdateTenant(ctx context.Context, tenantID int64, name, slug string) (*domain.Tenant, error)
	AttachMember(ctx context.Context, tenantID, userID int64, role domain.Role) (*domain.Membership, error)
	SetEntitlements(ctx context.Context, tenantID int64, entitlements map[string]any) error
	Suspend(ctx context.Context, tenantID int64) error
	Restore(ctx context.Context, tenantID int64) error
	DenyMember(ctx context.Context, tenantID, userID int64) error
	RemoveMember(ctx context.Context, tenantID, userID int64) error
	SetMemberRole(ctx context.Context, tenantID, userID int64, role domain.Role) error
	SetSystemAdmin(ctx context.Context, callerID, targetID int64, isSystemAdmin bool) error
}

// SystemSettings reads/writes instance + tenant settings; *service.SettingsService satisfies it.
type SystemSettings interface {
	SystemSet(ctx context.Context, key string, value any) error
	SystemGet(ctx context.Context, key string) (json.RawMessage, error)
	TenantEntitlements(ctx context.Context, scope domain.TenantScope) (map[string]json.RawMessage, error)
}

// MemberLister lists a tenant's members; *store.MembershipRepository satisfies it.
type MemberLister interface {
	ListByTenant(ctx context.Context, scope domain.TenantScope) ([]memberships.AccessRequest, error)
}

// UserLister returns the global (cross-tenant) user list; *store.UserRepository satisfies it.
type UserLister interface {
	ListUsers(ctx context.Context) ([]*domain.User, error)
}

// TenantLister returns all tenants; *store.TenantRepository satisfies it.
type TenantLister interface {
	List(ctx context.Context) ([]domain.Tenant, error)
}

// activityPurger deletes journal rows for a tenant. *store.ActivityRepository satisfies it.
type ActivityPurger interface {
	Purge(ctx context.Context, scope domain.TenantScope, olderThan *time.Time) (int64, error)
}

// purgeCutoff maps a purge depth to a cutoff time; "all" → (nil, true) meaning delete all;
// ok=false for an unknown depth.
type TenantDTO struct {
	ID     int64  `json:"id"`
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

func PurgeCutoff(depth string) (t *time.Time, ok bool) {
	now := time.Now()
	switch depth {
	case "quarter":
		c := now.AddDate(0, -3, 0)
		return &c, true
	case "year":
		c := now.AddDate(0, -12, 0)
		return &c, true
	case "all":
		return nil, true
	default:
		return nil, false
	}
}
func ToTenantDTO(t *domain.Tenant) TenantDTO {
	return TenantDTO{ID: t.ID, Slug: t.Slug, Name: t.Name, Status: string(t.Status)}
}
func PathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		WriteError(w, http.StatusBadRequest, "invalid tenant id")
		return 0, false
	}
	return id, true
}
func WriteJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func WriteError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// Transition applies a tenant status change (suspend/restore); the two endpoints
// differ only by the target status.
func Transition(w http.ResponseWriter, r *http.Request, prov Provisioner, fn func(context.Context, int64) error) {
	tenantID, ok := PathID(w, r)
	if !ok {
		return
	}
	if err := fn(r.Context(), tenantID); err != nil {
		if errors.Is(err, tenants.ErrNotFound) {
			WriteError(w, http.StatusNotFound, "tenant not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
