// Package system implements the system-admin provisioning API (/api/v1/system/*).
// These endpoints are cross-tenant by design and gated by auth.RequireSystemAdminMiddleware;
// they take the tenant id from the URL, never from request context.
package system

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"okrs/internal/domain"
	"okrs/internal/store/tenants"

	"github.com/go-chi/chi/v5"
)

// Provisioner is the control surface; *service.ProvisioningService satisfies it.
type Provisioner interface {
	CreateTenant(ctx context.Context, name, slug string) (*domain.Tenant, error)
	AttachMember(ctx context.Context, tenantID, userID int64, role domain.Role) (*domain.Membership, error)
	SetEntitlements(ctx context.Context, tenantID int64, entitlements map[string]any) error
	Suspend(ctx context.Context, tenantID int64) error
	Restore(ctx context.Context, tenantID int64) error
}

// SystemSettings writes global instance settings; *service.SettingsService satisfies it.
type SystemSettings interface {
	SystemSet(ctx context.Context, key string, value any) error
}

// UserLister returns the global (cross-tenant) user list; *store.UserRepository satisfies it.
type UserLister interface {
	ListUsers(ctx context.Context) ([]*domain.User, error)
}

// TenantLister returns all tenants; *store.TenantRepository satisfies it.
type TenantLister interface {
	List(ctx context.Context) ([]domain.Tenant, error)
}

type Handler struct {
	prov     Provisioner
	settings SystemSettings
	users    UserLister
	tenants  TenantLister
}

func New(prov Provisioner, settings SystemSettings, users UserLister, tenantsList TenantLister) *Handler {
	return &Handler{prov: prov, settings: settings, users: users, tenants: tenantsList}
}

type tenantDTO struct {
	ID     int64  `json:"id"`
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

func toTenantDTO(t *domain.Tenant) tenantDTO {
	return tenantDTO{ID: t.ID, Slug: t.Slug, Name: t.Name, Status: string(t.Status)}
}

// POST /api/v1/system/tenants  {name, slug, entitlements?}
func (h *Handler) HandleCreateTenant(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name         string         `json:"name"`
		Slug         string         `json:"slug"`
		Entitlements map[string]any `json:"entitlements"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	tn, err := h.prov.CreateTenant(r.Context(), body.Name, body.Slug)
	if err != nil {
		switch {
		case errors.Is(err, tenants.ErrInvalidSlug):
			writeError(w, http.StatusUnprocessableEntity, "invalid slug")
		case errors.Is(err, tenants.ErrSlugTaken):
			writeError(w, http.StatusConflict, "slug already taken")
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	if len(body.Entitlements) > 0 {
		if err := h.prov.SetEntitlements(r.Context(), tn.ID, body.Entitlements); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, toTenantDTO(tn))
}

// GET /api/v1/system/tenants
func (h *Handler) HandleListTenants(w http.ResponseWriter, r *http.Request) {
	list, err := h.tenants.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]tenantDTO, 0, len(list))
	for i := range list {
		out = append(out, toTenantDTO(&list[i]))
	}
	writeJSON(w, out)
}

// POST /api/v1/system/tenants/{id}/members  {user_id, role}
func (h *Handler) HandleAttachMember(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		UserID int64  `json:"user_id"`
		Role   string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.UserID == 0 {
		writeError(w, http.StatusBadRequest, "user_id required")
		return
	}
	role := domain.Role(body.Role)
	if role != domain.RoleAdmin && role != domain.RoleUser {
		role = domain.RoleUser
	}
	m, err := h.prov.AttachMember(r.Context(), tenantID, body.UserID, role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{
		"user_id": m.UserID, "tenant_id": m.TenantID, "role": string(m.Role), "status": string(m.Status),
	})
}

// PUT /api/v1/system/tenants/{id}/entitlements  { "sso": true, "max_users": 50 }
func (h *Handler) HandleSetEntitlements(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := pathID(w, r)
	if !ok {
		return
	}
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.prov.SetEntitlements(r.Context(), tenantID, body); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/system/tenants/{id}/suspend
func (h *Handler) HandleSuspend(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, h.prov.Suspend)
}

// POST /api/v1/system/tenants/{id}/restore
func (h *Handler) HandleRestore(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, h.prov.Restore)
}

func (h *Handler) transition(w http.ResponseWriter, r *http.Request, fn func(context.Context, int64) error) {
	tenantID, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := fn(r.Context(), tenantID); err != nil {
		if errors.Is(err, tenants.ErrNotFound) {
			writeError(w, http.StatusNotFound, "tenant not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/v1/system/users
func (h *Handler) HandleListUsers(w http.ResponseWriter, r *http.Request) {
	list, err := h.users.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	type userDTO struct {
		ID            int64  `json:"id"`
		DisplayName   string `json:"display_name"`
		Email         string `json:"email"`
		IsSystemAdmin bool   `json:"is_system_admin"`
	}
	out := make([]userDTO, 0, len(list))
	for _, u := range list {
		out = append(out, userDTO{ID: u.ID, DisplayName: u.DisplayName, Email: u.Email, IsSystemAdmin: u.IsSystemAdmin})
	}
	writeJSON(w, out)
}

// PUT /api/v1/system/settings/default-registration-tenant  {tenant_id|null}
func (h *Handler) HandleSetDefaultRegistrationTenant(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TenantID *int64 `json:"tenant_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.settings.SystemSet(r.Context(), "default_registration_tenant_id", body.TenantID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid tenant id")
		return 0, false
	}
	return id, true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
