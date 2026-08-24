// Package team is the team entity service: CRUD over the team repository plus the
// in-memory hierarchy it builds from them. It touches exactly one repository and never
// writes the activity journal — anything that orchestrates more than teams is a usecase.
package team

import (
	"context"
	"sort"

	"okrs/internal/core/domain"
	"okrs/internal/store/teams"
)

// Service is the team entity service.
type Service struct {
	repo Repo
}

func New(repo Repo) *Service { return &Service{repo: repo} }

type Repo interface {
	ListTeams(ctx context.Context, scope domain.TenantScope) ([]domain.Team, error)
	ListDeletedTeams(ctx context.Context, scope domain.TenantScope) ([]domain.Team, error)
	ListAllTeams(ctx context.Context, scope domain.TenantScope) ([]domain.Team, error)
	GetTeam(ctx context.Context, scope domain.TenantScope, id int64) (domain.Team, error)
	CreateTeam(ctx context.Context, scope domain.TenantScope, input teams.TeamInput) (int64, error)
	UpdateTeam(ctx context.Context, scope domain.TenantScope, input teams.TeamInput, id int64) error
	TeamHasGoals(ctx context.Context, scope domain.TenantScope, id int64) (bool, error)
	TeamHasGoalsInPeriod(ctx context.Context, scope domain.TenantScope, id, periodID int64) (bool, error)
	ListTeamIDsWithGoalsInPeriod(ctx context.Context, scope domain.TenantScope, periodID int64) (map[int64]struct{}, error)
	SoftDeleteTeam(ctx context.Context, scope domain.TenantScope, id int64) error
	RestoreTeam(ctx context.Context, scope domain.TenantScope, id int64) error
	HardDeleteTeam(ctx context.Context, scope domain.TenantScope, id int64) error
}

type Node struct {
	Team     domain.Team
	Children []Node
}

func (s *Service) Hierarchy(ctx context.Context, scope domain.TenantScope, periodID *int64) ([]Node, error) {
	allTeams, err := s.repo.ListAllTeams(ctx, scope)
	if err != nil {
		return nil, err
	}
	return s.HierarchyFromTeams(ctx, scope, allTeams, periodID)
}
func (s *Service) HierarchyFromTeams(ctx context.Context, scope domain.TenantScope, allTeams []domain.Team, periodID *int64) ([]Node, error) {
	teamIDsWithGoals := map[int64]struct{}{}
	if periodID != nil && *periodID > 0 {
		ids, err := s.repo.ListTeamIDsWithGoalsInPeriod(ctx, scope, *periodID)
		if err != nil {
			return nil, err
		}
		teamIDsWithGoals = ids
	}
	visibleTeams := make([]domain.Team, 0, len(allTeams))
	for _, team := range allTeams {
		if team.DeletedAt == nil {
			visibleTeams = append(visibleTeams, team)
			continue
		}
		if periodID == nil || *periodID == 0 {
			continue
		}
		_, hasGoals := teamIDsWithGoals[team.ID]
		if hasGoals {
			visibleTeams = append(visibleTeams, team)
		}
	}
	_, childrenMap, roots := BuildHierarchy(visibleTeams)
	nodes := make([]Node, 0, len(roots))
	for _, team := range roots {
		node := BuildNode(team, childrenMap)
		nodes = append(nodes, node)
	}
	return nodes, nil
}
func (s *Service) Get(ctx context.Context, scope domain.TenantScope, teamID int64) (domain.Team, error) {
	return s.repo.GetTeam(ctx, scope, teamID)
}
func (s *Service) Delete(ctx context.Context, scope domain.TenantScope, teamID int64) error {
	hasGoals, err := s.repo.TeamHasGoals(ctx, scope, teamID)
	if err != nil {
		return err
	}
	if hasGoals {
		return s.repo.SoftDeleteTeam(ctx, scope, teamID)
	}
	return s.repo.HardDeleteTeam(ctx, scope, teamID)
}
func (s *Service) Restore(ctx context.Context, scope domain.TenantScope, teamID int64) error {
	return s.repo.RestoreTeam(ctx, scope, teamID)
}
func (s *Service) HardDelete(ctx context.Context, scope domain.TenantScope, teamID int64) error {
	hasGoals, err := s.repo.TeamHasGoals(ctx, scope, teamID)
	if err != nil {
		return err
	}
	if hasGoals {
		return domain.ErrTeamHasGoals
	}
	return s.repo.HardDeleteTeam(ctx, scope, teamID)
}
func (s *Service) VisibleInPeriod(ctx context.Context, scope domain.TenantScope, team domain.Team, periodID int64) (bool, error) {
	hasGoals, err := s.repo.TeamHasGoalsInPeriod(ctx, scope, team.ID, periodID)
	if err != nil {
		return false, err
	}
	if hasGoals {
		return true, nil
	}
	return team.DeletedAt == nil, nil
}
func (s *Service) List(ctx context.Context, scope domain.TenantScope) ([]domain.Team, error) {
	return s.repo.ListTeams(ctx, scope)
}
func (s *Service) ListDeleted(ctx context.Context, scope domain.TenantScope) ([]domain.Team, error) {
	return s.repo.ListDeletedTeams(ctx, scope)
}
func (s *Service) ListAll(ctx context.Context, scope domain.TenantScope) ([]domain.Team, error) {
	return s.repo.ListAllTeams(ctx, scope)
}
func (s *Service) Create(ctx context.Context, scope domain.TenantScope, input teams.TeamInput) (int64, error) {
	return s.repo.CreateTeam(ctx, scope, input)
}
func (s *Service) Update(ctx context.Context, scope domain.TenantScope, input teams.TeamInput, id int64) error {
	return s.repo.UpdateTeam(ctx, scope, input, id)
}
func BuildHierarchy(allTeams []domain.Team) (map[int64]domain.Team, map[int64][]domain.Team, []domain.Team) {
	teamsByID := make(map[int64]domain.Team, len(allTeams))
	childrenMap := make(map[int64][]domain.Team)
	roots := make([]domain.Team, 0)
	for _, team := range allTeams {
		teamsByID[team.ID] = team
	}
	for _, team := range allTeams {
		if team.ParentID != nil {
			if _, ok := teamsByID[*team.ParentID]; ok {
				childrenMap[*team.ParentID] = append(childrenMap[*team.ParentID], team)
				continue
			}
		}
		roots = append(roots, team)
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i].Name < roots[j].Name })
	return teamsByID, childrenMap, roots
}
func BuildNode(team domain.Team, childrenMap map[int64][]domain.Team) Node {
	children := childrenMap[team.ID]
	sort.Slice(children, func(i, j int) bool { return children[i].Name < children[j].Name })
	node := Node{Team: team}
	for _, child := range children {
		node.Children = append(node.Children, BuildNode(child, childrenMap))
	}
	return node
}
func FindDirectChildren(targetID int64, nodes []Node) []Node {
	var children []Node
	var walk func(items []Node) bool
	walk = func(items []Node) bool {
		for _, node := range items {
			if node.Team.ID == targetID {
				children = node.Children
				return true
			}
			if walk(node.Children) {
				return true
			}
		}
		return false
	}
	_ = walk(nodes)
	return children
}
func CollectDescendantIDs(targetID int64, nodes []Node) []int64 {
	var descendants []int64
	var walk func(items []Node, collect bool)
	walk = func(items []Node, collect bool) {
		for _, node := range items {
			nextCollect := collect || node.Team.ID == targetID
			if collect {
				descendants = append(descendants, node.Team.ID)
			}
			if len(node.Children) > 0 {
				walk(node.Children, nextCollect)
			}
		}
	}
	walk(nodes, false)
	return descendants
}
