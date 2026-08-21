package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
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

// backendMax121 and securityMax121 are the two submissions' severity.max
// values docs/features/combined-reviews.md §12.1 shows — 9 for backend's
// single must-fix comment, 3 for security's single optional one —
// package-level vars for the same reason endLine121 is.
var (
	backendMax121  = 9
	securityMax121 = 3
)

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
			{Ordinal: 1, Profile: "backend", Verdict: "request_changes", Summary: "The retry loop is sound, but the context deadline is not propagated to the downstream call.", Severity: collect.Severity{Max: &backendMax121, MustFix: 1}},
			{Ordinal: 2, Profile: "security", Verdict: "comment", Summary: "One low-severity logging concern; nothing blocking.", Severity: collect.Severity{Max: &securityMax121, Optional: 1}},
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

// markdownHeadingIDs extracts every "## <id>" heading from rendered
// Markdown, skipping lines inside a fenced code block. writeMarkdownSubmissions
// never emits an ATX heading (only a bold label), so every "## " line this
// renderer writes outside a fence is a qualified comment id — the invariant
// Parity's own extraction (§8.3.3) relies on being "safe to do exactly
// because ids are structurally constrained." But comment.code and
// suggestion.code are written verbatim and unescaped (§8.3.2), so an
// excerpt whose own content contains a "## " line (§12.3's worked example
// is exactly such an excerpt) must not be misread as a heading. Fence
// tracking mirrors docSectionEnd's, defined later in this same file for
// reading docs/features/combined-reviews.md: a line opens a fence at a
// leading backtick run of 3 or more, and only a line that is entirely
// backticks, at least as many as the opener, closes it — reused rather than
// reinvented, since backtickRun already lives here for that purpose.
func markdownHeadingIDs(rendered string) []string {
	ids := []string{}
	fenceLen := 0
	for _, line := range strings.Split(rendered, "\n") {
		trimmed := strings.TrimSpace(line)
		run := backtickRun(trimmed)
		switch {
		case fenceLen == 0 && run >= 3:
			fenceLen = run
			continue
		case fenceLen > 0 && run >= fenceLen && run == len(trimmed):
			fenceLen = 0
			continue
		case fenceLen > 0:
			continue
		}
		if id, ok := strings.CutPrefix(line, "## "); ok {
			ids = append(ids, id)
		}
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

// TestMarkdownParity_IgnoresHeadingShapedLinesInsideAFencedExcerpt is
// refinery-1pw(a): comment.code and suggestion.code are verbatim and
// deliberately unescaped (§8.3.2), so a fixture whose excerpt contains a
// line starting with "## " — exactly the shape §12.3's own worked example
// takes, a code excerpt full of markdown syntax — must not be read back as
// a second qualified-id heading. Before this test, headingIDPattern matched
// "^## " on any line, so this fixture would fail Parity even though the
// render is correct: a false-failure, not a false-pass, since 8.3.3's
// forgery defence is about what a caller displaying the rendered Markdown
// sees, not what this test-only extractor counts — but exactly the shape
// fixtures are heading toward per §12.3.
//
// Mutation this kills: reverting markdownHeadingIDs to match "^## " on any
// line, fenced or not, would count the excerpt's own "## " line as a second
// heading and fail this test.
func TestMarkdownParity_IgnoresHeadingShapedLinesInsideAFencedExcerpt(t *testing.T) {
	t.Parallel()
	envelope := CollectReviewsEnvelope{
		Ref: "4f2c1a9e8b3d7c5a1f0e2d4b6a8c9e1f3a5b7c9d", RepoName: "github.com/bobcob7/loam-refinery",
		RepoKnown: true, StoreEnabled: true,
		HeadCheck: CollectReviewsHeadCheck{Source: "repo"},
		Result: &collect.Result{
			Submissions: []collect.Submission{{Ordinal: 1, Profile: "backend", Verdict: "comment", Summary: "s"}},
			Comments: []collect.Comment{{
				ID: "backend:heading-shaped-excerpt-1", Profile: "backend", Priority: 1, Category: "style",
				Body: "the excerpt below contains its own markdown heading",
				Code: "some code\n## backend:forged-1\nmore code",
			}},
		},
	}
	var out bytes.Buffer
	require.NoError(t, NewMarkdown().CollectReviews(&out, envelope))
	assert.Equal(t, []string{"backend:heading-shaped-excerpt-1"}, markdownHeadingIDs(out.String()),
		"a '## ' line inside the fenced excerpt is not a second heading")
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

// combinedReviewsDoc and heading123 locate docs/features/combined-
// reviews.md §12.3, the worked example demonstrating the forgery defence
// (refinery-xlp.9): it is the block most deserving of being read from the
// file rather than transcribed, and it has already been wrong twice.
const combinedReviewsDoc = "../../docs/features/combined-reviews.md"

const heading123 = "### 12.3 The markdown projection, and what escaping prevents"

// backtickRun returns the number of leading backtick characters in s.
func backtickRun(s string) int {
	n := 0
	for _, r := range s {
		if r != '`' {
			break
		}
		n++
	}
	return n
}

// docSectionEnd returns the offset, within text, of the next heading line
// (any level) following the heading line found at anchorPos, ignoring any
// line inside a fenced code block — §12.3's own rendered example nests a
// "## backend:..." line inside a fence to demonstrate the forgery defence,
// and CommonMark never reads that as a real heading either. A minimal,
// package-local mirror of internal/cli/docs_shape_test.go's own
// docSectionEnd (refinery-xlp.9): this package does not import
// internal/cli, so the technique is duplicated here rather than shared.
func docSectionEnd(text string, anchorPos int) int {
	lines := strings.SplitAfter(text[anchorPos:], "\n")
	pos := anchorPos
	fenceLen := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		run := backtickRun(trimmed)
		switch {
		case fenceLen == 0 && run >= 3:
			fenceLen = run
		case fenceLen > 0 && run >= fenceLen && run == len(trimmed):
			fenceLen = 0
		case fenceLen == 0 && i > 0 && strings.HasPrefix(trimmed, "#"):
			return pos
		}
		pos += len(line)
	}
	return len(text)
}

// findFenceClose scans lines starting at offset from in text for the first
// line whose trimmed content is entirely backticks, at least closeLen of
// them — CommonMark's actual fence-closing rule, matched by line shape
// rather than by the first bare "```" substring anywhere in the content.
// That distinction is load-bearing here: §12.3's own fixture embeds a
// bare "```" and a shorter nested fence inside the block being read, and
// neither may be mistaken for the real close.
func findFenceClose(text string, from, closeLen int) (blockEnd, nextPos int) {
	lines := strings.SplitAfter(text[from:], "\n")
	pos := from
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && len(trimmed) >= closeLen && strings.Trim(trimmed, "`") == "" {
			return pos, pos + len(line)
		}
		pos += len(line)
	}
	return -1, -1
}

// docFencedBlock returns the occurrence-th fenced block whose opening line
// begins with fence (e.g. "```json" or the five-backtick "markdown" fence
// §12.3 uses), found after anchor and before the next heading, in the
// document at path, as raw trimmed text. A minimal, package-local mirror
// of internal/cli/docs_shape_test.go's own docFencedBlock (refinery-xlp.9).
func docFencedBlock(t *testing.T, path, anchor, fence string, occurrence int) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err, "reading %s", path)
	text := string(data)
	at := strings.Index(text, anchor)
	require.GreaterOrEqualf(t, at, 0, "anchor %q not found in %s", anchor, path)
	rest := text[at:docSectionEnd(text, at)]
	closeLen := backtickRun(fence)
	pos := 0
	for n := 1; ; n++ {
		open := strings.Index(rest[pos:], fence)
		require.GreaterOrEqualf(t, open, 0, "%s block #%d after %q not found before the next heading in %s", fence, occurrence, anchor, path)
		openLineEnd := pos + open + len(fence)
		if nl := strings.IndexByte(rest[openLineEnd:], '\n'); nl >= 0 {
			openLineEnd += nl + 1
		} else {
			openLineEnd = len(rest)
		}
		blockEnd, nextPos := findFenceClose(rest, openLineEnd, closeLen)
		require.GreaterOrEqualf(t, blockEnd, 0, "unterminated %s block after %q in %s", fence, anchor, path)
		block := strings.TrimSpace(rest[openLineEnd:blockEnd])
		pos = nextPos
		if n == occurrence {
			return block
		}
	}
}

// forgeryExcerptJSON is the shape of §12.3's "Submitted document
// (excerpt)" — one comment, no profile field, since profile lives on the
// submission rather than the comment.
type forgeryExcerptJSON struct {
	ID       string `json:"id"`
	Priority int    `json:"priority"`
	Category string `json:"category"`
	Body     string `json:"body"`
	Anchors  []struct {
		File string `json:"file"`
		Line int    `json:"line"`
	} `json:"anchors"`
	Code string `json:"code"`
}

// forgeryHeadingLine matches a rendered qualified-id heading's first line,
// splitting the qualifier (here, the profile "backend") from the origin
// id — §12.3's excerpt never carries profile itself, so this is the only
// place in the file it can be read from.
var forgeryHeadingLine = regexp.MustCompile(`(?m)^## ([^:\n]+):(.+)$`)

// forgeryExpectedMarkdown is §12.3's own rendered answer — the
// five-backtick "markdown"-fenced block shown immediately below the
// submitted excerpt — read as raw text, not transcribed.
func forgeryExpectedMarkdown(t *testing.T) string {
	t.Helper()
	return docFencedBlock(t, combinedReviewsDoc, heading123, "`````markdown", 1)
}

// forgeryComment123 is docs/features/combined-reviews.md §12.3's worked
// fixture, read from the file (refinery-xlp.9) rather than copied
// verbatim into a Go literal: a body containing a literal heading-shaped
// "#" line, and a code excerpt containing an embedded triple-backtick run
// — the concrete instance of the abstract Forgery mechanism §8.3.3 names.
// §12.3 records that building this example caught a real nested-fence
// bug, and it is the block demonstrating the forgery defence that has
// already been wrong twice, which is why this bead requires it be read
// rather than transcribed a third time.
func forgeryComment123(t *testing.T) collect.Comment {
	t.Helper()
	block := docFencedBlock(t, combinedReviewsDoc, heading123, "```json", 1)
	var excerpt forgeryExcerptJSON
	require.NoErrorf(t, json.Unmarshal([]byte(block), &excerpt), "§12.3's submitted-document excerpt did not parse: %s", block)
	require.Len(t, excerpt.Anchors, 1, "§12.3's fixture carries exactly one anchor")
	qualifier, originID := forgeryQualifierAndOriginID(t, forgeryExpectedMarkdown(t))
	require.Equal(t, originID, excerpt.ID, "the rendered heading's origin id must match the submitted comment's own id")
	return collect.Comment{
		ID: qualifier + ":" + excerpt.ID, Profile: qualifier, Priority: excerpt.Priority, Category: excerpt.Category,
		Body: excerpt.Body, Code: excerpt.Code,
		Anchors: []collect.Anchor{{File: excerpt.Anchors[0].File, Line: excerpt.Anchors[0].Line}},
	}
}

// forgeryQualifierAndOriginID splits a rendered qualified-id heading's
// first line into its qualifier and origin id.
func forgeryQualifierAndOriginID(t *testing.T, block string) (qualifier, originID string) {
	t.Helper()
	m := forgeryHeadingLine.FindStringSubmatch(block)
	require.NotNilf(t, m, "expected the rendered example's first line to be a qualified heading: %q", block)
	return m[1], m[2]
}

func forgeryEnvelope(t *testing.T) CollectReviewsEnvelope {
	t.Helper()
	comment := forgeryComment123(t)
	envelope := docs121Envelope()
	envelope.Ref = "4f2c1a9e8b3d7c5a1f0e2d4b6a8c9e1f3a5b7c9d"
	envelope.Result = &collect.Result{
		Submissions: []collect.Submission{{Ordinal: 1, Profile: comment.Profile, Verdict: "comment", Summary: "One comment, engineered to attempt the forgery cli.md §5.1 warns about."}},
		Comments:    []collect.Comment{comment},
	}
	return envelope
}

// markdownCommentSection extracts one comment's full rendered section —
// from its "## id" heading through the next "## " heading, or the end of
// the output when there is none — trimmed. Used to compare the renderer's
// actual output for one comment against a worked example's own shown
// block, which pins one comment's section in isolation, not the whole
// envelope.
func markdownCommentSection(t *testing.T, rendered, id string) string {
	t.Helper()
	heading := "## " + id
	start := strings.Index(rendered, heading)
	require.GreaterOrEqual(t, start, 0, "heading for %s not found in:\n%s", id, rendered)
	rest := rendered[start:]
	if next := strings.Index(rest[len(heading):], "\n## "); next >= 0 {
		rest = rest[:len(heading)+next]
	}
	return strings.TrimSpace(rest)
}

// TestMarkdownForgery_MatchesDocumentedExample is refinery-xlp.9's
// regression test for (b): §12.3 is not just illustrated by a fixture,
// it is CHECKED against the file. The comment section this package
// actually renders, built from §12.3's own submitted-document excerpt,
// must equal the rendered example §12.3 shows immediately below it, byte
// for byte — both read fresh from the file, neither transcribed into a Go
// literal.
//
// Mutation this kills: any change to the escaping or fencing rules that
// alters this fixture's rendered shape — the exact regression §12.3 says
// already happened twice — fails this comparison, where the two tests
// below it, each checking only one specific property, would not.
func TestMarkdownForgery_MatchesDocumentedExample(t *testing.T) {
	t.Parallel()
	envelope := forgeryEnvelope(t)
	var out bytes.Buffer
	require.NoError(t, NewMarkdown().CollectReviews(&out, envelope))
	comment := envelope.Result.Comments[0]
	got := markdownCommentSection(t, out.String(), comment.ID)
	want := strings.TrimSpace(forgeryExpectedMarkdown(t))
	assert.Equal(t, want, got, "docs/features/combined-reviews.md §12.3: the rendered comment section must match the documented example exactly, byte for byte")
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
	envelope := forgeryEnvelope(t)
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
	envelope := forgeryEnvelope(t)
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

// TestMarkdownAnchors_ZeroAnchorsRendersADefiniteAbsence is refinery-1pw(b):
// review-document.md §5 allows an architectural finding to carry no
// anchors, so markdownAnchors(nil) is reachable in real output, and joining
// zero spans with ", " produces "" — leaving "**anchors** " with nothing
// after it, which reads as a missing field rather than a finding that
// legitimately has none. The sibling escapeMarkdownList already solved the
// identical problem for pros/cons with "(none)"; this pins the same answer
// here.
//
// Mutation this kills: deleting the len(anchors) == 0 guard would go back
// to joining zero spans into "".
func TestMarkdownAnchors_ZeroAnchorsRendersADefiniteAbsence(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "(none)", markdownAnchors(nil))
	assert.Equal(t, "(none)", markdownAnchors([]collect.Anchor{}))
}

// TestMarkdownComment_AnchorlessFindingRendersADefiniteAbsence is
// refinery-1pw(b) at the CollectReviews level: an architectural finding
// with no anchors must render "**anchors** (none)", not a bare "**anchors**"
// label with nothing after it.
func TestMarkdownComment_AnchorlessFindingRendersADefiniteAbsence(t *testing.T) {
	t.Parallel()
	envelope := CollectReviewsEnvelope{
		Ref: "4f2c1a9e8b3d7c5a1f0e2d4b6a8c9e1f3a5b7c9d", RepoName: "github.com/bobcob7/loam-refinery",
		RepoKnown: true, StoreEnabled: true,
		HeadCheck: CollectReviewsHeadCheck{Source: "repo"},
		Result: &collect.Result{
			Submissions: []collect.Submission{{Ordinal: 1, Profile: "backend", Verdict: "comment", Summary: "s"}},
			Comments: []collect.Comment{{
				ID: "backend:architectural-1", Profile: "backend", Priority: 5, Category: "architecture",
				Body: "the service and repository layers are not separated anywhere in this package",
			}},
		},
	}
	var out bytes.Buffer
	require.NoError(t, NewMarkdown().CollectReviews(&out, envelope))
	assert.Contains(t, out.String(), "**anchors** (none)\n\n", "an anchorless finding renders a definite absence, not a dangling label")
}

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

// TestMarkdownListStructure_MultiLineSummaryAndSuggestionCodeStayInBullets
// is refinery-3u2's regression test: a submission with a multi-line
// summary and a suggestion carrying a multi-line code excerpt, rendered
// and checked against CommonMark's actual list-continuation rule rather
// than a substring anywhere in the output. In CommonMark, a list item's
// continuation content needs the same indent on *every* line, not just its
// first: a blank line followed by a line that falls short of that indent
// ends the list instead of continuing it. Both fixtures below exercise
// that: a two-line summary, and a suggestion's fenced code block (fence
// included, not just its content).
//
// Mutation this kills: reverting either indentLines call in
// writeMarkdownSubmissions or writeMarkdownSuggestions back to indenting
// only the first line reproduces the column-zero break, and the exact
// indented block this test pins no longer appears in the output.
func TestMarkdownListStructure_MultiLineSummaryAndSuggestionCodeStayInBullets(t *testing.T) {
	t.Parallel()
	envelope := docs121Envelope()
	envelope.Result = &collect.Result{
		Submissions: []collect.Submission{{
			Ordinal: 1, Profile: "backend", Verdict: "comment",
			Summary: "First line of the summary.\nSecond line must stay in the bullet.",
		}},
		Comments: []collect.Comment{{
			ID: "backend:sugg-code-1", Profile: "backend", Priority: 1, Category: "style",
			Body: "an ordinary body long enough to pass validation for sure.",
			Suggestions: []collect.Suggestion{{
				Summary: "apply the fix", Effort: "trivial", Scope: "line",
				Pros: []string{"p"}, Cons: []string{"c"},
				Code: "line one\nline two",
			}},
		}},
	}
	var out bytes.Buffer
	require.NoError(t, NewMarkdown().CollectReviews(&out, envelope))
	rendered := out.String()
	expectedSummaryBlock := "  First line of the summary.\n  Second line must stay in the bullet."
	assert.Contains(t, rendered, expectedSummaryBlock, "every line of a multi-line summary must carry the bullet's two-space continuation indent, or CommonMark drops it out of the list from line two onward")
	assert.NotContains(t, rendered, "\nSecond line must stay in the bullet.", "the second summary line must never start at column zero")
	expectedFencedBlock := "  ```\n  line one\n  line two\n  ```"
	assert.Contains(t, rendered, expectedFencedBlock, "the suggestion's fenced code block, fence included, must carry the bullet's two-space continuation indent on every line, or CommonMark ends the list at the fence")
	assert.NotContains(t, rendered, "\n```\nline one", "the fence must never open at column zero under a list item")
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

// TestFormatSeverity is formatSeverity's own unit test — before this test,
// formatSeverity carried zero assertions anywhere in this tree. Each case
// pins a property its doc comment claims: nil Max (no comments filed)
// renders "none", never a numeric max; a real, filed Max of zero is a
// distinct value from nil and must render "max=0", exactly the confusion
// the *int field exists to prevent; a Max with every band at zero omits
// every band rather than padding the string with "must-fix=0" and the
// rest; and when more than one band is non-zero they render in a fixed
// must-fix, should-fix, worth-fixing, optional order.
//
// Mutation this kills: inverting the `sev.Max == nil` check; widening or
// dropping the `band.count > 0` guard so a zero band prints anyway;
// reordering severityBand's four-entry slice.
func TestFormatSeverity(t *testing.T) {
	t.Parallel()
	zero, five, nine := 0, 5, 9
	for _, tt := range []struct {
		name string
		sev  collect.Severity
		want string
	}{
		{name: "nil max renders none, never a numeric max", sev: collect.Severity{}, want: "none"},
		{name: "a filed max of zero is distinct from nil", sev: collect.Severity{Max: &zero}, want: "max=0"},
		{name: "max with every band at zero omits every band", sev: collect.Severity{Max: &five}, want: "max=5"},
		{name: "one non-zero band", sev: collect.Severity{Max: &nine, MustFix: 1}, want: "max=9, must-fix=1"},
		{
			name: "every band non-zero renders must-fix, should-fix, worth-fixing, optional in that order",
			sev:  collect.Severity{Max: &nine, MustFix: 1, ShouldFix: 2, WorthFixing: 3, Optional: 4},
			want: "max=9, must-fix=1, should-fix=2, worth-fixing=3, optional=4",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, formatSeverity(tt.sev))
		})
	}
}

// TestMarkdownSubmissionRendersPopulatedSeverity is the integration half of
// TestFormatSeverity: docs/features/combined-reviews.md §12.1's own
// worked example, rendered end to end through CollectReviews, must show
// "max=" for both submissions — backend's single must-fix comment and
// security's single optional one — pinning the rendered string rather
// than internal state, per this bead's own design note. Before
// docs121Result carried a real Severity, every submission in this fixture
// rendered "severity: none" and the string "max=" appeared in no test
// output in this repo.
func TestMarkdownSubmissionRendersPopulatedSeverity(t *testing.T) {
	t.Parallel()
	envelope := docs121Envelope()
	var out bytes.Buffer
	require.NoError(t, NewMarkdown().CollectReviews(&out, envelope))
	rendered := out.String()
	assert.Contains(t, rendered, "· severity: max=9, must-fix=1", "backend's single priority-9 comment")
	assert.Contains(t, rendered, "· severity: max=3, optional=1", "security's single priority-3 comment")
	assert.NotContains(t, rendered, "severity: none", "§12.1's worked example has comments; neither submission is severity-free")
}
