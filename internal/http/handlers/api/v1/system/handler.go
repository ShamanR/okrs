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
	"time"

	"okrs/internal/auth"
	"okrs/internal/domain"
	"okrs/internal/service"
	"okrs/internal/store/memberships"
	"okrs/internal/store/tenants"
	"okrs/internal/store/users"

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
type activityPurger interface {
	Purge(ctx context.Context, scope domain.TenantScope, olderThan *time.Time) (int64, error)
}

type Handler struct {
	prov     Provisioner
	settings SystemSettings
	users    UserLister
	tenants  TenantLister
	members  MemberLister
	activity activityPurger
}

func New(prov Provisioner, settings SystemSettings, users UserLister, tenantsList TenantLister, members MemberLister, activity activityPurger) *Handler {
	return &Handler{prov: prov, settings: settings, users: users, tenants: tenantsList, members: members, activity: activity}
}

// purgeCutoff maps a purge depth to a cutoff time; "all" → (nil, true) meaning delete all;
// ok=false for an unknown depth.
func purgeCutoff(depth string) (t *time.Time, ok bool) {
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

// HandlePurgeActivity handles POST /api/v1/system/tenants/{id}/activity/purge.
// Body: {"older_than":"quarter"|"year"|"all"}. System-admin authority is enforced by
// RequireSystemAdminMiddleware on the route group; the tenant id comes from the path.
func (h *Handler) HandlePurgeActivity(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		OlderThan string `json:"older_than"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	cutoff, ok := purgeCutoff(body.OlderThan)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "invalid older_than")
		return
	}
	deleted, err := h.activity.Purge(r.Context(), domain.TenantScope{TenantID: tenantID}, cutoff)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"deleted": deleted})
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

// PATCH /api/v1/system/tenants/{id}  {name, slug}
func (h *Handler) HandlePatchTenant(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	tn, err := h.prov.UpdateTenant(r.Context(), tenantID, body.Name, body.Slug)
	if err != nil {
		switch {
		case errors.Is(err, tenants.ErrNotFound):
			writeError(w, http.StatusNotFound, "tenant not found")
		case errors.Is(err, tenants.ErrSlugTaken):
			writeError(w, http.StatusConflict, "slug already taken")
		case errors.Is(err, tenants.ErrInvalidSlug):
			writeError(w, http.StatusUnprocessableEntity, "invalid slug")
		case errors.Is(err, tenants.ErrInvalidName):
			writeError(w, http.StatusUnprocessableEntity, "invalid name")
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, toTenantDTO(tn))
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

// PUT /api/v1/system/tenants/{id}/members/{userID}/role  {role}
func (h *Handler) HandleSetMemberRole(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := pathID(w, r)
	if !ok {
		return
	}
	userID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	if err != nil || userID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var body struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	role := domain.Role(body.Role)
	if role != domain.RoleAdmin && role != domain.RoleUser {
		writeError(w, http.StatusUnprocessableEntity, "invalid role")
		return
	}
	switch err := h.prov.SetMemberRole(r.Context(), tenantID, userID, role); {
	case errors.Is(err, memberships.ErrNotFound):
		writeError(w, http.StatusNotFound, "membership not found")
	case errors.Is(err, service.ErrLastAdmin):
		writeError(w, http.StatusConflict, "cannot demote the last admin")
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error())
	default:
		w.WriteHeader(http.StatusNoContent)
	}
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

// PUT /api/v1/system/users/{userID}/system-admin  {is_system_admin}
func (h *Handler) HandleSetSystemAdmin(w http.ResponseWriter, r *http.Request) {
	userID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	if err != nil || userID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var body struct {
		IsSystemAdmin bool `json:"is_system_admin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	var callerID int64
	if u := auth.UserFromContext(r.Context()); u != nil {
		callerID = u.ID
	}
	switch err := h.prov.SetSystemAdmin(r.Context(), callerID, userID, body.IsSystemAdmin); {
	case errors.Is(err, users.ErrNotFound):
		writeError(w, http.StatusNotFound, "user not found")
	case errors.Is(err, service.ErrLastSystemAdmin):
		writeError(w, http.StatusConflict, "cannot revoke the last system admin")
	case errors.Is(err, service.ErrSelfLockout):
		writeError(w, http.StatusConflict, "cannot revoke your own system-admin")
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error())
	default:
		w.WriteHeader(http.StatusNoContent)
	}
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

// GET /api/v1/system/tenants/{id}/members
func (h *Handler) HandleListMembers(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	list, err := h.members.ListByTenant(r.Context(), domain.TenantScope{TenantID: id})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, m := range list {
		out = append(out, map[string]any{
			"user_id": m.UserID, "display_name": m.DisplayName, "email": m.Email,
			"role": string(m.Role), "status": string(m.Status),
		})
	}
	writeJSON(w, out)
}

// POST /api/v1/system/tenants/{id}/members/{userID}/deny
func (h *Handler) HandleDenyMember(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	userID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	if err != nil || userID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if err := h.prov.DenyMember(r.Context(), id, userID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /api/v1/system/tenants/{id}/members/{userID}
func (h *Handler) HandleRemoveMember(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	userID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	if err != nil || userID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if err := h.prov.RemoveMember(r.Context(), id, userID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/v1/system/tenants/{id}/entitlements
func (h *Handler) HandleGetEntitlements(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ent, err := h.settings.TenantEntitlements(r.Context(), domain.TenantScope{TenantID: id})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ent == nil {
		ent = map[string]json.RawMessage{}
	}
	writeJSON(w, ent)
}

// GET /api/v1/system/settings
func (h *Handler) HandleGetSettings(w http.ResponseWriter, r *http.Request) {
	raw, err := h.settings.SystemGet(r.Context(), "default_registration_tenant_id")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var tenantID *int64
	if raw != nil {
		_ = json.Unmarshal(raw, &tenantID)
	}
	msgRaw, err := h.settings.SystemGet(r.Context(), "no_access_message")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var msg string
	if msgRaw != nil {
		_ = json.Unmarshal(msgRaw, &msg)
	}
	writeJSON(w, map[string]any{
		"default_registration_tenant_id": tenantID,
		"no_access_message":              msg,
	})
}

// PUT /api/v1/system/settings/no-access-message  {message}
func (h *Handler) HandleSetNoAccessMessage(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.settings.SystemSet(r.Context(), "no_access_message", body.Message); err != nil {
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
