---
name: gortex-auth-67-dirs
description: "Work in the auth +67 dirs area — 1295 symbols across 127 files (78% cohesion)"
---

# auth +67 dirs

1295 symbols | 127 files | 78% cohesion

## When to Use

Use this skill when working on files in:
- ``
- `internal/auth/context.go`
- `internal/auth/context_test.go`
- `internal/auth/policy.go`
- `internal/auth/policy_test.go`
- `internal/core/domain/models.go`
- `internal/http/dto/notification.go`
- `internal/http/handlers/api/v1/activity/activitycommon/activitycommon.go`
- `internal/http/handlers/api/v1/activity/categorycounts/handler.go`
- `internal/http/handlers/api/v1/activity/categorycounts/routes.go`
- `internal/http/handlers/api/v1/activity/handler.go`
- `internal/http/handlers/api/v1/activity/routes.go`
- `internal/http/handlers/api/v1/activity/routes_test.go`
- `internal/http/handlers/api/v1/activity/treecounts/handler.go`
- `internal/http/handlers/api/v1/activity/treecounts/handler_test.go`
- `internal/http/handlers/api/v1/activity/treecounts/routes.go`
- `internal/http/handlers/api/v1/admin/admincommon/admincommon.go`
- `internal/http/handlers/api/v1/admin/periods/archive/handler.go`
- `internal/http/handlers/api/v1/admin/periods/archive/routes.go`
- `internal/http/handlers/api/v1/admin/periods/handler.go`
- `internal/http/handlers/api/v1/admin/periods/handler_test.go`
- `internal/http/handlers/api/v1/admin/periods/overview/handler.go`
- `internal/http/handlers/api/v1/admin/periods/overview/routes.go`
- `internal/http/handlers/api/v1/admin/periods/routes.go`
- `internal/http/handlers/api/v1/admin/periods/stats/handler.go`
- `internal/http/handlers/api/v1/admin/periods/stats/routes.go`
- `internal/http/handlers/api/v1/admin/periods/teams/activate/handler.go`
- `internal/http/handlers/api/v1/admin/periods/teams/bulkstatus/bulkstatus.go`
- `internal/http/handlers/api/v1/admin/periods/teams/close/handler.go`
- `internal/http/handlers/api/v1/admin/periods/unarchive/handler.go`
- `internal/http/handlers/api/v1/admin/periods/unarchive/routes.go`
- `internal/http/handlers/api/v1/admin/settings/notifications/routes.go`
- `internal/http/handlers/api/v1/admin/settings/notifications/test/routes.go`
- `internal/http/handlers/api/v1/admin/teams/handler.go`
- `internal/http/handlers/api/v1/admin/teams/handler_test.go`
- `internal/http/handlers/api/v1/admin/teams/hard/handler.go`
- `internal/http/handlers/api/v1/admin/teams/hard/routes.go`
- `internal/http/handlers/api/v1/admin/teams/restore/handler.go`
- `internal/http/handlers/api/v1/admin/teams/restore/routes.go`
- `internal/http/handlers/api/v1/admin/teams/routes.go`
- `internal/http/handlers/api/v1/cache.go`
- `internal/http/handlers/api/v1/errors.go`
- `internal/http/handlers/api/v1/goals/comments/handler.go`
- `internal/http/handlers/api/v1/goals/comments/replies/handler.go`
- `internal/http/handlers/api/v1/goals/comments/replies/routes.go`
- `internal/http/handlers/api/v1/goals/comments/routes.go`
- `internal/http/handlers/api/v1/goals/goalcommon/goalcommon.go`
- `internal/http/handlers/api/v1/goals/handler.go`
- `internal/http/handlers/api/v1/goals/keyresults/handler.go`
- `internal/http/handlers/api/v1/goals/keyresults/routes.go`
- `internal/http/handlers/api/v1/goals/linkable/handler.go`
- `internal/http/handlers/api/v1/goals/linkable/handler_test.go`
- `internal/http/handlers/api/v1/goals/linkable/routes.go`
- `internal/http/handlers/api/v1/goals/links/handler.go`
- `internal/http/handlers/api/v1/goals/links/routes.go`
- `internal/http/handlers/api/v1/goals/movedown/handler.go`
- `internal/http/handlers/api/v1/goals/moveup/handler.go`
- `internal/http/handlers/api/v1/goals/routes.go`
- `internal/http/handlers/api/v1/goals/routes_test.go`
- `internal/http/handlers/api/v1/goals/share/handler.go`
- `internal/http/handlers/api/v1/goals/share/routes.go`
- `internal/http/handlers/api/v1/goals/transfer/handler.go`
- `internal/http/handlers/api/v1/goals/transfer/routes.go`
- `internal/http/handlers/api/v1/goals/weight/handler.go`
- `internal/http/handlers/api/v1/goals/weight/routes.go`
- `internal/http/handlers/api/v1/goaltree/handler.go`
- `internal/http/handlers/api/v1/goaltree/routes.go`
- `internal/http/handlers/api/v1/helpers_response.go`
- `internal/http/handlers/api/v1/hierarchy/handler.go`
- `internal/http/handlers/api/v1/hierarchy/routes.go`
- `internal/http/handlers/api/v1/krs/description/handler.go`
- `internal/http/handlers/api/v1/krs/description/routes.go`
- `internal/http/handlers/api/v1/krs/handler.go`
- `internal/http/handlers/api/v1/krs/krscommon/krscommon.go`
- `internal/http/handlers/api/v1/krs/krscommon/krscommon_test.go`
- `internal/http/handlers/api/v1/krs/note/handler.go`
- `internal/http/handlers/api/v1/krs/note/routes.go`
- `internal/http/handlers/api/v1/krs/progress/boolean/handler.go`
- `internal/http/handlers/api/v1/krs/progress/boolean/routes.go`
- `internal/http/handlers/api/v1/krs/progress/numerical/handler.go`
- `internal/http/handlers/api/v1/krs/progress/numerical/routes.go`
- `internal/http/handlers/api/v1/krs/progress/project/handler.go`
- `internal/http/handlers/api/v1/krs/progress/project/routes.go`
- `internal/http/handlers/api/v1/krs/routes.go`
- `internal/http/handlers/api/v1/krs/routes_test.go`
- `internal/http/handlers/api/v1/method_not_allowed.go`
- `internal/http/handlers/api/v1/notifications/handler.go`
- `internal/http/handlers/api/v1/notifications/preferences/handler.go`
- `internal/http/handlers/api/v1/notifications/preferences/handler_test.go`
- `internal/http/handlers/api/v1/notifications/preferences/routes.go`
- `internal/http/handlers/api/v1/notifications/read/handler.go`
- `internal/http/handlers/api/v1/notifications/read/handler_test.go`
- `internal/http/handlers/api/v1/notifications/read/routes.go`
- `internal/http/handlers/api/v1/notifications/routes.go`
- `internal/http/handlers/api/v1/notifications/unreadcount/handler.go`
- `internal/http/handlers/api/v1/notifications/unreadcount/routes.go`
- `internal/http/handlers/api/v1/periods/handler.go`
- `internal/http/handlers/api/v1/periods/overview/handler.go`
- `internal/http/handlers/api/v1/periods/overview/routes.go`
- `internal/http/handlers/api/v1/periods/routes.go`
- `internal/http/handlers/api/v1/periods/teams/activate/handler.go`
- `internal/http/handlers/api/v1/teams/export/handler.go`
- `internal/http/handlers/api/v1/teams/export/routes.go`
- `internal/http/handlers/api/v1/teams/goals/handler.go`
- `internal/http/handlers/api/v1/teams/goals/routes.go`
- `internal/http/handlers/api/v1/teams/handler.go`
- `internal/http/handlers/api/v1/teams/okrs/handler.go`
- `internal/http/handlers/api/v1/teams/okrs/routes.go`
- `internal/http/handlers/api/v1/teams/overview/handler.go`
- `internal/http/handlers/api/v1/teams/overview/routes.go`
- `internal/http/handlers/api/v1/teams/routes.go`
- `internal/http/handlers/api/v1/teams/routes_test.go`
- `internal/http/handlers/api/v1/teams/status/handler.go`
- `internal/http/handlers/api/v1/teams/status/routes.go`
- `internal/http/handlers/api/v1/testutil/integration.go`
- `internal/http/handlers/api/v1/users/handler.go`
- `internal/http/handlers/api/v1/users/routes.go`
- `internal/http/handlers/web/common/common.go`
- `internal/http/handlers/web/common/team_type_test.go`
- `internal/http/server.go`
- `internal/render/export/export.go`
- `internal/service/goalshare/goalshare.go`
- `internal/service/keyresult/keyresult.go`
- `internal/service/team/team.go`
- `internal/service/user/user.go`
- `internal/usecase/export/export.go`
- `internal/usecase/keyresult/keyresult.go`

## Key Files

| File | Symbols |
|------|---------|
| `` | NewDecoder, shares, Atoi, krs, List, ... |
| `internal/auth/context.go` | TenantScopeFromContext, ctx, UserIDFromContext, ctx, u, ... |
| `internal/auth/context_test.go` | t, got, TestUserIDFromContextWithoutUserReturnsAnonymous |
| `internal/auth/policy.go` | AllowedTeamIDsFromCtx, ids, ids, ctx, CanAccessTeamFromCtx, ... |
| `internal/auth/policy_test.go` | t, TestAllowedTeamIDsFromCtxNotSet, TestCanAccessTeamFromCtxAdminNilSliceAllowsAll, ctx, TestCanAccessTeamFromCtxNoScopeAllows, ... |
| `internal/core/domain/models.go` | TeamType, KRKind, FocusType |
| `internal/http/dto/notification.go` | Channels, UnreadCount, NextCursor, Count, NotificationPreferences, ... |
| `internal/http/handlers/api/v1/activity/activitycommon/activitycommon.go` | ScopeTeams, allowed, r, ok |
| `internal/http/handlers/api/v1/activity/categorycounts/handler.go` | counts, scope, n, total, w, ... |
| `internal/http/handlers/api/v1/activity/categorycounts/routes.go` | r, RegisterRoutes, h |
| `internal/http/handlers/api/v1/activity/handler.go` | page, activity, New, ok, err, ... |
| `internal/http/handlers/api/v1/activity/routes.go` | h, RegisterRoutes, r |
| `internal/http/handlers/api/v1/activity/routes_test.go` | TestMethodNotAllowed, w, t, uri, r |
| `internal/http/handlers/api/v1/activity/treecounts/handler.go` | teamID, Get, q, n, err, ... |
| `internal/http/handlers/api/v1/activity/treecounts/handler_test.go` | since, periodID, TreeCounts, ids |
| `internal/http/handlers/api/v1/activity/treecounts/routes.go` | h, r, RegisterRoutes |
| `internal/http/handlers/api/v1/admin/admincommon/admincommon.go` | err, cfg, settings, r, scope, ... |
| `internal/http/handlers/api/v1/admin/periods/archive/handler.go` | Post, w, scope, err, ok, ... |
| `internal/http/handlers/api/v1/admin/periods/archive/routes.go` | h, RegisterRoutes, r |
| `internal/http/handlers/api/v1/admin/periods/handler.go` | scope, periods, Patch, end, ok, ... |
| `internal/http/handlers/api/v1/admin/periods/handler_test.go` | w, t, TestGateGetRequiresTenant, w, t, ... |
| `internal/http/handlers/api/v1/admin/periods/overview/handler.go` | periodUC, err, periodID, settings, w, ... |
| `internal/http/handlers/api/v1/admin/periods/overview/routes.go` | r, h, RegisterRoutes |
| `internal/http/handlers/api/v1/admin/periods/routes.go` | h, r, RegisterRoutes |
| `internal/http/handlers/api/v1/admin/periods/stats/handler.go` | ok, scope, Get, settings, err, ... |
| `internal/http/handlers/api/v1/admin/periods/stats/routes.go` | h, RegisterRoutes, r |
| `internal/http/handlers/api/v1/admin/periods/teams/activate/handler.go` | periodUC, leads, New, teams |
| `internal/http/handlers/api/v1/admin/periods/teams/bulkstatus/bulkstatus.go` | periodID, w, teams, leads, leads, ... |
| `internal/http/handlers/api/v1/admin/periods/teams/close/handler.go` | periodUC, teams, New, leads |
| `internal/http/handlers/api/v1/admin/periods/unarchive/handler.go` | Handler, periods, periods, scope, periodID, ... |
| `internal/http/handlers/api/v1/admin/periods/unarchive/routes.go` | RegisterRoutes, r, h |
| `internal/http/handlers/api/v1/admin/settings/notifications/routes.go` | h, RegisterRoutes, r |
| `internal/http/handlers/api/v1/admin/settings/notifications/test/routes.go` | h, RegisterRoutes, r |
| `internal/http/handlers/api/v1/admin/teams/handler.go` | scope, Delete, Patch, teamType, w, ... |
| `internal/http/handlers/api/v1/admin/teams/handler_test.go` | t, TestGatePostRequiresTenant, w |
| `internal/http/handlers/api/v1/admin/teams/hard/handler.go` | teamID, teams, Handler, teams, New, ... |
| `internal/http/handlers/api/v1/admin/teams/hard/routes.go` | h, RegisterRoutes, r |
| `internal/http/handlers/api/v1/admin/teams/restore/handler.go` | err, scope, teams, teamID, Handler, ... |
| `internal/http/handlers/api/v1/admin/teams/restore/routes.go` | RegisterRoutes, h, r |
| `internal/http/handlers/api/v1/admin/teams/routes.go` | h, r, RegisterRoutes |
| `internal/http/handlers/api/v1/cache.go` | SetAPICacheControl, w, w, setAPICacheControl |
| `internal/http/handlers/api/v1/errors.go` | WriteError, WriteJSON, message, code, w, ... |
| `internal/http/handlers/api/v1/goals/comments/handler.go` | uc, shares, r, err, isAdmin, ... |
| `internal/http/handlers/api/v1/goals/comments/replies/handler.go` | err, scope, r, goal, w, ... |
| `internal/http/handlers/api/v1/goals/comments/replies/routes.go` | r, h, RegisterRoutes |
| `internal/http/handlers/api/v1/goals/comments/routes.go` | RegisterRoutes, h, r |
| `internal/http/handlers/api/v1/goals/goalcommon/goalcommon.go` | d, ResolveDeps, Mover, w, goal, ... |
| `internal/http/handlers/api/v1/goals/handler.go` | err, users, children, parents, err, ... |
| `internal/http/handlers/api/v1/goals/keyresults/handler.go` | err, krID, goals, err, goal, ... |
| `internal/http/handlers/api/v1/goals/keyresults/routes.go` | r, RegisterRoutes, h |
| `internal/http/handlers/api/v1/goals/linkable/handler.go` | items, periodID, Get, ok, allowed, ... |
| `internal/http/handlers/api/v1/goals/linkable/handler_test.go` | w, t, TestGateGetRequiresTenant |
| `internal/http/handlers/api/v1/goals/linkable/routes.go` | h, r, RegisterRoutes |
| `internal/http/handlers/api/v1/goals/links/handler.go` | Handler, r, Post, err, goalID, ... |
| `internal/http/handlers/api/v1/goals/links/routes.go` | RegisterRoutes, h, r |
| `internal/http/handlers/api/v1/goals/movedown/handler.go` | RegisterRoutes, h, r |
| `internal/http/handlers/api/v1/goals/moveup/handler.go` | RegisterRoutes, r, h |
| `internal/http/handlers/api/v1/goals/routes.go` | h, r, RegisterRoutes |
| `internal/http/handlers/api/v1/goals/routes_test.go` | req, w, t, TestMethodNotAllowedOnComments, r |
| `internal/http/handlers/api/v1/goals/share/handler.go` | goal, err, Post, ok, goals, ... |
| `internal/http/handlers/api/v1/goals/share/routes.go` | h, r, RegisterRoutes |
| `internal/http/handlers/api/v1/goals/transfer/handler.go` | Handler, r, goal, Post, mode, ... |
| `internal/http/handlers/api/v1/goals/transfer/routes.go` | RegisterRoutes, h, r |
| `internal/http/handlers/api/v1/goals/weight/handler.go` | New, goals, goal, goals, scope, ... |
| `internal/http/handlers/api/v1/goals/weight/routes.go` | RegisterRoutes, r, h |
| `internal/http/handlers/api/v1/goaltree/handler.go` | u, callerUDID, adminAll, callerUDID, periods, ... |
| `internal/http/handlers/api/v1/goaltree/routes.go` | r, RegisterRoutes, h |
| `internal/http/handlers/api/v1/helpers_response.go` | u, BuildUserRefMap, m, users, ref |
| `internal/http/handlers/api/v1/hierarchy/handler.go` | periodID, users, leadUDIDs, err, w, ... |
| `internal/http/handlers/api/v1/hierarchy/routes.go` | h, RegisterRoutes, r |
| `internal/http/handlers/api/v1/krs/description/handler.go` | krs, Handler, r, w, goals, ... |
| `internal/http/handlers/api/v1/krs/description/routes.go` | h, RegisterRoutes, r |
| `internal/http/handlers/api/v1/krs/handler.go` | goals, err, goal, err, krs, ... |
| `internal/http/handlers/api/v1/krs/krscommon/krscommon.go` | krID, TenantScope, d, NormalizeNoteText, direction, ... |
| `internal/http/handlers/api/v1/krs/krscommon/krscommon_test.go` | t, TestNormalizeNoteTextLeavesLFAlone, got, t, TestNormalizeNoteTextConvertsCRLFToLF, ... |
| `internal/http/handlers/api/v1/krs/note/handler.go` | err, ok, New, Post, Handler, ... |
| `internal/http/handlers/api/v1/krs/note/routes.go` | h, RegisterRoutes, r |
| `internal/http/handlers/api/v1/krs/progress/boolean/handler.go` | uc, Handler, Post, err, krID, ... |
| `internal/http/handlers/api/v1/krs/progress/boolean/routes.go` | h, r, RegisterRoutes |
| `internal/http/handlers/api/v1/krs/progress/numerical/handler.go` | uc, w, scope, err, r, ... |
| `internal/http/handlers/api/v1/krs/progress/numerical/routes.go` | RegisterRoutes, h, r |
| `internal/http/handlers/api/v1/krs/progress/project/handler.go` | scope, err, w, r, Handler, ... |
| `internal/http/handlers/api/v1/krs/progress/project/routes.go` | r, RegisterRoutes, h |
| `internal/http/handlers/api/v1/krs/routes.go` | h, r, RegisterRoutes |
| `internal/http/handlers/api/v1/krs/routes_test.go` | req, t, w, TestMethodNotAllowedOnNote, r |
| `internal/http/handlers/api/v1/method_not_allowed.go` | r, closure@10, RegisterMethodNotAllowed |
| `internal/http/handlers/api/v1/notifications/handler.go` | out, Delete, err, w, err, ... |
| `internal/http/handlers/api/v1/notifications/preferences/handler.go` | err, it, w, err, scope, ... |
| `internal/http/handlers/api/v1/notifications/preferences/handler_test.go` | t, fullMatrix, w, TestGetSetsAPICacheControl, h |
| `internal/http/handlers/api/v1/notifications/preferences/routes.go` | h, r, RegisterRoutes |
| `internal/http/handlers/api/v1/notifications/read/handler.go` | err, w, err, scope, r, ... |
| `internal/http/handlers/api/v1/notifications/read/handler_test.go` | MarkRead, userID, ids, all |
| `internal/http/handlers/api/v1/notifications/read/routes.go` | RegisterRoutes, h, r |
| `internal/http/handlers/api/v1/notifications/routes.go` | r, RegisterRoutes, h |
| `internal/http/handlers/api/v1/notifications/unreadcount/handler.go` | err, ok, n, w, r, ... |
| `internal/http/handlers/api/v1/notifications/unreadcount/routes.go` | h, r, RegisterRoutes |
| `internal/http/handlers/api/v1/periods/handler.go` | w, err, ok, r, Get, ... |
| `internal/http/handlers/api/v1/periods/overview/handler.go` | w, ov, teamFilter, err, ok, ... |
| `internal/http/handlers/api/v1/periods/overview/routes.go` | r, RegisterRoutes, h |
| `internal/http/handlers/api/v1/periods/routes.go` | r, h, RegisterRoutes |
| `internal/http/handlers/api/v1/periods/teams/activate/handler.go` | New, leads, periodUC, teams |
| `internal/http/handlers/api/v1/teams/export/handler.go` | q, exportUC, teamID, res, err, ... |
| `internal/http/handlers/api/v1/teams/export/routes.go` | RegisterRoutes, r, h |
| `internal/http/handlers/api/v1/teams/goals/handler.go` | users, goalUC, scope, teamID, err, ... |
| `internal/http/handlers/api/v1/teams/goals/routes.go` | RegisterRoutes, r, h |
| `internal/http/handlers/api/v1/teams/handler.go` | teamID, err, w, r, Get, ... |
| `internal/http/handlers/api/v1/teams/okrs/handler.go` | period, teamID, periodID, err, okr, ... |
| `internal/http/handlers/api/v1/teams/okrs/routes.go` | RegisterRoutes, r, h |
| `internal/http/handlers/api/v1/teams/overview/handler.go` | period, w, Get, users, periodID, ... |
| `internal/http/handlers/api/v1/teams/overview/routes.go` | RegisterRoutes, r, h |
| `internal/http/handlers/api/v1/teams/routes.go` | r, RegisterRoutes, h |
| `internal/http/handlers/api/v1/teams/routes_test.go` | t, w, r, TestMethodNotAllowedOnStatus |
| `internal/http/handlers/api/v1/teams/status/handler.go` | err, scope, New, status, ok, ... |
| `internal/http/handlers/api/v1/teams/status/routes.go` | RegisterRoutes, h, r |
| `internal/http/handlers/api/v1/testutil/integration.go` | closure@87, allowedTeamIDs, t, bus, grantsCache, ... |
| `internal/http/handlers/api/v1/users/handler.go` | teamID, resp, ok, w, team, ... |
| `internal/http/handlers/api/v1/users/routes.go` | h, r, RegisterRoutes |
| `internal/http/handlers/web/common/common.go` | value, ValidTeamType, ValidTeamPeriodStatus, TrimmedFormValue, id, ... |
| `internal/http/handlers/web/common/team_type_test.go` | t, TestValidTeamType, valid, invalid, tt, ... |
| `internal/http/server.go` | closure@629, r, registerAdminRoutes, d, registerApiRoutes, ... |
| `internal/render/export/export.go` | Format |
| `internal/service/goalshare/goalshare.go` | ctx, UpdateWeight, goalID, weight, scope, ... |
| `internal/service/keyresult/keyresult.go` | ctx, id, Get, scope |
| `internal/service/team/team.go` | ctx, teamID, Restore, Get, ctx, ... |
| `internal/service/user/user.go` | ListLeadTeams, udids, GetByUDIDs, ctx, udids, ... |
| `internal/usecase/export/export.go` | UseCase, board, teams, periods, goals, ... |
| `internal/usecase/keyresult/keyresult.go` | ID, ProjectStageUpdate, IsDone |

## Entry Points

- `internal/http/handlers/api/v1/hierarchy/handler.go::Handler.HandleHierarchy`
- `internal/http/handlers/api/v1/goaltree/handler.go::Handler.HandleGoalTree`

## Connected Communities

- **auth +32 dirs** (49 cross-edges)
- **usecase/goal +36 dirs** (37 cross-edges)
- **service/servicetest +33 dirs** (37 cross-edges)
- **service/activity +86 dirs** (24 cross-edges)
- **http/handlers · URLParam** (17 cross-edges)
- **service/activity +61 dirs** (10 cross-edges)
- **system/notificationchannels +16 dirs** (9 cross-edges)
- **http/dto +36 dirs** (7 cross-edges)
- **core/progress +22 dirs** (6 cross-edges)
- **activity/purge +12 dirs** (5 cross-edges)
- **goals/movedown** (5 cross-edges)
- **goals/moveup** (5 cross-edges)
- **http +3 dirs** (4 cross-edges)
- **platform/eventbus +7 dirs** (3 cross-edges)
- **api/v1 · WriteError** (3 cross-edges)
- **auth +4 dirs · TestUsersEndpoint_ScopedSearch_…** (3 cross-edges)
- **activity/activitycommon · ParseFilter** (3 cross-edges)
- **api/v1 · writeError** (3 cross-edges)
- **krs/krscommon +2 dirs** (3 cross-edges)
- **service/healthcheckin +6 dirs** (3 cross-edges)
- **auth +4 dirs · fillGoalRefProgress** (3 cross-edges)
- **usecase/export +2 dirs** (2 cross-edges)
- **teams/activate · Post · handler · routes (10) #2** (2 cross-edges)
- **comments/unresolve** (2 cross-edges)
- **comments/resolve · New** (2 cross-edges)
- **settings/healthcheckin** (2 cross-edges)
- **v1/config +2 dirs** (2 cross-edges)
- **service/notificationpref +3 dirs** (2 cross-edges)
- **render/export +1 dirs · Filename** (2 cross-edges)
- **store/periods +8 dirs** (2 cross-edges)
- **web/shell +4 dirs** (2 cross-edges)
- **comments/unresolve +1 dirs** (2 cross-edges)
- **v1/hierarchy** (2 cross-edges)
- **comments/resolve · Post** (2 cross-edges)
- **render/export +1 dirs · Markdown** (2 cross-edges)
- **web/common** (2 cross-edges)
- **. +4 dirs · ParseNumericalMeta** (2 cross-edges)
- **teams/close · Post · handler · routes (10) #2** (2 cross-edges)
- **. +2 dirs · TestBotIDRetryAfterTransientErr…** (1 cross-edges)
- **usecase** (1 cross-edges)
- **service/goal +2 dirs** (1 cross-edges)
- **admin/invitations** (1 cross-edges)
- **service/healthcheckin +3 dirs** (1 cross-edges)
- **http/dto +2 dirs** (1 cross-edges)
- **v1/admin · Get · admincommon · handler (16)** (1 cross-edges)
- **settings/access** (1 cross-edges)
- **service/activity · Feed** (1 cross-edges)
- **activity/activitycommon · TestSinceFromRange** (1 cross-edges)
- **v1/admin · Post** (1 cross-edges)
- **service/team** (1 cross-edges)
- **service/keyresult +4 dirs** (1 cross-edges)
- **v1/notifications +2 dirs** (1 cross-edges)
- **service/period** (1 cross-edges)
- **store/memberships +14 dirs** (1 cross-edges)
- **accessrequests/approve** (1 cross-edges)
- **users/admin** (1 cross-edges)
- **v1/admin · Get · admincommon · handler (18)** (1 cross-edges)
- **service/goal +8 dirs** (1 cross-edges)
- **usecase/goal · SetParents** (1 cross-edges)
- **service/notificationchannel +10 dirs** (1 cross-edges)
- **admin/admincommon +2 dirs** (1 cross-edges)
- **http · FeedResponse** (1 cross-edges)
- **accessrequests/deny** (1 cross-edges)
- **admin/members** (1 cross-edges)
- **auth · ctxWithAllowedTeams** (1 cross-edges)
- **core/event +2 dirs · UpdateWithMeta** (1 cross-edges)
- **store/krs · TestKRsScopedByTenant** (1 cross-edges)

## How to Explore

```
analyze(operation:"communities", id:"community-71")
explore(operation:"context", task:"understand auth +67 dirs", format:"gcx")
relations(operation:"usages", target:{symbol:"internal/http/handlers/api/v1/hierarchy/handler.go::Handler.HandleHierarchy"}, format:"gcx")
```

_`format: "gcx"` returns the [GCX1 compact wire format](../../docs/wire-format.md) — round-trippable, ~27% fewer tokens than JSON. Drop it for JSON output; agents using `@gortex/wire` or the Go `github.com/gortexhq/gcx-go` package decode either._
