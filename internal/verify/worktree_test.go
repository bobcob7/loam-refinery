package verify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorktreeDivergedAgreesWithGit(t *testing.T) {
	t.Parallel()
	t.Run("a file never touched since the commit is not diverged", func(t *testing.T) {
		t.Parallel()
		repository, sha := divergeRepo(t, "line one\nline two\n")
		diverged, err := repository.worktreeDiverged(t.Context(), sha, "file.txt")
		require.NoError(t, err)
		assert.False(t, diverged)
	})
	t.Run("a file re-saved with identical content is not diverged", func(t *testing.T) {
		t.Parallel()
		repository, sha := divergeRepo(t, "line one\nline two\n")
		writeFile(t, repository.Root(), "file.txt", "line one\nline two\n")
		diverged, err := repository.worktreeDiverged(t.Context(), sha, "file.txt")
		require.NoError(t, err)
		assert.False(t, diverged)
	})
	t.Run("a modified file diverges", func(t *testing.T) {
		t.Parallel()
		repository, sha := divergeRepo(t, "original\n")
		writeFile(t, repository.Root(), "file.txt", "changed\n")
		diverged, err := repository.worktreeDiverged(t.Context(), sha, "file.txt")
		require.NoError(t, err)
		assert.True(t, diverged)
	})
	t.Run("a file deleted from the working tree is not diverged", func(t *testing.T) {
		t.Parallel()
		repository, sha := divergeRepo(t, "gone soon\n")
		require.NoError(t, os.Remove(filepath.Join(repository.Root(), "file.txt")))
		diverged, err := repository.worktreeDiverged(t.Context(), sha, "file.txt")
		require.NoError(t, err)
		assert.False(t, diverged, "a deleted file says nothing about what a reviewer read")
	})
	t.Run("a path present in the working tree but absent at ref errors", func(t *testing.T) {
		t.Parallel()
		repository, sha := divergeRepo(t, "tracked\n")
		writeFile(t, repository.Root(), "new.txt", "untracked\n")
		_, err := repository.worktreeDiverged(t.Context(), sha, "new.txt")
		assert.Error(t, err)
	})
	t.Run("a ref that is not HEAD is still compared, unrestricted", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeFile(t, dir, "file.txt", "first\n")
		run(t, dir, "init", "-q")
		run(t, dir, "add", "-A")
		run(t, dir, "-c", "user.email=test@example.com", "-c", "user.name=test", "commit", "-qm", "first")
		firstSHA := strings.TrimSpace(run(t, dir, "rev-parse", "HEAD"))
		writeFile(t, dir, "file.txt", "second\n")
		run(t, dir, "add", "-A")
		run(t, dir, "-c", "user.email=test@example.com", "-c", "user.name=test", "commit", "-qm", "second")
		repository, err := Discover(t.Context(), dir)
		require.NoError(t, err)
		// The working tree now holds "second", which is HEAD's content, but we
		// ask about the earlier ref. The primitive has no HEAD restriction of
		// its own — that restriction belongs to the caller — so it must report
		// what git actually says about firstSHA rather than silencing it.
		diverged, err := repository.worktreeDiverged(t.Context(), firstSHA, "file.txt")
		require.NoError(t, err)
		assert.True(t, diverged, "the working tree holds \"second\", firstSHA names \"first\" — a real difference")
	})
}

// TestWorktreeDivergedIgnoresLineEndingNormalizationUnderAutocrlf is the test
// that matters most for this bead: a raw byte comparison sees every checkout
// under core.autocrlf=true as diverged, which would make the check worse than
// useless. See the mutation-check report for proof this actually catches that
// swap.
func TestWorktreeDivergedIgnoresLineEndingNormalizationUnderAutocrlf(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	run(t, dir, "init", "-q")
	run(t, dir, "config", "core.autocrlf", "true")
	writeFile(t, dir, "file.txt", "line one\nline two\n")
	run(t, dir, "add", "-A")
	run(t, dir, "-c", "user.email=test@example.com", "-c", "user.name=test", "commit", "-qm", "init")
	sha := strings.TrimSpace(run(t, dir, "rev-parse", "HEAD"))
	// checkout rewrites the working tree from the index, and with
	// core.autocrlf=true that means converting the stored LF endings to CRLF
	// on the way out — the exact scenario a raw byte comparison gets wrong.
	// The file must be removed first: git skips rewriting a file it believes
	// already matches the index, which would silently make this a no-op.
	require.NoError(t, os.Remove(filepath.Join(dir, "file.txt")))
	run(t, dir, "checkout", "--", "file.txt")
	worktreeBytes, err := os.ReadFile(filepath.Join(dir, "file.txt"))
	require.NoError(t, err)
	require.Contains(t, string(worktreeBytes), "\r\n", "the checkout must actually have produced CRLF, or this test proves nothing")
	blobText := run(t, dir, "show", sha+":file.txt")
	require.NotEqual(t, string(worktreeBytes), blobText, "the raw bytes must differ, or this test proves nothing about filtering")
	repository, err := Discover(t.Context(), dir)
	require.NoError(t, err)
	diverged, err := repository.worktreeDiverged(t.Context(), sha, "file.txt")
	require.NoError(t, err)
	assert.False(t, diverged, "line-ending normalization under core.autocrlf is not a divergence")
}

// TestWorktreeDivergedHonorsAGitattributesFilter covers the .gitattributes
// side of the same claim, independent of core.autocrlf: an eol=lf attribute
// drives the same clean-filter normalization hash-object applies.
func TestWorktreeDivergedHonorsAGitattributesFilter(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, ".gitattributes", "file.txt text eol=lf\n")
	writeFile(t, dir, "file.txt", "line one\nline two\n")
	run(t, dir, "init", "-q")
	run(t, dir, "add", "-A")
	run(t, dir, "-c", "user.email=test@example.com", "-c", "user.name=test", "commit", "-qm", "init")
	sha := strings.TrimSpace(run(t, dir, "rev-parse", "HEAD"))
	// Editing the working copy to CRLF without staging exercises the same eol
	// attribute hash-object applies, independent of core.autocrlf.
	writeFile(t, dir, "file.txt", "line one\r\nline two\r\n")
	repository, err := Discover(t.Context(), dir)
	require.NoError(t, err)
	diverged, err := repository.worktreeDiverged(t.Context(), sha, "file.txt")
	require.NoError(t, err)
	assert.False(t, diverged, "the eol=lf attribute normalizes the CRLF edit the same way core.autocrlf does")
}

// divergeRepo builds a throwaway repository holding one committed file and
// returns it with the SHA of its only commit.
func divergeRepo(t *testing.T, content string) (*Repository, string) {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", content)
	run(t, dir, "init", "-q")
	run(t, dir, "add", "-A")
	run(t, dir, "-c", "user.email=test@example.com", "-c", "user.name=test", "commit", "-qm", "init")
	sha := strings.TrimSpace(run(t, dir, "rev-parse", "HEAD"))
	repository, err := Discover(t.Context(), dir)
	require.NoError(t, err)
	return repository, sha
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
}
