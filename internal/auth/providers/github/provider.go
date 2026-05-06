// Package github registers the GitHub OAuth2 provider.
// Import this package with a blank import to activate it:
//
//	import _ "okrs/internal/auth/providers/github"
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"okrs/internal/auth"

	"golang.org/x/oauth2"
	githuboauth "golang.org/x/oauth2/github"
)

func init() {
	auth.Register("github", func(cfg auth.Config) (auth.Provider, error) {
		if cfg.GitHub.ClientID == "" || cfg.GitHub.ClientSecret == "" {
			return nil, fmt.Errorf("github: CLIENT_ID and CLIENT_SECRET are required")
		}
		return &provider{
			oauth2: &oauth2.Config{
				ClientID:     cfg.GitHub.ClientID,
				ClientSecret: cfg.GitHub.ClientSecret,
				RedirectURL:  cfg.GitHub.RedirectURL,
				Scopes:       []string{"read:user", "user:email"},
				Endpoint:     githuboauth.Endpoint,
			},
		}, nil
	})
}

type provider struct {
	oauth2 *oauth2.Config
}

func (p *provider) Name() string        { return "github" }
func (p *provider) DisplayName() string { return "GitHub" }

func (p *provider) AuthURL(state string) string {
	return p.oauth2.AuthCodeURL(state, oauth2.AccessTypeOnline)
}

func (p *provider) Exchange(ctx context.Context, code string) (*auth.Identity, error) {
	token, err := p.oauth2.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("github exchange: %w", err)
	}
	client := p.oauth2.Client(ctx, token)

	userResp, err := doGitHubRequest(client, "https://api.github.com/user")
	if err != nil {
		return nil, err
	}
	var user struct {
		ID        int    `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
		Email     string `json:"email"`
	}
	if err := json.Unmarshal(userResp, &user); err != nil {
		return nil, fmt.Errorf("github user parse: %w", err)
	}

	email := user.Email
	if email == "" {
		email = fetchGitHubPrimaryEmail(client)
	}
	displayName := user.Name
	if displayName == "" {
		displayName = user.Login
	}
	return &auth.Identity{
		Provider:    "github",
		Subject:     fmt.Sprintf("%d", user.ID),
		DisplayName: displayName,
		AvatarURL:   user.AvatarURL,
		Email:       email,
	}, nil
}

func doGitHubRequest(client *http.Client, url string) ([]byte, error) {
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github request %s: %w", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github request %s %d: %s", url, resp.StatusCode, body)
	}
	return body, nil
}

func fetchGitHubPrimaryEmail(client *http.Client) string {
	body, err := doGitHubRequest(client, "https://api.github.com/user/emails")
	if err != nil {
		return ""
	}
	var emails []struct {
		Email   string `json:"email"`
		Primary bool   `json:"primary"`
	}
	if err := json.Unmarshal(body, &emails); err != nil {
		return ""
	}
	for _, e := range emails {
		if e.Primary {
			return e.Email
		}
	}
	return ""
}
