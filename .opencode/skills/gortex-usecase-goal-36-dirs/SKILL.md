---
name: gortex-usecase-goal-36-dirs
description: "Work in the usecase/goal +36 dirs area — 1936 symbols across 81 files (68% cohesion)"
---

# usecase/goal +36 dirs

1936 symbols | 81 files | 68% cohesion

## When to Use

Use this skill when working on files in:
- ``
- `internal/auth/context_test.go`
- `internal/auth/policy_test.go`
- `internal/core/domain/models.go`
- `internal/core/domain/scope.go`
- `internal/core/domain/tenant.go`
- `internal/core/domain/tenant_test.go`
- `internal/core/event/diff_test.go`
- `internal/core/event/event_test.go`
- `internal/core/event/events.go`
- `internal/http/handlers/api/v1/admin/users/handler_test.go`
- `internal/http/handlers/api/v1/goaltree/handler_test.go`
- `internal/http/handlers/api/v1/krs/krscommon/krscommon.go`
- `internal/http/handlers/api/v1/notifications/preferences/handler_test.go`
- `internal/http/handlers/web/common/common.go`
- `internal/render/notify/notify_test.go`
- `internal/service/activity/activity_test.go`
- `internal/service/activity/feed_test.go`
- `internal/service/activity/journal.go`
- `internal/service/activity/journal_test.go`
- `internal/service/goallink/goallink.go`
- `internal/service/goalshare/goalshare.go`
- `internal/service/healthcheckin/cache.go`
- `internal/service/healthcheckin/cache_test.go`
- `internal/service/healthcheckin/healthcheckin_test.go`
- `internal/service/notification/notification_test.go`
- `internal/service/notificationchannel/notificationchannel.go`
- `internal/service/notificationchannel/notificationchannel_test.go`
- `internal/service/notificationpref/notificationpref_test.go`
- `internal/service/servicetest/eventbus.go`
- `internal/service/servicetest/goalstore.go`
- `internal/service/team/team.go`
- `internal/service/team/team_test.go`
- `internal/store/activity/activity.go`
- `internal/store/activity/activity_isolation_test.go`
- `internal/store/activity/activity_test.go`
- `internal/store/goallinks/goallinks.go`
- `internal/store/goals/copy_test.go`
- `internal/store/goals/goals.go`
- `internal/store/goals/goals_comments_test.go`
- `internal/store/goals/goals_isolation_test.go`
- `internal/store/goals/goals_reorder_test.go`
- `internal/store/goals/goals_tree_test.go`
- `internal/store/grants/grants.go`
- `internal/store/grants/grants_cache_test.go`
- `internal/store/grants/grants_isolation_test.go`
- `internal/store/grants/grants_repository_test.go`
- `internal/store/invitations/invitations_test.go`
- `internal/store/krs/krs.go`
- `internal/store/krs/krs_test.go`
- `internal/store/notificationprefs/notificationprefs_test.go`
- `internal/store/notifications/notifications_test.go`
- `internal/store/periods/periods.go`
- `internal/store/shares/shares.go`
- `internal/store/shares/shares_isolation_test.go`
- `internal/store/shares/shares_test.go`
- `internal/store/teams/teams.go`
- `internal/store/teams/teams_isolation_test.go`
- `internal/store/teams/teams_test.go`
- `internal/store/testutil/testutil.go`
- `internal/store/users/users_test.go`
- `internal/usecase/goal/activity_test.go`
- `internal/usecase/goal/comments.go`
- `internal/usecase/goal/copy_goal_test.go`
- `internal/usecase/goal/goal.go`
- `internal/usecase/goal/goal_links_test.go`
- `internal/usecase/goal/goal_test.go`
- `internal/usecase/goal/repodeps_test.go`
- `internal/usecase/goal/service_test.go`
- `internal/usecase/keyresult/activity_test.go`
- `internal/usecase/keyresult/checkin_test.go`
- `internal/usecase/keyresult/goal_test.go`
- `internal/usecase/keyresult/keyresult.go`
- `internal/usecase/keyresult/progress_test.go`
- `internal/usecase/keyresult/repodeps_test.go`
- `internal/usecase/keyresult/service_test.go`
- `internal/usecase/notification/notification_test.go`
- `internal/usecase/okrboard/okrboard_test.go`
- `internal/usecase/period/activity_test.go`
- `internal/usecase/period/bulkstatus_test.go`
- `internal/usecase/period/overview_test.go`

## Key Files

| File | Symbols |
|------|---------|
| `` | len, Background, Sleep |
| `internal/auth/context_test.go` | TestTenantFromContextNilWhenAbsent, got, t, ok |
| `internal/auth/policy_test.go` | ctx, got, t, want, i, ... |
| `internal/core/domain/models.go` | TargetValue, EntityTitle, CreatedAt, Priority, KRNumerical, ... |
| `internal/core/domain/scope.go` | TenantID, TenantScope |
| `internal/core/domain/tenant.go` | isLower, ValidTenantSlug, c, i, isDigit, ... |
| `internal/core/domain/tenant_test.go` | cases, t, want, got, TestTenantSlugValid, ... |
| `internal/core/event/diff_test.go` | got, ok, t, TestDiffKeepsOnlyChanged |
| `internal/core/event/event_test.go` | TestKREventsCarryGoalID, seen, ev, ev, k, ... |
| `internal/core/event/events.go` | HealthBefore, KRID, NoteAfter, KRKind, ProgressBefore, ... |
| `internal/http/handlers/api/v1/admin/users/handler_test.go` | w, h, t, r, TestHandleListUsersIsTenantScopedWithStatus, ... |
| `internal/http/handlers/api/v1/goaltree/handler_test.go` | quarterGoal, leadSrv, pool, ctx, err, ... |
| `internal/http/handlers/api/v1/krs/krscommon/krscommon.go` | weight, ParseProjectStages, weights, weightValue, dones, ... |
| `internal/http/handlers/api/v1/notifications/preferences/handler_test.go` | h, items, t, TestPutReplacesWholeMatrix, t, ... |
| `internal/http/handlers/web/common/common.go` | values, r, seen, out, out, ... |
| `internal/render/notify/notify_test.go` | got, TestRenderCoversEveryNotifyingKind, kinds, t, k |
| `internal/service/activity/activity_test.go` | t, TestRecordWithNilRepoIsNoOp, t, svc, svc, ... |
| `internal/service/activity/feed_test.go` | f, back, List, err, repo, ... |
| `internal/service/activity/journal.go` | ToRowsForTest, ev |
| `internal/service/activity/journal_test.go` | base, closure@363, closure@404, rows, TestToRowSkipsUnmappedEvents, ... |
| `internal/service/goallink/goallink.go` | Repo, goals, New, repo, GoalProgressReader |
| `internal/service/goalshare/goalshare.go` | scope, List, goalID, ctx, goalID, ... |
| `internal/service/healthcheckin/cache.go` | ctx, Get, entry, periodID, scope, ... |
| `internal/service/healthcheckin/cache_test.go` | loader, err, ctx, c, hcKeyProbe, ... |
| `internal/service/healthcheckin/healthcheckin_test.go` | s, t, now, gOwner, gShared, ... |
| `internal/service/notification/notification_test.go` | svc, t, next, TestListEncodesNextCursor, err, ... |
| `internal/service/notificationchannel/notificationchannel.go` | out, out, name, Descriptors |
| `internal/service/notificationchannel/notificationchannel_test.go` | TestDescriptorsShowsBuildWhileAvailableShowsGranted, t, err, svc, got |
| `internal/service/notificationpref/notificationpref_test.go` | TestSetAllWritesEveryRow, t, svc, repo, err |
| `internal/service/servicetest/eventbus.go` | FakeBus, Events, mu |
| `internal/service/servicetest/goalstore.go` | goalIDs, ListGoalSharesByGoalIDs, result, sl, id |
| `internal/service/team/team.go` | scope, Delete, err, hasGoals, ctx, ... |
| `internal/service/team/team_test.go` | err, t, TestDeleteTeamUsesHardDeleteWhenTeamHasNoGoals, children, missing, ... |
| `internal/store/activity/activity.go` | out, scope, Purge, arg, ctx, ... |
| `internal/store/activity/activity_isolation_test.go` | recent, idOld, err, s1, team1, ... |
| `internal/store/activity/activity_test.go` | scope, ar, scope, ctx, evs, ... |
| `internal/store/goallinks/goallinks.go` | db, GoalLinkRepository, db, NewGoalLinkRepository |
| `internal/store/goals/copy_test.go` | err, srcGoal, ctx, err, repo, ... |
| `internal/store/goals/goals.go` | krsRepo, err, scope, authorUserID, CreateGoal, ... |
| `internal/store/goals/goals_comments_test.go` | err, ctx, cleanup, gr, pr, ... |
| `internal/store/goals/goals_isolation_test.go` | own, pool, ctx, err, TestGoalsScopedByTenant, ... |
| `internal/store/goals/goals_reorder_test.go` | gr, err, owner, ownerList, closure@73, ... |
| `internal/store/goals/goals_tree_test.go` | scope, gr, err, scoped, pool, ... |
| `internal/store/grants/grants.go` | scope, grantedByUserID, RemoveAllUserGrants, ListUserGrants, err, ... |
| `internal/store/grants/grants_cache_test.go` | err, cache, grants, grants, ctx, ... |
| `internal/store/grants/grants_isolation_test.go` | gs1, teamID, cleanup, TestGrantsScopedByTenant, err, ... |
| `internal/store/grants/grants_repository_test.go` | t2, g8, cleanup, g7, err, ... |
| `internal/store/invitations/invitations_test.go` | scope, TestListPendingByTenantFields, ctx, got, err, ... |
| `internal/store/krs/krs.go` | db, NewKRRepository |
| `internal/store/krs/krs_test.go` | err, ctx, repo, num, TestBatchLoadNotes_AbsentKR, ... |
| `internal/store/notificationprefs/notificationprefs_test.go` | byOrd, pool, pool, cleanup, ctx, ... |
| `internal/store/notifications/notifications_test.go` | scope, n, repo, n, err, ... |
| `internal/store/periods/periods.go` | db, NewPeriodRepository |
| `internal/store/shares/shares.go` | scope, scope, closure@91, DeleteGoalShare, teamIDs, ... |
| `internal/store/shares/shares_isolation_test.go` | err, err, goalID, err, err, ... |
| `internal/store/shares/shares_test.go` | list, TestGetGoalShare, goalID, t, cleanup, ... |
| `internal/store/teams/teams.go` | scope, Description, input, err, Name, ... |
| `internal/store/teams/teams_isolation_test.go` | b, ctx, err, err, cleanup, ... |
| `internal/store/teams/teams_test.go` | TestHardDeleteReparentsChildren, cleanup, err, ctx, r, ... |
| `internal/store/testutil/testutil.go` | err, tbl, err, container, SetupDB, ... |
| `internal/store/users/users_test.go` | t, err, list, r, err, ... |
| `internal/usecase/goal/activity_test.go` | err, ev, s, isTask, TestDeleteGoalByOwnerWithSharesRecordsOwnerChange, ... |
| `internal/usecase/goal/comments.go` | periodID, AddReply, parentID, authorUserID, teamID, ... |
| `internal/usecase/goal/copy_goal_test.go` | scope, svc, t, gf, flipped, ... |
| `internal/usecase/goal/goal.go` | err, targets, ownerWeight, beforeSet, teamID, ... |
| `internal/usecase/goal/goal_links_test.go` | pid, t, ctx, gr, pool, ... |
| `internal/usecase/goal/goal_test.go` | st, svc, TestDeleteGoalByOwnerTransfersOwnershipWhenShared, err, TestCreateGoalKeepsStatusWhenAlreadyForming, ... |
| `internal/usecase/goal/repodeps_test.go` | Events, rawDeps, Statuses, Teams, gf, ... |
| `internal/usecase/goal/service_test.go` | TestShareGoalRejectsStartedPeriodTarget, svc, store, err, newTestService, ... |
| `internal/usecase/keyresult/activity_test.go` | bus, TestKRNoteUpdateRecordsEvent, err, bus, ev, ... |
| `internal/usecase/keyresult/checkin_test.go` | s, current, st, err, ev, ... |
| `internal/usecase/keyresult/goal_test.go` | t, st, svc, st, t, ... |
| `internal/usecase/keyresult/keyresult.go` | scope, CheckInInput, Numerical, nerr, Note, ... |
| `internal/usecase/keyresult/progress_test.go` | updates, store, TestResaveAt100DoesNotOverrideManualHealth, t, t, ... |
| `internal/usecase/keyresult/repodeps_test.go` | v, store, store, gf, ptr, ... |
| `internal/usecase/keyresult/service_test.go` | service, err, service, TestUpdateKRProgressBoolean, store, ... |
| `internal/usecase/notification/notification_test.go` | uc, w, m1, w, err, ... |
| `internal/usecase/okrboard/okrboard_test.go` | now, t, store, svc, overview, ... |
| `internal/usecase/period/activity_test.go` | err, st, ok, t, ev, ... |
| `internal/usecase/period/bulkstatus_test.go` | bus, svc, store, err, res, ... |
| `internal/usecase/period/overview_test.go` | b, goals, t, TestComputeBalances_CountsAndPercentsWithFixedOrder |

## Connected Communities

- **service/servicetest +33 dirs** (105 cross-edges)
- **auth +67 dirs** (31 cross-edges)
- **store/periods +8 dirs** (20 cross-edges)
- **service/activity +61 dirs** (17 cross-edges)
- **v1/goals +9 dirs** (17 cross-edges)
- **service/activity +86 dirs** (16 cross-edges)
- **usecase/notification +4 dirs** (13 cross-edges)
- **service/healthcheckin +6 dirs** (13 cross-edges)
- **store +14 dirs** (12 cross-edges)
- **service/notificationchannel +10 dirs** (11 cross-edges)
- **service/goal +8 dirs** (9 cross-edges)
- **store · TestSearchUsersInSet** (7 cross-edges)
- **auth +32 dirs** (7 cross-edges)
- **service/keyresult +4 dirs** (6 cross-edges)
- **store/goals · TestSetGoalCommentResolvedIdemp…** (6 cross-edges)
- **usecase/okrboard +3 dirs** (6 cross-edges)
- **service/healthcheckin · TestComputeCommentScope_LeadDep…** (6 cross-edges)
- **http/dto +36 dirs** (5 cross-edges)
- **store/grants · TestListDescendantTeamIDsTree** (5 cross-edges)
- **usecase/goal +1 dirs · Copy** (5 cross-edges)
- **store/teams · TestSoftDeleteReparentsChildren** (5 cross-edges)
- **store/invitations +3 dirs** (5 cross-edges)
- **store/notifications** (5 cross-edges)
- **store/notificationprefs** (4 cross-edges)
- **service/notificationpref +3 dirs** (3 cross-edges)
- **store/memberships +14 dirs** (3 cross-edges)
- **core/event +2 dirs · TestToRowCoversEveryEventType** (3 cross-edges)
- **render/export +1 dirs · Markdown** (3 cross-edges)
- **core/event +2 dirs · UpdateWithMeta** (2 cross-edges)
- **store/krs · TestKRsScopedByTenant** (2 cross-edges)
- **service/activity · Feed** (2 cross-edges)
- **store/teams · HardDeleteTeam** (2 cross-edges)
- **v1/config +2 dirs** (2 cross-edges)
- **. +4 dirs · ListByUser** (2 cross-edges)
- **http +3 dirs** (2 cross-edges)
- **usecase/period +1 dirs** (2 cross-edges)
- **core/progress +22 dirs** (2 cross-edges)
- **usecase/okrboard · TestGetTeamsWithPeriodSummaryKe…** (1 cross-edges)
- **auth · ctxWithAllowedTeams** (1 cross-edges)
- **usecase/period · bucketsFor** (1 cross-edges)
- **auth +4 dirs · TestUsersEndpoint_ScopedSearch_…** (1 cross-edges)
- **usecase/goal +1 dirs · Delete** (1 cross-edges)
- **core/domain +1 dirs** (1 cross-edges)
- **render/notify +5 dirs** (1 cross-edges)
- **store/teams · scanTeams** (1 cross-edges)
- **. +2 dirs · startNotificationRetentionLoop** (1 cross-edges)
- **. +1 dirs · writeKR** (1 cross-edges)
- **usecase/notification +1 dirs · ResolveAddressed** (1 cross-edges)
- **usecase/goal · Deps** (1 cross-edges)
- **. +4 dirs · ParseNumericalMeta** (1 cross-edges)
- **usecase/notification +1 dirs · anchorOf** (1 cross-edges)
- **store/krs · ApplyCheckIn** (1 cross-edges)

## How to Explore

```
analyze(operation:"communities", id:"community-148")
explore(operation:"context", task:"understand usecase/goal +36 dirs", format:"gcx")
```

_`format: "gcx"` returns the [GCX1 compact wire format](../../docs/wire-format.md) — round-trippable, ~27% fewer tokens than JSON. Drop it for JSON output; agents using `@gortex/wire` or the Go `github.com/gortexhq/gcx-go` package decode either._
