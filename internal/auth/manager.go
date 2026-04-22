package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"okrs/internal/domain"
	"okrs/internal/store"
)

// Manager handles provider selection, session creation, and user upsert.
type Manager struct {
	cfg       Config
	providers map[string]Provider
	store     *store.Store
}

func NewManager(cfg Config, st *store.Store) (*Manager, error) {
	providers, err := buildProviders(cfg)
	if err != nil {
		return nil, err
	}
	return &Manager{cfg: cfg, providers: providers, store: st}, nil
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
	if m.cfg.NewUserPolicy != PolicyDefaultNode || m.cfg.DefaultNodeID == 0 {
		return nil
	}
	grants, err := m.store.ListUserGrants(ctx, user.ID)
	if err != nil {
		return err
	}
	if len(grants) > 0 {
		return nil
	}
	return m.store.AddUserGrant(ctx, user.ID, m.cfg.DefaultNodeID, domain.SystemUserAnonymous)
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
