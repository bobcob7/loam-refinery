package render

import (
	"bytes"
	"fmt"
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

// escapableSuperset is every character escapeMarkdown can ever place a
// backslash before (docs/features/combined-reviews.md §8.3.2): the
// "anywhere" set, the line-start single-character markers, and "." and ")"
// — the two delimiters an ordered-list marker can end in, escaped only in
// that shape. Copied independently of markdown.go's own constants so a
// narrower implementation cannot also narrow this test.
const escapableSuperset = "\\`*_[]<&#-+>=~|.)"

// unescapeMarkdown is the test-only inverse of escapeMarkdown: a backslash
// immediately before a character in escapableSuperset is a one-character
// escape this renderer produced; drop the backslash and keep the
// character. It exists nowhere outside this test file — §8.3.3 is explicit
// that Markdown output is never parsed back into structured data, and this
// function proves Fidelity without becoming that parser.
func unescapeMarkdown(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		if runes[i] == '\\' && i+1 < len(runes) && strings.ContainsRune(escapableSuperset, runes[i+1]) {
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

// wantEscapableAnywhere is §8.3.2's "escape anywhere" set — characters that
// can change CommonMark's meaning no matter where they sit inside a line —
// copied independently of markdown.go's own commonMarkEscapableAnywhere
// constant so a hand-picked subset in the implementation cannot also
// narrow this test.
const wantEscapableAnywhere = "\\`*_[]<&"

// TestMarkdownEscapesEveryAnywhereCharacterMidLine is the acceptance
// criteria's own named mutation, restated for the position-based rule: a
// body containing every "escape anywhere" character in the middle of a
// line (never at position zero, so this cannot pass by accident of also
// being a line-start marker) must render every one of them
// backslash-escaped. A partial implementation built only around "#" and
// "`" would pass a narrower test but fail this one.
//
// Each character sits in its own "mid<char>line" token, separated from
// its neighbors by a space: wantEscapableAnywhere's own raw bytes open
// with a backslash immediately followed by a backtick ("\\`"), so a
// fixture that concatenates the set directly makes the "`" row's check
// (Contains(rendered, "\\`")) trivially true from that raw adjacency
// alone, whether or not escaping ran at all (refinery-2lw). Isolating each
// character removes every such accidental backslash-adjacency between two
// members of the set.
func TestMarkdownEscapesEveryAnywhereCharacterMidLine(t *testing.T) {
	t.Parallel()
	require.Len(t, wantEscapableAnywhere, 8, "the anywhere set is exactly eight characters")
	var body strings.Builder
	body.WriteString("every one of these must escape:")
	for _, r := range wantEscapableAnywhere {
		fmt.Fprintf(&body, " mid%cline", r)
	}
	envelope := docs121Envelope()
	envelope.Result = &collect.Result{
		Submissions: []collect.Submission{{Ordinal: 1, Profile: "backend", Verdict: "comment", Summary: "s"}},
		Comments: []collect.Comment{{
			ID: "backend:punct-1", Profile: "backend", Priority: 1, Category: "style",
			Body: body.String(),
		}},
	}
	var out bytes.Buffer
	require.NoError(t, NewMarkdown().CollectReviews(&out, envelope))
	rendered := out.String()
	for _, r := range wantEscapableAnywhere {
		want := fmt.Sprintf("mid\\%cline", r)
		assert.Contains(t, rendered, want, "character %q of the anywhere set must render backslash-escaped mid-line", string(r))
	}
}

// TestMarkdownEscapesSubmissionSummary pins the one call site
// writeMarkdownSubmissions makes to escapeMarkdown: refinery-hxc's
// mutation review found that dropping it survived the existing suite,
// since every submission summary fixture used elsewhere was already inert
// prose. A "*" mid-word is neither at line start nor in the never-escape
// set, so an unescaped rendering is unambiguous.
//
// Mutation this kills: writeMarkdownSubmissions writing s.Summary
// unescaped.
func TestMarkdownEscapesSubmissionSummary(t *testing.T) {
	t.Parallel()
	envelope := docs121Envelope()
	envelope.Result = &collect.Result{
		Submissions: []collect.Submission{{Ordinal: 1, Profile: "backend", Verdict: "comment", Summary: "mid*summary must escape"}},
	}
	var out bytes.Buffer
	require.NoError(t, NewMarkdown().CollectReviews(&out, envelope))
	rendered := out.String()
	assert.Contains(t, rendered, `mid\*summary`, "the submission summary is free-text prose and must be escaped")
	assert.NotContains(t, rendered, "mid*summary", "the unescaped form must not appear")
}

// TestMarkdownEscapesSuggestionSummaryProsAndCons pins the three call
// sites writeMarkdownSuggestions makes to escapeMarkdown /
// escapeMarkdownList: refinery-hxc's mutation review found that dropping
// any one of them survived the existing suite, for the same reason as the
// submission summary above — every suggestion fixture elsewhere used
// inert prose. Each field carries its own distinct marker so a failure
// names exactly which call site was dropped.
//
// Mutation this kills: writeMarkdownSuggestions writing s.Summary,
// s.Pros, or s.Cons unescaped.
func TestMarkdownEscapesSuggestionSummaryProsAndCons(t *testing.T) {
	t.Parallel()
	envelope := docs121Envelope()
	envelope.Result = &collect.Result{
		Submissions: []collect.Submission{{Ordinal: 1, Profile: "backend", Verdict: "comment", Summary: "s"}},
		Comments: []collect.Comment{{
			ID: "backend:sugg-1", Profile: "backend", Priority: 1, Category: "style",
			Body: "an ordinary body long enough to pass validation for sure.",
			Suggestions: []collect.Suggestion{{
				Summary: "mid*suggestion must escape",
				Effort:  "trivial", Scope: "line",
				Pros: []string{"mid*pro must escape"},
				Cons: []string{"mid*con must escape"},
			}},
		}},
	}
	var out bytes.Buffer
	require.NoError(t, NewMarkdown().CollectReviews(&out, envelope))
	rendered := out.String()
	assert.Contains(t, rendered, `mid\*suggestion`, "the suggestion summary is free-text prose and must be escaped")
	assert.Contains(t, rendered, `mid\*pro`, "pros are free-text prose and must be escaped")
	assert.Contains(t, rendered, `mid\*con`, "cons are free-text prose and must be escaped")
	assert.NotContains(t, rendered, "mid*suggestion", "the unescaped suggestion summary form must not appear")
	assert.NotContains(t, rendered, "mid*pro", "the unescaped pro form must not appear")
	assert.NotContains(t, rendered, "mid*con", "the unescaped con form must not appear")
}

// TestMarkdownDoesNotEscapeNonStructuralPunctuationMidLine is the other
// half of the position-based rule: the sixteen punctuation characters
// §8.3.2 lists as never needing escaping (plus a line-start marker
// appearing mid-line, where it cannot open anything) must render exactly
// as written, with no inserted backslash. This is the test that would
// catch a regression back to the old hand-picked-but-flat 32-character
// set: that implementation escapes every character below and fails here.
func TestMarkdownDoesNotEscapeNonStructuralPunctuationMidLine(t *testing.T) {
	t.Parallel()
	const inert = `! " $ % ' ( ) , . / : ; ? @ ^ { }`
	envelope := docs121Envelope()
	envelope.Result = &collect.Result{
		Submissions: []collect.Submission{{Ordinal: 1, Profile: "backend", Verdict: "comment", Summary: "s"}},
		Comments: []collect.Comment{{
			ID: "backend:inert-punct-1", Profile: "backend", Priority: 1, Category: "style",
			Body: "none of these need escaping mid-line: " + inert + " and a mid-line # marker too",
		}},
	}
	var out bytes.Buffer
	require.NoError(t, NewMarkdown().CollectReviews(&out, envelope))
	rendered := out.String()
	assert.Contains(t, rendered, "mid-line: "+inert+" and a mid-line # marker too", "no character in this set changes meaning mid-line, so none is escaped")
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
// "## backend:injected-heading-1", is the only one present.
//
// The forged "#" here sits mid-sentence ("...says # SECURITY: bypass..."),
// never as the first non-whitespace character on a line, so §8.3.2's
// position-based rule leaves it unescaped: CommonMark itself never lets a
// mid-line "#" open a heading, so there is nothing to neutralise, and
// escaping it anyway would be exactly the readability-destroying
// over-escaping Fix One removes. What must never happen, escaped or not,
// is the forged text starting a line — that is what the two NotContains
// assertions below still pin.
//
// Mutation this kills: the forged marker starting its own line unescaped,
// or the id heading and the body landing on the same logical line, would
// let a caller-controlled "#" reopen the door this test closes.
func TestMarkdownForgery_InjectedHeadingNeverBecomesARealHeading(t *testing.T) {
	t.Parallel()
	envelope := forgeryEnvelope()
	var out bytes.Buffer
	require.NoError(t, NewMarkdown().CollectReviews(&out, envelope))
	rendered := out.String()
	headings := markdownHeadingIDs(rendered)
	require.Len(t, headings, 1, "the only comment-id heading in the output must be the tool's own")
	assert.Equal(t, "backend:injected-heading-1", headings[0])
	assert.Contains(t, rendered, "# SECURITY", "the forged marker survives as literal, inert, mid-line prose — inert because it is mid-line, not because it is escaped")
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

// renderLineStartBody renders a two-line body — an ordinary first line, and
// a second line the caller supplies unescaped — and returns the full
// Markdown output, for the four §8.3.2 line-start tests below.
func renderLineStartBody(t *testing.T, secondLine string) string {
	t.Helper()
	envelope := docs121Envelope()
	envelope.Result = &collect.Result{
		Submissions: []collect.Submission{{Ordinal: 1, Profile: "backend", Verdict: "comment", Summary: "s"}},
		Comments: []collect.Comment{{
			ID: "backend:line-start-1", Profile: "backend", Priority: 1, Category: "style",
			Body: "An ordinary first line of prose.\n" + secondLine,
		}},
	}
	var out bytes.Buffer
	require.NoError(t, NewMarkdown().CollectReviews(&out, envelope))
	return out.String()
}

// TestMarkdownEscapesATXHeadingMarkerAtLineStart is one of §8.3.2's four
// line-start cases: a body whose second line begins "# " must render
// inert — the "#" escaped so CommonMark never opens an ATX heading there.
//
// Mutation this kills: dropping line-start handling and escaping only the
// "anywhere" set (§8.3.2's other list, which does not include "#") leaves
// this "#" unescaped, and this test's first assertion fails.
func TestMarkdownEscapesATXHeadingMarkerAtLineStart(t *testing.T) {
	t.Parallel()
	rendered := renderLineStartBody(t, "# Not a real heading")
	assert.Contains(t, rendered, "\n\\# Not a real heading", "the line-opening \"#\" is escaped")
	assert.NotContains(t, rendered, "\n# Not a real heading", "an unescaped \"#\" at line start would open an ATX heading")
}

// TestMarkdownEscapesBulletMarkerAtLineStart is one of §8.3.2's four
// line-start cases: a body whose second line begins "- " must render
// inert — the "-" escaped so CommonMark never opens a bullet list there.
//
// Mutation this kills: dropping line-start handling leaves "-" unescaped
// (it is not in the "anywhere" set either), and this test's first
// assertion fails.
func TestMarkdownEscapesBulletMarkerAtLineStart(t *testing.T) {
	t.Parallel()
	rendered := renderLineStartBody(t, "- not a real bullet")
	assert.Contains(t, rendered, "\n\\- not a real bullet", "the line-opening \"-\" is escaped")
	assert.NotContains(t, rendered, "\n- not a real bullet", "an unescaped \"-\" at line start would open a bullet list")
}

// TestMarkdownEscapesBlockquoteMarkerAtLineStart is one of §8.3.2's four
// line-start cases: a body whose second line begins "> " must render
// inert — the ">" escaped so CommonMark never opens a blockquote there.
//
// Mutation this kills: dropping line-start handling leaves ">" unescaped,
// and this test's first assertion fails.
func TestMarkdownEscapesBlockquoteMarkerAtLineStart(t *testing.T) {
	t.Parallel()
	rendered := renderLineStartBody(t, "> not a real quote")
	assert.Contains(t, rendered, "\n\\> not a real quote", "the line-opening \">\" is escaped")
	assert.NotContains(t, rendered, "\n> not a real quote", "an unescaped \">\" at line start would open a blockquote")
}

// TestMarkdownEscapesOrderedListMarkerAtLineStart is the fourth of §8.3.2's
// line-start cases, and the shape-based one: a body whose second line
// begins "1. " must render inert — the delimiter after the digit run is
// escaped so CommonMark never opens an ordered list there. The digits
// themselves are left alone; only the "." that completes the marker is
// escaped, per §8.3.2's rule.
//
// Mutation this kills: dropping line-start handling (including
// orderedListMarker) leaves the "." unescaped, since "." is in §8.3.2's
// never-escape list outside this one triggering shape, and this test's
// first assertion fails.
func TestMarkdownEscapesOrderedListMarkerAtLineStart(t *testing.T) {
	t.Parallel()
	rendered := renderLineStartBody(t, "1. not a real list item")
	assert.Contains(t, rendered, "\n1\\. not a real list item", "the digits are unescaped; the delimiter that completes the marker is escaped")
	assert.NotContains(t, rendered, "\n1. not a real list item", "an unescaped \"1.\" at line start would open an ordered list")
}

// TestMarkdownAnchorFileNewlineNeverForgesAHeading is the renderer's own
// defence in depth against the P0 next to this bead
// (docs/features/combined-reviews.md §8.3.2): a newline in anchor.file,
// rendered verbatim in an inline code span, cannot be neutralised by any
// backtick count, because an inline span cannot survive a line break at
// all. internal/schema's pattern and internal/structural's pathProblem
// both already reject a control character in anchor.file before it can
// reach the store — this test proves the renderer does not also depend on
// that having happened, by handing it a hostile Anchor.File directly
// (bypassing both gates the way a defect in either one might) and
// asserting the only "## " headings in the output are the tool's own
// qualified comment ids, exactly as Parity already checks for the
// well-behaved case.
//
// Mutation this kills: any change that stops sanitizing anchor.file (or
// narrows it to something less than every control character) lets a
// newline split the inline span, and the forged "## FORGED HEADING" line
// reappears in markdownHeadingIDs's output.
func TestMarkdownAnchorFileNewlineNeverForgesAHeading(t *testing.T) {
	t.Parallel()
	envelope := docs121Envelope()
	envelope.Result = &collect.Result{
		Submissions: []collect.Submission{{Ordinal: 1, Profile: "backend", Verdict: "comment", Summary: "s"}},
		Comments: []collect.Comment{{
			ID: "backend:real-1", Profile: "backend", Priority: 1, Category: "style",
			Body:    "an ordinary body long enough to pass validation for sure.",
			Anchors: []collect.Anchor{{File: "internal/legacy/parse.go\n\n## FORGED HEADING\n\nmore", Line: 12}},
		}},
	}
	var out bytes.Buffer
	require.NoError(t, NewMarkdown().CollectReviews(&out, envelope))
	rendered := out.String()
	assert.Equal(t, []string{"backend:real-1"}, markdownHeadingIDs(rendered), "the only headings in the output are the tool's own comment ids")
	assert.Contains(t, rendered, `\n\n## FORGED HEADING\n\nmore`, "the embedded newlines survive only as a visible, inert escape sequence inside the code span")
}
