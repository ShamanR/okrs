package service

import (
	"context"
	"slices"
	"strings"

	"okrs/internal/core/domain"
	"okrs/internal/render/export"
)

type ExportParams struct {
	TeamID         int64
	PeriodID       int64
	GoalID         int64
	Scope          export.Scope
	Options        export.Options
	AllowedTeamIDs []int64 // nil = unrestricted (admin)
}

type ExportResult struct {
	Filename string
	Markdown string
	Lines    int
}

func (s *Service) ExportOKR(ctx context.Context, scope domain.TenantScope, p ExportParams) (ExportResult, error) {
	period, err := s.periods.GetPeriod(ctx, scope, p.PeriodID)
	if err != nil {
		return ExportResult{}, err
	}
	team, err := s.teams.GetTeam(ctx, scope, p.TeamID)
	if err != nil {
		return ExportResult{}, err
	}

	var blocks []export.TeamBlock
	switch p.Scope {
	case export.ScopeGoal:
		blocks, err = s.exportGoalBlocks(ctx, scope, team, period, p.GoalID)
	case export.ScopeTree:
		blocks, err = s.exportTreeBlocks(ctx, scope, team, period, p)
	default: // ScopeTeam
		blocks, err = s.exportTeamBlocks(ctx, scope, team, period)
	}
	if err != nil {
		return ExportResult{}, err
	}

	md := export.Markdown(period, blocks, p.Options)
	return ExportResult{
		Filename: export.Filename(period, p.Scope, team, p.GoalID),
		Markdown: md,
		Lines:    strings.Count(md, "\n"),
	}, nil
}

// exportTeamBlocks reuses GetTeamOKR (goals with KRs, notes, comments, progress).
func (s *Service) exportTeamBlocks(ctx context.Context, scope domain.TenantScope, team domain.Team, period domain.Period) ([]export.TeamBlock, error) {
	okrData, err := s.GetTeamOKR(ctx, scope, team.ID, period.ID, period)
	if err != nil {
		return nil, err
	}
	goals := make([]domain.Goal, 0, len(okrData.Goals))
	for _, gd := range okrData.Goals {
		goals = append(goals, gd.Goal)
	}
	return []export.TeamBlock{{Heading: team.Name, TeamID: team.ID, Goals: goals}}, nil
}

// exportGoalBlocks filters a single goal out of the team's board (guarantees board membership + access).
func (s *Service) exportGoalBlocks(ctx context.Context, scope domain.TenantScope, team domain.Team, period domain.Period, goalID int64) ([]export.TeamBlock, error) {
	okrData, err := s.GetTeamOKR(ctx, scope, team.ID, period.ID, period)
	if err != nil {
		return nil, err
	}
	for _, gd := range okrData.Goals {
		if gd.Goal.ID == goalID {
			return []export.TeamBlock{{Heading: team.Name, TeamID: team.ID, Goals: []domain.Goal{gd.Goal}}}, nil
		}
	}
	return nil, ErrGoalNotOnTeamBoard
}

func (s *Service) exportTreeBlocks(ctx context.Context, scope domain.TenantScope, team domain.Team, period domain.Period, p ExportParams) ([]export.TeamBlock, error) {
	hierarchy, err := s.GetHierarchy(ctx, scope, &period.ID)
	if err != nil {
		return nil, err
	}
	// Strip nodes outside the caller's grant scope before deriving paths, so headings never leak
	// the names of inaccessible ancestors (matches the hierarchy endpoint's scope filtering).
	// nil AllowedTeamIDs means admin/unrestricted — no filtering.
	if p.AllowedTeamIDs != nil {
		hierarchy = filterNodesByScope(hierarchy, p.AllowedTeamIDs)
	}
	// team + descendants, in DFS order, intersected with allowed scope.
	ordered := orderedSubtreeIDs(team.ID, hierarchy)
	teamsByID, pathByID := indexTeamsWithPaths(hierarchy)
	teamIDs := make([]int64, 0, len(ordered))
	for _, id := range ordered {
		if allowedContains(p.AllowedTeamIDs, id) {
			teamIDs = append(teamIDs, id)
		}
	}
	goalsByTeam, err := s.goals.ListGoalsByTeamsPeriod(ctx, scope, period.ID, teamIDs)
	if err != nil {
		return nil, err
	}

	// Collect KR / goal IDs for batched notes/comments/ownership.
	var krIDs, goalIDs []int64
	for _, gs := range goalsByTeam {
		for gi := range gs {
			g := &gs[gi]
			goalIDs = append(goalIDs, g.ID)
			for ki := range g.KeyResults {
				krIDs = append(krIDs, g.KeyResults[ki].ID)
			}
		}
	}
	// The batched board loader reports the board team as goal.TeamID; recover the true owner
	// so the formatter can render a shared goal as a reference under non-owner teams.
	ownerByGoal, err := s.goals.ListGoalOwnerTeamIDs(ctx, scope, goalIDs)
	if err != nil {
		return nil, err
	}
	var notes map[int64]*domain.KeyResultNote
	if p.Options.Format == export.FormatFull && len(krIDs) > 0 {
		if notes, err = s.krs.BatchLoadNotes(ctx, scope, krIDs); err != nil {
			return nil, err
		}
	}
	var comments map[int64][]domain.GoalComment
	if p.Options.Comments && len(goalIDs) > 0 {
		if comments, err = s.goals.ListGoalCommentsByGoals(ctx, scope, goalIDs); err != nil {
			return nil, err
		}
	}

	included := make(map[int64]bool, len(teamIDs))
	for _, id := range teamIDs {
		included[id] = true
	}

	// A goal is rendered fully exactly once. If its owner team has its own block in this export,
	// non-owner teams show a reference (the owner block carries the full body). If the owner is
	// outside the exported/accessible subtree, no owner block exists, so the goal is rendered
	// fully at its first visible occurrence and referenced only in later ones.
	renderedFull := make(map[int64]bool)
	blocks := make([]export.TeamBlock, 0, len(teamIDs))
	for _, id := range teamIDs {
		goals := goalsByTeam[id]
		var refGoals map[int64]string
		for gi := range goals {
			g := &goals[gi]
			// compute progress (batched loader does not set it)
			for ki := range g.KeyResults {
				g.KeyResults[ki].Progress = CalculateKRProgress(g.KeyResults[ki])
			}
			g.Progress = CalculateGoalProgress(g)
			if notes != nil {
				for ki := range g.KeyResults {
					g.KeyResults[ki].Note = notes[g.KeyResults[ki].ID]
				}
			}
			if comments != nil {
				g.Comments = comments[g.ID]
			}
			owner := ownerByGoal[g.ID]
			var asRef bool
			if included[owner] {
				asRef = owner != id // owner block renders full; others reference it
			} else {
				asRef = renderedFull[g.ID] // no owner block: full once, reference after
			}
			if asRef {
				if refGoals == nil {
					refGoals = make(map[int64]string)
				}
				// Owner name only when the owner team is part of the export (never leak an
				// inaccessible team's name); blank falls back to a generic reference.
				refGoals[g.ID] = teamsByID[owner].Name
			} else {
				renderedFull[g.ID] = true
			}
		}
		blocks = append(blocks, export.TeamBlock{
			Heading:  strings.Join(pathByID[id], " / "),
			TeamID:   id,
			Goals:    goals,
			RefGoals: refGoals,
		})
	}
	return blocks, nil
}

// filterNodesByScope removes tree nodes not in allowedIDs and promotes accessible children of
// removed nodes to their parent's level, so the result is a valid forest rooted at the caller's
// access boundary. Mirrors the same logic in the hierarchy API handler.
func filterNodesByScope(nodes []TeamNode, allowedIDs []int64) []TeamNode {
	result := make([]TeamNode, 0, len(nodes))
	for _, node := range nodes {
		filteredChildren := filterNodesByScope(node.Children, allowedIDs)
		if slices.Contains(allowedIDs, node.Team.ID) {
			node.Children = filteredChildren
			result = append(result, node)
		} else {
			result = append(result, filteredChildren...)
		}
	}
	return result
}

func allowedContains(allowed []int64, id int64) bool {
	if allowed == nil {
		return true // unrestricted (admin)
	}
	for _, a := range allowed {
		if a == id {
			return true
		}
	}
	return false
}

// orderedSubtreeIDs returns the subtree rooted at rootID (inclusive) in DFS order.
func orderedSubtreeIDs(rootID int64, nodes []TeamNode) []int64 {
	var out []int64
	var walk func(items []TeamNode, inSubtree bool)
	walk = func(items []TeamNode, inSubtree bool) {
		for _, n := range items {
			here := inSubtree || n.Team.ID == rootID
			if here {
				out = append(out, n.Team.ID)
			}
			walk(n.Children, here)
		}
	}
	walk(nodes, false)
	return out
}

// indexTeamsWithPaths returns maps id->team and id->path (root→node names).
func indexTeamsWithPaths(nodes []TeamNode) (map[int64]domain.Team, map[int64][]string) {
	teams := map[int64]domain.Team{}
	paths := map[int64][]string{}
	var walk func(items []TeamNode, prefix []string)
	walk = func(items []TeamNode, prefix []string) {
		for _, n := range items {
			p := append(append([]string{}, prefix...), n.Team.Name)
			teams[n.Team.ID] = n.Team
			paths[n.Team.ID] = p
			walk(n.Children, p)
		}
	}
	walk(nodes, nil)
	return teams, paths
}
