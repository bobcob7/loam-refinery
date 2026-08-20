package cli

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bobcob7/loam-refinery/internal/store"
	"github.com/stretchr/testify/require"
)

const configDoc = "../../docs/config.md"

// TestReviewsDefaultIndexMatchesDocumentedShape reproduces docs/config.md
// §6.1's default index example for real — a row with every field the
// default form ever prints, so a field the docs show and the tool no
// longer prints (or vice versa) fails here rather than surviving the way
// refinery-a96.21's three-vs-four counts keys did.
func TestReviewsDefaultIndexMatchesDocumentedShape(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	at := time.Date(2026, 8, 19, 14, 30, 5, 0, time.UTC)
	comments, errs, advisories, skipped := 6, 0, 2, 0
	h.reviews.KnownFunc = func(context.Context, string) (bool, error) { return true, nil }
	h.reviews.ListReviewsFunc = func(context.Context, string, string, int) ([]store.Review, int, error) {
		return []store.Review{{
			At: at, Ref: testRef, Digest: "deadbeef", Verdict: "request_changes",
			Counts: store.Counts{Comments: &comments, Errors: &errs, Advisories: &advisories, Skipped: &skipped},
			Path:   "/tmp/x.json",
		}}, 14, nil
	}
	require.Equal(t, ExitValid, h.app.Run(t.Context(), []string{"reviews", "--repo=some/repo"}))
	assertShapeMatchesDoc(t, realJSON(t, h.stdout.String()), configDoc, "### 6.1 Output", 1,
		"docs/config.md §6.1: the default index envelope's shape must match the documented example")
}

// TestReviewsFailedRowMatchesDocumentedShape reproduces the bare --failed
// row docs/config.md §6.1 shows on its own, with both ref and path present
// (the row is shown with a run that has both) — the third JSON block after
// the heading, since §6.1 shows the default row and its envelope first.
func TestReviewsFailedRowMatchesDocumentedShape(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	at := time.Date(2026, 8, 19, 14, 22, 41, 0, time.UTC)
	comments, errs, advisories, skipped := 3, 2, 1, 0
	h.reviews.KnownFunc = func(context.Context, string) (bool, error) { return true, nil }
	h.reviews.ListFailedRunsFunc = func(context.Context, string, string, int) ([]store.FailedRun, int, error) {
		return []store.FailedRun{{
			At: at, Ref: testRef, ExitCode: 1,
			Counts: store.Counts{Comments: &comments, Errors: &errs, Advisories: &advisories, Skipped: &skipped},
			Path:   "/tmp/rejected.json",
		}}, 3, nil
	}
	require.Equal(t, ExitValid, h.app.Run(t.Context(), []string{"reviews", "--repo=some/repo", "--failed"}))
	got := realJSON(t, h.stdout.String())
	row := got.(map[string]any)["failed"].([]any)[0]
	assertShapeMatchesDoc(t, row, configDoc, "### 6.1 Output", 2,
		"docs/config.md §6.1: a --failed row with both ref and path must match the documented bare-row example")
}

// TestReviewsFailedEnvelopeMatchesDocumentedShape reproduces docs/config.md
// §6.1's statement that --failed is wrapped exactly like the default
// index — repo, total, then the array, named failed in place of reviews.
// The doc's own example elides the array's contents with a comment, so
// this checks the envelope alone and leaves the row itself to
// TestReviewsFailedRowMatchesDocumentedShape above.
func TestReviewsFailedEnvelopeMatchesDocumentedShape(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	h.reviews.KnownFunc = func(context.Context, string) (bool, error) { return true, nil }
	h.reviews.ListFailedRunsFunc = func(context.Context, string, string, int) ([]store.FailedRun, int, error) {
		return nil, 0, nil
	}
	require.Equal(t, ExitValid, h.app.Run(t.Context(), []string{"reviews", "--repo=some/repo", "--failed"}))
	assertShapeMatchesDoc(t, realJSON(t, h.stdout.String()), configDoc, "### 6.1 Output", 3,
		"docs/config.md §6.1: the --failed envelope must match repo, total, failed — the same wrapper as the default index")
}

// TestReviewsListMatchesDocumentedShape reproduces docs/config.md §6.1's
// --list example: one repository backed by real reviews, one with none —
// exactly the two rows the doc shows, so both known cases are covered.
func TestReviewsListMatchesDocumentedShape(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	h.reviews.ListReposFunc = func(context.Context) ([]store.RepoCount, error) {
		return []store.RepoCount{
			{Name: "github.com/bobcob7/loam-refinery", Reviews: 14, Failed: 3},
			{Name: "no-repo", Reviews: 0, Failed: 7},
		}, nil
	}
	require.Equal(t, ExitValid, h.app.Run(t.Context(), []string{"reviews", "--list"}))
	assertShapeMatchesDoc(t, realJSON(t, h.stdout.String()), configDoc, "### 6.1 Output", 4,
		"docs/config.md §6.1: the --list envelope must match the documented example")
}

// TestReviewsUnreadableKeyNameMatchesDocumented pins docs/config.md
// §6.1/§6.3's exact key name for the unreadable-file count: "unreadable",
// present only once --content has actually opened a file. The doc shows
// only the bare fragment `"unreadable": 2` rather than a full envelope, so
// this checks the one thing that fragment states — the key's name — by
// wrapping it the same way it would sit inside a real envelope, and
// confirms a real --content run that hits an unreadable file uses that
// same name.
func TestReviewsUnreadableKeyNameMatchesDocumented(t *testing.T) {
	t.Parallel()
	docKey := docJSONFragment(t, configDoc, "### 6.3 Missing and foreign files", 1).(map[string]any)
	_, hasUnreadable := docKey["unreadable"]
	require.True(t, hasUnreadable, "docs/config.md §6.3's example must itself be named \"unreadable\"")
	h := newHarness(t, "")
	h.reviews.KnownFunc = func(context.Context, string) (bool, error) { return true, nil }
	h.reviews.ListReviewsFunc = func(context.Context, string, string, int) ([]store.Review, int, error) {
		return []store.Review{{Ref: testRef, Digest: "deadbeef", Verdict: "approve", Path: "/tmp/gone.json"}}, 1, nil
	}
	h.reviews.ReadContentFunc = func(string) ([]byte, error) { return nil, errors.New("no such file") }
	require.Equal(t, ExitValid, h.app.Run(t.Context(), []string{"reviews", "--repo=some/repo", "--content"}))
	got := realJSON(t, h.stdout.String()).(map[string]any)
	_, hasKey := got["unreadable"]
	require.True(t, hasKey, "docs/config.md §6.3: a run that opened an unreadable file must report \"unreadable\"")
}
