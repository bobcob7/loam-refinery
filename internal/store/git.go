package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/bobcob7/loam-refinery/internal/verify"
)

// gitTimeout bounds one git call, matching internal/verify's reasoning: a
// call here is local and object-store bound, so a slow one means something
// is wrong rather than something is big.
const gitTimeout = 30 * time.Second

// waitDelay bounds the wait after a kill, for the same reason
// internal/verify carries one: Wait blocks on the pipes a descendant
// process may have inherited.
const waitDelay = 5 * time.Second

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
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.Exited() {
		return "", nil
	}
	return "", fmt.Errorf("reading origin remote: %w", err)
}

func (g *Git) run(ctx context.Context, root string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	cmd.Env = plainEnv()
	cmd.WaitDelay = waitDelay
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return stdout.Bytes(), nil
}

// plainEnv gives git an environment whose answers do not depend on the
// caller's locale, matching internal/verify's reasoning for the same
// variables.
func plainEnv() []string {
	env := make([]string, 0, len(os.Environ())+3)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "GIT_TRACE") || strings.HasPrefix(entry, "GIT_CURL_VERBOSE") {
			continue
		}
		env = append(env, entry)
	}
	return append(env, "LC_ALL=C", "LANGUAGE=", "GIT_NO_LAZY_FETCH=1")
}
