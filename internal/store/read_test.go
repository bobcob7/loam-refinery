package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOversizedRejectedInput_RecordsRunWithATruncatedFile is the end-to-end
// version of config.md section 4.4.1's acceptance criterion: a 2 MiB input
// records a run, and ListFailedRuns reports a path for it exactly like any
// other exit_code=1 row — nothing about the listing distinguishes a
// truncated file from a whole one, because the store no longer decides
// whether to write a file based on size, only how much of it to keep.
func TestOversizedRejectedInput_RecordsRunWithATruncatedFile(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	data := make([]byte, 2*1024*1024)
	digest, path, err := s.WriteRejected("github.com/example/example", data)
	require.NoError(t, err)
	require.NotEmpty(t, path)
	require.NoError(t, s.Record(t.Context(), RunInput{
		Repo: "github.com/example/example", Digest: digest, ExitCode: 1,
		ToolVersion: "0.1.0", SchemaVersion: "1",
	}))
	failed, total, err := s.ListFailedRuns(t.Context(), "github.com/example/example", "", 0)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, failed, 1)
	assert.Equal(t, path, failed[0].Path, "a truncated input's path is reported like any other kept file")
	assert.Empty(t, failed[0].Ref, "ref is empty when the run has none to report")
}

func TestListFailedRuns_KeptInputHasAPath(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	digest, path, err := s.WriteRejected("github.com/example/example", []byte("{}"))
	require.NoError(t, err)
	require.NotEmpty(t, path)
	require.NoError(t, s.Record(t.Context(), RunInput{
		Repo: "github.com/example/example", Digest: digest, ExitCode: 1,
		ToolVersion: "0.1.0", SchemaVersion: "1",
	}))
	failed, _, err := s.ListFailedRuns(t.Context(), "github.com/example/example", "", 0)
	require.NoError(t, err)
	require.Len(t, failed, 1)
	assert.Equal(t, path, failed[0].Path)
}

func TestListReviews_LimitZeroMeansUnlimited(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	for i := range 3 {
		require.NoError(t, s.Record(t.Context(), RunInput{
			Repo: "github.com/example/example", Digest: string(rune('a' + i)), ExitCode: 0,
			Verdict: "approve", ToolVersion: "0.1.0", SchemaVersion: "1",
		}))
	}
	reviews, total, err := s.ListReviews(t.Context(), "github.com/example/example", "", 0)
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, reviews, 3)
}

func TestListReviews_LimitCapsResultsButNotTotal(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	for i := range 3 {
		require.NoError(t, s.Record(t.Context(), RunInput{
			Repo: "github.com/example/example", Digest: string(rune('a' + i)), ExitCode: 0,
			Verdict: "approve", ToolVersion: "0.1.0", SchemaVersion: "1",
		}))
	}
	reviews, total, err := s.ListReviews(t.Context(), "github.com/example/example", "", 1)
	require.NoError(t, err)
	assert.Equal(t, 3, total, "total counts every matching row, not just the ones returned")
	assert.Len(t, reviews, 1)
}

func TestListRepos_OrderedByNameWithCounts(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.NoError(t, s.Record(t.Context(), RunInput{Repo: "b", Digest: "1", ExitCode: 0, Verdict: "approve", ToolVersion: "v", SchemaVersion: "1"}))
	require.NoError(t, s.Record(t.Context(), RunInput{Repo: "a", Digest: "2", ExitCode: 1, ToolVersion: "v", SchemaVersion: "1"}))
	require.NoError(t, s.Record(t.Context(), RunInput{Repo: "a", Digest: "3", ExitCode: 0, Verdict: "approve", ToolVersion: "v", SchemaVersion: "1"}))
	repos, err := s.ListRepos(t.Context())
	require.NoError(t, err)
	require.Len(t, repos, 2)
	assert.Equal(t, "a", repos[0].Name)
	assert.Equal(t, 1, repos[0].Reviews)
	assert.Equal(t, 1, repos[0].Failed)
	assert.Equal(t, "b", repos[1].Name)
	assert.Equal(t, 1, repos[1].Reviews)
	assert.Equal(t, 0, repos[1].Failed)
}

func TestKnown_TrueForARepoWithRowsFalseOtherwise(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.NoError(t, s.Record(t.Context(), RunInput{Repo: "known", Digest: "1", ExitCode: 0, Verdict: "approve", ToolVersion: "v", SchemaVersion: "1"}))
	known, err := s.Known(t.Context(), "known")
	require.NoError(t, err)
	assert.True(t, known)
	known, err = s.Known(t.Context(), "mistyped")
	require.NoError(t, err)
	assert.False(t, known)
}

// TestDistinctDigests_SameDigestTwoRowsKeepsEarliestAt proves
// combined-reviews.md section 5.3.1's "one review is one digest, not one
// row": two runs sharing a digest (a byte-identical resubmission, config.md
// section 4.4's O_EXCL race) collapse into a single result whose At is the
// earliest of the two, not the latest and not whichever row a query without
// an explicit min() happened to pick.
func TestDistinctDigests_SameDigestTwoRowsKeepsEarliestAt(t *testing.T) {
	t.Parallel()
	const repo, ref = "github.com/example/example", "4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f"
	earlier := time.Date(2026, 8, 19, 14, 0, 0, 0, time.UTC)
	later := time.Date(2026, 8, 19, 15, 0, 0, 0, time.UTC)
	times := []time.Time{earlier, later}
	call := 0
	clk := &clockMock{NowFunc: func() time.Time {
		got := times[call]
		call++
		return got
	}}
	s, err := New(t.Context(), t.TempDir(), clk)
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	require.NoError(t, s.Record(t.Context(), RunInput{
		Repo: repo, Ref: ref, Digest: "d1", ExitCode: 0,
		Verdict: "approve", ToolVersion: "v", SchemaVersion: "1",
	}))
	require.NoError(t, s.Record(t.Context(), RunInput{
		Repo: repo, Ref: ref, Digest: "d1", ExitCode: 0,
		Verdict: "approve", ToolVersion: "v", SchemaVersion: "1",
	}))
	digests, err := s.DistinctDigests(t.Context(), repo, ref)
	require.NoError(t, err)
	require.Len(t, digests, 1, "two rows sharing one digest are one review, not two")
	assert.Equal(t, "d1", digests[0].Digest)
	assert.True(t, earlier.Equal(digests[0].At), "at is the earliest of the rows sharing the digest")
}

// TestDistinctDigests_TwoDigestsOrderedOldestFirst proves the other half of
// section 5.3.1: two genuinely different digests are two distinct reviews,
// both survive, and the ordering collect-reviews needs is oldest-at-first
// (combined-reviews.md section 8.1's "oldest internally-first"), not
// alphabetical by digest and not insertion order.
func TestDistinctDigests_TwoDigestsOrderedOldestFirst(t *testing.T) {
	t.Parallel()
	const repo, ref = "github.com/example/example", "4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f"
	earlier := time.Date(2026, 8, 19, 14, 0, 0, 0, time.UTC)
	later := time.Date(2026, 8, 19, 15, 0, 0, 0, time.UTC)
	times := []time.Time{earlier, later}
	call := 0
	clk := &clockMock{NowFunc: func() time.Time {
		got := times[call]
		call++
		return got
	}}
	s, err := New(t.Context(), t.TempDir(), clk)
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	require.NoError(t, s.Record(t.Context(), RunInput{
		Repo: repo, Ref: ref, Digest: "zzz-earlier", ExitCode: 0,
		Verdict: "approve", ToolVersion: "v", SchemaVersion: "1",
	}))
	require.NoError(t, s.Record(t.Context(), RunInput{
		Repo: repo, Ref: ref, Digest: "aaa-later", ExitCode: 0,
		Verdict: "approve", ToolVersion: "v", SchemaVersion: "1",
	}))
	digests, err := s.DistinctDigests(t.Context(), repo, ref)
	require.NoError(t, err)
	require.Len(t, digests, 2)
	assert.Equal(t, "zzz-earlier", digests[0].Digest, "earliest at comes first regardless of digest's alphabetical order")
	assert.Equal(t, "aaa-later", digests[1].Digest)
}

// TestDistinctDigests_NoMatchingRowsReturnsEmptyNoError matches this
// package's existing empty-answer convention (e.g. ListReviews on an
// unknown ref): a repo and ref the store has nothing for is not an error.
func TestDistinctDigests_NoMatchingRowsReturnsEmptyNoError(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	digests, err := s.DistinctDigests(t.Context(), "unknown/repo", "0000000000000000000000000000000000000000")
	require.NoError(t, err)
	assert.Empty(t, digests)
}

// TestDistinctDigests_ExcludesFailedRuns proves a query that forgot the
// exit_code = 0 filter would silently start counting rejected inputs as
// reviews: a failed run's digest for the same repo and ref must not appear
// beside a passing one's.
func TestDistinctDigests_ExcludesFailedRuns(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	const repo, ref = "github.com/example/example", "4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f"
	require.NoError(t, s.Record(t.Context(), RunInput{
		Repo: repo, Ref: ref, Digest: "passed", ExitCode: 0,
		Verdict: "approve", ToolVersion: "v", SchemaVersion: "1",
	}))
	require.NoError(t, s.Record(t.Context(), RunInput{
		Repo: repo, Ref: ref, Digest: "failed", ExitCode: 1,
		ToolVersion: "v", SchemaVersion: "1",
	}))
	digests, err := s.DistinctDigests(t.Context(), repo, ref)
	require.NoError(t, err)
	require.Len(t, digests, 1)
	assert.Equal(t, "passed", digests[0].Digest)
}

// TestDistinctDigests_ExcludesOtherRepoOrRef proves both filters are
// applied independently: a digest recorded under a different repo, and one
// recorded under a different ref of the same repo, must both stay out of
// an answer for the target repo and ref.
func TestDistinctDigests_ExcludesOtherRepoOrRef(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	const repo, ref = "github.com/example/example", "4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f"
	require.NoError(t, s.Record(t.Context(), RunInput{
		Repo: repo, Ref: ref, Digest: "target", ExitCode: 0,
		Verdict: "approve", ToolVersion: "v", SchemaVersion: "1",
	}))
	require.NoError(t, s.Record(t.Context(), RunInput{
		Repo: "github.com/other/other", Ref: ref, Digest: "other-repo", ExitCode: 0,
		Verdict: "approve", ToolVersion: "v", SchemaVersion: "1",
	}))
	require.NoError(t, s.Record(t.Context(), RunInput{
		Repo: repo, Ref: "0000000000000000000000000000000000000000", Digest: "other-ref", ExitCode: 0,
		Verdict: "approve", ToolVersion: "v", SchemaVersion: "1",
	}))
	digests, err := s.DistinctDigests(t.Context(), repo, ref)
	require.NoError(t, err)
	require.Len(t, digests, 1)
	assert.Equal(t, "target", digests[0].Digest)
}

func TestReadContent_ReturnsExactBytes(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	data := []byte("not json {{{")
	_, path, err := s.WriteRejected("example", data)
	require.NoError(t, err)
	on, err := s.ReadContent(path)
	require.NoError(t, err)
	assert.Equal(t, data, on)
}
