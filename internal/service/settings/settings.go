package settings

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"okrs/internal/core/domain"
	"okrs/internal/store/settings"
	"okrs/internal/store/tenantsettings"
)

// EntitlementPrefix marks tenant_settings keys writable only by system-admin/provisioning.
const EntitlementPrefix = "entitlement."

// ErrEntitlementNamespace is returned when a write targets the wrong namespace for its
// authority: a tenant-admin product write touching entitlement.*, or an entitlement write
// missing the entitlement.* prefix.
var ErrEntitlementNamespace = errors.New("settings: entitlement.* is system-admin only")

// Service reads settings from per-tenant / global snapshot caches and enforces
// per-namespace write-authority, invalidating the relevant cache after each write.
type Service struct {
	tsCache  *tenantsettings.TenantSettingsCache
	tsRepo   *tenantsettings.TenantSettingsRepository
	sysCache *settings.SystemSettingsCache
	sysRepo  *settings.SettingsRepository
}

func New(
	tsCache *tenantsettings.TenantSettingsCache,
	tsRepo *tenantsettings.TenantSettingsRepository,
	sysCache *settings.SystemSettingsCache,
	sysRepo *settings.SettingsRepository,
) *Service {
	return &Service{tsCache: tsCache, tsRepo: tsRepo, sysCache: sysCache, sysRepo: sysRepo}
}

// TenantSnapshot returns the cached snapshot of all keys for a tenant.
func (s *Service) TenantSnapshot(ctx context.Context, scope domain.TenantScope) (map[string]json.RawMessage, error) {
	return s.tsCache.GetAll(ctx, scope)
}

// GetTenant looks a key up in the cached tenant snapshot (nil if unset).
func (s *Service) GetTenant(ctx context.Context, scope domain.TenantScope, key string) (json.RawMessage, error) {
	snap, err := s.tsCache.GetAll(ctx, scope)
	if err != nil {
		return nil, err
	}
	return snap[key], nil
}

// TenantEntitlements returns the tenant's entitlement.* keys with the prefix stripped.
func (s *Service) TenantEntitlements(ctx context.Context, scope domain.TenantScope) (map[string]json.RawMessage, error) {
	snap, err := s.tsCache.GetAll(ctx, scope)
	if err != nil {
		return nil, err
	}
	out := make(map[string]json.RawMessage)
	for k, v := range snap {
		if strings.HasPrefix(k, EntitlementPrefix) {
			out[strings.TrimPrefix(k, EntitlementPrefix)] = v
		}
	}
	return out, nil
}

// SetTenantProduct writes a tenant-admin product key. entitlement.* is rejected.
func (s *Service) SetTenantProduct(ctx context.Context, scope domain.TenantScope, key string, value any) error {
	if strings.HasPrefix(key, EntitlementPrefix) {
		return ErrEntitlementNamespace
	}
	if err := s.tsRepo.Set(ctx, scope, key, value); err != nil {
		return err
	}
	s.tsCache.Invalidate(scope.TenantID)
	return nil
}

// SetTenantEntitlement writes an entitlement.* key (system-admin/provisioning path).
func (s *Service) SetTenantEntitlement(ctx context.Context, scope domain.TenantScope, key string, value any) error {
	if !strings.HasPrefix(key, EntitlementPrefix) {
		return ErrEntitlementNamespace
	}
	if err := s.tsRepo.Set(ctx, scope, key, value); err != nil {
		return err
	}
	s.tsCache.Invalidate(scope.TenantID)
	return nil
}

// SystemGet looks a global system key up in the cached global snapshot.
func (s *Service) SystemGet(ctx context.Context, key string) (json.RawMessage, error) {
	return s.sysCache.Get(ctx, key)
}

// SystemSet writes a global system key and invalidates the global snapshot.
func (s *Service) SystemSet(ctx context.Context, key string, value any) error {
	if err := s.sysRepo.SetSetting(ctx, key, value); err != nil {
		return err
	}
	s.sysCache.Invalidate()
	return nil
}
