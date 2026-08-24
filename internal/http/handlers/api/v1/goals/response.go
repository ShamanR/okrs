package goals

import (
	"okrs/internal/core/domain"
	"okrs/internal/http/dto"
	v1 "okrs/internal/http/handlers/api/v1"
)

func newGoalResponse(goal domain.Goal, userRefs map[string]*dto.UserRef) dto.GoalResponse {
	comments := make([]dto.GoalComment, 0, len(goal.Comments))
	for _, comment := range goal.Comments {
		comments = append(comments, v1.MapGoalComment(comment))
	}
	krList := make([]dto.KeyResult, 0, len(goal.KeyResults))
	for _, kr := range goal.KeyResults {
		krList = append(krList, v1.MapKeyResult(kr))
	}
	goalDetail := dto.GoalDetails{
		ID:          goal.ID,
		TeamID:      goal.TeamID,
		PeriodID:    goal.PeriodID,
		Title:       goal.Title,
		Description: goal.Description,
		Priority:    string(goal.Priority),
		Weight:      goal.Weight,
		WorkType:    string(goal.WorkType),
		FocusType:   string(goal.FocusType),
		Owners:      v1.ResolveOwnersByUDIDs(goal.OwnerUDIDs, goal.OwnerText, userRefs),
		Progress:    goal.Progress,
		KeyResults:  krList,
		Parents:     v1.MapGoalRefs(goal.Parents),
		Children:    v1.MapGoalRefs(goal.Children),
		CreatedAt:   goal.CreatedAt,
		UpdatedAt:   goal.UpdatedAt,
	}
	return dto.GoalResponse{Goal: goalDetail, Comments: comments}
}
