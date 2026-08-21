package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func intPtr(n int) *int { return &n }

func TestRecord_InsertsARow(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	err := s.Record(t.Context(), RunInput{
		Repo:          "github.com/example/example",
		Ref:           "4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f",
		Digest:        "deadbeef",
		ExitCode:      0,
		Verdict:       "approve",
		Assessment:    "strong",
		NumComments:   intPtr(2),
		ToolVersion:   "0.1.0",
		SchemaVersion: "1",
	})
	require.NoError(t, err)
	reviews, total, err := s.ListReviews(t.Context(), "github.com/example/example", "", 0)
	require.NoError(t, err)
	require.Len(t, reviews, 1)
	assert.Equal(t, 1, total)
	assert.Equal(t, "approve", reviews[0].Verdict)
	assert.Equal(t, "strong", reviews[0].Assessment)
	require.NotNil(t, reviews[0].Counts.Comments)
	assert.Equal(t, 2, *reviews[0].Counts.Comments)
}

// TestRecord_RejectsInvalidVerdict proves the Store wrapper surfaces the
// database's own CHECK constraint on verdict (config.md section 4.5.2).
func TestRecord_RejectsInvalidVerdict(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	err := s.Record(t.Context(), RunInput{
		Repo: "github.com/example/example", Digest: "d", ExitCode: 0,
		Verdict: "bogus", ToolVersion: "0.1.0", SchemaVersion: "1",
	})
	assert.Error(t, err)
}

// TestRecord_RejectsInvalidAssessment mirrors
// TestRecord_RejectsInvalidVerdict for the database's CHECK constraint on
// assessment (config.md section 4.5.2), added alongside the column.
func TestRecord_RejectsInvalidAssessment(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	err := s.Record(t.Context(), RunInput{
		Repo: "github.com/example/example", Digest: "d", ExitCode: 0,
		Assessment: "bogus", ToolVersion: "0.1.0", SchemaVersion: "1",
	})
	assert.Error(t, err)
}

// TestRecord_OmittedAssessmentReadsBackAsAbsent proves an assessment left
// unset stores as NULL and reads back as "" — absent, not an empty string
// masquerading as a value that was actually recorded (RunInput's own doc
// comment; mirrors verdict's same NULL-for-"" convention).
func TestRecord_OmittedAssessmentReadsBackAsAbsent(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.NoError(t, s.Record(t.Context(), RunInput{
		Repo: "github.com/example/example", Digest: "d", ExitCode: 0,
		Verdict: "approve", ToolVersion: "0.1.0", SchemaVersion: "1",
	}))
	reviews, _, err := s.ListReviews(t.Context(), "github.com/example/example", "", 0)
	require.NoError(t, err)
	require.Len(t, reviews, 1)
	assert.Equal(t, "", reviews[0].Assessment)
}

// TestRecord_UnrecognizedExitCodeInsertsWithoutMigration proves config.md
// section 4.5.2: exit_code is deliberately unconstrained, so this binary
// accepts a code it does not itself define without a schema change.
func TestRecord_UnrecognizedExitCodeInsertsWithoutMigration(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	err := s.Record(t.Context(), RunInput{
		Repo: "github.com/example/example", Digest: "d", ExitCode: 111,
		ToolVersion: "0.1.0", SchemaVersion: "1",
	})
	require.NoError(t, err)
	failed, total, err := s.ListFailedRuns(t.Context(), "github.com/example/example", "", 0)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, failed, 1)
	assert.Equal(t, 111, failed[0].ExitCode)
}

// TestRecord_DuplicateBytesRecordTwoRuns proves config.md section 4.4:
// storing the same bytes twice writes one file but records two rows.
func TestRecord_DuplicateBytesRecordTwoRuns(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	data := []byte(`{"ref":"4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f"}`)
	ref := "4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f"
	digest, _, err := s.WriteReview("github.com/example/example", ref, data)
	require.NoError(t, err)
	for range 2 {
		digest2, _, err := s.WriteReview("github.com/example/example", ref, data)
		require.NoError(t, err)
		require.Equal(t, digest, digest2)
		require.NoError(t, s.Record(t.Context(), RunInput{
			Repo: "github.com/example/example", Ref: ref, Digest: digest, ExitCode: 0,
			Verdict: "approve", ToolVersion: "0.1.0", SchemaVersion: "1",
		}))
	}
	_, total, err := s.ListReviews(t.Context(), "github.com/example/example", "", 0)
	require.NoError(t, err)
	assert.Equal(t, 2, total, "two runs stored the same bytes, so two rows are recorded")
}

func TestRecord_UsesTheInjectedClock(t *testing.T) {
	t.Parallel()
	fixed := time.Date(2026, 8, 19, 14, 30, 5, 0, time.UTC)
	clk := &clockMock{NowFunc: func() time.Time { return fixed }}
	s, err := New(t.Context(), t.TempDir(), clk)
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	require.NoError(t, s.Record(t.Context(), RunInput{
		Repo: "github.com/example/example", Digest: "d", ExitCode: 0,
		Verdict: "approve", ToolVersion: "0.1.0", SchemaVersion: "1",
	}))
	reviews, _, err := s.ListReviews(t.Context(), "github.com/example/example", "", 0)
	require.NoError(t, err)
	require.Len(t, reviews, 1)
	assert.True(t, fixed.Equal(reviews[0].At))
}
