package invitations

import (
	"context"
	"errors"
	"time"

	"okrs/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("invitations: not found")

// InvitationRepository handles tenant_invitations persistence. Tokens are stored hashed;
// the raw token lives only in the delivered link.
type InvitationRepository struct {
	db *pgxpool.Pool
}

func NewInvitationRepository(db *pgxpool.Pool) *InvitationRepository {
	return &InvitationRepository{db: db}
}

func (r *InvitationRepository) Create(ctx context.Context, scope domain.TenantScope, email string, role domain.Role, tokenHash string, createdBy int64, expiresAt *time.Time) (*domain.Invitation, error) {
	var inv domain.Invitation
	err := r.db.QueryRow(ctx, `
		INSERT INTO tenant_invitations (tenant_id, email, role, token_hash, status, created_by_user_id, expires_at)
		VALUES ($1, $2, $3, $4, 'pending', $5, $6)
		RETURNING id, tenant_id, email, role, status, created_by_user_id, created_at, expires_at`,
		scope.TenantID, email, role, tokenHash, createdBy, expiresAt).
		Scan(&inv.ID, &inv.TenantID, &inv.Email, &inv.Role, &inv.Status, &inv.CreatedByUserID, &inv.CreatedAt, &inv.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

// GetPendingByTokenHash is global by token (the claimer has no tenant context yet); the
// token hash carries its own tenant. Returns ErrNotFound if absent, claimed, or expired.
func (r *InvitationRepository) GetPendingByTokenHash(ctx context.Context, tokenHash string) (*domain.Invitation, error) {
	var inv domain.Invitation
	err := r.db.QueryRow(ctx, `
		SELECT id, tenant_id, email, role, status, created_by_user_id, created_at, expires_at
		FROM tenant_invitations
		WHERE token_hash = $1 AND status = 'pending' AND (expires_at IS NULL OR expires_at > now())`,
		tokenHash).
		Scan(&inv.ID, &inv.TenantID, &inv.Email, &inv.Role, &inv.Status, &inv.CreatedByUserID, &inv.CreatedAt, &inv.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

// MarkClaimed atomically consumes a pending invitation; returns true iff a row changed.
func (r *InvitationRepository) MarkClaimed(ctx context.Context, id int64) (bool, error) {
	ct, err := r.db.Exec(ctx, `UPDATE tenant_invitations SET status = 'claimed' WHERE id = $1 AND status = 'pending'`, id)
	if err != nil {
		return false, err
	}
	return ct.RowsAffected() == 1, nil
}

func (r *InvitationRepository) ListPendingByTenant(ctx context.Context, scope domain.TenantScope) ([]domain.Invitation, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, tenant_id, email, role, status, created_by_user_id, created_at, expires_at
		FROM tenant_invitations
		WHERE tenant_id = $1 AND status = 'pending' ORDER BY created_at DESC`, scope.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Invitation
	for rows.Next() {
		var inv domain.Invitation
		if err := rows.Scan(&inv.ID, &inv.TenantID, &inv.Email, &inv.Role, &inv.Status, &inv.CreatedByUserID, &inv.CreatedAt, &inv.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}
