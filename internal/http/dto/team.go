package dto

import "time"

type TeamInfo struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	TypeLabel   string   `json:"type_label"`
	Description string   `json:"description,omitempty"`
	Lead        *UserRef `json:"lead,omitempty"`
	ParentID    *int64   `json:"parent_id,omitempty"`
}

type ProgressBarInfo struct {
	Actual   int    `json:"actual"`
	Forecast int    `json:"forecast"`
	Delta    int    `json:"delta"`
	Status   string `json:"status"`
}

type TeamChildSummaryResult struct {
	Team              TeamInfo         `json:"team"`
	Status            string           `json:"status"`
	StatusLabel       string           `json:"status_label"`
	HasGoals          bool             `json:"has_goals"`
	GoalsCount        int              `json:"goals_count"`
	HighPriorityCount int              `json:"high_priority_count"`
	ProgressMeta      *ProgressBarInfo `json:"progress_meta,omitempty"`
	LastUpdated       *time.Time       `json:"last_updated,omitempty"`
}

type TeamChildrenSummaryResponse struct {
	Period PeriodInfo               `json:"period"`
	Items  []TeamChildSummaryResult `json:"items"`
}

type TeamOKRResponse struct {
	Team            TeamInfo        `json:"team"`
	Period          PeriodInfo      `json:"period"`
	PeriodStatus    string          `json:"period_status"`
	StatusLabel     string          `json:"status_label"`
	StatusChangedAt *time.Time      `json:"status_changed_at,omitempty"`
	PeriodProgress  int             `json:"period_progress"`
	GoalsCount      int             `json:"goals_count"`
	GoalsWeight     int             `json:"goals_weight"`
	ProgressMeta    ProgressBarInfo `json:"progress_meta"`
	Goals           []GoalDetails   `json:"goals"`
}

type TeamOverviewResponse struct {
	AverageProgress int                         `json:"average_progress"`
	TeamsWithGoals  int                         `json:"teams_with_goals"`
	ProgressMeta    ProgressBarInfo             `json:"progress_meta"`
	ChildrenSummary TeamChildrenSummaryResponse `json:"children_summary"`
}
