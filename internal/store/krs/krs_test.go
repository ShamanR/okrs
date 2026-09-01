package krs_test

import (
	"context"
	"testing"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/store/krs"
	"okrs/internal/store/testutil"
)

func TestUpsertKeyResultNote(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := krs.NewKRRepository(pool)
	scope := domain.TenantScope{TenantID: 1}

	var teamID int64
	pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('T') RETURNING id`).Scan(&teamID)
	var periodID int64
	pool.QueryRow(ctx, `INSERT INTO periods (name, start_date, end_date) VALUES ('Q1', '2024-01-01', '2024-03-31') RETURNING id`).Scan(&periodID)
	var goalID int64
	pool.QueryRow(ctx, `INSERT INTO goals (team_id, period_id, title, priority, weight, work_type, focus_type, sort_order) VALUES ($1,$2,'G','P1',100,'Delivery','STABILITY',1) RETURNING id`, teamID, periodID).Scan(&goalID)
	var userID int64
	pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name) VALUES ('system:test','system','test','Test User') RETURNING id`).Scan(&userID)
	var krID int64
	pool.QueryRow(ctx, `INSERT INTO key_results (goal_id, title, weight, kind, sort_order) VALUES ($1,'KR1',100,'NUMERICAL',1) RETURNING id`, goalID).Scan(&krID)

	// First upsert — creates
	if err := repo.UpsertKeyResultNote(ctx, scope, krID, "first note", userID); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	notes, err := repo.BatchLoadNotes(ctx, scope, []int64{krID})
	if err != nil {
		t.Fatalf("batch load: %v", err)
	}
	note, ok := notes[krID]
	if !ok {
		t.Fatal("note not found after first upsert")
	}
	if note.Text != "first note" {
		t.Fatalf("expected 'first note', got %q", note.Text)
	}
	if note.AuthorName != "Test User" {
		t.Fatalf("expected 'Test User', got %q", note.AuthorName)
	}

	// Second upsert — updates
	if err := repo.UpsertKeyResultNote(ctx, scope, krID, "updated note", userID); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	notes2, _ := repo.BatchLoadNotes(ctx, scope, []int64{krID})
	if notes2[krID].Text != "updated note" {
		t.Fatalf("expected 'updated note', got %q", notes2[krID].Text)
	}
}

func TestUpdateKeyResultDescription(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := krs.NewKRRepository(pool)
	scope := domain.TenantScope{TenantID: 1}

	var teamID int64
	pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('T') RETURNING id`).Scan(&teamID)
	var periodID int64
	pool.QueryRow(ctx, `INSERT INTO periods (name, start_date, end_date) VALUES ('Q1', '2024-01-01', '2024-03-31') RETURNING id`).Scan(&periodID)
	var goalID int64
	pool.QueryRow(ctx, `INSERT INTO goals (team_id, period_id, title, priority, weight, work_type, focus_type, sort_order) VALUES ($1,$2,'G','P1',100,'Delivery','STABILITY',1) RETURNING id`, teamID, periodID).Scan(&goalID)
	var krID int64
	pool.QueryRow(ctx, `INSERT INTO key_results (goal_id, title, weight, kind, sort_order) VALUES ($1,'KR1',100,'NUMERICAL',1) RETURNING id`, goalID).Scan(&krID)

	if err := repo.UpdateKeyResultDescription(ctx, scope, krID, "added context"); err != nil {
		t.Fatalf("update description: %v", err)
	}
	kr, err := repo.GetKeyResult(ctx, scope, krID)
	if err != nil {
		t.Fatalf("get kr: %v", err)
	}
	if kr.Description != "added context" {
		t.Fatalf("expected description 'added context', got %q", kr.Description)
	}
}

func TestBatchLoadNotes_AbsentKR(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := krs.NewKRRepository(pool)
	scope := domain.TenantScope{TenantID: 1}

	notes, err := repo.BatchLoadNotes(ctx, scope, []int64{99999})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := notes[99999]; ok {
		t.Fatal("expected no entry for KR without note")
	}
}

func TestBatchLoadNotes_Empty(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := krs.NewKRRepository(pool)
	scope := domain.TenantScope{TenantID: 1}

	notes, err := repo.BatchLoadNotes(ctx, scope, []int64{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notes) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(notes))
	}
}

func TestUpsertAndLoadNumericalMeta(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := krs.NewKRRepository(pool)
	scope := domain.TenantScope{TenantID: 1}

	var teamID, periodID, goalID, krID int64
	pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('T') RETURNING id`).Scan(&teamID)
	pool.QueryRow(ctx, `INSERT INTO periods (name, start_date, end_date) VALUES ('Q1', '2024-01-01', '2024-03-31') RETURNING id`).Scan(&periodID)
	pool.QueryRow(ctx, `INSERT INTO goals (team_id, period_id, title, priority, weight, work_type, focus_type, sort_order) VALUES ($1,$2,'G','P1',100,'Delivery','STABILITY',1) RETURNING id`, teamID, periodID).Scan(&goalID)
	pool.QueryRow(ctx, `INSERT INTO key_results (goal_id, title, weight, kind, sort_order) VALUES ($1,'KR',100,'NUMERICAL',1) RETURNING id`, goalID).Scan(&krID)

	in := krs.NumericalMetaInput{
		KeyResultID:  krID,
		StartValue:   100,
		TargetValue:  180,
		CurrentValue: 150,
		Unit:         "RPS",
		Checkpoints: []domain.KRNumericalCheckpoint{
			{Value: 100, ProgressPercent: 0},
			{Value: 150, ProgressPercent: 50},
			{Value: 180, ProgressPercent: 100},
		},
	}
	if err := repo.UpsertNumericalMeta(ctx, scope, in); err != nil {
		t.Fatalf("upsert numerical meta: %v", err)
	}

	// Round-trips together with the KR (no separate query needed by the caller).
	krsLoaded, err := repo.ListKeyResultsByGoal(ctx, scope, goalID)
	if err != nil {
		t.Fatalf("list krs: %v", err)
	}
	if len(krsLoaded) != 1 || krsLoaded[0].Numerical == nil {
		t.Fatalf("expected one numerical KR with meta, got %+v", krsLoaded)
	}
	num := krsLoaded[0].Numerical
	if num.StartValue != 100 || num.TargetValue != 180 || num.CurrentValue != 150 {
		t.Fatalf("unexpected values: %+v", num)
	}
	if num.Unit != "RPS" {
		t.Fatalf("unexpected unit: %+v", num)
	}
	if len(num.Checkpoints) != 3 || num.Checkpoints[1].Value != 150 || num.Checkpoints[1].ProgressPercent != 50 {
		t.Fatalf("unexpected checkpoints: %+v", num.Checkpoints)
	}

	// UpdateNumericalCurrent changes only the current value and stamps progress.
	if err := repo.UpdateNumericalCurrent(ctx, scope, krID, 170); err != nil {
		t.Fatalf("update current: %v", err)
	}
	var current float64
	var progressUpdated *time.Time
	if err := pool.QueryRow(ctx, `SELECT current_value, progress_updated_at FROM key_results WHERE id=$1`, krID).Scan(&current, &progressUpdated); err != nil {
		t.Fatalf("scan current: %v", err)
	}
	if current != 170 {
		t.Fatalf("expected current 170, got %v", current)
	}
	if progressUpdated == nil {
		t.Fatalf("expected progress_updated_at to be set")
	}
}

func TestZeroingCriteriaAllKinds(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := krs.NewKRRepository(pool)
	scope := domain.TenantScope{TenantID: 1}

	var teamID, periodID, goalID int64
	pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('T') RETURNING id`).Scan(&teamID)
	pool.QueryRow(ctx, `INSERT INTO periods (name, start_date, end_date) VALUES ('Q1','2024-01-01','2024-03-31') RETURNING id`).Scan(&periodID)
	pool.QueryRow(ctx, `INSERT INTO goals (team_id, period_id, title, priority, weight, work_type, focus_type, sort_order) VALUES ($1,$2,'G','P1',100,'Delivery','STABILITY',1) RETURNING id`, teamID, periodID).Scan(&goalID)

	for _, kind := range []domain.KRKind{domain.KRKindBoolean, domain.KRKindProject} {
		krID, err := repo.CreateKeyResult(ctx, scope, krs.KeyResultInput{
			GoalID:          goalID,
			Title:           "KR " + string(kind),
			ZeroingCriteria: "инцидент P1 = 0%",
			Weight:          10,
			Kind:            kind,
		})
		if err != nil {
			t.Fatalf("create %s: %v", kind, err)
		}
		got, err := repo.GetKeyResult(ctx, scope, krID)
		if err != nil {
			t.Fatalf("get %s: %v", kind, err)
		}
		if got.ZeroingCriteria != "инцидент P1 = 0%" {
			t.Fatalf("kind %s: expected zeroing round-trip, got %q", kind, got.ZeroingCriteria)
		}
	}

	// Update path also persists zeroing.
	krID, _ := repo.CreateKeyResult(ctx, scope, krs.KeyResultInput{GoalID: goalID, Title: "U", Weight: 10, Kind: domain.KRKindBoolean})
	if err := repo.UpdateKeyResult(ctx, scope, krs.KeyResultUpdateInput{ID: krID, Title: "U", ZeroingCriteria: "новый критерий", Weight: 10, Kind: domain.KRKindBoolean}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := repo.GetKeyResult(ctx, scope, krID)
	if got.ZeroingCriteria != "новый критерий" {
		t.Fatalf("expected updated zeroing, got %q", got.ZeroingCriteria)
	}
}

func TestUpdateHealthStatus(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := krs.NewKRRepository(pool)
	scope := domain.TenantScope{TenantID: 1}

	var teamID int64
	pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('T') RETURNING id`).Scan(&teamID)
	var periodID int64
	pool.QueryRow(ctx, `INSERT INTO periods (name, start_date, end_date) VALUES ('Q1', '2024-01-01', '2024-03-31') RETURNING id`).Scan(&periodID)
	var goalID int64
	pool.QueryRow(ctx, `INSERT INTO goals (team_id, period_id, title, priority, weight, work_type, focus_type, sort_order) VALUES ($1,$2,'G','P1',100,'Delivery','STABILITY',1) RETURNING id`, teamID, periodID).Scan(&goalID)
	var krID int64
	pool.QueryRow(ctx, `INSERT INTO key_results (goal_id, title, weight, kind, sort_order, start_value, target_value, current_value, unit) VALUES ($1,'KR1',100,'NUMERICAL',1,0,100,0,'%') RETURNING id`, goalID).Scan(&krID)

	kr, err := repo.GetKeyResult(ctx, scope, krID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if kr.HealthStatus != domain.KRHealthNotStarted {
		t.Fatalf("default = %q, want not_started", kr.HealthStatus)
	}

	if err := repo.UpdateHealthStatus(ctx, scope, krID, domain.KRHealthAtRisk); err != nil {
		t.Fatalf("update: %v", err)
	}

	kr, _ = repo.GetKeyResult(ctx, scope, krID)
	if kr.HealthStatus != domain.KRHealthAtRisk {
		t.Fatalf("after update via GetKeyResult = %q, want at_risk", kr.HealthStatus)
	}

	list, err := repo.ListKeyResultsByGoal(ctx, scope, goalID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].HealthStatus != domain.KRHealthAtRisk {
		t.Fatalf("after update via ListKeyResultsByGoal = %+v, want at_risk", list)
	}
}

// Находка ревью: чек-ин обязан быть атомарным. Раньше прогресс, статус и заметка
// писались тремя отдельно закоммиченными запросами — отказ на второй оставлял
// первую сохранённой, вызывающий получал ошибку, а событие не публиковалось: база
// уезжала, а журнал и уведомления оставались на месте, и заметить это было нечем.
//
// Отказ вызывается настоящим нарушением внешнего ключа: у заметки автор, которого
// нет в users. Он срабатывает ПОСЛЕ записи прогресса внутри той же транзакции —
// ровно та последовательность, о которой находка.
func TestApplyCheckInRollsBackProgressWhenNoteFails(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := krs.NewKRRepository(pool)
	scope := domain.TenantScope{TenantID: 1}

	var teamID, periodID, goalID, krID int64
	pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('T') RETURNING id`).Scan(&teamID)
	pool.QueryRow(ctx, `INSERT INTO periods (name, start_date, end_date) VALUES ('Q1','2024-01-01','2024-03-31') RETURNING id`).Scan(&periodID)
	pool.QueryRow(ctx, `INSERT INTO goals (team_id, period_id, title, priority, weight, work_type, focus_type, sort_order)
	                    VALUES ($1,$2,'G','P1',100,'Delivery','STABILITY',1) RETURNING id`, teamID, periodID).Scan(&goalID)
	pool.QueryRow(ctx, `INSERT INTO key_results (goal_id, title, weight, kind, sort_order, current_value)
	                    VALUES ($1,'KR1',100,'NUMERICAL',1,10) RETURNING id`, goalID).Scan(&krID)

	newValue := 99.0
	missingAuthor := int64(999999) // такого пользователя нет — нарушение внешнего ключа
	err := repo.ApplyCheckIn(ctx, scope, krID, krs.CheckInWrites{
		Numerical: &newValue,
		Note:      &krs.CheckInNote{Text: "заметка", AuthorUserID: missingAuthor},
	})
	if err == nil {
		t.Fatal("ожидалась ошибка: автор заметки не существует")
	}

	var current float64
	if err := pool.QueryRow(ctx,
		`SELECT current_value FROM key_results WHERE id=$1`, krID).Scan(&current); err != nil {
		t.Fatalf("чтение прогресса: %v", err)
	}
	if current != 10 {
		t.Fatalf("прогресс = %v, ожидалось 10: транзакция не откатилась, чек-ин сохранился частично", current)
	}
	var notes int
	pool.QueryRow(ctx, `SELECT count(*) FROM key_result_notes WHERE key_result_id=$1`, krID).Scan(&notes)
	if notes != 0 {
		t.Fatalf("заметок: %d, ожидалось 0", notes)
	}
}
