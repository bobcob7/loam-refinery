package verify

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// gitTimeout bounds one git call. Every call here is local and object-store
// bound, so a slow one means something is wrong rather than something is big,
// and a review tool must not hang a caller's pipeline waiting on it.
const gitTimeout = 30 * time.Second

// waitDelay bounds the wait after the kill. Killing the process is not enough
// on its own: Wait still blocks on the goroutines copying git's pipes, and a
// descendant that inherited them — a hook, a credential helper, a pager — holds
// them open after git itself is gone. Without this the call outlives gitTimeout
// indefinitely, and a hang is the one outcome the exit-code contract cannot
// express.
const waitDelay = 5 * time.Second

// errNotAnswered marks a git call that ended because the deadline passed rather
// than because git decided anything. ProcessState.Exited() catches this on Unix
// by reporting a signalled process, but it is hardcoded to true on Windows, so
// the context is asked directly and the answer holds on every platform.
var errNotAnswered = errors.New("git did not answer")

// ErrNoRepository means the working directory is not inside a git repository.
// It is exported because it is the one discovery failure a caller may treat as
// ordinary and go on without: every other way discovery fails is this machine
// failing, and skipping the anchor checks for those would pass a document
// nobody ever checked.
var ErrNoRepository = errors.New("not a git repository")

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
	cmd.Env = plainEnv()
	cmd.WaitDelay = waitDelay
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%w: running git: %w", errNotAnswered, err)
		}
		return nil, discoveryFailure(err)
	}
	return &Repository{root: strings.TrimSpace(string(out))}, nil
}

// discoveryFailure separates the one answer that means there is no repository
// here from every other way rev-parse can fail. Git exits 128 both for a plain
// directory and for a checkout it refuses to touch — dubious ownership, a bare
// repository — so the exit status alone cannot tell them apart, and reading a
// refusal as an absent repository skips every anchor check and passes the
// document while standing inside the repository that would have refuted it.
func discoveryFailure(err error) error {
	var exit *exec.ExitError
	if !errors.As(err, &exit) || !exit.Exited() {
		return fmt.Errorf("running git: %w", err)
	}
	// Matching git's own sentence is the only signal there is; the exit status
	// is 128 for both. Note the one case it cannot separate: a .git that exists
	// but cannot be read produces this identical message, so an unreadable
	// checkout reports absence. That mis-states the reason but not the outcome,
	// since neither verifies anything.
	refusal := firstLine(string(exit.Stderr))
	if strings.Contains(refusal, "not a git repository") {
		return ErrNoRepository
	}
	if refusal == "" {
		return fmt.Errorf("git could not identify a repository here: %w", err)
	}
	return fmt.Errorf("git could not identify a repository here: %s", refusal)
}

// plainEnv pins git to its untranslated messages. Discovery decides whether a
// directory is a repository by reading what git said, and a German or Japanese
// checkout would otherwise answer in a sentence no match here recognises.
func plainEnv() []string {
	return append(os.Environ(), "LC_ALL=C", "LANGUAGE=")
}

// firstLine keeps the sentence git leads with. Its later lines are advice for a
// person, and this one is going into a machine-readable reason.
func firstLine(text string) string {
	text = strings.TrimSpace(text)
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		return strings.TrimSpace(text[:i])
	}
	return text
}

// answered reports whether git ran and said no, as opposed to never running at
// all. A non-zero exit is a statement about the repository and belongs in the
// review; a missing binary or a cancelled context is a fact about this machine
// and must not be reported as a finding against the document.
func answered(err error) bool {
	_, ok := exitStatus(err)
	return ok
}

// exitStatus returns the status git exited with, and false when it never
// exited at all. Exited() is the distinction a killed process fails: it also
// arrives as an ExitError, carrying exit code -1 and no opinion whatsoever
// about the repository.
func exitStatus(err error) (int, bool) {
	var exit *exec.ExitError
	if errors.Is(err, errNotAnswered) || !errors.As(err, &exit) || !exit.Exited() {
		return 0, false
	}
	return exit.ExitCode(), true
}

// Root is the absolute path of the repository root.
func (r *Repository) Root() string {
	return r.root
}

func (r *Repository) run(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", r.root}, args...)...)
	cmd.Env = plainEnv()
	cmd.WaitDelay = waitDelay
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		failure := fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%w: %w", errNotAnswered, failure)
		}
		return nil, failure
	}
	return stdout.Bytes(), nil
}
