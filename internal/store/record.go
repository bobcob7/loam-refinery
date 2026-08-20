// Recording a run (config.md section 4.5.1).
package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/bobcob7/loam-refinery/internal/store/sqlc"
)

// atLayout is the format the "at" column is written and read in — RFC 3339
// in UTC, matching the schema's comment.
const atLayout = time.RFC3339

// RunInput is one run's row, per config.md section 4.5.1. Ref and Verdict
// are "" for NULL — a ref is always 40 hex characters and a verdict is
// always one of three non-empty words when either is present, so an empty
// string is unambiguous. The counters are pointers so a caller can record
// an explicit zero and leave the rest NULL, for a run that got far enough
// to count some things but not others.
type RunInput struct {
	Repo          string
	Ref           string
	Digest        string
	ExitCode      int
	Verdict       string
	NumComments   *int
	NumErrors     *int
	NumAdvisories *int
	NumSkipped    *int
	ToolVersion   string
	SchemaVersion string
}

// Record inserts one row for in, stamped with the store's clock. It is the
// caller's responsibility to have already written any file the row
// accounts for (config.md section 4.6): Record does not write files, only
// the row that follows one.
func (s *Store) Record(ctx context.Context, in RunInput) error {
	_, err := s.queries.InsertRun(ctx, sqlc.InsertRunParams{
		At:            s.clock.Now().UTC().Format(atLayout),
		Repo:          in.Repo,
		Ref:           nullString(in.Ref),
		Digest:        in.Digest,
		ExitCode:      int64(in.ExitCode),
		Verdict:       nullString(in.Verdict),
		NumComments:   nullInt(in.NumComments),
		NumErrors:     nullInt(in.NumErrors),
		NumAdvisories: nullInt(in.NumAdvisories),
		NumSkipped:    nullInt(in.NumSkipped),
		ToolVersion:   in.ToolVersion,
		SchemaVersion: in.SchemaVersion,
	})
	if err != nil {
		return fmt.Errorf("recording run: %w", err)
	}
	return nil
}

// nullString turns "" into SQL NULL, and anything else into itself.
func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// nullInt turns a nil pointer into SQL NULL, and a non-nil one into its
// value.
func nullInt(n *int) sql.NullInt64 {
	if n == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*n), Valid: true}
}
