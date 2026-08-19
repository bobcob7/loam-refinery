package advisory

import (
	"fmt"

	"github.com/bobcob7/refinery/internal/review"
)

// softCategories conventionally stay below the blocking band.
var softCategories = map[string]bool{
	"testing": true, "maintainability": true, "documentation": true, "style": true,
}

const commentFloodLimit = 25

// calibration returns the advisories about how the scale was used.
func calibration() []Advisory {
	return []Advisory{
		{
			Meta: review.Check{
				Name:    "priority-category-convention",
				Tier:    review.TierAdvisory,
				Summary: "a testing, maintainability, documentation or style comment filed at 9 or 10",
				Title:   "Category above the blocking band",
				Body: `Fires when a testing, maintainability, documentation or style comment is filed
at priority 9 or 10 — the band that says the change must not merge as it stands.
Those four categories conventionally sit below it: a formatting complaint is not
a reason to stop a change, however strongly you feel about it.

The format encodes no ceiling per category; any such table would invent
precision nobody has. This is a note that a convention was crossed, not a
finding that you were wrong — undocumented behaviour in a public API genuinely
can be worth blocking on, and a reviewer who means it should say so in the body
and carry the advisory.

  before: category "documentation", priority 9, "godoc missing on Fetch"
  after:  priority 5 — or keep 9 and justify it in the body

There is no floor: a security finding at priority 2 is fine.`,
				Related: []string{"priority", "category"},
			},
			Run: priorityCategoryConvention,
		},
		{
			Meta: review.Check{
				Name:    "priority-flat",
				Tier:    review.TierAdvisory,
				Summary: "every comment in a review of four or more carries one priority",
				Title:   "Flat priority distribution",
				Body: `Fires when a review carries four or more comments and every one of them has the
same priority. A subset sharing a priority is ordinary and says nothing; it is
the absence of any spread across the whole review that suggests the scale was
not used: the field exists so a consumer can sort, threshold and merge
across reviewers, and a review where everything is a 7 gives it nothing to sort
by.

Spread the findings across the bands you actually mean. 9-10 must fix before
merge. 7-8 should fix, a real defect. 4-6 worth fixing, does not block. 1-3
optional polish the author may decline.

  before: 7, 7, 7, 7
  after:  9, 7, 5, 2

This is an aggregate check: it runs only when every comment has a usable
priority, because a confident claim computed over the subset that happened to
parse is worse than no claim. If one priority is ill-typed you will see this
reported as skipped rather than answered.`,
				Related: []string{"priority", "category"},
			},
			Run: priorityFlat,
		},
		{
			Meta: review.Check{
				Name:    "duplicate-anchor",
				Tier:    review.TierAdvisory,
				Summary: "two comments anchor the identical file, line and end_line",
				Title:   "Two comments on one span",
				Body: `Fires when two comments anchor the identical span — same file, line and
end_line. Usually it is one finding filed twice, which costs the consumer two
reads to discover they are the same thing.

If they really are two findings about one span, they should have different
anchors or be one comment carrying two suggestions. If it is one finding with
several fixes, that is exactly what the suggestions list is for.

  before: two comments, both anchored at client.go:88-94
  after:  one comment at client.go:88-94 with two suggestions

Two anchors on the same file at different lines are not duplicates, and neither
is the same finding filed once with several anchors — that is what the anchors
list is for.`,
				Related: []string{"anchors", "suggestions", "duplicate-body"},
			},
			Run: duplicateAnchor,
		},
		{
			Meta: review.Check{
				Name:    "duplicate-body",
				Tier:    review.TierAdvisory,
				Summary: "two comments have identical bodies after normalization",
				Title:   "Repeated comment body",
				Body: `Fires when two comments carry the same body once whitespace is normalized. The
same text filed twice is either a copy-paste that lost its second finding, or
one finding split across two ids for no reason a consumer can see.

If it is genuinely the same problem in several places, that is one comment with
several anchors — the anchors list exists for exactly this, and filing it once
keeps the finding and its suggestions together:

  before: two comments, identical bodies, one anchor each
  after:  one comment, two anchors

If the findings differ, say how in each body. A consumer reading them side by
side has nothing else to go on.`,
				Related: []string{"body", "anchors", "duplicate-anchor"},
			},
			Run: duplicateBody,
		},
		{
			Meta: review.Check{
				Name:    "comment-flood",
				Tier:    review.TierAdvisory,
				Summary: "more than 25 comments",
				Title:   "Too many comments",
				Body: `Fires when a review carries more than 25 comments. Feedback at that volume is
not actionable: nobody triages it in one pass, and the findings that matter are
buried among the ones that do not.

Two things usually fix it. Collapse repetition — the same unchecked error at
nine call sites is one comment with nine anchors, not nine comments. And drop
the 1-3 band entirely on a large change: optional polish is what a consumer
discards first anyway, and filing it costs the same attention as a defect.

If more than 25 distinct blocking findings really are present, the honest report
is usually a summary saying the change needs restructuring, with the strongest
handful anchored.

This is an aggregate check and runs only when every element of comments is an
object; otherwise it is reported as skipped.`,
				Related: []string{"comments", "duplicate-body"},
			},
			Run: commentFlood,
		},
	}
}

func priorityCategoryConvention(doc *review.Document) ([]review.Diagnostic, []review.Skipped) {
	diagnostics := []review.Diagnostic{}
	for _, comment := range doc.Comments {
		if !comment.Priority.OK || !comment.Category.OK || comment.Priority.Value < 9 {
			continue
		}
		if !softCategories[comment.Category.Value] {
			continue
		}
		diagnostics = append(diagnostics, diagnostic("priority-category-convention", comment, comment.Path+"/priority",
			fmt.Sprintf("%s at priority %d claims the change must not merge", comment.Category.Value, comment.Priority.Value)))
	}
	return diagnostics, nil
}

func priorityFlat(doc *review.Document) ([]review.Diagnostic, []review.Skipped) {
	usable, reason := prioritiesUsable(doc)
	if !usable {
		return nil, skip(reason, "priority-flat")
	}
	if len(doc.Comments) < 4 {
		return nil, nil
	}
	first := doc.Comments[0].Priority.Value
	for _, comment := range doc.Comments {
		if comment.Priority.Value != first {
			return nil, nil
		}
	}
	return []review.Diagnostic{documentDiagnostic("priority-flat", "",
		fmt.Sprintf("all %s are priority %d; the scale is not being used", plural(len(doc.Comments), "comment"), first))}, nil
}

func duplicateAnchor(doc *review.Document) ([]review.Diagnostic, []review.Skipped) {
	type placement struct {
		comment review.Comment
		span    string
	}
	seen := map[string]placement{}
	diagnostics := []review.Diagnostic{}
	for _, comment := range doc.Comments {
		for _, anchor := range comment.Anchors {
			if !anchor.File.OK || !anchor.Line.OK {
				continue
			}
			key := fmt.Sprintf("%s\x00%d\x00%d", anchor.File.Value, anchor.Line.Value, anchor.EndLine.Value)
			earlier, found := seen[key]
			if !found {
				seen[key] = placement{comment: comment, span: span(anchor)}
				continue
			}
			if earlier.comment.Index == comment.Index {
				continue
			}
			diagnostics = append(diagnostics, diagnostic("duplicate-anchor", comment, anchor.Path,
				fmt.Sprintf("anchors the same span as %s (%s)", commentLabel(earlier.comment), earlier.span)))
		}
	}
	return diagnostics, nil
}

func span(anchor review.Anchor) string {
	switch {
	case anchor.EndLine.OK:
		return fmt.Sprintf("%s:%d-%d", anchor.File.Value, anchor.Line.Value, anchor.EndLine.Value)
	case anchor.Line.OK:
		return fmt.Sprintf("%s:%d", anchor.File.Value, anchor.Line.Value)
	default:
		return anchor.File.Value
	}
}

func duplicateBody(doc *review.Document) ([]review.Diagnostic, []review.Skipped) {
	seen := map[string]review.Comment{}
	diagnostics := []review.Diagnostic{}
	for _, comment := range doc.Comments {
		if !comment.Body.OK {
			continue
		}
		key := fold(comment.Body.Value)
		earlier, found := seen[key]
		if !found {
			seen[key] = comment
			continue
		}
		diagnostics = append(diagnostics, diagnostic("duplicate-body", comment, comment.Path+"/body",
			fmt.Sprintf("body is identical to %s", commentLabel(earlier))))
	}
	return diagnostics, nil
}

func commentLabel(comment review.Comment) string {
	if comment.ID.OK {
		return comment.ID.Value
	}
	return fmt.Sprintf("comments[%d]", comment.Index)
}

func commentFlood(doc *review.Document) ([]review.Diagnostic, []review.Skipped) {
	if !doc.CommentsArray {
		return nil, skip("comments is not an array", "comment-flood")
	}
	if !doc.CommentsWellTyped {
		return nil, skip("some comments are not objects", "comment-flood")
	}
	if len(doc.Comments) <= commentFloodLimit {
		return nil, nil
	}
	return []review.Diagnostic{documentDiagnostic("comment-flood", "/comments",
		fmt.Sprintf("%d comments; feedback at this volume is not actionable", len(doc.Comments)))}, nil
}
