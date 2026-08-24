package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/core/progress"
	"okrs/internal/store"
	"okrs/internal/store/activity"
	"okrs/internal/store/goallinks"
	"okrs/internal/store/goals"
	"okrs/internal/store/grants"
	"okrs/internal/store/krs"
	"okrs/internal/store/periods"
	"okrs/internal/store/shares"
	"okrs/internal/store/teams"
)

// GrantsProvider gives the service access to the cached user_hierarchy_grants snapshot.
// *grants.GrantsCache satisfies this interface.
type GrantsProvider interface {
	AllGrants(ctx context.Context) (map[int64][]grants.HierarchyGrant, error)
}

// Per-entity repository interfaces used by the service layer.
// Each interface is satisfied by the corresponding store.*Repository type.

type TeamRepo interface {
	ListTeams(ctx context.Context, scope domain.TenantScope) ([]domain.Team, error)
	ListDeletedTeams(ctx context.Context, scope domain.TenantScope) ([]domain.Team, error)
	ListAllTeams(ctx context.Context, scope domain.TenantScope) ([]domain.Team, error)
	GetTeam(ctx context.Context, scope domain.TenantScope, id int64) (domain.Team, error)
	CreateTeam(ctx context.Context, scope domain.TenantScope, input teams.TeamInput) (int64, error)
	UpdateTeam(ctx context.Context, scope domain.TenantScope, input teams.TeamInput, id int64) error
	TeamHasGoals(ctx context.Context, scope domain.TenantScope, id int64) (bool, error)
	TeamHasGoalsInPeriod(ctx context.Context, scope domain.TenantScope, id, periodID int64) (bool, error)
	ListTeamIDsWithGoalsInPeriod(ctx context.Context, scope domain.TenantScope, periodID int64) (map[int64]struct{}, error)
	SoftDeleteTeam(ctx context.Context, scope domain.TenantScope, id int64) error
	RestoreTeam(ctx context.Context, scope domain.TenantScope, id int64) error
	HardDeleteTeam(ctx context.Context, scope domain.TenantScope, id int64) error
}

type GoalRepo interface {
	ListGoalsByTeamPeriod(ctx context.Context, scope domain.TenantScope, teamID, periodID int64) ([]domain.Goal, error)
	ListGoalsByTeamsPeriod(ctx context.Context, scope domain.TenantScope, periodID int64, teamIDs []int64) (map[int64][]domain.Goal, error)
	GetGoal(ctx context.Context, scope domain.TenantScope, id int64) (domain.Goal, error)
	CreateGoal(ctx context.Context, scope domain.TenantScope, input goals.GoalInput) (int64, error)
	CopyGoal(ctx context.Context, scope domain.TenantScope, in goals.CopyGoalInput) (int64, error)
	DeleteGoal(ctx context.Context, scope domain.TenantScope, id int64) error
	UpdateGoal(ctx context.Context, scope domain.TenantScope, input goals.GoalUpdateInput) error
	UpdateGoalFields(ctx context.Context, scope domain.TenantScope, input goals.GoalFieldsUpdateInput) error
	UpdateGoalOwner(ctx context.Context, scope domain.TenantScope, goalID, teamID int64, weight int) error
	MoveGoal(ctx context.Context, scope domain.TenantScope, teamID, goalID int64, direction int) error
	AddGoalComment(ctx context.Context, scope domain.TenantScope, goalID int64, text string, authorUserID int64) (int64, error)
	AddGoalReply(ctx context.Context, scope domain.TenantScope, goalID, parentID int64, text string, authorUserID int64) (int64, error)
	GetGoalCommentMeta(ctx context.Context, scope domain.TenantScope, goalID, commentID int64) (int64, bool, error)
	DeleteGoalComment(ctx context.Context, scope domain.TenantScope, goalID, commentID int64) error
	SetGoalCommentResolved(ctx context.Context, scope domain.TenantScope, goalID, commentID int64, resolved bool, userID int64) (bool, error)
	ListGoalComments(ctx context.Context, scope domain.TenantScope, goalID int64) ([]domain.GoalComment, error)
	ListGoalCommentsByGoals(ctx context.Context, scope domain.TenantScope, goalIDs []int64) (map[int64][]domain.GoalComment, error)
	ListGoalOwnerTeamIDs(ctx context.Context, scope domain.TenantScope, goalIDs []int64) (map[int64]int64, error)
	ListGoalsByIDs(ctx context.Context, scope domain.TenantScope, ids []int64) ([]domain.Goal, error)
	ListTeamLastGoalUpdateInPeriod(ctx context.Context, scope domain.TenantScope, periodID int64, teamIDs []int64) (map[int64]time.Time, error)
	ListGoalsForPeriods(ctx context.Context, scope domain.TenantScope, periodIDs []int64, allowedTeamIDs []int64, adminAll bool) ([]domain.Goal, error)
}

type GoalShareRepo interface {
	ListGoalShares(ctx context.Context, scope domain.TenantScope, goalID int64) ([]shares.GoalShare, error)
	ListGoalSharesByGoalIDs(ctx context.Context, scope domain.TenantScope, goalIDs []int64) (map[int64][]shares.GoalShare, error)
	GetGoalShare(ctx context.Context, scope domain.TenantScope, goalID, teamID int64) (shares.GoalShare, error)
	ReplaceGoalShares(ctx context.Context, scope domain.TenantScope, goalID int64, shares []shares.GoalShareInput) error
	DeleteGoalShare(ctx context.Context, scope domain.TenantScope, goalID, teamID int64) error
	UpdateGoalTeamWeight(ctx context.Context, scope domain.TenantScope, goalID, teamID int64, weight int) error
}

// GoalLinkRepo manages goal↔goal parent/child links.
type GoalLinkRepo interface {
	ReplaceParents(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, adminAll bool, childID int64, parentIDs []int64) (added, removed []int64, err error)
	ListLinksForGoals(ctx context.Context, scope domain.TenantScope, goalIDs, allowedTeamIDs []int64, adminAll bool) (parents, children map[int64][]domain.GoalRef, err error)
	ListLinkable(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, adminAll bool, periodID *int64, excludeGoalID int64, q string) ([]goallinks.LinkableGoal, error)
}

type PeriodRepo interface {
	ListPeriods(ctx context.Context, scope domain.TenantScope) ([]domain.Period, error)
	GetPeriod(ctx context.Context, scope domain.TenantScope, id int64) (domain.Period, error)
	FindPeriodForDate(ctx context.Context, scope domain.TenantScope, date time.Time) (domain.Period, error)
	CreatePeriod(ctx context.Context, scope domain.TenantScope, input periods.PeriodInput) (int64, error)
	UpdatePeriod(ctx context.Context, scope domain.TenantScope, periodID int64, input periods.PeriodInput) error
	DeletePeriod(ctx context.Context, scope domain.TenantScope, periodID int64) error
	ArchivePeriod(ctx context.Context, scope domain.TenantScope, periodID int64) error
	UnarchivePeriod(ctx context.Context, scope domain.TenantScope, periodID int64) error
}

type KRRepo interface {
	GetKeyResult(ctx context.Context, scope domain.TenantScope, id int64) (domain.KeyResult, error)
	CreateKeyResult(ctx context.Context, scope domain.TenantScope, input krs.KeyResultInput) (int64, error)
	UpdateKeyResult(ctx context.Context, scope domain.TenantScope, input krs.KeyResultUpdateInput) error
	DeleteKeyResult(ctx context.Context, scope domain.TenantScope, id int64) error
	MoveKeyResult(ctx context.Context, scope domain.TenantScope, krID int64, direction int) error
	UpsertKeyResultNote(ctx context.Context, scope domain.TenantScope, krID int64, text string, authorUserID int64) error
	GetKeyResultNote(ctx context.Context, scope domain.TenantScope, krID int64) (*domain.KeyResultNote, error)
	BatchLoadNotes(ctx context.Context, scope domain.TenantScope, krIDs []int64) (map[int64]*domain.KeyResultNote, error)
	GetBooleanMeta(ctx context.Context, scope domain.TenantScope, krID int64) (*domain.KRBoolean, error)
	UpdateKeyResultDescription(ctx context.Context, scope domain.TenantScope, krID int64, description string) error
	FindGoalIDByKR(ctx context.Context, scope domain.TenantScope, krID int64) (int64, error)
	FindGoalIDByStage(ctx context.Context, scope domain.TenantScope, stageID int64) (int64, error)
	UpdateNumericalCurrent(ctx context.Context, scope domain.TenantScope, krID int64, current float64) error
	UpdateHealthStatus(ctx context.Context, scope domain.TenantScope, krID int64, status domain.KRHealthStatus) error
	UpdateBoolean(ctx context.Context, scope domain.TenantScope, krID int64, done bool) error
	ListProjectStages(ctx context.Context, scope domain.TenantScope, krID int64) ([]domain.KRProjectStage, error)
	UpdateProjectStageDone(ctx context.Context, scope domain.TenantScope, stageID int64, done bool) error
	BatchUpdateProjectStagesDone(ctx context.Context, scope domain.TenantScope, krID int64, updates map[int64]bool) error
	UpsertNumericalMeta(ctx context.Context, scope domain.TenantScope, input krs.NumericalMetaInput) error
	UpsertBooleanMeta(ctx context.Context, scope domain.TenantScope, krID int64, done bool) error
	ReplaceProjectStages(ctx context.Context, scope domain.TenantScope, krID int64, stages []krs.ProjectStageInput) error
}

type TeamStatusRepo interface {
	GetTeamPeriodStatus(ctx context.Context, scope domain.TenantScope, teamID, periodID int64) (domain.TeamPeriodStatus, error)
	GetTeamPeriodStatusWithTime(ctx context.Context, scope domain.TenantScope, teamID, periodID int64) (domain.TeamPeriodStatus, *time.Time, error)
	ListTeamPeriodStatuses(ctx context.Context, scope domain.TenantScope, periodID int64, teamIDs []int64) (map[int64]domain.TeamPeriodStatus, error)
	SetTeamPeriodStatus(ctx context.Context, scope domain.TenantScope, teamID, periodID int64, status domain.TeamPeriodStatus) error
	SetTeamPeriodStatuses(ctx context.Context, scope domain.TenantScope, periodID int64, teamIDs []int64, status domain.TeamPeriodStatus) error
}

type UserRepo interface {
	GetUsersByDisplayNames(ctx context.Context, names []string) ([]*domain.User, error)
	SearchUsersUnrestricted(ctx context.Context, q string, limit int) ([]*domain.User, error)
	SearchUsersInSet(ctx context.Context, userIDs []int64, leadUDIDs []string, q string, limit int) ([]*domain.User, error)
	GetUsersByUDIDs(ctx context.Context, udids []string) ([]*domain.User, error)
	ListUserLeadTeams(ctx context.Context) (map[string]string, error)
	ValidateUDIDsExist(ctx context.Context, udids []string) ([]string, error)
}

// ActivityRepo records and reads the append-only activity journal.
type ActivityRepo interface {
	Record(ctx context.Context, scope domain.TenantScope, ev domain.ActivityEvent) (int64, error)
	RecordBatch(ctx context.Context, scope domain.TenantScope, evs []domain.ActivityEvent) error
	List(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, f activity.ListFilter) ([]domain.ActivityEvent, *activity.Cursor, error)
	TreeCounts(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, periodID *int64, since *time.Time) (map[int64]int, error)
	CategoryCounts(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, f activity.ListFilter) (map[string]int, error)
	Purge(ctx context.Context, scope domain.TenantScope, olderThan *time.Time) (int64, error)
}

// Deps holds all repository dependencies for the service.
type Deps struct {
	Teams        TeamRepo
	Goals        GoalRepo
	Shares       GoalShareRepo
	GoalLinks    GoalLinkRepo
	Periods      PeriodRepo
	KRs          KRRepo
	Statuses     TeamStatusRepo
	Users        UserRepo
	Grants       GrantsProvider
	HCCache      *HealthCheckInCache
	Activity     ActivityRepo
	ProgressSnap ProgressSnapRepo
	Logger       *slog.Logger
}

type Service struct {
	teams        TeamRepo
	goals        GoalRepo
	shares       GoalShareRepo
	goalLinks    GoalLinkRepo
	periods      PeriodRepo
	krs          KRRepo
	statuses     TeamStatusRepo
	users        UserRepo
	grants       GrantsProvider
	hcCache      *HealthCheckInCache
	activity     ActivityRepo
	progressSnap ProgressSnapRepo
	logger       *slog.Logger
}

var (
	ErrTeamHasGoals                = errors.New("team has goals")
	ErrTeamNotVisibleInPeriod      = errors.New("team not visible in period")
	ErrPeriodClosed                = errors.New("period is closed")
	ErrCannotShareWithClosedPeriod = errors.New("cannot share goal with team whose period is in_progress or closed")
	ErrShareTargetNotInTenant      = errors.New("share target team is not in the active tenant")
	ErrPeriodNotClosed             = errors.New("period must be closed to archive")
	ErrTransferTargetSameAsSource  = errors.New("transfer target equals source team and period")
	ErrTransferTargetNotFound      = errors.New("transfer target team or period not found in tenant")
	// ErrForbidden signals an authorization failure the handler maps to HTTP 403.
	ErrForbidden = errors.New("forbidden")
	// ErrGoalNotOnTeamBoard signals a goal-scope export where the goal is not on the context team's board.
	ErrGoalNotOnTeamBoard = errors.New("goal not on team board")
	// Goal-link errors mapped by handlers to 400 (self/not accessible) and 409 (cycle).
	ErrGoalLinkSelf          = errors.New("goal cannot link to itself")
	ErrGoalLinkNotAccessible = errors.New("parent goal not accessible")
	ErrGoalLinkCycle         = errors.New("goal link would create a cycle")
)

// New constructs a Service from a Deps bundle.
func New(deps Deps) *Service {
	return &Service{
		teams:        deps.Teams,
		goals:        deps.Goals,
		shares:       deps.Shares,
		goalLinks:    deps.GoalLinks,
		periods:      deps.Periods,
		krs:          deps.KRs,
		statuses:     deps.Statuses,
		users:        deps.Users,
		grants:       deps.Grants,
		hcCache:      deps.HCCache,
		activity:     deps.Activity,
		progressSnap: deps.ProgressSnap,
		logger:       deps.Logger,
	}
}

// NewFromStore constructs a Service from a *store.Store and a GrantsProvider.
// Use this at the wiring layer instead of building Deps manually.
func NewFromStore(st *store.Store, grantsProvider GrantsProvider, hcCache *HealthCheckInCache, logger *slog.Logger) *Service {
	return New(Deps{
		Teams:        st.Teams,
		Goals:        st.Goals,
		Shares:       st.Shares,
		GoalLinks:    st.GoalLinks,
		Periods:      st.Periods,
		KRs:          st.KRs,
		Statuses:     st.Statuses,
		Users:        st.Users,
		Grants:       grantsProvider,
		HCCache:      hcCache,
		Activity:     st.Activity,
		ProgressSnap: st.ProgressSnap,
		Logger:       logger,
	})
}

type TeamNode struct {
	Team     domain.Team
	Children []TeamNode
}

type TeamSummary struct {
	ID             int64
	Name           string
	Type           domain.TeamType
	Indent         int
	Status         domain.TeamPeriodStatus
	PeriodProgress int
	GoalsCount     int
	GoalsWeight    int
	Goals          []TeamGoalSummary
}

type TeamGoalSummary struct {
	ID         int64
	Title      string
	Weight     int
	Progress   int
	ShareTeams []TeamShareInfo
	Priority   string
	WorkType   domain.WorkType
}

type TeamShareInfo struct {
	ID     int64
	Name   string
	Type   domain.TeamType
	Weight int
}

type TeamChildSummary struct {
	Team              domain.Team
	Status            domain.TeamPeriodStatus
	HasGoals          bool
	Progress          int
	GoalsCount        int
	HighPriorityCount int
	LastUpdateAt      *time.Time
}

type TeamOverview struct {
	AverageProgress int
	TeamsWithGoals  int
	ChildrenSummary []TeamChildSummary
}

type TeamOKR struct {
	Team            domain.Team
	Period          domain.Period
	PeriodStatus    domain.TeamPeriodStatus
	StatusChangedAt *time.Time
	PeriodProgress  int
	GoalsCount      int
	GoalsWeight     int
	Goals           []GoalDetails
}

type GoalDetails struct {
	Goal       domain.Goal
	ShareTeams []TeamShareInfo
}

func (s *Service) GetHierarchy(ctx context.Context, scope domain.TenantScope, periodID *int64) ([]TeamNode, error) {
	allTeams, err := s.teams.ListAllTeams(ctx, scope)
	if err != nil {
		return nil, err
	}
	return s.getHierarchyFromTeams(ctx, scope, allTeams, periodID)
}

func (s *Service) getHierarchyFromTeams(ctx context.Context, scope domain.TenantScope, allTeams []domain.Team, periodID *int64) ([]TeamNode, error) {
	teamIDsWithGoals := map[int64]struct{}{}
	if periodID != nil && *periodID > 0 {
		ids, err := s.teams.ListTeamIDsWithGoalsInPeriod(ctx, scope, *periodID)
		if err != nil {
			return nil, err
		}
		teamIDsWithGoals = ids
	}
	visibleTeams := make([]domain.Team, 0, len(allTeams))
	for _, team := range allTeams {
		if team.DeletedAt == nil {
			visibleTeams = append(visibleTeams, team)
			continue
		}
		if periodID == nil || *periodID == 0 {
			continue
		}
		_, hasGoals := teamIDsWithGoals[team.ID]
		if hasGoals {
			visibleTeams = append(visibleTeams, team)
		}
	}
	_, childrenMap, roots := buildTeamHierarchy(visibleTeams)
	nodes := make([]TeamNode, 0, len(roots))
	for _, team := range roots {
		node := buildTeamNode(team, childrenMap)
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func (s *Service) GetTeam(ctx context.Context, scope domain.TenantScope, teamID int64) (domain.Team, error) {
	return s.teams.GetTeam(ctx, scope, teamID)
}

func (s *Service) ListPeriods(ctx context.Context, scope domain.TenantScope) ([]domain.Period, error) {
	return s.periods.ListPeriods(ctx, scope)
}

// ListPeriodViews returns periods enriched with parent/depth/status via domain.BuildPeriodViews.
// When includeArchived is false, archived periods are filtered out before building the views, so
// a visible period's ParentID never points at a period the caller can't see.
func (s *Service) ListPeriodViews(ctx context.Context, scope domain.TenantScope, includeArchived bool) ([]domain.PeriodView, error) {
	all, err := s.periods.ListPeriods(ctx, scope)
	if err != nil {
		return nil, err
	}
	src := all
	if !includeArchived {
		src = make([]domain.Period, 0, len(all))
		for _, p := range all {
			if p.ArchivedAt == nil {
				src = append(src, p)
			}
		}
	}
	return domain.BuildPeriodViews(src, time.Now()), nil
}

func (s *Service) GetPeriod(ctx context.Context, scope domain.TenantScope, periodID int64) (domain.Period, error) {
	return s.periods.GetPeriod(ctx, scope, periodID)
}

func (s *Service) GetTeamsWithPeriodSummary(ctx context.Context, scope domain.TenantScope, periodID int64, orgID *int64) ([]TeamSummary, error) {
	allTeams, err := s.teams.ListAllTeams(ctx, scope)
	if err != nil {
		return nil, err
	}
	return s.getTeamsWithPeriodSummaryFromTeams(ctx, scope, allTeams, periodID, orgID)
}

// teamSummaryBatch holds pre-loaded data for building TeamSummary without per-team DB queries.
type teamSummaryBatch struct {
	teamIDsWithGoals map[int64]struct{}
	goalsByTeam      map[int64][]domain.Goal
	statuses         map[int64]domain.TeamPeriodStatus
	sharesByGoal     map[int64][]shares.GoalShare
}

func (s *Service) getTeamsWithPeriodSummaryFromTeams(ctx context.Context, scope domain.TenantScope, allTeams []domain.Team, periodID int64, orgID *int64) ([]TeamSummary, error) {
	teamsByID, childrenMap, roots := buildTeamHierarchy(allTeams)

	allTeamIDs := make([]int64, len(allTeams))
	for i, t := range allTeams {
		allTeamIDs[i] = t.ID
	}

	teamIDsWithGoals, err := s.teams.ListTeamIDsWithGoalsInPeriod(ctx, scope, periodID)
	if err != nil {
		return nil, err
	}
	goalsByTeam, err := s.goals.ListGoalsByTeamsPeriod(ctx, scope, periodID, allTeamIDs)
	if err != nil {
		return nil, err
	}
	statuses, err := s.statuses.ListTeamPeriodStatuses(ctx, scope, periodID, allTeamIDs)
	if err != nil {
		return nil, err
	}

	allGoalIDs := make([]int64, 0)
	for _, gList := range goalsByTeam {
		for _, g := range gList {
			allGoalIDs = append(allGoalIDs, g.ID)
		}
	}
	sharesByGoal, err := s.shares.ListGoalSharesByGoalIDs(ctx, scope, allGoalIDs)
	if err != nil {
		return nil, err
	}

	batch := &teamSummaryBatch{
		teamIDsWithGoals: teamIDsWithGoals,
		goalsByTeam:      goalsByTeam,
		statuses:         statuses,
		sharesByGoal:     sharesByGoal,
	}

	filteredRoots := roots
	if orgID != nil {
		if team, ok := teamsByID[*orgID]; ok {
			filteredRoots = []domain.Team{team}
		}
	}
	rows := make([]TeamSummary, 0, len(allTeams))
	for _, team := range filteredRoots {
		s.appendTeamSummaryFromBatch(&rows, team, 0, childrenMap, teamsByID, batch)
	}
	return rows, nil
}

func (s *Service) appendTeamSummaryFromBatch(rows *[]TeamSummary, team domain.Team, level int, childrenMap map[int64][]domain.Team, teamsByID map[int64]domain.Team, batch *teamSummaryBatch) {
	_, hasGoals := batch.teamIDsWithGoals[team.ID]
	visible := team.DeletedAt == nil || hasGoals

	if visible {
		goalsList := batch.goalsByTeam[team.ID]

		status := domain.TeamPeriodStatusNoGoals
		if s, ok := batch.statuses[team.ID]; ok {
			status = s
		}

		goalRows := make([]TeamGoalSummary, 0, len(goalsList))
		goalsWeight := 0
		for i := range goalsList {
			goalsList[i].Progress = CalculateGoalProgress(&goalsList[i])
			goalsWeight += goalsList[i].Weight
			shareTeams := buildShareInfosFromBatch(goalsList[i], batch.sharesByGoal[goalsList[i].ID], teamsByID)
			goalRows = append(goalRows, TeamGoalSummary{
				ID:         goalsList[i].ID,
				Title:      goalsList[i].Title,
				Weight:     goalsList[i].Weight,
				Progress:   goalsList[i].Progress,
				ShareTeams: shareTeams,
				Priority:   string(goalsList[i].Priority),
				WorkType:   goalsList[i].WorkType,
			})
		}

		*rows = append(*rows, TeamSummary{
			ID:             team.ID,
			Name:           team.Name,
			Type:           team.Type,
			Indent:         level * 24,
			Status:         status,
			PeriodProgress: progress.PeriodProgress(goalsList),
			GoalsCount:     len(goalsList),
			GoalsWeight:    goalsWeight,
			Goals:          goalRows,
		})
	}

	nextLevel := level
	if visible {
		nextLevel = level + 1
	}
	children := childrenMap[team.ID]
	sort.Slice(children, func(i, j int) bool { return children[i].Name < children[j].Name })
	for _, child := range children {
		s.appendTeamSummaryFromBatch(rows, child, nextLevel, childrenMap, teamsByID, batch)
	}
}

func (s *Service) GetTeamOKR(ctx context.Context, scope domain.TenantScope, teamID, periodID int64, period domain.Period) (TeamOKR, error) {
	team, err := s.teams.GetTeam(ctx, scope, teamID)
	if err != nil {
		return TeamOKR{}, err
	}
	visible, err := s.isTeamVisibleInPeriod(ctx, scope, team, periodID)
	if err != nil {
		return TeamOKR{}, err
	}
	if !visible {
		return TeamOKR{}, ErrTeamNotVisibleInPeriod
	}
	goalsList, err := s.goals.ListGoalsByTeamPeriod(ctx, scope, teamID, periodID)
	if err != nil {
		return TeamOKR{}, err
	}

	goalIDs := make([]int64, len(goalsList))
	for i, g := range goalsList {
		goalIDs[i] = g.ID
	}
	sharesByGoal, err := s.shares.ListGoalSharesByGoalIDs(ctx, scope, goalIDs)
	if err != nil {
		return TeamOKR{}, err
	}
	allTeams, err := s.teams.ListAllTeams(ctx, scope)
	if err != nil {
		return TeamOKR{}, err
	}
	teamsByID := make(map[int64]domain.Team, len(allTeams))
	for _, t := range allTeams {
		teamsByID[t.ID] = t
	}

	goalsWeight := 0
	goalDetails := make([]GoalDetails, 0, len(goalsList))
	for i := range goalsList {
		goalsList[i].Progress = CalculateGoalProgress(&goalsList[i])
		goalsWeight += goalsList[i].Weight
		shareTeams := buildShareInfosFromBatch(goalsList[i], sharesByGoal[goalsList[i].ID], teamsByID)
		goalDetails = append(goalDetails, GoalDetails{
			Goal:       goalsList[i],
			ShareTeams: shareTeams,
		})
	}

	periodProgress := progress.PeriodProgress(goalsList)
	status, statusChangedAt, err := s.statuses.GetTeamPeriodStatusWithTime(ctx, scope, teamID, periodID)
	if err != nil {
		return TeamOKR{}, err
	}
	return TeamOKR{
		Team:            team,
		Period:          period,
		PeriodStatus:    status,
		StatusChangedAt: statusChangedAt,
		PeriodProgress:  periodProgress,
		GoalsCount:      len(goalsList),
		GoalsWeight:     goalsWeight,
		Goals:           goalDetails,
	}, nil
}

// buildShareInfosFromBatch builds TeamShareInfo slice from pre-loaded shares and teams.
// This avoids per-goal DB calls in loops.
func buildShareInfosFromBatch(goal domain.Goal, shareList []shares.GoalShare, teamsByID map[int64]domain.Team) []TeamShareInfo {
	teamIDs := make(map[int64]struct{}, len(shareList)+1)
	teamIDs[goal.TeamID] = struct{}{}
	for _, share := range shareList {
		teamIDs[share.TeamID] = struct{}{}
	}
	teamInfos := make([]TeamShareInfo, 0, len(teamIDs))
	for teamID := range teamIDs {
		team, ok := teamsByID[teamID]
		if !ok {
			continue
		}
		weight := goal.Weight
		for _, share := range shareList {
			if share.TeamID == teamID {
				weight = share.Weight
				break
			}
		}
		teamInfos = append(teamInfos, TeamShareInfo{ID: team.ID, Name: team.Name, Type: team.Type, Weight: weight})
	}
	sort.Slice(teamInfos, func(i, j int) bool { return teamInfos[i].Name < teamInfos[j].Name })
	return teamInfos
}

func (s *Service) GetDirectChildrenSummary(ctx context.Context, scope domain.TenantScope, teamID, periodID int64) ([]TeamChildSummary, error) {
	allTeams, err := s.teams.ListAllTeams(ctx, scope)
	if err != nil {
		return nil, err
	}
	hierarchy, err := s.getHierarchyFromTeams(ctx, scope, allTeams, &periodID)
	if err != nil {
		return nil, err
	}
	children := findDirectChildren(teamID, hierarchy)
	if len(children) == 0 {
		return []TeamChildSummary{}, nil
	}
	return s.buildDirectChildrenSummary(ctx, scope, periodID, children, nil)
}

func (s *Service) buildDirectChildrenSummary(ctx context.Context, scope domain.TenantScope, periodID int64, children []TeamNode, goalsByTeam map[int64][]domain.Goal) ([]TeamChildSummary, error) {
	childIDs := make([]int64, 0, len(children))
	for _, child := range children {
		childIDs = append(childIDs, child.Team.ID)
	}
	statuses, err := s.statuses.ListTeamPeriodStatuses(ctx, scope, periodID, childIDs)
	if err != nil {
		return nil, err
	}
	lastUpdates, err := s.goals.ListTeamLastGoalUpdateInPeriod(ctx, scope, periodID, childIDs)
	if err != nil {
		return nil, err
	}
	result := make([]TeamChildSummary, 0, len(childIDs))
	for _, child := range children {
		item := TeamChildSummary{
			Team:   child.Team,
			Status: domain.TeamPeriodStatusNoGoals,
		}
		if status, ok := statuses[child.Team.ID]; ok {
			item.Status = status
		}
		var goalsList []domain.Goal
		if goalsByTeam != nil {
			goalsList = goalsByTeam[child.Team.ID]
		} else {
			goalsList, err = s.goals.ListGoalsByTeamPeriod(ctx, scope, child.Team.ID, periodID)
			if err != nil {
				return nil, err
			}
		}
		item.GoalsCount = len(goalsList)
		item.HasGoals = item.GoalsCount > 0
		if item.HasGoals {
			for i := range goalsList {
				goalsList[i].Progress = CalculateGoalProgress(&goalsList[i])
			}
			item.Progress = progress.PeriodProgress(goalsList)
			for _, g := range goalsList {
				if g.Priority == domain.PriorityP0 || g.Priority == domain.PriorityP1 {
					item.HighPriorityCount++
				}
			}
		}
		if updatedAt, ok := lastUpdates[child.Team.ID]; ok {
			value := updatedAt
			item.LastUpdateAt = &value
		}
		result = append(result, item)
	}
	return result, nil
}

func (s *Service) GetTeamOverview(ctx context.Context, scope domain.TenantScope, teamID, periodID int64) (TeamOverview, error) {
	allTeams, err := s.teams.ListAllTeams(ctx, scope)
	if err != nil {
		return TeamOverview{}, err
	}
	hierarchy, err := s.getHierarchyFromTeams(ctx, scope, allTeams, &periodID)
	if err != nil {
		return TeamOverview{}, err
	}
	children := findDirectChildren(teamID, hierarchy)
	descendantIDs := collectDescendantIDs(teamID, hierarchy)
	goalsByTeam, err := s.goals.ListGoalsByTeamsPeriod(ctx, scope, periodID, descendantIDs)
	if err != nil {
		return TeamOverview{}, err
	}
	summaryByID := make(map[int64]TeamSummary, len(descendantIDs))
	for _, id := range descendantIDs {
		goalsList := goalsByTeam[id]
		if len(goalsList) == 0 {
			continue
		}
		for i := range goalsList {
			goalsList[i].Progress = CalculateGoalProgress(&goalsList[i])
		}
		summaryByID[id] = TeamSummary{
			ID:             id,
			GoalsCount:     len(goalsList),
			PeriodProgress: progress.PeriodProgress(goalsList),
		}
	}
	childrenSummary := []TeamChildSummary{}
	if len(children) > 0 {
		childrenSummary, err = s.buildDirectChildrenSummary(ctx, scope, periodID, children, goalsByTeam)
		if err != nil {
			return TeamOverview{}, err
		}
	}

	totalProgress := 0
	teamsWithGoals := 0

	for _, id := range descendantIDs {
		summary, ok := summaryByID[id]
		if !ok || summary.GoalsCount == 0 {
			continue
		}
		teamsWithGoals++
		totalProgress += summary.PeriodProgress
	}

	avgProgress := 0
	if teamsWithGoals > 0 {
		avgProgress = totalProgress / teamsWithGoals
	}

	return TeamOverview{
		AverageProgress: avgProgress,
		TeamsWithGoals:  teamsWithGoals,
		ChildrenSummary: childrenSummary,
	}, nil
}

func findDirectChildren(targetID int64, nodes []TeamNode) []TeamNode {
	var children []TeamNode
	var walk func(items []TeamNode) bool
	walk = func(items []TeamNode) bool {
		for _, node := range items {
			if node.Team.ID == targetID {
				children = node.Children
				return true
			}
			if walk(node.Children) {
				return true
			}
		}
		return false
	}
	_ = walk(nodes)
	return children
}

func collectDescendantIDs(targetID int64, nodes []TeamNode) []int64 {
	var descendants []int64
	var walk func(items []TeamNode, collect bool)
	walk = func(items []TeamNode, collect bool) {
		for _, node := range items {
			nextCollect := collect || node.Team.ID == targetID
			if collect {
				descendants = append(descendants, node.Team.ID)
			}
			if len(node.Children) > 0 {
				walk(node.Children, nextCollect)
			}
		}
	}
	walk(nodes, false)
	return descendants
}

// recordKRProgress records a progress event with explicit before/after percent (0..100).
// The caller computes the percentages from the KR's meta because store.GetKeyResult does not
// populate the computed KeyResult.Progress field.
func (s *Service) recordKRProgress(ctx context.Context, scope domain.TenantScope, krID int64, kr domain.KeyResult, beforeProg, afterProg int, actorUserID int64) {
	g, gerr := s.goals.GetGoal(ctx, scope, kr.GoalID)
	if gerr != nil {
		return
	}
	teamID, periodID, goalID, krRef := g.TeamID, g.PeriodID, g.ID, krID
	s.recordActivity(ctx, scope, domain.ActivityEvent{
		ActorUserID: actorUserID, Category: domain.ActivityProgress, Action: domain.ActionKRProgress,
		TeamID: &teamID, PeriodID: &periodID, GoalID: &goalID, KRID: &krRef, EntityTitle: kr.Title,
		Payload: map[string]any{
			"before":     map[string]any{"progress": beforeProg},
			"after":      map[string]any{"progress": afterProg},
			"kind":       string(kr.Kind),
			"goal_title": g.Title,
		},
	})
}

func (s *Service) UpdateKRProgressNumerical(ctx context.Context, scope domain.TenantScope, krID int64, current float64, actorUserID int64) error {
	kr, err := s.krs.GetKeyResult(ctx, scope, krID)
	if err != nil {
		return err
	}
	if kr.Kind != domain.KRKindNumerical {
		return fmt.Errorf("unsupported kr kind for numerical update: %s", kr.Kind)
	}
	if err := s.krs.UpdateNumericalCurrent(ctx, scope, krID, current); err != nil {
		return err
	}
	if n := kr.Numerical; n != nil {
		beforeProg := progress.NumericalProgress(n.StartValue, n.TargetValue, n.CurrentValue, n.Checkpoints)
		afterProg := progress.NumericalProgress(n.StartValue, n.TargetValue, current, n.Checkpoints)
		s.recordKRProgress(ctx, scope, krID, kr, beforeProg, afterProg, actorUserID)
		s.autoCompleteHealth(ctx, scope, krID, kr, beforeProg, afterProg)
	}
	return nil
}

// UpdateKRHealthStatus sets the manual health status of a KR. Access is checked by the caller
// (same as progress update). Health status is informational and does not affect progress math.
func (s *Service) UpdateKRHealthStatus(ctx context.Context, scope domain.TenantScope, krID int64, status domain.KRHealthStatus) error {
	if !domain.IsValidKRHealthStatus(string(status)) {
		return fmt.Errorf("invalid health status: %s", status)
	}
	return s.krs.UpdateHealthStatus(ctx, scope, krID, status)
}

// autoCompleteHealth sets health=done exactly once, on the progress transition <100 -> =100,
// and only if the KR is not already done. Never reverts on later drops. kr is the pre-update state.
func (s *Service) autoCompleteHealth(ctx context.Context, scope domain.TenantScope, krID int64, kr domain.KeyResult, before, after int) {
	if before < 100 && after == 100 && kr.HealthStatus != domain.KRHealthDone {
		// best-effort: an auto-complete failure must not fail the progress mutation
		_ = s.krs.UpdateHealthStatus(ctx, scope, krID, domain.KRHealthDone)
	}
}

func (s *Service) UpdateKRProgressBoolean(ctx context.Context, scope domain.TenantScope, krID int64, done bool, actorUserID int64) error {
	kr, err := s.krs.GetKeyResult(ctx, scope, krID)
	if err != nil {
		return err
	}
	if kr.Kind != domain.KRKindBoolean {
		return fmt.Errorf("unsupported kr kind for boolean update: %s", kr.Kind)
	}
	beforeDone := false
	if bm, berr := s.krs.GetBooleanMeta(ctx, scope, krID); berr == nil && bm != nil {
		beforeDone = bm.IsDone
	}
	if err := s.krs.UpdateBoolean(ctx, scope, krID, done); err != nil {
		return err
	}
	beforeProg := progress.BooleanProgress(beforeDone)
	afterProg := progress.BooleanProgress(done)
	s.recordKRProgress(ctx, scope, krID, kr, beforeProg, afterProg, actorUserID)
	s.autoCompleteHealth(ctx, scope, krID, kr, beforeProg, afterProg)
	return nil
}

type ProjectStageUpdate struct {
	ID     int64
	IsDone bool
}

func (s *Service) UpdateKRProgressProject(ctx context.Context, scope domain.TenantScope, krID int64, updates []ProjectStageUpdate, actorUserID int64) error {
	kr, err := s.krs.GetKeyResult(ctx, scope, krID)
	if err != nil {
		return err
	}
	if kr.Kind != domain.KRKindProject {
		return fmt.Errorf("unsupported kr kind for project update: %s", kr.Kind)
	}
	stages, err := s.krs.ListProjectStages(ctx, scope, krID)
	if err != nil {
		return err
	}
	updatesByID := make(map[int64]bool, len(updates))
	for _, u := range updates {
		updatesByID[u.ID] = u.IsDone
	}
	validUpdates := make(map[int64]bool, len(updates))
	for _, stage := range stages {
		if done, ok := updatesByID[stage.ID]; ok {
			validUpdates[stage.ID] = done
		}
	}
	beforeProg := progress.ProjectProgress(stages)
	if err := s.krs.BatchUpdateProjectStagesDone(ctx, scope, krID, validUpdates); err != nil {
		return err
	}
	afterStages := make([]domain.KRProjectStage, len(stages))
	copy(afterStages, stages)
	for i := range afterStages {
		if done, ok := validUpdates[afterStages[i].ID]; ok {
			afterStages[i].IsDone = done
		}
	}
	afterProg := progress.ProjectProgress(afterStages)
	s.recordKRProgress(ctx, scope, krID, kr, beforeProg, afterProg, actorUserID)
	s.autoCompleteHealth(ctx, scope, krID, kr, beforeProg, afterProg)
	return nil
}

type ShareTarget struct {
	TeamID int64
	Weight int
}

func (s *Service) ShareGoal(ctx context.Context, scope domain.TenantScope, goalID int64, targets []ShareTarget, actorUserID int64) error {
	goal, err := s.goals.GetGoal(ctx, scope, goalID)
	if err != nil {
		return err
	}
	// Validate every target team belongs to the active tenant before writing shares. The share
	// repository only scopes the goal, so an unchecked target team_id could attach the goal to a
	// team in another tenant. One scoped lookup builds the allow-set (no per-target query).
	if len(targets) > 0 {
		tenantTeams, err := s.teams.ListAllTeams(ctx, scope)
		if err != nil {
			return err
		}
		valid := make(map[int64]struct{}, len(tenantTeams))
		for _, t := range tenantTeams {
			valid[t.ID] = struct{}{}
		}
		for _, target := range targets {
			if _, ok := valid[target.TeamID]; !ok {
				return ErrShareTargetNotInTenant
			}
		}
	}
	// The /share endpoint replaces the whole goal_shares set, so diff the current set against the
	// new targets to log ADDING teams (goal_shared) and REMOVING teams (goal_unshared) separately,
	// and to guard only NEWLY added teams below. The read error must propagate: swallowing it would
	// leave beforeSet empty, misclassify every target as newly added, and could reject an unchanged
	// save with 409 just because an existing participant is already in_progress/closed.
	cur, err := s.shares.ListGoalShares(ctx, scope, goalID)
	if err != nil {
		return err
	}
	beforeSet := make(map[int64]bool, len(cur))
	for _, sh := range cur {
		beforeSet[sh.TeamID] = true
	}
	shareInputs := make([]shares.GoalShareInput, 0, len(targets))
	newSet := map[int64]bool{}
	for _, target := range targets {
		shareInputs = append(shareInputs, shares.GoalShareInput{TeamID: target.TeamID, Weight: target.Weight})
		newSet[target.TeamID] = true
	}
	var added, removed []int64
	for _, target := range targets {
		if !beforeSet[target.TeamID] {
			added = append(added, target.TeamID)
		}
	}
	for teamID := range beforeSet {
		if !newSet[teamID] {
			removed = append(removed, teamID)
		}
	}
	// Guard: a team whose period is already in_progress or closed cannot be NEWLY added as a share
	// target — its OKR set for the period is locked, so a shared goal must not appear after the
	// fact. Only newly added teams are checked (one batched status lookup); teams already sharing
	// the goal are left untouched even if their period has since advanced. This mirrors the UI,
	// which greys out such teams and blocks selection, but is enforced server-side as source of truth.
	if len(added) > 0 {
		statuses, serr := s.statuses.ListTeamPeriodStatuses(ctx, scope, goal.PeriodID, added)
		if serr != nil {
			return serr
		}
		for _, teamID := range added {
			switch statuses[teamID] {
			case domain.TeamPeriodStatusInProgress, domain.TeamPeriodStatusClosed:
				return ErrCannotShareWithClosedPeriod
			}
		}
	}
	if err := s.shares.ReplaceGoalShares(ctx, scope, goalID, shareInputs); err != nil {
		return err
	}
	teamID, periodID := goal.TeamID, goal.PeriodID
	if len(added) > 0 {
		s.recordActivity(ctx, scope, domain.ActivityEvent{
			ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionGoalShared,
			TeamID: &teamID, PeriodID: &periodID, GoalID: &goalID, EntityTitle: goal.Title,
			Payload: map[string]any{"shared_with_team_ids": added},
		})
	}
	if len(removed) > 0 {
		s.recordActivity(ctx, scope, domain.ActivityEvent{
			ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionGoalUnshared,
			TeamID: &teamID, PeriodID: &periodID, GoalID: &goalID, EntityTitle: goal.Title,
			Payload: map[string]any{"unshared_team_ids": removed},
		})
	}
	return nil
}

func (s *Service) UpdateGoalWeight(ctx context.Context, scope domain.TenantScope, goalID, teamID int64, weight int) error {
	return s.shares.UpdateGoalTeamWeight(ctx, scope, goalID, teamID, weight)
}

func (s *Service) AddGoalComment(ctx context.Context, scope domain.TenantScope, goalID int64, text string, authorUserID int64) error {
	commentID, err := s.goals.AddGoalComment(ctx, scope, goalID, text, authorUserID)
	if err != nil {
		return err
	}
	if g, gerr := s.goals.GetGoal(ctx, scope, goalID); gerr == nil {
		teamID, periodID := g.TeamID, g.PeriodID
		s.recordActivity(ctx, scope, domain.ActivityEvent{
			ActorUserID: authorUserID, Category: domain.ActivityDiscussion, Action: domain.ActionCommentAdded,
			TeamID: &teamID, PeriodID: &periodID, GoalID: &goalID, CommentID: &commentID,
			EntityTitle: g.Title, Payload: map[string]any{"text": text},
		})
	}
	return nil
}

func (s *Service) SetGoalCommentResolved(ctx context.Context, scope domain.TenantScope, goalID, commentID int64, resolved bool, userID int64) error {
	changed, err := s.goals.SetGoalCommentResolved(ctx, scope, goalID, commentID, resolved, userID)
	if err != nil {
		return err
	}
	if !changed {
		return nil // already in the target state → no event, no re-stamp
	}
	if g, gerr := s.goals.GetGoal(ctx, scope, goalID); gerr == nil {
		action := domain.ActionCommentReopened
		if resolved {
			action = domain.ActionCommentResolved
		}
		teamID, periodID := g.TeamID, g.PeriodID
		s.recordActivity(ctx, scope, domain.ActivityEvent{
			ActorUserID: userID, Category: domain.ActivityDiscussion, Action: action,
			TeamID: &teamID, PeriodID: &periodID, GoalID: &goalID, CommentID: &commentID,
			EntityTitle: g.Title,
			Payload:     map[string]any{"before": map[string]any{"resolved": !resolved}, "after": map[string]any{"resolved": resolved}},
		})
	}
	return nil
}

func (s *Service) AddGoalReply(ctx context.Context, scope domain.TenantScope, goalID, parentID int64, text string, authorUserID int64) error {
	replyID, err := s.goals.AddGoalReply(ctx, scope, goalID, parentID, text, authorUserID)
	if err != nil {
		return err // includes goals.ErrNotFound for a bad/non-task parent
	}
	if g, gerr := s.goals.GetGoal(ctx, scope, goalID); gerr == nil {
		teamID, periodID := g.TeamID, g.PeriodID
		s.recordActivity(ctx, scope, domain.ActivityEvent{
			ActorUserID: authorUserID, Category: domain.ActivityDiscussion, Action: domain.ActionReplyAdded,
			TeamID: &teamID, PeriodID: &periodID, GoalID: &goalID, CommentID: &replyID,
			EntityTitle: g.Title, Payload: map[string]any{"text": text},
		})
	}
	return nil
}

// DeleteGoalComment removes a task (cascading its replies) or a reply. Authorization:
// the requesting user must be the author, or a tenant admin. Returns isTask so the
// caller/log distinguishes a task deletion (comment_deleted) from a reply (reply_deleted).
// A cascaded task deletion logs a single comment_deleted event (replies vanish silently).
func (s *Service) DeleteGoalComment(ctx context.Context, scope domain.TenantScope, goalID, commentID, requestingUserID int64, isAdmin bool) (bool, error) {
	author, isTask, err := s.goals.GetGoalCommentMeta(ctx, scope, goalID, commentID)
	if err != nil {
		return false, err // goals.ErrNotFound if absent
	}
	if author != requestingUserID && !isAdmin {
		return false, ErrForbidden
	}
	if err := s.goals.DeleteGoalComment(ctx, scope, goalID, commentID); err != nil {
		return false, err
	}
	action := domain.ActionReplyDeleted
	if isTask {
		action = domain.ActionCommentDeleted
	}
	// The goal is not deleted by removing a comment, so it is still readable for the
	// team/period/title snapshot of the journal entry.
	if g, gerr := s.goals.GetGoal(ctx, scope, goalID); gerr == nil {
		teamID, periodID := g.TeamID, g.PeriodID
		s.recordActivity(ctx, scope, domain.ActivityEvent{
			ActorUserID: requestingUserID, Category: domain.ActivityDiscussion, Action: action,
			TeamID: &teamID, PeriodID: &periodID, GoalID: &goalID, CommentID: &commentID,
			EntityTitle: g.Title,
		})
	}
	return isTask, nil
}

func (s *Service) UpsertKeyResultNote(ctx context.Context, scope domain.TenantScope, krID int64, text string, authorUserID int64) error {
	beforeText := ""
	if before, berr := s.krs.GetKeyResultNote(ctx, scope, krID); berr == nil && before != nil {
		beforeText = before.Text
	}
	if err := s.krs.UpsertKeyResultNote(ctx, scope, krID, text, authorUserID); err != nil {
		return err
	}
	if beforeText != text {
		if kr, kerr := s.krs.GetKeyResult(ctx, scope, krID); kerr == nil {
			if g, gerr := s.goals.GetGoal(ctx, scope, kr.GoalID); gerr == nil {
				teamID, periodID, goalID, krRef := g.TeamID, g.PeriodID, g.ID, krID
				s.recordActivity(ctx, scope, domain.ActivityEvent{
					ActorUserID: authorUserID, Category: domain.ActivityDiscussion, Action: domain.ActionKRNoteUpdated,
					TeamID: &teamID, PeriodID: &periodID, GoalID: &goalID, KRID: &krRef, EntityTitle: kr.Title,
					Payload: map[string]any{"before": map[string]any{"note": beforeText}, "after": map[string]any{"note": text}},
				})
			}
		}
	}
	return nil
}

func (s *Service) UpdateKeyResultDescription(ctx context.Context, scope domain.TenantScope, krID int64, description string) error {
	return s.krs.UpdateKeyResultDescription(ctx, scope, krID, description)
}

func (s *Service) GetKeyResult(ctx context.Context, scope domain.TenantScope, id int64) (domain.KeyResult, error) {
	return s.krs.GetKeyResult(ctx, scope, id)
}

func (s *Service) GetGoal(ctx context.Context, scope domain.TenantScope, id int64) (domain.Goal, error) {
	goal, err := s.goals.GetGoal(ctx, scope, id)
	if err != nil {
		return domain.Goal{}, err
	}
	goal.Progress = CalculateGoalProgress(&goal)
	return goal, nil
}

type KeyResultMetaInput struct {
	NumericalStart       float64
	NumericalTarget      float64
	NumericalCurrent     float64
	NumericalUnit        string
	NumericalCheckpoints []domain.KRNumericalCheckpoint
	BooleanDone          bool
	ProjectStages        []krs.ProjectStageInput
}

func (s *Service) UpdateGoal(ctx context.Context, scope domain.TenantScope, input goals.GoalUpdateInput, actorUserID int64) error {
	before, _ := s.goals.GetGoal(ctx, scope, input.ID)
	if err := s.goals.UpdateGoal(ctx, scope, input); err != nil {
		return err
	}
	if after, aerr := s.goals.GetGoal(ctx, scope, input.ID); aerr == nil {
		changed := diffFields(map[string][2]any{
			"title":       {before.Title, after.Title},
			"description": {before.Description, after.Description},
			"priority":    {string(before.Priority), string(after.Priority)},
			"weight":      {before.Weight, after.Weight},
		})
		if len(changed) > 0 {
			teamID, periodID, gid := after.TeamID, after.PeriodID, after.ID
			s.recordActivity(ctx, scope, domain.ActivityEvent{
				ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionGoalFieldsChanged,
				TeamID: &teamID, PeriodID: &periodID, GoalID: &gid, EntityTitle: after.Title,
				Payload: map[string]any{"changed": changed},
			})
		}
	}
	return nil
}

func (s *Service) MoveGoal(ctx context.Context, scope domain.TenantScope, teamID, goalID int64, direction int) error {
	return s.goals.MoveGoal(ctx, scope, teamID, goalID, direction)
}

func (s *Service) MoveKeyResult(ctx context.Context, scope domain.TenantScope, krID int64, direction int) error {
	return s.krs.MoveKeyResult(ctx, scope, krID, direction)
}

func (s *Service) CreateKeyResultWithMeta(ctx context.Context, scope domain.TenantScope, input krs.KeyResultInput, meta KeyResultMetaInput, actorUserID int64) (int64, error) {
	krID, err := s.krs.CreateKeyResult(ctx, scope, input)
	if err != nil {
		return 0, err
	}
	if err := s.applyKeyResultMeta(ctx, scope, krID, input.Kind, meta); err != nil {
		return 0, err
	}
	if g, gerr := s.goals.GetGoal(ctx, scope, input.GoalID); gerr == nil {
		teamID, periodID, goalID := g.TeamID, g.PeriodID, input.GoalID
		s.recordActivity(ctx, scope, domain.ActivityEvent{
			ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionKRCreated,
			TeamID: &teamID, PeriodID: &periodID, GoalID: &goalID, KRID: &krID, EntityTitle: input.Title,
		})
	}
	return krID, nil
}

func (s *Service) UpdateKeyResultWithMeta(ctx context.Context, scope domain.TenantScope, input krs.KeyResultUpdateInput, meta KeyResultMetaInput, actorUserID int64) error {
	before, _ := s.krs.GetKeyResult(ctx, scope, input.ID)
	if err := s.krs.UpdateKeyResult(ctx, scope, input); err != nil {
		return err
	}
	if err := s.applyKeyResultMeta(ctx, scope, input.ID, input.Kind, meta); err != nil {
		return err
	}
	if after, aerr := s.krs.GetKeyResult(ctx, scope, input.ID); aerr == nil {
		changed := diffFields(map[string][2]any{
			"title":       {before.Title, after.Title},
			"description": {before.Description, after.Description},
			"weight":      {before.Weight, after.Weight},
		})
		if len(changed) > 0 {
			if g, gerr := s.goals.GetGoal(ctx, scope, after.GoalID); gerr == nil {
				teamID, periodID, gid, krID := g.TeamID, g.PeriodID, g.ID, input.ID
				s.recordActivity(ctx, scope, domain.ActivityEvent{
					ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionKRFieldsChanged,
					TeamID: &teamID, PeriodID: &periodID, GoalID: &gid, KRID: &krID, EntityTitle: after.Title,
					Payload: map[string]any{"changed": changed},
				})
			}
		}
	}
	return nil
}

func (s *Service) applyKeyResultMeta(ctx context.Context, scope domain.TenantScope, krID int64, kind domain.KRKind, meta KeyResultMetaInput) error {
	switch kind {
	case domain.KRKindNumerical:
		return s.krs.UpsertNumericalMeta(ctx, scope, krs.NumericalMetaInput{
			KeyResultID:  krID,
			StartValue:   meta.NumericalStart,
			TargetValue:  meta.NumericalTarget,
			CurrentValue: meta.NumericalCurrent,
			Unit:         meta.NumericalUnit,
			Checkpoints:  meta.NumericalCheckpoints,
		})
	case domain.KRKindBoolean:
		return s.krs.UpsertBooleanMeta(ctx, scope, krID, meta.BooleanDone)
	case domain.KRKindProject:
		return s.krs.ReplaceProjectStages(ctx, scope, krID, meta.ProjectStages)
	default:
		return nil
	}
}

func (s *Service) UpdateTeamPeriodStatus(ctx context.Context, scope domain.TenantScope, teamID, periodID int64, status domain.TeamPeriodStatus, actorUserID int64) error {
	before, _ := s.statuses.GetTeamPeriodStatus(ctx, scope, teamID, periodID)
	if err := s.statuses.SetTeamPeriodStatus(ctx, scope, teamID, periodID, status); err != nil {
		return err
	}
	title := ""
	if team, terr := s.teams.GetTeam(ctx, scope, teamID); terr == nil {
		title = team.Name
	}
	tID, pID := teamID, periodID
	s.recordActivity(ctx, scope, domain.ActivityEvent{
		ActorUserID: actorUserID, Category: domain.ActivityStatus, Action: domain.ActionStatusChanged,
		TeamID: &tID, PeriodID: &pID, EntityTitle: title,
		Payload: map[string]any{"before": map[string]any{"status": string(before)}, "after": map[string]any{"status": string(status)}},
	})
	return nil
}

func (s *Service) DeleteTeam(ctx context.Context, scope domain.TenantScope, teamID int64) error {
	hasGoals, err := s.teams.TeamHasGoals(ctx, scope, teamID)
	if err != nil {
		return err
	}
	if hasGoals {
		return s.teams.SoftDeleteTeam(ctx, scope, teamID)
	}
	return s.teams.HardDeleteTeam(ctx, scope, teamID)
}

func (s *Service) RestoreTeam(ctx context.Context, scope domain.TenantScope, teamID int64) error {
	return s.teams.RestoreTeam(ctx, scope, teamID)
}

func (s *Service) HardDeleteTeam(ctx context.Context, scope domain.TenantScope, teamID int64) error {
	hasGoals, err := s.teams.TeamHasGoals(ctx, scope, teamID)
	if err != nil {
		return err
	}
	if hasGoals {
		return ErrTeamHasGoals
	}
	return s.teams.HardDeleteTeam(ctx, scope, teamID)
}

func (s *Service) isTeamVisibleInPeriod(ctx context.Context, scope domain.TenantScope, team domain.Team, periodID int64) (bool, error) {
	hasGoals, err := s.teams.TeamHasGoalsInPeriod(ctx, scope, team.ID, periodID)
	if err != nil {
		return false, err
	}
	if hasGoals {
		return true, nil
	}
	return team.DeletedAt == nil, nil
}

func buildTeamHierarchy(allTeams []domain.Team) (map[int64]domain.Team, map[int64][]domain.Team, []domain.Team) {
	teamsByID := make(map[int64]domain.Team, len(allTeams))
	childrenMap := make(map[int64][]domain.Team)
	roots := make([]domain.Team, 0)
	for _, team := range allTeams {
		teamsByID[team.ID] = team
	}
	for _, team := range allTeams {
		if team.ParentID != nil {
			if _, ok := teamsByID[*team.ParentID]; ok {
				childrenMap[*team.ParentID] = append(childrenMap[*team.ParentID], team)
				continue
			}
		}
		roots = append(roots, team)
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i].Name < roots[j].Name })
	return teamsByID, childrenMap, roots
}

func buildTeamNode(team domain.Team, childrenMap map[int64][]domain.Team) TeamNode {
	children := childrenMap[team.ID]
	sort.Slice(children, func(i, j int) bool { return children[i].Name < children[j].Name })
	node := TeamNode{Team: team}
	for _, child := range children {
		node.Children = append(node.Children, buildTeamNode(child, childrenMap))
	}
	return node
}

// — Team passthroughs —

func (s *Service) ListTeams(ctx context.Context, scope domain.TenantScope) ([]domain.Team, error) {
	return s.teams.ListTeams(ctx, scope)
}

func (s *Service) ListDeletedTeams(ctx context.Context, scope domain.TenantScope) ([]domain.Team, error) {
	return s.teams.ListDeletedTeams(ctx, scope)
}

func (s *Service) ListAllTeams(ctx context.Context, scope domain.TenantScope) ([]domain.Team, error) {
	return s.teams.ListAllTeams(ctx, scope)
}

func (s *Service) CreateTeam(ctx context.Context, scope domain.TenantScope, input teams.TeamInput) (int64, error) {
	return s.teams.CreateTeam(ctx, scope, input)
}

func (s *Service) UpdateTeam(ctx context.Context, scope domain.TenantScope, input teams.TeamInput, id int64) error {
	return s.teams.UpdateTeam(ctx, scope, input, id)
}

// — Period passthroughs —

func (s *Service) FindPeriodForDate(ctx context.Context, scope domain.TenantScope, date time.Time) (domain.Period, error) {
	return s.periods.FindPeriodForDate(ctx, scope, date)
}

func (s *Service) CreatePeriod(ctx context.Context, scope domain.TenantScope, input periods.PeriodInput) (int64, error) {
	return s.periods.CreatePeriod(ctx, scope, input)
}

func (s *Service) UpdatePeriod(ctx context.Context, scope domain.TenantScope, periodID int64, input periods.PeriodInput) error {
	return s.periods.UpdatePeriod(ctx, scope, periodID, input)
}

func (s *Service) DeletePeriod(ctx context.Context, scope domain.TenantScope, periodID int64) error {
	return s.periods.DeletePeriod(ctx, scope, periodID)
}

// ArchivePeriod archives a period, but only once it is closed — archiving an active or future
// period would hide it from the tree while it's still in use.
func (s *Service) ArchivePeriod(ctx context.Context, scope domain.TenantScope, periodID int64) error {
	p, err := s.periods.GetPeriod(ctx, scope, periodID)
	if err != nil {
		return err
	}
	if domain.PeriodStatusFor(p, time.Now()) != domain.PeriodStatusClosed {
		return ErrPeriodNotClosed
	}
	return s.periods.ArchivePeriod(ctx, scope, periodID)
}

func (s *Service) UnarchivePeriod(ctx context.Context, scope domain.TenantScope, periodID int64) error {
	return s.periods.UnarchivePeriod(ctx, scope, periodID)
}

func (s *Service) GetUsersByDisplayNames(ctx context.Context, names []string) ([]*domain.User, error) {
	return s.users.GetUsersByDisplayNames(ctx, names)
}

func (s *Service) GetUsersByUDIDs(ctx context.Context, udids []string) ([]*domain.User, error) {
	return s.users.GetUsersByUDIDs(ctx, udids)
}

func (s *Service) ListUserLeadTeams(ctx context.Context) (map[string]string, error) {
	return s.users.ListUserLeadTeams(ctx)
}

func (s *Service) ValidateUserUDIDsExist(ctx context.Context, udids []string) ([]string, error) {
	return s.users.ValidateUDIDsExist(ctx, udids)
}

// SearchUsersInScope returns up to 20 non-system users visible in the given scope.
//   - scopeTeamIDs == nil → admin/unrestricted: all users
//   - scopeTeamIDs != nil → users with a hierarchy grant to any team related to the scope nodes:
//     ancestors (access from above), the nodes themselves, or descendants (access from below).
//
// Uses the GrantsProvider cache; falls back to empty result when cache is unavailable.
func (s *Service) SearchUsersInScope(ctx context.Context, scope domain.TenantScope, scopeTeamIDs []int64, q string, limit int) ([]*domain.User, error) {
	if limit <= 0 {
		limit = 20
	}
	if scopeTeamIDs == nil {
		return s.users.SearchUsersUnrestricted(ctx, q, limit)
	}
	if len(scopeTeamIDs) == 0 || s.grants == nil {
		return nil, nil
	}

	allTeams, err := s.teams.ListAllTeams(ctx, scope)
	if err != nil {
		return nil, err
	}

	// Build both maps for bidirectional tree traversal.
	parentMap := make(map[int64]int64, len(allTeams))
	childrenMap := make(map[int64][]int64, len(allTeams))
	for _, t := range allTeams {
		if t.ParentID != nil {
			parentMap[t.ID] = *t.ParentID
			childrenMap[*t.ParentID] = append(childrenMap[*t.ParentID], t.ID)
		}
	}

	// Related set: scope nodes + all their ancestors + all their descendants.
	relatedSet := make(map[int64]struct{})
	for _, id := range scopeTeamIDs {
		// Walk up.
		cur := id
		for {
			relatedSet[cur] = struct{}{}
			parent, ok := parentMap[cur]
			if !ok {
				break
			}
			cur = parent
		}
		// Walk down via BFS.
		queue := []int64{id}
		for len(queue) > 0 {
			cur, queue = queue[0], queue[1:]
			for _, child := range childrenMap[cur] {
				if _, visited := relatedSet[child]; !visited {
					relatedSet[child] = struct{}{}
					queue = append(queue, child)
				}
			}
		}
	}

	allGrants, err := s.grants.AllGrants(ctx)
	if err != nil {
		return nil, err
	}

	// Collect IDs of users whose grants intersect the related set.
	eligibleIDs := make([]int64, 0)
	seen := make(map[int64]struct{})
	for userID, userGrants := range allGrants {
		for _, g := range userGrants {
			if _, ok := relatedSet[g.TeamID]; ok {
				if _, dup := seen[userID]; !dup {
					seen[userID] = struct{}{}
					eligibleIDs = append(eligibleIDs, userID)
				}
				break
			}
		}
	}

	// Team leads of all related nodes are eligible regardless of explicit grants.
	leadUDIDs := make([]string, 0)
	for _, t := range allTeams {
		if _, ok := relatedSet[t.ID]; ok && t.LeadUDID != nil && t.DeletedAt == nil {
			leadUDIDs = append(leadUDIDs, *t.LeadUDID)
		}
	}

	return s.users.SearchUsersInSet(ctx, eligibleIDs, leadUDIDs, q, limit)
}

// — Goal passthroughs —

func (s *Service) ListGoalsByTeamPeriod(ctx context.Context, scope domain.TenantScope, teamID, periodID int64) ([]domain.Goal, error) {
	return s.goals.ListGoalsByTeamPeriod(ctx, scope, teamID, periodID)
}

func (s *Service) UpdateGoalFields(ctx context.Context, scope domain.TenantScope, input goals.GoalFieldsUpdateInput, actorUserID int64) error {
	before, _ := s.goals.GetGoal(ctx, scope, input.ID)
	if err := s.goals.UpdateGoalFields(ctx, scope, input); err != nil {
		return err
	}
	if after, aerr := s.goals.GetGoal(ctx, scope, input.ID); aerr == nil {
		changed := diffFields(map[string][2]any{
			"title":       {before.Title, after.Title},
			"description": {before.Description, after.Description},
			"priority":    {string(before.Priority), string(after.Priority)},
		})
		if len(changed) > 0 {
			teamID, periodID, gid := after.TeamID, after.PeriodID, after.ID
			s.recordActivity(ctx, scope, domain.ActivityEvent{
				ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionGoalFieldsChanged,
				TeamID: &teamID, PeriodID: &periodID, GoalID: &gid, EntityTitle: after.Title,
				Payload: map[string]any{"changed": changed},
			})
		}
	}
	return nil
}

func (s *Service) ListGoalComments(ctx context.Context, scope domain.TenantScope, goalID int64) ([]domain.GoalComment, error) {
	return s.goals.ListGoalComments(ctx, scope, goalID)
}

func (s *Service) GetTeamPeriodStatus(ctx context.Context, scope domain.TenantScope, teamID, periodID int64) (domain.TeamPeriodStatus, error) {
	return s.statuses.GetTeamPeriodStatus(ctx, scope, teamID, periodID)
}

func (s *Service) GetGoalShare(ctx context.Context, scope domain.TenantScope, goalID, teamID int64) (shares.GoalShare, error) {
	return s.shares.GetGoalShare(ctx, scope, goalID, teamID)
}

func (s *Service) DeleteGoalShare(ctx context.Context, scope domain.TenantScope, goalID, teamID int64, actorUserID int64) error {
	g, _ := s.goals.GetGoal(ctx, scope, goalID)
	if err := s.shares.DeleteGoalShare(ctx, scope, goalID, teamID); err != nil {
		return err
	}
	ownerTeam, periodID, shareTeam := g.TeamID, g.PeriodID, teamID
	s.recordActivity(ctx, scope, domain.ActivityEvent{
		ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionGoalUnshared,
		TeamID: &ownerTeam, PeriodID: &periodID, GoalID: &goalID, EntityTitle: g.Title,
		Payload: map[string]any{"unshared_team_id": shareTeam},
	})
	return nil
}

func (s *Service) ListGoalShares(ctx context.Context, scope domain.TenantScope, goalID int64) ([]shares.GoalShare, error) {
	return s.shares.ListGoalShares(ctx, scope, goalID)
}

// — Key result passthroughs —

func (s *Service) DeleteKeyResult(ctx context.Context, scope domain.TenantScope, id int64, actorUserID int64) error {
	kr, _ := s.krs.GetKeyResult(ctx, scope, id)
	if err := s.krs.DeleteKeyResult(ctx, scope, id); err != nil {
		return err
	}
	var g domain.Goal
	if kr.GoalID != 0 {
		g, _ = s.goals.GetGoal(ctx, scope, kr.GoalID)
	}
	teamID, periodID, goalID, krID := g.TeamID, g.PeriodID, g.ID, id
	s.recordActivity(ctx, scope, domain.ActivityEvent{
		ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionKRDeleted,
		TeamID: &teamID, PeriodID: &periodID, GoalID: &goalID, KRID: &krID, EntityTitle: kr.Title,
	})
	return nil
}

func (s *Service) FindGoalIDByKR(ctx context.Context, scope domain.TenantScope, krID int64) (int64, error) {
	return s.krs.FindGoalIDByKR(ctx, scope, krID)
}

func (s *Service) FindGoalIDByStage(ctx context.Context, scope domain.TenantScope, stageID int64) (int64, error) {
	return s.krs.FindGoalIDByStage(ctx, scope, stageID)
}

// — Business logic —

// CreateGoal creates a goal and auto-advances status from NoGoals to Forming on first goal.
// Returns ErrPeriodClosed if the team's period status is InProgress or Closed.
func (s *Service) CreateGoal(ctx context.Context, scope domain.TenantScope, input goals.GoalInput, actorUserID int64) (int64, error) {
	status, err := s.statuses.GetTeamPeriodStatus(ctx, scope, input.TeamID, input.PeriodID)
	if err != nil {
		return 0, err
	}
	if status == domain.TeamPeriodStatusClosed || status == domain.TeamPeriodStatusInProgress {
		return 0, ErrPeriodClosed
	}
	goalID, err := s.goals.CreateGoal(ctx, scope, input)
	if err != nil {
		return 0, err
	}
	if status == domain.TeamPeriodStatusNoGoals {
		if err := s.statuses.SetTeamPeriodStatus(ctx, scope, input.TeamID, input.PeriodID, domain.TeamPeriodStatusForming); err != nil {
			return 0, err
		}
	}
	teamID, periodID := input.TeamID, input.PeriodID
	s.recordActivity(ctx, scope, domain.ActivityEvent{
		ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionGoalCreated,
		TeamID: &teamID, PeriodID: &periodID, GoalID: &goalID, EntityTitle: input.Title,
	})
	return goalID, nil
}

// CopyGoalMode selects copy (keep source) or move (copy then hard-delete source).
type CopyGoalMode string

const (
	CopyGoalModeCopy CopyGoalMode = "copy"
	CopyGoalModeMove CopyGoalMode = "move"
)

// CopyGoalParams are the inputs for CopyGoal.
type CopyGoalParams struct {
	SourceGoalID   int64
	TargetTeamID   int64
	TargetPeriodID int64
	Mode           CopyGoalMode
	WithProgress   bool
	WithComments   bool
}

// CopyGoal copies (or moves) a goal into a target team/period.
// It rejects a target whose team period status is InProgress/Closed (ErrPeriodClosed),
// and a move whose target equals the source pair (ErrTransferTargetSameAsSource).
// Shares are never carried. On move, the source is hard-deleted (cascade).
func (s *Service) CopyGoal(ctx context.Context, scope domain.TenantScope, p CopyGoalParams, actorUserID int64) (int64, error) {
	src, err := s.goals.GetGoal(ctx, scope, p.SourceGoalID)
	if err != nil {
		return 0, err
	}
	if p.Mode == CopyGoalModeMove && p.TargetTeamID == src.TeamID && p.TargetPeriodID == src.PeriodID {
		return 0, ErrTransferTargetSameAsSource
	}
	// Validate both target records live in the caller's tenant. The goals FK only enforces the
	// global period/team id, so without these scoped lookups a caller could copy into another
	// tenant's period/team (or a nonexistent one surfaces as an opaque insert error).
	if _, err := s.teams.GetTeam(ctx, scope, p.TargetTeamID); err != nil {
		return 0, ErrTransferTargetNotFound
	}
	if _, err := s.periods.GetPeriod(ctx, scope, p.TargetPeriodID); err != nil {
		return 0, ErrTransferTargetNotFound
	}
	targetStatus, err := s.statuses.GetTeamPeriodStatus(ctx, scope, p.TargetTeamID, p.TargetPeriodID)
	if err != nil {
		return 0, err
	}
	if targetStatus == domain.TeamPeriodStatusClosed || targetStatus == domain.TeamPeriodStatusInProgress {
		return 0, ErrPeriodClosed
	}

	// For a move, the copy and the source deletion run in one store transaction so the move
	// cannot partially succeed (copy committed, source left behind).
	newGoalID, err := s.goals.CopyGoal(ctx, scope, goals.CopyGoalInput{
		SourceGoalID:   p.SourceGoalID,
		TargetTeamID:   p.TargetTeamID,
		TargetPeriodID: p.TargetPeriodID,
		WithProgress:   p.WithProgress,
		WithComments:   p.WithComments,
		DeleteSource:   p.Mode == CopyGoalModeMove,
	})
	if err != nil {
		return 0, err
	}

	if targetStatus == domain.TeamPeriodStatusNoGoals {
		if err := s.statuses.SetTeamPeriodStatus(ctx, scope, p.TargetTeamID, p.TargetPeriodID, domain.TeamPeriodStatusForming); err != nil {
			return 0, err
		}
	}

	action := domain.ActionGoalCopied
	if p.Mode == CopyGoalModeMove {
		action = domain.ActionGoalMoved
	}
	tt, tp, ng := p.TargetTeamID, p.TargetPeriodID, newGoalID
	s.recordActivity(ctx, scope, domain.ActivityEvent{
		ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: action,
		TeamID: &tt, PeriodID: &tp, GoalID: &ng, EntityTitle: src.Title,
		Payload: map[string]any{
			"source_goal_id":   src.ID,
			"source_team_id":   src.TeamID,
			"source_period_id": src.PeriodID,
			"with_progress":    p.WithProgress,
			"with_comments":    p.WithComments,
		},
	})

	if p.Mode == CopyGoalModeMove {
		// Source already deleted inside the copy transaction above; reset its team status
		// if that removal left the source team with no goals in the source period.
		_ = s.resetStatusIfNoGoals(ctx, scope, src.TeamID, src.PeriodID)
	}
	return newGoalID, nil
}

// DeleteGoal removes a goal or a team's share of it, transferring ownership when the owner deletes.
// Returns the effective requesting teamID and the goal's periodID for redirect.
// Returns ErrPeriodClosed if the owner tries to delete in a closed period with no shares.
func (s *Service) DeleteGoal(ctx context.Context, scope domain.TenantScope, goalID, requestingTeamID int64, actorUserID int64) (effectiveTeamID, periodID int64, err error) {
	goal, err := s.goals.GetGoal(ctx, scope, goalID)
	if err != nil {
		return 0, 0, err
	}
	if requestingTeamID == 0 {
		requestingTeamID = goal.TeamID
	}
	if requestingTeamID != goal.TeamID {
		if err := s.shares.DeleteGoalShare(ctx, scope, goalID, requestingTeamID); err != nil {
			return 0, 0, err
		}
		// A shared team declined the goal — record it (anchored to the owner team, whose feed
		// the owner watches; payload carries the team that left).
		ownerTeam, pID, gid, decliner := goal.TeamID, goal.PeriodID, goalID, requestingTeamID
		s.recordActivity(ctx, scope, domain.ActivityEvent{
			ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionGoalUnshared,
			TeamID: &ownerTeam, PeriodID: &pID, GoalID: &gid, EntityTitle: goal.Title,
			Payload: map[string]any{"declined_by_team_id": decliner},
		})
		_ = s.resetStatusIfNoGoals(ctx, scope, requestingTeamID, goal.PeriodID)
		return requestingTeamID, goal.PeriodID, nil
	}
	shareList, err := s.shares.ListGoalShares(ctx, scope, goalID)
	if err != nil {
		return 0, 0, err
	}
	if len(shareList) > 0 {
		newOwner := shareList[0]
		if err := s.goals.UpdateGoalOwner(ctx, scope, goalID, newOwner.TeamID, newOwner.Weight); err != nil {
			return 0, 0, err
		}
		if err := s.shares.DeleteGoalShare(ctx, scope, goalID, newOwner.TeamID); err != nil {
			return 0, 0, err
		}
		// Owner "deleted" a shared goal → ownership transferred to a shared team; log the composition change.
		oldOwner, pID, gid, newOwnerTeam := goal.TeamID, goal.PeriodID, goalID, newOwner.TeamID
		s.recordActivity(ctx, scope, domain.ActivityEvent{
			ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionGoalOwnerChanged,
			TeamID: &newOwnerTeam, PeriodID: &pID, GoalID: &gid, EntityTitle: goal.Title,
			Payload: map[string]any{"before": map[string]any{"owner_team_id": oldOwner}, "after": map[string]any{"owner_team_id": newOwnerTeam}},
		})
		_ = s.resetStatusIfNoGoals(ctx, scope, requestingTeamID, goal.PeriodID)
		return requestingTeamID, goal.PeriodID, nil
	}
	status, err := s.statuses.GetTeamPeriodStatus(ctx, scope, goal.TeamID, goal.PeriodID)
	if err != nil {
		return 0, 0, err
	}
	if status == domain.TeamPeriodStatusClosed || status == domain.TeamPeriodStatusInProgress {
		return 0, 0, ErrPeriodClosed
	}
	if err := s.goals.DeleteGoal(ctx, scope, goalID); err != nil {
		return 0, 0, err
	}
	teamID, pID := goal.TeamID, goal.PeriodID
	s.recordActivity(ctx, scope, domain.ActivityEvent{
		ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionGoalDeleted,
		TeamID: &teamID, PeriodID: &pID, GoalID: &goalID, EntityTitle: goal.Title,
	})
	_ = s.resetStatusIfNoGoals(ctx, scope, requestingTeamID, goal.PeriodID)
	return requestingTeamID, goal.PeriodID, nil
}

func (s *Service) resetStatusIfNoGoals(ctx context.Context, scope domain.TenantScope, teamID, periodID int64) error {
	goalsList, err := s.goals.ListGoalsByTeamPeriod(ctx, scope, teamID, periodID)
	if err != nil || len(goalsList) > 0 {
		return err
	}
	status, err := s.statuses.GetTeamPeriodStatus(ctx, scope, teamID, periodID)
	if err != nil || status == domain.TeamPeriodStatusNoGoals {
		return nil
	}
	return s.statuses.SetTeamPeriodStatus(ctx, scope, teamID, periodID, domain.TeamPeriodStatusNoGoals)
}

// UpdateGoalOwnerAndShares updates goal ownership and sharing based on the selected team set.
// Returns ErrCannotShareWithClosedPeriod if any selected team has an in_progress or closed period.
func (s *Service) UpdateGoalOwnerAndShares(ctx context.Context, scope domain.TenantScope, goalID int64, selectedTeamIDs []int64, actorUserID int64) (ownerID, periodID int64, err error) {
	goal, err := s.goals.GetGoal(ctx, scope, goalID)
	if err != nil {
		return 0, 0, err
	}
	oldOwner := goal.TeamID
	shareList, err := s.shares.ListGoalShares(ctx, scope, goalID)
	if err != nil {
		return 0, 0, err
	}
	shareWeights := make(map[int64]int, len(shareList))
	for _, share := range shareList {
		shareWeights[share.TeamID] = share.Weight
	}
	selectedSet := make(map[int64]struct{}, len(selectedTeamIDs))
	for _, id := range selectedTeamIDs {
		selectedSet[id] = struct{}{}
	}
	ownerID = goal.TeamID
	if _, ok := selectedSet[ownerID]; !ok && len(selectedTeamIDs) > 0 {
		ownerID = selectedTeamIDs[0]
	}
	newShares := make([]shares.GoalShareInput, 0, len(selectedTeamIDs))
	for _, teamID := range selectedTeamIDs {
		status, err := s.statuses.GetTeamPeriodStatus(ctx, scope, teamID, goal.PeriodID)
		if err != nil {
			return 0, 0, err
		}
		if status == domain.TeamPeriodStatusInProgress || status == domain.TeamPeriodStatusClosed {
			return 0, 0, ErrCannotShareWithClosedPeriod
		}
		if teamID == ownerID {
			ownerWeight := goal.Weight
			if ownerID != goal.TeamID {
				if w, ok := shareWeights[ownerID]; ok {
					ownerWeight = w
				} else {
					ownerWeight = 0
				}
			}
			if err := s.goals.UpdateGoalOwner(ctx, scope, goalID, ownerID, ownerWeight); err != nil {
				return 0, 0, err
			}
			continue
		}
		weight := 0
		if w, ok := shareWeights[teamID]; ok {
			weight = w
		}
		newShares = append(newShares, shares.GoalShareInput{TeamID: teamID, Weight: weight})
	}
	if err := s.shares.ReplaceGoalShares(ctx, scope, goalID, newShares); err != nil {
		return 0, 0, err
	}
	// Only log an owner change when the owner actually changed (avoid X→X noise).
	if ownerID != oldOwner {
		gid, pID := goalID, goal.PeriodID
		s.recordActivity(ctx, scope, domain.ActivityEvent{
			ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionGoalOwnerChanged,
			TeamID: &ownerID, PeriodID: &pID, GoalID: &gid, EntityTitle: goal.Title,
			Payload: map[string]any{"before": map[string]any{"owner_team_id": oldOwner}, "after": map[string]any{"owner_team_id": ownerID}},
		})
	}
	return ownerID, goal.PeriodID, nil
}

// ── Activity journal ─────────────────────────────────────────────────────────

// diffFields returns only the entries whose before != after, as {field: {"before":x,"after":y}}.
func diffFields(pairs map[string][2]any) map[string]any {
	out := map[string]any{}
	for field, ba := range pairs {
		if ba[0] != ba[1] {
			out[field] = map[string]any{"before": ba[0], "after": ba[1]}
		}
	}
	return out
}

// recordActivity persists one event best-effort: a failure is logged, never returned,
// so the activity journal can never break the user's mutation.
func (s *Service) recordActivity(ctx context.Context, scope domain.TenantScope, ev domain.ActivityEvent) {
	if s.activity == nil {
		return
	}
	if _, err := s.activity.Record(ctx, scope, ev); err != nil && s.logger != nil {
		s.logger.Warn("activity: record failed", "action", string(ev.Action), "tenant", scope.TenantID, "err", err)
	}
}

// ListActivity returns a scoped page of journal events. allowedTeamIDs == nil = admin/unrestricted.
func (s *Service) ListActivity(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, f activity.ListFilter) ([]domain.ActivityEvent, *activity.Cursor, error) {
	return s.activity.List(ctx, scope, allowedTeamIDs, f)
}

// ActivityTreeCounts returns direct per-team event counts for the sidebar tree.
func (s *Service) ActivityTreeCounts(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, periodID *int64, since *time.Time) (map[int64]int, error) {
	return s.activity.TreeCounts(ctx, scope, allowedTeamIDs, periodID, since)
}

// ActivityCategoryCounts returns per-category event counts for the feed's tab counters,
// stable across the selected category (the filter excludes category).
func (s *Service) ActivityCategoryCounts(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, f activity.ListFilter) (map[string]int, error) {
	return s.activity.CategoryCounts(ctx, scope, allowedTeamIDs, f)
}

// PurgeActivity deletes journal rows for the caller's tenant. Authority (tenant-admin) is
// enforced by RequireTenantAdminMiddleware on the route; the system plane uses
// ProvisioningService.PurgeActivityForTenant instead.
func (s *Service) PurgeActivity(ctx context.Context, scope domain.TenantScope, olderThan *time.Time) (int64, error) {
	return s.activity.Purge(ctx, scope, olderThan)
}
