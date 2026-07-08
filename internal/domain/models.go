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
	KRKindProject   KRKind = "PROJECT"
	KRKindNumerical KRKind = "NUMERICAL"
	KRKindBoolean   KRKind = "BOOLEAN"
)

// KRUnits is the closed set of measurement units a NUMERICAL KR may use.
var KRUnits = []string{"%", "RPS", "мс", "сек", "мин", "час", "дней", "шт", "₽", "запросов", "ошибок", "пользователей", "заказов", "рублей"}

// IsValidKRUnit reports whether u is an allowed NUMERICAL unit.
func IsValidKRUnit(u string) bool {
	for _, allowed := range KRUnits {
		if allowed == u {
			return true
		}
	}
	return false
}

type TeamType string

// Team types form the org hierarchy levels, from broadest to narrowest:
// department → cluster → unit → group → team → squad → employee. The tree itself
// nests by parent_id and is not constrained by type; the type is a display label
// for the level.
const (
	TeamTypeDepartment TeamType = "department"
	TeamTypeCluster    TeamType = "cluster"
	TeamTypeUnit       TeamType = "unit"
	TeamTypeGroup      TeamType = "group"
	TeamTypeTeam       TeamType = "team"
	TeamTypeSquad      TeamType = "squad"
	TeamTypeEmployee   TeamType = "employee"
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
	LeadUDID    *string
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
	OwnerUDIDs  []string
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
	ZeroingCriteria   string
	Weight            int
	Kind              KRKind
	Progress          int
	SortOrder         int
	Project           *KRProject
	Numerical         *KRNumerical
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

type KRNumerical struct {
	StartValue   float64
	TargetValue  float64
	CurrentValue float64
	Unit         string
	Checkpoints  []KRNumericalCheckpoint
}

type KRNumericalCheckpoint struct {
	Value           float64 `json:"value"`
	ProgressPercent int     `json:"progress_percent"`
}

type KRBoolean struct {
	IsDone bool
}

type Period struct {
	ID         int64
	Name       string
	StartDate  time.Time
	EndDate    time.Time
	ArchivedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
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
	IsSystemAdmin      bool
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

	ActiveTenantID *int64
}

const (
	SystemUserAnonymous int64 = 1
	SystemUserMigration int64 = 2
)
