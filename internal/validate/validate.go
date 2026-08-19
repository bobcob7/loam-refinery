// Package validate runs every check tier over one document and assembles the
// result. No tier gates another: a schema failure does not stop verification,
// and a verification failure does not stop the advisories.
package validate

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/bobcob7/refinery/internal/review"
	"github.com/bobcob7/refinery/internal/verify"
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
		verified = append(verified, unverified(verification, skipped)...)
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
		return nil, verify.SkipAll(err.Error()), review.Verification{Source: "none", Reason: err.Error()}
	case err != nil:
		v.log.Debug("verification unavailable", "reason", err)
		return nil, verify.SkipAll(err.Error()), review.Verification{Source: "unavailable", Reason: err.Error()}
	}
	return repository.Verify(ctx, doc)
}

// unverified reports the one thing --require-verification asks about: whether
// the anchor claims were actually checked. A repository that did not answer and
// an anchor whose file could not be read are the same answer to that question —
// no one confirmed this — however different their causes.
//
// It is produced before demote, so --warn-only=verification-required demotes it
// like any other verification check. Asking for both is contradictory, but it is
// contradictory on the command line where a reader can see it, which beats a
// flag that silently outranks another.
func unverified(verification review.Verification, skipped []review.Skipped) []review.Diagnostic {
	if verification.Source == "repo" && len(skipped) == 0 {
		return nil
	}
	reason := verification.Reason
	if reason == "" && len(skipped) > 0 {
		reason = skipped[0].Reason
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
