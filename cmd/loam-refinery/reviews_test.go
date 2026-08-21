package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/bobcob7/loam-refinery/internal/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A store that has never been written to is read as empty, and reading it
// must never create the directory, store.db, or anything else (docs/config.md
// §2.2, §6.2).
func TestReviewsAdapter_AbsentStoreIsReadWithoutCreatingAnything(t *testing.T) {
	home := homeFor(t)
	adapter := newReviewsAdapter(quietLog())
	known, err := adapter.Known(t.Context(), "no-repo")
	require.NoError(t, err)
	assert.False(t, known)
	reviews, total, err := adapter.ListReviews(t.Context(), "no-repo", "", 10)
	require.NoError(t, err)
	assert.Empty(t, reviews)
	assert.Equal(t, 0, total)
	failed, total, err := adapter.ListFailedRuns(t.Context(), "no-repo", "", 10)
	require.NoError(t, err)
	assert.Empty(t, failed)
	assert.Equal(t, 0, total)
	repos, err := adapter.ListRepos(t.Context())
	require.NoError(t, err)
	assert.Empty(t, repos)
	assert.NoFileExists(t, filepath.Join(home, "store.db"))
	assert.NoDirExists(t, filepath.Join(home, "reviews"))
	assert.NoDirExists(t, filepath.Join(home, "rejected"))
}

// Once validate has stored something, reviews reads it back: Known
// distinguishes the repository it wrote to from one it never heard of.
func TestReviewsAdapter_KnownDistinguishesAWrittenRepoFromAMistypedOne(t *testing.T) {
	homeFor(t)
	store := newStoreAdapter(quietLog())
	dir := t.TempDir()
	ref := "4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f"
	require.NoError(t, store.Save(t.Context(), cli.StoreInput{
		Dir: dir, Source: []byte(`{"version":"1","verdict":"approve","ref":"` + ref + `","comments":[]}`),
		Valid: true, Ref: ref, Verdict: "approve", ToolVersion: "test", SchemaVersion: "1",
	}))
	adapter := newReviewsAdapter(quietLog())
	known, err := adapter.Known(t.Context(), "no-repo")
	require.NoError(t, err)
	assert.True(t, known, "the repository validate wrote to is known")
	known, err = adapter.Known(t.Context(), "no-repo-typo")
	require.NoError(t, err)
	assert.False(t, known, "a mistyped repository is not known")
}

// ListReviews and ListFailedRuns read what Save wrote, and reading never
// materializes anything new.
func TestReviewsAdapter_ListsWhatWasStored(t *testing.T) {
	homeFor(t)
	store := newStoreAdapter(quietLog())
	dir := t.TempDir()
	ref := "4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f"
	require.NoError(t, store.Save(t.Context(), cli.StoreInput{
		Dir: dir, Source: []byte(`{"version":"1","verdict":"approve","ref":"` + ref + `","comments":[]}`),
		Valid: true, Ref: ref, Verdict: "approve", Comments: 3, ToolVersion: "test", SchemaVersion: "1",
	}))
	require.NoError(t, store.Save(t.Context(), cli.StoreInput{
		Dir: dir, Source: []byte(`{}`), Valid: false, ToolVersion: "test", SchemaVersion: "1",
	}))
	adapter := newReviewsAdapter(quietLog())
	reviews, total, err := adapter.ListReviews(t.Context(), "no-repo", "", 10)
	require.NoError(t, err)
	require.Len(t, reviews, 1)
	assert.Equal(t, 1, total)
	assert.Equal(t, ref, reviews[0].Ref)
	assert.FileExists(t, reviews[0].Path)
	failed, total, err := adapter.ListFailedRuns(t.Context(), "no-repo", "", 10)
	require.NoError(t, err)
	require.Len(t, failed, 1)
	assert.Equal(t, 1, total)
	assert.Empty(t, failed[0].Ref, "an input that never parsed has no ref")
}

// ReadContent surfaces a deleted file as an error rather than fabricating
// content (docs/config.md §6.3).
func TestReviewsAdapter_ReadContentReportsADeletedFile(t *testing.T) {
	homeFor(t)
	adapter := newReviewsAdapter(quietLog())
	path := filepath.Join(t.TempDir(), "gone.json")
	_, err := adapter.ReadContent(path)
	assert.Error(t, err)
}

// RepoName reports ok=false outside a repository, so reviews can tell the
// caller to pass --repo or use --list, rather than silently falling back to
// no-repo the way validate's storing path does.
func TestReviewsAdapter_RepoNameIsFalseOutsideARepository(t *testing.T) {
	homeFor(t)
	adapter := newReviewsAdapter(quietLog())
	dir := t.TempDir()
	name, ok, err := adapter.RepoName(t.Context(), dir)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Empty(t, name)
}

// RepoName resolves a real checkout's name the way validate's storing path
// does, so `reviews` inside a repository asks about the same repository
// validate would have stored to.
func TestReviewsAdapter_RepoNameResolvesAnActualRepository(t *testing.T) {
	homeFor(t)
	adapter := newReviewsAdapter(quietLog())
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	run("init", "--quiet")
	run("-c", "user.email=test@example.com", "-c", "user.name=test", "commit", "--quiet", "--allow-empty", "-m", "seed")
	name, ok, err := adapter.RepoName(t.Context(), dir)
	require.NoError(t, err)
	assert.True(t, ok)
	resolved, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	assert.Equal(t, "local/"+filepath.Base(resolved), name)
}

// reviews must answer from a store on a read-only filesystem: refinery-a96.19
// found it going through store.New, the writer's constructor, which fails
// outright on a read-only directory. This is the ratchet — without
// reviewsAdapter.open using store.NewReadOnly, this test fails again.
func TestReviewsAdapter_ReadsFromAReadOnlyStoreDirectory(t *testing.T) {
	home := homeFor(t)
	writer := newStoreAdapter(quietLog())
	dir := t.TempDir()
	ref := "4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f"
	require.NoError(t, writer.Save(t.Context(), cli.StoreInput{
		Dir: dir, Source: []byte(`{"version":"1","verdict":"approve","ref":"` + ref + `","comments":[]}`),
		Valid: true, Ref: ref, Verdict: "approve", ToolVersion: "test", SchemaVersion: "1",
	}))
	require.NoError(t, writer.Save(t.Context(), cli.StoreInput{
		Dir: dir, Source: []byte(`{}`), Valid: false, ToolVersion: "test", SchemaVersion: "1",
	}))
	require.NoError(t, os.Chmod(home, 0o500))
	t.Cleanup(func() { require.NoError(t, os.Chmod(home, 0o700)) })
	adapter := newReviewsAdapter(quietLog())
	reviews, total, err := adapter.ListReviews(t.Context(), "no-repo", "", 10)
	require.NoError(t, err)
	assert.Len(t, reviews, 1)
	assert.Equal(t, 1, total)
	failed, total, err := adapter.ListFailedRuns(t.Context(), "no-repo", "", 10)
	require.NoError(t, err)
	assert.Len(t, failed, 1)
	assert.Equal(t, 1, total)
	repos, err := adapter.ListRepos(t.Context())
	require.NoError(t, err)
	assert.Len(t, repos, 1)
	content, err := adapter.ReadContent(reviews[0].Path)
	require.NoError(t, err)
	assert.NotEmpty(t, content)
}

// DistinctDigests reads what Save wrote, the same way ListReviews does, and
// answers empty rather than erroring for a repo or ref the store has never
// heard of.
func TestReviewsAdapter_DistinctDigestsReadsWhatWasStored(t *testing.T) {
	homeFor(t)
	writer := newStoreAdapter(quietLog())
	dir := t.TempDir()
	ref := "4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f"
	require.NoError(t, writer.Save(t.Context(), cli.StoreInput{
		Dir: dir, Source: []byte(`{"version":"1","verdict":"approve","ref":"` + ref + `","comments":[]}`),
		Valid: true, Ref: ref, Verdict: "approve", ToolVersion: "test", SchemaVersion: "1",
	}))
	adapter := newReviewsAdapter(quietLog())
	digests, err := adapter.DistinctDigests(t.Context(), "no-repo", ref)
	require.NoError(t, err)
	require.Len(t, digests, 1)
	digests, err = adapter.DistinctDigests(t.Context(), "no-repo", "0000000000000000000000000000000000000000")
	require.NoError(t, err)
	assert.Empty(t, digests, "a ref the store has never heard of answers empty, not an error")
}

// ReviewPath resolves the same path WriteReview actually placed the file
// at, so collect-reviews's own reader (composing ReviewPath and
// ReadContent) can read what DistinctDigests just enumerated.
func TestReviewsAdapter_ReviewPathMatchesWhereWriteReviewPlacedTheFile(t *testing.T) {
	homeFor(t)
	writer := newStoreAdapter(quietLog())
	dir := t.TempDir()
	ref := "4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f"
	require.NoError(t, writer.Save(t.Context(), cli.StoreInput{
		Dir: dir, Source: []byte(`{"version":"1","verdict":"approve","ref":"` + ref + `","comments":[]}`),
		Valid: true, Ref: ref, Verdict: "approve", ToolVersion: "test", SchemaVersion: "1",
	}))
	adapter := newReviewsAdapter(quietLog())
	digests, err := adapter.DistinctDigests(t.Context(), "no-repo", ref)
	require.NoError(t, err)
	require.Len(t, digests, 1)
	path, err := adapter.ReviewPath(t.Context(), "no-repo", ref, digests[0].Digest)
	require.NoError(t, err)
	assert.FileExists(t, path)
	content, err := adapter.ReadContent(path)
	require.NoError(t, err)
	assert.Contains(t, string(content), ref)
}

// StoreEnabled reads store.enabled off config the same way storeAdapter.Save
// does, and defaults to true (config.Load's own default) rather than
// erroring on a machine with no config file at all.
func TestReviewsAdapter_StoreEnabledReadsConfig(t *testing.T) {
	home := homeFor(t)
	adapter := newReviewsAdapter(quietLog())
	enabled, err := adapter.StoreEnabled(t.Context())
	require.NoError(t, err)
	assert.True(t, enabled, "no config file at all defaults to enabled")
	require.NoError(t, os.WriteFile(filepath.Join(home, "config.json"),
		[]byte(`{"version":"1","store":{"enabled":false}}`), 0o600))
	enabled, err = adapter.StoreEnabled(t.Context())
	require.NoError(t, err)
	assert.False(t, enabled, "an explicit store.enabled:false is read back, not overridden")
}

// A malformed store.repos override is a config error surfaced on a read
// exactly as it is on a write, caught before any store filesystem lookup.
func TestReviewsAdapter_BadRepoOverrideFailsBeforeAnyFilesystemLookup(t *testing.T) {
	home := homeFor(t)
	require.NoError(t, os.WriteFile(filepath.Join(home, "config.json"),
		[]byte(`{"version":"1","store":{"repos":{"/some/path":"../../etc"}}}`), 0o600))
	adapter := newReviewsAdapter(quietLog())
	_, err := adapter.Known(t.Context(), "no-repo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "store.repos")
	assert.NoFileExists(t, filepath.Join(home, "store.db"))
}
