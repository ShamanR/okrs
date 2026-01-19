package store

import (
	"context"
	"fmt"

	"okrs/internal/domain"
)

func (s *Store) SeedDemo(ctx context.Context, year, quarter int) error {
	teams := []string{"Platform", "Payments", "Growth"}
	teamIDs := make([]int64, 0, len(teams))
	for _, name := range teams {
		var id int64
		err := s.DB.QueryRow(ctx, `INSERT INTO teams (name) VALUES ($1) ON CONFLICT (name) DO UPDATE SET name=EXCLUDED.name RETURNING id`, name).Scan(&id)
		if err != nil {
			return err
		}
		teamIDs = append(teamIDs, id)
	}

	for _, teamID := range teamIDs {
		goalID, err := s.CreateGoal(ctx, GoalInput{
			TeamID:      teamID,
			Year:        year,
			Quarter:     quarter,
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
		krID, err := s.CreateKeyResult(ctx, KeyResultInput{
			GoalID:      goalID,
			Title:       "Incident reduction project",
			Description: "Deliver reliability initiatives.",
			Weight:      100,
			Kind:        domain.KRKindProject,
		})
		if err != nil {
			return err
		}
		_ = s.AddProjectStage(ctx, ProjectStageInput{KeyResultID: krID, Title: "Audit", Weight: 40, SortOrder: 1, IsDone: true})
		_ = s.AddProjectStage(ctx, ProjectStageInput{KeyResultID: krID, Title: "Remediations", Weight: 60, SortOrder: 2, IsDone: false})

		goalID2, err := s.CreateGoal(ctx, GoalInput{
			TeamID:      teamID,
			Year:        year,
			Quarter:     quarter,
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
		krID2, err := s.CreateKeyResult(ctx, KeyResultInput{
			GoalID:      goalID2,
			Title:       "MAU growth",
			Description: "Increase monthly active usage.",
			Weight:      100,
			Kind:        domain.KRKindPercent,
		})
		if err != nil {
			return err
		}
		_ = s.UpsertPercentMeta(ctx, PercentMetaInput{KeyResultID: krID2, StartValue: 1000, TargetValue: 1500, CurrentValue: 1200})
	}

	return nil
}
