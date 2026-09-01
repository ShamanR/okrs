---
name: gortex-usecase-notification-4-dirs
description: "Work in the usecase/notification +4 dirs area — 236 symbols across 10 files (75% cohesion)"
---

# usecase/notification +4 dirs

236 symbols | 10 files | 75% cohesion

## When to Use

Use this skill when working on files in:
- ``
- `internal/core/event/event.go`
- `internal/core/event/events.go`
- `internal/service/activity/activity.go`
- `internal/service/activity/journal.go`
- `internal/service/notification/notification.go`
- `internal/service/notification/notification_test.go`
- `internal/usecase/notification/mapping.go`
- `internal/usecase/notification/notification.go`
- `internal/usecase/notification/notification_test.go`

## Key Files

| File | Symbols |
|------|---------|
| `` | Unix, Join |
| `internal/core/event/event.go` | Event |
| `internal/core/event/events.go` | Kind, Changed, GoalID, Meta, Title, ... |
| `internal/service/activity/activity.go` | evs, ctx, scope, RecordBatch |
| `internal/service/activity/journal.go` | Handle, err, tenantID, byTenant, rows, ... |
| `internal/service/notification/notification.go` | Repo, New, repo |
| `internal/service/notification/notification_test.go` | repo, svc, t, err, token, ... |
| `internal/usecase/notification/mapping.go` | notifyType, ev |
| `internal/usecase/notification/notification.go` | items, pending, items, m, errs, ... |
| `internal/usecase/notification/notification_test.go` | w, TestAllTenantFailuresAreJoinedRegardlessOfOrder, err, m, TestCoalesceKeySameWithinTenMinuteBucket, ... |

## Connected Communities

- **usecase/goal +36 dirs** (51 cross-edges)
- **service/servicetest +33 dirs** (14 cross-edges)
- **service/activity +61 dirs** (5 cross-edges)
- **auth +32 dirs** (4 cross-edges)
- **http/dto +36 dirs** (2 cross-edges)
- **service/healthcheckin +6 dirs** (2 cross-edges)
- **service/notificationpref +3 dirs** (2 cross-edges)
- **v1/goals +9 dirs** (1 cross-edges)
- **core/domain +1 dirs** (1 cross-edges)
- **render/notify +5 dirs** (1 cross-edges)
- **usecase/notification +1 dirs · anchorOf** (1 cross-edges)
- **platform/eventbus +7 dirs** (1 cross-edges)
- **system/notificationchannels +16 dirs** (1 cross-edges)
- **auth +67 dirs** (1 cross-edges)
- **usecase/notification +1 dirs · TestPayloadOfTruncatesBothNoteS…** (1 cross-edges)
- **store/periods +8 dirs** (1 cross-edges)
- **service/notificationchannel +10 dirs** (1 cross-edges)

## How to Explore

```
analyze(operation:"communities", id:"community-208")
explore(operation:"context", task:"understand usecase/notification +4 dirs", format:"gcx")
```

_`format: "gcx"` returns the [GCX1 compact wire format](../../docs/wire-format.md) — round-trippable, ~27% fewer tokens than JSON. Drop it for JSON output; agents using `@gortex/wire` or the Go `github.com/gortexhq/gcx-go` package decode either._
