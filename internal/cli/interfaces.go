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

// renderer writes results and entries in one output format.
type renderer interface {
	Result(stdout, stderr io.Writer, result *review.Result) error
	Entries(w io.Writer, entries []entry.Entry) error
	Index(w io.Writer, groups []entry.Group) error
}

// entryRegistry resolves lens names to entries.
type entryRegistry interface {
	Resolve(name string) (entry.Entry, error)
	Index() []entry.Group
}
