// Package store owns the on-disk review store: the SQLite database that
// records one row per run (config.md §4.5) and the trees of stored review
// files beside it.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

//go:generate sqlc generate -f ../../sqlc.yaml

//go:embed sql/schema.sql
var schema string

//go:embed sql/migrations/0002_assessment.sql
var migration0002 string

// migrations maps a schema version to the SQL that advances a database
// already at that version to the next one. migrate walks this from a
// database's current PRAGMA user_version up to schemaVersion, one step at a
// time — the "explicit migration alongside the schema.sql change that
// produced it" schemaVersion's own comment asks for, kept here rather than
// folded into schema.sql because schema.sql is what a brand-new database
// gets applied whole (version 0), while a database already at a prior
// version needs exactly the delta, not the whole shape replayed.
var migrations = map[int]string{
	1: migration0002, // 1 -> 2: adds the assessment column (config.md §4.5.2).
}

// schemaVersion is the PRAGMA user_version stamped on a database created by
// the schema this binary embeds. Bump it, and ship an explicit migration
// alongside the schema.sql change that produced it, whenever the shape of
// runs changes (config.md §4.5.4).
const schemaVersion = 2

// busyTimeoutMillis is the milliseconds a writer waits for a lock before
// giving up (config.md §4.6). It doubles as the budget convertToWAL retries
// against: PRAGMA journal_mode=WAL bypasses the busy handler this same
// value configures, so it needs its own wait loop to honor the promise the
// pragma's name implies.
const busyTimeoutMillis = 5000

// Open opens the SQLite database at path in WAL mode with a busy timeout
// (config.md §4.6), applying the embedded schema when the file is new and
// stamping PRAGMA user_version so a later revision knows what it is
// migrating from. sqlc does not do migrations (config.md §4.5.4); this
// ordering is the store's own code.
//
// Concurrent first use — several processes opening a store that does not
// exist yet — has two races the driver's busy handler does not cover on
// its own, both handled explicitly here rather than papered over:
//
//   - PRAGMA journal_mode=WAL on a database not yet in WAL mode takes an
//     exclusive lock, and does NOT run the busy handler busy_timeout
//     installs — a documented SQLite quirk, not a bug in this driver. N
//     processes racing to create a store mostly lose without help.
//     convertToWAL retries the pragma itself, with its own backoff,
//     against the same budget busy_timeout already promises. That was
//     chosen over serializing creation behind a separate lock file: it
//     needs no new cross-process primitive, and the wait a caller sees is
//     the one config.md §4.6 already documents, not a second one.
//   - PRAGMA user_version, read outside a transaction, is a check a
//     concurrent writer can invalidate before this connection acts on it:
//     two processes can both read version 0 and both try to CREATE TABLE.
//     The DSN's _txlock=immediate makes every transaction on this
//     connection take the write lock at BEGIN rather than at its first
//     write statement, so migrate's version check and its schema
//     application below are atomic against a concurrent Open the ordinary
//     way SQLite already serializes writers — through busy_timeout, which
//     unlike the WAL pragma does honor the busy handler for this kind of
//     wait.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	dsn := storeDSN(path, fmt.Sprintf("_pragma=busy_timeout(%d)&_txlock=immediate", busyTimeoutMillis))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database %s: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connecting to database %s: %w", path, err)
	}
	if err := convertToWAL(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("converting database %s to WAL: %w", path, err)
	}
	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrating database %s: %w", path, err)
	}
	return db, nil
}

// OpenReadOnly opens the SQLite database at path read-only: no write, no
// WAL conversion, no migration, and no MkdirAll of anything above it
// (config.md §6 — reviews "writes nothing at all"). It errors if path does
// not already exist; a caller that must not create a store checks Exists
// first.
//
// mode=ro is tried first because it is the connection that reads correctly
// regardless of what state the WAL is in: verified with a live, uncheck-
// pointed -wal file (a writer's rows not yet folded into the main file)
// present on a directory chmod'd to 0500, it returns every row, including
// ones only in the WAL — unlike immutable=1, which assumes the file will
// never change and reads only the checkpointed main file, silently missing
// them. But mode=ro alone is not sufficient on its own: a store that HAS
// been fully checkpointed — the ordinary state between validate runs,
// since the last connection to close checkpoints and deletes the WAL —
// still fails mode=ro on a read-only directory, with SQLITE_READONLY_
// DIRECTORY, because opening a WAL-mode database always tries to confirm
// it can create the -shm file even when there is nothing in a -wal file to
// read. That is the specific, narrow condition this function falls back to
// immutable=1 on: it is also the one condition where immutable=1's
// limitation is irrelevant, because a fully checkpointed database has no
// -wal content for it to miss in the first place.
func OpenReadOnly(ctx context.Context, path string) (*sql.DB, error) {
	db, err := openReadOnlyMode(ctx, path, "mode=ro")
	if err == nil {
		return db, nil
	}
	if !isSQLiteReadOnlyDirectory(err) {
		return nil, err
	}
	db, fallbackErr := openReadOnlyMode(ctx, path, "mode=ro&immutable=1")
	if fallbackErr != nil {
		return nil, fallbackErr
	}
	return db, nil
}

// openReadOnlyMode opens path with modeParams (a mode=ro DSN fragment,
// optionally with immutable=1 added) plus the store's busy timeout.
func openReadOnlyMode(ctx context.Context, path, modeParams string) (*sql.DB, error) {
	dsn := storeDSN(path, fmt.Sprintf("%s&_pragma=busy_timeout(%d)", modeParams, busyTimeoutMillis))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database %s: %w", path, err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connecting to database %s: %w", path, err)
	}
	return db, nil
}

// isSQLiteReadOnlyDirectory reports whether err is SQLITE_READONLY_
// DIRECTORY — SQLite's own name for "the directory needed to create a
// lock or -shm file could not be written to," which is exactly what a
// mode=ro connection hits opening a checkpointed WAL-mode database on a
// directory it cannot write to. See OpenReadOnly's doc comment for why
// this is the one error that is safe to retry as immutable=1.
func isSQLiteReadOnlyDirectory(err error) bool {
	var sqliteErr *sqlite.Error
	return errors.As(err, &sqliteErr) && sqliteErr.Code() == sqlite3.SQLITE_READONLY_DIRECTORY
}

// storeDSN builds a SQLite URI DSN for path, escaping path into the URI
// rather than concatenating it ahead of rawQuery (config.md §4.1). A path
// holding '?', '#', or '%' is legal on every filesystem this tool runs on;
// concatenating it lets SQLite parse everything after the first such
// character as the query string instead of the filename, silently
// dropping every pragma that follows and creating the database beside the
// configured store directory rather than inside it.
func storeDSN(path, rawQuery string) string {
	u := url.URL{Scheme: "file", Path: path, RawQuery: rawQuery}
	return u.String()
}

// convertToWAL sets journal_mode=WAL on db, retrying on SQLITE_BUSY with a
// short backoff up to busyTimeoutMillis. See Open's doc comment for why
// this cannot be left to the DSN's busy_timeout pragma.
func convertToWAL(ctx context.Context, db *sql.DB) error {
	deadline := time.Now().Add(busyTimeoutMillis * time.Millisecond)
	backoff := 5 * time.Millisecond
	for {
		_, err := db.ExecContext(ctx, "PRAGMA journal_mode=WAL")
		if err == nil {
			return nil
		}
		if !isSQLiteBusy(err) || time.Now().After(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		if backoff < 100*time.Millisecond {
			backoff *= 2
		}
	}
}

// isSQLiteBusy reports whether err is SQLITE_BUSY — the code PRAGMA
// journal_mode=WAL returns when another connection holds the exclusive
// lock the conversion needs, and the one convertToWAL retries on.
func isSQLiteBusy(err error) bool {
	var sqliteErr *sqlite.Error
	return errors.As(err, &sqliteErr) && sqliteErr.Code() == sqlite3.SQLITE_BUSY
}

// Exists reports whether a store already has a database at root, without
// creating anything (docs/config.md §2.2, §6.2). A reader — reviews — checks
// this before ever calling New, since New creates root and store.db when
// either is missing, which a read must never do on a machine that has no
// store yet.
func Exists(root string) (bool, error) {
	_, err := os.Stat(filepath.Join(root, "store.db"))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("checking store %s: %w", root, err)
}

// migrate applies the embedded schema to a database that does not yet have
// one, or the pending steps in migrations to one that has an earlier
// version, and records the revision it left the file at. A database already
// at schemaVersion is left untouched; one ahead of it is a downgrade this
// binary cannot open.
//
// The version check and the schema application happen inside the same
// transaction, on purpose: reading PRAGMA user_version outside a
// transaction is a check a concurrent Open can invalidate before this one
// acts on it, and two processes racing to create a store can both read
// version 0 and both try to CREATE TABLE. Starting the transaction first —
// on a connection whose DSN sets _txlock=immediate — takes the write lock
// before the version is even read, so the check this function makes is the
// one it also acts on.
func migrate(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning migration: %w", err)
	}
	defer tx.Rollback()
	var version int
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}
	if version == schemaVersion {
		return tx.Commit()
	}
	if version > schemaVersion {
		return fmt.Errorf("database is at schema version %d, this binary knows %d", version, schemaVersion)
	}
	if version == 0 {
		if _, err := tx.ExecContext(ctx, schema); err != nil {
			return fmt.Errorf("applying schema: %w", err)
		}
	} else {
		for v := version; v < schemaVersion; v++ {
			stmt, ok := migrations[v]
			if !ok {
				return fmt.Errorf("no migration from schema version %d to %d", v, v+1)
			}
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("migrating database from schema version %d: %w", v, err)
			}
		}
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)); err != nil {
		return fmt.Errorf("setting schema version: %w", err)
	}
	return tx.Commit()
}
