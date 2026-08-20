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
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

//go:generate sqlc generate -f ../../sqlc.yaml

//go:embed sql/schema.sql
var schema string

// schemaVersion is the PRAGMA user_version stamped on a database created by
// the schema this binary embeds. Bump it, and ship an explicit migration
// alongside the schema.sql change that produced it, whenever the shape of
// runs changes (config.md §4.5.4).
const schemaVersion = 1

// Open opens the SQLite database at path in WAL mode with a busy timeout
// (config.md §4.6), applying the embedded schema when the file is new and
// stamping PRAGMA user_version so a later revision knows what it is
// migrating from. sqlc does not do migrations (config.md §4.5.4); this
// ordering is the store's own code.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database %s: %w", path, err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connecting to database %s: %w", path, err)
	}
	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrating database %s: %w", path, err)
	}
	return db, nil
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
// one, and records the revision it left the file at. A database already at
// schemaVersion is left untouched; one ahead of it is a downgrade this
// binary cannot open.
func migrate(ctx context.Context, db *sql.DB) error {
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}
	if version == schemaVersion {
		return nil
	}
	if version != 0 {
		return fmt.Errorf("database is at schema version %d, this binary knows %d", version, schemaVersion)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning migration: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("applying schema: %w", err)
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)); err != nil {
		return fmt.Errorf("setting schema version: %w", err)
	}
	return tx.Commit()
}
