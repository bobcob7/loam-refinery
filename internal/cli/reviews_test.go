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

func TestReviews_FailedOmitsRefAndPathWhenAbsent(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	at := time.Date(2026, 8, 19, 14, 22, 41, 0, time.UTC)
	h.reviews.KnownFunc = func(context.Context, string) (bool, error) { return true, nil }
	h.reviews.ListFailedRunsFunc = func(context.Context, string, string, int) ([]store.FailedRun, int, error) {
		return []store.FailedRun{{At: at, ExitCode: 1, Path: ""}}, 1, nil
	}
	require.Equal(t, ExitValid, h.app.Run(t.Context(), []string{"reviews", "--repo=some/repo", "--failed"}))
	var raw map[string]any
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &raw))
	failed := raw["failed"].([]any)
	require.Len(t, failed, 1)
	row := failed[0].(map[string]any)
	_, hasRef := row["ref"]
	_, hasPath := row["path"]
	assert.False(t, hasRef, "ref must be absent, not null, when the run has none")
	assert.False(t, hasPath, "path must be absent, not null, when the input was not kept")
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
