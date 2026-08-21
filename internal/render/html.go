// internal/render/html.go — bead .1 owns this file: the HTML renderer
// type, the embedded-template wiring, and CollectReviews's entry point.
// See html_view_envelope.go and html_view_finding.go for the two halves
// of the view model each fills — split into two files, rather than one
// html_view.go, specifically so beads .3 and .9 (graph-parallel, per
// docs/features/html-report.md's file-ownership map) never edit the same
// file. See templates/report.gohtml for the page skeleton every partial
// below is called from.
package render

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io"
)

// htmlTemplatesFS embeds every partial this renderer parses: the
// .gohtml page and its named {{define}} regions, plus report.css and
// code.css (§6.2, §9) and report.js (§5), each of which wraps its own
// static content in a {{define}} block with zero {{ }} actions inside —
// see templates/report.css, templates/code.css, and templates/report.js
// for why that shape, rather than a Go string constant, keeps
// html/template's own escaper-bypass types (template.JS, template.JSStr)
// out of this renderer entirely (§5.1, §2.2.1's script-purity pin).
// ParseFS, not a hand-maintained file list, is what keeps "adding a
// partial never means editing a parse list" true.
//
//go:embed templates/*.gohtml templates/*.css templates/*.js
var htmlTemplatesFS embed.FS

// HTML renders collect-reviews's envelope as one self-contained web page
// (docs/features/html-report.md) — a third projection of the identical
// CollectReviewsEnvelope value JSON.CollectReviews and Markdown.CollectReviews
// already take (§2), constructed the same way as those two rather than
// added to the renderer interface (§2.3): internal/cli's dispatch calls
// NewHTML().CollectReviews directly, the same precedent
// NewMarkdown().CollectReviews already set.
type HTML struct {
	tmpl *template.Template
}

// NewHTML returns the HTML renderer, with every template partial parsed
// once at construction — adding a partial never means editing a parse
// list, per htmlTemplatesFS's own comment.
func NewHTML() *HTML {
	tmpl := template.Must(template.New("report.gohtml").ParseFS(htmlTemplatesFS, "templates/*.gohtml", "templates/*.css", "templates/*.js"))
	return &HTML{tmpl: tmpl}
}

// CollectReviews writes collect-reviews's envelope as one HTML page to w
// (docs/features/html-report.md §1). It computes nothing collect.Assemble
// or the CLI-wiring layer did not already compute (§2): buildHTMLView is
// a pure function of envelope, ranging only over collect.Result's own
// ordered slices, never a map (§10) — rendering the same envelope twice
// in one process produces byte-identical output.
func (h *HTML) CollectReviews(w io.Writer, envelope CollectReviewsEnvelope) error {
	view := buildHTMLView(envelope)
	var buf bytes.Buffer
	if err := h.tmpl.ExecuteTemplate(&buf, "report.gohtml", view); err != nil {
		return fmt.Errorf("rendering html report: %w", err)
	}
	if _, err := w.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("writing html output: %w", err)
	}
	return nil
}

// htmlView is the root view model report.gohtml renders. Its two fields
// are each built in a sibling file, per htmlTemplatesFS's own comment on
// why the split exists.
type htmlView struct {
	Envelope htmlEnvelopeView
	Findings []htmlFindingView
}

// buildHTMLView assembles htmlView from envelope. It re-sorts nothing
// and re-filters nothing (§2): every element it produces comes from
// ranging over one of collect.Result's own ordered slices.
func buildHTMLView(envelope CollectReviewsEnvelope) htmlView {
	return htmlView{
		Envelope: buildHTMLEnvelope(envelope),
		Findings: buildHTMLFindings(envelope.Result.Comments),
	}
}
