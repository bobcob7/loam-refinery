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
	"strings"
	"testing"

	"github.com/bobcob7/loam-refinery/internal/collect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
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

// TestFragmentHrefCannotBeReachedFromReviewerProse is the forgery test
// docs/features/html-report.md §2.2.1 calls for at this bead's altitude:
// a fixture whose Body, Summary, Pros, Cons, and suggestion text each
// carry an HTML metacharacter, a quote, a <script> element, and a
// javascript: URL, rendered through the real HTML renderer end to end.
// It parses the output with golang.org/x/net/html — never a string
// match, per §2.2.1 — and asserts by property: the document still
// parses, carries no <script> element born from that content, and the
// href on the one qualified, well-formed comment id in the fixture still
// resolves to the literal fragment §8.2 specifies. This is the model
// TestAnAuthoredValueCannotForgeOutput (render_test.go) sets for the
// JSON renderer, applied to the one escaping bypass this format has.
func TestFragmentHrefCannotBeReachedFromReviewerProse(t *testing.T) {
	t.Parallel()
	hostile := `<script>alert(1)</script> "FORGED" javascript:alert(1)`
	envelope := CollectReviewsEnvelope{
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
	var buf bytes.Buffer
	require.NoError(t, NewHTML().CollectReviews(&buf, envelope))
	output := buf.String()
	assert.NotContains(t, output, "<script>alert(1)</script>", "the reviewer's own script element must never survive unescaped")
	doc, err := html.Parse(strings.NewReader(output))
	require.NoError(t, err, "the page must still parse as HTML even though every free-text field is hostile")
	assert.Zero(t, countInjectedScripts(doc), "no <script> element originates from reviewer-authored content")
	assert.Equal(t, `#backend:dropped-context-1`, findHref(doc, "backend:dropped-context-1"), "the one well-formed id in the fixture still resolves to its literal fragment")
}

// countInjectedScripts walks doc and counts every <script> element whose
// text content is not empty — the only <script> this renderer ever ships
// on its own is report.js's static, content-free define (§5.1), so any
// non-empty one in a hostile fixture's output would be attacker content
// that escaped its context.
func countInjectedScripts(n *html.Node) int {
	count := 0
	if n.Type == html.ElementNode && n.Data == "script" {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.TextNode && strings.TrimSpace(c.Data) != "" {
				count++
				break
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		count += countInjectedScripts(c)
	}
	return count
}

// findHref returns the href attribute of the first <a> element in doc
// whose text content equals text, or "" if none matches.
func findHref(n *html.Node, text string) string {
	if n.Type == html.ElementNode && n.Data == "a" && anchorText(n) == text {
		for _, attr := range n.Attr {
			if attr.Key == "href" {
				return attr.Val
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if href := findHref(c, text); href != "" {
			return href
		}
	}
	return ""
}

// anchorText returns n's own direct text-node content, concatenated.
func anchorText(n *html.Node) string {
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			sb.WriteString(c.Data)
		}
	}
	return sb.String()
}
