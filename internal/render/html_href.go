// internal/render/html_href.go — the fragment-href helper (§8.2, §8.3):
// computes the href that links to a comment's own id attribute, and is
// the one place in this renderer that produces a template.URL value —
// html/template's own escaper-bypass type for a URL context, the mirror
// image of the template.JS/JSStr bypass §5.1 forbids everywhere on this
// page. It percent-encodes only an ordinal-qualified id's leading "#"
// (RFC 3986's fragment grammar has no room for a raw one, §8.2) and
// passes every other character through untouched, since the value it
// ever receives is the qualifier and origin id §6.1 of
// docs/features/combined-reviews.md already constrains structurally —
// never Body, Summary, Pros, Cons, or any other free-text field (§8.3).
//
// Bead .2 owns this file's content.
package render
