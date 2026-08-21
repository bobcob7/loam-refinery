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
func (v *Validator) Validate(ctx context.Context, source []byte, options Options) (*review.Result, error) {
	doc, err := review.Parse(source)
	if err != nil {
		return nil, fmt.Errorf("reading review document: %w", err)
	}
	result := &review.Result{Strict: options.Strict, Comments: len(doc.Comments)}
	result.Diagnostics = append(result.Diagnostics, v.structural.Check(doc)...)
	verified, skipped, verification := v.verify(ctx, doc, options)
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
// anchor claims were actually checked. A repository that did not answer, an
// anchor whose file could not be read, and an anchor whose working-tree copy
// diverged from ref are the same answer to that question — no one confirmed
// this — however different their causes, and there is no flag to accept the
// gap instead.
func unverified(verification review.Verification, skipped []review.Skipped) []review.Diagnostic {
	if verification.Anchors == 0 {
		return nil
	}
	skipGap := verification.Source != "repo" || len(skipped) > 0
	divergedGap := len(verification.Unverified) > 0
	if !skipGap && !divergedGap {
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
