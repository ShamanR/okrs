// Package keycloak registers the Keycloak OIDC provider.
// Import this package with a blank import to activate it:
//
//	import _ "okrs/internal/auth/providers/keycloak"
package keycloak

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"okrs/internal/auth"

	"golang.org/x/oauth2"
)

func init() {
	auth.Register("keycloak", func(cfg auth.Config) (auth.Provider, error) {
		kc := cfg.Keycloak
		if kc.IssuerURL == "" || kc.ClientID == "" || kc.ClientSecret == "" {
			return nil, fmt.Errorf("keycloak: ISSUER_URL, CLIENT_ID and CLIENT_SECRET are required")
		}
		issuer := strings.TrimRight(kc.IssuerURL, "/")
		return &provider{
			issuer: issuer,
			oauth2: &oauth2.Config{
				ClientID:     kc.ClientID,
				ClientSecret: kc.ClientSecret,
				RedirectURL:  kc.RedirectURL,
				Scopes:       []string{"openid", "profile", "email"},
				Endpoint: oauth2.Endpoint{
					AuthURL:  issuer + "/protocol/openid-connect/auth",
					TokenURL: issuer + "/protocol/openid-connect/token",
				},
			},
		}, nil
	})
}

type provider struct {
	issuer string
	oauth2 *oauth2.Config
}

func (p *provider) Name() string        { return "keycloak" }
func (p *provider) DisplayName() string { return "Keycloak" }

func (p *provider) AuthURL(state string) string {
	return p.oauth2.AuthCodeURL(state, oauth2.AccessTypeOnline)
}

func (p *provider) Exchange(ctx context.Context, code string) (*auth.Identity, error) {
	token, err := p.oauth2.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("keycloak exchange: %w", err)
	}
	client := p.oauth2.Client(ctx, token)
	resp, err := client.Get(p.issuer + "/protocol/openid-connect/userinfo")
	if err != nil {
		return nil, fmt.Errorf("keycloak userinfo: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("keycloak userinfo %d: %s", resp.StatusCode, body)
	}
	var info struct {
		Sub               string `json:"sub"`
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
		Email             string `json:"email"`
		Picture           string `json:"picture"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("keycloak userinfo parse: %w", err)
	}
	displayName := info.Name
	if displayName == "" {
		displayName = info.PreferredUsername
	}
	return &auth.Identity{
		Provider:    "keycloak",
		Subject:     info.Sub,
		DisplayName: displayName,
		AvatarURL:   info.Picture,
		Email:       info.Email,
	}, nil
}
