package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"okrs/internal/domain"
	"okrs/internal/store/grants"
	"okrs/internal/store/users"
)

// authStorage is the persistence contract Manager needs.
// *store.Store satisfies it via its forwarding methods.
type authStorage interface {
	UpsertUser(ctx context.Context, in users.UpsertUserInput) (*domain.User, error)
	GetUser(ctx context.Context, id int64) (*domain.User, error)
	CreateSession(ctx context.Context, sessionID string, userID int64, provider string, ttl time.Duration, userAgent, ip string) (*domain.AuthSession, error)
	GetSession(ctx context.Context, sessionID string) (*domain.AuthSession, error)
	TouchSession(ctx context.Context, sessionID string) error
	DeleteSession(ctx context.Context, sessionID string) error
	GetSetting(ctx context.Context, key string) (json.RawMessage, error)
	GetTenantSetting(ctx context.Context, scope domain.TenantScope, key string) (json.RawMessage, error)
}

// userGranter is the minimal interface Manager needs for the new-user grant policy.
// Both *store.Store and *store.GrantsCache satisfy it.
type userGranter interface {
	ListUserGrants(ctx context.Context, scope domain.TenantScope, userID int64) ([]grants.HierarchyGrant, error)
	AddUserGrant(ctx context.Context, scope domain.TenantScope, userID, teamID, grantedByUserID int64) error
}

// Manager handles provider selection, session creation, and user upsert.
type Manager struct {
	cfg       Config
	providers map[string]Provider
	store     authStorage
	grants    userGranter
}

func NewManager(cfg Config, st authStorage, grants userGranter) (*Manager, error) {
	providers, err := buildProviders(cfg)
	if err != nil {
		return nil, err
	}
	return &Manager{cfg: cfg, providers: providers, store: st, grants: grants}, nil
}

func (m *Manager) Disabled() bool {
	return m.cfg.Mode == ModeDisabled
}

func (m *Manager) Provider(name string) (Provider, bool) {
	p, ok := m.providers[name]
	return p, ok
}

func (m *Manager) Providers() []Provider {
	out := make([]Provider, 0, len(m.providers))
	for _, name := range m.cfg.EnabledProviders {
		if p, ok := m.providers[name]; ok {
			out = append(out, p)
		}
	}
	return out
}

func (m *Manager) Config() Config {
	return m.cfg
}

// Login upserts the user from an identity, creates a session, and returns both.
func (m *Manager) Login(ctx context.Context, identity *Identity, userAgent, ip string) (*domain.User, *domain.AuthSession, error) {
	user, err := m.store.UpsertUser(ctx, users.UpsertUserInput{
		ProviderSubjectKey: ProviderSubjectKey(identity.Provider, identity.Subject),
		Provider:           identity.Provider,
		Subject:            identity.Subject,
		DisplayName:        identity.DisplayName,
		AvatarURL:          identity.AvatarURL,
		Email:              identity.Email,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("upsert user: %w", err)
	}

	if err := m.applyNewUserPolicy(ctx, user); err != nil {
		return nil, nil, err
	}

	sessionID, err := generateSessionID()
	if err != nil {
		return nil, nil, err
	}
	sess, err := m.store.CreateSession(ctx, sessionID, user.ID, identity.Provider, m.cfg.SessionTTL, userAgent, ip)
	if err != nil {
		return nil, nil, fmt.Errorf("create session: %w", err)
	}
	return user, sess, nil
}

func (m *Manager) applyNewUserPolicy(ctx context.Context, user *domain.User) error {
	// TODO(tenancy): the registration tenant is hardcoded to #1 here. Plan 4 resolves
	// it from the global default_registration_tenant_id (or routes to onboarding).
	// Product keys moved to tenant_settings in migration 033, so read them scoped.
	scope := domain.TenantScope{TenantID: 1}

	// Read policy from tenant settings; fall back to env-var cfg for backward compat.
	policy := m.cfg.NewUserPolicy
	if raw, _ := m.store.GetTenantSetting(ctx, scope, "new_user_policy"); raw != nil {
		var p NewUserPolicy
		if json.Unmarshal(raw, &p) == nil && p != "" {
			policy = p
		}
	}
	if policy != PolicyDefaultNode {
		return nil
	}

	nodeID := m.cfg.DefaultNodeID
	if raw, _ := m.store.GetTenantSetting(ctx, scope, "default_hierarchy_node_id"); raw != nil {
		var id int64
		if json.Unmarshal(raw, &id) == nil && id != 0 {
			nodeID = id
		}
	}
	if nodeID == 0 {
		return nil
	}

	grants, err := m.grants.ListUserGrants(ctx, scope, user.ID)
	if err != nil {
		return err
	}
	if len(grants) > 0 {
		return nil
	}
	return m.grants.AddUserGrant(ctx, scope, user.ID, nodeID, domain.SystemUserAnonymous)
}

// ResolveSession loads the session and user by session ID.
func (m *Manager) ResolveSession(ctx context.Context, sessionID string) (*domain.User, *domain.AuthSession, error) {
	sess, err := m.store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, nil, err
	}
	user, err := m.store.GetUser(ctx, sess.UserID)
	if err != nil {
		return nil, nil, err
	}
	_ = m.store.TouchSession(ctx, sessionID)
	return user, sess, nil
}

// Logout deletes the session.
func (m *Manager) Logout(ctx context.Context, sessionID string) error {
	return m.store.DeleteSession(ctx, sessionID)
}

// CookieName returns the configured session cookie name.
func (m *Manager) CookieName() string {
	return m.cfg.SessionCookie
}

// SessionTTL returns the configured session TTL.
func (m *Manager) SessionTTL() time.Duration {
	return m.cfg.SessionTTL
}

func generateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
