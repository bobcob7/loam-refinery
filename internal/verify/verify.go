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
	// err is set when git failed to answer at all, which is a fact about this
	// machine rather than about the review, and must not become a diagnostic.
	err error
}

// outcome is what checking one anchor established.
type outcome int

const (
	verified outcome = iota
	refuted
	unchecked
)

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
	exists, err := v.refExists(ctx, ref)
	if err != nil {
		v.log.Debug("ref lookup failed", "ref", short(ref), "error", err)
		verification = review.Verification{Source: "none", Anchors: verification.Anchors, Reason: err.Error()}
		if verification.Anchors == 0 {
			return nil, nil, verification
		}
		return nil, skips("git could not resolve the document ref"), verification
	}
	if !exists {
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
	unusable, unreadable := 0, 0
	for _, comment := range doc.Comments {
		for _, anchor := range comment.Anchors {
			if !anchor.Object {
				continue
			}
			if !anchorUsable(anchor) {
				unusable++
				continue
			}
			diagnostic, result := v.checkAnchor(ctx, comment, anchor, ref)
			switch result {
			case verified:
				verification.Verified++
			case unchecked:
				unreadable++
			default:
				diagnostics = append(diagnostics, diagnostic)
			}
		}
	}
	skipped := []review.Skipped{}
	if unusable > 0 {
		skipped = append(skipped, skips(fmt.Sprintf("unusable field on %s", plural(unusable, "anchor")))...)
	}
	if unreadable > 0 {
		skipped = append(skipped, skips(fmt.Sprintf("git could not read the file for %s", plural(unreadable, "anchor")))...)
	}
	v.log.Debug("verification complete", "anchors", verification.Anchors, "verified", verification.Verified)
	return diagnostics, skipped, verification
}

// anchorUsable reports whether every field this tier reads is readable. A line
// that is present but ill-typed cannot be range checked, and counting such an
// anchor as verified would claim the opposite of what was established.
func anchorUsable(anchor review.Anchor) bool {
	switch {
	case !anchor.File.OK:
		return false
	case anchor.Line.Present && !anchor.Line.OK:
		return false
	case anchor.EndLine.Present && !anchor.EndLine.OK:
		return false
	}
	return true
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

func (v *Verifier) checkAnchor(ctx context.Context, comment review.Comment, anchor review.Anchor, ref string) (review.Diagnostic, outcome) {
	file := anchor.File.Value
	state := v.fileAt(ctx, ref, file)
	if state.err != nil {
		return review.Diagnostic{}, unchecked
	}
	switch state.kind {
	case "":
		return review.Diagnostic{
			Severity: review.SeverityError,
			Name:     "anchor-file-missing",
			Comment:  commentID(comment),
			Path:     anchor.Path + "/file",
			Message:  fmt.Sprintf("%s does not exist at %s", file, short(ref)),
		}, refuted
	case "blob":
	default:
		return review.Diagnostic{
			Severity: review.SeverityError,
			Name:     "anchor-file-missing",
			Comment:  commentID(comment),
			Path:     anchor.Path + "/file",
			Message:  fmt.Sprintf("%s is a directory at %s, not a file", file, short(ref)),
		}, refuted
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
		}, refuted
	}
	return review.Diagnostic{}, verified
}

// refExists asks whether the commit is present without asking git to read it.
// cat-file -e answers that with an exit status rather than a sentence: 1 means
// the object is not there, and every other failure means git could not look,
// which says nothing about the review. Reading a corrupt object store as an
// absent commit would fail a correct document over a bad disk.
func (v *Verifier) refExists(ctx context.Context, ref string) (bool, error) {
	if _, err := v.git.run(ctx, "cat-file", "-e", ref); err != nil {
		if status, exited := exitStatus(err); exited && status == 1 {
			return false, nil
		}
		return false, err
	}
	out, err := v.git.run(ctx, "cat-file", "-t", ref)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) == "commit", nil
}

// fileAt looks a path up at the ref, once per distinct path.
func (v *Verifier) fileAt(ctx context.Context, ref, file string) fileState {
	key := ref + ":" + file
	if state, cached := v.files[key]; cached {
		return state
	}
	state := v.lookUp(ctx, ref, file)
	if state.err != nil {
		v.log.Debug("git could not read a file", "file", file, "ref", short(ref), "error", state.err)
	}
	v.files[key] = state
	return state
}

// lookUp reads one path out of the tree. ls-tree is used rather than cat-file
// because it distinguishes the two answers by shape instead of by prose: a path
// that is not in the tree is an empty listing and a zero exit, while a tree git
// cannot read is a non-zero exit. cat-file conflates them, reporting both as
// exit 128 and leaving only an English sentence to tell a missing file from an
// unreadable one — so a corrupt object turned a correct anchor into a finding.
func (v *Verifier) lookUp(ctx context.Context, ref, file string) fileState {
	// -z drops the c-style quoting ls-tree otherwise applies to unusual paths,
	// and --literal-pathspecs stops a path containing a wildcard from matching
	// entries the anchor never named.
	out, err := v.git.run(ctx, "--literal-pathspecs", "ls-tree", "-z", ref, "--", file)
	if err != nil {
		return fileState{err: err}
	}
	kind, object, ok := treeEntry(string(out), file)
	if !ok {
		return fileState{}
	}
	if kind != "blob" {
		return fileState{kind: kind}
	}
	content, err := v.git.run(ctx, "cat-file", "blob", object)
	if err != nil {
		return fileState{err: err}
	}
	return fileState{kind: kind, lines: countLines(content)}
}

// treeEntry parses the single entry ls-tree -z prints for a path:
// "<mode> <type> <object>\t<path>". Anything else — no entry, several entries,
// or a name that is not the one asked for — is reported as not found, which
// skips the anchor rather than inventing a claim about it.
func treeEntry(out, file string) (kind, object string, ok bool) {
	entries := strings.Split(strings.TrimRight(out, "\x00"), "\x00")
	if len(entries) != 1 || entries[0] == "" {
		return "", "", false
	}
	meta, name, found := strings.Cut(entries[0], "\t")
	if !found || name != file {
		return "", "", false
	}
	fields := strings.Fields(meta)
	if len(fields) != 3 {
		return "", "", false
	}
	return fields[1], fields[2], true
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
