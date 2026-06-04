# UDID-Based User Linking Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace all display-name-based user linking (goal owners, team leads, health-check scope) with UDID-based linking.

**Architecture:** Add `lead_udid TEXT REFERENCES users(udid)` to teams and `owner_udids TEXT[]` to goals. Display-name fields remain for UI rendering. Health-check scope switches to UDID matching with admin bypass. All write endpoints validate UDIDs at request time.

**Tech Stack:** Go, PostgreSQL/pgx v5, React (CDN), no build step.

---

## File Map

| File | Change |
|------|--------|
| `migrations/022_udid_linking.up.sql` | new — add columns + backfill |
| `migrations/022_udid_linking.down.sql` | new — drop columns |
| `internal/domain/models.go` | add `LeadUDID *string` to Team, `OwnerUDIDs []string` to Goal |
| `internal/store/teams/teams.go` | TeamInput + SQL to include lead_udid |
| `internal/store/goals/goals.go` | GoalInput variants + SQL to include owner_udids |
| `internal/store/users/users.go` | SearchUsersInSet → leadUDIDs; ListUserLeadTeams → UDID-keyed; add ValidateUDIDsExist |
| `internal/service/healthcheckin.go` | computeScope by UDID; GetHealthCheckIn admin bypass |
| `internal/service/service.go` | SearchUsersInScope uses leadUDIDs; add ValidateUserUDIDsExist |
| `internal/http/handlers/api/v1/helpers_response.go` | BuildUserRefMap UDID-keyed; add ResolveOwnersByUDIDs |
| `internal/http/handlers/api/v1/healthcheckin/handler.go` | pass user.UDID + user.IsAdmin |
| `internal/http/handlers/api/v1/users/handler.go` | leadTeams[u.UDID] |
| `internal/http/handlers/api/v1/goals/handler.go` | read via GetUsersByUDIDs; write accepts owner_udids |
| `internal/http/handlers/api/v1/goals/response.go` | use ResolveOwnersByUDIDs |
| `internal/http/handlers/api/v1/teams/handler.go` | collect UDIDs for read; create goal accepts owner_udids |
| `internal/http/handlers/api/v1/hierarhy/handler.go` | collectLeadUDIDs; GetUsersByUDIDs |
| `internal/http/handlers/api/v1/admin/service_handler.go` | teamRow adds lead_udid; create/update accepts lead_udid, validates, fills lead |
| `internal/service/healthcheckin_test.go` | update for UDID signatures |
| `internal/store/users/users_test.go` | update SearchUsersInSet test |
| `specs/040-api-contract.md` | update health-checkin scope description |
| `specs/020-domain-model.md` | add LeadUDID and OwnerUDIDs to domain fields |

---

## Task 1: DB Migration

**Files:**
- Create: `migrations/022_udid_linking.up.sql`
- Create: `migrations/022_udid_linking.down.sql`

- [ ] **Step 1: Write up migration**

Create `migrations/022_udid_linking.up.sql`:

```sql
-- Add lead_udid to teams (FK → users.udid, nullable)
ALTER TABLE teams ADD COLUMN lead_udid TEXT REFERENCES users(udid) ON DELETE SET NULL;

-- Add owner_udids to goals (TEXT array, nullable)
ALTER TABLE goals ADD COLUMN owner_udids TEXT[];

-- Backfill lead_udid: match teams.lead to users.display_name (unique matches only)
UPDATE teams t
SET lead_udid = u.udid
FROM (
    SELECT display_name, udid,
           COUNT(*) OVER (PARTITION BY display_name) AS cnt
    FROM users
    WHERE provider NOT IN ('system')
) u
WHERE t.lead = u.display_name
  AND u.cnt = 1;

-- Backfill owner_udids: resolve each comma-separated name in owner_text to a UDID (unique matches only)
WITH parsed AS (
    SELECT g.id                                          AS goal_id,
           trim(part)                                    AS owner_name
    FROM goals g,
         unnest(string_to_array(g.owner_text, ',')) AS part
    WHERE g.owner_text != ''
),
resolved AS (
    SELECT p.goal_id, u.udid
    FROM parsed p
    JOIN users u ON u.display_name = p.owner_name
                AND u.provider NOT IN ('system')
    WHERE (
        SELECT COUNT(*) FROM users u2
        WHERE u2.display_name = p.owner_name
          AND u2.provider NOT IN ('system')
    ) = 1
),
aggregated AS (
    SELECT goal_id, array_agg(udid ORDER BY udid) AS owner_udids
    FROM resolved
    GROUP BY goal_id
)
UPDATE goals g
SET owner_udids = a.owner_udids
FROM aggregated a
WHERE g.id = a.goal_id;
```

Create `migrations/022_udid_linking.down.sql`:

```sql
ALTER TABLE goals DROP COLUMN IF EXISTS owner_udids;
ALTER TABLE teams DROP COLUMN IF EXISTS lead_udid;
```

- [ ] **Step 2: Run migration**

```bash
make migrate-up
# or: go run ./cmd/migrate up
```

Expected: no errors, `\d teams` shows `lead_udid TEXT`, `\d goals` shows `owner_udids TEXT[]`.

- [ ] **Step 3: Commit**

```bash
git add migrations/022_udid_linking.up.sql migrations/022_udid_linking.down.sql
git commit -m "feat: add lead_udid to teams and owner_udids to goals with backfill migration"
```

---

## Task 2: Domain Model

**Files:**
- Modify: `internal/domain/models.go`

- [ ] **Step 1: Update Team struct**

In `internal/domain/models.go`, change the Team struct from:
```go
type Team struct {
	ID          int64
	Name        string
	Type        TeamType
	ParentID    *int64
	Lead        string
	Description string
	DeletedAt   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
```
to:
```go
type Team struct {
	ID          int64
	Name        string
	Type        TeamType
	ParentID    *int64
	Lead        string
	LeadUDID    *string
	Description string
	DeletedAt   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
```

- [ ] **Step 2: Update Goal struct**

Change the Goal struct, adding `OwnerUDIDs` after `OwnerText`:
```go
type Goal struct {
	ID          int64
	TeamID      int64
	PeriodID    int64
	Title       string
	Description string
	Priority    Priority
	Weight      int
	WorkType    WorkType
	FocusType   FocusType
	OwnerText   string
	OwnerUDIDs  []string
	Progress    int
	CreatedAt   time.Time
	UpdatedAt   time.Time
	KeyResults  []KeyResult
	Comments    []GoalComment
}
```

- [ ] **Step 3: Build to verify**

```bash
go build ./...
```

Expected: compile errors in store files — that's expected, we fix them next.

- [ ] **Step 4: Commit**

```bash
git add internal/domain/models.go
git commit -m "feat: add LeadUDID to Team and OwnerUDIDs to Goal domain types"
```

---

## Task 3: Store — Teams

**Files:**
- Modify: `internal/store/teams/teams.go`
- Test: `internal/store/teams/teams_test.go`

- [ ] **Step 1: Write failing test**

In `internal/store/teams/teams_test.go`, add after the existing `TestTeamsCRUD`:

```go
func TestTeamLeadUDID(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	// Create user and team using that user's UDID as lead
	ur := users.NewUserRepository(testDB(t))
	u, err := ur.UpsertUser(ctx, users.UpsertUserInput{
		ProviderSubjectKey: "test:lead-udid-user",
		Provider: "test", Subject: "lead-udid-user",
		DisplayName: "Lead User", AvatarURL: "", Email: "",
	})
	if err != nil {
		t.Fatalf("upsert user: %v", err)
	}

	id, err := r.CreateTeam(ctx, TeamInput{
		Name: "UDID Team", Type: domain.TeamTypeTeam,
		Lead: "Lead User", LeadUDID: &u.UDID,
	})
	if err != nil {
		t.Fatalf("create team: %v", err)
	}

	team, err := r.GetTeam(ctx, id)
	if err != nil {
		t.Fatalf("get team: %v", err)
	}
	if team.LeadUDID == nil || *team.LeadUDID != u.UDID {
		t.Errorf("LeadUDID: got %v, want %q", team.LeadUDID, u.UDID)
	}

	// Update: clear lead_udid
	if err := r.UpdateTeam(ctx, TeamInput{
		Name: "UDID Team", Type: domain.TeamTypeTeam, Lead: "Lead User", LeadUDID: nil,
	}, id); err != nil {
		t.Fatalf("update team: %v", err)
	}
	team2, _ := r.GetTeam(ctx, id)
	if team2.LeadUDID != nil {
		t.Errorf("expected nil LeadUDID after clear, got %v", team2.LeadUDID)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/store/teams/... -run TestTeamLeadUDID -v
```

Expected: compile error (LeadUDID not in TeamInput yet).

- [ ] **Step 3: Update TeamInput and SQL**

In `internal/store/teams/teams.go`:

a) Add `LeadUDID *string` to `TeamInput`:
```go
type TeamInput struct {
	Name        string
	Type        domain.TeamType
	ParentID    *int64
	Lead        string
	LeadUDID    *string
	Description string
}
```

b) Update `CreateTeam`:
```go
func (r *TeamRepository) CreateTeam(ctx context.Context, input TeamInput) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx,
		`INSERT INTO teams (name, team_type, parent_id, lead, lead_udid, description) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		input.Name, input.Type, input.ParentID, input.Lead, input.LeadUDID, input.Description).Scan(&id)
	return id, err
}
```

c) Update `UpdateTeam`:
```go
func (r *TeamRepository) UpdateTeam(ctx context.Context, input TeamInput, id int64) error {
	_, err := r.db.Exec(ctx,
		`UPDATE teams SET name=$1, team_type=$2, parent_id=$3, lead=$4, lead_udid=$5, description=$6, updated_at=NOW() WHERE id=$7`,
		input.Name, input.Type, input.ParentID, input.Lead, input.LeadUDID, input.Description, id)
	return err
}
```

d) Update all SELECT queries to include `lead_udid`. Change every occurrence of:
```sql
SELECT id, name, team_type, parent_id, lead, description, deleted_at, created_at, updated_at
FROM teams
```
to:
```sql
SELECT id, name, team_type, parent_id, lead, lead_udid, description, deleted_at, created_at, updated_at
FROM teams
```

e) Update `scanTeams` to scan `lead_udid`:
```go
func scanTeams(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close()
}) ([]domain.Team, error) {
	var teams []domain.Team
	for rows.Next() {
		var team domain.Team
		var parentID sql.NullInt64
		var leadUDID sql.NullString
		var deletedAt sql.NullTime
		if err := rows.Scan(&team.ID, &team.Name, &team.Type, &parentID, &team.Lead, &leadUDID, &team.Description, &deletedAt, &team.CreatedAt, &team.UpdatedAt); err != nil {
			return nil, err
		}
		if parentID.Valid {
			value := parentID.Int64
			team.ParentID = &value
		}
		if leadUDID.Valid {
			team.LeadUDID = &leadUDID.String
		}
		if deletedAt.Valid {
			value := deletedAt.Time
			team.DeletedAt = &value
		}
		teams = append(teams, team)
	}
	return teams, rows.Err()
}
```

f) Update `GetTeam` scan to match the new SELECT (7 base columns + lead_udid):
```go
func (r *TeamRepository) GetTeam(ctx context.Context, id int64) (domain.Team, error) {
	var team domain.Team
	var parentID sql.NullInt64
	var leadUDID sql.NullString
	var deletedAt sql.NullTime
	row := r.db.QueryRow(ctx,
		`SELECT id, name, team_type, parent_id, lead, lead_udid, description, deleted_at, created_at, updated_at
		 FROM teams WHERE id=$1`, id)
	if err := row.Scan(&team.ID, &team.Name, &team.Type, &parentID, &team.Lead, &leadUDID, &team.Description, &deletedAt, &team.CreatedAt, &team.UpdatedAt); err != nil {
		return domain.Team{}, err
	}
	if parentID.Valid {
		value := parentID.Int64
		team.ParentID = &value
	}
	if leadUDID.Valid {
		team.LeadUDID = &leadUDID.String
	}
	if deletedAt.Valid {
		value := deletedAt.Time
		team.DeletedAt = &value
	}
	return team, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/store/teams/... -run TestTeamLeadUDID -v
```

Expected: PASS.

- [ ] **Step 5: Run all team tests**

```bash
go test ./internal/store/teams/... -v
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/store/teams/teams.go internal/store/teams/teams_test.go
git commit -m "feat: add lead_udid read/write to TeamRepository"
```

---

## Task 4: Store — Goals

**Files:**
- Modify: `internal/store/goals/goals.go`

- [ ] **Step 1: Update GoalInput, GoalUpdateInput, GoalFieldsUpdateInput**

In `internal/store/goals/goals.go`, add `OwnerUDIDs []string` to all three input structs:

```go
type GoalInput struct {
	TeamID      int64
	PeriodID    int64
	Title       string
	Description string
	Priority    domain.Priority
	Weight      int
	WorkType    domain.WorkType
	FocusType   domain.FocusType
	OwnerText   string
	OwnerUDIDs  []string
}

type GoalUpdateInput struct {
	ID          int64
	Title       string
	Description string
	Priority    domain.Priority
	Weight      int
	WorkType    domain.WorkType
	FocusType   domain.FocusType
	OwnerText   string
	OwnerUDIDs  []string
}

type GoalFieldsUpdateInput struct {
	ID          int64
	Title       string
	Description string
	Priority    domain.Priority
	WorkType    domain.WorkType
	FocusType   domain.FocusType
	OwnerText   string
	OwnerUDIDs  []string
}
```

- [ ] **Step 2: Update CreateGoal**

Change the INSERT to include `owner_udids`:

```go
func (r *GoalRepository) CreateGoal(ctx context.Context, input GoalInput) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx, `
		INSERT INTO goals (team_id, period_id, title, description, priority, weight, work_type, focus_type, owner_text, owner_udids, sort_order)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,
		  COALESCE((SELECT MAX(sort_order)+1 FROM goals WHERE team_id=$1 AND period_id=$2), 0))
		RETURNING id`,
		input.TeamID, input.PeriodID, input.Title, input.Description, input.Priority, input.Weight,
		input.WorkType, input.FocusType, input.OwnerText, input.OwnerUDIDs,
	).Scan(&id)
	return id, err
}
```

- [ ] **Step 3: Update UpdateGoal**

```go
func (r *GoalRepository) UpdateGoal(ctx context.Context, input GoalUpdateInput) error {
	if input.Weight != 0 {
		_, err := r.db.Exec(ctx, `
			UPDATE goals SET title=$1, description=$2, priority=$3, weight=$4, work_type=$5, focus_type=$6,
			                 owner_text=$7, owner_udids=$8, updated_at=NOW()
			WHERE id=$9`,
			input.Title, input.Description, input.Priority, input.Weight, input.WorkType, input.FocusType,
			input.OwnerText, input.OwnerUDIDs, input.ID)
		return err
	}
	_, err := r.db.Exec(ctx, `
		UPDATE goals SET title=$1, description=$2, priority=$3, work_type=$4, focus_type=$5,
		                 owner_text=$6, owner_udids=$7, updated_at=NOW()
		WHERE id=$8`,
		input.Title, input.Description, input.Priority, input.WorkType, input.FocusType,
		input.OwnerText, input.OwnerUDIDs, input.ID)
	return err
}
```

- [ ] **Step 4: Update UpdateGoalFields**

```go
func (r *GoalRepository) UpdateGoalFields(ctx context.Context, input GoalFieldsUpdateInput) error {
	_, err := r.db.Exec(ctx, `
		UPDATE goals SET title=$1, description=$2, priority=$3, work_type=$4, focus_type=$5,
		                 owner_text=$6, owner_udids=$7, updated_at=NOW()
		WHERE id=$8`,
		input.Title, input.Description, input.Priority, input.WorkType, input.FocusType,
		input.OwnerText, input.OwnerUDIDs, input.ID)
	return err
}
```

- [ ] **Step 5: Update all SELECT queries to include owner_udids**

Find every `SELECT` in goals.go that lists goal columns. Each one like:
```sql
SELECT id, team_id, period_id, title, description, priority, weight, work_type, focus_type, owner_text, created_at, updated_at
```
becomes:
```sql
SELECT id, team_id, period_id, title, description, priority, weight, work_type, focus_type, owner_text, owner_udids, created_at, updated_at
```

There are four such SELECT statements (around lines 86, 354, 417, 788). Update all of them.

- [ ] **Step 6: Update all Scan calls to include &goal.OwnerUDIDs**

For every `rows.Scan` or `row.Scan` that reads goal columns, add `&goal.OwnerUDIDs` after `&goal.OwnerText`. For example, the scan at line ~120:
```go
if err := rows.Scan(
    &goal.ID, &goal.TeamID, &goal.PeriodID,
    &goal.Title, &goal.Description,
    &goal.Priority, &goal.Weight, &goal.WorkType, &goal.FocusType,
    &goal.OwnerText, &goal.OwnerUDIDs,
    &goal.CreatedAt, &goal.UpdatedAt,
); err != nil {
```

Repeat for all Scan calls that read the goal row. pgx v5 scans `TEXT[]` directly into `[]string`.

- [ ] **Step 7: Build to verify**

```bash
go build ./...
```

Expected: clean compile (callers still pass empty OwnerUDIDs, which is fine).

- [ ] **Step 8: Run goal store tests**

```bash
go test ./internal/store/goals/... -v
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/store/goals/goals.go
git commit -m "feat: add owner_udids read/write to GoalRepository"
```

---

## Task 5: Store — Users

**Files:**
- Modify: `internal/store/users/users.go`
- Test: `internal/store/users/users_test.go`

- [ ] **Step 1: Write failing test for SearchUsersInSet with leadUDIDs**

In `internal/store/users/users_test.go`, update `TestSearchUsersInSet`:

```go
func TestSearchUsersInSet(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	u1, _ := r.UpsertUser(ctx, UpsertUserInput{
		ProviderSubjectKey: "test:set1", Provider: "test", Subject: "set1",
		DisplayName: "Set User One",
	})
	u2, _ := r.UpsertUser(ctx, UpsertUserInput{
		ProviderSubjectKey: "test:set2", Provider: "test", Subject: "set2",
		DisplayName: "Set User Two",
	})

	// find by id
	results, err := r.SearchUsersInSet(ctx, []int64{u1.ID}, nil, "", 10)
	if err != nil {
		t.Fatalf("SearchUsersInSet: %v", err)
	}
	if len(results) != 1 || results[0].ID != u1.ID {
		t.Errorf("expected u1, got %v", results)
	}

	// find by lead UDID
	results2, err := r.SearchUsersInSet(ctx, nil, []string{u2.UDID}, "", 10)
	if err != nil {
		t.Fatalf("SearchUsersInSet by UDID: %v", err)
	}
	if len(results2) != 1 || results2[0].ID != u2.ID {
		t.Errorf("expected u2, got %v", results2)
	}

	// empty → nil
	none, err := r.SearchUsersInSet(ctx, nil, nil, "", 10)
	if err != nil {
		t.Fatalf("SearchUsersInSet empty: %v", err)
	}
	if none != nil {
		t.Errorf("expected nil for empty set")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/store/users/... -run TestSearchUsersInSet -v
```

Expected: FAIL — function signature doesn't accept `[]string` as second parameter yet.

- [ ] **Step 3: Update SearchUsersInSet signature**

Change the method signature and SQL (replace `display_name = ANY($2)` with `udid = ANY($2)`):

```go
// SearchUsersInSet returns up to limit non-system users whose id is in userIDs OR whose
// udid is in leadUDIDs, filtered by optional text query q.
// Returns nil when both userIDs and leadUDIDs are empty.
func (r *UserRepository) SearchUsersInSet(ctx context.Context, userIDs []int64, leadUDIDs []string, q string, limit int) ([]*domain.User, error) {
	if limit <= 0 {
		limit = 20
	}
	if len(userIDs) == 0 && len(leadUDIDs) == 0 {
		return nil, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, udid, provider_subject_key, provider, subject, display_name, avatar_url, COALESCE(email,''), attributes_json, is_admin, created_at, updated_at, last_login_at
		FROM users
		WHERE provider NOT IN ('system')
		AND (id = ANY($1) OR udid = ANY($2))
		AND ($3 = '' OR LOWER(display_name) LIKE '%' || LOWER($3) || '%' OR LOWER(COALESCE(email,'')) LIKE '%' || LOWER($3) || '%')
		ORDER BY last_login_at DESC NULLS LAST
		LIMIT $4`, userIDs, leadUDIDs, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanUsersRows(rows)
}
```

- [ ] **Step 4: Update ListUserLeadTeams to use lead_udid**

Replace the existing `ListUserLeadTeams` with a UDID-keyed version:

```go
// ListUserLeadTeams returns a map of user UDID → team name for all active team leads.
func (r *UserRepository) ListUserLeadTeams(ctx context.Context) (map[string]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT lead_udid, name FROM teams
		WHERE deleted_at IS NULL AND lead_udid IS NOT NULL
		ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]string)
	for rows.Next() {
		var leadUDID, teamName string
		if err := rows.Scan(&leadUDID, &teamName); err != nil {
			return nil, err
		}
		if _, exists := result[leadUDID]; !exists {
			result[leadUDID] = teamName
		}
	}
	return result, rows.Err()
}
```

- [ ] **Step 5: Add ValidateUDIDsExist**

```go
// ValidateUDIDsExist returns UDIDs from the input slice that do NOT exist in users.
func (r *UserRepository) ValidateUDIDsExist(ctx context.Context, udids []string) ([]string, error) {
	if len(udids) == 0 {
		return nil, nil
	}
	rows, err := r.db.Query(ctx, `SELECT udid FROM users WHERE udid = ANY($1)`, udids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	found := make(map[string]struct{}, len(udids))
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		found[u] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var missing []string
	for _, u := range udids {
		if _, ok := found[u]; !ok {
			missing = append(missing, u)
		}
	}
	return missing, nil
}
```

- [ ] **Step 6: Run user store tests**

```bash
go test ./internal/store/users/... -v
```

Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/store/users/users.go internal/store/users/users_test.go
git commit -m "feat: update user store for UDID-based lead lookup and UDID validation"
```

---

## Task 6: Service — Health-Check Scope

**Files:**
- Modify: `internal/service/healthcheckin.go`
- Test: `internal/service/healthcheckin_test.go`

- [ ] **Step 1: Write failing tests**

Replace existing computeScope tests and add admin bypass test in `internal/service/healthcheckin_test.go`:

```go
func strPtr(s string) *string { return &s }

func makeTeamWithUDID(id int64, name string, leadUDID *string, parentID *int64) domain.Team {
	return domain.Team{ID: id, Name: name, LeadUDID: leadUDID, ParentID: parentID}
}

func makeGoalWithUDIDs(id, teamID int64, ownerUDIDs []string, krs []domain.KeyResult) domain.Goal {
	return domain.Goal{ID: id, TeamID: teamID, OwnerUDIDs: ownerUDIDs, KeyResults: krs, Weight: 100}
}

func TestComputeScope_LeadUDIDGetsSubtree(t *testing.T) {
	teams := []domain.Team{
		makeTeamWithUDID(1, "Root", strPtr("udid-alice"), nil),
		makeTeamWithUDID(2, "Child", nil, teamPtr(1)),
		makeTeamWithUDID(3, "Grandchild", nil, teamPtr(2)),
		makeTeamWithUDID(4, "Other", strPtr("udid-bob"), nil),
	}
	goals := map[int64][]domain.Goal{}
	ids := computeScope(teams, goals, "udid-alice")
	got := toSet(ids)
	if !got[1] || !got[2] || !got[3] {
		t.Errorf("expected IDs 1,2,3; got %v", ids)
	}
	if got[4] {
		t.Errorf("team 4 should not be in scope")
	}
}

func TestComputeScope_OwnerUDIDGetsOnlyOwnerTeam(t *testing.T) {
	teams := []domain.Team{
		makeTeamWithUDID(10, "Team A", nil, nil),
		makeTeamWithUDID(11, "Team B", nil, teamPtr(10)),
	}
	goals := map[int64][]domain.Goal{
		10: {makeGoalWithUDIDs(1, 10, []string{"udid-alice", "udid-bob"}, nil)},
	}
	ids := computeScope(teams, goals, "udid-alice")
	got := toSet(ids)
	if !got[10] {
		t.Errorf("expected team 10 in owner scope")
	}
	if got[11] {
		t.Errorf("team 11 should NOT be in owner scope")
	}
}

func TestComputeScope_EmptyWhenNoUDIDMatch(t *testing.T) {
	teams := []domain.Team{makeTeamWithUDID(1, "T", strPtr("udid-bob"), nil)}
	goals := map[int64][]domain.Goal{}
	if computeScope(teams, goals, "udid-alice") != nil {
		t.Error("expected nil scope for non-matching UDID")
	}
}

func TestComputeScope_EmptyOwnerUDIDsNoScope(t *testing.T) {
	teams := []domain.Team{makeTeamWithUDID(1, "T", nil, nil)}
	goals := map[int64][]domain.Goal{
		1: {makeGoalWithUDIDs(1, 1, nil, nil)},
	}
	if computeScope(teams, goals, "udid-alice") != nil {
		t.Error("expected nil scope when OwnerUDIDs is empty")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/service/... -run "TestComputeScope_LeadUDID|TestComputeScope_OwnerUDID|TestComputeScope_EmptyWhenNoUDID|TestComputeScope_EmptyOwnerUDIDs" -v
```

Expected: FAIL — `computeScope` still uses `displayName string`.

- [ ] **Step 3: Replace computeScope**

In `internal/service/healthcheckin.go`, replace the entire `computeScope` function:

```go
func computeScope(teams []domain.Team, goalsByTeam map[int64][]domain.Goal, userUDID string) []int64 {
	if userUDID == "" {
		return nil
	}

	childrenMap := make(map[int64][]int64, len(teams))
	for _, t := range teams {
		if t.ParentID != nil {
			childrenMap[*t.ParentID] = append(childrenMap[*t.ParentID], t.ID)
		}
	}

	scopeSet := make(map[int64]struct{})

	var addDescendants func(id int64)
	addDescendants = func(id int64) {
		if _, exists := scopeSet[id]; exists {
			return
		}
		scopeSet[id] = struct{}{}
		for _, childID := range childrenMap[id] {
			addDescendants(childID)
		}
	}

	for _, t := range teams {
		if t.DeletedAt == nil && t.LeadUDID != nil && *t.LeadUDID == userUDID {
			addDescendants(t.ID)
		}
	}

	for teamID, goals := range goalsByTeam {
		for _, g := range goals {
			for _, uid := range g.OwnerUDIDs {
				if uid == userUDID {
					scopeSet[teamID] = struct{}{}
					break
				}
			}
		}
	}

	if len(scopeSet) == 0 {
		return nil
	}
	result := make([]int64, 0, len(scopeSet))
	for id := range scopeSet {
		result = append(result, id)
	}
	return result
}
```

Also delete `ownerTextContains` and `splitOwners` functions — they are no longer used in scope computation. (They were only used by `computeScope`.)

- [ ] **Step 4: Update GetHealthCheckIn signature and add admin bypass**

Replace the `GetHealthCheckIn` method:

```go
func (s *Service) GetHealthCheckIn(ctx context.Context, userUDID string, isAdmin bool, periodID int64, cfg HealthCheckInConfig) (*HealthCheckInResult, error) {
	if s.hcCache == nil {
		return &HealthCheckInResult{HasScope: false}, nil
	}
	data, err := s.hcCache.Get(ctx, periodID)
	if err != nil {
		return nil, err
	}

	var scopeIDs []int64
	if isAdmin {
		scopeIDs = make([]int64, 0, len(data.Teams))
		for _, t := range data.Teams {
			scopeIDs = append(scopeIDs, t.ID)
		}
	} else {
		scopeIDs = computeScope(data.Teams, data.GoalsByTeam, userUDID)
		if scopeIDs == nil {
			return &HealthCheckInResult{HasScope: false, PeriodID: periodID, Categories: emptyCategories(cfg)}, nil
		}
	}
	return computeCategories(data, scopeIDs, cfg, time.Now()), nil
}
```

- [ ] **Step 5: Update serviceProvider interface in healthcheckin handler**

In `internal/http/handlers/api/v1/healthcheckin/handler.go`, update the interface:
```go
type serviceProvider interface {
	GetHealthCheckIn(ctx context.Context, userUDID string, isAdmin bool, periodID int64, cfg service.HealthCheckInConfig) (*service.HealthCheckInResult, error)
}
```

- [ ] **Step 6: Run tests**

```bash
go test ./internal/service/... -run "TestComputeScope" -v
```

Expected: all new tests PASS.

- [ ] **Step 7: Run all service tests**

```bash
go test ./internal/service/... -v
```

Expected: PASS (old `TestComputeScope_LeadGetsSubtree` etc. will now fail — remove them and keep the new UDID-based ones). Remove the old display-name tests from `healthcheckin_test.go`.

- [ ] **Step 8: Commit**

```bash
git add internal/service/healthcheckin.go internal/service/healthcheckin_test.go \
        internal/http/handlers/api/v1/healthcheckin/handler.go
git commit -m "feat: health-check scope uses UDID; admin sees all teams"
```

---

## Task 7: Service — User Search Scope

**Files:**
- Modify: `internal/service/service.go`

- [ ] **Step 1: Update SearchUsersInSet interface**

In `internal/service/service.go`, find the interface definition for `SearchUsersInSet` (around line 110) and update:
```go
SearchUsersInSet(ctx context.Context, userIDs []int64, leadUDIDs []string, q string, limit int) ([]*domain.User, error)
```

- [ ] **Step 2: Update SearchUsersInScope to collect leadUDIDs**

Find `SearchUsersInScope` (around line 968) and update it to collect `LeadUDID` instead of `Lead`:

```go
func (s *Service) SearchUsersInScope(ctx context.Context, scopeTeamIDs []int64, q string, limit int) ([]*domain.User, error) {
	if limit <= 0 {
		limit = 20
	}
	if len(scopeTeamIDs) == 0 {
		return s.users.SearchUsersUnrestricted(ctx, q, limit)
	}

	scopeSet := make(map[int64]struct{}, len(scopeTeamIDs))
	for _, id := range scopeTeamIDs {
		scopeSet[id] = struct{}{}
	}

	allTeams, err := s.teams.ListAllTeams(ctx)
	if err != nil {
		return nil, err
	}

	relatedSet := make(map[int64]struct{})
	// ... (keep existing related-set logic unchanged)

	allGrants, err := s.grants.AllGrants(ctx)
	if err != nil {
		return nil, err
	}

	eligibleIDs := make([]int64, 0)
	seen := make(map[int64]struct{})
	for userID, userGrants := range allGrants {
		for _, g := range userGrants {
			if _, ok := relatedSet[g.TeamID]; ok {
				if _, dup := seen[userID]; !dup {
					seen[userID] = struct{}{}
					eligibleIDs = append(eligibleIDs, userID)
				}
				break
			}
		}
	}

	leadUDIDs := make([]string, 0)
	for _, t := range allTeams {
		if _, ok := relatedSet[t.ID]; ok && t.LeadUDID != nil && t.DeletedAt == nil {
			leadUDIDs = append(leadUDIDs, *t.LeadUDID)
		}
	}

	return s.users.SearchUsersInSet(ctx, eligibleIDs, leadUDIDs, q, limit)
}
```

This is the **only change** needed in `SearchUsersInScope` — the `relatedSet` computation, `parentMap`, `childrenMap`, `eligibleIDs`/`seen` logic above is untouched. Only the last block (`leadNames` → `leadUDIDs`) changes.

- [ ] **Step 3: Add ValidateUserUDIDsExist to service**

Add to service.go:
```go
func (s *Service) ValidateUserUDIDsExist(ctx context.Context, udids []string) ([]string, error) {
	return s.users.ValidateUDIDsExist(ctx, udids)
}
```

Also add `ValidateUDIDsExist` to the user repo interface in service.go:
```go
ValidateUDIDsExist(ctx context.Context, udids []string) ([]string, error)
```

- [ ] **Step 4: Build to verify**

```bash
go build ./...
```

Expected: compile errors in handlers that still call `GetHealthCheckIn` with old signature — those will be fixed in the next tasks.

- [ ] **Step 5: Commit**

```bash
git add internal/service/service.go
git commit -m "feat: service user search uses lead UDIDs; add ValidateUserUDIDsExist"
```

---

## Task 8: API Helpers

**Files:**
- Modify: `internal/http/handlers/api/v1/helpers_response.go`
- Test: `internal/http/handlers/api/v1/helpers_response_test.go` (if exists)

- [ ] **Step 1: Update BuildUserRefMap to be UDID-keyed**

In `internal/http/handlers/api/v1/helpers_response.go`, replace `BuildUserRefMap`:

```go
// BuildUserRefMap builds a udid→UserRef lookup from a slice of users.
func BuildUserRefMap(users []*domain.User) map[string]*dto.UserRef {
	m := make(map[string]*dto.UserRef, len(users))
	for _, u := range users {
		if u.UDID == "" {
			continue
		}
		ref := &dto.UserRef{UDID: u.UDID, DisplayName: u.DisplayName, AvatarURL: u.AvatarURL}
		m[u.UDID] = ref
	}
	return m
}
```

- [ ] **Step 2: Add ResolveOwnersByUDIDs and ResolveLeadByUDID**

Add two new functions after `ResolveOwners` (keep `ResolveOwners` for the no-auth text fallback):

```go
// ResolveOwnersByUDIDs resolves owner_udids to UserRef list using the UDID-keyed refs map.
// For each UDID not found in refs, returns a placeholder.
// Falls back to ResolveOwners(ownerText, refs) when ownerUDIDs is empty (no-auth mode).
func ResolveOwnersByUDIDs(ownerUDIDs []string, ownerText string, refs map[string]*dto.UserRef) []dto.UserRef {
	if len(ownerUDIDs) == 0 {
		return ResolveOwners(ownerText, refs)
	}
	out := make([]dto.UserRef, 0, len(ownerUDIDs))
	for _, uid := range ownerUDIDs {
		if refs != nil {
			if ref, ok := refs[uid]; ok {
				out = append(out, *ref)
				continue
			}
		}
		out = append(out, dto.UserRef{UDID: uid, DisplayName: "Удалённый пользователь"})
	}
	return out
}

// ResolveLeadByUDID looks up a team lead by UDID in the UDID-keyed refs map.
// Returns nil when leadUDID is nil or the user is not found (e.g. deleted).
func ResolveLeadByUDID(leadUDID *string, refs map[string]*dto.UserRef) *dto.UserRef {
	if leadUDID == nil || *leadUDID == "" {
		return nil
	}
	if refs != nil {
		if ref, ok := refs[*leadUDID]; ok {
			return ref
		}
	}
	return nil
}
```

- [ ] **Step 3: Update MapGoalDetails to use ResolveOwnersByUDIDs**

In the `MapGoalDetails` function, change the `Owners` line:
```go
Owners: ResolveOwnersByUDIDs(goal.OwnerUDIDs, goal.OwnerText, userRefs),
```

- [ ] **Step 4: Build to verify**

```bash
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add internal/http/handlers/api/v1/helpers_response.go
git commit -m "feat: BuildUserRefMap keyed by UDID; add ResolveOwnersByUDIDs with deleted-user placeholder"
```

---

## Task 9: Health-Check Handler

**Files:**
- Modify: `internal/http/handlers/api/v1/healthcheckin/handler.go`

- [ ] **Step 1: Update HandleHealthCheckIn to pass UDID and IsAdmin**

Replace the call site in `HandleHealthCheckIn`:

```go
func (h *Handler) HandleHealthCheckIn(w http.ResponseWriter, r *http.Request) {
	periodIDStr := r.URL.Query().Get("period_id")
	periodID, err := strconv.ParseInt(periodIDStr, 10, 64)
	if err != nil || periodID <= 0 {
		writeError(w, http.StatusBadRequest, "period_id required")
		return
	}

	user := auth.UserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	cfg, err := service.LoadHealthCheckInConfig(r.Context(), h.settings)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return
	}

	result, err := h.svc.GetHealthCheckIn(r.Context(), user.UDID, user.IsAdmin, periodID, cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, result)
}
```

- [ ] **Step 2: Build**

```bash
go build ./...
```

Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add internal/http/handlers/api/v1/healthcheckin/handler.go
git commit -m "feat: health-check handler passes UDID and IsAdmin to service"
```

---

## Task 10: Users Handler

**Files:**
- Modify: `internal/http/handlers/api/v1/users/handler.go`

- [ ] **Step 1: Update led_team lookup to use UDID**

Change `leadTeams[u.DisplayName]` to `leadTeams[u.UDID]`:

```go
resp := make([]userResponse, 0, len(users))
for _, u := range users {
    item := userResponse{
        UDID:        u.UDID,
        DisplayName: u.DisplayName,
        AvatarURL:   u.AvatarURL,
        Provider:    u.Provider,
        Email:       u.Email,
    }
    if team, ok := leadTeams[u.UDID]; ok {
        item.LedTeam = team
    }
    resp = append(resp, item)
}
```

- [ ] **Step 2: Build**

```bash
go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add internal/http/handlers/api/v1/users/handler.go
git commit -m "feat: users handler resolves led_team by UDID"
```

---

## Task 11: Goals API — Read

**Files:**
- Modify: `internal/http/handlers/api/v1/goals/handler.go`
- Modify: `internal/http/handlers/api/v1/goals/response.go`

- [ ] **Step 1: Update HandleGoal to load users by UDIDs**

In `HandleGoal`, replace the display-name lookup:

```go
func (h *Handler) HandleGoal(w http.ResponseWriter, r *http.Request) {
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid goal id", map[string]string{"goal_id": "invalid"})
		return
	}
	goal, err := h.service.GetGoal(r.Context(), goalID)
	if err != nil {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	if !auth.CanAccessTeamFromCtx(r.Context(), goal.TeamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	var userRefs map[string]*dto.UserRef
	if len(goal.OwnerUDIDs) > 0 {
		users, _ := h.service.GetUsersByUDIDs(r.Context(), goal.OwnerUDIDs)
		userRefs = v1.BuildUserRefMap(users)
	}
	v1.WriteJSON(w, http.StatusOK, newGoalResponse(goal, userRefs))
}
```

- [ ] **Step 2: Update response.go to use ResolveOwnersByUDIDs**

In `internal/http/handlers/api/v1/goals/response.go`, change the `Owners` line:

```go
goalDetail := dto.GoalDetails{
    // ...
    Owners: v1.ResolveOwnersByUDIDs(goal.OwnerUDIDs, goal.OwnerText, userRefs),
    // ...
}
```

- [ ] **Step 3: Build**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/http/handlers/api/v1/goals/handler.go \
        internal/http/handlers/api/v1/goals/response.go
git commit -m "feat: goal read handler resolves owners by UDID"
```

---

## Task 12: Goals API — Write

**Files:**
- Modify: `internal/http/handlers/api/v1/goals/handler.go`

- [ ] **Step 1: Update HandleUpdateGoal to accept owner_udids**

In `HandleUpdateGoal`, find the request struct and add `OwnerUDIDs`:

```go
var req struct {
    Title       string   `json:"title"`
    Description string   `json:"description"`
    Priority    string   `json:"priority"`
    Weight      int      `json:"weight"`
    WorkType    string   `json:"work_type"`
    FocusType   string   `json:"focus_type"`
    OwnerUDIDs  []string `json:"owner_udids"`
}
```

After decoding `req`, add UDID validation before the service call:

```go
if len(req.OwnerUDIDs) > 0 {
    missing, err := h.service.ValidateUserUDIDsExist(r.Context(), req.OwnerUDIDs)
    if err != nil {
        v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to validate owners", nil)
        return
    }
    if len(missing) > 0 {
        v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "unknown owner UDIDs", map[string]string{"owner_udids": "unknown: " + strings.Join(missing, ", ")})
        return
    }
}
```

Pass `OwnerUDIDs` to the service call:
```go
if err := h.service.UpdateGoal(r.Context(), goals.GoalUpdateInput{
    ID:          goalID,
    Title:       req.Title,
    Description: req.Description,
    Priority:    priority,
    Weight:      goalWeight,
    WorkType:    workType,
    FocusType:   focusType,
    OwnerUDIDs:  req.OwnerUDIDs,
}); err != nil {
```

Also add `"strings"` to imports if not already present.

- [ ] **Step 2: Build**

```bash
go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add internal/http/handlers/api/v1/goals/handler.go
git commit -m "feat: goal update accepts owner_udids with UDID existence validation"
```

---

## Task 13: Teams API Handler

**Files:**
- Modify: `internal/http/handlers/api/v1/teams/handler.go`

- [ ] **Step 1: Replace collectOKRUserNames with collectOKRUserUDIDs**

```go
func collectOKRUserUDIDs(okr service.TeamOKR) []string {
	seen := make(map[string]struct{})
	if okr.Team.LeadUDID != nil {
		seen[*okr.Team.LeadUDID] = struct{}{}
	}
	for _, g := range okr.Goals {
		for _, uid := range g.Goal.OwnerUDIDs {
			seen[uid] = struct{}{}
		}
	}
	udids := make([]string, 0, len(seen))
	for uid := range seen {
		udids = append(udids, uid)
	}
	return udids
}

func collectOverviewUserUDIDs(overview service.TeamOverview) []string {
	seen := make(map[string]struct{})
	for _, item := range overview.ChildrenSummary {
		if item.Team.LeadUDID != nil {
			seen[*item.Team.LeadUDID] = struct{}{}
		}
	}
	udids := make([]string, 0, len(seen))
	for uid := range seen {
		udids = append(udids, uid)
	}
	return udids
}
```

- [ ] **Step 2: Update HandleTeamOKR and HandleTeamOverview to use UDIDs**

In `HandleTeamOKR`, change:
```go
udids := collectOKRUserUDIDs(okr)
users, _ := h.service.GetUsersByUDIDs(r.Context(), udids)
v1.WriteJSON(w, http.StatusOK, newTeamOKRResponse(okr, v1.BuildUserRefMap(users)))
```

In `HandleTeamOverview`, change:
```go
udids := collectOverviewUserUDIDs(overview)
users, _ := h.service.GetUsersByUDIDs(r.Context(), udids)
v1.WriteJSON(w, http.StatusOK, newTeamOverviewResponse(period, overview, v1.BuildUserRefMap(users)))
```

- [ ] **Step 3: Update HandleCreateGoal to accept owner_udids**

Find the request struct in `HandleCreateGoal` and change `OwnerText string` to `OwnerUDIDs []string`:

```go
var req struct {
    PeriodID    int64    `json:"period_id"`
    Title       string   `json:"title"`
    Description string   `json:"description"`
    Priority    string   `json:"priority"`
    Weight      int      `json:"weight"`
    WorkType    string   `json:"work_type"`
    FocusType   string   `json:"focus_type"`
    OwnerUDIDs  []string `json:"owner_udids"`
}
```

After decoding, add validation:
```go
if len(req.OwnerUDIDs) > 0 {
    missing, err := h.service.ValidateUserUDIDsExist(r.Context(), req.OwnerUDIDs)
    if err != nil {
        v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to validate owners", nil)
        return
    }
    if len(missing) > 0 {
        v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "unknown owner UDIDs", map[string]string{"owner_udids": "unknown: " + strings.Join(missing, ", ")})
        return
    }
}
```

Pass to service:
```go
goalID, err := h.service.CreateGoal(r.Context(), goals.GoalInput{
    TeamID:      teamID,
    PeriodID:    req.PeriodID,
    Title:       req.Title,
    Description: req.Description,
    Priority:    priority,
    Weight:      req.Weight,
    WorkType:    workType,
    FocusType:   focusType,
    OwnerUDIDs:  req.OwnerUDIDs,
})
```

Also: in the response builders (`newTeamOKRResponse`, check if it calls `ResolveOwners`) — find and update any call that uses `ResolveOwners` with `goal.OwnerText` to use `ResolveOwnersByUDIDs(goal.OwnerUDIDs, goal.OwnerText, refs)`.

- [ ] **Step 4: Update teams/response.go to use ResolveLeadByUDID**

In `internal/http/handlers/api/v1/teams/response.go`, change both calls to `v1.ResolveUserRef(…, userRefs)` that pass a display name string:

```go
// in newTeamOKRResponse:
Team: dto.TeamInfo{
    ID: data.Team.ID, Name: data.Team.Name,
    Type: string(data.Team.Type), TypeLabel: common.TeamTypeLabel(data.Team.Type),
    Lead:     v1.ResolveLeadByUDID(data.Team.LeadUDID, userRefs),
    ParentID: data.Team.ParentID,
},

// in mapTeamChildrenSummaryResponse:
Team: dto.TeamInfo{
    ID: item.Team.ID, Name: item.Team.Name,
    Type: string(item.Team.Type), TypeLabel: common.TeamTypeLabel(item.Team.Type),
    Lead:     v1.ResolveLeadByUDID(item.Team.LeadUDID, userRefs),
    ParentID: item.Team.ParentID,
},
```

- [ ] **Step 5: Build**

```bash
go build ./...
```

- [ ] **Step 6: Commit**

```bash
git add internal/http/handlers/api/v1/teams/handler.go \
        internal/http/handlers/api/v1/teams/response.go
git commit -m "feat: teams API handler uses UDID for user resolution and goal owner input"
```

---

## Task 14: Hierarchy Handler

**Files:**
- Modify: `internal/http/handlers/api/v1/hierarhy/handler.go`

- [ ] **Step 1: Replace collectLeadNames with collectLeadUDIDs**

```go
func collectLeadUDIDs(nodes []service.TeamNode) []string {
	seen := make(map[string]struct{})
	var walk func([]service.TeamNode)
	walk = func(ns []service.TeamNode) {
		for _, n := range ns {
			if n.Team.LeadUDID != nil {
				seen[*n.Team.LeadUDID] = struct{}{}
			}
			walk(n.Children)
		}
	}
	walk(nodes)
	udids := make([]string, 0, len(seen))
	for uid := range seen {
		udids = append(udids, uid)
	}
	return udids
}
```

- [ ] **Step 2: Update HandleHierarchy**

Change the user lookup:
```go
leadUDIDs := collectLeadUDIDs(nodes)
users, _ := h.service.GetUsersByUDIDs(r.Context(), leadUDIDs)
v1.WriteJSON(w, http.StatusOK, newHierarchyResponse(nodes, metrics, v1.BuildUserRefMap(users)))
```

- [ ] **Step 3: Update hierarhy/response.go to use ResolveLeadByUDID**

In `internal/http/handlers/api/v1/hierarhy/response.go`, change the lead resolution line:

```go
Lead: v1.ResolveLeadByUDID(node.Team.LeadUDID, userRefs),
```

- [ ] **Step 4: Build**

```bash
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add internal/http/handlers/api/v1/hierarhy/handler.go \
        internal/http/handlers/api/v1/hierarhy/response.go
git commit -m "feat: hierarchy handler resolves team leads by UDID"
```

---

## Task 15: Admin Teams Handler

**Files:**
- Modify: `internal/http/handlers/api/v1/admin/service_handler.go`

- [ ] **Step 1: Add LeadUDID to teamRow response**

In `teamRow` struct, add:
```go
type teamRow struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	TypeLabel   string  `json:"type_label"`
	ParentID    *int64  `json:"parent_id"`
	Lead        string  `json:"lead"`
	LeadUDID    *string `json:"lead_udid,omitempty"`
	Description string  `json:"description"`
	DeletedAt   *string `json:"deleted_at,omitempty"`
}
```

Update `mapTeamRow`:
```go
func mapTeamRow(t domain.Team) teamRow {
	var deletedAt *string
	if t.DeletedAt != nil {
		s := t.DeletedAt.Format("2006-01-02")
		deletedAt = &s
	}
	return teamRow{
		ID:          t.ID,
		Name:        t.Name,
		Type:        string(t.Type),
		TypeLabel:   common.TeamTypeLabel(t.Type),
		ParentID:    t.ParentID,
		Lead:        t.Lead,
		LeadUDID:    t.LeadUDID,
		Description: t.Description,
		DeletedAt:   deletedAt,
	}
}
```

- [ ] **Step 2: Update HandleCreateTeam to accept lead_udid**

Change the request struct:
```go
var req struct {
    Name        string  `json:"name"`
    Type        string  `json:"type"`
    ParentID    *int64  `json:"parent_id"`
    Lead        string  `json:"lead"`
    LeadUDID    *string `json:"lead_udid"`
    Description string  `json:"description"`
}
```

Add validation after decoding:
```go
if req.LeadUDID != nil && *req.LeadUDID != "" {
    missing, err := h.service.ValidateUserUDIDsExist(r.Context(), []string{*req.LeadUDID})
    if err != nil {
        v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to validate lead", nil)
        return
    }
    if len(missing) > 0 {
        v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "unknown lead_udid", map[string]string{"lead_udid": "not found"})
        return
    }
}
```

Pass to service:
```go
id, err := h.service.CreateTeam(r.Context(), teams.TeamInput{
    Name: req.Name, Type: teamType, ParentID: req.ParentID,
    Lead: req.Lead, LeadUDID: req.LeadUDID, Description: req.Description,
})
```

- [ ] **Step 3: Update HandleUpdateTeam the same way**

Apply the same `lead_udid` field and validation to `HandleUpdateTeam`.

```go
var req struct {
    Name        string  `json:"name"`
    Type        string  `json:"type"`
    ParentID    *int64  `json:"parent_id"`
    Lead        string  `json:"lead"`
    LeadUDID    *string `json:"lead_udid"`
    Description string  `json:"description"`
}
// ... same validation as HandleCreateTeam ...
if err := h.service.UpdateTeam(r.Context(), teams.TeamInput{
    Name: req.Name, Type: teamType, ParentID: req.ParentID,
    Lead: req.Lead, LeadUDID: req.LeadUDID, Description: req.Description,
}, teamID); err != nil {
```

- [ ] **Step 4: Build**

```bash
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add internal/http/handlers/api/v1/admin/service_handler.go
git commit -m "feat: admin team create/update accepts lead_udid with validation"
```

---

## Task 16: Update Specs

**Files:**
- Modify: `specs/040-api-contract.md`
- Modify: `specs/020-domain-model.md`

- [ ] **Step 1: Update health-checkin scope in 040-api-contract.md**

Find the section that says:
```
Scope: сервер определяет по `user.display_name` из сессии:
- lead-scope: команды, где `teams.lead = display_name` + все потомки
- owner-scope: команды с целями, где `goal.owner_text` содержит `display_name` (word match)
```

Replace with:
```
Scope: сервер определяет по UDID пользователя из сессии.
- Администраторы (включая режим `AUTH_MODE=disabled`) видят все команды без scope-фильтрации.
- Для обычных пользователей: lead-scope: команды, где `teams.lead_udid = user.udid` + все потомки; owner-scope: команды с целями, где `user.udid = ANY(goal.owner_udids)`.
```

Also update the goal create/update contract section for `owner_udids`:
- `POST /api/v1/teams/{teamID}/goals` — принимает `owner_udids: ["uuid1","uuid2"]` вместо `owner_text`
- `POST /api/v1/goals/{goalID}` — принимает `owner_udids: ["uuid1","uuid2"]` вместо `owner_text`
- Validation: все UDIDs должны существовать в таблице users → `400 VALIDATION_ERROR` иначе

Update team admin endpoints:
- `POST /api/v1/admin/teams` — принимает `lead_udid: "uuid"` (опционально); `lead` для display сохраняется как есть
- `PATCH /api/v1/admin/teams/{teamID}` — то же самое

- [ ] **Step 2: Update 020-domain-model.md**

In the `Team` section, add after `lead`:
```
- lead_udid (nullable, FK → users.udid) — UDID пользователя-руководителя; NULL в режиме без авторизации
```

In the `Goal` section, add after `owner_text`:
```
- owner_udids (TEXT[], nullable) — массив UDID владельцев цели; пустой в режиме без авторизации
```

- [ ] **Step 3: Commit**

```bash
git add specs/040-api-contract.md specs/020-domain-model.md
git commit -m "docs: update specs for UDID-based user linking"
```

---

## Final Verification

- [ ] **Run full test suite**

```bash
go test ./... -v 2>&1 | tail -30
```

Expected: all PASS, no compilation errors.

- [ ] **Build production binary**

```bash
go build ./cmd/...
```

Expected: clean build.
