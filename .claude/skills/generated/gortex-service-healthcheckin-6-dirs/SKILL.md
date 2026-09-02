---
name: gortex-service-healthcheckin-6-dirs
description: "Work in the service/healthcheckin +6 dirs area — 366 symbols across 10 files (74% cohesion)"
---

# service/healthcheckin +6 dirs

366 symbols | 10 files | 74% cohesion

## When to Use

Use this skill when working on files in:
- ``
- `internal/core/domain/models.go`
- `internal/http/handlers/api/v1/admin/admincommon/admincommon.go`
- `internal/http/handlers/api/v1/helpers_response_test.go`
- `internal/service/healthcheckin/cache.go`
- `internal/service/healthcheckin/healthcheckin.go`
- `internal/service/healthcheckin/healthcheckin_test.go`
- `internal/usecase/keyresult/keyresult.go`
- `internal/usecase/period/bulkstatus.go`
- `internal/usecase/period/overview_test.go`

## Key Files

| File | Symbols |
|------|---------|
| `` | Now, Count |
| `internal/core/domain/models.go` | WorkType, Goal, TeamID, Priority, Parents, ... |
| `internal/http/handlers/api/v1/admin/admincommon/admincommon.go` | now, c, c, PurgeCutoff, depth |
| `internal/http/handlers/api/v1/helpers_response_test.go` | TestMapGoalDetailsIncludesProgressMeta, period, t, detail, result |
| `internal/service/healthcheckin/cache.go` | ttl, NewCache, loader, hcKey, tenantID, ... |
| `internal/service/healthcheckin/healthcheckin.go` | rc, path, weightSum, ok, kr, ... |
| `internal/service/healthcheckin/healthcheckin_test.go` | g1, t, comments, ownerText, makeTeam, ... |
| `internal/usecase/keyresult/keyresult.go` | kr, id, krID, g, scope, ... |
| `internal/usecase/period/bulkstatus.go` | terr, UpdateTeamStatus, team, status, title, ... |
| `internal/usecase/period/overview_test.go` | data, TestComputePeriodOverview_ValidatedCountsAsInProgress, TestServicePeriodOverview_UsesCache, cache, loader, ... |

## Connected Communities

- **service/servicetest +33 dirs** (19 cross-edges)
- **usecase/goal +36 dirs** (15 cross-edges)
- **core/progress +22 dirs** (8 cross-edges)
- **service/healthcheckin · TestComputeCommentScope_LeadDep…** (4 cross-edges)
- **usecase/period · numericKR** (3 cross-edges)
- **http/dto +36 dirs** (2 cross-edges)
- **service/activity +61 dirs** (1 cross-edges)

## How to Explore

```
analyze(operation:"communities", id:"community-276")
explore(operation:"context", task:"understand service/healthcheckin +6 dirs", format:"gcx")
```

_`format: "gcx"` returns the [GCX1 compact wire format](../../docs/wire-format.md) — round-trippable, ~27% fewer tokens than JSON. Drop it for JSON output; agents using `@gortex/wire` or the Go `github.com/gortexhq/gcx-go` package decode either._
