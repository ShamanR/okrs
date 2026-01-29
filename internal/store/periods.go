package store

import (
	"context"
	"time"

	"okrs/internal/domain"
)

func (s *Store) ListPeriods(ctx context.Context) ([]domain.Period, error) {
	rows, err := s.DB.Query(ctx, `
		SELECT id, name, start_date, end_date, sort_order, created_at, updated_at
		FROM periods
		ORDER BY sort_order, start_date, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	periods := make([]domain.Period, 0)
	for rows.Next() {
		var period domain.Period
		if err := rows.Scan(&period.ID, &period.Name, &period.StartDate, &period.EndDate, &period.SortOrder, &period.CreatedAt, &period.UpdatedAt); err != nil {
			return nil, err
		}
		periods = append(periods, period)
	}
	return periods, rows.Err()
}

func (s *Store) GetPeriod(ctx context.Context, periodID int64) (domain.Period, error) {
	var period domain.Period
	row := s.DB.QueryRow(ctx, `
		SELECT id, name, start_date, end_date, sort_order, created_at, updated_at
		FROM periods
		WHERE id=$1`, periodID)
	if err := row.Scan(&period.ID, &period.Name, &period.StartDate, &period.EndDate, &period.SortOrder, &period.CreatedAt, &period.UpdatedAt); err != nil {
		return domain.Period{}, err
	}
	return period, nil
}

func (s *Store) FindPeriodForDate(ctx context.Context, date time.Time) (domain.Period, error) {
	var period domain.Period
	row := s.DB.QueryRow(ctx, `
		SELECT id, name, start_date, end_date, sort_order, created_at, updated_at
		FROM periods
		WHERE $1::date BETWEEN start_date AND end_date
		ORDER BY sort_order DESC, end_date DESC
		LIMIT 1`, date)
	if err := row.Scan(&period.ID, &period.Name, &period.StartDate, &period.EndDate, &period.SortOrder, &period.CreatedAt, &period.UpdatedAt); err != nil {
		return domain.Period{}, err
	}
	return period, nil
}

func (s *Store) CreatePeriod(ctx context.Context, input PeriodInput) (int64, error) {
	var id int64
	row := s.DB.QueryRow(ctx, `
		INSERT INTO periods (name, start_date, end_date, sort_order)
		VALUES ($1, $2, $3, COALESCE((SELECT MAX(sort_order) + 1 FROM periods), 1))
		RETURNING id`, input.Name, input.StartDate, input.EndDate)
	if err := row.Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

func (s *Store) MovePeriod(ctx context.Context, periodID int64, direction int) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var currentOrder int
	if err := tx.QueryRow(ctx, `SELECT sort_order FROM periods WHERE id=$1 FOR UPDATE`, periodID).Scan(&currentOrder); err != nil {
		return err
	}

	var swapID int64
	var swapOrder int
	var comparator, ordering string
	if direction < 0 {
		comparator = "<"
		ordering = "DESC"
	} else {
		comparator = ">"
		ordering = "ASC"
	}
	query := `
		SELECT id, sort_order
		FROM periods
		WHERE sort_order ` + comparator + ` $1
		ORDER BY sort_order ` + ordering + `
		LIMIT 1
		FOR UPDATE`
	if err := tx.QueryRow(ctx, query, currentOrder).Scan(&swapID, &swapOrder); err != nil {
		return tx.Commit(ctx)
	}

	if _, err := tx.Exec(ctx, `UPDATE periods SET sort_order=$1 WHERE id=$2`, currentOrder, swapID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE periods SET sort_order=$1 WHERE id=$2`, swapOrder, periodID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
