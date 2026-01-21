package domain

import "time"

type Priority string

const (
	PriorityP0 Priority = "P0"
	PriorityP1 Priority = "P1"
	PriorityP2 Priority = "P2"
	PriorityP3 Priority = "P3"
)

type WorkType string

const (
	WorkTypeDiscovery WorkType = "Discovery"
	WorkTypeDelivery  WorkType = "Delivery"
)

type FocusType string

const (
	FocusProfitability    FocusType = "PROFITABILITY"
	FocusStability        FocusType = "STABILITY"
	FocusSpeedEfficiency  FocusType = "SPEED_EFFICIENCY"
	FocusTechIndependence FocusType = "TECH_INDEPENDENCE"
)

type KRKind string

const (
	KRKindProject KRKind = "PROJECT"
	KRKindPercent KRKind = "PERCENT"
	KRKindBoolean KRKind = "BOOLEAN"
)

type TeamType string

const (
	TeamTypeCluster TeamType = "cluster"
	TeamTypeUnit    TeamType = "unit"
	TeamTypeTeam    TeamType = "team"
)

type Team struct {
	ID        int64
	Name      string
	Type      TeamType
	ParentID  *int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Goal struct {
	ID          int64
	TeamID      int64
	Year        int
	Quarter     int
	Title       string
	Description string
	Priority    Priority
	Weight      int
	WorkType    WorkType
	FocusType   FocusType
	OwnerText   string
	Progress    int
	CreatedAt   time.Time
	UpdatedAt   time.Time
	KeyResults  []KeyResult
	Comments    []GoalComment
}

type GoalComment struct {
	ID        int64
	GoalID    int64
	Text      string
	CreatedAt time.Time
}

type KeyResult struct {
	ID          int64
	GoalID      int64
	Title       string
	Description string
	Weight      int
	Kind        KRKind
	Progress    int
	Project     *KRProject
	Percent     *KRPercent
	Boolean     *KRBoolean
	Comments    []KeyResultComment
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type KeyResultComment struct {
	ID          int64
	KeyResultID int64
	Text        string
	CreatedAt   time.Time
}

type KRProject struct {
	Stages []KRProjectStage
}

type KRProjectStage struct {
	ID          int64
	KeyResultID int64
	Title       string
	Weight      int
	IsDone      bool
	SortOrder   int
}

type KRPercent struct {
	StartValue   float64
	TargetValue  float64
	CurrentValue float64
	Checkpoints  []KRPercentCheckpoint
}

type KRPercentCheckpoint struct {
	ID          int64
	KeyResultID int64
	MetricValue float64
	KRPercent   int
}

type KRBoolean struct {
	IsDone bool
}

type Quarter struct {
	Year    int
	Quarter int
}
