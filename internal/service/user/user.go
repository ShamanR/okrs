// Package user is the user entity service. It touches exactly one repository and
// never writes the activity journal — anything orchestrating more is a usecase.
package user

import (
	"context"

	"okrs/internal/core/domain"
	"okrs/internal/store/users"
)

// Service is the user entity service.
type Service struct {
	repo Repo
}

func New(repo Repo) *Service { return &Service{repo: repo} }

type Repo interface {
	GetUsersByDisplayNames(ctx context.Context, names []string) ([]*domain.User, error)
	SearchUsersUnrestricted(ctx context.Context, q string, limit int) ([]*domain.User, error)
	SearchUsersInSet(ctx context.Context, userIDs []int64, leadUDIDs []string, q string, limit int) ([]*domain.User, error)
	GetUsersByUDIDs(ctx context.Context, udids []string) ([]*domain.User, error)
	ListUserLeadTeams(ctx context.Context) (map[string]string, error)
	ValidateUDIDsExist(ctx context.Context, udids []string) ([]string, error)
	ContactsByIDs(ctx context.Context, ids []int64) (map[int64]users.Contact, error)
}

func (s *Service) GetByDisplayNames(ctx context.Context, names []string) ([]*domain.User, error) {
	return s.repo.GetUsersByDisplayNames(ctx, names)
}
func (s *Service) GetByUDIDs(ctx context.Context, udids []string) ([]*domain.User, error) {
	return s.repo.GetUsersByUDIDs(ctx, udids)
}

// ContactsByIDs resolves names and addresses for a whole batch at once — the
// delivery path names actors and addresses recipients from the same call.
//
// Батчевая операция: не превращать в цикл — это N+1.
func (s *Service) ContactsByIDs(ctx context.Context, ids []int64) (map[int64]users.Contact, error) {
	return s.repo.ContactsByIDs(ctx, ids)
}

func (s *Service) ListLeadTeams(ctx context.Context) (map[string]string, error) {
	return s.repo.ListUserLeadTeams(ctx)
}
func (s *Service) ValidateUDIDsExist(ctx context.Context, udids []string) ([]string, error) {
	return s.repo.ValidateUDIDsExist(ctx, udids)
}

// — Однострочные операции над сущностью, нужные сценариям слоя usecase. —

func (s *Service) SearchInSet(ctx context.Context, userIDs []int64, leadUDIDs []string, q string, limit int) ([]*domain.User, error) {
	return s.repo.SearchUsersInSet(ctx, userIDs, leadUDIDs, q, limit)
}

func (s *Service) SearchUnrestricted(ctx context.Context, q string, limit int) ([]*domain.User, error) {
	return s.repo.SearchUsersUnrestricted(ctx, q, limit)
}
