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

// Validate parses and checks one document. It returns an error for the two
// things that are not findings about the review: input that is not a single
// JSON object, and a machine that could not be asked. Neither may be reported
// as a defect in the document.
func (v *Validator) Validate(ctx context.Context, source []byte, options Options) (*review.Result, error) {
	doc, err := review.Parse(source)
	if err != nil {
		return nil, fmt.Errorf("reading review document: %w", err)
	}
	result := &review.Result{Strict: options.Strict, Comments: len(doc.Comments)}
	result.Diagnostics = append(result.Diagnostics, v.structural.Check(doc)...)
	verified, skipped, verification, err := v.verify(ctx, doc, options)
	if err != nil {
		return nil, err
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

// verify has three outcomes, not two. Running outside a repository is ordinary
// and the anchor checks are simply reported as skipped; a repository that could
// not be reached at all is a broken machine, and returning it as an error keeps
// the run from exiting 0 with every anchor claim silently unexamined.
func (v *Validator) verify(ctx context.Context, doc *review.Document, options Options) ([]review.Diagnostic, []review.Skipped, review.Verification, error) {
	repository, err := v.finder.Find(ctx, options.Dir)
	switch {
	case errors.Is(err, verify.ErrNoRepository):
		v.log.Debug("verification skipped", "reason", err)
		return nil, verify.SkipAll(err.Error()), review.Verification{Source: "none", Reason: err.Error()}, nil
	case err != nil:
		v.log.Debug("verification unavailable", "reason", err)
		return nil, nil, review.Verification{}, fmt.Errorf("verifying anchors: %w", err)
	}
	diagnostics, skipped, verification := repository.Verify(ctx, doc)
	return diagnostics, skipped, verification, nil
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
