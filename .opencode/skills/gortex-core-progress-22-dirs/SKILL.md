---
name: gortex-core-progress-22-dirs
description: "Work in the core/progress +22 dirs area — 457 symbols across 29 files (70% cohesion)"
---

# core/progress +22 dirs

457 symbols | 29 files | 70% cohesion

## When to Use

Use this skill when working on files in:
- ``
- `internal/core/progress/goal.go`
- `internal/core/progress/goal_test.go`
- `internal/core/progress/progress.go`
- `internal/core/progress/progress_test.go`
- `internal/http/handlers/api/v1/admin/admincommon/admincommon.go`
- `internal/http/handlers/api/v1/config/handler.go`
- `internal/http/handlers/api/v1/krs/movedown/handler_test.go`
- `internal/http/handlers/api/v1/krs/moveup/handler_test.go`
- `internal/http/handlers/api/v1/notifications/unreadcount/handler_test.go`
- `internal/http/handlers/web/common/common.go`
- `internal/scheduler/scheduler.go`
- `internal/service/healthcheckin/healthcheckin.go`
- `internal/service/keyresult/keyresult.go`
- `internal/service/notification/notification.go`
- `internal/service/notification/notification_test.go`
- `internal/service/onboarding/onboarding_test.go`
- `internal/service/servicetest/store.go`
- `internal/service/user/user.go`
- `internal/store/goals/goals.go`
- `internal/store/krs/krs.go`
- `internal/store/memberships/memberships.go`
- `internal/store/notifications/notifications.go`
- `internal/store/shares/shares.go`
- `internal/usecase/keyresult/keyresult.go`
- `internal/usecase/okrboard/okrboard.go`
- `internal/usecase/period/bulkstatus.go`
- `internal/usecase/period/overview.go`
- `internal/usecase/period/progress.go`

## Key Files

| File | Symbols |
|------|---------|
| `` | int, math, Round, copy, Slice |
| `internal/core/progress/goal.go` | kr, ForKR, i, ForGoal, goal |
| `internal/core/progress/goal_test.go` | t, kr, got, done, TestCalculateKRProgressNilMetaReturnsZero, ... |
| `internal/core/progress/progress.go` | weighted, stage, weighted, done, PeriodProgress, ... |
| `internal/core/progress/progress_test.go` | TestGoalProgress, got, t, got, got, ... |
| `internal/http/handlers/api/v1/admin/admincommon/admincommon.go` | ctx, SettingInt, settings, scope, def, ... |
| `internal/http/handlers/api/v1/config/handler.go` | key, ctx, settingInt, scope, def, ... |
| `internal/http/handlers/api/v1/krs/movedown/handler_test.go` | Move, direction |
| `internal/http/handlers/api/v1/krs/moveup/handler_test.go` | Move, direction |
| `internal/http/handlers/api/v1/notifications/unreadcount/handler_test.go` | UnreadCount, userID |
| `internal/http/handlers/web/common/common.go` | ValidateStageWeights, newWeight, existing |
| `internal/scheduler/scheduler.go` | SnapshotRunner |
| `internal/service/healthcheckin/healthcheckin.go` | calcExpectedPace, now, frac, total, elapsed, ... |
| `internal/service/keyresult/keyresult.go` | scope, before, Move, krID, ctx, ... |
| `internal/service/notification/notification.go` | ctx, Purge, readDays, anyDays |
| `internal/service/notification/notification_test.go` | PurgeOlderThan, UnreadCount |
| `internal/service/onboarding/onboarding_test.go` | intp, n |
| `internal/service/servicetest/store.go` | krID, direction, MoveGoal, direction, goalID, ... |
| `internal/service/user/user.go` | limit, SearchUnrestricted, ctx, q |
| `internal/store/goals/goals.go` | goalID, scope, weight, err, UpdateGoalOwner, ... |
| `internal/store/krs/krs.go` | direction, row, ctx, krID, err, ... |
| `internal/store/memberships/memberships.go` | scope, err, CountActiveAdmins, ctx |
| `internal/store/notifications/notifications.go` | PurgeOlderThan, err, readDays, tag, anyDays, ... |
| `internal/store/shares/shares.go` | UpdateGoalTeamWeight, goalID, err, res, teamID, ... |
| `internal/usecase/keyresult/keyresult.go` | in, krID, u, ctx, done, ... |
| `internal/usecase/okrboard/okrboard.go` | WorkType, Weight, rows, goalsList, ShareTeams, ... |
| `internal/usecase/period/bulkstatus.go` | statuses, teams, target, cur, goalsByTeam, ... |
| `internal/usecase/period/overview.go` | ctx, goalItems, status, avg, data, ... |
| `internal/usecase/period/progress.go` | today, Progress, PeriodStart, snaps, acc, ... |

## Connected Communities

- **service/servicetest +33 dirs** (31 cross-edges)
- **usecase/goal +36 dirs** (25 cross-edges)
- **core/progress +2 dirs** (9 cross-edges)
- **http/dto +36 dirs** (6 cross-edges)
- **service/healthcheckin +6 dirs** (3 cross-edges)
- **auth +32 dirs** (2 cross-edges)
- **usecase/period · bucketsFor** (2 cross-edges)
- **v1/healthcheckin** (1 cross-edges)
- **service/activity +61 dirs** (1 cross-edges)
- **http +3 dirs** (1 cross-edges)
- **auth +67 dirs** (1 cross-edges)
- **v1/config +2 dirs** (1 cross-edges)

## How to Explore

```
analyze(operation:"communities", id:"community-215")
explore(operation:"context", task:"understand core/progress +22 dirs", format:"gcx")
```

_`format: "gcx"` returns the [GCX1 compact wire format](../../docs/wire-format.md) — round-trippable, ~27% fewer tokens than JSON. Drop it for JSON output; agents using `@gortex/wire` or the Go `github.com/gortexhq/gcx-go` package decode either._
