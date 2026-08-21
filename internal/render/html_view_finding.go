// internal/render/html_view_finding.go — the findings half of the view
// model report.gohtml renders (§7.3, §7.4). Bead .1 seeds the minimal
// id/body shape a structurally complete page needs. Bead .9 owns
// extending this file: the 140-rune summary derivation, the
// priority-≥-7 open/closed default, the category/priority badges, the
// data-* filter attributes (§5.1, §5.4), and the suggestion view model
// templates/suggestion.gohtml renders all belong here rather than in a
// new file, since .9 is the bead that owns both templates/finding.gohtml
// and templates/suggestion.gohtml.
package render

import "github.com/bobcob7/loam-refinery/internal/collect"

// htmlFindingView is one comment's own view model (§7.3): the qualified
// id, unmodified — no substitution, in either qualifier form (§8.1) —
// and the free-text body, read straight off collect.Comment and never
// re-derived.
type htmlFindingView struct {
	ID   string
	Body string
}

// buildHTMLFindings converts every collect.Comment into its own
// htmlFindingView, in the same order Result.Comments already carries
// them — never a second sort, never a map (§10).
func buildHTMLFindings(comments []collect.Comment) []htmlFindingView {
	findings := make([]htmlFindingView, 0, len(comments))
	for _, c := range comments {
		findings = append(findings, htmlFindingView{ID: c.ID, Body: c.Body})
	}
	return findings
}
