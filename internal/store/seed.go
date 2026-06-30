package store

import (
	"context"
	"errors"
	"fmt"

	"okrs/internal/domain"
	"okrs/internal/store/goals"
	"okrs/internal/store/krs"

	"github.com/jackc/pgx/v5"
)

func seedDemo(ctx context.Context, goalsRepo *goals.GoalRepository, krsRepo *krs.KRRepository, periodID int64) error {
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

	for _, teamID := range teamIDs {
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
	}

	return nil
}
