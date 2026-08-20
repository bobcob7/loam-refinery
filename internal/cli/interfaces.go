package cli

import (
	"context"
	"io"

	"github.com/bobcob7/loam-refinery/internal/entry"
	"github.com/bobcob7/loam-refinery/internal/review"
	"github.com/bobcob7/loam-refinery/internal/validate"
)

//go:generate moq -out moq_test.go . documentValidator renderer entryRegistry documentStore

// documentValidator runs every check tier over one document.
type documentValidator interface {
	Validate(ctx context.Context, source []byte, options validate.Options) (*review.Result, error)
}

// StoreInput is what validate has learned about one run by the time storing
// happens (docs/config.md §5). Ref and Verdict are read straight off the
// submitted document, unvalidated — a documentStore applies whatever rules
// keep them safe to record, the same way it applies docs/config.md §4.8 to
// the repository name.
type StoreInput struct {
	// Dir is where repository identity is resolved from (docs/config.md
	// §4.2), the same directory verification already starts from.
	Dir string
	// Source is the exact bytes submitted, addressed and stored verbatim.
	Source []byte
	// Valid says whether this run exits 0 or 1 — which tree, if either,
	// keeps the bytes (docs/config.md §5).
	Valid bool
	// Ref and Verdict come from the document as submitted; both are "" when
	// the field was absent or not a string.
	Ref     string
	Verdict string
	// Comments, Errors, Advisories, and Skipped are the run's counts,
	// always known by the time storing happens.
	Comments   int
	Errors     int
	Advisories int
	Skipped    int
	// ToolVersion and SchemaVersion are recorded on every row.
	ToolVersion   string
	SchemaVersion string
}

// documentStore persists one run per docs/config.md §5: exit 0 keeps the
// review, exit 1 keeps the input unless it is oversized, and every call
// records a row. A nil error covers both a run that stored something and one
// with nothing to keep — store.enabled:false, most likely — so validate
// cannot tell the two apart and does not need to. A non-nil error means the
// store could not be established, written, or recorded to; the caller exits
// ExitTool with nothing on stdout (docs/config.md §5.1).
type documentStore interface {
	Save(ctx context.Context, in StoreInput) error
}

// renderer writes everything a command has to say. There is one implementation
// and one format: two of either is how the same run came to be described two
// different ways, which is the defect this interface no longer permits.
type renderer interface {
	Result(w io.Writer, result *review.Result) error
	Entries(w io.Writer, entries []entry.Entry) error
	Index(w io.Writer, groups []entry.Group) error
	Summary(w io.Writer, text string, groups []entry.Group) error
}

// entryRegistry resolves lens names to entries.
type entryRegistry interface {
	Resolve(name string) (entry.Entry, error)
	Index() []entry.Group
}
