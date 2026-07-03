package periods

import (
	"context"
	"time"

	"okrs/internal/domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PeriodRepository handles all period persistence.
type PeriodRepository struct {
	db *pgxpool.Pool
}

func NewPeriodRepository(db *pgxpool.Pool) *PeriodRepository {
	return &PeriodRepository{db: db}
}

// PeriodInput is used by CreatePeriod and UpdatePeriod.
type PeriodInput struct {
	Name      string
	StartDate time.Time
	EndDate   time.Time
}

func (r *PeriodRepository) ListPeriods(ctx context.Context, scope domain.TenantScope) ([]domain.Period, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, name, start_date, end_date, archived_at, created_at, updated_at
		FROM periods
		WHERE tenant_id = $1
		ORDER BY start_date, id`, scope.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	periods := make([]domain.Period, 0)
	for rows.Next() {
		var period domain.Period
		if err := rows.Scan(&period.ID, &period.Name, &period.StartDate, &period.EndDate, &period.ArchivedAt, &period.CreatedAt, &period.UpdatedAt); err != nil {
			return nil, err
		}
		periods = append(periods, period)
	}
	return periods, rows.Err()
}

func (r *PeriodRepository) GetPeriod(ctx context.Context, scope domain.TenantScope, periodID int64) (domain.Period, error) {
	var period domain.Period
	row := r.db.QueryRow(ctx, `
		SELECT id, name, start_date, end_date, archived_at, created_at, updated_at
		FROM periods
		WHERE id=$1 AND tenant_id=$2`, periodID, scope.TenantID)
	if err := row.Scan(&period.ID, &period.Name, &period.StartDate, &period.EndDate, &period.ArchivedAt, &period.CreatedAt, &period.UpdatedAt); err != nil {
		return domain.Period{}, err
	}
	return period, nil
}

func (r *PeriodRepository) FindPeriodForDate(ctx context.Context, scope domain.TenantScope, date time.Time) (domain.Period, error) {
	var period domain.Period
	row := r.db.QueryRow(ctx, `
		SELECT id, name, start_date, end_date, archived_at, created_at, updated_at
		FROM periods
		WHERE tenant_id=$2 AND $1::date BETWEEN start_date AND end_date
		ORDER BY (end_date - start_date) ASC, end_date DESC
		LIMIT 1`, date, scope.TenantID)
	if err := row.Scan(&period.ID, &period.Name, &period.StartDate, &period.EndDate, &period.ArchivedAt, &period.CreatedAt, &period.UpdatedAt); err != nil {
		return domain.Period{}, err
	}
	return period, nil
}

func (r *PeriodRepository) CreatePeriod(ctx context.Context, scope domain.TenantScope, input PeriodInput) (int64, error) {
	var id int64
	row := r.db.QueryRow(ctx, `
		INSERT INTO periods (name, start_date, end_date, tenant_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id`, input.Name, input.StartDate, input.EndDate, scope.TenantID)
	if err := row.Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

func (r *PeriodRepository) ArchivePeriod(ctx context.Context, scope domain.TenantScope, periodID int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE periods SET archived_at=NOW(), updated_at=NOW()
		WHERE id=$1 AND tenant_id=$2`, periodID, scope.TenantID)
	return err
}

func (r *PeriodRepository) UnarchivePeriod(ctx context.Context, scope domain.TenantScope, periodID int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE periods SET archived_at=NULL, updated_at=NOW()
		WHERE id=$1 AND tenant_id=$2`, periodID, scope.TenantID)
	return err
}

func (r *PeriodRepository) UpdatePeriod(ctx context.Context, scope domain.TenantScope, periodID int64, input PeriodInput) error {
	_, err := r.db.Exec(ctx, `
		UPDATE periods
		SET name=$1, start_date=$2, end_date=$3, updated_at=NOW()
		WHERE id=$4 AND tenant_id=$5`, input.Name, input.StartDate, input.EndDate, periodID, scope.TenantID)
	return err
}

func (r *PeriodRepository) DeletePeriod(ctx context.Context, scope domain.TenantScope, periodID int64) error {
	_, err := r.db.Exec(ctx, `DELETE FROM periods WHERE id=$1 AND tenant_id=$2`, periodID, scope.TenantID)
	return err
}
