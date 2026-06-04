// Package config exposes public client-facing configuration that the SPA needs
// at boot (e.g. the optional documentation link). It is readable by any
// authenticated user, unlike the admin settings endpoints.
package config

import (
	"context"
	"encoding/json"
	"net/http"
)

const settingKeyDocumentationURL = "documentation_url"

// settingsReader is satisfied by *store.SettingsRepository.
type settingsReader interface {
	GetSetting(ctx context.Context, key string) (json.RawMessage, error)
}

type Handler struct {
	settings settingsReader
}

func New(settings settingsReader) *Handler {
	return &Handler{settings: settings}
}

type configResponse struct {
	DocumentationURL string `json:"documentation_url"`
}

// GET /api/v1/config
func (h *Handler) HandleConfig(w http.ResponseWriter, r *http.Request) {
	resp := configResponse{DocumentationURL: h.documentationURL(r.Context())}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *Handler) documentationURL(ctx context.Context) string {
	raw, err := h.settings.GetSetting(ctx, settingKeyDocumentationURL)
	if err != nil || raw == nil {
		return ""
	}
	var link string
	_ = json.Unmarshal(raw, &link)
	return link
}
