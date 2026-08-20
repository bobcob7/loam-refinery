package cli

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/bobcob7/loam-refinery/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testRef = "4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f"

func TestReviews_ListIsExclusive(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"reviews", "--list", "--repo=x"},
		{"reviews", "--list", "--ref=" + testRef},
		{"reviews", "--list", "--limit=5"},
		{"reviews", "--list", "--content"},
		{"reviews", "--list", "--failed"},
	} {
		t.Run(args[2], func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, "")
			assert.Equal(t, ExitUsage, h.app.Run(t.Context(), args))
			assert.Contains(t, h.stderr.String(), "--list")
		})
	}
}

func TestReviews_MalformedRefIsUsageError(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	h.reviews.RepoNameFunc = func(context.Context, string) (string, bool, error) { return "some/repo", true, nil }
	assert.Equal(t, ExitUsage, h.app.Run(t.Context(), []string{"reviews", "--ref=abcdef12"}))
	assert.Contains(t, h.stderr.String(), "--ref")
}

func TestReviews_MalformedRepoIsUsageError(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	assert.Equal(t, ExitUsage, h.app.Run(t.Context(), []string{"reviews", "--repo=../nope"}))
	assert.Contains(t, h.stderr.String(), "--repo")
}

func TestReviews_OutsideARepositoryWithNoRepoFlagIsUsageError(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	h.reviews.RepoNameFunc = func(context.Context, string) (string, bool, error) { return "", false, nil }
	assert.Equal(t, ExitUsage, h.app.Run(t.Context(), []string{"reviews"}))
	assert.Contains(t, h.stderr.String(), "--list")
}

func TestReviews_AStoreErrorExitsTool(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		mock func(*reviewStoreMock)
	}{
		{"Known fails", func(m *reviewStoreMock) {
			m.KnownFunc = func(context.Context, string) (bool, error) { return false, errors.New("disk fell over") }
		}},
		{"ListReviews fails", func(m *reviewStoreMock) {
			m.ListReviewsFunc = func(context.Context, string, string, int) ([]store.Review, int, error) {
				return nil, 0, errors.New("disk fell over")
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, "")
			h.reviews.RepoNameFunc = func(context.Context, string) (string, bool, error) { return "some/repo", true, nil }
			test.mock(h.reviews)
			assert.Equal(t, ExitTool, h.app.Run(t.Context(), []string{"reviews"}))
			assert.Contains(t, h.stderr.String(), "disk fell over")
			assert.Empty(t, h.stdout.String())
		})
	}
}

func TestReviews_KnownDistinguishesAMistypedRepoFromAnEmptyOne(t *testing.T) {
	t.Parallel()
	for _, known := range []bool{true, false} {
		h := newHarness(t, "")
		h.reviews.KnownFunc = func(context.Context, string) (bool, error) { return known, nil }
		h.reviews.ListReviewsFunc = func(context.Context, string, string, int) ([]store.Review, int, error) {
			return nil, 0, nil
		}
		require.Equal(t, ExitValid, h.app.Run(t.Context(), []string{"reviews", "--repo=some/repo"}))
		var payload struct {
			Repo struct {
				Known bool `json:"known"`
			} `json:"repo"`
			Total   int   `json:"total"`
			Reviews []any `json:"reviews"`
		}
		require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &payload))
		assert.Equal(t, known, payload.Repo.Known)
		assert.Equal(t, 0, payload.Total)
		assert.Empty(t, payload.Reviews)
	}
}

func TestReviews_DefaultIndexShape(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	at := time.Date(2026, 8, 19, 14, 30, 5, 0, time.UTC)
	comments := 6
	h.reviews.KnownFunc = func(context.Context, string) (bool, error) { return true, nil }
	h.reviews.ListReviewsFunc = func(context.Context, string, string, int) ([]store.Review, int, error) {
		return []store.Review{{
			At: at, Ref: testRef, Digest: "deadbeef", Verdict: "request_changes",
			Counts: store.Counts{Comments: &comments}, Path: "/tmp/x.json",
		}}, 14, nil
	}
	require.Equal(t, ExitValid, h.app.Run(t.Context(), []string{"reviews", "--repo=some/repo"}))
	var payload struct {
		Repo struct {
			Name  string `json:"name"`
			Known bool   `json:"known"`
		} `json:"repo"`
		Total   int `json:"total"`
		Reviews []struct {
			At      string `json:"at"`
			Ref     string `json:"ref"`
			Digest  string `json:"digest"`
			Verdict string `json:"verdict"`
			Counts  struct {
				Comments *int `json:"comments"`
				Errors   *int `json:"errors"`
			} `json:"counts"`
			Path   string          `json:"path"`
			Review json.RawMessage `json:"review"`
		} `json:"reviews"`
	}
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &payload))
	assert.Equal(t, "some/repo", payload.Repo.Name)
	assert.True(t, payload.Repo.Known)
	assert.Equal(t, 14, payload.Total)
	require.Len(t, payload.Reviews, 1)
	row := payload.Reviews[0]
	assert.Equal(t, "2026-08-19T14:30:05Z", row.At)
	assert.Equal(t, testRef, row.Ref)
	assert.Equal(t, "deadbeef", row.Digest)
	assert.Equal(t, "request_changes", row.Verdict)
	require.NotNil(t, row.Counts.Comments)
	assert.Equal(t, 6, *row.Counts.Comments)
	assert.Nil(t, row.Counts.Errors, "a nil counter is omitted, not printed as null")
	assert.Equal(t, "/tmp/x.json", row.Path)
	assert.Nil(t, row.Review, "no --content, no review field")
}

func TestReviews_FailedOmitsRefWhenAbsentButAlwaysHasAPath(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	at := time.Date(2026, 8, 19, 14, 22, 41, 0, time.UTC)
	h.reviews.KnownFunc = func(context.Context, string) (bool, error) { return true, nil }
	h.reviews.ListFailedRunsFunc = func(context.Context, string, string, int) ([]store.FailedRun, int, error) {
		return []store.FailedRun{{At: at, ExitCode: 1, Path: "/tmp/rejected.json"}}, 1, nil
	}
	require.Equal(t, ExitValid, h.app.Run(t.Context(), []string{"reviews", "--repo=some/repo", "--failed"}))
	var raw map[string]any
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &raw))
	failed := raw["failed"].([]any)
	require.Len(t, failed, 1)
	row := failed[0].(map[string]any)
	_, hasRef := row["ref"]
	path, hasPath := row["path"]
	assert.False(t, hasRef, "ref must be absent, not null, when the run has none")
	assert.True(t, hasPath, "every rejected input is kept now, truncated when it was over 1 MiB, so path is never omitted")
	assert.Equal(t, "/tmp/rejected.json", path)
}

func TestReviews_ContentOnFailedReturnsNonJSONBytesUnaltered(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	h.reviews.KnownFunc = func(context.Context, string) (bool, error) { return true, nil }
	h.reviews.ListFailedRunsFunc = func(context.Context, string, string, int) ([]store.FailedRun, int, error) {
		return []store.FailedRun{{ExitCode: 1, Path: "/tmp/rejected.json"}}, 1, nil
	}
	h.reviews.ReadContentFunc = func(path string) ([]byte, error) {
		require.Equal(t, "/tmp/rejected.json", path)
		return []byte("not json at all"), nil
	}
	require.Equal(t, ExitValid, h.app.Run(t.Context(), []string{"reviews", "--repo=some/repo", "--failed", "--content"}))
	var raw map[string]any
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &raw))
	failed := raw["failed"].([]any)
	require.Len(t, failed, 1)
	row := failed[0].(map[string]any)
	assert.Equal(t, "not json at all", row["review"])
	assert.Equal(t, float64(0), raw["unreadable"])
}

func TestReviews_UnreadableFileIsCountedAndDroppedFromContent(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	h.reviews.KnownFunc = func(context.Context, string) (bool, error) { return true, nil }
	h.reviews.ListReviewsFunc = func(context.Context, string, string, int) ([]store.Review, int, error) {
		return []store.Review{{Ref: testRef, Digest: "deadbeef", Verdict: "approve", Path: "/tmp/gone.json"}}, 1, nil
	}
	h.reviews.ReadContentFunc = func(string) ([]byte, error) { return nil, errors.New("no such file") }
	require.Equal(t, ExitValid, h.app.Run(t.Context(), []string{"reviews", "--repo=some/repo", "--content"}))
	var raw map[string]any
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &raw))
	assert.Equal(t, float64(1), raw["unreadable"])
	reviews := raw["reviews"].([]any)
	require.Len(t, reviews, 1)
	row := reviews[0].(map[string]any)
	_, hasReview := row["review"]
	assert.False(t, hasReview, "an unreadable file leaves the row without a review field")
}

// TestReviews_UnparseableFileIsCountedNotFatal reproduces refinery-a96.36:
// json.RawMessage validates at marshal time, so embedding one corrupt
// stored file verbatim used to fail the whole encoding and take every other
// row down with it, exiting 2 as if the invocation were wrong. A corrupt
// file is unreadable the same way a missing one is.
func TestReviews_UnparseableFileIsCountedNotFatal(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	h.reviews.KnownFunc = func(context.Context, string) (bool, error) { return true, nil }
	h.reviews.ListReviewsFunc = func(context.Context, string, string, int) ([]store.Review, int, error) {
		return []store.Review{
			{Ref: testRef, Digest: "bad", Verdict: "approve", Path: "/tmp/bad.json"},
			{Ref: testRef, Digest: "good", Verdict: "approve", Path: "/tmp/good.json"},
		}, 2, nil
	}
	h.reviews.ReadContentFunc = func(path string) ([]byte, error) {
		if path == "/tmp/bad.json" {
			return []byte("oops"), nil
		}
		return []byte(`{"version":"1"}`), nil
	}
	code := h.app.Run(t.Context(), []string{"reviews", "--repo=some/repo", "--content"})
	require.Equal(t, ExitValid, code, h.stderr.String())
	var raw map[string]any
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &raw))
	assert.Equal(t, float64(1), raw["unreadable"], "the corrupt file is counted, the good one is not")
	reviews := raw["reviews"].([]any)
	require.Len(t, reviews, 2, "the good row survives alongside the bad one")
	bad := reviews[0].(map[string]any)
	_, hasReview := bad["review"]
	assert.False(t, hasReview, "the corrupt file's row has no review field")
	good := reviews[1].(map[string]any)
	review, ok := good["review"].(map[string]any)
	require.True(t, ok, "the good file's row still embeds its content")
	assert.Equal(t, "1", review["version"])
}

// TestReviews_EmptyStoredFileIsCountedNotVanished reproduces the quiet half
// of refinery-a96.36: a zero-byte stored file reads without error, so it used
// to be dropped by omitempty and reported as though nothing were lost.
func TestReviews_EmptyStoredFileIsCountedNotVanished(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	h.reviews.KnownFunc = func(context.Context, string) (bool, error) { return true, nil }
	h.reviews.ListReviewsFunc = func(context.Context, string, string, int) ([]store.Review, int, error) {
		return []store.Review{{Ref: testRef, Digest: "empty", Verdict: "approve", Path: "/tmp/empty.json"}}, 1, nil
	}
	h.reviews.ReadContentFunc = func(string) ([]byte, error) { return []byte{}, nil }
	code := h.app.Run(t.Context(), []string{"reviews", "--repo=some/repo", "--content"})
	require.Equal(t, ExitValid, code, h.stderr.String())
	var raw map[string]any
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &raw))
	assert.Equal(t, float64(1), raw["unreadable"], "an empty file must be counted, not silently dropped")
	reviews := raw["reviews"].([]any)
	require.Len(t, reviews, 1)
	row := reviews[0].(map[string]any)
	_, hasReview := row["review"]
	assert.False(t, hasReview)
}

// TestReviews_FailedEmptyContentIsDistinctFromNoPath checks the concern
// refinery-a96.36 raised about --failed: a zero-byte kept input reads
// successfully and is wrapped as a string, so it must not collapse into the
// same shape as a row that never had a path to read at all.
func TestReviews_FailedEmptyContentIsDistinctFromNoPath(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	h.reviews.KnownFunc = func(context.Context, string) (bool, error) { return true, nil }
	h.reviews.ListFailedRunsFunc = func(context.Context, string, string, int) ([]store.FailedRun, int, error) {
		return []store.FailedRun{
			{ExitCode: 1, Path: "/tmp/empty-input.json"},
			{ExitCode: 1, Path: ""},
		}, 2, nil
	}
	h.reviews.ReadContentFunc = func(string) ([]byte, error) { return []byte{}, nil }
	code := h.app.Run(t.Context(), []string{"reviews", "--repo=some/repo", "--failed", "--content"})
	require.Equal(t, ExitValid, code, h.stderr.String())
	var raw map[string]any
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &raw))
	failed := raw["failed"].([]any)
	require.Len(t, failed, 2)
	kept := failed[0].(map[string]any)
	review, hasReview := kept["review"]
	require.True(t, hasReview, "a zero-byte kept input is still read and reported")
	assert.Equal(t, "", review)
	notKept := failed[1].(map[string]any)
	_, hasReviewOnMissing := notKept["review"]
	assert.False(t, hasReviewOnMissing, "a row with no path never had a file to read")
	assert.Equal(t, float64(0), raw["unreadable"], "reading succeeded for the one file that was opened")
}

// TestReviews_UnreadableOmittedWhenNoFileWasOpened reproduces refinery-a96.25:
// --content on a query that matched nothing never opens a file, so
// unreadable must be omitted rather than printed as 0.
func TestReviews_UnreadableOmittedWhenNoFileWasOpened(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	h.reviews.KnownFunc = func(context.Context, string) (bool, error) { return false, nil }
	h.reviews.ListReviewsFunc = func(context.Context, string, string, int) ([]store.Review, int, error) {
		return nil, 0, nil
	}
	require.Equal(t, ExitValid, h.app.Run(t.Context(), []string{"reviews", "--repo=example.com/no/such", "--content"}))
	var raw map[string]any
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &raw))
	_, has := raw["unreadable"]
	assert.False(t, has, "no file was opened, so unreadable must not appear")
}

// TestReviews_FailedUnreadableOmittedWhenNoFileWasOpened is
// TestReviews_UnreadableOmittedWhenNoFileWasOpened's --failed counterpart.
func TestReviews_FailedUnreadableOmittedWhenNoFileWasOpened(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	h.reviews.KnownFunc = func(context.Context, string) (bool, error) { return true, nil }
	h.reviews.ListFailedRunsFunc = func(context.Context, string, string, int) ([]store.FailedRun, int, error) {
		return nil, 0, nil
	}
	require.Equal(t, ExitValid, h.app.Run(t.Context(), []string{"reviews", "--repo=some/repo", "--failed", "--content"}))
	var raw map[string]any
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &raw))
	_, has := raw["unreadable"]
	assert.False(t, has, "no file was opened, so unreadable must not appear")
}

// TestReviews_FailedUnreadableOmittedWhenNoRowHasAPath covers rows that did
// come back but every one of them lacks a kept input: --content never opens
// a file, so unreadable must still be omitted (refinery-a96.25).
func TestReviews_FailedUnreadableOmittedWhenNoRowHasAPath(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	h.reviews.KnownFunc = func(context.Context, string) (bool, error) { return true, nil }
	h.reviews.ListFailedRunsFunc = func(context.Context, string, string, int) ([]store.FailedRun, int, error) {
		return []store.FailedRun{{ExitCode: 1, Path: ""}}, 1, nil
	}
	require.Equal(t, ExitValid, h.app.Run(t.Context(), []string{"reviews", "--repo=some/repo", "--failed", "--content"}))
	var raw map[string]any
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &raw))
	_, has := raw["unreadable"]
	assert.False(t, has, "the one row had no path, so no file was opened")
}

func TestReviews_ContentIsUnbudgetedAndUsesTheFullReviewField(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	h.reviews.KnownFunc = func(context.Context, string) (bool, error) { return true, nil }
	h.reviews.ListReviewsFunc = func(context.Context, string, string, int) ([]store.Review, int, error) {
		return []store.Review{{Ref: testRef, Digest: "deadbeef", Verdict: "approve", Path: "/tmp/x.json"}}, 1, nil
	}
	h.reviews.ReadContentFunc = func(string) ([]byte, error) { return []byte(`{"version":"1"}`), nil }
	require.Equal(t, ExitValid, h.app.Run(t.Context(), []string{"reviews", "--repo=some/repo", "--content"}))
	var raw map[string]any
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &raw))
	reviews := raw["reviews"].([]any)
	row := reviews[0].(map[string]any)
	review, ok := row["review"].(map[string]any)
	require.True(t, ok, "a passing review's content is embedded as parsed JSON, not a string")
	assert.Equal(t, "1", review["version"])
}

func TestReviews_ListShape(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	h.reviews.ListReposFunc = func(context.Context) ([]store.RepoCount, error) {
		return []store.RepoCount{{Name: "github.com/x/y", Reviews: 14, Failed: 3}}, nil
	}
	require.Equal(t, ExitValid, h.app.Run(t.Context(), []string{"reviews", "--list"}))
	var payload struct {
		Repos []struct {
			Name    string `json:"name"`
			Reviews int    `json:"reviews"`
			Failed  int    `json:"failed"`
		} `json:"repos"`
	}
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &payload))
	require.Len(t, payload.Repos, 1)
	assert.Equal(t, "github.com/x/y", payload.Repos[0].Name)
	assert.Equal(t, 14, payload.Repos[0].Reviews)
	assert.Equal(t, 3, payload.Repos[0].Failed)
}

func TestReviews_LimitDefaultsToTenAndIsPassedThrough(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	h.reviews.KnownFunc = func(context.Context, string) (bool, error) { return true, nil }
	var gotLimit int
	h.reviews.ListReviewsFunc = func(_ context.Context, _, _ string, limit int) ([]store.Review, int, error) {
		gotLimit = limit
		return nil, 0, nil
	}
	require.Equal(t, ExitValid, h.app.Run(t.Context(), []string{"reviews", "--repo=some/repo"}))
	assert.Equal(t, 10, gotLimit)
	h2 := newHarness(t, "")
	h2.reviews.KnownFunc = func(context.Context, string) (bool, error) { return true, nil }
	h2.reviews.ListReviewsFunc = func(_ context.Context, _, _ string, limit int) ([]store.Review, int, error) {
		gotLimit = limit
		return nil, 0, nil
	}
	require.Equal(t, ExitValid, h2.app.Run(t.Context(), []string{"reviews", "--repo=some/repo", "--limit=0"}))
	assert.Equal(t, 0, gotLimit)
}
