package service_test

import (
	"context"
	"errors"
	"testing"

	"okrs/internal/domain"
	"okrs/internal/service"
	"okrs/internal/store/grants"
	"okrs/internal/store/invitations"
	"okrs/internal/store/memberships"
	"okrs/internal/store/settings"
	"okrs/internal/store/tenants"
	"okrs/internal/store/tenantsettings"
	"okrs/internal/store/testutil"

	"github.com/jackc/pgx/v5/pgxpool"
)

func newOnboardingForTest(t *testing.T, pool *pgxpool.Pool) *service.OnboardingService {
	t.Helper()
	invRepo := invitations.NewInvitationRepository(pool)
	memRepo := memberships.NewMembershipRepository(pool)
	tnRepo := tenants.NewTenantRepository(pool)
	tsRepo := tenantsettings.NewTenantSettingsRepository(pool)
	sysRepo := settings.NewSettingsRepository(pool)
	settingsSvc := service.NewSettingsService(
		tenantsettings.NewTenantSettingsCache(tsRepo), tsRepo,
		settings.NewSystemSettingsCache(sysRepo), sysRepo,
	)
	granter := grants.NewGrantsCache(grants.NewGrantRepository(pool))
	return service.NewOnboardingService(invRepo, memRepo, memberships.NewMembershipCache(memRepo), tnRepo, settingsSvc, granter)
}

func newSettingsForTest(t *testing.T, pool *pgxpool.Pool) *service.SettingsService {
	t.Helper()
	tsRepo := tenantsettings.NewTenantSettingsRepository(pool)
	sysRepo := settings.NewSettingsRepository(pool)
	return service.NewSettingsService(
		tenantsettings.NewTenantSettingsCache(tsRepo), tsRepo,
		settings.NewSystemSettingsCache(sysRepo), sysRepo,
	)
}

func TestApproveRequestAppliesDefaultAccess(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	svc := newOnboardingForTest(t, pool)
	settingsSvc := newSettingsForTest(t, pool)
	grantRepo := grants.NewGrantRepository(pool)

	// Configure tenant 1 default-node policy pointing at a seeded team.
	var teamID int64
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('Root') RETURNING id`).Scan(&teamID); err != nil {
		t.Fatalf("seed team: %v", err)
	}
	scope := domain.TenantScope{TenantID: 1}
	if err := settingsSvc.SetTenantProduct(ctx, scope, "new_user_policy", "default_node"); err != nil {
		t.Fatalf("set policy: %v", err)
	}
	if err := settingsSvc.SetTenantProduct(ctx, scope, "default_hierarchy_node_id", teamID); err != nil {
		t.Fatalf("set node: %v", err)
	}

	var uid int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('github:a','github','a','A') RETURNING id`).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := svc.RequestAccess(ctx, "default", uid); err != nil {
		t.Fatalf("request: %v", err)
	}
	if err := svc.ApproveRequest(ctx, scope, uid); err != nil {
		t.Fatalf("approve: %v", err)
	}

	gs, err := grantRepo.ListUserGrants(ctx, scope, uid)
	if err != nil {
		t.Fatalf("list grants: %v", err)
	}
	if len(gs) != 1 || gs[0].TeamID != teamID {
		t.Fatalf("grants = %+v, want one grant on team %d", gs, teamID)
	}
}

func TestLeaveTenantLastAdminGuard(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	svc := newOnboardingForTest(t, pool)
	mem := memberships.NewMembershipRepository(pool)

	var admin int64
	_ = pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('github:la','github','la','LA') RETURNING id`).Scan(&admin)
	_, _ = mem.Upsert(ctx, domain.Membership{UserID: admin, TenantID: 1, Role: domain.RoleAdmin, Status: domain.MembershipActive})

	// Sole admin cannot leave.
	if err := svc.LeaveTenant(ctx, 1, admin); !errors.Is(err, service.ErrLastAdmin) {
		t.Fatalf("err = %v, want ErrLastAdmin", err)
	}

	// A plain user can leave (and it removes the membership).
	var user int64
	_ = pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('github:pu','github','pu','PU') RETURNING id`).Scan(&user)
	_, _ = mem.Upsert(ctx, domain.Membership{UserID: user, TenantID: 1, Role: domain.RoleUser, Status: domain.MembershipActive})
	if err := svc.LeaveTenant(ctx, 1, user); err != nil {
		t.Fatalf("leave: %v", err)
	}
	if _, err := mem.Get(ctx, user, 1); !errors.Is(err, memberships.ErrNotFound) {
		t.Fatalf("membership still present: %v", err)
	}

	// Leaving a tenant you're not in is a no-op.
	if err := svc.LeaveTenant(ctx, 1, 999999); err != nil {
		t.Fatalf("non-member leave: %v", err)
	}
}

func TestJoinRequestApproveDeny(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	svc := newOnboardingForTest(t, pool)
	mem := memberships.NewMembershipRepository(pool)

	var uid int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('github:j','github','j','J') RETURNING id`).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}

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

	// Unknown slug → ErrTenantNotFound.
	if err := svc.RequestAccess(ctx, "nope", uid); err != service.ErrTenantNotFound {
		t.Fatalf("expected ErrTenantNotFound, got %v", err)
	}
}

func TestDenyRemovesRequest(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	svc := newOnboardingForTest(t, pool)
	mem := memberships.NewMembershipRepository(pool)

	var uid int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('github:d','github','d','D') RETURNING id`).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := svc.RequestAccess(ctx, "default", uid); err != nil {
		t.Fatalf("request: %v", err)
	}
	if err := svc.DenyRequest(ctx, domain.TenantScope{TenantID: 1}, uid); err != nil {
		t.Fatalf("deny: %v", err)
	}
	if _, err := mem.Get(ctx, uid, 1); err != memberships.ErrNotFound {
		t.Fatalf("denied request should be removed, got %v", err)
	}
}

func TestEnsureRegistrationUsesDefaultTenant(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	svc := newOnboardingForTest(t, pool)
	mem := memberships.NewMembershipRepository(pool)

	// Seed a team to grant, and tenant #1 product settings + the global default tenant.
	var teamID int64
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name, tenant_id) VALUES ('Root', 1) RETURNING id`).Scan(&teamID); err != nil {
		t.Fatalf("seed team: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO system_settings (key, value_json) VALUES ('default_registration_tenant_id', '1')`); err != nil {
		t.Fatalf("seed sys setting: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tenant_settings (tenant_id, key, value_json) VALUES
		(1, 'new_user_policy', '"default_node"'),
		(1, 'default_hierarchy_node_id', to_jsonb($1::bigint))
		ON CONFLICT (tenant_id, key) DO UPDATE SET value_json = EXCLUDED.value_json`, teamID); err != nil {
		t.Fatalf("seed tenant settings: %v", err)
	}

	var uid int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('github:n','github','n','N') RETURNING id`).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	created, err := svc.EnsureRegistration(ctx, uid)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if !created {
		t.Fatal("expected a membership to be created")
	}
	m, err := mem.Get(ctx, uid, 1)
	if err != nil || m.Status != domain.MembershipActive || m.Role != domain.RoleUser {
		t.Fatalf("membership = %+v err=%v", m, err)
	}
}

func TestEnsureRegistrationNoDefaultTenantIsNoop(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	svc := newOnboardingForTest(t, pool)
	mem := memberships.NewMembershipRepository(pool)

	var uid int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('github:m','github','m','M') RETURNING id`).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	created, err := svc.EnsureRegistration(ctx, uid)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if created {
		t.Fatal("no default tenant → no membership")
	}
	if _, err := mem.Get(ctx, uid, 1); err != memberships.ErrNotFound {
		t.Fatalf("expected no membership, got %v", err)
	}
}

func TestClaimInvitationAppliesNewUserPolicy(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	svc := newOnboardingForTest(t, pool)

	// Tenant #1: default-node policy pointing at a seeded team.
	var teamID int64
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name, tenant_id) VALUES ('Root', 1) RETURNING id`).Scan(&teamID); err != nil {
		t.Fatalf("seed team: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tenant_settings (tenant_id, key, value_json) VALUES
		(1, 'new_user_policy', '"default_node"'),
		(1, 'default_hierarchy_node_id', to_jsonb($1::bigint))
		ON CONFLICT (tenant_id, key) DO UPDATE SET value_json = EXCLUDED.value_json`, teamID); err != nil {
		t.Fatalf("seed tenant settings: %v", err)
	}

	var uid int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('github:inv','github','inv','Inv') RETURNING id`).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	raw, hash, _ := service.GenerateInviteToken()
	inv := invitations.NewInvitationRepository(pool)
	if _, err := inv.Create(ctx, domain.TenantScope{TenantID: 1}, domain.RoleUser, hash, 1, intp(1), nil); err != nil {
		t.Fatalf("seed invite: %v", err)
	}

	if _, err := svc.ClaimInvitation(ctx, raw, uid); err != nil {
		t.Fatalf("claim: %v", err)
	}

	gs, err := grants.NewGrantRepository(pool).ListUserGrants(ctx, domain.TenantScope{TenantID: 1}, uid)
	if err != nil {
		t.Fatalf("list grants: %v", err)
	}
	if len(gs) != 1 || gs[0].TeamID != teamID {
		t.Fatalf("claim must apply the default-node policy grant; grants = %+v", gs)
	}
}

func TestSetMemberRole(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	svc := newOnboardingForTest(t, pool)
	mem := memberships.NewMembershipRepository(pool)
	scope := domain.TenantScope{TenantID: 1}

	var uid int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('github:sr','github','sr','SR') RETURNING id`).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := mem.Upsert(ctx, domain.Membership{UserID: uid, TenantID: 1, Role: domain.RoleUser, Status: domain.MembershipActive}); err != nil {
		t.Fatalf("seed membership: %v", err)
	}

	if err := svc.SetMemberRole(ctx, scope, uid, domain.RoleAdmin); err != nil {
		t.Fatalf("promote: %v", err)
	}
	m, _ := mem.Get(ctx, uid, 1)
	if m.Role != domain.RoleAdmin {
		t.Fatalf("role = %q, want admin", m.Role)
	}
	if err := svc.SetMemberRole(ctx, scope, uid, domain.RoleUser); err != nil {
		t.Fatalf("demote: %v", err)
	}
	m, _ = mem.Get(ctx, uid, 1)
	if m.Role != domain.RoleUser {
		t.Fatalf("role = %q, want user", m.Role)
	}
}

func TestRemoveMemberUnlinksFromTenant(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	svc := newOnboardingForTest(t, pool)
	mem := memberships.NewMembershipRepository(pool)
	gr := grants.NewGrantRepository(pool)
	scope := domain.TenantScope{TenantID: 1}

	var uid, teamID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('github:rm','github','rm','RM') RETURNING id`).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name, tenant_id) VALUES ('Root', 1) RETURNING id`).Scan(&teamID); err != nil {
		t.Fatalf("seed team: %v", err)
	}
	if _, err := mem.Upsert(ctx, domain.Membership{UserID: uid, TenantID: 1, Role: domain.RoleUser, Status: domain.MembershipActive}); err != nil {
		t.Fatalf("seed membership: %v", err)
	}
	if err := gr.AddUserGrant(ctx, scope, uid, teamID, domain.SystemUserAnonymous); err != nil {
		t.Fatalf("seed grant: %v", err)
	}

	if err := svc.RemoveMember(ctx, scope, uid); err != nil {
		t.Fatalf("remove: %v", err)
	}

	if _, err := mem.Get(ctx, uid, 1); err != memberships.ErrNotFound {
		t.Fatalf("membership must be gone, got %v", err)
	}
	gs, err := gr.ListUserGrants(ctx, scope, uid)
	if err != nil {
		t.Fatalf("list grants: %v", err)
	}
	if len(gs) != 0 {
		t.Fatalf("grants must be revoked, got %+v", gs)
	}

	// Idempotent: removing a non-member again is a no-op.
	if err := svc.RemoveMember(ctx, scope, uid); err != nil {
		t.Fatalf("second remove should be no-op nil, got %v", err)
	}
}

func TestClaimInvitationSingleUseBindsToIdentity(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	svc := newOnboardingForTest(t, pool)

	if _, err := pool.Exec(ctx, `INSERT INTO tenants (id, slug, name) OVERRIDING SYSTEM VALUE VALUES (2,'t2','T2')`); err != nil {
		t.Fatalf("tenant 2: %v", err)
	}
	var uid int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name, email)
		VALUES ('github:x','github','x','X','someone-else@example.com') RETURNING id`).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	raw, hash, _ := service.GenerateInviteToken()
	inv := invitations.NewInvitationRepository(pool)
	if _, err := inv.Create(ctx, domain.TenantScope{TenantID: 2}, domain.RoleAdmin, hash, 1, intp(1), nil); err != nil {
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

func intp(n int) *int { return &n }
