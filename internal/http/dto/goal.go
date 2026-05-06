package dto

import "time"

type ShareTeam struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	TypeLabel string `json:"type_label"`
	Weight    int    `json:"weight"`
}

type GoalComment struct {
	ID         int64     `json:"id"`
	Text       string    `json:"text"`
	AuthorName string    `json:"author_name"`
	AuthorUDID string    `json:"author_udid"`
	CreatedAt  time.Time `json:"created_at"`
}

type GoalDetails struct {
	ID           int64           `json:"id"`
	TeamID       int64           `json:"team_id"`
	PeriodID     int64           `json:"period_id"`
	Title        string          `json:"title"`
	Description  string          `json:"description"`
	Priority     string          `json:"priority"`
	Weight       int             `json:"weight"`
	WorkType     string          `json:"work_type"`
	FocusType    string          `json:"focus_type"`
	Owners       []UserRef       `json:"owners"`
	Progress     int             `json:"progress"`
	ProgressMeta ProgressBarInfo `json:"progress_meta"`
	KeyResults   []KeyResult     `json:"key_results"`
	ShareTeams   []ShareTeam     `json:"share_teams"`
	Comments     []GoalComment   `json:"comments"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type GoalResponse struct {
	Goal     GoalDetails   `json:"goal"`
	Comments []GoalComment `json:"comments"`
}
