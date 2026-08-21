// Reading the store (config.md section 6), the query methods the reviews
// command needs. Every method here answers from store.db.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/bobcob7/loam-refinery/internal/store/sqlc"
)

// unlimited is what a caller's --limit=0 becomes for the query layer: SQL's
// LIMIT 0 returns nothing, so "no limit" is expressed as LIMIT -1 instead
// (a SQLite idiom the schema does not encode). Callers of this package pass
// 0 for "no limit"; this package owns the translation.
const unlimited = -1

// Counts is the four counters a run may carry (config.md section 4.5.1).
// Each is nil when the row has none.
type Counts struct {
	Comments   *int
	Errors     *int
	Advisories *int
	Skipped    *int
}

// Review is one row from the reviews half of the log: a run that stored a
// passing review. Assessment is "" when the row has none — optional, like
// the field it came from (config.md section 4.5.1), not every passing run
// carries one the way every passing run carries a Verdict.
type Review struct {
	At         time.Time
	Ref        string
	Digest     string
	Verdict    string
	Assessment string
	Counts     Counts
	Path       string
}

// FailedRun is one row from the other half: a run that stored no review.
// Ref is "" when the run has none to report — a caller renders that by
// omitting the field, not by publishing a zero value. Path is always set
// for an exit-1 row: every rejected input is kept, truncated to its first
// 1 MiB when it was larger (config.md section 4.4.1). It is computed from
// the row rather than stored, and its presence is not a claim that the
// file is still there or that it holds the whole input.
type FailedRun struct {
	At       time.Time
	Ref      string
	ExitCode int
	Counts   Counts
	Path     string
}

// RepoCount is one repository's row for --list: its name and how many
// reviews and failed runs the store holds for it.
type RepoCount struct {
	Name    string
	Reviews int
	Failed  int
}

// DigestRow is one distinct digest for a repo and ref, per
// combined-reviews.md section 5.3.1: At is the earliest at among every row
// that shares the digest, not any one row's own timestamp.
type DigestRow struct {
	Digest string
	At     time.Time
}

// ListReviews returns up to limit passing runs for repo, newest first, and
// the total number matching before limit was applied (config.md section
// 6.1). ref, when non-empty, restricts the result to that commit; limit of
// 0 means unlimited.
//
// Against a store still at schema version 1 (s.hasAssessment false), this
// runs ListReviewsLegacy instead of ListReviews — the same rows, from a
// query that does not name the assessment column, because a version-1
// database has none (refinery-xij). legacyReviewRun upgrades each row to
// sqlc.Run shape with Assessment left invalid, so every row-to-Review
// conversion below runs unmodified regardless of which query produced it.
func (s *Store) ListReviews(ctx context.Context, repo, ref string, limit int) ([]Review, int, error) {
	arg := nullString(ref)
	var rows []sqlc.Run
	var err error
	if s.hasAssessment {
		rows, err = s.queries.ListReviews(ctx, sqlc.ListReviewsParams{Repo: repo, Ref: arg, Limit: int64(sqlLimit(limit))})
	} else {
		var legacy []sqlc.ListReviewsLegacyRow
		legacy, err = s.queries.ListReviewsLegacy(ctx, sqlc.ListReviewsLegacyParams{Repo: repo, Ref: arg, Limit: int64(sqlLimit(limit))})
		rows = make([]sqlc.Run, 0, len(legacy))
		for _, row := range legacy {
			rows = append(rows, legacyReviewRun(row))
		}
	}
	if err != nil {
		return nil, 0, fmt.Errorf("listing reviews: %w", err)
	}
	total, err := s.queries.CountReviews(ctx, sqlc.CountReviewsParams{Repo: repo, Ref: arg})
	if err != nil {
		return nil, 0, fmt.Errorf("counting reviews: %w", err)
	}
	reviews := make([]Review, 0, len(rows))
	for _, row := range rows {
		at, err := time.Parse(atLayout, row.At)
		if err != nil {
			return nil, 0, fmt.Errorf("parsing run %d's timestamp: %w", row.ID, err)
		}
		digest := row.Digest
		reviews = append(reviews, Review{
			At:         at,
			Ref:        row.Ref.String,
			Digest:     digest,
			Verdict:    row.Verdict.String,
			Assessment: row.Assessment.String,
			Counts:     countsOf(row),
			Path:       s.ReviewPath(repo, row.Ref.String, digest),
		})
	}
	return reviews, int(total), nil
}

// DistinctDigests returns one row per distinct digest among passing runs
// for repo and ref, oldest first, each row's At the earliest at among
// every row sharing that digest (combined-reviews.md section 5.3.1). A
// repo and ref with no matching rows returns an empty slice and no error.
func (s *Store) DistinctDigests(ctx context.Context, repo, ref string) ([]DigestRow, error) {
	rows, err := s.queries.ListDistinctDigests(ctx, sqlc.ListDistinctDigestsParams{Repo: repo, Ref: nullString(ref)})
	if err != nil {
		return nil, fmt.Errorf("listing distinct digests: %w", err)
	}
	digests := make([]DigestRow, 0, len(rows))
	for _, row := range rows {
		at, err := time.Parse(atLayout, row.At)
		if err != nil {
			return nil, fmt.Errorf("parsing digest %s's timestamp: %w", row.Digest, err)
		}
		digests = append(digests, DigestRow{Digest: row.Digest, At: at})
	}
	return digests, nil
}

// ListFailedRuns returns up to limit runs for repo that stored no review,
// newest first, and the total number matching before limit was applied.
// ref and limit behave as in ListReviews, including the schema-version-1
// fallback to ListFailedRunsLegacy (see ListReviews's doc comment).
func (s *Store) ListFailedRuns(ctx context.Context, repo, ref string, limit int) ([]FailedRun, int, error) {
	arg := nullString(ref)
	var rows []sqlc.Run
	var err error
	if s.hasAssessment {
		rows, err = s.queries.ListFailedRuns(ctx, sqlc.ListFailedRunsParams{Repo: repo, Ref: arg, Limit: int64(sqlLimit(limit))})
	} else {
		var legacy []sqlc.ListFailedRunsLegacyRow
		legacy, err = s.queries.ListFailedRunsLegacy(ctx, sqlc.ListFailedRunsLegacyParams{Repo: repo, Ref: arg, Limit: int64(sqlLimit(limit))})
		rows = make([]sqlc.Run, 0, len(legacy))
		for _, row := range legacy {
			rows = append(rows, legacyFailedRun(row))
		}
	}
	if err != nil {
		return nil, 0, fmt.Errorf("listing failed runs: %w", err)
	}
	total, err := s.queries.CountFailedRuns(ctx, sqlc.CountFailedRunsParams{Repo: repo, Ref: arg})
	if err != nil {
		return nil, 0, fmt.Errorf("counting failed runs: %w", err)
	}
	failed := make([]FailedRun, 0, len(rows))
	for _, row := range rows {
		at, err := time.Parse(atLayout, row.At)
		if err != nil {
			return nil, 0, fmt.Errorf("parsing run %d's timestamp: %w", row.ID, err)
		}
		// Every rejected input is now kept, truncated to its first 1 MiB
		// when it was larger (config.md section 4.4.1), so an exit-1 row
		// always has a file and Path is always computed for it — no stat
		// needed to decide whether to omit it. Exit 3 is exactly such a row
		// (config.md section 4.5.1): its precondition fires before a
		// document is examined, so there is nothing to point at, and it
		// falls through this guard with an empty Path, same as today.
		path := ""
		if row.ExitCode == 1 {
			path = s.RejectedPath(repo, row.Digest)
		}
		failed = append(failed, FailedRun{At: at, Ref: row.Ref.String, ExitCode: int(row.ExitCode), Counts: countsOf(row), Path: path})
	}
	return failed, int(total), nil
}

// ListRepos returns every repository the store has a row for, with its
// review and failed-run counts, ordered by name (config.md section 6.1,
// --list).
func (s *Store) ListRepos(ctx context.Context) ([]RepoCount, error) {
	rows, err := s.queries.ListRepoCounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing repositories: %w", err)
	}
	repos := make([]RepoCount, 0, len(rows))
	for _, row := range rows {
		repos = append(repos, RepoCount{Name: row.Repo, Reviews: int(row.Reviews.Float64), Failed: int(row.Failed.Float64)})
	}
	return repos, nil
}

// Known reports whether the store has any row at all for repo, so a caller
// can distinguish a mistyped repository from one the store knows but has
// nothing recent for (config.md section 6.2).
func (s *Store) Known(ctx context.Context, repo string) (bool, error) {
	known, err := s.queries.RepoKnown(ctx, repo)
	if err != nil {
		return false, fmt.Errorf("checking repository %q: %w", repo, err)
	}
	return known, nil
}

// sqlLimit translates a caller's --limit=0 ("unlimited") into the value
// SQL's LIMIT needs to mean the same thing: SQLite treats LIMIT 0 as "zero
// rows", so unlimited is spelled -1 instead. Any positive N passes through
// unchanged.
func sqlLimit(limit int) int {
	if limit <= 0 {
		return unlimited
	}
	return limit
}

// legacyReviewRun upgrades a ListReviewsLegacy row to sqlc.Run shape.
// Assessment is left at its zero value (an invalid sql.NullString), which
// is exactly what every caller downstream already treats as absent — the
// same state a NULL assessment column reads back as on a migrated store.
func legacyReviewRun(row sqlc.ListReviewsLegacyRow) sqlc.Run {
	return sqlc.Run{
		ID:            row.ID,
		At:            row.At,
		Repo:          row.Repo,
		Ref:           row.Ref,
		Digest:        row.Digest,
		ExitCode:      row.ExitCode,
		Verdict:       row.Verdict,
		NumComments:   row.NumComments,
		NumErrors:     row.NumErrors,
		NumAdvisories: row.NumAdvisories,
		NumSkipped:    row.NumSkipped,
		ToolVersion:   row.ToolVersion,
		SchemaVersion: row.SchemaVersion,
	}
}

// legacyFailedRun is legacyReviewRun for a ListFailedRunsLegacy row.
func legacyFailedRun(row sqlc.ListFailedRunsLegacyRow) sqlc.Run {
	return sqlc.Run{
		ID:            row.ID,
		At:            row.At,
		Repo:          row.Repo,
		Ref:           row.Ref,
		Digest:        row.Digest,
		ExitCode:      row.ExitCode,
		Verdict:       row.Verdict,
		NumComments:   row.NumComments,
		NumErrors:     row.NumErrors,
		NumAdvisories: row.NumAdvisories,
		NumSkipped:    row.NumSkipped,
		ToolVersion:   row.ToolVersion,
		SchemaVersion: row.SchemaVersion,
	}
}

// countsOf reads the four nullable counters off a runs row.
func countsOf(row sqlc.Run) Counts {
	return Counts{
		Comments:   intOrNil(row.NumComments),
		Errors:     intOrNil(row.NumErrors),
		Advisories: intOrNil(row.NumAdvisories),
		Skipped:    intOrNil(row.NumSkipped),
	}
}

// intOrNil turns a SQL NULL into a nil pointer, and a value into a pointer
// to it.
func intOrNil(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}
