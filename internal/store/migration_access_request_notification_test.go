package store

import "testing"

// Миграция 048 допускает тип «заявка на доступ» в обеих таблицах уведомлений,
// а откат удаляет строки этого типа — иначе прежнее ограничение не восстановить.
func TestMigration048AllowsAccessRequestedAndDownRemovesIt(t *testing.T) {
	db, cleanup := migrateTo(t, 47)
	defer cleanup()

	insertNotif := `
		INSERT INTO notifications (tenant_id, user_id, type, kind, actor_user_id, coalesce_key)
		VALUES (1, 1, 'access_requested', 'access_requested', 1, 'k')`
	if _, err := db.Exec(insertNotif); err == nil {
		t.Fatal("до 048 тип access_requested должен отклоняться ограничением")
	}

	migrateDBTo(t, db, 48)

	if _, err := db.Exec(insertNotif); err != nil {
		t.Fatalf("после 048 уведомление access_requested: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO notification_preferences (tenant_id, user_id, type, enabled)
		VALUES (1, 1, 'access_requested', TRUE),
		       (1, 1, 'goal_changed', FALSE)`); err != nil {
		t.Fatalf("после 048 настройка access_requested: %v", err)
	}

	migrateDBTo(t, db, 47)

	var n int
	if err := db.QueryRow(`SELECT count(*) FROM notifications WHERE type = 'access_requested'`).Scan(&n); err != nil {
		t.Fatalf("count notifications: %v", err)
	}
	if n != 0 {
		t.Errorf("после отката осталось %d уведомлений access_requested", n)
	}
	if err := db.QueryRow(`SELECT count(*) FROM notification_preferences`).Scan(&n); err != nil {
		t.Fatalf("count prefs: %v", err)
	}
	if n != 1 {
		t.Errorf("после отката настроек %d, want 1 (строки прочих типов не трогаются)", n)
	}
	if _, err := db.Exec(insertNotif); err == nil {
		t.Error("после отката тип access_requested снова должен отклоняться")
	}
}
