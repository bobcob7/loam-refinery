package validate

import (
	"context"
	"log/slog"

	"github.com/bobcob7/refinery/internal/verify"
)

// GitFinder discovers the git repository containing the working directory.
type GitFinder struct {
	log *slog.Logger
}

// NewGitFinder returns a finder that walks up from the working directory.
func NewGitFinder(log *slog.Logger) *GitFinder {
	return &GitFinder{log: log}
}

// Find returns a verifier for the repository containing dir.
func (f *GitFinder) Find(ctx context.Context, dir string) (verifier, error) {
	repository, err := verify.Discover(ctx, dir)
	if err != nil {
		return nil, err
	}
	f.log.Debug("repository discovered", "root", repository.Root())
	return verify.New(repository, f.log), nil
}
