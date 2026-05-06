package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"okrs/internal/domain"
	"okrs/internal/store"
)

// userGranter is the minimal interface Manager needs for the new-user grant policy.
// Both *store.Store and *store.GrantsCache satisfy it.
type userGranter interface {
	ListUserGrants(ctx context.Context, userID int64) ([]store.HierarchyGrant, error)
	AddUserGrant(ctx context.Context, userID, teamID, grantedByUserID int64) error
}

// Manager handles provider selection, session creation, and user upsert.
type Manager struct {
	cfg       Config
	providers map[string]Provider
	store     *store.Store
	grants    userGranter
}

func NewManager(cfg Config, st *store.Store, grants userGranter) (*Manager, error) {
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
	user, err := m.store.UpsertUser(ctx, store.UpsertUserInput{
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
	// Read policy from DB settings; fall back to env-var cfg for backward compat.
	policy := m.cfg.NewUserPolicy
	if raw, _ := m.store.GetSetting(ctx, "new_user_policy"); raw != nil {
		var p NewUserPolicy
		if json.Unmarshal(raw, &p) == nil && p != "" {
			policy = p
		}
	}
	if policy != PolicyDefaultNode {
		return nil
	}

	nodeID := m.cfg.DefaultNodeID
	if raw, _ := m.store.GetSetting(ctx, "default_hierarchy_node_id"); raw != nil {
		var id int64
		if json.Unmarshal(raw, &id) == nil && id != 0 {
			nodeID = id
		}
	}
	if nodeID == 0 {
		return nil
	}

	grants, err := m.grants.ListUserGrants(ctx, user.ID)
	if err != nil {
		return err
	}
	if len(grants) > 0 {
		return nil
	}
	return m.grants.AddUserGrant(ctx, user.ID, nodeID, domain.SystemUserAnonymous)
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
