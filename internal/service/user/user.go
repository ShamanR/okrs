// Package user is the user entity service. It touches exactly one repository and
// never writes the activity journal — anything orchestrating more is a usecase.
package user

import (
	"context"

	"okrs/internal/core/domain"
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
}

func (s *Service) GetByDisplayNames(ctx context.Context, names []string) ([]*domain.User, error) {
	return s.repo.GetUsersByDisplayNames(ctx, names)
}
func (s *Service) GetByUDIDs(ctx context.Context, udids []string) ([]*domain.User, error) {
	return s.repo.GetUsersByUDIDs(ctx, udids)
}
func (s *Service) ListLeadTeams(ctx context.Context) (map[string]string, error) {
	return s.repo.ListUserLeadTeams(ctx)
}
func (s *Service) ValidateUDIDsExist(ctx context.Context, udids []string) ([]string, error) {
	return s.repo.ValidateUDIDsExist(ctx, udids)
}
