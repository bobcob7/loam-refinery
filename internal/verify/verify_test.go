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

	"github.com/bobcob7/refinery/internal/review"
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
	return &gitRunnerMock{runFunc: func(_ context.Context, args ...string) ([]byte, error) {
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
	}}
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
	assert.Len(t, git.runCalls(), 4, "the ref resolves once, then one tree and one blob read per distinct path")
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
	assert.Equal(t, "none", verification.Source)
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
	assert.Equal(t, "none", verification.Source)
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
		assert.Equal(t, "none", verification.Source)
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
	t.Run("a wildcard never matches a path it did not name", func(t *testing.T) {
		t.Parallel()
		doc := document(t, sha, []map[string]any{{"file": "internal/fetch/*.go"}})
		diagnostics, _, _ := New(repository, logger()).Verify(t.Context(), doc)
		require.Len(t, diagnostics, 1)
		assert.Equal(t, "anchor-file-missing", diagnostics[0].Name,
			"a pathspec wildcard must not verify an anchor against some other file")
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
		assert.False(t, answered(err), "a call that never finished is not git answering")
		assert.Less(t, time.Since(start), waitDelay+10*time.Second)
	case <-time.After(waitDelay + 15*time.Second):
		t.Fatal("the git call outlived its deadline: WaitDelay is not bounding the wait")
	}
}
