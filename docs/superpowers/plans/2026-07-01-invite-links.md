# Invite Links Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn tenant invitations into generic, reusable invite links (no email binding, one-time or multi-use) that a tenant-admin creates in `/admin?section=users`, and that add an already-authenticated visitor to the tenant immediately.

**Architecture:** Migration `034` makes `tenant_invitations.email` nullable and adds `max_uses`/`use_count`. The repository gains an atomic `Consume` (single UPDATE guarded by `use_count < max_uses`) that replaces the `GetPendingByTokenHash`+`MarkClaimed` pair in the claim path, plus `Revoke`. `OnboardingService.ClaimInvitation` switches to `Consume`. `authhandler.HandleInvite` claims immediately when a session exists. The admin API drops email and accepts `max_uses`; a React panel in the users section creates/lists/revokes links.

**Tech Stack:** Go, pgx/v5, chi router, golang-migrate, testcontainers-go (`testutil.SetupDB`), React 18 UMD + Babel standalone (served from `internal/web/static/`).

## Global Constraints

- Specs are source of truth: `README-specs.md`, `specs/010`–`specs/050`; update `specs/040-api-contract.md` in this change set, do not touch unrelated specs.
- Follow clean/layered architecture; do not leak abstractions across layers (domain ← store ← service ← http).
- Keep tests current and cover new functionality; keep `seed_demo` current if table structure changes.
- Do NOT make git commits (the user commits). The `git commit` steps below are written per the skill template but MUST be skipped — stop at `git add` and hand back for the user to commit.
- No mention of AI/Claude/assistants in code, comments, commit messages, or docs.
- Tokens are stored hashed (sha256 hex via `service.HashInviteToken`); the raw token lives only in the delivered link.
- Roles are `domain.RoleUser` (`"user"`) / `domain.RoleAdmin` (`"admin"`); membership status `domain.MembershipActive` (`"active"`).
- Latest applied migration is `033`; this feature adds `034`.

---

### Task 1: Migration 034 — nullable email, max_uses, use_count

**Files:**
- Create: `migrations/034_invite_links.up.sql`
- Create: `migrations/034_invite_links.down.sql`

**Interfaces:**
- Consumes: existing `tenant_invitations` table from `031` (`email TEXT NOT NULL`, `status CHECK (pending|claimed|revoked)`).
- Produces: `tenant_invitations.email` nullable; new columns `max_uses INT NULL`, `use_count INT NOT NULL DEFAULT 0`; existing rows backfilled `max_uses = 1`.

- [ ] **Step 1: Write the up migration**

Create `migrations/034_invite_links.up.sql`:

```sql
-- Invite links: generic (no email), one-time or multi-use.
ALTER TABLE tenant_invitations ALTER COLUMN email DROP NOT NULL;
ALTER TABLE tenant_invitations ADD COLUMN max_uses INT;
ALTER TABLE tenant_invitations ADD COLUMN use_count INT NOT NULL DEFAULT 0;

-- Existing invitations were single-use; preserve that semantic.
UPDATE tenant_invitations SET max_uses = 1 WHERE max_uses IS NULL;
```

- [ ] **Step 2: Write the down migration**

Create `migrations/034_invite_links.down.sql`. Restore `email NOT NULL` — backfill NULLs to `''` first so the constraint can be re-added:

```sql
UPDATE tenant_invitations SET email = '' WHERE email IS NULL;
ALTER TABLE tenant_invitations ALTER COLUMN email SET NOT NULL;
ALTER TABLE tenant_invitations DROP COLUMN use_count;
ALTER TABLE tenant_invitations DROP COLUMN max_uses;
```

- [ ] **Step 3: Verify the migration applies and reverts against a scratch database**

Start a scratch Postgres and run migrate up then down then up. Run:

```bash
docker run -d --rm --name okrs_mig -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=okrs -p 55432:5432 postgres:16-alpine
until docker exec okrs_mig pg_isready -U postgres >/dev/null 2>&1; do sleep 1; done
DBURL='postgres://postgres:postgres@localhost:55432/okrs?sslmode=disable'
go run -tags '' github.com/golang-migrate/migrate/v4/cmd/migrate -path migrations -database "$DBURL" up
go run github.com/golang-migrate/migrate/v4/cmd/migrate -path migrations -database "$DBURL" down 1
go run github.com/golang-migrate/migrate/v4/cmd/migrate -path migrations -database "$DBURL" up
docker exec okrs_mig psql -U postgres -d okrs -c '\d tenant_invitations'
docker stop okrs_mig
```

Expected: final `\d tenant_invitations` shows `email` nullable, `max_uses integer`, `use_count integer not null default 0`; no migrate errors.

> If `github.com/golang-migrate/migrate/v4/cmd/migrate` is not runnable via `go run` in this repo, apply the two SQL files manually with `docker exec -i okrs_mig psql -U postgres -d okrs < migrations/034_invite_links.up.sql` (then the `.down.sql`, then `.up.sql` again) and inspect with the same `\d` command.

- [ ] **Step 4: Commit**

```bash
git add migrations/034_invite_links.up.sql migrations/034_invite_links.down.sql
# Do NOT run git commit — hand back to the user.
```

---

### Task 2: Domain fields + repository (Create without email, Consume, Revoke)

**Files:**
- Modify: `internal/domain/tenant.go` (the `Invitation` struct)
- Modify: `internal/store/invitations/invitations.go`
- Test: `internal/store/invitations/invitations_test.go`

**Interfaces:**
- Consumes: migration `034` columns; `domain.Role`, `domain.TenantScope`, `domain.InvitationPending`.
- Produces:
  - `domain.Invitation.Email *string`, `domain.Invitation.MaxUses *int`, `domain.Invitation.UseCount int`.
  - `type ClaimResult struct { TenantID int64; Role domain.Role }` in package `invitations`.
  - `(*InvitationRepository) Create(ctx context.Context, scope domain.TenantScope, role domain.Role, tokenHash string, createdBy int64, maxUses *int, expiresAt *time.Time) (*domain.Invitation, error)` — writes `email = NULL`.
  - `(*InvitationRepository) Consume(ctx context.Context, tokenHash string) (*ClaimResult, error)` — atomic; `ErrNotFound` when invalid/exhausted/expired/revoked.
  - `(*InvitationRepository) Revoke(ctx context.Context, scope domain.TenantScope, id int64) error` — idempotent (nil even on 0 rows), tenant-scoped.
  - `(*InvitationRepository) ListPendingByTenant(ctx context.Context, scope domain.TenantScope) ([]domain.Invitation, error)` — now also populates `MaxUses`, `UseCount`, `Email`.
  - `GetPendingByTokenHash` and `MarkClaimed` are removed (replaced by `Consume`).

- [ ] **Step 1: Update the domain struct**

In `internal/domain/tenant.go`, replace the `Invitation` struct with:

```go
type Invitation struct {
	ID              int64
	TenantID        int64
	Email           *string // nullable — generic links store NULL
	Role            Role
	Status          InvitationStatus
	CreatedByUserID *int64
	CreatedAt       time.Time
	ExpiresAt       *time.Time
	MaxUses         *int // nil = unlimited, 1 = one-time, N = up to N uses
	UseCount        int
}
```

- [ ] **Step 2: Write failing repository tests**

Replace the entire body of `internal/store/invitations/invitations_test.go` with:

```go
package invitations_test

import (
	"context"
	"testing"

	"okrs/internal/domain"
	"okrs/internal/store/invitations"
	"okrs/internal/store/testutil"
)

func intp(n int) *int { return &n }

func TestConsumeOneTime(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := invitations.NewInvitationRepository(pool)
	scope := domain.TenantScope{TenantID: 1}

	inv, err := repo.Create(ctx, scope, domain.RoleUser, "hash-one", 1, intp(1), nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if inv.Status != domain.InvitationPending || inv.Email != nil || inv.MaxUses == nil || *inv.MaxUses != 1 {
		t.Fatalf("unexpected invitation: %+v", inv)
	}

	res, err := repo.Consume(ctx, "hash-one")
	if err != nil {
		t.Fatalf("first consume: %v", err)
	}
	if res.TenantID != 1 || res.Role != domain.RoleUser {
		t.Fatalf("claim result = %+v", res)
	}
	if _, err := repo.Consume(ctx, "hash-one"); err != invitations.ErrNotFound {
		t.Fatalf("second consume of one-time link must fail, got %v", err)
	}
}

func TestConsumeLimited(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := invitations.NewInvitationRepository(pool)
	scope := domain.TenantScope{TenantID: 1}

	if _, err := repo.Create(ctx, scope, domain.RoleUser, "hash-lim", 1, intp(2), nil); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := repo.Consume(ctx, "hash-lim"); err != nil {
		t.Fatalf("consume 1: %v", err)
	}
	if _, err := repo.Consume(ctx, "hash-lim"); err != nil {
		t.Fatalf("consume 2: %v", err)
	}
	if _, err := repo.Consume(ctx, "hash-lim"); err != invitations.ErrNotFound {
		t.Fatalf("consume 3 must fail (limit 2), got %v", err)
	}
}

func TestConsumeUnlimited(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := invitations.NewInvitationRepository(pool)
	scope := domain.TenantScope{TenantID: 1}

	if _, err := repo.Create(ctx, scope, domain.RoleAdmin, "hash-unl", 1, nil, nil); err != nil {
		t.Fatalf("create: %v", err)
	}
	for i := 0; i < 5; i++ {
		res, err := repo.Consume(ctx, "hash-unl")
		if err != nil {
			t.Fatalf("consume %d: %v", i, err)
		}
		if res.Role != domain.RoleAdmin {
			t.Fatalf("role = %q", res.Role)
		}
	}
}

func TestConsumeUnknown(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := invitations.NewInvitationRepository(pool)
	if _, err := repo.Consume(ctx, "nope"); err != invitations.ErrNotFound {
		t.Fatalf("unknown token → ErrNotFound, got %v", err)
	}
}

func TestRevokeThenConsume(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := invitations.NewInvitationRepository(pool)
	scope := domain.TenantScope{TenantID: 1}

	inv, err := repo.Create(ctx, scope, domain.RoleUser, "hash-rev", 1, nil, nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.Revoke(ctx, scope, inv.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := repo.Consume(ctx, "hash-rev"); err != invitations.ErrNotFound {
		t.Fatalf("revoked link must not consume, got %v", err)
	}
	// Revoking again / a non-existent id is idempotent.
	if err := repo.Revoke(ctx, scope, inv.ID); err != nil {
		t.Fatalf("re-revoke should be no-op nil, got %v", err)
	}
	if err := repo.Revoke(ctx, scope, 999999); err != nil {
		t.Fatalf("revoke missing id should be no-op nil, got %v", err)
	}
	// Revoke is tenant-scoped: another tenant cannot revoke this row (no error, no effect).
	if err := repo.Revoke(ctx, domain.TenantScope{TenantID: 2}, inv.ID); err != nil {
		t.Fatalf("cross-tenant revoke should be no-op nil, got %v", err)
	}
}

func TestListPendingByTenantFields(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := invitations.NewInvitationRepository(pool)
	scope := domain.TenantScope{TenantID: 1}

	if _, err := repo.Create(ctx, scope, domain.RoleUser, "hash-list", 1, intp(3), nil); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := repo.Consume(ctx, "hash-list"); err != nil {
		t.Fatalf("consume: %v", err)
	}
	list, err := repo.ListPendingByTenant(ctx, scope)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("want 1 pending, got %d", len(list))
	}
	got := list[0]
	if got.MaxUses == nil || *got.MaxUses != 3 || got.UseCount != 1 {
		t.Fatalf("list fields = %+v", got)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/store/invitations/...`
Expected: compile failure — `Create` signature mismatch, `Consume`/`Revoke`/`ClaimResult` undefined.

- [ ] **Step 4: Rewrite the repository**

Replace the body of `internal/store/invitations/invitations.go` (keep the package, imports, `ErrNotFound`, `InvitationRepository` type and `NewInvitationRepository`) with these methods. Remove `GetPendingByTokenHash` and `MarkClaimed`:

```go
// ClaimResult is the outcome of a successful Consume: which tenant/role the claimer joins.
type ClaimResult struct {
	TenantID int64
	Role     domain.Role
}

// Create inserts a generic pending invite link (no email). maxUses nil = unlimited,
// 1 = one-time, N = up to N uses. Only the token hash is stored.
func (r *InvitationRepository) Create(ctx context.Context, scope domain.TenantScope, role domain.Role, tokenHash string, createdBy int64, maxUses *int, expiresAt *time.Time) (*domain.Invitation, error) {
	var inv domain.Invitation
	err := r.db.QueryRow(ctx, `
		INSERT INTO tenant_invitations (tenant_id, email, role, token_hash, status, created_by_user_id, expires_at, max_uses, use_count)
		VALUES ($1, NULL, $2, $3, 'pending', $4, $5, $6, 0)
		RETURNING id, tenant_id, email, role, status, created_by_user_id, created_at, expires_at, max_uses, use_count`,
		scope.TenantID, role, tokenHash, createdBy, expiresAt, maxUses).
		Scan(&inv.ID, &inv.TenantID, &inv.Email, &inv.Role, &inv.Status, &inv.CreatedByUserID, &inv.CreatedAt, &inv.ExpiresAt, &inv.MaxUses, &inv.UseCount)
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

// Consume atomically redeems a pending invite link by token hash. A single UPDATE increments
// use_count and, for capped links, flips status to 'claimed' on the final use. The WHERE clause
// rejects revoked/expired/exhausted links, so concurrent callers cannot over-consume a cap.
// Returns ErrNotFound when nothing valid matched.
func (r *InvitationRepository) Consume(ctx context.Context, tokenHash string) (*ClaimResult, error) {
	var res ClaimResult
	err := r.db.QueryRow(ctx, `
		UPDATE tenant_invitations
		SET use_count = use_count + 1,
		    status = CASE WHEN max_uses IS NOT NULL AND use_count + 1 >= max_uses THEN 'claimed' ELSE status END
		WHERE token_hash = $1 AND status = 'pending'
		  AND (expires_at IS NULL OR expires_at > now())
		  AND (max_uses IS NULL OR use_count < max_uses)
		RETURNING tenant_id, role`,
		tokenHash).Scan(&res.TenantID, &res.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// Revoke marks a pending invite link revoked. Tenant-scoped and idempotent: a missing/foreign/
// already-revoked id affects zero rows and returns nil.
func (r *InvitationRepository) Revoke(ctx context.Context, scope domain.TenantScope, id int64) error {
	_, err := r.db.Exec(ctx,
		`UPDATE tenant_invitations SET status = 'revoked' WHERE id = $1 AND tenant_id = $2 AND status = 'pending'`,
		id, scope.TenantID)
	return err
}

func (r *InvitationRepository) ListPendingByTenant(ctx context.Context, scope domain.TenantScope) ([]domain.Invitation, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, tenant_id, email, role, status, created_by_user_id, created_at, expires_at, max_uses, use_count
		FROM tenant_invitations
		WHERE tenant_id = $1 AND status = 'pending' ORDER BY created_at DESC`, scope.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Invitation
	for rows.Next() {
		var inv domain.Invitation
		if err := rows.Scan(&inv.ID, &inv.TenantID, &inv.Email, &inv.Role, &inv.Status, &inv.CreatedByUserID, &inv.CreatedAt, &inv.ExpiresAt, &inv.MaxUses, &inv.UseCount); err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}
```

- [ ] **Step 5: Run the repository tests to verify they pass**

Run: `go test ./internal/store/invitations/...`
Expected: PASS (all `TestConsume*`, `TestRevokeThenConsume`, `TestListPendingByTenantFields`).

- [ ] **Step 6: Commit**

```bash
git add internal/domain/tenant.go internal/store/invitations/invitations.go internal/store/invitations/invitations_test.go
# Do NOT run git commit — hand back to the user.
```

---

### Task 3: Service — ClaimInvitation via Consume

**Files:**
- Modify: `internal/service/onboarding.go` (`ClaimInvitation`)
- Test: `internal/service/onboarding_test.go`

**Interfaces:**
- Consumes: `(*invitations.InvitationRepository) Consume(ctx, tokenHash) (*invitations.ClaimResult, error)`, `invitations.ErrNotFound`.
- Produces: `ClaimInvitation(ctx, rawToken, userID) (*domain.Membership, error)` unchanged signature; now upserts an active membership from the `ClaimResult` (idempotent for repeat same-user claims of a multi-use link); invalid/exhausted → `ErrInvalidInvitation`.

- [ ] **Step 1: Update the failing service test**

In `internal/service/onboarding_test.go`, replace `TestClaimInvitationSingleUseBindsToIdentity` (around line 164) with a version using the new `Create` signature (no email) plus a multi-use case. Replace the function with:

```go
func TestClaimInvitationSingleUseBindsToIdentity(t *testing.T) {
	svc, inv, _, ctx, uid := setupClaimTest(t)

	raw, hash, err := service.GenerateInviteToken()
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if _, err := inv.Create(ctx, domain.TenantScope{TenantID: 2}, domain.RoleAdmin, hash, 1, intp(1), nil); err != nil {
		t.Fatalf("seed invite: %v", err)
	}

	m, err := svc.ClaimInvitation(ctx, raw, uid)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if m.TenantID != 2 || m.Role != domain.RoleAdmin || m.Status != domain.MembershipActive {
		t.Fatalf("membership = %+v", m)
	}
	// One-time link: replay fails.
	if _, err := svc.ClaimInvitation(ctx, raw, uid); err != service.ErrInvalidInvitation {
		t.Fatalf("replay must be invalid, got %v", err)
	}
	// Unknown token is invalid.
	if _, err := svc.ClaimInvitation(ctx, "deadbeef", uid); err != service.ErrInvalidInvitation {
		t.Fatalf("unknown token must be invalid, got %v", err)
	}
}
```

> `setupClaimTest` and `intp` must exist in the test file. If the existing test built its `svc`/`inv`/`uid` inline, extract that setup into a `setupClaimTest(t) (*service.OnboardingService, *invitations.InvitationRepository, *memberships.MembershipRepository, context.Context, int64)` helper and add `func intp(n int) *int { return &n }`. Read the current lines 164–198 first and reuse the exact wiring already there (repos, seeded tenant #2, seeded user `uid`).

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/service/ -run TestClaimInvitation`
Expected: compile failure — `inv.Create` old signature / `intp` maybe undefined.

- [ ] **Step 3: Rewrite ClaimInvitation**

In `internal/service/onboarding.go`, replace the `ClaimInvitation` method body with:

```go
// ClaimInvitation redeems a pending invite link (atomic, cap-safe) and binds an active
// membership in the link's tenant to the current identity. Repeat claims of a multi-use link
// by an already-active member are idempotent (Upsert). Invalid/expired/revoked/exhausted →
// ErrInvalidInvitation.
func (s *OnboardingService) ClaimInvitation(ctx context.Context, rawToken string, userID int64) (*domain.Membership, error) {
	res, err := s.inv.Consume(ctx, HashInviteToken(rawToken))
	if errors.Is(err, invitations.ErrNotFound) {
		return nil, ErrInvalidInvitation
	}
	if err != nil {
		return nil, err
	}
	m, err := s.mem.Upsert(ctx, domain.Membership{
		UserID:   userID,
		TenantID: res.TenantID,
		Role:     res.Role,
		Status:   domain.MembershipActive,
	})
	if err != nil {
		return nil, err
	}
	s.memCache.InvalidateUser(userID)
	return m, nil
}
```

- [ ] **Step 4: Run the service tests to verify they pass**

Run: `go test ./internal/service/ -run TestClaimInvitation`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service/onboarding.go internal/service/onboarding_test.go
# Do NOT run git commit — hand back to the user.
```

---

### Task 4: Admin API — create (no email, max_uses), list, revoke

**Files:**
- Modify: `internal/http/handlers/api/v1/onboarding/handler.go`
- Test: `internal/http/handlers/api/v1/onboarding/handler_test.go`

**Interfaces:**
- Consumes: `(*invitations.InvitationRepository)` methods `Create(ctx, scope, role, tokenHash, createdBy, maxUses, expiresAt)`, `ListPendingByTenant`, `Revoke(ctx, scope, id)`; `service.GenerateInviteToken`; `auth.TenantScopeFromContext`, `auth.UserFromContext`.
- Produces:
  - `invitationStore` interface updated: `Create(ctx, scope, role domain.Role, tokenHash string, createdBy int64, maxUses *int, expiresAt *time.Time) (*domain.Invitation, error)`, `ListPendingByTenant(...)`, `Revoke(ctx, scope, id int64) error`.
  - `POST /api/v1/admin/invitations` body `{role?, max_uses?, expires_at?}` → `201 {token, url, role, max_uses}`.
  - `POST /api/v1/admin/invitations/{id}/revoke` → `204`.
  - `GET /api/v1/admin/invitations` list items `{id, role, status, max_uses, use_count, created_at, expires_at}` (no email).
  - New handler method `HandleRevokeInvitation(w, r)`.

- [ ] **Step 1: Write failing handler tests**

In `internal/http/handlers/api/v1/onboarding/handler_test.go`: (a) add the revoke route to `buildRouter`; (b) replace `TestCreateAndListInvitation`; (c) add revoke + validation tests.

In `buildRouter`, after the existing `r.Post("/api/v1/admin/invitations", ...)` line add:

```go
	r.Post("/api/v1/admin/invitations/{id}/revoke", h.HandleRevokeInvitation)
```

Replace `TestCreateAndListInvitation` and add new tests:

```go
func TestCreateInvitationLink(t *testing.T) {
	admin := &domain.User{ID: 1, DisplayName: "Admin"}
	r, _ := buildRouter(t, admin)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/admin/invitations",
		strings.NewReader(`{"role":"admin","max_uses":5}`)))
	if w.Code != http.StatusCreated {
		t.Fatalf("create: code %d (%s)", w.Code, w.Body.String())
	}
	var created struct {
		Token   string `json:"token"`
		URL     string `json:"url"`
		Role    string `json:"role"`
		MaxUses *int   `json:"max_uses"`
	}
	_ = json.NewDecoder(w.Body).Decode(&created)
	if created.Token == "" || !strings.HasSuffix(created.URL, "/invite/"+created.Token) {
		t.Fatalf("bad create body: %+v", created)
	}
	if created.Role != "admin" || created.MaxUses == nil || *created.MaxUses != 5 {
		t.Fatalf("bad create body: %+v", created)
	}

	// List returns the link with counts and no email.
	lw := httptest.NewRecorder()
	r.ServeHTTP(lw, httptest.NewRequest(http.MethodGet, "/api/v1/admin/invitations", nil))
	if lw.Code != http.StatusOK {
		t.Fatalf("list: code %d", lw.Code)
	}
	var list []map[string]any
	_ = json.NewDecoder(lw.Body).Decode(&list)
	if len(list) != 1 {
		t.Fatalf("want 1 link, got %d", len(list))
	}
	if _, hasEmail := list[0]["email"]; hasEmail {
		t.Fatalf("list must not expose email: %+v", list[0])
	}
	if list[0]["use_count"] == nil || list[0]["max_uses"] == nil {
		t.Fatalf("list must include counts: %+v", list[0])
	}
}

func TestCreateInvitationRejectsBadMaxUses(t *testing.T) {
	admin := &domain.User{ID: 1, DisplayName: "Admin"}
	r, _ := buildRouter(t, admin)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/admin/invitations",
		strings.NewReader(`{"role":"user","max_uses":0}`)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("max_uses=0 must be 400, got %d (%s)", w.Code, w.Body.String())
	}
}

func TestRevokeInvitation(t *testing.T) {
	admin := &domain.User{ID: 1, DisplayName: "Admin"}
	r, pool := buildRouter(t, admin)
	_ = pool

	cw := httptest.NewRecorder()
	r.ServeHTTP(cw, httptest.NewRequest(http.MethodPost, "/api/v1/admin/invitations",
		strings.NewReader(`{"role":"user"}`)))
	if cw.Code != http.StatusCreated {
		t.Fatalf("create: %d", cw.Code)
	}
	// One link exists → id is 1 in a fresh DB.
	rw := httptest.NewRecorder()
	r.ServeHTTP(rw, httptest.NewRequest(http.MethodPost, "/api/v1/admin/invitations/1/revoke", nil))
	if rw.Code != http.StatusNoContent {
		t.Fatalf("revoke: %d (%s)", rw.Code, rw.Body.String())
	}
	// Revoking again is idempotent 204.
	rw2 := httptest.NewRecorder()
	r.ServeHTTP(rw2, httptest.NewRequest(http.MethodPost, "/api/v1/admin/invitations/1/revoke", nil))
	if rw2.Code != http.StatusNoContent {
		t.Fatalf("re-revoke: %d", rw2.Code)
	}
	// Now not listed.
	lw := httptest.NewRecorder()
	r.ServeHTTP(lw, httptest.NewRequest(http.MethodGet, "/api/v1/admin/invitations", nil))
	var list []map[string]any
	_ = json.NewDecoder(lw.Body).Decode(&list)
	if len(list) != 0 {
		t.Fatalf("revoked link must not be listed, got %d", len(list))
	}
}
```

> If any existing test elsewhere in this file (or in `service`/`store`) still calls the old `Create(..., email, ...)` shape, update those call sites to the new signature in the same task.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/http/handlers/api/v1/onboarding/...`
Expected: compile failure — `HandleRevokeInvitation` undefined and `invitationStore.Create` signature mismatch.

- [ ] **Step 3: Update the interface and handlers**

In `internal/http/handlers/api/v1/onboarding/handler.go`:

Replace the `invitationStore` interface:

```go
type invitationStore interface {
	Create(ctx context.Context, scope domain.TenantScope, role domain.Role, tokenHash string, createdBy int64, maxUses *int, expiresAt *time.Time) (*domain.Invitation, error)
	ListPendingByTenant(ctx context.Context, scope domain.TenantScope) ([]domain.Invitation, error)
	Revoke(ctx context.Context, scope domain.TenantScope, id int64) error
}
```

Replace `HandleCreateInvitation`:

```go
// POST /api/v1/admin/invitations  {role?, max_uses?, expires_at?}
func (h *Handler) HandleCreateInvitation(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusForbidden, "no active tenant")
		return
	}
	var body struct {
		Role      string     `json:"role"`
		MaxUses   *int       `json:"max_uses"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.MaxUses != nil && *body.MaxUses <= 0 {
		writeError(w, http.StatusBadRequest, "max_uses must be positive")
		return
	}
	role := domain.Role(body.Role)
	if role != domain.RoleAdmin && role != domain.RoleUser {
		role = domain.RoleUser
	}
	var createdBy int64
	if u := auth.UserFromContext(r.Context()); u != nil {
		createdBy = u.ID
	}

	raw, hash, err := service.GenerateInviteToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := h.invites.Create(r.Context(), scope, role, hash, createdBy, body.MaxUses, body.ExpiresAt); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{
		"token":    raw,
		"url":      h.baseURL + "/invite/" + raw,
		"role":     string(role),
		"max_uses": body.MaxUses,
	})
}
```

Replace the list-item mapping loop in `HandleListInvitations`:

```go
	out := make([]map[string]any, 0, len(list))
	for _, inv := range list {
		out = append(out, map[string]any{
			"id": inv.ID, "role": string(inv.Role), "status": string(inv.Status),
			"max_uses": inv.MaxUses, "use_count": inv.UseCount,
			"created_at": inv.CreatedAt, "expires_at": inv.ExpiresAt,
		})
	}
	writeJSON(w, out)
```

Add the revoke handler (near the other admin handlers):

```go
// POST /api/v1/admin/invitations/{id}/revoke
func (h *Handler) HandleRevokeInvitation(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusForbidden, "no active tenant")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.invites.Revoke(r.Context(), scope, id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

> `strconv`, `time`, and `chi` are already imported in this file. If `go vet` reports an unused import after edits, remove it.

- [ ] **Step 4: Run the handler tests to verify they pass**

Run: `go test ./internal/http/handlers/api/v1/onboarding/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/http/handlers/api/v1/onboarding/handler.go internal/http/handlers/api/v1/onboarding/handler_test.go
# Do NOT run git commit — hand back to the user.
```

---

### Task 5: Route wiring + immediate claim for authenticated visitors

**Files:**
- Modify: `internal/http/server.go` (register revoke route)
- Modify: `internal/http/handlers/web/authhandler/handler.go` (`HandleInvite`)
- Test: `internal/http/handlers/web/authhandler/handler_test.go` (create if absent)

**Interfaces:**
- Consumes: `auth.UserFromContext`, `auth.SessionFromContext`, `Onboarder.ClaimInvitation`, `sessionWriter.SetActiveTenant` (all already on `authhandler.Handler`).
- Produces: `/invite/{token}` claims immediately for an authenticated visitor (session present → `SetActiveTenant`), then redirects `/`; unauthenticated visitor keeps the cookie→login flow; `POST /api/v1/admin/invitations/{id}/revoke` mounted in the admin group.

- [ ] **Step 1: Register the revoke route**

In `internal/http/server.go`, immediately after the existing line

```go
		r.Post("/api/v1/admin/invitations", onboardH.HandleCreateInvitation)
```

add:

```go
		r.Post("/api/v1/admin/invitations/{id}/revoke", onboardH.HandleRevokeInvitation)
```

(This is inside the same tenant-admin-gated group as the other `/api/v1/admin/invitations` routes — do not move it out of that group.)

- [ ] **Step 2: Write a failing authhandler test**

Create `internal/http/handlers/web/authhandler/handler_test.go` (or add to it if present). It uses fakes for the `Onboarder` and `sessionWriter` seams — no DB needed:

```go
package authhandler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"okrs/internal/auth"
	"okrs/internal/domain"
	"okrs/internal/http/handlers/web/authhandler"

	"github.com/go-chi/chi/v5"
)

type fakeOnboard struct {
	claimTenant int64
	claimErr    error
	claimedRaw  string
}

func (f *fakeOnboard) ClaimInvitation(_ context.Context, raw string, _ int64) (*domain.Membership, error) {
	f.claimedRaw = raw
	if f.claimErr != nil {
		return nil, f.claimErr
	}
	return &domain.Membership{TenantID: f.claimTenant, Role: domain.RoleUser, Status: domain.MembershipActive}, nil
}
func (f *fakeOnboard) EnsureRegistration(context.Context, int64) (bool, error) { return false, nil }

type fakeSessions struct{ setTenant int64 }

func (f *fakeSessions) SetActiveTenant(_ context.Context, _ string, tenantID int64) error {
	f.setTenant = tenantID
	return nil
}

// serve runs HandleInvite for token "tok" with the given request context.
func serve(t *testing.T, ob authhandler.Onboarder, sw *fakeSessions, ctx context.Context) *httptest.ResponseRecorder {
	t.Helper()
	h := authhandler.New(nil, nil, nil, ob, sw)
	r := chi.NewRouter()
	r.Get("/invite/{token}", h.HandleInvite)
	req := httptest.NewRequest(http.MethodGet, "/invite/tok", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestHandleInviteAuthenticatedClaimsImmediately(t *testing.T) {
	ob := &fakeOnboard{claimTenant: 7}
	sw := &fakeSessions{}
	ctx := auth.WithSession(auth.WithUser(context.Background(), &domain.User{ID: 3}), &domain.AuthSession{ID: "sess-1"})
	w := serve(t, ob, sw, ctx)

	if w.Code != http.StatusFound || w.Header().Get("Location") != "/" {
		t.Fatalf("want 302 → /, got %d → %q", w.Code, w.Header().Get("Location"))
	}
	if ob.claimedRaw != "tok" {
		t.Fatalf("claim not called with token, got %q", ob.claimedRaw)
	}
	if sw.setTenant != 7 {
		t.Fatalf("SetActiveTenant tenant = %d, want 7", sw.setTenant)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == "okrs_invite" && c.Value != "" && c.MaxAge >= 0 {
			t.Fatalf("authenticated claim must not stash okrs_invite cookie")
		}
	}
}

func TestHandleInviteUnauthenticatedStashesCookie(t *testing.T) {
	ob := &fakeOnboard{}
	sw := &fakeSessions{}
	w := serve(t, ob, sw, context.Background()) // no user in context

	if w.Code != http.StatusFound || w.Header().Get("Location") != "/login?next=/" {
		t.Fatalf("want 302 → /login?next=/, got %d → %q", w.Code, w.Header().Get("Location"))
	}
	var found bool
	for _, c := range w.Result().Cookies() {
		if c.Name == "okrs_invite" && c.Value == "tok" {
			found = true
		}
	}
	if !found {
		t.Fatalf("unauthenticated visit must stash okrs_invite=tok")
	}
	if ob.claimedRaw != "" {
		t.Fatalf("must not claim before login, claimed %q", ob.claimedRaw)
	}
}
```

> Confirm the context setters are named `auth.WithUser` and `auth.WithSession` (grep `internal/auth/context.go`). If a setter has a different name, use the actual one. `authhandler.New(nil, nil, nil, ...)` passes nil `mgr`/`tmpl`/`logger`; `HandleInvite` must not touch them (it only reads context + calls `onboard`/`sessions`).

- [ ] **Step 3: Run to verify failure**

Run: `go test ./internal/http/handlers/web/authhandler/...`
Expected: FAIL — current `HandleInvite` ignores the session and always stashes the cookie (first test fails on `Location`/`SetActiveTenant`).

- [ ] **Step 4: Rewrite HandleInvite**

In `internal/http/handlers/web/authhandler/handler.go`, replace `HandleInvite`:

```go
// HandleInvite redeems an invite-link token. An already-authenticated visitor is claimed
// immediately (and their session is focused on the invite's tenant) and sent to the app;
// an anonymous visitor gets the token stashed in a short-lived cookie and is sent to login,
// where the callback redeems it. Invalid tokens never block: authenticated visitors are
// bounced to the app (RequireMembership routes them onward), anonymous ones to login.
func (h *Handler) HandleInvite(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if user := auth.UserFromContext(r.Context()); user != nil {
		if m, err := h.onboard.ClaimInvitation(r.Context(), token, user.ID); err == nil {
			if sess := auth.SessionFromContext(r.Context()); sess != nil {
				_ = h.sessions.SetActiveTenant(r.Context(), sess.ID, m.TenantID)
			}
		}
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "okrs_invite",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   600,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/login?next=/", http.StatusFound)
}
```

- [ ] **Step 5: Run the authhandler tests to verify they pass**

Run: `go test ./internal/http/handlers/web/authhandler/...`
Expected: PASS.

- [ ] **Step 6: Full build + vet + test**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: builds clean, vet clean, all tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/http/server.go internal/http/handlers/web/authhandler/handler.go internal/http/handlers/web/authhandler/handler_test.go
# Do NOT run git commit — hand back to the user.
```

---

### Task 6: Frontend — «Пригласительные ссылки» panel in `/admin?section=users`

**Files:**
- Modify: `internal/web/static/admin.js` (add `InviteLinksPanel`, render it in `UsersSection`)

**Interfaces:**
- Consumes: `apiGet`, `apiPost` (send CSRF header + JSON), `T` theme object, `inpStyle`, React hooks (`useState`, `useEffect`). Endpoints: `POST /api/v1/admin/invitations`, `GET /api/v1/admin/invitations`, `POST /api/v1/admin/invitations/{id}/revoke`.
- Produces: a self-contained panel component rendered above the users list in `UsersSection`.

- [ ] **Step 1: Add the `InviteLinksPanel` component**

In `internal/web/static/admin.js`, add this component just above `function UsersSection(` (around line 1091):

```jsx
function InviteLinksPanel() {
  const [links, setLinks] = useState([]);
  const [role, setRole] = useState('user');
  const [kind, setKind] = useState('once'); // once | unlimited | limited
  const [limit, setLimit] = useState(5);
  const [expires, setExpires] = useState(''); // yyyy-mm-dd or ''
  const [created, setCreated] = useState(null); // {url}
  const [busy, setBusy] = useState(false);

  const load = () => apiGet('/api/v1/admin/invitations').then(r=>r&&r.json()).then(d=>setLinks(Array.isArray(d)?d:[])).catch(()=>setLinks([]));
  useEffect(()=>{ load(); },[]);

  async function create() {
    setBusy(true);
    try {
      const body = {role};
      if (kind==='once') body.max_uses = 1;
      else if (kind==='limited') body.max_uses = Math.max(1, parseInt(limit,10)||1);
      // 'unlimited' → omit max_uses
      if (expires) body.expires_at = new Date(expires+'T23:59:59').toISOString();
      const r = await apiPost('/api/v1/admin/invitations', body);
      if (!r || !r.ok) { alert('Не удалось создать ссылку'); return; }
      const d = await r.json();
      setCreated({url:d.url});
      await load();
    } finally { setBusy(false); }
  }

  async function revoke(id) {
    const r = await apiPost(`/api/v1/admin/invitations/${id}/revoke`, {});
    if (r && r.ok) load(); else alert('Не удалось отозвать ссылку');
  }

  const usesLabel = l => l.max_uses==null ? `${l.use_count}/∞` : `${l.use_count}/${l.max_uses}`;

  return <div style={{background:'white',borderRadius:14,border:'1px solid '+T.cardBorder,boxShadow:'0 1px 3px rgba(15,23,42,0.04)',overflow:'hidden',marginBottom:16}}>
    <div style={{padding:'14px 16px',borderBottom:'1px solid '+T.hairline,fontSize:13,fontWeight:700,color:T.headingFg}}>Пригласительные ссылки</div>
    <div style={{padding:'14px 16px',display:'flex',gap:12,alignItems:'flex-end',flexWrap:'wrap',borderBottom:'1px solid '+T.hairline}}>
      <label style={{display:'flex',flexDirection:'column',gap:4,fontSize:12,color:T.mutedFg,fontWeight:600}}>Роль
        <select value={role} onChange={e=>setRole(e.target.value)} style={{...inpStyle,padding:'8px 10px'}}>
          <option value="user">Пользователь</option>
          <option value="admin">Администратор</option>
        </select>
      </label>
      <label style={{display:'flex',flexDirection:'column',gap:4,fontSize:12,color:T.mutedFg,fontWeight:600}}>Тип
        <select value={kind} onChange={e=>setKind(e.target.value)} style={{...inpStyle,padding:'8px 10px'}}>
          <option value="once">Одноразовая</option>
          <option value="unlimited">Многоразовая (без лимита)</option>
          <option value="limited">До N использований</option>
        </select>
      </label>
      {kind==='limited' && <label style={{display:'flex',flexDirection:'column',gap:4,fontSize:12,color:T.mutedFg,fontWeight:600}}>N
        <input type="number" min="1" value={limit} onChange={e=>setLimit(e.target.value)} style={{...inpStyle,padding:'8px 10px',width:80}}/>
      </label>}
      <label style={{display:'flex',flexDirection:'column',gap:4,fontSize:12,color:T.mutedFg,fontWeight:600}}>Срок (опц.)
        <input type="date" value={expires} onChange={e=>setExpires(e.target.value)} style={{...inpStyle,padding:'8px 10px'}}/>
      </label>
      <button onClick={create} disabled={busy} style={{padding:'9px 16px',border:'none',borderRadius:8,background:T.accent,color:'#fff',fontWeight:600,cursor:busy?'default':'pointer',fontFamily:'inherit',opacity:busy?0.6:1}}>Создать</button>
    </div>
    {created && <div style={{padding:'12px 16px',borderBottom:'1px solid '+T.hairline,display:'flex',gap:10,alignItems:'center',background:'#f5f3ff'}}>
      <input readOnly value={created.url} style={{...inpStyle,flex:1,fontFamily:'ui-monospace,Menlo,monospace',fontSize:12.5}} onFocus={e=>e.target.select()}/>
      <button onClick={()=>{ navigator.clipboard?.writeText(created.url); }} style={{padding:'9px 14px',border:'1.5px solid '+T.cardBorder,borderRadius:8,background:'#fff',color:T.accent,fontWeight:600,cursor:'pointer',fontFamily:'inherit'}}>Копировать</button>
    </div>}
    <div>
      {links.length===0 && <div style={{padding:'20px 16px',textAlign:'center',color:T.dimFg,fontSize:13}}>Активных ссылок нет</div>}
      {links.map(l=>(
        <div key={l.id} style={{display:'flex',alignItems:'center',gap:12,padding:'10px 16px',borderBottom:'1px solid '+T.hairline}}>
          <div style={{flex:1,minWidth:0}}>
            <div style={{fontSize:13.5,fontWeight:600,color:T.headingFg}}>{l.role==='admin'?'Администратор':'Пользователь'} · использовано {usesLabel(l)}</div>
            <div style={{fontSize:11.5,color:T.mutedFg,marginTop:2}}>{l.expires_at?`Действует до ${fmtDateTime(l.expires_at)}`:'Без срока'}</div>
          </div>
          <button onClick={()=>revoke(l.id)} style={{padding:'6px 12px',border:'1.5px solid '+T.cardBorder,borderRadius:7,background:'#fff',color:T.danger,fontWeight:600,cursor:'pointer',fontFamily:'inherit'}}>Отозвать</button>
        </div>
      ))}
    </div>
  </div>;
}
```

> `fmtDateTime` and `inpStyle` and `T` are already defined in this file (used by `UsersSection`/`UserModal`). Confirm they are in scope; if `fmtDateTime` is not exported at module scope, use the same date rendering the file already uses for `expires_at`-like fields.

- [ ] **Step 2: Render the panel in `UsersSection`**

In `UsersSection`'s returned JSX, wrap the existing content so the panel renders above the users card. Change the opening of the return from:

```jsx
  return <div style={{padding:'20px 24px 24px'}}>
    <div style={{background:'white',borderRadius:14,border:'1px solid '+T.cardBorder,...
```

to insert `<InviteLinksPanel/>` right after the outer `<div style={{padding:'20px 24px 24px'}}>`:

```jsx
  return <div style={{padding:'20px 24px 24px'}}>
    <InviteLinksPanel/>
    <div style={{background:'white',borderRadius:14,border:'1px solid '+T.cardBorder,boxShadow:'0 1px 3px rgba(15,23,42,0.04)',overflow:'hidden'}}>
```

(Only add the `<InviteLinksPanel/>` line; leave the rest of the users card unchanged.)

- [ ] **Step 3: Manual verification against DoD (`specs/010`)**

Build and run the server against a live DB (per the `verify` workflow used earlier: standalone Postgres container + `PROVISIONING_TOKEN` + `AUTH_MODE=disabled`, or a real auth run). Then, as a tenant-admin:

1. Open `/admin?section=users`. Expect the «Пригласительные ссылки» panel above the users list.
2. Create a **one-time** link → a URL appears with «Копировать». Open it in a fresh authenticated session (different user) → that user is added to the tenant and lands in the app. Open the same URL again → not added (link exhausted); the user is bounced to `/` without error.
3. Create a **multi-use (до N=2)** link → two different authenticated users can join; a third cannot.
4. Create an **unlimited** link → multiple users can join.
5. In the list, «Отозвать» a link → it disappears and can no longer be claimed.

Record the observed results.

- [ ] **Step 4: Commit**

```bash
git add internal/web/static/admin.js
# Do NOT run git commit — hand back to the user.
```

---

### Task 7: Update the API contract spec

**Files:**
- Modify: `specs/040-api-contract.md`

**Interfaces:**
- Consumes: nothing.
- Produces: documented invite-link endpoints.

- [ ] **Step 1: Update the invitations section**

In `specs/040-api-contract.md`, locate the existing `POST /api/v1/admin/invitations` documentation (grep for `invitations`). Replace/extend it so it reads (adapt wording to the file's existing table/prose style):

- `POST /api/v1/admin/invitations` — tenant-admin. Body `{role?: "user"|"admin", max_uses?: int, expires_at?: RFC3339}`. Creates a generic invite link (no email). `max_uses` absent/null → unlimited; `1` → one-time; `N` → up to N uses. `max_uses <= 0` → `400`. Response `201 {token, url, role, max_uses}`.
- `GET /api/v1/admin/invitations` — tenant-admin. Lists pending links: `[{id, role, status, max_uses, use_count, created_at, expires_at}]` (no email).
- `POST /api/v1/admin/invitations/{id}/revoke` — tenant-admin. Marks the link revoked; idempotent, tenant-scoped → `204`.
- `GET /invite/{token}` — public. Authenticated visitor is added to the invite's tenant immediately and redirected to `/`; anonymous visitor is sent to login and claimed in the OAuth callback. Invalid/expired/revoked/exhausted tokens redirect without error.

- [ ] **Step 2: Verify the doc references no email field for invitations**

Run: `rg -n "invitations" specs/040-api-contract.md`
Expected: the create/list entries no longer mention an `email` request/response field.

- [ ] **Step 3: Commit**

```bash
git add specs/040-api-contract.md
# Do NOT run git commit — hand back to the user.
```

---

## Self-Review

**Spec coverage** (against `docs/superpowers/specs/2026-07-01-invite-links-design.md`):
- Migration 034 (email nullable, max_uses/use_count, backfill max_uses=1, down restores NOT NULL) → Task 1. ✅
- Repo `Create` without email, atomic `Consume`, `Revoke`, `ListPendingByTenant` new fields, `domain.Invitation` new fields → Task 2. ✅
- Service `ClaimInvitation` via `Consume` + idempotent Upsert → Task 3. ✅
- `HandleInvite` immediate claim for authenticated + `SetActiveTenant` + soft redirect → Task 5. ✅
- API create `{role, max_uses?, expires_at?}` → `201 {token,url,role,max_uses}`; `max_uses<=0` → 400; revoke → 204 idempotent; list without email + counts → Task 4 (+ route in Task 5). ✅
- UI panel in `/admin?section=users` (role, type once/unlimited/limited-N, optional expiry, copy URL, list with «Отозвать») → Task 6. ✅
- Error handling: invalid/exhausted → `ErrInvalidInvitation`; revoke idempotent 204/no-op; gates via existing `RequireTenantAdmin` group → Tasks 2/4/5. ✅
- Tests: repo (`Consume` one-time/limited/unlimited/unknown, revoke isolation, list fields), service (single + replay + unknown), handler (create/list/revoke/validation), authhandler (auth vs anon) → Tasks 2–5. ✅
- specs/040 update → Task 7. ✅
- Out of scope (SMTP, branding, analytics, email-matching) → not implemented. ✅

**Placeholder scan:** No TBD/TODO/"handle edge cases"; every code step shows full code. The two `>` notes (extract `setupClaimTest`, confirm `auth.WithUser`/`WithSession` names, confirm `fmtDateTime` scope) are verification instructions, not placeholders — they tell the implementer to reuse exact existing names.

**Type consistency:** `Create(ctx, scope, role, tokenHash, createdBy, maxUses *int, expiresAt *time.Time)` and `Consume(ctx, tokenHash) (*ClaimResult, error)` and `Revoke(ctx, scope, id int64) error` are used identically in repo (Task 2), interface + handler (Task 4), and service (Task 3). `ClaimResult{TenantID, Role}` consistent. `domain.Invitation` fields `Email *string`, `MaxUses *int`, `UseCount int` consistent across scans and list mapping. `HandleRevokeInvitation` name consistent between handler (Task 4) and route (Task 5). Frontend endpoint paths match the Go routes.
