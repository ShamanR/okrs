package auth

import "time"

type Mode string

const (
	ModeDisabled Mode = "disabled"
	ModeEnabled  Mode = "enabled"
)

type NewUserPolicy string

const (
	PolicyEmpty       NewUserPolicy = "empty"
	PolicyDefaultNode NewUserPolicy = "default_node"
)

type Config struct {
	Mode             Mode
	EnabledProviders []string
	SessionCookie    string
	SessionTTL       time.Duration
	BaseURL          string
	NewUserPolicy    NewUserPolicy
	DefaultNodeID    int64

	// ProvisioningToken, when set, authorizes machine callers of /api/v1/system/*
	// via "Authorization: Bearer <token>" (instance-level, like the DSN).
	ProvisioningToken string
	// BootstrapSystemAdmin (provider:subject or email) is promoted to system-admin
	// on first matching login when no system-admin exists yet.
	BootstrapSystemAdmin string

	Google   OAuthConfig
	GitHub   OAuthConfig
	Keycloak KeycloakConfig
}

type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

type KeycloakConfig struct {
	IssuerURL    string
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

func DefaultConfig() Config {
	return Config{
		Mode:          ModeDisabled,
		SessionCookie: "okrs_session",
		SessionTTL:    720 * time.Hour,
	}
}
