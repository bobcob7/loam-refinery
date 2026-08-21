package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/bobcob7/loam-refinery/internal/verify"
)

// errGitTimedOut marks a git invocation that ended because verify.GitTimeout
// passed rather than because git decided anything. exec.ExitError.Exited()
// cannot be trusted to tell those apart on its own: it is hardcoded true on
// Windows even for a process this package killed, so once the deadline has
// passed the exit status is not evidence of anything — see internal/verify's
// identical reasoning for exitStatus.
var errGitTimedOut = errors.New("git did not answer")

// Git resolves repository identity by consulting git. Worktree discovery
// reuses internal/verify's, since duplicating its "not a repository"
// detection would be the one part of this package worth getting wrong
// twice; the origin remote is looked up directly, since verify has no need
// of it.
type Git struct {
	log *slog.Logger
}

// NewGit returns a Git that logs discovery at debug level.
func NewGit(log *slog.Logger) *Git {
	return &Git{log: log}
}

// root returns the worktree root containing dir, or verify.ErrNoRepository
// when dir is not inside a git repository.
func (g *Git) root(ctx context.Context, dir string) (string, error) {
	repo, err := verify.Discover(ctx, dir)
	if err != nil {
		return "", err
	}
	g.log.Debug("repository discovered", "root", repo.Root())
	return repo.Root(), nil
}

// originURL returns the origin remote configured at root, or "" when none
// is set. A git failure that is not simply "no such key" — a missing
// binary, a cancelled context — is returned as an error rather than read as
// absence, on the same reasoning internal/verify applies to discovery.
func (g *Git) originURL(ctx context.Context, root string) (string, error) {
	out, err := g.run(ctx, root, "config", "--get", "remote.origin.url")
	if err == nil {
		return strings.TrimSpace(string(out)), nil
	}
	return classifyOriginErr(err)
}

// classifyOriginErr turns originURL's failure into either an absent remote
// or an error. errGitTimedOut is checked first and unconditionally: once
// the deadline has passed, whatever exec.ExitError.Exited() reports cannot
// be trusted, so a timeout is never read as git having looked and found
// nothing. Only once a timeout is ruled out does an exited process read as
// an absent remote.
func classifyOriginErr(err error) (string, error) {
	if errors.Is(err, errGitTimedOut) {
		return "", fmt.Errorf("reading origin remote: %w", err)
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.Exited() {
		return "", nil
	}
	return "", fmt.Errorf("reading origin remote: %w", err)
}

func (g *Git) run(ctx context.Context, root string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, verify.GitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	cmd.Env = verify.PlainEnv()
	cmd.WaitDelay = verify.WaitDelay
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%w: %w", errGitTimedOut, err)
		}
		return nil, err
	}
	return stdout.Bytes(), nil
}
