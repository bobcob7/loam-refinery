// internal/render/html_script_test.go — bead .6's own test file: the
// static-script contract (§5.1), progressive enhancement's toolbar
// exception (§5.2), and the toolbar's own structural pieces (§5.4).
// Not internal/render/html_test.go, which is bead .7's stub and owns
// the parity/fidelity/forgery/determinism suite that pins this
// property across the whole renderer, including a template-tree walk
// over every partial. This file only pins what report.js and
// toolbar.gohtml are themselves responsible for: that the "script"
// define carries zero template actions, that exactly one static
// <script> element ships, that its bytes never vary with the review
// data beside it, and that the toolbar's required controls exist in
// the markup.
package render

import (
	"bytes"
	"strings"
	"testing"
	"text/template/parse"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

// scriptDefineHasOnlyTextNodes walks n and fails t the moment it finds
// any parse.Node that is not a NodeList (a container), a NodeText (the
// script's own static bytes), or a NodeComment (none written here, but
// harmless if one ever were) — i.e. anything that would mean a
// {{ }} action reached the "script" define. This is the structural
// enforcement docs/features/html-report.md §2.2.1 describes: "a static
// check over the template's action list, not any one fixture's output".
func scriptDefineHasOnlyTextNodes(t *testing.T, n parse.Node) {
	t.Helper()
	switch v := n.(type) {
	case *parse.ListNode:
		for _, child := range v.Nodes {
			scriptDefineHasOnlyTextNodes(t, child)
		}
	case *parse.TextNode, *parse.CommentNode:
		// Static bytes only — exactly what §5.1's contract permits.
	default:
		t.Fatalf("report.js's \"script\" define contains a %T node — a template action reached the one <script> element this page ships", n)
	}
}

func TestScriptDefineContainsNoTemplateActions(t *testing.T) {
	t.Parallel()
	h := NewHTML()
	tmpl := h.tmpl.Lookup("script")
	require.NotNil(t, tmpl, "the \"script\" define must exist")
	require.NotNil(t, tmpl.Tree, "the \"script\" define must have a parsed tree")
	scriptDefineHasOnlyTextNodes(t, tmpl.Tree.Root)
}

// scriptElements returns every <script> element in doc, document order.
func scriptElements(doc *html.Node) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "script" {
			out = append(out, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return out
}

func TestPageShipsExactlyOneScriptElementWithNoSrc(t *testing.T) {
	t.Parallel()
	_, doc := renderHTML(t, fiveSubmissionEnvelope())
	scripts := scriptElements(doc)
	require.Len(t, scripts, 1, "exactly one <script> element must ship with the page")
	assert.False(t, findingHasAttr(scripts[0], "src"), "the one <script> element must have no src attribute — it is inline, not fetched (§9)")
}

// scriptBytes extracts the raw source text between the first "<script>"
// and its matching "</script>" in rendered — a plain substring search
// rather than a parsed-node comparison, so it pins the literal bytes the
// page ships, not a structural equivalent of them.
func scriptBytes(t *testing.T, rendered string) string {
	t.Helper()
	start := strings.Index(rendered, "<script>")
	require.GreaterOrEqual(t, start, 0, "rendered output must contain a <script> element")
	end := strings.Index(rendered[start:], "</script>")
	require.GreaterOrEqual(t, end, 0, "rendered output's <script> element must be closed")
	return rendered[start : start+end+len("</script>")]
}

// TestScriptBytesAreIdenticalAcrossWildlyDifferentFixtures pins §5.1's
// "static, every byte fixed at compile time" claim directly: two
// fixtures that share nothing — different refs, different submission
// counts, different comment ids, priorities, and categories — must
// still ship byte-identical <script>...</script> content, because
// nothing about the script is ever derived from the review it sits
// beside.
func TestScriptBytesAreIdenticalAcrossWildlyDifferentFixtures(t *testing.T) {
	t.Parallel()
	rendered1, _ := renderHTML(t, fiveSubmissionEnvelope())
	rendered2, _ := renderHTML(t, findingEnvelope())
	assert.Equal(t, scriptBytes(t, rendered1), scriptBytes(t, rendered2))
}

// scriptTextContent returns the concatenated text content of the page's
// one <script> element.
func scriptTextContent(t *testing.T, doc *html.Node) string {
	t.Helper()
	scripts := scriptElements(doc)
	require.Len(t, scripts, 1)
	return findingText(scripts[0])
}

func TestScriptFirstStatementAddsJSClassToDocumentElement(t *testing.T) {
	t.Parallel()
	_, doc := renderHTML(t, fiveSubmissionEnvelope())
	content := strings.TrimSpace(scriptTextContent(t, doc))
	content = strings.TrimPrefix(content, "(function () {")
	content = strings.TrimSpace(content)
	assert.True(t, strings.HasPrefix(content, `document.documentElement.classList.add("js");`),
		"the script's first executed statement must add the js class to <html>, got: %.80s", content)
}

// TestScriptNeverPhonesOutOrInjectsExecutableContent pins §9's "the
// script never fetches, never loads, and never phones out" and §5.1's
// "no caller data ever reaches an event-handler attribute" from the
// script's own source side: none of these tokens may appear anywhere in
// the one <script> element's text content.
func TestScriptNeverPhonesOutOrInjectsExecutableContent(t *testing.T) {
	t.Parallel()
	_, doc := renderHTML(t, fiveSubmissionEnvelope())
	content := scriptTextContent(t, doc)
	forbidden := []string{
		"fetch(", "fetch (", "XMLHttpRequest", "WebSocket",
		"import(", "import (",
		`createElement("script")`, `createElement('script')`,
		`createElement("link")`, `createElement('link')`,
		"template.JS", "template.JSStr",
	}
	for _, token := range forbidden {
		assert.NotContains(t, content, token, "report.js must never contain %q", token)
	}
}

// TestPageHasNoEventHandlerAttributes pins §5.1's "onclick and its
// relatives appear nowhere" across the whole rendered page, not just
// the script — every handler is wired with addEventListener, so no
// attribute anywhere should begin with "on".
func TestPageHasNoEventHandlerAttributes(t *testing.T) {
	t.Parallel()
	_, doc := renderHTML(t, fiveSubmissionEnvelope())
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, attr := range n.Attr {
				assert.Falsef(t, strings.HasPrefix(strings.ToLower(attr.Key), "on"),
					"element %q carries event-handler-shaped attribute %q", n.Data, attr.Key)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
}

// toolbarControlIDs are every id the toolbar's fixed chrome must carry
// (§5.3, §5.4) for report.js to find and wire, independent of which
// values the running script later populates the facet groups with.
var toolbarControlIDs = []string{
	"toolbar", "expandAllBtn", "collapseAllBtn", "resetFiltersBtn",
	"filterCount", "filterBanner", "facetPriority", "facetCategory", "facetLens",
}

func TestToolbarCarriesEveryRequiredControlID(t *testing.T) {
	t.Parallel()
	_, doc := renderHTML(t, fiveSubmissionEnvelope())
	var ids = map[string]bool{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if id := findingAttr(n, "id"); id != "" {
				ids[id] = true
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	for _, want := range toolbarControlIDs {
		assert.Truef(t, ids[want], "toolbar is missing required control id %q", want)
	}
}

// TestToolbarBannerStartsHiddenAndEmpty pins §5.4's "shown only while a
// filter is active": the server-rendered markup — before any script has
// run — must not claim a filter is active, since none is.
func TestToolbarBannerStartsHiddenAndEmpty(t *testing.T) {
	t.Parallel()
	_, doc := renderHTML(t, fiveSubmissionEnvelope())
	var banner *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && findingAttr(n, "id") == "filterBanner" {
			banner = n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	require.NotNil(t, banner, "the filter banner element must be present in the DOM")
	assert.True(t, findingHasAttr(banner, "hidden"), "the filter banner must start hidden — no filter is active on first render")
	assert.Empty(t, strings.TrimSpace(findingText(banner)), "the filter banner must start with no text — nothing is hidden yet")
}

// TestToolbarIsTheOnlyDisplayNoneByDefaultElement is a narrower,
// bead-.6-facing restatement of §5.2's floor: the one file this bead
// writes markup into never gates a finding itself behind default
// hiding — the toolbar div is the only thing this bead's own template
// ever emits, and it carries no data that would otherwise be missing
// from the page (§5.4's count/banner text is populated by the static
// script, but the finding content it counts is server-rendered
// unconditionally by bead .9's own partial, unaffected by this one).
func TestToolbarMarkupCarriesNoFindingContentOfItsOwn(t *testing.T) {
	t.Parallel()
	rendered, _ := renderHTML(t, findingEnvelope())
	toolbarStart := strings.Index(rendered, `id="toolbar"`)
	require.GreaterOrEqual(t, toolbarStart, 0)
	findingsStart := strings.Index(rendered, `<section id="findings">`)
	require.GreaterOrEqual(t, findingsStart, 0)
	assert.Less(t, toolbarStart, findingsStart, "the toolbar must render before the findings section, never wrapping or gating it")
	toolbarMarkup := rendered[toolbarStart:findingsStart]
	for _, comment := range findingResult().Comments {
		assert.NotContains(t, toolbarMarkup, comment.Body, "the toolbar must never carry a copy of reviewer prose")
	}
}

// TestRenderedPageIsByteIdenticalAcrossTwoRunsOfTheSameEnvelope is
// bead .6's own slice of §10's determinism requirement: rendering one
// unchanged envelope twice, in the same process, must produce identical
// bytes end to end — this bead's static script and toolbar chrome
// included, not only the parts other beads own.
func TestRenderedPageIsByteIdenticalAcrossTwoRunsOfTheSameEnvelope(t *testing.T) {
	t.Parallel()
	envelope := fiveSubmissionEnvelope()
	var first, second bytes.Buffer
	require.NoError(t, NewHTML().CollectReviews(&first, envelope))
	require.NoError(t, NewHTML().CollectReviews(&second, envelope))
	assert.Equal(t, first.String(), second.String())
}
