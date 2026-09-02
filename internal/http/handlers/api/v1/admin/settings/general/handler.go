// Package general serves the /api/v1/admin/… endpoints under its URI segment.
package general

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
	renamer  admincommon.TenantRenamer
	settings admincommon.TenantSettings
}

func New(renamer admincommon.TenantRenamer, settings admincommon.TenantSettings) *Handler {
	return &Handler{renamer: renamer, settings: settings}
}

// GET /api/v1/admin/settings/general
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		admincommon.WriteError(w, http.StatusForbidden, "no active tenant")
		return
	}
	name := ""
	if t := auth.TenantFromContext(r.Context()); t != nil {
		name = t.Name
	}
	admincommon.WriteJSON(w, map[string]any{
		"name":                            name,
		"documentation_url":               admincommon.SettingString(r.Context(), h.settings, scope, admincommon.SettingKeyDocumentationURL),
		"empty_hierarchy_message":         admincommon.SettingString(r.Context(), h.settings, scope, admincommon.SettingKeyEmptyHierarchyMessage),
		"progress_snapshot_interval_days": admincommon.SettingInt(r.Context(), h.settings, scope, admincommon.SettingKeyProgressSnapshotIntervalDays, 1),
	})
}

// POST /api/v1/admin/settings/general  body: {"documentation_url":"https://..."}
func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name                  string `json:"name"`
		DocumentationURL      string `json:"documentation_url"`
		EmptyHierarchyMessage string `json:"empty_hierarchy_message"`
		// Optional: omitting it preserves the stored value (a client saving unrelated
		// general settings must not reset a configured snapshot interval).
		ProgressSnapshotIntervalDays *int `json:"progress_snapshot_interval_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		admincommon.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		admincommon.WriteError(w, http.StatusBadRequest, "name must not be empty")
		return
	}
	link := strings.TrimSpace(body.DocumentationURL)
	if link != "" && !admincommon.IsValidHTTPURL(link) {
		admincommon.WriteError(w, http.StatusBadRequest, "documentation_url must be a valid http(s) URL")
		return
	}
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		admincommon.WriteError(w, http.StatusForbidden, "no active tenant")
		return
	}
	// Каждое изменение фиксируется СРАЗУ после своего успеха, а не одной записью
	// в конце: записи идут поочерёдно и не в одной транзакции, поэтому отказ на
	// любой из последующих оставил бы уже применённое переименование организации
	// незапротоколированным — а это как раз то изменение, о котором аудит должен
	// узнать в первую очередь.
	if err := h.renamer.RenameTenant(r.Context(), scope.TenantID, name); err != nil {
		admincommon.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	logging.AccessChanged(r.Context(), "tenant_renamed")

	if err := h.settings.SetTenantProduct(r.Context(), scope, admincommon.SettingKeyDocumentationURL, link); err != nil {
		admincommon.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	logging.AccessChanged(r.Context(), "tenant_setting_saved",
		slog.String("setting", admincommon.SettingKeyDocumentationURL))

	if err := h.settings.SetTenantProduct(r.Context(), scope, admincommon.SettingKeyEmptyHierarchyMessage, body.EmptyHierarchyMessage); err != nil {
		admincommon.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	logging.AccessChanged(r.Context(), "tenant_setting_saved",
		slog.String("setting", admincommon.SettingKeyEmptyHierarchyMessage))

	if body.ProgressSnapshotIntervalDays != nil {
		snapshotDays := *body.ProgressSnapshotIntervalDays
		if snapshotDays < 1 {
			snapshotDays = 1
		}
		if err := h.settings.SetTenantProduct(r.Context(), scope, admincommon.SettingKeyProgressSnapshotIntervalDays, snapshotDays); err != nil {
			admincommon.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		logging.AccessChanged(r.Context(), "tenant_setting_saved",
			slog.String("setting", admincommon.SettingKeyProgressSnapshotIntervalDays),
			slog.Int("value", snapshotDays))
	}
	w.WriteHeader(http.StatusNoContent)
}
