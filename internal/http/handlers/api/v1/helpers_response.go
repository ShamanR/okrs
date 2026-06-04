package v1

import (
	"strings"
	"time"

	"okrs/internal/domain"
	"okrs/internal/http/dto"
	"okrs/internal/http/handlers/web/common"
	"okrs/internal/service"
)

// BuildUserRefMap builds a udid→UserRef lookup from a slice of users.
func BuildUserRefMap(users []*domain.User) map[string]*dto.UserRef {
	m := make(map[string]*dto.UserRef, len(users))
	for _, u := range users {
		if u.UDID == "" {
			continue
		}
		ref := &dto.UserRef{UDID: u.UDID, DisplayName: u.DisplayName, AvatarURL: u.AvatarURL}
		m[u.UDID] = ref
	}
	return m
}

// ResolveUserRef returns a *dto.UserRef for name, or nil if name is empty or not in the map.
func ResolveUserRef(name string, refs map[string]*dto.UserRef) *dto.UserRef {
	if name == "" {
		return nil
	}
	if refs != nil {
		if ref, ok := refs[name]; ok {
			return ref
		}
	}
	return &dto.UserRef{DisplayName: name}
}

// ResolveOwners splits a comma-separated owner_text and resolves each name to a UserRef.
func ResolveOwners(ownerText string, refs map[string]*dto.UserRef) []dto.UserRef {
	if ownerText == "" {
		return nil
	}
	parts := strings.Split(ownerText, ",")
	out := make([]dto.UserRef, 0, len(parts))
	for _, p := range parts {
		name := strings.TrimSpace(p)
		if name == "" {
			continue
		}
		if refs != nil {
			if ref, ok := refs[name]; ok {
				out = append(out, *ref)
				continue
			}
		}
		out = append(out, dto.UserRef{DisplayName: name})
	}
	return out
}

// ResolveOwnersByUDIDs resolves owner_udids to UserRef list using the UDID-keyed refs map.
// For each UDID not found in refs, returns a placeholder with just the UDID.
// Falls back to ResolveOwners(ownerText, refs) when ownerUDIDs is empty (no-auth mode).
func ResolveOwnersByUDIDs(ownerUDIDs []string, ownerText string, refs map[string]*dto.UserRef) []dto.UserRef {
	if len(ownerUDIDs) == 0 {
		return ResolveOwners(ownerText, refs)
	}
	out := make([]dto.UserRef, 0, len(ownerUDIDs))
	for _, uid := range ownerUDIDs {
		if refs != nil {
			if ref, ok := refs[uid]; ok {
				out = append(out, *ref)
				continue
			}
		}
		out = append(out, dto.UserRef{UDID: uid, DisplayName: "Удалённый пользователь"})
	}
	return out
}

// ResolveLeadByUDID looks up a team lead by UDID in the UDID-keyed refs map.
// Returns nil when leadUDID is nil or the user is not found (e.g. deleted).
func ResolveLeadByUDID(leadUDID *string, refs map[string]*dto.UserRef) *dto.UserRef {
	if leadUDID == nil || *leadUDID == "" {
		return nil
	}
	if refs != nil {
		if ref, ok := refs[*leadUDID]; ok {
			return ref
		}
	}
	return nil
}

func MapPeriodInfo(period domain.Period) dto.PeriodInfo {
	return dto.PeriodInfo{
		ID:        period.ID,
		Name:      period.Name,
		StartDate: period.StartDate,
		EndDate:   period.EndDate,
		SortOrder: period.SortOrder,
	}
}

func BuildProgressBarInfo(actual int, period domain.Period) dto.ProgressBarInfo {
	forecast := CalculatePeriodForecast(period, time.Now())
	delta := actual - forecast
	status := "on_track"
	if delta > 10 {
		status = "above"
	} else if delta < -10 {
		status = "below"
	}
	return dto.ProgressBarInfo{Actual: actual, Forecast: forecast, Delta: delta, Status: status}
}

func CalculatePeriodForecast(period domain.Period, now time.Time) int {
	if period.EndDate.Before(period.StartDate) {
		return 0
	}
	if now.Before(period.StartDate) {
		return 0
	}
	if now.After(period.EndDate) {
		return 100
	}
	duration := period.EndDate.Sub(period.StartDate)
	if duration <= 0 {
		return 100
	}
	elapsed := now.Sub(period.StartDate)
	value := int((elapsed * 100) / duration)
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func MapGoalDetails(detail service.GoalDetails, period domain.Period, userRefs map[string]*dto.UserRef) dto.GoalDetails {
	krList := make([]dto.KeyResult, 0, len(detail.Goal.KeyResults))
	for _, kr := range detail.Goal.KeyResults {
		krList = append(krList, MapKeyResult(kr))
	}
	shareTeams := make([]dto.ShareTeam, 0, len(detail.ShareTeams))
	for _, share := range detail.ShareTeams {
		shareTeams = append(shareTeams, dto.ShareTeam{
			ID:        share.ID,
			Name:      share.Name,
			Type:      string(share.Type),
			TypeLabel: common.TeamTypeLabel(share.Type),
			Weight:    share.Weight,
		})
	}
	comments := make([]dto.GoalComment, 0, len(detail.Goal.Comments))
	for _, c := range detail.Goal.Comments {
		comments = append(comments, dto.GoalComment{ID: c.ID, Text: c.Text, AuthorName: c.AuthorName, AuthorUDID: c.AuthorUDID, CreatedAt: c.CreatedAt})
	}
	goal := detail.Goal
	return dto.GoalDetails{
		ID:           goal.ID,
		TeamID:       goal.TeamID,
		PeriodID:     goal.PeriodID,
		Title:        goal.Title,
		Description:  goal.Description,
		Priority:     string(goal.Priority),
		Weight:       goal.Weight,
		WorkType:     string(goal.WorkType),
		FocusType:    string(goal.FocusType),
		Owners:       ResolveOwnersByUDIDs(goal.OwnerUDIDs, goal.OwnerText, userRefs),
		Progress:     goal.Progress,
		ProgressMeta: BuildProgressBarInfo(goal.Progress, period),
		KeyResults:   krList,
		ShareTeams:   shareTeams,
		Comments:     comments,
		CreatedAt:    goal.CreatedAt,
		UpdatedAt:    goal.UpdatedAt,
	}
}

func MapKeyResult(kr domain.KeyResult) dto.KeyResult {
	var note *dto.KRNote
	if kr.Note != nil {
		note = &dto.KRNote{
			Text:       kr.Note.Text,
			AuthorName: kr.Note.AuthorName,
			AuthorUDID: kr.Note.AuthorUDID,
			UpdatedAt:  kr.Note.UpdatedAt,
		}
	}
	return dto.KeyResult{
		ID:          kr.ID,
		GoalID:      kr.GoalID,
		Title:       kr.Title,
		Description: kr.Description,
		Weight:      kr.Weight,
		Kind:        string(kr.Kind),
		Progress:    kr.Progress,
		Measure:     buildMeasure(kr),
		Note:        note,
		CreatedAt:   kr.CreatedAt,
		UpdatedAt:   kr.UpdatedAt,
	}
}

func buildMeasure(kr domain.KeyResult) dto.Measure {
	switch kr.Kind {
	case domain.KRKindPercent:
		if kr.Percent == nil {
			return dto.Measure{Kind: string(kr.Kind)}
		}
		checkpoints := make([]dto.PercentCheckpoint, 0, len(kr.Percent.Checkpoints))
		for _, cp := range kr.Percent.Checkpoints {
			checkpoints = append(checkpoints, dto.PercentCheckpoint{ID: cp.ID, MetricValue: cp.MetricValue, Percent: cp.KRPercent})
		}
		return dto.Measure{Kind: string(kr.Kind), Percent: &dto.PercentMeasure{StartValue: kr.Percent.StartValue, TargetValue: kr.Percent.TargetValue, CurrentValue: kr.Percent.CurrentValue}, Checkpoints: checkpoints}
	case domain.KRKindLinear:
		if kr.Linear == nil {
			return dto.Measure{Kind: string(kr.Kind)}
		}
		return dto.Measure{Kind: string(kr.Kind), Linear: &dto.LinearMeasure{StartValue: kr.Linear.StartValue, TargetValue: kr.Linear.TargetValue, CurrentValue: kr.Linear.CurrentValue}}
	case domain.KRKindBoolean:
		if kr.Boolean == nil {
			return dto.Measure{Kind: string(kr.Kind)}
		}
		return dto.Measure{Kind: string(kr.Kind), Boolean: &dto.BooleanMeasure{IsDone: kr.Boolean.IsDone}}
	case domain.KRKindProject:
		if kr.Project == nil {
			return dto.Measure{Kind: string(kr.Kind)}
		}
		stages := make([]dto.ProjectStage, 0, len(kr.Project.Stages))
		for _, stage := range kr.Project.Stages {
			stages = append(stages, dto.ProjectStage{ID: stage.ID, Title: stage.Title, Weight: stage.Weight, IsDone: stage.IsDone})
		}
		return dto.Measure{Kind: string(kr.Kind), Project: &dto.ProjectMeasure{Stages: stages}}
	default:
		return dto.Measure{Kind: string(kr.Kind)}
	}
}
