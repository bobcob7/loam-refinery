package collect

import "context"

//go:generate moq -out moq_test.go . reader

// reader is what Assemble needs to read one distinct digest's stored
// bytes, defined here at the consumer rather than in internal/store, per
// this project's "interfaces at the consumer" rule. This package must not
// import internal/store: that is now an acceptance criterion of
// refinery-uyb.9, not a suggestion, precisely so this package's own tests
// — the docs/features/combined-reviews.md section 12 fixtures — run as
// plain Go values, with no temp dir and no SQLite file. The caller (the
// CLI-wiring bead) adapts a *store.Store to this interface by composing
// its already-exported ReviewPath and ReadContent methods.
type reader interface {
	// ReadReview returns the raw bytes stored under digest, for whichever
	// repo and ref Assemble was called about. A missing or corrupted file
	// is reported through the returned error; Assemble treats that as a
	// skip-and-count against Result.Unreadable, never as a fatal error
	// (combined-reviews.md section 9's "skipped and counted, not fatal").
	ReadReview(ctx context.Context, digest string) ([]byte, error)
}
