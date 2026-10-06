package shares_test

import (
	"context"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/store/shares"
	"okrs/internal/store/testutil"
)

// Нерасшаренная цель видна ровно в одной команде — своей.
func TestTeamIDsByGoalIDsReturnsOwnerForUnsharedGoal(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	goalID := prepareGoal(t, pool, ctx, "owner-only")

	var owner int64
	if err := pool.QueryRow(ctx, `SELECT team_id FROM goals WHERE id=$1`, goalID).Scan(&owner); err != nil {
		t.Fatalf("owner: %v", err)
	}

	got, err := shares.NewGoalShareRepository(pool).TeamIDsByGoalIDs(ctx, sc1, []int64{goalID})
	if err != nil {
		t.Fatalf("TeamIDsByGoalIDs: %v", err)
	}
	if len(got[goalID]) != 1 || got[goalID][0] != owner {
		t.Fatalf("got %v, want [%d]", got[goalID], owner)
	}
}

// Общая цель видна у владельца и у каждого участника, идентификаторы — по
// возрастанию и без дублей.
func TestTeamIDsByGoalIDsReturnsOwnerAndParticipants(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	goalID := prepareGoal(t, pool, ctx, "shared")

	var owner, teamB, teamC int64
	if err := pool.QueryRow(ctx, `SELECT team_id FROM goals WHERE id=$1`, goalID).Scan(&owner); err != nil {
		t.Fatalf("owner: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('ShareB') RETURNING id`).Scan(&teamB); err != nil {
		t.Fatalf("teamB: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('ShareC') RETURNING id`).Scan(&teamC); err != nil {
		t.Fatalf("teamC: %v", err)
	}
	r := shares.NewGoalShareRepository(pool)
	if err := r.ReplaceGoalShares(ctx, sc1, goalID, []shares.GoalShareInput{
		{TeamID: teamB, Weight: 50}, {TeamID: teamC, Weight: 50},
	}); err != nil {
		t.Fatalf("ReplaceGoalShares: %v", err)
	}

	got, err := r.TeamIDsByGoalIDs(ctx, sc1, []int64{goalID})
	if err != nil {
		t.Fatalf("TeamIDsByGoalIDs: %v", err)
	}
	want := sortedInt64s([]int64{owner, teamB, teamC})
	if !sameInt64s(got[goalID], want) {
		t.Fatalf("got %v, want %v (по возрастанию, без дублей)", got[goalID], want)
	}
}

// Несколько целей за один запрос: каждая получает свой набор команд.
func TestTeamIDsByGoalIDsAnswersForWholeBatch(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	first := prepareGoal(t, pool, ctx, "batch-1")
	second := prepareGoal(t, pool, ctx, "batch-2")

	got, err := shares.NewGoalShareRepository(pool).TeamIDsByGoalIDs(ctx, sc1, []int64{first, second})
	if err != nil {
		t.Fatalf("TeamIDsByGoalIDs: %v", err)
	}
	if len(got[first]) != 1 || len(got[second]) != 1 {
		t.Fatalf("каждая цель обязана попасть в ответ: %v", got)
	}
	if got[first][0] == got[second][0] {
		t.Fatalf("у целей разные команды-владельцы, got %v", got)
	}
}

// Цель чужого пространства в ответ не попадает — иначе уведомление ушло бы
// команде другого тенанта.
func TestTeamIDsByGoalIDsIsTenantScoped(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	goalID := prepareGoal(t, pool, ctx, "tenant")

	got, err := shares.NewGoalShareRepository(pool).TeamIDsByGoalIDs(ctx,
		domain.TenantScope{TenantID: 999}, []int64{goalID})
	if err != nil {
		t.Fatalf("TeamIDsByGoalIDs: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("цель другого пространства не должна отдаваться, got %v", got)
	}
}

// Пустой список не должен ходить в БД вовсе.
func TestTeamIDsByGoalIDsEmptyInput(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	got, err := shares.NewGoalShareRepository(pool).TeamIDsByGoalIDs(context.Background(), sc1, nil)
	if err != nil {
		t.Fatalf("TeamIDsByGoalIDs: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v, want пустую карту", got)
	}
}

func sortedInt64s(in []int64) []int64 {
	out := append([]int64(nil), in...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func sameInt64s(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
