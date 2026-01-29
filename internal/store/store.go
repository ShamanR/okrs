package store

import (
	"time"

	"okrs/internal/domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	DB *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Store {
	return &Store{DB: db}
}

type GoalInput struct {
	TeamID      int64
	PeriodID    int64
	Title       string
	Description string
	Priority    domain.Priority
	Weight      int
	WorkType    domain.WorkType
	FocusType   domain.FocusType
	OwnerText   string
}

type PeriodInput struct {
	Name      string
	StartDate time.Time
	EndDate   time.Time
}

type KeyResultInput struct {
	GoalID      int64
	Title       string
	Description string
	Weight      int
	Kind        domain.KRKind
}

type ProjectStageInput struct {
	KeyResultID int64
	Title       string
	Weight      int
	SortOrder   int
	IsDone      bool
}

type PercentMetaInput struct {
	KeyResultID  int64
	StartValue   float64
	TargetValue  float64
	CurrentValue float64
}

type PercentCheckpointInput struct {
	KeyResultID int64
	MetricValue float64
	KRPercent   int
}

type LinearMetaInput struct {
	KeyResultID  int64
	StartValue   float64
	TargetValue  float64
	CurrentValue float64
}
