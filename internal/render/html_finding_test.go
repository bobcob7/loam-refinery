// internal/render/html_finding_test.go — bead .9's own tests for
// html_view_finding.go and the templates it drives (finding.gohtml,
// suggestion.gohtml): the headline truncation rule (§7.3) and its
// degenerate cases, the priority-≥-7 collapse default, the id chip and
// provenance tags for both qualifier forms, the data-* filter attributes
// (§5.1), and §12's two comment-level absence rows. Bead .7's own suite
// (internal/render/html_test.go) owns parity, fidelity, forgery,
// determinism, and the one golden file at the whole-page altitude; this
// file covers only what this bead's own files are responsible for.
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

// ptrTo returns a pointer to v — Go has no address-of-literal operator,
// and this package's other test files each carry their own small
// pointer helper for exactly that reason (see intPtr in a sibling test
// file); this one is named distinctly to avoid colliding with it at
// package scope.
func ptrTo[T any](v T) *T { return &v }

// findingWords repeats "word " until the result carries at least n
// runes, then trims it to exactly n runes — used to build headline
// fixtures of a precise length without hand-counting characters.
func findingWords(n int) string {
	var b strings.Builder
	for b.Len() < n {
		b.WriteString("word ")
	}
	runes := []rune(b.String())
	return string(runes[:n])
}

// --- deriveHeadline: the truncation rule and its degenerate cases ---

// TestDeriveHeadline_ShorterThanCap pins the no-op case: a body with no
// sentence terminator and fewer than headlineCap runes renders whole,
// with no ellipsis (§7.3).
func TestDeriveHeadline_ShorterThanCap(t *testing.T) {
	t.Parallel()
	body := "Fix this quickly"
	assert.Equal(t, body, deriveHeadline(body))
}

// TestDeriveHeadline_LongerThanCap pins the truncating case: a body
// whose first (and only) sentence has no terminator and runs well past
// headlineCap is cut at the last word boundary at or before the cap,
// with an ellipsis appended, and the result is a byte-for-byte prefix of
// body.
func TestDeriveHeadline_LongerThanCap(t *testing.T) {
	t.Parallel()
	body := findingWords(200)
	headline := deriveHeadline(body)
	require.True(t, strings.HasSuffix(headline, headlineEllipsis))
	prefix := strings.TrimSuffix(headline, headlineEllipsis)
	assert.True(t, strings.HasPrefix(body, prefix), "headline must be a prefix of body")
	assert.LessOrEqual(t, len([]rune(prefix)), headlineCap)
	assert.False(t, strings.HasSuffix(prefix, " "), "cut must not leave a trailing space before the ellipsis")
	bodyRunes := []rune(body)
	nextRune := bodyRunes[len([]rune(prefix))]
	assert.True(t, nextRune == ' ', "cut must land right before a word boundary, not mid-word; next rune was %q", nextRune)
}

// TestDeriveHeadline_FirstSentenceEndsExactlyAtBoundary pins the edge
// case explicitly named in this bead's own acceptance criteria: when the
// first sentence's own terminator lands exactly at headlineCap runes,
// the whole sentence renders, unabridged, with no ellipsis — the cap was
// met, not exceeded, so nothing was actually cut.
func TestDeriveHeadline_FirstSentenceEndsExactlyAtBoundary(t *testing.T) {
	t.Parallel()
	sentence := findingWords(headlineCap-1) + "."
	require.Len(t, []rune(sentence), headlineCap)
	body := sentence + " A second sentence follows, well past the cap on its own."
	headline := deriveHeadline(body)
	assert.Equal(t, sentence, headline)
	assert.False(t, strings.HasSuffix(headline, headlineEllipsis))
}

// TestDeriveHeadline_UnbrokenTokenLongerThanCap pins §7.3's one accepted
// exception to cutting at a word boundary: a single token — an
// identifier, a URL — with no whitespace anywhere inside the capped
// window is cut hard at headlineCap runes rather than overflowing the
// summary uncut or vanishing outright.
func TestDeriveHeadline_UnbrokenTokenLongerThanCap(t *testing.T) {
	t.Parallel()
	body := strings.Repeat("x", 200)
	headline := deriveHeadline(body)
	assert.Equal(t, strings.Repeat("x", headlineCap)+headlineEllipsis, headline)
}

// TestDeriveHeadline_NeverRewords is the "cut, never a rewrite" property
// stated three separate ways in §7.3: whatever deriveHeadline returns,
// once its ellipsis (if any) is stripped, is found verbatim at the start
// of body.
func TestDeriveHeadline_NeverRewords(t *testing.T) {
	t.Parallel()
	bodies := []string{
		"Short.",
		findingWords(300),
		strings.Repeat("y", 500),
		"A sentence with no terminator at all just keeps going and going and going and going and going and going and going and going",
	}
	for _, body := range bodies {
		headline := strings.TrimSuffix(deriveHeadline(body), headlineEllipsis)
		assert.True(t, strings.HasPrefix(body, headline), "headline %q must prefix body %q", headline, body)
	}
}

// --- findingFixture: a small collect.Result exercising both qualifier
// forms, both sides of the collapse boundary, and §12's two absence rows ---

// findingEndLine is comment two's anchor.EndLine (collect.Anchor's own
// *int field), a package-level var only because Go has no address-of-
// literal operator for a struct field initializer — the same reason
// endLine121 exists in markdown_test.go, kept separate here so this
// file's fixtures do not reach across into that one's.
var findingEndLine = 94

// findingResult builds a *collect.Result with four comments: one
// profile-qualified at priority 8 (open, must/should-fix band), one
// ordinal-qualified (unprofiled submission) at priority 3 (closed,
// optional band, zero anchors, zero code, two suggestions), one
// profile-qualified at priority 6 (closed, the should-fix/worth-fixing
// boundary), and one ordinal-qualified whose submission still carries a
// profile (the superseded case) at priority 9. This exercises every
// property this bead's acceptance criteria name without reaching into
// bead .3's submissions-index fixtures.
func findingResult() *collect.Result {
	return &collect.Result{
		Comments: []collect.Comment{
			{
				ID: "backend:dropped-context-1", Profile: "backend", Priority: 8, Category: "correctness",
				Body:    "The retry loop calls c.do with context.Background rather than the caller's ctx. This drops every deadline the caller set.",
				Anchors: []collect.Anchor{{File: "internal/fetch/client.go", Line: 88, EndLine: &findingEndLine}},
				Code:    "c.do(context.Background(), req)",
			},
			{
				ID: "#2:missing-test-1", Priority: 3, Category: "testing",
				Body: "No test covers the retry loop's context propagation.",
				Suggestions: []collect.Suggestion{
					{Summary: "Add a table test asserting the passed context", Effort: "small", Scope: "file", Pros: []string{"Catches a regression here"}, Cons: []string{"One more test to keep green"}},
					{Summary: "Add an integration test instead", Effort: "medium", Scope: "module", Pros: []string{"Exercises the real call path"}, Cons: []string{"Slower, flakier"}},
				},
			},
			{
				ID: "security:boundary-1", Profile: "security", Priority: 6, Category: "maintainability",
				Body: "Naming is inconsistent between the two retry helpers.",
			},
			{
				ID: "#4:stale-finding-1", Profile: "go", Priority: 9, Category: "correctness",
				Body: "This submission was superseded, but the finding still stands.",
			},
		},
	}
}

func findingEnvelope() CollectReviewsEnvelope {
	return CollectReviewsEnvelope{
		Ref: "4f2c1a9e8b3d7c5a1f0e2d4b6a8c9e1f3a5b7c9d", RepoName: "github.com/bobcob7/loam-refinery",
		RepoKnown: true, StoreEnabled: true,
		HeadCheck: CollectReviewsHeadCheck{Source: "repo", IsHead: ptrTo(true), Diverged: []CollectReviewsDiverged{}},
		Result:    findingResult(),
	}
}

// renderFinding renders findingEnvelope through the real HTML renderer
// and parses it with golang.org/x/net/html — a structural query, per
// §2.2.1, never a text scan.
func renderFinding(t *testing.T) *html.Node {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, NewHTML().CollectReviews(&buf, findingEnvelope()))
	doc, err := html.Parse(&buf)
	require.NoError(t, err)
	return doc
}

// findingDetailsElements returns every <details class="finding"> element
// in doc, in document order.
func findingDetailsElements(doc *html.Node) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "details" && findingAttr(n, "class") == "finding" {
			out = append(out, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return out
}

// findingAttr returns n's own attribute named key, or "" with ok false
// when n carries none.
func findingAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// findingHasAttr reports whether n carries an attribute named key at
// all, distinguishing a present-but-empty value from an absent one —
// needed for the boolean "open" attribute, which carries no value.
func findingHasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

// findingElementsByTag returns every descendant of n with the given tag
// name, in document order.
func findingElementsByTag(n *html.Node, tag string) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == tag {
			out = append(out, node)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return out
}

// findingText returns the concatenated text content of every descendant
// text node of n.
func findingText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			b.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

// TestFindingParity pins §2.2's parity property at this bead's altitude:
// one <details class="finding"> per comment, and the set of their id
// attributes equals the set of comments[].id, exactly, no decoding step.
func TestFindingParity(t *testing.T) {
	t.Parallel()
	doc := renderFinding(t)
	elements := findingDetailsElements(doc)
	require.Len(t, elements, len(findingResult().Comments))
	gotIDs := make(map[string]bool, len(elements))
	for _, el := range elements {
		gotIDs[findingAttr(el, "id")] = true
	}
	for _, c := range findingResult().Comments {
		assert.True(t, gotIDs[c.ID], "missing details element for id %q", c.ID)
	}
}

// TestFindingCollapseDefault pins §7.3's collapse default: priority >= 7
// open, priority <= 6 closed, computed server-side into the open
// attribute — checked against a fixture carrying priority 6 (closed) and
// both 8 and 9 (open).
func TestFindingCollapseDefault(t *testing.T) {
	t.Parallel()
	doc := renderFinding(t)
	want := map[string]bool{
		"backend:dropped-context-1": true,
		"#2:missing-test-1":         false,
		"security:boundary-1":       false,
		"#4:stale-finding-1":        true,
	}
	for _, el := range findingDetailsElements(doc) {
		id := findingAttr(el, "id")
		assert.Equal(t, want[id], findingHasAttr(el, "open"), "open attribute for %q", id)
	}
}

// TestFindingCollapseDefaultIsDeterministic pins §7.3's own closing
// claim: rendering the same envelope twice opens the identical set of
// details elements both times.
func TestFindingCollapseDefaultIsDeterministic(t *testing.T) {
	t.Parallel()
	var first, second bytes.Buffer
	envelope := findingEnvelope()
	require.NoError(t, NewHTML().CollectReviews(&first, envelope))
	require.NoError(t, NewHTML().CollectReviews(&second, envelope))
	assert.Equal(t, first.String(), second.String())
}

// TestFindingChipAndTags_ProfileQualified pins §7.3's chip and tag shape
// for a profile-qualified finding: the chip's own text is the full
// qualified id, its title carries the full qualified id, and the
// profile renders as its own tag with no ordinal tag present.
func TestFindingChipAndTags_ProfileQualified(t *testing.T) {
	t.Parallel()
	doc := renderFinding(t)
	details := findingByID(t, doc, "backend:dropped-context-1")
	links := findingElementsByTag(details, "a")
	require.NotEmpty(t, links)
	chip := findingChipLink(t, links, "backend:dropped-context-1")
	assert.Equal(t, "backend:dropped-context-1", findingAttr(chip, "title"))
	tags := findingText(details)
	assert.Contains(t, tags, "backend")
	assert.NotContains(t, findingText(details), "#1")
}

// TestFindingChipAndTags_OrdinalQualified pins §7.3's chip and tag shape
// for an ordinal-qualified finding: the chip's own visible text is the
// origin id alone, never the "#N:" prefix, while its title and the
// element's own id attribute still carry the full qualified id, and the
// ordinal renders as its own tag.
func TestFindingChipAndTags_OrdinalQualified(t *testing.T) {
	t.Parallel()
	doc := renderFinding(t)
	details := findingByID(t, doc, "#2:missing-test-1")
	assert.Equal(t, "#2:missing-test-1", findingAttr(details, "id"))
	links := findingElementsByTag(details, "a")
	chip := findingChipLink(t, links, "missing-test-1")
	assert.Equal(t, "#2:missing-test-1", findingAttr(chip, "title"))
	assert.Equal(t, "missing-test-1", findingText(chip), "the chip's own text must be the origin id alone, not the full qualified id")
	assert.Contains(t, findingText(details), "#2")
}

// TestFindingChipAndTags_OrdinalQualifiedWithProfile pins the superseded
// case: an ordinal-qualified finding whose submission still claims a
// profile renders both tags — the profile (who wrote it) and the
// ordinal (which submission) — since the two answer independent
// questions and neither implies the other (combined-reviews.md §6.1).
func TestFindingChipAndTags_OrdinalQualifiedWithProfile(t *testing.T) {
	t.Parallel()
	doc := renderFinding(t)
	details := findingByID(t, doc, "#4:stale-finding-1")
	text := findingText(details)
	assert.Contains(t, text, "go")
	assert.Contains(t, text, "#4")
}

// findingByID returns the <details class="finding"> element in doc whose
// id attribute equals id, failing the test if none is found.
func findingByID(t *testing.T, doc *html.Node, id string) *html.Node {
	t.Helper()
	for _, el := range findingDetailsElements(doc) {
		if findingAttr(el, "id") == id {
			return el
		}
	}
	require.Fail(t, "no finding element found", "id %q", id)
	return nil
}

// findingChipLink returns the <a> element among links whose own text
// content equals text, failing the test if none is found.
func findingChipLink(t *testing.T, links []*html.Node, text string) *html.Node {
	t.Helper()
	for _, link := range links {
		if findingText(link) == text {
			return link
		}
	}
	require.Fail(t, "no chip link found", "text %q", text)
	return nil
}

// TestFindingDataAttributes pins §5.1: every finding carries non-empty
// data-priority-css, data-category-css, and data-lens, each a
// grammar-constrained enum token, and asserts the specific bands this
// fixture exercises.
func TestFindingDataAttributes(t *testing.T) {
	t.Parallel()
	doc := renderFinding(t)
	want := map[string]struct{ priority, category, lens string }{
		"backend:dropped-context-1": {"pri-shouldfix", "cat-correctness", "backend"},
		"#2:missing-test-1":         {"pri-optional", "cat-testing", "#2"},
		"security:boundary-1":       {"pri-worthfixing", "cat-maintainability", "security"},
		"#4:stale-finding-1":        {"pri-mustfix", "cat-correctness", "go"},
	}
	for _, el := range findingDetailsElements(doc) {
		id := findingAttr(el, "id")
		exp, ok := want[id]
		require.True(t, ok, "unexpected id %q", id)
		assert.Equal(t, exp.priority, findingAttr(el, "data-priority-css"))
		assert.Equal(t, exp.category, findingAttr(el, "data-category-css"))
		assert.Equal(t, exp.lens, findingAttr(el, "data-lens"))
	}
}

// TestFindingDataAttributesCarryNoReviewerText is this bead's own slice
// of the forgery property §2.2.1 requires: rendered against a fixture
// whose free-text fields are hostile, the union of every data-*
// attribute value on the page contains none of that hostile text.
func TestFindingDataAttributesCarryNoReviewerText(t *testing.T) {
	t.Parallel()
	hostile := `"><script>data-priority-css=pwned</script>`
	envelope := CollectReviewsEnvelope{
		Ref: "10974f70",
		Result: &collect.Result{
			Comments: []collect.Comment{{
				ID: "backend:dropped-context-1", Profile: "backend", Priority: 8, Category: "correctness",
				Body: hostile, Code: hostile,
				Anchors: []collect.Anchor{{File: hostile}},
				Suggestions: []collect.Suggestion{{
					Summary: hostile, Effort: "trivial", Scope: "line", Pros: []string{hostile}, Cons: []string{hostile},
				}},
			}},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, NewHTML().CollectReviews(&buf, envelope))
	doc, err := html.Parse(&buf)
	require.NoError(t, err)
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, a := range n.Attr {
				if strings.HasPrefix(a.Key, "data-") {
					assert.NotContains(t, a.Val, "script", "data-* attribute %q=%q must never carry reviewer prose", a.Key, a.Val)
					assert.NotContains(t, a.Val, "pwned", "data-* attribute %q=%q must never carry reviewer prose", a.Key, a.Val)
				}
				assert.False(t, strings.HasPrefix(a.Key, "on"), "no attribute name may begin with \"on\": found %q", a.Key)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
}

// --- §12's two comment-level absence rows ---

// TestFindingNoAnchorsRendersNone pins §12: a comment with zero anchors
// renders the literal text "(none)", not an empty element.
func TestFindingNoAnchorsRendersNone(t *testing.T) {
	t.Parallel()
	doc := renderFinding(t)
	details := findingByID(t, doc, "#2:missing-test-1")
	assert.Contains(t, findingText(details), "(none)")
}

// TestFindingWithAnchorsRendersNoPlaceholder is the mirror check: a
// comment that does carry anchors never shows the "(none)" placeholder.
func TestFindingWithAnchorsRendersNoPlaceholder(t *testing.T) {
	t.Parallel()
	doc := renderFinding(t)
	details := findingByID(t, doc, "backend:dropped-context-1")
	assert.NotContains(t, findingText(details), "(none)")
}

// TestFindingNoCodeRendersNoPre pins §12: a comment with an empty Code
// field renders zero <pre> elements inside its own finding — not an
// empty one, which would visually claim there was something to show.
func TestFindingNoCodeRendersNoPre(t *testing.T) {
	t.Parallel()
	doc := renderFinding(t)
	details := findingByID(t, doc, "#2:missing-test-1")
	assert.Empty(t, findingElementsByTag(details, "pre"))
}

// TestFindingSuggestionCards pins §7.4's acceptance criterion: a comment
// with two suggestions renders two suggestion cards, each with its pros
// and cons in sibling containers rather than one nested in the other.
func TestFindingSuggestionCards(t *testing.T) {
	t.Parallel()
	doc := renderFinding(t)
	details := findingByID(t, doc, "#2:missing-test-1")
	cards := findingElementsByClass(details, "suggestion-card")
	require.Len(t, cards, 2)
	for _, card := range cards {
		prosCons := findingElementsByClass(card, "suggestion-pros-cons")
		require.Len(t, prosCons, 1)
		pros := findingElementsByClass(prosCons[0], "suggestion-pros")
		cons := findingElementsByClass(prosCons[0], "suggestion-cons")
		require.Len(t, pros, 1)
		require.Len(t, cons, 1)
		assert.False(t, findingContains(pros[0], cons[0]), "cons must be a sibling of pros, not nested inside it")
		assert.False(t, findingContains(cons[0], pros[0]), "pros must be a sibling of cons, not nested inside it")
	}
}

// findingElementsByClass returns every descendant of n whose class
// attribute contains className as one of its space-separated tokens.
func findingElementsByClass(n *html.Node, className string) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			for _, token := range strings.Fields(findingAttr(node, "class")) {
				if token == className {
					out = append(out, node)
					break
				}
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return out
}

// findingContains reports whether target is a descendant of ancestor.
func findingContains(ancestor, target *html.Node) bool {
	for c := ancestor.FirstChild; c != nil; c = c.NextSibling {
		if c == target || findingContains(c, target) {
			return true
		}
	}
	return false
}

// TestFindingNoOnAttributes pins §5.1's inline-handler prohibition at
// this bead's altitude: nowhere in a rendered page does an attribute
// name begin with "on" — every handler is wired by report.js via
// addEventListener, never an inline onclick and its relatives.
func TestFindingNoOnAttributes(t *testing.T) {
	t.Parallel()
	doc := renderFinding(t)
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, a := range n.Attr {
				assert.False(t, strings.HasPrefix(a.Key, "on"), "no attribute name may begin with \"on\": found %q on <%s>", a.Key, n.Data)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
}
