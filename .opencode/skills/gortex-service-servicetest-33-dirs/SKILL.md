---
name: gortex-service-servicetest-33-dirs
description: "Work in the service/servicetest +33 dirs area — 1132 symbols across 48 files (69% cohesion)"
---

# service/servicetest +33 dirs

1132 symbols | 48 files | 69% cohesion

## When to Use

Use this skill when working on files in:
- ``
- `internal/core/domain/models.go`
- `internal/core/domain/period_status.go`
- `internal/core/domain/period_tree.go`
- `internal/http/dto/goal.go`
- `internal/http/dto/period.go`
- `internal/http/handlers/api/v1/admin/settings/notifications/handler.go`
- `internal/http/handlers/api/v1/helpers_response.go`
- `internal/http/handlers/api/v1/hierarchy/handler.go`
- `internal/http/handlers/api/v1/hierarchy/response.go`
- `internal/http/handlers/api/v1/notifications/preferences/handler_test.go`
- `internal/http/handlers/api/v1/periods/response.go`
- `internal/http/handlers/api/v1/teams/teamscommon/teamscommon.go`
- `internal/http/handlers/web/logout/handler_test.go`
- `internal/render/export/export.go`
- `internal/service/goal/goal.go`
- `internal/service/healthcheckin/cache.go`
- `internal/service/healthcheckin/healthcheckin.go`
- `internal/service/notificationchannel/notificationchannel_test.go`
- `internal/service/period/period.go`
- `internal/service/period/period_test.go`
- `internal/service/servicetest/activity.go`
- `internal/service/servicetest/eventbus.go`
- `internal/service/servicetest/goalstore.go`
- `internal/service/servicetest/store.go`
- `internal/service/team/team.go`
- `internal/service/team/team_test.go`
- `internal/store/activity/activity.go`
- `internal/store/goals/goals.go`
- `internal/store/grants/grants.go`
- `internal/store/grants/grants_cache_test.go`
- `internal/store/krs/krs.go`
- `internal/store/memberships/cache.go`
- `internal/store/periods/periods.go`
- `internal/store/settings/settings.go`
- `internal/store/tenants/cache.go`
- `internal/store/tenantsettings/cache.go`
- `internal/store/users/users.go`
- `internal/usecase/export/export.go`
- `internal/usecase/goaltree/goaltree.go`
- `internal/usecase/healthcheckin/loader.go`
- `internal/usecase/notification/notification_test.go`
- `internal/usecase/okrboard/okrboard.go`
- `internal/usecase/okrboard/okrboard_test.go`
- `internal/usecase/period/bulkstatus.go`
- `internal/usecase/period/bulkstatus_test.go`
- `internal/usecase/user/search_test.go`
- `internal/usecase/user/user.go`

## Key Files

| File | Symbols |
|------|---------|
| `` | SliceStable, make, append |
| `internal/core/domain/models.go` | LeadUDID, Team, CreatedAt, Lead, GoalComment, ... |
| `internal/core/domain/period_status.go` | PeriodStatus |
| `internal/core/domain/period_tree.go` | best, c, best, closure@143, r, ... |
| `internal/http/dto/goal.go` | AuthorUDID, AuthorName, ID, CreatedAt, Text, ... |
| `internal/http/dto/period.go` | Items, PeriodsResponse |
| `internal/http/handlers/api/v1/admin/settings/notifications/handler.go` | secret, v, f, st, out, ... |
| `internal/http/handlers/api/v1/helpers_response.go` | replies, c, replies, r, MapGoalComment |
| `internal/http/handlers/api/v1/hierarchy/handler.go` | filterNodesByScope, seen, nodes, closure@100, filteredChildren, ... |
| `internal/http/handlers/api/v1/hierarchy/response.go` | node |
| `internal/http/handlers/api/v1/notifications/preferences/handler_test.go` | ps, userID, SetAll |
| `internal/http/handlers/api/v1/periods/response.go` | items, newPeriodsResponse, items, views, v |
| `internal/http/handlers/api/v1/teams/teamscommon/teamscommon.go` | item, CollectOverviewUserUDIDs, seen, udids, uid, ... |
| `internal/http/handlers/web/logout/handler_test.go` | DeleteSession, id |
| `internal/render/export/export.go` | task, c, b, suffix, tasks, ... |
| `internal/service/goal/goal.go` | ListCommentsByGoals, ctx, ctx, scope, ListByTeamsPeriod, ... |
| `internal/service/healthcheckin/cache.go` | InvalidateAll |
| `internal/service/healthcheckin/healthcheckin.go` | result, seen, userUDID, next, kr, ... |
| `internal/service/notificationchannel/notificationchannel_test.go` | out, out, List, c |
| `internal/service/period/period.go` | p, ListViews, ctx, src, src, ... |
| `internal/service/period/period_test.go` | arch, err, TestListPeriodViews_ExcludesArchivedForPublic, pub, all, ... |
| `internal/service/servicetest/activity.go` | scope, RecordBatch, evs |
| `internal/service/servicetest/eventbus.go` | Publish, ev, evs, PublishBatch |
| `internal/service/servicetest/goalstore.go` | DeleteGoalComment, UpsertNumericalMeta, id, DeleteGoal, commentID, ... |
| `internal/service/servicetest/store.go` | items, HardDeleteTeam, ListDeletedTeams, id, teamID, ... |
| `internal/service/team/team.go` | ok, team, walk, hasGoals, node, ... |
| `internal/service/team/team_test.go` | t, err, closure@167, deletedAt, ok, ... |
| `internal/store/activity/activity.go` | rows, scope, err, sql, TreeCounts, ... |
| `internal/store/goals/goals.go` | result, goalIDs, kr, krsSlice, err, ... |
| `internal/store/grants/grants.go` | UserID, ID, CreatedAt, err, TenantID, ... |
| `internal/store/grants/grants_cache_test.go` | cp, g, userID, userID, filtered, ... |
| `internal/store/krs/krs.go` | scope, doneValues, err, raw, scope, ... |
| `internal/store/memberships/cache.go` | InvalidateAll |
| `internal/store/periods/periods.go` | ctx, ListPeriods, scope, err, periods, ... |
| `internal/store/settings/settings.go` | rows, out, ctx, ListAll, err, ... |
| `internal/store/tenants/cache.go` | t, b, ttl, InvalidateAll, newTenantCacheWithBackend, ... |
| `internal/store/tenantsettings/cache.go` | InvalidateAll |
| `internal/store/users/users.go` | err, udids, missing, err, u, ... |
| `internal/usecase/export/export.go` | allowed, team, goals, err, teamIDs, ... |
| `internal/usecase/goaltree/goaltree.go` | GoalTreeNode, ledByMe, GoalTreeTeam, teamIDSet, GoalTree, ... |
| `internal/usecase/healthcheckin/loader.go` | closure@36, NewPeriodLoader, Goals, Teams, Statuses, ... |
| `internal/usecase/notification/notification_test.go` | Resolve, out, targets, scope, i, ... |
| `internal/usecase/okrboard/okrboard.go` | child, periodID, value, scope, totalProgress, ... |
| `internal/usecase/okrboard/okrboard_test.go` | deletedAt, t, err, err, service, ... |
| `internal/usecase/period/bulkstatus.go` | BulkSetTeamPeriodStatus, t, target, affected, skipped, ... |
| `internal/usecase/period/bulkstatus_test.go` | svc, res, TestBulkSetTeamPeriodStatus_TeamFilterRestrictsToScope, err, t, ... |
| `internal/usecase/user/search_test.go` | t, users, svc, st, st, ... |
| `internal/usecase/user/user.go` | deps, userID, userGrants, Grants, eligibleIDs, ... |

## Entry Points

- `internal/store/goals/goals.go::GoalRepository.ListGoalsByTeamsPeriod`

## Connected Communities

- **usecase/goal +36 dirs** (122 cross-edges)
- **core/progress +22 dirs** (14 cross-edges)
- **service/activity +61 dirs** (9 cross-edges)
- **usecase/okrboard +3 dirs** (5 cross-edges)
- **service/healthcheckin +6 dirs** (5 cross-edges)
- **store/periods +8 dirs** (4 cross-edges)
- **http +3 dirs** (3 cross-edges)
- **. +1 dirs · writeKR** (2 cross-edges)
- **http/dto +36 dirs** (2 cross-edges)
- **service/notificationpref +3 dirs** (2 cross-edges)
- **core/domain · PeriodStatusFor** (1 cross-edges)
- **service/goal +8 dirs** (1 cross-edges)
- **auth +4 dirs · TestUsersEndpoint_ScopedSearch_…** (1 cross-edges)
- **auth +67 dirs** (1 cross-edges)
- **usecase/period +1 dirs** (1 cross-edges)
- **auth +6 dirs** (1 cross-edges)
- **usecase** (1 cross-edges)
- **auth +32 dirs** (1 cross-edges)
- **auth +4 dirs · fillGoalRefProgress** (1 cross-edges)

## How to Explore

```
analyze(operation:"communities", id:"community-144")
explore(operation:"context", task:"understand service/servicetest +33 dirs", format:"gcx")
relations(operation:"usages", target:{symbol:"internal/store/goals/goals.go::GoalRepository.ListGoalsByTeamsPeriod"}, format:"gcx")
```

_`format: "gcx"` returns the [GCX1 compact wire format](../../docs/wire-format.md) — round-trippable, ~27% fewer tokens than JSON. Drop it for JSON output; agents using `@gortex/wire` or the Go `github.com/gortexhq/gcx-go` package decode either._
