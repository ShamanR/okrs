---
name: gortex-service-activity-61-dirs
description: "Work in the service/activity +61 dirs area — 1236 symbols across 77 files (66% cohesion)"
---

# service/activity +61 dirs

1236 symbols | 77 files | 66% cohesion

## When to Use

Use this skill when working on files in:
- ``
- `internal/auth/policy.go`
- `internal/auth/policy_test.go`
- `internal/core/domain/models.go`
- `internal/core/event/event_test.go`
- `internal/http/handlers/api/v1/activity/categorycounts/handler_test.go`
- `internal/http/handlers/api/v1/admin/accessrequests/approve/handler.go`
- `internal/http/handlers/api/v1/admin/accessrequests/approve/handler_test.go`
- `internal/http/handlers/api/v1/admin/accessrequests/deny/handler_test.go`
- `internal/http/handlers/api/v1/admin/admincommon/admincommon.go`
- `internal/http/handlers/api/v1/admin/members/handler_test.go`
- `internal/http/handlers/api/v1/admin/periods/archive/handler_test.go`
- `internal/http/handlers/api/v1/admin/users/admin/handler.go`
- `internal/http/handlers/api/v1/admin/users/admin/handler_test.go`
- `internal/http/handlers/api/v1/admin/users/handler_test.go`
- `internal/http/handlers/api/v1/goals/comments/resolve/handler_test.go`
- `internal/http/handlers/api/v1/goals/comments/unresolve/handler_test.go`
- `internal/http/handlers/api/v1/goals/goalcommon/goalcommon.go`
- `internal/http/handlers/api/v1/goals/movedown/handler_test.go`
- `internal/http/handlers/api/v1/goals/moveup/handler_test.go`
- `internal/http/handlers/api/v1/goaltree/handler.go`
- `internal/http/handlers/api/v1/goaltree/handler_test.go`
- `internal/http/handlers/api/v1/krs/krscommon/krscommon.go`
- `internal/http/handlers/api/v1/notifications/preferences/handler_test.go`
- `internal/http/handlers/api/v1/onboarding/joinrequest/handler_test.go`
- `internal/http/handlers/api/v1/onboarding/onboardingcommon/onboardingcommon.go`
- `internal/http/handlers/api/v1/periods/overview/handler_test.go`
- `internal/http/handlers/api/v1/periods/teams/activate/handler_test.go`
- `internal/http/handlers/api/v1/session/memberships/handler_test.go`
- `internal/http/handlers/api/v1/session/tenant/handler_test.go`
- `internal/http/handlers/api/v1/session/tenants/handler_test.go`
- `internal/http/handlers/api/v1/system/systemcommon/systemcommon.go`
- `internal/http/handlers/api/v1/system/tenants/entitlements/handler_test.go`
- `internal/http/handlers/api/v1/system/tenants/handler_test.go`
- `internal/http/handlers/api/v1/system/tenants/restore/handler.go`
- `internal/http/handlers/api/v1/system/tenants/restore/handler_test.go`
- `internal/http/handlers/api/v1/system/tenants/suspend/handler_test.go`
- `internal/http/handlers/web/goals/delete/handler.go`
- `internal/http/handlers/web/invite/handler_test.go`
- `internal/http/handlers/web/logout/handler_test.go`
- `internal/service/activity/activity.go`
- `internal/service/activity/activity_test.go`
- `internal/service/activity/feed_test.go`
- `internal/service/activity/journal.go`
- `internal/service/goal/goal.go`
- `internal/service/goallink/goallink.go`
- `internal/service/goalshare/goalshare.go`
- `internal/service/keyresult/keyresult.go`
- `internal/service/notification/notification.go`
- `internal/service/notification/notification_test.go`
- `internal/service/notificationchannel/notificationchannel_test.go`
- `internal/service/period/period.go`
- `internal/service/progresssnap/progresssnap.go`
- `internal/service/servicetest/activity.go`
- `internal/service/servicetest/goalstore.go`
- `internal/service/servicetest/store.go`
- `internal/service/team/team.go`
- `internal/service/teamstatus/teamstatus.go`
- `internal/service/user/user.go`
- `internal/store/goals/copy.go`
- `internal/store/goals/goals.go`
- `internal/store/grants/grants.go`
- `internal/store/grants/grants_cache_test.go`
- `internal/store/krs/krs.go`
- `internal/store/krs/krs_test.go`
- `internal/store/memberships/cache_test.go`
- `internal/store/migration_kr_health_test.go`
- `internal/store/progresssnap/progresssnap.go`
- `internal/store/shares/shares.go`
- `internal/store/store.go`
- `internal/store/teams/teams.go`
- `internal/store/tenants/cache_test.go`
- `internal/store/users/users.go`
- `internal/store/usersettings/usersettings.go`
- `internal/usecase/goal/comments.go`
- `internal/usecase/notification/notification_test.go`
- `internal/usecase/user/search_test.go`

## Key Files

| File | Symbols |
|------|---------|
| `` | int64, error, New, bool |
| `internal/auth/policy.go` | st, ctx, err, err, scope, ... |
| `internal/auth/policy_test.go` | fakeGrants, id, out, ListUserGrants, rootIDs, ... |
| `internal/core/domain/models.go` | KeyResultID, Weight, KRProjectStage, ArchivedAt, EndDate, ... |
| `internal/core/event/event_test.go` | TestMetaIsEmbedded, t, ev, teamID |
| `internal/http/handlers/api/v1/activity/categorycounts/handler_test.go` | ids, CountByCategory, flt |
| `internal/http/handlers/api/v1/admin/accessrequests/approve/handler.go` | onboard |
| `internal/http/handlers/api/v1/admin/accessrequests/approve/handler_test.go` | err, RemoveMember, id, id, userID, ... |
| `internal/http/handlers/api/v1/admin/accessrequests/deny/handler_test.go` | err, ApproveRequest, id, RemoveMember, userID, ... |
| `internal/http/handlers/api/v1/admin/admincommon/admincommon.go` | MemberRoleSetter, GrantsStore |
| `internal/http/handlers/api/v1/admin/members/handler_test.go` | ApproveRequest, id, fakeOnboard, ListAccessRequests, DenyRequest, ... |
| `internal/http/handlers/api/v1/admin/periods/archive/handler_test.go` | fakePeriodRepo, ArchivePeriod, UpdatePeriod, period, CreatePeriod, ... |
| `internal/http/handlers/api/v1/admin/users/admin/handler.go` | roles |
| `internal/http/handlers/api/v1/admin/users/admin/handler_test.go` | userID, role, fakeRoles, gotRole, gotUser, ... |
| `internal/http/handlers/api/v1/admin/users/handler_test.go` | ListUserGrants, AddUserGrant, fakeGrants, RemoveUserGrant, leadScopeCalled, ... |
| `internal/http/handlers/api/v1/goals/comments/resolve/handler_test.go` | SetCommentResolved, resolved, Get |
| `internal/http/handlers/api/v1/goals/comments/unresolve/handler_test.go` | resolved, Get, SetCommentResolved |
| `internal/http/handlers/api/v1/goals/goalcommon/goalcommon.go` | Mover |
| `internal/http/handlers/api/v1/goals/movedown/handler_test.go` | goalID, teamID, Get, s, teamID, ... |
| `internal/http/handlers/api/v1/goals/moveup/handler_test.go` | teamID, s, Get, goalID, teamID, ... |
| `internal/http/handlers/api/v1/goaltree/handler.go` | emptyIfNil, ids |
| `internal/http/handlers/api/v1/goaltree/handler_test.go` | findGoal, id, g, tr, t |
| `internal/http/handlers/api/v1/krs/krscommon/krscommon.go` | KRMover |
| `internal/http/handlers/api/v1/notifications/preferences/handler_test.go` | GetAll, userID |
| `internal/http/handlers/api/v1/onboarding/joinrequest/handler_test.go` | RemoveMember, fakeOnboard, slug, RequestAccess, ListAccessRequests, ... |
| `internal/http/handlers/api/v1/onboarding/onboardingcommon/onboardingcommon.go` | OnboardService |
| `internal/http/handlers/api/v1/periods/overview/handler_test.go` | id, AddUserGrant, udid, AllGrants, all, ... |
| `internal/http/handlers/api/v1/periods/teams/activate/handler_test.go` | ListUserGrants, AddUserGrant, ListDescendantTeamIDs, fakeGrants, id, ... |
| `internal/http/handlers/api/v1/session/memberships/handler_test.go` | id, tenantID, ok, GetByID, SetActiveTenant, ... |
| `internal/http/handlers/api/v1/session/tenant/handler_test.go` | tenantID, t, ok, id, GetByID, ... |
| `internal/http/handlers/api/v1/session/tenants/handler_test.go` | id, LeaveTenant, tenantID, ok, t, ... |
| `internal/http/handlers/api/v1/system/systemcommon/systemcommon.go` | Provisioner |
| `internal/http/handlers/api/v1/system/tenants/entitlements/handler_test.go` | UpdateTenant, SetSystemAdmin, SetMemberRole, AttachMember, CreateTenant, ... |
| `internal/http/handlers/api/v1/system/tenants/handler_test.go` | SetMemberRole, created, entCalls, Restore, SetSystemAdmin, ... |
| `internal/http/handlers/api/v1/system/tenants/restore/handler.go` | prov |
| `internal/http/handlers/api/v1/system/tenants/restore/handler_test.go` | fakeProv, err, DenyMember, id, called, ... |
| `internal/http/handlers/api/v1/system/tenants/suspend/handler_test.go` | SetMemberRole, fakeProv, err, id, UpdateTenant, ... |
| `internal/http/handlers/web/goals/delete/handler.go` | r, w, teamID, redirectToTeam, periodID |
| `internal/http/handlers/web/invite/handler_test.go` | EnsureRegistration |
| `internal/http/handlers/web/logout/handler_test.go` | AnySystemAdmin, UpsertUser, GetSession, SetSystemAdmin, fakeStore, ... |
| `internal/service/activity/activity.go` | scope, Purge, allowedTeamIDs, scope, olderThan, ... |
| `internal/service/activity/activity_test.go` | Record, Purge, ev, RecordBatch, evs |
| `internal/service/activity/feed_test.go` | Record, Purge |
| `internal/service/activity/journal.go` | srcPeriod, copyPayload, withComments, srcTeam, srcGoal, ... |
| `internal/service/goal/goal.go` | scope, scope, scope, ctx, ctx, ... |
| `internal/service/goallink/goallink.go` | parentIDs, ctx, adminAll, childID, ReplaceParents, ... |
| `internal/service/goalshare/goalshare.go` | ctx, ctx, ListByGoalIDs, goalID, Repo, ... |
| `internal/service/keyresult/keyresult.go` | scope, ctx, authorUserID, krID, UpdateNumericalCurrent, ... |
| `internal/service/notification/notification.go` | ids, ctx, scope, all, Create, ... |
| `internal/service/notification/notification_test.go` | Insert, Delete |
| `internal/service/notificationchannel/notificationchannel_test.go` | ch, c, Get, ok |
| `internal/service/period/period.go` | ctx, scope, periodID, Repo, Get |
| `internal/service/progresssnap/progresssnap.go` | List, ctx, ctx, teamIDs, periodID, ... |
| `internal/service/servicetest/activity.go` | ev, Record, Purge |
| `internal/service/servicetest/goalstore.go` | GoalID, SetGoalCommentResolved, ListUserLeadTeams, UpsertBoolCalls, Weight, ... |
| `internal/service/servicetest/store.go` | TeamHasGoals, periodID, teamID, stageID, Store, ... |
| `internal/service/team/team.go` | hasGoals, scope, Repo, id, team, ... |
| `internal/service/teamstatus/teamstatus.go` | List, ctx, status, Repo, Get, ... |
| `internal/service/user/user.go` | Repo |
| `internal/store/goals/copy.go` | tasks, dstGoalID, replies, author, text, ... |
| `internal/store/goals/goals.go` | err, rows, err, err, ListTeamLastGoalUpdateInPeriod, ... |
| `internal/store/grants/grants.go` | userID, userUDID, userID, ctx, ctx, ... |
| `internal/store/grants/grants_cache_test.go` | ListDescendantTeamIDs, rootIDs |
| `internal/store/krs/krs.go` | scope, UpdateKeyResultDescription, BatchLoadNotes, scope, ctx, ... |
| `internal/store/krs/krs_test.go` | notes2, pool, notes, ctx, err, ... |
| `internal/store/memberships/cache_test.go` | ListByUser, userID |
| `internal/store/migration_kr_health_test.go` | db, err, err, err, err, ... |
| `internal/store/progresssnap/progresssnap.go` | ctx, nilIfEmpty, TeamID, rows, out, ... |
| `internal/store/shares/shares.go` | GetGoalShare, ctx, err, ListGoalSharesByGoalIDs, rows, ... |
| `internal/store/store.go` | key, scope, ctx, SetSystemAdmin, AnySystemAdmin, ... |
| `internal/store/teams/teams.go` | ctx, rows, periodID, err, ctx, ... |
| `internal/store/tenants/cache_test.go` | GetByID, id |
| `internal/store/users/users.go` | err, ctx, AnySystemAdmin |
| `internal/store/usersettings/usersettings.go` | ctx, GetAll, out, err, userID, ... |
| `internal/usecase/goal/comments.go` | periodID, ctx, userID, author, requestingUserID, ... |
| `internal/usecase/notification/notification_test.go` | v, teamPtr |
| `internal/usecase/user/search_test.go` | userIDs, searchCapturingStore, lastUserIDs, leadNames, returnUsers, ... |

## Connected Communities

- **service/servicetest +33 dirs** (32 cross-edges)
- **usecase/goal +36 dirs** (24 cross-edges)
- **service/healthcheckin +6 dirs** (2 cross-edges)
- **store/krs · TestKRsScopedByTenant** (2 cross-edges)
- **store +14 dirs** (2 cross-edges)
- **store/krs · ApplyCheckIn** (1 cross-edges)
- **store/progresssnap +3 dirs** (1 cross-edges)
- **store/periods +8 dirs** (1 cross-edges)
- **core/domain · TestIsValidKRHealthStatus** (1 cross-edges)
- **web/invite +1 dirs** (1 cross-edges)
- **v1/goals +9 dirs** (1 cross-edges)
- **. +1 dirs · migrateDBTo** (1 cross-edges)
- **core/progress +22 dirs** (1 cross-edges)
- **service/notificationchannel +10 dirs** (1 cross-edges)
- **auth +32 dirs** (1 cross-edges)
- **. +4 dirs · ListByUser** (1 cross-edges)
- **http/dto +36 dirs** (1 cross-edges)

## How to Explore

```
analyze(operation:"communities", id:"community-136")
explore(operation:"context", task:"understand service/activity +61 dirs", format:"gcx")
```

_`format: "gcx"` returns the [GCX1 compact wire format](../../docs/wire-format.md) — round-trippable, ~27% fewer tokens than JSON. Drop it for JSON output; agents using `@gortex/wire` or the Go `github.com/gortexhq/gcx-go` package decode either._
