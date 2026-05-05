package hierarhy

import (
	"okrs/internal/http/dto"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/web/common"
	"okrs/internal/service"
)

func newHierarchyResponse(nodes []service.TeamNode, metrics map[int64]service.TeamSummary, userRefs map[string]*dto.UserRef) dto.HierarchyResponse {
	return dto.HierarchyResponse{Items: mapHierarchyWithMetrics(nodes, metrics, userRefs)}
}

func mapHierarchyWithMetrics(nodes []service.TeamNode, metrics map[int64]service.TeamSummary, userRefs map[string]*dto.UserRef) []dto.TeamNode {
	result := make([]dto.TeamNode, 0, len(nodes))
	for _, node := range nodes {
		result = append(result, mapTeamNode(node, metrics, userRefs))
	}
	return result
}

func mapTeamNode(node service.TeamNode, metrics map[int64]service.TeamSummary, userRefs map[string]*dto.UserRef) dto.TeamNode {
	children := make([]dto.TeamNode, 0, len(node.Children))
	for _, child := range node.Children {
		children = append(children, mapTeamNode(child, metrics, userRefs))
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
		Lead:      v1.ResolveUserRef(node.Team.Lead, userRefs),
		HasGoals:  hasGoals,
		Progress:  progress,
		Children:  children,
	}
}
