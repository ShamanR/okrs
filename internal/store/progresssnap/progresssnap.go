// Package progresssnap persists daily per-team progress snapshots used by the
// period progress chart.
package progresssnap

import (
	"context"
	"time"

	"okrs/internal/domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository handles team_period_progress_snapshots persistence.
type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

// Snapshot is one team's progress at snapshot time.
type Snapshot struct {
	TeamID   int64
	Progress int
}

// UpsertSnapshots writes today's per-team progress for a period, idempotent per day
// on (tenant_id, period_id, team_id, snapshot_date).
func (r *Repository) UpsertSnapshots(ctx context.Context, scope domain.TenantScope, periodID int64, day time.Time, snaps []Snapshot) error {
	if len(snaps) == 0 {
		return nil
	}
	teamIDs := make([]int64, len(snaps))
	progs := make([]int32, len(snaps))
	for i, s := range snaps {
		teamIDs[i] = s.TeamID
		progs[i] = int32(s.Progress)
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO team_period_progress_snapshots (tenant_id, period_id, team_id, snapshot_date, progress)
		SELECT $1, $2, t.team_id, $4::date, t.progress
		FROM unnest($3::bigint[], $5::int[]) AS t(team_id, progress)
		ON CONFLICT (tenant_id, period_id, team_id, snapshot_date)
		DO UPDATE SET progress = EXCLUDED.progress`,
		scope.TenantID, periodID, teamIDs, day, progs)
	return err
}

// SeriesRow is one stored snapshot point (one team on one date).
type SeriesRow struct {
	TeamID   int64
	Date     time.Time
	Progress int
}

// ListSnapshots returns all snapshots for a period, optionally restricted to teamIDs
// (empty → all teams in the period), ordered by date.
func (r *Repository) ListSnapshots(ctx context.Context, scope domain.TenantScope, periodID int64, teamIDs []int64) ([]SeriesRow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT team_id, snapshot_date, progress
		FROM team_period_progress_snapshots
		WHERE tenant_id = $1 AND period_id = $2
		  AND ($3::bigint[] IS NULL OR team_id = ANY($3))
		ORDER BY snapshot_date`, scope.TenantID, periodID, nilIfEmpty(teamIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SeriesRow
	for rows.Next() {
		var sr SeriesRow
		if err := rows.Scan(&sr.TeamID, &sr.Date, &sr.Progress); err != nil {
			return nil, err
		}
		out = append(out, sr)
	}
	return out, rows.Err()
}

func nilIfEmpty(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}
	return ids
}
