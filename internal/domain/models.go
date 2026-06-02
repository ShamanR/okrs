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
	KRKindLinear  KRKind = "LINEAR"
	KRKindBoolean KRKind = "BOOLEAN"
)

type TeamType string

const (
	TeamTypeCluster TeamType = "cluster"
	TeamTypeUnit    TeamType = "unit"
	TeamTypeTeam    TeamType = "team"
)

type TeamPeriodStatus string

const (
	TeamPeriodStatusNoGoals    TeamPeriodStatus = "no_goals"    // computed: no goals in period
	TeamPeriodStatusForming    TeamPeriodStatus = "forming"     // черновик: full edit
	TeamPeriodStatusReady      TeamPeriodStatus = "ready"       // к валидации: full edit
	TeamPeriodStatusInProgress TeamPeriodStatus = "in_progress" // в работе: progress/comments only
	TeamPeriodStatusClosed     TeamPeriodStatus = "closed"      // закрыты: comments only
)

type Team struct {
	ID          int64
	Name        string
	Type        TeamType
	ParentID    *int64
	Lead        string
	Description string
	DeletedAt   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Goal struct {
	ID          int64
	TeamID      int64
	PeriodID    int64
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
	ID         int64
	GoalID     int64
	Text       string
	AuthorName string
	AuthorUDID string
	CreatedAt  time.Time
}

type KeyResultNote struct {
	KeyResultID int64
	Text        string
	AuthorName  string
	AuthorUDID  string
	UpdatedAt   time.Time
}

type KeyResult struct {
	ID                int64
	GoalID            int64
	Title             string
	Description       string
	Weight            int
	Kind              KRKind
	Progress          int
	SortOrder         int
	Project           *KRProject
	Percent           *KRPercent
	Linear            *KRLinear
	Boolean           *KRBoolean
	Note              *KeyResultNote
	CreatedAt         time.Time
	UpdatedAt         time.Time
	ProgressUpdatedAt *time.Time
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

type KRLinear struct {
	StartValue   float64
	TargetValue  float64
	CurrentValue float64
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

type Period struct {
	ID        int64
	Name      string
	StartDate time.Time
	EndDate   time.Time
	SortOrder int
	CreatedAt time.Time
	UpdatedAt time.Time
}

type User struct {
	ID                 int64
	UDID               string
	ProviderSubjectKey string
	Provider           string
	Subject            string
	DisplayName        string
	AvatarURL          string
	Email              string
	AttributesJSON     map[string]any
	IsAdmin            bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
	LastLoginAt        time.Time
}

type AuthSession struct {
	ID         string
	UserID     int64
	Provider   string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	LastSeenAt time.Time
	UserAgent  string
	IP         string
}

const (
	SystemUserAnonymous int64 = 1
	SystemUserMigration int64 = 2
)
