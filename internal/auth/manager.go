package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"okrs/internal/core/domain"
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
	AnySystemAdmin(ctx context.Context) (bool, error)
	SetSystemAdmin(ctx context.Context, userID int64, v bool) error
}

// Manager handles provider selection, session creation, and user upsert.
type Manager struct {
	cfg       Config
	providers map[string]Provider
	store     authStorage
}

func NewManager(cfg Config, st authStorage) (*Manager, error) {
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

	if err := m.maybeBootstrapSystemAdmin(ctx, identity, user); err != nil {
		return nil, nil, err
	}

	// New-user routing (membership in the registration tenant + new_user_policy) is handled
	// post-login by service.OnboardingService.EnsureRegistration, invoked from the OAuth callback.

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

// maybeBootstrapSystemAdmin promotes the configured bootstrap identity to system-admin
// on first matching login, but only while no system-admin exists yet. The identity is
// matched by provider:subject or by email (the operator may know only the email).
func (m *Manager) maybeBootstrapSystemAdmin(ctx context.Context, identity *Identity, user *domain.User) error {
	want := m.cfg.BootstrapSystemAdmin
	if want == "" || user.IsSystemAdmin {
		return nil
	}
	matches := want == ProviderSubjectKey(identity.Provider, identity.Subject) ||
		(identity.Email != "" && want == identity.Email)
	if !matches {
		return nil
	}
	exists, err := m.store.AnySystemAdmin(ctx)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if err := m.store.SetSystemAdmin(ctx, user.ID, true); err != nil {
		return err
	}
	user.IsSystemAdmin = true
	return nil
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
