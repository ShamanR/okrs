package sessions

import (
	"context"
	"time"

	"okrs/internal/domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SessionRepository handles auth_sessions persistence.
type SessionRepository struct {
	db *pgxpool.Pool
}

func NewSessionRepository(db *pgxpool.Pool) *SessionRepository {
	return &SessionRepository{db: db}
}

func (r *SessionRepository) CreateSession(ctx context.Context, sessionID string, userID int64, provider string, ttl time.Duration, userAgent, ip string) (*domain.AuthSession, error) {
	now := time.Now()
	expires := now.Add(ttl)
	_, err := r.db.Exec(ctx, `
		INSERT INTO auth_sessions (id, user_id, provider, created_at, expires_at, last_seen_at, user_agent, ip)
		VALUES ($1, $2, $3, $4, $5, $4, $6, $7)`,
		sessionID, userID, provider, now, expires, nullableString(userAgent), nullableString(ip),
	)
	if err != nil {
		return nil, err
	}
	return &domain.AuthSession{
		ID: sessionID, UserID: userID, Provider: provider,
		CreatedAt: now, ExpiresAt: expires, LastSeenAt: now,
		UserAgent: userAgent, IP: ip,
	}, nil
}

func (r *SessionRepository) GetSession(ctx context.Context, sessionID string) (*domain.AuthSession, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, user_id, provider, created_at, expires_at, last_seen_at,
		       COALESCE(user_agent,''), COALESCE(ip,''), active_tenant_id
		FROM auth_sessions WHERE id = $1 AND expires_at > NOW()`, sessionID)
	var sess domain.AuthSession
	err := row.Scan(&sess.ID, &sess.UserID, &sess.Provider, &sess.CreatedAt,
		&sess.ExpiresAt, &sess.LastSeenAt, &sess.UserAgent, &sess.IP, &sess.ActiveTenantID)
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

// SetActiveTenant records which tenant the session is currently viewing.
func (r *SessionRepository) SetActiveTenant(ctx context.Context, sessionID string, tenantID int64) error {
	_, err := r.db.Exec(ctx,
		`UPDATE auth_sessions SET active_tenant_id = $2 WHERE id = $1`, sessionID, tenantID)
	return err
}

func (r *SessionRepository) TouchSession(ctx context.Context, sessionID string) error {
	_, err := r.db.Exec(ctx, `UPDATE auth_sessions SET last_seen_at = NOW() WHERE id = $1`, sessionID)
	return err
}

func (r *SessionRepository) DeleteSession(ctx context.Context, sessionID string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM auth_sessions WHERE id = $1`, sessionID)
	return err
}

func (r *SessionRepository) DeleteExpiredSessions(ctx context.Context) error {
	_, err := r.db.Exec(ctx, `DELETE FROM auth_sessions WHERE expires_at < NOW()`)
	return err
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
