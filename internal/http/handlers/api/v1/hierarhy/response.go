package hierarhy

import (
	"okrs/internal/http/dto"
	"okrs/internal/http/handlers/web/common"
	"okrs/internal/service"
)

func newHierarchyResponse(nodes []service.TeamNode, metrics map[int64]service.TeamSummary) dto.HierarchyResponse {
	return dto.HierarchyResponse{Items: mapHierarchyWithMetrics(nodes, metrics)}
}

func mapHierarchyWithMetrics(nodes []service.TeamNode, metrics map[int64]service.TeamSummary) []dto.TeamNode {
	result := make([]dto.TeamNode, 0, len(nodes))
	for _, node := range nodes {
		result = append(result, mapTeamNode(node, metrics))
	}
	return result
}

func mapTeamNode(node service.TeamNode, metrics map[int64]service.TeamSummary) dto.TeamNode {
	children := make([]dto.TeamNode, 0, len(node.Children))
	for _, child := range node.Children {
		children = append(children, mapTeamNode(child, metrics))
	}
	var progress *int
	hasGoals := false
	if summary, ok := metrics[node.Team.ID]; ok {
		hasGoals = summary.GoalsCount > 0
		if hasGoals {
			value := summary.PeriodProgress
			progress = &value
		}
	}
	return dto.TeamNode{
		ID:        node.Team.ID,
		Name:      node.Team.Name,
		Type:      string(node.Team.Type),
		TypeLabel: common.TeamTypeLabel(node.Team.Type),
		Lead:      node.Team.Lead,
		HasGoals:  hasGoals,
		Progress:  progress,
		Children:  children,
	}
}
