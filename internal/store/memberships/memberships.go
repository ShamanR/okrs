package memberships

import (
	"context"
	"errors"

	"okrs/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("memberships: not found")

// MembershipRepository handles membership persistence (user ↔ tenant ↔ role/status).
type MembershipRepository struct {
	db *pgxpool.Pool
}

func NewMembershipRepository(db *pgxpool.Pool) *MembershipRepository {
	return &MembershipRepository{db: db}
}

func (r *MembershipRepository) Upsert(ctx context.Context, m domain.Membership) (*domain.Membership, error) {
	role := m.Role
	if role == "" {
		role = domain.RoleUser
	}
	status := m.Status
	if status == "" {
		status = domain.MembershipActive
	}
	var out domain.Membership
	err := r.db.QueryRow(ctx, `
		INSERT INTO memberships (user_id, tenant_id, role, status, created_by_user_id)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, tenant_id) DO UPDATE SET role = EXCLUDED.role, status = EXCLUDED.status
		RETURNING id, user_id, tenant_id, role, status, created_at, created_by_user_id`,
		m.UserID, m.TenantID, role, status, m.CreatedByUserID).
		Scan(&out.ID, &out.UserID, &out.TenantID, &out.Role, &out.Status, &out.CreatedAt, &out.CreatedByUserID)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *MembershipRepository) Get(ctx context.Context, userID, tenantID int64) (*domain.Membership, error) {
	var m domain.Membership
	err := r.db.QueryRow(ctx, `
		SELECT id, user_id, tenant_id, role, status, created_at, created_by_user_id
		FROM memberships WHERE user_id = $1 AND tenant_id = $2`, userID, tenantID).
		Scan(&m.ID, &m.UserID, &m.TenantID, &m.Role, &m.Status, &m.CreatedAt, &m.CreatedByUserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *MembershipRepository) ListByUser(ctx context.Context, userID int64) ([]domain.Membership, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, user_id, tenant_id, role, status, created_at, created_by_user_id
		FROM memberships WHERE user_id = $1 AND status = 'active' ORDER BY tenant_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Membership
	for rows.Next() {
		var m domain.Membership
		if err := rows.Scan(&m.ID, &m.UserID, &m.TenantID, &m.Role, &m.Status, &m.CreatedAt, &m.CreatedByUserID); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *MembershipRepository) SetStatus(ctx context.Context, userID, tenantID int64, status domain.MembershipStatus) error {
	ct, err := r.db.Exec(ctx, `UPDATE memberships SET status = $3 WHERE user_id = $1 AND tenant_id = $2`,
		userID, tenantID, status)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
