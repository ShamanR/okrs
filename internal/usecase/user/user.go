// Package user holds user-facing scenarios that cross entity boundaries. Scope-aware
// search is the only one: it intersects the caller's grants with the team hierarchy
// before asking the user service, which is why it cannot live in that service.
package user

import (
	"context"

	"okrs/internal/core/domain"
	teamsvc "okrs/internal/service/team"
	usersvc "okrs/internal/service/user"
	"okrs/internal/store/grants"
)

// GrantsProvider gives the usecase the cached user_hierarchy_grants snapshot.
type GrantsProvider interface {
	AllGrants(ctx context.Context) (map[int64][]grants.HierarchyGrant, error)
}

type Deps struct {
	Users  *usersvc.Service
	Teams  *teamsvc.Service
	Grants GrantsProvider
}

type UseCase struct {
	users  *usersvc.Service
	teams  *teamsvc.Service
	grants GrantsProvider
}

func New(deps Deps) *UseCase {
	return &UseCase{users: deps.Users, teams: deps.Teams, grants: deps.Grants}
}

// SearchUsersInScope returns up to 20 non-system users visible in the given scope.
//   - scopeTeamIDs == nil → admin/unrestricted: all users
//   - scopeTeamIDs != nil → users with a hierarchy grant to any team related to the scope nodes:
//     ancestors (access from above), the nodes themselves, or descendants (access from below).
//
// Uses the GrantsProvider cache; falls back to empty result when cache is unavailable.
func (s *UseCase) SearchInScope(ctx context.Context, scope domain.TenantScope, scopeTeamIDs []int64, q string, limit int) ([]*domain.User, error) {
	if limit <= 0 {
		limit = 20
	}
	if scopeTeamIDs == nil {
		return s.users.SearchUnrestricted(ctx, q, limit)
	}
	if len(scopeTeamIDs) == 0 || s.grants == nil {
		return nil, nil
	}

	allTeams, err := s.teams.ListAll(ctx, scope)
	if err != nil {
		return nil, err
	}

	// Build both maps for bidirectional tree traversal.
	parentMap := make(map[int64]int64, len(allTeams))
	childrenMap := make(map[int64][]int64, len(allTeams))
	for _, t := range allTeams {
		if t.ParentID != nil {
			parentMap[t.ID] = *t.ParentID
			childrenMap[*t.ParentID] = append(childrenMap[*t.ParentID], t.ID)
		}
	}

	// Related set: scope nodes + all their ancestors + all their descendants.
	relatedSet := make(map[int64]struct{})
	for _, id := range scopeTeamIDs {
		// Walk up.
		cur := id
		for {
			relatedSet[cur] = struct{}{}
			parent, ok := parentMap[cur]
			if !ok {
				break
			}
			cur = parent
		}
		// Walk down via BFS.
		queue := []int64{id}
		for len(queue) > 0 {
			cur, queue = queue[0], queue[1:]
			for _, child := range childrenMap[cur] {
				if _, visited := relatedSet[child]; !visited {
					relatedSet[child] = struct{}{}
					queue = append(queue, child)
				}
			}
		}
	}

	allGrants, err := s.grants.AllGrants(ctx)
	if err != nil {
		return nil, err
	}

	// Collect IDs of users whose grants intersect the related set.
	eligibleIDs := make([]int64, 0)
	seen := make(map[int64]struct{})
	for userID, userGrants := range allGrants {
		for _, g := range userGrants {
			if _, ok := relatedSet[g.TeamID]; ok {
				if _, dup := seen[userID]; !dup {
					seen[userID] = struct{}{}
					eligibleIDs = append(eligibleIDs, userID)
				}
				break
			}
		}
	}

	// Team leads of all related nodes are eligible regardless of explicit grants.
	leadUDIDs := make([]string, 0)
	for _, t := range allTeams {
		if _, ok := relatedSet[t.ID]; ok && t.LeadUDID != nil && t.DeletedAt == nil {
			leadUDIDs = append(leadUDIDs, *t.LeadUDID)
		}
	}

	return s.users.SearchInSet(ctx, eligibleIDs, leadUDIDs, q, limit)
}
