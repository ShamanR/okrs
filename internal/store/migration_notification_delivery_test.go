package store

import "testing"

// Миграция 046 меняет смысл хранимого выбора пользователя: список включённых
// каналов становится картой ОТКЛОНЕНИЙ от значения, заданного администратором.
// Разница не косметическая — список не отличал «канал выключен» от «канала тогда
// не существовало», и ровно на этом различии стоит подключение нового канала.
// Поэтому переносу нужен тест на данных, а не только факт того, что миграция
// накатывается.
func TestMigration046ConvertsChannelListToOverrides(t *testing.T) {
	db, cleanup := migrateTo(t, 45) // до перехода
	defer cleanup()

	// Строки в прежней форме: у одного пользователя колокольчик в списке,
	// у другого список пуст — он его выключил.
	if _, err := db.Exec(`
		INSERT INTO notification_preferences (tenant_id, user_id, type, enabled, scope, channels)
		VALUES (1, 1, 'goal_changed', TRUE,  'subtree', '{in_app}'),
		       (1, 2, 'kr_progress',  TRUE,  'own',     '{}'),
		       (1, 1, 'goal_comment', FALSE, 'own',     '{in_app}')`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	migrateDBTo(t, db, 46)

	cases := []struct {
		userID int64
		typ    string
		want   string
	}{
		// Строка существует только у того, кто настройки сохранял, поэтому его
		// выбор по колокольчику явный — в обе стороны.
		{1, "goal_changed", "true"},
		{2, "kr_progress", "false"},
		{1, "goal_comment", "true"},
	}
	for _, c := range cases {
		var got string
		err := db.QueryRow(`
			SELECT channel_overrides->>'in_app'
			  FROM notification_preferences
			 WHERE tenant_id = 1 AND user_id = $1 AND type = $2`, c.userID, c.typ).Scan(&got)
		if err != nil {
			t.Fatalf("user %d / %s: %v", c.userID, c.typ, err)
		}
		if got != c.want {
			t.Errorf("user %d / %s: in_app = %q, want %q", c.userID, c.typ, got, c.want)
		}
	}

	// Остальные поля строки перенос не трогает.
	var enabled bool
	var scope string
	if err := db.QueryRow(`
		SELECT enabled, scope FROM notification_preferences
		 WHERE tenant_id = 1 AND user_id = 1 AND type = 'goal_comment'`).Scan(&enabled, &scope); err != nil {
		t.Fatalf("остальные поля: %v", err)
	}
	if enabled || scope != "own" {
		t.Errorf("перенос задел соседние поля: enabled=%v scope=%q", enabled, scope)
	}

	// Прежняя колонка исчезла: два источника об одном факте — это два шанса
	// прочитать их по-разному.
	var n int
	if err := db.QueryRow(`
		SELECT count(*) FROM information_schema.columns
		 WHERE table_name = 'notification_preferences' AND column_name = 'channels'`).Scan(&n); err != nil {
		t.Fatalf("схема: %v", err)
	}
	if n != 0 {
		t.Error("колонка channels обязана быть удалена вместе с переносом")
	}
}

// Пользователь, никогда не открывавший настройки, строки не имеет — и не должен
// её получить: предзаполнение сделало бы его выбор явным и отрезало бы от
// будущих изменений значения по умолчанию.
func TestMigration046DoesNotBackfillAbsentRows(t *testing.T) {
	db, cleanup := migrateTo(t, 45)
	defer cleanup()

	migrateDBTo(t, db, 46)

	var n int
	if err := db.QueryRow(`SELECT count(*) FROM notification_preferences`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("миграция предзаполнила %d строк, ожидалось 0", n)
	}
}

// Существующие каналы после наката выключены «по умолчанию у всех»: выкатка сама
// по себе не должна начать писать людям в мессенджер — это решение администратора,
// а не побочный эффект обновления.
func TestMigration046LeavesExistingChannelsOffByDefault(t *testing.T) {
	db, cleanup := migrateTo(t, 45)
	defer cleanup()

	if _, err := db.Exec(`
		INSERT INTO notification_channels (tenant_id, channel, enabled, config_json)
		VALUES (1, 'mattermost', TRUE, '{"base_url":"https://mm"}'::jsonb)`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	migrateDBTo(t, db, 46)

	var enabled, defaultOn bool
	if err := db.QueryRow(`
		SELECT enabled, default_on FROM notification_channels
		 WHERE tenant_id = 1 AND channel = 'mattermost'`).Scan(&enabled, &defaultOn); err != nil {
		t.Fatalf("канал: %v", err)
	}
	if !enabled {
		t.Error("миграция не должна выключать уже работающий канал")
	}
	if defaultOn {
		t.Error("существующий канал не должен становиться включённым у всех сам по себе")
	}
}

// Откат возвращает прежнюю форму: явно выключенный колокольчик остаётся
// выключенным, остальное читается как включённое.
func TestMigration046DownRestoresTheChannelList(t *testing.T) {
	db, cleanup := migrateTo(t, 45)
	defer cleanup()

	if _, err := db.Exec(`
		INSERT INTO notification_preferences (tenant_id, user_id, type, enabled, scope, channels)
		VALUES (1, 1, 'goal_changed', TRUE, 'own', '{in_app}'),
		       (1, 2, 'kr_progress',  TRUE, 'own', '{}')`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	migrateDBTo(t, db, 46)
	migrateDBTo(t, db, 45)

	for _, c := range []struct {
		userID int64
		typ    string
		want   int
	}{
		{1, "goal_changed", 1},
		{2, "kr_progress", 0},
	} {
		var n int
		if err := db.QueryRow(`
			SELECT cardinality(channels) FROM notification_preferences
			 WHERE tenant_id = 1 AND user_id = $1 AND type = $2`, c.userID, c.typ).Scan(&n); err != nil {
			t.Fatalf("user %d: %v", c.userID, err)
		}
		if n != c.want {
			t.Errorf("user %d / %s: каналов %d, want %d", c.userID, c.typ, n, c.want)
		}
	}
}
