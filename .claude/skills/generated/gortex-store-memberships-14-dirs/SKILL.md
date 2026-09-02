---
name: gortex-store-memberships-14-dirs
description: "Work in the store/memberships +14 dirs area — 660 symbols across 25 files (77% cohesion)"
---

# store/memberships +14 dirs

660 symbols | 25 files | 77% cohesion

## When to Use

Use this skill when working on files in:
- ``
- `internal/core/domain/tenant.go`
- `internal/http/handlers/api/v1/admin/invitations/revoke/handler.go`
- `internal/http/handlers/api/v1/onboarding/handler_test.go`
- `internal/http/handlers/api/v1/system/handler_test.go`
- `internal/http/handlers/api/v1/system/settings/handler.go`
- `internal/http/server.go`
- `internal/service/onboarding/onboarding.go`
- `internal/service/onboarding/onboarding_test.go`
- `internal/service/provisioning/provisioning.go`
- `internal/service/provisioning/provisioning_test.go`
- `internal/service/settings/settings.go`
- `internal/service/settings/settings_test.go`
- `internal/store/grants/grants.go`
- `internal/store/memberships/cache.go`
- `internal/store/memberships/memberships.go`
- `internal/store/memberships/memberships_test.go`
- `internal/store/settings/cache.go`
- `internal/store/settings/cache_test.go`
- `internal/store/settings/settings.go`
- `internal/store/tenants/cache.go`
- `internal/store/tenants/tenants.go`
- `internal/store/tenantsettings/cache.go`
- `internal/store/tenantsettings/cache_test.go`
- `internal/store/tenantsettings/tenantsettings.go`

## Key Files

| File | Symbols |
|------|---------|
| `` | TrimPrefix |
| `internal/core/domain/tenant.go` | TenantStatus |
| `internal/http/handlers/api/v1/admin/invitations/revoke/handler.go` | New, invites |
| `internal/http/handlers/api/v1/onboarding/handler_test.go` | cleanup, tsRepo, r, closure@59, settingsSvc, ... |
| `internal/http/handlers/api/v1/system/handler_test.go` | tsRepo, pool, user, memRepo, grantsCache, ... |
| `internal/http/handlers/api/v1/system/settings/handler.go` | New, settings |
| `internal/http/server.go` | provisioning, hcCache, resolver, logger, notifChannels, ... |
| `internal/service/onboarding/onboarding.go` | slug, userID, scope, existing, ListAccessRequests, ... |
| `internal/service/onboarding/onboarding_test.go` | cleanup, scope, memRepo, err, TestApproveRequestAppliesDefaultAccess, ... |
| `internal/service/provisioning/provisioning.go` | memberCache, Suspend, tenantID, name, entitlements, ... |
| `internal/service/provisioning/provisioning_test.go` | tsRepo, got, sysRepo, closure@211, err, ... |
| `internal/service/settings/settings.go` | err, v, scope, SystemSet, tsRepo, ... |
| `internal/service/settings/settings_test.go` | err, err, ok, tsRepo, scope, ... |
| `internal/store/grants/grants.go` | db, GrantRepository, db, NewGrantRepository |
| `internal/store/memberships/cache.go` | r, InvalidateUser, userID, NewMembershipCache |
| `internal/store/memberships/memberships.go` | DeleteRequested, userID, userID, role, ct, ... |
| `internal/store/memberships/memberships_test.go` | scope, TestCountActiveAdmins, seat, closure@42, t, ... |
| `internal/store/settings/cache.go` | newSystemSettingsCacheWithBackend, repo, NewSystemSettingsCache, b, ttl, ... |
| `internal/store/settings/cache_test.go` | data, fakeSysBackend, ListAll, calls |
| `internal/store/settings/settings.go` | SettingsRepository, db, db, NewSettingsRepository |
| `internal/store/tenants/cache.go` | ok, err, e, ctx, slug, ... |
| `internal/store/tenants/tenants.go` | db, id, NewTenantRepository, GetByID, ctx |
| `internal/store/tenantsettings/cache.go` | mu, entries, b, Invalidate, backend, ... |
| `internal/store/tenantsettings/cache_test.go` | c, err, TestTenantSettingsCacheCachesPerTenant, err, ctx, ... |
| `internal/store/tenantsettings/tenantsettings.go` | NewTenantSettingsRepository, db |

## Connected Communities

- **usecase/goal +36 dirs** (46 cross-edges)
- **store/memberships +4 dirs** (14 cross-edges)
- **auth +32 dirs** (13 cross-edges)
- **auth +67 dirs** (11 cross-edges)
- **v1/goals +9 dirs** (11 cross-edges)
- **store · TestSearchUsersInSet** (8 cross-edges)
- **system/notificationchannels +16 dirs** (7 cross-edges)
- **http/handlers · URLParam** (6 cross-edges)
- **service/servicetest +33 dirs** (4 cross-edges)
- **service/notificationchannel +10 dirs** (4 cross-edges)
- **api/v1 · WriteError** (4 cross-edges)
- **. +4 dirs · specURIs** (4 cross-edges)
- **store/invitations +3 dirs** (4 cross-edges)
- **service/goal +8 dirs** (3 cross-edges)
- **core/progress +22 dirs** (3 cross-edges)
- **store/tenantsettings · TestTenantSettingsScopedByTenant** (3 cross-edges)
- **v1/system · Put** (2 cross-edges)
- **accessrequests/deny** (2 cross-edges)
- **service/activity +86 dirs** (2 cross-edges)
- **activity/purge +12 dirs** (2 cross-edges)
- **usecase/okrboard +3 dirs** (2 cross-edges)
- **admin/members** (2 cross-edges)
- **auth · Resolve** (2 cross-edges)
- **store/tenants +2 dirs** (2 cross-edges)
- **settings/noaccessmessage** (2 cross-edges)
- **store/memberships +8 dirs** (2 cross-edges)
- **store/users · TestSystemAdminCountAndSet** (1 cross-edges)
- **service/activity +61 dirs** (1 cross-edges)
- **service/healthcheckin +3 dirs** (1 cross-edges)
- **tenants/suspend** (1 cross-edges)
- **auth +4 dirs · TestUsersEndpoint_ScopedSearch_…** (1 cross-edges)
- **store/tenants · TestTenantRepositoryCreateAndGet** (1 cross-edges)
- **auth +3 dirs** (1 cross-edges)
- **. +4 dirs · ListByUser** (1 cross-edges)
- **admin/invitations** (1 cross-edges)
- **http +1 dirs · renderShell** (1 cross-edges)
- **store/settings · TestSettingsJsonTypes** (1 cross-edges)
- **auth +4 dirs · fillGoalRefProgress** (1 cross-edges)
- **tenants/entitlements** (1 cross-edges)
- **accessrequests/approve** (1 cross-edges)
- **service/healthcheckin +6 dirs** (1 cross-edges)
- **store/tenants · getBy** (1 cross-edges)
- **store/tenants · TestTenantRename** (1 cross-edges)

## How to Explore

```
analyze(operation:"communities", id:"community-145")
explore(operation:"context", task:"understand store/memberships +14 dirs", format:"gcx")
```

_`format: "gcx"` returns the [GCX1 compact wire format](../../docs/wire-format.md) — round-trippable, ~27% fewer tokens than JSON. Drop it for JSON output; agents using `@gortex/wire` or the Go `github.com/gortexhq/gcx-go` package decode either._
