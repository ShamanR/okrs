package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"okrs/internal/platform/logging"
)

func serverMigrationDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dsn := os.Getenv("MIGRATIONS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set MIGRATIONS_TEST_DATABASE_URL for PostgreSQL startup tests")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("server_migration_%d", time.Now().UnixNano())
	if _, err = admin.Exec(`CREATE DATABASE ` + name); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	db, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(); admin.Exec(`DROP DATABASE ` + name + ` WITH (FORCE)`); admin.Close() })
	return db, u.String()
}
func TestMigrationStartupAndLogs(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	t.Run("success and no-op", func(t *testing.T) {
		db, dsn := serverMigrationDB(t)
		if err := runMigrations(dsn); err != nil {
			t.Fatal(err)
		}
		if err := runMigrations(dsn); err != nil {
			t.Fatal(err)
		}
		entries, err := os.ReadDir("migrations")
		if err != nil {
			t.Fatal(err)
		}
		var version int
		if err := db.QueryRow(`SELECT max(version_id) FROM goose_db_version`).Scan(&version); err != nil || version != len(entries) {
			t.Fatal(version, err)
		}
	})
	for _, scenario := range []string{"connection", "permissions", "migration"} {
		t.Run(scenario, func(t *testing.T) {
			db, dsn := serverMigrationDB(t)
			if scenario == "connection" {
				u, _ := url.Parse(dsn)
				q := u.Query()
				q.Set("port", "1")
				q.Set("connect_timeout", "1")
				u.RawQuery = q.Encode()
				dsn = u.String()
			}
			if scenario == "permissions" {
				role := fmt.Sprintf("migration_denied_%d", time.Now().UnixNano())
				if _, err := db.Exec(`CREATE ROLE ` + role + ` LOGIN PASSWORD 'startup-test-secret'; REVOKE CREATE ON SCHEMA public FROM PUBLIC`); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { db.Exec(`DROP ROLE ` + role) })
				u, _ := url.Parse(dsn)
				u.User = url.UserPassword(role, "startup-test-secret")
				dsn = u.String()
			}
			if scenario == "migration" {
				if _, err := db.Exec(`CREATE TABLE orphan(id int)`); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("DATABASE_URL", dsn)
			t.Setenv("LOG_FORMAT", "json")
			t.Setenv("TZ", "UTC")
			buf := &bytes.Buffer{}
			if code := runWith(buf, false); code != 1 {
				t.Fatal("expected startup exit 1", code)
			}
			records := decodeRecords(t, buf)
			errors := 0
			migration := 0
			for _, rec := range records {
				if rec[logging.KeyEvent] == logging.EventAppReady {
					t.Fatal("ready after migration failure")
				}
				if rec["level"] == "ERROR" {
					errors++
				}
				if rec[logging.KeyEvent] == logging.EventMigration {
					migration++
					for _, key := range []string{"time", "level", "msg", "event", "service", "env", "err"} {
						if rec[key] == nil {
							t.Errorf("missing log field %s", key)
						}
					}
				}
			}
			if errors != 1 || migration != 1 {
				t.Fatalf("errors=%d migrations=%d logs=%s", errors, migration, buf)
			}
			if strings.Contains(buf.String(), dsn) || strings.Contains(buf.String(), "startup-test-secret") {
				t.Fatal("secret exposed in logs")
			}
			if err := db.PingContext(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}
