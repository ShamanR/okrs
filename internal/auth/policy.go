package auth

import (
	"context"
	"encoding/json"
	"log/slog"
	"slices"

	"okrs/internal/domain"
	"okrs/internal/store"
)

type policyContextKey int

const allowedTeamsKey policyContextKey = 0

// PolicyEvaluator resolves and caches per-request team access scope.
type PolicyEvaluator struct {
	store  *store.Store
	logger *slog.Logger
}

func NewPolicyEvaluator(st *store.Store, logger *slog.Logger) *PolicyEvaluator {
	return &PolicyEvaluator{store: st, logger: logger}
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
// Admin users get nil (unrestricted). Returns enriched context.
func (e *PolicyEvaluator) LoadScope(ctx context.Context, user *domain.User, cfg Config) (context.Context, error) {
	if user == nil || user.IsAdmin {
		return context.WithValue(ctx, allowedTeamsKey, []int64(nil)), nil
	}

	grants, err := e.store.ListUserGrants(ctx, user.ID)
	if err != nil {
		return ctx, err
	}

	rootIDs := make([]int64, 0, len(grants))

	// Also check if there's a configured default node
	if cfg.NewUserPolicy == PolicyDefaultNode && cfg.DefaultNodeID != 0 {
		rootIDs = append(rootIDs, cfg.DefaultNodeID)
	}

	for _, g := range grants {
		if !slices.Contains(rootIDs, g.TeamID) {
			rootIDs = append(rootIDs, g.TeamID)
		}
	}

	allIDs, err := e.store.ListDescendantTeamIDs(ctx, rootIDs)
	if err != nil {
		return ctx, err
	}

	return context.WithValue(ctx, allowedTeamsKey, allIDs), nil
}

// CanAccessTeam returns true if the user may access the given team.
func (e *PolicyEvaluator) CanAccessTeam(ctx context.Context, teamID int64) bool {
	ids, ok := ctx.Value(allowedTeamsKey).([]int64)
	if !ok {
		return false
	}
	if ids == nil {
		return true // admin: unrestricted
	}
	return slices.Contains(ids, teamID)
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
