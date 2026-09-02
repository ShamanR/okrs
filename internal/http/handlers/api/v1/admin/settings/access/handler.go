// Package access serves the /api/v1/admin/… endpoints under its URI segment.
package access

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"okrs/internal/auth"
	"okrs/internal/http/handlers/api/v1/admin/admincommon"
	"okrs/internal/platform/logging"
)

type Handler struct {
	settings admincommon.TenantSettings
}

func New(settings admincommon.TenantSettings) *Handler { return &Handler{settings: settings} }

// GET /api/v1/admin/settings/access
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		admincommon.WriteError(w, http.StatusForbidden, "no active tenant")
		return
	}
	policy, _ := h.settings.GetTenant(r.Context(), scope, "new_user_policy")
	nodeID, _ := h.settings.GetTenant(r.Context(), scope, "default_hierarchy_node_id")
	admincommon.WriteJSON(w, map[string]any{
		"new_user_policy":           json.RawMessage(policy),
		"default_hierarchy_node_id": json.RawMessage(nodeID),
	})
}

// POST /api/v1/admin/settings/access  body: {"new_user_policy":"default_node","default_hierarchy_node_id":5}
func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	var body struct {
		NewUserPolicy          string `json:"new_user_policy"`
		DefaultHierarchyNodeID *int64 `json:"default_hierarchy_node_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		admincommon.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		admincommon.WriteError(w, http.StatusForbidden, "no active tenant")
		return
	}
	// Каждое изменение фиксируется СРАЗУ после своего успеха, а не одной записью
	// в конце: записи идут поочерёдно и не в одной транзакции, так что отказ
	// на второй оставил бы первую применённой, но незапротоколированной.
	if body.NewUserPolicy != "" {
		if err := h.settings.SetTenantProduct(r.Context(), scope, "new_user_policy", body.NewUserPolicy); err != nil {
			admincommon.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		logging.AccessChanged(r.Context(), "tenant_setting_saved",
			slog.String("setting", "new_user_policy"),
			slog.String("value", body.NewUserPolicy))
	}
	if body.DefaultHierarchyNodeID != nil {
		if err := h.settings.SetTenantProduct(r.Context(), scope, "default_hierarchy_node_id", *body.DefaultHierarchyNodeID); err != nil {
			admincommon.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		logging.AccessChanged(r.Context(), "tenant_setting_saved",
			slog.String("setting", "default_hierarchy_node_id"),
			slog.Int64("value_id", *body.DefaultHierarchyNodeID))
	}
	w.WriteHeader(http.StatusNoContent)
}
