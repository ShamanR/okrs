---
name: gortex-store-periods-8-dirs
description: "Work in the store/periods +8 dirs area — 253 symbols across 13 files (76% cohesion)"
---

# store/periods +8 dirs

253 symbols | 13 files | 76% cohesion

## When to Use

Use this skill when working on files in:
- ``
- `cmd/server/main.go`
- `internal/core/domain/period_status_test.go`
- `internal/http/handlers/api/v1/helpers_response.go`
- `internal/http/handlers/api/v1/helpers_response_test.go`
- `internal/scheduler/scheduler.go`
- `internal/service/healthcheckin/healthcheckin_test.go`
- `internal/service/period/period.go`
- `internal/store/periods/periods.go`
- `internal/store/periods/periods_isolation_test.go`
- `internal/store/periods/periods_test.go`
- `internal/usecase/period/bulkstatus_test.go`
- `internal/usecase/period/progress_test.go`

## Key Files

| File | Symbols |
|------|---------|
| `` | Date, Month, FixedZone, time |
| `cmd/server/main.go` | quarterPeriod, quarter, end, startMonth, year, ... |
| `internal/core/domain/period_status_test.go` | end, start, t, got, got, ... |
| `internal/http/handlers/api/v1/helpers_response.go` | duration, period, CalculatePeriodForecast, elapsed, now, ... |
| `internal/http/handlers/api/v1/helpers_response_test.go` | t, period, TestCalculatePeriodForecastBounds, got, got, ... |
| `internal/scheduler/scheduler.go` | tn, a, has, latest, latest, ... |
| `internal/service/healthcheckin/healthcheckin_test.go` | pace, TestCalcExpectedPace, t, p, now |
| `internal/service/period/period.go` | ctx, ctx, periodID, Delete, periodID, ... |
| `internal/store/periods/periods.go` | periodID, err, date, rows, ctx, ... |
| `internal/store/periods/periods_isolation_test.go` | scope2, r, cleanup, err, TestPeriodsScopedByTenant, ... |
| `internal/store/periods/periods_test.go` | err, ids, ctx, err, err, ... |
| `internal/usecase/period/bulkstatus_test.go` | goalsByTeam, skipped, affected, t, deleted, ... |
| `internal/usecase/period/progress_test.go` | start, rows, end, end, d1, ... |

## Connected Communities

- **usecase/goal +36 dirs** (26 cross-edges)
- **core/progress +22 dirs** (7 cross-edges)
- **service/servicetest +33 dirs** (6 cross-edges)
- **core/domain · PeriodStatusFor** (2 cross-edges)
- **service/healthcheckin +3 dirs** (1 cross-edges)
- **service/activity +61 dirs** (1 cross-edges)
- **v1/goals +9 dirs** (1 cross-edges)
- **service/healthcheckin +6 dirs** (1 cross-edges)

## How to Explore

```
analyze(operation:"communities", id:"community-275")
explore(operation:"context", task:"understand store/periods +8 dirs", format:"gcx")
```

_`format: "gcx"` returns the [GCX1 compact wire format](../../docs/wire-format.md) — round-trippable, ~27% fewer tokens than JSON. Drop it for JSON output; agents using `@gortex/wire` or the Go `github.com/gortexhq/gcx-go` package decode either._
