package advisory

import (
	"fmt"
	"strings"

	"github.com/bobcob7/refinery/internal/review"
)

// vacuousPhrases are bodies that meet the schema minimum while saying nothing.
var vacuousPhrases = []string{
	"lgtm", "looks good", "looks good to me", "no issues", "no issues found",
	"nothing to add", "see above", "see comment above", "consider refactoring",
	"could be better", "needs work", "fine by me", "no comments",
}

const bodyFloor = 60

// substance returns the advisories about a review carrying actual content.
func substance() []Advisory {
	return []Advisory{
		{
			Meta: review.Check{
				Name:    "body-thin",
				Tier:    review.TierAdvisory,
				Summary: "body is under 60 characters after normalization",
				Title:   "Thin comment body",
				Body: `Fires when a body is under 60 characters once whitespace is normalized. It
meets the schema minimum but is unlikely to carry a rationale, and a finding
without one is a preference the author can dismiss without cost.

A body should say what is wrong and what follows from it. The consequence is the
part that survives the trip to whoever fixes it.

  before: "Context handling is wrong here."
  after:  "The retry loop passes context.Background() rather than the caller's
           ctx, so a cancelled request keeps retrying for the full backoff
           schedule, holding the connection past the caller's deadline."

Length is a proxy, not the point: a genuinely one-line finding that names the
symptom and the consequence will clear 60 characters on its own.`,
				Related: []string{"body", "vacuous-body"},
			},
			Run: bodyThin,
		},
		{
			Meta: review.Check{
				Name:    "vacuous-body",
				Tier:    review.TierAdvisory,
				Summary: "body matches a low-signal phrase and says nothing actionable",
				Title:   "Vacuous comment body",
				Body: `Fires when a body, normalized and lowercased, is nothing but a low-signal
phrase: "lgtm", "looks good", "no issues", "see above", "consider refactoring"
with nothing following. These pass the schema's length floor while transferring
no information, and they are the exact failure mode this format exists to catch
— a hollow review that is indistinguishable from a real one downstream.

"See above" is worth calling out separately: a comment is addressed by id and
read in isolation, so a body that refers to another comment by position is
unreadable to the consumer that received only this one. Name the id, or repeat
the finding.

  before: "Consider refactoring."
  after:  "parseHeader and parseTrailer differ only in the delimiter; folding
           them into one function keeps the two parsers from drifting."`,
				Related: []string{"body", "body-thin"},
			},
			Run: vacuousBody,
		},
		{
			Meta: review.Check{
				Name:    "suggestion-absent",
				Tier:    review.TierAdvisory,
				Summary: "a comment at priority 7 or above proposes no fix",
				Title:   "High-priority finding with no suggestion",
				Body: `Fires when a comment at priority 7 or above carries no suggestions. At that
priority you are saying the change should not merge as it stands, so the
orchestrator has to act — and with no suggestion it has to derive the fix from
your prose, which is the work you were asked to do.

One suggestion is enough when there is one move. Two are better when the finding
can be answered at different blast radii: patch the call site, or change the
type so the mistake stops being expressible.

  "suggestions": [{ "summary": "Pass the caller's ctx through to c.do",
    "effort": "trivial", "scope": "line", "pros": ["Deadlines propagate"],
    "cons": ["Callers relying on the old behaviour see a change"] }]

If you cannot propose anything, the priority is usually too high: lower it and
say what you are unsure about.`,
				Related: []string{"suggestions", "priority"},
			},
			Run: suggestionAbsent,
		},
		{
			Meta: review.Check{
				Name:    "suggestion-no-cons",
				Tier:    review.TierAdvisory,
				Summary: "a suggestion lists no cons",
				Title:   "Suggestion with no stated downside",
				Body: `Fires when a suggestion's cons list is empty. A fix with no stated downside is
either trivially correct or under-examined, and the consumer choosing between
your suggestions cannot tell which — that difference is most of the value of
offering a choice at all.

State the cost even when it is small: a behaviour change some caller may rely
on, a test that has to be rewritten, a signature other packages import.

  "cons": ["Callers relying on retries outliving the request context see a
            behaviour change"]

If the fix really is free, say so in a pro ("no caller-visible change") and
carry the advisory. It is a note that the tradeoff was examined and found empty,
not a finding that you were wrong.`,
				Related: []string{"cons", "pros", "broad-scope-no-cons"},
			},
			Run: suggestionNoCons,
		},
		{
			Meta: review.Check{
				Name:    "suggestion-no-pros",
				Tier:    review.TierAdvisory,
				Summary: "a suggestion lists no pros",
				Title:   "Suggestion with no stated upside",
				Body: `Fires when a suggestion's pros list is empty. With one suggestion the omission
is mild; with two it is the whole problem, because a consumer choosing between
them has only the summaries to go on and no statement of what each option buys.

Say what this option gets that the alternatives do not — and say it in terms of
consequences, not restatement of the summary.

  before: "pros": []
  after:  "pros": ["Cancellation propagates immediately",
                   "Nothing outside the loop body has to be re-read"]

An empty list is legal, and sometimes honest for a fix whose only merit is that
it is correct. Filling it in costs one line and makes the comparison possible.`,
				Related: []string{"pros", "cons"},
			},
			Run: suggestionNoPros,
		},
		{
			Meta: review.Check{
				Name:    "broad-scope-alone",
				Tier:    review.TierAdvisory,
				Summary: "a comment's only suggestion has module or project blast radius",
				Title:   "Wide fix offered with no alternative",
				Body: `Fires when a comment's single suggestion has scope module or project. A wide
blast radius offered with no narrower alternative gives the orchestrator no
choice exactly where the choice costs most: it can take a change that reaches
other packages or the whole repository, or it can take nothing.

Offer the narrow option too, even if you prefer the wide one. Scope sits on the
suggestion precisely because one finding can be answered at several radii.

  1. "Pass the caller's context through at this call site"  — scope: line
  2. "Take a context on the exported Fetch method"          — scope: module

The advisory does not say the wide fix is wrong. It says the consumer should be
allowed to defer it, and that deferring is only possible if you named something
smaller.`,
				Related: []string{"scope", "suggestions", "broad-scope-no-cons"},
			},
			Run: broadScopeAlone,
		},
		{
			Meta: review.Check{
				Name:    "broad-scope-no-cons",
				Tier:    review.TierAdvisory,
				Summary: "a module or project suggestion lists no cons",
				Title:   "Wide fix with no stated cost",
				Body: `Fires when a suggestion at scope module or project has an empty cons list.
Reaching that far always costs something — other packages adapt, an exported
signature breaks, a convention has to be applied everywhere it was not — and a
wide change presented as free is the one a consumer is most likely to apply
without checking.

Name what has to be re-read, re-tested or re-approved:

  "scope": "module",
  "cons": ["Breaks the exported signature; every caller of Fetch is updated",
           "Larger blast radius than the defect strictly requires"]

This fires alongside suggestion-no-cons, which is deliberate: the empty list is
worth one note on its own and another when the fix is this wide.`,
				Related: []string{"scope", "cons", "broad-scope-alone"},
			},
			Run: broadScopeNoCons,
		},
		{
			Meta: review.Check{
				Name:    "summary-thin",
				Tier:    review.TierAdvisory,
				Summary: "summary is under 60 characters on a review with three or more comments",
				Title:   "Thin review summary",
				Body: `Fires when the document summary is under 60 characters and the review carries
three or more comments. The summary is what a consumer reads when it will not
read twelve comments, so on a review with real findings it has to carry the
shape of them — not restate the verdict, which is already a field.

  before: "Changes requested."
  after:  "The retry loop is sound, but the context deadline is not propagated
           to the downstream call, so a cancelled request keeps retrying until
           the attempt budget is exhausted."

One to three sentences is the target. Say what holds up as well as what does
not: a summary that only lists problems reads as a rejection even when the
verdict is comment.`,
				Related: []string{"field:summary", "verdict"},
			},
			Run: summaryThin,
		},
	}
}

func bodyThin(doc *review.Document) ([]review.Diagnostic, []review.Skipped) {
	diagnostics := []review.Diagnostic{}
	for _, comment := range doc.Comments {
		if !comment.Body.OK {
			continue
		}
		length := len([]rune(normalize(comment.Body.Value)))
		if length >= bodyFloor {
			continue
		}
		diagnostics = append(diagnostics, diagnostic("body-thin", comment, comment.Path+"/body",
			fmt.Sprintf("body is %d characters; state the finding and what follows from it", length)))
	}
	return diagnostics, nil
}

func vacuousBody(doc *review.Document) ([]review.Diagnostic, []review.Skipped) {
	diagnostics := []review.Diagnostic{}
	for _, comment := range doc.Comments {
		if !comment.Body.OK {
			continue
		}
		if !allClausesVacuous(comment.Body.Value) {
			continue
		}
		diagnostics = append(diagnostics, diagnostic("vacuous-body", comment, comment.Path+"/body",
			fmt.Sprintf("body (%q) says nothing a consumer can act on", clip(comment.Body.Value, 40))))
	}
	return diagnostics, nil
}

func suggestionAbsent(doc *review.Document) ([]review.Diagnostic, []review.Skipped) {
	diagnostics := []review.Diagnostic{}
	for _, comment := range doc.Comments {
		if !comment.Priority.OK || comment.Priority.Value < 7 || !comment.SuggestionsArray {
			continue
		}
		if len(comment.Suggestions) > 0 {
			continue
		}
		diagnostics = append(diagnostics, diagnostic("suggestion-absent", comment, comment.Path+"/suggestions",
			fmt.Sprintf("priority %d with no suggestions; propose a way out", comment.Priority.Value)))
	}
	return diagnostics, nil
}

func suggestionNoCons(doc *review.Document) ([]review.Diagnostic, []review.Skipped) {
	return emptyList(doc, "suggestion-no-cons", "cons", "state the tradeoff or say the fix is free")
}

func suggestionNoPros(doc *review.Document) ([]review.Diagnostic, []review.Skipped) {
	return emptyList(doc, "suggestion-no-pros", "pros", "say what this option buys")
}

func emptyList(doc *review.Document, name, field, advice string) ([]review.Diagnostic, []review.Skipped) {
	diagnostics := []review.Diagnostic{}
	for _, comment := range doc.Comments {
		for _, suggestion := range comment.Suggestions {
			list := suggestion.Cons
			if field == "pros" {
				list = suggestion.Pros
			}
			if !list.OK || len(list.Value) > 0 {
				continue
			}
			diagnostics = append(diagnostics, diagnostic(name, comment, suggestion.Path+"/"+field,
				fmt.Sprintf("suggestion %d (%q) lists no %s; %s", suggestion.Index+1, clip(suggestion.Summary.Value, 40), field, advice)))
		}
	}
	return diagnostics, nil
}

func broadScopeAlone(doc *review.Document) ([]review.Diagnostic, []review.Skipped) {
	diagnostics := []review.Diagnostic{}
	for _, comment := range doc.Comments {
		if len(comment.Suggestions) != 1 {
			continue
		}
		suggestion := comment.Suggestions[0]
		if !suggestion.Scope.OK || !broadScope(suggestion.Scope.Value) {
			continue
		}
		diagnostics = append(diagnostics, diagnostic("broad-scope-alone", comment, suggestion.Path+"/scope",
			fmt.Sprintf("the only suggestion is scope %s; offer a narrower alternative too", suggestion.Scope.Value)))
	}
	return diagnostics, nil
}

func broadScopeNoCons(doc *review.Document) ([]review.Diagnostic, []review.Skipped) {
	diagnostics := []review.Diagnostic{}
	for _, comment := range doc.Comments {
		for _, suggestion := range comment.Suggestions {
			if !suggestion.Scope.OK || !broadScope(suggestion.Scope.Value) {
				continue
			}
			if !suggestion.Cons.OK || len(suggestion.Cons.Value) > 0 {
				continue
			}
			diagnostics = append(diagnostics, diagnostic("broad-scope-no-cons", comment, suggestion.Path+"/cons",
				fmt.Sprintf("suggestion %d is scope %s with no cons; reaching that far always costs something",
					suggestion.Index+1, suggestion.Scope.Value)))
		}
	}
	return diagnostics, nil
}

func broadScope(scope string) bool {
	return scope == "module" || scope == "project"
}

func summaryThin(doc *review.Document) ([]review.Diagnostic, []review.Skipped) {
	if !doc.Summary.OK || len(doc.Comments) < 3 {
		return nil, nil
	}
	length := len([]rune(normalize(doc.Summary.Value)))
	if length >= bodyFloor {
		return nil, nil
	}
	return []review.Diagnostic{documentDiagnostic("summary-thin", "/summary",
		fmt.Sprintf("summary is %d characters with %s; expand it", length, plural(len(doc.Comments), "comment")))}, nil
}

// allClausesVacuous reports whether every sentence in a body is a stock phrase.
// Comparing the whole body against the list missed the shape that actually
// occurs -- two fillers joined to clear the schema's 20-character floor, as in
// "Looks good to me. LGTM." -- because no single phrase is that long. Judging
// any one sentence instead would flag "Looks good overall, but the retry loop
// drops the deadline", which is a real finding.
func allClausesVacuous(body string) bool {
	vacuous := false
	for _, clause := range strings.FieldsFunc(body, endsClause) {
		folded := fold(clause)
		if folded == "" {
			continue
		}
		if !isVacuousPhrase(folded) {
			return false
		}
		vacuous = true
	}
	return vacuous
}

func endsClause(r rune) bool {
	return r == '.' || r == '!' || r == '?' || r == ';' || r == '\n'
}

func isVacuousPhrase(folded string) bool {
	for _, phrase := range vacuousPhrases {
		if folded == phrase {
			return true
		}
	}
	return false
}
