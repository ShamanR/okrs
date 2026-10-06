package dto

import "time"

type KRNote struct {
	Text       string    `json:"text"`
	AuthorName string    `json:"author_name"`
	AuthorUDID string    `json:"author_udid"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type NumericalCheckpoint struct {
	Value           float64 `json:"value"`
	ProgressPercent int     `json:"progress_percent"`
}

type NumericalMeasure struct {
	StartValue  float64 `json:"start_value"`
	TargetValue float64 `json:"target_value"`
	// CurrentValue всегда число: пока прогресс не обновляли, это стартовое значение.
	CurrentValue float64 `json:"current_value"`
	// CurrentValueRecorded — записывал ли чек-ин текущее значение. Нужен потребителю,
	// чтобы различать «значения ещё нет» и «значение равно стартовому»: по самим числам
	// эти случаи неотличимы, а progress_updated_at у данных, созданных до его появления,
	// пуст и при записанном значении.
	CurrentValueRecorded bool                  `json:"current_value_recorded"`
	Unit                 string                `json:"unit"`
	Checkpoints          []NumericalCheckpoint `json:"checkpoints,omitempty"`
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
	Kind      string            `json:"kind"`
	Numerical *NumericalMeasure `json:"numerical,omitempty"`
	Boolean   *BooleanMeasure   `json:"boolean,omitempty"`
	Project   *ProjectMeasure   `json:"project,omitempty"`
}

type KeyResult struct {
	ID              int64     `json:"id"`
	GoalID          int64     `json:"goal_id"`
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	ZeroingCriteria string    `json:"zeroing_criteria,omitempty"`
	Weight          int       `json:"weight"`
	Kind            string    `json:"kind"`
	Progress        int       `json:"progress"`
	HealthStatus    string    `json:"health_status"`
	Measure         Measure   `json:"measure"`
	Note            *KRNote   `json:"note"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	// ProgressUpdatedAt — время последнего обновления прогресса, а не любой правки
	// KR: UpdatedAt двигает и смена названия, веса или описания. nil означает, что
	// прогресс ещё ни разу не обновляли.
	ProgressUpdatedAt *time.Time `json:"progress_updated_at"`
}
