// internal/render/html_view_envelope_test.go — bead .3's own test file:
// the envelope strip, the submissions index, and the empty state (§7.1,
// §7.2, §12), the submission half of the degradation table this bead
// owns. Not internal/render/html_test.go, which is bead .7's stub.
package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/bobcob7/loam-refinery/internal/collect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

// htmlSeverity builds a collect.Severity with Max set, since Severity's
// zero value already means "no comments filed" (§12) and every test
// below that wants a real severity needs to say so explicitly.
func htmlSeverity(max int) collect.Severity {
	return collect.Severity{Max: &max}
}

// htmlAssessment returns a pointer to level, the same *string shape
// collect.Submission.Assessment carries, for tests that want a present
// assessment.
func htmlAssessment(level string) *string {
	return &level
}

// renderHTML renders envelope through the HTML renderer and parses the
// result with golang.org/x/net/html, per §2.2.1's own testing strategy:
// structural assertions against the parsed document, not a regular
// expression or a whole-file golden.
func renderHTML(t *testing.T, envelope CollectReviewsEnvelope) (string, *html.Node) {
	t.Helper()
	var out bytes.Buffer
	require.NoError(t, NewHTML().CollectReviews(&out, envelope))
	doc, err := html.Parse(strings.NewReader(out.String()))
	require.NoError(t, err, "rendered output must parse as HTML")
	return out.String(), doc
}

// htmlNodesWithClass walks doc and returns every element node carrying
// class among its space-separated class list.
func htmlNodesWithClass(doc *html.Node, class string) []*html.Node {
	var found []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, attr := range n.Attr {
				if attr.Key == "class" && classListContains(attr.Val, class) {
					found = append(found, n)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return found
}

func classListContains(classList, class string) bool {
	for _, c := range strings.Fields(classList) {
		if c == class {
			return true
		}
	}
	return false
}

// htmlAncestorTags returns every tag name of n's ancestors, root-most
// last, for tests that need to assert an element is never nested inside
// a given tag anywhere on the page.
func htmlAncestorTags(n *html.Node) []string {
	var tags []string
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Type == html.ElementNode {
			tags = append(tags, p.Data)
		}
	}
	return tags
}

// htmlNodeText concatenates every text node under n, depth-first.
func htmlNodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

// fiveSubmissionEnvelope is a fixture with five submissions covering
// this bead's own degradation rows: a normal profiled submission, an
// unprofiled one, one with no assessment, one with no comments at all
// (Severity's zero value), and one superseded by a later submission
// sharing its profile — the case the manual verification ref happened
// not to exercise, exercised here instead so firstFindingID's
// ordinal-qualified branch is covered by something.
func fiveSubmissionEnvelope() CollectReviewsEnvelope {
	return CollectReviewsEnvelope{
		Ref: "10974f7077704bb9f43c4a81741fcea84f510e49", RepoName: "github.com/bobcob7/loam-refinery",
		RepoKnown: true, StoreEnabled: true,
		HeadCheck: CollectReviewsHeadCheck{Source: "repo"},
		Result: &collect.Result{
			Submissions: []collect.Submission{
				{Ordinal: 1, Profile: "backend", Verdict: "request_changes", Assessment: htmlAssessment("mixed"), Severity: htmlSeverity(9)},
				{Ordinal: 2, Profile: "", Verdict: "comment", Assessment: htmlAssessment("sound"), Severity: htmlSeverity(3)},
				{Ordinal: 3, Profile: "security", Verdict: "approve", Assessment: nil, Severity: htmlSeverity(2)},
				{Ordinal: 4, Profile: "docs", Verdict: "approve", Assessment: htmlAssessment("strong"), Severity: collect.Severity{}},
				{Ordinal: 5, Profile: "go", Verdict: "request_changes", Assessment: htmlAssessment("weak"), SupersededBy: intPtr(6), Severity: htmlSeverity(8)},
			},
			Comments: []collect.Comment{
				{ID: "backend:dropped-context-1", Profile: "backend", Priority: 9, Category: "correctness", Body: "context not propagated"},
				{ID: "#2:probe-1", Priority: 3, Category: "style", Body: "an unprofiled finding"},
				{ID: "security:filed-nothing-would-differ-1", Profile: "security", Priority: 2, Category: "security", Body: "unused"},
				{ID: "#5:superseded-finding-1", Profile: "go", Priority: 8, Category: "go", Body: "a finding from the superseded go submission"},
			},
		},
	}
}

// TestHTMLIndex_OneRowPerSubmission_FieldsMatchTheFixture is the
// acceptance criterion in bead .3's own description: for a fixture with
// N submissions the parsed output contains exactly N index rows, each
// carrying its own ordinal, profile, verdict, assessment, and severity.
func TestHTMLIndex_OneRowPerSubmission_FieldsMatchTheFixture(t *testing.T) {
	t.Parallel()
	envelope := fiveSubmissionEnvelope()
	_, doc := renderHTML(t, envelope)
	rows := htmlNodesWithClass(doc, "index-row")
	require.Len(t, rows, len(envelope.Result.Submissions)+1, "one header row plus one row per submission")
	dataRows := rows[1:]
	require.Len(t, dataRows, len(envelope.Result.Submissions))
	want := []string{
		"#1 backend request_changes mixed max=9",
		"#2 (none) comment sound max=3",
		"#3 security approve (none) max=2",
		"#4 docs approve strong none",
		"#5 go request_changes weak max=8",
	}
	for i, row := range dataRows {
		assert.Equal(t, want[i], strings.Join(strings.Fields(htmlNodeText(row)), " "), "row %d text", i+1)
	}
}

// TestHTMLIndex_SupersededSubmissionLinksItsOwnOrdinalQualifiedFinding
// pins firstFindingID's superseded branch directly: submission 5 shares
// its profile ("go") with no current submission in this fixture, but
// carries SupersededBy, so its own finding is ordinal-qualified
// ("#5:superseded-finding-1"), not profile-qualified — the row must
// link to that id, verbatim, through fragmentHref.
func TestHTMLIndex_SupersededSubmissionLinksItsOwnOrdinalQualifiedFinding(t *testing.T) {
	t.Parallel()
	out, _ := renderHTML(t, fiveSubmissionEnvelope())
	assert.Contains(t, out, `href="#%235:superseded-finding-1"`, "the percent-encoded ordinal-qualified href")
}

// TestHTMLIndex_SubmissionWithNoCommentsRendersAPlainOrdinal is §12's
// own "there is nothing to expand for a submission that filed nothing":
// a submission with no comments has no finding to link to, so its
// ordinal cell renders as plain text, not an <a>.
func TestHTMLIndex_SubmissionWithNoCommentsRendersAPlainOrdinal(t *testing.T) {
	t.Parallel()
	envelope := CollectReviewsEnvelope{
		Ref: "r", RepoName: "repo", RepoKnown: true, StoreEnabled: true,
		HeadCheck: CollectReviewsHeadCheck{Source: "repo"},
		Result: &collect.Result{
			Submissions: []collect.Submission{{Ordinal: 1, Profile: "backend", Verdict: "approve"}},
		},
	}
	_, doc := renderHTML(t, envelope)
	rows := htmlNodesWithClass(doc, "index-row")
	require.Len(t, rows, 2)
	ordinalCell := htmlNodesWithClass(doc, "index-ordinal")
	require.Len(t, ordinalCell, 1)
	assert.Equal(t, "#1", strings.TrimSpace(htmlNodeText(ordinalCell[0])))
	var linkTags []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			linkTags = append(linkTags, n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(ordinalCell[0])
	assert.Empty(t, linkTags, "a submission with no comments has nothing to link to")
}

// TestHTMLEnvelope_TotalAndUnreadableCountUnreadableSubmissions is bead
// .3's second acceptance criterion: 2 readable submissions plus
// Unreadable: 1 renders total 3 and unreadable 1 in the strip. An
// unreadable submission is counted, never dropped silently and never a
// broken or partial page (§12).
func TestHTMLEnvelope_TotalAndUnreadableCountUnreadableSubmissions(t *testing.T) {
	t.Parallel()
	envelope := CollectReviewsEnvelope{
		Ref: "r", RepoName: "repo", RepoKnown: true, StoreEnabled: true,
		HeadCheck: CollectReviewsHeadCheck{Source: "repo"},
		Result: &collect.Result{
			Submissions: []collect.Submission{
				{Ordinal: 1, Profile: "backend", Verdict: "approve", Severity: htmlSeverity(1)},
				{Ordinal: 2, Profile: "security", Verdict: "comment", Severity: htmlSeverity(2)},
			},
			Unreadable: 1,
		},
	}
	_, doc := renderHTML(t, envelope)
	envelopeNodes := htmlNodesWithClass(doc, "envelope")
	require.Len(t, envelopeNodes, 1)
	text := htmlNodeText(envelopeNodes[0])
	assert.Contains(t, text, "total: 3")
	assert.Contains(t, text, "unreadable: 1")
	rows := htmlNodesWithClass(doc, "index-row")
	assert.Len(t, rows, 3, "the index itself only lists readable submissions: header plus the two readable rows")
}

// TestHTMLIndex_AbsentAssessmentRendersTheLiteralPlaceholder is bead
// .3's third acceptance criterion, half one: Assessment nil renders the
// literal text "(none)", never one of the four real grade words. This
// is not cosmetic — it is what keeps a reader from mistaking "declined
// to grade" for "graded middling".
func TestHTMLIndex_AbsentAssessmentRendersTheLiteralPlaceholder(t *testing.T) {
	t.Parallel()
	_, doc := renderHTML(t, fiveSubmissionEnvelope())
	cells := htmlNodesWithClass(doc, "index-assessment")
	require.Len(t, cells, 5)
	assert.Equal(t, "(none)", strings.TrimSpace(htmlNodeText(cells[2])), "submission 3's own Assessment is nil")
	for i, level := range []string{"mixed", "sound", "(none)", "strong", "weak"} {
		assert.Equal(t, level, strings.TrimSpace(htmlNodeText(cells[i])))
	}
}

// TestHTMLIndex_AbsentSeverityRendersTheLiteralPlaceholder is bead .3's
// third acceptance criterion, half two: Severity.Max nil renders the
// literal text "none", never a grade word or a number — formatSeverity's
// own treatment of a submission that filed no comments (§12).
func TestHTMLIndex_AbsentSeverityRendersTheLiteralPlaceholder(t *testing.T) {
	t.Parallel()
	_, doc := renderHTML(t, fiveSubmissionEnvelope())
	cells := htmlNodesWithClass(doc, "index-severity")
	require.Len(t, cells, 5)
	assert.Equal(t, "none", strings.TrimSpace(htmlNodeText(cells[3])), "submission 4 filed no comments")
}

// TestHTMLEnvelope_HeadCheckUnavailable_RendersSourceAndOmitsIsHead is
// bead .3's fourth acceptance criterion, half one: with
// HeadCheck.Source "unavailable" the strip contains the word
// "unavailable" and no is_head field — never silently omitted, never
// presented as if verification ran (§12).
func TestHTMLEnvelope_HeadCheckUnavailable_RendersSourceAndOmitsIsHead(t *testing.T) {
	t.Parallel()
	envelope := CollectReviewsEnvelope{
		Ref: "r", RepoName: "repo", RepoKnown: true, StoreEnabled: true,
		HeadCheck: CollectReviewsHeadCheck{Source: "unavailable"},
		Result:    &collect.Result{},
	}
	_, doc := renderHTML(t, envelope)
	envelopeNodes := htmlNodesWithClass(doc, "envelope")
	require.Len(t, envelopeNodes, 1)
	text := htmlNodeText(envelopeNodes[0])
	assert.Contains(t, text, "unavailable")
	assert.NotContains(t, text, "is_head")
}

// TestHTMLEnvelope_HeadCheckNone_RendersSourceAndOmitsIsHead is the
// same §12 row for the other named absent source, "none".
func TestHTMLEnvelope_HeadCheckNone_RendersSourceAndOmitsIsHead(t *testing.T) {
	t.Parallel()
	envelope := CollectReviewsEnvelope{
		Ref: "r", RepoName: "repo", RepoKnown: true, StoreEnabled: true,
		HeadCheck: CollectReviewsHeadCheck{Source: "none"},
		Result:    &collect.Result{},
	}
	_, doc := renderHTML(t, envelope)
	envelopeNodes := htmlNodesWithClass(doc, "envelope")
	require.Len(t, envelopeNodes, 1)
	text := htmlNodeText(envelopeNodes[0])
	assert.Contains(t, text, "none")
	assert.NotContains(t, text, "is_head")
}

// TestHTMLEnvelope_DivergedNil_RendersNoDivergedList is bead .3's fifth
// acceptance criterion, half one: with Diverged nil no diverged list is
// rendered — the check did not apply.
func TestHTMLEnvelope_DivergedNil_RendersNoDivergedList(t *testing.T) {
	t.Parallel()
	isHead := true
	envelope := CollectReviewsEnvelope{
		Ref: "r", RepoName: "repo", RepoKnown: true, StoreEnabled: true,
		HeadCheck: CollectReviewsHeadCheck{Source: "repo", IsHead: &isHead, Diverged: nil},
		Result:    &collect.Result{},
	}
	_, doc := renderHTML(t, envelope)
	envelopeNodes := htmlNodesWithClass(doc, "envelope")
	require.Len(t, envelopeNodes, 1)
	assert.NotContains(t, htmlNodeText(envelopeNodes[0]), "diverged")
}

// TestHTMLEnvelope_DivergedEmpty_RendersExplicitZero is bead .3's fifth
// acceptance criterion, half two: with Diverged empty (checked, found
// nothing) an explicit "0 diverged" is rendered — distinguishing "did
// not check" from "checked, found nothing" (§4.3.1 of
// combined-reviews.md).
func TestHTMLEnvelope_DivergedEmpty_RendersExplicitZero(t *testing.T) {
	t.Parallel()
	isHead := true
	envelope := CollectReviewsEnvelope{
		Ref: "r", RepoName: "repo", RepoKnown: true, StoreEnabled: true,
		HeadCheck: CollectReviewsHeadCheck{Source: "repo", IsHead: &isHead, Diverged: []CollectReviewsDiverged{}},
		Result:    &collect.Result{},
	}
	_, doc := renderHTML(t, envelope)
	envelopeNodes := htmlNodesWithClass(doc, "envelope")
	require.Len(t, envelopeNodes, 1)
	assert.Contains(t, htmlNodeText(envelopeNodes[0]), "0 diverged")
}

// TestHTMLEmptyState_ZeroSubmissionsRendersAWellFormedPage is bead .3's
// sixth acceptance criterion: a zero-submission envelope renders a page
// that parses, contains the envelope strip and the string "no
// submissions found", and contains zero elements matching
// details.finding — an empty store, or a ref with zero submissions, is
// not an error (§12, matching combined-reviews.md §9's exit-0
// treatment of the identical case in JSON and Markdown).
func TestHTMLEmptyState_ZeroSubmissionsRendersAWellFormedPage(t *testing.T) {
	t.Parallel()
	envelope := CollectReviewsEnvelope{
		Ref: "10974f7077704bb9f43c4a81741fcea84f510e49", RepoName: "github.com/bobcob7/loam-refinery",
		RepoKnown: true, StoreEnabled: true,
		HeadCheck: CollectReviewsHeadCheck{Source: "repo"},
		Result:    &collect.Result{},
	}
	out, doc := renderHTML(t, envelope)
	assert.Len(t, htmlNodesWithClass(doc, "envelope"), 1, "the envelope strip still renders")
	assert.Contains(t, out, "no submissions found", "an explicit empty state, not a blank page")
	var findingDetails int
	for _, n := range htmlNodesWithClass(doc, "finding") {
		if n.Data == "details" {
			findingDetails++
		}
	}
	assert.Zero(t, findingDetails, "zero elements matching details.finding")
}

// TestHTMLEnvelopeAndIndex_NeverInsideADetailsElement is bead .3's
// seventh acceptance criterion: neither the strip nor the index is
// inside a details element anywhere in the output. Both are what a
// reader needs before anything else on the page; hiding either behind a
// click would spend the reader's first action on un-hiding information
// the rest of the page assumes they already have (§7.1, §7.2).
func TestHTMLEnvelopeAndIndex_NeverInsideADetailsElement(t *testing.T) {
	t.Parallel()
	_, doc := renderHTML(t, fiveSubmissionEnvelope())
	for _, class := range []string{"envelope", "submissions"} {
		for _, n := range htmlNodesWithClass(doc, class) {
			assert.NotContains(t, htmlAncestorTags(n), "details", "%s element must not sit inside a <details>", class)
		}
	}
}

// TestHTMLView_RenderingTwiceProducesByteIdenticalOutput is §10's
// determinism requirement, scoped to this bead's own view-model code:
// buildHTMLEnvelope and buildHTMLSubmissions are pure functions of
// envelope, ranging only over Result's own ordered slices, so rendering
// the same envelope twice produces byte-identical bytes.
func TestHTMLView_RenderingTwiceProducesByteIdenticalOutput(t *testing.T) {
	t.Parallel()
	envelope := fiveSubmissionEnvelope()
	var first, second bytes.Buffer
	require.NoError(t, NewHTML().CollectReviews(&first, envelope))
	require.NoError(t, NewHTML().CollectReviews(&second, envelope))
	assert.Equal(t, first.String(), second.String())
}
