---
name: gortex-v1-goals-9-dirs
description: "Work in the v1/goals +9 dirs area — 412 symbols across 17 files (74% cohesion)"
---

# v1/goals +9 dirs

412 symbols | 17 files | 74% cohesion

## When to Use

Use this skill when working on files in:
- ``
- `internal/http/handlers/api/v1/goals/access_test.go`
- `internal/http/handlers/api/v1/goals/leave_share_test.go`
- `internal/http/handlers/api/v1/goals/links_board_test.go`
- `internal/http/handlers/api/v1/goals/links_test.go`
- `internal/http/handlers/api/v1/goals/move_test.go`
- `internal/http/handlers/api/v1/goals/replies_test.go`
- `internal/http/handlers/api/v1/goals/repro_weight_test.go`
- `internal/http/handlers/api/v1/goals/resolve_test.go`
- `internal/http/handlers/api/v1/goaltree/handler_test.go`
- `internal/http/handlers/api/v1/krs/access_test.go`
- `internal/http/handlers/api/v1/notifications/e2e_test.go`
- `internal/service/notificationchannel/notificationchannel.go`
- `internal/store/goals/goals.go`
- `internal/store/grants/grants.go`
- `internal/store/shares/shares.go`
- `internal/usecase/notification/notification.go`

## Key Files

| File | Symbols |
|------|---------|
| `` | multipart, Post, NewRequest, NewWriter, NewBuffer, ... |
| `internal/http/handlers/api/v1/goals/access_test.go` | closure@98, err, closure@126, goalID, pool, ... |
| `internal/http/handlers/api/v1/goals/leave_share_test.go` | TestLeaveGoalShareRemovesGoalFromTeamOnly, sees, mustTeam, closure@60, gc, ... |
| `internal/http/handlers/api/v1/goals/links_board_test.go` | closure@60, repo, pool, teamID, closure@74, ... |
| `internal/http/handlers/api/v1/goals/links_test.go` | pool, repo, err, err, closure@136, ... |
| `internal/http/handlers/api/v1/goals/move_test.go` | err, shared, repo, resp, TestMoveGoalReadsTeamIDFromJSON, ... |
| `internal/http/handlers/api/v1/goals/replies_test.go` | scope, err, TestReplyAndDeleteHandlers, gc, server, ... |
| `internal/http/handlers/api/v1/goals/repro_weight_test.go` | closure@32, resp, w, TestSharedGoalWeightEditKeepsGoalVisible, err, ... |
| `internal/http/handlers/api/v1/goals/resolve_test.go` | comments, err, err, repo, ctx, ... |
| `internal/http/handlers/api/v1/goaltree/handler_test.go` | TestGoalTree_TenantIsolation, pool, ctx, g, err, ... |
| `internal/http/handlers/api/v1/krs/access_test.go` | closure@261, closure@204, multipartBody, closure@218, closure@154, ... |
| `internal/http/handlers/api/v1/notifications/e2e_test.go` | resp, t, err, pool, err, ... |
| `internal/service/notificationchannel/notificationchannel.go` | Unwrap, Channel, Label, Key, FieldRequiredError, ... |
| `internal/store/goals/goals.go` | FocusType, WorkType, TeamID, OwnerUDIDs, Priority, ... |
| `internal/store/grants/grants.go` | r, NewGrantsCache |
| `internal/store/shares/shares.go` | GoalShareRepository, db, db, NewGoalShareRepository |
| `internal/usecase/notification/notification.go` | at, coalesceKey, entity, at, m, ... |

## Connected Communities

- **usecase/goal +36 dirs** (33 cross-edges)
- **auth +67 dirs** (31 cross-edges)
- **store +14 dirs** (21 cross-edges)
- **v1/config +2 dirs** (4 cross-edges)
- **http/dto +36 dirs** (4 cross-edges)
- **service/healthcheckin +6 dirs** (3 cross-edges)
- **service/activity +61 dirs** (1 cross-edges)
- **service/keyresult +4 dirs** (1 cross-edges)
- **auth +32 dirs** (1 cross-edges)

## How to Explore

```
analyze(operation:"communities", id:"community-266")
explore(operation:"context", task:"understand v1/goals +9 dirs", format:"gcx")
```

_`format: "gcx"` returns the [GCX1 compact wire format](../../docs/wire-format.md) — round-trippable, ~27% fewer tokens than JSON. Drop it for JSON output; agents using `@gortex/wire` or the Go `github.com/gortexhq/gcx-go` package decode either._
