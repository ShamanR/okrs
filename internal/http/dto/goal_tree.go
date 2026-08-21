package dto

// GoalTreePeriod — период-слой графа (гранулярность = depth).
type GoalTreePeriod struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Depth  int    `json:"depth"`
	Status string `json:"status"`
}

// GoalTreeTeam — команда графа (для групп-боксов, дерева корней и «Мои цели»).
type GoalTreeTeam struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	TypeLabel string `json:"type_label"`
	ParentID  *int64 `json:"parent_id"`
	LedByMe   bool   `json:"led_by_me"`
}

// GoalTreeGoal — вершина графа + рёбра (id-ссылки в пределах набора).
type GoalTreeGoal struct {
	ID            int64   `json:"id"`
	Title         string  `json:"title"`
	TeamID        int64   `json:"team_id"`
	PeriodID      int64   `json:"period_id"`
	Progress      int     `json:"progress"`
	Priority      string  `json:"priority"`
	Weight        int     `json:"weight"`
	WorkType      string  `json:"work_type"`
	FocusType     string  `json:"focus_type"`
	OwnerText     string  `json:"owner_text"`
	ParentGoalIDs []int64 `json:"parent_goal_ids"`
	ChildGoalIDs  []int64 `json:"child_goal_ids"`
}

type GoalTreeResponse struct {
	Periods []GoalTreePeriod `json:"periods"`
	Teams   []GoalTreeTeam   `json:"teams"`
	Goals   []GoalTreeGoal   `json:"goals"`
}
