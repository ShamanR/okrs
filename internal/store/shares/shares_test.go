package shares_test

import (
	"context"
	"testing"

	"okrs/internal/domain"
	"okrs/internal/store/goals"
	"okrs/internal/store/krs"
	"okrs/internal/store/shares"
	"okrs/internal/store/testutil"

	"github.com/jackc/pgx/v5/pgxpool"
)

func prepareGoal(t *testing.T, pool *pgxpool.Pool, ctx context.Context, suffix string) int64 {
	t.Helper()
	var teamID, periodID int64
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ($1) RETURNING id`, "ShareTeam_"+suffix).Scan(&teamID); err != nil {
		t.Fatalf("insert team %s: %v", suffix, err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO periods (name,start_date,end_date,sort_order) VALUES ($1,'2025-01-01','2025-03-31',1) RETURNING id`, "SP_"+suffix).Scan(&periodID); err != nil {
		t.Fatalf("insert period %s: %v", suffix, err)
	}
	gr := goals.NewGoalRepository(pool, krs.NewKRRepository(pool))
	goalID, err := gr.CreateGoal(ctx, goals.GoalInput{
		TeamID: teamID, PeriodID: periodID,
		Title: "G " + suffix, Priority: domain.PriorityP1, Weight: 100,
	})
	if err != nil {
		t.Fatalf("CreateGoal %s: %v", suffix, err)
	}
	return goalID
}

func TestGoalSharesReplaceAndList(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()

	var sharedTeamID int64
	pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('Shared') RETURNING id`).Scan(&sharedTeamID)
	goalID := prepareGoal(t, pool, ctx, "rpl")

	r := shares.NewGoalShareRepository(pool)

	list, err := r.ListGoalShares(ctx, goalID)
	if err != nil {
		t.Fatalf("ListGoalShares empty: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 shares initially, got %d", len(list))
	}

	if err := r.ReplaceGoalShares(ctx, goalID, []shares.GoalShareInput{{TeamID: sharedTeamID, Weight: 50}}); err != nil {
		t.Fatalf("ReplaceGoalShares add: %v", err)
	}
	list, _ = r.ListGoalShares(ctx, goalID)
	if len(list) != 1 || list[0].TeamID != sharedTeamID {
		t.Fatalf("expected 1 share for sharedTeam, got %+v", list)
	}

	if err := r.ReplaceGoalShares(ctx, goalID, nil); err != nil {
		t.Fatalf("ReplaceGoalShares nil: %v", err)
	}
	list, _ = r.ListGoalShares(ctx, goalID)
	if len(list) != 0 {
		t.Fatalf("expected 0 shares after nil replace, got %d", len(list))
	}
}

func TestGetGoalShare(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()

	var sharedTeamID int64
	pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('GGShared') RETURNING id`).Scan(&sharedTeamID)
	goalID := prepareGoal(t, pool, ctx, "get")

	r := shares.NewGoalShareRepository(pool)
	r.ReplaceGoalShares(ctx, goalID, []shares.GoalShareInput{{TeamID: sharedTeamID, Weight: 30}})

	share, err := r.GetGoalShare(ctx, goalID, sharedTeamID)
	if err != nil {
		t.Fatalf("GetGoalShare: %v", err)
	}
	if share.GoalID != goalID || share.TeamID != sharedTeamID {
		t.Fatalf("unexpected share: %+v", share)
	}
}

func TestDeleteGoalShare(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()

	var sharedTeamID int64
	pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('DelShared') RETURNING id`).Scan(&sharedTeamID)
	goalID := prepareGoal(t, pool, ctx, "del")

	r := shares.NewGoalShareRepository(pool)
	r.ReplaceGoalShares(ctx, goalID, []shares.GoalShareInput{{TeamID: sharedTeamID, Weight: 20}})

	if err := r.DeleteGoalShare(ctx, goalID, sharedTeamID); err != nil {
		t.Fatalf("DeleteGoalShare: %v", err)
	}
	list, _ := r.ListGoalShares(ctx, goalID)
	if len(list) != 0 {
		t.Fatalf("expected 0 shares after delete, got %d", len(list))
	}
}
