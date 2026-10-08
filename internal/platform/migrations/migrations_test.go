package migrations

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// The golang-migrate history ends at lastLegacyVersion: a legacy database carries
// at most this version, and fixtures in testdata/legacy are generated up to it from
// legacyRevision, the last upstream revision with paired .up/.down files. Versions
// after it exist only in goose format and are checked by plain Up.
const (
	lastLegacyVersion = 49
	legacyRevision    = "58fbf14f93a98c32fb33d8bb99d96dfab20de531"
	migrationsDir     = "../../../migrations"
)

// latestVersion is the number of migration files, which form a consecutive prefix.
func latestVersion(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}

func legacyFixture(name string) string {
	return fmt.Sprintf("testdata/legacy/"+name, lastLegacyVersion)
}

// requireLegacyRevision skips git-based checks in checkouts without the history
// of legacyRevision, e.g. shallow clones or source archives.
func requireLegacyRevision(t *testing.T) {
	t.Helper()
	if err := exec.Command("git", "-C", "../../..", "cat-file", "-e", legacyRevision+"^{commit}").Run(); err != nil {
		t.Skipf("legacy revision %s is unavailable: %v", legacyRevision, err)
	}
}

func testDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dsn := os.Getenv("MIGRATIONS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set MIGRATIONS_TEST_DATABASE_URL to PostgreSQL 15 to run integration tests")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("migration_test_%d", time.Now().UnixNano())
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
func restore(t *testing.T, dsn, file string) {
	t.Helper()
	cmd := exec.Command("psql", dsn, "-v", "ON_ERROR_STOP=1", "-f", file)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("restore: %v: %s", err, out)
	}
}
func dump(t *testing.T, dsn string, schema bool) string {
	t.Helper()
	args := []string{dsn, "--no-owner", "--no-privileges", "--exclude-table=schema_migrations", "--exclude-table=goose_db_version", "--exclude-table=goose_db_version_id_seq"}
	if schema {
		args = append(args, "--schema-only")
	}
	out, err := exec.Command("pg_dump", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("dump: %v: %s", err, out)
	}
	var lines []string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "--") || strings.HasPrefix(line, `\restrict`) || strings.HasPrefix(line, `\unrestrict`) || strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
func TestLegacyAndClean(t *testing.T) {
	source := os.DirFS(migrationsDir)
	ctx := context.Background()
	reference, referenceDSN := testDB(t)
	restore(t, referenceDSN, legacyFixture("v%d.sql"))
	_ = reference
	clean, cleanDSN := testDB(t)
	if err := To(ctx, clean, source, lastLegacyVersion); err != nil {
		t.Fatal(err)
	}
	if got, want := dump(t, cleanDSN, true), dump(t, referenceDSN, true); got != want {
		t.Fatal("clean schema differs from legacy reference")
	}
	if err := Up(ctx, clean, source); err != nil {
		t.Fatal(err)
	}
	for _, version := range []int{22, 47, 48, lastLegacyVersion} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			db, dsn := testDB(t)
			restore(t, dsn, fmt.Sprintf("testdata/legacy/v%d.sql", version))
			before := dump(t, dsn, false)
			if err := To(ctx, db, source, lastLegacyVersion); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := db.QueryRow(`SELECT count(*) FROM goose_db_version WHERE is_applied`).Scan(&count); err != nil || count != lastLegacyVersion+1 {
				t.Fatalf("history count %d: %v", count, err)
			}
			var old int
			if err := db.QueryRow(`SELECT version FROM schema_migrations WHERE NOT dirty`).Scan(&old); err != nil || old != version {
				t.Fatalf("legacy version %d: %v", old, err)
			}
			if dump(t, dsn, true) != dump(t, referenceDSN, true) {
				t.Fatal("updated schema differs from reference")
			}
			after := dump(t, dsn, false)
			expectedDB, expectedDSN := testDB(t)
			_ = expectedDB
			restore(t, expectedDSN, fmt.Sprintf("testdata/legacy/v%d-to%d.sql", version, lastLegacyVersion))
			if normalizeGeneratedDates(after, version) != normalizeGeneratedDates(dump(t, expectedDSN, false), version) {
				t.Fatal("updated application data differs from independent legacy continuation")
			}

			if version == lastLegacyVersion && before != after {
				t.Fatal("migration changed existing data or sequences")
			}
			if err := To(ctx, db, source, lastLegacyVersion); err != nil {
				t.Fatal(err)
			}
			if dump(t, dsn, false) != after {
				t.Fatal("repeat start changed application state")
			}
			if err := To(ctx, db, source, lastLegacyVersion-1); err != nil {
				t.Fatal(err)
			}
			if err := To(ctx, db, source, lastLegacyVersion); err != nil {
				t.Fatal(err)
			}
			if err := To(ctx, db, source, lastLegacyVersion); err != nil {
				t.Fatal(err)
			}
			if err := Up(ctx, db, source); err != nil {
				t.Fatal(err)
			}
			latest := dump(t, dsn, false)
			if err := Up(ctx, db, source); err != nil {
				t.Fatal(err)
			}
			if dump(t, dsn, false) != latest {
				t.Fatal("repeat start changed application state")
			}
		})
	}
	if err := clean.PingContext(ctx); err != nil {
		t.Fatal("runner closed caller database", err)
	}
}
func TestConcurrentAndRollback(t *testing.T) {
	db, dsn := testDB(t)
	other, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	source := fstest.MapFS{"001_test.sql": {Data: []byte("-- +goose Up\nCREATE TABLE once_only(id int);\n-- +goose Down\nDROP TABLE once_only;\n")}}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, conn := range []*sql.DB{db, other} {
		wg.Add(1)
		go func(db *sql.DB) { defer wg.Done(); errs <- Up(context.Background(), db, source) }(conn)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	source["002_fail.sql"] = &fstest.MapFile{Data: []byte("-- +goose Up\nCREATE TABLE rolled_back(id int);\nSELECT 1/0;\n-- +goose Down\nDROP TABLE rolled_back;\n")}
	if err := Up(context.Background(), db, source); err == nil {
		t.Fatal("expected error")
	}
	var exists bool
	if err := db.QueryRow(`SELECT to_regclass('rolled_back') IS NOT NULL`).Scan(&exists); err != nil || exists {
		t.Fatal("transaction did not roll back", err)
	}
	delete(source, "002_fail.sql")
	if err := Up(context.Background(), other, source); err != nil {
		t.Fatal("lock leaked", err)
	}
}

func TestHistoryRefusalAndEmptyStates(t *testing.T) {
	cases := []struct {
		name, setup string
		ok          bool
	}{
		{"new", "", true},
		{"empty legacy", `CREATE TABLE schema_migrations(version bigint PRIMARY KEY,dirty boolean NOT NULL)`, true},
		{"dirty", `CREATE TABLE schema_migrations(version bigint PRIMARY KEY,dirty boolean NOT NULL);INSERT INTO schema_migrations VALUES(1,true)`, false},
		{"future", `CREATE TABLE schema_migrations(version bigint PRIMARY KEY,dirty boolean NOT NULL);INSERT INTO schema_migrations VALUES(1000,false)`, false},
		{"unknown zero legacy", `CREATE TABLE schema_migrations(version bigint PRIMARY KEY,dirty boolean NOT NULL);INSERT INTO schema_migrations VALUES(0,false)`, false},
		{"multiple legacy", `CREATE TABLE schema_migrations(version bigint PRIMARY KEY,dirty boolean NOT NULL);INSERT INTO schema_migrations VALUES(1,false),(2,false)`, false},
		{"invalid legacy structure", `CREATE TABLE schema_migrations(version text,dirty boolean);INSERT INTO schema_migrations VALUES('1',false)`, false},
		{"tables without history", `CREATE TABLE orphan(id int)`, false},
		{"empty legacy with table", `CREATE TABLE schema_migrations(version bigint PRIMARY KEY,dirty boolean NOT NULL);CREATE TABLE orphan(id int)`, false},
		{"zero goose", `CREATE TABLE goose_db_version(id integer PRIMARY KEY GENERATED BY DEFAULT AS IDENTITY,version_id bigint NOT NULL,is_applied boolean NOT NULL,tstamp timestamp NOT NULL DEFAULT now());INSERT INTO goose_db_version(version_id,is_applied)VALUES(0,true)`, true},
		{"zero goose with table", `CREATE TABLE goose_db_version(id integer PRIMARY KEY GENERATED BY DEFAULT AS IDENTITY,version_id bigint NOT NULL,is_applied boolean NOT NULL,tstamp timestamp NOT NULL DEFAULT now());INSERT INTO goose_db_version(version_id,is_applied)VALUES(0,true);CREATE TABLE orphan(id int)`, false},
		{"empty goose", `CREATE TABLE goose_db_version(id integer PRIMARY KEY GENERATED BY DEFAULT AS IDENTITY,version_id bigint NOT NULL,is_applied boolean NOT NULL,tstamp timestamp NOT NULL DEFAULT now())`, false},
		{"gap", `CREATE TABLE goose_db_version(id integer PRIMARY KEY GENERATED BY DEFAULT AS IDENTITY,version_id bigint NOT NULL,is_applied boolean NOT NULL,tstamp timestamp NOT NULL DEFAULT now());INSERT INTO goose_db_version(version_id,is_applied)VALUES(0,true),(2,true)`, false},
		{"future goose", `CREATE TABLE goose_db_version(id integer PRIMARY KEY GENERATED BY DEFAULT AS IDENTITY,version_id bigint NOT NULL,is_applied boolean NOT NULL,tstamp timestamp NOT NULL DEFAULT now());INSERT INTO goose_db_version(version_id,is_applied)VALUES(0,true),(1000,true)`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db, dsn := testDB(t)
			if c.setup != "" {
				if _, err := db.Exec(c.setup); err != nil {
					t.Fatal(err)
				}
			}
			before, err := exec.Command("pg_dump", dsn, "--no-owner", "--no-privileges").Output()
			if err != nil {
				t.Fatal(err)
			}
			err = Up(context.Background(), db, os.DirFS(migrationsDir))
			if c.ok {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected refusal")
			}
			after, err := exec.Command("pg_dump", dsn, "--no-owner", "--no-privileges").Output()
			if err != nil {
				t.Fatal(err)
			}
			normalize := func(s string) string {
				var out []string
				for _, line := range strings.Split(s, "\n") {
					if strings.HasPrefix(line, `\restrict`) || strings.HasPrefix(line, `\unrestrict`) {
						continue
					}
					out = append(out, line)
				}
				return strings.Join(out, "\n")
			}
			if normalize(string(before)) != normalize(string(after)) {
				t.Fatal("refusal changed database")
			}
			conn, err := db.Conn(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			var name, schema string
			if err = conn.QueryRowContext(context.Background(), `SELECT current_database(),current_schema()`).Scan(&name, &schema); err != nil {
				t.Fatal(err)
			}
			var free bool
			if err = conn.QueryRowContext(context.Background(), `SELECT pg_try_advisory_lock($1)`, legacyLockID(name, schema)).Scan(&free); err != nil || !free {
				t.Fatal("lock leaked", err)
			}
			if _, err = conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, legacyLockID(name, schema)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Only migrations 027 and 028 generate these timestamps after the version-22
// fixture. Preserve all timestamps already present in the input database.
func normalizeGeneratedDates(dump string, version int) string {
	if version != 22 {
		return dump
	}
	lines := strings.Split(dump, "\n")
	column := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "COPY public.tenants (") || strings.HasPrefix(line, "COPY public.memberships (") {
			a := strings.Index(line, "(")
			b := strings.Index(line, ")")
			column = -1
			for n, name := range strings.Split(line[a+1:b], ", ") {
				if name == "created_at" {
					column = n
				}
			}
			continue
		}
		if strings.HasPrefix(line, `\.`) {
			column = -1
			continue
		}
		if column >= 0 {
			values := strings.Split(line, "\t")
			if len(values) > column {
				values[column] = "<generated by legacy migrations 027/028>"
				lines[i] = strings.Join(values, "\t")
			}
		}
	}
	return strings.Join(lines, "\n")
}

func TestLegacyLockAndReturn(t *testing.T) {
	exe := os.Getenv("LEGACY_MIGRATION_RUNNER")
	if exe == "" {
		t.Skip("set LEGACY_MIGRATION_RUNNER to independent v4.17.1 runner")
	}
	for _, v := range [][2]string{{"okrs", "public"}, {"other_database", "public"}, {"okrs", "tenant_schema"}} {
		out, err := exec.Command(exe, "key", v[0], v[1]).Output()
		if err != nil {
			t.Fatal(err)
		}
		want, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		if got := legacyLockID(v[0], v[1]); got != want {
			t.Fatalf("lock key %v: %d != %d", v, got, want)
		}
	}
	requireLegacyRevision(t)
	db, dsn := testDB(t)
	restore(t, dsn, legacyFixture("v%d.sql"))
	source := os.DirFS(migrationsDir)
	before := dump(t, dsn, false)
	if err := Up(context.Background(), db, source); err != nil {
		t.Fatal(err)
	}
	historical := t.TempDir()
	cmd := exec.Command("git", "-C", "../../..", "archive", legacyRevision, "migrations")
	data, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	tar := exec.Command("tar", "-x", "-C", historical)
	tar.Stdin = strings.NewReader(string(data))
	if out, err := tar.CombinedOutput(); err != nil {
		t.Fatalf("extract historical SQL %v %s", err, out)
	}
	if out, err := exec.Command(exe, historical+"/migrations", dsn, strconv.Itoa(lastLegacyVersion)).CombinedOutput(); err != nil {
		t.Fatalf("previous release: %v %s", err, out)
	}
	if err := Up(context.Background(), db, source); err != nil {
		t.Fatal(err)
	}
	if before != dump(t, dsn, false) {
		t.Fatal("new-old-new changed application data")
	}
	// Legacy acquires first: the new runner must wait and honor cancellation.
	old := exec.Command(exe, "lock", dsn)
	stdin, err := old.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := old.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = old.Start(); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(stdout)
	line, err := reader.ReadString('\n')
	if err != nil || line != "locked\n" {
		t.Fatal("old lock", line, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	err = Up(ctx, db, source)
	cancel()
	if err == nil {
		t.Fatal("new runner bypassed old lock")
	}
	io.WriteString(stdin, "release\n")
	stdin.Close()
	if err = old.Wait(); err != nil {
		t.Fatal(err)
	}
	if err = Up(context.Background(), db, source); err != nil {
		t.Fatal(err)
	}
	// New-compatible lock first: the actual legacy driver must wait.
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var name, schema string
	conn.QueryRowContext(context.Background(), `SELECT current_database(),current_schema()`).Scan(&name, &schema)
	key := legacyLockID(name, schema)
	if _, err = conn.ExecContext(context.Background(), `SELECT pg_advisory_lock($1)`, key); err != nil {
		t.Fatal(err)
	}
	old = exec.Command(exe, "lock", dsn)
	stdin, _ = old.StdinPipe()
	stdout, _ = old.StdoutPipe()
	if err = old.Start(); err != nil {
		t.Fatal(err)
	}
	acquired := make(chan string, 1)
	go func() { line, _ := bufio.NewReader(stdout).ReadString('\n'); acquired <- line }()
	select {
	case line := <-acquired:
		t.Fatalf("old runner bypassed lock: %s", line)
	case <-time.After(120 * time.Millisecond):
	}
	if _, err = conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, key); err != nil {
		t.Fatal(err)
	}
	select {
	case line := <-acquired:
		if line != "locked\n" {
			t.Fatal(line)
		}
	case <-time.After(5 * time.Second):
		old.Process.Kill()
		t.Fatal("legacy failed to acquire released lock")
	}
	io.WriteString(stdin, "release\n")
	stdin.Close()
	if err = old.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestBootstrapFailuresAndResume(t *testing.T) {
	source := os.DirFS(migrationsDir)
	t.Run("before commit", func(t *testing.T) {
		db, dsn := testDB(t)
		restore(t, dsn, legacyFixture("v%d.sql"))
		_, err := db.Exec(`CREATE FUNCTION reject_import() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected bootstrap failure'; END $$;
 CREATE FUNCTION install_import_failure() RETURNS event_trigger LANGUAGE plpgsql AS $$
 BEGIN IF to_regclass('goose_db_version') IS NOT NULL THEN EXECUTE 'CREATE TRIGGER reject_import BEFORE INSERT ON goose_db_version FOR EACH STATEMENT EXECUTE FUNCTION reject_import()'; END IF; END $$;
 CREATE EVENT TRIGGER inject_bootstrap_failure ON ddl_command_end WHEN TAG IN ('CREATE TABLE') EXECUTE FUNCTION install_import_failure()`)
		if err != nil {
			t.Fatal(err)
		}
		before := dump(t, dsn, false)
		if err := Up(context.Background(), db, source); err == nil {
			t.Fatal("expected bootstrap failure")
		}
		var exists bool
		if err := db.QueryRow(`SELECT to_regclass('goose_db_version') IS NOT NULL`).Scan(&exists); err != nil || exists {
			t.Fatal("bootstrap DDL was not rolled back", err)
		}
		if dump(t, dsn, false) != before {
			t.Fatal("failed bootstrap changed application state")
		}
		if _, err := db.Exec(`DROP EVENT TRIGGER inject_bootstrap_failure;DROP FUNCTION install_import_failure();DROP FUNCTION reject_import()`); err != nil {
			t.Fatal(err)
		}
		if err := Up(context.Background(), db, source); err != nil {
			t.Fatal("retry failed", err)
		}
	})
	t.Run("committed bootstrap before provider", func(t *testing.T) {
		db, dsn := testDB(t)
		restore(t, dsn, "testdata/legacy/v22.sql")
		before := dump(t, dsn, false)
		conn, err := db.Conn(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		l := &sessionLocker{wait: time.Second, versions: make(map[int64]bool)}
		for version := int64(1); version <= lastLegacyVersion; version++ {
			l.versions[version] = true
		}
		if err := l.SessionLock(context.Background(), conn); err != nil {
			t.Fatal(err)
		}
		var pid int
		if err := conn.QueryRowContext(context.Background(), `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`SELECT pg_terminate_backend($1)`, pid); err != nil {
			t.Fatal(err)
		}
		conn.Close()
		var max int
		if err := db.QueryRow(`SELECT max(version_id) FROM goose_db_version`).Scan(&max); err != nil || max != 22 {
			t.Fatal(max, err)
		}
		if before != dump(t, dsn, false) {
			t.Fatal("committed bootstrap changed application state")
		}
		if err := Up(context.Background(), db, source); err != nil {
			t.Fatal("restart after disconnected bootstrap", err)
		}
	})
	t.Run("after commit", func(t *testing.T) {
		db, dsn := testDB(t)
		restore(t, dsn, "testdata/legacy/v22.sql")
		altered := fstest.MapFS{}
		entries, err := os.ReadDir(migrationsDir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			data, err := os.ReadFile(filepath.Join(migrationsDir, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(entry.Name(), "023_") {
				data = []byte("-- +goose Up\nCREATE TABLE rolled_back(id int);\nSELECT 1/0;\n-- +goose Down\nDROP TABLE rolled_back;\n")
			}
			altered[entry.Name()] = &fstest.MapFile{Data: data}
		}
		before := dump(t, dsn, false)
		if err := Up(context.Background(), db, altered); err == nil {
			t.Fatal("expected failed migration after import")
		}
		var max, count int
		if err := db.QueryRow(`SELECT max(version_id),count(*) FROM goose_db_version`).Scan(&max, &count); err != nil || max != 22 || count != 23 {
			t.Fatalf("history %d/%d: %v", max, count, err)
		}
		if dump(t, dsn, false) != before {
			t.Fatal("failed transactional migration changed application state")
		}
		if err := Up(context.Background(), db, source); err != nil {
			t.Fatal("resume", err)
		}
	})
}

func TestCustomSchemaAndLockTimeout(t *testing.T) {
	db, dsn := testDB(t)
	if _, err := db.Exec(`CREATE SCHEMA "Fixture Schema"`); err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(dsn)
	q := u.Query()
	q.Set("search_path", `"Fixture Schema"`)
	u.RawQuery = q.Encode()
	scoped, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer scoped.Close()
	if err := Up(context.Background(), scoped, os.DirFS(migrationsDir)); err != nil {
		t.Fatal(err)
	}
	var schema string
	if err := scoped.QueryRow(`SELECT current_schema()`).Scan(&schema); err != nil || schema != "Fixture Schema" {
		t.Fatal(schema, err)
	}
	first, err := scoped.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := scoped.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	var name string
	if err := first.QueryRowContext(context.Background(), `SELECT current_database()`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	key := legacyLockID(name, schema)
	if _, err := first.ExecContext(context.Background(), `SELECT pg_advisory_lock($1)`, key); err != nil {
		t.Fatal(err)
	}
	l := &sessionLocker{wait: 100 * time.Millisecond}
	start := time.Now()
	if err := l.SessionLock(context.Background(), second); err == nil {
		t.Fatal("expected lock timeout")
	}
	if time.Since(start) > time.Second {
		t.Fatal("lock wait exceeded timeout")
	}
	if _, err := first.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, key); err != nil {
		t.Fatal(err)
	}
	if err := Up(context.Background(), scoped, os.DirFS(migrationsDir)); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentLegacyAndContinuousLock(t *testing.T) {
	db, dsn := testDB(t)
	restore(t, dsn, "testdata/legacy/v22.sql")
	other, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	source := os.DirFS(migrationsDir)
	errs := make(chan error, 2)
	for _, pool := range []*sql.DB{db, other} {
		go func(pool *sql.DB) { errs <- Up(context.Background(), pool, source) }(pool)
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	var versions int
	if err := db.QueryRow(`SELECT count(*) FROM goose_db_version`).Scan(&versions); err != nil || versions != latestVersion(t)+1 {
		t.Fatal(versions, err)
	}
	exe := os.Getenv("LEGACY_MIGRATION_RUNNER")
	if exe == "" {
		t.Skip("set LEGACY_MIGRATION_RUNNER for mixed-process lock test")
	}
	// A synthetic migration after the latest one is confined to the in-memory
	// test FS. Its delay exposes the lock across actual SQL work.
	db, dsn = testDB(t)
	restore(t, dsn, legacyFixture("v%d.sql"))
	delayed := fstest.MapFS{}
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(migrationsDir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		delayed[entry.Name()] = &fstest.MapFile{Data: data}
	}
	delayed[fmt.Sprintf("%03d_probe.sql", latestVersion(t)+1)] = &fstest.MapFile{Data: []byte("-- +goose Up\nSELECT pg_sleep(1.5) /* migration_lock_probe */;\n-- +goose Down\nSELECT 1;\n")}
	done := make(chan error, 1)
	go func() { done <- Up(context.Background(), db, delayed) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var running bool
		if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE pid<>pg_backend_pid() AND state='active' AND query LIKE '%migration_lock_probe%')`).Scan(&running); err != nil {
			t.Fatal(err)
		}
		if running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("migration did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	old := exec.Command(exe, "lock", dsn)
	stdin, _ := old.StdinPipe()
	stdout, _ := old.StdoutPipe()
	if err := old.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if old.Process != nil {
			old.Process.Kill()
		}
	}()
	acquired := make(chan string, 1)
	go func() { line, _ := bufio.NewReader(stdout).ReadString('\n'); acquired <- line }()
	select {
	case line := <-acquired:
		t.Fatalf("legacy acquired lock during new migration: %s", line)
	case <-time.After(150 * time.Millisecond):
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	select {
	case line := <-acquired:
		if line != "locked\n" {
			t.Fatal(line)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("legacy lock was not released")
	}
	io.WriteString(stdin, "release\n")
	stdin.Close()
	if err := old.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestSQLBodiesMatchHistoricalRevision(t *testing.T) {
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < lastLegacyVersion {
		t.Fatalf("expected at least %d migration files, got %d", lastLegacyVersion, len(entries))
	}
	prefix := []byte("-- +goose Up\n-- +goose StatementBegin\n")
	separator := []byte("\n-- +goose StatementEnd\n\n-- +goose Down\n-- +goose StatementBegin\n")
	suffix := []byte("\n-- +goose StatementEnd\n")
	for n, entry := range entries {
		version, err := strconv.Atoi(entry.Name()[:3])
		if err != nil || version != n+1 {
			t.Fatalf("invalid source version %s", entry.Name())
		}
	}
	requireLegacyRevision(t)
	for _, entry := range entries[:lastLegacyVersion] {
		data, err := os.ReadFile(filepath.Join(migrationsDir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		parts := bytes.Split(data, separator)
		if len(parts) != 2 || !bytes.HasPrefix(parts[0], prefix) || !bytes.HasSuffix(parts[1], suffix) {
			t.Fatalf("invalid SQL sections in %s", entry.Name())
		}
		bodies := [][]byte{bytes.TrimPrefix(parts[0], prefix), bytes.TrimSuffix(parts[1], suffix)}
		for i, direction := range []string{"up", "down"} {
			original := "migrations/" + strings.TrimSuffix(entry.Name(), ".sql") + "." + direction + ".sql"
			expected, err := exec.Command("git", "-C", "../../..", "show", legacyRevision+":"+original).Output()
			if err != nil {
				t.Fatal("historical revision is required", err)
			}
			if !bytes.Equal(bodies[i], expected) {
				t.Fatalf("SQL body changed: %s", original)
			}
		}
	}
}
