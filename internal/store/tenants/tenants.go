package tenants

import (
	"context"
	"errors"
	"fmt"

	"okrs/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidSlug = errors.New("tenants: invalid slug")
	ErrSlugTaken   = errors.New("tenants: slug already taken")
	ErrNotFound    = errors.New("tenants: not found")
)

// TenantRepository handles tenant persistence.
type TenantRepository struct {
	db *pgxpool.Pool
}

func NewTenantRepository(db *pgxpool.Pool) *TenantRepository {
	return &TenantRepository{db: db}
}

func (r *TenantRepository) Create(ctx context.Context, slug, name string) (*domain.Tenant, error) {
	if !domain.ValidTenantSlug(slug) {
		return nil, ErrInvalidSlug
	}
	var t domain.Tenant
	err := r.db.QueryRow(ctx, `
		INSERT INTO tenants (slug, name) VALUES ($1, $2)
		RETURNING id, slug, name, status, created_at, deleted_at`,
		slug, name).Scan(&t.ID, &t.Slug, &t.Name, &t.Status, &t.CreatedAt, &t.DeletedAt)
	if err != nil {
		if pgErrCode(err) == "23505" { // unique_violation
			return nil, ErrSlugTaken
		}
		return nil, fmt.Errorf("create tenant: %w", err)
	}
	return &t, nil
}

func (r *TenantRepository) GetByID(ctx context.Context, id int64) (*domain.Tenant, error) {
	return r.getBy(ctx, `WHERE id = $1`, id)
}

func (r *TenantRepository) GetBySlug(ctx context.Context, slug string) (*domain.Tenant, error) {
	return r.getBy(ctx, `WHERE slug = $1`, slug)
}

func (r *TenantRepository) getBy(ctx context.Context, where string, arg any) (*domain.Tenant, error) {
	var t domain.Tenant
	err := r.db.QueryRow(ctx, `
		SELECT id, slug, name, status, created_at, deleted_at FROM tenants `+where, arg).
		Scan(&t.ID, &t.Slug, &t.Name, &t.Status, &t.CreatedAt, &t.DeletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *TenantRepository) List(ctx context.Context) ([]domain.Tenant, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, slug, name, status, created_at, deleted_at FROM tenants
		WHERE deleted_at IS NULL ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Tenant
	for rows.Next() {
		var t domain.Tenant
		if err := rows.Scan(&t.ID, &t.Slug, &t.Name, &t.Status, &t.CreatedAt, &t.DeletedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// SetStatus transitions a tenant between active/suspended.
func (r *TenantRepository) SetStatus(ctx context.Context, id int64, status domain.TenantStatus) error {
	ct, err := r.db.Exec(ctx, `UPDATE tenants SET status = $2 WHERE id = $1`, id, status)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func pgErrCode(err error) string {
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) {
		return pgErr.SQLState()
	}
	return ""
}
