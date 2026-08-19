// Package verify checks anchor claims against the git repository the review is
// about: does this path exist at the document ref, and does it have this many
// lines. It never reads what is on a line.
package verify

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/bobcob7/refinery/internal/review"
	"github.com/bobcob7/refinery/internal/structural"
)

// anchorChecks are the checks that need a resolved ref before they can run.
var anchorChecks = []string{"anchor-file-missing", "anchor-line-out-of-range"}

// Verifier resolves anchors against a repository.
type Verifier struct {
	git   gitRunner
	log   *slog.Logger
	files map[string]fileState
}

type fileState struct {
	kind  string
	lines int
}

// New returns a Verifier reading the supplied repository.
func New(git gitRunner, log *slog.Logger) *Verifier {
	return &Verifier{git: git, log: log, files: map[string]fileState{}}
}

// Verify resolves the document ref once, then checks every anchor it can read
// against it. An anchor whose own inputs are unusable is skipped individually
// and reported; nothing document-wide stops.
func (v *Verifier) Verify(ctx context.Context, doc *review.Document) ([]review.Diagnostic, []review.Skipped, review.Verification) {
	verification := review.Verification{Source: "repo", Anchors: doc.AnchorCount()}
	if reason, ok := v.refUsable(doc); !ok {
		if verification.Anchors == 0 {
			return nil, nil, verification
		}
		return nil, skips(reason), verification
	}
	ref := doc.Ref.Value
	if !v.refExists(ctx, ref) {
		diagnostic := review.Diagnostic{
			Severity: review.SeverityError,
			Name:     "ref-unknown",
			Path:     "/ref",
			Message:  fmt.Sprintf("ref %s does not resolve in this repository", short(ref)),
		}
		if verification.Anchors == 0 {
			return []review.Diagnostic{diagnostic}, nil, verification
		}
		return []review.Diagnostic{diagnostic}, skips("the document ref does not resolve"), verification
	}
	diagnostics := []review.Diagnostic{}
	unusable := 0
	for _, comment := range doc.Comments {
		for _, anchor := range comment.Anchors {
			if !anchor.Object {
				continue
			}
			if !anchor.File.OK {
				unusable++
				continue
			}
			diagnostic, ok := v.checkAnchor(ctx, comment, anchor, ref)
			if ok {
				verification.Verified++
				continue
			}
			diagnostics = append(diagnostics, diagnostic)
		}
	}
	skipped := []review.Skipped{}
	if unusable > 0 {
		skipped = skips(fmt.Sprintf("unusable file on %s", plural(unusable, "anchor")))
	}
	v.log.Debug("verification complete", "anchors", verification.Anchors, "verified", verification.Verified)
	return diagnostics, skipped, verification
}

// refUsable reports whether the document carries a ref worth looking up. A
// missing one is the advisory ref-missing's business and a malformed one is
// ref-format's; neither is repeated here.
func (v *Verifier) refUsable(doc *review.Document) (string, bool) {
	switch {
	case !doc.Ref.Present:
		return "the document has no ref", false
	case !doc.Ref.OK || !structural.ValidSHA(doc.Ref.Value):
		return "the document ref is not a commit SHA", false
	}
	return "", true
}

func (v *Verifier) checkAnchor(ctx context.Context, comment review.Comment, anchor review.Anchor, ref string) (review.Diagnostic, bool) {
	file := anchor.File.Value
	state := v.fileAt(ctx, ref, file)
	switch state.kind {
	case "":
		return review.Diagnostic{
			Severity: review.SeverityError,
			Name:     "anchor-file-missing",
			Comment:  commentID(comment),
			Path:     anchor.Path + "/file",
			Message:  fmt.Sprintf("%s does not exist at %s", file, short(ref)),
		}, false
	case "blob":
	default:
		return review.Diagnostic{
			Severity: review.SeverityError,
			Name:     "anchor-file-missing",
			Comment:  commentID(comment),
			Path:     anchor.Path + "/file",
			Message:  fmt.Sprintf("%s is a directory at %s, not a file", file, short(ref)),
		}, false
	}
	for _, candidate := range []struct {
		name  string
		field review.Field[int]
	}{{"line", anchor.Line}, {"end_line", anchor.EndLine}} {
		if !candidate.field.OK || candidate.field.Value <= state.lines {
			continue
		}
		return review.Diagnostic{
			Severity: review.SeverityError,
			Name:     "anchor-line-out-of-range",
			Comment:  commentID(comment),
			Path:     anchor.Path + "/" + candidate.name,
			Message: fmt.Sprintf("%s %d is out of range in a %d-line file at %s",
				candidate.name, candidate.field.Value, state.lines, short(ref)),
		}, false
	}
	return review.Diagnostic{}, true
}

func (v *Verifier) refExists(ctx context.Context, ref string) bool {
	out, err := v.git.run(ctx, "cat-file", "-t", ref)
	return err == nil && strings.TrimSpace(string(out)) == "commit"
}

// fileAt looks a path up at the ref, once per distinct path.
func (v *Verifier) fileAt(ctx context.Context, ref, file string) fileState {
	key := ref + ":" + file
	if state, cached := v.files[key]; cached {
		return state
	}
	state := fileState{}
	out, err := v.git.run(ctx, "cat-file", "-t", key)
	if err == nil {
		state.kind = strings.TrimSpace(string(out))
	}
	if state.kind == "blob" {
		if content, err := v.git.run(ctx, "cat-file", "blob", key); err == nil {
			state.lines = countLines(content)
		}
	}
	v.files[key] = state
	return state
}

// countLines counts lines the way an editor does: a trailing fragment with no
// newline is still a line, and an empty file has none.
func countLines(content []byte) int {
	if len(content) == 0 {
		return 0
	}
	lines := strings.Count(string(content), "\n")
	if !strings.HasSuffix(string(content), "\n") {
		lines++
	}
	return lines
}

// skips reports the anchor checks as skipped, so a caller can never read "no
// anchor errors" as "the anchors were checked".
func skips(reason string) []review.Skipped {
	skipped := make([]review.Skipped, 0, len(anchorChecks))
	for _, name := range anchorChecks {
		skipped = append(skipped, review.Skipped{Name: name, Reason: reason})
	}
	return skipped
}

// SkipAll reports every verification check as skipped, for a run with no
// repository to check against. A skipped check is never counted as a pass.
func SkipAll(reason string) []review.Skipped {
	names := append([]string{"ref-unknown"}, anchorChecks...)
	skipped := make([]review.Skipped, 0, len(names))
	for _, name := range names {
		skipped = append(skipped, review.Skipped{Name: name, Reason: reason})
	}
	return skipped
}

func commentID(comment review.Comment) string {
	if comment.ID.OK {
		return comment.ID.Value
	}
	return ""
}

func short(ref string) string {
	if len(ref) > 7 {
		return ref[:7]
	}
	return ref
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
