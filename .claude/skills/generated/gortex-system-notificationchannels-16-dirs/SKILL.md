---
name: gortex-system-notificationchannels-16-dirs
description: "Work in the system/notificationchannels +16 dirs area — 230 symbols across 31 files (72% cohesion)"
---

# system/notificationchannels +16 dirs

230 symbols | 31 files | 72% cohesion

## When to Use

Use this skill when working on files in:
- ``
- `internal/auth/manager.go`
- `internal/core/domain/tenant.go`
- `internal/http/handlers/api/v1/admin/activity/purge/handler.go`
- `internal/http/handlers/api/v1/goals/goalcommon/goalcommon.go`
- `internal/http/handlers/api/v1/system/notificationchannels/handler.go`
- `internal/http/handlers/api/v1/system/notificationchannels/handler_test.go`
- `internal/http/handlers/api/v1/system/notificationchannels/routes.go`
- `internal/http/handlers/api/v1/system/settings/handler.go`
- `internal/http/handlers/api/v1/system/settings/handler_test.go`
- `internal/http/handlers/api/v1/system/settings/routes.go`
- `internal/http/handlers/api/v1/system/systemcommon/systemcommon.go`
- `internal/http/handlers/api/v1/system/tenants/activity/purge/handler.go`
- `internal/http/handlers/api/v1/system/tenants/activity/purge/handler_test.go`
- `internal/http/handlers/api/v1/system/tenants/activity/purge/routes.go`
- `internal/http/handlers/api/v1/system/tenants/entitlements/handler.go`
- `internal/http/handlers/api/v1/system/tenants/entitlements/handler_test.go`
- `internal/http/handlers/api/v1/system/tenants/entitlements/routes.go`
- `internal/http/handlers/api/v1/system/tenants/handler.go`
- `internal/http/handlers/api/v1/system/tenants/handler_test.go`
- `internal/http/handlers/api/v1/system/tenants/members/deny/handler.go`
- `internal/http/handlers/api/v1/system/tenants/members/deny/routes.go`
- `internal/http/handlers/api/v1/system/tenants/members/handler.go`
- `internal/http/handlers/api/v1/system/tenants/members/role/handler.go`
- `internal/http/handlers/api/v1/system/tenants/members/role/routes.go`
- `internal/http/handlers/api/v1/system/tenants/members/routes.go`
- `internal/http/handlers/api/v1/system/tenants/routes.go`
- `internal/http/handlers/api/v1/system/users/systemadmin/handler.go`
- `internal/http/handlers/api/v1/system/users/systemadmin/routes.go`
- `internal/http/server.go`
- `internal/platform/entitlements/entitlements_test.go`

## Key Files

| File | Symbols |
|------|---------|
| `` | ParseInt |
| `internal/auth/manager.go` | Config |
| `internal/core/domain/tenant.go` | Role |
| `internal/http/handlers/api/v1/admin/activity/purge/handler.go` | activity, New |
| `internal/http/handlers/api/v1/goals/goalcommon/goalcommon.go` | OptionalID, value, id, err |
| `internal/http/handlers/api/v1/system/notificationchannels/handler.go` | List, out, r, Title, d, ... |
| `internal/http/handlers/api/v1/system/notificationchannels/handler_test.go` | ds, Descriptors, fakeSvc |
| `internal/http/handlers/api/v1/system/notificationchannels/routes.go` | h, RegisterRoutes, r |
| `internal/http/handlers/api/v1/system/settings/handler.go` | w, err, err, Get, msgRaw, ... |
| `internal/http/handlers/api/v1/system/settings/handler_test.go` | key, SystemGet |
| `internal/http/handlers/api/v1/system/settings/routes.go` | h, RegisterRoutes, r |
| `internal/http/handlers/api/v1/system/systemcommon/systemcommon.go` | w, r, Name, w, id, ... |
| `internal/http/handlers/api/v1/system/tenants/activity/purge/handler.go` | w, tenantID, New, activity, Post, ... |
| `internal/http/handlers/api/v1/system/tenants/activity/purge/handler_test.go` | Purge, scope, olderThan |
| `internal/http/handlers/api/v1/system/tenants/activity/purge/routes.go` | h, r, RegisterRoutes |
| `internal/http/handlers/api/v1/system/tenants/entitlements/handler.go` | ok, err, ent, Put, err, ... |
| `internal/http/handlers/api/v1/system/tenants/entitlements/handler_test.go` | id, ent, SetEntitlements, TenantEntitlements |
| `internal/http/handlers/api/v1/system/tenants/entitlements/routes.go` | RegisterRoutes, h, r |
| `internal/http/handlers/api/v1/system/tenants/handler.go` | out, r, tn, err, tn, ... |
| `internal/http/handlers/api/v1/system/tenants/handler_test.go` | ent, CreateTenant, name, slug, SetEntitlements, ... |
| `internal/http/handlers/api/v1/system/tenants/members/deny/handler.go` | err, r, err, prov, ok, ... |
| `internal/http/handlers/api/v1/system/tenants/members/deny/routes.go` | RegisterRoutes, h, r |
| `internal/http/handlers/api/v1/system/tenants/members/handler.go` | err, userID, w, Get, ok, ... |
| `internal/http/handlers/api/v1/system/tenants/members/role/handler.go` | Handler, Put, err, prov, err, ... |
| `internal/http/handlers/api/v1/system/tenants/members/role/routes.go` | RegisterRoutes, r, h |
| `internal/http/handlers/api/v1/system/tenants/members/routes.go` | RegisterRoutes, r, h |
| `internal/http/handlers/api/v1/system/tenants/routes.go` | r, h, RegisterRoutes |
| `internal/http/handlers/api/v1/system/users/systemadmin/handler.go` | err, userID, r, prov, New, ... |
| `internal/http/handlers/api/v1/system/users/systemadmin/routes.go` | h, r, RegisterRoutes |
| `internal/http/server.go` | r, csrf, registerSystemRoutes, closure@529 |
| `internal/platform/entitlements/entitlements_test.go` | f, ok, isUnlimited, t, TestUnlimitedRegisteredByDefault |

## Connected Communities

- **auth +67 dirs** (21 cross-edges)
- **auth +32 dirs** (6 cross-edges)
- **service/servicetest +33 dirs** (6 cross-edges)
- **http/dto +36 dirs** (4 cross-edges)
- **usecase/goal +36 dirs** (4 cross-edges)
- **http/handlers · URLParam** (3 cross-edges)
- **web/shell +4 dirs** (2 cross-edges)
- **settings/noaccessmessage** (2 cross-edges)
- **v1/system · Put** (2 cross-edges)
- **. +2 dirs · TestBotIDRetryAfterTransientErr…** (2 cross-edges)
- **activity/purge +12 dirs** (2 cross-edges)
- **store/memberships +14 dirs** (2 cross-edges)
- **service/healthcheckin +6 dirs** (1 cross-edges)
- **auth +4 dirs · TestUsersEndpoint_ScopedSearch_…** (1 cross-edges)
- **service/activity +86 dirs** (1 cross-edges)
- **tenants/entitlements** (1 cross-edges)
- **tenants/suspend** (1 cross-edges)
- **store/memberships +8 dirs** (1 cross-edges)
- **admin/members** (1 cross-edges)
- **accessrequests/deny** (1 cross-edges)

## How to Explore

```
analyze(operation:"communities", id:"community-107")
explore(operation:"context", task:"understand system/notificationchannels +16 dirs", format:"gcx")
```

_`format: "gcx"` returns the [GCX1 compact wire format](../../docs/wire-format.md) — round-trippable, ~27% fewer tokens than JSON. Drop it for JSON output; agents using `@gortex/wire` or the Go `github.com/gortexhq/gcx-go` package decode either._
