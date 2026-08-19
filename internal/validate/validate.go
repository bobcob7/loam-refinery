// Package validate runs every check tier over one document and assembles the
// result. No tier gates another: a schema failure does not stop verification,
// and a verification failure does not stop the advisories.
package validate

import (
	"context"
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

func (v *Validator) verify(ctx context.Context, doc *review.Document, options Options) ([]review.Diagnostic, []review.Skipped, review.Verification) {
	repository, err := v.finder.Find(ctx, options.Dir)
	if err != nil {
		v.log.Debug("verification skipped", "reason", err)
		return nil, verify.SkipAll(err.Error()), review.Verification{Source: "none", Reason: err.Error()}
	}
	return repository.Verify(ctx, doc)
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
