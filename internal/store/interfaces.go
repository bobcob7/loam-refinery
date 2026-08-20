package store

import (
	"context"
	"time"
)

//go:generate moq -out moq_test.go . clock gitRunner

// clock supplies the current time for a run's "at" column (config.md
// section 4.5.1). A real clock is used in production; a test pins it so a
// recorded row is deterministic.
type clock interface {
	Now() time.Time
}

// gitRunner resolves the two facts repository naming needs from git
// (config.md section 4.2): the worktree root containing a directory, and
// its origin remote, if it has one. It is the only way this package
// touches a repository.
type gitRunner interface {
	// root returns the absolute worktree root containing dir. It returns
	// verify.ErrNoRepository when dir is not inside a git repository.
	root(ctx context.Context, dir string) (string, error)
	// originURL returns the raw origin remote URL configured at root, or ""
	// when no origin is configured. Normalization happens in this package,
	// not here.
	originURL(ctx context.Context, root string) (string, error)
}
