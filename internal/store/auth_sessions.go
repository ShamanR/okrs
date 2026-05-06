package store

import (
	"context"
	"time"

	"okrs/internal/domain"
)

func (s *Store) CreateSession(ctx context.Context, sessionID string, userID int64, provider string, ttl time.Duration, userAgent, ip string) (*domain.AuthSession, error) {
	now := time.Now()
	expires := now.Add(ttl)
	_, err := s.DB.Exec(ctx, `
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

func (s *Store) GetSession(ctx context.Context, sessionID string) (*domain.AuthSession, error) {
	row := s.DB.QueryRow(ctx, `
		SELECT id, user_id, provider, created_at, expires_at, last_seen_at,
		       COALESCE(user_agent,''), COALESCE(ip,'')
		FROM auth_sessions WHERE id = $1 AND expires_at > NOW()`, sessionID)
	var sess domain.AuthSession
	err := row.Scan(&sess.ID, &sess.UserID, &sess.Provider, &sess.CreatedAt,
		&sess.ExpiresAt, &sess.LastSeenAt, &sess.UserAgent, &sess.IP)
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *Store) TouchSession(ctx context.Context, sessionID string) error {
	_, err := s.DB.Exec(ctx, `UPDATE auth_sessions SET last_seen_at = NOW() WHERE id = $1`, sessionID)
	return err
}

func (s *Store) DeleteSession(ctx context.Context, sessionID string) error {
	_, err := s.DB.Exec(ctx, `DELETE FROM auth_sessions WHERE id = $1`, sessionID)
	return err
}

func (s *Store) DeleteExpiredSessions(ctx context.Context) error {
	_, err := s.DB.Exec(ctx, `DELETE FROM auth_sessions WHERE expires_at < NOW()`)
	return err
}
