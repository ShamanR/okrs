package dto

type ActivityActor struct {
	UDID        string `json:"udid,omitempty"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	Removed     bool   `json:"removed"`
}

type ActivityTarget struct {
	Section   string `json:"section"`
	TeamID    int64  `json:"team_id"`
	PeriodID  *int64 `json:"period_id,omitempty"`
	GoalID    *int64 `json:"goal_id,omitempty"`
	KRID      *int64 `json:"kr_id,omitempty"`
	CommentID *int64 `json:"comment_id,omitempty"`
}

type ActivityEvent struct {
	ID          int64           `json:"id"`
	Category    string          `json:"category"`
	Action      string          `json:"action"`
	Actor       ActivityActor   `json:"actor"`
	TeamID      *int64          `json:"team_id,omitempty"`
	PeriodID    *int64          `json:"period_id,omitempty"`
	GoalID      *int64          `json:"goal_id,omitempty"`
	KRID        *int64          `json:"kr_id,omitempty"`
	CommentID   *int64          `json:"comment_id,omitempty"`
	EntityTitle string          `json:"entity_title"`
	Target      *ActivityTarget `json:"target,omitempty"`
	Payload     map[string]any  `json:"payload,omitempty"`
	CreatedAt   string          `json:"created_at"`
}

type ActivityFeedResponse struct {
	Items      []ActivityEvent `json:"items"`
	NextCursor string          `json:"next_cursor,omitempty"`
}
