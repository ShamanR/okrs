package store

import (
	"context"
	"encoding/json"
	"time"

	"okrs/internal/domain"
	"okrs/internal/store/goals"
	"okrs/internal/store/grants"
	"okrs/internal/store/krs"
	"okrs/internal/store/memberships"
	"okrs/internal/store/periods"
	"okrs/internal/store/sessions"
	"okrs/internal/store/settings"
	"okrs/internal/store/shares"
	"okrs/internal/store/statuses"
	"okrs/internal/store/teams"
	"okrs/internal/store/tenants"
	"okrs/internal/store/users"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store is the composite repository factory.
// All per-entity repositories are exposed as public fields.
// Store also implements the auth.authStorage interface via forwarding methods
// so it can be passed to auth.Manager without changes to the auth layer.
type Store struct {
	DB       *pgxpool.Pool
	Teams    *teams.TeamRepository
	Goals    *goals.GoalRepository
	Periods  *periods.PeriodRepository
	KRs      *krs.KRRepository
	Shares   *shares.GoalShareRepository
	Statuses *statuses.TeamStatusRepository
	Users    *users.UserRepository
	Sessions *sessions.SessionRepository
	Grants   *grants.GrantRepository
	Settings *settings.SettingsRepository

	Tenants     *tenants.TenantRepository
	Memberships *memberships.MembershipRepository
}

// New constructs a Store and wires all repositories.
func New(db *pgxpool.Pool) *Store {
	krsRepo := krs.NewKRRepository(db)
	return &Store{
		DB:       db,
		Teams:    teams.NewTeamRepository(db),
		Goals:    goals.NewGoalRepository(db, krsRepo),
		Periods:  periods.NewPeriodRepository(db),
		KRs:      krsRepo,
		Shares:   shares.NewGoalShareRepository(db),
		Statuses: statuses.NewTeamStatusRepository(db),
		Users:    users.NewUserRepository(db),
		Sessions: sessions.NewSessionRepository(db),
		Grants:   grants.NewGrantRepository(db),
		Settings: settings.NewSettingsRepository(db),

		Tenants:     tenants.NewTenantRepository(db),
		Memberships: memberships.NewMembershipRepository(db),
	}
}

// NewGrantsCache is a convenience alias for grants.NewGrantsCache for backward compatibility.
func NewGrantsCache(r *grants.GrantRepository) *grants.GrantsCache {
	return grants.NewGrantsCache(r)
}

// ── auth.authStorage forwarding methods ──────────────────────────────────────
// These allow *Store to satisfy the auth.authStorage interface so that
// auth.Manager can keep its current constructor signature.

func (s *Store) UpsertUser(ctx context.Context, in users.UpsertUserInput) (*domain.User, error) {
	return s.Users.UpsertUser(ctx, in)
}

func (s *Store) GetUser(ctx context.Context, id int64) (*domain.User, error) {
	return s.Users.GetUser(ctx, id)
}

func (s *Store) CreateSession(ctx context.Context, sessionID string, userID int64, provider string, ttl time.Duration, userAgent, ip string) (*domain.AuthSession, error) {
	return s.Sessions.CreateSession(ctx, sessionID, userID, provider, ttl, userAgent, ip)
}

func (s *Store) GetSession(ctx context.Context, sessionID string) (*domain.AuthSession, error) {
	return s.Sessions.GetSession(ctx, sessionID)
}

func (s *Store) TouchSession(ctx context.Context, sessionID string) error {
	return s.Sessions.TouchSession(ctx, sessionID)
}

func (s *Store) DeleteSession(ctx context.Context, sessionID string) error {
	return s.Sessions.DeleteSession(ctx, sessionID)
}

func (s *Store) GetSetting(ctx context.Context, key string) (json.RawMessage, error) {
	return s.Settings.GetSetting(ctx, key)
}

// SeedDemo inserts demo data (used only by the --seed flag in main).
func (s *Store) SeedDemo(ctx context.Context, periodID int64) error {
	return seedDemo(ctx, s.Goals, s.KRs, periodID)
}
