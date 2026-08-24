package service

import (
	"context"
	"log/slog"
	"time"

	"okrs/internal/core/domain"
	serviceactivity "okrs/internal/service/activity"
	"okrs/internal/service/goal"
	"okrs/internal/service/goallink"
	"okrs/internal/service/goalshare"
	"okrs/internal/service/healthcheckin"
	"okrs/internal/service/keyresult"
	"okrs/internal/service/onboarding"
	"okrs/internal/service/period"
	"okrs/internal/service/progresssnap"
	"okrs/internal/service/provisioning"
	"okrs/internal/service/settings"
	"okrs/internal/service/team"
	"okrs/internal/service/teamstatus"
	"okrs/internal/service/user"
	"okrs/internal/store"
	"okrs/internal/store/activity"
	"okrs/internal/store/goallinks"
	"okrs/internal/store/goals"
	"okrs/internal/store/grants"
	"okrs/internal/store/invitations"
	"okrs/internal/store/krs"
	"okrs/internal/store/memberships"
	"okrs/internal/store/periods"
	storesettings "okrs/internal/store/settings"
	"okrs/internal/store/shares"
	"okrs/internal/store/teams"
	"okrs/internal/store/tenants"
	"okrs/internal/store/tenantsettings"
	goaluc "okrs/internal/usecase/goal"
	kruc "okrs/internal/usecase/keyresult"
	"okrs/internal/usecase/okrboard"
	perioduc "okrs/internal/usecase/period"
)

// GrantsProvider gives the service access to the cached user_hierarchy_grants snapshot.
// *grants.GrantsCache satisfies this interface.
type GrantsProvider interface {
	AllGrants(ctx context.Context) (map[int64][]grants.HierarchyGrant, error)
}

// Per-entity repository interfaces used by the service layer.
// Each interface is satisfied by the corresponding store.*Repository type.

type TeamRepo = team.Repo

type GoalRepo = goal.Repo

type GoalShareRepo = goalshare.Repo

// GoalLinkRepo manages goal↔goal parent/child links.
type GoalLinkRepo = goallink.Repo

type PeriodRepo = period.Repo

type KRRepo = keyresult.Repo

type TeamStatusRepo = teamstatus.Repo

type UserRepo = user.Repo

// ActivityRepo records and reads the append-only activity journal.
type ActivityRepo = serviceactivity.Repo

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
	ProgressSnap progresssnap.Repo
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
	progressSnap progresssnap.Repo
	logger       *slog.Logger

	teamSvc       *team.Service
	userSvc       *user.Service
	teamstatusSvc *teamstatus.Service
	goallinkSvc   *goallink.Service
	goalshareSvc  *goalshare.Service
	goalSvc       *goal.Service
	periodSvc     *period.Service
	keyresultSvc  *keyresult.Service
	activitySvc   *serviceactivity.Service
	hcSvc         *healthcheckin.Service

	okrboardUC  *okrboard.UseCase
	goalUC      *goaluc.UseCase
	keyresultUC *kruc.UseCase
	periodUC    *perioduc.UseCase
}

// Доменные ошибки переехали в core/domain — единый дом для service, usecase и handler.
// Здесь остаются алиасы (те же самые переменные), чтобы существующие errors.Is в handlers
// продолжали работать без правок; удаляются на этапе E вместе с фасадом.
var (
	ErrTeamHasGoals                = domain.ErrTeamHasGoals
	ErrTeamNotVisibleInPeriod      = domain.ErrTeamNotVisibleInPeriod
	ErrPeriodClosed                = domain.ErrPeriodClosed
	ErrPeriodNotClosed             = domain.ErrPeriodNotClosed
	ErrCannotShareWithClosedPeriod = domain.ErrCannotShareWithClosedPeriod
	ErrShareTargetNotInTenant      = domain.ErrShareTargetNotInTenant
	ErrTransferTargetSameAsSource  = domain.ErrTransferTargetSameAsSource
	ErrTransferTargetNotFound      = domain.ErrTransferTargetNotFound
	ErrForbidden                   = domain.ErrForbidden
	ErrGoalNotOnTeamBoard          = domain.ErrGoalNotOnTeamBoard
	ErrGoalLinkSelf                = domain.ErrGoalLinkSelf
	ErrGoalLinkNotAccessible       = domain.ErrGoalLinkNotAccessible
	ErrGoalLinkCycle               = domain.ErrGoalLinkCycle
)

// New constructs a Service from a Deps bundle.
func New(deps Deps) *Service {
	// Сервисы сущностей строятся до литерала: goallink получает уже созданный
	// goalSvc через порт GoalProgressReader, а не собственную вторую копию.
	// Сервисы сущностей строятся до литерала: их переиспользуют и поля Service,
	// и usecase, которым они нужны как collaborators. Одна инстанция на сервис.
	teamSvc := team.New(deps.Teams)
	goalSvc := goal.New(deps.Goals)
	shareSvc := goalshare.New(deps.Shares)
	linkSvc := goallink.New(deps.GoalLinks, goalSvc)
	statusSvc := teamstatus.New(deps.Statuses)
	periodSvc := period.New(deps.Periods)
	krSvc := keyresult.New(deps.KRs)
	userSvc := user.New(deps.Users)
	activitySvc := serviceactivity.New(deps.Activity, deps.Logger)

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

		teamSvc:       teamSvc,
		userSvc:       userSvc,
		goalSvc:       goalSvc,
		goalshareSvc:  shareSvc,
		goallinkSvc:   linkSvc,
		teamstatusSvc: statusSvc,
		periodSvc:     periodSvc,
		keyresultSvc:  krSvc,
		activitySvc:   activitySvc,
		hcSvc:         healthcheckin.New(deps.HCCache),

		okrboardUC:  okrboard.New(okrboard.Deps{Teams: teamSvc, Goals: goalSvc, Shares: shareSvc, Statuses: statusSvc, Periods: periodSvc, Links: linkSvc}),
		goalUC:      goaluc.New(goaluc.Deps{Goals: goalSvc, Shares: shareSvc, Links: linkSvc, Statuses: statusSvc, Periods: periodSvc, Teams: teamSvc, KRs: krSvc, Activity: activitySvc}),
		keyresultUC: kruc.New(kruc.Deps{KRs: krSvc, Goals: goalSvc, Activity: activitySvc}),
		periodUC:    perioduc.New(perioduc.Deps{Periods: periodSvc, Teams: teamSvc, Goals: goalSvc, Statuses: statusSvc, Snaps: progresssnap.New(deps.ProgressSnap), Activity: activitySvc, HCCache: deps.HCCache, Logger: deps.Logger}),
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

// TeamNode is an alias so handlers keep compiling against service.TeamNode until stage E.
type TeamNode = team.Node

type TeamSummary = okrboard.TeamSummary

type TeamGoalSummary = okrboard.TeamGoalSummary

type TeamShareInfo = okrboard.TeamShareInfo

type TeamChildSummary = okrboard.TeamChildSummary

type TeamOverview = okrboard.TeamOverview

type TeamOKR = okrboard.TeamOKR

type GoalDetails = okrboard.GoalDetails

// ListPeriodViews returns periods enriched with parent/depth/status via domain.BuildPeriodViews.
// When includeArchived is false, archived periods are filtered out before building the views, so
// a visible period's ParentID never points at a period the caller can't see.

// teamSummaryBatch holds pre-loaded data for building TeamSummary without per-team DB queries.

// buildShareInfosFromBatch builds TeamShareInfo slice from pre-loaded shares and teams.
// This avoids per-goal DB calls in loops.

// UpdateKRHealthStatus sets the manual health status of a KR. Access is checked by the caller
// (same as progress update). Health status is informational and does not affect progress math.

// autoCompleteHealth sets health=done exactly once, on the progress transition <100 -> =100,
// and only if the KR is not already done. Never reverts on later drops. kr is the pre-update state.

type ProjectStageUpdate = kruc.ProjectStageUpdate

type ShareTarget = goaluc.ShareTarget

// KeyResultMetaInput is an alias so handlers keep compiling until stage E.
type KeyResultMetaInput = keyresult.MetaInput

// — Team passthroughs —

// — Period passthroughs —

// ArchivePeriod archives a period, but only once it is closed — archiving an active or future
// period would hide it from the tree while it's still in use.

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

// — Key result passthroughs —

// — Business logic —

type CopyGoalMode = goaluc.CopyGoalMode

const (
	CopyGoalModeCopy = goaluc.CopyGoalModeCopy
	CopyGoalModeMove = goaluc.CopyGoalModeMove
)

type CopyGoalParams = goaluc.CopyGoalParams

// ── Activity journal ─────────────────────────────────────────────────────────

// — Делегирование в service/team. Фасад сохраняет старые имена, чтобы handlers
// не менялись до этапа E; сами реализации живут в пакете сущности.

func (s *Service) ListTeams(ctx context.Context, scope domain.TenantScope) ([]domain.Team, error) {
	return s.teamSvc.List(ctx, scope)
}

func (s *Service) ListDeletedTeams(ctx context.Context, scope domain.TenantScope) ([]domain.Team, error) {
	return s.teamSvc.ListDeleted(ctx, scope)
}

func (s *Service) ListAllTeams(ctx context.Context, scope domain.TenantScope) ([]domain.Team, error) {
	return s.teamSvc.ListAll(ctx, scope)
}

func (s *Service) GetTeam(ctx context.Context, scope domain.TenantScope, teamID int64) (domain.Team, error) {
	return s.teamSvc.Get(ctx, scope, teamID)
}

func (s *Service) CreateTeam(ctx context.Context, scope domain.TenantScope, input teams.TeamInput) (int64, error) {
	return s.teamSvc.Create(ctx, scope, input)
}

func (s *Service) UpdateTeam(ctx context.Context, scope domain.TenantScope, input teams.TeamInput, id int64) error {
	return s.teamSvc.Update(ctx, scope, input, id)
}

func (s *Service) DeleteTeam(ctx context.Context, scope domain.TenantScope, teamID int64) error {
	return s.teamSvc.Delete(ctx, scope, teamID)
}

func (s *Service) RestoreTeam(ctx context.Context, scope domain.TenantScope, teamID int64) error {
	return s.teamSvc.Restore(ctx, scope, teamID)
}

func (s *Service) HardDeleteTeam(ctx context.Context, scope domain.TenantScope, teamID int64) error {
	return s.teamSvc.HardDelete(ctx, scope, teamID)
}

func (s *Service) GetHierarchy(ctx context.Context, scope domain.TenantScope, periodID *int64) ([]TeamNode, error) {
	return s.teamSvc.Hierarchy(ctx, scope, periodID)
}

func (s *Service) isTeamVisibleInPeriod(ctx context.Context, scope domain.TenantScope, tm domain.Team, periodID int64) (bool, error) {
	return s.teamSvc.VisibleInPeriod(ctx, scope, tm, periodID)
}

// — Делегирование в service/period (фасад, удаляется на этапе E). —

func (s *Service) ListPeriods(ctx context.Context, scope domain.TenantScope) ([]domain.Period, error) {
	return s.periodSvc.List(ctx, scope)
}

func (s *Service) ListPeriodViews(ctx context.Context, scope domain.TenantScope, includeArchived bool) ([]domain.PeriodView, error) {
	return s.periodSvc.ListViews(ctx, scope, includeArchived)
}

func (s *Service) GetPeriod(ctx context.Context, scope domain.TenantScope, periodID int64) (domain.Period, error) {
	return s.periodSvc.Get(ctx, scope, periodID)
}

func (s *Service) FindPeriodForDate(ctx context.Context, scope domain.TenantScope, date time.Time) (domain.Period, error) {
	return s.periodSvc.FindForDate(ctx, scope, date)
}

func (s *Service) CreatePeriod(ctx context.Context, scope domain.TenantScope, input periods.PeriodInput) (int64, error) {
	return s.periodSvc.Create(ctx, scope, input)
}

func (s *Service) UpdatePeriod(ctx context.Context, scope domain.TenantScope, periodID int64, input periods.PeriodInput) error {
	return s.periodSvc.Update(ctx, scope, periodID, input)
}

func (s *Service) DeletePeriod(ctx context.Context, scope domain.TenantScope, periodID int64) error {
	return s.periodSvc.Delete(ctx, scope, periodID)
}

func (s *Service) ArchivePeriod(ctx context.Context, scope domain.TenantScope, periodID int64) error {
	return s.periodSvc.Archive(ctx, scope, periodID)
}

func (s *Service) UnarchivePeriod(ctx context.Context, scope domain.TenantScope, periodID int64) error {
	return s.periodSvc.Unarchive(ctx, scope, periodID)
}

// — Делегирование в service/keyresult (фасад, удаляется на этапе E). —

func (s *Service) GetKeyResult(ctx context.Context, scope domain.TenantScope, id int64) (domain.KeyResult, error) {
	return s.keyresultSvc.Get(ctx, scope, id)
}

func (s *Service) MoveKeyResult(ctx context.Context, scope domain.TenantScope, krID int64, direction int) error {
	return s.keyresultSvc.Move(ctx, scope, krID, direction)
}

func (s *Service) UpdateKeyResultDescription(ctx context.Context, scope domain.TenantScope, krID int64, description string) error {
	return s.keyresultSvc.UpdateDescription(ctx, scope, krID, description)
}

func (s *Service) UpdateKRHealthStatus(ctx context.Context, scope domain.TenantScope, krID int64, status domain.KRHealthStatus) error {
	return s.keyresultSvc.UpdateHealthStatus(ctx, scope, krID, status)
}

func (s *Service) FindGoalIDByKR(ctx context.Context, scope domain.TenantScope, krID int64) (int64, error) {
	return s.keyresultSvc.FindGoalIDByKR(ctx, scope, krID)
}

func (s *Service) FindGoalIDByStage(ctx context.Context, scope domain.TenantScope, stageID int64) (int64, error) {
	return s.keyresultSvc.FindGoalIDByStage(ctx, scope, stageID)
}

// — Делегирование в service/goal (фасад, удаляется на этапе E). —

func (s *Service) GetGoal(ctx context.Context, scope domain.TenantScope, id int64) (domain.Goal, error) {
	return s.goalSvc.Get(ctx, scope, id)
}

func (s *Service) MoveGoal(ctx context.Context, scope domain.TenantScope, teamID, goalID int64, direction int) error {
	return s.goalSvc.Move(ctx, scope, teamID, goalID, direction)
}

func (s *Service) ListGoalsByTeamPeriod(ctx context.Context, scope domain.TenantScope, teamID, periodID int64) ([]domain.Goal, error) {
	return s.goalSvc.ListByTeamPeriod(ctx, scope, teamID, periodID)
}

func (s *Service) ListGoalComments(ctx context.Context, scope domain.TenantScope, goalID int64) ([]domain.GoalComment, error) {
	return s.goalSvc.ListComments(ctx, scope, goalID)
}

// — Делегирование в service/goalshare (фасад, удаляется на этапе E). —

func (s *Service) GetGoalShare(ctx context.Context, scope domain.TenantScope, goalID, teamID int64) (shares.GoalShare, error) {
	return s.goalshareSvc.Get(ctx, scope, goalID, teamID)
}

func (s *Service) ListGoalShares(ctx context.Context, scope domain.TenantScope, goalID int64) ([]shares.GoalShare, error) {
	return s.goalshareSvc.List(ctx, scope, goalID)
}

func (s *Service) UpdateGoalWeight(ctx context.Context, scope domain.TenantScope, goalID, teamID int64, weight int) error {
	return s.goalshareSvc.UpdateWeight(ctx, scope, goalID, teamID, weight)
}

// — Делегирование в service/teamstatus (фасад, удаляется на этапе E). —

func (s *Service) GetTeamPeriodStatus(ctx context.Context, scope domain.TenantScope, teamID, periodID int64) (domain.TeamPeriodStatus, error) {
	return s.teamstatusSvc.Get(ctx, scope, teamID, periodID)
}

// — Делегирование в service/goallink (фасад, удаляется на этапе E). —

func (s *Service) ListLinksForGoals(ctx context.Context, scope domain.TenantScope, goalIDs, allowedTeamIDs []int64, adminAll bool) (map[int64][]domain.GoalRef, map[int64][]domain.GoalRef, error) {
	return s.goallinkSvc.ListForGoals(ctx, scope, goalIDs, allowedTeamIDs, adminAll)
}

func (s *Service) ListLinkableGoals(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, adminAll bool, periodID *int64, excludeGoalID int64, q string) ([]goallinks.LinkableGoal, error) {
	return s.goallinkSvc.ListLinkable(ctx, scope, allowedTeamIDs, adminAll, periodID, excludeGoalID, q)
}

// — Делегирование в service/user (фасад, удаляется на этапе E). —

func (s *Service) GetUsersByDisplayNames(ctx context.Context, names []string) ([]*domain.User, error) {
	return s.userSvc.GetByDisplayNames(ctx, names)
}

func (s *Service) GetUsersByUDIDs(ctx context.Context, udids []string) ([]*domain.User, error) {
	return s.userSvc.GetByUDIDs(ctx, udids)
}

func (s *Service) ListUserLeadTeams(ctx context.Context) (map[string]string, error) {
	return s.userSvc.ListLeadTeams(ctx)
}

func (s *Service) ValidateUserUDIDsExist(ctx context.Context, udids []string) ([]string, error) {
	return s.userSvc.ValidateUDIDsExist(ctx, udids)
}

// — Делегирование в service/activity (фасад, удаляется на этапе E). —

func (s *Service) ListActivity(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, f activity.ListFilter) ([]domain.ActivityEvent, *activity.Cursor, error) {
	return s.activitySvc.List(ctx, scope, allowedTeamIDs, f)
}

func (s *Service) ActivityTreeCounts(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, periodID *int64, since *time.Time) (map[int64]int, error) {
	return s.activitySvc.TreeCounts(ctx, scope, allowedTeamIDs, periodID, since)
}

func (s *Service) ActivityCategoryCounts(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, f activity.ListFilter) (map[string]int, error) {
	return s.activitySvc.CategoryCounts(ctx, scope, allowedTeamIDs, f)
}

func (s *Service) PurgeActivity(ctx context.Context, scope domain.TenantScope, olderThan *time.Time) (int64, error) {
	return s.activitySvc.Purge(ctx, scope, olderThan)
}

// — Алиасы на переехавшие сервисы. Handlers продолжают писать service.X до этапа E,
// когда перейдут на пакеты напрямую и эти алиасы вместе с фасадом исчезнут. —

type (
	SettingsService     = settings.Service
	ProvisioningService = provisioning.Service
	OnboardingService   = onboarding.Service
	HealthCheckInCache  = healthcheckin.Cache
	HealthCheckInConfig = healthcheckin.Config
	HealthCheckInResult = healthcheckin.Result
	HCActive            = healthcheckin.Active
	PeriodData          = healthcheckin.PeriodData
	SettingsReader      = healthcheckin.SettingsReader
)

const (
	EntitlementPrefix               = settings.EntitlementPrefix
	ProgressSnapshotIntervalDaysKey = healthcheckin.ProgressSnapshotIntervalDaysKey
)

var (
	ErrLastAdmin       = provisioning.ErrLastAdmin
	ErrLastSystemAdmin = provisioning.ErrLastSystemAdmin
	ErrSelfLockout     = provisioning.ErrSelfLockout
	ErrAlreadyMember   = onboarding.ErrAlreadyMember
	ErrTenantNotFound  = onboarding.ErrTenantNotFound
)

// Обёртки конструкторов: alias для функции в Go невозможен, а server.go зовёт
// service.NewXxx до этапа E.

func NewSettingsService(
	tsCache *tenantsettings.TenantSettingsCache,
	tsRepo *tenantsettings.TenantSettingsRepository,
	sysCache *storesettings.SystemSettingsCache,
	sysRepo *storesettings.SettingsRepository,
) *settings.Service {
	return settings.New(tsCache, tsRepo, sysCache, sysRepo)
}

func LoadHealthCheckInConfig(ctx context.Context, scope domain.TenantScope, r healthcheckin.SettingsReader) (healthcheckin.Config, error) {
	return healthcheckin.LoadConfig(ctx, scope, r)
}

func GenerateInviteToken() (string, string, error) { return onboarding.GenerateInviteToken() }

func NewHealthCheckInCache(loader func(ctx context.Context, scope domain.TenantScope, periodID int64) (*healthcheckin.PeriodData, error), ttl time.Duration, logger *slog.Logger) *healthcheckin.Cache {
	return healthcheckin.NewCache(loader, ttl, logger)
}

func NewOnboardingService(inv *invitations.InvitationRepository, mem *memberships.MembershipRepository, memCache *memberships.MembershipCache, tn *tenants.TenantRepository, st *settings.Service, granter onboarding.NewUserGranter) *onboarding.Service {
	return onboarding.New(inv, mem, memCache, tn, st, granter)
}

func LoadProgressSnapshotIntervalDays(ctx context.Context, scope domain.TenantScope, sr healthcheckin.SettingsReader) int {
	return healthcheckin.LoadProgressSnapshotIntervalDays(ctx, scope, sr)
}

// GetHealthCheckIn делегирует в service/healthcheckin, сохраняя имя для handlers.
func (s *Service) GetHealthCheckIn(ctx context.Context, scope domain.TenantScope, userUDID string, isAdmin bool, periodID int64, cfg healthcheckin.Config) (*healthcheckin.Result, error) {
	return s.hcSvc.Get(ctx, scope, userUDID, isAdmin, periodID, cfg)
}

func NewProvisioningService(
	tnRepo *tenants.TenantRepository, tenantCache *tenants.TenantCache,
	memRepo *memberships.MembershipRepository, memberCache *memberships.MembershipCache,
	st *settings.Service, grants provisioning.GrantRemover, defaultAccess provisioning.DefaultAccessApplier,
	users provisioning.SystemAdminStore,
) *provisioning.Service {
	return provisioning.New(tnRepo, tenantCache, memRepo, memberCache, st, grants, defaultAccess, users)
}

// — Делегирование в usecase/okrboard (фасад, удаляется на этапе E). —

func (s *Service) GetTeamsWithPeriodSummary(ctx context.Context, scope domain.TenantScope, periodID int64, orgID *int64) ([]TeamSummary, error) {
	return s.okrboardUC.TeamsWithPeriodSummary(ctx, scope, periodID, orgID)
}

func (s *Service) GetTeamOKR(ctx context.Context, scope domain.TenantScope, teamID, periodID int64, period domain.Period) (TeamOKR, error) {
	return s.okrboardUC.TeamOKRFor(ctx, scope, teamID, periodID, period)
}

func (s *Service) GetDirectChildrenSummary(ctx context.Context, scope domain.TenantScope, teamID, periodID int64) ([]TeamChildSummary, error) {
	return s.okrboardUC.DirectChildrenSummary(ctx, scope, teamID, periodID)
}

func (s *Service) GetTeamOverview(ctx context.Context, scope domain.TenantScope, teamID, periodID int64) (TeamOverview, error) {
	return s.okrboardUC.TeamOverviewFor(ctx, scope, teamID, periodID)
}

// — Делегирование в usecase/goal (фасад, удаляется на этапе E). —

func (s *Service) CreateGoal(ctx context.Context, scope domain.TenantScope, input goals.GoalInput, actorUserID int64) (int64, error) {
	return s.goalUC.Create(ctx, scope, input, actorUserID)
}

func (s *Service) UpdateGoal(ctx context.Context, scope domain.TenantScope, input goals.GoalUpdateInput, actorUserID int64) error {
	return s.goalUC.Update(ctx, scope, input, actorUserID)
}

func (s *Service) UpdateGoalFields(ctx context.Context, scope domain.TenantScope, input goals.GoalFieldsUpdateInput, actorUserID int64) error {
	return s.goalUC.UpdateFields(ctx, scope, input, actorUserID)
}

func (s *Service) DeleteGoal(ctx context.Context, scope domain.TenantScope, goalID, requestingTeamID int64, actorUserID int64) (int64, int64, error) {
	return s.goalUC.Delete(ctx, scope, goalID, requestingTeamID, actorUserID)
}

func (s *Service) CopyGoal(ctx context.Context, scope domain.TenantScope, p CopyGoalParams, actorUserID int64) (int64, error) {
	return s.goalUC.Copy(ctx, scope, p, actorUserID)
}

func (s *Service) ShareGoal(ctx context.Context, scope domain.TenantScope, goalID int64, targets []ShareTarget, actorUserID int64) error {
	return s.goalUC.Share(ctx, scope, goalID, targets, actorUserID)
}

func (s *Service) UpdateGoalOwnerAndShares(ctx context.Context, scope domain.TenantScope, goalID int64, selectedTeamIDs []int64, actorUserID int64) (int64, int64, error) {
	return s.goalUC.UpdateOwnerAndShares(ctx, scope, goalID, selectedTeamIDs, actorUserID)
}

func (s *Service) DeleteGoalShare(ctx context.Context, scope domain.TenantScope, goalID, teamID int64, actorUserID int64) error {
	return s.goalUC.DeleteShare(ctx, scope, goalID, teamID, actorUserID)
}

func (s *Service) AddGoalComment(ctx context.Context, scope domain.TenantScope, goalID int64, text string, authorUserID int64) error {
	return s.goalUC.AddComment(ctx, scope, goalID, text, authorUserID)
}

func (s *Service) AddGoalReply(ctx context.Context, scope domain.TenantScope, goalID, parentID int64, text string, authorUserID int64) error {
	return s.goalUC.AddReply(ctx, scope, goalID, parentID, text, authorUserID)
}

func (s *Service) SetGoalCommentResolved(ctx context.Context, scope domain.TenantScope, goalID, commentID int64, resolved bool, userID int64) error {
	return s.goalUC.SetCommentResolved(ctx, scope, goalID, commentID, resolved, userID)
}

func (s *Service) DeleteGoalComment(ctx context.Context, scope domain.TenantScope, goalID, commentID, requestingUserID int64, isAdmin bool) (bool, error) {
	return s.goalUC.DeleteComment(ctx, scope, goalID, commentID, requestingUserID, isAdmin)
}

func (s *Service) SetGoalParents(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, adminAll bool, childID int64, parentIDs []int64, actorUserID int64) error {
	return s.goalUC.SetParents(ctx, scope, allowedTeamIDs, adminAll, childID, parentIDs, actorUserID)
}

func (s *Service) AttachGoalLinks(ctx context.Context, scope domain.TenantScope, details []GoalDetails, allowedTeamIDs []int64, adminAll bool) error {
	return s.okrboardUC.AttachLinks(ctx, scope, details, allowedTeamIDs, adminAll)
}

// — Делегирование в usecase/keyresult (фасад, удаляется на этапе E). —

func (s *Service) UpdateKRProgressNumerical(ctx context.Context, scope domain.TenantScope, krID int64, current float64, actorUserID int64) error {
	return s.keyresultUC.UpdateProgressNumerical(ctx, scope, krID, current, actorUserID)
}

func (s *Service) UpdateKRProgressBoolean(ctx context.Context, scope domain.TenantScope, krID int64, done bool, actorUserID int64) error {
	return s.keyresultUC.UpdateProgressBoolean(ctx, scope, krID, done, actorUserID)
}

func (s *Service) UpdateKRProgressProject(ctx context.Context, scope domain.TenantScope, krID int64, updates []ProjectStageUpdate, actorUserID int64) error {
	return s.keyresultUC.UpdateProgressProject(ctx, scope, krID, updates, actorUserID)
}

func (s *Service) CreateKeyResultWithMeta(ctx context.Context, scope domain.TenantScope, input krs.KeyResultInput, meta KeyResultMetaInput, actorUserID int64) (int64, error) {
	return s.keyresultUC.CreateWithMeta(ctx, scope, input, meta, actorUserID)
}

func (s *Service) UpdateKeyResultWithMeta(ctx context.Context, scope domain.TenantScope, input krs.KeyResultUpdateInput, meta KeyResultMetaInput, actorUserID int64) error {
	return s.keyresultUC.UpdateWithMeta(ctx, scope, input, meta, actorUserID)
}

func (s *Service) DeleteKeyResult(ctx context.Context, scope domain.TenantScope, id int64, actorUserID int64) error {
	return s.keyresultUC.Delete(ctx, scope, id, actorUserID)
}

func (s *Service) UpsertKeyResultNote(ctx context.Context, scope domain.TenantScope, krID int64, text string, authorUserID int64) error {
	return s.keyresultUC.UpsertNote(ctx, scope, krID, text, authorUserID)
}

// — Алиасы read-model типов usecase/period (удаляются на этапе E). —

type (
	PeriodTeamSummary     = perioduc.PeriodTeamSummary
	PeriodOverviewSummary = perioduc.PeriodOverviewSummary
	BalanceBucket         = perioduc.BalanceBucket
	PeriodBalances        = perioduc.PeriodBalances
	PeriodGoalItem        = perioduc.PeriodGoalItem
	PeriodKRItem          = perioduc.PeriodKRItem
	PeriodOverview        = perioduc.PeriodOverview
	PeriodStatsItem       = perioduc.PeriodStatsItem
	BulkStatusResult      = perioduc.BulkStatusResult
	SeriesPoint           = perioduc.SeriesPoint
	ProgressSeries        = perioduc.ProgressSeries
)

// — Делегирование в usecase/period (фасад, удаляется на этапе E). —

func (s *Service) PeriodOverview(ctx context.Context, scope domain.TenantScope, periodID int64, weightTolerance int) (PeriodOverview, error) {
	return s.periodUC.PeriodOverview(ctx, scope, periodID, weightTolerance)
}

func (s *Service) PeriodOverviewScoped(ctx context.Context, scope domain.TenantScope, periodID int64, weightTolerance int, teamFilter map[int64]bool) (PeriodOverview, error) {
	return s.periodUC.PeriodOverviewScoped(ctx, scope, periodID, weightTolerance, teamFilter)
}

func (s *Service) PeriodStats(ctx context.Context, scope domain.TenantScope, weightTolerance int) ([]PeriodStatsItem, error) {
	return s.periodUC.PeriodStats(ctx, scope, weightTolerance)
}

func (s *Service) BulkSetTeamPeriodStatus(ctx context.Context, scope domain.TenantScope, periodID int64, target domain.TeamPeriodStatus, actorUserID int64, teamFilter map[int64]bool) (BulkStatusResult, error) {
	return s.periodUC.BulkSetTeamPeriodStatus(ctx, scope, periodID, target, actorUserID, teamFilter)
}

func (s *Service) SnapshotActivePeriods(ctx context.Context, day time.Time, actives []HCActive) error {
	return s.periodUC.SnapshotActivePeriods(ctx, day, actives)
}

func (s *Service) UpdateTeamPeriodStatus(ctx context.Context, scope domain.TenantScope, teamID, periodID int64, status domain.TeamPeriodStatus, actorUserID int64) error {
	return s.periodUC.UpdateTeamStatus(ctx, scope, teamID, periodID, status, actorUserID)
}
