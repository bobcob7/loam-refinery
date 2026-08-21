// internal/render/html_view_envelope.go — the envelope-strip half of the
// view model report.gohtml renders (§7.1). Bead .1 seeds the minimal
// fields the envelope strip needs — ref, repo, store.enabled, head_check,
// and the total/unreadable counts. Bead .3 owns extending this file: the
// submissions-index view model templates/index.gohtml renders (§7.2), and
// the empty-state case (§12), both belong here rather than in a new file,
// since .3 is the bead that owns both templates/envelope.gohtml and
// templates/index.gohtml.
package render

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
	return view
}
