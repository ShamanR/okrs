package hierarchy

import (
	"okrs/internal/http/dto"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/web/common"
	teamsvc "okrs/internal/service/team"
	okrboarduc "okrs/internal/usecase/okrboard"
)

func newHierarchyResponse(nodes []teamsvc.Node, metrics map[int64]okrboarduc.TeamSummary, userRefs map[string]*dto.UserRef, forecast int) dto.HierarchyResponse {
	return dto.HierarchyResponse{Items: mapHierarchyWithMetrics(nodes, metrics, userRefs, forecast)}
}

func mapHierarchyWithMetrics(nodes []teamsvc.Node, metrics map[int64]okrboarduc.TeamSummary, userRefs map[string]*dto.UserRef, forecast int) []dto.TeamNode {
	result := make([]dto.TeamNode, 0, len(nodes))
	for _, node := range nodes {
		result = append(result, mapTeamNode(node, metrics, userRefs, forecast))
	}
	return result
}

func mapTeamNode(node teamsvc.Node, metrics map[int64]okrboarduc.TeamSummary, userRefs map[string]*dto.UserRef, forecast int) dto.TeamNode {
	children := make([]dto.TeamNode, 0, len(node.Children))
	for _, child := range node.Children {
		children = append(children, mapTeamNode(child, metrics, userRefs, forecast))
	}
	var progress *int
	var forecastPtr *int
	status := ""
	hasGoals := false
	if summary, ok := metrics[node.Team.ID]; ok {
		status = string(summary.Status)
		hasGoals = summary.GoalsCount > 0
		if hasGoals {
			value := summary.PeriodProgress
			progress = &value
			f := forecast
			forecastPtr = &f
		}
	}
	return dto.TeamNode{
		ID:          node.Team.ID,
		Name:        node.Team.Name,
		Type:        string(node.Team.Type),
		TypeLabel:   common.TeamTypeLabel(node.Team.Type),
		Description: node.Team.Description,
		Lead:        v1.ResolveLeadByUDID(node.Team.LeadUDID, userRefs),
		HasGoals:    hasGoals,
		Progress:    progress,
		Forecast:    forecastPtr,
		Status:      status,
		Children:    children,
	}
}
