package store

import (
	"testing"
)

// TestMigration050GoalDirection verifies migration 050: every goal gets a direction from
// its focus by the approved table, the column rejects values outside the set, a goal
// inserted without focus gets an empty focus_type (old replicas scan it into a string), and
// the down migration restores a focus for such goals and for goals whose direction was
// changed after the rollout, leaving untouched goals with their original focus.
func TestMigration050GoalDirection(t *testing.T) {
	db, cleanup := migrateTo(t, 49)
	defer cleanup()

	var teamID, periodID int64
	if err := db.QueryRow(`INSERT INTO teams (name, tenant_id) VALUES ('T', 1) RETURNING id`).Scan(&teamID); err != nil {
		t.Fatalf("insert team: %v", err)
	}
	if err := db.QueryRow(`INSERT INTO periods (name, start_date, end_date, tenant_id) VALUES ('Q1','2024-01-01','2024-03-31',1) RETURNING id`).Scan(&periodID); err != nil {
		t.Fatalf("insert period: %v", err)
	}
	insertGoal := func(focus string, order int) int64 {
		t.Helper()
		var id int64
		if err := db.QueryRow(`INSERT INTO goals (team_id, period_id, title, priority, weight, work_type, focus_type, sort_order, tenant_id)
			VALUES ($1,$2,$3,'P1',25,'Delivery',$3,$4,1) RETURNING id`, teamID, periodID, focus, order).Scan(&id); err != nil {
			t.Fatalf("insert goal %s: %v", focus, err)
		}
		return id
	}
	profitability := insertGoal("PROFITABILITY", 1)
	stability := insertGoal("STABILITY", 2)
	speed := insertGoal("SPEED_EFFICIENCY", 3)
	techIndependence := insertGoal("TECH_INDEPENDENCE", 4)
	want := map[int64]string{
		profitability:    "PRODUCT",
		stability:        "TECH",
		speed:            "PROCESS",
		techIndependence: "TECH",
	}

	migrateDBTo(t, db, 50)

	for id, dir := range want {
		var got string
		if err := db.QueryRow(`SELECT direction FROM goals WHERE id = $1`, id).Scan(&got); err != nil {
			t.Fatalf("select goal %d: %v", id, err)
		}
		if got != dir {
			t.Errorf("goal %d: direction = %q, want %q", id, got, dir)
		}
	}

	if _, err := db.Exec(`INSERT INTO goals (team_id, period_id, title, priority, weight, work_type, direction, sort_order, tenant_id)
		VALUES ($1,$2,'bad','P1',0,'Delivery','STABILITY',5,1)`, teamID, periodID); err == nil {
		t.Fatal("direction outside the set must be rejected by the CHECK constraint")
	}

	var newID int64
	var focus string
	if err := db.QueryRow(`INSERT INTO goals (team_id, period_id, title, priority, weight, work_type, direction, sort_order, tenant_id)
		VALUES ($1,$2,'new','P1',0,'Delivery','PROCESS',6,1) RETURNING id, focus_type`, teamID, periodID).Scan(&newID, &focus); err != nil {
		t.Fatalf("insert goal without focus: %v", err)
	}
	if focus != "" {
		t.Errorf("focus_type of a goal created on directions = %q, want empty", focus)
	}

	// After the rollout a user changes the direction of a migrated goal. A rollback must keep
	// that edit even though the goal still has its legacy focus.
	if _, err := db.Exec(`UPDATE goals SET direction = 'PROCESS' WHERE id = $1`, stability); err != nil {
		t.Fatalf("edit direction: %v", err)
	}

	migrateDBTo(t, db, 49)

	if err := db.QueryRow(`SELECT focus_type FROM goals WHERE id = $1`, newID).Scan(&focus); err != nil {
		t.Fatalf("select after down: %v", err)
	}
	if focus != "SPEED_EFFICIENCY" {
		t.Errorf("after down: focus_type = %q, want SPEED_EFFICIENCY", focus)
	}
	// The edited goal gets its focus back from the new direction; untouched goals keep their
	// original focus, including Tech Independence, which shares TECH with Stability.
	for id, wantFocus := range map[int64]string{
		stability:        "SPEED_EFFICIENCY",
		techIndependence: "TECH_INDEPENDENCE",
		profitability:    "PROFITABILITY",
		speed:            "SPEED_EFFICIENCY",
	} {
		var got string
		if err := db.QueryRow(`SELECT focus_type FROM goals WHERE id = $1`, id).Scan(&got); err != nil {
			t.Fatalf("select goal %d after down: %v", id, err)
		}
		if got != wantFocus {
			t.Errorf("goal %d after down: focus_type = %q, want %q", id, got, wantFocus)
		}
	}
	var stillHasDirection bool
	if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'goals' AND column_name = 'direction')`).Scan(&stillHasDirection); err != nil {
		t.Fatalf("columns: %v", err)
	}
	if stillHasDirection {
		t.Error("down migration must drop goals.direction")
	}
}
