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
	return &Store{root: root, db: db, queries: sqlc.New(db), clock: clk}, nil
}

// NewReadOnly opens the store already rooted at root for reading only
// (config.md section 6: reviews "writes nothing at all"). Unlike New, it
// never calls os.MkdirAll and never migrates: a store reviews may read is
// one validate has already created, and reviews checks Exists before ever
// calling this so a machine with no store yet is answered as empty rather
// than materialized. The Store it returns has no clock and must never
// reach Record — every read method works from it exactly as it does from
// one New returns.
func NewReadOnly(ctx context.Context, root string) (*Store, error) {
	db, err := OpenReadOnly(ctx, filepath.Join(root, "store.db"))
	if err != nil {
		return nil, err
	}
	return &Store{root: root, db: db, queries: sqlc.New(db)}, nil
}

// Close closes the underlying database.
func (s *Store) Close() error {
	return s.db.Close()
}
