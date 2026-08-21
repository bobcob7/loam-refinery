// Package validate runs every check tier over one document and assembles the
// result. No tier gates another: a schema failure does not stop verification,
// and a verification failure does not stop the advisories.
package validate

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/bobcob7/loam-refinery/internal/review"
	"github.com/bobcob7/loam-refinery/internal/verify"
)

// Options are the per-run choices a caller makes with flags.
type Options struct {
	// Strict promotes advisories to errors for the exit code.
	Strict bool
	// Dir is where repository discovery starts.
	Dir string
}

// Validator assembles one result from the three check tiers.
type Validator struct {
	structural structuralChecker
	advisories advisoryRunner
	finder     repositoryFinder
	log        *slog.Logger
}

// New wires the tiers.
func New(structural structuralChecker, advisories advisoryRunner, finder repositoryFinder, log *slog.Logger) *Validator {
	return &Validator{structural: structural, advisories: advisories, finder: finder, log: log}
}

// Validate parses and checks one document. It returns an error only when the
// input is not a single JSON object, which is the one true stop.
//
// Verification runs before the structural tier now, not after it, because
// the precondition it can report (docs/cli.md §2.3.1) has to be checked
// before structural checks or advisories run at all: when ref names HEAD and
// an anchored file has diverged in the working tree, nothing else about the
// document is examined, and Result.Precondition is how the caller learns to
// exit 3 rather than 1. Ordinary verification results are unaffected by the
// reorder — no tier gates another either way — and still land after the
// structural diagnostics once the precondition does not fire.
func (v *Validator) Validate(ctx context.Context, source []byte, options Options) (*review.Result, error) {
	doc, err := review.Parse(source)
	if err != nil {
		return nil, fmt.Errorf("reading review document: %w", err)
	}
	verified, skipped, verification := v.verify(ctx, doc, options)
	if diagnostic, ok := worktreeDivergedDiagnostic(verification); ok {
		return &review.Result{
			Strict:       options.Strict,
			Comments:     len(doc.Comments),
			Verification: review.Verification{Source: verification.Source, Anchors: verification.Anchors},
			Diagnostics:  []review.Diagnostic{diagnostic},
			Precondition: true,
		}, nil
	}
	result := &review.Result{Strict: options.Strict, Comments: len(doc.Comments)}
	result.Diagnostics = append(result.Diagnostics, v.structural.Check(doc)...)
	verified = append(verified, unverified(verification, skipped)...)
	result.Diagnostics = append(result.Diagnostics, verified...)
	result.Skipped = append(result.Skipped, skipped...)
	result.Verification = verification
	advisories, unrun := v.advisories.Run(doc)
	result.Diagnostics = append(result.Diagnostics, advisories...)
	result.Skipped = append(result.Skipped, unrun...)
	review.SortDiagnostics(result.Diagnostics)
	result.Valid = result.Errors() == 0 && (!options.Strict || result.Advisories() == 0)
	return result, nil
}

// worktreeDivergedRemedy is appended to the first diverged anchor's own
// message to build the precondition's diagnostic (docs/cli.md §2.3.1): one
// diagnostic for the whole document, naming both remedies and saying
// plainly that this is not a check to retry against the same ref.
const worktreeDivergedRemedy = "; the reviewed state is not a commit. " +
	`Commit what was reviewed, or run "git stash create" and resubmit against that SHA — do not retry against this ref.`

// worktreeDivergedDiagnostic reports the precondition (docs/cli.md §2.3.1):
// verify's own pass already found every diverged anchor, in
// verification.Unverified, and reports it here only when Source is "repo" —
// Unverified is populated only when ref is HEAD (internal/verify/verify.go),
// so this never fires for any other ref, with no extra check needed. Several
// anchors, or several files, can have diverged; only the first is named,
// because the diagnostic is one per document, not one per anchor.
func worktreeDivergedDiagnostic(verification review.Verification) (review.Diagnostic, bool) {
	if verification.Source != "repo" || len(verification.Unverified) == 0 {
		return review.Diagnostic{}, false
	}
	return review.Diagnostic{
		Severity: review.SeverityError,
		Name:     "anchor-worktree-diverged",
		Message:  verification.Unverified[0].Message + worktreeDivergedRemedy,
	}, true
}

// verify has three outcomes, not two, and none of them stops the run: a
// document is still a document when there is nothing to check it against, and
// discarding the structural and advisory findings already computed would blame
// the machine's failure on the review by saying nothing about it at all.
//
// The three are told apart by Source rather than by exit code. Standing outside
// a repository is ordinary and reports "none"; a repository that could not be
// reached is "unavailable", which carries git's own words, so a caller can tell
// "there was nothing to check against" from "the check never happened".
func (v *Validator) verify(ctx context.Context, doc *review.Document, options Options) ([]review.Diagnostic, []review.Skipped, review.Verification) {
	repository, err := v.finder.Find(ctx, options.Dir)
	switch {
	case errors.Is(err, verify.ErrNoRepository):
		v.log.Debug("verification skipped", "reason", err)
		return nil, verify.SkipAll(err.Error()), review.Verification{Source: "none", Reason: err.Error(), Anchors: doc.AnchorCount()}
	case err != nil:
		v.log.Debug("verification unavailable", "reason", err)
		return nil, verify.SkipAll(err.Error()), review.Verification{Source: "unavailable", Reason: err.Error(), Anchors: doc.AnchorCount()}
	}
	return repository.Verify(ctx, doc)
}

// unverified reports the one thing verification always asks now: whether the
// anchor claims were actually checked. A repository that did not answer or
// an anchor whose file could not be read are the same answer to that
// question — no one confirmed this — however different their causes, and
// there is no flag to accept the gap instead. A diverged working tree no
// longer reaches here at all: worktreeDivergedDiagnostic already stopped the
// run on it, in Validate, before this ever runs — verification.Unverified is
// therefore always empty by the time this is called.
func unverified(verification review.Verification, skipped []review.Skipped) []review.Diagnostic {
	if verification.Anchors == 0 {
		return nil
	}
	if verification.Source == "repo" && len(skipped) == 0 {
		return nil
	}
	reason := verification.Reason
	if reason == "" {
		reason = strings.Join(reasons(skipped), "; ")
	}
	if reason == "" {
		reason = "no repository answered"
	}
	return []review.Diagnostic{{
		Severity: review.SeverityError,
		Name:     "verification-required",
		Message:  fmt.Sprintf("the anchors were not verified: %s", reason),
	}}
}

// reasons lists the distinct causes, in the order verification reported them,
// so a document-side skip cannot hide a machine-side one behind it.
func reasons(skipped []review.Skipped) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, skip := range skipped {
		if skip.Reason == "" || seen[skip.Reason] {
			continue
		}
		seen[skip.Reason] = true
		out = append(out, skip.Reason)
	}
	return out
}
