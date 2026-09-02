---
name: gortex-http-handlers-urlparam
description: "Work in the http/handlers · URLParam area — 275 symbols across 51 files (62% cohesion)"
---

# http/handlers · URLParam

275 symbols | 51 files | 62% cohesion

## When to Use

Use this skill when working on files in:
- `internal/http/handlers/api/v1/admin/accessrequests/approve/handler.go`
- `internal/http/handlers/api/v1/admin/accessrequests/approve/handler_test.go`
- `internal/http/handlers/api/v1/admin/accessrequests/deny/handler.go`
- `internal/http/handlers/api/v1/admin/accessrequests/deny/handler_test.go`
- `internal/http/handlers/api/v1/admin/invitations/revoke/handler_test.go`
- `internal/http/handlers/api/v1/admin/members/handler.go`
- `internal/http/handlers/api/v1/admin/members/handler_test.go`
- `internal/http/handlers/api/v1/admin/periods/handler_test.go`
- `internal/http/handlers/api/v1/admin/periods/teams/activate/handler_test.go`
- `internal/http/handlers/api/v1/admin/periods/teams/close/handler_test.go`
- `internal/http/handlers/api/v1/admin/periods/unarchive/handler_test.go`
- `internal/http/handlers/api/v1/admin/teams/handler_test.go`
- `internal/http/handlers/api/v1/admin/teams/hard/handler_test.go`
- `internal/http/handlers/api/v1/admin/teams/restore/handler_test.go`
- `internal/http/handlers/api/v1/admin/users/admin/handler_test.go`
- `internal/http/handlers/api/v1/admin/users/grants/handler.go`
- `internal/http/handlers/api/v1/admin/users/grants/handler_test.go`
- `internal/http/handlers/api/v1/goals/comments/handler.go`
- `internal/http/handlers/api/v1/goals/comments/handler_test.go`
- `internal/http/handlers/api/v1/goals/comments/replies/handler.go`
- `internal/http/handlers/api/v1/goals/comments/replies/handler_test.go`
- `internal/http/handlers/api/v1/goals/keyresults/handler_test.go`
- `internal/http/handlers/api/v1/goals/links/handler_test.go`
- `internal/http/handlers/api/v1/goals/share/handler.go`
- `internal/http/handlers/api/v1/goals/share/handler_test.go`
- `internal/http/handlers/api/v1/goals/transfer/handler_test.go`
- `internal/http/handlers/api/v1/goals/weight/handler_test.go`
- `internal/http/handlers/api/v1/krs/description/handler_test.go`
- `internal/http/handlers/api/v1/krs/note/handler_test.go`
- `internal/http/handlers/api/v1/krs/progress/boolean/handler_test.go`
- `internal/http/handlers/api/v1/krs/progress/numerical/handler_test.go`
- `internal/http/handlers/api/v1/krs/progress/project/handler_test.go`
- `internal/http/handlers/api/v1/periods/teams/close/handler.go`
- `internal/http/handlers/api/v1/periods/teams/close/handler_test.go`
- `internal/http/handlers/api/v1/system/tenants/members/deny/handler.go`
- `internal/http/handlers/api/v1/system/tenants/members/deny/handler_test.go`
- `internal/http/handlers/api/v1/system/tenants/members/role/handler_test.go`
- `internal/http/handlers/api/v1/system/tenants/restore/handler.go`
- `internal/http/handlers/api/v1/system/tenants/restore/handler_test.go`
- `internal/http/handlers/api/v1/system/tenants/suspend/handler.go`
- `internal/http/handlers/api/v1/system/tenants/suspend/handler_test.go`
- `internal/http/handlers/api/v1/system/users/systemadmin/handler_test.go`
- `internal/http/handlers/api/v1/teams/export/handler_test.go`
- `internal/http/handlers/api/v1/teams/goals/handler.go`
- `internal/http/handlers/api/v1/teams/goals/handler_test.go`
- `internal/http/handlers/api/v1/teams/okrs/handler.go`
- `internal/http/handlers/api/v1/teams/okrs/handler_test.go`
- `internal/http/handlers/api/v1/teams/overview/handler.go`
- `internal/http/handlers/api/v1/teams/overview/handler_test.go`
- `internal/http/handlers/api/v1/teams/status/handler_test.go`
- `internal/http/handlers/handlertest/handlertest.go`

## Key Files

| File | Symbols |
|------|---------|
| `internal/http/handlers/api/v1/admin/accessrequests/approve/handler.go` | New |
| `internal/http/handlers/api/v1/admin/accessrequests/approve/handler_test.go` | t, v, t, w, TestRequiresTenant, ... |
| `internal/http/handlers/api/v1/admin/accessrequests/deny/handler.go` | New, onboard |
| `internal/http/handlers/api/v1/admin/accessrequests/deny/handler_test.go` | TestBadUserIDIs400, t, t, closure@52, w, ... |
| `internal/http/handlers/api/v1/admin/invitations/revoke/handler_test.go` | TestGatePostRequiresTenant, t, w |
| `internal/http/handlers/api/v1/admin/members/handler.go` | New, onboard |
| `internal/http/handlers/api/v1/admin/members/handler_test.go` | w, closure@52, TestRequiresTenant, TestBadUserIDIs400, t, ... |
| `internal/http/handlers/api/v1/admin/periods/handler_test.go` | TestGatePatchRequiresTenant, t, w, TestGateDeleteRequiresTenant, w, ... |
| `internal/http/handlers/api/v1/admin/periods/teams/activate/handler_test.go` | TestRequiresTenant, t, w, t, TestBadPeriodIDIs400, ... |
| `internal/http/handlers/api/v1/admin/periods/teams/close/handler_test.go` | TestRequiresTenant, w, t, t, w, ... |
| `internal/http/handlers/api/v1/admin/periods/unarchive/handler_test.go` | w, TestGatePostRequiresTenant, t |
| `internal/http/handlers/api/v1/admin/teams/handler_test.go` | w, w, TestGateGetRequiresTenant, w, t, ... |
| `internal/http/handlers/api/v1/admin/teams/hard/handler_test.go` | t, w, TestGateDeleteRequiresTenant |
| `internal/http/handlers/api/v1/admin/teams/restore/handler_test.go` | w, t, TestGatePostRequiresTenant |
| `internal/http/handlers/api/v1/admin/users/admin/handler_test.go` | t, w, TestRequiresTenant |
| `internal/http/handlers/api/v1/admin/users/grants/handler.go` | New, grants |
| `internal/http/handlers/api/v1/admin/users/grants/handler_test.go` | w, t, m, TestRequiresTenant, closure@54, ... |
| `internal/http/handlers/api/v1/goals/comments/handler.go` | New, shares, uc, goals |
| `internal/http/handlers/api/v1/goals/comments/handler_test.go` | t, TestGateDeleteBadGoalID, w, TestGateDeleteRequiresTenant, TestGatePostRequiresTenant, ... |
| `internal/http/handlers/api/v1/goals/comments/replies/handler.go` | New, Handler, shares, uc, shares, ... |
| `internal/http/handlers/api/v1/goals/comments/replies/handler_test.go` | t, t, TestGatePostBadGoalID, w, TestGatePostRequiresTenant, ... |
| `internal/http/handlers/api/v1/goals/keyresults/handler_test.go` | w, t, TestGatePostBadGoalID |
| `internal/http/handlers/api/v1/goals/links/handler_test.go` | t, TestGatePostRequiresTenant, t, TestGatePostBadGoalID, w, ... |
| `internal/http/handlers/api/v1/goals/share/handler.go` | New, uc, goals, shares |
| `internal/http/handlers/api/v1/goals/share/handler_test.go` | t, t, w, TestGatePostBadGoalID, w, ... |
| `internal/http/handlers/api/v1/goals/transfer/handler_test.go` | w, TestGatePostRequiresTenant, t, TestGatePostBadGoalID, t, ... |
| `internal/http/handlers/api/v1/goals/weight/handler_test.go` | w, TestGatePostBadGoalID, t, TestGatePostRequiresTenant, t, ... |
| `internal/http/handlers/api/v1/krs/description/handler_test.go` | w, t, TestGatePostBadKrID |
| `internal/http/handlers/api/v1/krs/note/handler_test.go` | t, TestGatePostBadKrID, w |
| `internal/http/handlers/api/v1/krs/progress/boolean/handler_test.go` | w, t, TestGatePostBadKrID |
| `internal/http/handlers/api/v1/krs/progress/numerical/handler_test.go` | w, TestGatePostBadKrID, t |
| `internal/http/handlers/api/v1/krs/progress/project/handler_test.go` | w, t, TestGatePostBadKrID |
| `internal/http/handlers/api/v1/periods/teams/close/handler.go` | periodUC, New, leads, teams |
| `internal/http/handlers/api/v1/periods/teams/close/handler_test.go` | t, w, t, TestRequiresTenant, w, ... |
| `internal/http/handlers/api/v1/system/tenants/members/deny/handler.go` | New, prov |
| `internal/http/handlers/api/v1/system/tenants/members/deny/handler_test.go` | w, TestGatePostBadUserID, t |
| `internal/http/handlers/api/v1/system/tenants/members/role/handler_test.go` | TestGatePutBadUserID, t, w |
| `internal/http/handlers/api/v1/system/tenants/restore/handler.go` | New |
| `internal/http/handlers/api/v1/system/tenants/restore/handler_test.go` | w, t, TestMissingTenantIs404, TestBadTenantIDIs400, v, ... |
| `internal/http/handlers/api/v1/system/tenants/suspend/handler.go` | New, prov |
| `internal/http/handlers/api/v1/system/tenants/suspend/handler_test.go` | TestAppliesPackageTransition, t, f, w, TestBadTenantIDIs400, ... |
| `internal/http/handlers/api/v1/system/users/systemadmin/handler_test.go` | t, w, TestGatePutBadUserID |
| `internal/http/handlers/api/v1/teams/export/handler_test.go` | TestGateGetRequiresTenant, t, t, w, TestGateGetBadTeamID, ... |
| `internal/http/handlers/api/v1/teams/goals/handler.go` | goalUC, users, New |
| `internal/http/handlers/api/v1/teams/goals/handler_test.go` | TestMalformedBodyIs400, TestBadTeamIDIs400, w, t, w, ... |
| `internal/http/handlers/api/v1/teams/okrs/handler.go` | New, periods, board, users |
| `internal/http/handlers/api/v1/teams/okrs/handler_test.go` | t, w, TestGateGetRequiresTenant, TestGateGetBadTeamID, w, ... |
| `internal/http/handlers/api/v1/teams/overview/handler.go` | New, periods, users, board |
| `internal/http/handlers/api/v1/teams/overview/handler_test.go` | w, w, TestGateGetRequiresTenant, t, t, ... |
| `internal/http/handlers/api/v1/teams/status/handler_test.go` | TestGatePostRequiresTenant, w, t |
| `internal/http/handlers/handlertest/handlertest.go` | IsError, w, URLParam, ok, t, ... |

## Connected Communities

- **service/activity +86 dirs** (109 cross-edges)
- **auth +67 dirs** (27 cross-edges)
- **system/notificationchannels +16 dirs** (2 cross-edges)
- **service/activity +61 dirs** (2 cross-edges)
- **auth +32 dirs** (2 cross-edges)
- **store/memberships +14 dirs** (1 cross-edges)

## How to Explore

```
analyze(operation:"communities", id:"community-111")
explore(operation:"context", task:"understand http/handlers · URLParam", format:"gcx")
```

_`format: "gcx"` returns the [GCX1 compact wire format](../../docs/wire-format.md) — round-trippable, ~27% fewer tokens than JSON. Drop it for JSON output; agents using `@gortex/wire` or the Go `github.com/gortexhq/gcx-go` package decode either._
