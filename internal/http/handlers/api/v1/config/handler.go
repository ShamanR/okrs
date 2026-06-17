// Package config exposes public client-facing configuration that the SPA needs
// at boot (e.g. the optional documentation link). It is readable by any
// authenticated user, unlike the admin settings endpoints.
package config

import (
	"context"
	"encoding/json"
	"net/http"

	"okrs/internal/service"
)

const (
	settingKeyDocumentationURL        = "documentation_url"
	settingKeyFeedbackURL             = "feedback_url"
	settingKeyFeedbackPopupEnabled    = "feedback_popup_enabled"
	settingKeyFeedbackMenuLinkEnabled = "feedback_menu_link_enabled"
	settingKeyFeedbackFrequencyDays   = "feedback_frequency_days"
)

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
	// StaleDays drives the "N дней без обновлений" warning on goal pages; it
	// mirrors the Health Check-in threshold so both stay in sync.
	StaleDays int `json:"stale_days"`
	// BehindMargin is the lag tolerance (п.п.) from the Health Check-in "Отстающие"
	// category; the sidebar colors team progress red when progress < forecast - behind_margin.
	BehindMargin int `json:"behind_margin"`
	// Feedback collection config consumed by the shared header (menu item + nudge).
	FeedbackURL             string `json:"feedback_url"`
	FeedbackPopupEnabled    bool   `json:"feedback_popup_enabled"`
	FeedbackMenuLinkEnabled bool   `json:"feedback_menu_link_enabled"`
	FeedbackFrequencyDays   int    `json:"feedback_frequency_days"`
}

// GET /api/v1/config
func (h *Handler) HandleConfig(w http.ResponseWriter, r *http.Request) {
	cfg, _ := service.LoadHealthCheckInConfig(r.Context(), h.settings)
	resp := configResponse{
		DocumentationURL: h.documentationURL(r.Context()),
		StaleDays:        cfg.StaleDays,
		BehindMargin:     cfg.BehindMargin,
	}
	resp.FeedbackURL = h.settingString(r.Context(), settingKeyFeedbackURL)
	resp.FeedbackPopupEnabled = h.settingBool(r.Context(), settingKeyFeedbackPopupEnabled)
	resp.FeedbackMenuLinkEnabled = h.settingBool(r.Context(), settingKeyFeedbackMenuLinkEnabled)
	resp.FeedbackFrequencyDays = h.settingInt(r.Context(), settingKeyFeedbackFrequencyDays, 30)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *Handler) documentationURL(ctx context.Context) string {
	return h.settingString(ctx, settingKeyDocumentationURL)
}

func (h *Handler) settingString(ctx context.Context, key string) string {
	raw, err := h.settings.GetSetting(ctx, key)
	if err != nil || raw == nil {
		return ""
	}
	var s string
	_ = json.Unmarshal(raw, &s)
	return s
}

func (h *Handler) settingBool(ctx context.Context, key string) bool {
	raw, err := h.settings.GetSetting(ctx, key)
	if err != nil || raw == nil {
		return false
	}
	var b bool
	_ = json.Unmarshal(raw, &b)
	return b
}

// settingInt returns def when the value is unset, malformed, or < 1.
func (h *Handler) settingInt(ctx context.Context, key string, def int) int {
	raw, err := h.settings.GetSetting(ctx, key)
	if err != nil || raw == nil {
		return def
	}
	var n int
	if json.Unmarshal(raw, &n) != nil || n < 1 {
		return def
	}
	return n
}
