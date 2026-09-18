# OKR Tracker

OKR Tracker is a self-hosted web application for running OKRs in an organisation that plans as a
tree — department, cluster, unit, group, team, squad. You pick a team and a period in the sidebar,
and the right side shows that team's objectives, their key results, the discussion around them, and
how the whole subtree underneath is doing. Climb one level up and the same screen becomes a roll-up
of everything below it.

It is a finished application rather than a kit to assemble: planning, check-ins, discussion,
decomposition, notifications, export and administration are all part of it. What you deploy is one
Go binary plus PostgreSQL — migrations run on startup, the front end is served from the binary, and
there is no build step, no Node runtime and no separate API service to operate.

![Team board](docs/screenshots/tracker-board.png)

> The product interface is Russian; this README is English. Screenshots show the interface as it is.

## Features

- **[Team board](#team-board)** — a team in a period: status stepper, weighted objectives, forecast, and a roll-up card per child team.
- **[Key results](#key-results)** — three kinds (numerical, boolean, project), each with its own check-in form, note and zeroing criteria.
- **[Progress and forecast](#progress-and-forecast)** — goal progress is the weighted average of its KRs, team progress the weighted average of its goals, and a pace line says whether you are ahead or behind.
- **[Shared goals](#shared-goals)** — one goal visible in several teams, each with its own weight and ordering. No copies to keep in sync.
- **[Goal tree](#goal-tree)** — the decomposition graph: annual goals above, quarterly goals below, edges between them. Pan, zoom, collapse.
- **[Discussion](#discussion)** — two-level comments: a remark that can be resolved and reopened, plus replies. Markdown throughout.
- **[Period overview](#period-overview)** — one period across many teams: status counts, weight errors, Discovery/Delivery and priority balances, progress over time, and bulk status changes.
- **[Activity log](#activity-log)** — append-only history of who changed what and when, filterable by category, author and time.
- **[Notifications](#notifications)** — in-app bell plus per-type delivery scope; Mattermost is included as an external channel.
- **[Markdown export](#markdown-export)** — one goal, a team, or a whole subtree; short or detailed; with or without comments.
- **[Administration](#administration)** — hierarchy, periods, users, invitations, access grants, and health check-in thresholds.
- **[OAuth2 / OIDC sign-in](#authentication-and-access)** — Google, GitHub and Keycloak in the box. Providers come from a registry, so teaching it another one is a small package rather than a fork.
- **[Access control](#authentication-and-access)** — run it wide open, or require sign-in and grant visibility per hierarchy node.
- **[Spaces](#spaces-and-the-system-panel)** — several isolated spaces in one instance, administered from a system-level panel.

## Quick install

You need Docker and about two minutes.

```bash
git clone https://github.com/ShamanR/okrs.git
cd okrs
docker compose up --build
```

The app comes up on <http://localhost:8080>. Migrations apply themselves on the first start, so the
database is ready by the time the log prints `app_ready`.

An empty tracker is not very interesting, so load the demo data:

```bash
docker compose exec -T db psql -U postgres -d okrs < seed_demo.sql
```

That script starts with `TRUNCATE`, so it is safe to re-run — it replaces the contents rather than
adding to them. Never point it at a database you care about. It gives you 32 teams across two
parallel hierarchies, nine periods (annual and quarterly), 31 goals with 60 key results of all three
kinds, shared goals, a discussion thread with a resolved remark, a decomposition tree and an
activity feed.

If you would rather have a small English-labelled dataset instead, start the server with `-seed` —
see [Run it locally](#run-it-locally) below.

## Run it locally

For development you do not need the app in Docker, only Postgres. Go 1.25 or newer.

```bash
docker compose up -d db                 # Postgres on :5432, nothing else

export DATABASE_URL=postgres://postgres:postgres@localhost:5432/okrs?sslmode=disable
export PORT=8080
export LOG_FORMAT=text                  # readable lines instead of JSON
export WEB_ASSETS_DEV=1                 # development React build, usable stack traces

go run ./cmd/server -seed
```

Migrations apply themselves, and `-seed` creates three teams — Platform, Payments, Growth — with
two goals each in the current quarter, plus one annual goal above them so the goal tree has
something to draw. Open <http://localhost:8080> and pick the quarter in the period selector: it
opens on the enclosing year, which holds only that single parent goal.

Front-end changes need no build step — JSX in `web/static/` is compiled in the browser, so reload
the page and you have them. Go changes need a restart.

The rest of the loop:

```bash
go test ./...                                                        # DB tests skip without Docker
docker compose exec -T db psql -U postgres -d okrs < seed_demo.sql   # swap in the large dataset
docker compose down -v                                               # drop the database, start clean
```

Beware that `-seed` is idempotent for team rows but not for goals, so a second run against the same
database gives you the goals twice. `seed_demo.sql` truncates first and is safe to repeat.

## Quick start

Open <http://localhost:8080> and work through the quarter in the order it actually happens.

1. **Find your team.** The sidebar is the org tree. Click a team to open its board; click a unit or
   cluster to see the same board plus a roll-up of everything below it. The team and period you are
   looking at are always in the URL (`/?team=102&period=2`), so you can paste a link into a chat and
   your colleague lands on exactly the same screen.
2. **Pick the period.** Top of the sidebar. Periods nest: a quarter sits inside its year, and the
   goal tree uses that nesting to place annual goals above quarterly ones.
3. **Write the objectives.** While the team is in *Черновик* (draft), the board is fully editable.
   Give every goal a weight; the weights of a team's goals should add up to 100. That is the point
   of the number — it is how the team says where its attention goes.
4. **Add key results.** Each goal decomposes into KRs whose weights also add up to 100. Pick the
   kind that matches how you will actually measure: a number moving from a start to a target, a
   yes/no fact, or a set of weighted project stages.
5. **Hand them over.** Move the team to *К валидации* when the goals are ready for a manager to
   look at, then to *В работе* once they are agreed. From that point the composition is frozen
   server-side and the board switches to check-ins and comments.
6. **Keep it current.** *Обновить прогресс* on a KR opens the right form for its kind and records
   the new value, a health status and an optional note in one operation. Everything else — the
   goal's percentage, the team's, the subtree's, the forecast marker — follows from that.
7. **Close the quarter.** *Закрыты* leaves comments available and nothing else.

Two places are worth a visit once you have data: **Обзор периода** shows a whole period across many
teams at once, and **Лог активностей** shows what changed and who changed it.

## Screenshots

**A unit's board** — its own goals, plus one card per child team with progress, status and the
count of P0–P1 objectives.

![Team board](docs/screenshots/tracker-board.png)

**Key results inside a goal.** Weight, kind, current value against the target, the zeroing criteria
that can invalidate a result, and the check-in button.

![Key results](docs/screenshots/key-results.png)

**The goal tree.** Annual goals in the upper band, the quarterly goals they decompose into below,
grouped by parent and team.

![Goal tree](docs/screenshots/goal-tree.png)

**Period overview.** How a period is going across the organisation: status counts, formation
errors, balances, and the progress curve against an even-pace diagonal.

![Period overview](docs/screenshots/period-overview.png)

**Activity log.** Append-only, grouped by day, filterable by category, author, favourites and time
window.

![Activity log](docs/screenshots/activity-log.png)

## Reference

### Running the server

The binary takes its configuration from the environment and has exactly one flag.

```bash
go run ./cmd/server          # normal start
go run ./cmd/server -seed    # also create a small demo dataset on startup
```

`-seed` creates three teams (Platform, Payments, Growth) with two goals each in the current
quarter, a project KR with stages, a numerical KR, progress snapshots for the chart, and a
two-band goal tree. It is idempotent for team rows but not for goals, so repeated runs against the
same database duplicate the goals. [Run it locally](#run-it-locally) shows the full development
command line.

Migrations are applied from `migrations/` at startup — there is no separate migrate step. The
process handles `SIGINT`/`SIGTERM` gracefully: it stops accepting requests, drains the event bus so
queued notifications still reach the database, flushes the notification channels, and only then
closes the pool.

### Configuration

| Variable | Default | What it does |
| --- | --- | --- |
| `DATABASE_URL` | `postgres://postgres:postgres@localhost:5432/okrs?sslmode=disable` | PostgreSQL connection string |
| `PORT` | `8080` | HTTP port |
| `TZ` | `Asia/Bangkok` | Time zone used for dates and period boundaries |
| `WEB_ASSETS_DEV` | `0` | Serve the development React build from `/static/vendor` (`1`/`true`/`on`) |
| `LOG_FORMAT` | `json` | `json` for a collector, `text` for reading with your eyes |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `SERVICE_NAME` | `okrs` | Value of the `service` field in every log record |
| `ENV` | `dev` | Value of the `env` field in every log record |
| `AUTH_MODE` | `disabled` | `disabled` or `enabled` |
| `AUTH_ENABLED_PROVIDERS` | — | Comma-separated: `google,github,keycloak` |
| `AUTH_SESSION_COOKIE_NAME` | `okrs_session` | Session cookie name |
| `AUTH_SESSION_TTL` | `720h` | Session lifetime |
| `AUTH_BASE_URL` | — | Public URL of the instance, used to build callback URLs |
| `AUTH_DEFAULT_NEW_USER_POLICY` | `empty` | `empty` or `default_node` — what a brand-new user can see |
| `AUTH_DEFAULT_NODE_ID` | — | Hierarchy node granted under the `default_node` policy |
| `AUTH_GOOGLE_CLIENT_ID` / `_SECRET` / `_REDIRECT_URL` | — | Google OAuth2 |
| `AUTH_GITHUB_CLIENT_ID` / `_SECRET` / `_REDIRECT_URL` | — | GitHub OAuth2 |
| `AUTH_KEYCLOAK_ISSUER_URL` / `_CLIENT_ID` / `_SECRET` / `_REDIRECT_URL` | — | Keycloak (OIDC realm endpoint) |
| `NOTIFICATIONS_SECRET_KEY` | — | Encrypts external-channel credentials at rest. Without it, channels that store a secret cannot be configured |
| `BOOTSTRAP_SYSTEM_ADMIN` | — | `provider:subject` or an email, promoted to system admin on first matching login while no system admin exists |
| `PROVISIONING_TOKEN` | — | Bearer token authorising machine callers of `/api/v1/system/*` |

### Team board

The board is the main screen. Its header carries the team name, its type badge, the description its
lead wrote, and the aggregate progress for the period. Below that is the status stepper, then —
for a node with children — a roll-up card per child team, then the node's own goals.

Each goal card shows priority (P0–P3), weight, work type (Delivery or Discovery), strategic focus,
the driver, a progress bar with a forecast marker, and how recently it was touched. A goal shared
with other teams carries a banner naming them. The `🔗` icon copies a deep link to that goal.

The status a team is in decides what the board lets you do, and the server enforces the same rule
rather than trusting the UI:

| Status | What is possible |
| --- | --- |
| Нет целей | No goals yet; the lifecycle steps are not selectable |
| Черновик | Full editing: create, edit, delete goals and KRs |
| К валидации | Still fully editable; the goals have been handed to a manager to review |
| В работе | Progress check-ins and comments. Composition is frozen |
| Закрыты | Comments only |

### Key results

A key result belongs to exactly one goal and comes in three kinds.

| Kind | Progress is |
| --- | --- |
| Числовой | Linear from a start value to a target, in the unit you name — or by explicit checkpoints if you define them |
| Бинарный | 100% when done, 0% otherwise |
| Проектный | The summed weight of the completed stages |

Beyond the number, a KR can carry a **description** (what exactly is being measured — it is shown
in the check-in dialog, where it matters), a **zeroing criterion** — the condition under which the
result does not count even if the number is reached — and a **note** for context and blockers.

![Update progress](docs/screenshots/kr-progress.png)

A check-in is a single operation: value, health status and note are saved together, and the
previous values are kept. The health status (Not Started / On Track / At Risk / Closed) is a
judgement call by a human and deliberately does not feed into any calculation.

### Progress and forecast

Everything rolls up by weight, with one rule used everywhere:

- goal progress = weighted average of its key results;
- team progress in a period = weighted average of its goals;
- a parent node's progress = the average across child teams that actually have goals.

Alongside the bar sits the **expected pace** — where you would be today if the period filled evenly
— and progress is labelled against it: *ahead*, *on track* (within the tolerance), *below*, or
*stale* when nobody has updated it for a while. The thresholds, including a "count as on track
above N%" override, live in the admin panel under Health Check-in.

![Health check-in settings](docs/screenshots/admin-health-checkin.png)

These same thresholds decide which problems — goals without updates, teams with no goals, teams
waiting to be moved into work, weight sums that do not reach 100, goals behind pace — get counted
as things needing attention.

### Shared goals

A goal is owned by one team but can be shared with others. Each participating team stores its own
weight and its own ordering for it, so a goal that is central to one team and a side contribution
for another is weighted accordingly, without duplicating anything. Sharing does not change who can
edit the goal, and the Markdown export of a subtree deduplicates shared goals rather than printing
them once per team.

### Goal tree

The goal tree is a read-only view of the decomposition graph: which annual or manager-level goal a
quarterly team goal came from. A goal may have several parents and several children; cycles are
rejected, and a link to a goal in a team you cannot see simply does not exist for you.

![Goal tree](docs/screenshots/goal-tree.png)

Goals are banded by the depth of their period, so an annual band sits above the quarterly one and
edges cross between them. Sidebar toggles control whether links across periods are followed,
whether unlinked goals are hidden, and whether the view narrows to teams you lead; a root picker
focuses the tree on a single branch. Click a card for a read-only detail panel with a link straight
into the tracker.

### Discussion

Comments on a goal are two levels deep and have a state. A top-level comment is a *remark* — it can
be marked resolved and reopened later, and unresolved ones are counted on the goal card. Replies
hang under a remark and have no state of their own.

![Discussion](docs/screenshots/goal-discussion.png)

Comment bodies, goal and KR descriptions, team descriptions and KR notes all accept a safe subset
of Markdown — bold, italic, headings, lists, quotes, inline code and links — with a preview tab in
the editor.

On the board a long goal or KR description is shown clipped to the first few lines, faded at the
cut, with a **Показать полностью** toggle that expands it in place — so a detailed description does
not turn the card into a wall of text.

### Period overview

The overview answers "how is this period going" for a set of teams rather than one. Scope switches
between the teams you lead and the whole organisation.

![Period overview](docs/screenshots/period-overview.png)

It counts teams by status, flags formation errors (goal weights that do not sum to 100), shows the
Discovery/Delivery split, the strategic focus mix, the P0–P3 distribution and the KR health
breakdown — each bar clickable to see what is inside it. The chart plots recorded progress against
an even-pace diagonal. At the bottom are the bulk operations: move every team that has goals into
*В работе*, or close the period for all of them at once. Teams without goals are skipped and the
operation is idempotent.

### Activity log

![Activity log](docs/screenshots/activity-log.png)

The log is append-only and covers progress updates, goal and KR composition changes, status
transitions and discussion events. Filter by category, author, favourite teams, or a time window,
and search the text. Each entry links back to the goal it is about. A space administrator can purge
the log; nothing else modifies it.

### Notifications

The bell in the sidebar is available on every page and shows what happened to things you are
involved in: comments on your goals, your remarks being resolved, goal and KR changes, and progress
updates.

![Notifications](docs/screenshots/notifications.png)

Under **Настройки → Уведомления** each person decides, per event type, whether to receive it and how
far down the hierarchy to look — only my teams, one level below, or the whole subtree. "Someone
resolved my remark" ignores scope and always arrives.

![Notification preferences](docs/screenshots/settings-notifications.png)

External delivery is pluggable and Mattermost ships in the box. Updates accumulate for a while and
go out as one message rather than one per event, and anything still buffered is flushed on
shutdown. A channel that stores a credential needs `NOTIFICATIONS_SECRET_KEY` set on the instance
and the channel granted to the space from the system panel; until both are true the admin page says
the space has no external channels.

### Markdown export

The `···` menu on a goal card opens the export dialog. Three scopes: this goal, the team's goals,
or the team and everything below it. Two levels of detail, comments optional.

![Markdown export](docs/screenshots/export-markdown.png)

You get a preview, a copy button and a `.md` download. Export never changes anything, is available
in every period status, and is cut to your visibility on the server rather than in the browser.

### Administration

Space administration lives under `/admin`.

**Команды** — the hierarchy. Seven node types from department down to employee, each team with a
parent and a lead. Deleting is context-aware: a team that has goals in any period is soft-deleted so
history survives and can be restored; a team with no goals is removed outright. Children are lifted
one level up rather than disappearing with the parent.

![Teams administration](docs/screenshots/admin-teams.png)

**Периоды** — the planning horizons. Nesting (year → quarter) and status (planned / active /
closed) are both computed from the dates; only archiving is manual, and it is reversible.

![Periods administration](docs/screenshots/admin-periods.png)

**Пользователи** — everyone who has ever signed in, their role, and which hierarchy nodes they can
reach. Invitation links are generated here, single-use or reusable, with an optional expiry.

![Users administration](docs/screenshots/admin-users.png)

**Настройки** — the space name, a documentation URL that adds a *Документация* item to the user
menu, and the message shown to someone with no team access. **Health Check-in** holds the
thresholds described above, and **Уведомления** lists the external channels the space has been
granted.

### Authentication and access

**Without auth** (`AUTH_MODE=disabled`, the default) every page and API is open and actions are
attributed to the system user `anonymous-local`, which holds the admin role inside space #1. Good
for a local demo; not something to expose.

**With auth** (`AUTH_MODE=enabled`) unauthenticated page requests are redirected to `/login` and
API requests get `401`. Sessions are stored in the database. Providers are registered by adding
their name to `AUTH_ENABLED_PROVIDERS` and filling in the matching variables:

```bash
AUTH_MODE=enabled
AUTH_ENABLED_PROVIDERS=google,github
AUTH_BASE_URL=https://okr.example.com

AUTH_GOOGLE_CLIENT_ID=...
AUTH_GOOGLE_CLIENT_SECRET=...
AUTH_GOOGLE_REDIRECT_URL=https://okr.example.com/auth/google/callback

AUTH_GITHUB_CLIENT_ID=...
AUTH_GITHUB_CLIENT_SECRET=...
AUTH_GITHUB_REDIRECT_URL=https://okr.example.com/auth/github/callback
```

Keycloak works the same way with `AUTH_KEYCLOAK_ISSUER_URL` pointing at the realm.

Providers live in a registry rather than a switch statement. Each one is a package under
`internal/auth/providers/` that calls `auth.Register("name", factory)` from its `init`, and
`cmd/server/main.go` pulls it in with a blank import; Google, GitHub and Keycloak are wired exactly
that way. Supporting a fourth — your company's OIDC, GitLab, anything speaking OAuth2 — is one
small package and one import line, and the name you register is the name that then goes into
`AUTH_ENABLED_PROVIDERS`. A name that nothing registered fails the start with
`auth: unknown provider "x" (not registered)` rather than quietly doing nothing.

Visibility is granted per hierarchy node: a user sees the nodes granted to them and everything
below. A brand-new user sees nothing by default; set `AUTH_DEFAULT_NEW_USER_POLICY=default_node`
with `AUTH_DEFAULT_NODE_ID` if you would rather they land somewhere. Administration is split into
three planes with separate gates — space administration under `/admin`, instance administration
under `/system`, and machine provisioning via `PROVISIONING_TOKEN` — so being a space admin does not
make you a system admin.

### Spaces and the system panel

One instance can host several spaces (tenants). Data does not cross between them: every query is
scoped, and a user can belong to more than one space and switch between them from
**Настройки → Мои пространства**.

`/system` is the instance-level panel, reachable only by a system administrator or, for machine
callers, with `PROVISIONING_TOKEN`. It creates and suspends spaces, manages their members and
roles, grants entitlements such as the Mattermost channel, and sets instance-wide settings like the
default registration space. The first system administrator is bootstrapped with
`BOOTSTRAP_SYSTEM_ADMIN`: the first login matching that `provider:subject` or email is promoted,
and only while no system administrator exists yet.

### Logging

The application writes structured logs to stdout, one JSON object per line, and opens no connection
to any log system — shipping is the collector's job (Filebeat, Fluent Bit, Vector). The one thing
to agree on before rollout is that the pipeline parses JSON rather than grepping a text pattern.
`LOG_FORMAT=text` gives human-readable output without a code change.

Every record carries `time`, `level`, `msg`, `event`, `service`, `env`; request-scoped records also
carry `request_id` and, when known, `tenant_id`, `actor_id`, `team_id`, `period_id`.

`event` is the stable machine-readable record type that filters and alerts are built on:

| `event` | Written when |
| --- | --- |
| `http_request` | Any HTTP request completes. Level follows the status: 5xx → `error`, 4xx → `warn` |
| `http_panic` | Unhandled panic, with stack |
| `domain_event` | A domain event occurred (type only, no user text) |
| `event_dropped` | An event did not reach a subscriber |
| `auth_login` / `auth_logout` / `auth_failed` | Sign-in, sign-out, failed authentication |
| `authz_denied` | Access refused, with the reason and the requested resource |
| `access_changed` | Role, grant or administrative setting changed |
| `app_start` / `app_ready` / `app_shutdown` | Startup, readiness, graceful stop |
| `migration` | Result of applying migrations |
| `background_task` / `background_panic` | Background job lifecycle and failures |
| `external_call` | Outcome of a call to an external system |
| `config_invalid` | Bad logging configuration |

Renaming one of these breaks dashboards the same way renaming an API field does. Treat them as an
external contract.

Each request gets a `request_id`; an incoming `X-Request-Id` is reused if it passes validation, and
the id is returned to the client in the same header, so a user can quote it.

Secrets, email addresses, display names, the contents of domain events (goal titles, comment text,
check-in notes) and URL query strings are deliberately kept out of the logs. Users appear as
numeric ids.

### Development

[Run it locally](#run-it-locally) has the working loop. Beyond that:

- The front end is plain React with JSX compiled in the browser — no bundler, no `node_modules`.
  Sources live in `web/static/`, templates in `web/templates/`, and both are embedded into the
  binary. `WEB_ASSETS_DEV=1` swaps in the development React build for readable stack traces.
- `go test ./...` runs everything. Tests that need a database use testcontainers and skip
  themselves when Docker is unavailable.
- If you change a table, keep `seed_demo.sql` working — it is the dataset the screenshots and the
  demo rely on.

### Documentation and specifications

`openspec/specs/` is the normative description of how the product behaves, one directory per
capability (goal management, progress tracking, notifications, tenancy, authorization and so on).
It is the source of truth: when code and spec disagree, the spec is the thing to reconcile against.
`openspec/changes/` holds proposed changes together with their design and task list.

`docs/` is the end-user guide — how to phrase objectives, how to pick key results, what happens
through the quarter, how to close it — starting at [docs/index.md](docs/index.md).

Both are written in Russian.
