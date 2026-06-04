# UDID-Based User Linking

**Date:** 2026-06-04  
**Status:** approved

## Problem

Users are currently linked to goals and teams by `display_name` (plain text):

- `goals.owner_text` — comma-separated display names stored as free text
- `teams.lead` — display name of team lead stored as free text
- `GET /api/v1/health-checkin` scope — computed by matching `user.display_name` against `teams.lead` and `goals.owner_text`
- User search scope (`SearchUsersInSet`) — finds eligible users by matching `leadNames []string` against `users.display_name`

This creates two security risks:
1. `display_name` is not unique — two users named "Ivan Ivanov" share scope
2. Renaming a user can grant or revoke access that should not change

## Solution

Replace display-name linking with UDID (UUID string) linking throughout, keeping `owner_text` and `teams.lead` as display-only fields.

---

## Schema Changes

### `teams` — add column

```sql
ALTER TABLE teams ADD COLUMN lead_udid TEXT REFERENCES users(udid) ON DELETE SET NULL;
```

- `lead TEXT` remains for display and no-auth mode
- `lead_udid` is set alongside `lead` in auth mode; NULL in no-auth
- `ON DELETE SET NULL` ensures referential safety at DB level

### `goals` — add column

```sql
ALTER TABLE goals ADD COLUMN owner_udids TEXT[];
```

- `owner_text TEXT` remains for display and no-auth mode
- `owner_udids TEXT[]` stores UDIDs of owners in auth mode; NULL/empty in no-auth
- No FK constraint (TEXT[] array); integrity enforced at write time via validation

### Backfill migration

Best-effort: match `teams.lead` and names in `goals.owner_text` against `users.display_name`.  
- Unique match → populate `lead_udid` / `owner_udids`
- Ambiguous match (multiple users with same display_name) → leave NULL, do not guess

---

## Domain Model

```go
// Team
LeadUDID *string   // new field, nil when not set

// Goal
OwnerUDIDs []string  // new field, nil/empty when not set
```

---

## Health-Check Scope

`computeScope` is updated to accept `userUDID string` instead of `userDisplayName string`.

**Admin bypass:** if `user.IsAdmin == true` (includes `anonymous-local` in no-auth mode), skip scope computation and return all teams. Health-check returns full data.

**Auth mode scope rules:**
- lead-scope: teams where `lead_udid = userUDID` + all descendants
- owner-scope: teams with goals where `userUDID = ANY(owner_udids)`

Text-based fallback (`teams.lead`, `owner_text`) is removed from scope computation.

**Handler change:** passes `user.UDID` (not `user.DisplayName`) to `GetHealthCheckIn`.

---

## User Search Scope

`SearchUsersInSet` currently passes `leadNames []string` to SQL (`display_name = ANY($2)`).

After change: passes `leadUDIDs []string` → SQL matches `udid = ANY($2)`.

---

## API Contract

### Goal create / update (all three handlers)

**Before:** `owner_text: "Ivan Petrov, Maria Sidorova"`  
**After:** `owner_udids: ["uuid1", "uuid2"]`

- `owner_text` removed from write endpoints in auth mode
- In no-auth mode `owner_udids` is omitted; `owner_text` remains a plain text field
- Validation: all UDIDs in `owner_udids` must exist in `users` → `400 VALIDATION_ERROR` otherwise

**Goal response** — `owners[]` format unchanged: `{udid, display_name, avatar_url}`.  
Resolution changes from name-lookup to UDID-lookup.

### Team lead (admin)

**Before:** `lead: "Ivan Petrov"`  
**After:** `lead_udid: "uuid"`

- `lead` (display name) populated by server from user record on resolve
- Validation: UDID must exist in `users` → `400 VALIDATION_ERROR` otherwise

### Health-check spec update

Scope section changes from:

> сервер определяет по `user.display_name` из сессии: lead-scope: `teams.lead = display_name`; owner-scope: `goal.owner_text` содержит `display_name`

To:

> сервер определяет по UDID пользователя из сессии. Admin (включая no-auth режим) видит всё без scope-фильтрации. Для обычных пользователей: lead-scope: `teams.lead_udid = user.udid` + потомки; owner-scope: goals где `user.udid = ANY(owner_udids)`.

---

## Corner Cases: Non-Existent UDIDs

### On write
- Validate all `owner_udids` and `lead_udid` exist in `users` at write time
- Unknown UDID → `400 VALIDATION_ERROR`; do not silently ignore

### On read (goal response)
- `owner_udids` is TEXT[], no FK — UDID can become stale if user is deleted directly in DB
- Bulk-lookup by UDID; unresolvable UDID → placeholder: `{udid: "...", display_name: "Удалённый пользователь", avatar_url: null}`
- Never silently drop — UI must know the owner slot existed

### `teams.lead_udid`
- FK `ON DELETE SET NULL` clears `lead_udid` automatically on user deletion
- `lead TEXT` preserves display name for history

### Health-check scope
- Stale UDID in `owner_udids` → no match against live users → goal excluded from scope
- No access leak: deleted user loses scope coverage

### Practical note
- Current domain has no user soft-delete or deactivation; direct DB deletion is the only trigger for these cases

---

## No-Auth Mode

- `anonymous-local` user is `IsAdmin = true` → bypasses scope in health-check
- `owner_udids` and `lead_udid` are not set; `owner_text` / `teams.lead` remain plain text
- Write endpoints in no-auth mode continue to accept `owner_text` as plain text
- No behaviour change for no-auth users

---

## Frontend

- `UserSelector` (tracker.js, admin.js, app.js) already returns objects with `udid`
- Goal save payload: send `owner_udids[]` instead of `owner_text` in auth mode
- Team lead admin form: send `lead_udid` instead of `lead`
- In no-auth mode: `UserSelector` is not used for owner input; `owner_text` remains a plain text `<input>`

---

## Tests

- Unit: `computeScope` — UDID match, admin bypass, empty `owner_udids`, nil `lead_udid`
- Unit: `ResolveOwners` — placeholder for unresolvable UDIDs
- Handler: write validation — unknown UDID → 400
- Migration: backfill logic — unique name match populates UDID; ambiguous match leaves NULL

---

## Definition of Done

- Migration added for `lead_udid` and `owner_udids` columns + backfill
- Domain `Team.LeadUDID` and `Goal.OwnerUDIDs` added
- `computeScope` uses UDID; admin bypass for no-auth
- `SearchUsersInSet` uses UDID for lead matching
- All three goal handlers updated (api/v1/goals, api/v1/teams, web/goals)
- Admin team handler updated
- Specs `040-api-contract.md` and `020-domain-model.md` updated
- Tests added for new behaviour
