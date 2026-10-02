// Package teamscommon holds the team→DTO mapping and the UDID collection every
// /api/v1/teams/** endpoint shares. A leaf package: the parent mounts the sub-packages,
// so importing it back would be an import cycle.
package teamscommon

import (
	"okrs/internal/http/dto"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/web/common"
	okrboarduc "okrs/internal/usecase/okrboard"
)

func TeamOKRResponse(data okrboarduc.TeamOKR, userRefs map[string]*dto.UserRef) dto.TeamOKRResponse {
	goals := make([]dto.GoalDetails, 0, len(data.Goals))
	for _, goal := range data.Goals {
		goals = append(goals, v1.MapGoalDetails(goal, data.Period, userRefs))
	}
	return dto.TeamOKRResponse{
		Team: dto.TeamInfo{
			ID: data.Team.ID, Name: data.Team.Name,
			Type: string(data.Team.Type), TypeLabel: common.TeamTypeLabel(data.Team.Type),
			Description: data.Team.Description,
			Lead:        v1.ResolveLeadByUDID(data.Team.LeadUDID, userRefs),
			ParentID:    data.Team.ParentID,
		},
		Period:          v1.MapPeriodInfo(data.Period),
		PeriodStatus:    string(data.PeriodStatus),
		StatusLabel:     common.TeamPeriodStatusLabel(data.PeriodStatus),
		StatusChangedAt: data.StatusChangedAt,
		PeriodProgress:  data.PeriodProgress,
		GoalsCount:      data.GoalsCount,
		GoalsWeight:     data.GoalsWeight,
		ProgressMeta:    v1.BuildProgressBarInfo(data.PeriodProgress, data.Period),
		Goals:           goals,
	}
}

func CollectOKRUserUDIDs(okr okrboarduc.TeamOKR) []string {
	seen := make(map[string]struct{})
	if okr.Team.LeadUDID != nil {
		seen[*okr.Team.LeadUDID] = struct{}{}
	}
	for _, g := range okr.Goals {
		for _, uid := range g.Goal.OwnerUDIDs {
			seen[uid] = struct{}{}
		}
	}
	udids := make([]string, 0, len(seen))
	for uid := range seen {
		udids = append(udids, uid)
	}
	return udids
}
