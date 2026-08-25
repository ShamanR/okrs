// Package goal is the goal entity service. It touches exactly one repository and
// never writes the activity journal — anything orchestrating more is a usecase.
package goal

import (
	"context"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/core/progress"
	"okrs/internal/store/goals"
)

// Service is the goal entity service.
type Service struct {
	repo Repo
}

func New(repo Repo) *Service { return &Service{repo: repo} }

type Repo interface {
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

func (s *Service) Get(ctx context.Context, scope domain.TenantScope, id int64) (domain.Goal, error) {
	goal, err := s.repo.GetGoal(ctx, scope, id)
	if err != nil {
		return domain.Goal{}, err
	}
	goal.Progress = progress.ForGoal(&goal)
	return goal, nil
}
func (s *Service) Move(ctx context.Context, scope domain.TenantScope, teamID, goalID int64, direction int) error {
	return s.repo.MoveGoal(ctx, scope, teamID, goalID, direction)
}
func (s *Service) ListByTeamPeriod(ctx context.Context, scope domain.TenantScope, teamID, periodID int64) ([]domain.Goal, error) {
	return s.repo.ListGoalsByTeamPeriod(ctx, scope, teamID, periodID)
}
func (s *Service) ListComments(ctx context.Context, scope domain.TenantScope, goalID int64) ([]domain.GoalComment, error) {
	return s.repo.ListGoalComments(ctx, scope, goalID)
}

func (s *Service) ProgressByIDs(ctx context.Context, scope domain.TenantScope, ids []int64) (map[int64]int, error) {
	if len(ids) == 0 {
		return map[int64]int{}, nil
	}
	linked, err := s.repo.ListGoalsByIDs(ctx, scope, ids)
	if err != nil {
		return nil, err
	}
	progressByID := make(map[int64]int, len(linked))
	for i := range linked {
		progressByID[linked[i].ID] = progress.ForGoal(&linked[i])
	}
	return progressByID, nil
}

// — Однострочные операции над сущностью, нужные сценариям слоя usecase. —

func (s *Service) AddComment(ctx context.Context, scope domain.TenantScope, goalID int64, text string, authorUserID int64) (int64, error) {
	return s.repo.AddGoalComment(ctx, scope, goalID, text, authorUserID)
}

func (s *Service) AddReply(ctx context.Context, scope domain.TenantScope, goalID, parentID int64, text string, authorUserID int64) (int64, error) {
	return s.repo.AddGoalReply(ctx, scope, goalID, parentID, text, authorUserID)
}

func (s *Service) Copy(ctx context.Context, scope domain.TenantScope, in goals.CopyGoalInput) (int64, error) {
	return s.repo.CopyGoal(ctx, scope, in)
}

func (s *Service) Create(ctx context.Context, scope domain.TenantScope, input goals.GoalInput) (int64, error) {
	return s.repo.CreateGoal(ctx, scope, input)
}

func (s *Service) Delete(ctx context.Context, scope domain.TenantScope, id int64) error {
	return s.repo.DeleteGoal(ctx, scope, id)
}

func (s *Service) DeleteComment(ctx context.Context, scope domain.TenantScope, goalID, commentID int64) error {
	return s.repo.DeleteGoalComment(ctx, scope, goalID, commentID)
}

func (s *Service) GetCommentMeta(ctx context.Context, scope domain.TenantScope, goalID, commentID int64) (int64, bool, error) {
	return s.repo.GetGoalCommentMeta(ctx, scope, goalID, commentID)
}

// Батчевая операция: один запрос на весь набор. Не превращать в цикл — это N+1.
func (s *Service) ListByTeamsPeriod(ctx context.Context, scope domain.TenantScope, periodID int64, teamIDs []int64) (map[int64][]domain.Goal, error) {
	return s.repo.ListGoalsByTeamsPeriod(ctx, scope, periodID, teamIDs)
}

// Батчевая операция: один запрос на весь набор. Не превращать в цикл — это N+1.
func (s *Service) ListCommentsByGoals(ctx context.Context, scope domain.TenantScope, goalIDs []int64) (map[int64][]domain.GoalComment, error) {
	return s.repo.ListGoalCommentsByGoals(ctx, scope, goalIDs)
}

// Батчевая операция: один запрос на весь набор. Не превращать в цикл — это N+1.
func (s *Service) ListForPeriods(ctx context.Context, scope domain.TenantScope, periodIDs []int64, allowedTeamIDs []int64, adminAll bool) ([]domain.Goal, error) {
	return s.repo.ListGoalsForPeriods(ctx, scope, periodIDs, allowedTeamIDs, adminAll)
}

// Батчевая операция: один запрос на весь набор. Не превращать в цикл — это N+1.
func (s *Service) ListOwnerTeamIDs(ctx context.Context, scope domain.TenantScope, goalIDs []int64) (map[int64]int64, error) {
	return s.repo.ListGoalOwnerTeamIDs(ctx, scope, goalIDs)
}

// Батчевая операция: один запрос на весь набор. Не превращать в цикл — это N+1.
func (s *Service) ListTeamLastUpdateInPeriod(ctx context.Context, scope domain.TenantScope, periodID int64, teamIDs []int64) (map[int64]time.Time, error) {
	return s.repo.ListTeamLastGoalUpdateInPeriod(ctx, scope, periodID, teamIDs)
}

func (s *Service) SetCommentResolved(ctx context.Context, scope domain.TenantScope, goalID, commentID int64, resolved bool, userID int64) (bool, error) {
	return s.repo.SetGoalCommentResolved(ctx, scope, goalID, commentID, resolved, userID)
}

func (s *Service) Update(ctx context.Context, scope domain.TenantScope, input goals.GoalUpdateInput) error {
	return s.repo.UpdateGoal(ctx, scope, input)
}

func (s *Service) UpdateFields(ctx context.Context, scope domain.TenantScope, input goals.GoalFieldsUpdateInput) error {
	return s.repo.UpdateGoalFields(ctx, scope, input)
}

func (s *Service) UpdateOwner(ctx context.Context, scope domain.TenantScope, goalID, teamID int64, weight int) error {
	return s.repo.UpdateGoalOwner(ctx, scope, goalID, teamID, weight)
}
