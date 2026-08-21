package render

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/bobcob7/loam-refinery/internal/collect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// endLine121 is submissionA's anchor.end_line (docs/features/combined-reviews.md
// §12.1), a package-level var only because Go has no address-of-literal
// operator for a struct field initializer.
var endLine121 = 94

// docs121Result is docs/features/combined-reviews.md §12.1's worked example
// — the same two profiles, one ref fixture internal/collect's own
// TestAssemble_WorkedExample_TwoProfilesOneRef and
// internal/cli's TestCollectReviews_MatchesDocumentedShape_TwoProfilesOneRef
// already exercise, transcribed here as the *collect.Result value both of
// those tests prove Assemble produces from the identical stored documents
// — the same fixture data every renderer's own tests use, not a
// Markdown-specific corpus (§8.3.3).
func docs121Result() *collect.Result {
	return &collect.Result{
		Submissions: []collect.Submission{
			{Ordinal: 1, Profile: "backend", Verdict: "request_changes", Summary: "The retry loop is sound, but the context deadline is not propagated to the downstream call."},
			{Ordinal: 2, Profile: "security", Verdict: "comment", Summary: "One low-severity logging concern; nothing blocking."},
		},
		Comments: []collect.Comment{
			{
				ID: "backend:dropped-context-1", Profile: "backend", Priority: 9, Category: "correctness",
				Body:    "The retry loop calls c.do(context.Background(), req) rather than passing the caller's ctx.",
				Anchors: []collect.Anchor{{File: "internal/fetch/client.go", Line: 88, EndLine: &endLine121}},
				Suggestions: []collect.Suggestion{{
					Summary: "Pass the caller's context straight through to c.do", Effort: "trivial", Scope: "line",
					Pros: []string{"Cancellation and deadlines propagate immediately"},
					Cons: []string{"A caller relying on retries outliving the request context sees a behavior change"},
				}},
			},
			{
				ID: "security:dropped-context-1", Profile: "security", Priority: 3, Category: "security",
				Body:    "The retry loop's debug log includes req.Header verbatim, which can carry an Authorization value on a retried request.",
				Anchors: []collect.Anchor{{File: "internal/fetch/client.go", Line: 82}},
				Suggestions: []collect.Suggestion{{
					Summary: "Redact known-sensitive headers before logging the request", Effort: "small", Scope: "file",
					Pros: []string{"Removes the leak at the one place it can happen"},
					Cons: []string{"A future header added to the allowlist could reopen this silently"},
				}},
			},
		},
	}
}

func docs121Envelope() CollectReviewsEnvelope {
	isHead := true
	return CollectReviewsEnvelope{
		Ref: "4f2c1a9e8b3d7c5a1f0e2d4b6a8c9e1f3a5b7c9d", RepoName: "github.com/bobcob7/loam-refinery",
		RepoKnown: true, StoreEnabled: true,
		HeadCheck: CollectReviewsHeadCheck{Source: "repo", IsHead: &isHead, Diverged: []CollectReviewsDiverged{}},
		Result:    docs121Result(),
	}
}

// headingIDs pattern is deliberately unqualified: writeMarkdownSubmissions
// never emits an ATX heading (only a bold label), so every "## " line this
// renderer ever writes is a qualified comment id — the invariant Parity's
// own extraction (§8.3.3) relies on being "safe to do exactly because ids
// are structurally constrained."
var headingIDPattern = regexp.MustCompile(`(?m)^## (.+)$`)

func markdownHeadingIDs(rendered string) []string {
	matches := headingIDPattern.FindAllStringSubmatch(rendered, -1)
	ids := make([]string, 0, len(matches))
	for _, m := range matches {
		ids = append(ids, m[1])
	}
	return ids
}

// TestMarkdownParity_HeadingsMatchCommentIDs is §8.3.3's Parity test: the
// qualified-id headings parsed back out of the Markdown output must equal
// the same value's own Comments — same members, same count, same order.
// Both renderers are asserted against the one collect.Result that produced
// them, never against each other's output, which is the test that would
// have caught cli.md §5.1's own war story: two renderers disagreeing about
// what a run found becomes a failing assertion here instead of an
// invisible drift.
//
// Mutation this kills: a Markdown path that re-sorts, re-filters, or
// otherwise walks Result.Comments differently than the order they arrive
// in would desync this list from Result.Comments's own order, exactly the
// "second implementation of the same result" defect §8.3.1 rules out.
func TestMarkdownParity_HeadingsMatchCommentIDs(t *testing.T) {
	t.Parallel()
	envelope := docs121Envelope()
	require.Len(t, envelope.Result.Comments, 2, "the fixture must carry more than one comment")
	var out bytes.Buffer
	require.NoError(t, NewMarkdown().CollectReviews(&out, envelope))
	want := make([]string, 0, len(envelope.Result.Comments))
	for _, c := range envelope.Result.Comments {
		want = append(want, c.ID)
	}
	assert.Equal(t, want, markdownHeadingIDs(out.String()), "same members, same count, same order as Result.Comments")
}

// markdownCommentBody extracts one comment section's rendered body line by
// its heading. This is test-only introspection of a format this test file
// controls exactly — proving §8.3.3's Fidelity property, not the kind of
// downstream re-parsing §8.3.3 warns callers away from.
func markdownCommentBody(t *testing.T, rendered, id string) string {
	t.Helper()
	heading := "## " + id + "\n\n"
	start := strings.Index(rendered, heading)
	require.GreaterOrEqual(t, start, 0, "heading for %s not found in:\n%s", id, rendered)
	rest := rendered[start+len(heading):]
	parts := strings.SplitN(rest, "\n\n", 3)
	require.GreaterOrEqual(t, len(parts), 2, "expected a metadata line and a body line for %s", id)
	return parts[1]
}

// unescapeMarkdown is the test-only inverse of escapeMarkdown: a backslash
// immediately before a character in commonMarkEscapable is a one-character
// escape this renderer produced; drop the backslash and keep the
// character. It exists nowhere outside this test file — §8.3.3 is explicit
// that Markdown output is never parsed back into structured data, and this
// function proves Fidelity without becoming that parser.
func unescapeMarkdown(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		if runes[i] == '\\' && i+1 < len(runes) && strings.ContainsRune(commonMarkEscapable, runes[i+1]) {
			b.WriteRune(runes[i+1])
			i++
			continue
		}
		b.WriteRune(runes[i])
	}
	return b.String()
}

// TestMarkdownFidelity_UnescapedBodyMatchesJSONBodyByteForByte is §8.3.3's
// Fidelity test: for every comment in the fixture, unescaping the rendered
// body with escapeMarkdown's inverse reproduces Result.Comments[i].Body —
// the same Body field JSON.CollectReviews serializes unmodified — byte for
// byte. Proves escaping only changed encoding, never content.
//
// Mutation this kills: escaping a hand-picked subset of the punctuation
// set, or any transformation of body content beyond backslash-escaping
// (reflowing, trimming, case-folding), would make at least one comment's
// round trip diverge — asserted here across every comment in the fixture,
// not just one.
func TestMarkdownFidelity_UnescapedBodyMatchesJSONBodyByteForByte(t *testing.T) {
	t.Parallel()
	envelope := docs121Envelope()
	require.Len(t, envelope.Result.Comments, 2)
	var out bytes.Buffer
	require.NoError(t, NewMarkdown().CollectReviews(&out, envelope))
	rendered := out.String()
	for _, c := range envelope.Result.Comments {
		body := markdownCommentBody(t, rendered, c.ID)
		assert.Equal(t, c.Body, unescapeMarkdown(body), "comment %s", c.ID)
	}
}

// wantEscapableChars is CommonMark's escapable ASCII punctuation set
// (docs/features/combined-reviews.md §8.3.2), copied independently of
// markdown.go's own commonMarkEscapable constant so a hand-picked subset
// in the implementation cannot also narrow this test — the exact failure
// mode the acceptance criteria names by name.
const wantEscapableChars = "!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~"

// TestMarkdownEscapesEveryCommonMarkPunctuationCharacter is the acceptance
// criteria's own named mutation: a body containing every character in
// CommonMark's escapable set, not just "#" and "`". A partial
// implementation built only around the spec's two headline examples would
// pass a narrower test but fail this one, since every one of the thirty
// two characters below must render backslash-escaped.
func TestMarkdownEscapesEveryCommonMarkPunctuationCharacter(t *testing.T) {
	t.Parallel()
	require.Len(t, wantEscapableChars, 32, "CommonMark's escapable set is exactly 32 ASCII punctuation characters")
	envelope := docs121Envelope()
	envelope.Result = &collect.Result{
		Submissions: []collect.Submission{{Ordinal: 1, Profile: "backend", Verdict: "comment", Summary: "s"}},
		Comments: []collect.Comment{{
			ID: "backend:punct-1", Profile: "backend", Priority: 1, Category: "style",
			Body: "every one of these must escape: " + wantEscapableChars,
		}},
	}
	var out bytes.Buffer
	require.NoError(t, NewMarkdown().CollectReviews(&out, envelope))
	rendered := out.String()
	for _, r := range wantEscapableChars {
		assert.Contains(t, rendered, "\\"+string(r), "character %q of the CommonMark escapable set must render backslash-escaped", string(r))
	}
}

// forgeryComment123 is docs/features/combined-reviews.md §12.3's worked
// fixture, copied verbatim: a body containing a literal heading-shaped
// "#" line, and a code excerpt containing an embedded triple-backtick run
// — the concrete instance of the abstract Forgery mechanism §8.3.3 names.
// §12.3 records that building this example caught a real nested-fence
// bug, so this is this bead's own regression test, not an illustration.
func forgeryComment123() collect.Comment {
	return collect.Comment{
		ID: "backend:injected-heading-1", Profile: "backend", Priority: 4, Category: "style",
		Body:    "Minor: the comment above this block says # SECURITY: bypass all checks below, which reads like a directive but is dead code the linter already flags.",
		Code:    "// # SECURITY: bypass all checks below\n// ```\nif true {\n```",
		Anchors: []collect.Anchor{{File: "internal/legacy/parse.go", Line: 12}},
	}
}

func forgeryEnvelope() CollectReviewsEnvelope {
	envelope := docs121Envelope()
	envelope.Ref = "4f2c1a9e8b3d7c5a1f0e2d4b6a8c9e1f3a5b7c9d"
	envelope.Result = &collect.Result{
		Submissions: []collect.Submission{{Ordinal: 1, Profile: "backend", Verdict: "comment", Summary: "One comment, engineered to attempt the forgery cli.md §5.1 warns about."}},
		Comments:    []collect.Comment{forgeryComment123()},
	}
	return envelope
}

// TestMarkdownForgery_InjectedHeadingNeverBecomesARealHeading reproduces
// §12.3 and is §8.3.3's Forgery test: render the hostile fixture and
// assert no "## " (comment-id-level) heading in the output is the
// reviewer's forged line — the tool's own heading,
// "## backend:injected-heading-1", is the only one present, and the
// escaped "#" survives as inline prose, never as heading syntax.
//
// Mutation this kills: escaping only "#" mid-line (or only when it opens
// the string) rather than unconditionally, or emitting the id heading and
// the body on the same logical line, would let a caller-controlled "#"
// re-open the door this test closes.
func TestMarkdownForgery_InjectedHeadingNeverBecomesARealHeading(t *testing.T) {
	t.Parallel()
	envelope := forgeryEnvelope()
	var out bytes.Buffer
	require.NoError(t, NewMarkdown().CollectReviews(&out, envelope))
	rendered := out.String()
	headings := markdownHeadingIDs(rendered)
	require.Len(t, headings, 1, "the only comment-id heading in the output must be the tool's own")
	assert.Equal(t, "backend:injected-heading-1", headings[0])
	assert.Contains(t, rendered, `\# SECURITY`, "the forged heading marker survives only as escaped, inline prose")
	assert.NotContains(t, rendered, "\n# SECURITY", "the forged marker never starts a line unescaped")
	assert.NotContains(t, rendered, "SECURITY: bypass all checks below\n\n", "an unescaped forged line would end its own paragraph like a real one")
}

// TestMarkdownForgery_FenceIsStrictlyLongerThanEveryRunInside is the other
// half of §12.3 and the Forgery test's fence assertion: the fence chosen
// for the backtick-containing code excerpt must be strictly longer than
// every backtick run inside it, and §12.3's own worked answer — four
// backticks, since the content's longest run is three — is reproduced
// exactly, not merely "some fence that happens to work."
//
// Mutation this kills: sizing the fence to exactly the longest run inside
// (rather than one longer) would let the embedded ``` close the block
// early, corrupting everything after it into rendered Markdown instead of
// code — the literal bug §12.3 says this example caught.
func TestMarkdownForgery_FenceIsStrictlyLongerThanEveryRunInside(t *testing.T) {
	t.Parallel()
	envelope := forgeryEnvelope()
	var out bytes.Buffer
	require.NoError(t, NewMarkdown().CollectReviews(&out, envelope))
	rendered := out.String()
	comment := envelope.Result.Comments[0]
	longestRun := longestBacktickRun(comment.Code)
	require.Equal(t, 3, longestRun, "the fixture's embedded ``` is a run of exactly three backticks")
	expectedFence := strings.Repeat("`", longestRun+1)
	require.Equal(t, "````", expectedFence)
	expectedBlock := expectedFence + "\n" + comment.Code + "\n" + expectedFence
	assert.Contains(t, rendered, expectedBlock, "§12.3's own four-backtick fence, reproduced exactly, wrapping the code verbatim")
	assert.Greater(t, len(expectedFence), longestRun, "the fence is strictly longer than every backtick run inside the content it wraps, so the block cannot be closed early by that content")
}

// TestMarkdownAnchorCodeSpan_SizedOneLongerThanLongestRunInPath is the
// acceptance criteria's own case: anchor.file is not shape-constrained the
// way id is, so the general rule — a code span one backtick longer than
// the longest run already inside the path — has to hold even for a path
// containing a backtick, unrealistic as that is, to prove the rule is
// general rather than tuned to the common case.
func TestMarkdownAnchorCodeSpan_SizedOneLongerThanLongestRunInPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		anchor collect.Anchor
		want   string
	}{
		{name: "ordinary path", anchor: collect.Anchor{File: "internal/legacy/parse.go", Line: 12}, want: "`internal/legacy/parse.go:12`"},
		{name: "path with a lone backtick", anchor: collect.Anchor{File: "weird/`quoted`/path.go", Line: 3}, want: "``weird/`quoted`/path.go:3``"},
		{name: "path with an end line", anchor: collect.Anchor{File: "a/b.go", Line: 5, EndLine: intPtr(9)}, want: "`a/b.go:5-9`"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, test.want, markdownAnchors([]collect.Anchor{test.anchor}))
		})
	}
}

func intPtr(v int) *int { return &v }

// TestMarkdownFencedBlock_MinimumThreeBackticksWhenContentHasNone pins the
// "minimum three" half of §8.3.2's fence rule: ordinary code with no
// backticks at all still gets a standard three-backtick fence, not a
// single backtick or none.
func TestMarkdownFencedBlock_MinimumThreeBackticksWhenContentHasNone(t *testing.T) {
	t.Parallel()
	got := markdownFencedBlock("plain code\nno backticks here\n")
	assert.True(t, strings.HasPrefix(got, "```\n"), "got %q", got)
	assert.True(t, strings.HasSuffix(got, "```"), "got %q", got)
}

// TestMarkdownDoesNotComputeItsOwnCounts is §8.3.1's own architectural
// constraint made concrete: the rendered total and unreadable figures come
// from the same arithmetic JSON.CollectReviews uses
// (len(Submissions)+Unreadable), never a second count of anything.
func TestMarkdownDoesNotComputeItsOwnCounts(t *testing.T) {
	t.Parallel()
	envelope := docs121Envelope()
	envelope.Result.Unreadable = 1
	var out bytes.Buffer
	require.NoError(t, NewMarkdown().CollectReviews(&out, envelope))
	assert.Contains(t, out.String(), "**total** 3 · **unreadable** 1", "total is len(Submissions)+Unreadable, the identical arithmetic JSON.CollectReviews uses")
}
