package verify

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These are the real-repository tests for anchor-worktree-diverged: the
// three disjoint cases from docs/cli.md §2.3.1, plus the edge conditions the
// design calls out by name. worktree_test.go already pins the primitive
// itself against real git; these pin the policy Verify applies on top of it.

func TestVerifyChecksAnUnmodifiedFileAtHEADNormally(t *testing.T) {
	t.Parallel()
	repository, sha := divergeRepo(t, "line one\nline two\n")
	doc := document(t, sha, []map[string]any{{"file": "file.txt", "line": 1}})
	diagnostics, skipped, verification := New(repository, logger()).Verify(t.Context(), doc)
	assert.Empty(t, diagnostics)
	assert.Empty(t, skipped)
	assert.Empty(t, verification.Unverified)
	assert.Equal(t, 1, verification.Verified, "an untouched working tree does not withhold a verification")
}

func TestVerifyReportsAModifiedFileAtHEADAsUnverified(t *testing.T) {
	t.Parallel()
	repository, sha := divergeRepo(t, "line one\nline two\n")
	writeFile(t, repository.Root(), "file.txt", "changed\n")
	doc := document(t, sha, []map[string]any{{"file": "file.txt", "line": 1}})
	diagnostics, skipped, verification := New(repository, logger()).Verify(t.Context(), doc)
	assert.Empty(t, diagnostics, "a diverged anchor is never a diagnostic")
	assert.Empty(t, skipped, "a diverged anchor is not a skipped check either — the check ran")
	assert.Zero(t, verification.Verified)
	require.Len(t, verification.Unverified, 1)
	assert.Equal(t, "anchor-worktree-diverged", verification.Unverified[0].Name)
	assert.Equal(t, "dropped-context-1", verification.Unverified[0].Comment)
	assert.Equal(t, "/comments/0/anchors/0", verification.Unverified[0].Path,
		"the pointer names the whole anchor, the same way the field it explains would")
	assert.Contains(t, verification.Unverified[0].Message, "file.txt")
}

// A ref that is not HEAD must be checked exactly as it was before this
// feature existed: the working tree is irrelevant to a commit that is not
// checked out, and consulting it anyway would silence anchors the object
// database can already answer.
func TestVerifyNeverDivergesWhenRefIsNotHEAD(t *testing.T) {
	t.Parallel()
	repository, firstSHA, _ := twoCommitRepo(t)
	// The working tree now holds "second", which differs from firstSHA's
	// "first" — a real difference, but ref names the earlier commit, so it
	// must not be consulted.
	doc := document(t, firstSHA, []map[string]any{{"file": "file.txt", "line": 1}})
	diagnostics, skipped, verification := New(repository, logger()).Verify(t.Context(), doc)
	assert.Empty(t, diagnostics)
	assert.Empty(t, skipped)
	assert.Empty(t, verification.Unverified, "ref is not HEAD, so the working tree must never be consulted")
	assert.Equal(t, 1, verification.Verified)
}

// A path absent at ref stays anchor-file-missing, full stop: the working
// tree is not consulted and cannot soften it, whatever exists on disk.
func TestVerifyKeepsAPathAbsentAtRefAsFileMissingRegardlessOfTheWorkingTree(t *testing.T) {
	t.Parallel()
	repository, sha := divergeRepo(t, "tracked\n")
	writeFile(t, repository.Root(), "new.txt", "untracked, but present on disk\n")
	doc := document(t, sha, []map[string]any{{"file": "new.txt", "line": 1}})
	diagnostics, _, verification := New(repository, logger()).Verify(t.Context(), doc)
	require.Len(t, diagnostics, 1)
	assert.Equal(t, "anchor-file-missing", diagnostics[0].Name)
	assert.Empty(t, verification.Unverified, "a file the working tree happens to hold is not a divergence of a file at ref")
}

// A working-tree copy that no longer exists says nothing about what the
// reviewer read, so ref stays authoritative and the anchor is checked
// normally rather than reported diverged.
func TestVerifyDoesNotDivergeWhenTheWorkingTreeCopyIsDeleted(t *testing.T) {
	t.Parallel()
	repository, sha := divergeRepo(t, "line one\nline two\n")
	require.NoError(t, os.Remove(filepath.Join(repository.Root(), "file.txt")))
	doc := document(t, sha, []map[string]any{{"file": "file.txt", "line": 1}})
	diagnostics, _, verification := New(repository, logger()).Verify(t.Context(), doc)
	assert.Empty(t, diagnostics)
	assert.Empty(t, verification.Unverified, "a deleted working-tree file falls through to being checked, not reported diverged")
	assert.Equal(t, 1, verification.Verified)
}

// The benefit the spec names explicitly: a diverged anchor is never
// line-checked, so a hallucinated line number that would fail
// anchor-line-out-of-range at ref is reported diverged instead, not refuted
// for a length that was never the review's problem.
func TestVerifyNeverLineChecksADivergedAnchor(t *testing.T) {
	t.Parallel()
	repository, sha := divergeRepo(t, "only one line\n")
	writeFile(t, repository.Root(), "file.txt", "still just one line, but a different one\n")
	doc := document(t, sha, []map[string]any{{"file": "file.txt", "line": 900}})
	diagnostics, skipped, verification := New(repository, logger()).Verify(t.Context(), doc)
	assert.Empty(t, diagnostics, "anchor-line-out-of-range must not fire on a diverged anchor")
	assert.Empty(t, skipped)
	assert.Zero(t, verification.Verified)
	require.Len(t, verification.Unverified, 1)
	assert.Equal(t, "anchor-worktree-diverged", verification.Unverified[0].Name)
}

// twoCommitRepo builds a repository with two commits touching file.txt and
// returns it with both SHAs; the second is HEAD.
func twoCommitRepo(t *testing.T) (repository *Repository, firstSHA, secondSHA string) {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "first\n")
	run(t, dir, "init", "-q")
	run(t, dir, "add", "-A")
	run(t, dir, "-c", "user.email=test@example.com", "-c", "user.name=test", "commit", "-qm", "first")
	firstSHA = strings.TrimSpace(run(t, dir, "rev-parse", "HEAD"))
	writeFile(t, dir, "file.txt", "second\n")
	run(t, dir, "add", "-A")
	run(t, dir, "-c", "user.email=test@example.com", "-c", "user.name=test", "commit", "-qm", "second")
	secondSHA = strings.TrimSpace(run(t, dir, "rev-parse", "HEAD"))
	repository, err := Discover(t.Context(), dir)
	require.NoError(t, err)
	return repository, firstSHA, secondSHA
}

// The mock-based tests below are the honest unit for the policy itself: they
// pin the exact conjunction Verify applies — ref is HEAD, path present at
// ref, a working-tree copy exists, and it differs — without depending on
// git's own answers, and without the noise of building a real repository per
// condition.

// headMatching builds a git mock that resolves ref as an existing commit,
// finds every path as a 4-line blob at ref, and resolves HEAD to headSHA —
// giving full control over the ref-is-HEAD condition without a real
// repository. blob, when non-nil, overrides exactly one of the four
// per-anchor calls the same way answering's override does.
func headMatching(headSHA string, blob func(kind string, args []string) ([]byte, error, bool)) *gitRunnerMock {
	return &gitRunnerMock{runFunc: func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) == 2 && args[0] == "rev-parse" && args[1] == "HEAD" {
			return []byte(headSHA + "\n"), nil
		}
		kind := gitCall(args)
		if blob != nil {
			if out, err, handled := blob(kind, args); handled {
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

// Mutation guard: dropping the ref-is-HEAD condition would compare any ref
// against the working tree. headSHA here is deliberately not absentSHA, so
// isHEAD must be false — and worktreeDiverged, wired to panic-worthy content
// if it were ever consulted, must never be called at all.
func TestDivergenceIsNeverConsultedWhenRefIsNotHEAD(t *testing.T) {
	t.Parallel()
	git := headMatching(strings.Repeat("f", 40), nil)
	git.worktreeDivergedFunc = func(context.Context, string, string) (bool, error) {
		return true, nil
	}
	doc := document(t, absentSHA, []map[string]any{{"file": "a.go", "line": 1}})
	diagnostics, _, verification := New(git, logger()).Verify(t.Context(), doc)
	assert.Empty(t, diagnostics)
	assert.Empty(t, verification.Unverified)
	assert.Equal(t, 1, verification.Verified)
	assert.Empty(t, git.worktreeDivergedCalls(), "the working tree must never be asked about when ref is not HEAD")
}

// Mutation guard: row 2's mistake, rejected by design — a path absent at ref
// must never be softened by consulting the working tree, even one git would
// call identical.
func TestDivergenceIsNeverConsultedForAPathAbsentAtRef(t *testing.T) {
	t.Parallel()
	git := headMatching(absentSHA, func(kind string, _ []string) ([]byte, error, bool) {
		return nil, nil, kind == "ls-tree"
	})
	git.worktreeDivergedFunc = func(context.Context, string, string) (bool, error) {
		return false, nil
	}
	doc := document(t, absentSHA, []map[string]any{{"file": "a.go", "line": 1}})
	diagnostics, _, verification := New(git, logger()).Verify(t.Context(), doc)
	require.Len(t, diagnostics, 1)
	assert.Equal(t, "anchor-file-missing", diagnostics[0].Name)
	assert.Empty(t, verification.Unverified)
	assert.Empty(t, git.worktreeDivergedCalls(), "a path absent at ref is never consulted against the working tree")
}

// Mutation guard: anchor-worktree-diverged must never become a Diagnostic
// (it would then be an error by default), and a diverged anchor must never
// reach the line-range checks below it.
func TestADivergedAnchorIsReportedUnverifiedNeverAsADiagnostic(t *testing.T) {
	t.Parallel()
	git := headMatching(absentSHA, nil)
	git.worktreeDivergedFunc = func(context.Context, string, string) (bool, error) {
		return true, nil
	}
	// line 900 would fail anchor-line-out-of-range against the mock's 4-line
	// blob if the divergence check did not short-circuit first.
	doc := document(t, absentSHA, []map[string]any{{"file": "a.go", "line": 900}})
	diagnostics, skipped, verification := New(git, logger()).Verify(t.Context(), doc)
	assert.Empty(t, diagnostics, "a diverged anchor is reported in verification.unverified, never as an error diagnostic")
	assert.Empty(t, skipped)
	assert.Zero(t, verification.Verified)
	require.Len(t, verification.Unverified, 1)
	assert.Equal(t, "anchor-worktree-diverged", verification.Unverified[0].Name)
}

// A git failure comparing the working tree is a machine problem, not a
// finding: it must not be read as either verified or diverged.
func TestAFailedWorktreeComparisonIsSkippedNotReportedAsDiverged(t *testing.T) {
	t.Parallel()
	git := headMatching(absentSHA, nil)
	git.worktreeDivergedFunc = func(context.Context, string, string) (bool, error) {
		return false, gitFailure
	}
	doc := document(t, absentSHA, []map[string]any{{"file": "a.go", "line": 1}})
	diagnostics, skipped, verification := New(git, logger()).Verify(t.Context(), doc)
	assert.Empty(t, diagnostics)
	assert.Empty(t, verification.Unverified, "a git failure is not a divergence finding")
	assert.Zero(t, verification.Verified)
	require.NotEmpty(t, skipped)
	assert.Equal(t, "git could not read the file for 1 anchor", skipped[0].Reason)
}

// A HEAD that cannot be resolved must not be misread as "ref is HEAD": the
// anchor falls through to being checked normally, exactly as when ref
// genuinely is not HEAD. Nothing stubs worktreeDivergedFunc here, so a wrong
// isHEAD would panic rather than silently pass.
func TestAFailedHEADLookupFallsThroughToCheckedNormally(t *testing.T) {
	t.Parallel()
	git := &gitRunnerMock{runFunc: func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) == 2 && args[0] == "rev-parse" && args[1] == "HEAD" {
			return nil, gitFailure
		}
		kind := gitCall(args)
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
	doc := document(t, absentSHA, []map[string]any{{"file": "a.go", "line": 1}})
	diagnostics, _, verification := New(git, logger()).Verify(t.Context(), doc)
	assert.Empty(t, diagnostics)
	assert.Equal(t, 1, verification.Verified, "an unresolvable HEAD must not be read as ref being HEAD")
}

// HEAD is resolved once per run, the same way ref existence already is —
// not once per anchor, even across anchors naming distinct paths.
func TestHEADIsResolvedOncePerRunNotPerAnchor(t *testing.T) {
	t.Parallel()
	git := answering(nil)
	doc := document(t, absentSHA, []map[string]any{
		{"file": "a.go", "line": 1},
		{"file": "b.go", "line": 1},
		{"file": "c.go", "line": 1},
	})
	_, _, verification := New(git, logger()).Verify(t.Context(), doc)
	assert.Equal(t, 3, verification.Verified)
	headCalls := 0
	for _, call := range git.runCalls() {
		if len(call.Args) == 2 && call.Args[0] == "rev-parse" && call.Args[1] == "HEAD" {
			headCalls++
		}
	}
	assert.Equal(t, 1, headCalls, "HEAD is resolved once per run, not once per anchor")
}
