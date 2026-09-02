---
name: gortex-http-dto-36-dirs
description: "Work in the http/dto +36 dirs area — 618 symbols across 48 files (60% cohesion)"
---

# http/dto +36 dirs

618 symbols | 48 files | 60% cohesion

## When to Use

Use this skill when working on files in:
- ``
- `cmd/server/main.go`
- `internal/auth/middleware.go`
- `internal/auth/providers/github/provider.go`
- `internal/auth/providers/google/provider.go`
- `internal/auth/providers/keycloak/provider.go`
- `internal/core/domain/kr_health_test.go`
- `internal/http/dto/goal.go`
- `internal/http/dto/goal_tree.go`
- `internal/http/dto/kr.go`
- `internal/http/dto/period.go`
- `internal/http/dto/team.go`
- `internal/http/dto/user.go`
- `internal/http/handlers/api/v1/goals/goalcommon/goalcommon.go`
- `internal/http/handlers/api/v1/goaltree/handler.go`
- `internal/http/handlers/api/v1/helpers_response.go`
- `internal/http/handlers/api/v1/helpers_response_test.go`
- `internal/http/handlers/api/v1/hierarchy/response.go`
- `internal/http/handlers/api/v1/notifications/routes_test.go`
- `internal/http/handlers/api/v1/periods/teams/activate/handler_test.go`
- `internal/http/handlers/api/v1/session/memberships/handler_test.go`
- `internal/http/handlers/api/v1/session/tenants/handler_test.go`
- `internal/http/handlers/api/v1/system/settings/defaultregistrationtenant/handler_test.go`
- `internal/http/handlers/api/v1/system/settings/noaccessmessage/handler_test.go`
- `internal/http/handlers/api/v1/system/tenants/entitlements/handler_test.go`
- `internal/http/handlers/api/v1/system/tenants/handler_test.go`
- `internal/http/handlers/api/v1/teams/teamscommon/teamscommon.go`
- `internal/http/handlers/web/common/common.go`
- `internal/http/handlers/web/common/team_type_test.go`
- `internal/http/middleware/csrf.go`
- `internal/http/routes_golden_test.go`
- `internal/platform/secretbox/secretbox.go`
- `internal/platform/secretbox/secretbox_test.go`
- `internal/render/export/export.go`
- `internal/render/notify/notify.go`
- `internal/service/activity/activity.go`
- `internal/service/activity/feed.go`
- `internal/service/activity/feed_test.go`
- `internal/service/notification/notification.go`
- `internal/service/notification/notification_test.go`
- `internal/service/notificationpref/notificationpref.go`
- `internal/service/user/user.go`
- `internal/store/activity/activity.go`
- `internal/store/goals/copy.go`
- `internal/store/store.go`
- `internal/store/tenants/cache_test.go`
- `internal/store/tenantsettings/tenantsettings.go`
- `internal/usecase/export/export_integration_test.go`

## Key Files

| File | Symbols |
|------|---------|
| `` | string, SplitN, byte |
| `cmd/server/main.go` | t, s, parts, p, out, ... |
| `internal/auth/middleware.go` | name, SetSessionCookie, secure, value, w, ... |
| `internal/auth/providers/github/provider.go` | AuthURL, state |
| `internal/auth/providers/google/provider.go` | AuthURL, state |
| `internal/auth/providers/keycloak/provider.go` | AuthURL, state |
| `internal/core/domain/kr_health_test.go` | t, TestKRHealthConstsMatchStrings |
| `internal/http/dto/goal.go` | CreatedAt, UpdatedAt, TeamID, ID, Title, ... |
| `internal/http/dto/goal_tree.go` | GoalTreeResponse, Name, Status, Periods, LedByMe, ... |
| `internal/http/dto/kr.go` | AuthorUDID, BooleanMeasure, NumericalCheckpoint, Weight, UpdatedAt, ... |
| `internal/http/dto/period.go` | PeriodInfo, Name, Depth, ID, EndDate, ... |
| `internal/http/dto/team.go` | Period, ProgressMeta, ID, Items, TeamChildSummaryResult, ... |
| `internal/http/dto/user.go` | UDID, AvatarURL, UserRef, DisplayName |
| `internal/http/handlers/api/v1/goals/goalcommon/goalcommon.go` | GoalResponse, comments, comment, krList, goal, ... |
| `internal/http/handlers/api/v1/goaltree/handler.go` | resp, g, n, buildResponse, t, ... |
| `internal/http/handlers/api/v1/helpers_response.go` | out, out, buildMeasure, detail, r, ... |
| `internal/http/handlers/api/v1/helpers_response_test.go` | TestBuildMeasureNumerical, t, kr, t, kind, ... |
| `internal/http/handlers/api/v1/hierarchy/response.go` | mapTeamNode, children, summary, userRefs, f, ... |
| `internal/http/handlers/api/v1/notifications/routes_test.go` | cursor, userID, List, filter |
| `internal/http/handlers/api/v1/periods/teams/activate/handler_test.go` | key, withURLParam, value, rctx, r |
| `internal/http/handlers/api/v1/session/memberships/handler_test.go` | t, GetBySlug, slug |
| `internal/http/handlers/api/v1/session/tenants/handler_test.go` | slug, GetBySlug, t |
| `internal/http/handlers/api/v1/system/settings/defaultregistrationtenant/handler_test.go` | key, SystemGet |
| `internal/http/handlers/api/v1/system/settings/noaccessmessage/handler_test.go` | key, SystemGet |
| `internal/http/handlers/api/v1/system/tenants/entitlements/handler_test.go` | SystemGet, key |
| `internal/http/handlers/api/v1/system/tenants/handler_test.go` | UpdateTenant, slug, name |
| `internal/http/handlers/api/v1/teams/teamscommon/teamscommon.go` | period, userRefs, progressMeta, period, rows, ... |
| `internal/http/handlers/web/common/common.go` | TeamPeriodStatusLabel, TeamTypeLabel, t, KRKindLabel, k, ... |
| `internal/http/handlers/web/common/team_type_test.go` | tt, cases, t, got, TestTeamTypeLabel, ... |
| `internal/http/middleware/csrf.go` | cookie, isUnsafeMethod, value, r, err, ... |
| `internal/http/routes_golden_test.go` | b, itoa, n, n |
| `internal/platform/secretbox/secretbox.go` | r, Hint, plaintext |
| `internal/platform/secretbox/secretbox_test.go` | t, TestHintShowsOnlyTail, got, got, got |
| `internal/render/export/export.go` | segs, words, line, line, f, ... |
| `internal/render/notify/notify.go` | before, m, noteChanged, text, s, ... |
| `internal/service/activity/activity.go` | ctx, err, ev, scope, Record |
| `internal/service/activity/feed.go` | raw, err, err, encodeCursor, err, ... |
| `internal/service/activity/feed_test.go` | t, got, TestEncodeCursorNilIsEmpty, TestCursorRoundTrip, TestDecodeCursorGarbageIsFirstPage, ... |
| `internal/service/notification/notification.go` | c, ctx, cursor, raw, err, ... |
| `internal/service/notification/notification_test.go` | s, TestDecodeCursorGarbageIsError, err, TestEncodeCursorNilIsEmpty, t, ... |
| `internal/service/notificationpref/notificationpref.go` | notifType, ResolveAddressed, userIDs, ctx, Resolve, ... |
| `internal/service/user/user.go` | leadUDIDs, q, ctx, SearchInSet, limit, ... |
| `internal/store/activity/activity.go` | CreatedAt, ID, Cursor |
| `internal/store/goals/copy.go` | id, TargetTeamID, weight, scope, err, ... |
| `internal/store/store.go` | NotificationChannels, GetSetting, userAgent, provider, userID, ... |
| `internal/store/tenants/cache_test.go` | GetBySlug, slug, t |
| `internal/store/tenantsettings/tenantsettings.go` | key, Delete, ctx, err, scope |
| `internal/usecase/export/export_integration_test.go` | itoa, n, t, start, n, ... |

## Connected Communities

- **service/servicetest +33 dirs** (43 cross-edges)
- **usecase/goal +36 dirs** (24 cross-edges)
- **v1/goals +9 dirs** (6 cross-edges)
- **auth +67 dirs** (6 cross-edges)
- **platform/eventbus +7 dirs** (5 cross-edges)
- **http · BuildProgressBarInfo** (4 cross-edges)
- **service/activity +61 dirs** (4 cross-edges)
- **http +3 dirs** (3 cross-edges)
- **core/progress +22 dirs** (3 cross-edges)
- **render/export +1 dirs · Filename** (2 cross-edges)
- **service/healthcheckin +6 dirs** (2 cross-edges)
- **system/notificationchannels +16 dirs** (2 cross-edges)
- **. +1 dirs · writeKR** (2 cross-edges)
- **store/periods +8 dirs** (2 cross-edges)
- **core/domain · PeriodStatusFor** (1 cross-edges)
- **render/notify +5 dirs** (1 cross-edges)
- **auth/start +3 dirs** (1 cross-edges)
- **auth +6 dirs** (1 cross-edges)
- **auth +32 dirs** (1 cross-edges)

## How to Explore

```
analyze(operation:"communities", id:"community-82")
explore(operation:"context", task:"understand http/dto +36 dirs", format:"gcx")
```

_`format: "gcx"` returns the [GCX1 compact wire format](../../docs/wire-format.md) — round-trippable, ~27% fewer tokens than JSON. Drop it for JSON output; agents using `@gortex/wire` or the Go `github.com/gortexhq/gcx-go` package decode either._
