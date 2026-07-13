package users

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"okrs/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when an update targets a user id that does not exist.
var ErrNotFound = errors.New("users: not found")

// UserRepository handles user persistence.
type UserRepository struct {
	db *pgxpool.Pool
}

func NewUserRepository(db *pgxpool.Pool) *UserRepository {
	return &UserRepository{db: db}
}

type UpsertUserInput struct {
	ProviderSubjectKey string
	Provider           string
	Subject            string
	DisplayName        string
	AvatarURL          string
	Email              string
}

func (r *UserRepository) UpsertUser(ctx context.Context, in UpsertUserInput) (*domain.User, error) {
	now := time.Now()
	row := r.db.QueryRow(ctx, `
		INSERT INTO users (provider_subject_key, provider, subject, display_name, avatar_url, email, created_at, updated_at, last_login_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $7, $7)
		ON CONFLICT (provider_subject_key) DO UPDATE SET
			display_name  = EXCLUDED.display_name,
			avatar_url    = EXCLUDED.avatar_url,
			email         = EXCLUDED.email,
			updated_at    = EXCLUDED.updated_at,
			last_login_at = EXCLUDED.last_login_at
		RETURNING id, udid, provider_subject_key, provider, subject, display_name, avatar_url, COALESCE(email,''), attributes_json, is_system_admin, created_at, updated_at, last_login_at`,
		in.ProviderSubjectKey, in.Provider, in.Subject, in.DisplayName, in.AvatarURL, nullableString(in.Email),
		now,
	)
	return scanUser(row)
}

func (r *UserRepository) GetUser(ctx context.Context, id int64) (*domain.User, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, udid, provider_subject_key, provider, subject, display_name, avatar_url, COALESCE(email,''), attributes_json, is_system_admin, created_at, updated_at, last_login_at
		FROM users WHERE id = $1`, id)
	return scanUser(row)
}

func (r *UserRepository) ListUsers(ctx context.Context) ([]*domain.User, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, udid, provider_subject_key, provider, subject, display_name, avatar_url, COALESCE(email,''), attributes_json, is_system_admin, created_at, updated_at, last_login_at
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
func (r *UserRepository) GetUsersByUDIDs(ctx context.Context, udids []string) ([]*domain.User, error) {
	if len(udids) == 0 {
		return nil, nil
	}
	if len(udids) > 100 {
		udids = udids[:100]
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, udid, provider_subject_key, provider, subject, display_name, avatar_url, COALESCE(email,''), attributes_json, is_system_admin, created_at, updated_at, last_login_at
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

// SearchUsersUnrestricted returns up to limit non-system users matching q with no scope filter.
// q == "" returns the most-recently-logged-in users.
func (r *UserRepository) SearchUsersUnrestricted(ctx context.Context, q string, limit int) ([]*domain.User, error) {
	if limit <= 0 {
		limit = 20
	}
	return r.searchUsersUnrestricted(ctx, q, limit)
}

func (r *UserRepository) searchUsersUnrestricted(ctx context.Context, q string, limit int) ([]*domain.User, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, udid, provider_subject_key, provider, subject, display_name, avatar_url, COALESCE(email,''), attributes_json, is_system_admin, created_at, updated_at, last_login_at
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

// SearchUsersInSet returns up to limit non-system users whose id is in userIDs OR whose
// udid is in leadUDIDs, filtered by optional text query q.
// Returns nil when both userIDs and leadUDIDs are empty.
func (r *UserRepository) SearchUsersInSet(ctx context.Context, userIDs []int64, leadUDIDs []string, q string, limit int) ([]*domain.User, error) {
	if limit <= 0 {
		limit = 20
	}
	if len(userIDs) == 0 && len(leadUDIDs) == 0 {
		return nil, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, udid, provider_subject_key, provider, subject, display_name, avatar_url, COALESCE(email,''), attributes_json, is_system_admin, created_at, updated_at, last_login_at
		FROM users
		WHERE provider NOT IN ('system')
		AND (id = ANY($1) OR udid = ANY($2))
		AND ($3 = '' OR LOWER(display_name) LIKE '%' || LOWER($3) || '%' OR LOWER(COALESCE(email,'')) LIKE '%' || LOWER($3) || '%')
		ORDER BY last_login_at DESC NULLS LAST
		LIMIT $4`, userIDs, leadUDIDs, q, limit)
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
func (r *UserRepository) GetUsersByDisplayNames(ctx context.Context, names []string) ([]*domain.User, error) {
	if len(names) == 0 {
		return nil, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, udid, provider_subject_key, provider, subject, display_name, avatar_url, COALESCE(email,''), attributes_json, is_system_admin, created_at, updated_at, last_login_at
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

// ListUserLeadTeams returns a map of user UDID → team name for all active team leads.
func (r *UserRepository) ListUserLeadTeams(ctx context.Context) (map[string]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT lead_udid, name FROM teams
		WHERE deleted_at IS NULL AND lead_udid IS NOT NULL
		ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]string)
	for rows.Next() {
		var leadUDID, teamName string
		if err := rows.Scan(&leadUDID, &teamName); err != nil {
			return nil, err
		}
		if _, exists := result[leadUDID]; !exists {
			result[leadUDID] = teamName
		}
	}
	return result, rows.Err()
}

// ValidateUDIDsExist returns UDIDs from the input slice that do NOT exist in users.
func (r *UserRepository) ValidateUDIDsExist(ctx context.Context, udids []string) ([]string, error) {
	if len(udids) == 0 {
		return nil, nil
	}
	rows, err := r.db.Query(ctx, `SELECT udid FROM users WHERE udid = ANY($1)`, udids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	found := make(map[string]struct{}, len(udids))
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		found[u] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var missing []string
	for _, u := range udids {
		if _, ok := found[u]; !ok {
			missing = append(missing, u)
		}
	}
	return missing, nil
}

// SetSystemAdmin sets the tenant-less instance superadmin flag. ErrNotFound if the user is missing.
func (r *UserRepository) SetSystemAdmin(ctx context.Context, userID int64, v bool) error {
	ct, err := r.db.Exec(ctx, `UPDATE users SET is_system_admin = $1, updated_at = NOW() WHERE id = $2`, v, userID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AnySystemAdmin reports whether at least one system admin exists (bootstrap guard).
func (r *UserRepository) AnySystemAdmin(ctx context.Context) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE is_system_admin)`).Scan(&exists)
	return exists, err
}

// CountSystemAdmins returns how many instance system-admins exist (last-admin guard input).
func (r *UserRepository) CountSystemAdmins(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `SELECT count(*) FROM users WHERE is_system_admin`).Scan(&n)
	return n, err
}

// IsSystemAdmin reports a single user's current system-admin flag. ErrNotFound if the user is missing.
func (r *UserRepository) IsSystemAdmin(ctx context.Context, userID int64) (bool, error) {
	var v bool
	err := r.db.QueryRow(ctx, `SELECT is_system_admin FROM users WHERE id = $1`, userID).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	return v, err
}

type scanner interface {
	Scan(dest ...any) error
}

// TenantUser pairs a user with its membership in a specific tenant.
type TenantUser struct {
	User   *domain.User
	Status domain.MembershipStatus
	Role   domain.Role
}

// ListByTenant returns every user with a membership in the tenant (any status), with that
// membership's status and role, ordered by display name.
func (r *UserRepository) ListByTenant(ctx context.Context, scope domain.TenantScope) ([]TenantUser, error) {
	rows, err := r.db.Query(ctx, `
		SELECT u.id, u.udid, u.provider_subject_key, u.provider, u.subject, u.display_name,
		       u.avatar_url, COALESCE(u.email,''), u.attributes_json, u.is_system_admin,
		       u.created_at, u.updated_at, u.last_login_at, m.status, m.role
		FROM memberships m JOIN users u ON u.id = m.user_id
		WHERE m.tenant_id = $1
		ORDER BY u.display_name`, scope.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TenantUser
	for rows.Next() {
		var u domain.User
		var attrRaw []byte
		var status domain.MembershipStatus
		var role domain.Role
		if err := rows.Scan(
			&u.ID, &u.UDID, &u.ProviderSubjectKey, &u.Provider, &u.Subject, &u.DisplayName,
			&u.AvatarURL, &u.Email, &attrRaw, &u.IsSystemAdmin,
			&u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt, &status, &role,
		); err != nil {
			return nil, err
		}
		if len(attrRaw) > 0 {
			_ = json.Unmarshal(attrRaw, &u.AttributesJSON)
		}
		out = append(out, TenantUser{User: &u, Status: status, Role: role})
	}
	return out, rows.Err()
}

func scanUser(row scanner) (*domain.User, error) {
	var u domain.User
	var attrRaw []byte
	err := row.Scan(
		&u.ID, &u.UDID, &u.ProviderSubjectKey, &u.Provider, &u.Subject,
		&u.DisplayName, &u.AvatarURL, &u.Email, &attrRaw,
		&u.IsSystemAdmin, &u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt,
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
