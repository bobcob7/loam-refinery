package verify

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// errNoRepository means the working directory is not inside a git repository.
var errNoRepository = errors.New("not a git repository")

// Repository is a git repository discovered from the working directory.
type Repository struct {
	root string
}

// Discover finds the repository containing dir the way git itself does, by
// walking up until a repository root is found. It never touches the network.
func Discover(ctx context.Context, dir string) (*Repository, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, errNoRepository
	}
	return &Repository{root: strings.TrimSpace(string(out))}, nil
}

// Root is the absolute path of the repository root.
func (r *Repository) Root() string {
	return r.root
}

func (r *Repository) run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", r.root}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}
