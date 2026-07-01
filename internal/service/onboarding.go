package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"okrs/internal/domain"
	"okrs/internal/store/grants"
	"okrs/internal/store/invitations"
	"okrs/internal/store/memberships"
	"okrs/internal/store/tenants"
)

// policyDefaultNode mirrors auth.PolicyDefaultNode; kept as a literal here to avoid an
// import cycle (auth must not depend on service).
const policyDefaultNode = "default_node"

var (
	ErrInvalidInvitation = errors.New("onboarding: invalid or expired invitation")
	ErrAlreadyMember     = errors.New("onboarding: already an active member")
	ErrTenantNotFound    = errors.New("onboarding: tenant not found")
)

// NewUserGranter applies the new-user hierarchy-grant policy. Both *grants.GrantRepository
// and *grants.GrantsCache satisfy it.
type NewUserGranter interface {
	ListUserGrants(ctx context.Context, scope domain.TenantScope, userID int64) ([]grants.HierarchyGrant, error)
	AddUserGrant(ctx context.Context, scope domain.TenantScope, userID, teamID, grantedByUserID int64) error
}

// OnboardingService decides post-login outcomes: invitation claim, self-service join-request,
// and new-user registration. It takes explicit scope where a tenant is in play.
type OnboardingService struct {
	inv      *invitations.InvitationRepository
	mem      *memberships.MembershipRepository
	memCache *memberships.MembershipCache
	tenants  *tenants.TenantRepository
	settings *SettingsService
	granter  NewUserGranter
}

func NewOnboardingService(
	inv *invitations.InvitationRepository,
	mem *memberships.MembershipRepository,
	memCache *memberships.MembershipCache,
	tn *tenants.TenantRepository,
	settings *SettingsService,
	granter NewUserGranter,
) *OnboardingService {
	return &OnboardingService{inv: inv, mem: mem, memCache: memCache, tenants: tn, settings: settings, granter: granter}
}

// GenerateInviteToken returns a random raw token and its sha256 hex hash. Only the hash is stored.
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

// RequestAccess records a self-service join request (status=requested) for a tenant by slug.
// An existing active membership → ErrAlreadyMember; unknown slug → ErrTenantNotFound.
func (s *OnboardingService) RequestAccess(ctx context.Context, slug string, userID int64) error {
	tn, err := s.tenants.GetBySlug(ctx, slug)
	if errors.Is(err, tenants.ErrNotFound) {
		return ErrTenantNotFound
	}
	if err != nil {
		return err
	}
	existing, err := s.mem.Get(ctx, userID, tn.ID)
	if err != nil && !errors.Is(err, memberships.ErrNotFound) {
		return err
	}
	if existing != nil && existing.Status == domain.MembershipActive {
		return ErrAlreadyMember
	}
	if _, err := s.mem.Upsert(ctx, domain.Membership{
		UserID:   userID,
		TenantID: tn.ID,
		Role:     domain.RoleUser,
		Status:   domain.MembershipRequested,
	}); err != nil {
		return err
	}
	s.memCache.InvalidateUser(userID)
	return nil
}

// ListAccessRequests returns the tenant's pending join requests.
func (s *OnboardingService) ListAccessRequests(ctx context.Context, scope domain.TenantScope) ([]memberships.AccessRequest, error) {
	return s.mem.ListAccessRequests(ctx, scope)
}

// ApproveRequest activates a pending membership.
func (s *OnboardingService) ApproveRequest(ctx context.Context, scope domain.TenantScope, userID int64) error {
	if err := s.mem.SetStatus(ctx, userID, scope.TenantID, domain.MembershipActive); err != nil {
		return err
	}
	s.memCache.InvalidateUser(userID)
	return nil
}

// DenyRequest removes a pending membership.
func (s *OnboardingService) DenyRequest(ctx context.Context, scope domain.TenantScope, userID int64) error {
	if err := s.mem.DeleteRequested(ctx, scope, userID); err != nil {
		return err
	}
	s.memCache.InvalidateUser(userID)
	return nil
}

// EnsureRegistration routes a logged-in user with no active membership into the
// default_registration_tenant_id tenant (if configured), creating an active role=user
// membership and applying that tenant's new_user_policy. Returns (true, nil) if a membership
// was created; (false, nil) if the user already has one or no default tenant is configured
// (caller then routes to the no-membership page).
func (s *OnboardingService) EnsureRegistration(ctx context.Context, userID int64) (bool, error) {
	existing, err := s.mem.ListByUser(ctx, userID) // active-only
	if err != nil {
		return false, err
	}
	if len(existing) > 0 {
		return false, nil
	}
	tenantID, ok, err := s.defaultRegistrationTenant(ctx)
	if err != nil || !ok {
		return false, err
	}
	scope := domain.TenantScope{TenantID: tenantID}
	if _, err := s.mem.Upsert(ctx, domain.Membership{
		UserID:   userID,
		TenantID: tenantID,
		Role:     domain.RoleUser,
		Status:   domain.MembershipActive,
	}); err != nil {
		return false, err
	}
	s.memCache.InvalidateUser(userID)
	if err := s.applyNewUserPolicy(ctx, scope, userID); err != nil {
		return false, err
	}
	return true, nil
}

// defaultRegistrationTenant reads the global default_registration_tenant_id system setting.
func (s *OnboardingService) defaultRegistrationTenant(ctx context.Context) (int64, bool, error) {
	raw, err := s.settings.SystemGet(ctx, "default_registration_tenant_id")
	if err != nil || raw == nil {
		return 0, false, err
	}
	var id *int64
	if err := json.Unmarshal(raw, &id); err != nil || id == nil || *id == 0 {
		return 0, false, nil
	}
	return *id, true, nil
}

// applyNewUserPolicy grants the registration tenant's default hierarchy node to a new member
// (policy "default_node"), if the user has no grant there yet. Moved here from auth.Manager so
// it targets the resolved registration tenant, not a hardcoded one.
func (s *OnboardingService) applyNewUserPolicy(ctx context.Context, scope domain.TenantScope, userID int64) error {
	policyRaw, _ := s.settings.GetTenant(ctx, scope, "new_user_policy")
	var policy string
	if policyRaw != nil {
		_ = json.Unmarshal(policyRaw, &policy)
	}
	if policy != policyDefaultNode {
		return nil
	}
	nodeRaw, _ := s.settings.GetTenant(ctx, scope, "default_hierarchy_node_id")
	var nodeID int64
	if nodeRaw != nil {
		_ = json.Unmarshal(nodeRaw, &nodeID)
	}
	if nodeID == 0 {
		return nil
	}
	gs, err := s.granter.ListUserGrants(ctx, scope, userID)
	if err != nil {
		return err
	}
	if len(gs) > 0 {
		return nil
	}
	return s.granter.AddUserGrant(ctx, scope, userID, nodeID, domain.SystemUserAnonymous)
}
