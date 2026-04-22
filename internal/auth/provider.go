package auth

import "context"

// Provider is the interface every OAuth/OIDC provider must implement.
type Provider interface {
	Name() string
	DisplayName() string
	// AuthURL returns the redirect URL that starts the OAuth flow.
	AuthURL(state string) string
	// Exchange completes the callback phase and returns a normalised Identity.
	Exchange(ctx context.Context, code string) (*Identity, error)
}

// Identity is the normalised external user representation returned by any provider.
type Identity struct {
	Provider    string
	Subject     string
	DisplayName string
	AvatarURL   string
	Email       string
}

// ProviderSubjectKey builds the unique key used in the users table.
func ProviderSubjectKey(provider, subject string) string {
	return provider + ":" + subject
}
