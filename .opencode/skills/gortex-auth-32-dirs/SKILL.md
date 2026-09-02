---
name: gortex-auth-32-dirs
description: "Work in the auth +32 dirs area — 1014 symbols across 55 files (78% cohesion)"
---

# auth +32 dirs

1014 symbols | 55 files | 78% cohesion

## When to Use

Use this skill when working on files in:
- ``
- `app/app.go`
- `app/app_test.go`
- `internal/auth/config.go`
- `internal/auth/context.go`
- `internal/auth/context_test.go`
- `internal/auth/manager.go`
- `internal/auth/middleware.go`
- `internal/auth/middleware_test.go`
- `internal/auth/policy.go`
- `internal/core/domain/models.go`
- `internal/core/domain/tenant.go`
- `internal/core/event/event.go`
- `internal/http/handlers/api/v1/admin/activity/purge/handler_test.go`
- `internal/http/handlers/api/v1/admin/admincommon/admincommon.go`
- `internal/http/handlers/api/v1/admin/periods/archive/handler.go`
- `internal/http/handlers/api/v1/admin/periods/archive/handler_test.go`
- `internal/http/handlers/api/v1/admin/periods/overview/handler_test.go`
- `internal/http/handlers/api/v1/admin/periods/stats/handler_test.go`
- `internal/http/handlers/api/v1/admin/settings/general/handler.go`
- `internal/http/handlers/api/v1/admin/settings/general/handler_test.go`
- `internal/http/handlers/api/v1/admin/users/handler.go`
- `internal/http/handlers/api/v1/admin/users/handler_test.go`
- `internal/http/handlers/api/v1/config/handler_test.go`
- `internal/http/handlers/api/v1/errors.go`
- `internal/http/handlers/api/v1/errors_test.go`
- `internal/http/handlers/api/v1/hierarchy/routes_test.go`
- `internal/http/handlers/api/v1/me/handler.go`
- `internal/http/handlers/api/v1/me/handler_test.go`
- `internal/http/handlers/api/v1/me/routes.go`
- `internal/http/handlers/api/v1/onboarding/handler_test.go`
- `internal/http/handlers/api/v1/periods/routes_test.go`
- `internal/http/handlers/api/v1/periods/teams/activate/handler_test.go`
- `internal/http/handlers/api/v1/session/memberships/handler.go`
- `internal/http/handlers/api/v1/session/memberships/handler_test.go`
- `internal/http/handlers/api/v1/session/memberships/routes.go`
- `internal/http/handlers/api/v1/session/tenant/handler.go`
- `internal/http/handlers/api/v1/session/tenant/handler_test.go`
- `internal/http/handlers/api/v1/session/tenant/routes.go`
- `internal/http/handlers/api/v1/session/tenants/handler.go`
- `internal/http/handlers/api/v1/session/tenants/handler_test.go`
- `internal/http/handlers/api/v1/session/tenants/routes.go`
- `internal/http/handlers/api/v1/system/handler_test.go`
- `internal/http/handlers/api/v1/system/systemcommon/systemcommon.go`
- `internal/http/handlers/api/v1/system/tenants/activity/purge/handler_test.go`
- `internal/http/handlers/api/v1/testutil/integration.go`
- `internal/http/handlers/handlertest/handlertest.go`
- `internal/http/handlers/web/goals/delete/handler_test.go`
- `internal/http/handlers/web/shell/handler.go`
- `internal/http/handlers/web/shell/handler_test.go`
- `internal/http/middleware/csrf.go`
- `internal/http/middleware/csrf_test.go`
- `internal/http/server.go`
- `internal/service/user/user.go`
- `internal/store/store.go`

## Key Files

| File | Symbols |
|------|---------|
| `` | NewRecorder, app, Dir, New, http, ... |
| `app/app.go` | c, d, withAuthDefaults |
| `app/app_test.go` | err, tid, mounted, cleanup, t, ... |
| `internal/auth/config.go` | DefaultConfig |
| `internal/auth/context.go` | s, s, ctx, WithSession, WithUser, ... |
| `internal/auth/context_test.go` | u, TestSessionFromContextNilWhenNotSet, got, t, tn, ... |
| `internal/auth/manager.go` | Disabled |
| `internal/auth/middleware.go` | anon, AnonymousUserMiddleware, closure@78, policy, status, ... |
| `internal/auth/middleware_test.go` | h, called, rw, req, req, ... |
| `internal/auth/policy.go` | cfg, err, allIDs, rootIDs, ctx, ... |
| `internal/core/domain/models.go` | Email, CreatedAt, AttributesJSON, Provider, UpdatedAt, ... |
| `internal/core/domain/tenant.go` | DeletedAt, Name, ID, CreatedAt, Slug, ... |
| `internal/core/event/event.go` | Context |
| `internal/http/handlers/api/v1/admin/activity/purge/handler_test.go` | h, TestHandlePurgeActivity, withTenant, w, r, ... |
| `internal/http/handlers/api/v1/admin/admincommon/admincommon.go` | TenantRenamer, UserAdminStore |
| `internal/http/handlers/api/v1/admin/periods/archive/handler.go` | periods, periods, Handler, New |
| `internal/http/handlers/api/v1/admin/periods/archive/handler_test.go` | h, now, r, TestHandleArchivePeriod_AllowsClosed, h, ... |
| `internal/http/handlers/api/v1/admin/periods/overview/handler_test.go` | TestHandlePeriodOverview_ForbiddenWithoutScope, value, req, withURLParam, rctx, ... |
| `internal/http/handlers/api/v1/admin/periods/stats/handler_test.go` | w, t, TestHandlePeriodStats_ForbiddenWithoutScope, h, req |
| `internal/http/handlers/api/v1/admin/settings/general/handler.go` | renamer, settings, New |
| `internal/http/handlers/api/v1/admin/settings/general/handler_test.go` | h, r, r, body, TestHandleUpdateGeneralSettingsStoresValidURL, ... |
| `internal/http/handlers/api/v1/admin/users/handler.go` | Status, Role, GrantedNodeCount, User, userListItem |
| `internal/http/handlers/api/v1/admin/users/handler_test.go` | ListUsers, GetUser, withTenant, fakeUsers, tenantUsers, ... |
| `internal/http/handlers/api/v1/config/handler_test.go` | r, t, err, r, TestHandleConfigIsSystemAdmin, ... |
| `internal/http/handlers/api/v1/errors.go` | ErrorResponse, Error |
| `internal/http/handlers/api/v1/errors_test.go` | err, t, recorder, TestWriteError |
| `internal/http/handlers/api/v1/hierarchy/routes_test.go` | TestRegisterRoutes, r, req, t, w |
| `internal/http/handlers/api/v1/me/handler.go` | New, UDID, DisplayName, w, r, ... |
| `internal/http/handlers/api/v1/me/handler_test.go` | TestHandleMeReturnsUserJSON, r, r, TestHandleMeReturns401WhenNoUser, u, ... |
| `internal/http/handlers/api/v1/me/routes.go` | RegisterRoutes, r, h |
| `internal/http/handlers/api/v1/onboarding/handler_test.go` | err, TestCreateInvitationLink, err, req, lw, ... |
| `internal/http/handlers/api/v1/periods/routes_test.go` | TestRegisterRoutes, r, req, t, w |
| `internal/http/handlers/api/v1/periods/teams/activate/handler_test.go` | ctx, req, req, r, withTenant, ... |
| `internal/http/handlers/api/v1/session/memberships/handler.go` | w, r, user, list, w, ... |
| `internal/http/handlers/api/v1/session/memberships/handler_test.go` | err, req, ctx, req2, method, ... |
| `internal/http/handlers/api/v1/session/memberships/routes.go` | r, RegisterRoutes, h |
| `internal/http/handlers/api/v1/session/tenant/handler.go` | TenantID, tn, Handler, targetID, Post, ... |
| `internal/http/handlers/api/v1/session/tenant/handler_test.go` | t, t, deps, ctx, TestSwitchTenantUpdatesSession, ... |
| `internal/http/handlers/api/v1/session/tenant/routes.go` | RegisterRoutes, r, h |
| `internal/http/handlers/api/v1/session/tenants/handler.go` | m, out, user, tn, sess, ... |
| `internal/http/handlers/api/v1/session/tenants/handler_test.go` | body, rw, deps, err, t, ... |
| `internal/http/handlers/api/v1/session/tenants/routes.go` | r, h, RegisterRoutes |
| `internal/http/handlers/api/v1/system/handler_test.go` | admin, code, code, admin, w, ... |
| `internal/http/handlers/api/v1/system/systemcommon/systemcommon.go` | MemberLister |
| `internal/http/handlers/api/v1/system/tenants/activity/purge/handler_test.go` | rctx, fp, w, h, r, ... |
| `internal/http/handlers/api/v1/testutil/integration.go` | closure@143, wrapped, grantsCache, NewAPIV1RouterWithUser, allowedTeamIDs, ... |
| `internal/http/handlers/handlertest/handlertest.go` | Form, body, method, err, body, ... |
| `internal/http/handlers/web/goals/delete/handler_test.go` | mw, w, multipart, r, o, ... |
| `internal/http/handlers/web/shell/handler.go` | keep, rd, KeepQuery, closure@109, To, ... |
| `internal/http/handlers/web/shell/handler_test.go` | closure@103, r, table, r, TestEachRedirectGoesToItsOwnTarget, ... |
| `internal/http/middleware/csrf.go` | writeCSRFError, CSRFMiddleware, Handler, next, r, ... |
| `internal/http/middleware/csrf_test.go` | mw, t, req, h, closure@40, ... |
| `internal/http/server.go` | csrf, closure@337, closure@365, staticFiles, deps, ... |
| `internal/service/user/user.go` | GetByDisplayNames, names, ctx |
| `internal/store/store.go` | ctx, GetUser, id |

## Entry Points

- `internal/http/handlers/api/v1/onboarding/handler_test.go::TestJoinRequestEndpoint`
- `internal/http/server.go::Server.Routes`

## Connected Communities

- **auth +67 dirs** (43 cross-edges)
- **usecase/goal +36 dirs** (37 cross-edges)
- **store/memberships +14 dirs** (17 cross-edges)
- **activity/purge +12 dirs** (13 cross-edges)
- **system/notificationchannels +16 dirs** (8 cross-edges)
- **web/invite +1 dirs** (7 cross-edges)
- **service/servicetest +33 dirs** (6 cross-edges)
- **render/notify +5 dirs** (5 cross-edges)
- **store/memberships +8 dirs** (5 cross-edges)
- **store · TestSearchUsersInSet** (4 cross-edges)
- **service/goal +8 dirs** (4 cross-edges)
- **. +4 dirs · specURIs** (4 cross-edges)
- **auth +6 dirs** (4 cross-edges)
- **v1/config +2 dirs** (4 cross-edges)
- **http/dto +36 dirs** (4 cross-edges)
- **render/export +1 dirs · Filename** (3 cross-edges)
- **store/memberships +4 dirs** (3 cross-edges)
- **v1/system · TestSystemPatchTenant** (3 cross-edges)
- **service/activity +86 dirs** (3 cross-edges)
- **v1/admin · Get · admincommon · handler (18)** (3 cross-edges)
- **web/noaccess +1 dirs** (2 cross-edges)
- **auth/start +3 dirs** (2 cross-edges)
- **web/logout +1 dirs** (2 cross-edges)
- **. +2 dirs · TestBotIDRetryAfterTransientErr…** (2 cross-edges)
- **service/healthcheckin +6 dirs** (2 cross-edges)
- **store/tenantsettings · TestTenantSettingsScopedByTenant** (2 cross-edges)
- **usecase/notification +1 dirs · TestPayloadOfTruncatesBothNoteS…** (1 cross-edges)
- **auth · Resolve** (1 cross-edges)
- **service/notificationpref +3 dirs** (1 cross-edges)
- **teams/activate · Post · handler · routes (10) #1** (1 cross-edges)
- **auth +3 dirs** (1 cross-edges)
- **v1/goals +9 dirs** (1 cross-edges)
- **auth/callback** (1 cross-edges)
- **api/v1 · writeError** (1 cross-edges)
- **api/v1 · WriteError** (1 cross-edges)
- **web/logout** (1 cross-edges)
- **web/noaccess** (1 cross-edges)
- **usecase/notification +4 dirs** (1 cross-edges)
- **web/shell +4 dirs** (1 cross-edges)
- **v1/hierarchy** (1 cross-edges)

## How to Explore

```
analyze(operation:"communities", id:"community-268")
explore(operation:"context", task:"understand auth +32 dirs", format:"gcx")
relations(operation:"usages", target:{symbol:"internal/http/handlers/api/v1/onboarding/handler_test.go::TestJoinRequestEndpoint"}, format:"gcx")
```

_`format: "gcx"` returns the [GCX1 compact wire format](../../docs/wire-format.md) — round-trippable, ~27% fewer tokens than JSON. Drop it for JSON output; agents using `@gortex/wire` or the Go `github.com/gortexhq/gcx-go` package decode either._
