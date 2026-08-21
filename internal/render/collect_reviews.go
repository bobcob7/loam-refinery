package render

import (
	"io"

	"github.com/bobcob7/loam-refinery/internal/collect"
)

// CollectReviewsEnvelope is everything collect-reviews's JSON renderer needs
// to encode the envelope docs/features/combined-reviews.md §8.1 specifies:
// what internal/collect.Assemble computed, plus the repo, store, and
// head_check facts that package deliberately does not know about
// (internal/collect's own package doc) and the CLI-wiring layer assembles
// around it.
type CollectReviewsEnvelope struct {
	Ref          string
	RepoName     string
	RepoKnown    bool
	StoreEnabled bool
	HeadCheck    CollectReviewsHeadCheck
	Result       *collect.Result
}

// CollectReviewsHeadCheck is head_check's shape (§4.3.1). IsHead is nil
// when Source != "repo" — absent, never a guessed false. Diverged is nil
// when the check does not apply (Source != "repo", or IsHead false or
// nil) and non-nil, possibly empty, once the check actually ran — the
// same absent-versus-empty distinction §4.3.1's own table draws.
type CollectReviewsHeadCheck struct {
	Source   string
	IsHead   *bool
	Diverged []CollectReviewsDiverged
}

// CollectReviewsDiverged is one entry in head_check.diverged (§4.3.1).
type CollectReviewsDiverged struct {
	Name    string
	Comment string
	File    string
	Message string
}

// collectReviewsPayload is the encoded shape §8.1 and §12's worked examples
// pin. Submissions and Comments are never nil going in — CollectReviews
// always initializes both — so an empty result renders "submissions":[] and
// "comments":[], never null.
type collectReviewsPayload struct {
	Ref         string                         `json:"ref"`
	Repo        collectReviewsRepoJSON         `json:"repo"`
	Store       collectReviewsStoreJSON        `json:"store"`
	HeadCheck   collectReviewsHeadCheckJSON    `json:"head_check"`
	Total       int                            `json:"total"`
	Unreadable  int                            `json:"unreadable"`
	Submissions []collectReviewsSubmissionJSON `json:"submissions"`
	Comments    []collectReviewsCommentJSON    `json:"comments"`
}

type collectReviewsRepoJSON struct {
	Name  string `json:"name"`
	Known bool   `json:"known"`
}

type collectReviewsStoreJSON struct {
	Enabled bool `json:"enabled"`
}

// collectReviewsHeadCheckJSON's Diverged is a pointer to a slice, not a
// plain slice, deliberately: encoding/json's omitempty treats a non-nil
// empty slice and a nil one identically (both "empty", both omitted), which
// would erase §4.3.1's absent-versus-[]-found-nothing distinction. A nil
// *[]... omits the key; a non-nil pointer to a possibly-empty slice keeps
// it, exactly the same trick reviews.go's Unreadable *int already plays for
// an int whose zero value is itself meaningful.
type collectReviewsHeadCheckJSON struct {
	Source   string                        `json:"source"`
	IsHead   *bool                         `json:"is_head,omitempty"`
	Diverged *[]collectReviewsDivergedJSON `json:"diverged,omitempty"`
}

type collectReviewsDivergedJSON struct {
	Name    string `json:"name"`
	Comment string `json:"comment"`
	File    string `json:"file"`
	Message string `json:"message"`
}

type collectReviewsSubmissionJSON struct {
	Ordinal      int    `json:"ordinal"`
	Profile      string `json:"profile,omitempty"`
	Verdict      string `json:"verdict"`
	Summary      string `json:"summary"`
	SupersededBy *int   `json:"superseded_by,omitempty"`
}

type collectReviewsCommentJSON struct {
	ID          string                         `json:"id"`
	Profile     string                         `json:"profile,omitempty"`
	Priority    int                            `json:"priority"`
	Category    string                         `json:"category"`
	Body        string                         `json:"body"`
	Code        string                         `json:"code,omitempty"`
	Anchors     []collectReviewsAnchorJSON     `json:"anchors"`
	Suggestions []collectReviewsSuggestionJSON `json:"suggestions"`
}

type collectReviewsAnchorJSON struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	EndLine *int   `json:"end_line,omitempty"`
}

type collectReviewsSuggestionJSON struct {
	Summary string   `json:"summary"`
	Effort  string   `json:"effort"`
	Scope   string   `json:"scope"`
	Pros    []string `json:"pros"`
	Cons    []string `json:"cons"`
	Code    string   `json:"code,omitempty"`
}

// CollectReviews writes collect-reviews's envelope to stdout
// (docs/features/combined-reviews.md §8.1). Total is derived, not carried on
// the envelope: it is always len(Result.Submissions) + Result.Unreadable —
// "the number of distinct digests found for the ref, before any are dropped
// as unreadable" (§8.1) — which is exactly what Assemble's own accounting
// already guarantees, so there is nothing for a caller to get wrong by
// passing it separately.
func (j *JSON) CollectReviews(w io.Writer, envelope CollectReviewsEnvelope) error {
	payload := collectReviewsPayload{
		Ref:   envelope.Ref,
		Repo:  collectReviewsRepoJSON{Name: envelope.RepoName, Known: envelope.RepoKnown},
		Store: collectReviewsStoreJSON{Enabled: envelope.StoreEnabled},
		HeadCheck: collectReviewsHeadCheckJSON{
			Source: envelope.HeadCheck.Source,
			IsHead: envelope.HeadCheck.IsHead,
		},
		Total:       len(envelope.Result.Submissions) + envelope.Result.Unreadable,
		Unreadable:  envelope.Result.Unreadable,
		Submissions: make([]collectReviewsSubmissionJSON, 0, len(envelope.Result.Submissions)),
		Comments:    make([]collectReviewsCommentJSON, 0, len(envelope.Result.Comments)),
	}
	if envelope.HeadCheck.Diverged != nil {
		diverged := make([]collectReviewsDivergedJSON, 0, len(envelope.HeadCheck.Diverged))
		for _, d := range envelope.HeadCheck.Diverged {
			diverged = append(diverged, collectReviewsDivergedJSON{Name: d.Name, Comment: d.Comment, File: d.File, Message: d.Message})
		}
		payload.HeadCheck.Diverged = &diverged
	}
	for _, s := range envelope.Result.Submissions {
		payload.Submissions = append(payload.Submissions, collectReviewsSubmissionJSON{
			Ordinal:      s.Ordinal,
			Profile:      s.Profile,
			Verdict:      s.Verdict,
			Summary:      s.Summary,
			SupersededBy: s.SupersededBy,
		})
	}
	for _, c := range envelope.Result.Comments {
		payload.Comments = append(payload.Comments, collectReviewsCommentJSON{
			ID:          c.ID,
			Profile:     c.Profile,
			Priority:    c.Priority,
			Category:    c.Category,
			Body:        c.Body,
			Code:        c.Code,
			Anchors:     collectReviewsAnchors(c.Anchors),
			Suggestions: collectReviewsSuggestions(c.Suggestions),
		})
	}
	return write(w, payload)
}

func collectReviewsAnchors(anchors []collect.Anchor) []collectReviewsAnchorJSON {
	out := make([]collectReviewsAnchorJSON, 0, len(anchors))
	for _, a := range anchors {
		out = append(out, collectReviewsAnchorJSON{File: a.File, Line: a.Line, EndLine: a.EndLine})
	}
	return out
}

func collectReviewsSuggestions(suggestions []collect.Suggestion) []collectReviewsSuggestionJSON {
	out := make([]collectReviewsSuggestionJSON, 0, len(suggestions))
	for _, s := range suggestions {
		out = append(out, collectReviewsSuggestionJSON{
			Summary: s.Summary,
			Effort:  s.Effort,
			Scope:   s.Scope,
			Pros:    nonNilStrings(s.Pros),
			Cons:    nonNilStrings(s.Cons),
			Code:    s.Code,
		})
	}
	return out
}

// nonNilStrings returns in unchanged when it is already non-nil, or an
// empty, non-nil slice otherwise, so pros/cons render "[]" rather than
// null when the stored suggestion never set either.
func nonNilStrings(in []string) []string {
	if in != nil {
		return in
	}
	return []string{}
}
