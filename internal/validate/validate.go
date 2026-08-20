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
	// Disabled advisory names are not run.
	Disabled map[string]bool
	// WarnOnly verification check names are demoted to advisories.
	WarnOnly map[string]bool
	// Dir is where repository discovery starts.
	Dir string
	// RequireVerification fails a run whose anchors were not actually checked.
	// Off by default: a document is not wrong because no repository answered,
	// and only the caller knows whether it needed one to.
	RequireVerification bool
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
func (v *Validator) Validate(ctx context.Context, source []byte, options Options) (*review.Result, error) {
	doc, err := review.Parse(source)
	if err != nil {
		return nil, fmt.Errorf("reading review document: %w", err)
	}
	result := &review.Result{Strict: options.Strict, Comments: len(doc.Comments)}
	result.Diagnostics = append(result.Diagnostics, v.structural.Check(doc)...)
	verified, skipped, verification := v.verify(ctx, doc, options)
	if options.RequireVerification {
		verified = append(verified, unverified(verification, skipped, options.WarnOnly)...)
	}
	result.Diagnostics = append(result.Diagnostics, demote(verified, options.WarnOnly)...)
	result.Skipped = append(result.Skipped, skipped...)
	result.Verification = verification
	advisories, unrun := v.advisories.Run(doc, options.Disabled)
	result.Diagnostics = append(result.Diagnostics, advisories...)
	result.Skipped = append(result.Skipped, unrun...)
	review.SortDiagnostics(result.Diagnostics)
	result.Valid = result.Errors() == 0 && (!options.Strict || result.Advisories() == 0)
	return result, nil
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

// unverified reports the one thing --require-verification asks about: whether
// the anchor claims were actually checked. A repository that did not answer,
// an anchor whose file could not be read, and an anchor whose working-tree
// copy diverged from ref are the same answer to that question — no one
// confirmed this — however different their causes.
//
// It is produced before demote, so --warn-only=verification-required demotes it
// like any other verification check. Asking for both is contradictory, but it is
// contradictory on the command line where a reader can see it, which beats a
// flag that silently outranks another.
func unverified(verification review.Verification, skipped []review.Skipped, warnOnly map[string]bool) []review.Diagnostic {
	if verification.Anchors == 0 {
		return nil
	}
	skipGap := verification.Source != "repo" || len(skipped) > 0
	divergedGap := len(verification.Unverified) > 0
	if !skipGap && !divergedGap {
		return nil
	}
	// Each gap is excused on its own terms: --warn-only=anchor-worktree-diverged
	// excuses only a diverged working tree, the same way --warn-only=ref-unknown
	// excuses only a repository lacking the reviewed commit, and demoting one
	// never waves through the other.
	if (!skipGap || excused(skipped, warnOnly)) && (!divergedGap || warnOnly["anchor-worktree-diverged"]) {
		return nil
	}
	reason := verification.Reason
	if reason == "" {
		parts := reasons(skipped)
		if divergedGap {
			parts = append(parts, fmt.Sprintf("%s diverged from ref in the working tree", plural(len(verification.Unverified), "anchor")))
		}
		reason = strings.Join(parts, "; ")
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

// plural matches the wording verify already uses for the same counts.
func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// excused reports whether the caller already accepted the condition that
// stopped verification. --warn-only=ref-unknown says a repository legitimately
// lacking the reviewed commit is not a failure; the anchor checks it skips are
// that same fact reported again, and failing on them would make the two flags
// impossible to combine — the caller could never say "verify these, but I know
// this commit is gone".
//
// Each skip says for itself which check would excuse it, so demoting one check
// never covers a gap some other condition caused: no repository at all, a file
// git could not read, and a field too malformed to check name nothing, and no
// flag waves them through.
func excused(skipped []review.Skipped, warnOnly map[string]bool) bool {
	if len(skipped) == 0 {
		return false
	}
	for _, skip := range skipped {
		if skip.Excuses == "" || !warnOnly[skip.Excuses] {
			return false
		}
	}
	return true
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

// demote turns the verification checks a caller named in --warn-only into
// advisories, for a repository that legitimately lacks the reviewed commit.
func demote(diagnostics []review.Diagnostic, warnOnly map[string]bool) []review.Diagnostic {
	for i, diagnostic := range diagnostics {
		if warnOnly[diagnostic.Name] {
			diagnostics[i].Severity = review.SeverityAdvisory
		}
	}
	return diagnostics
}
