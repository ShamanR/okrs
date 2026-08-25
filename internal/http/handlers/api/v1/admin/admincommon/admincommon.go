// Package admincommon holds what the /api/v1/admin/** endpoints share: the tenant
// weight-tolerance lookup and the admin-panel team shape. A leaf package for the same
// reason as goalcommon — the parent mounts the sub-packages.
package admincommon

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"context"
	"encoding/json"

	"time"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/web/common"
	hcsvc "okrs/internal/service/healthcheckin"
	"okrs/internal/store/grants"
	"okrs/internal/store/users"

	"github.com/go-chi/chi/v5"
)

// SettingsReader is the settings port the health-checkin config loader needs;
// *settings.Service satisfies it.
type SettingsReader = hcsvc.SettingsReader

// WeightTolerance loads the tenant's health-checkin weight tolerance (defaults to 0).
// Shared by every admin endpoint that renders period aggregates.
func WeightTolerance(r *http.Request, settings SettingsReader, scope domain.TenantScope) int {
	if settings == nil {
		return 0
	}
	cfg, err := hcsvc.LoadConfig(r.Context(), scope, settings)
	if err != nil {
		return 0
	}
	return cfg.WeightTolerance
}

// TeamRow is the admin-panel shape of a team.
type TeamRow struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	TypeLabel   string  `json:"type_label"`
	ParentID    *int64  `json:"parent_id"`
	Lead        string  `json:"lead"`
	LeadUDID    *string `json:"lead_udid,omitempty"`
	Description string  `json:"description"`
	DeletedAt   *string `json:"deleted_at,omitempty"`
}

func MapTeamRow(t domain.Team) TeamRow {
	var deletedAt *string
	if t.DeletedAt != nil {
		s := t.DeletedAt.Format("2006-01-02")
		deletedAt = &s
	}
	return TeamRow{
		ID:          t.ID,
		Name:        t.Name,
		Type:        string(t.Type),
		TypeLabel:   common.TeamTypeLabel(t.Type),
		ParentID:    t.ParentID,
		Lead:        t.Lead,
		LeadUDID:    t.LeadUDID,
		Description: t.Description,
		DeletedAt:   deletedAt,
	}
}

// — Порты, которые admin-эндпоинты получают вместо конкретных типов. —

// userAdminStore covers user operations. *store.UserRepository satisfies it.
type UserAdminStore interface {
	ListUsers(ctx context.Context) ([]*domain.User, error)
	ListByTenant(ctx context.Context, scope domain.TenantScope) ([]users.TenantUser, error)
	GetUser(ctx context.Context, id int64) (*domain.User, error)
}

// tenantSettings covers per-tenant product settings. *service.SettingsService satisfies it.
// Writes go through the product path, which rejects entitlement.* keys.
type TenantSettings interface {
	GetTenant(ctx context.Context, scope domain.TenantScope, key string) (json.RawMessage, error)
	SetTenantProduct(ctx context.Context, scope domain.TenantScope, key string, value any) error
}

// grantsStore covers the user_hierarchy_grants operations. *store.GrantsCache satisfies it.
type GrantsStore interface {
	ListUserGrants(ctx context.Context, scope domain.TenantScope, userID int64) ([]grants.HierarchyGrant, error)
	AllGrants(ctx context.Context) (map[int64][]grants.HierarchyGrant, error)
	ListDescendantTeamIDs(ctx context.Context, scope domain.TenantScope, rootIDs []int64) ([]int64, error)
	AddUserGrant(ctx context.Context, scope domain.TenantScope, userID, teamID, grantedByUserID int64) error
	RemoveUserGrant(ctx context.Context, scope domain.TenantScope, userID, teamID int64) error
}

// memberRoleSetter toggles a user's tenant-scoped role (admin/user). *service.OnboardingService
// satisfies it. Admin status is tenant-scoped, so the toggle writes memberships.role.
type MemberRoleSetter interface {
	SetMemberRole(ctx context.Context, scope domain.TenantScope, userID int64, role domain.Role) error
}

// tenantRenamer renames the active tenant. *service.ProvisioningService satisfies it.
type TenantRenamer interface {
	RenameTenant(ctx context.Context, id int64, name string) error
}

// activityPurger deletes journal rows for the active tenant. *service.Service satisfies it.
type ActivityPurger interface {
	Purge(ctx context.Context, scope domain.TenantScope, olderThan *time.Time) (int64, error)
}

// — Локальные помощники admin-панели: её формат ответа проще общего v1. —

func ParseID(r *http.Request, param string) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, param), 10, 64)
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

// — Ключи настроек и типизированные аксессоры к ним. —

const SettingKeyDocumentationURL = "documentation_url"

const (
	SettingKeyFeedbackURL             = "feedback_url"
	SettingKeyFeedbackPopupEnabled    = "feedback_popup_enabled"
	SettingKeyFeedbackMenuLinkEnabled = "feedback_menu_link_enabled"
	SettingKeyFeedbackFrequencyDays   = "feedback_frequency_days"
	SettingKeyEmptyHierarchyMessage   = "empty_hierarchy_message"
	// SettingKeyProgressSnapshotIntervalDays controls how often the background job records
	// a per-team progress point for the period chart (in days, ≥1; 1 = daily).
	SettingKeyProgressSnapshotIntervalDays = "progress_snapshot_interval_days"
)

// PurgeCutoff maps a purge depth to a cutoff time. "all" returns (nil, true), meaning
// delete everything. ok=false for an unknown depth.
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

// HasUnsafeURLScheme reports whether s uses a scheme that can execute script
// when placed in an href (javascript:, data:, vbscript:). Used where any link
// shape is allowed but rendered-href XSS must still be prevented.
func HasUnsafeURLScheme(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	for _, scheme := range []string{"javascript:", "data:", "vbscript:"} {
		if strings.HasPrefix(lower, scheme) {
			return true
		}
	}
	return false
}

// IsValidHTTPURL reports whether s is an absolute http(s) URL. Restricting the
// scheme keeps unsafe values (e.g. javascript:) out of a rendered href.
func IsValidHTTPURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// SettingString reads a string setting; empty when unset or malformed.
func SettingString(ctx context.Context, settings TenantSettings, scope domain.TenantScope, key string) string {
	raw, err := settings.GetTenant(ctx, scope, key)
	if err != nil || raw == nil {
		return ""
	}
	var s string
	_ = json.Unmarshal(raw, &s)
	return s
}

// SettingBool reads a bool setting; false when unset or malformed.
func SettingBool(ctx context.Context, settings TenantSettings, scope domain.TenantScope, key string) bool {
	raw, err := settings.GetTenant(ctx, scope, key)
	if err != nil || raw == nil {
		return false
	}
	var b bool
	_ = json.Unmarshal(raw, &b)
	return b
}

// SettingInt reads an int setting; returns def when unset, malformed, or < 1.
func SettingInt(ctx context.Context, settings TenantSettings, scope domain.TenantScope, key string, def int) int {
	raw, err := settings.GetTenant(ctx, scope, key)
	if err != nil || raw == nil {
		return def
	}
	var n int
	if json.Unmarshal(raw, &n) != nil || n < 1 {
		return def
	}
	return n
}

// SetMemberRole applies a tenant-scoped role change to the target member.
// SetMemberRole is the body behind both POST and DELETE on …/users/{userID}/admin:
// the two verbs differ only by the role they assign.
func SetMemberRole(w http.ResponseWriter, r *http.Request, roles MemberRoleSetter, role domain.Role) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		WriteError(w, http.StatusForbidden, "no active tenant")
		return
	}
	userID, err := ParseID(r, "userID")
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if err := roles.SetMemberRole(r.Context(), scope, userID, role); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
