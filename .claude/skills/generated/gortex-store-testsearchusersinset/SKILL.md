---
name: gortex-store-testsearchusersinset
description: "Work in the store · TestSearchUsersInSet area — 235 symbols across 5 files (78% cohesion)"
---

# store · TestSearchUsersInSet

235 symbols | 5 files | 78% cohesion

## When to Use

Use this skill when working on files in:
- `internal/store/sessions/sessions.go`
- `internal/store/sessions/sessions_test.go`
- `internal/store/store.go`
- `internal/store/users/users.go`
- `internal/store/users/users_test.go`

## Key Files

| File | Symbols |
|------|---------|
| `internal/store/sessions/sessions.go` | DeleteExpiredSessions, TouchSession, ctx, nullableString, ctx, ... |
| `internal/store/sessions/sessions_test.go` | err, t, ctx, err, repo, ... |
| `internal/store/store.go` | ctx, in, UpsertUser |
| `internal/store/users/users.go` | s, id, rows, nullableString, udids, ... |
| `internal/store/users/users_test.go` | ctx, err, t, ctx, r, ... |

## Connected Communities

- **usecase/goal +36 dirs** (26 cross-edges)
- **service/servicetest +33 dirs** (5 cross-edges)
- **service/healthcheckin +6 dirs** (2 cross-edges)
- **auth +67 dirs** (1 cross-edges)
- **auth +32 dirs** (1 cross-edges)
- **store/users · TestSystemAdminCountAndSet** (1 cross-edges)

## How to Explore

```
analyze(operation:"communities", id:"community-193")
explore(operation:"context", task:"understand store · TestSearchUsersInSet", format:"gcx")
```

_`format: "gcx"` returns the [GCX1 compact wire format](../../docs/wire-format.md) — round-trippable, ~27% fewer tokens than JSON. Drop it for JSON output; agents using `@gortex/wire` or the Go `github.com/gortexhq/gcx-go` package decode either._
