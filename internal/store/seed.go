package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"okrs/internal/domain"
	"okrs/internal/store/goals"
	"okrs/internal/store/krs"
	"okrs/internal/store/progresssnap"

	"github.com/jackc/pgx/v5"
)

func seedDemo(ctx context.Context, goalsRepo *goals.GoalRepository, krsRepo *krs.KRRepository, snapRepo *progresssnap.Repository, periodID int64) error {
	scope := domain.TenantScope{TenantID: 1} // TODO(tenancy): seed supports single-tenant only
	teamNames := []string{"Platform", "Payments", "Growth"}
	teamIDs := make([]int64, 0, len(teamNames))
	for _, name := range teamNames {
		// Team names are not unique (no constraint to use as an ON CONFLICT
		// arbiter), so make seeding idempotent by reusing an existing active
		// team with this name and inserting only when none exists.
		var id int64
		err := goalsRepo.DB().QueryRow(ctx,
			`SELECT id FROM teams WHERE name=$1 AND deleted_at IS NULL AND tenant_id=$2 ORDER BY id LIMIT 1`, name, scope.TenantID).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			// tenant_id is explicit: migration 032 dropped the transitional DEFAULT 1, so the
			// seed must set it (single-tenant seed → default tenant #1).
			err = goalsRepo.DB().QueryRow(ctx,
				`INSERT INTO teams (name, team_type, tenant_id) VALUES ($1,$2,$3) RETURNING id`, name, domain.TeamTypeTeam, scope.TenantID).Scan(&id)
		}
		if err != nil {
			return err
		}
		teamIDs = append(teamIDs, id)
	}

	// Collected per team to seed demo goal links after all goals exist.
	reliabilityIDs := make([]int64, 0, len(teamIDs))
	var platformAdoptionID int64
	for i, teamID := range teamIDs {
		goalID, err := goalsRepo.CreateGoal(ctx, scope, goals.GoalInput{
			TeamID:      teamID,
			PeriodID:    periodID,
			Title:       fmt.Sprintf("Improve reliability for team %d", teamID),
			Description: "Reduce incidents and improve on-call experience.",
			Priority:    domain.PriorityP1,
			Weight:      60,
			WorkType:    domain.WorkTypeDelivery,
			FocusType:   domain.FocusStability,
			OwnerText:   "Engineering Lead",
		})
		if err != nil {
			return err
		}
		krID, err := krsRepo.CreateKeyResult(ctx, scope, krs.KeyResultInput{
			GoalID:      goalID,
			Title:       "Incident reduction project",
			Description: "Deliver reliability initiatives.",
			Weight:      100,
			Kind:        domain.KRKindProject,
		})
		if err != nil {
			return err
		}
		_ = krsRepo.AddProjectStage(ctx, scope, krs.ProjectStageInput{KeyResultID: krID, Title: "Audit", Weight: 40, SortOrder: 1, IsDone: true})
		_ = krsRepo.AddProjectStage(ctx, scope, krs.ProjectStageInput{KeyResultID: krID, Title: "Remediations", Weight: 60, SortOrder: 2, IsDone: false})
		_ = krsRepo.UpdateHealthStatus(ctx, scope, krID, domain.KRHealthAtRisk)
		reliabilityIDs = append(reliabilityIDs, goalID)

		goalID2, err := goalsRepo.CreateGoal(ctx, scope, goals.GoalInput{
			TeamID:      teamID,
			PeriodID:    periodID,
			Title:       fmt.Sprintf("Grow adoption for team %d", teamID),
			Description: "Ship features that increase engagement.",
			Priority:    domain.PriorityP2,
			Weight:      40,
			WorkType:    domain.WorkTypeDiscovery,
			FocusType:   domain.FocusSpeedEfficiency,
			OwnerText:   "Product Manager",
		})
		if err != nil {
			return err
		}
		krID2, err := krsRepo.CreateKeyResult(ctx, scope, krs.KeyResultInput{
			GoalID:      goalID2,
			Title:       "MAU growth",
			Description: "Increase monthly active usage.",
			Weight:      100,
			Kind:        domain.KRKindNumerical,
		})
		if err != nil {
			return err
		}
		_ = krsRepo.UpsertNumericalMeta(ctx, scope, krs.NumericalMetaInput{KeyResultID: krID2, StartValue: 1000, TargetValue: 1500, CurrentValue: 1200, Unit: "пользователей"})
		_ = krsRepo.UpdateHealthStatus(ctx, scope, krID2, domain.KRHealthOnTrack)
		if i == 0 {
			platformAdoptionID = goalID2
		}
	}

	// Demo goal links: the first team's reliability goal is the parent; the other teams'
	// reliability goals and the first team's adoption goal are children (shows ↑/↓ labels).
	// The link row itself is idempotent across re-seeds via ON CONFLICT DO NOTHING; the
	// linked goals/KRs are not — they follow the existing seed idiom above (no existence
	// check), so a second --seed on the same tenant duplicates them, same as the
	// pre-existing reliability/adoption goals.
	if len(reliabilityIDs) > 0 {
		parentGoalID := reliabilityIDs[0]
		children := append([]int64{}, reliabilityIDs[1:]...)
		if platformAdoptionID != 0 {
			children = append(children, platformAdoptionID)
		}
		for _, childID := range children {
			if _, err := goalsRepo.DB().Exec(ctx, `
				INSERT INTO goal_links (tenant_id, child_goal_id, parent_goal_id)
				VALUES ($1,$2,$3)
				ON CONFLICT DO NOTHING`, scope.TenantID, childID, parentGoalID); err != nil {
				return err
			}
		}

		// The Goal Tree page (specs/060-goal-tree.md) bands goals by period depth
		// (year above quarter). Everything above lives in the single seeded quarter
		// period, so without this the demo tree would render as one flat band with
		// no cross-band edges. Wrap that quarter in its enclosing calendar year (an
		// annual period, created here if it doesn't exist yet) and add one more
		// annual-level goal that the quarter's reliability goal decomposes from, so
		// the demo shows a real two-band tree (annual parent -> quarterly child).
		if annualGoalID, err := seedAnnualParentGoal(ctx, goalsRepo, krsRepo, scope, teamIDs[0], periodID); err != nil {
			return err
		} else if annualGoalID != 0 {
			if _, err := goalsRepo.DB().Exec(ctx, `
				INSERT INTO goal_links (tenant_id, child_goal_id, parent_goal_id)
				VALUES ($1,$2,$3)
				ON CONFLICT DO NOTHING`, scope.TenantID, parentGoalID, annualGoalID); err != nil {
				return err
			}
		}
	}

	// Seeded teams are «в работе», so they count toward aggregate progress and the
	// chart (draft/forming teams are excluded from progress calculations).
	for _, teamID := range teamIDs {
		if _, err := goalsRepo.DB().Exec(ctx, `
			INSERT INTO team_period_statuses (team_id, period_id, status, tenant_id, updated_at)
			VALUES ($1,$2,$3,$4,NOW())
			ON CONFLICT (team_id, period_id) DO UPDATE SET status=EXCLUDED.status, updated_at=NOW()`,
			teamID, periodID, domain.TeamPeriodStatusInProgress, scope.TenantID); err != nil {
			return err
		}
	}

	// Demo progress snapshots so the period progress chart is non-empty. Ascending
	// points across the period; upsert keeps this idempotent across re-seeds.
	if snapRepo != nil {
		var start time.Time
		if err := goalsRepo.DB().QueryRow(ctx,
			`SELECT start_date FROM periods WHERE id=$1 AND tenant_id=$2`, periodID, scope.TenantID).Scan(&start); err == nil {
			points := []struct {
				d time.Time
				p int
			}{
				{start.AddDate(0, 0, 5), 5},
				{start.AddDate(0, 1, 0), 15},
				{start.AddDate(0, 2, 0), 30},
			}
			for _, teamID := range teamIDs {
				for _, pt := range points {
					_ = snapRepo.UpsertSnapshots(ctx, scope, periodID, pt.d, []progresssnap.Snapshot{{TeamID: teamID, Progress: pt.p}})
				}
			}
		}
	}

	return nil
}

// seedAnnualParentGoal ensures a demo goal exists in the calendar year that
// encloses periodID, so the Goal Tree page has a real annual band above the
// seeded quarter band. It returns 0 (no error) if periodID's period is not
// meaningfully narrower than a full year (e.g. a pre-existing annual period
// was passed in), since wrapping it would create two same-range periods that
// don't nest and the cross-band link would not make sense.
func seedAnnualParentGoal(ctx context.Context, goalsRepo *goals.GoalRepository, krsRepo *krs.KRRepository, scope domain.TenantScope, teamID int64, periodID int64) (int64, error) {
	var qStart, qEnd time.Time
	if err := goalsRepo.DB().QueryRow(ctx,
		`SELECT start_date, end_date FROM periods WHERE id=$1 AND tenant_id=$2`, periodID, scope.TenantID).Scan(&qStart, &qEnd); err != nil {
		return 0, err
	}
	// ~300 days: periods shorter than ~10 months are treated as sub-annual and get an
	// annual wrapper; a period already near a full year is assumed pre-existing annual
	// and skipped.
	if qEnd.Sub(qStart) >= 300*24*time.Hour {
		return 0, nil
	}

	year := qStart.Year()
	yearStart := time.Date(year, time.January, 1, 0, 0, 0, 0, qStart.Location())
	yearEnd := time.Date(year, time.December, 31, 0, 0, 0, 0, qStart.Location())

	var annualPeriodID int64
	if err := goalsRepo.DB().QueryRow(ctx, `
		INSERT INTO periods (name, start_date, end_date, tenant_id)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (tenant_id, name) DO UPDATE SET start_date = EXCLUDED.start_date
		RETURNING id`, fmt.Sprintf("%d", year), yearStart, yearEnd, scope.TenantID).Scan(&annualPeriodID); err != nil {
		return 0, err
	}

	annualGoalID, err := goalsRepo.CreateGoal(ctx, scope, goals.GoalInput{
		TeamID:      teamID,
		PeriodID:    annualPeriodID,
		Title:       fmt.Sprintf("%d annual reliability commitment", year),
		Description: "Company-level yearly reliability target that the quarterly plan decomposes into.",
		Priority:    domain.PriorityP0,
		Weight:      100,
		WorkType:    domain.WorkTypeDelivery,
		FocusType:   domain.FocusStability,
		OwnerText:   "Engineering Lead",
	})
	if err != nil {
		return 0, err
	}
	annualKRID, err := krsRepo.CreateKeyResult(ctx, scope, krs.KeyResultInput{
		GoalID:      annualGoalID,
		Title:       "Reliability program coverage",
		Description: "Share of teams delivering their quarterly reliability initiatives.",
		Weight:      100,
		Kind:        domain.KRKindNumerical,
	})
	if err != nil {
		return 0, err
	}
	_ = krsRepo.UpsertNumericalMeta(ctx, scope, krs.NumericalMetaInput{KeyResultID: annualKRID, StartValue: 0, TargetValue: 100, CurrentValue: 35, Unit: "%"})

	return annualGoalID, nil
}
