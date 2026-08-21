// internal/render/html_href_test.go — bead .2's own tests for
// fragmentHref (§8.1, §8.2, §8.3) and for the confinement argument
// html_href.go's doc comments make: that reaching this file's one
// template.URL bypass with reviewer-authored free text fails the render
// rather than forging one. Bead .7's full test suite (internal/render/
// html_test.go) owns the rest of the format's properties; this file
// covers only what this bead's own file is responsible for.
package render

import (
	"bytes"
	"html/template"
	"testing"

	"github.com/bobcob7/loam-refinery/internal/collect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFragmentHrefQualifierForms pins §8.2's two worked examples: a
// profile-qualified id's href is byte-identical to its id attribute, and
// an ordinal-qualified id's href differs only in its own leading "#"
// becoming "%23". Both are rendered through a real html/template, in the
// exact `<a href="#{{fragmentHref .}}">` shape html.go registers, rather
// than compared as raw Go strings, so a regression in either
// fragmentHref or the escaper interaction it depends on shows up here.
func TestFragmentHrefQualifierForms(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		id   string
		href string
	}{
		{name: "profile-qualified", id: "tests:band-sub-one-untested-1", href: `#tests:band-sub-one-untested-1`},
		{name: "ordinal-qualified", id: "#3:dropped-context-1", href: `#%233:dropped-context-1`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			tmpl := template.Must(template.New("t").Funcs(hrefFuncs).Parse(`<a href="#{{fragmentHref .}}">`))
			var buf bytes.Buffer
			require.NoError(t, tmpl.Execute(&buf, test.id))
			assert.NotContains(t, buf.String(), "ZgotmplZ")
			assert.Contains(t, buf.String(), `href="`+test.href+`"`)
		})
	}
}

// TestFragmentHrefRejectsNonQualifiedID is the confinement property from
// the other side: fragmentHref never emits a template.URL for input that
// is not shaped like a qualified id (§6.1 of
// docs/features/combined-reviews.md), which is what a call site
// mistakenly wired to Body, Summary, Pros, Cons, or suggestion text
// would supply. None of these render a link; each is rejected before a
// single byte of it reaches template.URL.
func TestFragmentHrefRejectsNonQualifiedID(t *testing.T) {
	t.Parallel()
	hostile := []string{
		"",
		"plain prose with no colon at all",
		`<script>alert(1)</script>`,
		`javascript:alert(1)`,
		`backend:"><script>alert(1)</script>`,
		"backend:dropped-context-1\nSet-Cookie: pwn=1",
		"backend:dropped-context-1 extra",
		"BACKEND:dropped-context-1",
		"backend::dropped-context-1",
		"#0:dropped-context-1",
		"#-1:dropped-context-1",
	}
	for _, id := range hostile {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			href, err := fragmentHref(id)
			require.Error(t, err)
			assert.ErrorIs(t, err, errNotQualifiedID)
			assert.Empty(t, href)
		})
	}
}

// hostileFindingEnvelope builds a fixture whose Body, anchor file, and
// every suggestion field carry an HTML metacharacter, a quote, a
// <script> element, and a javascript: URL — the fixture
// TestFragmentHrefCannotBeReachedFromReviewerProse and
// TestHostileFixtureNeverAltersTheStaticScript share. ID stays a single,
// well-formed, profile-qualified id: comment ids are validated upstream
// of this renderer (review-document.md §11.1's id-unique and
// profile-format checks), the same "safe by construction" category §4
// and §8.3 give profile and id, so a fixture that put attacker bytes
// there instead would be testing a precondition this renderer already
// assumes, not this bead's own escaping discipline.
func hostileFindingEnvelope(hostile string) CollectReviewsEnvelope {
	return CollectReviewsEnvelope{
		Ref: "10974f70",
		Result: &collect.Result{
			Comments: []collect.Comment{{
				ID:       "backend:dropped-context-1",
				Profile:  "backend",
				Priority: 8,
				Category: "correctness",
				Body:     hostile,
				Anchors:  []collect.Anchor{{File: hostile}},
				Suggestions: []collect.Suggestion{{
					Summary: hostile,
					Pros:    []string{hostile},
					Cons:    []string{hostile},
				}},
			}},
		},
	}
}

// TestFragmentHrefCannotBeReachedFromReviewerProse is the forgery test
// docs/features/html-report.md §2.2.1 calls for at this bead's altitude:
// a fixture whose Body, summary, pros, cons, and anchor file each carry
// an HTML metacharacter, a quote, a <script> element, and a javascript:
// URL, rendered through the real HTML renderer end to end. It parses the
// output with golang.org/x/net/html — never a string match, per §2.2.1
// — and asserts by property: the document still parses, and the href on
// the one qualified, well-formed comment id in the fixture still
// resolves to the literal fragment §8.2 specifies. This is the model
// TestAnAuthoredValueCannotForgeOutput (render_test.go) sets for the
// JSON renderer, applied to the one escaping bypass this format has.
//
// What this test does not itself assert about the page's <script>
// element is deliberate: bead .6 landed the inline script §5 requires
// after this test was first written, so "no <script> element anywhere
// in the output" stopped being the right property the moment a
// legitimate, content-free one started shipping with every page.
// TestHostileFixtureNeverAltersTheStaticScript, below, is this file's
// half of that updated property — coordinated with, not duplicating,
// bead .6's own TestScriptBytesAreIdenticalAcrossWildlyDifferentFixtures
// (html_script_test.go), which already pins that the script's bytes
// never vary between two very different but non-adversarial fixtures.
// That test does not exercise a fixture built specifically to inject
// into the script; this one does.
func TestFragmentHrefCannotBeReachedFromReviewerProse(t *testing.T) {
	t.Parallel()
	hostile := `<script>alert(1)</script> "FORGED" javascript:alert(1)`
	rendered, doc := renderHTML(t, hostileFindingEnvelope(hostile))
	assert.NotContains(t, rendered, "<script>alert(1)</script>", "the reviewer's own script element must never survive unescaped")
	chips := htmlNodesWithClass(doc, "finding-id")
	require.Len(t, chips, 1)
	assert.Equal(t, `#backend:dropped-context-1`, findingAttr(chips[0], "href"), "the one well-formed id in the fixture still resolves to its literal fragment")
}

// TestHostileFixtureNeverAltersTheStaticScript is this file's half of
// the property TestFragmentHrefCannotBeReachedFromReviewerProse used to
// pin alone, before bead .6's inline script made "the page ships no
// script at all" stop being true. The distinction that still matters is
// between the one known, static script (§5.1) and anything else: a
// hostile Body, anchor file, or suggestion field must never grow a
// second <script> element, and must never inject text into the one that
// already ships. This is deliberately narrower than
// TestScriptBytesAreIdenticalAcrossWildlyDifferentFixtures
// (html_script_test.go), which already pins the script's bytes constant
// across two very different, but non-adversarial, fixtures — that
// property does not, on its own, rule out a fixture built specifically
// to try to break into the script rather than merely differ from it.
func TestHostileFixtureNeverAltersTheStaticScript(t *testing.T) {
	t.Parallel()
	hostile := `<script>alert(1)</script> "FORGED" javascript:alert(1)`
	_, doc := renderHTML(t, hostileFindingEnvelope(hostile))
	scripts := scriptElements(doc)
	require.Len(t, scripts, 1, "a hostile fixture must still ship exactly the one static <script> element (§5.1) — never a second one forged from reviewer content")
	content := findingText(scripts[0])
	for _, token := range []string{hostile, "alert(1)", "FORGED", "javascript:alert(1)", "<script>"} {
		assert.NotContains(t, content, token, "the static script's content must never carry reviewer-authored text")
	}
}
