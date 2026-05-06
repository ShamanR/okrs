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
		RETURNING id, udid, provider_subject_key, provider, subject, display_name, avatar_url, COALESCE(email,''), attributes_json, is_admin, created_at, updated_at, last_login_at`,
		in.ProviderSubjectKey, in.Provider, in.Subject, in.DisplayName, in.AvatarURL, nullableString(in.Email),
		now,
	)
	return scanUser(row)
}

func (s *Store) GetUser(ctx context.Context, id int64) (*domain.User, error) {
	row := s.DB.QueryRow(ctx, `
		SELECT id, udid, provider_subject_key, provider, subject, display_name, avatar_url, COALESCE(email,''), attributes_json, is_admin, created_at, updated_at, last_login_at
		FROM users WHERE id = $1`, id)
	return scanUser(row)
}

func (s *Store) ListUsers(ctx context.Context) ([]*domain.User, error) {
	rows, err := s.DB.Query(ctx, `
		SELECT id, udid, provider_subject_key, provider, subject, display_name, avatar_url, COALESCE(email,''), attributes_json, is_admin, created_at, updated_at, last_login_at
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

// GetUsersByUDIDs returns non-system users with the given UDIDs (max 100, extras silently dropped).
func (s *Store) GetUsersByUDIDs(ctx context.Context, udids []string) ([]*domain.User, error) {
	if len(udids) == 0 {
		return nil, nil
	}
	if len(udids) > 100 {
		udids = udids[:100]
	}
	rows, err := s.DB.Query(ctx, `
		SELECT id, udid, provider_subject_key, provider, subject, display_name, avatar_url, COALESCE(email,''), attributes_json, is_admin, created_at, updated_at, last_login_at
		FROM users WHERE udid = ANY($1) AND provider NOT IN ('system')`, udids)
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

// SearchUsersInScope returns up to limit non-system users matching q (case-insensitive substring of
// display_name or email) that are visible in the given scope:
//   - scopeTeamIDs == nil  → admin/unrestricted: search all users
//   - scopeTeamIDs != nil  → only users who have a hierarchy grant covering ≥1 scoped team
//     OR are a lead of any team in the scope
//
// q == "" returns the most-recently-logged-in users.
func (s *Store) SearchUsersInScope(ctx context.Context, scopeTeamIDs []int64, q string, limit int) ([]*domain.User, error) {
	if limit <= 0 {
		limit = 20
	}

	if scopeTeamIDs == nil {
		return s.searchUsersUnrestricted(ctx, q, limit)
	}
	return s.searchUsersScoped(ctx, scopeTeamIDs, q, limit)
}

func (s *Store) searchUsersUnrestricted(ctx context.Context, q string, limit int) ([]*domain.User, error) {
	rows, err := s.DB.Query(ctx, `
		SELECT id, udid, provider_subject_key, provider, subject, display_name, avatar_url, COALESCE(email,''), attributes_json, is_admin, created_at, updated_at, last_login_at
		FROM users
		WHERE provider NOT IN ('system')
		AND ($1 = '' OR LOWER(display_name) LIKE '%' || LOWER($1) || '%' OR LOWER(COALESCE(email,'')) LIKE '%' || LOWER($1) || '%')
		ORDER BY last_login_at DESC NULLS LAST
		LIMIT $2`, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanUsersRows(rows)
}

func (s *Store) searchUsersScoped(ctx context.Context, scopeTeamIDs []int64, q string, limit int) ([]*domain.User, error) {
	// Scoped search: users who have a grant to any team that covers ≥1 scope node
	// (scope nodes + their ancestors form the set of "covering" teams), plus team leads.
	// The recursive CTE walks up from scope nodes to their ancestors so that a grant
	// to a parent team is correctly recognised as covering the child scope node.
	rows, err := s.DB.Query(ctx, `
		WITH RECURSIVE ancestor_scope AS (
			SELECT id, parent_id FROM teams WHERE id = ANY($1)
			UNION ALL
			SELECT t.id, t.parent_id FROM teams t
			JOIN ancestor_scope a ON t.id = a.parent_id
		)
		SELECT
			u.id, u.udid, u.provider_subject_key, u.provider, u.subject,
			u.display_name, u.avatar_url, COALESCE(u.email,''), u.attributes_json,
			u.is_admin, u.created_at, u.updated_at, u.last_login_at
		FROM users u
		WHERE u.provider NOT IN ('system')
		AND (
			EXISTS (
				SELECT 1 FROM user_hierarchy_grants g
				JOIN ancestor_scope a ON g.team_id = a.id
				WHERE g.user_id = u.id
			)
			OR EXISTS (
				SELECT 1 FROM teams t
				WHERE t.id = ANY($1)
				AND t.lead = u.display_name
				AND t.deleted_at IS NULL
				AND t.lead != ''
			)
		)
		AND ($2 = '' OR LOWER(u.display_name) LIKE '%' || LOWER($2) || '%' OR LOWER(COALESCE(u.email,'')) LIKE '%' || LOWER($2) || '%')
		ORDER BY u.last_login_at DESC NULLS LAST
		LIMIT $3`, scopeTeamIDs, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanUsersRows(rows)
}

func scanUsersRows(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]*domain.User, error) {
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

// GetUsersByDisplayNames returns non-system users whose display_name is in names.
func (s *Store) GetUsersByDisplayNames(ctx context.Context, names []string) ([]*domain.User, error) {
	if len(names) == 0 {
		return nil, nil
	}
	rows, err := s.DB.Query(ctx, `
		SELECT id, udid, provider_subject_key, provider, subject, display_name, avatar_url, COALESCE(email,''), attributes_json, is_admin, created_at, updated_at, last_login_at
		FROM users WHERE display_name = ANY($1) AND provider NOT IN ('system')`, names)
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
		&u.ID, &u.UDID, &u.ProviderSubjectKey, &u.Provider, &u.Subject,
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
