package dto

import "time"

type KRComment struct {
	ID         int64     `json:"id"`
	Text       string    `json:"text"`
	AuthorName string    `json:"author_name"`
	AuthorUDID string    `json:"author_udid"`
	CreatedAt  time.Time `json:"created_at"`
}

type PercentCheckpoint struct {
	ID          int64   `json:"id"`
	MetricValue float64 `json:"metric_value"`
	Percent     int     `json:"percent"`
}

type PercentMeasure struct {
	StartValue   float64 `json:"start_value"`
	TargetValue  float64 `json:"target_value"`
	CurrentValue float64 `json:"current_value"`
}

type LinearMeasure struct {
	StartValue   float64 `json:"start_value"`
	TargetValue  float64 `json:"target_value"`
	CurrentValue float64 `json:"current_value"`
}

type BooleanMeasure struct {
	IsDone bool `json:"is_done"`
}

type ProjectStage struct {
	ID     int64  `json:"id"`
	Title  string `json:"title"`
	Weight int    `json:"weight"`
	IsDone bool   `json:"is_done"`
}

type ProjectMeasure struct {
	Stages []ProjectStage `json:"stages"`
}

type Measure struct {
	Kind        string              `json:"kind"`
	Percent     *PercentMeasure     `json:"percent,omitempty"`
	Linear      *LinearMeasure      `json:"linear,omitempty"`
	Boolean     *BooleanMeasure     `json:"boolean,omitempty"`
	Project     *ProjectMeasure     `json:"project,omitempty"`
	Checkpoints []PercentCheckpoint `json:"checkpoints,omitempty"`
}

type KeyResult struct {
	ID          int64       `json:"id"`
	GoalID      int64       `json:"goal_id"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Weight      int         `json:"weight"`
	Kind        string      `json:"kind"`
	Progress    int         `json:"progress"`
	Measure     Measure     `json:"measure"`
	Comments    []KRComment `json:"comments"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}
