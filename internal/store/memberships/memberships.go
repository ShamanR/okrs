package memberships

import (
	"context"
	"errors"
	"time"

	"okrs/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("memberships: not found")

// AccessRequest is the read model for the tenant-admin "join requests" queue.
type AccessRequest struct {
	UserID      int64
	DisplayName string
	Email       string
	Role        domain.Role
	CreatedAt   time.Time
}

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

// ListAccessRequests returns the tenant's pending (status='requested') memberships joined
// to users for display in the tenant-admin queue.
func (r *MembershipRepository) ListAccessRequests(ctx context.Context, scope domain.TenantScope) ([]AccessRequest, error) {
	rows, err := r.db.Query(ctx, `
		SELECT m.user_id, u.display_name, COALESCE(u.email,''), m.role, m.created_at
		FROM memberships m JOIN users u ON u.id = m.user_id
		WHERE m.tenant_id = $1 AND m.status = 'requested'
		ORDER BY m.created_at`, scope.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AccessRequest
	for rows.Next() {
		var a AccessRequest
		if err := rows.Scan(&a.UserID, &a.DisplayName, &a.Email, &a.Role, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// DeleteRequested removes a pending join-request membership (deny). No-op if none.
func (r *MembershipRepository) DeleteRequested(ctx context.Context, scope domain.TenantScope, userID int64) error {
	_, err := r.db.Exec(ctx, `DELETE FROM memberships WHERE user_id = $2 AND tenant_id = $1 AND status = 'requested'`,
		scope.TenantID, userID)
	return err
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
