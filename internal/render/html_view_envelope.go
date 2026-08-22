// internal/render/html_view_envelope.go — the envelope-strip half of the
// view model report.gohtml renders (§7.1). Bead .1 seeds the minimal
// fields the envelope strip needs — ref, repo, store.enabled, head_check,
// and the total/unreadable counts. Bead .3 owns extending this file: the
// submissions-index view model templates/index.gohtml renders (§7.2), and
// the empty-state case (§12), both belong here rather than in a new file,
// since .3 is the bead that owns both templates/envelope.gohtml and
// templates/index.gohtml.
package render

import (
	"strconv"
	"strings"

	"github.com/bobcob7/loam-refinery/internal/collect"
)

// htmlEnvelopeView is the envelope strip's own view model (§7.1): ref,
// repo, store.enabled, head_check, and the total/unreadable counts —
// every field CollectReviewsEnvelope and its Result already carry (§2),
// nothing computed a second time. IsHead and Diverged are flattened out
// of their *bool / []CollectReviewsDiverged pointer shapes into a value
// plus a Has* bool, the same absent-versus-present distinction
// markdown.go's writeMarkdownEnvelope already renders, so the template
// itself never has to reason about a pointer's nilness.
type htmlEnvelopeView struct {
	Ref                  string
	RepoName             string
	RepoKnown            bool
	StoreEnabled         bool
	HeadCheckSource      string
	HeadCheckIsHead      bool
	HeadCheckHasIsHead   bool
	HeadCheckDiverged    int
	HeadCheckHasDiverged bool
	Total                int
	Unreadable           int
	// Submissions is the submissions-index view model (§7.2), one entry
	// per collect.Submission in Result's own order — never re-sorted,
	// never filtered. Empty (never nil) when the ref has no
	// submissions, which is exactly what templates/index.gohtml checks
	// to render §12's "no submissions found for this ref" empty state
	// instead of a grid with nothing in it.
	Submissions []htmlSubmissionView
}

// htmlSubmissionView is one submission's own row in the submissions index
// (§7.2): ordinal, profile, verdict, assessment, and severity, the same
// five fields the index's fixed CSS-grid columns render, plus the
// qualified id of the first finding this submission filed — the link
// templates/index.gohtml wires through fragmentHref (html_href.go) so a
// reader who wants to see why a row reads the way it does can jump
// straight into the findings section, the same "where do I go next"
// question the submissions index exists to answer first. Profile and
// Assessment are already resolved to their placeholder text ("(none)")
// here, not left as "" / nil, so the template never has to reason about
// an absent value's zero shape — the same reasoning
// buildHTMLEnvelope's own IsHead/Diverged flattening already applies.
//
// Summary and HasSupersededBy/SupersededBy are the two fields
// markdown.go's writeMarkdownSubmissions already renders per submission
// (§8.1's superseded_by, §8.3.2's summary) that the fixed five-column
// grid never carried — dropping them made the HTML projection lossy
// next to the other two (refinery-t1c.4). Neither joins the grid as a
// sixth column: Summary is 30-1500 characters of free-text prose, the
// same shape markdown already sets apart as an indented paragraph below
// its bullet rather than a sixth inline field, so templates/index.gohtml
// renders it the same way — its own full-width row beneath the
// five-column one, never crammed into a fixed-width cell. SupersededBy
// is a currency signal, not content: a reader has to know a row is
// stale before reading it, so it renders as a small tag inline in the
// ordinal cell, the first thing a reader scans, with the row itself
// carrying an "index-row-superseded" class so its cells sit visually
// muted next to the submission that replaced it.
type htmlSubmissionView struct {
	Ordinal    int
	Profile    string
	Verdict    string
	Assessment string
	Severity   string
	// Summary is s.Summary, verbatim — free-text prose, escaped by
	// html/template's own contextual autoescaper the same way every
	// other reviewer-authored field on this page already is (§4),
	// never trimmed, elided, or summarized.
	Summary string
	// HasSupersededBy and SupersededBy flatten collect.Submission's
	// *int the same way HeadCheckHasIsHead/HeadCheckIsHead already
	// flatten HeadCheck.IsHead above: nil (current, or unprofiled, which
	// has no supersession axis at all) leaves HasSupersededBy false and
	// the template renders no marker; non-nil names the ordinal of the
	// submission that is current for this one's profile, the identical
	// fact markdown renders as "· superseded_by=#N" beside verdict.
	HasSupersededBy bool
	SupersededBy    int
	// FindingID is the qualified id of the first comment (in Result's
	// own Comments order) belonging to this submission, "" when the
	// submission filed none. templates/index.gohtml only renders a
	// link when this is non-empty — a submission with no comments has
	// nothing to link to (§12: "there is nothing to expand for a
	// submission that filed nothing").
	FindingID string
}

// buildHTMLEnvelope reads envelope's own fields into htmlEnvelopeView,
// verbatim. Total is the same "submissions + unreadable" computation
// json.go's CollectReviews and markdown.go's writeMarkdownEnvelope both
// already make (§8.1 of docs/features/combined-reviews.md), not re-derived
// a third way here.
func buildHTMLEnvelope(envelope CollectReviewsEnvelope) htmlEnvelopeView {
	view := htmlEnvelopeView{
		Ref:             envelope.Ref,
		RepoName:        envelope.RepoName,
		RepoKnown:       envelope.RepoKnown,
		StoreEnabled:    envelope.StoreEnabled,
		HeadCheckSource: envelope.HeadCheck.Source,
		Total:           len(envelope.Result.Submissions) + envelope.Result.Unreadable,
		Unreadable:      envelope.Result.Unreadable,
	}
	if envelope.HeadCheck.IsHead != nil {
		view.HeadCheckHasIsHead = true
		view.HeadCheckIsHead = *envelope.HeadCheck.IsHead
	}
	if envelope.HeadCheck.Diverged != nil {
		view.HeadCheckHasDiverged = true
		view.HeadCheckDiverged = len(envelope.HeadCheck.Diverged)
	}
	view.Submissions = buildHTMLSubmissions(envelope.Result.Submissions, envelope.Result.Comments)
	return view
}

// buildHTMLSubmissions converts every collect.Submission into its own
// htmlSubmissionView, in Result.Submissions' own order — never a second
// sort, never a map (§10). Always non-nil, even when submissions is
// empty, so templates/index.gohtml's own "no submissions" check reads
// len(.Submissions) == 0 rather than a nil-vs-empty distinction that
// carries no meaning here.
func buildHTMLSubmissions(submissions []collect.Submission, comments []collect.Comment) []htmlSubmissionView {
	views := make([]htmlSubmissionView, 0, len(submissions))
	for _, s := range submissions {
		view := htmlSubmissionView{
			Ordinal:    s.Ordinal,
			Profile:    submissionProfileText(s.Profile),
			Verdict:    s.Verdict,
			Assessment: submissionAssessmentText(s.Assessment),
			Severity:   formatSeverity(s.Severity),
			Summary:    s.Summary,
			FindingID:  firstFindingID(s, comments),
		}
		if s.SupersededBy != nil {
			view.HasSupersededBy = true
			view.SupersededBy = *s.SupersededBy
		}
		views = append(views, view)
	}
	return views
}

// submissionProfileText renders a submission's Profile, or the "(none)"
// placeholder markdown.go's own writeMarkdownSubmissions already uses
// for an unprofiled submission (§12) — never a blank cell, which would
// read as a rendering gap rather than a genuine "claimed no profile".
func submissionProfileText(profile string) string {
	if profile == "" {
		return "(none)"
	}
	return profile
}

// submissionAssessmentText renders a submission's Assessment. nil — the
// reviewer never set the field — renders the literal "(none)", the
// identical placeholder markdown.go already uses for the same absent
// case (§12), never one of the four real grade words standing in for
// silence: collapsing absence to a default level here would undo what
// the assessment feature's own JSON layer went to a whole bead's worth
// of trouble to keep distinguishable.
func submissionAssessmentText(assessment *string) string {
	if assessment == nil {
		return "(none)"
	}
	return *assessment
}

// firstFindingID returns the qualified id of the first comment, in
// Result.Comments' own order, that belongs to submission s — "" when s
// filed none. The qualifier a comment's id carries follows
// combined-reviews.md §6.1's own rule exactly: s's Profile when s is
// current for that profile (SupersededBy nil and Profile set),
// otherwise "#" plus s's Ordinal. Matching on that qualifier's prefix,
// rather than ranging s.Document's own comment ids or s.QualifiedIDs
// (a map, whose iteration order is not deterministic and whose doc
// comment already says it is carried for head_check, not for
// rendering), keeps this a pure, order-preserving function of the two
// slices §10 already requires this renderer to only ever range.
func firstFindingID(s collect.Submission, comments []collect.Comment) string {
	qualifier := "#" + strconv.Itoa(s.Ordinal)
	if s.Profile != "" && s.SupersededBy == nil {
		qualifier = s.Profile
	}
	prefix := qualifier + ":"
	for _, c := range comments {
		if strings.HasPrefix(c.ID, prefix) {
			return c.ID
		}
	}
	return ""
}
