package verify

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bobcob7/loam-refinery/internal/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const absentSHA = "0123456789abcdef0123456789abcdef01234567"

func TestVerifyChecksAnchorsAgainstARealRepository(t *testing.T) {
	t.Parallel()
	repository, sha := repo(t)
	tests := []struct {
		name     string
		anchors  []map[string]any
		want     string
		message  string
		verified int
	}{
		{
			name:     "a file and a line that exist verify",
			anchors:  []map[string]any{{"file": "internal/fetch/client.go", "line": 12, "end_line": 20}},
			verified: 1,
		},
		{
			name:    "a path that does not exist at the ref",
			anchors: []map[string]any{{"file": "internal/fetch/missing.go", "line": 1}},
			want:    "anchor-file-missing",
			message: fmt.Sprintf("internal/fetch/missing.go does not exist at %s", sha[:7]),
		},
		{
			name:    "a directory is not a file",
			anchors: []map[string]any{{"file": "internal/fetch"}},
			want:    "anchor-file-missing",
			message: fmt.Sprintf("internal/fetch is a directory at %s, not a file", sha[:7]),
		},
		{
			name:    "a line past the end of the file",
			anchors: []map[string]any{{"file": "internal/fetch/client.go", "line": 900}},
			want:    "anchor-line-out-of-range",
			message: fmt.Sprintf("line 900 is out of range in a 61-line file at %s", sha[:7]),
		},
		{
			name:    "an end_line past the end of the file",
			anchors: []map[string]any{{"file": "internal/fetch/client.go", "line": 60, "end_line": 62}},
			want:    "anchor-line-out-of-range",
			message: fmt.Sprintf("end_line 62 is out of range in a 61-line file at %s", sha[:7]),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			doc := document(t, sha, test.anchors)
			diagnostics, skipped, verification := New(repository, logger()).Verify(t.Context(), doc)
			assert.Empty(t, skipped)
			assert.Equal(t, "repo", verification.Source)
			assert.Equal(t, len(test.anchors), verification.Anchors)
			assert.Equal(t, test.verified, verification.Verified)
			if test.want == "" {
				assert.Empty(t, diagnostics)
				return
			}
			require.Len(t, diagnostics, 1)
			assert.Equal(t, test.want, diagnostics[0].Name)
			assert.Equal(t, review.SeverityError, diagnostics[0].Severity)
			assert.Equal(t, test.message, diagnostics[0].Message)
		})
	}
}

func TestUnknownRefIsOneDiagnosticForTheDocument(t *testing.T) {
	t.Parallel()
	repository, _ := repo(t)
	doc := document(t, absentSHA, []map[string]any{
		{"file": "internal/fetch/client.go", "line": 1},
		{"file": "internal/fetch/client.go", "line": 2},
	})
	diagnostics, skipped, verification := New(repository, logger()).Verify(t.Context(), doc)
	require.Len(t, diagnostics, 1, "an unresolvable ref is one diagnostic, not one per anchor")
	assert.Equal(t, "ref-unknown", diagnostics[0].Name)
	assert.Equal(t, "/ref", diagnostics[0].Path)
	assert.Equal(t, "ref 0123456 does not resolve in this repository", diagnostics[0].Message)
	assert.Zero(t, verification.Verified)
	names := []string{}
	for _, skip := range skipped {
		names = append(names, skip.Name)
		assert.Equal(t, "the document ref does not resolve", skip.Reason)
	}
	assert.Equal(t, []string{"anchor-file-missing", "anchor-line-out-of-range"}, names,
		"the anchor checks never ran, and a caller must not read that as a pass")
}

func TestADocumentWithNoRefIsSkippedNotPassed(t *testing.T) {
	t.Parallel()
	repository, _ := repo(t)
	doc := document(t, "", []map[string]any{{"file": "internal/fetch/client.go", "line": 12}})
	diagnostics, skipped, verification := New(repository, logger()).Verify(t.Context(), doc)
	assert.Empty(t, diagnostics)
	assert.Equal(t, 1, verification.Anchors)
	assert.Zero(t, verification.Verified, "an unverifiable anchor is never counted as verified")
	names := []string{}
	for _, skip := range skipped {
		names = append(names, skip.Name)
		assert.Equal(t, "the document has no ref", skip.Reason)
	}
	assert.Equal(t, []string{"anchor-file-missing", "anchor-line-out-of-range"}, names)
}

func TestOneMalformedAnchorDoesNotStopTheOthers(t *testing.T) {
	t.Parallel()
	repository, sha := repo(t)
	doc := document(t, sha, []map[string]any{
		{"file": 12},
		{"file": "internal/fetch/client.go", "line": 12},
	})
	_, skipped, verification := New(repository, logger()).Verify(t.Context(), doc)
	assert.Equal(t, 1, verification.Verified, "one unusable anchor does not stop the other")
	require.NotEmpty(t, skipped)
	assert.Equal(t, "unusable field on 1 anchor", skipped[0].Reason)
}

func TestDiscoverFailsOutsideARepository(t *testing.T) {
	t.Parallel()
	_, err := Discover(t.Context(), t.TempDir())
	require.Error(t, err)
	assert.Equal(t, "not a git repository", err.Error())
}

// gitCall names which of the verifier's four calls this is, so a test can
// answer the one it is about without matching argument positions.
func gitCall(args []string) string {
	switch {
	case args[len(args)-1] == "-z" || contains(args, "ls-tree"):
		return "ls-tree"
	case args[1] == "-e":
		return "exists"
	case args[1] == "-t":
		return "type"
	default:
		return "blob"
	}
}

func contains(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

// treeEntryFor is what ls-tree -z prints for one blob at path.
func treeEntryFor(path string) []byte {
	return []byte("100644 blob " + strings.Repeat("a", 40) + "\t" + path + "\x00")
}

// answering builds a git that resolves the ref and finds every path as a blob,
// then lets a test override exactly one of the four calls.
func answering(override func(kind string, args []string) ([]byte, error, bool)) *gitRunnerMock {
	return &gitRunnerMock{
		runFunc: func(_ context.Context, args ...string) ([]byte, error) {
			kind := gitCall(args)
			if override != nil {
				if out, err, handled := override(kind, args); handled {
					return out, err
				}
			}
			switch kind {
			case "exists":
				return nil, nil
			case "type":
				return []byte("commit\n"), nil
			case "ls-tree":
				return treeEntryFor(args[len(args)-1]), nil
			default:
				return []byte("one\ntwo\nthree\nfour\n"), nil
			}
		},
		// No working-tree copy by default: a mocked git never has a real
		// checkout behind it, so a missing file stays a plain hallucination
		// unless a test opts into refinery-qu7's other case explicitly.
		worktreeExistsFunc: func(string) (bool, error) { return false, nil },
	}
}

func TestFileLookupsAreCachedPerRefAndPath(t *testing.T) {
	t.Parallel()
	git := answering(nil)
	doc := document(t, absentSHA, []map[string]any{
		{"file": "a.go", "line": 1},
		{"file": "a.go", "line": 2},
		{"file": "a.go", "line": 3},
	})
	_, _, verification := New(git, logger()).Verify(t.Context(), doc)
	assert.Equal(t, 3, verification.Verified)
	assert.Len(t, git.runCalls(), 5,
		"the ref resolves once, HEAD resolves once, then one tree and one blob read per distinct path")
}

func TestCountLinesTreatsATrailingFragmentAsALine(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 0, countLines(nil))
	assert.Equal(t, 1, countLines([]byte("one\n")))
	assert.Equal(t, 2, countLines([]byte("one\ntwo")))
	assert.Equal(t, 2, countLines([]byte("one\ntwo\n")))
}

// repo builds a throwaway repository holding one 61-line file and returns it
// with the SHA of its only commit.
func repo(t *testing.T) (*Repository, string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "internal", "fetch"), 0o755))
	lines := make([]string, 61)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i+1)
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "internal", "fetch", "client.go"),
		[]byte(strings.Join(lines, "\n")+"\n"), 0o644))
	run(t, dir, "init", "-q")
	run(t, dir, "add", "-A")
	run(t, dir, "-c", "user.email=test@example.com", "-c", "user.name=test", "commit", "-qm", "init")
	sha := strings.TrimSpace(run(t, dir, "rev-parse", "HEAD"))
	repository, err := Discover(t.Context(), dir)
	require.NoError(t, err)
	return repository, sha
}

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s: %s", strings.Join(args, " "), out)
	return string(out)
}

func document(t *testing.T, ref string, anchors []map[string]any) *review.Document {
	t.Helper()
	doc := &review.Document{}
	if ref != "" {
		doc.Ref = review.Field[string]{Value: ref, Present: true, OK: true}
	}
	comment := review.Comment{
		Index: 0, Path: "/comments/0", Object: true,
		ID: review.Field[string]{Value: "dropped-context-1", Present: true, OK: true},
	}
	for i, anchor := range anchors {
		parsed := review.Anchor{Index: i, Path: fmt.Sprintf("/comments/0/anchors/%d", i), Object: true}
		if file, ok := anchor["file"].(string); ok {
			parsed.File = review.Field[string]{Value: file, Present: true, OK: true}
		} else if _, present := anchor["file"]; present {
			parsed.File = review.Field[string]{Present: true}
		}
		if line, ok := anchor["line"].(int); ok {
			parsed.Line = review.Field[int]{Value: line, Present: true, OK: true}
		}
		if end, ok := anchor["end_line"].(int); ok {
			parsed.EndLine = review.Field[int]{Value: end, Present: true, OK: true}
		}
		comment.Anchors = append(comment.Anchors, parsed)
	}
	doc.Comments = []review.Comment{comment}
	doc.CommentsArray = true
	doc.CommentsWellTyped = true
	return doc
}

func logger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func TestAnAnchorWithAnUnreadableLineIsNotCountedAsVerified(t *testing.T) {
	t.Parallel()
	repository, sha := repo(t)
	doc := document(t, sha, []map[string]any{{"file": "internal/fetch/client.go"}})
	doc.Comments[0].Anchors[0].Line = review.Field[int]{Present: true}
	diagnostics, skipped, verification := New(repository, logger()).Verify(t.Context(), doc)
	assert.Empty(t, diagnostics)
	assert.Zero(t, verification.Verified, "a line that cannot be read was never range checked")
	require.NotEmpty(t, skipped)
	assert.Equal(t, "unusable field on 1 anchor", skipped[0].Reason)
}

// gitFailure is what a git that never ran looks like: not an *exec.ExitError,
// so nothing about the repository was established.
var gitFailure = errors.New("exec: \"git\": executable file not found in $PATH")

func TestAFailedBlobReadIsSkippedNotReportedAsAZeroLineFile(t *testing.T) {
	t.Parallel()
	git := answering(func(kind string, _ []string) ([]byte, error, bool) {
		return nil, gitFailure, kind == "blob"
	})
	doc := document(t, absentSHA, []map[string]any{{"file": "a.go", "line": 12}})
	diagnostics, skipped, verification := New(git, logger()).Verify(t.Context(), doc)
	assert.Empty(t, diagnostics, "a git that could not answer says nothing about the review")
	assert.Zero(t, verification.Verified)
	require.NotEmpty(t, skipped)
	assert.Equal(t, "git could not read the file for 1 anchor", skipped[0].Reason)
}

func TestAFailedRefLookupIsSkippedNotReportedAsRefUnknown(t *testing.T) {
	t.Parallel()
	git := &gitRunnerMock{runFunc: func(_ context.Context, _ ...string) ([]byte, error) {
		return nil, gitFailure
	}}
	doc := document(t, absentSHA, []map[string]any{{"file": "a.go", "line": 12}})
	diagnostics, skipped, verification := New(git, logger()).Verify(t.Context(), doc)
	assert.Empty(t, diagnostics, "a ref that could not be looked up is not a ref that does not resolve")
	assert.Equal(t, "unavailable", verification.Source,
		"a repository that answered nothing is not a run made outside one")
	require.NotEmpty(t, skipped)
	assert.Equal(t, "git could not resolve the document ref", skipped[0].Reason)
}

// killedProcess is what a git call looks like when the timeout or the caller's
// context kills it: a real *exec.ExitError that carries no exit status at all.
// This is the shape that makes a broken machine look like an answer, so the
// tests below build a genuine one rather than a stand-in.
func killedProcess(t *testing.T) error {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	cmd := exec.CommandContext(ctx, "sleep", "30")
	require.NoError(t, cmd.Start())
	cancel()
	err := cmd.Wait()
	var exit *exec.ExitError
	require.ErrorAs(t, err, &exit)
	require.False(t, exit.Exited(), "a signalled process has no exit status")
	return err
}

// refused is a real non-zero exit: the command ran to completion and said no.
func refused(t *testing.T, code int) *exec.ExitError {
	t.Helper()
	err := exec.Command("sh", "-c", fmt.Sprintf("exit %d", code)).Run()
	var exit *exec.ExitError
	require.ErrorAs(t, err, &exit)
	require.True(t, exit.Exited())
	return exit
}

func TestAKilledGitIsNotReportedAsARefThatDoesNotResolve(t *testing.T) {
	t.Parallel()
	killed := killedProcess(t)
	git := &gitRunnerMock{runFunc: func(context.Context, ...string) ([]byte, error) { return nil, killed }}
	doc := document(t, absentSHA, []map[string]any{{"file": "a.go", "line": 12}})
	diagnostics, skipped, verification := New(git, logger()).Verify(t.Context(), doc)
	assert.Empty(t, diagnostics, "a git that was killed never said the ref was absent")
	assert.Equal(t, "unavailable", verification.Source,
		"a repository that answered nothing is not a run made outside one")
	require.NotEmpty(t, skipped)
	assert.Equal(t, "git could not resolve the document ref", skipped[0].Reason)
}

func TestAKilledGitIsNotReportedAsAMissingFile(t *testing.T) {
	t.Parallel()
	killed := killedProcess(t)
	git := answering(func(kind string, _ []string) ([]byte, error, bool) {
		return nil, killed, kind == "ls-tree"
	})
	doc := document(t, absentSHA, []map[string]any{{"file": "a.go", "line": 12}})
	diagnostics, skipped, verification := New(git, logger()).Verify(t.Context(), doc)
	assert.Empty(t, diagnostics, "a git that was killed never said the file was absent")
	assert.Zero(t, verification.Verified)
	require.NotEmpty(t, skipped)
	assert.Equal(t, "git could not read the file for 1 anchor", skipped[0].Reason)
}

// The other half of the contract: a real non-zero exit is git answering, and
// that answer belongs in the review.
func TestARealNonZeroExitIsAnAnswerAboutTheRepository(t *testing.T) {
	t.Parallel()
	t.Run("an absent ref is reported", func(t *testing.T) {
		t.Parallel()
		absent := refused(t, 1)
		git := answering(func(kind string, _ []string) ([]byte, error, bool) {
			return nil, absent, kind == "exists"
		})
		doc := document(t, absentSHA, []map[string]any{{"file": "a.go", "line": 12}})
		diagnostics, _, _ := New(git, logger()).Verify(t.Context(), doc)
		require.Len(t, diagnostics, 1)
		assert.Equal(t, "ref-unknown", diagnostics[0].Name)
	})
	t.Run("an absent file is reported", func(t *testing.T) {
		t.Parallel()
		git := answering(func(kind string, _ []string) ([]byte, error, bool) {
			return nil, nil, kind == "ls-tree"
		})
		doc := document(t, absentSHA, []map[string]any{{"file": "a.go", "line": 12}})
		diagnostics, _, _ := New(git, logger()).Verify(t.Context(), doc)
		require.Len(t, diagnostics, 1)
		assert.Equal(t, "anchor-file-missing", diagnostics[0].Name)
	})
}

func TestDiscoveryTellsAbsenceApartFromRefusal(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		stderr string
		absent bool
		reason string
	}{
		{
			name:   "no repository here is an ordinary answer",
			stderr: "fatal: not a git repository (or any of the parent directories): .git\n",
			absent: true,
		},
		{
			name:   "a refused checkout is not an absent repository",
			stderr: "fatal: detected dubious ownership in repository at '/src'\nTo add an exception run:\n  git config ...\n",
			reason: "git could not identify a repository here: fatal: detected dubious ownership in repository at '/src'",
		},
		{
			name:   "a bare repository is not an absent repository",
			stderr: "fatal: this operation must be run in a work tree\n",
			reason: "git could not identify a repository here: fatal: this operation must be run in a work tree",
		},
		{
			name:   "an exit with nothing to say is still not an absent repository",
			stderr: "",
			reason: "git could not identify a repository here",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			exit := refused(t, 128)
			exit.Stderr = []byte(test.stderr)
			err := discoveryFailure(exit)
			if test.absent {
				assert.ErrorIs(t, err, ErrNoRepository)
				return
			}
			assert.NotErrorIs(t, err, ErrNoRepository,
				"a refusal read as an absent repository skips every anchor check and passes the document")
			assert.Contains(t, err.Error(), test.reason)
		})
	}
}

func TestDiscoveryReportsAKilledGitAsAMachineFailure(t *testing.T) {
	t.Parallel()
	err := discoveryFailure(killedProcess(t))
	assert.NotErrorIs(t, err, ErrNoRepository)
	assert.Contains(t, err.Error(), "running git")
}

func TestDiscoverTellsAbsenceApartFromRefusalAgainstRealGit(t *testing.T) {
	t.Parallel()
	t.Run("outside a repository", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Run() == nil {
			t.Skip("the temp dir is itself inside a repository")
		}
		_, err := Discover(t.Context(), dir)
		assert.ErrorIs(t, err, ErrNoRepository)
	})
	t.Run("inside a repository", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, exec.Command("git", "-C", dir, "init", "--quiet").Run())
		repository, err := Discover(t.Context(), dir)
		require.NoError(t, err)
		assert.NotEmpty(t, repository.Root())
	})
	t.Run("a bare repository is a refusal, not an absence", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, exec.Command("git", "init", "--bare", "--quiet", dir).Run())
		_, err := Discover(t.Context(), dir)
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrNoRepository,
			"a bare repository read as an absent one would pass every anchor unchecked")
	})
}

// corrupt replaces an object's file with bytes git cannot inflate, which is
// what a bad disk or an interrupted write leaves behind. git then exits 128 —
// the same status it uses for "this is not here" — so nothing but the shape of
// the answer separates a broken store from an absent object.
func corrupt(t *testing.T, dir, object string) {
	t.Helper()
	path := filepath.Join(dir, ".git", "objects", object[:2], object[2:])
	require.NoError(t, os.Chmod(filepath.Dir(path), 0o755))
	require.NoError(t, os.Chmod(path, 0o644))
	require.NoError(t, os.WriteFile(path, []byte("not zlib"), 0o644))
}

// The bug this guards: a review whose anchors are all correct must not be
// refuted because the object store underneath it is damaged.
func TestAnUnreadableObjectIsSkippedNotReportedAsAbsent(t *testing.T) {
	t.Parallel()
	t.Run("a corrupt tree does not refute the anchor", func(t *testing.T) {
		t.Parallel()
		repository, sha := repo(t)
		tree := strings.TrimSpace(run(t, repository.Root(), "rev-parse", sha+"^{tree}"))
		corrupt(t, repository.Root(), tree)
		doc := document(t, sha, []map[string]any{{"file": "internal/fetch/client.go", "line": 12}})
		diagnostics, skipped, verification := New(repository, logger()).Verify(t.Context(), doc)
		assert.Empty(t, diagnostics, "a tree git cannot read says nothing about the anchor")
		assert.Zero(t, verification.Verified)
		require.NotEmpty(t, skipped)
		assert.Equal(t, "git could not read the file for 1 anchor", skipped[0].Reason)
	})
	t.Run("a corrupt blob does not refute the line range", func(t *testing.T) {
		t.Parallel()
		repository, sha := repo(t)
		blob := strings.TrimSpace(run(t, repository.Root(), "rev-parse", sha+":internal/fetch/client.go"))
		corrupt(t, repository.Root(), blob)
		doc := document(t, sha, []map[string]any{{"file": "internal/fetch/client.go", "line": 40}})
		diagnostics, skipped, verification := New(repository, logger()).Verify(t.Context(), doc)
		assert.Empty(t, diagnostics, "an unreadable blob is not a file with too few lines")
		assert.Zero(t, verification.Verified)
		require.NotEmpty(t, skipped)
	})
	t.Run("a corrupt commit does not refute the ref", func(t *testing.T) {
		t.Parallel()
		repository, sha := repo(t)
		corrupt(t, repository.Root(), sha)
		doc := document(t, sha, []map[string]any{{"file": "internal/fetch/client.go", "line": 12}})
		diagnostics, skipped, verification := New(repository, logger()).Verify(t.Context(), doc)
		assert.Empty(t, diagnostics, "a commit git cannot read is not a commit that does not resolve")
		assert.Equal(t, "unavailable", verification.Source)
		require.NotEmpty(t, skipped)
		assert.Equal(t, "git could not resolve the document ref", skipped[0].Reason)
	})
}

// The other half: against a healthy repository the absent cases are still
// reported, so the fix above did not buy its safety by checking nothing.
func TestAbsenceIsStillReportedAgainstAHealthyRepository(t *testing.T) {
	t.Parallel()
	repository, sha := repo(t)
	t.Run("an absent ref", func(t *testing.T) {
		t.Parallel()
		doc := document(t, absentSHA, []map[string]any{{"file": "internal/fetch/client.go"}})
		diagnostics, _, _ := New(repository, logger()).Verify(t.Context(), doc)
		require.Len(t, diagnostics, 1)
		assert.Equal(t, "ref-unknown", diagnostics[0].Name)
	})
	t.Run("an absent path", func(t *testing.T) {
		t.Parallel()
		doc := document(t, sha, []map[string]any{{"file": "internal/fetch/absent.go"}})
		diagnostics, _, _ := New(repository, logger()).Verify(t.Context(), doc)
		require.Len(t, diagnostics, 1)
		assert.Equal(t, "anchor-file-missing", diagnostics[0].Name)
	})
	t.Run("a path git spells differently is still found", func(t *testing.T) {
		t.Parallel()
		doc := document(t, sha, []map[string]any{{"file": "./internal/fetch/client.go", "line": 3}})
		diagnostics, _, verification := New(repository, logger()).Verify(t.Context(), doc)
		assert.Empty(t, diagnostics, "git answers with the path it stores, not the one it was handed")
		assert.Equal(t, 1, verification.Verified)
	})
	t.Run("pathspec magic is read as a name, not as a pattern", func(t *testing.T) {
		t.Parallel()
		doc := document(t, sha, []map[string]any{{"file": ":(glob)internal/fetch/client.go"}})
		diagnostics, skipped, _ := New(repository, logger()).Verify(t.Context(), doc)
		require.Len(t, diagnostics, 1)
		assert.Equal(t, "anchor-file-missing", diagnostics[0].Name,
			"a colon-prefixed anchor must be an absent file, not a failed git call")
		assert.Empty(t, skipped)
	})
}

// gitTimeout is only half a bound. Killing git does not close the pipes a
// descendant inherited, and Wait blocks on the goroutines copying them, so
// without WaitDelay this call outlives its deadline for as long as the child
// lives. A hang is the one outcome the exit-code contract cannot express.
func TestAGitCallReturnsEvenWhenAChildHoldsItsPipes(t *testing.T) {
	dir := t.TempDir()
	shim := filepath.Join(dir, "git")
	// Absolute paths: PATH is about to be narrowed to the shim's own directory,
	// so a bare "sleep" here would not resolve and the shim would exit at once,
	// leaving nothing holding the pipe and nothing for the test to prove.
	require.NoError(t, os.WriteFile(shim, []byte("#!/bin/sh\n/bin/sleep 120 &\n/bin/sleep 120\n"), 0o755))
	t.Setenv("PATH", dir)
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	start := time.Now()
	go func() {
		_, err := Discover(ctx, dir)
		done <- err
	}()
	select {
	case err := <-done:
		require.Error(t, err)
		assert.ErrorIs(t, err, errNotAnswered, "the deadline, not the process state, carries this")
		_, ok := exitStatus(err)
		assert.False(t, ok, "a call that never finished is not git answering")
		assert.Less(t, time.Since(start), WaitDelay+10*time.Second)
	case <-time.After(WaitDelay + 15*time.Second):
		t.Fatal("the git call outlived its deadline: WaitDelay is not bounding the wait")
	}
}

// cat-file -e exits 1 for an absent object and for several ways of failing to
// look for one. Only git's silence separates them, so these pin the layouts
// where the status alone would refute a correct ref: the earlier corruption
// tests all damage loose objects, the one layout where -e never has to read.
func TestAnObjectStoreItCannotSearchIsNotAnAbsentRef(t *testing.T) {
	t.Parallel()
	t.Run("an unreadable alternates store", func(t *testing.T) {
		t.Parallel()
		origin, sha := repo(t)
		clone := t.TempDir()
		run(t, clone, "clone", "--quiet", "--shared", origin.Root(), clone)
		require.NoError(t, os.Chmod(filepath.Join(origin.Root(), ".git", "objects"), 0o000))
		t.Cleanup(func() { _ = os.Chmod(filepath.Join(origin.Root(), ".git", "objects"), 0o755) })
		repository, err := Discover(t.Context(), clone)
		require.NoError(t, err)
		doc := document(t, sha, []map[string]any{{"file": "internal/fetch/client.go"}})
		diagnostics, skipped, verification := New(repository, logger()).Verify(t.Context(), doc)
		assert.Empty(t, diagnostics, "a store git could not search is not a ref that does not resolve")
		assert.Equal(t, "unavailable", verification.Source)
		require.NotEmpty(t, skipped)
	})
	t.Run("a damaged pack index", func(t *testing.T) {
		t.Parallel()
		repository, sha := repo(t)
		run(t, repository.Root(), "repack", "-adq")
		packs, err := filepath.Glob(filepath.Join(repository.Root(), ".git", "objects", "pack", "*.idx"))
		require.NoError(t, err)
		require.NotEmpty(t, packs)
		for _, pack := range packs {
			require.NoError(t, os.Chmod(pack, 0o644))
			require.NoError(t, os.WriteFile(pack, []byte("too small to be an index"), 0o644))
		}
		doc := document(t, sha, []map[string]any{{"file": "internal/fetch/client.go"}})
		diagnostics, skipped, verification := New(repository, logger()).Verify(t.Context(), doc)
		assert.Empty(t, diagnostics, "a damaged pack index is not a ref that does not resolve")
		assert.Equal(t, "unavailable", verification.Source)
		require.NotEmpty(t, skipped)
	})
}

// The deadline classification cannot fail on Unix through exitStatus alone,
// because a killed process already reports Exited() false there. This pins the
// marker itself, which is what carries the answer on Windows.
func TestADeadlineIsNeverReadAsGitAnswering(t *testing.T) {
	t.Parallel()
	exited := refused(t, 1)
	_, ok := exitStatus(exited)
	require.True(t, ok, "a real exit is git answering")
	timedOut := fmt.Errorf("%w: %w", errNotAnswered, exited)
	_, ok = exitStatus(timedOut)
	assert.False(t, ok,
		"a call abandoned at its deadline is not an answer, whatever the platform says about the process")
	assert.False(t, objectAbsent(timedOut), "and it is certainly not proof the object is absent")
}

// loam-refinery-0v1 asks for both call sites, and Repository.run is the one every
// anchor goes through.
func TestRepositoryRunReturnsWhenAChildHoldsItsPipes(t *testing.T) {
	dir := t.TempDir()
	shim := filepath.Join(dir, "git")
	require.NoError(t, os.WriteFile(shim, []byte("#!/bin/sh\n/bin/sleep 120 &\n/bin/sleep 120\n"), 0o755))
	t.Setenv("PATH", dir)
	repository := &Repository{root: dir}
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := repository.run(ctx, "cat-file", "-e", absentSHA)
		done <- err
	}()
	select {
	case err := <-done:
		require.Error(t, err)
		assert.ErrorIs(t, err, errNotAnswered, "the deadline, not the process state, carries this")
		_, ok := exitStatus(err)
		assert.False(t, ok)
		assert.False(t, objectAbsent(err), "a call that never finished never found the object absent")
	case <-time.After(WaitDelay + 15*time.Second):
		t.Fatal("Repository.run outlived its deadline: WaitDelay is not bounding the wait")
	}
}

// Absence is read from git's silence, so anything that can put text on stderr
// without git having failed is a way to lose a real finding. These pin the two
// that reach it: the caller's environment, and a partial clone's lazy fetch.
func TestNoiseOnStderrDoesNotHideAnAbsentRef(t *testing.T) {
	t.Run("a trace variable inherited from the caller", func(t *testing.T) {
		t.Setenv("GIT_TRACE", "1")
		repository, _ := repo(t)
		doc := document(t, absentSHA, []map[string]any{{"file": "internal/fetch/client.go"}})
		diagnostics, _, _ := New(repository, logger()).Verify(t.Context(), doc)
		require.Len(t, diagnostics, 1, "trace output is not git failing to look")
		assert.Equal(t, "ref-unknown", diagnostics[0].Name)
	})
	t.Run("a partial clone asked for a ref nobody has", func(t *testing.T) {
		t.Parallel()
		origin, _ := repo(t)
		run(t, origin.Root(), "config", "uploadpack.allowfilter", "true")
		dir := t.TempDir()
		clone := filepath.Join(dir, "clone")
		if err := exec.Command("git", "clone", "--quiet", "--filter=blob:none",
			"--no-local", "file://"+origin.Root(), clone).Run(); err != nil {
			t.Skip("this git cannot make a partial clone here")
		}
		repository, err := Discover(t.Context(), clone)
		require.NoError(t, err)
		doc := document(t, absentSHA, []map[string]any{{"file": "internal/fetch/client.go"}})
		diagnostics, _, _ := New(repository, logger()).Verify(t.Context(), doc)
		require.Len(t, diagnostics, 1,
			"a lazy fetch reaching for a ref that exists nowhere is still an absent ref")
		assert.Equal(t, "ref-unknown", diagnostics[0].Name)
	})
}

func TestComplainedSeparatesGitsWarningsFromItsFailures(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		stderr string
		want   bool
	}{
		{name: "silence"},
		{name: "a warning is not a failure", stderr: "warning: unable to access '.git/x'"},
		{name: "trace output is not a failure", stderr: "12:00:00.1 git.c:463  trace: built-in: git cat-file"},
		{name: "an error is", stderr: "error: index file .git/objects/pack/p.idx is too small", want: true},
		{name: "a fatal is", stderr: "fatal: git upload-pack: not our ref", want: true},
		{name: "an error after a warning is", stderr: "warning: nope\nerror: unable to open object pack directory", want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, test.want, complained(test.stderr))
		})
	}
}

func TestStderrIsReadFromEitherShapeOfGitFailure(t *testing.T) {
	t.Parallel()
	exit := refused(t, 1)
	exit.Stderr = []byte("fatal: something\n")
	assert.Equal(t, "fatal: something", stderrOf(exit), "cmd.Output puts git's words on the ExitError")
	wrapped := fmt.Errorf("wrapped: %w", &gitError{args: []string{"cat-file"}, stderr: "error: nope", err: exit})
	assert.Equal(t, "error: nope", stderrOf(wrapped), "and Repository.run puts them on a gitError")
	assert.Empty(t, stderrOf(errors.New("plain")), "an error that is not git's says nothing about git")
}

// A ".." segment is not a spelling git normalises away, it is a different file.
// Cleaning it would confirm the anchor against something it does not name,
// while anchor-path-safe is separately calling it unsafe.
func TestATraversalPathIsNeverVerifiedAgainstAnotherFile(t *testing.T) {
	t.Parallel()
	repository, sha := repo(t)
	doc := document(t, sha, []map[string]any{{"file": "internal/../internal/fetch/client.go"}})
	diagnostics, _, verification := New(repository, logger()).Verify(t.Context(), doc)
	assert.Zero(t, verification.Verified, "the anchor names a path git does not store")
	require.Len(t, diagnostics, 1)
	assert.Equal(t, "anchor-file-missing", diagnostics[0].Name)
}

// loam-refinery-4fw names this case: a partial clone whose promisor is gone must
// leave a correct review passing rather than refuting anchors it cannot read.
func TestAPromisorItCannotReachDoesNotRefuteAnchors(t *testing.T) {
	t.Parallel()
	origin, sha := repo(t)
	run(t, origin.Root(), "config", "uploadpack.allowfilter", "true")
	dir := t.TempDir()
	clone := filepath.Join(dir, "clone")
	// --no-checkout, or the clone fetches the very blob this test needs absent.
	if err := exec.Command("git", "clone", "--quiet", "--filter=blob:none", "--no-checkout",
		"--no-local", "file://"+origin.Root(), clone).Run(); err != nil {
		t.Skip("this git cannot make a partial clone here")
	}
	require.NoError(t, os.Rename(origin.Root(), origin.Root()+"-moved"))
	repository, err := Discover(t.Context(), clone)
	require.NoError(t, err)
	doc := document(t, sha, []map[string]any{{"file": "internal/fetch/client.go", "line": 40}})
	diagnostics, skipped, _ := New(repository, logger()).Verify(t.Context(), doc)
	assert.Empty(t, diagnostics, "a blob the promisor cannot supply is not a wrong line number")
	require.NotEmpty(t, skipped)
}

// The reason is read as one string, so it keeps git's first sentence rather
// than its whole complaint.
func TestAGitFailureReasonStaysOnOneLine(t *testing.T) {
	t.Parallel()
	failure := &gitError{
		args:   []string{"cat-file", "-e", absentSHA},
		stderr: "error: unable to open object pack directory: /x\nerror: again: /y\nfatal: could not get object info",
		err:    refused(t, 128),
	}
	assert.NotContains(t, failure.Error(), "\n", "a reason is one sentence, not a transcript")
	assert.Contains(t, failure.Error(), "unable to open object pack directory")
}

// plainEnv and complained() are deliberately redundant: either alone keeps a
// trace variable from hiding an absent ref, which means neither is pinned by a
// test that only runs git. These pin them separately.
func TestPlainEnvDropsTheVariablesThatWriteToStderr(t *testing.T) {
	t.Setenv("GIT_TRACE", "1")
	t.Setenv("GIT_TRACE2_PERF", "1")
	t.Setenv("GIT_CURL_VERBOSE", "1")
	t.Setenv("GIT_AUTHOR_NAME", "keep me")
	env := PlainEnv()
	for _, entry := range env {
		assert.NotContains(t, entry, "GIT_TRACE", "trace output would be read as git failing to look")
		assert.NotContains(t, entry, "GIT_CURL_VERBOSE")
	}
	assert.Contains(t, env, "GIT_AUTHOR_NAME=keep me", "only the noisy variables go")
	assert.Contains(t, env, "GIT_NO_LAZY_FETCH=1")
	assert.Contains(t, env, "LC_ALL=C")
}

func TestOnlyAComplaintWithdrawsTheClaimOfAbsence(t *testing.T) {
	t.Parallel()
	absent := func(stderr string) error {
		return &gitError{args: []string{"cat-file", "-e", absentSHA}, stderr: stderr, err: refused(t, 1)}
	}
	assert.True(t, objectAbsent(absent("")), "silence is the answer that means it is not here")
	assert.True(t, objectAbsent(absent("12:00:00.1 git.c:463  trace: built-in: git cat-file")),
		"noise on stderr is not git failing to look")
	assert.True(t, objectAbsent(absent("warning: unable to access '/x'")),
		"nor is a warning git carried on past")
	assert.False(t, objectAbsent(absent("error: index file p.idx is too small")))
	assert.False(t, objectAbsent(absent("fatal: git upload-pack: not our ref")))
	assert.False(t, objectAbsent(&gitError{stderr: "", err: refused(t, 128)}),
		"only exit 1 is the not-here answer at all")
}
