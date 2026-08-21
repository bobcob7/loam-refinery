package cli

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bobcob7/loam-refinery/internal/review"
	"github.com/bobcob7/loam-refinery/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const combinedReviewsDoc = "../../docs/features/combined-reviews.md"

// submissionA and submissionB are docs/features/combined-reviews.md §12.1's
// two stored documents, copied verbatim — the same fixture internal/collect's
// own worked-example test uses, reproduced here because it is unexported
// there and this test drives the real command end to end, not the
// collect-assemble Go value in isolation (the acceptance criteria's own
// distinction: "not just the collect-assemble Go value in isolation").
const submissionA = `{
  "version": "1",
  "verdict": "request_changes",
  "profile": "backend",
  "ref": "4f2c1a9e8b3d7c5a1f0e2d4b6a8c9e1f3a5b7c9d",
  "summary": "The retry loop is sound, but the context deadline is not propagated to the downstream call.",
  "comments": [
    {
      "id": "dropped-context-1",
      "priority": 9,
      "category": "correctness",
      "body": "The retry loop calls c.do(context.Background(), req) rather than passing the caller's ctx.",
      "anchors": [
        { "file": "internal/fetch/client.go", "line": 88, "end_line": 94 }
      ],
      "suggestions": [
        {
          "summary": "Pass the caller's context straight through to c.do",
          "effort": "trivial",
          "scope": "line",
          "pros": ["Cancellation and deadlines propagate immediately"],
          "cons": ["A caller relying on retries outliving the request context sees a behavior change"]
        }
      ]
    }
  ]
}`

const submissionB = `{
  "version": "1",
  "verdict": "comment",
  "profile": "security",
  "ref": "4f2c1a9e8b3d7c5a1f0e2d4b6a8c9e1f3a5b7c9d",
  "summary": "One low-severity logging concern; nothing blocking.",
  "comments": [
    {
      "id": "dropped-context-1",
      "priority": 3,
      "category": "security",
      "body": "The retry loop's debug log includes req.Header verbatim, which can carry an Authorization value on a retried request.",
      "anchors": [
        { "file": "internal/fetch/client.go", "line": 82 }
      ],
      "suggestions": [
        {
          "summary": "Redact known-sensitive headers before logging the request",
          "effort": "small",
          "scope": "file",
          "pros": ["Removes the leak at the one place it can happen"],
          "cons": ["A future header added to the allowlist could reopen this silently"]
        }
      ]
    }
  ]
}`

const ref121 = "4f2c1a9e8b3d7c5a1f0e2d4b6a8c9e1f3a5b7c9d"

const repo121 = "github.com/bobcob7/loam-refinery"

// docs/features/combined-reviews.md §12.2's three stored documents.
const submissionD1 = `{
  "version": "1",
  "verdict": "request_changes",
  "profile": "backend",
  "ref": "9c1a2e4f6b8d0c3a5e7f9b1d3c5a7e9f1b3d5c7a",
  "summary": "Cache invalidation is missing on the write path.",
  "comments": [
    {
      "id": "stale-cache-1",
      "priority": 6,
      "category": "correctness",
      "body": "The write path updates the DB but never invalidates the cache entry.",
      "anchors": [{ "file": "internal/cache/store.go", "line": 41 }],
      "suggestions": []
    }
  ]
}`

const submissionD2 = `{
  "version": "1",
  "verdict": "request_changes",
  "profile": "backend",
  "ref": "9c1a2e4f6b8d0c3a5e7f9b1d3c5a7e9f1b3d5c7a",
  "summary": "Cache invalidation is missing on two write paths, not one.",
  "comments": [
    {
      "id": "stale-cache-1",
      "priority": 8,
      "category": "correctness",
      "body": "The write path updates the DB but never invalidates the cache entry; on closer read, the batch-update path has the same gap.",
      "anchors": [{ "file": "internal/cache/store.go", "line": 41 }],
      "suggestions": []
    },
    {
      "id": "missing-invalidation-1",
      "priority": 8,
      "category": "correctness",
      "body": "Same gap as stale-cache-1, on the batch-update path.",
      "anchors": [{ "file": "internal/cache/store.go", "line": 96 }],
      "suggestions": []
    }
  ]
}`

const submissionD3 = `{
  "version": "1",
  "verdict": "comment",
  "ref": "9c1a2e4f6b8d0c3a5e7f9b1d3c5a7e9f1b3d5c7a",
  "summary": "One stray TODO; nothing blocking.",
  "comments": [
    {
      "id": "todo-left-in-1",
      "priority": 2,
      "category": "style",
      "body": "A TODO from the previous change is still here and looks resolved by this one.",
      "anchors": [{ "file": "internal/cache/store.go", "line": 12 }],
      "suggestions": []
    }
  ]
}`

const ref122 = "9c1a2e4f6b8d0c3a5e7f9b1d3c5a7e9f1b3d5c7a"

const repo122 = "github.com/bobcob7/loam-refinery"

// setCollectReviewsStore configures mock in place (rather than replacing
// h.reviews wholesale) so every test can keep using the harness newHarness
// already wired into h.app. digests is read in the given order; content
// supplies each digest's stored bytes, keyed by digest, absent for a digest
// this test wants to report as unreadable.
func setCollectReviewsStore(t *testing.T, mock *reviewStoreMock, repo, ref string, known, storeEnabled bool, digests []string, content map[string]string) {
	t.Helper()
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	rows := make([]store.DigestRow, 0, len(digests))
	for i, d := range digests {
		rows = append(rows, store.DigestRow{Digest: d, At: at.Add(time.Duration(i) * time.Hour)})
	}
	mock.RepoNameFunc = func(context.Context, string) (string, bool, error) { return repo, true, nil }
	mock.KnownFunc = func(_ context.Context, gotRepo string) (bool, error) {
		require.Equal(t, repo, gotRepo)
		return known, nil
	}
	mock.StoreEnabledFunc = func(context.Context) (bool, error) { return storeEnabled, nil }
	mock.DistinctDigestsFunc = func(_ context.Context, gotRepo, gotRef string) ([]store.DigestRow, error) {
		require.Equal(t, repo, gotRepo)
		require.Equal(t, ref, gotRef)
		return rows, nil
	}
	mock.ReviewPathFunc = func(_ context.Context, _, _, digest string) (string, error) {
		return "/store/" + digest + ".json", nil
	}
	mock.ReadContentFunc = func(path string) ([]byte, error) {
		for _, d := range digests {
			if path == "/store/"+d+".json" {
				body, ok := content[d]
				if !ok {
					return nil, errors.New("no content for digest " + d)
				}
				return []byte(body), nil
			}
		}
		return nil, errors.New("unexpected path " + path)
	}
}

// setHeadCheckStub configures mock to answer every call the same way — one
// ref, one repository state — which is all these tests need: every
// submission collect-reviews reads here was submitted against the ref
// under test.
func setHeadCheckStub(mock *headCheckerMock, source string, isHead bool, diverged []DivergedAnchor) {
	mock.CheckDivergenceFunc = func(context.Context, string, *review.Document, map[string]string) (string, bool, []DivergedAnchor, error) {
		return source, isHead, diverged, nil
	}
}

// asObject decodes stdout as a JSON object.
func asObject(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &out), "stdout was %q", stdout)
	return out
}

// TestCollectReviews_MissingRefExitsUsageNamingIt pins the one flag
// requiredness difference from reviews, where --ref defaults to "all
// refs": collect-reviews has no default, and the message must name --ref
// specifically (docs/features/combined-reviews.md §2). Mutation this
// kills: copy-pasting reviews's flag handling, where --ref is optional,
// would let this exit 0 instead of naming the missing flag.
func TestCollectReviews_MissingRefExitsUsageNamingIt(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	code := h.app.Run(t.Context(), []string{"collect-reviews", "--repo=some/repo"})
	assert.Equal(t, ExitUsage, code)
	assert.Contains(t, h.stderr.String(), "--ref")
	assert.Empty(t, h.reviews.KnownCalls(), "a usage error must never reach the store")
}

// TestCollectReviews_MalformedRefExitsUsage pins §9's "malformed --ref"
// row. Mutation this kills: skipping store.ValidateRef would let a
// too-short or uppercase ref reach the store instead of exiting 2.
func TestCollectReviews_MalformedRefExitsUsage(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	code := h.app.Run(t.Context(), []string{"collect-reviews", "--ref=not-a-sha"})
	assert.Equal(t, ExitUsage, code)
	assert.Contains(t, h.stderr.String(), "--ref")
}

// TestCollectReviews_MalformedRepoExitsUsage pins §9's "malformed --repo"
// row, reusing resolveRepo the same way reviews does.
func TestCollectReviews_MalformedRepoExitsUsage(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	code := h.app.Run(t.Context(), []string{"collect-reviews", "--ref=" + testRef, "--repo=UPPER CASE"})
	assert.Equal(t, ExitUsage, code)
	assert.Contains(t, h.stderr.String(), "--repo")
}

// TestCollectReviews_FormatTextExitsUsage pins §2's "the one place the
// flag survives to reject it": --format=text is still an error on
// collect-reviews even though the flag itself is gone from every other
// command. Mutation this kills: accepting any string (or only rejecting an
// empty one) would let "text" silently mean "json".
func TestCollectReviews_FormatTextExitsUsage(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	code := h.app.Run(t.Context(), []string{"collect-reviews", "--ref=" + testRef, "--format=text"})
	assert.Equal(t, ExitUsage, code)
	assert.Contains(t, h.stderr.String(), "--format")
	assert.Empty(t, h.reviews.KnownCalls(), "a usage error must never reach the store")
}

// TestCollectReviews_FormatMarkdownIsAcceptedAtFlagParsing pins the
// acceptance criteria's own wording: markdown is accepted at the
// flag-parsing level. Mutation this kills: rejecting "markdown" the way
// "text" is rejected would make --format=markdown a usage error.
func TestCollectReviews_FormatMarkdownIsAcceptedAtFlagParsing(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	setCollectReviewsStore(t, h.reviews, "some/repo", testRef, true, true, nil, nil)
	code := h.app.Run(t.Context(), []string{"collect-reviews", "--ref=" + testRef, "--repo=some/repo", "--format=markdown"})
	assert.Equal(t, ExitValid, code, h.stderr.String())
}

// TestCollectReviews_FormatMarkdownRendersMarkdownNotJSON pins the
// refinery-uyb.12 wiring: --format=markdown must reach
// render.Markdown.CollectReviews, not the JSON path silently used for
// every format the way it was before this bead. Mutation this kills:
// collectReviewsRun ignoring format and always calling a.renderer's JSON
// path would still exit 0 here, but stdout would be JSON, not Markdown.
func TestCollectReviews_FormatMarkdownRendersMarkdownNotJSON(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	setCollectReviewsStore(t, h.reviews, repo121, ref121, true, true,
		[]string{"digest-a", "digest-b"},
		map[string]string{"digest-a": submissionA, "digest-b": submissionB},
	)
	setHeadCheckStub(h.headChecker, "repo", true, []DivergedAnchor{})
	code := h.app.Run(t.Context(), []string{"collect-reviews", "--ref=" + ref121, "--repo=" + repo121, "--format=markdown"})
	require.Equal(t, ExitValid, code, h.stderr.String())
	stdout := h.stdout.String()
	assert.True(t, strings.HasPrefix(stdout, "# collect-reviews:"), "markdown output, not a JSON object: %q", stdout)
	assert.Contains(t, stdout, "## backend:dropped-context-1")
	assert.Contains(t, stdout, "## security:dropped-context-1")
	assert.NotEqual(t, byte('{'), stdout[0], "must not be the JSON envelope")
}

// TestCollectReviews_FormatJSONStillRendersJSON is the negative case
// alongside the test above: the default and explicit "json" formats must
// still go through the unchanged JSON path once collectReviewsRun starts
// branching on format.
func TestCollectReviews_FormatJSONStillRendersJSON(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	setCollectReviewsStore(t, h.reviews, "some/repo", testRef, true, true, nil, nil)
	setHeadCheckStub(h.headChecker, "none", false, nil)
	code := h.app.Run(t.Context(), []string{"collect-reviews", "--ref=" + testRef, "--repo=some/repo", "--format=json"})
	require.Equal(t, ExitValid, code, h.stderr.String())
	got := asObject(t, h.stdout.String())
	assert.Contains(t, got, "ref")
}

// TestCollectReviewsEmptyCases pins every exit-0 row of §9's table that is
// distinguishable at this layer — known with nothing stored, an unknown
// repository (which stands in for both "repository not in the store" and
// "store does not exist at all": setCollectReviewsStore's mock only ever
// exposes Known and StoreEnabled as booleans, so those two §9 rows drive
// this command through the identical known:false, storeEnabled:true input
// and cannot be told apart by any assertion this test could make — a
// distinct row here would be two names for one test, not two tests), and
// store.enabled:false. All produce the identical known/total/submissions/
// comments shape (empty) except for known and store.enabled themselves,
// which differ per row. store.enabled:false gets its own explicit
// assertion (mutation this kills: a special-cased branch for
// store.enabled:false — the exact over-specification §9 warns against by
// name — would diverge this row's shape from the others, e.g. by
// short-circuiting before DistinctDigests is even called).
func TestCollectReviewsEmptyCases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		known        bool
		storeEnabled bool
	}{
		{name: "known repo, ref has no stored submissions", known: true, storeEnabled: true},
		{name: "repository not in the store (same shape as no store at all)", known: false, storeEnabled: true},
		{name: "store.enabled is false, same empty shape as no store", known: false, storeEnabled: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, "")
			setCollectReviewsStore(t, h.reviews, "some/repo", testRef, test.known, test.storeEnabled, nil, nil)
			setHeadCheckStub(h.headChecker, "none", false, nil)
			code := h.app.Run(t.Context(), []string{"collect-reviews", "--ref=" + testRef, "--repo=some/repo"})
			require.Equal(t, ExitValid, code, h.stderr.String())
			got := asObject(t, h.stdout.String())
			repoObj := got["repo"].(map[string]any)
			assert.Equal(t, test.known, repoObj["known"])
			storeObj := got["store"].(map[string]any)
			assert.Equal(t, test.storeEnabled, storeObj["enabled"])
			assert.Equal(t, float64(0), got["total"])
			assert.Equal(t, float64(0), got["unreadable"])
			assert.Empty(t, got["submissions"])
			assert.Empty(t, got["comments"])
			assert.NotNil(t, got["submissions"], "submissions must be [], never omitted or null")
			assert.NotNil(t, got["comments"], "comments must be [], never omitted or null")
		})
	}
}

// TestCollectReviews_DatabaseCannotBeOpenedExitsTool pins §9's last row:
// exit 101, not 2 or 1 — a tool failure, not a caller mistake or a finding
// about the review.
func TestCollectReviews_DatabaseCannotBeOpenedExitsTool(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	h.reviews.KnownFunc = func(context.Context, string) (bool, error) {
		return false, errors.New("store.db: disk I/O error")
	}
	code := h.app.Run(t.Context(), []string{"collect-reviews", "--ref=" + testRef, "--repo=some/repo"})
	assert.Equal(t, ExitTool, code)
	assert.Contains(t, h.stderr.String(), "disk I/O error")
}

// TestCollectReviews_UnreadableFileIsCountedNotDropped pins config.md
// §6.3's "a silent skip would let a store quietly lose reviews": one of two
// stored digests cannot be read, and the envelope must still say total: 2,
// unreadable: 1 — never silently reporting total: 1 as though the second
// submission never existed. Mutation this kills: forgetting to add
// Result.Unreadable into Total (rendering Total as len(Submissions) alone)
// would make this pass with total: 1, hiding the loss the counter exists to
// surface.
func TestCollectReviews_UnreadableFileIsCountedNotDropped(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	setCollectReviewsStore(t, h.reviews, "some/repo", testRef, true, true,
		[]string{"digest-a", "digest-b"},
		map[string]string{"digest-a": submissionA},
	)
	setHeadCheckStub(h.headChecker, "none", false, nil)
	code := h.app.Run(t.Context(), []string{"collect-reviews", "--ref=" + testRef, "--repo=some/repo"})
	require.Equal(t, ExitValid, code, h.stderr.String())
	got := asObject(t, h.stdout.String())
	assert.Equal(t, float64(2), got["total"], "total counts every distinct digest, before any are dropped as unreadable")
	assert.Equal(t, float64(1), got["unreadable"])
	submissions, ok := got["submissions"].([]any)
	require.True(t, ok)
	require.Len(t, submissions, 1, "the one readable submission still appears")
}

// TestCollectReviews_DivergedKeyAbsentWhenNotHead is one of the four
// JSON-shape assertions moved onto this bead from refinery-uyb.10:
// head_check.diverged must be ABSENT from the encoded JSON — not null, not
// [] — when is_head is false. Mutation this kills: rendering Diverged as a
// plain (non-pointer) slice, or using ordinary encoding/json omitempty on
// it, would let a nil slice and a non-nil empty one collapse to the same
// omission, and separately, forgetting the omission path entirely would
// print "diverged": null.
func TestCollectReviews_DivergedKeyAbsentWhenNotHead(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	setCollectReviewsStore(t, h.reviews, "some/repo", testRef, true, true, nil, nil)
	setHeadCheckStub(h.headChecker, "repo", false, nil)
	code := h.app.Run(t.Context(), []string{"collect-reviews", "--ref=" + testRef, "--repo=some/repo"})
	require.Equal(t, ExitValid, code, h.stderr.String())
	got := asObject(t, h.stdout.String())
	headCheck := got["head_check"].(map[string]any)
	_, hasDiverged := headCheck["diverged"]
	assert.False(t, hasDiverged, "diverged must be absent from the encoded JSON entirely, not present as null")
	_, hasIsHead := headCheck["is_head"]
	assert.True(t, hasIsHead, "is_head is present whenever source == \"repo\", regardless of its value")
	assert.Equal(t, false, headCheck["is_head"])
}

// TestCollectReviews_DivergedKeyIsAnEmptyArrayWhenHeadAndNothingDiverged is
// the second of the four JSON-shape assertions: once the check actually
// ran and found nothing, diverged must be present as [], not omitted and
// not null — "the populated case", as distinct from the absent case above.
func TestCollectReviews_DivergedKeyIsAnEmptyArrayWhenHeadAndNothingDiverged(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	setCollectReviewsStore(t, h.reviews, "some/repo", testRef, true, true, nil, nil)
	setHeadCheckStub(h.headChecker, "repo", true, []DivergedAnchor{})
	code := h.app.Run(t.Context(), []string{"collect-reviews", "--ref=" + testRef, "--repo=some/repo"})
	require.Equal(t, ExitValid, code, h.stderr.String())
	got := asObject(t, h.stdout.String())
	headCheck := got["head_check"].(map[string]any)
	diverged, hasDiverged := headCheck["diverged"]
	require.True(t, hasDiverged, "diverged must be present once the check ran, even with nothing to report")
	assert.Equal(t, []any{}, diverged)
}

// TestCollectReviews_DivergedPopulatedFieldsFlowThroughUnscrambled is the
// populated-diverged case the absent/empty tests above leave unpinned
// (refinery-hxc): with a real drifted anchor, head_check.diverged in the
// encoded JSON must carry every field keyed to its own JSON name, not some
// other field's value. Each of the four DivergedAnchor fields is given a
// distinct, recognizable value so no permutation of the four could satisfy
// this test by accident.
//
// Mutation this kills: convertDiverged replaced with an empty slice (every
// drifted anchor silently dropped), and convertDiverged scrambled so Name
// carries the message and Comment carries the file, both leave this red.
func TestCollectReviews_DivergedPopulatedFieldsFlowThroughUnscrambled(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	setCollectReviewsStore(t, h.reviews, "some/repo", testRef, true, true, nil, nil)
	setHeadCheckStub(h.headChecker, "repo", true, []DivergedAnchor{{
		Name:    "anchor-worktree-diverged",
		Comment: "backend:dropped-context-1",
		File:    "internal/fetch/client.go",
		Message: "the anchored line no longer matches the working tree",
	}})
	code := h.app.Run(t.Context(), []string{"collect-reviews", "--ref=" + testRef, "--repo=some/repo"})
	require.Equal(t, ExitValid, code, h.stderr.String())
	got := asObject(t, h.stdout.String())
	headCheck := got["head_check"].(map[string]any)
	diverged, ok := headCheck["diverged"].([]any)
	require.True(t, ok)
	require.Len(t, diverged, 1)
	entry := diverged[0].(map[string]any)
	assert.Equal(t, "anchor-worktree-diverged", entry["name"])
	assert.Equal(t, "backend:dropped-context-1", entry["comment"])
	assert.Equal(t, "internal/fetch/client.go", entry["file"])
	assert.Equal(t, "the anchored line no longer matches the working tree", entry["message"])
}

// TestCollectReviews_HeadCheckMergesDivergedAcrossSubmissionsUsingFirstsSourceAndIsHead
// drives collectHeadCheck's cross-submission merge (§4.3) with two
// submissions whose headChecker answers genuinely differ — unlike every
// other test in this file, which stubs one identical answer for every
// call via setHeadCheckStub and so cannot tell a merge from a pick
// (refinery-hxc).
//
// Mutation this kills: replacing the diverged concatenation with "take the
// last answer" would drop the first submission's entry from the merged
// list; moving the i==0 source/is_head capture to the last submission
// would report is_head:false (security's answer) instead of true
// (backend's, the first submission processed).
func TestCollectReviews_HeadCheckMergesDivergedAcrossSubmissionsUsingFirstsSourceAndIsHead(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	setCollectReviewsStore(t, h.reviews, repo121, ref121, true, true,
		[]string{"digest-a", "digest-b"},
		map[string]string{"digest-a": submissionA, "digest-b": submissionB},
	)
	h.headChecker.CheckDivergenceFunc = func(_ context.Context, _ string, doc *review.Document, _ map[string]string) (string, bool, []DivergedAnchor, error) {
		if doc.Profile.Value == "backend" {
			return "repo", true, []DivergedAnchor{{Name: "anchor-worktree-diverged", Comment: "backend:dropped-context-1", File: "a.go", Message: "from backend's submission"}}, nil
		}
		return "repo", false, []DivergedAnchor{{Name: "anchor-worktree-diverged", Comment: "security:dropped-context-1", File: "b.go", Message: "from security's submission"}}, nil
	}
	code := h.app.Run(t.Context(), []string{"collect-reviews", "--ref=" + ref121, "--repo=" + repo121})
	require.Equal(t, ExitValid, code, h.stderr.String())
	got := asObject(t, h.stdout.String())
	headCheck := got["head_check"].(map[string]any)
	assert.Equal(t, true, headCheck["is_head"], "is_head is the FIRST submission's answer (backend, true), not the last's (security, false)")
	diverged, ok := headCheck["diverged"].([]any)
	require.True(t, ok)
	require.Len(t, diverged, 2, "both submissions' diverged entries are concatenated, not just the last one")
	messages := make([]string, 0, 2)
	for _, d := range diverged {
		messages = append(messages, d.(map[string]any)["message"].(string))
	}
	assert.Contains(t, messages, "from backend's submission")
	assert.Contains(t, messages, "from security's submission")
}

// TestCollectReviews_MatchesDocumentedShape_TwoProfilesOneRef is the third
// of the four JSON-shape assertions: the populated case must match §12.1's
// worked example exactly — not just in shape, but value for value, since
// the fixture below is that example's own two stored documents, copied
// verbatim. This is also the acceptance criteria's §12.1 docs-shape parity
// test, driven through the real command rather than the collect-assemble
// Go value in isolation.
func TestCollectReviews_MatchesDocumentedShape_TwoProfilesOneRef(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	setCollectReviewsStore(t, h.reviews, repo121, ref121, true, true,
		[]string{"digest-a", "digest-b"},
		map[string]string{"digest-a": submissionA, "digest-b": submissionB},
	)
	setHeadCheckStub(h.headChecker, "repo", true, []DivergedAnchor{})
	code := h.app.Run(t.Context(), []string{"collect-reviews", "--ref=" + ref121, "--repo=" + repo121})
	require.Equal(t, ExitValid, code, h.stderr.String())
	got := realJSON(t, h.stdout.String())
	want := docJSONBlock(t, combinedReviewsDoc, "### 12.1 Two profiles, one ref", 3)
	assertShapeMatchesDoc(t, got, combinedReviewsDoc, "### 12.1 Two profiles, one ref", 3,
		"docs/features/combined-reviews.md §12.1: the envelope's shape must match the documented example")
	assert.Equal(t, want, got, "docs/features/combined-reviews.md §12.1: the populated envelope must match the worked example exactly, value for value")
}

// TestCollectReviews_MatchesDocumentedShape_RevisedAndUnprofiled is the
// fourth of the four JSON-shape assertions and the acceptance criteria's
// §12.2 docs-shape parity test: one profile revised, superseding its own
// earlier submission, plus one unprofiled submission — and, since this
// ref is not HEAD, head_check.diverged is absent, exercised here at the
// full-envelope level rather than in isolation.
func TestCollectReviews_MatchesDocumentedShape_RevisedAndUnprofiled(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	setCollectReviewsStore(t, h.reviews, repo122, ref122, true, true,
		[]string{"d1", "d2", "d3"},
		map[string]string{"d1": submissionD1, "d2": submissionD2, "d3": submissionD3},
	)
	setHeadCheckStub(h.headChecker, "repo", false, nil)
	code := h.app.Run(t.Context(), []string{"collect-reviews", "--ref=" + ref122, "--repo=" + repo122})
	require.Equal(t, ExitValid, code, h.stderr.String())
	got := realJSON(t, h.stdout.String())
	want := docJSONBlock(t, combinedReviewsDoc, "### 12.2 One profile, revised, plus one unprofiled submission", 1)
	assertShapeMatchesDoc(t, got, combinedReviewsDoc, "### 12.2 One profile, revised, plus one unprofiled submission", 1,
		"docs/features/combined-reviews.md §12.2: the envelope's shape must match the documented example")
	assert.Equal(t, want, got, "docs/features/combined-reviews.md §12.2: the envelope must match the worked example exactly, value for value")
}
