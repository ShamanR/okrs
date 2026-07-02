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

// ClaimResult is the outcome of a successful Consume: which tenant/role the claimer joins.
type ClaimResult struct {
	TenantID int64
	Role     domain.Role
}

// Create inserts a generic pending invite link (no email). maxUses nil = unlimited,
// 1 = one-time, N = up to N uses. Only the token hash is stored.
func (r *InvitationRepository) Create(ctx context.Context, scope domain.TenantScope, role domain.Role, tokenHash string, createdBy int64, maxUses *int, expiresAt *time.Time) (*domain.Invitation, error) {
	var inv domain.Invitation
	err := r.db.QueryRow(ctx, `
		INSERT INTO tenant_invitations (tenant_id, email, role, token_hash, status, created_by_user_id, expires_at, max_uses, use_count)
		VALUES ($1, NULL, $2, $3, 'pending', $4, $5, $6, 0)
		RETURNING id, tenant_id, email, role, status, created_by_user_id, created_at, expires_at, max_uses, use_count`,
		scope.TenantID, role, tokenHash, createdBy, expiresAt, maxUses).
		Scan(&inv.ID, &inv.TenantID, &inv.Email, &inv.Role, &inv.Status, &inv.CreatedByUserID, &inv.CreatedAt, &inv.ExpiresAt, &inv.MaxUses, &inv.UseCount)
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

// Consume atomically redeems a pending invite link by token hash. A single UPDATE increments
// use_count and, for capped links, flips status to 'claimed' on the final use. The WHERE clause
// rejects revoked/expired/exhausted links, so concurrent callers cannot over-consume a cap.
// Returns ErrNotFound when nothing valid matched.
func (r *InvitationRepository) Consume(ctx context.Context, tokenHash string) (*ClaimResult, error) {
	var res ClaimResult
	err := r.db.QueryRow(ctx, `
		UPDATE tenant_invitations
		SET use_count = use_count + 1,
		    status = CASE WHEN max_uses IS NOT NULL AND use_count + 1 >= max_uses THEN 'claimed' ELSE status END
		WHERE token_hash = $1 AND status = 'pending'
		  AND (expires_at IS NULL OR expires_at > now())
		  AND (max_uses IS NULL OR use_count < max_uses)
		RETURNING tenant_id, role`,
		tokenHash).Scan(&res.TenantID, &res.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// Revoke marks a pending invite link revoked. Tenant-scoped and idempotent: a missing/foreign/
// already-revoked id affects zero rows and returns nil.
func (r *InvitationRepository) Revoke(ctx context.Context, scope domain.TenantScope, id int64) error {
	_, err := r.db.Exec(ctx,
		`UPDATE tenant_invitations SET status = 'revoked' WHERE id = $1 AND tenant_id = $2 AND status = 'pending'`,
		id, scope.TenantID)
	return err
}

func (r *InvitationRepository) ListPendingByTenant(ctx context.Context, scope domain.TenantScope) ([]domain.Invitation, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, tenant_id, email, role, status, created_by_user_id, created_at, expires_at, max_uses, use_count
		FROM tenant_invitations
		WHERE tenant_id = $1 AND status = 'pending' ORDER BY created_at DESC`, scope.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Invitation
	for rows.Next() {
		var inv domain.Invitation
		if err := rows.Scan(&inv.ID, &inv.TenantID, &inv.Email, &inv.Role, &inv.Status, &inv.CreatedByUserID, &inv.CreatedAt, &inv.ExpiresAt, &inv.MaxUses, &inv.UseCount); err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}
