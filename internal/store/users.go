package store

import (
	"context"
	"encoding/json"
	"time"

	"okrs/internal/domain"
)

type UpsertUserInput struct {
	ProviderSubjectKey string
	Provider           string
	Subject            string
	DisplayName        string
	AvatarURL          string
	Email              string
}

func (s *Store) UpsertUser(ctx context.Context, in UpsertUserInput) (*domain.User, error) {
	now := time.Now()
	row := s.DB.QueryRow(ctx, `
		INSERT INTO users (provider_subject_key, provider, subject, display_name, avatar_url, email, created_at, updated_at, last_login_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $7, $7)
		ON CONFLICT (provider_subject_key) DO UPDATE SET
			display_name  = EXCLUDED.display_name,
			avatar_url    = EXCLUDED.avatar_url,
			email         = EXCLUDED.email,
			updated_at    = EXCLUDED.updated_at,
			last_login_at = EXCLUDED.last_login_at
		RETURNING id, provider_subject_key, provider, subject, display_name, avatar_url, COALESCE(email,''), attributes_json, is_admin, created_at, updated_at, last_login_at`,
		in.ProviderSubjectKey, in.Provider, in.Subject, in.DisplayName, in.AvatarURL, nullableString(in.Email),
		now,
	)
	return scanUser(row)
}

func (s *Store) GetUser(ctx context.Context, id int64) (*domain.User, error) {
	row := s.DB.QueryRow(ctx, `
		SELECT id, provider_subject_key, provider, subject, display_name, avatar_url, COALESCE(email,''), attributes_json, is_admin, created_at, updated_at, last_login_at
		FROM users WHERE id = $1`, id)
	return scanUser(row)
}

func (s *Store) ListUsers(ctx context.Context) ([]*domain.User, error) {
	rows, err := s.DB.Query(ctx, `
		SELECT id, provider_subject_key, provider, subject, display_name, avatar_url, COALESCE(email,''), attributes_json, is_admin, created_at, updated_at, last_login_at
		FROM users WHERE provider NOT IN ('system') ORDER BY last_login_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []*domain.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *Store) ListUserLeadTeams(ctx context.Context) (map[string]string, error) {
	rows, err := s.DB.Query(ctx, `
		SELECT lead, name FROM teams
		WHERE deleted_at IS NULL AND lead != ''
		ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]string)
	for rows.Next() {
		var lead, teamName string
		if err := rows.Scan(&lead, &teamName); err != nil {
			return nil, err
		}
		if _, exists := result[lead]; !exists {
			result[lead] = teamName
		}
	}
	return result, rows.Err()
}

func (s *Store) SetUserAdmin(ctx context.Context, userID int64, isAdmin bool) error {
	_, err := s.DB.Exec(ctx, `UPDATE users SET is_admin = $1, updated_at = NOW() WHERE id = $2`, isAdmin, userID)
	return err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanUser(row scanner) (*domain.User, error) {
	var u domain.User
	var attrRaw []byte
	err := row.Scan(
		&u.ID, &u.ProviderSubjectKey, &u.Provider, &u.Subject,
		&u.DisplayName, &u.AvatarURL, &u.Email, &attrRaw,
		&u.IsAdmin, &u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt,
	)
	if err != nil {
		return nil, err
	}
	if len(attrRaw) > 0 {
		_ = json.Unmarshal(attrRaw, &u.AttributesJSON)
	}
	return &u, nil
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
