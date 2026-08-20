package verify

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// gitError is a git call that failed, carrying what git said on stderr. The
// status alone is not enough to tell a refusal from a complaint: cat-file -e
// exits 1 both for an object that is absent and for one it could not look for,
// and only the silence of stderr separates them.
type gitError struct {
	args   []string
	stderr string
	err    error
}

// Error keeps only the sentence git led with. The whole message can run to
// several lines, and this one ends up in a Reason a caller reads as one string;
// the rest is advice for a person and is already on stderr.
func (e *gitError) Error() string {
	if e.stderr == "" {
		return fmt.Sprintf("git %s: %v", strings.Join(e.args, " "), e.err)
	}
	return fmt.Sprintf("git %s: %v: %s", strings.Join(e.args, " "), e.err, firstLine(e.stderr))
}

func (e *gitError) Unwrap() error { return e.err }

// stderrOf returns what git wrote to stderr before failing, or "" when it wrote
// nothing. An empty result is a claim about git's silence, so anything that is
// not a recognised git failure reports empty rather than guessing.
func stderrOf(err error) string {
	var failure *gitError
	if errors.As(err, &failure) {
		return failure.stderr
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return strings.TrimSpace(string(exit.Stderr))
	}
	return ""
}

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

// plainEnv gives git an environment whose answers mean what they say.
//
// LC_ALL and LANGUAGE pin the untranslated messages: discovery decides whether
// a directory is a repository by reading what git said, and a German checkout
// would otherwise answer in a sentence no match here recognises.
//
// GIT_NO_LAZY_FETCH stops a partial clone reaching for the network when it is
// asked about an object it does not have. Absence is read from git's silence,
// and a lazy fetch turns the plainest absence there is into a fatal error about
// a remote — which would be read as "could not look" and skipped, letting a
// review name a commit that exists nowhere and still pass.
//
// The trace variables are dropped because they write to the same stderr the
// silence is read from. Inheriting one from the caller's shell would make every
// absent object look unreadable.
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

// complained reports whether git raised a problem, as opposed to saying nothing
// or merely warning. The two prefixes are git's own severity markers and are
// not translated, so this holds wherever it runs. It is the backstop under
// plainEnv: silence carries the claim, and anything git calls an error or a
// fatal withdraws it, whatever else may have reached stderr.
func complained(stderr string) bool {
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "error:") || strings.HasPrefix(line, "fatal:") {
			return true
		}
	}
	return false
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
		failure := &gitError{args: args, stderr: strings.TrimSpace(stderr.String()), err: err}
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%w: %w", errNotAnswered, failure)
		}
		return nil, failure
	}
	return stdout.Bytes(), nil
}

// worktreeDiverged reports whether path's working-tree copy exists and
// differs from the blob ref names, using git's own comparison — git
// hash-object on the worktree file against git rev-parse on the blob at
// ref — so .gitattributes, core.autocrlf, and any clean/smudge filter are
// honored exactly as they are everywhere else in the user's tooling. A raw
// byte comparison would invent a second definition of "changed" that
// disagrees with git's own: under core.autocrlf=true it would see line-ending
// normalization as a difference and call every tracked file diverged.
//
// A path with no working-tree copy is reported not diverged, never as an
// error: a deleted file says nothing about what a reviewer read, so ref
// stays authoritative for it. This also keeps the answer disjoint from
// "changed" — git itself reports a deletion as a difference, and this
// function deliberately does not.
//
// It does not check that ref is HEAD; that restriction belongs to whoever
// decides when the comparison is meaningful, not to the comparison itself.
func (r *Repository) worktreeDiverged(ctx context.Context, ref, path string) (bool, error) {
	_, err := os.Lstat(filepath.Join(r.root, filepath.FromSlash(path)))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("statting working-tree copy of %s: %w", path, err)
	}
	blob, err := r.run(ctx, "rev-parse", "--verify", ref+":"+path)
	if err != nil {
		return false, fmt.Errorf("resolving %s at %s: %w", path, short(ref), err)
	}
	worktree, err := r.run(ctx, "hash-object", "--", path)
	if err != nil {
		return false, fmt.Errorf("hashing working-tree copy of %s: %w", path, err)
	}
	return strings.TrimSpace(string(blob)) != strings.TrimSpace(string(worktree)), nil
}
