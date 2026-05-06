// Package google registers the Google OAuth2 provider.
// Import this package with a blank import to activate it:
//
//	import _ "okrs/internal/auth/providers/google"
package google

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"okrs/internal/auth"

	"golang.org/x/oauth2"
	googleoauth "golang.org/x/oauth2/google"
)

func init() {
	auth.Register("google", func(cfg auth.Config) (auth.Provider, error) {
		if cfg.Google.ClientID == "" || cfg.Google.ClientSecret == "" {
			return nil, fmt.Errorf("google: CLIENT_ID and CLIENT_SECRET are required")
		}
		return &provider{
			oauth2: &oauth2.Config{
				ClientID:     cfg.Google.ClientID,
				ClientSecret: cfg.Google.ClientSecret,
				RedirectURL:  cfg.Google.RedirectURL,
				Scopes:       []string{"openid", "profile", "email"},
				Endpoint:     googleoauth.Endpoint,
			},
		}, nil
	})
}

type provider struct {
	oauth2 *oauth2.Config
}

func (p *provider) Name() string        { return "google" }
func (p *provider) DisplayName() string { return "Google" }

func (p *provider) AuthURL(state string) string {
	return p.oauth2.AuthCodeURL(state, oauth2.AccessTypeOnline)
}

func (p *provider) Exchange(ctx context.Context, code string) (*auth.Identity, error) {
	token, err := p.oauth2.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("google exchange: %w", err)
	}
	client := p.oauth2.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v3/userinfo")
	if err != nil {
		return nil, fmt.Errorf("google userinfo: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google userinfo %d: %s", resp.StatusCode, body)
	}
	var info struct {
		Sub     string `json:"sub"`
		Name    string `json:"name"`
		Email   string `json:"email"`
		Picture string `json:"picture"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("google userinfo parse: %w", err)
	}
	return &auth.Identity{
		Provider:    "google",
		Subject:     info.Sub,
		DisplayName: info.Name,
		AvatarURL:   info.Picture,
		Email:       info.Email,
	}, nil
}
