package v1

import (
	"time"

	"okrs/internal/domain"
	"okrs/internal/http/handlers/common"
	"okrs/internal/service"
)

type hierarchyResponse struct {
	Items []teamNode `json:"items"`
}

type teamNode struct {
	ID        int64      `json:"id"`
	Name      string     `json:"name"`
	Type      string     `json:"type"`
	TypeLabel string     `json:"type_label"`
	Children  []teamNode `json:"children"`
}

type teamsResponse struct {
	Year    int           `json:"year"`
	Quarter int           `json:"quarter"`
	Items   []teamSummary `json:"items"`
}

type teamSummary struct {
	ID              int64             `json:"id"`
	Name            string            `json:"name"`
	Type            string            `json:"type"`
	TypeLabel       string            `json:"type_label"`
	Indent          int               `json:"indent"`
	Status          string            `json:"status"`
	StatusLabel     string            `json:"status_label"`
	QuarterProgress int               `json:"quarter_progress"`
	GoalsCount      int               `json:"goals_count"`
	GoalsWeight     int               `json:"goals_weight"`
	Goals           []teamGoalSummary `json:"goals"`
}

type teamGoalSummary struct {
	ID         int64       `json:"id"`
	Title      string      `json:"title"`
	Weight     int         `json:"weight"`
	Progress   int         `json:"progress"`
	ShareTeams []shareTeam `json:"share_teams"`
	Priority   string      `json:"priority"`
}

type shareTeam struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	TypeLabel string `json:"type_label"`
	Weight    int    `json:"weight"`
}

type teamOKRResponse struct {
	Team            teamInfo      `json:"team"`
	Year            int           `json:"year"`
	Quarter         int           `json:"quarter"`
	QuarterStatus   string        `json:"quarter_status"`
	StatusLabel     string        `json:"status_label"`
	QuarterProgress int           `json:"quarter_progress"`
	GoalsCount      int           `json:"goals_count"`
	GoalsWeight     int           `json:"goals_weight"`
	Goals           []goalDetails `json:"goals"`
}

type teamInfo struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	TypeLabel string `json:"type_label"`
	ParentID  *int64 `json:"parent_id,omitempty"`
}

type goalDetails struct {
	ID          int64       `json:"id"`
	TeamID      int64       `json:"team_id"`
	Year        int         `json:"year"`
	Quarter     int         `json:"quarter"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Priority    string      `json:"priority"`
	Weight      int         `json:"weight"`
	WorkType    string      `json:"work_type"`
	FocusType   string      `json:"focus_type"`
	OwnerText   string      `json:"owner_text"`
	Progress    int         `json:"progress"`
	KeyResults  []keyResult `json:"key_results"`
	ShareTeams  []shareTeam `json:"share_teams"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

type keyResult struct {
	ID          int64       `json:"id"`
	GoalID      int64       `json:"goal_id"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Weight      int         `json:"weight"`
	Kind        string      `json:"kind"`
	Progress    int         `json:"progress"`
	Measure     measure     `json:"measure"`
	Comments    []krComment `json:"comments"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

type krComment struct {
	ID        int64     `json:"id"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

type goalComment struct {
	ID        int64     `json:"id"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

type goalResponse struct {
	Goal     goalDetails   `json:"goal"`
	Comments []goalComment `json:"comments"`
}

type measure struct {
	Kind        string              `json:"kind"`
	Percent     *percentMeasure     `json:"percent,omitempty"`
	Linear      *linearMeasure      `json:"linear,omitempty"`
	Boolean     *booleanMeasure     `json:"boolean,omitempty"`
	Project     *projectMeasure     `json:"project,omitempty"`
	Checkpoints []percentCheckpoint `json:"checkpoints,omitempty"`
}

type percentMeasure struct {
	StartValue   float64 `json:"start_value"`
	TargetValue  float64 `json:"target_value"`
	CurrentValue float64 `json:"current_value"`
}

type linearMeasure struct {
	StartValue   float64 `json:"start_value"`
	TargetValue  float64 `json:"target_value"`
	CurrentValue float64 `json:"current_value"`
}

type booleanMeasure struct {
	IsDone bool `json:"is_done"`
}

type projectMeasure struct {
	Stages []projectStage `json:"stages"`
}

type projectStage struct {
	ID     int64  `json:"id"`
	Title  string `json:"title"`
	Weight int    `json:"weight"`
	IsDone bool   `json:"is_done"`
}

type percentCheckpoint struct {
	ID          int64   `json:"id"`
	MetricValue float64 `json:"metric_value"`
	Percent     int     `json:"percent"`
}

func mapHierarchy(nodes []service.TeamNode) []teamNode {
	result := make([]teamNode, 0, len(nodes))
	for _, node := range nodes {
		result = append(result, mapTeamNode(node))
	}
	return result
}

func mapTeamNode(node service.TeamNode) teamNode {
	children := make([]teamNode, 0, len(node.Children))
	for _, child := range node.Children {
		children = append(children, mapTeamNode(child))
	}
	return teamNode{
		ID:        node.Team.ID,
		Name:      node.Team.Name,
		Type:      string(node.Team.Type),
		TypeLabel: common.TeamTypeLabel(node.Team.Type),
		Children:  children,
	}
}

func mapTeamsResponse(year, quarter int, teams []service.TeamSummary) teamsResponse {
	items := make([]teamSummary, 0, len(teams))
	for _, team := range teams {
		goals := make([]teamGoalSummary, 0, len(team.Goals))
		for _, goal := range team.Goals {
			shareTeams := make([]shareTeam, 0, len(goal.ShareTeams))
			for _, share := range goal.ShareTeams {
				shareTeams = append(shareTeams, shareTeam{
					ID:        share.ID,
					Name:      share.Name,
					Type:      string(share.Type),
					TypeLabel: common.TeamTypeLabel(share.Type),
					Weight:    share.Weight,
				})
			}
			goals = append(goals, teamGoalSummary{
				ID:         goal.ID,
				Title:      goal.Title,
				Weight:     goal.Weight,
				Progress:   goal.Progress,
				ShareTeams: shareTeams,
				Priority:   goal.Priority,
			})
		}
		items = append(items, teamSummary{
			ID:              team.ID,
			Name:            team.Name,
			Type:            string(team.Type),
			TypeLabel:       common.TeamTypeLabel(team.Type),
			Indent:          team.Indent,
			Status:          string(team.Status),
			StatusLabel:     common.TeamQuarterStatusLabel(team.Status),
			QuarterProgress: team.QuarterProgress,
			GoalsCount:      team.GoalsCount,
			GoalsWeight:     team.GoalsWeight,
			Goals:           goals,
		})
	}
	return teamsResponse{Year: year, Quarter: quarter, Items: items}
}

func mapTeamOKRResponse(data service.TeamOKR) teamOKRResponse {
	goals := make([]goalDetails, 0, len(data.Goals))
	for _, goal := range data.Goals {
		goals = append(goals, mapGoalDetails(goal))
	}
	return teamOKRResponse{
		Team: teamInfo{
			ID:        data.Team.ID,
			Name:      data.Team.Name,
			Type:      string(data.Team.Type),
			TypeLabel: common.TeamTypeLabel(data.Team.Type),
			ParentID:  data.Team.ParentID,
		},
		Year:            data.Year,
		Quarter:         data.Quarter,
		QuarterStatus:   string(data.QuarterStatus),
		StatusLabel:     common.TeamQuarterStatusLabel(data.QuarterStatus),
		QuarterProgress: data.QuarterProgress,
		GoalsCount:      data.GoalsCount,
		GoalsWeight:     data.GoalsWeight,
		Goals:           goals,
	}
}

func mapGoalDetails(detail service.GoalDetails) goalDetails {
	krList := make([]keyResult, 0, len(detail.Goal.KeyResults))
	for _, kr := range detail.Goal.KeyResults {
		krList = append(krList, mapKeyResult(kr))
	}
	shareTeams := make([]shareTeam, 0, len(detail.ShareTeams))
	for _, share := range detail.ShareTeams {
		shareTeams = append(shareTeams, shareTeam{
			ID:        share.ID,
			Name:      share.Name,
			Type:      string(share.Type),
			TypeLabel: common.TeamTypeLabel(share.Type),
			Weight:    share.Weight,
		})
	}
	goal := detail.Goal
	return goalDetails{
		ID:          goal.ID,
		TeamID:      goal.TeamID,
		Year:        goal.Year,
		Quarter:     goal.Quarter,
		Title:       goal.Title,
		Description: goal.Description,
		Priority:    string(goal.Priority),
		Weight:      goal.Weight,
		WorkType:    string(goal.WorkType),
		FocusType:   string(goal.FocusType),
		OwnerText:   goal.OwnerText,
		Progress:    goal.Progress,
		KeyResults:  krList,
		ShareTeams:  shareTeams,
		CreatedAt:   goal.CreatedAt,
		UpdatedAt:   goal.UpdatedAt,
	}
}

func mapGoalResponse(goal domain.Goal) goalResponse {
	comments := make([]goalComment, 0, len(goal.Comments))
	for _, comment := range goal.Comments {
		comments = append(comments, goalComment{ID: comment.ID, Text: comment.Text, CreatedAt: comment.CreatedAt})
	}
	krList := make([]keyResult, 0, len(goal.KeyResults))
	for _, kr := range goal.KeyResults {
		krList = append(krList, mapKeyResult(kr))
	}
	goalDetail := goalDetails{
		ID:          goal.ID,
		TeamID:      goal.TeamID,
		Year:        goal.Year,
		Quarter:     goal.Quarter,
		Title:       goal.Title,
		Description: goal.Description,
		Priority:    string(goal.Priority),
		Weight:      goal.Weight,
		WorkType:    string(goal.WorkType),
		FocusType:   string(goal.FocusType),
		OwnerText:   goal.OwnerText,
		Progress:    goal.Progress,
		KeyResults:  krList,
		CreatedAt:   goal.CreatedAt,
		UpdatedAt:   goal.UpdatedAt,
	}
	return goalResponse{Goal: goalDetail, Comments: comments}
}

func mapKeyResult(kr domain.KeyResult) keyResult {
	comments := make([]krComment, 0, len(kr.Comments))
	for _, comment := range kr.Comments {
		comments = append(comments, krComment{ID: comment.ID, Text: comment.Text, CreatedAt: comment.CreatedAt})
	}
	return keyResult{
		ID:          kr.ID,
		GoalID:      kr.GoalID,
		Title:       kr.Title,
		Description: kr.Description,
		Weight:      kr.Weight,
		Kind:        string(kr.Kind),
		Progress:    kr.Progress,
		Measure:     buildMeasure(kr),
		Comments:    comments,
		CreatedAt:   kr.CreatedAt,
		UpdatedAt:   kr.UpdatedAt,
	}
}

func buildMeasure(kr domain.KeyResult) measure {
	switch kr.Kind {
	case domain.KRKindPercent:
		if kr.Percent == nil {
			return measure{Kind: string(kr.Kind)}
		}
		checkpoints := make([]percentCheckpoint, 0, len(kr.Percent.Checkpoints))
		for _, cp := range kr.Percent.Checkpoints {
			checkpoints = append(checkpoints, percentCheckpoint{
				ID:          cp.ID,
				MetricValue: cp.MetricValue,
				Percent:     cp.KRPercent,
			})
		}
		return measure{
			Kind:        string(kr.Kind),
			Percent:     &percentMeasure{StartValue: kr.Percent.StartValue, TargetValue: kr.Percent.TargetValue, CurrentValue: kr.Percent.CurrentValue},
			Checkpoints: checkpoints,
		}
	case domain.KRKindLinear:
		if kr.Linear == nil {
			return measure{Kind: string(kr.Kind)}
		}
		return measure{Kind: string(kr.Kind), Linear: &linearMeasure{StartValue: kr.Linear.StartValue, TargetValue: kr.Linear.TargetValue, CurrentValue: kr.Linear.CurrentValue}}
	case domain.KRKindBoolean:
		if kr.Boolean == nil {
			return measure{Kind: string(kr.Kind)}
		}
		return measure{Kind: string(kr.Kind), Boolean: &booleanMeasure{IsDone: kr.Boolean.IsDone}}
	case domain.KRKindProject:
		if kr.Project == nil {
			return measure{Kind: string(kr.Kind)}
		}
		stages := make([]projectStage, 0, len(kr.Project.Stages))
		for _, stage := range kr.Project.Stages {
			stages = append(stages, projectStage{ID: stage.ID, Title: stage.Title, Weight: stage.Weight, IsDone: stage.IsDone})
		}
		return measure{Kind: string(kr.Kind), Project: &projectMeasure{Stages: stages}}
	default:
		return measure{Kind: string(kr.Kind)}
	}
}
