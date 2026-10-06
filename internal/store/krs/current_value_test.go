package krs_test

import (
	"context"
	"testing"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/store/krs"
	"okrs/internal/store/testutil"
)

// TestNumericalCurrentValueFollowsStartUntilFirstCheckIn pins the behaviour the KR editor
// used to break: creating a numerical KR leaves its current value unset, reads report it as
// the start value, and editing the start before the first check-in moves what is reported.
// After a check-in the value is the KR's own and survives further edits of the definition.
func TestNumericalCurrentValueFollowsStartUntilFirstCheckIn(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := krs.NewKRRepository(pool)
	scope := domain.TenantScope{TenantID: 1}

	var teamID, periodID, goalID int64
	pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('T') RETURNING id`).Scan(&teamID)
	pool.QueryRow(ctx, `INSERT INTO periods (name, start_date, end_date) VALUES ('Q1','2024-01-01','2024-03-31') RETURNING id`).Scan(&periodID)
	pool.QueryRow(ctx, `INSERT INTO goals (team_id, period_id, title, priority, weight, work_type, focus_type, sort_order) VALUES ($1,$2,'G','P1',100,'Delivery','STABILITY',1) RETURNING id`, teamID, periodID).Scan(&goalID)

	krID, err := repo.CreateKeyResult(ctx, scope, krs.KeyResultInput{
		GoalID: goalID, Title: "KR", Weight: 100, Kind: domain.KRKindNumerical,
	})
	if err != nil {
		t.Fatalf("create kr: %v", err)
	}
	// The defect's scenario: start 0, target 200, progress never updated.
	if err := repo.UpsertNumericalMeta(ctx, scope, krs.NumericalMetaInput{
		KeyResultID: krID, StartValue: 0, TargetValue: 200, Unit: "%",
	}); err != nil {
		t.Fatalf("upsert meta: %v", err)
	}

	measure := func() domain.KRNumerical {
		t.Helper()
		kr, err := repo.GetKeyResult(ctx, scope, krID)
		if err != nil {
			t.Fatalf("get kr: %v", err)
		}
		if kr.Numerical == nil {
			t.Fatalf("expected numerical meta, got %+v", kr)
		}
		return *kr.Numerical
	}
	current := func() float64 {
		t.Helper()
		return measure().CurrentValue
	}
	// recorded is the marker readers get instead of the NULL: it is the only trustworthy
	// answer to "was a current value ever written", so it is asserted at every step.
	recorded := func() bool {
		t.Helper()
		return measure().CurrentValueRecorded
	}
	rawCurrentIsNull := func() bool {
		t.Helper()
		var raw *float64
		if err := pool.QueryRow(ctx, `SELECT current_value FROM key_results WHERE id=$1`, krID).Scan(&raw); err != nil {
			t.Fatalf("select current_value: %v", err)
		}
		return raw == nil
	}
	// Read from the column, not through GetKeyResult: that query does not select
	// progress_updated_at (the board queries in store/goals do).
	progressStamped := func() bool {
		t.Helper()
		var stamp *time.Time
		if err := pool.QueryRow(ctx, `SELECT progress_updated_at FROM key_results WHERE id=$1`, krID).Scan(&stamp); err != nil {
			t.Fatalf("select progress_updated_at: %v", err)
		}
		return stamp != nil
	}

	if !rawCurrentIsNull() {
		t.Fatal("creating a KR must not write a current value")
	}
	if got := current(); got != 0 {
		t.Fatalf("expected the start value 0, got %v", got)
	}
	if recorded() {
		t.Fatal("a KR with no check-in must report its current value as not recorded")
	}
	if progressStamped() {
		t.Fatal("creating a KR must not count as updating its progress")
	}

	// Edit the definition: start 0 → 100. This is what used to keep reporting 0.
	if err := repo.UpsertNumericalMeta(ctx, scope, krs.NumericalMetaInput{
		KeyResultID: krID, StartValue: 100, TargetValue: 200, Unit: "%",
	}); err != nil {
		t.Fatalf("upsert meta (edit): %v", err)
	}
	if got := current(); got != 100 {
		t.Fatalf("expected the edited start value 100, got %v", got)
	}
	if recorded() {
		t.Fatal("editing the definition must not make the current value look recorded")
	}
	if !rawCurrentIsNull() || progressStamped() {
		t.Fatal("editing the definition must not write progress")
	}

	// First check-in: the current value becomes the KR's own.
	if err := repo.UpdateNumericalCurrent(ctx, scope, krID, 150); err != nil {
		t.Fatalf("update current: %v", err)
	}
	if got := current(); got != 150 {
		t.Fatalf("expected 150 after the check-in, got %v", got)
	}
	if !recorded() {
		t.Fatal("after a check-in the current value must report as recorded")
	}
	if !progressStamped() {
		t.Fatal("a check-in must stamp progress_updated_at")
	}

	// Editing the definition afterwards leaves the progress where it is.
	if err := repo.UpsertNumericalMeta(ctx, scope, krs.NumericalMetaInput{
		KeyResultID: krID, StartValue: 0, TargetValue: 300, Unit: "шт",
	}); err != nil {
		t.Fatalf("upsert meta (edit after check-in): %v", err)
	}
	if got := current(); got != 150 {
		t.Fatalf("expected the check-in value 150 to survive the edit, got %v", got)
	}
}

// TestRecordedMarkerSurvivesCheckInAtStartValue is the case neither the values nor the
// progress timestamp can express: a check-in that lands exactly on the start value. The KR
// then looks identical to an untouched one — same numbers — and only the recorded marker
// tells them apart. Readers claiming "progress was never updated" rely on it.
func TestRecordedMarkerSurvivesCheckInAtStartValue(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := krs.NewKRRepository(pool)
	scope := domain.TenantScope{TenantID: 1}

	var teamID, periodID, goalID int64
	pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('T') RETURNING id`).Scan(&teamID)
	pool.QueryRow(ctx, `INSERT INTO periods (name, start_date, end_date) VALUES ('Q1','2024-01-01','2024-03-31') RETURNING id`).Scan(&periodID)
	pool.QueryRow(ctx, `INSERT INTO goals (team_id, period_id, title, priority, weight, work_type, focus_type, sort_order) VALUES ($1,$2,'G','P1',100,'Delivery','STABILITY',1) RETURNING id`, teamID, periodID).Scan(&goalID)

	krID, err := repo.CreateKeyResult(ctx, scope, krs.KeyResultInput{
		GoalID: goalID, Title: "KR", Weight: 100, Kind: domain.KRKindNumerical,
	})
	if err != nil {
		t.Fatalf("create kr: %v", err)
	}
	if err := repo.UpsertNumericalMeta(ctx, scope, krs.NumericalMetaInput{
		KeyResultID: krID, StartValue: 100, TargetValue: 200, Unit: "%",
	}); err != nil {
		t.Fatalf("upsert meta: %v", err)
	}
	// "No movement yet" — a real check-in reporting the start value.
	if err := repo.UpdateNumericalCurrent(ctx, scope, krID, 100); err != nil {
		t.Fatalf("update current: %v", err)
	}

	kr, err := repo.GetKeyResult(ctx, scope, krID)
	if err != nil {
		t.Fatalf("get kr: %v", err)
	}
	if kr.Numerical.CurrentValue != 100 {
		t.Fatalf("expected 100, got %v", kr.Numerical.CurrentValue)
	}
	if !kr.Numerical.CurrentValueRecorded {
		t.Fatal("a check-in at the start value must still report the current value as recorded")
	}

	// And the definition edit that follows must leave that check-in alone.
	if err := repo.UpsertNumericalMeta(ctx, scope, krs.NumericalMetaInput{
		KeyResultID: krID, StartValue: 150, TargetValue: 200, Unit: "%",
	}); err != nil {
		t.Fatalf("upsert meta (edit): %v", err)
	}
	kr, err = repo.GetKeyResult(ctx, scope, krID)
	if err != nil {
		t.Fatalf("get kr (after edit): %v", err)
	}
	if kr.Numerical.CurrentValue != 100 || !kr.Numerical.CurrentValueRecorded {
		t.Fatalf("expected the recorded 100 to survive, got %v recorded=%v",
			kr.Numerical.CurrentValue, kr.Numerical.CurrentValueRecorded)
	}
}
