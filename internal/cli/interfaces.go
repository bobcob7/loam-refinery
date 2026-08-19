package cli

import (
	"context"
	"io"

	"github.com/bobcob7/refinery/internal/entry"
	"github.com/bobcob7/refinery/internal/review"
	"github.com/bobcob7/refinery/internal/validate"
)

//go:generate moq -out moq_test.go . documentValidator renderer entryRegistry

// documentValidator runs every check tier over one document.
type documentValidator interface {
	Validate(ctx context.Context, source []byte, options validate.Options) (*review.Result, error)
}

// renderer writes everything a command has to say. There is one implementation
// and one format: two of either is how the same run came to be described two
// different ways, which is the defect this interface no longer permits.
type renderer interface {
	Result(stdout, stderr io.Writer, result *review.Result) error
	Entries(w io.Writer, entries []entry.Entry) error
	Index(w io.Writer, groups []entry.Group) error
	Summary(w io.Writer, text string, groups []entry.Group) error
}

// entryRegistry resolves lens names to entries.
type entryRegistry interface {
	Resolve(name string) (entry.Entry, error)
	Index() []entry.Group
}
