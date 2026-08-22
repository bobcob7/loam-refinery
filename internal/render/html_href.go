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

import (
	"fmt"
	"html/template"
	"regexp"
	"strings"
)

// errNotQualifiedID is fragmentHref's own rejection: the value it was
// asked to build a href for is not shaped like the qualified id
// combined-reviews.md §6.1 defines. In normal operation this never
// fires — review-document.md §11.1's id-unique and profile-format
// checks, and §4's comment-id grammar, already reject anything of this
// shape before collect-reviews ever assembles comments[].id — but it is
// what keeps this file's one template.URL call from ever trusting a
// value on the strength of a Go string type alone. A qualified id is
// "safe by construction" (§8.3) because its grammar admits nothing an
// HTML or URL context treats specially; this check is what makes that a
// property fragmentHref enforces rather than one it merely assumes.
var errNotQualifiedID = fmt.Errorf("not a qualified id")

// qualifiedIDPattern is combined-reviews.md §6.1's qualified id grammar,
// in both forms: "<profile>:<origin_id>", where profile is
// review-document.md §11.1's profile-format
// (^[a-z0-9]+(-[a-z0-9]+)*$), or "#<ordinal>:<origin_id>", where ordinal
// is a bare 1-based position with no leading zero. origin_id, in either
// form, is review-document.md §4's comment-id grammar
// (^[a-z][a-z0-9]*(-[a-z0-9]+)*-[1-9][0-9]*$). Neither half admits a
// colon, so the one literal ":" between them is also the one colon the
// whole id ever carries (§8.3) — and reviewer-authored free text (Body,
// Summary, Pros, Cons, suggestion text) has no reason to already be
// shaped this way, which is what makes it fail this match rather than
// pass through as a forged href.
var qualifiedIDPattern = regexp.MustCompile(
	`^(?:[a-z0-9]+(?:-[a-z0-9]+)*|#[1-9][0-9]*):[a-z][a-z0-9]*(?:-[a-z0-9]+)*-[1-9][0-9]*$`,
)

// fragmentHref computes the href that links to a comment's own qualified
// id (§8.2), the one deliberate bypass of this renderer's escaping
// discipline (§8.3, §4). Verified directly against html/template: the
// whole attribute as one dynamic pipeline, `<a href="{{.}}">`, renders
// `href="#ZgotmplZ"`, because the escaper's urlFilter reads the text
// before an id's single colon as a URL scheme and refuses it; a static
// "#" prefix with the id as a plain dynamic suffix, `<a href="#{{.}}">`,
// avoids that but still percent-encodes the colon,
// `href="#tests%3aband-sub-one-untested-1"`; only wrapping the computed
// value in template.URL, in that same `<a href="#{{fragmentHref .}}">`
// shape, yields the literal id. The one encoding fragmentHref itself
// performs is percent-encoding the ordinal form's own leading "#" — RFC
// 3986's fragment grammar (*( pchar / "/" / "?" )) has no room for a raw
// one — passing every other byte, colon included, through untouched, so
// a profile-qualified id's href comes out byte-identical to its id
// attribute and an ordinal-qualified id's differs only in that one
// escape (§8.2).
//
// id must be the qualified id itself — never Body, Summary, Pros, Cons,
// or any other reviewer-authored free-text field (§8.3) — and
// fragmentHref rejects, rather than encodes, anything that does not
// already match qualifiedIDPattern, so a call site wired to one of those
// fields by mistake fails the render instead of forging a link from it.
func fragmentHref(id string) (template.URL, error) {
	if !qualifiedIDPattern.MatchString(id) {
		return "", fmt.Errorf("fragment href for %q: %w", id, errNotQualifiedID)
	}
	if strings.HasPrefix(id, "#") {
		return template.URL("%23" + id[1:]), nil
	}
	return template.URL(id), nil
}

// hrefFuncs is the FuncMap entry that exposes fragmentHref to templates,
// under the one name a partial calls it by. It is unexported, lives only
// in this file, and is the only place in this package that constructs a
// template.URL value (docs/features/html-report.md's bead-.2 file
// ownership: "no other file in this renderer imports template.URL,
// template.HTML, template.JS or template.JSStr") — so wiring a fragment
// href anywhere in this renderer always means calling through here,
// never adding a second template.URL call site elsewhere.
var hrefFuncs = template.FuncMap{"fragmentHref": fragmentHref}
