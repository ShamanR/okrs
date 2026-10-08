// Package migrations prepares PostgreSQL databases and imports legacy migration history.
package migrations

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"hash/crc32"
	"io/fs"
	"time"

	"github.com/pressly/goose/v3"
)

// Up applies all available migrations. The caller retains ownership of db.
func Up(ctx context.Context, db *sql.DB, source fs.FS) error {
	return run(ctx, db, source, nil)
}

// To moves a database to a target version, including zero.
func To(ctx context.Context, db *sql.DB, source fs.FS, target int64) error {
	return run(ctx, db, source, &target)
}

func run(ctx context.Context, db *sql.DB, source fs.FS, target *int64) (retErr error) {
	if db == nil {
		return errors.New("migration database must not be nil")
	}
	reserved, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, reserved.Close()) }()
	var operationErr error
	rawErr := reserved.Raw(func(raw any) error {
		underlying, ok := raw.(driver.Conn)
		if !ok {
			return errors.New("unsupported migration driver connection")
		}
		private := sql.OpenDB(&pinnedConnector{conn: underlying})
		private.SetMaxOpenConns(1)
		private.SetMaxIdleConns(1)
		defer private.Close()
		p, err := goose.NewProvider(goose.DialectPostgres, private, source, goose.WithDisableGlobalRegistry(true), goose.WithLogger(goose.NopLogger()))
		if err != nil {
			operationErr = err
			return nil
		}
		locker := &sessionLocker{wait: 5 * time.Minute, versions: make(map[int64]bool)}
		for _, s := range p.ListSources() {
			locker.versions[s.Version] = true
		}
		for n, s := range p.ListSources() {
			if s.Version != int64(n+1) {
				operationErr = errors.New("migration files must form a consecutive prefix starting at 1")
				return nil
			}
		}

		if target != nil && (*target < 0 || (*target != 0 && !locker.versions[*target])) {
			operationErr = fmt.Errorf("unknown target version %d", *target)
			return nil
		}
		conn, err := private.Conn(ctx)
		if err != nil {
			operationErr = err
			return nil
		}
		err = locker.SessionLock(ctx, conn)
		if err != nil {
			operationErr = errors.Join(err, conn.Close())
			if locker.discard {
				return driver.ErrBadConn
			}
			return nil
		}
		// Return the borrowed handle to the private pool; the original reservation
		// still pins the physical session and its lock throughout Provider calls.
		if err = conn.Close(); err != nil {
			operationErr = err
			return driver.ErrBadConn
		}
		if target == nil {
			_, operationErr = p.Up(ctx)
		} else {
			current, err := p.GetDBVersion(ctx)
			operationErr = err
			if err == nil {
				switch {
				case current < *target:
					_, operationErr = p.UpTo(ctx, *target)
				case current > *target:
					_, operationErr = p.DownTo(ctx, *target)
				}
			}
		}
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		conn, err = private.Conn(cleanup)
		if err != nil {
			operationErr = errors.Join(operationErr, err)
			return driver.ErrBadConn
		}
		unlockErr := locker.SessionUnlock(cleanup, conn)
		operationErr = errors.Join(operationErr, unlockErr, conn.Close())
		if unlockErr != nil {
			return driver.ErrBadConn
		}
		return nil
	})
	return errors.Join(operationErr, rawErr)
}

type sessionLocker struct {
	versions map[int64]bool
	wait     time.Duration
	key      int64
	discard  bool
}

// legacyLockID follows golang-migrate v4.17.1 database.GenerateAdvisoryLockId
// (MIT license, Copyright golang-migrate contributors): CRC32 of schema, table,
// database separated by NUL, multiplied with uint32 overflow by its salt.
func legacyLockID(database, schema string) int64 {
	return int64(crc32.ChecksumIEEE([]byte(schema+"\x00schema_migrations\x00"+database)) * uint32(1486364155))
}
func (l *sessionLocker) SessionLock(ctx context.Context, conn *sql.Conn) (retErr error) {
	waiting, cancel := context.WithTimeout(ctx, l.wait)
	defer cancel()
	var database, schema string
	if err := conn.QueryRowContext(waiting, `SELECT current_database(), current_schema()`).Scan(&database, &schema); err != nil {
		return err
	}
	l.key = legacyLockID(database, schema)
	for {
		var acquired bool
		if err := conn.QueryRowContext(waiting, `SELECT pg_try_advisory_lock($1)`, l.key).Scan(&acquired); err != nil {
			l.discard = true
			return fmt.Errorf("migration lock: %w", err)
		}
		if acquired {
			break
		}
		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-waiting.Done():
			timer.Stop()
			return fmt.Errorf("migration lock: %w", waiting.Err())
		case <-timer.C:
		}
	}
	if err := l.bootstrap(ctx, conn); err != nil {
		return errors.Join(err, l.SessionUnlock(context.WithoutCancel(ctx), conn))
	}
	return nil
}
func (l *sessionLocker) SessionUnlock(ctx context.Context, conn *sql.Conn) error {
	cleanup, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var unlocked bool
	if err := conn.QueryRowContext(cleanup, `SELECT pg_advisory_unlock($1)`, l.key).Scan(&unlocked); err != nil {
		// An uncertain session must not return to the pool with a live lock.
		l.discard = true
		return err
	}
	if !unlocked {
		l.discard = true
		return errors.New("migration lock was not held")
	}
	return nil
}

func (l *sessionLocker) bootstrap(ctx context.Context, conn *sql.Conn) error {
	rows, err := conn.QueryContext(ctx, `SELECT relname FROM pg_class WHERE relnamespace=to_regnamespace(quote_ident(current_schema())) AND relkind IN ('r','p','S','v','m')`)
	if err != nil {
		return err
	}
	old, modern, objects := false, false, false
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		switch name {
		case "schema_migrations":
			old = true
		case "goose_db_version", "goose_db_version_id_seq":
			modern = true
		default:
			objects = true
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	var legacy int64
	if old {
		if err := validateHistoryTable(ctx, conn, "schema_migrations", map[string]string{"version": "bigint", "dirty": "boolean"}, "version"); err != nil {
			return err
		}
		rows, err := conn.QueryContext(ctx, `SELECT version,dirty FROM schema_migrations`)
		if err != nil {
			return fmt.Errorf("invalid legacy history: %w", err)
		}
		count := 0
		for rows.Next() {
			var dirty bool
			count++
			if err := rows.Scan(&legacy, &dirty); err != nil {
				rows.Close()
				return err
			}
			if dirty {
				rows.Close()
				return errors.New("legacy migration history is dirty")
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if count > 1 {
			return errors.New("legacy history must contain at most one row")
		}
		if count == 1 && !l.versions[legacy] {
			return fmt.Errorf("unknown legacy migration version %d", legacy)
		}
	}
	if modern {
		if err := validateHistoryTable(ctx, conn, "goose_db_version", map[string]string{"id": "integer", "version_id": "bigint", "is_applied": "boolean", "tstamp": "timestamp without time zone"}, "id"); err != nil {
			return err
		}
		return l.validateGoose(ctx, conn, objects)
	}
	if legacy == 0 {
		if objects {
			return errors.New("application objects exist without migration history")
		}
		return nil
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `CREATE TABLE goose_db_version (
 id integer PRIMARY KEY GENERATED BY DEFAULT AS IDENTITY,
 version_id bigint NOT NULL,is_applied boolean NOT NULL,
 tstamp timestamp NOT NULL DEFAULT now())`)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO goose_db_version (version_id,is_applied) SELECT version,true FROM generate_series(0,$1::bigint) AS version`, legacy)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (l *sessionLocker) validateGoose(ctx context.Context, conn *sql.Conn, objects bool) error {
	rows, err := conn.QueryContext(ctx, `SELECT id,version_id,is_applied,tstamp FROM goose_db_version ORDER BY id`)
	if err != nil {
		return fmt.Errorf("invalid goose history: %w", err)
	}
	defer rows.Close()
	applied := map[int64]bool{}
	seenIDs := map[int64]bool{}
	zero := 0
	for rows.Next() {
		var id, version int64
		var appliedFlag bool
		var stamp time.Time
		if err := rows.Scan(&id, &version, &appliedFlag, &stamp); err != nil {
			return err
		}
		if seenIDs[id] || id <= 0 {
			return errors.New("invalid goose history id")
		}
		seenIDs[id] = true
		if version == 0 {
			zero++
			if !appliedFlag {
				return errors.New("invalid goose zero version")
			}
		} else if !l.versions[version] {
			return fmt.Errorf("unknown goose migration version %d", version)
		}
		if _, exists := applied[version]; exists {
			return fmt.Errorf("duplicate goose migration version %d", version)
		}
		if !appliedFlag {
			return fmt.Errorf("unapplied record in goose history for version %d", version)
		}
		applied[version] = appliedFlag
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if zero != 1 {
		return errors.New("goose history must contain one initial version")
	}
	var max int64
	for version, ok := range applied {
		if ok && version > max {
			max = version
		}
	}
	for version := int64(1); version <= max; version++ {
		if !applied[version] {
			return fmt.Errorf("incomplete goose history: missing version %d", version)
		}
	}
	if max == 0 && objects {
		return errors.New("application objects exist with only zero migration history")
	}
	return nil
}

func validateHistoryTable(ctx context.Context, conn *sql.Conn, table string, expected map[string]string, primary string) error {
	rows, err := conn.QueryContext(ctx, `SELECT a.attname,format_type(a.atttypid,a.atttypmod),a.attnotnull,
 EXISTS(SELECT 1 FROM pg_constraint c WHERE c.conrelid=a.attrelid AND c.contype='p' AND c.conkey=ARRAY[a.attnum]),
 a.attidentity,COALESCE(pg_get_expr(d.adbin,d.adrelid),'')
 FROM pg_attribute a JOIN pg_class t ON t.oid=a.attrelid LEFT JOIN pg_attrdef d ON d.adrelid=a.attrelid AND d.adnum=a.attnum
 WHERE t.relnamespace=to_regnamespace(quote_ident(current_schema())) AND t.relname=$1 AND t.relkind IN ('r','p') AND a.attnum>0 AND NOT a.attisdropped`, table)
	if err != nil {
		return err
	}
	defer rows.Close()
	count := 0
	hasPrimary := false
	for rows.Next() {
		var name, kind, identity, defaultExpr string
		var required, pk bool
		if err := rows.Scan(&name, &kind, &required, &pk, &identity, &defaultExpr); err != nil {
			return err
		}
		if expected[name] != kind || !required {
			return fmt.Errorf("invalid %s column %s", table, name)
		}
		if table == "goose_db_version" && ((name == "id" && identity != "d") || (name == "tstamp" && defaultExpr != "now()")) {
			return fmt.Errorf("invalid %s default for %s", table, name)
		}
		if name == primary && pk {
			hasPrimary = true
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != len(expected) || !hasPrimary {
		return fmt.Errorf("invalid %s structure", table)
	}
	return nil
}
