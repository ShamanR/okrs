package service

import (
	"context"
	"encoding/json"
	"math"
	"sort"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/core/progress"
	"okrs/internal/store/progresssnap"
)

// ProgressSnapshotIntervalDaysKey is the tenant_settings (general) key controlling how
// often the progress snapshot job records a point for the period chart, in days (≥1).
const ProgressSnapshotIntervalDaysKey = "progress_snapshot_interval_days"

// LoadProgressSnapshotIntervalDays reads the per-tenant snapshot interval in days,
// defaulting to 1 (daily) when unset or invalid.
func LoadProgressSnapshotIntervalDays(ctx context.Context, scope domain.TenantScope, sr SettingsReader) int {
	raw, err := sr.GetTenant(ctx, scope, ProgressSnapshotIntervalDaysKey)
	if err != nil || raw == nil {
		return 1
	}
	var n int
	if json.Unmarshal(raw, &n) != nil || n < 1 {
		return 1
	}
	return n
}

// ProgressSnapRepo persists and reads daily per-team progress snapshots.
// *progresssnap.Repository satisfies it.
type ProgressSnapRepo interface {
	UpsertSnapshots(ctx context.Context, scope domain.TenantScope, periodID int64, day time.Time, snaps []progresssnap.Snapshot) error
	ListSnapshots(ctx context.Context, scope domain.TenantScope, periodID int64, teamIDs []int64) ([]progresssnap.SeriesRow, error)
}

// SeriesPoint is one dated point on the period progress chart.
type SeriesPoint struct {
	Date     string `json:"date"` // YYYY-MM-DD
	Progress int    `json:"progress"`
}

// ProgressSeries is the period progress-over-time chart payload.
type ProgressSeries struct {
	PeriodStart string        `json:"period_start"` // YYYY-MM-DD
	PeriodEnd   string        `json:"period_end"`
	Points      []SeriesPoint `json:"points"`
}

const dateLayout = "2006-01-02"

// buildProgressSeries aggregates per-team snapshots into one averaged series for the
// selected scope. For each date it averages progress across teams-in-scope that have a
// snapshot that date, then overwrites/inserts the live «today» point (todayAvg).
func buildProgressSeries(rows []progresssnap.SeriesRow, teamFilter map[int64]bool, today string, todayAvg int, start, end time.Time) ProgressSeries {
	type acc struct{ sum, n int }
	byDate := map[string]*acc{}
	for _, r := range rows {
		if teamFilter != nil && !teamFilter[r.TeamID] {
			continue
		}
		key := r.Date.Format(dateLayout)
		a := byDate[key]
		if a == nil {
			a = &acc{}
			byDate[key] = a
		}
		a.sum += r.Progress
		a.n++
	}
	// Live today point overrides any snapshot already stored for today.
	byDate[today] = &acc{sum: todayAvg, n: 1}

	dates := make([]string, 0, len(byDate))
	for d := range byDate {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	points := make([]SeriesPoint, 0, len(dates))
	for _, d := range dates {
		a := byDate[d]
		points = append(points, SeriesPoint{Date: d, Progress: int(math.Round(float64(a.sum) / float64(a.n)))})
	}
	return ProgressSeries{
		PeriodStart: start.Format(dateLayout),
		PeriodEnd:   end.Format(dateLayout),
		Points:      points,
	}
}

// computeTeamSnapshots computes current progress for each active team that has goals.
// Soft-deleted teams, teams without goals, and draft (черновик/forming) teams are
// skipped — the last so the progress chart matches the aggregate progress. Pure — no I/O.
func computeTeamSnapshots(data *PeriodData) []progresssnap.Snapshot {
	out := make([]progresssnap.Snapshot, 0, len(data.Teams))
	for _, team := range data.Teams {
		if team.DeletedAt != nil {
			continue
		}
		src := data.GoalsByTeam[team.ID]
		if len(src) == 0 {
			continue
		}
		// Empty/no_goals status buckets to forming, so drafts are excluded here too.
		if bucketStatusWithGoals(data.Statuses[team.ID]) == "forming" {
			continue
		}
		goals := make([]domain.Goal, len(src))
		copy(goals, src)
		for i := range goals {
			goals[i].Progress = CalculateGoalProgress(&goals[i])
		}
		out = append(out, progresssnap.Snapshot{TeamID: team.ID, Progress: progress.PeriodProgress(goals)})
	}
	return out
}

// SnapshotActivePeriods materialises the given day's per-team progress for each active
// period. Best-effort: a period whose data fails to load is skipped, not fatal.
func (s *Service) SnapshotActivePeriods(ctx context.Context, day time.Time, actives []HCActive) error {
	if s.hcCache == nil || s.progressSnap == nil {
		return nil
	}
	for _, a := range actives {
		if a.PeriodID == 0 {
			continue
		}
		data, err := s.hcCache.Get(ctx, a.Scope, a.PeriodID)
		if err != nil {
			continue
		}
		snaps := computeTeamSnapshots(data)
		if err := s.progressSnap.UpsertSnapshots(ctx, a.Scope, a.PeriodID, day, snaps); err != nil {
			return err
		}
	}
	return nil
}
