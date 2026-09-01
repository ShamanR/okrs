---
name: gortex-store-14-dirs
description: "Work in the store +14 dirs area — 535 symbols across 20 files (78% cohesion)"
---

# store +14 dirs

535 symbols | 20 files | 78% cohesion

## When to Use

Use this skill when working on files in:
- ``
- `cmd/server/main.go`
- `external-call::dep:github.com/golang-migrate/migrate/v4/database/postgres`
- `external-call::dep:github.com/jackc/pgx/v5/pgxpool`
- `external-call::dep:github.com/testcontainers/testcontainers-go/modules/postgres`
- `external-call::dep:github.com/testcontainers/testcontainers-go/wait`
- `internal/http/handlers/api/v1/activity/handler_test.go`
- `internal/http/handlers/api/v1/goals/access_test.go`
- `internal/http/handlers/api/v1/krs/access_test.go`
- `internal/http/handlers/api/v1/krs/integration_test.go`
- `internal/http/handlers/api/v1/teams/export_integration_test.go`
- `internal/http/handlers/api/v1/teams/integration_test.go`
- `internal/http/handlers/api/v1/testutil/integration.go`
- `internal/http/handlers/api/v1/users/handler_test.go`
- `internal/store/krs/krs.go`
- `internal/store/krs/krs_test.go`
- `internal/store/migration_tenancy_test.go`
- `internal/store/store.go`
- `internal/store/store_test.go`
- `internal/store/testutil/testutil.go`

## Key Files

| File | Symbols |
|------|---------|
| `` | WithTimeout, Open, sql, Get |
| `cmd/server/main.go` | migrationsPath, db, databaseURL, runMigrations, err, ... |
| `external-call::dep:github.com/golang-migrate/migrate/v4/database/postgres` | github.com/golang-migrate/migrate/v4/database/postgres |
| `external-call::dep:github.com/jackc/pgx/v5/pgxpool` | github.com/jackc/pgx/v5/pgxpool |
| `external-call::dep:github.com/testcontainers/testcontainers-go/modules/postgres` | github.com/testcontainers/testcontainers-go/modules/postgres |
| `external-call::dep:github.com/testcontainers/testcontainers-go/wait` | github.com/testcontainers/testcontainers-go/wait |
| `internal/http/handlers/api/v1/activity/handler_test.go` | err, srv, resp2, TestActivityFeedAndTreeCountsEndpoints, err, ... |
| `internal/http/handlers/api/v1/goals/access_test.go` | err, err, container, err, pool, ... |
| `internal/http/handlers/api/v1/krs/access_test.go` | err, ctx, err, t, closure@54, ... |
| `internal/http/handlers/api/v1/krs/integration_test.go` | err, err, TestUpdateKRProgressIntegration, dbURL, closure@377, ... |
| `internal/http/handlers/api/v1/teams/export_integration_test.go` | err, err, err, err, closure@38, ... |
| `internal/http/handlers/api/v1/teams/integration_test.go` | Forecast, err, err, server, historyHierarchyResp, ... |
| `internal/http/handlers/api/v1/testutil/integration.go` | err, cancel, RunMigrations, err, NewAPIV1Router, ... |
| `internal/http/handlers/api/v1/users/handler_test.go` | setupDB, err, container, closure@52, t, ... |
| `internal/store/krs/krs.go` | CreateKeyResult, Kind, GoalID, input, KeyResultInput, ... |
| `internal/store/krs/krs_test.go` | got, krID, TestZeroingCriteriaAllKinds, krID, repo, ... |
| `internal/store/migration_tenancy_test.go` | TestMigration027CreatesDefaultTenant, TestMigration032RemovesTenantDefault, ctx, cleanup, cleanup, ... |
| `internal/store/store.go` | db, krsRepo, New |
| `internal/store/store_test.go` | db, err, t, closure@379, err, ... |
| `internal/store/testutil/testutil.go` | path, databaseURL, runMigrations, m, err, ... |

## Entry Points

- `internal/http/handlers/api/v1/teams/export_integration_test.go::TestTeamExportEndpoint`
- `internal/http/handlers/api/v1/teams/integration_test.go::TestDeletedTeamsVisibilityDependsOnPeriodIntegration`
- `internal/store/store_test.go::TestKRActivityTimestampsUsedForGoalAndTeamUpdates`

## Connected Communities

- **usecase/goal +36 dirs** (47 cross-edges)
- **v1/goals +9 dirs** (43 cross-edges)
- **auth +67 dirs** (15 cross-edges)
- **store/periods +8 dirs** (7 cross-edges)
- **store/krs · TestKRsScopedByTenant** (5 cross-edges)
- **. +4 dirs · resolveMigrationsPath** (5 cross-edges)
- **store/memberships +14 dirs** (5 cross-edges)
- **v1/config +2 dirs** (4 cross-edges)
- **service/keyresult +4 dirs** (2 cross-edges)
- **http/dto +36 dirs** (2 cross-edges)
- **store · TestSearchUsersInSet** (2 cross-edges)
- **store/notificationprefs** (1 cross-edges)
- **store/notifications** (1 cross-edges)
- **store/usersettings** (1 cross-edges)
- **service/servicetest +33 dirs** (1 cross-edges)
- **store/invitations +3 dirs** (1 cross-edges)
- **platform/eventbus +7 dirs** (1 cross-edges)
- **render/notify +5 dirs** (1 cross-edges)
- **store/statuses · TestSetTeamPeriodStatuses_Batch** (1 cross-edges)
- **service/keyresult +1 dirs** (1 cross-edges)
- **store/notificationchannels · TestUpsertIsIdempotentPerChannel** (1 cross-edges)
- **store/progresssnap +3 dirs** (1 cross-edges)
- **render/export +1 dirs · Filename** (1 cross-edges)

## How to Explore

```
analyze(operation:"communities", id:"community-86")
explore(operation:"context", task:"understand store +14 dirs", format:"gcx")
relations(operation:"usages", target:{symbol:"internal/http/handlers/api/v1/teams/export_integration_test.go::TestTeamExportEndpoint"}, format:"gcx")
```

_`format: "gcx"` returns the [GCX1 compact wire format](../../docs/wire-format.md) — round-trippable, ~27% fewer tokens than JSON. Drop it for JSON output; agents using `@gortex/wire` or the Go `github.com/gortexhq/gcx-go` package decode either._
