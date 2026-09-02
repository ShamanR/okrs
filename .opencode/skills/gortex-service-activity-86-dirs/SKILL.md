---
name: gortex-service-activity-86-dirs
description: "Work in the service/activity +86 dirs area — 1014 symbols across 112 files (74% cohesion)"
---

# service/activity +86 dirs

1014 symbols | 112 files | 74% cohesion

## When to Use

Use this skill when working on files in:
- `internal/core/domain/models.go`
- `internal/http/handlers/api/v1/activity/categorycounts/handler.go`
- `internal/http/handlers/api/v1/activity/categorycounts/handler_test.go`
- `internal/http/handlers/api/v1/activity/handler.go`
- `internal/http/handlers/api/v1/activity/treecounts/handler.go`
- `internal/http/handlers/api/v1/activity/treecounts/handler_test.go`
- `internal/http/handlers/api/v1/admin/accessrequests/approve/handler_test.go`
- `internal/http/handlers/api/v1/admin/accessrequests/deny/handler_test.go`
- `internal/http/handlers/api/v1/admin/accessrequests/handler.go`
- `internal/http/handlers/api/v1/admin/accessrequests/handler_test.go`
- `internal/http/handlers/api/v1/admin/activity/purge/handler_test.go`
- `internal/http/handlers/api/v1/admin/admincommon/admincommon.go`
- `internal/http/handlers/api/v1/admin/invitations/handler.go`
- `internal/http/handlers/api/v1/admin/members/handler_test.go`
- `internal/http/handlers/api/v1/admin/periods/handler.go`
- `internal/http/handlers/api/v1/admin/periods/overview/handler.go`
- `internal/http/handlers/api/v1/admin/periods/stats/handler.go`
- `internal/http/handlers/api/v1/admin/settings/access/handler.go`
- `internal/http/handlers/api/v1/admin/settings/feedback/handler.go`
- `internal/http/handlers/api/v1/admin/settings/general/handler.go`
- `internal/http/handlers/api/v1/admin/settings/healthcheckin/handler.go`
- `internal/http/handlers/api/v1/admin/settings/healthcheckin/handler_test.go`
- `internal/http/handlers/api/v1/admin/settings/notifications/handler.go`
- `internal/http/handlers/api/v1/admin/settings/notifications/handler_test.go`
- `internal/http/handlers/api/v1/admin/settings/notifications/test/handler.go`
- `internal/http/handlers/api/v1/admin/settings/notifications/test/handler_test.go`
- `internal/http/handlers/api/v1/admin/teams/handler.go`
- `internal/http/handlers/api/v1/admin/users/admin/handler.go`
- `internal/http/handlers/api/v1/admin/users/admin/handler_test.go`
- `internal/http/handlers/api/v1/admin/users/grants/handler.go`
- `internal/http/handlers/api/v1/admin/users/handler.go`
- `internal/http/handlers/api/v1/goals/comments/resolve/handler_test.go`
- `internal/http/handlers/api/v1/goals/comments/unresolve/handler_test.go`
- `internal/http/handlers/api/v1/goals/goalcommon/goalcommon.go`
- `internal/http/handlers/api/v1/goals/handler.go`
- `internal/http/handlers/api/v1/goals/linkable/handler.go`
- `internal/http/handlers/api/v1/goals/movedown/handler_test.go`
- `internal/http/handlers/api/v1/goals/moveup/handler_test.go`
- `internal/http/handlers/api/v1/healthcheckin/handler.go`
- `internal/http/handlers/api/v1/healthcheckin/handler_test.go`
- `internal/http/handlers/api/v1/hierarchy/handler.go`
- `internal/http/handlers/api/v1/krs/krscommon/krscommon.go`
- `internal/http/handlers/api/v1/krs/movedown/handler_test.go`
- `internal/http/handlers/api/v1/krs/moveup/handler_test.go`
- `internal/http/handlers/api/v1/krs/progress/boolean/handler_test.go`
- `internal/http/handlers/api/v1/krs/progress/numerical/handler_test.go`
- `internal/http/handlers/api/v1/krs/progress/project/handler_test.go`
- `internal/http/handlers/api/v1/me/handler.go`
- `internal/http/handlers/api/v1/notifications/handler.go`
- `internal/http/handlers/api/v1/notifications/preferences/handler.go`
- `internal/http/handlers/api/v1/notifications/preferences/handler_test.go`
- `internal/http/handlers/api/v1/notifications/read/handler.go`
- `internal/http/handlers/api/v1/notifications/read/handler_test.go`
- `internal/http/handlers/api/v1/notifications/routes_test.go`
- `internal/http/handlers/api/v1/notifications/unreadcount/handler.go`
- `internal/http/handlers/api/v1/notifications/unreadcount/handler_test.go`
- `internal/http/handlers/api/v1/onboarding/joinrequest/handler.go`
- `internal/http/handlers/api/v1/onboarding/joinrequest/handler_test.go`
- `internal/http/handlers/api/v1/periods/handler.go`
- `internal/http/handlers/api/v1/periods/overview/handler.go`
- `internal/http/handlers/api/v1/session/memberships/handler.go`
- `internal/http/handlers/api/v1/session/tenants/handler.go`
- `internal/http/handlers/api/v1/system/notificationchannels/handler.go`
- `internal/http/handlers/api/v1/system/notificationchannels/handler_test.go`
- `internal/http/handlers/api/v1/system/settings/handler.go`
- `internal/http/handlers/api/v1/system/settings/handler_test.go`
- `internal/http/handlers/api/v1/system/systemcommon/systemcommon.go`
- `internal/http/handlers/api/v1/system/tenants/activity/purge/handler_test.go`
- `internal/http/handlers/api/v1/system/tenants/entitlements/handler.go`
- `internal/http/handlers/api/v1/system/tenants/handler.go`
- `internal/http/handlers/api/v1/system/tenants/handler_test.go`
- `internal/http/handlers/api/v1/system/tenants/members/handler.go`
- `internal/http/handlers/api/v1/system/tenants/members/handler_test.go`
- `internal/http/handlers/api/v1/system/users/handler.go`
- `internal/http/handlers/api/v1/system/users/handler_test.go`
- `internal/http/handlers/api/v1/teams/export/handler.go`
- `internal/http/handlers/api/v1/teams/handler.go`
- `internal/http/handlers/api/v1/teams/okrs/handler.go`
- `internal/http/handlers/api/v1/teams/overview/handler.go`
- `internal/http/handlers/api/v1/users/handler.go`
- `internal/http/handlers/handlertest/handlertest.go`
- `internal/http/handlers/web/auth/callback/handler.go`
- `internal/http/handlers/web/auth/start/handler.go`
- `internal/http/handlers/web/auth/start/handler_test.go`
- `internal/http/handlers/web/goals/delete/handler_test.go`
- `internal/http/handlers/web/invite/handler.go`
- `internal/http/handlers/web/login/handler.go`
- `internal/http/handlers/web/noaccess/handler.go`
- `internal/scheduler/scheduler.go`
- `internal/service/activity/activity.go`
- `internal/service/activity/activity_test.go`
- `internal/service/activity/feed_test.go`
- `internal/service/goalshare/goalshare.go`
- `internal/service/healthcheckin/cache.go`
- `internal/service/healthcheckin/healthcheckin.go`
- `internal/service/notification/notification.go`
- `internal/service/notification/notification_test.go`
- `internal/service/notificationchannel/notificationchannel.go`
- `internal/service/notificationchannel/notificationchannel_test.go`
- `internal/service/period/period.go`
- `internal/service/progresssnap/progresssnap.go`
- `internal/service/servicetest/activity.go`
- `internal/service/servicetest/eventbus.go`
- `internal/service/team/team.go`
- `internal/service/teamstatus/teamstatus.go`
- `internal/store/activity/activity.go`
- `internal/store/memberships/memberships.go`
- `internal/store/notificationchannels/notificationchannels.go`
- `internal/store/notifications/notifications.go`
- `internal/store/settings/cache.go`
- `internal/store/tenantsettings/tenantsettings.go`
- `internal/store/usersettings/usersettings.go`

## Key Files

| File | Symbols |
|------|---------|
| `internal/core/domain/models.go` | ZeroingCriteria, Project, Kind, SortOrder, Description, ... |
| `internal/http/handlers/api/v1/activity/categorycounts/handler.go` | Handler, now, activity, CategoryCounter, activity, ... |
| `internal/http/handlers/api/v1/activity/categorycounts/handler_test.go` | TestSumsTotal, counts, t, h, w, ... |
| `internal/http/handlers/api/v1/activity/handler.go` | Handler, now, activity |
| `internal/http/handlers/api/v1/activity/treecounts/handler.go` | activity, now, activity, TreeCounter, New, ... |
| `internal/http/handlers/api/v1/activity/treecounts/handler_test.go` | closure@49, TestPassesPeriodRangeAndScope, t, w, fakeTree, ... |
| `internal/http/handlers/api/v1/admin/accessrequests/approve/handler_test.go` | w, f, TestAppliesPackageOperation, t |
| `internal/http/handlers/api/v1/admin/accessrequests/deny/handler_test.go` | t, TestAppliesPackageOperation, w, f |
| `internal/http/handlers/api/v1/admin/accessrequests/handler.go` | Handler, onboard, onboard, New |
| `internal/http/handlers/api/v1/admin/accessrequests/handler_test.go` | w, TestGateGetRequiresTenant, t |
| `internal/http/handlers/api/v1/admin/activity/purge/handler_test.go` | lastCutoff, called, deleted, fakePurger |
| `internal/http/handlers/api/v1/admin/admincommon/admincommon.go` | ActivityPurger |
| `internal/http/handlers/api/v1/admin/invitations/handler.go` | invites, Handler, baseURL |
| `internal/http/handlers/api/v1/admin/members/handler_test.go` | f, TestAppliesPackageOperation, t, w |
| `internal/http/handlers/api/v1/admin/periods/handler.go` | Handler, periods |
| `internal/http/handlers/api/v1/admin/periods/overview/handler.go` | periodUC, settings, Handler |
| `internal/http/handlers/api/v1/admin/periods/stats/handler.go` | periodUC, Handler, settings |
| `internal/http/handlers/api/v1/admin/settings/access/handler.go` | settings, Handler |
| `internal/http/handlers/api/v1/admin/settings/feedback/handler.go` | settings, Handler |
| `internal/http/handlers/api/v1/admin/settings/general/handler.go` | Handler, renamer, settings |
| `internal/http/handlers/api/v1/admin/settings/healthcheckin/handler.go` | New, settings, cache, CacheInvalidator, SettingsProvider, ... |
| `internal/http/handlers/api/v1/admin/settings/healthcheckin/handler_test.go` | t, w, cases, TestPostRequiresTenant, TestPostRejectsMalformedBody, ... |
| `internal/http/handlers/api/v1/admin/settings/notifications/handler.go` | New, svc, Handler, Channels, svc |
| `internal/http/handlers/api/v1/admin/settings/notifications/handler_test.go` | unknown, unavailable, t, by, body, ... |
| `internal/http/handlers/api/v1/admin/settings/notifications/test/handler.go` | svc, Channels, New |
| `internal/http/handlers/api/v1/admin/settings/notifications/test/handler_test.go` | rec, h, unavailable, h, leak, ... |
| `internal/http/handlers/api/v1/admin/teams/handler.go` | teams, users, Handler |
| `internal/http/handlers/api/v1/admin/users/admin/handler.go` | New |
| `internal/http/handlers/api/v1/admin/users/admin/handler_test.go` | c, w, closure@54, TestBadUserIDIs400, TestPostGrantsAdminAndDeleteRevokes, ... |
| `internal/http/handlers/api/v1/admin/users/grants/handler.go` | grants, Handler |
| `internal/http/handlers/api/v1/admin/users/handler.go` | users, grants, Handler |
| `internal/http/handlers/api/v1/goals/comments/resolve/handler_test.go` | Get, goal, List, t, t, ... |
| `internal/http/handlers/api/v1/goals/comments/unresolve/handler_test.go` | fakeShares, goal, t, Get, List, ... |
| `internal/http/handlers/api/v1/goals/goalcommon/goalcommon.go` | ShareGetter, GoalGetter, ShareLister |
| `internal/http/handlers/api/v1/goals/handler.go` | uc, shares, Handler, users, links, ... |
| `internal/http/handlers/api/v1/goals/linkable/handler.go` | links, Handler |
| `internal/http/handlers/api/v1/goals/movedown/handler_test.go` | TestInaccessibleTeamIs403, List, w, goal, t, ... |
| `internal/http/handlers/api/v1/goals/moveup/handler_test.go` | TestInaccessibleTeamIs403, fakeGoals, h, Get, fakeShares, ... |
| `internal/http/handlers/api/v1/healthcheckin/handler.go` | svc, New, settings, Handler, settings, ... |
| `internal/http/handlers/api/v1/healthcheckin/handler_test.go` | TestRequiresTenant, t, w, w, t, ... |
| `internal/http/handlers/api/v1/hierarchy/handler.go` | PeriodReader |
| `internal/http/handlers/api/v1/krs/krscommon/krscommon.go` | KRGetter, GoalGetter |
| `internal/http/handlers/api/v1/krs/movedown/handler_test.go` | goal, t, fakeGoals, kr, calls, ... |
| `internal/http/handlers/api/v1/krs/moveup/handler_test.go` | kr, Get, w, TestInaccessibleGoalIs404, t, ... |
| `internal/http/handlers/api/v1/krs/progress/boolean/handler_test.go` | h, body, h, h, t, ... |
| `internal/http/handlers/api/v1/krs/progress/numerical/handler_test.go` | bus, store, store, t, body, ... |
| `internal/http/handlers/api/v1/krs/progress/project/handler_test.go` | body, h, h, body, body, ... |
| `internal/http/handlers/api/v1/me/handler.go` | Handler |
| `internal/http/handlers/api/v1/notifications/handler.go` | Handler, svc, svc, NotificationReader, New |
| `internal/http/handlers/api/v1/notifications/preferences/handler.go` | PrefService, Handler, svc, svc, New |
| `internal/http/handlers/api/v1/notifications/preferences/handler_test.go` | fakeSvc, got, body, t, body, ... |
| `internal/http/handlers/api/v1/notifications/read/handler.go` | svc, New, ReadMarker |
| `internal/http/handlers/api/v1/notifications/read/handler_test.go` | fakeMarker, TestAllTrueSkipsIDsRequirement, t, w, w, ... |
| `internal/http/handlers/api/v1/notifications/routes_test.go` | svc, TestDeleteRequiresTenantScope, w2, t, foreign, ... |
| `internal/http/handlers/api/v1/notifications/unreadcount/handler.go` | New, Handler, svc, UnreadCounter, svc |
| `internal/http/handlers/api/v1/notifications/unreadcount/handler_test.go` | t, TestForbiddenWithoutTenant, h, gotUserID, fakeCounter, ... |
| `internal/http/handlers/api/v1/onboarding/joinrequest/handler.go` | New, onboard |
| `internal/http/handlers/api/v1/onboarding/joinrequest/handler_test.go` | TestEmptySlugIs400, f, w, t, w, ... |
| `internal/http/handlers/api/v1/periods/handler.go` | Handler, New, periods, periods |
| `internal/http/handlers/api/v1/periods/overview/handler.go` | periodUC, teams, settings, leads, Handler |
| `internal/http/handlers/api/v1/session/memberships/handler.go` | members, Handler, leaver |
| `internal/http/handlers/api/v1/session/tenants/handler.go` | tenants, members, Handler |
| `internal/http/handlers/api/v1/system/notificationchannels/handler.go` | New, svc, Handler, svc, Channels |
| `internal/http/handlers/api/v1/system/notificationchannels/handler_test.go` | TestListReturnsBuildChannelsWithEntitlementKeys, t, rec, h, rec, ... |
| `internal/http/handlers/api/v1/system/settings/handler.go` | settings, Handler |
| `internal/http/handlers/api/v1/system/settings/handler_test.go` | w, f, t, w, t, ... |
| `internal/http/handlers/api/v1/system/systemcommon/systemcommon.go` | ActivityPurger, UserLister, TenantLister |
| `internal/http/handlers/api/v1/system/tenants/activity/purge/handler_test.go` | lastScope, deleted, lastCutoff, calledCount, fakeSysPurger |
| `internal/http/handlers/api/v1/system/tenants/entitlements/handler.go` | prov, settings, Handler |
| `internal/http/handlers/api/v1/system/tenants/handler.go` | Handler, prov, tenants |
| `internal/http/handlers/api/v1/system/tenants/handler_test.go` | t, w, TestGetEmptyIsArray |
| `internal/http/handlers/api/v1/system/tenants/members/handler.go` | prov, members, members, Handler, prov, ... |
| `internal/http/handlers/api/v1/system/tenants/members/handler_test.go` | t, w, TestGateDeleteBadUserID |
| `internal/http/handlers/api/v1/system/users/handler.go` | users, New, users, Handler |
| `internal/http/handlers/api/v1/system/users/handler_test.go` | fakeUsers, w, TestReturnsUsers, TestStoreErrorIs500, w, ... |
| `internal/http/handlers/api/v1/teams/export/handler.go` | Handler, exportUC |
| `internal/http/handlers/api/v1/teams/handler.go` | teams, teams, New, Handler |
| `internal/http/handlers/api/v1/teams/okrs/handler.go` | board, Handler, periods, users |
| `internal/http/handlers/api/v1/teams/overview/handler.go` | board, Handler, periods, users |
| `internal/http/handlers/api/v1/users/handler.go` | Handler, search, dir |
| `internal/http/handlers/handlertest/handlertest.go` | w, UserEmail, Tenant, t, r, ... |
| `internal/http/handlers/web/auth/callback/handler.go` | mgr, onboard, Handler, logger, sessions |
| `internal/http/handlers/web/auth/start/handler.go` | Handler, mgr, New, mgr |
| `internal/http/handlers/web/auth/start/handler_test.go` | w, TestEmptyProviderIs400, TestUnknownProviderIs400, err, t, ... |
| `internal/http/handlers/web/goals/delete/handler_test.go` | w, TestBadGoalIDRendersError, t |
| `internal/http/handlers/web/invite/handler.go` | onboard, Handler, sessions |
| `internal/http/handlers/web/login/handler.go` | mgr, logger, tmpl, Handler |
| `internal/http/handlers/web/noaccess/handler.go` | Handler, name |
| `internal/scheduler/scheduler.go` | TenantLister, PeriodFinder, NotificationPurger |
| `internal/service/activity/activity.go` | Repo, Service, repo, logger |
| `internal/service/activity/activity_test.go` | List, TreeCounts, failNext, recordingRepo, recorded, ... |
| `internal/service/activity/feed_test.go` | got, CategoryCounts, f, TreeCounts, capturingRepo, ... |
| `internal/service/goalshare/goalshare.go` | repo, Service |
| `internal/service/healthcheckin/cache.go` | Cache, ttl, periods, logger, mu, ... |
| `internal/service/healthcheckin/healthcheckin.go` | cache, Service, cache, New |
| `internal/service/notification/notification.go` | ins, scope, CreateBatch, repo, Service, ... |
| `internal/service/notification/notification_test.go` | err, got, next, MarkRead, InsertBatch, ... |
| `internal/service/notificationchannel/notificationchannel.go` | Values, Descriptor, Enabled, ChannelState, Configured, ... |
| `internal/service/notificationchannel/notificationchannel_test.go` | fakeRepo, rows |
| `internal/service/period/period.go` | Service, repo |
| `internal/service/progresssnap/progresssnap.go` | repo, Service |
| `internal/service/servicetest/activity.go` | List, CategoryCounts, Events, ActivityRepo, TreeCounts, ... |
| `internal/service/servicetest/eventbus.go` | ev, out, KindsPublished, out |
| `internal/service/team/team.go` | repo, Service |
| `internal/service/teamstatus/teamstatus.go` | Service, repo |
| `internal/store/activity/activity.go` | ActivityRepository, db |
| `internal/store/memberships/memberships.go` | db, MembershipRepository |
| `internal/store/notificationchannels/notificationchannels.go` | db, Repository |
| `internal/store/notifications/notifications.go` | Repository, db |
| `internal/store/settings/cache.go` | loaded, SystemSettingsCache, mu, backend, ttl, ... |
| `internal/store/tenantsettings/tenantsettings.go` | db, TenantSettingsRepository |
| `internal/store/usersettings/usersettings.go` | db, UserSettingsRepository |

## Connected Communities

- **http/handlers · URLParam** (88 cross-edges)
- **usecase/goal +36 dirs** (24 cross-edges)
- **service/servicetest +33 dirs** (20 cross-edges)
- **auth +32 dirs** (12 cross-edges)
- **service/activity +61 dirs** (10 cross-edges)
- **render/notify +5 dirs** (8 cross-edges)
- **service/goal +8 dirs** (6 cross-edges)
- **krs/movedown · TestBadKRIDIs400** (4 cross-edges)
- **comments/resolve · New** (4 cross-edges)
- **comments/unresolve +1 dirs** (4 cross-edges)
- **krs/moveup · TestBadKRIDIs400** (4 cross-edges)
- **service/notificationchannel +10 dirs** (3 cross-edges)
- **service/healthcheckin +6 dirs** (3 cross-edges)
- **goals/moveup** (2 cross-edges)
- **system/tenants** (2 cross-edges)
- **store/memberships +14 dirs** (2 cross-edges)
- **auth +67 dirs** (2 cross-edges)
- **goals/movedown** (2 cross-edges)
- **mattermost +1 dirs · TestBotIDAllWaitOnSingleFailure** (1 cross-edges)
- **auth +4 dirs · TestUsersEndpoint_ScopedSearch_…** (1 cross-edges)
- **auth +1 dirs** (1 cross-edges)
- **http/dto +36 dirs** (1 cross-edges)
- **auth +6 dirs** (1 cross-edges)
- **store/periods +8 dirs** (1 cross-edges)
- **mattermost +1 dirs · call** (1 cross-edges)

## How to Explore

```
analyze(operation:"communities", id:"community-93")
explore(operation:"context", task:"understand service/activity +86 dirs", format:"gcx")
```

_`format: "gcx"` returns the [GCX1 compact wire format](../../docs/wire-format.md) — round-trippable, ~27% fewer tokens than JSON. Drop it for JSON output; agents using `@gortex/wire` or the Go `github.com/gortexhq/gcx-go` package decode either._
