package store

import (
	"testing"
)

// TestMigration049NullsUntouchedCurrentValue verifies migration 049: the current value of a
// numerical KR whose progress was never updated becomes NULL, so that a later edit of its
// start value moves the value reads report. The predicate must leave everything else alone —
// most importantly a KR whose progress WAS updated before progress_updated_at existed
// (migration 012), which looks un-updated but holds a real value.
func TestMigration049NullsUntouchedCurrentValue(t *testing.T) {
	db, cleanup := migrateTo(t, 48)
	defer cleanup()

	var teamID, periodID, goalID int64
	if err := db.QueryRow(`INSERT INTO teams (name, tenant_id) VALUES ('T', 1) RETURNING id`).Scan(&teamID); err != nil {
		t.Fatalf("insert team: %v", err)
	}
	if err := db.QueryRow(`INSERT INTO periods (name, start_date, end_date, tenant_id) VALUES ('Q1','2024-01-01','2024-03-31',1) RETURNING id`).Scan(&periodID); err != nil {
		t.Fatalf("insert period: %v", err)
	}
	if err := db.QueryRow(`INSERT INTO goals (team_id, period_id, title, priority, weight, work_type, focus_type, sort_order, tenant_id) VALUES ($1,$2,'G','P1',100,'Delivery','STABILITY',1,1) RETURNING id`, teamID, periodID).Scan(&goalID); err != nil {
		t.Fatalf("insert goal: %v", err)
	}

	// The predicate only converts rows created after the earliest known progress
	// timestamp, so the fixture needs control over created_at as well: a legacy row is one
	// that predates every stamp in the table.
	insertKR := func(title, kind string, order int, start, target, current float64, progress, createdAt string) int64 {
		t.Helper()
		var id int64
		if err := db.QueryRow(`INSERT INTO key_results
			(goal_id, title, weight, kind, sort_order, start_value, target_value, current_value, unit, progress_updated_at, created_at, tenant_id)
			VALUES ($1,$2,25,$3,$4,$5,$6,$7,'%',`+progress+`,`+createdAt+`,1) RETURNING id`,
			goalID, title, kind, order, start, target, current).Scan(&id); err != nil {
			t.Fatalf("insert kr %s: %v", title, err)
		}
		return id
	}

	// A real check-in from a month ago. It is also the earliest stamp in the table, which
	// is what the predicate uses as the moment migration 012 is known to have existed.
	checkedIn := insertKR("checked-in", "NUMERICAL", 1, 0, 100, 0, "NOW() - INTERVAL '30 days'", "NOW() - INTERVAL '60 days'")
	// Never updated and standing at its start, created after that stamp: the row this
	// migration is for.
	untouched := insertKR("untouched", "NUMERICAL", 2, 0, 100, 0, "NULL", "NOW() - INTERVAL '10 days'")
	// Never stamped, holding a value that differs from the start — progress updated before
	// migration 012 added the stamp. Must survive.
	unstamped := insertKR("unstamped", "NUMERICAL", 3, 0, 100, 50, "NULL", "NOW() - INTERVAL '10 days'")
	// The ambiguous legacy row: that same pre-012 check-in, but it landed exactly on the
	// start value, so values alone cannot tell it from an untouched KR. It predates every
	// stamp in the table, and the migration must leave it alone rather than guess.
	legacyAtStart := insertKR("legacy-at-start", "NUMERICAL", 4, 0, 100, 0, "NULL", "NOW() - INTERVAL '90 days'")
	// Not a numerical KR: the kind filter must skip it even though the values match.
	boolean := insertKR("boolean", "BOOLEAN", 5, 0, 100, 0, "NULL", "NOW() - INTERVAL '10 days'")

	migrateDBTo(t, db, 49)

	assertCurrent := func(krID int64, want *float64) {
		t.Helper()
		var got *float64
		if err := db.QueryRow(`SELECT current_value FROM key_results WHERE id=$1`, krID).Scan(&got); err != nil {
			t.Fatalf("select current_value: %v", err)
		}
		switch {
		case want == nil && got != nil:
			t.Fatalf("kr %d: expected NULL current_value, got %v", krID, *got)
		case want != nil && got == nil:
			t.Fatalf("kr %d: expected %v, got NULL", krID, *want)
		case want != nil && *want != *got:
			t.Fatalf("kr %d: expected %v, got %v", krID, *want, *got)
		}
	}
	val := func(f float64) *float64 { return &f }

	assertCurrent(untouched, nil)
	assertCurrent(unstamped, val(50))
	assertCurrent(checkedIn, val(0))
	assertCurrent(legacyAtStart, val(0))
	assertCurrent(boolean, val(0))

	// Down restores the previous representation: a numerical KR always had a current value,
	// and for an un-updated one it equalled the start value.
	migrateDBTo(t, db, 48)
	assertCurrent(untouched, val(0))
	assertCurrent(unstamped, val(50))
	assertCurrent(checkedIn, val(0))
	assertCurrent(legacyAtStart, val(0))
	assertCurrent(boolean, val(0))
}

// TestMigration049DoesNothingWithoutAnyProgressStamp pins the fallback: when the table
// holds no progress timestamp at all, nothing can be proven about any row and the
// migration must leave every one of them untouched.
func TestMigration049DoesNothingWithoutAnyProgressStamp(t *testing.T) {
	db, cleanup := migrateTo(t, 48)
	defer cleanup()

	var teamID, periodID, goalID int64
	if err := db.QueryRow(`INSERT INTO teams (name, tenant_id) VALUES ('T', 1) RETURNING id`).Scan(&teamID); err != nil {
		t.Fatalf("insert team: %v", err)
	}
	if err := db.QueryRow(`INSERT INTO periods (name, start_date, end_date, tenant_id) VALUES ('Q1','2024-01-01','2024-03-31',1) RETURNING id`).Scan(&periodID); err != nil {
		t.Fatalf("insert period: %v", err)
	}
	if err := db.QueryRow(`INSERT INTO goals (team_id, period_id, title, priority, weight, work_type, focus_type, sort_order, tenant_id) VALUES ($1,$2,'G','P1',100,'Delivery','STABILITY',1,1) RETURNING id`, teamID, periodID).Scan(&goalID); err != nil {
		t.Fatalf("insert goal: %v", err)
	}
	var krID int64
	if err := db.QueryRow(`INSERT INTO key_results
		(goal_id, title, weight, kind, sort_order, start_value, target_value, current_value, unit, tenant_id)
		VALUES ($1,'lonely',25,'NUMERICAL',1,0,100,0,'%',1) RETURNING id`, goalID).Scan(&krID); err != nil {
		t.Fatalf("insert kr: %v", err)
	}

	migrateDBTo(t, db, 49)

	var got *float64
	if err := db.QueryRow(`SELECT current_value FROM key_results WHERE id=$1`, krID).Scan(&got); err != nil {
		t.Fatalf("select current_value: %v", err)
	}
	if got == nil || *got != 0 {
		t.Fatalf("expected the row to be left at 0, got %v", got)
	}
}
