// Package collect assembles collect-reviews's merge semantics: N stored
// review documents for one repository and ref become one ordered set of
// Submission and Comment values, per docs/features/combined-reviews.md
// section 5 ("Merge semantics") and section 6.1 ("The qualified id").
//
// This package does not render JSON or Markdown — internal/render does
// that, from the Result Assemble produces — and it does not know about
// head_check, repo, or store.enabled, all of which the CLI-wiring bead
// assembles around this package's output. It also does not import
// internal/store: Assemble takes the ordered digest+at pairs
// internal/store.Store.DistinctDigests already computed, plus a small
// reader interface (interfaces.go) for the file content, so this
// package's own tests — the worked examples in
// docs/features/combined-reviews.md section 12 — run as plain Go values.
package collect

import (
	"context"
	"sort"
	"strconv"
	"time"

	"github.com/bobcob7/loam-refinery/internal/review"
)

// DigestRow is one distinct digest for a repo and ref, mirroring
// internal/store.DigestRow's shape without this package importing that
// one (see interfaces.go). At is the earliest recorded time among every
// run that shares the digest (combined-reviews.md section 5.3.1) — used
// only to order and to break supersession ties, never reported.
type DigestRow struct {
	Digest string
	At     time.Time
}

// Anchor is one location a comment applies to, carried through from the
// comment's own submission unchanged (review-document.md section 5).
// EndLine is nil when the comment did not set one.
type Anchor struct {
	File    string
	Line    int
	EndLine *int
}

// Suggestion is one proposed fix, carried through unchanged
// (review-document.md section 6).
type Suggestion struct {
	Summary string
	Effort  string
	Scope   string
	Pros    []string
	Cons    []string
	Code    string
}

// Comment is one surviving finding. Comments are never fused across
// profiles (section 5.2) — every comment that reaches this type is its
// own entry, verbatim from the submission that carried it, except for ID,
// which is qualified (section 6.1).
type Comment struct {
	// ID is the qualified id: "<profile>:<origin_id>" when the owning
	// submission is current for its profile, "#<ordinal>:<origin_id>"
	// otherwise (section 6.1). origin_id is recoverable by splitting on
	// the first colon; it is never carried as a separate field (section
	// 8.1).
	ID string
	// Profile answers who wrote this comment, independent of whether that
	// submission is still current — present exactly when the owning
	// submission claimed a profile, current or superseded (section 8.1).
	Profile     string
	Priority    int
	Category    string
	Body        string
	Code        string
	Anchors     []Anchor
	Suggestions []Suggestion
}

// Severity is one submission's severity shape: the highest priority among
// its comments, plus how many comments fall in each of the bands
// docs/review-document.md section 8 defines — the same bands
// priority-flat and priority-category-convention (internal/advisory)
// already reason about, reused here rather than inventing a third
// vocabulary for priority.
//
// Max is nil for a submission with no comments — an approve carrying
// none, the one case the schema permits (review-document.md section 3) —
// rather than 0, so "nothing filed" can never be read as "filed at
// priority 0". A *int costs an allocation Field[int] does not, but this
// type is not a parsed document field: it is computed, and Submission
// already uses a nil *int for the same "no such value" case
// (SupersededBy).
type Severity struct {
	// Max is the highest Priority among the submission's comments, nil
	// when it has none.
	Max *int
	// MustFix counts comments at priority 9-10: must fix before merge.
	MustFix int
	// ShouldFix counts comments at priority 7-8: should fix before merge,
	// a real defect.
	ShouldFix int
	// WorthFixing counts comments at priority 4-6: worth fixing, does not
	// block.
	WorthFixing int
	// Optional counts comments at priority 1-3: preference or polish.
	Optional int
}

// Submission is one distinct digest's contribution to the combined
// output (sections 5.3 and 8.1).
//
// Submission carries strictly more than what section 8.1's rendered shape
// encodes. The rendered shape has no room for a stored review's own
// parsed form, but refinery-uyb.10's head_check needs two things this
// type alone can supply: Document, to run anchor verification against,
// and QualifiedIDs, to translate a per-anchor verification result (keyed
// by the submission's own origin comment ids) into the same qualified-id
// space this package's Comments use. Do not "simplify" this type down to
// only the fields section 8.1 encodes — a later bead depends on the rest.
type Submission struct {
	// Ordinal is this submission's 1-based position in the ordering
	// section 8.1 defines: submissions sharing a profile cluster
	// together, alphabetically by profile, oldest internally-first,
	// followed by every unprofiled submission, also oldest-first.
	Ordinal int
	// Profile is "" when this submission claimed none.
	Profile string
	Verdict string
	Summary string
	// SupersededBy names the Ordinal of the current submission for this
	// submission's profile, nil when this submission is current (or
	// unprofiled, which has no supersession axis at all).
	SupersededBy *int
	// Document is the full parsed review this submission came from —
	// carried for refinery-uyb.10's head_check, not part of section
	// 8.1's rendered shape.
	Document *review.Document
	// QualifiedIDs maps each of Document's own comment ids (origin ids)
	// to the qualified id this package assigned it among Result.Comments
	// — also carried for refinery-uyb.10, not rendered directly.
	QualifiedIDs map[string]string
	// Severity is this submission's severity shape, computed from
	// Document.Comments at assembly time — not reconstructed downstream
	// from qualified ids, which section 6.1 owns the formatting of.
	Severity Severity
}

// Result is everything Assemble determined for one repo and ref: the
// ordered Submission and Comment values both the JSON and Markdown
// renderers project from, plus how many distinct digests could not be
// read at all.
type Result struct {
	Submissions []Submission
	Comments    []Comment
	// Unreadable counts distinct digests whose file could not be read, or
	// whose content did not parse as a single JSON object — skipped, not
	// fatal (combined-reviews.md section 9).
	Unreadable int
}

// parsed is one successfully read and parsed digest, still in assembly
// order (oldest at first, ties broken on digest), before profile
// grouping.
type parsed struct {
	digest  string
	at      time.Time
	profile string
	doc     *review.Document
}

// Assemble reads every digest in digests through r, parses each with
// review.Parse, and produces the ordered, qualified Result
// docs/features/combined-reviews.md sections 5, 6.1, and 8.1 describe. It
// returns an error only if ctx is done; a digest that fails to read or
// fails to parse is skipped and counted in Result.Unreadable instead
// (section 9).
func Assemble(ctx context.Context, digests []DigestRow, r reader) (*Result, error) {
	ordered := sortedDigests(digests)
	items := make([]parsed, 0, len(ordered))
	unreadable := 0
	for _, row := range ordered {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		data, err := r.ReadReview(ctx, row.Digest)
		if err != nil {
			unreadable++
			continue
		}
		doc, err := review.Parse(data)
		if err != nil {
			unreadable++
			continue
		}
		items = append(items, parsed{digest: row.Digest, at: row.At, profile: claimedProfile(doc), doc: doc})
	}
	submissions := buildSubmissions(items)
	comments := buildComments(submissions)
	return &Result{Submissions: submissions, Comments: comments, Unreadable: unreadable}, nil
}

// sortedDigests returns digests ordered oldest-at first, ties broken on
// digest string comparison (section 5.3.3) — arbitrary but deterministic,
// so repeated calls against an unchanged store return the same order.
func sortedDigests(digests []DigestRow) []DigestRow {
	ordered := make([]DigestRow, len(digests))
	copy(ordered, digests)
	sort.Slice(ordered, func(i, j int) bool {
		if !ordered[i].At.Equal(ordered[j].At) {
			return ordered[i].At.Before(ordered[j].At)
		}
		return ordered[i].Digest < ordered[j].Digest
	})
	return ordered
}

// claimedProfile reads a document's own claimed profile, "" when the
// field was absent or not a string — the same absent-means-unclaimed
// reading every other optional string field in review.Document gets.
func claimedProfile(doc *review.Document) string {
	if doc.Profile.Present && doc.Profile.OK {
		return doc.Profile.Value
	}
	return ""
}

// buildSubmissions groups items by claimed profile (section 5.2: never by
// slug, never fused), orders the groups alphabetically by profile name
// followed by every unprofiled item, assigns ordinals by that final
// position (section 8.1), and marks every submission but the last in each
// profile group as superseded by that last one (section 5.3.2). items is
// assumed already ordered oldest-first; that order survives grouping
// because both bucketing and the final concatenation are stable.
func buildSubmissions(items []parsed) []Submission {
	profiled := map[string][]parsed{}
	var unprofiled []parsed
	var names []string
	for _, item := range items {
		if item.profile == "" {
			unprofiled = append(unprofiled, item)
			continue
		}
		if _, seen := profiled[item.profile]; !seen {
			names = append(names, item.profile)
		}
		profiled[item.profile] = append(profiled[item.profile], item)
	}
	sort.Strings(names)
	submissions := make([]Submission, 0, len(items))
	for _, name := range names {
		group := profiled[name]
		start := len(submissions)
		for _, item := range group {
			submissions = append(submissions, submissionOf(item, len(submissions)+1))
		}
		current := submissions[len(submissions)-1].Ordinal
		for i := start; i < len(submissions)-1; i++ {
			superseded := current
			submissions[i].SupersededBy = &superseded
		}
	}
	for _, item := range unprofiled {
		submissions = append(submissions, submissionOf(item, len(submissions)+1))
	}
	return submissions
}

// submissionOf builds one Submission from a parsed item and its assigned
// ordinal. SupersededBy starts nil; buildSubmissions fills it in
// afterward, once a whole profile group's last (current) member is
// known. Comment qualified ids are filled in by buildComments once every
// submission's ordinal and currency is known.
func submissionOf(item parsed, ordinal int) Submission {
	return Submission{
		Ordinal:      ordinal,
		Profile:      item.profile,
		Verdict:      item.doc.Verdict.Value,
		Summary:      item.doc.Summary.Value,
		Document:     item.doc,
		QualifiedIDs: map[string]string{},
		Severity:     severityOf(item.doc),
	}
}

// severityOf computes one submission's Severity from its document's own
// comments, before qualified ids exist — the qualified id is a rendering
// concern (section 6.1) this computation never needs. A document with no
// comments leaves Max nil, the absent case Severity documents.
//
// A comment whose priority did not parse as an integer (Field.OK false —
// absent, or the wrong JSON type) is excluded entirely, matching
// review-document.md section 11.4's table: "wrong type... skipped by
// priority-based checks", the same rule priority-category-convention
// applies (internal/advisory). A submission whose comments are all
// unusable this way ends up indistinguishable from one with none, which
// is the correct read: no usable severity data exists either way.
func severityOf(doc *review.Document) Severity {
	var sev Severity
	for _, c := range doc.Comments {
		if !c.Priority.OK {
			continue
		}
		priority := c.Priority.Value
		if sev.Max == nil || priority > *sev.Max {
			max := priority
			sev.Max = &max
		}
		bandOf(priority, &sev)
	}
	return sev
}

// bandOf increments the Severity band priority falls in, per the scale
// docs/review-document.md section 8 defines: 9-10 must fix before merge,
// 7-8 should fix before merge, 4-6 worth fixing, 1-3 optional. Callers
// only ever pass a priority whose Field was OK — see severityOf — so
// what reaches here is always a real integer, but not necessarily one
// review-document.md section 3's schema would accept: table §11.4 says a
// well-typed, out-of-range priority such as 12 still runs through
// priority-based reasoning rather than being discarded, so 12 counts as
// MustFix (>= 9) and a sub-1 value, which the schema also forbids but a
// lenient parse does not, falls to Optional rather than being dropped
// uncounted.
func bandOf(priority int, sev *Severity) {
	switch {
	case priority >= 9:
		sev.MustFix++
	case priority >= 7:
		sev.ShouldFix++
	case priority >= 4:
		sev.WorthFixing++
	default:
		sev.Optional++
	}
}

// buildComments assigns every submission's comments a qualified id
// (section 6.1), fills in each Submission's QualifiedIDs map as it goes,
// and returns the flat comment list ordered by id, lexicographically
// (section 8.1).
func buildComments(submissions []Submission) []Comment {
	var comments []Comment
	for i := range submissions {
		sub := &submissions[i]
		qualifier := qualifierFor(sub)
		for _, c := range sub.Document.Comments {
			origin := c.ID.Value
			id := qualifier + origin
			sub.QualifiedIDs[origin] = id
			comments = append(comments, Comment{
				ID:          id,
				Profile:     sub.Profile,
				Priority:    c.Priority.Value,
				Category:    c.Category.Value,
				Body:        c.Body.Value,
				Code:        c.Code.Value,
				Anchors:     convertAnchors(c.Anchors),
				Suggestions: convertSuggestions(c.Suggestions),
			})
		}
	}
	sort.Slice(comments, func(i, j int) bool { return comments[i].ID < comments[j].ID })
	return comments
}

// qualifierFor returns the "<profile>:" or "#<ordinal>:" prefix section
// 6.1 defines for one submission's comments: profile-qualified exactly
// when it claimed a profile and is current (SupersededBy nil),
// ordinal-qualified otherwise. Building the profile form as
// profile+":"+origin_id relies on an invariant this package cannot check
// itself: profile-format (docs/review-document.md §11.1) rejects any
// profile containing a colon before submit-review ever writes it to the
// store, which is what keeps the two qualifier forms from ever colliding
// with each other or with an origin id. A profile that reached this code
// with a colon in it would already be a bug upstream of this package.
func qualifierFor(sub *Submission) string {
	if sub.Profile != "" && sub.SupersededBy == nil {
		return sub.Profile + ":"
	}
	return "#" + strconv.Itoa(sub.Ordinal) + ":"
}

// convertAnchors carries a comment's anchors through unchanged, dropping
// only the validation scaffolding (Field presence/OK) that a stored
// review has already passed by the time collect-reviews reads it
// (docs/cli.md §2.3.1: nothing enters the store unverified).
func convertAnchors(anchors []review.Anchor) []Anchor {
	out := make([]Anchor, 0, len(anchors))
	for _, a := range anchors {
		converted := Anchor{File: a.File.Value, Line: a.Line.Value}
		if a.EndLine.Present && a.EndLine.OK {
			end := a.EndLine.Value
			converted.EndLine = &end
		}
		out = append(out, converted)
	}
	return out
}

// convertSuggestions carries a comment's suggestions through unchanged,
// for the same reason convertAnchors does.
func convertSuggestions(suggestions []review.Suggestion) []Suggestion {
	out := make([]Suggestion, 0, len(suggestions))
	for _, s := range suggestions {
		out = append(out, Suggestion{
			Summary: s.Summary.Value,
			Effort:  s.Effort.Value,
			Scope:   s.Scope.Value,
			Pros:    s.Pros.Value,
			Cons:    s.Cons.Value,
			Code:    s.Code.Value,
		})
	}
	return out
}
