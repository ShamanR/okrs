---
name: gortex-store-notifications
description: "Work in the store/notifications area — 311 symbols across 2 files (86% cohesion)"
---

# store/notifications

311 symbols | 2 files | 86% cohesion

## When to Use

Use this skill when working on files in:
- `internal/store/notifications/notifications.go`
- `internal/store/notifications/notifications_test.go`

## Key Files

| File | Symbols |
|------|---------|
| `internal/store/notifications/notifications.go` | Payload, ins, br, p, Delete, ... |
| `internal/store/notifications/notifications_test.go` | t, newRepo, ctx, in, ctx, ... |

## Connected Communities

- **usecase/goal +36 dirs** (37 cross-edges)
- **service/servicetest +33 dirs** (2 cross-edges)
- **. +1 dirs · writeKR** (1 cross-edges)
- **core/progress +22 dirs** (1 cross-edges)
- **service/activity +61 dirs** (1 cross-edges)
- **v1/config +2 dirs** (1 cross-edges)
- **auth +32 dirs** (1 cross-edges)

## How to Explore

```
analyze(operation:"communities", id:"community-170")
explore(operation:"context", task:"understand store/notifications", format:"gcx")
```

_`format: "gcx"` returns the [GCX1 compact wire format](../../docs/wire-format.md) — round-trippable, ~27% fewer tokens than JSON. Drop it for JSON output; agents using `@gortex/wire` or the Go `github.com/gortexhq/gcx-go` package decode either._
