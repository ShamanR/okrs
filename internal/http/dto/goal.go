package dto

import "time"

type ShareTeam struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	TypeLabel string `json:"type_label"`
	Weight    int    `json:"weight"`
}

type GoalReply struct {
	ID         int64     `json:"id"`
	Text       string    `json:"text"`
	AuthorName string    `json:"author_name"`
	AuthorUDID string    `json:"author_udid"`
	CreatedAt  time.Time `json:"created_at"`
}

type GoalComment struct {
	ID             int64       `json:"id"`
	Text           string      `json:"text"`
	AuthorName     string      `json:"author_name"`
	AuthorUDID     string      `json:"author_udid"`
	CreatedAt      time.Time   `json:"created_at"`
	Resolved       bool        `json:"resolved"`
	ResolvedByName string      `json:"resolved_by_name"`
	ResolvedByUDID string      `json:"resolved_by_udid"`
	ResolvedAt     *time.Time  `json:"resolved_at"`
	Replies        []GoalReply `json:"replies"`
}

// GoalRef — компактная сводка связанной цели (родитель/ребёнок).
type GoalRef struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	PeriodID   int64  `json:"period_id"`
	PeriodName string `json:"period_name"`
	TeamID     int64  `json:"team_id"`
	TeamName   string `json:"team_name"`
	TeamType   string `json:"team_type"`
	Progress   int    `json:"progress"`
}

// LinkableGoal — кандидат в родители для пикера связи.
type LinkableGoal struct {
	GoalRef
	Lead string `json:"lead"`
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
	Parents      []GoalRef       `json:"parents"`
	Children     []GoalRef       `json:"children"`
	Comments     []GoalComment   `json:"comments"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type GoalResponse struct {
	Goal     GoalDetails   `json:"goal"`
	Comments []GoalComment `json:"comments"`
}
