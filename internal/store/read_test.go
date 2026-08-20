package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOversizedRejectedInput_RecordsRunButWritesNoFile is the end-to-end
// version of config.md section 4.4.1's acceptance criterion: a 2 MiB input
// records a run, and the row it produces is visible with its path absent —
// the same signal a deleted file would give.
func TestOversizedRejectedInput_RecordsRunButWritesNoFile(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	data := make([]byte, 2*1024*1024)
	digest, path, err := s.WriteRejected("github.com/example/example", data)
	require.NoError(t, err)
	assert.Empty(t, path)
	require.NoError(t, s.Record(t.Context(), RunInput{
		Repo: "github.com/example/example", Digest: digest, ExitCode: 1,
		ToolVersion: "0.1.0", SchemaVersion: "1",
	}))
	failed, total, err := s.ListFailedRuns(t.Context(), "github.com/example/example", "", 0)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, failed, 1)
	assert.Empty(t, failed[0].Path, "an omitted path is the only signal a caller gets that the input was not kept")
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
