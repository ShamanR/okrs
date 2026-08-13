package store

import (
	"testing"
)

// TestMigrationKRHealth verifies migration 042 adds the health_status column with a
// not_started default and backfills 'done' for KRs already at 100% progress (per kind).
func TestMigrationKRHealth(t *testing.T) {
	db, cleanup := migrateTo(t, 41)
	defer cleanup()

	// Seed a team/period/goal in the default tenant (1). tenant_id has no default at v41.
	var teamID, periodID, goalID int64
	if err := db.QueryRow(`INSERT INTO teams (name, tenant_id) VALUES ('T', 1) RETURNING id`).Scan(&teamID); err != nil {
		t.Fatalf("insert team: %v", err)
	}
	if err := db.QueryRow(`INSERT INTO periods (name, start_date, end_date, tenant_id) VALUES ('Q1', '2024-01-01', '2024-03-31', 1) RETURNING id`).Scan(&periodID); err != nil {
		t.Fatalf("insert period: %v", err)
	}
	if err := db.QueryRow(`INSERT INTO goals (team_id, period_id, title, priority, weight, work_type, focus_type, sort_order, tenant_id) VALUES ($1,$2,'G','P1',100,'Delivery','STABILITY',1,1) RETURNING id`, teamID, periodID).Scan(&goalID); err != nil {
		t.Fatalf("insert goal: %v", err)
	}

	// BOOLEAN done -> expect done.
	var boolKR int64
	if err := db.QueryRow(`INSERT INTO key_results (goal_id, title, weight, kind, sort_order, tenant_id) VALUES ($1,'bool',25,'BOOLEAN',1,1) RETURNING id`, goalID).Scan(&boolKR); err != nil {
		t.Fatalf("insert bool kr: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO kr_boolean_meta (key_result_id, is_done) VALUES ($1, true)`, boolKR); err != nil {
		t.Fatalf("insert bool meta: %v", err)
	}

	// PROJECT with done-stage weights summing to 100 -> expect done.
	var projKR int64
	if err := db.QueryRow(`INSERT INTO key_results (goal_id, title, weight, kind, sort_order, tenant_id) VALUES ($1,'proj',25,'PROJECT',2,1) RETURNING id`, goalID).Scan(&projKR); err != nil {
		t.Fatalf("insert proj kr: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO kr_project_stages (key_result_id, title, weight, is_done, sort_order) VALUES ($1,'s1',60,true,1),($1,'s2',40,true,2)`, projKR); err != nil {
		t.Fatalf("insert stages: %v", err)
	}

	// NUMERICAL increasing, current reached target -> expect done.
	var numDoneKR int64
	if err := db.QueryRow(`INSERT INTO key_results (goal_id, title, weight, kind, sort_order, start_value, target_value, current_value, unit, tenant_id) VALUES ($1,'numdone',25,'NUMERICAL',3,0,100,100,'%',1) RETURNING id`, goalID).Scan(&numDoneKR); err != nil {
		t.Fatalf("insert num-done kr: %v", err)
	}

	// NUMERICAL increasing, still below target -> expect not_started.
	var numBelowKR int64
	if err := db.QueryRow(`INSERT INTO key_results (goal_id, title, weight, kind, sort_order, start_value, target_value, current_value, unit, tenant_id) VALUES ($1,'numbelow',25,'NUMERICAL',4,0,100,50,'%',1) RETURNING id`, goalID).Scan(&numBelowKR); err != nil {
		t.Fatalf("insert num-below kr: %v", err)
	}

	// Apply migration 042.
	migrateDBTo(t, db, 42)

	assertHealth := func(krID int64, want string) {
		t.Helper()
		var got string
		if err := db.QueryRow(`SELECT health_status FROM key_results WHERE id=$1`, krID).Scan(&got); err != nil {
			t.Fatalf("scan health for kr %d: %v", krID, err)
		}
		if got != want {
			t.Fatalf("kr %d health_status = %q, want %q", krID, got, want)
		}
	}

	assertHealth(boolKR, "done")
	assertHealth(projKR, "done")
	assertHealth(numDoneKR, "done")
	assertHealth(numBelowKR, "not_started")
}
