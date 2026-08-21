// internal/render/html_view_finding.go — the findings half of the view
// model report.gohtml renders (§7.3, §7.4). Bead .1 seeded the minimal
// id/body shape a structurally complete page needs; this file is bead .9's:
// the 140-rune headline derivation, the priority-≥-7 open/closed default,
// the category/priority/profile/ordinal tags, the data-* filter attributes
// (§5.1, §5.4), and the suggestion view model templates/suggestion.gohtml
// renders.
package render

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/bobcob7/loam-refinery/internal/collect"
)

// headlineCap is §7.3's 140-rune truncation cap for a finding's derived
// headline.
const headlineCap = 140

// headlineEllipsis is appended only when the cut actually happened (§7.3):
// a first sentence that already fits under headlineCap renders whole, with
// no trailing mark implying a truncation that never occurred.
const headlineEllipsis = "…"

// profileFormatPattern is review-document.md §11.1's profile-format check
// (^[a-z0-9]+(-[a-z0-9]+)*$), reapplied here as a defence-in-depth guard on
// data-lens (§5.1): Comment.Profile ought to already satisfy this by the
// time it reaches this renderer, the same structural precondition
// html_href.go's qualifiedIDPattern leans on for the id it validates, but
// this file does not import that one — each escaping-adjacent file holds
// its own grammar check rather than sharing state across a bead boundary.
var profileFormatPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// htmlFindingView is one comment's own view model (§7.3): the qualified id,
// unmodified — no substitution, in either qualifier form (§8.1) — a derived
// headline, the id chip's own display text and title, the priority/category
// tags, the profile-or-ordinal provenance tags, the collapse default, the
// data-* filter attributes (§5.1), the anchor badges, the optional code
// excerpt, and any suggestions.
type htmlFindingView struct {
	// ID is the qualified id verbatim (§8.1) — the element's own id
	// attribute and the value fragmentHref (bead .2, html_href.go) turns
	// into the chip's self-link.
	ID string
	// Headline is the summary's emphasis: the first sentence of Body,
	// verbatim reviewer text, truncated at a word boundary to
	// headlineCap runes (§7.3) — never a paraphrase.
	Headline string
	// ChipText is the id chip's own display text: the full qualified id
	// for a profile-qualified finding, or the origin id alone — without
	// its leading "#<ordinal>:" — for an ordinal-qualified one (§7.3).
	// The element's id attribute and the chip's title carry the full
	// qualified id regardless; only the chip's visible text differs.
	ChipText string
	Body     string
	Priority int
	Category string
	// HasProfile and Profile render the profile tag (§7.3) whenever the
	// owning submission claimed a profile — true for every
	// profile-qualified finding, and also true for an ordinal-qualified
	// one whose submission claimed a profile but is superseded
	// (combined-reviews.md §6.1).
	HasProfile bool
	Profile    string
	// HasOrdinal and OrdinalTag render the ordinal tag (§7.3) — "#3" —
	// whenever ID is ordinal-qualified, independent of HasProfile: the
	// two tags answer different questions ("who wrote it" versus "which
	// submission") and are not mutually exclusive.
	HasOrdinal bool
	OrdinalTag string
	// Open is §7.3's collapse default, computed here and nowhere else:
	// true for Priority >= 7, false otherwise. It is a pure function of
	// Priority, so rendering the same finding twice produces the
	// identical value both times.
	Open bool
	// Anchors is every anchor's own "file:line" or "file:line-end_line"
	// label (§8.4), in Comment.Anchors' own order. Empty, never nil,
	// when the comment carries none — templates/finding.gohtml renders
	// the literal "(none)" for that case (§12), never an empty badge row.
	Anchors []string
	// HasCode and Code carry the comment's own code excerpt (§7.5) into
	// the "code" partial (bead .4). HasCode is false, and Code the zero
	// value, when Comment.Code is "" — templates/finding.gohtml never
	// calls {{template "code"}} in that case, so no empty <pre> renders
	// (§12).
	HasCode bool
	Code    htmlCodeView
	// Suggestions is every suggestion's own card view model (§7.4), in
	// Comment.Suggestions' own order.
	Suggestions []htmlSuggestionView
	// DataPriorityCSS, DataCategoryCSS, and DataLens are §5.1's three
	// data-* filter attributes: a priority band, a category, and a
	// profile-or-ordinal lens, each a grammar-constrained enum or
	// integer-derived token, never a byte of reviewer prose (§5.1).
	DataPriorityCSS string
	DataCategoryCSS string
	DataLens        string
}

// htmlCodeView is one code excerpt's own input to the "code" partial
// (bead .4, templates/code.gohtml, internal/render/html_highlight.go):
// the excerpt itself, verbatim, and the filename §6.3 says to infer a
// language from — the owning comment's first anchor for comment.code,
// the parent comment's first anchor for a suggestion's own code (§6.3),
// and "" when there is no anchor to read one from, which the code
// partial treats identically to an unrecognized extension (lexers.Fallback).
type htmlCodeView struct {
	Code     string
	Filename string
}

// htmlSuggestionView is one suggestion's own card view model (§7.4):
// summary, effort and scope as badge text, pros and cons in their own
// slices — rendered as sibling containers, never one nested in the
// other — and an optional code excerpt.
type htmlSuggestionView struct {
	Summary string
	Effort  string
	Scope   string
	Pros    []string
	Cons    []string
	HasCode bool
	Code    htmlCodeView
}

// buildHTMLFindings converts every collect.Comment into its own
// htmlFindingView, in the same order Result.Comments already carries them
// — never a second sort, never a map (§10).
func buildHTMLFindings(comments []collect.Comment) []htmlFindingView {
	findings := make([]htmlFindingView, 0, len(comments))
	for _, c := range comments {
		findings = append(findings, buildHTMLFinding(c))
	}
	return findings
}

// buildHTMLFinding builds one comment's htmlFindingView (§7.3).
func buildHTMLFinding(c collect.Comment) htmlFindingView {
	isOrdinal, ordinalTag, chipText := splitQualifiedID(c.ID)
	view := htmlFindingView{
		ID:              c.ID,
		Headline:        deriveHeadline(c.Body),
		ChipText:        chipText,
		Body:            c.Body,
		Priority:        c.Priority,
		Category:        c.Category,
		HasProfile:      c.Profile != "",
		Profile:         c.Profile,
		HasOrdinal:      isOrdinal,
		OrdinalTag:      ordinalTag,
		Open:            c.Priority >= 7,
		Anchors:         buildAnchorLabels(c.Anchors),
		Suggestions:     buildHTMLSuggestions(c),
		DataPriorityCSS: priorityBandCSS(c.Priority),
		DataCategoryCSS: categoryCSS(c.Category),
		DataLens:        dataLens(c.Profile, isOrdinal, ordinalTag),
	}
	view.Code, view.HasCode = buildHTMLCode(c.Code, c.Anchors)
	return view
}

// splitQualifiedID reads apart a qualified id (combined-reviews.md §6.1)
// into what §7.3's chip and tags need: whether it is ordinal-qualified,
// the ordinal tag text ("#3") when it is, and the chip's own display
// text — the full id for a profile-qualified finding, or the origin id
// alone for an ordinal-qualified one, per §7.3's worked example
// (`dropped-context-1`, not `#3:dropped-context-1`).
func splitQualifiedID(id string) (isOrdinal bool, ordinalTag, chipText string) {
	if !strings.HasPrefix(id, "#") {
		return false, "", id
	}
	rest := id[1:]
	colon := strings.IndexByte(rest, ':')
	if colon < 0 {
		return true, "", id
	}
	return true, "#" + rest[:colon], rest[colon+1:]
}

// deriveHeadline is §7.3's headline derivation: the first sentence of
// body — everything up to and including the first '.', '!', or '?' that
// is immediately followed by whitespace or the end of body, or the whole
// body when it carries no such terminator — truncated at a word boundary
// to headlineCap runes when that sentence alone exceeds the cap.
// Truncation is a cut, never a rewrite: nothing here invents, summarises,
// reorders, or rewords reviewer prose. An ellipsis is appended only when
// the cut actually happened; a first sentence that already fits under
// the cap renders whole.
//
// Requiring trailing whitespace (or end of body) after the punctuation
// mark is what keeps a period inside a token — "schema.sql",
// "internal/store/sql/schema.sql:9" — from being misread as a sentence
// end; a real fixture (10974f7's own architecture:migrated-column-order-diverges-1)
// surfaced exactly this: "schema.sql declares assessment eighth…" was
// truncated to "schema." before this check existed. This is not a full
// abbreviation-aware sentence splitter — "e.g. " still reads as a
// sentence end — but it is the minimal fix for the shape of false
// positive real review prose actually produces, without inventing a
// heuristic the spec never asked for.
func deriveHeadline(body string) string {
	runes := []rune(body)
	end := len(runes)
	for i, r := range runes {
		if r != '.' && r != '!' && r != '?' {
			continue
		}
		if i+1 == len(runes) || unicode.IsSpace(runes[i+1]) {
			end = i + 1
			break
		}
	}
	if end <= headlineCap {
		return string(runes[:end])
	}
	return truncateAtWordBoundary(runes, headlineCap) + headlineEllipsis
}

// truncateAtWordBoundary cuts runes to at most capN runes, backing off to
// the last whitespace rune within that window so the cut lands after a
// whole word rather than mid-word. The single exception (§7.3) is a run
// with no whitespace at all inside the window — one unbroken token, an
// identifier or a URL, longer than the cap by itself — which is cut hard
// at capN rather than either overflowing the cap or vanishing outright,
// since there is no earlier word boundary to cut it at.
func truncateAtWordBoundary(runes []rune, capN int) string {
	window := runes[:capN]
	lastSpace := -1
	for i := len(window) - 1; i >= 0; i-- {
		if unicode.IsSpace(window[i]) {
			lastSpace = i
			break
		}
	}
	if lastSpace == -1 {
		return string(window)
	}
	return strings.TrimRight(string(window[:lastSpace]), " \t\n\r")
}

// buildAnchorLabels renders every anchor as its own "file:line" or
// "file:line-end_line" label (§8.4), in Comment.Anchors' own order.
// Always non-nil, even when anchors is empty, so
// templates/finding.gohtml's own emptiness check reads len(.Anchors) ==
// 0 rather than a nil-vs-empty distinction that carries no meaning here
// — the same convention html_view_envelope.go's buildHTMLSubmissions
// already follows.
func buildAnchorLabels(anchors []collect.Anchor) []string {
	labels := make([]string, 0, len(anchors))
	for _, a := range anchors {
		label := a.File + ":" + strconv.Itoa(a.Line)
		if a.EndLine != nil {
			label += "-" + strconv.Itoa(*a.EndLine)
		}
		labels = append(labels, label)
	}
	return labels
}

// buildHTMLCode builds one code excerpt's htmlCodeView and reports
// whether it should render at all. ok is false, and the returned value
// the zero htmlCodeView, when code is "" (§12) — the caller never calls
// the "code" partial in that case, so no empty <pre> renders. filename is
// anchors[0].File when anchors carries at least one entry, "" otherwise
// (§6.3).
func buildHTMLCode(code string, anchors []collect.Anchor) (htmlCodeView, bool) {
	if code == "" {
		return htmlCodeView{}, false
	}
	filename := ""
	if len(anchors) > 0 {
		filename = anchors[0].File
	}
	return htmlCodeView{Code: code, Filename: filename}, true
}

// buildHTMLSuggestions converts one comment's Suggestions into their own
// view models, in the comment's own order. code, when a suggestion
// carries one, infers its language from the parent comment's first
// anchor (§6.3) — a suggestion carries no anchors of its own.
func buildHTMLSuggestions(c collect.Comment) []htmlSuggestionView {
	views := make([]htmlSuggestionView, 0, len(c.Suggestions))
	for _, s := range c.Suggestions {
		view := htmlSuggestionView{
			Summary: s.Summary,
			Effort:  s.Effort,
			Scope:   s.Scope,
			Pros:    s.Pros,
			Cons:    s.Cons,
		}
		view.Code, view.HasCode = buildHTMLCode(s.Code, c.Anchors)
		views = append(views, view)
	}
	return views
}

// priorityBandCSS maps a priority to review-document.md §8's four bands,
// as the data-priority-css enum value §5.1 specifies — "pri-mustfix" for
// 9-10, "pri-shouldfix" for 7-8, "pri-worthfixing" for 4-6, "pri-optional"
// for 1-3 and anything else. This is the identical boundary §7.3's Open
// default uses (priority >= 7 spans should-fix and must-fix), computed
// from an int, never a string, so there is nothing here for reviewer
// prose to forge.
func priorityBandCSS(priority int) string {
	switch {
	case priority >= 9:
		return "pri-mustfix"
	case priority >= 7:
		return "pri-shouldfix"
	case priority >= 4:
		return "pri-worthfixing"
	default:
		return "pri-optional"
	}
}

// categoryCSS maps category to its data-category-css enum value —
// "cat-" plus the category name, for exactly the seven values
// review-document.md §9 defines. Anything outside that closed set — an
// upstream validation gap, not an expected input — maps to the constant
// "cat-other" rather than being echoed into the attribute verbatim, so a
// malformed Category can never reach a data-* attribute as arbitrary
// text.
func categoryCSS(category string) string {
	switch category {
	case "correctness", "security", "performance", "maintainability", "testing", "documentation", "style":
		return "cat-" + category
	default:
		return "cat-other"
	}
}

// dataLens computes §5.1's data-lens value: the profile when the owning
// submission claimed one and it is shaped like profile-format, the
// ordinal tag when it did not (or the claimed value fails that shape
// check), and the fixed fallback "lens-unknown" in the pathological case
// neither is available — a case combined-reviews.md §6.1's own qualifier
// rule should make unreachable, but this function never leaves data-lens
// empty regardless.
func dataLens(profile string, isOrdinal bool, ordinalTag string) string {
	if profile != "" && profileFormatPattern.MatchString(profile) {
		return profile
	}
	if isOrdinal && ordinalTag != "" {
		return ordinalTag
	}
	return "lens-unknown"
}
