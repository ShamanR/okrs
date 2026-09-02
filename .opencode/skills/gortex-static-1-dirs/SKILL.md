---
name: gortex-static-1-dirs
description: "Work in the static +1 dirs area — 565 symbols across 23 files (100% cohesion)"
---

# static +1 dirs

565 symbols | 23 files | 100% cohesion

## When to Use

Use this skill when working on files in:
- ``
- `web/static/activity.js`
- `web/static/admin.js`
- `web/static/api.js`
- `web/static/balance_bars.js`
- `web/static/goal_tree.js`
- `web/static/goal_tree_layout.js`
- `web/static/markdown.js`
- `web/static/no_access.js`
- `web/static/notifications.js`
- `web/static/period-overview.js`
- `web/static/period_overview_view.js`
- `web/static/period_select.js`
- `web/static/period_url.js`
- `web/static/progress_chart.js`
- `web/static/settings.js`
- `web/static/sidebar.js`
- `web/static/storage.js`
- `web/static/stub.js`
- `web/static/system.js`
- `web/static/tracker.js`
- `web/static/ui.js`
- `web/static/userselector.js`

## Key Files

| File | Symbols |
|------|---------|
| `` | removeChild, sort, pop, appendChild, forEach, ... |
| `web/static/activity.js` | openMenu, writeFavorites, apiGet, eventMarkdownBody, walk, ... |
| `web/static/admin.js` | save, onPop, App, UserSelector, apiPut, ... |
| `web/static/api.js` | readCSRF, csrfHeaders |
| `web/static/balance_bars.js` | BalanceBars |
| `web/static/goal_tree.js` | collapsedDescendants, endPan, pick, openPanel, GoalTreeApp, ... |
| `web/static/goal_tree_layout.js` | place, shiftSubtree, sameBandParents, cmp, pr, ... |
| `web/static/markdown.js` | onChange, Markdown, applyMarkdownFormat, MarkdownEditor, renderMarkdown, ... |
| `web/static/no_access.js` | NoAccess, submit |
| `web/static/notifications.js` | NotificationsBell, onFocus, _notifSafeHref, _notifAgo, markRead, ... |
| `web/static/period-overview.js` | apiGet, load, App, dur, onApply, ... |
| `web/static/period_overview_view.js` | drillTeams, PeriodOverviewContent, tile, teamsByStatus, teamsWithErr, ... |
| `web/static/period_select.js` | openMenu, fmtPeriodDate, PeriodSelect, fmtDateRange |
| `web/static/period_url.js` | writePeriodURL, periodLinkParams, pickPeriodFromURL, readPeriodURL, h, ... |
| `web/static/progress_chart.js` | yOf, pcFormatDate, xOf, ProgressChart |
| `web/static/settings.js` | TypeBadge, subtreeState, collectAll, patch, keep, ... |
| `web/static/sidebar.js` | onKey, FeedbackNudge, logout, dismiss, SidebarSections, ... |
| `web/static/storage.js` | readJSON, writeJSON |
| `web/static/stub.js` | StubApp |
| `web/static/system.js` | save, TenantsSection, UsersSection, del, NotificationChannelsSection, ... |
| `web/static/tracker.js` | clampPct, SETTINGS_SIDEBAR_KEY, SidebarNode, fmtVal, rem, ... |
| `web/static/ui.js` | __modalUnlockScroll, ModalCloseConfirm, buildTargetURL, __modalLockScroll, onKey, ... |
| `web/static/userselector.js` | UserSelector, handleQueryChange, _cachedUsersList, onKey, _cacheUserRef, ... |

## Entry Points

- `web/static/tracker.js::App`
- `web/static/goal_tree.js::GoalTreeApp`
- `web/static/activity.js::App`
- `web/static/activity.js::Feed`
- `web/static/goal_tree.js::RootPicker`

## How to Explore

```
analyze(operation:"communities", id:"community-279")
explore(operation:"context", task:"understand static +1 dirs", format:"gcx")
relations(operation:"usages", target:{symbol:"web/static/tracker.js::App"}, format:"gcx")
```

_`format: "gcx"` returns the [GCX1 compact wire format](../../docs/wire-format.md) — round-trippable, ~27% fewer tokens than JSON. Drop it for JSON output; agents using `@gortex/wire` or the Go `github.com/gortexhq/gcx-go` package decode either._
