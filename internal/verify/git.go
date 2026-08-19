package verify

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// gitTimeout bounds one git call. Every call here is local and object-store
// bound, so a slow one means something is wrong rather than something is big,
// and a review tool must not hang a caller's pipeline waiting on it.
const gitTimeout = 30 * time.Second

// errNoRepository means the working directory is not inside a git repository.
var errNoRepository = errors.New("not a git repository")

// Repository is a git repository discovered from the working directory.
type Repository struct {
	root string
}

// Discover finds the repository containing dir the way git itself does, by
// walking up until a repository root is found. It never touches the network.
func Discover(ctx context.Context, dir string) (*Repository, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		if !answered(err) {
			return nil, fmt.Errorf("running git: %w", err)
		}
		return nil, errNoRepository
	}
	return &Repository{root: strings.TrimSpace(string(out))}, nil
}

// answered reports whether git ran and said no, as opposed to never running at
// all. A non-zero exit is a statement about the repository and belongs in the
// review; a missing binary or a cancelled context is a fact about this machine
// and must not be reported as a finding against the document.
func answered(err error) bool {
	var exit *exec.ExitError
	return errors.As(err, &exit)
}

// Root is the absolute path of the repository root.
func (r *Repository) Root() string {
	return r.root
}

func (r *Repository) run(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", r.root}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}
