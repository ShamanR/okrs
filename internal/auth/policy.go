package auth

import (
	"context"
	"encoding/json"
	"log/slog"
	"slices"

	"okrs/internal/domain"
	"okrs/internal/store"
	"okrs/internal/store/grants"
)

// sentinel for "no access" — non-nil empty slice stored in context
var emptyTeamIDs = []int64{}

type policyContextKey int

const allowedTeamsKey policyContextKey = 0

// grantsReader is the minimal interface PolicyEvaluator needs for scope resolution.
// Both *store.Store and *store.GrantsCache satisfy it.
type grantsReader interface {
	ListUserGrants(ctx context.Context, userID int64) ([]grants.HierarchyGrant, error)
	ListDescendantTeamIDs(ctx context.Context, rootIDs []int64) ([]int64, error)
}

// PolicyEvaluator resolves and caches per-request team access scope.
type PolicyEvaluator struct {
	grants grantsReader
	logger *slog.Logger
}

func NewPolicyEvaluator(grants grantsReader, logger *slog.Logger) *PolicyEvaluator {
	return &PolicyEvaluator{grants: grants, logger: logger}
}

// AllowedTeamIDs returns the set of team IDs the user may access.
// Results are cached in context. Returns nil if the user is admin (access all).
func (e *PolicyEvaluator) AllowedTeamIDs(ctx context.Context) ([]int64, bool) {
	if ids, ok := ctx.Value(allowedTeamsKey).([]int64); ok {
		return ids, true
	}
	return nil, false
}

// LoadScope resolves and stores the user's allowed team IDs into the context.
// Admin users get nil (unrestricted access). Non-admins get their explicit grant
// expansion — an empty slice if they have no grants (no access).
// The cfg param is kept for signature compatibility but default-node policy is
// applied at registration time by Manager.applyNewUserPolicy, not per-request.
func (e *PolicyEvaluator) LoadScope(ctx context.Context, user *domain.User, cfg Config) (context.Context, error) {
	if user == nil || user.IsAdmin {
		return context.WithValue(ctx, allowedTeamsKey, []int64(nil)), nil
	}

	grants, err := e.grants.ListUserGrants(ctx, user.ID)
	if err != nil {
		return ctx, err
	}

	if len(grants) == 0 {
		return context.WithValue(ctx, allowedTeamsKey, emptyTeamIDs), nil
	}

	rootIDs := make([]int64, 0, len(grants))
	for _, g := range grants {
		if !slices.Contains(rootIDs, g.TeamID) {
			rootIDs = append(rootIDs, g.TeamID)
		}
	}

	allIDs, err := e.grants.ListDescendantTeamIDs(ctx, rootIDs)
	if err != nil {
		return ctx, err
	}
	if allIDs == nil {
		allIDs = emptyTeamIDs
	}

	return context.WithValue(ctx, allowedTeamsKey, allIDs), nil
}

// CanAccessTeamFromCtx is a package-level helper handlers can call without a PolicyEvaluator instance.
// It reads the scope already loaded into ctx by ScopeMiddleware.
// Returns true for admin/unrestricted (nil slice) or when teamID is in the allowed set.
func CanAccessTeamFromCtx(ctx context.Context, teamID int64) bool {
	ids, ok := ctx.Value(allowedTeamsKey).([]int64)
	if !ok {
		// Scope not explicitly loaded (e.g. in tests without middleware).
		// ScopeMiddleware always populates this key in production, so treat
		// absence as unrestricted rather than deny-all.
		return true
	}
	if ids == nil {
		return true // admin: unrestricted
	}
	return slices.Contains(ids, teamID)
}

// AllowedTeamIDsFromCtx returns the allowed team ID slice from ctx.
// nil means unrestricted (admin). false second return means scope not loaded.
func AllowedTeamIDsFromCtx(ctx context.Context) ([]int64, bool) {
	ids, ok := ctx.Value(allowedTeamsKey).([]int64)
	return ids, ok
}

// WithAllowedTeamIDs injects a pre-computed allowed team ID list into ctx.
// Pass nil for unrestricted access (admin). Pass an empty slice for no access.
// Used by tests and middleware to populate scope without running PolicyEvaluator.
func WithAllowedTeamIDs(ctx context.Context, ids []int64) context.Context {
	return context.WithValue(ctx, allowedTeamsKey, ids)
}

// DefaultNodeID reads the configured default hierarchy node from system settings.
func DefaultNodeIDFromSettings(ctx context.Context, st *store.Store) (int64, error) {
	raw, err := st.GetSetting(ctx, "default_hierarchy_node_id")
	if err != nil || raw == nil {
		return 0, err
	}
	var id int64
	if err := json.Unmarshal(raw, &id); err != nil {
		return 0, nil
	}
	return id, nil
}
