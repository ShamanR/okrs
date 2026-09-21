package store

import (
	"database/sql"
	"testing"
)

func settingValue(t *testing.T, db *sql.DB, tenantID int64, key string) (string, bool) {
	t.Helper()
	var v string
	err := db.QueryRow(`SELECT value_json::text FROM tenant_settings WHERE tenant_id = $1 AND key = $2`, tenantID, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", false
	}
	if err != nil {
		t.Fatalf("read %s: %v", key, err)
	}
	return v, true
}

// Миграция 047 переносит четыре порога из health_checkin_config в отдельные ключи,
// отбрасывает настройки удалённой сводки и откатывается обратно без потери порогов.
func TestMigration047SplitsHealthCheckinConfig(t *testing.T) {
	db, cleanup := migrateTo(t, 46)
	defer cleanup()

	if _, err := db.Exec(`INSERT INTO tenants (id, slug, name) OVERRIDING SYSTEM VALUE VALUES (2, 'second', 'Second')`); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	// Tenant 1: all four thresholds plus summary-only fields.
	// Tenant 2: only stale_days set, and a non-numeric green_threshold.
	if _, err := db.Exec(`INSERT INTO tenant_settings (tenant_id, key, value_json) VALUES
		(1, 'health_checkin_config', '{"stale_days":14,"behind_margin":5,"green_threshold":70,"weight_tolerance":3,"comment_depth":2,"in_counter":{"stale":true}}'),
		(2, 'health_checkin_config', '{"stale_days":9,"green_threshold":"x"}')`); err != nil {
		t.Fatalf("seed settings: %v", err)
	}

	migrateDBTo(t, db, 47)

	want := map[string]string{
		"progress_stale_days": "14", "progress_behind_margin": "5",
		"progress_green_threshold": "70", "progress_weight_tolerance": "3",
	}
	for key, v := range want {
		if got, ok := settingValue(t, db, 1, key); !ok || got != v {
			t.Errorf("tenant 1 %s: want %s, got %q (present=%v)", key, v, got, ok)
		}
	}
	if got, ok := settingValue(t, db, 2, "progress_stale_days"); !ok || got != "9" {
		t.Errorf("tenant 2 progress_stale_days: want 9, got %q", got)
	}
	for _, key := range []string{"progress_behind_margin", "progress_green_threshold", "progress_weight_tolerance"} {
		if _, ok := settingValue(t, db, 2, key); ok {
			t.Errorf("tenant 2 %s: unset or non-numeric field must not be migrated", key)
		}
	}
	for _, tenant := range []int64{1, 2} {
		if _, ok := settingValue(t, db, tenant, "health_checkin_config"); ok {
			t.Errorf("tenant %d: health_checkin_config must be removed", tenant)
		}
	}

	// Down restores the four thresholds and drops the progress_* keys.
	migrateDBTo(t, db, 46)

	got, ok := settingValue(t, db, 1, "health_checkin_config")
	if !ok || got != `{"stale_days": 14, "behind_margin": 5, "green_threshold": 70, "weight_tolerance": 3}` {
		t.Errorf("tenant 1 restored config: got %q", got)
	}
	if got, ok := settingValue(t, db, 2, "health_checkin_config"); !ok || got != `{"stale_days": 9}` {
		t.Errorf("tenant 2 restored config: got %q", got)
	}
	if _, ok := settingValue(t, db, 1, "progress_stale_days"); ok {
		t.Error("progress_* keys must be removed on down")
	}

	// Re-applying up is safe.
	migrateDBTo(t, db, 47)
	if got, ok := settingValue(t, db, 1, "progress_green_threshold"); !ok || got != "70" {
		t.Errorf("re-up: progress_green_threshold want 70, got %q", got)
	}
}
