// Package feedback serves the /api/v1/admin/… endpoints under its URI segment.
package feedback

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"okrs/internal/auth"
	"okrs/internal/http/handlers/api/v1/admin/admincommon"
	"okrs/internal/platform/logging"
)

type Handler struct {
	settings admincommon.TenantSettings
}

func New(settings admincommon.TenantSettings) *Handler { return &Handler{settings: settings} }

// GET /api/v1/admin/settings/feedback
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	scope, ok := auth.TenantScopeFromContext(ctx)
	if !ok {
		admincommon.WriteError(w, http.StatusForbidden, "no active tenant")
		return
	}
	admincommon.WriteJSON(w, map[string]any{
		"feedback_url":               admincommon.SettingString(ctx, h.settings, scope, admincommon.SettingKeyFeedbackURL),
		"feedback_popup_enabled":     admincommon.SettingBool(ctx, h.settings, scope, admincommon.SettingKeyFeedbackPopupEnabled),
		"feedback_menu_link_enabled": admincommon.SettingBool(ctx, h.settings, scope, admincommon.SettingKeyFeedbackMenuLinkEnabled),
		"feedback_frequency_days":    admincommon.SettingInt(ctx, h.settings, scope, admincommon.SettingKeyFeedbackFrequencyDays, 30),
	})
}

// POST /api/v1/admin/settings/feedback
// body: {"feedback_url":"https://...","feedback_popup_enabled":true,"feedback_menu_link_enabled":true,"feedback_frequency_days":30}
func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	var body struct {
		FeedbackURL             string `json:"feedback_url"`
		FeedbackPopupEnabled    bool   `json:"feedback_popup_enabled"`
		FeedbackMenuLinkEnabled bool   `json:"feedback_menu_link_enabled"`
		FeedbackFrequencyDays   int    `json:"feedback_frequency_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		admincommon.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	link := strings.TrimSpace(body.FeedbackURL)
	// No strict http(s) requirement here (unlike documentation_url) — any link is
	// accepted. The value is rendered into an href, so only block schemes that
	// could execute script (XSS), per the no-raw-HTML architecture rule.
	if link != "" && admincommon.HasUnsafeURLScheme(link) {
		admincommon.WriteError(w, http.StatusBadRequest, "feedback_url must not use a javascript:, data:, or vbscript: scheme")
		return
	}
	if body.FeedbackFrequencyDays < 1 {
		admincommon.WriteError(w, http.StatusBadRequest, "feedback_frequency_days must be >= 1")
		return
	}
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		admincommon.WriteError(w, http.StatusForbidden, "no active tenant")
		return
	}
	// Каждая запись фиксируется сразу после своего успеха: записи идут поочерёдно
	// и не в одной транзакции, поэтому отказ на последующей не должен отменять
	// запись о уже применённой.
	set := func(key string, val any) bool {
		if err := h.settings.SetTenantProduct(r.Context(), scope, key, val); err != nil {
			admincommon.WriteError(w, http.StatusInternalServerError, err.Error())
			return false
		}
		logging.AccessChanged(r.Context(), "tenant_setting_saved", slog.String("setting", key))
		return true
	}
	if !set(admincommon.SettingKeyFeedbackURL, link) ||
		!set(admincommon.SettingKeyFeedbackPopupEnabled, body.FeedbackPopupEnabled) ||
		!set(admincommon.SettingKeyFeedbackMenuLinkEnabled, body.FeedbackMenuLinkEnabled) ||
		!set(admincommon.SettingKeyFeedbackFrequencyDays, body.FeedbackFrequencyDays) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
