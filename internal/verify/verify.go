// Package verify checks anchor claims against the git repository the review is
// about: does this path exist at the document ref, and does it have this many
// lines. It never reads what is on a line.
package verify

import (
	"context"
	"fmt"
	"log/slog"
	"path"
	"strings"

	"github.com/bobcob7/loam-refinery/internal/review"
	"github.com/bobcob7/loam-refinery/internal/structural"
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
	// diverged means anchor-worktree-diverged fired: the anchored file exists
	// at ref, but the working-tree copy differs from it and ref is HEAD, so
	// the anchor is reported unverified rather than checked against either
	// copy.
	diverged
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
		verification = review.Verification{Source: "unavailable", Anchors: verification.Anchors, Reason: err.Error()}
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
	// HEAD is resolved once per run, the same way ref existence is: the
	// working tree only ever matters when ref is the checked-out commit, and
	// that is one fact about this run, not one fact per anchor.
	isHEAD := v.RefIsHEAD(ctx, ref)
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
			diagnostic, result := v.checkAnchor(ctx, comment, anchor, ref, isHEAD)
			switch result {
			case verified:
				verification.Verified++
			case unchecked:
				unreadable++
			case diverged:
				verification.Unverified = append(verification.Unverified, review.Unverified{
					Name:    diagnostic.Name,
					Comment: diagnostic.Comment,
					Path:    diagnostic.Path,
					Message: diagnostic.Message,
				})
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

// refUsable reports whether the document carries a ref worth looking up. The
// schema requires the field, so a document missing it already earns a schema
// diagnostic from the structural tier; since no tier gates another, this one
// may still see it and quietly skips rather than repeating that finding. A
// present but malformed ref is ref-format's business and is not repeated
// here either.
func (v *Verifier) refUsable(doc *review.Document) (string, bool) {
	switch {
	case !doc.Ref.Present:
		return "the document has no ref", false
	case !doc.Ref.OK || !structural.ValidSHA(doc.Ref.Value):
		return "the document ref is not a commit SHA", false
	}
	return "", true
}

func (v *Verifier) checkAnchor(ctx context.Context, comment review.Comment, anchor review.Anchor, ref string, isHEAD bool) (review.Diagnostic, outcome) {
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
			Comment:  comment.DiagnosticID(),
			Path:     anchor.Path + "/file",
			Message:  fmt.Sprintf("%s does not exist at %s", file, short(ref)),
		}, refuted
	case "blob":
	default:
		return review.Diagnostic{
			Severity: review.SeverityError,
			Name:     "anchor-file-missing",
			Comment:  comment.DiagnosticID(),
			Path:     anchor.Path + "/file",
			Message:  fmt.Sprintf("%s is a directory at %s, not a file", file, short(ref)),
		}, refuted
	}
	// The working tree is consulted only now that the path is confirmed
	// present at ref as a file: a path absent at ref stays anchor-file-missing
	// above, and the working tree never softens that. Case 2's remaining two
	// conditions — a working-tree copy exists, and it differs — are exactly
	// what worktreeDiverged answers; a copy that does not exist reports
	// "not diverged" on its own, which is what keeps a deleted working-tree
	// file falling through to the line checks below rather than landing here.
	if isHEAD {
		differs, err := v.git.worktreeDiverged(ctx, ref, file)
		if err != nil {
			v.log.Debug("working-tree comparison failed", "file", file, "ref", short(ref), "error", err)
			return review.Diagnostic{}, unchecked
		}
		if differs {
			return review.Diagnostic{
				Name:    "anchor-worktree-diverged",
				Comment: comment.DiagnosticID(),
				Path:    anchor.Path,
				Message: fmt.Sprintf("%s differs from %s in the working tree", file, short(ref)),
			}, diverged
		}
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
			Comment:  comment.DiagnosticID(),
			Path:     anchor.Path + "/" + candidate.name,
			Message: fmt.Sprintf("%s %d is out of range in a %d-line file at %s",
				candidate.name, candidate.field.Value, state.lines, short(ref)),
		}, refuted
	}
	return review.Diagnostic{}, verified
}

// RefIsHEAD reports whether ref names the checked-out commit, resolved once
// per run rather than once per anchor. A HEAD that cannot be resolved is not
// a reason to fail verification — it only means the working tree has nothing
// to say about ref, which is the same conclusion "ref is not HEAD" reaches,
// so anchors fall through to being checked normally rather than being
// silently skipped over a machine problem the caller never asked about.
//
// Exported for docs/features/combined-reviews.md §4.3.1's head_check: it is
// the one fact about a document's ref that Verify's own return value cannot
// answer on its own — Verification.Unverified is only ever populated when
// this already holds, so an empty Unverified cannot tell "ref is HEAD and
// nothing has drifted" from "ref is not HEAD" apart. A caller asking exactly
// that question calls this directly rather than re-deriving it: two
// implementations of "is ref HEAD" is one that will drift from the other.
func (v *Verifier) RefIsHEAD(ctx context.Context, ref string) bool {
	out, err := v.git.run(ctx, "rev-parse", "HEAD")
	if err != nil {
		v.log.Debug("HEAD lookup failed", "error", err)
		return false
	}
	return strings.TrimSpace(string(out)) == ref
}

// refExists asks whether the commit is present without asking git to read it.
// cat-file -e reports an absent object as exit 1, but it also exits 1 when it
// could not look — an unreadable alternates directory, a damaged pack index, a
// promisor clone that cannot reach its remote — so the status alone would fail a
// correct document over a bad disk. What separates them is that git complains
// first: absence is the silent answer. See objectAbsent.
func (v *Verifier) refExists(ctx context.Context, ref string) (bool, error) {
	if _, err := v.git.run(ctx, "cat-file", "-e", ref); err != nil {
		if objectAbsent(err) {
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

// objectAbsent reports git's one answer that means the object is not in this
// repository: exit 1 with nothing said. Every way git fails to look says so on
// stderr first, so silence carries the claim rather than the status, and a git
// that grows a new diagnostic is read as unable to look rather than as certain.
func objectAbsent(err error) bool {
	status, exited := exitStatus(err)
	return exited && status == 1 && !complained(stderrOf(err))
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
	// -z drops the c-style quoting ls-tree otherwise applies to unusual paths.
	// --literal-pathspecs stops an anchor being read as pathspec magic: without
	// it a file whose name begins with a colon is parsed as ":(glob)..." and
	// fails the whole call, which would be reported as an unreadable file.
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
//
// The names are compared cleaned, because git answers with the path it stores
// rather than the one it was handed: "./internal/x.go" and "internal//x.go"
// both come back as "internal/x.go", and refuting those would call a legal
// anchor missing over a spelling git itself does not keep. A ".." segment is
// not such a spelling — cleaning it away would confirm an anchor against a file
// it does not name — so those are refused outright, as anchor-path-safe already
// says of them.
func treeEntry(out, file string) (kind, object string, ok bool) {
	if traverses(file) {
		return "", "", false
	}
	entries := strings.Split(strings.TrimRight(out, "\x00"), "\x00")
	if len(entries) != 1 || entries[0] == "" {
		return "", "", false
	}
	meta, name, found := strings.Cut(entries[0], "\t")
	if !found || path.Clean(name) != path.Clean(file) {
		return "", "", false
	}
	fields := strings.Fields(meta)
	if len(fields) != 3 {
		return "", "", false
	}
	return fields[1], fields[2], true
}

// traverses reports whether any segment of the path is "..", in any spelling
// git would accept.
func traverses(file string) bool {
	for _, segment := range strings.Split(file, "/") {
		if segment == ".." {
			return true
		}
	}
	return false
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
