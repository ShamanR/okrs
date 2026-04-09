package dto

import "time"

type TeamInfo struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	TypeLabel string `json:"type_label"`
	ParentID  *int64 `json:"parent_id,omitempty"`
}

type ProgressBarInfo struct {
	Actual   int    `json:"actual"`
	Forecast int    `json:"forecast"`
	Delta    int    `json:"delta"`
	Status   string `json:"status"`
}

type PrioritySummaryInfo struct {
	P0 int `json:"p0"`
	P1 int `json:"p1"`
	P2 int `json:"p2"`
	P3 int `json:"p3"`
}

type WorkBalanceInfo struct {
	Discovery int `json:"discovery"`
	Delivery  int `json:"delivery"`
}

type TeamChildSummaryResult struct {
	Team         TeamInfo         `json:"team"`
	Status       string           `json:"status"`
	StatusLabel  string           `json:"status_label"`
	HasGoals     bool             `json:"has_goals"`
	ProgressMeta *ProgressBarInfo `json:"progress_meta,omitempty"`
	LastUpdated  *time.Time       `json:"last_updated,omitempty"`
}

type TeamChildrenSummaryResponse struct {
	Period PeriodInfo               `json:"period"`
	Items  []TeamChildSummaryResult `json:"items"`
}

type TeamOKRResponse struct {
	Team           TeamInfo        `json:"team"`
	Period         PeriodInfo      `json:"period"`
	PeriodStatus   string          `json:"period_status"`
	StatusLabel    string          `json:"status_label"`
	PeriodProgress int             `json:"period_progress"`
	GoalsCount     int             `json:"goals_count"`
	GoalsWeight    int             `json:"goals_weight"`
	ProgressMeta   ProgressBarInfo `json:"progress_meta"`
	Goals          []GoalDetails   `json:"goals"`
}

type TeamOverviewResponse struct {
	AverageProgress int                         `json:"average_progress"`
	TeamsWithGoals  int                         `json:"teams_with_goals"`
	ProgressMeta    ProgressBarInfo             `json:"progress_meta"`
	Priorities      PrioritySummaryInfo         `json:"priorities"`
	WorkBalance     WorkBalanceInfo             `json:"work_balance"`
	ChildrenSummary TeamChildrenSummaryResponse `json:"children_summary"`
}
