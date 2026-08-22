// internal/render/html_test.go — the parity, fidelity, and forgery tests
// §2.2 requires, the structural and script-purity assertions §2.2.1
// specifies, the determinism check §10 requires, and the one small,
// hand-reviewed golden file §2.2.1 keeps. Fixtures are the same ones
// docs/features/combined-reviews.md §8.3.3 and §12.3 already use.
//
// Bead .7 owns this file's content, plus one golden under
// internal/render/testdata/.
//
// What earlier beads' own test files already pin, and this file
// deliberately does not re-pin: TestFindingParity (html_finding_test.go)
// already proves parity against bead .9's own fixture; this file's own
// parity test below runs the identical property against
// docs121Envelope, the fixture markdown_test.go's parity test uses, per
// §2.2's "identical fixtures" requirement — a different claim
// (cross-renderer fixture parity), not a duplicate of the same one.
// TestHTMLView_RenderingTwiceProducesByteIdenticalOutput
// (html_view_envelope_test.go), TestFindingCollapseDefaultIsDeterministic
// (html_finding_test.go), and
// TestRenderedPageIsByteIdenticalAcrossTwoRunsOfTheSameEnvelope
// (html_script_test.go) already pin §10's determinism property at three
// different altitudes; nothing here repeats them.
// TestHostileFixtureNeverAltersTheStaticScript and
// TestFragmentHrefCannotBeReachedFromReviewerProse (html_href_test.go)
// already prove a hostile Body/anchor/suggestion field cannot reach the
// static script or forge a working href; this file's own forgery test
// below is the §2.2.1-shaped version — the §12.3-adapted fixture, a
// byte-for-byte compare of the script against its own known source, and
// an element-count comparison against a clean fixture of the same shape
// — none of which the earlier tests assert.
package render

import (
	"bytes"
	"html/template"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"text/template/parse"

	"github.com/bobcob7/loam-refinery/internal/collect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

// --- Parity (§2.2 point 1), run against docs121Envelope — the fixture
// TestMarkdownParity_HeadingsMatchCommentIDs (markdown_test.go) already
// uses, per §2.2's "identical fixtures" requirement. ---

// TestHTMLParity_DetailsFindingIDsMatchCommentIDs_DocsSharedFixture is
// §2.2's Parity test, run against docs121Envelope — the same
// docs/features/combined-reviews.md §12.1 worked example
// TestMarkdownParity_HeadingsMatchCommentIDs already renders, so "one
// structure, three renderers" is a fact this test checks for HTML too,
// not only for Markdown (§2.2's own closing line). Unlike Markdown's
// heading-id extraction, there is no decoding step here to undo: §8.1's
// decision is that the id attribute IS the qualified id, unmodified,
// leading "#" on the ordinal form included.
//
// Mutation this kills: deleting one comment from buildHTMLFindings's
// range loop, or renaming an id on the way through, desyncs the parsed
// id set from Result.Comments and fails this test.
func TestHTMLParity_DetailsFindingIDsMatchCommentIDs_DocsSharedFixture(t *testing.T) {
	t.Parallel()
	envelope := docs121Envelope()
	require.Len(t, envelope.Result.Comments, 2, "the fixture must carry more than one comment")
	_, doc := renderHTML(t, envelope)
	var got []string
	for _, el := range findingDetailsElements(doc) {
		got = append(got, findingAttr(el, "id"))
	}
	var want []string
	for _, c := range envelope.Result.Comments {
		want = append(want, c.ID)
	}
	assert.ElementsMatch(t, want, got, "same members, same count as Result.Comments — no decoding step, the id attribute IS the qualified id")
}

// --- Fidelity (§2.2 point 2) ---

// TestHTMLFidelity_BodyRoundTripsByteForByte is §2.2's Fidelity test,
// run against docs121Envelope — the identical fixture
// TestMarkdownFidelity_UnescapedBodyMatchesJSONBodyByteForByte
// (markdown_test.go) already uses. golang.org/x/net/html's own parser
// already runs every character reference in the rendered output back
// through the identical decoding html.UnescapeString performs, so
// reading a parsed text node's own Data achieves the round trip §2.2
// specifies without a second, hand-rolled unescape step — and does so
// by parsing, never by string-matching (§2.2.1). The parsed
// <p class="body"> text must equal Result.Comments[i].Body byte for
// byte: html/template's own contract, and the test that would catch
// finding.gohtml's `{{.Body}}` silently becoming
// `{{.Body | someHelperThatBypassesEscaping}}`.
//
// Mutation this kills: swapping finding.gohtml's `{{.Body}}` for any
// wrapper that alters, re-encodes, or truncates the value — an
// accidental template.HTML cast included — desyncs the parsed text from
// Result.Comments[i].Body and fails this test.
func TestHTMLFidelity_BodyRoundTripsByteForByte(t *testing.T) {
	t.Parallel()
	envelope := docs121Envelope()
	require.Len(t, envelope.Result.Comments, 2, "the fixture must carry more than one comment")
	_, doc := renderHTML(t, envelope)
	for _, c := range envelope.Result.Comments {
		details := findingByID(t, doc, c.ID)
		bodyEls := findingElementsByClass(details, "body")
		require.Len(t, bodyEls, 1, "comment %s must carry exactly one .body paragraph", c.ID)
		assert.Equal(t, c.Body, findingText(bodyEls[0]), "comment %s: rendered body must round-trip to Result.Comments[i].Body byte for byte", c.ID)
	}
}

// TestHTMLFidelity_CodeRoundTripsByteForByte is Fidelity's code-excerpt
// half, proven at the whole-page altitude §2.2 states it at — parsing
// report.gohtml's own rendered output — rather than only at
// html_highlight_test.go's htmlCodeView.Spans() altitude
// (TestSpansConcatenationReproducesSourceByteForByte), which never
// renders through the template or compares against a comment's own
// collect.Comment.Code field. findingResult's own
// backend:dropped-context-1 (html_finding_test.go) carries a real code
// excerpt for exactly this reason — docs121Envelope's own comments carry
// none, so it cannot exercise this half of Fidelity on its own.
// Concatenating every text node under the rendered <pre> — chroma's own
// per-token <span> wrapping included — reproduces chroma's per-span
// Value fields in order without a second traversal of html_highlight.go's
// own span slice, since that is exactly what a depth-first walk of the
// parsed subtree already does.
//
// Mutation this kills: code.gohtml routing a span's Text through
// anything but ordinary escaped text content — an accidental
// template.HTML wrapper, or a merge step that drops or reorders a byte
// — desyncs the parsed <pre> text from Comment.Code and fails this
// test.
func TestHTMLFidelity_CodeRoundTripsByteForByte(t *testing.T) {
	t.Parallel()
	comment := findingResult().Comments[0]
	require.NotEmpty(t, comment.Code, "fixture comment must carry a code excerpt")
	doc := renderFinding(t)
	details := findingByID(t, doc, comment.ID)
	pres := findingElementsByTag(details, "pre")
	require.Len(t, pres, 1)
	assert.Equal(t, comment.Code, findingText(pres[0]), "the rendered code excerpt, chroma spans stripped, must round-trip to Comment.Code byte for byte")
}

// --- Forgery (§2.2 point 3) ---

// htmlForgeryComment is §2.2's own Forgery fixture: combined-reviews.md
// §12.3's shared metadata — the same id, profile, priority, category,
// and anchor forgeryComment123 (markdown_test.go) reads from the doc
// file — with Body and Code replaced by the three payloads §2.2 names
// for HTML's own grammar. Markdown's own §12.3 payload (a "#"-shaped
// line and a triple-backtick fence) threatens Markdown's escaping, not
// html/template's, so the HTML forgery test is "adapted to its grammar"
// (§2.2) rather than reusing that payload verbatim, while still tying
// the fixture's identity — id, profile, priority, category, anchor — to
// the one the doc and the Markdown suite already share.
func htmlForgeryComment(t *testing.T) collect.Comment {
	t.Helper()
	c := forgeryComment123(t)
	c.Body = `Renaming c.do's context arg looks safe <script>alert(1)</script> but a hostile reviewer could try </details><details open><summary>FORGED to reopen this page's own structure.`
	c.Code = `fmt.Println("</code></pre> is just text here")`
	return c
}

// htmlForgeryCleanComment is the same shape as htmlForgeryComment — the
// identical id, profile, priority, category, and anchor — with
// ordinary, non-adversarial Body and Code text instead, so the two
// fixtures differ only in whether their free-text fields are hostile,
// never in their structure. TestHTMLForgery_ElementCountMatchesACleanFixtureOfTheSameShape
// below renders both and compares total element counts.
func htmlForgeryCleanComment(t *testing.T) collect.Comment {
	t.Helper()
	c := forgeryComment123(t)
	c.Body = "Renaming c.do's context arg looks safe, and nothing about this sentence resembles markup at all."
	c.Code = `fmt.Println("nothing unusual about this line either")`
	return c
}

// htmlForgeryEnvelopeWith builds a one-comment, one-submission envelope
// around comment, sharing the ref and repo docs121Envelope uses.
func htmlForgeryEnvelopeWith(comment collect.Comment) CollectReviewsEnvelope {
	return CollectReviewsEnvelope{
		Ref: "4f2c1a9e8b3d7c5a1f0e2d4b6a8c9e1f3a5b7c9d", RepoName: "github.com/bobcob7/loam-refinery",
		RepoKnown: true, StoreEnabled: true,
		HeadCheck: CollectReviewsHeadCheck{Source: "repo"},
		Result: &collect.Result{
			Submissions: []collect.Submission{{Ordinal: 1, Profile: comment.Profile, Verdict: "comment", Summary: "One comment, engineered to attempt the forgery §2.2 warns about."}},
			Comments:    []collect.Comment{comment},
		},
	}
}

// bead7CountElements returns the total number of html.ElementNode nodes
// anywhere under n — used to compare a hostile fixture's own element
// count against a clean fixture of the same shape (§2.2 point 3): an
// injected closing tag that actually closed something, or an injected
// <script> that actually became one, changes this count.
func bead7CountElements(n *html.Node) int {
	count := 0
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			count++
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return count
}

// TestHTMLForgery_ScriptStaysExactlyOneAndByteIdenticalToItsOwnSource is
// §2.2 point 3's first assertion: the hostile fixture still ships
// exactly one <script> element, and its bytes equal a fixed constant —
// templates/report.js's own known, version-controlled source, read
// independently of html/template's execution path — never the review
// data beside it (§5.1's whole contract).
//
// Mutation this kills: any future change that starts interpolating
// something into the script — the one regression §5.1 exists to
// prevent — desyncs the rendered <script>...</script> bytes from
// report.js's own source and fails this test.
func TestHTMLForgery_ScriptStaysExactlyOneAndByteIdenticalToItsOwnSource(t *testing.T) {
	t.Parallel()
	hostileRendered, hostileDoc := renderHTML(t, htmlForgeryEnvelopeWith(htmlForgeryComment(t)))
	scripts := scriptElements(hostileDoc)
	require.Len(t, scripts, 1, "the hostile fixture must still ship exactly one <script> element")
	source, err := os.ReadFile("templates/report.js")
	require.NoError(t, err)
	// report.js's own leading doc comment mentions both
	// `{{define "script"}}` and "<script>...</script>" in prose
	// (explaining why the define is shaped the way it is), ahead of the
	// real define this test wants — the LAST occurrence of the define
	// keyword is the actual directive, since the comment's own mention
	// of it comes first in the file, and this is what keeps scriptBytes
	// finding the real script content rather than the comment describing
	// it.
	define := strings.LastIndex(string(source), `{{define "script"}}`)
	require.GreaterOrEqual(t, define, 0, "templates/report.js must contain the \"script\" define")
	assert.Equal(t, scriptBytes(t, string(source)[define:]), scriptBytes(t, hostileRendered),
		"the script's bytes must equal report.js's own known source, unchanged by the hostile fixture rendered beside it")
}

// TestHTMLForgery_InjectedSequencesSurviveOnlyAsEscapedTextInTheBody is
// §2.2 point 3's second assertion: the injected `<script>alert(1)</script>`
// and the fake `</details><details open><summary>FORGED` sequence never
// appear as raw, structural bytes in the rendered output — both are
// escaped by html/template's own contextual autoescaper — yet both
// survive, decoded, as ordinary text inside the real comment's own body
// paragraph, and the forged "FORGED" text never becomes a real
// <summary> element's own content.
//
// Mutation this kills: any escaping regression that lets either
// sequence reach the page as raw structural bytes — a second <script>
// element, or a forged <details>/<summary> boundary — fails this test.
func TestHTMLForgery_InjectedSequencesSurviveOnlyAsEscapedTextInTheBody(t *testing.T) {
	t.Parallel()
	comment := htmlForgeryComment(t)
	rendered, doc := renderHTML(t, htmlForgeryEnvelopeWith(comment))
	assert.NotContains(t, rendered, "<script>alert(1)</script>", "the injected script must never reach the page as raw, unescaped bytes")
	assert.NotContains(t, rendered, "</details><details open><summary>FORGED", "the forged details/summary sequence must never reach the page as raw, unescaped bytes")
	details := findingByID(t, doc, comment.ID)
	bodyEls := findingElementsByClass(details, "body")
	require.Len(t, bodyEls, 1)
	bodyText := findingText(bodyEls[0])
	assert.Contains(t, bodyText, "<script>alert(1)</script>", "the injected script text must survive, decoded, as inert text inside the body paragraph")
	assert.Contains(t, bodyText, "</details><details open><summary>FORGED", "the injected details-reopening sequence must survive, decoded, as inert text inside the body paragraph")
	for _, s := range findingElementsByTag(doc, "summary") {
		assert.NotEqual(t, "FORGED", findingText(s), "the forged text must never become a real <summary> element's own content")
	}
}

// TestHTMLForgery_ElementCountMatchesACleanFixtureOfTheSameShape is
// §2.2 point 3's third assertion: the hostile fixture still parses as
// well-formed HTML with the same element count a clean fixture of the
// identical shape produces — an injected closing tag that actually
// closed something, or an injected <script> that actually became one,
// would change that count.
//
// Mutation this kills: an escaper regression that lets the forged
// `</details><details open><summary>FORGED` sequence actually open a
// second <details>/<summary> pair, or the injected `<script>` actually
// become a second <script> element, changes the hostile fixture's own
// element count relative to the clean one and fails this test.
func TestHTMLForgery_ElementCountMatchesACleanFixtureOfTheSameShape(t *testing.T) {
	t.Parallel()
	_, hostileDoc := renderHTML(t, htmlForgeryEnvelopeWith(htmlForgeryComment(t)))
	_, cleanDoc := renderHTML(t, htmlForgeryEnvelopeWith(htmlForgeryCleanComment(t)))
	assert.Equal(t, bead7CountElements(cleanDoc), bead7CountElements(hostileDoc),
		"the hostile and clean fixtures share the identical shape, so their rendered element counts must match")
}

// --- The static escaping and script-purity pin (§2.2.1) ---

// bead7CollectTemplateFuncCalls walks every parse.Node under n and
// records, into calls, the name of every function identifier any
// template action invokes anywhere in the parsed tree — including
// inside an {{if}}/{{range}}/{{with}} branch a particular fixture's data
// might never take, since this walks the static tree, not a rendered
// output. TestEscapingBypassFuncsAreConfinedToFragmentHref below
// cross-references the result against this renderer's own FuncMap.
func bead7CollectTemplateFuncCalls(n parse.Node, calls map[string]bool) {
	switch v := n.(type) {
	case *parse.ListNode:
		if v == nil {
			return
		}
		for _, c := range v.Nodes {
			bead7CollectTemplateFuncCalls(c, calls)
		}
	case *parse.ActionNode:
		bead7CollectTemplateFuncCalls(v.Pipe, calls)
	case *parse.PipeNode:
		if v == nil {
			return
		}
		for _, cmd := range v.Cmds {
			bead7CollectTemplateFuncCalls(cmd, calls)
		}
	case *parse.CommandNode:
		for _, arg := range v.Args {
			bead7CollectTemplateFuncCalls(arg, calls)
		}
	case *parse.IdentifierNode:
		calls[v.Ident] = true
	case *parse.ChainNode:
		bead7CollectTemplateFuncCalls(v.Node, calls)
	case *parse.IfNode:
		bead7CollectTemplateFuncCalls(&v.BranchNode, calls)
	case *parse.RangeNode:
		bead7CollectTemplateFuncCalls(&v.BranchNode, calls)
	case *parse.WithNode:
		bead7CollectTemplateFuncCalls(&v.BranchNode, calls)
	case *parse.BranchNode:
		bead7CollectTemplateFuncCalls(v.Pipe, calls)
		bead7CollectTemplateFuncCalls(v.List, calls)
		if v.ElseList != nil {
			bead7CollectTemplateFuncCalls(v.ElseList, calls)
		}
	case *parse.TemplateNode:
		if v.Pipe != nil {
			bead7CollectTemplateFuncCalls(v.Pipe, calls)
		}
	}
}

// TestEscapingBypassFuncsAreConfinedToFragmentHref is §2.2.1's static
// escaping pin: walk every text/template.Template.Tree this renderer
// parses (h.tmpl.Templates() — every {{define}} across every .gohtml,
// .css, and .js file ParseFS pulled in, not only the "script" define
// TestScriptDefineContainsNoTemplateActions, html_script_test.go,
// already walks alone), collecting every function identifier any action
// invokes anywhere in the tree, then cross-reference each one this
// renderer itself registered (hrefFuncs) against its own reflected
// return type: template.JS, template.JSStr, and template.HTML must
// never be returned by any of them, and template.URL only by
// fragmentHref (§8.3's named exception). This is a check over the
// parsed tree and the FuncMap's own declared types, not any one
// fixture's rendered output, so it cannot be defeated by a fixture that
// happens not to exercise a given branch.
//
// Mutation this kills: adding a second custom template func that
// returns template.HTML (or wiring an existing bypass-typed func into a
// second partial) fails this test even if no fixture in this suite ever
// renders the branch that calls it.
func TestEscapingBypassFuncsAreConfinedToFragmentHref(t *testing.T) {
	t.Parallel()
	h := NewHTML()
	calls := map[string]bool{}
	walked := 0
	for _, tmpl := range h.tmpl.Templates() {
		if tmpl.Tree == nil || tmpl.Tree.Root == nil {
			continue
		}
		walked++
		bead7CollectTemplateFuncCalls(tmpl.Tree.Root, calls)
	}
	require.Positive(t, walked, "the walk must actually visit at least one parsed template tree")
	jsType := reflect.TypeOf(template.JS(""))
	jsStrType := reflect.TypeOf(template.JSStr(""))
	htmlType := reflect.TypeOf(template.HTML(""))
	urlType := reflect.TypeOf(template.URL(""))
	checked := 0
	for name := range calls {
		fn, ok := hrefFuncs[name]
		if !ok {
			continue
		}
		checked++
		rt := reflect.TypeOf(fn)
		require.Equal(t, reflect.Func, rt.Kind(), "hrefFuncs[%q] must be a function", name)
		for i := 0; i < rt.NumOut(); i++ {
			out := rt.Out(i)
			assert.NotEqual(t, jsType, out, "template func %q must never return template.JS", name)
			assert.NotEqual(t, jsStrType, out, "template func %q must never return template.JSStr", name)
			assert.NotEqual(t, htmlType, out, "template func %q must never return template.HTML", name)
			if out == urlType {
				assert.Equal(t, "fragmentHref", name, "template.URL must be produced only by fragmentHref (§8.3)")
			}
		}
	}
	require.Positive(t, checked, "the walk must find at least one call into this renderer's own FuncMap, or the cross-reference above is vacuous")
}

// bead7WalkFieldTypes recursively visits every struct field type
// reachable from t — through nested structs, slices, and pointers —
// calling visit once per field's own reflect.Type. Cycles are not a
// concern: every view-model type this renderer builds (htmlView and
// everything reachable from it) is a plain, acyclic value shape.
func bead7WalkFieldTypes(t reflect.Type, visit func(reflect.Type), seen map[reflect.Type]bool) {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || seen[t] {
		return
	}
	seen[t] = true
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i).Type
		visit(field)
		bead7WalkFieldTypes(field, visit, seen)
	}
}

// TestViewModelFieldsNeverCarryAnEscaperBypassType closes the one gap
// TestEscapingBypassFuncsAreConfinedToFragmentHref above cannot: a
// template action reading `{{.Body}}` off a struct field declared as
// template.HTML (rather than string) never calls a registered
// function at all, so the FuncMap cross-reference above would never
// see it — html/template resolves that trust at execute time, from the
// field's own static Go type, not from anything the parsed action tree
// records. This test closes that gap the other way: reflecting over
// htmlView (report.gohtml's own root value) and every struct type
// reachable from it, asserting no field anywhere in that graph is
// declared template.HTML, template.JS, or template.JSStr, and none is
// template.URL either — this renderer's one legitimate template.URL
// value is computed by fragmentHref inside the template itself
// (§8.3), never stored on a view-model field. Like the FuncMap check
// above, this is fixture-independent: it inspects declared field
// types, not any fixture's rendered bytes, so it cannot be defeated by
// a fixture that happens not to exercise the field in question.
//
// Mutation this kills: changing htmlFindingView.Body's declared type
// from string to template.HTML — which would let a hostile Body's
// `<script>` tags become real structural markup for any fixture,
// adversarial or not — fails this test even against a wholly
// non-adversarial fixture, unlike every forgery test in this suite,
// which only notices because their own fixtures happen to be hostile.
func TestViewModelFieldsNeverCarryAnEscaperBypassType(t *testing.T) {
	t.Parallel()
	jsType := reflect.TypeOf(template.JS(""))
	jsStrType := reflect.TypeOf(template.JSStr(""))
	htmlType := reflect.TypeOf(template.HTML(""))
	urlType := reflect.TypeOf(template.URL(""))
	visited := 0
	bead7WalkFieldTypes(reflect.TypeOf(htmlView{}), func(field reflect.Type) {
		visited++
		assert.NotEqual(t, jsType, field, "no view-model field may be declared template.JS")
		assert.NotEqual(t, jsStrType, field, "no view-model field may be declared template.JSStr")
		assert.NotEqual(t, htmlType, field, "no view-model field may be declared template.HTML")
		assert.NotEqual(t, urlType, field, "no view-model field may be declared template.URL — fragmentHref computes it inside the template instead (§8.3)")
	}, map[reflect.Type]bool{})
	require.Positive(t, visited, "the walk must actually visit at least one view-model field")
}

// --- Structural coverage (§2.2.1): id uniqueness across the OUTPUT ---

// TestEveryIDAttributeIsUniqueAcrossTheWholePage is §2.2.1's structural
// coverage requirement: review-document.md §11.1's id-unique check only
// covers the INPUT document's comment ids; this pins the OUTPUT's own
// id attributes — every id on the page, findings and toolbar chrome
// alike — as a set with no duplicate, the property a copied #fragment
// link and every details/summary open-state toggle both depend on.
//
// Mutation this kills: two findings ending up with the same id
// attribute — a defect TestHTMLParity's own set-membership comparison
// would not catch, since ElementsMatch is multiplicity-aware and would
// pass on two matching duplicates — collides in the DOM and fails this
// test.
func TestEveryIDAttributeIsUniqueAcrossTheWholePage(t *testing.T) {
	t.Parallel()
	_, doc := renderHTML(t, fiveSubmissionEnvelope())
	seen := map[string]int{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if id := findingAttr(n, "id"); id != "" {
				seen[id]++
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	require.NotEmpty(t, seen, "the page must carry at least one id attribute")
	for id, count := range seen {
		assert.Equal(t, 1, count, "id %q must appear exactly once across the page", id)
	}
}

// --- Progressive enhancement (§5.2, §2.2.1) ---

// bead7StyleText concatenates every <style> element's text content from
// the rendered page — the one place both bead7DisplayNoneSelectors and
// the named-rule tests below read the emitted stylesheet from, so there
// is exactly one scan over the page's <style> elements, not two.
func bead7StyleText(t *testing.T, doc *html.Node) string {
	t.Helper()
	var css strings.Builder
	for _, style := range findingElementsByTag(doc, "style") {
		css.WriteString(findingText(style))
		css.WriteString("\n")
	}
	require.NotZero(t, css.Len(), "the page must carry at least one <style> element to scan")
	return css.String()
}

// bead7DisplayNoneSelectors returns every CSS selector, from the
// rendered page's own <style> elements, whose declaration block sets
// "display: none" — a scan over the emitted bytes (§9's self-contained
// page carries its whole stylesheet inline, so this needs no external
// file), not a full CSS parser: this renderer's stylesheet has no
// nested rules besides @media blocks, so a flat "selector { body }"
// regex is sufficient for exactly the two files bead .5 and bead .6 own.
func bead7DisplayNoneSelectors(t *testing.T, doc *html.Node) []string {
	t.Helper()
	css := bead7StyleText(t, doc)
	ruleRE := regexp.MustCompile(`([^{}]+)\{([^{}]*)\}`)
	displayNoneRE := regexp.MustCompile(`display\s*:\s*none`)
	var selectors []string
	for _, m := range ruleRE.FindAllStringSubmatch(css, -1) {
		if !displayNoneRE.MatchString(m[2]) {
			continue
		}
		for _, sel := range strings.Split(m[1], ",") {
			sel = strings.TrimSpace(sel)
			if sel != "" {
				selectors = append(selectors, sel)
			}
		}
	}
	return selectors
}

// TestOnlyToolbarIsHiddenByDefault is §5.2's progressive-enhancement
// floor, checked from the emitted bytes alone — no browser required
// (§2.2.1): every "display: none" selector in the page's own <style>
// elements is either §5.2's one permitted exception (".toolbar", paired
// with bead .6's "html.js .toolbar { display: flex }" override that
// undoes it once script has run), the "[hidden]" failsafe rule (report.css's
// own comment: it only ever fires on an element that already carries
// the hidden attribute — filterBanner alone, per
// TestToolbarBannerStartsHiddenAndEmpty, html_script_test.go — never a
// second, silent default), or the decorative <details> marker glyph
// ("::-webkit-details-marker" hides only the native disclosure
// triangle, never a finding's own content). Anything else would be a
// second piece of content hidden by default with no script required to
// reveal it — exactly what §5.2 forbids. The assertion names ".toolbar"
// itself, per this bead's own acceptance criteria, rather than merely
// asserting a count.
//
// Mutation this kills: a future style rule such as
// ".suggestion-card { display: none }" — an "easier" way to collapse a
// section than the open attribute — passes every other test in this
// suite (nothing else parses the stylesheet) and would only be caught
// here.
func TestOnlyToolbarIsHiddenByDefault(t *testing.T) {
	t.Parallel()
	_, doc := renderHTML(t, fiveSubmissionEnvelope())
	var offenders []string
	for _, sel := range bead7DisplayNoneSelectors(t, doc) {
		switch {
		case sel == ".toolbar":
		case sel == "[hidden]":
		case strings.HasPrefix(sel, "html.js "):
		case strings.HasSuffix(sel, "::-webkit-details-marker"):
		default:
			offenders = append(offenders, sel)
		}
	}
	assert.Empty(t, offenders, `".toolbar" must be the only selector hidden by default`)
}

// bead7HiddenAttributeAlwaysWinsRE pins §5.4's mandatory defence by its
// own exact shape, not merely its presence in bead7DisplayNoneSelectors'
// permitted list: the "[hidden]" selector's declaration block must set
// "display: none" AND carry "!important" — report.css's own comment
// explains why the "!important" is load-bearing, not decorative: it is
// what lets this one rule outrank any later, more specific selector that
// also sets display on the same element, regardless of source order.
var bead7HiddenAttributeAlwaysWinsRE = regexp.MustCompile(`\[hidden\]\s*\{[^{}]*display\s*:\s*none\s*!important[^{}]*\}`)

// TestHiddenAttributeAlwaysWinsTheCascade requires — not merely permits
// — §5.4's own defence to exist: refinery-t1c.5 reproduced that deleting
// "[hidden] { display: none !important }" from report.css failed only
// TestHTMLGolden, and one -update run made the suite green with the
// defence gone. TestOnlyToolbarIsHiddenByDefault above whitelists this
// selector in its offender scan but never requires it to be present at
// all — an empty stylesheet passes that test just as cleanly as one
// carrying the rule. This test is what actually requires it.
//
// Mutation this kills: deleting the "[hidden]" rule, or weakening it to
// "display: none" without "!important" (letting a later, more specific
// selector on the same element silently win), fails this test by name —
// not only the golden.
func TestHiddenAttributeAlwaysWinsTheCascade(t *testing.T) {
	t.Parallel()
	_, doc := renderHTML(t, fiveSubmissionEnvelope())
	css := bead7StyleText(t, doc)
	assert.True(t, bead7HiddenAttributeAlwaysWinsRE.MatchString(css),
		`§5.4's mandatory defence is missing or weakened: the stylesheet must contain "[hidden] { display: none !important }"`)
}

// bead7ToolbarRevealRE pins the toolbar's own reveal by its exact shape:
// "html.js .toolbar" must set "display: flex", the only rule that undoes
// ".toolbar { display: none }" once report.js's first statement has
// added the "js" class to <html> (TestScriptFirstStatementAddsJSClassToDocumentElement,
// html_script_test.go). Without it the toolbar stays display:none
// forever, script or no script.
var bead7ToolbarRevealRE = regexp.MustCompile(`html\.js\s+\.toolbar\s*\{[^{}]*display\s*:\s*flex[^{}]*\}`)

// TestToolbarRevealRuleExists requires — not merely permits — the
// toolbar's only reveal to exist. refinery-t1c.5 reproduced the same
// golden-only failure mode for this rule that
// TestHiddenAttributeAlwaysWinsTheCascade guards against above: deleting
// "html.js .toolbar { display: flex }" fails only TestHTMLGolden today,
// and regenerating the golden erases the loss silently.
//
// Mutation this kills: deleting the "html.js .toolbar" reveal, or
// changing its declared display value away from "flex", fails this test
// by name — the toolbar would stay hidden even after script has run.
func TestToolbarRevealRuleExists(t *testing.T) {
	t.Parallel()
	_, doc := renderHTML(t, fiveSubmissionEnvelope())
	css := bead7StyleText(t, doc)
	assert.True(t, bead7ToolbarRevealRE.MatchString(css),
		`the toolbar's only reveal is missing: the stylesheet must contain "html.js .toolbar { display: flex }"`)
}

// --- Script vitality (§5, §2.2.1) ---
//
// TestHTMLForgery_ScriptStaysExactlyOneAndByteIdenticalToItsOwnSource
// above, and TestScriptBytesAreIdenticalAcrossWildlyDifferentFixtures
// (html_script_test.go), each compare the rendered <script> element
// against report.js's own source — real tests for real properties, but
// neither can ever see a syntax error: both sides of every comparison
// they make come from the same broken file, so a mismatched brace that
// kills the whole IIFE at parse time passes both of them and fails
// nothing but TestHTMLGolden (refinery-t1c.5). This package has no JS
// runtime to actually execute report.js and observe the breakage
// (html_script_test.go's own TestScriptFacetStateIsPrototypePollutionSafe
// doc comment notes the same limit), so the guard below checks the one
// thing byte-scanning a script with no string-embedded delimiters can
// check honestly: that every (), {}, and [] closes what it opened, in
// the order it opened it. report.js's own doc comment records that it
// contains no single-quoted strings and no "//" comments (grep confirms
// both), so a plain double-quote-aware scan — skip everything between an
// unescaped '"' and the next unescaped '"' — never misreads a delimiter
// character that was actually part of string content.

// bead7CheckBalancedDelimiters walks script one byte at a time, skipping
// double-quoted string content (backslash-escape aware, per report.js's
// own strings), and fails t the moment a closing delimiter has no
// matching opener, closes the wrong opener, or any opener is left
// unclosed at the end — the shape a dropped or duplicated brace takes
// when it kills the whole enclosing IIFE.
func bead7CheckBalancedDelimiters(t *testing.T, script string) {
	t.Helper()
	closers := map[byte]byte{')': '(', '}': '{', ']': '['}
	var stack []byte
	inString := false
	escaped := false
	for i := 0; i < len(script); i++ {
		c := script[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '(', '{', '[':
			stack = append(stack, c)
		case ')', '}', ']':
			if !assert.NotEmptyf(t, stack, "unexpected closing %q at byte %d with nothing open", c, i) {
				return
			}
			top := stack[len(stack)-1]
			if !assert.Equalf(t, closers[c], top, "mismatched delimiter: %q closes %q, not %q, at byte %d", c, top, closers[c], i) {
				return
			}
			stack = stack[:len(stack)-1]
		}
	}
	assert.Emptyf(t, stack, "script ends with unclosed delimiter(s) still open: %q", stack)
}

// bead7ScriptSpine is every top-level call report.js's own tail makes to
// actually wire the page up (its last five statements, in source order):
// building the filter chips, wiring the toolbar's buttons, applying the
// filter state once, and routing the page's own fragment on load and on
// every later hash change. A syntax error early in the file (an extra
// "}" closing a function one statement too soon, for instance) can
// leave these calls sitting outside the IIFE they were meant to run
// inside, present in the bytes but never invoked — invisible to a
// substring check, but not to the delimiter-balance one above, which is
// why this list is a second, independent signal rather than a
// replacement for it.
var bead7ScriptSpine = []string{
	"buildFacets();",
	"wireToolbar();",
	"applyFilters();",
	`document.addEventListener("DOMContentLoaded", routeHash);`,
	`window.addEventListener("hashchange", routeHash);`,
}

// TestScriptSyntaxIsWellFormed is refinery-t1c.5's behavioural guard on
// report.js: reproduced by the tests lens as seven logic mutations plus
// a deliberate syntax error that kills the entire script IIFE, all
// golden-only. This test gives the syntax error specifically a named
// failure: delimiter balance is checkable from the bytes with no JS
// runtime, and it is exactly what a mismatched or dropped brace/paren/
// bracket violates. bead7ScriptSpine is the complementary check — the
// calls that actually make the IIFE do anything once it parses.
//
// Mutation this kills: an extra or missing "}", "(", or "[" anywhere in
// report.js, or deleting the wiring calls at its tail, fails this test —
// not only TestHTMLGolden.
func TestScriptSyntaxIsWellFormed(t *testing.T) {
	t.Parallel()
	_, doc := renderHTML(t, fiveSubmissionEnvelope())
	content := scriptTextContent(t, doc)
	bead7CheckBalancedDelimiters(t, content)
	for _, call := range bead7ScriptSpine {
		assert.Containsf(t, content, call, "report.js must still invoke %q to wire the page up", call)
	}
}

// --- Self-contained (§9) ---

// bead7OffPageURLPattern matches a url(...) or href value naming any
// scheme other than the page's own empty fragment — http(s), a
// protocol-relative "//", or any other "scheme:" prefix.
var bead7OffPageURLPattern = regexp.MustCompile(`(?i)url\(\s*['"]?(https?:|//)|href\s*=\s*['"](https?:|//)`)

// TestSelfContainedNoNetworkReferencesAnywhereInTheOutput pins §9: the
// page never references anything off itself — no <link> element (a
// stylesheet or icon fetched over the network), no src attribute on any
// script (TestPageShipsExactlyOneScriptElementWithNoSrc,
// html_script_test.go, already pins this for the one <script> the page
// ships; this is the whole-page version, covering every element, not
// only script), no @import inside the emitted CSS, and no url(...) or
// href naming any scheme but the page's own empty fragment.
//
// Mutation this kills: a future style rule pulling in a hosted web font
// via @import or url(https://…), or a template emitting a
// <link rel="stylesheet"> instead of an inline <style>, passes every
// other test in this suite and would only be caught here.
func TestSelfContainedNoNetworkReferencesAnywhereInTheOutput(t *testing.T) {
	t.Parallel()
	out, doc := renderHTML(t, fiveSubmissionEnvelope())
	assert.Empty(t, findingElementsByTag(doc, "link"), "no <link> element may appear — nothing is fetched over the network (§9)")
	for _, el := range findingElementsByTag(doc, "script") {
		assert.False(t, findingHasAttr(el, "src"), "no <script> element may carry a src attribute")
	}
	for _, style := range findingElementsByTag(doc, "style") {
		assert.NotContains(t, findingText(style), "@import", "no stylesheet may @import anything")
	}
	assert.False(t, bead7OffPageURLPattern.MatchString(out), "no url() or href may name a scheme other than the page's own empty fragment")
	for _, el := range findingElementsByTag(doc, "a") {
		href := findingAttr(el, "href")
		if href == "" {
			continue
		}
		assert.True(t, strings.HasPrefix(href, "#"), "every <a href> on the page must name only a same-page fragment, got %q", href)
	}
}

// --- The one retained golden (§2.2.1) ---

// bead7ElidedCSSPlaceholder and bead7ElidedScriptPlaceholder replace, in
// the golden alone, the inner bytes of every <style> and the one
// <script> element the rendered page ships. refinery-t1c.5: 583 of this
// golden's 697 lines were a verbatim copy of report.css, code.css, and
// report.js, which made §2.2.1's own "small enough that a reviewer
// reads the whole diff" false, and made regenerating the golden — the
// routine response to any CSS edit — silently swallow a real regression
// (deleting "[hidden] { display: none !important }", or
// "html.js .toolbar { display: flex }", each failed only this test, and
// one -update run erased the loss). Both rules are now required BY NAME
// in TestHiddenAttributeAlwaysWinsTheCascade and
// TestToolbarRevealRuleExists above, the script's syntactic health is
// required by name in TestScriptSyntaxIsWellFormed, and script/CSS
// byte-identity against their own known source files is already pinned
// elsewhere (TestHTMLForgery_ScriptStaysExactlyOneAndByteIdenticalToItsOwnSource
// above; TestScriptBytesAreIdenticalAcrossWildlyDifferentFixtures,
// html_script_test.go). With every correctness-carrying property of the
// CSS and script asserted by name, this golden's only remaining job is
// the page's STRUCTURE — element placement, escaping, ids, the shape
// bead7GoldenEnvelope's own doc comment already describes — which is
// exactly what stays legible once the copied asset bytes are elided.
// The <style> and <script> tags themselves are left in place, so a
// change that adds, removes, or reorders one of the three style blocks
// or the one script element still shows up in the diff; only their
// interiors do not.
const (
	bead7ElidedCSSPlaceholder    = "/* elided by TestHTMLGolden (refinery-t1c.5) — see report.css / code.css; the correctness-carrying rules are pinned by name in TestHiddenAttributeAlwaysWinsTheCascade and TestToolbarRevealRuleExists */"
	bead7ElidedScriptPlaceholder = "// elided by TestHTMLGolden (refinery-t1c.5) — see report.js; syntax and wiring are pinned by name in TestScriptSyntaxIsWellFormed, byte-identity to source in TestHTMLForgery_ScriptStaysExactlyOneAndByteIdenticalToItsOwnSource"
)

var (
	bead7StyleBlockRE  = regexp.MustCompile(`(?s)<style>.*?</style>`)
	bead7ScriptBlockRE = regexp.MustCompile(`(?s)<script>.*?</script>`)
)

// bead7ElideStaticAssets returns rendered with every <style>...</style>
// and <script>...</script> element's own inner bytes replaced by a
// short, fixed placeholder. See the doc comment on
// bead7ElidedCSSPlaceholder for why TestHTMLGolden calls this before
// comparing against (or writing) the golden file, rather than comparing
// the full page.
func bead7ElideStaticAssets(rendered string) string {
	rendered = bead7StyleBlockRE.ReplaceAllString(rendered, "<style>"+bead7ElidedCSSPlaceholder+"</style>")
	rendered = bead7ScriptBlockRE.ReplaceAllString(rendered, "<script>"+bead7ElidedScriptPlaceholder+"</script>")
	return rendered
}

// bead7GoldenEnvelope is the one hand-reviewed golden fixture §2.2.1
// keeps — small enough that a reviewer can read the whole diff, but
// touching every layout region once: the envelope strip, the
// submissions index, one open (priority >= 7) finding carrying an
// anchor and a code excerpt, and one suggestion card with pros and
// cons. Every property this format has is pinned by an assertion
// elsewhere in this suite, never by this fixture's own bytes — see
// docs/features/html-report.md §2.2.1's own "why not a bigger golden"
// reasoning.
func bead7GoldenEnvelope() CollectReviewsEnvelope {
	return CollectReviewsEnvelope{
		Ref: "4f2c1a9e8b3d7c5a1f0e2d4b6a8c9e1f3a5b7c9d", RepoName: "github.com/bobcob7/loam-refinery",
		RepoKnown: true, StoreEnabled: true,
		HeadCheck: CollectReviewsHeadCheck{Source: "repo", IsHead: ptrTo(true), Diverged: []CollectReviewsDiverged{}},
		Result: &collect.Result{
			Submissions: []collect.Submission{
				{Ordinal: 1, Profile: "backend", Verdict: "request_changes", Assessment: ptrTo("mixed"), Severity: htmlSeverity(8)},
			},
			Comments: []collect.Comment{{
				ID: "backend:dropped-context-1", Profile: "backend", Priority: 8, Category: "correctness",
				Body:    "The retry loop calls c.do(context.Background(), req) rather than passing the caller's ctx.",
				Anchors: []collect.Anchor{{File: "internal/fetch/client.go", Line: 88}},
				Code:    "c.do(context.Background(), req)",
				Suggestions: []collect.Suggestion{{
					Summary: "Pass the caller's context straight through", Effort: "trivial", Scope: "line",
					Pros: []string{"Cancellation and deadlines propagate immediately"},
					Cons: []string{"A caller relying on retries outliving the request context sees a behavior change"},
				}},
			}},
		},
	}
}

// TestHTMLGolden is §2.2.1's one retained golden file for this
// renderer: reviewed by eye, not asserted on structurally, and kept
// small enough that a reviewer can read the whole diff — true again,
// per refinery-t1c.5, once bead7ElideStaticAssets removes the copied
// report.css/code.css/report.js bytes that used to make up 583 of its
// 697 lines. Run `go test ./internal/render/... -run TestHTMLGolden
// -update` to regenerate it after a deliberate, reviewed change; a
// change confined to the CSS or script's own bytes, with no
// correctness-carrying rule touched, now regenerates nothing here at
// all, since this golden no longer contains those bytes.
func TestHTMLGolden(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	require.NoError(t, NewHTML().CollectReviews(&out, bead7GoldenEnvelope()))
	golden(t, "html-report.html", bead7ElideStaticAssets(out.String()))
}
