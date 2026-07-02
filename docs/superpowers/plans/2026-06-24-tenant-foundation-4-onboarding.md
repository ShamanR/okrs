# Tenant Foundation — Plan 4: Onboarding (invitations, join-requests, new-user flow)

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans (inline,
> compiler-driven — subagents flap in this environment). Steps use checkbox (`- [ ]`) syntax.
> Build/vet/test green after every task; commits are the user's (agent does `git add` +
> proposes a message, no AI attribution).

**Goal:** Turn the three onboarding primitives into working flows — token-link **invitations**
(tenant-admin invites by email, claimed by whoever opens the link), **join-requests** (a logged-in
user with no membership asks to join a tenant by slug, a tenant-admin approves), and the
**new-user flow** (first SSO login routes to the `default_registration_tenant_id` tenant or to a
pluggable no-membership page) — and replace the inline `/no-access` stub with a pluggable
`NoMembershipHandler` seam.

**Architecture:** All decisioning lives in one `service.OnboardingService` that takes an explicit
`domain.TenantScope` where a tenant is in play and is invoked from the OAuth callback (post-login)
and from HTTP handlers. The OAuth round-trip carries an invite token through an `okrs_invite`
cookie (same pattern as the existing `okrs_oauth_next` cookie). Invitations are claimed only by a
valid single-use token (consumed atomically), binding the active membership to the **current
identity** (`provider:subject`), never by email-match. The new-user grant logic
(`applyNewUserPolicy`) moves out of `auth.Manager.Login` into the onboarding step so registration
targets the resolved tenant, not a hardcoded #1.

**Tech Stack:** Go, pgx/v5, chi, testcontainers-go. No new migration — `tenant_invitations`
(migration 031) and `memberships.status` (`active`/`requested`, migration 028) already exist.

---

## Spec ↔ code state (read before implementing)

- `tenant_invitations` table exists (031): `id, tenant_id, email, role, token_hash, status
  (pending|claimed|revoked), created_by_user_id, created_at, expires_at`. **No repository yet.**
- `memberships.status` is `active`|`requested` (028). Repo has `Upsert`, `Get`, `ListByUser`
  (active-only), `SetStatus`. **No tenant-scoped "list requested" query yet.**
- `auth.Manager.Login` still calls `applyNewUserPolicy`, which **hardcodes registration tenant #1**
  (`internal/auth/manager.go`, `TODO(tenancy)`); this plan generalizes it via the global
  `default_registration_tenant_id` and moves it into onboarding.
- `default_registration_tenant_id` is writable via `/api/v1/system/settings/default-registration-tenant`
  (Plan 3) and readable via `service.SettingsService.SystemGet`. **Not consumed anywhere yet.**
- The OAuth callback (`internal/http/handlers/web/authhandler/handler.go`) calls `Login` then
  redirects to the `okrs_oauth_next` cookie value; `/no-access` is an inline HTML stub in
  `server.go` outside the membership-gated group; `RequireMembershipMiddleware` redirects there.
- `sessions.SetActiveTenant(ctx, sessionID, tenantID)` exists (used to focus a session on a tenant
  after claim/approve).
- Provisioning `AttachMember` (Plan 3) is direct-membership only; this plan adds the email→invite
  and self-service join paths.

No `specs/` are contradicted; `030`/`040`/`050` are updated in the last task.

---

## Global Constraints

- **Explicit `domain.TenantScope`** as the first business param on scoped service/repo methods;
  services/repos never read tenant from context. Handlers read `auth.TenantScopeFromContext`
  (tenant-admin endpoints) or `auth.UserFromContext` (onboarding endpoints, where no tenant is
  resolved yet).
- **Claim by token only.** A membership is granted by redeeming a valid, unexpired, unclaimed
  token, bound to the **current logged-in identity**. Email is a delivery label, never a key:
  logging in without the token grants nothing even if the verified email matches an invite.
- **Single-use tokens.** Claim consumes the token atomically (`UPDATE … WHERE status='pending'`
  returning rows affected); replay/expired/revoked/unknown → `ErrInvalidInvitation`.
- **Tokens stored hashed.** The raw token is returned once (to build the link); only
  `sha256(token)` hex is persisted. Use `crypto/rand` (mirror `authhandler.generateState`).
- **`requested` never downgrades `active`.** Join-request must not clobber an existing active
  membership.
- Commits are the user's (`git add` + proposed message); **no AI/Claude attribution** anywhere.
- testcontainers (docker required); `store/testutil` restores `DEFAULT 1` for single-tenant fixtures.

---

### Task 1: `InvitationRepository` + `domain.Invitation`

**Files:**
- Modify: `internal/domain/tenant.go` (add `Invitation`, `InvitationStatus`)
- Create: `internal/store/invitations/invitations.go`
- Create: `internal/store/invitations/invitations_test.go`
- Modify: `internal/store/store.go` (wire `Invitations`)

**Interfaces:**
- Produces:
  - `domain.InvitationStatus` (`InvitationPending`/`InvitationClaimed`/`InvitationRevoked`),
    `domain.Invitation{ ID, TenantID int64; Email string; Role Role; Status InvitationStatus;
    CreatedByUserID *int64; CreatedAt time.Time; ExpiresAt *time.Time }`.
  - `func NewInvitationRepository(db *pgxpool.Pool) *InvitationRepository`
  - `(*InvitationRepository) Create(ctx, scope domain.TenantScope, email string, role domain.Role, tokenHash string, createdBy int64, expiresAt *time.Time) (*domain.Invitation, error)`
  - `(*InvitationRepository) GetPendingByTokenHash(ctx, tokenHash string) (*domain.Invitation, error)` — returns `ErrNotFound` if absent/not pending.
  - `(*InvitationRepository) MarkClaimed(ctx, id int64) (bool, error)` — atomic `UPDATE … SET status='claimed' WHERE id=$1 AND status='pending'`; returns `true` iff a row changed.
  - `(*InvitationRepository) ListPendingByTenant(ctx, scope domain.TenantScope) ([]domain.Invitation, error)`
  - `var ErrNotFound = errors.New("invitations: not found")`

> `GetPendingByTokenHash` is global by token (the claimer has no tenant context yet); the token
> hash is the authority and carries its own tenant. `Create`/`ListPendingByTenant` are tenant-scoped.

- [ ] **Step 1: Add domain types** to `internal/domain/tenant.go`:

```go
type InvitationStatus string

const (
	InvitationPending InvitationStatus = "pending"
	InvitationClaimed InvitationStatus = "claimed"
	InvitationRevoked InvitationStatus = "revoked"
)

type Invitation struct {
	ID              int64
	TenantID        int64
	Email           string
	Role            Role
	Status          InvitationStatus
	CreatedByUserID *int64
	CreatedAt       time.Time
	ExpiresAt       *time.Time
}
```

- [ ] **Step 2: Write the failing test**

```go
// internal/store/invitations/invitations_test.go
package invitations_test

import (
	"context"
	"testing"

	"okrs/internal/domain"
	"okrs/internal/store/invitations"
	"okrs/internal/store/testutil"
)

func TestInvitationCreateClaimSingleUse(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := invitations.NewInvitationRepository(pool)
	scope := domain.TenantScope{TenantID: 1}

	inv, err := repo.Create(ctx, scope, "a@example.com", domain.RoleUser, "hash123", 1, nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if inv.Status != domain.InvitationPending {
		t.Fatalf("status = %q", inv.Status)
	}

	got, err := repo.GetPendingByTokenHash(ctx, "hash123")
	if err != nil || got.ID != inv.ID {
		t.Fatalf("get by hash: %v / %+v", err, got)
	}

	ok, err := repo.MarkClaimed(ctx, inv.ID)
	if err != nil || !ok {
		t.Fatalf("first claim should succeed: ok=%v err=%v", ok, err)
	}
	// Second claim is a no-op (single use).
	ok, _ = repo.MarkClaimed(ctx, inv.ID)
	if ok {
		t.Fatalf("second claim must not change a row")
	}
	// No longer pending → lookup fails.
	if _, err := repo.GetPendingByTokenHash(ctx, "hash123"); err != invitations.ErrNotFound {
		t.Fatalf("claimed token must not be pending, got %v", err)
	}
}
```

- [ ] **Step 3: Run to verify it fails** — `go test ./internal/store/invitations` → FAIL (package absent).

- [ ] **Step 4: Implement the repository**

```go
// internal/store/invitations/invitations.go
package invitations

import (
	"context"
	"errors"
	"time"

	"okrs/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("invitations: not found")

type InvitationRepository struct {
	db *pgxpool.Pool
}

func NewInvitationRepository(db *pgxpool.Pool) *InvitationRepository {
	return &InvitationRepository{db: db}
}

func (r *InvitationRepository) Create(ctx context.Context, scope domain.TenantScope, email string, role domain.Role, tokenHash string, createdBy int64, expiresAt *time.Time) (*domain.Invitation, error) {
	var inv domain.Invitation
	err := r.db.QueryRow(ctx, `
		INSERT INTO tenant_invitations (tenant_id, email, role, token_hash, status, created_by_user_id, expires_at)
		VALUES ($1, $2, $3, $4, 'pending', $5, $6)
		RETURNING id, tenant_id, email, role, status, created_by_user_id, created_at, expires_at`,
		scope.TenantID, email, role, tokenHash, createdBy, expiresAt).
		Scan(&inv.ID, &inv.TenantID, &inv.Email, &inv.Role, &inv.Status, &inv.CreatedByUserID, &inv.CreatedAt, &inv.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

func (r *InvitationRepository) GetPendingByTokenHash(ctx context.Context, tokenHash string) (*domain.Invitation, error) {
	var inv domain.Invitation
	err := r.db.QueryRow(ctx, `
		SELECT id, tenant_id, email, role, status, created_by_user_id, created_at, expires_at
		FROM tenant_invitations
		WHERE token_hash = $1 AND status = 'pending' AND (expires_at IS NULL OR expires_at > now())`,
		tokenHash).
		Scan(&inv.ID, &inv.TenantID, &inv.Email, &inv.Role, &inv.Status, &inv.CreatedByUserID, &inv.CreatedAt, &inv.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

func (r *InvitationRepository) MarkClaimed(ctx context.Context, id int64) (bool, error) {
	ct, err := r.db.Exec(ctx, `UPDATE tenant_invitations SET status = 'claimed' WHERE id = $1 AND status = 'pending'`, id)
	if err != nil {
		return false, err
	}
	return ct.RowsAffected() == 1, nil
}

func (r *InvitationRepository) ListPendingByTenant(ctx context.Context, scope domain.TenantScope) ([]domain.Invitation, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, tenant_id, email, role, status, created_by_user_id, created_at, expires_at
		FROM tenant_invitations
		WHERE tenant_id = $1 AND status = 'pending' ORDER BY created_at DESC`, scope.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Invitation
	for rows.Next() {
		var inv domain.Invitation
		if err := rows.Scan(&inv.ID, &inv.TenantID, &inv.Email, &inv.Role, &inv.Status, &inv.CreatedByUserID, &inv.CreatedAt, &inv.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}
```

- [ ] **Step 5: Wire into `store.go`** — add field `Invitations *invitations.InvitationRepository`
  and construct it in `New` (mirror `TenantSettings`).

- [ ] **Step 6: Run** `go test ./internal/store/invitations && go build ./...` → PASS.
- [ ] **Step 7: Stage** (message: `feat(store): tenant invitations repository`).

---

### Task 2: Membership access-request query

**Files:**
- Modify: `internal/store/memberships/memberships.go` (`ListAccessRequests`)
- Test: `internal/store/memberships/memberships_test.go`

**Interfaces:**
- Produces:
  - `type AccessRequest struct { UserID int64; DisplayName, Email string; Role domain.Role; CreatedAt time.Time }`
  - `(*MembershipRepository) ListAccessRequests(ctx, scope domain.TenantScope) ([]AccessRequest, error)`
    — `status='requested'` memberships in the tenant, joined to `users` for display.

- [ ] **Step 1: Write the failing test** — seed a `requested` membership for a user, assert
  `ListAccessRequests` returns it with the display name; an `active` membership is excluded.

```go
func TestListAccessRequests(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := memberships.NewMembershipRepository(pool)

	var uid int64
	_ = pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name, email)
		VALUES ('github:r','github','r','Req','r@example.com') RETURNING id`).Scan(&uid)
	if _, err := repo.Upsert(ctx, domain.Membership{UserID: uid, TenantID: 1, Role: domain.RoleUser, Status: domain.MembershipRequested}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	reqs, err := repo.ListAccessRequests(ctx, domain.TenantScope{TenantID: 1})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(reqs) != 1 || reqs[0].UserID != uid || reqs[0].DisplayName != "Req" {
		t.Fatalf("got %+v", reqs)
	}
}
```

- [ ] **Step 2: Run to verify it fails.**

- [ ] **Step 3: Implement**

```go
type AccessRequest struct {
	UserID      int64
	DisplayName string
	Email       string
	Role        domain.Role
	CreatedAt   time.Time
}

func (r *MembershipRepository) ListAccessRequests(ctx context.Context, scope domain.TenantScope) ([]AccessRequest, error) {
	rows, err := r.db.Query(ctx, `
		SELECT m.user_id, u.display_name, COALESCE(u.email,''), m.role, m.created_at
		FROM memberships m JOIN users u ON u.id = m.user_id
		WHERE m.tenant_id = $1 AND m.status = 'requested'
		ORDER BY m.created_at`, scope.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AccessRequest
	for rows.Next() {
		var a AccessRequest
		if err := rows.Scan(&a.UserID, &a.DisplayName, &a.Email, &a.Role, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
```

  Add `"time"` to the imports if not present.

- [ ] **Step 4: Run** `go test ./internal/store/memberships` → PASS.
- [ ] **Step 5: Stage** (message: `feat(store): list pending access requests per tenant`).

---

### Task 3: `OnboardingService` — token utilities + invitation claim

**Files:**
- Create: `internal/service/onboarding.go`
- Test: `internal/service/onboarding_test.go`

**Interfaces:**
- Consumes: `*invitations.InvitationRepository`, `*memberships.MembershipRepository`,
  `*tenants.TenantRepository`, `*memberships.MembershipCache` (invalidate on write),
  `*SettingsService`, and a granter for the new-user policy (Task 5).
- Produces:
  - `func NewOnboardingService(inv *invitations.InvitationRepository, mem *memberships.MembershipRepository, memCache *memberships.MembershipCache, tn *tenants.TenantRepository, settings *SettingsService, granter NewUserGranter) *OnboardingService`
  - `func GenerateInviteToken() (raw string, hash string, err error)` — 32 random bytes hex + `sha256` hex.
  - `func HashInviteToken(raw string) string`
  - `var ErrInvalidInvitation = errors.New("onboarding: invalid or expired invitation")`
  - `(*OnboardingService) ClaimInvitation(ctx, rawToken string, userID int64) (*domain.Membership, error)`
    — looks up the pending token, **atomically** marks it claimed, then creates an active
    membership (invite's role) for `userID` in the invite's tenant; invalidates the membership
    cache. Replay/expired/revoked/unknown → `ErrInvalidInvitation`.

> `NewUserGranter` is defined in Task 5; declare it there. In this task `ClaimInvitation` does not
> touch grants, so the field may be nil-tolerant — but to keep the constructor stable, define the
> full signature now and pass the granter from the start (Task 5 supplies the impl).

- [ ] **Step 1: Write the failing test** (claim once, replay rejected, unknown rejected, and that
  claim binds to the *current* user id regardless of invite email):

```go
// internal/service/onboarding_test.go (excerpt)
func TestClaimInvitationSingleUseBindsToIdentity(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	svc := newOnboardingForTest(t, pool) // helper builds repos + service

	// Create tenant 2 + a user whose email differs from the invite email.
	_, _ = pool.Exec(ctx, `INSERT INTO tenants (id, slug, name) OVERRIDING SYSTEM VALUE VALUES (2,'t2','T2')`)
	var uid int64
	_ = pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name, email)
		VALUES ('github:x','github','x','X','someone-else@example.com') RETURNING id`).Scan(&uid)

	raw, hash, _ := service.GenerateInviteToken()
	inv := invitations.NewInvitationRepository(pool)
	if _, err := inv.Create(ctx, domain.TenantScope{TenantID: 2}, "invited@example.com", domain.RoleAdmin, hash, 1, nil); err != nil {
		t.Fatalf("seed invite: %v", err)
	}

	m, err := svc.ClaimInvitation(ctx, raw, uid)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if m.TenantID != 2 || m.Role != domain.RoleAdmin || m.Status != domain.MembershipActive {
		t.Fatalf("membership = %+v", m)
	}
	// Replay rejected.
	if _, err := svc.ClaimInvitation(ctx, raw, uid); err != service.ErrInvalidInvitation {
		t.Fatalf("replay must fail, got %v", err)
	}
	// Unknown token rejected.
	if _, err := svc.ClaimInvitation(ctx, "deadbeef", uid); err != service.ErrInvalidInvitation {
		t.Fatalf("unknown token must fail, got %v", err)
	}
}
```

  > The `newOnboardingForTest` helper builds the repos + `OnboardingService` from the pool (mirror
  > the construction in `provisioning_test.go`); pass a real granter (Task 5) or a no-op for this test.

- [ ] **Step 2: Run to verify it fails.**

- [ ] **Step 3: Implement token utils + `ClaimInvitation`**

```go
func GenerateInviteToken() (string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	raw := hex.EncodeToString(b)
	return raw, HashInviteToken(raw), nil
}

func HashInviteToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (s *OnboardingService) ClaimInvitation(ctx context.Context, rawToken string, userID int64) (*domain.Membership, error) {
	inv, err := s.inv.GetPendingByTokenHash(ctx, HashInviteToken(rawToken))
	if errors.Is(err, invitations.ErrNotFound) {
		return nil, ErrInvalidInvitation
	}
	if err != nil {
		return nil, err
	}
	claimed, err := s.inv.MarkClaimed(ctx, inv.ID) // atomic single-use guard
	if err != nil {
		return nil, err
	}
	if !claimed {
		return nil, ErrInvalidInvitation
	}
	m, err := s.mem.Upsert(ctx, domain.Membership{
		UserID: userID, TenantID: inv.TenantID, Role: inv.Role, Status: domain.MembershipActive,
	})
	if err != nil {
		return nil, err
	}
	s.memCache.InvalidateUser(userID)
	return m, nil
}
```

  Imports: `crypto/rand`, `crypto/sha256`, `encoding/hex`, `errors`,
  `okrs/internal/store/invitations`, `okrs/internal/store/memberships`,
  `okrs/internal/store/tenants`, `okrs/internal/domain`.

- [ ] **Step 4: Run** `go test ./internal/service -run TestClaimInvitation` → PASS.
- [ ] **Step 5: Stage** (message: `feat(service): onboarding invitation claim (single-use token)`).

---

### Task 4: `OnboardingService` — join-request + approve/deny

**Files:**
- Modify: `internal/service/onboarding.go`
- Test: `internal/service/onboarding_test.go`

**Interfaces:**
- Produces:
  - `var ErrAlreadyMember = errors.New("onboarding: already an active member")`
  - `var ErrTenantNotFound = errors.New("onboarding: tenant not found")`
  - `(*OnboardingService) RequestAccess(ctx, slug string, userID int64) error` — resolves the
    tenant by slug; if the user already has an active membership there → `ErrAlreadyMember`;
    otherwise upserts a `requested` membership (`role=user`). Invalidates the membership cache.
  - `(*OnboardingService) ListAccessRequests(ctx, scope domain.TenantScope) ([]memberships.AccessRequest, error)`
  - `(*OnboardingService) ApproveRequest(ctx, scope domain.TenantScope, userID int64) error` —
    `SetStatus(active)`; invalidates the user's membership cache.
  - `(*OnboardingService) DenyRequest(ctx, scope domain.TenantScope, userID int64) error` —
    deletes the requested membership; invalidates.
- Consumes: `memberships.SetStatus` (exists), a new `memberships.Delete(ctx, scope, userID)` —
  add it: `DELETE FROM memberships WHERE user_id=$2 AND tenant_id=$1 AND status='requested'`.

- [ ] **Step 1: Write the failing test** — request by slug creates a `requested` row;
  `RequireMembership` does not pass `requested` (already covered by middleware tests — assert at the
  service level that the membership status is `requested`); approve flips to `active`; deny removes it;
  requesting where already active → `ErrAlreadyMember`.

```go
func TestJoinRequestApproveDeny(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	svc := newOnboardingForTest(t, pool)
	mem := memberships.NewMembershipRepository(pool)

	var uid int64
	_ = pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('github:j','github','j','J') RETURNING id`).Scan(&uid)

	if err := svc.RequestAccess(ctx, "default", uid); err != nil {
		t.Fatalf("request: %v", err)
	}
	m, _ := mem.Get(ctx, uid, 1)
	if m.Status != domain.MembershipRequested {
		t.Fatalf("status = %q, want requested", m.Status)
	}
	if err := svc.ApproveRequest(ctx, domain.TenantScope{TenantID: 1}, uid); err != nil {
		t.Fatalf("approve: %v", err)
	}
	m, _ = mem.Get(ctx, uid, 1)
	if m.Status != domain.MembershipActive {
		t.Fatalf("status = %q, want active", m.Status)
	}
	// Re-requesting when already active → ErrAlreadyMember.
	if err := svc.RequestAccess(ctx, "default", uid); err != service.ErrAlreadyMember {
		t.Fatalf("expected ErrAlreadyMember, got %v", err)
	}
}
```

- [ ] **Step 2: Run to verify it fails.**
- [ ] **Step 3: Add `memberships.Delete`** (requested-only) and implement the four service methods.
  `RequestAccess` resolves the slug via `s.tenants.GetBySlug` (map `tenants.ErrNotFound` →
  `ErrTenantNotFound`), `Get`s any existing membership (active → `ErrAlreadyMember`), else
  `Upsert{Status: requested}`.
- [ ] **Step 4: Run** `go test ./internal/service -run TestJoinRequest && go test ./internal/store/memberships` → PASS.
- [ ] **Step 5: Stage** (message: `feat(service): join-request + approve/deny onboarding`).

---

### Task 5: `OnboardingService` — new-user registration (move `applyNewUserPolicy`)

**Files:**
- Modify: `internal/service/onboarding.go` (registration + grant)
- Modify: `internal/auth/manager.go` (remove `applyNewUserPolicy` call from `Login`)
- Test: `internal/service/onboarding_test.go`, `internal/auth/manager_bootstrap_test.go` (adjust)

**Interfaces:**
- Produces:
  - `type NewUserGranter interface { ListUserGrants(ctx, scope domain.TenantScope, userID int64) ([]grants.HierarchyGrant, error); AddUserGrant(ctx, scope domain.TenantScope, userID, teamID, grantedByUserID int64) error }`
    (both `*store.GrantsCache` and `*grants.GrantRepository` satisfy it — same as `auth.userGranter`).
  - `(*OnboardingService) EnsureRegistration(ctx, userID int64) (claimed bool, err error)` —
    if the user already has any active membership → returns `false, nil` (no-op). Else reads the
    global `default_registration_tenant_id` via `settings.SystemGet`; if set → creates an active
    `role=user` membership in that tenant, applies that tenant's `new_user_policy`
    (`default_node` → grant `default_hierarchy_node_id` if the user has no grant there yet), and
    returns `true, nil`. If unset → returns `false, nil` (caller routes to no-membership).
- Consumes: `domain.User` is no longer policy-applied inside `Login`.

- [ ] **Step 1: Write the failing test** — with `default_registration_tenant_id=1` and tenant #1's
  `new_user_policy="default_node"` + `default_hierarchy_node_id=<seeded team>`, `EnsureRegistration`
  creates an active membership in #1 and a grant; with the setting unset, it returns
  `(false, nil)` and creates no membership.

- [ ] **Step 2: Run to verify it fails.**

- [ ] **Step 3: Implement `EnsureRegistration`** — port the grant logic from
  `auth/manager.go applyNewUserPolicy` (read `new_user_policy` / `default_hierarchy_node_id` via
  `settings.GetTenant(scope, …)` for the resolved registration tenant; grant via the
  `NewUserGranter`). Resolve the registration tenant from
  `settings.SystemGet(ctx, "default_registration_tenant_id")` (JSON `int64`; nil/absent → no-op).

- [ ] **Step 4: Remove `applyNewUserPolicy` from `Login`** — delete the call (and the now-unused
  method + the `grants`/`userGranter` wiring in `auth.Manager` **only if** nothing else uses it;
  if `userGranter` is no longer referenced, drop it from `NewManager`'s signature and update
  `cmd/server/main.go` + `auth` tests accordingly). The bootstrap promotion stays in `Login`.

  > **Behaviour check:** previously a first-time login auto-granted in tenant #1. That behaviour is
  > now reproduced by `EnsureRegistration` invoked from the OAuth callback (Task 8), gated on
  > `default_registration_tenant_id` (OSS default is `1`, preserving single-tenant behaviour once
  > the operator sets it; if unset, new users correctly land on no-membership).

- [ ] **Step 5: Run** `go test ./internal/service ./internal/auth && go build ./...` → PASS.
  Fix `manager_bootstrap_test.go` if it relied on `applyNewUserPolicy` (it asserts only promotion,
  so it should still pass; the `fakeAuthStore`'s `userGranter` methods may become unused — drop them
  if the granter is removed from `NewManager`).

- [ ] **Step 6: Stage** (message: `feat(service): new-user registration via default_registration_tenant_id`).

---

### Task 6: `NoMembershipHandler` seam + OSS join-request page

**Files:**
- Create: `internal/onboarding/nomembership.go` (interface + registry + OSS stub)
- Create: `internal/http/templates/no_membership.html`
- Modify: `internal/http/server.go` (`/no-access` renders the registered handler)
- Test: `internal/onboarding/nomembership_test.go`

**Interfaces:**
- Produces (pure seam, no `internal/store` imports — mirror `entitlements`):
  - `type NoMembershipHandler interface { ServeNoMembership(w http.ResponseWriter, r *http.Request) }`
  - `func Register(name string, h NoMembershipHandler)` / `func Get(name string) (NoMembershipHandler, bool)`
  - `type StubHandler struct{ Render func(w http.ResponseWriter, r *http.Request) }` implementing
    the interface by delegating to `Render` (lets the HTTP layer inject template rendering without
    the seam importing templates).

- [ ] **Step 1: Write the failing test** — register a `StubHandler`, `Get` it, call
  `ServeNoMembership`, assert the injected `Render` ran.
- [ ] **Step 2: Run to verify it fails.**
- [ ] **Step 3: Implement** the interface + mutex-guarded registry + `StubHandler`.
- [ ] **Step 4: Add `no_membership.html`** — a minimal page (mirror `system_shell.html` style)
  explaining "no access" + a join-request form (`<input slug>` + submit) that `POST`s to
  `/api/v1/onboarding/join-request` (endpoint built in Task 11) via vanilla `fetch` using
  `textContent` for any rendered errors (no raw HTML / XSS).
- [ ] **Step 5: Wire `/no-access` in `server.go`** — replace the inline HTML stub with: look up the
  registered handler (default name `"stub"`, registered at startup with `Render` =
  `s.tmpl.ExecuteTemplate(w, "no-membership", nil)`); call `ServeNoMembership`. Keep the route
  outside the membership-gated group.
- [ ] **Step 6: Run** `go test ./internal/onboarding && go build ./...` → PASS.
- [ ] **Step 7: Stage** (message: `feat(onboarding): pluggable no-membership handler + join-request page`).

---

### Task 7: Invite link route + OAuth cookie threading

**Files:**
- Modify: `internal/http/handlers/web/authhandler/handler.go` (invite cookie helpers; consume in callback)
- Modify: `internal/http/server.go` (`/invite/{token}` route)
- Test: `internal/http/handlers/web/authhandler/handler_test.go` (or add one)

**Interfaces:**
- `/invite/{token}` (public) sets cookie `okrs_invite=<token>` (short-lived, `HttpOnly`, `Path=/`)
  and redirects to `/login?next=/`. The callback reads + clears it.

- [ ] **Step 1: Write the failing test** — `GET /invite/abc` sets the `okrs_invite` cookie and
  redirects (302) to `/login`.
- [ ] **Step 2: Run to verify it fails.**
- [ ] **Step 3: Add the invite handler** `HandleInvite` to `authhandler` (read `chi.URLParam("token")`,
  set cookie, redirect) and a `r.Get("/invite/{token}", authH.HandleInvite)` route in `server.go`
  (public group, alongside `/login`).
- [ ] **Step 4: Run** `go test ./internal/http/handlers/web/authhandler && go build ./...` → PASS.
- [ ] **Step 5: Stage** (message: `feat(auth): invite link route carries token through OAuth`).

---

### Task 8: Callback onboarding integration

**Files:**
- Modify: `internal/http/handlers/web/authhandler/handler.go` (`HandleCallback`, `New` takes onboarding)
- Modify: `internal/http/server.go` (construct `authhandler.New` with the onboarding service + sessions)
- Test: handler test for the three post-login outcomes

**Interfaces:**
- `authhandler.New` gains an onboarding dependency:

```go
type Onboarder interface {
	ClaimInvitation(ctx context.Context, rawToken string, userID int64) (*domain.Membership, error)
	EnsureRegistration(ctx context.Context, userID int64) (bool, error)
}
```

  and a session writer (`SetActiveTenant`) to focus the session on the claimed/registered tenant.

- [ ] **Step 1: Write the failing test** — after a stubbed `Login` returns a user, exercise:
  (a) with an `okrs_invite` cookie whose token claims tenant 2 → membership created, session active
  tenant set to 2, redirect to app; (b) no cookie, no membership, `EnsureRegistration` returns
  `true` → redirect to app; (c) no cookie, `EnsureRegistration` returns `false` → redirect to
  `/no-access`. Use a fake `Onboarder`.
- [ ] **Step 2: Run to verify it fails.**
- [ ] **Step 3: Wire the callback** — after `Login` + session cookie, in order:

```go
claimed := false
if ic, err := r.Cookie("okrs_invite"); err == nil && ic.Value != "" {
	http.SetCookie(w, &http.Cookie{Name: "okrs_invite", MaxAge: -1, Path: "/"})
	if m, err := h.onboard.ClaimInvitation(r.Context(), ic.Value, user.ID); err == nil {
		_ = h.sessions.SetActiveTenant(r.Context(), sess.ID, m.TenantID)
		claimed = true
	}
	// Invalid invite falls through to the normal new-user routing below.
}
if !claimed {
	if _, err := h.onboard.EnsureRegistration(r.Context(), user.ID); err != nil {
		h.logger.Error("ensure registration", slog.String("error", err.Error()))
	}
}
```

  Keep the existing `okrs_oauth_next` redirect. If after onboarding the user still has no active
  membership, `RequireMembershipMiddleware` will route them to `/no-access` on the next request, so
  the callback can keep redirecting to `next` (the middleware is the single source of truth for the
  no-membership gate) — do **not** duplicate the membership check here.

- [ ] **Step 4: Run** `go test ./internal/http/handlers/web/authhandler && go build ./...` → PASS.
- [ ] **Step 5: Stage** (message: `feat(auth): claim invite / register new user after login`).

---

### Task 9: Tenant-admin invitation endpoint

**Files:**
- Create: `internal/http/handlers/api/v1/onboarding/handler.go` (admin + user onboarding handlers)
- Test: `internal/http/handlers/api/v1/onboarding/handler_test.go`
- Modify: `internal/http/server.go` (mount under the tenant-admin group)

**Interfaces:**
- `POST /api/v1/admin/invitations` `{ "email": "...", "role": "user|admin" }` → `201`
  `{ "token": "<raw>", "url": "<BaseURL>/invite/<raw>", "email": "...", "role": "..." }`.
  Tenant from `auth.TenantScopeFromContext`; `created_by` from `auth.UserFromContext`. OSS returns
  the raw link for the admin to deliver (no SMTP). The raw token is shown **once** here.
- `GET /api/v1/admin/invitations` → list pending (from `ListPendingByTenant`).

> The admin onboarding handler depends on a small interface satisfied by the repos/services it needs
> (`Create`/`ListPendingByTenant` + `GenerateInviteToken`). Build the link from the configured
> `BaseURL` (pass it into the handler from `server.go`; `auth.Config().BaseURL`).

- [ ] **Step 1: Write the failing test** — POST creates an invitation, response carries a non-empty
  `token` and a `url` ending in that token; a second `GET` lists one pending invite. Inject tenant #1
  + an admin user into context.
- [ ] **Step 2: Run to verify it fails.**
- [ ] **Step 3: Implement** `HandleCreateInvitation` (validate role ∈ {user,admin}; generate token;
  `Invitations.Create` with the hash; return raw token + url) and `HandleListInvitations`.
- [ ] **Step 4: Mount in `registerAdminRoutes`** (`server.go`), inside the `RequireTenantAdmin` group:
  `r.Post("/api/v1/admin/invitations", obAdmin.HandleCreateInvitation)` and the `GET`.
- [ ] **Step 5: Run** `go test ./internal/http/handlers/api/v1/onboarding ./internal/http/... && go build ./...` → PASS.
- [ ] **Step 6: Stage** (message: `feat(admin): create + list tenant invitations`).

---

### Task 10: Tenant-admin access-request endpoints

**Files:**
- Modify: `internal/http/handlers/api/v1/onboarding/handler.go`
- Test: `internal/http/handlers/api/v1/onboarding/handler_test.go`
- Modify: `internal/http/server.go`

**Interfaces:**
- `GET /api/v1/admin/access-requests` → `[{user_id, display_name, email, role, created_at}]`.
- `POST /api/v1/admin/access-requests/{userID}/approve` → `204`.
- `POST /api/v1/admin/access-requests/{userID}/deny` → `204`.
  Tenant from context; delegate to `OnboardingService.ListAccessRequests/ApproveRequest/DenyRequest`.

- [ ] **Step 1: Write the failing test** — seed a `requested` membership; `GET` lists it; `approve`
  flips it to active (assert via a follow-up `GET` returning empty); a separate `deny` removes one.
- [ ] **Step 2: Run to verify it fails.**
- [ ] **Step 3: Implement** the three handlers (parse `{userID}` with `chi.URLParam` + `strconv`).
- [ ] **Step 4: Mount** under the `RequireTenantAdmin` group in `server.go`.
- [ ] **Step 5: Run** `go test ./internal/http/handlers/api/v1/onboarding && go build ./...` → PASS.
- [ ] **Step 6: Stage** (message: `feat(admin): access-request queue approve/deny`).

---

### Task 11: User join-request endpoint + onboarding HTTP group

**Files:**
- Modify: `internal/http/handlers/api/v1/onboarding/handler.go` (`HandleJoinRequest`)
- Modify: `internal/http/server.go` (new auth-but-not-membership group)
- Test: `internal/http/handlers/api/v1/onboarding/handler_test.go`

**Interfaces:**
- `POST /api/v1/onboarding/join-request` `{ "slug": "acme" }` → `204` (created/idempotent);
  `404` if the slug is unknown (`ErrTenantNotFound`); `409` if already an active member
  (`ErrAlreadyMember`). Auth required (user from context), membership **not** required.

- [ ] **Step 1: Write the failing test** — with a user in context, POST `{"slug":"default"}` →
  `204` and a `requested` membership exists; unknown slug → `404`.
- [ ] **Step 2: Run to verify it fails.**
- [ ] **Step 3: Implement `HandleJoinRequest`** (read user from context → 401 if absent; decode slug;
  call `OnboardingService.RequestAccess`; map `ErrTenantNotFound`→404, `ErrAlreadyMember`→409).
- [ ] **Step 4: Add the onboarding group in `server.go`** — a group with `RequireAuth` (enabled mode)
  + `csrf.Handler` but **no** `TenantResolve`/`RequireMembership` (a user with no membership must
  reach it):

```go
r.Group(func(r chi.Router) {
	if !s.auth.Disabled() {
		r.Use(auth.RequireAuthMiddleware)
	}
	r.Use(csrf.Handler)
	r.Post("/api/v1/onboarding/join-request", obUser.HandleJoinRequest)
})
```

- [ ] **Step 5: Run** `go test ./internal/http/handlers/api/v1/onboarding ./internal/http/... && go build ./...` → PASS.
- [ ] **Step 6: Stage** (message: `feat(onboarding): self-service join-request by slug`).

---

### Task 12: Wire `OnboardingService` in `server.go`; register OSS no-membership; specs & seed

**Files:**
- Modify: `internal/http/server.go` (build `OnboardingService`; register `"stub"` no-membership;
  inject onboarding into `authhandler` + the onboarding handlers)
- Modify: `cmd/server/main.go` if construction lives there
- Modify: `specs/030-user-flows.md`, `specs/040-api-contract.md`, `specs/050-permissions-and-lifecycle.md`
- Test: full suite `go test ./...`

- [ ] **Step 1: Build `OnboardingService` once in `NewServer`** (reuse `s.membershipCache`,
  `s.settingsSvc`, `s.grantsCache` as the `NewUserGranter`, `st.Invitations`, `st.Memberships`,
  `st.Tenants`); store it on `Server` and inject into `authhandler.New`, the admin onboarding
  handler, and the user onboarding handler.
- [ ] **Step 2: Register the OSS no-membership handler** at startup:
  `onboarding.Register("stub", onboarding.StubHandler{Render: func(w, r){ _ = s.tmpl.ExecuteTemplate(w, "no-membership", nil) }})`
  and have `/no-access` resolve `"stub"`.
- [ ] **Step 3: Update specs (same change set, no unrelated edits):**
  - `specs/030-user-flows.md`: add the three onboarding flows (invite-claim, join-request+approve,
    new-user via `default_registration_tenant_id` / no-membership page).
  - `specs/040-api-contract.md`: document `/api/v1/admin/invitations`,
    `/api/v1/admin/access-requests[/{userID}/approve|deny]`, `/api/v1/onboarding/join-request`,
    and the public `/invite/{token}` route.
  - `specs/050-permissions-and-lifecycle.md`: note invitations are claimed by token (not email),
    `requested` memberships don't pass `RequireMembership`, and approve/deny is a tenant-admin action.
- [ ] **Step 4: `seed_demo.sql`** — no new structure; add a one-line comment that onboarding uses
  `tenant_invitations` + `memberships.status='requested'` (no seed rows needed).
- [ ] **Step 5: Run the FULL suite** `go build ./... && go vet ./... && go test ./...` → all PASS.
- [ ] **Step 6: Stage** (message: `feat(onboarding): wire onboarding service, no-membership seam, specs`).

---

## Self-Review Notes

- **Spec coverage** (design spec §"Аутентификация, регистрация, онбординг" + §"Новый пользователь"):
  - Invitation by token-link, claim binds to identity, single-use, OSS shows link: Tasks 1, 3, 7, 8, 9.
  - Join-request by slug + approve/deny queue, `requested` gate: Tasks 2, 4, 10, 11.
  - New-user flow via `default_registration_tenant_id` / no-membership page: Tasks 5, 6, 8.
  - Pluggable `NoMembershipHandler` seam (OSS stub + form): Task 6.
- **Security tests (the core):** claim only by valid single-use token (replay/expired/unknown →
  `ErrInvalidInvitation`), bound to current identity; email-match alone grants nothing (a normal
  login without the token never creates a membership — `EnsureRegistration` only fires on the global
  default-tenant setting, never on invite email). One email via two providers stays two accounts
  (identity = `provider:subject`, unchanged from Plan 1).
- **Layering:** decisioning in `OnboardingService` (explicit scope); the OAuth callback and HTTP
  handlers are the only context readers; the `NoMembershipHandler`/`entitlements` seams stay free of
  `internal/store`. The join-request endpoint sits in an auth-but-not-membership group so a
  member-less user can reach it.
- **Behaviour preserved:** removing `applyNewUserPolicy` from `Login` is offset by
  `EnsureRegistration` in the callback; single-tenant OSS keeps auto-onboarding once
  `default_registration_tenant_id=1` (the documented OSS default).
- **No migration:** `tenant_invitations` (031) and `memberships.status` (028) already exist.
- **Out of scope (later phases):** SMTP delivery of invites (SaaS), self-service "create
  organization" onboarding page (SaaS wrapper over provisioning), per-tenant SSO, public tenant
  catalog. This plan ships the in-box primitives only.

## Execution recommendation

Inline, compiler-driven, one task at a time. Natural review batches: **service core** (Tasks 1–5),
**onboarding HTTP + callback** (Tasks 6–8), **admin/user endpoints + wiring** (Tasks 9–12). After
each task: `go build ./... && go vet ./... && go test ./...` green, then `git add` + propose a commit
message (no AI attribution); the user commits.
