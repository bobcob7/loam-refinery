package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bobcob7/loam-refinery/internal/store/sqlc"
)

// Store is the on-disk review store rooted at root (config.md section
// 4.1): store.db and the reviews/ and rejected/ trees beside it.
type Store struct {
	root    string
	db      *sql.DB
	queries *sqlc.Queries
	clock   clock
	// hasAssessment is false only for a read-only Store opened against a
	// database still at schema version 1, which predates the assessment
	// column entirely (refinery-xij). New always leaves it true: Open
	// migrates every database it touches up to schemaVersion first.
	hasAssessment bool
}

// New opens the store rooted at root, creating root and store.db when
// either is missing (config.md section 2.2: "first use creates what it
// needs"). The reviews/ and rejected/ trees are created on demand by the
// first write beneath them. clk supplies the time recorded on every run;
// pass NewClock() outside tests. This is validate's constructor alone
// (config.md section 6): a read never creates anything, so it uses
// NewReadOnly instead.
func New(ctx context.Context, root string, clk clock) (*Store, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("creating store %s: %w", root, err)
	}
	db, err := Open(ctx, filepath.Join(root, "store.db"))
	if err != nil {
		return nil, err
	}
	return &Store{root: root, db: db, queries: sqlc.New(db), clock: clk, hasAssessment: true}, nil
}

// NewReadOnly opens the store already rooted at root for reading only
// (config.md section 6: reviews "writes nothing at all"). Unlike New, it
// never calls os.MkdirAll and never migrates: a store reviews may read is
// one validate has already created, and reviews checks Exists before ever
// calling this so a machine with no store yet is answered as empty rather
// than materialized. The Store it returns has no clock and must never
// reach Record — every read method works from it exactly as it does from
// one New returns.
//
// A store at schema version 1 — every store written before assessment was
// added, and one store.enabled: false or a read-only store directory can
// never migrate on its own (refinery-xij) — has no assessment column at
// all. Rather than error, this reads PRAGMA user_version once and remembers
// whether the column exists; ListReviews and ListFailedRuns use that to
// pick a query that does not name the column, reporting every row's
// assessment as absent instead of failing. That keeps a version-1 store
// fully readable without ever writing to it.
func NewReadOnly(ctx context.Context, root string) (*Store, error) {
	db, err := OpenReadOnly(ctx, filepath.Join(root, "store.db"))
	if err != nil {
		return nil, err
	}
	version, err := readSchemaVersion(ctx, db)
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{root: root, db: db, queries: sqlc.New(db), hasAssessment: version >= assessmentColumnVersion}, nil
}

// Close closes the underlying database.
func (s *Store) Close() error {
	return s.db.Close()
}
