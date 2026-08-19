package advisory

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/bobcob7/refinery/internal/review"
)

var idPattern = regexp.MustCompile(`^(.*)-([1-9][0-9]*)$`)

// consistency returns the advisories about a document agreeing with itself.
func consistency() []Advisory {
	return []Advisory{
		{
			Meta: review.Check{
				Name:    "id-grouping",
				Tier:    review.TierAdvisory,
				Summary: "suffixes within a slug do not run contiguously from 1",
				Title:   "Non-contiguous id suffixes",
				Body: `Fires when the suffixes on a slug do not run 1, 2, 3 — a gap (foo-1, foo-3), a
start above 1, or a slug used once as foo-2. Nothing breaks, but the gap reads
as a missing finding: a consumer collapsing a slug into one theme cannot tell
whether foo-2 was dropped on purpose or lost in an edit.

The slug is the grouping mechanism and the suffix is the address, so number
within a slug from 1 and stay contiguous. Do not renumber to dodge a collision
between different kinds of finding — give those different slugs.

  before: "missing-context-1", "missing-context-3"
  after:  "missing-context-1", "missing-context-2"`,
				Related: []string{"id", "id-unique"},
			},
			Run: idGrouping,
		},
		{
			Meta: review.Check{
				Name:    "ref-missing",
				Tier:    review.TierAdvisory,
				Summary: "an anchor carries a line but the document has no ref",
				Title:   "Line number with no ref",
				Body: `Fires when an anchor carries a line number and the document supplies no ref. A
line number without a commit is a claim about a moment nobody recorded: the line
it meant is somewhere else the instant anything moves, and no reader — a person,
a later agent, a CI job — can ever check it.

The fix is one field at the document root, inherited by every anchor:

  "ref": "4f2c1a9e8b3d7c5a1f0e2d4b6a8c9e1f3a5b7c9d"

git rev-parse HEAD produces it, and you are holding a checkout already. It is an
advisory rather than an error because not every review is of a git repository;
if yours is not, drop the line numbers and anchor at file level instead of
recording numbers nobody can resolve.`,
				Related: []string{"ref", "line", "ref-format"},
			},
			Run: refMissing,
		},
	}
}

func idGrouping(doc *review.Document) ([]review.Diagnostic, []review.Skipped) {
	type slugState struct {
		suffixes []int
		comments map[int]review.Comment
		order    int
	}
	slugs := map[string]*slugState{}
	order := []string{}
	for _, comment := range doc.Comments {
		if !comment.ID.OK {
			continue
		}
		match := idPattern.FindStringSubmatch(comment.ID.Value)
		if match == nil {
			continue
		}
		slug := match[1]
		suffix, err := strconv.Atoi(match[2])
		if err != nil {
			continue
		}
		state, seen := slugs[slug]
		if !seen {
			state = &slugState{comments: map[int]review.Comment{}}
			slugs[slug] = state
			order = append(order, slug)
		}
		if _, duplicate := state.comments[suffix]; duplicate {
			continue
		}
		state.suffixes = append(state.suffixes, suffix)
		state.comments[suffix] = comment
	}
	diagnostics := []review.Diagnostic{}
	for _, slug := range order {
		state := slugs[slug]
		sort.Ints(state.suffixes)
		offender, ok := firstGap(state.suffixes)
		if ok {
			continue
		}
		comment := state.comments[offender]
		diagnostics = append(diagnostics, diagnostic("id-grouping", comment, comment.Path+"/id",
			fmt.Sprintf("slug %q has suffixes %s; renumber contiguously", slug, join(state.suffixes))))
	}
	return diagnostics, nil
}

// firstGap returns the suffix that broke contiguity, and whether the run is sound.
func firstGap(suffixes []int) (int, bool) {
	for i, suffix := range suffixes {
		if suffix != i+1 {
			return suffix, false
		}
	}
	return 0, true
}

func join(values []int) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.Itoa(value))
	}
	return strings.Join(parts, ", ")
}

func refMissing(doc *review.Document) ([]review.Diagnostic, []review.Skipped) {
	if doc.Ref.Present {
		return nil, nil
	}
	anchored := 0
	for _, comment := range doc.Comments {
		for _, anchor := range comment.Anchors {
			if anchor.Line.OK {
				anchored++
			}
		}
	}
	if anchored == 0 {
		return nil, nil
	}
	carry, them := "carries", "it"
	if anchored > 1 {
		carry, them = "carry", "them"
	}
	return []review.Diagnostic{documentDiagnostic("ref-missing", "/ref",
		fmt.Sprintf("%s %s a line number and the document has no ref; nobody can verify %s",
			plural(anchored, "anchor"), carry, them))}, nil
}
